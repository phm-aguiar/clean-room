# Windows nativo: complementos específicos

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

> Complemento de [windows.md](windows.md). Este documento descreve só o que muda quando o ai-memory roda no Windows nativo: o wrapper de linha de comando, os scripts de hook em PowerShell, o spool e o dreno de processo, os caminhos de configuração e de dados, e as regras de caminho. Instalação, cenários de uso, serviço e suporte estão em [windows.md](windows.md). O contrato geral dos scripts de hook (sequência, spool, variante POSIX) está em [hook-scripts-and-container-deploy.md](hook-scripts-and-container-deploy.md).

## 1. Resumo das diferenças

| Área | O que muda no Windows | Motivo |
|---|---|---|
| Troca atômica de arquivo da wiki | Repete a troca em caso de bloqueio transitório | Antivírus e indexadores seguram o arquivo recém-criado |
| Git da wiki | Usa o programa `git` como fallback quando a biblioteca de Git falha | Erro de resolução de caminho na biblioteca de Git |
| Dreno do spool | Processo destacado com flags do Windows e tentativa de sair do grupo de jobs do agente | O dreno precisa sobreviver ao fim do processo do agente |
| Resolução de executável | Aplica as extensões do PATHEXT e nunca escolhe o arquivo sem extensão | Um arquivo sem extensão (shim de gerenciador de pacotes) não é executável no Windows |
| Comando de hook instalado | Forma nativa, sem shell, como padrão; PowerShell e Bash como alternativas | Evita um processo intermediário por evento |
| Wrapper | Encaminha ao contêiner pelo Docker Desktop, montando a pasta pessoal e a pasta atual | O contêiner Linux não enxerga o host |
| Caminhos | Diferenças de separador e de caixa em unidades e caminhos UNC | A mesma pasta não pode virar dois projetos |
| Permissões | Modos Unix (0700 e 0600) não são aplicados | O Windows não usa bits de permissão Unix |

## 2. Wrapper de linha de comando

Há dois wrappers com o nome `ai-memory`: um em PowerShell e um em cmd. O de cmd apenas chama o de PowerShell, na mesma pasta, com política de execução liberada, repassando os argumentos e o código de saída. Ambos são para uso com Docker Desktop e não substituem o binário nativo.

### 2.1 Fluxo

1. Lê três variáveis do host: `AI_MEMORY_DOCKER` (padrão `docker`), `AI_MEMORY_IMAGE` (padrão: a imagem oficial, tag `latest`) e `AI_MEMORY_DATA_VOLUME` (padrão `ai-memory-data`).
2. Se o comando do Docker não é encontrado, mostra erro citando `AI_MEMORY_DOCKER` e sai com código 127.
3. Se o primeiro argumento é `upgrade`, faz o pull da imagem e sai com o código do Docker. Nenhum outro passo roda.
4. Caso contrário, monta `docker run --rm -i` com as montagens (2.3) e as variáveis (2.4). Adiciona `-t` só se `AI_MEMORY_NO_TTY` não estiver definida e nem a entrada nem a saída estiverem redirecionadas.
5. Executa a imagem com os argumentos e sai com o código do contêiner.

### 2.2 Subcomando real

Opções globais podem vir antes do subcomando:

- `--config` e `--data-dir` consomem o argumento seguinte como valor.
- Qualquer outro argumento que comece com `--` é pulado sozinho.
- O primeiro argumento restante é o subcomando. Opções curtas (um hífen) não são puladas. Sem nenhum, o subcomando é vazio.

O subcomando só decide se a URL do servidor é definida (2.4).

### 2.3 Montagens e pasta de trabalho

