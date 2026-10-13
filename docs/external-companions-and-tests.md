# Companions e testes fora do núcleo

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

> Este documento descreve os companions do ecossistema que ficam fora do servidor principal: a biblioteca de privacidade compartilhada, o relay de eventos, o exportador de wiki para repositório de time, o importador, o app de barra de menu para macOS, os harnesses de avaliação e a estrutura de testes de ponta a ponta. A política de fronteira está em [companions.md](companions.md).
>
> Os valores numéricos são os do momento da redação e podem mudar entre versões.

## 0. Princípio: companion não é núcleo

O servidor ai-memory é um **substrato de memória**: hooks, MCP, wiki em markdown, índice local, autenticação, webhooks de admissão e navegador somente leitura. Tudo que é "produto ao redor" (importar de outra ferramenta, entregar eventos de um orquestrador, exportar a wiki para um repositório de time, empacotar para macOS) vive em **pacotes separados**. Cada pacote tem **seu próprio conjunto de dependências e de versões travadas**, e conversa com o servidor somente por superfícies públicas.

Regras de integração (valem para qualquer companion):

| Pode | Não pode |
|---|---|
| Chamar as rotas `/api/v1/*` (somente leitura), as ferramentas MCP (`memory_write_page`, `memory_query`…), `/admin/write-page` e `/admin/delete-page` com token de operador, `/hook` e `/hook/batch` | Fazer parte do conjunto de pacotes do núcleo por padrão |
| Rodar suas próprias filas, prompts, UI, políticas | Escrever arquivos da wiki ou linhas do banco diretamente |
| Servir SPA alternativa via `--web-ui-dir` (exige a interface web habilitada) | Burlar a autorização por nível, os webhooks de admissão, a atribuição de ator, a resolução de escopo ou o escritor único |
| Ter CLI, imagem Docker, releases e docs próprios | Exigir plugin dentro do processo do servidor |

Se um companion expõe escrita via navegador, ele precisa de **seu próprio broker de mutação** (proteção contra falsificação de requisição entre sites, confirmação, auditoria e limite de taxa); o navegador fala com o companion, e o companion fala com o servidor com um token de operador.

Cada companion tem validação **separada** (formatação, lint e testes próprios). A suíte de testes do núcleo **não** os cobre, porque eles não são membros do conjunto de pacotes do núcleo. A CI tem um job `companions` que valida cada um.

```text
orquestrador/fonte externa
   │                                   ┌─ biblioteca de privacidade (compartilhada)
   ├─ relay ───────────────────────►   │        ▲                ▲
   │    fila local + /hook/batch       │        │                │
   ├─ importador ──────────────────►   │  (importador e relay usam as 4 operações)
   │    /hook/batch + /admin/write-page│
   └─ exportador da wiki ◄──────────── /api/v1 (somente leitura) ──► repositório do time
                                  servidor ai-memory
 app macOS ── serviço de login + barra de menu ── abre /web, status, configuração
```

---

## 1. Biblioteca de privacidade compartilhada

Pacote não publicado. É uma biblioteca de **quatro operações**, usada pelo relay e pelo importador para garantir que nada sensível seja persistido localmente ou enviado antes da sanitização do servidor atuar. É a **primeira** fronteira; a sanitização do núcleo continua sendo a segunda.

| Operação | Para que serve | Quem usa |
|---|---|---|
| Recusar dado sensível | Recusa (erro) credenciais e caracteres de controle **sem renomear** identidades. Aplica a sanitização a uma cópia do valor e falha se o resultado for diferente do original | relay (identidades nativas, vínculo); importador (identidades de origem, URLs de entrega) |
| Sanitizar texto de conversa | Redação de texto livre (regras na seção "Redação de texto" abaixo) | importador (antes de hash, corpo, prévia e manifesto) |
| Sanitizar valor | Redação recursiva de strings e de valores opacos sob chaves de credencial | relay (ingresso) e importador (planejamento do evento) |
| Validar corpo | Gate final: precisa ser **objeto**, ter até 256 KiB serializado, passar na recusa de dado sensível e **não pode trazer autoridade** | relay (antes de enfileirar e de entregar); importador (antes do hash) |

### Regras que importam

- **Autoridade no corpo é proibida.** A validação do corpo recusa estes campos no topo: `workspace`, `project`, `actor`, `author_id`, `headers`. Destino e ator vêm do vínculo da fila, das flags ou do token do servidor, nunca do payload.
- **Limites:** aninhamento de até 64 níveis (mais que isso é recusado); cada string, inclusive nomes de chave, de até 256 KiB em bytes.
- **Chave sensível.** O nome da chave é normalizado: uma letra maiúscula precedida de minúscula (ou de maiúscula seguida de minúscula) vira separador; todo caractere não alfanumérico vira separador; depois o nome é dividido em partes não vazias. A chave é sensível se alguma parte, sem o `s` final, estiver em: `authorization auth jwt pwd bearer password passwd secret token cookie credential apikey privatekey accesskey signature`; ou se houver a sequência `api`, `private`, `access` ou `session` seguida de `key` ou `keys`. O valor vira `"[REDACTED]"`, qualquer que seja o tipo dele.
- **Exceção para métricas de uso.** Um **número** (não string) sob um destes 11 nomes normalizados mantém o valor, porque contagem de tokens não é credencial: `max_tokens`, `input_tokens`, `token_count`, `token_usage`, `output_tokens`, `total_tokens`, `cache_read_input_tokens`, `cache_creation_input_tokens`, `prompt_tokens`, `completion_tokens`, `reasoning_tokens`. Além da lista, qualquer número cujo nome tenha mais de uma parte e termine em `tokens` também é métrica, **a menos que** uma parte anterior seja qualificador de credencial: `access`, `refresh`, `id`, `auth`, `authorization`, `bearer`, `session`, `api`, `private`, `client`, `oauth`, `secret`, `password`, `credential`, `cookie`. Um nome que seja só `tokens` não é métrica. String ou objeto sob essas chaves é **sempre** redigido.
- **Nomes de chave também são validados.** Um nome de chave que contenha credencial faz a operação falhar; ele não é redigido em silêncio.
- **Idempotência da redação:** marcadores completos de redação sobrevivem a varreduras repetidas, e o truncamento do importador nunca corta um marcador ao meio.
- **Ordem:** a validação de identidade nativa precede a sanitização. Bytes já pendentes na fila nunca são reescritos. Uma sessão com cabeça insegura adia só ela mesma.
- **Proveniência não é confiança:** um bloco de captura no corpo continua sendo dado não confiável; escopo, autorização e política de captura do servidor decidem.

#### Redação de texto

Aplicada em sequência, nesta ordem:

