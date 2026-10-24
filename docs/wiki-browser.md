# Navegador web do wiki e API JSON somente leitura

> **Papel:** duas superfícies somente leitura sobre o mesmo estado do
> armazenamento e do wiki:
> 1. **Navegador HTML** renderizado no servidor, montado sob um prefixo
>    configurável (padrão `/web`).
> 2. **API JSON** em `/api/v1`, para frontends de terceiros (inclusive uma
>    interface própria servida pelo próprio servidor).
>
> **Nunca escreve conteúdo:** nenhuma rota aceita mutação de páginas. A única
> ação de escrita visível é aprovar ou rejeitar propostas pendentes, e a página
> de pendências envia essa ação a rotas de administração fora deste componente.
> Um editor web é um [produto companion](companions.md) separado, não parte
> deste componente.

Depende apenas de leitura: da camada de leitura do armazenamento e da camada
de wiki. A autenticação por sessão ou token é aplicada pelo servidor que monta
o componente, não por ele.

---

## 1. Componentes

| Componente | Responsabilidade |
|---|---|
| Rotas HTML | Páginas do navegador embutido: início, projeto, página, busca, pendências, login e troca de senha. |
| Rotas da API | Respostas JSON, cabeçalhos de cache e ETag. |
| Renderização de markdown | Conversão para HTML sanitizado e reescrita de wikilinks. |
| Montagem | Prefixo, base de URLs, divisão entre rotas públicas e protegidas, interface customizada e injeção no HTML. |
| Redirecionamento de autenticação | Converte respostas 401 e 403 de navegação em redirecionamentos para as páginas de login e de troca de senha. |
| Recursos estáticos | Folha de estilo e logo embutidos no executável. |

---

## 2. Rotas HTML

Caminhos relativos ao ponto de montagem (normalmente `/web`).

| Método e rota | Conteúdo |
|---|---|
| `GET /` | Cards de projetos. |
| `GET /w/{workspace}/{projeto}` | Árvore de páginas e atividade recente. |
| `GET /w/{workspace}/{projeto}/p/{caminho}` | Página renderizada e metadados; ou listagem de namespace quando o caminho é uma pasta (2.3). |
| `GET /w/{workspace}/{projeto}/h/{caminho}` | Histórico de versões da página e diff (2.8). |
| `GET /w/{workspace}/{projeto}/s/{id_da_sessão}` | Linha do tempo de uma sessão (2.9). |
| `GET /search?q=` | Lista de resultados da busca, global ou de um projeto (2.4). |
| `GET /pending` | Triagem de propostas pendentes (somente raiz). |
| `GET /health` | Painel de saúde do servidor (somente raiz, 2.10). |
| `GET /login` e `GET /change-password` | Formulários públicos de autenticação. |
| `GET /static/...` | Folha de estilo e logo embutidos, públicos. |
| `GET /favicon.ico` | Ícone, montado na raiz do host e não sob o prefixo nem sob `/web`, pois o navegador busca esse caminho na origem. Só existe quando a interface web está habilitada. |
| `GET {slug}/` | Redirecionamento permanente com status **308** (não 301: método e corpo são preservados) para `{slug}`, preservando a query string. Ver 5.4. |

