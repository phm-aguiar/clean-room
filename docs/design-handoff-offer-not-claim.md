# Handoff no início de sessão: reivindicar ou apenas oferecer

Especificação do comportamento de entrega do handoff (bastão de continuidade) na
rota `GET /handoff`, com a chave de configuração `[handoff] claim_on_session_start`.

Este documento separa o comportamento **já implementado** das **propostas**. Itens
marcados como *(proposta)* não têm implementação correspondente.

## 1. Problema que o modo oferta resolve

Hoje um handoff pendente é reivindicado automaticamente pela primeira sessão que
iniciar em um escopo elegível (qualquer harness, interativa ou não). A reivindicação é
de uso único e irreversível. Na prática, o bastão muitas vezes é consumido por uma sessão
não relacionada ou por um lançamento não interativo (por exemplo, um comando de execução
sem interface, uma sonda de saúde ou uma execução gerenciada).

O modo oferta remove a reivindicação automática: o início de sessão apenas **avisa** que
existe um handoff, e quem decide pegá-lo chama `memory_handoff_accept` com o identificador
exato.

## 2. Configuração

```toml
[handoff]
claim_on_session_start = true   # padrão: reivindica automaticamente (oferta desligada)
create_on_session_end  = true   # padrão: fim de sessão cria handoff automático
```

| Chave | Padrão | Efeito |
|---|---|---|
| `claim_on_session_start` | `true` | `true`: `GET /handoff` reivindica o handoff e o entrega. `false`: modo oferta (seção 5). |
| `create_on_session_end` | `true` | `false`: o fim de sessão ainda grava a página de resumo e enfileira consolidação, mas **não** cria handoff automático. Isso vale também para o bastão de checkpoint de turno de um harness específico (OpenCode, turno raiz). Handoffs manuais (`memory_handoff_begin`) e runs gerenciados não são afetados. |

Ambas são **globais do servidor** (valem para todos os operadores). Equivalente por
variável de ambiente: `AI_MEMORY_HANDOFF__CLAIM_ON_SESSION_START=false` (separador `__`).

**Não existe** override por projeto no marcador do projeto. *(proposta: override por
projeto, deixado para uma mudança futura.)*

## 3. Seleção do handoff candidato

Entrada: escopo `(workspace, projeto)` resolvido, diretório de trabalho (`cwd`) do
receptor (opcional), filtro de dono e `busy_since = agora − 10 min`.

Regras, na ordem em que são aplicadas:

1. **Filtro de dono** (primeiro). Se o ator é identificado, o filtro é "o dono é este
   operador **ou** não tem dono". Se não há ator identificado, o filtro é "não tem dono".
   Handoffs sem dono são visíveis para todo o projeto (comportamento de operador único).
2. Só handoffs com estado `open`.
3. **Manual** (criado por `memory_handoff_begin`, sem sessão de origem) é candidato em
   qualquer `cwd` do projeto e **sempre vence** um handoff automático.
4. **Automático** (criado no fim de sessão) só é candidato se o `cwd` armazenado é o `cwd`
   do receptor ou um ancestral dele, comparado por fronteira de componente de caminho.
   Se o `cwd` armazenado está vazio, o handoff é tratado como do projeto inteiro. Se o
   receptor não informa `cwd`, todo handoff aberto é candidato.
5. **Bastão de sessão viva.** Um handoff automático cuja sessão de origem ainda está
   **aberta** (não encerrada) e teve alguma observação registrada depois de `busy_since`
   (últimos 10 minutos) não é oferecido nem reivindicado. Uma sessão encerrada não bloqueia
   o bastão, mesmo que tenha eventos recentes. Isso é o que permite que uma sessão nova
   pegue o bastão logo após o fim da anterior.
6. **Ordem de preferência entre candidatos:** manual antes de automático; depois o mais
   recente (`created_at`); só em empate de timestamp entre automáticos, o `cwd` mais
   específico (mais longo, depois de normalizado); por último, o identificador, como
   desempate determinístico.

