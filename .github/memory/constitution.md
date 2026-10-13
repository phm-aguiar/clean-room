# Constituição Arquitetural

> Este arquivo define as regras incontornáveis do projeto.
> **Os docs (`docs/`) são a referência do produto e têm prioridade sobre os artefatos do SDD e o código.**
> Um doc nunca é modificado para se ajustar a uma task, a uma spec ou ao código; ajustes nos docs (organização,
> links, correções) só por decisão explícita do dono do produto. O produto é tudo o que os docs descrevem, limitado
> ao escopo das specs. Esta constituição, como todo artefato do SDD, é derivada deles: quando divergir de um doc, é ajustada para seguir o doc e a mudança é
> registrada na feature que a motivou. Onde o doc é omisso, a regra é "decisão nossa", marcada como tal.
>
> Origem: invariantes de `docs/overview.md` §4 (numeração `INV-nn` é a do projeto de origem), regras de
> fronteira, grafo de dependências (§9) e decisões do discovery `reports/001-ai-memory-go-clean-room-discovery.md`.
> Itens marcados **(a confirmar)** não foram verificados no código de referência; a decisão tomada está em ADR.
> Os antigos docs adiados (LLM, embeddings, auto-melhoria) foram incorporados a `docs/` em 2026-10-13.
> Revisão de 2026-10-13: INV-01, INV-06, INV-09, INV-10, resolução de escopo e mutações da wiki alinhadas aos
> docs; decisões D1 a D4 da auditoria da 001 incorporadas.

## Portões de Qualidade
<!-- Regras automatizadas que bloqueiam PRs se violadas -->

- `CGO_ENABLED=0 go build ./...` e `go vet ./...` passam para `windows/amd64` e `linux/amd64`.
- `go test -race ./...` passa. O `-race` exige cgo e um compilador C. Se não houver gcc no host, o caminho padrão é um container Docker `golang:<major.minor de go.mod>` (a imagem oficial já traz gcc), executado por `scripts/check.sh` e `scripts/check.ps1`. O binário entregue continua sem CGO (arquitetura).
- Toda mudança em resolução de escopo, supersessão de página, identidade de sessão, aceitação de handoff, escritor SQLite ou filtros de dono exige **teste multi-sessão explícito** (INV-16). Teste de sessão única não basta.
- Cada invariante tem um teste de contrato nomeado com o seu `INV-nn`, ou um débito registrado em ADR.
- Nenhuma feature começa sem `Aprovado: true` no `spec.md` e sem cenário Gherkin correspondente.
- Dependência nova só entra se listada em `.github/memory/tech.md`; senão, registrar ADR antes.

## Restrições Arquiteturais
<!-- Decisões estruturais que não podem ser violadas -->

- **Binário único `ai-memory`** em módulo Go único, sem CGO, layout `cmd/ai-memory` + `internal/{domain,config,storage,wiki,ingest,mcp,cli}`.
- **Grafo de dependências (imports)**, aplicado por teste de imports:
  - `domain` e `config` não importam nenhum outro componente. `domain` **não faz I/O**; `config` é o único que lê arquivo de configuração e ambiente.
  - `storage` → `domain`. `wiki` → `domain`, `storage`. `ingest` → `domain`, `storage`, `wiki`.
  - `mcp` → `domain`, `storage`, `wiki`; **não** importa `ingest` em produção (só em testes).
  - `cli` é o único que compõe objetos e conhece todos os componentes.
