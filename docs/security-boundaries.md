# Fronteiras de segurança e de isolamento

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

O ai-memory é uma memória de wiki de um único locatário, com atribuição opcional a vários
operadores. Algumas barreiras impedem que um projeto, workspace, operador ou entrada não confiável
atravesse para outro. Este documento é o inventário dessas fronteiras: quem pode o quê, onde o
sistema confina caminhos, onde isola o processo de execução, onde aplica autorização por projeto, e
o mapa dos cenários adversariais que testam cada uma.

Cada barreira só vale tanto quanto o teste que tenta quebrá-la. Um teste de caminho feliz ou de
usuário único não enxerga defeito de isolamento. Por isso cada fronteira abaixo traz a violação que
deve ser recusada e um caso legítimo de controle, para que uma negação geral não seja tomada por
barreira funcionando.

Ameaças, tratamento de dados, credenciais e dados pessoais estão em
[threat-model-and-data-handling.md](threat-model-and-data-handling.md). Este documento não repete
esse conteúdo. As invariantes numeradas citadas aqui estão em [overview.md](overview.md).

## Veredictos de cobertura

- **STRONG**: há cenário adversarial que falharia se a barreira fosse removida, mais um caso de controle.
- **PARTIAL**: só alguns caminhos da barreira são exercitados. A lacuna está descrita na fronteira.
- **FUTURE**: fronteira ainda não construída.
- **WATCH**: não há teste automatizado. Qualquer mudança no caminho exige revisão.
- **MEDIUM**: fronteira com teste, mas o risco residual é pequeno e está justificado na fronteira.

## Resumo

| # | Fronteira | Cobertura |
|---|-----------|-----------|
| 1 | Isolamento por projeto (workspace, projeto e caminho) | STRONG |
| 2 | Isolamento por workspace | STRONG |
| 3 | Escada de autenticação e papéis | STRONG |
| 4 | Handoff: reivindicação única | STRONG |
| 4b | Handoff: recuperação de outro dono restrita a administrador | STRONG |
| 5a | Páginas compartilhadas: autor nunca é filtro de leitura | STRONG |
| 5b | Supersessão de páginas: a versão perdedora continua acessível | STRONG |
| 6 | Ponteiro de projeto ativo por ator, sem sobrescrita | STRONG |
| 7 | Fronteira do sanitizador (texto não confiável) | STRONG (estrutural) |
| 8a | Mensagens: visibilidade só para o destinatário | STRONG |
| 8b | Mensagens: cancelamento só pelo remetente | STRONG |
| 8c | Mensagens: retirada exatamente uma vez | STRONG |
| 8d | Mensagens: leitura por escopo inferido é diagnosticada | STRONG |
| 9 | Resolução de escopo fecha em falha | STRONG |
| 10 | Operações destrutivas: recusa com processo vivo e confirmação | STRONG |
| 10b | Purga de sessão restrita a sessão, workspace e projeto | STRONG |
| 11a | Contrapressão do hook e fan-out limitado | STRONG |
| 11b | Exclusões de captura antes do armazenamento | STRONG |
| 11c | Orçamento de 200 ms da captura | WATCH |
| 12 | Postura de rede e autenticação HTTP | STRONG |
| 13 | Atribuição de transcrição em execuções gerenciadas | PARTIAL |
| 14 | Tempo fornecido pelo cliente, limitado e restrito ao próprio chamador | MEDIUM |
| 15 | Confinamento de caminhos da wiki e de arquivos | STRONG |
| 16 | Isolamento do processo de execução (sandbox e modo de auto-aprovação) | STRONG |
| 17 | Autorização por projeto | PARTIAL |
| 18 | Limites de entrada e validação de requisição | PARTIAL |

## Identidade e papéis

### 3. Escada de autenticação e papéis

Papéis: **anônimo** (nenhuma autenticação configurada), **raiz** (token raiz configurado, ou
identidade de proxy que casa com o par raiz) e **usuário** (credencial de usuário do banco, ou
identidade de proxy que não é a raiz). Credenciais e seus armazenamentos estão no
[threat-model-and-data-handling.md](threat-model-and-data-handling.md) e em
[users.md](users.md).

Garantias:

