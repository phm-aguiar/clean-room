# Ingestão de eventos de ciclo de vida

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

> **Papel:** ponto de entrada do servidor para os eventos que os agentes (Claude Code,
> Codex, OpenCode, Cursor, Gemini CLI e outros) emitem em seus hooks de ciclo de vida.
> Converte um JSON heterogêneo em um evento normalizado, aplica política de captura e
> de controle de carga, sanitiza o texto antes de qualquer gravação, grava observações de
> forma idempotente e, no fim da sessão, sintetiza a página de sessão (sem LLM), cria o
> handoff automático e registra um commit. Também atende o endpoint síncrono de início de
> sessão e a API de workstreams gerenciados.

Este documento descreve o lado do servidor e a política compartilhada. O lado cliente
(comandos de hook executados no processo do agente, spool local e drenagem) está em
[cli.md](cli.md).

---

## 1. Fluxo de ponta a ponta

```text
Agente -> hook nativo (cliente) -> spool local -> POST /hook/batch  (ou POST /hook?event=...)
                                                       |
                                                       v
 1. remove o campo bruto da mensagem final do assistente (sempre)
 2. normaliza o JSON em evento (parsing tolerante)
 3. recusa o evento se os aliases do marcador de projeto forem inválidos       -> 202, descartado
 4. aplica o excerto do assistente (somente com o opt-in duplo)
 5. política de captura: Keep / Drop / MetadataOnly                             -> Drop: 202
 6. descarta o rascunho reservado do Cursor                                     -> 202, descartado
 7. descarte de subagente (somente se o projeto optou)                          -> 202, descartado
 8. limite global de processamento simultâneo (cheio)                           -> 429
 9. limite por fonte (sem token)                                                -> 429
10. responde 202 "queued" e processa em segundo plano
```

**Garantias de projeto**

- O hook nunca bloqueia o agente: o cliente tem timeout curto (um segundo, no hook de shell).
- O servidor responde **202 imediatamente** ou **429** quando saturado. Não há fila ilimitada.
- O processamento pesado ocorre depois da resposta, mas o evento só é considerado
  persistido depois que a escrita no armazenamento confirma (índices gravados na mesma
  transação do dado).
- Falhas no processamento em segundo plano não alteram a resposta já enviada no caminho
  de evento único. Para lotes, a falha aparece no reconhecimento (seção 2.2).

---

## 2. Rotas e contratos de resposta

| Rota | Método | Função |
|---|---|---|
| `/hook` | POST | Um evento: parâmetros de consulta + corpo JSON. |
| `/hook/batch` | POST | Lote de até **256** itens `{url, body}`; confirmações parciais. |
| `/handoff` | GET | Contexto síncrono de início de sessão (handoff, bloco de contexto, aviso de caixa de entrada). |

Antes destas rotas, o middleware de autenticação identifica o ator e o nível de
autorização. A lista de webhooks a pular, enviada no cabeçalho `X-Memory-Skip-Admission-Chain`,
é lida antes do processamento em segundo plano.

### 2.1 Parâmetros de consulta de `/hook`

`event` (obrigatório), `agent`, `cwd`, `workspace`, `project` (sobrepõe o marcador
`.ai-memory.toml`), `session_id` (fallback quando o corpo não traz), `project_strategy`
(`basename` ou `repo-root`), `project_src` (`marker` ou `repo-root`), `identity`,
`identity_src`, `identity_style`, `aliases`, `extension` e `source_event` (vocabulário de
terceiros), `drop_subagent`, `default_global`, `managed_run`, `capture_assistant`,
`ingest_key` e `all_owners`.

### 2.2 Reconhecimento de lote (`/hook/batch`)

A resposta traz:

- `accepted`: prefixo contínuo confirmado, mantido por compatibilidade com drenagens antigas.
- `accepted_indices`: índices confirmados, quando não coincidem com o prefixo.
- `failed_index`: item que falhou no processamento, quando houver.
- `results`: desfecho de cada item confirmado.

Regras:

- Item descartado por política, inválido, rascunho do Cursor, subagente, colisão de sessão
  ou autorização negada é **confirmado** (o cliente o remove do spool). Colisão de sessão é
  terminal e não é retentada.
- Item sem identificador de sessão que não seja início de sessão nunca pode ser gravado.
  É confirmado e descartado, para não bloquear os itens seguintes em retentativas.
- Item recusado pelo limite por fonte é **pulado** e a varredura continua. Ele não é
  confirmado e permanece no spool.
- **Falha de processamento interrompe o lote** (fail-fast). A resposta é HTTP 200 com
  `failed_index`, e os itens posteriores não são processados.
- Saturação global devolve **429** com os índices já aceitos. Os itens restantes são
  contados como descartados por saturação.
- Lote com mais de 256 itens devolve **413** sem processar nada (`accepted` = 0).

### 2.3 Respostas de evento único