1. Caracteres de controle viram U+FFFD, exceto quebra de linha, retorno de carro e tabulação. Caracteres de controle de direção de texto (faixas bidi) também viram U+FFFD.
2. Bloco de chave privada (do cabeçalho `BEGIN … PRIVATE KEY` até `END … PRIVATE KEY`, em várias linhas) vira `[REDACTED PRIVATE KEY]`.
3. Tokens com prefixos conhecidos viram `[REDACTED CREDENTIAL]`, como palavra inteira: `github_pat_` seguido de 20 ou mais caracteres `[A-Za-z0-9_]`; `gh` seguido de uma letra entre `p`, `o`, `u`, `s`, `r`, depois `_` e 20 ou mais alfanuméricos; `sk-` seguido de 16 ou mais caracteres `[A-Za-z0-9_-]`; `AKIA` seguido de 16 caracteres `[0-9A-Z]`.
4. A palavra `Bearer` (sem diferenciar maiúsculas), seguida de espaço e de 8 ou mais caracteres `[A-Za-z0-9._~+/=-]`, mantém a palavra e troca o valor por `[REDACTED]`.
5. Atribuições de `api_key`, `api-key`, `apikey`, `access_token`, `auth_token`, `token`, `secret` ou `password` (palavra inteira, sem diferenciar maiúsculas), seguidas de `:` ou `=` com espaços opcionais, têm o valor trocado por `[REDACTED]` e o separador é preservado. Valores entre aspas com 6 ou mais caracteres entre elas, e valores sem espaço, vírgula ou ponto e vírgula, são redigidos. Valor com menos de 6 caracteres, e valor que já seja um marcador de redação, ficam como estão.

### Para reimplementar

A biblioteca é pequena. O valor está na *lista de chaves* e na regra "número de métrica passa, string de credencial não". Replique as listas exatamente se quiser compatibilidade de comportamento com o servidor.

---

## 2. Relay de eventos: entrega durável de ciclo de vida

**Problema:** um orquestrador (que lança agentes e observa seu ciclo de vida) precisa entregar eventos ao servidor **mesmo com o servidor fora do ar ou com o processo reiniciado**. O relay é uma **fila local em arquivo** com um cliente de `POST /hook/batch`. Ele **não** abre o banco nem a wiki do ai-memory, **não** lança agentes e **não** reivindica handoffs.

O contrato de captura (quem é dono da captura, chave de ingestão, variável `AI_MEMORY_CAPTURE_OWNER`) está em [complementary-contracts.md](complementary-contracts.md), seção 1.

### 2.1 Comandos

| Comando | Efeito |
|---|---|
| `init --queue-dir D --server-url U --producer P --actor A --workspace W --project X` | Cria o diretório da fila (o **pai** precisa existir) e grava o *vínculo* (uma linha). Repetir `init` idêntico é seguro; vínculo diferente é **rejeitado**. `actor` é só um namespace estável para derivar chaves: **não** seleciona nem imita usuário do servidor |
| `enqueue --queue-dir D --file ARQUIVO` | Valida o array **inteiro** e grava em **uma transação** (tudo ou nada). Item: `{event_id, agent, event, body}`; `body` é objeto com `session_id` e `cwd` explícitos |
| `flush --queue-dir D [--max-batches 64]` | Entrega via `/hook/batch`. Token só por `AI_MEMORY_AUTH_TOKEN` (nunca armazenado nem impresso). Um flush por vez por fila: um segundo flush concorrente falha de imediato, sem esperar |
| `status --queue-dir D` | JSON sem conteúdo de eventos: pendentes (itens, bytes, sessões), itens expirados, recibos e desfechos (10 chaves fixas, com zeros). Também traz tentativas, idade do pendente mais antigo, sessões conhecidas, limites e janela de retry |

Códigos de saída: `0` ok (flush sem pendências) · `2` falha (mensagem em stderr; nada não reconhecido é perdido) · `3` flush terminou **com eventos ainda pendentes**.

### 2.2 Identidade e chave de retry

A chave de ingestão é o hash SHA-256, em hexadecimal (64 caracteres), de uma lista JSON compacta de strings, nesta ordem: `external-capture-v1`, produtor, ator, agente, id da sessão, evento de origem, id do evento.

- O **conteúdo do payload não entra na chave**: um produtor reiniciado deriva a mesma chave. `event_id` é escolhido pelo produtor (texto livre limitado, ex.: `run/123/event/2`).
- Reenfileirar a mesma identidade com o mesmo corpo (a ordem das chaves JSON não importa) é reconhecido enquanto o item ou o recibo existir. A mesma identidade com **conteúdo diferente rejeita o arquivo inteiro**. Dois corpos iguais com `event_id` diferentes são eventos distintos.
- Limites de campo: produtor e ator são tokens de até 64 caracteres; evento de origem é token de até 128; `event_id` tem de 1 a 128 caracteres, não vazio após aparar espaços, sem controle; agente é token de até 64; workspace e projeto têm de 1 a 128 caracteres; id da sessão de 1 a 256; cwd de 1 a 4096; corpo até 256 KiB. "Token" = letras ASCII, dígitos, `.`, `_`, `-`, `:`.
- Cada item segue para o servidor como `{url, body}`. A URL é `/hook` com estes parâmetros, nesta ordem: `event`, `agent`, `workspace`, `project`, `extension` (= produtor), `source_event` (= evento), `ingest_key`. O id da sessão **não** vai na URL, porque já está no corpo. `extension` e `source_event` são **ambos** necessários para preservar o namespace em eventos reconhecidos.
- `session-end` é o único **evento terminal**.

### 2.3 Esquema da fila

Versão 2, com modo de escrita antecipada (WAL), sincronização completa e chaves estrangeiras ligadas.

| Tabela | Conteúdo |
|---|---|
| Metadados | pares chave/valor: identidade fixa da fila e versão do esquema |
| Vínculo (uma linha) | endereço do servidor, produtor, ator, workspace, projeto, instante de criação |
| Pendentes (uma linha por evento) | sequência de chegada (autoincremental); chave de ingestão (única); id do evento; agente; evento; id da sessão; cwd; corpo canônico; hash do corpo; tamanho do corpo; instante do primeiro registro; instante da primeira tentativa (anulável); número de tentativas; último erro. Índice por agente, sessão e sequência |
| Recibos | chave de ingestão (primária); hash do corpo; instante da primeira tentativa; instante da confirmação; desfecho (anulável). Recibos protegem contra reenvio mesmo após a entrega |
| Sessões | id da sessão (primária); agente; instante da última vez vista. Detecta uma sessão nativa trocando de agente dentro de uma fila |

Migração e compatibilidade:

- Migração v1→v2: adiciona a coluna de desfecho ao recibo, em **uma transação de escrita imediata** que re-checa a versão sob o bloqueio de escrita. Migração interrompida faz rollback e pode ser repetida.
- Binários antigos **recusam** a fila v2. *Downgrade* exige restaurar um backup anterior ao upgrade, com todos os usuários da fila parados.
- Esquema desconhecido é recusado.
- Após a abertura, a espera por banco ocupado ou travado é repetida dentro de 10 s, e a identidade e a versão são re-checadas a cada tentativa.

### 2.4 Algoritmo de entrega (flush)

