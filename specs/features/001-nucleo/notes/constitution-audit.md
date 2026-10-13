# Auditoria da constituição: 001-nucleo

Data: 2026-10-12. Escopo: tarefa T072. Auditor: analista (somente leitura; nenhum arquivo do projeto foi editado).

Método: leitura de `.github/memory/constitution.md`, `plan.md` §4, `tasks.md`, `tech.md`, `scripts/check.sh`, `.github/workflows/ci.yml`, `.golangci.yml`, `go.mod` e as notas `inv-test-matrix.md`, `bench-baseline.md` e `phase0-findings.md`. Grep em `internal/`, `cmd/` e `test/`.

Comandos executados na auditoria:
- `CGO_ENABLED=0 go build ./...` e `go vet ./...` para `windows/amd64` e `linux/amd64`: os quatro passaram.
- `go test ./internal/archtest/`: ok (grafo de imports e guarda de ambiente).
- `gofmt -l .`: sem saída.
- `which gcc cc`: ausentes.

Não foram executados a suíte completa nem `-race`. Nomes de teste, arquivos e linhas vêm de grep ou leitura. Quando a afirmação depende só das notas, isso é dito.

## 1. Resumo

| Status | Qtde |
|---|---|
| CONFORME | 32 |
| NÃO CONFORME | 1 |
| PARCIAL | 7 |
| NÃO VERIFICÁVEL AQUI | 3 |
| FORA DA 001 (não aplicável, registrado) | 7 |

Veredito: **COM PENDÊNCIAS.** O que bloqueia:
1. **INV-01 (NÃO CONFORME):** `reindex` chama `config.Load` duas vezes no mesmo processo. Correção pequena (P1).
2. **`go test -race ./...` não executado** (sem gcc). A execução só é provável no CI (windows-latest e ubuntu-latest), que também nunca rodou. Portão obrigatório pela constituição, em aberto.
3. **golangci-lint e CI sem execução.** `gofmt` está limpo; o binário do linter não existe aqui.

Não bloqueiam, mas pedem decisão ou registro em ADR: INV-06 ("a confirmar"), escopo de INV-10 (appends e Windows), criação de escopo por `reindex` e watcher, e superfície pública sem chamador de produção.

## 2. Portões de qualidade

| # | Portão | Status | Evidência |
|---|---|---|---|
| G1 | `CGO_ENABLED=0 go build` e `go vet` em windows/amd64 e linux/amd64 | CONFORME | Executados nesta auditoria: 4 de 4 ok. `scripts/check.sh` passos 1 e 2 fazem o mesmo. |
| G2 | `go test -race ./...` passa | NÃO VERIFICÁVEL AQUI | Sem gcc nem cc no PATH. `scripts/check.sh` passo 3 sai com código 2. `ci.yml` instala MinGW no Windows e roda `sh scripts/check.sh`, mas nunca rodou no GitHub Actions. |
| G3 | Teste multi-sessão explícito (escopo, supersessão, escritor, filtros de dono) | CONFORME (existência) | `test/acceptance/concurrent_supersession_test.go` `TestConcurrentSupersessionMesmoCaminho`; `test/contract/wiki_inv_test.go` `TestINV16_SupersedeNuncaDestroi`, `TestINV16_ReadPageDevolveCorrenteSemFiltrarAutoria`, `TestINV16_EscritaConcorrenteDivergenteSupersede`; `test/contract/storage_inv_test.go` `TestINV02_EscritorSerializaConcorrentes`; `test/acceptance/scope_fail_closed_test.go` `TestScopeLeituraSemEscopoResolvelFalhaFechada`. Ressalva: a execução sob `-race` depende de G2. Divergência de plano: P-10 e ADR-16 falam em `t.Parallel`, mas não há uso dele; a concorrência vem de goroutines e canal de largada. |
| G4 | Cada INV tem teste nomeado `INV-nn` ou débito em ADR | CONFORME | `grep -rn "func TestINV" test/contract` dá 23 funções, o mesmo número da matriz `inv-test-matrix.md`. INV-05 e INV-12 fora/reservadas. Ressalva: a matriz só confere nome e existência, não o conteúdo. |
| G5 | Feature só com `Aprovado: true` e Gherkin | CONFORME | `specs/features/001-nucleo/spec.md:10` "**Aprovado:** true". Gherkin em `acceptance/nucleo.feature`. |
| G6 | Dependência nova só se listada em `tech.md` | CONFORME | Diretas do `go.mod`: fsnotify, google/uuid, knadh/koanf, spf13/cobra, golang.org/x/text, gopkg.in/yaml.v3, modernc.org/sqlite. Todas constam em `tech.md`; x/text como `unicode/norm` (F0-ADR-01). `mattn/go-isatty` é só indireta; não há `mattn/go-sqlite3`. |
| G7 | Formatação e lint (T068) | PARCIAL | `gofmt -l .` sem saída (conforme). `golangci-lint` v2.1.6 só roda no CI; sem binário aqui. `.golangci.yml` existe. |
| G8 | CI executado | NÃO VERIFICÁVEL AQUI | `ci.yml` nunca rodou, segundo o próprio cabeçalho. |