- **202 "queued"**: evento aceito para processamento.
- **202** com motivo (`invalid marker aliases`, `capture policy dropped`,
  `cursor draft placeholder dropped`, `subagent capture dropped`): descarte reconhecido,
  sem consumir capacidade.
- **429** `hook queue full`: limite global atingido.
- **429** `hook source rate limited`: limite por fonte atingido.

### 2.4 Resposta de `/handoff`

- **200** com o texto de contexto.
- **200** com corpo vazio quando não há nada a entregar ou quando a busca falha (a falha é registrada).
- **403** com o motivo quando a identidade ou o escopo do repositório são recusados. Esse
  erro nunca é injetado como contexto do agente.

---

## 3. Evento normalizado

### 3.1 Vocabulário de eventos

Eventos canônicos: início de sessão, prompt do usuário, pré-ferramenta, pós-ferramenta,
pré-compactação, pós-compactação, notificação, parada (`Stop`), fim de sessão, início de
subagente, fim de subagente e outro.

Nomes aceitos (as formas em kebab-case, snake_case, PascalCase e os dialetos dos harnesses
são todas reconhecidas):

| Canônico | Exemplos de nomes aceitos |
|---|---|
| Início de sessão | `session-start`, `SessionStart`, `sessionStart` |
| Prompt do usuário | `user-prompt`, `UserPromptSubmit`, `beforeSubmitPrompt` |
| Pré-ferramenta | `pre-tool-use`, `PreToolUse`, `BeforeTool` |
| Pós-ferramenta | `post-tool-use`, `PostToolUse`, `postToolUseFailure`, `AfterTool` |
| Pré-compactação | `pre-compact`, `PreCompact`, `PreCompress` |
| Pós-compactação | `post-compaction`, `PostCompaction` |
| Notificação | `notification`, `Notification` |
| Parada | `stop`, `Stop` |
| Fim de sessão | `session-end`, `SessionEnd`, `sessionEnd` |
| Início de subagente | `subagent-start`, `SubagentStart`, `subagentStart` |
| Fim de subagente | `subagent-stop`, `SubagentStop`, `subagent-end`, `SubagentEnd` |
| Outro | qualquer outro nome |

Eventos de subagente e os de "outro" são armazenados com o tipo de observação "outro".
Os demais mantêm o tipo de mesmo nome.

### 3.2 Importância da observação

| Evento | Importância |
|---|---|
| Início e fim de sessão | 7 |
| Prompt do usuário | 8 |
| Pré e pós-ferramenta | 5 |
| Parada, pré-compactação, pós-compactação | 6 |
| Notificação, outro, início e fim de subagente | 3 |

### 3.3 Extração tolerante

Nenhum campo é obrigatório no parsing. A obrigatoriedade do identificador de sessão é
aplicada depois (seção 8).

- **Agente.** Se o corpo traz a marca de versão do Cursor, o agente é Cursor, mesmo que o
  hook tenha sido instalado como Claude Code. Caso contrário, vale o parâmetro `agent`.
- **Identificador de sessão.** Procurado nas chaves de sessão do corpo (`session_id`,
  `sessionId`, `sessionID`, `session`, `conversationId`) e em caminhos aninhados comuns
  dos harnesses. Se nada for encontrado, usa o parâmetro de consulta. Quando o identificador
  é uma string qualquer (não um UUID), ele é convertido em UUID versão 5, no namespace OID,
  de forma determinística. Assim, POST e GET da mesma sessão usam a mesma chave.
- **Diretório (`cwd`).** Procurado em `cwd`, `current_dir`, `working_dir`, `directory`, no
  primeiro item das listas `workspacePaths` e `workspace_roots` (Cursor) e em caminhos
  aninhados. **O valor do corpo prevalece sobre o da consulta.**
- **Título e corpo.** Para eventos de ferramenta dos agentes de esquema fechado (seção 3.5),
  só entram a família da ferramenta e o identificador da chamada. Argumentos nunca entram.
  Para os demais eventos, título e corpo vêm dos campos indicados na seção 3.4.
- **Horário original (`occurred_at`).** Lido **somente do topo do corpo**. Vale se for
  positivo e no máximo cinco minutos à frente do relógio do servidor. Caso contrário, usa
  "agora". Não passa pela sanitização. É usado pela importação retroativa.
- **Chave de ingestão (`ingest_key`).** Aceita se tiver de 1 a 64 caracteres entre
  letras ASCII, dígitos, `-` e `_`. Se for inválida, é ignorada. Nunca gera erro 4xx: o
  evento segue como entrega "pelo menos uma vez".
- **Subagente.** Um evento é marcado como subagente quando o topo do corpo traz `subagentType`
  ou `agent_id` como string não vazia.
- **Representação de depuração.** Ao registrar o evento em log de depuração, o payload bruto,
  o título e o corpo aparecem como `<redacted>`. Um dump de depuração nunca vaza prompt
  ou conteúdo de ferramenta.

### 3.4 Título e corpo por evento (caminho genérico)

