# Integração de frontends com a API `/api/v1`

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

> Superfície JSON somente leitura e modelo de hospedagem de interface própria
> (SPA) para frontends de terceiros que consomem um servidor `ai-memory`.

Este documento descreve o comportamento observável: métodos, caminhos,
parâmetros, formatos de resposta, códigos de erro, constantes e regras de
ordenação. Onde uma regra depende de configuração, a configuração é indicada.

## 1. Escopo

| | O que é possível | O que não é possível |
|---|---|---|
| `/api/v1/*` | Navegar por workspaces, projetos e páginas; ler a página completa (markdown, frontmatter, links e backlinks); buscar por texto completo (global, de um projeto ou de vários projetos); obter snapshots agregados ("overview"); listar páginas recentes e atualizações incrementais; listar handoffs; listar as sessões de um projeto e ler as observações brutas de uma sessão; ler o histórico de versões de uma página com diff (4.14); listar arestas entre projetos. | Escrever, apagar, renomear, rodar lint, consolidar, executar varreduras ou alterar handoffs. Esta superfície não possui nenhuma operação de escrita. Escritas passam pelas rotas `/admin/*` (usadas pela CLI) ou pelas ferramentas MCP. |
| Hospedagem de interface própria (`--web-ui-dir`) | Servir uma SPA em `/web` (ou no slug configurado), com mesma origem da API. Sem essa opção, o navegador HTML embutido continua disponível. | Hospedar a SPA em outra origem sem proxy reverso. Use mesma origem ou configure CORS (seção 9). |

A API `/api/v1` só é montada quando `--enable-web` (variável
`AI_MEMORY_ENABLE_WEB`) ou `--enable-api` (variável `AI_MEMORY_ENABLE_API`)
está ativo. `--enable-api` monta apenas a API JSON, sem a interface web.
`--web-ui-dir` (variável `AI_MEMORY_WEB_UI_DIR`) exige `--enable-web`.

## 2. Autenticação

A superfície `/api/v1/*` e as rotas `/admin/*` aceitam dois mecanismos:

- **Sessão humana (navegador).** O operador faz `POST /auth/login` com corpo
  JSON `{"username": "...", "password": "..."}`. Em caso de sucesso, a
  resposta é HTTP 200 com o snapshot de identidade (seção `GET /auth/me`) e
  dois cookies:
  - `ai_memory_session`: `HttpOnly`, `SameSite=Strict`, `Path=/`, com
    `Secure` quando o servidor estiver configurado para cookies seguros.
  - `ai_memory_csrf`: legível pelo script, com `SameSite=Strict` e `Path=/`.
  O cliente envia requisições com `credentials: "include"`. Toda requisição
  autenticada por sessão que não seja `GET`, `HEAD` ou `OPTIONS` (inclusive
  `POST /api/v1/search`) precisa do cabeçalho `X-CSRF-Token` com o mesmo valor
  do cookie `ai_memory_csrf`. Sem o cabeçalho, com valor diferente ou sem o
  cookie, a resposta é `403` com `"csrf required"`.
- **Bearer (máquina).** `Authorization: Bearer <token>`. O token raiz vem da
  variável `AI_MEMORY_AUTH_TOKEN` (ou do campo equivalente de configuração) e
  tem autoridade de máquina raiz. Chaves de API com prefixo `aim_` são
  resolvidas para um usuário e recebem nível de usuário. Requisições com Bearer
  não exigem CSRF.

Regras de precedência:

- Um Bearer reconhecido tem precedência sobre qualquer credencial de navegador.
- Um Bearer inválido falha fechado (`401`), sem cair para outra credencial.
- Bearers não autenticam as rotas `/auth/*`.
- Antes de existir senha humana ou bootstrap concluído, requisições `GET` podem
  ser autenticadas pelo token raiz via HTTP Basic ou pelo cookie `HttpOnly`
  `ai_memory_auth`. Esse caminho está descontinuado e é desligado
  imediatamente quando a autenticação humana é ativada. Depois da ativação,
  Basic e esquemas desconhecidos não impedem o uso de uma sessão válida.
- As rotas `/mcp`, hooks, handoff, workstreams e `/identity` aceitam apenas
  Bearer. Um cookie de sessão não as autentica.
- Um usuário com troca de senha pendente recebe `403` com
  `"password change required"` em todas as rotas protegidas até concluir a
  troca.
- Um cookie de sessão de usuário desabilitado recebe `401`.
- Um cabeçalho `Host` fora da lista permitida recebe `403` antes de qualquer
  avaliação de autenticação (proteção contra DNS rebinding). Sem cabeçalho
  `Host`, a resposta é `400`.

A interface própria (shell e arquivos estáticos) é pública para que a tela de
login carregue. Os dados continuam protegidos. Comece por:

```http
GET /auth/me
```

Resposta (servidor com autenticação):

```json
{
  "username": "alice",
  "name": "Alice Smith",
  "role": "user",
  "must_change_password": false,
  "via": "session",
  "capabilities": {
    "normal_read": true,
    "normal_write": true,
    "admin": false,
    "user_management": false
  }
}
```

- `via` é `session` quando há sessão humana e `anonymous` quando o servidor não
  tem autenticação configurada. Nesse último caso, `username`, `name` e `role`
  são `null`, e `must_change_password` é `false`.
- `capabilities` são booleanos calculados pelo servidor. Os campos listados são
  os que a resposta retorna.

Não guarde `AI_MEMORY_AUTH_TOKEN`, chaves `aim_`, valores de sessão ou valores
de CSRF em `localStorage`. Para bootstrap, rotação de senha, recuperação, papéis,
expiração de sessão e ciclo de vida de chaves de API, consulte
[users.md](users.md). As rotas `POST /auth/password`, `POST /auth/logout` e
`POST /auth/recovery` também existem; a semântica de cada uma está descrita em
[users.md](users.md).

## 3. Modelo de erros

As respostas de erro das rotas de API e de autenticação têm corpo JSON:

```json
{ "error": "mensagem legível" }
```

Exceções:

- `400` e `403` gerados pelo filtro de `Host` têm corpo em texto simples.
- `401` gerado pela autenticação por Bearer tem corpo em texto simples
  (`auth required`) e cabeçalho `WWW-Authenticate: Bearer`.
- Parâmetros de query ou corpos JSON que não podem ser convertidos para o tipo
  esperado são rejeitados pela camada HTTP antes do tratador, com corpo em
  **texto simples** (não JSON): query de tipo errado (`limit=abc`, `limit=-1`,
  `include_open=1` — booleanos só aceitam `true` e `false`) = 400; corpo JSON
  sintaticamente inválido = 400; `POST /search` sem `Content-Type:
  application/json` = 415; JSON válido com campo de tipo errado = 422. A
  mensagem exata é da camada HTTP e não faz parte do contrato; um cliente deve
  tratar qualquer 4xx sem depender do formato do corpo. (Na busca `GET`, o
  `limit` é lido pelo próprio tratador e erra em JSON, ver 4.5.)
- Mensagens de escopo (corpo JSON `{"error": …}`):

