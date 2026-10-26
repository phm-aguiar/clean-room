# Plano: Núcleo (domínio, armazenamento e wiki)

## Metadados
- **Feature:** 001 — `specs/features/001-nucleo/spec.md` (Aprovado: true)
- **Aceitação:** `specs/features/001-nucleo/acceptance/nucleo.feature`
- **Notas de contrato:** `notes/domain-contract.md`, `notes/markdown-contract.md`, `notes/search-contract.md`, `notes/wiki-layout-contract.md`
- **Stack:** Go 1.26, módulo único, `CGO_ENABLED=0`; `modernc.org/sqlite`, `gopkg.in/yaml.v3`, `fsnotify`, `google/uuid`, `golang.org/x/text`, `go-git/v5`, `koanf`, `log/slog` (todas em `.github/memory/tech.md`)
- **Alvos:** `windows/amd64` (primeiro MVP) e `linux/amd64`
- **Escopo:** alicerce zero-LLM testável por contratos: domínio sem I/O, configuração carregada uma vez, SQLite com escritor único e busca híbrida RRF, wiki markdown com escrita atômica, supersessão, remoção com quarentena, watcher/reconciliação, reindex e cópia online do banco. Sem hooks, MCP nem CLI de usuário (002).
- **Precedência:** `docs/` > notas > este plano. Em dúvida, comportamento do original.

## Contratos e Fronteiras

### Pacotes e grafo de imports (verificado por `TestImportGraph`)
| Pacote | Responsabilidade | Pode importar |
|---|---|---|
| `internal/domain` | IDs UUIDv7 tipados, `PagePath` (validação básica + `ensure_portable` + chave portável), tiers, tipos de observação, tipo sanitizado e sanitizador, erros de domínio | stdlib, `google/uuid`, `x/text` |
| `internal/config` | Camadas de `docs/cli.md` §2.1, diretório de dados canonicalizado (§2.5), logger `slog` com filtro do próprio módulo | stdlib, `koanf` |
| `internal/storage` | Abertura (pragmas, WAL, `journal_size_limit` 64 MiB), migrações, escritor único, pool de leitura de 4, resolvedor de escopo, busca, cópia online | `domain` |
| `internal/wiki` | Leitura/emissão de frontmatter, escrita atômica, fluxo de escrita/remoção de página, Git, watcher, reconciliação, reindex | `domain`, `storage` |
| `internal/cli` | Composição mínima para testes (sem comandos de usuário) | todos |

`ingest` e `mcp` não existem nesta feature.

### Interfaces internas (formas indicativas; nomes Go são convenção, não contrato)
- **Escritor** (`docs/storage-and-retrieval.md` §3): `Writer.Do(ctx, cmd)` envia a um canal de capacidade 1024 com canal de resposta de uso único; fila cheia **espera**; encerramento descarta a fila e responde `writer actor is no longer running`; `Writer.Occupancy() (queued, capacity int)`. Um comando = uma transação, com FTS, entidades, links, evidências e auditoria juntos.
- **Leitor:** pool somente leitura de 4 conexões, `busy_timeout` 5000 ms; pânico vira `reader pool task did not complete: <detalhe>`; fechado vira `read connection pool is closed`.
- **Resolvedor de escopo** (§14.1): `ResolveRead(ctx, caller, ws, proj) (Scope, ResolvedBy, error)` sem criar; `ResolveWrite(...)` conforme a tabela de escrita; origens `explicit|session|shared_slot|startup_seed|default|default_after_mismatch`. A escrita de página **não** usa `ResolveWrite` criador: só confere que o projeto existe no workspace (`docs/wiki-files-layer.md` §5.2 passo 7).
- **Wiki:** `WritePage(ctx, req) (PageVersion, error)`, `DeletePath(ctx, scope, path, expectedVersion)`, `ReindexPage`, `ReindexAll`; rastreia caminhos alterados para o próximo commit automático (sem commit na 001).
- **Busca** (§11.3, `notes/search-contract.md`): `Query(ctx, scope, q, opts) (Result{Pages, RawHits, ResolvedBy})`; RRF k=60 sobre FTS, entidades e vizinhos de link do mesmo projeto; autoridade limitada a `[0,55; 1,50]`; `raw_hits` quando nenhuma página casa; reforço de acesso throttled 1/min por página e operador (anônimo incluso).

