# Provedores de modelo de linguagem

Este documento é o catálogo dos provedores de modelo de linguagem (LLM) aceitos pelo ai-memory: nome de configuração, credenciais, URL padrão, modelo padrão e casos especiais de cada um. O comportamento comum a todos os provedores (interface de chat, classes de erro, classificação de falhas transitórias, renovação de tokens, cadeia de fallback, embeddings e reranking) está em [cliente de LLM](llm-client.md) e não é repetido aqui.

## 1. Quando usar um provedor

O ai-memory funciona sem LLM. Sem provedor configurado, os hooks continuam capturando sessões, a busca combina texto completo (FTS5), entidades declaradas e vizinhos do grafo, e os resumos são gerados por regras.

Um provedor habilita:

- consolidação por LLM no checkpoint de compactação (PreCompact), sob demanda com `memory_consolidate` e, opcionalmente, no fim de sessão quando `AI_MEMORY_CONSOLIDATE_ON_SESSION_END` está ligada;
- lint mais rico e bootstrap;
- reranking opcional de `memory_query` (seção 6.4).

Sessões com conteúdo substantivo geram uma página de resumo por regras, com ou sem provedor. Sessões que têm apenas eventos de início e fim não geram página, handoff nem job de provedor. O handoff automático só é criado quando suas próprias condições valem, e a regra completa está em [ingestão de eventos de ciclo de vida](lifecycle-event-ingestion.md). A consolidação em si está em [consolidação](consolidation.md).

## 2. Seleção do provedor

### 2.1 Precedência da configuração

Os valores são lidos nesta ordem, do menos para o mais prioritário:

1. padrões internos;
2. `config.toml`, no diretório de dados ou no caminho passado pela linha de comando;
3. variáveis de ambiente `AI_MEMORY_*`.

Em `config.toml`, as chaves têm o nome da variável sem o prefixo: `llm_provider`, `llm_model`, `llm_base_url`, `llm_headers`, `llm_timeout_secs`, `llm_reasoning_effort`, `llm_compat_strict` e `llm_compat_disable_thinking`, mais a lista `[[llm_fallbacks]]` (só no TOML, ver [llm-provider-fallback](llm-provider-fallback.md)). Sem `llm_provider` (ou `AI_MEMORY_LLM_PROVIDER`), o LLM fica desligado. Padrões: `llm_compat_strict = true`; `llm_compat_disable_thinking = false`; `llm_timeout_secs = 300` (0 é rejeitado); `llm_reasoning_effort` ausente (o modelo decide). Valores de provedor são comparados exatamente (minúsculas), sem aparar além do que a leitura de variáveis já faz (variáveis são aparadas; vazio vale ausente). Nome desconhecido: `AI_MEMORY_LLM_PROVIDER=<valor> is not one of anthropic|openai|gemini|openai-compat|openai-oauth|codex|copilot|anthropic-oauth|opencode|cursor`.

### 2.2 Nomes aceitos

| Nome canônico | Aliases aceitos na configuração |
|---|---|
| `anthropic` | nenhum |
| `anthropic-oauth` | `anthropic_oauth` |
| `openai` | nenhum |
| `openai-oauth` | `openai_oauth` |
| `openai-compat` | `openai_compat` |
| `gemini` | `google` |
| `codex` | nenhum |
| `copilot` | `github-copilot`, `github_copilot` |
| `opencode` | `opencode-zen`, `opencode_zen` |
| `cursor` | nenhum |

Os aliases valem para `AI_MEMORY_LLM_PROVIDER`, para `llm_provider` e para os provedores de `llm_fallbacks`. O comando `llm-test` aceita somente o nome canônico. A escolha do endpoint do `opencode` depende da URL base, não do alias usado.

### 2.3 Modelos padrão

Quando `AI_MEMORY_LLM_MODEL` (ou `llm_model`) não é informado, cada provedor usa o modelo abaixo. Exceção: `openai-compat` exige o modelo explicitamente.

| Provedor | Modelo padrão | Quando usar |
|---|---|---|
| `anthropic` | `claude-haiku-4-5` | Escolha padrão para qualidade de consolidação e classificação de regras. |
| `anthropic-oauth` | `claude-sonnet-4-6` | Assinatura Claude Pro/Max, com token gerado por `claude setup-token`, sem chave de API. |
| `openai` | `gpt-5.4-mini` | Opção hospedada mais barata e mais rápida. |
| `openai-oauth` | `gpt-5.5` | Backend ChatGPT/Codex com login do próprio ai-memory, sem chave da plataforma de API. |
| `codex` | `gpt-5.6-luna` | Reutiliza o login já feito no Codex CLI. |
| `copilot` | `gpt-5.5` | Assinatura GitHub Copilot. |
| `gemini` | `gemini-3.5-flash` | Opção hospedada com camada gratuita generosa. |
| `opencode` | `mimo-v2.6-flash` | Assinatura OpenCode Go (padrão) ou pagamento por token (Zen). Defina `AI_MEMORY_LLM_MODEL` com um id que o endpoint escolhido atende. |
| `openai-compat` | sem padrão (obrigatório) | Servidores locais e agregadores compatíveis: OpenRouter, Atlas Cloud, OrcaRouter, Ollama, vLLM, LM Studio e outros. |
| `cursor` | `cursor-grok-4.6-high` | Assinatura Cursor, via o CLI `agent` já logado (§4.10). |

