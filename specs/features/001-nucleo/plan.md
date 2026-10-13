# plan.md: Plano de Implementação Técnica — Núcleo (001)

## 1. Metadados do Plano
- **Stack Tecnológico:** Go 1.26 (toolchain local 1.26.5), `modernc.org/sqlite` (WAL, FTS5), `git` do sistema 2.56 via `os/exec`, `log/slog`, `regexp` (RE2), `google/uuid` (UUIDv7), `gopkg.in/yaml.v3` para frontmatter, `fsnotify` para o watcher, `koanf` (TOML + env) e `cobra` só para o mínimo de CLI de teste. `CGO_ENABLED=0`, alvos `windows/amd64` e `linux/amd64`. Itens novos (`yaml.v3`, `fsnotify`) precisam entrar em `tech.md` antes do primeiro uso (portão da constituição).
- **Feature Fonte:** `specs/features/001-nucleo/spec.md`
- **Escopo:** domínio sem I/O, configuração, SQLite com escritor único e busca RRF, wiki com escrita atômica, supersessão e git, watcher/reindex. Sem hooks, MCP, LLM, embeddings ou multiusuário (feature 002 e seguintes, ver `roadmap.md`).

### Layout de pacotes e regra de imports

```text
cmd/ai-memory/main.go            ← chama internal/cli
internal/domain/                 ← sem I/O: ids, scope, pagepath, tier, observation, sanitize
internal/config/                 ← carrega TOML + AI_MEMORY_* uma vez (único lugar com os.Getenv)
internal/storage/                ← open, migrations/, writer (ator), reader (pool), search, scope
internal/wiki/                   ← markdown, atomicfile, gitrepo, locks, write, reindex, watcher
internal/cli/                    ← único compositor (cobra): serve e comandos mínimos
```

Imports permitidos (verificados por teste de imports, ver §4): `domain` → nada; `config` → nada;
`storage` → `domain`; `wiki` → `domain`, `storage`; `cli` → todos. `config` é entregue pela `cli` por
valor (struct tipada) aos demais pacotes, que nunca leem o ambiente.

## 2. Design de Contratos e Fronteiras
- **Contrato:** não há API de rede nesta feature. Os contratos são (a) as interfaces Go entre pacotes, (b) o esquema SQL `V01` e (c) o formato em disco da wiki. Os nomes de tabela e coluna do `docs/database-schema.md` são contrato e são mantidos.
- **Geração de Código:** nenhuma (`sqlc` e afins ficam de fora; SQL escrito à mão em `embed.FS`). `go generate` não é usado.

### 2.1 Domínio (`internal/domain`)
- `ID [16]byte` (UUIDv7) com `NewID()`, `ParseID`, `String()`; armazenado como `BLOB` de 16 bytes.
- `Scope{WorkspaceID, ProjectID ID}` e `PageRef{Scope, Path PagePath}` (identidade de 3 elementos, INV-04).
- `PagePath`: construtor `ParsePagePath(string) (PagePath, error)` normaliza separador POSIX, recusa `..`, caminhos absolutos, segmentos reservados e nomes não portáveis no Windows (`CON`, `NUL`, `:`, `?`, ponto/espaço no fim). `PortableKey()` (minúsculas + Unicode NFC) detecta colisão de caixa. `SearchText()` calcula `path_search` pela regra do doc.
- `Tier` (`working|episodic|semantic|procedural`), `PageKind`, `ObservationKind`.
- `Sanitized` e `sanitize.Text(raw string, limits Limits) Sanitized`: **único** construtor do tipo (campo não exportado). Remove segredos e depois limita (título 80, corpo 16 KiB). Padrões em RE2.
- `Page`, `Frontmatter` (mapa ordenado), `Observation{... Title, Body Sanitized}`.

### 2.2 Armazenamento (`internal/storage`)

