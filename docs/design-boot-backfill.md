# Design: backfill de histórico local anterior ao hook

**Status: implementado na versão 2.3.0.** Este documento descreve o comportamento atual da importação única do histórico local de um projeto e separa o que existe do que é proposta. Itens marcados como **(proposta)** não existem no código. Itens marcados como **(descartado)** foram substituídos por outra abordagem.

## Resumo do estado

| Item | Estado |
|---|---|
| Gatilho automático no início de sessão, sem bloquear o início | Implementado |
| Importação só quando a loja do projeto está vazia (no escopo do chamador) | Implementado |
| Comando manual `backfill` com `--dry-run`, `--force`, `--session`, `--max-sessions`, `--json`, `--quiet` | Implementado |
| Importação por replay pelo ingresso de captura ao vivo | Implementado (caminho B) |
| Limites de sessões, de eventos totais e de tamanho de lote | Implementado (parcial, ver Limites) |
| Trava (lease) de importação única por projeto | Proposta. Substituída por sentinela local, sem trava de concorrência |
| Cursor incremental por sessão | Proposta |
| Verificação de vazio no próprio handshake de início de sessão | Proposta |
| Aviso do resultado na sessão seguinte | Proposta |
| Opt-out por marcador de projeto | Proposta |
| Aplicação de exclusões de captura (`ignore_paths`) ao backfill | Requisito, não implementado |
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

No início de sessão, quando nenhum dono externo de captura está declarado no ambiente, o hook verifica se existe a **sentinela local** do diretório de trabalho atual. Se não existir, dispara um processo de backfill destacado, sem herdar a entrada e saída do hook, e retorna sem esperar. O hook não consulta o servidor para isso e não importa nada.

Consequências:

- Uma vez por diretório de trabalho e por máquina, esse processo é iniciado **mesmo com o recurso desligado ou com a loja não vazia**. Ele sai sem importar e grava a sentinela.
- A sentinela só é gravada ao final da execução. Enquanto uma importação longa está em andamento, novos inícios de sessão no mesmo diretório disparam outros processos. Eles são barrados pelo portão de vazio assim que a primeira sessão importada está gravada (ver Idempotência).
- Uma falha ao disparar o processo não afeta o início de sessão.

### 2. Comando `backfill`

Fluxo de uma execução real:

1. **Opt-out automático.** Se a execução é automática e o recurso está desligado, grava a sentinela e sai, sem contatar o servidor.
2. **Escopo.** Resolve workspace e projeto (padrão: escopo do projeto atual; sobrescritos por `--workspace` e `--project`) e a localização das lojas nativas. Se isso falhar, sai com erro **sem gravar a sentinela**. A próxima abertura de sessão tenta de novo.
3. **Servidor.** Na execução automática, usa o mesmo destino do hook que a disparou: um perfil de servidor ou a URL de instalação do hook. A credencial é o token persistido do hook e, depois, OIDC. A credencial de ambiente ou de configuração **não** é usada nesse caminho. Na execução manual, o servidor vem da configuração.
4. **Portão de vazio** (ver Idempotência), salvo com `--force`. Se a loja não está vazia, grava a sentinela (exceto em simulação), gera o relatório com `skipped_non_empty` e sai.
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
- **Mensagem de usuário** (papel `user` ou `human`): vira uma observação de prompt de usuário (evento `user-prompt`), com o texto original.
- **Mensagem de outro papel, chamada de ferramenta e resultado de ferramenta**: vira uma observação de extensão com o rótulo `ai-memory-backfill` e o evento `backfill.<origem>`. A origem é `tool_call`, `tool_result`, `<papel>-message` ou `message` (quando não há papel). O corpo traz o texto completo. O título é a primeira linha, aparada e cortada em 120 caracteres. Se ficar vazio, o título é `(backfilled event)`.
- **Compactação, checkpoint e anotação**: descartados. Não são conteúdo da sessão.
- **Chave de ingestão** por evento: `<sessão>:<id do evento>`. A abertura usa `<sessão>:session-start` e o fechamento, `<sessão>:session-end`. Ver Idempotência: o servidor não aceita essas chaves.
- **Agente e sessão**: o tipo de agente é o do harness de origem, e o identificador de sessão é o identificador nativo. Assim, a sessão importada tem o mesmo identificador que a captura ao vivo usaria.
- **Origem do projeto**: é informada como marcador.

A contagem de "eventos" do relatório conta só os itens de conteúdo, sem abertura e fechamento.

### 5. Horário de ocorrência