- Sem token configurado, todas as chamadas passam como anônimas. Um cabeçalho de Bearer enviado
  mesmo assim é tratado como anônimo, não recusado.
- Com token configurado, credencial ausente ou desconhecida recebe 401. Esquema diferente de Bearer
  é ignorado. Mais de uma tentativa Bearer no mesmo pedido falha fechado.
- Token raiz vazio ou só com espaços não ativa a raiz. A comparação do token é feita em tempo
  constante.
- O token do proxy confiável é separado do token raiz. Um pedido só com o token do proxy não vira raiz.
- O proxy afirma a identidade por cabeçalhos. Esses cabeçalhos só valem com o token do proxy. Sem
  ele, são ignorados. Com o token do proxy, cabeçalho repetido, cabeçalho com vírgula, identidade
  ausente ou par emissor/sujeito incompleto recebem 400. A identidade afirmada é raiz só se casar
  exatamente com o par emissor e sujeito configurados. Nome de usuário nunca dá raiz.
- Gestão de usuários é sempre só para raiz, inclusive em instalação sem multiusuário completo.
- Com operadores distinguíveis (há usuários no banco ou há proxy confiável), toda rota administrativa
  é só para raiz. Usuário responde 403 e anônimo responde 401. A checagem é feita a cada requisição,
  sem cache, então o primeiro usuário fecha o acesso de bootstrap sem reiniciar o servidor.
- Sem operadores distinguíveis, a rota administrativa aceita qualquer chamador já admitido pelo
  nível de acesso atual. Isso é aceitável só em instalação local.
- Usuário não recebe papel administrativo. A cadeia de admissão de webhooks pode ser pulada só pela
  raiz e pelo anônimo. Para usuário, o cabeçalho de pulo é ignorado (lista vazia), sem recusar o
  pedido.
- Entrada local sem camada de autenticação (transporte por entrada padrão ou processo local) não tem
  verificação de papel. O pressuposto é que quem tem o diretório de dados já tem acesso a ele.

Cenários adversariais:

- Token raiz errado recebe 401. Esquema diferente de Bearer é ignorado.
- Usuário do banco tenta criar usuário e recebe 403. Anônimo recebe 401 nas rotas multiusuário.
- Usuário do banco tenta rota administrativa multiusuário e é recusado.
- Usuário do banco envia o cabeçalho de pulo da cadeia de admissão e não consegue pular o webhook.
- Cabeçalho de identidade do cliente, sem o token do proxy, não altera a identidade (cenário
  "impostor").
- Proxy envia cabeçalhos de identidade duplicados, com vírgula, ou com par incompleto, e recebe 400.
- Identidade do proxy só vira raiz com o par exato configurado.
- Controle: a raiz com o token correto passa nas rotas administrativas.

Cobertura: STRONG.

### 4. Handoff: reivindicação única

Garantias:

- Um handoff só é aceito se estiver aberto. A transição é uma atualização condicional que exige
  estado aberto e dono compatível, na mesma operação. Um segundo aceitante não rouba um handoff já
  aceito.
- Dono compatível significa: handoff sem dono (compartilhado) aceito por qualquer um; handoff com dono
  aceito só pelo próprio dono. Um leitor sem atribuição não vê handoffs com dono.
- A sessão receptora que já aceitou um handoff não aceita um segundo. Uma nova tentativa do início
  de sessão não consome dois batons.
- Para a maioria dos agentes, uma sessão encerrada não recebe handoff. Um agente que reutiliza o
  identificador após o fim reabre a sessão antes de receber.

Cenários: segundo aceite não rouba o handoff; outro operador não o reivindica nem o cancela; leitor
sem atribuição não vê handoff com dono; nova tentativa de início não entrega dois handoffs.

Cobertura: STRONG. Ver também [design-handoff-offer-not-claim.md](design-handoff-offer-not-claim.md).

### 4b. Handoff: recuperação de outro dono restrita a administrador

O parâmetro que age sobre handoffs de qualquer dono (aceitar ou cancelar) exige capacidade
administrativa. A verificação acontece antes de qualquer efeito: nenhuma reivindicação ocorre e nenhum
webhook de admissão é disparado.

