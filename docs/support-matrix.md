# Matriz de suporte

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

Esta matriz diz, para cada plataforma e cada agente (ou harness), o que é suportado: instalação de hooks, captura de eventos, MCP, skills, transcrição e execução gerenciada. Os nomes dos comandos e das opções são os da interface atual. Detalhes de produtos de terceiros estão nas páginas de [`desktop/`](desktop/claude-desktop.md) e não são repetidos aqui.

Uma célula marcada como "a confirmar" significa que o comportamento não foi comprovado na implementação. Não deve ser lida como suporte.

## Legenda de status

| Status | Significado |
|---|---|
| Suportado | Há instalador, integração e cobertura no projeto para os itens listados. |
| Parcial | Há integração, mas um item de captura ou de entrega de contexto não está disponível. |
| Só MCP | O agente é conectado só pelo protocolo MCP. Não há hooks de ciclo de vida. |
| Só hooks | Há hooks de ciclo de vida, sem entrada MCP instalada pelo projeto. |
| Gerenciado | Disponível só pela execução gerenciada (`ai-memory run`). Não há instalador de hooks. |
| Experimental | Suportado, mas com cobertura de testes ou de release mais limitada. |
| Comunidade | Integração mantida fora do projeto. O projeto não instala nem testa. |

## Plataformas

| Plataforma | Status | Como se instala e o que está coberto |
|---|---|---|
| Linux | Suportado | Binários `tar.gz` para `x86_64` e `aarch64`. Imagens Docker para `linux/amd64` e `linux/arm64`. Pacote RPM. Pacote Arch (AUR) com unidades systemd de sistema e de usuário. CI roda os testes em Linux em todo pull request. |
| macOS | Suportado | Binários `tar.gz` para `aarch64` e `x86_64`, com unidade launchd por usuário. Os testes do workspace no macOS rodam só com o rótulo `full-ci` ou por disparo manual, e são obrigatórios antes de release. Há um app de barra de menus para macOS, compilado localmente, que empacota o binário, inicia a unidade launchd e abre a interface web e o status (ver [`desktop/installer-and-coexistence.md`](desktop/installer-and-coexistence.md)). |
| Windows nativo | Experimental | Zip `ai-memory-windows-x86_64.zip` com o executável `ai-memory.exe`. Hooks usam o binário diretamente por padrão. Scripts de PowerShell e Bash são o caminho de compatibilidade. Há um wrapper em PowerShell para usar a imagem Docker no Docker Desktop. A CI nativa roda todo dia, sob demanda e em pull requests que tocam caminhos de Windows ou recebem o rótulo `windows`. Ver [`windows.md`](windows.md). |
| Windows via WSL2 | Caminho Linux (a confirmar) | Usa-se o caminho de instalação do Linux dentro do WSL2 quando o agente roda lá. Não há teste específico para WSL2 na CI. |

## Agentes e harnesses: instalação

Os nomes da coluna de instalação são os aceitos pelos comandos `install-hooks`, `install-mcp` e `install-skills`. Entre parênteses estão os aliases aceitos. "Sem" significa que o comando não tem destino para esse agente.

