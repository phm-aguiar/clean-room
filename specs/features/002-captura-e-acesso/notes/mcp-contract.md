# Contrato das 12 tools MCP e resolução de escopo (T003)

Fontes, em ordem de prioridade: `docs/` (vence), depois `specs/features/002-captura-e-acesso/spec.md` e `plan.md`. Onde o doc é omisso, o texto marca **decisão nossa**. Siglas: MS = `docs/mcp-server.md`; AR = `docs/architecture.md` (seção "MCP tool surface"); SR = `docs/storage-and-retrieval.md`; DV = `docs/domain-vocabulary.md`; HO = `docs/design-handoff-offer-not-claim.md`; DD = `docs/design-decisions.md`.

## 0. Divergências entre fontes (atenção do Analyst)

- MS §1 e cabeçalho dizem "23 ferramentas". AR diz "20 tools" no título, mas a tabela lista 22 linhas. A contagem não é consistente nos docs. A 002 implementa um **subconjunto de 12** (spec.md "Dentro do escopo", linha das tools; plan.md §2.3). Ficam **fora** da 002: `memory_feedback`, `memory_forget_sweep`, `memory_lint`, `memory_install_self_routing`, `memory_explore`, `memory_consolidate`, `memory_auto_improve`, `memory_message_*` (4) (spec.md "Fora do escopo"; plan.md §6 "O que não construir").
- O doc usa `memory_handoff` como 4 tools separadas (AR; MS §3.3; HO §7). O plano já alinhou (plan.md linha 15; spec.md tabela de divergências, item 1).
- Algumas respostas só aparecem no doc com "a confirmar" (marcadas abaixo).

## 1. Princípios do servidor (MS §1, §2; plan.md §2.3, Q-07)

- Protocolo MCP fixo em **2024-11-05**; nome do servidor `ai-memory`; capacidade `tools`; o servidor anuncia instruções de uso (MS §1). Se o `mcp-go` não permitir fixar a versão, a primeira task de MCP registra ADR (plan.md Q-07).
- Transportes: stdio (padrão) e HTTP streamable sem estado, resposta JSON, em `POST /mcp` (plan.md §2.2 e Q-07). Autenticação Bearer; sessão web nunca autentica `/mcp` (MS §4.3, §5).
- Identidade vem só do contexto da requisição, preenchido pelo middleware de autenticação, nunca de cabeçalho cru no handler. Não usar variáveis associadas à tarefa, porque a lib MCP roda cada tool em tarefa separada (MS §1).
- Escopo sempre pelo resolvedor de escopo; leitura não cria e falha fechado; escrita pode criar workspace e projeto nomeados (MS §1).
- **Ponto único de chamada de tool** registra atividade do cliente e alimenta o relógio de atividade do ciclo de sonho (MS §1). Na 002 não há dream pass; **decisão nossa**: manter o ponto único (wrapper) e registrar a atividade só se a 001 já tiver o armazenamento (`client_activity`); caso contrário, deixar fora, sem stub (plan.md §6).
- Schemas das tools gerados pelo `mcp-go` a partir das structs de argumento (plan.md §1, "Geração de Código"). Dialetos `?flavor=` ficam **fora** (plan.md §6).
- Toda escrita de página passa pelo caminho único da wiki (sanitização, admissão, atribuição, rollback, índice) (MS §1). Na 002 não há cadeia de admissão (plan.md linha 6).
- O pacote `mcp` não importa `ingest` em produção; ponteiro de projeto ativo, seleção e reivindicação de handoff moram em `storage` (plan.md §1, Q-05, Q-06).
- Sem `panic`: pânico em handler vira erro de tool, sem derrubar o servidor (plan.md checklist, linha 223).

## 2. Resolução de escopo e chave do ator

### 2.1 Chave do ator (MS §2.1; DV §9.3, §8.4; plan.md §2.3)

- Chave do ator = (usuário, id de sessão). O "usuário" é a **chave de identidade** de armazenamento (DV §8.4): `user:<nome>` ou `oidc:<len emissor>:<emissor><sujeito>`. Na 002 só existe o ator anônimo ou o do token único, então o usuário é vazio ou `user:<nome>` (**decisão nossa**: multiusuário/OIDC fora do escopo, plan.md linha 6).
- Id de sessão, nesta ordem (MS §2.1; plan.md §2.3): (1) contexto de transporte; (2) cabeçalho `x-memory-actor-session-id`; (3) id de sessão nativo; (4) cabeçalho `mcp-session-id`.
- O roteador de hooks precisa usar a **mesma regra**; senão leituras e escritas do mesmo operador caem em slots diferentes (MS §2.1).
- Chave vazia (sem usuário nem sessão) usa o slot compartilhado (DV §9.1, §9.3).

