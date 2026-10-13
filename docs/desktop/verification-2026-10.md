# Verificação ao vivo de apps desktop (2026-10)

> Registro de observações feitas em 2026-10-08 sobre o comportamento de apps
> de terceiros, em um ambiente de teste com pastas isoladas. A configuração
> global do operador não foi alterada. Este registro não é reproduzível a
> partir do projeto: o projeto só confirma o próprio lado do ai-memory (quais
> arquivos de hook e de MCP grava, como lê os dados recebidos e como responde).
> Afirmações sobre os apps estão marcadas como **a confirmar** ou foram
> removidas.

## Como ler este registro

- **Verificado no projeto**: o comportamento do ai-memory descrito é sustentado
  pela implementação e por testes do projeto.
- **A confirmar**: observação ao vivo ou afirmação de terceiros que o projeto
  não sustenta. Cada item da seção de limites diz como confirmar.

## Contrato de entrada dos hooks (verificado no projeto)

- O identificador de sessão e o diretório de trabalho são aceitos sob vários
  nomes de campo, em ordem fixa. Se o corpo não traz o diretório, o parâmetro
  da URL é usado.
- Um corpo com a chave `cursor_version` é tratado como sendo do Cursor, mesmo
  que a URL declare outro agente.
- Nenhum campo do corpo é obrigatório. Um corpo que não é JSON não gera
  captura; a resposta de sucesso é enviada mesmo assim.
- `occurred_at` só é aceito se for positivo e no máximo 5 minutos no futuro.

## Codex desktop

### Verificado no projeto

- O instalador grava os hooks em `hooks.json` dentro de `CODEX_HOME`, ou em
  `~/.codex/hooks.json` quando a variável não está definida. Não altera o
  arquivo de configuração principal do Codex.
- O instalador avisa que o Codex exige confirmação ("confiar") antes de
  executar hooks novos, e indica a opção de bypass para revisão manual.

### A confirmar

- Hooks instalados e não confiáveis não são executados, e o app desktop não
  avisa o usuário. Observado ao vivo no modo servidor do app; o aviso do
  instalador descreve a interface de terminal.
- Alterar o comando de um hook faz o Codex tratá-lo como alterado e exigir nova
  confirmação até ser confiado de novo.
- O app desktop e a CLI usam a mesma configuração, o mesmo arquivo de hooks e o
  mesmo estado de confiança. Observado com o launcher do app, que define
  `CODEX_HOME` quando a variável não existe.
- Payload dos hooks no app: as chaves recebidas são as mesmas da CLI. Nada no
  payload identifica o app desktop; a origem aparece apenas no registro da
  sessão.
- O evento de fim de sessão não dispara ao desinscrever a conversa; dispara
  quando a conversa é arquivada ou quando o app é encerrado. Uma conversa
  deixada aberta só recebe o fim de sessão nesses casos.
- Nenhuma sessão iniciada pelo app desktop foi registrada na máquina de teste.
  A verificação de ponta a ponta do app ainda não foi feita.

Ver também [Codex desktop](codex-desktop.md).

## Antigravity

### Verificado no projeto

- O instalador grava os hooks da CLI do Antigravity em
  `~/.gemini/config/hooks.json`. O evento `PreInvocation` é registrado como
  início de sessão, `Stop` como fim, e `PreToolUse` e `PostToolUse` como
  ferramentas.
- Um `PreInvocation` só conta como início de sessão quando o contador
  `invocationNum` é o inteiro 0. Valores ausentes, fracionários, negativos ou
  em texto (`"0"`) descartam o evento, sem gerar captura.
- O hook de `PreToolUse` responde `{"decision": "allow"}`, tanto no script de
  shell quanto no caminho nativo. A ferramenta não é bloqueada pelo projeto.
- No `PostToolUse`, quando o corpo não traz a saída da ferramenta, o projeto
  lê o texto do arquivo de saída do passo, dentro do diretório de artefatos.
  Isso só vale para estas ferramentas:
  - `run_command`, `view_file`, `list_dir`, `find_by_name`, `grep_search`,
    `search_web`, `manage_task`, `manage_subagents`.
