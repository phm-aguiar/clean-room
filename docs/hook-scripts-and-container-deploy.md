# Scripts de hook, imagem de contêiner e deploy

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

Este documento descreve o que roda fora do processo principal para ligar agentes de código ao servidor: os scripts de hook de cada agente (variante POSIX e variante Windows), a imagem de contêiner e as formas de deploy.

O caminho nativo, em que o próprio binário recebe o evento do agente, está na seção de cliente de hooks nativo do [documento de CLI](cli.md). As duas formas produzem a mesma requisição ao servidor.

A tradução de cada evento para o vocabulário interno (aliases, importância, título e corpo) está em [ingestão de eventos de ciclo de vida](lifecycle-event-ingestion.md) e não é repetida aqui. Para configurar o Windows nativo, veja [suporte a Windows](windows.md).

---

## 1. Scripts de hook por agente

### 1.1 Nomes e eventos

Cada agente tem um script por evento, em duas variantes: POSIX (shell) e Windows (PowerShell). O nome do script é o nome do evento em kebab-case. O valor enviado no parâmetro `event` é o evento canônico aceito pelo servidor; por exemplo, o evento `user-prompt-submit` envia `user-prompt`. O parâmetro `agent` também pode diferir do nome do agente: o agente opencode envia `open-code`.

| Agente (`agent=`) | Eventos com script | Não existem para este agente |
|---|---|---|
| claude-code | session-start, user-prompt-submit, pre-tool-use, post-tool-use, pre-compact, stop, session-end, subagent-start, subagent-stop | — |
| copilot-cli | igual ao claude-code | — |
| kimi-code | igual ao claude-code | — |
| grok | igual ao claude-code | — |
| codex | session-start, user-prompt-submit, pre-tool-use, post-tool-use, pre-compact, stop, session-end | subagentes |
| cursor | igual ao codex | subagentes |
| gemini-cli | igual ao codex | subagentes |
| opencode (`open-code`) | igual ao codex | subagentes |
| devin | session-start, user-prompt-submit, pre-tool-use, post-tool-use, post-compaction, stop, session-end | pre-compact, subagentes |
| kiro-cli | session-start, user-prompt-submit, pre-tool-use, post-tool-use, stop | pre-compact, session-end, subagentes |
| pool | session-start, user-prompt-submit, pre-tool-use, post-tool-use, stop | pre-compact, session-end, subagentes |
| command-code | session-start, pre-tool-use, post-tool-use, stop | user-prompt-submit, pre-compact, session-end, subagentes |
| antigravity-cli | session-start, pre-tool-use, post-tool-use, stop | user-prompt-submit, pre-compact, session-end, subagentes |

A lista de eventos aceitos, com seus aliases, está em [ingestão de eventos de ciclo de vida](lifecycle-event-ingestion.md). Eventos de subagente e os de tipo "outro" são armazenados com o tipo de observação "outro".

### 1.2 Sequência comum

Todos os scripts seguem a mesma sequência:

1. Carregam a biblioteca compartilhada do mesmo diretório ou, quando rodam a partir da árvore do repositório, do diretório pai.
2. Definem a URL do servidor: o valor da variável `AI_MEMORY_HOOK_URL`, ou `http://127.0.0.1:49374` se ela não estiver definida.
3. Leem o payload inteiro da entrada padrão. O payload é o JSON do evento enviado pelo agente.
4. Resolvem o diretório de trabalho (cwd) a partir do campo `cwd` do payload. Os scripts do Devin acrescentam dois fallbacks: a variável `DEVIN_PROJECT_DIR` e, por último, o diretório do processo. Os demais agentes não têm esses fallbacks.
5. Montam a query de marcador (seção 2.2).
6. Enviam o payload, sem alteração, por POST para `/hook`, com `event` e `agent` na query. O envio tem timeout curto; saída e erros são descartados.
7. Só nos eventos de início de sessão (e nos casos da seção 1.3), fazem GET de `/handoff` de forma síncrona e devolvem o conteúdo no formato que o agente espera.
8. Escrevem a resposta do protocolo do agente e saem com código 0 sempre. Um hook nunca pode falhar nem bloquear o agente.

### 1.3 Respostas ao agente

