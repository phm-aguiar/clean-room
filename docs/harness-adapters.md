# Adaptadores de harness gerenciados

> **Papel:** camada cliente do comando `ai-memory run`, responsável pela continuidade de trabalho entre harnesses. Ela não fala com o servidor nem com o armazenamento do ai-memory, e não altera nada que lê. Faz três coisas:
> 1. planeja o comando nativo de cada harness (criar ou retomar uma sessão) sem filtrar os argumentos do usuário;
> 2. identifica o checkout (repositório ou worktree) e registra um checkpoint do git sem mutar o repositório;
> 3. lê o transcript nativo de cada harness de forma incremental e o normaliza em eventos portáveis do workstream.
>
> O orquestrador do comando abre um lease no servidor, usa essas três partes, importa os eventos ao sair e finaliza a execução. O ciclo completo está em [Workstreams gerenciados](managed-workstreams.md).

Esta camada usa o binário `git` como processo filho e abre os bancos SQLite nativos de alguns harnesses somente para leitura.

## Harnesses suportados

| Harness | Nomes aceitos na linha de comando | Executável padrão | Tipo de agente registrado |
|---|---|---|---|
| Claude Code | `claude`, `claude-code` | `claude` | Claude Code |
| Codex | `codex` | `codex` | Codex |
| OpenCode | `opencode`, `open-code` | `opencode` | OpenCode |
| OpenCode 2 (beta) | `opencode2`, `opencode-v2`, `open-code2` | `opencode2` | OpenCode (mesmo tipo e mesmo armazenamento de sessões do OpenCode) |
| Pi | `pi` | `pi` | Pi |
| Crush | `crush` | `crush` | Crush |
| Oh My Pi | `omp`, `oh-my-pi` | `omp` | Oh My Pi |
| Kimi Code | `kimi`, `kimi-code`, `kimi-cli` | `kimi` | Kimi Code |
| Command Code | `command-code`, `commandcode`, `cmdc`, `cmd` | `command-code` (no Windows, `cmdc`) | Command Code |
| Kiro, motor v2 | `kiro`, `kiro-cli` | `kiro-cli` | Kiro CLI |
| Kiro, motor v3 | selecionado por argumento (ver abaixo) | `kiro-cli` | Kiro CLI |
| Grok Build | `grok`, `grok-build` | `grok` | Grok |
| Antigravity | `antigravity`, `antigravity-cli`, `agy` | `agy` | Antigravity CLI |
| GitHub Copilot CLI | `copilot`, `copilot-cli` | `copilot` | Copilot CLI |

- São **14** adaptadores. Kiro v3 não tem nome próprio na linha de comando: o nome estável (usado em mensagens e no relatório) é `kiro-v3`.
- O executável padrão é resolvido pelo `PATH`. O usuário pode informar outro executável.
- Capacidade de recuperação do armazenamento nativo: `Export` para todos, exceto o Antigravity (`SpoolOnly`: só prompts, e nenhum cursor de recuperação confiável).
- Kiro v2 e v3 compartilham o mesmo executável e são diferenciados pelos argumentos: `--v3`, `--mode` ou `--agent-engine v3` selecionam o motor v3. Os dois motores têm espaços de identificador de sessão separados e uma sessão de um não pode ser retomada pelo outro.
- Sem hook de fim de sessão: Command Code, Kiro (v2 e v3) e Antigravity. Nesses harnesses a sessão permanece aberta até a finalização explícita feita pelo usuário.

## 1. Planejamento do comando

### Regra central

- Os argumentos do usuário nunca são validados, reordenados ou removidos. Sua ordem e seus bytes são preservados.
- Um seletor de sessão só é acrescentado quando o lançamento é de sessão, o usuário não passou um seletor próprio e há uma sessão a criar ou a retomar.
- O plano de lançamento contém: programa, argumentos, identificador de sessão esperado (quando já é conhecido antes do lançamento), diretório de sessão e modo de lançamento.

### Modos de lançamento

- **Sessão:** invocação interativa ou persistida. Participa da continuidade.
- **Passthrough:** invocação utilitária ou efêmera. Os argumentos são repassados sem alteração e o checkpoint do repositório continua sendo registrado, mas não há vínculo de sessão nem exportação de transcrição.