1. Abre a fila, lê o vínculo, valida workspace e projeto com a mesma regra de escopo, e remove recibos e sessões fora da janela de 30 dias.
2. Lê o token do ambiente (ver 2.6) e monta o cliente.
3. Repete até `max_batches` (padrão 64) ou até o orçamento total de 900 s:
   - a. Seleciona o lote: até 256 itens e até 8 MiB de custo de transmissão (ver 2.8). Sessões já adiadas neste flush são puladas.
   - b. Para cada item, revalida identidade, corpo e URL com a biblioteca de privacidade. Se a revalidação falhar (só possível em itens enfileirados antes da sanitização local), o item é descartado localmente com desfecho `dropped_policy`, gera recibo, e a sessão segue.
   - c. Registra a tentativa de cada item (instante da primeira tentativa, se ainda não houver, e incremento do contador) **antes** do POST, em operação durável.
   - d. Envia o POST, com retries de transporte (ver 2.8):
     - erro de transporte: registra falha nos itens, tudo fica pendente, encerra;
     - status diferente de 200 e de 429: tudo fica pendente. 401 e 403 indicam problema de token (a mensagem muda conforme havia token ou não). 413 indica lote grande demais. Os demais viram "status inesperado". Encerra;
     - corpo do ack ilegível: tudo fica pendente, encerra.
   - e. Valida o ack inteiro (2.4.1). Se for inconsistente, tudo fica pendente e encerra.
   - f. Confirma os itens aceitos com seus desfechos: remove da fila e cria o recibo.
   - g. Se houve `failed_index`, registra falha nesse item.
   - h. Para os itens não aceitos: se houve `failed_index` e o item está **depois** dele, continua elegível neste flush. Caso contrário (a própria cabeça que falhou, ou itens anteriores limitados por taxa), a sessão é adiada até o fim deste flush.
   - i. Status 429: aplica o ack parcial e encerra o flush (o chamador faz backoff).
4. Relatório final: confirmados, desfechos, backoff, descartes locais, sessões adiadas e sessões retidas por expiração. `status` e `flush` imprimem apenas contagens.

Pontos de projeto que explicam o algoritmo:

- **Uma cabeça por sessão por lote** torna segura uma confirmação *parcial*: dois eventos da mesma sessão nunca estão em voo juntos. Nenhum subconjunto do ack pode confirmar o evento 2 com o 1 ainda pendente, e um `session-end` nunca ultrapassa um evento anterior.
- **Tentativa antes do POST, sem reiniciar o relógio:** reinício, resposta perdida ou retry nunca "reiniciam" a janela de 30 dias. As chaves de ingestão do servidor expiram em 30 dias (**a confirmar** no núcleo; a janela do relay espelha esse valor). Itens expirados **não são reenviados automaticamente** e **não são descartados**: o comando não tem operação de remoção.
- **Ack validado por inteiro antes de liberar qualquer item.**
- **Desfechos:** 10 valores fixos: `stored`, `replayed`, `resumed`, `ignored_end`, `dropped_policy`, `dropped_subagent`, `dropped_unauthorized`, `dropped_collision`, `dropped_invalid`, `unknown`. Servidor antigo sem resultados por item, ou texto de desfecho futuro, vira `unknown`. **Descarte também é confirmação terminal**: fila drenada não prova que tudo virou observação. Um recibo mantém seu primeiro desfecho conhecido; `unknown` ou nulo legado pode ser preenchido depois.

#### 2.4.1 Validação do ack

- Os índices aceitos, se informados, são estritamente crescentes, sem duplicatas, e cada um é menor que o tamanho do lote.
- `accepted` é obrigatório e precisa igualar o tamanho do prefixo contíguo `0, 1, 2…` de `accepted_indices`. Sem `accepted_indices`, valem os índices `0` até `accepted − 1`, e `accepted` não pode passar do tamanho do lote.
- `failed_index`, se informado, é menor que o lote, não pode estar entre os aceitos, e **nenhum aceito pode vir depois dele** (o servidor falha rápido e não processa além).
- `results`, se informado, precisa casar **exatamente** com os índices aceitos, em ordem.
- Qualquer inconsistência faz o lote inteiro permanecer pendente, com a classe de falha "ack inconsistente".

### 2.5 Limites da fila

| Recurso | Limite |
|---|---|
| Eventos pendentes | 50 000 |
| Bytes pendentes de corpo | 64 MiB |
| Pendentes + recibos retidos | 200 000 |
| Identidades de sessão retidas | 50 000 |
| Um corpo | 256 KiB |
| Um arquivo de entrada | 32 MiB e 10 000 eventos |
| Um lote HTTP | 8 MiB e 256 eventos (custo inclui URL e enquadramento) |
| Corpo do ack | 64 KiB |

Atingir um limite **rejeita a entrada nova**, inteira. Nunca descarta eventos antigos.

### 2.6 Rede, arquivos e segurança

- URL do servidor: http ou https, com host; **sem** credenciais, query ou fragmento. Barra final do caminho é removida (suporta servidor atrás de prefixo). **Redirecionamentos são recusados.** Não há lista de hosts permitidos: servidor HTTPS remoto é implantação legítima.
- Token: somente pela variável `AI_MEMORY_AUTH_TOKEN`, lida a cada flush. Espaços nas bordas são removidos; valor vazio equivale a sem token; caractere de controle é erro. O valor nunca é impresso, nem quando rejeitado.
- Arquivos (sistemas Unix): diretório novo com permissão `0700`; arquivos com `0600`. Diretório existente precisa já ser privado e dedicado à fila: arquivo estranho dentro dele é erro. O pai do diretório precisa existir. Caminho com `..` é recusado. Links simbólicos e pontos de reparse do Windows são recusados, assim como arquivos da fila com mais de um hard link.
- Windows: o relay **não** define ACL. Use um diretório restrito à conta.
- O produtor aplica as exclusões de captura (`ignore_paths`, na configuração de captura) **antes** de enfileirar. O relay não carrega a política do projeto nem inspeciona payloads de ferramenta por caminhos sensíveis. A sanitização do servidor não protege uma fila local que já contém o dado.
- Backup ou mover a fila: pare produtores e flushers; mantenha o arquivo do banco e seus auxiliares de journal juntos; nunca edite tabelas para "marcar como entregue"; **não recrie a fila** para contornar uma entrega ambígua, porque isso perde o histórico de tentativas e os recibos.
- Relógio do host importa: a janela de retry usa tempo de parede.

### 2.7 Teste ponta a ponta

O script de smoke test sobe um servidor ai-memory real e isolado, mais o relay, e verifica: enfileiramento com o servidor fora do ar e entrega posterior (3 eventos pendentes antes da entrega); retry após resposta perdida; identidades distintas (mesmo corpo com `event_id` diferente conta como evento distinto); **15 sessões concorrentes**; entrega de handoff com captura nativa suprimida; autenticação recusada; colisões de sessão reconhecidas; e asserções de recibo (armazenado, replay, descarte por colisão). Diretório e logs temporários são mantidos, e o caminho é impresso.

