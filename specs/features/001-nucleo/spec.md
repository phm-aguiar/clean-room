# Spec: Núcleo (domínio, armazenamento e wiki)

## Metadados
- **ID:** 001
- **Status:** concluída (2026-10-13), com as correções contra os docs abertas como tasks na 002 (ver "Revisão contra os docs")
- **Autor:** phm-aguiar
- **Data:** 2026-10-12
- **Feature Anterior:** — (primeira feature)
- **Discovery Report:** `reports/001-ai-memory-go-clean-room-discovery.md` (F001)
- **Aprovado:** true

## Propósito
> Para quem, o quê e por quê.

O ai-memory-go é uma reimplementação clean-room, em Go, do ai-memory: um servidor único que dá memória de
longo prazo a agentes de código. Antes de capturar sessões ou expor MCP, é preciso o alicerce que todo o
resto consome: um domínio tipado sem I/O, um índice SQLite com escritor único e busca híbrida, e uma wiki
em markdown versionada em git, que é a fonte da verdade. O público é o autor (dev solo, Windows e Linux).

Importa agora porque as invariantes (sanitização como fronteira de tipo, índice na mesma transação dos
dados, escrita atômica, isolamento por `(workspace_id, project_id, path)`) são caras de retrofitar. Esta
feature é **zero-LLM** e entrega o núcleo testável por contratos, sem hooks nem MCP.

## Escopo

### Dentro do escopo
- **Domínio** (`internal/domain`, sem I/O): UUIDv7, identidade `(workspace_id, project_id, path)`, caminho de página portável, tiers, tipos de observação, tipo sanitizado e sanitizador.
- **Configuração** (carregada uma vez): TOML + `AI_MEMORY_*` com `__`, diretório de dados absoluto e canônico registrado no log, logger `slog` com filtro do próprio módulo.
- **Armazenamento** (`internal/storage`): SQLite pure-Go (WAL, FTS5), migrações `V<nn>__nome.sql` com snapshot antes de migrar, escritor único (goroutine + canal de 1024), pool de leitura, resolução central de escopo que falha fechado.
- **Busca**: FTS5 + casamento de entidades + vizinhos de link, fundidos por RRF (k = 60), multiplicador de autoridade `[0,55; 1,50]`, fallback para observações cruas e reforço de acesso throttled (1/min por página e operador).
- **Wiki** (`internal/wiki`): escrita atômica com frontmatter, upsert na mesma transação com rollback do arquivo, supersessão, `log.md`, commit git por evento, watcher (debounce 300 ms) e reconciliação de 30 s.
- **Reindex** a partir do markdown (o SQLite é derivado) e backup online do SQLite.

### Fora do escopo (explicitamente)
- Hooks, `/hook`, spool, sessões e handoffs (**feature 002**).
- Servidor MCP e CLI de usuário, exceto o mínimo para executar os testes (**feature 002**).
- LLM, embeddings, reranker, consolidação, lint por LLM (features 016 a 022, ver `roadmap.md`).
- Multiusuário, SSO, autorização por projeto, webhooks de admissão, perfil entre projetos (features 007, 008, 009 e 012, ver `roadmap.md`).
- Navegador web, workstreams, macOS e demais agentes.

## Comportamento Esperado

### Cenário Principal (Happy Path)
1. O chamador pede a escrita da página `decisions/db.md` no projeto `demo`; o escopo é resolvido pelo resolvedor central.
2. O corpo e o frontmatter passam por validação do caminho e pela sanitização.
3. O arquivo é instalado de forma atômica (temporário, `fsync`, `rename`, `fsync` do diretório) e a página e os índices FTS, entidades e links commitam em **uma** transação do escritor único.
4. Um commit git é criado, uma linha é anexada ao `log.md` e a resposta traz identificador, caminho e checkpoint.
5. Uma consulta posterior retorna a página, ranqueada por RRF entre FTS, entidades e vizinhos de link, com autoridade aplicada.

### Cenários Alternativos
- **Falha no banco após instalar o arquivo:** o arquivo é revertido, o erro é propagado de forma não ambígua, nenhum commit é criado e o índice permanece inalterado.
- **Leitura sem escopo resolvível:** falha fechada com erro de escopo; nenhum workspace ou projeto é criado.
- **Nenhuma página compilada casa:** a consulta devolve `raw_hits` por FTS nas observações do escopo.

## Edge Cases
- **Escrita concorrente no mesmo caminho:** uma versão é escrita e a outra supersede a anterior numa cadeia; a perdedora continua alcançável; a leitura não é filtrada por autoria (INV-16).
- **Edição manual da wiki no disco:** o watcher (300 ms) ou a reconciliação (30 s) reindexa; escritas do próprio servidor são ignoradas pelo prefixo do temporário.
- **Segredo no texto de entrada:** o segredo é removido **antes** de limitar tamanho (título ≤ 80 chars, corpo ≤ 16 KiB); o tipo sanitizado só nasce da função de sanitização.
- **Banco corrompido ou ausente:** `reindex` reconstrói o SQLite a partir do markdown, com guarda de processo vivo.

