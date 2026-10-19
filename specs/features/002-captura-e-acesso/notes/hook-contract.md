# Contrato do evento de hook do Claude Code (T001)

Fontes, em ordem de prioridade: `docs/` (vence), depois `specs/features/002-captura-e-acesso/spec.md` e `plan.md`. Cada afirmação cita arquivo e seção. Onde o doc é omisso, o texto marca **decisão nossa** ou **não definido no doc**.

Abreviações: LEI = `docs/lifecycle-event-ingestion.md`; HSC = `docs/hook-scripts-and-container-deploy.md`; API = `docs/api-reference.md`; CDX = `docs/desktop/codex-desktop.md`.

## 1. Observação sobre a fonte do payload

- O doc não traz um esquema completo e oficial do JSON que o Claude Code envia ao hook. O que existe é o que o ai-memory lê (LEI §3.3, §3.4, §3.5, §5). Daí vem este contrato.
- O parsing é tolerante: "Nenhum campo é obrigatório no parsing. A obrigatoriedade do identificador de sessão é aplicada depois" (LEI §3.3 e §8).
- O script de hook lê o JSON inteiro do stdin, extrai o `cwd` do payload e o envia **sem alteração** por POST em `/hook` com `event` e `agent` na query (HSC §1.2, passos 3 a 6). O hook sai sempre com código 0 (HSC §1.2, passo 8).
- Para Claude Code, `agent=claude-code`. Os aliases `claude` e `claude-code` estão em `docs/harness-adapters.md` (tabela inicial, linha Claude Code).

## 2. Eventos e nomes

Eventos com script para `claude-code`: `session-start`, `user-prompt-submit`, `pre-tool-use`, `post-tool-use`, `pre-compact`, `stop`, `session-end`, `subagent-start`, `subagent-stop` (HSC §1.1). Não há `post-compaction` para Claude Code. Esse evento é específico do Devin (HSC §1.4).

O valor de `event` na query é o canônico aceito pelo servidor. Exemplo: o script `user-prompt-submit` envia `user-prompt` (HSC §1.1). As formas em kebab-case, snake_case e PascalCase são aceitas (LEI §3.1).

| Evento do hook | `event` canônico (LEI §3.1) | Importância (LEI §3.2) | Limite de corpo (LEI §3.6) |
|---|---|---|---|
| SessionStart | início de sessão | 7 | 16 KiB |
| UserPromptSubmit | prompt do usuário (`user-prompt`) | 8 | 16 KiB |
| PreToolUse | pré-ferramenta | 5 | 2 KB |
| PostToolUse | pós-ferramenta | 5 | 2 KB |
| PreCompact | pré-compactação | 6 | 16 KiB |
| Stop | parada | 6 | 2 KB |
| SessionEnd | fim de sessão | 7 | 16 KiB |
| SubagentStart | início de subagente (tipo de observação "outro") | 3 | 16 KiB |
| SubagentStop | fim de subagente (tipo de observação "outro") | 3 | 16 KiB |

A spec 002 exercita SessionStart, UserPromptSubmit, PostToolUse e SessionEnd no fluxo principal (spec.md, "Fluxo", passo 2). Os demais eventos entram no contrato por serem os do script do Claude Code no doc.

## 3. Campos do payload por evento

Todos os nomes abaixo são de topo do corpo JSON, salvo indicação.

### 3.1 Campos comuns a todos os eventos

| Campo | Uso no ai-memory | Fonte |
|---|---|---|
| `session_id` | Identificador de sessão nativo (detalhes na seção 4) | LEI §3.3, §8 |
| `cwd` | Diretório de trabalho (seção 5) | LEI §3.3; HSC §1.2 passo 4 |
| `occurred_at` | Horário original, lido só do topo do corpo (seção 9) | LEI §3.3 |
| `ingest_key` | Chave de idempotência (seção 9) | LEI §3.3; spec.md |
| `agent_id` ou `subagentType` (string não vazia) | Marca o evento como de subagente | LEI §3.3, "Subagente" |

