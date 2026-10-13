# Spec: Navegador web da wiki e API JSON `/api/v1`

## Metadados
- **ID:** 014
- **Status:** draft (revisada contra os docs em 2026-10-13; o contrato campo a campo da API é extraído na fase 0 do `tasks.md`; nova revisão prevista quando a 007 terminar)
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
- **Navegador HTML** (`docs/wiki-browser.md` §2): início com cards de projetos, árvore do projeto, página renderizada com metadados, busca, login e troca de senha, recursos estáticos embutidos, `favicon.ico` na raiz do host, redirecionamento `{slug}/` → `{slug}`, prefixo `/web` configurável; redirecionamento de 401 e 403 de navegação para login e troca de senha; tema.
- **API `/api/v1`** (`docs/wiki-browser.md` §3, `docs/frontend-api.md` §4): workspaces, projetos, páginas, página completa (com links de saída, backlinks e autor), busca GET e POST, recentes (snapshot e incremental por cursor opaco), briefing, overviews de workspace e de projeto, handoffs (com `redacted`), sessões, observações de sessão, grafo entre projetos, favicon; parâmetros e limites da tabela do doc; erros JSON `{"error": "..."}` com 500 genérico.
- **Cache e ETag** (§3.4): políticas por resposta, ETag SHA-256 da resposta inteira na página completa, 304 com os mesmos cabeçalhos, nada de cache em erro.
- **CORS** só em `/api/v1`, só com origens configuradas e validadas antes de subir (§3.1, `docs/frontend-api.md` §9).
- **Markdown seguro** (§4): HTML bruto escapado, esquemas perigosos neutralizados, imagens só relativas, nunca script, wikilinks reescritos para rotas do navegador, ids de cabeçalho, trechos de busca.
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

## Edge Cases
- Busca com termo vazio: 200 com lista vazia, sem cabeçalho de cache.
- `recent` incremental: cursor opaco validado inteiro antes de usar (tamanhos e versão).
- Handoffs sem autorização para o corpo: campos de prompt omitidos e `redacted: true`.
- Interface atrás de proxy em subcaminho: URLs relativas e base injetada funcionam sem reescrever templates.

## Constraints
- **Segurança:** teste com payloads de XSS; CORS restrito; nada de mutação de conteúdo.
- **Tecnologia:** `html/template` e `embed` da stdlib; renderizador de markdown de terceiros (dependência nova, ADR no plano).

## Critérios de Sucesso
- [ ] Tabela de parâmetros e limites de `docs/wiki-browser.md` §3.3 coberta por teste
- [ ] Tabela de cache e ETag do §3.4 coberta por teste
- [ ] Suíte de XSS sobre a renderização de markdown
- [ ] `scripts/check.sh` e o lint passam sem apontamentos

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