| Situação | Status | Mensagem |
|---|---|---|
| Workspace inexistente | 404 | `workspace '<w>' not found` |
| Projeto inexistente nas rotas por projeto (listagem de páginas, `recent`, `briefing`, `overview`, `handoffs`, `sessions`, observações, `history`) e na busca de um projeto (`workspace`+`project`) | 404 | `project '<p>' not found` |
| Projeto inexistente num escopo de `scopes`/`scope` da busca, e na leitura de **página completa** quando o chamador tem restrição por projeto | 404 | `project '<p>' not found in workspace '<w>'` (e, se o nome do projeto contém uma única `/`, acrescenta `; project names never contain '/': pass workspace "<ws>" and project "<proj>" as separate arguments`) |
| Nome de projeto ambíguo no workspace | 400 | `project '<p>' is ambiguous in workspace '<w>'` (seguido da lista de titulares, quando conhecida) |
| Escopo de `scopes` com workspace/projeto vazio | 400 | `scope workspace cannot be empty` / `scope project cannot be empty` |
| Sem permissão de leitura, com nível de acesso `N` | 403 | `not authorized for <projeto>: you have <N> access and this needs read. Ask the server operator to raise your access.` |
| Sem permissão de leitura e sem nenhum acesso | 403 | `not authorized for <projeto>. This is an access problem, not an empty memory — ask the server operator to grant you read access on it.` |
| Overview de workspace sem projeto legível | 403 | `not authorized for <workspace>: no project in it is readable by this user. This is an access problem, not an empty workspace.` |

| Status | Quando |
|---|---|
| `400 Bad Request` | Parâmetro inválido: `limit` não inteiro em `GET /search`; `order`, `kinds` ou `state` desconhecidos; identificador de sessão malformado; timestamp RFC 3339 inválido; cursor inválido ou de outro escopo; cursor inconsistente com `updated_since`; escopo de busca fora do formato `workspace/project`; `workspace` sem `project` ou o inverso; `scopes` combinado com `workspace`/`project`; mais de 25 escopos; percentual de codificação inválido na query. |
| `401 Unauthorized` | Sessão humana ou Bearer ausente, expirado, revogado ou inválido. |
| `403 Forbidden` | `Host` fora da lista permitida; mutação por sessão sem CSRF válido; troca de senha pendente; `all_owners=true` sem ser root; autenticação sem a capacidade exigida; projeto existente sem permissão de leitura para o usuário (quando a autorização por repositório está ativa); `GET /overview` de workspace em que o usuário não pode ler nenhum projeto. |
| `404 Not Found` | Workspace, projeto ou página inexistente; registro da página sem arquivo no disco; sessão não visível naquele projeto para o chamador. |
| `429 Too Many Requests` | Limite de tentativas de login ou recuperação atingido, ou fila de derivação de senha saturada. |
| `500 Internal Server Error` | Falha do pool de leitura ou do banco. Em `/api/v1`, o corpo é sempre `{"error": "internal server error"}`; a causa é registrada no servidor e não é retornada. |

## 4. Referência de endpoints

Todos os endpoints são `GET`, exceto `POST /api/v1/search`. Caminhos são
relativos a `/api/v1`. Respostas são JSON. Identificadores são strings no
formato UUID. Timestamps são strings ISO 8601 em UTC.

### 4.1 Workspaces

```http
GET /api/v1/workspaces
```

**Resposta:** array JSON direto (sem envelope). Cada item:

| Campo | Tipo | Descrição |
|---|---|---|
| `workspace_name` | string | Nome do workspace. |
| `project_count` | inteiro | Projetos do workspace (só os legíveis pelo chamador, com autorização por projeto). |
| `page_count` | inteiro | Páginas com `is_latest = 1` **e não expiradas** no workspace. |
| `last_updated` | string ou `null` | `updated_at` mais recente; `null` quando não há páginas. |

```json
[
  {
    "workspace_name": "default",
    "project_count": 3,
    "page_count": 412,
    "last_updated": "2026-05-28T14:02:11.123Z"
  }
]
```

Com autorização por repositório ativa, só entram os projetos legíveis pelo
chamador, e workspaces sem nenhum projeto legível são omitidos. Ordem: nome do
workspace crescente. Um workspace sem projetos (e sem restrição por projeto)
aparece com `project_count` 0.

### 4.2 Projetos

```http
GET /api/v1/projects
GET /api/v1/projects?workspace=NAME
```

Sem `workspace`, lista os projetos de todos os workspaces. Com `workspace`
(após aparar espaços; vazio equivale a ausente), lista apenas os projetos
daquele workspace.

**Resposta:** array JSON direto. Cada item:

| Campo | Tipo | Descrição |
|---|---|---|
| `workspace_id` | string | Identificador (UUID) do workspace. Estável, ao contrário do nome. |
| `workspace_name` | string | Workspace do projeto. |
| `project_id` | string | Identificador (UUID) do projeto: a coordenada que não é ambígua quando dois projetos têm o mesmo nome legado. |
| `project_name` | string | Nome do projeto. |
| `page_count` | inteiro | Páginas com `is_latest = 1` e não expiradas. |
| `last_updated` | string ou `null` | `updated_at` mais recente; `null` sem páginas. |

Ordem: `last_updated` decrescente, projetos sem páginas por último (empates
sem ordem definida). O HTML da página inicial usa a mesma lista.

```json
[
  {
    "workspace_id": "0198f0a2-0000-7000-8000-000000000001",
    "workspace_name": "default",
    "project_id": "0198f0a2-0000-7000-8000-000000000002",
    "project_name": "ai-memory",
    "page_count": 138,
    "last_updated": "2026-05-28T14:02:11.123Z"
  }
]
```

### 4.3 Páginas (listagem)

```http
GET /api/v1/workspaces/{workspace}/projects/{project}/pages
```

**Resposta:** array JSON direto com as páginas `is_latest = 1` do projeto,
ordenadas por `path` em ordem crescente. Cada item:

| Campo | Tipo | Descrição |
|---|---|---|
| `path` | string | Caminho relativo na árvore da wiki. |
| `title` | string | Título da página. |
| `kind` | string | Tipo semântico (seção 4.6, "Tipo de página"). |
| `tier` | string | `working`, `episodic`, `semantic` ou `procedural`. |
| `updated_at` | string | Última atualização. |

```json
[
  {
    "path": "decisions/0007-db.md",
    "title": "Standardised on one database",
    "kind": "decision",
    "tier": "semantic",
    "updated_at": "2026-05-27T09:12:00.000Z"
  }
]
```

`404` se o workspace ou o projeto não existir ou não for legível pelo chamador.

### 4.4 Página (leitura completa)

```http
GET /api/v1/workspaces/{workspace}/projects/{project}/pages/{path}
```

`{path}` é um caminho com barras, por exemplo `decisions/0007-db.md` ou
`concepts/foo/bar.md`.

**Resposta:** objeto JSON.

