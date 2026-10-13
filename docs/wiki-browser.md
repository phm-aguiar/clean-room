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
| `GET /w/{workspace}/{projeto}/p/{caminho}` | Página renderizada e metadados. |
| `GET /search?q=` | Lista de resultados da busca. |
| `GET /pending` | Triagem de propostas pendentes (somente raiz). |
| `GET /login` e `GET /change-password` | Formulários públicos de autenticação. |
| `GET /static/...` | Folha de estilo e logo embutidos, públicos. |
| `GET /favicon.ico` | Ícone, montado na raiz do host e não sob o prefixo nem sob `/web`, pois o navegador busca esse caminho na origem. Só existe quando a interface web está habilitada. |
| `GET {slug}/` | Redirecionamento permanente (301) para `{slug}`, preservando a query string. |

A interface web é desligada por padrão. O prefixo do navegador (`/web`) vem do
parâmetro de slug e pode ser trocado por variável de ambiente.

### 2.1 Página inicial

- Um card por projeto, com workspace, nome do projeto, quantidade de páginas e
  data relativa da última atualização.
- Quando a autorização por projeto está ativa e o visualizador não é raiz, só
  aparecem os projetos que ele pode ler. Sem visualizador identificado (instalação
  sem usuários de banco, ou raiz), aparecem todos.
- Se existe registro de backup de migração, a página mostra um diálogo
  informativo com caminho, tamanho e data do arquivo. O descarte do diálogo
  acontece no navegador (mecanismo a confirmar).

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

### 2.3 Página

- Metadados exibidos: tipo, tier, indicação de fixada, datas de criação e
  atualização (relativas), link para a página substituída (quando houver) e
  autor (nome de usuário, nome e e-mail como link `mailto`).
- Antes de renderizar, remove-se o primeiro H1 do corpo quando o texto dele é
  igual ao título da página. Vale para o formato com `#` e para o formato
  sublinhado. Qualquer outro H1 permanece.
- Links de saída e backlinks **não** aparecem na página HTML. Estão disponíveis
  na API (seção 4.2).
- Se o caminho não corresponde a uma página, mas existem páginas sob
  `caminho/`, a resposta é uma listagem de namespace (título, tipo, data
  relativa e link de cada página). Sem páginas sob o caminho, a resposta é 404.
- Se o visualizador não pode ler o projeto, a resposta é 403 em texto simples.
  Projeto inexistente recebe a página 404 comum.
- Arquivo ausente no wiki: 404. Falha interna: 500.
- Caminho inválido: a confirmar se responde 400. A consulta de metadados é feita
  antes da validação do caminho, então um caminho inválido pode cair em 404.

### 2.4 Busca

- O termo é aparado. Termo vazio não executa consulta e mostra zero resultados.
- Até 50 resultados. A busca percorre apenas os projetos que o visualizador pode
  ler.
- Cada resultado mostra workspace, projeto, caminho, título e trecho com o termo
  destacado. O trecho é escapado, e só as marcações de realce são preservadas.
- O número total de resultados aparece na página.

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
  que começa com `_rules/`), confiança em percentual inteiro, data relativa de
  envio e conteúdo.

### 2.6 Autenticação nas páginas HTML

Aplica-se apenas a navegação: requisições GET ou HEAD que indicam documento
(cabeçalho de destino `document` ou `Accept` contendo `text/html`) e que não
são de `/api/v1`.

| Resposta original | Resultado para a navegação |
|---|---|
| 401 | Redirecionamento 303 para o login, com `next` sanitizado contendo o caminho externo completo e a query. |
| 403 comum | Redirecionamento 303 para a troca de senha. |
| 403 de administração (página de pendências) | Mantém a página de 403. |
| Qualquer outra | Sem alteração. |

O parâmetro `next` (usado no login e na troca de senha) é aceito somente se:
- não contém `://`, não começa com `//`, não contém barra invertida nem byte nulo;
- começa com `/`;
- o caminho (antes de `?` ou `#`) é a raiz do navegador ou está abaixo dela;
- não contém segmentos `.` ou `..`.

Qualquer outro valor é trocado pela raiz do navegador, ou por `/` quando o
navegador está montado na raiz do host.