## 3. Restrições arquiteturais

| # | Restrição | Status | Evidência |
|---|---|---|---|
| R1 | Binário único, módulo único, sem CGO, layout | CONFORME | `cmd/ai-memory/main.go` chama `cli.NewRootCommand().Execute()`. `internal/` tem archtest, cli, config, domain, storage, wiki. `ingest` e `mcp` ausentes por escopo (002). |
| R2 | Grafo de imports | CONFORME | `internal/archtest/archtest_test.go`: `TestImportGraph` passou nesta auditoria; `TestImportGraphDetectsViolation` prova a detecção. `domain` não faz I/O (único `os` é em `sanitize_test.go`). |
| R3 | INV-01 carga única de config | **NÃO CONFORME** | `internal/cli/reindex_cmd.go:46` chama `config.Load(opts)` e na linha 55 `Compose(ctx, opts)`, que chama `config.Load` de novo em `internal/cli/root.go:82`. O comentário em `root.go:2` ("chama config.Load uma única vez") fica falso para `reindex`. Por que os testes não pegam: `TestINV01_ConfigCarregadaUmaVez` só exige que os chamadores estejam em `internal/cli/`, não que haja uma chamada por execução. Ação: P1. |
| R4 | INV-01 leitura de ambiente fora de config; `.env` não versionado | CONFORME | Grep de `os.Getenv/LookupEnv/Environ` em produção: zero fora de `internal/config`. Ocorrências só em testes (montar ambiente de processo filho). `config.go` lê o ambiente via provider `env` do koanf. `.gitignore` exclui `.env`; não há `.env` na raiz. `.env.example` tem só 4 chaves sem segredo. Decisão D4 sobre testes. |
| R5 | INV-02 escritor único, fila 1024, pool de leitura | CONFORME | `internal/storage/writer.go:11` `queueCap = 1024`; `writer.go:54` `go w.run()`. Testes `TestINV02_*` (3). |
| R6 | INV-03 índices na mesma transação | CONFORME | FTS por triggers em `V01__init.sql`; `storage/delete.go` faz delete, órfãs e auditoria na mesma `Writer.Do`. Testes `TestINV03_IndicesMesmaTransacaoAposRetorno`, `TestINV03_FalhaDesfazIndicesEPagina`. |
| R7 | INV-04 identidade `(workspace, project, path)` | CONFORME | `domain.PageRef`, `NewPageRef`, `NewScope`. Testes `TestINV04_*` (4). |
| R8 | INV-05 hooks 202/429 | FORA DA 001 | Feature 002. |
| R9 | INV-06 sanitização como fronteira de tipo | PARCIAL | `Sanitized` tem campo privado `text` (`internal/domain/sanitize/sanitize.go:43`); só `Title` e `Body` o preenchem. `domain.NewObservation` recebe `sanitize.Sanitized`. O único INSERT em observações é `storage.InsertObservation`. Lacunas: (a) o zero value `Sanitized{}` é criável fora do pacote, mas é texto vazio; (b) nenhum teste nomeado prova a fronteira de tipo; (c) a constituição ainda diz "(a confirmar)" e não há ADR; (d) divergência de plano: `plan.md` §2.1 e P-07 falam em `sanitize.Text()` e `InsertObservation` como método de `Writer`; o código tem `Title`/`Body` e função livre. Ação: P4. |
| R10 | INV-09 processo vivo e backup online | CONFORME | `internal/cli/liveguard.go`; `reindex_cmd.go:50` chama `CheckNoLiveProcess` antes de `Compose`. Teste `TestINV09_GuardaDeProcessoVivoAntesDeReindex`. Backup via `storage.Snapshot`. `reset`, `restore` e `uninstall` não existem na 001. |
| R11 | INV-10 escrita atômica; watcher ignora temporários | PARCIAL | `internal/wiki/atomicfile.go` com variantes Unix e Windows. Desvios: (a) fsync do diretório é no-op no Windows (P-06, "melhor esforço"); (b) `internal/wiki/ledger.go:59` faz `O_APPEND` + `Sync` no `log.md`, sem temporário+rename; (c) `gitrepo.go:173` cria `.gitignore` com `O_EXCL`; (d) fora da wiki, `storage/open.go:197` copia o banco para o snapshot pré-migração e `storage/snapshot.go:62` grava o destino do backup. Ação: D2 e P5. |
| R12 | INV-11 `data_dir` absoluto e canônico, registrado no log | CONFORME | `config.go:202` `canonicalPath` (`filepath.Abs` + `EvalSymlinks`); `cli.LogStartup` em `root.go:76`. Teste `TestINV11_DataDirAbsolutoECanonicoNoLog`. |
| R13 | INV-12 reservada | FORA DA 001 (reservada) | Sem regra; registrada na matriz. |
| R14 | INV-13 zero-LLM | CONFORME | `go.mod` sem provedor, embedding nem cliente HTTP de LLM. Teste `TestINV13_FuncionaSemProvedorLLM`. |
| R15 | INV-15 logs filtram o próprio módulo | CONFORME | `internal/cli/logging.go`; `NewLogger` em `root.go:74`. Teste `TestINV15_LogIgnoraProprioModulo`. |
| R16 | INV-16 multi-sessão (parte da 001) | CONFORME | Supersessão, perdedora alcançável e leitura sem filtro por autoria (testes listados em G3). `grep author_id` em produção: zero. `storage.DeletePath` não toca versões superseded. |
| R16b | INV-16 handoff com duas guardas e ponteiro de projeto ativo por ator | FORA DA 001 | `plan.md` §4 diz que ficam na 002. A 002 ainda não tem `plan.md`. Ver D5. |
| R17 | Resolução de escopo central; leitura e busca sem criar | PARCIAL | `storage.ResolveScope` não cria; o único INSERT em `workspaces`/`projects` está em `scope_write.go` (`EnsureScope`). Ponto aberto: `wiki.Reindex` (`reindex.go:238`) e o watcher (`watcher.go:239`) chamam `EnsureScope` ao descobrir diretórios UUID no disco. Ver D3. |
| R18 | Autorização por capacidade; tokens em tempo constante | FORA DA 001 | Sem rede nesta feature. |
| R19 | Mutações da wiki só pela camada `wiki` | CONFORME | `UpsertPage`/`UpsertPageTx` em produção só em `internal/wiki/write.go:205` e `reindex.go:131`. |
| R20 | Markdown em git é a fonte da verdade; SQLite reconstruível | CONFORME | `wiki.Reindex` e comando `reindex`; rollback de arquivo em falha de banco (`write.go:502`). Testes de aceitação e de reindex. |
| R21 | INV-07, INV-08, INV-14 | FORA DA 001 | Valem a partir da F003. |