### 2.2 Ponteiro de projeto ativo (DV §9; plan.md Q-06, §2.4)

- Modos: `single`, `per_session`, `per_actor` (**padrão**) (DV §9.1). Na 002: `per_actor` padrão; `single` e `per_session` por configuração (plan.md Q-06).
- Cada entrada guarda projeto resolvido, preferência `default_global` e instante de inserção. TTL **1 h**; teto **4096** entradas, removendo as mais antigas; expiradas removidas na leitura e na escrita (DV §9.3).
- Marca "ponteiro ativo" (alguma entrada já publicada) nunca é apagada (DV §9.4).
- Consulta de escrita `Lookup` devolve `Resolved | Mismatch | Unset`. `Mismatch` = chamador tem coordenada, ponteiro ativo e nada casou; **nunca** responde com o slot de outra sessão, falha fechada (DV §9.4).
- Consulta de leitura `Read` devolve origem: `Session`, `SharedSlot`, `StartupSeed`, mais `Mismatch` e `Unset` (DV §9.4).
- A semente de startup (`--project`) é **só para leitura**; nunca grava (DV §9.4).
- Hooks (início de sessão, prompt, pré-ferramenta) avançam o ponteiro; eventos de conclusão só atualizam a entrada exata do ator (plan.md Q-06).

### 2.3 `ScopeSource` (SR §14.1; DD §11; MS §2.1; AR `memory_status`)

- Valores: `Explicit`, `Session`, `SharedSlot`, `StartupSeed`, `Default`, `DefaultAfterMismatch` (SR §14.1). No JSON de `memory_status.scope.resolved_by`: `explicit`, `session`, `shared_slot`, `startup_seed`, `default`, `default_after_mismatch` (AR; DD §11). AR marca a lista de valores "a confirmar".
- "Fallback" = `StartupSeed` e `DefaultAfterMismatch`; "inferidas" = tudo exceto `Explicit` e `Session` (SR §14.1). Essas classificações alimentam avisos: leitura sem escopo resolvida por `startup_seed` ou `default_after_mismatch` registra aviso do servidor (MS §2.1; AR).
- Precedência de **leitura** (MS §2.1): (1) `project` explícito no workspace ativo do ator; (2) `project` explícito no workspace padrão do servidor; (3) ponteiro de projeto ativo do ator; (4) projeto padrão de startup (`--project`). DD §11 diz que a ordem exata é "a confirmar"; MS §2.1 é o texto mais específico, portanto é o adotado (plan.md linha 83 também o adota).
- Leitura sem escopo nunca falha: degrada por ponteiro, semente de startup e padrão do servidor, e informa a origem (SR §14.1).

### 2.4 Regras de escopo para leitura e escrita

- **Leitura, busca e operações destrutivas:** busca sem criação; escopo explícito inexistente falha fechado (não encontrado) (SR §14.1).
- **Escrita** (única que cria workspace e projeto nomeados) (SR §14.1; MS §2.1):
  - sem `project` explícito, usa o ponteiro ativo;
  - com `project` e sem `workspace`, usa o workspace **do ponteiro**, não o padrão do servidor (evita recriar `default/<projeto>`);
  - ponteiro que não corresponde ao pedido: falha com erro de escrita **não ambígua**; nunca grava silenciosamente no padrão;
  - sem escopo e sem ponteiro resolvido: recusada (escrita ambígua) (SR §14.1). Decisão nossa para a interação com `Unset`: DV §9.4 diz que, em `Unset`, "o padrão configurado é a única resposta"; logo, com `Unset` (instalação sem hooks) a escrita usa o padrão configurado; só `Mismatch` recusa.
