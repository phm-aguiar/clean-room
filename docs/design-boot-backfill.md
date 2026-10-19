# Design: backfill de histórico local anterior ao hook

**Status: implementado na versão 2.3.0.** Este documento descreve o comportamento atual da importação única do histórico local de um projeto e separa o que existe do que é proposta. Itens marcados como **(proposta)** não existem no código. Itens marcados como **(descartado)** foram substituídos por outra abordagem.

## Resumo do estado

| Item | Estado |
|---|---|
| Gatilho automático no início de sessão, sem bloquear o início | Implementado |
| Importação só quando a loja do projeto está vazia (no escopo do chamador) | Implementado |
| Comando manual `backfill` com `--dry-run`, `--force`, `--session`, `--max-sessions`, `--json`, `--quiet` | Implementado |
| Importação por replay pelo ingresso de captura ao vivo | Implementado (caminho B) |
| Limites de sessões, de eventos totais e de tamanho de lote (itens e bytes) | Implementado (parcial, ver Limites) |
| Trava de disparo (um só processo automático por diretório) | Implementado: marcador "em andamento" com trava de arquivo e dono (ver §1). Não há trava de importação no servidor |
| Chaves de ingestão válidas para o replay (deduplicação no servidor) | Implementado |
| Aplicação das exclusões de captura (`ignore_paths`) e do modo de captura ao backfill | Implementado (ver §4 e §11) |
| Cursor incremental por sessão | Proposta |
| Verificação de vazio no próprio handshake de início de sessão | Proposta |
| Aviso do resultado na sessão seguinte | Proposta |
| Opt-out por marcador de projeto | Proposta |
| Restrição do automático em servidor multiusuário | Questão em aberto, não implementada |

## Problema

A captura é só para frente. Os hooks de ciclo de vida registram eventos a partir do momento em que são instalados. Se você já trabalhou num projeto por semanas, com uma sessão longa do Claude Code ou do Codex no disco, e só agora instalou o ai-memory, esse contexto anterior não está na memória. Você tem um "início frio": a ferramenta que existe para dar continuidade entre sessões começa sem saber nada da sessão que você está retomando.

O diagnóstico `ai-memory doctor` (ver [cli.md](cli.md)) torna visível o problema relacionado: "este harness rodou aqui e não capturou nada". Este design cobre uma instância específica: **quando a loja do projeto é nova (vazia) e o harness retoma uma sessão que já tem histórico local, importar esse histórico uma vez**, para que as primeiras páginas consolidadas reflitam o trabalho real.

## Base reutilizada

A parte difícil (ler a transcrição nativa, sanitizar e gravar no armazenamento) já existe. O backfill é um gatilho com guardas em volta da ingestão existente.

- **Adaptadores de transcrição** leem, em modo somente leitura, o armazenamento nativo de cada harness, filtrado pelo diretório de trabalho (ver [harness-adapters.md](harness-adapters.md)). O diagnóstico usa a mesma listagem de sessões nativas.
- **Exportação de transcrição** devolve a sessão nativa, uma lista limitada de eventos visíveis, um cursor opaco de leitura incremental e um registro de perdas (dados privados, malformados ou não suportados). O backfill usa apenas os eventos: não passa cursor e não registra as perdas.
- **Execução gerenciada** (`ai-memory run`) envia eventos em lotes ao final de uma execução gerenciada. Esse caminho alimenta o registro de continuidade do workstream, não a memória pesquisável (ver [managed-workstreams.md](managed-workstreams.md)).
- **Importador de conversas externas** (ver [companions.md](companions.md)) reenvia conversas pelo mesmo ingresso de eventos. Seus limites (128 mensagens por conversa, 16 KiB por mensagem do usuário, 1 MiB por conversa) e seu tipo de agente próprio (`external-import`) são do importador. **O backfill não os reutiliza.**
- **Sanitização**: todo evento recebido pelo ingresso de captura ao vivo passa pela sanitização antes de virar observação (ver [threat-model-and-data-handling.md](threat-model-and-data-handling.md)).
- **Execução destacada a partir do hook**: o hook dispara um processo separado e não espera por ele (ver [lifecycle-event-ingestion.md](lifecycle-event-ingestion.md)).

