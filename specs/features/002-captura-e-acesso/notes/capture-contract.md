# Contrato do marcador `.ai-memory.toml` e da política de captura (feature 002)

Wiki-Keywords: marker-file, capture exclusions, ignore_paths, MetadataOnly

Hierarquia de fontes: `docs/` vence a spec. Siglas de citação: **MF** = `docs/marker-file.md`; **LEI** = `docs/lifecycle-event-ingestion.md`; **SPEC** = `specs/features/002-captura-e-acesso/spec.md`. Em toda dúvida vale o comportamento do original; os vetores de `docs/assets/fixtures/capture-policy.json` (sincronizados com o original) são o teste de aceitação.

## 1. Localização e caminhada (MF "Discovery")

- Nome exato `.ai-memory.toml`, com ponto inicial. Precisa ser arquivo regular; diretório com esse nome é ignorado (MF "File name and location").
- Procurado no cwd e nos ancestrais. O primeiro marcador encontrado vence; campos nunca são mesclados entre arquivos (MF "File name and location").
- Fronteira da caminhada (MF "Walk boundaries"):

| cwd | A caminhada para em |
|---|---|
| Dentro de `$HOME` | `$HOME`, inclusive |
| Fora de `$HOME`, dentro de checkout git | Raiz do checkout, inclusive (ancestral mais próximo com entrada `.git`, arquivo ou diretório) |
| Fora de `$HOME`, fora de checkout | Só o cwd |
| `$HOME` não definido | Raiz do sistema de arquivos |

- **Duas caminhadas** sobre o mesmo conjunto de arquivos (MF "Two walks"):
  - *Scope walk*: pula marcadores que declaram apenas `[capture]`. Marcador "declara algo" se contém, em qualquer lugar, uma das chaves: `workspace`, `project`, `project_strategy`, `drop_subagent_captures`, `identity`, `identity_style`, `aliases`, `server`, `default_global`, `inject_on_session_start`, `max_chars`, `contribute`, `consume`. Detecção por linha, ignora cabeçalhos de seção.
  - *Capture walk*: usada **só** para `[capture]`; pega o marcador mais próximo de **qualquer tipo**, inclusive o que só tem `[capture]`. Esse marcador substitui por completo as regras de captura dos externos.
- Marcador em `$HOME` (home marker) nunca é declaração local para os diretórios abaixo; é só fallback (MF "Home marker"). Rotas `[routes.*]` são só do home marker (MF "Keys"); seguem MF e `docs/assets/fixtures/home_route_cases.json`, como no original.
- Modo denylist (padrão): sem marcador, tudo é capturado. Modo allowlist: a presença de marcador na capture walk é o opt-in; sem marcador nenhum evento é emitido, nem prompts, descartados no processo do hook antes de spool ou rede (MF "Allowlist mode"; LEI §4.1). Persistido por instalação, `install-hooks --capture-mode allowlist` (MF "Allowlist mode").

## 2. Leitura do arquivo (MF "File format and reading")

- **Tamanho**: `[capture]` e inspeção leem no máximo 64 KiB (65 536 bytes); maior que isso é rejeitado ali. Codificação UTF-8 obrigatória.
- **Campos de escopo** (workspace, project etc.): leitura **por linha**, simples, sem TOML completo. Chave lida de linha que começa com o nome da chave, depois `=`. Strings exigem valor entre aspas duplas, terminando na próxima aspa dupla; valor nu não é lido. Flags aceitam token com ou sem aspas, com comentário `#` inline removido. Cabeçalhos de seção não são rastreados; a primeira linha que casa vence, em qualquer posição.
- **TOML estrito**: usado para `[capture]` e para o estado de inspeção. A inspeção compara cada chave string com a leitura estrita; divergência vira `conflicting`.
- **Resolução tolerante, captura estrita**: arquivo com TOML inválido ainda fornece campos de escopo à resolução; só a inspeção o marca `invalid`. A captura de ciclo de vida usa a leitura estrita para `[capture]`, então `[capture]` quebrado torna a política de captura **inválida** (MF "Inspection states", último parágrafo).
- Verdadeiros: `1`, `true`, `yes`, `on`; falsos: `0`, `false`, `no`, `off` (sem distinguir caixa, com ou sem aspas) (MF "File format and reading").
- Estados de inspeção, na ordem: `ignored`, `invalid_home_routes`, `absent`/`invalid`, `unreadable` (>64 KiB ou ilegível), `invalid`, `conflicting`, `valid`/`home_*_route` (MF "Inspection states"). Marcador ilegível como texto na etapa 3 conta como `absent`.
- Chave `drop_subagent_captures` (truthy, raiz) é o nome no MF "Keys" e "Subagent captures". Não há conflito: no original, a chave do arquivo é `drop_subagent_captures` e o parâmetro de consulta de `/hook` é `drop_subagent` (MF; LEI §2.1 e §4.5).

