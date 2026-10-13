# Spec: SSO e identidade corporativa (OIDC)

## Metadados
- **ID:** 009
- **Status:** draft (revisada contra os docs em 2026-10-13; nova revisão prevista quando as features anteriores terminarem)
- **Autor:** phm-aguiar
- **Data:** 2026-10-13
- **Feature Anterior:** 007 (níveis, chaves de identidade, ponto único de autenticação)
- **Roadmap:** `specs/features/roadmap.md`
- **Aprovado:** false

## Propósito
> Para quem, o quê e por quê.

Equipes com um provedor de identidade central (IdP OIDC) precisam que cada desenvolvedor se identifique uma vez na CLI e que essa identidade chegue ao servidor. O ai-memory **não valida** token OIDC no servidor: a identidade chega por um **proxy de identidade confiável** (um gateway que valida o token e chama o servidor com um bearer próprio e cabeçalhos `X-Memory-Actor-*`). Esta feature entrega o login do desenvolvedor pelo fluxo de dispositivo, o armazenamento e a renovação do token, a ordem de bearer no cliente, e o degrau do proxy confiável na autenticação do servidor.

## Escopo

### Dentro do escopo
- **Login** `ai-memory auth login oidc-device --issuer --client-id [--timeout-secs]` (`docs/sso.md` § "Sign-in flow"): descoberta (só os endpoints de dispositivo e de token), autorização de dispositivo com escopo `openid profile offline_access`, impressão da URI de verificação, sondagem com intervalo do servidor (padrão 5 s, mínimo 1 s) mais 3 s, `slow_down` soma 5 s, limite pelo menor entre `--timeout-secs` (600) e `expires_in` (600), desfechos da tabela do doc, refresh token obrigatório, validade por `expires_in` (300 se ausente).
- **Armazenamento** em `auth.json` no diretório de dados, chave `oidc`, tipo `oauth`, preservando outros provedores; escrita atômica; `0600` em Unix; CLI imprime emissor e caminho, nunca os tokens.
- **Renovação** (§ Refresh): margem de 60 s, `refresh_token` sem segredo de cliente, tempo máximo de 300 s, novo refresh token substitui o antigo se vier, salvo antes de usar; falha de renovação ou de gravação usa o token antigo sem avisar quem chamou.
- **Ordem de bearer** (§ "Where the token is used"): hooks nativos (token explícito, token persistido da instalação, token OIDC, nenhum) e comandos da CLI que chamam o servidor (`AI_MEMORY_AUTH_TOKEN` ou `[auth].bearer_token`, token OIDC, nenhum); no spool, o evento grava só o modo, e o token é resolvido (e renovado) uma vez por envio.
- **`auth status`** e **`auth logout oidc-device`** (remove só a chave `oidc`, sem contatar o IdP).
- **Proxy de identidade confiável no servidor** (§ "Trusted proxy identity", `docs/mcp-server.md` §4.2, `docs/users.md`): `[auth].actor_proxy_bearer_token`, comparação em tempo constante, só com bearer explícito (nunca Basic nem cookie); cabeçalhos `X-Memory-Actor-User`, `-Issuer`, `-Sub`, `-Agent`, `-Client`, `-Session-Id`; cabeçalho repetido ou com vírgula dá 400; precisa de usuário ou do par completo emissor/assunto, senão 400; vazios contam como ausentes; nível usuário, root só com `[auth].root_issuer` e `[auth].root_subject` exatos; chave de identidade com o par vencendo o username.
- O proxy configurado conta para "distingue operadores" (`docs/users.md`).
- **Recusas da inicialização** do proxy (`docs/users.md` § "Startup refusals"): proxy sem bearer root, proxy igual ao root, emissor e assunto não definidos juntos.

### Fora do escopo (explicitamente)
- Validação de assinatura, emissor, audiência ou validade do token no servidor; busca de chaves do IdP; mapeamento de grupos ou papéis; ligação entre identidade OIDC e conta local (o doc diz que nada disso existe).
- Aplicar o acesso por projeto restrito a identidades do proxy (limitação documentada em `docs/users.md`).
- Outros provedores de login do mesmo comando (fora do `sso.md`).
- Ponte `mcp-bridge` (feature 010).

## Comportamento Esperado

### Cenário Principal (Happy Path)
1. O desenvolvedor roda `ai-memory auth login oidc-device --issuer https://idp/realms/team --client-id ai-memory-cli`, abre a URI impressa e aprova.
2. A CLI grava o token em `auth.json` e imprime o emissor e o caminho.
3. Os hooks e a CLI passam a usar o token OIDC como bearer quando não há bearer estático; perto do vencimento, renovam.
4. Num servidor atrás de um gateway, o gateway valida o token e chama o servidor com o bearer do proxy e `X-Memory-Actor-Issuer`/`-Sub`; o servidor atribui as escritas à chave `oidc:...`.

### Cenários Alternativos
- **Descoberta ou autorização com status de erro, `access_denied`, `expired_token`, tempo esgotado, resposta sem refresh token:** login falha e nada é gravado.
- **Token OIDC cru enviado a um servidor protegido:** 401 com desafio Bearer.
- **Proxy com cabeçalho repetido ou com vírgula, sem identidade ou com par parcial:** 400.
- **Bearer de proxy que não confere:** cai para o próximo degrau, normalmente 401.

## Edge Cases
- **Renovação recusada pelo IdP:** usa o token vencido; o servidor provavelmente devolve 401.
- **Token renovado que não pode ser salvo:** usa o token antigo naquela requisição, para ficar coerente com o arquivo.
- **Username só, sem par:** mesma chave de um usuário local com o mesmo nome; recomendação do doc é sempre afirmar o par.
- **Perfil de hook com servidor e token próprios:** nunca usa o token OIDC.

## Constraints
- **Segurança:** tokens nunca impressos nem em log; arquivo atômico e `0600`; cabeçalhos de identidade só valem com o bearer do proxy; root nunca por username.
- **Tecnologia:** `net/http`, `encoding/json`, `crypto/subtle` da stdlib.

## Critérios de Sucesso
- [ ] Tabela de desfechos da sondagem e tabela de falhas de `docs/sso.md` cobertas por teste contra um IdP de teste
- [ ] Ordem de bearer de hooks e CLI testada
- [ ] Regras do proxy (400, nível, chave de identidade) testadas, incluindo os adversariais do `docs/mcp-server.md` §7
- [ ] `scripts/check.sh` e o lint passam sem apontamentos

## Abordagem Escolhida
Cliente OIDC no `cli` (fluxo de dispositivo, armazenamento e renovação), usado pela resolução de bearer do hook e dos comandos. No servidor, um degrau novo no ponto único de autenticação da 007, entre o root e as credenciais de banco, que produz o contexto do ator a partir dos cabeçalhos.

## Dependências
- 007: ponto único de autenticação, chaves de identidade, níveis, recusas da inicialização.
- 002: spool e cliente de hook.
- `docs/sso.md`, `docs/users.md`, `docs/mcp-server.md` §4, `docs/security.md`.

## Checklist de Prontidão
- [x] Propósito claro e sem ambiguidade
- [x] Escopo delimitado (dentro/fora)
- [x] Cenários cobrem happy path e alternativos
- [x] Edge cases identificados com comportamento esperado
- [x] Constraints explícitas
- [x] Critérios de sucesso mensuráveis
- [x] Abordagem escolhida justificada
