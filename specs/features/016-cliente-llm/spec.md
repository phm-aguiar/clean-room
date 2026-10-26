# Spec: Cliente LLM e provedores por chave

## Metadados
- **ID:** 016
- **Status:** spec auditada contra o original (ver "Correções da auditoria" e os relatórios em `reports/`); sem plano nem tasks; em toda dúvida, reproduzir o original
- **Autor:** phm-aguiar
- **Data:** 2026-10-13
- **Feature Anterior:** 002 (configuração, servidor, `/admin/status`, `status`), 015
- **Roadmap:** `specs/features/roadmap.md`
- **Aprovado:** false

## Propósito
> Para quem, o quê e por quê.

Todo recurso opcional que usa modelo (consolidação, rerank, `answer`, `memory_explore`, dream, auto-melhoria) precisa de uma única fronteira de rede para os provedores, com saída estruturada, erros classificados, contingência entre provedores e saúde visível. Esta feature entrega essa fronteira e os provedores que se autenticam por chave de API. Ela não liga nenhum consumidor: sem `llm_provider`, nada muda (INV-13).

## Escopo

### Dentro do escopo
- **Interfaces** (`docs/llm-client.md` §2): chat textual e estruturado (o esquema JSON é enviado; a resposta não é validada contra um tipo), wrapper tipado que deriva o esquema de um tipo Go e desserializa (falha vira erro de serialização), id de operação UUIDv7 por operação lógica enviado em todas as tentativas e candidatos e nunca derivado do id de sessão, tipos neutros (papel, mensagem, requisição com limite padrão de 1024 tokens no construtor de prompt único, resposta, uso).
- **Esforço de raciocínio** (§2.4): valores aceitos (`none` a `persistent`), rejeição na carga, mapeamento por destino (OpenAI e compatível, OpenRouter, xAI, Anthropic, Responses; Gemini e Copilot ignoram).
- **Cabeçalhos extras** `llm_headers` (§2.5): formato, separador, nomes reservados, erro sem expor o valor, precedência, `x-request-id` reservado com `openai-compat`.
- **Erros** (§3): categorias e rótulos de classe estáveis; transitório (429, 5xx incluindo 52x, timeout, falha de conexão) contra determinístico; corpo de erro lido até 1 MiB e exibido até 1 KiB; resposta acima de 16 MiB; texto com caractere de substituição.
- **Princípios** (§1): cliente nativo por provedor, sem retentativa no chat, autenticação resolvida antes da construção (INV-14), segredos redigidos em representações de depuração, timeout de 300 s (`llm_timeout_secs`, 0 rejeitado), `User-Agent: ai-memory/<versão>` por baixo dos cabeçalhos do operador.
- **Provedores por chave** (§5 e `docs/llm-providers.md`): `anthropic` (Messages, ferramenta única `result` com escolha forçada, reconhecimento de modelo, temperatura removida a partir de 4.7, pisos e ajustes de esforço), `openai` (Chat Completions, `json_schema` estrito `Result` com a normalização do esquema, famílias gpt-5/gpt-6/o-series, teto de 16 384), `gemini` (`responseSchema` preparado: referências inline até profundidade 16, chaves removidas, tipos em lista; `thinkingBudget: 0` nos modelos listados), `openai-compat` (base e modelo obrigatórios, chave opcional, modo estrito com recaída para o tolerante nas condições do doc, extração do primeiro objeto balanceado, `x-request-id`, cabeçalhos de atribuição do OpenRouter), `opencode` (bases Go e Zen, escolha de protocolo pelo nome do modelo, `x-opencode-session`, tolerante na rota de Chat Completions, modelo padrão).
- **Seleção** (`llm-providers.md` §2 e §3): nomes canônicos e aliases, modelos padrão por provedor, precedência de configuração, base URL aplicada só onde o catálogo permite, `LLM_BASE_URL` só para `openai-compat` e `opencode`, credenciais por variável (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `GEMINI_API_KEY`/`GOOGLE_API_KEY`, `LLM_API_KEY`, `OPENCODE_API_KEY`).
- **Contingência** (`docs/llm-provider-fallback.md`, `llm-client.md` §6): `[[llm_fallbacks]]` só no TOML; validação na carga (provedor e modelo, `api_key_env` obrigatório para os provedores por chave e nunca herdado, perfil construído e descartado); credencial ausente vira pendência `llm_fallback_unresolved` e só o `serve` recusa; sem primário, fallbacks ignorados; cadeia com circuito em memória de 30 s por posição, erro agregado `llm fallback chain exhausted after N candidate(s): ...`, determinístico devolvido na hora.
- **Saúde passiva** (§7 e `llm-provider-fallback.md` § Saúde): estado por papel sem sondagem, mensagem truncada em 1024 caracteres, dica de nova tentativa, `llm_candidates` no `/admin/status` e as linhas do `ai-memory status`.
- **`llm-test`** (`llm-providers.md` §9): `--provider` canônico, `--model`, `--prompt`, `--structured`, `--base-url`, `--api-key` (precede o ambiente); resolve o endpoint como o servidor; temperatura representativa 0,2.