## Decisões (confirmadas com o operador)

- **Ligado por padrão**, com desligamento simples.
- **Bootstrap do projeto inteiro**: quando a loja está vazia, importa **todas** as sessões locais do diretório de trabalho em todos os harnesses suportados, e não só a sessão retomada. Há limites e um aviso de uma linha.
- **Uso único**: o automático só inicializa um projeto sem sessões. Um projeto com histórico nunca é importado automaticamente.

## Comportamento atual

### 1. Gatilho no início de sessão

No início de sessão, quando nenhum dono externo de captura está declarado no ambiente, o hook verifica se existe a **sentinela local** do diretório de trabalho atual. Se não existir, tenta **reivindicar o marcador "em andamento"** do diretório; só quem o reivindica dispara um processo de backfill destacado, sem herdar a entrada e saída do hook, e retorna sem esperar. O hook não consulta o servidor para isso e não importa nada. Um hook que não emite nada (modo de captura que não admite o diretório, ou perfil de servidor do marcador recusado) também não dispara backfill.

O processo disparado é, em termos de linha de comando, `backfill --auto --quiet --inflight-claim <token>` mais **um** destino: `--server-profile <nome>` quando o marcador do repositório seleciona um perfil de servidor, senão `--server-url <url da instalação do hook>`. O token do servidor nunca vai na linha de comando nem no ambiente: o backfill o lê do arquivo do hook ou do perfil. A saída de erro do processo vai para `<dados>/logs/backfill.log`, que passa a `backfill.log.old` quando ultrapassa 1 MiB (uma geração).

**Marcador "em andamento".** Arquivo `<hash do diretório>.inflight` ao lado da sentinela, criado de forma exclusiva e protegido por uma trava de arquivo por diretório (`<hash>.inflight.lock`, que nunca é apagada). Contém um token aleatório de 32 hexadecimais (UUID v4 sem hífens) que identifica o dono. Regras:

- A trava é tomada **sem esperar**: contenção ou diretório de estado inutilizável significa "não reivindicou", e o hook segue sem disparar nada (nunca bloqueia o início de sessão).
- Se o marcador já existe e foi modificado há **menos de 30 minutos**, ninguém mais reivindica. Se tem 30 minutos ou mais, é considerado órfão: é renomeado para `.stale.<token novo>` e apagado, e a reivindicação é refeita (no máximo duas tentativas).
- O processo disparado renova o marcador (atualiza o horário de modificação) no início e depois de cada sessão importada, mas só enquanto o conteúdo do arquivo ainda for o seu token. Ao terminar, de qualquer forma (sucesso, erro ou queda de controle), apaga o marcador, mas só se ainda for dele. Assim um processo atrasado nunca libera nem renova a reivindicação de quem assumiu depois.
- Se o disparo falha, o hook libera o marcador na hora.
- Efeito: uma rajada de inícios de sessão no mesmo diretório dispara **um** backfill, não um por início.

Consequências:

- Uma vez por diretório de trabalho e por máquina, esse processo é iniciado **mesmo com o recurso desligado ou com a loja não vazia**. Ele sai sem importar e grava a sentinela.
- A sentinela só é gravada ao final da execução. Enquanto o marcador "em andamento" é válido, novos inícios de sessão no mesmo diretório não disparam outro processo; se a execução morrer sem liberar, o disparo volta a ser possível depois de 30 minutos. Execuções manuais (`backfill` sem `--auto`) não usam o marcador.
- Uma falha ao disparar o processo não afeta o início de sessão.

### 2. Comando `backfill`

Fluxo de uma execução real:

0. **Marcador.** Na execução automática com token de reivindicação, renova o marcador "em andamento" (ver §1); ele é liberado ao final, de qualquer forma.
1. **Opt-out automático.** Se a execução é automática e o recurso está desligado, grava a sentinela (exceto em simulação) e sai com sucesso, sem contatar o servidor.
2. **Escopo.** Resolve workspace e projeto (padrão: escopo do projeto atual; sobrescritos por `--workspace` e `--project`) e a localização das lojas nativas. Se isso falhar, sai com erro **sem gravar a sentinela**. A próxima abertura de sessão tenta de novo.
2b. **Admissão de captura.** Aplica ao diretório o mesmo portão do hook ao vivo (modo de captura da instalação e marcador `.ai-memory.toml`). Se o hook ao vivo não emitiria nada daqui (modo de captura "lista de permissão" sem marcador que adira, ou marcador ilegível), o relatório traz `skipped_not_admitted: true`, **nada é importado, o servidor nem é contatado e a sentinela não é gravada**. Mensagem humana: `ai-memory: capture mode is allowlist and this checkout has no .ai-memory.toml marker opting in (or its marker is unreadable) — nothing to backfill for <ws>/<proj>.`
3. **Servidor.** Na execução automática, usa o mesmo destino do hook que a disparou: um perfil de servidor (com o token do próprio perfil, o único que apresenta) ou a URL de instalação do hook. Com URL, a credencial é o token persistido do hook e, depois, OIDC. A credencial de ambiente ou de configuração **não** é usada nesse caminho. Na execução manual, o servidor vem da configuração (e da resolução normal de credencial). Um nome de perfil inválido ou recusado é erro (`` `<nome>` is not a valid server profile name`` / ``server profile `<nome>` was refused (<motivo>)``).
4. **Portão de vazio** (ver Idempotência), salvo com `--force`, com **tempo máximo de 30 s** na consulta (estouro vira erro: ``the server did not answer within 30s``, sem sentinela). Se a loja não está vazia, grava a sentinela (exceto em simulação), gera o relatório com `skipped_non_empty` e sai.
5. **Enumeração.** Lista as sessões locais do diretório de trabalho em todos os harnesses suportados (ver Cobertura de harness).
6. **Seleção.** Mantém as N mais recentes (ver Seleção).
7. **Importação.** Importa as sessões selecionadas em ordem cronológica, da mais antiga para a mais recente, até o limite de eventos.
8. **Fechamento.** Grava a sentinela, emite o relatório e, se alguma sessão falhou, sai com código de erro.

Simulação (`--dry-run`) executa os passos 1 a 6, inclusive a consulta de vazio ao servidor. Não importa nada e **não grava a sentinela**, então não suprime a tentativa automática seguinte.

### 3. Seleção de sessões

- Por harness, são enumeradas no máximo as **200 sessões mais recentes** do diretório de trabalho. Se um harness atingir esse limite, o fato é calculado mas não é mostrado ao usuário.
- Ordenação: data de atualização, da mais recente para a mais antiga. Em caso de empate, identificador nativo em ordem crescente.
- `--session <id>` mantém apenas a sessão com esse identificador nativo exato, antes da seleção.
- `--max-sessions` (padrão 25) corta a lista ordenada. Valor zero é tratado como 1.

### 4. Mapeamento de eventos

Cada sessão selecionada vira:

- uma **abertura sintética** (início de sessão) com o horário da primeira vez válida;
- os **eventos mapeados**, na ordem da transcrição;
- um **fechamento sintético** (fim de sessão) com o horário da última vez válida.

A abertura e o fechamento são enviados **sempre**, mesmo quando a sessão não tem conteúdo. Uma sessão sem conteúdo cria a sessão na loja e conta zero eventos.

Regras por evento:

