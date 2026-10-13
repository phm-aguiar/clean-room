# ai-memory-go

## O que é

`ai-memory-go` é uma reimplementação clean-room, em Go, de uma memória de longo prazo para agentes de código. O binário único `ai-memory` guarda páginas de conhecimento em **markdown versionado em git**, que é a fonte da verdade, e mantém um **índice SQLite derivado** (reconstruível a partir dos arquivos) para busca textual, de entidades e de vizinhos de link.

O público é o autor (dev solo, Windows e Linux). A especificação completa da fase está em `specs/features/001-nucleo/spec.md`.

### Escopo desta fase (feature 001, núcleo)

Esta fase entrega o alicerce:

- domínio tipado, sem I/O (`internal/domain`);
- configuração carregada uma vez (`internal/config`);
- armazenamento SQLite pure-Go com WAL e FTS5, escritor único e migrações (`internal/storage`);
- wiki em markdown versionada em git, com escrita atômica, supersessão, watcher e reconciliação (`internal/wiki`);
- busca híbrida (FTS5, entidades e vizinhos de link, fundidos por RRF);
- `reindex` a partir do markdown e `backup` online do SQLite.

**Fora desta fase:** hooks de captura, servidor MCP e CLI de usuário mais ampla (feature 002); LLM, embeddings, reranker e consolidação (fase seguinte, F003). Esta fase é **zero-LLM**: o sistema funciona completo sem provedor.

## Pré-requisitos

- **Go**, na versão indicada em `go.mod` (hoje `go 1.26.0`).
- **git** no PATH. A wiki usa o `git` do sistema para os commits; sem ele, a abertura da wiki falha com erro explícito.
- **Docker** (ou gcc/cc no PATH), apenas para `go test -race`, porque o detector de race exige cgo. Com gcc no host, os scripts usam o gcc local. Sem ele, usam o container oficial `golang:<versão de go.mod>`, que já traz gcc:
  - **Windows e Linux:** instale o Docker e deixe o daemon rodando. Não é preciso instalar nada mais.
  - **Alternativa:** gcc no PATH (no Windows, MSYS2 com `mingw-w64-ucrt-x86_64-gcc` e `C:\msys64\ucrt64\bin` no PATH).

O **build do binário não precisa de gcc**: ele é feito com `CGO_ENABLED=0`.

## Build

Linux:

```sh
CGO_ENABLED=0 go build -o ai-memory ./cmd/ai-memory
```

Windows (PowerShell):

```powershell
$env:CGO_ENABLED = "0"
go build -o ai-memory.exe ./cmd/ai-memory
```

Em Git Bash no Windows, use a mesma linha do Linux com `-o ai-memory.exe`.

## Configuração

A configuração é resolvida nesta ordem, da menor para a maior precedência:

1. **padrões** embutidos;
2. **arquivo TOML** (`--config`);
3. **arquivo `.env`** local (`--env-file`, ou `AI_MEMORY_ENV_FILE`, ou `./.env` no diretório de trabalho);
4. **variáveis de ambiente do processo** `AI_MEMORY_*`.

O separador `__` aninha chaves: `AI_MEMORY_LOG__LEVEL` define `log.level`.

### Arquivo `.env`

O `.env` fica na raiz do repositório quando você executa o binário a partir dela, **não é versionado** (está no `.gitignore`) e não deve conter segredos. Copie `.env.example` para `.env` e ajuste. As chaves desta fase são:

| Chave | Padrão | Significado |
|---|---|---|
| `AI_MEMORY_DATA_DIR` | `data` | Diretório de dados (`wiki/` e `db/`). Aceita caminho relativo ou absoluto; o caminho final canônico é registrado no log. |
| `AI_MEMORY_LOG__LEVEL` | `info` | Nível de log: `debug`, `info`, `warn` ou `error`. |
| `AI_MEMORY_WIKI__WATCH_DEBOUNCE_MS` | `300` | Debounce do watcher da wiki, em milissegundos (inteiro positivo). |
| `AI_MEMORY_WIKI__RECONCILE_INTERVAL_S` | `30` | Intervalo da reconciliação da wiki, em segundos (inteiro positivo). |

`AI_MEMORY_ENV_FILE` indica outro caminho para o `.env`; é lida do ambiente do processo.

### Arquivo TOML

Passado com `--config`. As mesmas chaves, em formato TOML:

