# Nota: contrato do domínio

Fontes (caminhos relativos à raiz do repo):
- `docs/domain-vocabulary.md` (DV)
- `docs/overview.md` (OV)
- `docs/database-schema.md` (DB)
- `specs/features/001-nucleo/spec.md` (SPEC)
- `specs/features/001-nucleo/plan.md` (PLAN)
- `.github/memory/constitution.md` (CONST)

> **Auditoria de 2026-10-18, passada 2.** Lacunas fechadas pelos docs revisados (que vencem esta nota):
> - **`PageKind` na busca** (a.1): é o `kind` do frontmatter usado como está (sem aparar, caixa
>   sensível); sem ele, o tipo vem do prefixo do caminho (`_rules/`→rule, `_slots/`→slot,
>   `sessions/`→session, `decisions/`→decision, `gotchas/`→gotcha, `concepts/`→concept,
>   `procedures/`→procedure, `notes/`→note, `profile/`→preference, comparado sem distinguir caixa ASCII),
>   senão `fact` (SR §11.5). As regras de 64 caracteres/aparar de DV §6.6 valem para os metadados públicos
>   de escrita, não para a leitura do tipo na busca.
> - **Segmentos reservados** (linha 48): não existem na validação de caminho de referência (DV §4.2);
>   decisão pendente T108.
> - **Nomes não portáveis** (linhas 149–150): lista completa em DV §4.2 (`ensure_portable`), que é uma
>   validação **separada**, aplicada só a caminho novo na escrita de página; o construtor só faz a
>   validação básica e recusa `\` (não converte).

## (a) Tipos de página

"Tipo de página" tem dois sentidos nas fontes. Registro os dois e não os fundo.

**a.1 `PageKind`**
- PLAN §2.1 lista `Tier`, `PageKind` e `ObservationKind` como tipos do domínio. Nenhuma fonte define o conjunto de valores de `PageKind`.
- LACUNA: os valores de `PageKind` não estão definidos em nenhuma das seis fontes.
- Os dois campos abaixo existem no contrato, mas não são um enum fechado.
  - DV §6.6 define `kind` nos metadados públicos de escrita como texto livre, até 64 caracteres, sem caractere de controle, aparado. Vazio significa ausente.
  - DV §14 usa o `kind` do frontmatter para derivar o tipo OKF quando vale `fact`, `note`, `procedure` ou `decision`.
- LACUNA: não está dito se `PageKind` equivale ao `kind` do frontmatter nem se é restrito aos quatro valores do OKF.

**a.2 Tipo derivado OKF (DV §14)**
- A derivação segue esta ordem:
  1. `kind` do frontmatter vale `fact`, `note`, `procedure` ou `decision`. O tipo é `Fact`, `Note`, `Procedure` ou `Decision`.
  2. Senão, `slot_kind` vale `invariant` ou `state`. O tipo é `Invariant` ou `State`.
  3. Senão, vale a família do caminho, isto é, o primeiro segmento.

| Família (1º segmento) | Tipo |
|---|---|
| `sessions` | Session Summary |
| `_rules` | Rule |
| `gotchas` | Gotcha |
| `decisions` | Decision |
| `procedures` | Procedure |
| `concepts` | Concept |
| `notes` | Note |
| `runbooks` | Runbook |
| `_slots` | State |
| `_lint` | Lint Report |
| `_pending` | Pending Note |
| qualquer outra | Note |

- "Conforme" significa ter `type` não vazio. A conformação do frontmatter é determinística e idempotente (DV §14).
- Nomes reservados de diretório: o prefixo `_` segue a convenção de nomes reservados da wiki, como `_pending/` e `_lint/` (DV §2).
- Layout em disco da wiki: `_meta.md`, `index.md`, `log.md`, `sessions/`, `concepts/`, `decisions/` etc. (PLAN §2.4).
- PLAN §2.1 cita "segmentos reservados" na recusa do `ParsePagePath`.
- LACUNA: a lista de segmentos reservados que `ParsePagePath` recusa não é definida. Não confundir com a convenção `_` do DV §2, que reserva nomes de projeto e de diretório de wiki.

**a.3 Metadados de escrita (DV §6.6)**
- `entities`: até 10 nomes, cada um com até 64 caracteres brutos, validados antes da normalização.
- `abstract` (L0): até 1024 caracteres, aparado.
- `relations`: até 32 alvos, cada um com até 1024 caracteres, sem caractere de controle.
- Alvo de relação: `caminho`, `projeto:caminho` ou `workspace/projeto:caminho`. Os componentes de escopo não podem estar vazios nem ter espaço nas bordas, e `.` e `..` são proibidos.
- Extensão do caminho: se o último segmento não tem extensão, acrescenta-se `.md`. Outra extensão é rejeitada. O caminho final precisa passar em `ensure_portable`.

**a.4 Relações e links (DV §6.8, DB §1)**
- Vocabulário fechado de relações: `causes`, `fixes`, `contradicts`.
- Relação desconhecida não é erro. O link vira referência simples.
- Valor padrão de `link_type` no schema: `references` (DB `links`).

## (b) Tiers

Fonte: DV §6.1. O tipo se chama `Tier` e tem quatro valores em minúsculas.

| Valor | Significado (DV §6.1) |
|---|---|
| `working` | Sessão atual: últimas observações e arquivos em uso. |
| `episodic` | Resumos por sessão, com etiquetas de conceito e arquivos tocados. |
| `semantic` | Fatos, preferências e notas de arquitetura destiladas. São as páginas da wiki propriamente ditas. |
| `procedural` | Padrões repetidos extraídos de agrupamentos episódicos. |

- DB `pages.tier` tem `CHECK (tier IN ('working','episodic','semantic','procedural'))`, fechado, sem valor padrão.
- PLAN §2.1 confirma `Tier` com esses quatro valores.
- DV §3: conversor de texto para conjunto fechado devolve "registro malformado" para valor desconhecido.
- DV §6.1: o nível define a política de retenção, que está em `storage-and-retrieval.md`.
- LACUNA: `storage-and-retrieval.md` não estava entre as fontes pedidas. A política de retenção por tier não está coberta aqui.
- DV §6.1: na saída estruturada do consolidador (LLM, F003) o conjunto é restrito ao esquema. Isso não se aplica à feature 001, que é zero-LLM.
- Relação com o OKF: não há mapeamento definido entre tier e tipo OKF.

## (c) Kinds de observação

**c.1 `ObservationKind` (DV §5.1)**
- Forma canônica em kebab-case, 10 valores: `session-start`, `user-prompt`, `pre-tool-use`, `post-tool-use`, `pre-compact`, `post-compaction`, `notification`, `stop`, `session-end`, `other`.
- A conversão também aceita snake_case e PascalCase. Os exemplos citados são `PostToolUse`, `session_start` e `UserPromptSubmit` (este último para `user-prompt`).
- Valor desconhecido gera "registro malformado". Aqui o conversor é estrito, diferente do `AgentKind`.
- LACUNA: o mapeamento exato de cada alias PascalCase para o valor canônico só tem os 3 exemplos citados. Não inventar os demais. Há uma ambiguidade: `PreCompact` e `PostCompaction` são nomes naturais, mas a fonte não os lista.

**c.2 Esquema (DB §2)**
- `observations.kind` é `TEXT NOT NULL` sem `CHECK`. O comentário diz "texto livre no esquema". A validação do conjunto fica na aplicação.

**c.3 Campos de uma observação (DV §5.2)**
- Escopo: `session_id`, `workspace_id` e `project_id`.
- `kind`: um dos valores de c.1.
- `extension` e `source_event` preservam vocabulário de terceiros sem expandir o conjunto de kinds. Ficam ausentes quando o evento não declara namespace.
- `title` e `body` chegam já sanitizados.
- `importance`: inteiro de 1 a 10, padrão 5. DB tem `CHECK (importance BETWEEN 1 AND 10) DEFAULT 5`.
  - A ingestão de hooks atribui valores por evento: sessão 7, fim de sessão 7, prompt 8, uso de ferramenta 5, parada e compactação 6, notificação e demais 3.
  - Isso é feature 002 e está fora do escopo da 001.
- `occurred_at`: em microssegundos. Ausente significa "agora", preenchido no momento da gravação.
- A observação materializada acrescenta o identificador e o instante de criação (`created_at`).

**c.4 Tipo sanitizado (DV §7.4, OV invariante 6, CONST INV-06, PLAN P-07)**
- O único construtor cria o tipo `Sanitized`, e o campo interno é não exportado.
- Ordem: sanitiza o título e depois aplica o teto de 80 caracteres exibidos, com reticências ao cortar. Sanitiza o corpo e depois aplica o teto de 16 KiB (16×1024 bytes).
- Truncamento do corpo (DV §7.4):
  - Corte de cabeça e cauda. Cada ponta recebe metade do orçamento útil, que é o teto menos 48 bytes reservados ao marcador.
  - O marcador é `\n...[truncated N bytes]...\n`, onde N é o número de bytes omitidos.
  - Teto abaixo de 64 bytes: corta só a cabeça.
  - Nunca corta no meio de caractere multibyte.
- A observação armazenada usa `Title` e `Body` do tipo `Sanitized` (PLAN §2.1).
- Pipeline de sanitização (DV §7.1):
  1. Remoção de controle: escapes de terminal, C0 e C1 exceto tab, LF e CR, DEL, e marcadores bidi.
  2. Padrões em ordem, do mais específico ao mais geral. Cada trecho vira `[REDACTED:<rótulo>]`, salvo se contiver item da allowlist.
- LACUNA: DV §7.2 lista só rótulos e descrições dos padrões, não as regexes. As regexes estão em `docs/prompts-schemas-and-sanitizer.md`, que está ausente (SPEC "Débitos"). A lista de padrões será reescrita a partir dos docs e de fixtures próprias.
- LACUNA: DV §7.4 diz que o teto de título é de "80 caracteres exibidos". O PLAN P-07 diz que trunca "em fronteira de rune". "Exibidos" (grafemas) e "rune" não são a mesma coisa. Esta ambiguidade precisa de decisão.
- LACUNA: o PLAN menciona limites de entidade e as regras do DV §7, mas "Limits" em `sanitize.Text(raw, limits Limits)` não tem os campos definidos.

## (d) Regras de `PagePath`

Fontes: DV §4.2, PLAN §2.1, PLAN §4 (anti-padrões), CONST (anti-padrão "concatenar caminho à mão"), DB `pages.path`.

**d.1 Regras obrigatórias na construção (DV §4.2)**
O caminho deve satisfazer todas estas condições:
- Não é vazio.
- Não começa com `/`.
- Não tem prefixo de unidade do Windows (`C:/…`).
- Não usa `\` como separador.
- Nenhum segmento é vazio, `.` ou `..`.

A validação ocorre na construção, então o armazenamento pode tratar o caminho como chave plana.

**d.2 Normalização POSIX e rejeição de `..` (PLAN §2.1)**
- `ParsePagePath(string) (PagePath, error)` "normaliza separador POSIX, recusa `..`, caminhos absolutos, segmentos reservados e nomes não portáveis no Windows (`CON`, `NUL`, `:`, `?`, ponto/espaço no fim)".
- DB `pages.path` é "relativo à raiz da wiki, separador POSIX".
- CONST proíbe concatenar caminho de página à mão. É obrigatório usar o tipo do domínio. PLAN §4 acrescenta `wiki.pathFor(scope, path)`.
- LACUNA (conflito entre fontes): DV §4.2 exige que o `PagePath` rejeite `\` como separador. PLAN §2.1 fala em "normaliza separador POSIX".
  - Não está dito se `\` é convertido em `/` ou rejeitado.
  - A leitura mais literal do DV é rejeitar. O PLAN sugere conversão.
  - Decisão pendente para o Analyst.

**d.3 Nomes portáveis do Windows (`ensure_portable`)**
- DV §4.2 trata `ensure_portable` como validação separada e mais estrita. Ela garante que o arquivo possa ser materializado em qualquer SO, por exemplo contra nomes reservados do Windows.
- Aplicação (DV §4.2):
  - Só é aplicada quando um caminho novo entra no sistema, não na leitura de linhas já persistidas.
  - Motivo: apertar a validação de leitura tornaria ilegível uma listagem inteira por causa de uma única página antiga.
  - A regra é a mesma em todas as plataformas, de propósito. Uma wiki criada em Linux precisa ser utilizável no Windows com a mesma versão.
- Lista citada no PLAN §2.1: `CON`, `NUL`, `:`, `?`, ponto ou espaço no fim.
- LACUNA: a lista completa de nomes reservados do Windows não é dada. Os exemplos são `CON` e `NUL`. Não estão listados `PRN`, `AUX`, `COM1..9`, `LPT1..9` nem se vale com extensão (`CON.md`). Também faltam os caracteres inválidos `< > " | *` e os caracteres de controle. Não inventar. Precisa de decisão ou de consulta à documentação do Windows.
- LACUNA: DV §4.2 chama a função `ensure_portable` e o PLAN §2.1 não a nomeia. Não está claro se `ParsePagePath` a incorpora ou se ela é uma chamada separada. O PLAN diz que `ParsePagePath` já recusa nomes não portáveis, o que contrasta com "separada" no DV.
- LACUNA: DV §4.2 diz que a validação de leitura (sem `ensure_portable`) deve continuar tolerante para linhas antigas. O PLAN não diz como isso se traduz na API Go, por exemplo dois construtores.

