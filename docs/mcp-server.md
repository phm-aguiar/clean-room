# Servidor MCP, API administrativa e autenticação HTTP

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

> **Papel:** a superfície de entrada de agentes e operadores. Reúne:
> 1. **Servidor MCP** com as **23 ferramentas** de memória;
> 2. **Rotas administrativas** sob `/admin/*` (ciclo de vida, manutenção, usuários, perfil entre projetos);
> 3. **Autenticação HTTP**: token portador (administrador, usuário do banco ou proxy de identidade confiável)
>    e **autenticação humana** (senha, sessão web, proteção CSRF e recuperação).
>
> O componente não tem persistência própria. Toda leitura e escrita passa pelo resolvedor de escopo,
> pelo leitor e escritor do armazenamento, pela wiki e pelo consolidador.

Componentes envolvidos: núcleo de domínio, armazenamento, wiki, cliente de LLM, consolidação e
transporte MCP. Para o vocabulário, ver [domain-vocabulary.md](domain-vocabulary.md). Para a
tabela completa de propósito e flags das ferramentas, ver [architecture.md](architecture.md).

---

## 1. Princípios

- **Versão do protocolo MCP fixa em 2024-11-05.** Isso evita negociar para baixo até uma versão
  em que o cliente descarta ferramentas. O servidor anuncia o nome `ai-memory`, a capacidade
  `tools` e as instruções de uso da memória.
- **Identidade vem da camada de autenticação, nunca de cabeçalhos crus.** Os handlers leem o
  ator, o nível de autorização e o usuário do contexto da requisição. Não use variáveis
  associadas à tarefa de execução: a biblioteca de MCP executa cada ferramenta em tarefa
  separada, que não herda essas variáveis.
- **Escopo sempre pelo resolvedor de escopo.** Leitura não cria nada e falha fechado. Escrita
  pode criar workspace e projeto nomeados.
- **Toda escrita de página passa pelo caminho único da wiki** (sanitização, admissão,
  atribuição, rollback e índice).
- **Toda chamada de ferramenta passa por um ponto único** que registra a atividade do cliente
  e alimenta o relógio de atividade usado pelo ciclo de sonho (dream pass).

---

## 2. Construção do servidor

O servidor é criado com leitor, escritor, workspace padrão e projeto padrão. O limite padrão de
resultados é 10 e não muda por construtor. Os demais parâmetros entram por encadeamento de construtores. Nenhum deles lê
variáveis de ambiente; quem monta o servidor é responsável pela configuração.

Construtores disponíveis:

- wiki, embedder, reranker, consolidador, sanitizador e projeto ativo;
- parâmetros de decaimento e de peso de amplitude (breadth) do decaimento;
- compactação de episódios frios e retenção de observações;
- revisão de auto-melhoria (aprovação obrigatória e configuração de revisão);
- slots por usuário, identidade de proxy confiável, remoção de combinadores na raiz do schema e
  schemas seguros para Gemini;
- limiar de contradição e configuração de perfil entre projetos.

O relógio de atividade compartilhado é exposto para o ciclo de sonho.

### 2.1 Identidade e escopo nas ferramentas

- **Chave do ator.** Combina o usuário (chave de identidade usada no armazenamento) com o
  identificador de sessão. O identificador de sessão é obtido, nesta ordem, do contexto de
  transporte, do cabeçalho `x-memory-actor-session-id`, do identificador de sessão nativo e do
  cabeçalho `mcp-session-id`. O roteador de hooks precisa usar a mesma regra; caso contrário,
  leituras e escritas do mesmo operador caem em slots diferentes do projeto ativo.
- **Leitura:** retorna o escopo e a origem da escolha (`resolved_by`). Os valores no fio de `resolved_by` são:
  - `"explicit"` (`workspace`/`project` explícito);
  - `"active_project"` (ponteiro de projeto ativo com chave de ator correspondente);
  - `"active_project_shared"` (slot único compartilhado do processo);
  - `"active_project_fallback"` (semente de inicialização `startup_seed`);
  - `"default"` (escopo padrão porque o ponteiro estava `Unset`);
  - `"default_fallback"` (escopo padrão após `Mismatch` do ponteiro em leitura).

  Quando a origem é uma **substituição de fallback** (`active_project_fallback` ou `default_fallback`),
  o servidor registra um aviso. Isso acontece quando um cliente MCP estático responde por um
  projeto que nunca foi nomeado.
- **Escrita:** cria workspace e projeto nomeados se não existirem.
  Sem `project` explícito, consulta o ponteiro ativo:
  - se houver correspondência (`Matched`, `MatchedShared` ou `StartupSeed`), usa o escopo do ponteiro;
  - se o ponteiro estiver **`Unset`** (nenhuma entrada com chave foi publicada neste processo e o slot único está vazio, ex.: modo `stdio` ou servidor recém-iniciado), cai para o escopo padrão configurado (`default/scratch`);
  - se o ponteiro estiver em **`Mismatch`** (já houve publicação com chave no processo ou o usuário tem entrada somente-identidade, e a coordenada do chamador não casa), a escrita é recusada com erro JSON-RPC `-32602` (`ambiguous unscoped write`); nunca grava silenciosamente no padrão quando há ambiguidade.
  Com `project` mas sem `workspace`, usa o workspace **do ponteiro** (quando resolvido), e não o padrão do servidor, para não recriar `default/<projeto>` sem querer.