- **Conteúdo vazio** (após remover espaços nas pontas): descartado.
- **Mensagem de usuário** (papel `user` ou `human`): vira uma observação de prompt de usuário (evento `user-prompt`), com o texto original. Corpo: `session_id`, `prompt`, `occurred_at`.
- **Mensagem de outro papel, chamada de ferramenta e resultado de ferramenta**: vira uma observação de extensão com o rótulo `ai-memory-backfill` e o evento `backfill.<origem>` (parâmetros de URL `extension=ai-memory-backfill` e `source_event=<origem>`). A origem é `tool_call`, `tool_result`, `<papel>-message` ou `message` (quando não há papel). O corpo traz `session_id`, `title`, `message` (o texto completo) e `occurred_at`. O título é a primeira linha, aparada e cortada em 120 caracteres. Se ficar vazio, o título é `(backfilled event)`.
- **Compactação, checkpoint e anotação**: descartados. Não são conteúdo da sessão.
- **Política de captura por evento** (a mesma tabela do hook ao vivo, aplicada pelo cliente; o servidor nunca recebe os padrões de `ignore_paths`). Cada chamada de ferramenta é reconstruída no formato de payload do agente de origem e inspecionada pela política do diretório; cada resultado herda a decisão da chamada a que responde (pelo identificador de chamada nativo; sem identificador, a decisão **mais estrita** de todas as chamadas desde a última mensagem de usuário; órfão: descartado se há política ativa, senão registrado sem conteúdo). Decisões, da mais frouxa à mais estrita: **manter** (conteúdo como gravado) < **desconhecida** (ferramenta não reconhecida: registra só que rodou, corpo `title: "tool unknown"`, `message: "tool_family: unknown"` mais `\noutcome: unknown` em resultados) < **só metadados** (corpo de metadados da política, com `occurred_at`, e o protocolo de captura que o servidor valida e reconstrói) < **descartar**. Mensagens são mantidas, exceto: (a) a saída de comando de shell do Claude Code (mensagem de usuário que abre com `<bash-stdout>`, `<bash-stderr>`, `<local-command-stdout>` ou `<local-command-stderr>`), que a captura ao vivo nunca recebe e é descartada; (b) tudo, se o diretório não for admitido. Um evento cujo registro nomeia o próprio diretório é julgado a partir dele: precisa passar o mesmo portão de admissão, entregar ao mesmo servidor e resolver ao mesmo projeto, senão é descartado; as políticas de outros diretórios também valem (vale a mais estrita), e uma chamada cujo destino de arquivo cai em outro diretório obedece à admissão, rota e política desse destino. Um comando de shell cortado pelo adaptador ou gravado como texto não analisável não pode ser casado: com política ativa é descartado.
- **Chave de ingestão** por evento: `recovery_` seguido dos 55 primeiros caracteres hexadecimais do SHA-256 de quatro campos (intervalo de recuperação, vazio no backfill comum; identificador de sessão; tipo do evento; id do evento), cada um precedido do seu comprimento em 8 bytes big-endian. A abertura e o fechamento usam o tipo `session-start`/`session-end` e o id `boundary`. A chave tem 64 caracteres alfanuméricos ou sublinhado, portanto **o servidor a aceita** e deduplica o replay (resultado `replayed`). Ver Idempotência.
- **Agente e sessão**: o tipo de agente é o do harness de origem, e o identificador de sessão é o identificador nativo. Assim, a sessão importada tem o mesmo identificador que a captura ao vivo usaria.
- **Origem do projeto**: o parâmetro `project_src=marker`, além de `workspace`, `project`, `agent`, `event`, `session_id` e `ingest_key`, vai na URL de cada item.

A contagem de "eventos" do relatório conta só os itens de conteúdo, sem abertura e fechamento.

### 5. Horário de ocorrência

- Cada evento usa o horário da própria transcrição. Valor ausente ou que não é RFC 3339 é tratado como ausente.
- Evento sem horário herda o horário válido mais próximo anterior. Eventos antes do primeiro horário válido herdam esse primeiro horário.
- A abertura usa o **menor** horário válido e o fechamento, o **maior**. Não se usa o primeiro e o último da lista, porque a transcrição pode não estar ordenada no tempo.
- Horários mais de 5 minutos à frente do relógio do servidor são descartados pelo servidor, em vez de confiados.

### 6. Envio em lotes e retentativas

