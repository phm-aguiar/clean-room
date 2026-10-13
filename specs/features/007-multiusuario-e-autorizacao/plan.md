# plan.md: Plano de Implementação Técnica — Multiusuário e autorização (007)

> Plano gerado em 2026-10-13, antes de a 002 ser executada. Revisar quando as features anteriores terminarem.

## 1. Metadados do Plano
- **Stack Tecnológico:** a da 002 mais **`golang.org/x/crypto/argon2`** (Argon2id para senhas; a stdlib não tem). `crypto/sha256`, `crypto/subtle`, `crypto/rand`, `encoding/base64` (URL-safe, 43 caracteres para 32 bytes) da stdlib.
- **Feature Fonte:** `specs/features/007-multiusuario-e-autorizacao/spec.md`
- **Escopo:** contas, chaves nativas, senha, sessão web, CSRF, recuperação, bootstrap, níveis e capacidades, dono e atribuição, acesso por projeto, fatias por operador, atividade de cliente, recusas da inicialização, `init` e `generate-auth-token`. Sem proxy de identidade (009) e sem admissão (008).
- **Fontes de contrato:** `docs/users.md`, `docs/design-per-project-authz.md`, `docs/mcp-server.md` §4 a §6, `docs/security.md`, `docs/security-boundaries.md`, `docs/api-reference.md` §1, §3.6, §4 e §5.

## 2. Design de Contratos e Fronteiras
- **Rotas:** `/auth/login`, `/auth/recovery` (públicas); `/auth/me`, `/auth/password`, `/auth/logout` (sessão); `/admin/users*`, `/admin/human-users`, `/admin/api-credentials*`, `/admin/projects/grants`, `/admin/projects/access`, `/admin/users/{username}/grants`, `/admin/activity/by-client`, `/identity`.
- **CLI:** `init`, `generate-auth-token`, `user` (`add`, `add-human`, `list`, `expire`, `revive`, `rotate-token`, `reset-password`, `disable`, `enable`, `patch`, `grant`, `revoke`, `grants`), `api-key` (`add`, `list`, `rotate`, `revoke`), `project` (`access`, `grants`).
- **Configuração `[auth]`:** `bearer_token`, `token_pepper`, `root_username`, `root_email`, `root_name`, `secure_cookie`, `recovery_token`, `new_projects_restricted`; ambiente `AI_MEMORY_AUTH__INITIAL_ROOT_PASSWORD`, `AI_MEMORY_AUTH__RECOVERY_TOKEN`. `[slots] per_user`.
- **Esquema SQL:** sem migração. `users`, `api_credentials`, `web_sessions`, `human_auth_state`, `project_grants`, `projects.access_mode`, `projects.created_by`, `pages.author_id`, `audit_log`, `client_activity` já na V01.

## 3. Decisões Arquiteturais e Justificativas

- **Decisão (U-01): dependência `golang.org/x/crypto/argon2`.** Senhas em formato PHC Argon2id com parâmetros padrão e sal por senha.
  - **Justificativa:** `docs/users.md` e `docs/mcp-server.md` §5 exigem Argon2id; a stdlib não implementa. É mantida pelo time do Go. Entra em `tech.md` na task que a importa, antes do uso (portão da constituição).
  - **Alternativa Rejeitada:** bcrypt da mesma biblioteca (não é o contrato) e implementação própria (risco de segurança).

- **Decisão (U-02): um ponto de autenticação e um de autorização.** O middleware da 002 passa a resolver, na ordem do `mcp-server.md` §4.2: token vazio rejeitado; root por comparação em tempo constante; chave nativa ou token de compatibilidade pelo digest com pepper; senão rejeitado. Sessão web (cookie) só nas rotas que a aceitam. O resultado é o contexto do ator (nível, chave de identidade, conta). A autorização é uma função `Can(ctx, capacidade)` e uma função `Decide(ctx, viewer, projeto, necessidade)`, as únicas consultadas por rotas, tools e resolvedor de escopo.
  - **Justificativa:** `docs/mcp-server.md` §7 ("uma função de identidade", "controle de acesso por capacidade em um só lugar").
  - **Alternativa Rejeitada:** checagens espalhadas nos handlers.