Para provedores de assinatura, um modelo pequeno e rápido reduz o consumo do limite de uso. O trabalho do ai-memory (consolidação, lint, exploração) é sumarização, não raciocínio difícil. Por isso:

- `anthropic-oauth`: defina `claude-haiku-4-5`.
- `openai-oauth` e `codex`: mantenha o modelo padrão. O backend do Codex/ChatGPT aceita um conjunto de ids definido pelo servidor e recusa os outros com erro 400 (comportamento do serviço externo; o ai-memory não valida o id e devolve o 400 como erro determinístico).
- `copilot`: um id da classe mini, como `gpt-5-mini`, pode funcionar, mas o conjunto de modelos aceitos pelo Copilot é definido pelo serviço (o ai-memory consulta o catálogo `/models` só para escolher o protocolo, §4.7, e não valida o id). Se o endpoint recusar a escolha, volte ao padrão.

### 2.4 Cabeçalhos extras e esforço de raciocínio

`AI_MEMORY_LLM_HEADERS` (`llm_headers`) aceita uma lista de cabeçalhos no formato `Nome: Valor` ou `Nome=Valor` (no TOML, lista de textos; na variável de ambiente, entradas separadas por vírgula, então o valor não pode conter vírgula), enviada a todos os provedores de chat HTTP (não ao `cursor`). Cabeçalhos que o próprio ai-memory define (`anthropic-beta`, `anthropic-version`, `authorization`, `content-length`, `content-type`, `host`, `openai-beta`, `x-api-key`, `x-goog-api-key`; e `x-request-id` com `openai-compat`), e entradas malformadas, interrompem a inicialização. Detalhes em [cliente de LLM](llm-client.md), seção 2.5.

`AI_MEMORY_LLM_REASONING_EFFORT` (`llm_reasoning_effort`) aceita um valor de esforço; valor inválido interrompe a carga. Os provedores que ignoram o esforço (`gemini`, `copilot`, `cursor`) e o mapeamento por destino estão em [cliente de LLM](llm-client.md), seções 2.4 e 5.9. Valores aceitos: `none`, `minimal`, `low`, `medium`, `high`, `xhigh`, `max`, `ultra`, `persistent`.

## 3. Catálogo resumido

| Provedor | Credencial | URL padrão | Base URL configurável |
|---|---|---|---|
| `anthropic` | `ANTHROPIC_API_KEY` | `https://api.anthropic.com` | Sim, só pela base explícita do ai-memory (`llm_base_url` / `AI_MEMORY_LLM_BASE_URL`); o `LLM_BASE_URL` ambiente é ignorado. |
| `anthropic-oauth` | `ANTHROPIC_OAUTH_TOKEN`, ou `CLAUDE_CODE_OAUTH_TOKEN` | `https://api.anthropic.com` | Sim, só pela base explícita do ai-memory. |
| `openai` | `OPENAI_API_KEY` | `https://api.openai.com` | Sim, só pela base explícita do ai-memory; o `LLM_BASE_URL` ambiente é ignorado. |
| `openai-oauth` | Login próprio, em `auth.json` no diretório de dados | Backend ChatGPT/Codex (`https://chatgpt.com/backend-api/codex/responses`) | Não. |
| `codex` | Arquivo de login do Codex CLI | Mesmo backend do `openai-oauth` | Não. O executável do Codex é configurável (`AI_MEMORY_CODEX_EXECUTABLE`). |
| `copilot` | Token GitHub do ambiente, login armazenado ou token de API direto | `https://api.githubcopilot.com`, ou o host indicado pelo token | Sim, por `COPILOT_API_URL` ou pela base URL (seção 4.7). |
| `gemini` | `GEMINI_API_KEY`, ou `GOOGLE_API_KEY` | `https://generativelanguage.googleapis.com` | Sim, só pela base explícita do ai-memory. |
| `opencode` | `OPENCODE_API_KEY` | `https://opencode.ai/zen/go/v1` (Go) | Sim (também pelo `LLM_BASE_URL` ambiente). Para Zen, use `https://opencode.ai/zen/v1`. |
| `openai-compat` | `LLM_API_KEY` (opcional) | Nenhuma | Obrigatória (explícita ou `LLM_BASE_URL` ambiente). |
| `cursor` | nenhuma (CLI `agent` logado) | não usa HTTP | Não. Executável por `AI_MEMORY_CURSOR_AGENT` (padrão `agent` no PATH). |