| Campo | Tipo | Descrição |
|---|---|---|
| `id` | string | Identificador (UUID) da **versão atual**. É o token que uma escrita/remoção condicional passa de volta. Muda a cada nova versão, e por isso o `ETag` também muda. |
| `workspace` | string | Nome do workspace. |
| `project` | string | Nome do projeto. |
| `path` | string | Caminho da página. |
| `title` | string | Título. |
| `kind` | string | Tipo semântico. |
| `tier` | string | Camada de memória. |
| `pinned` | boolean | Página fixada. |
| `created_at` | string | Criação: instante da **versão mais antiga** da cadeia de substituições que ainda existe (versões removidas por retenção encurtam a cadeia). |
| `updated_at` | string | Última atualização. |
| `supersedes` | string ou `null` | Caminho da versão anterior substituída. |
| `frontmatter` | objeto | Frontmatter da página. |
| `body_markdown` | string | Corpo em markdown, lido do arquivo no momento da requisição. |
| `links` | array | Páginas mais recentes referenciadas por esta página. |
| `backlinks` | array | Páginas mais recentes que referenciam esta página. |
| `author` | objeto, opcional | Atribuição do autor (`username`, e opcionalmente `name` e `email`). Campo omitido quando ausente. |

Cada item de `links` e `backlinks` tem `path`, `title`, `kind`, `workspace` e
`project`.

```json
{
  "id": "0198f0a2-3c4d-7e5f-8a9b-0c1d2e3f4a5b",
  "workspace": "default",
  "project": "ai-memory",
  "path": "decisions/0007-db.md",
  "title": "Standardised on one database",
  "kind": "decision",
  "tier": "semantic",
  "pinned": true,
  "created_at": "2026-05-27T09:12:00.000Z",
  "updated_at": "2026-05-28T11:04:33.123Z",
  "supersedes": null,
  "frontmatter": { "tags": ["adr"], "pinned": true },
  "body_markdown": "# Standardised on one database\n\n…",
  "links": [
    { "path": "concepts/db-rules.md", "title": "DB rules", "kind": "rule", "workspace": "default", "project": "ai-memory" }
  ],
  "backlinks": [
    { "path": "sessions/2026-05-27.md", "title": "Session 2026-05-27", "kind": "session", "workspace": "default", "project": "ai-memory" }
  ]
}
```

**Erros:**

- `404` com `"page not found"` quando não há registro `is_latest = 1` para o caminho.
- `404` com `"page file not found"` quando o registro existe, mas o arquivo não está no disco.
- `400` com `"invalid path: <motivo>"` para caminho inválido. A validação do
  caminho ocorre depois de localizar o registro da página (a busca do registro
  usa o texto exato), então, na prática, um caminho inválido responde `404
  "page not found"`: nenhum caminho inválido existe no índice.
- Ordem das checagens: escopo (só com restrição por projeto: 403/404 de
  escopo) → registro → validação do caminho → leitura do arquivo → links.
- A leitura **não** esconde uma página expirada (a listagem, o `recent` e a
  busca escondem); `links` e `backlinks` omitem páginas expiradas e as de
  projetos que o chamador não pode ler, vêm sem repetição, ordenados por
  workspace, projeto e caminho, sem limite de tamanho.

**Cache:** `Cache-Control: private, max-age=300` e `ETag`. O `ETag` é o
SHA-256, em hexadecimal e entre aspas, do JSON exato retornado. Se
`If-None-Match` for igual ao `ETag` atual, a resposta é `304 Not Modified`,
sem corpo, com `ETag` e `Cache-Control` repetidos. A comparação é exata; não há
tratamento de tags fracas.

### 4.5 Busca

Há duas formas: query string (um projeto, vários projetos com `scope`, ou
global) e corpo JSON (vários escopos).

```http
GET /api/v1/search?q=karpathy&limit=20
GET /api/v1/search?q=karpathy&workspace=default&project=ai-memory
GET /api/v1/search?q=karpathy&scope=default/ai-memory,default/shared-notes&scope=default/infra
```

```http
POST /api/v1/search
Content-Type: application/json

{
  "q": "karpathy",
  "scopes": [
    { "workspace": "default", "project": "ai-memory" },
    { "workspace": "default", "project": "shared-notes" }
  ],
  "limit": 20
}
```

**Parâmetros (GET):**

- `q` (ou `query`): termo de busca.
- `workspace` e `project`: devem vir juntos, ou nenhum dos dois.
- `scope`: pode ser repetido. Cada valor aceita uma lista separada por vírgula
  de itens no formato `workspace/project`. Item sem exatamente uma barra, ou
  com parte vazia, recebe `400` com `"scope must use the workspace/project format"`.
- `limit`: inteiro. Padrão `10`. Valor não inteiro recebe `400` com
  `"limit must be an integer"`.

**Parâmetros (POST):** `q` (ou `query`), `workspace` e `project` (juntos),
`scopes` (lista de `{workspace, project}`), `limit`.

**Regras:**

- `q` é aparado antes do uso. Se ficar vazio, a resposta é `200` com `[]`.
  Não há erro para `q` vazio.
- O texto é tratado como termos de busca em texto completo. Operadores da
  sintaxe de busca são neutralizados e palavras de parada configuradas são
  descartadas. Se nada pesquisável restar, a resposta é `[]`.
- Sem `workspace`/`project`/`scopes`, a busca cobre todos os projetos legíveis
  pelo chamador. Com `workspace` e `project`, cobre um projeto. Com `scopes`,
  cobre a união dos escopos listados.
- Escopos múltiplos: cada escopo é pesquisado separadamente (cada um devolve
  até `limit` hits). Hits repetidos são unificados pelo identificador da página,
  mantendo o melhor `rank` (o menor). Depois, os hits são ordenados por `rank`
  crescente, com desempate por `path` crescente e depois pelos bytes do
  identificador da página, e cortados em `limit`. Com um só escopo não há
  unificação nem reordenação extra.
- Busca de um projeto ou de vários (`workspace`+`project`, `scope`, `scopes`)
  usa o ranqueador híbrido completo (texto, entidades, vetores se houver
  embedder, grafo, observações, autoridade, reranqueador opcional); a busca
  global (sem escopo) é só texto completo. Ver
  [storage-and-retrieval.md](storage-and-retrieval.md). As ordens são as mesmas
  da ferramenta MCP de consulta.
- O corte em `limit` acontece antes de descartar hits sem registro; por isso a
  resposta pode ter menos itens que `limit` mesmo havendo mais resultados.
- A ordem das checagens e as mensagens literais de cada 400 estão em
  [wiki-browser.md](wiki-browser.md) 3.3 ("Busca, regras gerais"). Em
  particular, o termo vazio vence qualquer erro de combinação de parâmetros
  (200 `[]`), mas não vence um erro de decodificação da query nem `limit`
  inválido no `GET`.
- Escopos múltiplos são limitados a 25 (`"at most 25 scopes are allowed"`).
  `scopes` não pode ser combinado com `workspace`/`project`
  (`"scopes cannot be combined with workspace/project"`). Escopo inexistente
  recebe `404`; escopo sem permissão de leitura recebe `403` (textos na tabela
  de mensagens de escopo da seção 3).
- `limit` tem padrão `10` e é limitado ao intervalo `1..100`.
- Apenas páginas `is_latest = 1` e não expiradas são pesquisadas.
- O `rank` já inclui um ajuste de autoridade pela página (tipo, camada e
  fixação). Quanto menor o `rank`, melhor o resultado.
- Hits cujo registro de página não pode ser resolvido são omitidos sem erro.

**Resposta:** array JSON direto. Cada item:

