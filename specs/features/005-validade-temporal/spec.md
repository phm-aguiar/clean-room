# Spec: Validade temporal (as_of), arestas tipadas e sinais de ranking opcionais

## Metadados
- **ID:** 005
- **Status:** draft (revisada contra os docs em 2026-10-13; **auditada contra o comportamento do original e corrigida em 2026-10-19** — `temporal.md`, `typed-edges.md`, `storage-and-retrieval.md` §11.5 e `mcp-server.md` atualizados; `plan.md` e `tasks.md` precisam ser regenerados)
- **Autor:** phm-aguiar
- **Data:** 2026-10-13
- **Feature Anterior:** 002 (`memory_query`, `memory_write_page`, `memory_status`), 004 (aposentadoria por decaimento que fecha janelas)
- **Roadmap:** `specs/features/roadmap.md`
- **Aprovado:** false

## Propósito
> Para quem, o quê e por quê.

Responder "o que a memória considerava atual naquele instante?" para auditorias e análises pós-incidente, e deixar explícito no grafo de que tipo é uma relação entre páginas (`causes`, `fixes`, `contradicts`). A 001 já grava as janelas de ingestão de página e de vínculo; falta a consulta `as_of` com seus dois fluxos, e falta completar as arestas tipadas na escrita pública, na explicação da busca e no status.

## Escopo

