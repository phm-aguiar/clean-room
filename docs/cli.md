# CLI `ai-memory`: servidor, cliente e integração com agentes

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

> **Papel:** o executável `ai-memory` é o ponto de entrada único e cumpre três funções:
>
> 1. **`serve`**: o servidor. Monta o armazenamento, a wiki, o protocolo MCP, os hooks,
>    a administração, a interface web, a autenticação e os agendadores de manutenção e
>    consolidação.
> 2. **Cliente fino**: dezenas de subcomandos que chamam a API administrativa
>    (`/admin/*`) do servidor por HTTP e apresentam o resultado (`status`, `search`,
>    `backup`, `lint`, `bootstrap`, ...). Algumas operações são locais; a seção 9 indica
>    quais.
> 3. **Cliente de hooks e lançador**: captura nativa de eventos com fila local (*spool*),
>    instalação da integração em cada agente (`install-hooks`, `install-mcp`,
>    `install-instructions`) e lançamento gerenciado de agentes (`run`).
>
> A leitura das variáveis de ambiente do processo é concentrada neste executável.
> A confirmar: nenhum outro componente lê variáveis de ambiente diretamente.

Documentos relacionados: [harness-adapters.md](harness-adapters.md) (adaptadores de
agentes), [managed-workstreams.md](managed-workstreams.md) (workstreams gerenciados),
[marker-file.md](marker-file.md) (arquivo de marcação do repositório),
[lifecycle-event-ingestion.md](lifecycle-event-ingestion.md) (ingestão de eventos),
[https-via-proxy.md](https-via-proxy.md) (TLS por proxy),
[lifecycle-ops.md](lifecycle-ops.md) (operações de ciclo de vida) e
[design-boot-backfill.md](design-boot-backfill.md) (importação no boot).

---

## 1. Fluxo de execução

1. Leitura dos argumentos globais (`--data-dir`, `--config`) e do subcomando.
2. **Caminho rápido, sem configuração nem log:** o subcomando de hook, a drenagem da
   fila de hooks (`hook-drain`) e a geração de *completions*. Hooks disparam a cada
   chamada de ferramenta, então devem ser baratos e escrever **somente** JSON na saída
   padrão. *Completions* emitem texto puro.
3. Carregamento da configuração, uma vez por execução (seção 2).
4. Inicialização do log (seção 11).
5. Despacho para o comando. Os comandos `run`, `show`, `continue` e `resume` devolvem um
   código de saída; se ele for diferente de zero, o processo termina com esse código.
6. O runtime assíncrono é encerrado sem esperar tarefas pendentes (tempo de espera zero).
   Motivo: a leitura da entrada padrão do transporte stdio é bloqueante e não pode ser
   cancelada. Sem isso, o processo ficava preso após Ctrl-C.

---

## 2. Configuração

### 2.1 Camadas

Da menor para a maior precedência:

1. Padrões internos.
2. Arquivo `config.toml` no diretório de dados, ou o caminho passado em `--config`.
3. Variáveis `AI_MEMORY_*`. Para aninhar chaves, usa-se `__` como separador.
4. Sobreposições explícitas: `AI_MEMORY_AUTH_TOKEN`, `AI_MEMORY_SERVER_URL`,
   `AI_MEMORY_ADMISSION_WEBHOOKS_JSON` e `--data-dir`.

O ambiente que não é chave de configuração (diretório pessoal do usuário,
`AI_MEMORY_HOME`, identificador de sessão do agente, chaves de provedores, diretório
atual) é capturado **uma única vez**. O diretório pessoal é, nesta ordem: `AI_MEMORY_HOME`,
`HOME`, e no Windows `USERPROFILE` (o Windows nativo muitas vezes não tem `HOME`). Para comparações
de prefixo, ele é guardado com barras normais e sem barra final.

Variáveis lidas como valor único, fora da regra de `__` (porque o formato delas não cabe nela):

- `AI_MEMORY_SEARCH_FTS_STOPWORDS`: lista separada por vírgula que substitui
  `[search.fts] stopwords`. Cada item é aparado e itens vazios são descartados. Variável ausente,
  vazia ou só com espaços **não** muda a configuração (para desligar o filtro, use `stopwords = []`
  no arquivo). Um valor só com vírgulas (`" , , "`) vira lista vazia explícita, que desliga o filtro;
- `AI_MEMORY_ADMISSION_WEBHOOKS_JSON`: lista JSON de webhooks que substitui a do arquivo;
- `AI_MEMORY_EMBEDDING_QUERY_PREFIX` e `AI_MEMORY_EMBEDDING_DOCUMENT_PREFIX`. Depois disso, não há novas leituras de ambiente,
exceto as exceções documentadas: o diretório de backup e as variáveis de tempo dos
hooks (seção 5).

### 2.2 Validações na carga

As validações ocorrem na carga da configuração, para falhar no início e não horas
depois:

- `decay.breadth_weight` deve ser finito e maior ou igual a zero.
- Cada meia-vida em `decay.half_life_days` (por camada) deve ser finita e maior que zero.
- `decay.observation_retention_days` deve ser maior ou igual a zero.
- `decay.observation_prune_batch` deve ser maior que zero.
- Limites do filtro de entropia são validados (faixas numéricas).
- `search.fts.stopwords`: no máximo **2000** itens; cada item é aparado e não pode ficar vazio, ter mais
  de **64** caracteres Unicode, nem ter espaço interno (um item com duas palavras nunca casaria com um
  token). Ver [storage-and-retrieval.md](storage-and-retrieval.md) §11.2.
- `consolidation.max_input_tokens` deve deixar pelo menos **20 040 caracteres** efetivos (`floor(max_input_tokens × 3 × input_token_safety_margin)`), ou seja, **8350** com a margem padrão 0,8 (6680 com margem 1,0); `consolidation.input_token_safety_margin` deve estar em (0, 1]. Ver [consolidation-llm.md](consolidation-llm.md) §3.3.
- `consolidation.max_output_tokens` deve ser maior ou igual a **1000**.
- `llm_timeout_secs` deve ser maior ou igual a **1**.
- Cabeçalhos extras do cliente LLM (`llm_headers`) devem ser válidos e não podem
  sobrescrever cabeçalhos que o próprio sistema controla.
- Segredos de autenticação devem ser coerentes entre si (seção 2.4).
- **Cada entrada de `llm_fallbacks` é construída e descartada na carga.** Uma
  configuração ruim não pode ficar dormente até o provedor principal falhar. Credencial
  ausente não reprova a carga: é registrada como pendência (`llm_fallback_unresolved`) e
  só falha no comando `serve` e na montagem da cadeia de provedores.

### 2.3 Campos principais

