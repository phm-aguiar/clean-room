# Contrato do marcador `.ai-memory.toml` e da política de captura (feature 002)

Wiki-Keywords: marker-file, capture exclusions, ignore_paths, MetadataOnly

Hierarquia de fontes: `docs/` vence a spec e o plan. Siglas de citação: **MF** = `docs/marker-file.md`; **LEI** = `docs/lifecycle-event-ingestion.md`; **SPEC** = `specs/features/002-captura-e-acesso/spec.md`; **PLAN** = `specs/features/002-captura-e-acesso/plan.md`. Onde o doc é omisso aparece "decisão nossa".

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
- Marcador em `$HOME` (home marker) nunca é declaração local para os diretórios abaixo; é só fallback (MF "Home marker"). Rotas `[routes.*]` são só do home marker (MF "Keys"). Fora do escopo da captura da 002 (decisão nossa: a 002 usa o home marker apenas se o plano de escopo o exigir; não há texto da SPEC/PLAN sobre rotas).
- Modo denylist (padrão): sem marcador, tudo é capturado. Modo allowlist: a presença de marcador na capture walk é o opt-in; sem marcador nenhum evento é emitido, nem prompts, descartados no processo do hook antes de spool ou rede (MF "Allowlist mode"; LEI §4.1). Persistido por instalação, `install-hooks --capture-mode allowlist` (MF "Allowlist mode"; PLAN Q-11).

## 2. Leitura do arquivo (MF "File format and reading")

- **Tamanho**: `[capture]` e inspeção leem no máximo 64 KiB (65 536 bytes); maior que isso é rejeitado ali. Codificação UTF-8 obrigatória.
- **Campos de escopo** (workspace, project etc.): leitura **por linha**, simples, sem TOML completo. Chave lida de linha que começa com o nome da chave, depois `=`. Strings exigem valor entre aspas duplas, terminando na próxima aspa dupla; valor nu não é lido. Flags aceitam token com ou sem aspas, com comentário `#` inline removido. Cabeçalhos de seção não são rastreados; a primeira linha que casa vence, em qualquer posição.
- **TOML estrito**: usado para `[capture]` e para o estado de inspeção. A inspeção compara cada chave string com a leitura estrita; divergência vira `conflicting`.
- **Resolução tolerante, captura estrita**: arquivo com TOML inválido ainda fornece campos de escopo à resolução; só a inspeção o marca `invalid`. A captura de ciclo de vida usa a leitura estrita para `[capture]`, então `[capture]` quebrado torna a política de captura **inválida** (MF "Inspection states", último parágrafo).
- Verdadeiros: `1`, `true`, `yes`, `on`; falsos: `0`, `false`, `no`, `off` (sem distinguir caixa, com ou sem aspas) (MF "File format and reading").
- Estados de inspeção, na ordem: `ignored`, `invalid_home_routes`, `absent`/`invalid`, `unreadable` (>64 KiB ou ilegível), `invalid`, `conflicting`, `valid`/`home_*_route` (MF "Inspection states"). Marcador ilegível como texto na etapa 3 conta como `absent`.
- Chave `drop_subagent_captures` (truthy, raiz) é o nome no MF "Keys" e "Subagent captures". A LEI §4.5 chama a opção de `drop_subagent`. **Conflito entre docs**; o MF é o doc do marcador e a LEI §2.1/PLAN §2.1 usam `drop_subagent` como parâmetro de consulta de `/hook`. Decisão nossa: chave do arquivo = `drop_subagent_captures`; parâmetro de consulta = `drop_subagent` (PLAN §2.1).

## 3. Seção `[capture]` estrita (MF "Capture exclusions", "Parsing and failure")

```toml
[capture]
ignore_paths = ["private/**", "~/personal-notes/**"]
```

- O marcador mais próximo é autoritativo; seções não se mesclam. `[capture]` ausente ou `ignore_paths` vazio = **inativa**, mantém o comportamento atual.
- `[capture]` aceita **somente** `ignore_paths`. Tornam a política **inteira** inválida (nada é aplicado parcialmente): chave desconhecida; valor que não é array; entrada que não é string; TOML inválido; texto não UTF-8; arquivo > 64 KiB; qualquer padrão que falhe na validação.
- Estados da política para o decisor (LEI §4.3): **Inativa** (sem marcador em denylist), **Inválida** (marcador malformado), **Ativa**.

## 4. Regras dos padrões (MF "Pattern rules")

- Casa o caminho **inteiro**, normalizado lexicalmente; não é substring.
- `*` casa dentro de um componente; `?` casa um caractere que não é separador; `**` cruza diretórios.
- Padrão relativo tem raiz no diretório do marcador; absoluto é usado como está; `~/` expande para o home.
- Caminhos relativos em eventos de ferramenta de arquivo são resolvidos a partir do cwd do evento.
- **Rejeitados** no padrão: os caracteres `! { } [ ] ( ) | ^ $ %`; a sequência `${`; três ou mais `*` seguidos; segmento `..`; `~` não seguido de `/`; caminhos relativos a drive como `C:foo`.
- Caixa: sensível em caminhos POSIX; drive e UNC do Windows casam ASCII sem distinguir caixa.
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
| Ativa | arquivo | algum candidato casa com padrão | `Drop` do evento inteiro (MF "Decisions") |
| Ativa | arquivo | sem caminho extraível, malformado ou não suportado | `MetadataOnly` (LEI §4.3; MF "Decisions") |
| Ativa | arquivo | caminhos extraídos, nenhum casa | `Keep` (LEI §4.3) |
| Ativa | shell | algum argumento casa, ou orçamento esgotado | `Drop` (LEI §4.3, §4.2; MF "Decisions") |
| Ativa | shell | nenhum casa | `Keep` (LEI §4.3) |
| Ativa | demais famílias | sempre | `Keep` (LEI §4.3) |
| Inválida | arquivo ou shell | marcador malformado | `MetadataOnly` (fail-closed) (LEI §4.3; MF "Decisions") |
| Inválida | demais | sempre | `Keep` (LEI §4.3) |