- Ferramentas de edição (`write_to_file`, `replace_file_content` e similares)
  nunca têm a saída substituída, porque o conteúdo real da mudança vem dos
  argumentos. Ferramentas não listadas (`read_url_content`, `read_resource`,
  `call_mcp_tool`) também não são enriquecidas.
- Limites da leitura do arquivo de saída:
  - no máximo 2048 bytes;
  - só arquivo regular: links simbólicos e outros tipos são recusados;
  - o corte não quebra caracteres UTF-8;
  - espaços finais são removidos;
  - arquivo vazio ou só com espaços resulta em nenhuma saída.
- O número do passo (`stepIdx`) é aceito como inteiro ou como texto numérico.
- O diretório de artefatos vem de `artifactDirectoryPath`, com `~` expandido.
  Se o campo falta, o projeto deduz o diretório a partir do caminho do
  transcript, quando esse arquivo fica dentro de `.system_generated/logs/`.
- O enriquecimento só ocorre quando `tool_response` está vazio.
- O projeto não grava hooks por workspace (`.agents/hooks.json`). Só o caminho
  global é gravado.

### A confirmar

- `invocationNum` volta a 0 a cada turno do usuário dentro da mesma conversa
  (observado). Se confirmado, o início de sessão e a busca de handoff se
  repetem a cada turno. Pelo projeto, o Antigravity CLI injeta handoff no início
  de sessão, e o contador 0 é admitido. Se o envio por turno é aceitável é uma
  decisão de produto em aberto.
- Um `PreToolUse` sem o campo `decision` faz a ferramenta ser negada, e o
  `PostToolUse` não dispara (observado com um hook de registro que responde
  `{}`). Os hooks do projeto respondem `allow`, então este caso não se aplica a
  eles.
- O `PostToolUse` traz o nome da ferramenta, `stepIdx` e erro, mas não a saída
  (observado). A lista exata de campos é a confirmar.
- O caminho do transcript observado termina em `transcript_full.jsonl`, e a
  documentação de terceiros cita `transcript.jsonl`. O projeto aceita qualquer
  nome dentro de `.system_generated/logs/`.
- Na impressão em modo não interativo (`-p`), o histórico global não recebe
  linha para o prompt, mas o transcript recebe. A confirmar.
- O IDE do Antigravity lê `~/.gemini/config/hooks.json` e os hooks por
  workspace. O projeto não tem tratamento específico para o IDE, e o IDE não foi
  exercitado nesta verificação.

Ver também [Antigravity](antigravity.md).

## Claude Desktop

### Verificado no projeto

- O instalador de hooks do Claude Code grava em `settings.json` dentro de
  `CLAUDE_CONFIG_DIR`, ou em `~/.claude/settings.json` quando a variável não
  existe.
