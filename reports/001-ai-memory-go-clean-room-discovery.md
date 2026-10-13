# Discovery Report: ai-memory-go (clean-room)

## Metadados

| Campo | Valor |
|-------|-------|
| **ID** | 001 |
| **Slug** | ai-memory-go-clean-room |
| **Status** | approved |
| **Autor** | phm-aguiar |
| **Data** | 2026-10-12 |
| **Versão** | 1.0 |
| **Discovery Report** | `reports/001-ai-memory-go-clean-room-discovery.md` |
| **Aprovado** | true |

---

## 1. Contexto e Problema

O ai-memory (versão de referência 2.6.2, originalmente em Rust) é um servidor único que dá memória de
longo prazo e entre sessões a agentes de código (Claude Code, Codex, Cursor, Gemini CLI etc.), via **MCP**
e **hooks de ciclo de vida**. A documentação em `docs/` (~26 mil linhas, ~50 documentos) descreve o
comportamento, as invariantes e os contratos. O objetivo é uma **reimplementação clean-room em Go**
(`ai-memory-go`) a partir apenas dessa documentação, sem acesso ao código original.

O sistema é grande demais para uma única feature. Este discovery define o **mapa de 3 features grandes** e
detalha o escopo das duas primeiras (F001 e F002). A F003 fica registrada como backlog.

### Usuários Impactados

- **Primários:** o autor (desenvolvedor solo) usando vários agentes de código na própria máquina (Windows 11) e em um servidor doméstico/contêiner (Linux).
- **Secundários (indiretos):** futuros operadores de times (multiusuário/SSO), cobertos só por features futuras.

### Situação Atual (sem a feature)

Não existe código. Cada sessão de agente começa sem contexto das anteriores, e trocar de agente no meio da
tarefa exige re-explicar tudo manualmente. O repositório contém apenas a documentação de referência e o
esqueleto de skills SDD em `.claude/`.

---

## 2. Objetivos e Não-Objetivos

### Objetivos — o que este discovery FAZ

- Entregar um binário Go único `ai-memory`, **zero-LLM**, com paridade de contratos e invariantes dos docs para o escopo de F001 e F002.
- **F001 — Núcleo:** domínio (UUIDv7, `(workspace_id, project_id, path)`, tiers, sanitização como fronteira de tipo), configuração (TOML + `AI_MEMORY_*`), SQLite/FTS5 com escritor único, wiki markdown com escrita atômica e commits git.
- **F002 — Captura e acesso:** endpoint `/hook`, spool local, síntese de sessão por regras, handoffs, servidor MCP (stdio + HTTP) e CLI `serve`/`hook`/`install` para Claude Code.
- Provar a fidelidade por **testes de contrato** derivados dos docs e de `docs/assets/fixtures`, incluindo testes multi-sessão (invariante 16).

### Não-Objetivos — o que este discovery explicitamente NÃO FAZ

- **F003 / backlog:** cliente LLM, consolidação, auto-melhoria, lint por LLM, embeddings e RRF vetorial, reranker, `answer`/`memory_explore`/`memory_consolidate`/`memory_auto_improve`.
- **Multiusuário e corporativo:** usuários, SSO/OIDC, autorização por projeto (grants), webhooks de admissão, perfil entre projetos, TLS/proxy, instalação air-gapped.
- **Workstreams gerenciados** (`ai-memory run`), navegador web da wiki, backup/restore avançado, companions, apps desktop, macOS.
- Hooks nativos para outros agentes além do Claude Code.

> Regra: se não está na lista de objetivos, está fora. Nenhuma suposição silenciosa.

### Mapa de Features

| Feature | Conteúdo | Depende de |
|---------|----------|------------|
| **F001 Núcleo** | Domínio, config, sanitizador, SQLite (migrações, escritor único, pool de leitura, FTS5), busca RRF (FTS + entidades + vizinhos de link, autoridade [0,55–1,50], fallback de observações cruas), wiki (escrita atômica, frontmatter, supersessão, git, watcher + reconciliação 30 s) | — |
| **F002 Captura e acesso** | `/hook` + `/hook/batch` (202/429, idempotência), spool e `ai-memory hook`, política de captura (marker), sessão + handoff automático, MCP (9 tools), CLI `serve`/`hook`/`install` para Claude Code | F001 |
| **F003 Adiado** | LLM, embeddings, consolidação, workstreams, multiusuário/SSO/authz, web, backup avançado, demais agentes | F001, F002 |