Cenários: aceite com "qualquer dono" por não administrador é recusado sem reivindicar e sem webhook;
cancelamento equivalente também. Controle: o mesmo pedido feito pela raiz passa.

Cobertura: STRONG.

## Isolamento de dados

### 1. Isolamento por projeto (workspace, projeto e caminho)

Toda linha e todo arquivo de domínio carrega a tripla workspace, projeto e caminho. Consultas de
leitura filtram pela dupla workspace e projeto. Remover um projeto não toca dados de outro.

Cenários: sessão irmã no mesmo projeto permanece intacta; leitura em um projeto não vê páginas de
outro com o mesmo caminho.

Cobertura: STRONG. Mapa de resolução de escopo na fronteira 9.

### 2. Isolamento por workspace

Mesmo mecanismo da fronteira 1. Um projeto com o mesmo nome em workspaces diferentes recebe
identificadores distintos.

Cenários: linhas de resolução entre workspaces diferentes não se misturam; a deduplicação e a
validação de múltiplos escopos não juntam projetos de workspaces distintos.

Cobertura: STRONG.

### 5a. Páginas compartilhadas: autor nunca é filtro de leitura

O autor de uma página (campo de atribuição) é juntado à consulta só para exibição. Nunca vira termo
de filtro de leitura. Todos os operadores com acesso ao projeto leem as páginas uns dos outros. Esse
é o invariante 16 do [overview.md](overview.md): escopar leitura de páginas por operador silencia a
colaboração do time e nenhum teste de usuário único detecta isso.

Cenários: página com autor não nulo (operador A) é legível pelo operador B no mesmo projeto.

Cobertura: STRONG.

### 5b. Supersessão de páginas: a versão perdedora continua acessível

Escritas concorrentes no mesmo caminho geram uma cadeia de supersessão. A versão anterior é marcada
como não-última, nunca apagada. O campo de vínculo de supersessão liga as versões.

Cenários: duas escritas concorrentes no mesmo caminho resultam em supersessão, não em destruição; a
versão perdedora continua alcançável pelas leituras que incluem versões superadas.

Cobertura: STRONG.

### 6. Ponteiro de projeto ativo por ator, sem sobrescrita

O ponteiro de "projeto atual" é chaveado pela identidade do chamador. O modo padrão é por ator.
Harnesses paralelos do mesmo usuário têm ponteiros separados. Dois operadores nunca leem o ponteiro
um do outro.

Se a chave já foi usada por outra sessão e a coordenada não bate (divergência), a leitura não cai no
padrão: resolve para "divergente" (falha fechado). Uma escrita sem escopo nesse caso é recusada,
com as duas correções possíveis na mensagem. Sem nenhuma coordenada (chamador anônimo ou legado), a
escrita sem escopo usa o projeto padrão.

Cenários: sessões paralelas do mesmo usuário mantêm ponteiros separados; dois operadores não leem
o ponteiro um do outro; divergência de sessão falha fechado depois que a chave já foi usada.

Cobertura: STRONG.

### 7. Fronteira do sanitizador (texto não confiável)

Observações só entram no armazenamento depois de passar pela remoção de segredos e de dados
pessoais. A garantia é de tipo, verificada na compilação:

- O tipo de observação sanitizada tem o valor interno privado. Só a função sanitizadora o constrói.
- A escrita de observações aceita só esse tipo.
- As operações internas de escrita não são acessíveis fora da camada de armazenamento.

Cobertura: STRONG (estrutural). Há também um cenário de ponta a ponta, em que um segredo passado pela
escrita não chega ao disco, e testes unitários da remoção. Ver
[prompts-schemas-and-sanitizer.md](prompts-schemas-and-sanitizer.md).

## Mensagens entre agentes

Mensagens têm remetente e destinatário, ambos por coordenada de projeto (workspace e projeto), e a
retirada registra a sessão, o agente e o operador que retiraram. Ver
[agent-messaging.md](agent-messaging.md).

### 8a. Mensagens: visibilidade só para o destinatário

A retirada e a listagem da caixa de entrada só enxergam mensagens cujo destino é a coordenada do
projeto consultado. Mensagem endereçada a outro projeto é invisível para essa caixa.