- **Protocolo de erros das ferramentas MCP:** falhas de validação, escopo ausente/ambíguo ou página/sessão/handoff não encontrado retornam **erro JSON-RPC** (`-32602` para parâmetros/escopo inválidos, `-32600` com `data: {reason: "precondition_failed", path, expected_page_id, current_page_id}` para conflito de versão esperada, e `-32603` para entidade não encontrada, permissão negada ou falha de armazenamento), e não resultado de ferramenta com `isError: true`.
- **Múltiplos escopos** (`scopes`: lista de pares `{workspace, project}`, no máximo **25**).
  Resolve cada par como existente, sem criar.
- **Aliases** nos parâmetros: `query` aceita também `q` e `search`; limites aceitam `n` ou `top_k`
  (em `memory_query`) e `n` (em `memory_recent`).

### 2.2 Compatibilidade do schema de ferramentas (`tools/list`)

Os schemas de entrada são gerados a partir dos tipos de argumentos. Para provedores de LLM mais
restritivos, existem quatro dialetos, ordenados por rigor:

| Dialeto | O que faz | Ativado por |
|---|---|---|
| Original | Schema gerado, sem alteração. | padrão |
| Sem combinadores na raiz | Remove `anyOf`, `oneOf` e `allOf` do nível raiz. | configuração `strip_root_combinators` ou `?flavor=bedrock` |
| Referências achatadas | Além do anterior, substitui toda referência `$ref` pelo conteúdo e remove a tabela `$defs` vazia. Combinadores aninhados permanecem. | `?flavor=moonshot` |
| Seguro para Gemini | Além do sem combinadores na raiz: converte `type: [T, "null"]` em `type: T` com `nullable: true`; achata combinadores com irmãos; mantém `$defs` e `$ref`. | configuração `gemini_safe_schemas` ou `?flavor=gemini` / `?flavor=vertex` |

O dialeto efetivo é o **maior** entre a configuração do operador e o marcador `?flavor=` da
requisição. O motivo do achatamento de referências no caso Moonshot é que o validador desse
provedor não resolve `$ref` em nenhuma profundidade e rejeita a requisição.

---

## 3. As 23 ferramentas

Os detalhes de propósito e de flags estão em [architecture.md](architecture.md). Aqui está o
comportamento.

### 3.1 `memory_query`

A ferramenta mais complexa. Parâmetros: `query`, `workspace` e `project`, `scopes`, `global`,
`as_of`, `limit` (limite padrão do servidor, limitado a **1..100**), `include_expired`,
`include_superseded`, `explain`, `pin_first`, `answer` e `reasoning`.

Fluxo:

1. **Consulta global** (`global=true`, ou o ponteiro do ator tem o padrão de recall global ativo e
   **nenhum** argumento de escopo foi passado). Argumento explícito sempre vence.
   - Proíbe combinar com `workspace`, `project` ou `scopes` (erro).
   - Usa um ranqueador **somente FTS**, sem fusão RRF, sem vetores e sem reranqueamento.
   - `answer` não é suportado; a resposta traz uma nota `answer_unavailable`.
2. **Viagem no tempo** (`as_of`, instante ISO-8601). Instante inválido gera erro.
   - Proíbe `global` e `scopes` (erro).
   - Usa a busca por entidade e FTS, filtrada pelas janelas de ingestão de página vigentes no
     instante informado.
   - Não usa vetores, grafo, observações brutas nem incremento de acesso.
3. **Fluxo normal:**
   - Gera o vetor da consulta. Se a geração falhar, registra um aviso e **degrada**: a busca segue
     só com FTS, entidade e grafo.
   - Quantidade de candidatos: sem reranqueador, é o próprio `limit`. Com reranqueador, é
     `max(limit, min(limit × 3, 30))`.
   - Com `scopes`, busca em cada escopo, mescla por identificador mantendo o **menor rank** de
     cada página, ordena por (rank, caminho, identificador) e trunca nos candidatos.
   - Sem `scopes`, busca em um único projeto (o resolvido).
4. **Reranqueamento** (somente se houver reranqueador configurado, com `AI_MEMORY_RERANKER=llm`):
   - Só roda com **2 ou mais** candidatos. Considera no máximo **30**.
   - Limite de **4** chamadas simultâneas. Se o limite estiver cheio, não espera: mantém a ordem
     anterior.
   - Tempo limite de **20 s**. Erro ou tempo esgotado mantém a ordem anterior.
   - A resposta precisa trazer exatamente uma pontuação por candidato, finita e no intervalo
     [0, 1], sem duplicata e sem identificador desconhecido. Se qualquer regra falhar, mantém a
     ordem anterior.
   - Ordena por pontuação decrescente. Empates seguem a posição original. Candidatos além de 30
     são anexados ao fim. Com `explain`, a pontuação aparece como `rerank_score`.