---

## 3. Requisitos

### 3.1 Funcionais

| ID | Requisito | Prioridade |
|----|-----------|-----------|
| RF-01 | Domínio sem I/O: UUIDv7, identidade `(workspace_id, project_id, path)`, caminho de página portável, tiers e tipo de observação sanitizada criável só pela função de sanitização | Must |
| RF-02 | Configuração carregada uma única vez (TOML + `AI_MEMORY_*`, separador `__`); diretório de dados absoluto e canônico registrado no log | Must |
| RF-03 | SQLite (modernc, WAL, FTS5) com migrações `V<nn>__nome.sql`, snapshot antes de migrar e escritor único (goroutine + canal, capacidade 1024) | Must |
| RF-04 | Wiki: gravação atômica (tmp + fsync + rename + fsync do diretório), upsert de página na mesma transação, rollback do arquivo se o upsert falhar, commit git por evento | Must |
| RF-05 | Supersessão: escrita concorrente divergente no mesmo caminho gera cadeia de supersessão; a versão perdedora continua alcançável | Must |
| RF-06 | Sanitização remove segredos antes de limitar tamanho (título ≤ 80 chars, corpo ≤ 16 KiB); único caminho de texto não confiável para o armazenamento | Must |
| RF-07 | `/hook` responde 202 ao enfileirar e 429 na saturação (≤ 1024 em voo; ≤ 256 itens/lote); chave de ingestão idempotente na mesma transação da observação | Must |
| RF-08 | `ai-memory hook` com spool local, lock de arquivo e drenagem desacoplada; nunca bloqueia o agente | Must |
| RF-09 | SessionEnd gera página de sessão por regras (sem LLM) em `sessions/` e abre handoff automático, tudo atômico | Must |
| RF-10 | Handoff reivindicado exatamente uma vez (duas guardas independentes de estado `aberto`); páginas compartilhadas, handoffs com dono | Must |
| RF-11 | MCP stdio + HTTP streamable com tools: `memory_query`, `memory_read_page`, `memory_recent`, `memory_write_page`, `memory_status`, `memory_briefing`, `memory_handoff` (begin/accept/list), `memory_delete_page`, `memory_read_session_observations`; autenticação Bearer | Must |
| RF-12 | `memory_query`: FTS5 + entidades + vizinhos de link com RRF k = 60, multiplicador de autoridade clampado, fallback `raw_hits`, reforço de acesso throttled (1/min por página e operador) | Must |
| RF-13 | Resolução central de escopo; leitura/busca/destrutivo usam busca sem criar e falham fechado em escopo ausente/parcial | Must |
| RF-14 | CLI cobra: `serve`, `hook`, `install` (Claude Code), `status`; guarda de processo vivo antes de `reset`/`reindex` | Should |
| RF-15 | Watcher de arquivos com debounce de 300 ms e reconciliação periódica de 30 s; ignora escritas próprias pelo prefixo temporário | Should |
| RF-16 | Backup online do SQLite via API de backup (`ai-memory backup`) | Could |

### 3.2 Não-Funcionais

| Categoria | Requisito | Métrica de Aceite |
|-----------|-----------|------------------|
| Performance | Hook nunca bloqueia o agente; POST do hook com timeout curto configurável | Padrão 2 s configurável (ADR-04); `ai-memory hook` retorna < 200 ms sem rede |
| Segurança | Segredos removidos antes de gravar; comparação de token em tempo constante; escuta padrão `127.0.0.1:49374` | Suíte de fixtures de segredos passa; nenhum segredo cru em `observations` |
| Observabilidade | Log estruturado (`log/slog`) com filtro do próprio módulo; linha por escrita no `log.md` da wiki | Sem laço de log; cada escrita gera `## [ts] <evento> \| <título>` |
| Compatibilidade | Windows 11 + Linux, binário único pure-Go sem CGO | `go build` com `CGO_ENABLED=0` para `windows/amd64` e `linux/amd64` |

---

## 4. Cenários Gherkin (BDD)

