# Spec: Implantação (contêiner, releases, `upgrade`, proxy HTTPS, Windows nativo)

## Metadados
- **ID:** 015
- **Status:** draft (revisada contra os docs em 2026-10-13; nova revisão prevista quando a 014 terminar)
- **Autor:** phm-aguiar
- **Data:** 2026-10-13
- **Feature Anterior:** 014 (normalizador de prefixo, interface web), 007 (autenticação, limite de login), 002 (servidor, hooks, backup pelo servidor)
- **Roadmap:** `specs/features/roadmap.md`
- **Aprovado:** false

## Propósito
> Para quem, o quê e por quê.

Operadores querem rodar o servidor fora da própria máquina com segurança: em contêiner num homelab, atrás de um proxy com HTTPS, sem rede (air-gapped) ou como serviço no Windows. Esta feature entrega os artefatos de distribuição (imagem, releases com checksum, unidades de serviço, wrappers), o comando `upgrade`, o prefixo de URL do servidor inteiro, o IP do cliente atrás de proxy confiável e os ajustes de Windows nativo que ainda faltam.

## Escopo

### Dentro do escopo
- **Imagem de contêiner** (`docs/hook-scripts-and-container-deploy.md` §4.1): build em estágio separado (toolchain Go), runtime Debian bookworm-slim só com certificados de autoridade, usuário `ai-memory` uid/gid 1000 com `/data`, variáveis `AI_MEMORY_DATA_DIR=/data`, `AI_MEMORY_ALLOWED_HOSTS=localhost,127.0.0.1,::1,host.docker.internal`, `AI_MEMORY_IN_CONTAINER=1`, porta 49374, healthcheck pelo `status` (30 s, timeout 5 s, início 5 s, 3 tentativas), entrypoint o binário com `serve --transport http --bind 0.0.0.0:49374 --enable-web`, scripts de hook em `/usr/local/share/ai-memory/hooks`; alvos de runtime pré-compilados amd64 e arm64.
- **Composições de referência** (§4.2): desenvolvimento local (porta só em loopback), homelab na LAN, TLS com Caddy (três variantes), túnel de saída, harness de múltiplos operadores com proxy de identidade. Arquivo de ambiente sem aspas (§4.3).
- **Releases** (`docs/airgapped-install.md`): arquivos `ai-memory-linux-x86_64.tar.gz`, `ai-memory-linux-aarch64.tar.gz`, `ai-memory-windows-x86_64.zip`, cada um com `.sha256` no formato da ferramenta padrão.
- **Comando `upgrade` nativo** (`docs/airgapped-install.md` § Updates, `docs/windows.md` § Upgrade): host configurável (`release_base_url` / `AI_MEMORY_RELEASE_BASE_URL`), arquivo em `<base>/download/<tag>/<arquivo>` e checksum no mesmo caminho com `.sha256`, checksum verificado antes de extrair ou trocar o binário, versão fixável, última tag pela API do GitHub no host padrão ou por `latest/tag` em host próprio; troca do executável em execução (no Windows, renomeia o atual para o lado e remove a cópia antiga por melhor esforço); reexecuta `install-hooks --apply` para cada agente instalado; só nas plataformas com release.
- **Wrappers** (`docs/hook-scripts-and-container-deploy.md` §5.4, `docs/windows-native-extras.md` §2): POSIX, PowerShell e cmd; montagens, variáveis repassadas, URL do servidor por subcomando, cliente nativo baixado e verificado por checksum para `run`, `show`, `continue`, `resume`, `workstreams`, `rename-workstream`, `mcp-bridge` e `install-hooks`; `upgrade` do wrapper (POSIX: seis passos; Windows: só o pull da imagem); checagem de versão no máximo uma vez por dia.
- **Unidades de serviço Linux** (§5.1): unidade de sistema endurecida e unidade de usuário, sysusers e tmpfiles.
- **Prefixo do servidor inteiro** (`docs/https-via-proxy.md` § Subcaminho): `AI_MEMORY_BASE_PATH` com o normalizador da 014; todas as rotas sob o prefixo; clientes usam o caminho da URL configurada como prefixo base.
- **IP do cliente atrás de proxy** (§ `X-Forwarded-For`): honrado só em conexões de `[auth].trusted_proxy_cidrs`, usado só no limite de tentativas de login e recuperação da 007.
- **Keepalive TCP** por conexão (`tcp_keepalive_secs`, 0 desliga; `docs/cli.md` §3.5).
- **Windows nativo** (`docs/windows-native-extras.md`, `docs/windows.md`): erro de dono do repositório da wiki registrado em nível de erro na inicialização com a instrução de rodar como o dono; exemplo de serviço com WinSW e guia da conta de serviço.
- **Rotinas externas** (`docs/backup.md`, `docs/deploy.md`): script de snapshot com espelho e push (exclusões do doc), unidades de timer, exemplo de compactação condicional (nunca `VACUUM` incondicional).
- **Sem chamada de rede por padrão** (`docs/airgapped-install.md` § Runtime): teste que prova que, com a configuração padrão, só há conexão ao servidor configurado.