## 3. Seção `[capture]` estrita (MF "Capture exclusions", "Parsing and failure")

```toml
[capture]
ignore_paths = ["private/**", "~/personal-notes/**"]
```

- O marcador mais próximo é autoritativo; seções não se mesclam. `[capture]` ausente ou `ignore_paths` vazio = **inativa**, mantém o comportamento atual.
- `[capture]` aceita **somente** `ignore_paths`. Tornam a política **inteira** inválida (nada é aplicado parcialmente): chave desconhecida; valor que não é array; entrada que não é string; TOML inválido; texto não UTF-8; arquivo > 64 KiB; qualquer padrão que falhe na validação.
- Estados da política para o decisor (LEI §4.3): **Inativa** (sem marcador em denylist), **Inválida** (marcador malformado), **Ativa**.

## 4. Regras dos padrões e normalização no Windows (MF "Pattern rules"; `docs/windows.md`)

- Casa o caminho **inteiro**, normalizado lexicalmente; não é substring.
- `*` casa dentro de um componente; `?` casa um caractere que não é separador; `**` cruza diretórios.
- Padrão relativo tem raiz no diretório do marcador; absoluto é usado como está; `~/` expande para o home.
- Caminhos relativos em eventos de ferramenta de arquivo são resolvidos a partir do cwd do evento.
- **Rejeitados** no padrão: os caracteres `! { } [ ] ( ) | ^ $ %`; a sequência `${`; três ou mais `*` seguidos; segmento `..`; `~` não seguido de `/`; caminhos relativos a drive como `C:foo`; e, quando o diretório do marcador é um caminho Windows (`C:/...` ou UNC), qualquer padrão iniciado por `/` sem letra de drive nem prefixo UNC (`/foo` é rejeitado como caminho relativo à raiz de drive no Windows).
- **Normalização de caminhos Windows na política de captura**:
  1. Remove prefixos de provedor do PowerShell (`Microsoft.PowerShell.Core\FileSystem::` e `FileSystem::`, sem diferenciar maiúsculas).
  2. Converte prefixos estendidos Win32: `\\?\UNC\server\share\...` -> `//server/share/...`; `\\?\C:\...`, `\\.\C:\...` e `\??\C:\...` -> `C:/...`; caminhos de compartilhamento administrativo `\\localhost\C$\...` ou `\\127.0.0.1\c$\...` -> `C:/...`.
  3. Caminhos estilo MSYS2 / Git Bash / Cygwin (`/c/Users/...` ou `/cygdrive/c/Users/...`) geram **dois candidatos**: o caminho POSIX original e a forma com letra de drive `C:/Users/...`.
  4. Em caminhos Windows (`C:/...` ou UNC), remove sufixos de NTFS Alternate Data Streams (`:stream` ou `:$DATA` após o prefixo do drive) e pontos ou espaços finais de cada segmento (`secret.txt... ` -> `secret.txt`), e compara todos os segmentos ASCII sem diferenciar maiúsculas.
  5. Ao tokenizar comandos de shell, além de escapes `\`, remove escapes por acento grave (`` ` ``) do PowerShell e circunflexo (`^`) do `cmd.exe` quando o evento ou os tokens indicarem ambiente Windows.
- Casamento **léxico**: links simbólicos, junctions, bind mounts e aliases não são resolvidos; cada alias visível precisa de padrão próprio.

## 5. Limites (MF "Bounds"; LEI §4.2)

| Limite | Valor |
|---|---|
| Padrões por marcador | 128 |
| Caracteres por padrão | 1 024 |
| Tamanho do marcador | 64 KiB |
| Caminhos candidatos diretos por evento | 32 |
| Caracteres por candidato | 4 096 |
| Comparações padrão x candidato por evento | 1 000 000 |