### 2.7 Tema

O tema segue a preferência do sistema (modo escuro automático). Não há seletor
nem preferência salva. O único script embutido nas páginas é o botão de sair,
que chama a rota de logout enviando o token CSRF lido de um cookie.

---

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
| Busca, regras gerais | Termo vazio: resposta 200 com lista vazia, sem cabeçalho de cache. Máximo de 25 escopos. Escopos não podem ser combinados com `workspace` e `project` (400). `workspace` sem `project`, ou o contrário, é 400. Escopo com projeto inexistente é 404. |
| Busca, com vários escopos | Resultados unidos sem repetição (ficam os de menor rank por página), ordenados por rank crescente e cortados no limite. Resultados sem metadados correspondentes são descartados. |
| `recent` sem `updated_since` nem `cursor` | Snapshot: as páginas mais recentes por data de atualização, até `limit` (padrão 10, faixa 1 a 100). |
| `recent` com `updated_since` ou `cursor` | Modo incremental: páginas com atualização estritamente posterior ao instante informado. Resposta `{pages, next_cursor}`. A ordem das páginas: a confirmar. |
| `recent`, instante | RFC 3339, com 20 a 64 bytes, fração de até 9 dígitos e sufixo `Z` ou deslocamento `±hh:mm`. A precisão efetiva é de microssegundos, com truncamento. Valor inválido: 400. |
| `recent`, cursor | Opaco: base64 URL-safe sem preenchimento, com JSON versão 1. Limites: 8192 caracteres de entrada, 6144 bytes decodificados e 2048 caracteres no caminho. Deve pertencer ao mesmo escopo, senão 400. Se `updated_since` vier junto, deve ser igual ao instante do cursor, senão 400. O instante do cursor deve ser posterior ao instante `updated_since`, senão 400. |
| `briefing` e overviews | `limit`, padrão 10, faixa 1 a 100. |
| `handoffs` | `state`: `open`, `accepted` ou `expired`. Vazio ou ausente lista todos. Valor desconhecido: 400. `limit`, padrão 50, faixa 1 a 200. `all_owners`: só a raiz pode usar; outros chamadores recebem 403. Sem autorização para o corpo, o resumo, as perguntas em aberto e os próximos passos são omitidos e o campo `redacted` fica verdadeiro. |
| `sessions` | `limit`, padrão 20, faixa 1 a 100. `offset`, padrão 0. `include_open`, padrão falso: inclui sessões que ainda não foram encerradas. |
| `sessions/{id}/observations` | `limit`, padrão 50, faixa 1 a 200. `offset`. `order`: `asc` (padrão) ou `desc`, sem diferenciar maiúsculas; outro valor é 400. `kinds`: lista separada por vírgula; tipo desconhecido é 400. `q`: busca de texto restrita à sessão. `body_max_chars`, padrão 4000, faixa 200 a 16384. Corpos mais longos são cortados e recebem um marcador com a quantidade de caracteres omitidos. ID de sessão inválido: 400. Sessão inexistente ou não visível: 404. |
| `graph` | Arestas em que as duas extremidades estão em projetos diferentes e são legíveis pelo visualizador. Não há filtro por projeto. |

Campos da página completa: workspace, projeto, caminho, título, tipo, tier,
fixada, substituída, datas de criação e atualização, autor, frontmatter, corpo
em markdown, links de saída e backlinks.

### 3.4 Cache e ETag

| Política de cabeçalho | Respostas |
|---|---|
| `private, max-age=30` | Workspaces, projetos, grafo, busca com termo não vazio (GET e POST), `recent` em modo snapshot. |
| `private, max-age=60` | Lista de páginas do projeto. |
| `private, max-age=300` com ETag | Página completa. A resposta 304 também carrega esses cabeçalhos. |
| `private, no-store` | `recent` incremental, briefing, overviews, handoffs, sessões e observações. |
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

- Links aceitos sem alteração: `http://`, `https://`, `mailto:`, caminhos
  começando com `/` (inclusive caminhos de rede `//host`), âncoras `#...` e
  caminhos relativos sem `:`. Qualquer outro esquema vira `#`.