| Campo | Padrão | Comportamento |
|---|---|---|
| `bind` | `127.0.0.1:49374` | Endereço de escuta HTTP. |
| `tcp_keepalive_secs` | `60` | Keepalive aplicado a cada conexão aceita (seção 3). |
| `server_url` | `http://127.0.0.1:49374` | URL usada pelos clientes. |
| `base_path` | vazio | Prefixo de todas as rotas. |
| `allowed_hosts` | `localhost`, `127.0.0.1`, `::1` | Lista de `Host` aceitos; defesa contra *DNS rebinding*. |
| `cors_allow_origins` | vazio | Origens CORS permitidas. |
| `llm_provider`, `llm_model`, `llm_base_url` | ausentes (LLM desligado) | Provedor e modelo do cliente LLM ([llm-providers.md](llm-providers.md)); sem `llm_model`, vale o modelo padrão do provedor (obrigatório em `openai-compat`). |
| `llm_compat_strict` | `true` | Modo estrito de compatibilidade do cliente LLM. |
| `llm_timeout_secs` | `300` | Tempo máximo por requisição LLM. |
| `llm_reasoning_effort` | ausente | Esforço de raciocínio do provedor (omitido = padrão do modelo). |
| `llm_headers` | vazio | Cabeçalhos extras (validados na carga). |
| `llm_fallbacks[]` | vazio | Provedores de contingência, cada um com provedor, modelo e, opcionalmente, URL base e variável de ambiente da chave. |
| `reranker` | desligado | Único valor aceito: `llm` (sem diferenciar maiúsculas e minúsculas). Qualquer outro valor é erro. |
| `embedding_provider`, `embedding_model`, `embedding_dim`, `embedding_base_url`, `embedding_query_prefix`, `embedding_document_prefix` | ausentes (embedder local em melhor esforço; `none`/`off`/`disabled` desligam) | Configuração do embedding ([local-embeddings.md](local-embeddings.md), [llm-client.md](llm-client.md) §8). Os prefixos não são aparados. |
| `consolidate_on_session_end` | `false` | Liga a consolidação ao fim de sessão (seção 4). |
| `capture_assistant` | `false` | Captura também as respostas do assistente. |
| `backfill_on_start` | `true` | Importa o histórico local no primeiro início de projeto vazio. Desligável por configuração ou por variável de ambiente. |
| `run_autowire` | `true` | Instala hooks e MCP automaticamente ao lançar um agente (seção 7). |
| `strip_root_combinators` | `false` | Remove combinadores na raiz dos esquemas de ferramentas na listagem de ferramentas (`tools/list`), para todos os clientes. Destinado a provedores estritos que rejeitam esses combinadores. A validação em tempo de execução continua ativa. |
| `gemini_safe_schemas` | `false` | Ativa o modo de esquemas compatível com Gemini. Implica o item anterior. |
| `hook_rate_per_sec`, `hook_rate_burst` | `0` | Limite de taxa para hooks. A confirmar o significado exato do valor `0`. |

Seções agrupadas: `[decay]`, `[maintenance]`, `[dream]`, `[retrieval]`, `[slots]`,
`[consolidation]`, `[auto_improve]` (com subseções de avaliação e agendador),
`[sanitize]`, `[auth]`, `[auto_scope]`, `[routing]` e `[[admission_webhooks]]`
([admission-webhooks.md](admission-webhooks.md)).

### 2.4 Autenticação

Campos da seção `[auth]`: token bearer, usuário, e-mail e nome do usuário raiz,
par emissor/assunto OIDC do usuário raiz, *pepper* de tokens, token de proxy de
identidade, token de recuperação, indicador de cookie seguro e lista de redes de proxy
confiáveis. A representação textual da configuração deve redigir segredos (a confirmar).

Caminhos derivados de arquivos de credenciais: token de autenticação, token OAuth do
provedor OpenAI, arquivo de autenticação do Codex, token do Copilot e token do
dispositivo OIDC.

**Token de hook persistido:** fica em arquivo com permissão `0600` no diretório de dados.
O bearer **não** aparece na linha de comando do hook.

### 2.5 Diretório de dados

- **Precedência:** `--data-dir` > `AI_MEMORY_DATA_DIR` > chave `data_dir` do `config.toml` > padrão.
- **Padrão**, a pasta `ai-memory` dentro do diretório de dados local do usuário:
  - **Windows:** `%LOCALAPPDATA%\ai-memory` (pasta conhecida "Local AppData", em geral
    `C:\Users\<usuário>\AppData\Local\ai-memory`);
  - Linux: `$XDG_DATA_HOME/ai-memory`, ou `~/.local/share/ai-memory`;
  - macOS: `~/Library/Application Support/ai-memory`;
  - se o sistema não informa esse diretório: `./ai-memory`, relativo ao diretório atual.
- **Onde o `config.toml` é procurado:** em `--config`, se informado; senão em `<dir>/config.toml`, com
  `<dir>` = `--data-dir`, ou `AI_MEMORY_DATA_DIR`, ou o padrão. A chave `data_dir` escrita **dentro**
  do arquivo muda o diretório de dados, mas não muda onde o próprio arquivo foi procurado. Arquivo
  ausente não é erro: valem os padrões.