| Situação | Saída no stdout |
|---|---|
| Eventos de rotina em claude-code, copilot-cli, command-code, devin e grok | Objeto vazio `{}` |
| Eventos de rotina em codex, cursor, gemini-cli, opencode, kimi-code, kiro-cli e pool | Nada |
| post-tool-use em antigravity-cli | Objeto vazio `{}` |
| session-start em claude-code, devin e command-code | Objeto com a chave de saída específica de SessionStart e o handoff como texto em `additionalContext`; sem handoff, `{}` |
| session-start em copilot-cli | Objeto com `additionalContext` no topo; sem handoff, `{}` |
| session-start em antigravity-cli | Só na primeira invocação da sessão (contador `invocationNum` igual a 0): objeto com `injectSteps`, contendo o handoff em `ephemeralMessage`; nas demais, `{}` |
| session-start em codex, cursor, gemini-cli e opencode | Texto cru do handoff, sem envelope; nada se não houver handoff |
| session-start em kiro-cli | Texto cru do handoff; o GET inclui o briefing só na primeira vez da sessão |
| session-start em grok e pool | Sem GET de handoff; grok responde `{}` |
| user-prompt-submit em kimi-code | Texto cru do handoff (o agente descarta a saída do session-start, por isso o handoff chega pelo primeiro prompt) |
| post-tool-use em grok | Contexto de PostToolUse com o handoff, só na primeira chamada da sessão; depois, `{}` |
| pre-tool-use em antigravity-cli | Objeto de decisão `allow` |
| stop em antigravity-cli | Objeto de decisão vazia |
| stop em claude-code com o campo bruto de mensagem do assistente (seção 1.6) | `{}`, sem envio |
| subagent-stop em claude-code, copilot-cli, grok e kimi-code | `{}`, exceto kimi-code, que não imprime nada |

O handoff é o texto de continuidade gerado pelo servidor para o próximo início de sessão (ver [ingestão de eventos de ciclo de vida](lifecycle-event-ingestion.md), seção de handoff no início de sessão).

### 1.4 Devin: identidade de sessão e eventos

Devin não envia um identificador de sessão estável no payload. Os scripts do Devin geram um identificador local no formato `devin-<epoch>-<pid>` e o gravam no diretório de estado (seção 2.7):

- O evento de início de sessão sempre gera um identificador novo e o grava.
- Os demais eventos releem o arquivo. Se o arquivo não existir, geram e gravam um novo identificador.
- O evento de fim de sessão apaga o arquivo depois de enviar o evento.
- Se a variável `AI_MEMORY_SESSION_ID` estiver definida, ela fixa o identificador e o arquivo não é usado.

O servidor aceita identificadores que não são UUID: o texto é convertido em UUID v5 de forma determinística. Um identificador que já é UUID é usado como está. Como POST e GET usam o mesmo texto, eles caem na mesma chave.

O início de sessão do Devin faz POST e GET de handoff; o GET leva o identificador de sessão e, se o marcador do projeto ativar o briefing, o parâmetro de briefing.

O evento `post-compaction` é específico do Devin. Ele é disparado depois da compactação do contexto e traz um campo `summary`. O servidor faz checkpoint da página da sessão com esse resumo (ver [ingestão de eventos de ciclo de vida](lifecycle-event-ingestion.md), seção de checkpoints).

### 1.5 Subagentes

O evento `subagent-stop` existe para claude-code, copilot-cli, grok e kimi-code. Ele encaminha o payload por POST para o servidor, sem injetar contexto, para que o servidor esqueça o identificador da sessão do subagente quando o projeto optou por descartar capturas de subagente. O tratamento está em [ingestão de eventos de ciclo de vida](lifecycle-event-ingestion.md), seção de descarte de subagente.

O evento `subagent-start` é encaminhado do mesmo modo. Os demais agentes não têm eventos de subagente.

### 1.6 Stop do Claude Code com mensagem do assistente

O script de stop do claude-code verifica se o payload bruto contém a chave `last_assistant_message`, que traz o texto da última resposta do assistente. Se contiver, o script descarta o evento inteiro e responde `{}`. A variante script não consegue sanitizar esse campo, então o evento é descartado em vez de enviado sem tratamento. A captura da mensagem do assistente exige a instalação nativa (ver [ingestão de eventos de ciclo de vida](lifecycle-event-ingestion.md), seção de captura da mensagem do assistente).

---

## 2. Biblioteca compartilhada (variante POSIX)

A biblioteca comum dos scripts POSIX é escrita em shell POSIX, sem parser JSON e sem dependências além de um cliente HTTP de linha de comando e ferramentas de texto padrão. O instalador a copia para o diretório de hooks junto com os scripts.

### 2.1 Descoberta do marcador `.ai-memory.toml`

- A busca sobe a partir do cwd procurando o arquivo `.ai-memory.toml` (ver [arquivo marcador](marker-file.md)).
- Fronteira: se o cwd está dentro do diretório pessoal do usuário (`$HOME`), a busca para nele. Se está fora, a busca para na raiz do checkout git mais próximo (diretório ou arquivo `.git`). Sem checkout, só o próprio cwd é verificado.
- Um marcador que declara apenas a seção `[capture]` é transparente para escopo e configurações: a busca segue para o próximo ancestral. Um marcador que declara `workspace`, `project`, `project_strategy`, `drop_subagent_captures`, `identity`, `identity_style`, `aliases`, `server`, `default_global`, `inject_on_session_start`, `max_chars`, `contribute` ou `consume` é um marcador de configuração. A seção `[capture]` sempre usa o marcador mais próximo, sem pular nenhum.
- Leitura: `chave = valor` na raiz do arquivo, com ou sem aspas (no caso de flags). Comentário no fim da linha é removido. Vale a primeira ocorrência, e cabeçalhos de seção são ignorados.
- Perfil de servidor: se algum marcador na subida declara `server = ...`, o script não consegue rotear o perfil. Nesse caso a query recebe `server_routed=1`, o POST é descartado e o GET de handoff não é feito. Dentro do diretório pessoal, a subida para em `$HOME`; fora dele, continua além da raiz do checkout, para que um marcador de organização acima do repositório conte.
- Rotas no marcador do diretório pessoal (seções `routes` por caminho ou por identidade do repositório) mapeiam workspace e projeto. Limites: arquivo de até 64 KiB, até 64 rotas, seletor de até 512 bytes e nomes que casam com `[a-z0-9][a-z0-9._-]*`. Vence a rota de maior profundidade de caminho. Empate ou configuração inválida gera o parâmetro `ai_memory_invalid_home_routes=1`: o POST é descartado e o GET não é feito.

