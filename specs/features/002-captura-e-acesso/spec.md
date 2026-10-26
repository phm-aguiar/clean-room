# Spec: Captura e acesso (hooks, sessões, handoffs, MCP e CLI)

## Metadados
- **ID:** 002
- **Status:** spec auditada contra o original (relatório `reports/010-auditoria-002.md`); sem plano nem tasks; não há código de reimplementação no workspace
- **Autor:** phm-aguiar
- **Data:** 2026-10-12 (revisada em 2026-10-25)
- **Feature Anterior:** 001 (Núcleo), que fornece domínio, armazenamento e wiki.
- **Discovery Report:** — (o relatório de discovery não está mais em `reports/`)
- **Aprovado:** true

## Propósito
> Para quem, o quê e por quê.

Com o núcleo pronto, o sistema precisa capturar o trabalho dos agentes sem esforço manual e entregá-lo ao
próximo agente. Esta feature liga o Claude Code ao ai-memory por hooks de ciclo de vida, transforma cada
sessão em uma página durável por regras (sem LLM), abre um handoff para quem vier depois e expõe a
memória por MCP e por uma CLI. O público é o autor, que alterna entre agentes no mesmo diretório.

Importa agora porque a promessa central do produto é sair de um agente no meio da tarefa e continuar em
outro sem re-explicar o contexto. Os hooks são caminho quente do agente, então precisam ser rápidos,
limitados e idempotentes. Em toda dúvida, o comportamento é o mais parecido possível com o original,
inclusive nos defeitos conhecidos descritos em `docs/`.

## Escopo

### Dentro do escopo
- **Ingestão**: `POST /hook` e `/hook/batch` (202 ao enfileirar, 429 na saturação, ≤ 1024 em voo, ≤ 256 itens por lote, 413 acima disso), `GET /handoff` (contexto de início de sessão), chave de ingestão idempotente na mesma transação da observação, sanitização na admissão.
- **Ledger de eventos**: cada evento processado acrescenta uma linha ao arquivo mensal `log-AAAA-MM.md` do projeto (`docs/lifecycle-event-ingestion.md` §13).
- **CLI de hook**: `ai-memory hook --event ...` com spool local (`{created_ms:013}-{pid}-{seq:016x}-{recovery_key}.json`), trava de arquivo do sistema operacional (`LockFileEx` no Windows / `flock` no Unix sobre `<data_dir>/hook-spool/.drain.lock`), drenagem desacoplada (`hook-drain`, oculto) e política de captura por marker (`.ai-memory.toml`); nunca bloqueia o agente. Em estouro de capacidade (10 000 arquivos, verificado sob `<data_dir>/hook-spool..capacity.lock`, com os dois pontos que o original produz; a espera de 50 ms pela trava nunca descarta o evento), preserva o backlog existente e descarta o **novo evento recebido** com aviso no stderr.
- **Sessão**: SessionEnd gera `sessions/<id>.md` por regras (tier episodic), e encerra a sessão e grava o handoff automático numa só transação. Sessão efêmera (sem prompt, pré-ferramenta nem pós-ferramenta) encerra sem página e sem handoff, e **reabre** (`state = 'open'`, limpando `accepted_at`, `accepted_by_session` e `accepted_by_agent`) o handoff mais recente aceito por aquela sessão efêmera para não perder o bastão.
- **Handoffs**: `begin`, `accept`, `list` e `cancel`, reivindicados exatamente uma vez, páginas compartilhadas e handoffs com dono. O `cancel` marca como expirado o handoff aberto de identificador exato, para que a próxima sessão não o consuma (`docs/mcp-server.md` §3.3).
- **Instância única e processo vivo (INV-09)**: `ai-memory serve` segura a trava exclusiva do sistema operacional sobre `<data_dir>/.serve.lock` (`LockFileEx` no Windows / `flock` no Unix, com PID informativo em `.serve.lock.holder`) enquanto roda. A guarda de `reset`, `restore`, `reindex` e `uninstall --purge-data` **enumera os processos do sistema** (comparando `ai-memory` e `ai-memory.exe` sem diferenciar maiúsculas no Windows) e recusa se houver outro processo `ai-memory` vivo (`docs/cli.md` §3.1, `docs/overview.md` item 9). Não há arquivo de PID.
- **Horário de ocorrência (`occurred_at`)**: lido do topo do corpo do evento; vale se for positivo e no máximo 5 minutos à frente do relógio do servidor, senão "agora" (`docs/lifecycle-event-ingestion.md` §3.3). É campo de entrada da observação e vira `created_at`; o esquema não tem a coluna (`docs/domain-vocabulary.md` §5.2, `docs/database-schema.md`). **Sem migração nova.**
- **Servidor MCP** (stdio e HTTP streamable, autenticação Bearer, protocolo fixado em `2024-11-05` como no original, `docs/mcp-server.md` §1), 12 tools: `memory_query`, `memory_read_page`, `memory_recent`, `memory_write_page`, `memory_status`, `memory_briefing`, `memory_delete_page`, `memory_read_session_observations`, `memory_handoff_begin`, `memory_handoff_accept`, `memory_handoff_list` e `memory_handoff_cancel` (`docs/architecture.md`, `docs/mcp-server.md` §3).
- **CLI** (único compositor): `serve`, `hook`, `install-hooks --agent claude-code` (hooks em `~/.claude/settings.json`; no Windows nativo usa formato de execução direta com `command` e array `args`, e `"timeout": 15` em `SessionEnd`), `install-mcp --client claude-code` (servidor em `~/.claude.json`) e `status` (`docs/cli.md` §8, `docs/mcp-install.md`, `docs/windows.md`).
- Ponteiro de projeto ativo por ator; resolução de escopo conforme `docs/mcp-server.md` §2.1 e `docs/storage-and-retrieval.md`.