| Agente | Status | Hooks (`install-hooks --agent`) | MCP (`install-mcp --client`) | Skills (`install-skills --agent`) |
|---|---|---|---|---|
| Claude Code | Suportado | `claude-code` (padrão do comando). | `claude-code` (padrão do comando). Aceita `--session-aware`, só para este cliente. | `claude-code` (padrão do comando). Projeto em `.claude/skills`. Global em `$CLAUDE_CONFIG_DIR/skills`, ou `~/.claude/skills` sem essa variável. `both` instala também em `.agents/skills`. |
| Codex | Suportado | `codex`. | `codex`, em `$CODEX_HOME/config.toml` (padrão `~/.codex/config.toml`). | Via `agents` (`.agents/skills`). A leitura desse diretório pelo Codex é a confirmar. |
| Command Code | Suportado | `command-code` (`commandcode`, `cmdc`, `cmd`). Grava em `~/.commandcode/settings.json`. | `command-code`, em `~/.commandcode/mcp.json`. | Sem. |
| Devin CLI | Suportado | `devin`. Grava em `~/.devin/hooks.v1.json`, ou na chave `hooks` de `~/.devin/config.json`. | `devin`, em `~/.devin/config.json`. | Projeto em `.devin/skills`. Global em `~/.devin/skills`. No Windows, global em `%APPDATA%/devin/skills`. |
| OpenCode (V1) | Suportado | `open-code` (`opencode`). Gera um plugin em `~/.config/opencode/plugins/`. | `open-code` (`opencode`). Entrada remota, formato V1. | Sem. |
| OpenCode 2 (beta) | Suportado (beta) | `opencode2` (`opencode-v2`, `open-code2`). Gera o plugin V2 no mesmo diretório. Compartilha diretório de configuração e sessões com o V1. | `opencode2`. Entrada aninhada em `mcp.servers`, com `type` remoto. | Sem. |
| Cursor | Suportado | `cursor`. Grava em `~/.cursor/hooks.json`. | `cursor`, em `~/.cursor/mcp.json` ou `.cursor/mcp.json`. | Sem. |
| Gemini CLI | Suportado | `gemini-cli`. Grava em `~/.gemini/settings.json`. | `gemini-cli`, em `~/.gemini/settings.json`. | Sem. |
| Oh My Pi (OMP) | Suportado | `omp` (`oh-my-pi`). Gera extensão em `~/.omp/agent/extensions/`, ou no diretório do perfil ativo. | `omp` (`oh-my-pi`), em `mcp.json` do diretório do agente OMP. | Sem. |
| Pi | Suportado | `pi`. Gera extensão em `~/.pi/agent/extensions/`. A mesma extensão faz a ponte HTTP para o MCP. | Sem. `install-mcp --client pi` não grava. Ele orienta a usar `install-hooks --agent pi --apply`. | Sem. |
| Crush | Gerenciado | Sem. | Sem. | Sem. |
| Kimi Code | Suportado | `kimi-code` (`kimi`). Grava blocos `[[hooks]]` em `$KIMI_CODE_HOME/config.toml`. | `kimi-code` (`kimi`), em `$KIMI_CODE_HOME/mcp.json`. | Sem. |
| Kiro CLI (v2) | Suportado | `kiro-cli` (`kiro`). Faz merge nas configurações de agente existentes, em `~/.kiro/agents/`. | `kiro-cli` (`kiro`), em `$KIRO_HOME/settings/mcp.json`. | Sem. |
| Kiro CLI (v3) | Suportado | `kiro-cli-v3` (`kiro-v3`). Grava o registro próprio em `$KIRO_HOME/hooks/ai-memory.json`. | Igual ao v2. | Sem. |
| Grok Build CLI | Suportado | `grok`. Grava em `$GROK_HOME/hooks/ai-memory.json`, ou em `~/.grok/hooks/ai-memory.json`. | `grok`, em `$GROK_HOME/config.toml`, ou em `~/.grok/config.toml`. | Projeto em `.grok/skills`. Global em `$GROK_HOME/skills`, ou `~/.grok/skills`. |
| Antigravity CLI | Suportado | `antigravity-cli` (`antigravity`, `agy`). Grava em `~/.gemini/config/hooks.json`. | `antigravity-cli` (`antigravity`, `agy`), em `~/.gemini/config/mcp_config.json`. A chave do servidor é `serverUrl`. | Sem. |
| Zero | Suportado | `zero`. Grava comandos em forma de execução direta em `$XDG_CONFIG_HOME/zero/hooks.json`. | `zero`, em `~/.config/zero/config.json`. Entrada HTTP nativa com bearer. | Sem. |
| ZCode | Suportado | `zcode` (`zai`). Grava na chave `hooks` de `~/.zcode/cli/config.json`. | `zcode`, na chave `mcp.servers` do mesmo arquivo. Entrada estrita: só `type`, `url` e `headers`. | Sem. |
| Pool (Poolside) | Só hooks | `pool` (`poolside`). Não grava arquivos. Imprime um bloco `hooks:` para colar em `.poolside/settings.yaml`, na raiz do repositório. | Sem cliente. | Sem. |
| Copilot CLI | Suportado | `copilot-cli`. Grava em `$COPILOT_HOME/hooks/ai-memory.json`, ou em `~/.copilot/hooks/ai-memory.json`. | `copilot-cli`, em `$COPILOT_HOME/mcp-config.json`. | Sem. |
| OpenClaw | Suportado | `openclaw`. Instala um plugin nativo com `--apply`. | `openclaw`, em `~/.openclaw/config.json`. | Sem. |
| Hermes Agent | Comunidade | `hermes` (`hermes-agent`). Imprime o bloco para `~/.hermes/config.yaml`, sem gravar o arquivo. O Hermes exige aceitação própria antes de rodar hooks. | Sem cliente. A confirmar. | `hermes`, em `.hermes/skills` (projeto) ou `~/.hermes/skills` (global). |
| Swival CLI | Só MCP | Sem. | `swival`, com `--apply`. Grava `.swival/mcp.json` na raiz do projeto, preservando outros servidores. | Sem. |
| Muse Code | Só MCP | Sem. | `muse`, com `--apply`. Grava em `~/.config/muse/settings.json`, na chave `mcp_servers`. | Sem destino próprio. O instalador indica `~/.agents/skills` (`agents`), que o Muse lê. |
| Claude Desktop | Só MCP | Sem. | `claude-desktop`. Usa uma ponte stdio de terceiros, porque o aplicativo não aceita transporte HTTP direto. | Sem. |
| VS Code Copilot | Só MCP | Sem. | `vscode-copilot` (`copilot`, `github-copilot`). Grava `.vscode/mcp.json` no workspace. | Sem. |
| Zed | Só MCP | Sem. | `zed`, com `--apply`. Grava a chave `context_servers` do `settings.json` do usuário. | Sem. |

