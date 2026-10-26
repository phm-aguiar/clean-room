# Nota: contrato do domínio

Fontes (caminhos relativos à raiz do repo):
- `docs/domain-vocabulary.md` (DV)
- `docs/overview.md` (OV)
- `docs/database-schema.md` (DB)
- `docs/storage-and-retrieval.md` (SR)
- `docs/prompts-schemas-and-sanitizer.md` (PSS)
- `specs/features/001-nucleo/spec.md` (SPEC)
- `.github/memory/constitution.md` (CONST)

Regra: os docs vencem esta nota, e em toda dúvida vale o comportamento do original (inclusive defeitos
conhecidos). Os nomes Go citados (`PagePath`, `PortableKey` etc.) são convenção futura de Go 1.26, não
contrato.

## (a) Tipos de página

O original **não tem** um tipo fechado de "tipo de página" no domínio (não há `PageKind` enumerado). O
"tipo" de uma página aparece em três formas, todas derivadas e nenhuma restrita:

1. **`kind` do frontmatter**: texto livre (DV §6.6: até 64 caracteres, sem controle, aparado nos metadados
   públicos de escrita; vazio = ausente). Não é validado contra lista.
2. **Tipo de ranking** (SR §11.5): o `kind` do frontmatter usado como está (sem aparar, sensível à caixa),
   convertido para texto quando presente e não nulo (qualquer valor, inclusive número); senão o prefixo do
   caminho (`_rules/` → `rule`, `_slots/` → `slot`, `sessions/` → `session`, `decisions/` → `decision`,
   `gotchas/` → `gotcha`, `concepts/` → `concept`, `procedures/` → `procedure`, `notes/` → `note`,
   `profile/` → `preference`, sem distinguir caixa ASCII); senão `fact`. Só os valores da tabela de
   autoridade mudam o fator; os outros são neutros.
3. **`type` OKF** (DV §14, `docs/okf.md`): `kind` igual a `fact`, `note`, `procedure` ou `decision`
   (comparação exata) vira `Fact`, `Note`, `Procedure` ou `Decision`; senão `slot_kind` `invariant`/`state`
   vira `Invariant`/`State`; senão o primeiro segmento do caminho (tabela abaixo); senão `Note`. Um `type`
   já presente e não vazio é mantido.

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
- O prefixo `_` é convenção de nomes da wiki (`_pending/`, `_lint/`, DV §2). **Não há** lista de nomes
  reservados na validação de caminho de página (DV §4.2): `_meta.md`, `bootstrap.md`, ledgers e `_pending/`
  são excluídos pelo watcher e pela reindexação, não pela validação. `index.md` é página comum.

**Metadados de escrita (DV §6.6)**
- `entities`: até 10 nomes, cada um com até 64 caracteres brutos, validados antes da normalização.
- `abstract` (L0): até 1024 caracteres, aparado.
- `relations`: até 32 alvos, cada um com até 1024 caracteres, sem caractere de controle; alvo gravado como enviado.
- Alvo de relação: `caminho`, `projeto:caminho` ou `workspace/projeto:caminho`. Os componentes de escopo não podem estar vazios nem ter espaço nas bordas, e `.` e `..` são proibidos.
- Extensão do caminho: se o último segmento não tem extensão, acrescenta-se `.md`. Outra extensão é rejeitada. O caminho final precisa passar em `ensure_portable`.

**Relações e links (DV §6.8, DB §1, `docs/typed-edges.md`)**
- Vocabulário fechado de relações: `causes`, `fixes`, `contradicts`.
- Chave de relação desconhecida no frontmatter é **ignorada com aviso** (`docs/wiki-files-layer.md` §3.5).
- Link de corpo grava `references`, o valor padrão de `link_type` no schema.

## (b) Tiers

Fonte: DV §6.1. Quatro valores em minúsculas.

