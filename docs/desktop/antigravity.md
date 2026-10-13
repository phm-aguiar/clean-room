# Antigravity

Este documento descreve como o ai-memory se integra ao agente de linha de comando do Antigravity (`agy`). Ele registra só o comportamento do projeto: o que a instalação grava e o que os hooks fazem com cada evento. Afirmações sobre o produto de terceiros que o projeto não confirma aparecem marcadas como "a confirmar".

## Escopo

- Identificador do agente no projeto: `antigravity-cli`. Também são aceitos os nomes `antigravity` e `agy`.
- Coberto: hooks de ciclo de vida, instalação do servidor MCP e captura da saída de ferramentas.
- Não coberto: aplicativo desktop e IDE do Antigravity. Não há código do projeto para eles, então o comportamento deles é a confirmar.

## Resumo

| Recurso | Situação | Comando |
|---|---|---|
| Hooks de ciclo de vida | Suportado | `ai-memory install-hooks --agent antigravity-cli` (grava com `--apply`) |
| Servidor MCP | Suportado | `ai-memory install-mcp --client antigravity-cli` (grava com `--apply`) |
| Captura da saída de ferramentas | Suportado para as ferramentas listadas em "Captura de ferramentas" | Instalado com os hooks |
| Finalização da sessão | Comando manual | `ai-memory finalize-session --agent antigravity-cli` |
| Aplicativo desktop e IDE | A confirmar | Não há instalação específica |

## Instalação dos hooks

### Modos de execução

- Sem `--apply`, o comando imprime o trecho de configuração e os caminhos dos scripts. Não grava nada.
- Com `--apply`, o comando faz uma gravação atômica no arquivo de configuração.
- Se o arquivo não existir, ele é criado (resultado "new file").
- Se a gravação mudar o conteúdo, um backup do arquivo anterior é gravado ao lado dele com o nome `<arquivo>.bak-<segundos Unix>`, antes da alteração.
- Se o conteúdo gerado já estiver presente, o resultado é "already up to date". Nenhum backup é criado.

### Arquivo de configuração

- Caminho padrão: `~/.gemini/config/hooks.json`. A opção `--config-file` substitui o caminho.
- Os scripts de hook são copiados para um subdiretório do diretório de dados, reservado a este agente (o diretório de dados vem de `--data-dir`). A configuração aponta para essas cópias por caminho absoluto.
- Toda a configuração fica em um grupo nomeado `ai-memory`, no nível superior do arquivo.
- Eventos de ferramenta (`PreToolUse` e `PostToolUse`) usam formato aninhado, com `matcher` vazio, o que os aplica a todas as ferramentas.
- Eventos de ciclo de vida (`PreInvocation` e `Stop`) usam formato plano, com lista direta de handlers.
- Cada handler tem `type: command` e um comando com a URL do servidor em `AI_MEMORY_HOOK_URL`.
  - Se o token de autenticação for informado na instalação, ele é gravado no comando como `AI_MEMORY_AUTH_TOKEN`. Ou seja, fica em texto puro no arquivo de configuração.
  - Se houver estratégia de projeto, ela é gravada como `AI_MEMORY_PROJECT_STRATEGY`.
  - No Windows, o comando define as mesmas variáveis no ambiente e chama a variante de script correspondente.
- Os handlers não definem `timeout`. O tempo limite padrão do produto é a confirmar.

### Merge com configuração existente

- Outros grupos nomeados no arquivo são preservados.
- Dentro do grupo `ai-memory`, para cada evento, as entradas do ai-memory são removidas e substituídas pelas novas. Entradas de terceiros no mesmo evento são mantidas.
- A identificação de "entrada do ai-memory" é feita pelo conteúdo do comando.
- Uma nova execução com `--apply` sobre um arquivo já igual não altera nada (idempotente).

## Eventos e respostas

Cada hook lê o JSON do evento pela entrada padrão e o envia ao servidor do ai-memory com `POST` no endpoint `/hook`, com `event` e `agent=antigravity-cli` como parâmetros. Todos saem com código 0.