### Fora do escopo (explicitamente)
- Provedores por assinatura ou login (`anthropic-oauth`, `openai-oauth`, `codex`, `copilot`): feature 018.
- Embeddings e reranker: features 017 e 019.
- Qualquer consumidor (consolidação, `answer`, dream, auto-melhoria): features 019 a 022.

## Comportamento Esperado

### Cenário Principal (Happy Path)
1. Com `llm_provider = "openai-compat"`, base e modelo configurados, `ai-memory llm-test --provider openai-compat --model m --base-url http://127.0.0.1:11434/v1 --structured --prompt "..."` devolve um objeto JSON.
2. Com um primário e um fallback, um 503 do primário abre o circuito dele por 30 s e a resposta vem do fallback; `ai-memory status` mostra a cadeia com `(last selected)` no fallback.

### Cenários Alternativos
- **Erro determinístico (401, 422):** devolvido na hora, sem tentar o próximo candidato.
- **Todos os circuitos abertos:** erro agregado com N = 0.
- **`llm_headers` com `authorization`:** falha na carga, citando a posição da entrada.
- **Fallback com `api_key_env` apontando para variável vazia:** a carga passa; o `serve` recusa iniciar.

## Edge Cases
- `openai-compat` em modo estrito recebendo 400 que cita `response_format`: segunda chamada no modo tolerante com o mesmo `x-request-id`.
- Resposta tolerante com bloco `<think>` e cerca ```` ```json ````: extraída corretamente.
- Corpo de erro de 2 MiB: mensagem "resposta do provedor excedeu N bytes".
- Modelo Anthropic com alias sem versão: nenhum campo de esforço ou temperatura alterado.

## Constraints
- **INV-07** (só JSON Schema), **INV-13** (nada muda sem provedor), **INV-14** (autenticação antes da construção).
- **Tecnologia:** `net/http`, `encoding/json`, `reflect` (derivação do esquema) da stdlib; `github.com/google/uuid` (já no `go.mod`) para UUIDv7. Sem dependência nova.
- **Segurança:** segredos e valores de cabeçalho nunca em log; resumo da cadeia só com rótulos, classe e status.

## Critérios de Sucesso
- [ ] Cada provedor testado contra servidor falso (protocolo, saída estruturada, esforço, temperatura, cabeçalhos)
- [ ] Tabela de classificação de erros e a cadeia com circuito cobertas por teste
- [ ] Validações de carga de `llm_*` e `llm_fallbacks` cobertas por teste
- [ ] Suíte inteira passa sem provedor configurado
- [ ] `go vet ./...`, `go test -race ./...` e `golangci-lint run` passam sem apontamentos

## Abordagem Escolhida
Pacote `internal/llm` sem dependência de `storage` nem de `wiki`: interface `Provider`, wrapper tipado, um arquivo por provedor, cadeia e saúde. A configuração resolve credenciais e entrega material tipado (`internal/config`). O servidor monta o provedor (ou a cadeia) uma vez no boot.

## Dependências
- 002, 015; `docs/llm-client.md`, `llm-providers.md`, `llm-provider-fallback.md`, `architecture-llm.md` § Provedores; `docs/cli.md` §2.2 e §2.3.

## Checklist de Prontidão
- [x] Propósito claro e sem ambiguidade
- [x] Escopo delimitado (dentro/fora)
- [x] Cenários cobrem happy path e alternativos
- [x] Edge cases identificados com comportamento esperado
- [x] Constraints explícitas
- [x] Critérios de sucesso mensuráveis
- [x] Abordagem escolhida justificada

## Correções da auditoria

Auditoria de 2026-10-20/22 contra o comportamento original (o relatório dessa rodada não está mais em `reports/`). Os docs foram corrigidos; esta spec passa a exigir:

- **Classes de erro:** além das listadas, `serde` (rótulo literal), `truncated-response` (resposta estruturada com `finish_reason=length`) e `empty-content` (2xx sem conteúdo). Ambas são terminais, livres de conteúdo, e o modo estrito do `openai-compat` não recorre ao tolerante para elas. Textos de erro literais em `llm-client.md` §3.1.
- **Corpo de erro:** segredos são redigidos e sequências de controle removidas **antes** do corte em 1 KiB; marcador de leitura `<failed to read provider error body: ...>`; excesso vira `provider response exceeded <N> bytes`.
- **Redirecionamentos:** só dentro da mesma origem (máx. 10); endpoints de credenciais exigem https (loopback pode ser http).
- **Base URL:** a base explícita do ai-memory vale para todos os provedores HTTP (inclusive `anthropic` e `openai`); só o `LLM_BASE_URL` sem prefixo se restringe a `openai-compat` e `opencode`. O texto anterior do catálogo estava errado.
- **Anthropic:** Haiku só recebe campos de esforço a partir de 5.5; escolha de ferramenta forçada não vale para Opus/Sonnet >= 5.5 e Fable/Mythos >= 5.1 (modo automático com leitura tolerante do texto); `thinking` tem os modos `adaptive`, `disabled` e `between_tools` (Sonnet >= 5.5 com `none`); temperatura removida em qualquer família >= 4.7; reconhecimento aceita prefixos de Bedrock/Vertex.
- **Cabeçalhos extras:** na variável de ambiente as entradas são separadas por vírgula (TOML: lista); numeração 1-based que conta entradas vazias; textos de erro literais.
- **OpenAI:** montagem de URL (`v<1-2 dígitos>`), famílias `gpt-5/6`, `o1/o3/o4` com `max_completion_tokens`; normalização de schema detalhada (`$ref` isolado, `oneOf` -> `anyOf`, `required` completo).
- **`openai-compat`:** chave ausente = `Bearer dummy`; normalização feita pelo próprio provedor; condição de recaída inclui `structured-output`; limpeza do tolerante inclui tag de fechamento avulsa (DeepSeek-R1/QwQ/Qwen3) e cerca de código; `AI_MEMORY_LLM_COMPAT_DISABLE_THINKING` (`chat_template_kwargs`); `HTTP-Referer`/`X-Title` literais do OpenRouter.
- **Gemini:** URL e corpo exatos; profundidade 16 conta aninhamento, não só `$ref`; chaves removidas por nome em qualquer objeto (inclusive propriedades de dados com esses nomes); regras de `thinkingBudget: 0`.
- **`opencode`:** listas de modelos por protocolo transcritas por inteiro; "base Go" definida pelo caminho da URL; rota Anthropic usa `x-api-key`; rota Responses sem stream e sem temperatura.
- **Cadeia:** resumo `provedor/modelo: classe status` (sem colchetes); perfis `llm_fallbacks` não herdam base nem chave; variável `api_key_env` ausente vira pendência mesmo em provedores com credencial própria; dica de nova tentativa do status é `ai-memory llm-test --provider <p> --model <m>[ --base-url <b>] --prompt ping`.
- **`llm-test`:** `--model` e `--prompt` obrigatórios; schema fixo `answer`; formato de saída em `llm-providers.md` §9; aceita também `cursor`.
- **Provedor `cursor`** (CLI `agent` do Cursor) existia no original e não estava nos docs: agora documentado (`llm-client.md` §5.10). Entra na 018 (assinatura), mas o nome e o modelo padrão fazem parte do catálogo da 016.

### Status
Os docs agora bastam para reimplementar (não há plano nem tasks). `AI_MEMORY_LLM_HEADERS` pela variável de ambiente: entradas separadas só por vírgula, como no original (resolvido: reproduzir o original; a quebra de linha como separador extra, permitida por `docs/llm-client.md` §2.5, não é adotada).