- Os itens de uma sessão (abertura, conteúdo, fechamento) são enviados a `POST /hook/batch` em blocos de até **256 itens e 8 MiB** por requisição (cada item conta o JSON mais um byte; o bloco parte de 2 bytes), na ordem da transcrição. Um único item acima de 8 MiB faz a sessão falhar (`one sanitized hook item exceeds the recovery request byte limit`).
- A confirmação do servidor é validada item a item. Confirmação incoerente (índices fora de faixa ou fora de ordem, contagem que não bate com o prefixo, resultados em número diferente dos aceitos, índice de falha dentro dos aceitos) faz a sessão falhar (`invalid hook acknowledgement; keeping transcript for retry`). Cada resultado precisa ser `stored` ou `replayed`; `ignored_end` só vale para o item de fechamento; `resumed` e qualquer outro desfecho (por exemplo `dropped_*`) fazem a sessão falhar sem tentar de novo (``hook item <n> was not durably stored (<desfecho>); keeping transcript``). Um índice de falha informado pelo servidor também falha (``hook processing failed at item <n>; keeping transcript for retry``).
- Se o servidor aceitar só um prefixo **não vazio** do bloco, a sessão **falha** (``server acknowledged only <a> of <b> transcript items; keeping transcript for a safe retry``); não há reenvio do restante.
- Se nenhum item for aceito, a tentativa é repetida até **5 vezes seguidas**, com espera de 200 ms vezes o número da tentativa. Depois disso, a sessão falha (``server acknowledged none of a hook batch after <n> attempts (rate limited or saturated); rerun to resume``).
- Uma falha no meio deixa a sessão **parcialmente importada**. Não há retomada automática dentro da execução (ver Casos de borda); como as chaves agora deduplicam, uma nova execução com `--force` reenvia sem duplicar os itens já aceitos.
- Antes de ler a transcrição, o importador espera o esvaziamento da escrita do harness (melhor esforço, resultado ignorado).

### 7. Saída, aviso e código de saída

**Execução manual** (sem `--quiet` nem `--json`) imprime um resumo de uma linha:

> 📼 ai-memory imported N prior local session(s) (~M events) for workspace/project. [X session(s) failed.] [Y older session(s) skipped (import cap).] Opt out with AI_MEMORY_BACKFILL_ON_START=false.

- `N` é o número de sessões importadas. `M` é o número de eventos de conteúdo importados.
- `X` aparece quando há falhas.
- `Y` é o número de sessões **não importadas por causa do limite de eventos** (ver Limites). Essas são as sessões mais recentes do conjunto selecionado, pois a importação segue a ordem cronológica.

Mensagens fixas, nos outros casos (ordem de precedência: `--json`, depois `--quiet`, depois estas):

- Diretório não admitido: a mensagem da etapa 2b.
- Loja não vazia: `ai-memory: <ws>/<proj> already has captured sessions — nothing to backfill (use --force to import anyway).`
- Simulação: `ai-memory: would import <N> local session(s) into <ws>/<proj> (dry run).`
- Nenhum histórico (nenhuma sessão importada e nenhuma falha): `ai-memory: no local session history found to backfill for <ws>/<proj>.`
- Cada sessão que falha escreve na saída de erro `ai-memory: backfill of <harness> session <8 primeiros caracteres do id> failed: <erro>`, mesmo com `--quiet`.

**Execução automática** é destacada e silenciosa, como a memória automática do próprio harness. Não há resumo. Falhas de sessão são sempre gravadas no registro de log do processo de segundo plano, inclusive com `--quiet`.

**Código de saída.** Se alguma sessão selecionada falhar, o comando sai com código diferente de zero **depois de emitir o relatório** (a sentinela já foi gravada), com a mensagem `backfill failed to import <X> of <N> selected session(s)`. As demais sessões continuam sendo tentadas. Com `--json`, o relatório completo vai para a saída padrão mesmo com falhas. Erros anteriores à importação (escopo, localização das lojas, servidor/credencial, consulta de vazio) saem com erro sem relatório.

`--quiet` suprime o resumo, não as falhas.

### 8. Contrato do relatório JSON

Campos, nesta ordem:

| Campo | Significado |
|---|---|
| `workspace`, `project` | Escopo resolvido |
| `selected` | Sessões selecionadas, após `--max-sessions` |
| `imported_sessions` | Sessões importadas com sucesso |
| `imported_events` | Eventos de conteúdo importados, somando todas as sessões |
| `skipped_for_cap` | Sessões não tentadas por causa do limite de eventos |
| `failed_sessions` | Sessões com erro de importação |
| `skipped_non_empty` | `true` quando a loja já tinha sessões e nada foi importado |
| `skipped_not_admitted` | `true` quando o modo de captura da instalação não admite o diretório e nada foi importado |
| `dry_run` | `true` na simulação |

A saída é o JSON indentado desse objeto, todos os campos sempre presentes (contagens como inteiros, indicadores como booleanos).

### 9. Sentinela, portão de vazio e idempotência

