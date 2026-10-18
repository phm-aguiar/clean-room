# Consolidação da fase 0 (T006): feature 002 Captura e acesso

Wiki-Keywords: a confirmar, ADR, constitution, decisão nossa

Hierarquia: `docs/` vence spec e plan. Entradas: notas T001 (`hook-contract.md`), T002 (`handoff-contract.md`), T003 (`mcp-contract.md`), T004 (`mcp-go-evaluation.md`), T005 (`capture-contract.md`), `spec.md`, `plan.md`, `.github/memory/constitution.md`.
Siglas: LEI = `docs/lifecycle-event-ingestion.md`; HSC = `docs/hook-scripts-and-container-deploy.md`; MF = `docs/marker-file.md`; MS = `docs/mcp-server.md`; AR = `docs/architecture.md`; DHO = `docs/design-handoff-offer-not-claim.md`; DV = `docs/domain-vocabulary.md`; SR = `docs/storage-and-retrieval.md`; PIW = `docs/design-page-ingestion-windows.md`.

## 0. Resumo executivo

- Nenhum item bloqueia T007. Há três pendências para o lead resolver antes das tasks que as consomem (seção 5): ADR do pino de protocolo sem task dona (antes de T033), escopo de `answer`/`global`/`as_of` em `memory_query` (antes da task do `memory_query`), e escrita com `Unset` (antes da task do `memory_write_page`).
- A contagem de tools não é inconsistente: 20 (AR) + 3 (`memory_explore`, `memory_consolidate`, `memory_auto_improve`) = 23 (MS). A nota T003 dizia "22 linhas"; contei 20 linhas na tabela do AR.
- Duas notas se contradizem (sessão viva por id: T002 aplica, T003 não aplica). Vence o plan Q-05 (aplica).
- A chave de subagente não é um conflito real entre docs (item 3.3).
- Nenhuma violação da constituição nas notas, salvo dois pontos a ajustar (seção 4).

## 1. Consolidação por tema

### 1.1 Evento de hook do Claude Code (T001)
- Eventos com script: `session-start`, `user-prompt-submit`, `pre-tool-use`, `post-tool-use`, `pre-compact`, `stop`, `session-end`, `subagent-start`, `subagent-stop` (HSC §1.1). Não há `post-compaction` no Claude Code.
- Parsing tolerante: nenhum campo obrigatório; `session_id` obrigatório só depois (LEI §3.3, §8). Não-UUID vira UUID v5 no namespace OID. `cwd` do corpo vence o da query. `occurred_at` vale se positivo e até 5 min à frente, senão "agora". `ingest_key` de 1 a 64 caracteres `[A-Za-z0-9_-]`; inválida é ignorada sem 4xx.
- Esquema fechado de ferramenta (LEI §3.5): título `tool <família>`, família em `file | search-list | non-file | unknown`; argumentos nunca entram em título nem corpo; pós-ferramenta traz `outcome: unknown` para Claude Code, separador `---` e saída.
- Limites de corpo (LEI §3.6, confirmado): 16 KiB para prompt, pós-compactação, início e fim de sessão, pré-compactação e subagentes; 2 KB para notificação, pré e pós-ferramenta, parada e outro.
- `last_assistant_message`: removido incondicionalmente antes de qualquer etapa (LEI §5). O script de shell descarta o evento todo (HSC §1.6); o cliente nativo remove o campo.
- Resposta do hook no SessionStart: objeto com `hookSpecificOutput.additionalContext` (confirmado em `docs/install.md` linhas 997, 1076 e 1279; HSC linha 57 diz "chave de saída específica de SessionStart"); sem handoff, `{}`. O hook sempre sai com código 0.