```go
// Writer é o ator de escrita. Uma goroutine dona da conexão; fila de 1024.
type Writer interface {
    UpsertPage(ctx context.Context, in UpsertPage) (UpsertResult, error)
    DeletePath(ctx context.Context, ref domain.PageRef) (DeleteResult, error)
    EnsureScope(ctx context.Context, in EnsureScope) (domain.Scope, error) // único que cria
    InsertObservation(ctx context.Context, in InsertObservation) error     // F002 chama
    BumpAccess(ctx context.Context, in BumpAccess) error                   // reforço throttled
    Snapshot(ctx context.Context, toPath string) error                     // backup online
    QueueLen() int
    Close(ctx context.Context) error
}

// Reader usa pool somente leitura (soft cap 4).
type Reader interface {
    ResolveScope(ctx context.Context, req ScopeRequest) (domain.Scope, ScopeSource, error) // nunca cria
    ReadPage(ctx context.Context, ref domain.PageRef) (domain.Page, error)
    Recent(ctx context.Context, s domain.Scope, limit int) ([]PageSummary, error)
    Search(ctx context.Context, q SearchQuery) (SearchResult, error) // FTS+entidades+grafo, RRF, autoridade, raw_hits
    PageChain(ctx context.Context, ref domain.PageRef) ([]domain.PageVersion, error)
}
```

- **Erros tipados:** `ErrWriterClosed`, `ErrScopeNotFound`, `ErrScopeAmbiguous`, `ErrPathCollision`, `ErrSchemaAhead`, `ErrInconsistentState`. O resolvedor de escopo **só** devolve `ErrScopeNotFound` (falha fechada) em leitura.
- **Pragmas:** `journal_mode=WAL`, `synchronous=NORMAL`, `busy_timeout=5000`, `foreign_keys=ON` (OFF durante as migrações).

### 2.3 Esquema SQL `V01__init.sql` (estado final completo)
`V01__init.sql` materializa o **estado final completo** de `docs/database-schema.md` (todas as tabelas, índices, triggers de FTS e de pareamento), mais `schema_migrations`. As tabelas são criadas já nesta migração por fidelidade, mas **só são usadas conforme cada feature precisar**:
- **Usadas na 001:** `workspaces`, `projects`, `pages` + `pages_fts`, `links`, `entities`, `entity_page_links`, `page_access`, `page_evidence`, `audit_log`, `sessions`, `observations` + `observations_fts` (fallback `raw_hits`).
- **Criadas e ainda sem código (features seguintes):** `ingest_keys`, `session_consolidation_jobs`, `handoffs`, `agent_messages` (002); embeddings, `page_feedback`, `users`, `api_credentials`, `web_sessions`, `project_grants`, auto-melhoria, workstreams, perfil e demais (features 003 a 016, ver `roadmap.md`).
- Tabelas sem uso não têm repositório, handler nem teste de comportamento; só o teste `TestSchemaV01` confere que o esquema aplica e é idempotente. Isso não infringe "sem código morto" da constituição, porque é DDL e não código Go.
- **Desvio deliberado mantido:** `sessions.agent_kind` **sem** `CHECK` fechado (validação na aplicação), conforme a dica do próprio doc.
- Colunas `author_id` e `embedding_*` existem no esquema, mas a 001 não as lê nem as escreve (leitura nunca filtra por autoria, INV-16).

### 2.4 Wiki em disco
`<data_dir>/wiki/<workspace_id>/<project_id>/{_meta.md,index.md,log.md,sessions/,concepts/,decisions/,...}`. Diretórios por UUID, nunca por nome. Commits com autor fixo `ai-memory <ai-memory@local>`. Temporário `.ai-memory-tmp.*` no mesmo diretório do destino.

## 3. Decisões Arquiteturais e Justificativas