**d.4 Colisão de caixa (`portable_page_key`, DV §4.2)**
- Dois caminhos com a mesma chave são o mesmo arquivo em sistemas que dobram caixa ou normalizam Unicode (macOS e Windows).
- A chave sai em duas etapas, nesta ordem: 1) minúsculas; 2) normalização Unicode NFC.
- A ordem importa. A letra `İ` em minúscula vira `i` seguido de U+0307, que só se recompõe depois da conversão para minúsculas.
- PLAN §2.1 chama o método de `PortableKey()`.
- PLAN P-04, caso C: a escrita de caminho novo checa colisão por `PortableKey` e levanta `ErrPathCollision`.
- LACUNA: não está especificado como armazenar ou indexar a `PortableKey`. DB não tem coluna para ela, e o único índice de unicidade é `idx_pages_latest_path (workspace_id, project_id, path) WHERE is_latest = 1`. O PLAN diz que `ErrPathCollision` existe, mas não diz se a checagem é por consulta ou por coluna nova. O PLAN fixa a V01 como o estado final do `database-schema.md`, o que sugere consulta em tempo de escrita.
- LACUNA: DV não diz se a minúscula é Unicode completa (`strings.ToLower`) ou ASCII. O exemplo com `İ` indica Unicode completo.

**d.5 Identidade de 3 elementos (OV §4 inv. 4, CONST INV-04, PLAN §2.1)**
- `Scope{WorkspaceID, ProjectID ID}`.
- `PageRef{Scope, Path PagePath}`.
- `ID [16]byte` é UUIDv7, com `NewID()`, `ParseID` e `String()`, armazenado como `BLOB` de 16 bytes (DB §0).
- Cada tipo de identificador é distinto (DV §4.1). Esta feature usa pelo menos `WorkspaceId`, `ProjectId`, `SessionId`, `ObservationId` e `PageId`.
- `PageId` identifica uma versão da página, não o caminho (DV §4.1).
- LACUNA: PLAN §2.1 define um único `ID` e DV §4.1 exige tipos distintos por entidade. Não está claro se o Go terá tipos nomeados (`type ProjectID ID`) ou apenas `ID`.
- LACUNA: DV §4.1 exige "leitura de bytes exige exatamente 16 bytes". PLAN §2.1 não cita `ParseID` sobre bytes.