### Fora do escopo (explicitamente)
- LLM, embeddings, `memory_consolidate`, `memory_auto_improve`, `memory_explore`, `answer` com síntese LLM (features 016 a 022; no modo zero-LLM, `answer: true` retorna `answer_unavailable`).
- Multiusuário, SSO, grants por projeto, webhooks de admissão (features 007, 008 e 009).
- Hooks de outros agentes além do Claude Code, workstreams (`ai-memory run`), mensagens entre agentes.
- Navegador web, perfil entre projetos e demais tools MCP (feedback, forget_sweep, lint, install_self_routing).

## Comportamento Esperado

### Cenário Principal (Happy Path)
1. `ai-memory install-hooks --agent claude-code --apply` registra os hooks e `ai-memory install-mcp --client claude-code --apply` registra o servidor MCP; o servidor sobe com `ai-memory serve`.
2. Em cada evento (SessionStart, UserPromptSubmit, PostToolUse, SessionEnd), o `ai-memory hook` aplica a política de captura, grava no spool e envia ao `/hook`, que responde 202.
3. A observação sanitizada e a chave de ingestão são gravadas na mesma transação.
4. No SessionEnd, o servidor escreve `sessions/<id>.md` sem LLM, abre o handoff automático e faz commit git.
5. Em outro agente, no mesmo diretório, o hook de início de sessão (`GET /handoff`) ou `memory_handoff_accept` entrega o resumo; `memory_query` encontra o conhecimento da sessão anterior.