- **Decisão (P-01): migrações com runner próprio sobre `embed.FS`**, arquivos `V<nn>__nome.sql`, uma transação por migração, tabela `schema_migrations(version,name,applied_on,checksum)`, `foreign_keys=OFF` durante e `ON` depois. Um schema à frente do binário recusa a abertura (`ErrSchemaAhead`). Snapshot do banco antes de migrar.
  - **Justificativa:** o contrato do doc (checksum tolerado, recusa de schema novo, FK desligada) é pequeno e específico demais para ser contornado por uma biblioteca.
  - **Alternativa Rejeitada:** `golang-migrate`/`goose`. Trazem dependência e semântica de FK e checksum que divergem do contrato; o runner próprio tem ~150 linhas.

- **Decisão (P-02): ator de escrita genérico por comando.** Uma goroutine consome `chan command` (capacidade 1024); cada comando é uma closure `func(*sql.Tx) error` com canal de resposta de uso único. Cada método público = uma transação (INV-02, INV-03). Fila cheia bloqueia o chamador até o `ctx` cancelar (o 429 é responsabilidade do ingest, feature 002).
  - **Justificativa:** serializa escritas sem mutex, dá métrica de ocupação e mantém índices (FTS por triggers, links, entidades, evidência, auditoria) na mesma transação.
  - **Alternativa Rejeitada:** `MaxOpenConns(1)` no `database/sql` (sem fila observável nem lote) e `sync.Mutex` (sem backpressure).

- **Decisão (P-03): arquivo antes do índice, com rollback.** Fluxo de `wiki.WritePage`: confinamento e portabilidade → sanitização → derivações (título, links, entidades, `expires_at`) → emissão → trava por página (mapa de mutex ordenado) → instala arquivo guardando o anterior → `storage.UpsertPage` → em falha restaura o anterior; se a restauração falhar, retorna `ErrInconsistentState`. Commit git só depois do sucesso do índice (falha de commit vira aviso, não erro).
  - **Justificativa:** o markdown nunca fica atrás do banco; o reindex reconcilia o resto (decisão do doc). Cobre o cenário Gherkin de falha no banco.
  - **Alternativa Rejeitada:** índice primeiro e arquivo depois (deixa banco apontando para arquivo inexistente em crash) e 2PC (complexidade sem ganho em processo único).

- **Decisão (P-04): supersessão no `UpsertPage`.** Casos A (idêntico: não cria versão, só registra evidência), B (mudou: antiga `is_latest=0`, `valid_to=agora`, fecha janelas de entidade; nova com `supersedes`) e C (nova: checa colisão de caminho por `PortableKey`; ressuscita tombstone se houver). Tudo em uma transação; índice único parcial `idx_pages_latest_path` é a rede de segurança.
  - **Justificativa:** cumpre INV-16 (supersede, nunca destrói) mesmo sob escrita concorrente de duas sessões, já que o escritor é único e o índice único rejeita a corrida.
  - **Alternativa Rejeitada:** `UPDATE` in-place da página (perde a versão perdedora) e optimistic locking por hash (empurra a corrida para o cliente).

- **Decisão (P-05): busca = três fluxos + grafo, RRF depois autoridade.** Janela `clamp(limit×4, 20, limit+300)`; fluxo FTS5 com preparação segura da consulta (`prepareFTS5Query`: detecta sintaxe explícita, remove 57 stop words, une com `OR`, neutraliza `:`, cita pontuação ASCII, valida com uma sonda FTS5 e degrada para termos citados); fluxo de entidades (tokens 3–64 chars, máx 12, peso `1/freq`); grafo bidirecional a partir das sementes; `score = Σ 1/(60+pos)`; fator de autoridade em `[0,55; 1,50]` somando ajustes por tipo, tier, `pinned`, caminho e tags, e aplicado multiplicando se `rank ≤ 0` e dividindo se `> 0`. Sem fluxo de vetor (campo ausente, não stub). Se nenhuma página compilada casar, `raw_hits` por `observations_fts`.
  - **Justificativa:** reproduz o contrato do doc sem LLM; autoridade pós-fusão exige a janela larga.
  - **Alternativa Rejeitada:** só BM25 do FTS5 (perde recall por entidade e vizinhança) e reranking por LLM (fora de escopo, INV-13).

