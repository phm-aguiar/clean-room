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

Em `config.toml`, as chaves têm o nome da variável sem o prefixo: `llm_provider`, `llm_model`, `llm_base_url`, `llm_headers`, `llm_timeout_secs`, `llm_reasoning_effort` e `llm_compat_strict`. Sem `llm_provider` (ou `AI_MEMORY_LLM_PROVIDER`), o LLM fica desligado.

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

Para provedores de assinatura, um modelo pequeno e rápido reduz o consumo do limite de uso. O trabalho do ai-memory (consolidação, lint, exploração) é sumarização, não raciocínio difícil. Por isso:

- `anthropic-oauth`: defina `claude-haiku-4-5`.
- `openai-oauth` e `codex`: mantenha o modelo padrão. O backend do Codex/ChatGPT aceita um conjunto de ids definido pelo servidor e recusa os outros com erro 400 (a confirmar, comportamento do serviço externo que não se verifica no código).
- `copilot`: um id da classe mini, como `gpt-5-mini`, pode funcionar, mas o conjunto de modelos aceitos pelo Copilot não foi verificado (a confirmar). Se o endpoint recusar a escolha, volte ao padrão.

### 2.4 Cabeçalhos extras e esforço de raciocínio

`AI_MEMORY_LLM_HEADERS` (`llm_headers`) aceita uma lista de cabeçalhos no formato `Nome: Valor`, enviada a todos os provedores de chat. Cabeçalhos que o próprio ai-memory define, e entradas malformadas, interrompem a inicialização. Detalhes em [cliente de LLM](llm-client.md), seção 2.5.

`AI_MEMORY_LLM_REASONING_EFFORT` (`llm_reasoning_effort`) aceita um valor de esforço; valor inválido interrompe a carga. Os provedores que ignoram o esforço estão listados em [cliente de LLM](llm-client.md), seção 5.9.

## 3. Catálogo resumido

| Provedor | Credencial | URL padrão | Base URL configurável |
|---|---|---|---|
| `anthropic` | `ANTHROPIC_API_KEY` | `https://api.anthropic.com` | Não. A base informada não é aplicada. |
| `anthropic-oauth` | `ANTHROPIC_OAUTH_TOKEN`, ou `CLAUDE_CODE_OAUTH_TOKEN` | `https://api.anthropic.com` | Sim. |
| `openai` | `OPENAI_API_KEY` | `https://api.openai.com` | Não. A base informada não é aplicada. |
| `openai-oauth` | Login próprio, em `auth.json` no diretório de dados | Backend ChatGPT/Codex (`https://chatgpt.com/backend-api/codex/responses`) | Não. |
| `codex` | Arquivo de login do Codex CLI | Mesmo backend do `openai-oauth` | Não. O executável do Codex é configurável (`AI_MEMORY_CODEX_EXECUTABLE`). |
| `copilot` | Token GitHub do ambiente, login armazenado ou token de API direto | `https://api.githubcopilot.com`, ou o host indicado pelo token | Sim, por `COPILOT_API_URL` ou pela base URL (seção 4.7). |
| `gemini` | `GEMINI_API_KEY`, ou `GOOGLE_API_KEY` | `https://generativelanguage.googleapis.com` | Sim. |
| `opencode` | `OPENCODE_API_KEY` | `https://opencode.ai/zen/go/v1` (Go) | Sim. Para Zen, use `https://opencode.ai/zen/v1`. |
| `openai-compat` | `LLM_API_KEY` (opcional) | Nenhuma | Obrigatória. |

Para os provedores marcados como "Não", a base URL configurada (`AI_MEMORY_LLM_BASE_URL` ou `llm_base_url`) não é aplicada. Esses provedores falam sempre com o host do fornecedor. Para apontar a um proxy, use um provedor que aceite base URL, como `openai-compat`.

## 4. Detalhes por provedor

### 4.1 `anthropic`

- **Protocolo:** Messages, no caminho `/v1/messages` da base. A versão da API é fixa.
- **Autenticação:** a chave de API vai no cabeçalho de chave dedicado da API.
- **Saída estruturada:** ferramenta única com o schema e escolha forçada. Detalhes em [cliente de LLM](llm-client.md), seção 5.1.
- **Temperatura:** ver seção 5.2.

### 4.2 `anthropic-oauth`

- **Token:** gerado uma vez por `claude setup-token`. Esse comando grava `CLAUDE_CODE_OAUTH_TOKEN`, que o ai-memory aceita. O nome canônico é `ANTHROPIC_OAUTH_TOKEN`; se os dois estiverem definidos, vale o canônico.
- **Sem chave de API:** `ANTHROPIC_API_KEY` não é necessária.
- **Autenticação:** o token vai como bearer, junto com o cabeçalho de beta de OAuth. O mesmo endpoint de `anthropic` é usado. Detalhes em [cliente de LLM](llm-client.md), seção 5.1.
- **Modelo recomendado:** `claude-haiku-4-5` (seção 2.3).

