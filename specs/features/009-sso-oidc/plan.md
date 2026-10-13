# plan.md: Plano de Implementação Técnica — SSO e OIDC (009)

> Plano gerado em 2026-10-13, antes de a 002 ser executada. Revisar quando as features anteriores terminarem.

## 1. Metadados do Plano
- **Stack Tecnológico:** a da 007, sem dependência nova. Cliente OAuth do fluxo de dispositivo escrito sobre `net/http` (são três chamadas e uma sondagem).
- **Feature Fonte:** `specs/features/009-sso-oidc/spec.md`
- **Escopo:** login por dispositivo, `auth.json`, renovação, ordem de bearer, `auth status` e `logout`, proxy de identidade confiável no servidor.
- **Fontes de contrato:** `docs/sso.md`, `docs/users.md` § "Trusted proxy identity" e § "Startup refusals", `docs/mcp-server.md` §4.2, `docs/cli.md` §2.4 e §5.2.

## 2. Design de Contratos e Fronteiras
- **Arquivo `auth.json`** no diretório de dados: objeto com a chave `oidc` → `{type: "oauth", access_token, refresh_token, expires_at, issuer, client_id, token_endpoint}`; outras chaves preservadas.
- **Configuração `[auth]`:** `actor_proxy_bearer_token`, `root_issuer`, `root_subject`.
- **Cabeçalhos aceitos com o bearer do proxy:** `X-Memory-Actor-User`, `-Issuer`, `-Sub`, `-Agent`, `-Client`, `-Session-Id`.

## 3. Decisões Arquiteturais e Justificativas

- **Decisão (S-01): cliente de dispositivo próprio sobre `net/http`.** Descoberta em `<issuer sem barra final>/.well-known/openid-configuration` (só `device_authorization_endpoint` e `token_endpoint`), autorização com `client_id` e o escopo do doc, sondagem com o intervalo do doc e os desfechos da tabela.
  - **Justificativa:** o contrato é pequeno e preciso; o doc diz que o cliente não lê o ID token nem decodifica o access token.
  - **Alternativa Rejeitada:** `golang.org/x/oauth2` (dependência a mais para três requisições, e o comportamento de sondagem do doc é específico).

- **Decisão (S-02): `auth.json` com escrita atômica.** Leitura e reescrita do arquivo inteiro preservando outros provedores, pela escrita atômica (temporário, fsync, rename), `0600` em Unix. O token renovado é salvo antes de usar; se não salvar, usa o antigo.
  - **Justificativa:** `docs/sso.md` § Store e § Refresh.
  - **Alternativa Rejeitada:** um arquivo por provedor (o doc fixa o arquivo compartilhado).

- **Decisão (S-03): resolução de bearer única no cliente.** Uma função com as duas ordens do doc (hook e CLI), usada pelo cliente HTTP da 002 e pela drenagem do spool; o item do spool guarda só o modo `oidc`, e a drenagem resolve e renova uma vez por envio.
  - **Justificativa:** `docs/sso.md` § "Where the token is used" e `docs/cli.md` §5.2.
  - **Alternativa Rejeitada:** guardar o token no item do spool (o doc grava só o modo).

- **Decisão (S-04): degrau do proxy no ponto único de autenticação.** Ordem: token vazio rejeitado; root; **proxy** (só com bearer explícito; cabeçalhos validados; nível e chave de identidade); credencial de banco; senão rejeitado. Cabeçalhos `X-Memory-Actor-*` ignorados fora desse degrau.
  - **Justificativa:** `docs/mcp-server.md` §4.2 e `docs/sso.md` § "How the server treats a bearer".
  - **Alternativa Rejeitada:** validar o token OIDC no servidor (o doc diz que o servidor não valida).

- **Decisão (S-05): contratos de teste.** IdP de teste em `httptest` cobrindo cada desfecho; ordem de bearer; renovação com e sem novo refresh token, recusada e não salva; proxy com cabeçalho repetido, vírgula, par parcial, sem identidade e root por par exato.
  - **Justificativa:** tabela de falhas do doc e adversariais do `mcp-server.md` §7.
  - **Alternativa Rejeitada:** testar contra um IdP real.

### O que não construir nesta feature
Validação de token, mapeamento de papéis, ligação com conta local, acesso por projeto para identidades do proxy, `mcp-bridge` (010), outros provedores.

## 4. Auditoria de Constituição

**Portões de Qualidade**
- [ ] Build cruzado, `go vet`, `go test -race` — S-05.
- [ ] Gherkin — os cenários da spec viram o `.feature` da 009.
- [ ] Dependência nova — nenhuma (S-01).

**Restrições Arquiteturais**
- [ ] INV-01 — `[auth]` e caminho de `auth.json` resolvidos por `internal/config`.
- [ ] INV-10 — `auth.json` com escrita atômica (S-02).
- [ ] Autorização por capacidade; tokens em tempo constante — S-04.
- [ ] INV-16 — chave de identidade do proxy como dono de handoffs e sessões (via 007).

**Anti-Padrões Proibidos**
- [ ] Segredos nunca em log nem na saída — S-02.
- [ ] Sem `panic` — falhas viram erros e status do doc.