### 2.8 Algoritmos em detalhe (para reimplementar o relay)

**Validação de um item de entrada.** Nesta ordem. Qualquer erro recusa o **arquivo inteiro** e cita só o índice e o `event_id`, nunca conteúdo do corpo.

1. Recusa de dado sensível sobre: `event_id`, agente, evento, produtor e ator.
2. Recusa de dado sensível sobre as identidades que o corpo declare (id da sessão, cwd, produtor, ator, id do evento, agente, evento, chave de ingestão). Um segredo numa identidade é recusado, **nunca** renomeado.
3. Sanitização de valor e validação do corpo (seção 1).
4. `event_id`: 1 a 128 caracteres, não vazio, sem controle. Agente: token de até 64. Evento: token de até 128.
5. O corpo precisa ser objeto com id da sessão (texto, 1 a 256) e cwd (texto, 1 a 4096), **explícitos**.
6. Serializa o corpo em JSON compacto e exige até 256 KiB. Hash do corpo = SHA-256 hexadecimal desse JSON. Chave de ingestão pela fórmula de 2.2. O JSON é serializado com **chaves em ordem lexicográfica**, então a ordem das chaves no arquivo de entrada não altera o hash. Uma reimplementação deve ordenar as chaves (ou canonicalizar) antes de calcular o hash.

**`enqueue`** (uma transação de escrita imediata; tudo ou nada):

```text
contadores: itens e bytes pendentes, recibos, sessões registradas
para cada evento do arquivo:
  agente vinculado = agente já registrado para a sessão, ou o visto neste mesmo arquivo
  se não há vínculo: conta uma sessão nova; se passar de 50 000 → ERRO "tabela de sessões cheia"
  se há vínculo e é diferente do agente do evento → ERRO "conflito de identidade de sessão"
  se a mesma chave já apareceu neste arquivo com hash diferente → ERRO colisão
  conhecido = hash de pendente ou de recibo com a mesma chave
  se conhecido: hash diferente → ERRO colisão; igual → "reconhecido" (não reinsere)
  senão: soma 1 item e o tamanho do corpo
         se itens > 50 000 ou bytes > 64 MiB → ERRO "fila cheia" (nada é descartado)
         se recibos + itens > 200 000 → ERRO "proteção contra replay cheia"
         insere como pendente, com primeiro registro = agora
no fim: registra ou atualiza cada sessão do arquivo com o instante de última vista
```

O espaço do recibo é reservado já no enqueue, porque todo pendente vira recibo ao ser confirmado. Contar só recibos deixaria a fila prometer mais proteção do que o limite.

**Seleção do lote:** percorre os pendentes em ordem de chegada e mantém um conjunto de pares (agente, sessão) já vistos. Só a **primeira** (mais antiga) de cada sessão entra. Sessão adiada neste flush é pulada sem alterar a ordem. Item cuja primeira tentativa já completou 30 dias conta como sessão retida, e é pulado (a sessão fica bloqueada). Custo do item = tamanho da URL + tamanho do corpo + 32 bytes de enquadramento. Um item que estoure o orçamento de 8 MiB, com o lote já não vazio, é adiado, e a varredura continua. Limite de itens: o menor entre o pedido e 256.

**Expiração:** um item está expirado quando `agora − primeira tentativa ≥ 30 dias`. O próprio limite já conta como expirado.

**Registro de tentativa:** em operação durável própria, grava o instante da primeira tentativa (só se ainda não houver, para não reiniciar o relógio) e incrementa o número de tentativas.

**Confirmação:** para cada chave aceita, lê o hash, a primeira tentativa e o primeiro registro do pendente. Cria ou atualiza o recibo: instante de entrega = agora; desfecho = o novo, exceto quando o recibo já tem desfecho conhecido. Depois remove o pendente. A primeira tentativa do recibo é a do pendente, ou o primeiro registro se nunca foi tentado (descarte local).

**Registro de falha:** grava o último erro, com no máximo 80 caracteres e sem controle. As classes são rótulos curtos: `transporte`, `http <código>`, `ack malformado`, `ack inconsistente`, `falha de índice informada pelo servidor`. **Nunca** entram corpo do servidor nem payload.

**Poda** (no início de todo flush): apaga recibos com primeira tentativa anterior a `agora − 30 dias`, e registros de sessão sem pendentes e com última vista anterior a `agora − 30 dias`.

**Envio com retries:**

- Até 2 retries após a primeira tentativa. A espera antes de cada retry é `250 ms × 2^tentativa`, com teto de 2 s: 250 ms antes do primeiro retry e 500 ms antes do segundo.
- Antes de cada tentativa, verifica dois prazos: o restante do orçamento do flush (900 s no total) e o restante da janela de 30 dias do item mais antigo do lote. Se esse item cruzou os 30 dias durante o flush, não reenvia.
- Timeout de cada POST = menor valor entre 2 s × número de itens, 120 s e o restante do orçamento (nunca abaixo de 1 ms).
- Timeout de conexão: 10 s. Cliente **sem redirecionamentos**.
- Corpo do ack lido até 64 KiB. Acima disso é erro de transporte, e portanto é retentado.

**Normalização da URL do servidor:** http ou https, com host, sem userinfo, query nem fragmento; barra final do caminho removida. **URL de cada item:** base + `/hook?` + os 7 parâmetros de 2.2, em formulário URL-encoded. O corpo do item é o JSON do evento.

**Arquivo de entrada:** lido até 32 MiB. Array com até 10 000 eventos. Arquivo maior é erro antes de processar.

---

## 3. Exportador de wiki para repositório de time

**Objetivo:** deixar a memória compartilhada do time como **markdown revisável em controle de versão**. O exportador puxa famílias de páginas explicitamente permitidas via `/api/v1` (somente leitura) para um diretório dentro de um repositório de projeto. É a primeira fase de um plano maior. **Nunca** escreve no servidor e **nunca** executa comandos de controle de versão.

### 3.1 Uso

```text
plan   --server URL --workspace W --project P --dest ./wiki --include _rules --include decisions
export --server URL ... --apply
```

- `plan` nunca escreve, nem o arquivo de estado. `export` sem `--apply` continua sendo dry-run. `export --apply` escreve ou atualiza e **imprime** os comandos de adição, commit e envio que você pode executar.
- `--include FAMÍLIA` é allowlist **estrita** de diretórios de topo. Ao menos uma é obrigatória. `*`, `?`, `[` e `]` são recusados. Cada família precisa ser um nome de diretório portável.
- Servidor: padrão `http://127.0.0.1:49374`, sobrescrevível por `--server` ou pela variável `AI_MEMORY_SERVER_URL`.
- Auth: `--token` ou `AI_MEMORY_AUTH_TOKEN`. O token nunca aparece na ajuda, em log ou em arquivo. `--force` (só em `export`) sobrescreve arquivos divergentes.

### 3.2 O que escreve