| Campo | Tipo | Descrição |
|---|---|---|
| `workspace` | string | Workspace da página. |
| `project` | string | Projeto da página. |
| `path` | string | Caminho da página. |
| `title` | string | Título. |
| `kind` | string | Tipo semântico. |
| `snippet` | string | Trecho do corpo com os termos em `<mark>…</mark>`. Bordas cortadas do trecho recebem `…`. |
| `rank` | número | Rank ajustado. Menor é melhor. |

```json
[
  {
    "workspace": "default",
    "project": "ai-memory",
    "path": "concepts/karpathy-wiki.md",
    "title": "Karpathy LLM Wiki pattern",
    "kind": "concept",
    "snippet": "Andrej <mark>Karpathy</mark>'s LLM wiki design …",
    "rank": -8.4
  }
]
```

**Cache:** `Cache-Control: private, max-age=30` para buscas com `q` não vazio.
A resposta com `q` vazio não traz cabeçalho de cache.

### 4.6 Páginas recentes

```http
GET /api/v1/workspaces/{workspace}/projects/{project}/recent?limit=10
GET /api/v1/workspaces/{workspace}/projects/{project}/recent?updated_since=2026-05-28T00:00:00Z&limit=50
GET /api/v1/workspaces/{workspace}/projects/{project}/recent?cursor={cursor}&limit=50
```

O endpoint tem três modos, escolhidos pelos parâmetros:

**Modo 1: sem `updated_since` e sem `cursor`.** Retorna um array JSON direto
com até `limit` páginas `is_latest = 1`, ordenadas por `updated_at` decrescente.
Cada item tem `path`, `title`, `kind`, `tier` e `updated_at`. Cache:
`private, max-age=30`.

**Modo 2: com `updated_since`.** Retorna páginas `is_latest = 1` e não
expiradas com `updated_at` estritamente posterior ao instante informado,
ordenadas por `updated_at` crescente e, em empate, por `path` crescente. A
resposta é um objeto com as páginas e um cursor para a próxima página. Cache:
`private, no-store`.

**Modo 3: com `cursor`.** Continua a leitura a partir de um cursor retornado
pelo modo 2. Se `updated_since` também for enviado, ele deve ser igual ao
instante embutido no cursor; caso contrário, `400`. O cursor só vale para o
mesmo workspace e projeto.

```json
{
  "pages": [
    {
      "path": "sessions/2026-05-28.md",
      "title": "Session 2026-05-28",
      "kind": "session",
      "tier": "episodic",
      "updated_at": "2026-05-28T14:02:11.123Z"
    }
  ],
  "next_cursor": "<cursor opaco>"
}
```

- `next_cursor` é `null` quando não há mais páginas.
- O cursor é opaco: base64url sem preenchimento, sobre um JSON interno. Tamanho
  máximo de 8192 caracteres. Não o interprete nem o monte.
- `updated_since` aceita RFC 3339 no formato `AAAA-MM-DDTHH:MM:SS`, com fração
  opcional de 1 a 9 dígitos, seguido de `Z` (ou `z`) ou de `±HH:MM`. Qualquer
  outra forma recebe `400` com `"invalid RFC3339 timestamp"`.
- Erros de cursor: `"invalid recent cursor"` (malformado ou grande demais),
  `"recent cursor does not match scope"` e `"conflicting recent cursor bounds"`.
- `limit` tem padrão `10` e é limitado ao intervalo `1..100`. Nos modos 2 e 3,
  a existência de mais páginas é detectada buscando `limit + 1` linhas.

**Tipo de página (`kind`).** Todos os endpoints usam a mesma regra. Um `kind`
explícito no frontmatter vence. Sem ele, o prefixo do caminho define o tipo:

| Prefixo do caminho | `kind` |
|---|---|
| `_rules/` | `rule` |
| `_slots/` | `slot` |
| `sessions/` | `session` |
| `decisions/` | `decision` |
| `gotchas/` | `gotcha` |
| `concepts/` | `concept` |
| `procedures/` | `procedure` |
| `notes/` | `note` |
| `profile/` | `preference` |
| Outros | `fact` |

### 4.7 Briefing (snapshot estruturado)

```http
GET /api/v1/workspaces/{workspace}/projects/{project}/briefing?limit=10
```

Retorna o snapshot de projeto usado pela ferramenta MCP `memory_briefing`,
sem o bloco opcional `settled`, que esta API nunca inclui. A montagem é só SQL,
sem LLM e de forma determinística. Cache: `private, no-store`.

**Resposta:** objeto JSON.

| Campo | Tipo | Descrição |
|---|---|---|
| `counts` | objeto | Totais de vida do projeto: `pages_latest`, `pages_all`, `sessions`, `observations`, `evidence_rows`. |
| `activity_7d` | objeto | Atividade nos últimos 7 dias. |
| `activity_30d` | objeto | Atividade nos últimos 30 dias. |
| `last_observation_at` | string ou `null` | Observação mais recente; `null` sem observações. |
| `pending_handoff_count` | inteiro | Handoffs abertos (não aceitos) visíveis ao chamador. |
| `pending_message_count` | inteiro | Mensagens pendentes na caixa de entrada do projeto. |
| `rules` | array | Todas as páginas em `_rules/`. |
| `slots` | array | Páginas em `_slots/`. |
| `recent_pages` | array | As `limit` páginas `is_latest = 1` mais recentes. |
| `pinned` | array, opcional | Páginas fixadas e mais recentes, em ordem decrescente, com limite. Omitido quando vazio. |
| `cross_project_dependents` | inteiro | Projetos distintos cujas páginas apontam para este projeto. |
| `cross_project_dependencies` | inteiro | Projetos distintos para os quais as páginas deste projeto apontam. |

`activity_7d` e `activity_30d` têm `days`, `sessions`, `observations` e
`pages_updated`. Sessões são contadas por `started_at`, observações por
`created_at` e páginas por `updated_at`, sempre dentro da janela até agora e
considerando apenas `is_latest = 1` para páginas.

Itens de `rules`, `slots`, `recent_pages` e `pinned` têm `path`, `title`,
`kind` e `updated_at`. O corpo da página não é incluído.

```json
{
  "counts": { "pages_latest": 138, "pages_all": 162, "sessions": 27, "observations": 4198, "evidence_rows": 0 },
  "activity_7d":  { "days": 7,  "sessions": 6,  "observations": 921,  "pages_updated": 41 },
  "activity_30d": { "days": 30, "sessions": 24, "observations": 3712, "pages_updated": 102 },
  "last_observation_at": "2026-05-28T13:58:02.123Z",
  "pending_handoff_count": 0,
  "pending_message_count": 0,
  "rules": [{ "path": "_rules/database.md", "title": "Single database only", "kind": "rule", "updated_at": "…" }],
  "slots": [{ "path": "_slots/focus.md", "title": "Current focus", "kind": "slot", "updated_at": "…" }],
  "recent_pages": [
    { "path": "sessions/2026-05-28.md", "title": "Session 2026-05-28", "kind": "session", "updated_at": "…" }
  ],
  "cross_project_dependents": 1,
  "cross_project_dependencies": 0
}
```

### 4.8 Overview (agregados de workspace e de projeto)