Entram em passthrough:
- pedidos de versão ou ajuda (`--help`, `-h`, `--version`, `-v`; no Kiro, `-v` é verbose e a versão é `-V`, então `--help-all` também conta como ajuda). Uma flag casa por igualdade ou por prefixo `flag=`;
- `--no-session` e `--no-session-persistence`;
- subcomandos utilitários de cada harness (o **primeiro argumento** deve ser exatamente um destes; no Kiro, o primeiro argumento que não é flag, pulando `--agent <valor>` e `--resume-id <valor>`):
  - Claude Code: `agents`, `auth`, `auto-mode`, `doctor`, `install`, `mcp`, `plugin`, `plugins`, `project`, `setup-token`, `ultrareview`, `update`, `upgrade`;
  - Codex: `review`, `login`, `logout`, `mcp`, `plugin`, `mcp-server`, `app-server`, `remote-control`, `completion`, `update`, `doctor`, `sandbox`, `debug`, `apply`, `archive`, `delete`, `unarchive`, `cloud`, `exec-server`, `features`, `help`;
  - OpenCode: `completion`, `acp`, `mcp`, `attach`, `debug`, `providers`, `agent`, `upgrade`, `uninstall`, `serve`, `web`, `models`, `stats`, `export`, `import`, `github`, `pr`, `session`, `plugin`, `db` (`run` continua sendo sessão);
  - OpenCode 2: `upgrade`, `acp`, `api`, `debug`, `console`, `auth`, `mcp`, `plugin`, `models`, `stats`, `export`, `import`, `service`, `pair`, `serve` (`run` e `mini` continuam sendo sessão);
  - Pi: `install`, `remove`, `uninstall`, `update`, `list`, `config`;
  - Crush: `completion`, `dirs`, `help`, `login`, `logout`, `logs`, `models`, `projects`, `server`, `session`, `stats`, `update-providers` (`run` continua sendo sessão);
  - Oh My Pi: `acp`, `agents`, `auth-broker`, `auth-gateway`, `commit`, `config`, `grep`, `grievances`, `plugin`, `read`, `search`, `setup`, `shell`, `ssh`, `stats`, `update`, `worktree`;
  - Kimi Code: `export`, `provider`, `acp`, `web`, `server`, `login`, `doctor`, `vis`, `migrate`, `upgrade`, `update`, `__plugin_run_node`;
  - Command Code: `info`, `status`, `help`, `whoami`, `update`, `feedback`, `taste`, `learn-taste`, `mcp`, `skills`, `mods`, `login`, `logout`;
  - Kiro (v2 e v3): `debug`, `settings`, `setup`, `update`, `diagnostic`, `init`, `theme`, `issue`, `login`, `logout`, `whoami`, `profile`, `user`, `doctor`, `launch`, `quit`, `restart`, `integrations`, `translate`, `dashboard`, `mcp`, `inline`, `agent`, `acp`, `help` (todos os comandos de raiz exceto `chat`; chamadas sem comando ou só com flags abrem o chat e são sessão);
  - Grok: `agent`, `completions`, `dashboard`, `doctor`, `export`, `help`, `inspect`, `leader`, `login`, `logout`, `mcp`, `memory`, `models`, `plugin`, `sessions`, `setup`, `trace`, `update`, `version`, `v`, `worktree`, `wrap`;
  - Antigravity: `agent`, `agents`, `changelog`, `help`, `install`, `models`, `plugin`, `plugins`, `update`;
  - Copilot CLI: `app`, `completion`, `config`, `help`, `init`, `instruction`, `login`, `lsp`, `mcp`, `memories`, `plugin`, `sandbox`, `sessions`, `skill`, `update`, `version`, `workflow`;
- no Codex: `exec review` e `exec help`;
- no Command Code: `--list-models` e `--ide-setup`;
- no Kiro v2: seleção de motor não padrão (`--v3`, `--mode` ou `--agent-engine` com valor diferente de `v2`), e também `--no-interactive`, `--list-sessions`, `-l`, `--list-models`, `--delete-session` e `-d`. Um valor desconhecido de `--agent-engine` não é atribuído a nenhum adaptador.

### Validação de identificadores

- Um identificador nativo (explícito ou vinculado) é validado antes do lançamento. Se for inválido, o planejamento falha com erro.
- Regras: não vazio após remover espaços; até 512 bytes; sem caracteres de controle; sem caracteres de formatação invisíveis; sem marcadores de redação; e sem alteração pelo filtro de privacidade. Além disso, não pode começar com hífen, não pode ser `.` nem `..`, e não pode conter barra ou barra invertida.
- No Command Code e no Copilot CLI (que também resolvem nome de sessão ou prefixo de id), um identificador explícito que não seja UUID é descartado do plano (sem erro): só um UUID exato identifica a sessão antes do lançamento.
- **Identificador explícito do usuário** (lido só para o plano, nunca altera os argumentos): Claude Code `--resume`/`-r`/`--session-id`; Codex o argumento depois de `resume` (ou `exec resume`) que não começa com `-`; OpenCode/OpenCode 2 `--session`/`-s`; Pi `--session`/`--session-id`; Crush `--session`/`-s`; Oh My Pi `--resume`/`-r`; Kimi `--session`/`-S`/`--resume`/`-r`; Command Code `--session`/`--resume`/`-r`; Kiro `--resume-id`; Grok `--resume`/`-r`/`--session-id`/`-s`; Antigravity `--conversation`; Copilot `--resume`/`-r`/`--session-id`. O valor pode vir como `--flag valor` (o valor não pode começar com `-`) ou `--flag=valor` (não vazio). Um seletor sem valor (por exemplo `--resume` sozinho, que abre o seletor nativo) não dá identificador.

### Diretório de sessão

Ordem de resolução:
1. flag explícita: `--session-dir` (Pi e Oh My Pi) ou `--data-dir` / `-D` (Crush);
2. variável de ambiente do harness;
3. para o Crush, o diretório de dados derivado do checkout e da home.

Variáveis de ambiente consultadas (valor vazio ou só com espaços é tratado como ausente):

| Harness | Variável | Diretório resultante |
|---|---|---|
| Claude Code | `CLAUDE_CONFIG_DIR` | subpasta `projects` |
| Codex | `CODEX_HOME` | subpasta `sessions` |
| OpenCode e OpenCode 2 | `XDG_DATA_HOME` | subpasta `opencode` |
| Pi | `PI_CODING_AGENT_SESSION_DIR`, depois `PI_CODING_AGENT_DIR` | o próprio valor, ou subpasta `sessions` |
| Oh My Pi | `PI_CODING_AGENT_SESSION_DIR`; depois o perfil nomeado (quando há home) ou `PI_CODING_AGENT_DIR` | valor direto, ou subpasta `sessions` |
| Kimi Code | `KIMI_CODE_HOME` | subpasta `sessions` |
| Kiro v2 | `KIRO_HOME` | subpasta `sessions/cli` |
| Kiro v3 | `KIRO_HOME` | subpasta `sessions` |
| Grok | `GROK_HOME` | subpasta `sessions` |

| Copilot CLI | `COPILOT_HOME` | subpasta `session-state` |

Command Code, Crush e Antigravity não têm variável de ambiente de raiz de sessão. No Oh My Pi, um perfil nomeado inválido ou sem home resolvível não produz diretório extra.

**Raízes padrão** (usadas quando nada acima se aplica; relativas à home nativa): Claude Code `.claude/projects`; Codex `.codex/sessions`; OpenCode e OpenCode 2 `.local/share/opencode` (o banco `opencode.db`, compartilhado pelos dois); Pi `.pi/agent/sessions`; Crush `.crush` (arquivo `crush.db`); Oh My Pi `.omp/agent/sessions`; Kimi Code `.kimi-code/sessions`; Command Code `.commandcode/projects`; Kiro v2 `.kiro/sessions/cli`; Kiro v3 `.kiro/sessions`; Grok `.grok/sessions`; Antigravity `.gemini/antigravity-cli/conversations`; Copilot `.copilot/session-state`. Um valor vazio ou só com espaços nas variáveis de raiz conta como ausente.

