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

- O executável padrão é resolvido pelo `PATH`. O usuário pode informar outro executável.
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
- pedidos de versão ou ajuda (`--help`, `-h`, `--version`, `-v`; no Kiro, `-v` é verbose e a versão é `-V`, então `--help-all` também conta como ajuda);
- `--no-session` e `--no-session-persistence`;
- subcomandos utilitários de cada harness, como autenticação, plugins, servidores MCP, atualização e diagnóstico;
- no Codex: `exec review` e `exec help`;
- no Command Code: `--list-models` e `--ide-setup`;
- no Kiro v2: seleção de motor não padrão (`--v3`, `--mode` ou `--agent-engine` com valor diferente de `v2`), e também `--no-interactive`, `--list-sessions`, `-l`, `--list-models`, `--delete-session` e `-d`. Um valor desconhecido de `--agent-engine` não é atribuído a nenhum adaptador.

### Validação de identificadores

- Um identificador nativo (explícito ou vinculado) é validado antes do lançamento. Se for inválido, o planejamento falha com erro.
- Regras: não vazio após remover espaços; até 512 bytes; sem caracteres de controle; sem caracteres de formatação invisíveis; sem marcadores de redação; e sem alteração pelo filtro de privacidade. Além disso, não pode começar com hífen, não pode ser `.` nem `..`, e não pode conter barra ou barra invertida.
- No Command Code, um identificador explícito que não seja UUID é descartado do plano (sem erro).

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

Command Code e Crush não têm variável de ambiente de raiz de sessão. No Oh My Pi, um perfil nomeado inválido ou sem home resolvível não produz diretório extra.

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

- Em lançamento de sessão do Kiro v3 sem seleção explícita do motor v3, o argumento `--v3` é inserido no início.
- Um seletor próprio do usuário (por exemplo `--resume`, `--continue`, `--session`) impede a injeção de qualquer seletor do adaptador.

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

Existe ainda um modo opcional exclusivo do Claude Code. Ele acrescenta uma configuração de sessão que define o modo de permissão como `bypassPermissions`. A intenção é que as regras explícitas de "perguntar" e "negar" continuem valendo, porque essa configuração é unida às demais em vez de substituí-las. Para os outros harnesses, não faz nada.

### Adoção de sessão nativa existente

Um lançamento só pode oferecer a adoção de uma sessão nativa já existente no checkout quando:
- o modo é de sessão;
- não há seletor explícito do usuário;
- a invocação não é não interativa. São não interativas: Claude Code, Pi, Oh My Pi e Command Code com `--print` ou `-p`; Codex com `exec`; OpenCode, OpenCode 2 e Crush com `run`; Kimi Code com `--prompt` ou `-p`; Kiro com `--no-interactive`; Grok com `--single`, `-p`, `--prompt-file` ou `--prompt-json`; Antigravity com `--print`, `-p` ou `--prompt`.

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
  - no máximo 50 000 entradas examinadas em cada varredura de diretório;
  - identificador nativo de no máximo 512 bytes.
- **Sem reentrada de contexto.** O pacote de contexto entregue pelo ai-memory e o contexto sintético de cada harness não entram de volta no registro (ver 3.2).

### 3.1 Tipos de fonte

#### JSONL em arquivo

Usado por Claude Code, Codex, Pi, Oh My Pi, Kimi Code, Command Code, Kiro v2, Kiro v3 e Grok.

- **Cursor:** caminho do arquivo, offset em bytes, motor (apenas para Kiro) e, quando aplicável, hash do prefixo.
- **Confirmação por nova linha:** o avanço para até o último byte de nova linha completo. Uma linha final parcial, que ainda está sendo escrita, é relida na próxima leitura.
- **Cursor inválido:** cursor de outro caminho, ou de outro motor Kiro, é ignorado e a leitura começa do zero.
- **Offset maior que o arquivo:** nos formatos de somente acréscimo, o offset é limitado ao tamanho atual. Nos formatos reescritos (abaixo), o cursor é descartado e a leitura recomeça do zero.
- **Journal reescrito no lugar:** Kimi Code (em bifurcação, compactação ou retomada), Grok (em rewind) e Kiro v2 e v3. No caso do Kiro, o comportamento somente de acréscimo não é documentado, então ele é tratado como reescrevível.
  - O cursor guarda o SHA-256 de todos os bytes confirmados. Se o prefixo não bater com o hash guardado, a leitura recomeça do zero.
  - O identificador do registro é o SHA-256 da linha crua. Assim ele permanece estável após reescrita e a deduplicação no servidor continua correta.
- **Demais formatos:** o identificador do registro é o campo de identidade do próprio registro (por exemplo `uuid`, `id`, `messageId`, `call_id`, `callId` ou o mesmo campo dentro de `payload`). Sem nenhum deles, usa `byte-<offset>`.
- **Linha malformada:** vira perda com a posição em bytes, e a leitura segue para a próxima linha.

#### SQLite nativo

Usado por OpenCode, OpenCode 2 e Crush. Os bancos são abertos somente leitura.

- **Cursor:** par (instante de atualização, identificador). A leitura seguinte pega registros com instante maior, ou com o mesmo instante e identificador maior, na ordem composta.

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
- **Identificador do evento:** `native:` seguido do SHA-256 de agente, identificador da sessão, identificador do registro, índice do bloco, tipo e conteúdo. Reenviar o mesmo lote gera os mesmos identificadores, então a importação é idempotente. Alterar o conteúdo muda o identificador.
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