- **Decisão (P-06): escrita atômica de arquivo com retry no Windows.** `atomicfile.Write` = temporário `.ai-memory-tmp.*` no mesmo diretório → `Sync` → `os.Rename` → `fsync` do diretório (melhor esforço; no-op no Windows). No Windows, `ERROR_SHARING_VIOLATION` (32) e `ERROR_LOCK_VIOLATION` (33) repetem até 5 vezes com 10–50 ms. Retorna o identificador do arquivo para o watcher reconhecer as próprias escritas (além do prefixo).
  - **Justificativa:** INV-10 e a realidade de antivírus/indexador no Windows, que é o ambiente do autor.
  - **Alternativa Rejeitada:** `os.WriteFile` direto (não atômico) e biblioteca de terceiros de rename atômico (dependência para ~40 linhas).

- **Decisão (P-07): sanitizador como fronteira de tipo.** Pacote `domain/sanitize` expõe `Text()` e o tipo `Sanitized` (campo privado); `storage` e `wiki` só aceitam `Sanitized` nas assinaturas de escrita. Padrões de segredo em lista versionada (chaves de API de provedores comuns, tokens Bearer, JWT, chaves privadas PEM, `password=`/`secret=`/`token=`, URLs com credenciais), testados contra fixtures em `internal/domain/sanitize/testdata`. Remove segredo antes de truncar; trunca em fronteira de rune.
  - **Justificativa:** INV-06 imposta pelo compilador, não por convenção. `docs-adiados/` estava ausente, então a lista é nossa (hoje em `docs/prompts-schemas-and-sanitizer.md` §11; alinhamento na 002/T093) e fica registrada como débito de fidelidade.
  - **Alternativa Rejeitada:** sanitizar nos handlers (esquecível) e `string` com função `Clean()` opcional.

- **Decisão (P-08): watcher com `fsnotify`, debounce de 300 ms e reconciliação de 30 s; reindex como caminho único.** O watcher e a reconciliação convergem para `wiki.IndexFile(ref)`, o mesmo código do `reindex` completo, que lê o markdown, deriva e chama `storage.UpsertPage`. Escritas próprias são ignoradas pelo prefixo e pelo identificador do arquivo. `reindex` e `reset` consultam processo vivo antes de agir (INV-09). Remoção de arquivo não apaga a página por padrão (tombstone opcional, desligado).
  - **Justificativa:** um único caminho de indexação evita divergência entre tempo real e reconstrução; a reconciliação cobre eventos perdidos (especialmente no Windows).
  - **Alternativa Rejeitada:** só `fsnotify` (perde eventos) e só varredura periódica (latência alta).

- **Decisão (P-09): git por `os/exec`, serializado.** `gitrepo` inicializa o repositório, valida `git` no PATH na abertura (erro claro), faz `add`+`commit` por evento sob mutex próprio, com `-c user.name=ai-memory -c user.email=ai-memory@local -c commit.gpgsign=false`. Paths sempre passados por `--` e relativos à raiz.
  - **Justificativa:** ADR-03 do discovery; evita divergência do `go-git` com a config do usuário.
  - **Alternativa Rejeitada:** `go-git` (comportamento incompleto) e git opcional (mais caminhos de teste; adiado).

