# Desktop landscape — apps × capacidades (matriz de pesquisa)

> Pesquisa de 2026-10-07, com verificação ao vivo registrada em
> [verification-2026-10.md](verification-2026-10.md). O plano de fases está em
> [plan.md](plan.md). Cada app tem uma nota própria com as fontes.

Esta matriz compara apps desktop e agentes de terceiros. Só duas coisas são
afirmadas como fato:

1. O que o próprio ai-memory faz para cada agente: instalação da configuração
   MCP, instalação dos hooks de captura e o transporte MCP escrito na
   configuração. Esses pontos foram conferidos no comportamento do projeto.
2. Tudo o mais sobre produtos de terceiros (recursos da interface, formatos
   internos, APIs de extensão, licença, sincronização com a CLI) está marcado
   como **a confirmar**. Não use essas células como fato até haver evidência.

## Como ler

- **Suporte do projeto**: a instalação do ai-memory escreve a configuração
  para o agente. A tabela abaixo lista o caminho, o transporte e os eventos
  registrados.
- **a confirmar**: não há evidência no projeto nem verificação registrada.
- Para o comportamento de instalação completo, veja [mcp-install.md](../mcp-install.md)
  e [support-matrix.md](../support-matrix.md).

## Suporte do projeto por agente

Esta seção reflete o que o ai-memory escreve. Não diz se o app de terceiros
dispara esses eventos.