| Valor | Significado (DV §6.1) |
|---|---|
| `working` | Sessão atual: últimas observações e arquivos em uso. |
| `episodic` | Resumos por sessão, com etiquetas de conceito e arquivos tocados. |
| `semantic` | Fatos, preferências e notas de arquitetura destiladas. São as páginas da wiki propriamente ditas. |
| `procedural` | Padrões repetidos extraídos de agrupamentos episódicos. |

- DB `pages.tier` tem `CHECK (tier IN ('working','episodic','semantic','procedural'))`, fechado, sem valor padrão.
- DV §3: conversor de texto para conjunto fechado devolve "registro malformado" para valor desconhecido.
- Na reindexação sem `tier` no frontmatter, o padrão é `semantic` (`docs/wiki-files-layer.md` §5.5).
- A política de retenção por tier está em SR §12 (feature 004). Não há mapeamento entre tier e tipo OKF.

## (c) Tipos de observação

**c.1 Vocabulário (DV §5.1)**
- Forma canônica em kebab-case, 10 valores: `session-start`, `user-prompt`, `pre-tool-use`, `post-tool-use`, `pre-compact`, `post-compaction`, `notification`, `stop`, `session-end`, `other`.
- Grafias aceitas na leitura: as listadas em DV §5.1 (conferidas contra o original na auditoria de 2026-10-25). Valor desconhecido gera "registro malformado".

**c.2 Esquema (DB §2)**
- `observations.kind` é `TEXT NOT NULL` sem `CHECK`. A validação do conjunto fica na aplicação.

**c.3 Campos de uma observação (DV §5.2)**
- Escopo: `session_id`, `workspace_id` e `project_id`.
- `kind`: um dos valores de c.1.
- `extension` e `source_event` preservam vocabulário de terceiros sem expandir o conjunto. Ficam ausentes quando o evento não declara namespace.
- `title` e `body` chegam já sanitizados.
- `importance`: inteiro de 1 a 10, padrão 5 (`CHECK (importance BETWEEN 1 AND 10) DEFAULT 5`). Os valores por evento de hook são da feature 002.
- `occurred_at`: em microssegundos. Ausente significa "agora", preenchido no momento da gravação.
- A observação materializada acrescenta o identificador e o instante de criação (`created_at`).

**c.4 Tipo sanitizado (PSS §11, OV invariante 6, CONST INV-06)**
- A barreira de tipo existe **só para observações**: o tipo sanitizado tem valor interno não exportado e só a função de sanitização o constrói. Páginas, handoff manual, mensagens, motivo de feedback e eventos de workstream são higienizados pelo chamador, sem tipo; a reindexação não higieniza.
- Pipeline e limites: PSS §11.1 (passada única; remoção de controle com as faixas exatas do doc; padrões na ordem da tabela de PSS §11.2, que tem 27 padrões; título de observação ≤ 80 caracteres com reticências dentro do teto; corpo ≤ 16 KiB com corte de cabeça e cauda e marcador `\n...[truncated N bytes]...\n`; teto abaixo de 64 bytes corta só a cabeça com reticências `…` dentro do teto; nunca corta no meio de caractere multibyte).
- Classes `\s`, `\d` e `\b` dos padrões são Unicode (PSS §11.2, notas de leitura).
- Allowlist e padrões extras do operador: PSS §11.3, sem validação além de compilar, como no original.

## (d) Regras do caminho de página

