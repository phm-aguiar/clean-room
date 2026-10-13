# Grok: Grok Build CLI e superfícies desktop

> Escopo: o projeto cobre a Grok Build CLI, o agente de codificação da xAI.
> Não cobre o aplicativo de chat do Grok, a versão web nem o app desktop
> chamado Grok Bot. Tudo o que não tem evidência no projeto está marcado como
> "a confirmar".

## 1. Escopo por superfície

| Superfície | Coberta pelo projeto | Situação |
|---|---|---|
| Grok Build CLI | Sim: servidor MCP, hooks de ciclo de vida e skills | Verificada (seções 2 a 5) |
| Aplicativo de chat do Grok | Não | a confirmar |
| Versão web (grok.com) | Não | a confirmar, inclusive o suporte a MCP |
| Grok Bot (app desktop) | Não | a confirmar: origem, suporte a MCP, local de configuração e formato de entrada |
| App desktop oficial da xAI | Não | a confirmar |

A Grok Build CLI e o Grok Bot são produtos distintos. Não devem ser tratados
como um mesmo alvo de instalação. A estratégia de captura para apps sem hooks
está em [captura e deduplicação sem hooks](capture-and-dedup.md).

## 2. Grok Build CLI: servidor MCP

- **Comando:** `ai-memory install-mcp --client grok`. Sem `--apply`, imprime o
  trecho de configuração. Com `--apply`, altera o arquivo no lugar.
- **Arquivo:** `$GROK_HOME/config.toml`. Sem essa variável, o padrão é
  `~/.grok/config.toml`. `--config-file` sobrescreve o caminho.
- **Transporte:** HTTP nativo. A entrada gerada contém só a URL. Nenhum
  transporte por entrada e saída padrão é gerado para este cliente.
- **URL padrão:** `http://127.0.0.1:49374/mcp`. Se o usuário informar uma URL
  base sem o sufixo `/mcp`, o sufixo é acrescentado.
- **Nome padrão da entrada:** `ai-memory`.
- **Forma da entrada:** uma tabela de servidor com `url` e `enabled = true`.
  Com token, há uma subtabela `headers` com `Authorization: Bearer <token>`.
  A chave `http_headers`, usada por outro cliente, não é gerada para o Grok.
- **Idempotência:** reexecutar substitui só a entrada com o mesmo nome. Os
  demais servidores são preservados, e tabelas inline são convertidas para
  tabelas em bloco.
- **Backup:** antes de cada escrita com `--apply`, é criada uma cópia com
  carimbo de data e hora ao lado do arquivo original.
- **Schema do produto:** o projeto diz seguir a documentação de MCP do Grok.
  Os nomes de chave (`enabled`, `headers`) não foram conferidos na
  documentação do produto. **A confirmar.**
- **Alternativa por comando do produto** (`grok mcp add --transport http`):
  citada no próprio projeto. **A confirmar** no produto.

## 3. Grok Build CLI: hooks de ciclo de vida

### 3.1 Instalação

- **Comando:** `ai-memory install-hooks --agent grok`. Sem `--apply`, imprime a
  configuração. Com `--apply`, grava `$GROK_HOME/hooks/ai-memory.json`. Sem a
  variável, o padrão é `~/.grok/hooks/ai-memory.json`.
- **Arquivo próprio:** o projeto grava um arquivo separado no diretório de
  hooks, sem editar arquivos de terceiros. Que o Grok carrega todos os `*.json`
  desse diretório: **a confirmar.**
- **Idempotência:** a mutação substitui só as entradas do projeto e preserva as
  demais. Antes de cada escrita, é criada uma cópia com carimbo de data e hora.
- **Scripts:** o instalador copia os scripts de hook para o diretório de dados
  e grava, para cada evento, um comando que aponta para essa cópia. O comando
  inclui a URL do servidor, o token (se informado) e o diretório de dados.

### 3.2 Eventos

São nove eventos, com o mesmo vocabulário do perfil de hooks do Claude Code:

| Evento do Grok | Parâmetro `event` enviado ao servidor |
|---|---|
| SessionStart | `session-start` |
| UserPromptSubmit | `user-prompt` |
| PreToolUse | `pre-tool-use` |
| PostToolUse | `post-tool-use` |
| PreCompact | `pre-compact` |
| Stop | `stop` |
| SessionEnd | `session-end` |
| SubagentStart | `subagent-start` |
| SubagentStop | `subagent-stop` |

Cada evento tem um script para POSIX e um para Windows.

### 3.3 Comportamento dos scripts

- Lê o payload da entrada padrão e extrai o diretório de trabalho.
- Envia o payload por POST para `<servidor>/hook?event=<evento>&agent=grok`,
  com os parâmetros de projeto derivados do diretório de trabalho.
- URL padrão do servidor: `http://127.0.0.1:49374`. A variável
  `AI_MEMORY_HOOK_URL` sobrescreve o padrão.
- Falhas no envio são ignoradas. Os scripts respondem com um objeto JSON vazio
  e saem com código 0, para que o agente não seja interrompido.
- Marcador de subagente: nos eventos de ferramenta disparados por um subagente,
  a chave `subagentType` identifica o subagente. O servidor usa esse marcador
  para associar a sessão ao subagente.
- O script de início de sessão não busca handoff. Aceitar um handoff é uma
  operação destrutiva, e a saída desse script é descartada pelo Grok (comentário
  do projeto). **A confirmar** no produto.

### 3.4 Limite: sem injeção de handoff no início de sessão

- A captura de eventos funciona. A injeção automática do handoff no início de
  sessão não funciona: o projeto assume que o Grok ignora a saída padrão do
  evento SessionStart. **A confirmar** no produto.
- **Recuperação manual:** a ferramenta MCP `memory_handoff_list` lista os
  handoffs abertos sem reivindicá-los nem expirá-los. A ferramenta
  `memory_handoff_accept` reivindica um deles pelo identificador.
- **Criação de handoff no fim da sessão:** não verificada nesta revisão.
  **A confirmar.**

### 3.5 Sessões

- O tipo de agente persistido é `grok`. O alias `grok-build` também é aceito.
  A comparação não diferencia maiúsculas de minúsculas.
- Bancos criados antes desse tipo recebem a ampliação do conjunto aceito por
  uma migração própria.

### 3.6 Configuração de projeto

- Se houver `.grok/config.toml` no diretório atual ou em um ancestral até a
  raiz do repositório, a inferência automática de servidor e token é recusada.
  Nesse caso, é preciso passar `--server-url` e `--auth-token` explicitamente.
- Sem repositório detectado, só o diretório atual é verificado.
- O projeto avisa que esse arquivo pode sobrescrever `GROK_HOME`. **A
  confirmar** a precedência no produto.

## 4. Grok Build CLI: skills

- O agente `grok` tem dois destinos: o diretório de skills do projeto
  (`.grok/skills`) e o diretório global (`$GROK_HOME/skills`, padrão
  `~/.grok/skills`).
- As regras de uso ficam em `AGENTS.md`.

## 5. Pendências (a confirmar)

- Schema do arquivo de MCP do produto: nomes de chave e sintaxe de headers.
- Alternativa por comando do produto para registrar o servidor MCP.
- Carregamento de todos os arquivos `*.json` do diretório de hooks.
- Schema completo do payload nativo dos eventos. O projeto lê só algumas
  chaves, como `tool_name`, `tool_input`, `tool_use_id` e `subagentType`.
- Criação de handoff no fim da sessão.
- Precedência entre configuração de projeto e `GROK_HOME`.
- Se o Grok carrega MCP de fontes de compatibilidade de outros clientes.
- Versão web e aplicativo de chat: suporte a MCP e a hooks.
- Grok Bot: origem, suporte a MCP, local de configuração e formato de entrada.
  Não há documentação pública localizada nesta revisão.
- App desktop oficial da xAI: não verificado.

## 6. Relacionados

- [Panorama dos apps desktop](desktop-landscape.md)
- [Captura e deduplicação sem hooks](capture-and-dedup.md)
- [Superfícies de extensão e plugins](extensions.md)
