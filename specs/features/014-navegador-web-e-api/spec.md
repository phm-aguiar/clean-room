# Spec: Navegador web da wiki e API JSON `/api/v1`

## Metadados
- **ID:** 014
- **Status:** spec auditada contra o original (ver "Correções da auditoria" e os relatórios em `reports/`); sem plano nem tasks; em toda dúvida, reproduzir o original
- **Autor:** phm-aguiar
- **Data:** 2026-10-13
- **Feature Anterior:** 007 (sessão web, CSRF, acesso por projeto), 002 (leitura, busca, handoffs, sessões)
- **Roadmap:** `specs/features/roadmap.md`
- **Aprovado:** false

## Propósito
> Para quem, o quê e por quê.

Pessoas querem ler a memória sem um agente: navegar pelos projetos, abrir uma página renderizada, buscar e ver sessões e handoffs. Times querem construir interfaces próprias sobre a mesma memória. Esta feature entrega duas superfícies **somente leitura** sobre o mesmo estado: um navegador HTML renderizado no servidor (desligado por padrão) e uma API JSON em `/api/v1` para frontends de terceiros, com a opção de servir uma interface própria (SPA).

## Escopo

### Dentro do escopo
- **Navegador HTML** (`docs/wiki-browser.md` §2): início com cards de projetos (selo e faixa de correio pendente), árvore do projeto (pastas com 5 páginas, caixa de correio, sessões recentes), página renderizada com metadados, índice, citada-por/cita e link de histórico, listagem de namespace (pasta) paginada, **histórico de versões com diff de linhas** (§2.8), **linha do tempo de sessão** (§2.9), busca global e de um projeto com filtros de tipo e tier e rastro de ranking (`explain`), login e troca de senha, recursos estáticos embutidos, `favicon.ico` na raiz do host, redirecionamento `{slug}/` → `{slug}` **com status 308**, prefixo `/web` configurável; redirecionamento de 401 (exceto com desafio Basic) e 403 de navegação para login e troca de senha; cabeçalhos anti-moldura; tema.
- **Painel `/health`** (somente raiz, `docs/wiki-browser.md` §2.10): verificações com comando de correção e contadores do processo (a parte de embeddings/provedores só existe depois das features que as criam; sem elas, o painel mostra "none"/"off").
- **API `/api/v1`** (`docs/wiki-browser.md` §3, `docs/frontend-api.md` §4): workspaces, projetos (com ids), páginas, página completa (com `id`, links de saída, backlinks e autor), busca GET e POST, **histórico de página com diff** (§4.14), recentes (snapshot e incremental por cursor opaco), briefing, overviews de workspace e de projeto, handoffs (com `redacted`), sessões, observações de sessão, grafo entre projetos, favicon; parâmetros e limites da tabela do doc; erros JSON `{"error": "..."}` com 500 genérico; mensagens literais de escopo (`docs/frontend-api.md` §3).
- **Cache e ETag** (§3.4): políticas por resposta, ETag SHA-256 da resposta inteira na página completa, 304 com os mesmos cabeçalhos, nada de cache em erro.
- **CORS** só em `/api/v1`, só com origens configuradas e validadas antes de subir (§3.1, `docs/frontend-api.md` §9).
- **Markdown seguro** (§4): HTML bruto escapado, esquemas perigosos neutralizados, imagens só relativas **e confinadas** (sem `/`, sem `..`, sem primeiro segmento igual a uma rota do servidor), nunca script, wikilinks reescritos para rotas do navegador, ids de cabeçalho, trechos de busca.
- **Montagem e prefixo** (§5): normalização do prefixo, base de URLs injetada no HTML, divisão entre rotas públicas e protegidas, interface customizada servida pelo próprio servidor (`docs/frontend-api.md` §6).
- **Autenticação** pela 007: autenticação dupla (bearer ou sessão web) nas rotas protegidas e na API; troca de senha pendente bloqueia `/api/v1` com 403.
- **Acesso por projeto:** listas mostram só projetos legíveis pelo visualizador; escopo proibido 403; inexistente 404; leitura nunca cria escopo.

### Fora do escopo (explicitamente)
- Página `/pending` de triagem de propostas: depende das propostas da feature 022 (que acrescenta a página ao navegador).
- Diálogo de aviso de migração OKF na página inicial: fica para uma revisão da 006 se o doc for detalhado; o mecanismo de descarte está em "pendências de verificação" do próprio doc.
- Edição pela web (produto companion separado, segundo o doc).

## Comportamento Esperado

### Cenário Principal (Happy Path)
1. Com a interface web ligada, o operador abre `/web`, faz login, vê os cards dos projetos legíveis, abre um projeto e uma página renderizada.
2. Um frontend de terceiros chama `GET /api/v1/workspaces/w/projects/p/pages/notes/x.md` com bearer, recebe a página completa e um ETag; a segunda chamada com `If-None-Match` recebe 304.

### Cenários Alternativos
- **Escopo inexistente:** 404; escopo proibido: 403.
- **Parâmetro fora da faixa, cursor inválido ou de outro escopo:** 400.
- **Navegação sem sessão:** redirecionamento para login; a API devolve o erro sem redirecionar.
- **Página com HTML e script hostis:** renderizada com o conteúdo escapado e sem script.