### 1.2 Marcador e política de captura (T005)
- Duas caminhadas: scope walk pula marcadores que só têm `[capture]`; capture walk usa o marcador mais próximo de qualquer tipo. Fronteiras: `$HOME`, raiz do checkout git, ou só o cwd (MF "Walk boundaries").
- Campos de escopo lidos por linha; `[capture]` por TOML estrito; limites: 128 padrões, 1 024 caracteres por padrão, 64 KiB, 32 candidatos por evento, 4 096 caracteres por candidato, 1 000 000 comparações. Orçamento esgotado: arquivo vira `MetadataOnly`, shell vira `Drop` (MF "Bounds"; LEI §4.2).
- Tabela Keep/Drop/MetadataOnly por estado da política (Inativa, Ativa, Inválida) e família: ver `capture-contract.md` §6; conferida contra LEI §4.3 pela nota e consistente com o plan Q-01/Q-09.
- Corpo substituto de `MetadataOnly` e `--check-capture`: ver `capture-contract.md` §7 e §8.
- Confirmei no MF: "capped at 2,000 UTF-8-safe bytes" (MF linha 357), e a lista de chaves da scope walk (MF linha 40).

### 1.3 Handoff (T002)
- Estados `open -> accepted | expired`; reivindicação exatamente uma vez. Seleção: filtro de dono, só `open`, manual vence automático, automático só se o `cwd` armazenado é igual ou ancestral do `cwd` do receptor por fronteira de componente, sessão viva (observação nos últimos 10 min) protegida, desempate `created_at`, `cwd` mais longo, id (DHO §3).
- `GET /handoff`: lê e renderiza tudo antes de reivindicar (DHO §5; LEI §14 item 4). Modo oferta (`claim_on_session_start = false`) só emite a nota de metadados.
- Expiração: ao criar automático, expira os automáticos abertos de `cwd` exato igual e mesmo dono; pós-reivindicação varre os de chave de preferência menor. Manuais nunca são varridos (DHO §4).
- Criação no fim de sessão, resumo, próximos passos, perguntas em aberto e página `sessions/<id>.md`: `handoff-contract.md` §6 e §7 (LEI §8.3, §9, §10).
- Configuração: `[handoff] claim_on_session_start` e `create_on_session_end`, globais; sem override por projeto, sem opt-out por execução, sem `to_agent` (DHO §8, propostas).

### 1.4 Tools MCP e escopo (T003)
- As 12 tools da 002 são exatamente as 12 de AR que não são mensagens, feedback, forget_sweep, lint nem install_self_routing. Conferido: AR lista `query, recent, read_page, read_session_observations, status, briefing, handoff_begin, handoff_list, handoff_accept, handoff_cancel, write_page, delete_page` mais 8 fora do escopo.
- Chave do ator: usuário + id de sessão, na ordem contexto de transporte, `x-memory-actor-session-id`, id nativo, `mcp-session-id` (MS §2.1). Ponteiro de projeto ativo: modo `per_actor`, TTL 1 h, 4 096 entradas, `Lookup` e `Read` (DV §9).
- Limites e respostas por tool: `mcp-contract.md` §3 (conferido contra o plan §2.3, com as divergências da seção 3).

### 1.5 Biblioteca MCP (T004)
- Recomendado `github.com/mark3labs/mcp-go v1.2.1` (go 1.25.5 <= 1.26). Suporta HTTP streamable sem estado (`WithStateLess`), `WithDisableStreaming`, `WithHTTPContextFunc` e stdio (`ServeStdio` com `WithStdioContextFunc`).
- Não fixa o protocolo (item 3.1). Dependências novas a registrar em `tech.md`: `google/jsonschema-go`, `santhosh-tekuri/jsonschema/v6`, `spf13/cast`, `yosida95/uritemplate/v3` (`google/uuid` já existe).

## 2. Lacunas dos docs (o doc é omisso) e decisão nossa