## (e) Chaves portáveis (`PortableKey`) e `SearchText`

**e.1 `PortableKey()`**
- Definição: `strings.ToLower` e depois normalização NFC, nesta ordem (DV §4.2, PLAN §2.1). Detalhes e a lacuna de armazenamento estão em d.4.
- Dependência: a normalização NFC exige `golang.org/x/text/unicode/norm`.
- LACUNA: o PLAN §1 e a CONST não listam `x/text`. A CONST diz que dependência nova só entra se listada em `.github/memory/tech.md`, que não foi lido (não estava nas fontes). Verificar `tech.md` antes de usar. A CONST pede "preferir stdlib".

**e.2 `SearchText()` (coluna `path_search`)**
- Origem: DB §1, PLAN §2.1 ("`SearchText()` calcula `path_search` pela regra do doc").
- Regra do doc (DB §1):
  1. Troque `/` e `.` por espaço, produzindo `segmentos`.
  2. Troque também `-` e `_` por espaço em `segmentos`, produzindo `palavras`.
  3. Grave `segmentos || ' ' || palavras`.
- Exemplo oficial: `notes/foo-bar.md` vira `notes foo-bar md notes foo bar md`.
- Recalcular a cada mudança de `path`.
- Uso: `path_search` alimenta o FTS5 em `pages_fts(title, body, path_search)`.
  - Tokenizer: `unicode61 remove_diacritics 2 tokenchars '/_-'`, ou seja, `/`, `_` e `-` são caracteres de token (DB §1.1).
  - Coluna: `path_search TEXT NOT NULL DEFAULT ''`.