**Sentinela.** Arquivo vazio em um subdiretório `backfill-state` dentro do diretório de dados local. O nome do arquivo é o hash SHA-256, em hexadecimal, do caminho do diretório de trabalho. Ela é gravada, sem efeito se já existir:

- na execução automática com o recurso desligado (exceto em simulação);
- na execução real com loja não vazia;
- ao fim de toda execução real que chega à importação, manual ou automática, mesmo quando algumas sessões falharam e mesmo com `--session` sem resultado.

Não é gravada na simulação, quando o diretório não é admitido, nem quando um erro ocorre antes da importação (escopo, perfil de servidor, conexão para a consulta de vazio, consulta com erro ou estouro dos 30 s). Nesses casos, a próxima abertura de sessão tenta de novo. O subdiretório é criado se faltar; falha ao gravar a sentinela é ignorada (só causa uma consulta redundante depois).

Como a sentinela é por caminho, o mesmo projeto aberto em outro diretório tem sua própria tentativa.

**Portão de vazio.** "Vazio" significa que a soma das contagens de sessões por agente, para o escopo, é zero.

- A consulta usa o filtro padrão de proprietário: conta só as sessões que o chamador pode ver. Em um servidor multiusuário, sessões de outros usuários não impedem a importação.
- Projeto inexistente no servidor (resposta 404 do escopo): conta como vazio.
- Acesso negado (403, típico de um membro comum em servidor multiusuário): o portão consulta a lista de sessões da API web, com limite de uma sessão e sessões abertas incluídas. Lista vazia é vazio. 404 com erro em JSON também é vazio. 404 sem corpo JSON é **erro**, para não importar duas vezes.
- Qualquer outro erro: a execução falha sem gravar a sentinela.

**Idempotência.**

- Uma segunda execução, automática ou manual sem `--force`, não importa nada. O relatório traz `skipped_non_empty`, e o número de sessões não muda.
- A primeira sessão gravada torna a loja não vazia para as execuções seguintes. Isso vale até para sessões sem conteúdo, que criam a sessão.
- **Disparos automáticos concorrentes são evitados pelo marcador "em andamento"** (§1). Execuções **manuais** não usam o marcador, e duas execuções (manual e automática, ou duas manuais) que consultem a loja vazia ao mesmo tempo ainda podem ambas importar; a deduplicação por chave (abaixo) é a rede de proteção.
- **As chaves de ingestão do backfill são aceitas pelo servidor.** O servidor só aceita chaves de 1 a 64 caracteres alfanuméricos, hífen ou sublinhado; as do backfill (`recovery_` + 55 hexadecimais = 64) cumprem isso e são derivadas de dados estáveis (sessão, tipo e id do evento), então a repetição do mesmo evento é reconhecida (`replayed`) e gravada uma vez. Isso vale também para `--force` sobre sessões já importadas e para os prompts reenviados por `finalize-session`. A proteção contra reimportação em massa continua sendo o portão de vazio e a sentinela.

### 10. Limites

| Limite | Valor | Comportamento |
|---|---|---|
| Sessões selecionadas | 25 (`--max-sessions`) | As mais recentes. Valor zero vira 1 |
| Sessões enumeradas por harness | 200 | As mais recentes. O atingimento do limite não é mostrado |
| Eventos de conteúdo no total | 50.000 | Verificado **antes de cada sessão**. A última sessão iniciada pode ultrapassar o limite. As sessões restantes não são tentadas e contam em `skipped_for_cap` |
| Itens por requisição | 256 | Igual ao limite do servidor |
| Bytes por requisição | 8 MiB | Um item sozinho acima disso falha a sessão |
| Título do evento | 120 caracteres | Primeira linha, aparada |
| Tentativas sem aceite | 5 | Espera de 200 ms vezes o número da tentativa |
| Consulta de vazio | 30 s | Estouro é erro, sem sentinela |
| Marcador "em andamento" | 30 min | Depois disso é órfão e pode ser tomado |

A ordem de importação é cronológica entre as selecionadas. Por isso, quando o limite de eventos é atingido, **as sessões que ficam de fora são as mais recentes** do conjunto. Não há limite por sessão, nem limite total em bytes, nem registro de truncamento nas perdas (ver Propostas).