> Estes cenários são o contrato de desenvolvimento. Nenhuma task de implementação deve começar
> sem que o cenário correspondente esteja aqui definido.

```gherkin
Feature: Memória de longo prazo compartilhada entre agentes
  Como desenvolvedor que usa vários agentes de código
  Quero que o contexto das sessões seja capturado, indexado e entregue ao próximo agente
  Para continuar uma tarefa em outro agente sem re-explicar o contexto

  Background:
    Dado que o servidor ai-memory está em execução com diretório de dados absoluto e canônico
    E o escritor SQLite único está ativo e a wiki é um repositório git inicializado
    E existe um workspace "default" com o projeto "demo"

  # ============================================================
  # CENÁRIOS DE SUCESSO (mínimo 3)
  # ============================================================

  Scenario: Escrever uma página atualiza arquivo, índice e git juntos
    Dado que a página "decisions/db.md" não existe no projeto "demo"
    Quando o cliente chama memory_write_page com path "decisions/db.md" e corpo válido
    Então o arquivo markdown é instalado de forma atômica na wiki
    E a página e o índice FTS são commitados na mesma transação SQLite
    E um commit git é criado e a resposta traz identificador, caminho e checkpoint
    E uma linha é anexada ao log.md da wiki

  Scenario: Captura de sessão por hooks e handoff automático no SessionEnd
    Dado que o Claude Code tem hooks instalados para o projeto "demo"
    Quando ocorrem SessionStart, UserPromptSubmit, PostToolUse e SessionEnd
    Então cada evento retorna 202 e vira uma observação sanitizada com chave de ingestão
    E a página "sessions/<id>.md" é escrita sem LLM com tier episodic
    E um handoff automático é aberto na mesma transação do carimbo de fim de sessão

  Scenario: Busca híbrida retorna páginas compiladas ranqueadas por RRF
    Dado que existem páginas que casam por texto, por entidade e por vizinhança de link
    Quando o cliente chama memory_query com a consulta "banco de dados"
    Então os resultados são fundidos por RRF com k igual a 60
    E o multiplicador de autoridade fica entre 0,55 e 1,50
    E o acesso das páginas retornadas é reforçado no máximo uma vez por minuto por operador

  Scenario: Segundo agente recebe o handoff no mesmo diretório
    Dado que a sessão do agente A terminou e abriu um handoff para o diretório do projeto
    Quando o agente B inicia uma sessão nesse diretório e aceita o handoff
    Então o handoff é entregue com resumo, itens e arquivos dentro dos limites de tamanho
    E o estado do handoff passa de aberto para reivindicado

  # ============================================================
  # CENÁRIOS DE FALHA (mínimo 2)
  # ============================================================

  Scenario: Falha no banco após instalar o arquivo reverte a página
    Dado que o upsert da página no SQLite vai falhar após o rename do arquivo
    Quando o cliente chama memory_write_page com path "decisions/db.md"
    Então o sistema retorna erro de escrita não ambíguo ao cliente
    E o arquivo markdown é removido ou restaurado à versão anterior
    E nenhum commit git é criado e o índice permanece inalterado

  Scenario: Leitura sem escopo resolvível falha fechada
    Dado que o chamador não informa projeto e não há ponteiro de projeto ativo nem projeto padrão
    Quando o cliente chama memory_query ou memory_read_page
    Então o sistema retorna erro de escopo ausente
    E nenhum workspace nem projeto é criado
    E o estado do sistema permanece inalterado

  Scenario: Servidor saturado responde 429 sem bloquear o agente
    Dado que já existem 1024 requisições de ingestão em voo
    Quando um novo evento chega em POST /hook
    Então o servidor responde HTTP 429 imediatamente
    E o comando ai-memory hook mantém o evento no spool local e retorna ao agente sem bloquear
    E um lote com mais de 256 itens é rejeitado por inteiro

  # ============================================================
  # EDGE CASES (mínimo 1)
  # ============================================================

  Scenario: Duas sessões escrevem o mesmo caminho ao mesmo tempo
    Dado que os agentes A e B têm sessões abertas no projeto "demo"
    Quando ambos chamam memory_write_page para "rules/style.md" com conteúdos divergentes
    Então uma versão é escrita e a outra supersede a anterior em uma cadeia de supersessão
    E a versão perdedora continua alcançável pelo histórico
    E a leitura da página não é filtrada pela autoria

  Scenario: Handoff disputado por dois agentes é reivindicado uma única vez
    Dado que existe um handoff aberto não compartilhado
    Quando dois agentes tentam aceitá-lo ao mesmo tempo
    Então exatamente um aceite tem sucesso
    E o outro recebe erro de handoff já reivindicado

  Scenario: Replay do mesmo evento não duplica observação
    Dado que um SessionEnd foi entregue e processado com a chave de ingestão K
    Quando o mesmo evento com chave K é reenviado, inclusive após interrupção no meio do processamento
    Então nenhuma segunda observação é gravada
    E os efeitos downstream pendentes (página e handoff) convergem sem duplicar

  Scenario: Edição manual da wiki no disco é reindexada
    Dado que o usuário edita "rules/style.md" por um editor externo
    Quando o watcher dispara após o debounce de 300 ms ou a reconciliação de 30 s roda
    Então o índice SQLite é atualizado a partir do markdown
    E as escritas feitas pelo próprio servidor não geram reindexação por ecoarem no watcher

  Scenario: Segredo no payload do hook nunca chega ao armazenamento
    Dado que o payload do evento contém uma chave de API e um título com mais de 80 caracteres
    Quando o evento é admitido em POST /hook
    Então o segredo é removido antes de limitar o tamanho
    E o título é truncado em 80 caracteres e o corpo em 16 KiB
```