Regra da base URL (corrige uma versão anterior deste catálogo, que dizia que `anthropic` e `openai` ignoravam a base): uma base **explícita do ai-memory** (`llm_base_url` no `config.toml` ou `AI_MEMORY_LLM_BASE_URL`) é aplicada a todo provedor HTTP, inclusive `anthropic`, `anthropic-oauth`, `openai` e `gemini` (proxy deliberado do endpoint do fornecedor). O `LLM_BASE_URL` **sem prefixo** é uma convenção entre ferramentas e só vale para `openai-compat` e `opencode`; para os demais é ignorado com um aviso no log. `openai-oauth`, `codex` e `cursor` nunca aceitam base URL. O `copilot` usa `COPILOT_API_URL`, depois a base explícita do ai-memory (seção 4.7). Perfis de `llm_fallbacks` só usam o `base_url` do próprio perfil.

## 4. Detalhes por provedor

### 4.1 `anthropic`

- **Protocolo:** Messages, em `POST <base>/v1/messages`. A versão da API é fixa (`anthropic-version: 2023-06-01`).
- **Autenticação:** a chave de API vai no cabeçalho `x-api-key`.
- **Saída estruturada:** ferramenta única com o schema e escolha forçada. Detalhes em [cliente de LLM](llm-client.md), seção 5.1.
- **Temperatura:** ver seção 5.2.

### 4.2 `anthropic-oauth`

- **Token:** gerado uma vez por `claude setup-token`. Esse comando grava `CLAUDE_CODE_OAUTH_TOKEN`, que o ai-memory aceita. O nome canônico é `ANTHROPIC_OAUTH_TOKEN`; se os dois estiverem definidos, vale o canônico.
- **Sem chave de API:** `ANTHROPIC_API_KEY` não é necessária.
- **Autenticação:** o token vai como `Authorization: Bearer`, junto com `anthropic-beta: oauth-2025-04-20,claude-code-20250219`, e sem `x-api-key`. O mesmo endpoint de `anthropic` é usado. O `llm-test` acrescenta uma dica quando este provedor recebe um 429: a Anthropic costuma responder 429 a tokens `claude setup-token` usados por clientes que não são o Claude Code, e isso não é limite de taxa real (a dica manda usar `--provider anthropic` com chave de API ou um gateway compatível com OpenAI). Detalhes em [cliente de LLM](llm-client.md), seção 5.1.
- **Modelo recomendado:** `claude-haiku-4-5` (seção 2.3).