| Evento nativo | Evento enviado ao servidor | Quando | Resposta ao agente |
|---|---|---|---|
| `PreInvocation` | `session-start` | Antes de cada chamada ao modelo (pressuposto do código; a confirmar) | Ver "Início de sessão" |
| `PreToolUse` | `pre-tool-use` | Antes de cada ferramenta | `{"decision": "allow"}`. O hook nunca bloqueia a ferramenta. |
| `PostToolUse` | `post-tool-use` | Depois de cada ferramenta | `{}` |
| `Stop` | `stop` | Fim de um ciclo de execução | `{"decision":""}` |

### Início de sessão

- Só é processado quando o campo `invocationNum` é o inteiro `0`. Texto (como `"0"`), decimal, valor ausente ou malformado não contam. Nesses casos, o hook responde `{}` e não envia nada ao servidor.
- Se for o início, o hook envia o evento `session-start` com o parâmetro `session_id`, quando há identificador.
- Em seguida, busca o handoff pendente no endpoint `/handoff`, com `agent=antigravity-cli`, o `session_id`, os parâmetros do projeto e, se habilitado, o parâmetro de briefing.
- A busca do handoff tem limite de 1 segundo. Se não houver resposta ou se ela vier vazia, a resposta ao agente é `{}`.
- Se houver handoff, a resposta é um objeto com `injectSteps`, contendo uma mensagem efêmera cujo conteúdo é o texto do handoff, codificado como string JSON.
- O código trata cada `invocationNum` igual a `0` como início de sessão. Se o produto reiniciar esse contador a cada turno do usuário, cada turno enviará `session-start` e buscará o handoff. Esse reinício é a confirmar.
- O comando nativo de hook tem o mesmo filtro: descarta `session-start` quando `invocationNum` não é `0`. O instalador deste agente, porém, registra os scripts, não o comando nativo.

### Fim de sessão

- `Stop` encerra um ciclo de execução, não a conversa. O instalador informa isso ao usuário.
- Para fechar a sessão, o usuário executa `ai-memory finalize-session --agent antigravity-cli` após o último turno. O comando cria o resumo final e o handoff e enfileira a consolidação de fim de sessão, que é opt-in.

## Contrato de envio ao servidor

- Endpoint de eventos: `POST` em `/hook`, com corpo JSON igual ao do evento recebido.
- URL do servidor: valor de `AI_MEMORY_HOOK_URL`. Padrão: `http://127.0.0.1:49374`.
- Tempo limite do envio de evento: 0,2 segundo. Tempo limite da busca de handoff: 1,0 segundo.
- Token: se `AI_MEMORY_AUTH_TOKEN` estiver definido, é enviado como cabeçalho `Authorization: Bearer`. Se houver arquivo de cabeçalho de autenticação, ele é usado no lugar.
- Tratamento da resposta ao envio de evento:
  - Código 2xx: envio concluído. Dispara a drenagem da fila local.
  - Códigos 408, 425 e 429, falha de conexão, 5xx e outros códigos que não são 2xx nem 4xx: o evento é gravado em uma fila local em disco, para reenvio posterior. A fila fica em um diretório de estado com permissão 700 para o dono. Cada item é um arquivo com a URL e o corpo originais. O nome do arquivo combina tempo em milissegundos, identificador do processo e sequência.
  - Outros códigos 4xx: o evento é descartado.

### Identificação do projeto

- O diretório de trabalho (`cwd`) é lido do campo `cwd` do evento. Se ele estiver ausente ou vazio, usa-se o primeiro item de `workspacePaths`. Se nenhum dos dois existir, nenhum `cwd` é enviado. Não há fallback para o diretório do processo.
- A partir do `cwd`, o hook procura um arquivo `.ai-memory.toml` subindo pelos diretórios pais.
  - Se o `cwd` estiver dentro do diretório pessoal, a busca para no diretório pessoal.
  - Se estiver fora dele, a busca para na raiz do repositório Git mais próxima.
  - Se não houver repositório Git, só o próprio `cwd` é verificado.