### 2.2 Query string de marcador

Os campos abaixo são enviados com percent-encoding, na ordem indicada:

- `cwd`: sempre, quando o cwd existe.
- `workspace` e `project`: do marcador (ou da rota do diretório pessoal).
- `project_src`: `marker` quando o projeto veio do marcador ou de rota; `repo-root` quando veio do nome da raiz do repositório principal.
- `project_strategy`: do marcador. Se o marcador não fixa estratégia, usa a variável `AI_MEMORY_PROJECT_STRATEGY` definida na instalação.
- `identity`, `identity_src` (`explicit` ou `git_remote`) e `identity_style` (`path` ou `host_path`; ausente ou inválido vira `path`).
- `aliases`: lista JSON de até 16 itens, cada um com até 128 caracteres no padrão `[A-Za-z0-9][A-Za-z0-9._-]*`.
- `drop_subagent`, `default_global`, `profile_contribute` e `profile_consume`. Os dois últimos valem 0 para `0`, `false`, `no` ou `off` (sem distinção de caixa) e 1 para qualquer outro valor. São enviados só quando há marcador.
- `managed_run`: a partir da variável `AI_MEMORY_RUN_ID`, quando definida.

Regras de resolução:

- Projeto por raiz de repositório: se a estratégia for `repo-root` e nenhum projeto estiver fixado, o script calcula no host o nome do repositório principal (seguindo worktrees) e o envia como projeto. O servidor em contêiner não enxerga o checkout, por isso a resolução acontece no host. Fora de um repositório git, nada é enviado.
- Identidade: uma identidade explícita no marcador vence (em minúsculas). Um projeto declarado, sem aliases, impede a consulta ao git. Sem nada disso, usa a URL do remoto `upstream` e, na falta dele, `origin`. Credenciais são removidas no host. Remotos com esquema `file` e caminhos locais são ignorados. Formatos `host:caminho` e `esquema://host/caminho` são normalizados para `host/caminho` em minúsculas, sem `.git` final.
- Aliases só são aceitos com identidade git ou projeto. Em outros casos o valor enviado é `invalid` (a confirmar como o servidor trata esse valor).
- Briefing: `briefing` e `briefing_budget` são enviados só quando `inject_on_session_start` vale `1`, `true`, `yes` ou `on` (sem distinção de caixa). O `max_chars` vira o orçamento. O briefing não faz parte da query de marcador comum. É enviado no GET de início de sessão a cada início. Nos agentes que controlam a entrega (kimi-code, kiro-cli e grok), ele vai só uma vez por sessão.

### 2.3 Envio (POST)

- Timeout de 0.2 s. É o orçamento de um hook para não bloquear o agente.
- Autenticação: a variável `AI_MEMORY_AUTH_TOKEN`, quando definida, vira o cabeçalho `Authorization: Bearer`. Se não estiver definida, o script usa o arquivo de cabeçalho de autenticação do diretório de dados (criado pelo instalador com permissão 0600), passado como arquivo para que o token não apareça na lista de processos. Sem nenhum dos dois, o envio é anônimo.
- Chave de ingestão (`ingest_key`): é gerada antes do primeiro envio, no formato `sh` seguido de 16 caracteres hexadecimais aleatórios (com fallback baseado em data e pid). A mesma chave acompanha a entrega repetida pelo spool, para que o servidor descarte duplicatas.
- Resposta do servidor:
  - 2xx: dispara a drenagem destacada do spool (seção 2.5).
  - 408, 425 e 429: a entrega vai para o spool.
  - Qualquer outro 4xx: descartado. É rejeição permanente e não é repetido.
  - 5xx, timeout ou falha de conexão (código 000): vai para o spool.
- Captura externa: se `AI_MEMORY_CAPTURE_OWNER` tiver texto não vazio (após remover espaços), o POST e o spool são pulados e o payload é descartado. O GET de handoff continua a ser feito, porque é entrega e não captura.
- Rotas inválidas do diretório pessoal e perfis roteados também descartam o POST (seção 2.1).
- Erros nunca se propagam. A saída do POST é descartada.

### 2.4 Handoff (GET)