| # | Lacuna | Decisão nossa | Justificativa |
|---|---|---|---|
| L1 | Formato do erro de tool MCP | Resultado de tool com `isError: true`, texto curto, sem segredo, sem caminho do sistema de arquivos, sem eco de valor informado. Erro JSON-RPC só para método ou tool desconhecida e argumentos que não decodificam. Duas classes de erro de escopo: entrada inválida e não encontrado, distinguidas na mensagem | MS e SR §14.1 só fixam as classes (400-like, 404-like). Falha fechada sem vazar dados |
| L2 | Layout markdown do handoff entregue | Seções em inglês, na ordem: aviso fixo, cabeçalho (`# Handoff <id>`, agente de origem, tipo manual ou automático, `cwd`, data), `## Summary`, `## Open questions`, `## Next steps`, `## Files touched`; seção vazia omitida; tudo entre `untrusted-history:start` e `:end`. Os testes de aceitação verificam o envelope, o aviso e a presença dos campos, nunca o texto exato dos cabeçalhos | DHO §5 só exige envelope e "aviso de segurança" sem fixar texto; os rótulos do doc são em inglês (`Tools used:`, `Unresolved question:`). Fixar o layout por fixture na task de síntese |
| L3 | Texto exato do aviso de segurança | Uma frase fixa em inglês, constante única, dizendo que o conteúdo é dado histórico não confiável, não instruções, e que nunca deve ser executado nem alterar permissões ou políticas | DHO §5 e LEI §11.2 descrevem o sentido, não o texto |
| L4 | Nota de oferta: "ago ago" (item 3.4) | Ver 3.4 | |
| L5 | Cálculo de `<idade>` | `floor` por faixa: `< 60 s` "just now"; `< 3 600 s` minutos; `< 86 400 s` horas; `< 2 592 000 s` dias; senão meses (30 dias); singular quando N = 1 | DHO §6 fixa os cortes, não o arredondamento nem o singular |
| L6 | Mapeamento `tool_name` -> família (`file`, `search-list`, `non-file`, `unknown`) | Tabela fechada no `ingest/normalize`, coberta por fixtures (Read, Edit, Write, MultiEdit, NotebookEdit = `file`; Grep, Glob, LS = `search-list`; Bash e demais conhecidos = `non-file`; desconhecido = `unknown`) | LEI §3.5 não lista nomes. Lista inicial é hipótese de implementação e deve ser revisada contra o Claude Code real na task do adaptador |
| L7 | Chaves de `tool_input` lidas pela política de captura (`file_path`, `notebook_path`, `path`, `edits[]`, `command`) | Seguir LEI §4.3 e §4.4 (não relidas nesta fase) e a lista de MF "Decisions"; somente campos de caminho explícitos | T001 declarou não ter detalhado LEI §4.3/§4.4; T005 cobre a política, mas não a lista de chaves por ferramenta. Ler LEI §4.3/§4.4 na task da política |
| L8 | Validade de `tool_use_id` | Limite de 512 bytes, sem caracteres de controle, sanitizado; inválido segue sem `tool_call_id` | API "Allowlist de metadados" dá 512 bytes; o hook é omisso |
| L9 | `tool_response` como objeto | Aceitar string, objeto (renderizado como JSON compacto) e ausente | Doc omisso |
| L10 | Esquema completo do payload (`hook_event_name`, `transcript_path`, `permission_mode`, `source`) | A normalização não depende desses campos; fixtures podem incluí-los | Só observados para o Codex (`docs/desktop/codex-desktop.md`) |
| L11 | Diretório das fixtures | `internal/ingest/testdata/claude-code/` | O plan Q-13 já fixa; vence sobre o `testdata/hooks/claude-code/` sugerido na T001 |
| L12 | Corpo de `MetadataOnly` e `cwd` | O cwd é metadado de roteamento permitido (LEI §4.3 o lista); nenhum caminho de `tool_input` entra | MF diz "sem caminhos"; LEI lista "diretório". LEI é mais específica |
| L13 | "2 KB" é 2 000 ou 2 048 bytes | 2 000 bytes UTF-8 seguros | MF linha 357 ("2,000 UTF-8-safe bytes") e LEI §5 (excerto, 2000 bytes) |
| L14 | Nome do parâmetro de consulta da chave de subagente | Ver 3.3 | |
| L15 | Campos exatos de `memory_handoff_begin`, `memory_recent`, `memory_status`, `memory_write_page` (`path`, `body`) | `summary`, `open_questions`, `next_steps`, `files_touched`, `shared`, `cwd`; `memory_recent` devolve os campos de um hit sem scores; `memory_status` devolve páginas (últimas versões), observações, sessões e handoffs abertos, mais `scope.resolved_by`; `memory_write_page` segue a assinatura da 001 | DV §10 nomeia os campos de handoff; os demais seguem a 001 |
| L16 | Limite excedido em `memory_handoff_begin` | Truncar após sanitização | MS diz "limitados" |
| L17 | `limit` fora de 1..100 | Clamp, sem erro | MS §3.1 "limitado a 1..100" |
| L18 | `query` vazio, `path` e `query` ausentes em `memory_read_page`, `scopes` > 25 | Entrada inválida | Doc omisso na classe |
| L19 | Alias e canônico juntos com valores diferentes | Canônico vence | Doc omisso |
| L20 | `memory_handoff_cancel` e `memory_handoff_accept` sem alvo | `cancel`: `{cancelled: true, handoff_id}` ou `cancelled: false` sem erro; `accept` sem elegível: `handoff: null` sem erro | Doc omisso |
| L21 | `order` de `memory_read_session_observations`; sessão inexistente | `asc` (padrão) e `desc`; sessão inexistente = não encontrado | Doc omisso |
| L22 | `any_owner` para anônimo | Só administrador (token raiz); anônimo recusado | MS §4.3 dá acesso local irrestrito ao anônimo, mas omite `any_owner`. Na 002 não há multiusuário |
| L23 | Registro de atividade do cliente no ponto único de chamada | Não construir na 002 | Sem dream pass, o wrapper seria "superfície pública sem chamador" (anti-padrão da constituição). T003 propunha mantê-lo condicionado à 001; descartado |
| L24 | Escrita de `scope: "profile"` e projetos `_profile` | `"profile"` recebe o mesmo erro de valor não aceito, listando só `global` | Perfil é feature 012 |
| L25 | Hooks que avançam o ponteiro e `Mismatch` | Conforme plan Q-06 e DV §9.4 | |

