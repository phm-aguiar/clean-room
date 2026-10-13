# Captura e deduplicação sem hooks de ciclo de vida (pesquisa)

> Pesquisa para a proposta de captura em apps de desktop. O documento separa três coisas: o comportamento que o projeto já tem (hooks, fila local, deduplicação), o que é proposta e o que ainda não foi verificado. Afirmações sobre produtos de terceiros sem evidência no código, ou sem verificação ao vivo registrada, estão marcadas como "a confirmar".

Regra vigente: todo dado que entra no armazenamento passa pela sanitização e chega por um dos endpoints de captura: evento único (`/hook`) ou lote (`/hook/batch`). Nos pontos de captura revisados não há escrita direta fora desses endpoints (a confirmar em revisão completa).

## 1. Mapa de captura por superfície

| Superfície | Mecanismo de captura | Situação | Esforço (estimativa) |
| --- | --- | --- | --- |
| Claude Code (CLI) | Hooks de ciclo de vida instalados pelo projeto; eventos chegam por `/hook`. | Coberto pelo projeto | 0 |
| Claude Desktop, aba Code | A sessão roda o Claude Code com as configurações do usuário, então os hooks disparam com o agente `claude-code`. Observado em pesquisa (ver [verificação de out/2026](verification-2026-10.md)). | A confirmar: sem evidência no código | 0, se confirmado |
| Claude Desktop, Cowork | As tarefas rodam em máquina virtual no Linux; não foi verificado no ambiente de pesquisa. | A confirmar | verificar |
| Codex (modo Codex do app desktop) | O projeto instala hooks para o Codex no mesmo diretório de configuração do CLI. Que o processo do app dispare esses hooks está a confirmar; falta uma verificação no próprio app. | Parcial: instalação coberta; comportamento no app a confirmar | verificar |
| Antigravity (IDE, 2.0, CLI) | Hooks nativos do CLI instalados pelo projeto (ver seção 5.4). Leitura do mesmo arquivo pela IDE: a confirmar. | Coberto para o CLI; IDE a confirmar | ajuste de adaptador |
| Cursor | Hooks nativos do projeto. Cópia de hook do Claude Code disparada pelo Cursor é descartada quando os hooks nativos do Cursor estão instalados (seção 5.6). | Coberto | 0 |
| Zed (agentes ACP e agentes de terminal) | Hooks do próprio agente; o Zed é transparente. | A confirmar | 0 |
| Claude Desktop, aba Chat; ChatGPT web; Grok Bot | Sem hooks e sem armazenamento local conhecido. Só há chamadas de ferramenta via MCP (model-discretion) e uma ferramenta explícita de captura, que é proposta. | Sem captura automática | fase 1 da proposta |
| Plugins hospedados do ChatGPT | Só funcionam como plugin publicado, com servidor alcançável. | A confirmar | nova superfície |

## 2. O que o servidor MCP enxerga

O servidor de memória é também o servidor MCP. Em qualquer app conectado por MCP, o servidor observa:

- nome e argumentos das chamadas de ferramenta;
- resultados dessas chamadas;
- horários das chamadas.

Não observa turnos de conversa, prompts do usuário nem saída do modelo (a confirmar: não conferido no código do servidor MCP, mas coerente com o escopo do protocolo).

Consequências práticas:

- A observação do lado do servidor serve para telemetria de uso (quais ferramentas o modelo chama) e para sinais de efetividade de recall. Não serve para captura de conversa.
- Para captura explícita, a proposta prevê uma ferramenta que o modelo pode ser instruído a chamar. Essa ferramenta não existe no código revisado. Em apps sem nenhuma superfície de hook, seria o único caminho dentro da sessão, e a qualidade dependeria de o modelo aceitar resumir a conversa.

## 3. Fontes locais (watchers)

Para cada app, o caminho do armazenamento local é descrito pela pesquisa e não foi verificado no código do projeto. Todos os caminhos abaixo estão marcados como a confirmar.

- Claude Code (inclui a aba Code do desktop): transcritos de sessão no diretório de projetos do usuário (a confirmar). Um watcher só agrega valor em fluxos sem hooks, por exemplo quando o usuário desativa os hooks.
- Codex: registros de sessão e histórico no diretório do usuário (a confirmar).
- Antigravity: transcritos por conversa, em diretório indexado pelo identificador da conversa (a confirmar; não há conversas de IDE na máquina de pesquisa).
- Zed: armazenamento interno do app. Não deve ser usado como fonte, porque é instável e depende da versão. A recomendação é usar os agentes ACP.
- Chat do Claude Desktop, ChatGPT web e Grok Bot: sem armazenamento local, então a classe de watcher não se aplica.

Um watcher ou importador envia os dados no mesmo formato de lote de `/hook/batch`, por meio de um companheiro externo ao núcleo do projeto. Ele fica fora do núcleo do projeto.

## 4. Entrega: fila local, lote e idempotência

### 4.1 Fila local

