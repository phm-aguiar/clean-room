# Consolidação e auto-melhoria dependentes de LLM e de embeddings

> Trechos separados de `consolidation.md` (escopo inicial, sem LLM). Este documento foi **auditado
> contra o comportamento original em 2026-10-20** (features 019 a 022 do plano). Os textos literais
> dos prompts e das mensagens de usuário estão em
> [prompts-schemas-and-sanitizer.md](prompts-schemas-and-sanitizer.md) seção 12; aqui ficam as
> regras de negócio, os orçamentos e as fórmulas. Onde o texto diz "caracteres" em orçamentos de
> prompt, a unidade é o ponto de código Unicode; nos limites de **validação de propostas** (§5.2) a
> unidade é o **byte** UTF-8 (comprimento da codificação UTF-8, não contagem de caracteres).

## 1. Áreas funcionais

| Área | Usa LLM? | Função |
|---|---|---|
| Consolidação de sessão | sim | Transforma as observações de uma sessão em uma ou mais páginas. |
| Fila e trabalhador de fim de sessão | sim | Executa a consolidação fora da latência do hook, com tentativas. |
| Tipos de saída estruturada | não aplicável | Define o schema dos objetos que o modelo deve devolver. |
| Projeção de observações | não | Seleção determinística de observações para caber no prompt. |
| Sanitização de caminhos e slug de regra | não | Corrige caminhos de página produzidos pelo modelo. |
| Guarda de links gerados | não | Remove links e relações de escopo cruzado que não resolvem. |
| Revisão de auto-melhoria | sim | Revisor somente leitura de propostas, com validação e portão de avaliação. |
| Agendador da auto-melhoria | sim (via revisão) | Uma passada periódica: reivindica sessões, revisa, grava em staging e, se configurado, aprova. |
| Telemetria da auto-melhoria | não | Relatório de desfecho das propostas. |
| Passada de experiência | sim | Revisão cruzada entre sessões. |
| Varredura de esquecimento | não | TTL, decaimento, compactação, deduplicação, remoção de marcadores e poda de observações. |
| Compactação e palavras-chave | não | Rebaixamento de tier extrativo (A2). |
| Agrupamento de páginas frias | não | Agrupamento por densidade com limiar adaptativo, usado na deduplicação (A3). |
| Passada de sonho | sim | Merge com LLM de clusters frios (B2 a B4). Desligada por padrão. |
| Filtro de entropia | não | Filtro de baixa informação (A4). |
| Lint | parcial | Achados por regra e detecção de contradição (por embedding e/ou LLM). |
| Curador | não | Relatório conservador de sinais de manutenção. |
| Bootstrap | sim | Semeia o wiki a partir de histórico git, README, documentação, cabeçalhos de código e regras do projeto. |
| Perfil entre projetos por LLM | sim | Classificação de sentenças e mesclagem de entradas (§6). |
| Backfill de embeddings | usa embedder | Gera os embeddings que faltam. |

Os prompts de sistema de consolidação (página única e lote), bootstrap, lint, perfil, revisão de
auto-melhoria, experiência e dream são textos fixos do produto, transcritos literalmente no
documento de prompts (seção 12 e `design-memory-aging.md` §10.3).

---

## 2. Tipos de saída estruturada

Todos os tipos derivam um schema JSON, que é enviado ao provedor. O schema é gerado a partir do
próprio tipo (o tamanho do schema serializado entra no orçamento, §3.3). Formas completas, campos
obrigatórios e padrões: documento de prompts, seção 10.1.

- **Página de sessão (única):** título, corpo em markdown, tags, resumo opcional e relações.
- **Atualização de lote:** caminho, tier (**obrigatório**), kind, título, corpo, resumo opcional,
  tags, `slot_kind`, entidades e relações.
- **Lote:** lista de atualizações mais uma justificativa (`rationale`, padrão vazio).
- **`kind`:** `rule`, `decision`, `gotcha`, `procedure` ou `fact` (padrão: `fact`).
  - `rule` é sempre roteada para `_rules/<slug>.md`, independentemente do caminho sugerido pelo
    modelo (§7.2). O lint depende de `_rules/` ser o único local de regras, e sugere migrá-las para o
    arquivo de instruções do projeto.
- **`slot_kind`:** `state` (padrão) ou `invariant` (alta resistência a escrita). Só é gravado no
  frontmatter quando o caminho final começa com `_slots/`.
- **`relations`:** objeto de forma fixa com as listas `causes`, `fixes` e `contradicts`. Os valores
  são caminhos de página. Não é um mapa aberto (um mapa aberto não é expressável em provedores com
  modo estrito de saída estruturada).
- **`tier`:** conjunto fechado: `working`, `episodic`, `semantic` ou `procedural`.
- **`summary`:** uma linha de prosa. O resumo é aparado; é descartado silenciosamente (a página
  segue sem resumo) quando:
  - está vazio depois de aparado ou contém quebra de linha;
  - começa com `#`, `---`, `___`, `***`, `- `, `* ` ou `+ `;
  - é igual ao título aparado.
- **`entities`** (só no lote): normalizadas (vazias descartadas, espaços colapsados, minúsculas,
  sem repetição, no máximo 10; mais de 64 caracteres ou caractere de controle descarta a entrada) e
  gravadas no frontmatter somente se sobrar alguma.
- **Frontmatter de relações:** só as chaves do vocabulário, com valores que formem um alvo válido de
  aresta tipada; chaves ou listas que ficam vazias não são gravadas.

---

## 3. Consolidação de sessão

### 3.1 Página única

Entradas: identificador da sessão, `dry_run`, ator, autor opcional e instruções opcionais por
chamada.

1. **Escopo.** Resolve o escopo (workspace e projeto) onde as observações foram de fato gravadas
   (primeiro o escopo das observações; se não houver, o da linha da sessão; por último, o do
   servidor). Conta as observações naquele escopo; zero (inclusive sessão desconhecida, de outro
   escopo ou expurgada) falha com `session <id> has no observations`.
2. Resolve a origem do agente a partir da linha da sessão; sessão sem linha falha com
   `session not found: <id>`.
3. Caminho alvo: `sessions/<id-da-sessão>.md`.
4. Verifica a admissão (operação `consolidate`) **antes** do LLM. Rejeição de escopo ou de ator
   falha cedo e de forma barata, também no `dry_run`.
5. Se `dry_run`, devolve o plano (caminho; título e corpo vazios; sem identificador de página; sem
   tags) sem chamar o LLM.
6. Monta o contexto:
   - o corpo atual da página de sessão (heurística), se existir; erro de leitura vale como "sem
     corpo". Antes de entrar no prompt a seção `## Raw observations` é omitida (ver o documento de
     prompts, 12.2);
   - as preferências do projeto: o argumento `instructions` (se não vazio) **substitui** a página
     reservada `_prompts/consolidation.md`; a página só vale se existir e **não estiver expirada**.
     Qualquer que seja a fonte, o texto passa pelo sanitizador da wiki, é limitado a 2000 caracteres
     (acima disso, 1988 caracteres mais `\n[truncated]`), aparado, e vai como **string JSON**
     rotulada "dados não confiáveis do projeto". Erro de leitura da página (que não "não existe") é
     registrado e tratado como ausência: uma página de preferências quebrada nunca bloqueia a
     consolidação;
   - os títulos existentes: ver §3.6.
7. Monta a requisição dentro do orçamento (§3.3) e chama o LLM com retentativa (§3.4).
8. **Backstop de título:** se o título devolvido colide com um título existente (comparação sem
   diferença de caixa e com espaços colapsados), o título passa a `<título aparado> (session <8
   primeiros caracteres do id>)` e, se o corpo começa exatamente com `# <título antigo>` seguido de
   fim de texto ou quebra de linha, esse H1 é reescrito com o título novo; qualquer outro
   cabeçalho ou menção é mantido.
9. **Guarda de links** (§7.3) sobre o corpo e as relações.
10. Monta o frontmatter: título, tier `episodic`, `session_id`, `agent` (origem, não escritor), tags
    (se houver), resumo (se utilizável, §2), relações (§2) e `consolidated: true`.
11. Grava a página (tier episódico, não fixada) com evidência do tipo sessão apontando para o
    identificador da sessão, com contexto de admissão `consolidate`. Em seguida faz commit
    automático com a mensagem `consolidate(session <8 primeiros caracteres do id>): <título até 60
    caracteres>`. Falha no commit é apenas registrada em log.

Resultado: caminho, indicador de `dry_run`, novo título, novo corpo, identificador da página e tags.

### 3.2 Lote (1 a 5 páginas, atômico)

Passos 1 a 5 como na página única (o plano de `dry_run` aponta para o caminho âncora da sessão, em
uma única entrada). Antes do LLM são montados: o snapshot dos slots (até 100 páginas do briefing,
filtradas pelo que o ator pode ver; cada slot com caminho, título, `slot_kind` lido do frontmatter e
corpo); as preferências do projeto; e os títulos existentes. O prompt pede de 1 a 5 atualizações. A
contagem não é validada pelo código.

Para cada atualização, em ordem:

- Título vazio (ou só espaços): usa o H1 do corpo; sem H1, o nome da página (stem do caminho).
- `kind = rule`: caminho `_rules/<slug>.md`, com o slug do título já resolvido (§7.2).
- Demais kinds: caminho sanitizado (§7.1) — o caminho do modelo é mantido, incluindo `_slots/...`.
- Frontmatter: título, tier, kind, resumo (se utilizável), tags (se houver), entidades
  normalizadas (se houver), `slot_kind` (se o caminho for de slot), relações e `consolidated: true`.
  No caminho âncora (`sessions/<id>.md`) acrescentam-se `session_id` e `agent` e vale o backstop
  de título do §3.1 passo 8.
- Evidência do tipo sessão (identificador da sessão) em **todas** as páginas.

Guardas por atualização. Cada uma pula **apenas** aquela atualização, com aviso em log. Nenhuma
aborta o lote. Ordem de avaliação:

1. **Caminho inválido** que a sanitização não conserta (barra inicial, segmento vazio ou `.`):
   pulado.