### Persistência
- Esquema e convenções: `docs/database-schema.md` (IDs `BLOB` 16, microssegundos UTC, hashes 32 bytes). Migrações `V<nn>__<nome>.sql` embutidas por `embed.FS`, controladas por `refinery_schema_history` com os tipos de §0; banco com migração desconhecida falha com o texto literal de "schema à frente"; **sem cópia** do banco antes de migrar.
- Disco: árvore de `docs/wiki-files-layer.md` §1 (`<data_dir>/wiki/<ws_id>/<proj_id>/…`, `_meta.md` por escopo, índice em `<data_dir>/db/memory.sqlite`).
- Formato de página: frontmatter conforme §3.1–§3.3 (casos dourados reemitidos com os mesmos bytes).

### Erros
Categorias e textos literais de `docs/storage-and-retrieval.md` §19 e o texto de estado inconsistente de `docs/wiki-files-layer.md` §5.2 passo 9, como valores sentinela/tipos em `domain` e `storage`, testados por igualdade de string.

### Eventos externos e APIs HTTP
Nenhum nesta feature.

## ADRs

- **ADR-001 — Escritor único como goroutine dona da conexão**
  - **Decision:** uma goroutine possui a conexão de escrita e consome `chan command` de capacidade 1024; cada comando traz `chan result` de capacidade 1; o laço termina por `context` e, ao parar, drena a fila respondendo "escritor fechado".
  - **Rationale:** reproduz exatamente §3 (ordem de chegada, espera em fila cheia, descarte no encerramento), dá backpressure natural e evita disputa de lock entre escritores (INV-02, INV-16).
  - **Alternative Rejected:** `sync.Mutex` sobre `*sql.DB` — sem ocupação observável, sem semântica de descarte no encerramento e convida a transações espalhadas.

- **ADR-002 — Runner de migrações próprio compatível com `refinery_schema_history`**
  - **Decision:** runner mínimo em `storage` que lê `V<nn>__<nome>.sql` de `embed.FS`, aplica cada uma numa transação e grava a linha em `refinery_schema_history` com as colunas e tipos de `docs/database-schema.md` §0.
  - **Rationale:** o nome e o formato da tabela são contrato observável (um banco do original precisa abrir); nenhuma biblioteca Go produz essa tabela; o runner é pequeno.
  - **Alternative Rejected:** `golang-migrate`/`goose` — tabela de controle própria (divergência do original) e dependência fora de `tech.md`.

- **ADR-003 — Sanitizador em RE2 com classes Unicode explícitas e fronteira verificada em código**
  - **Decision:** os 27 padrões de `docs/prompts-schemas-and-sanitizer.md` §11.2 são mantidos na ordem do doc, mas compilados com `\s`, `\d` trocados por classes Unicode explícitas (`[\p{White_Space}…]` equivalente, `\p{Nd}`) e `\b` substituído por checagem de fronteira Unicode feita em Go ao redor de cada casamento; passada única conforme §11.1.
  - **Rationale:** RE2 tem `\s\d\b` só ASCII; o doc exige semântica Unicode (ex.: `Bearer` com espaço não separável) e o original não usa retrocesso, então RE2 com classes ajustadas reproduz o resultado sem dependência nova.
  - **Alternative Rejected:** `dlclark/regexp2` — retrocesso (semântica diferente do original e risco de ReDoS) e dependência fora de `tech.md`.

- **ADR-004 — YAML pelo `yaml.Node` com resolução de escalares própria**
  - **Decision:** o frontmatter é decodificado em `yaml.Node` e os escalares sem tag são resolvidos por uma função própria que implementa a tabela de `docs/wiki-files-layer.md` §3.2 (`true`/`false` booleanos, `yes` texto, `007` texto, inteiro fora de 64 bits é erro); a emissão gera texto à mão a partir da ordem de chaves preservada, conforme §3.3.
  - **Rationale:** a resolução implícita do `yaml.v3` e o seu emissor não garantem os mesmos valores nem os mesmos bytes do original; os casos dourados exigem byte a byte.
  - **Alternative Rejected:** `Unmarshal` em `map[string]any` + `yaml.Marshal` — perde ordem, muda tipos em casos de borda e reformata a saída.