### Cenários Alternativos
- **Servidor saturado (429):** o hook mantém o evento no spool e retorna ao agente sem bloquear; um lote com mais de 256 itens é rejeitado por inteiro.
- **Leitura com escopo explícito inexistente:** `memory_query` e `memory_read_page` falham fechado (não encontrado), sem criar workspace nem projeto. **Leitura sem escopo** nunca falha: degrada pelo ponteiro de projeto ativo, pela semente de startup e pelo padrão do servidor, e informa a origem (`docs/storage-and-retrieval.md`).
- **Escrita sem escopo com ponteiro divergente (`Mismatch`):** quando o ponteiro de projeto ativo já recebeu entradas por chave neste processo (ou o usuário possui entrada só de identidade) e nenhuma entrada casa com a coordenada da sessão do chamador (`Mismatch`), a escrita é recusada como escrita ambígua (`-32602 ambiguous unscoped write`), nunca gravando no padrão em silêncio. Quando nenhuma entrada chaveada jamais foi publicada e o slot compartilhado está vazio (`Unset`, ex.: modo `stdio` ou servidor recém-iniciado sem hooks), a escrita sem escopo recai para o escopo padrão configurado (`default/scratch`).
- **Falha no banco após escrever o arquivo:** `memory_write_page` reverte o arquivo e retorna erro de escrita não ambíguo.
- **Servidor offline:** o evento fica no spool e é drenado por um helper com lock quando o servidor voltar. O spool descarta entradas com mais de **7 dias** e, **cheio** (10 000 arquivos), mantém o atraso existente e descarta o evento **novo** com aviso (`docs/external-lifecycle.md` §8) — não expulsa os mais antigos.
- **Instalação nova (`init`):** gera um token raiz em `<dados>/auth-tokens/default`, registra `[auth].bearer_token_file` e semeia as credenciais dos hooks, de modo que o servidor exige bearer em todas as rotas menos `/healthz`; instalações existentes seguem sem autenticação em loopback até `ai-memory auth enable` (`docs/users.md` § "Local root bearer token file"; detalhe na 007).
- **Item de lista de descarte do servidor** (`[ingest] drop_tool_use_id_prefixes` / `drop_tool_names`): o evento de ferramenta é reconhecido com 202 e descartado (`dropped_policy`) antes de sanitizador, limitador e armazenamento (`docs/lifecycle-event-ingestion.md` §4.6).

## Edge Cases
- **Replay de evento (idempotência):** a mesma chave, inclusive após interrupção no meio do SessionEnd, não duplica observação; os efeitos downstream convergem.
- **Handoff disputado:** dois agentes aceitam o mesmo handoff; exatamente um vence, o outro recebe `handoff: null` (duas guardas independentes; `docs/design-handoff-offer-not-claim.md` §7).
- **Escrita concorrente na mesma página:** vira cadeia de supersessão, sem perder a versão perdedora.
- **Segredo no payload do hook:** removido antes de gravar. Depois da remoção, título ≤ 80 caracteres e corpo limitado por tipo de evento: 16 KiB para prompt, pós-compactação, início e fim de sessão, pré-compactação e subagentes; 2 KB para notificação, pré e pós-ferramenta, parada e outro (`docs/lifecycle-event-ingestion.md` §3.6).
- **Campo bruto da resposta do assistente** (`last_assistant_message`): sempre removido antes de qualquer etapa; nunca chega a log, spool, rede ou armazenamento (`docs/lifecycle-event-ingestion.md` §5). A captura opt-in fica fora desta feature.
- **Dois agentes no mesmo projeto:** o ponteiro de projeto ativo é chaveado por ator e não é sobrescrito pelo outro.

## Constraints
- **Performance:** `ai-memory hook` retorna em < 200 ms sem rede; POST do hook com timeout padrão de 2 s, configurável; `/hook` não bloqueia em fila cheia.
- **Tecnologia:** Go 1.26; SDK MCP atrás de interface própria, sem mudar o contrato observável do original (protocolo `2024-11-05`, respostas e erros de `docs/mcp-server.md`); `cobra`/`koanf`, `modernc.org/sqlite`, `log/slog`, sem CGO; Windows primeiro, depois Linux. O pacote `mcp` não importa `ingest` em produção (convenção futura de pacotes Go).
- **Prazo:** não especificado.
- **Segurança/Compliance:** autenticação Bearer com comparação em tempo constante; escuta padrão `127.0.0.1:49374`; bind fora de loopback sem autenticação é recusado; limite de corpo de 10 MiB. A resolução de projeto do hook é caminho de escrita e **cria** workspace e projeto quando necessário, mas recusa o nome reservado `_global` (captura nunca vai para o escopo global) e nunca grava como caminho de repositório um diretório que não é raiz git (`docs/lifecycle-event-ingestion.md` §7.1).

## Herdado da 001
> Obrigações que a 001 deixa para esta feature.

