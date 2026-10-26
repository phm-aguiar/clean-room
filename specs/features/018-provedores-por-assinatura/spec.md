# Spec: Provedores por assinatura (anthropic-oauth, openai-oauth, codex, copilot)

## Metadados
- **ID:** 018
- **Status:** spec auditada contra o original (ver "Correções da auditoria" e os relatórios em `reports/`); sem plano nem tasks; em toda dúvida, reproduzir o original
- **Autor:** phm-aguiar
- **Data:** 2026-10-13
- **Feature Anterior:** 016 (interface de provedor, cadeia, saúde), 017 (interface de embedder), 009 (`auth.json`, comandos `auth`)
- **Roadmap:** `specs/features/roadmap.md`
- **Aprovado:** false

## Propósito
> Para quem, o quê e por quê.

Quem já paga uma assinatura (Claude Pro/Max, ChatGPT Plus/Pro, Codex, GitHub Copilot) quer usar os recursos de LLM sem comprar chave de API (`docs/design-hindsight-borrowings.md` P5). Esta feature acrescenta os provedores que se autenticam por token de assinatura ou login, sobre a fronteira da 016.

## Escopo

### Dentro do escopo
- **`anthropic-oauth`** (`docs/llm-client.md` §4.1 e §5.1, `llm-providers.md` §4.2): `ANTHROPIC_OAUTH_TOKEN` com alias `CLAUDE_CODE_OAUTH_TOKEN` (o canônico vence), bearer e `anthropic-beta: oauth-2025-04-20,claude-code-20250219`, sem cabeçalho de chave, base URL aceita, modelo padrão; o aviso de que o uso vai contra as políticas do fornecedor aparece na documentação e no `llm-test`.
- **`openai-oauth`** (§4.3 e §5.3, `llm-providers.md` §4.4): `ai-memory auth login openai-oauth` (espera de até 600 s, `--timeout-secs`), entrada `oauth` no `auth.json` da 009 com acesso, renovação e expiração em ms, renovação com 60 s de margem por refresh grant (timeout 300 s, refresh novo substitui, padrão 3600 s), Responses por stream SSE (`stream: true`, `store: false`, evento de conclusão obrigatório, erro como 502 sintético, stream truncado), cabeçalho `chatgpt-account-id`, identificador de sessão novo por requisição, `auth logout openai-oauth`.
- **`codex`** (§5.4, `llm-providers.md` §4.5): leitura de `tokens.access_token` e `tokens.account_id` de `$CODEX_HOME/auth.json` ou `.codex/auth.json` a cada chamada, nunca grava credencial; recuperação de 401 (reler; se igual, executar `codex app-server --stdio` serializado no processo com os limites de saída de 256 KiB por linha, 1 MiB no total, 64 KiB de stderr e o menor entre o timeout e 30 s; nova tentativa única); `AI_MEMORY_CODEX_EXECUTABLE`.
- **`copilot`** (§4.1, §4.3 e §5.7, `llm-providers.md` §4.7 e §7.5): ordem de credenciais (`GITHUB_COPILOT_API_TOKEN` direto; `COPILOT_GITHUB_TOKEN`, `GH_TOKEN`, `GITHUB_TOKEN`; login armazenado), troca por token de API de curta duração com cache e margem de 120 s, base por `COPILOT_API_URL` ou configuração ou token, escolha de Chat Completions ou Responses pelo catálogo de modelos (com cache na instância e recaída para Chat Completions), cabeçalhos de integração de editor sem o `User-Agent` padrão; `ai-memory auth login copilot` (fluxo de dispositivo do GitHub, `--client-id` ou `AI_MEMORY_COPILOT_CLIENT_ID`, `--github-token`); embedder `copilot` (`text-embedding-3-small`, 1536, timeout 300 s, retentativa para 429, truncamento).
- **Integração com a 016:** nomes e aliases (`anthropic_oauth`, `openai_oauth`, `github-copilot`, `github_copilot`), modelos padrão, perfis de `llm_fallbacks` sem `api_key_env` para esses provedores (compartilham a credencial do primário), `llm-test` com os quatro provedores.
- **Wrappers de contêiner** repassando as variáveis desses provedores aos comandos auxiliares (`llm-providers.md` §8), sobre os wrappers da 015.

### Fora do escopo (explicitamente)
- OIDC de dispositivo para hooks (feature 009).

## Comportamento Esperado

### Cenário Principal (Happy Path)
1. `ai-memory auth login copilot` conclui o fluxo de dispositivo e grava a credencial; com `AI_MEMORY_LLM_PROVIDER=copilot`, `llm-test --structured` responde.
2. Com `openai-oauth`, um token a 30 s de expirar é renovado antes da chamada; o novo refresh token substitui o anterior no `auth.json`.

### Cenários Alternativos
- **`codex` com 401 e arquivo inalterado depois da recuperação:** erro de autenticação orientando o login do Codex.
- **Renovação OAuth recusada:** erro de autenticação com o status e a orientação de novo login.
- **Copilot sem nenhuma credencial:** a inicialização falha orientando `auth login copilot` ou `COPILOT_GITHUB_TOKEN`.
- **Stream SSE sem evento de conclusão:** forma inesperada.