**Oh My Pi, perfis.** O perfil vem de `--profile` (só se aparecer **no início** da linha, depois de um `launch` ou `acp` opcional, e sem outro `--profile` mais adiante), senão de `OMP_PROFILE` (vale mesmo vazio), senão do legado `PI_PROFILE`. O nome é aparado; vazio ou `default` seleciona o perfil padrão; senão precisa casar `^[a-z0-9][a-z0-9._-]{0,63}$`, não pode ser `.` nem `..`, terminar em `.` ou ser nome reservado do Windows (`CON`, `PRN`, `AUX`, `NUL`, `COM0-9`, `LPT0-9`, também com extensão), com a recusa `Invalid OMP profile "<nome>". Profile names must match ^[a-z0-9][a-z0-9._-]{0,63}$, …`. A raiz de configuração é `<home>/.omp`, ou `<home>/<PI_CONFIG_DIR>` (juntado como no Node: `..` sobe, caminho absoluto continua sob a home). Perfil nomeado usa `<raiz>/profiles/<nome>/agent` e ignora `PI_CODING_AGENT_DIR`; o padrão usa `PI_CODING_AGENT_DIR` não vazio (descartado se for o diretório que um OMP pai derivou para o perfil dele) ou `<raiz>/agent`. As sessões ficam em `<agente>/sessions`, exceto em Linux e macOS quando o diretório do agente está no local padrão e `XDG_DATA_HOME/omp` (perfil: `XDG_DATA_HOME/omp/profiles/<nome>`) já existe: então `<isso>/sessions`.

**Crush, diretório de dados.** Sem `--data-dir`/`-D`, a pasta do `crush.db` é: o último `options.data_directory` entre os JSON de configuração do Crush na ordem de mesclagem (arquivo do sistema `/etc/crush/crush.json` fora do Windows; configuração global `$CRUSH_GLOBAL_CONFIG/crush.json`, senão `$XDG_CONFIG_HOME/crush/crush.json`, senão `~/.config/crush/crush.json`; dados globais `$CRUSH_GLOBAL_DATA/crush.json`, senão `$XDG_DATA_HOME/crush/crush.json`, senão `%LOCALAPPDATA%\crush\crush.json` no Windows ou `~/.local/share/crush/crush.json`; depois `crush.json` e `.crush.json` de cada diretório da busca, do topo para baixo até o diretório atual), senão o `.crush` mais próximo subindo do diretório atual até a raiz do worktree git (não vale um `.crush` direto na home, e a busca para num item de outro dono), senão `<diretório atual>/.crush`. Valor relativo conta a partir do diretório atual. Um `crushrc` não é lido (exigiria executar o script do usuário): quem o usa deve passar `--data-dir`.

### Seletores por harness

Quando há identificador vinculado (retomada), ou quando o harness aceita escolher o identificador de uma sessão nova, o planejador injeta o seletor abaixo.

| Harness | Nova sessão | Retomada (identificador vinculado) | Posição do seletor |
|---|---|---|---|
| Claude Code | `--session-id <UUID gerado>` | `--resume <id>` | no fim |
| Codex | nenhum; identificador descoberto após a saída | `resume <id>`; se a invocação for `exec`, `exec resume <id>` | no início |
| OpenCode e OpenCode 2 | nenhum | `--session <id>`; se o primeiro argumento for `run`, logo após `run`; senão, no início | depende do primeiro argumento |
| Pi | `--session-id <UUID gerado>` | `--session <id>` | no fim |
| Crush | nenhum; vinculado pelo que o próprio lançamento criou | `--session <id>` | no início |
| Oh My Pi | nenhum | `--resume=<id>` | no fim |
| Kimi Code | nenhum (não aceita identificador escolhido); vinculado por hook ou descoberto após a saída | `--session <id>` | no fim |
| Command Code | nenhum; identificador lido do cabeçalho do transcript após a saída | `--session <id>` | no fim |
| Kiro v2 e v3 | nenhum; descoberto após a saída | `--resume-id <id>` | no fim |
| Grok | `--session-id <UUID gerado>` | `--resume <id>` | no fim |
| Antigravity | nenhum; vinculado por hook ou descoberto no armazenamento após a saída | `--conversation <id>` | no fim |
| Copilot CLI | `--session-id <UUID gerado>` | `--resume=<id>` (forma com `=`, para a opção de valor opcional não consumir o argumento seguinte e para uma sessão ausente falhar em vez de ser recriada) | no fim |

- Em lançamento de sessão do Kiro v3 sem seleção explícita do motor v3, o argumento `--v3` é inserido no início.
- Um seletor próprio do usuário impede a injeção de qualquer seletor do adaptador. Os que contam como seletor próprio, por harness (cada um casa por igualdade ou por `flag=`): Claude Code `--resume`, `-r`, `--continue`, `-c`, `--session-id`; Codex primeiro argumento `resume` ou `fork`, ou `exec resume`; OpenCode e OpenCode 2 `--session`, `-s`, `--continue`, `-c`, `--fork`; Pi `--session`, `--session-id`, `--continue`, `-c`, `--resume`, `-r`, `--fork`; Crush `--session`, `-s`, `--continue`, `-C`; Oh My Pi `--resume`, `-r`, `--continue`, `-c`; Kimi `--session`, `-S`, `--resume`, `-r`, `--continue`, `-c`, `-C`; Command Code `--session`, `--resume`, `--sessions`, `-r`, `--continue`, `-c`, `--fork-session`; Kiro `--resume`, `-r`, `--resume-id`, `--resume-picker`, `--list`; Grok `--resume`, `-r`, `--continue`, `-c`, `--session-id`, `-s`, `--fork-session`; Antigravity `--conversation`, `--continue`, `-c`; Copilot `--resume`, `-r`, `--continue`, `--session-id`, `--connect`. Um seletor sem valor (picker nativo) também conta.
- No OpenCode/OpenCode 2 a retomada insere `--session <id>` depois de `run` quando `run` é o primeiro argumento (posição 1), senão no início. No Codex com `exec` como primeiro argumento a retomada vira `exec resume <id>` e os demais argumentos seguem depois, na ordem.

