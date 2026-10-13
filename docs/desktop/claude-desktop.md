# Claude Desktop (pesquisa)

> Pesquisa sobre o aplicativo desktop de terceiros do fabricante do Claude. O documento separa duas coisas: o comportamento verificado do projeto para este cliente (instalação de MCP e tipo de agente `claude-desktop`) e as afirmações sobre o aplicativo. Afirmações sobre o aplicativo sem evidência no projeto estão marcadas como "a confirmar".

## Resumo

- Cliente somente MCP. O projeto não instala hooks de ciclo de vida para este cliente.
- O registro é feito com `install-mcp --client claude-desktop`. Como o aplicativo aceita só servidores stdio na configuração JSON, o projeto usa uma ponte stdio até o endpoint HTTP.
- Captura pela aba Code: a confirmar. Depende de o aplicativo usar o mesmo arquivo de configurações do Claude Code que recebe os hooks do projeto.
- Captura pela aba Chat: sem hooks. Depende de chamadas de ferramentas MCP feitas pelo modelo.

## 1. Identidade no projeto

- Tipo de agente: `claude-desktop`. O valor `claude_desktop` também é aceito na leitura.
- Não existe opção de instalação de hooks para este cliente. O comando de instalação de hooks não aceita esse agente e indica `install-mcp` para clientes somente MCP.
- Backup e restauração de agentes incluem o arquivo de configuração MCP deste cliente apenas em macOS e Windows. Em Linux, o arquivo não entra no backup.

## 2. Instalação do servidor MCP

### 2.1 Comando e opções

`ai-memory install-mcp --client claude-desktop` imprime o trecho de configuração. Com `--apply`, grava o trecho no arquivo de configuração do aplicativo. O cliente padrão do comando é outro, então `--client claude-desktop` precisa ser informado.

| Opção | Efeito neste cliente |
| --- | --- |
| `--server-url` | URL base ou endpoint completo do servidor. |
| `--name` | Nome da entrada em `mcpServers`. Padrão: `ai-memory`. |
| `--auth-token` | Token bearer. Se omitido, usa o token da configuração do projeto. |
| `--apply` | Grava no arquivo de configuração em vez de só imprimir. |
| `--config-file` | Substitui o caminho detectado automaticamente. |
| `--flavor` | Acrescenta à URL o marcador de dialeto de schema. Sem padrão para este cliente. |
| `--session-aware` | Não se aplica. Usar com este cliente gera erro. |

### 2.2 URL do servidor

- Precedência: `--server-url`, depois a URL configurada no projeto (arquivo de configuração ou variável de ambiente), depois o endereço local padrão `http://127.0.0.1:49374/mcp`.
- URL base: remove barras finais e acrescenta `/mcp` quando o sufixo falta.
- URL que já termina em `/mcp`: não é alterada.

### 2.3 Entrada gerada

A entrada fica em `mcpServers`, com o nome informado em `--name`.

- Sem token: `command` é o executável da ponte, e `args` é `[<argumentos da ponte>, <URL do endpoint>]`.
- Com token: `args` recebe `--header` e `Authorization:${AI_MEMORY_AUTH_HEADER}`, e `env` recebe `AI_MEMORY_AUTH_HEADER` com o valor `Bearer <token>`.
- Consequência de segurança: com token, o valor fica em texto puro no arquivo de configuração do aplicativo e também nas cópias de backup geradas por `--apply`.
- A ponte stdio é um pacote de terceiros mantido pela comunidade. Ela exige o runtime dessa ponte instalado na mesma máquina.

### 2.4 Saída sem `--apply`

- A saída começa com linhas de comentário iniciadas por `#` e só depois traz o JSON. Não é um arquivo JSON puro: copie apenas o objeto.
- Os comentários listam os caminhos do arquivo por sistema operacional e pedem para fechar e reabrir o aplicativo por completo. Atualizar o aplicativo não basta. O comportamento de reinício do aplicativo é a confirmar.

### 2.5 Gravação com `--apply`

- Arquivo inexistente: é criado, junto com os diretórios pais.
- Arquivo existente e conteúdo idêntico após a mesclagem: nada é gravado e não há backup. A saída informa que o registro já está atualizado.
- Arquivo existente com alteração: antes de gravar, copia o arquivo original para `<arquivo>.bak-<segundos Unix>` no mesmo diretório. Depois substitui o arquivo de forma atômica.
- Mesclagem: só a entrada de mesmo nome dentro de `mcpServers` é substituída. As outras entradas são preservadas.
- `mcpServers` presente mas que não é um objeto: a operação falha e nada é gravado.
- Caminho que é link simbólico: a escrita vai para o destino do link, e o próprio link não é substituído.

