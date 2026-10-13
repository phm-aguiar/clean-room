#!/bin/sh
# Verificacao de qualidade do ai-memory (feature 001-nucleo, T067).
# Ordem: build cruzado CGO_ENABLED=0 (windows/amd64, linux/amd64) -> go vet ->
# go test -race. Para no primeiro erro (set -e).
# Roda em Git Bash (Windows) e em Linux. Uso: sh scripts/check.sh
# Variavel GO opcional aponta para o binario go (ex.: GO=/c/Program\ Files/Go/bin/go).
set -e

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

GO="${GO:-go}"
if ! command -v "$GO" >/dev/null 2>&1; then
  echo "check: binario 'go' nao encontrado no PATH. Defina GO=/caminho/para/go." >&2
  exit 1
fi

echo "==> [1/3] build CGO_ENABLED=0 windows/amd64"
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 "$GO" build ./...

echo "==> [1/3] build CGO_ENABLED=0 linux/amd64"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 "$GO" build ./...

echo "==> [2/3] go vet CGO_ENABLED=0 windows/amd64"
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 "$GO" vet ./...

echo "==> [2/3] go vet CGO_ENABLED=0 linux/amd64"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 "$GO" vet ./...

echo "==> [3/3] go test -race ./..."
# Versao do Go vem do go.mod (ex.: 1.26.0 -> imagem golang:1.26).
GO_IMAGE="golang:$(sed -n 's/^go \([0-9]*\.[0-9]*\).*/\1/p' go.mod)"
if command -v gcc >/dev/null 2>&1 || command -v cc >/dev/null 2>&1; then
  CGO_ENABLED=1 "$GO" test -race ./...
elif command -v docker >/dev/null 2>&1; then
  echo "    gcc ausente no host; rodando -race no container $GO_IMAGE."
  MSYS_NO_PATHCONV=1 docker run --rm -v "$ROOT:/src" -w /src "$GO_IMAGE" \
    sh -c "CGO_ENABLED=1 go test -race ./..."
else
  echo "FALHA: go test -race requer cgo e um compilador C (gcc)." >&2
  echo "       Nenhum gcc/cc no PATH e docker indisponivel. Instale gcc, inicie o Docker ou rode em CI Linux." >&2
  echo "       O passo -race NAO foi executado; o check nao esta completo." >&2
  exit 2
fi

echo "OK: build cruzado, vet e go test -race passaram."
