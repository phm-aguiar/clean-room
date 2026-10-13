# Verificacao de qualidade do ai-memory (feature 001-nucleo, T067).
# Ordem: build cruzado CGO_ENABLED=0 (windows/amd64, linux/amd64) -> go vet ->
# go test -race. Para no primeiro erro. Compativel com Windows PowerShell 5.1.
# Uso: powershell -NoProfile -ExecutionPolicy Bypass -File scripts\check.ps1
# Variavel GO opcional aponta para o binario go.
$ErrorActionPreference = 'Stop'

$Root = Split-Path -Parent $PSScriptRoot
Set-Location $Root

$Go = 'go'
if ($env:GO) { $Go = $env:GO }
if (-not (Get-Command $Go -ErrorAction SilentlyContinue)) {
    Write-Error "check: binario 'go' nao encontrado no PATH. Defina a variavel GO com o caminho do binario."
    exit 1
}

# Executa go com variaveis de ambiente temporarias e restaura o ambiente ao final.
function Invoke-Go {
    param(
        [string]$CgoEnabled,
        [string]$GoOS,
        [string]$GoArch,
        [string[]]$GoArgs
    )
    $prevCgo = $env:CGO_ENABLED
    $prevOS = $env:GOOS
    $prevArch = $env:GOARCH
    try {
        $env:CGO_ENABLED = $CgoEnabled
        if ($GoOS) { $env:GOOS = $GoOS } else { Remove-Item Env:GOOS -ErrorAction SilentlyContinue }
        if ($GoArch) { $env:GOARCH = $GoArch } else { Remove-Item Env:GOARCH -ErrorAction SilentlyContinue }
        & $Go @GoArgs
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    }
    finally {
        $env:CGO_ENABLED = $prevCgo
        if ($null -eq $prevOS) { Remove-Item Env:GOOS -ErrorAction SilentlyContinue } else { $env:GOOS = $prevOS }
        if ($null -eq $prevArch) { Remove-Item Env:GOARCH -ErrorAction SilentlyContinue } else { $env:GOARCH = $prevArch }
    }
}

Write-Host '==> [1/3] build CGO_ENABLED=0 windows/amd64'
Invoke-Go -CgoEnabled '0' -GoOS 'windows' -GoArch 'amd64' -GoArgs @('build', './...')

Write-Host '==> [1/3] build CGO_ENABLED=0 linux/amd64'
Invoke-Go -CgoEnabled '0' -GoOS 'linux' -GoArch 'amd64' -GoArgs @('build', './...')

Write-Host '==> [2/3] go vet CGO_ENABLED=0 windows/amd64'
Invoke-Go -CgoEnabled '0' -GoOS 'windows' -GoArch 'amd64' -GoArgs @('vet', './...')

Write-Host '==> [2/3] go vet CGO_ENABLED=0 linux/amd64'
Invoke-Go -CgoEnabled '0' -GoOS 'linux' -GoArch 'amd64' -GoArgs @('vet', './...')

Write-Host '==> [3/3] go test -race ./...'
$hasCc = [bool](Get-Command gcc -ErrorAction SilentlyContinue) -or [bool](Get-Command cc -ErrorAction SilentlyContinue)
$hasDocker = [bool](Get-Command docker -ErrorAction SilentlyContinue)
# Versao do Go vem do go.mod (ex.: 1.26.0 -> imagem golang:1.26).
$goLine = (Select-String -Path (Join-Path $Root 'go.mod') -Pattern '^go (\d+\.\d+)').Matches[0].Groups[1].Value
$GoImage = "golang:$goLine"
if ($hasCc) {
    Invoke-Go -CgoEnabled '1' -GoOS $null -GoArch $null -GoArgs @('test', '-race', './...')
}
elseif ($hasDocker) {
    Write-Host "    gcc ausente no host; rodando -race no container $GoImage."
    $prevPathConv = $env:MSYS_NO_PATHCONV
    try {
        $env:MSYS_NO_PATHCONV = '1'
        & docker run --rm -v "${Root}:/src" -w /src $GoImage sh -c 'CGO_ENABLED=1 go test -race ./...'
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    }
    finally {
        if ($null -eq $prevPathConv) { Remove-Item Env:MSYS_NO_PATHCONV -ErrorAction SilentlyContinue } else { $env:MSYS_NO_PATHCONV = $prevPathConv }
    }
}
else {
    [Console]::Error.WriteLine('FALHA: go test -race requer cgo e um compilador C (gcc).')
    [Console]::Error.WriteLine('       Nenhum gcc/cc no PATH e docker indisponivel. Instale gcc, inicie o Docker ou rode em CI Linux.')
    [Console]::Error.WriteLine('       O passo -race NAO foi executado; o check nao esta completo.')
    exit 2
}

Write-Host 'OK: build cruzado, vet e go test -race passaram.'