```http
GET /api/v1/workspaces/{workspace}/overview?limit=10
GET /api/v1/workspaces/{workspace}/projects/{project}/overview?limit=10
```

Reúne em uma chamada o que a tela inicial costuma precisar. Cache:
`private, no-store`.

**Resposta:** objeto com três campos.

- `handoff`: o handoff aberto mais recente visível ao chamador no escopo, ou
  `null`. No overview de workspace, o escopo é o workspace inteiro. No overview
  de projeto, é o projeto. Campos: `agent`, `at`, `project`, `summary`,
  `open_questions` e `next_steps`. Esses três últimos sempre aparecem neste
  campo, sujeitos apenas ao filtro de dono (seção 4.9).
- `briefing`: snapshot de briefing (seção 4.7). No overview de workspace, os
  contadores são agregados de todos os projetos do workspace.
- `health`: contadores de saúde e listas de detalhe.

| Campo de `health` | Tipo | Descrição |
|---|---|---|
| `stale` | inteiro | Páginas atuais, não expiradas, de tier `episodic`, com `updated_at` anterior a (agora − 30 dias). |
| `duplicates` | inteiro | Páginas atuais e não expiradas que repetem um título: para cada título (comparação exata) com `n > 1` páginas no escopo (workspace ou projeto), soma `n − 1`. |
| `contradictions` | inteiro | Sempre `0` nesta versão. |
| `orphans` | inteiro | Páginas atuais e não expiradas sem nenhuma linha de link de saída **nem** de entrada. |
| `audited_at` | `null` | Sempre `null` nesta versão. |
| `stale_pages` | array de páginas de saúde | Até `limit` itens, `updated_at` crescente (as mais antigas primeiro). |
| `duplicate_pages` | array de páginas de saúde | Até `limit` itens das páginas cujo título se repete, ordenadas por título e depois nome do projeto. |
| `orphan_pages` | array de páginas de saúde | Até `limit` itens, `updated_at` decrescente. |

Os contadores são calculados sobre o workspace (overview de workspace, só nos
projetos legíveis pelo chamador) ou sobre o projeto (overview de projeto);
nas listas de duplicatas e no contador, uma página oculta ao chamador também
não faz outra parecer duplicada.

Cada página de saúde tem `workspace`, `project`, `path`, `title` e `kind`.

```json
{
  "handoff": {
    "agent": "claude-code",
    "at": "2026-07-28T12:00:00Z",
    "project": "ai-memory",
    "summary": "…",
    "open_questions": [],
    "next_steps": []
  },
  "briefing": { "counts": { }, "activity_7d": { }, "rules": [ ], "recent_pages": [ ] },
  "health": {
    "stale": 4,
    "duplicates": 1,
    "contradictions": 0,
    "orphans": 12,
    "audited_at": null,
    "stale_pages": [
      { "workspace": "default", "project": "ai-memory", "path": "concepts/old-thing.md", "title": "Old thing", "kind": "concept" }
    ],
    "duplicate_pages": [],
    "orphan_pages": []
  }
}
```

`limit` tem padrão `10` e é limitado ao intervalo `1..100`. Em
`GET /workspaces/{workspace}/overview`, o workspace inexistente recebe `404`
com `"workspace '…' not found"`. Se a autorização por repositório estiver ativa
e o chamador não puder ler nenhum projeto do workspace, a resposta é `403`. A
mensagem informa que é problema de acesso, não de workspace vazio.

### 4.9 Handoffs

```http
GET /api/v1/workspaces/{workspace}/projects/{project}/handoffs?state=open&limit=50
GET /api/v1/workspaces/{workspace}/projects/{project}/handoffs?all_owners=true
```

**Parâmetros:**

- `state`: `open`, `accepted` ou `expired`. Valores exatos, em minúsculas, após
  aparar espaços. Omitido ou vazio lista todos os estados, o que permite
  encontrar um handoff já consumido. Outro valor recebe `400` com
  `"unknown handoff state: X"`.
- `limit`: padrão `50`, limitado ao intervalo `1..200`.
- `all_owners`: `true` lista os handoffs de todos os operadores. Só o root pode
  usar esse parâmetro. Outro chamador recebe `403` com
  `"all_owners requires root authorization"`. O padrão é `false`.

**Escopo por dono.** Sem `all_owners`, o chamador vê os próprios handoffs e os
compartilhados (sem dono). Um chamador sem identidade nomeada vê apenas os
compartilhados. A identidade nomeada é o par issuer/subject quando existe; caso
contrário, o nome de usuário. Um proxy que encerra OIDC e repassa o par obtém
leitura dos próprios handoffs com `redacted: false`.

**Ordenação:** `created_at` decrescente.

**Corpo de prompt (`summary`, `open_questions`, `next_steps`).** Esses campos
são derivados de prompts do operador. Eles são omitidos e `redacted` vale
`true` quando:

- o servidor tem autenticação configurada e o chamador não tem identidade
  nomeada e não é root (tabela completa abaixo); ou
- (defesa em profundidade) o filtro "todos os donos" estiver em uso por quem
  não é root. Na prática isso não ocorre: `all_owners=true` de quem não é root
  já recebeu `403` antes de qualquer leitura.

Tabela de decisão (filtro de dono × nível de autenticação do chamador):

| Filtro de dono | Corpo servido quando o nível é… |
|---|---|
| Dono nomeado (o chamador tem identidade) | `user` ou `root` (nível anônimo ou ausente: omitido) |
| Todos os donos (`all_owners=true`) | somente `root` |
| Sem dono atribuível (chamador sem identidade) | `root`, `anônimo` (servidor sem autenticação) ou ausente (API montada sem camada de autenticação); `user` sem identidade nomeada: omitido |

Em servidor sem autenticação, o corpo é sempre servido. Quando o corpo é
omitido, a interface deve mostrar que é preciso autenticar, não um cartão vazio.

**Resposta:** objeto `{"handoffs": [...]}`. Cada item:

| Campo | Tipo | Descrição |
|---|---|---|
| `id` | string | Identificador do handoff. |
| `agent` | string | Agente que criou o handoff. |
| `at` | string | Criação. |
| `state` | string | `open`, `accepted` ou `expired`. |
| `summary` | string, opcional | Omitido quando `redacted` é `true`. |
| `open_questions` | array, opcional | Omitido quando `redacted` é `true`. |
| `next_steps` | array, opcional | Omitido quando `redacted` é `true`. |
| `redacted` | boolean | `true` quando o corpo foi omitido. |
| `files_touched` | array de string | Arquivos tocados. |
| `cwd` | string, opcional | Diretório de trabalho capturado. |
| `owner` | string, opcional | Dono, na chave de armazenamento qualificada. Omitido quando compartilhado. |
| `accepted_by` | string, opcional | Quem aceitou, na chave qualificada. |
| `accepted_at` | string, opcional | Momento da aceitação. |

A chave qualificada tem o formato `user:<nome>` ou
`oidc:<tamanho-do-issuer-em-bytes>:<issuer><subject>`.