- **Decisão (U-03): decisão por projeto no resolvedor de escopo e no escritor.** Leituras com escopo decidem no caminho de leitura; escritas decidem na conexão do escritor, dentro da transação, para uma troca de grant não correr contra a escrita. Leituras sem escopo recebem o predicado de visibilidade (aberto, criado pelo viewer, com grant, ou escopo global) dentro da consulta, antes do `LIMIT`. Entradas por id cru checam o projeto da linha.
  - **Justificativa:** `docs/design-per-project-authz.md` § "Where the rule is enforced".
  - **Alternativa Rejeitada:** filtrar depois do limite (vaza existência e contagem).

- **Decisão (U-04): senhas fora do escritor, com limite de 2.** Verificação e hash Argon2id rodam num semáforo de 2; saturado, `429 kdf saturated`. Usuário inexistente, desabilitado ou sem senha executa uma verificação fictícia para igualar o tempo.
  - **Justificativa:** `docs/mcp-server.md` §5.
  - **Alternativa Rejeitada:** hash dentro do escritor (bloquearia todas as escritas).

- **Decisão (U-05): limite de login por IP em memória.** Mapa de no máximo 4096 IPs com janela de 60 s e 10 falhas; ao estourar, sai o IP mais antigo. IP do cliente pela conexão (o `X-Forwarded-For` com CIDRs confiáveis entra com a 015).
  - **Justificativa:** `docs/mcp-server.md` §5.
  - **Alternativa Rejeitada:** persistir em tabela (o doc não define).

- **Decisão (U-06): dono gravado só quando distingue operadores.** Sessões e handoffs recebem a chave de identidade do ator só se o servidor distingue operadores; `NULL` é compartilhado; leituras de handoff e de sessão trazem as próprias linhas e as compartilhadas; `any_owner`/`all_owners` exigem root.
  - **Justificativa:** `docs/users.md` § "Owner stamping".
  - **Alternativa Rejeitada:** sempre gravar o dono (a CLI local e o stdio deixariam de ver dados gravados por HTTP).

- **Decisão (U-07): atribuição na escrita de página.** O `WritePage` recebe o ator: com conta, grava `pages.author_id`; com username, acrescenta o bloco `last_modified_by` (`username`, `name`, `email`) no frontmatter (passo 3 do `docs/wiki-files-layer.md` §5.2); anônimo não gera bloco.
  - **Justificativa:** `docs/users.md` § Attribution.
  - **Alternativa Rejeitada:** guardar autoria só no banco (o markdown é a fonte da verdade).

- **Decisão (U-08): recusas da inicialização no `serve`.** Uma função de validação roda depois de abrir o banco e antes de servir, com as condições de `docs/users.md` § "Startup refusals" e `docs/cli.md` §3.1 passo 7.
  - **Justificativa:** falhar no início, não horas depois.
  - **Alternativa Rejeitada:** validar sob demanda.

- **Decisão (U-09): contratos de teste.** Matriz de capacidades nos dois modos; regra de decisão passo a passo; degradação; filtro antes do limite; id cru; adversariais do `mcp-server.md` §7; recusas da inicialização; tempo igual no login.
  - **Justificativa:** `docs/security-boundaries.md` lista o mapa de testes adversariais.
  - **Alternativa Rejeitada:** só caminho feliz.

### O que não construir nesta feature
Proxy de identidade e OIDC (009), admissão (008), interface web (014), redes de proxy confiáveis e `X-Forwarded-For` (015), propostas por operador (022).

## 4. Auditoria de Constituição

**Portões de Qualidade**
- [ ] Build cruzado, `go vet`, `go test -race` — U-09.
- [ ] Teste multi-sessão em filtros de dono — U-06 (dois operadores, handoffs e sessões).
- [ ] Gherkin — os cenários da spec viram o `.feature` da 007.
- [ ] Dependência nova só via `tech.md` — U-01 (`golang.org/x/crypto/argon2`).

**Restrições Arquiteturais**
- [ ] INV-01 — `[auth]` e as variáveis `AI_MEMORY_AUTH__*` só em `internal/config`.
- [ ] INV-02 — decisão de escrita na conexão do escritor (U-03); hash fora (U-04).
- [ ] INV-16 — dono só em handoffs e sessões; leitura de página nunca filtrada por autoria.
- [ ] Autorização por capacidade, nunca por nome de usuário; tokens em tempo constante — U-02.
- [ ] Resolução de escopo central — U-03.

**Anti-Padrões Proibidos**
- [ ] Sem filtro de leitura por autoria — U-07 é atribuição, não filtro.
- [ ] Sem `panic` — falhas de autenticação viram 401/403/429.