### Cobertura

| Tipo | Quantidade | Status |
|------|-----------|--------|
| Cenários de sucesso | 4 | [x] ≥ 3 |
| Cenários de falha | 3 | [x] ≥ 2 |
| Edge cases | 5 | [x] ≥ 1 |

---

## 5. Decisão Arquitetural (ADR)

### Status

`approved` — aprovado em 2026-10-12

### Drivers de Decisão

- Fidelidade aos contratos e invariantes dos docs (clean-room), com escopo faseado.
- Binário único, fácil de distribuir e cross-compilar (Windows + Linux), sem toolchain C.
- Aderência às invariantes: leitura única de config, escritor único, sanitização como fronteira de tipo, grafo de dependências entre componentes.

### Opções Consideradas

| Opção | Descrição Resumida | Prós | Contras |
|-------|--------------------|------|---------|
| A: Módulo único, 1 binário, camadas em `internal/` | `cmd/ai-memory` + `internal/{domain,storage,wiki,ingest,mcp,cli}`; imports seguem o grafo do overview | Simples, idiomático, cross-compile trivial, regra de import verificável | Isolamento por convenção, não por módulo |
| B: `go.work` com módulos por componente | Cada componente é um módulo | Isolamento forte de dependências | Overhead alto para o estágio atual |
| C: 2 binários (servidor e hook mínimo) | Hook com cold start mínimo | Hook mais rápido | Dois artefatos para distribuir; sem evidência de necessidade |

### Decisão

**Opção escolhida:** A, com as escolhas tecnológicas abaixo.

| ADR | Tema | Decisão |
|-----|------|---------|
| ADR-01 | SQLite | `modernc.org/sqlite` (pure Go, sem CGO, FTS5, backup online) |
| ADR-02 | Escritor único | Goroutine dona da conexão de escrita + canal de capacidade 1024; leitura em pool `*sql.DB` somente leitura |
| ADR-03 | Git | Executar o `git` do sistema via `os/exec`; `serve` valida a presença do git na inicialização com erro claro |
| ADR-04 | Itens "a confirmar" | Decidir, registrar como "decisão nossa" e listar como débito de fidelidade. Timeout de POST do hook: padrão 2 s, configurável. Invariante 12: reservada (sem imposição) |
| ADR-05 | MCP | `mark3labs/mcp-go` (stdio + HTTP streamable), atrás de uma interface própria para permitir trocar o SDK |
| ADR-06 | CLI e config | `cobra` + `koanf` (TOML + env `AI_MEMORY_*` com `__`), carregados uma vez na CLI |
| ADR-07 | Sanitizador | `regexp` da stdlib (RE2, sem retrocesso); padrões dos docs adiados reescritos nos testes de fixture |
| ADR-08 | Identificadores e logs | UUIDv7 (`google/uuid`), `log/slog` com filtro do próprio módulo |
| ADR-09 | Agente de hooks | Claude Code primeiro (`settings.json`); demais agentes na F003 |
| ADR-10 | Verificação de fidelidade | Testes de contrato a partir dos docs e de `docs/assets/fixtures`; cada invariante vira teste, incluindo multi-sessão |

