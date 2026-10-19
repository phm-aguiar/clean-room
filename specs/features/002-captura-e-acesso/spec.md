# Spec: Captura e acesso (hooks, sessões, handoffs, MCP e CLI)

## Metadados
- **ID:** 002
- **Status:** approved
- **Autor:** phm-aguiar
- **Data:** 2026-10-12 (atualizada e aprovada em 2026-10-13; revisada contra os docs na geração do plano, mesma data)
- **Feature Anterior:** 001 (Núcleo), que fornece domínio, armazenamento e wiki. A 001 está implementada e verde (`scripts/check.sh`, lint com 0 apontamentos). Segue aberta na fase 5 do `tasks.md` da 001 só a T090 (fechamento sem CI verde), que não bloqueia esta spec. T082 e T089 foram resolvidas pelos docs em 2026-10-13.
- **Discovery Report:** `reports/001-ai-memory-go-clean-room-discovery.md` (F002)
- **Aprovado:** true

## Propósito
> Para quem, o quê e por quê.

Com o núcleo pronto, o sistema precisa capturar o trabalho dos agentes sem esforço manual e entregá-lo ao
próximo agente. Esta feature liga o Claude Code ao ai-memory por hooks de ciclo de vida, transforma cada
sessão em uma página durável por regras (sem LLM), abre um handoff para quem vier depois e expõe a
memória por MCP e por uma CLI. O público é o autor, que alterna entre agentes no mesmo diretório.

Importa agora porque a promessa central do produto é sair de um agente no meio da tarefa e continuar em
outro sem re-explicar o contexto. Os hooks são caminho quente do agente, então precisam ser rápidos,
limitados e idempotentes.

## Escopo

### Dentro do escopo
- **Ingestão** (`internal/ingest`): `POST /hook` e `/hook/batch` (202 ao enfileirar, 429 na saturação, ≤ 1024 em voo, ≤ 256 itens por lote, 413 acima disso), `GET /handoff` (contexto de início de sessão), chave de ingestão idempotente na mesma transação da observação, sanitização na admissão.
- **Ledger de eventos**: cada evento processado acrescenta uma linha ao arquivo mensal `log-AAAA-MM.md` do projeto (`docs/lifecycle-event-ingestion.md` §13).
- **CLI de hook**: `ai-memory hook --event ...` com spool local (`{created_ms:013}-{pid}-{seq:016x}-{recovery_key}.json`), trava de arquivo do sistema operacional (`LockFileEx` no Windows / `flock` no Unix sobre `<data_dir>/hook-spool/.drain.lock`), drenagem desacoplada (`hook-drain`, oculto) e política de captura por marker (`.ai-memory.toml`); nunca bloqueia o agente. Em estouro de capacidade (10 000 arquivos, verificado sob `<data_dir>/hook-spool.capacity.lock`), preserva o backlog existente e descarta o **novo evento recebido** com aviso no stderr.
- **Sessão**: SessionEnd gera `sessions/<id>.md` por regras (tier episodic), e encerra a sessão e grava o handoff automático numa só transação. Sessão efêmera (sem prompt, pré-ferramenta nem pós-ferramenta) encerra sem página e sem handoff, e **reabre** (`state = 'open'`, limpando `accepted_at`, `accepted_by_session` e `accepted_by_agent`) o handoff mais recente aceito por aquela sessão efêmera para não perder o bastão.
- **Handoffs**: `begin`, `accept`, `list` e `cancel`, reivindicados exatamente uma vez, páginas compartilhadas e handoffs com dono. O `cancel` marca como expirado o handoff aberto de identificador exato, para que a próxima sessão não o consuma (`docs/mcp-server.md` §3.3).
- **Instância única e processo vivo (INV-09)**: `ai-memory serve` segura a trava exclusiva do sistema operacional sobre `<data_dir>/.serve.lock` (`LockFileEx` no Windows / `flock` no Unix, com PID informativo em `.serve.lock.holder`) enquanto roda. A guarda de `reset`, `restore`, `reindex` e `uninstall --purge-data` **enumera os processos do sistema** (comparando `ai-memory` e `ai-memory.exe` sem diferenciar maiúsculas no Windows) e recusa se houver outro processo `ai-memory` vivo (`docs/cli.md` §3.1, `docs/overview.md` item 9). Substitui o `ai-memory.pid` da 001 (origem: 001/T086).
- **Horário de ocorrência (`occurred_at`)**: lido do topo do corpo do evento; vale se for positivo e no máximo 5 minutos à frente do relógio do servidor, senão "agora" (`docs/lifecycle-event-ingestion.md` §3.3). É campo de entrada da observação e vira `created_at`; o esquema não tem a coluna (`docs/domain-vocabulary.md` §5.2, `docs/database-schema.md`). A 001 já grava assim. **Sem migração nova.**
- **Servidor MCP** (`internal/mcp`, stdio e HTTP streamable, autenticação Bearer), 12 tools: `memory_query`, `memory_read_page`, `memory_recent`, `memory_write_page`, `memory_status`, `memory_briefing`, `memory_delete_page`, `memory_read_session_observations`, `memory_handoff_begin`, `memory_handoff_accept`, `memory_handoff_list` e `memory_handoff_cancel` (`docs/architecture.md`, `docs/mcp-server.md` §3).
- **CLI** (`internal/cli`, único compositor): `serve`, `hook`, `install-hooks --agent claude-code` (hooks em `~/.claude/settings.json`; no Windows nativo usa formato de execução direta com `command` e array `args`, e `"timeout": 15` em `SessionEnd`), `install-mcp --client claude-code` (servidor em `~/.claude.json`) e `status` (`docs/cli.md` §8, `docs/mcp-install.md`, `docs/windows.md`).
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
- **Performance:** `ai-memory hook` retorna em < 200 ms sem rede; POST do hook com timeout padrão de 2 s, configurável (ADR-04); `/hook` não bloqueia em fila cheia.
- **Tecnologia:** `mark3labs/mcp-go` atrás de interface própria, `cobra`/`koanf`, `modernc.org/sqlite`, sem CGO; Windows e Linux. O pacote `mcp` não importa `ingest` em produção.
- **Prazo:** não especificado.
- **Segurança/Compliance:** autenticação Bearer com comparação em tempo constante; escuta padrão `127.0.0.1:49374`; bind fora de loopback sem autenticação é recusado; limite de corpo de 10 MiB. A resolução de projeto do hook é caminho de escrita e **cria** workspace e projeto quando necessário, mas recusa o nome reservado `_global` (captura nunca vai para o escopo global) e nunca grava como caminho de repositório um diretório que não é raiz git (`docs/lifecycle-event-ingestion.md` §7.1).