Limites de tamanho por evento específicos desse caminho: a confirmar. O limite de corpo por requisição do servidor existe, mas não foi conferido para o caminho de lote.

### 11. Segurança

- **Texto retroativo é tratado como texto de captura ao vivo.** Cada observação passa pela sanitização do ingresso de captura antes de ser gravada. Um segredo plantado numa transcrição de teste não aparece no estado persistido nem nos resultados de consulta de memória. Esse comportamento é verificado por um teste de canário, que roda só em sistemas POSIX.
- **Credenciais.** A execução automática não usa credencial de ambiente. A manual usa a configuração.
- **Exclusões de captura (`ignore_paths`) e modo de captura se aplicam ao backfill**, pelo cliente (§4): o diretório precisa ser admitido, e cada chamada e resultado de ferramenta segue a mesma tabela de decisão do hook ao vivo. Eventos de ferramentas que tocam caminhos excluídos viram "só metadados" ou são descartados; o servidor não recebe os padrões.
- **Sobrescrita.** O portão de vazio e a sentinela impedem que o automático toque em histórico de um projeto estabelecido. Só `--force` faz isso, de forma explícita.
- **Multiusuário.** O automático roda em qualquer postura de servidor. Não há restrição a instalações de operador único (ver Questões em aberto).

### 12. Configuração

- Chave **`backfill_on_start`**, no nível raiz da configuração, com padrão verdadeiro. Não fica dentro de uma seção de captura.
- Variável de ambiente **`AI_MEMORY_BACKFILL_ON_START`** sobrescreve a chave. O valor documentado para desligar é `false`.
- O desligamento afeta só a execução automática. O comando manual ignora o desligamento e sempre roda.
- Opt-out por marcador de projeto: **proposta**, não implementada (ver Propostas).
- O modelo padrão de configuração não lista a chave (a confirmar).

### 13. Cobertura de harness

Os mesmos harnesses que o diagnóstico lê, ou seja, o conjunto de adaptadores de transcrição somente leitura (13 tipos; ver [support-matrix.md](support-matrix.md)). Um harness cuja loja está ausente ou ilegível contribui com zero sessões, sem erro. O backfill não inventa histórico que não consegue ler.

### 14. Casos de borda

- **Projeto sem sessões locais**: nada selecionado. O relatório informa que não há histórico. A sentinela é gravada.
- **Sessão só com metadados**: cria a sessão na loja e conta zero eventos.
- **`--session` com identificador inexistente**: nada importado. Em execução real, a sentinela é gravada.
- **Erro ao resolver o escopo**: sai sem gravar a sentinela. A cada início de sessão, um novo processo é disparado até o escopo ser resolvido.
- **Interrupção no meio da importação** (processo encerrado): a sentinela não é gravada, mas a primeira sessão já gravada torna a loja não vazia. A execução automática seguinte não reimporta nada, e a sessão parcial fica como está.
- **Falha no meio de uma sessão**: a mensagem de erro pede para "rerodar para retomar", mas a rerodagem sem `--force` é barrada pelo portão de vazio, porque a sessão parcial já existe. A retomada exige `--force`; os itens já aceitos são reconhecidos pelas chaves (`replayed`) e não duplicam.
- **Diretório não admitido pelo modo de captura**: nada importado, servidor não contatado, sem sentinela (a próxima abertura tenta de novo, mas o hook também não dispara nada nesse modo).
- **Marcador "em andamento" órfão** (processo morto): bloqueia novos disparos por até 30 minutos.
- **Harness com mais de 200 sessões**: são consideradas as 200 mais recentes daquele harness.
- **Timestamps futuros ou inválidos** na transcrição: tratados conforme a seção de horário de ocorrência.

## Alternativas de caminho de ingestão

- **(A) Endpoints de execução gerenciada (descartado).** Era o caminho recomendado no plano original, pela leitura incremental por cursor. Um teste ao vivo mostrou que esse caminho alimenta o registro de continuidade do workstream e não o pipeline pesquisável: a contagem de observações ficou em zero, e o histórico não aparecia em consulta. Esse resultado é relatado; não há teste automatizado para ele.
- **(B) Replay pelo ingresso de captura ao vivo (adotado).** Usa a mesma ingestão e a mesma sanitização da captura ao vivo, e passa pela consolidação normal. Custo: não há cursor incremental, e a idempotência por chave não funciona (ver Idempotência).