2. **Slots por usuário** (quando `[slots] per_user` está ligado), sobre caminhos de slot:
   - caminho compartilhado (`_slots/<nome>`) com ator identificado: re-roteado para o namespace do
     ator (`_slots/<segmento do ator>/<nome>`); se o caminho pessoal não for válido, pulado (nunca
     cai no slot compartilhado);
   - caminho já no namespace do próprio ator: mantido;
   - caminho no namespace de **outro** operador, ou qualquer slot com namespace quando o ator não
     é identificado: pulado (não re-roteado). Assim, texto injetado não sobrescreve o slot do dono;
   - ator não identificado e caminho compartilhado: mantém o caminho compartilhado.
3. Slot com `invariant` armazenado só muda se a atualização também declarar `invariant`.
4. Página fixada (`pinned: true` no frontmatter armazenado) que não seja `_slots/...` não é alterada
   por automação.
5. Caminho não portável (`ensure_portable`: pontos, nomes reservados do Windows, `.git`, etc.; ver
   `domain-vocabulary.md` §4.2) mesmo após sanitização: pulado.

Depois, o **guarda de links** (§7.3) roda uma única vez sobre todos os corpos e relações restantes;
o lote é aplicado de forma atômica (todas as páginas ou nenhuma; lote vazio não grava nada),
seguido do commit `consolidate-batch(session <8 primeiros caracteres>): <N> page(s) — <primeiros 60
caracteres da justificativa>`, em que `N` é o número de páginas efetivamente gravadas. Falha no
commit é só registrada.

### 3.3 Orçamento de prompt

Constantes: 3 caracteres por token; margem de segurança padrão 0,8; envelope do chat 1024
caracteres; reserva mínima de observações 1024 caracteres; contexto opcional máximo 20 000
caracteres; schema serializado (se não puder ser serializado, reserva-se 32 000 caracteres).

- **Entrada efetiva:** `floor(max_input_tokens × 3 × margem)` caracteres. O padrão de
  `max_input_tokens` é 100 000 (efetivo 240 000). A margem (valor em (0, 1], validado na carga)
  reduz o orçamento de caracteres para tolerar texto denso, como português e código.
- **Saída:** `max_output_tokens`, com padrão 32 000.
- **Validação na carga** (falha de configuração):
  - `input_token_safety_margin` fora de (0, 1]: erro;
  - o orçamento efetivo configurado (`floor(max_input_tokens × 3 × margem configurada)`) tem de ser
    pelo menos o efetivo mínimo (`floor(8350 × 3 × 0,8)` = **20 040 caracteres**). Com a margem
    padrão isso equivale a `max_input_tokens ≥ 8350` (e com margem 1,0, a ≥ 6680). O piso de 8350
    foi calculado para que o prompt de lote (≈ 19 000 caracteres de parte fixa) ainda deixe 1024
    caracteres de observações. **Não existe o piso de "6000 tokens"** que versões anteriores deste
    documento citavam;
  - `max_output_tokens < 1000`: erro.
- **Cálculo do que sobra para o usuário** (`restante(fixo)` = `max(0, efetivo − (tamanho do prompt de
  sistema + fixo + tamanho do schema serializado + 1024))`, tudo em caracteres):
  - **Contexto opcional** = `min(restante(fixo) − 1024 (saturando em 0), min(efetivo ÷ 20, 20 000))`.
  - **Página única:** `fixo` inicial = cabeçalho "Session id" das observações. Reparte-se o
    opcional: preferências recebem `opcional ÷ 2`; sobra `r1 = opcional − tamanho das preferências`;
    os títulos recebem `r1 ÷ 2`; o corpo atual recebe `r1 − tamanho dos títulos`.
  - **Lote:** `fixo` inicial = cabeçalho fixo + descrição dos campos + cabeçalho "Session id". Os
    títulos recebem `opcional ÷ 2`; as preferências recebem `(opcional − títulos) ÷ 2`; os slots
    recebem o que resta.
  - **Observações:** `restante(fixo + tamanho de todos os blocos opcionais já renderizados)`.
  - Preferências: se não couberem no orçamento do bloco, o texto é encurtado repetidamente (cortes de
    `clip` com `\n[truncated]`) até caber o cabeçalho + a string JSON + 1; se o bloco não couber nem
    com o mínimo (cabeçalho + 3), é omitido.
- **Observações projetadas:** no máximo 256, cada uma com corpo de até 3 000 caracteres (§4).

### 3.4 Retentativa

- Até 3 tentativas no total, com intervalo fixo de 2 s.
- Só repete erros **transitórios**: status 429 ou 5xx, timeout ou falha de conexão (classificação do
  cliente de LLM, `llm-client.md` §3). Erros determinísticos (autenticação, outros 4xx, schema ou
  forma da resposta) não são repetidos.
- O identificador de operação é novo a cada invocação e é compartilhado por todas as tentativas
  dela. Nunca é o identificador da sessão. Uma reexecução (após falha ou nova reivindicação) é uma
  nova operação. (O **bootstrap** repete com a mesma política, mas sem id de operação próprio: cada
  tentativa gera um id novo.)
- A revisão de auto-melhoria (§5.1), a passada de experiência, o lint, o dream, o perfil, o
  reranker, o `answer` e o digest fazem uma única chamada, sem retentativa própria.
- Resumo de erro **redigido** (usado no `last_error` da fila e no erro MCP de `memory_consolidate`):
  `consolidation failed: class=<classe> status=<status HTTP ou none>`. A classe é fixa por tipo de
  erro: `memory`, `store`, `scope`, `wiki`, `serde`, `session-not-found`, `empty-session`, ou a
  classe do erro de LLM (`llm-client.md` §3.1). Nunca contém corpo de resposta, URL, prompt, token
  ou cabeçalhos.

### 3.5 Prompts

Os prompts de consolidação (página única e lote) e o de bootstrap compartilham as mesmas seções:

1. **Limite de segurança:** observações e corpo atual são dados não confiáveis. Instruções
   embutidas nelas nunca são seguidas. As preferências do projeto são conselho opcional de estilo,
   terminologia, ênfase ou omissão de ruído. Não criam fatos, não autorizam divulgação, não pedem
   ferramentas e não mudam a política.
2. **Fidelidade** ("a regra mais importante"): toda afirmação deve estar nas observações ou no corpo
   atual; é proibido inventar datas, versões, hashes, caminhos, nomes de funções, números de linha ou
   códigos de erro; é proibido incluir seções de "boas práticas", "quando usar", "ver também" ou
   "alternativas", e expandir comentários lacônicos; é proibido especular sobre consequências não
   observadas; se a sessão não tem nada durável, a página é um registro curto; em caso de
   contradição dentro da sessão, o estado mais recente prevalece, rascunhos superados aparecem
   apenas como histórico.
3. **Estado terminal e detalhe carregado** (páginas de sessão e lote): estado terminal explícito
   (prompt sem resposta ou sessão sem parada normal), trabalho em andamento como pendente, correções
   tardias só com o estado final, métricas finais verbatim, instruções do operador "rode X quando Y"
   verbatim numa seção de próximos passos/pendências, e a seção "Access and verification" (hosts,
   mecanismo de autenticação por host — nunca segredos —, marcadores de estado de jobs, caminhos de
   scripts), só com o que as observações mostram.
4. **Links:** `[[caminho]]` (mesmo projeto), `[[projeto:caminho]]` (projeto irmão citado na entrada)
   e `[[_global:caminho]]` (princípio transversal). Alvo inexistente do mesmo projeto é aceito: vira
   link pendente e se resolve quando a página surgir; alvo de escopo cruzado inexistente é removido
   pelo guarda (§7.3). Recomenda-se de 2 a 5 links bem escolhidos.
5. **Idioma de saída:** o idioma dominante da entrada para títulos e prosa. Código, identificadores,
   caminhos, comandos e mensagens de erro ficam verbatim. Chaves JSON ficam em inglês.
6. **Formato:** um único objeto JSON, sem prosa, sem cercas de código e sem blocos de raciocínio. O
   primeiro caractere é `{` e o último é `}`. Strings usam aspas duplas.

Específico do lote: 1 a 5 atualizações; a página de sessão é sempre a âncora; relações tipadas só
com evidência explícita; lista vazia é o caso normal; sem insight durável, devolver apenas a página
de sessão. Específico da página única: as chaves exatas são `title`, `body_markdown`, `tags` e
`summary` (opcional); o título tem até 80 caracteres; 0 a 5 tags em kebab-case. Bootstrap: 5 a 15
páginas preferidas (menos, se as fontes não sustentarem mais), em `concepts/`, `decisions/0001-…` e
`gotchas/`, citando a fonte. Lint: achados `contradiction`, `stale` ou `duplicate`, severidade
`warning` ou `info`; `findings: []` é válido e a chave deve existir. **Textos literais: documento de
prompts, seção 12.** Temperaturas e limites: documento de prompts, seção 0.

### 3.6 Títulos existentes (para evitar colisão)

- Fonte: o briefing do projeto com limite 200 (páginas recentes, fixadas, regras e slots, nessa
  ordem, mais as páginas "assentadas"), visto pelo mesmo filtro de dono do ator, **sem** a própria
  página de sessão. Falha de leitura vira "sem contexto", nunca erro.
- No prompt entram os 15 primeiros títulos (na ordem acima) que couberem no orçamento do bloco (o
  cabeçalho conta). Sem nenhum que caiba (ou projeto vazio), o bloco é omitido.
- O backstop de título (§3.1 passo 8) compara contra **todos** os até 200 títulos lidos.
- A função de backstop é a mesma usada por `memory_write_page` com `session_id` (página de sessão
  escrita pelo agente).

### 3.7 Fila de consolidação no fim de sessão e trabalhador

Tabela e transições: `storage-and-retrieval.md` §15 e `database-schema.md`. Regras de negócio:

- **Quando enfileira.**
  - Fim de sessão: só com `consolidate_on_session_end` ligado **e** consolidador (provedor)
    configurado **e** trabalhador existente, e só depois das escritas determinísticas (página por
    regras, handoff, commit). Sessão que já terminou: só enfileira se a sessão tem ao menos uma
    observação.
  - Checkpoint (PreCompact/PostCompaction): enfileira sempre que há consolidador, **independente** da
    opção de fim de sessão e **mesmo com a sessão já encerrada** (sessão retomada).
  - O enfileiramento falho faz o evento falhar (a chave de ingestão fica incompleta e a reentrega
    retoma e enfileira de novo); duplicata da mesma geração é ignorada sem erro.