```json
{
  "handoffs": [
    {
      "id": "01930…",
      "agent": "claude-code",
      "at": "2026-07-28T12:00:00Z",
      "state": "accepted",
      "summary": "…",
      "open_questions": [],
      "next_steps": [],
      "redacted": false,
      "files_touched": [],
      "cwd": "/home/me/work/app",
      "owner": "user:alice",
      "accepted_by": "user:alice",
      "accepted_at": "2026-07-28T13:00:00Z"
    }
  ]
}
```

Esta leitura não altera o estado do handoff. Aceitar um handoff é uma operação
de escrita, fora desta superfície.

### 4.10 Grafo entre projetos

```http
GET /api/v1/graph
```

Retorna as arestas resolvidas de links entre páginas de projetos diferentes.
Só entram páginas `is_latest = 1`, não expiradas, e arestas cujos dois extremos
são legíveis pelo chamador. Não há filtro por workspace ou projeto nesta versão.
Cache: `private, max-age=30`.

```json
{
  "edges": [
    {
      "from_workspace": "default",
      "from_project": "ai-memory",
      "from_path": "decisions/0014-storage.md",
      "to_workspace": "default",
      "to_project": "infra",
      "to_path": "runbooks/wal-mode.md"
    }
  ]
}
```

### 4.11 Ícone da aba (favicon)

```http
GET /favicon.ico
```

Retorna um PNG transparente com `Content-Type: image/png`, mesmo com extensão
`.ico`. O logo é o mesmo que a interface embutida usa no cabeçalho. Navegadores
buscam este caminho automaticamente.

- Existe quando `--enable-web` está ativo.
- Fica na raiz do host: fora de `--base-path` e fora do slug da interface.
- Não exige autenticação e não passa pela verificação de `Host`, para que a aba
  carregue o ícone antes do login.

### 4.12 Sessões de um projeto

```http
GET /api/v1/workspaces/{workspace}/projects/{project}/sessions?limit=20&offset=0&include_open=false
```

Lista as sessões que tocaram o projeto. Uma sessão entra na lista quando a sua
linha está ancorada no projeto ou quando pelo menos uma das suas observações foi
registrada ali. Assim, uma sessão que trocou de repositório aparece em ambos.

- `limit`: padrão `20`, intervalo `1..100`.
- `offset`: padrão `0`.
- `include_open`: padrão `false`. Com `false`, só entram sessões com `ended_at`
  preenchido.
- Ordenação: `started_at` decrescente e, em empate, `session_id` decrescente.
- Filtro de dono igual ao dos handoffs: chamador com identidade nomeada vê as
  próprias sessões e as sem dono; chamador sem identidade vê só as sem dono.

**Resposta:** objeto `{"sessions": [...]}`. Cada item:

| Campo | Tipo | Descrição |
|---|---|---|
| `session_id` | string | Identificador da sessão. |
| `cwd` | string ou `null` | Diretório de trabalho capturado. |
| `agent_kind` | string | Tipo de agente, como `claude-code`. |
| `started_at` | string | Início. |
| `ended_at` | string ou `null` | Fim; `null` enquanto a sessão estiver aberta. |
| `observation_count` | inteiro | Observações desta sessão registradas neste projeto. |
| `actor_user` | string ou `null` | Operador dono da sessão. |
| `consolidation` | objeto ou `null` | Resumo da geração de consolidação mais recente no escopo, com `state` e `attempts`. |

Esta listagem não retorna `total`.

Cache: `private, no-store`.

### 4.13 Observações de uma sessão

```http
GET /api/v1/workspaces/{workspace}/projects/{project}/sessions/{session_id}/observations?limit=50&offset=0&order=asc&kinds=user-prompt,stop&q=migration&body_max_chars=4000
```

Retorna as observações brutas de hooks (prompts, chamadas de ferramenta,
paradas) de uma sessão, paginadas. Só entram linhas registradas em
`{workspace}/{project}`.

**Visibilidade.** A sessão precisa ser visível pela mesma regra da seção 4.12,
incluindo sessões abertas e o filtro de dono. Caso contrário, `404` com
`"session '…' not found"`. Um `session_id` que não é um identificador válido
recebe `400` com `"invalid session id: …"`.

**Parâmetros:**

- `limit`: padrão `50`, intervalo `1..200`.
- `offset`: padrão `0`.
- `order`: `asc` (ordem de captura, padrão) ou `desc`. Não diferencia maiúsculas.
  Outro valor recebe `400` com `"unknown order: …"`.
- `kinds`: lista separada por vírgula, com itens aparados. Itens vazios são
  ignorados. Valores aceitos: `session-start`, `user-prompt`, `pre-tool-use`,
  `post-tool-use`, `pre-compact`, `post-compaction`, `notification`, `stop`,
  `session-end` e `other`. O parser também aceita as variantes com sublinhado
  e os nomes originais de evento. Valor desconhecido recebe `400` com
  `"unknown observation kind: …"`.
- `q`: busca em texto completo restrita à sessão. Segue as mesmas regras de
  normalização de `q` na busca (seção 4.5). Se nada pesquisável restar, o
  filtro é ignorado e a sessão é listada inteira.
- `body_max_chars`: padrão `4000`, intervalo `200..16384`. Corpos maiores são
  cortados e recebem, no fim, a linha `[body truncated; N chars omitted]`
  precedida de quebra de linha. O corte conta caracteres, não bytes.

**Contagens:**

- `total`: linhas desta sessão no projeto que atendem a `kinds` e `q`, antes da
  paginação.
- `elided_other_scope`: linhas da mesma sessão registradas em outros projetos.
  Essa contagem não é afetada por `kinds` nem por `q`.

**Resposta:** objeto JSON:

```json
{
  "session": {
    "session_id": "0198f0a2-3c4d-7e5f-8a9b-0c1d2e3f4a5b",
    "cwd": "/home/me/work/app",
    "agent_kind": "claude-code",
    "started_at": "2026-08-16T09:12:03.412Z",
    "ended_at": "2026-08-16T10:47:55.001Z",
    "observation_count": 143,
    "actor_user": null,
    "consolidation": null
  },
  "observations": [
    {
      "id": "0198f0a2-4d5e-7f60-9a0b-1c2d3e4f5a6b",
      "session_id": "0198f0a2-3c4d-7e5f-8a9b-0c1d2e3f4a5b",
      "kind": "user-prompt",
      "title": "User prompt",
      "body": "Add a migration for the sessions table ...",
      "importance": 5,
      "created_at": "2026-08-16T09:12:10.020Z",
      "extension": null,
      "source_event": null
    }
  ],
  "total": 12,
  "offset": 0,
  "limit": 50,
  "order": "asc",
  "elided_other_scope": 0,
  "body_max_chars": 4000
}
```

- `limit` e `body_max_chars` na resposta são os valores já limitados.
- `importance` vai de `1` a `10`.
- `extension` e `source_event` são `null` quando não há extensão de origem.
- Os corpos foram sanitizados e limitados na ingestão. Trate-os como texto
  histórico não confiável.

Esta leitura usa a mesma visibilidade e o mesmo formato da ferramenta MCP
`memory_read_session_observations`. Cache: `private, no-store`.

### 4.14 Histórico de versões de uma página e diff

```http
GET /api/v1/workspaces/{workspace}/projects/{project}/history/{path}?from=<id>&to=<id>&limit=50&offset=0
```

