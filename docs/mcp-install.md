# Registro do MCP em clientes adicionais

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

Esta página descreve como registrar o ai-memory como servidor MCP em clientes
além da instalação rápida do README. Para cada
cliente, o comando `ai-memory install-mcp` imprime a configuração ou, com
`--apply`, grava a entrada no arquivo de configuração do cliente.

Itens marcados como **depende do produto externo** (antes "a confirmar") descrevem
comportamento de produtos de terceiros que o código do instalador não define. Na auditoria 002,
o que o instalador define (caminhos, chaves, formatos e flags) foi conferido no código. Eles devem
ser conferidos com a documentação do cliente antes de depender deles.

## Antes de começar

**Endereço padrão.** O endereço padrão é `http://127.0.0.1:49374/mcp` (a porta
49374 equivale a `0xC0DE`). Para um servidor remoto (homelab, caixa na LAN), passe
`--server-url`. Ele aceita a URL base (`http://host:49374`) ou o endpoint completo
(`http://host:49374/mcp`). Se o sufixo `/mcp` faltar, ele é acrescentado. Barras
finais são removidas, e a normalização é idempotente.

**Token.** Passe `--auth-token` para embutir o token de autenticação. Se ele for
omitido, o comando usa o token de bearer da configuração do ai-memory, quando
existir. O token é gravado como `Authorization: Bearer <token>` no cabeçalho da
entrada, exceto nos clientes em que a seção correspondente indica outra forma.

**Transporte padrão sem estado.** O transporte HTTP responde a cada requisição de
forma independente, em JSON simples, sem cabeçalho `Mcp-Session-Id`. Por isso
clientes que apontam uma URL remota para `/mcp`, como OpenCode em modo remoto ou
`curl`, funcionam sem ponte stdio. Esse é o padrão desde a versão 0.1.2. Se um
cliente exigir continuidade de sessão ou fluxos SSE iniciados pelo servidor, suba
o servidor com `ai-memory serve --transport http --http-stateful`. Note que o
transporte padrão de `serve` é stdio, então o modo HTTP precisa ser pedido
explicitamente.

**Ponte stdio para o Claude Desktop.** O Claude Desktop aceita apenas servidores
stdio na configuração JSON. A entrada gerada usa uma ponte de transporte MCP (biblioteca MCP do ambiente do cliente),
que exige o runtime dessa ponte instalado na mesma máquina. Isso não tem relação com
estado de sessão.

**Captura é separada do registro MCP.** Registrar o MCP só dá ao modelo acesso às
ferramentas de memória. Captura automática de eventos de sessão exige os hooks,
instalados com `ai-memory install-hooks --agent <cliente> --apply`. O suporte por
agente está em [support-matrix.md](support-matrix.md) e os contratos de hooks em
[harness-adapters.md](harness-adapters.md).

| | O que você tem | O que não tem |
|---|---|---|
| **Somente MCP** | O modelo consulta a memória, aceita handoffs, roda consolidação e revisões de aprendizado (`memory_auto_improve`) | Resumo automático no fim da sessão; handoff automático nas fronteiras de sessão |
| **MCP + hooks** | Tudo acima, mais observações sanitizadas de prompts e ciclo de vida de ferramentas, capturadas automaticamente. O handoff aparece no início da sessão quando o cliente consome a saída do hook de início, ou um resultado equivalente de injeção de contexto | Observações de hooks não são transcrições nativas completas. Grok e Zero descartam a saída do início de sessão, e nesses casos é preciso pedir `memory_handoff_accept` ao retomar |

Clientes **somente MCP** (Claude Desktop, VS Code Copilot, Zed e Muse Code) não
enviam eventos de sessão ao endpoint de hooks. Para continuidade manual, peça ao
modelo que chame `memory_handoff_begin` antes de sair e `memory_handoff_accept`
ao retomar.