Esgotar o orçamento de comparações falha fechado: ferramenta de arquivo vira `MetadataOnly`; ferramenta de shell vira `Drop` (MF "Bounds"; LEI §4.2 última frase).

## 6. Decisões Keep / Drop / MetadataOnly

Pergunta que a política responde: manter, descartar ou reduzir a metadados um evento de ferramenta de arquivo ou comando de shell (LEI §4). Tabela consolidada de LEI §4.3 e MF "Decisions":

| Estado | Família da ferramenta | Condição | Decisão |
|---|---|---|---|
| Inativa | qualquer | sem marcador (denylist) ou sem `ignore_paths` | `Keep` (LEI §4.3; MF "Parsing and failure") |
| Ativa | busca ou listagem | sempre | `Drop` conservador (LEI §4.3; MF "Decisions") |
| Ativa | arquivo | algum candidato casa com padrão (ou aponta para arquivo de spool do harness) | `Drop` do evento inteiro (MF "Decisions") |
| Ativa | arquivo | sem caminho extraível, malformado ou não suportado | `MetadataOnly` (LEI §4.3; MF "Decisions") |
| Ativa | arquivo | caminhos extraídos, nenhum casa | `Keep` (LEI §4.3) |
| Ativa | shell | algum argumento casa, orçamento esgotado, `output_reader` em `PostToolUse`, `inline_code`, ou `PreToolUse` sem comando | `Drop` (LEI §4.3, §4.2; MF "Decisions") |
| Ativa | shell | `PostToolUse` sem comando extraível | `MetadataOnly` (LEI §4.3) |
| Ativa | shell | comando extraído, nenhum argumento casa | `Keep` (LEI §4.3) |
| Ativa | demais famílias | sempre | `Keep` (LEI §4.3) |
| Inválida | arquivo ou shell | marcador malformado | `MetadataOnly` (fail-closed) (LEI §4.3; MF "Decisions") |
| Inválida | demais | sempre | `Keep` (LEI §4.3) |

## 7. Corpo substituto de `MetadataOnly`

- LEI §4.3: o payload JSON substituído pelo cliente contém apenas `session_id`, `cwd`, `tool_name` (nome canônico da família ou `"unknown"`), `tool_use_id` (se válido) e, dentro de `tool_input` (`PreToolUse`) ou `tool_response` (`PostToolUse`), o objeto:
  ```json
  {
    "_ai_memory_capture_policy": 1,
    "decision": "metadata_only",
    "tool_family": "<file|search_list|shell|other|unknown>",
    "path_count": 0,
    "tool_call_id": "<id opcional>"
  }
  ```
- O servidor também reinspeciona eventos sem a marca `_ai_memory_capture_policy: 1` usando uma política ativa sintética com o padrão sentinela `"C:/__ai_memory_capture_server_inspection_never_match__"` no Windows e `"/__ai_memory_capture_server_inspection_never_match__"` no Unix.

## 8. Protocolo cliente e servidor e `--check-capture` (LEI §4.4; MF)

- Checagem local: `ai-memory hook --event ... --agent ... --check-capture` lê um JSON da entrada padrão, não faz spool, fila, drenagem, rede nem handoff, e imprime um JSON com **15 campos fixos**: `protocol_version` (`1`), `capture_mode`, `event`, `agent`, `policy_state`, `marker_present`, `ignore_rule_count`, `tool_family`, `candidate_path_count`, `disposition` (`"keep"` ou `"drop"` — `MetadataOnly` mapeia para `"drop"` em `disposition`), `effective_decision` (`"keep"`, `"metadata_only"` ou `"drop"`), `reason`, `extraction_status`, `scope_Marker_state` e `drop_subagent_captures`.

## 9. Descarte de subagente (relacionado; LEI §4.5, MF "Subagent captures")

- Opt-in por projeto (`drop_subagent_captures` no marcador; `drop_subagent` na query string de `/hook`). Com valor truthy, o servidor aceita (202 `subagent capture dropped`) mas não armazena capturas de subagentes.
- Claude Code não marca a sessão pai: só eventos marcados (e início/fim de subagente) são descartados (LEI §4.5).

## 10. Aplicação na feature 002

- Escopo: política de captura por marcador pertence ao CLI de hook e à reinspeção do servidor.
- Leitura: campos de escopo pela regra de linha e `[capture]` por TOML estrito, com os limites acima (MF).
- `last_assistant_message` é sempre removido antes de qualquer etapa (LEI §5).