Campos vistos no payload do Codex pela doc: `hook_event_name`, `model`, `permission_mode` (CDX, "Payload fields", marcado como "Observed" para o Codex). **Não definido no doc** para Claude Code. A leitura desses campos pelo servidor também não é definida, e o doc de ingestão não os usa. **Decisão nossa**: as fixtures podem incluí-los para realismo, e a normalização não deve depender deles. Fica para o Analyst confirmar.

### 3.2 Por evento

- **SessionStart**
  - O título vem de `model`, depois `title`. Não há corpo (LEI §3.4).
  - Sem `session_id`, o evento só é válido como início de sessão e um identificador novo é gerado (LEI §8, passo 1).
  - Na resposta do hook ao agente, o handoff vai em `additionalContext`, dentro da chave de saída específica de SessionStart. Sem handoff, o hook responde `{}` (HSC §1.3).
  - O hook faz GET síncrono em `/handoff` (HSC §1.2, passo 7).
- **UserPromptSubmit** (`user-prompt`)
  - O título e o corpo vêm de `prompt`, `message` ou `text`. A primeira chave encontrada vale, e aceita blocos `[{type:text,text}]` (LEI §3.4).
  - O doc cita `prompt` como campo lido pelo hook path no Codex (CDX, "Payload fields"). Para Claude Code vale a regra genérica da LEI §3.4.
- **PreToolUse**
  - Usa o esquema fechado do Claude Code (LEI §3.5). Exige a presença do objeto `tool_input` ou `args` no payload (caso contrário nenhum evento é emitido).
  - Título: `tool <família>`, com família em `file`, `search-list`, `non-file` ou `unknown`.
  - Corpo: `tool_family: <família>` mais `tool_call_id: <id>` quando `tool_use_id` existir e tiver de 1 a 128 caracteres ASCII em `[A-Za-z0-9_-]`.
- **PostToolUse**
  - Mesmas regras de nome, família e identificador do PreToolUse.
  - Acrescenta `outcome: unknown` para Claude Code. Se `tool_family == "unknown"`, o corpo contém **apenas** o cabeçalho (`tool_family: unknown`, `tool_call_id` opcional e `outcome: unknown`), **sem** o separador `---` e **sem** saída. Se `tool_family` pertencer a `file`, `search-list` ou `non-file`, acrescenta `\n---\n` e a saída extraída de `tool_response`, `tool_output`, `output` ou `result` (string direta, array de blocos ou JSON compacto), ou `(no output captured)` quando ausente (LEI §3.5).
- **PreCompact**: não há título nem corpo (LEI §3.4). Faz checkpoint da página de sessão (LEI §8, passo 11).
- **Stop**
  - Sem título nem corpo no caminho genérico (LEI §3.4).
  - Só nesse evento pode existir `last_assistant_message`, no topo do corpo, para Claude Code, Codex e OpenCode (LEI §5, "Campo bruto").
  - O campo é **removido incondicionalmente** antes de qualquer outra etapa. Nunca chega a log, spool, rede ou armazenamento (LEI §5).
  - O script de shell do Claude Code descarta o evento inteiro e responde `{}` se o payload bruto contiver essa chave (HSC §1.6).
  - O corpo da parada só é preenchido com o opt-in duplo: `capture_assistant` no servidor (padrão desligado) e instalação do cliente com captura. O excerto fica em 2000 bytes no cliente, com marca `_ai_memory_assistant` v1, e em 2 KB no servidor (LEI §5, §3.6).
- **SessionEnd**: título vazio; se o payload trouxer `reason` (string não vazia), o corpo é `reason: <primeira_linha>` (LEI §3.4). Segue o fim de sessão da LEI §8.3.
- **SubagentStart e SubagentStop**
  - Sem título nem corpo (LEI §3.4).
  - Encaminhados por POST sem injeção de contexto (HSC §1.5).
  - `drop_subagent` descarta com 202 o evento marcado ou de início ou fim de subagente (LEI §4.5).
  - O Claude Code **não** marca a sessão pai. Só os eventos marcados são descartados (LEI §4.5).

## 4. `session_id`