- A pasta pessoal do usuário é montada em `/host-home`, e a variável `HOME` do contêiner aponta para ela.
- A pasta atual é montada em `/work` e é o diretório de trabalho.
- Escopo do projeto (o que o contêiner trata como projeto):
  - Se a pasta atual é a pasta pessoal ou está dentro dela, o escopo é o caminho relativo dentro de `/host-home`. Não há montagem extra.
  - Caso contrário, o escopo é a raiz do repositório Git da pasta atual, detectada pelo programa `git`. Sem `git`, ou se a pasta não é repositório, o escopo é a própria pasta atual. A raiz é montada em `/scope` como somente leitura.
  - A detecção nunca interrompe o wrapper, nem quando a pasta não é repositório.
- A comparação de caminhos ignora diferenças de caixa.
- Dados: se `AI_MEMORY_DATA_DIR` aponta para uma pasta existente no host, ela é montada em `/data`. Caso contrário, usa o volume nomeado de `AI_MEMORY_DATA_VOLUME`.
- Pasta de hooks no host: `$HOME/.local/share/ai-memory/hooks`, passada ao contêiner em `AI_MEMORY_HOOKS_HOST_ROOT`.

### 2.4 Variáveis e URL do servidor

- As variáveis do host abaixo são repassadas ao contêiner somente se não estiverem vazias: `AI_MEMORY_SERVER_URL`, `AI_MEMORY_AUTH_TOKEN`, as de LLM e de embeddings (`AI_MEMORY_LLM_*`, `AI_MEMORY_EMBEDDING_*`), `AI_MEMORY_COPILOT_CLIENT_ID`, `AI_MEMORY_ALLOWED_HOSTS`, `AI_MEMORY_WORKSTREAM_ID`, `CLAUDE_CONFIG_DIR`, `CLAUDE_CODE_SESSION_ID`, `RUST_LOG` e as chaves de provedores listadas no wrapper.
- `AI_MEMORY_EMBEDDING_QUERY_PREFIX` e `AI_MEMORY_EMBEDDING_DOCUMENT_PREFIX` são repassadas se estiverem definidas, mesmo vazias. Vazia significa apagar o prefixo do arquivo de configuração.
- O contêiner também recebe `AI_MEMORY_HOST_CWD` (pasta do host), `AI_MEMORY_SCOPE_CWD` (pasta dentro do escopo), `AI_MEMORY_DATA_DIR=/data`, `AI_MEMORY_HOOK_PLATFORM=windows` e `AI_MEMORY_HOOKS_HOST_ROOT`. Com a plataforma `windows`, o contêiner gera hooks em PowerShell que apontam para os scripts da pasta de hooks do host.
- URL do servidor: se `AI_MEMORY_SERVER_URL` não está definida e o subcomando não é `install-mcp`, `install-hooks` ou `setup-agent`, o contêiner recebe `http://host.docker.internal:49374`. Esses três comandos gravam configuração no host, onde esse nome não resolve. Para eles a URL não é definida, e a CLI grava o padrão do host, `http://127.0.0.1:49374`.
- `AI_MEMORY_NO_TTY` desliga só a alocação de terminal; a entrada padrão continua aberta.

## 3. Scripts de hook em PowerShell

### 3.1 Organização

- Os scripts ficam em uma pasta de scripts de hook do repositório, com uma subpasta por agente (13 agentes) e uma subpasta com a biblioteca comum. São 89 scripts de evento. Os nomes de evento seguem o contrato geral.
- Cada script é fino: carrega a biblioteca comum, chama uma função com o evento e o agente, com opções como buscar handoff, formato de saída do agente ou briefing de entrega única, e termina com código 0.
- A sequência e as respostas são as do contrato geral. A diferença está na implementação, descrita abaixo.

### 3.2 O que a função faz

Ordem de execução (detalhes no contrato geral):