Limite: dentro do projeto destinatário, a fronteira não separa agentes nem operadores. Qualquer
retirada na caixa do projeto recebe a mensagem mais antiga pendente. Se essa separação for
necessária, é uma fronteira nova (a confirmar se a ausência é intencional).

Cenários: destinatário retira a mensagem; agente de outro projeto não consegue retirá-la nem
listá-la; controle: o destinatário vê a mensagem enquanto ela está pendente.

Cobertura: STRONG.

### 8b. Mensagens: cancelamento só pelo remetente

O cancelamento exige que a coordenada do remetente da mensagem seja a do chamador. Cancelar toda a
caixa de saída de outro remetente é recusado. Cancelar por identificador específico de mensagem alheia
também é recusado.

Cenários: destinatário tenta cancelar a caixa de saída do remetente e é recusado; tenta cancelar por
identificador específico e é recusado; controle: o remetente cancela a própria mensagem pendente.

Cobertura: STRONG.

### 8c. Mensagens: retirada exatamente uma vez

A retirada é uma atualização condicional que só muda mensagens pendentes. Duas retiradas sequenciais
ou concorrentes da mesma mensagem entregam exatamente uma vez.

Cenários: retirada sequencial; retiradas concorrentes da mesma mensagem.

Cobertura: STRONG.

### 8d. Mensagens: leitura por escopo inferido é diagnosticada

Quando a leitura usa escopo inferido e a retirada ou listagem volta vazia, a resposta traz uma dica de
diagnóstico. A resposta não é um nulo silencioso.

Cenários: retirada sem escopo explícito que não encontra a mensagem recebe a dica.

Cobertura: STRONG.

Limite de caixa: uma caixa cheia recusa novos envios. Há cenário para isso.

## Escopo e operações destrutivas

### 9. Resolução de escopo fecha em falha

Regras de resolução, comuns a todas as superfícies (rotas administrativas, ferramentas MCP, API e
interface web):

- Leitura, manutenção e operações destrutivas usam busca sem criação. Escopo parcial (só workspace
  ou só projeto) é recusado. Escopo explícito inexistente responde "não encontrado" e não cria nada.
- Só caminhos explícitos de escrita ou criação podem criar workspace ou projeto.
- Escrita sem coordenada explícita usa o ponteiro de projeto ativo do chamador. Se o ponteiro não
  casa com o chamador (divergência), a escrita é recusada, em vez de cair no projeto padrão. Sem
  coordenada alguma (chamador anônimo ou legado), a escrita usa o projeto padrão.
- Rota nova usa o resolvedor central ou os auxiliares explícitos, nunca uma cadeia manual de busca
  de workspace e projeto.

Cenários: tabela de resolução de leitura cobre explícito, parcial, inexistente e controle; escrita
com coordenada não resolvível recebe erro e não usa o padrão; leitura nunca cria escopo.

Cobertura: STRONG.

### 10. Operações destrutivas: recusa com processo vivo e confirmação

Garantias:

- Reset, restore, reindex e uninstall com remoção de dados (flag de purga) consultam os processos do
  sistema antes de qualquer alteração. Se houver outro processo de ai-memory vivo, a operação recusa
  antes de tocar no diretório de dados. O backup não passa por essa checagem: ele continua online,
  pela API de backup do servidor.
- Operações destrutivas exigem confirmação explícita. Sem ela, a purga de projeto responde 400, e a
  prévia (dry-run) mostra os mesmos contadores que a purga confirmada apagaria.
- Purga de projeto e purga de sessão não passam pela checagem de processo vivo. Elas são protegidas
  pela confirmação e pela camada de servidor.
- Prévia (dry-run) não dispara webhook de admissão.

Cenários: com outro processo vivo injetado, cada um dos quatro comandos recusa antes de tocar nos
dados; sem processo vivo, o comando segue (controle). Purga sem confirmação responde 400. Projeto ou
workspace inexistente responde 404. Prévia e purga confirmada reportam os mesmos contadores.

Cobertura: STRONG.

### 10b. Purga de sessão restrita a sessão, workspace e projeto

A purga de uma sessão seleciona só itens dessa sessão, no mesmo workspace e projeto. A sessão é
identificada pelo identificador do próprio resumo, ou pelo caminho canônico da sessão com o
identificador de sessão confirmado no frontmatter do próprio registro. A cadeia recursiva de versões
não decide a propriedade. Se a sessão não pertence ao escopo, a resposta é "não encontrado" e nada é
apagado. Toda a operação roda em uma transação.