- **Par obrigatório:** `workspace` e `project` vêm **juntos**; informar só um é erro de entrada (SR §14.1; MS §7 item 4). Exceção da escrita acima: `project` sem `workspace` na escrita usa o workspace do ponteiro (MS §2.1). **Decisão nossa** para reconciliar: na leitura, metade do par é erro de entrada; na escrita, `project` sozinho herda o workspace do ponteiro, e `workspace` sozinho é erro.
- **`scopes`:** lista de pares `{workspace, project}`, no máximo **25**; cada par resolvido como existente, sem criar (MS §2.1; plan.md §2.3).
- Erros de escopo: **entrada inválida** (equivale a 400) e **não encontrado** (equivale a 404) (SR §14.1). Como tool MCP: **decisão nossa**, ambos viram resultado de erro da tool (`isError`), com mensagem distinguindo as duas classes; o doc não define o formato do erro da tool.
- Nomes de projeto de perfil são reservados para escrita de perfil (SR §14.1); fora da 002 (perfil excluído), mas **decisão nossa**: recusar escrita em `_profile` e similares.
- Escopo reservado `_global` (`default/_global`): leitura de projeto único une `_global` em `global_scope_hits`, sem duplicar se o projeto resolvido já é `_global` (MS §3.1 passo 7; plan.md §2.3).
- Autorização por projeto (SR §14.2): sem distinguir operadores, **permite**. Na 002 (sem multiusuário), é sempre permitido. Escrita em `_global` por usuário sem concessão é multiusuário, fora do escopo.

### 2.5 Aliases de parâmetros (MS §2.1; AR)

- `query` aceita também `q` e `search` (`memory_query`; MS §2.1 diz "`query` aceita também `q` e `search`").
- Limite: `n` ou `top_k` em `memory_query`; `n` em `memory_recent` (MS §2.1; plan.md §2.3). AR: "Limit fields accept `n` or `top_k` where shipped". Os aliases são deliberadamente poucos; `project` e `cwd` usam o nome canônico (AR).
- **Decisão nossa** (omissão): se canônico e alias vierem juntos com valores diferentes, o canônico vence.

## 3. Contrato por tool (12)

Tipos de hint (AR): leitura = read-only; escrita/handoff_begin/accept/cancel = destructive. Argumentos de escopo comuns às tools de projeto único: `workspace` + `project` (par).

### 3.1 `memory_query` (leitura) — MS §3.1; AR; plan.md §2.3