> **Aviso:** este método não é oficial e vai contra as políticas de uso da Anthropic. Use por sua conta e risco: a conta pode sofrer limitação de taxa ou banimento. Ver [instalação](install.md#anthropic-via-claude-subscription-oauth).

### 4.3 `openai`

- **Protocolo:** Chat Completions, na base do fornecedor.
- **Autenticação:** chave de API como bearer.
- **Saída estruturada:** `json_schema` estrito com nome `Result`. Detalhes em [cliente de LLM](llm-client.md), seção 5.2.
- **Limites de saída e temperatura:** os modelos cujo nome começa com `gpt-5`, `gpt-6`, `o1`, `o3` ou `o4` usam o campo de limite de tokens de conclusão (sem teto) e não recebem temperatura. Os demais modelos recebem `max_tokens` = o menor valor entre o pedido e 16 384 tokens.

### 4.4 `openai-oauth`

- **Login:** `ai-memory auth login openai-oauth` (fluxo de dispositivo: o comando mostra `Open this URL: https://auth.openai.com/codex/device` e um código, e aguarda). A espera pela autorização termina em 600 s por padrão (`--timeout-secs`). O protocolo completo (URLs, corpos, polling) está em [cliente de LLM](llm-client.md), seção 4.5; o fluxo de navegador com PKCE **não** é usado.
- **Armazenamento:** os tokens ficam na chave `openai` de `<data_dir>/auth.json` (acesso, renovação, expiração em ms e `accountId`). Esse arquivo é compartilhado com outros logins, e cada login tem a sua entrada. `auth logout openai-oauth` remove a chave; `auth status` mostra o estado sem segredos. Ver [cliente de LLM](llm-client.md), seção 4.3.
- **Endpoint:** o backend ChatGPT/Codex, não `api.openai.com`.
- **Identificação da conta:** o cabeçalho de conta vem do token de identidade ou do token de acesso. Ver [cliente de LLM](llm-client.md), seção 5.3.
- **Contêineres:** execute o login pelo wrapper de Docker. Assim o token cai no mesmo volume de dados (por padrão `ai-memory-data`) que o servidor usa.

### 4.5 `codex`

- **Credencial:** lê apenas `tokens.access_token` e `tokens.account_id` do arquivo de login do Codex CLI. Os dois campos são obrigatórios e não podem estar vazios.
- **Localização do arquivo:** `$CODEX_HOME/auth.json`. Se `CODEX_HOME` não estiver definido ou estiver vazio, usa `.codex/auth.json` no diretório home da plataforma (no Windows, `%USERPROFILE%\.codex\auth.json`).
- **Não copia credenciais:** o provedor nunca grava credenciais no diretório de dados do ai-memory e não lê refresh token nem ID token. Cada requisição relê o arquivo.
- **Modo de armazenamento:** só o arquivo é lido. Credenciais que existem apenas no armazenamento do sistema (keyring) ou só em memória não são lidas, e o erro indica que o arquivo está indisponível (`Codex auth file is unavailable at <caminho> (file storage is required; keyring-only and ephemeral storage are unsupported)`). O modo `auto` do Codex CLI funciona quando ele acaba gravando o mesmo `auth.json`; o ai-memory não consulta o modo configurado, só tenta ler o arquivo.
- **Recuperação de 401:** uma primeira resposta 401 pode disparar uma recuperação, que executa `codex app-server --stdio` (protocolo JSON-RPC em [cliente de LLM](llm-client.md), seção 5.4) e tenta de novo uma vez. Um segundo 401 gera erro de autenticação com a orientação de verificar o login do Codex. Detalhes e limites do processo em [cliente de LLM](llm-client.md), seção 5.4.
- **Executável:** `AI_MEMORY_CODEX_EXECUTABLE` define o caminho. O padrão é `codex` no PATH.
- **Independência:** `codex` não depende de `openai-oauth` e não usa o `auth.json` do ai-memory.
- **Contêineres:** o executável, `CODEX_HOME` e o arquivo de login precisam existir no mesmo ambiente do ai-memory. Nada disso é configurado automaticamente pelo wrapper de Docker.

Exemplo:

```bash
export AI_MEMORY_LLM_PROVIDER=codex
export AI_MEMORY_LLM_MODEL=gpt-5.6-luna
export AI_MEMORY_LLM_REASONING_EFFORT=medium
ai-memory llm-test --provider codex --model gpt-5.6-luna --prompt "Reply with OK"
ai-memory llm-test --provider codex --model gpt-5.6-luna --structured --prompt "Return a short answer"
```

### 4.6 `gemini`

- **Chave:** `GEMINI_API_KEY`. Se ausente, usa `GOOGLE_API_KEY`. A chave vai no cabeçalho próprio da API do Google.
- **Base URL:** só a base informada explicitamente é aplicada. O `LLM_BASE_URL` do ambiente compartilhado não se aplica a este provedor.
- **Saída estruturada:** schema preparado e modo de resposta JSON. Detalhes em [cliente de LLM](llm-client.md), seção 5.6.
- **Esforço de raciocínio:** ignorado.

### 4.7 `copilot`

- **Login:** `ai-memory auth login copilot`. Ordem: `--github-token`; senão o token GitHub do ambiente (`COPILOT_GITHUB_TOKEN`, `GH_TOKEN`, `GITHUB_TOKEN`); senão o fluxo de dispositivo do GitHub (cliente público `Iv1.b507a08c87ecfe98`, escopo `read:user`, substituível por `--client-id` / `AI_MEMORY_COPILOT_CLIENT_ID`). Protocolo em [cliente de LLM](llm-client.md), seção 4.5.
- **Ordem das credenciais:**
  1. token de API direto em `GITHUB_COPILOT_API_TOKEN`, usado como está, sem troca e sem gravação;
  2. token GitHub do ambiente, na ordem `COPILOT_GITHUB_TOKEN`, `GH_TOKEN`, `GITHUB_TOKEN`;
  3. token GitHub armazenado no login.
- **Troca de token:** o token GitHub é trocado por um token de API de curta duração. Esse token é gravado no arquivo compartilhado e reutilizado enquanto faltarem mais de 120 s para a expiração. Ver [cliente de LLM](llm-client.md), seção 4.3.
- **Sem credencial:** a inicialização falha e orienta a fazer o login ou definir `COPILOT_GITHUB_TOKEN`.
- **Base URL:** `COPILOT_API_URL` tem precedência. Sem ela, usa `llm_base_url` (ou `AI_MEMORY_LLM_BASE_URL`; nunca o `LLM_BASE_URL` ambiente). Sem nenhuma das duas, o host indicado pelo campo `proxy-ep` do próprio token de API (com `proxy.` trocado por `api.`) ou, na falta, `https://api.githubcopilot.com`. O argumento `--base-url` do `llm-test` não afeta o Copilot.
- **Endpoint por modelo:** Chat Completions ou Responses, conforme o modelo anuncia no catálogo do Copilot. Detalhes em [cliente de LLM](llm-client.md), seção 5.7.
- **Cabeçalhos:** identificação de integração de editor (`copilot-integration-id: vscode-chat`, `editor-version: vscode/1.107.0`, `editor-plugin-version: copilot-chat/0.35.0`, `user-agent: GitHubCopilotChat/0.35.0` e os demais do cliente de LLM, seção 5.7), sem o User-Agent padrão do ai-memory.
- **Esforço de raciocínio:** ignorado.

### 4.8 `opencode`

- **Chave:** `OPENCODE_API_KEY`, obrigatória. Não há fallback para `LLM_API_KEY`.
- **Endpoints:**
  - padrão: catálogo **Go** (`https://opencode.ai/zen/go/v1`), assinatura com franquia mensal por modelo;
  - Zen: defina `AI_MEMORY_LLM_BASE_URL=https://opencode.ai/zen/v1` (ou `llm_base_url`), para o catálogo geral com cobrança por token.
- **Ids de modelo:** escritos sem prefixo, por exemplo `mimo-v2.5` ou `glm-5.3-flash`. A forma `opencode-go/<modelo>` usada pelo cliente oficial do OpenCode não é tratada pelo ai-memory (não há remoção de prefixo): o id vai como configurado ao endpoint, que decide se o aceita (não verificado contra o serviço).
- **Escolha do protocolo pelo nome do modelo:** a comparação é exata e não diferencia maiúsculas, contra listas fixas (transcritas por inteiro em [cliente de LLM](llm-client.md), seção 5.8). Resumo da ordem: (1) só se a base for do catálogo Go (caminho terminando em `/zen/go/v1` ou `/zen/go`), `muse-spark-1.3-contributor` e `muse-spark-1.2-contributor` usam Responses, e `minimax-m3`, `minimax-m2.7` e `qwen3.8-max` usam Anthropic Messages; (2) lista geral de Responses: 31 ids das famílias `gpt-5*`/`gpt-6*`, `grok-*` e `muse-spark-*`; (3) lista geral de Anthropic Messages: 14 ids `claude-*` e 5 ids `qwen3.*`; (4) qualquer outro id usa Chat Completions.

  Um modelo novo precisa de inclusão nessas listas para usar outro protocolo. Enquanto isso, ele cai em Chat Completions. Observação: com a base apontando para Zen (não Go), os ids específicos de Go (por exemplo `minimax-m3`) caem em Chat Completions.
- **Base do protocolo Anthropic:** a base é usada sem os sufixos de versão e de mensagens.
- **Cabeçalhos:** as três rotas enviam `User-Agent: ai-memory/<versão>` e `x-opencode-session: <UUID da operação>`. `AI_MEMORY_LLM_HEADERS` pode sobrescrevê-los. Na rota Anthropic a chave vai em `x-api-key`; nas outras, como bearer.
- **Saída estruturada:** segue o protocolo da rota. Na rota de Chat Completions, o modo é o tolerante. O modo estrito de `openai-compat` não pode ser ligado por este provedor (seção 4.9).
- **Modelo padrão:** `mimo-v2.6-flash`, definido na escolha do provedor.

### 4.9 `openai-compat`

Cobre servidores locais e agregadores que falam o dialeto de Chat Completions.

- **Base URL:** obrigatória, por `AI_MEMORY_LLM_BASE_URL`, `llm_base_url` ou `LLM_BASE_URL`. Sem ela, a inicialização falha.
- **Modelo:** obrigatório. Sem valor, a inicialização falha.
- **Chave:** opcional, lida de `LLM_API_KEY`. Sem chave, a requisição envia um valor substituto no cabeçalho de autorização. Servidores locais costumam ignorá-lo.
- **Dialeto:** Chat Completions legado. Não há campo de limite de tokens de conclusão, nem ajuste de temperatura, nem limite de saída imposto pelo cliente.
- **Modo estrito** (`AI_MEMORY_LLM_COMPAT_STRICT`, padrão `true`). Com `false`, o modo tolerante é usado sempre. Use `false` apenas com um endpoint incompatível.
  - No modo estrito, o schema vai em `response_format` do tipo `json_schema`. Se o endpoint recusar o campo, ou devolver uma forma inesperada, o provedor recorre ao modo tolerante com uma segunda chamada.
  - No modo tolerante, o texto passa por limpeza de blocos de raciocínio, remoção de cerca de código e extração do primeiro objeto JSON balanceado.
  - Se `AI_MEMORY_LLM_COMPAT_DISABLE_THINKING=true` (ou `llm_compat_disable_thinking = true`), todo pedido leva `chat_template_kwargs: {"enable_thinking": false}` (para vLLM/SGLang com modelos da classe Qwen3 que gastam o orçamento em raciocínio e truncam o JSON). Padrão desligado.
  - Detalhes completos em [cliente de LLM](llm-client.md), seção 5.5.
- **Custo:** o modo estrito seguido de fallback faz duas chamadas. Para modelos de raciocínio que emitem blocos de pensamento, desligue o modo estrito.
- **Cabeçalhos:** `x-request-id` em cada tentativa, com o id da operação. Quando a base contém `openrouter.ai`, o provedor envia também `HTTP-Referer: https://github.com/akitaonrails/ai-memory` e `X-Title: ai-memory`, com precedência para o que o operador definir.
- **Sem chave:** a requisição leva `Authorization: Bearer dummy`.

### 4.10 `cursor`

Chama o CLI `agent` do Cursor (assinatura), já autenticado no aplicativo; nenhuma credencial é lida ou copiada pelo ai-memory.

- **Seleção:** `AI_MEMORY_LLM_PROVIDER=cursor`; modelo padrão `cursor-grok-4.6-high` (qualquer outro id de `agent --list-models`; `default` não passa `--model`). `llm-test --provider cursor` é aceito.
- **Executável:** `AI_MEMORY_CURSOR_AGENT`, senão `agent` do PATH. Não é verificado na inicialização; um executável ausente só falha na primeira chamada, com a orientação de instalar o agente ou definir a variável.
- **Segurança:** o filho roda com ambiente em lista permitida (nunca o do servidor), em modo `ask` (somente leitura), sem `--yolo`/`--force`, em um diretório temporário privado apagado ao fim.
- **Limites e erros:** timeout igual ao de chat; saída máxima de 1 MiB (stdout) e 64 KiB (stderr); sem esforço de raciocínio, sem cabeçalhos extras, sem uso de tokens na resposta.
- Protocolo, lista de variáveis e mensagens: [cliente de LLM](llm-client.md), seção 5.10.

## 5. Limites e ajustes comuns

### 5.1 Timeout

Cada requisição de chat tem limite de 300 s. O valor pode ser alterado por `AI_MEMORY_LLM_TIMEOUT_SECS` ou por `llm_timeout_secs` no `config.toml`:

```toml
llm_timeout_secs = 900
```

Zero é rejeitado na carga. O padrão tolera um engine local que carrega um modelo grande na primeira requisição. Em gateways hospedados lentos, cujas respostas longas passam desse limite, cada requisição falha com erro de envio de requisição, e a consolidação esgota as retentativas. Nesse caso, aumente o valor. A renovação de token OAuth usa limite próprio (ver [cliente de LLM](llm-client.md), seção 4.3).

### 5.2 Temperatura nos provedores Anthropic

Para os dois provedores Anthropic, o ai-memory omite `temperature` em modelos a partir da versão 4.7 e no Mythos Preview, porque esses modelos rejeitam parâmetros de amostragem diferentes do padrão. Nos demais, a temperatura é enviada. O `llm-test` envia o mesmo valor representativo de 0.2 usado no pipeline normal, antes dessa regra.

### 5.3 Modelos locais de contexto pequeno

Para modelos locais com janela pequena, configure os dois limites de consolidação. O limite de entrada considera o prompt inteiro (contexto de slots, página atual limitada e o schema de saída estruturada). O limite de saída é enviado ao provedor. A soma deve caber na janela do modelo, com folga, porque os tokenizadores variam:

```toml
[consolidation]
max_input_tokens = 8350
max_output_tokens = 1000
```

Os equivalentes em ambiente são `AI_MEMORY_CONSOLIDATION__MAX_INPUT_TOKENS` e `AI_MEMORY_CONSOLIDATION__MAX_OUTPUT_TOKENS`. Na carga, os mínimos validados são 20 040 caracteres efetivos de entrada (8 350 tokens com a margem de segurança padrão 0,8, `[consolidation] input_token_safety_margin`; 6 680 com margem 1,0) e 1 000 tokens de saída. Também existe `[consolidation] verbatim_residue` (padrão falso): ver [consolidation-llm.md](consolidation-llm.md) §4.3.

## 6. Consolidação e reranking que usam o provedor

### 6.1 Preferências de consolidação por projeto

Para manter o estilo da consolidação específico de um projeto, escreva `_prompts/consolidation.md` na wiki desse projeto. O corpo pode expressar preferências como "prefira títulos em português" ou "omita ruído rotineiro de CI".

- A consolidação automática, de página única e de várias páginas usam essa página. Uma chamada manual de `memory_consolidate` pode passar `instructions`, que substitui a página apenas nessa chamada.
- O texto é limitado a 2 000 caracteres (com marcador de truncamento), passa pelo sanitizador da wiki e é enviado como string JSON na mensagem do usuário, marcado como dado de projeto não confiável.
- A preferência é consultiva. Não pode fornecer fatos, pedir uso de ferramentas ou divulgação, nem sobrepor o schema e as regras de fidelidade da consolidação.
- Páginas de preferência expiradas por TTL são ignoradas. Sem página ativa e sem argumento, nenhum bloco de preferência é acrescentado ao prompt.

### 6.2 Falhas e retentativas

- **Dentro de uma tentativa:** erros transitórios (status 429, status 5xx, timeout ou conexão recusada) são repetidos até 3 tentativas no total, com atraso fixo de 2 s. Erros determinísticos (autenticação, schema, requisição malformada, corpo inválido ou truncado) não são repetidos.
- **Checkpoint automático (PreCompact e PostCompaction):** a página de regras é gravada **na hora** e a reescrita pelo provedor vai para a fila durável (§6.3); falha do provedor deixa a página de regras. Política de admissão que recusa `consolidate` faz o checkpoint falhar fechado antes da página; falha de infraestrutura só gera aviso (ver [consolidation-llm.md](consolidation-llm.md) §3.8).

### 6.3 Fila de consolidação no fim de sessão

Com `AI_MEMORY_CONSOLIDATE_ON_SESSION_END` ligada (e, para os checkpoints de compactação, sempre que há provedor), o trabalho de provedor entra em uma fila durável, por sessão e geração de observações. O enfileiramento acontece depois das escritas determinísticas e fora da resposta do hook. Um único worker no servidor processa a fila:

- verifica a fila a cada 15 s, ou antes, quando notificado;
- reserva cada job por um lease de 10 min, após o qual o job pode ser recuperado;
- até 5 tentativas por job. As esperas entre tentativas são de 30 s, 60 s, 120 s e 240 s; erro de forma da resposta (JSON inválido, forma inesperada, resposta truncada ou vazia) encerra o job na hora, sem esperar;
- ao esgotar as tentativas, o job termina com falha e a página de regras permanece;
- sobrevive a reinício do servidor;
- uma nova geração de observações substitui o trabalho pendente da geração anterior da mesma sessão.

Sem a opção ligada, o fim de sessão não enfileira trabalho de provedor. A página de regras é gravada como descrito na seção 1.

### 6.4 Reranking (opcional)

O reranking é desligado por padrão. Com um provedor configurado, `AI_MEMORY_RERANKER=llm` faz consultas de `memory_query` com escopo de projeto ou de escopos explícitos buscarem mais candidatos, e um único chamado ao LLM reordena os melhores.

- **Candidatos:** o limite de busca é 3 vezes o pedido, com teto de 30 candidatos e nunca abaixo do limite pedido.
- **Conteúdo enviado:** a consulta, mais no máximo 30 títulos e trechos de página. Trechos são limitados a 600 bytes, títulos a 200 bytes e a consulta a 1 000 bytes. Todos os valores são tratados como dados não confiáveis.
- **Concorrência:** no máximo 4 chamadas ao provedor em voo. Quando o limite está ocupado, a consulta mantém a ordem local, sem esperar.
- **Tempo limite:** 20 s por chamada. Após esse prazo, a consulta responde com a ordem anterior ao reranking.
- **Falhas:** erro do provedor, resposta parcial ou inválida e tempo esgotado preservam a ordem anterior.
- **Consultas com `global=true`:** não passam pelo reranking.

## 7. Embeddings

Embeddings são independentes do provedor de chat. Podem usar outro provedor, e o `AI_MEMORY_LLM_BASE_URL` não os afeta.

### 7.1 Escolha e padrão

- **Sem `AI_MEMORY_EMBEDDING_PROVIDER`:** o ai-memory usa embeddings locais, em processo, com melhor esforço. Se o modelo não puder ser obtido ou carregado, o servidor segue sem embeddings.
- **`none`, `off` ou `disabled`:** desliga os embeddings.
- **Provedor explícito:** falhas de configuração ou de carga interrompem a inicialização, sem degradação silenciosa.
- **Provedores aceitos:** `openai`, `voyage`, `google` (ou `gemini`), `openai-compat` (ou `openai_compat`), `local` e `copilot` (comparação exata, minúsculas).
- **Prefixos assimétricos:** `AI_MEMORY_EMBEDDING_QUERY_PREFIX` / `AI_MEMORY_EMBEDDING_DOCUMENT_PREFIX` (ou `embedding_query_prefix` / `embedding_document_prefix`) valem só para `openai` e `openai-compat`, sem aparar; modelos conhecidos e textos exatos (E5, Nemotron, e5-mistral-instruct, Qwen3-Embedding) em [cliente de LLM](llm-client.md), seção 8.4. Mudar o prefixo de documento troca a identidade do modelo (vetores antigos ficam obsoletos); o de consulta não.
- **Retentativa e timeouts:** timeout de 120 s (OpenAI, Voyage, Google, compatível) ou 300 s (Copilot); 429 (e 503 em OpenAI/compatível, respeitando `Retry-After`) é repetido até 5 vezes, no máximo 62 s de espera (seção 8.3 do cliente de LLM).

Para recuperação vetorial, defina explicitamente o provedor. A busca combina embeddings com texto completo, entidades e vizinhos do grafo. Nos caminhos somente FTS e híbrido, a mesma autoridade de página, com ajuste limitado, é aplicada depois da geração dos candidatos. Embeddings melhoram o recall, mas não decidem qual fonte é canônica.

### 7.2 Modelos e dimensões padrão

Quando `AI_MEMORY_EMBEDDING_MODEL` ou `AI_MEMORY_EMBEDDING_DIM` não são informados, valem os padrões abaixo.

| Provedor | Modelo padrão | Dimensão padrão |
|---|---|---|
| `openai` | `text-embedding-3-small` | 1536 (3072 para `text-embedding-3-large`) |
| `voyage` | `voyage-3` | 1024 |
| `google` / `gemini` | `gemini-embedding-001` | 768 |
| `local` | `all-MiniLM-L6-v2` | 384 |
| `copilot` | `text-embedding-3-small` | 1536 |
| `openai-compat` | obrigatório | obrigatória (dimensões variam por modelo) |

`AI_MEMORY_EMBEDDING_DIM` precisa ser maior que zero, quando informada. Para o Google, o modelo `gemini-embedding-001` é gravado como `models/gemini-embedding-001` na identidade do vetor. Para `openai`, qualquer modelo diferente de `text-embedding-3-large` assume 1536.

### 7.3 Credenciais de embedding

- **`openai`:** `EMBEDDING_API_KEY`, depois `OPENAI_API_KEY`. Se `AI_MEMORY_EMBEDDING_BASE_URL` estiver definida, `LLM_API_KEY` também é aceita como último recurso. Sem nenhuma das três, a configuração falha.
- **`voyage`:** `VOYAGE_API_KEY`.
- **`google` / `gemini`:** `GEMINI_API_KEY`, ou `GOOGLE_API_KEY`.
- **`openai-compat`:** `EMBEDDING_API_KEY`, depois `LLM_API_KEY`. A chave é opcional, e servidores locais normalmente não precisam de uma.
- **`copilot`:** a mesma credencial do provedor de chat `copilot` (seção 4.7). Não há chave própria.
- **`local`:** nenhuma credencial.

### 7.4 Base URL e `openai-compat`

- `AI_MEMORY_EMBEDDING_BASE_URL` (`embedding_base_url`) define o endpoint de embeddings, separado do endpoint de chat. Para mudar o endpoint de embeddings, defina essa variável; `AI_MEMORY_LLM_BASE_URL` não a altera.
- `openai-compat` exige `AI_MEMORY_EMBEDDING_BASE_URL`, `AI_MEMORY_EMBEDDING_MODEL` e `AI_MEMORY_EMBEDDING_DIM`. Serve para Ollama, LM Studio, vLLM e engines similares.

### 7.5 Embeddings do Copilot

Usa `POST <base do Copilot>/embeddings` (base resolvida como no chat, seção 4.7), com corpo `{"input": <texto truncado a 8000 bytes>, "model": <modelo>}` e os cabeçalhos de runtime do Copilot; resposta no formato OpenAI. O formato exato desse endpoint não foi coberto por teste contra o serviço real (o código segue o contrato compatível com OpenAI e a prática de outros clientes do Copilot). Faça um teste de fumaça com o Copilot real antes de depender dele em produção. Retentativa só para 429; sem prefixos.

### 7.6 Embeddings locais

`AI_MEMORY_EMBEDDING_PROVIDER=local` não precisa de chave nem de servidor. A inferência roda em processo, com o modelo `all-MiniLM-L6-v2` (384 dimensões).

- Os arquivos do modelo (cerca de 87 MB) são obtidos uma vez para `<data_dir>/models/all-MiniLM-L6-v2/`.
- Cada arquivo tem um SHA-256 fixado. Um arquivo divergente faz a carga falhar, para que um modelo alterado não mude silenciosamente todos os vetores.
- Em instalações offline, os três arquivos podem ser colocados nesse diretório manualmente. A verificação de checksum continua valendo.

Detalhes em [embeddings locais](local-embeddings.md).

## 8. Uso em contêineres

O wrapper de Docker encaminha por nome, para os comandos auxiliares de curta duração (como `llm-test`), estas variáveis: `ANTHROPIC_API_KEY`, `ANTHROPIC_OAUTH_TOKEN`, `CLAUDE_CODE_OAUTH_TOKEN`, `OPENAI_API_KEY`, `GEMINI_API_KEY`, `GOOGLE_API_KEY`, `COPILOT_GITHUB_TOKEN`, `GITHUB_COPILOT_API_TOKEN`, `COPILOT_API_URL`, `OPENCODE_API_KEY`, `VOYAGE_API_KEY`, `LLM_API_KEY`, `EMBEDDING_API_KEY`, além das variáveis `AI_MEMORY_LLM_*` e `AI_MEMORY_EMBEDDING_*`.

- O contêiner do servidor, de longa duração, precisa ser configurado à parte. Ver [instalação](install.md#llm-provider-tiers).
- Para `openai-oauth`, o login feito pelo wrapper grava o token no volume de dados padrão (`ai-memory-data`).
- Para `codex`, o executável, `CODEX_HOME` e o arquivo de login não são montados automaticamente (seção 4.5).

## 9. Teste de provedor

```bash
export AI_MEMORY_LLM_PROVIDER=opencode
export AI_MEMORY_LLM_MODEL=mimo-v2.5
ai-memory llm-test --provider opencode --model mimo-v2.5 --prompt "Reply with OK"
```

- `llm-test` aceita `--provider` com o nome canônico (`anthropic`, `anthropic-oauth`, `openai`, `gemini`, `openai-compat`, `openai-oauth`, `codex`, `copilot`, `opencode`, `cursor`), `--model` (**obrigatório**), `--prompt` (**obrigatório**), `--structured` (resposta com schema JSON), `--base-url` (obrigatória para `openai-compat` salvo `LLM_BASE_URL` no ambiente) e `--api-key` (sobrepõe a variável de ambiente; vazio é ignorado).
- Pedido enviado: um único prompt de usuário, temperatura 0,2 (antes das regras de cada provedor), limite de saída padrão de 1024 tokens. Usa o timeout, o esforço, os cabeçalhos e o modo estrito da configuração.
- Modo estruturado: schema fixo `{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"]}`. Saída: `--- model: <modelo> ---` e o JSON indentado.
- Modo texto: `--- model: <modelo ecoado> ---`, `--- usage: in=<n> out=<n> ---` (só se houver uso) e o texto.
- Falhas viram erro de execução com contexto `building LLM provider`, `calling provider` ou `calling provider for structured output`. Para `anthropic-oauth` com status 429 acrescenta a dica descrita na seção 4.2.
- O `llm-test` resolve o endpoint do mesmo modo que o servidor. Inclui a troca do `opencode` para Zen e a recusa de herdar `LLM_BASE_URL` em provedores de endpoint fixo.
- Os comandos completos estão em [CLI](cli.md).