### Fora do escopo (explicitamente)
- macOS (releases, launchd, wrapper de macOS): fora do escopo inicial (`.github/memory/tech.md`). Os docs publicam arquivos de macOS; aqui não.
- Pacotes AUR, RPM e Nix (os testes de empacotamento Nix do doc ficam de fora junto).
- Assinatura de código (o próprio doc a deixa para decisão do mantenedor).
- Download do modelo de embeddings local (feature 017).
- Migração de formato 2.0 (não se aplica: este produto nasce no OKF 0.2).

## Comportamento Esperado

### Cenário Principal (Happy Path)
1. O operador sobe a composição de homelab com `AI_MEMORY_AUTH_TOKEN` e a lista de hosts com o nome público; o healthcheck fica saudável; o proxy termina o TLS.
2. Numa máquina sem rede, o operador confere o `.sha256`, extrai o arquivo, roda o servidor; nenhum tráfego sai do host.
3. Mais tarde, `ai-memory upgrade` baixa a versão nova do espelho interno, confere o checksum, troca o binário e reinstala os hooks.

### Cenários Alternativos
- **Checksum diferente no `upgrade`:** recusa antes de extrair; binário intacto.
- **`AI_MEMORY_BASE_PATH` com segmento inválido:** monta na raiz e registra o motivo.
- **`X-Forwarded-For` de conexão não confiável ou com IP inválido:** vale o IP da conexão.
- **Serviço Windows com outra conta que não a dona do diretório:** servidor sobe, commits da wiki falham, erro registrado na inicialização com a orientação.

## Correções da auditoria (2026-10-20)
Pontos em que a spec ou os docs divergiam do ai-memory de referência (docs corrigidos; a spec passa a exigir):
- **Entrada de registro de logon e `service-host` (faltava inteiro):** no Windows o servidor em segundo plano do produto de referência não é serviço: é o valor `ai-memory` em `HKCU\...\CurrentVersion\Run` que inicia um supervisor oculto (reinício após 5 s, desiste após 5 falhas em menos de 10 s, parada cooperativa por arquivo, estado em `service-host.json`). Entra no escopo como alternativa sem administrador ao exemplo WinSW; `docs/windows-native-extras.md` §14. O comando `setup` que o instala é maior que esta feature: ver pendências no relatório 003.
- **`upgrade` completo:** recusas (contêiner, prefixos de pacote, diretório não gravável), limites (120 s por requisição, 128 MiB por download, 4 096 entradas, 512 MiB expandidos), formato do `.sha256`, lista de permissão de caminhos do arquivo, nomes `.new`/`.old` (extensão trocada), `--force`, "already up to date", reaplicação de hooks só dos agentes preparados, caso especial do Claude Code, aviso de servidor remoto, mensagens literais: `docs/airgapped-install.md`.
- **Healthcheck é cliente HTTP:** `status` chama `/admin/status` em `127.0.0.1` com o prefixo de base; `AI_MEMORY_ALLOWED_HOSTS` deve manter `127.0.0.1`/`localhost` e o usuário uid 1000 deve ler `/data`.
- **Redirecionamento da barra final é 308**; cliente de linha de comando: o caminho da URL do servidor vence `AI_MEMORY_BASE_PATH`, ambos normalizados (inválido vira raiz); URL padrão derivada do `bind` (`0.0.0.0` → `127.0.0.1`).
- **`X-Forwarded-For`:** itens separados por vírgula e aparados, qualquer item inválido (inclusive vazio) invalida a lista, sem par da conexão = `0.0.0.0`; `trusted_proxy_cidrs` aceita endereço puro (`/32`,`/128`), entrada inválida impede a subida; limite de 10 falhas por 60 s e 4 096 IPs.
- **Keepalive:** o mesmo valor serve de tempo ocioso e de intervalo; falha de ajuste é só aviso.
- **Erro de dono da wiki:** registrado em nível de erro no passo do checkpoint-base da inicialização (texto no `docs/windows.md`); o fallback de commit exclui os ledgers de hook.
- **Resolvidos "a confirmar":** `raw/` guarda segmentos imutáveis de workstreams gerenciados (entra no backup se houver workstreams); `config.toml` pode conter segredos de `[auth]`; espaço necessário para `compact` ≈ tamanho do banco.
- **Conflito spec × doc:** o critério "nenhuma conexão de saída por padrão" colide com o doc (`docs/airgapped-install.md`), que registra uma chamada ligada por padrão: o download em segundo plano do modelo de embeddings locais na primeira subida com provedor não definido. O teste vale para o MVP sem embeddings (feature 017 fora); ao implementar a 017 o teste deve fixar `embedding_provider = "none"` ou excluir esse download, e o exemplo do Happy Path 2 deve preparar os arquivos do modelo.
- **Não verificáveis (ausentes do material de referência):** wrappers POSIX/PowerShell/cmd, script de deploy, unidades systemd/launchd/sysusers/tmpfiles, script e unidades de backup, composições Nix/AUR. O que os docs dizem deles ficou como estava, sem confirmação.