Cenários: sessão irmã no mesmo projeto fica intacta; sessão de outro projeto é recusada e nada é
apagado; sessão de outro workspace é recusada; página de mesmo caminho em outro projeto não é
apagada; versões antigas do resumo são removidas, mas a página manual anterior é preservada.

Cobertura: STRONG.

## Hooks e captura

Ver [lifecycle-event-ingestion.md](lifecycle-event-ingestion.md) para o contrato dos hooks.

### 11a. Contrapressão do hook e fan-out limitado

- A ingestão responde de imediato: 202 com o evento na fila, sem esperar o processamento.
- Limite de tarefas em voo, com padrão de 1024. Quando o limite é atingido, a resposta é 429.
- Limite de taxa por fonte, em balde de tokens. Acima do limite de rajada, a resposta é 429.
  A fonte é a sessão do chamador, ou, sem sessão, uma chave formada por agente, evento, diretório e
  conteúdo do pedido. Chaves longas (mais de 128 bytes) são reduzidas a um resumo de tamanho fixo.
- O mapa de chaves de taxa é limitado a 4096 entradas. Sem limite, um fluxo de chaves únicas cresceria
  sem fim.
- Lote com mais de 256 itens é recusado com 413, antes de qualquer processamento.
- Evento descartado por política de captura responde 202 sem consumir capacidade.

Cenários: saturação da fila responde 429; rajada de uma fonte responde 429 e outra fonte, controle,
continua aceita; o mapa de taxa não cresce além do limite; lote acima do máximo é recusado.

Cobertura: STRONG.

### 11b. Exclusões de captura antes do armazenamento

A política de captura decide antes de qualquer fila ou tarefa. Evento excluído responde 202 e é
descartado. Para cada agente suportado, a forma das ferramentas é fechada e as exclusões configuradas
são honradas.

Cenários: por agente, exclusões configuradas são honradas e a forma de ferramenta conhecida é aceita
de forma fechada. Uma forma não reconhecida não é capturada (a confirmar em cada agente).

Cobertura: STRONG (testes por agente).

### 11c. Orçamento de 200 ms da captura

O envio de captura feito pelos scripts de hook tem limite de 200 ms. Se o servidor está fora do ar ou
responde erro de servidor, o evento é gravado em spool para envio posterior. Caminhos com orçamento
maior são separados: busca de contexto com 1 s e drenagem em segundo plano com 2 s. Esse é o invariante
5 do [overview.md](overview.md).

Cobertura: WATCH. O orçamento está nos scripts de hook e não tem teste automatizado. Qualquer mudança no
caminho de captura exige revisão desse número.

## Rede e transporte

### 12. Postura de rede e autenticação HTTP

Garantias:

- O endereço padrão de escuta é loopback. Escutar em endereço não loopback sem token configurado é
  recusado na inicialização, a menos que o operador passe o override explícito. Com o override, o
  aviso é mostrado no terminal (não só no log). Em contêiner, o caso é tratado como indeterminado e
  gera aviso, em vez de recusa.
- Autenticação humana em endereço não loopback exige cookie seguro, isto é, HTTPS atrás de um proxy
  confiável. Cookie não seguro só é permitido em loopback. Ver [https-via-proxy.md](https-via-proxy.md).
- Allowlist de hosts, com padrão localhost, 127.0.0.1 e ::1. Comparação sem diferenciar maiúsculas,
  aceitando host com porta. Sem cabeçalho Host, a resposta é 400. Host fora da lista recebe 403.
  O filtro vale para todo o roteador, inclusive para as rotas internas de introspecção.
- Rotas de máquina (MCP, ingestão de hooks, fluxos de workstream e identidade) exigem token. Sem token
  configurado, o anônimo passa.

Limite: a allowlist defende contra troca de Host pelo navegador (DNS rebinding). Ela não substitui
autenticação, porque um cliente que alcança a porta define o Host livremente.

Cenários: cabeçalho Host ausente recebe 400; Host forjado recebe 403; endereço não loopback sem token
é recusado na inicialização; token errado recebe 401; controle: host permitido, com porta, passa.

