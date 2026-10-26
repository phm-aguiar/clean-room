# Contratos e funcionalidades complementares

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

Este documento descreve o funcionamento de peças que cruzam vários componentes ou ficam na borda do produto e não cabem em um único documento de componente: contrato de captura externa, memória programática via MCP, identidade de repositório, reconhecimento do ledger de eventos, backups, política de handoff, perfil entre projetos, modo `--yolo` com isolamento, captura em aplicativos de desktop e adaptador de reranker.

Onde algo é **decisão aceita mas ainda não implementada**, está marcado como tal. Onde um detalhe não pôde ser confirmado, está marcado como **a confirmar**.

Índice: [1 Captura externa](#1-contrato-de-captura-externa) ·
[2 Memória programática](#2-memória-programática-via-mcp) ·
[3 Identidade de repositório](#3-identidade-de-repositório) ·
[4 Ledger de log](#4-reconhecimento-do-ledger-de-eventos-logmd) ·
[5 Backups](#5-backups) · [6 Handoff](#6-handoff-oferecer-não-reivindicar) ·
[7 Perfil](#7-perfil-entre-projetos) · [8 yolo e isolamento](#8---yolo-e-isolamento-ai-jail) ·
[9 Desktop](#9-captura-em-aplicativos-desktop) · [10 Reranker](#10-adaptador-de-reranker-externo) ·
[11 Prioridades para uma reimplementação](#11-como-isso-entra-numa-reimplementação)

Para a visão geral do produto, veja [overview.md](overview.md).

---

## 1. Contrato de captura externa

Um **orquestrador** que observa os agentes pode enviar eventos de ciclo de vida a **um** servidor ai-memory compartilhado, via `POST /hook/batch`. O servidor continua dono de armazenamento, recuperação, consolidação e memória durável; os adaptadores de framework ficam **fora** do núcleo. A implementação de referência é o *relay*, descrito em [external-companions-and-tests.md](external-companions-and-tests.md) (seção 2).

### 1.1 Escolher UM caminho de captura antes de lançar

`AI_MEMORY_CAPTURE_OWNER=<namespace-do-produtor>` no **ambiente do harness lançado**. Qualquer valor não vazio (ignorando espaços em branco) faz aquele processo, e os filhos que herdam o ambiente, optarem por captura externa. Variável vazia ou ausente ⇒ captura nativa normal.

| Responsabilidade | Com captura externa |
|---|---|
| Observações nativas de ciclo de vida | **Puladas antes** de spool, fila ou POST |
| Drenos nativos de hook e backfill de início de sessão | Pulados |
| Entrega de handoff, briefing e aviso de caixa de entrada | **Preservada** |
| Identidade nativa usada por handoff e MCP sensível a sessão | Preservada |
| Recall e escritas deliberadas via MCP | Inalterados |
| Política de captura do repositório | O **produtor** a verifica e aplica antes de enfileirar |

Notas:

- Claude Code recebe o contexto no início de sessão; Kimi Code recebe o contexto no envio de cada prompt (`UserPromptSubmit`). Entregar contexto pode registrar a sessão e **reivindicar** um handoff: suprimir a captura **não** torna o hook somente leitura. Se o hook é dono da entrega, o orquestrador **não** deve reivindicar o mesmo handoff.
- É **contexto herdado do processo**, não configuração do servidor, identidade de segurança nem lease. Mantenha-o fora do ambiente do servidor e do shell global. Um runtime que hospeda vários agentes num só processo aplica a variável a todos; ele precisa tratar a propriedade de forma mais fina por conta própria.
- `ai-memory hook --check-capture` reporta `external_capture: true` e `admits_capture: false` quando a captura externa está ativa, sem ingerir nada.
- **Pré-checagem do produtor:** `policy_admits_capture` ignora a propriedade de captura, mas verifica a política do repositório, a elegibilidade do evento, as exclusões, a completude do escopo e a resolução do perfil de servidor. Escopo parcial ou indisponível, ou perfil recusado, resulta em `false`. O produtor aplica a `disposition` (inclusive a remoção de metadados) antes de enfileirar.
  - `scope` carrega dicas de roteamento sanitizadas de até **512 bytes**. Dicas maiores são omitidas, e a pré-checagem falha fechada.
  - `scope_resolution` distingue `explicit`, `partial`, `server-derived` e `unavailable`.
  - `server_may_remap` avisa que o servidor pode escolher coordenadas diferentes.
- `ai-memory doctor` mostra se a invocação delega a captura nativa e conta, por agente, as sessões com eventos de **múltiplas fontes de captura**. É sinal de diagnóstico, não prova de duplicidade.

### 1.2 Reutilize o contrato público de eventos

- Para um harness em execução: envie o `agent` real do fio (`claude-code`, `codex` etc.) e o `session_id` nativo. Use o mesmo `session_id`, o mesmo cwd, o mesmo escopo e o mesmo operador autenticado da integração nativa restante. O `session_id` do **corpo** do evento vence o da query. Trocar só o ID de ingestão por um ID lógico do orquestrador **separa** o evento da identidade nativa usada por handoff e MCP.
- `workspace` e `project` explícitos em cada evento. Nomes canônicos de evento: `session-start`, `user-prompt-submit`, `post-tool-use`, `session-end` etc.
- `session-end` indica que a **execução** terminou, não que um turno acabou. Sessões que só têm eventos desconhecidos não são "substantivas" para resumos automáticos.
- `extension=<namespace>` (até 64 caracteres) **e** `source_event=<evento>` (até 128 caracteres) servem para proveniência. Ambos são necessários para preservar o namespace em eventos reconhecidos. Alfabeto dos dois: letras, dígitos, `.`, `_`, `-` e `:`. Eles descrevem a origem e não expandem o conjunto de eventos.
- Mantenha agentes lógicos, relações pai/filho e IDs de workflow no estado do **adaptador**. Não reaproveite campos `agent_id` nativos, pois alguns payloads os usam como marcador de subagente. O campo `managed_run` de workstreams tem lease, ledger e handoff próprios.

### 1.3 Identidade estável de retry

`ingest_key` é uma chave já existente, com escopo de projeto, independente de `extension`. Construa a chave **uma vez**, a partir da identidade estável do evento, e persista-a antes de entregar. **Nunca** derive a igualdade de eventos a partir do conteúdo.

```text
ingest_key = hex(SHA-256( JSON(["external-capture-v1", producer, actor, agent,
                                session_id, source_event, event_id]) ))   # 64 hex
```

- `actor` é o namespace de operador do adaptador, nunca um token.
- Chaves aceitas pelo servidor: de 1 a 64 caracteres, somente letras, dígitos, `_` ou `-`. Chave malformada **cai para ingestão sem chave**, por isso valide antes de enviar.
- O servidor reivindica chave e observação **na mesma transação**: replay completo é pulado; replay pendente retoma o trabalho a jusante sem inserir outra observação.
- Chaves expiram em **30 dias**. É proteção de retry limitada, **não** deduplicação permanente, e **não** é "exatamente uma vez" para todo efeito colateral da wiki.

### 1.4 Lotes ordenados e recuperação

`POST /hook/batch` recebe um array de itens `{url, body}`. A URL só fornece a query do evento; o servidor não a busca.

- Limite de **256** itens por lote. Acima disso, a requisição é recusada com HTTP 413 antes de qualquer processamento. Limites de corpo e sanitização normais valem.
- Processamento **inline, na ordem do array**. Não há ordenação global entre requisições concorrentes e **não há transação tudo-ou-nada** no lote.
- Falha de um item **interrompe** o lote naquele ponto. O reconhecimento informa `failed_index`, que identifica o item que falhou depois dos itens pulados antes dele.
- Reconhecimento: prefira `accepted_indices`. Sem ele, `accepted` é o prefixo contíguo aceito. Preserve para retry tudo o que não foi reconhecido. `results[]` traz `{index, outcome}` para cada índice aceito.
- Desfechos: `stored`, `replayed`, `resumed`, `ignored_end`, `dropped_policy`, `dropped_subagent`, `dropped_unauthorized`, `dropped_collision`, `dropped_invalid`. Um *drop* é **ack terminal**: o item não deve ser reenviado.
  - `stored`: observação gravada.
  - `replayed`: entrega com chave já concluída, repetida.
  - `resumed`: retomada de efeitos a jusante, sem nova escrita prometida.
  - `ignored_end`: fim ausente, com escopo inválido, ou sessão já encerrada sem efeito recuperado.
  - `dropped_policy`: a política de captura descartou a entrega.
  - `dropped_subagent`: captura de subagente desativada.
  - `dropped_unauthorized`: quem enviou não tem permissão de escrita no projeto.
  - `dropped_collision`: conflito de identidade ou de dono da sessão.
  - `dropped_invalid`: evento sem identificador de sessão e que não é início de sessão.
- Uma fonte limitada por taxa pode ser pulada enquanto as outras avançam. Nesse caso, o item é pulado com aviso e o lote continua. Inspecione o reconhecimento mesmo com HTTP 429 ou falha parcial.
- Timeout ou resposta perdida: reenvie os itens e as chaves **sem alterá-los**. Faça backoff em saturação.
- Serialize a entrega **dentro** de uma sessão. Sessões diferentes podem compartilhar o servidor. Observações tardias e um `session-end` posterior usam o comportamento existente para sessão retomada. Encerrar o processo pai **não** sintetiza fins para os filhos.
- `POST /hook` continua disponível, mas seu `202` é devolvido **antes** da persistência (o evento entra numa fila). Se a fila estiver cheia, a resposta é 429. Prefira `/hook/batch` se precisar de reconhecimento inline.

### 1.5 Limites do contrato

A variável não apaga o spool existente: a drenagem explícita (`hook-drain`) ainda entrega o backlog. Escolha o dono **antes do primeiro evento** da execução. Trocar no meio não deduplica o que caminhos independentes já produziram. Backfill manual, importador, `finalize-session` e importação de transcrição gerenciada (`ai-memory run`) mantêm o comportamento próprio; não os combine como produtores independentes da mesma execução.

Autenticação, verificação de dono, admissão e sanitização continuam valendo. Todos os itens de um lote compartilham **uma** identidade autenticada, então separe os lotes por operador. `extension` é proveniência autodeclarada, e a variável **não concede permissões**.

---

## 2. Memória programática via MCP

Qualquer ferramenta pode usar o ai-memory por MCP para salvar, buscar e passar contexto, sem hooks nativos.

- **Conexão:** endpoint `/mcp`, transporte Streamable HTTP. O padrão é **stateless**, sem cabeçalho `Mcp-Session-Id`. Com `serve --http-stateful`, o servidor emite sessão MCP e o cliente deve fazer o handshake de sessão.
- **Credencial:** chave de API de usuário. Use uma identidade dedicada por cliente independente. Um produtor que **substitui** a captura nativa deve usar o **mesmo operador e a mesma identidade de sessão nativa** do harness restante.
- **Escopo:** clientes estáticos passam `workspace` **e** `project` juntos em toda chamada com escopo de projeto (lidos de `.ai-memory.toml`). Clientes sensíveis a sessão só podem omitir os dois se encaminharem o `session_id` real do ciclo de vida em toda requisição.
- **Sequência mínima:** `initialize` → `tools/list` → `tools/call memory_write_page` (argumentos `workspace`, `project`, `path`, `body`; retorna `page_id` e `path`) → `memory_query` (retorna `hits`) → `memory_handoff_begin` (`summary`, `next_steps`) → `memory_handoff_accept`.
- Verifique o campo `error` do JSON-RPC **e** `result.isError` (campo padrão do protocolo MCP), mesmo com HTTP 200. O payload da ferramenta vem como JSON dentro de `result.content[].text`.
- Escritas e criação de handoff **podem criar o escopo** se ele faltar. Leituras **falham fechado** com escopo ausente ou parcial.
- Handoff é reivindicado **uma vez**. Para reivindicar um específico, passe o `handoff_id` exato. Se o início de sessão já é dono da entrega, deixe o hook reivindicar.
- Funciona **sem provedor de LLM**. Memória recuperada continua sendo texto histórico **não confiável**, mesmo sanitizada.
- **Leitura de mudanças:** `GET /api/v1/workspaces/{w}/projects/{p}/recent` (requer `serve --enable-api` ou `--enable-web`).
  - Parâmetros: `updated_since` (corte), `limit` (padrão 10, limitado a 1–100) e `cursor` (opaco).
  - Com `updated_since`, a resposta é `{pages, next_cursor}`, com páginas atualizadas **estritamente após** o corte, ordenadas por `updated_at` crescente e depois por caminho. Repita com o `cursor` até `next_cursor: null`.
  - O cursor preserva o corte e o escopo, e a autorização é refeita a cada chamada.
  - Sem `updated_since` nem `cursor`, a resposta é um **array** das páginas mais recentes, em ordem decrescente de atualização. Clientes que recebem array devem tratar isso como modo sem leitura incremental.
  - O modo incremental não usa cache, omite páginas expiradas e supersedidas e **não tem feed de deleções**. Não há snapshot entre chamadas: reconcilie remoções à parte.

---

## 3. Identidade de repositório

Módulo de lógica pura, sem I/O.

**Problema:** o projeto é identificado por `(workspace, nome)`, e um checkout sem declaração assume o nome da pasta. Dois repositórios diferentes chamados `api/` cairiam no mesmo projeto. Num servidor multiusuário com permissões por projeto, isso é um problema de **acesso**, não só de organização. Identificar pelo caminho local, por outro lado, dividiria o mesmo repositório em projetos diferentes em cada máquina.

### 3.1 Cadeia de resolução (o primeiro degrau que produzir um valor vence)

| # | Degrau (origem da identidade) | Origem | Único globalmente? | Roteia por identidade? |
|---|---|---|---|---|
| 1 | `explicit` | `identity = "…"` no `.ai-memory.toml` | sim (como o autor quis) | **sim** |
| 2 | `manifest` | `project = "…"` no `.ai-memory.toml` | por acordo | não (roteia por nome) |
| 3 | `git_remote` | remoto `upstream`, senão `origin`, normalizado | sim | **sim** |
| 4 | `folder_name` | nome-base do diretório | **não** | não |

- Declaração vence inferência. Um fork que é produto próprio tem `upstream` apontando para o projeto pai, mas o marcador diz o que ele é.
- `upstream` vence `origin` porque, em fluxo de fork, o `origin` de cada contribuidor é o próprio fork.
- A cadeia tenta `upstream` e depois `origin`, e usa o primeiro que normaliza para uma identidade válida. Um remoto que é **caminho local** (ou que não normaliza) não interrompe a cadeia; ela segue para o próximo remoto e, se nenhum servir, para o degrau seguinte. A cadeia **não escolhe outro remoto** além desses dois, porque nomes de remoto são pessoais: escolher outro geraria identidades diferentes por pessoa.
- A identidade é sempre **minúscula**. Identidades aceitas do cliente têm limite de **512 bytes**, para que um cliente não ocupe um índice único com um valor enorme. Esse limite é verificado na aceitação de dados recebidos pelo servidor (ver 3.3); a resolução local não o aplica.

### 3.2 Normalização de remoto

Todos estes formatos produzem `github.com/acme/api`:

- `git@github.com:Acme/API.git`
- `https://github.com/acme/api/`
- `https://user@github.com:443/Acme/API`
- `ssh://git@github.com:22/Acme/API.git`
- `git://github.com/acme/api`

Regras:

1. Separa o esquema (se houver).
2. `file://` ⇒ sem identidade (nenhum valor).
3. Remove credenciais até o **último** `@` que precede o caminho. Assim, senhas que contêm `@` não vazam fragmentos para a identidade.
4. Na forma de URL (com esquema), o resultado é `host[:porta]/caminho`, sem a porta.
5. Sem esquema: se `:` aparece **antes** de qualquer `/`, a forma é scp-like (`host:caminho`). Caso contrário, é caminho de arquivo ⇒ sem identidade.
6. Host de um caractere (letra de drive) ou caminho com barra invertida ⇒ sem identidade.
7. Identidade sem `/` (apenas um host) ⇒ sem identidade.
8. Normalização final: minúsculas, remove `.git` final, remove barras finais e colapsa barras duplicadas.

Caminhos locais (`/srv/git/api.git`, `../irmao`, `C:\repos\api`) não geram identidade, e a cadeia segue.

### 3.3 Onde roda e como se mantém coerente

A normalização roda no **cliente**, que é o único que vê o checkout. Assim, um remoto com credenciais embutidas nunca sai da máquina.

O servidor **não** reexecuta a cadeia, porque nunca vê o checkout. Ao receber uma identidade, ele valida só a **forma**: minúscula, sem espaços nas bordas, até 512 bytes, sem caracteres de controle, e para remotos no formato `host/caminho` sem segmento vazio nem espaços internos. Ele aceita só os degraus que roteiam por identidade (`explicit` e `git_remote`). Os demais são ignorados, e a captura roteia pelo nome, exatamente como um cliente que não enviou nada.

Os quatro clientes que implementam a mesma lógica são: o núcleo, os scripts de hook de linha de comando para sistemas POSIX, o hook para Windows e os plugins gerados. Todos são verificados contra um arquivo compartilhado de casos de teste, que contém os casos de normalização e a gramática limitada de aliases. **Qualquer caso novo deve ser verificado nos quatro.**

### 3.4 Aliases e estilo da chave

- **Aliases do marcador**: nomes antigos do projeto, em no máximo **16** aliases de até **128** bytes cada.
  - Cada alias é aparado. Deve começar com letra ou dígito, e depois aceita letras, dígitos, `.`, `_` e `-`.
  - Duplicatas dentro da lista são removidas, mantendo a primeira ocorrência. A contagem de limite é feita antes da remoção.
  - Gramática portável **mínima**: uma única declaração `aliases = [...]`, na raiz do marcador, numa linha, com um array JSON de strings. Não são aceitos escapes, tabelas, arrays multilinha, vírgula final, comentários nem uma segunda declaração.
  - Erros possíveis: `too many marker aliases` (mais de 16 entradas), `invalid marker alias` (alias fora da gramática ou mais longo que 128) e `invalid marker aliases wire value` (formato que não é o array esperado).
- **Estilo da chave**:
  - `host_path` (padrão e formato legado no fio): `github.com/acme/api`.
  - `path`: `acme/api`.
  - Só a identidade de `git_remote` tem host a remover. Declaração, nome de manifesto e nome de pasta passam inalterados.
  - Se o nome `path` já pertence a outra identidade completa (duas forjas com o mesmo caminho), o armazenamento usa o nome com host.

---

## 4. Reconhecimento do ledger de eventos (`log.md`)

O **ledger bruto** de eventos de hook fica em `log.md` ou `log-AAAA-MM.md` dentro da wiki. **O nome sozinho nunca basta**: uma página comum pode se chamar `log-2026-09.md`.

Regras de reconhecimento (lógica pura, compartilhada por todas as superfícies que precisam decidir se um arquivo é ledger):

- **Forma do nome:** `log.md`, ou `log-` + 4 dígitos + `-` + 2 dígitos + `.md`. Só a forma é verificada: o mês não é validado quanto ao intervalo.
- **Conteúdo:** o corpo só é ledger se **abrir com uma entrada de log**, no formato `## [timestamp] …`. Pode haver um bloco de frontmatter YAML antes, porque a migração para o formato de bundle OKF carimba `type: Note` em todo arquivo `.md`, inclusive nos ledgers.
  - Frontmatter sem fechamento dentro de **64** linhas ⇒ **não** é ledger.
  - Corpo que termina antes da linha que decide ⇒ **não** é ledger.
  - Linha vazia no início (antes de qualquer conteúdo) decide **contra** o ledger.
  - Na dúvida, a resposta é "não". Essa é a direção segura para quem decide o que apagar.
- **Leitura necessária:** basta um prefixo de **8 KiB** do arquivo. Isso importa porque um armazenamento pode ter dezenas de GB de ledgers, e o reconhecimento não deve ler tudo.
- **Por que uma regra única:** o observador de arquivos, a migração OKF e a exportação de bundle têm o *arquivo*, enquanto a limpeza de versões de ledger do armazenamento já tem o conteúdo da página em memória e não pode reler do disco. Usar **uma só definição, em dois pontos de entrada**, impede que "isto é ledger" divirja entre a superfície que indexa ledgers e a que os apaga. Sem isso, um append de hook poderia supersedir a linha de uma página real, ou uma limpeza de ledgers poderia atingir a cadeia de versões de uma página real.

---

## 5. Backups

### 5.1 Dados do servidor (camadas)

| Diretório ou arquivo | Backup? | Motivo |
|---|---|---|
| `wiki/` | **Sempre** | Markdown, fonte da verdade; já é repositório git |
| `raw/` | a confirmar | Contém dados brutos de workstreams gerenciados (a confirmar se há ledger de observações neste diretório) |
| `db/` | **Não** (via git) | Índice do banco **derivado**; reconstruível com `ai-memory reindex`. Commitar arquivos de escrita do banco em git desperdiça espaço |
| `logs/`, `models/`, `.serve.lock` | Não | Logs; pesos baixáveis com checksum fixado; arquivo de trava do servidor |
| `config.toml` | Sim, **sem segredos** | Arquivos `*.env`, `auth.json` e bundles de CA privada **nunca** devem ir para um espelho |

Sessões, observações, handoffs, usuários, tokens, auditoria e embeddings vivem **no banco de dados** e não voltam com `reindex`:

- `reindex` reconstrói o banco a partir da wiki e não recria o estado episódico (sessões, observações, handoffs e contadores de decaimento). Embeddings são recalculados depois, com um comando separado.
- Para usuários, tokens e auditoria, a não reconstrução é **a confirmar**.
- Para restaurar esses dados, use o backup do servidor (`ai-memory backup --to <tarball>`) e a restauração (`ai-memory restore --from <tarball> --data-dir <caminho> --force`).
- O backup do servidor é um snapshot online do banco, com a wiki e o `config.toml` (se existir), em um arquivo tar.gz. Ele é feito pelo servidor em execução. O `config.toml` entra **sem filtro** de segredos: se ele contiver credenciais, trate o tarball como sensível.
- Combine: espelho git (diário) para versionamento, e tarball para recuperação rápida, sem reindex.

**Padrões:**

1. *Só a wiki:* `git remote add origin … && git push` dentro do diretório `wiki/`. Execute apenas comandos de leitura e push, com o servidor rodando. **Nada** de rebase, merge ou force-push: o servidor escreve nesse repositório.
2. *Instalação completa:* sincronizar o diretório de dados (com exclusões de `models/`, `db/`, `logs/`, `.serve.lock` e padrões de segredo `*.env`, `auth.json`, `.secrets`, `*.pem`, `*.key`, `*.crt`) e o diretório de configuração para um repositório espelho → `git add -A` e commit, se houver mudança → `git push` com timeout → arquivo tar.gz datado → rotação de 30 dias. Agendado por um timer de usuário do sistema (com execução persistente e atraso aleatório), ou pelo agendador do sistema operacional.
3. *Conteúdo sensível demais para texto em um remoto:* arquivo cifrado (ferramentas como restic, borg, age ou gpg), criptografia de caminhos com git-crypt (configure as chaves **antes** do primeiro commit), ou backup somente local em mídia externa.

**Aviso de segurança:** a wiki contém **tudo que o agente viu**: prompts, entradas e saídas de ferramentas, corpos sintetizados. O sanitizador remove credenciais óbvias, mas não prova que todo conteúdo era seguro. Trate qualquer remoto como legível pelos administradores de quem o hospeda, inclusive repositórios "privados" corporativos. A sincronização remota é **seu** canal, não parte do produto, e a criptografia em repouso também está fora do escopo.

### 5.2 Backup e restauração de ativos de agentes (`backup-agents` / `restore-agents`)

Resolve a dificuldade de migrar de máquina sem reconfigurar cada agente à mão. São comandos **do cliente**, que funcionam com o servidor parado.

```bash
ai-memory backup-agents -o agent-assets.tar.gz [--agents claude,codex,antigravity] [--scope global|project|both] [--include-secrets] [--dry-run]
ai-memory restore-agents -i backup.tar.gz           # dry-run por padrão: mostra o plano
ai-memory restore-agents -i backup.tar.gz --apply   # aplica
```

- Escopo padrão: `both`. Os escopos são `global` e `project`.
- `--dry-run` no backup não grava o arquivo.

**Tipos:**

- Tipos de ativo: `mcp-config`, `skill`, `plugin` e `instruction`.
- Escopo do ativo: `global` (perfil do usuário) ou `project` (repositório).
- Manifesto (versão do esquema `1`): data de criação, sistema operacional, arquitetura, diretório home (opcional), indicador de sanitização e lista de entradas.
- Cada entrada tem: agente, tipo de ativo, escopo, caminho no arquivo, caminho relativo de destino, indicador de sanitização, SHA-256 (opcional) e modo (opcional).
- O modo guarda os bits de permissão do Unix, **sem** bits set-id nem sticky.
- O manifesto fica em `manifest.json` na raiz do tar.gz. Os caminhos das entradas têm a forma `<agente>/…` (por exemplo, `claude/skills`).

**Matriz** (onde cada harness guarda configurações MCP, skills, plugins e instruções):

- Claude Code e Claude Desktop: `~/.claude.json`, `~/.claude/settings.json`, `~/.claude/skills`, `~/.claude/plugins`, `CLAUDE.md` no projeto. O arquivo de configuração do Claude Desktop só é incluído em macOS e Windows.
- Codex: `~/.codex/config.toml`, `~/.codex/mcp.json`, `~/.codex/skills`, `~/.codex/plugins`, `~/.agents/skills`, `AGENTS.md`.
- Antigravity e Gemini CLI: configurações MCP em `~/.gemini/…`, `GEMINI.md`, e skills em `.gemini/skills`. Presença de um diretório `rules/` como fonte: **a confirmar**.
- Cursor: `~/.cursor/mcp.json`, `.cursorrules`, `.cursor/rules/`, `.cursor/mcp.json` no projeto.
- OpenCode: `opencode.json` ou `opencode.jsonc` (projeto e global), `~/.config/opencode/skills` e `~/.config/opencode/plugins`.
- Outros agentes com caminhos próprios: Devin, Grok, Kiro, OpenClaw, Command Code e Kimi Code. A inclusão do VS Code Copilot **não** aparece na varredura de backup (a confirmar se deveria).
- Claude Desktop e OpenCode usam o **mesmo resolvedor de caminhos por plataforma** do comando de instalação de MCP. Isso inclui instalações empacotadas do Windows. No Windows, o OpenCode também usa o diretório de configuração do usuário (`%USERPROFILE%\.config\opencode`).

**Invariantes de segurança:**

1. *Redação:* por padrão, tokens, credenciais de tipo bearer e chaves de ambiente (variáveis com sufixo `_TOKEN`, `_KEY`, `_SECRET`, `_PASSWORD` e similares) em configurações MCP de texto UTF-8 são sanitizados. **Skills, plugins e instruções são copiados sem alteração** e podem conter segredos. `--include-secrets` desliga a redação das configurações MCP. O arquivo gerado sai com modo `0600` no Unix, independentemente do modo das entradas.
2. *Links simbólicos:* na criação, nunca segue link simbólico externo. Na restauração, recusa link simbólico no destino ou em qualquer diretório intermediário controlado pelo arquivo, abaixo da raiz aprovada.
3. *Travessia de caminho:* só são aceitos destinos previstos para cada agente. Entradas do arquivo devem ser caminhos UTF-8 regulares, sem componentes `..` ou absolutos. Número de entradas, tamanho do manifesto, tamanho por arquivo e tamanho total expandido são limitados **antes** de preparar qualquer conteúdo:
   - até 20 000 entradas;
   - manifesto de até 4 MiB;
   - até 128 MiB por arquivo;
   - até 512 MiB no total expandido.
   Cada entrada do manifesto precisa ter exatamente um corpo e um SHA-256 bem formado. Duplicatas, corpos ausentes ou sem manifesto e divergência de checksum são recusados. O SHA-256 detecta corrupção, mas **não autentica o autor**.
4. *Atomicidade:* o arquivo de saída é completado e sincronizado em um arquivo temporário irmão antes de substituir o destino. Os destinos são gravados com arquivo temporário, renomeação e sincronização. Se uma falha ocorre no fim da restauração, as escritas anteriores são desfeitas, e cópias datadas (`.bak-<carimbo>`) dos arquivos sobrescritos ficam disponíveis. Arquivos personalizados não gerenciados **não** são sobrescritos sem `--force`.
5. *Conteúdo ativo:* skills, plugins e instruções podem executar código ou instruir o modelo. Por isso, a restauração é dry-run por padrão e avisa. Aplique só a partir de um arquivo de origem confiável.

**Testes existentes:** ida e volta (backup e restauração), redação de segredos por padrão, recusa de travessia de caminho (por exemplo, `../../etc/passwd`), recusa de destino não autorizado, verificação de SHA-256, recusa de link simbólico no destino e no pai, recusa de entradas acima do limite de tamanho, e preservação de backup do arquivo anterior com `--force`.

---

## 6. Handoff: oferecer, não reivindicar

**Problema:** um handoff pendente é reivindicado pela **próxima** sessão que começar no projeto, qualquer que seja o harness, o propósito, e mesmo que seja não interativa. A reivindicação é de uso único e irreversível. Na prática, o bastão raramente chega à sessão pretendida: uma sessão irrelevante o consome; continuar em outro harness o perde; e lançamentos não interativos (execuções em lote, sondagens, `ai-memory run`) também reivindicam.

**Comportamento atual (padrão):**

- O bastão é buscado no início de sessão. A busca usa o dono (operador autenticado) como filtro: handoffs de outro operador não são candidatos. Handoffs sem dono ficam visíveis para todos, o que preserva o comportamento de operador único.
- A reivindicação acontece **na mesma passagem** da cadeia de admissão. Webhooks veem a reivindicação. Se a admissão for recusada ou expirar, só a reivindicação é cancelada: o conteúdo ainda é entregue.
- Seleção do candidato:
  1. Verificação de dono **antes** de qualquer atalho para handoff manual.
  2. Handoff **manual** (sem sessão de origem) é de projeto inteiro e sempre vence um automático.
  3. Handoff **automático** (criado no fim de sessão) é limitado por fronteira de caminho do cwd.
  4. Entre os elegíveis, vence o mais novo. Em empate de tempo, vence o automático com cwd mais específico.
- Início de sessão e bastão de sessão ainda aberta: um bastão cuja sessão de origem ainda está aberta só entra na seleção depois de **10 minutos de silêncio** da sessão (`LIVE_BATON_QUIET_PERIOD`). Isso evita entregar a outra sessão em uso o contexto de uma conversa diferente. Sessões encerradas e handoffs manuais não são afetados.
- Handoffs automáticos expiram sozinhos: ao criar um novo no fim de sessão, os abertos do mesmo cwd cuja sessão já encerrou são expirados; há também varredura de supersedidos depois de uma reivindicação. Handoffs manuais ficam abertos até serem aceitos ou cancelados.
- Invariante: um handoff é reivindicado **exatamente uma vez**. A escrita só altera linhas em estado `aberto`, e a condição de estado é verificada novamente na transação.

**Decisão aceita (implementada): modo "oferta".** Nova chave de configuração:

```toml
[handoff]
claim_on_session_start = true   # padrão: comportamento inalterado
```

Com `false`, o início de sessão **não** reivindica. Em vez disso, anexa ao contexto um **aviso não consumidor**, no mesmo padrão do aviso de caixa de entrada:

```text
📬 ai-memory: a pending handoff `<id>` from `<from_agent>`, left `<age>` ago.
   To pick it up, call `memory_handoff_accept` with handoff_id `<id>`.
```

- O aviso nomeia o `handoff_id` **exato**. Aceitar "o mais recente" poderia reivindicar outro bastão que chegou depois. Com o id exato, um aviso antigo só perde a corrida.
- **Regra de segurança herdada do aviso de caixa de entrada:** só metadados (id, agente de origem, idade). **Nunca** texto controlado por mensagem ou resumo, para que um resumo hostil não injete instruções numa sessão que não pediu. O resumo completo só aparece, como conteúdo não confiável, depois de uma aceitação explícita.
- O ledger e o contexto de run gerenciado **continuam** sendo reivindicados no início da sessão. Só o slot único do handoff vira oferta. Assim, um run gerenciado não perde seu pacote de continuidade. O briefing e o conteúdo de run gerenciado não mudam.
- A chave é global do servidor: vale para todo operador de um servidor compartilhado. Um ajuste por projeto, no `.ai-memory.toml`, é um acompanhamento possível (ainda não implementado).

**Não perseguidas** (decisão de 2026-10-02):

- *Opt-out por execução* (variável de ambiente ou flag para desligar o handoff): não pode ser feito pulando a requisição de início de sessão, que também entrega o pacote de run gerenciado, o briefing e o aviso de caixa de entrada. Se voltar a importar, o formato seguro é um parâmetro do lado do servidor (`handoff=off`) que não reivindica nada, mas entrega o restante.
- *Filtro por destinatário* (`to_agent`) como critério de candidato: desnecessário, porque nada é reivindicado automaticamente. O campo existe, é sempre nulo, e a seleção nunca o lê.

---

## 7. Perfil entre projetos

*Aceito em 2026-10-04, implementado na versão 2.6.* Um registro **pequeno, curado e entre projetos** de como *este usuário* costuma trabalhar. É construído a partir de evidências e entregue a todo projeto e harness como **padrão**, quando nada mais específico diz o contrário. A lógica do perfil não faz I/O: o armazenamento resolve os escopos, o roteador de hooks entrega o digest e a CLI escreve o bloco de regras.

### 7.1 Doutrina: memória como fallback, regras como override

Precedência (o nível inferior preenche lacunas e **nunca** sobrepõe o superior):

1. instruções atuais do usuário;
2. arquivo de regras do repositório (`AGENTS.md` ou `CLAUDE.md`);
3. memória do projeto;
4. **perfil**.

O texto recuperado continua sendo **dado**. Uma entrada de perfil é uma preferência *expressa pelo usuário*: define um padrão (ferramenta, layout, convenção), mas **não autoriza** nada que as instruções atuais não autorizariam. O digest é cercado e rotulado como o briefing, e o agente deve **dizer qual padrão aplicou**.

### 7.2 Onde vive (`[profile] share`)

| Implantação | Padrão | Local |
|---|---|---|
| Usuário único (sem usuários no banco, sem proxy confiável) | ligado, `global` | `default/_global`, sob `profile/` |
| Multiusuário | **desligado**; ativado com `enabled = true` | `user`: um projeto **restrito** por operador, `_profile.<id simples do usuário>` |

Valores de `share`:

- `global`: um perfil por servidor. Em multiusuário, é um perfil de *time* e só colhe de projetos abertos.
- `workspace`: projeto reservado `_profile` do workspace.
- `user`: privado. Só lê e escreve sessões e páginas do próprio operador, e só de projetos que ele lê. A chave é o **ID imutável** do usuário, não o nome, para que reuso de nome de usuário não herde o perfil de outra pessoa.
- `off`: sem perfil.

A decisão entre instalação de operador único e multiusuário é a mesma usada pelo restante do sistema (o servidor "distingue operadores" ou não). Projetos reservados nunca recebem captura de eventos.

### 7.3 Pipeline

1. **Colheita (por projeto, sem LLM).** Acontece depois do sanitizador e das exclusões de captura, e **nunca** lê saída de ferramenta. Fontes:
   - (a) *Prompts do usuário* com forma de preferência ou correção. Frases reconhecidas (em inglês e em português): *always, never, prefer X over Y, use X instead of Y, don't use, from now on, in all my projects, by default*, e equivalentes em português (*sempre, nunca, prefiro, prefira, em vez de*…). Também contam *everywhere* e *every project*.
   - (b) *Páginas curadas* nas pastas `_rules/`, `decisions/`, `gotchas/` e `procedures/`.
   - (c) *Sinais de stack* em caminhos observados: nomes de arquivos de manifesto de gerenciadores de pacotes e de build (um por ecossistema), registrados como `applies_to`.

   Cada candidato é registrado com a tupla de identidade, a fonte (`session:<id>` ou `page:<caminho>`), a data e o operador. Projeto com `contribute = false` no marcador **nunca** é colhido.
2. **Convergência** (job serializado de escrita):
   - Um candidato vira entrada, ou atualiza uma entrada existente, se (a) o usuário o escopou explicitamente além do projeto ("em todos os meus projetos", "everywhere" e equivalentes), ou (b) a mesma escolha aparece em pelo menos `min_projects` projetos distintos (padrão 2). Um "always", "never" ou "by default" soltos **não** contam como escopo geral.
   - Conflito no mesmo escopo: o veredito mais recente **supersede** o anterior, e a cadeia de supersessão é preservada.
   - Escolha restrita a uma só stack vira entrada com escopo (`applies_to: [<stack>]`, por exemplo).
   - Agrupamento de tópicos sem LLM: normaliza o texto (minúsculas, sem palavras vazias, mantendo nomes de ferramentas e linguagens) e agrupa por sobreposição de tokens. Usa um índice invertido de tokens: um candidato só é comparado com grupos que compartilham pelo menos uma palavra de tópico. **Não** há comparação de todos os pares.
3. **Armazenamento:** páginas da wiki em `profile/<categoria>/<slug>.md`.
   - Categorias, em ordem: `stack`, `architecture`, `workflow`, `testing`, `tools`, `style`, `habits`.
   - Frontmatter: `kind: preference`, `applies_to`, `projects`, `first_seen`, `last_seen`, `confidence`, `evidence` (limitada) e, opcionalmente, `enforced_by` (regra ou hook que já impõe a preferência; isso a tira do digest).
   - Corpo: uma linha imperativa (até **240 bytes**) mais o raciocínio do usuário.
   - Toda escrita passa pela escrita de página da wiki.
   - Página colhida leva `generated_by: profile-harvest` e o SHA-256 do corpo (`generated_sha`). O colhedor **só reescreve** páginas assim, e **só se** o corpo ainda bater com o hash. Página editada à mão nunca é reescrita: o grupo é relatado como pulado.
   - Há um registro do que foi escrito. Se a página de uma linha sumiu, o usuário a apagou, e o tópico não é recriado até chegar um candidato mais novo.
4. **Entrega:**
   - *Digest de início de sessão:* seção cercada, colocada **primeiro** no conteúdo entregue (antes de handoff, contexto gerenciado, briefing e aviso de caixa de entrada, que mudam a cada sessão), para que a parte reutilizável fique estável e possa ser cacheada pelo harness.
   - Uma linha por entrada (escopo, afirmação, página), em ordem fixa de categoria e depois caminho. Nada que dependa do tempo é impresso (sem datas), para que o texto seja idêntico entre sessões enquanto o perfil não mudar.
   - Entradas já impostas em outro lugar (`enforced_by`) são omitidas. Entradas com escopo de stack só aparecem se o projeto mostrar essa stack, ou se nenhuma stack for conhecida ainda (projeto novo recebe a base inteira).
   - Limite de bytes: `digest_max_bytes` (padrão 3 000; intervalo aceito de 1 500 a 12 000). Entradas que não cabem são contadas, e o texto indica a dica de consulta.
   - *Baseline de projeto novo* (sem páginas): usa `baseline_max_bytes` (padrão 6 000; intervalo de 2 000 a 20 000) e sugere `ai-memory profile apply`.
   - *União na consulta de memória:* a busca já une o perfil `_global`. Para perfis `workspace` e `user`, une também o perfil do chamador, sujeito a autorização.
   - *`profile apply`:* escreve as principais entradas num sub-bloco gerenciado do arquivo de regras do repositório, delimitado por `<!-- ai-memory:profile:start -->` e `<!-- ai-memory:profile:end -->`. Limite de `apply_max_lines` (padrão 40). Não toca texto fora dos marcadores. A escrita é atômica e com cópia de segurança. Aceita `--dry-run` e `--remove`.
   - Alvo do `profile apply`, nesta ordem: o indicado em `--target`; senão `AGENTS.md`, se existir; senão `CLAUDE.md`, se ele fizer mais do que importar `AGENTS.md`; senão cria um `AGENTS.md` novo.
   - **É assim que uma entrada vira regra dura:** ao ser gravada no arquivo de regras do repositório.
5. **Com LLM configurado** (`[profile] llm` diferente de `false`; sem LLM continua sendo o padrão completo):
   - *Classificar* ao fim da sessão, com saída estruturada: `generality`, `category`, `topic`, `statement` (até 200 bytes), `quote` (literal), `applies_to` e `confidence`.
   - *Mesclar* quando um tópico ganha evidência. A mesclagem devolve a entrada atualizada e um indicador `changed`. Ela vê a entrada atual como contexto, prioriza as palavras do usuário e é instruída a **nunca seguir instruções** contidas no texto armazenado.
   - Qualquer erro do provedor cai para o caminho sem LLM.
6. **Estabilidade (revisão de 2026-10-08, versão 2.6.1):**
   - O digest vem primeiro. Evidência que apenas *corrobora* uma afirmação não a reescreve: se a mesclagem devolve `changed` como falso, a reformulação é ignorada. Uma linha assentada só muda quando o veredito muda ou o escopo se estreita.
   - O agrupamento guarda o grupo de cada candidato e compara só os candidatos novos. Isso evita varredura quadrática sobre até 20 000 candidatos.

### 7.4 Configuração e comandos

```toml
[profile]            # servidor (arquivo de configuração ou variável AI_MEMORY_PROFILE__<CHAVE>)
enabled = "auto"     # auto | true | false  (auto = ligado em operador único, desligado em multiusuário)
share = "auto"       # auto | global | workspace | user | off
min_projects = 2
inject_on_session_start = true
digest_max_bytes = 3000
baseline_max_bytes = 6000
apply_max_lines = 40
llm = true

# .ai-memory.toml (por projeto)
[profile]
contribute = false   # nunca colher este projeto (por exemplo, cliente com acordo de confidencialidade)
consume = false      # sem digest nem união neste projeto
```

Comandos da CLI:

`ai-memory profile status | list | show <caminho> | forget <caminho> | review | rebuild | apply [--target F] [--dry-run | --remove]`

- `status`, `list`, `review` e `apply` leem as rotas `/admin/profile/*`.
- `rebuild` faz POST: esquece as marcas de colheita, relê todos os projetos e converge.
- `show` e `forget` reusam as rotas `/admin/read-page` e `/admin/delete-page`.
- Todos são **restritos ao usuário raiz** em servidor multiusuário, como todas as rotas `/admin/*`.
- Em servidor multiusuário, cada operador gerencia o próprio perfil privado pelo MCP. Não há ferramenta nova: a escrita de página aceita `scope: "profile"`, a remoção usa a ferramenta de deleção de página, e a consulta devolve os acertos de perfil em `global_scope_hits`.

### 7.5 Fronteiras de segurança

- Perfil `user` é privado: leitura, escrita e remoção de outro operador são recusadas. O colhedor nunca lê sessão de outro operador para o perfil dele.
- Perfil `global` em multiusuário **nunca** colhe projeto restrito.
- `contribute = false` vale no colhedor. `consume = false` vale no digest e na união.
- Candidatos vêm só de prompts e de páginas curadas, **depois** do sanitizador.
- Digest e prompts de LLM tratam o texto armazenado como dado.
- Escopos compartilhados (`_global` e `_profile` do workspace) são de leitura aberta e de escrita controlada (usuário raiz ou permissão `write`).
- Cada fronteira tem teste adversarial e uma linha no documento de [fronteiras de segurança](security-boundaries.md).
- **Fora de escopo:** editar o arquivo de regras automaticamente (só com `profile apply`), suporte universal a todos os harnesses, e promover conhecimento sem evidência de mais de um projeto ou sem uma afirmação geral explícita do usuário.

---

## 8. `--yolo` e isolamento (ai-jail)

`ai-memory run … --yolo` traduz a flag para o equivalente perigoso de cada harness. Para o Claude, é `--dangerously-skip-permissions`.

**Aceito (versão 2.5):** tudo fica condicionado a `--yolo`, exceto o `--jail` explícito. Em caminho **não interativo** (hook, CI, processo destacado ou pipe), **nunca** há pergunta.

1. **Aviso (todos os sistemas):** em terminal real (entrada **e** saída de erro são terminais) e fora de um isolamento ai-jail, o aviso é: "`--yolo` runs every tool call with no confirmation. An agent can delete files, run any command, and reach the network unsupervised. Proceed? [Y/n]". Enter confirma. `n` ou `no` aborta antes de lançar o agente, com saída diferente de zero. Qualquer outra resposta também confirma.
2. **Oferta de ai-jail (Linux e macOS):** só é feita se o ai-jail estiver utilizável:
   - o binário `ai-jail` existe no `PATH`, ou em `~/.local/bin/ai-jail` como alternativa;
   - existe um backend de sandbox candidato: `bwrap` (inclusive o caminho dado pela variável `BWRAP_BIN`) no Linux, ou `sandbox-exec` no macOS;
   - uma execução de teste `ai-jail --dry-run`, sem executar nada, passa (isso confirma a confiança no backend e na configuração local).

   Nunca é oferecido no Windows. Se o usuário aceitar, o ai-memory **relança a si mesmo** com os mesmos argumentos, dentro de `ai-jail --network --agent-state --no-save-config --env AI_MEMORY_SERVER_URL --env … -- <executável> run <harness> … --yolo`:
   - `--` separa as flags do ai-jail das flags do comando, para evitar colisão de nomes (como `--env`).
   - `--network` (e não `--allow-host`) compartilha a rede do host, para que o servidor em `127.0.0.1:49374` continue alcançável. A proteção é "macia": a saída de rede fica livre, porque o agente precisa de rede para a API do modelo de qualquer forma.
   - O ai-jail **limpa o ambiente** por padrão. Por isso, só são repassadas as variáveis efetivamente definidas, com `--env NOME`, de uma lista fixa: URL do servidor e do hook, diretório de dados e token de autenticação do ai-memory, diretório de configuração do Claude, e chaves de provedores de modelo (Anthropic, OpenAI, Google, Gemini, OpenRouter, GitHub Copilot).
   - `--no-save-config` impede que o ai-jail grave flags transitórias no arquivo `.ai-jail` do repositório. Sem isso, o checklist sumiria e as credenciais deixariam de ser montadas nas próximas execuções.
   - `--agent-state` persiste o login do harness entre execuções.
3. **Já dentro do ai-jail (sem aninhar):** não há sentinela dedicada. No Linux, compara-se o hostname (`ai-sandbox`, em um namespace de UTS). No macOS, usa-se a variável `PS1` começando com `(jail) `. É um sinal fraco, mas só **suprime** o aviso: um falso negativo apenas mostra o aviso de novo. No Windows, a resposta é sempre "não estou dentro".
4. **"Yolo verdadeiro" (opt-in, só Claude):** a chave `[run] claude_true_yolo`, a variável de ambiente `AI_MEMORY_CLAUDE_TRUE_YOLO=true` ou a flag `--true-yolo` (é um **superconjunto** de `--yolo`) acrescenta `--settings` com `{"permissions":{"defaultMode":"bypassPermissions"}}`. A flag de linha de comando vence as configurações de usuário ou de projeto.
   - **Não** silencia regras `ask` explícitas, porque as permissões de `--settings` *se somam* às de outros escopos. Também **não** desliga as checagens internas de segurança de comando do Claude Code.
   - Não define variáveis de ambiente de desativação de checagens.
   - A chave de configuração só "promove" um lançamento que já é `--yolo`.
   - Recomendado somente dentro de um ai-jail.
5. **`--jail` e `--no-jail`, e o checklist:**
   - `--jail` relança agora com os padrões inteligentes.
   - `--jail=github,aws,no-mise`, `--jail=all` e `--jail=none`: a lista é aplicada da esquerda para a direita, e as linhas visíveis que não foram nomeadas ficam **desligadas à força**.
   - `--no-jail` nunca relança e conflita com `--jail`.
   - `--jail` exige `=` para a lista, para não consumir o nome do harness como se fosse valor.
   - Não há flags soltas como `--github`, para não colidir com flags nativas, como `--worktree` do Claude.
   - Credenciais (GitHub CLI, AWS, Kubernetes, gcloud, configuração do Docker) vêm marcadas **se existirem**.
   - `ssh` só é marcado quando o `push` do `origin` usa SSH (lido da URL de push).
   - `worktree` é marcado em worktree vinculado.
   - `docker` (que concede root do host), `gpu`, `display`, `pictures` e `tailscale` ficam desligados por padrão.
   - `audio`, `x11`, `host-shm`, `terminal-passthrough`, `update-check`, `mise` e `toolchains` só são ativados com `--jail=`.
   - **Seleção explícita é exata:** o checklist passa cada linha visível como `--X` ou `--no-X`, e as flags de linha de comando vencem a configuração do ai-jail.
   - Um arquivo `.ai-jail` do projeto **substitui** o checklist. Ele não é confiável, porque vive no repositório do agente. Só a **presença** do arquivo é verificada; o conteúdo nunca é lido.

---

## 9. Captura em aplicativos desktop

Registro de pesquisa para a proposta de captura em aplicativos desktop (verificação de 2026-10). A regra que vale: tudo que entra no armazenamento passa pelo sanitizador e chega pelo endpoint `/hook` ou `/hook/batch`. **Nunca** há escrita direta na wiki ou no banco.

Linhas sobre produtos de terceiros são registros de pesquisa. Onde o código do ai-memory confirma o mecanismo, isso está indicado; o restante é **a confirmar**.

| Superfície | Mecanismo de captura | Esforço |
|---|---|---|
| Claude Desktop, aba **Code** (local) | Capturada por hooks do Claude Code. Sessão sem pasta usa um cwd de *scratch workspace* (projeto `scratch`). Os hooks virem de `~/.claude/settings.json`: **a confirmar** | 0 (a confirmar) |
| Claude Desktop, **Cowork** | Não verificada (roda em VM) | verificar |
| Codex desktop | Mesma configuração de host do CLI (`~/.codex/hooks.json`); hooks confiáveis disparados pelo servidor de aplicativo do Codex. **A confirmar** | verificar 1 prompt |
| Antigravity (IDE, versão 2.0 e CLI) | Hooks nativos da CLI são suportados. O payload usa `conversationId`, `transcriptPath`, `workspacePaths[]` e `toolCall`. Mapeamento: agente `antigravity`, sessão = `conversationId`, cwd = `workspacePaths[0]` (mapeamento **a confirmar**). O hook do IDE: **a confirmar** | polimento de adaptador |
| Zed (agentes ACP e de terminal) | Hooks do próprio agente; o Zed é transparente. **A confirmar** | 0 |
| Cursor desktop | Hooks suportados (há diretório de hooks para Cursor). Execução dos hooks de `~/.claude/settings.json` por padrão: **a confirmar** | 0 (a confirmar) |
| Claude **Chat**, ChatGPT hospedado, Grok (bot) | Sem hooks nem armazenamento local. Só há chamada de ferramenta MCP, por decisão do modelo, ou uma ferramenta de captura explícita com escopo fixo (`personal/desktop-chat`) | proposta fase 1 |

O servidor MCP vê, em qualquer aplicativo, **só chamadas de ferramenta**: nome, argumentos, resultados e timestamps. Isso basta para telemetria de uso e sinais de eficácia de recall, mas **não** para capturar a conversa.

Observadores de armazenamento local (transcrições de sessão em arquivos `.jsonl` de cada harness) seguem o padrão do importador, via `/hook/batch`, e ficam **fora do núcleo**. Não construa sobre o banco interno do Zed, que é instável.

Deduplicação entre CLI e Desktop: **não é necessária** para o mesmo `session_id` retomado. Só a sessão desktop **sem pasta** vira sessão nova sob um cwd efêmero, e a escolha (um projeto único `desktop/claude-scratch`, ou um por scratch com tag) fica em aberto. **A confirmar** a afirmação de que o `session_id` é mantido ao retomar.

Plano em fases (nada agendado):

0. Documentação e verificações.
1. Paridade de captura do Antigravity IDE.
2. Polimento de escopo e deduplicação.
3. Ferramenta explícita para superfícies sem hook. Afeta a lista de ferramentas MCP e os testes de superfície de prompt.
4. Instalador com ícone na bandeja do sistema, para não desenvolvedores. Regras de coexistência: detectar antes de lançar o servidor.

---

## 10. Adaptador de reranker externo

Com `AI_MEMORY_RERANKER=llm`, o servidor reordena os candidatos de `memory_query` usando o provedor de chat configurado.

- O reranker pede uma nota graduada para até **30** candidatos, com trechos de até 600 bytes cada. Com um modelo de raciocínio hospedado, isso pode levar dezenas de segundos.
- O servidor tem **timeout de 20 s** para o reranker. Se estoura, a consulta volta à ordem original. Na prática, a funcionalidade fica sem efeito.
- Falha, resposta malformada ou resposta parcial: a consulta **mantém a ordem original** ("sem rerank", nunca "sem busca").

O prompt do reranker é uma **rubrica** com quatro níveis: 1.0 (responde diretamente), 0.7 (mesmo tema, contexto útil), 0.3 (tangencial) e 0.0 (irrelevante). Um juiz que pontua por rubrica fixa responde sem decodificação autorregressiva longa.

**Adaptador (a confirmar):** a ideia é um pequeno serviço intermediário, sem dependências externas, entre o ai-memory e o provedor. Ele divide o tráfego pela forma do pedido:

1. Pedido cujo *system prompt* começa com a frase fixa `You are a retrieval reranker for a software project's memory wiki.` é do reranker.
2. A mensagem de usuário com `{query, candidates:[{candidate,title,text}]}` (índices a partir de 1) vira **uma** chamada em lote ao endpoint `/v1/systemone` do juiz. O juiz devolve a relevância pelo índice da rubrica (0.0, 0.3, 0.7 ou 1.0), e o adaptador responde `{"scores":[{"candidate":n,"relevance":f}]}` como conteúdo de chat, no formato que o analisador tolerante do reranker espera.
3. **Todo o resto** (consolidação, lint, bootstrap, chats simples) é repassado sem alteração ao provedor real, com o cabeçalho `Authorization` encaminhado. O adaptador não guarda segredos.
4. Falha do juiz ⇒ resposta HTTP 500 ⇒ o servidor mantém a ordem própria.

Configuração por ambiente do adaptador: `JEV_URL`, `JEV_MODEL`, `UPSTREAM` e `LISTEN`, com padrões em loopback. **A confirmar:** o código do adaptador não está nos fontes revisados; apenas a forma do prompt e o comportamento do lado do servidor foram conferidos.

Para apontar o provedor para o adaptador:

```text
AI_MEMORY_LLM_PROVIDER=openai-compat
AI_MEMORY_LLM_BASE_URL=http://127.0.0.1:18097/v1
AI_MEMORY_RERANKER=llm
```

**Variante "choice-contrastiva":** em vez de uma nota por candidato, faz **uma** pergunta de escolha sobre a lista inteira e mapeia a probabilidade de escolha para a relevância. É seguro porque o consumidor só **ordena**: o servidor ordena por relevância e nunca usa o valor absoluto. Medidas de uma implantação (102 consultas), **a confirmar** por não estarem no código:

- sem reranker: hit@1 de 0,495 e NDCG@10 de 0,739;
- modelos hospedados: hit@1 de 0,77–0,78 e NDCG@10 de 0,87–0,875, com 14–20 s de latência;
- juiz pelo adaptador: hit@1 de 0,778 e NDCG@10 de 0,873, com 0,205 s de latência (cerca de 100 vezes mais rápido);
- variante *choice* no mesmo backend: ganho de 14 a 21 pontos de hit@1 sobre a rubrica, com latência também menor.

Use a variante *choice* quando o reranker é o único consumidor. Mantenha a rubrica se algo a jusante lê graus absolutos (limiar, fusão ou heurísticas de log).

**Ressalvas:**

- A detecção depende do prefixo exato do prompt. Se ele mudar, o que quebra primeiro é a **detecção**: o tráfego volta ao modelo hospedado, o que não é uma queda, mas perde a vantagem de velocidade.
- O log do servidor ainda atribui o rerank ao modelo configurado.
- Rode o adaptador em loopback ou em rede privada confiável, porque ele encaminha `Authorization` sem autenticação própria.

---

## 11. Como isso entra numa reimplementação

| Item | Prioridade para um primeiro corte | Observação |
|---|---|---|
| Identidade de repositório (§3) | **Alta** se houver mais de um repositório com nome genérico ou multiusuário | pequena, sem I/O; use os casos de normalização do arquivo compartilhado de testes |
| Reconhecimento de ledger (§4) | Alta se o ledger ficar dentro da wiki | evita apagar ou supersedir página real |
| Backup de dados (§5.1) | **Alta** | wiki em git privado mais tarball; trate o remoto como legível por administradores |
| Captura externa (§1) | Só se houver orquestrador | veja o relay de referência |
| Handoff em modo oferta (§6) | Média | comece com `claim_on_session_start = false` se agentes disparam sessões não interativas |
| Perfil entre projetos (§7) | Baixa (fase posterior) | depende da wiki, dos hooks e, idealmente, de LLM |
| Backup de ativos de agentes (§5.2) | Baixa | conveniência para troca de máquina |
| `--yolo` e isolamento (§8) | Baixa, ou só se usar `ai-memory run` | |
| Desktop (§9) e reranker externo (§10) | Opcional | |