### 2.6 Caminho do arquivo de configuração

Ordem de decisão: `--config-file`, quando informado. Caso contrário, a regra da plataforma abaixo.

- macOS: `~/Library/Application Support/Claude/claude_desktop_config.json`.
- Windows, instalação sem pacote: `%APPDATA%\Claude\claude_desktop_config.json`. As variáveis `APPDATA` e `LOCALAPPDATA` são lidas quando definidas e não vazias.
- Windows, instalação empacotada (MSIX): o comando procura diretórios cujo nome começa com `Claude_` dentro de `%LOCALAPPDATA%\Packages`. O arquivo fica em `LocalCache\Roaming\Claude\claude_desktop_config.json` dentro do pacote.
  - Um único pacote com arquivo de configuração existente: usa esse arquivo.
  - Nenhum pacote com arquivo, e um único pacote encontrado: usa o caminho desse pacote, que será criado.
  - Nenhum pacote encontrado: usa o caminho sem pacote.
  - Qualquer outro caso ambíguo (mais de um pacote com arquivo, ou vários pacotes sem arquivo): erro pedindo `--config-file`. O comando não escolhe por conta própria.
- Linux (beta): `$XDG_CONFIG_HOME/Claude/claude_desktop_config.json`, com padrão `~/.config/Claude/`. Sistemas que não são macOS nem Windows usam essa mesma regra. O projeto trata a versão Linux como beta do fabricante. Distribuições oficialmente suportadas: a confirmar.

### 2.7 Desinstalação

`uninstall` remove do arquivo de configuração a entrada deste cliente cuja ponte aponta para a URL do servidor, mesmo que a entrada tenha outro nome. Verificado por teste de unidade.

## 3. Captura e hooks

- Aba Chat: sem hooks de ciclo de vida. A captura depende de chamadas de ferramentas MCP feitas pelo modelo. O comportamento do aplicativo é a confirmar.
- Aba Code: a confirmar. Do lado do projeto, o instalador de hooks do Claude Code registra eventos de ciclo de vida, incluindo início de sessão, no arquivo de configurações do Claude Code do usuário, com agente `claude-code`. Se a aba Code usa esse mesmo arquivo e dispara esses hooks, as sessões chegam ao projeto como `claude-code`. Essa ligação não foi verificada ao vivo.
- Aba Cowork: a confirmar. Não há verificação.

## 4. Afirmações sobre o aplicativo de terceiros (a confirmar)

Nenhuma das afirmações abaixo tem evidência no código do projeto, exceto a menção de que o projeto conhece as abas Code e Cowork e trata a versão Linux como beta.

- O aplicativo tem três áreas: Chat, Cowork e Code.
- Plataformas oficiais são macOS e Windows. Linux está em beta.
- A aba Code usa o mesmo motor do CLI. Ela compartilha configurações e memória de projeto por meio dos arquivos de instruções do Claude Code.
- A aba Chat aceita conectores remotos HTTPS configurados no próprio aplicativo.
- Extensões no formato `.mcpb` são o caminho indicado pelo fabricante para instalar servidores MCP locais. O projeto não gera esse formato.
- Plugins da aba Code podem agrupar servidores MCP e hooks.
- A aba Code aceita transportes stdio, SSE, HTTP, WebSocket e um tipo in-process reservado a aplicativos hospedeiros. O projeto usa stdio para este cliente.
- A configuração JSON do aplicativo aceita somente servidores stdio. O projeto assume essa limitação ao gerar a ponte.
- Sessões da aba Code têm registros locais que as vinculam a sessões do CLI, e uma sessão pode ser movida ou retomada entre os dois. Não há evidência no projeto. A identidade de sessão do projeto é o par agente e identificador de sessão do Claude Code. O vínculo com o aplicativo não foi conferido.
- Servidores MCP remotos por sessão aparecem nos registros locais da aba Code. A confirmar.
- O aplicativo é de código fechado. A confirmar.
- Caminhos de logs locais e de armazenamento de sessões do aplicativo. A confirmar.

## Evidência e fontes

- Comportamento do projeto (seções 1 a 3): verificado no código do projeto, incluindo o instalador de MCP, o instalador de hooks, o backup e restauração de agentes, a desinstalação e os testes de unidade correspondentes. Revisão de 2026-10-11.
- Afirmações do aplicativo (seção 4): sem verificação ao vivo registrada neste documento. Para o estado da verificação, veja [verificação de out/2026](verification-2026-10.md).
- Veja também: [captura e deduplicação sem hooks](capture-and-dedup.md), [panorama de apps desktop](desktop-landscape.md) e [extensões](extensions.md).