- **Decisão (P-11): configuração em camadas, com `.env` local não versionado.** `config.Load` é o único ponto que lê ambiente (INV-01) e resolve, em ordem crescente de precedência: padrões embutidos < arquivo TOML < `.env` < variáveis do processo `AI_MEMORY_*` (separador `__`). O `.env` é opcional, lido da raiz do repositório (diretório de trabalho) ou de `AI_MEMORY_ENV_FILE`, e **nunca é versionado**: `.gitignore` o exclui e `.env.example` (versionado, só nomes e valores de exemplo, sem segredos) documenta as chaves. Chaves entram no `.env.example` à medida que cada feature precisar delas. O resultado é uma struct `Config` tipada e imutável, entregue por valor aos demais pacotes. Uso de `koanf` com os providers `file`, `env` e o parser `dotenv`.
  - **Justificativa:** o autor quer um `.env` local populado conforme o projeto cresce; manter tudo em um caminho preserva a invariante e mantém segredos fora do git.
  - **Alternativa Rejeitada:** `godotenv` chamando `os.Setenv` (muta o ambiente global e espalha a leitura) e `viper` (vetado pela constituição).

- **Decisão (P-10): contratos de teste por invariante.** Cada `INV-nn` ganha um teste `TestINVnn_*` (ver §4). Fixtures vêm de `docs/assets/fixtures` e `docs/test-fixtures.md`; testes multi-sessão usam duas `Writer`/`Reader` sobre o mesmo banco temporário com `t.Parallel` e `-race`. Benchmark `BenchmarkSearch5k` registra o baseline (< 100 ms).
  - **Justificativa:** a constituição exige teste multi-sessão e invariante nomeada.
  - **Alternativa Rejeitada:** só testes unitários por função (não exercitam concorrência entre sessões).

## 4. Auditoria de Constituição

**Portões de Qualidade**
- [ ] `CGO_ENABLED=0 go build ./...` e `go vet ./...` para `windows/amd64` e `linux/amd64` — P-01 a P-09 usam só pure-Go; verificado no portão de CI/tasks.
- [ ] `go test -race ./...` — P-02, P-04 e P-10.
- [ ] Teste multi-sessão para escopo, supersessão, escritor e filtros de dono — P-04 e P-10 (`TestINV16_*`).
- [ ] Cada invariante com teste nomeado ou débito registrado — P-10; INV-12 registrada como reservada.
- [ ] Nenhuma feature sem `Aprovado: true` e sem Gherkin — spec 001 aprovado; cenários do discovery §4.
- [x] Dependência nova só se listada em `tech.md` — `gopkg.in/yaml.v3`, `fsnotify` e `koanf` (providers/parsers) já registrados em `tech.md`.

**Restrições Arquiteturais**
- [ ] Binário único, módulo único, sem CGO, layout `cmd/ai-memory` + `internal/...` — §1 (acrescenta `internal/config`, ver nota abaixo).
- [ ] Grafo de imports (`domain` sem I/O; `storage`→`domain`; `wiki`→`domain`,`storage`; só `cli` compõe) — §1; teste `TestImportGraph` com `go list -deps`.
- [ ] INV-01 config uma vez, único `os.Getenv` — `internal/config` + P-11 (`.env` não versionado) + teste que falha se `os.Getenv`/`os.LookupEnv`/`os.Environ` aparecer fora dele.
- [ ] INV-02 escritor único, fila 1024, pool de leitura — P-02.
- [ ] INV-03 índices na mesma transação — P-02, P-04 (FTS por triggers).
- [ ] INV-04 identidade `(workspace_id, project_id, path)` — §2.1 (`PageRef`).
- [ ] INV-05 hooks 202/429 — **não se aplica** a esta feature (feature 002); nada em `ingest` aqui.
- [ ] INV-06 sanitização como fronteira de tipo — P-07.
- [ ] INV-09 processo vivo antes de `reset`/`reindex`/`restore`; backup online — P-08 e `Writer.Snapshot` (API de backup do modernc).
- [ ] INV-10 escrita atômica e watcher ignora temporários — P-06 e P-08.
- [ ] INV-11 `data_dir` absoluto e canônico, registrado no log — `config.Load` (`filepath.Abs` + `EvalSymlinks` do pai) e log em `info` na abertura.
- [ ] INV-12 reservada — nenhuma regra imposta.
- [ ] INV-13 zero-LLM — nenhum pacote de provedor; busca sem vetor.
- [ ] INV-15 logs filtram o próprio módulo — `slog.Handler` com filtro por atributo `module` em `cli`.
- [ ] INV-16 multi-sessão (páginas compartilhadas, supersessão, ponteiro por ator) — P-04; handoffs e ponteiro ficam na feature 002.
- [ ] Resolução de escopo central e falha fechada; só escrita explícita cria — `Reader.ResolveScope` vs `Writer.EnsureScope`.
- [ ] Autorização por capacidade e token em tempo constante — **não se aplica** (sem rede nesta feature).
- [ ] Mutações da wiki só pela camada `wiki` — P-03; `storage.UpsertPage` não é chamado fora de `wiki`.
- [ ] Markdown em git é a fonte da verdade; SQLite reconstruível — P-03 e P-08.