- **ADR-005 — Escrita de página com rollback explícito do arquivo**
  - **Decision:** `wiki.WritePage` lê o conteúdo anterior (ou a ausência), instala o arquivo atomicamente (temporário `.ai-memory-tmp.`, `fsync`, `rename`, `fsync` do diretório em melhor esforço; no Windows repete só 32/33 com 10–50 ms reaproveitando o temporário), e só então envia **um** comando ao escritor com página + índices; falha no banco reverte em ordem inversa e, se a reversão falhar, devolve o erro de estado inconsistente literal.
  - **Rationale:** é o fluxo de §5.2 e da regra transversal da constituição; arquivo é a fonte da verdade e o índice nunca fica à frente dele.
  - **Alternative Rejected:** gravar no banco primeiro e depois o arquivo — o índice poderia apontar para conteúdo inexistente após queda, contra "Markdown em git é a fonte da verdade".

- **ADR-006 — Git via `go-git` com fallback para `git` só no Windows**
  - **Decision:** adaptador `wiki/git` com `go-git/v5` como caminho normal (autor fixo `ai-memory <ai-memory@local>`, sem hooks, assinatura nem identidade do usuário, ledgers fora do índice, nenhum commit sem mudança) e fallback `os/exec` restrito às condições de `docs/windows-native-extras.md` §6.2 e `notes/wiki-layout-contract.md` (d). Na 001 o adaptador só **registra** caminhos; o commit automático é da 002, mas inicialização e confinamento do repositório entram aqui.
  - **Rationale:** igual ao original; o `git` não é exigido no PATH.
  - **Alternative Rejected:** só `os/exec` — diverge do original e exige `git` instalado.

- **ADR-007 — Chave portável com `x/text/cases` + NFC**
  - **Decision:** chave = `cases.Lower(language.Und)` (minúsculas completas, incluindo `İ` e sigma final) seguida de `norm.NFC`; colisão com página viva do projeto vira o erro literal de colisão de caminho.
  - **Rationale:** `strings.ToLower` faz só mapeamento simples, e o doc exige o completo (`docs/domain-vocabulary.md` §4.2); `x/text` já está em `tech.md`.
  - **Alternative Rejected:** `strings.ToLower` + `norm.NFC` — diverge nos casos especiais do doc.

- **ADR-008 — Watcher com `fsnotify`, debounce por caminho e reconciliação periódica**
  - **Decision:** `fsnotify` recursivo (registro por diretório), debounce de 300 ms, reconciliação a cada 30 s com o primeiro disparo descartado, filtros de `docs/wiki-files-layer.md` §8.1, disjuntor de 5 falhas (`watcher_degraded`); goroutines canceladas por `context`. Escritas próprias absorvidas pelo prefixo do temporário e pela regra de conteúdo idêntico (sem inode/NTFS id).
  - **Rationale:** reproduz §8 e cobre perdas de evento do `fsnotify` (especialmente em OneDrive/Windows) pela reconciliação.
  - **Alternative Rejected:** só polling — latência e custo maiores, diverge do debounce de 300 ms do original.

## Auditoria da Constituição

### Portões de Qualidade
- [x] Build `CGO_ENABLED=0` + `go vet` para `windows/amd64` e `linux/amd64` — todas as dependências são pure Go (ADR-002/003/004/006).
- [x] `go test -race ./...` — previsto; tasks devem incluir execução em container `golang:1.26` ou gcc local.
- [x] `golangci-lint run` — configuração criada na task de bootstrap do módulo.
- [x] Teste multi-sessão explícito para supersessão, escritor SQLite e resolução de escopo (INV-16).
- [x] Teste de contrato `TestINVnn_...` para INV-01..06, 09..11, 13, 15 e 16 (critério da spec). INV-05 nesta feature cobre só o primitivo de ocupação da fila; `/hook` é da 002. INV-09 cobre a guarda de processo vivo do `reindex`; `reset`/`restore`/`uninstall`/`backup` são da 002.
- [x] Valores literais contra docs e `docs/assets/fixtures/` (erros §19, casos dourados de YAML, fixtures de busca e sanitização).
- [x] Spec com `Aprovado: true` e `acceptance/nucleo.feature`.
- [x] Nenhuma dependência fora de `tech.md` (ADR-002 e ADR-003 rejeitam bibliotecas não listadas).