## Edge Cases
- `upgrade` com `--version` vazio: erro; com tag igual à instalada: "already up to date" sem baixar; sem `.sha256` utilizável: recusa antes de extrair.
- Valor `ai-memory` do registro de terceiros: nem `setup` nem `uninstall` o tocam.
- `X-Forwarded-For` com todos os IPs confiáveis: usa o primeiro da lista.
- Wrapper sem o comando do Docker: código 127 citando `AI_MEMORY_DOCKER`.
- `upgrade` do wrapper sem checksum publicado: recusa a autoatualização.
- Prefixo `//wiki//` vira `/wiki`; `..` derruba para a raiz.

## Constraints
- **Segurança:** checksum antes de qualquer troca de binário; arquivo de ambiente sem segredo commitado; nenhuma telemetria.
- **Tecnologia:** sem dependência nova no binário (HTTP, `archive/tar`, `archive/zip`, `compress/gzip`, `crypto/sha256` da stdlib).

## Critérios de Sucesso
- [ ] Imagem constrói e o healthcheck passa num smoke local
- [ ] `upgrade` testado contra um servidor de releases falso (checksum certo, errado, ausente, versão fixada)
- [ ] Normalizador de prefixo e `X-Forwarded-For` cobertos por teste de tabela
- [ ] Teste de "nenhuma conexão de saída por padrão"
- [ ] `scripts/check.sh` e o lint passam sem apontamentos

## Abordagem Escolhida
Artefatos de distribuição em `deploy/` e `contrib/` (fora do binário); `upgrade` e keepalive no `cli`; prefixo do servidor reaproveitando o normalizador da 014; IP do cliente como função pura chamada pelo limitador de login da 007.

## Decisões tomadas
- **Imagem com Go:** o doc descreve a receita da implementação de referência (toolchain própria); aqui o estágio de build usa a imagem oficial do Go na versão do `go.mod`, e o binário é estático (`CGO_ENABLED=0`). Runtime mantém o Debian bookworm-slim do doc.
- **Fallback de Git no Windows (`docs/windows-native-extras.md` §6.2) não se aplica:** este produto já usa o programa `git` (não há biblioteca de Git embutida). Fica só o tratamento do erro de dono.
- **Troca atômica com repetição no Windows (§6.1):** já existe na 001 (`internal/wiki/atomicfile.go`); a T001 só reconfere os números (5 tentativas, 10 a 50 ms).
- **Regras de caminho (§8.2):** já existem na 001 (`internal/domain/pagepath.go`); a T001 reconfere.
- **Ajustes feitos em outras features nesta revisão:** 002/T039 (regras 1, 4, 5 e 6 de exposição), 002/T040 (trava do sistema e `.serve.lock.holder`), 007/T028 (regras 2 e 3), 013/T016 (resolução de executável com PATHEXT).

## Dependências
- 014, 007, 002; `docs/deploy.md`, `docs/airgapped-install.md`, `docs/https-via-proxy.md`, `docs/windows.md`, `docs/windows-native-extras.md`, `docs/backup.md`, `docs/hook-scripts-and-container-deploy.md` §4 e §5, `docs/cli.md` §3.4 e §3.5.

## Checklist de Prontidão
- [x] Propósito claro e sem ambiguidade
- [x] Escopo delimitado (dentro/fora)
- [x] Cenários cobrem happy path e alternativos
- [x] Edge cases identificados com comportamento esperado
- [x] Constraints explícitas
- [x] Critérios de sucesso mensuráveis
- [x] Abordagem escolhida justificada
