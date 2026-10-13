# Spec: Multiusuário, autenticação humana e autorização por projeto

## Metadados
- **ID:** 007
- **Status:** draft (revisada contra os docs em 2026-10-13; nova revisão prevista quando as features anteriores terminarem)
- **Autor:** phm-aguiar
- **Data:** 2026-10-13
- **Feature Anterior:** 002 (servidor HTTP com token de administrador, MCP, hooks, handoffs com dono)
- **Roadmap:** `specs/features/roadmap.md`
- **Aprovado:** false

## Propósito
> Para quem, o quê e por quê.

Um servidor compartilhado precisa responder "quem fez esta requisição" e "quem escreveu ou é dono deste dado": contas com chaves de API nativas, login humano com sessão web e CSRF, atribuição de autoria e, opcionalmente, acesso restrito por projeto. Contas **atribuem**; só o acesso por projeto (opt-in) restringe leitura. Uma instalação de um operador só, sem contas, mantém o comportamento da 002.

## Escopo

### Dentro do escopo
- **Modo multiusuário** (`docs/users.md` § "When multi-user mode applies"): "distingue operadores" quando existe ao menos uma conta (o proxy de identidade fica na 009), avaliado a cada requisição sem cache.
- **Níveis e capacidades** (`docs/users.md` § "Capabilities by level", `docs/mcp-server.md` §4.4): anônimo, usuário e root; decisão num único ponto; `/admin/*` só root quando distingue operadores; gestão de contas e credenciais sempre só root; recuperação entre donos (`any_owner`, `all_owners`) só root; pular a cadeia de admissão só para root e anônimo.
- **Contas** (§ Accounts): campos, regras de username, nome, e-mail e senha; criação de pessoa (`user add-human`, `POST /admin/human-users`) com senha temporária mostrada uma vez; bootstrap do primeiro root por `AI_MEMORY_AUTH__INITIAL_ROOT_PASSWORD`; status (`disabled`, `must-change`, `active`, `expired`); `must-change`; desabilitar e habilitar; redefinir senha; trocar a própria senha; alterar nome, e-mail e papel; o último root recuperável não pode ser desabilitado nem rebaixado.
- **Credenciais** (§ Credentials): token root (configuração), chave nativa `aim_` (digest com pepper e prévia de 4 caracteres), senha Argon2id, senha temporária, segredo de sessão `ams_` e CSRF (SHA-256 sem pepper), token de recuperação (SHA-256 em memória). Token de compatibilidade e comandos `user add`, `expire`, `revive`, `rotate-token` (obsoletos, ainda no contrato).
- **Chaves de API nativas** (§ "Native API keys"): `api-key add`, `rotate`, `revoke`, `list`; várias por conta; sempre nível usuário; validade opcional honrada na consulta.
- **Login humano e sessão web** (§ "Human login", `docs/mcp-server.md` §5): cookie de sessão HttpOnly `SameSite=Strict`, cookie de CSRF legível, 7 dias sem tempo ocioso, último uso no máximo a cada minuto, CSRF em métodos não seguros, limite de 10 falhas em 60 s por IP (tabela de 4096 IPs), verificação fictícia para usuário inexistente, até 2 verificações Argon2id simultâneas (`kdf saturated`), recuperação (`POST /auth/recovery`, 204, sem sessão), `/auth/login`, `/auth/me`, `/auth/password`, `/auth/logout`; transição do transporte legado (Basic e cookie antigo só para GET até ativar o login humano).
- **Chaves de identidade e dono** (§ "Identity keys and ownership"): `user:<username>` e `oidc:<len><issuer><subject>`; dono gravado só quando distingue operadores; `NULL` é compartilhado; leituras trazem as próprias linhas e as compartilhadas.
- **Atribuição** (§ Attribution): `pages.author_id` para chave nativa e sessão web; bloco `last_modified_by` no frontmatter quando o ator tem username; conta na auditoria; objeto `author` nas respostas de página; root nomeado por `root_username`.
- **Acesso por projeto** (`docs/design-per-project-authz.md`, partes "Implemented"): modo `open` ou `restricted`, criador, modo inicial por nome (`_profile.` restrito; `scratch`, `_global` e `_profile` abertos; demais por `[auth] new_projects_restricted`), grants `read` e `write` com auditoria, regra de decisão na ordem do doc, degradação para aberto com aviso, filtro dentro da consulta nas leituras sem escopo, checagem nas entradas por id cru, `project access`, `user grant`, `user grants`, `user revoke` e as rotas correspondentes.
- **Fatias por operador** (`[slots] per_user`, § "Per-operator memory slots") e **atividade de cliente MCP** (`GET /admin/activity/by-client`, tabela `client_activity`, até 128 rótulos por dia).
- **Recusas na inicialização** (§ "Migration and backward compatibility") e comandos `init` e `generate-auth-token` (pepper e token).
- **Hooks por pessoa:** `install-hooks --as-user --auth-token` (`docs/users.md` § "Hooks and agent wiring").