Exatamente a projeção canônica do servidor (caminho, título e corpo), com o corpo **verbatim**, que já traz o título `#`. Nunca forja frontmatter de atribuição, de geração ou de sincronização.

Toda a contabilidade local fica em **um** arquivo de estado, dentro de um diretório oculto no destino. Ele tem permissão `0600` e é substituído atomicamente ao fim de cada lote. Por página, guarda o SHA-256 dos bytes escritos e o ETag do servidor naquele momento. O diretório de estado traz um arquivo de ignore com `*`. O estado é **por clone**: dois clones compartilhando estado conflitariam a cada merge e confiariam nas linhas de base um do outro. Diretório de estado que seja link simbólico é recusado.

### 3.3 Classificação em três vias

A classificação é pura. Para cada página há três versões: **disco** (cópia local), **base** (o que a ferramenta escreveu por último, do estado) e **servidor** (revalidado por ETag).

| Condição (nesta ordem) | Resultado |
|---|---|
| disco ausente | Criar |
| disco == servidor (hash) | Inalterado (mesmo sem base: sobra de execução interrompida entre escrever e salvar o estado) |
| disco == base e servidor mudou | Atualizar (avanço rápido) |
| disco == base e servidor sem mudança (304) | Inalterado |
| disco existe, **sem base** | Divergente: arquivo desconhecido |
| disco ≠ base e ≠ servidor | Divergente: edição local, ou frontmatter local se o arquivo começa com `---` |

Regras complementares:

- Se **qualquer** arquivo divergir, o **lote inteiro é recusado**: nada é escrito. Um resumo de diff limitado é mostrado, com caracteres de controle neutralizados.
- `--force` grava o conteúdo do servidor, e a ação fica marcada como forçada.
- **Nunca apaga** arquivos locais. Remoções ficam fora desta versão.

### 3.4 Segurança e limites

- Só a API de leitura (`/api/v1`). A listagem incremental `recent` tem paginação por cursor: parâmetro `updated_since` com a época zero, 100 itens por rodada, no máximo 1 000 rodadas. Cursor repetido aborta. Resposta que é array direto é o formato legado, aceito como fallback. A leitura de página usa `ETag` e `If-None-Match`. Timeout de 30 s por requisição. `401`, `403` e `404` viram erros claros. Um `404` na leitura significa que a listagem e a leitura se cruzaram (página removida ou expirada no meio da execução).
- Limite de 10 000 páginas por execução (excesso é erro, pedindo para estreitar `--include`) e de 8 MiB por página (corpo maior é erro).
- **Caminho portável:** só letras ASCII, dígitos, `.`, `-` e `_`. Segmento não pode começar nem terminar com ponto, o que recusa `.`, `..` e arquivos ocultos. Nomes reservados do Windows (`CON`, `PRN`, `AUX`, `NUL`, `COM1` a `COM9`, `LPT1` a `LPT9`, sem diferenciar maiúsculas, inclusive com extensão) são recusados. Caminho de até 180 bytes; segmento de até 64 bytes; profundidade de até 8 segmentos; folha com extensão `.md`. Barra invertida, NUL, caminho absoluto e caractere não ASCII são recusados. Colisões de caminho sem diferenciar maiúsculas são recusadas, porque macOS e Windows não diferenciam. Destino, componentes e diretório de estado que sejam links simbólicos são recusados. A escrita é atômica: arquivo temporário, renomeação e sincronização.
- O caminho devolvido pelo servidor precisa ser **idêntico** ao pedido, senão é recusado.
- O corpo da página é **dado**: é transportado, nunca executado nem interpretado.
- **Fora desta versão (planejado):** escrita comparada no núcleo; aplicação bidirecional por ferramentas públicas de escrita; remoções e relatório de conflito; gancho pós-merge ou de CI.

### 3.5 Algoritmo do lote e cliente

1. Prepara o destino: cria ou normaliza o diretório, e recusa link simbólico e arquivo. O modo `plan` não cria nada.
2. Carrega o estado. Estado corrompido, ou de versão desconhecida, falha fechado: não recomeça do zero.
3. Lista as páginas (só versões correntes) e seleciona por família, que é o primeiro segmento do caminho. Remove duplicatas e valida cada caminho.
4. Para cada página: lê o arquivo local, recusando link simbólico em qualquer componente. Se o arquivo existe, envia o ETag guardado em `If-None-Match`, porque um 304 só prova algo quando há arquivo local. Um 304 não traz corpo. Corpo acima de 8 MiB é erro. Classifica. Se estiver divergente, com `--force` e sem corpo, relê **sem** ETag para obter os bytes. Registra a decisão no relatório e no novo estado.
5. Se `--apply` e (sem divergências, ou com `--force`): grava todos os arquivos (temporário, renomeação e sincronização) e **só depois** salva o estado, com substituição atômica. Falha no meio deixa o estado antigo. A próxima execução "adota" os arquivos que já batem com o servidor, e eles saem como inalterados.

**Cliente HTTP:**

- Listagem: `GET` em `/api/v1/workspaces/{w}/projects/{p}/recent` com `updated_since=1970-01-01T00:00:00Z` e `limit=100`; nas rodadas seguintes, acrescenta `cursor`.
- Página: `GET` em `/api/v1/workspaces/{w}/projects/{p}/pages/{caminho}`, codificando cada segmento separadamente e preservando as barras. A resposta traz caminho, título, corpo em markdown e outros campos, mais o cabeçalho `ETag`. O caminho devolvido precisa ser igual ao pedido.
- Cabeçalhos: `Accept: application/json`; `Authorization: Bearer` quando há token; `If-None-Match` quando aplicável.
- Modos: `plan` (nunca escreve) e `export` (sem `--apply` continua dry-run).

**Relatório:** contagens (listadas, selecionadas, a criar, a atualizar, forçadas, inalteradas, divergentes), bytes a escrever, resumo de divergência (tamanhos, linhas, primeira linha diferente, sem caracteres de controle) e dicas de adição, commit e envio impressas após aplicar.

---

## 4. Importador: importação one-shot de corpora externos

Duas fontes. O modo padrão é **dry-run**. O modo vivo exige `--apply` e `--manifest-out`, e mais:

- fonte `omc-wiki`: `--workspace` e `--project` explícitos;
- fonte `external-conversation`: `--workspace` explícito. O projeto vem do envelope.

A autenticação é **somente** pela variável `AI_MEMORY_AUTH_TOKEN`; de propósito não há argumento de token. O importador nunca abre o banco nem a wiki do servidor, nunca apaga páginas e não tem modo de caixa de entrada nem de observação de pasta.

### 4.1 Fonte `omc-wiki`: diretório plano de markdown

Origem: wiki plana de markdown do oh-my-claudecode (OMC).