**Justificativa:** pure-Go e módulo único minimizam atrito no Windows e entregam um binário autônomo. O
canal com capacidade fixa dá backpressure natural para o 429. O `git` do sistema evita divergências de
comportamento do go-git com a configuração do usuário. O SDK `mark3labs/mcp-go` foi escolha explícita do autor.

### Consequências

**Positivas:**
- Cross-compile direto (`CGO_ENABLED=0`) e binário único.
- Camadas verificáveis por regra de imports (domínio sem I/O).
- Invariantes viram testes automatizados reutilizáveis nas features seguintes.

**Negativas / Riscos:**
- `modernc.org/sqlite` é mais lento que o SQLite nativo e carrega o risco de diferenças sutis (WAL, FTS5 tokenizers).
- Dependência do `git` instalado na máquina.
- `mcp-go` é de comunidade e pode divergir da especificação MCP.

---

## 6. Débitos Técnicos Antecipados

> Liste explicitamente o que sabemos que será imperfeito neste ciclo. Débito não documentado
> é débito invisível — o pior tipo.

| Débito | Impacto | Plano de Mitigação |
|--------|---------|-------------------|
| Repositório sem `.github/memory/tech.md`, `constitution.md` e `specs/` | Alto | Rodar `/sdd-init-repo` antes de `/sdd-generate-spec`, gravando as ADR-01 a ADR-10 em `tech.md` e as invariantes em `constitution.md` |
| `docs-adiados/` ausente: prompts, schemas e regexes do sanitizador não estão no repo | Alto | Escrever o conjunto de padrões de segredo a partir dos docs e de fixtures próprias; revisar quando o material for recuperado |
| Pontos "a confirmar" decididos por nós (timeout de hook, invariante 12, cobertura de sanitização) | Médio | Registrar cada um em ADR como "decisão nossa" e criar teste de contrato; reavaliar na F003 |
| Sem cobertura de macOS, multiusuário, SSO e outros agentes | Médio | Backlog explícito na F003; manter a camada de autorização atrás de interface (capacidades) para não refatorar depois |
| Fidelidade provada só por docs e fixtures, sem comparar com o binário original | Médio | Matriz invariante→teste mantida em `test-fixtures`; marcar testes por documento de origem |
| Dependência do SDK `mark3labs/mcp-go` | Baixo | Isolar atrás da interface do pacote `mcp`; teste de conformidade do protocolo (`tools/list`, `tools/call`) |
| Desempenho do FTS5 do modernc não medido | Baixo | Benchmark com 5 mil páginas na F001; alvo de consulta < 100 ms; registrar baseline |

---

## 7. Cross-Reference (Wiki + Codebase)

### Padrões Encontrados na Wiki

| Fonte | Relevância | Implicação para esta feature |
|-------|-----------|------------------------------|
| `docs/overview.md` §4, §7, §9 | Alta | 16 invariantes, ordem de implementação e grafo de dependências viram a regra de camadas e a lista de testes de contrato |
| `docs/architecture.md` (fluxo de dados) | Alta | Define o loop captura → admissão → fim de sessão → consulta e a idempotência at-least-once |
| `docs/design-decisions.md` §3–§4 | Alta | Markdown como fonte da verdade, SQLite derivado e reconstruível, sem transação atômica arquivo+banco (rollback best-effort) |
| `docs/mcp-server.md` §2–§3 | Alta | Resolução de escopo (precedência, falha fechada), contrato de cada tool, limites numéricos |
| `docs/domain-vocabulary.md`, `database-schema.md`, `storage-and-retrieval.md`, `wiki-files-layer.md`, `lifecycle-event-ingestion.md`, `marker-file.md` | Alta | Fontes detalhadas para a spec de F001 e F002; serão lidas integralmente em `/sdd-generate-spec` |

### Código Existente Relacionado