**Regras de Negócio Transversais**
- [ ] Idempotência de ingestão — **adiada** para a feature 002 (`ingest_keys` em `V02`).
- [ ] Rollback do arquivo quando o banco falha — P-03.
- [ ] Observação nunca chega ao storage sem sanitizador — P-07 (assinatura exige `Sanitized`).
- [ ] Tombstone de páginas removidas desligado por padrão — P-08.
- [ ] Escopo faseado, sem LLM — plano sem embeddings, rerank ou provedor.

**Preferências de Bibliotecas**
- [ ] `modernc.org/sqlite` (sem mattn) — §1.
- [ ] `koanf` + `cobra` (sem viper) — §1.
- [ ] `google/uuid`, `slog`, `regexp` RE2 — §1.
- [ ] `git` do sistema (sem go-git) — P-09.
- [ ] Stdlib sempre que resolver — migrações, ator e atomicfile são código próprio sobre a stdlib.

**Anti-Padrões Proibidos**
- [ ] Sem `os.Getenv` fora de `config` — teste de grep em CI.
- [ ] Sem escrita de arquivo da wiki fora de `wiki` — `atomicfile` é interno de `wiki`.
- [ ] Sem "indexar depois" fora da transação — P-02/P-04.
- [ ] Sem filtro de leitura por autoria — `pages` não tem `author_id` nesta feature; leitura nunca filtra por ator.
- [ ] Sem criar escopo em leitura/busca/destrutivo — `Reader` não tem método que cria.
- [ ] Sem concatenar caminho à mão — `PagePath` e `wiki.pathFor(scope, path)`.
- [ ] Sem `panic` em caminho de execução — erros tipados; pânico no pool de leitura vira erro.
- [ ] Sem abstração preemptiva, código morto ou stubs — interfaces `Writer`/`Reader` têm chamador (wiki e testes); `InsertObservation` só entra se o `raw_hits` precisar dele em teste, caso contrário sai para a feature 002.
- [ ] Goroutine sempre com `context` e sem fan-out ilimitado — ator, watcher e reconciliação.
- [ ] "A confirmar" tratado como decisão em ADR — timeout de hook (feature 002) e INV-12 já registrados no discovery.
- [ ] Clean-room: nenhuma cópia do binário original — contratos vêm só dos docs.

> **Notas de desvio / pendências**
> 1. `internal/config` é um pacote novo em relação ao layout original da constituição. Não viola o grafo (não importa ninguém) e concentra o único acesso ao ambiente. **Resolvido:** acrescentado à constituição (o autor definiu o `.env` local, P-11).
> 2. **Resolvido:** `tech.md` atualizado com `gopkg.in/yaml.v3`, `fsnotify` e os providers/parsers do `koanf`.
> 3. **Resolvido:** `V01` contém todo o esquema do doc; as tabelas sem uso ficam só como DDL até a feature que as exigir (§2.3).
> 4. `InsertObservation` e a tabela `observations` entram porque o fallback `raw_hits` é exigido pelo escopo; a escrita real de observações só é exercida nos testes até a feature 002.