Detalhes por família (MF "Decisions"):
- Arquivo: só campos de caminho explícitos e arrays diretos documentados; entradas de multi-edit e multi-leitura são incluídas. O descarte ocorre localmente, antes de spool, fila, rede, logs de transporte ou armazenamento do servidor.
- Shell: o comando é dividido em palavras e comparado. Texto livre não é analisado além disso; texto de patch não é analisado em busca de caminhos.
- Prompts, texto do assistente, notificações e conteúdo citado não são atribuíveis a caminho e **não** são filtrados por esta política (MF "Decisions"; MF "What is kept from tool events", último parágrafo).
- Isto é fronteira léxica de captura, não prevenção completa de perda de dados (MF "Decisions").
- A decisão **nunca guarda** argumentos brutos, caminhos ou nomes arbitrários; guarda só família da ferramenta, contagem de caminhos e identificador válido da chamada (LEI §4.3).

## 7. Corpo substituto de `MetadataOnly`

- LEI §4.3: o corpo substituto contém identificador de sessão, diretório, família, nome canônico da ferramenta, identificador da chamada e a marca de protocolo.
- MF "Decisions": mantém só metadados limitados de roteamento, ferramenta e decisão; **não** contém caminhos, padrões, argumentos, saída, erros, títulos nem payload aninhado.
- Tensão: a LEI lista "diretório" (cwd) e o MF diz "sem caminhos". Decisão nossa: o cwd é metadado de roteamento permitido (a LEI vence nesse ponto por listá-lo explicitamente); nenhum caminho vindo de `tool_input` entra no corpo.
- Eventos pré-ferramenta não mantêm comando, argumentos, caminhos, corpos de entrada nem nomes arbitrários de ferramenta; pós-ferramenta acrescenta excerto de resposta ou erro, com o corpo renderizado completo limitado a 2 000 bytes UTF-8 seguros (MF "What is kept from tool events"; LEI §3.6 e SPEC "Edge Cases" dão 2 KB para pré e pós-ferramenta).

## 8. Protocolo cliente e servidor (LEI §4.4; PLAN)

- O cliente anexa a marca de protocolo (versão 1) ao aplicar a política. Política v1 é imposta pelo comando nativo de hook (MF "Supported integrations and refresh").
- O servidor **reinspeciona**: se o cliente pediu `Drop`, responde **202** e conta como "descartado pela política" sem gastar capacidade (LEI §4.4; PLAN §2.1 `capture policy dropped`; PLAN Q-02).
- Evento descartado reconhecido nunca entra em spool, fila, transporte, log ou armazenamento (LEI §4.4).
- Novos clientes são seguros com servidores antigos porque descartar e reduzir acontece no cliente (MF "Supported integrations and refresh").
- Checagem local: `ai-memory hook --event ... --agent ... --check-capture` lê um JSON da entrada padrão, não faz spool, fila, drenagem, rede nem handoff, e imprime só metadados limitados: versão do protocolo, estado da política, família, contagem de caminhos, disposição, estado da extração e estado da resolução de escopo; nunca caminhos, padrões ou conteúdo (MF "Checking a decision locally"; PLAN §2.5).

## 9. Descarte de subagente (relacionado; LEI §4.5, MF "Subagent captures")

- Opt-in por projeto. Com valor truthy, o servidor aceita (202) mas não armazena capturas de sessões de subagente; sessões de topo sempre são armazenadas.
- Claude Code não marca a sessão pai: só eventos marcados (e início/fim de subagente) são descartados (LEI §4.5; PLAN Q-09).

## 10. Aplicação na feature 002

- Escopo: política de captura por marcador pertence ao CLI de hook (SPEC "Dentro do escopo", CLI de hook); lógica pura em `internal/ingest/capture`, usada pelo cliente e pela reinspeção do servidor (PLAN Q-01).
- Leitura: campos de escopo pela regra de linha e `[capture]` por TOML estrito, com os limites acima (PLAN Q-09).
- Modo padrão denylist (PLAN Q-09); allowlist via `install-hooks --capture-mode` (PLAN Q-11).
- `last_assistant_message` é sempre removido antes de qualquer etapa; o marcador não restringe captura do assistente (MF "What is kept from tool events", último parágrafo; SPEC "Edge Cases"; LEI §5).
- Fora da 002: hooks de script de shell legados, Docker wrapper e demais agentes não impõem a política (MF "Supported integrations and refresh", marcado "a confirmar"; PLAN "O que não construir").

## 11. Pontos "a confirmar" ou omissos (registrar como decisão nossa)

- Onde o servidor impõe o padrão de nome de `workspace`/`project`: "a confirmar" no MF "Name rules". Decisão nossa: não validar além do que a 001 já faz; a regra `^[a-z0-9][a-z0-9._-]*$` vale para rotas do home marker.
- Se o doc diz "metadados limitados" sem enumerar o corpo exato além de LEI §4.3: decisão nossa é seguir a lista da LEI §4.3 exatamente.
- Nome da chave de subagente: ver §2 (decisão nossa).
- Interação do marcador inválido com campos de escopo: resolução continua tolerante; captura falha fechado (MF "Inspection states").