Todas as rotas HTML são só `GET`. Rotas inexistentes caem no tratamento de
rota desconhecida do servidor (sem página 404 estilizada: a página "Page not
found" só existe para páginas, projetos e sessões não encontrados).

A interface web é desligada por padrão. O prefixo do navegador (`/web`) vem do
parâmetro de slug e pode ser trocado por variável de ambiente.

### 2.0 Elementos comuns a todas as páginas

- A interface está em **inglês** (`lang="en"`), com os textos literais citados
  neste documento. Não há internacionalização.
- Toda resposta HTML do navegador (e da API) leva `X-Frame-Options: DENY` e
  `Content-Security-Policy: frame-ancestors 'none'`, a menos que o tratador da
  rota já tenha definido o cabeçalho. Motivo: com autenticação desligada, uma
  página emoldurada por outro site poderia levar o visitante a clicar em
  Approve/Reject da página de pendências. O `favicon.ico` da raiz do host não
  passa por essa camada.
- Páginas HTML não trazem `Cache-Control` (exceto `/health`: `private,
  no-store`).
- Cabeçalho fixo: logo (`static/logo.png`) com o texto `ai-memory` (oculto em
  telas estreitas) como link para a página inicial (`.`); caixa de busca com
  rótulo oculto "Search pages" e dica `Search…` (nas páginas de um projeto, a
  caixa é escopada: rótulo "Search pages in {ws}/{proj}", dica `Search
  {ws}/{proj}…`, e envia `workspace` e `project` ocultos); links de navegação
  `Pending` e `Health`; botão `Log out`.
- `Log out`: faz `POST {prefixo-de-base}/auth/logout` com credenciais e o
  cabeçalho `X-CSRF-Token` lido do cookie `ai_memory_csrf` (decodificado); ao
  terminar (com sucesso ou não) navega para o caminho de login. O caminho de
  login vem da `<meta name="ai-memory-login-path">` injetada (5.3); se
  ausente, `{prefixo-de-base}/web/login`. O prefixo de base vem da
  `<meta name="ai-memory-base-path">`.
- Link "Skip to content" (`#main-content`): um pequeno script reescreve o `href`
  para a URL absoluta atual sem fragmento + `#main-content`, porque a base
  injetada levaria o âncora nua à raiz do navegador. Se a URL atual começa com
  `//`, o uso da URL absoluta evita virar link para outro host.
- Rodapé: `ai-memory · read-only` e `v<versão do servidor>`.
- Título da aba: `<título específico> — ai-memory`.
- Datas relativas ("humanizadas", em inglês): diferença absoluta entre agora e
  o instante; menos de 60 s: `just now`; menos de 60 min: `N minute(s) ago`;
  menos de 24 h: `N hour(s) ago`; menos de 30 dias: `N day(s) ago`; depois
  `N month(s) ago` (meses = dias ÷ 30, divisão inteira) até 12; depois `N
  year(s) ago` (anos = meses ÷ 12). Singular quando N = 1. Datas futuras usam o
  valor absoluto (também "ago"). Instante não interpretável: mostra o texto
  bruto. O instante completo aparece no atributo de dica (`title`).
- Identificador curto (sessões, versões): os **últimos 8 caracteres** do UUID
  (os primeiros são um prefixo de tempo compartilhado entre ids próximos).

### 2.1 Página inicial

- Título `Projects` e subtítulo `All workspaces and projects in your wiki.`
- Um card por projeto, com workspace, nome do projeto, quantidade de páginas
  (`1 page` / `N pages`) e data relativa da última atualização (omitida quando
  o projeto não tem páginas). Conta só páginas atuais e não expiradas. Ordem:
  atualização mais recente primeiro, projetos sem páginas por último. O card
  inteiro é um link para a árvore do projeto. Se o projeto tem mensagens
  pendentes na caixa de entrada, o card mostra o selo `N inbox` (dica
  "Messages from other projects waiting for an agent here"); com zero, sem
  selo.
- Faixa de correio, só quando a soma das mensagens pendentes dos projetos
  listados é maior que zero: `N message(s) waiting for agents` e `in
  <ws/proj> (n), <ws/proj> (n)…`, com no máximo 5 projetos (mais mensagens
  primeiro; empate por workspace e depois projeto); se houver mais, termina
  com ` and N more project(s)`. A faixa usa só os cards visíveis, então nunca
  cita um projeto que a grade não mostra.
- Aviso fixo `This is LLM-optimised memory, not hand-curated documentation.`
  seguido de um parágrafo explicando que as páginas são compiladas por LLM a
  partir de observações de sessão, que a interface principal são as
  ferramentas MCP (`memory_query`, `memory_recent`, `memory_explore`, …) e que
  o navegador serve para inspecionar o que foi registrado, não como
  documentação. O botão de fechar (rótulo "Dismiss permanently", dica "Don't
  show this again") grava `1` na chave `ai-memory-llm-notice-dismissed` do
  armazenamento local do navegador; com a chave em `1` o aviso não aparece.
  Armazenamento bloqueado: o aviso aparece sempre.
- Sem nenhum projeto: `No projects yet — wire up a hook and start a session.`
- Quando a autorização por projeto está ativa e o visualizador não é raiz, só
  aparecem os projetos que ele pode ler. Sem visualizador identificado (instalação
  sem usuários de banco, ou raiz), aparecem todos.
- Diálogo da migração para o formato 2.0 (ver [okf.md](okf.md)): aparece
  **sempre que existe o registro (recibo) da migração** no diretório de dados,
  mesmo depois de o arquivo de backup ter sido apagado (aí o texto muda).
  Título `Your memory was upgraded to the 2.0 format`; informa a data da
  migração e o formato (Open Knowledge Format, OKF v0.2).
  - Com o arquivo de backup presente: mostra o caminho absoluto e o tamanho
    (unidades `B`, `KB`, `MB`, `GB`, base 1024; bytes inteiros abaixo de 1024,
    senão uma casa decimal, ex. `1.5 MB`) e três passos de reversão (parar o
    servidor; mover o diretório de dados atual; descompactar o arquivo como
    diretório de dados e iniciar a versão anterior), além de dizer que o
    arquivo pode ser apagado quando tudo estiver certo.
  - Com o arquivo ausente: diz que o backup verificado foi apagado e que o
    histórico git da wiki ainda guarda o commit de checkpoint
    `pre-okf-migration`.
  - **Descarte (resolve a pendência):** caixa `Do not show me again` + botão
    `Got it` (a tecla Esc equivale a fechar). Se a caixa estiver marcada ao
    fechar, grava `1` no armazenamento local do navegador sob a chave
    `ai-memory-okf-dialog-<data da migração>`; uma migração posterior tem outra
    data e reexibe o diálogo. Fechar sem marcar não grava nada (reaparece na
    próxima visita). Armazenamento bloqueado: o diálogo aparece sempre. O foco
    vai para o botão ao abrir, volta ao elemento anterior ao fechar, e Tab
    fica preso entre a caixa e o botão.

### 2.2 Projeto (árvore)

- Páginas de conhecimento são agrupadas em pastas pelo primeiro segmento do
  caminho. Páginas sem barra no caminho ficam no grupo `(root)`.
- Páginas de sistema ficam em uma seção `Sistema`, recolhida por padrão. São
  consideradas de sistema:
  - caminhos que começam com `_`, exceto os que começam com `_rules/`;
  - caminhos que começam com `sessions/`;
  - na raiz, os arquivos `index.md`, `_meta.md` e os que seguem o padrão `log-*.md`.
- A seção "Atividade recente" mostra só páginas de conhecimento, ordenadas da
  mais recente para a mais antiga pela data de atualização, limitadas a 20.
  Cada linha: título (link), tipo, caminho e data relativa. Sem páginas:
  `No pages yet.`
- Cabeçalho da página: breadcrumb `home / <workspace> / <projeto>`, título
  `<workspace>/<projeto>` e o formulário `Search this project` (campo `q`,
  caixa `Explain ranking` que envia `explain=1`, botão `Search`), que envia
  `workspace` e `project` para a rota de busca.
- Barra lateral `Pages`: para cada pasta, `nome/ (total)` e as **5 páginas mais
  recentes** da pasta (ordem: atualização decrescente, empate por caminho
  crescente), exibidas pelo título. O nome da pasta é link para a listagem de
  namespace (`…/p/<pasta>/`, ver 2.3) e, quando a pasta tem mais de 5 páginas,
  aparece `View all N`. O grupo `(root)` não tem namespace: lista **todas** as
  suas páginas e não tem link. Sem páginas: `No pages yet.` A seção `System`
  (elemento recolhível, fechado por padrão) repete a mesma estrutura para as
  páginas de sistema.
- Seção `Mailbox` (somente leitura: nunca consome nem cancela a fila, que é
  das ferramentas dos agentes). Mostra até **5** mensagens pendentes do projeto,
  as mais antigas primeiro. Cada item: assunto (vazio ou só espaços vira `(no
  subject)`), remetente como `from <ws>/<proj>` (link para o projeto) ou, quando
  o visualizador não pode ler o projeto remetente, `from a project you cannot
  read` (sem link), trecho do corpo e data relativa. O trecho junta as
  palavras com um espaço, pega os primeiros **160 caracteres** e acrescenta `…`
  se houver mais. O selo de contagem é `N pending`, ou `n of N pending` quando
  a lista foi cortada (a contagem total nunca é menor que as linhas
  mostradas). Rodapé: `An agent in this project reads them with
  memory_message_pop.`; quando cortada, antes disso: `Showing the oldest n of
  N; ai-memory message list --workspace <ws> --project <proj> --limit 200 lists
  up to 200.` Sem mensagens: `No pending messages for this project.`
- Seção `Recent Sessions` (só se houver sessões): até **10** sessões, abertas
  inclusive, pelo mesmo leitor com filtro de dono das sessões da API
  (4.12 de [frontend-api.md](frontend-api.md)): uma sessão de outro operador
  não é listada. Cada item (link para a linha do tempo, 2.9): tipo de agente,
  id curto, `N observation(s)` (e ` · open` se ainda aberta) e data relativa do
  início.
- Projeto inexistente: as listas de páginas, correio e sessões saem vazias e a
  página responde 200 quando não há visualizador restrito (as listas são
  buscadas pelo nome exato); com visualizador restrito, o projeto inexistente
  responde a página 404.

### 2.3 Página

- Metadados exibidos: tipo, tier, indicação de fixada, datas de criação e
  atualização (relativas), link para a página substituída (quando houver) e
  autor (nome de usuário, nome e e-mail como link `mailto`).
- Também aparecem: o selo `expired` quando a página já passou do prazo de
  validade (a leitura direta por caminho **não** esconde páginas expiradas,
  diferentemente das listas e da busca), o link `History` (2.8) e as datas
  como `Updated <relativa>` e `Created <relativa>`. A data de criação é a da
  **versão mais antiga** da cadeia de substituições (a primeira versão
  retida), não a da versão atual. `Supersedes <caminho>` é link para a página
  substituída. Autor: `Last edited by <usuário> (<nome>) <e-mail>`; nome e
  e-mail só quando existem, e-mail como `mailto:`.
- Antes de renderizar, remove-se o primeiro H1 do corpo quando o texto dele é
  igual ao título da página. Regra exata: ignora quebras de linha iniciais;
  formato `# Título` (um `#` seguido de espaço, nunca `##`): compara o texto da
  linha, sem espaços nas pontas, **com igualdade exata e sensível a caixa** com
  o título (também sem espaços nas pontas); se igual, remove a linha e as
  quebras de linha seguintes, senão mantém tudo. Formato sublinhado: a
  primeira linha, aparada, igual ao título, seguida de uma linha só de `=`
  (uma ou mais); remove as duas linhas. Qualquer outro H1 permanece.
- **Barra lateral de navegação** (a página HTML mostra, sim, as relações):
  - `On this page`: índice dos cabeçalhos de nível 2 e 3 (nível 3 recuado), com
    âncoras iguais aos ids do 4.4. Só aparece se houver **2 ou mais** itens.
  - `Cited by` (backlinks) e `Cites` (links de saída): as páginas mais recentes
    e não expiradas ligadas a esta, sem repetição, ordenadas por workspace,
    projeto e caminho, **no máximo 10** de cada direção, mais `and N more`
    quando há mais. Páginas de projetos que o visualizador não pode ler são
    omitidas da lista **e da contagem**. Uma página de outro projeto mostra o
    rótulo `workspace/projeto`. `Cited by` vazio: `No page links here yet.`;
    `Cites` vazio some. A API devolve as listas completas (sem o limite de 10).
- Se o caminho não corresponde a uma página, mas existem páginas (atuais e não
  expiradas) sob `caminho/`, a resposta é uma **listagem de namespace** (a barra
  final do caminho é aparada; caminho vazio dá 404):
  - Título `<pasta>/`, `N page(s) in this namespace.` e, quando a paginação
    corta a lista (total maior que os itens da página), `Showing a–b.`
  - Ordenação pelo parâmetro `sort`: `title` (título sem diferenciar caixa;
    empate por atualização decrescente e depois caminho) ou qualquer outro
    valor/ausente = `updated` (atualização decrescente, empate por caminho).
    Links `Recently updated` e `Title`.
  - 25 itens por página, parâmetro `page` (1 em diante). Valor não numérico,
    zero ou ausente vira 1; acima da última página vira a última. Links
    `← Previous` / `Next →` (`rel=prev/next`) preservam `sort`.
  - Cada item: título (link), tipo, caminho e data relativa. Parâmetros
    inválidos nunca geram 400: caem no padrão (uma query que nem decodifica,
    como chave repetida, também é ignorada em qualquer página).
  - Sem páginas sob o caminho: página 404.
- Se o visualizador não pode ler o projeto, a resposta é 403 em **texto
  simples** com a mensagem de recusa (ver os textos em
  [frontend-api.md](frontend-api.md), seção 3). Projeto inexistente (quando há
  visualizador restrito) recebe a página 404 comum. Página 404: `404`, `Page not
  found`, `The page you're looking for doesn't exist or has been superseded.`
  e link `Back to home`.
- Arquivo ausente no wiki para uma página existente no índice: 404 (mesma
  página). Falha interna: 500 sem corpo.
- **Caminho inválido (resolvido):** a consulta é feita pelo texto exato do
  caminho **antes** de qualquer validação. Como nenhum caminho inválido existe
  no índice, ele cai no tratamento de namespace e termina em 404 (ou numa
  listagem, se for um prefixo válido). O 400 só ocorreria para um caminho
  inválido que existisse no índice, o que não acontece. Logo: caminho
  inválido = 404.

### 2.4 Busca

Parâmetros (todos de query string; valores ausentes ou vazios após aparar
contam como "não informado"):

| Parâmetro | Regra |
|---|---|
| `q` | Termo, aparado. Vazio: não executa consulta; a página mostra `Enter a search term above.` |
| `scope` | `workspace/projeto` vindo do seletor da página; vazio = todos os projetos. **Vence** `workspace`+`project` quando os dois vêm. Sem barra, ou com parte vazia: 400 em texto simples `scope must be workspace/project`. |
| `workspace`, `project` | Só valem juntos. Um sem o outro: 400 em texto simples `workspace and project must be provided together`. |
| `kind` | Tipo de página; vazio = qualquer. Tipos conhecidos, nesta ordem: `rule`, `slot`, `session`, `decision`, `gotcha`, `concept`, `procedure`, `note`, `preference`, `fact` (os dez que a tabela de tipos por prefixo de caminho de [frontend-api.md](frontend-api.md) 4.6 pode produzir). A comparação do filtro com o tipo da página ignora diferença de caixa ASCII. Valor fora da lista conhecida é aceito (tipo personalizado de frontmatter) e mantido no seletor; sem resultados, a página avisa `<valor> is not a standard page kind; check the spelling.` |
| `tier` | `working`, `episodic`, `semantic` ou `procedural`, sem diferenciar caixa (a forma canônica é a usada). Outro valor: 400 em texto simples `tier must be working, episodic, semantic or procedural`. |
| `limit` | Inteiro; faixa 1 a 100 (valor fora da faixa é ajustado, não rejeitado). Padrão **50** na busca entre projetos e **20** na busca de um projeto. Valor que não é inteiro não negativo: a query inteira não decodifica e a resposta é 400 do servidor HTTP. |
| `explain` | `1`, `true`, `on` ou `yes` (minúsculas) liga o rastro de ranking. |

Execução:

- **Busca de um projeto** (`scope` ou `workspace`+`project`): o escopo é
  resolvido sem criar nada e com checagem de leitura **antes** de qualquer
  ranking: projeto inexistente = página 404; sem permissão = 403 em texto
  simples. Usa o mesmo ranqueador híbrido da ferramenta de consulta de memória
  (texto completo, entidades, vetores se houver embedder, grafo, observações,
  autoridade e reranqueador opcional), de modo que a ordem é a mesma da busca
  MCP para o mesmo projeto. Filtros `kind`/`tier` entram **dentro** de cada
  fluxo, para que um tipo raro não seja engolido por outros; os sobreviventes
  são reordenados entre si (a ordem pode diferir levemente da busca sem
  filtro). Ver [storage-and-retrieval.md](storage-and-retrieval.md).
- **Busca entre projetos** (sem escopo): só texto completo, sobre todos os
  projetos legíveis pelo visualizador (como a consulta MCP global). Não há
  detalhes por fluxo.
- Pede-se um resultado a mais que o limite para saber se a lista foi cortada.
- Sem termo, nenhuma consulta é feita, mas o seletor e os filtros continuam
  sendo mostrados.

Página:

- Breadcrumb `home / [<ws>/<proj> /] search`; título `Search: <termo>` (ou só
  `Search`); em busca de projeto, o selo `in <ws>/<proj>` com um `×` que leva à
  busca entre projetos mantendo `q`, `kind` e `tier`.
- Formulário: seletor `Search in` (`All projects` + um grupo por workspace, em
  ordem alfabética, com os projetos em ordem alfabética; só projetos legíveis
  pelo visualizador), `Kind` (`Any kind` + os dez tipos conhecidos na ordem
  acima, mais o valor atual se for personalizado), `Tier` (`Any tier` + os
  quatro tiers na ordem `working`, `episodic`, `semantic`, `procedural`), campo
  `Search pages`,
  caixa `Explain ranking` e botão `Search`.
- Resultado: título (link para a página), trecho, `workspace/projeto · caminho`.
  O trecho vem do índice com marcações `<mark>`; é escapado por inteiro (`&`,
  `<`, `>`, `"` e `'` viram entidades) e só os pares `<mark>` e `</mark>`
  exatos são restaurados.
- Contagem: `N result(s)` (singular quando 1), ou, se cortada,
  `Showing the first <limite> results. Refine your search to narrow them.`
- Sem resultados: `No results for <termo>` + ` with these filters` se havia
  filtro + o aviso de tipo desconhecido + (em busca de projeto) o link `Search
  all projects`.
- **Rastro de ranking** (`explain` ligado e termo não vazio), seção `Recall
  trace`: em busca de projeto, `Hybrid ranker (the one memory_query uses for a
  project). Streams: <fluxos ativos>`, `Embedder: <provedor / modelo (dim N)
  ou none> · Reranker: <nome / modelo ou none>` e tabela `Stream | ms` com o
  tempo de cada fluxo que rodou (`fts`, `entity`, `vector`, `graph`,
  `observation`, `rerank`; duas casas decimais). Em busca entre projetos:
  `Cross-project search is FTS5 only, like memory_query(global=true), so hits
  carry no per-stream details. Search from a project page to see the full
  trace.` e só o tempo `fts`. Por resultado (só em busca de projeto): `#<posição>
  · rank <5 casas> · fused <5 casas> · authority <4 casas>` e, se houver, `
  · intent boost <3 casas>` e ` · rerank <3 casas>`; tabela `Stream | Rank |
  Raw score | RRF` com linhas `fts`, `entity`, `vector`, `abstract` (só se
  teve posição ou contribuição), `graph`, `observation` (idem); posição
  ausente, score ausente e contribuição zero aparecem como `—`; score com 4
  casas, contribuição com 5; `Matched entities: a, b` e `Reached through:
  <página-semente> (<direção>[, <aresta>])` quando existirem. Sem embedder, o
  fluxo vetorial e o reranqueador ficam fora (o MVP sem LLM/embeddings mostra
  só `fts`, `entity`, `graph` e `observation`).

### 2.5 Pendências

- Página exclusiva da raiz (administrador).
  - Sem autenticação: 401. A navegação transforma essa resposta em redirecionamento
    para o login (seção 2.6).
  - Autenticado, mas sem privilégio de administrador: 403 com página de
    "administração necessária". Essa resposta **não** redireciona para a troca
    de senha.
- Lista propostas pendentes de todos os projetos, com limite de 500 por
  carregamento. Se houver mais, a página indica truncamento e aponta a interface
  de linha de comando para filas maiores.
- Filtro por projeto: a chave é `workspace/projeto` com cada nome
  percent-encoded. Chaves que não correspondem a um projeto com pendências são
  ignoradas, e a seleção volta para "todos os projetos".
- Ordenação: `project` (padrão, por projeto e depois por data de envio),
  `confidence-asc` (confiança crescente), `confidence-desc` (confiança
  decrescente) e `staged-desc` (envio mais recente primeiro). Valor
  desconhecido usa o padrão.
- Para cada proposta são exibidos: alvo com link, operação, se reescreve uma
  página existente (qualquer operação diferente de criação), se é regra (alvo
  que começa com `_rules/`), confiança em percentual inteiro (arredondado),
  data relativa de envio e conteúdo.
- Cabeçalho `Pending writes`; contagem `N shown · M pending in K project(s)`
  e, quando truncada, `· only the oldest 500 of this selection are listed; pick
  a project, or use ai-memory pending-writes list`. Seletores `Project` (`All
  projects (total)` + `workspace/projeto (n)`; nomes com `/` aparecem entre
  aspas, ex. `"a/b"/"c"`) e `Sort` (rótulos `Project, then staged date`,
  `Confidence, low first`, `Confidence, high first`, `Staged, newest first`);
  os seletores reenviam o formulário ao mudar. Desempate das ordenações:
  `project` = rótulo do escopo e data de envio; `confidence-*` = confiança,
  depois escopo, depois data de envio; `staged-desc` = envio decrescente, depois
  escopo. Sem propostas: `No pending proposals.`
- Cada proposta é um bloco recolhível com título, operação, tipo, escopo, alvo,
  `confidence N%` e `staged <relativa>`. Aviso extra para alvos em `_rules/`
  ("Every agent session loads those pages as instructions") e para reescritas
  (link `current page`). Mostra `Rationale`, `Proposed body (<modo de edição>)`
  e o id.
- Aprovar/rejeitar (a página não escreve nada): os botões `Approve` e `Reject`
  (com campo `Reject reason (optional)`, máximo 500 caracteres) fazem `POST
  {prefixo-de-base}/admin/pending-writes/{id}/approve|reject?workspace=…&project=…`
  com o cabeçalho `X-CSRF-Token`, corpo `{}` (aprovar) ou `{"reason": …}`
  (rejeitar). Sucesso remove o item e mostra `Approved <caminho>.` /
  `Rejected <caminho>.`; 409 mostra que a página mudou depois da proposta e que
  ela virou conflito; outras falhas mostram `<Approve|Reject> failed for
  <caminho> (<status>): <erro>`; falha de rede: `Network error. Try again.`
- Navegador autenticado só com o token raiz (sessão "legada", que só lê): no
  lugar dos botões, a página mostra os comandos `ai-memory pending-writes
  approve|reject <id> --workspace <ws> --project <proj>` em duas formas, POSIX
  (argumentos simples sem aspas; com aspas simples e `'\''` quando há
  caracteres fora de `[A-Za-z0-9-_./:@]`) e PowerShell (sempre entre aspas
  simples, apóstrofo duplicado), além de um aviso para configurar o login por
  senha.
- Página 403 de "administração necessária": `403`, `Root access required`,
  `Only the root account can approve or reject pending writes.`, link `Back to
  home`.

### 2.6 Autenticação nas páginas HTML

Aplica-se apenas a navegação: requisições GET ou HEAD que indicam documento
(cabeçalho de destino `document` ou `Accept` contendo `text/html`) e que não
são de `/api/v1`.

| Resposta original | Resultado para a navegação |
|---|---|
| 401 que traz um desafio `WWW-Authenticate` do tipo **Basic** (servidor só com token e sem contas humanas, o caso de toda instalação nova) | **Sem redirecionamento**: o navegador recebe o 401 com o desafio e mostra o seu diálogo de senha; o token do arquivo de token vale como senha. Redirecionar deixaria o navegador sem como entrar. |
| Outro 401 | Redirecionamento 303 (See Other) para o login (`{web_root}/login`, ou `/login` com o navegador montado na raiz), com `?next=` contendo o caminho externo completo (prefixo de base + caminho + `?query`), sanitizado como abaixo e codificado em percentual (letras, dígitos e `-_.~` ficam; `/`→`%2F`, `?`→`%3F`, `=`→`%3D`, `&`→`%26`, todo outro byte `%XX` maiúsculo). |
| 403 comum | Redirecionamento 303 para a troca de senha (`{web_root}/change-password`; sem `next`). |
| 403 de administração (página de pendências e de saúde) | Mantém a página de 403. |
| Qualquer outra | Sem alteração. |

O desafio Basic é detectado em qualquer cabeçalho `WWW-Authenticate` da
resposta cujo esquema (comparado sem diferença de caixa) seja `Basic`.
Requisições `HEAD` seguem a mesma regra do `GET`. Toda requisição cujo caminho
(dentro do prefixo de base) seja `/api/v1` ou comece por `/api/v1/` fica fora
desta regra.

O parâmetro `next` (usado no login e na troca de senha; ausente ou só com
espaços vira a raiz do navegador) é aparado e aceito somente se:
- não contém `://`, não começa com `//`, não contém barra invertida nem byte nulo;
- começa com `/`;
- o caminho (antes de `?` ou `#`) é a raiz do navegador ou está abaixo dela;
- não contém segmentos `.` ou `..`.

Qualquer outro valor é trocado pela raiz do navegador, ou por `/` quando o
navegador está montado na raiz do host.

### 2.7 Tema

O tema segue a preferência do sistema (modo escuro automático). Não há seletor
nem preferência salva. Os scripts embutidos nas páginas, todos com degradação
elegante (as páginas funcionam sem eles), são: o botão de sair (2.0); o ajuste
do link "Skip to content" (2.0); o aviso e o diálogo da página inicial (2.1);
a página de pendências (2.5); e o controle de navegação da linha do tempo de
sessão (2.9). Nenhum carrega recurso externo; a folha de estilo e o logo são
embutidos (seção 6).

### 2.8 Histórico de uma página (`/w/{ws}/{proj}/h/{caminho}`)

Mostra as versões guardadas de um caminho, da mais nova para a mais antiga,
e um diff de linhas entre duas delas. Só lê. O mesmo dado sai em JSON em
[frontend-api.md](frontend-api.md) 4.14.

- Escopo resolvido sem criar nada e com checagem de leitura; a recusa vira 403
  em texto simples e o projeto inexistente vira a página 404.
- Caminho: validado como caminho de página; inválido = 400 em texto simples
  `invalid path: <motivo>`. Sem nenhuma versão visível no caminho: página 404.
- Parâmetros: `from` e `to` (ids de versão; vazios contam como ausentes; um id
  que não é UUID = 400 `invalid from version id` / `invalid to version id`; um id
  que não pertence a este caminho/projeto = página 404 de "version not found"),
  `limit` (padrão **50**, faixa 1 a **200**) e `offset` (padrão 0; versões a
  pular, da mais nova).
- Versões **expiradas** ficam ocultas; se a versão atual do caminho expirou,
  todo o histórico some (a varredura de esquecimento apaga todas as linhas do
  caminho). Um elo `supersedes` para versão oculta é tratado como ausente.
- Lado novo (`to`): o id pedido ou, sem ele, a versão mais nova do caminho (se
  `offset` > 0, ainda assim a mais nova, e não a primeira da página).
- Lado antigo (`from`): o id pedido (deve existir neste caminho) ou, sem ele, a
  versão que `to` substituiu. Se a substituída está em outro caminho (página
  movida ou ressuscitada) ou oculta, o padrão é **comparar com o vazio** (a
  tela mostra o conteúdo inteiro como acréscimo); só um `from` explícito
  inexistente falha.
- Diff (linhas, com 3 linhas de contexto, algoritmo de menor edição): ver 2.8.1.
- Tela: título `History of <caminho>`; `N version(s), newest first.
  Superseded versions stay in the record; nothing here can change them.`;
  seção do diff (`Changes from <id curto> to <id curto>` ou `Content of <id
  curto> (no earlier version at this path)`; títulos e instantes das duas
  versões); mensagens `The two versions have the same body.`, `No diff shown:
  <motivo>.`, `The bodies differ only in line endings.` e `Diff cut short at the
  line limit.`; tabela `Versions a–b of N` com colunas `From`, `To` (botões de
  rádio para escolher o par), `Version` (id curto, selo `latest` na versão
  atual e `replaced <id curto>` com a versão que ela substituiu), `Written`
  (data relativa), `Author` (usuário ou `—`), `Title`, `Size` (`N chars`, em
  caracteres do corpo) e `vs previous` (atalho `?from=<substituída>&to=<id>`
  quando há substituída); botão `Compare selected` e links `Newer versions` /
  `Older versions` (preservam `limit`, `from` e `to`; a página anterior recua
  `limit` itens, a seguinte avança até o fim da página atual).
- Linhas do diff: cabeçalho de trecho `@@ -a,b +c,d @@` e linhas prefixadas
  com espaço (contexto), `-` (removida) ou `+` (acrescentada).

#### 2.8.1 Limites e regras do diff de linhas

| Limite | Valor | Resultado ao exceder |
|---|---|---|
| Tamanho de cada lado | 512 KiB (524 288 bytes) | `too_large`: `a version is larger than the diff limit` |
| Linhas de cada lado | 10 000 | `too_large`: `a version has more lines than the diff limit` |
| Distância de edição (linhas inseridas+removidas exploradas) | 1 000 | `too_large`: `the versions differ in more lines than the diff limit` |
| Linhas de saída (contexto + mudanças) | 2 000 | resultado `changed` com a marca de truncado e os trechos já emitidos |

- Textos idênticos: `same` (sem trechos). Texto igual linha a linha mas
  diferente no fim de linha ou na quebra final: `changed` **com lista de trechos
  vazia** (a tela diz "differ only in line endings").
- Linhas separadas por `\n` (e `\r\n`: o `\r` não faz parte do texto da linha);
  o conteúdo vazio tem zero linhas.
- Prefixo e sufixo comuns são aparados antes do cálculo. O roteiro de edição
  deve ser o **mínimo** (algoritmo guloso de Myers, para a frente, guardando
  uma fronteira por passo de edição para reconstruir o caminho). Para que a
  saída seja idêntica entre implementações, o desempate é: em cada passo `d`,
  para cada diagonal `k` de `−d` a `d` de 2 em 2, parte-se da diagonal `k+1`
  (**inserção**) quando `k = −d` ou quando `k ≠ d` e o alcance em `k−1` é
  **estritamente menor** que o alcance em `k+1`; caso contrário parte-se da
  diagonal `k−1` (**remoção**, avançando uma linha do texto antigo). Depois
  segue-se a diagonal enquanto as linhas forem iguais.
- Trechos: cada mudança recebe 3 linhas de contexto antes e depois; mudanças
  separadas por até **6 linhas inalteradas** (2 × contexto) cujas janelas de
  contexto se tocam viram um único trecho. Cabeçalho
  `@@ -início,tamanho +início,tamanho @@`, com início de 1 em diante; um lado
  sem linhas no trecho usa o número da linha anterior (0 no topo).
- Corte de saída: ao atingir 2 000 linhas emitidas, o trecho em curso é
  fechado com as linhas já emitidas, nenhum outro é gerado e o resultado é
  marcado como truncado.

### 2.9 Linha do tempo de uma sessão (`/w/{ws}/{proj}/s/{id}`)

Lista as observações de uma sessão, em ordem de captura, com paginação e um
"scrubber". Usa os mesmos leitores e limites da API de sessões (4.12 e 4.13 de
[frontend-api.md](frontend-api.md)).

- Escopo: leitura sem criar; recusa = 403 em texto simples; inexistente = 404.
  Id de sessão que não é UUID: 400 em texto simples `invalid session id`.
  Sessão sem linha nem observação neste projeto, ou escondida pelo filtro de
  dono: página 404 (um id conhecido não sonda outro projeto ou operador).
- Parâmetros: `limit` (padrão **50**, faixa 1 a 200), `offset` (a partir de 0;
  acima do total vira o total, isto é, página vazia) e `at` (posição 1-based
  para saltar; é ajustada a `1..total` e **substitui** `offset` pelo início da
  página que a contém: `(at − 1) ÷ limit × limit`, divisão inteira). Valores não
  numéricos da query rejeitam a requisição (400 do servidor HTTP).
- Cabeçalho: `Session timeline`; tipo do agente, `started <instante>` e `, ended
  <instante>` ou `, still open`. Nota fixa: `Captured observations, in order.
  They were sanitized when captured and are shown as plain text.` e, se a sessão
  deixou observações em outro projeto, `This session also left N observation(s)
  in another project, not shown here.`
- Corpo de cada observação cortado em **4 000 caracteres** com a mesma marca da
  API (`[body truncated; N chars omitted]` após quebra de linha); sempre texto
  puro.
- Scrubber: `« First`, `‹ Prev`, `a–b of N`, `Next ›`, `Last »` (os inativos
  aparecem como texto sem link; `Last` leva ao `offset` do último bloco:
  `(total − 1) ÷ limit × limit`), e o formulário `Jump to` (`at`, `limit`) com
  botão `Go`. Cada item é `#posição` (posição global 1-based, âncora
  `obs-<posição>`, link para `?at=<posição>&limit=…`), tipo, título e data
  relativa; sem itens: `No observations on this page.`
- Reforço opcional por script: um controle deslizante ligado ao campo de salto
  e atalhos `j`/seta direita (próxima) e `k`/seta esquerda (anterior), que
  cruzam a borda da página seguindo os links prev/next. A página inteira
  funciona sem script.

### 2.10 Painel de saúde do servidor (`/health`)

Somente raiz (mesma regra e mesmas respostas da página de pendências: 401 sem
autenticação, 403 com a página "Root access required" para quem não é raiz). Só
lê; não há botão: cada correção é um comando a executar. Resposta com
`Cache-Control: private, no-store`.

- Título `Server health`; texto `Counters, queues and model state for this server
  process. Read-only: each fix is a command to run, not a button.` Se o
  processo não ligou os contadores na interface, aviso: `This server did not
  wire its process counters into the web UI; hook, spool and provider figures
  below are empty.`
- Seção **Checks**: lista as verificações que **falham** (vazia = `Every
  server-side check passes.`), das mais graves para as menos (todas `error`
  antes das `warn`; dentro de cada gravidade, na ordem da tabela). Cada item:
  gravidade (`error` ou `warn`), detalhe e, quando existe, `Fix: <comando>`.
  Nada na página cita corpo de página, prompt, observação, nome de escopo, texto
  de erro de provedor ou credencial: só contagens, instantes, rótulos
  configurados e comandos fixos.

| id | Gravidade | Condição | Detalhe | Comando |
|---|---|---|---|---|
| `hook_shed` | warn | respostas 429 de hooks desde o início > 0 (saturação + limite de taxa) | `N hook event(s) were answered 429 since the server started (a saturated, b rate-limited); clients spool and retry them.` | — |
| `hook_failed` | error | entregas de hook que falharam ao gravar > 0 | `N hook delivery(ies) failed to store since the server started.` | — |
| `write_queue` | warn | fila do escritor com `enfileirados × 4 ≥ capacidade × 3` (75 %) e capacidade > 0 | `The write queue holds Q of C commands.` | — |
| `spool_evicting` | error | spool de hooks local com `pendentes ≥ capacidade` (capacidade > 0) | `The hook spool is full (N events): newly captured events are dropped until space is available.` | `ai-memory recover` |
| `spool_pending` | warn | (senão) pendentes > 0 e (tentativas falhas > 0 **ou** idade do mais antigo desconhecida **ou** ≥ 15 min) | `N hook event(s) wait in the local spool and delivery looks stuck (oldest M min, R failed attempt(s)).` (`age unknown` quando desconhecida) | `ai-memory recover` |
| `journal_unreadable` | error | diário de recuperação existe mas não pôde ser lido | `The recovery journal could not be read.` | `ai-memory recover` |
| `journal_pending` | warn | (senão) execuções pendentes no diário > 0 | `N run(s) wait in the recovery journal (d degraded, f finish-failed).` | `ai-memory recover` |
| `llm_provider` | error | última chamada ao provedor de LLM falhou | `The LLM provider's last call failed` + ` (HTTP S)` se houver status + `.` | dica de nova tentativa do provedor, se houver |
| `embedding_provider` | error | última chamada ao provedor de embeddings falhou | `The embedding provider's last call failed` + ` (HTTP S)` + `; searches run without the vector stream until it answers.` | — |
| `stale_vectors` | warn | há embedder configurado e vetores guardados sob outra tripla (provedor, modelo, dimensão) | `N stored vector(s) were made under another provider, model or dimension and are ignored.` | `ai-memory embed --force` |
| `missing_vectors` | warn | há embedder e (páginas atuais sem vetor > 0 ou falhas de embed não resolvidas > 0) | `M latest page(s) have no vector (F after a failed or skipped embed).` | `ai-memory embed` |
| `storage_reclaim` | warn | espaço recuperável ≥ 20 % do banco **e** ≥ 64 MiB (67 108 864 bytes) | `R of the D-byte database is reclaimable; compacting blocks writes while it runs.` | `ai-memory compact --confirm` |
| `consolidation_failed` | warn | jobs de consolidação de sessão que desistiram após as tentativas > 0 | `N session consolidation job(s) gave up after their retries.` | — |

  Verificações de vetor só existem com embedder configurado (fora do MVP sem
  embeddings). Os comandos são as mesmas constantes que `ai-memory status`
  imprime.
- Seções de dados (`dt/dd`): **Hooks (since the server started)** — `accepted`;
  `shed (429)` = `T (s saturated, r rate-limited)`; `outcomes` = `a stored, b
  replayed, c resumed, d ignored end, e failed`; `dropped` = `a capture policy,
  b unauthorized, c collision, d invalid`; `last write` (instante ou `never`).
  **Writer, spool and recovery** — `write queue` (`Q/C` ou `unknown`), `hook
  spool` (`N pending of C (oldest <idade>, R retries)` ou `not visible to this
  server`), `recovery journal` (`N pending (d degraded, f finish-failed)`,
  `unreadable` ou `not visible to this server`). **Consolidation** — `last
  completed` (ou `never`), `queue` (`p pending, r running, c completed, f
  failed`), `oldest pending since`. **Embedder and models** — `embedder`
  (`provedor/modelo (dim N)` ou `none (FTS5 + entity + graph only)`), `embedding
  provider` e `llm provider` (`status · provedor/modelo (dim N) · last ok <t> ·
  last error <t> (HTTP S)`; a mensagem de erro do provedor nunca é exibida),
  `reranker` (`on`/`off`), `vectors` (`N rows; M latest pages missing; U
  unembeddable`) e uma linha `stored triple` (`provedor/modelo dim D: N rows`)
  por tripla guardada. **Index and storage** — `fts` (`pages a/b; observations
  c/d`, índice/total) e `database` (`N bytes, R reclaimable`).
- Idade (`age`): em segundos até 59 s `Ns`; até 59 min `Nm`; até 23 h `Nh`;
  depois `Nd`; desconhecida = `n/a`.
- Falha ao ler o estado de índice ou a fila de consolidação: 500 sem corpo; a
  leitura de tamanho do banco falhando usa zeros.

---

### 2.11 Triagem de propostas pendentes (`/pending`)

Página só para a conta **raiz** (administrador). A página só lê; os botões chamam
`POST /admin/pending-writes/{id}/approve|reject` (com cookie de sessão e cabeçalho `X-CSRF-Token`
lido do cookie `ai_memory_csrf`). Título "Pending writes".

- **Consulta:** `project` (chave `workspace/projeto`, cada nome com escape percentual, de modo que `/`
  dentro de um nome não confunda dois projetos; vazio = todos; chave que não corresponde a um projeto
  com pendência é ignorada) e `sort` (valor desconhecido = padrão). Ordenações: `project` ("Project,
  then staged date", padrão), `confidence-asc` ("Confidence, low first"), `confidence-desc`
  ("Confidence, high first") e `staged-desc` ("Staged, newest first"). O seletor "Project" lista
  `All projects (<total>)` e `<ws>/<proj> (<n>)` (nomes com `/` aparecem entre aspas).
- **Limite:** até **500** propostas por carga; excedendo, mostra "only the oldest 500 of this
  selection are listed; pick a project, or use `ai-memory pending-writes list`". Contagem exibida:
  `<n> shown · <total> pending in <k> project(s)`.
- **Cada proposta** (recolhível): título, selo da operação (destaque se reescreve página existente),
  selo `rule` se o alvo está em `_rules/`, kind, escopo, caminho, `confidence <N>%` (arredondada),
  `staged <tempo relativo>`, aviso para `_rules/` ("Every agent session loads those pages as
  instructions. Read the full body before you approve it."), aviso de substituição com link para a
  página atual, justificativa, "Proposed body (<modo>)" como texto puro, `id`, e botões `Approve`,
  `Reject` e campo de motivo (máx. 500 caracteres, opcional).
- **Sessão só de leitura** (login legado por token raiz): em vez dos botões, mostra os comandos
  `ai-memory pending-writes approve|reject <id> --workspace <ws> --project <proj>` para **POSIX**
  (argumento sem aspas só com `[A-Za-z0-9-_./:@]`; senão aspas simples com `'` escapado como `'\''`)
  e para **PowerShell** (sempre entre aspas simples, apóstrofo dobrado).
- **Resultado do clique:** sucesso remove o item e mostra "Approved <caminho>." / "Rejected
  <caminho>."; 409 mostra "The page at <caminho> changed after the proposal was staged. The proposal
  is now marked as a conflict. Reload the page."; outros erros "Approve|Reject failed for <caminho>
  (<status>): <erro>"; falha de rede "Network error. Try again.".

## 3. API `/api/v1`

### 3.1 Acesso e CORS

- Todas as rotas são GET, exceto a busca, que aceita também POST.
- A API fica atrás da mesma autenticação das rotas protegidas. O
  redirecionamento de navegação da seção 2.6 não se aplica a `/api/v1`, então
  os erros de autenticação chegam ao cliente sem alteração.
- CORS é aplicado **somente** a `/api/v1`, e só quando há origens configuradas
  (opção de origem permitida para CORS). As origens são validadas antes do
  servidor subir. Métodos permitidos: GET, POST e OPTIONS. Cabeçalhos
  permitidos: Authorization, Content-Type e X-CSRF-Token. Credenciais são
  permitidas e o cache do pré-voo dura 600 segundos.
- A API pode ser habilitada sem a interface web. Nesse caso só `/api/v1` é
  montado.

### 3.2 Rotas

Todas com prefixo `/api/v1`.

| Rota | Retorno |
|---|---|
| `/workspaces` | Lista de workspaces com estatísticas. |
| `/projects` | Lista de projetos com estatísticas. Filtro opcional `workspace`; valor vazio ou só com espaços equivale a todos. |
| `/workspaces/{ws}/projects/{p}/pages` | Lista de páginas do projeto. |
| `/workspaces/{ws}/projects/{p}/pages/{caminho}` | Página completa: metadados, corpo em markdown, frontmatter, links de saída, backlinks e autor. |
| `/search` (GET e POST) | Resultados da busca, com workspace, projeto, caminho, título, trecho, tipo e rank. |
| `/workspaces/{ws}/projects/{p}/history/{caminho}` | Versões guardadas de uma página e diff entre duas (mesmos dados do 2.8; contrato em [frontend-api.md](frontend-api.md) 4.14). |
| `…/recent` | Snapshot das páginas mais recentes ou modo incremental por cursor (seção 3.3). |
| `…/briefing` | Briefing do projeto. |
| `/workspaces/{ws}/overview` | Visão geral do workspace. |
| `…/projects/{p}/overview` | Visão geral do projeto. |
| `…/handoffs` | Handoffs do projeto. |
| `…/sessions` | Sessões do projeto. |
| `…/sessions/{id}/observations` | Observações de uma sessão. |
| `/graph` | Arestas entre projetos diferentes (seção 3.3). |

### 3.3 Parâmetros e limites

| Endpoint | Parâmetros e regras |
|---|---|
| Busca (GET) | `q` ou `query` (termo). `workspace` e `project`, que devem vir juntos. `scope`, repetível, no formato `workspace/projeto`. `limit`, padrão 10, faixa 1 a 100. |
| Busca (POST) | Corpo JSON com `q` (aceita o alias `query`), `workspace`, `project`, `scopes` (lista de objetos com `workspace` e `project`) e `limit` (mesma regra). |
| Busca, regras gerais | Ordem de avaliação (importa para qual erro sai): (1) GET: decodificação da query (`+` vira espaço; `%XX` inválido ou truncado = 400 `invalid percent-encoding in query`; bytes que não formam UTF-8 = 400 `query must be valid UTF-8`), `limit` não inteiro não negativo = 400 `limit must be an integer`, e cada item de `scope` fora de `workspace/projeto` (sem barra, parte vazia ou barra extra no projeto) = 400 `scope must use the workspace/project format`; chaves desconhecidas são ignoradas; chave repetida vale a última (exceto `scope`, que acumula); (2) termo aparado vazio: **200 com `[]`**, sem cabeçalho de cache e sem validar mais nada; (3) `limit` ajustado a 1..100; (4) `scopes` junto com `workspace` ou `project` não vazios (após aparar) = 400 `scopes cannot be combined with workspace/project`; (5) mais de 25 escopos = 400 `at most 25 scopes are allowed`; (6) resolução dos escopos: workspace ou projeto inexistente = 404, sem leitura = 403, nome de projeto ambíguo = 400; (7) sem `scopes`: `workspace` sem `project` (ou o contrário) = 400 `workspace and project must be provided together`; os dois = busca de um projeto (404/403 como acima, com a mensagem `project '<p>' not found`); nenhum = busca global. |
| Busca, com vários escopos | Resultados unidos sem repetição (ficam os de menor rank por página), ordenados por rank crescente e cortados no limite. Resultados sem metadados correspondentes são descartados. |
| `recent` sem `updated_since` nem `cursor` | Snapshot: as páginas mais recentes por data de atualização, até `limit` (padrão 10, faixa 1 a 100). |
| `recent` com `updated_since` ou `cursor` | Modo incremental: páginas atuais e não expiradas com atualização estritamente posterior ao instante informado. Resposta `{pages, next_cursor}`. Ordem (resolvida): `updated_at` crescente e, em empate, `path` crescente; o cursor retoma estritamente depois do par (`updated_at`, `path`) da última página devolvida. Busca-se `limit + 1` linhas para saber se há mais; `next_cursor` é `null` quando não há. |
| `recent`, instante | RFC 3339, com 20 a 64 bytes, fração de até 9 dígitos e sufixo `Z` ou deslocamento `±hh:mm`. A precisão efetiva é de microssegundos, com truncamento. Valor inválido: 400. |
| `recent`, cursor | Opaco: base64 URL-safe sem preenchimento, com JSON versão 1. Limites: 8192 caracteres de entrada, 6144 bytes decodificados e 2048 caracteres no caminho. Deve pertencer ao mesmo escopo, senão 400. Se `updated_since` vier junto, deve ser igual ao instante do cursor, senão 400. O instante do cursor deve ser posterior ao instante `updated_since`, senão 400. |
| `briefing` e overviews | `limit`, padrão 10, faixa 1 a 100. |
| `handoffs` | `state`: `open`, `accepted` ou `expired`. Vazio ou ausente lista todos. Valor desconhecido: 400. `limit`, padrão 50, faixa 1 a 200. `all_owners`: só a raiz pode usar; outros chamadores recebem 403. Sem autorização para o corpo, o resumo, as perguntas em aberto e os próximos passos são omitidos e o campo `redacted` fica verdadeiro. |
| `sessions` | `limit`, padrão 20, faixa 1 a 100. `offset`, padrão 0. `include_open`, padrão falso: inclui sessões que ainda não foram encerradas. |
| `sessions/{id}/observations` | `limit`, padrão 50, faixa 1 a 200. `offset`. `order`: `asc` (padrão) ou `desc`, sem diferenciar maiúsculas; outro valor é 400. `kinds`: lista separada por vírgula; tipo desconhecido é 400. `q`: busca de texto restrita à sessão. `body_max_chars`, padrão 4000, faixa 200 a 16384. Corpos mais longos são cortados e recebem um marcador com a quantidade de caracteres omitidos. ID de sessão inválido: 400. Sessão inexistente ou não visível: 404. |
| `graph` | Arestas em que as duas extremidades estão em projetos diferentes e são legíveis pelo visualizador. Não há filtro por projeto. |

Campos da página completa: `id` (da versão atual; muda a cada nova versão, e
por isso também o ETag), workspace, projeto, caminho, título, tipo, tier,
fixada, substituída (caminho da versão anterior ou `null`), datas de criação
(da versão mais antiga da cadeia) e atualização, autor (omitido quando não há),
frontmatter, corpo em markdown, links de saída e backlinks (listas completas,
sem o limite de 10 do HTML, ordenadas por workspace, projeto e caminho, sem
repetição, só páginas atuais, não expiradas e legíveis pelo visualizador).
A página completa **não** esconde páginas expiradas (as listas e a busca sim).
Corpo e frontmatter são lidos do arquivo no momento da requisição; arquivo
ausente = 404 `page file not found`.

### 3.4 Cache e ETag

| Política de cabeçalho | Respostas |
|---|---|
| `private, max-age=30` | Workspaces, projetos, grafo, busca com termo não vazio (GET e POST), `recent` em modo snapshot. |
| `private, max-age=60` | Lista de páginas do projeto. |
| `private, max-age=300` com ETag | Página completa. A resposta 304 também carrega esses cabeçalhos. |
| `private, no-store` | `recent` incremental, briefing, overviews, handoffs, sessões, observações e histórico de página. A página HTML `/health` também. |
| Sem cabeçalho de cache | Todos os erros e a busca com termo vazio. |

- Respostas de erro nunca recebem cabeçalho de cache. Respostas de sucesso
  recebem o cabeçalho conforme a tabela, e o 304 também.
- O ETag da página completa é o SHA-256 do JSON inteiro da resposta, incluindo
  links e backlinks. Qualquer mudança nesses campos altera o ETag, mesmo quando
  o corpo não muda.
- Se `If-None-Match` for exatamente igual ao ETag emitido, a resposta é 304 sem
  corpo, com o ETag e o cabeçalho de cache.

### 3.5 Erros

Erros são sempre JSON no formato `{"error": "..."}`.

| Status | Quando |
|---|---|
| 400 | Parâmetro inválido, combinação de parâmetros inválida, cursor inválido, instante inválido. |
| 403 | Escopo proibido para o visualizador, ou `all_owners` sem privilégio de raiz. |
| 404 | Workspace, projeto, página, sessão ou arquivo não encontrado. |
| 500 | Erro interno. A mensagem é sempre genérica (`internal server error`) e o detalhe fica só no registro do servidor. |

---

## 4. Markdown seguro

O conteúdo do wiki é **não confiável**: pode vir de prompts, hooks ou saída de
modelos de linguagem. A renderização segue estas regras.

### 4.1 Opções e HTML bruto

- Recursos ativados: tabelas, notas de rodapé, tachado, listas de tarefas e
  pontuação inteligente.
- HTML bruto, em bloco ou inline, é mostrado como texto e nunca é interpretado.
- A reescrita de wikilinks (seção 4.3) acontece antes da conversão.

### 4.2 URLs e links

- Links aceitos sem alteração: `http://`, `https://`, `mailto:` (esquemas
  comparados sem diferença de caixa e depois de remover espaços à esquerda do
  destino), caminhos começando com `/` (inclusive caminhos de rede `//host`),
  âncoras `#...` e destinos sem nenhum `:`. Qualquer outro destino (qualquer
  outro esquema, como `javascript:`, `data:`, `vbscript:`, `file:`, `tel:`)
  vira `#`. A reescrita de links relativos do wiki (abaixo) acontece **antes**
  desta lista de permissão.
- Links de rede (`//host`) **são mantidos**. Só imagens bloqueiam esse caso.
- Links relativos do wiki (que não começam com `#`, `/` ou `\` e não têm `:`;
  destino aparado) são reescritos para a URL da página dentro do projeto atual,
  como se fossem relativos à raiz do projeto. Isso vale para arquivos `.md` e
  para diretórios, como `_lint/`. A âncora ou query (a partir do primeiro `#`
  ou `?`) é preservada.
  - O caminho passa por decodificação de `%XX` (um byte de controle decodificado
    ou bytes que não formam UTF-8 tornam o destino `#`). Depois da decodificação:
    se vazio, se contém `..` em qualquer posição (inclusive dentro de um nome,
    como `a..b`), se começa com `/` ou contém `\` ou `:`, o link **não é
    reescrito** (permanece como estava, a menos que a decodificação tenha
    mudado o texto, caso em que vira `#`).
  - Um `./` inicial é removido (repetidamente) e as barras finais são aparadas;
    um resultado vazio, `.` ou começando com `/` deixa o link como estava.
  - Links que já estão no formato `w/…/p/…` (começam com `w/` e contêm `/p/`)
    não são reescritos de novo.
- Imagens: o destino só é aceito se for **relativo e confinado**; senão vira
  `#`. Regras, nesta ordem, sobre o destino sem os caracteres de controle e
  espaços das pontas: vazio = `#`; começa com `#` = aceito; começa com `/` ou
  contém `\` ou `:` = rejeitado (isso bloqueia caminhos de rede `//host`,
  caminhos absolutos e qualquer esquema); no caminho (antes de `?` ou `#`),
  decodificado uma vez em `%XX`: começar com `/`, conter `\` ou ter qualquer
  segmento `..` = rejeitado (o navegador resolve `%2e%2e` como `..`); o
  primeiro segmento não vazio e diferente de `.` não pode ser (sem diferenciar
  caixa) `admin`, `auth`, `handoff`, `hook`, `identity`, `mcp` nem `workstream`
  (rotas do servidor que a imagem alcançaria, com a sessão do leitor, quando a
  interface está montada na raiz do prefixo). Isso evita requisições automáticas
  a origens externas e a rotas internas ao abrir a página.

### 4.3 Wikilinks

Formas reconhecidas:

- `[[alvo]]`: página no projeto atual.
- `[[alvo|rótulo]]`: o rótulo é o texto exibido. O divisor é o primeiro `|`, e
  o rótulo pode conter outros `|`. Alvo e rótulo são aparados. Sem rótulo, ou
  com rótulo vazio, o texto exibido é o **alvo inteiro como escrito** (aparado,
  com qualificador de escopo e `#âncora`/`?query`, ex.: `[[infra:runbooks/db#x]]`
  mostra `infra:runbooks/db#x`).
- `[[projeto:caminho]]`: página de outro projeto do mesmo workspace.
- `[[workspace/projeto:caminho]]`: página de outro workspace.
- `[[_global:caminho]]`: projeto reservado `_global` no workspace padrão,
  independentemente da página atual.

Regras:

- O qualificador de escopo aceita só letras, dígitos, `-`, `_`, `/` e `.`.
  Qualquer outro caractere faz a parte antes dos dois-pontos ser tratada como
  caminho.
- Ficam como texto literal: alvo vazio, alvo que começa com `#`, alvo com
  esquema externo (contém `://`, ou começa — sem diferenciar caixa — com
  `mailto:`, `data:`, `javascript:`, `vbscript:`, `tel:` ou `file:`), e `[[`
  sem `]]` correspondente **na mesma linha** (o texto é processado linha a
  linha: um `[[` aberto nunca fecha com um `]]` de linha posterior; o fecho é o
  primeiro `]]` depois do `[[`).
- Caminho (depois de separar o qualificador e o sufixo `#`/`?`, e de aparar):
  vazio, último segmento vazio (alvo terminado em `/`) ou último segmento
  igual a `.md` = literal. Se o último segmento tem ponto, precisa terminar em
  `.md`, senão fica literal. Sem ponto, `.md` é acrescentado. Depois a
  validação de caminho de página rejeita travessia, caminho absoluto, barra
  invertida, prefixo de unidade e segmentos vazios. Se a validação falhar, o
  texto fica literal.
- O qualificador é separado no **primeiro** `:` do alvo. Com `ws/proj:` e
  `proj` vazio, o alvo todo é tratado como caminho (literal por conter `:`).
  `[[_global:x]]` aponta sempre para o projeto `_global` do workspace `default`.
- Sufixos `#âncora` e `?query` são preservados. Caracteres que quebrariam o
  destino do link (espaço, parênteses, colchetes, aspas e barra invertida) são
  percent-encoded.
- Dentro de código (inline, cercado ou indentado) e de blocos HTML, `[[...]]`
  nunca é reescrito. A detecção usa o mesmo analisador da renderização, então
  o que é código para a renderização também é código para os wikilinks.
- No rótulo, colchetes e parênteses são escapados para não fechar o link antes
  do fim.

### 4.4 Identificadores de cabeçalho

- O texto-base do cabeçalho é a concatenação do texto puro e do código inline
  dos seus filhos (marcação como ênfase ou links contribui só com o texto).
  O identificador é esse texto em minúsculas, percorrido caractere a caractere
  (Unicode): letra ou dígito (de qualquer alfabeto) é mantido em minúscula; `_`
  é mantido; hífen `-`, travessão `—`, meia-risca `–` e qualquer espaço em
  branco viram **um único `-`**, mas só depois de já haver algum caractere
  (separadores no início são descartados) e nunca dois seguidos; toda outra
  pontuação ou símbolo é removida (não gera separador). Hífens finais são
  aparados. Texto vazio (ou só pontuação) vira `section`.
- Identificadores repetidos recebem sufixo numérico: `-1`, `-2` e assim por
  diante. O contador de cada base parte de 1 e pula candidatos já usados na
  página (inclusive ids explícitos), de modo que o resultado é sempre único.
  Ex.: três cabeçalhos "Notas" geram `notas`, `notas-1`, `notas-2`.
- Um identificador explícito no cabeçalho também passa pela deduplicação.
- Âncoras de wikilink coincidem com esses identificadores.

### 4.5 Trechos da busca

O trecho vindo do índice de busca é escapado por completo (`&` → `&amp;`, `<` →
`&lt;`, `>` → `&gt;`, `"` → `&quot;`, `'` → `&#39;`), e depois apenas as
marcações de realce `<mark>` e `</mark>` são restauradas (substituição de
`&lt;mark&gt;` e `&lt;/mark&gt;`; um `<mark>` digitado no corpo da página
também sobrevive, o que é inofensivo por não carregar atributos).

### 4.6 Outras opções de renderização

- A "pontuação inteligente" converte aspas retas em tipográficas, `--` em
  meia-risca, `---` em travessão e `...` em reticências no HTML; isso não afeta
  o texto de títulos de cabeçalho usado nos identificadores (4.4), que usa o
  texto convertido da forma como o analisador o entrega.
- Tabelas, notas de rodapé, tachado `~~x~~` e listas de tarefas `- [ ]`/`- [x]`
  seguem a especificação GFM. O frontmatter já chega separado do corpo; o corpo
  renderizado nunca o contém.
- O HTML do corpo é inserido sem novo escape (`safe`) no modelo da página, por
  já ter passado pelas regras acima; qualquer outro campo (título, caminho,
  autor, rótulos) é escapado pelo modelo.

---

## 5. Montagem e prefixo

### 5.1 Normalização do prefixo

`normalize_prefix(raw)` retorna `""` (raiz) ou `/<a>/<b>`:

- Uma barra inicial, sem barra final. Segmentos vazios (`//`) são colapsados.
- Cada segmento só pode ter caracteres não reservados da RFC 3986: letras,
  dígitos, `-`, `.`, `_` e `~`.
- Os segmentos `.` e `..` são rejeitados.
- Qualquer violação faz o resultado virar `""`. Assim, uma variável de ambiente
  malformada nunca injeta marcação, nem um `//` que vire protocolo relativo.

O mesmo tratamento vale para o slug do navegador. Quando o slug é inseguro, o
servidor registra um aviso e monta o navegador na raiz do prefixo de base.

### 5.2 URLs base

- `web_base_href(base_path, slug)` retorna o prefixo de base concatenado ao
  slug normalizado. O resultado termina sempre em `/` e nunca começa com `//`.
  Quando o resultado é vazio, retorna `/`.
- As URLs geradas pelo navegador são **relativas** (`w/{ws}/{proj}/p/{caminho}`,
  `search`, `static/...`) e são resolvidas contra a base injetada. O mesmo HTML
  funciona em `/web/...` ou atrás de um proxy reverso em `/wiki/web/...`.
- Segmentos de URL são percent-encoded e só mantêm letras, dígitos, `-`, `.`,
  `_` e `~`.

### 5.3 Injeção no HTML

- Só respostas `text/html` são alteradas. Estáticos e redirecionamentos passam
  sem mudança.
- Nas respostas do navegador embutido, o corpo é lido com limite de 10 MiB.
  Acima disso, a resposta vira 500.
- Corpo que não é UTF-8 passa sem injeção.
- Injeta-se, logo após a primeira tag que começa com `<head` (até o primeiro
  `>` seguinte), fora de comentários HTML:
  - `<base href="...">`, com a base de URLs;
  - `<meta name="ai-memory-base-path" content="...">`, com o prefixo de base,
    para que a interface monte `{base}/api/v1`;
  - no navegador embutido, `<meta name="ai-memory-login-path" content="...">`.
  As três inserções são feitas **uma após a outra, cada uma imediatamente depois
  de `<head…>`**, na ordem acima; portanto a ordem final no documento do
  navegador embutido é: `login-path`, `base-path`, `base` (a base fica por
  último, antes do conteúdo original do `<head>`). Na interface customizada só
  há duas (`base-path`, depois `base`). Cada injeção também remove o
  `Content-Length` (recalculado pelo servidor).
- A varredura percorre o texto caractere a caractere (não byte a byte): um
  documento com BOM UTF-8 ou texto não ASCII antes do `<head>` é suportado e o
  BOM permanece no início.
- A detecção da tag `<head` é por prefixo. Por isso uma tag como `<header`
  antes de `<head` também casa. Isso não ocorre nos templates embutidos.
- Comentário HTML não terminado, ou ausência de `<head`, faz a injeção ser
  feita no início do documento.
- Valores injetados são escapados para `&`, `"`, `<` e `>`.

### 5.4 Divisão das rotas

`split_web_routers` produz duas partes, que o servidor protege de forma diferente:

- **Pública:** a interface customizada (seção 5.5), ou, no navegador embutido,
  as páginas de login, troca de senha e estáticos.
- **Protegida:** `/api/v1` e, quando é a interface escolhida, o navegador
  embutido. O servidor aplica autenticação e o redirecionamento da seção 2.6.

Casos:

| Configuração | Montado |
|---|---|
| Nada habilitado | Nenhuma rota. |
| Só a API habilitada | `/api/v1` protegido. |
| Interface web, sem interface customizada | Públicas: login, troca de senha, estáticos. Protegidas: `/api/v1` e navegador embutido. |
| Interface web com interface customizada | Pública: a interface customizada. Protegida: `/api/v1`. Sem redirecionamento de login HTML, porque o login fica na própria interface. |

O navegador embutido redireciona `{slug}/` para a forma canônica com status
**308 (Permanent Redirect)**, não 301. O destino é a base de URLs sem a barra
final (prefixo de base + slug; `/` se ambos vazios), com a query string
preservada (`?q=x` continua); o `Location` carrega o prefixo externo completo,
porque o aninhamento no prefixo de base não reescreve cabeçalhos. A interface
customizada responde 200 com o índice injetado em `{slug}/` e não redireciona
(um SPA fica melhor sem um salto a cada atualização da raiz). Com slug vazio, o
navegador fica na raiz e não há redirecionamento.

Toda superfície web (pública e protegida, inclusive `/api/v1`) recebe os
cabeçalhos anti-moldura da seção 2.0; o `favicon.ico` da raiz do host, não.
As rotas públicas do navegador embutido (login, troca de senha, estáticos) passam
pela expiração do cookie de autenticação legado (ver [users.md](users.md)).

### 5.5 Interface customizada

- Escolhida por uma opção de diretório de interface. Só tem efeito quando a
  interface web está habilitada.
- O arquivo de índice é lido na montagem. Se não puder ser lido, a montagem
  falha com erro que cita o diretório.
- O índice recebe as injeções da seção 5.3 e é servido para `/`, `/index.html`
  e `{slug}/`.
- Demais caminhos são arquivos estáticos do diretório. Qualquer caminho que não
  existe como arquivo recebe o índice injetado com 200, sem 404. Isso serve as
  rotas do lado do cliente.

---

## 6. Estilo e build

- A folha de estilo é compilada e embutida no executável. Não há leitura de
  arquivo em tempo de execução.
- Regenerar a folha exige um modo de build explícito. Esse modo baixa a
  ferramenta de geração de CSS, fixada em versão e com checksum por
  plataforma. Se o checksum não bater, a build falha.
- A verificação contínua recompila a folha nesse modo e falha se o arquivo
  versionado estiver desatualizado.
- Um build comum não precisa de rede: usa o arquivo versionado.

---

## 7. Notas para reimplementação

1. Mantenha esta camada **somente leitura e sem regras de negócio**. Ela só
   consulta a camada de leitura e a camada de wiki. Toda escrita deve passar
   pelo caminho de mutação do núcleo.
2. Trate o conteúdo como hostil: escape HTML bruto, neutralize esquemas
   perigosos, aceite imagens só relativas e nunca renderize script. Teste com
   payloads de XSS.
3. Gere URLs relativas e uma base de URLs injetada. Assim o mesmo HTML funciona
   atrás de proxy reverso em subcaminho, sem reescrever templates.
4. Cacheie só respostas de sucesso, use ETag na página completa e marque como
   `no-store` tudo o que depende de identidade ou de estado operacional.
5. Falhe fechado em escopo: escopo inexistente vira 404, escopo proibido vira
   403, e a leitura nunca cria workspace ou projeto.
6. Limite todo parâmetro numérico com faixa explícita. Trate cursores como
   opacos e valide-os inteiros antes de usar. Limite o tamanho de corpos
   injetados.

---

## 8. Pendências de verificação

Todas as pendências anteriores foram resolvidas lendo o código de referência:

- Caminho de página inválido na rota HTML de página: responde 404 (seção 2.3).
- Ordem das páginas no modo incremental de `recent`: `updated_at` crescente,
  depois `path` crescente (seção 3.3).
- Descarte do diálogo de migração: armazenamento local do navegador, chave
  `ai-memory-okf-dialog-<data da migração>` (seção 2.1).

Pontos que o código de referência não permite confirmar a partir do material
disponível: o texto exato de erro que o servidor HTTP devolve para parâmetros
de query ou corpo JSON que não convertem ao tipo esperado (corpo em texto
simples, status 400 para query e para JSON sintaticamente inválido; 415 para
JSON sem o `Content-Type` correto; 422 para JSON válido de formato errado) —
é comportamento da camada HTTP, fora do contrato.