### Restrições Arquiteturais
- [x] Binário único, módulo único, layout `cmd/ai-memory` + `internal/…` (só os pacotes da 001).
- [x] Grafo de imports aplicado por `TestImportGraph`; `domain` sem I/O; `config` único leitor de arquivo/ambiente.
- [x] INV-01 — configuração carregada uma vez em `config`, camadas de `docs/cli.md` §2.1, sem `.env`.
- [x] INV-02 — ADR-001; pool de leitura somente leitura de 4.
- [x] INV-03 — índices na mesma transação (ADR-001/005).
- [x] INV-04 — identidade `(workspace_id, project_id, path)` tipada no domínio e em toda linha.
- [~] INV-05 — fora do escopo (hooks são da 002); só a ocupação da fila do escritor é exposta.
- [x] INV-06 — sanitizador antes do teto; barreira de tipo só para observações; páginas sanitizadas pelo chamador; reindex sem higienizar (ADR-003).
- [~] INV-09 — guarda de processo vivo no `reindex`; trava `.serve.lock` e backup online por HTTP ficam para a 002 (a 001 entrega o primitivo de cópia online do SQLite).
- [x] INV-10 — escrita atômica de ADR-005; watcher ignora `.ai-memory-tmp.`; ledger fora do escopo.
- [x] INV-11 — diretório de dados absoluto, canonicalizado como no original e registrado no log.
- [x] INV-12 — reservada, nada a fazer.
- [x] INV-13 — feature inteira zero-LLM.
- [x] INV-15 — handler `slog` filtra o próprio módulo.
- [x] INV-16 — leitura sem filtro de autoria; supersessão preserva perdedora; ponteiro por ator no resolvedor. Handoff exatamente uma vez: fora do escopo (002).
- [x] Resolução de escopo central; leitura não cria; escrita de página não cria; `reindex` recria a partir do `_meta.md` mantendo UUIDs.
- [~] Autorização por capacidade e tokens em tempo constante — sem rotas nesta feature; nada a implementar.
- [x] Mutações da wiki só pela camada `wiki`.
- [x] Markdown em git como fonte da verdade; SQLite reconstruível (`reindex`).

### Regras de Negócio Transversais
- [~] Idempotência por chave de ingestão — ingestão é da 002.
- [x] Falha no upsert após instalar o arquivo reverte o arquivo e propaga o erro (ADR-005).
- [x] Lápide de páginas sumidas opcional e desligada por padrão.
- [x] Fase zero-LLM respeitada; INV-07/08/14 não se aplicam.

### Preferências de Bibliotecas
- [x] `modernc.org/sqlite`, sem `mattn/go-sqlite3`.
- [~] MCP — fora do escopo.
- [x] `koanf` (sem `viper`); `cobra` só quando houver CLI (002).
- [x] UUIDv7 via `google/uuid`; logs via `log/slog`.
- [x] Sanitizador com semântica Unicode sem retrocesso (ADR-003).
- [x] `go-git/v5` com fallback `git` só no Windows (ADR-006).
- [x] Stdlib preferida (runner de migrações e emissor de YAML próprios só onde o contrato exige).

### Anti-Padrões Proibidos
- [x] Sem leitura de ambiente fora de `config`; sem segredos versionados.
- [x] Sem escrita de arquivo da wiki fora de `wiki`.
- [x] Sem "indexar depois".
- [x] Sem filtro de leitura por autoria.
- [x] Sem criação de escopo em leitura, busca ou remoção.
- [x] Caminho de página só via `domain.PagePath`.
- [x] Sem `panic` em caminho de execução (pânico no pool é recuperado e vira erro tipado).
- [x] Sem abstração preemptiva nem stubs: interfaces só onde há dois consumidores ou teste (escritor, Git com fallback).
- [x] Goroutines (escritor, watcher, reconciliação) com cancelamento por `context`; sem fan-out ilimitado.
- [x] Sem "melhorar" o original (ex.: sem herança de `pinned`, regravação de arquivo idêntico mantida).
- [x] "A confirmar" resolvido pelo original (spec, seção de correções da auditoria).
- [x] Clean-room: nada copiado além dos docs.

Legenda: `[x]` atendido pelo plano; `[~]` não se aplica a esta feature (justificado).