- **Argumentos na 002:** `query` (aliases `q`, `search`), `workspace`+`project`, `scopes` (≤ 25), `global`, `as_of`, `limit` (padrão do servidor = 10, faixa 1..100; aliases `n`, `top_k`), `include_expired`, `include_superseded`, `explain`, `pin_first`.
- **Fora da 002:** `answer` e `reasoning` (INV-13: sem provedor, vetor ou rerank; plan.md §2.3, checklist). **Decisão nossa:** não declarar `answer`/`reasoning` no schema. O doc diz que, com `answer`, sem provedor a resposta traz nota `answer_unavailable` (MS §3.1 passo 9), mas isso depende de LLM; se a spec decidir aceitar o argumento, devolver `answer_unavailable` (nunca erro). Pendente para o Analyst.
- **Limite padrão do servidor:** 10, fixo, "não muda por construtor" (MS §2).
- **Fluxo na 002:** só FTS + entidade + grafo, sem vetor (plan.md §2.3). Equivale ao ramo "geração de vetor falhou", que **degrada** sem erro (MS §3.1 passo 3). RRF e ajuste de autoridade (kind, tier, pinned, tag) conforme AR e 001.
- **Candidatos:** sem reranqueador, o próprio `limit` (MS §3.1 passo 3). Com `scopes`: busca em cada escopo, mescla por id mantendo o **menor rank**, ordena por (rank, caminho, id) e trunca (MS §3.1 passo 3).
- **`global=true`:** (também ligado se o ponteiro do ator tem `default_global` e nenhum argumento de escopo foi passado; argumento explícito sempre vence). Proíbe combinar com `workspace`, `project` ou `scopes` (erro). Ranqueador só FTS, sem RRF (MS §3.1 passo 1). Cada hit é rotulado com workspace e projeto (AR). Resposta usa `global_hits` (MS §3.1 passo 10).
- **`as_of`:** instante ISO-8601; inválido = erro. Proíbe `global` e `scopes` (erro). Usa entidade + FTS filtrados pelas janelas de ingestão de página vigentes no instante; sem vetores, grafo, observações brutas nem incremento de acesso (MS §3.1 passo 2). Dependência de `docs/design-page-ingestion-windows.md` (não lido; confirmar na 001 se existe suporte).
- **Incremento de acesso assíncrono:** em segundo plano; cada par (página, operador) recebe no máximo 1 incremento a cada **60 s**; falhas só registradas (MS §3.1 passo 5).
- **Fallback de observações brutas:** se nenhuma página compilada casou, FTS nas observações do escopo resolvido (ou em cada escopo de `scopes`, mesclando por rank); resultado em `raw_hits` (MS §3.1 passo 6).
- **Preferências globais:** consulta de escopo único une `_global` em `global_scope_hits` (MS §3.1 passo 7). A parte de perfil entre projetos fica fora (plan.md §6).
- **`pin_first`:** só em projeto único sem `scopes`; antepõe até **10** páginas fixadas (limitadas ao `limit`), dedup por id, marcadas `pinned: true`, e trunca de novo no `limit` (MS §3.1 passo 8; AR).
- **`include_expired`:** inclui páginas com TTL vencido. **`include_superseded`:** inclui versões substituídas, cada uma com `superseded: true`; a versão atual nunca é rotulada; com `false` o resultado é igual ao "somente última" (AR).
- **`explain`:** adiciona `score_details` por hit (ranks por stream, entidades casadas, scores brutos, contribuições RRF, proveniência do grafo, contagem de evidência, multiplicador de autoridade) e `streams_active` no topo (AR; MS §3.1 passo 10). `streams_active` na 002: consulta normal lista `fts`, `entity`, `graph` (sem `vector` e `abstract`); `global` lista só `fts`; `as_of` lista `entity` e `fts` (MS §3.1 passo 10). `rerank_score` não existe na 002.
- **Resposta:** `hits`, `raw_hits`, `global_hits`, `global_scope_hits`, `streams_active` (só com `explain`), `score_details` (só com `explain`) (MS §3.1 passo 10). `answer` e `answer_unavailable` ficam fora (acima).
- **Erros:** `global` + (`workspace`|`project`|`scopes`) = erro; `as_of` inválido ou combinado com `global`/`scopes` = erro; `scopes` > 25 = entrada inválida (**decisão nossa** sobre a classe, o doc só fixa o teto); `limit` fora de 1..100: o doc diz "limitado a 1..100" (MS §3.1), então **clamp** e não erro (**decisão nossa**: clamp, sem erro); escopo inexistente = não encontrado; metade do par = entrada inválida; `query` vazio: o doc é omisso, **decisão nossa**: entrada inválida.

### 3.2 `memory_read_page` (leitura) — MS §3.2; AR; plan.md §2.3

- **Argumentos:** `path` ou `query`; `workspace`+`project`; `include_related` (opt-in); `related_depth` (padrão 1, máximo **3**).
- **Precedência:** `path` vence `query`. Com `query`, FTS e corpo completo do primeiro resultado (MS §3.2).
- **Resposta:** `path`, `title`, `body`, `frontmatter`; `served_from` quando o `.md` falta e a página é servida do banco (MS §3.2). Com `include_related`, `related`: lista (máx. **50** nós, entre projetos) de itens `path`, `title`, `kind`, `workspace`, `project`, `depth`, `direction` (`link` ou `backlink`). Sem a opção, `related` não aparece (MS §3.2; AR). Padrão de 1 salto é "a confirmar" em AR, mas MS §3.2 o fixa.
- **Erros:** página inexistente no escopo resolvido = erro (MS §3.2); nem `path` nem `query` = entrada inválida (**decisão nossa**).
- **Efeito:** sucesso reforça a página lida e os vizinhos percorridos, com o mesmo incremento de 3.1 (60 s por operador); leitura com erro não reforça (MS §3.2).

### 3.3 `memory_recent` (leitura) — MS §3.2; plan.md §2.3

- **Argumentos:** `limit` (padrão 10, 1..100; alias `n`); `workspace`+`project` (**decisão nossa**: o doc só diz que é leitura de escopo resolvido; segue o par padrão).
- **Resposta:** páginas mais recentes por `updated_at` decrescente (MS §3.2). Forma dos itens: **decisão nossa** (mesmos campos de um hit de `memory_query`, sem scores).