## Edge Cases
- Duas chamadas concorrentes com 401 no `codex`: só uma recuperação roda no processo.
- `auth.json` com entradas de outros provedores: preservadas na escrita (contrato da 009).
- Catálogo do Copilot fora do ar: Chat Completions.

## Constraints
- **INV-14:** credenciais resolvidas na borda; arquivos de token lidos pela configuração e entregues como material tipado; renovação gerida pelo próprio provedor.
- **Segurança:** token GitHub cru nunca vai ao host da API do Copilot; nada de token em log.
- **Tecnologia:** sem dependência nova.

## Critérios de Sucesso
- [ ] Cada provedor testado contra servidor falso (autenticação, renovação, stream, recuperação de 401)
- [ ] Recuperação do `codex` testada com executável falso que respeita e que estoura os limites
- [ ] `go vet ./...`, `go test -race ./...` e `golangci-lint run` passam sem apontamentos

## Abordagem Escolhida
Arquivos novos em `internal/llm` (um por provedor e um para o stream de Responses compartilhado por `openai-oauth`, `codex` e `copilot`) e o embedder `copilot` em `internal/embed`; comandos `auth login` estendidos no `cli` sobre o `auth.json` da 009.

## Decisões tomadas
- **Fluxo do `auth login openai-oauth`:** fluxo de dispositivo, como no original (resolvido na auditoria; ver "Correções da auditoria").

## Dependências
- 016, 017, 009, 015; `docs/llm-client.md` §4 e §5, `llm-providers.md` §4, §7.5 e §8, `install.md` § Anthropic via Claude subscription, § OpenAI OAuth, § Codex credential reuse, § GitHub Copilot.

## Checklist de Prontidão
- [x] Propósito claro e sem ambiguidade
- [x] Escopo delimitado (dentro/fora)
- [x] Cenários cobrem happy path e alternativos
- [x] Edge cases identificados com comportamento esperado
- [x] Constraints explícitas
- [x] Critérios de sucesso mensuráveis
- [x] Abordagem escolhida justificada

## Correções da auditoria

Auditoria de 2026-10-20/22 contra o comportamento original (o relatório dessa rodada não está mais em `reports/`).

- **Decisão em aberto resolvida:** o `auth login openai-oauth` usa **fluxo de dispositivo** (não navegador/PKCE): URLs `https://auth.openai.com/api/accounts/deviceauth/usercode` e `.../deviceauth/token`, página `https://auth.openai.com/codex/device`, troca em `https://auth.openai.com/oauth/token` com `redirect_uri=https://auth.openai.com/deviceauth/callback`, cliente `app_EMoamEEZ73f0CkXaXp7hrann`, polling `interval + 3 s` com 403/404 = pendente. Texto completo em `llm-client.md` §4.5. Não há decisão nossa a registrar.
- **`openai-oauth`:** endpoint `https://chatgpt.com/backend-api/codex/responses`; cabeçalhos fixos (`openai-beta: responses=experimental`, `originator: codex_cli_rs`, `session_id` UUID v4); `max_output_tokens` nunca enviado; `temperature` omitida para `gpt-5*`, `gpt-6*` e qualquer modelo iniciado por `o`; extração de `chatgpt-account-id` (3 locais do JWT); parser SSE detalhado; evento terminal = erro 502 sintético (transitório, aciona fallback).
- **`codex`:** protocolo JSON-RPC da recuperação (`initialize`, `initialized`, `account/read` com `refreshToken: true`), mensagens, trava por instância (chamadas concorrentes **esperam**), `CODEX_HOME` definido para a pasta do arquivo.
- **`copilot`:** constantes e cabeçalhos transcritos; troca de token em `GET https://api.github.com/copilot_internal/v2/token`; base derivada de `proxy-ep` (`proxy.` -> `api.`); catálogo `/models`: ausente = Chat Completions (guardado), HTTP não-2xx = Chat Completions (não guardado), falha de transporte propaga; Responses sem stream; `auth login copilot` aceita token do ambiente (`COPILOT_GITHUB_TOKEN`, `GH_TOKEN`, `GITHUB_TOKEN`) antes do fluxo de dispositivo (escopo `read:user`); embedder do Copilot tem 300 s e só repete 429.
- **`anthropic-oauth`:** dica extra do `llm-test` em 429; o "aviso de políticas" está nos docs de instalação, não no `llm-test`.
- **Novo provedor `cursor`** (CLI `agent`): ambiente em lista permitida, `--mode ask`, diretório temporário privado, limites 1 MiB/64 KiB, status 504 no timeout. Deve entrar no escopo desta feature (login pelo aplicativo do Cursor; sem comando `auth`).
- **`auth.json`:** chaves `openai`, `copilot`, `oidc` com campos exatos, escrita atômica `auth.json.tmp`, apagado quando vazio; `auth status` / `auth logout` com textos literais.
- Perfis de `llm_fallbacks` para provedores com credencial própria não usam `api_key_env` como credencial, mas uma variável nomeada e ausente ainda gera pendência.

### Status
**Precisa de ajustes**: incluir `cursor`, o protocolo de login por dispositivo e os detalhes acima; o item "Decisões tomadas" (protocolo do login) está resolvido pelos docs.