- Timeout de 1.0 s. O resultado vira contexto síncrono do agente, por isso o limite é um pouco maior que o do POST.
- Usa a mesma autenticação do POST.
- Não é feito para perfis roteados nem para rotas inválidas do diretório pessoal.
- Nada é gravado no spool quando o GET falha: o handoff simplesmente não é entregue nessa sessão.

### 2.5 Spool offline e drenagem

Quando um envio não é entregue (seção 2.3), o evento vai para uma fila em disco, no diretório `hook-spool` dentro do diretório de estado.

- Permissões: diretório 0700 e arquivos 0600, criados com máscara que nega acesso a outros usuários.
- Nome do arquivo: timestamp em milissegundos com 13 dígitos, pid, sequência de 16 dígitos hexadecimais e extensão `.json`. A ordem alfabética dos nomes é a ordem de chegada. O arquivo é escrito primeiro com sufixo `.tmp` e depois renomeado, de modo que um leitor nunca vê arquivo parcial.
- Conteúdo: a URL completa (com `ingest_key`), o corpo original do payload, `created_ms`, `auth_mode` (`static` ou `none`), o token em texto quando `auth_mode` é `static`, e `attempts` igual a 0. O token fica no arquivo, protegido só pelas permissões.
- Timestamp: se a fonte de tempo em alta resolução não estiver disponível, usa segundos vezes 1000.
- Drenagem: é disparada em processo destacado depois de qualquer envio 2xx, para não esperar o agente. Se não houver nenhum arquivo, nada é feito. A passagem processa os arquivos do mais antigo para o mais novo, no máximo 64 por passagem.
  - Cada entrega é um POST com timeout de 2.0 s, usando o token guardado no arquivo.
  - 2xx ou 4xx: o arquivo é apagado.
  - 408, 425 ou 429, ou qualquer outro código: a passagem para, e o restante fica para a próxima. A ordem é preservada.
  - Arquivos com escapes JSON além dos simples (por exemplo `\uXXXX`) não são decodificados pela variante POSIX. Esses arquivos são pulados e mantidos, para serem processados pela drenagem do binário nativo.
- Nenhum script chama a drenagem de forma síncrona. O comentário da biblioteca menciona drenagem nas fronteiras de sessão, mas a única chamada é a destacada, disparada após um 2xx (a confirmar se há outro caminho de drenagem fora dos scripts).

### 2.6 Identidade de sessão nos demais agentes

Fora do Devin, o identificador de sessão vem do próprio payload, pelas chaves `session_id`, `sessionId`, `sessionID`, `session` e `conversationId`, na primeira que tiver valor de texto. Ele é enviado no GET de handoff, e não no POST de início de sessão. A variável `AI_MEMORY_SESSION_ID` só tem efeito nos scripts do Devin, o único agente que usa o mecanismo de arquivo.

### 2.7 Estado local

- Diretório de estado, variante POSIX: a variável `AI_MEMORY_DATA_DIR`; senão `XDG_DATA_HOME` seguido de `/ai-memory`; senão `HOME` seguido de `/.local/share/ai-memory`; senão `.ai-memory` relativo ao cwd.
- Diretório de estado, variante Windows: `AI_MEMORY_DATA_DIR`; senão `XDG_DATA_HOME` seguido de `\ai-memory`; senão `LOCALAPPDATA` seguido de `\ai-memory`; senão `HOME` seguido de `\.local\share\ai-memory`.
- Subdiretórios:
  - `hook-state`: identificadores de sessão gerados (seção 1.4).
  - `hook-spool`: fila offline (seção 2.5).
  - `briefed`: marcadores de entrega única por sessão. Os dois variantes mantêm só os 512 marcadores mais recentes.

### 2.8 Codificação e extração