### Modo de permissão ampliada (`yolo`)

Quando o usuário pede o modo de permissão ampliada, o adaptador acrescenta o flag nativo equivalente, a menos que ele já esteja presente. Nunca amplia uma permissão mais restrita escolhida pelo usuário.

| Harness | Flag acrescentado | Grafias que já satisfazem o pedido |
|---|---|---|
| Claude Code | `--dangerously-skip-permissions` | a própria |
| Codex | `--dangerously-bypass-approvals-and-sandbox` | a própria |
| OpenCode e OpenCode 2 | `--auto` | a própria |
| Pi | `--approve` | a própria |
| Crush | `--yolo` | a própria |
| Oh My Pi | nenhum | — |
| Kimi Code | `--yolo` | `-y`, `--yes`, `--auto-approve`, `--auto` |
| Command Code | `--yolo` | `--dangerously-skip-permissions` |
| Kiro v2 | `--trust-all-tools` (não aplicado se o motor não padrão foi selecionado) | `-a`, `--trust-tools` (escolha mais restrita do usuário) |
| Kiro v3 | nenhum | — |
| Grok | `--yolo` | `--always-approve` |
| Antigravity | `--dangerously-skip-permissions` | a própria |
| Copilot CLI | `--yolo` | `--allow-all` (mesmo conjunto de permissões) |

Uma flag "já presente" casa por igualdade ou por `flag=valor`. O flag é acrescentado **no fim** dos argumentos.

Existe ainda um modo opcional exclusivo do Claude Code. Ele acrescenta uma configuração de sessão que define o modo de permissão como `bypassPermissions`. A intenção é que as regras explícitas de "perguntar" e "negar" continuem valendo, porque essa configuração é unida às demais em vez de substituí-las. Para os outros harnesses, não faz nada.

### Adoção de sessão nativa existente

Um lançamento só pode oferecer a adoção de uma sessão nativa já existente no checkout quando:
- o modo é de sessão;
- não há seletor explícito do usuário;
- a invocação não é não interativa. São não interativas: Claude Code, Pi, Oh My Pi e Command Code com `--print` ou `-p`; Codex com `exec`; OpenCode, OpenCode 2 e Crush com `run`; Kimi Code com `--prompt` ou `-p`; Kiro com `--no-interactive`; Grok com `--single`, `-p`, `--prompt-file` ou `--prompt-json`; Antigravity com `--print`, `-p` ou `--prompt`; Copilot CLI com `--prompt` ou `-p` (`-i`/`--interactive` semeia um prompt e mantém a sessão aberta, então continua adotável).

`--prompt-interactive` e `-i` no Antigravity não tornam a invocação não interativa: a sessão continua adotável.

Do lado do servidor, a adoção só é oferecida enquanto nenhum harness tiver estabelecido o workstream. Quando um harness o estabelece, um harness novo começa limpo e recebe o histórico portátil.

## 2. Identidade e checkpoint do repositório

A inspeção recebe o diretório de trabalho, o resolve para o caminho canônico e falha se ele não puder ser resolvido. Não faz commit, stash, reset nem qualquer outra alteração.

### Fingerprints

- **Fingerprint do repositório** (SHA-256 em hexadecimal, 64 caracteres). A semente é a primeira disponível, nesta ordem:
  1. URLs de todos os remotos `origin`, se houver algum não vazio;
  2. diretório comum do git (`git-common-dir`);
  3. raiz do worktree (`toplevel`);
  4. caminho canônico do diretório (quando não é um repositório git).
  Worktrees ligados ao mesmo repositório compartilham este fingerprint.

  **Fórmula exata.** Todas as saídas do git são decodificadas como UTF-8 (falha → como ausente) e aparadas nas pontas; um comando que falha conta como ausente. `remotos` = saída de `git -C <cwd> remote get-url --all origin`; `comum` = `git rev-parse --path-format=absolute --git-common-dir`; `raiz` = `git rev-parse --show-toplevel`; `gitdir` = `git rev-parse --path-format=absolute --git-dir`; `canônico` = o caminho canônico do diretório (resolvido por links; no Windows, a forma estendida do sistema). A semente do repositório é o texto `git-remotes
<remotos>` se `remotos` não é vazio; senão `git-common
<comum>`; senão `git-root
<raiz>`; senão `directory
<canônico>`. `repo_fingerprint = SHA-256 hex minúsculo da semente`. A semente do worktree é `<repo_fingerprint>
git-dir
<gitdir>`; senão `<repo_fingerprint>
root
<raiz>`; senão `<repo_fingerprint>
cwd
<canônico>`; `worktree_fingerprint = SHA-256 hex da semente`. Observação: no Windows o git imprime caminhos com `/` (`C:/Users/x/repo/.git`); os valores entram na semente exatamente como impressos. Para interoperar com dados criados pela implementação de referência, a semente `directory` usa o caminho canônico exatamente como o sistema o devolve; uma implementação nova que não precise ler fingerprints antigos pode escolher outra normalização desde que seja estável.
- **Fingerprint do worktree**: SHA-256 da combinação do fingerprint do repositório com o diretório git do worktree. Se o diretório git não estiver disponível, usa a raiz do worktree ou o caminho do diretório. Cada worktree tem um valor distinto.
- **Worktree ligado**: verdadeiro quando o diretório git difere do diretório comum.
- **URL de push do origin**: primeira linha da URL de push do `origin` (considera `pushurl` e `pushInsteadOf`). Ausente se não houver.

### Checkpoint