- **Handoff exatamente uma vez e ponteiro de projeto ativo por ator (INV-16):** com teste multi-sessão explícito e `go test -race`; teste de sessão única não basta (constituição).
- **Escopo:** criam escopo só as operações listadas em `docs/auto-scope.md` (`memory_write_page`, `memory_handoff_begin`, o ingresso de hooks e `POST /workstream/runs`); a reindexação e o watcher não criam (diretório órfão é pulado). Leitura, busca e caminhos destrutivos usam busca sem criação.
- **Frontmatter que o MCP aceita:** `memory_write_page` recebe frontmatter de fora e segue o contrato de YAML do original (`docs/wiki-files-layer.md` §3): `pinned: true` é booleano, `yes` e `007` são texto, datas são texto; só o booleano `true` fixa; **não há herança** de `pinned` da versão anterior; `_slots/` é sempre fixado.
- **Escopo `_global`:** `[[_global:x]]` resolve para `default/_global` (`docs/wiki-files-layer.md`). `memory_write_page` com `scope: "global"` e a união de `_global` em `memory_query` usam o mesmo escopo.
- **Comportamentos da 001 usados aqui** (ver a spec 001): escritas rastreadas no git, com o fim de sessão fazendo um único commit `session <id8>: <título>`; escrita de página sem linha de ledger e sem `index.md`; backup como tarball gerado pelo servidor (`POST /admin/backup`); reindex com manifesto obrigatório e checagem de banco limpo; preenchimento de `_meta.md` no boot; watcher registrando remoções no git; exclusão de todas as versões do caminho com quarentena (usada por `memory_delete_page`); ressurreição de lápide que limpa o `superseded_at`; semente de leitura do ponteiro de projeto ativo no boot; sanitizador com os 27 padrões, `allowlist` e `extra_patterns` (`docs/prompts-schemas-and-sanitizer.md` §11); fluxo de observações da busca, ligado por padrão.
- **Portões de qualidade:** `CGO_ENABLED=0 go build ./...`, `go vet ./...` e `go test -race ./...` em `windows/amd64` (MVP) e `linux/amd64`.

## Revisão contra os docs (2026-10-13)
> Neste projeto o doc de origem vence a spec. O que mudou na spec ao compará-la com os docs:

| # | Antes | Agora | Fonte |
|---|---|---|---|
| 1 | 9 tools, com uma `memory_handoff` de ações | 12 tools; o handoff são `memory_handoff_begin`, `_accept`, `_list` e `_cancel` | `docs/architecture.md`, `docs/mcp-server.md` §3 |
| 2 | `occurred_at` por migração V02 | Campo de entrada que vira `created_at`; sem migração | `docs/domain-vocabulary.md` §5.2, `docs/database-schema.md` |
| 3 | `ai-memory install` grava hooks em `settings.json` | `install-hooks` (`~/.claude/settings.json`) e `install-mcp` (`~/.claude.json`) | `docs/cli.md` §8, `docs/mcp-install.md` |
| 4 | `serve` grava `ai-memory.pid` e a guarda lê o arquivo | `.serve.lock` para instância única; a guarda enumera os processos do sistema | `docs/cli.md` §3.1, `docs/overview.md` item 9, `docs/security-boundaries.md` §10 |
| 5 | "nenhuma rota de hook cria escopo fora do ponteiro ativo" | O hook cria workspace e projeto quando necessário; recusa só `_global` | `docs/lifecycle-event-ingestion.md` §7.1 |
| 6 | (ausente) | Ledger mensal `log-AAAA-MM.md` por evento | `docs/lifecycle-event-ingestion.md` §13 |
| 7 | Leitura sem escopo resolvível falha fechado | Só escopo **explícito** inexistente falha fechado; leitura sem escopo degrada pelo ponteiro, semente e padrão | `docs/storage-and-retrieval.md` (resolução de escopo) |
| 8 | Corpo ≤ 16 KiB | Limite por tipo de evento: 16 KiB ou 2 KB | `docs/lifecycle-event-ingestion.md` §3.6 |
| 9 | Perdedor do handoff recebe "já reivindicado" | Perdedor recebe `handoff: null` | `docs/design-handoff-offer-not-claim.md` §7 |
| 10 | (ausente) | `last_assistant_message` sempre removido; sessão efêmera sem página nem handoff; `GET /handoff` no escopo | `docs/lifecycle-event-ingestion.md` §5, §8.3 e §11 |