## 3. Pontos tratados explicitamente

### 3.1 mcp-go v1.2.1 não fixa o protocolo em 2024-11-05
- Fonte: T004 (leitura de `mcp/version.go` e `server/server.go` da tag v1.2.1). A lib ecoa a versão do cliente quando válida e não "moderna"; conhece 2026-07-28. **Não reverifiquei contra o código-fonte e não houve build** (ver seção 7).
- MS §1 (linha 22): a versão fixa é requisito do doc (evitar negociar para baixo até uma versão em que o cliente descarta ferramentas). O doc vence.
- O plan **prevê o risco, mas não o desenho**. Plan Q-07 (linha 282): "se o `mcp-go` não permitir fixar 2024-11-05, a primeira task de MCP registra o desvio em ADR antes de seguir". Falhas do plan: (a) chama de "desvio" algo que, com wrapper, cumpre o doc; (b) nenhuma task de `tasks.md` cria o ADR (grep por "ADR" em `tasks.md` não acha task de MCP que o produza); a primeira task de MCP é T033 (interface e adaptador) e cita "protocolo 2024-11-05", mas lista nenhum arquivo de ADR; (c) o plan não prevê o wrapper.
- Decisão nossa: o adaptador (T033) fixa o pino por wrapper. Preferência: reescrever o `protocolVersion` da **resposta** do `initialize` para `2024-11-05` e rejeitar `server/discover` (protocolo moderno); alternativa, reescrever a requisição `initialize` (proposta de T004). O wrapper fica no adaptador, nunca nos handlers. Teste de contrato: cliente que pede 2025-11-25 recebe 2024-11-05. O ADR "pino de protocolo por wrapper" é criado em T033 (o lead deve incluir o arquivo de ADR na lista de arquivos de T033 e ajustar o texto do plan Q-07 de "desvio" para "pino por wrapper"). Pendência P1.
- Risco anotado: o handshake e as regras de cabeçalho `MCP-Protocol-Version` da v1.2.1 devem ser testados em T033 em stdio e HTTP. Não verificado por execução.

### 3.2 Contagem de tools
- AR (linha 410): "MCP tool surface (20 tools)"; a tabela tem **20 linhas** (conferido: 20 itens `| \`memory_...\`` entre as linhas 410 e 435). A nota T003 dizia 22; incorreta.
- MS (linhas 6 e 100): "23 ferramentas". As 3 a mais são `memory_explore`, `memory_consolidate` e `memory_auto_improve` (citadas em MS e fora da tabela de 20 do AR). 20 + 3 = 23: **os docs são consistentes entre si**, descrevendo superfícies diferentes (AR sem as três de LLM e consolidação).
- A 002 fixa 12 = as 20 do AR menos 8 (4 `memory_message_*`, `memory_feedback`, `memory_forget_sweep`, `memory_lint`, `memory_install_self_routing`). Spec e plan estão certos; nenhuma ação.