5. **Incremento de acesso** assíncrono (tarefa em segundo plano, sem bloquear a resposta). Cada
   par (página, operador) recebe no máximo **1 incremento a cada 60 s**. A janela é por operador:
   um operador não consome o incremento de outro. Falhas são registradas e não chegam ao cliente.
6. **Fallback de observações brutas:** se **nenhuma página compilada** casou, faz busca FTS nas
   observações do escopo resolvido (ou em cada escopo de `scopes`, mesclando por rank e truncando
   no limite). O resultado vai em `raw_hits`.
7. **Preferências globais:** em consulta de escopo único, a resposta também une o escopo reservado
   `_global` (em `global_scope_hits`). Não há duplicação se o projeto resolvido já for o próprio
   `_global`. A união também traz o **perfil entre projetos**:
   - se o projeto desativou o consumo do perfil (`[profile] consume = false`), as páginas de perfil
     são removidas dos resultados globais;
   - se o perfil está no escopo de workspace ou de usuário, faz uma busca adicional nesse escopo.
     Falha ao resolver o perfil não falha a consulta: apenas não traz essas páginas.
8. **`pin_first`** (somente em consulta de projeto único, sem `scopes`): antepõe até **10** páginas
   fixadas do projeto (limitadas ao `limit`). Remove duplicatas por identificador e trunca
   novamente no `limit`.
9. **`answer=true`** (opt-in; desligado por padrão). Só com esse argumento o LLM é acionado.
   - Pede ao LLM uma resposta estruturada (`answer` e `citations`) sobre os resultados do projeto e
     os de `_global`, com instrução de usar **somente** o conteúdo recuperado.
   - Sem provedor configurado: retorna os resultados com a nota `answer_unavailable` (nunca erro).
   - Falha do provedor: retorna os resultados com nota de falha contendo a classe do erro e o
     status HTTP (ou `none`).
   - `reasoning` (`minimal` padrão, `low`, `medium`, `high`, `max`) escala o orçamento de tokens da
     síntese sobre a base de **2000** tokens: `minimal` 1×, `low` 1,5×, `medium` 2×, `high` 3×,
     `max` 4×. Valor desconhecido é rejeitado. Não há campo de raciocínio por requisição fora desse.
10. **Resposta:** `hits`, `answer` (opcional), `answer_unavailable`
    (opcional), `raw_hits` (opcional), `global_hits` (opcional), `global_scope_hits` (opcional) e `streams_active` (opcional).
    - `streams_active` só aparece com `explain`. Em consulta normal lista `fts`, `entity`, `vector`
      (somente se houve vetor), `abstract` (somente se houve vetor e a configuração de vetores de
      resumo está ativa) e `graph`.
    - Consulta global lista só `fts`. Viagem no tempo lista `entity` e `fts`.
    - `score_details` só aparece com `explain`.

### 3.2 Demais ferramentas de leitura

- **`memory_recent`:** páginas mais recentes por `updated_at` decrescente. Limite padrão do servidor
  (10), limitado a 1..100 (aceita alias `n`). Retorna `{ recent: [...] }`.
- **`memory_read_page`:** lê uma página por `path` ou por `query`.
  - Se os dois forem dados, `path` tem precedência. Se nenhum for dado, retorna erro `-32602`.
  - Com `query`, faz busca FTS e devolve o corpo completo do primeiro resultado.
  - Retorna `page_id`, `path`, `title`, `body` e `frontmatter` (mais `expired: true` quando a página está expirada). Quando o arquivo Markdown falta no disco e a página é
    servida do banco, inclui `served_from: "db-fallback"`.
  - Erro `-32603` se a página não existe no escopo resolvido.
  - `related` (booleano opt-in, padrão `false`) percorre o grafo de links em volta da página e devolve `related`.
    Com `related_depth = 1` (padrão), `related` tem `{ links, backlinks }`. Com `related_depth` em `2..3`, `related` tem `{ depth, nodes }` (onde cada nó traz `path`, `title`, `kind`, `workspace`, `project`, `depth` e `direction` `link` ou `backlink`, limitado a **50** nós, incluindo projetos cruzados). Sem `related: true`, o campo `related` é omitido.
  - Em caso de sucesso, reforça a página lida e os vizinhos percorridos (mesmo incremento de acesso
    descrito em 3.1, com a mesma janela por operador). Leitura com erro não reforça nada.