Lista as versões guardadas do caminho (da mais nova para a mais antiga) e o
diff de linhas entre duas delas. É a mesma leitura da página HTML de histórico
([wiki-browser.md](wiki-browser.md) 2.8, que também descreve as regras de
visibilidade, os limites do diff e o algoritmo).

**Parâmetros:**

- `from`, `to`: ids de versão (UUID); vazios contam como ausentes. Id malformado
  = `400` com `"invalid from version id"` / `"invalid to version id"`. `to`
  padrão: a versão mais nova do caminho (mesmo com `offset` > 0). `from`
  padrão: a versão que `to` substituiu; se ela não existe neste caminho (página
  movida/ressuscitada) ou está oculta, o diff parte do **vazio** (`from` fica
  `null`). Um `from`/`to` explícito que não pertence a este caminho e projeto =
  `404` com `"version not found"`.
- `limit`: padrão `50`, intervalo `1..200`. `offset`: padrão `0`.
- Caminho inválido = `400` com `"invalid path: <motivo>"`. Sem versão visível =
  `404` com `"page not found"`.

**Visibilidade:** versões expiradas são ocultas; se a versão atual do caminho
expirou, o histórico inteiro some. O `supersedes` de uma versão que aponta para
uma versão oculta vem como `null`. Escopo como nas demais rotas (404/403).

**Resposta:** objeto JSON, campos nesta ordem:

| Campo | Tipo | Descrição |
|---|---|---|
| `workspace`, `project`, `path` | string | O que foi pedido (o caminho como veio na URL). |
| `total` | inteiro | Versões visíveis do caminho, antes da paginação. |
| `offset`, `limit` | inteiro | Valores usados (`limit` já ajustado). |
| `versions` | array | A janela pedida, da mais nova para a mais antiga (empate de instante: ordem de inserção decrescente). |
| `diff` | objeto | O diff (abaixo). |

Cada item de `versions`: `id`, `supersedes` (id ou `null`), `is_latest`
(boolean), `title`, `tier`, `created_at`, `body_chars` (tamanho do corpo em
**caracteres**) e `author` (`username`, e opcionalmente `name` e `email`;
omitido quando a escrita não foi atribuída).

`diff`: `from` (objeto `{id, title, created_at}` ou `null`), `to` (idem),
`status` (`same`, `changed` ou `too_large`), `reason` (só com `too_large`: uma
de `a version is larger than the diff limit`, `a version has more lines than the
diff limit`, `the versions differ in more lines than the diff limit`), `truncated`
(boolean: o limite de 2 000 linhas de saída cortou `hunks`) e `hunks` (array;
vazio com `same`, `too_large` e na diferença só de fim de linha). Cada `hunk`:
`old_start`, `old_len`, `new_start`, `new_len` (inteiros) e `lines`, com itens
`{op, text}` onde `op` é `" "` (contexto), `"-"` ou `"+"` e `text` é a linha sem
o fim de linha.

Cache: `private, no-store`.

## 5. Limites, paginação e cache

**Limites de `limit`:**

| Endpoint | Padrão | Intervalo |
|---|---|---|
| Busca (GET e POST) | 10 | 1..100 |
| Recentes | 10 | 1..100 |
| Briefing e overviews | 10 | 1..100 |
| Handoffs | 50 | 1..200 |
| Sessões | 20 | 1..100 |
| Observações de sessão | 50 | 1..200 |
| `body_max_chars` | 4000 | 200..16384 |

**Paginação:**

- `offset` em sessões e em observações. Observações também retornam `total`.
- `cursor` em páginas recentes, no modo incremental.
- Busca, handoffs e grafo não têm paginação.
- Escopos de busca múltipla são limitados a 25.

**Cache por endpoint:**

| Cabeçalho | Endpoints |
|---|---|
| `private, max-age=30` | Workspaces, projetos, recentes sem `updated_since` nem `cursor`, busca com `q` não vazio, grafo. |
| `private, max-age=60` | Listagem de páginas. |
| `private, max-age=300` e `ETag` | Leitura de página (com `304`). |
| `private, no-store` | Recentes com `updated_since` ou `cursor`, briefing, overviews, handoffs, sessões, observações de sessão e histórico de página. |

Atenção: quando a autorização por repositório está ativa, as respostas com
`max-age` dependem do usuário autenticado. Um navegador pode reutilizar a
resposta do usuário anterior por até o TTL, mesmo após a troca de credenciais na
mesma URL. Respostas que dependem de identidade ou contêm texto derivado de
prompts usam `no-store`.

**Tamanho do corpo.** Esta superfície não define limite próprio de corpo (só
`POST /search` lê corpo). Vale o limite padrão da camada HTTP para corpos JSON,
**2 MiB** (2 097 152 bytes); acima disso a requisição é recusada com `413` em
texto simples. As rotas de máquina (`/mcp`, hooks e workstreams) têm limite de
10 MB. Na injeção de `<base>` (interface customizada ou navegador embutido), o
corpo HTML de **resposta** é lido com teto de 10 MiB (acima: 500).

**Limitação de taxa.** Não há limitador geral nesta superfície. O único
limitador é o de login e recuperação de senha, que responde `429`.

## 6. Hospedagem de interface própria e caminhos base

```bash
ai-memory serve \
    --transport http \
    --bind 127.0.0.1:49374 \
    --enable-web \
    --web-ui-dir /path/to/your-spa/dist
```

O diretório estático é servido em `/web` (ou no slug configurado). Regras:

- **Shell público, dados protegidos.** `index.html`, os fallbacks do roteador
  do cliente e os arquivos estáticos carregam sem credenciais, para que a tela de
  login apareça. `/auth/me`, `/api/v1/*` e `/admin/*` seguem o contrato de sessão
  e Bearer da seção 2. O navegador HTML embutido, usado quando `--web-ui-dir`
  está ausente, também continua protegido, porque renderiza dados no servidor.
- **Fallback de SPA.** Caminhos inexistentes respondem `200` com o `index.html`
  injetado. Isso inclui caminhos de arquivos ausentes: a aplicação não recebe
  `404` para eles e deve tratar essa situação. Assim, um roteador do cliente
  pode tratar `/web/qualquer-coisa` sem `404`.
- **Travessia de caminho** é rejeitada pela proteção padrão do servidor de
  arquivos estáticos.
- **Validação antes de iniciar.** O diretório precisa existir e conter
  `index.html`. Caso contrário, o servidor encerra com erro antes de abrir a
  porta. `--web-ui-dir` sem `--enable-web` também encerra com erro.
- **Injeção de caminho base.** O shell servido recebe `<base href="...">` e
  `<meta name="ai-memory-base-path" content="...">`. Isso vale para `/web`,
  `/web/index.html` e os caminhos de fallback. Arquivos estáticos são servidos
  sem alteração.

Quando um proxy reverso mantém o ai-memory sob um subcaminho de URL, defina
`--base-path` (ou `AI_MEMORY_BASE_PATH`). Todas as superfícies HTTP passam a
ficar sob esse prefixo:

```bash
ai-memory serve \
    --transport http \
    --bind 127.0.0.1:49374 \
    --enable-web \
    --base-path /wiki
```