### 3.3 Chave de subagente
- Evidência: MF (linhas 65, 97) e cli.md (linha 372) e HSC (linha 107) chamam a **chave do marcador** `drop_subagent_captures`. HSC (linha 122) lista `drop_subagent` entre os **parâmetros enviados** pelo hook de shell; LEI §2.1 (linha 67) lista `drop_subagent` entre os parâmetros de consulta do `/hook`. A LEI §4.5 (linha 300) escreve "opção `drop_subagent` no marcador", o que é um rótulo impreciso para a chave do marcador.
- Conclusão: **não há conflito de dois nomes para a mesma coisa**: o marcador usa `drop_subagent_captures` e o `/hook` recebe `drop_subagent`. A decisão registrada na T005 fica confirmada: arquivo `drop_subagent_captures`, query `drop_subagent`. O plan §2.1 e Q-09 já estão assim. Valor truthy: `1,true,yes,on` (MF); falso: `0,false,no,off`.

### 3.4 "left N minutes ago ago"
- Confirmado no doc: DHO §6 (linhas 137 a 144): o template tem `left <idade> ago`, `<idade>` já termina assim, e o doc declara a saída real "left 3 minutes ago ago" / "left just now ago", "comportamento observável atual", normalização permitida ("decisão a confirmar"). Observação: o doc descreve `<idade>` como "just now" ou "N minute(s)|..." (sem "ago"); a leitura da nota T002 ("`<idade>` já termina assim") é aproximada, mas o defeito existe no template.
- Decisão nossa (plan Q-05, "um único ago"), formalizada: `left <quando>.` com `<quando>` = `just now` ou `N <unidade>(s) ago`. Frase completa: `📬 ai-memory: a pending handoff \`<id>\` from \`<from_agent>\`, left just now. To pick it up, call \`memory_handoff_accept\` with handoff_id \`<id>\`.` (e `left 3 minutes ago.`). O ponto após `<quando>` vem do template do doc. O plan não fixa "just now" sem sufixo; esta é a forma adotada. O teste verifica que "ago ago" não aparece e que a nota não contém nenhum campo do resumo.

### 3.5 Cenário 6 de DHO §9 e admissão
- Fato: DHO §9 cenário 6 é "cadeia de admissão recusando `HandoffAccept`". A 002 não tem cadeia de admissão (spec "Fora do escopo": webhooks de admissão são das features 007 a 009; plan "O que não construir"). O plan Q-13 (linha 304) lista os cenários 1, 2, 4, 6 e 7 como aceitação da 002.
- Resolução: **tirar o cenário 6 da 002** (fonte que vence: o escopo da spec e a constituição "Proibido stubs de trabalho futuro"; testar com stub criaria exatamente o stub proibido). O cenário passa para a feature da cadeia de admissão (spec 008). A 002 testa 1, 2, 4 e 7. O plan Q-13 deve ser corrigido de "1, 2, 4, 6 e 7" para "1, 2, 4 e 7". Pendência P4, ação de edição para o lead; o .feature da T013 não deve incluir o cenário 6.

### 3.6 Regra de sessão viva em `memory_handoff_accept` por id
- DHO §7 (linhas 167 e 168) marca "a confirmar". **As notas se contradizem**: T002 §3.2 (reavalia dentro de `ClaimHandoffTx`) e T003 §3.11 ("não aplicar por id"). O plan Q-05 e a auditoria (linha 383) dizem "aplicada".
- Decisão nossa: **aplicar**. `ClaimHandoffTx` reavalia a sessão viva dentro da transação para handoff automático, também quando o id é informado; a varredura pós-reivindicação (`SweepAfterClaimTx`) roda no caminho comum. Justificativa: um único caminho atômico (DHO §7 "mesmo caminho atômico"), nunca tirar o bastão de uma sessão ainda ativa e plan Q-05 vence a nota. Handoff manual não sofre o filtro. Considerar a nota T003 §3.11 obsoleta neste ponto. Registrar em ADR/notas de decisão junto com a nota de oferta. Teste `TestINV16_*` cobre o caso por id.

