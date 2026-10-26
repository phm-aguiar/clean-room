# Spec: Multiusuário, autenticação humana e autorização por projeto

## Metadados
- **ID:** 007
- **Status:** spec auditada contra o original (ver "Correções da auditoria" e os relatórios em `reports/`); sem plano nem tasks; em toda dúvida, reproduzir o original
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
- **Credenciais** (§ Credentials): token root (configuração), chave nativa `aim_` (SHA-256 de `segredo:pepper`; prévia `…` + 4 últimos caracteres), senha Argon2id (m=19 456 KiB, t=2, p=1, sal de 16 bytes, PHC), senha temporária de 32 caracteres, segredo de sessão `ams_` e CSRF (SHA-256 sem pepper), token de recuperação (SHA-256 em memória). **Fora do contrato:** os comandos e rotas `user add`, `expire`, `revive`, `rotate-token` e `POST /admin/users` foram **removidos** no original (3.0); só resta `GET /admin/users`. Não há loja antiga a migrar (decisão herdada da 005), então também não há linhas `legacy-user-token` a tratar.
- **Chaves de API nativas** (§ "Native API keys"): `api-key add`, `rotate`, `revoke`, `list`; várias por conta; sempre nível usuário; validade opcional honrada na consulta.
- **Login humano e sessão web** (§ "Human login" e "Login", com a ordem exata de verificações e respostas em `docs/users.md`): cookies `ai_memory_session` (HttpOnly) e `ai_memory_csrf` (legível), ambos `SameSite=Strict; Path=/; Max-Age` e `Secure` conforme `[auth].secure_cookie`, mais expiração do cookie legado `ai_memory_auth`; 7 dias absolutos, último uso no máximo a cada minuto; cabeçalho `X-CSRF-Token` igual ao cookie e ao digest da sessão em métodos não seguros (403 `csrf required`); exigência de `Content-Type` JSON (415); corpo de até 64 KiB; limite de 10 falhas em 60 s por endereço (tabela de 4 096 endereços; endereço do par TCP, ou salto mais à direita fora dos CIDRs confiáveis de `X-Forwarded-For`); verificação fictícia para usuário inexistente/desabilitado/sem senha; até 2 verificações Argon2id simultâneas e no máximo 32 na fila (`kdf saturated`, 429); recuperação (`POST /auth/recovery`, 204, sem sessão), `/auth/login`, `/auth/me`, `/auth/password` (204, 400/401/409 conforme o doc), `/auth/logout` (204); transição do transporte legado (Basic e cookie antigo só para GET até ativar o login humano; Basic nunca emite cookie). O endpoint interno `POST /internal/auth/session-introspect` pertence à feature 009.
- **Chaves de identidade e dono** (§ "Identity keys and ownership"): `user:<username>` e `oidc:<N>:<issuer><subject>` (N = bytes do emissor, seguido de dois-pontos); nome de diretório por operador `u-<usuário>` / `uh-<hex>` / `o-<hex>` (UUID v5, espaço de nomes URL, 32 hex minúsculos); dono gravado só quando distingue operadores; `NULL` é compartilhado; leituras trazem as próprias linhas e as compartilhadas.
- **Atribuição** (§ Attribution): `pages.author_id` para chave nativa e sessão web; bloco `last_modified_by` no frontmatter quando o ator tem username; conta na auditoria; objeto `author` nas respostas de página; root nomeado por `root_username`.
- **Acesso por projeto** (`docs/design-per-project-authz.md`, partes "Implemented"): modo `open` ou `restricted`, criador (admitido pela regra de criador, **sem linha de grant**), modo inicial por nome (`_profile.` restrito; `scratch`, `_global` e `_profile` abertos; demais por `[auth] new_projects_restricted`, lido na inicialização), grants `read` e `write` com auditoria (`grant_access`/`revoke_access` com o detalhe do doc `users.md`), regra de decisão na ordem do doc, degradação para aberto com aviso, filtro dentro da consulta nas leituras sem escopo, checagem nas entradas por id cru, `project access` (recusa `_global` e perfis), `project grants`, `user grant`, `user grants`, `user revoke` e as rotas correspondentes com as respostas exatas do `users.md`. Sessão web de usuário e chave nativa carregam o espectador; sessão root, token root e instalação sem contas não.
- **Fatias por operador** (`[slots] per_user`, § "Per-operator memory slots") e **atividade de cliente MCP** (`GET /admin/activity/by-client`, tabela `client_activity`, até 128 rótulos por dia).
- **Recusas na inicialização** (§ "Migration and backward compatibility", lista ampliada no doc: arquivo de token ausente, CIDR inválido, recuperação curta ou com prefixo reservado, senha inicial inválida, exposição sem autenticação fora de loopback) e comandos `init` e `generate-auth-token` (pepper e token).
- **Token raiz em arquivo** (`docs/users.md` § "Local root bearer token file"): `[auth].bearer_token_file`; `init` em instalação **nova** (sem configuração nem banco) gera o token em `<dados>/auth-tokens/default`, registra a chave na configuração e semeia as credenciais dos hooks, de modo que o servidor passa a exigir bearer em todas as rotas exceto `/healthz`; instalações existentes não mudam; `auth enable` (idempotente, `--no-mcp`) faz o mesmo para instalações antigas e reregistra os clientes MCP; recusa com as duas chaves juntas, arquivo ausente/vazio faz o servidor não subir; descoberta implícita em loopback; linhas de postura em `status`/`doctor`.
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
- **Mutações de execução gerenciada (finalizar, cancelar, heartbeat, vincular, aceitar contexto):** a regra de dono vale **também para root**: o dono da execução precisa coincidir com a identidade de quem chama, senão 403 `managed run belongs to another operator`, sem alterar a execução; chamador sem identidade só mexe em execução sem dono; execução sem dono é admitida a todos. Depois vem a checagem de escrita no projeto em modo estrito (erro, não degradação). Exceção: identidade de proxy confiável sem linha de usuário só passa pela regra de dono.
- **Evento de hook com identificador de sessão de outro dono ou agente:** descartado com confirmação terminal (`dropped_collision`), aviso no log sem detalhes da linha, sem erro ao remetente e sem nova tentativa.
- **`install-hooks --as-user`:** só rótulo; exige `--auth-token` (aparado) com a mensagem do doc, imprime `[ai-memory] hooks installing for user: <nome>` no erro padrão e não consulta o servidor.
- **Política de senha:** mensagem genérica `does not meet policy`; as rotas admin nunca recebem senha escolhida (só temporária gerada).
- **Grant no escopo global:** permitido (basta o projeto existir); só mudar o **modo** de acesso do escopo global é recusado.
- **Lacunas que o original também deixa abertas** (replicar, não corrigir): consolidação de sessão sem alvo resolvível não faz checagem de escrita antes de rodar (pertence à feature de consolidação).