- O servidor procura nas chaves `session_id`, `sessionId`, `sessionID`, `session` e `conversationId`, e em caminhos aninhados comuns dos harnesses (LEI §3.3). O Claude Code usa `session_id`. As outras chaves valem para outros agentes.
- Se nada for encontrado no corpo, usa o parâmetro de query `session_id` (LEI §2.1 e §3.3).
- Se o valor não for UUID, vira UUID v5 no namespace OID, de forma determinística. Assim POST e GET da mesma sessão caem na mesma chave. Se já for UUID, é usado como está (LEI §3.3; HSC §1.4).
- Em `/hook/batch`, item sem identificador de sessão que não seja início de sessão é confirmado e descartado, e nunca gravado (LEI §2.2 e §8).
- A chave de ator combina o usuário e o `session_id` cru (LEI §8, passo 2).
- No lado MCP, a ponte `--session-aware` repassa `CLAUDE_CODE_SESSION_ID` e falha se o Claude não fornecer o id (`docs/mcp-install.md`, seção Claude Code). Não é parte do payload do hook.

## 5. `cwd`

- Procurado em `cwd`, `current_dir`, `working_dir`, `directory`, nas listas `workspacePaths` e `workspace_roots` (Cursor) e em caminhos aninhados. **O valor do corpo prevalece sobre o da query** (LEI §3.3).
- O script do hook lê o campo `cwd` do payload. Só o Devin tem fallbacks, e o Claude Code não tem nenhum (HSC §1.2, passo 4).
- O `cwd` alimenta a resolução de projeto (marcador `.ai-memory.toml`, repo-root ou basename) em LEI §7.1 e HSC §2.2.

## 6. `tool_name`, `tool_use_id`, `tool_input`, `tool_response`

- **`tool_name`**
  - É o nome da ferramenta em Claude Code e "agentes parecidos" (LEI §3.5).
  - No esquema fechado, o nome é normalizado (removendo caracteres não alfanuméricos ASCII e convertendo para minúsculas) para derivar a família (`file`, `search-list`, `non-file`, `unknown`) conforme a tabela completa de LEI §3.5 e §4.3. O título é `tool <família>`.
- **`tool_use_id`**
  - É o identificador da chamada (LEI §3.5). Vira `tool_call_id` no corpo quando tiver de **1 a 128 caracteres** e contiver apenas `[A-Za-z0-9_-]`. Caso contrário, é descartado silenciosamente.
- **`tool_input`**
  - Argumentos nunca entram em título e corpo no esquema fechado (LEI §3.3, §3.5).
  - Em `PreToolUse`, a presença de `tool_input` ou `args` é exigida para emitir a observação.
  - A política de captura lê as chaves documentadas em LEI §4.3 para extrair até 32 caminhos candidatos diretos (4096 caracteres cada) ou comandos de shell.
- **`tool_response`**
  - É a saída da ferramenta no `PostToolUse` (lida de `tool_response`, `tool_output`, `output` ou `result`).
  - O corpo respeita o teto de 2 000 bytes UTF-8 seguros depois da sanitização (LEI §3.6). Para Claude Code o `outcome` é sempre `unknown`, e ferramentas com `tool_family == "unknown"` omitem `---` e a saída.

## 7. Respostas do servidor que o hook precisa tratar

- `202 queued`: aceito. `202` com motivo: descarte reconhecido (`invalid marker aliases`, `capture policy dropped`, `cursor draft placeholder dropped`, `subagent capture dropped`). `429 hook queue full` e `429 hook source rate limited` (LEI §2.3).
- `/hook/batch`: até 256 itens `{url, body}`, 413 acima disso. A falha de processamento interrompe o lote e devolve 200 com `failed_index` (LEI §2.2).
- O hook tem timeout curto (1 segundo no hook de shell, LEI §1). A spec fixa o POST do hook em 2 s por padrão e configurável (spec.md, "Performance", ADR-04).

## 8. Privacidade e segurança no payload

- Segredo no payload é removido antes de gravar (spec.md, "Segredo no payload do hook").
- Os campos `payload`, `title` e `body` aparecem como `<redacted>` em log de debug (LEI §3.3, "Representação de depuração").
- O topo do corpo com `last_assistant_message` é removido sempre (LEI §5).

## 9. Outros campos de topo lidos

