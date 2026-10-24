# Cliente de LLM: provedores de chat, embeddings e reranking

> **Papel:** fronteira única de rede para modelos. Define as interfaces de chat, embedding e reranking; implementa um cliente nativo e tipado para cada provedor, sem gateway genérico; resolve a autenticação antes de construir o cliente; encadeia provedores com fallback e circuit breaker; registra a saúde passiva dos provedores; e oferece embeddings locais.
>
> Fora de testes, esta camada não lê variáveis de ambiente. A configuração resolve tudo e entrega o material já tipado.
>
> Tudo que depende de modelo é opcional. Sem provedor configurado, o sistema opera sem LLM.

---

## 1. Princípios de design

1. **Cliente nativo por provedor.** Cada provedor é falado no protocolo próprio, e as respostas são lidas em estruturas nomeadas. Não há camada genérica de tradução, para que divergências de protocolo apareçam como erro de desserialização ou de forma inesperada, e não como degradação silenciosa. Campos desconhecidos em respostas de provedor são ignorados; a única estrutura que recusa campos extras é o formato de julgamento do reranking (§9).
2. **Saída estruturada apenas por JSON Schema**, pelo mecanismo nativo de cada provedor (§5). A única exceção é o modo tolerante do provedor compatível, que extrai JSON do texto (§5.5).
3. **Sem retentativa automática no chat.** Uma chamada de chat não é repetida dentro do provedor; a troca de candidato acontece apenas no fallback (§6). A única retentativa interna é a dos embeddings, e só para HTTP 429 (§8.3).
4. **Autenticação resolvida antes da construção.** Os clientes recebem material de autenticação já resolvido e nunca leem o ambiente (§4).
5. **Segredos.** Chaves e tokens ficam em tipo de valor sensível. As representações de depuração redigem o conteúdo: credenciais aparecem como redigidas, e cabeçalhos extras mostram apenas os nomes.
6. **Limites de tamanho.** Resposta bem-sucedida: até 16 MiB. Corpo de erro: lido até 1 MiB e exibido com até 1 KiB (§3.3).
7. **Timeout por requisição de chat:** 300 s por padrão, tolerante ao carregamento a frio de modelos grandes em servidores locais. Pode ser sobrescrito por `AI_MEMORY_LLM_TIMEOUT_SECS`, lido uma vez pela configuração; o valor 0 é rejeitado.
8. **User-Agent padrão** `ai-memory/<versão>`, aplicado por baixo dos cabeçalhos do operador (`AI_MEMORY_LLM_HEADERS`). O Copilot é a exceção (§5.7). O provedor `cursor` não faz HTTP (§5.10).
9. **Redirecionamentos só dentro da mesma origem.** Todo cliente HTTP de provedor e de embedding segue redirecionamentos apenas quando o destino tem o mesmo esquema, host e porta da requisição anterior, com no máximo 10 saltos. Qualquer outro destino é recusado com a mensagem "refused a redirect to another origin (<origem>); set the endpoint to its final URL"; o excesso de saltos gera "too many redirects". Motivo: cabeçalhos de credencial próprios (`x-api-key`, `x-goog-api-key`, os do operador) não são removidos pelos clientes HTTP padrão ao trocar de host. Exceções deliberadas: o download do modelo local (§8.6) usa a política padrão do cliente HTTP, porque o repositório público redireciona para outro host; e os clientes de login e de renovação de credenciais (§4) não seguem nenhum redirecionamento.
10. **Endpoints que recebem credenciais exigem TLS.** Toda URL que recebe token de renovação, código de dispositivo ou token emitido (endpoints OAuth e OIDC) precisa ser `https`; `http` só é aceito para `localhost`, `127.0.0.1` e `::1`. Violação gera erro de autenticação: `<contexto> "<url>" must use https (plain http is accepted only on localhost, 127.0.0.1 or ::1)`; URL inválida gera `<contexto> "<url>" is not a valid URL: <motivo>`.
11. **Segredos em mensagens de erro.** O corpo de erro de provedor passa por redação de segredos e remoção de sequências de controle de terminal antes de ser cortado (§3.3).

---

## 2. Interfaces e tipos

### 2.1 Interface de chat

Um provedor de chat expõe:

- nome fixo do provedor e nome do modelo;
- completar texto, com ou sem id de operação (§2.2);
- completar com saída estruturada: recebe o JSON Schema e devolve o JSON produzido pelo modelo, sem validá-lo contra um tipo;
- estado de candidatos: lista vazia por padrão; só o fallback a preenche (§6).

Implementações que não expõem identidade de operação no protocolo usam o caminho comum e ignoram o id.

Há um wrapper tipado: ele deriva o JSON Schema de um tipo alvo, chama a variante estruturada e desserializa o resultado. Falha de desserialização vira erro de serialização (§3.1).

### 2.2 Id de operação

- Formato UUIDv7, gerado uma vez por **operação lógica** (por exemplo, uma consolidação ou uma revisão de auto-melhoria).
- É enviado em todas as tentativas HTTP da operação e em todos os candidatos do fallback, desde que o chamador use as variantes que recebem o id.
- Nunca é derivado de um id de sessão: duas operações da mesma sessão, ou uma operação retomada após falha, não podem parecer uma só para o provedor.
- Se o chamador não passa id, cada chamada gera um novo. Nesse caso, um fallback também gera um id novo para cada candidato.

### 2.3 Tipos neutros

- **Papel:** usuário ou assistente.
- **Mensagem:** papel e conteúdo.
- **Requisição:** prompt de sistema opcional, lista de mensagens, limite de tokens de saída e temperatura opcional. O construtor de prompt único usa limite de 1024 tokens e não define temperatura.
- **Resposta:** texto, uso opcional e o nome do modelo como o provedor o ecoou.
- **Uso:** tokens de entrada e tokens de saída.

### 2.4 Esforço de raciocínio

Valores aceitos: `none`, `minimal`, `low`, `medium`, `high`, `xhigh`, `max`, `ultra`, `persistent`. Um valor desconhecido na configuração é rejeitado no carregamento. Ausente significa padrão do modelo, e o campo é omitido na requisição.

| Destino | Campo enviado | Regra |
|---|---|---|
| Chat OpenAI e compatível genérico | `reasoning_effort` no topo | `ultra` e `persistent` viram `max` |
| OpenRouter (base com `openrouter.ai`) | objeto `reasoning` | effort como acima, com `exclude: true` (o raciocínio não entra no conteúdo) |
| xAI Grok (host da xAI) | `reasoning_effort` | só aceita `low`, `medium`, `high`, `xhigh`; `none` e `minimal` viram `low`; `max`, `ultra` e `persistent` viram `xhigh`; não é possível desligar |
| Anthropic | `output_config.effort` e `thinking` | ver §5.1 |
| Codex e Responses | `reasoning.effort` | mesma regra OpenAI |
| Gemini, Copilot e Cursor | — | ignoram o valor |

Detalhes que a tabela não mostra:

- **Detecção do destino** (só vale para o dialeto genérico `openai-compat`, que também é a rota de Chat Completions do `opencode`): a base contém `openrouter.ai` (sem diferenciar maiúsculas) para OpenRouter; a base contém `api.x.ai`, `://x.ai/` ou `.x.ai/` para xAI; qualquer outra base usa o formato OpenAI. O provedor `openai` oficial sempre usa o formato OpenAI, mesmo que a base seja trocada.
- **Mapeamento OpenAI:** `ultra` e `persistent` viram `max`; os demais valores passam como estão (inclusive `none`, `minimal` e `xhigh`).
- **Mapeamento xAI:** `none` e `minimal` viram `low`; `low`, `medium`, `high` e `xhigh` passam; `max`, `ultra` e `persistent` viram `xhigh`.
- **OpenRouter:** o objeto é `{"effort": <mapeamento OpenAI>, "exclude": true}`; o campo de topo `reasoning_effort` não é enviado.
- **Responses (`openai-oauth`, `codex`, `opencode` na rota Responses):** objeto `reasoning` com `effort` pelo mapeamento OpenAI. O `copilot` não envia nada, mesmo na rota Responses.
- Quando o esforço não está configurado, nenhum campo de raciocínio é enviado.

### 2.5 Cabeçalhos extras (`AI_MEMORY_LLM_HEADERS`)

- **Formato:** lista de entradas `Nome: Valor` ou `Nome=Valor`. Na configuração TOML (`llm_headers`) é uma lista de textos. Pela variável de ambiente, as entradas são separadas por vírgula (então, por esse caminho, um valor não pode conter vírgula; use o TOML nesse caso). Uma implementação pode aceitar também quebra de linha como separador de entradas na variável de ambiente, sem conflito com o contrato. Cada entrada é aparada; o separador é o primeiro `:` ou `=` encontrado (o que vier antes), então o valor pode conter o outro caractere; nome e valor também são aparados. Entradas vazias (após aparar) são ignoradas, mas continuam contando na numeração.
- **Erros na inicialização** (falha de configuração, fail-closed): entrada sem separador, nome de cabeçalho inválido (fora do conjunto de caracteres de "token" HTTP), valor inválido (caracteres de controle), ou nome reservado. A mensagem cita a posição da entrada (a partir de 1, contando as entradas vazias), nunca o valor. Textos exatos: `AI_MEMORY_LLM_HEADERS entry N is neither `Name: Value` nor `Name=Value``; `AI_MEMORY_LLM_HEADERS entry N: "<nome>" is not a valid HTTP header name`; `AI_MEMORY_LLM_HEADERS entry N: <nome> is set by ai-memory itself and cannot be overridden`; `AI_MEMORY_LLM_HEADERS entry N: the value for <nome> is not a valid HTTP header value`. Nomes são comparados sem diferenciar maiúsculas.
- **Cabeçalhos reservados:** `anthropic-beta`, `anthropic-version`, `authorization`, `content-length`, `content-type`, `host`, `openai-beta`, `x-api-key`, `x-goog-api-key`. O motivo é que o cliente HTTP acrescentaria o valor em vez de substituir, quebrando a requisição.
- **Sensibilidade:** os valores são marcados como sensíveis; a representação de depuração mostra só os nomes.
- **Precedência:** a aplicação substitui por nome, sem duplicar. Os padrões do sistema são aplicados apenas se o operador não definiu o cabeçalho; nunca sobrescrevem uma escolha do operador.
- **`x-request-id`:** é reservado para o provedor compatível, que o envia por conta própria em cada tentativa. Uma entrada estática com esse nome é recusada na inicialização quando o provedor compatível está configurado.

---

## 3. Erros

### 3.1 Categorias

| Categoria | Quando ocorre | Rótulo de classe |
|---|---|---|
| Transporte | falha de HTTP (timeout, conexão, leitura) | `http` |
| Provedor | status HTTP de erro, com status e corpo | `provider` |
| Serialização | falha de JSON na ida ou na volta | rótulo do formato de dados |
| Forma inesperada | resposta válida em JSON, mas sem o formato esperado | `unexpected-shape` |
| Não configurado | falta configuração, chave, URL ou arquivo | `not-configured` |
| Autenticação | token rejeitado, expirado ou falha de renovação | `auth` |
| Schema | não foi possível derivar ou transformar o schema | `schema` |
| Resposta truncada | resposta estruturada parou no limite de tokens (`finish_reason = length`) | `truncated-response` |
| Conteúdo vazio | resposta 2xx estruturada sem conteúdo de mensagem (ausente, vazio ou só espaços) | `empty-content` |
| Cadeia esgotada | nenhum candidato do fallback respondeu | `all-candidates-failed` |

Os rótulos de classe são estáveis e nunca contêm corpo de resposta nem segredo. O rótulo da categoria "Serialização" é, literalmente, `serde` (valor histórico que aparece em `last_error_class` do status; a reimplementação deve emiti-lo igual). A cadeia esgotada carrega o número de tentativas e um resumo.

**Textos de erro (formato de exibição, usados também em logs e na mensagem de saúde):**

| Categoria | Texto |
|---|---|
| Transporte | `http: <detalhe do cliente HTTP>` |
| Provedor | `provider error <status>: <corpo>` |
| Serialização | `serde: <detalhe>` |
| Forma inesperada | `unexpected response shape: <detalhe>` |
| Resposta truncada | `structured response truncated at token budget (finish_reason=length) for model <modelo>` |
| Conteúdo vazio | `provider returned no message content for model <modelo>` |
| Não configurado | `provider not configured: <detalhe>` |
| Autenticação | `auth: <detalhe>` |
| Schema | `schema: <detalhe>` |
| Cadeia esgotada | `llm fallback chain exhausted after <N> candidate(s): <resumo>` |

`truncated-response` e `empty-content` são **terminais e livres de conteúdo**: nunca carregam trecho da resposta, só o nome do modelo configurado e (internamente) a contagem de tokens de saída informada. Não são transitórios, e o modo estrito do `openai-compat` **não** recorre ao modo tolerante para eles (§5.5). O provedor `openai` oficial também os produz no caminho estruturado (§5.2).

### 3.2 Transitório ou determinístico

- **Transitório:** erro de provedor com status 429 ou entre 500 e 599 (inclui os códigos 52x de CDN); erro de transporte que é timeout ou falha de conexão.
- **Determinístico:** todo o resto, incluindo autenticação, schema, 4xx que não seja 429, desserialização, forma inesperada, não configurado e cadeia esgotada.
- **Efeito no fallback:** só erro transitório avança para o próximo candidato (§6). Erro determinístico é devolvido imediatamente, porque outro candidato não corrigiria a requisição.
- **Status HTTP:** é o status do erro de provedor; para erro de transporte, é o status quando ele existir; caso contrário, não há status.

### 3.3 Corpos de resposta