### 3.4 `memory_read_session_observations` (leitura) — MS §3.2; AR; plan.md §2.3

- **Argumentos:** `session_id` (opcional), `limit` (padrão **50**, máx **200**), `body_max_chars` (padrão **4000**, limitado a **200..16384**), `order` (padrão crescente), `workspace`+`project`.
- **Sem `session_id`:** usa a sessão **concluída** mais recente visível no projeto (MS §3.2; AR).
- **Resposta:** observações paginadas; `total`; `elided_other_scope` (contagem de itens de outro escopo omitidos) (MS §3.2). Corpo acima do limite é cortado com marcador (MS §3.2). Restrito a linhas do escopo resolvido e a sessões que o chamador pode ver (AR).
- **Erros / omissões:** valor de `order` (nomes dos valores) não está no doc; **decisão nossa**: `asc` e `desc`, padrão `asc`. Sessão inexistente: doc omisso; **decisão nossa**: não encontrado.

### 3.5 `memory_status` (leitura) — MS §3.2; AR; plan.md §2.3

- **Argumentos:** `workspace`+`project` (opcionais).
- **Resposta:** contagens, caminhos, versão, e `scope` com `workspace`, `project` e `resolved_by` (valores em 2.3) (AR; MS §3.2). Quais contagens exatas: doc omisso; **decisão nossa**: páginas (últimas versões), observações, sessões, handoffs abertos do escopo.
- **Efeito colateral:** leitura sem escopo resolvida por `startup_seed` ou `default_after_mismatch` registra aviso (AR; MS §2.1).

### 3.6 `memory_briefing` (leitura) — MS §3.2; AR; plan.md §2.3

- **Argumentos:** `workspace`+`project`; `settled_first` (bool, opt-in).
- **Resposta:** snapshot estruturado: contagens, atividade, regras, slots e páginas recentes (AR). `pinned`: até **10** páginas fixadas, separadas dos slots, omitido quando vazio (AR). `settled` (só com `settled_first=true`): até **8** páginas de regra e decisão de maior posição, por contagem de evidência e depois recência (MS §3.2). AR marca o limite 8 como "a confirmar"; MS §3.2 o fixa, e a spec o adota.
- **Fora da 002:** incremento de acesso do digest é de `memory_explore` (fora).

### 3.7 `memory_handoff_list` (leitura) — MS §3.3; HO §7; AR; plan.md §2.3

- **Argumentos:** `workspace`+`project`; `limit` (padrão **50**, máx **200**); `any_owner` (só administrador).
- **Comportamento:** lista handoffs `open` visíveis ao chamador (próprios e compartilhados), com corpo e campos de identidade inspecionáveis; **não** reivindica nem expira (AR; MS §3.3). Filtro de dono: primeiro (HO §3 regra 1): ator identificado vê "dono é este operador ou sem dono"; sem ator, só sem dono.
- **`any_owner=true`:** "root-only" (AR; MS §3.3). **Decisão nossa** na 002 (sem multiusuário): o nível administrador é o do token raiz; usuário comum não existe; anônimo sem autoridade configurada tem acesso local irrestrito (MS §4.3), mas **o doc não diz se anônimo pode usar `any_owner`**; decisão nossa: permitir só se autenticado como administrador (token raiz), recusar para anônimo.
- **Ordem:** "mais antigo primeiro" no comando `ai-memory handoffs` (AR); para a tool, doc omisso, **decisão nossa**: igual.

### 3.8 `memory_write_page` (escrita) — MS §3.3; AR; plan.md §2.3; spec.md linha 81