- `occurred_at`: lido só do topo do corpo. Vale se for positivo e no máximo 5 minutos à frente do relógio do servidor. Senão usa "agora". Não passa pela sanitização (LEI §3.3; spec.md, item `occurred_at`).
- `ingest_key`: de 1 a 64 caracteres entre letras ASCII, dígitos, `-` e `_`. Se inválida, é ignorada, sem erro 4xx (LEI §3.3).
- Marca de versão do Cursor no corpo faz o agente virar Cursor, mesmo com hook instalado como Claude Code (LEI §3.3). Não deve ocorrer nas fixtures do Claude Code.

## 10. Lacunas do doc, a decidir no plan

1. Esquema completo do payload do Claude Code (`hook_event_name`, `transcript_path`, `permission_mode`, `source` no SessionStart etc.). O doc só observa isso para o Codex (CDX).
2. Mapeamento nome de ferramenta para família.
3. Chaves do `tool_input` lidas pela política de captura (ver LEI §4.3 e §4.4).
4. Critério de "tool_use_id válido".
5. Se `tool_response` do Claude Code pode ser objeto e não string. O doc não diz. **Decisão nossa**: tratar objeto, string e ausente nas fixtures.

## 11. Fixtures de payload a versionar

O plan (plan.md, Contexto e decisões) diz que o adaptador de hooks do Claude Code fica isolado, com fixtures de payload versionadas. Sugestão de diretório: `testdata/hooks/claude-code/` (**decisão nossa**; o doc não fixa o caminho).

| Arquivo | Evento coberto | O que valida |
|---|---|---|
| `session_start.json` | SessionStart | `session_id` UUID, `cwd`, `model` como título |
| `session_start_no_session_id.json` | SessionStart | Geração de identificador novo (LEI §8, passo 1) |
| `user_prompt_submit.json` | UserPromptSubmit | `prompt` como título e corpo |
| `user_prompt_submit_content_blocks.json` | UserPromptSubmit | Blocos `[{type:text,text}]` |
| `pre_tool_use_file.json` | PreToolUse | Família `file`, `tool_use_id`, `tool_input` com caminho |
| `pre_tool_use_shell.json` | PreToolUse | Família `non-file`, comando de shell |
| `pre_tool_use_unknown_tool.json` | PreToolUse | Família `unknown` |
| `pre_tool_use_invalid_tool_use_id.json` | PreToolUse | Sem identificador válido: segue sem `tool_call_id` |
| `post_tool_use_success.json` | PostToolUse | `tool_response` e `outcome: unknown` |
| `post_tool_use_no_output.json` | PostToolUse | `(no output captured)` |
| `post_tool_use_large_response.json` | PostToolUse | Corte em 2 KB e sanitização |
| `post_tool_use_with_secret.json` | PostToolUse | Segredo removido antes de gravar |
| `pre_compact.json` | PreCompact | Checkpoint, sem título nem corpo |
| `stop.json` | Stop | Sem `last_assistant_message`; parada sem corpo |
| `stop_with_last_assistant_message.json` | Stop | Remoção incondicional do campo bruto; descarte no hook (HSC §1.6) |
| `stop_with_assistant_mark.json` | Stop | Marca `_ai_memory_assistant` v1 e opt-in duplo (LEI §5) |
| `session_end.json` | SessionEnd | Fim de sessão (LEI §8.3) |
| `subagent_start.json` | SubagentStart | Tipo de observação "outro"; `agent_id` |
| `subagent_stop.json` | SubagentStop | Idem; `drop_subagent` |
| `tool_event_from_subagent.json` | PreToolUse | Marca por `agent_id` ou `subagentType` (LEI §3.3, §4.5) |
| `cwd_in_body_vs_query.json` | qualquer | `cwd` do corpo prevalece sobre o da query |
| `session_id_non_uuid.json` | qualquer | UUID v5 determinístico no namespace OID |
| `occurred_at_valid.json` e `occurred_at_future.json` | qualquer | Horário original e limite de 5 minutos à frente |
| `ingest_key_valid.json` e `ingest_key_invalid.json` | qualquer | Chave de 1 a 64 caracteres; inválida é ignorada |
| `empty_object.json` | qualquer | Parsing tolerante: nenhum campo obrigatório |

Os payloads devem ser sintéticos, sem dados reais. O servidor deve receber as fixtures sem alteração, como o hook as repassa (HSC §1.2, passo 6).