## Constraints
- **Performance:** consulta com 5 mil páginas < 100 ms (baseline registrado em benchmark); leitura nunca bloqueia no escritor.
- **Tecnologia:** Go, `modernc.org/sqlite`, `git` do sistema, `slog`, `regexp` RE2; `CGO_ENABLED=0`; módulo único; grafo de imports da constituição.
- **Prazo:** não especificado.
- **Segurança/Compliance:** segredos nunca persistidos crus; comparação de token em tempo constante; escuta padrão `127.0.0.1`; diretório de dados absoluto e canônico.

## Critérios de Sucesso
- [x] `CGO_ENABLED=0 go build ./...` e `go vet ./...` passam para `windows/amd64` e `linux/amd64`
- [x] `go test -race ./...` passa, incluindo testes multi-sessão (supersessão e escritor)
- [x] Escrever uma página produz arquivo, linhas no SQLite e commit git consistentes; reverter quando o banco falha
- [x] Busca RRF retorna os resultados esperados nas fixtures de `docs/assets/fixtures`
- [x] Nenhum segredo cru persistido nas fixtures de sanitização
- [x] Invariantes INV-01..06, 09..11, 13, 15 e 16 têm teste de contrato nomeado ou débito registrado
- [x] Testes automatizados cobrem o happy path e os edge cases identificados

> **Evidência (2026-10-13):** `sh scripts/check.sh` completo e verde (build cruzado, `go vet` nos dois alvos e `go test -race ./...` em todos os pacotes, no container `golang:1.26` com gcc 14.2) e `golangci-lint` v2.14.0 com 0 apontamentos, na última versão do código Go da 001 (depois de 001/T083 a T085). Uma repetição posterior foi interrompida por queda do Docker Desktop, sem mudança de código entre as duas. Testes: `test/acceptance/write_page_test.go` e `rollback_test.go` (critério 3), `search_rrf_test.go` (4), `secret_sanitization_test.go` (5), `test/contract/*_inv_test.go` e `notes/inv-test-matrix.md` (6), `concurrent_supersession_test.go` e `manual_edit_test.go` (multi-sessão e edge cases). CI do GitHub fora do critério, por decisão do autor (001/T090).

## Abordagem Escolhida
Módulo Go único com camadas em `internal/`, sem CGO. O domínio não faz I/O. O armazenamento usa
`modernc.org/sqlite` e uma goroutine dona da conexão de escrita (canal de capacidade 1024, o que dá
backpressure natural). A wiki usa o `git` do sistema e a escrita atômica de arquivo. Decisões em ADR-01,
02, 03, 04, 07, 08 e 10 do discovery.

Débitos relevantes: `docs-adiados/` ausente na época (hoje em `docs/prompts-schemas-and-sanitizer.md` §11; alinhamento na 002/T093) (os padrões de segredo do sanitizador serão reescritos a
partir dos docs e de fixtures próprias); pontos "a confirmar" decididos por nós via ADR; desempenho do
FTS5 do modernc ainda não medido.

### Alternativas Consideradas
| Abordagem | Trade-off | Por que não |
|-----------|-----------|-------------|
| `mattn/go-sqlite3` (CGO) | Mais rápido e maduro | Exige toolchain C; complica build e cross-compile no Windows |
| `go-git` | Sem dependência externa de git | Operações lentas ou incompletas; ignora configuração do repositório do usuário |
| `go.work` com módulos por componente | Isolamento forte | Overhead alto neste estágio; o grafo é verificável por teste de imports |
| Mutex sobre uma conexão | Mais simples | Sem backpressure nem lote de comandos |

## Revisão contra os docs (2026-10-13)
> Feita antes de executar a 002. Neste projeto o doc vence a spec. A 001 continua implementada como foi aprovada; as divergências abaixo viram tasks de correção na 002 (`specs/features/002-captura-e-acesso/tasks.md`, fase 1b).