- Percent-encoding de query: a lista permitida é letras, dígitos e os caracteres `.`, `_`, `~` e `-`. Qualquer outro byte, inclusive cada byte de caracteres multibyte em UTF-8, é codificado.
- Escape JSON de texto: o valor é envolvido em aspas. Barra invertida, aspas, tabulação e retorno de carro são escapados. Quebra de linha vira `\n`. Os demais caracteres de controle (U+0001 a U+001F) viram `\u00XX`. A conversão é linear no tamanho do texto.
- Caminho do cwd: sequências de escape `\\` viram `\` e `\/` viram `/`, para que caminhos Windows entrem corretamente na query.
- Cwd em payloads de agentes sem campo `cwd`: se o campo `cwd` não tiver valor de texto, usa o primeiro item de `workspacePaths` (Antigravity) ou de `workspace_roots` (Cursor).
- Antigravity: o início de sessão é reconhecido só quando `invocationNum` vale exatamente 0. Campo ausente ou malformado conta como não-inicial, para que uma invocação repetida não consuma o handoff de outra sessão.

---

## 3. Variante Windows (PowerShell)

Os scripts Windows são finos: cada um carrega uma biblioteca e chama uma única função com o evento, o agente e as opções. Essa função tem parâmetros para buscar o handoff, para o formato de saída de cada agente (envelope do Copilot CLI, saída do Antigravity, contexto de PostToolUse do Grok) e para o briefing de entrega única. A variante POSIX não roda em PowerShell puro. No Windows nativo, use a variante PowerShell ou o binário nativo.

Sequência de execução:

1. Lê a entrada padrão com limite de 2 s, somente se a entrada estiver redirecionada. Caso contrário, o payload é vazio.
2. Antigravity: se `invocationNum` não for um inteiro igual a 0, responde `{}` e sai.
3. Stop do claude-code com o campo `last_assistant_message`: descarta o evento (seção 1.6).
4. Resolve o cwd (payload; no Devin, `DEVIN_PROJECT_DIR`; por último, o diretório do processo).
5. Se o cwd cai em perfil roteado ou em rota inválida, não envia nada. Responde `{}` para Antigravity e Grok.
6. Acrescenta `managed_run` a partir de `AI_MEMORY_RUN_ID`.
7. No Devin, resolve o identificador de sessão com as mesmas regras da seção 1.4. No `session-end`, apaga o arquivo.
8. Autenticação: usa somente `AI_MEMORY_AUTH_TOKEN`. Não lê o arquivo de cabeçalho de autenticação.
9. POST com timeout de 3 s e chave de ingestão no formato `ps` seguido de 16 caracteres hexadecimais. Códigos de resposta: 2xx dispara a drenagem destacada; 4xx, exceto 408, 425 e 429, é descartado; os demais vão para o spool. O spool usa o mesmo formato de arquivo e a mesma regra de drenagem da variante POSIX.
10. Se houver handoff a buscar, faz GET com timeout de 2 s. Saída: texto cru, salvo quando as opções pedem envelope. No Grok, o contexto é truncado em 10 000 caracteres com a marca `[truncated]` no final. No Antigravity, vai dentro de `injectSteps`. Sem handoff, com opções de envelope, responde `{}`.
11. Grok no PostToolUse: payload de subagente (chaves `subagentType`, `subagent_type`, `agent_type`, `agent_id` ou `parentSessionId` com texto) responde `{}` sem GET. Depois, se já houve handoff nesta sessão, responde `{}` sem GET. A marca de "já mostrado" é por sessão, ou por hash de agente e cwd quando não há identificador de sessão.
12. Briefing de entrega única (kimi-code): a marca é por sessão ou por hash de agente e cwd, e só é gravada depois do GET, de forma fail-open. O digest de perfil é enviado uma vez por sessão; nas seguintes, vai com `profile_digest=0`.

Diferenças entre as variantes:

| Aspecto | POSIX | Windows |
|---|---|---|
| Timeout do POST | 0.2 s | 3 s |
| Timeout do GET de handoff | 1.0 s | 2 s |
| Timeout de cada entrega da drenagem | 2.0 s | 3 s |
| Autenticação por arquivo de cabeçalho | sim | não |
| Diretório de estado | sem `LOCALAPPDATA` | com `LOCALAPPDATA` |
| Prefixo da chave de ingestão | `sh` | `ps` |
| Limite do contexto do Grok | sem limite | 10 000 caracteres |
| Marca de "já mostrado" do Grok | soma de verificação de agente e cwd | hash SHA-256 de agente e cwd (a confirmar que as duas variantes geram marcas compatíveis) |
| Ao receber stop com mensagem do assistente | descarta | descarta |

---

## 4. Imagem de contêiner

### 4.1 Receita de build

A receita tem quatro estágios:

- **Build**: toolchain da linguagem na versão 1.95, em imagem slim do Debian bookworm. Copia os arquivos de manifesto e de lock de dependências, os componentes do workspace, o diretório de avaliação (membro do workspace; sem ele a resolução do workspace falha) e o diretório de documentação (as imagens da interface web são embutidas no binário). Compila o binário de linha de comando em modo release, com lock travado. Usa caches de build do BuildKit.
- **Base de runtime**: Debian bookworm-slim, com apenas os certificados de autoridade certificadora instalados (para chamadas HTTPS a provedores de LLM). Cria o usuário de sistema `ai-memory` com uid e gid 1000, sem shell, com `/data` como diretório pessoal e dono.
- **Runtime de origem** (alvo padrão, o último estágio do arquivo): copia o binário do estágio de build e roda como usuário `ai-memory`.
- **Runtimes pré-compilados** (amd64 e arm64): copiam o binário de um diretório de artefatos já compilados, em vez de compilar de novo. O CI usa esses alvos para não compilar a linguagem duas vezes.

Todos os estágios de runtime copiam os scripts de hook para `/usr/local/share/ai-memory/hooks`, para que o comando de configuração de agente funcione sem checkout do repositório.

Variáveis de ambiente da imagem:

- `AI_MEMORY_DATA_DIR=/data`.
- `AI_MEMORY_ALLOWED_HOSTS=localhost,127.0.0.1,::1,host.docker.internal`. Esse padrão permite que o wrapper de macOS alcance o servidor pelo nome do host do Docker Desktop. Um deploy exposto deve definir a própria lista, que substitui este valor inteiro.
- `AI_MEMORY_IN_CONTAINER=1`. Diz ao servidor que está em contêiner. O bind em `0.0.0.0` é exigência do namespace de rede, não prova de exposição à rede; a verificação de exposição HTTP do servidor usa esse sinal.

Metadados e processo:

- Diretório de trabalho `/data`. Porta exposta: 49374.
- Healthcheck: executa o comando `status` do binário a cada 30 s, com timeout de 5 s, período inicial de 5 s e 3 tentativas.
- Entrypoint: o binário. Comando padrão: `serve` em transporte HTTP, bind `0.0.0.0:49374`, com interface web habilitada.

### 4.2 Composições de referência

| Cenário | O que faz | Pontos-chave |
|---|---|---|
| Desenvolvimento local | Constrói a imagem a partir da receita e sobe o servidor | Usuário 1000:1000; volume nomeado para `/data`; porta publicada só em loopback (`127.0.0.1:49374`); arquivo de ambiente `.env` opcional; filtro de log por componente em `RUST_LOG`; healthcheck de 30 s |
| Homelab na LAN (modelo de produção) | Usa imagem publicada em registry | Porta publicada em `0.0.0.0:49374`; bind mount de diretório do host, para que o backup seja feito copiando esse diretório; rótulo SELinux desativado (necessário em sistemas com SELinux enforcing, como MicroOS e Fedora); segredos no arquivo `.env.production` |
| TLS com proxy reverso (Caddy) | Termina HTTPS no proxy | O servidor não publica porta no host; o proxy escuta em 80 e 443. Volumes do proxy guardam a chave de conta e os certificados, que devem ter backup. Três variantes de configuração do proxy: domínio público com Let's Encrypt; LAN com autoridade certificadora interna (os nomes e IPs devem estar no SAN, e a raiz deve ser instalada em cada cliente); certificados externos com recarga automática. Passo a passo em [HTTPS por proxy reverso](https-via-proxy.md) |
| TLS com túnel de saída (Cloudflare Tunnel) | Expõe o servidor por túnel iniciado pelo host | Sem portas de entrada; o token do túnel vem da variável `CLOUDFLARE_TUNNEL_TOKEN`; o hostname público deve apontar para o serviço interno na porta 49374, configurado no painel do provedor |
| Harness de múltiplos operadores | Simula dois a três operadores atrás de um proxy de identidade | Ver abaixo |

Harness de múltiplos operadores: o servidor roda com a configuração de autenticação de administrador, um token de proxy distinto e a opção de slots por usuário ativada. Um proxy escuta em uma porta por operador (alice, bob e carol, a última identificada só pelo sujeito OIDC). Cada porta substitui o cabeçalho `Authorization` pelo token do proxy e fixa os cabeçalhos `X-Memory-Actor-*` do operador, zerando os que não afirma. A porta 49374 fica publicada em loopback sem proxy de propósito, para testar que cabeçalhos `X-Memory-Actor-*` forjados sem o segredo são ignorados e que cabeçalho duplicado falha fechado.

Requisito de segurança de qualquer proxy que injeta identidade: o proxy deve **substituir** os cabeçalhos `X-Memory-Actor-*`, nunca anexá-los. Se anexar, o valor do cliente chega primeiro e o servidor responde 400 por ambiguidade (ver [usuários e atribuição](users.md), seção de identidade por proxy confiável).

### 4.3 Variáveis em arquivo de ambiente

O arquivo de ambiente tem uma atribuição `CHAVE=valor` por linha e **sem aspas**, porque a leitura feita pelo systemd não é um shell. Variáveis comuns:

- `AI_MEMORY_AUTH_TOKEN`: gerado com o comando `ai-memory generate-auth-token`.
- `AI_MEMORY_ALLOWED_HOSTS`: lista de nomes de host aceitos.
- `AI_MEMORY_LLM_PROVIDER` e a chave do provedor escolhido (por exemplo `ANTHROPIC_API_KEY` ou `OPENAI_API_KEY`). Valores aceitos de provedor: anthropic, openai, openai-oauth, anthropic-oauth, copilot, gemini, openai-compat e opencode.
- `AI_MEMORY_LLM_REASONING_EFFORT`: esforço de raciocínio do modelo.
- `AI_MEMORY_LLM_HEADERS`: cabeçalhos extras em toda chamada ao LLM, no formato `Nome=Valor` separados por vírgula.
- `AI_MEMORY_EMBEDDING_PROVIDER`: provedor de embeddings.

---

## 5. Deploy

### 5.1 Serviço no Linux

Há duas unidades de serviço, a de sistema e a de usuário.

Unidade de sistema:

- Roda com o usuário e grupo `ai-memory`.
- Lê as variáveis de `/etc/ai-memory/env` (opcional: se o arquivo faltar, o serviço sobe sem ele).
- Usa a configuração `/etc/ai-memory/config.toml` e o diretório de dados `/var/lib/ai-memory`, criado pelo systemd com modo 0750.
- Comando: `serve` em transporte HTTP, com interface web.
- Reinicia em caso de falha, após 5 s.
- Endurecimento: sem ganho de privilégios, diretório temporário privado, diretório pessoal inacessível, sistema de arquivos somente leitura exceto o diretório de dados.

Unidade de usuário:

- Roda na sessão do usuário, sem endurecimento de diretórios de sistema.
- Lê `~/.config/ai-memory/env`, usa `~/.config/ai-memory/config.toml` e `~/.local/share/ai-memory`.
- Instalada no alvo padrão de usuário.

Os pacotes também trazem configuração para criar o usuário de serviço e os diretórios (sysusers e tmpfiles). Há um arquivo de serviço equivalente para macOS (launchd), não detalhado aqui. No Windows nativo não há systemd: use o empacotamento de serviço descrito em [suporte a Windows](windows.md), cenário de servidor como serviço nativo.

### 5.2 Pacotes para Arch Linux (AUR)

Há dois pacotes:

- Pacote que compila a tag com a toolchain local.
- Pacote com o binário pré-compilado publicado nas releases, para x86_64 e aarch64.

Cada pacote precisa de um repositório git próprio no AUR, com o arquivo de metadados `.SRCINFO` gerado pelo `makepkg --printsrcinfo`.

O pacote pré-compilado tem versão e somas de verificação como marcadores. A automação de release reescreve esses campos a partir da tag, por isso editar à mão não tem efeito. Os dois pacotes conflitam entre si. O pré-compilado fornece o mesmo nome do pacote compilado e conflita também com a variante de depuração.

Arquivos instalados:

- Binário em `/usr/bin`.
- Documentação e licença em `/usr/share`.
- Scripts de hook em `/usr/share/ai-memory/hooks`.
- Configuração padrão em `/etc/ai-memory/config.toml` (modo 0644).
- Arquivo de ambiente em `/etc/ai-memory/env` (modo 0640, porque guarda segredos).
- Unidades de serviço de sistema e de usuário, configuração de sysusers e de tmpfiles.

Os dois arquivos de configuração estão na lista de backup do pacote, para que atualizações não os sobrescrevam.

### 5.3 Deploy para homelab com script

O script de deploy constrói a imagem na máquina local, envia a imagem para um registry e atualiza o servidor remoto por SSH.

Pré-requisitos:

- Docker na máquina local e acesso SSH ao servidor, com Docker Compose instalado lá.
- Um arquivo de configuração `deploy.env`, criado a partir do modelo, com:
  - `SERVER`: alvo SSH (obrigatório).
  - `DEPLOY_DIR`: diretório remoto onde estão o arquivo compose e o `.env.production` (obrigatório).
  - `IMAGE`: tag da imagem (obrigatória).
  - `CONTAINER_NAME`: nome do contêiner (opcional; padrão `ai-memory`).
- Sem o arquivo de configuração, o script sai com código 64 e uma mensagem de instrução.

Proteção contra sobrescrita: se a tag em `IMAGE` já apontar para um manifesto multiarquitetura (publicado pelo CI), o script sai com código 65 sem construir nada. Motivo: a imagem construída vale só para a arquitetura da máquina local, e sobrescrever o manifesto quebraria os outros hosts com erro de formato de executável. Por isso a tag deve ser própria, por exemplo `:homelab`, nunca `:latest` ou uma versão.

Passos, na ordem:

1. Constrói a imagem com a receita da seção 4.1, tagueando com `IMAGE`.
2. Envia a imagem ao registry.
3. Conecta ao servidor por SSH, ignorando a configuração SSH pessoal do usuário e aceitando uma chave de host nova no primeiro contato. No diretório `DEPLOY_DIR`, executa `docker compose pull`, `docker compose down` e `docker compose up -d`. Entre o `down` e o `up`, o serviço fica indisponível.
4. Espera 3 s e imprime o estado e o estado de saúde do contêiner.
5. Imprime uma dica para testar o endpoint MCP na porta 49374 do servidor.

O script não altera o arquivo compose do servidor. Esse arquivo deve apontar para a mesma tag em `IMAGE`. Para rodar a imagem publicada sem construir nada, basta executar `docker compose pull` e `docker compose up -d` no servidor.

### 5.4 Wrapper de linha de comando

O wrapper roda o comando `ai-memory` dentro do contêiner da imagem publicada, com os montes necessários:

- Diretório pessoal do usuário, montado no mesmo caminho dentro do contêiner, para que os comandos de instalação editem as configurações reais de cada agente.
- Projeto atual, montado em `/work`, para o comando de bootstrap.
- Volume de dados em `/data`: volume nomeado `ai-memory-data` por padrão, ou um diretório do host se `AI_MEMORY_DATA_DIR` for definido.

Motor de contêiner: `AI_MEMORY_DOCKER` quando definida; senão `docker`; senão `podman`.

Variáveis de ambiente do wrapper:

| Variável | Efeito |
|---|---|
| `AI_MEMORY_IMAGE` | Imagem usada. Padrão: a imagem publicada, tag `latest`. |
| `AI_MEMORY_DOCKER` | Comando do motor de contêiner. |
| `AI_MEMORY_DATA_VOLUME` | Nome do volume de dados. Padrão: `ai-memory-data`. |
| `AI_MEMORY_DATA_DIR` | Diretório do host montado em `/data`, no lugar do volume. |
| `AI_MEMORY_SERVER_URL` | URL do servidor para comandos cliente. |
| `AI_MEMORY_NO_TTY` | Força modo não interativo. |
| `AI_MEMORY_NO_VERSION_CHECK` | Desativa a checagem de versão, feita no máximo uma vez por dia. |
| `AI_MEMORY_NATIVE_BIN` | Caminho de um binário nativo já instalado, usado no lugar do baixado. |
| `AI_MEMORY_WRAPPER_URL` e `AI_MEMORY_WRAPPER_SHA256_URL` | Origem alternativa da atualização do próprio wrapper. O checksum é obrigatório. |
| `AI_MEMORY_SKIP_SELF_UPGRADE` | Pula a atualização do próprio wrapper no `upgrade`. |
| `CLAUDE_CONFIG_DIR` | Diretório de configuração do Claude Code, repassado ao contêiner. |
| Tokens de provedores (`COPILOT_GITHUB_TOKEN`, `ANTHROPIC_OAUTH_TOKEN`, `CLAUDE_CODE_OAUTH_TOKEN`) | Repassados aos comandos auxiliares. |

Comandos que executam o agente no host usam um cliente nativo, baixado das releases e verificado por checksum publicado, guardado no diretório de dados do usuário. Esses comandos são `run`, `show`, `continue`, `resume`, `workstreams`, `rename-workstream` e `mcp-bridge`, porque um contêiner não consegue executar o agente do host nem enxergar o checkout real. O mesmo cliente nativo atende `install-hooks`, a menos que `AI_MEMORY_HOOK_PLATFORM` seja `posix` ou `windows`, caso em que o comando usa o pacote de scripts do contêiner e perde os controles de privacidade de captura do cliente nativo. O cliente é baixado na primeira vez e baixado de novo quando o checksum remoto difere do local.

O comando `upgrade`, exclusivo do wrapper:

1. Atualiza o próprio wrapper, exigindo checksum publicado. Sem checksum, recusa a atualização.
2. Puxa a imagem configurada.
3. Atualiza o cliente nativo. Se isso falhar, para antes de atualizar os hooks.
4. Para cada agente com scripts no diretório de hooks do usuário (`~/.local/share/ai-memory/hooks`), executa a reinstalação dos hooks com `install-hooks --apply`. O diretório compartilhado de biblioteca é ignorado.
5. Se houver um contêiner local chamado `ai-memory` em execução, imprime instruções para recriá-lo com a imagem nova. Não recria sozinho. Quando não consegue inspecionar o contêiner, orienta a recriação manual.
6. Se `AI_MEMORY_SERVER_URL` apontar para um host remoto, avisa que a atualização não tocou nesse servidor e sugere o deploy da seção 5.3 ou o pull no servidor.

No Windows, o wrapper equivalente monta o diretório pessoal em outro caminho do contêiner, e as configurações de hook que ele gera apontam para os scripts PowerShell instalados no host. Detalhes em [suporte a Windows](windows.md).

---

## 6. Testes de referência

Os testes de shell e os de integração que cobrem este documento são:

- Testes unitários da biblioteca POSIX, com cliente HTTP simulado: casos do Antigravity no início de sessão, captura externa (sem POST e sem spool), Grok (primeira entrega, chamadas seguintes e subagente) e handoff de início de sessão.
- Smoke de ponta a ponta do handoff.
- Testes de atualização do wrapper.
- Aceitação do `ai-memory run` com workstreams gerenciados.
- Verificação de empacotamento Nix em CI: o binário existe, responde a `--version`, o diretório de hooks existe e não está vazio, a configuração padrão existe e a execução por flake funciona. Não altera o host nem exige systemd.
- Smoke privilegiado do módulo NixOS em contêiner com systemd como processo 1: o serviço fica ativo, o endpoint de saúde responde dentro do contêiner, um marcador em `/var/lib/ai-memory` sobrevive a reinício do serviço e, por padrão, sobrevive também à recriação com volume nomeado. Fora do escopo: matriz de upgrade entre versões e cobertura exaustiva das opções do módulo.
- Teste manual, fora do CI, do caminho Arch: contêiner descartável de Arch, instalação das dependências e da toolchain, construção da árvore atual, instalação do layout nativo, validação das unidades, criação do usuário de serviço, início do serviço com systemd real, teste do perfil de usuário e escrita das configurações de agentes.
- Verificação de changelog: falha se alguma seção de versão repete um título de seção, e falha se a mudança edita uma seção já publicada em relação à base.