### 3.7 `memory_delete_page` e página inexistente
- AR (linha 432): "Idempotent". MS (linha 233): "Comportamento para página inexistente a confirmar". Confirmado nos dois docs.
- Decisão nossa: página inexistente retorna `deleted: false` sem erro, sem checkpoint (idempotente, segue AR). Não é violação: `DeletePage` da 001 deve permitir distinguir "não havia". Teste: deletar duas vezes seguidas dá `true` e depois `false`. Soft delete/tombstone continua (constituição).

### 3.8 `answer` e `reasoning` em `memory_query`
- Fatos: MS §3.1 lista `answer` e `reasoning` entre os parâmetros (linha 100); sem provedor, o MS devolve **sempre** os resultados com a nota `answer_unavailable`, "nunca erro" (linha 160); o fluxo global já cita `answer_unavailable` (linha 117). Spec 002 põe `answer` fora do escopo; plan §2.3 e checklist INV-13: "sem `answer`".
- Divergência spec/plan x doc. O doc também dá `answer_unavailable` como o caminho **zero-LLM**, que é o da 002; INV-13 exige funcionar completo sem provedor, e o doc não exige a síntese.
- Decisão nossa (recomendada, AMBIGUOUS entre duas leituras): (A) não declarar os argumentos (plan atual); cliente que os passe recebe o comportamento do SDK para argumento desconhecido; (B) **declarar `answer` (bool) e `reasoning` (enum `minimal|low|medium|high|max`, valor desconhecido rejeitado) e, com `answer=true`, devolver os resultados com `answer_unavailable`** (comportamento sem provedor do doc). Recomendo (B): o doc vence, é o contrato público, é zero-LLM e não deixa stub (é o ramo "sem provedor" completo; a feature 019 acrescenta a síntese). Se o lead escolher (A), registrar como "fora do escopo por spec" e aceitar a divergência com o doc. Pendência P2 (afeta a task do `memory_query`).

### 3.9 `as_of` e `docs/design-page-ingestion-windows.md`
- Li o PIW completo. `as_of` depende das janelas de página `valid_from`/`valid_to` (colunas na V62; implementado no produto, fase A), de busca textual filtrada por versão, RRF k=60 entre entidade e texto, e ajuste de autoridade (PIW §6.2).
- A **001 cobre o armazenamento**: `docs/database-schema.md` (linhas 128 e 143) define as colunas e o índice; o plan da 001 (P-04, linha 88) fecha `valid_to` na supersessão; o código tem `valid_to`/`valid_from` em `internal/storage`. A V01 da 001 já está no estado final.
- A **consulta `as_of` não é da 002**: o plan §2.3 não a lista, o roadmap atribui `as_of` à 005, e a spec 005 diz que "falta a consulta `as_of`" (spec 005 linhas 20 a 23). A nota T003 §3.1 incluiu `as_of` entre os argumentos da 002: **divergência da nota; vence o plan/roadmap** (escopo). A 002 não declara `as_of` no schema. Nenhuma dependência de PIW pende para a 002; o backfill de V62 (PIW §5.1) não se aplica (V01 já é o estado final; conferir apenas que a 001 grava `valid_from`/`valid_to` na escrita).
- Não verificado: se a 001 já preenche `valid_to` na aposentadoria por exclusão/decaimento (fora da 002).

### 3.10 `global`, ponteiro `default_global` (derivado do item anterior)
- A nota T003 inclui `global=true` e `default_global` em `memory_query` da 002; o plan §2.3 não lista `global`, embora cite a união com `_global` em `global_scope_hits`, e nenhuma outra spec detém o argumento (a spec 007 pressupõe que existe). Doc: MS §3.1 passo 1.
- Decisão nossa (AMBIGUOUS, recomendo implementar): `global=true` entra na 002 (ranqueador só FTS, zero-LLM; cada hit rotulado; erro ao combinar com `workspace`, `project` ou `scopes`) e o plan §2.3 ganha a coluna. O flag `default_global` do ponteiro é guardado como DV §9.3 manda (entrada guarda a preferência). Se o lead preferir cortar, registrar em `global_hits` fora e deixar só `global_scope_hits`. Pendência P2, junto com 3.8.