Com `--base-path /wiki`, a API fica em `/wiki/api/v1`, o MCP em `/wiki/mcp`, os
hooks em `/wiki/hook`, as rotas de administração em `/wiki/admin/*` e a
interface embutida em `/wiki/web`. Use `--web-slug /` (ou
`AI_MEMORY_WEB_SLUG=/`) para montar a interface ou a SPA na raiz do prefixo
(`/wiki`), em vez de `/wiki/web`. O padrão do slug é `/web`.

**Regras de segurança do caminho base.** `--base-path` e `--web-slug` passam pelo
mesmo normalizador:

- Espaços nas pontas são removidos. Barras no início e no fim são ignoradas, e
  barras duplicadas internas são colapsadas.
- Cada segmento deve conter apenas caracteres não reservados da RFC 3986
  (`A-Z`, `a-z`, `0-9`, `-`, `.`, `_`, `~`).

Três situações reduzem o prefixo para `""` (montagem na raiz), com um aviso
`WARN` na inicialização:

- Segmentos `.` ou `..`.
- Qualquer caractere fora do conjunto não reservado (espaços, `<`, `"` etc.).
- Entrada vazia ou só com espaços.

O redirecionamento de barra final `{base_path}{web_slug}/` para
`{base_path}{web_slug}` é permanente (status **308**, que preserva o método) e
preserva a query string; vale só para o navegador embutido (a interface
customizada responde 200 na barra final). Fragmentos são do lado do cliente e
nunca chegam ao servidor.

Sem `--web-ui-dir`, o navegador HTML embutido é o padrão: renderização somente
leitura, busca em texto completo e árvore de projetos.

## 7. Exemplo mínimo: consumo a partir de um cliente

O exemplo usa pseudocódigo de cliente web.

```js
// Resolve a base a partir do shell injetado pelo ai-memory. A meta fica vazia
// na raiz do host e recebe algo como "/wiki" atrás de um proxy com subcaminho.
const basePath = document
  .querySelector('meta[name="ai-memory-base-path"]')
  ?.getAttribute("content") ?? "";
const origin = `${location.origin}${basePath}`;
const API = `${origin}/api/v1`;

async function login(username, password) {
  const resp = await fetch(`${origin}/auth/login`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username, password }),
  });
  if (!resp.ok) throw new Error(`login failed: ${resp.status}`);
  return resp.json();
}

function cookie(name) {
  const prefix = `${encodeURIComponent(name)}=`;
  return document.cookie
    .split(";")
    .map(value => value.trim())
    .find(value => value.startsWith(prefix))
    ?.slice(prefix.length);
}

async function apiGet(path, params) {
  const url = new URL(`${API}${path}`, location.origin);
  if (params) Object.entries(params).forEach(([key, value]) =>
    value != null && url.searchParams.set(key, value));
  const resp = await fetch(url, {
    credentials: "include",
    headers: { Accept: "application/json" },
  });
  if (!resp.ok) {
    const { error } = await resp.json().catch(() => ({ error: resp.statusText }));
    throw new Error(`${resp.status}: ${error}`);
  }
  return resp.json();
}

// Chame login() antes. O cookie HttpOnly de sessão não é visível aqui.
const overview = await apiGet("/workspaces/default/projects/ai-memory/overview", {
  limit: 10,
});
console.log(overview.briefing.counts.pages_latest, "pages");

// POST exige CSRF mesmo sendo uma operação de leitura.
const hits = await fetch(`${API}/search`, {
  method: "POST",
  credentials: "include",
  headers: {
    "Content-Type": "application/json",
    "X-CSRF-Token": decodeURIComponent(cookie("ai_memory_csrf") ?? ""),
  },
  body: JSON.stringify({
    q: "karpathy",
    scopes: [
      { workspace: "default", project: "ai-memory" },
      { workspace: "default", project: "shared-notes" },
    ],
    limit: 20,
  }),
}).then(response => response.json());
```

Teste rápido por linha de comando, com token de máquina:

```bash
TOKEN=$(ai-memory generate-auth-token)
curl -fsS "http://127.0.0.1:49374/api/v1/workspaces" -H "Authorization: Bearer $TOKEN" | jq
```

## 8. Referência

Se o comportamento do servidor divergir deste documento, o servidor é a
referência e este documento deve ser corrigido. Para os nomes das ferramentas
MCP e dos comandos de linha de comando, consulte
[mcp-server.md](mcp-server.md) e [cli.md](cli.md). Para o ciclo de vida de
usuários e credenciais, consulte [users.md](users.md).

## 9. CORS

`/api/v1` aceita requisições de origens cruzadas quando o operador configura
a lista de origens permitidas. A camada CORS vale só para esse roteador:
`/mcp`, hooks, `/admin/*` e `/web` continuam de mesma origem.

Configuração, por qualquer uma das formas:

- `--cors-allow-origin <origem>`, repetível, no subcomando `serve`.
- `AI_MEMORY_CORS_ALLOW_ORIGINS=<lista separada por vírgula>` no ambiente.

Origens do arquivo de configuração e da linha de comando são combinadas, sem
duplicatas, com a ordem preservada.

Validação na inicialização. Uma origem inválida impede o servidor de iniciar:

- Precisa começar com `http://` ou `https://`.
- Não pode terminar com `/`.
- Não pode ser `*`.

Use apenas `esquema://host[:porta]`. A validação não rejeita caminho nem query
string; não os inclua.

Parâmetros da resposta CORS:

- Métodos permitidos: `GET`, `POST` e `OPTIONS`.
- Cabeçalhos permitidos: `Authorization`, `Content-Type` e `X-CSRF-Token`.
- Credenciais permitidas (cookies de sessão são enviados).
- Cache de preflight: 600 segundos.

Misturar `http://` e `https://` na lista é permitido.

## 10. Lacunas conhecidas e não objetivos

- **Sem escrita, por desenho.** A wiki registra o que um projeto produziu, a
  partir de sumarização automática de observações capturadas. Recuperação,
  proveniência e trilha de auditoria dependem de ninguém ter ajustado o conteúdo
  depois. Editar uma página à mão faz a wiki responder "o que alguém quis que
  dissesse", sem como distinguir os dois casos depois.

  Isso não proíbe a contribuição humana. A ferramenta MCP `memory_write_page`
  registra anotações humanas duráveis na mesma linhagem de todo o resto: uma
  alteração do corpo gera uma nova versão e substitui a anterior. A distinção é
  entre acrescentar ao registro pelo caminho normal e editá-lo de fora. Uma wiki
  editável por humanos é um produto separado.
- **Sem limitador geral de taxa.** Requisições autenticadas abusivas não são
  limitadas por esta superfície. Apenas login e recuperação de senha limitam
  tentativas (`429`).
- **Mensagem interna em erros de autenticação.** Em algumas falhas internas das
  rotas `/auth/*`, o corpo `500` inclui a mensagem do erro interno. Em
  `/api/v1`, o corpo é sempre genérico (seção 3).
- **Cache com identidade.** Respostas com `max-age` podem ser reutilizadas pelo
  navegador após troca de credenciais na mesma URL, pelo TTL indicado na seção 5.
- **Briefing sem bloco `settled`.** O bloco opcional de páginas mais estabelecidas
  não é retornado por esta API.