| Evento | Título | Corpo |
|---|---|---|
| Início de sessão | `model`, depois `title` | nenhum |
| Prompt do usuário | `prompt`, `message` ou `text` (aceita blocos de conteúdo `[{type:text,text}]`) | o mesmo campo |
| Pré e pós-ferramenta | `tool`, `tool_name` ou `name`; ou `toolCall.name`; ou `step <n>` | só no pós-ferramenta: `tool: <nome>` + separador + resultado, onde o resultado vem de `tool_response`, `tool_output`, `output` ou `result`; senão `error`; senão `(no output captured)` |
| Notificação | `message` ou `text` | o mesmo campo |
| Pós-compactação | `summary` | `summary` |
| Demais (parada, pré-compactação, fim de sessão, subagentes) | nenhum | nenhum |

Se o evento vier de uma extensão (campos `extension` e `source_event`), o título e o corpo
usam as pistas da extensão como alternativa.

### 3.5 Agentes de esquema fechado

Para os agentes cujo esquema de ferramenta é fechado (Claude Code, CommandCode, Codex,
Cursor, OpenCode, Pi, Omp, AntigravityCli, Grok, Hermes, Pool, Zcode, CopilotCli e
Grizzybot), eventos de pré e pós-ferramenta seguem estas regras:

- **Título**: `tool <família>`, onde a família é `file`, `search-list`, `non-file` ou
  `unknown`.
- **Corpo**: `tool_family: <família>`, mais `tool_call_id` quando existir e for válido.
  No pós-ferramenta, acrescenta `outcome: success | error | unknown` e, se a família não
  for `unknown`, o separador e a saída. Quando não há saída, o texto é `(no output captured)`.
- **Nome e identificador da chamada** vêm de campos específicos de cada agente. Por exemplo,
  `tool_name` e `tool_use_id` para Claude Code e agentes parecidos, `tool` e `callID` para
  OpenCode, Pi e Omp, `toolCall.name` para AntigravityCli. Sem identificador válido, a chamada
  segue sem ele.
- **Desfecho do pós-ferramenta**: para Pi e Omp, vem de um campo booleano de erro. Para
  AntigravityCli e Zcode, de um campo de erro textual.
  Para CopilotCli e Hermes, de campos de resultado próprios. Nos demais, o desfecho é sempre
  `unknown`. Isso é intencional, porque no Codex o pós-ferramenta também dispara em comandos
  que falharam.
- **Caso especial**: no OpenCode, o pós-ferramenta usa o caminho legado, com o nome em `tool`
  e o resultado nos campos de saída comuns.

Os demais agentes (por exemplo, Gemini CLI, Kimi Code e Devin) usam o caminho genérico da
seção 3.4.

### 3.6 Limites de corpo

O corpo durável tem um teto aplicado **depois** da sanitização:

| Evento | Limite |
|---|---|
| Prompt do usuário e pós-compactação | 16 KiB |
| Notificação, pré e pós-ferramenta, parada, outro | 2 KB |
| Demais (início e fim de sessão, pré-compactação, subagentes) | 16 KiB |
| Excerto do assistente (seção 5) | 2 KB |

O corte respeita limites de caractere UTF-8. O cliente corta antes do envio, e o servidor
repete o corte.

---

## 4. Política de captura