| Agente | Configuração MCP escrita | Transporte MCP | Hooks de captura | Eventos registrados |
| --- | --- | --- | --- | --- |
| Claude Desktop | Arquivo `claude_desktop_config.json`. Caminhos: macOS em Application Support/Claude; Windows em AppData (instalação comum ou pacote MSIX; com mais de um pacote MSIX, é preciso escolher o ativo por opção de caminho); Linux (beta) em `$XDG_CONFIG_HOME/Claude/`, padrão `~/.config/Claude/`. | Sem HTTP direto no arquivo. Usa um shim stdio de terceiros, com o cabeçalho Authorization passado por variável de ambiente. Exige o runtime instalado. | Nenhum alvo de hooks para este app. | Nenhum |
| Claude Code (CLI, base do Code tab) | Comando `claude mcp add`. | a confirmar (transporte não conferido nesta revisão) | `settings.json` do Claude Code, em `$CLAUDE_CONFIG_DIR` ou `~/.claude/`. | A confirmar (lista de eventos não conferida nesta revisão) |
| Codex (CLI; app desktop a confirmar) | `config.toml` em `$CODEX_HOME`, padrão `~/.codex/`. Entrada `mcp_servers` com HTTP por transporte streamable; o token vai por variável de ambiente ou cabeçalho estático, nunca literal. | Streamable HTTP. | `hooks.json` em `$CODEX_HOME`, padrão `~/.codex/hooks.json`. | 7: SessionStart, UserPromptSubmit, PreToolUse, PostToolUse, PreCompact, Stop, SessionEnd |
| Antigravity CLI (`agy`) | `~/.gemini/config/mcp_config.json`, campo `serverUrl`, com timeout em milissegundos. | HTTP (`serverUrl`). | `~/.gemini/config/hooks.json`. | 4: PreToolUse e PostToolUse (captura de ferramenta), PreInvocation (início de sessão), Stop (fim de ciclo) |
| Cursor (IDE) | `~/.cursor/mcp.json` ou `.cursor/mcp.json` no workspace, campo `url` com cabeçalhos. | HTTP (`url`). | `~/.cursor/hooks.json`. | A confirmar (lista de eventos não conferida nesta revisão) |
| Zed | Chave de nível superior `context_servers` no `settings.json`. Caminhos: macOS e Linux em `~/.config/zed/` (ou `$XDG_CONFIG_HOME`); Windows em `%APPDATA%\Zed\`. | HTTP (`url` com cabeçalhos). | Nenhum. O projeto declara o agente como somente MCP. | Nenhum |
| Grok Build CLI | `~/.grok/config.toml` (ou `$GROK_HOME`), tabela `[mcp_servers.<nome>]` com `url` e `headers`. | HTTP. | `~/.grok/hooks/ai-memory.json`. | A confirmar (lista de eventos não conferida nesta revisão) |

Observações sobre o comportamento do projeto:

- Entre os clientes da tabela, o Claude Desktop é o único que usa shim stdio
  para falar com o endpoint HTTP do ai-memory. Os demais recebem a entrada
  HTTP direta.
- Para Cursor, o projeto trata a duplicidade de eventos: se a CLI do Cursor
  também executa os hooks do Claude Code, o evento só é contado uma vez
  quando o hook nativo do Cursor está instalado.
- Para Grok Build CLI e Zero, o projeto registra que o handoff de início de
  sessão não é injetado pelo hook, porque o agente ignora a saída do hook.
  O handoff é recuperado pela ferramenta MCP de aceitação de handoff.
- Não há alvo de instalação para ChatGPT hospedado, Grok webapp nem Grok Bot.
  Os agentes Grok da tabela acima são a CLI.

## Matriz de apps de terceiros

Colunas com o mesmo significado da versão anterior. Onde há suporte do projeto,
a célula diz o que o projeto faz. O resto está como **a confirmar**.

| App | MCP client | Remote HTTP MCP | Lifecycle hooks | Local transcript store | Plugin/extension API | Open source | Session bridge to CLI |
| --- | --- | --- | --- | --- | --- | --- | --- |
| **Claude Desktop** (abas Chat, Cowork e Code; Linux beta) | Chat: arquivo de configuração com shim stdio (suporte do projeto). Conectores pela interface e Code tab: a confirmar. | Não pelo arquivo de configuração (suporte do projeto usa shim). Conectores e Code tab: a confirmar. | Code tab: a confirmar. O projeto não tem alvo de hooks para o app; os hooks do Claude Code ficam em `settings.json`. Chat e Cowork: a confirmar. | a confirmar | a confirmar | a confirmar | a confirmar |
| **Codex desktop** (app desktop; identidade com o app de chat e plataformas a confirmar) | Arquivo `config.toml` compartilhado com a CLI (suporte do projeto). Interface do app: a confirmar. | Sim no arquivo (streamable HTTP, suporte do projeto). Comportamento no app: a confirmar. | Hooks em `hooks.json` com 7 eventos (suporte do projeto). Disparo pelo app: a confirmar. | a confirmar | a confirmar | a confirmar | a confirmar |
| **Antigravity IDE** (IDE e CLI `agy`) | CLI: arquivo `mcp_config.json` (suporte do projeto). IDE e `.agents/mcp_config.json`: a confirmar. | Sim na CLI (`serverUrl`, suporte do projeto). IDE: a confirmar. | CLI: `hooks.json` com 4 eventos (suporte do projeto). IDE e `.agents/hooks.json`: a confirmar. O payload traz `conversationId` e `transcriptPath` (confirmado); `workspacePaths`: a confirmar. | Localização do transcript: a confirmar. O projeto lê o `transcriptPath` para achar a saída dos passos. | a confirmar | a confirmar | a confirmar |
| **ChatGPT desktop** (lado do chat hospedado) | Sem alvo de instalação no projeto. | a confirmar | a confirmar | a confirmar | a confirmar | a confirmar | a confirmar |
| **Grok webapp** (launcher local) | Sem alvo no projeto. | a confirmar | a confirmar | a confirmar | a confirmar | a confirmar | a confirmar |
| **Grok Bot** | Sem alvo no projeto. | a confirmar | a confirmar | a confirmar | a confirmar | a confirmar | a confirmar |
| **Cursor** (app desktop e CLI `cursor-agent`) | `~/.cursor/mcp.json` ou `.cursor/mcp.json` com `url` (suporte do projeto). App desktop: a confirmar. | Sim (campo `url`, suporte do projeto). | `~/.cursor/hooks.json` (suporte do projeto). Execução dos hooks do Claude Code pelo app: a confirmar; o projeto trata essa duplicidade para a CLI. | `transcript_path` no payload: a confirmar | a confirmar | a confirmar | a confirmar |
| **Zed** | `context_servers` no `settings.json` (suporte do projeto). Extensões MCP: a confirmar. | Sim (campo `url` com cabeçalhos, suporte do projeto). | Não. O projeto declara o agente como somente MCP. Agentes ACP e de terminal: a confirmar. | a confirmar | a confirmar | a confirmar | a confirmar |

## Oportunidades de captura (hipóteses)

Esta lista não é fato. Cada item depende de confirmação, e não há código do
projeto que a sustente hoje.

1. Claude Desktop, aba Code: reutilizar os hooks do Claude Code. A confirmar.
2. Codex desktop: disparo dos hooks de `hooks.json` pelo app. A confirmar.
3. Antigravity IDE: reutilizar `hooks.json` da CLI. O projeto cobre só a CLI.
4. Zed: captura por agentes ACP ou de terminal, cada um com seus hooks. A confirmar.
5. Aba Chat do Claude Desktop, ChatGPT hospedado e Grok Bot: só ferramentas MCP,
   sem captura automática. A confirmar.

## Afirmações sem evidência neste repositório

Estas afirmações aparecem em pesquisas de terceiros e não têm código do projeto
que as confirme. Ficam como **a confirmar** até haver verificação:

- Builds desktop oficiais para Linux do Claude Desktop (beta) e do app Codex.
- Grok Bot é um produto da Cursor, com suporte embutido a MCP e sem documentação
  pública.
- Transporte HTTP remoto é o padrão em Codex, Antigravity, Zed e Claude.
- Codex MCP com OAuth, CIMD ou DCR, e transporte stdio.
- Número de eventos de hooks do Codex (12, segundo a pesquisa).
- Antigravity IDE usa a mesma raiz de configuração da CLI.
- Layout do transcript do Antigravity em `brain/<conversationId>/`.