- **Corpo de erro:** lido até 1 MiB (1 048 576 bytes). Se passar desse limite, o corpo vira a mensagem literal `provider response exceeded 1048576 bytes`, não o início do corpo.
- **Exibição do corpo de erro:** ordem das operações: (1) ler até 1 MiB; (2) decodificar como UTF-8 com substituição de bytes inválidos; (3) **redigir segredos** com o sanitizador embutido (os mesmos padrões de segredo do sanitizador do produto, ver [prompts-schemas-and-sanitizer](prompts-schemas-and-sanitizer.md)) e remover sequências de controle de terminal (escape ESC, campainha etc.); (4) só então cortar em 1024 bytes sem quebrar caractere UTF-8 e acrescentar a reticência `…` (U+2026, 3 bytes) quando houve corte. Cortar depois de redigir garante que um segredo que cruza o limite de 1 KiB não sobreviva como prefixo irreconhecível. Se a leitura do corpo falhar, o texto é `<failed to read provider error body: <motivo>>`.
- **Resposta bem-sucedida acima de 16 MiB (16 777 216 bytes):** vira erro de provedor com o status recebido e o corpo `provider response exceeded 16777216 bytes`. Se esse status for de sucesso (2xx), o erro é determinístico. O limite vale também para o corpo inteiro de streams SSE (§5.3), que é lido por completo antes de ser interpretado.
- **Texto de resposta:** bytes inválidos viram caractere de substituição. **JSON de resposta:** é desserializado inteiro.

---

## 4. Autenticação

### 4.1 Requisito por provedor

| Provedor | Requisito | Origem |
|---|---|---|
| `anthropic` | chave de API obrigatória | `ANTHROPIC_API_KEY` |
| `anthropic-oauth` | token OAuth de assinatura obrigatório | `ANTHROPIC_OAUTH_TOKEN`, com alias `CLAUDE_CODE_OAUTH_TOKEN`; o nome canônico tem precedência; o token é gerado pelo comando de token do Claude |
| `openai` | chave de API obrigatória | `OPENAI_API_KEY` |
| `openai-oauth` | arquivo de token OAuth obrigatório | login do próprio sistema |
| `codex` | arquivo de autenticação do Codex CLI | diretório indicado por `CODEX_HOME`; executável em `AI_MEMORY_CODEX_EXECUTABLE` |
| `copilot` | token GitHub ou autenticação armazenada | `COPILOT_GITHUB_TOKEN`, depois `GH_TOKEN`, depois `GITHUB_TOKEN`; ou login do sistema |
| `gemini` | chave de API obrigatória | `GEMINI_API_KEY`, com alias `GOOGLE_API_KEY` |
| `openai-compat` | chave opcional; URL base obrigatória | `LLM_API_KEY`; URL em `AI_MEMORY_LLM_BASE_URL` ou `LLM_BASE_URL` |
| `opencode` | chave de API obrigatória | `OPENCODE_API_KEY` |
| `cursor` | CLI `agent` do Cursor já autenticado (nenhuma credencial é lida pelo ai-memory) | executável em `AI_MEMORY_CURSOR_AGENT` (padrão: `agent` no PATH) |

### 4.2 Fontes e precedência

1. Override explícito de chave pela linha de comando (por exemplo, no comando de teste de provedor, `--api-key`) tem precedência sobre a variável de ambiente; um valor vazio é ignorado.
2. Para Copilot, a ordem completa é: token de API direto (`GITHUB_COPILOT_API_TOKEN`) > token GitHub do ambiente (`COPILOT_GITHUB_TOKEN`, depois `GH_TOKEN`, depois `GITHUB_TOKEN`) > token GitHub armazenado no `auth.json` > token de API de curta duração em cache (§5.7).
3. Sem fonte disponível: a credencial é "não fornecida".
4. Toda variável de ambiente é lida **uma vez**, na carga da configuração; o valor é aparado e uma variável vazia ou só com espaços equivale a ausente. No Windows, nomes de variáveis são comparados sem diferenciar maiúsculas.

A origem usada fica registrada para diagnóstico, sem o valor. Credencial ausente gera erro de não configurado. Mensagens literais (o texto após `provider not configured: `):

| Situação | Mensagem |
|---|---|
| chave obrigatória ausente (`anthropic`, `openai`, `gemini`, `opencode`) ou `openai-compat` sem URL | apenas o nome da variável (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `GEMINI_API_KEY`, `OPENCODE_API_KEY`, `LLM_BASE_URL`) |
| `anthropic-oauth` sem token | `anthropic-oauth token missing; run `claude setup-token` and set ANTHROPIC_OAUTH_TOKEN (or CLAUDE_CODE_OAUTH_TOKEN)` |
| `openai-oauth` sem entrada em `auth.json` | `no openai-oauth token found at <caminho>; run `ai-memory auth login openai-oauth`` |
| `codex` sem arquivo | `codex auth file missing; run `codex login status` and authenticate Codex again` |
| `copilot` sem nenhuma fonte | `no copilot auth found at <caminho>; run `ai-memory auth login copilot` or set COPILOT_GITHUB_TOKEN` |
| `copilot` sem token GitHub no momento da troca | `copilot GitHub token missing; run `ai-memory auth login copilot` or set COPILOT_GITHUB_TOKEN` |
| `cursor` sem executável | `cursor agent executable missing; set AI_MEMORY_CURSOR_AGENT or install `agent`` |

Credencial do tipo errado (por exemplo, token onde se pediu chave) também gera erro de não configurado (`API key credential expected, got <tipo>` etc.); isso só ocorre por erro interno de montagem.

### 4.3 Tokens OAuth persistidos

- **Arquivo:** um único arquivo JSON compartilhado (`<data_dir>/auth.json`; o mesmo arquivo serve ao `openai-oauth`, ao `copilot` e ao login OIDC de dispositivo), um objeto de primeiro nível com uma chave por provedor. Cada entrada tem `"type": "oauth"`. Chaves e campos exatos:

  | Chave de 1º nível | Campos da entrada |
  |---|---|
  | `openai` | `type`, `access` (token de acesso), `refresh` (token de renovação), `expires` (milissegundos desde a época Unix, inteiro sem sinal), `accountId` (opcional; omitido quando não há) |
  | `copilot` | `type`, `githubAccess` (token GitHub, opcional), `githubExpires` (ms, opcional), `copilotAccess` (token de API de curta duração, opcional), `copilotExpires` (ms, opcional), `apiBaseUrl` (opcional) |
  | `oidc` | `type`, `access`, `refresh`, `expires` (ms), `issuer`, `client_id`, `token_endpoint` |

  Campos opcionais ausentes são **omitidos** (não gravados como `null`). O arquivo é gravado com indentação (JSON "pretty"), sem ordem de chaves garantida.
- **Leitura:** a ausência do arquivo equivale a "sem credencial". Arquivo ilegível gera erro de autenticação `read auth file <caminho>: <erro>`; JSON inválido gera `parse auth file <caminho>: <erro>`; entrada de formato errado gera `parse <chave> auth entry: <erro>`. Entradas cujo `type` não seja `"oauth"`, ou de outras chaves, são ignoradas.
- **Escrita:** lê o arquivo atual (se existir; se não for um objeto JSON, erro `auth file <caminho> must contain a JSON object`), substitui só a sua chave e preserva as demais, incluindo chaves desconhecidas. Cria o diretório pai se faltar. Se o objeto ficar vazio após remover uma chave (logout), o arquivo é **apagado**.
- **Escrita atômica:** grava um temporário ao lado do arquivo (nome do arquivo trocando a extensão por `json.tmp`, ou seja `auth.json.tmp`), sincroniza no disco (fsync) e renomeia sobre o destino. Em Unix, o temporário recebe permissão 0600 antes de receber bytes. No Windows não há ajuste de permissão (o arquivo herda a ACL do diretório de dados; recomenda-se manter o diretório de dados privado ao usuário), e a renomeação sobre um arquivo existente deve usar a substituição atômica do sistema (no Windows, a operação de mover com substituição); ela falha com "acesso negado" se outro processo mantiver o arquivo aberto sem compartilhamento, caso que deve ser reportado como erro de autenticação `rename auth file ...`.
- **Mensagens de erro de E/S:** `create auth dir <dir>: ...`, `open auth tmp <arq>: ...`, `write auth tmp <arq>: ...`, `fsync auth tmp <arq>: ...`, `rename auth file <arq>: ...`, `remove auth file <arq>: ...`.
- **Limite:** não há trava entre processos. Escritas concorrentes de processos distintos não são serializadas (a última vence, e pode apagar a renovação da outra). Dentro de um mesmo provedor/processo, a renovação é serializada por uma trava em memória que cobre também a gravação.
- **Renovação:** quando faltam 60 s ou menos para a expiração (ou ela já passou), o token é trocado por um refresh grant (POST de formulário `application/x-www-form-urlencoded` com `grant_type=refresh_token`, `refresh_token` e `client_id` do provedor), com timeout de 300 s. O endpoint precisa passar na regra de TLS (princípio 10). A resposta não segue redirecionamentos.
  - Se o servidor devolver um novo refresh token, ele substitui o anterior; se não devolver, o anterior é mantido.
  - A expiração vem de `expires_in` (segundos, convertido para ms somando ao instante atual); na renovação e no login OpenAI, o padrão quando ausente é 3600 s; no OIDC é 300 s.
  - Falha na renovação gera erro de autenticação `<rótulo> (<status>): <corpo>. <orientação>`, por exemplo `openai-oauth refresh failed (401 Unauthorized): <corpo>. Run `ai-memory auth login openai-oauth` again.` (para OIDC: `OIDC refresh failed (...): .... Run `ai-memory auth login oidc-device` again.`). `<status>` é o código com a frase-padrão (`401 Unauthorized`).
  - Token novo é gravado no arquivo **antes** de ser usado.
- **Cache de token Copilot:** o token de API de curta duração é reutilizado enquanto faltarem mais de 120 s para a expiração (a renovação ocorre quando `agora + 120 s >= expiração`).

### 4.4 OIDC device flow

Serve para que processos sem interação (hooks) enviem um JWT por desenvolvedor às rotas `/hook` e `/handoff` do servidor, em vez de um token estático compartilhado. O fluxo é:

1. **Descoberta:** exige que o emissor e a URL derivada passem na regra de TLS (princípio 10); lê `<emissor sem barra final>/.well-known/openid-configuration` (GET sem corpo) e obtém `device_authorization_endpoint` e `token_endpoint`, que também precisam ser `https` (ou loopback). Status de falha gera erro de autenticação `OIDC discovery failed (<status>) for <url>: <corpo>`.
2. **Código de dispositivo:** POST de formulário no endpoint de dispositivo com `client_id` e `scope` (padrão `openid profile offline_access`). Falha: `device authorization request failed (<status>): <corpo>`. Resposta: `device_code`, `user_code`, `verification_uri`, `verification_uri_complete` (opcional), `interval` (opcional, padrão 5) e `expires_in` (opcional, padrão 600).
3. **Polling:** POST de formulário no endpoint de token com `grant_type=urn:ietf:params:oauth:grant-type:device_code`, `device_code` e `client_id`. Cada resposta vira um resultado:
   - sucesso (2xx): token recebido (`access_token`, `refresh_token` opcional, `expires_in` opcional);
   - erro cujo corpo JSON tem `error`: `authorization_pending` (pendente, continuar); `slow_down` (aumentar o intervalo em 5 s); `access_denied` (negado); `expired_token` (código expirado); qualquer outro valor: resultado "outro" com o código;
   - corpo de erro que não é JSON ou sem `error`: resultado "outro" com o corpo (já redigido e cortado);
   - falha de transporte: erro (aborta o login).
4. **Renovação:** pelo refresh grant (§4.3), usando o `token_endpoint`, o `client_id` e o `issuer` guardados na entrada `oidc` (sem nova descoberta). Quando a resposta não traz refresh token novo, mantém o anterior. Sem nenhum refresh token disponível no login, erro de autenticação `OIDC token response carried no refresh_token; request the `offline_access` scope and enable it on the Keycloak client`. Falha gera erro de autenticação orientando `ai-memory auth login oidc-device`.

A validação do token acontece no servidor (que exige o papel `mcp:read` do usuário; ele é um papel de realm, não um escopo). Ver [SSO](sso.md). O comando `auth login oidc-device` é descrito em §4.5.

### 4.5 Fluxos de login (`ai-memory auth login/logout/status`)

Todos os fluxos usam um cliente HTTP próprio: timeout de 60 s por requisição, `User-Agent: ai-memory/<versão>`, **sem seguir nenhum redirecionamento** (um redirecionamento é devolvido como resposta e tratado como falha). Opções comuns de `login`: `--timeout-secs` (padrão 600) limita a espera pela autorização; `--github-token`, `--client-id` e `--issuer` só se aplicam a alguns provedores (combinação inválida é recusada: `--github-token / --client-id / --issuer do not apply to `auth login openai-oauth``; `--issuer applies only to `auth login oidc-device``; `--github-token applies only to `auth login copilot``; `--issuer is required for `auth login oidc-device``; `--client-id is required for `auth login oidc-device``).

**`auth login openai-oauth`** (fluxo de dispositivo do ChatGPT/Codex). Constantes: cliente público `app_EMoamEEZ73f0CkXaXp7hrann`; emissor `https://auth.openai.com`.