A captura por hook não espera a rede. Cada evento é gravado numa fila local (escrita pequena e imediata) e enviado depois. A fila é drenada nestes momentos:

- no início de sessão, com uma passagem de limpeza;
- em segundo plano nos eventos de fim de turno (stop), antes de compactação e fim de sessão;
- em um disparo por limiar durante sequências de ferramentas (post-tool-use), para não acumular.

Cada evento na fila carrega a sua própria autenticação. Token estático fica guardado no evento, com permissão restrita ao usuário. Em OIDC, o evento guarda só o modo, e o token é resolvido e renovado no momento do envio.

### 4.2 Constantes e limites

| Parâmetro | Valor | Observação |
| --- | --- | --- |
| Chave de ingestão | 32 caracteres hexadecimais | Gerada uma vez por evento, no enfileiramento, e reenviada em todas as retentativas. |
| Validação da chave no servidor | 1 a 64 caracteres: letras e dígitos ASCII, `-` e `_` | Chave inválida é tratada como ausente, sem erro 4xx. O evento passa a ter entrega "pelo menos uma vez". |
| Validade da chave no servidor | 30 dias | Varredura oportunista de chaves com mais de 30 dias. |
| Idade máxima de um evento na fila | 7 dias | Eventos mais velhos são descartados, mesmo sem tentativas esgotadas. |
| Máximo de eventos na fila | 10 000 arquivos | Ao exceder, o enfileiramento remove os eventos mais antigos ainda não entregues, com aviso. |
| Tentativas falhas antes de descartar | 8 (padrão, configurável) | Valor 0 desativa o descarte por tentativas. Servidor inacessível não consome tentativas. |
| Eventos por lote | até 256 | Limite de contagem. |
| Bytes por lote | até 8 MiB (orçamento flexível) | Um lote sempre leva pelo menos um evento, mesmo que ele sozinho passe do orçamento. |
| Teto de corpo de uso de ferramenta, notificação, stop e eventos genéricos | 2 000 bytes | Aplicado depois da sanitização. |
| Teto de corpo de prompt do usuário e de pós-compactação | 16 KiB | Aplicado depois da sanitização. |
| Teto de corpo dos demais eventos | 16 KiB | Aplicado depois da sanitização. |

### 4.3 Chave de ingestão: deduplicação de entrega

A deduplicação de entrega não depende do agente nem da sessão. Ela usa a chave de ingestão por projeto:

1. O servidor reivindica a chave na mesma transação em que grava a observação.
2. Depois que os efeitos posteriores do evento (páginas de wiki, handoff) terminam, a chave é marcada como completa.
3. Ao receber uma chave já conhecida, o servidor responde conforme o estado:
   - chave não existe: grava a observação (inserida);
   - chave existe e não está completa: não grava de novo e retoma só os efeitos posteriores pendentes;
   - chave existe e está completa: ignora o evento como já processado.

Entregas concorrentes com a mesma chave no mesmo projeto são serializadas dentro do processo do servidor.

A janela de 30 dias da chave cobre a janela de 7 dias da fila local. Assim, uma retentativa de um evento ainda enfileirado encontra a chave.

### 4.4 Sem fila: caminho direto e fallback

- Se o servidor não tem o endpoint de lote (resposta 404 ou 405, por exemplo versão anterior), o cliente envia os eventos restantes um a um por `/hook`.
- Se a fila local não pode ser gravada, o evento é perdido e o hook emite um aviso. A chamada da ferramenta não é bloqueada.
- Uma entrada da fila ilegível é removida, para não travar a fila.

### 4.5 Sanitização antes da gravação

O trecho de corpo durável do evento é sanitizado e só depois truncado no teto por tipo de evento (seção 4.2). A captura da mensagem final do assistente (opt-in) não foi revisada nesta versão do documento: a confirmar.

## 5. Identidade de sessão e deduplicação por superfície

### 5.1 Regras de identidade no servidor

- Um identificador nativo que já é UUID é usado como está. Qualquer outro texto vira um UUID determinístico (UUID v5, com namespace fixo). Assim, o mesmo texto sempre gera o mesmo identificador de sessão, e o servidor e as ferramentas offline chegam ao mesmo resultado.
- Um evento sem identificador de sessão: o início de sessão cria um identificador novo; qualquer outro evento é rejeitado.
- Tupla imutável: a sessão guarda workspace, projeto, tipo de agente e dono. Um evento com o mesmo identificador e outro tipo de agente é uma colisão e é rejeitado de forma terminal.
- Escopo não é identidade: se o diretório de trabalho muda no meio da sessão, o evento vai para o projeto que o novo diretório aponta. O fim de sessão só encerra a sessão no escopo registrado, ou quando vem do mesmo diretório.
- Sessão sem diretório de trabalho cai no projeto padrão de rascunho (chamado "scratch" no código).
- Exceção: para o Grok, o identificador de sessão pode ser reutilizado depois do fim. Para os demais agentes, uma sessão encerrada é final.

### 5.2 Claude Code (CLI e aba Code do desktop)