### Dentro do escopo
- **`as_of` em `memory_query`** (`docs/temporal.md`, `docs/design-page-ingestion-windows.md` §6.2):
  - entrada ISO-8601 **com fuso** (`Z`, deslocamento ou zona); data sem hora/fuso é erro; texto vazio ou só com espaços equivale a ausência; valor inválido é erro (`as_of must be an ISO-8601 instant: …`); combinar com `scopes` é erro. **`global`:** o original avalia a busca global antes de `as_of`; com `global = true` sem escopo explícito a busca global responde e `as_of` é **ignorado sem erro**; com escopo explícito o erro vem da busca global. (Decisão de fidelidade: replicar. Rejeitar `as_of` + `global` é endurecimento aceitável, mas diverge do original e deve ser decisão explícita);
  - fluxo de entidades pelas janelas de vínculo (`valid_from ≤ T` e `superseded_at` nulo ou `> T`), sem filtro de expiração, com as regras de termo e de casamento do `temporal.md` (3 a 64 caracteres, até 12 termos, partes de compostos, igualdade, prefixo do nome ou prefixo de palavra do nome após espaço, `-` ou `_`, sem diferença de caixa, `%`/`_`/`\` literais), peso `1/N` com N = vínculos da entidade vigentes em T; ordem peso ↓, **número de entidades casadas** ↓, caminho ↑;
  - fluxo de texto pelas janelas de página (`valid_from ≤ T` e `valid_to` nulo ou `> T`), com a expiração avaliada em T (BM25 do índice atual, consulta normalizada como na busca padrão);
  - até `max(4 × limite, 20)` candidatos por fluxo (limitado a `limite + 300`; limite padrão da ferramenta, 1..100), fusão RRF com k = 60 (posições a partir de 1), rank = −pontuação multiplicado pelo ajuste de autoridade da busca padrão (`storage-and-retrieval.md` §11.5: kind, tier, fixada, caminho, tags; `[0,55; 1,50]`, tetos 0,55/0,65/0,80), autoridade lida da própria versão para páginas que só vêm do fluxo de entidades; desempate por caminho ↑;
  - sem vetor, grafo, observações brutas, `_global`, roteamento de recall, peso de crença nem reforço de acesso; `answer` pedido traz `answer_unavailable = "answer synthesis is not supported for as_of (time-travel) queries"`;
  - `explain` com `fts_rank`, `fts_score`, `entity_rank`, `entity_weight`, `matched_entities` (nomes de entidades), `rrf`, `fused`, `authority` e `streams_active = ["entity", "fts"]` só quando `explain` é verdadeiro; sem `timings`, `evidence_count` nem `confidence`.
- **Arestas tipadas** (`docs/typed-edges.md`):
  - campo `relations` nos metadados da escrita pública (`memory_write_page` e a escrita administrativa), com os limites (até 32 alvos no total, 1024 caracteres por alvo, sem caractere de controle, qualificador dividido no primeiro `:` com no máximo uma `/` e componentes não vazios, sem espaço nas bordas e diferentes de `.`/`..`, último segmento sem extensão ganha `.md`, qualquer outra extensão é erro, alvo portável) e rejeição da escrita inteira (`invalid or oversized page metadata: relations`); chave de relação desconhecida ou valor que não é lista de textos falha já na leitura do pedido; `relations` é substituição integral dos metadados, não remendo;
  - no frontmatter de arquivo, chave de relação desconhecida e alvo inválido são **ignorados com aviso**, sem limite de quantidade;
  - em toda nova versão de um caminho (supersessão, ressurreição ou cadeia nova), na mesma transação, todas as linhas de link cujo alvo casa com o caminho são reapontadas para ela, inclusive as já resolvidas, de qualquer versão de origem (`docs/typed-edges.md` § Storage; regras para alvo local, `projeto:caminho`, `workspace/projeto:caminho` e `_global`); a 001 só faz isso na supersessão;
  - campo `edge` dentro de `graph_via` no `explain` da busca para saltos de grafo cujo rótulo não é `references`;
  - contagem por relação dos links tipados vindos de versões atuais no **status administrativo** (`GET /admin/status`, `derived.typed_links_from_latest_pages`, `[relação, contagem]` ordenado por relação, global, sem filtro de expiração) e no CLI `status`; **não** vai no `memory_status` do MCP. O `memory_status` do MCP traz `counts` por projeto, incluindo `evidence_rows`.

- **Sinais de ranking opcionais** (`docs/storage-and-retrieval.md` §11.5 e §13, `docs/architecture.md` § `[retrieval]`, `docs/mcp-server.md` §3.2, `docs/design-hindsight-borrowings.md` P2 e P4; nenhum depende de LLM e todos ficam desligados por padrão):
  - **confiança derivada na leitura**, nunca armazenada: amplitude por sessões distintas (`session` e `reconsolidation`) mais crédito de 0,2 por evidência residual com teto 1,0; suporte `1 − e^(−amplitude/3)`; recência `0,5 + 0,5·e^(−idade/30 dias)` (1,0 sem instante, idade negativa vira zero); penalidade `1/(1 + contradições vivas)`, contando só arestas `contradicts` declaradas pela versão cujo alvo é uma página **atual e não expirada**; amplitude = sessões distintas (`session`, `reconsolidation`) + `min(1,0; 0,2 × (total de registros − sessões distintas))`; teto 0,95; sem evidência, 0; calculada apenas com `explain` ou peso positivo;
  - **peso de crença** `[retrieval].belief_authority_weight` (padrão 0,0): soma `peso × confiança` ao fator de autoridade dentro de `[0,55; 1,50]`; com peso 0 e sem `explain`, nenhuma consulta de evidência no caminho de busca; versões superseded nunca recebem o bônus;
  - **roteamento de recall de sessão** `[retrieval].query_intent` (desligado) e `session_recall_bonus` (0,25): consulta com marcador léxico de recall de sessão devolve às páginas `session` a penalidade de tipo e de tier e soma o bônus; intenção `session_recall` no `explain`. A lista completa de marcadores **não é decisão nossa**: é a do original, agora em `storage-and-retrieval.md` §11.5 (21 chinesas por substring, 17 inglesas e 9 portuguesas por palavra inteira, com a normalização descrita); a penalidade devolvida é 0,15 de tipo mais a de tier negativa (0,08 `episodic`, 0,03 `working`);
  - `explain` com `evidence_count`, `confidence`, `belief_factor` (quando o peso está ativo e a versão não é superseded), `intent = "session_recall"` e `intent_boost`; a confiança é calculada no `explain` mesmo com o peso desligado;
  - **`settled_first`** no `memory_briefing` (padrão falso): lista `settled` (omitida se falso ou vazia) com até 8 páginas atuais de tipo `rule` ou `decision` (fixadas ou não), não expiradas, `{path, title, kind, evidence_count}`, por contagem de evidência ↓ e depois `updated_at` ↓;
  - `evidence_rows` (total de registros de evidência do projeto) no status.

### Fora do escopo (explicitamente)
- Fase B, tempo do mundo (`true_from`, `true_until`): adiada pelo próprio doc.
- Mudança de ranking no caminho padrão por frescor ou por `contradicts`, peso por tipo de aresta e travessia de cadeias `fixes`/`causes` (propostas nos docs, não implementadas).
- Preenchimento de janelas e de vínculos de dados antigos na abertura do armazenamento: o esquema V01 já nasce com as colunas e toda escrita as preenche; não há loja antiga a migrar (decisão nossa).
- Lint de `contradicts` e de `broken_link` (feature 004).

## Comportamento Esperado

### Cenário Principal (Happy Path)
1. Em junho, `notes/db.md` diz "usamos postgres" (entidade `postgres`). Em agosto, a página é substituída por "migramos para sqlite" (entidade `sqlite`).
2. `memory_query {"query": "postgres"}` não traz a página, porque o conhecimento atual mudou.
3. `memory_query {"query": "postgres", "as_of": "2026-07-01T00:00:00Z"}` traz a versão de junho, pelo fluxo de entidades.
4. Uma pergunta sem o termo exato ("qual banco usávamos?") com o mesmo `as_of` encontra a versão de junho pelo fluxo de texto.

### Cenários Alternativos
- **`as_of` inválido:** erro.
- **`as_of` com `scopes`:** erro. **Com `global`:** ver o Escopo (original ignora `as_of` quando `global` está sozinho).
- **`as_of` só com data (`2026-07-01`) ou sem fuso:** erro.
- **`as_of` vazio ou só espaços:** consulta normal.
- **`relations` na escrita pública com chave desconhecida, alvo inválido, mais de 32 alvos ou alvo acima de 1024 caracteres:** a escrita inteira é rejeitada com erro de validação e nada é salvo.
- **Chave de relação desconhecida no frontmatter do arquivo:** a chave é ignorada com aviso e a página é indexada.

## Edge Cases
- **Instante exato de supersessão:** vale a versão nova (intervalo meio-aberto).
- **Aposentadoria por decaimento:** fecha as duas janelas, e `as_of` não ressuscita a página depois do instante da retirada.
- **Exclusão explícita e expiração por TTL da varredura:** removem a linha do tempo (todas as versões); `as_of` não encontra a página em nenhum instante.
- **Reorganização (`reorg`, feature 004):** fecha a janela de página e de vínculo de todas as versões atuais do workspace `default` no mesmo instante, sem tombstone.
- **Ressurreição:** entre o `valid_to` do tombstone e a criação da nova versão, nenhuma versão daquele caminho vale.
- **Ranking em `as_of`:** usa as estatísticas do índice atual; a ordem pode mudar com escritas posteriores para o mesmo T. Os testes conferem o conjunto, não a ordem exata entre fluxos.
- **Mesmo alvo com rótulos diferentes:** são linhas separadas; o mesmo alvo com o mesmo rótulo numa página é gravado uma vez.

## Constraints
- **Performance:** cada fluxo limitado a `max(4 × limite, 20)` candidatos; consulta `as_of` sem efeito colateral de escrita.
- **Tecnologia:** sem dependência nova; SQL sobre as colunas de janela da V01.

## Conflito entre docs (registrado)
`docs/domain-vocabulary.md` §6.8 diz que nome de relação desconhecido "fica como referência simples"; `docs/wiki-files-layer.md` §3.5 e `docs/typed-edges.md` dizem que a chave desconhecida é ignorada com aviso, "para que um erro de digitação não crie um tipo de aresta novo". A 001 seguiu o primeiro. Esta feature segue o doc específico de arestas tipadas, que tem a concordância do doc da camada de arquivos.

## Critérios de Sucesso
- [ ] Os cenários de aceitação de `docs/design-page-ingestion-windows.md` §8 que se aplicam a uma loja nova passam como teste
- [ ] O exemplo de `docs/temporal.md` (postgres e sqlite) passa pelos dois fluxos
- [ ] `relations` na escrita pública respeita todos os limites e rejeita a escrita inteira em violação
- [ ] `explain` traz `edge` e `streams_active` como no doc; status traz a contagem por relação
- [ ] `scripts/check.sh` e o lint passam sem apontamentos

## Abordagem Escolhida
Um caminho de consulta `as_of` separado dentro da busca do `storage`, com os dois fluxos filtrados pelas janelas e a mesma fusão RRF e o mesmo ajuste de autoridade do caminho padrão. Validação de `relations` na fronteira da escrita pública, reaproveitando a gramática de alvo da 001. A correção da relação desconhecida fica na derivação de links da wiki.

## Dependências
- 001: janelas de página e de vínculo, derivação de links e relações, autoridade.
- 002: `memory_query`, `memory_write_page`, `memory_status`.
- 004: aposentadoria por decaimento (para o caso de borda de `as_of` depois da retirada).

## Checklist de Prontidão
- [x] Propósito claro e sem ambiguidade
- [x] Escopo delimitado (dentro/fora)
- [x] Cenários cobrem happy path e alternativos
- [x] Edge cases identificados com comportamento esperado
- [x] Constraints explícitas
- [x] Critérios de sucesso mensuráveis
- [x] Abordagem escolhida justificada
