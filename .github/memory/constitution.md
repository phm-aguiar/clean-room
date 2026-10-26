# Constituição Arquitetural

> Este arquivo define as regras incontornáveis do projeto.
> **Os docs (`docs/`) são a referência do produto e têm prioridade sobre os artefatos do SDD e o código.**
> Um doc nunca é modificado para se ajustar a uma task, a uma spec ou ao código; ajustes nos docs só por decisão
> explícita do dono do produto. O produto é tudo o que os docs descrevem, limitado ao escopo das specs. Esta
> constituição é derivada dos docs: quando divergir de um doc, é ajustada para seguir o doc.
>
> **Fidelidade ao original:** em toda dúvida, a reimplementação se comporta o mais parecido possível com o
> ai-memory original, **inclusive nos defeitos conhecidos**. Os docs foram auditados contra o código original
> (relatórios em `reports/`). Onde um doc ainda marcar algo como "a confirmar", a resposta vem do comportamento do
> original, não de uma decisão nossa. Nomenclatura, pacotes e idioma do código seguem Go 1.26 idiomático.
>
> Origem: invariantes de `docs/overview.md` §4 (numeração `INV-nn` do projeto de origem), regras de fronteira e
> grafo de dependências. Revisão de 2026-10-26: alinhada aos docs auditados; removidas a camada `.env`, o Git só por
> `os/exec` e as "decisões nossas" que divergiam do original.

## Portões de Qualidade
<!-- Regras automatizadas que bloqueiam PRs se violadas -->

- `CGO_ENABLED=0 go build ./...` e `go vet ./...` passam para `windows/amd64` (primeiro MVP) e `linux/amd64`.
- `go test -race ./...` passa. O `-race` exige cgo e um compilador C no host ou num container `golang:1.26`; o binário entregue continua sem CGO.
- `golangci-lint run` passa.
- Toda mudança em resolução de escopo, supersessão de página, identidade de sessão, aceitação de handoff, escritor SQLite ou filtros de dono exige **teste multi-sessão explícito** (INV-16). Teste de sessão única não basta.
- Cada invariante tem um teste de contrato nomeado com o seu `INV-nn`.
- Comportamento observável (mensagens, códigos, formatos, limites) é testado contra os valores literais dos docs e das fixtures de `docs/assets/fixtures/`.
- Nenhuma feature começa sem `Aprovado: true` no `spec.md` e sem cenário Gherkin correspondente.
- Dependência nova só entra se listada em `.github/memory/tech.md`.

## Restrições Arquiteturais
<!-- Decisões estruturais que não podem ser violadas -->

- **Binário único `ai-memory`** em módulo Go único, sem CGO. Layout planejado: `cmd/ai-memory` + `internal/{domain,config,storage,wiki,ingest,mcp,cli}` (pacotes novos entram conforme as features).
- **Grafo de dependências (imports)**, aplicado por teste de imports:
  - `domain` e `config` não importam nenhum outro componente. `domain` **não faz I/O**; `config` é o único que lê arquivo de configuração e ambiente.
  - `storage` → `domain`. `wiki` → `domain`, `storage`. `ingest` → `domain`, `storage`, `wiki`.
  - `mcp` → `domain`, `storage`, `wiki`; **não** importa `ingest` em produção (só em testes).
  - `cli` é o único que compõe objetos e conhece todos os componentes.