A política responde: **este evento de ferramenta de arquivo (ou de comando de shell) deve ser
mantido, descartado ou reduzido a metadados?** É configurada pela lista de caminhos
ignorados (`[capture] ignore_paths`) do marcador `.ai-memory.toml` mais próximo. Ver
[marker-file.md](marker-file.md#capture-exclusions).

### 4.1 Modos

- **Denylist** (padrão): sem marcador, tudo é capturado.
- **Allowlist**: sem marcador, **nada** é capturado. A regra vale para qualquer evento,
  inclusive prompts, e é aplicada pelo cliente antes de qualquer spool.

### 4.2 Limites

| Limite | Valor |
|---|---|
| Padrões de ignorar por marcador | 128 |
| Tamanho de cada padrão | 1024 caracteres |
| Tamanho do marcador | 64 KiB |
| Tamanho de cada caminho candidato | 4096 caracteres |
| Caminhos candidatos por chamada | 32 |
| Orçamento de comparações por chamada | 1 000 000 |

Se o orçamento de comparações for excedido em um caminho de arquivo, o resultado é
`MetadataOnly` (fail-safe). Em comandos de shell, o resultado é `Drop` (seção 4.4).

### 4.3 Estado do marcador e decisão

O marcador é lido por quem faz a E/S, e a política recebe um de três estados:

- **Inativa** (sem marcador, em modo denylist): `Keep`.
- **Inválida** (marcador malformado): para ferramenta de arquivo ou comando de shell,
  `MetadataOnly`. Para as demais, `Keep`. Um marcador quebrado não prova que o argumento de
  um comando de shell evita os caminhos ignorados, então falha fechado.
- **Ativa**:
  - busca ou listagem: `Drop`;
  - arquivo com caminho(s) que casam com um padrão: `Drop`;
  - arquivo sem caminho extraível ou com caminho malformado: `MetadataOnly`;
  - arquivo sem casamento: `Keep`;
  - comando de shell: `Drop` se algum argumento casa com um caminho ignorado, `Keep` se nenhum casa;
  - demais famílias: `Keep`.

A decisão **nunca guarda** argumentos brutos, caminhos ou nomes arbitrários. Guarda apenas
família da ferramenta, quantidade de caminhos e identificador válido da chamada.

Para um evento reduzido a metadados, o corpo substituto contém: identificador de sessão,
diretório, família, nome canônico da ferramenta, identificador da chamada e a marca de
protocolo.

### 4.4 Protocolo cliente e servidor

- O cliente anexa a marca de protocolo (versão 1) quando aplica a política.
- O servidor **reinspeciona** o evento. Se o cliente pediu `Drop`, o servidor responde 202 e
  conta como "descartado pela política", sem gastar capacidade. Essa checagem protege contra
  clientes antigos.
- Um evento descartado reconhecido **nunca** entra em spool, fila, transporte, log ou
  armazenamento.

---

### 4.5 Descarte de subagente

Só vale para projetos que optaram por descartar capturas de subagente (opção `drop_subagent`
no marcador do projeto).

- Um evento marcado como subagente (seção 3.3) ou um evento de início ou fim de subagente é
  descartado com 202.
- Para agentes que não são Claude Code, a sessão marcada entra num conjunto de sessões de
  subagente (limite de 4096). Qualquer evento **sem marcador** dessa sessão também é
  descartado, inclusive o final dela. O fim de sessão remove a sessão do conjunto.
- Claude Code não marca a sessão pai. Só os eventos marcados são descartados.

---

## 5. Captura da mensagem do assistente

O texto final do assistente é útil, mas sensível. Por isso exige **opt-in duplo**: o servidor
precisa ter `capture_assistant` ligado (padrão: desligado) **e** o cliente precisa ter sido
instalado com captura do assistente.

- **Campo bruto.** Só existe no evento de parada de Claude Code, Codex e OpenCode, no campo
  `last_assistant_message` do topo do corpo.
- **Remoção incondicional.** O campo bruto é removido do JSON antes de qualquer outra etapa.
  Nunca chega a log, spool, rede ou armazenamento, independentemente da versão do cliente.
- **Lado cliente.** Com a captura ligada, o cliente sanitiza e corta o texto em no máximo
  2000 bytes, e anexa a marca `_ai_memory_assistant` (versão 1). Entradas acima de 64 KiB
  contam como ausentes.
- **Lado servidor.** O servidor consome a marca, sanitiza de novo com os próprios padrões e
  só então preenche o corpo da parada. Se qualquer portão falhar, o corpo fica vazio e a
  resposta continua 202.
- **Uso.** O excerto alimenta o resumo do handoff automático (seção 9). Não entra na página
  de sessão.

---

## 6. Estado compartilhado, limites e caches

| Item | Valor e comportamento |
|---|---|
| Processamento simultâneo global | 1024 por padrão. Acima disso, 429. |
| Gates de ingestão por (projeto, chave) | Serializam reentregas sobrepostas do mesmo evento. Entradas mortas são removidas de forma oportunista ao chegar a 4096. |
| Limite por fonte | Balde de fichas por chave. **Desligado por padrão** (taxa 0 em `hook_rate_per_sec`); liga com taxa positiva. Quando ligado, o tamanho de cada chave é limitado a 128 bytes (acima disso, usa hash) e o número de chaves a 4096. A chave mais antiga sai primeiro. |
| Chave do limite por fonte | Com identificador de sessão: ator + sessão. Sem identificador: impressão digital do evento (agente, tipo, diretório, sobreposições e estratégia de projeto) + hash do corpo. |
| Cache de resolução diretório para projeto | 4096 entradas. É só otimização: entradas removidas são resolvidas de novo. |
| Sessões marcadas como subagente | Conjunto limitado a 4096. |
| Lote | 256 itens por requisição. |
| Proteção de sessão viva no handoff | Janela de 10 minutos. Ver seção 11.2. |

Campos de estado compartilhado: o ponteiro de projeto ativo (também usado pelo servidor de
ferramentas), a opção de consolidação ao fim da sessão (padrão desligada), a opção de criar
handoff ao fim da sessão (padrão ligada), a opção de reivindicar handoff no início de
sessão (padrão ligada) e o modo de roteamento de meio de sessão.

---

## 7. Resolução de projeto e sessão pegajosa

### 7.1 Ordem de resolução

1. Sem diretório e sem sobreposição: usa o projeto padrão do servidor.
2. Consulta o cache pela combinação de diretório, espaço de trabalho, projeto, estratégia e
   identidade.
3. Nome do projeto: a sobreposição do marcador ou, na falta dela, o nome derivado do diretório
   pela estratégia. Estratégia `basename` usa o nome da pasta. Estratégia `repo-root` usa a raiz
   do repositório principal. **O nome deriva do caminho cru**, sem normalizar maiúsculas
   e minúsculas, para que uma mesma pasta não vire dois projetos em sistemas que não
   diferenciam essas letras.
4. O caminho do repositório só é gravado se for a **raiz de um repositório git real**. Nunca
   grava o diretório atual nu (ele poderia transformar a pasta pessoal do usuário em um
   projeto que engole todos os outros).
5. Se o nome for o reservado `_global`, a resolução é **recusada** e o projeto padrão é usado.
   Captura nunca vai para o escopo global reservado.
6. Sem sobreposição: escolhe o projeto cujo caminho registrado é o **prefixo mais longo** do
   diretório. Um subdiretório cai no projeto pai em vez de criar fragmento.
7. Cria o espaço de trabalho e o projeto quando necessário.

### 7.2 Sessão pegajosa

Para um evento de sessão já existente, o escopo da própria sessão é a referência. Um
`cd` para um subdiretório, em um projeto sem git, não deve fragmentar a sessão.

- Um marcador explícito sempre vence.
- O diretório do evento precisa estar **dentro** da subárvore do diretório da sessão.
- Sessão enraizada na raiz do sistema de arquivos ou na pasta pessoal nunca "gruda".
- O modo `[routing] mid_session` decide o resto:
  - `follow-cwd` (padrão): só gruda dentro da subárvore da sessão, ou fora dela quando a
    estratégia é `repo-root` e não há sobreposição.
  - `sticky`: também gruda fora da subárvore, para qualquer estratégia, desde que a sessão
    tenha uma âncora significativa. Pode substituir uma sobreposição derivada da raiz do
    repositório, mas **nunca** um marcador explícito.

---

## 8. Processamento de um evento

1. **Identificador de sessão.** Se houver identificador nativo, ele é usado. Se for uma string
   qualquer, vira UUID versão 5 (seção 3.3). Sem identificador, o evento só é válido como
   início de sessão, e nesse caso um identificador novo é gerado.
2. **Chave de ator.** Combina o usuário (chave de armazenamento) e o identificador de sessão
   cru. A chave segue a mesma convenção do servidor de ferramentas.
3. **Escopo.** Sessão pegajosa (seção 7.2) ou resolução completa (seção 7.1).
4. **Dono do registro.** Só é preenchido quando o servidor distingue operadores.
5. **Filtro de dono.** Um dono específico ou "não atribuído". A opção "todos os donos" só vale
   em fim de sessão e exige capacidade de administrador. Sem ela, a tentativa vira colisão
   de sessão.
6. **Admissão.** No máximo uma nova tentativa. Monta a sessão e a observação (com a importância
   da seção 3.2), **sanitiza o texto** (fronteira de privacidade), pega o gate de
   (projeto, chave de ingestão) e grava sessão e observação em **uma única transação**. Se a
   referência de escopo estiver obsoleta, invalida o cache, resolve de novo e tenta uma vez.
7. **Desfecho da admissão.**
   - Fim de sessão inválido (sem início ou com escopo incompatível): ignorado com log.
   - Sessão já encerrada: recuperação. Faz o commit, reenfileira a consolidação (se a sessão
     não for efêmera) e marca a chave como concluída.
   - Observação nova, fim de sessão aberto ou reabertura: segue.
   - Já concluído: retorna sem efeito.
   - Efeitos pendentes de uma entrega anterior: retoma os efeitos sem nova observação.
8. **Vínculo de execução gerenciada.** Se o identificador de execução for válido, vincula a
   sessão. Se o vínculo for recusado (outro projeto ou outro operador), registra o motivo.
9. **Projeto ativo.** Início de sessão, prompt e pré-ferramenta avançam o ponteiro
   compartilhado. Eventos de conclusão só atualizam a entrada exata do ator, para que uma
   entrega atrasada do spool não redirecione uma sessão nova.
10. **Log.** Acrescenta uma linha ao log do projeto (seção 13).
11. **Pré e pós-compactação.** Faz checkpoint (seção 8.2).
12. **Fim de sessão e checkpoint de turno.** Ver seções 8.3 e 8.4.
13. **Conclusão.** A chave de ingestão só é marcada como concluída **no final**. Até aí, os
    efeitos são "pelo menos uma vez" e podem ser retomados.

### 8.1 Gravação de sessão

O identificador de sessão e a observação são gravados juntos (passo 6). Se a mesma
entrega chegar de novo com a mesma chave, o servidor não duplica a observação.

### 8.2 Checkpoints (pré-compactação, pós-compactação)

- Não encerram a sessão e não criam handoff.
- Se o agente já escreveu a página de sessão, essa página é **mantida**, e o checkpoint não a
  sobrescreve.
- Se a consolidação por LLM estiver configurada, ela é tentada primeiro:
  - resposta vazia: nada é escrito;
  - falha do **provedor** (recusa, limite de taxa, indisponibilidade, resposta não mapeável):
    cai para a página sintetizada sem LLM;
  - falha de **política ou infraestrutura** (webhook de admissão recusou o ator ou o escopo,
    falha de armazenamento, sessão sem resolução): **não** cai para o modo sem LLM. Falha
    fechado. A operação de admissão de consolidação continua valendo para a página de fallback.
    Em qualquer caso, o evento segue e o erro é registrado.
- Se o provedor não for usado, ou depois do fallback, a página é sintetizada sem LLM e
  escrita em `sessions/<identificador>.md`. Sessão efêmera não gera página.
- O commit usa o rótulo do checkpoint: `pre-compact(session <id8>): checkpoint` ou
  `post-compaction(session <id8>): checkpoint`. `<id8>` são os oito primeiros caracteres do
  identificador.

### 8.3 Fim de sessão

**Sessão efêmera** (nenhuma observação de prompt do usuário, pré-ferramenta ou pós-ferramenta):

- A sessão é encerrada **sem página, sem handoff e sem consolidação**.
- Faz commit com o título `lifecycle-only session <id8>`.
- Se a gravação descobrir trabalho substantivo concorrente, segue o caminho normal abaixo.
- A definição de efêmera deve ser idêntica no cálculo do servidor e na verificação do
  armazenamento. Se divergirem, o veredito se perde.

**Sessão substantiva:**

1. Sintetiza a página de sessão (seção 10) e grava em `sessions/<identificador>.md`, com
   autoria atribuída ao **dono da sessão**, não a quem entregou o evento. Se o agente já
   tiver escrito essa página, ela é mantida.
2. Handoff automático só é criado se **todas** as condições valerem:
   - a sessão não for gerenciada;
   - a sessão não for uma sessão filha de subagente no OpenCode;
   - a opção `create_on_session_end` estiver ligada (padrão: ligada).
   Antes da escrita, consulta a admissão da operação de início de handoff. Se for recusada, a
   sessão termina **sem** handoff, e o restante continua.
3. Encerra a sessão e grava o handoff na **mesma transação**, para que uma falha no meio não
   deixe uma sessão encerrada sem sucessor.
4. Notifica observadores somente se o handoff foi gravado.
5. Faz commit com o título `session <id8>: <título com até 60 caracteres>`.
6. Enfileira a consolidação por LLM somente se a opção `consolidate_on_session_end` estiver
   ligada (padrão: desligada) e houver consolidador configurado.
7. Avisa o trabalhador de perfil.

### 8.4 Checkpoint de turno (OpenCode)

O OpenCode mantém o serviço vivo após o término da CLI. Um turno completo pode, portanto,
publicar um checkpoint de continuação **sem encerrar a sessão nativa**. Condições, todas
obrigatórias:

- agente OpenCode;
- evento de parada com o campo booleano `turn_checkpoint` igual a verdadeiro;
- sem vínculo de execução gerenciada;
- não é subagente.

Efeitos:

- Atualiza a página de sessão e o handoff de continuação. A sessão continua aberta.
- Não enfileira consolidação por LLM.
- Se a sessão for efêmera ou não estiver mais aberta, não escreve nada.
- Se a admissão do handoff for recusada, mantém a página e a sessão viva.
- O texto do handoff de checkpoint usa a última solicitação do usuário (seção 9).

---

## 9. Handoff automático (sem LLM)

### 9.1 Resumo

- **Um prompt**: `Session focused on: <prompt>`.
- **Vários prompts**: `Started: <primeiro>` + linha em branco + `Last: <último>`.
- **Nenhum prompt**: `Session ended; <N> observations recorded.` Para checkpoint de turno,
  o início muda para `Turn checkpoint; <N> observations recorded.`
- Se existir um excerto do assistente, acrescenta `Latest assistant response: <excerto>`.
- Cada texto de prompt é limitado a **1500 caracteres**. O corte acrescenta `…`.
- Blocos de markup entregues como turno do usuário (por exemplo, notificações do próprio
  harness) não contam como prompt.

### 9.2 Próximos passos

`Tools used: <nomes>`: nomes distintos das ferramentas, em ordem alfabética. Rótulos de família
(`tool file`, `tool non-file`, ...) são descartados, porque não dizem qual ferramenta foi usada.

### 9.3 Perguntas em aberto

Em handoff automático de fim de sessão, a primeira regra que casar vence:

1. O último prompt do usuário (sem blocos de markup do harness) termina com `?` ou `？`:
   `Unresolved question: <prompt>`.
2. A última ferramenta foi da família de arquivo e a sessão não tem nenhum evento de parada:
   - `Session ended without a normal stop while working with files`
   - `Check the working tree for uncommitted or unverified changes`
3. Há parada, mas não há observação de fim de sessão: `Continue from (mid-task exit): <prompt>`.
4. Fim normal: `Continue from: <último prompt substantivo>`.
5. Nenhuma das anteriores: lista vazia.

Em **checkpoint de turno**, a regra é só uma: `Continue from last request: <último prompt
substantivo>`, ou nada.

**Prompt de confirmação** (ignorado como pergunta de continuação): vazio, ou uma frase curta
de confirmação com até 20 caracteres, sozinha ou seguida só de pontuação. Exemplos:
`ok`, `thanks`, `got it`, `好的`, `谢谢`. Frases mais longas são mantidas.

---

## 10. Página de sessão sintetizada (sem LLM)

Caminho: `sessions/<identificador-da-sessão>.md`. Camada: episódica.

Frontmatter:

- `title`: título da página (regra abaixo).
- `session_id`: identificador da sessão.
- `agent`: origem da sessão, imutável. Não é quem escreveu a página.
- `tier`: `episodic`.
- `summary` (opcional): resumo de uma linha, presente só se houver o que dizer.

### 10.1 Título

Ordem de preferência:

1. O primeiro prompt do usuário com título não vazio e que não seja texto de apoio do harness
   (contexto de IDE, prompt de shell ecoado).
2. Preâmbulo de subagente (`You are a subagent spawned by another session.`): usa a primeira
   linha útil do corpo como título. Se não houver linha útil, o título é `Session <id>`, e
   o título **não** cai para outras observações.
3. Qualquer outro título útil de outro evento. Pulam o início de sessão, títulos iguais ao
   nome do tipo de evento e rótulos `tool <família>`.
4. Senão, `Session <identificador>`.
5. Só quando não há observação nenhuma, o título é o literal `session`.

### 10.2 Resumo

Uma linha de prosa, sem marcador de lista, sem cabeçalho e sem metadados em formato de
lista. Exemplo: `2 prompts, 3 completed tool calls across Bash and Edit, over 18m.`

- Contagem de prompts (no singular ou plural).
- Chamadas de ferramenta concluídas: **só pós-ferramenta** conta. Contar pré e pós duplicaria.
- As três ferramentas mais usadas são nomeadas, por volume, com desempate alfabético, seguidas de
  `and <K> more` quando houver mais. Rótulos de família são contados, mas não nomeados. Se só
  houver rótulos, a cláusula de nomes é omitida.
- Duração entre o início e o fim da sessão, quando maior que zero: `<n>s`, `<n>m`, `<h>h` ou
  `<h>h <m>m`.
- Se nada couber, o resumo é omitido.

### 10.3 Corpo

1. `# <título>`
2. `## Session metadata`: identificador, início e fim em UTC, número de observações.
3. `## Prompts`: lista numerada dos títulos dos prompts (se houver).
4. `## Tool calls`: contagem por nome (se houver).
5. `## Raw observations`: uma linha por observação, com tipo, horário UTC e título (até 80
   caracteres). Com até 500 observações, todas aparecem. Acima disso, aparecem as 250
   primeiras e as 250 últimas, com uma nota informando quantas foram omitidas no meio.

---

## 11. Handoff no início de sessão (`/handoff`)

### 11.1 Parâmetros

`agent`, `cwd`, `workspace`, `project`, `session_id`, `project_strategy`, além das opções de
contexto (brief e perfil). Se houver uma execução gerenciada no parâmetro correspondente, o
ledger dela também é entregue.

### 11.2 Ordem de operações

1. Resolve o projeto e exige autorização de leitura para o repositório. Recusa resulta em 403.
2. Lê o contexto da execução gerenciada, se houver. É aditivo.
3. Publica o projeto ativo, se houver dica de escopo.
4. Escolhe o handoff a entregar:
   - filtro de dono do ator (ou não atribuídos);
   - handoff **manual** vence sobre o **automático**;
   - automático só vale se o diretório do handoff for ancestral ou igual ao diretório atual, na
     fronteira de caminho (`/repo` vale para `/repo/api`, nunca para `/repo-other`);
   - **sessão viva é protegida**: um handoff de sessão que ainda está aberta e capturou algo nos
     últimos **10 minutos** não é oferecido. Assim, um terminal parecido, no mesmo diretório,
     não recebe o contexto de outra conversa em andamento;
   - sessão encerrada e handoff manual não sofrem esse filtro.
5. Contexto de brief (opt-in): recomposto a cada início, sem consumir nada.
6. Resumo de perfil, quando a requisição o solicita.
7. **Reivindicação do handoff** (uso único). Se a opção `claim_on_session_start` estiver ligada
   (padrão: ligada):
   - consulta a admissão de aceitação do handoff com limite de **750 ms**, abaixo do limite
     de um segundo do hook de shell. Recusa ou tempo esgotado deixam o handoff aberto e o
     restante é entregue;
   - só marca o handoff como aceito se essa etapa for bem-sucedida.

   Se a opção estiver desligada, o handoff **não é reivindicado**. Em vez disso, o contexto
   traz um aviso com identificador, origem e idade (somente metadados, nunca o conteúdo
   armazenado), para que o agente o aceite de forma deliberada.
8. **Gravação de aceitação em uma única transação**: reivindica o handoff e o contexto da
   execução gerenciada. Só acontece **depois** de todas as leituras e renderizações terem dado
   certo, porque um handoff é de uso único, e perder a saída seria perdê-lo. Se outra sessão
   ganhou a corrida, o handoff não é aceito.
9. Notifica observadores somente se o handoff foi de fato aceito.
10. Aviso de caixa de entrada: **somente a contagem** de mensagens pendentes. Nunca inclui texto
    controlado por quem enviou a mensagem.
11. Concatena, nesta ordem: resumo de perfil, handoff, aviso de handoff pendente, contexto de
    execução gerenciada, brief e aviso de caixa de entrada.

Todo conteúdo recuperado vem precedido de um aviso fixo: o conteúdo armazenado é **dado
histórico não confiável, não instruções**. Ele nunca deve ser executado, nem deve levar à
alteração de permissões ou políticas.

Agentes cujo stdout de início de sessão é descartado pelo harness **não devem** chamar este
endpoint, porque a reivindicação é destrutiva e o contexto se perderia.

---

## 12. API de workstreams gerenciados

Rotas, montadas ao lado de `/hook` e sob a mesma autenticação:

| Rota | Método | Função |
|---|---|---|
| `/workstream/runs` | POST | Prepara uma execução. |
| `/workstream/runs/{id}` | GET | Status da execução. |
| `/workstream/runs/{id}/heartbeat` | POST | Sinal de vida. |
| `/workstream/runs/{id}/cancel` | POST | Cancela. |
| `/workstream/runs/{id}/context` | POST | Obtém o contexto. |
| `/workstream/runs/{id}/context/accept` | POST | Aceita o contexto. |
| `/workstream/runs/{id}/link` | POST | Vincula uma sessão. |
| `/workstream/runs/{id}/finish` | POST | Finaliza com eventos. |
| `/workstream/recent` | POST | Lista workstreams recentes. |
| `/workstream/rename` | POST | Renomeia. |
| `/workstream/{id}/events` | GET | Busca no ledger. |

Autorização por capacidade (leitura ou escrita normal), verificada pelo nível de autenticação.

Limites:

| Item | Limite |
|---|---|
| Eventos por finalização | 4096 (acima disso, rejeita) |
| Conteúdo de cada evento | 64 KiB. O excedente é cortado e marcado como `[truncated by ai-memory]`. O conteúdo é sanitizado antes do corte. |
| Identificador de evento | de 1 a 512 bytes |
| Diretório informado | 16 KiB |
| Nomes | 256 bytes |
| Metadados lidos do armazenamento | acima de 16 KiB, são tratados como ausentes |
| Metadados na escrita | a confirmar |

O conteúdo é sanitizado antes de ser gravado no segmento bruto imutável, sob o diretório de
dados do servidor, e antes da indexação.

---

## 13. Log do projeto

Cada evento processado acrescenta uma linha ao arquivo mensal do projeto, na raiz do projeto
na wiki:

- Nome do arquivo: `log-AAAA-MM.md`, pelo horário do evento em UTC. Eventos de um mês
  vão para o arquivo daquele mês.
- Formato da linha: `## [AAAA-MM-DDTHH:MM:SSZ] <evento> | <título>`. O prefixo permite busca
  com ferramentas de texto (`^## \[`).
- O nome do evento vem em kebab-case (`session-start`, `user-prompt`, `pre-tool-use`,
  `post-tool-use`, `pre-compact`, `post-compaction`, `notification`, `stop`, `session-end`,
  `subagent-start`, `subagent-stop`, `other`).
- O título tem quebras de linha trocadas por espaços e é cortado em **120 caracteres**.
- Cada linha é anexada ao fim do arquivo. Em plataformas POSIX, escritas com anexação de
  menos de 4 KiB não se intercalam.
- Os arquivos de log são reservados e não são indexados como páginas da wiki.
- Não há varredura de retenção. O histórico cresce devagar, e cada arquivo mensal é pequeno.

---

## 14. Notas de implementação

1. **Separe em três camadas**: normalização tolerante (seção 3), política pura sem E/S
   (seção 4) e orquestração do processamento (seção 8). A política pura deve ser testável sem
   servidor.
2. **Contrato "202 ou 429, nunca bloquear"**, mais idempotência por chave e conclusão só ao
   final, dá semântica "pelo menos uma vez" segura e retomável.
3. **Sanitize antes de qualquer gravação**, por tipo: nenhum texto não sanitizado chega ao
   armazenamento. Trate todo texto recuperado como não confiável ao reinjetá-lo no contexto.
4. **Handoff é de uso único**: leia e renderize tudo **antes** de reivindicar, e reivindique
   com uma operação atômica.
5. **Mantenha a definição de sessão efêmera idêntica** entre o cálculo do servidor e a
   verificação do armazenamento.
6. **Ordem sugerida de adoção**: comece com quatro eventos (início de sessão, prompt, pós-
   ferramenta e fim de sessão) e a síntese sem LLM. O restante (política de captura, subagentes,
   captura do assistente, execuções gerenciadas) é incremental.
