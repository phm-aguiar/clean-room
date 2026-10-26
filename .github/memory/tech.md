# Stack Tecnológica

> Revisado em 2026-10-26. O código da reimplementação foi removido; esta é a **stack planejada**, sem itens
> detectados no repositório (não há `go.mod`, scripts, CI nem linter configurados no momento). Ao recriar o
> módulo, reexecute `/sdd-explore-tech` para registrar versões fixadas e comandos reais.
> Toda escolha preserva o comportamento observável do ai-memory original (ver constituição).

## Linguagens
| Linguagem | Versão | Observação |
|---|---|---|
| Go | 1.26 | Módulo único, `CGO_ENABLED=0` no binário entregue |

## Frameworks e Bibliotecas
| Nome | Propósito |
|---|---|
| `modernc.org/sqlite` | SQLite pure Go: FTS5, WAL, backup online |
| `github.com/mark3labs/mcp-go` (candidato) | Servidor MCP (stdio e HTTP streamable) atrás da interface do pacote `mcp`; o protocolo é fixado em `2024-11-05` por wrapper, como no original |
| `github.com/spf13/cobra` | Subcomandos da CLI |
| `github.com/knadh/koanf` (providers `file`, `env`, `confmap`; parser `toml`) | Camadas de configuração de `docs/cli.md` §2.1 (sem `.env`) |
| `gopkg.in/yaml.v3` | Frontmatter YAML preservando a ordem das chaves |
| `github.com/fsnotify/fsnotify` | Watcher de arquivos da wiki |
| `github.com/google/uuid` | UUIDv7 |
| `golang.org/x/text` (`unicode/norm`) | Normalização NFC (a stdlib não tem) |
| `github.com/go-git/go-git/v5` | Repositório Git da wiki embutido, caminho normal como no original |
| `git` do sistema via `os/exec` | Só fallback no Windows, nos casos de `docs/windows-native-extras.md` §6.2 |
| `log/slog` (stdlib) | Logs estruturados com filtro do próprio módulo |
| `regexp` (stdlib, RE2) | Sanitizador onde reproduz a semântica Unicode exigida por `docs/prompts-schemas-and-sanitizer.md` §11.2 |

## Ferramentas de Build e Teste
| Ferramenta | Comando |
|---|---|
| go build (cruzado) | `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...` e `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./...` |
| go vet | `go vet ./...` |
| go test | `go test ./...`; com race: `CGO_ENABLED=1 go test -race ./...` (exige gcc no PATH ou container `golang:1.26`) |
| golangci-lint | `golangci-lint run` (versão compilada com Go ≥ 1.26) |
| gofmt | `gofmt -l .` (sem saída = formatado) |

## CI/CD
(não configurado no momento)

## Plataformas alvo
| Alvo | Observação |
|---|---|
| `windows/amd64` | Primeiro MVP |
| `linux/amd64` | Depois do Windows; macOS fora do escopo inicial |

## Atualizado em
2026-10-26 (revisão após a remoção do código e a auditoria dos docs contra o original)