- Lê só arquivos `.md` do topo do diretório, em ordem de nome. Ignora `index.md` e arquivos cujo nome começa com `session-log-`; `--include-session-logs` os inclui.
- Destino determinístico `omc/<slug>.md`. Escreve **somente** pelo endpoint de escrita de páginas, `POST /admin/write-page`.
- Mapeia só os metadados que o endpoint suporta: `title`, `kind`, `tier`, `tags`, `pinned` e `body`. Frontmatter desconhecido é ignorado. `--pinned` fixa todas as páginas.
- Falha fechada em caminho: destino absoluto, com `..`, ou colisão de destino entre dois arquivos aborta o planejamento.
- Página que já existe no destino aborta, salvo com `--overwrite`. Ela é **re-checada imediatamente antes** de cada escrita (proteção best-effort contra corrida).
- Destino (workspace e projeto) inexistente exige `--create-destination`.
- Para no primeiro erro de escrita. Grava no manifesto o que foi concluído, mais o ponto de verificação que falhou.
- Versão de importação: `omc-wiki-v1`.

### 4.2 Fonte `external-conversation`: envelope genérico de conversa

Entrada: um envelope JSON com a forma abaixo. Adaptadores específicos de produto ficam **fora** deste repositório; eles só precisam emitir este envelope.

```json
{ "project": "meu-projeto", "source": "chatgpt", "session_id": "id-exportado",
  "messages": [ {"role":"user","content":"…"}, {"role":"assistant","content":"…"} ] }
```

**Validação do envelope:**

- Campos e papéis desconhecidos **falham fechado**. O envelope e cada mensagem recusam campos extras, e papel fora de `system`, `user` e `assistant` é erro.
- `project` e `workspace`: de 1 a 128 bytes após aparar espaços, sem caractere de controle, sem barra e sem barra invertida.
- `source`: até 64 caracteres ASCII, só letras, dígitos, `.`, `_`, `-` e `:`.
- `session_id`: até 256 bytes, sem caractere de controle.
- Arquivo de até 2 MiB. Até 128 mensagens. Soma do conteúdo bruto (antes da sanitização) de até 1 MiB.
- Mensagem de usuário é truncada a 16 KiB, de forma segura para UTF-8. Mensagem de assistente ou de sistema é truncada a 2 000 bytes. As truncadas são contadas, e a contagem aparece no dry-run e no manifesto.
- Exige ao menos uma mensagem de usuário. Mensagem vazia após a sanitização aborta.

**Ordem de execução:**

- Sanitização em duas camadas. O texto passa pela redação do cliente (seção 1) antes de hash, prévia e manifesto, e depois pelo sanitizador normal do servidor.
- Identidades de origem sensíveis são recusadas **antes** de derivar o id da sessão ou de gravar o manifesto.
- Proveniência no corpo é não confiável e não pode fornecer ator nem destino.

**Replay pelo pipeline público de hooks.** Um único lote vai para `/hook/batch` e é processado em ordem. O lote só retorna quando a página de sessão e os efeitos do fim de sessão já foram gravados.

| # | Evento | Observações |
|---|---|---|
| 0 | `session-start` | corpo: id da sessão estável, modelo `external:<origem>` (até 160 bytes), origem e id externo da sessão |
| 1..n | `user-prompt` (usuário); `external.assistant-message` ou `external.system-message` (assistente ou sistema) | corpo do usuário: prompt, origem e papel. Corpo de assistente ou sistema: título, mensagem, origem e papel. O título é a primeira linha não vazia, até 160 bytes; sem linha, vale "Imported <papel> message". Para assistente e sistema, a URL também leva a extensão fixa do importador e `source_event` = `<papel>-message` |
| n+1 | `session-end` | corpo traz o fingerprint do transcript |

Cada URL de evento leva os parâmetros `event`, `agent` (= `external-import`, identidade fixa do agente do fio), `workspace`, `project`, `project_src` (= `marker`), `session_id` (estável) e `ingest_key`. `extension` e `source_event` só aparecem nas mensagens de assistente e de sistema.

O agente `external-import` nunca é o nome de um harness nativo. O núcleo deve tratá-lo como importação externa, sem criar tipo de agente por produto. **A confirmar:** o mapeamento desse agente desconhecido para a categoria genérica no núcleo não foi localizado no código lido.

**Idempotência (chaves):**

- Id de sessão estável = `external-` seguido dos 32 primeiros caracteres hexadecimais do SHA-256 sobre as partes [`external-conversation-v1`, workspace, projeto, origem, id da sessão].
- Chave de ingestão de cada evento = SHA-256 sobre as partes [versão, id estável, índice em decimal, nome do evento, bytes exatos do corpo].
- Cada parte entra no hash **precedida pelo comprimento**, em little-endian, o que evita ambiguidade de concatenação.
- Fingerprint do transcript = SHA-256 sobre [versão, projeto, origem, id da sessão] e, para cada mensagem, [papel, conteúdo].
- Reexecutar uma importação interrompida é seguro. **Anexar mensagens** a uma exportação muda o fingerprint, o que gera uma nova chave terminal, e então o `session-end` roda de novo.
- Sucesso exige resposta HTTP de sucesso **e** ack que aceite todos os eventos: sem `failed_index`, e com índices aceitos iguais a `0..n−1` ou `accepted` igual a `n`. Aceite parcial deixa o manifesto `failed`, com a contagem aceita. Rode de novo a mesma fonte para retomar.

**Dry-run:** mostra o plano. Com `--show-body`, mostra os corpos sanitizados. Com `--manifest-out`, grava o manifesto em estado planejado. Sem manifesto, imprime a contagem de eventos e de mensagens truncadas.

**Manifesto:** JSON com versão `external-conversation-v1`, caminho da origem, hash da origem, origem, id externo da sessão, id estável, fingerprint do transcript, workspace, projeto, quantidade de eventos planejados, mensagens truncadas, eventos aceitos, estado (`planned`, `imported` ou `failed`) e erro. É gravado de forma atômica (arquivo temporário irmão e renomeação). O diretório pai precisa existir.

**Roadmap (sem implementação):** importação do grafo de memória de outro assistente de código, e de coleções de um banco vetorial externo. Nenhum dos dois existe hoje.

### 4.3 Detalhes da fonte omc-wiki e do cliente HTTP do importador

**Planejamento (omc-wiki):**