- **`memory_read_session_observations`:** observações brutas de uma sessão, com paginação.
  - `session_id` é obrigatório (UUID da sessão). Aceita filtros opcionais `kinds` (lista de tipos de observação) e `query` (busca FTS nas observações da sessão).
  - `offset` padrão **0**; `limit` padrão **50**, limitado a **1..200**.
  - `body_max_chars` padrão **4000**, limitado a **200..16 384**. Corpo acima do limite é cortado com
    marcador `\n… [truncated <N> chars]`.
  - `order`: `"asc"` (padrão) ou `"desc"`.
  - A resposta traz `{ session, observations, total, offset, limit, order, elided_other_scope, body_max_chars }`.
- **`memory_status`:** retorna `{ counts: { pages_latest, pages_all, sessions, observations, evidence_rows }, scope: { workspace, project, resolved_by } }` (ver 2.1).
- **`memory_briefing`:** snapshot estruturado com parâmetros `recent_pages_limit` (padrão 10, limitado a 1..100) e `settled_first` (padrão `false`).
  - Retorna `counts`, `activity_7d`, `activity_30d`, `last_observation_at`, `pending_handoff_count`, `pending_message_count`, `rules`, `slots`, `recent_pages`, `cross_project_dependents`, `cross_project_dependencies`, `scope`, mais `pinned` (até **10** páginas fixadas, omitido se vazio) e `settled` (somente com `settled_first=true`: até **8** páginas atuais e não expiradas cujo tipo derivado é `rule` ou `decision` — fixadas ou não —, cada uma com `path`, `title`, `kind` e `evidence_count`, ordenadas por contagem de registros de evidência decrescente e depois por `updated_at` decrescente).
- **`memory_explore`:** digest em prosa feito pelo LLM, com o prompt de sistema do digest (ver
  [prompts-schemas-and-sanitizer.md](prompts-schemas-and-sanitizer.md)). Orçamento base de
  **16 000** tokens, escalado por `reasoning` da mesma forma que em 3.1.
  - Sem provedor de LLM: responde com `prose: null`, um motivo e o snapshot do briefing em JSON.
  - Registra incremento de acesso nas páginas que expôs (regras, slots, recentes, fixadas e
    assentadas), resolvidas em lote.
- **`memory_handoff_list`** e **`memory_message_list`:** listagens somente leitura (ver 3.3).
- **`memory_install_self_routing`:** devolve o bloco marcado de roteamento, a lista de skills
  gerenciadas e dicas de destino por harness.

### 3.3 Ferramentas de escrita

- **`memory_write_page`:**
  - `tier` padrão: **`episodic`** quando omitido (`working`, `episodic`, `semantic` ou `procedural`).
  - Valida o caminho e exige que ele seja portável.
  - `scope` aceita:
    - `"global"`: escreve no escopo reservado `default/_global`. Não combina com `workspace` nem `project`;
    - `"profile"`: escreve no perfil entre projetos. Não combina com `workspace` nem `project`;
    - qualquer outro valor: erro `-32602` com os valores aceitos.
  - `session_id` e `scope` não podem ser usados juntos: uma sessão pertence a um projeto.
  - Aceita controle de concorrência otimista: `expected_page_id` (UUID da versão atual esperada) e `create_only` (booleano; falha se a página já existir). Em caso de divergência, retorna erro JSON-RPC `-32600` com `data: { reason: "precondition_failed", path, expected_page_id, current_page_id }`.
  - Frontmatter aceita `title`, `tags`, `pinned` (padrão `false`; páginas sob `_slots/` são sempre fixadas), `expires_at` (formato RFC 3339 ou `AAAA-MM-DD`) e metadados OKF (`kind`, `entities`, `abstract`, `relations`, `applies_to`, `enforced_by`).
    Validação completa é feita pela wiki.
  - Identidade do ator e do autor vem da camada de autenticação. A cadeia de admissão só é
    acionada se houver ator identificado ou lista de pulo.
  - Antes da escrita tenta um checkpoint prévio (`pre-checkpoint before mcp write-page <path>`) e depois da escrita tenta um checkpoint (`mcp write-page <path>`). Falha no checkpoint gera aviso, não erro.
  - Resposta: `{ page_id, path, checkpoint }` (e `manifest_warning` opcional).
- **`memory_delete_page`:** operação de exclusão física de todas as versões do caminho no banco e do arquivo em disco (com limpeza de entidades órfãs e registro em `audit_log` se alguma linha foi apagada). Aceita `expected_page_id` opcional (retornando `-32600` `precondition_failed` se a versão atual divergir ou se a página não existir). Sem `expected_page_id`, a operação é idempotente e retorna sempre `{ path, deleted: true, pre_checkpoint?, checkpoint? }`.
  `scope: "profile"` é aceito e não combina com `workspace` nem `project` (`scope: "global"` não é aceito na exclusão).
- **`memory_feedback`:** tipos `helpful`, `not_helpful`, `stale` ou `wrong`. O motivo é opcional:
  é higienizado (removem-se segredos, depois colapsam-se espaços) e cortado em **500** caracteres.