O cursor incremental era o principal motivo para preferir (A). Com (B), ele vira item de proposta.

## Propostas (não implementadas)

1. **Trava de importação única por projeto no servidor**, no padrão de reivindicação única de handoff e mensagens. Hoje a sentinela cobre a repetição e o marcador "em andamento" cobre o disparo automático concorrente, mas não duas execuções manuais simultâneas (a deduplicação por chave as torna inofensivas).
2. **Cursor incremental por sessão**, persistido ao fim da importação, para que a leitura ao vivo continue depois do ponto importado sem capturar duas vezes. A não-sobreposição depende da premissa de que a captura ao vivo começa após a instalação.
3. **Verificação de vazio no handshake de início de sessão**, para não disparar um processo quando a loja já tem histórico.
4. **Aviso do resultado na sessão seguinte**, pelo mesmo caminho de entrega de avisos de início de sessão. Hoje o resumo só aparece na execução manual.
5. **Opt-out por marcador de projeto** (`.ai-memory.toml`), para desligar o automático em um projeto específico, independentemente da configuração global.
8. **Retomada de sessão parcialmente importada** (hoje exige `--force`, sem duplicar graças às chaves).
9. **Limite por sessão, limite total em bytes e registro de truncamento** nas perdas e no aviso.
10. **Modo sem limite de sessões** (`--all-sessions`). Hoje o padrão já é importar até o limite.
11. **Restringir o automático** em servidor com operadores distintos (ver Questões em aberto).
12. **Limite de conteúdo por evento** específico do backfill, em vez de depender do caminho de captura.

## Relação com o diagnóstico

As duas funções são complementares. O **diagnóstico detecta** a lacuna de cobertura (um harness rodou aqui e não capturou nada). O **backfill preenche** a instância específica da primeira inicialização. O diagnóstico é a checagem contínua, e o backfill é o bootstrap de uso único. A documentação de ambos deve fazer referência cruzada (ver [cli.md](cli.md)).

## Testes

**Existentes:**

- **Unidade:** seleção das mais recentes dentro do limite; limite zero vira um; caminho da sentinela por diretório; codificação de nomes no caminho de membro; mapeamento de prompts de usuário e de eventos de ferramenta; resolução de horário (preenchimento para frente e para trás, horário inválido, mínimo e máximo fora de ordem, sem nenhum horário); descarte de eventos vazios e de fronteira; descoberta de uma sessão plantada para o diretório; autenticação da execução automática; opt-out sem nenhuma requisição ao servidor.
- **Integração:** a simulação não grava a sentinela; a primeira execução importa uma sessão (0 para 1 sessão); a segunda não importa nada; o canário não aparece na consulta nem no estado persistido. Esses testes usam o layout POSIX de transcrições e rodam só em sistemas Unix.
- **Histórico de Antigravity:** sem histórico, a execução conta uma sessão e zero eventos. Com histórico, conta só os prompts do próprio projeto.

**Não cobertos hoje:**

- limite de eventos totais (não há teste);
- concorrência entre duas inicializações (não há trava, nem teste);
- sobreposição com cursor (o cursor não existe);
- exclusões de captura no backfill;
- falha no meio de uma sessão e a retomada.

## Questões em aberto (resolvidas na implementação)

1. **Caminho A ou B.** Resolvido para **B**, pelo teste ao vivo descrito em Alternativas.
2. **Servidor compartilhado.** Foi enviado **ligado por padrão**, com desligamento pela configuração ou pela variável de ambiente. Não há opção de linha de comando para isso. O portão de vazio limita o risco em qualquer postura de servidor.
3. **Entrega do aviso.** O automático é silencioso. O resumo aparece só na execução manual. O aviso na sessão seguinte é proposta.
4. **Qualidade da consolidação.** Foi enviado com o limite (25 sessões e 50 mil eventos) e está em ajuste. Uma avaliação dedicada fica como seguimento.

## Versionamento

Mudança aditiva: novo comando, nova chave de configuração e comportamento padrão ligado, sem quebra de compatibilidade. Classificação: versão menor. O recurso foi introduzido na versão 2.3.0.