**d.1 Validação básica (DV §4.2)**, aplicada na construção e na leitura:
- Não é vazio.
- Não começa com `/`.
- Não tem prefixo de unidade do Windows (`C:/…`).
- `\` é **recusado** (não convertido).
- Nenhum segmento é vazio, `.` ou `..`.

A validação ocorre na construção, então o armazenamento pode tratar o caminho como chave plana. DB
`pages.path` é relativo à raiz da wiki, separador POSIX. CONST proíbe concatenar caminho de página à mão.

**d.2 Portabilidade (`ensure_portable`, DV §4.2)**
- Validação separada e mais estrita, aplicada **só** quando um caminho novo entra pela escrita de página. Leitura, links, reindexação e watcher usam só a validação básica, para não tornar ilegível uma listagem inteira por causa de uma página antiga.
- A regra é a mesma em todas as plataformas. A lista completa (nomes de dispositivo com e sem extensão, incluindo `COM¹`…, espaço antes da extensão, 255 bytes, `git~1`…`git~4`, caracteres inválidos e controles) está em DV §4.2.

**d.3 Colisão de caixa (chave portável, DV §4.2)**
- Dois caminhos com a mesma chave são o mesmo arquivo em sistemas que dobram caixa ou normalizam Unicode.
- A chave sai em duas etapas, nesta ordem: 1) minúsculas Unicode completas (não a conversão simples: `İ` e sigma final seguem a regra do doc); 2) normalização NFC.
- Página nova cuja chave colide com outra página viva do projeto é recusada com o erro de colisão de caminho (texto literal em SR §19). O schema não tem coluna para a chave; o índice único é `idx_pages_latest_path (workspace_id, project_id, path) WHERE is_latest = 1`.

**d.4 Identidade de 3 elementos (OV invariante 4, CONST INV-04, DV §4.1)**
- Identificadores UUIDv7 armazenados como `BLOB` de 16 bytes (DB §0); leitura de bytes exige exatamente 16.
- Cada tipo de identificador é distinto (workspace, projeto, sessão, observação, página). O identificador de página identifica uma versão, não o caminho.

## (e) `path_search`

- Regra do doc (DB §1):
  1. Troque `/` e `.` por espaço, produzindo `segmentos`.
  2. Troque também `-` e `_` por espaço em `segmentos`, produzindo `palavras`.
  3. Grave `segmentos || ' ' || palavras`, literalmente (sem colapsar espaços nem aparar; caixa preservada).
- Exemplo oficial: `notes/foo-bar.md` vira `notes foo-bar md notes foo bar md`.
- Recalcular a cada mudança de `path`. Alimenta `pages_fts(title, body, path_search)`, tokenizer `unicode61 remove_diacritics 2 tokenchars '/_-'` (DB §1.1).

## Contexto adicional

**Entidades (DV §6.5)**: até 64 caracteres por entidade (rejeita, não trunca) e 10 por página; aparar, colapsar espaços internos, minúsculas; descarta inválidas, deduplica preservando a ordem. DB: `entities.name CHECK (length(name) BETWEEN 1 AND 64)`, `UNIQUE (workspace_id, project_id, name)`.

**Evidência (DV §6.4, DB §4)**: `source_kind IN ('session','observation','feedback','reconsolidation')`; gravada na mesma transação da página; citar a mesma fonte duas vezes não duplica; não entra no ranqueamento.

**TTL / `expires_at` (DV §6.7)**: RFC 3339 com deslocamento obrigatório ou `YYYY-MM-DD` (fim do dia em UTC, 23:59:59,999999); banco em microssegundos; gravado na escrita de página.

**Versionamento (DV §6.3, DB §1)**: atualização cria nova linha com `is_latest = 1` e `supersedes` apontando para a anterior; a anterior vira `is_latest = 0`, com `valid_to` igual ao instante da nova. `body_sha256` é SHA-256 do corpo, `BLOB` de 32 bytes.

**Constantes globais (DV §2)**: workspace padrão `default`; projeto de fallback `scratch`; projeto de escopo global `_global`.

**Convenções de armazenamento (DB §0)**: IDs em `BLOB` de 16 bytes; timestamps em microssegundos Unix UTC; hashes em 32 bytes; booleanos como `INTEGER` com `CHECK`; listas e objetos como `TEXT` JSON. O controle de migração é a tabela `refinery_schema_history`, com os tipos de DB §0. `sessions.agent_kind` segue o schema do original (DB §2).