- O identificador nativo da sessão identifica a sessão: a chave `(agente, identificador)` já cobre o caso de uma única sessão.
- Que a aba Code e o CLI compartilhem o mesmo identificador, e que o recurso `--resume` mova o identificador entre superfícies: a confirmar.
- Sessões da aba Code iniciadas sem pasta de projeto usam um diretório de trabalho temporário gerado pelo app. Que isso aconteça com esse formato de caminho: a confirmar. No código, a regra existente é: sem diretório de trabalho, a sessão vai para o projeto padrão de rascunho. Duas opções de produto ficam em aberto (seção 6).
- O identificador local de sessão do app não chega aos hooks. Não deve ser usado como fonte de captura; é só metadado opcional, e não é uma dependência.

### 5.3 Codex

- O identificador nativo identifica a sessão. A regra de design é que CLI, app desktop e extensão de IDE compartilham o mesmo tipo de agente, porque compartilham o mesmo espaço de identificadores (proposta).
- Que os identificadores sejam UUIDs dos registros locais de sessão: a confirmar.
- Sessões na nuvem do Codex não aparecem no armazenamento local. Se algum dia forem capturadas, devem entrar como tipo de agente separado, para evitar contagem dupla com os registros locais. Esse tipo não existe no código (proposta).

### 5.4 Antigravity

- Identidade: o campo `conversationId` do corpo do evento é o identificador de sessão. O mesmo identificador vale para IDE, CLI e 2.0 na mesma conversa (a confirmar para IDE e 2.0).
- Tipo de agente: `antigravity-cli`. Os nomes `antigravity` e `agy` também são aceitos como entrada.
- Diretório de trabalho, nesta ordem de prioridade:
  1. campos diretos de diretório (`cwd`, entre outros) do evento;
  2. o primeiro caminho não vazio de `workspacePaths`;
  3. caminhos aninhados de diretório do evento.
- Distinguir superfície pelo prefixo de `transcriptPath` (IDE, CLI, 2.0): a confirmar. O campo é usado no código para localizar o diretório de artefatos da execução.
- Eventos instalados pelo projeto: uso de ferramenta antes e depois (PreToolUse, PostToolUse), início de turno (evento de origem `PreInvocation`, gravado como início de sessão) e `Stop`. Não há hook de fim de sessão.
- Início de sessão só é processado quando `invocationNum` vale 0. A premissa de que esse número volta a 0 a cada turno do usuário: a confirmar, o código depende dela.
- Fim de sessão: `Stop` encerra um ciclo de execução, não a conversa. Depois do último turno, o usuário roda `ai-memory finalize-session --agent antigravity-cli`, que gera o resumo final, o handoff e enfileira a consolidação opcional de fim de sessão.
- Instalação: `ai-memory install-hooks --agent antigravity-cli` grava em `~/.gemini/config/hooks.json`. Um arquivo `.agents/hooks.json` e a leitura desse mesmo arquivo pela IDE: a confirmar.

### 5.5 Regra geral de deduplicação

- Chave de sessão: `(agente, identificador)`, com identificador nativo convertido como na seção 5.1.
- Superfícies de um mesmo ecossistema que compartilham o espaço de identificadores ficam em um só tipo de agente. Exemplos: Claude Code cobre CLI e aba Code; Codex cobre CLI, app e extensão de IDE; Antigravity cobre IDE, CLI e 2.0 (proposta, a confirmar por superfície).
- Quando um app fechado tem identificador próprio (por exemplo, um identificador local que não aparece nos hooks), esse identificador é só metadado. Não se deve criar captura a partir dele.
- O tipo de agente `claude-desktop` existe no código e é usado para cópia de configuração de agentes. Ele não é o tipo das sessões capturadas pela aba Code, que usam `claude-code`.

### 5.6 Cursor e a cópia do Claude Code

Quando o Cursor roda as configurações do Claude Code, o evento chega com o agente `claude-code`, mas o corpo identifica o Cursor. Se os hooks nativos do Cursor estão instalados, essa cópia é descartada, para manter uma observação por evento do Cursor.

## 6. Questões abertas

1. Codex no app desktop: os hooks instalados disparam pelo processo do app? Falta excluir uma sobreposição no próprio app.
2. Antigravity: a IDE lê `~/.gemini/config/hooks.json` na prática? O CLI lê o arquivo `.agents/hooks.json` (a confirmar). O campo `workspacePaths` tem mais de um caminho nesta configuração?
3. Chat do Claude Desktop: há cache local de conversas previsto em extensões ou plugins? Reavaliar a cada trimestre; não é viável hoje.
4. Formato de entrada do Grok Bot: só vale a pena investigar se o produto ganhar tração; não há documentação pública.
5. Sessões de rascunho: pool único (`desktop/claude-scratch`) ou um projeto por diretório temporário? É decisão de produto e afeta o escopo automático.
6. Antigravity: o número de invocação reinicia a cada turno, então o início de sessão é mapeado por turno. Aceitar ou limitar por conversa, depois de medir o efeito no servidor?
