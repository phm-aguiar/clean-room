# 016 — Fidelidade das specs ao original

Data: 2026-10-25. Escopo: `specs/features/*/spec.md`, `001-nucleo/notes/*`, `002-captura-e-acesso/notes/*`,
`acceptance/*.feature`, `roadmap.md`. O código original foi lido só para desempate, por meio dos docs já
corrigidos; nenhum trecho de código foi copiado. `docs/` não foi alterado. Nenhum `plan.md`/`tasks.md` foi criado.

Regra aplicada: em toda dúvida, reproduzir o original (inclusive defeitos conhecidos); nomenclatura e
ferramentas no estilo Go 1.26 (`go test -race`, `go vet`, `golangci-lint run`, `log/slog`, pacotes em
minúsculas; `internal/...` só como convenção futura).

## Arquivos removidos (lidos inteiros antes)

| Arquivo | Motivo |
|---|---|
| `001-nucleo/notes/bench-baseline.md` | Medição do benchmark do código Go apagado (`test/bench`, `storage.Search`); sem valor de requisito. |
| `001-nucleo/notes/constitution-audit.md` | Auditoria (T072) do código Go apagado (`internal/cli`, `internal/storage`...), com "decisões nossas" D1–D6. |
| `001-nucleo/notes/phase0-findings.md` | ADRs da fase 0 (T007) com decisões nossas já revogadas (`schema_migrations`, nomes reservados, título 80, regexes próprias, commit por evento, `log.md`, não regravar arquivo idêntico). |
| `001-nucleo/notes/inv-test-matrix.md` | Matriz de nomes de teste do código apagado (`test/contract/*`). |
| `001-nucleo/notes/sanitizer-patterns.md` | 18 regexes RE2 "decisão nossa" e faixas bidi erradas; contradizia os 27 padrões do original (doc §11). |
| `001-nucleo/notes/fixtures-contract.md` | Desenho de testes baseado em plan/tasks (T013, T057, T060, `internal/domain/sanitize/testdata`); inventário desatualizado. |
| `002-captura-e-acesso/notes/phase0-findings.md` | Consolidação T006 com pendências para "o lead" e decisões contraditórias (D6 "recusar" escrita `Unset`); conteúdo válido já está nos docs. |
| `002-captura-e-acesso/notes/mcp-go-evaluation.md` | Avaliação da biblioteca `mark3labs/mcp-go` (T004); premissa de biblioteca, não de comportamento. |
| `002-captura-e-acesso/notes/adr-protocolo-mcp.md` | ADR do adaptador `internal/mcp/adapter_mcpgo.go` (T033) e testes do código apagado; o requisito (pino 2024-11-05) está no doc. |

## Arquivos ajustados (antes → depois)

