# Baseline: BenchmarkSearch5k (T066)

Meta da SPEC (Constraints de Performance): consulta com 5 mil páginas < 100 ms.
Fonte do benchmark: `test/bench/search_bench_test.go`.

## Resultado

**Meta atendida nesta medição.** A média ficou em cerca de 33 ms (benchmark) e 37 ms (teste de alvo), abaixo de 100 ms. O p95 mais alto observado foi 66 ms (consulta "gotcha fila escritor", 10 amostras).

| Medição | Amostras | p50 | p95 | Média |
|---|---|---|---|---|
| `BenchmarkSearch5k` (`-benchtime=40x`), todas as consultas | 40 | 33,1 ms | 55,9 ms | 33,0 ms |
| `TestSearch5kAbaixoDe100ms`, consulta "postgres replicação" | 30 (após 1 de aquecimento) | 36,9 ms | 55,7 ms | 37,3 ms |

Por consulta (`-benchtime=40x`, 10 amostras cada):

| Consulta | p50 | p95 | Média |
|---|---|---|---|
| "postgres replicação" | 35,4 ms | 42,3 ms | 35,7 ms |
| "timeout cache" | 31,2 ms | 38,2 ms | 30,8 ms |
| "sessão decisão" | 16,7 ms | 22,0 ms | 16,2 ms |
| "gotcha fila escritor" | 46,7 ms | 66,1 ms | 49,2 ms |

Amostras pequenas (10 por consulta, 30 no teste): os percentis são indicativos, não estatística robusta.

## Ambiente

- Data da medição: 2026-10-12
- SO: Windows 11 Home (10.0.26200)
- GOOS/GOARCH: windows/amd64
- CPU: Intel Core Ultra 7 255HX, 20 CPUs lógicas (`runtime.NumCPU()` = 20)
- Go: go1.26.5
- `CGO_ENABLED=0`
- Banco: SQLite (`modernc.org/sqlite`) em `t.TempDir()`, em WAL, via `storage.Open` e `storage.OpenReader`
- Máquina de desenvolvimento com outras cargas ativas (não isolada); não houve controle de frequência de CPU

## Carga do corpus

- 5000 páginas, criadas no escopo com `storage.UpsertPageTx` dentro de `Writer.Do`, em lotes de 500 (10 transações no escritor único). Todas no caso C (`UpsertCreated`), o que foi checado.
- Conteúdo determinístico: `math/rand/v2` com semente fixa (`NewPCG(42, 7)`). Corpo com 24 termos de um vocabulário de 42 palavras, duas entidades por página (12 nomes possíveis), tiers em rodízio, `pinned` em 1 de cada 97 páginas.
- Corte da busca: instante da escrita + 1 hora. `limit = 10`.
- Custo da carga: estimativa de cerca de 14 s para as 5000 páginas (diferença entre a duração do teste de alvo, 15,5 s, e o tempo das 31 buscas). Não foi medido separadamente.

## Comandos usados

```
CGO_ENABLED=0 "/c/Program Files/Go/bin/go" test ./test/bench/ -run Search5k -count=1 -v
CGO_ENABLED=0 "/c/Program Files/Go/bin/go" test ./test/bench/ -bench Search5k -benchtime=3x -run xxx -v
CGO_ENABLED=0 "/c/Program Files/Go/bin/go" test ./test/bench/ -bench Search5k -benchtime=40x -run xxx -count=1 -v
```

Verificações: `go vet ./test/bench/` com `GOOS=windows` e `GOOS=linux`, e `gofmt -l test/bench/` (sem saída).

## Observações

- O benchmark mede `storage.Search` com cronômetro externo por chamada (`time.Now` antes e depois). A carga fica fora do timer (`StopTimer`/`StartTimer`).
- O setup roda uma vez por invocação do benchmark. Com `-benchtime=Nx` ele roda duas vezes (N=1 e N=Nx), então a carga aparece duas vezes na saída.
- O teste de alvo falha se a média passar de 100 ms. Nesta medição não falhou. O p50/p95 vão para o log, para que uma máquina mais lenta mostre o número e não só a falha.
- Não foi medido o item "leitura nunca bloqueia no escritor" da SPEC. Este baseline mede só a latência de leitura, sem escrita concorrente.