- Canonicaliza o diretório de origem, que precisa ser um diretório. Percorre só o primeiro nível, ordenado por nome, e considera arquivos `*.md`. Ignora `index.md` e `session-log-*`, salvo com a flag. Para cada arquivo: hash SHA-256 do conteúdo; leitura do markdown; destino `omc/<slug>.md`; validação do destino. Colisão de destino entre dois arquivos aborta o planejamento.
- `slugify`: converte para minúsculas primeiro. Todo caractere que não seja `a` a `z` ou `0` a `9` vira `-`, sem repetir. Os `-` das pontas são aparados. Se o resultado for vazio, vira `page`.
- Leitura do markdown: remove o BOM inicial. Aceita o delimitador de frontmatter `---` com quebra de linha LF **ou** CRLF, para checkouts com conversão automática de fim de linha. Lê o YAML e extrai título, tipo, nível, fixado e tags (lista ou string única). O corpo é o que vem depois do segundo delimitador. Sem frontmatter, o corpo é o arquivo inteiro. Delimitador de fechamento ausente faz o arquivo inteiro virar corpo, sem metadados. YAML inválido aborta. Título ausente vem do primeiro H1 (`# `) do corpo.
- Nível (`tier`) precisa ser `working`, `episodic`, `semantic` ou `procedural`. O padrão é `semantic`. Outro valor aborta o planejamento, antes de qualquer escrita.
- Validação do destino: recusa caminho absoluto, barra invertida, `..`, raiz ou prefixo de unidade. O primeiro segmento não pode ser um destes reservados: `_rules`, `_internal`, `.git`, `sessions`, `session-logs`, `procedures`, `decisions`, `gotchas`. Nesta fonte o destino sempre começa em `omc/`, então essa lista não se aplica na prática. O destino precisa terminar em `.md`.
- Validação da origem relativa: sem caminho absoluto e sem `..`.

**Execução viva (omc-wiki):**

1. Grava o manifesto em estado `planned` **antes** de qualquer rede.
2. Pré-verificação: `GET` na listagem de páginas do projeto. Status 200 significa que o projeto existe. 404 com `--create-destination` segue. 404 sem a flag é erro. Qualquer outro status é erro.
3. Sem `--overwrite`: lista os caminhos existentes e aborta se algum destino planejado já existe.
4. Por página: re-checa com `GET` na página. 200 marca `failed` e aborta. 404 segue. Outro status é erro. Depois faz `POST` em `/admin/write-page` com workspace, projeto, caminho, corpo, título (opcional), tipo (opcional), nível, tags e fixado. A resposta traz id da página, caminho e ponto de verificação (opcional). Atualiza a entrada e **regrava o manifesto a cada página**. Em erro, marca `failed` com a mensagem e para.

**Manifesto omc-wiki:** versão `omc-wiki-v1`; entradas com versão, caminho de origem, hash da origem, destino, estado (`planned`, `written` ou `failed`), id da página, caminho, ponto de verificação e erro (estes três últimos opcionais). Escrita atômica.

**Cliente HTTP:**

- Timeout de 30 s. O caminho base do servidor é preservado, para servidor atrás de prefixo.
- `Authorization: Bearer` quando a variável de token está definida e não vazia.
- `post_hook_batch` revalida, para cada evento e imediatamente antes de enviar, o corpo e a URL. Depois envia o array `[{url, body}]` ao endpoint de lote e lê o ack: `accepted`, `accepted_indices` (opcional) e `failed_index` (opcional). Sucesso exige todos os eventos aceitos e nenhum `failed_index`. Resposta parcial marca o manifesto como `failed`, com a contagem aceita. Reexecutar a mesma fonte retoma pelas chaves estáveis.

---

## 5. App macOS: barra de menu e serviço de login

**Wrapper, não um segundo console de operação.** É um aplicativo de barra de menu, sem ícone no Dock. Ele tem duas camadas: a interface, e uma camada de lógica testável, sem interface. Ele **embute o runtime**, **gerencia o serviço de login** (LaunchAgent) e **abre as superfícies que a ferramenta já tem**: `/web`, status, arquivo de configuração e logs.

| Dentro do app (substituível) | No diretório de dados do usuário (sobrevive a updates) |
|---|---|
| Interface | banco, wiki e arquivo de configuração do servidor |
| Binário do ai-memory | demais dados do servidor |
| Hooks embutidos | |
| Modelo do serviço de login | plist renderizado em `~/Library/LaunchAgents/` |
| | logs em `~/Library/Logs/ai-memory/` |

Dados: `~/Library/Application Support/ai-memory` (padrão do binário no macOS). O override opcional nas configurações grava a variável `AI_MEMORY_DATA_DIR` **apenas** no plist. Mudar o override **não move** arquivos existentes.

**Componentes:**

| Componente | Papel |
|---|---|
| Gerenciador do serviço | Usa um rótulo fixo de serviço, no formato de identificador reverso de domínio, definido no código do app. Renderiza o modelo trocando dois marcadores: caminho do binário e diretório pessoal. Injeta a variável de diretório de dados se houver override. Executa bootstrap, bootout e kickstart com `-k` pelo gerenciador de serviços do macOS. Estados: não instalado, parado e em execução (detectado pela linha de estado da saída de consulta) |
| Runtime embutido | Resolve binário, hooks e modelo do plist dentro do bundle, ou pelo caminho da variável `AI_MEMORY_MENU_RUNTIME`. Primeira execução = não existe arquivo de configuração no diretório de dados; nesse caso roda a inicialização embutida |
| Cliente de saúde | `GET /admin/status`, com token opcional. Resposta ok deixa o ícone verde |
| Cofre de token | Keychain do macOS, e uma versão em memória para testes |
| Preferências | URL do servidor e override do diretório de dados |
| Execução de processos | Subprocessos com timeout de 30 s, e leitura de log |

**Fluxo:**

- **Instalar e iniciar:** roda a inicialização se necessário, renderiza o modelo do serviço e faz bootstrap.
- **Reiniciar:** re-renderiza o modelo (o caminho do binário pode ter mudado entre versões) e reinicia o serviço.
- **Atualizar:** substituir o app em `/Applications`.
- O script de build compila o runtime e o app, e monta o bundle com o runtime embutido.
- Notarização, identidade de desenvolvedor e cask de Homebrew ficaram **fora** da primeira versão.
- Testes: a suíte da camada de lógica roda no workflow de CI dedicado ao app macOS.

**Comportamento do app:**

- Polling do estado a cada **15 s**; a cada **1 s** enquanto o ícone está em "iniciando".
- Carência de "iniciando": **45 s** após a ação de iniciar. Depois disso o ícone deixa de estar em "iniciando", mesmo sem resposta.
- Cliente de saúde com timeout de **3 s** e `Authorization: Bearer` se houver token no cofre.
- O estado do ícone é derivado do estado do serviço e da resposta de `/admin/status`.
- Ações: instalar e iniciar, iniciar, parar, reiniciar, atualizar status, abrir a interface web, abrir a configuração, abrir o diretório de dados, abrir logs, salvar configurações.
- Falhas de saúde são traduzidas em mensagem: tempo esgotado, não autorizado e outras falhas.

**Decodificação de `GET /admin/status`:** campos desconhecidos são ignorados, para que servidor de outra versão ainda seja decodificado no que o app mostra. Campos: versão; diretório de dados (opcional); bind (opcional); contagens (páginas mais recentes, todas as versões, sessões, observações); fila de escrita (opcional, no formato de par `[enfileirados, capacidade]`); provedores de LLM e de embedding (opcionais), cada um com status, provedor e modelo (opcionais); ingestão (opcional): aceitos, descartados por política, descartes por saturação e por limite de taxa, e último instante persistido (opcional). Campo ausente conta como 0.