> **Aviso:** este método não é oficial e vai contra as políticas de uso da Anthropic. Use por sua conta e risco: a conta pode sofrer limitação de taxa ou banimento. Ver [instalação](install.md#anthropic-via-claude-subscription-oauth).

### 4.3 `openai`

- **Protocolo:** Chat Completions, na base do fornecedor.
- **Autenticação:** chave de API como bearer.
- **Saída estruturada:** `json_schema` estrito com nome `Result`. Detalhes em [cliente de LLM](llm-client.md), seção 5.2.
- **Limites de saída e temperatura:** as famílias gpt-5, gpt-6 e o-series usam o campo de limite de tokens de conclusão e não recebem temperatura. Os demais modelos recebem o menor valor entre o pedido e 16 384 tokens.

### 4.4 `openai-oauth`

- **Login:** `ai-memory auth login openai-oauth`. A espera pela autorização termina em 600 s por padrão (`--timeout-secs`).
- **Armazenamento:** o refresh token fica em `auth.json` no diretório de dados. Esse arquivo é compartilhado com outros logins, e cada login tem a sua entrada. Ver [cliente de LLM](llm-client.md), seção 4.3.
- **Endpoint:** o backend ChatGPT/Codex, não `api.openai.com`.
- **Identificação da conta:** o cabeçalho de conta vem do token de identidade ou do token de acesso. Ver [cliente de LLM](llm-client.md), seção 5.3.
- **Contêineres:** execute o login pelo wrapper de Docker. Assim o token cai no mesmo volume de dados (por padrão `ai-memory-data`) que o servidor usa.

### 4.5 `codex`

- **Credencial:** lê apenas `tokens.access_token` e `tokens.account_id` do arquivo de login do Codex CLI. Os dois campos são obrigatórios e não podem estar vazios.
- **Localização do arquivo:** `$CODEX_HOME/auth.json`. Se `CODEX_HOME` não estiver definido ou estiver vazio, usa `.codex/auth.json` no diretório home da plataforma.
- **Não copia credenciais:** o provedor nunca grava credenciais no diretório de dados do ai-memory e não lê refresh token nem ID token. Cada requisição relê o arquivo.
- **Modo de armazenamento:** só o arquivo é lido. Credenciais que existem apenas no armazenamento do sistema (keyring) ou só em memória não são lidas, e o erro indica que o arquivo está indisponível. Como o provedor se comporta quando o Codex CLI está configurado em modo `auto`: a confirmar.
- **Recuperação de 401:** uma primeira resposta 401 pode disparar uma recuperação, que executa o servidor do Codex e tenta de novo uma vez. Um segundo 401 gera erro de autenticação com a orientação de verificar o login do Codex. Detalhes e limites do processo em [cliente de LLM](llm-client.md), seção 5.4.
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

- **Login:** `ai-memory auth login copilot` (fluxo de dispositivo do GitHub). A opção `--github-token` grava um token GitHub informado, em vez de executar o fluxo.
- **Ordem das credenciais:**
  1. token de API direto em `GITHUB_COPILOT_API_TOKEN`, usado como está, sem troca e sem gravação;
  2. token GitHub do ambiente, na ordem `COPILOT_GITHUB_TOKEN`, `GH_TOKEN`, `GITHUB_TOKEN`;
  3. token GitHub armazenado no login.
- **Troca de token:** o token GitHub é trocado por um token de API de curta duração. Esse token é gravado no arquivo compartilhado e reutilizado enquanto faltarem mais de 120 s para a expiração. Ver [cliente de LLM](llm-client.md), seção 4.3.
- **Sem credencial:** a inicialização falha e orienta a fazer o login ou definir `COPILOT_GITHUB_TOKEN`.
- **Base URL:** `COPILOT_API_URL` tem precedência. Sem ela, usa `llm_base_url` (ou `AI_MEMORY_LLM_BASE_URL`). Sem nenhuma das duas, o host padrão `https://api.githubcopilot.com` ou o host indicado pelo próprio token de API.
- **Endpoint por modelo:** Chat Completions ou Responses, conforme o modelo anuncia no catálogo do Copilot. Detalhes em [cliente de LLM](llm-client.md), seção 5.7.
- **Cabeçalhos:** identificação de integração de editor (identificador `vscode-chat`), sem o User-Agent padrão do ai-memory.
- **Esforço de raciocínio:** ignorado.

### 4.8 `opencode`

- **Chave:** `OPENCODE_API_KEY`, obrigatória. Não há fallback para `LLM_API_KEY`.
- **Endpoints:**
  - padrão: catálogo **Go** (`https://opencode.ai/zen/go/v1`), assinatura com franquia mensal por modelo;
  - Zen: defina `AI_MEMORY_LLM_BASE_URL=https://opencode.ai/zen/v1` (ou `llm_base_url`), para o catálogo geral com cobrança por token.
- **Ids de modelo:** escritos sem prefixo, por exemplo `mimo-v2.5` ou `glm-5.3-flash`. A forma `opencode-go/<modelo>` usada pelo cliente oficial do OpenCode não deve ser usada aqui (a confirmar, a forma aceita pelo endpoint não foi testada).
- **Escolha do protocolo pelo nome do modelo:** a comparação é exata e não diferencia maiúsculas.
  1. No catálogo Go, primeiro as listas específicas: Responses para `muse-spark-1.3-contributor` e `muse-spark-1.2-contributor`; Anthropic Messages para `minimax-m3`, `minimax-m2.7` e `qwen3.8-max`.
  2. Lista geral para Responses: famílias GPT (incluindo as variantes Luna), `grok-*` e `muse-spark-*`.
  3. Lista geral para Anthropic Messages: modelos Claude e Qwen.
  4. Qualquer outro id usa Chat Completions.
  
  Um modelo novo precisa de inclusão nessas listas para usar outro protocolo. Enquanto isso, ele cai em Chat Completions.
- **Base do protocolo Anthropic:** a base é usada sem os sufixos de versão e de mensagens.
- **Cabeçalhos:** as três rotas enviam a identificação padrão do ai-memory e o cabeçalho `x-opencode-session`, com um id por operação. `AI_MEMORY_LLM_HEADERS` pode sobrescrevê-los.
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
  - Detalhes completos em [cliente de LLM](llm-client.md), seção 5.5.
- **Custo:** o modo estrito seguido de fallback faz duas chamadas. Para modelos de raciocínio que emitem blocos de pensamento, desligue o modo estrito.
- **Cabeçalhos:** `x-request-id` em cada tentativa, com o id da operação. Quando a base aponta para o OpenRouter, o provedor envia também os cabeçalhos de identificação do aplicativo, com precedência para o que o operador definir.

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
max_input_tokens = 6500
max_output_tokens = 1000
```

Os equivalentes em ambiente são `AI_MEMORY_CONSOLIDATION__MAX_INPUT_TOKENS` e `AI_MEMORY_CONSOLIDATION__MAX_OUTPUT_TOKENS`. Na carga, os mínimos validados são 6 000 tokens de entrada e 1 000 de saída.

## 6. Consolidação e reranking que usam o provedor

### 6.1 Preferências de consolidação por projeto

Para manter o estilo da consolidação específico de um projeto, escreva `_prompts/consolidation.md` na wiki desse projeto. O corpo pode expressar preferências como "prefira títulos em português" ou "omita ruído rotineiro de CI".

- A consolidação automática, de página única e de várias páginas usam essa página. Uma chamada manual de `memory_consolidate` pode passar `instructions`, que substitui a página apenas nessa chamada.
- O texto é limitado a 2 000 caracteres (com marcador de truncamento), passa pelo sanitizador da wiki e é enviado como string JSON na mensagem do usuário, marcado como dado de projeto não confiável.
- A preferência é consultiva. Não pode fornecer fatos, pedir uso de ferramentas ou divulgação, nem sobrepor o schema e as regras de fidelidade da consolidação.
- Páginas de preferência expiradas por TTL são ignoradas. Sem página ativa e sem argumento, nenhum bloco de preferência é acrescentado ao prompt.

### 6.2 Falhas e retentativas

- **Dentro de uma tentativa:** erros transitórios (status 429, status 5xx, timeout ou conexão recusada) são repetidos até 3 tentativas no total, com atraso fixo de 2 s. Erros determinísticos (autenticação, schema, requisição malformada, corpo inválido ou truncado) não são repetidos.
- **Checkpoint automático (PreCompact e PostCompaction):** falhas do provedor caem para a página de regras, e a sessão não perde o estado de trabalho. Falhas de admissão, de armazenamento e de escopo não são cobertas por esse fallback (a confirmar).

### 6.3 Fila de consolidação no fim de sessão

Com `AI_MEMORY_CONSOLIDATE_ON_SESSION_END` ligada, o trabalho de provedor do fim de sessão entra em uma fila durável, por sessão e geração de observações. O enfileiramento acontece depois das escritas determinísticas e fora da resposta do hook. Um único worker no servidor processa a fila:

- verifica a fila a cada 15 s, ou antes, quando notificado;
- reserva cada job por um lease de 10 min, após o qual o job pode ser recuperado;
- até 5 tentativas por job. As esperas entre tentativas são de 30 s, 60 s, 120 s e 240 s;
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
- **Provedores aceitos:** `openai`, `voyage`, `google` (ou `gemini`), `openai-compat` (ou `openai_compat`), `local` e `copilot`.

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

`AI_MEMORY_EMBEDDING_DIM` precisa ser maior que zero, quando informada.

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

Usa o endpoint `/embeddings` da API do Copilot, com formato compatível com OpenAI. O formato exato desse endpoint não foi coberto por teste contra o serviço real. Faça um teste de fumaça com o Copilot real antes de depender dele em produção.

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

- `llm-test` aceita `--provider` com o nome canônico, `--model`, `--prompt`, `--structured` (resposta com schema JSON), `--base-url` (obrigatória para `openai-compat`) e `--api-key` (sobrepõe a variável de ambiente).
- O `llm-test` resolve o endpoint do mesmo modo que o servidor. Inclui a troca do `opencode` para Zen e a recusa de herdar `LLM_BASE_URL` em provedores de endpoint fixo.
- Os comandos completos estão em [CLI](cli.md).