- Os parâmetros lidos desse arquivo são enviados como query string.

### Identificador da sessão

- A ordem de busca no corpo do evento é: `session_id`, `sessionId`, `sessionID`, `session` e `conversationId`. O primeiro encontrado vale. O agente usa `conversationId`.
- O servidor também aceita esses nomes, além de caminhos aninhados usados por outros agentes.

## Campos do evento usados

| Campo | Uso |
|---|---|
| `conversationId` | Identificador da sessão |
| `workspacePaths` | Diretório de trabalho (primeiro item), quando `cwd` não existe |
| `invocationNum` | Inteiro. Só `0` marca o início da sessão |
| `toolCall.name` | Nome da ferramenta. A comparação de nomes não diferencia maiúsculas de minúsculas |
| `toolCall.args` | Argumentos da ferramenta. Obrigatório em `PreToolUse` para a ferramenta ser classificada com a forma de entrada conhecida |
| `stepIdx` | Índice do passo. Aceita inteiro ou texto numérico |
| `artifactDirectoryPath` | Diretório de artefatos do passo. Ignorado se vazio ou só com espaços |
| `transcriptPath` | Usado só para localizar o diretório de artefatos, se `artifactDirectoryPath` não estiver disponível |
| `error` | Texto. Se não vazio em `PostToolUse`, o desfecho é erro |
| `tool_response` | Saída da ferramenta. Se vier vazia ou só com espaços, o hook tenta preenchê-la com o arquivo de saída do passo |

O registro de ferramenta não inclui identificador de chamada para este agente.

## Captura de ferramentas

### Classificação por nome

| Nome em `toolCall.name` | Família | Saída de arquivo | Conteúdo capturado |
|---|---|---|---|
| `view_file` | Arquivo | Sim | Caminho em `AbsolutePath` ou `TargetFile` |
| `write_to_file`, `replace_file_content`, `multi_replace_file_content` | Arquivo | Não | Conteúdo em `CodeContent`, `ReplacementContent` ou `ReplacementChunks` |
| `list_dir`, `find_by_name`, `grep_search` | Busca e listagem | Sim | Saída do passo |
| `run_command` | Não arquivo | Sim | Comando em `CommandLine` e saída do passo |
| `search_web`, `manage_task`, `manage_subagents` | Não arquivo | Sim | Saída do passo |
| Qualquer outro nome (por exemplo `read_url_content`, `read_resource`, `call_mcp_tool`, `generate_image`, `ask_question`, `schedule`) | Desconhecida | Não | Só família e desfecho |

- Para ferramentas de família desconhecida, o registro contém apenas `tool_family: unknown` e o desfecho. Argumentos e saída não entram no registro.
- Para ferramentas de arquivo, o caminho em `AbsolutePath` ou `TargetFile` passa pela política de exclusão de captura.

### Desfecho

- `error` com texto não vazio: desfecho `error`.
- `error` ausente, nulo ou vazio: desfecho `unknown`. O projeto não registra sucesso para este agente.

### Ferramentas de edição

- A saída do passo nunca substitui o conteúdo de edição. Gravar a saída do passo apagaria o código capturado, porque ela costuma ser só uma confirmação.
- O conteúdo da edição vem de `CodeContent`, `ReplacementContent` ou `ReplacementChunks`, nessa ordem de busca.
- Se a edição falhou, o texto de `error` tem prioridade sobre o conteúdo tentado.

### Leitura do arquivo de saída do passo

O enriquecimento só ocorre quando todas as condições abaixo são verdadeiras:

- O evento é `PostToolUse`.
- `tool_response` está ausente, vazio ou só com espaços.
- O nome da ferramenta é elegível (lista acima, sem diferenciar maiúsculas de minúsculas). Se `toolCall.name` estiver ausente, usa-se o campo `tool` de nível superior.
- `stepIdx` é um inteiro ou texto numérico.
- Há um diretório de artefatos resolvível.