Cobertura: STRONG.

## Execuções gerenciadas e tempo

### 13. Atribuição de transcrição em execuções gerenciadas

Várias execuções podem rodar no mesmo diretório de trabalho ao mesmo tempo. Garantias:

- O vínculo com a sessão nativa é gravado só pela execução que a criou. Os dois finalizadores descartam
  o carimbo de vínculo se a sessão mudou. O estado da execução informa se houve vínculo.
- Na conclusão, a execução só adota uma sessão vinculada se ela estiver no diretório de trabalho da
  execução. No harness que grava o diretório da sessão, a verificação usa esse registro.
- A execução nunca volta a um vínculo que ela mesma descartou.
- Sem vínculo, a execução usa a sessão mais recente do diretório.

Cenários: sessão vinculada por uma execução não vira sessão de outra; sessão vinculada durante a
execução vence a descoberta, mesmo quando há sessão concorrente mais nova; vínculo de outro checkout é
recusado sem cair de volta nele; controle sem vínculo.

Cobertura: PARTIAL. Lacuna declarada: uma execução que não vincula nada ainda cai na descoberta da
sessão mais recente do diretório.

### 14. Tempo fornecido pelo cliente, limitado e restrito ao próprio chamador

O campo opcional de horário do evento é aceito só se for positivo e no máximo 5 minutos à frente do
relógio do servidor. Valor inválido, zero, negativo ou fora da faixa é descartado (tratado como
ausente), não rejeitado com erro. O valor afeta só registros da sessão admitida para o próprio chamador
(término de sessão e horário de criação dos registros; campos exatos a confirmar). A deduplicação da
ingestão usa sempre o relógio do servidor.

Cobertura: MEDIUM. Não há superfície entre inquilinos: um cliente já controla o próprio conteúdo, e o
efeito de um horário falso se limita à própria sessão.

## Caminhos, arquivos e processo

### 15. Confinamento de caminhos da wiki e de arquivos

Garantias de caminho da wiki:

- Caminho relativo normalizado: vazio, absoluto, ou com qualquer componente que não seja nome normal
  (inclui referências ao diretório pai e ao próprio diretório) é recusado.
- Cada componente existente do caminho, desde a raiz, deve ser diretório comum. Ligação simbólica ou
  ponto de reparse do Windows é recusado. Arquivo onde se espera diretório gera erro.
- O destino final, se existir e for ligação, é recusado. Se não existir, a operação segue.
- Diretórios faltantes só são criados nos modos de escrita. O modo de inspeção não cria nada e para no
  primeiro componente ausente.
- Varredura da árvore: toda entrada observada que seja ligação é recusada. Entradas que somem durante a
  varredura, inclusive por bloqueio de arquivo no Windows durante uma escrita concorrente, são
  ignoradas, porque não foram observadas. Uma ligação observada continua recusada.
- Metadados do repositório da wiki: diretório comum dentro da raiz. Arquivos de redirecionamento para
  fora da raiz (repositório comum e alternativas de objetos) são recusados antes de o repositório ser
  aberto. A configuração de diretório de trabalho deve resolver para a raiz da wiki, senão é recusada.

Validação de caminhos de página, em duas etapas:

1. Forma: sem vazio, sem barra inicial, sem letra de unidade, sem barra invertida, sem componentes
   `.` ou `..`.
2. Portabilidade: sem caracteres reservados pelo Windows, sem caracteres de controle, sem nome
   terminado em ponto ou espaço, sem nome de dispositivo do DOS, sem nome reservado ao repositório de
   versão.

Caminhos gerados por modelo de linguagem passam antes por uma higienização de portabilidade:
caracteres proibidos viram hífen e `.md` é acrescentado quando falta. Essa higienização cuida de
portabilidade, não de confinamento. O confinamento é garantido pela validação acima.

Arquivos recebidos (atualização, restauração, instalação confinada): entradas com caminho que escape
do diretório de destino são recusadas.

Cenários: ligação simbólica no caminho recusa; ligação no destino recusa; metadados redirecionados
recusam antes de abrir o repositório; diretório de trabalho fora da raiz recusa; caminho com `..`
recusa; backup de árvore com ligação recusa sem gerar arquivo; arquivo compactado com caminho de
travessia recusa; restauração de entrada fora do layout permitido não toca dados vivos; instalação
confinada recusa destino fora da raiz; controle: arquivo regular e entrada que some durante a
varredura passam.

