# Cadeia de fallback de provedores LLM

> **Papel:** define a cadeia ordenada de provedores de LLM que o servidor tenta em sequência quando um provedor falha de forma transitória. É opt-in: sem perfis de fallback configurados, o servidor usa um único provedor, como antes. Para a fronteira de rede dos modelos, ver [Cliente de LLM](llm-client.md). Para o catálogo de provedores e suas credenciais, ver [Provedores de LLM](llm-providers.md).

## Motivação

Antes da cadeia, o servidor usava um único provedor de LLM para todas as operações baseadas em modelo. As novas tentativas existentes repetem a chamada no mesmo provedor. Uma limitação temporária de taxa ou uma indisponibilidade do provedor degradava bootstrap, consolidação, lint e revisão, mesmo quando outro provedor configurado estava saudável.

A cadeia acrescenta, atrás da mesma interface de provedor, uma lista ordenada de candidatos. Ela troca de candidato apenas após uma falha transitória.

## Objetivos

- Trocar de candidato apenas após uma falha transitória, usando a classificação de erro descrita na seção [Classificação de erros](#classificação-de-erros).
- Preservar, em cada tentativa, a requisição original, o esquema JSON da saída estruturada e o identificador lógico da operação. Assim, a retentativa feita pelo chamador mantém a semântica de idempotência.
- Manter credenciais fora de logs, do payload de status e do estado persistido.
- Não impor um prazo global à cadeia. Cada candidato usa o timeout de requisição do LLM (ver [Limites](#limites)).
- Tornar visível, pela saúde passiva do provedor, qual candidato respondeu e o resultado do failover.

## Fora de escopo

- Roteador genérico para APIs de terceiros ou integração com ferramentas externas de agentes.
- Leitura de credenciais do banco de dados de outra aplicação ou instalação de CLI de provedor.
- Repetição de falhas determinísticas: autenticação, requisição inválida, esquema não suportado e resposta malformada.
- Alteração de embeddings, latência de hooks, comportamento de wiki e armazenamento, ou esquemas MCP.

## Configuração

### Primário

Os campos de LLM de topo da configuração (provedor, modelo, URL base, timeout e demais) definem o primário. O primário é sempre o primeiro candidato. Se o modelo não for informado, o primário usa o modelo padrão do próprio provedor.

### Perfis de fallback

Cada entrada `[[llm_fallbacks]]` do arquivo de configuração (TOML) é um candidato adicional:

| Campo | Obrigatório | Regra |
| --- | --- | --- |
| `provider` | sim | Um dos valores: `anthropic`, `openai`, `gemini` (alias `google`), `openai-compat` (alias `openai_compat`), `openai-oauth` (alias `openai_oauth`), `codex`, `copilot` (aliases `github-copilot`, `github_copilot`), `anthropic-oauth` (alias `anthropic_oauth`), `opencode` (aliases `opencode-zen`, `opencode_zen`), `cursor`. Vazio ou desconhecido é erro. |
| `model` | sim | Não pode ser vazio. Não há modelo padrão para fallback. |
| `base_url` | depende do provedor | Opcional. Obrigatório para `openai-compat`. Vale só para o próprio perfil: não herda a base do primário nem o `LLM_BASE_URL` do ambiente. Ignorado por `openai-oauth`, `codex`, `copilot` e `cursor`. |
| `api_key_env` | depende do provedor | Nome da variável de ambiente que contém a chave de API. Obrigatório para `anthropic`, `openai`, `gemini` e `opencode`. Opcional para `openai-compat`. Não se aplica aos provedores com fonte nativa de credencial (`openai-oauth`, `codex`, `copilot`, `anthropic-oauth`), que compartilham a credencial do primário. Nunca herda a variável de ambiente do primário. |

Um valor vazio ou só com espaços conta como ausente. Os perfis de fallback não definem timeout, modo estrito de compatibilidade, esforço de raciocínio nem cabeçalhos extras: esses valores vêm dos campos de LLM de topo e valem para o primário e para cada fallback.

Não há atalho por variável de ambiente para a lista de fallbacks, porque uma única variável não representaria com clareza os limites de cada perfil nem os nomes de credencial.

```toml
llm_provider = "opencode"
llm_model = "mimo-v2.5-free"

[[llm_fallbacks]]
provider = "openai-compat"
model = "poolside/laguna-s-2.1-free"
base_url = "http://127.0.0.1:49375/v1"
api_key_env = "AI_MEMORY_LOCAL_ROUTER_TOKEN"

[[llm_fallbacks]]
provider = "gemini"
model = "gemini-3.5-flash"
api_key_env = "GEMINI_API_KEY"
```

### Ordem

A ordem de tentativa é o primário e, depois, os fallbacks na ordem em que aparecem no arquivo de configuração.

### Validação

A validação ocorre na carga da configuração, que roda antes de qualquer comando, exceto a geração de completions:

- **Falha na carga (todo comando falha):** `provider` vazio ou desconhecido, `model` vazio, provedor que exige chave sem `api_key_env`, e perfil malformado (por exemplo, `openai-compat` sem `base_url`). Os perfis são validados e o provedor de cada um é construído já na carga, para que um perfil ruim não fique silencioso até o primário cair.
- **Credencial ausente (diferida):** se a variável referenciada por `api_key_env` estiver ausente ou vazia, a carga não falha. Esse perfil fica pendente e:
  - `ai-memory serve` recusa iniciar, mesmo sem primário configurado;
  - a montagem da cadeia retorna erro.
  
  Os demais comandos não montam a cadeia e seguem normalmente. Por isso, a variável só precisa existir no ambiente do servidor (por exemplo, no bloco de ambiente de um gerenciador de serviço), e não em cada shell.
- **Fallbacks sem primário:** se `llm_provider` estiver vazio, nenhum provedor é montado. Os perfis de fallback são ignorados sem erro, e o servidor opera sem LLM, como em qualquer instalação sem provedor. A checagem de credenciais do `serve` ainda se aplica a esses perfis.

## Montagem da cadeia

- Sem perfis de fallback: o servidor usa um único provedor, sem cadeia, sem circuitos e sem erro agregado.
- Com perfis: o servidor constrói o primário e cada fallback pelo mesmo caminho de construção do provedor único, e envolve os candidatos em uma cadeia.
- Cada candidato guarda apenas o nome do provedor e o modelo (para diagnóstico) e a instância do provedor. A cadeia não guarda a configuração bruta nem as credenciais.
- Para o restante do servidor, a cadeia é um único provedor. O nome e o modelo que ela expõe são os do primário.
- Apenas o comando `serve` monta a cadeia.

## Ordem de tentativa

1. Para cada candidato, na ordem: se o circuito dele estiver aberto, ele é pulado e não conta como tentativa.
2. O candidato é chamado com a mesma requisição, o mesmo esquema JSON e o mesmo identificador de operação usados pelo chamador.
3. **Sucesso:** o candidato registra o sucesso, o circuito dele fecha e a resposta é devolvida.
4. **Falha transitória:** o erro é registrado, o circuito do candidato abre (ver [Circuito por candidato](#circuito-por-candidato)) e a cadeia avança para o próximo.
5. **Falha determinística:** o erro é registrado e devolvido imediatamente, sem tentar os candidatos restantes.
6. Se todos os candidatos elegíveis falharam de forma transitória, ou se não restou nenhum elegível, a cadeia devolve um erro agregado.

O erro agregado tem a forma `llm fallback chain exhausted after N candidate(s): ...` (cada entrada `provedor/modelo: classe` ou `provedor/modelo: classe status`, sem colchetes). N conta apenas as tentativas feitas; candidatos pulados por circuito aberto não entram na conta. O resumo lista uma entrada por tentativa, no formato `provedor/modelo: classe` ou `provedor/modelo: classe status`, separadas por `; `. O resumo contém apenas rótulos e status, nunca corpo de resposta nem segredo.

Casos de borda:

- Se todos os circuitos estiverem abertos, N é 0 e o resumo é vazio.
- Um erro determinístico em um candidato posterior é devolvido como está, e não como erro agregado, mesmo que candidatos anteriores tenham falhado de forma transitória.
- O erro agregado não é transitório. Ele não é repetido pela própria cadeia.
- Nos endpoints administrativos de bootstrap e de auto-improve, um erro de LLM, inclusive o agregado, vira resposta HTTP 502.

## Classificação de erros

| Falha | Tenta o próximo candidato? |
| --- | --- |
| Status 429 | Sim |
| Status de 500 a 599 (inclusive os da faixa 52x) | Sim |
| Timeout de requisição | Sim |
| Falha de conexão | Sim |
| Status 400, 401, 403, 404, 422 e qualquer outro status não listado acima | Não |
| Erro de autenticação, de esquema, de (de)serialização, de forma inesperada da resposta ou de configuração ausente | Não |
| Outro erro de transporte (não é timeout nem falha de conexão) | Não |
| Erro agregado da própria cadeia | Não |

A classificação é a mesma para o primário e para cada fallback.

## Circuito por candidato

- O estado do circuito fica em memória e é indexado pela posição do candidato na cadeia. Duas entradas com o mesmo provedor e modelo têm circuitos separados.
- Um erro transitório abre o circuito do candidato por 30 segundos. Um erro determinístico não altera o circuito.
- Ao fim dos 30 segundos, o candidato volta a ser elegível sem nenhuma chamada explícita.
- Um sucesso fecha o circuito do candidato imediatamente.
- Reiniciar o servidor limpa todos os circuitos. Não há persistência de estado nem cooldown compartilhado entre processos.
- A cadeia não faz sondagem: o estado do circuito muda apenas com chamadas reais.

## Saúde passiva

O relatório de saúde do provedor de LLM tem duas partes.

**Papel LLM (campos de topo, sem mudança de formato):** provedor, modelo, estado, instante da última chamada, instantes do último sucesso e do último erro, status e mensagem do último erro, e dica de nova tentativa. A mensagem do último erro é truncada em 1.024 caracteres e pode conter o corpo de resposta do provedor, como no comportamento anterior à cadeia.

**Candidatos (`llm_candidates`):** lista na ordem declarada, em que o índice 0 é o primário. Fica vazia quando não há cadeia. Cada entrada traz:

- provedor e modelo;
- se o candidato foi o último selecionado, isto é, se respondeu à última chamada concluída com sucesso;
- instante do último sucesso e instante do último erro;
- status HTTP do último erro, quando houver;
- classe do último erro, um rótulo estável da mesma tabela de classificação (`http`, `provider`, `serde`, `unexpected-shape`, `truncated-response`, `empty-content`, `not-configured`, `auth`, `schema`, `all-candidates-failed`);
- instante em que o circuito reabre, apenas enquanto o circuito estiver aberto.

O estado por candidato nunca guarda corpo de resposta nem segredo.

O comando `ai-memory status` lista a cadeia somente quando `llm_candidates` não está vazia. Cada linha tem o formato `provedor/modelo`, com `(last selected)` quando aplicável, seguido de um destes sufixos, nesta ordem de prioridade:

1. `— circuit open until <instante>`, se o circuito estiver aberto;
2. `— last error: status N`, se houver status do último erro;
3. `— last error: <classe>`, se houver classe do último erro.

A saúde registra apenas chamadas que o servidor já fez. Não há sondagem de fallbacks nem tráfego de recuperação em segundo plano.

## Limites

- Circuitos e saúde ficam só em memória. Reiniciar o servidor os zera.
- Não há prazo total para a cadeia. Cada candidato usa o timeout de requisição do LLM, que é global e tem padrão de 300 segundos. Não existe timeout por perfil. Assim, uma cadeia com vários candidatos lentos pode somar vários timeouts antes de devolver um erro.
- Os perfis de fallback herdam do primário o timeout, o modo estrito de compatibilidade, o esforço de raciocínio e os cabeçalhos extras.
- Sem primário, os perfis de fallback são ignorados (ver [Validação](#validação)).
- O estado do circuito não é durável e não é compartilhado entre processos.