- Cada evento usa o horário da própria transcrição. Valor ausente ou que não é RFC 3339 é tratado como ausente.
- Evento sem horário herda o horário válido mais próximo anterior. Eventos antes do primeiro horário válido herdam esse primeiro horário.
- A abertura usa o **menor** horário válido e o fechamento, o **maior**. Não se usa o primeiro e o último da lista, porque a transcrição pode não estar ordenada no tempo.
- Horários mais de 5 minutos à frente do relógio do servidor são descartados pelo servidor, em vez de confiados.

### 6. Envio em lotes e retentativas

- Os itens de uma sessão são enviados em blocos de até **256 itens** por requisição, o mesmo limite de lote do servidor, na ordem da transcrição.
- Se o servidor aceitar só um prefixo do bloco, o restante é reenviado.
- Se nenhum item for aceito, a tentativa é repetida até **5 vezes seguidas**, com espera de 200 ms vezes o número da tentativa. Depois disso, a sessão falha.
- Uma falha no meio deixa a sessão **parcialmente importada**. Não há retomada automática (ver Casos de borda).

### 7. Saída, aviso e código de saída

**Execução manual** (sem `--quiet` nem `--json`) imprime um resumo de uma linha:

> 📼 ai-memory imported N prior local session(s) (~M events) for workspace/project. [X session(s) failed.] [Y older session(s) skipped (import cap).] Opt out with AI_MEMORY_BACKFILL_ON_START=false.

- `N` é o número de sessões importadas. `M` é o número de eventos de conteúdo importados.
- `X` aparece quando há falhas.
- `Y` é o número de sessões **não importadas por causa do limite de eventos** (ver Limites). Essas são as sessões mais recentes do conjunto selecionado, pois a importação segue a ordem cronológica.

Mensagens fixas, nos outros casos:

- Loja não vazia: informa que o projeto já tem sessões capturadas e que nada será importado, com a sugestão de `--force`.
- Simulação: informa quantas sessões seriam importadas.
- Nenhum histórico: informa que não há histórico local para o projeto.

**Execução automática** é destacada e silenciosa, como a memória automática do próprio harness. Não há resumo. Falhas de sessão são sempre gravadas no registro de log do processo de segundo plano, inclusive com `--quiet`.

**Código de saída.** Se alguma sessão selecionada falhar, o comando sai com código diferente de zero **depois de emitir o relatório**. As demais sessões continuam sendo tentadas. Com `--json`, o relatório completo vai para a saída padrão mesmo com falhas.

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
| `dry_run` | `true` na simulação |

### 9. Sentinela, portão de vazio e idempotência

**Sentinela.** Arquivo vazio em um subdiretório `backfill-state` dentro do diretório de dados local. O nome do arquivo é o hash SHA-256, em hexadecimal, do caminho do diretório de trabalho. Ela é gravada, sem efeito se já existir:

- na execução automática com o recurso desligado;
- na execução real com loja não vazia;
- ao fim de toda execução real, manual ou automática, mesmo quando algumas sessões falharam.

Não é gravada na simulação nem quando um erro ocorre antes da importação (escopo, conexão para a consulta de vazio, consulta com erro). Nesses casos, a próxima abertura de sessão tenta de novo.

Como a sentinela é por caminho, o mesmo projeto aberto em outro diretório tem sua própria tentativa.

**Portão de vazio.** "Vazio" significa que a soma das contagens de sessões por agente, para o escopo, é zero.

- A consulta usa o filtro padrão de proprietário: conta só as sessões que o chamador pode ver. Em um servidor multiusuário, sessões de outros usuários não impedem a importação.
- Projeto inexistente no servidor (resposta 404 do escopo): conta como vazio.
- Acesso negado (403, típico de um membro comum em servidor multiusuário): o portão consulta a lista de sessões da API web, com limite de uma sessão e sessões abertas incluídas. Lista vazia é vazio. 404 com erro em JSON também é vazio. 404 sem corpo JSON é **erro**, para não importar duas vezes.
- Qualquer outro erro: a execução falha sem gravar a sentinela.

**Idempotência.**

- Uma segunda execução, automática ou manual sem `--force`, não importa nada. O relatório traz `skipped_non_empty`, e o número de sessões não muda.
- A primeira sessão gravada torna a loja não vazia para as execuções seguintes. Isso vale até para sessões sem conteúdo, que criam a sessão.
- **Não há trava entre execuções concorrentes.** Duas execuções que consultem a loja vazia ao mesmo tempo podem importar em duplicidade.
- **As chaves de ingestão do backfill não são aceitas pelo servidor.** O servidor só aceita chaves de 1 a 64 caracteres alfanuméricos, hífen ou sublinhado. Chave inválida é descartada sem erro, e a requisição segue como se não tivesse chave. Por isso, o servidor não deduplica o replay. A proteção contra reimportação é só o portão de vazio e a sentinela. Uma execução com `--force` sobre um projeto que já tem as sessões pode duplicar observações (a confirmar).