Limite: em Windows sem privilégio para criar ligações, os cenários de ligação são pulados. A cobertura
desse ambiente depende de rodar com o privilégio.

Cobertura: STRONG.

### 16. Isolamento do processo de execução (sandbox e modo de auto-aprovação)

O comando de execução de harness roda o agente diretamente, por padrão. Há dois modos de risco:

- **Auto-aprovação** (`--yolo`): em sessão interativa, mostra o aviso de que cada chamada de ferramenta
  roda sem confirmação e pede confirmação. Enter confirma; resposta negativa aborta antes de iniciar
  a execução. Em execução não interativa, nem o aviso nem a confirmação aparecem.
- **Sandbox externo** (ferramenta ai-jail, ver [design-yolo-safety-ai-jail.md](design-yolo-safety-ai-jail.md)):
  só é oferecido quando o sistema é Linux ou macOS, quando o backend de sandbox do sistema e a
  ferramenta estão disponíveis, e quando uma validação prévia da ferramenta instalada passa. No Windows
  nunca é oferecido.

Escolha do operador (flags de execução):

- `--jail` sem sandbox utilizável falha. A execução não cai para o modo sem sandbox.
- `--no-jail` executa sem sandbox, sem perguntar.
- `--jail` junto com `--no-jail` é erro.
- Com `--yolo` e sem flag de sandbox, a oferta de sandbox só aparece em sessão interativa, depois do
  aviso de auto-aprovação, e só se o sandbox for utilizável.
- Dentro de um sandbox, nada se aplica: sem nova reexecução e sem aviso. A detecção é pelo nome de
  host, no Linux, e pelo prompt, no macOS. Sinal ausente ou não reconhecido é tratado como fora do
  sandbox, então o aviso aparece.
- Quando reexecuta sob o sandbox, o repasse de variáveis de ambiente é uma lista fixa. Só são repassadas
  as que estão definidas. A lista inclui o endereço e o token do servidor, o diretório de dados e as
  chaves de provedores de modelo. Isso é intencional: sem elas, o agente não funciona dentro do
  sandbox. Essas credenciais, portanto, chegam ao sandbox.
- As opções do sandbox vêm antes de um separador. Depois dele, vem o executável e os argumentos
  originais. O conjunto de alternâncias (toggles) pedido é validado contra o que a ferramenta instalada
  aceita. Alternância desconhecida é recusada.
- A invocação sempre inclui uma opção de rede. O efeito exato dessa opção no sandbox é a confirmar.

Cenários: `--jail` sem sandbox utilizável recusa e não executa sem sandbox; `--no-jail` executa sem
sandbox (controle); dentro do sandbox não há reexecução nem aviso; no Windows a oferta não aparece;
resposta negativa ao aviso aborta; alternância não suportada é recusada; repasse de variáveis inclui
só as definidas; a ordem das opções do sandbox é fixa.

Limite: os testes que chamam a ferramenta real dependem dela estar instalada. Sem ela, esses testes
não exercitam o sandbox de verdade (a confirmar a condição de pulo).

Cobertura: STRONG (tabela de decisão e falha fechada). Os cenários de aceitação com sandbox real são
PARTIAL pelo limite acima.

### 17. Autorização por projeto

Cada projeto tem um modo de acesso: **aberto** (padrão) ou **restrito**. Ver o desenho em
[design-per-project-authz.md](design-per-project-authz.md).

Garantias:

- Em projeto aberto, qualquer chamador admitido passa.
- Em projeto restrito, passam a raiz, o criador do projeto e quem tem concessão. Concessão de leitura
  basta para ler. Escrever exige concessão de escrita. Anônimo, ou usuário sem concessão, recebe recusa
  com mensagem que diz o que pedir.
- Sem distinção de operadores (instalação local, sem usuários e sem proxy), o controle é ignorado.
- Escopo global reservado: a leitura é aberta; a escrita exige raiz ou concessão de escrita, em
  qualquer modo. Criá-lo primeiro não concede nada.