- Links de rede (`//host`) **são mantidos**. Só imagens bloqueiam esse caso.
- Links relativos do wiki (sem `#`, `/`, `\` e sem `:`) são reescritos para a
  URL da página dentro do projeto atual, como se fossem relativos à raiz do
  projeto. Isso vale para arquivos `.md` e para diretórios, como `_lint/`. A
  âncora ou query é preservada.
  - Caminhos com `..` não são reescritos. Um `./` inicial é removido, e a barra
    final é aparada.
  - Links que já estão no formato `w/…/p/…` não são reescritos de novo.
- Imagens são aceitas só se forem relativas: começam com `/` ou `#`, ou não têm
  `:`. Caminhos de rede, barra invertida e destino vazio viram `#`. Isso evita
  requisições automáticas a origens externas ao abrir a página.

### 4.3 Wikilinks

Formas reconhecidas:

- `[[alvo]]`: página no projeto atual.
- `[[alvo|rótulo]]`: o rótulo é o texto exibido. O divisor é o primeiro `|`, e
  o rótulo pode conter outros `|`.
- `[[projeto:caminho]]`: página de outro projeto do mesmo workspace.
- `[[workspace/projeto:caminho]]`: página de outro workspace.
- `[[_global:caminho]]`: projeto reservado `_global` no workspace padrão,
  independentemente da página atual.

Regras:

- O qualificador de escopo aceita só letras, dígitos, `-`, `_`, `/` e `.`.
  Qualquer outro caractere faz a parte antes dos dois-pontos ser tratada como
  caminho.
- Ficam como texto literal: alvo vazio, alvo que começa com `#`, alvo com
  esquema externo (`://`, `mailto:`, `data:`, `javascript:`, `vbscript:`,
  `tel:` ou `file:`), e `[[` sem `]]` correspondente.
- Caminho: se o último segmento tem ponto, precisa terminar em `.md`. Sem ponto,
  `.md` é acrescentado. Depois a validação de caminho de página rejeita
  travessia, caminho absoluto, barra invertida e segmentos vazios. Se a
  validação falhar, o texto fica literal.
- Sufixos `#âncora` e `?query` são preservados. Caracteres que quebrariam o
  destino do link (espaço, parênteses, colchetes, aspas e barra invertida) são
  percent-encoded.
- Dentro de código (inline, cercado ou indentado) e de blocos HTML, `[[...]]`
  nunca é reescrito. A detecção usa o mesmo analisador da renderização, então
  o que é código para a renderização também é código para os wikilinks.
- No rótulo, colchetes e parênteses são escapados para não fechar o link antes
  do fim.

### 4.4 Identificadores de cabeçalho

- O identificador é o texto do cabeçalho em minúsculas. Letras e dígitos são
  mantidos, sublinhados são mantidos, espaços e hífens viram um único `-`, e a
  pontuação é removida. Hífens finais são aparados. Texto vazio vira `section`.
- Identificadores repetidos recebem sufixo numérico: `-1`, `-2` e assim por
  diante.
- Um identificador explícito no cabeçalho também passa pela deduplicação.
- Âncoras de wikilink coincidem com esses identificadores.

### 4.5 Trechos da busca

O trecho vindo do índice de busca é escapado por completo, e depois apenas as
marcações de realce `<mark>` e `</mark>` são restauradas.

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
- Injeta-se, logo após a primeira tag que começa com `<head`, fora de comentários
  HTML:
  - `<base href="...">`, com a base de URLs;
  - `<meta name="ai-memory-base-path" content="...">`, com o prefixo de base,
    para que a interface monte `{base}/api/v1`;
  - no navegador embutido, `<meta name="ai-memory-login-path" content="...">`.
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

O navegador embutido redireciona `{slug}/` para `{slug}` com 301. A interface
customizada responde 200 em `{slug}/` e não redireciona. Com slug vazio, o
navegador fica na raiz e não há redirecionamento.

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

- Caminho de página inválido responde 400 ou 404? Ver seção 2.3.
- Ordem das páginas no modo incremental de `recent`. Ver seção 3.3.
- Mecanismo de descarte do diálogo de migração na página inicial. Ver seção 2.1.