## Constraints
- **Segurança:** comparação em tempo constante para token root; chaves e sessões achadas pelo digest; senhas Argon2id com sal por senha; nada de segredo em listagem; cookies `Secure` conforme `[auth].secure_cookie`, nunca por cabeçalho do proxy; login humano por HTTP simples só em loopback.
- **Tecnologia:** Argon2id exige `golang.org/x/crypto/argon2` (fora da stdlib), registrado em `tech.md` por ADR antes do uso.
- **Concorrência:** decisão de escrita tomada na conexão do escritor, para uma troca de grant não correr contra a escrita que ela guarda.

## Critérios de Sucesso
- [ ] Matriz de capacidades de `docs/users.md` coberta por teste em modo de um operador e em modo multiusuário
- [ ] Regra de decisão de `docs/design-per-project-authz.md` coberta passo a passo, incluindo degradação e escopo global
- [ ] Testes adversariais de `docs/mcp-server.md` §7 que se aplicam (token repetido, escopo de outro projeto, `any_owner` sem root)
- [ ] Recusas da inicialização testadas uma a uma
- [ ] `go vet ./...`, `go test -race ./...` e `golangci-lint run` passam sem apontamentos

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

## Correções da auditoria

Auditoria 012 (Chinese Wall, contra o código original). Relatório: `reports/012-auditoria-006-007-009.md`.

- `docs/users.md`: nomes dos cookies (`ai_memory_session`, `ai_memory_csrf`, legado `ai_memory_auth`), forma exata do `Set-Cookie`, regra CSRF completa (cabeçalho `x-csrf-token` = cookie, tempo constante, e SHA-256 = hash guardado; 403 `csrf required`); bearer em rota só de sessão → 401; sessão de conta desativada → 401 mesmo sem revogação.
- `docs/users.md`: nova seção da introspecção interna de sessão (`/internal/auth/session-introspect`): ordem das checagens, CSRF só para POST/PUT/PATCH/DELETE e sem comparação com cookie, `can_manage_api_keys`, e o fato de a rota não recusar por si conta desativada.
- `docs/users.md`: resolvido o "a confirmar" da sessão alheia em hook (evento descartado como colisão, sem detalhes da linha); mensagens literais das checagens de segredos na inicialização; token root sem mínimo; inexistência de rota de exclusão de conta.
- `docs/users.md`: **erro corrigido** — `generate-auth-token 32` não existe; o tamanho é a opção `--bytes` (padrão 32). Hash da chave nativa: SHA-256 de segredo ‖ `:` ‖ pepper, com o pepper usado como texto (hex não decodificado).
- `docs/security.md`: nomes dos cookies e CSRF; `generate-auth-token` (hex minúsculo, sem mínimo, mensagem de falha); hooks em script não usam token OIDC (resolvido).
- `docs/threat-model-and-data-handling.md`: resolvidos `new_projects_restricted` (desligado por omissão), ACL no Windows (herdada, sem código), mínimo do token de recuperação (32 bytes, sem prefixo reservado), ausência de exclusão de conta.
- `docs/design-per-project-authz.md`: "page evidence counts" fechado (não há entrada por id cru; contagem só em recuperação já escopada); aplicação estrutural **não** aplicada (consultas por id sem escopo seguem públicas na camada de leitura); consolidação sem alvo permanece com a feature 020.
- Verificados sem divergência: Argon2id (m=19456 KiB, t=2, p=1, sal 16 bytes, PHC), fila de KDF (2 simultâneos, 32 pendentes, 429 `kdf saturated`), verificação fictícia, senha temporária (24 bytes → 32 caracteres URL-safe), sessão `ams_`+43, CSRF 24 bytes→32 caracteres, TTL 7 dias, toque ≥ 60 s, SHA-256 sem pepper.