| Spec 001 dizia | Doc define | Fonte | Correção |
|---|---|---|---|
| Diretório de dados configurável (padrão `data`, relativo ao diretório atual); só `--config` | Padrão no diretório de dados local do usuário, subpasta `ai-memory`; `--data-dir` > `AI_MEMORY_DATA_DIR` > arquivo; `config.toml` no diretório de dados | `docs/design-decisions.md` (diretório de dados padrão), `docs/cli.md` §1, §2.1 e §2.5 | 002/T079 |
| "commit git por evento" (cada escrita de página faz commit `page-write`) | Escritas rastreadas por caminho; commits automáticos em pontos definidos (fim de sessão com `session <id8>: <título>`, checkpoints, comando de commit, `memory_write_page`) | `docs/wiki-files-layer.md` §7 | 002/T080 |
| "backup online do SQLite" (`backup --to` grava um arquivo SQLite) | `backup --to` (ou `-o`) pede ao servidor um tarball gzip com `wiki/`, `db/memory.sqlite` e `config.toml` | `docs/lifecycle-ops.md` § backup, `docs/cli.md` §9, `docs/mcp-server.md` §6 | 002/T081 e T082 |
| Reindex reconstrói o nome do escopo pelo id quando falta `_meta.md` | Manifesto ausente é erro com o caminho exato | `docs/lifecycle-ops.md` § reindex | 002/T083 |
| Reindex sem checagem do banco | Recusa se as tabelas reconstruídas já têm linhas e nomeia as contagens | `docs/lifecycle-ops.md` § reindex | 002/T084 |
| (ausente) | Boot do servidor preenche `_meta.md` faltante de forma idempotente | `docs/cli.md` §3.1 (passo 11), `docs/lifecycle-ops.md` § reindex | 002/T085 |
| Watcher só conta remoções | Remoção registra o caminho no Git para o próximo commit | `docs/wiki-files-layer.md` §8.1 | 002/T086 |
| Ledger reconhecido pelo nome (`log*.md`) | Nome exato (`log.md` ou `log-AAAA-MM.md`) e conteúdo que abre com entrada, até 8 KiB | `docs/wiki-files-layer.md` §9 | 002/T022 |
| Guarda de processo vivo por `ai-memory.pid` | `.serve.lock` e enumeração dos processos do sistema | `docs/cli.md` §3.1, `docs/overview.md` item 9 | 002/T040 e T041 |
| Escrita e exclusão de página acrescentam linha em `log.md`, garantem `index.md` e fazem commit | Escrita de página sem linha de log, sem `index.md` e sem commit; ledger é só de eventos de hook; `log.md` não é adotado; `index.md` só pela migração OKF; páginas de `_slots/` são fixadas | `docs/wiki-files-layer.md` §5.2, `docs/okf.md` (arquivos no diretório da wiki) | 002/T080 e T089 |
| Frontmatter só aceita texto e lista de texto; mapeamento é recusado (001/T084 manteve a recusa); `generated.by`/`generated.at` gravados como chaves planas; `relations` lido como lista de texto | `generated: {by, at}` é mapeamento; `sources` é lista de mapeamentos; `relations` é mapeamento por relação (`{fixes: [...]}`). Um arquivo escrito à mão no formato do doc era recusado inteiro | `docs/okf.md` § Proveniência, `docs/typed-edges.md` | 002/T090 e T091 |
| Relação desconhecida no frontmatter vira `references` | Chave desconhecida é ignorada com aviso (conflito com `docs/domain-vocabulary.md` §6.8, resolvido pelo doc específico) | `docs/typed-edges.md`, `docs/wiki-files-layer.md` §3.5 | 005/T009 |
| Links reapontados só na supersessão | Toda nova versão reaponta todas as linhas cujo alvo casa com o caminho, inclusive as já resolvidas | `docs/typed-edges.md` § Storage | 005/T022 |
| Exclusão por caminho remove só a versão corrente; versão diferente da esperada é erro | Remove fisicamente todas as versões do caminho; com versão diferente, não faz nada e informa que não removeu | `docs/storage-and-retrieval.md` §5.2 | 002/T087 |
| Ressurreição mantém o `superseded_at` da lápide e fecha o `valid_to` | Limpa o `superseded_at` da lápide e mantém o `valid_to` original | `docs/storage-and-retrieval.md` §5.1 (caso C) | 002/T088 |
| Origens de escopo `explicit`, `default`, `repo_path` | `Explicit`, `Session`, `SharedSlot`, `StartupSeed`, `Default`, `DefaultAfterMismatch` | `docs/storage-and-retrieval.md` (resolução de escopo) | 002/T020 |

## Dependências
- `docs/overview.md`, `domain-vocabulary.md`, `database-schema.md`, `storage-and-retrieval.md`, `wiki-files-layer.md` (fonte dos contratos).
- `docs/assets/fixtures` e `docs/test-fixtures.md` (golden files para os testes de contrato).
- `.github/memory/constitution.md` e `.github/memory/tech.md`.

## Checklist de Prontidão
- [x] Propósito claro e sem ambiguidade
- [x] Escopo delimitado (dentro/fora)
- [x] Cenários cobrem happy path e alternativos
- [x] Edge cases identificados com comportamento esperado
- [x] Constraints explícitas
- [x] Critérios de sucesso mensuráveis
- [x] Abordagem escolhida justificada