- Leitura com dados de acesso ilegíveis degrada para aberto. Isso evita trancar todos os usuários por
  falha de leitura. Mutações de execução gerenciada usam resolução estrita: nesses casos, a falha
  recusa. Usuário do banco que não existe mais também é recusado na via estrita.
- Concessão não é filtro de linha. Todos com acesso ao projeto leem as mesmas páginas (ver 5a).
- Raiz e instalações sem usuários ignoram a checagem por projeto, por desenho.

Cenários: matriz de leitura e escrita por papel (raiz, concessão de leitura, concessão de escrita,
sem concessão, anônimo) em projeto aberto e restrito; criador admitido em projeto restrito; escrita
no escopo global reservado exige raiz ou concessão de escrita; instalação sem distinção de operadores
ignora o controle; a migração deixa todos os projetos abertos e reaplicá-la não muda nada; a resolução
de escopo recusa projeto restrito sem concessão.

Lacunas: o próprio desenho lista pontos em aberto, todos marcados como a confirmar: requisições sem
visualizador, operação de contagem de evidências de página, consolidação de sessão sem alvo
resolvível, imposição estrutural de que buscas sem guarda fiquem privadas ao módulo, e concessão no
escopo global. Por isso a cobertura é PARTIAL.

Cobertura: PARTIAL.

### 18. Limites de entrada e validação de requisição

Limites de tamanho e de quantidade, aplicados antes do processamento:

- Corpo de requisição: 10 MiB nas rotas de máquina. Exceção: a rota de bootstrap administrativo aceita
  até 32 MiB.
- Lote de ingestão de hooks: no máximo 256 itens (ver 11a).
- Restauração de ativos de agentes: manifesto de até 4 MiB, ativo de até 128 MiB, arquivo de até
  512 MiB.
- Marcador de captura lido pelos hooks: até 64 KiB. Acima disso, a leitura é recusada.
- Dica de escopo de captura: até 512 bytes.
- Chave de ingestão: de 1 a 64 caracteres, letras e dígitos ASCII, hífen ou sublinhado. Chave inválida
  é tratada como ausente (sem erro 4xx), e a ingestão segue como se não houvesse chave.
- Nome de usuário usado em caminho: até 64 bytes, só minúsculas ASCII, dígitos, ponto, hífen e
  sublinhado, sem ponto final. Qualquer outro nome, e toda identidade OIDC, vira um segmento derivado
  de forma determinística da chave de armazenamento. Isso evita que dois nomes diferentes colidam em
  sistemas de arquivos que ignoram caixa.
- Chave de identidade de armazenamento é validada antes de ser aceita na fronteira de armazenamento.

Cenários: lote acima do máximo é recusado; chave de ingestão inválida não gera erro; nomes de usuário
que diferem só em caixa geram segmentos distintos.

Limite: os limites de corpo para cada rota e os de tamanho de importação não têm cenário
adversarial verificado nesta revisão (a confirmar). A cobertura é PARTIAL.

Cabeçalho Host e autenticação das rotas estão na fronteira 12.

## Protocolo de manutenção

1. **Mudou uma barreira**: acrescente ou estenda um cenário adversarial que falharia sem a barreira,
   com o caso de controle, e atualize a linha da tabela.
2. **Nova fronteira** (nova dimensão de isolamento, nova capacidade ou nova superfície entre escopos):
   nova entrada neste documento e seus cenários no mesmo change.
3. **Entrada sem escopo ou por identificador bruto é suspeita até ser testada.** Um handler que recebe
   identificador de sessão, execução, página ou mensagem sem passar pela resolução de escopo, ou que
   varre projetos (escopo global, listagem recente, busca), precisa resolver e autorizar antes, ou
   filtrar antes do limite. Também precisa de cenário que prove a recusa de identificador ou escopo
   estrangeiro.
4. **Prove que o cenário morde.** Um cenário que continua passando com a barreira removida não é teste
   de fronteira. O correto é falhar sem a barreira e passar com ela.
5. **Testes unitários não enxergam estes defeitos**, porque exercitam uma sessão ou um locatário por
   vez. Os cenários de fronteira ficam em testes de integração, com mais de uma sessão, mais de um
   operador e mais de um projeto.