- **Degradado** = fila de escrita com itens enfileirados **ou** status de LLM igual a `error` **ou** status de embedding igual a `error`.
- **Linhas do menu:** `Bind` (se houver); `Pages N  (all versions M)`; `Sessions`; `Observations`; `LLM <status>  <provedor>/<modelo>` (ou `LLM unknown`); `Embed …` com a mesma regra; `Write queue q/cap` só se `q > 0`; e, se houver bloco de ingestão, `Ingest accepted`, `Dropped by policy` (só se maior que zero) e `Last write`. `Last write` mostra `—` se nunca houve escrita; senão mostra `Ns ago`, `Nm ago`, `Nh ago` ou `Nd ago`.

**Estados do ícone** (ordem de derivação):

- Com relatório: degradado, ou ok.
- HTTP 401 ou 403: autenticação necessária.
- Falha de busca: não instalado (se o serviço não está instalado), ou inacessível.
- Caso contrário: desconhecido.
- Durante a carência de inicialização, mantém "iniciando" até a resposta de status chegar (ok, degradado e autenticação necessária passam por cima) ou a carência expirar.

**Cores do ícone:** ok verde · iniciando laranja · degradado e autenticação necessária amarelo · inacessível vermelho · não instalado e desconhecido cinza. O ícone é uma bolinha de 18×18 **não-template**, porque menus achatam a cor de tinta, e só uma imagem não-template muda de cor.

**Menu:** versão; última mensagem de erro do servidor (até 12 linhas monoespaçadas, para o motivo aparecer sem abrir o log de erro, por exemplo "Address already in use"); linhas de estatística; aviso "Runtime not bundled" (com a instrução de rodar o script de build) quando o runtime não está embutido. Ações do serviço conforme o estado: não instalado mostra "Install & Start Server"; parado mostra "Start Server"; em execução mostra "Stop Server" e "Restart Server". Essas ações ficam desabilitadas durante uma operação e, para iniciar e reiniciar, durante "iniciando". Depois vêm "Open Web UI", "Show Status…" (quando aplicável), "Open Config", "Open Data Directory", "Open Logs", "Settings…" e "Quit".

Como app acessório nunca é o aplicativo em primeiro plano, "Settings…" e "Show Status…" ativam o app antes de abrir a janela. Senão a janela abriria atrás dos outros.

**Janela de configurações:** seção Servidor (URL; token em campo seguro, salvo no cofre; "Clear saved token"); seção Dados (override do diretório; vazio = `~/Library/Application Support/ai-memory`; mudar não move arquivos existentes); seção Login ("Launch menu bar app at login", pelo serviço de login do sistema, com erro inline).

**Janela de status:** mínimo de 560×360. Mostra a saída do comando de status em texto monoespaçado selecionável, com botões Refresh e Copy.

Relevância para ambiente corporativo: se você só usa Linux ou Windows, pode ignorar este app. Ele é um empacotamento de conveniência; o servidor é o mesmo.

---

## 6. Harnesses de avaliação (fora de escopo)

Existem dois harnesses de avaliação, reunidos em um executável de linha de comando com dois subcomandos:

- **`ab`**: roda o **mesmo prompt de consolidação usado em produção** contra dois provedores de LLM lado a lado, e grava as duas saídas para comparação humana.
- **`retrieval`**: benchmark de recuperação (LongMemEval), conduzido de ponta a ponta por um subprocesso real do servidor.

Eles **não fazem parte do produto distribuído**: não entram na imagem de container, não são executados pela CI, e são compilados junto com o repositório só para compartilhar dependências.

Quando usar: depois de trocar de provedor ou de modelo, ou ao editar o prompt de consolidação. Os fixtures são sessões sintéticas curtas, que exercitam casos difíceis: extração de regra durável, separação de múltiplos tópicos, sessões em que o correto é não dizer nada, e distinção entre decisão e armadilha. Cada fixture tem nome, descrição e uma lista de observações, cada uma com tipo, título e corpo.

Mínimo útil para um sinal de regressão de qualidade: uma pasta de fixtures de sessões sintéticas, o mesmo código de montagem de requisição usado em produção (ver [prompts-schemas-and-sanitizer.md](prompts-schemas-and-sanitizer.md)), e leitura humana lado a lado das saídas de dois provedores.

Relatórios de benchmark não fazem parte desta documentação.

---

## 7. Mapa de testes fora do núcleo

| Local | O que valida | Como roda |
|---|---|---|
| Suíte de testes dos hooks (POSIX) | Funções de biblioteca dos hooks. Usa diretório temporário para não tocar a instalação real, com as variáveis de dados e de configuração removidas do ambiente | Executar o script de testes dos hooks; ele também chama as variantes para Windows quando o runtime está disponível |
| Testes de contrato do spool (variante Windows) | Contrato em disco do spool: nome do arquivo, campos da entrada, ausência de BOM, token. E um dreno com conexão recusada (porta 1) que mantém o backlog | Pela suíte de hooks |
| Testes de funções de handoff do Grok (variante Windows) | Funções puras de handoff, como o gate de sessão-filha | Pela suíte de hooks |
| Smoke de handoff ponta a ponta | Handoff e recall com **duas chamadas reais** a um provedor de LLM gratuito, com modelos diferentes por sessão. Cobre ingresso de hook, handoff automático no fim da sessão, leitura do handoff, e que um segredo-canário plantado no prompt **não** aparece em nada persistido | Script de smoke. A chave do provedor vem de variável de ambiente |
| Smoke do relay | Ver seção 2.7 | Script de smoke, sem framework de teste externo |
| Testes unitários dos companions | Cada companion tem sua própria validação de formatação, lint e testes | Pelo job `companions` da CI, não pela suíte raiz |
| Teste de atualização do wrapper de CLI | Atualização do wrapper de linha de comando | Script |

Jobs de CI: `fmt`, `clippy`, `test`, `windows-cross`, `companions`, `release-build`, `source-install`, `native-packaging`, `hooks-shell`, `changelog`, `docker-smoke`, `deny`, `audit`, `gitleaks`. Workflows à parte: app macOS, `nix`, `release`, `secret-scan` e `windows`.

---

## 8. O que levar para a sua reimplementação

1. **Comece sem companions.** O servidor sozinho já captura, consolida e serve memória (ordem de construção em [overview.md](overview.md)). Companions são opcionais.
2. **Se tiver um orquestrador**, o relay é o modelo a copiar: fila local durável, chave de ingestão determinística sem hash de conteúdo, uma cabeça por sessão por lote, ack validado por inteiro, recibos com desfecho tipado e janela de retry de 30 dias medida desde o **primeiro envio**.
3. **Se precisar migrar conteúdo**, copie o desenho do importador: dry-run por padrão, manifesto, IDs estáveis derivados por hash com prefixo de comprimento, replay pelo pipeline **público** de hooks (nunca escrita direta no banco) e redação em duas camadas.
4. **Se quiser memória do time em controle de versão**, o exportador mostra a política segura: allowlist explícita, classificação em três vias com recusa do lote inteiro em caso de divergência, estado por clone e nunca apagar.
