# Cursor desktop

Pesquisa sobre o app desktop do Cursor (editor de terceiros, código fechado), feita para a RFC #878 em 2026-10-08. Cada afirmação sobre o produto foi conferida com o comportamento do projeto ou com teste registrado. Quando não há evidência, o texto diz "a confirmar".

## Visão geral

O agente do Cursor roda dentro do app e também pode ser usado pelo terminal. Os dados de configuração ficam em `~/.cursor/`.

O projeto integra o Cursor por três caminhos:

- **Hooks de ciclo de vida**: o instalador grava `~/.cursor/hooks.json`, e o Cursor chama o ai-memory em cada evento.
- **Servidor MCP**: o instalador grava uma entrada em `~/.cursor/mcp.json`.
- **Hooks do Claude Code**: o Cursor também executa os comandos de `~/.claude/settings.json`, e o projeto trata esses eventos como eventos do Cursor quando identifica a origem.

## Instalação dos hooks

- Sem a opção de aplicar, o instalador imprime o trecho de configuração e não altera arquivo algum.
- Com a opção de aplicar, o instalador altera `~/.cursor/hooks.json`, ou o caminho passado em `--config-file`. Antes de cada escrita que altera o arquivo, grava uma cópia de segurança ao lado do original.
- O arquivo tem o formato `{"version": 1, "hooks": {...}}`. Cada evento recebe uma lista de entradas planas, com `type: "command"` e o comando no mesmo nível. O formato usado pelos agentes que aninham os comandos em uma lista interna não se aplica aqui.
- A instalação faz merge: remove apenas as entradas do ai-memory do evento, acrescenta as novas e mantém os hooks de terceiros do mesmo evento. Rodar a instalação de novo não duplica entradas.
- Se houver entrada de terceiros que mencione o ai-memory sem ser reconhecida como do ai-memory (por exemplo, um invólucro que injeta o `cwd`), o instalador emite um aviso, porque o evento poderia ser capturado duas vezes.
- Com o modo de captura `allowlist`, o instalador avisa que repositórios sem marcador `.ai-memory.toml` não emitem eventos de ciclo de vida. O efeito disso em janelas sem pasta de trabalho é a confirmar.

### Eventos instalados

São instaladas oito entradas, que usam sete scripts. `postToolUse` e `postToolUseFailure` usam o mesmo tratamento.

| Evento do Cursor | Evento canônico | Observação |
|---|---|---|
| `sessionStart` | SessionStart | Também busca handoff pendente (ver abaixo) |
| `sessionEnd` | SessionEnd | |
| `beforeSubmitPrompt` | UserPrompt | |
| `preToolUse` | PreToolUse | |
| `postToolUse` | PostToolUse | |
| `postToolUseFailure` | PostToolUse | Mesmo script de `postToolUse` |
| `preCompact` | PreCompact | |
| `stop` | Stop | |

### Comando instalado

- Por padrão, o comando chama o binário do ai-memory com `hook --event <evento> --agent cursor --server-url <url>`. O nome do evento vem do nome do script, por exemplo `session-start` ou `user-prompt-submit`. O parser aceita os dois nomes para o mesmo evento canônico.
- No Windows, o comando usa o caminho do binário entre aspas duplas, na forma de string. Para o Cursor não há forma sem aspas.
- Scripts de compatibilidade só são usados quando a variável `AI_MEMORY_HOOK_PLATFORM` força o modo `posix` ou `windows`. Nesse caso, os scripts são copiados para `<diretório de dados>/hooks/cursor` e o comando os executa.
- Mesmo com o comando nativo, os scripts são copiados para o diretório de dados.

### Comportamento dos scripts

- Cada script lê o JSON do evento da entrada padrão e envia um POST para `/hook?event=<evento>&agent=cursor`, mais os parâmetros do marcador `.ai-memory.toml` encontrado subindo a partir do `cwd`.
- O endereço do servidor vem de `AI_MEMORY_HOOK_URL`. Se a variável não estiver definida, o padrão é `http://127.0.0.1:49374`.
- O envio é "dispara e esquece": erros são ignorados e o script sempre termina com sucesso. Assim, o script nunca bloqueia o agente.
- O script de `sessionStart` também faz um GET em `/handoff?agent=cursor`, com o `session_id` e os parâmetros do marcador, e imprime o resultado na saída padrão. A intenção é que a saída seja injetada na sessão seguinte. Se o Cursor usa essa saída, é a confirmar.

## Instalação do MCP

- `install-mcp --client cursor` sem a opção de aplicar imprime o trecho. Com a opção de aplicar, grava em `~/.cursor/mcp.json`, ou no caminho de `--config-file`.
- A chave usada é `mcpServers`. O nome padrão da entrada é `ai-memory`.
- A entrada tem `url`, com padrão `http://127.0.0.1:49374/mcp`. Quando há token, inclui `headers.Authorization`.
- O arquivo de projeto `.cursor/mcp.json` não é gravado pela ferramenta. O trecho impresso menciona esse caminho, e o usuário pode apontá-lo com `--config-file`.
- O trecho recomenda reiniciar o Cursor ou desligar e ligar o servidor em Configurações → MCP. Se a recarga automática funciona de forma confiável, é a confirmar.

## Como o servidor interpreta os eventos do Cursor