### 3.11 Escrita com ponteiro `Unset` (decisão da nota T003 que contradiz a spec)
- A nota T003 §2.4 decidiu que, com `Unset`, a escrita usa o padrão configurado, a partir de DV §9.4 ("o padrão configurado é a única resposta", linha 604). Mas SR §14.1 (linha 566), MS §2.1 (linhas 74 a 76) e a **spec 002 (linha 56)** e a constituição dizem: escrita sem escopo e sem ponteiro resolvido é **recusada**, "nunca grava no padrão em silêncio".
- Os docs apontam em direções diferentes (DV §9.4 descreve o resultado do lookup; SR/MS descrevem a regra de escrita). Decisão nossa: **recusar** (SR e MS são o contrato da escrita, a spec e a constituição concordam; o DV só define o que o lookup devolve). Cliente MCP sem hooks escreve passando `workspace` e `project`. A nota T003 §2.4 fica corrigida aqui. Pendência P3 (confirmação do lead, antes da task do `memory_write_page`).

### 3.12 Erro de tool e layout do handoff
- Ver L1 a L3. Omissos nos docs; decididos acima. Nenhum bloqueia T007.

## 4. Checagem da constituição

Auditoria por checklist (nenhuma verificação automática de código; as notas são texto):
- INV-01: a política exige `config.LoadHook` em `internal/config`; as notas não propõem `os.Getenv` fora dele. OK.
- INV-05, INV-06, INV-10, INV-13, INV-16: contratos consistentes com as notas. OK.
- Grafo de imports (`mcp` não importa `ingest`): T003 respeita (seleção e reivindicação em `storage`). OK.
- Anti-padrão "abstração preemptiva / superfície sem chamador": **violaria** o wrapper de atividade do cliente (L23). Corrigido na decisão L23.
- Anti-padrão "stubs de trabalho futuro": violaria testar o cenário 6 com stub (item 3.5). Corrigido.
- Anti-padrão "tratar 'a confirmar' como fato": todas as decisões "a confirmar" desta nota viram "decisão nossa" e devem entrar em ADR/notas de decisão (nota de oferta, sessão viva por id, idempotência de delete).
- Dependência nova só via `tech.md`: T007 deve listar as 4 dependências transitivas novas no `tech.md`. O `tech.md` (linha 19) ainda diz "não usado nesta feature"; atualizar.
- Clean-room: a lista de nomes de ferramentas do L6 é hipótese de implementação, a rever com a documentação pública do Claude Code.

## 5. Divergências spec/plan x doc (e entre notas)

| # | Divergência | Fonte que vence | Ação |
|---|---|---|---|
| D1 | Plan Q-07 trata o pino do protocolo como "desvio em ADR"; o doc exige o pino (MS §1) e a lib não o faz | MS §1 | Pino por wrapper no adaptador (T033); ADR do wrapper; reescrever o texto do plan |
| D2 | Plan Q-13 lista o cenário 6 de DHO §9 (admissão) como aceitação da 002; a spec exclui admissão | Escopo da spec + constituição (sem stub) | Remover o cenário 6 da 002 (item 3.5) |
| D3 | Plan §2.3 e spec: `memory_query` sem `answer`; MS §3.1 define `answer`/`reasoning` e `answer_unavailable` sem provedor | MS §3.1 (doc), com escopo zero-LLM | Recomendação B (item 3.8); aguarda o lead |
| D4 | Plan §2.3 não lista `global` em `memory_query`; MS §3.1 lista | MS §3.1 | Item 3.10; aguarda o lead |
| D5 | Nota T003 inclui `as_of` na 002; plan e roadmap o põem na 005 | Plan/roadmap (escopo) | Não declarar `as_of` (item 3.9) |
| D6 | Nota T003 §2.4: escrita com `Unset` grava no padrão; spec/SR/MS recusam | SR §14.1, MS §2.1, spec linha 56 | Recusar (item 3.11) |
| D7 | Nota T003 §3.11 não aplica sessão viva por id; plan Q-05 e T002 aplicam | Plan Q-05 | Aplicar (item 3.6) |
| D8 | Nota T001 sugere `testdata/hooks/claude-code/`; plan Q-13 fixa `internal/ingest/testdata/claude-code/` | Plan Q-13 | Usar o do plan |
| D9 | Nota T003 diz "22 linhas" na tabela do AR; são 20 | AR | Corrigir a leitura; contagem consistente (item 3.2) |
| D10 | Nota T003 §3.1: `answer_unavailable` "depende de LLM"; MS mostra que é o ramo sem LLM | MS §3.1 linha 160 | Ver D3 |
| D11 | Chave de subagente aparentemente em conflito (MF x LEI §4.5) | MF para a chave do arquivo; LEI §2.1 e HSC linha 122 para o parâmetro | Sem conflito real (item 3.3) |
| D12 | Spec e plan citam "2 KB" sem dizer 2 000 ou 2 048 | MF linha 357 (2 000) | 2 000 bytes (L13) |