### 10. Limites

| Limite | Valor | Comportamento |
|---|---|---|
| Sessões selecionadas | 25 (`--max-sessions`) | As mais recentes. Valor zero vira 1 |
| Sessões enumeradas por harness | 200 | As mais recentes. O atingimento do limite não é mostrado |
| Eventos de conteúdo no total | 50.000 | Verificado **antes de cada sessão**. A última sessão iniciada pode ultrapassar o limite. As sessões restantes não são tentadas e contam em `skipped_for_cap` |
| Itens por requisição | 256 | Igual ao limite do servidor |
| Título do evento | 120 caracteres | Primeira linha, aparada |
| Tentativas sem aceite | 5 | Espera de 200 ms vezes o número da tentativa |

A ordem de importação é cronológica entre as selecionadas. Por isso, quando o limite de eventos é atingido, **as sessões que ficam de fora são as mais recentes** do conjunto. Não há limite por sessão, nem limite total em bytes, nem registro de truncamento nas perdas (ver Propostas).

Limites de tamanho por evento específicos desse caminho: a confirmar. O limite de corpo por requisição do servidor existe, mas não foi conferido para o caminho de lote.

### 11. Segurança

- **Texto retroativo é tratado como texto de captura ao vivo.** Cada observação passa pela sanitização do ingresso de captura antes de ser gravada. Um segredo plantado numa transcrição de teste não aparece no estado persistido nem nos resultados de consulta de memória. Esse comportamento é verificado por um teste de canário, que roda só em sistemas POSIX.
- **Credenciais.** A execução automática não usa credencial de ambiente. A manual usa a configuração.
- **Exclusões de captura (`ignore_paths`) não se aplicam ao backfill.** Este é um requisito do design e ainda não está implementado. Hoje as regras do ingresso ao vivo só são inspecionadas quando a requisição traz o marcador de captura, que o backfill não envia (a confirmar com teste).
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
- **Falha no meio de uma sessão**: a mensagem de erro pede para "rerodar para retomar", mas a rerodagem sem `--force` é barrada pelo portão de vazio, porque a sessão parcial já existe. A retomada exige `--force`, com o risco de duplicidade descrito em Idempotência.
- **Harness com mais de 200 sessões**: são consideradas as 200 mais recentes daquele harness.
- **Timestamps futuros ou inválidos** na transcrição: tratados conforme a seção de horário de ocorrência.

## Alternativas de caminho de ingestão

- **(A) Endpoints de execução gerenciada (descartado).** Era o caminho recomendado no plano original, pela leitura incremental por cursor. Um teste ao vivo mostrou que esse caminho alimenta o registro de continuidade do workstream e não o pipeline pesquisável: a contagem de observações ficou em zero, e o histórico não aparecia em consulta. Esse resultado é relatado; não há teste automatizado para ele.
- **(B) Replay pelo ingresso de captura ao vivo (adotado).** Usa a mesma ingestão e a mesma sanitização da captura ao vivo, e passa pela consolidação normal. Custo: não há cursor incremental, e a idempotência por chave não funciona (ver Idempotência).

O cursor incremental era o principal motivo para preferir (A). Com (B), ele vira item de proposta.

## Propostas (não implementadas)

1. **Trava de importação única por projeto**, no padrão de reivindicação única de handoff e mensagens. Hoje a sentinela cobre a repetição, mas não a concorrência.
2. **Cursor incremental por sessão**, persistido ao fim da importação, para que a leitura ao vivo continue depois do ponto importado sem capturar duas vezes. A não-sobreposição depende da premissa de que a captura ao vivo começa após a instalação.
3. **Verificação de vazio no handshake de início de sessão**, para não disparar um processo quando a loja já tem histórico.
4. **Aviso do resultado na sessão seguinte**, pelo mesmo caminho de entrega de avisos de início de sessão. Hoje o resumo só aparece na execução manual.
5. **Opt-out por marcador de projeto** (`.ai-memory.toml`), para desligar o automático em um projeto específico, independentemente da configuração global.
6. **Aplicação de `ignore_paths` no backfill**, para descartar eventos de ferramentas de arquivo que casem com as exclusões.
7. **Chaves de ingestão válidas para o replay**, ou um formato de chave aceito pelo servidor. Sem isso, não há deduplicação de reenvio.
8. **Retomada de sessão parcialmente importada.**
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