1. `POST https://auth.openai.com/api/accounts/deviceauth/usercode` com corpo JSON `{"client_id": "<cliente>"}`. Resposta: `device_auth_id`, `user_code`, `interval` (**texto**, em segundos). Falha: `openai-oauth device authorization failed (<status>): <corpo>`.
2. Imprime `Open this URL: https://auth.openai.com/codex/device`, `Enter code: <user_code>` e `Waiting for authorization...`.
3. Polling: espera `max(interval,1) + 3` s entre tentativas (se `interval` não for número, usa 5). A cada tentativa: `POST https://auth.openai.com/api/accounts/deviceauth/token` com JSON `{"device_auth_id": ..., "user_code": ...}`. Status 2xx: resposta com `authorization_code` e `code_verifier`. Status **403 ou 404**: ainda pendente, aguardar. Qualquer outro status: falha `openai-oauth authorization polling failed (<status>): <corpo>`. Ao ultrapassar o tempo total (`--timeout-secs`): `timed out waiting for openai-oauth authorization`. A primeira tentativa ocorre imediatamente.
4. Troca: `POST https://auth.openai.com/oauth/token` (formulário) com `grant_type=authorization_code`, `code=<authorization_code>`, `redirect_uri=https://auth.openai.com/deviceauth/callback`, `client_id=<cliente>`, `code_verifier=<code_verifier>`. Falha: `openai-oauth token exchange failed (<status>): <corpo>`. A resposta precisa ter `refresh_token` (senão `openai-oauth token response did not include refresh_token`); `expires_in` ausente vale 3600.
5. Grava a entrada `openai` (com `accountId` extraído dos tokens, §5.3) e imprime `openai-oauth: logged in`, `account: <id>` (se houver) e `token file: <caminho>`.
6. Endpoints de autorização e escopos "de navegador" existem apenas como constantes de referência (`https://auth.openai.com/oauth/authorize`; escopos `openid profile email offline_access`); o fluxo implementado é o de dispositivo acima, não o de navegador com PKCE.

**`auth login copilot`.** Ordem: (1) `--github-token` (não vazio) é gravado como está; (2) senão, o token GitHub do ambiente (`COPILOT_GITHUB_TOKEN`, `GH_TOKEN`, `GITHUB_TOKEN`) é gravado como está; (3) senão, fluxo de dispositivo do GitHub com cliente público `Iv1.b507a08c87ecfe98` (substituível por `--client-id` ou `AI_MEMORY_COPILOT_CLIENT_ID`):
1. `POST https://github.com/login/device/code` (formulário `client_id`, `scope=read:user`; cabeçalho `Accept: application/json`). Resposta: `device_code`, `user_code`, `verification_uri`, `expires_in`, `interval` (opcional, padrão 5). Falha: `copilot GitHub device authorization failed (<status>): <corpo>`.
2. Imprime `Open this URL: <verification_uri>`, `Enter code: <user_code>`, `Waiting for authorization...`.
3. Polling a cada `max(interval,1)` s (sem margem extra), até o menor entre `--timeout-secs` e `expires_in`: `POST https://github.com/login/oauth/access_token` (formulário `client_id`, `device_code`, `grant_type=urn:ietf:params:oauth:grant-type:device_code`; `Accept: application/json`). Status de falha: `copilot GitHub authorization polling failed (<status>): <corpo>`. Corpo com `access_token`: concluído. Senão, pelo campo `error`: `authorization_pending` (continua); `slow_down` (+5 s no intervalo); `expired_token` (`copilot GitHub device code expired`); `access_denied` (`copilot GitHub authorization denied`); outro: `copilot GitHub authorization failed: <error> <error_description>`; sem `error`: `copilot GitHub token response did not include access_token`. Tempo esgotado: `timed out waiting for copilot GitHub authorization`.
4. Grava a entrada `copilot` apenas com `githubAccess` (sem expiração) e imprime `copilot: logged in` e `token file: <caminho>`.

**`auth login oidc-device`** (`--issuer` e `--client-id` obrigatórios): descoberta e código de dispositivo (§4.4); imprime `Open this URL: <verification_uri_complete>` quando existir, senão `Open this URL: <verification_uri>` e `Enter code: <user_code>`; depois `Waiting for authorization...`. Polling: intervalo inicial `max(interval,1) + 3` s (padrão de `interval` 5), +5 s a cada `slow_down`; tempo total = menor entre `--timeout-secs` e `expires_in` (padrão 600); mensagens `timed out waiting for oidc-device authorization`, `oidc-device authorization denied`, `oidc-device code expired before authorization`, `oidc-device authorization failed: <código>`. Grava a entrada `oidc` e imprime `oidc-device: logged in`, `issuer: <emissor>`, `token file: <caminho>`.

**`auth logout <provedor>`** remove a chave correspondente (apagando o arquivo se ficar vazio) e imprime `<provedor>: logged out` e `token file: <caminho>`; sem entrada, é um sucesso silencioso.

**`auth status`** imprime, para cada um dos três blocos (sem mostrar segredos):
- `openai-oauth: logged in` + `account: <id>` (se houver) + `expires in: <tempo>` + `token file: <caminho>`; ou `openai-oauth: not logged in` + `token file: <caminho>`;
- `copilot: logged in` (há token GitHub renovável), ou `copilot: cached token valid (no GitHub token stored for refresh)`, ou `copilot: not logged in (cached token expired and no refreshable GitHub token stored)`; seguido de `github token expires in: ...`, `cached copilot token expires in: ...` e `api base: ...` quando houver; ou `copilot: not logged in` + `token file`;
- `oidc-device: logged in` + `issuer: ...` + `expires in: ...` + `token file: ...`; ou `oidc-device: not logged in` + `token file: ...`.
O tempo restante é `expired` quando já passou, `<h>h <m>m` com horas, ou `<m>m`.

---

## 5. Provedores de chat

Catálogo e exemplos de configuração estão em [llm-providers](llm-providers.md).

| Nome | Autenticação | Protocolo | Saída estruturada |
|---|---|---|---|
| `anthropic` | chave | Messages | ferramenta única com o schema, escolha forçada |
| `anthropic-oauth` | token de assinatura | Messages | igual ao anterior |
| `openai` | chave | Chat Completions | `json_schema` estrito, nome `Result` |
| `openai-oauth` | token OAuth | Responses via stream | `json_schema` estrito |
| `codex` | arquivo do Codex CLI | Responses via stream | `json_schema` estrito |
| `copilot` | token GitHub, depois token de API | Chat Completions ou Responses | `json_schema` estrito |
| `gemini` | chave | generateContent | `application/json` com `responseSchema` preparado |
| `openai-compat` | chave opcional | Chat Completions | modo estrito ou tolerante (§5.5) |
| `opencode` | chave | Responses, Messages ou Chat Completions, conforme o modelo | conforme o protocolo escolhido |
| `cursor` | CLI `agent` logado | processo filho, sem HTTP | esquema no prompt, resposta lida de forma tolerante |

### 5.1 Anthropic (`anthropic` e `anthropic-oauth`)

- **Endpoint:** `POST <base sem barras finais>/v1/messages`; base padrão `https://api.anthropic.com`. Cabeçalhos fixos: `anthropic-version: 2023-06-01` e `content-type: application/json`. Timeout por requisição (§1, item 7).
- **Autenticação:** chave vai no cabeçalho `x-api-key`. Token de assinatura vai em `Authorization: Bearer <token>`, junto com `anthropic-beta: oauth-2025-04-20,claude-code-20250219`, e **sem** `x-api-key` (enviar os dois faz a Anthropic recusar). Nota de risco: esse conjunto de cabeçalhos replica o handshake do Claude Code e pode mudar sem aviso; a combinação foi derivada, não validada contra o serviço ao vivo.
- **Corpo:** `model`, `max_tokens` (o pedido, sem teto do cliente), `system` (omitido quando ausente; texto simples), `messages` (`role` `user`/`assistant`, `content` texto simples), `temperature` (omitido quando ausente ou removido, ver abaixo), `tools`, `tool_choice`, `output_config` e `thinking` (omitidos quando não se aplicam).
- **Texto simples:** os blocos de texto da resposta são concatenados com `\n`; blocos de outros tipos (`thinking`, `redacted_thinking` etc.) são ignorados, sem falhar. Uso: tokens de entrada e saída.
- **Estruturado:** uma única ferramenta chamada `result`, com descrição `Emit the structured result.` e o schema como entrada.
  - Para a maioria dos modelos, a escolha da ferramenta é **forçada** (`tool_choice` do tipo `tool`, nome `result`). Exceção: os modelos que respondem 400 a escolha forçada mantêm a escolha automática, e a descrição da ferramenta passa a ser `Emit the structured result. Call this tool once instead of replying in text.` Modelos que rejeitam a escolha forçada: Opus e Sonnet a partir de 5.5; Fable e Mythos a partir de 5.1 (Haiku 5.5, Opus 5, Sonnet 5 e Fable 5 ainda aceitam).
  - A saída é a entrada do **primeiro** bloco de uso de ferramenta. Sem esse bloco: se a escolha era automática, os blocos de texto (unidos com `\n`) passam pelo mesmo tratamento do modo tolerante (§5.5: remover blocos de raciocínio e cerca de código, objeto inteiro ou primeiro objeto balanceado); se isso não render um objeto JSON, ou se a escolha era forçada, o erro é de forma inesperada `anthropic response had no tool_use block`.
- **Reconhecimento do modelo:** o nome é passado para minúsculas e procurado a partir da primeira ocorrência de `claude-` (então prefixos de Bedrock e Vertex funcionam); o restante é separado em segmentos por qualquer caractere não alfanumérico: `<família>`, `<maior>` e opcionalmente `<menor>`. Famílias: `opus`, `sonnet`, `haiku`, `fable`, `mythos`. Cada versão tem 1 ou 2 dígitos; um segmento maior (data como `20250514`) não conta como versão (o menor ausente vale 0). `claude-mythos-preview` (sem versão numérica) é o "Mythos Preview", tratado como atendendo a todos os pisos abaixo. Aliases sem versão (`claude-opus-latest`), identificadores legados com a família no fim (`claude-3-5-sonnet-...`) e nomes que não começam em uma família conhecida **não são reconhecidos**; nesse caso nenhum campo de esforço nem de temperatura é alterado e a escolha de ferramenta é forçada.
- **Temperatura:** removida para qualquer modelo reconhecido a partir de 4.7 (qualquer família) e para o Mythos Preview. Nos demais (inclusive não reconhecidos), é enviada se existir.
- **Piso de esforço** (`output_config.effort` e `thinking` só são enviados quando o modelo atinge o piso da família): Opus a partir de 4.5; Sonnet a partir de 4.6; Haiku a partir de **5.5** (o Haiku 4.5 devolve 400 para esses campos); Fable e Mythos a partir de 5.0. Abaixo do piso, nenhum dos dois campos é enviado, inclusive para `none`.
- **Thinking adaptativo** (`{"type":"adaptive"}`, enviado junto com `output_config` quando há esforço configurado diferente de `none`): Opus e Sonnet a partir de 4.6; Haiku a partir de 5.5; Fable e Mythos a partir de 5.0 (e o Mythos Preview).
- **Mapeamento de esforço** (para valores diferentes de `none`): `minimal` vira `low`; `ultra` e `persistent` viram `max`; os demais passam. Em seguida, o ajuste por modelo (clamp):
  - Opus 4.5: `xhigh` e `max` viram `high`.
  - Opus 4.6, Sonnet 4.6 e Mythos Preview: `xhigh` vira `max`.
- **Esforço `none`:** nunca envia `output_config`. Desativa o thinking conforme o modelo:
  - Sonnet a partir de 5.5 (que responde 400 a `disabled`): envia `thinking` `{"type":"between_tools"}` ("sem raciocínio estendido");
  - modelos que sempre pensam (Fable e Mythos a partir de 5.0, Mythos Preview, Opus a partir de 5.5): não envia nada;
  - demais modelos acima do piso: envia `thinking` `{"type":"disabled"}`;
  - modelos abaixo do piso ou não reconhecidos: não envia nada.

### 5.2 OpenAI (`openai`)

- **Endpoint:** `POST` em `<base>/chat/completions`, com `Authorization: Bearer <chave>` e `content-type: application/json`; base padrão `https://api.openai.com`. **Montagem da URL** (vale para todo cliente de estilo OpenAI: chat, embeddings, `opencode`, `openai-compat`): remover barras finais da base; se a base já termina em `/<endpoint>` (por exemplo `/chat/completions`), usar como está; senão, se o último segmento do caminho é uma versão (`v` seguido de 1 ou 2 dígitos, como `v1`, `v4`), anexar `/<endpoint>`; senão anexar `/v1/<endpoint>`. Exemplos: `https://api.openai.com` -> `.../v1/chat/completions`; `https://openrouter.ai/api/v1` -> `.../api/v1/chat/completions`; `http://localhost:11434/v1` -> `.../v1/chat/completions`; `https://api.z.ai/api/coding/paas/v4` -> `.../v4/chat/completions`.
- **Corpo:** `model`; `messages` (a mensagem de sistema, se houver, vai primeiro com `role: "system"`, seguida das mensagens na ordem); `max_tokens` **ou** `max_completion_tokens` (um só); `temperature` (omitido se ausente ou não suportado); `response_format` (só no estruturado); `reasoning_effort` ou `reasoning` (§2.4). Sem campo `stream` neste provedor.
- **Estruturado:** `response_format` `{"type":"json_schema","json_schema":{"name":"Result","schema":<schema normalizado>,"strict":true}}`. O schema é normalizado antes do envio, recursivamente (objetos e listas):
  - um nó com `$ref` mantém só a própria chave `$ref` (irmãos como `description` são descartados) e a recursão para ali;
  - `oneOf` vira `anyOf` (mesmo conteúdo);
  - todo nó com `"type":"object"` ou com `properties` recebe `additionalProperties: false`, sobrescrevendo qualquer valor anterior;
  - nesse mesmo nó, quando há `properties`, `required` passa a listar **todas** as chaves de `properties` (a ordem da lista não é significativa), sobrescrevendo a lista anterior; opcionalidade é expressa por tipo anulável (`["string","null"]`).