## 4. Regras de negócio, bibliotecas e anti-padrões

| # | Item | Status | Evidência |
|---|---|---|---|
| N1 | Idempotência de ingestão | FORA DA 001 | Feature 002. |
| N2 | Falha do banco após instalar arquivo reverte o arquivo | CONFORME | `wiki/write.go:502`; `TestRollbackFalhaDoBancoReverteArquivo`; `ErrInconsistentState` quando a restauração falha. |
| N3 | Observação nunca chega ao armazenamento sem sanitizador | PARCIAL | Pelo tipo (R9). Nenhum chamador de produção grava observação; a regra vale pela assinatura. |
| N4 | Tombamento desligado por padrão | CONFORME | Não há código de tombstone na 001; o watcher só conta remoções (`Removals()`). |
| N5 | Escopo faseado, sem LLM | CONFORME | Sem `ingest`, `mcp` nem provedor. |
| L1 | `modernc.org/sqlite`, sem `mattn/go-sqlite3` | CONFORME | `go.mod`: `modernc.org/sqlite v1.60.1`. |
| L2 | MCP `mark3labs/mcp-go` | FORA DA 001 | Ausente do `go.mod`. |
| L3 | `cobra` e `koanf`; sem `viper` | CONFORME | `go.mod`; `grep viper` zero. |
| L4 | `google/uuid`, `log/slog`, `regexp` RE2 | CONFORME | Presentes no código e no `go.mod`. |
| L5 | Git via `os/exec`; sem `go-git` | CONFORME | `internal/wiki/gitrepo.go:9` importa `os/exec`. |
| A1 | Proibido ler ambiente ou `.env` fora de `internal/config` | CONFORME | Ver R4. |
| A2 | Proibido escrever arquivo da wiki fora da camada `wiki` | CONFORME | `os.Create`/`os.WriteFile` zero em produção; `os.Rename` só em `wiki/atomicfile*.go`. |
| A3 | Proibido "indexar depois" | CONFORME | FTS por triggers na mesma transação. |
| A4 | Proibido filtrar leitura por autoria | CONFORME | `grep author_id` em produção: zero. |
| A6 | Proibido concatenar caminho de página à mão | CONFORME (por amostra) | Caminho de página sai de `domain.PagePath` e `wiki.pathFor`. |
| A7 | Proibido `panic` em caminho de execução | CONFORME | `grep "panic("` em produção: zero. |
| A8 | Proibido abstração preemptiva, superfície sem chamador e código morto | PARCIAL | `storage.Search`, `ResolveScope`, `Recent`, `PageChain`, `InsertObservation` e `BumpAccess` sem chamador em outros pacotes; `wiki.NewWatcher` sem composição na `cli` (não há `serve`). Ver P7. |
| A9 | Proibido goroutine sem `context` e fan-out ilimitado | CONFORME | Única goroutine em produção: `writer.go:54`, com cancelamento e `Close` idempotente. `Watcher.Run(ctx)` respeita `ctx.Done()`. |
| A10 | "a confirmar" tratado como decisão em ADR | PARCIAL | Decisões da Fase 0 em `phase0-findings.md`. INV-06 continua "(a confirmar)" sem ADR próprio. |
| A11 | Clean-room: nenhuma cópia do binário original | NÃO VERIFICÁVEL AQUI | Não há como provar por grep. Exige revisão humana. |