- LACUNA: a regra não diz se espaços repetidos são colapsados. No exemplo, `notes/foo-bar.md` não gera espaços duplos, mas um caminho com `-` seguido de `/` geraria. Seguir literalmente (sem colapsar) a menos que o Analyst decida outra coisa.
- LACUNA: a regra não diz se há trim das pontas nem se a caixa é preservada. O exemplo preserva a caixa.

## Contexto adicional útil para outras tarefas

**Entidades (DV §6.5)**
- Máximo de 64 caracteres por entidade (rejeita, não trunca) e de 10 por página.
- Normalização: aparar espaços, colapsar espaços internos, minúsculas, rejeitar vazio ou com caractere de controle ou acima de 64.
- Listas: descarta as inválidas, deduplica preservando a ordem e corta em 10.
- DB: `entities.name CHECK (length(name) BETWEEN 1 AND 64)`. `UNIQUE (workspace_id, project_id, name)`.
- LACUNA: "64 caracteres" no DV e `length(name)` no SQLite contam caracteres, mas o DV não diz se "caracteres" são runes.

**Evidência (DV §6.4, DB §4)**
- `source_kind IN ('session','observation','feedback','reconsolidation')`.
- Gravada na mesma transação da página. Citar a mesma fonte duas vezes não duplica.
- É só registrada e exposta; não entra no ranqueamento.