- **`memory_consolidate`:** consolida uma sessão em uma página ou em várias (`multi_page=true`).
  Sem `session_id`, usa a sessão concluída mais recente visível no projeto. Aceita `dry_run` (plano
  sem chamar o LLM) e `instructions` por chamada, tratadas como preferências consultivas e não
  confiáveis.
- **`memory_auto_improve`:** executa a auto-melhoria de uma sessão. Os argumentos `dry_run`,
  `stage` e `mode` foram removidos e são rejeitados. A revisão manual é configurada em
  `[auto_improve].require_approval`. Sem `session_id`, usa a sessão concluída mais recente sem
  execução de auto-melhoria.
- **`memory_forget_sweep`:** varredura de esquecimento. `dry_run` mostra o que faria sem alterar nada.
  Exige autorização de administrador.
- **`memory_lint`:** verificação de qualidade da wiki. Sem `dry_run`, grava o relatório na área
  de lint da wiki.

**Passagem de bastão (handoffs):**

- `begin`:
  - honra o `workspace` informado na resolução de escopo de **escrita**;
  - aceita `summary` (obrigatório), `next_steps`, `open_questions`, `files_touched`, `from_agent` (padrão `"mcp"`), `to_agent`, `cwd`, `session_id` e `shared` (padrão `false`);
  - o resumo e os itens passam por sanitização e são truncados com sufixo explícito (nunca rejeitados por tamanho): resumo **3000** caracteres (`\n… [handoff summary truncated at 3000 chars]`); cada item **1500** (`\n[<label> truncated; <omitted> chars omitted]`); cada caminho de arquivo **512**; listas de texto **6000** caracteres no total e listas de arquivos **4096**; no máximo **20** itens por lista (`[<list_label> truncated; <N> additional item(s) omitted]`);
  - o dono é o criador. Com `shared=true`, o dono fica vazio e qualquer operador pode pegar o
    bastão;
  - a autorização de criação ocorre **antes** da inserção; os observadores são notificados depois;
  - o retorno traz `{ handoff_id, created_at }`.
- `list`: não reivindica nada. Aceita `state` (`"open"` [padrão], `"accepted"`, `"expired"` ou `"all"`), `limit` (padrão **20**, limitado a **1..100**) e `all_owners` (booleano; só o administrador pode usar `all_owners=true`). Retorna `{ handoffs: [...] }`.
- `accept`: reivindica um bastão pelo identificador exato (`handoff_id`) ou o melhor candidato para `cwd`, com comparação atômica (só uma sessão concorrente vence). Não aplica o filtro de 10 minutos de sessão viva, mas executa a varredura pós-reivindicação quando reivindica um handoff automático. `all_owners=true` só para administrador. Retorna `{ handoff: <objeto|null>, status: "claimed" | "consumed_by_hook" | "none_pending" }`.
- `cancel`: marca como expirado o bastão com o identificador exato (`handoff_id`). `all_owners=true` só para administrador. Se não encontrado ou invisível ao operador, retorna erro JSON-RPC `-32603`; se cancelado agora, retorna `{ handoff_id, cancelled: true, state: "expired" }`; se já estava fechado, retorna `{ handoff_id, cancelled: false, state: "accepted" | "expired" }`.

**Mensagens entre projetos:**

- `send`: exige que o `to_workspace` e o `to_project` já existam. Nunca cria destinatário; um erro
  de digitação falha de forma fechada. O corpo passa por sanitização e é limitado a **8000**
  caracteres; o assunto a **200**. Passa pela admissão de envio de mensagem no escopo do
  destinatário.
- `list`: `box=inbox` (padrão, caixa de entrada) ou `outbox` (caixa de saída). Limite padrão **50**,
  máximo **200**. Se a caixa de entrada foi resolvida por escopo **inferido** (e não nomeado), e a
  lista vem vazia, a resposta inclui uma dica explicando que a caixa pode não ser a esperada e
  sugerindo informar `workspace` e `project`.
- `pop`: retira a próxima mensagem. O corpo sai delimitado como **entrada não confiável**, com a
  proveniência do remetente fora do delimitador, para que possa ser avaliada primeiro. A garantia
  exata de atomicidade da retirada a confirmar no armazenamento.
- `cancel`: retira uma mensagem enviada pelo próprio remetente, pelo identificador. Mensagens já
  retiradas não são afetadas.

Todas as operações de mensagem passam pela admissão com operação própria de mensagem.

---

## 4. Autenticação HTTP

### 4.1 Estado

O estado de autenticação guarda:

- o token de administrador (raiz) esperado;
- se o cookie de sessão deve ser marcado como seguro;
- o ator associado ao administrador;
- o resolvedor de usuários do banco (quando há autenticação multiusuário), com o pepper dos hashes
  de token e os leitores e escritores do armazenamento;
- o token do proxy de identidade confiável;
- o estado da autenticação humana e a intenção de habilitá-la.