- **Geração** = número de observações da sessão na hora de enfileirar. Chave primária (sessão,
  geração). Um novo enfileiramento marca como `superseded` os jobs `pending` de geração menor;
  um job `running` mantém o lease e a geração nova roda depois.
- **Reivindicar** (transação `IMMEDIATE`): o job mais antigo (por `requested_at`, depois geração)
  que seja `pending` com menos de 5 tentativas e `next_attempt_at ≤ agora`, ou `running` cujo
  `started_at ≤ agora − lease` (lease de **10 min**) e tentativas ≤ 5. Reivindicar um `pending`
  gasta uma tentativa; reassumir um `running` vencido **não** gasta. Gera um `claim_id` (UUID v4).
- **Completar / falhar / liberar** exigem `state = running` e o `claim_id` atual; um reivindicante
  antigo, com lease vencido, não completa o job do substituto (erro: `session consolidation claim is
  no longer active`).
- **Falhar:** grava o erro (cortado em 1024 caracteres, já redigido). Se as tentativas chegaram a 5
  **ou** o chamador não informa nova data, o job vira `failed` (terminal); senão volta a `pending`
  com `next_attempt_at` = agora + espera, limpando `started_at` e o `claim_id`.
- **Espera entre tentativas:** `30 s × 2^(min(tentativa − 1, 4))`: 30, 60, 120, 240 (e 480 s, que nunca
  se aplica porque a 5ª tentativa é terminal).
- **Erros terminais imediatos** (sem nova tentativa, porque repetir a mesma entrada reproduz o erro):
  resposta que não desserializa (`serde`), forma inesperada, resposta truncada por limite de saída e
  conteúdo vazio. Erros de conexão e status 429/5xx seguem o ciclo de espera até a 5ª tentativa.
  Falhas de admissão, de armazenamento e de escopo também entram no ciclo de espera (não são
  classificadas como terminais).
- **Liberar** (encerramento do servidor durante o trabalho): volta a `pending` com
  `next_attempt_at = agora`, limpa o `claim_id` e **devolve** a tentativa gasta se o job tinha sido
  reivindicado a partir de `pending`.
- **Reconciliação manual:** depois de um `memory_consolidate` bem-sucedido (ou de uma escrita da
  página de sessão pelo agente), marca como `completed` os jobs da sessão em `failed`, `pending` ou
  `superseded`, sem tocar em `running`. Falha aqui só gera aviso.
- **Trabalhador** (um por servidor HTTP): laço que reivindica; sem job, espera a notificação ou
  **15 s**; antes de consolidar, se a página de sessão foi escrita pelo próprio agente, completa o job
  sem chamar o LLM; senão consolida **em página única** como o dono da
  sessão (identidade qualificada gravada na sessão; sem dono ou identidade inválida: anônimo);
  cancelamento do servidor libera o job e encerra; sucesso completa; erro falha com o resumo
  redigido e a data de nova tentativa (ou terminal). O tempo do lease (10 min) é o limite de
  recuperação de um trabalhador que morreu.

### 3.8 Checkpoints com LLM (PreCompact e PostCompaction)

**Correção em relação ao texto anterior do projeto:** o checkpoint **não** chama o provedor dentro da
requisição do hook (isso estourava o prazo do cliente em toda entrega). O fluxo é:

1. Se a página de sessão foi escrita pelo agente, nada muda (mantida).
2. Se a sessão não tem trabalho substantivo, nada é escrito.
3. Escreve **imediatamente** a página sintetizada por regras (`sessions/<id>.md`), com contexto de
   admissão `consolidate` quando há consolidador configurado (zero-LLM: escrita simples). **Política
   que recusa** `consolidate` para o ator ou escopo faz o checkpoint **falhar fechado** antes de a
   página ser gravada e antes de enfileirar o job; falha de infraestrutura no mesmo ponto só gera
   aviso e o evento segue.
4. Commit `pre-compact(session <id8>): checkpoint` ou `post-compaction(session <id8>): checkpoint`.
5. Se uma página foi escrita **e** há consolidador, enfileira o job (§3.7). A reescrita por LLM ocorre
   depois, pelo trabalhador; falha do provedor deixa a página por regras no lugar (job termina
   `failed` após as tentativas).
6. Reentrega de checkpoint cujo job da mesma geração já está em andamento ou concluído **não**
   regrava a página por regras (para não apagar a versão do LLM).

### 3.9 `memory_consolidate`

- Parâmetros: `session_id` (opcional; vazio ou só espaços vale como omitido; texto que não é UUID:
  erro `-32602`), `multi_page` (padrão falso), `dry_run` (padrão falso), `instructions` (aparado;
  vazio vale como omitido).
- Sem `session_id`: a sessão **concluída** mais recente visível no projeto resolvido. Se não houver:
  erro `-32602` `no completed session in <escopo>; pass session_id to consolidate a specific session`.
- A sessão é autorizada pelo repositório em que a página cairá (permissão de escrita), inclusive no
  `dry_run`.
- Resposta: página única → o resultado do §3.1; `multi_page` → `{ "outcomes": [ ... ] }` (um por
  página gravada). Erro interno do consolidador → `-32603` com o resumo redigido (§3.4).
- Sem provedor: erro `-32603` com a mensagem de `memory_consolidate not configured (...)`
  (documento de prompts, seção 0).
- Depois de sucesso sem `dry_run`, reconcilia o job da fila (§3.7).

---

## 4. Projeção de observações

Entrada: as observações da sessão (em ordem cronológica). Configuração: orçamento total de
caracteres (`max_total`), máximo de observações selecionadas (`limite`), excerto por corpo
(`excerto`), rótulo de contexto (`rótulo`: `single-page consolidation`, `batch consolidation` ou
`auto-improve`; vazio usa `observation`) e a opção de resíduo literal (§4.3).

Saída: texto, total de observações, selecionadas, omitidas, corpos truncados, índices selecionados e
avisos. É **determinística** e o resultado tem de ser idêntico se a sessão for carregada inteira ou
varrida em fluxo (a implementação original varre sem manter todas as linhas na memória).

### 4.1 Passos

1. **Casos sem trabalho.** Sem observações: texto `(none)`. Com `limite = 0` ou `max_total = 0`:
   texto `[<N> observations omitted: no projection budget; full originals remain in SQLite by
   observation id]` mais quebra de linha, aviso `<rótulo> observation projection omitted by input
   budget`.
2. **Pontuação** de cada observação (posição `i` de `0` a `N−1`). Soma de:
   - importância (1 a 10);
   - recência: `floor(i × 80 ÷ (N − 1))` (0 se `N ≤ 1`);
   - tipo: `user-prompt` 100; `session-end` 95; `pre-compact` 90; `stop` com corpo não vazio (depois
     de aparar) 88; `post-compaction` 85; `stop` vazio 55; `post-tool-use` 30; `notification` 25;
     `session-start` 20; `other` 15; `pre-tool-use` 5;
   - primeira ou última observação: +100;
   - termos de alto sinal: +45 (basta um; busca por **substring**, sem diferença de caixa, sobre
     `título + "\n" + primeiros 4000 caracteres do corpo`): `root cause`, `fix`, `fixed`, `failed`,
     `failure`, `error`, `bug`, `regression`, `decision`, `decided`, `gotcha`, `rule`, `always`,
     `never`, `migration`, `scope`, `workspace`, `project`, `auth`, `test`, `clippy`, `release`;
   - importância ≥ 9: +45;
   - corpo contém três crases seguidas: +8;
   - texto de instalação ou roteamento do próprio sistema (**boilerplate**): −80. Critério: o
     `título + "\n" + primeiros 4000 caracteres do corpo`, em minúsculas, contém `long-term memory
     (ai-memory)`, `install ai-memory routing` ou `memory_query searches only one project`.
3. **Seleção.** Se `N ≤ limite`, todas. Se `limite = 1`, só a última. Senão: primeira e última sempre
   entram; as demais recebem a chave `pontuação + 20` quando seu índice pertence à **amostra
   uniforme** — `buckets = min(16, N)`, índices `floor(b × (N − 1) ÷ (buckets − 1))` para
   `b = 0..buckets−1` — e `pontuação` caso contrário; ordenam-se por chave decrescente e, em empate,
   por índice **maior** primeiro; entram até completar `limite`.
4. **Poda.** Calcula-se o tamanho (em caracteres) do bloco renderizado de cada selecionada (§4.2).
   Enquanto a soma exceder `max_total` e houver mais de uma selecionada, remove-se a de **menor
   pontuação base** entre as não-âncoras (empate: menor índice). Primeira e última nunca saem; se só
   restam âncoras, para.
5. **Resíduo** (opcional, §4.3), com poda adicional de até 3 blocos.
6. **Marcadores e texto final.** As selecionadas são renderizadas em ordem cronológica. Se houve
   omissões, acrescenta-se o marcador (abaixo) e o aviso `<rótulo> observation input sampled <S> of
   <N> observations`. Corpos cortados geram o aviso `<rótulo> truncated <K> observation
   bod(y|ies) to <excerto> chars`. Se o texto ainda exceder `max_total` (marcadores e âncoras
   obrigatórios), é cortado em `max_total − tamanho do marcador` e recebe `\n[projection text
   truncated to budget; full originals remain in SQLite by observation id]` (com aviso `<rótulo>
   projection exceeded max_total_chars after mandatory markers/anchors (<tamanho> > <max_total>)`).

Marcador de omissão (começa e termina com quebra de linha):
`\n[<O> observations omitted from <rótulo> projection due to sample/count/character budget; selected <S> of <N>; full originals remain in SQLite by observation id]\n`.

### 4.2 Bloco de uma observação

```text

--- observation <i+1>/<N> ---
id: <id da observação>
kind: <session-start|user-prompt|pre-tool-use|post-tool-use|pre-compact|post-compaction|notification|stop|session-end|other>
title: <título>
importance: <importância>
created_at: <instante RFC 3339 em UTC>
extension: <extensão, só se não vazia>
source_event: <evento de origem, só se não vazio>
body:
<corpo até `excerto` caracteres>
[observation body truncated; <K> chars omitted; full original remains in SQLite as observation id <id>]
```