O campo `to_agent` existe na linha do handoff e é gravado sempre vazio pelo handoff
automático. **A seleção não o lê.** *(Não implementado: direcionamento por agente de destino.)*

## 4. Expiração e supersessão

- **Fim de sessão (criação de handoff automático):** antes de inserir o novo handoff
  automático, todos os outros handoffs automáticos abertos com o **mesmo `cwd` exato** e o
  **mesmo dono** são marcados como `expired`, desde que a sessão de origem tenha encerrado
  **ou** seja a própria sessão que está gravando. Sessões ainda abertas são poupadas: cada
  uma atualiza o próprio bastão. Não há critério de idade nesta etapa. A supersessão não
  usa ancestralidade de `cwd`, só igualdade.
- **Varredura pós-reivindicação:** depois que um handoff automático é reivindicado, são
  expirados os handoffs automáticos abertos do **mesmo dono**, com `cwd` compatível com o
  `cwd` do receptor (por fronteira de componente), cuja chave de preferência é **menor** que
  a do handoff reivindicado, e cuja sessão de origem **não** está viva (regra 5). Handoffs
  manuais nunca são varridos.
- **Manuais** ficam abertos até serem aceitos ou cancelados (`memory_handoff_cancel`
  marca como `expired`). O estado `expired` existe para os dois casos.

## 5. Fluxo de `GET /handoff`

A ordem importa. Falhas de leitura resultam em `200` com corpo vazio, exceto a recusa de
autorização, que resulta em `403`.

1. Resolve o projeto e autoriza leitura. Se negado: `403` com o motivo no corpo. O cliente
   imprime o motivo em stderr e **nunca** o injeta como contexto.
2. Contexto de run gerenciado (`managed_run`), se válido. Um run inválido, sem run ativo ou
   de outro agente é ignorado sem interromper a requisição.
3. Publica o projeto no ponteiro de projeto ativo, se há dica de escopo (`cwd` ou `project`).
4. Seleção do candidato (seção 3) e renderização do handoff em texto markdown.
5. Bloco de brief opt-in (não destrutivo, recomposto a cada início de sessão).
6. Digest do perfil entre projetos (ver `cross-project-profile.md`).
7. **Decisão de reivindicar:**

| Modo | Comportamento |
|---|---|
| `claim_on_session_start = true` | Consulta a cadeia de admissão para a operação `handoff_accept` com prazo de **750 ms**. Recusa, falha da cadeia ou estouro de prazo: o handoff **permanece aberto**, e o restante (brief, inbox, managed) continua sendo entregue. Aprovação: o handoff segue para a reivindicação do passo 8. |
| `claim_on_session_start = false` | **Não** consulta a cadeia de admissão e **não** reivindica. Gera a nota de oferta (seção 6). |

8. **Reivindicação atômica.** Uma única transação reivindica o handoff (se havia) e o
   contexto de run gerenciado. Ela só ocorre depois que as leituras e renderizações de
   handoff, brief, perfil e run terminaram com sucesso (perder a saída perderia o bastão).
   Dentro da transação:
   - a reivindicação do handoff exige `state = 'open'` na leitura e na atualização (troca
     de estado atômica: só um racer consegue);
   - a regra de sessão viva é reavaliada dentro da transação;
   - uma sessão receptora que já reivindicou um handoff não reivindica outro (idempotência
     de retry: a segunda chamada devolve "não reivindicado" sem erro);
   - a sessão receptora (a do `session_id` do hook) é registrada na **mesma** transação, **antes** da reivindicação, e precisa pertencer ao mesmo workspace, projeto e agente do hook; sessão inexistente ou de outro escopo/agente aborta a transação com erro de estado inválido (`handoff receiver session does not exist` / `… does not match the accepting scope and agent`); uma sessão receptora **já encerrada** é recusada (`an ended session cannot accept a handoff`), com uma única exceção: o Grok reutiliza o identificador de sessão depois de um fim de sessão, e para ele a sessão é reaberta (`ended_at` limpo) se ainda não tiver reivindicado nada;
   - o filtro de dono é conferido na própria atualização de estado (uma única comparação-e-troca `open` → `accepted`); ator identificado só reivindica handoff seu ou sem dono, ator não identificado só reivindica handoff sem dono; combinação incoerente entre filtro de dono e operador receptor aborta com erro de estado inválido;
   - só uma transição real `open` → `accepted` é auditada (`accept_handoff`), e a varredura que expira handoffs automáticos antigos é auditada à parte (`expire_superseded_handoffs`);
   - **se um run gerenciado foi pedido mas não é mais reivindicável**, a transação inteira
     é descartada: o handoff **continua aberto e não é entregue** nesta requisição. Só o
     run gerenciado seria entregue normalmente.
   Em modo oferta, o run gerenciado ainda é reivindicado normalmente. Só o slot único do
   handoff vira oferta.