## 5. Checklist do plan.md §4, item a item

Observação: no `plan.md`, só o item de `tech.md` está marcado `[x]`; os demais continuam `[ ]`. Não foram atualizados.

**Portões de Qualidade**
1. Build e vet em windows/amd64 e linux/amd64 com `CGO_ENABLED=0`: CONFORME (G1).
2. `go test -race ./...`: NÃO VERIFICÁVEL AQUI (G2).
3. Teste multi-sessão (`TestINV16_*`): CONFORME em existência (G3). Execução sob `-race` pendente.
4. Cada invariante com teste nomeado: CONFORME (G4).
5. `Aprovado: true` e Gherkin: CONFORME (G5).
6. Dependência só se listada em `tech.md`: CONFORME (G6).

**Restrições Arquiteturais**
7. Binário único, módulo único, sem CGO, layout: CONFORME (R1).
8. Grafo de imports: CONFORME (R2).
9. INV-01: PARCIAL (R4 conforme; R3 não conforme no `reindex`).
10. INV-02: CONFORME (R5).
11. INV-03: CONFORME (R6).
12. INV-04: CONFORME (R7).
13. INV-05: FORA DA 001 (R8).
14. INV-06: PARCIAL (R9).
15. INV-09: CONFORME (R10).
16. INV-10: PARCIAL (R11).
17. INV-11: CONFORME (R12).
18. INV-12: FORA DA 001, reservada (R13).
19. INV-13: CONFORME (R14).
20. INV-15: CONFORME (R15).
21. INV-16: CONFORME na parte da 001 (R16); handoff e ponteiro por ator na 002 (R16b).
22. Resolução de escopo central e falha fechada: PARCIAL (R17).
23. Autorização por capacidade: FORA DA 001 (R18).
24. Mutações da wiki só pela camada `wiki`: CONFORME (R19).
25. Markdown em git como fonte da verdade: CONFORME (R20).