- `head`: hash do commit atual. Ausente se não houver commit ou se o git não estiver disponível.
- `branch`: nome da branch atual. Quando a saída é vazia (HEAD destacado), vale `(detached HEAD)`.
- `dirty_hash`: SHA-256 da saída do status em formato estável com terminadores nulos. Ausente quando o working tree está limpo.
- `changed_paths`: até 256 caminhos alterados. Nomes com espaço são tratados corretamente. Em renomeações e cópias, os dois caminhos entram na lista. Entradas que não são UTF-8 válido são ignoradas.

Para listar workstreams do checkout, existe uma inspeção mais leve que calcula só os dois fingerprints e não executa o status. Isso evita bloquear a listagem em repositórios grandes.

Sem o binário git disponível, a identidade cai para o caminho do diretório e o checkpoint fica sem `head`, `branch` e `dirty_hash`.

## 3. Transcrição: extração incremental

`exportar transcrição` recebe harness, home, checkout, diretório de sessão, identificador nativo e cursor anterior. Devolve o identificador lido, o novo cursor, a lista de eventos e a lista de perdas.

### Princípios

- **Somente leitura.** Os armazenamentos nativos nunca são modificados.
- **Cursor opaco.** O servidor guarda o cursor apenas para a próxima leitura local. Ninguém mais deve interpretá-lo.
- **Perdas explícitas.** Registros malformados, privados ou não suportados viram entradas em uma lista de perdas, deduplicadas. Nunca há falha silenciosa.
- **Limites:**
  - conteúdo de evento de no máximo 128 KiB, truncado em fronteira de caractere UTF-8;
  - no máximo 50 000 arquivos coletados em cada varredura de diretório (busca em profundidade; links simbólicos de diretório não são seguidos como diretório);
  - identificador nativo de no máximo 512 bytes;
  - por exportação: no máximo **32 768** registros de origem, **1 MiB** por registro, **64 MiB** de bytes de origem, **4 096** eventos e **32 MiB** de eventos serializados; estourar qualquer um é **erro** (`native transcript exceeds the … limit`), não truncamento. O cabeçalho de sessão lido para descoberta tem no máximo 128 KiB (e metadados de sessão, 1 MiB);
  - listar sessões nativas examina no máximo os **2 000** arquivos mais recentes (ordenados por modificação, desempate pelo caminho) e devolve candidatos únicos cujo diretório registrado coincide com o do checkout.
- **Sem reentrada de contexto.** O pacote de contexto entregue pelo ai-memory e o contexto sintético de cada harness não entram de volta no registro (ver 3.2).

### 3.1 Tipos de fonte

#### JSONL em arquivo

Usado por Claude Code, Codex, Pi, Oh My Pi, Kimi Code, Command Code, Kiro v2, Kiro v3, Grok e Copilot CLI.

Quais arquivos são transcrições (o resto é ignorado):
- Claude Code: qualquer `*.jsonl`, **exceto** o que estiver sob uma pasta `subagents` (cadeias laterais de subagente carregam o id e o diretório do pai e fazem parte da sessão, nunca são sessão própria);
- Codex: `*.jsonl`;
- Pi e Oh My Pi: `*.jsonl` e também temporários `*.jsonl.*.tmp` (arquivo ainda sendo reescrito);
- Kimi Code: só `agents/main/wire.jsonl` (diários de subagente e outros `.jsonl` ficam fora);
- Command Code: `*.jsonl` cujo nome sem extensão é um UUID (arquivos como `<id>.checkpoints.jsonl` ficam fora);
- Kiro v2: `*.jsonl` cujo nome sem extensão é um UUID;
- Kiro v3: `messages.jsonl` numa pasta `sess_<UUID>`; a busca também varre a raiz padrão `~/.kiro/sessions` além da raiz configurada (compatibilidade com o Kiro 2.16.2, que grava v3 na raiz padrão mesmo com `KIRO_HOME`), e cada resultado passa pela validação estrita de metadados e de checkout;
- Grok: só `chat_history.jsonl` (`events.jsonl`, `updates.jsonl`, `rewind_points.jsonl` ficam fora);
- Copilot CLI: só `<UUID>/events.jsonl` sob a raiz de sessões (checkpoints, arquivos e artefatos de pesquisa na mesma pasta ficam fora). O cabeçalho é a **primeira linha**, que precisa ser `session.start` com `data.sessionId` igual ao nome da pasta (e UUID) e `data.context.cwd` absoluto.

- **Cursor:** caminho do arquivo, offset em bytes, motor (apenas para Kiro) e, quando aplicável, hash do prefixo.
- **Confirmação por nova linha:** o avanço para até o último byte de nova linha completo. Uma linha final parcial, que ainda está sendo escrita, é relida na próxima leitura.
- **Cursor inválido:** cursor de outro caminho, ou de outro motor Kiro, é ignorado e a leitura começa do zero.
- **Offset maior que o arquivo:** nos formatos de somente acréscimo, o offset é limitado ao tamanho atual. Nos formatos reescritos (abaixo), o cursor é descartado e a leitura recomeça do zero.
- **Journal reescrito no lugar:** Kimi Code (em bifurcação, compactação ou retomada), Grok (em rewind), Kiro v2 e v3 e Copilot CLI. No caso do Kiro e do Copilot, o comportamento somente de acréscimo não é documentado, então eles são tratados como reescrevíveis.
- **Registro lido:** cada linha é um objeto JSON; linha acima de 1 MiB, ou além de 32 768 registros, aborta a exportação. Uma linha sem `\n` no fim não é consumida. O cursor (JSON) carrega `path`, `offset`, `flavor` (só `kiro-v2` ou `kiro-v3`) e, nos formatos reescritos, `prefix_sha256`. Um cursor cujo caminho ou `flavor` não bate com a sessão é ignorado e a leitura recomeça do zero.
- **Contexto de diretório por registro:** Claude Code carrega o `cwd` em cada linha e Codex em `session_meta`/`turn_context` (`payload.cwd`); eventos desses harnesses recebem o `cwd` absoluto do próprio registro nos metadados (chave `cwd`), porque a sessão pode mudar de diretório. A importação da política de captura (ver `lifecycle-event-ingestion.md`) usa esse rótulo.
  - O cursor guarda o SHA-256 de todos os bytes confirmados. Se o prefixo não bater com o hash guardado, a leitura recomeça do zero.
  - O identificador do registro é o SHA-256 da linha crua. Assim ele permanece estável após reescrita e a deduplicação no servidor continua correta.