Spec e plan **já** incorporam: 12 tools, ausência de migração para `occurred_at`, `install-hooks`/`install-mcp`, `.serve.lock`, ledger mensal, `handoff: null` para o perdedor (spec "Revisão contra os docs", 10 itens). Nenhum desses foi reaberto.

## 6. Pendências e o que bloqueia as próximas tasks

| Id | Pendência | Bloqueia | Gravidade |
|---|---|---|---|
| P1 | ADR do pino de protocolo sem task dona; adicionar arquivo de ADR e o wrapper ao escopo de T033; ajustar texto de plan Q-07 | T033 | Alta (antes de T033). **Não bloqueia T007** |
| P2 | Decidir (A) ou (B) para `answer`/`reasoning` e implementar ou não `global` em `memory_query`; corrigir plan §2.3 e checklist INV-13 conforme a escolha | Task do `memory_query` | Média |
| P3 | Confirmar "recusar" para escrita com `Unset` | Task do `memory_write_page` | Média |
| P4 | Corrigir plan Q-13 (cenários 1, 2, 4 e 7); T013 não inclui cenário 6 | T013 e task de aceitação | Baixa |
| P5 | `tech.md`: registrar as 4 dependências transitivas de `mcp-go`; trocar "não usado nesta feature" | T007 (já lista `tech.md` em Arquivos) | Baixa |
| P6 | Ler LEI §4.3 e §4.4 para a lista de chaves de `tool_input` (L7) | Task da política de captura | Baixa |
| P7 | Validar o mapeamento de L6 contra o Claude Code real | Task do adaptador de hook | Baixa |

**Nada bloqueia T007**, com a condição de que T007 faça o build de prova (`CGO_ENABLED=0 go build ./...` e `go vet`, windows/amd64 e linux/amd64), porque a compatibilidade da v1.2.1 foi só inferida por T004. Se o build falhar, T007 reporta e o lead decide; a versão v1.2.1 não deve ser fixada sem o build.

## 7. O que não foi verificado

- Nada sobre `mcp-go` foi confirmado por mim: versão v1.2.1, data de release, `go.mod`, comportamento de negociação de protocolo e opções (`WithStateLess` etc.) vêm da T004 (leitura de GitHub); sem build, sem execução.
- Compilação com Go 1.26 e `CGO_ENABLED=0` e a ausência de cgo nas dependências: inferidas.
- Não reli LEI §4.3 e §4.4, §8, §9, §11 nem DV §9 inteiros: citei as notas e confirmei só os trechos listados (LEI §3.6 e §4.5, MF linhas 40, 65, 97 e 357, HSC linhas 57, 107 e 122, DHO §5 a §9, MS linhas 6, 22, 100, 117, 160 e 233, AR linhas 410 a 435, SR linha 566, DV linha 604, PIW inteiro).
- Não verifiquei o texto exato do "aviso de segurança" (o doc não o traz) nem o JSON exato do hook de saída (`hookEventName`).
- Não verifiquei se a 001 grava `valid_to` em todos os caminhos de aposentadoria (item 3.9) nem a assinatura de `DeletePage` para o retorno `deleted: false`.
- Não executei nenhuma verificação determinística por grep no código (credenciais, ORM, hard delete): esta task é de documentação e não altera código.
- Não conferi as demais citações das notas (por exemplo, LEI §8.3, §9, §10 e §11.2, HSC §1.x, DV §10) além do que aparece acima; elas permanecem como "afirmadas pelas notas".