- **Argumentos:** `path`, conteúdo/`body`, `frontmatter` (`title`, `tags`, `pinned`, `expires_at`), `tier`, `scope`, `session_id`, `workspace`+`project`. Nomes exatos de `path`/`body`: decisão nossa seguindo a 001 (`wiki.WritePage`).
- **`tier`:** padrão `semantic`, exceto `sessions/<id>.md` que é `episodic` (MS §3.3).
- **Caminho:** validado e portável (MS §3.3, `PagePath` DV §4.2).
- **`scope`:** `"global"` grava em `default/_global`, sem `workspace`/`project`; `"profile"` fica **fora da 002** (perfil excluído); qualquer outro valor = erro listando os aceitos (MS §3.3). **Decisão nossa:** `"profile"` retorna o mesmo erro de valor não aceito, listando só `global`.
- **`session_id` + `scope`:** não podem ser usados juntos, a sessão pertence a um projeto (MS §3.3).
- **Frontmatter:** `expires_at` em RFC 3339 ou `AAAA-MM-DD`; validação completa pela wiki (MS §3.3). A tool apoia-se nos comportamentos de 001/T084 e T085 (escalares YAML não-texto aceitos; escrita sem `pinned` herda a fixação anterior; só `pinned: false` explícito desafixa) e não os reimplementa (spec.md linha 81).
- **Escopo:** regras de escrita em 2.4 (cria só nomeado; ponteiro; `Mismatch` falha).
- **Identidade:** ator e autor vêm da camada de autenticação; a cadeia de admissão só roda com ator identificado ou lista de pulo (MS §3.3). Na 002 não há cadeia (plan.md linha 6), logo atribuição ao ator do contexto.
- **Checkpoint:** após a escrita tenta commit; falha no checkpoint gera **aviso, não erro** (MS §3.3).
- **Resposta:** id da página, caminho e checkpoint (MS §3.3; plan.md §2.3). Avisos: campo à parte (**decisão nossa**: `warnings`).
- **Erros:** valor de `scope` inválido; `session_id`+`scope`; `scope=global` com `workspace`/`project`; caminho inválido; frontmatter inválido; `Mismatch` do ponteiro (escrita ambígua); página do ledger/`sessions` protegidas conforme regras da 001.

### 3.9 `memory_delete_page` (escrita) — MS §3.3; AR; plan.md §2.3

- **Argumentos:** `path` exato; `workspace`+`project`; `scope` (`"global"` por simetria; **decisão nossa**; MS só cita `"profile"`, fora da 002).
- **Resposta:** `deleted: true` e os checkpoints anterior e posterior (MS §3.3; plan.md §2.3). Soft delete (tombstone) obrigatório (constituição; AR/MS: exclusão por caminho exato).
- **Idempotência:** AR diz "Idempotent"; MS §3.3 diz "Comportamento para página inexistente a confirmar". **Doc vence com a ressalva:** **decisão nossa**: página inexistente retorna `deleted: false` sem erro (idempotente, conforme AR). Sinalizar ao Analyst por causa do "a confirmar".
- AR: roda a cadeia de admissão com operação de exclusão; na 002 não há cadeia.

### 3.10 `memory_handoff_begin` (escrita) — MS §3.3; HO §7; AR; plan.md §2.3

- **Argumentos:** resumo, perguntas em aberto, próximos passos, arquivos tocados (DV §10); `shared` (bool); `cwd` (registrado, mas não restringe quem recebe handoff manual: HO §7); `workspace`+`project`. Nomes exatos dos campos: decisão nossa seguindo DV §10 (`summary`, `open_questions`, `next_steps`, `files_touched`). `to_agent` não aceito (HO §3 e §8: proposta não implementada).
- **Limites (após sanitização):** resumo **3000** caracteres; cada item **1500**; cada caminho de arquivo **512**; listas de texto **6000** caracteres no total; listas de arquivos **4096**; no máximo **20** itens por lista (MS §3.3). Excesso: o doc fala em "limitados", ou seja, **truncados**; decisão nossa: truncar e não rejeitar, conforme o texto do doc.
- **Dono:** o criador. Com `shared=true`, dono vazio e qualquer operador pode pegar (MS §3.3; HO §7).
- **Escopo:** honra `workspace` informado na resolução de escopo de **escrita** (MS §3.3).
- **Ordem:** autorização de criação **antes** da inserção; observadores notificados depois (MS §3.3). Na 002 não há observadores.
- **Resposta:** identificador do handoff (MS §3.3). Handoff manual não tem sessão de origem e vence qualquer automático na seleção (HO §3 regra 3).

### 3.11 `memory_handoff_accept` (escrita) — MS §3.3; HO §7, §3; AR; plan.md §2.3