### 001-nucleo
- `spec.md` (reescrita, mesmo conteúdo de requisito):
  - Status "concluída / reaberta, tasks T091–T119" → "spec auditada, sem plano nem tasks".
  - Removidas as seções "Revisão contra os docs", "Revisão de clean room", "Auditoria contra a referência", "Revisão 3.1.1" e "Tasks da passada 2" (colunas "Go hoje" e tasks); evidência de `scripts/check.sh` e `notes/inv-test-matrix.md`; critérios `[x]` que dependiam do código.
  - Decisões em aberto → reproduzir o original: escrita de página não cria escopo; sem herança de `pinned`; arquivo idêntico é regravado; `refinery_schema_history` (antes `schema_migrations`); reindex sem higienizar; sem nomes reservados na validação de caminho; allowlist/extra vazios aceitos (antes "recusados na construção"); fluxo de observações ligado por padrão (antes "desvio registrado"); diretório de dados canonicalizado como no original e forma `\\?\` mantida.
  - Git: "git do sistema no Windows" → biblioteca embutida com fallback para `git` só no Windows; reimplementação reproduz o comportamento observável.
  - "Correções da auditoria": decisões marcadas como resolvidas ("reproduzir o original"); `PageKind` registrado como inexistente.
- `notes/domain-contract.md` (reescrita): retiradas LACUNAs baseadas no PLAN (`PageKind`, `ParsePagePath`, `\` convertido, tipos de ID, `schema_migrations` prevalecendo); relação desconhecida "vira referência" → ignorada com aviso; barreira de tipo só para observações; truncamento < 64 bytes com reticências.
- `notes/search-contract.md` (reescrita): sem PLAN/TASKS; fluxo de vetor/observações como no original; normalização de tags "espaços removidos" → "só pontas aparadas, bônus uma vez"; seção de tasks removida.
- `notes/markdown-contract.md`: referência a `phase0-findings`/ADR-17 → regra do doc (ordem e estilo); "não regrava idêntico" → regrava os mesmos bytes; fechamento sem quebra aceito; título sem teto; relação desconhecida ignorada; `index.md` página comum; lacunas fechadas.
- `notes/wiki-layout-contract.md` (reescrita): commit por evento e linha em `log.md` (PLAN P-09, ADR-21/25) → sem commit na escrita, ledger mensal só de hooks; `_meta.md`/`index.md` conforme doc; commit MCP `mcp write-page <path>`; Windows 32/33 e 5.
- `acceptance/nucleo.feature`: cabeçalho e cenário de conteúdo idêntico ("mantém os bytes" → "regrava os mesmos bytes, nova data de modificação").

### 002-captura-e-acesso
- `spec.md`: status/metadados sem T090/`scripts/check.sh`; pacotes `internal/*` removidos; trava de capacidade `hook-spool.capacity.lock` → `hook-spool..capacity.lock` (defeito do original); `mark3labs/mcp-go` "escolha do autor" → SDK atrás de interface sem mudar o contrato (pino 2024-11-05); "Herdado da 001" e "Decisões tomadas" reescritos: escalares YAML não-texto "como texto" e herança de `pinned` (decisões do autor T084/T085) → contrato do original; critério de lint/check → `go vet`, `go test -race`, `golangci-lint run`; "teto de chaves" do ponteiro → resolvido sem teto.
- `notes/mcp-contract.md`: sem plan/Analyst/"decisão nossa"; 24 ferramentas no original; respostas de `write_page`, `delete_page`, `handoff_begin/accept/cancel/list` alinhadas ao doc (`any_owner`, `{page_id, path, checkpoint}`, `-32603`); accept sem filtro de 10 min; 401 em texto `auth required`; sem teto de chave.
- `notes/hook-contract.md`: "decisão nossa"/lacunas para o plan → lacunas fechadas pelos docs; fixtures no `testdata/` (convenção Go).
- `notes/handoff-contract.md`: referências a plan Q-04/Q-05/Q-13, `SelectHandoff`, `*Tx` removidas; "ago ago" marcado como defeito a reproduzir; cenário 6 movido para a 008.
- `notes/capture-contract.md`: PLAN e "decisão nossa" removidos; `drop_subagent_captures`/`drop_subagent` sem conflito.
- `acceptance/captura.feature`: referência a `phase0-findings.md` removida.

### 003 a 022
- Todas: linha de Status → "spec auditada contra o original…; sem plano nem tasks"; critério `scripts/check.sh` → `go vet ./...`, `go test -race ./...`, `golangci-lint run`.
- 003: referências T019/T027/T039–T041/T080/T081 removidas; `merge-workspace` "excluindo `_profile`" → `_profile.*` não filtrado (como no original).
- 004, 006: referências T022/T042/T080/T088/T090/T091 e `internal/config` removidas.
- 005: "sem preenchimento de dados antigos (decisão nossa)" → preenchimento na abertura, como no original.
- 010: "contratos vêm da fase 0" → `docs/harness-adapters.md`.
- 012: "A task T002…" → "A implementação…"; relatório 006 inexistente.
- 013: aceitar `copilot` no diário, normalizar `\\?\`, paginar exportação e preservar código do filho → reproduzir o original (§6.10/§6.11); relatório 007 inexistente; "fase 0" e plano/tasks removidos.
- 014: "ADR no plano" → `tech.md`, saída igual à do original.
- 015: "Fallback de Git não se aplica" e referências a `internal/wiki/atomicfile.go`, `internal/domain/pagepath.go`, T001/T016/T028/T039/T040 → comportamento observável do fallback do original; diferença registrada (git no PATH).
- 016: "nova rodada de plano/tarefas" e aceitar quebra de linha em `AI_MEMORY_LLM_HEADERS` → só vírgula, como no original; relatório 004 inexistente.
- 017: T002 e "decisão nossa" → benchmark Go e "tecnologia, não comportamento"; relatório 004 inexistente.
- 018: T002/"decisão nossa registrada no plano" → fluxo de dispositivo do original; relatório 004 inexistente.
- 020: cabeçalhos de módulo "decisão a tomar no Go (`doc.go`)" → só `.rs` e `//!`, como no original.
- 008 (sem assinatura de webhooks) e 003 (`delete-workspace` sem `confirm`) já reproduziam o original; sem mudança.

### roadmap.md
- Removidas a coluna Tasks (88, 93, 52…), os estados "concluída", "aprovada, pronta para executar" e "draft + plano + tasks provisórias", e as referências 002/T039, T040, T049, T093, 005/T023–T027, 007/T028, 013/T016.
- Todas as features: "spec auditada (relatório em `reports/`); sem plan/tasks". Referência ao discovery apagado removida.

## Observações
- Specs 012, 013, 016–018 citavam relatórios (`001`, `004`, `006`, `007`) que não existem mais em `reports/`; as citações foram trocadas por "relatório dessa rodada não está mais em `reports/`".
- 016, 017 e 018 continuam em CRLF; os demais arquivos em LF.
- Restam menções a `internal/<pacote>` nas "Abordagem Escolhida" de 006–022, mantidas como convenção futura de pacotes Go.
- Git: substituído pela seção "Verificação" abaixo (decisão do usuário: biblioteca embutida como no original).

## Verificação (2026-10-26)

### 1. Git igual ao original
- Conferido no código original: biblioteca Git embutida é o caminho normal; o programa `git` é fallback **só no Windows**. Gatilho: a abertura do repositório falha com erro que não é de dono e é "não encontrado" ou erro do sistema com `failed to resolve path`. Usos: validação na abertura, commit, contagem de commits, checkpoints recentes, leitura por revisão. `init`: no Windows, qualquer falha cai para `git init -q`. Antes de usar o `git`, valida o confinamento e `rev-parse --show-toplevel/--absolute-git-dir/--git-common-dir`; roda com `-C <raiz>` e sem as variáveis `GIT_*` de diretório/índice/objetos. Commit do fallback: `rm --cached --force` dos ledgers, `add -A` sem os ledgers (pathspecs NUL pela entrada padrão), `diff --cached --quiet` sem mudança = nenhum commit, `commit -q -m` só com `user.name`/`user.email` fixados (assinatura e hooks não são desligados; reproduzir), `rev-parse HEAD`.
- Ajustados: `001-nucleo/spec.md` (Tecnologia: `go-git` como caminho normal; Abordagem; tabela de alternativas: "só `git`" vira a alternativa rejeitada), `001-nucleo/notes/wiki-layout-contract.md` (d) (regra completa acima), `015-implantacao/spec.md` (Git no Windows). Nenhuma outra spec/nota tratava do assunto.

### 2. `001-nucleo/notes/search-contract.md` linha a linha
Conferida contra `docs/storage-and-retrieval.md` §8, §11.2–§11.5 e o código original. Correções na reconstrução:
- FTS: faltavam rank BM25 sem pesos, `snippet` de `body` com `<mark>`, `…` e 24 tokens.
- Entidades: faltava a tokenização exata; `freq` segue o mesmo filtro de versão da busca (não expiradas; todas as versões com `include_superseded`), conferido no código.
- Vetor: faltavam tripla igual e produto escalar. Grafo: ordem das sementes (FTS, vetor, abstract, entidades), primeira posição vence, para na janela.
- Observações: faltavam limites (`min(janela×4, 1000)`, 16 tokens, 3 ids, voto em `sessions/<id>.md`, filtro de dono) e a regra de contribuição FTS zero na fusão.
- Pós-fusão: descritor (600/240), leitura única dos dados de autoridade, página sem dados fica sem fator; empate por bytes; vários escopos.
- Autoridade: tipo por `kind` convertido para texto sem aparar e sensível à caixa (conferido no código), prefixos sem caixa ASCII; tags em minúsculas **ASCII** (a nota dizia minúsculas genéricas), itens não texto ignorados, ordem de cálculo e exclusividade `_lint/`/`investigations/`.
- Consulta FTS: detalhes de token, citação (aspas dobradas, `*`), sonda por thread e degradação também quando a sonda não abre.
- Reforço: pulo sem renovar o instante, segundo plano, lote numa transação.
- `raw_hits`: era atribuído a `docs/mcp-server.md` §3.1; agora segue SR §11.3.1 (camada MCP, mesmo limite, 24 tokens, filtro de dono, vários escopos).

### 3. Fim de linha e linhas "Status:"
- Padrão original (verificado antes das edições): só 016, 017 e 018 em CRLF; todas as outras specs, notas, `.feature` e `roadmap.md` em LF. Estado atual: 016 (100/100 linhas com CR), 017 (100/100), 018 (89/89); demais 0 CR. Sem `\r\r`.
- O `perl` só reescreveu linhas que começavam com `- **Status:** ` e o critério exato de `scripts/check.sh`. Linhas com "Status:" fora da linha 5: só `017:96` era conteúdo (`- **Status:** \`latest_pages_missing_embeddings\`…`), já restaurada com o texto original; as demais (`**Status: Fiel**` em 019–022) não casavam o padrão e estão intactas.