| Arquivo | Padrão Identificado | Reutilizável? | Ação |
|---------|-------------------|--------------|------|
| (nenhum — repositório greenfield; apenas `docs/` e `.claude/`) | — | Não | Criar a estrutura Go do zero; rodar `/sdd-init-repo` |
| `.claude/skills/sdd-*` | Pipeline SDD já instalada | Sim | Usar `/sdd-generate-spec` → `plan` → `tasks` → `coordinate` |

### Ambiguidades Resolvidas Durante o Discovery

| Termo/Conceito Ambíguo | Definição Adotada |
|------------------------|------------------|
| "Poucas features grandes" | 3 features: F001 Núcleo, F002 Captura e acesso, F003 backlog adiado |
| "Binários" (plural) | Um único binário `ai-memory` com subcomandos |
| "Zero-LLM" | Nenhuma chamada a provedor de LLM; síntese de sessão e resumos por regras |
| "Fiel" | Contratos e invariantes dos docs cobertos por testes de contrato, não paridade byte a byte com o original |
| "Um agente" nos hooks | Claude Code |
| "Plataformas" | Windows e Linux; macOS fora |
| "Multiusuário" | Fora do escopo; o `actor` existe como identidade única local (um operador) |
| "Cenário `memory_query` completo" | FTS5 + entidades + vizinhos de link (RRF k = 60) + autoridade + fallback de observações cruas; sem vetor, rerank nem `answer` |

---

## 8. Riscos

| Risco | Probabilidade | Impacto | Mitigação |
|-------|--------------|---------|-----------|
| Escopo das F001/F002 ainda grande demais para uma entrega | Alta | Alto | Fatiar em tasks pequenas no `generate-tasks`; marcar RF "Could" como cortáveis |
| Divergência de comportamento do FTS5 no modernc (tokenizer, ranking) | Média | Médio | Testes de contrato de busca com fixtures; benchmark inicial |
| Bugs de concorrência (supersessão, handoff, idempotência) invisíveis em testes de sessão única | Média | Alto | Suíte multi-sessão obrigatória e `go test -race` no CI |
| Locks de arquivo e processos vivos diferem entre Windows e Linux | Média | Médio | Abstração por build tags e testes nas duas plataformas |
| Hooks do Claude Code mudam de formato | Baixa | Médio | Adapter isolado e fixtures de payload versionadas |
| Lacunas dos docs ("a confirmar", `docs-adiados` ausente) geram retrabalho | Média | Médio | ADR por decisão e débito de fidelidade explícito |

---

## 9. Critérios de Aceitação (Definition of Done)

- [ ] `go build ./...` e `go vet ./...` passam com `CGO_ENABLED=0` para `windows/amd64` e `linux/amd64`
- [ ] `go test -race ./...` passa, incluindo a suíte multi-sessão (supersessão, handoff único, idempotência)
- [ ] Os 16 invariantes têm teste de contrato ou débito registrado (12 como reservada)
- [ ] Fluxo ponta a ponta: hook de Claude Code → observação → `sessions/<id>.md` → handoff → `memory_query` retorna o resultado
- [ ] As 9 tools MCP da F002 respondem via stdio e HTTP com autenticação Bearer
- [ ] Rollback de página validado quando o banco falha após o rename do arquivo
- [ ] Nenhum segredo cru em `observations` nas fixtures de sanitização
- [ ] Todos os cenários Gherkin passam (automatizados ou verificados manualmente)
- [ ] Nenhum débito técnico não documentado introduzido

---

## Quality Score

| Dimensão | Score | Observações |
|----------|-------|-------------|
| Completeness (0-5) | 5 | Todas as 9 seções preenchidas, sem TODO/TBD nem placeholders |
| Gherkin coverage (0-5) | 5 | 4 sucesso + 3 falha + 5 edge, todos com Dado/Quando/Então e estado inalterado nas falhas |
| Ambiguity (0-5) | 4 | Termos definidos na tabela de ambiguidades; limites finos de `memory_briefing` e do formato do handoff ficam para a spec |
| Debt surface (0-5) | 5 | 7 débitos com impacto e mitigação concreta |
| Wiki alignment (0-5) | 4 | 5 grupos de docs consultados com implicações; os 6 docs de detalhe serão lidos na spec; sem código existente |
| **Total** | **23/25** | Mínimo: 20/25 |