- **Argumentos:** `handoff_id` (opcional); `cwd` (do receptor, opcional); `any_owner` (só admin); `workspace`+`project`.
- **Sem `handoff_id`:** reivindica o mais recente elegível para o `cwd` (HO §3 e §7). Seleção: filtro de dono; só `open`; manual vence automático; automático só se o `cwd` armazenado é o do receptor ou ancestral por fronteira de componente (vazio = projeto inteiro; receptor sem `cwd` = todos candidatos); bastão de sessão viva (sessão aberta com observação nos últimos 10 min) não é reivindicado; desempate: manual antes de automático, `created_at` mais recente, `cwd` mais longo, id (HO §3).
- **Com `handoff_id`:** busca esse id no escopo, só se estiver aberto (HO §7). HO §7 marca "a confirmar" se a regra de sessão viva e a varredura pós-reivindicação valem por id. **Decisão nossa:** não aplicar a regra de sessão viva por id; aplicar a varredura pós-reivindicação (`SweepAfterClaimTx`, plan.md §2.4).
- **Atomicidade:** comparação atômica `open → accepted`, só uma sessão concorrente vence (MS §3.3; DV §10; plan.md §2.4, `ClaimHandoffTx` com dupla guarda).
- **Resposta:** só entrega o corpo se esta chamada **venceu**; quem perde a corrida recebe `handoff: null` (HO §7). Nenhum handoff elegível: **decisão nossa**: também `handoff: null` sem erro.
- **Efeito:** buscar é destrutivo (DV §10 lembrete); nunca buscar quando a saída será descartada.
- **Admissão `HandoffAccept`:** HO §7 cita a consulta à cadeia de admissão; fora da 002 (sem cadeia).

### 3.12 `memory_handoff_cancel` (escrita) — MS §3.3; HO §7; AR; plan.md §2.3

- **Argumentos:** `handoff_id` exato (obrigatório); `any_owner` (só admin); `workspace`+`project`.
- **Comportamento:** marca como `expired` o handoff visível e **aberto** de id exato, para a próxima sessão não consumi-lo (MS §3.3; AR "one exact visible open handoff").
- **Resposta:** doc omisso. **Decisão nossa:** `{cancelled: true, handoff_id}` quando mudou de `open` para `expired`; `cancelled: false` (sem erro) se já não estava aberto ou não é visível ao chamador; id ausente = entrada inválida.

## 4. Formato de erro (omissão do doc)

Os docs não definem o corpo do erro de tool MCP. **Decisão nossa:** resultado de tool com `isError: true` e texto curto sem segredos, sem caminhos do sistema de arquivos, sem valores informados que possam ser sensíveis. Erros de protocolo JSON-RPC apenas para método/ferramenta desconhecida e argumentos que não decodificam o schema. Reutilizar as categorias de erro do domínio (DV §3) e a divisão de erros de escopo (400-like / 404-like, SR §14.1). Credenciais: 401 com `WWW-Authenticate: Bearer realm="ai-memory", error="invalid_token"`, sem anunciar Basic (MS §4.3). Sem autoridade configurada: passagem anônima (MS §4.3).

## 5. Autenticação das rotas de máquina que afeta `/mcp` (MS §4.2, §4.3; plan.md §2.2)

- Cabeçalho `Authorization` aceita **um único** Bearer; mais de um = vazio = falha fechada; Basic e esquemas desconhecidos ignorados; `Bearer` sem diferenciar maiúsculas (MS §4.2).
- Token em branco configurado = autenticação desabilitada; nunca aceitar token vazio como administrador (MS §4.1).
- Comparação em tempo constante (`crypto/subtle`) (MS §4.2; plan.md §1).
- Corpo limitado a 10 MiB e filtro de `Host` antes da autenticação (plan.md §2.2).
- Proxy de identidade confiável e usuário no banco (MS §4.2 itens 3 e 4): fora da 002 (multiusuário).

## 6. Pendências para o Analyst

1. Aceitar ou não `answer`/`reasoning` em `memory_query` (INV-13 diz sem; MS devolve `answer_unavailable`).
2. Idempotência de `memory_delete_page` (AR sim; MS "a confirmar").
3. Formato dos campos de `memory_handoff_begin`, de `memory_recent` e de `memory_status` (doc omisso).
4. Regra de sessão viva em `memory_handoff_accept` por id (HO §7 "a confirmar").
5. Contagem inconsistente de tools nos docs (23 / 20 / 22 linhas); a 002 fixa 12.
6. Confirmar se `as_of` depende de janelas de ingestão de página já existentes na 001 (`docs/design-page-ingestion-windows.md`, não lido).