- **Canonicalização:** o caminho final é convertido para absoluto e canônico (links resolvidos). Se
  ele ainda não existe, canonicaliza-se o diretório pai e o último componente é acrescentado; se nem
  o pai existe, o caminho fica como foi dado. O resultado é registrado no log no início do servidor.
  - **Windows:** a forma canônica tem o prefixo de caminho estendido `\\?\` (por exemplo
    `\\?\C:\Users\me\AppData\Local\ai-memory`). Ela é usada internamente. Onde um caminho é
    mostrado ao usuário ou gravado na configuração de outro programa (comandos de hook, por exemplo),
    o prefixo é removido: `\\?\C:\…` vira `C:\…` e `\\?\UNC\servidor\…` vira `\\servidor\…`
    (o `UNC` sem distinguir caixa). Outras formas com `\\?\` ficam como estão.
- Layout: `wiki/`, `raw/`, `db/`, `models/`, `logs/`, o arquivo `config.toml`, a pasta
  `hook-spool/`, `auth.json` (credenciais OIDC do cliente), `client-projects.json`
  (registro local de checkouts), `autowire-state/` e o arquivo de trava `.serve.lock`.

---

## 3. Comando `serve`: sequência de boot e transporte

### 3.1 Ordem de boot

A ordem importa.

1. Instalação dos tratadores de sinal, antes de qualquer operação lenta. Um sinal
   recebido durante backup ou migração precisa ser capturado.
2. Validação do diretório da interface web, das credenciais de contingência do LLM e
   das origens CORS.
3. **Trava de instância única** (`.serve.lock`, com informação do detentor). Um segundo
   `serve` sobre o mesmo diretório seria um segundo escritor, um segundo processo git e
   um segundo ponteiro de projeto ativo. A opção `--force` ignora a trava.
4. **Backup pré-migração**, antes de abrir o armazenamento. O arquivo precisa conter o
   esquema anterior para que o *rollback* seja possível.
5. Abertura do armazenamento (aplica as migrações do banco) e configuração da
   recuperação de busca herdada por todos os clones de leitura.
6. Limpeza de caminhos de repositório que viraram "catch-all" (diretório pessoal, raiz
   do sistema ou um diretório que não é raiz de repositório git).
7. Validações de autenticação contra o estado do armazenamento: *pepper* versus
   credenciais existentes, colisão de segredos configurados, bootstrap do usuário raiz,
   modo humano, token de recuperação, usuários existentes versus autenticação
   configurada, proxy confiável e redes CIDR.
8. **Migrações de estrutura da wiki**, antes do observador de arquivos.
9. Criação do workspace e do projeto padrão (`default` e `scratch`, ou os valores de
   `--workspace` e `--project`).
10. Montagem do sanitizador (expressão regular inválida aborta), da wiki, da cadeia de
    admissão (se houver webhooks) e do embedder. Na montagem do embedder, divergências
    do trio de identificação do modelo são validadas e geram aviso. Se necessário, o
    modelo local é baixado.
11. Preenchimento idempotente de manifestos `_meta.md` e reparo de `stale_after`
    (não fatais). Em seguida, **ponto de controle git de linha de base**. Falha de
    verificação de propriedade do repositório, típica de serviço Windows rodando com
    outro usuário, é registrada como erro alto, porque silencia o histórico git.
12. Início do observador de arquivos (desligado com `--no-watcher`).
13. Projeto ativo, com modo, tempo de vida e número máximo de entradas definidos pelo
    isolamento automático ([auto-scope.md](auto-scope.md)).
14. Montagem do servidor MCP com seus componentes (wiki, decaimento, retenção,
    auto-melhoria, projeto ativo, sanitizador, proxy de identidade, slots por usuário e
    esquemas), do consolidador (se houver provedor LLM), do reranqueador e da saúde do
    provedor.
15. Início dos agendadores (seção 4).
16. Transporte (seção 3.2).

### 3.2 Transporte

- **stdio** (padrão, `--transport stdio`): o servidor MCP atende a entrada e saída
  padrão e corre contra o sinal de encerramento.
- **HTTP** (`--transport http`):
  - semeia a leitura do projeto ativo mais recente;
  - inicia o trabalhador de consolidação de fim de sessão, se habilitado;
  - o endpoint MCP opera em **modo sem estado com resposta JSON por padrão**.
    `--http-stateful` restaura sessão e fluxo de eventos (SSE);
  - aplica a lista de `Host` permitidos e monta as rotas de hooks, workstreams,
    administração, autenticação e interface web.

Tamanhos: o corpo de requisição de MCP, hooks e handoff é limitado a **10 MiB**. As
rotas administrativas aceitam até **32 MiB**, para a importação de bootstrap.

### 3.3 Roteamento HTTP

```text
máquina   = /mcp (MCP) + /hook + /hook/batch + /handoff + /workstream/*
            limite de corpo 10 MiB, autenticação por bearer
admin     = /admin/*  limite de corpo 32 MiB (bootstrap), autenticação dupla
roteador  = máquina
          + /healthz
          + admin
          + autenticação pública (/auth/login, /auth/recovery)
          + autenticação de sessão (/auth/me e demais rotas de sessão)
          + autenticação interna
          + interface web protegida (/api/v1 e wiki embutida), autenticação dupla
          + interface web pública (SPA)
→ filtro de Host (allowed_hosts) → prefixo base_path → /favicon.ico na raiz do host
```

A confirmar: lista completa das rotas de sessão e de autenticação interna.

### 3.4 Exposição de rede

- Bind em endereço que não é de loopback **sem autenticação é recusado**, a menos que
  `--allow-insecure-no-auth` seja passado explicitamente.
- Em contêiner, onde o endereço de bind não revela a exposição real, é emitido aviso.
- Um banner é escrito na saída de erro sempre que a exposição não for segura. O aviso
  estruturado de log também é emitido.
- Autenticação ativa em endereço que não é de loopback com HTTP sem TLS gera aviso
  para colocar um proxy TLS na frente ([https-via-proxy.md](https-via-proxy.md)).
- Autenticação de usuário humano sobre HTTP simples só é aceita em loopback, porque
  cookies não seguros não devem trafegar em rede.

### 3.5 Conexões e encerramento

- **Keepalive por conexão:** cada conexão aceita recebe keepalive de socket com o
  intervalo de `tcp_keepalive_secs` (padrão 60; 0 desliga). O mesmo valor é o tempo
  ocioso antes da primeira sonda **e** o intervalo entre sondas; falha ao ajustar um
  socket só gera aviso e a conexão segue. Isso evita acúmulo de descritores de pares
  mortos até o limite do sistema. Detalhes em [windows-native-extras.md](windows-native-extras.md) §11.
- **Encerramento:** há uma **janela de graça de 5 s** para drenar conexões. Clientes com
  fluxo de eventos ou sessão não se encerram sozinhos. O trabalhador de consolidação
  também é limitado por essa janela. Escritas pendentes no escritor podem ser
  descartadas (a confirmar).

---

## 4. Agendadores de manutenção

Cada tarefa roda em segundo plano. Tarefas globais (varredura de esquecimento e lint de
regras) **persistem o último sucesso** no armazenamento. Na inicialização, calcula-se o
atraso restante até a próxima execução, com teto de **60 s** no atraso de inicialização.
Assim, reinícios não reexecutam a tarefa e um período longo de indisponibilidade não a
faz deixar de rodar.

Em falha, a nova tentativa usa atraso limitado pelo intervalo da tarefa e pelo teto de
60 s.

| Tarefa | Gatilho | O que faz |
|---|---|---|
| Varredura de esquecimento | manutenção habilitada e intervalo maior que zero | Enumera **todos os escopos** e executa a varredura com parâmetros de decaimento, amplitude, retenção de observações, compactação e deduplicação. Falha em um escopo faz a tarefa falhar e ser repetida. |
| Varredura de projetos vazios | manutenção habilitada | 60 s após o início e depois **uma vez por dia**. Remove projetos sem dados, criados há pelo menos **7 dias**. Nunca remove o projeto `scratch`, o perfil `_profile` nem o escopo global. Um projeto com páginas, sessões, observações, *handoffs*, workstreams ou registros de auto-melhoria não é considerado vazio. |
| Lint de regras | intervalo de lint maior que zero | Executa em todos os escopos. Inclui o LLM se configurado. |
| Backfill de embeddings (início) | há embedder | Uma passada em segundo plano no boot, para páginas antigas receberem vetor. |
| Backfill de embeddings (periódico) | intervalo maior que zero e embedder | Executa em laço. |
| Auto-melhoria | agendador habilitado, intervalo, máximo de sessões por ciclo maiores que zero e **LLM presente** | Inicializa os escopos no boot. O laço **dorme após cada ciclo**, então ciclos nunca se sobrepõem. Experiência: a cada N sessões, se configurado. |
| Dream (consolidação em ociosidade) | `[dream]` habilitado, **e** LLM, **e** embedder | A cada intervalo, só executa se o operador estiver ocioso há pelo menos `idle_window_secs`. Um observador checa a cada **2 s** e cancela a execução se houver atividade. |
| Consolidação de fim de sessão | `consolidate_on_session_end` e LLM | Consulta a fila a cada **15 s** ou quando notificado. Cada job tem uma **concessão (lease) de 10 min**. Fluxo: reivindicar, consolidar, e então concluir, ou falhar com recuo, ou liberar. Máximo de **5 tentativas**. Esgotadas as tentativas, a página heurística permanece. |

---

## 5. Cliente de hooks nativo

### 5.1 Problema e solução

Um hook por chamada de ferramenta **não pode** bloquear o agente nem perder eventos
quando o servidor está lento ou remoto. A solução é uma **fila local (spool)** com
drenagem em pontos de fronteira.

### 5.2 Fluxo do comando `hook`

Invocação: `ai-memory hook --event <evento> --agent <agente> --server-url <url>`, com
`--auth-token`, `--check-capture` e flags de captura opcionais. O payload é lido da
entrada padrão. Ordem de processamento:

1. **Parse do JSON** (marca de ordem de bytes UTF-8 removida). Se falhar, escreve aviso na
   saída de erro, responde sucesso ao agente e não captura nada. Um hook nunca derruba o
   agente.
2. **Filtro de evento por agente.** Para o Antigravity, só a invocação de número zero
   vira `session-start`. Para o Cursor, descarta a cópia redundante quando os hooks
   nativos do próprio Cursor estão instalados.
3. **Captura do assistente** (se `--capture-assistant`) ou remoção defensiva do campo
   bruto. Depois, o corpo de eventos de ciclo de vida é reduzido ao necessário para o
   cliente.
4. **Política de captura**, aplicada só a eventos de ferramenta. O modo de captura é
   resolvido nesta ordem: flag, valor persistido, padrão `Denylist`.
   - Em modo **Allowlist** sem arquivo de marcação no repositório, **nada** é emitido,
     nem mesmo prompts.
   - A decisão pode resultar em descartar o evento, substituir o corpo por apenas
     metadados, ou manter o evento com um campo indicando a política aplicada.
    - `--check-capture` imprime a decisão em JSON (sem capturar, sem usar spool ou rede), com 15 campos fixos (`version`, `capture_mode`, `marker_present`, `admits_capture`, `policy_admits_capture`, `event_admits_capture`, `policy_state`, `tool_family`, `path_count`, `disposition` [que reporta `"drop"` também quando a disposição interna é `MetadataOnly`], `extraction_state`, `scope`, `scope_resolution`, `server_profile`, `server_resolution`) e `external_capture: true` opcional. Serve para diagnóstico.
5. **Montagem da requisição:** diretório atual, identificadores do marcador do
   repositório (espaço, projeto, estratégia de nome de projeto, descarte de capturas de
   subagentes, escopo padrão, briefing), identificador de sessão e identificador de
   execução gerenciada (variável `AI_MEMORY_RUN_ID`). Agentes sem identificador estável
   de sessão usam estado persistido em disco.
6. **Chave de ingestão (UUID) gerada uma única vez** e incluída na query string da `url` gravada no item da fila (`ingest_key=<uuid>`). Toda
   nova tentativa reenvia a mesma URL, o que torna a ingestão idempotente.
7. **Escrita na fila local**, instantânea, em arquivo temporário `.json.tmp` renomeado para `{created_ms:013}-{pid}-{seq:016x}-{recovery_key}.json` (com permissão `0600` em Unix). O JSON do item contém `{ url, body, created_ms, auth_mode, token?, attempts, profile? }`. A
   autenticação é guardada por item (`auth_mode`): token estático embutido (`static`), credencial OIDC resolvida
   e renovada na drenagem (`oidc`), ou anônimo (`none`); esses são os três valores gravados no campo `auth_mode` do arquivo.
8. **Drenagem incremental:** em evento de uso de ferramenta pós-execução, se a fila tiver
   pelo menos **32** itens (configurável), drena-se com orçamento de **250 ms**. A fila
   não cresce indefinidamente, e a chamada de ferramenta nunca espera a rede por muito
   tempo.
9. **Início de sessão:** dispara a importação de histórico em segundo plano na primeira
   vez em um checkout. Drena a fila com orçamento de **3 s**. Só para agentes que
   injetam na sessão o que é impresso na saída padrão do início de sessão, faz
   `GET /handoff` com timeout de **3 s** e imprime o envelope que o agente espera (para Claude Code: `{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"..."}}` quando há contexto, ou `{}` quando vazio).
   Atenção: essa leitura de handoff é **destrutiva** no servidor. Agentes que não
   injetam esse conteúdo recebem o handoff sob demanda, por ferramenta MCP. O agente Kimi recebe o briefing opt-in no primeiro prompt (`UserPromptSubmit`).
10. **Fronteiras** (`stop`, `pre-compact`, `session-end`): dispara um drenador em segundo
    plano (`hook-drain`), com entrada/saída padrão nulas, saída de erro em `logs/hook-drain.log`, trava exclusiva de sistema operacional na fila (`.drain.lock`, esperada por até 30 s; se continuar ocupada, o drenador escreve um aviso em `hook-drain.log` e não envia nada nessa passada) e orçamento de
    **5 min**.
11. Sempre responde sucesso ao agente.

### 5.3 Fila local (spool)

Diretório: `<diretório de dados>/hook-spool/`.

| Parâmetro | Valor |
|---|---|
| Máximo de tentativas por evento | **8** (padrão em `[hook_spool].max_attempts` ou `AI_MEMORY_SPOOL_MAX_ATTEMPTS`; `0` desativa descarte por tentativas) |
| Idade máxima | **7 dias**. Eventos mais antigos são descartados. |
| Máximo de arquivos na fila | **10 000** (coordenado por `<data_dir>/hook-spool.capacity.lock`, espera de até 50 ms). Ao atingir 10 000 arquivos, a fila existente é preservada e o **novo evento recebido é descartado** na inserção (com aviso em stderr). |
| Lote de envio | Até **256 itens** e **8 MiB** por lote. Sempre pelo menos 1 evento. |

No início da drenagem, arquivos `.json.tmp` órfãos com mais de **5 minutos** (300 000 ms) são inspecionados: se tiverem até 10 MiB, JSON válido e `created_ms` idêntico ao prefixo do nome, são recuperados (renomeados para `.json`); caso contrário, são apagados.

Envio: `POST /hook/batch`. A resposta de confirmação indica quais itens foram aceitos e
qual falhou (com compatibilidade para o formato antigo de confirmação). Apenas os itens
confirmados são apagados. Os que falharam (qualquer resposta que não seja 2xx, `401`, `403`, `404`/`405` ou `429`; um `401` que persiste depois da retentativa; o item apontado por `failed_index`) têm o contador de tentativas incrementado. **Só o `403` descarta o item na hora** (o autor não pode escrever no projeto: reenviar seria recusado para sempre); `400`, `408`, `413`, `422` e os demais `4xx` contam como falha comum. A tabela completa de desfechos está em [downtime-recovery.md](downtime-recovery.md) §4.3.

- Em lote recusado por **401**, a drenagem repete **uma vez** com o token atual que lhe
  foi entregue (`AI_MEMORY_AUTH_TOKEN` ou `<data_dir>/auth-token`). Isso cobre token rotacionado após a captura.
- Respostas de limitação de taxa (**429**), estouro do tempo da requisição no cliente, conexão recusada e falha de DNS (o endereço está morto, não o item) não consomem tentativas. Uma resposta HTTP `408` **consome** (conta como falha comum).
- Endereços gravados nos itens são reapontados para o `server_url` atual antes do envio.
- Um status de saúde da fila (contagens, idade, tentativas, **sem conteúdo**) alimenta
  os comandos `status` e `doctor`.

Variáveis de tempo (únicas leituras de ambiente no caminho do hook). Unidade: **minutos**,
máximo **60**:

- `AI_MEMORY_HOOK_DRAIN_TIMEOUT_MINUTES`
- `AI_MEMORY_HOOK_HANDOFF_TIMEOUT_MINUTES`
- `AI_MEMORY_HOOK_START_BUDGET_MINUTES`
- `AI_MEMORY_HOOK_BACKGROUND_DRAIN_BUDGET_MINUTES`
- `AI_MEMORY_HOOK_INCREMENTAL_THRESHOLD` (quantidade de itens, padrão 32)

### 5.4 Scripts de shell

Alternativa POSIX e PowerShell que não depende do binário no caminho quente. Lêem a
entrada padrão, extraem o diretório atual e o identificador de sessão, sobem a árvore de
diretórios procurando o arquivo de marcação e enviam o evento ao endpoint de hook. Para
`session-start`, fazem a leitura de handoff e imprimem JSON. Têm **o mesmo contrato**
do hook nativo, verificado por testes de paridade.

A confirmar: valores de timeout de rede e código de saída dos scripts.

---

## 6. Arquivo de marcação `.ai-memory.toml`

Arquivo no repositório que declara a identidade do projeto. Campos: espaço de trabalho
(`workspace`), projeto (`project`), estratégia de nome (`project_strategy`:
`basename` ou `repo-root`), descarte de capturas de subagentes
(`drop_subagent_captures`), escopo padrão de leitura (`[recall] default_global`),
briefing de início (`[briefing] inject_on_session_start` e `max_chars`) e caminhos
ignorados na captura (`[capture] ignore_paths`).

Busca: a partir do diretório atual, sobe até o diretório pessoal do usuário. Se o início
estiver fora do diretório pessoal, sobe até a raiz do repositório git (ou só o próprio
diretório, se não houver repositório).

- A leitura é **linha a linha**, espelhando os scripts de shell. Só a seção `[capture]`
  é lida como TOML completo.
- Um marcador que só contém `[capture]` é **transparente para o escopo**: não redefine
  espaço e projeto em relação a um marcador mais acima.
- Usado pelo hook e por todos os comandos cliente. Antes, só o hook o lia, e os comandos
  `run`, `bootstrap` e `search` caíam no projeto padrão, dividindo um mesmo checkout em
  dois escopos.

### 6.1 Perfis de servidor

Permitem roteamento **por repositório** da captura de hooks. Uma máquina registra vários
servidores com nomes locais. O arquivo de marcação do repositório escolhe um deles pelo
nome.

O arquivo de marcação é conteúdo versionado, portanto **entrada não confiável**. Por isso
ele carrega apenas um **nome**. Um repositório clonado escolhe entre servidores que o
operador registrou. Ele nunca introduz um destino novo nem tem acesso a credenciais.

**Arquivos:**

- Registro `servers.toml` no diretório de dados, com no máximo **64 KiB**. Contém apenas
  seções `[servers.<nome>]` com `url` e `roots`. Chave desconhecida, tipo errado ou outra
  tabela de topo é erro. Um erro de digitação só faz o roteamento **recusar**, nunca
  adivinhar. Arquivo ausente equivale a registro vazio.
- Tokens, **um por arquivo**, em `auth-tokens/<nome>` no diretório de dados, com
  permissão `0600`. Nunca ficam no registro, em configuração de hook renderizada nem na
  linha de comando.

**Nome de perfil:** `[a-z0-9][a-z0-9_-]{0,63}`. Não pode ser nome reservado de dispositivo
do Windows (`nul`, `con` etc.), recusado em todas as plataformas para o registro ser
portável. O nome vira nome de arquivo, então um marcador com caminho relativo não chega
ao sistema de arquivos como caminho.

**Validações:** `url` só aceita `http` ou `https` com host; barras finais são removidas.
Cada `root` deve ser absoluto ou começar com `~/`.

**Resolução** (para um nome, diretório do marcador e diretório pessoal):

1. Nome válido.
2. Registro carregável.
3. Perfil existe.
4. Se o perfil **não tem `roots`** e há mais de um perfil registrado, o resultado é
   `RootsRequired`.
5. Se o perfil tem `roots`, o diretório do marcador que declarou a chave deve estar
   sob algum root, **comparando componente por componente**. Exemplo: o root
   `~/work/team-b` aceita `~/work/team-b/repo`, mas não `~/work/team-bb`. Caso contrário,
   o resultado é `OutsideRoots`.
6. Leitura do token (quebra de linha final removida). Token vazio equivale a ausente,
   resultado `NoToken`.

**Busca sem checagem de roots:** existe para quem age sobre uma seleção já validada pelo
hook (importação disparada pelo hook e drenagem da fila).

**Falha fecha, nunca recai.** Qualquer erro gera uma recusa e o evento é **descartado**.
Recair para o endereço padrão da instalação entregaria a captura de um time ao servidor
de outro, exatamente o que perfis existem para impedir. Rótulos estáveis, sem segredo,
aparecem em `--check-capture` e nos avisos: `rejected-invalid-profile-name`,
`rejected-invalid-registry`, `rejected-unknown-profile`, `rejected-no-token`,
`rejected-roots-required`, `rejected-outside-roots` e `rejected-unreadable-marker`
(marcador existe na subida mas não pode ser lido).

**Escrita (`add` e `remove`):**

- `add` valida o registro **antes** de reescrevê-lo, e recusa montar sobre registro que o
  hook já rejeitaria. Preserva comentários e os outros perfis, e grava com escrita
  atômica (arquivo temporário com permissão `0600`).
- Efeito colateral: se a URL mudou **sem token novo**, o token antigo é apagado, para não
  usar o token do servidor A no servidor B. O perfil então recusa eventos até receber um
  token.
- Sem `--root`, mantém os roots já registrados. Assim, rotacionar token não remove a
  restrição.
- `remove` apaga o perfil e o token.
- `clear_tokens` (chamado pela desinstalação) apaga só os tokens e mantém o registro, que
  não tem segredos. Reinstalar lista cada perfil com `token: missing`.

**Comandos (locais, nunca contatam servidor):**

- `server add <nome> --url <url> [--root <caminho>]* [--auth-token <t> | --auth-token-stdin]`.
  Prefira a entrada padrão: o token fica fora do histórico de comandos e da lista de
  processos. As duas formas de token são mutuamente exclusivas, e entrada padrão vazia é
  erro.
- `server list [--json]`: mostra `token: stored` ou `token: missing`, nunca o token.
  Roots vazios aparecem como `(any)`.
- `server remove <nome>`.
- `add` avisa quando há vários perfis e algum não tem root.

---

## 7. Comando `run`: lançamento gerenciado

Fluxo (detalhes de cada agente em [harness-adapters.md](harness-adapters.md)):

1. Inspeção do repositório a partir do diretório atual. Resolução do agente: explícito
   ou automático. No automático, considera sessões locais do checkout cujo executável
   existe.
2. Resolução do escopo pelo arquivo de marcação, depois pedido de preparação ao
   servidor (`POST /workstream/runs`). Se o servidor estiver ocupado, há nova tentativa
   por até **5 s**. Registra o vínculo cliente-local do checkout no registro
   `client-projects.json`. O servidor **nunca** expõe caminhos do computador do cliente.
3. O servidor devolve o agente resolvido, o identificador de sessão nativa (se houver), o
   cursor de origem e as marcas de sincronização.
4. **Instalação automática** de hooks e MCP, uma vez por agente e versão (estado em
   `autowire-state/`), em modo de melhor esforço.
5. Validação de que a sessão ligada existe no armazenamento nativo do agente. Se não
   existir, a sessão é tratada como órfã: começa uma nova e o vínculo é reapontado.
   Adoção interativa de sessão existente só com terminal interativo e workstream vazio.
6. Vínculo da sessão no servidor (`POST …/link`). Contexto para agentes sem injeção de
   início de sessão, recebido por mecanismo do próprio adaptador, com confirmação
   `…/context/accept`.
7. Lançamento do agente com `AI_MEMORY_RUN_ID`, `AI_MEMORY_WORKSTREAM_ID` e
   `AI_MEMORY_HOOK_URL`. Entrada, saída e erro são herdados. **Batimento cardíaco a cada
   30 s** (timeout de 10 s). Interrupções são capturadas.
8. Ao sair: descobre a sessão nativa, exporta o que foi gravado, importa em lotes de
   até **400 eventos ou 1 MiB**, grava checkpoint final e finaliza com nova tentativa. Se
   a finalização falhar, a concessão do workstream é cancelada. Imprime
   `saved N new event(s)`.

**Falha de conexão no início:**

- Se o servidor não responde à sondagem inicial (cerca de 2 s), o agente é lançado em
  **modo degradado**, sem workstream gerenciado.
- Com `--require-server` (ou `run.require_server` na configuração), a falha é erro e o
  agente **não é iniciado**.
- Quando a sondagem passa (qualquer resposta HTTP) mas a preparação falha (conexão perdida, autenticação, erro do servidor, workstream ocupado), o erro é fatal e o agente **não** é iniciado (`opening managed workstream; the agent was not started`).
- Opções completas de `run` (inclui `--profile`, `--env`, `--env-file`, `--require-server`, `--true-yolo`, `--force-unlock`, `--no-autowire`, `--jail`, `--no-jail`), menu pós-execução e `ai-memory recover`: ver [managed-workstreams.md](managed-workstreams.md) e [downtime-recovery.md](downtime-recovery.md).

Comandos de navegação sobre este fluxo: `show`, `continue`, `resume`, `workstreams`,
`rename-workstream` e `workstream-search`. Ver seção 9.1 para `continue`.

---

## 8. Integração com agentes

- **`install-hooks [--agent X] [--apply]`:** sem `--apply`, imprime a configuração de
  hooks do agente. Com `--apply`, **mescla** a configuração no arquivo do agente. Formatos
  suportados: JSON e TOML, plugins ou extensões gerados como scripts de extensão, pacotes de plugin
  e outros. A lista de eventos de cada agente fica em um único lugar.
- **Escrita idempotente** (`apply_atomic`): resolve links simbólicos sem substituí-los.
  Executa a transformação sobre o conteúdo atual. Se o resultado for igual, não toca o
  disco (`NoOp`). Caso contrário, copia o arquivo para `<arquivo>.bak-<timestamp Unix>`
  e grava com escrita atômica (temporário, sincronização, troca e sincronização do
  diretório).
- **`install-mcp`:** imprime ou aplica o registro do servidor MCP no cliente escolhido.
- **`setup-agent`:** extrai scripts de hook de um contêiner Docker para o computador
  do host e imprime a configuração, com prefixo de caminho (`--host-prefix`).
- **`install-instructions`:** insere o trecho de instruções (por exemplo, em arquivos
  de instruções do agente) entre marcadores HTML. É idempotente.
- **`install-skills`:** instala habilidades gerenciadas (Agent Skills).
- **`mcp-bridge`** (oculto): ponte stdio para o servidor HTTP, em dois modos. Com sessão (padrão;
  `--agent claude-code|muse`) envia o identificador de sessão do agente no cabeçalho
  `X-Memory-Actor-Session-Id` e **falha fechado** se ele faltar. Com `--no-session-id` não envia
  identidade de sessão (clientes estáticos). Detalhes em `mcp-install.md`.
- **`auth enable`:** exige bearer no servidor desta instalação e reaponta os clientes (ver `users.md`,
  "Local root bearer token file"); opção `--no-mcp`.
- **`uninstall`:** operação inversa simétrica. Remove **só** o que pertence ao
  ai-memory. Sem `--apply`, é simulação. Com `--purge-data`, apaga dados após
  verificar que nenhum processo ai-memory está ativo.
- **`doctor`:** compara, por agente, as sessões locais com as sessões capturadas pelo
  servidor. Avisa quando um agente rodou no computador mas nenhuma sessão foi capturada,
  com o comando exato para corrigir. Contrato completo:
  - **Opções:** `--workspace`, `--project` (padrão: escopo do diretório atual, pelo marcador),
    `--since-days` (padrão **30**; 0 = toda sessão em disco conta como recente) e `--json`.
    Com `--workspace` e `--project` ambos não vazios, o marcador é ignorado (`bypassed_explicit_scope`);
    um mapa de rotas do diretório pessoal inválido encerra com `invalid home route map; doctor did not
    query a fallback server scope`.
  - **Só leitura.** Nunca abre o banco: o lado capturado vem de `GET /admin/sessions/by-agent`, o lado
    local é a listagem somente leitura dos adaptadores de transcrição (14 harnesses: Claude, Codex,
    OpenCode, OpenCode 2, Pi, Crush, OMP, Kimi, Command Code, Kiro v2, Kiro v3, Grok, Antigravity,
    Copilot), no máximo **1 000** sessões por harness; harness com loja ausente ou ilegível contribui
    com zero. Variantes do mesmo agente (Kiro v2/v3, OpenCode V1/V2) somam numa linha.
  - **Veredito por agente (conservador):** "não capturado" só quando há sessão local **recente** e
    **zero** capturadas. Uma sessão capturada prova que o hook funciona; histórico só antigo não é
    acionável; contagem parcial nunca é lacuna (um `--resume` pode mapear vários arquivos a uma sessão).
    Linhas de agentes que não rodaram aqui nem capturaram nada são omitidas; ordem: não capturados
    primeiro, depois alfabética.
  - **Servidor inacessível:** `capture coverage: unavailable (the server did not answer; captured counts
    are unavailable)`, as contagens capturadas viram "indisponível" (`?`), e há o aviso de que os eventos
    estão sendo enfileirados. Escopo ambíguo no servidor: `server scope is ambiguous; …`. Escopo
    inexistente (404) conta como zero capturadas. Outro erro encerra o comando.
  - **Seções do relatório:** conexão (identidade `GET /identity` ou "unavailable", postura de
    autenticação, aviso se `AI_MEMORY_CAPTURE_OWNER` está ativo, diagnóstico da coordenada do projeto
    `GET /admin/project-coordinate` com candidato canônico/legado e mensagens do marcador); saúde da
    captura (fila de hooks cheia ou com eventos pendentes e idade do mais antigo `Ns|Nm|Nh|Nd`; confiança
    dos hooks do Codex — só consulta o Codex se há hooks do ai-memory instalados; plugins do marketplace
    para Claude Code e Codex com versão defasada, hooks duplicados em arquivo de configuração, habilidades
    duplicadas, servidor MCP duplicado, `disableAllHooks`/`allowManagedHooksOnly` e hooks pulados por
    falta de confiança; arquivo do Muse (hooks ligados ou não, entrada MCP sob `mcpServers` ou
    `mcp_servers`, mapas conflitantes, `schema_version` ausente); resíduos do agentmemory — só detecta,
    nunca altera); janela recente; entrada de logon do Windows que aponta para um executável que não
    existe mais; marcador com seção `[capture]` inválida (falha fechada: o conteúdo de ferramentas de
    arquivo e shell vira só metadados e as exclusões `ignore_paths` **não** valem até corrigir); memória
    nativa do Claude Code (`projects/<projeto>/memory/`, achada pelo conteúdo, a partir da raiz do
    repositório) com contagem de arquivos e estado `excluded`, `not excluded`, `metadata only` ou
    `marker invalid`, mais a nota de que uma instalação por scripts não aplica a política de captura;
    cobertura por agente (`✓`, `⚠` ou `?`, com `<n> local (<m> recent) → <k> captured`); sessões com
    eventos de várias fontes de captura ("misturadas"; não prova captura duplicada).
  - **Frases finais:** sem linhas e sem problema, `No local harness sessions found for this project and
    nothing captured yet.`; tudo capturado, `✓ Every harness that ran in this project recently is
    captured on the server.`; faltando, `⚠ <n> harness(es) ran here recently with no captured sessions:
    <lista>.` seguido de `Install the hook(s) above, then re-run `ai-memory doctor` to confirm.` (com dono
    externo de captura ativo, a orientação é verificar o produtor externo e a política do repositório).
  - **JSON:** objeto com `workspace`, `project`, `server`, `since_days`, `rows[]`
    (`agent`, `local_total`, `local_recent`, `captured`, `uncaptured`, `mixed_capture_sessions`),
    `capture_coverage_problem`, `uncaptured[]`, `marker_capture_problem`, `capture_owner_active`,
    `identity`, `auth_posture`, `project_coordinate`, `native_memory[]`, `spool`, `server_unreachable`,
    `codex_hooks`, `agentmemory_residue[]`, `plugins[]`, `muse` e, só quando há problema, `windows_logon`.
- **`backfill`** (também automático no primeiro início de sessão de um projeto **vazio**):
  reexecuta o histórico nativo do agente pelo mesmo endpoint de lote de hooks. Como a
  entrada é a mesma da captura ao vivo, a sanitização também é a mesma. Usa a data e hora
  original de cada evento. Tem sentinela por checkout e **nunca sobrescreve** um projeto
  já estabelecido.

---

## 9. Subcomandos por família

Os nomes abaixo são os aceitos pelo executável. Comandos ocultos aparecem entre
parênteses.

| Família | Comandos |
|---|---|
| Servidor | `serve`, `init`, `generate-auth-token` |
| Operação e diagnóstico | `status`, `list-projects`, `doctor`, `audit-contamination`, `llm-test`, `auth` (`login`, `logout`, `status`, `enable`), `completions` |
| Páginas da wiki | `search`, `read-page`, `write-page`, `delete-page`, `restore-page`, `checkpoints` |
| Manutenção | `forget-sweep`, `lint`, `curator`, `compact`, `reclaim-ledger-versions`, `embed`, `reindex`, `reorg`, `commit` |
| Aprendizado | `auto-improve`, `auto-improve-report`, `pending-writes` (`list`, `show`, `diff`, `approve`, `reject`), `bootstrap`, `backfill`, `repair-backfill-timestamps`, `finalize-session` |
| Continuidade | `run`, `show`, `continue`, `resume`, `workstreams`, `rename-workstream`, `workstream-search`, `handoffs`, `message` (`send`, `list`, `pop`, `cancel`) |
| Ciclo de vida | `backup`, `backup-agents`, `restore`, `restore-agents`, `reset`, `export-okf`, `purge-project`, `purge-session`, `rename-project`, `move-project`, `move-session`, `uninstall`, `upgrade` |
| Usuários e credenciais | `user` (`add-human`, `list`, `reset-password`, `disable`, `enable`, `patch`, `grant`, `revoke`, `grants`; os comandos `add`, `expire`, `revive` e `rotate-token` da série 1.x foram removidos), `api-key` (`add`, `list`, `rotate`, `revoke`), `project` (`access`, `grants`), `profile` (`status`, `list`, `show`, `forget`, `review`, `rebuild`, `apply`), `server` (`add`, `list`, `remove`) |
| Integração | `install-hooks`, `install-mcp`, `install-instructions`, `install-skills`, `setup-agent`, `hook` (oculto), `hook-drain` (oculto), `mcp-bridge` (oculto) |

Regras de implementação dos comandos:

- **Comandos finos:** leem argumentos, resolvem a configuração uma vez, chamam funções
  tipadas e apresentam o resultado.
- **Operações que tocam o disco diretamente** (`reset`, `restore`, `reindex` e
  `uninstall --purge-data`) verificam antes se há outro processo ai-memory ativo. Se
  houver, recusam.
- **`backup` não é direto.** Pede ao servidor um snapshot online pela rota de
  administração.
- **Operações destrutivas exigem confirmação explícita** (flag `--confirm` ou
  equivalente).

### 9.1 Comandos e módulos auxiliares

- **`continue`:** retoma o checkout gerenciado mais recente **em qualquer lugar da
  máquina**. Já `run` sem argumentos continua a sessão mais recente **do checkout atual**,
  e exige que o diretório atual esteja dentro do projeto. Por isso `continue` escolhe o
  checkout a partir do registro local de vínculos (`client-projects.json`), opcionalmente
  filtrado por espaço de trabalho, e entrega o diretório ao `run`.
  - A ordem vem do instante do último vínculo bem-sucedido. Empate: ordem alfabética por
    espaço de trabalho e projeto.
  - **Nenhuma consulta ao servidor decide o checkout.** Um vínculo antigo ou redirecionado
    precisa ser rejeitado nesta máquina de qualquer forma.
  - Antes de lançar, valida que o diretório ainda existe, não foi trocado por link
    simbólico e ainda resolve para o mesmo projeto. Um candidato rejeitado é informado
    (`skipping <espaço>/<projeto>: <motivo>`) e **nunca cai silenciosamente num checkout
    mais antigo**. Se todos forem rejeitados, é erro.
  - Um registro com instante ilegível é pulado, sem invalidar os demais.
  - Lança em modo nu, repassando `--yolo`, `--true-yolo` e `--fresh`.

- **`finalize-session`:** gera um fim de sessão sintético para agentes sem evento de fim.
  - **Seleção:** lista as sessões abertas do escopo (`GET /admin/open-sessions`, parâmetros
    `workspace`, `project`, `agent`, `all`, `all_owners`, `include_ended`, `session_id`). Escopo
    inexistente (404) significa "nada a finalizar", não erro. Sem `--all` nem `--session-id`, só a
    sessão aberta mais recente do agente; `--all` e `--session-id` são excludentes; `--reopen` exige
    `--session-id` e também casa a sessão já encerrada. `--agent` aceita qualquer agente que o
    armazenamento reconheça (padrão `codex`), não só os que têm instalador.
  - **Envio:** para cada sessão, um lote de **um** item (`POST /hook/batch`) com evento `session-end`,
    agente, `cwd` (o da sessão, senão o do host ou o atual), escopo e, com `--all-owners`,
    `all_owners=true`. Erro de transporte ou status de falha encerra o comando
    (`server returned <status>: <corpo>`); confirmação diferente de 1 item aceito também
    (`server accepted <n> of 1 synthetic session-end events`). Sessões já finalizadas antes do erro
    permanecem finalizadas.
  - **Antigravity CLI:** seus hooks ao vivo não trazem evento de prompt, então, **antes** do
    `session-end`, o comando reenvia os prompts da própria sessão a partir do histórico local
    (mesmas chaves de ingestão do backfill, logo sem duplicar), só se o diretório for admitido pelo
    modo de captura e **não** com `--all-owners` (prompts de outros donos não estão nesta máquina).
    Falha é só aviso e o fim de sessão segue (`ai-memory: could not replay the prompts of session <id>
    (<erro>); finalizing without them`; sem diretório pessoal: `ai-memory: no home directory; session
    <id> is finalized without its prompts`).
  - **Efeito local:** apaga o id de sessão guardado pelos hooks do agente (por exemplo ZCode, que não
    tem evento de fim) em `<dados>/hook-state/<agente>-session-id`. Só acontece se pelo menos uma sessão
    foi finalizada (nada selecionado = nada apagado); falha ao apagar é ignorada. O envio usa um cliente HTTP
    sem tempo limite próprio; o diretório do `cwd` de reserva é o da variável de diretório do host (quando
    definida e não vazia) ou o diretório atual; `--session-id` precisa ser um UUID (senão erro de parâmetro
    do analisador de linha de comando).
  - **Saída:** `No open <agente> sessions matched <ws>/<proj>` ou `Finalized <n> <agente> session(s) for
    <ws>/<proj>` seguido de uma linha `  - <id>` por sessão; com `--json`, `{workspace, project, agent,
    finalized[]}`.

- **`repair-backfill-timestamps`:** corrige datas de início e fim de sessões que uma
  importação antiga gravou com a data da própria importação, em vez do tempo real do
  histórico.
  - O servidor valida e grava. Só reescreve sessões **realmente** achatadas. Cada
    requisição é uma transação.
  - Lotes de até **2000 sessões** por requisição (teto do servidor).
  - **Simulação por padrão.** `--confirm` aplica. `--json` repassa as respostas.
  - Relatório: sessões `repaired` (valores antigo e novo) e `skipped` (motivo). Sem
    histórico legível, informa que não há nada a enviar.

- **Limpeza do diretório de dados (`reset`, `uninstall --purge-data`):** conjunto de
  subpastas apagadas: `wiki/`, `db/` e `raw/`. As pastas `logs/` e `models/` **nunca**
  são apagadas. Cada operação apaga e recria as pastas vazias. Não é atômica. Quem
  chama cuida da verificação de processos ativos.

- **Prévia de purga (`purge-project`, `purge-session`):** ambos recusam sem `--confirm`.
  Antes, pedem ao servidor uma prévia limitada (`dry_run: true`), com tempo máximo de
  **5 s**. A prévia é *melhor esforço*: a recusa acontece de qualquer forma e não depende
  da prévia. Classificação da resposta: prévia obtida; resposta sem prévia (servidor sem
  suporte, ignorada); recusa com código de status (404 projeto ou sessão inexistente,
  409 workstream gerenciado ativo, 403 autenticação); servidor antigo que não reconhece
  o campo; ou inacessível (tempo esgotado, conexão ou corpo inválido).

- **`path_util`:** cria diretórios privados (`0700` em Unix, idempotente e sem alterar
  permissões de diretórios existentes). Resolve o diretório pessoal: `AI_MEMORY_HOME` se
  não estiver vazio, senão o diretório pessoal do sistema. Resolve a raiz de configuração
  do agente, respeitando a variável que o próprio agente usa para mudá-la (exemplo:
  `CLAUDE_CONFIG_DIR`). Valor vazio ou só com espaços é tratado como não definido.
  Remove o prefixo de caminho estendido do Windows só nas formas seguras.

- **Nomes de instalação:** nome do executável `ai-memory`. No pacote do Windows, o
  arquivo é `ai-memory.exe`. A pasta de hooks é irmã do executável (a confirmar nome
  exato).

---

## 10. Cliente HTTP

Endereço do servidor: URL configurada, com `base_path` se houver.

**Precedência do bearer:**

1. Token estático passado na linha de comando ou na configuração **sempre vence**.
2. Senão, token OIDC armazenado em `auth.json`. Se vencido, é renovado (a confirmar).
3. Senão, requisição anônima.

Funções auxiliares (`get_json`, `post_json`, `post_json_no_content` e outras) concentram
URL e autenticação em um único lugar.

---

## 11. Logging

Filtro de nível, em ordem de prioridade:

1. Variável de ambiente de filtro de log global, se definida (nome a confirmar).
2. Senão, `log_level` da configuração, com dois ajustes:
   - O módulo do protocolo MCP é fixado em `warn` no início do filtro, então é o nível mais fraco e não reabilita
     mensagens de nível `error` ou `off`. Não é adicionado se `log_level` já for `warn` ou
     mais restrito.
   - `tracing_appender=warn` é acrescentado no fim e **não pode ser rebaixado** pela
     configuração. Esse ajuste impede que o próprio registrador em arquivo gere um laço de
     realimentação que encheu o disco em uma instalação de referência.
3. Log em arquivo degrada, **comandos não**. Se o diretório de logs do diretório de
   dados não puder ser usado, tenta o diretório temporário do sistema, e por fim só a saída
   de erro. Cada degradação informa o caminho que falhou (a confirmar a ordem exata de
   tentativas).

---

## 12. Notas para reimplementação

1. **A ordem de boot é a parte mais delicada:** trava de instância → backup pré-migração
   → migrar → reparo de caminhos → validações de autenticação → migrações da wiki →
   observador de arquivos → servidor → agendadores.
2. **Separe o cliente de hooks do servidor.** Fila local, chave de ingestão e drenagem em
   fronteiras são o que torna a captura confiável sem bloquear o agente.
3. **Mantenha os subcomandos como clientes HTTP finos.** O servidor é a única fonte de
   verdade e o único que acessa o banco de dados.
4. **Tarefas periódicas devem persistir o último sucesso**, enumerar escopos a cada ciclo
   e nunca se sobrepor (dormir após cada ciclo).
5. **Falhe cedo na configuração**, inclusive em contingências dormentes, e **recuse
   exposição de rede sem autenticação**.
6. **Toda edição de arquivo de configuração de terceiros** deve ter cópia datada, escrita
   atômica e idempotência (sem gravação quando o conteúdo já é igual).