Um token configurado **em branco** equivale a autenticação **desabilitada**. Nunca se pode aceitar
um token vazio como autenticação de administrador.

### 4.2 Resolução do token portador

Ordem de avaliação:

1. Token vazio: **rejeitado**.
2. **Administrador:** comparação em tempo constante com o token raiz (comparando os digests SHA-256 de 32 bytes do token fornecido e do esperado para não vazar o comprimento do token). Se bate, injeta o ator do
   administrador e o nível de administrador.
3. **Proxy de identidade confiável.** Só vale com token portador explícito. Nunca vale com Basic
   nem com cookie. A comparação com o token do proxy é em tempo constante. Depois lê os cabeçalhos
   `X-Memory-Actor-*`:
   - cabeçalho **repetido**, ou com vírgula, gera erro de **ambiguidade** (suspeita de falsificação).
     O proxy deve **substituir** esses cabeçalhos, nunca anexar. Resposta HTTP **400**;
   - é obrigatório informar `user`, **ou** o par completo `issuer` + `sub`. Par parcial gera **400**;
   - o nível é de administrador **somente** se o par `issuer` + `sub` coincidir exatamente com o do
     administrador configurado. Nome de usuário nunca concede administrador. Caso contrário, nível
     de usuário.
4. **Multiusuário:** calcula o hash do token com o pepper e procura um usuário ativo com esse hash.
   Se achar, injeta o ator (usuário, nome, e-mail), o identificador do usuário, a autorização de
   visualizador e o nível `User`. Em segundo plano (sem bloquear), atualiza o último uso da
   credencial e o último acesso do usuário; falhas são só registradas.
   Falha ao consultar o banco também resulta em **rejeição**.
5. Caso contrário: **rejeitado**.

Extração do cabeçalho `Authorization`: aceita **um único** token portador. Se houver mais de um,
o resultado é vazio, e a autenticação falha de forma fechada. Esquemas Basic e desconhecidos são
ignorados. O esquema `Bearer` é comparado sem diferenciar maiúsculas.

### 4.3 Rotas de máquina

Valem para `/mcp`, `/hook`, `/hook/batch`, `/handoff` e `/workstream/*` (todas atrás do filtro externo de cabeçalho `Host`, que responde `400 missing Host header` quando ausente e `403 forbidden host` quando fora de `allowed_hosts`):

- autenticado: segue;
- sem autoridade configurada: **passagem anônima** (acesso local sem configuração), sujeita à proteção contra requisições cross-origin de navegador (`Sec-Fetch-Site`, `Origin`, `Sec-Fetch-Dest` e exigência de `X-AI-Memory-Client` em `GET /handoff` quando não-loopback ou com cabeçalhos de encaminhamento). Um token portador antigo é tratado como anônimo;
- caso contrário: **401** com cabeçalho `WWW-Authenticate: Bearer` e corpo JSON `{"error": "unauthorized: missing or invalid Authorization: Bearer <token> header"}`.
  Nunca anuncia Basic.

Sessões web **nunca** autenticam `/mcp`.

### 4.4 Níveis de acesso

Três níveis de autoridade: anônimo, usuário e administrador. Token de banco e proxy de identidade
produzem usuário, exceto quando a identidade afirmada pelo proxy é a do administrador configurado.
Autenticação humana por senha gera uma sessão (seção 5), que não altera o nível de autoridade do token.

A decisão de autorização para cada operação é tomada por um único ponto, com estas capacidades:

- **administração** (`/admin/*`): aberta quando o servidor **não distingue operadores** (sem usuários
  no banco e sem proxy de identidade confiável). Quando distingue, exige administrador. Autenticação
  ausente gera **401**; negação gera **403**;
- **gestão de usuários**: sempre exige administrador;
- **pular a cadeia de admissão** (webhooks): usuários comuns não podem; administrador e acesso anônimo podem.

"Distingue operadores" é verdadeiro se o servidor tem proxy de identidade confiável **ou** se já existe
pelo menos um usuário no banco. A verificação é feita a cada requisição, sem cache: criar o primeiro
usuário fecha o acesso administrativo aberto imediatamente, sem reinício.

---

## 5. Autenticação humana

Quatro classes, isoladas entre si:

- **Senha:** só emite sessão.
- **Sessão web:** cookie `ai_memory_session` (somente HTTP) mais cookie de CSRF `ai_memory_csrf`
  (legível pelo navegador). Mutações exigem o cabeçalho `x-csrf-token` com o valor do cookie de CSRF.
  Autoriza `/auth/me`, `/admin/*` e `/api/v1/*`. **Nunca** autoriza `/mcp` nem hooks.
- **Recuperação:** redefine a senha do administrador por um segredo de recuperação. **Não** emite
  sessão. Responde **204** e expira o cookie legado.
- **Chave de API:** credencial de máquina (seção 4).