Para que clientes com suporte a MCP leiam as instruções de roteamento do projeto
de forma proativa, instale também o pacote de roteamento gerenciado, descrito em
[usage.md](usage.md#install-the-routing-snippet-and-agent-skills).

### Exclusões de captura

Exclusões de captura (`[capture] ignore_paths`) são separadas do registro MCP. Os
comandos nativos de hook e as integrações geradas para OpenCode, OMP, Pi e OpenClaw
aplicam essas exclusões. Os bundles de script legados (shell e PowerShell) e os
bundles só para remoto ou Docker não aplicam. Para ganhar a exclusão, reinstale o
hook ou o plugin existente. Ver [marker-file.md](marker-file.md#capture-exclusions).

## Ponte para hooks personalizados

O ai-memory não instala automaticamente sistemas de terceiros. Uma ponte própria
deve mapear os eventos de origem para os eventos canônicos do endpoint de hooks
(`session-start`, `post-tool-use`, `session-end` e demais) e encaminhar o objeto JSON
original. Quando o evento não tem equivalente canônico, use a extensão de metadados
em vez de ampliar o enum de eventos armazenado.

Exemplo de evento canônico:

```bash
curl -X POST \
  'http://127.0.0.1:49374/hook?event=user-prompt&agent=other' \
  -H 'content-type: application/json' \
  -d '{"session_id":"sess-123","cwd":"/repo","prompt":"Fix auth"}'
```

Exemplo com extensão:

```bash
curl -X POST \
  'http://127.0.0.1:49374/hook?event=lead.contact&agent=other&extension=fstech' \
  -H 'content-type: application/json' \
  -d '{"session_id":"sess-123","title":"Lead contacted","message":"Lead Maria requested a proposal"}'
```

Com `extension=<namespace>`, eventos desconhecidos são armazenados com o tipo
canônico `other`, e o evento de origem validado é preservado. Você pode passar
`source_event=<nome>` explicitamente. Se não passar, o valor de `event` vira o
evento de origem. Os dois tokens aceitam apenas letras ASCII, dígitos, `.`, `_`,
`-` e `:`. O namespace tem no máximo 64 bytes, e o nome do evento de origem, no
máximo 128 bytes. Eventos desconhecidos sem `extension` viram `other`, sem metadado
de origem.

### Hermes Agent

O ai-memory não tem instalador MCP para o Hermes Agent. O roteador de hooks
reconhece `agent=hermes` como tipo de sessão concreto e aceita o envelope de
ferramenta do Hermes (`tool_name` e `tool_input`) para metadados de família e para
a aplicação das exclusões de captura. O mapeamento recomendado é `on_session_start`
para `session-start`, `post_tool_call` para `post-tool-use` e `on_session_end` para
`session-end`.

`install-hooks --agent hermes` imprime o bloco `hooks:` pronto para colar em
`~/.hermes/config.yaml`, mas não grava esse arquivo. O Hermes ignora a saída do hook (**depende do produto externo**)
de início de sessão, então a injeção automática de handoff não funciona nele. Use
`memory_handoff_accept`.

Existe um plugin comunitário de terceiros para o Hermes (`ai-memory-hermes-plugin`).
Ele não é mantido pelo projeto. Antes de usá-lo, verifique a matriz de versões do
plugin e do ai-memory, o comportamento de instalação, atualização e remoção, a
cobertura de plataformas e o tratamento de segredos. Tokens e endereços do servidor
devem ficar em variáveis de ambiente ou em arquivo de configuração local, não em
arquivos gerados pelo plugin.

---

## Como o comando funciona

### Modo de impressão

Sem `--apply`, o comando imprime a configuração no terminal e não grava nada. A
saída começa com linhas de comentário (`#`) que explicam onde colar a entrada e
quais passos seguir (por exemplo, reiniciar o cliente). Em seguida vem o trecho em
JSON, TOML ou o comando de linha de comando equivalente, conforme o cliente.

### Modo de aplicação (`--apply`)

Com `--apply`, o comando lê o arquivo de configuração do cliente, altera apenas a
entrada do ai-memory e grava o resultado:

- **Nome da entrada.** A entrada é identificada pelo nome passado em `--name`
  (padrão `ai-memory`). Entradas com outros nomes não são tocadas. Se já existir
  uma entrada com o mesmo nome, ela é substituída.
- **Preservação.** Outros servidores MCP, outras chaves e outras configurações do
  arquivo são mantidos. Em arquivos TOML (Codex e Grok), comentários e outras
  tabelas são preservados. Em arquivos Zed, comentários e vírgulas finais são
  preservados. Em arquivos JSON estritos, o conteúdo é reescrito com indentação
  padrão e quebra de linha final.
- **Arquivo inexistente.** O arquivo e as pastas que faltam são criados. O resultado
  é reportado como `created`.
- **Arquivo JSON inválido ou não estritamente JSON.** Se o arquivo tiver comentários,
  vírgulas finais ou uma raiz que não seja objeto, o comando recusa a gravação e
  mostra uma mensagem pedindo para inspecionar o arquivo, renomeá-lo ou removê-lo.
  Nada é alterado. Isso vale para arquivos `.jsonc` com comentários, por exemplo
  `opencode.jsonc`: o comando não grava neles. Edite o arquivo à mão, ou aponte
  `--config-file` para um arquivo JSON estrito. O Zed é a exceção, porque o comando
  sabe ler e preservar o JSONC com comentários.
- **Sem mudança.** Se o conteúdo resultante for igual ao atual, nada é gravado, não
  há backup, e o resultado é `no-op` com a mensagem "already up to date".
- **Backup.** Quando o arquivo já existe e o conteúdo muda, antes da gravação o
  comando copia o arquivo original para `<arquivo>.bak-<timestamp>`, na mesma pasta,
  onde o timestamp é o instante em segundos desde a época Unix. Arquivos novos não
  geram backup.
- **Gravação atômica.** A escrita usa arquivo temporário na mesma pasta, seguida de
  renomeação, para que o arquivo nunca fique pela metade. Se o caminho for um link
  simbólico, o comando segue o link e grava no alvo, mantendo o link. A cadeia de
  links é limitada a 40 níveis.
- **Resultado.** O comando imprime `✓ created`, `✓ updated` ou `✓ no-op`, seguido do
  caminho do arquivo. O estado "updated" traz a indicação de que o backup foi escrito
  ao lado do arquivo.
- **Idempotência.** Rodar o mesmo comando de novo produz `no-op`.

Casos específicos:

- **Muse Code.** Se o arquivo não tiver `schema_version`, o comando acrescenta
  `"schema_version": 1`. Um valor já existente nunca é reescrito. O mesmo vale para a
  impressão, que já traz essa chave.
- **Pi.** O cliente não tem arquivo MCP nativo. `--apply` falha sem gravar nada e
  imprime a orientação de instalação da ponte (ver a seção do Pi).
- **Validações antes de gravar.** `--session-aware` só é aceito com
  `--client claude-code` (`--session-aware is supported only for --client claude-code`). Para o Kiro, a URL é validada antes de qualquer gravação
  (ver a seção do Kiro).

### Resolução de endereço e token

Ordem de resolução do endereço:

1. `--server-url`, quando informado, normalizado para terminar em `/mcp`.
2. Endereço configurado no ai-memory (variável `AI_MEMORY_SERVER_URL` ou a
   configuração), com `/mcp` acrescentado.
3. `http://127.0.0.1:49374/mcp`.

Ordem de resolução do token: `--auth-token`, depois o token de bearer da
configuração do ai-memory. Se nenhum existir, a entrada não tem cabeçalho
`Authorization`.

Para clientes OpenCode, com o nome padrão `ai-memory` e sem endereço ou token
configurados, o comando reaproveita o endereço e o token de uma entrada que ele
mesmo gerou antes, se ela existir no arquivo de configuração.

### Dialeto de schema (`--flavor`)

Alguns clientes repassam os schemas das ferramentas a um provedor de modelo que
aceita apenas um subconjunto do JSON Schema. Para esses casos, o endereço recebe a
marca `?flavor=<nome>`. Os valores válidos são `moonshot`, `bedrock` e `gemini`
(alias `vertex`). Os detalhes estão em
[Dialetos de schema para upstreams estritos](#dialetos-de-schema-para-upstreams-estritos).

- Kimi Code usa `moonshot` por padrão.
- Kiro usa `bedrock` por padrão.
- `--flavor` sobrepõe o padrão do cliente. Ele não empilha uma segunda marca.
- A marca é acrescentada de forma idempotente. Rodar de novo não duplica o parâmetro.

O comando de remoção reconhece todas as marcas em todos os clientes, então uma
entrada fixada manualmente com `--flavor` também é removida.

### Caminhos de configuração e variáveis de ambiente

Os caminhos padrão de cada cliente estão nas seções abaixo. Todos usam o diretório
pessoal do usuário. A variável `AI_MEMORY_HOME`, quando definida e não vazia,
substitui o diretório pessoal nesses caminhos padrão.

Variáveis que redirecionam a configuração de cada cliente. Valores vazios ou só com
espaços são tratados como não definidos:

| Cliente | Variável | Efeito no caminho padrão |
|---|---|---|
| Claude Code | `CLAUDE_CONFIG_DIR` | `$CLAUDE_CONFIG_DIR/.claude.json` |
| Codex | `CODEX_HOME` | `$CODEX_HOME/config.toml` |
| Grok Build CLI | `GROK_HOME` | `$GROK_HOME/config.toml` |
| Kimi Code | `KIMI_CODE_HOME` | `$KIMI_CODE_HOME/mcp.json` |
| Kiro CLI | `KIRO_HOME` | `$KIRO_HOME/settings/mcp.json` |
| Copilot CLI | `COPILOT_HOME` | `$COPILOT_HOME/mcp-config.json` |
| Oh My Pi | `OMP_PROFILE` / `PI_PROFILE`, `PI_CODING_AGENT_DIR`, `PI_CONFIG_DIR` | Ver a seção do OMP |
| Zed (Linux) | `XDG_CONFIG_HOME` | `$XDG_CONFIG_HOME/zed/settings.json` |
| Claude Desktop (Linux) | `XDG_CONFIG_HOME` | `$XDG_CONFIG_HOME/Claude/claude_desktop_config.json` |
| Claude Desktop (Windows) | `APPDATA`, `LOCALAPPDATA` | Ver a seção do Claude Desktop |

O comando não lê `XDG_CONFIG_HOME` para Zero e Muse Code. Esses dois usam
`~/.config/...` e, em uma instalação XDG não padrão, o caminho deve ser passado com
`--config-file`.

Para qualquer cliente, `--config-file` substitui o caminho padrão na impressão e
na aplicação. A exceção é o Pi, que não tem arquivo MCP. Para o Grok, o caminho
padrão aplicado é `$GROK_HOME/config.toml`, e um arquivo de projeto precisa ser
passado com `--config-file`.

---

## Remoção (`uninstall`)

```bash
ai-memory uninstall --only mcp            # dry-run: mostra o que seria removido
ai-memory uninstall --only mcp --apply --yes
```

Sem `--apply`, o comando só mostra o plano. Com `--apply`, ele grava. Sem `--yes`,
pede confirmação quando o terminal é interativo.

Opções relevantes:

- `--mcp-url`: endereço usado para casar as entradas. O padrão é
  `http://127.0.0.1:49374/mcp`, com `--server-url` como alias. Se as entradas foram
  criadas com um servidor remoto, passe a mesma URL aqui. Caso contrário, elas não
  são encontradas.
- `--mcp-name`: nome da entrada. Quando informado, só entradas com esse nome são
  removidas. Sem a opção, a entrada é casada apenas pelo endereço.

Regra de casamento. Uma entrada é removida quando:

- Seu nome bate com `--mcp-name`, se a opção foi informada; e
- Sua URL (nos campos `url`, `httpUrl` ou `serverUrl`) é igual ao endereço, ou a
  alguma variante com marca de dialeto; **ou**
- É uma ponte de transporte MCP ou uma ponte de sessão do Claude Code cujos argumentos
  incluem o endereço.

Só o nome não basta. Uma entrada chamada `ai-memory` que aponte para outro endereço
é preservada. A mesma regra vale para uma entrada com outro nome que aponte para o
servidor, quando `--mcp-name` é informado.

Alcance. O comando varre o caminho padrão de todos os clientes em uma única
execução. Ele também varre:

- Os caminhos de Claude Code no diretório redirecionado e no diretório pessoal.
- O caminho antigo do Codex em `~/.codex/config.toml`.
- Os diretórios de agente do OMP (ativo, `default`, `~/.omp/agent`).
- No caso do Swival, a raiz do projeto resolvida a partir do diretório atual no
  momento da remoção.

O comando não aceita `--config-file`. Arquivos em outros locais, como o arquivo de
projeto do Grok, o arquivo de projeto do Kiro ou o arquivo de perfil do VS Code,
precisam ser limpos à mão.

Ao gravar, cada arquivo alterado recebe backup `.bak-<timestamp>` ao lado, pelo
mesmo mecanismo da aplicação. Ao remover a última entrada de um mapa de servidores,
o mapa vazio também é removido. Para o Zed, `context_servers` vazio é removido. Os
contêineres pais que não são o próprio mapa de servidores são mantidos.

Por padrão, a remoção de hooks também apaga o token de bearer guardado no diretório
de dados. Com `--only mcp`, os hooks e o token são mantidos.

---

## Clientes

### Claude Code

- **Suporte MCP:** HTTP estático (padrão) ou ponte stdio com identificação de sessão
  (`--session-aware`).
- **Hooks:** `ai-memory install-hooks --agent claude-code --apply`.
- **Arquivo de configuração:** `~/.claude.json`, ou `$CLAUDE_CONFIG_DIR/.claude.json`
  quando a variável estiver definida. Não use o `settings.json`: o Claude Code não lê
  `mcpServers` nesse arquivo.

Registro padrão (HTTP estático):

```bash
ai-memory install-mcp --client claude-code --apply
```

A entrada gerada é:

```json
{
  "mcpServers": {
    "ai-memory": {
      "type": "http",
      "url": "http://127.0.0.1:49374/mcp",
      "headers": { "Authorization": "Bearer <token>" }
    }
  }
}
```

O cabeçalho `headers` só aparece quando há token. A impressão também traz a linha
equivalente `claude mcp add --transport http ai-memory <url>`, com o cabeçalho
quando houver token.

Ponte com identificação de sessão:

```bash
ai-memory install-mcp --client claude-code --session-aware --apply
```

A entrada gerada é uma ponte stdio:

```json
{
  "mcpServers": {
    "ai-memory": {
      "type": "stdio",
      "command": "ai-memory",
      "args": ["mcp-bridge", "--server-url", "http://127.0.0.1:49374/mcp"],
      "env": { "AI_MEMORY_AUTH_TOKEN": "<token>" }
    }
  }
}
```

O campo `env` só aparece quando há token. A ponte:

- Conecta ao endpoint `/mcp` configurado, local ou remoto, e preserva a autenticação
  por bearer.
- Acrescenta o cabeçalho `X-Memory-Actor-Session-Id` com o valor de
  `CLAUDE_CODE_SESSION_ID` em cada requisição ao servidor.
- Funciona tanto com o modo HTTP sem estado quanto com o modo com estado.
- Falha de forma explícita se o Claude não fornecer o id de sessão. Não há fallback
  silencioso para o slot compartilhado de sessão única.

Para que o isolamento funcione, o servidor precisa ter:

```toml
[auto_scope]
mode = "per_session"
```

Uma entrada HTTP estática não consegue carregar o id de sessão do hook. Por isso,
sem a ponte, `per_session` não isola duas sessões simultâneas do Claude Code.

Limites do cliente (**depende do produto externo**: documentação do Claude Code):

- O subprocesso stdio mantém o id com que foi iniciado depois de `/clear`. Reinicie o
  Claude Code após `/clear` quando precisar de continuidade exata da chave de sessão.
- `--continue` ou `--resume` sem id explícito pode expor o id de inicialização em vez
  do id retomado. Prefira `--resume <session-id>`.

O comando `ai-memory mcp-bridge` é interno (oculto). Ele não é um subcomando de uso direto.

**Dois modos da ponte** (escolhidos por quem a registra, nunca pelo ambiente):

- **Com sessão** (padrão do comando): repassa o id de sessão do agente que a lançou como
  `X-Memory-Actor-Session-Id` em toda requisição ao servidor e **recusa subir sem ele**. A opção
  `--agent` (`claude-code`, padrão, ou `muse`) escolhe a **única** variável lida:
  `CLAUDE_CODE_SESSION_ID` ou `MUSE_SESSION_ID`; a outra nunca é consultada, então um Claude Code
  aberto num terminal do Muse (ou o inverso) não repassa o id do agente de fora. Mensagem de recusa:
  `<VARIÁVEL> is missing; this session-aware bridge must be launched by <Claude Code|Muse Code> as an
  stdio MCP server (other clients register it with --no-session-id)`.
- **Sem sessão** (`--no-session-id`): retransmissão simples stdio-para-HTTP, sem identidade de
  sessão, para qualquer outro cliente (que passa `workspace` e `project` explicitamente, como num
  cliente estático). Ignora `CLAUDE_CODE_SESSION_ID` mesmo presente, porque um cliente aberto de um
  terminal do Claude Code herda esse id e repassá-lo desviaria as chamadas sem escopo para o ponteiro
  de projeto ativo de outra sessão. O instalador usa esse modo quando o token do servidor vive em
  arquivo (ver `users.md`, "Local root bearer token file").

Comportamento comum: opção `--server-url` (vazia conta como ausente e vale a URL configurada; a URL
ganha `/mcp` se faltar, mantendo uma query `?flavor=` depois do caminho); o bearer é o resolvido pela
configuração (`AI_MEMORY_AUTH_TOKEN`, `[auth].bearer_token` ou o arquivo de `[auth].bearer_token_file`),
enviado como cabeçalho `Authorization`; um valor de id de sessão com caracteres inválidos num cabeçalho
HTTP é erro. A ponte **conecta ao servidor antes de servir o stdio**: 401 ou servidor inalcançável
encerram o processo na hora, com `connecting the MCP bridge to <url>` mais `: the server answered 401
Unauthorized` quando for o caso, e, sem bearer, a dica de que ele é lido de `AI_MEMORY_AUTH_TOKEN`,
`[auth].bearer_token` ou `[auth].bearer_token_file`. O servidor remoto sem informações de servidor após a
inicialização também é erro. As ferramentas listadas e chamadas são repassadas ao servidor sem alteração,
e erros de transporte viram `upstream MCP request failed: <erro>`. Até a mensagem `initialize` chegar, a
ponte responde "método não encontrado" a qualquer outra requisição (`<método> is not available before
initialize`), ignora notificações e responde `ping`, para clientes que sondam primeiro com
`server/discover` e caem para `initialize`. URLs `https://` usam o verificador de certificados da
plataforma, com um provedor criptográfico instalado em processo.

Quando o registro é gravado: o comando da entrada é o **caminho absoluto** deste binário (clientes
costumam iniciar servidores MCP sem o `PATH` do shell de login); dentro de contêiner, o nome simples
`ai-memory`. A entrada de uma instalação anterior da ponte é reconhecida por comando `ai-memory` (nome ou
caminho absoluto de um binário chamado `ai-memory`) cujo primeiro argumento é `mcp-bridge`, com a URL
em `--server-url`.

### Codex

- **Suporte MCP:** HTTP em streaming. **Hooks:** `ai-memory install-hooks --agent codex --apply`.
- **Arquivo de configuração (TOML):** `$CODEX_HOME/config.toml` (valor em branco conta como ausente), senão `~/.codex/config.toml`.

```toml
[mcp_servers.ai-memory]
url = "http://127.0.0.1:49374/mcp"
default_tools_approval_mode = "approve"

[mcp_servers.ai-memory.http_headers]
Authorization = "Bearer <token>"
```

- `default_tools_approval_mode = "approve"` é sempre gravado: sem ele o Codex pede aprovação a **cada** chamada de ferramenta de memória, o que inviabiliza a captura automática.
- O token vai em `http_headers.Authorization` (a chave literal `bearer_token` é rejeitada pelo Codex para HTTP em streaming, e `headers` sem o prefixo `http_` é ignorada em silêncio). Quando o token vive em arquivo (ver `users.md`), a entrada traz `bearer_token_env_var = "AI_MEMORY_AUTH_TOKEN"` no lugar de `http_headers`, e o comando avisa `Codex reads the token from $AI_MEMORY_AUTH_TOKEN; export it where you launch codex.`
- Com `--apply` o arquivo é editado preservando comentários e as demais tabelas; `mcp_servers` em forma de tabela em linha é convertido para a forma em blocos (as outras entradas são preservadas), e só a entrada com o nome informado é substituída.

### OpenCode (V1) e OpenCode 2 (beta)

- **Arquivo:** `~/.config/opencode/opencode.json` (o mesmo para as duas versões). Arquivos `.jsonc` com comentários não são reescritos: passe um JSON estrito com `--config-file`.
- **V1** (`--client opencode`): servidores diretamente sob `mcp`:

```json
{ "mcp": { "ai-memory": { "type": "remote", "url": "http://127.0.0.1:49374/mcp", "enabled": true, "headers": { "Authorization": "Bearer <token>" } } } }
```

- **V2** (`--client opencode2`; aliases `opencode-v2`, `open-code2`): servidores sob `mcp.servers`, **sem** `enabled`, e com `"oauth": false` (sem ele o beta tenta descobrir OAuth no endpoint a cada conexão):

```json
{ "mcp": { "servers": { "ai-memory": { "type": "remote", "url": "http://127.0.0.1:49374/mcp", "oauth": false, "headers": { "Authorization": "Bearer <token>" } } } } }
```

- **Propriedade da entrada.** Uma entrada é "gerada" quando o tipo é `remote`, tem `url`, só usa as chaves `type`, `url`, `enabled`, `oauth`, `headers`, os cabeçalhos são ausentes ou exatamente `Authorization: Bearer <não vazio>`, e o formato do local bate (V1: `enabled == true` e sem `oauth`; V2: `oauth == false` e sem `enabled`). Entrada em modo ponte local (`type: "local"`, `command` em lista, `enabled` só no V1) também conta como gerada. Se a entrada **no local escolhido** existe e **não** é gerada, o comando recusa substituí-la: `refusing to replace user-owned OpenCode MCP entry '<nome>' in the selected major-version location`.
- **Migração.** Ao instalar numa versão, a entrada gerada da **outra** versão é removida (e o mapa `servers` vazio é apagado) somente se o endpoint dela for igual ao do comando e o formato gerado bater; se não for provadamente gerada, ela é preservada com o aviso `[ai-memory] warning: preserving ambiguous OpenCode MCP entry '<nome>' in the alternate major-version location`. Se houver entrada própria do usuário no local escolhido **e** uma gerada no alternativo, recusa com a mensagem sobre migrar a entrada gerada.
- **Inferência de endereço e token.** Sem `--server-url`/`--auth-token` e com o nome padrão `ai-memory`, o comando reaproveita endereço e token (do cabeçalho `Authorization: Bearer …`) de uma entrada gerada existente. Se as entradas gerada de V1 e V2 divergirem: `conflicting generated OpenCode MCP entries exist in the V1 and V2 locations; pass explicit --server-url and --auth-token before installing`. Se a entrada do local escolhido é do usuário e a alternativa é gerada: `a user-owned OpenCode MCP entry occupies the selected major-version location while a generated entry remains in the alternate location; …`. Entrada do usuário sozinha não é usada para inferir (aviso `preserving user-owned OpenCode MCP entry … pass explicit --server-url and --auth-token`).
- Quando o token vive em arquivo, a entrada vira ponte stdio local: `{"type": "local", "command": ["<ai-memory>", "mcp-bridge", "--no-session-id", "--server-url", "<url>"], "enabled": true}` (no V2 sem `enabled`).

### GitHub Copilot CLI

- **Hooks:** `ai-memory install-hooks --agent copilot-cli --apply`. (O nome `copilot` sozinho, em `install-mcp`, é o cliente do VS Code, não este.)
- **Arquivo:** `$COPILOT_HOME/mcp-config.json` (padrão `~/.copilot/mcp-config.json`). Arquivos de projeto `.mcp.json` ou `.github/mcp.json` entram com `--config-file`.

```json
{ "mcpServers": { "ai-memory": { "type": "http", "url": "http://127.0.0.1:49374/mcp", "headers": { "Authorization": "Bearer <token>" }, "tools": ["*"] } } }
```

`tools: ["*"]` (todas as ferramentas) é sempre gravado. Equivalente de linha de comando: `copilot mcp add --transport http <nome> <url>`. Com token em arquivo, a entrada é a ponte stdio sem sessão com `"type": "local"` e `"tools": ["*"]`.

### Factory Droid CLI

- **Hooks:** `ai-memory install-hooks --agent droid --apply`. **Arquivo:** `~/.factory/mcp.json` (sem variável que o realoque); um `.factory/mcp.json` de projeto entra com `--config-file`.

```json
{ "mcpServers": { "ai-memory": { "type": "http", "url": "http://127.0.0.1:49374/mcp", "headers": { "Authorization": "Bearer ${AI_MEMORY_AUTH_TOKEN}" } } } }
```

O Droid expande `${VAR}` só em `headers`, nunca em `url`; por isso a URL é literal e o bearer é **sempre referenciado por nome** (o token nunca é gravado). O cabeçalho só aparece quando há `--auth-token` (ou token em arquivo). O comando avisa `Droid expands ${AI_MEMORY_AUTH_TOKEN} in the Authorization header; export it where you launch droid, or the server fails to connect.` Equivalente: `droid mcp add <nome> <url> --type http`.

### IBM Bob (IDE e Bob Shell)

- **Hooks:** `ai-memory install-hooks --agent ibm-bob --apply`. **Arquivo:** `~/.bob/settings/mcp.json` (um `.bob/mcp.json` de projeto entra com `--config-file`).

```json
{ "mcpServers": { "ai-memory": { "type": "streamable-http", "url": "http://127.0.0.1:49374/mcp", "protocolNegotiation": "legacy", "headers": { "Authorization": "Bearer <token>" } } } }
```

`protocolNegotiation: "legacy"` é obrigatório na prática: no modo automático o Bob sonda um `server/discover` não padrão antes do `initialize` padrão do MCP, que o ai-memory implementa.

### Prime Agent

- **Arquivo:** `$PRIME_AGENT_CODING_AGENT_DIR/settings.json` (padrão `~/.prime/agent/settings.json`); o `.prime/agent/settings.json` de projeto entra com `--config-file`. **Sem hooks.**

```json
{ "mcpServers": { "ai-memory": { "type": "http", "url": "http://127.0.0.1:49374/mcp", "bearerTokenEnvVar": "AI_MEMORY_AUTH_TOKEN" } } }
```

O Prime Agent 0.10 só conta um servidor HTTP do usuário como conectado (e só o cita ao modelo) com um grant OAuth ou com `bearerTokenEnvVar` cuja variável esteja definida; um `headers.Authorization` literal ainda despacha chamadas, mas fica sem listar. Por isso o token é referenciado por variável (nunca gravado) e a chave é a camelCase (a forma snake_case que o `mcp add` do próprio agente grava é ignorada pelo carregador). `bearerTokenEnvVar` só aparece quando há `--auth-token` (ou token em arquivo); sem token, o comando explica que o agente lista o servidor como "não conectado". O modelo chama as ferramentas pelo REPL Python: `await mcp.call_tool("<nome>", "memory_query", {...})`.

### DeepSeek Harness (DSH)

- **Somente impressão** (`--apply` é recusado: `DeepSeek Harness has no fixed MCP config file; refusing to write MCP config. Re-run without --apply to print the Cordis patch fragment, then merge it into $DSH_HOME/profiles/<profile>/cordis.patch.yml`, com a nota "the printed entry carries the auth header" quando há token). O nome da entrada precisa casar `[A-Za-z0-9_-]{1,32}`.
- O cliente é uma entrada de plugin Cordis no `cordis.patch.yml` de um perfil. Fragmento (todo escalar interpolado entre aspas simples):

```yaml
- insert:
    - id: 'ai-memory'
      name: '@deepseek-ai/dsh-mcp-client'
      config:
        serverName: 'ai-memory'
        transport: streamable-http
        url: 'http://127.0.0.1:49374/mcp'
        headers:
          Authorization: 'Bearer <token>'
        toolCallTimeoutMs: 120000
```

Com token em arquivo, `transport: stdio`, `command` e `args` da ponte sem sessão no lugar de `url`/`headers`. Sem captura de ciclo de vida.

### Cursor

- **Suporte MCP:** sim. **Hooks:** `ai-memory install-hooks --agent cursor --apply`
  ([harness-adapters.md](harness-adapters.md)).
- **Arquivo de configuração:** `~/.cursor/mcp.json` (global, padrão do `--apply`).
  Para o escopo de projeto, `.cursor/mcp.json` na raiz do workspace, passado com
  `--config-file`.

```json
{
  "mcpServers": {
    "ai-memory": {
      "url": "http://127.0.0.1:49374/mcp"
    }
  }
}
```

Com token, a entrada ganha `"headers": { "Authorization": "Bearer <token>" }`.

- O HTTP usa a chave `url`. Entradas stdio usam `command` e `args`.
- Depois de editar, reinicie o Cursor ou desligue e ligue o servidor em
  **Settings → MCP**. O recarregamento ao vivo funciona em builds recentes, mas
  ainda é instável, segundo o texto impresso pelo comando.

### VS Code GitHub Copilot

- **Suporte MCP:** sim, no modo agente. **Hooks:** não. O VS Code não expõe eventos
  de ciclo de vida (`PreToolUse`, `PostToolUse`, `SessionStart`) para o Copilot. A
  captura automática fica inativa. Use `memory_query`, `memory_write_page` e as demais
  ferramentas pelo chat quando precisar.
- **Arquivo padrão:** `.vscode/mcp.json` no diretório de trabalho atual, quando o
  comando é executado. Rode-o a partir da raiz do workspace. Esse é o padrão que
  combina com o escopo automático por diretório do ai-memory.
- **Perfil de usuário:** execute **MCP: Open User Configuration** no VS Code e passe o
  caminho do arquivo aberto com `--config-file`. O caminho varia por plataforma e
  perfil.

A chave de nível superior é **`servers`**, não `mcpServers`. O VS Code ignora
silenciosamente `mcpServers`.

```json
{
  "servers": {
    "ai-memory": {
      "type": "http",
      "url": "http://127.0.0.1:49374/mcp",
      "headers": { "Authorization": "Bearer <token>" }
    }
  }
}
```

Os cabeçalhos só aparecem com token. Aliases do cliente: `copilot` e
`github-copilot`.

```bash
ai-memory install-mcp --client vscode-copilot                   # imprime
ai-memory install-mcp --client vscode-copilot --apply           # .vscode/mcp.json do diretório atual
ai-memory install-mcp --client vscode-copilot --apply \
  --config-file /caminho/do/perfil/mcp.json
```

### Zed

- **Suporte MCP:** sim, pelo mapa `context_servers` das configurações do Zed.
  **Hooks:** não. A captura automática e a continuidade de workstream gerenciada ficam
  inativas.
- **Arquivo de configuração:**
  - macOS: `~/.config/zed/settings.json`
  - Linux: `$XDG_CONFIG_HOME/zed/settings.json`, padrão `~/.config/zed/settings.json`
  - Windows: `%APPDATA%\Zed\settings.json`

```json
{
  "context_servers": {
    "ai-memory": {
      "url": "http://127.0.0.1:49374/mcp",
      "headers": { "Authorization": "Bearer <token>" }
    }
  }
}
```

A entrada do Zed tem somente `url` e, quando há token, `headers`. Não há chave `type`.

```bash
ai-memory install-mcp --client zed
ai-memory install-mcp --client zed --apply \
  --server-url "http://homelab:49374/mcp" \
  --auth-token "$TOKEN"
```

Com `--apply`, o arquivo é lido como JSON com comentários. Comentários, vírgulas
finais, outras configurações do Zed e outros servidores de contexto são preservados.
Se `context_servers` existir mas não for um objeto, ou se a raiz não for um objeto, o
comando recusa a gravação e não altera o arquivo.

### Muse Code

- **Suporte MCP:** sim, **pela ponte stdio com sessão** (`mcp-bridge --agent muse`), que repassa o
  `MUSE_SESSION_ID` que o Muse entrega aos seus servidores stdio como id de sessão do ator; uma entrada
  HTTP não carregaria id de sessão algum. **Hooks:** não há instalador. Sem adaptador de workstream
  gerenciada.
- **Arquivo de configuração:** `~/.config/muse/settings.json`. Para um diretório XDG
  não padrão, passe o caminho com `--config-file`.

O mapa de servidores é `mcpServers` (padrão). Se o arquivo **já** tiver o mapa legado `mcp_servers`, a entrada
vai nele, com o campo `type` renomeado para `transport`. Se o arquivo tiver **os dois** mapas, o comando
**recusa**: `this Muse settings file has both `mcpServers` and `mcp_servers`, and Muse ignores every MCP
server when both are present. Move the servers into one of the two maps and re-run this command.`

```json
{
  "schema_version": 1,
  "mcpServers": {
    "ai-memory": {
      "type": "stdio",
      "command": "/caminho/para/ai-memory",
      "args": ["mcp-bridge", "--agent", "muse", "--server-url", "http://127.0.0.1:49374/mcp"],
      "env": { "AI_MEMORY_AUTH_TOKEN": "<token>" },
      "mode": "optional"
    }
  }
}
```

O Muse inicia servidores stdio com o ambiente **limpo**. Por isso, quando a instalação usa um diretório de
dados próprio, a entrada traz `--data-dir <dir>` e a ponte lê o bearer da configuração dessa instalação
(`[auth].bearer_token_file`) ao subir; um token passado explicitamente vai em `env.AI_MEMORY_AUTH_TOKEN`
(o campo `env` só aparece então).

```bash
ai-memory install-mcp --client muse
ai-memory install-mcp --client muse --apply \
  --server-url "http://homelab:49374/mcp" \
  --auth-token "$TOKEN"
```

Dois detalhes do schema:

- **`"schema_version": 1` é obrigatório.** Sem essa chave, todo comando do `muse`
  falha na inicialização com `malformed settings file`. O `--apply` acrescenta a chave
  quando ela falta e nunca reescreve um valor existente, para que um schema futuro não
  seja rebaixado em silêncio.
- **`mode` é `optional`.** O padrão do Muse é `required`, e um servidor obrigatório
  que não sobe aborta a execução inteira. Com `optional`, um servidor de memória
  inalcançável custa recall, não a sessão. Por isso o comando grava o modo
  explicitamente.

Skills. `ai-memory install-skills` grava as skills do ai-memory em `~/.agents/skills`,
que é a raiz de skills entre clientes. Que o Muse também leia essa raiz e
`$XDG_CONFIG_HOME/muse/skills` **depende do produto externo**. Confira com `muse skills list`.

Sobre captura: o Muse tem uma superfície de hooks, mas o formato de saída do evento
de início não está especificado. Por isso a injeção automática de handoff não é
garantida. Use `memory_handoff_begin` e `memory_handoff_accept` manualmente.

### Claude Desktop

- **Suporte MCP:** sim, pela ponte de transporte MCP (ver [Antes de começar](#antes-de-começar)).
  **Hooks:** não há. Para a pesquisa sobre o produto, veja
  [desktop/claude-desktop.md](desktop/claude-desktop.md).
- **Arquivo de configuração:**
  - macOS: `~/Library/Application Support/Claude/claude_desktop_config.json`
  - Windows, instalação comum: `%APPDATA%\Claude\claude_desktop_config.json`
  - Windows, instalação MSIX (pacote da Microsoft Store):
    `%LOCALAPPDATA%\Packages\Claude_<id>\LocalCache\Roaming\Claude\claude_desktop_config.json`
  - Linux (beta, segundo o texto impresso pelo comando):
    `$XDG_CONFIG_HOME/Claude/claude_desktop_config.json`, padrão
    `~/.config/Claude/claude_desktop_config.json`

No Windows, o comando detecta pacotes MSIX em `%LOCALAPPDATA%\Packages`, cujo nome
começa com `Claude_` (sem diferenciar maiúsculas). A regra de escolha é:

1. Se **exatamente um** pacote já tiver o arquivo de configuração, usa esse.
2. Se nenhum tiver o arquivo e houver um único pacote candidato, usa o caminho dele (as pastas
   `LocalCache\…` que faltam são criadas pela gravação atômica).
3. Qualquer outro caso (vários candidatos sem arquivo, **ou dois ou mais com arquivo**): o
   comando para com `multiple Claude Desktop packages were found under <pasta> (<nomes>); pass --config-file with the active package's LocalCache\Roaming\Claude\claude_desktop_config.json path`
   (corrigido pela auditoria 002).
4. Se não houver pacote (ou a pasta `Packages` não existir), usa o caminho `%APPDATA%` da
   instalação comum. Erro de leitura da pasta `Packages` interrompe o comando.

`LOCALAPPDATA` e `APPDATA` são lidos primeiro das variáveis de ambiente (valor vazio é ignorado)
e só depois da pasta conhecida do sistema, para respeitar um perfil realocado.

Entrada gerada:

```json
{
  "mcpServers": {
    "ai-memory": {
      "command": "<executavel-da-ponte>",
      "args": ["<argumentos-da-ponte>", "http://127.0.0.1:49374/mcp"]
    }
  }
}
```

Com token, a entrada passa o cabeçalho pela variável de ambiente, para que o valor com
espaço não precise ser separado pelo Windows:

```json
{
  "mcpServers": {
    "ai-memory": {
      "command": "<executavel-da-ponte>",
      "args": [
        "<argumentos-da-ponte>", "http://127.0.0.1:49374/mcp",
        "--header", "Authorization:${AI_MEMORY_AUTH_HEADER}"
      ],
      "env": { "AI_MEMORY_AUTH_HEADER": "Bearer <token>" }
    }
  }
}
```

```bash
ai-memory install-mcp --client claude-desktop --apply
```

Depois de editar a configuração, feche o Claude Desktop por completo e abra de novo.
A opção "Check for Updates…" não basta.

Gotchas:

- Logs do MCP ficam em `~/Library/Logs/Claude/mcp*.log` no macOS, e em
  `%APPDATA%\Claude\logs\` (ou no equivalente dentro do pacote MSIX) no Windows. **Depende do produto
  externo**: documentação do Claude Desktop.
- O Claude Desktop também oferece conectores remotos e extensões `.mcpb` na conta.
  O CLI gerencia somente o arquivo JSON local, que funciona com servidores em
  localhost e LAN sem exigir um conector HTTPS publicado. **Depende do produto externo**: confira com a
  documentação do produto.
- Pacotes MSIX rodam em contêiner de aplicativo, e o Windows redireciona as gravações
  de `%APPDATA%` para uma árvore isolada em `LocalCache`. Por isso o `--apply` precisa
  do caminho do pacote, e não do caminho comum.

### Gemini CLI

- **Suporte MCP:** sim. **Hooks:** `ai-memory install-hooks --agent gemini-cli --apply`.
- **Arquivo de configuração:** `~/.gemini/settings.json`. Para um escopo de projeto,
  `.gemini/settings.json` com `--config-file`, porque o comando não grava esse arquivo
  por padrão.

```json
{
  "mcpServers": {
    "ai-memory": {
      "httpUrl": "http://127.0.0.1:49374/mcp",
      "timeout": 5000
    }
  }
}
```

- O transporte HTTP usa `httpUrl`, e não `url`. O valor de `timeout` é em milissegundos
  (5000 ms). Com token, a entrada ganha `headers`.
- Não misture `httpUrl` com `command`/`args` ou `url` na mesma entrada.
- Reinicie a sessão do CLI depois de alterar `~/.gemini/settings.json`.

### Antigravity CLI (`agy`)

- **Suporte MCP:** sim. **Hooks:** `ai-memory install-hooks --agent antigravity-cli
  --apply`, com arquivo próprio em `~/.gemini/config/hooks.json`. Os hooks estão em
  [desktop/antigravity.md](desktop/antigravity.md).
- **Arquivo de configuração (MCP):** `~/.gemini/config/mcp_config.json`. Aliases do
  cliente: `antigravity` e `agy`.

```json
{
  "mcpServers": {
    "ai-memory": {
      "serverUrl": "http://127.0.0.1:49374/mcp",
      "timeout": 5000
    }
  }
}
```

- O transporte HTTP usa `serverUrl`, e não `url` nem `httpUrl`. O `--apply` grava a
  chave certa. O `timeout` é em milissegundos. Com token, a entrada ganha `headers`.
- MCP e hooks ficam em arquivos separados. Não coloque hooks em `mcp_config.json`.
- O evento `PreInvocation` dispara antes de cada chamada ao modelo. O ai-memory o usa
  como equivalente mais próximo do início de sessão.
- O cliente não tem evento real de fim de sessão. Depois da última volta, rode
  `ai-memory finalize-session --agent antigravity-cli` para fechar a sessão e gerar o
  resumo e o handoff automático. O comando `finalize-session` também aceita `--all`,
  que fecha todas as sessões abertas correspondentes no escopo selecionado.

### Zero (Gitlawb/zero)

- **Suporte MCP:** sim, por HTTP nativo com cabeçalhos bearer. **Hooks:**
  `ai-memory install-hooks --agent zero --apply`.
- **Arquivo de configuração:** `~/.config/zero/config.json`. O comando não lê
  `XDG_CONFIG_HOME` para esse cliente. Em um setup XDG não padrão, use `--config-file`.

```bash
ai-memory install-mcp --client zero --apply \
    --server-url "http://homelab:49374/mcp" --auth-token "$TOKEN"
```

Mescla esta entrada em `mcp.servers`:

```json
{
  "mcp": {
    "servers": {
      "ai-memory": {
        "type": "http",
        "url": "http://homelab:49374/mcp",
        "headers": { "Authorization": "Bearer <token>" }
      }
    }
  }
}
```

A captura é independente do registro MCP. O Zero descarta a saída do hook de
`sessionStart`. Por isso a captura e a criação de handoff no fim de sessão funcionam,
mas a injeção do handoff no início não. Peça ao Zero que chame `memory_handoff_accept`
no início de uma sessão retomada.

### ZCode (z.ai)

- **Suporte MCP:** sim. **Hooks:** `ai-memory install-hooks --agent zcode --apply`.
- **Arquivo de configuração:** `~/.zcode/cli/config.json`, no escopo de usuário.
  Os escopos de workspace (`.zcode/config.json`, `zcode.json`, `.agents/mcp.json`)
  existem, mas o instalador de hooks nunca os usa, porque são protegidos por
  confiança e desligados por padrão. Para MCP, passe um deles com `--config-file`.

```bash
ai-memory install-mcp --client zcode --apply \
    --server-url "http://homelab:49374/mcp" --auth-token "$TOKEN"
```

Mescla esta entrada em `mcp.servers`:

```json
{
  "mcp": {
    "servers": {
      "ai-memory": {
        "type": "http",
        "url": "http://homelab:49374/mcp",
        "headers": { "Authorization": "Bearer <token>" }
      }
    }
  }
}
```

O schema de entrada do ZCode é estrito. Qualquer chave fora de `type`, `url`,
`headers`, `enabled` e `timeoutMs` faz o ZCode descartar o servidor sem aviso. Por
isso a entrada gerada tem só `type`, `url` e `headers`. O endpoint `/mcp` padrão, sem
estado, não precisa de marca de dialeto. A autenticação vai em `headers`.

Os hooks gravados por `install-hooks --agent zcode` ficam no mesmo arquivo, dentro do
bloco raiz `hooks`. O comando acrescenta `"enabled": true` ao bloco se a chave faltar.
Também eleva o limite de saída padrão dos hooks, de 32 KiB, para que um handoff não
seja truncado no meio do JSON, desde que o usuário não tenha escolhido outro valor.

### Swival CLI

- **Suporte MCP:** sim. **Hooks e workstreams gerenciadas:** não suportados. O
  Swival chama um único callback compartilhado de início e fim, sem um identificador
  estável de sessão. Assim, sessões simultâneas não podem ser correlacionadas com
  segurança (**depende do produto externo**: documentação do Swival).
- **Arquivo de configuração:** `.swival/mcp.json` na raiz do projeto. A raiz é o
  diretório ancestral mais próximo que contenha `.git` ou `swival.toml`. Se nenhum
  existir, usa o diretório atual. Esse é o padrão de busca documentado do Swival.
  Para outro arquivo MCP em JSON, passe `--config-file`.

```bash
ai-memory install-mcp --client swival --apply \
  --server-url "http://homelab:49374/mcp" \
  --auth-token "$TOKEN"
```

Mescla esta entrada em `mcpServers`:

```json
{
  "mcpServers": {
    "ai-memory": {
      "type": "http",
      "url": "http://homelab:49374/mcp",
      "headers": { "Authorization": "Bearer <token>" }
    }
  }
}
```

Para remover, use `ai-memory uninstall --only mcp --apply --yes` a partir do mesmo
diretório. A remoção resolve a mesma raiz de projeto e apaga somente a entrada do
ai-memory, preservando os demais servidores.

### Grok Build CLI

- **Suporte MCP:** sim. **Hooks:** `ai-memory install-hooks --agent grok --apply`.
  Os hooks ficam em um arquivo próprio, `$GROK_HOME/hooks/ai-memory.json`, detalhado em
  [desktop/grok-desktop.md](desktop/grok-desktop.md). O arquivo é separado de outros
  arquivos de hook do mesmo diretório, que o comando não altera (**depende do produto externo**: que o
  Grok carrega todos os `*.json` desse diretório).
- **Limite de handoff:** o Grok ignora a saída do `SessionStart`. Por isso o handoff
  não é injetado automaticamente. Peça ao Grok que chame `memory_handoff_accept` ao
  retomar.
- **Arquivo de configuração:** `$GROK_HOME/config.toml`, padrão `~/.grok/config.toml`.
  Para um arquivo de projeto ou outro caminho, passe o caminho exato com
  `--config-file`. O comando não deduz um arquivo de projeto. Informe a URL e o token
  explicitamente nesse caso. A remoção só alcança o caminho padrão, então um arquivo
  customizado precisa ser limpo à mão.

```bash
ai-memory install-mcp --client grok --apply \
    --server-url "http://homelab:49374/mcp" --auth-token "$TOKEN"
```

Mescla esta entrada em `mcp_servers`:

```toml
[mcp_servers.ai-memory]
url = "http://homelab:49374/mcp"
enabled = true

[mcp_servers.ai-memory.headers]
Authorization = "Bearer <token>"
```

- O Grok usa a tabela `headers`. A chave `http_headers` é de outro cliente e o Grok a
  ignora em silêncio.
- `enabled = true` é o interruptor documentado por servidor.
- A alternativa pela linha de comando é `grok mcp add --transport http ai-memory <url>`,
  com `--header` para o bearer.
- Expansão de `${VAR}` nos campos de texto para não gravar o token em claro (por
  exemplo, `Authorization = "Bearer ${AI_MEMORY_AUTH_TOKEN}"`) **depende do produto externo**: confira com a
  documentação do Grok.
- O `--apply` preserva comentários e outras tabelas. Um `mcp_servers` em forma inline
  é convertido para a forma de tabelas em bloco.

### Devin CLI

- **Suporte MCP:** sim. **Hooks:** `ai-memory install-hooks --agent devin --apply`.
  Por padrão, os hooks ficam em `~/.devin/hooks.v1.json`, cujo objeto raiz é o mapa de
  eventos. Os eventos suportados estão descritos em
  [harness-adapters.md](harness-adapters.md).
- **Arquivo de configuração:** `~/.devin/config.json`.

```bash
ai-memory install-mcp --client devin --apply \
    --server-url "http://homelab:49374/mcp" --auth-token "$TOKEN"
```

Mescla esta entrada em `mcpServers`:

```json
{
  "mcpServers": {
    "ai-memory": {
      "url": "http://homelab:49374/mcp",
      "transport": "http",
      "headers": { "Authorization": "Bearer <token>" }
    }
  }
}
```

Para gravar os hooks dentro de `~/.devin/config.json`, passe esse caminho com
`--config-file`. O ai-memory os mescla sob a chave `hooks` (confirmado na auditoria 002):
qualquer arquivo cujo nome não seja `hooks.v1.json` recebe os eventos dentro de `hooks`
(criada se faltar; se `hooks` existir e não for objeto, a instalação falha com
``hooks` is present in config.json but not an object`); em `hooks.v1.json` o arquivo inteiro é
o objeto de hooks. Em ambos, a escrita é atômica e sobrepõe só os eventos do ai-memory.

Detalhes de identidade dos hooks:

- **Diretório (cwd).** Se o payload trouxer `cwd`, ele é usado. Senão, a variável
  `DEVIN_PROJECT_DIR`, quando o launcher do Devin a fornece. Por último, o diretório de
  trabalho do processo do hook.
- **Id de sessão.** Quando o payload não traz `session_id`, o hook gera um id no
  início e o guarda em `<dados>/hook-state/devin-session-id`. Ele é reutilizado nos
  eventos seguintes. Para fixar um id externo, defina `AI_MEMORY_SESSION_ID` no
  ambiente do hook (ver [hook-scripts-and-container-deploy.md](hook-scripts-and-container-deploy.md)).
- **Sessões simultâneas.** Como o slot é único por host e agente, duas sessões do
  Devin rodando na mesma máquina compartilham o id. O início mais recente prevalece,
  e os eventos restantes das sessões anteriores são atribuídos a ele. Um payload que
  traz `session_id` próprio sempre tem precedência.

### Kimi Code

- **Suporte MCP:** sim. **Hooks:** `ai-memory install-hooks --agent kimi-code --apply`.
- **Arquivo de configuração (MCP):** `$KIMI_CODE_HOME/mcp.json`, padrão
  `~/.kimi-code/mcp.json`.

```bash
ai-memory install-mcp --client kimi-code --apply \
    --server-url "http://homelab:49374/mcp" --auth-token "$TOKEN"
```

Mescla esta entrada em `mcpServers`:

```json
{
  "mcpServers": {
    "ai-memory": {
      "url": "http://homelab:49374/mcp?flavor=moonshot",
      "headers": { "Authorization": "Bearer <token>" }
    }
  }
}
```

- Uma entrada com `url` e sem `transport` é HTTP em streaming. Não adicione
  `transport`. Ele só é usado por endpoints SSE legados.
- O comando acrescenta `?flavor=moonshot` sozinho, de forma idempotente. A API da
  Moonshot rejeita combinadores `anyOf`, `oneOf` e `allOf` na raiz dos schemas das
  ferramentas, e rejeita referências `$ref` em qualquer profundidade. Com a marca, o
  servidor serve schemas planos, com as referências embutidas. Sem a marca, a sessão
  falha com erro 400 em `tools/list`. Outros clientes continuam recebendo os schemas
  originais.
- Não remova `?flavor=moonshot` da URL instalada.

Cuidado com o registro feito por outro caminho. O `kimi mcp add` do próprio Kimi
grava a URL sem a marca (**depende do produto externo**). Com essa URL, os turnos do modelo falham
com 400, inclusive os que não usam ferramentas, porque os schemas vão em toda
requisição. O teste `kimi mcp test` pode passar, porque só lista as ferramentas e não
as envia ao modelo (**depende do produto externo**). Use `ai-memory install-mcp --client kimi-code
--apply` no lugar.

Se a entrada já foi criada sem a marca, há duas saídas: rodar `install-mcp` de novo,
ou configurar o servidor para servir o dialeto restrito a todos os `tools/list`, com
`AI_MEMORY_STRIP_ROOT_COMBINATORS=true` ou `strip_root_combinators = true`. Essa
chave só remove os combinadores da raiz. Ela não embute referências `$ref`, então
**não está garantido** que basta para o Moonshot. O dialeto completo do Moonshot só é
escolhido pela marca. Veja [Dialetos de schema](#dialetos-de-schema-para-upstreams-estritos).

Hooks. Os hooks de Kimi Code ficam como entradas `[[hooks]]` em
`$KIMI_CODE_HOME/config.toml` (padrão `~/.kimi-code/config.toml`), no mesmo arquivo que
guarda as configurações de provedor e modelo. O `install-hooks` mescla apenas as
entradas do ai-memory e preserva o restante. Cada entrada aceita os campos `event`,
`matcher`, `command` e `timeout`, este último em segundos. O padrão é 30 s, e o valor
válido vai de 1 a 600 (**depende do produto externo**: o ai-memory não grava `timeout`; confira com a documentação do Kimi). Campos extras
podem impedir o carregamento do arquivo, por isso prefira `install-hooks --apply` a
editar à mão.

### Command Code

- **Suporte MCP:** sim. **Hooks:** `ai-memory install-hooks --agent command-code --apply`,
  com quatro eventos estáveis de shell (início de sessão, antes e depois de ferramenta,
  e parada). Workstreams gerenciadas não são instaladas por este fluxo (confirmado na
  auditoria 002): o Command Code é um harness gerenciado de `ai-memory run` (está entre os
  harnesses de seleção automática), então workstreams vêm de `ai-memory run command-code`, não
  de `install-hooks`.
- **Arquivo de configuração (MCP):** `~/.commandcode/mcp.json`.
- **Arquivo de configuração (hooks):** `~/.commandcode/settings.json`.

```bash
ai-memory install-mcp --client command-code --apply \
    --server-url "http://homelab:49374/mcp" --auth-token "$TOKEN"
ai-memory install-hooks --agent command-code --apply \
    --server-url "http://homelab:49374" --auth-token "$TOKEN"
```

Mescla esta entrada em `mcpServers`, no formato de HTTP de escopo de usuário:

```json
{
  "mcpServers": {
    "ai-memory": {
      "transport": "http",
      "enabled": true,
      "url": "http://homelab:49374/mcp",
      "headers": { "Authorization": "Bearer <token>" }
    }
  }
}
```

A alternativa pela linha de comando é `cmd mcp add --transport http --scope user
ai-memory <url>`. O executável `cmdc` é o nome nativo no Windows.

Os hooks são registrados sem a chave `matcher` externa. Essa é a forma documentada
pelo próprio Command Code, em que a ausência de matcher significa "todas as
ferramentas". Um matcher impediria os eventos sem ferramenta, como início e parada, de
dispararem (**depende do produto externo**). Instalações nativas gravam eventos localmente, aplicam
as exclusões de captura sobre o envelope de ferramenta documentado e injetam handoffs
pendentes com `hookSpecificOutput.additionalContext` no início.

O evento de parada marca apenas o fim de um turno. Depois do último turno, rode
`ai-memory finalize-session --agent command-code`. Se várias sessões compartilham o
projeto, adicione `--session-id <uuid>`.

### Kiro CLI

- **Suporte MCP:** sim. **Hooks:** versão 2 com `install-hooks --agent kiro-cli`, e
  versão 3 com `install-hooks --agent kiro-cli-v3`. Os dois são apresentados em
  [support-matrix.md](support-matrix.md).
- **Arquivo de configuração:** `$KIRO_HOME/settings/mcp.json`, padrão
  `~/.kiro/settings/mcp.json`. Para o escopo de projeto de menor prioridade do Kiro, use
  `--config-file .kiro/settings/mcp.json` (**depende do produto externo**). O alias do cliente é
  `kiro`.

```bash
ai-memory install-mcp --client kiro-cli --apply \
    --server-url "https://memory.example/mcp" --auth-token "$TOKEN"
```

O comando preserva configurações e servidores não relacionados, e mescla esta entrada
de forma idempotente:

```json
{
  "mcpServers": {
    "ai-memory": {
      "url": "https://memory.example/mcp?flavor=bedrock",
      "headers": { "Authorization": "Bearer <token>" }
    }
  }
}
```

- **Esquema.** O Kiro envia os schemas das ferramentas pela Amazon Bedrock, que
  rejeita combinadores `anyOf`, `oneOf` e `allOf` na raiz. O comando acrescenta
  `?flavor=bedrock`. O servidor remove só esses combinadores da raiz, mantém os
  schemas aninhados e não altera a validação dos argumentos nos handlers.
- **HTTPS obrigatório fora do loopback.** O Kiro aceita URLs HTTPS remotas. HTTP só é
  aceito para endereços de loopback: `localhost`, `127.0.0.1`, `::1` ou outro endereço
  de loopback. O comando rejeita outra URL HTTP antes de gravar qualquer coisa, tanto
  na impressão quanto no `--apply`. Para um homelab, use um proxy reverso com HTTPS.
  Veja [https-via-proxy.md](https-via-proxy.md).
- **Versões de hook.** A versão 3 usa um arquivo independente, com gatilhos em
  PascalCase, e preserva entradas de terceiros. Para projetos, o comando de hooks aceita
  `--config-file` com o agente ou o arquivo de hook do projeto. Os detalhes de cada
  versão estão em [support-matrix.md](support-matrix.md).

### OpenClaw

- **Suporte MCP:** sim. **Hooks:** `ai-memory install-hooks --agent openclaw --apply`,
  que gera um plugin nativo do OpenClaw.
- **Arquivo de configuração:** `~/.openclaw/config.json` (caminho usado pelo comando;
  confira com `openclaw config show`, pois a documentação do OpenClaw cita esse caminho
  só indiretamente (**depende do produto externo**)).

Ao contrário dos outros clientes JSON, o OpenClaw usa `mcp.servers`, com o transporte
declarado explicitamente:

```json
{
  "mcp": {
    "servers": {
      "ai-memory": {
        "url": "http://127.0.0.1:49374/mcp",
        "transport": "streamable-http",
        "headers": { "Authorization": "Bearer <token>" }
      }
    }
  }
}
```

```bash
ai-memory install-mcp --client openclaw --apply
```

Plugin de hooks:

- O `install-hooks --agent openclaw --apply` grava um pacote de plugin local no
  diretório de dados do ai-memory. Se a CLI `openclaw` estiver no `PATH`, ele roda
  `openclaw plugins install --link <diretório> --force`. Se a CLI não estiver
  disponível, imprime o comando exato para rodar depois.
- O plugin registra os eventos `session_start`, `session_end`, `before_prompt_build`,
  `before_tool_call`, `after_tool_call`, `before_compaction` e `agent_end`.
- O evento `before_prompt_build` injeta handoffs pendentes pelo resultado
  `prependContext`.
- Depois de instalar ou atualizar o plugin, reinicie o Gateway do OpenClaw. Isso não é
  necessário se o Gateway gerenciado reiniciar sozinho após mudanças no código do plugin
  (**depende do produto externo**).

### Oh My Pi (OMP)

- **Suporte MCP:** sim, com `--client omp` (alias `oh-my-pi`). **Hooks:**
  `ai-memory install-hooks --agent omp --apply` (ou `--agent oh-my-pi`). O comando grava
  a extensão `ai-memory-omp.ts` no diretório de agente do OMP, em `extensions/`.
- **Arquivo de configuração:** `mcp.json` no diretório de agente do OMP. A escolha do
  diretório segue esta ordem:
  1. Perfil nomeado, definido por `OMP_PROFILE` (ou `PI_PROFILE`), diferente de vazio
     e de `default`: `~/.omp/profiles/<nome>/agent/mcp.json`.
  2. `PI_CODING_AGENT_DIR`, quando definida: `$PI_CODING_AGENT_DIR/mcp.json`.
  3. Padrão: `~/.omp/agent/mcp.json`.

  A variável `PI_CONFIG_DIR` renomeia a pasta raiz `.omp` dentro do diretório pessoal.
  O mesmo cálculo define onde a extensão é gravada. Para um arquivo de projeto
  (`.omp/mcp.json`), use `--config-file` (**depende do produto externo**: escopo de projeto do OMP).

```bash
ai-memory install-mcp --client omp --apply
```

Entrada gerada:

```json
{
  "mcpServers": {
    "ai-memory": {
      "type": "http",
      "url": "http://127.0.0.1:49374/mcp",
      "enabled": true,
      "headers": { "Authorization": "Bearer <token>" }
    }
  }
}
```

O campo `headers` só aparece com token. Reinicie o `omp` depois de alterar o MCP ou a
extensão.

Extensão de captura:

- As extensões do OMP são módulos de extensão (arquivos `.ts`), não scripts de shell. A saída padrão não
  é usada para injetar contexto. A extensão usa os eventos de ciclo de vida do OMP
  para captura de prompts e ferramentas, e `before_agent_start` para injetar handoffs
  pendentes.
- O OMP e o Pi carregam todas as extensões `*.ts` diretas do próprio diretório. Se os
  dois apontarem para o mesmo diretório, cada evento é capturado duas vezes, uma para
  cada identidade. O `install-hooks` avisa quando detecta essa situação. Dê a cada agente
  uma casa separada, ou limite o OMP a um perfil.

### Pi

O Pi não tem arquivo MCP nativo. O `install-mcp --client pi` não grava nada e imprime
orientação para instalar a extensão gerada, que faz captura e também ponte MCP:

```bash
ai-memory install-hooks --agent pi --apply --server-url http://homelab:49374 [--auth-token <token>]
```

- **Arquivo da extensão:** `~/.pi/agent/extensions/ai-memory-pi.ts`. Com
  `PI_CODING_AGENT_DIR` definida, o arquivo vai para
  `$PI_CODING_AGENT_DIR/extensions/ai-memory-pi.ts`.
- **O que a extensão faz:** envia eventos de ciclo de vida para o endpoint de hooks,
  busca handoffs pendentes em `before_agent_start`, inicializa o endpoint HTTP
  `/mcp`, lista as ferramentas e registra cada uma delas no Pi.
- Reinicie o Pi depois de instalar a extensão.
- `install-mcp --client pi --apply` falha sem gravar, com a mesma orientação. A
  mensagem de orientação impressa pelo código cita `~/.pi/agent/extensions/ai-memory.ts`,
  um nome diferente do arquivo que o instalador grava. Ver a seção de divergências na
  evidência de revisão.

O OMP (`--client omp`) é outro cliente, com diretórios `.omp`. Não o confunda com o Pi.

---

## Dialetos de schema para upstreams estritos

Alguns provedores de modelo validam os schemas das ferramentas MCP contra um dialeto
mais restrito que o JSON Schema, e rejeitam a lista de ferramentas inteira com erro 400.
O ai-memory pode servir um dialeto relaxado por requisição, pela marca `?flavor=` na
URL do MCP, ou para todas as requisições, pela configuração do servidor. Em qualquer
dialeto, a validação dos argumentos em tempo de execução é a mesma. Só muda o schema
anunciado.

| Marca | Chave de configuração do servidor | O que muda | Quem precisa |
|---|---|---|---|
| `?flavor=moonshot` | Nenhuma própria. Só pela marca | Remove combinadores `anyOf`, `oneOf` e `allOf` da raiz e embute as referências `$ref`, em qualquer profundidade. Os combinadores aninhados ficam inline | Kimi Code (Moonshot). O `install-mcp` acrescenta a marca |
| `?flavor=bedrock` | `strip_root_combinators` | Remove os combinadores da raiz. Mantém `$ref` e schemas aninhados | Kiro CLI (Bedrock). O `install-mcp` acrescenta a marca |
| `?flavor=gemini` (alias `vertex`) | `gemini_safe_schemas` | Igual ao bedrock, mais as uniões de tipo nulo colapsadas em um único `type` com `nullable: true`. Mantém `$defs` e `$ref` | Clientes que repassam os schemas sem alteração ao Gemini ou ao Vertex, por exemplo o OpenCode com um modelo do Vertex |

Chaves de configuração e variáveis de ambiente:

- `strip_root_combinators` no arquivo de configuração, ou `AI_MEMORY_STRIP_ROOT_COMBINATORS=true`.
- `gemini_safe_schemas` no arquivo de configuração, ou `AI_MEMORY_GEMINI_SAFE_SCHEMAS=true`.
  Essa chave inclui a de combinadores.

O servidor só eleva o piso do dialeto, nunca o reduz. Cada marca na requisição também
só eleva o dialeto. Se houver mais de uma marca, vale a mais restrita.

O `install-mcp` acrescenta `moonshot` e `bedrock` automaticamente para os clientes
com upstream fixo. Para os demais, passe o dialeto explicitamente:

```bash
ai-memory install-mcp --client command-code --flavor gemini --apply
```

O `--flavor` não é padrão por cliente porque clientes como o Command Code e o OpenCode
atendem a vários modelos. Só o modelo do Vertex precisa do dialeto Gemini, e fixá-lo
para todos reduziria o schema anunciado para modelos que nunca tiveram o problema.

Por que o dialeto Gemini existe. O gerador de schemas do servidor expressa cada
argumento opcional como uma união de tipo com `null`, por exemplo:

```json
"max_proposals": {
  "description": "Override the maximum validated proposal count for this run.",
  "type": ["integer", "null"], "format": "uint", "minimum": 0
}
```

O `Schema` do Gemini e do Vertex aceita um único `type`, e trata `any_of` como exclusivo
de qualquer outro campo irmão. Um conversor que repassa esse schema sem alteração gera
`any_of` junto com `description`, e o Vertex recusa a requisição com um erro como:

```text
Unable to submit request because `ai-memory_memory_auto_improve` functionDeclaration
`parameters.max_proposals` schema specified other fields alongside any_of.
When using any_of, it must be the only field set.
```

O dialeto converte isso em `"type": "integer"` com `nullable: true`. É a mesma
normalização que o Gemini CLI faz do lado do cliente. Por isso o Gemini CLI e o
Antigravity CLI funcionam no Vertex sem marca. Use o dialeto quando um cliente que só
repassa falhar em `tools/list` com esse erro.

O `install-mcp` não acrescenta `?flavor=gemini` para nenhum cliente. No caso do
OpenCode, o ajuste certo é a chave do servidor, porque o cliente é independente do
provedor.

```bash
# No servidor, para todos os clientes que não conseguem enviar marca na URL
AI_MEMORY_GEMINI_SAFE_SCHEMAS=true ai-memory serve --transport http
# ou gemini_safe_schemas = true no arquivo de configuração

# Ou por cliente, à mão, na configuração MCP dele
#   "url": "http://homelab:49374/mcp?flavor=gemini"
```

Para conferir o dialeto recebido por uma requisição, consulte `tools/list` diretamente:

```bash
curl -s 'http://127.0.0.1:49374/mcp?flavor=gemini' \
    -H 'Accept: application/json, text/event-stream' \
    -H 'Content-Type: application/json' \
    -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' \
  | jq '.result.tools[] | select(.name=="memory_auto_improve")
        | .inputSchema.properties.max_proposals'
```

---

## Verificar o registro

Independentemente do cliente, a primeira verificação é a mesma: peça ao modelo que
liste as ferramentas MCP disponíveis, ou que chame `memory_status` explicitamente.

```text
Você: Liste as ferramentas MCP que você pode chamar. Use uma delas para verificar o
      status do ai-memory.

Modelo: (qualquer cliente) Posso chamar as ferramentas do ai-memory, incluindo
      memory_query, memory_recent, memory_status, memory_briefing, memory_explore,
      memory_handoff_begin, memory_handoff_accept, memory_write_page e
      memory_install_self_routing, entre outras. memory_status reporta: 0 páginas,
      0 observações, 0 sessões.
```

O servidor expõe 24 ferramentas MCP. A lista completa, com entradas e saídas, está em
[mcp-server.md](mcp-server.md). O exemplo acima é ilustrativo.

Se o modelo vir as ferramentas mas não as usar por iniciativa própria, atualize o pacote
de roteamento gerenciado. A ferramenta `memory_install_self_routing` é somente leitura. Ela
devolve:

- o bloco de instruções compacto delimitado pelos marcadores
  `<!-- ai-memory:start -->` e `<!-- ai-memory:end -->`;
- o nome do arquivo de regras de cada agente. O Claude Code usa `CLAUDE.md`, e os demais
  clientes listados usam `AGENTS.md`;
- as skills gerenciadas, com `name`, `description`, `relative_path` e `content`;
- as raízes de destino `project` e `global` para as skills, como `.claude/skills`,
  `.agents/skills`, `.grok/skills` e `$GROK_HOME/skills`, além de outras;
- a orientação de sobrescrita.

O agente deve substituir apenas o bloco existente entre os marcadores, e gravar cada
skill gerenciada no `relative_path` indicado, preservando o conteúdo do usuário. Uma
skill sem o marcador gerenciado não deve ser sobrescrita sem confirmação humana.

Se o modelo não enxergar nenhuma das ferramentas, a entrada MCP não foi carregada.
Confira:

1. **O servidor está rodando?** Um GET em `/healthz` (sem autenticação) responde se o
   processo estiver escutando. Se a conexão for recusada, inicie o ai-memory com
   `ai-memory serve --transport http`, ou com `docker start ai-memory` se estiver em
   contêiner. Um GET em `/mcp` sem sessão responde 405, e isso é esperado.
2. **O cliente recarregou a configuração?** O Claude Desktop e o OMP precisam de
   reinício. O Cursor precisa de recarga do MCP ou de desligar e ligar o servidor. Para
   os plugins do OpenClaw, reinicie o Gateway, a menos que ele reinicie sozinho.
3. **A porta está certa?** O padrão é **49374**. Se você mudou a porta, atualize a URL em
   todas as configurações de cliente.

Se o modelo vê as ferramentas, mas todas retornam erro, o servidor provavelmente usa
outro diretório de dados. Verifique com `ai-memory status --json`, que mostra o diretório
de dados em uso, ou com `docker logs ai-memory`.

---

## Quando o handoff automático funciona

O handoff entre agentes exige que os dois lados conversem com o ai-memory. Um lado
é o agente que **encerra** a sessão. O outro é o agente que **inicia** a próxima.

| Lado | O que é necessário | Como é coberto |
|---|---|---|
| **Encerramento** | O agente cria o handoff por um evento real de fim de sessão, pelo finalizador manual ou por `memory_handoff_begin`. | Automático para os clientes com evento real de fim de sessão, por hook. Para clientes sem esse evento confiável, rode `ai-memory finalize-session --agent <cliente>` depois do último turno. Clientes somente MCP, como o Swival, precisam chamar `memory_handoff_begin` explicitamente. |
| **Início** | Ou (a) o hook de início injeta o handoff, ou (b) o modelo lista com `memory_handoff_list` e aceita com `memory_handoff_accept`, informando o `handoff_id`. | (a) exige um cliente que consuma a saída do hook de início, ou um resultado equivalente de injeção de contexto. Grok, Zero, Hermes e Swival não injetam pelo início. Para esses, use (b). (b) funciona com qualquer cliente com suporte a MCP, se você orientar o modelo a usá-lo. |

Por cliente, a matriz exata de quais eventos são suportados fica em
[support-matrix.md](support-matrix.md) e [harness-adapters.md](harness-adapters.md). O
desenho do handoff é descrito em
[design-handoff-offer-not-claim.md](design-handoff-offer-not-claim.md).

Notas sobre o encerramento:

- O OpenCode usa o evento oficial de plugin `session.deleted` para entregar o fim de sessão.
- O Codex registra um evento nativo de fim de sessão a partir da versão 0.145.0. Em
  versões anteriores, ou se o evento nativo falhar, use `ai-memory finalize-session`. Sem
  `--agent`, ele assume o Codex.
- Os eventos de parada do Codex e do Antigravity não são fim de sessão. O Antigravity
  precisa de finalização explícita com `--agent antigravity-cli`.
- `finalize-session` encontra a sessão aberta mais recente do workspace ou projeto atual
  e envia um evento sintético de `session-end` pelo mesmo caminho do servidor. Use `--all`
  só quando quiser fechar todas as sessões abertas correspondentes no escopo escolhido.

Exemplo de fluxo misto:

- **Claude Code para Cursor.** O `SessionEnd` do Claude Code cria o handoff sozinho. O
  hook `sessionStart` do Cursor busca e injeta o handoff, quando o
  `install-hooks --agent cursor --apply` está instalado.
- **Claude Desktop para Claude Code.** O Claude Desktop não grava handoff, porque não tem
  hooks. Para retomar no Claude Code, peça ao modelo do Claude Desktop que chame
  `memory_handoff_begin` antes de fechar. O conteúdo da wiki continua acessível por
  `memory_query` em qualquer caso.