- **INV-01** Configuração carregada uma única vez por processo, em `internal/config` (chamado pela `cli`). Nenhum outro código de produção lê o ambiente. Camadas, da menor para a maior precedência (`docs/cli.md` §2.1): padrões internos < `config.toml` (no diretório de dados, ou `--config`) < variáveis `AI_MEMORY_*` (separador `__`) < sobreposições explícitas (`AI_MEMORY_AUTH_TOKEN`, `AI_MEMORY_SERVER_URL`, `AI_MEMORY_ADMISSION_WEBHOOKS_JSON`, `--data-dir`). Não há camada `.env`. O ambiente que não é chave de configuração é capturado uma vez; as únicas leituras posteriores são as exceções do doc (diretório de backup e as variáveis `AI_MEMORY_HOOK_*` do caminho do hook, `docs/cli.md` §5.3). Testes podem usar `t.Setenv`.
- **INV-02** Escritor SQLite único: goroutine dona da conexão de escrita, canal de capacidade 1024. Agrupar trabalho quente em um comando/transação; sem N+1. Leitura em pool somente leitura.
- **INV-03** Índices (FTS, entidades, links) commitam na **mesma transação** dos dados.
- **INV-04** Identidade tipada `(workspace_id, project_id, path)` em toda linha e página de domínio.
- **INV-05** Hooks são fire-and-forget e limitados: `/hook` responde 202 ao enfileirar e 429 na saturação (1024 em processamento simultâneo); `/hook/batch` aceita até 256 itens e responde 413 acima disso (`docs/lifecycle-event-ingestion.md`). Timeouts de envio dos hooks conforme `docs/hook-scripts-and-container-deploy.md` e `docs/cli.md` §5.
- **INV-06** Sanitização antes do armazenamento: segredos são removidos **antes** de limitar tamanho (título ≤ 80 caracteres; corpo com o teto do tipo de entrada, `docs/lifecycle-event-ingestion.md` §3.6). Como no original, a barreira de tipo vale para observações; páginas, handoffs manuais, mensagens, motivo de feedback e eventos de workstream são sanitizados por quem chama (`docs/storage-and-retrieval.md`, `docs/prompts-schemas-and-sanitizer.md` §11).
- **INV-09** `reset`, `restore`, `reindex` e `uninstall --purge-data` consultam os processos do sistema e recusam antes de tocar no diretório de dados se houver outro processo `ai-memory` vivo, com a mesma comparação de nome do original (`docs/lifecycle-ops.md`). O `serve` segura a trava de instância única `.serve.lock`. `backup` é online: o cliente pede ao servidor (`POST /admin/backup`) um tarball com o snapshot do banco feito pelo backup online do SQLite (`docs/lifecycle-ops.md`).
- **INV-10** Escritas de arquivo são atômicas: temporário + `fsync` + `rename` + `fsync` do diretório; no Windows, só os erros 32 e 33 são repetidos (`docs/wiki-files-layer.md`). O watcher ignora as próprias escritas pelo prefixo do temporário. Exceção: o ledger de hooks é **somente acréscimo** no arquivo mensal `log-AAAA-MM.md` (mês UTC); `log.md` é só reconhecido e nunca recebe linhas novas (`docs/wiki-files-layer.md` §9).
- **INV-11** Diretório de dados absoluto, registrado no log na inicialização. Precedência: `--data-dir` > `AI_MEMORY_DATA_DIR` > `data_dir` do `config.toml` > padrão (`docs/cli.md` §2).
- **INV-12** Reservada. A entrada não existe na documentação de origem.
- **INV-13** Caminho padrão **zero-LLM**: o sistema funciona completo sem provedor.
- **INV-15** Assinantes de log filtram o próprio módulo (sem laço de realimentação).
- **INV-16** Multi-sessão é capacidade central:
  - Páginas são compartilhadas; handoffs têm dono. A autoria da página **nunca** vira filtro de leitura.
  - Escrita concorrente divergente **supersede, nunca destrói**: a versão perdedora continua alcançável.
  - Um handoff é reivindicado **exatamente uma vez**, com duas guardas independentes de estado `aberto` (consulta de metadados e atualização de reivindicação).
  - O ponteiro de "projeto ativo" é chaveado pela coordenada do chamador (por ator).
- **Resolução de escopo:** toda rota nova (MCP, administração, web) usa o resolvedor central. Leitura, busca, embedding, retenção e caminhos destrutivos usam busca **sem criar**: escopo explícito inexistente ou parcial **falha fechado**; leitura sem escopo degrada pelo ponteiro de projeto ativo, pela semente de startup e pelo padrão, e informa a origem; escrita sem escopo e sem ponteiro resolvido é recusada. Os caminhos que criam escopo são os de `docs/auto-scope.md` (escrita de página, `memory_handoff_begin`, ingresso de hooks, `POST /workstream/runs`) e o `reindex`, que recria o escopo a partir do `_meta.md` mantendo os UUIDs.
- **Autorização:** via checagem de capacidade do nível de autenticação, nunca por comparação de nome de usuário nem checagem de root ad hoc. Tokens comparados em tempo constante. A matriz por rota e ferramenta é a do original (`docs/design-per-project-authz.md`), inclusive as consultas por id sem escopo.
- **Mutações da wiki** passam só pela camada `wiki` (gravação de página, aplicação de lote e operações destrutivas, com sanitização, admissão, atribuição, rollback e índice juntos). Nenhum handler escreve arquivo da wiki diretamente; a exceção, como no original, são `reset`, `uninstall --purge-data` e `restore`, que trocam `wiki/`, `db/` e `raw/` inteiros com o servidor parado (`docs/design-decisions.md` §3). Os commits automáticos acontecem nos pontos definidos em `docs/wiki-files-layer.md` §7.
- **Markdown em git é a fonte da verdade**; o SQLite é índice derivado e reconstruível a partir dos arquivos.