1. Lê o payload da entrada padrão, só se ela estiver redirecionada.
2. Descobre a pasta de trabalho no payload, testando nomes de chave alternativos usados por diferentes agentes. No Devin, usa a variável de projeto e, por último, a pasta atual.
3. Se a pasta cai em repositório com perfil de servidor (chave `server` no marcador), não envia nada e, no Antigravity, responde `{}`. A variante PowerShell não roteia perfis, e só o binário nativo faz isso.
4. Monta a query: procura o marcador `.ai-memory.toml` subindo a partir da pasta, até a raiz do repositório ou a pasta pessoal; obtém a identidade do remoto Git (primeiro `upstream`, depois `origin`); aplica a estratégia de projeto e as flags de perfil. Se a query não pode ser montada (rota inválida), não envia nada e responde `{}` no Antigravity e no Grok.
5. Envia POST para `AI_MEMORY_HOOK_URL` ou, se não definida, `http://127.0.0.1:49374`, com `Authorization` quando `AI_MEMORY_AUTH_TOKEN` está definida. Timeout de 3 s.
   - 2xx: dispara o dreno destacado.
   - 4xx, exceto 408, 425 e 429: descartado, por ser rejeição permanente.
   - Sem resposta, código abaixo de 400, 5xx, 408, 425 ou 429: vai para o spool.
6. Se há handoff a buscar, faz GET com timeout de 2 s e imprime no formato do agente.
7. Drena o spool, com no máximo 64 entradas por passagem.

### 3.3 Limites da variante PowerShell

- Não aplica a política de captura v1: não há `ignore_paths`, lista de metadados permitidos nem marcador de protocolo `_ai_memory_capture`. Esses controles são do binário nativo. A biblioteca de shell também não os aplica.
- No Claude Code, o evento de parada que traz a mensagem do assistente é descartado inteiro, porque o script não sanitiza esse campo.
- Não roteia perfis de servidor (item 3 da seção 3.2).

## 4. Comando de hook instalado no Windows

O comando que o agente executa depende de um valor de plataforma. O valor vem de `AI_MEMORY_HOOK_PLATFORM` (sem diferença de caixa) ou, se ausente, do padrão da rota de geração.

| Valor | Forma gravada | Uso |
|---|---|---|
| `windows-native` | Chama o binário com argumentos, sem shell | Padrão da instalação no Windows |
| `windows` | `powershell.exe` com comando codificado, sem perfil e com política de execução liberada, executando o script de hook | Alternativa sem binário local; usada pelo wrapper |
| `windows-bash` | `bash -c` chamando o script POSIX, com o caminho convertido para o formato do Git Bash (`C:\x` vira `/c/x`) | Padrão de rotas que geram scripts para copiar, como setup-agent e os snippets de host |
| `posix`, `unix` | Script POSIX | Escolha explícita |
| `posix-native` | Chama o binário | Padrão em Linux e macOS |

Valores desconhecidos são ignorados, e vale o padrão da rota.

### 4.1 Forma nativa do Claude Code