- **Famílias com `max_completion_tokens`:** modelos cujo nome (em minúsculas) começa com `gpt-5`, `gpt-6`, `o1`, `o3` ou `o4`. Para eles: usa `max_completion_tokens` em vez de `max_tokens`, sem limite de saída imposto pelo cliente, e a temperatura é omitida (só o valor padrão é aceito).
- **Demais modelos:** `max_tokens` = o menor entre o pedido e 16 384; a temperatura é enviada se existir. (Modelos como `gpt-4-turbo`, com teto real de 4096, continuarão recebendo 400 do serviço; não há tratamento especial.)
- **Resposta (texto simples):** conteúdo da primeira escolha; sem conteúdo (nulo), o texto é vazio. `usage`: `prompt_tokens` e `completion_tokens`.
- **Resposta (estruturada):** (1) se o `finish_reason` da primeira escolha (aparado, sem diferenciar maiúsculas) é `length`: erro `truncated-response`; (2) se o conteúdo é nulo, vazio ou só espaços: erro `empty-content`; (3) senão, o conteúdo é interpretado como JSON (falha: erro de serialização). Nenhum dos dois primeiros dispara nova tentativa.
- **Retentativa:** nenhuma no chat.

### 5.3 OpenAI OAuth (`openai-oauth`)

- **Endpoint:** `POST https://chatgpt.com/backend-api/codex/responses` (backend ChatGPT/Codex, não a API pública), com o token OAuth do arquivo (§4.3). Antes de cada chamada, o token é renovado se faltarem 60 s ou menos (a renovação é serializada por trava em memória e grava o arquivo antes de usar o token novo). Sem entrada `openai` no arquivo, o provedor nem é construído (erro de não configurado, §4.2).
- **Cabeçalho de conta:** `chatgpt-account-id`, com o id da conta, enviado só quando conhecido. O id é extraído **sem validar assinatura** de um JWT (segmento do meio, base64 URL-safe sem preenchimento, JSON), procurando nesta ordem: campo `chatgpt_account_id`; campo `chatgpt_account_id` dentro do objeto `https://api.openai.com/auth`; `id` do primeiro item da lista `organizations`. Fontes, em ordem: token de identidade (`id_token`), token de acesso, valor guardado anteriormente (na renovação). Se nenhum tiver, fica ausente.
- **Cabeçalhos fixos (nesta ordem de aplicação):** `Authorization: Bearer <acesso>`; `content-type: application/json`; `accept: text/event-stream`; `openai-beta: responses=experimental`; `originator: codex_cli_rs`; `session_id: <UUID v4 novo a cada requisição>`; depois os cabeçalhos do operador (que substituem por nome, exceto os reservados); `chatgpt-account-id`. O `User-Agent` padrão `ai-memory/<versão>` entra por baixo dos do operador.
- **Corpo:** `model`; `instructions` (o prompt de sistema; omitido se ausente); `input`: lista de `{"role": "user"|"assistant", "content": [{"type":"input_text","text": <texto>}]}` (inclusive para turnos do assistente); `store: false`; `stream: true`; `text` (só no estruturado); `reasoning: {"effort": ...}` (se configurado, §2.4); `temperature` (omitido quando o modelo começa, em minúsculas, com `gpt-5`, `gpt-6` ou com a letra `o`, ou quando ausente). O limite `max_output_tokens` **nunca é enviado**: o backend recusa esse campo para chamadores OAuth, então o limite de tokens do pedido é ignorado por este provedor e por `codex`.
- **Stream (SSE):** o corpo inteiro (até 16 MiB) é lido e depois interpretado. Linhas separadas por `\n` (um `\r` final é descartado); linha vazia fecha o evento; `event:` define o nome do evento (aparado); linhas `data:` (com espaços iniciais removidos) acumulam e são unidas por `\n`; outras linhas são ignoradas. O evento final sem linha vazia é fechado no fim do corpo. Para cada evento com dados (dados vazios ou `[DONE]` são ignorados): os dados são JSON; se o JSON tem a chave `error` no topo, é terminal; o tipo é o campo `type` do JSON, senão o nome do evento. Tipos:
  - `response.output_text.delta`: acumula o campo `delta`;
  - `response.completed`: guarda `response` (ou o próprio objeto, se não houver `response`) como a resposta final;
  - `response.failed`, `response.incomplete`, `response.cancelled`, `error`, ou JSON com chave `error`: erro de provedor com **status 502 sintético** e corpo igual ao texto dos dados cortado em 1024 bytes (com `…`; sem a redação de segredos do §3.3). Como 502 é transitório (§3.2), um fallback entra em ação;
  - demais tipos: ignorados.
  - Se o último evento termina com JSON inválido e nenhuma conclusão foi vista: forma inesperada `openai-oauth stream truncated before final event closed (incomplete JSON payload)`. JSON inválido em evento intermediário: erro de serialização.
  - Sem evento de conclusão: forma inesperada `openai-oauth stream closed before response.completed`. Se a resposta final não tem texto de saída e houve deltas, o texto dos deltas é usado.
- **Texto da resposta:** o campo de texto agregado da resposta final, se não vazio; senão o primeiro texto não vazio entre os itens de conteúdo da saída; senão vazio (no estruturado, vazio vira erro de serialização). `usage`: `input_tokens`, `output_tokens` (ausentes valem 0); modelo ecoado: o da resposta, senão o literal `openai-oauth` (ou `codex`).
- **Estruturado:** `text.format` `{"type":"json_schema","name":"Result","schema":<normalizado>,"strict":true}`, com a mesma normalização do OpenAI (§5.2). A saída é o texto da resposta interpretado como JSON.
- **Constantes de login/renovação:** cliente público `app_EMoamEEZ73f0CkXaXp7hrann`; endpoint de token `https://auth.openai.com/oauth/token`; fluxo de login em §4.5.

### 5.4 Codex (`codex`)

- **Arquivo:** reutiliza o arquivo de autenticação do Codex CLI: `$CODEX_HOME/auth.json` (variável não vazia), senão `.codex/auth.json` no diretório pessoal da plataforma (no Windows, `%USERPROFILE%\.codex\auth.json`). Lido a **cada chamada** (e na construção, que falha se o arquivo estiver inutilizável). Os campos `tokens.access_token` e `tokens.account_id` são obrigatórios; ambos são aparados e não podem ficar vazios. Outros campos são ignorados (refresh token e id token nunca são lidos). O ai-memory nunca grava nesse arquivo.
- **Erros de credencial** (todos de autenticação, com o sufixo `; run `codex login status` and authenticate Codex again if needed`): arquivo ilegível ou ausente: `Codex auth file is unavailable at <caminho> (file storage is required; keyring-only and ephemeral storage are unsupported)`; JSON/estrutura inválida: `Codex auth file is invalid at <caminho>`; campos vazios: `Codex auth file requires non-empty tokens.access_token and tokens.account_id`.
- **Transporte:** o mesmo de §5.3 (mesmo endpoint, cabeçalhos, corpo, SSE), com `chatgpt-account-id` sempre presente (vem do arquivo) e sem renovação própria.
- **Recuperação de 401** (só quando a **primeira** resposta é HTTP 401):
  1. relê o arquivo; se o token de acesso mudou em relação ao usado na tentativa, usa o novo e tenta uma vez;
  2. se não mudou, aguarda a trava de recuperação do provedor (uma por instância; chamadas concorrentes com 401 esperam, não disparam outro processo); depois relê de novo; se agora mudou (outra chamada recuperou), segue para a nova tentativa sem executar nada; se continua igual, executa a recuperação (abaixo) e relê;
  3. se após a recuperação o token ainda é o mesmo: erro de autenticação `Codex completed recovery without replacing the access token` (+ sufixo);
  4. nova tentativa única com o token novo; um segundo 401 gera `Codex authentication was rejected after one recovery attempt` (+ sufixo). Outros erros da nova tentativa são devolvidos como estão.
- **Processo de recuperação:** executa `<executável> app-server --stdio` (executável de `AI_MEMORY_CODEX_EXECUTABLE`, padrão `codex` resolvido no PATH; no Windows isso inclui resolução por `PATHEXT`, e um shim `.cmd` pode exigir interpretador de comandos), com stdin/stdout/stderr em pipes e a variável `CODEX_HOME` definida como a pasta do arquivo de autenticação; o processo é morto ao final em qualquer caso e aguardado (o original encerra só o processo direto; no Windows, com shim `.cmd`, convém encerrar também a árvore de filhos, como cuidado de implementação). Protocolo: JSON-RPC 2.0 em linhas JSON (uma mensagem por linha, terminada em `\n`):
  1. envia `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"ai-memory","version":"<versão>"},"capabilities":{}}}` e espera a resposta de `id` 1;
  2. envia a notificação `{"jsonrpc":"2.0","method":"initialized","params":{}}`;
  3. envia `{"jsonrpc":"2.0","id":2,"method":"account/read","params":{"refreshToken":true}}` e espera a resposta de `id` 2.
  - Ao esperar uma resposta, linhas sem campo `id` (notificações) são puladas; uma linha que não é JSON, um `id` diferente do esperado, a presença de `error` ou a ausência de `result` são falhas.
  - Mensagens de falha (todas autenticação, + sufixo): `Could not start `codex app-server --stdio` for recovery`; `Codex recovery process did not expose stdin/stdout/stderr`; `Could not write to Codex recovery process`; `Could not flush Codex recovery request`; `Codex recovery process ended before replying`; `Could not read Codex recovery output`; `Codex recovery returned invalid JSON`; `Codex recovery returned an unexpected JSON-RPC response id`; `Codex recovery returned a JSON-RPC error`; `Codex recovery response did not contain a result`; `Codex recovery output exceeded its limit`; `Codex recovery stderr exceeded its limit`; `Codex recovery process ended before completing` (o stderr fechou antes de o protocolo terminar); `Codex authentication recovery timed out`.
- **Limites do processo de recuperação:** cada linha de saída até 256 KiB (262 144 bytes); saída total até 1 MiB (1 048 576); stderr até 64 KiB (65 536; o excesso é erro; o conteúdo do stderr é descartado, nunca registrado); tempo total = o menor entre o timeout da requisição (padrão 300 s) e 30 s; a falha de qualquer um encerra a recuperação com erro de autenticação.

### 5.5 Compatível com OpenAI (`openai-compat`)

Cobre servidores locais e agregadores, como Ollama, vLLM, LM Studio, llama.cpp e OpenRouter.

- **Base URL:** obrigatória. Sem ela, erro de não configurado `LLM_BASE_URL` (fonte: `AI_MEMORY_LLM_BASE_URL`/`llm_base_url`, senão `LLM_BASE_URL`). Mesma montagem de URL do §5.2 (`/chat/completions`).
- **Chave:** opcional (`LLM_API_KEY`). Sem chave, a requisição leva `Authorization: Bearer dummy` (texto literal `dummy`); servidores locais costumam ignorá-lo.
- **Dialeto:** o de Chat Completions legado: sempre `max_tokens` com o valor pedido, nunca `max_completion_tokens`, temperatura enviada como veio, nenhum limite de saída imposto pelo cliente e nenhuma normalização por família de modelo. Se `AI_MEMORY_LLM_COMPAT_DISABLE_THINKING` (ou `llm_compat_disable_thinking = true`; padrão desligado) estiver ligado, todo pedido de chat leva `"chat_template_kwargs": {"enable_thinking": false}`, para motores locais (vLLM, SGLang) que, com modelos da classe Qwen3, gastariam o orçamento de saída num passo de raciocínio e truncariam o JSON. Só este provedor envia o campo (a API oficial da OpenAI recusa parâmetros desconhecidos).
- **Modo estrito** (padrão ligado; `AI_MEMORY_LLM_COMPAT_STRICT=false` desliga):
  1. normaliza o schema como em §5.2 (a normalização é feita por este provedor, pois o caminho interno do dialeto compatível não a aplica) e envia `response_format` `json_schema` estrito, nome `Result`;
  2. se o resultado for um objeto JSON, aceita;
  3. recorre ao modo tolerante, com uma segunda chamada, quando: o resultado é JSON mas não é objeto; ocorre erro de forma inesperada ou de serialização/desserialização; ou ocorre status 400 ou 422 cujo corpo (já redigido e cortado em 1 KiB) contém, sem diferenciar maiúsculas, `response_format`, `json_schema`, `structured output` ou `structured-output`;
  4. qualquer outro erro (transporte, 5xx, 429, auth, 4xx genérico, `truncated-response`, `empty-content`) é devolvido sem recorrer ao tolerante.