## Regras de Negócio Transversais
<!-- Regras que afetam múltiplas features -->

- Todo endpoint de ingestão é idempotente por chave de ingestão; o replay não duplica observação e converge os efeitos downstream (at-least-once).
- Se o upsert no banco falhar depois que o arquivo foi instalado, o arquivo é revertido e o erro é propagado ao cliente. O `reindex` reconcilia o resto.
- A lápide de páginas que sumiram do disco (`reconcile_tombstones_deleted_pages`) é opcional e desligada por padrão; remoção física exige comando explícito.
- Escopo do produto em fases, na ordem de `specs/features/roadmap.md`: 001 a 015 são zero-LLM; LLM e embeddings só entram de 016 a 022. As invariantes de LLM (INV-07: saídas estruturadas por JSON Schema; INV-08: `{provider, model, dim}` junto de todo embedding; INV-14: autenticação do provedor resolvida antes do cliente) valem a partir da 016. O binário continua `CGO_ENABLED=0` também nessas features (o embedding local da 017 é inferência em Go puro).

## Preferências de Bibliotecas
<!-- Libs e frameworks homologados para o projeto -->

- SQLite: `modernc.org/sqlite` (pure Go). Sem `mattn/go-sqlite3`.
- MCP: SDK atrás de interface do pacote `mcp`, sem mudar o contrato observável do original (protocolo fixado em `2024-11-05`, respostas e erros de `docs/mcp-server.md`). Candidato: `mark3labs/mcp-go`.
- CLI: `cobra`. Config: `koanf` (TOML + env `AI_MEMORY_*`, separador `__`). Não usar `viper`.
- IDs: UUIDv7 via `google/uuid`. Logs: `log/slog`.
- Sanitizador: os padrões de segredo exigem a semântica Unicode do original (`docs/prompts-schemas-and-sanitizer.md` §11.2); o `regexp` (RE2) da stdlib só serve onde reproduz essa semântica, sem retrocesso.
- Git, igual ao original: biblioteca embutida `github.com/go-git/go-git/v5` como caminho normal; o programa `git` só como fallback no Windows, nos casos de `docs/windows-native-extras.md` §6.2.
- Preferir a stdlib sempre que ela resolver.

## Anti-Padrões Proibidos
<!-- Abordagens vetadas no projeto -->

- Proibido ler variáveis de ambiente fora de `internal/config` em código de produção (testes podem usar `t.Setenv`); proibido versionar segredos.
- Proibido escrever arquivos da wiki fora da camada `wiki` (salvo as exceções de INV-09 acima).
- Proibido tarefa "indexar depois" fora da transação de dados.
- Proibido filtrar leitura de página por autoria/operador.
- Proibido criar workspace ou projeto em caminho de leitura, busca ou destrutivo.
- Proibido concatenar caminho de página à mão; usar o tipo de caminho do domínio.
- Proibido `panic` em caminho de execução; preferir erro tipado.
- Proibido abstração preemptiva, superfície pública sem chamador, código morto e stubs de trabalho futuro.
- Proibido goroutine sem cancelamento por `context` e fan-out ilimitado.
- Proibido "melhorar" o comportamento do original: divergências (inclusive correção de defeitos conhecidos) só com decisão explícita do dono do produto registrada nos docs.
- Proibido tratar "a confirmar" como decisão livre: a resposta é o comportamento do original.
- Proibido copiar código, nomes internos ou estrutura do binário original além do que a documentação especifica (clean-room).