**Compatibilidade descontinuada:** credenciais Basic e o cookie legado `ai_memory_auth` continuam
válidos **somente para GET** de navegador, e só enquanto não existir senha humana nem bootstrap
concluído. Assim que houver um deles, o cookie legado deixa de valer imediatamente, sem reinício.

Rotas:

- públicas: `POST /auth/login`, `POST /auth/recovery`;
- de sessão: `GET /auth/me`, `POST /auth/password`, `POST /auth/logout`;
- interna (de sidecar, exige o token do proxy): `POST /internal/auth/session-introspect`.

Middlewares: exigir sessão, exigir sessão ou anônimo, e dupla autenticação (token portador **ou**
sessão, conforme a capacidade).

Login:

- corpo limitado a **64 KiB**; corpo inválido gera **400**;
- **limite por IP:** no máximo **10 falhas em 60 s** por IP. Acima disso, responde **429** antes de
  qualquer verificação de senha. A tabela de IPs tem no máximo **4096** chaves; ao estourar, remove o
  IP mais antigo;
- usuário inexistente, desativado ou sem senha: executa uma verificação fictícia de senha (para
  equalizar o tempo) e responde **401** `invalid credentials`;
- senha incorreta: **401** com a mesma mensagem;
- saturação do processamento de senha: **429** `kdf saturated`;
- sucesso: retorna o estado da conta e grava os cookies de sessão. O cookie de sessão é somente HTTP;
  o cookie de CSRF não é somente HTTP. Ambos recebem a marcação segura conforme a configuração (nunca
  por cabeçalho do proxy).

Endereço IP do cliente: sem proxy confiável, é o endereço de conexão. Com proxy confiável (lista de
CIDRs configurada), percorre o `X-Forwarded-For` do fim para o início e pega o primeiro endereço que
**não** pertence aos CIDRs confiáveis. Cabeçalho malformado ou ausente cai de volta para o endereço
de conexão.

Política de senha:

- entre **12** e **1024** bytes;
- não pode ser igual ao nome de usuário;
- não pode começar pelos prefixos de segredos de sessão nem de chaves de API (nativa ou externa);
- não pode ser um dos segredos reservados (configuração) nem o segredo de recuperação;
- não pode coincidir com nenhuma chave de API existente;
- erro de política é genérico (`does not meet policy`), para não revelar a regra.

Recuperação de senha:

- o segredo de recuperação é guardado como **hash SHA-256**; a comparação é em tempo constante;
- falha no segredo ou na política gera o mesmo tratamento de falha de login (contagem por IP);
- senhas são guardadas com **Argon2id**, em processamento separado do escritor do armazenamento,
  com no máximo **2** verificações simultâneas.

---

## 6. Rotas administrativas

Todas as rotas `/admin/*` passam pela mesma verificação de administrador descrita em 4.4. Nos modos
sem autenticação, mantêm o comportamento de instalação de operador único. Em multiusuário, exigem
administrador. Autenticação ausente gera **401**; negação gera **403**; falha ao ler a configuração de
operadores gera **500** `admin authorization unavailable`.

| Grupo | Rotas |
|---|---|
| Backup e exportação | `POST /admin/backup`: arquivo comprimido com cópia online do banco e da wiki, além do arquivo de configuração quando existir. Resposta binária em streaming. `POST /admin/export-okf` |
| Bootstrap e aprendizado | `POST /admin/bootstrap` (recebe fontes já coletadas pela CLI), `POST /admin/auto-improve`, `POST /admin/auto-improve/report`, `POST /admin/curator`, `POST /admin/embed` |
| Escritas pendentes | `GET /admin/pending-writes`, `GET /admin/pending-writes/{id}`, `GET /admin/pending-writes/{id}/diff`, `POST /admin/pending-writes/{id}/approve`, `POST /admin/pending-writes/{id}/reject` |
| Manutenção | `POST /admin/lint`, `POST /admin/forget-sweep`, `POST /admin/reorg`, `POST /admin/compact`, `POST /admin/commit`, `GET /admin/checkpoints`, `POST /admin/restore-page`, `POST /admin/reclaim-ledger-versions`, `POST /admin/repair-session-times` |
| Leitura | `GET /admin/status`, `GET /admin/projects`, `GET /admin/project-coordinate`, `GET /admin/open-sessions`, `GET /admin/sessions/by-agent`, `GET /admin/activity/by-client`, `GET /admin/audit-contamination`, `GET /admin/audit-log`, `GET /admin/search`, `GET /admin/read-page`, `GET /admin/handoffs`, `GET /admin/messages` |
| Acesso a projetos | `GET /admin/projects/grants`, `POST /admin/projects/access` |
| Bastões e mensagens | `POST /admin/handoffs/expire`, `POST /admin/messages/send`, `POST /admin/messages/pop`, `POST /admin/messages/cancel` |
| Ciclo de vida | `POST /admin/purge-project`, `POST /admin/purge-session`, `POST /admin/rename-project`, `POST /admin/rename-workspace`, `POST /admin/move-project`, `POST /admin/move-session`, `POST /admin/merge-workspace`, `POST /admin/delete-workspace` |
| Páginas | `POST /admin/write-page`, `POST /admin/delete-page` |
| Usuários e credenciais | `GET /admin/users`; `POST /admin/human-users`; `PATCH /admin/users/{username}`; `POST` em `/admin/users/{username}/` com `reset-password`, `disable`, `enable`, `grant` e `revoke`; `GET /admin/users/{username}/grants`; `GET` e `POST /admin/api-credentials`; `POST /admin/api-credentials/{id}/rotate`; `POST /admin/api-credentials/{id}/revoke` |

