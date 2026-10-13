# Stack Tecnológica

> Gerado por `/sdd-explore-tech` em 2026-10-12 e revisado após a feature 001-nucleo (2026-10-12).
> Reexecute para atualizar. Versões e comandos vêm de `go.mod`, `.golangci.yml`, `.github/workflows/ci.yml`,
> `scripts/check.sh`, `scripts/check.ps1` e `internal/config/config.go`.
> Entradas marcadas **(em uso)** são importadas pelo código. Entradas **(detectado)** estão no `go.mod` ou
> foram confirmadas nas ferramentas da máquina. A única entrada ainda não implementada é `github.com/mark3labs/mcp-go`,
> que não está no `go.mod` e não é usada nesta feature.

## Linguagens
| Linguagem | Versão | Detecção |
|---|---|---|
| Go | 1.26.0 (linha em `go.mod`) | (detectado) `go.mod`; toolchain local 1.26.5 (detectado) via `go version` |

## Frameworks e Bibliotecas
| Nome | Versão | Propósito |
|---|---|---|
| `modernc.org/sqlite` | v1.60.1 | (em uso, ADR-01) SQLite pure Go, FTS5, WAL, backup online |
| `github.com/mark3labs/mcp-go` | — | (planejado, ADR-05; não usado nesta feature) Servidor MCP stdio + HTTP streamable, atrás de interface própria |
| `github.com/spf13/cobra` | v1.10.2 | (em uso, ADR-06) Subcomandos da CLI |
| `github.com/knadh/koanf` | v1.5.0 | (em uso, ADR-06) Config TOML + env `AI_MEMORY_*` com separador `__` |
| `github.com/knadh/koanf` providers/parsers (`providers/file`, `providers/env`, `providers/confmap`, `parsers/toml`, `parsers/dotenv`) | v1.5.0 (mesmo módulo, conforme `go.mod`) | (em uso, ADR-06 e P-11 da 001) Camadas: padrões (`confmap`) < TOML < `.env` local não versionado < env do processo |
| `gopkg.in/yaml.v3` | v3.0.1 | (em uso, plano 001) Leitura e emissão de frontmatter YAML preservando a ordem das chaves |
| `github.com/fsnotify/fsnotify` | v1.10.1 | (em uso, plano 001 P-08) Watcher de arquivos da wiki, com debounce próprio de 300 ms |
| `github.com/google/uuid` | v1.6.0 | (em uso, ADR-08) UUIDv7 |
| `golang.org/x/text` (`unicode/norm`) | v0.43.0 | (em uso, F0-ADR-01 da 001) Normalização NFC da `PortableKey` (a stdlib não tem NFC) |
| `log/slog` (stdlib) | — (stdlib do Go 1.26) | (em uso, ADR-08) Logs estruturados com filtro do próprio módulo |
| `regexp` (stdlib, RE2) | — (stdlib do Go 1.26) | (em uso, ADR-07) Sanitizador sem retrocesso |
| `git` do sistema via `os/exec` | 2.56.0 (local) | (em uso, ADR-03; versão local detectada via `git --version`) Commits da wiki (`internal/wiki/gitrepo.go`) |

Dependências indiretas listadas no `go.mod` (sem import direto): `github.com/spf13/pflag` v1.0.9, `github.com/pelletier/go-toml` v1.7.0, `github.com/joho/godotenv` v1.3.0, `github.com/mitchellh/mapstructure` v1.5.0, `golang.org/x/sys` v0.48.0, `modernc.org/libc` v1.77.1, entre outras.

## Ferramentas de Build e Teste
| Ferramenta | Comando | Arquivo de Config |
|---|---|---|
| Verificação completa (Git Bash ou Linux) | `sh scripts/check.sh` | `scripts/check.sh` |
| Verificação completa (Windows PowerShell 5.1) | `powershell -NoProfile -ExecutionPolicy Bypass -File scripts\check.ps1` | `scripts/check.ps1` |
| go build (cruzado) | `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...` e `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./...` | `go.mod` |
| go vet | `go vet ./...` (executado também sob os dois alvos cruzados pelo `check`) | — |
| go test | `go test ./...`; com race: `CGO_ENABLED=1 go test -race ./...` (exige gcc/cc no PATH, ou Docker com a imagem `golang:<major.minor de go.mod>`, usada automaticamente pelos scripts) | — |
| Docker (opcional) | Container `golang:1.26` para o `-race` quando não há gcc no host | — |
| gofmt | `gofmt -l .` (sem saída = formatado) | `.golangci.yml` (formatter `gofmt`) |

Observação: `scripts/check.sh` e `scripts/check.ps1` param no primeiro erro. O passo `-race` usa gcc do host; se ausente e o Docker estiver disponível, roda no container `golang:<major.minor de go.mod>`. Sem gcc e sem Docker, falha com código 2 e o check não fica completo. Nesta máquina (Windows), gcc não está no PATH; o Docker foi verificado (`golang:1.26`, gcc 14.2, go1.26.9). Não verificado no CI.

## CI/CD
| Plataforma | Pipeline | Gatilhos |
|---|---|---|
| GitHub Actions | `.github/workflows/ci.yml` (job `check`, matriz `windows-latest` e `ubuntu-latest`, `fail-fast: false`) | `push` e `pull_request` para a branch `main` |

Passos do job: `actions/checkout@v4`; `actions/setup-go@v5` com `go-version-file: go.mod` e `cache: true`; no Windows, `msys2/setup-msys2@v2` instalando `mingw-w64-ucrt-x86_64-gcc` (UCRT64) e adicionando `C:\msys64\ucrt64\bin` ao PATH para o `-race`; `sh scripts/check.sh`; e `golangci/golangci-lint-action@v8` com `version: v2.14.0`. Permissões: `contents: read`.

Limitação registrada no próprio workflow: o `ci.yml` não foi executado no GitHub Actions a partir do ambiente de desenvolvimento (validação apenas sintática). Execução real no CI: (não verificado).

## Linting e Formatação
| Ferramenta | Config |
|---|---|
| golangci-lint v2.14.0 (formato v2; v2.1.6 foi compilado com Go 1.24 e recusa `go 1.26.0`, via `golangci/golangci-lint-action@v8` no CI) | `.golangci.yml` |
| Linters habilitados | `errcheck`, `govet`, `staticcheck` (com `linters.default: none`) |
| Formatter | `gofmt` (seção `formatters` do `.golangci.yml`) |

golangci-lint e staticcheck não estão instalados localmente nesta máquina (não verificado localmente; o lint roda no CI).

## Infraestrutura
| Ferramenta | Arquivos |
|---|---|
| Verificação de qualidade (build cruzado, vet, race) | `scripts/check.sh`, `scripts/check.ps1` |
| CI (GitHub Actions) | `.github/workflows/ci.yml` |
| Linter | `.golangci.yml` |
| Dependências Go | `go.mod`, `go.sum` |
| Configuração da aplicação | `internal/config/config.go` (precedência: padrões < TOML < `.env` < `AI_MEMORY_*`) |

## Plataformas alvo
| Alvo | Observação |
|---|---|
| `windows/amd64`, `linux/amd64` | (detectado em `scripts/check.sh`, `scripts/check.ps1` e `ci.yml`) Binário único `ai-memory`, `CGO_ENABLED=0` no build; macOS fora do escopo inicial |

## Atualizado em
2026-10-12