- **INV-01** Configuração carregada uma única vez por processo, em um único pacote (`internal/config`, chamado pela `cli`). Nenhum outro código de produção lê `os.Getenv`/`os.LookupEnv`/`os.Environ`. Precedência: padrões < `config.toml` (no diretório de dados, ou `--config`) < `.env` local < variáveis do processo `AI_MEMORY_*` < sobreposições explícitas (`--data-dir`, `AI_MEMORY_AUTH_TOKEN`, `AI_MEMORY_SERVER_URL`) (`docs/cli.md` §2.1). O caminho rápido do hook usa um carregador mínimo, também em `internal/config`, que lê só as variáveis de tempo `AI_MEMORY_HOOK_*` (`docs/cli.md` §5.3). O `.env` fica na raiz do repositório, **nunca é versionado** (está no `.gitignore`) e é documentado em `.env.example`, sem segredos. **Testes** podem ler e definir variáveis de ambiente (`t.Setenv`) para exercitar a configuração (decisão nossa, D4 da auditoria da 001).
- **INV-02** Escritor SQLite único: goroutine dona da conexão de escrita, canal de capacidade 1024. Agrupar trabalho quente em um comando/transação; sem N+1. Leitura em pool somente leitura.
- **INV-03** Índices (FTS, entidades, links) commitam na **mesma transação** dos dados. Sem "indexar depois do retorno".
- **INV-04** Identidade tipada `(workspace_id, project_id, path)` em toda linha e página de domínio.
- **INV-05** Hooks são fire-and-forget e limitados: `/hook` responde 202 ao enfileirar e 429 na saturação (≤ 1024 em voo; ≤ 256 itens por lote). Sem fan-out ilimitado nem filas nos caminhos de hook. Timeout de POST do hook: padrão 2 s, configurável (ADR-04; **a confirmar** no original).
- **INV-06** A remoção de privacidade é fronteira de tipo: observação só é armazenada após a sanitização, que remove segredos **antes** de limitar tamanho (título ≤ 80 chars; corpo com o teto do tipo de entrada, 16 KiB por padrão e, em eventos de hook, o limite por evento de `docs/lifecycle-event-ingestion.md` §3.6). O tipo sanitizado só é criável pelas funções do pacote de sanitização, e o zero value é o texto vazio (decisão nossa, D1 da auditoria da 001: o doc marca esse ponto como "a confirmar").
- **INV-09** `reset`, `restore`, `reindex` e `uninstall --purge-data` consultam os processos do sistema e recusam antes de tocar no diretório de dados se houver outro processo `ai-memory` vivo. O `serve` segura a trava de instância única `.serve.lock`. `backup` é online: o cliente pede ao servidor (`POST /admin/backup`) um tarball com o snapshot do banco feito pela API de backup do SQLite, com o escritor ativo (`docs/overview.md` item 9, `docs/cli.md` §3.1 e §9, `docs/lifecycle-ops.md`).
- **INV-10** Escritas de arquivo são atômicas: temporário + `fsync` + `rename` + `fsync` do diretório. O watcher ignora as próprias escritas pelo prefixo do arquivo temporário. Exceção do próprio produto: os ledgers `log.md` e `log-AAAA-MM.md` são **somente acréscimo** (`docs/wiki-files-layer.md` §9, `docs/lifecycle-event-ingestion.md` §13; D2 da auditoria da 001).
- **INV-11** Diretório de dados absoluto e canônico, registrado no log na inicialização. Padrão: diretório de dados local do usuário com a subpasta `ai-memory`; `--data-dir` > `AI_MEMORY_DATA_DIR` > arquivo (`docs/design-decisions.md`).
- **INV-12** Reservada. A entrada não existe na documentação de origem; nenhuma regra é imposta (ADR-04).
- **INV-13** Caminho padrão **zero-LLM**: o sistema funciona completo sem provedor.
- **INV-15** Assinantes de log filtram o próprio módulo (sem laço de realimentação).
- **INV-16** Multi-sessão é capacidade central:
  - Páginas são compartilhadas; handoffs têm dono. A autoria da página **nunca** vira filtro de leitura.
  - Escrita concorrente divergente **supersede, nunca destrói**: a versão perdedora continua alcançável.
  - Um handoff é reivindicado **exatamente uma vez**, com duas guardas independentes de estado `aberto` (consulta de metadados e atualização de reivindicação). Manter as duas.
  - O ponteiro de "projeto ativo" é chaveado pela coordenada do chamador (por ator).