- **Modo tolerante:** chamada comum (a mesma requisição, sem `response_format`; a instrução para responder em JSON precisa estar no prompt do chamador). Em seguida:
  1. se o `finish_reason` da primeira escolha é `length` (aparado, sem diferenciar maiúsculas): erro `truncated-response`, com o modelo ecoado pelo servidor (terminal; nada de tentar de novo);
  2. se o texto está vazio ou só com espaços: erro `empty-content`;
  3. **limpeza do texto:** (a) remover todos os blocos pareados `<think>...</think>`, `<thinking>...</thinking>`, `<analysis>...</analysis>`, `<reasoning>...</reasoning>` (nomes de tag sem diferenciar maiúsculas; o conteúdo pode ter várias linhas; o par precisa ter a mesma tag de abertura e fechamento; correspondência mínima); (b) se sobrar uma tag de **fechamento** avulsa (`</think>`, `</thinking>`, `</analysis>` ou `</reasoning>`, sem diferenciar maiúsculas) — caso de modelos cujo template de chat já abre o bloco, como DeepSeek-R1, QwQ e Qwen3 em modo de raciocínio —, descartar tudo até o fim da **primeira** dessas tags, a menos que o texto antes dela, sem espaços iniciais, já comece com `{`, `[` ou `` ` `` (aí a tag faz parte da resposta e fica); (c) se o texto restante, como um todo, é uma cerca de código, isto é, casa com `^\s*```(json)?\s*\n(.*)\n\s*```\s*$` (o conteúdo interno pode ter várias linhas), usar o conteúdo interno aparado; senão usar o texto aparado;
  4. tentar interpretar o texto limpo inteiro como JSON; se resultar em objeto, aceitar;
  5. senão, extrair o primeiro objeto balanceado: começa na primeira `{`; percorre byte a byte contando `{` e `}`, ignorando chaves dentro de literais de string (`"..."`) e respeitando a barra invertida como escape; termina quando a profundidade volta a 0; interpretar o trecho como JSON (falha: erro de serialização);
  6. se não existir objeto balanceado: forma inesperada `openai-compat response did not contain a JSON object` (e o início e o fim do texto, 2000 bytes cada, vão ao log de depuração, nunca ao erro).
- **Custo:** o modo estrito seguido de fallback faz duas chamadas. Quem usa modelos de raciocínio que emitem blocos de pensamento deve desligar o modo estrito.
- **Cabeçalhos:**
  - `x-request-id` com o id de operação (UUID) em cada tentativa; estrito e tolerante compartilham o mesmo id; substitui qualquer valor do operador (que, por isso, é recusado na carga: `AI_MEMORY_LLM_HEADERS must not set x-request-id: the openai-compat provider sends it itself on every chat attempt with the logical operation id`);
  - quando a base contém `openrouter.ai` (sem diferenciar maiúsculas), são enviados `HTTP-Referer: https://github.com/akitaonrails/ai-memory` e `X-Title: ai-memory` (atribuição de uso no OpenRouter), com precedência do operador; o `User-Agent` padrão também entra por baixo dos do operador;
  - o `opencode` usa esta mesma lógica na rota de Chat Completions, mas **sem** `x-request-id` e com o modo estrito sempre desligado (§5.8).

### 5.6 Gemini (`gemini`)

- **Endpoint:** `POST <base sem barras finais>/v1beta/models/<modelo>:generateContent`, base padrão `https://generativelanguage.googleapis.com`. A chave vai no cabeçalho `x-goog-api-key` (mais `content-type: application/json`). Sem `User-Agent` especial além do padrão do operador/sistema.
- **Corpo:** `contents`: lista de `{"role": "user"|"model", "parts": [{"text": <texto>}]}` (o papel do assistente é `model`); `systemInstruction`: `{"parts":[{"text": <sistema>}]}` (omitido se ausente); `generationConfig`: `maxOutputTokens` (o pedido), `temperature` (se existir; enviado sempre que existir), `thinkingConfig` (ver abaixo, quando se aplica) e, no estruturado, `responseMimeType` e `responseSchema`.
- **Resposta:** texto = concatenação **sem separador** dos textos das partes do primeiro candidato; vazio vira "sem texto". Texto simples sem candidato devolve texto vazio; no estruturado, "sem texto" é forma inesperada `gemini response had no candidate text`, e o texto é interpretado como JSON. `usage`: `promptTokenCount` e `candidatesTokenCount` (ausentes valem 0). Modelo ecoado: `modelVersion`, senão o nome configurado.
- **Estruturado:** `responseMimeType: "application/json"`, mais o `responseSchema` preparado pelos passos abaixo, nesta ordem, sobre uma cópia do schema:
  1. **Definições:** `$defs` e `definitions` da **raiz** são extraídos (e removidos da raiz) para uma tabela de nomes. Cada `$ref` em qualquer lugar é substituída pela definição cujo nome é o último segmento do caminho da referência (`#/$defs/Foo` -> `Foo`), inline e recursivamente (uma definição pode referenciar outras). Referência sem definição: erro de schema `gemini: unresolved $ref <ref>`. Profundidade: a recursão conta 1 por nível de aninhamento (cada objeto ou lista dentro de outro) e 1 a mais por `$ref` resolvida; ao passar de **16** o erro é de schema `recursive $ref depth exceeded while preparing gemini schema`. Isso limita também schemas profundos sem recursão, não apenas ciclos.
  2. **Chaves removidas**, em **qualquer** objeto em qualquer profundidade: `$schema`, `$id`, `$comment`, `$defs`, `definitions`, `additionalProperties`, `unevaluatedProperties`, `allOf`, `oneOf`, `const`, `default`, `examples`, `patternProperties`, `readOnly`, `writeOnly`. `anyOf` é mantido. (Efeito colateral a preservar: uma propriedade de dados chamada, por exemplo, `default` ou `const` dentro de `properties` também é removida, pois a remoção é puramente por nome de chave.)
  3. **Tipos em lista** (`type` com lista): `["t","null"]` vira `type: "t"` mais `nullable: true`; lista só com `"null"` (ou vazia) remove `type` e mantém `nullable: true` se havia `null`; vários tipos não nulos viram `anyOf` de objetos `{"type": t}` e removem `type` (com `nullable: true` se havia `null`). Itens que não são texto são ignorados.
- **Desligamento de raciocínio:** `thinkingConfig: {"thinkingBudget": 0}` é enviado (em chamadas de texto e estruturadas) quando o nome do modelo, em minúsculas, **contém** `gemini-2.5-flash` (inclusive `-lite`, que aceita o campo), ou contém `gemini-3.5-flash` **sem** `-lite` (a variante lite responde 400 ao campo), ou contém `gemini-3.8-flash` **sem** `-lite` (variante lite não verificada, tratada como a 3.5). Para qualquer outro modelo o campo é omitido. Isso impede que tokens ocultos de raciocínio consumam a saída JSON.
- **Esforço de raciocínio:** ignorado.

### 5.7 Copilot (`copilot`)

Constantes: cliente OAuth público do GitHub Copilot `Iv1.b507a08c87ecfe98` (login, §4.5); troca de token em `GET https://api.github.com/copilot_internal/v2/token`; base padrão da API `https://api.githubcopilot.com`; identificador de integração `vscode-chat`; editor `vscode/1.107.0`; plugin `copilot-chat/0.35.0`; `User-Agent: GitHubCopilotChat/0.35.0`; versão da API do GitHub `2025-04-01`.

- **Autenticação (resolução do token de API, a cada chamada, sob trava em memória):**
  1. token de API **direto** (`GITHUB_COPILOT_API_TOKEN`): usado como está, sem troca, sem expiração e sem gravar nada; base = `COPILOT_API_URL`/URL configurada, senão derivada do token (abaixo), senão a padrão;
  2. senão, se há token de API em cache no `auth.json` (`copilotAccess`) com mais de 120 s de vida (`agora + 120 s < copilotExpires`): usa o cache; se `COPILOT_API_URL`/URL configurada existir, ela substitui a base guardada; base guardada ausente vale a padrão;
  3. senão, token GitHub (do ambiente — `COPILOT_GITHUB_TOKEN`, `GH_TOKEN`, `GITHUB_TOKEN`, nessa ordem — ou, na falta, o `githubAccess` do arquivo) é trocado: `GET` com `Authorization: Bearer <token GitHub>` e os cabeçalhos `accept: application/json`, `accept-encoding: identity`, `editor-version`, `editor-plugin-version`, `user-agent`, `x-github-api-version` e `copilot-integration-id` acima. Resposta `{"token": ..., "expires_at": ...}`; `expires_at` ≤ 100 000 000 000 é segundos Unix (multiplicado por 1000), acima disso já são ms. Falha de status: erro de **autenticação** `copilot token exchange failed (<status>): <corpo>`. O token novo, a expiração e a base resolvida são gravados em `copilot` do `auth.json` (preservando `githubAccess`). O **token GitHub cru nunca é enviado ao host da API do Copilot**, só ao endpoint de troca.
  - **Base derivada do token de API:** o token é uma lista de campos separados por `;`; o campo `proxy-ep=<valor>` (aparado, não vazio) dá o endpoint. Se o valor não começa com `http://` ou `https://`, ganha `https://`; o host é minúsculo e, se começa com `proxy.`, esse prefixo vira `api.`; a porta, se houver, é mantida; o resultado é `<esquema>://<host>[:<porta>]`. Sem `proxy-ep` ou valor inválido: base padrão.
  - Ordem de precedência da base: `COPILOT_API_URL` > URL base configurada (`llm_base_url`/`AI_MEMORY_LLM_BASE_URL` explícita, nunca o `LLM_BASE_URL` ambiente) > derivada do token > padrão. Em perfis de `llm_fallbacks` só `COPILOT_API_URL` vale.
  - A construção do provedor falha (não configurado) se não há token direto, nem token GitHub de ambiente, nem `githubAccess` no arquivo, nem token de API em cache válido.
- **Escolha do endpoint por modelo** (`GET <base>/models`, com `Authorization: Bearer <token de API>` e os cabeçalhos de runtime abaixo; os do operador se aplicam), feita na primeira chamada e guardada em memória por instância:
  - resposta com lista `data[]` de `{id, supported_endpoints[]}`; o item cujo `id` é igual (exato, com diferenciação de maiúsculas) ao modelo configurado decide;
  - se `supported_endpoints` contém `/chat/completions`, usa Chat Completions; senão, se contém `/responses`, usa Responses; se não contém nenhum, erro de forma inesperada `copilot model "<modelo>" has no supported completion endpoint; advertised endpoints: [...]`;
  - modelo ausente da lista (implantações corporativas, apelidos, modelo mais novo): usa Chat Completions e **guarda** essa decisão;
  - consulta com status de falha (não 2xx): usa Chat Completions **sem guardar** (tenta de novo na próxima chamada);
  - falha de transporte ou corpo ilegível na consulta: o erro é propagado (a chamada falha), não há recaída silenciosa.
- **Chat Completions:** `POST <base>/chat/completions`; corpo `model`, `messages` (sistema primeiro), `max_tokens` (o pedido, sem teto), `temperature` (omitido para modelos que começam, em minúsculas, com `gpt-5`, `gpt-6` ou com a letra `o`), `stream: false`, `response_format` (estruturado). Texto vazio ou ausente é erro de forma inesperada `copilot /chat/completions returned no text content` (estruturado: `... returned no structured content`) — diferente do `openai`, que devolve texto vazio. Modelo ecoado: o da resposta, senão o literal `copilot`.
- **Responses:** `POST <base>/responses`; corpo `model`, `instructions`, `input` (lista de `{"type":"message","role":..,"content":[{"type":"input_text","text":..}]}`), `max_output_tokens` (o pedido), `temperature` (mesma regra), `store: false`, `stream: false`, `text.format` `{"type":"json_schema","name":"Result","schema":..,"strict":true}` (estruturado). Resposta: se `status` existe e não é `completed`: forma inesperada `copilot /responses finished with status "<status>": <mensagem|código|no error detail>`; texto = `output_text` não vazio, senão o primeiro conteúdo `output_text` não vazio dos itens `message`; um conteúdo `refusal` gera `copilot /responses refused the request: <motivo|no refusal detail>`; sem texto: `copilot /responses returned no text content`.
- **Estruturado:** `json_schema` estrito, nome `Result`, com o schema normalizado (§5.2), nas duas rotas.
- **Cabeçalhos de runtime** (todas as chamadas à API do Copilot: chat, responses, models, embeddings): `accept: application/json`, `accept-encoding: identity`, `editor-version: vscode/1.107.0`, `editor-plugin-version: copilot-chat/0.35.0`, `user-agent: GitHubCopilotChat/0.35.0`, `content-type: application/json`, `copilot-integration-id: vscode-chat`, `openai-organization: github-copilot`, `x-initiator: user`. O Copilot **não** recebe o `User-Agent` padrão do ai-memory, porque a API exige o identificador de plugin de editor; um `User-Agent` do operador, se existir, o substitui.
- **Timeout:** o de chat (300 s) vale também para a troca de token e a consulta de modelos.
- **Esforço de raciocínio:** ignorado.

### 5.8 OpenCode (`opencode`)

- **Duas bases:** a padrão é o catálogo Go, `https://opencode.ai/zen/go/v1`. Se `AI_MEMORY_LLM_BASE_URL` (ou `llm_base_url`, ou `LLM_BASE_URL`) for informada, ela substitui a base; para o catálogo geral Zen, `https://opencode.ai/zen/v1`. O alias usado na configuração (`opencode`, `opencode-zen`, `opencode_zen`) não influi.
- **Escolha do protocolo pelo nome do modelo** (comparação exata, sem diferenciar maiúsculas):
  1. **Só se a base for do catálogo Go** — o caminho da URL, sem barra final e sem um sufixo `/chat/completions`, `/responses` ou `/messages`, termina em `/zen/go/v1` ou `/zen/go` —, listas específicas:
     - Responses: `muse-spark-1.3-contributor`, `muse-spark-1.2-contributor`;
     - Anthropic Messages: `minimax-m3`, `minimax-m2.7`, `qwen3.8-max`;
  2. lista geral para **Responses**: `gpt-6-astra`, `gpt-6-sol`, `gpt-6.1-sol`, `gpt-6-luna`, `gpt-5.6-sol`, `gpt-5.6-terra`, `gpt-5.6-luna`, `gpt-5.5`, `gpt-5.5-pro`, `gpt-5.4`, `gpt-5.4-pro`, `gpt-5.4-mini`, `gpt-5.4-nano`, `gpt-5.3-codex`, `gpt-5.3-codex-spark`, `gpt-5.2`, `gpt-5.2-codex`, `gpt-5.1`, `gpt-5.1-codex`, `gpt-5.1-codex-max`, `gpt-5.1-codex-mini`, `gpt-5`, `gpt-5-codex`, `gpt-5-nano`, `grok-4.7`, `grok-4.6`, `grok-4.5`, `grok-build-0.1`, `muse-spark-1.3`, `muse-spark-1.2`, `muse-spark-1.3-contributor-free`;
  3. lista geral para **Anthropic Messages**: `claude-fable-5-1`, `claude-fable-5`, `claude-opus-5-5`, `claude-opus-5`, `claude-opus-4-8`, `claude-opus-4-7`, `claude-opus-4-6`, `claude-opus-4-5`, `claude-sonnet-5`, `claude-sonnet-5-5`, `claude-sonnet-4-6`, `claude-sonnet-4-5`, `claude-sonnet-4`, `claude-haiku-4-5`, `qwen3.8-flash`, `qwen3.7-max`, `qwen3.7-plus`, `qwen3.6-plus`, `qwen3.5-plus`;
  4. todos os demais ids usam **Chat Completions**. As listas são exatas: um id novo que não esteja nelas cai em Chat Completions até a lista ser atualizada (ids como `mimo-v2.5` não têm entrada e usam Chat Completions).
  - A ordem é: se a base é Go, testa as listas Go (Responses, depois Anthropic); em seguida (e também quando a base não é Go) testa as listas gerais (Responses, depois Anthropic).
- **Chat Completions:** igual ao `openai-compat` com o modo estrito **sempre desligado** (tolerante, §5.5), sem `x-request-id`; chave como bearer.
- **Responses:** `POST` em `<base>/responses` (montagem do §5.2), `Authorization: Bearer <chave>`, **sem stream** (resposta JSON comum); corpo `model`, `instructions` (se houver), `input` (lista de `{"role", "content": [{"type":"input_text","text"}]}`), `max_output_tokens` (o pedido), `store: false`, `text.format` `json_schema` `Result` estrito com o schema normalizado (§5.2) no estruturado, `reasoning.effort` se configurado; **sem temperatura**. Texto = primeiro texto não vazio dos itens de conteúdo da saída; vazio no texto simples vira texto vazio; no estruturado, vazio vira erro de serialização.
- **Anthropic Messages:** o mesmo cliente do §5.1, com a **chave no cabeçalho `x-api-key`** (modo de chave, não bearer); base = a base configurada sem barras finais e sem o sufixo `/v1/messages` ou `/v1` (o cliente acrescenta `/v1/messages`). As regras de modelo Claude do §5.1 (temperatura, esforço) valem pelo nome do modelo; os modelos `qwen` não são reconhecidos como Claude e passam intactos.
- **Cabeçalhos nas três rotas:** `User-Agent: ai-memory/<versão>` e `x-opencode-session: <id de operação UUID>` como padrões (o operador pode sobrescrever ambos por `AI_MEMORY_LLM_HEADERS`). O OpenCode identifica clientes por esses dois cabeçalhos e sinaliza contas sem eles.
- **Estruturado:** segue o protocolo da rota escolhida.
- **Modelo padrão**, quando não informado: `mimo-v2.6-flash`. Ids são escritos sem prefixo (`mimo-v2.5`, não `opencode-go/mimo-v2.5`).
- **Chave:** `OPENCODE_API_KEY`, obrigatória; não há recaída para `LLM_API_KEY`.

### 5.9 Construção por provedor

Todo provedor recebe o timeout e os cabeçalhos extras (e o `User-Agent` padrão por baixo deles, exceto no Copilot e no Cursor). Além disso:

| Provedor | Itens específicos da construção |
|---|---|
| `anthropic`, `anthropic-oauth` | esforço de raciocínio; base URL aceita |
| `openai` | esforço de raciocínio; base URL aceita |
| `openai-oauth`, `codex` | esforço de raciocínio; arquivo de token ou de autenticação; **não** aceitam base URL (endpoint fixo) |
| `gemini` | base URL aceita; ignora esforço |
| `openai-compat` | base URL obrigatória; modo estrito; `x-request-id`; atribuição OpenRouter; opção de desligar thinking |
| `copilot` | sem User-Agent padrão; ignora esforço; base pela cadeia do §5.7 (a base da construção é ignorada) |
| `opencode` | base URL substitui o catálogo; esforço de raciocínio |
| `cursor` | só timeout; ignora base URL, esforço e cabeçalhos |

**Resolução da base URL (correção de um equívoco comum):** uma base **explícita do ai-memory** (`llm_base_url` no `config.toml` ou `AI_MEMORY_LLM_BASE_URL`) vale para todo provedor HTTP que aceita base (anthropic, anthropic-oauth, openai, gemini, openai-compat, opencode; e copilot só na cadeia do §5.7), porque nomear a base é o jeito de o operador dizer que quer um proxy do endpoint do fornecedor. Já o `LLM_BASE_URL` **sem prefixo** (convenção entre ferramentas, que o operador exporta uma vez para um Ollama local e esquece) só vale para `openai-compat` e `opencode`; para os demais é ignorado e o motivo vai ao log como aviso (`ignoring ambient LLM_BASE_URL: this provider talks to a fixed vendor endpoint. Set llm_base_url (or AI_MEMORY_LLM_BASE_URL) to point it somewhere else on purpose.`). `openai-oauth`, `codex` e `cursor` nunca aceitam base. Perfis de `llm_fallbacks` só usam o `base_url` do próprio perfil (não herdam nem a do primário nem a ambiente). O `llm-test` resolve a base como o servidor (`--base-url` tem precedência).

### 5.10 Cursor (`cursor`)

Provedor sem HTTP: chama o CLI `agent` do Cursor, já autenticado pelo aplicativo do Cursor (assinatura). O ai-memory não lê nem copia nenhuma credencial do Cursor. Modelo padrão: `cursor-grok-4.6-high` (o rótulo "Grok 4.6" na lista do `agent --list-models`); o valor especial `default` não passa `--model`.

- **Executável:** `AI_MEMORY_CURSOR_AGENT`, senão `agent` do PATH. Não é verificado na construção; falha na primeira chamada: erro de não configurado `cursor agent CLI (<caminho>) failed to start: <erro>. Install the Cursor agent or set AI_MEMORY_CURSOR_AGENT`. Em Unix, "arquivo de executável ocupado" (ETXTBSY) tenta até 5 vezes com 20 ms de intervalo (irrelevante no Windows).
- **Segurança:** o texto do prompt é sessão capturada, não confiável. Por isso o filho **nunca** herda o ambiente do servidor; recebe só a lista permitida (nomes comparados sem diferenciar maiúsculas): `PATH`, `HOME`, `USER`, `LOGNAME`, `SHELL`, `LANG`, `LANGUAGE`, `TZ`, `TERM`, `TMPDIR`, `TMP`, `TEMP`, `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_CACHE_HOME`, `XDG_STATE_HOME`, `XDG_RUNTIME_DIR`, `DBUS_SESSION_BUS_ADDRESS`, `HTTP_PROXY`, `HTTPS_PROXY`, `ALL_PROXY`, `NO_PROXY`, `SSL_CERT_FILE`, `SSL_CERT_DIR`, `NODE_EXTRA_CA_CERTS`, `USERPROFILE`, `APPDATA`, `LOCALAPPDATA`, `PROGRAMDATA`, `SYSTEMROOT`, `WINDIR`, `COMSPEC`, `PATHEXT`, e qualquer variável com prefixo `LC_` ou `CURSOR_` (inclui `CURSOR_API_KEY`). Valores de variáveis nunca vão a log. `--yolo`/`--force` nunca são passados.
- **Execução (cada chamada):** cria um diretório de trabalho privado `<temp absoluto>/ai-memory-cursor-<UUID v7>` (modo 0700 em Unix; se o diretório temporário não for absoluto, erro de não configurado `cursor workspace root must be an absolute path, got <caminho>`), grava nele `request.md` e apaga o diretório inteiro ao terminar (inclusive em erro). Linha de comando, nesta ordem: `--print --mode ask --output-format text --trust --workspace <diretório>` [`--model <modelo>` se o modelo não é `default`] e o argumento final fixo `Read request.md in this workspace and answer it. Reply with only the requested answer. Do not use tools.`; diretório corrente = o de trabalho; stdin vazio; stdout/stderr capturados e **drenados em paralelo**.
- **Conteúdo de `request.md`:** se há sistema: `# System\n<sistema>\n\n`; para cada mensagem `# User\n<texto>\n\n` ou `# Assistant\n<texto>\n\n`; no estruturado, ao final `# Output\nReturn one JSON value matching this schema and nothing else.\n<schema em JSON compacto>\n`. O esquema vai apenas no prompt (o CLI não tem modo de schema); a validação fica por conta do chamador.
- **Limites:** timeout = o de chat (300 s; `llm_timeout_secs`); estouro: erro de provedor com **status 504** e corpo `cursor agent timed out` (transitório). stdout até 1 MiB e stderr até 64 KiB (excesso: forma inesperada `cursor agent output exceeded the size cap`). Saída com código diferente de zero: erro de provedor com **status = código de saída** (se não couber em 16 bits sem sinal, 1) e corpo = o stderr (decodificação com substituição) cortado em 500 **caracteres** + `…`. stdout vazio após aparar: forma inesperada `cursor agent returned an empty answer`. Resposta: texto aparado; sem `usage`; modelo = o configurado.
- **Resposta estruturada (leitura tolerante):** (1) aparar e tirar uma cerca de código inicial (o texto começando por três crases: remove-se a linguagem e quebras de linha após as crases, e as crases finais); (2) tentar o texto inteiro como JSON; (3) senão, o trecho do primeiro `{` ou `[` até o último `}` ou `]` (se o fim vem depois do início); (4) senão, o primeiro objeto balanceado (§5.5); (5) senão, forma inesperada `cursor agent did not return JSON: <200 primeiros caracteres>`. Aceita qualquer JSON, inclusive não objeto.
- **Classificação:** como o código de saída vira "status HTTP", um código de saída 429 ou entre 500 e 599, e o 504 do timeout, são classificados como transitórios (podem acionar o fallback). Reproduzir o comportamento tal como descrito.

## 6. Fallback ordenado com circuit breaker

Uma cadeia tem um primário e uma lista de fallbacks, em ordem de declaração. Cada candidato guarda apenas rótulos (provedor e modelo) e o provedor já construído. Não guarda configuração nem credencial.

Algoritmo, usado de forma idêntica por chamadas textuais e estruturadas:

1. Contador de tentativas em zero; resumo vazio.
2. Para cada candidato, em ordem:
   1. Se o circuito do candidato está aberto (falha transitória há menos de 30 s), pula sem contar.
   2. Conta uma tentativa e chama o candidato com a mesma requisição, o mesmo schema e o mesmo id de operação.
   3. Sucesso: fecha o circuito, marca o candidato como o último selecionado e devolve a resposta.
   4. Falha: registra o status e a classe. Se a falha for transitória, abre o circuito por 30 s. Acrescenta ao resumo `provedor/modelo: classe status` (o status só aparece quando existe; separado da classe por um espaço, **sem colchetes**; o resumo é montado antes de o estado do candidato ser atualizado). Se a falha não for transitória, devolve o erro imediatamente.
3. Esgotou a lista: devolve erro de cadeia esgotada com o número de tentativas e o resumo, com as entradas separadas por `; `.

Se todos os circuitos estiverem abertos, o número de tentativas é zero e o resumo é vazio.

Regras complementares:

- O estado dos circuitos fica só em memória. Reiniciar o processo limpa tudo; não há cooldown compartilhado entre processos.
- O resumo contém apenas rótulos, classes e status, nunca corpo de resposta.
- A configuração monta a cadeia: primário primeiro, depois os fallbacks na ordem declarada.
- Um perfil de fallback com provedor ou modelo vazio, ou com provedor desconhecido, impede a inicialização. Nenhum perfil é descartado em silêncio.
- Para provedores que exigem chave (`anthropic`, `openai`, `gemini`, `opencode`), o perfil precisa de `api_key_env` (nome de variável de ambiente); a ausência do campo gera erro na carga: `llm_fallbacks[<i>] provider=<provedor> requires api_key_env (no native credential source for this provider)`. Variável nomeada mas ausente ou vazia **não** reprova a carga: o perfil fica pendente (`llm_fallbacks[<i>].api_key_env=<NOME> is set but the environment variable is missing or empty`), é descartado da cadeia e só o `serve` (e a montagem da cadeia) falha com essa mensagem. Isso vale para qualquer provedor que declare `api_key_env`, mesmo os que têm credencial própria. `openai-compat` aceita `api_key_env` opcional (chave opcional). `openai-oauth`, `codex`, `copilot`, `anthropic-oauth` e `cursor` usam a credencial do processo (arquivo, login, variável própria) e **nunca** a chave do primário.
- Mensagens da carga: `llm_fallbacks[<i>].provider must not be empty`; `llm_fallbacks[<i>].provider=<valor> is not one of anthropic|openai|gemini|openai-compat|openai-oauth|codex|copilot|anthropic-oauth|opencode|cursor`; `llm_fallbacks[<i>].model must not be empty`. `<i>` começa em 0. Cada perfil é construído (e descartado) na carga para validar `base_url`, forma da credencial etc. O primário, ao contrário, só é validado/construído quando o `serve` (ou outro comando que monte o provedor) o monta.
- Sem `llm_provider`, a lista de fallbacks é ignorada (mas a checagem de credencial pendente do `serve` continua valendo). O nome e o modelo que a cadeia expõe são os do primário.
- Chamadas sem id de operação: cada candidato gera seu próprio id; com id, o mesmo id vai a todos.
- Desenho completo em [llm-provider-fallback](llm-provider-fallback.md).

---

## 7. Saúde passiva

- O wrapper de saúde envolve os provedores de chat e de embedding em uso. Ele registra o resultado de cada chamada real e **não faz sondagem nem gera tráfego próprio**.
- Estado por papel (chat e embedding):
  - status: `disabled`, `unknown` (configurado, sem chamada no processo), `ok` ou `error`;
  - provedor, modelo e, para embedding, dimensão;
  - instantes da última chamada, do último sucesso e do último erro;
  - status HTTP e mensagem do último erro.
- **Mensagem de erro:** é o texto do erro, truncado a 1024 **caracteres** (não bytes), com `...` (três pontos ASCII) acrescentado quando há corte. Para erro de provedor, a mensagem é o corpo já redigido e cortado do §3.3; para os demais, o texto de exibição do §3.1. A última chamada define o estado: um sucesso põe `ok` e **apaga** instante, status e mensagem do último erro; um erro põe `error` e guarda instante, status (quando existe) e mensagem. O instante da última chamada, do último sucesso e do último erro usam o relógio do processo (UTC, formato RFC 3339).
- **Dica de nova tentativa:** para o papel de chat, o servidor monta o texto `ai-memory llm-test --provider <provedor> --model <modelo>[ --base-url <base>] --prompt ping` (provedor, modelo e base do **primário**), exibido apenas enquanto o status é `error`.
- **Papel de embedding:** é registrado por `embed`, `embed_document` e `embed_query`; a dimensão é a configurada. O rótulo de provedor e o modelo mostrados são os da configuração (`embedding_model`, o nome de fio), não a identidade com sufixo de prefixo.
- **Candidatos do fallback:** para cada candidato, na ordem de declaração, o estado traz: marca de último selecionado; instantes de sucesso e de erro; status HTTP e classe do último erro; e `circuit_open_until`, preenchido só enquanto o instante ainda está no futuro. Esses dados contêm apenas rótulos, status, classe e instantes, nunca corpo ou segredo.
- **Consumidores:** o status administrativo e o status da CLI.
- **Escopo:** o estado é do processo e se perde ao reiniciar.

---

## 8. Embeddings

### 8.1 Interface

Um embedder expõe:

- rótulo do provedor: `openai`, `voyage`, `google`, `openai-compat`, `local`, `copilot` ou `synthetic`;
- nome do modelo, exatamente como enviado;
- identidade do modelo para documentos (é o próprio nome, exceto quando há prefixo de documento);
- dimensão;
- `embed`, que devolve um vetor unitário; `embed_document` e `embed_query`, que por padrão chamam `embed` (o Google sobrescreve `embed` para ser igual a `embed_document`; `openai` e `openai-compat` aplicam prefixos nas duas variantes, §8.4).

Vetores unitários fazem do produto escalar a similaridade de cosseno. Vetores são sequências de `float32`; a armazenagem é de 4 bytes por valor em ordem little-endian (ver [vector-backend-policy](vector-backend-policy.md)).

**Consulta de busca:** toda superfície de busca transforma a consulta em vetor por `embed_query` (nunca `embed`), junto com a tripla (rótulo, **identidade do modelo**, dimensão) das linhas elegíveis; se o embedder falha, registra aviso (`embedder failed; degrading search to FTS5 + entity + graph`) e a busca segue sem o fluxo vetorial.

**Identidade:** cada vetor é gravado com a tupla (rótulo, identidade do modelo, dimensão). Vetores com identidade diferente da configurada não entram na busca. Essa regra é contrato do consumidor do armazenamento.

**Prefixo de documento** não vazio muda a identidade: o nome do modelo recebe o sufixo `+dp1-` seguido dos primeiros 16 caracteres hexadecimais (minúsculos) do SHA-256 dos bytes UTF-8 do prefixo (exemplo: `text-embedding-3-small+dp1-0123456789abcdef`). Assim, vetores gerados com prefixos diferentes não se misturam. O prefixo de consulta não altera a identidade, porque consultas não são armazenadas.

### 8.2 Escolha e dimensões

O provedor de embedding é escolhido por `AI_MEMORY_EMBEDDING_PROVIDER` / `embedding_provider` (aparado; **comparação exata, com diferenciação de maiúsculas**): `openai`, `voyage`, `google` (ou `gemini`), `openai-compat` (ou `openai_compat`), `local`, `copilot`; `none`, `off` ou `disabled` desligam; vazio/ausente significa "padrão local em melhor esforço" (§8.5). Outro valor: erro de não configurado `AI_MEMORY_EMBEDDING_PROVIDER=<valor> not one of openai|voyage|google|gemini|openai-compat|local|copilot|none`. O modelo vem de `AI_MEMORY_EMBEDDING_MODEL` (aparado); a dimensão de `AI_MEMORY_EMBEDDING_DIM`; a base de `AI_MEMORY_EMBEDDING_BASE_URL` (esta última **não** é afetada por `AI_MEMORY_LLM_BASE_URL` nem por `LLM_BASE_URL`).

| Escolha | Modelo padrão | Dimensão padrão | Observações |
|---|---|---|---|
| `openai` | `text-embedding-3-small` | 1536; 3072 só para `text-embedding-3-large`; qualquer outro modelo assume 1536 | base padrão `https://api.openai.com`, URL pela montagem do §5.2 (`/embeddings`); `Authorization: Bearer`; timeout total de 120 s por requisição; retentativa 429/503 (§8.3); truncamento; prefixos |
| `voyage` | `voyage-3` | 1024 (inclusive `voyage-3-large`) | base `https://api.voyageai.com` (`/v1/embeddings`); bearer; corpo `{"input": ["<texto>"], "model": ...}` (lista de um item); timeout 120 s; **sem retentativa, sem truncamento, sem prefixos**; resposta lida de `data[0].embedding` |
| `google` / `gemini` | `gemini-embedding-001` | 768 (qualquer modelo) | `POST <base>/v1beta/models/<modelo>:embedContent` (base padrão `https://generativelanguage.googleapis.com`); cabeçalho `x-goog-api-key`; o nome do modelo recebe `models/` se faltar (aparado) e o **nome com `models/` é o que se grava na identidade**; corpo `{"content":{"parts":[{"text":...}]},"taskType":...,"outputDimensionality":<dim>}`; timeout 120 s; retentativa só para 429; sem truncamento; resposta `embedding.values` |
| `openai-compat` | obrigatório | obrigatória | base URL obrigatória (`AI_MEMORY_EMBEDDING_BASE_URL`); chave opcional (vazia = sem cabeçalho de autorização); timeout 120 s; retentativa 429/503; truncamento; prefixos; rótulo gravado: `openai-compat` |
| `local` | `all-MiniLM-L6-v2` | 384 | sem chave; os valores de modelo e dimensão configurados **não** mudam o modelo local; diretório `<data_dir>/models/` |
| `copilot` | `text-embedding-3-small` | 1536 | reutiliza o login Copilot (§5.7); `POST <base do Copilot>/embeddings` com corpo `{"input": ..., "model": ...}` e os cabeçalhos de runtime do Copilot; timeout 300 s; retentativa só para 429; truncamento; sem prefixos; o formato do endpoint segue o contrato compatível com OpenAI e **não foi validado contra o serviço ao vivo** |
| `synthetic` | `bag-of-words-v1` | a pedida | apenas testes (§8.7) |

**Chaves de API** (resolvidas na carga, uma vez):

| Provedor | Fontes, em ordem | Ausência |
|---|---|---|
| `openai` | `EMBEDDING_API_KEY`; `OPENAI_API_KEY`; se `AI_MEMORY_EMBEDDING_BASE_URL` está definida, também `LLM_API_KEY` | `EMBEDDING_API_KEY, OPENAI_API_KEY or LLM_API_KEY required for openai-compatible embeddings` (com base customizada) ou `EMBEDDING_API_KEY or OPENAI_API_KEY` |
| `voyage` | `VOYAGE_API_KEY` | `VOYAGE_API_KEY` |
| `google` / `gemini` | `GEMINI_API_KEY`; `GOOGLE_API_KEY` | `GEMINI_API_KEY or GOOGLE_API_KEY` |
| `openai-compat` | `EMBEDDING_API_KEY`; `LLM_API_KEY`; senão sem chave | nunca falha |
| `local`, `copilot` | nenhuma | — |

**Regras de construção e validação** (textos literais após `provider not configured: `):

- dimensão zero (`AI_MEMORY_EMBEDDING_DIM=0`): `AI_MEMORY_EMBEDDING_DIM must be greater than zero`; dimensão omitida em `openai-compat`: `AI_MEMORY_EMBEDDING_DIM must be set explicitly for openai-compat (self-hosted model dims vary)`;
- `openai-compat` sem modelo: `AI_MEMORY_EMBEDDING_MODEL must be set explicitly for openai-compat (no safe default for self-hosted engines)`; sem URL: `AI_MEMORY_EMBEDDING_BASE_URL required for openai-compat embeddings`;
- `openai-compat` com chave vazia: vira "sem chave";
- `local` sem diretório de modelos: `local embeddings need the data dir's models/ root`;
- `copilot` sem autenticação Copilot: `copilot embedding provider requires Copilot auth` (e, antes, o erro de credencial do §4.2).

### 8.3 Retentativa

- **Embeddings `openai` e `openai-compat`:** respostas HTTP **429 e 503** são repetidas. A espera antes da tentativa `k` (`k` de 1 a 5) é o valor do cabeçalho `Retry-After` quando ele é um número inteiro de segundos (limitado a 32 s; a forma de data HTTP **não** é suportada e cai no padrão), senão `2^k` s (2, 4, 8, 16, 32). A chamada desiste (e devolve o erro do último status, com o corpo de erro padrão do §3.3) quando já houve 5 esperas, ou quando `espera acumulada + próxima espera` passaria de **62 s**. Exemplo: seis respostas 429 sem `Retry-After` esperam 2+4+8+16+32 = 62 s e a sexta vira erro de provedor 429. Com `Retry-After: 32` repetido, só a primeira espera cabe (32 + 32 > 62). Nenhum outro status (400, 5xx diferente de 503, etc.) é repetido; falhas de transporte e timeouts também não.
- **Embeddings Google e Copilot:** só **429**, esperas fixas `2^k` s para `k` de 1 a 5 (2, 4, 8, 16, 32), sem `Retry-After` e sem orçamento; após a quinta espera, o 429 vira erro de provedor. Espera total máxima: 62 s.
- **Voyage:** sem retentativa.
- **Chat:** nenhuma (§1, princípio 3).

### 8.4 Prefixos, truncamento e validação

- **Prefixos de consulta e de documento** (`embedding_query_prefix`, `embedding_document_prefix`; variáveis `AI_MEMORY_EMBEDDING_QUERY_PREFIX` e `AI_MEMORY_EMBEDDING_DOCUMENT_PREFIX`): aplicados só pelos embedders `openai` e `openai-compat`, e só nas variantes de documento e de consulta; a chamada `embed` simples **não** aplica prefixo. Ocorrem antes do truncamento, então o limite vale para o corpo inteiro, prefixo incluído. São strings exatas, **sem aparar** (o espaço final e quebras de linha embutidas são significativos); a variável de ambiente é lida crua (não aparada), e uma variável definida mas **vazia** apaga o prefixo definido no `config.toml`; variável ausente deixa o arquivo valer. Modelos conhecidos:
  - modelos E5 de base (`intfloat/e5-base-v2`, E5 multilíngue) e `nvidia/Nemotron-3-Embed-1B-BF16`: consulta `"query: "` e documento `"passage: "` (ambos com um espaço final);
  - `e5-mistral-7b-instruct`: só consulta, `"Instruct: {descrição da tarefa}\nQuery: "` (com um espaço após `Query:`; `\n` é uma quebra de linha real); documento sem prefixo;
  - Qwen3-Embedding: só consulta, `"Instruct: {descrição da tarefa}\nQuery:"` (**sem** espaço após `Query:`; o texto da consulta vem logo depois dos dois pontos); documento sem prefixo. `{descrição da tarefa}` é texto livre escolhido pelo operador;
  - mudar só o prefixo de consulta nunca exige reembedding; mudar o de documento troca a identidade (§8.1) e torna obsoletos os vetores antigos;
  - Google tem formato próprio (abaixo); Voyage, local e Copilot ignoram os prefixos.
- **Google, modelos cujo nome contém `embedding-2`:** não enviam `taskType`; o texto é reformatado: documento `title: none | text: <texto>`; consulta `task: search result | query: <texto>`. Para os demais modelos Google: `taskType` = `RETRIEVAL_DOCUMENT` (documento e `embed`) ou `RETRIEVAL_QUERY` (consulta), texto sem alteração. `outputDimensionality` é sempre enviado (a dimensão configurada).
- **Utilitário de truncamento:** o limite em bytes é o menor entre `max_tokens × 3` e 8000. Texto com tamanho em bytes acima do limite é cortado em `limite − 3` bytes, sem quebrar caractere UTF-8 (recua até a fronteira), e recebe `…` (U+2026, 3 bytes) no final. Se o limite for de 3 bytes ou menos, o resultado é vazio.
  - `openai`, `openai-compat` e `copilot` usam 5000 tokens, o que dá o teto de 8000 bytes;
  - `local` corta antes em 2048 × 3 = 6144 bytes, e depois em 512 tokens (§8.6);
  - `voyage` e `google` não truncam.
- **Parse de resposta no formato OpenAI** (`openai`, `openai-compat`, `copilot`), com status = o HTTP recebido (normalmente 200), nesta ordem:
  1. corpo que não é JSON: erro de provedor com corpo `openai embeddings json: <erro>; body=<corpo>` (cortado em 1024 bytes + `…`);
  2. campo `error`: erro de provedor com `error.message` (senão `error` como texto, senão `unknown`), cortado em 1024;
  3. `data` é lista: primeiro item ausente (lista vazia): erro de provedor `openai embeddings data[] empty; body=<corpo>` (corte 512); `embedding` não é lista: `missing embedding array in data[0]; body=<corpo>` (512); elemento não numérico: `non-numeric embedding value in data[0]; body=<corpo>` (512); lista de números não vazia: é o vetor;
  4. caso contrário (inclusive `embedding` vazio), tentativa de leitura estrita do formato `{data:[{embedding:[...]}]}`: falha de formato: erro de provedor `openai embeddings shape: <erro>; body=<corpo>` (1024); sem `data[0]`: forma inesperada `openai response had no data[0]; body=<corpo cortado em 512>`.
- **Dimensão:** vetor com dimensão diferente da configurada vira forma inesperada `expected dim <N>, got <M>` (vale para todos os embedders remotos, inclusive Voyage e Google).
- **Normalização:** vetor (32 bits) é dividido pela norma L2. Vetor de norma zero é devolvido sem divisão.

### 8.5 Embedder padrão e degradação

- **Sem provedor de embedding configurado:** o padrão é o embedder local em melhor esforço. Se o modelo não estiver presente, a inicialização do servidor dispara o download **em segundo plano** (tarefa assíncrona), segue **sem embedder** nessa execução e, se o download falhar, só registra aviso; a próxima inicialização encontra os arquivos e ativa os vetores. Se os arquivos estiverem presentes mas o carregamento falhar (checksum, formato), o sistema degrada para "sem embedder", com aviso, e continua iniciando.
- **`local` explícito:** se faltar arquivo, baixa **antes** de seguir (síncrono) e falha a inicialização se não conseguir (`fetching the local embedding model (set up offline per docs/local-embeddings.md if this host has no network)`); falha de carga também é fatal (`building embedder from config`).
- **Provedor remoto explícito:** qualquer erro de configuração ou construção interrompe a inicialização.
- **Divergência de triplas:** ao iniciar, se existem linhas de `page_embeddings` (versões mais recentes) cuja tripla difere da configurada, o servidor registra um aviso (`stored embeddings use a different (provider, model, dim) than configured; hybrid search ignores stale rows until pages are re-embedded — run `ai-memory embed --force` (or wait for scheduled backfill)`) e **continua** (bloquear impediria o `embed --force`, que é cliente HTTP do servidor).
- **Falha do embedder em tempo de consulta:** o chamador registra aviso e degrada para busca por texto, por entidade e por grafo (e observações, se ligado). Não retorna erro ao usuário.
- **Embedding na escrita:** com embedder ativo, toda escrita de página (inclusive reindexação de arquivo editado fora e restauração) embute o corpo e o `abstract:` do frontmatter na mesma execução, depois de liberar as travas; falha só gera aviso e registro em `page_embed_failures` (ver [wiki-files-layer](wiki-files-layer.md)).
- **Backfill:** ver [consolidation-llm](consolidation-llm.md) §12. Roda uma passada em segundo plano a cada início (com embedder) e, opcionalmente, periodicamente (`maintenance.embedding_backfill_interval_secs`, padrão 0 = desligado; ignorado com aviso se não há embedder).

### 8.6 Embedding local (`local`)

- **Modelo:** `all-MiniLM-L6-v2` (modelo público `sentence-transformers/all-MiniLM-L6-v2`), dimensão 384, até 512 tokens. Os pesos não vêm no binário.
- **Arquivos** (três, cada um com SHA-256 fixado em 2026-09-01; tabela de hashes em [local-embeddings](local-embeddings.md)): `model.safetensors` (pesos, cerca de 87 MB), `tokenizer.json`, `config.json`.
- **Diretório:** `<data_dir>/models/all-MiniLM-L6-v2/`.
- **Obtenção:** `GET https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/main/<arquivo>` para cada um, na ordem acima, com conexão de 30 s e tempo total de 600 s **por arquivo**; segue redirecionamentos (política padrão do cliente HTTP, pois o repositório redireciona a um CDN em outro host); status de falha é erro (`error_for_status`).
  - arquivo presente com hash correto é mantido (pula);
  - arquivo ausente, ilegível ou com hash errado é baixado e substituído;
  - o corpo inteiro é lido em memória, o SHA-256 é calculado, e só se bater é gravado num temporário (mesmo nome com a extensão trocada por `part`) e renomeado sobre o destino; se não bater: erro `downloaded <arquivo> sha256 <hash> does not match pin <fixado>; refusing to install an unverified model` e nada é instalado;
  - cria o diretório (e pais) se faltar; falha de E/S vira erro com o texto do sistema.
- **Sondagem barata:** a inicialização só checa a existência dos três arquivos (não os hashes).
- **Carregamento:** lê cada arquivo inteiro e verifica o hash.
  - arquivo ilegível/ausente: erro de não configurado `local embedding model file missing: <caminho>. Start the server once with network access to fetch it, or download https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/main/<arquivo> manually into <diretório>`;
  - hash diferente: erro de não configurado `local embedding model file <caminho> sha256 <hash> does not match the pinned <fixado>; delete it and re-fetch`;
  - depois: `config.json` é lido como configuração do encoder; `tokenizer.json` como tokenizador; falhas viram forma inesperada `parsing model config: ...`, `loading tokenizer: ...`, `loading model weights: ...`, `building BERT model: ...`.
- **Offline:** basta colocar os três arquivos no diretório.
- **Inferência** (CPU, uma página por vez, sem lotes, fora da thread de atendimento): (1) o texto é truncado a 6144 bytes (§8.4); (2) tokenização com os tokens especiais de início/fim (CLS/SEP) **sem** preenchimento (o `tokenizer.json` fixado traz preenchimento fixo de 128, que deve ser desligado: com ele a atenção olha o preenchimento e todas as similaridades ficam em torno de 0,8) e com truncamento a 512 tokens; (3) entradas: ids dos tokens, tipos de segmento todos zero e máscara de atenção real (1 por token); (4) passagem pelo encoder BERT de 6 camadas; (5) média dos vetores de saída por token ponderada pela máscara (soma dos vetores × máscara ÷ máx(1, soma da máscara)); (6) normalização L2. Norma exatamente zero ou não finita: forma inesperada `degenerate embedding norm`. A configuração esperada do modelo público é: tamanho oculto 384, 6 camadas, 12 cabeças, intermediário 1536, ativação GELU, vocabulário de 30 522, posições até 512, tipos de segmento 2, epsilon de normalização 1e-12 (conferir contra o `config.json` fixado; o ai-memory lê esses valores do arquivo, não os fixa em código).
- **Entrada:** texto que tokeniza para lista vazia é forma inesperada `empty tokenization`; falha do tokenizador: `tokenizing: ...`.
- **Rótulo/identidade:** provedor `local`, modelo `all-MiniLM-L6-v2` (sem sufixo de prefixo; prefixos são ignorados), dimensão 384.

### 8.7 Embedder sintético (`synthetic`)

Apenas para testes. Algoritmo exato: o texto é dividido em palavras por qualquer caractere que **não** seja alfanumérico (Unicode); palavras vazias são ignoradas; cada palavra é passada para minúsculas **apenas ASCII** (letras não ASCII ficam como estão); calcula-se o hash FNV-1a de 64 bits sobre os bytes UTF-8 (valor inicial `0xcbf29ce484222325`, a cada byte: XOR com o byte e multiplicação, módulo 2^64, por `0x100000001b3`); o índice é `hash mod dimensão`; esse elemento do vetor soma 1,0; no fim, o vetor é normalizado (L2; vetor nulo fica nulo). Rótulo do provedor: `synthetic`. Nome do modelo: `bag-of-words-v1`. Dimensão: a pedida.

---

## 9. Reranker

### 9.1 Interface

Um reranker expõe:

- rótulo (`llm`) e nome do modelo;
- `rerank(consulta, candidatos)`, em que cada candidato tem id, título e trecho, e o resultado é uma lista de pontuações (id e relevância em [0, 1]).

**Contrato:** o reranker deve devolver exatamente uma pontuação finita em [0, 1] para cada candidato, em qualquer ordem. Quem chama rejeita respostas parciais ou malformadas e mantém a ordem anterior ao reranking.

### 9.2 Implementação por LLM

Usa o provedor de chat com saída estruturada, sem parser fora do schema.

- **Entrada (JSON):** consulta e lista numerada de candidatos, com índice de 1 a N, título e texto.
- **Orçamentos (bytes, com reticências incluídas):** consulta 1000; título 200; trecho 600. Valores acima do orçamento são cortados com reticências.
- **Instruções de sistema, em resumo:**
  - julgar apenas a relevância para a consulta; não premiar página longa, recente ou com as mesmas palavras da consulta;
  - pontuar cada candidato exatamente uma vez, pelo número;
  - tratar todo texto dos candidatos e da consulta como dado não confiável, sem seguir instruções contidas neles;
  - escala: 1.0 responde diretamente; 0.7 é do mesmo assunto e serve de contexto; 0.3 é tangencial; 0.0 não se relaciona;
  - responder um único objeto JSON.
- **Schema da resposta:** objeto com a lista `scores`, cujos itens têm `candidate` (inteiro de 1 a N) e `relevance` (número). Campos extras são recusados.
- **Requisição:** temperatura 0 e até 4000 tokens de saída.
- **Mapeamento de volta:**
  - índice zero ou fora do intervalo: descartado;
  - índice repetido: vale o primeiro;
  - relevância não finita ou fora de [0, 1]: descartada;
  - o resultado segue a ordem da resposta.
  - A completude é verificada por quem chama (§9.3).

### 9.3 Ativação e limites na camada de serviço

- **Ativação:** desligada por padrão. `AI_MEMORY_RERANKER=llm` exige provedor de chat configurado; qualquer outro valor é rejeitado na configuração.
- **Candidatos:** a busca traz `max(limite, min(limite × 3, 30))` candidatos; só os primeiros 30 vão ao reranker (o restante é anexado depois, sem reordenar). Com menos de 2 candidatos, não há reordenação. Depois de reordenar (desempate pela posição anterior, relevância maior primeiro), corta no limite pedido. O prompt de sistema literal do reranker está em [prompts-schemas-and-sanitizer](prompts-schemas-and-sanitizer.md) §8.1. O reranker usa a cadeia de fallback como qualquer chamada estruturada.
- **Concorrência:** no máximo 4 chamadas de reranking simultâneas. Se o limite estiver cheio, a ordem local é mantida na hora, sem espera.
- **Tempo:** 20 s por chamada. Estouro mantém a ordem anterior.
- **Falha ou resposta inválida:** mantém a ordem anterior. Inválida significa: quantidade de pontuações diferente da de candidatos, id desconhecido, id repetido ou relevância fora de [0, 1] ou não finita. Só classe e status são registrados em log, nunca o corpo.

---

## 10. Notas para reimplementação

1. Comece com um provedor e saída estruturada por JSON Schema. Separe a interface de chat de um wrapper tipado que deriva o schema e desserializa.
2. Separe autenticação de cliente. Resolva credenciais na borda, uma vez, e nunca leia o ambiente dentro do cliente.
3. Classifique erros em transitórios (429, 5xx, timeout, falha de conexão) e determinísticos. Avance no fallback apenas em transitório. Um circuito de 30 s em memória basta.
4. Grave a tupla (provedor, identidade do modelo, dimensão) junto de cada vetor. Ignore vetores de outra configuração e trate falha de embedding como degradação, não como erro.
5. Para modelos locais sem modo estruturado confiável, mantenha o modo tolerante com extração do primeiro objeto balanceado.
6. Limite o tamanho de respostas e de corpos de erro. Redija segredos em qualquer representação de depuração.
7. **Windows (MVP):** (a) `auth.json` e os arquivos de modelo ficam no diretório de dados; a gravação atômica depende de mover com substituição, que falha com "acesso negado" quando outro processo tem o arquivo aberto sem compartilhamento (trate como erro de autenticação/E/S, sem apagar o original); não há `chmod 0600`, então mantenha o diretório de dados com ACL restrita ao usuário (ver [windows](windows.md)); (b) nomes de variáveis de ambiente não diferenciam maiúsculas; variáveis vazias valem como ausentes; (c) `codex` e `cursor` lançam executáveis externos: resolva `codex`/`agent` pelo PATH com as extensões de `PATHEXT`; um `.cmd` precisa do interpretador de comandos; encerre o processo e seus filhos ao fim ou no timeout; o filho do Cursor recebe só a lista permitida de variáveis, que já inclui `USERPROFILE`, `APPDATA`, `LOCALAPPDATA`, `PROGRAMDATA`, `SYSTEMROOT`, `WINDIR`, `COMSPEC` e `PATHEXT`; (d) o diretório temporário do `cursor` precisa ser um caminho absoluto (ex.: `%TEMP%`); (e) o caminho padrão do arquivo do Codex é `%USERPROFILE%\.codex\auth.json`; (f) o download do modelo local lê o arquivo de ~87 MB inteiro em memória; (g) o servidor local não bloqueia a requisição durante a inferência (uma página por vez, em tarefa separada).
8. **Equívocos já encontrados em revisões anteriores:** a base URL explícita vale para todos os provedores HTTP (§5.9); a retentativa de embedding também cobre 503 e `Retry-After` (§8.3); o Haiku só recebe campos de esforço a partir de 5.5 (§5.1); a escolha de ferramenta do Anthropic nem sempre é forçada (§5.1); o resumo da cadeia esgotada não usa colchetes (§6); o provedor `cursor` existe (§5.10); o login `openai-oauth` é por dispositivo, não por navegador (§4.5).