- O instalador de MCP escolhe o arquivo de configuração do Claude Desktop pelo
  sistema:
  - macOS: pasta de suporte de aplicativos, em `Claude/`;
  - Windows: pasta `%APPDATA%\Claude\`, ou o pacote MSIX quando instalado,
    com `--config-file` para escolher entre pacotes;
  - Linux: `$XDG_CONFIG_HOME/Claude/claude_desktop_config.json`, com
    `~/.config` como padrão.
- Como o Claude Desktop não aceita servidores HTTP diretamente na
  configuração, o instalador usa uma ponte stdio de terceiros, que exige um
  runtime instalado.
- O instalador pede que o app seja encerrado por completo e aberto de novo
  após a edição. Verificar atualizações não basta.

### A confirmar

- Sessões da aba Code executam os hooks de `settings.json` do Claude Code.
  Observado em transcrições de 2026-10-02 a 10-04; não reproduzível a partir
  do projeto.
- O Cowork não foi executado na máquina de teste. Não há evidência local de
  hooks ou MCP dentro do Cowork.
- A aba Chat não tinha servidores MCP configurados na máquina de teste. A
  documentação de terceiros diz que a definição alcança o Chat e a aba Code,
  com preferência da aba Code em caso de nome repetido.
- O suporte oficial do app em Linux, e a lista de comandos de desktop, vêm da
  documentação de terceiros.
- A documentação de terceiros apresenta pacotes `.mcpb` como forma principal
  de instalar MCP local. Isso afeta o que [Claude Desktop](claude-desktop.md)
  deve recomendar, e está em aberto.

## Cursor

### Verificado no projeto

- O instalador grava os hooks nativos do Cursor em `~/.cursor/hooks.json`.
- Quando hooks nativos do Cursor estão instalados, a cópia de um hook do
  Claude Code disparada pelo Cursor é descartada. Assim fica uma observação por
  evento do Cursor.
- Um corpo com `cursor_version` é tratado como Cursor, mesmo que a URL declare
  Claude Code.
- O diretório de trabalho do Cursor vem de `workspace_roots` ou
  `workspacePaths`, quando `cwd` está ausente ou vazio. O primeiro item da lista
  é usado.
- Sem nenhum diretório de trabalho, a sessão cai no projeto de rascunho padrão
  do servidor.
- Eventos de ferramenta do Cursor recebem o título da família da ferramenta e
  a saída, que vem de `tool_output` como texto JSON. Há teste para esse caso.

### A confirmar

- Os hooks do Claude Code rodam ao abrir o app, antes de qualquer conversa,
  com `conversation_id` e `session_id` iguais a um identificador de rascunho e
  sem `workspace_roots`. Observado ao vivo; o projeto não tem tratamento
  especial para esse identificador.
- Cada abertura da janela vazia acrescenta uma observação de início de sessão
  ao mesmo registro de rascunho. A confirmar, porque o projeto não trata esse
  caso à parte.
- O caminho do transcript do Cursor aparece só no evento de parada, dentro da
  pasta de projetos do próprio Cursor. Observado ao vivo; o projeto não lê esse
  caminho, e o campo de transcript aparece nulo nos testes.
- A janela vazia não carrega os servidores MCP do ai-memory, só os embutidos
  do Cursor.
- Eventos de ferramenta do Cursor chegaram sem corpo ao servidor em um teste
  de replay. O projeto tem teste para o caso de `tool_output`, então a causa
  está na versão ou no payload. Não isolada.
- A documentação de terceiros diz que os hooks do Claude Code são carregados
  por padrão no Cursor. Observado no início do app; a confirmar.

Ver também [Cursor](cursor.md) e [Captura e deduplicação](capture-and-dedup.md).

## Limites e pendências

| Pergunta | O que confirmaria |
| --- | --- |
| O app desktop do Codex envia algo no início da thread que desativa hooks? | Um prompt no app com hooks confiáveis instalados, e checar se a sessão chega ao servidor com a origem do app. |
| O IDE do Antigravity executa os hooks do arquivo global e do workspace, com o mesmo payload da CLI? | Um prompt no IDE em workspace de teste com um hook de registro. |
| Hooks e MCP chegam ao Cowork, em Linux, macOS e Windows? | Uma tarefa do Cowork após a instalação da máquina virtual do app, procurando a saída do hook na transcrição. |
| Hooks não confiáveis no Codex desktop são pulados sem aviso? | Um hook não confiável no app desktop, com verificação de aviso. |
| Por que eventos de ferramenta do Cursor chegam sem corpo em alguma versão? | Reproduzir o payload capturado no teste de replay, comparando os campos com o caso de `tool_output` que o projeto já cobre. |
| O envio de início de sessão por turno no Antigravity é aceitável, ou deve ser limitado por conversa? | Medir o efeito no servidor de envios repetidos, e então decidir. |
| O instalador do Claude Desktop recusa o Linux sem `--config-file`? | Rodar a instalação de MCP no Linux sem `--config-file` e conferir se o caminho padrão é usado. |
| A ponte de terceiros do Claude Desktop continua necessária, ou o app passou a aceitar HTTP? | Testar o app com um servidor HTTP direto na configuração. |
| O transcript do Antigravity se chama `transcript_full.jsonl` em todas as versões? | Uma sessão em cada versão, conferindo o nome do arquivo dentro de `.system_generated/logs/`. |
| Os payloads nativos do Cursor em `hooks.json` e o `cwd` em pasta aberta são os esperados? | Um prompt em workspace de teste com um hook de registro para o Cursor e outro para o Claude Code. |