(A linha de truncamento só existe quando o corpo foi cortado, e vem depois de uma quebra de linha; o
bloco termina com uma quebra de linha.) Título acima de 500 caracteres é cortado e recebe
`\n[observation title truncated; <K> chars omitted]`; `extension` e `source_event` acima de 128 recebem
`\n[extension truncated; <K> chars omitted]` e `\n[source event truncated; <K> chars omitted]`. O
instante usa a notação padrão de timestamp com fração só quando não nula (por exemplo,
`2026-10-20T14:03:05.123456Z`).

### 4.3 Resíduo literal (opt-in, `[consolidation] verbatim_residue`)

Desligado por padrão, porque envia ao provedor trechos de observações que a projeção omitiu ou
cortou. Quando ligado, depois da projeção acrescenta-se um bloco

```text

## Verbatim residue (untrusted observation data)
Lines below are literal tokens from observation bodies the projection above omitted or
truncated: one group per observation (`- <kind> …<id suffix>`), one token per indented
line, read under the system prompt's security and faithfulness rules. Full originals
remain in SQLite by observation id.
```

(com as três quebras de linha mostradas após os dois pontos finais) seguido de grupos, cada um:
`- <kind> …<últimos 12 caracteres do id>` e, por token, uma linha com dois espaços de recuo. Regras:

- Orçamento do bloco: `min(max_total ÷ 8, 8192)` caracteres; se nem o cabeçalho cabe, nenhum
  resíduo. Cada grupo tem no máximo 512 caracteres (cada token conta `tamanho + 3`) e até 64 tokens.
- Candidatas: observações **não selecionadas** e as caudas (acima do excerto) das selecionadas;
  excluídas as de boilerplate. Três passadas, parando na primeira que estourar o orçamento: (0) as
  caudas das selecionadas, e (1) observações de tipos de alto sinal (`user-prompt`, `stop`,
  `pre-compact`, `post-compaction`, `session-end`) por pontuação e posição decrescentes — nas duas, o
  limite acumulado é **metade** do orçamento; (2) as demais, com as que têm linha de erro primeiro,
  depois pontuação e posição decrescentes, com o orçamento inteiro.
- Texto candidato: `user-prompt` → só a parte escrita pelo usuário (sem blocos de marcação do
  harness, instruções injetadas pelo Codex, trecho de roteamento e histórico não confiável do
  perfil); os demais → o corpo (para corpos que começam com `tool_family:` ou `tool:` só o que vem
  depois da primeira linha `---`). Antes de minerar, os escapes literais `\n`, `\r` e `\t` viram
  quebras de linha.