**Regras de Negócio Transversais**
26. Idempotência de ingestão: FORA DA 001 (N1).
27. Rollback do arquivo quando o banco falha: CONFORME (N2).
28. Observação nunca chega ao storage sem sanitizador: PARCIAL (N3).
29. Tombstone desligado por padrão: CONFORME (N4).
30. Escopo faseado, sem LLM: CONFORME (N5).

**Preferências de Bibliotecas**
31. `modernc.org/sqlite`, sem mattn: CONFORME (L1).
32. `koanf` e `cobra`, sem viper: CONFORME (L3).
33. `google/uuid`, `slog`, `regexp` RE2: CONFORME (L4).
34. `git` do sistema, sem go-git: CONFORME (L5).
35. Stdlib sempre que resolver: CONFORME (única dependência fora do previsto é `golang.org/x/text`, justificada em F0-ADR-01).

**Anti-Padrões Proibidos**
36. Sem `os.Getenv` fora de `config`: CONFORME (A1). O CI que roda o teste nunca foi executado (G8).
37. Sem escrita de arquivo da wiki fora de `wiki`: CONFORME (A2).
38. Sem "indexar depois": CONFORME (A3).
39. Sem filtro de leitura por autoria: CONFORME (A4).
40. Sem criar escopo em leitura, busca ou caminho destrutivo: PARCIAL (R17).
41. Sem concatenar caminho à mão: CONFORME por amostra (A6).
42. Sem `panic` em caminho de execução: CONFORME (A7).
43. Sem abstração preemptiva, código morto ou stubs: PARCIAL (A8).
44. Goroutine sempre com `context` e sem fan-out ilimitado: CONFORME (A9).
45. "a confirmar" tratado como decisão em ADR: PARCIAL (A10).
46. Clean-room: NÃO VERIFICÁVEL AQUI (A11).

## 6. Pendências, por severidade

**P1 (alta). INV-01: `reindex` carrega a config duas vezes.**
- Onde: `internal/cli/reindex_cmd.go:46` e `internal/cli/root.go:82` (via `Compose` chamado em `reindex_cmd.go:55`).
- Ação: separar carga de composição. `Compose` passa a receber `config.Config` já carregada (ou existe `ComposeWithConfig(ctx, cfg)`). `runReindex` faz uma única chamada a `config.Load`, usa `cfg.DataDir()` na guarda e compõe com a mesma `cfg`. Verificar `backup_cmd.go`. Reforçar `TestINV01_ConfigCarregadaUmaVez` para exigir um único site de `config.Load` em produção. Ajustar o comentário de `root.go:2`.

**P2 (alta, bloqueia o portão). `go test -race ./...` e CI nunca executados.**
- Ação: rodar `sh scripts/check.sh` num ambiente com gcc (MSYS2/MinGW-w64 ou Linux) ou obter a primeira execução verde do CI. Só então marcar G2 e G8 como CONFORME.

**P3 (média). golangci-lint não executado.**
- Ação: obter a execução no CI (v2.1.6) ou instalar o binário localmente.

**P4 (média). INV-06: sem ADR e sem teste da fronteira de tipo.**
- Ação: registrar ADR "fronteira de tipo do Sanitized". Adicionar um `TestINV06_` que prove que `Sanitized` não é criável de fora (teste de compilação negativa ou de reflexão). Alinhar `plan.md` (§2.1 e P-07) aos nomes reais.

**P5 (média). INV-10 e escritas fora do padrão atômico.**
- `wiki/ledger.go:59` (append em `log.md`), `wiki/gitrepo.go:173` (`.gitignore`), `storage/open.go:197` (cópia pré-migração), `storage/snapshot.go:62` (destino do backup) e o `syncDir` no-op do Windows. Registrar em ADR as exceções, ou tornar essas escritas atômicas.

**P6 (média). Criação de escopo por `reindex` e watcher** (`wiki/reindex.go:238`, `wiki/watcher.go:239`). Registrar em ADR (D3) ou restringir.