- **Resolução de escopo:** toda rota nova (MCP, administração, web) usa o resolvedor central. Nunca montar busca de workspace/projeto à mão. Leitura, busca, embedding, retenção e caminhos destrutivos usam busca **sem criar**: escopo explícito inexistente ou parcial (só workspace ou só projeto) **falha fechado**; leitura **sem** escopo degrada pelo ponteiro de projeto ativo, pela semente de startup e pelo padrão, e informa a origem; escrita sem escopo e sem ponteiro resolvido é recusada (`docs/storage-and-retrieval.md`). Só caminhos de escrita criam workspace ou projeto: a escrita explícita, a resolução de projeto do hook (que recusa o nome reservado `_global`; `docs/lifecycle-event-ingestion.md` §7.1) e o `reindex`, que recria o escopo a partir do `_meta.md` mantendo os UUIDs (`docs/lifecycle-ops.md`; D3 da auditoria da 001).
- **Autorização:** via checagem de capacidade do nível de autenticação, nunca por comparação de nome de usuário nem checagem de root ad hoc. Tokens comparados em tempo constante.
- **Mutações da wiki** passam só pela gravação de página, aplicação de lote ou helpers destrutivos existentes (sanitização, admissão, atribuição, rollback e atualização de índice andem juntos). Nenhum handler escreve arquivo da wiki diretamente. As escritas são rastreadas pelo adaptador git, e os commits automáticos acontecem nos pontos definidos pelo doc (fim de sessão, checkpoints, `memory_write_page`, comando de commit), incluindo só os caminhos registrados (`docs/wiki-files-layer.md` §7).
- **Markdown em git é a fonte da verdade**; o SQLite é índice derivado e reconstruível a partir dos arquivos.

## Regras de Negócio Transversais
<!-- Regras que afetam múltiplas features -->

- Todo endpoint de ingestão é idempotente por chave de ingestão; o replay não duplica observação e converge os efeitos downstream (at-least-once).
- Se o upsert no banco falhar depois que o arquivo foi instalado, o arquivo é revertido e o erro é propagado ao cliente. O `reindex` reconcilia o resto.
- Uma observação nunca chega ao armazenamento sem passar pelo sanitizador.
- Tombamento de páginas removidas do disco é opcional e desligado por padrão; remoção física exige comando explícito.
- Escopo do produto em fases, na ordem de `specs/features/roadmap.md`: 001 a 015 são zero-LLM; LLM e embeddings só entram de 016 a 022. As invariantes de LLM (INV-07: saídas estruturadas por JSON Schema; INV-08: `{provider, model, dim}` junto de todo embedding; INV-14: autenticação do provedor resolvida antes do cliente) passam a valer a partir da 016. O binário continua `CGO_ENABLED=0` também nessas features (o embedding local da 017 é inferência em Go puro).

## Preferências de Bibliotecas
<!-- Libs e frameworks homologados para o projeto -->

- SQLite: `modernc.org/sqlite` (pure Go). Sem `mattn/go-sqlite3`.
- MCP: `mark3labs/mcp-go`, sempre atrás de interface do pacote `mcp`.
- CLI: `cobra`. Config: `koanf` (TOML + env `AI_MEMORY_*`, separador `__`). Não usar `viper`.
- IDs: UUIDv7 via `google/uuid`. Logs: `log/slog`. Sanitizador: `regexp` da stdlib (RE2).
- Git: executável `git` do sistema via `os/exec`. Não usar `go-git`.
- Preferir a stdlib sempre que ela resolver.

## Anti-Padrões Proibidos
<!-- Abordagens vetadas no projeto -->

- Proibido ler variáveis de ambiente ou o `.env` fora de `internal/config` em código de produção (testes podem usar `t.Setenv`, ver INV-01); proibido versionar `.env` ou qualquer segredo.
- Proibido escrever arquivos da wiki fora da camada `wiki`.
- Proibido tarefa "indexar depois" fora da transação de dados.
- Proibido filtrar leitura de página por autoria/operador.
- Proibido criar workspace ou projeto em caminho de leitura, busca ou destrutivo.
- Proibido concatenar caminho de página à mão; usar o tipo de caminho do domínio.
- Proibido `panic` em caminho de execução; preferir fallback explícito ou erro tipado.
- Proibido abstração preemptiva, superfície pública sem chamador, código morto e stubs de trabalho futuro (vai para notas de design).
- Proibido goroutine sem cancelamento por `context` e fan-out ilimitado.
- Proibido tratar "a confirmar" como fato: decidir e registrar em ADR como decisão nossa.
- Proibido copiar código, nomes internos ou estrutura do binário original além do que a documentação especifica (clean-room).