- **Identificação do agente**: se o corpo do evento tem `cursor_version`, o agente é `cursor`, mesmo que a query diga outro agente.
- **Sessão**: o identificador vem de `session_id`. Também são aceitos `sessionId`, `sessionID`, `session` e `conversationId`. O campo `conversation_id` não é lido pelo código. Se os payloads dos hooks nativos trazem `session_id`, é a confirmar. Sem identificador no corpo, o servidor usa o `session_id` da query.
- **Pasta de trabalho (`cwd`)**: o servidor usa `cwd` do corpo, ignorando valor vazio. Sem ele, usa o primeiro item de `workspace_roots`. Só então usa o `cwd` da query. Sem nenhum deles, a sessão cai no projeto padrão do servidor (`default/scratch`).
- **Nomes de evento aceitos**: `beforeSubmitPrompt`, `preToolUse`, `postToolUse`, `postToolUseFailure`, `preCompact`, `sessionStart`, `sessionEnd`, `stop`, `subagentStart` e `subagentStop`. Qualquer outro nome vira "outro". `subagentStart` e `subagentStop` são reconhecidos, mas o instalador não os registra.
- **Corpo de evento de ferramenta**: o título tem a forma `tool: <nome>`, um separador e a saída. A saída vem de `tool_output`, `tool_response`, `output` ou `result`. Se não houver saída, usa `error`. Se nada existir, usa o texto fixo `(no output captured)`.
- **Horário de ocorrência**: o campo `occurred_at` é aceito só se for positivo e no máximo 5 minutos à frente do relógio do servidor. Caso contrário, é descartado.
- **Rascunho vazio**: um `sessionStart` do Cursor com `session_id` igual a `empty-state-draft` é descartado antes da ingestão. A resposta é 202 com o texto `cursor draft placeholder dropped`. O teste de 2026-10-08 registrou uma sessão criada por replay em um servidor de teste. Se o build desse teste tinha essa regra, é a confirmar.

### Duplicação com os hooks do Claude Code

- O Cursor executa também os comandos de `~/.claude/settings.json`. Eles foram instalados com `--agent claude-code`, então o evento chega ao servidor com a identidade errada.
- Quando o corpo traz `cursor_version` e o agente declarado é `claude-code`, o servidor marca o evento como `cursor`.
- O cliente de hook descarta a cópia `claude-code` quando o arquivo `~/.cursor/hooks.json` tem uma entrada do ai-memory com `--agent cursor`. Essa verificação lê no máximo 1 MiB. Se o arquivo não existe, não pode ser lido ou está malformado, a cópia é mantida.
- No app desktop, a execução dos hooks do Claude Code foi observada só no `sessionStart` ao abrir o app. Já a existência de uma opção para ativar ou desativar essa importação, o padrão dela e a lista de eventos do Claude que o Cursor converte são a confirmar.

## Payload observado em teste ao vivo (2026-10-08)

O teste rodou duas conversas na janela vazia do app desktop, com os hooks do Claude Code instalados. Os campos abaixo foram observados no teste. Só os listados como lidos pelo código têm uso no projeto.

- **Sequência por conversa**: `sessionStart`, depois `beforeSubmitPrompt`, depois `preToolUse` e `postToolUse` quando o agente roda uma ferramenta, e por fim `stop`. Com a janela aberta, não houve `sessionEnd`.
- **Campos lidos pelo código**: `session_id` (igual a `conversation_id`), `workspace_roots` (vazio na janela vazia), `cursor_version`, `prompt`, `tool_name` (por exemplo `Shell`), `tool_input`, `tool_use_id` e `tool_output`, que é uma string JSON e não o `tool_response` do Claude.
- **Campos não usados pelo código**: `conversation_id`, `generation_id`, `model`, `model_id`, `user_email`, `composer_mode`, `is_background_agent`, `duration`, `attachments`, `status`, `loop_count` e contagem de tokens. Nenhum deles tem uso no código do projeto.
- **`transcript_path`**: só aparece no `stop`, com um caminho de transcrição em JSONL. O código não lê esse campo. A estrutura de pastas do transcrito é a confirmar.
- **Conversa recusada pelo plano**: deixou no transcrito só um marcador de término com erro e não disparou nenhum hook de ferramenta nem `stop`.
- **Janela vazia**: o servidor do teste criou sessões do Cursor sem `cwd` no projeto padrão, com os prompts guardados. Os eventos de ferramenta foram guardados com corpo vazio e títulos genéricos `pre-tool-use` e `post-tool-use`. Os eventos equivalentes do Claude Code, no mesmo servidor, receberam resumos da política de captura.
- **Servidores MCP**: a janela vazia carregou só os servidores internos do Cursor. Os servidores `ai-memory` e outros configurados no Claude Code não foram carregados.

## Questões em aberto

1. **Corpo vazio nos eventos de ferramenta do Cursor**: a causa não foi isolada. O código lê `tool_name` e `tool_output` para esses eventos, então a diferença não vem do nome do campo. Dar um `cwd` real não mudou o resultado, e `Shell` é classificado como ferramenta sem arquivo. Próximo passo: teste automatizado que reproduza o replay.
2. **Payloads dos hooks nativos** (`~/.cursor/hooks.json`): só foi observada a cópia no formato do Claude Code. Falta observar o `cwd` e o `session_id` depois de abrir uma pasta de trabalho.
3. **CLI do Cursor**: se o agente de terminal carrega os hooks no formato do Claude Code. Exige login prévio.
4. **Servidores MCP de janelas com pasta**: se uma janela com pasta importa os servidores do Claude Code. A confirmar.
5. **Rascunho vazio**: se o descarte deve continuar. Hoje o código descarta. Decisão de produto.
6. **Saída do `sessionStart`**: se o Cursor injeta a saída do script de handoff no contexto da sessão. A confirmar.
7. **Códigos de saída e falha**: se o Cursor bloqueia uma ação quando um hook sai com código 2, e se há uma opção para falhar fechado. Não há evidência no projeto. A confirmar.

## Links relacionados

- [Captura e deduplicação](capture-and-dedup.md)
- [Panorama dos apps desktop](desktop-landscape.md)
- [Verificação de outubro de 2026](verification-2026-10.md)