**P7 (baixa). Superfície pública sem chamador de produção** (`NewWatcher`; `Search`, `ResolveScope`, `Recent`, `PageChain`, `InsertObservation`, `BumpAccess`). Registrar débito/ADR com a feature que os consome, ou mover para testes.

**P8 (baixa, documental).** (a) Atualizar os checkboxes do `plan.md` §4. (b) Alinhar o `plan.md` ao código: `Writer` expõe `Do(ctx, fn)` e funções livres, não os métodos do §2.2; ADR-16 cita nomes de teste que não existem como escritos; `t.Parallel` não foi usado. (c) `tech.md` já atualizado (T075). (d) A matriz (`inv-test-matrix.md`) diz que o grep de INV-01 não foi verificado: esta auditoria o verificou (R4).

## 7. Itens que exigem decisão do usuário

Nenhuma alteração na constituição foi feita; as sugestões dependem de decisão sua.

- **D1. INV-06 "(a confirmar)":** aceitar a decisão nossa (construtores `Title`/`Body`, zero value vazio) e registrar em ADR. Sugestão de texto: "O tipo sanitizado só é criável pelas funções do pacote de sanitização; o zero value é o texto vazio." Decidir também se aceita o débito de superfície sem chamador (P7) até a 002.
- **D2. Escopo de INV-10:** restringir a regra a arquivos da wiki (páginas e manifestos), com exceção registrada para append de `log.md`, `.gitignore` inicial e arquivos de banco/snapshot; ou aceitar o `syncDir` no-op do Windows como desvio permanente; ou tornar essas escritas atômicas.
- **D3. Criação de escopo em `reindex`/watcher:** aceitar como "reconstrução explícita" (ADR) ou exigir que só `EnsureScope` por escrita explícita crie escopo. Sugestão: "Reindex e watcher podem criar workspace/projeto a partir de diretórios UUID existentes na wiki; leitura e busca nunca criam."
- **D4. Testes que leem `os.Environ`:** a constituição proíbe leitura fora de `internal/config` sem ressalvar testes. Sugestão: aceitar a exceção para testes, explicitamente.
- **D5. INV-16 handoff e ponteiro por ator:** confirmar que ficam na 002 e copiar a obrigação para o `spec.md`/`plan.md` da 002 quando for gerado, com teste multi-sessão explícito lá.
- **D6. Fechamento da 001 com `-race` e CI pendentes** (ambiente sem gcc), ou condicionar o fechamento à primeira execução verde do CI. A constituição pede `-race` como portão de PR.

## 8. Atualização de 2026-10-13 (fechamento da 001)

| Item | Antes | Agora | Evidência |
|---|---|---|---|
| G2 `go test -race ./...` | NÃO VERIFICÁVEL AQUI (sem gcc) | CONFORME | `scripts/check.sh` roda o `-race` no container `golang:<go.mod>` quando não há gcc no host (constituição, Portões); execução completa e verde em 2026-10-13. |
| G8 CI executado | NÃO VERIFICÁVEL AQUI | DISPENSADO pelo autor | 001/T090: a 001 fecha sem o CI do GitHub; os portões rodam localmente. |
| Lint | não executado | CONFORME | `golangci-lint` v2.14.0, 0 apontamentos (001/T079). |
| D1 INV-06 "a confirmar" | aberta | RESOLVIDA (decisão nossa) | Constituição, INV-06. |
| D2 escopo da INV-10 | aberta | RESOLVIDA pelo doc | Ledgers somente acréscimo (`docs/wiki-files-layer.md` §9); constituição, INV-10. |
| D3 criação de escopo no reindex e watcher | aberta | RESOLVIDA pelo doc | `docs/lifecycle-ops.md` § reindex; constituição, resolução de escopo. |
| D4 testes que leem ambiente | aberta | RESOLVIDA (decisão nossa) | Constituição, INV-01 e anti-padrões. |
| D5 handoff e ponteiro por ator | aberta | MOVIDA para a 002 | `specs/features/002-captura-e-acesso/spec.md`, "Herdado da 001". |

Divergências do código da 001 com os docs, encontradas depois desta auditoria, estão na seção "Revisão contra os docs" da spec 001 e viram as tasks T020, T022, T040, T041 e T079 a T088 da 002.