9. Observadores de admissão só são notificados se a reivindicação foi efetiva. Uma sessão
   concorrente que perde a corrida não dispara nada.
10. Aviso de inbox entre projetos: **somente uma contagem inteira**, sem texto controlado
    por mensagem. A contagem é lida depois da reivindicação e é não fatal: se falhar, o
    aviso é omitido sem falhar a requisição.
11. Concatenação, separadas por linha em branco; partes vazias são omitidas, nesta ordem:
    **perfil → handoff → nota de oferta → contexto gerenciado → brief → inbox**.

Todo conteúdo recuperado (handoff, brief, managed) é envolvido como histórico não confiável,
delimitado por marcadores `<!-- ai-memory:untrusted-history:start -->` e
`<!-- ai-memory:untrusted-history:end -->`, com aviso de segurança.

## 6. Nota de oferta (modo `false`)

Texto exato (uma linha de contexto):

```text
📬 ai-memory: a pending handoff `<id>` from `<from_agent>`, left <idade> ago. To pick it up, call `memory_handoff_accept` with handoff_id `<id>`.
```

- `<idade>`: `just now` (menos de 60 s) ou `N minute(s)|hour(s)|day(s)|month(s)`, com cortes em
  60 s, 3 600 s, 86 400 s e 2 592 000 s (30 dias). O template acrescenta ainda ` ago`
  literal, então a saída real é, por exemplo, "left 3 minutes ago ago" ou
  "left just now ago". Essa redundância é comportamento observável do original;
  reproduzir o texto tal como está, sem normalizar.
- **Somente metadados** (identificador, agente de origem, idade). Nunca resumo, perguntas
  abertas ou próximos passos: o texto do handoff é escrito pelo agente ou operador anterior,
  e uma nota injetada não pode ser descartada pelo receptor da mesma forma que ele pode
  deixar de chamar uma ferramenta.
- Nomeia o **identificador exato**: "aceitar o mais recente" poderia pegar outro bastão que
  chegou depois. Com o identificador exato, uma nota obsoleta apenas perde a corrida; a
  regra de estado aberto decide.
- `memory_handoff_accept` com `handoff_id` reivindica pelo mesmo caminho atômico do modo
  automático (seção 7).

## 7. Reivindicação manual por ferramenta

`memory_handoff_accept`:

- Sem `handoff_id`: pega o mais recente elegível para o `cwd` informado (seção 3).
- Com `handoff_id`: busca esse identificador no escopo. Se já estiver aceito pelo mesmo `session_id` (ou `accepted` quando `session_id` não foi informado), retorna `{ "handoff": null, "status": "consumed_by_hook" }`; se não estiver aberto, retorna `{ "handoff": null, "status": "none_pending" }`.
- Só entrega o objeto `handoff` (`status: "claimed"`) se **esta** chamada venceu a reivindicação. Quem perde a corrida recebe `handoff: null`.
- Antes da reivindicação, consulta a cadeia de admissão `handoff_accept` (sem prazo de 750 ms). Observadores só são notificados em caso de reivindicação efetiva.
- Filtro de dono: por padrão, o operador da chamada. Ver todos os donos (argumento `any_owner: true`, **não** `all_owners`: esse nome só existe nas rotas `/admin/open-sessions` e `/admin/sessions/by-agent`) exige a capacidade administrativa (root em servidor multiusuário) e consome o bastão de outro operador. A ferramenta registra o agente receptor como `other` e não informa sessão receptora (`accepted_by_session` fica vazio). Se o argumento `handoff_id` não é um identificador válido: erro de parâmetros `invalid handoff_id: <erro>`. A consulta de admissão `handoff_accept` só ocorre depois de achar um handoff aberto (a resposta rotineira `handoff: null` não anuncia uma aceitação).
- **Sessão viva e varredura pós-reivindicação:** a ferramenta `memory_handoff_accept` **não** aplica o filtro de sessão viva (`busy_since` é omitido, permitindo aceitar um handoff cuja sessão de origem ainda está aberta), mas **executa** a varredura pós-reivindicação (expirando handoffs automáticos abertos do mesmo dono, compatíveis com o `cwd` e de chave de preferência menor que a do reivindicado, **exceto** os de qualquer sessão de origem ainda aberta: sem `busy_since` não há janela de tempo e toda sessão aberta é poupada; o doc anterior dizia "últimos 5 minutos", o que não existe no original) sempre que um handoff automático é reivindicado, inclusive quando `handoff_id` foi informado.

`memory_handoff_begin` cria handoff manual. `shared: true` o torna visível a todo o
projeto; por padrão, pertence ao operador que o criou. Os argumentos `from_agent`, `to_agent`, `cwd` e `session_id` são aceitos e registrados,
mas `cwd` e `to_agent` não restringem quem recebe handoff manual. `memory_handoff_list` lista handoffs (filtrados por `state`: `"open"` [padrão], `"accepted"`, `"expired"` ou `"all"`, e `limit` 1..100, padrão 20)
sem reivindicá-los, e `memory_handoff_cancel` os marca como `expired`.

## 8. Não implementado (propostas)

- **Opt-out por execução** (variável de ambiente que desliga handoff, ou opção de execução
  que o ignora): não existe. Sessões que não devem consumir handoff dependem do modo oferta
  global. *(proposta)*
- **Direcionamento por agente de destino** (`to_agent`): a coluna existe, mas
  `memory_handoff_begin` não aceita o argumento e a seleção não filtra por ele. *(proposta)*
- **Override por projeto** de `claim_on_session_start`. *(proposta)*

## 9. Cenários de aceitação

1. **Padrão:** a sessão A termina → handoff automático aberto. A sessão B inicia no mesmo
   `cwd` → a resposta contém o handoff e ele fica `accepted`. Uma segunda chamada de
   `GET /handoff` não o devolve mais.
2. **`claim_on_session_start = false`:** após o início da sessão B, o handoff continua
   `open`. O corpo contém a nota com o identificador e **não contém** o resumo (teste
   plantando um resumo hostil e procurando a string). `memory_handoff_accept(id)` o
   reivindica.
3. **Modo oferta + run gerenciado:** o contexto gerenciado é entregue e marcado como aceito.
4. **Sessão de origem ainda aberta com observação nos últimos 10 min:** o handoff não é
   oferecido nem reivindicado. Após a sessão de origem encerrar, ele volta a ser candidato
   (cenário 1 continua válido).
5. **Operador X** não recebe handoff de operador Y em servidor multiusuário. Handoffs sem
   dono continuam visíveis a todos.
6. **Cadeia de admissão recusando `handoff_accept`** (modo reivindicar): o handoff continua
   aberto, e brief e inbox continuam sendo servidos.
7. **`create_on_session_end = false`:** o fim de sessão grava a página de resumo e não cria
   handoff automático nem bastão de checkpoint de turno.
8. **Run gerenciado não reivindicável** no mesmo início de sessão: o handoff permanece aberto
   e não é entregue nesta requisição.