**TTL / `expires_at` (DV §6.7)**
- Aceita RFC 3339 ou `YYYY-MM-DD`, este último significando o fim do dia em UTC (23:59:59,999999).
- Banco: `INTEGER` em microssegundos.

**Versionamento (DV §6.3, DB §1)**
- Atualização cria nova linha com `is_latest = 1` e `supersedes` apontando para a anterior.
- A anterior vira `is_latest = 0`, com `valid_to` igual ao instante da nova.
- Índice único parcial `idx_pages_latest_path ON pages(workspace_id, project_id, path) WHERE is_latest = 1`.
- `body_sha256` é SHA-256 do corpo, `BLOB` de 32 bytes.

**Constantes globais (DV §2)**
- Workspace padrão: `default`.
- Projeto de fallback: `scratch`.
- Projeto de escopo global: `_global`.

**Convenções de armazenamento (DB §0)**
- IDs em `BLOB` de 16 bytes.
- Timestamps em microssegundos Unix UTC.
- Hashes em 32 bytes.
- Booleanos como `INTEGER` com `CHECK`.
- Listas e objetos como `TEXT` JSON.

**Itens da V01 relevantes à 001 que contrastam com o DB**
- PLAN §2.3 mantém `sessions.agent_kind` sem `CHECK` fechado (desvio deliberado).
- Tabelas da 001: `workspaces`, `projects`, `pages` + `pages_fts`, `links`, `entities`, `entity_page_links`, `page_access`, `page_evidence`, `audit_log`, `sessions`, `observations` + `observations_fts`.

**Divergências entre fontes a anotar**
- Controle de migração: DB §0 usa a tabela `refinery_schema_history`. O PLAN P-01 usa `schema_migrations`. O PLAN prevalece para o projeto (o próprio DB diz que o nome é "a validar").
- Versão do Go: a task do projeto descreve Go 1.25 (contexto do agente) e o PLAN §1 diz Go 1.26. Divergência a registrar, fora do escopo desta nota.

## Resumo de LACUNAs

1. Valores de `PageKind` não definidos.
2. Lista de "segmentos reservados" de `ParsePagePath` não definida.
3. `\` é rejeitado (DV) ou convertido em `/` (PLAN)?
4. Lista completa de nomes reservados do Windows e caracteres inválidos não dada. Só `CON`, `NUL`, `:`, `?`, ponto ou espaço no fim.
5. `ensure_portable` é separada do `ParsePagePath` (DV) ou embutida (PLAN)? Como manter a leitura tolerante?
6. Armazenamento e checagem da `PortableKey` (coluna, índice ou consulta) não especificados.
7. Dependência `golang.org/x/text/unicode/norm` não consta nas fontes. Conferir `tech.md`.
8. Detalhes de `SearchText` (colapso de espaços, trim) não especificados.
9. Aliases PascalCase e snake_case de `ObservationKind` só têm 3 exemplos.
10. "80 caracteres exibidos" (DV) contra "fronteira de rune" (PLAN).
11. Regexes do sanitizador ausentes (`docs-adiados/` ausente na época; hoje em `docs/prompts-schemas-and-sanitizer.md` §11; alinhamento na 002/T093). Campos de `Limits` não definidos.
12. Tipos de ID distintos (DV) contra um único `ID` (PLAN).
13. Política de retenção por tier está em `storage-and-retrieval.md`, não lido.
14. Mapeamento tier para tipo OKF não existe.