Regras de localização e leitura:

- O arquivo é `<diretório de artefatos>/.system_generated/steps/<stepIdx>/output.txt`.
- O diretório de artefatos é `artifactDirectoryPath`, se presente. Caminhos que começam com `~` não são expandidos aqui, porque o hook não recebe o diretório pessoal. Nesse caso, o hook tenta o `transcriptPath`.
- Se `artifactDirectoryPath` não estiver disponível, o diretório é derivado de `transcriptPath`. O arquivo de transcrição precisa estar em `<diretório>/.system_generated/logs/<arquivo>`. O diretório de artefatos é então `<diretório>`. Qualquer outro formato não resolve.
- Se o caminho for um link simbólico ou algo que não seja arquivo regular, a leitura é recusada.
- São lidos no máximo 2048 bytes, a partir do início do arquivo.
- Se a leitura cortar um caractere multibyte no fim, o caractere incompleto é descartado.
- Bytes finais inválidos em UTF-8 são descartados. Se o início do arquivo já for inválido, o resultado é vazio.
- Espaços no fim são removidos. Conteúdo só com espaços resulta em nada.
- Se a leitura resultar em texto, ele substitui `tool_response`.

## Servidor MCP

- Arquivo global: `~/.gemini/config/mcp_config.json`. A opção `--config-file` substitui o caminho.
- Não há suporte à configuração por workspace (por exemplo `.agents/mcp_config.json`). A confirmar.
- A entrada é gravada em `mcpServers`, com o nome `ai-memory` por padrão (a opção `--name` altera).
- Campos da entrada:
  - `serverUrl`: URL do servidor. Se não terminar em `/mcp`, esse caminho é acrescentado. O padrão do servidor é `http://127.0.0.1:49374/mcp`.
  - `timeout`: 5000 (milissegundos).
  - `headers.Authorization`: `Bearer <token>`, quando há token.
- O campo de URL é `serverUrl`, não `url` nem `httpUrl`.
- Sem `--apply`, o comando imprime o trecho para colar manualmente. Com `--apply`, mescla a entrada e preserva as demais entradas do arquivo.
- Métodos de autenticação do produto (Google ADC, OAuth, registro dinâmico de cliente e tokens em disco) não são configurados pelo instalador. A confirmar.
- Campos de permissão por ferramenta (como `disabled` ou `disabledTools`) não são gravados pelo instalador. A confirmar se o produto os suporta.

## Histórico e transcrição

- Os hooks não leem o arquivo de transcrição. Eles usam `transcriptPath` só para achar o diretório de artefatos, conforme a regra acima.
- A estrutura de diretórios `brain/<conversationId>/` aparece só como exemplo em teste do projeto, não como contrato de leitura. O nome de arquivo `transcript_full.jsonl` não aparece no código. Ambos são a confirmar.
- Para sessões iniciadas pelo próprio projeto (gerenciadas), o projeto importa apenas os prompts do histórico local do CLI. Passos do assistente e de ferramentas não são decodificados. O projeto registra essa limitação na importação.
- O código não distingue o aplicativo da CLI. Não há tratamento específico para um caminho de transcrição do aplicativo.

## Pontos a confirmar

- Se `PreInvocation` ocorre antes de cada chamada ao modelo.
- Se `invocationNum` volta a `0` a cada turno do usuário. Isso decide se cada turno dispara o início de sessão e a busca de handoff.
- Tempo limite padrão dos hooks no produto.
- Comportamento do aplicativo desktop e da IDE: se leem `~/.gemini/config/hooks.json` e se o caminho de transcrição deles segue o mesmo formato.
- Formato real de diretórios de transcrição e de artefatos, além da estrutura `brain/`.
- Suporte a configuração de MCP por workspace e a métodos de autenticação do produto.
- Suporte do produto a campos de permissão por ferramenta.