Regras relevantes:

- **Aprovação de escrita pendente** segue o mesmo caminho de escrita da wiki. Se a página mudou desde
  que a proposta foi preparada, a aprovação responde com conflito.
- **Operações destrutivas** exigem escopo explícito e falham de forma fechada. Passam pela admissão
  própria de cada operação de purga. Ao final, invalidam o cache de roteamento dos hooks
  (invalidador de escopo) e o ponteiro de projeto ativo.
- **`move-project`** recusa mover o projeto em uso, a menos que seja forçado.
- **`status`** agrega contagens, caminhos, versão, saúde dos provedores, métricas de ingestão,
  tamanho da fila de escrita, estado dos índices derivados e avisos de busca textual e de embeddings.

### 6.1 Perfil entre projetos

Visão do operador sobre o perfil entre projetos. Mesmo portão das demais rotas `/admin/*`: aberto em
operador único, administrador quando o servidor distingue operadores.

Escopo inspecionado (sem criar nada):

- `Global`: escopo padrão `default/_global`;
- `Workspace`: `<workspace>/_profile` (parâmetro `workspace`, padrão `default`);
- `Usuário`: escopo de perfil do usuário em `default`, identificado pelo parâmetro `user`. Sem `user`,
  ou com usuário desconhecido, o escopo é "nenhum".

Workspace ou projeto inexistente resulta em "nenhum escopo", não em erro. A CLI de perfil reutiliza
as rotas de leitura e de exclusão de página sobre o escopo informado pelo `status`.

| Rota | Faz |
|---|---|
| `GET /admin/profile/status` | Modo efetivo (`enabled`, `share`, `effective_share`), se o servidor distingue operadores, mínimo de projetos, injeção no início da sessão, tamanhos máximos do digest e da linha de base, tamanho máximo das linhas de aplicação, configuração de LLM, escopo inspecionado, número de entradas, tamanho do digest e projetos que optaram por não contribuir. |
| `GET /admin/profile/list` | Entradas do perfil inspecionado, por caminho. |
| `GET /admin/profile/review` | Entradas com evidência, grupos ainda abaixo do limiar e o que uma nova passada atualizaria, deixaria (editado à mão) ou manteria removido (esquecido). `user` inexistente gera **404**. |
| `GET /admin/profile/apply?workspace&project[&user]` | Linhas que a CLI de perfil grava no arquivo de regras do projeto, com `scope`, `consume`, `lines` e `omitted`. A seleção é a mesma do digest (escopo de stack; entradas aplicadas em outro lugar são puladas), limitada ao tamanho máximo de aplicação. Projeto com `consume = false` recebe `consume: false` e nenhuma linha. Projeto desconhecido recebe todas as entradas, como no digest de linha de base de projeto novo. Perfil desligado gera **409**. |
| `POST /admin/profile/rebuild` | Limpa as marcas de colheita e refaz a passada do perfil: relê todo projeto contribuinte desde o início e converge todos os escopos de perfil. Registrar o mesmo candidato duas vezes não tem efeito, então repetir é seguro. Classifica e funde com o LLM quando a configuração de perfil permite. Perfil desligado gera **409**. |

---

## 7. Notas para reimplementação

1. Comece pelas quatro ferramentas centrais (`memory_query`, `memory_write_page`, `memory_recent` e
   `memory_read_page`), sobre um resolvedor de escopo que falha de forma fechada. O resto é aditivo.
2. Tenha **uma** função de identidade, que transforma a autenticação em contexto do ator. Os handlers
   leem só esse contexto.
3. `memory_query` precisa **degradar** sem erro quando o embedder, o reranqueador ou a síntese
   falham, e ser observável (`explain` e `streams_active`).
4. Todo parâmetro de escopo é um par (`workspace` e `project`). Recuse quando vier só a metade.
5. Controle de acesso por capacidade em um só lugar. `/admin/*` vira exclusivo de administrador quando
   a implantação passa a distinguir operadores.
6. Testes adversariais valem mais que testes de caminho feliz: identificador ou escopo de outro
   projeto recusado, `global` combinado com escopo, token portador repetido, cabeçalho
   `X-Memory-Actor-*` duplicado, proxy que anexa em vez de substituir. Ver
   [security-boundaries.md](security-boundaries.md).