## Herdado da 001
> Obrigações que a 001 deixou para esta feature. Cada item aponta para a task ou o item de auditoria de origem.

- **Handoff exatamente uma vez e ponteiro de projeto ativo por ator (INV-16, auditoria D5):** a 001 não os implementou. A 002 traz a obrigação com teste multi-sessão explícito e `-race`; teste de sessão única não basta (constituição).
- **Guarda de processo vivo (origem: 001/T086, movida para cá em 2026-10-13):** ver "Instância única e processo vivo" em "Dentro do escopo". O mecanismo de PID da 001 é trocado pelo do doc.
- **`occurred_at` (origem: 001/T087, movida para cá em 2026-10-13):** a revisão dizia que faltava a coluna; não procede (ver "Horário de ocorrência"). Sobra para a 002 só ler o campo do corpo do hook.
- **Escopo criado só por escrita (auditoria D3):** `wiki.Reindex` e o watcher chamam `EnsureScope` ao descobrir diretórios UUID. Nas rotas novas, só caminhos de escrita criam escopo (`memory_write_page` com nome explícito e a resolução de projeto do hook); leitura, busca e caminhos destrutivos usam busca sem criação.
- **Frontmatter que o MCP aceita (001/T084 e T085, já implementadas):** `memory_write_page` recebe frontmatter de fora. A leitura aceita escalares YAML não-texto (`pinned: true`, data sem aspas, `okf_version: 0.2`) como texto literal, e uma escrita sem `pinned` herda a fixação da versão anterior; só `pinned: false` explícito desafixa. A tool deve se apoiar nesse comportamento e não reimplementá-lo.
- **Escopo `_global` (001/T083, já implementada):** `[[_global:x]]` resolve para `default/_global`, como `docs/wiki-files-layer.md` define. `memory_write_page` com `scope: "global"` e a união de `_global` em `memory_query` usam o mesmo escopo.
- **Correções da 001 contra os docs (fase 1b do `tasks.md`, T079 a T093):** frontmatter com mapeamentos (`generated: {by, at}`, `sources`, `relations`) e conformidade OKF na escrita; diretório de dados padrão e `--data-dir`; escritas rastreadas no git, com o commit fora do `WritePage`, para o fim de sessão fazer um único commit `session <id8>: <título>`; escrita de página sem linha em `log.md` e sem `index.md`, e `_slots/` fixado; backup como tarball gerado pelo servidor (`POST /admin/backup`); reindex com manifesto obrigatório e checagem de banco limpo; preenchimento de `_meta.md` no boot; watcher registrando remoções no git; exclusão de todas as versões do caminho (usada por `memory_delete_page`); ressurreição de lápide que limpa o `superseded_at`; semente de leitura do ponteiro de projeto ativo no boot (T092); sanitizador alinhado aos 27 padrões exatos, com `allowlist` e `extra_patterns` (T093, `docs/prompts-schemas-and-sanitizer.md` §11 e `docs/domain-vocabulary.md` §7.3). A lista com as fontes está na seção "Revisão contra os docs" da spec 001.
- **Portões de qualidade:** `scripts/check.sh` (build cruzado `CGO_ENABLED=0`, `go vet`, `go test -race` via container `golang:<go.mod>` quando não há gcc) e `golangci-lint v2.14.0` com 0 apontamentos. O CI do GitHub nunca rodou; fica fora do critério desta feature até ser executado.