Regras comuns a `install-mcp`:

- Sem `--apply`, o comando imprime o trecho de configuração e não altera arquivos.
- Com `--apply`, o comando grava o arquivo de forma idempotente. Ao atualizar um arquivo existente, cria um backup ao lado dele.
- O cliente Kiro exige HTTPS para servidores MCP remotos. HTTP simples só é aceito em loopback.
- Para o Muse, o arquivo precisa ter `schema_version` igual a 1. O comando adiciona esse campo quando falta e não reescreve um valor existente. A entrada usa `mode` igual a `optional`, para que o Muse não aborte a execução quando o servidor estiver fora do ar.

## Agentes e harnesses: captura e ciclo de vida

Os eventos canônicos são: início de sessão, prompt do usuário, antes e depois de tool, pré-compactação, pós-compactação, notificação, parada de turno, fim de sessão e início e fim de subagente. A coluna "Eventos instalados" lista o que o instalador registra no agente. Os nomes são os nativos do agente.

| Agente | Eventos instalados | Handoff entregue no início da sessão | Fim de sessão |
|---|---|---|---|
| Claude Code | `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `PreCompact`, `Stop`, `SessionEnd`, `SubagentStart`, `SubagentStop`. | Sim, pelo `SessionStart`. | `SessionEnd`. |
| Codex | `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `PreCompact`, `Stop`, `SessionEnd`. Sem eventos de subagente. | Sim, pelo `SessionStart`. | `SessionEnd`, presente a partir do Codex CLI 0.145.0. Em versões anteriores, use `finalize-session`. |
| Command Code | `SessionStart`, `PreToolUse`, `PostToolUse`, `Stop`. Só os eventos estáveis documentados. | Sim, pelo `SessionStart`. | Sem evento. Use `finalize-session --agent command-code` após o último turno. |
| Devin CLI | `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `PostCompaction`, `Stop`, `SessionEnd`. Sem eventos de subagente. | Sim, em `hookSpecificOutput.additionalContext`. | `SessionEnd`. |
| OpenCode (V1) | Plugin gerado: criação de sessão, mensagem do chat, antes e depois de tool, ociosidade (`Stop`), compactação (`PreCompact`) e exclusão (`SessionEnd`). | Sim, pelo `SessionStart`. | `SessionEnd`, a partir da exclusão da sessão. |
| OpenCode 2 (beta) | Plugin V2 gerado: prompt, antes e depois de tool, parada de turno com checkpoint, movimentação de sessão (vira `SessionStart`) e início de compactação. | Sim, pelo `SessionStart`. | Conforme o plugin V2. A confirmar em uso real. |
| Cursor | `sessionStart`, `sessionEnd`, `beforeSubmitPrompt`, `preToolUse`, `postToolUse`, `postToolUseFailure`, `preCompact`, `stop`. | Sim, pelo `sessionStart`. | `sessionEnd`. |
| Gemini CLI | `SessionStart`, `SessionEnd`, `BeforeTool`, `AfterTool`, `PreCompress`. Sem prompt e sem `Stop` na configuração instalada. | Sim, pelo `SessionStart`. | `SessionEnd`. |
| Oh My Pi (OMP) | Extensão gerada: início, início de turno (prompt), antes e depois de tool, compactação, fim de turno (`Stop`) e desligamento (`SessionEnd`). | Sim, pelo início da sessão. | Desligamento da sessão. |
| Pi | Extensão gerada: início, início de turno (prompt), antes e depois de tool, compactação, fim de turno e desligamento. | Sim, pelo início da sessão. | Desligamento da sessão. |
| Kimi Code | Os 9 eventos do Claude Code, mais `PostToolUseFailure`. Inclui início e fim de subagente. | Não pelo `SessionStart`. A entrega é pelo `UserPromptSubmit`, porque o `SessionStart` descarta a saída do hook. | `SessionEnd`. |
| Kiro CLI (v2) | `agentSpawn` (início), `userPromptSubmit`, `preToolUse`, `postToolUse`, `stop`. Sem `PreCompact`, sem fim de sessão e sem subagentes. | Sim, pelo início da sessão. A saída de `agentSpawn` vai para o contexto do modelo. | Sem evento. Use `finalize-session --agent kiro-cli`. Em sessões simultâneas, passe `--session-id` com um UUID. |
| Kiro CLI (v3) | `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `Stop`. Sem `PreCompact`, sem fim de sessão e sem subagentes. | Sim, pelo `SessionStart`. | Sem evento. Use `finalize-session --agent kiro-cli`. |
| Grok Build CLI | Os 9 eventos do Claude Code: início, prompt, antes e depois de tool, pré-compactação, parada, fim e início e fim de subagente. | Não pelo `SessionStart`. A entrega é pela saída do primeiro `PostToolUse` da sessão. Se a sessão não usar tool, recupere pelo MCP com `memory_handoff_list` e `memory_handoff_accept`. | `SessionEnd`. |
| Antigravity CLI | `PreInvocation` (vira início de sessão só quando `invocationNum` é 0), `Stop`, `PreToolUse`, `PostToolUse`. | Só na primeira invocação da conversa. Chamadas posteriores não consomem um handoff da próxima sessão. | Sem evento. Use `finalize-session --agent antigravity-cli`. |
| Zero | `sessionStart`, `sessionEnd`, `beforeTool`, `afterTool`, `specialistStart` (início de subagente), `specialistStop` (fim de subagente). | Não. A saída do início de sessão é descartada. Recupere pelo MCP. | `sessionEnd`. |
| ZCode | `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `Stop`. `PostToolUseFailure` grava na mesma trilha de captura, com o erro preservado. `PermissionRequest` não é instalado de propósito. | Sim, em `hookSpecificOutput.additionalContext`. | Sem evento. `Stop` é só fronteira de turno. Use `finalize-session --agent zcode`. |
| Pool | `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `Stop`. | Não. A injeção pelo início de sessão não foi demonstrada. Recupere pelo MCP. | Sem evento. Use `finalize-session --agent pool`. |
| Copilot CLI | `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `PreCompact`, `Stop`, `SessionEnd`, `SubagentStart`, `SubagentStop`. Os nomes são em PascalCase, como o formato compatível com o Claude Code. | Sim, pelo `SessionStart`, em `additionalContext` no nível superior da saída. | `SessionEnd`. |
| OpenClaw | Plugin nativo: início e fim de sessão, início de turno (prompt), antes e depois de tool, pré-compactação e fim de agente. | Sim, pelo início da sessão. | `session_end`. |
| Hermes Agent | `pre_tool_call` e `post_tool_call`. O fim de sessão só existe em plugin externo (`on_session_end`), que o projeto não instala. | Não. Recupere pelo MCP. | Sem evento instalado pelo projeto. |
| Crush | Sem hooks. | Não. O contexto portátil entra pela execução gerenciada. | A confirmar. |
| Swival, Muse, Claude Desktop, VS Code Copilot, Zed | Sem hooks. | Não. | Não há. |

Itens de captura comuns a todos os agentes com hooks:

- Na instalação local, o padrão é o comando nativo do binário, em Linux, macOS e Windows. Ele aplica a política de exclusão de captura (versão 1). As extensões e plugins gerados também aplicam a política.
- Os scripts de compatibilidade (`posix`, `windows`, `windows-bash`, selecionados por `AI_MEMORY_HOOK_PLATFORM`) não aplicam a política de captura. O instalador avisa quando a seleção cai nesse caminho.
- `--capture-mode denylist` é o padrão. Captura tudo, exceto o que um marcador `.ai-memory.toml` exclui. `--capture-mode allowlist` captura só repositórios com marcador. Repositório sem marcador não emite nenhum evento. Se a opção não for passada em um `--apply` posterior, vale o modo gravado antes.
- `--no-capture-prompts` e `--capture-prompts` só valem para Claude Code.
- A captura da mensagem final do assistente no `Stop` é opt-in em dois níveis: `install-hooks --capture-assistant` e `capture_assistant` no servidor. O instalador aceita o opt-in só para Claude Code, Codex e OpenCode 2, e só em plataformas com hooks nativos. O campo bruto é sempre removido antes de gravar. A entrada acima de 64 KiB conta como ausente. O trecho gravado tem no máximo 2000 bytes.
- Com a opção de descartar capturas de subagentes ativa no projeto, os eventos de subagente são descartados. O Claude Code marca o subagente pelo identificador de agente, e o Grok pelo tipo de subagente. Nos demais agentes, a sessão do subagente entra num conjunto. Os eventos seguintes dela, como prompt, parada e fim de sessão, também são descartados.
- No Cursor, a cópia redundante de um hook do Claude Code é descartada quando os hooks nativos do Cursor estão instalados.

## Execução gerenciada

`ai-memory run` é um lançador opt-in. Ele inicia o harness com um vínculo de workstream, importa a transcrição e mantém o histórico entre harnesses. Um lançamento direto do harness, sem `ai-memory run`, continua igual.

| Harness | Comando | Transcrição importada | Precisa de `finalize-session` |
|---|---|---|---|
| Claude Code | `claude` (qualquer nome que comece com `claude`) | Arquivo JSONL. | Não. |
| Codex | `codex` | Arquivo JSONL. | Não. |
| OpenCode | `opencode` (`open-code`) | Banco SQLite. | Não. |
| OpenCode 2 (beta) | `opencode2` | Banco SQLite. | Não. |
| Pi | `pi` | Arquivo JSONL da família Pi. | Não. |
| Crush | `crush` | Banco SQLite do projeto. Contexto entregue por um arquivo global temporário, apontado pela variável `CRUSH_GLOBAL_CONFIG`. | A confirmar. |
| Oh My Pi | `omp` (`oh-my-pi`) | Arquivo JSONL da família Pi. | Não. |
| Kimi Code | `kimi` (`kimi-code`, `kimi-cli`) | Arquivo JSONL. | Não. |
| Command Code | `command-code` (`commandcode`, `cmdc`, `cmd`) | Registro de sessão v3, com árvore de mensagens. Raciocínio oculto e metadados de provedor não são importados. | Sim. |
| Kiro CLI | `kiro` (`kiro-cli`). Motor v2 por padrão. | Arquivo JSONL do motor v2. | Sim. |
| Kiro CLI, motor v3 | `kiro` com `--v3`, `--mode`, ou `--agent-engine` com valor diferente de `v2`. | Arquivo JSONL do motor v3. | Sim. |
| Grok Build CLI | `grok` (`grok-build`) | Arquivo JSONL. Contexto entregue pela opção `--rules`. | Não. |
| Antigravity CLI | `antigravity` (`antigravity-cli`, `agy`). Retomada com `--conversation`. | Só os prompts digitados, lidos do histórico. Passos do assistente e de tools não são decodificados. O ledger vem da captura por hooks. | Sim. |

Regras da execução gerenciada:

- Os subcomandos utilitários de cada harness, como ajuda, versão ou configuração, passam direto, sem flags de retomada. No Command Code, o subcomando `mods` é tratado como utilitário e não abre sessão gerenciada.
- Sem harness na linha de comando, `ai-memory run` procura uma sessão nativa utilizável entre os harnesses automáticos: Claude Code, Codex, OpenCode, Pi, Crush, Kimi Code, Command Code e Kiro (v2 e v3). O executável do harness precisa estar disponível. Os harnesses OpenCode 2, OMP, Grok e Antigravity não entram nessa busca, e precisam de nome explícito.
- Se a busca encontra sessões ambíguas, a transcrição não é importada. Retome com o seletor de sessão nativo do harness para vincular.
- Opções de `--yolo` e de isolamento com a jaula de execução (`--jail`, `--no-jail`) estão descritas em [`design-yolo-safety-ai-jail.md`](design-yolo-safety-ai-jail.md).

Para adicionar um harness gerenciado, siga [`managed-harness-contributions.md`](managed-harness-contributions.md). O comportamento geral da execução está em [`managed-workstreams.md`](managed-workstreams.md).

## Provedores de LLM e autenticação

| Provedor | Autenticação | Notas |
|---|---|---|
| Anthropic | Chave de API. | API de mensagens. |
| Anthropic (OAuth) | Token de assinatura do Claude. | Usa o token de assinatura em vez de chave. |
| OpenAI | Chave de API. | API de chat completions. |
| OpenAI (OAuth) | Login do backend ChatGPT/Codex. | Armazenado no arquivo de autenticação compartilhado. |
| Codex | Reaproveita o login do Codex CLI. | Renovação delegada ao Codex CLI. |
| GitHub Copilot | Login do Copilot. | Backend do Copilot Chat. |
| Gemini | Chave de API. | API Generative Language. |
| Compatível com OpenAI | Sem chave, para endpoints locais. | Ollama, vLLM e LM Studio. |
| OpenCode | Chave da API cloud. | Plano Go por padrão. O plano Zen é selecionado pela URL base em `AI_MEMORY_LLM_BASE_URL`. |
| OIDC (device flow) | Login genérico em um emissor OIDC. | Dá um JWT por desenvolvedor para os hooks nativos, sem token compartilhado. Gravado no mesmo arquivo de autenticação, na chave `oidc`. |

## Provedores de embeddings

| Provedor | Notas |
|---|---|
| OpenAI | API de embeddings da OpenAI. |
| Voyage | API de embeddings da Voyage. |
| Google Gemini | Método `embedContent`. |
| Compatível com OpenAI | Pode ser sem chave. Exige URL base, modelo e dimensão. Ollama, LM Studio e vLLM. |
| Local (em processo) | Modelo all-MiniLM-L6-v2, com 384 dimensões. Sem chave e sem servidor. Os arquivos do modelo ficam em `models/` dentro do diretório de dados. A funcionalidade é compilada por padrão. |
| GitHub Copilot | Endpoint de embeddings compatível com OpenAI. Reaproveita o login do Copilot. |

Os vetores são gravados com o rótulo do provedor e do modelo. Uma consulta com provedor diferente do gravado é recusada. Para regerar os vetores com a configuração atual, use `embed --force`.

## Leitura adicional

- [`windows.md`](windows.md) para o caminho de instalação no Windows.
- [`mcp-install.md`](mcp-install.md) para o formato de configuração de cada cliente MCP.
- [`lifecycle-event-ingestion.md`](lifecycle-event-ingestion.md) para a normalização dos eventos.
- [`external-lifecycle.md`](external-lifecycle.md) para a captura de ciclo de vida fora do fluxo principal.
- [`harness-adapters.md`](harness-adapters.md) para os adaptadores de transcrição.
- [`auto-scope.md`](auto-scope.md) para o isolamento por sessão com `--session-aware`.
- [`marker-file.md`](marker-file.md) para o marcador `.ai-memory.toml`.
- [`llm-providers.md`](llm-providers.md) e [`local-embeddings.md`](local-embeddings.md) para os detalhes de provedores.
- [`windows-native-extras.md`](windows-native-extras.md) para o que fica fora do repositório principal no Windows.
- Páginas de produtos de terceiros: [`desktop/claude-desktop.md`](desktop/claude-desktop.md), [`desktop/cursor.md`](desktop/cursor.md), [`desktop/codex-desktop.md`](desktop/codex-desktop.md), [`desktop/zed.md`](desktop/zed.md), [`desktop/grok-desktop.md`](desktop/grok-desktop.md), [`desktop/antigravity.md`](desktop/antigravity.md) e [`desktop/extensions.md`](desktop/extensions.md).