## Critérios de Sucesso
- [ ] Fluxo ponta a ponta: hook de Claude Code → observação → `sessions/<id>.md` → handoff → `memory_query` encontra o resultado
- [ ] As 12 tools MCP respondem por stdio e HTTP com autenticação Bearer
- [ ] Handoff disputado por dois aceites tem um único vencedor (teste multi-sessão com `-race`)
- [ ] Replay do mesmo evento não gera segunda observação
- [ ] 429 na saturação e hook sem bloqueio verificados em teste de carga com 1025 requisições em voo
- [ ] INV-05 e INV-16 têm teste de contrato nomeado
- [ ] Um segundo `ai-memory serve` no mesmo `data_dir` recusa enquanto o primeiro segura `.serve.lock`; com outro processo `ai-memory` vivo, `reindex` recusa (INV-09), e sem ele prossegue
- [ ] Evento com `occurred_at` válido grava o instante original em `created_at`; ausente, no futuro além de 5 minutos ou não positivo, grava o instante da gravação
- [ ] `install-hooks` e `install-mcp` são idempotentes: a segunda aplicação não toca o disco, e entradas de terceiros nos arquivos do Claude Code são preservadas
- [ ] `go vet ./...`, `go test -race ./...` e `golangci-lint run` passam sem apontamentos
- [ ] Testes automatizados cobrem o happy path e os edge cases identificados

## Abordagem Escolhida
CLI como único compositor (cobra + koanf). A ingestão é um roteador HTTP padrão com semáforo de 1024 e
a idempotência fica na transação da observação. O hook nativo usa spool em arquivo com lock, para nunca
bloquear o agente. O MCP fica atrás de uma interface do pacote `mcp`, para que o SDK escolhido não mude o
contrato observável do original (o protocolo fica fixado em `2024-11-05` mesmo que o SDK negocie outra
versão). O adaptador de hooks do Claude Code fica isolado, com fixtures de payload versionadas.

Riscos: formatos de hook do Claude Code podem mudar; locks de arquivo e processos vivos diferem entre
Windows e Linux; o SDK MCP pode exigir adaptação para reproduzir o pino de protocolo.

### Alternativas Consideradas
| Abordagem | Trade-off | Por que não |
|-----------|-----------|-------------|
| Implementação própria de JSON-RPC | Zero dependência | Mais código e manutenção do protocolo |
| Dois binários (servidor e hook mínimo) | Cold start menor do hook | Dois artefatos; sem evidência de necessidade |
| Hooks para Claude Code e Codex | Prova a troca entre agentes diferentes | Dobra o trabalho de adapters; Codex vai para a 010 |

## Dependências
- Feature 001 (domínio, armazenamento, wiki).
- `docs/lifecycle-event-ingestion.md`, `marker-file.md`, `mcp-server.md`, `api-reference.md`, `cli.md`, `mcp-install.md`, `harness-adapters.md`, `design-handoff-offer-not-claim.md`.
- `.github/memory/constitution.md` (INV-05, INV-16, grafo de imports) e `tech.md`.

## Checklist de Prontidão
- [x] Propósito claro e sem ambiguidade
- [x] Escopo delimitado (dentro/fora)
- [x] Cenários cobrem happy path e alternativos
- [x] Edge cases identificados com comportamento esperado
- [x] Constraints explícitas
- [x] Critérios de sucesso mensuráveis
- [x] Abordagem escolhida justificada
- [x] Questões em aberto resolvidas: reproduzir o original (ver "Herdado da 001" e "Correções da auditoria")

## Correções da auditoria

Auditoria 002 (2026-10-25), docs conferidos contra o código original. Relatório completo em
`reports/010-auditoria-002.md`.

- `GET /recap` (recapitulação somente leitura da última compactação, disparada pelo prompt
  `reload context`) existia no código e faltava nos docs: documentada em
  lifecycle-event-ingestion.md §2.5, cli.md §5.2 (passo 9a) e api-reference.md §1.
- Ordem de admissão do `/hook` corrigida (excerto do assistente antes da checagem de aliases;
  lista de descarte `[ingest]` entre rascunho do Cursor e subagente); motivo 202
  `ingest drop list` acrescentado.