### Fora do escopo (explicitamente)
- Proxy de identidade confiável e SSO (feature 009); a 007 deixa a decisão de nível pronta para receber o terceiro degrau.
- Cadeia de admissão (feature 008).
- Interface web (feature 014); a 007 entrega as rotas `/auth/*` e as regras de sessão.
- Propostas de auto-melhoria por operador (feature 022).

## Comportamento Esperado

### Cenário Principal (Happy Path)
1. O operador roda `ai-memory init` (gera o pepper) e, com o token root, `ai-memory user add-human --username alice`; recebe uma senha temporária uma única vez.
2. `ai-memory api-key add --username alice --label claude-hooks` imprime a chave `aim_...` uma vez; `install-hooks --apply --agent claude-code --as-user alice --auth-token aim_...` grava o token nos arquivos do diretório de dados.
3. As escritas de Alice registram `author_id`, `last_modified_by` e a conta na auditoria; os handoffs dela têm dono e só ela os recebe.
4. Com `project access --mode restricted` num projeto, só root, o criador e quem tem grant entram.

### Cenários Alternativos
- **Credencial errada, conta desabilitada, conta só com token ou usuário inexistente no login:** 401 `invalid credentials`, com o mesmo tempo de resposta.
- **Mais de 10 falhas em 60 s do mesmo IP:** 429 antes de verificar a senha.
- **Sessão com troca de senha pendente:** `/admin/*` e `/api/v1` dão 403 `password change required`.
- **Usuário em `/admin/*` quando distingue operadores:** 403; anônimo: 401.
- **Projeto restrito sem grant:** recusa com a mensagem do doc, citando o projeto e os níveis.
- **Grants ilegíveis:** decisão degrada para aberto com aviso; no escopo global, leitura aberta e escrita recusada.
- **Pepper ausente com chaves existentes, e demais recusas da inicialização:** o servidor não sobe.

## Edge Cases
- **Primeira conta criada:** o acesso administrativo aberto fecha na hora, sem reinício.
- **Chave nativa de conta root:** continua nível usuário.
- **Desabilitar conta:** encerra as sessões; as chaves nativas continuam valendo até serem revogadas.
- **Leituras sem escopo** (`global`, recentes globais): filtro aplicado dentro da consulta, antes do limite; quem não tem grant vê só os projetos abertos.
- **Mensagens e entradas por id cru:** checam o projeto resolvido pela linha (por exemplo, aceitar handoff exige leitura).
- **Escopo global:** legível por todos; escrita só root ou grant `write`, em qualquer modo.
- **Instalação de um operador só:** nenhum dono é gravado, mesmo com `root_username`.

## Constraints
- **Segurança:** comparação em tempo constante para token root; chaves e sessões achadas pelo digest; senhas Argon2id com sal por senha; nada de segredo em listagem; cookies `Secure` conforme `[auth].secure_cookie`, nunca por cabeçalho do proxy; login humano por HTTP simples só em loopback.
- **Tecnologia:** Argon2id exige `golang.org/x/crypto/argon2` (fora da stdlib), registrado em `tech.md` por ADR antes do uso.
- **Concorrência:** decisão de escrita tomada na conexão do escritor, para uma troca de grant não correr contra a escrita que ela guarda.

## Critérios de Sucesso
- [ ] Matriz de capacidades de `docs/users.md` coberta por teste em modo de um operador e em modo multiusuário
- [ ] Regra de decisão de `docs/design-per-project-authz.md` coberta passo a passo, incluindo degradação e escopo global
- [ ] Testes adversariais de `docs/mcp-server.md` §7 que se aplicam (token repetido, escopo de outro projeto, `any_owner` sem root)
- [ ] Recusas da inicialização testadas uma a uma
- [ ] `scripts/check.sh` e o lint passam sem apontamentos

## Abordagem Escolhida
Um ponto único de autenticação no middleware (resolução do token portador e da sessão) que produz o contexto do ator, e um ponto único de autorização (capacidade por nível e decisão por projeto) consultado pelo resolvedor de escopo. Contas, chaves, sessões e grants no `storage`, sobre as tabelas da V01. Senhas em processamento separado do escritor, limitado a 2 verificações simultâneas.

## Dependências
- 002: middleware de autenticação, resolvedor de escopo, handoffs com dono, `install-hooks`.
- `docs/users.md`, `docs/design-per-project-authz.md`, `docs/mcp-server.md` §4 a §6, `docs/security.md`, `docs/security-boundaries.md`, `docs/api-reference.md` §3.6, §4 e §5.

## Checklist de Prontidão
- [x] Propósito claro e sem ambiguidade
- [x] Escopo delimitado (dentro/fora)
- [x] Cenários cobrem happy path e alternativos
- [x] Edge cases identificados com comportamento esperado
- [x] Constraints explícitas
- [x] Critérios de sucesso mensuráveis
- [x] Abordagem escolhida justificada