## Correções da auditoria (2026-10-20)
Pontos em que a versão anterior desta spec, ou os docs dos quais ela dependia, divergiam do ai-memory de referência. Os docs foram corrigidos; a spec passa a exigir:
- **Redirecionamento `{slug}/` → `{slug}` é 308**, não 301 (o doc e a spec diziam 301); preserva a query string e só existe no navegador embutido.
- **Funcionalidades que existiam no navegador de referência e não estavam nos docs:** histórico de versões com diff de linhas (HTML e JSON), linha do tempo de sessão com navegação por posição, painel `/health` (raiz), caixa de correio e sessões recentes na página do projeto, selo/faixa de correio na página inicial, índice "On this page" e "Cited by"/"Cites" na página (o doc afirmava que links e backlinks **não** apareciam no HTML), listagem de namespace com ordenação e paginação, filtros `kind`/`tier` e rastro de ranking na busca, aviso fixo "LLM-optimised memory", cabeçalhos `X-Frame-Options: DENY` e `frame-ancestors 'none'`.
- **Busca HTML:** limite padrão 50 (global) e 20 (um projeto), faixa 1–100 (o doc dizia "até 50"); `scope` vence `workspace`+`project`; mensagens de 400 literais; o termo vazio nunca valida escopo.
- **Busca da API:** ordem de avaliação dos erros (decodificação → `limit` → formato de `scope` → termo vazio → combinações → 25 escopos → resolução), desempate por `path` e depois pelo id, mensagens literais.
- **Autenticação de navegação:** um 401 com desafio `WWW-Authenticate: Basic` (servidor só com token, sem contas humanas) **não** é redirecionado ao login; a redireção usa 303 e `next` codificado com um conjunto de bytes definido.
- **Imagens do markdown:** `/` inicial é **rejeitado** (o doc o aceitava); também `\`, `:`, `..` (inclusive `%2e%2e`) e o primeiro segmento igual a `admin`, `auth`, `handoff`, `hook`, `identity`, `mcp` ou `workstream`.
- **Wikilinks:** sem rótulo, o texto exibido é o alvo inteiro (com escopo e sufixo); `[[` nunca fecha em linha posterior; último segmento vazio ou `.md` = literal; ids de cabeçalho tratam travessões como separador e mantêm letras Unicode.
- **Resolvidas as 3 pendências do doc:** caminho de página inválido = 404; `recent` incremental ordena por `updated_at` e `path` crescentes; o diálogo de migração é descartado por chave no armazenamento local do navegador (fora do escopo desta feature, mas descrito).
- **API:** `id` na página completa; `workspace_id`/`project_id` na lista de projetos; `created_at` = versão mais antiga da cadeia; a leitura direta de página **não** esconde expiradas; definições de `stale`/`duplicates`/`orphans`; tabela de decisão de `redacted`; erro de dono em `all_owners` é 403 antes de qualquer redação; limite de corpo do `POST /search` = 2 MiB (413).
- **Fora do escopo desta feature (existe no Rust):** o console de gerenciamento `/manage` (compilação opcional, desligada), a página `/pending` (022) e o conteúdo da folha de estilo (os docs não a especificam; qualquer CSS com modo escuro automático serve).

## Edge Cases
- Busca com termo vazio: 200 com lista vazia, sem cabeçalho de cache.
- `recent` incremental: cursor opaco validado inteiro antes de usar (tamanhos e versão).
- Handoffs sem autorização para o corpo: campos de prompt omitidos e `redacted: true`.
- Interface atrás de proxy em subcaminho: URLs relativas e base injetada funcionam sem reescrever templates.
- Servidor só com token e sem contas humanas: o 401 de navegação traz o desafio Basic e **não** vai para o login.
- Página de projeto inexistente sem visualizador restrito: 200 com listas vazias (HTML); com restrição: 404.
- Diff de versões: acima de 512 KiB, 10 000 linhas, distância de edição 1 000 ou 2 000 linhas de saída, o resultado é `too_large`/truncado, nunca erro.
- Versão substituída fora do caminho (página movida): o diff padrão parte do vazio.
- Imagem com `/`, `..`, `%2e%2e`, `\`, `:` ou primeiro segmento de rota do servidor: vira `#`.

## Constraints
- **Segurança:** teste com payloads de XSS; CORS restrito; nada de mutação de conteúdo.
- **Tecnologia:** `html/template` e `embed` da stdlib; renderizador de markdown de terceiros (dependência nova, registrada em `tech.md`), com saída igual à do original nos casos do doc.

## Critérios de Sucesso
- [ ] Tabela de parâmetros e limites de `docs/wiki-browser.md` §3.3 coberta por teste
- [ ] Tabela de cache e ETag do §3.4 coberta por teste
- [ ] Suíte de XSS sobre a renderização de markdown, incluindo a tabela de imagens confinadas
- [ ] Diff de linhas: saídas idênticas às da tabela de limites e do desempate de `docs/wiki-browser.md` §2.8.1 (inclusive `@@` e truncamento)
- [ ] Tabela de verificações do `/health` (id, gravidade, condição, texto, comando) coberta por teste
- [ ] `go vet ./...`, `go test -race ./...` e `golangci-lint run` passam sem apontamentos

## Abordagem Escolhida
Um componente no `cli` (é a camada que monta rotas e conhece todos os componentes), só de leitura, sobre o `Reader` do `storage` e a leitura da `wiki`. Templates e recursos estáticos embutidos. Markdown renderizado por uma biblioteca com HTML bruto desligado e uma etapa própria de sanitização de URL e de reescrita de wikilinks.

## Dependências
- 007, 002; `docs/wiki-browser.md`, `docs/frontend-api.md`, `docs/mcp-server.md` §5, `docs/cli.md` §3.3.

## Checklist de Prontidão
- [x] Propósito claro e sem ambiguidade
- [x] Escopo delimitado (dentro/fora)
- [x] Cenários cobrem happy path e alternativos
- [x] Edge cases identificados com comportamento esperado
- [x] Constraints explícitas
- [x] Critérios de sucesso mensuráveis
- [x] Abordagem escolhida justificada