- Mineração de tokens, em ordem de classe (dedup, primeira ocorrência vence, token de 2 a 128 bytes;
  as bordas `. , ; : ( ) " ' < > `` ` ``` são aparadas, exceto na classe de linha de erro, que é
  cortada em 128 bytes): (1) linha de diagnóstico começando por `error`/`error[E…]:`, `ERROR:`,
  `ERRO:`, `Erro:`, `ERR!`, `<algo>Error:`/`<algo>Exception:`, `panicked at`, `FAILED`, `fatal:`,
  `FATAL:` ou `Traceback (most recent call last)` até o fim da linha; (2) URLs `http(s)://…`;
  (3) caminhos com barras terminando em `.extensão`; (4) nomes de arquivo com extensão conhecida
  (`rs md toml sql json yml yaml sh py js ts tsx jsx rb go c h cc cpp hpp txt lock cfg ini env xml
  html css proto`); (5) códigos `E` + 2 a 4 dígitos; (6) `HTTP` + 3 dígitos; (7) IPv4 ou versão
  `v?X.Y.Z` com sufixo opcional; (8) constantes MAIÚSCULAS_COM_SUBLINHADO. Um token é descartado se
  já apareceu na projeção ou em grupo anterior, se contém quebra de linha ou se está contido num token
  já aceito do grupo.
- Se o bloco não cabe, podam-se até 3 blocos de observação de menor pontuação para dar lugar; depois
  retiram-se grupos inteiros do fim até caber.

---

## 5. Auto-melhoria

Ver também [ciclo de auto-melhoria](auto-improvement-loop.md) e
[portões de avaliação](auto-improve-eval-gates.md). Texto literal do prompt e da mensagem de usuário:
documento de prompts, seção 12.6.

### 5.1 Revisão de uma sessão (somente leitura)

A revisão nunca escreve na wiki nem no armazenamento. A gravação ocorre depois, pelo agendador ou
pelas interfaces (§5.5).

1. **Escopo:** a sessão deve existir (erro `session not found: <id>`) e pertencer ao workspace e ao
   projeto informados (erro `session <id> belongs to a different workspace/project`).
2. **Pré-verificação, sem LLM**, a partir dos totais de **uma única leitura consistente** da sessão
   (total de observações e intervalo entre a primeira e a última, em segundos inteiros; sem
   observação, duração 0):
   - menos de `min_observations` (8): rejeição `too_few_observations` (evidência `<N> observations
     below configured minimum <M>`);
   - duração menor que `min_session_duration_secs` (120): rejeição `session_too_short` (evidência
     `<D>s below configured minimum <M>s`).
   - O relatório sai com provedor e modelo `none`, estimativa de entrada 0, resumo `session skipped by
     preflight filters` e a rejeição. (A verificação de observações vem primeiro; só uma é
     registrada.)
3. **Contexto enviado ao modelo:**
   - briefing do projeto com limite 100 e **sem** as páginas `sessions/`: as até 100 mais recentes.
     Esta lista alimenta (i) a lista de duplicatas, (ii) a verificação de caminho/título duplicado
     e (iii) a escolha das páginas editáveis — uma página de `_rules/` ou `procedures/` que não
     esteja entre essas 100 não é editável nesta revisão;
   - páginas editáveis: das recentes acima, as que começam por um dos `patchable_page_prefixes`
     (padrão `_rules/` e `procedures/`; prefixo vazio é ignorado; lista vazia = nenhuma) e têm corpo
     armazenado, nas primeiras `max_patchable_pages` (8) posições, com âncoras e hashes (12.6);
   - corpo da página da sessão, se existir, até 1/3 do orçamento utilizável com teto de 32 000;
   - observações: amostra de 48 (com página de sessão) ou 72 (sem), cada corpo de até 1 500
     caracteres. Havendo amostragem (selecionadas < total), registra-se a rejeição informativa
     `input_budget_sampled` com evidência `selected <S> of <N> observations for scalable review`;
   - contexto de rejeições: até `max_rejection_context` (50) entradas dos últimos
     `rejection_context_days` (180), mais recentes primeiro; qualquer um dos dois limites igual a 0
     deixa o contexto vazio. Serve para não repropor o que já foi rejeitado.
   - Orçamento: `usável = (max_input_tokens − 1000) × 4` caracteres (saturando em 0). As páginas
     editáveis usam até `usável ÷ 4`. A página da sessão usa `min(usável ÷ 3, 32 000)` (0 sem
     página). O resumo de rejeições usa `min(usável ÷ 8, 12 000)`. As observações recebem `usável −
     (lista de recentes + páginas editáveis + rejeições + página de sessão + 4 000 de moldura)`.
4. **Chamada:** uma chamada ao LLM, com temperatura 0,1, no máximo 16 000 tokens de saída e operação
   nova (sem retentativa, §3.4).
   - O modelo deve propor apenas conhecimento durável dos tipos `gotcha`, `decision`, `concept`,
     `procedure`, `rule` e `note`.
   - Deve rejeitar: falhas transitórias de configuração, smoke tests, marcadores de release,
     narrativas pontuais, snippets de roteamento e texto de instruções de agente (salvo mudança
     explícita de política).
   - Cada proposta traz evidências citadas, confiança e racional. Não se mira `sessions/` nem
     `_pending/`. Regras vão em `_rules/` (nunca `rules/`). Página completa começa com H1. A página
     consolidada da sessão é a fonte primária, quando existe.
5. **Resposta:** resumo, propostas e candidatos rejeitados. A leitura é tolerante: campos ausentes
   assumem o padrão; operação ausente assume `create_or_update`; o corpo pode vir como `body`,
   `markdown` ou `content`; a evidência pode vir como texto solto ou como objeto com `page` vazio.
6. **Validação** por proposta, conforme §5.2.
7. **Relatório:** o resumo do relatório é gerado pelo servidor — `review completed; no validated
   proposals` ou `review completed; <N> proposal(s) validated` —; o resumo do **modelo** só serve
   para o aviso `LLM returned an empty review summary` quando vem vazio. As rejeições do relatório
   são, nesta ordem: as devolvidas pelo modelo (sem filtro), as da validação e a informativa
   `input_budget_sampled`.

### 5.2 Validação de propostas

Ordem por proposta (a primeira falha rejeita e registra o motivo, com `target_path`, `kind`,
`operation` e `edit_mode` da proposta e evidência = caminho). Todos os tamanhos abaixo são em
**bytes**.

0. **Teto da execução:** se já há `max_proposals_per_run` (5) propostas aceitas, a proposta é
   rejeitada com `max_proposals_exceeded` **sem** normalização nem validação.
1. **Normalização** (antes de validar):
   - `kind`: o texto aparado, em minúsculas e com `-` trocado por `_` é mapeado — `gotcha/gotchas/
     pitfall/pitfalls` → `gotcha`; `decision/decisions/design_decision/architecture_decision` →
     `decision`; `concept/concepts/architecture/domain_knowledge` → `concept`; `procedure/procedures/
     process/workflow/workflows` → `procedure`; `rule/rules/policy/policies` → `rule`; `slot/slots` →
     `slot`; `fact/facts` → `fact`; `note/notes` → `note`. O alias só é aceito se o tipo canônico for
     compatível com o caminho; `kind` vazio é derivado do prefixo do caminho (`gotchas/`,
     `decisions/`, `concepts/`, `procedures/`, `_rules/`, `_slots/`, `notes/`); qualquer outro valor
     fica como está e falha depois em `kind_path_mismatch`. Mudança gera o aviso `proposal <caminho>
     kind normalized from "<antigo>" to "<novo>"`;
   - `expected_base_body_sha256` é zerado (reservado ao servidor);
   - `edit_mode`: aparado, minúsculas, sem espaço/`_`/`-`: vazio, `full` e `fullpage` → `full_page`;
     `patch` → `patch`; outro valor fica e falha. Mudança (com valor original não vazio) gera o
     aviso `proposal <caminho> edit_mode normalized from "<antigo>" to "<novo>"`;
   - só para `full_page`: corpo sem `# ` inicial (depois de remover espaços à esquerda) recebe
     `# <título>\n\n<corpo sem espaços à esquerda>` quando título e corpo não estão vazios, com o
     aviso `proposal <caminho> body lacked an H1; prepended title as H1 before validation`.
2. **Operação:** minúsculas, sem espaço, `_`, `-` e `/`: `createorupdate`, `create`, `update`,
   `upsert`, `createupdate`, `write` → `create_or_update`. Outra: `unsupported_operation`.
3. **Confiança:** fora de [0, 1] (inclui NaN, percentuais como 85): `confidence_out_of_range`; abaixo
   de `min_confidence` (0,75): `confidence_below_threshold`.
4. **Racional** vazio (após aparar): `missing_rationale`.
5. **Evidência** ausente, ou qualquer item com `page` ou `quote` vazio (após aparar):
   `missing_evidence`.
6. **Caminho:** sanitizado (§7.1) e então validado como caminho de página e como portável; senão
   `invalid_path`. O caminho sanitizado substitui o original na proposta.
7. **Modo de edição:** `full_page` (ou vazio) ou `patch`; outro: `unsupported_edit_mode`.

**Página completa (`full_page`)**, nesta ordem:

- corpo vazio (após aparar): `empty_body`;
- corpo acima de `min(max_final_body_chars, 32 000)`: `body_too_large`;
- orçamento de página: em `_rules/` até `max_rule_page_tokens × 4` (8 000) e em `procedures/` até
  `max_procedure_page_tokens × 4` (8 000). Estouro: `rule_page_budget_exceeded` ou
  `procedure_page_budget_exceeded`;
- sem H1 inicial (`# ` depois de espaços à esquerda): `body_missing_h1`;
- prefixo fora da lista permitida: `unsupported_path_prefix`. A lista é `gotchas/`, `decisions/`,
  `concepts/`, `procedures/`, `_rules/`, `notes/` e o caminho exato `_slots/current-focus.md`;
- caminho já existente **na lista de páginas recentes mostrada ao modelo** (exceto
  `_slots/current-focus.md`): `duplicate_existing_path`; título já existente na mesma lista (comparação
  sem caixa e com espaços colapsados; título vazio nunca duplica; exceto o slot):
  `duplicate_existing_title`. Páginas fora das 100 recentes não são detectadas aqui;
- kind incompatível com o caminho: `kind_path_mismatch`. Correspondência: `gotcha` com `gotchas/`;
  `decision` com `decisions/`; `concept` com `concepts/`; `procedure` com `procedures/`; `rule` com
  `_rules/`; `slot` com `_slots/`; `fact` ou `note` com `notes/` ou `concepts/`.

**Patch** (`edit_mode = patch`), nesta ordem:

1. caminho fora de `_rules/` e `procedures/`: `patch_unsupported_path`; página que não foi
   **renderizada no prompt** (corpo ou só âncoras): `patch_target_not_in_context`;
2. sem edições: `patch_missing_edits`; mais de `max_edits_per_proposal` (5): `patch_too_many_edits`;
3. soma dos bytes de `content` acima de `max_changed_chars_per_proposal` (12 000):
   `patch_changed_chars_too_large`; uma edição acima de `max_edit_content_chars` (4 000):
   `patch_edit_content_too_large`;
4. corpo-alvo com cabeçalhos duplicados (comparação normalizada: sem caixa, espaços colapsados):
   `duplicate_anchor`;
5. **materialização** (abaixo); erros dela são o motivo;
6. corpo materializado: `body_too_large` (mesmo limite), orçamento de página (mesmos motivos),
   `body_missing_h1`, mais de um H1: `patch_extra_h1`, cabeçalhos duplicados: `duplicate_anchor`,
   kind incompatível: `kind_path_mismatch`;
7. a proposta passa a carregar o **corpo materializado** como `body_markdown` e o SHA-256 (hex, 64
   caracteres) do corpo-base do alvo como `expected_base_body_sha256`.

Depois de cada proposta de patch aceita, soma-se o número de edições; se a soma da execução passaria de
`max_patch_edits_per_run` (8), a proposta recebe `patch_run_edit_budget_exceeded` e não conta (as
seguintes ainda podem passar).

**Materialização.** Seções: um cabeçalho é uma linha de 1 a 6 `#` seguidos de espaço, fora de cercas
de código; a **âncora** é a linha do cabeçalho (sem o fim de linha). Uma seção vai do início do
cabeçalho até o início do próximo cabeçalho de nível **igual ou menor** (ou o fim do corpo), de modo
que inclui as subseções. O hash da seção (`section_sha256`) é o SHA-256 hex dos bytes dessa faixa.
Cada edição é aplicada em ordem sobre o resultado da anterior, e as seções são recalculadas a cada
edição:

- `op = delete_section`: `unsupported_patch_op`;
- mais de um cabeçalho com a âncora (comparação normalizada): `duplicate_anchor`;
- âncora procurada por **igualdade exata** (sem normalizar caixa nem espaços) — sem correspondência:
  `patch_anchor_not_found`;
- âncora de nível 1: `patch_h1_anchor_unsupported`;
- `append`: insere no **fim da seção** (depois das subseções) `\n\n` + conteúdo aparado + (`\n\n` se há
  texto depois da seção, senão `\n`);
- `add_section`: o conteúdo deve começar (primeira linha não vazia) com um cabeçalho — senão
  `add_section_missing_heading`; de nível 1: `add_section_h1_unsupported`; nível diferente do da âncora:
  `add_section_heading_level_mismatch`; insere no fim da seção da âncora, como no `append`, ou seja,
  uma seção **irmã** logo depois da âncora e de suas subseções;
- `replace_section`: exige `section_sha256` (`replace_section_missing_hash`) igual ao hash da seção
  atual (`replace_section_hash_mismatch`); a primeira linha não vazia do conteúdo deve ser igual à
  âncora (`replace_section_missing_heading` se não houver; `replace_section_anchor_mismatch` se
  diferir); substitui a seção inteira por conteúdo aparado + (`\n\n` se há texto depois, senão `\n`);
- qualquer outra `op`: `unsupported_patch_op`.

Resumo do modelo vazio: aviso `LLM returned an empty review summary`, sem rejeitar nada.

### 5.3 Portão de avaliação (opcional, desligado por padrão)

Contrato completo em [auto-improve-eval-gates.md](auto-improve-eval-gates.md). Resumo: configuração
(ligado; comando; timeout 120 s; alvos padrão `_rules` e `procedures` — prefixo de diretório:
caminho igual ao alvo ou começando por `alvo/`; `min_delta` 0,0); comando executado sem shell, linha
dividida por espaços em branco; entrada JSON no stdin; saída até 64 KiB; aceite = `passed` e (sem
os dois scores, ou `score_after − score_before ≥ min_delta`); resultados `success` / `rejected` /
`failure` / `timeout`; evidência `auto_improve_eval` na proposta aceita; rejeições `eval_gate_failed`,
`eval_gate_timeout`, `eval_gate_error`; ligado sem comando = aviso e rejeição das propostas-alvo;
registro por avaliação em `config_json.eval_results`. O portão roda apenas nas passadas de revisão e
de experiência, nunca no caminho de hooks.

### 5.4 Agendador (tick)

Só inicia com `[auto_improve.scheduler] enabled` (padrão verdadeiro), `interval_secs > 0` (padrão
3600), `max_sessions_per_tick > 0` (padrão 1) e provedor configurado. No início, grava a marca d'água
de todos os escopos conhecidos. O laço **dorme o intervalo primeiro** (o primeiro ciclo ocorre
depois de um intervalo) e dorme de novo depois de cada ciclo completo (ciclos nunca se sobrepõem).

Para cada escopo (workspace e projeto), em cada ciclo:

1. Garante o estado do agendador. Na primeira vez que o escopo é visto, a marca d'água é o maior
   `ended_at` das sessões já encerradas (0 se nenhuma) e `initialized_at` é agora. Sessões históricas
   não são processadas. (Consequência: um projeto criado **depois** do início do servidor e cuja
   primeira sessão já terminou antes de o agendador vê-lo pela primeira vez tem essa sessão
   absorvida na marca d'água e nunca revisada automaticamente.) Falha aqui conta em `errors` e pula
   o escopo.
2. Lista as sessões candidatas (consulta única): encerradas, com `ended_at` **maior que** a marca
   d'água e **menor ou igual a** `agora − min_session_age_secs` (600), sem execução de
   auto-melhoria registrada para a sessão e sem reivindicação "viva" (reivindicação em andamento ou
   estacionada; uma reivindicação de falha com tentativas < 3 não bloqueia), em ordem de `ended_at`
   crescente (depois `started_at`), até `max_sessions_per_tick`.
3. Passada de experiência (§5.6), se configurada e com sessões novas suficientes — **antes** das
   sessões individuais e independente de haver candidatas.
4. Para cada candidata: reivindica a sessão por inserção atômica (exige `ended_at` idêntico ao
   lido, acima da marca d'água e nenhuma execução registrada); se a inserção não ocorrer (ou o
   erro), pula. Caso contrário, roda a revisão (§5.1 a §5.3) e o staging (§5.5).
5. Falha da revisão ou do staging: registra a falha na reivindicação (`attempts + 1`, `last_error`
   = resumo redigido, `last_failed_at` = agora), de modo que a sessão volta a ser candidata no
   próximo tick. Quando `attempts` chega a 3 (`AUTO_IMPROVE_CLAIM_MAX_ATTEMPTS`), a sessão fica
   estacionada e é contada em `parked`. A falha ao gravar a falha é só registrada.
6. Erros por escopo ou por sessão são contados em `errors` e registrados em log com o resumo
   redigido `auto-improve failed: class=<classe> status=<status|none>` (classe fixa por tipo:
   `store`, `memory`, `session-not-found`, `session-out-of-scope`, `eval`, ou a do LLM; erro de
   causa desconhecida: `class=unrecognized`). Não abortam o tick.
7. Sucesso: a execução registrada impede nova candidatura (uma revisão manual ainda pode repetir).

Contadores do resultado: escopos, escopos com candidatas, revisadas, puladas (propostas não
gravadas), erros, estacionadas e execuções de experiência.

### 5.5 Staging e aplicação

- Cada proposta vira um registro com operação `update` se for patch, ou se o alvo é
  `_slots/current-focus.md` e já existe. Caso contrário, `create`. O hash-base é mantido. Falha ao
  converter o hash-base (não é hex de 64 caracteres) falha a execução inteira.
- O staging da execução (uma transação) sempre acontece antes de qualquer aplicação. Ele registra
  provedor, modelo, resumo, avisos, rejeições, configuração usada (limites; `eval`;
  `eval_results`; no agendador também `trigger` = `scheduler` ou `experience-scheduler` e
  `require_approval`), e o ator proponente (`proposal_actor`). Cada rejeição com `reason` não vazio
  entra também no buffer de rejeições (storage §16).
- Regras de transação: dois alvos iguais na mesma execução abortam **toda** a execução
  (`two proposals in the same run target <caminho>`, nada é gravado); `create` com alvo existente é
  pulada (`create proposal target already exists`); `update` com alvo inexistente é pulada
  (`update proposal target does not exist`); já existir pendência **do mesmo operador** para o alvo
  é pulada (`a proposal is already pending review for this path`); patch com operação diferente de
  `update`, sem edições, sem hash-base ou com hash-base diferente do alvo atual aborta tudo.
- Cada proposta gerada gera um arquivo revisável em `_pending/auto-improve/<id>.md` (formato no
  [loop de auto-melhoria](auto-improvement-loop.md)), fora do índice.
- Com aprovação obrigatória (`require_approval`), a proposta fica pendente.
- Sem aprovação obrigatória, a proposta é aprovada em seguida: o agendador usa o ator
  `auto_improve_scheduler_auto_approve`; as vias manuais (`/admin/auto-improve`, MCP, CLI) usam o
  ator da chamada com o rótulo de agente `auto_improve_auto_approve`. Todas passam pela admissão da
  operação de escrita de página e pelo mesmo caminho de aprovação (conflito se a base mudou; página
  fixada recusada). A aprovação manual por `pending-writes approve` usa a **mesma rotina**, com o
  ator autenticado de quem aprova (resolvido: a semântica é a mesma; só a atribuição muda).
- `skipped`: propostas que o armazenamento recusou, com caminho e motivo (acima). Cada uma é
  registrada individualmente.

### 5.6 Passada de experiência

Ver [experience.md](experience.md) (comportamento) e o documento de prompts, 12.7 (texto).

- Lê até `experience_sessions` (10) páginas-resumo de sessão (das `2 × experience_sessions` sessões
  mais recentes, só as encerradas e com página), cada uma até 6 000 **bytes**. O filtro de entropia
  (§9) é opcional e vem desligado.
- Propõe apenas o que é visível **entre** sessões: procedimento repetido, preferência reafirmada,
  fato redescoberto ou contradição.
- Usa o mesmo schema, validação, portão e staging da revisão. A resposta tem até 16 000 tokens.
- Cadência: só roda com pelo menos `experience_every_sessions` sessões concluídas depois da âncora
  `COALESCE(last_experience_run_at, initialized_at)`.
- Com menos páginas utilizáveis do que `experience_every_sessions` (também depois do filtro de
  entropia), o relatório sai com provedor `none`, resumo `experience pass skipped: <N> session
  page(s) found, <M> required`, e **não** avança a âncora. Execução concluída (mesmo sem propostas
  válidas) avança a âncora; erro não.

### 5.7 Telemetria

Formato completo no [loop de auto-melhoria](auto-improvement-loop.md) (seção "Relatório de
telemetria"). Resumo: janela padrão de 30 dias; top 10 por categoria (limite máximo 100 na rota);
taxas terminais sobre as propostas de **aprendizado** terminais (`approved`, `rejected`, `conflict`,
`failed`); pendentes e propostas de manutenção/relatório (tipos `curator_report` e
`auto_improve_report`) ficam fora do denominador; achados e pontos cegos fixos; saída em estrutura
de dados ou em markdown; a opção de staging cria uma página de relatório pendente.

---

## 6. Perfil entre projetos por LLM

`[profile] llm` (padrão ligado quando há provedor) liga a classificação e a mesclagem. Detalhes e
fallbacks em `design-cross-project-profile.md` §6 (verificado e coerente com este documento);
prompts literais no documento de prompts, 12.8. Resumo dos números: lotes de 20 sentenças e no máximo
8 chamadas de classificação por passe; no máximo 16 chamadas de mesclagem por passe; temperatura 0;
2 000 tokens de saída; sem retentativa; a falha de uma chamada cai para o caminho sem LLM só naquele
lote (classificação) ou naquela entrada (mesclagem, que mantém a declaração mais recente colhida),
com contagem de `llm_calls` e `llm_fallbacks` no relatório; confiança padrão do candidato 0,6 no
caminho com LLM e 0,5 sem. O trabalhador de perfil roda 120 s depois da partida, 30 s depois de um
fim de sessão (debounce) e a cada hora. O harvest ignora páginas curadas que são propostas de
auto-melhoria **aprovadas automaticamente pelo agendador** (ator
`auto_improve_scheduler_auto_approve`): saída de modelo que ninguém revisou não alimenta o perfil.

---

## 7. Caminhos, slugs, links e deduplicação

### 7.1 Sanitização de caminhos produzidos pelo modelo

- Caracteres ilegais em nomes de arquivo de sistemas comuns (`< > : " | ? *` e a barra invertida) e
  caracteres de controle ASCII (< 0x20) viram `-`, em cada componente separado por `/`. A estrutura
  de diretórios é preservada (não há outro tratamento: espaços, pontos e acentos permanecem).
- Se o último componente não termina em `.md` (sem diferença de caixa), acrescenta-se `.md`. A
  operação é idempotente.
- Um componente final vazio (caminho vazio ou terminado em `/`) não recebe extensão: o caminho
  continua inválido e é rejeitado/pulado depois.
- Usada pela consolidação em lote, pelo bootstrap e pela revisão de auto-melhoria.
- Motivo: um único caminho não portável aborta o lote inteiro na aplicação atômica. Por isso a
  sanitização roda antes e, ainda assim, o teste de portabilidade final pula só aquele item.

### 7.2 Slug de página de regra

Algoritmo exato do slug de `_rules/<slug>.md`:

1. Decomposição Unicode NFD e remoção das marcas combinantes (bloco U+0300 a U+036F).
2. Cada caractere é passado para minúsculas ASCII; se é `[a-z0-9]`, é mantido; qualquer outro vira
   hífen (sequências de não alfanuméricos viram um hífen só; hífens do início são ignorados).
3. Remove hífens finais.
4. Se o resultado ficou vazio (título sem letras ou dígitos ASCII, por exemplo cirílico, CJK ou só
   pontuação): se o título tem alguma letra ou dígito Unicode, o slug é `rule-` + os **8 primeiros
   caracteres hex** do SHA-256 da forma normalizada (decomposição NFD, divisão nos caracteres que não
   são letra/dígito/marca combinante, cada palavra em minúsculas Unicode, unidas por `-`) — assim
   regras com títulos diferentes não se sobrescrevem, e a mesma regra reescrita atualiza a mesma
   página; se não tem nenhuma letra ou dígito, `rule`.
5. Se o slug tem mais de 60 caracteres: procura o último `-` nos 61 primeiros caracteres; se ele
   está na posição 30 ou depois, corta ali; senão corta em 60; remove hífens finais.

### 7.3 Guarda de links gerados

Para toda página produzida por consolidação (única e lote) e por bootstrap, antes da gravação:

- Coletam-se os links de escopo cruzado do corpo (`[[projeto:caminho]]`, `[[ws/projeto:caminho]]`,
  `[[_global:caminho]]`) e todas as relações do frontmatter.
- Cada escopo nomeado é resolvido **sem criar** (workspace ausente, projeto ausente ou nome de projeto
  ambíguo contam como "não resolve"; falha real do armazenamento falha a operação). Os caminhos são
  verificados numa consulta por escopo (páginas atuais).
- Link de corpo que não resolve vira o **texto do rótulo sem colchetes**; o resto do corpo é preservado
  byte a byte. Links que resolvem são preservados.
- Relação do frontmatter que não resolve é removida; listas e o mapa `relations` que ficam vazios
  também. Relação de mesmo projeto cujo alvo não existe e não é uma página do mesmo lote também é
  removida. Links `[[caminho]]` do mesmo projeto no corpo **nunca** são tocados.
- O reescrito é feito só em memória; a gravação continua a atômica.

### 7.4 Agrupamento de páginas frias e deduplicação (A3)

Especificação completa (inclusive a compactação extrativa e a mineração de palavras-chave) em
[design-memory-aging.md](design-memory-aging.md) §6 e §7. Resumo e pontos corrigidos:

**Distância.** Cosseno = 1 − similaridade, limitado a [0, 2]. Vetor nulo ou de dimensões diferentes
resulta em 1,0.

**Limiar adaptativo (eps).** Para cada ponto, calcula-se a distância até seu k-ésimo vizinho
(k = `min_pts`, sem contar o próprio ponto). Os valores são ordenados. O cotovelo é o valor de maior
distância perpendicular à reta entre o primeiro e o último valor (empate: o primeiro índice); com 2
valores ou menos, é o último valor. O resultado é limitado a `[0, max_eps]`. Se `k = 0` ou n ≤ k, não
há eps.

**Agrupamento por densidade (DBSCAN).** Determinístico, na ordem dos índices; vizinhança é
`distância ≤ eps` incluindo o próprio ponto; ponto de borda entra no cluster mas não expande; ruído
não vira cluster.

**Padrões.** `min_pts` = 2 (valor 0 usa 2). `max_eps` = 0,15 (valor 0 ou inválido usa 0,15, que
equivale a similaridade de pelo menos 0,85).

**Aplicação** (somente na varredura agendada):

- Desligada por padrão. Exige ativação, a tripla de embedding (provedor, modelo e dimensão) usada nas
  páginas e a wiki. Sem embeddings dessa tripla ou sem wiki, é um no-op, sem erro.
- Elegíveis: páginas frias que não estão compactadas. Se houver menos de `min_pts` elegíveis ou com
  vetor, é um no-op.
- Cada cluster com 2 ou mais membros:
  - o sobrevivente é o de maior retenção; em **empate exato, o último** membro do cluster na ordem em
    que o agrupamento o alcançou (e não o primeiro);
  - o corpo do sobrevivente é reescrito pela compactação sobre o corpo do sobrevivente seguido, após
    uma linha em branco, do corpo de cada outro membro (o do sobrevivente conta uma vez). A mineração
    de palavras-chave roda sobre esse texto unido, portanto o sobrevivente ganha a **união** das
    palavras-chave, **sujeita ao teto de 48 tokens** da mineração;
  - cada membro vira um stub de fusão, que aponta para o sobrevivente com `[[...]]`, marcado com
    `compacted: true` e `merged_into`; o corpo original fica na cadeia de supersessão e no histórico;
  - o sobrevivente recebe, para cada membro, evidência de reconsolidação `a3_merge:<id do membro>`.
- Tudo é aplicado em um lote atômico por varredura. Nada é apagado. Se o lote falha, os clusters
  ficam para a próxima varredura.

---

## 8. Passada de sonho (B2 a B4), desligada por padrão

Especificação completa, com o prompt literal, o agendador e o relatório, em
[design-memory-aging.md](design-memory-aging.md) §10. Resumo:

**Gate:** ligado **e** com provedor **e** com embedder (e tripla de embedding). Sem qualquer um, o
relatório sai sem trabalho (`disabled`), com provedor `none`, e o agendador nem inicia.

**Configuração (padrões):** `min_pts` 2; `max_eps` 0,15; `max_clusters_per_run` 8;
`min_cold_pages` 2; `idle_window_secs` 300; `interval_secs` 3600.

**Fluxo:** conjunto frio da varredura (sem expiradas por TTL); clusters como em §7.4 (sobrevivente
= maior retenção, empate = o último, como na A3); ordem de novidade por *surprisal* (maior, entre os
membros, da menor distância até as páginas existentes — não frias e frias já compactadas; 1,0 se não
há existentes), mais novos primeiro, estável, truncado em `max_clusters_per_run`; `dry_run` devolve o
plano sem LLM; por cluster: preflight de admissão `consolidate` no caminho do sobrevivente, leitura de
todos os membros (falha pula), uma chamada estruturada (4000 tokens, temperatura 0,1, cada corpo até
6000 bytes), aplicação atômica (sobrevivente reescrito com `merged_from` e título trocado se o do
modelo não for vazio; perdedores viram stub `compacted` com `merged_into`; evidência
`reconsolidation` `b2_dream:<id do perdedor>`). Falha de admissão, de leitura, do LLM ou do lote pula
só aquele cluster, que é retentado na próxima execução.

Nunca apaga fonte. O sobrevivente do dream **não** é marcado `compacted`.

**Gatilho ocioso (B3):** o laço dorme `interval_secs` e, ao acordar, só começa se a atividade está
parada há `idle_window_secs`. "Atividade" é o relógio estampado em **toda chamada de ferramenta MCP**
(hooks e rotas administrativas não contam). Durante a execução, um observador amostra o relógio a
cada 2 s e aciona o cancelamento na primeira vez que ele avança; o cancelamento é checado antes de
cada escopo e antes de cada cluster. O relatório é sempre devolvido, inclusive quando cancelado.

---

## 9. Filtro de entropia (A4)

A classificação de um texto (dada a configuração) decide manter ou pular, com motivo. É uma função pura. Desligado por
padrão, sempre decide manter. Ordem das verificações, parâmetros e validação: ver
[design-memory-aging.md](design-memory-aging.md) §8 (padrões `min_chars` 16,
`min_entropy_bits_per_char` 2,0, `max_repetition_ratio` 0,7, `repetition_min_tokens` 6). Pontos
precisos: os caracteres em branco entram na distribuição da entropia; os tokens da repetição são
separados por qualquer espaço em branco Unicode e comparados em minúsculas ASCII.

`Skip` é consultivo e nunca apaga. Na passada de experiência, a página pulada sai do prompt, mas não é
consolidada nem apagada. A validação (entropia finita e ≥ 0; razão entre 0 e 1) roda na carga da
configuração **mesmo com o filtro desligado**. O exemplo de nota curta e informativa (uma linha com
caminho e código de erro) passa pelos limites padrão: tem mais de 16 caracteres não brancos e
entropia acima de 2,0 bits.

---

## 10. Contradição

**Por similaridade (sem LLM, consultiva):** especificação em
[design-memory-aging.md](design-memory-aging.md) §9. Roda somente no lint pedido pelo usuário
(`memory_lint`, `POST /admin/lint`, CLI), nunca no lint agendado.

**Por LLM (opt-in):**

- Roda no lint pedido pelo usuário, com provedor e sem `no_llm` (parâmetro `no_llm` do MCP e de
  `/admin/lint`; padrão falso). O lint agendado sempre usa só regras.
- Candidatas: páginas das camadas semântica e procedural, versão atual, não expiradas por TTL. Menos
  de 2: nada. As 20 de maior contagem de acesso (ordenação estável). Prévia = primeiros 400
  caracteres do corpo lido da wiki; leitura que falha vira `(unable to read)`.
- Prompt e mensagem: documento de prompts, 12.5. Achados `contradiction`, `stale` ou `duplicate`
  (o código não confere os valores de `kind` e `severity`; severidade ausente = `warning`; aceita
  `summary` como alias de `message`), lista vazia é válida.
- Falha da chamada vira aviso em log e não aborta o lint (os achados por regra continuam).
- Os achados do lint (todas as fontes) são gravados em `_lint/report.md` (uma página por projeto,
  substituída a cada execução; sem achados, a página é removida; páginas `_lint/AAAA-MM-DD.md` do
  formato antigo são apagadas). `dry_run` não grava nada.

---

## 11. Bootstrap

Semeia um wiki vazio a partir do histórico existente. É um cliente fino: o **cliente** coleta as
fontes localmente; o **servidor** poda, divide em chunks, chama o modelo e grava.

**Fontes (coletadas pelo cliente):**

- *Commits* (`git_commit`): histórico do HEAD, do mais novo ao mais antigo. O filtro `since` aceita
  `N days ago`, `N months ago` (30 dias cada), `N year ago`, `N years ago` (365 dias cada) ou
  `AAAA-MM-DD` (meia-noite UTC); **qualquer outro texto é ignorado em silêncio** (sem filtro). O
  percurso para no primeiro commit mais antigo que o limite. Commits triviais são pulados: entra o
  commit cujo resumo + corpo tenham 120 ou mais **bytes**, ou cujo resumo comece por `feat:`,
  `feat(`, `fix:`, `fix(`, `refactor:`, `refactor(`, `perf:`, `perf(`, `design:` ou `design(`. Rótulo
  `git: <resumo>`; texto `Commit <8 primeiros hex>\nAuthor: <nome>\nDate: <instante UTC>\n\n<resumo>\n\n<corpo>`
  (resumo ausente: `(no summary)`; autor ausente: `unknown`). Se a biblioteca de git não consegue
  abrir o repositório, **no Windows** o cliente recorre ao executável `git log` com o mesmo formato
  de campos (e filtro); em outros sistemas o erro é fatal.
- *README* (`readme`): o primeiro existente entre `README.md`, `README`, `Readme.md`, `readme.md`;
  rótulo `README (<nome do arquivo>)`.
- *Documentos* (`doc_file`): arquivos `.md` (extensão em minúsculas) de `docs/`, recursivo, sem
  seguir links simbólicos; rótulo `doc: <caminho relativo à raiz do repositório>`.
- *Cabeçalhos de módulo* (`module_header`): hoje só de arquivos do código do projeto da linguagem
  original (as linhas de documentação de módulo no topo de cada arquivo-fonte; ignora os diretórios
  `target`, `node_modules`, `.git` e `vendor`; sem seguir links simbólicos; para na primeira linha
  que não é documentação, tolerando linhas em branco só antes da primeira). Rótulo
  `rust: <caminho>`. Comportamento do original, reproduzir tal como está: a coleta examina só os arquivos Rust (`.rs`)
  do repositório analisado e só as linhas de documentação de módulo (`//!`) do topo; arquivos de outras linguagens não
  geram `module_header`, qualquer que seja a linguagem da reimplementação.
- *Regras do projeto* (`project_rules`): sempre coletadas. Entre `CLAUDE.md`, `AGENTS.md`, `AGENT.md`
  e `claude.md`, só os nomes que o diretório **lista de fato** (um sistema de arquivos que ignora
  caixa não pode ler o mesmo arquivo duas vezes), sem repetir texto idêntico. Rótulo `rules: <nome>`.

**Estimativa de tokens de uma fonte:** `ceil((bytes do rótulo + bytes do texto + 16) ÷ 4)`.

**Descarte acima do orçamento.** Orçamento utilizável = `floor(80% × max_input_tokens) − 1000`
(saturando em 0). As fontes são ordenadas pela prioridade de descarte decrescente (estável) e
descartadas, na ordem, até a soma caber:

| Fonte | Prioridade de descarte |
|---|---|
| Cabeçalhos de módulo | 4 (descartada primeiro) |
| Commits | 3 |
| Documentos em `docs/` | 2 |
| README | 1 |
| Regras do projeto | 0 (descartada por último; só vão embora se nem elas couberem) |

O descarte pode zerar as fontes: então o resultado é "sem fontes" (422). A estimativa é a soma das
fontes **antes** do descarte quando o orçamento já cobre tudo, e a soma das mantidas após o descarte
(o valor devolvido é o total restante).

**Orçamento do cliente.** O cliente poda antes de enviar (para não estourar o limite de corpo da rota
administrativa, 32 MiB) e informa `sources_collected` (o número original). Padrão do cliente:
`--max-input-tokens` 150 000; da rota: 50 000.

**Chunks.** `chunk_input_tokens` padrão 24 000 (o cliente e a rota); o limite efetivo é `min(chunk,
max_input_tokens)`; `0` desativa o chunking (uma chamada, 64 000 tokens de saída). Orçamento de cada
chunk: `floor(80% × chunk) − 1000` (se o chunk for ≤ 1000, sem a subtração). Se a soma de tudo cabe
num chunk, há uma chamada; senão as fontes são empacotadas de forma gulosa, **começando pelas de maior
sinal** (regras e README primeiro, depois docs, commits e cabeçalhos), fechando o chunk quando a próxima
fonte não cabe. Uma fonte maior que o chunk é dividida em partes de `chunk × 4 − (bytes do rótulo + 64)`
bytes (no mínimo 4), em fronteira de caractere, com rótulo `<rótulo> (part <n>)`. A chamada de cada chunk
recebe os caminhos já produzidos (§12.4 do documento de prompts) e retenta como em §3.4.

**Progresso durável.** A cada chunk concluído, grava-se (impressão digital, índice, páginas,
justificativa) em `bootstrap_chunk_progress`. A impressão digital é o SHA-256 hex de: id do workspace,
`0x00`, id do projeto, `0x00`, o orçamento do chunk (inteiro de 64 bits little-endian), e, para cada
chunk, `|chunk|` seguido de cada fonte (rótulo, `0x00`, texto, `0x00`). A retomada (`resume`) lê as
linhas da impressão digital atual e adota **somente o prefixo contíguo** (0, 1, 2, …) de chunks
concluídos, parando na primeira lacuna ou linha ilegível; reexecuta dali, porque cada chunk depende
das páginas dos anteriores (sobrescrevendo as linhas posteriores obsoletas). Mudou qualquer fonte ou o
orçamento: outra impressão digital, recomeça do zero. Sucesso apaga as linhas da impressão digital
(falha ao apagar é só aviso). Falha de um chunk aborta a execução (HTTP 502 se for erro de LLM), mantendo
as linhas dos chunks anteriores para um `--resume`.

**Mesclagem das páginas.** Por caminho, **a primeira página vence**; a repetida de chunk posterior é
descartada com aviso. Os caminhos passam pela sanitização (§7.1), a validação de caminho e o teste de
portabilidade (falha pula só a página).

**Páginas geradas:** tier `semantic`, não fixadas, ator anônimo, **sem passar pela cadeia de
admissão**, frontmatter `{title, tier: "semantic", tags, bootstrapped_at: <instante RFC 3339>}`; guarda de
links (§7.3); lote atômico; commit `bootstrap: <N> LLM chunk(s), ingested <coletadas> sources, wrote <P+1>
pages`.

**Manifesto e idempotência.** O manifesto `bootstrap.md` (tier `semantic`, **fixado**, frontmatter
`{title: "Bootstrap manifest", tier, bootstrapped_at, tags: ["bootstrap","manifest"]}`) é gravado no mesmo
lote, com o corpo:

```text
# Bootstrap manifest

> Generated by `ai-memory bootstrap` at <instante>.

## Sources

- Collected: **<coletadas>**
- Sent to LLM: **<enviadas>**
- Dropped to fit budget: **<descartadas>**
- Estimated input tokens: **<tokens>**
- LLM chunks: **<chunks>**

## Rationale

<justificativa>

## Pages produced

- `<caminho>`
...

---

_Re-running `ai-memory bootstrap` requires `--force` while this page exists. Delete this page (and the generated ones below) to reset._
```

A justificativa é: com um chunk, a do chunk (ou `_No chunk returned a rationale._` se vazia); com vários,
`Processed in <N> LLM chunks.\n\n` seguido das justificativas **não vazias** unidas por
`\n\n---\n\n` (ou `_No chunk returned a rationale._`). Sem `force`, se `bootstrap.md` já existe a execução é
recusada (422). Execução com `force` regrava o manifesto. O `dry_run` **não** verifica o manifesto.

**Nome do projeto** (resolução usada pelo cliente e pelo roteador de hooks):

- Por basename: último componente do caminho, tolerando `/` e `\` (um agente no Windows envia caminho
  Windows a um servidor em outro sistema).
- Pela raiz do repositório principal: raiz do repositório principal, resolvendo worktrees pelo diretório comum do git. Sem
  repositório (ou repositório "bare"), usa o basename.

**Rota `POST /admin/bootstrap`:** corpo `{workspace, project, sources[], sources_collected?,
max_input_tokens, chunk_input_tokens, dry_run, force, resume}`. `dry_run` responde sem criar projeto, sem
provedor e sem lock. Execução real: sem provedor 503 `LLM provider not configured on server`; cria o
workspace e o projeto se faltarem; **serializa** execuções reais (uma por vez, as outras esperam o lock);
nenhuma fonte coletada ou bootstrap já existente → 422; erro de LLM → 502; outros → 500. Resposta (sucesso e
`dry_run`): `{sources_collected, sources_sent, sources_dropped, sources_by_kind:{git_commits, readme,
doc_files, module_headers, project_rules}, estimated_input_tokens, pages_written[], rationale, dry_run,
llm_chunks, manifest_warning?}` (`manifest_warning` só quando houve aviso de reparo do manifesto de
escopo). `dry_run`: `rationale` = `(dry-run; LLM not invoked)`; o `--dry-run` do cliente nem chama o
servidor: calcula localmente (rationale `(dry-run; LLM not invoked, no network round-trip)`), exige ao
menos uma fonte e imprime o relatório humano e, depois de `\n--- machine-readable ---\n`, o JSON
indentado do mesmo objeto.

**Cliente `ai-memory bootstrap`:** flags `--repo-path`, `--workspace`, `--project`,
`--max-input-tokens`, `--chunk-input-tokens`, `--exclude-git/readme/docs/code`, `--since`, `--dry-run`,
`--force`, `--resume` (padrões em `install.md`). Sem `.git` acima do diretório atual, usa o diretório
atual e desliga os commits (aviso em stderr `note: no .git found at <dir>; bootstrapping from
README/docs/rules only (git-commit history skipped). Pass --repo-path or `git init` to include
commits.`). Com `--repo-path` informado, git é considerado presente se existir `<repo-path>/.git`.

---

## 12. Backfill de embeddings

Processa páginas `is_latest` que não têm embedding para a tripla atual (provedor, modelo e
dimensão).

- Páginas sem corpo (após remover espaços) são puladas (contam em `skipped`) e registradas em `page_embed_failures` como `skipped_empty`; página ilegível conta em `failed` e é registrada como `unreadable`; falha do embedder conta em `failed` e é registrada como `failed` (só a última falha fica registrada por página).
- As gravações são feitas em lotes de 100 (corpo e resumo em lotes separados); falha na gravação de um lote soma o tamanho do lote em `failed`.
- Uma página só é lida do disco se faltar o vetor do corpo ou o do resumo para a tripla atual; os demais contam em `skipped`. Páginas com `abstract` não vazio no frontmatter recebem também o vetor do resumo (falha dele conta em `failed`, sem registro em `page_embed_failures`).
- Em `dry_run`, nada é lido nem gravado; `would_embed` conta só as páginas com corpo a vetorizar.
- Com `reembed`, `POST /admin/embed` primeiro apaga as linhas de `page_embeddings` do escopo (projeto ou workspace inteiro) cuja tripla difere da atual e as ligadas a versões não mais recentes (a tabela de resumos não é apagada), e depois refaz tudo.
- Também gera embedding do resumo L0 (`abstract`), quando ele existe, em armazenamento separado.
- Opções: `reembed` refaz tudo; `dry_run` conta `would_embed` sem gravar.
- Falha em uma página é contada em `failed` e não aborta a execução.
- Mudança de prefixo de documento conta como mudança de modelo, então as linhas antigas ficam
  obsoletas.
- Contadores: `embedded`, `skipped`, `failed`, `would_embed` e `abstracts_embedded` (os quatro primeiros e `provider`, `model`, `dim` formam a resposta HTTP; `abstracts_embedded` existe só no contador interno e **não** é devolvido por `POST /admin/embed`). A passada de inicialização e a periódica percorrem todos os escopos (workspace e projeto) e só registram em log `embedded`, `skipped`, `failed` e erros de escopo.

---

## Notas para reimplementação (originais)

1. **Ordem sugerida:** (a) página de sessão por regra, a partir dos eventos de ciclo de vida (ver
   [ingestão de eventos de ciclo de vida](lifecycle-event-ingestion.md)); (b) consolidação de página
   única com um provedor; (c) varredura sem compactação nem deduplicação; (d) lint por regra; (e)
   auto-melhoria com staging manual (aprovação obrigatória); (f) o restante: compactação,
   deduplicação A3, passada de sonho, experiência e portão de avaliação.
2. A **validação** das propostas é o que torna o ciclo seguro: confiança mínima, evidência
   obrigatória, prefixos permitidos, H1, orçamento de tamanho e patch ancorado por hash. Implemente-a
   antes de ligar a aprovação automática.
3. Faça sempre o preflight de admissão antes de gastar LLM, e ofereça um `dry_run` barato.
4. Toda reescrita é supersessão. As remoções possíveis são: TTL explícito; remoção definitiva de
   marcadores após o prazo de `hard_delete_after_days`; e poda de observações (opt-in, irreversível).
   Documente isso para o operador.
5. Trate a saída do LLM como entrada não confiável: sanitize caminhos, limite tamanhos, normalize
   aliases e enums, e nunca deixe um caminho do modelo abortar o lote inteiro. Pule o item
   problemático.
6. Mantenha desligados por padrão os experimentos (passada de sonho, compactação, deduplicação e
   filtro de entropia) até medir ganho de recall. Veja também
   [projeto de envelhecimento da memória](design-memory-aging.md).
7. **Windows (MVP):** os caminhos gerados pelo modelo são sanitizados para os caracteres ilegais do
   Windows e passam pelo teste de portabilidade (nomes reservados, pontos finais, comprimento); o
   portão de avaliação executa o comando sem shell (use um executável nativo — `.exe` — ou um
   wrapper; scripts `.sh`/`python3` dos exemplos não existem por padrão); a trava de arquivo da wiki e
   o `git` do bootstrap seguem `windows.md`; o botão do navegador de `/pending` mostra também os
   comandos em PowerShell (aspas simples dobradas).