- **Demais formatos:** o identificador do registro é o campo de identidade do próprio registro (por exemplo `uuid`, `id`, `messageId`, `call_id`, `callId` ou o mesmo campo dentro de `payload`). Sem nenhum deles, usa `byte-<offset>`.
- **Linha malformada:** vira perda com a posição em bytes, e a leitura segue para a próxima linha.

#### SQLite nativo

Usado por OpenCode, OpenCode 2 e Crush. Os bancos são abertos somente leitura.

- **Cursor:** par (instante de atualização, identificador). A leitura seguinte pega registros com instante maior, ou com o mesmo instante e identificador maior, na ordem composta (`ORDER BY instante, id`, com limite de 32 769 linhas para detectar o estouro de 32 768).
- **OpenCode (v1):** banco `opencode.db`; descoberta e listagem em `session` por `directory` (as duas grafias do caminho do checkout: nativa e com barras normais) e `time_updated`; leitura de `part` unida a `message` (cursor sobre `part.time_updated`, `part.id`). Partes: `text` (só papéis `user` e `assistant`; outro papel gera a perda "OpenCode non-conversation message records were intentionally excluded"), `tool` (chamada `<nome>: <entrada compacta>` e, se houver saída não vazia, resultado com o status nos metadados), `compaction` (evento de compactação), `reasoning` (perda "OpenCode hidden reasoning was intentionally excluded"); mensagem ou parte malformada vira perda ("malformed OpenCode message for part <id>" / "malformed OpenCode part <id>").
- **OpenCode 2:** mesmo arquivo de banco, mas tabelas **próprias**: `session_v2` (descoberta, listagem, existência, `directory`/`time_updated`) e `session_message` (`id`, `time_updated`, `type`, `data`). Tipo `user` (campo `text`) vira mensagem; `assistant` percorre `content[]`: `text` vira mensagem, `tool` vira chamada `<nome>: <entrada>` e, se `state.status` for `completed` (padrão) e o texto concatenado de `state.content[].text` não for vazio, um resultado (o índice de bloco é `2 × posição` para a chamada e `+1` para o resultado); status diferente de `completed` gera a perda "OpenCode 2 tool <nome> ended with status <status>"; `reasoning` e outros tipos de parte geram perdas ("OpenCode 2 hidden reasoning was intentionally excluded", "OpenCode 2 <tipo> part was intentionally excluded"); assistente sem `content` gera "OpenCode 2 assistant message <id> has no content parts"; outros tipos (avisos de catálogo do sistema etc.) são ignorados. Linha com JSON ruim: "malformed OpenCode 2 message <id>".
- **Crush:** banco `crush.db` (tabelas `sessions`, `messages`, `files`). Listagem e descoberta ignoram sessões filhas (`parent_session_id` nulo apenas); instantes são segundos ou milissegundos (valor abaixo de 100 000 000 000 conta como segundos). Leitura de `messages` (`parts` em JSON, `is_summary_message`): texto de `user`/`assistant` vira mensagem (ou compactação, se for mensagem de resumo); `tool_call` vira chamada `<nome>: <entrada>` (as não terminadas geram a perda "unfinished Crush tool calls were intentionally excluded"); `tool_result` vira resultado (`tool`, `is_error` nos metadados); `reasoning` e `binary` geram perdas nomeadas; `parts` que não é lista gera "malformed Crush message parts for <id>". A descoberta de uma sessão nova olha as 64 sessões raiz mais recentes (`created_at` em lançamento novo, `updated_at` em continuação), aceita as criadas até 2 s antes do início e, se o diretório de dados fica **fora do projeto** (compartilhado), só as que editaram arquivo dentro do projeto (caminhos absolutos em `files`, incluindo as de sub-sessões, comparados com e sem resolver links); mais de uma candidata é a **ambiguidade** descrita em 3.3.

#### Antigravity

O armazenamento de conversas guarda cada passo como um blob protobuf cujo enum de tipos não é versionado. Por isso, passos do assistente e de ferramentas não são decodificados. A exportação importa apenas os prompts do usuário, lidos do arquivo de histórico compartilhado que o `agy` mantém ao lado do armazenamento.

- Uma linha do histórico só é aceita quando o identificador de conversa **e** o workspace batem com a conversa validada. Uma conversa retomada a partir de outro diretório mantém o identificador, mas o workspace gravado é outro, e essas linhas não entram.
- O primeiro prompt de uma conversa nova é gravado antes de a conversa ter identificador. Ele só é atribuído ao checkout quando há exatamente um prompt sem rótulo, desse workspace, digitado até 5 segundos antes da criação do arquivo da conversa.
- O histórico usa cursor com offset e hash do prefixo, como no JSONL reescrevível. Se o prefixo mudou, a leitura recomeça.
- Linha com mais de 64 KiB é pulada. Uma perda é registrada apenas se a linha mencionar a conversa.
- Uma linha final sem nova linha não é consumida.
- A perda "passos não decodificáveis; só prompts do usuário foram importados" é sempre registrada.
- Se o arquivo de histórico estiver ausente ou ilegível, a exportação não gera eventos, registra perda explícita e não devolve cursor.

### 3.2 Normalização

Cada parser transforma um registro nativo em eventos do tipo:

| Tipo | Origem |
|---|---|
| `message` | mensagens de usuário, assistente, desenvolvedor, sistema ou ferramenta |
| `tool_call` | invocação de ferramenta, sempre como histórico concluído |
| `tool_result` | saída concluída ou com erro de ferramenta |
| `compaction` | compactação ou resumo de contexto feito pelo harness |