- Na instalação, o Claude Code recebe o executável e uma lista de argumentos separados, sem shell: `--data-dir` (se configurado), `hook`, `--event` com o nome do script, `--agent claude-code`, `--server-url`, e, se houver, `--auth-token` e `--project-strategy`. Quando há token, ele fica nos argumentos gravados na configuração.
- Só a instalação para Claude Code usa essa forma. Setup-agent e os snippets de Docker mantêm a forma em string.
- Nos demais agentes, a forma em Windows é uma string com o binário entre aspas duplas. Exceção: o Antigravity recebe a forma sem aspas, porque o executor encapsula o comando em cmd e não remove aspas internas.
- O executável é o caminho do binário em execução. Hooks antigos gravados com prefixo de caminho longo (`\\?\`) são lidos sem esse prefixo.
- O código documenta que a forma nativa é cerca de 3,5 vezes mais rápida por hook que a forma Bash. Para o uso e a verificação, ver [windows.md](windows.md).

## 5. Spool e dreno no Windows

O formato do arquivo de spool, a drenagem e a regra de entrega estão no contrato geral ([hook-scripts-and-container-deploy.md](hook-scripts-and-container-deploy.md), seções 2.5 e 3). Aqui ficam só as diferenças de Windows.

### 5.1 Pasta do spool

- Variante PowerShell: pasta `hook-spool` dentro da pasta de estado, escolhida nesta ordem: `AI_MEMORY_DATA_DIR`; `XDG_DATA_HOME/ai-memory`; `%LOCALAPPDATA%\ai-memory`; `$HOME/.local/share/ai-memory`; e, por último, `.ai-memory` relativa à pasta atual.
- Binário nativo: pasta `hook-spool` dentro da pasta de dados, escolhida nesta ordem: `--data-dir`; `AI_MEMORY_DATA_DIR`; `%LOCALAPPDATA%\ai-memory`.
- Divergência: a variante PowerShell consulta `XDG_DATA_HOME` antes de `LOCALAPPDATA`, e o binário nativo não consulta essa variável. Se ela estiver definida no Windows, os dois podem usar pastas diferentes. A confirmar se a diferença é intencional.

### 5.2 Dreno destacado

- Depois de um POST com 2xx, o agente não espera pelo dreno. O dreno roda em processo separado, sem janela, com entrada e saída nulas. O erro vai para um arquivo de log no diretório de dados.
- Binário nativo no Windows: as flags de criação são `DETACHED_PROCESS` (0x00000008), `CREATE_NEW_PROCESS_GROUP` (0x00000200) e `CREATE_NO_WINDOW` (0x08000000). Primeiro tenta também `CREATE_BREAKAWAY_FROM_JOB` (0x01000000), para sair do grupo de jobs do agente. Se o lançamento falha com acesso negado (código 5), registra a tentativa no log e relança sem o breakaway.
- Variante PowerShell: não usa mecanismo de processo em segundo plano do sistema. Roda o próprio script em modo de dreno, em processo filho com saída redirecionada e sem janela.
- Trava de drenagem: a trava ocupada é reconhecida pelo erro de bloqueio (WouldBlock) e também pelo código nativo 33 (violação de bloqueio), que o Windows pode devolver no lugar do primeiro.
- Unix: nova sessão de processo, usando o utilitário `setsid` quando existe em `/usr/bin` ou `/bin`.

### 5.3 Permissões e segredos

- Em Unix, o diretório do spool é criado com 0700 e os arquivos com 0600.
- No Windows, a variante PowerShell não aplica modo (o chmod é ignorado). A proteção fica por conta do perfil do usuário, porque a pasta de dados padrão está no perfil. O binário nativo não define ACL explícita no código (a confirmar).
- Quando `AI_MEMORY_AUTH_TOKEN` está definida, a variante PowerShell grava o token em texto dentro do arquivo do spool.

## 6. Escrita da wiki e Git no Windows

### 6.1 Troca atômica de arquivo

- Ao trocar o arquivo temporário pelo definitivo, erros de compartilhamento (32) e de bloqueio (33) são tratados como transitórios. Essas falhas são repetidas até 5 vezes, com espera linear de 10, 20, 30, 40 e 50 ms. O pior caso soma 150 ms antes de o erro real ser devolvido.
- Os demais erros, inclusive acesso negado (5), propagam na hora.
- Fora do Windows, nenhum erro é tratado como transitório.
- A falha devolve o arquivo temporário, então o conteúdo já preparado é reaproveitado em cada nova tentativa.

### 6.2 Git da wiki

- A operação usa primeiro a biblioteca de Git embutida. No Windows, se ela falha com erro de resolução de caminho do sistema (texto "failed to resolve path") ou com repositório não encontrado, tenta o programa `git` instalado, que precisa estar no PATH.
- Erro de dono do repositório (verificação de propriedade feita pela biblioteca) não faz fallback, porque o programa `git` recusaria do mesmo jeito. Esse erro é tratado como erro específico de dono e registrado em nível de erro na inicialização do serviço, com a instrução de rodar como o usuário dono do diretório.
- Fallback de commit: adiciona todos os arquivos; se o índice não tem mudanças, não cria commit; caso contrário, cria o commit com a identidade fixa `ai-memory <ai-memory@local>` e devolve o hash.
- Fallback de contagem: conta os commits de HEAD com o programa `git`.
- Causa do fallback: resolução de caminho da biblioteca de Git no Windows. Qual padrão de caminho dispara o erro: a confirmar.
- Fora do Windows, não há fallback. O erro original é devolvido.

### 6.3 Raiz do repositório principal (worktrees)

- A descoberta da raiz do repositório principal, usada na consolidação, tem fallback no Windows. Ele consulta o programa `git` para saber se o repositório é "bare" e qual é o diretório comum do Git; a raiz é o pai desse diretório.
- Fora do Windows, o caso sem resolução vira erro de "não é repositório".

## 7. Caminhos de configuração e de dados

| Item | Caminho padrão no Windows | Como trocar |
|---|---|---|
| Pasta de dados | `%LOCALAPPDATA%\ai-memory` | `--data-dir` ou `AI_MEMORY_DATA_DIR` |
| Arquivo de configuração | `config.toml` dentro da pasta de dados | `--config` |
| Marcador do projeto | `.ai-memory.toml`, procurado subindo a partir da pasta atual | Não se aplica |
| Pasta de hooks no host (wrapper) | `$HOME/.local/share/ai-memory/hooks` | Definida pelo próprio wrapper (`AI_MEMORY_HOOKS_HOST_ROOT`) |
| Dados do wrapper | Volume `ai-memory-data`, ou a pasta de `AI_MEMORY_DATA_DIR` se existir | `AI_MEMORY_DATA_VOLUME` ou `AI_MEMORY_DATA_DIR` |
| Claude Desktop (MCP) | `%APPDATA%\Claude\claude_desktop_config.json`; na versão da loja, `%LOCALAPPDATA%\Packages\Claude_<id>\LocalCache\Roaming\Claude\claude_desktop_config.json` | Ver [mcp-install.md](mcp-install.md) |
| Zed (MCP) | `%APPDATA%\Zed\settings.json` | Ver [mcp-install.md](mcp-install.md) |

- Com a versão da loja do Claude Desktop, o instalador prefere o pacote que já contém o arquivo de configuração (regra de desempate completa: a confirmar).
- O padrão de estado dos scripts PowerShell e o do binário nativo podem divergir (ver 5.1).

## 8. Regras de caminho em todas as plataformas

Estas regras não são exclusivas do Windows, mas existem por causa dele.

### 8.1 Pasta de trabalho em chaves de projeto

- Antes de comparar, `\` vira `/` e barras finais são removidas (a raiz `/` fica).
- Se o caminho começa com letra de unidade seguida de `:`, ou com `//` (caminho UNC), a chave de cache usa minúsculas ASCII. Assim, `C:\Repo` e `c:\repo` formam a mesma árvore.
- O nome do projeto vem do caminho original, com a caixa como foi digitada. Só a chave é normalizada. Isso impede que a mesma pasta vire dois projetos por causa da caixa.

### 8.2 Caminhos de página

- Rejeitados: caminho vazio, caminho que começa com `/`, prefixo de unidade `X:/`, `\`, segmento `.` ou `..`, e caminho absoluto.
- Portabilidade (vale em todas as plataformas):
  - Um segmento não pode terminar em ponto ou espaço, nem conter `< > : " | ? *`.
  - Um segmento não pode ser nome de dispositivo do DOS (`CON`, `PRN`, `AUX`, `NUL`, `COM1` a `COM9`, `LPT1` a `LPT9`). A comparação ignora caixa e usa só o nome antes do primeiro ponto.
  - Um segmento não pode ser `.git`, em qualquer caixa.
- Motivo: uma wiki criada em Linux precisa ser legível no Windows.

### 8.3 Redação de caminhos de credencial

- Caminhos de credenciais em estilo Windows são redigidos como os de estilo POSIX. Isso inclui prefixo de unidade opcional e separadores `\` ou `/`.
- Cobre os diretórios `.ssh`, `.aws`, `.kube`, `.config/gcloud` e `.gnupg`.

## 9. Execução de programas e harnesses

### 9.1 Resolução de executável

- Nome sem pasta (nome de comando): percorre as pastas do PATH. No Windows, cada candidato é testado com as extensões de PATHEXT, na ordem; se PATHEXT não estiver definida, usa `.COM;.EXE;.BAT;.CMD`. O arquivo sem extensão nunca é escolhido.
- Nome com extensão explícita: usado como está, se existir.
- Nome com pasta: resolvido com as mesmas regras.
- No Unix, o arquivo precisa existir e ser executável.
- Motivo: no Windows, um comando instalado por gerenciador de pacotes pode ter um arquivo sem extensão que o sistema não executa. A versão com extensão é a que funciona.

### 9.2 Nome de executável por harness

- Command Code: `cmdc` no Windows e `command-code` nos demais sistemas.

### 9.3 Caminhos de sessão no OpenCode

- Ao procurar sessões pela pasta, o Windows consulta duas formas do caminho: a nativa (com `\`) e a com `/`. Isso vale só para o OpenCode.

## 10. Pacotes, distribuição e requisitos

- Distribuição nativa: o binário de linha de comando (`ai-memory.exe`), extraído de uma release ou compilado. O cenário do binário pré-compilado está em [windows.md](windows.md).
- Não há instalador do Windows (MSI, winget, Scoop ou Chocolatey) nas pastas de empacotamento e de CI. Os pacotes de systemd, launchd, AUR, RPM, sysusers, tmpfiles e Nix não se aplicam ao Windows.
- Há um workflow de CI específico para Windows.
- Requisitos por modo:
  - Binário nativo: o próprio binário basta para a forma nativa do Claude Code. O programa `git` no PATH é necessário para os fallbacks da seção 6. PowerShell é necessário para as variantes `windows` e para o wrapper de cmd.
  - Wrapper: Docker Desktop, ou o comando definido em `AI_MEMORY_DOCKER`.
  - Modo `windows-bash`: bash do Git para Windows.
- Regenerar o CSS da interface web exige a variável de build de regeneração. Nesse modo, o build baixa a ferramenta de CSS para Windows e confere um checksum fixado. Sem a variável, o build usa o CSS versionado.

## 11. O que não é exclusivo do Windows

- Keepalive TCP do servidor: o intervalo vem de `tcp_keepalive_secs` (0 desliga). Vale em todas as plataformas.
- Regras de caminho da seção 8.
- Só Unix: modos 0700 e 0600, nova sessão do dreno com `setsid`.

## 12. Testes de referência

- A pasta de testes de hooks tem dois scripts de teste em PowerShell: um para o Grok e outro para o spool.
- Sem o privilégio para criar symlinks (erro 1314), o teste de symlink pula a asserção e emite um aviso.
- Nos testes, os limites de tempo são maiores no Windows: 8 s contra 2 s no limite do eval gate.
- O auxiliar de testes para PowerShell tenta `powershell.exe` e, se não existir, `pwsh.exe`.

## 13. O que este documento não descreve

1. Migrações: a cópia de referência tem 76 migrações, e a última é a 76. O resumo por domínio está em [database-schema.md](database-schema.md).
2. Renderização de hooks e de configuração por agente (formatos JSON, TOML e de plugin): ver [cli.md](cli.md).
3. Handlers administrativos e leitores de transcrição por harness: ver [harness-adapters.md](harness-adapters.md).
4. Textos integrais dos prompts: estão resumidos em [consolidation.md](consolidation.md).
5. Testes de integração: são a melhor especificação executável. Os limites de segurança estão em [security-boundaries.md](security-boundaries.md).