- `/hook/batch` processa os itens dentro da requisição (não em segundo plano); valores de
  `outcome` e regras de omissão de `accepted_indices`/`failed_index` documentados.
- Servidor MCP tem **24** ferramentas (faltava `memory_restore_page`, somente administrador);
  contagem corrigida em mcp-server.md, api-reference.md e mcp-install.md.
- `memory_message_pop`: atomicidade confirmada (troca condicional `pending → claimed` em
  transação). `memory_message_cancel` sem `message_id` cancela todas as pendentes enviadas.
  `send`/`pop`/`cancel` recusam escopo inferido (mensagem literal registrada).
- "A confirmar" resolvidos: redação de segredos na configuração, encerramento (cada espera
  limitada a 5 s, checkpoint do WAL), nome da pasta `hooks`, renovação OIDC (margem de 60 s),
  limite de corpo do lote (10 MiB), chave `backfill_on_start` ausente do modelo padrão, término
  de sessões filhas não inferido.
- Funcionalidade ausente registrada: drenagem do `session-end` no próprio processo para o
  agente Zero (10 s / 20 s); trava de capacidade da fila e espera de 50 ms não descartante.
- Segunda rodada da auditoria (2026-10-25):
  - `GET /admin/metrics` (texto Prometheus, 6 famílias, rótulos fechados) estava ausente e foi
    documentado em mcp-server.md §6.
  - A resposta 401 das rotas de máquina é texto simples `auth required` com
    `WWW-Authenticate: Bearer realm="ai-memory", error="invalid_token"`, não JSON. As mensagens
    400 do proxy de identidade foram registradas literalmente.
  - O KDF de senha aceita no máximo 32 trabalhos pendentes; os parâmetros Argon2id foram
    registrados.
  - A deduplicação A3 de frias (opt-in, depende de embeddings) foi registrada em
    consolidation.md.
  - "Reload context" foi documentado em usage.md.
  - Bootstrap: o padrão do servidor é 50 000 tokens e o da CLI é 150 000.
  - Código `-32600` para recusas de autorização em programmatic-memory.md.
  - Não existe importador de vault OKF/Obsidian (cookbook.md).
  - Devin: hooks mesclados sob a chave `hooks` (mcp-install.md).
  - Command Code tem workstreams via `ai-memory run`, não via `install-hooks`.
  - Pegada de memória do mapa de projeto ativo: não há limite de tamanho das chaves
    (auto-scope.md). Resolvido: reproduzir o original, sem teto de chaves.
  - Regra de acesso no ingresso confirmada (external-lifecycle.md).
  - Contagem de 24 ferramentas corrigida em design-decisions.md (com a linha da tabela),
    design-hindsight-borrowings.md e desktop/plan.md.
- Terceira rodada da auditoria (2026-10-25):
  - auto-scope.md lista agora qual ferramenta MCP cria escopo (`memory_write_page`,
    `memory_handoff_begin`, além do ingresso de hooks e de `POST /workstream/runs`) e quais só
    buscam, com acesso de leitura ou de escrita.
  - design-decisions.md: a prioridade entre candidatos de handoff foi verificada (chave
    completa de escolha).
  - mcp-install.md: o MSIX do Claude Desktop exige **exatamente um** pacote com configuração;
    a mensagem de erro está registrada, e as variáveis de ambiente têm precedência. Todo
    "a confirmar" restante virou **depende do produto externo**, também em auto-scope.md e
    usage.md.
  - Wiki: as pastas de projeto são `<raiz>/<id do workspace>/<id do projeto>/`, nomeadas pelos
    UUIDs e não pelos nomes, num único repositório git na raiz (usage.md e cookbook.md
    corrigidos).
  - Flags e subcomandos citados em usage.md e cookbook.md existem na CLI.
  - `docs/assets/fixtures/capture-policy.json` foi sincronizado com os vetores de teste
    originais: 120 decisões, 256 vetores de shell e as seções novas
    `read_many_files_directories`, `windows`, `windows_posix_controls`, `windows_scopes`,
    `marker_spellings` e `tool_families`. As notas foram reescritas sem nomes de
    implementação, e test-fixtures.md foi atualizado.