Os tipos `checkpoint` e `annotation` existem no modelo, mas não são produzidos pela importação de transcrição. Essa importação registra perdas na lista própria, não como eventos.

Regras gerais:
- **Identificador do evento:** `native:` seguido do SHA-256 hexadecimal do texto `<agente>\0<sessão>\0<registro>\0<bloco>\0<tipo>\0<conteúdo truncado>` (separadores são o byte nulo; o conteúdo já truncado em 128 KiB). `agente` é o nome kebab do tipo de agente; `bloco` é o índice do bloco dentro do registro (inteiro decimal). Reenviar o mesmo lote gera os mesmos identificadores, então a importação é idempotente. Alterar o conteúdo muda o identificador. O evento guarda também `source_record_id` (o identificador do registro) e `native_session_id`.
- **Copilot CLI:** registros `user.message` (`data.content` vira mensagem de usuário); `assistant.message` (o texto vira mensagem de assistente e cada `toolRequests[]` vira uma chamada `<nome>: <argumentos compactos>` com `toolCallId`, com índice de bloco depois do texto; presença de `reasoningText`, `reasoningOpaque` ou `reasoningBlocks` gera a perda "Copilot hidden reasoning was intentionally excluded"); `tool.execution_complete` (resultado: com `success == false`, o texto é `error: <data.error.message>`, senão `data.result.content`; `detailedContent` não entra por repetir a saída com caminhos absolutos; metadados `success` e o id da chamada); `session.compaction_complete` com `success == true` e `summaryContent` vira compactação; `system.message` e `model.messages_snapshot` geram perdas ("Copilot system prompt records were intentionally excluded" / "Copilot model message snapshots were intentionally excluded"); demais tipos são ignorados.
- **Metadados do evento:** quando o registro traz o id da chamada de ferramenta, ele vai nos metadados (`tool_use_id`); chamadas guardam o nome da ferramenta em `tool`. Os ids que a política de captura exige válidos têm 1 a 128 caracteres ASCII em `[A-Za-z0-9_.-]`.
- **Conteúdo vazio ou só com espaços** não gera evento.
- **Conteúdo longo** é truncado a 128 KiB, sem cortar caractere UTF-8 ao meio.
- **Data:** `occurred_at` em RFC 3339. Timestamps em milissegundos (Kimi Code e Kiro) são convertidos.
- **Metadados:** cada parser escolhe poucos campos pequenos, como nome da ferramenta e status. Nenhum campo bruto do registro é copiado.
- **Raciocínio oculto do provedor:** não é importado. Gera uma perda nomeada pelo harness, indicando que o raciocínio foi excluído de propósito.
- **Chamadas históricas de ferramenta** nunca são reapresentadas como pendentes.
- **Compactação nativa** vira evento `compaction`.

Filtros de eco e de contexto sintético:
- **Claude Code:** mensagens que começam com o marcador do pacote gerenciado (ou com o prefixo legado) são descartadas. Registros marcados como metadados internos (`isMeta`) são excluídos, com perda. Blocos de lembrete do sistema (`<system-reminder>`) também são descartados.
- **Codex:** blocos de contexto sintético (instruções do `AGENTS.md`, contexto de ambiente, instruções de permissão) são descartados quando aparecem como mensagem do usuário.
- **Grok:** blocos de informação do usuário, de lembrete do sistema e de ambiente são descartados. Entrada real do usuário vem envolvida em marcador próprio.

### 3.3 Descoberta e verificação de sessão

- **Descoberta de sessão recém-criada.** Usada quando o harness não aceita identificador escolhido e a vinculação por hook não aconteceu. Os candidatos são ordenados pela modificação mais recente, e no máximo 512 são examinados. A busca para ao encontrar um arquivo modificado mais de 2 segundos antes do início do lançamento. O primeiro candidato cujo cabeçalho aponta para o checkout é aceito.
- **Bancos SQLite.** A descoberta segue a mesma lógica, filtrando por diretório do checkout e instante de atualização posterior ao início.
- **Crush.** Não tem hook para vincular a sessão. O lançamento reivindica apenas a sessão que criou (ou, na continuação, a que tocou).
- **Ambiguidade.** Se sessões novas aparecem num armazenamento que não registra qual lançamento as criou, e não é possível atribuí-las a este run, nenhuma delas é reivindicada. A descoberta falha com erro de ambiguidade, informando quantas sessões surgiram e, para armazenamento fora do projeto, quantas editaram arquivos do projeto.
- **Existência de sessão.** Retorna "não existe" somente quando a ausência é certa. Falha de acesso ao armazenamento ou de esquema é erro, para que um armazenamento ilegível não seja confundido com uma sessão apagada.
- **Pertence ao checkout.** Compara o diretório registrado na sessão com o checkout canônico. Para bancos SQLite, compara as duas grafias do caminho do checkout.
- **Localização do arquivo.** Identificador inválido não é procurado. Raízes são definidas por harness e podem ser sobrescritas pelo diretório de sessão.
- **Espera de gravação.** Antes de importar, verifica até 10 vezes, com 200 ms entre as verificações. Para cedo quando duas leituras consecutivas dão o mesmo resultado e a sessão existe. A comparação usa caminho e tamanho do arquivo, ou o instante de atualização no SQLite.
- **Kiro v3 com `KIRO_HOME`.** O motor v3 grava sessões na raiz padrão quando `KIRO_HOME` está definido, mas procura apenas na raiz configurada ao retomar. Se a sessão só existir na raiz padrão, o lançamento remove `KIRO_HOME` apenas para esse processo, para que a retomada encontre a sessão.

### 3.4 Exportação de recuperação (intervalo, delta e linha de base)

Além da leitura incremental, o adaptador oferece três operações usadas pela recuperação de execuções gerenciadas (013) e pelo backfill:

- **Linha de base:** devolve o cursor exato do fim atual de uma sessão (JSONL: varre o arquivo até o último `\n` completo, com os mesmos limites de registros e bytes, e calcula o hash do prefixo nos formatos reescritos; SQL: o último par (instante, id) de `part` no OpenCode, de `session_message` no OpenCode 2, de `messages` no Crush, ou o par zero se não houver linhas). O Antigravity não tem cursor confiável e a operação falha (`antigravity does not expose a reliable transcript cursor`).
- **Delta:** lê apenas depois de um cursor devolvido para esta sessão e adaptador. Cursor inválido ou obsoleto **falha fechado** em vez de reler tudo: cursor de outro arquivo ou de outro `flavor`, deslocamento maior que o arquivo, ou (nos formatos reescritos) hash do prefixo ausente ou diferente (`native transcript changed before the recovery cursor`).
- **Intervalo:** lê só entre dois cursores (`final_cursor` incluso, nada além dele); os dois são validados contra o adaptador e a sessão, e o final não pode preceder o inicial nem mudar de arquivo ou `flavor` (`native transcript final cursor precedes or changes its start cursor`). O Antigravity recusa intervalo (`antigravity transcripts support no bounded recovery range; …`).
- **Resumo semântico do intervalo:** lista ordenada de SHA-256 por evento sobre (id do evento, tipo, papel, `occurred_at`, conteúdo), cada campo precedido do seu comprimento em 8 bytes big-endian; serve para confirmar que um intervalo regravado é idêntico.
- Todas as exportações passam pelos limites da seção "Limites".

### 3.5 Continuação do Claude Code depois do lançamento

Todo registro de um processo do Claude Code nomeia em `session_id` a sessão com que o processo foi lançado e, em `sessionId`, a sessão do próprio arquivo. Dois movimentos deixam a sessão de lançamento para trás, e sem tratá-los o workstream continuaria na sessão errada:

- **`/resume` de uma sessão em segundo plano** (hospedada pelo daemon do Claude Code; seus registros trazem `"sessionKind": "bg"`): a conversa segue na transcrição da sessão em segundo plano.
- **`/clear`**: nasce uma sessão nova na mesma execução, cuja transcrição registra o comando (`<command-name>/clear</command-name>` num registro de usuário).

A busca olha as transcrições escritas desde o início do lançamento (mais recentes primeiro, no máximo 512, parando numa modificada mais de 2 s antes do início; ignora a própria sessão de lançamento): um registro `bg` com o `session_id` do lançamento e `cwd` igual ao checkout vale como continuação em segundo plano; uma sessão com `/clear` e registros que citam a de lançamento com `cwd` dentro do checkout (pode ter descido para um subdiretório) vale como `Cleared`. Cadeias de `/clear` terminam na última. Transcrições sem esses campos (versões antigas) nunca casam. Existe também a identificação de uma sessão em segundo plano **ainda viva** na saída de `claude agents --json` (`kind == background`, mesma sessão e mesmo diretório, id curto alfanumérico/hífen) para o lançador usar `claude attach <id>` em vez de `--resume`, que o Claude recusa nesse caso.

## 4. Fluxo do `ai-memory run`

1. **Identidade do checkout.** Calcula os fingerprints e o checkpoint inicial.
2. **Preparação.** Chama `POST /workstream/runs`. O servidor escolhe ou cria o workstream e abre um lease de **90 segundos**. A resposta traz, quando aplicáveis, o identificador nativo vinculado, o cursor de origem, a faixa de sincronização e o agente resolvido. Ao preparar, o servidor também expira leases vencidos de execuções ativas.
3. **Plano de lançamento.** Gera o comando nativo. O contexto inicial entra pelo hook de início de sessão. Para o Crush, que não tem esse hook, o contexto é gravado em arquivo global antes do lançamento.
4. **Execução.** O processo filho roda. A cada **30 segundos**, o orquestrador envia heartbeat, que renova o lease para mais 90 segundos. Um lease sem renovação expira, e a execução pode ser assumida por outra preparação. Os hooks do harness recebem a variável `AI_MEMORY_RUN_ID` com o identificador da execução.
5. **Saída:**
   1. aguarda a gravação do transcript (seção 3.3);
   2. resolve o identificador nativo. Se não for possível descobri-lo, nada é importado e a perda "native session id could not be discovered; transcript was not imported" é registrada;
   3. em lançamento de sessão, exporta o transcript. O cursor anterior só é usado se o identificador lido for o mesmo da preparação;
   4. calcula o checkpoint final, com uma nova inspeção do repositório;
   5. chama `POST /workstream/runs/{id}/finish` com os eventos, o novo cursor e o código de saída. Os eventos são idempotentes.
6. **Passthrough.** Nas etapas de saída, não há exportação de transcrição, mas o checkpoint final é enviado.

## 5. Notas para reimplementação

1. Torne o planejador de comando transparente: acrescente um seletor somente quando ele estiver ausente, e nunca reescreva argumentos do usuário.
2. Faça o cursor de JSONL avançar por offset de byte apenas até a última nova linha completa. Quando o formato pode ser reescrito, valide o cursor por hash do prefixo e use o hash do conteúdo da linha como identificador do registro. Em formatos de somente acréscimo, se o arquivo encolheu, limite o offset ao tamanho atual; em formatos reescritos, descarte o cursor.
3. Registre perdas em vez de adivinhar formato. Quando um harness não puder ser decodificado por completo, importe o que for legível e registre a perda nomeada, como no Antigravity.
4. Derive o identificador do evento da fonte, para que reenviar o mesmo lote não duplique nada.
5. Abra os bancos SQLite nativos somente para leitura, use cursor por par (instante, identificador) e trate falha de acesso como erro, nunca como "não encontrado".
6. Valide identificadores nativos antes de usá-los em caminhos de arquivo. Rejeite separadores de caminho e os nomes `.` e `..`.

## Veja também

- [Workstreams gerenciados](managed-workstreams.md)
- [Como adicionar um harness gerenciado](managed-harness-contributions.md)
- [Matriz de suporte por harness](support-matrix.md)
- [Modo yolo e ai-jail](design-yolo-safety-ai-jail.md)
- [Ingestão de eventos do ciclo de vida](lifecycle-event-ingestion.md)