## Decisões tomadas (2026-10-13)
> Substituem as questões em aberto do rascunho. Onde o doc de origem cobre o ponto, a decisão segue o doc; onde é omisso, é decisão do autor.

- **`[[_global:x]]`** resolve para o workspace padrão (`default/_global`), a menos que se escreva `workspace/_global`. Fonte: `docs/wiki-files-layer.md`. Implementada na 001/T083.
- **Frontmatter com escalares não-texto** (`true`, números, datas sem aspas) é aceito como texto literal; a reescrita pelo sistema emite com aspas. O doc é omisso; decisão do autor. Implementada na 001/T084.
- **`pinned` ausente numa escrita** mantém a fixação da versão anterior; só `pinned: false` explícito desafixa. O doc é omisso; decisão do autor. Implementada na 001/T085.
- **`occurred_at`:** tratado nesta feature, mas **sem migração**: a revisão contra os docs mostrou que é campo de entrada e vira `created_at` (ver "Revisão contra os docs").
- **`cancel` de handoff:** dentro do escopo.
- **Auditoria D5 (handoff e ponteiro por ator):** é entregue por esta feature; a obrigação está em "Herdado da 001". As D1 a D4 foram resolvidas pelos docs (D2 e D3) ou como decisão nossa onde o doc é omisso (D1 e D4), e incorporadas à constituição em 2026-10-13 (001/T089).

## Revisão contra os docs (2026-10-13)
> Neste projeto o doc de origem vence a spec. Ao gerar o `plan.md`, a spec aprovada foi comparada com os docs e ajustada sem mudar a aprovação. O que mudou:

| # | Antes | Agora | Fonte |
|---|---|---|---|
| 1 | 9 tools, com uma `memory_handoff` de ações | 12 tools; o handoff são `memory_handoff_begin`, `_accept`, `_list` e `_cancel` | `docs/architecture.md`, `docs/mcp-server.md` §3 |
| 2 | `occurred_at` por migração V02 | Campo de entrada que vira `created_at`; sem migração. A 001 já grava assim | `docs/domain-vocabulary.md` §5.2, `docs/database-schema.md` |
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
- [ ] `scripts/check.sh` e o lint passam sem apontamentos
- [ ] Testes automatizados cobrem o happy path e os edge cases identificados

## Abordagem Escolhida
CLI como único compositor (cobra + koanf). A ingestão é um roteador HTTP padrão com semáforo de 1024 e
a idempotência fica na transação da observação. O hook nativo usa spool em arquivo com lock, para nunca
bloquear o agente. O MCP usa `mark3labs/mcp-go` (escolha do autor) atrás de uma interface do pacote
`mcp`, para permitir trocar o SDK. O adaptador de hooks do Claude Code fica isolado, com fixtures de
payload versionadas. Decisões em ADR-04, 05, 06 e 09 do discovery.

Débitos relevantes: dependência de um SDK de comunidade que pode divergir da especificação MCP; formatos
de hook do Claude Code podem mudar; locks de arquivo e processos vivos diferem entre Windows e Linux.

### Alternativas Consideradas
| Abordagem | Trade-off | Por que não |
|-----------|-----------|-------------|
| SDK oficial `modelcontextprotocol/go-sdk` | Acompanha a spec | O autor escolheu `mark3labs/mcp-go` |
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
- [x] Questões em aberto resolvidas (ver "Decisões tomadas")
- [x] Obrigações herdadas da 001 aceitas pelo autor