```toml
data_dir = "data"

[log]
level = "info"

[wiki]
watch_debounce_ms = 300
reconcile_interval_s = 30
```

### Flags globais

- `--config <arquivo.toml>`: caminho do arquivo TOML. Ausente ou inexistente significa "sem essa camada".
- `--env-file <arquivo>`: caminho do `.env`. Sem a flag, usa `AI_MEMORY_ENV_FILE` ou `./.env`.

## Comandos

### `ai-memory reindex`

Reconstrói o índice SQLite a partir do markdown de `<data_dir>/wiki`. Antes de agir, recusa se houver um servidor ai-memory vivo usando o data_dir, verificado pelo arquivo `ai-memory.pid` dentro do data_dir.

Saída: uma linha `reindex concluído`, seguida de contadores (`escopos`, `páginas`, `inalteradas`, `erros`, `avisos`) e de uma linha `erro:` ou `aviso:` para cada problema por arquivo. Problemas por arquivo não fazem o comando falhar.

### `ai-memory backup --to <arquivo>`

Grava um snapshot online do banco SQLite com o escritor ativo, usando a API de backup do SQLite. O arquivo de destino **não pode existir** e o diretório de destino precisa existir.

Saída: uma linha `backup gravado em <arquivo>`.

## Testes e verificação

Testes unitários e de contrato:

```sh
go test ./...
```

Verificação completa (build cruzado com `CGO_ENABLED=0` para `windows/amd64` e `linux/amd64`, `go vet`, e `go test -race`):

```sh
sh scripts/check.sh
```

No Windows, pelo PowerShell:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\check.ps1
```

O passo `-race` usa gcc do host se existir; senão, roda no container Docker `golang:<versão de go.mod>`. Se nem gcc nem Docker estiverem disponíveis, os scripts param com código de saída 2 e a mensagem `FALHA: go test -race requer cgo e um compilador C (gcc)`; a verificação não fica completa nesse caso. O `go test -race ./...` direto, sem os scripts, exige gcc no host ou pode ser rodado no container:

```sh
MSYS_NO_PATHCONV=1 docker run --rm -v "$PWD:/src" -w /src golang:1.26 sh -c "CGO_ENABLED=1 go test -race ./..."
```

(`MSYS_NO_PATHCONV=1` evita que o Git Bash no Windows reescreva o caminho `/src`; em Linux pode ser omitido.)

O CI (`.github/workflows/ci.yml`) roda `scripts/check.sh` em `windows-latest` e `ubuntu-latest`, instalando o gcc via MSYS2 no Windows, e depois o golangci-lint (config em `.golangci.yml`).

## Estrutura do repositório

- `cmd/ai-memory/`: ponto de entrada do binário (só delega à CLI).
- `internal/config/`: carga da configuração (único pacote que lê ambiente e arquivos de config).
- `internal/domain/`: tipos de domínio sem I/O, incluindo `sanitize/` (sanitização de segredos).
- `internal/storage/`: SQLite (`modernc.org/sqlite`), migrações em `migrations/`, escritor único, leitura e busca.
- `internal/wiki/`: wiki em markdown, escrita atômica, repositório git, watcher, reindex.
- `internal/cli/`: raiz da CLI (cobra), `reindex`, `backup` e guarda de processo vivo.
- `internal/archtest/`: teste do grafo de imports da constituição.
- `test/`: testes de aceitação (`acceptance/`), de contrato (`contract/`), de fumaça (`smoke/`) e benchmark (`bench/`).
- `scripts/`: `check.sh` e `check.ps1` (verificação completa).
- `specs/`: especificações e planos das features (SDD).
- `docs/`: documentação de referência do projeto.

## Limitações conhecidas desta fase

- `go test -race` foi executado no container `golang:1.26` (go1.26.9, gcc 14.2): nenhum `DATA RACE` reportado, mas `test/bench` falha sob `-race` (`TestSearch5kAbaixoDe100ms`, média ~909 ms contra alvo < 100 ms; sem `-race` passa em ~10 s). O CI ainda não foi executado.
- O golangci-lint não foi executado localmente (roda no CI).
- O `reindex` ainda não verifica se o banco já está populado antes de reconstruí-lo.
- O servidor que grava `ai-memory.pid` ainda não existe nesta fase; a guarda de processo vivo já está implementada, mas só age se o arquivo existir.
