# plan.md: Plano de Implementação Técnica — Implantação (015)

> Plano gerado em 2026-10-13, antes de a 002 ser executada. Revisar quando a 014 terminar.

## 1. Metadados do Plano
- **Stack Tecnológico:** a da 002, sem dependência nova no binário. Artefatos fora do Go: Dockerfile, composições, unidades systemd, wrappers em sh, PowerShell e cmd, scripts de rotina.
- **Feature Fonte:** `specs/features/015-implantacao/spec.md`
- **Escopo:** imagem, composições, releases, `upgrade`, wrappers, unidades de serviço, prefixo do servidor, IP do cliente, keepalive, Windows nativo, rotinas externas.
- **Fontes de contrato:** as listadas na spec.

## 2. Design de Contratos e Fronteiras
- **Configuração nova:** `release_base_url` (`AI_MEMORY_RELEASE_BASE_URL`), `base_path` (`AI_MEMORY_BASE_PATH`), `[auth].trusted_proxy_cidrs`, `tcp_keepalive_secs`.
- **Releases:** `<base>/download/<tag>/<arquivo>` e `<...>.sha256`; última tag por API do GitHub ou `<base>/latest/tag`.
- **Layout no repositório:** `deploy/` (Dockerfile, composições, unidades, sysusers, tmpfiles, exemplo WinSW), `contrib/` (wrappers e rotinas), `.github/workflows/release.yml`.

## 3. Decisões Arquiteturais e Justificativas

- **Decisão (D-01): `upgrade` com verificação antes de tocar no disco.** Baixa o arquivo para um temporário, compara o SHA-256 com o `.sha256`, só então extrai e troca; no Windows renomeia o executável em execução antes de mover o novo; depois reexecuta `install-hooks --apply` para cada agente com hooks instalados.
  - **Justificativa:** `docs/airgapped-install.md` § Updates e `docs/windows.md` § Upgrade.
  - **Alternativa Rejeitada:** extrair e conferir depois (deixa binário corrompido no lugar).

- **Decisão (D-02): prefixo do servidor com o normalizador da 014.** O mesmo normalizador trata `base_path` e `web_slug`; o roteador monta tudo sob o prefixo; valor inválido monta na raiz com log; o cliente HTTP usa o caminho da URL configurada como base.
  - **Justificativa:** `docs/https-via-proxy.md` § Subcaminho.
  - **Alternativa Rejeitada:** confiar no proxy para remover o prefixo (o doc proíbe).

- **Decisão (D-03): IP do cliente como função pura.** Entrada: endereço da conexão, cabeçalhos `X-Forwarded-For` na ordem, CIDRs confiáveis; saída: IP para o limitador de login da 007. Nenhum outro uso.
  - **Justificativa:** § `X-Forwarded-For`; o servidor não interpreta `X-Forwarded-Proto` nem host.
  - **Alternativa Rejeitada:** middleware que reescreve o `RemoteAddr` (vaza para logs e auditoria).

- **Decisão (D-04): artefatos fora do binário testados por smoke.** Dockerfile e composições validados por `docker build` e `docker compose config` no script de checagem (quando há Docker); wrappers com teste de shell e Pester leve; unidades com `systemd-analyze verify` quando disponível.
  - **Justificativa:** `docs/hook-scripts-and-container-deploy.md` §6.
  - **Alternativa Rejeitada:** artefatos sem teste.

- **Decisão (D-05): sem chamada de rede por padrão, provado em teste.** Um teste sobe o servidor com a configuração padrão, com um discador que registra destinos, e confirma que nenhum destino além do loopback é usado.
  - **Justificativa:** `docs/airgapped-install.md` § Runtime.
  - **Alternativa Rejeitada:** só documentar.

### O que não construir nesta feature
macOS, AUR, RPM, Nix, assinatura de código, modelo de embeddings local.

## 4. Auditoria de Constituição
- [ ] Build cruzado (linux amd64 e arm64, windows amd64) — releases.
- [ ] `go vet`, `go test -race`, golangci-lint.
- [ ] INV-01 — chaves novas só em `internal/config`.
- [ ] INV-11 — diretório de dados padrão e `/data` na imagem.
- [ ] Segurança — checksum antes da troca; segredos fora do repositório; nada de telemetria.
- [ ] Sem `panic`; erros de `upgrade` com contexto e sem deixar binário parcial.
