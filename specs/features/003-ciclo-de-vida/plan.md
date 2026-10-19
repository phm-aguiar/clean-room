# plan.md: Plano de Implementação Técnica — Ciclo de vida dos dados (003)

> Plano gerado em 2026-10-13, antes de a 002 ser executada. Nomes de arquivo e de função da 002 citados aqui são os do `tasks.md` da 002; revisar quando a 002 terminar.

## 1. Metadados do Plano
- **Stack Tecnológico:** a da 002. Nada novo: `archive/tar` e `compress/gzip` da stdlib para o `restore` (o tarball é o do backup da 002/T081).
- **Feature Fonte:** `specs/features/003-ciclo-de-vida/spec.md`
- **Escopo:** purga de projeto e sessão, expiração de handoffs, renomear projeto e workspace, apagar workspace, mover projeto e sessão, checkpoints, restaurar página, commit, `restore`, `reset`, `uninstall --purge-data`, compactação e tombstones. Sem webhooks de admissão (008), `reorg` (004) nem agentes além do Claude Code (010).
- **Fontes de contrato:** `docs/lifecycle-ops.md` (fonte principal, comando a comando), `docs/api-reference.md` §3.4 e §3.5 (campos de `merge-workspace` e `compact`), `docs/storage-and-retrieval.md` §10, `docs/wiki-files-layer.md` §7 e §8.1, `docs/security-boundaries.md` §10, `docs/cli.md` §8, §9 e §9.1, `docs/mcp-server.md` §6.

### Layout
```text
internal/storage/   ← + lifecycle_*.go: exclusões em transação única, tombstones, re-stamp de workspace, contagens de prévia
internal/wiki/      ← + trava exclusiva de mutação, remoção de árvore de escopo, cópia de página para fusão, restaurar página do git
internal/cli/       ← + rotas /admin/* da 003 e comandos finos; restore, reset e uninstall diretos em disco
internal/ingest/    ← + checagem de tombstone na admissão do evento
```
Nenhum pacote novo. O grafo de imports da constituição não muda.

## 2. Design de Contratos e Fronteiras
- **Contrato:** rotas `/admin/*` e comandos da CLI de `docs/lifecycle-ops.md`, com os campos de requisição e de relatório e os códigos de status de lá (400 sem confirmação, 404 escopo ou sessão fora do escopo, 409 guardas, 422 validação, 500 falha antes do commit, 200 com `files_failed` depois do commit).
- **Esquema SQL:** sem migração. Usa `purged_sessions`, `purged_scopes`, `audit_log`, `managed_runs` (para a checagem de lease) e as tabelas que recebem o novo `workspace_id` no movimento real, todas já na V01.

### 2.1 Rotas

| Operação | Rota | Requisição | Relatório |
|---|---|---|---|
| Purga de projeto | `POST /admin/purge-project` | `workspace`, `project` **ou** `project_id` (mutuamente exclusivos; 422 se ambos/nenhum), `confirm`, `dry_run`, `force`, `compact` | `label`, `pages_deleted`, `sessions_deleted`, `observations_deleted`, `handoffs_deleted`, `embeddings_deleted`, `workstreams_deleted`, `managed_runs_deleted`, `workstream_ids`, `collateral_observations_deleted`, `collateral_handoffs_denulled`, `compacted`, `files_deleted`, `files_failed`, `pre_checkpoint`, `checkpoint` (`dry_run: true` ainda checa leases vivos e retorna 409 sem `force: true`) |
| Purga de sessão | `POST /admin/purge-session` | `session_id`, `confirm`, `dry_run`, `compact` (resolve escopo direto pela linha `sessions`) | `session_id`, `workspace`, `project`, `observations_deleted`, `handoffs_deleted`, `pages_deleted`, `auto_improve_runs_deleted`, `removed_paths`, `collateral_observations_deleted`, `collateral_handoffs_denulled`, `files_deleted`, `files_failed`, `compacted`, `pre_checkpoint`, `checkpoint` |
| Expirar handoffs | `POST /admin/handoffs/expire` | `workspace`, `project`, `confirm`, `dry_run`, `older_than_days` (opcional; 422 se `0`) | `{workspace, project, expired, dry_run, older_than_days}` (escopado pelo filtro de proprietário do chamador; grava `audit_log` `expire_open_handoffs` quando `expired > 0` confirmado) |
| Renomear projeto | `POST /admin/rename-project` | `workspace`, `from` **ou** `project_id`, `to` | `{workspace, from, to, pages, checkpoint, manifest_warning}` (200 no-op se `from == to`; 409 se destino já existe; 422 se `_profile`/`_profile.*` ou inválido) |
| Renomear workspace | `POST /admin/rename-workspace` | `from`, `to` | `{from, to, manifests_refreshed, manifest_warning, checkpoint}` (200 no-op se `from == to`; 409 se destino já existe) |
| Apagar workspace | `POST /admin/delete-workspace` | `workspace`, `confirm` (obrigatório quando `dry_run: false`), `force`, `compact`, `dry_run` | `{workspace, projects_deleted, pages_deleted, sessions_deleted, observations_deleted, handoffs_deleted, embeddings_deleted, workstreams_deleted, managed_runs_deleted, workstream_ids, collateral_observations_deleted, collateral_handoffs_denulled, compacted, files_deleted, files_failed, pre_checkpoint, checkpoint, dry_run}` (409 com `projects[]` se não vazio sem `force: true`, inclusive em `dry_run: true`) |
| Mover projeto | `POST /admin/move-project` | `from_workspace`, `project` **ou** `project_id`, `to_workspace`, `confirm`, `force`, `on_conflict` (`block\|overwrite\|duplicate`) | `{from_workspace, to_workspace, project, moved_via, pages_copied, pages_skipped, source_purged, conflicts, workstreams_moved, pre_checkpoint, checkpoint, manifest_warning}` |
| Fundir workspace | `POST /admin/merge-workspace` | `from`, `to`, `confirm`, `force`, `on_conflict` | `{from, to, projects_merged[<relatório de mover projeto>], source_workspace_deleted}` (422 se `from == to`; filtra `_profile` e `_profile.*`; falha parcial aborta retornando erro com `failed_project`, `reason`, `projects_merged`) |
| Compactar | `POST /admin/compact` | `{}` (na CLI exige `--confirm`) | `{bytes_before, bytes_after, bytes_reclaimed}` |
| Mover sessão | `POST /admin/move-session` | `session_id` **ou** `session_ids[]`, `to_workspace` (padrão `"default"`), `to_project`, `create`, `pages` (`move\|keep\|regenerate`), `confirm` | único: `{dry_run, session_id, source_scopes, to_workspace, to_project, created_project, observations_moved, handoffs_moved, pages_moved, page_regenerated, page_conflict, pre_checkpoint, checkpoint}`; lote: `{dry_run, to_workspace, to_project, total, moved, sessions[]}` |
| Checkpoints | `GET /admin/checkpoints` | `limit` 1..100 (padrão 20) | `{checkpoints: [{id, short_id, timestamp, message, author}]}` (mais novo primeiro) |
| Restaurar página | `POST /admin/restore-page` | `workspace`, `project`, `path`, `rev` (na CLI `--from <rev>`) | `{workspace, project, path, restored_from, checkpoint}` (valida `path` 422 antes de buscar escopo 404) |
| Commit | `POST /admin/commit` | `message` (opcional) | `{checkpoint, committed}` (`checkpoint: null, committed: false` se árvore limpa) |

Operações diretas em disco, só na CLI: `restore --from <tarball> [--force]`, `reset [--confirm]`, `uninstall [--agent claude-code] [--purge-data | --keep-data] [--yes]`.

## 3. Decisões Arquiteturais e Justificativas

- **Decisão (R-01): uma rota por operação, seguindo a ordem do doc passo a passo; CLI como cliente fino.** Cada handler é uma função linear com os passos numerados do `lifecycle-ops.md` (prévia, confirmação, escopo, admissão, checkpoint, guardas, banco, arquivos, efeitos). Sem `--confirm`, a CLI pede a prévia com tempo máximo de 5 s e sai com erro; a recusa não depende da prévia. Para `move-session --from-project`, a CLI primeiro resolve a lista de IDs via `GET /admin/sessions` e envia `session_ids[]` para `POST /admin/move-session`.
  - **Justificativa:** o doc é procedimental e a ordem é parte do contrato (por exemplo, em `purge-project` e `delete-workspace`, as guardas de lease vivo e workspace não vazio rodam mesmo em `dry_run: true`, retornando 409 sem `force: true`).
  - **Alternativa Rejeitada:** um "motor de operações" genérico com passos plugáveis (abstração preemptiva, proibida).

- **Decisão (R-02): o passo de admissão fica fora até a 008, sem ponto de extensão vazio.** Os handlers deixam a posição documentada num comentário com o nome da operação de admissão do doc (`purge_project`, `purge_session`, `purge_workspace`, `move_project`, `move_session`); a 008 insere a chamada ali.
  - **Justificativa:** a constituição proíbe stub e abstração sem chamador.
  - **Alternativa Rejeitada:** interface `Admission` com implementação vazia.

- **Decisão (R-03): exclusão numa transação com tombstone e auditoria; arquivos depois.** A transação apaga as linhas (incluindo `profile_candidates` e `ingest_keys` em `purge-project` e `purge-session`), grava `purged_sessions` ou `purged_scopes` e a linha de `audit_log` (exceto em `delete-workspace`, que delega cada projeto sem gravar entrada própria de `audit_log` nem checar `managed_runs`). A remoção de arquivos vem depois e não desfaz o banco; cada caminho vai para `files_deleted` ou `files_failed`. A purga de sessão segura a trava exclusiva de mutação da wiki (R-05) do início da transação até o fim da limpeza; em modo `dry_run: true`, executa apenas consultas `COUNT(*)` de leitura sem abrir transação de escrita.
  - **Justificativa:** `docs/lifecycle-ops.md` § "Concurrency and tombstones"; INV-02 e INV-03.
  - **Alternativa Rejeitada:** apagar arquivos antes (uma falha no banco deixaria arquivo apagado com linha viva).

- **Decisão (R-04): tombstones consultados em três pontos.** (a) Ingestão do hook (002/T028): evento de sessão em `purged_sessions` é descartado com 202, e a resolução de projeto não recria escopo em `purged_scopes`; (b) `reindex` e reconciliação pulam diretório de escopo tombstoned e não tocam nos arquivos; (c) watcher ignora `sessions/<id>.md` de sessão purgada, e se a consulta falhar processa o evento mesmo assim.
  - **Justificativa:** `docs/storage-and-retrieval.md` §10 e `docs/wiki-files-layer.md` §8.1.
  - **Alternativa Rejeitada:** apagar o diretório da wiki como única proteção (o reindex e um hook atrasado recriariam o escopo).

- **Decisão (R-05): trava exclusiva de mutação da wiki.** A wiki ganha uma trava leitor-escritor: escrita de página, reindex de arquivo pelo watcher e restauração de página pegam o lado de leitura; purga de sessão, remoção de árvore de escopo e movimentos pegam o lado exclusivo. A ordem de aquisição é sempre trava global antes das travas por página da 001.
  - **Justificativa:** o doc exige que, durante a purga de sessão, escritas e reindex em voo terminem antes e as novas esperem.
  - **Alternativa Rejeitada:** só as travas por página (não serializam operações que tocam muitos caminhos).

- **Decisão (R-06): purga de sessão apaga páginas no escopo da sessão.** Resolve `(workspace_id, project_id)` diretamente pela linha de `sessions`; coleta a página-resumo (`sessions.summary_page_id`), todas as linhas de `pages` naquele escopo com `path = 'sessions/<session_id>.md'` e quaisquer outras páginas naquele escopo cujo frontmatter traga `session_id` igual ao da sessão. Nunca apaga páginas de outro projeto.
  - **Justificativa:** garante isolamento estrito por escopo ao mesmo tempo que limpa todas as versões de `sessions/<session_id>.md` do escopo da sessão.
  - **Alternativa Rejeitada:** apagar por caminho global sem filtrar por `(workspace_id, project_id)`.

- **Decisão (R-07): movimento real (`true-move`) com banco primeiro e disco em seguida, sob trava exclusiva.** Sob a trava exclusiva de mutação da wiki: primeiro executa a transação no banco trocando o `workspace_id` em `projects`, `pages`, `sessions`, `observations`, `handoffs`, `audit_log`, `project_grants`, tabelas de auto-melhoria, `session_consolidation_jobs` e `workstreams` e gravando o `audit_log` de `move_project`; em seguida, renomeia o diretório `wiki/<from_ws>/<proj_id>` para `wiki/<to_ws>/<proj_id>`. Se a renomeação em disco falhar, reverte o `workspace_id` no banco para `from_ws.id` e apaga a entrada de `audit_log` (e se a reversão falhar, o erro 500 detalha o reparo manual).
  - **Justificativa:** `docs/lifecycle-ops.md` § true move. O id do projeto não muda, então links e embeddings seguem.
  - **Alternativa Rejeitada:** copiar e purgar sempre (perde sessões, observações e o histórico de supersessão).

- **Decisão (R-08): fusão por cópia e purga (`copy-purge`) com política de conflito.** Lista as versões mais recentes da origem; compara com a página no mesmo caminho do destino por corpo, frontmatter (ignorando `generated.at`), título, tier e `pinned`; aplica `block`, `overwrite` ou `duplicate` (`<stem>-from-<slug>.<ext>`, depois `-1`, `-2`, ..., até `-9999`); copia pelo `wiki.WritePage` (caminho normal, com sanitização e índice); só purga a origem se todas as páginas copiaram (`pages_skipped` vazio); se `pages_skipped` não for vazio, mantém a origem (`source_purged: false`) e faz commit `"move-project-partial <from_ws>/<proj> -> <to_ws>/<proj>"`. Execução gerenciada viva na origem dá 409, e `--force` não a sobrepõe.
  - **Justificativa:** `docs/lifecycle-ops.md` § copy-then-purge merge.
  - **Alternativa Rejeitada:** fundir por `INSERT ... SELECT` no banco (pularia sanitização, índice e wiki).

- **Decisão (R-09): `move-session` com simulação e conflito não-fatal de página.** Sem `--confirm`, calcula o relatório de simulação sem alterar estado; na execução confirmada, cria o destino quando `create: true`, atualiza `sessions`, `observations`, `handoffs` (`source_session_id`), `profile_candidates` (`source_project_id`) e `ingest_keys` (`UPDATE OR IGNORE` + `DELETE` das sobras) e move/regenera a página conforme `pages` (`move`, `keep` ou `regenerate`). Quando `pages == "move"` e `sessions/<id>.md` já existe no destino, não falha com 409: define `page_conflict = true` (`pages_moved = 0`), mantém o arquivo na origem e conclui o re-stamp no banco.
  - **Justificativa:** `docs/lifecycle-ops.md` § move-session.
  - **Alternativa Rejeitada:** abortar a movimentação de banco quando o arquivo Markdown de destino já existe.

- **Decisão (R-10): compactação no escritor.** Depois do commit da exclusão (ou via `POST /admin/compact`), um comando do escritor único executa `INSERT INTO <fts>(<fts>) VALUES('rebuild')` para `observations_fts`, `pages_fts` e `workstream_events_fts` e depois `VACUUM`. Escritas esperam na fila do escritor durante a operação.
  - **Justificativa:** o doc pede reconstruir os três índices sempre e reescrever o arquivo; o escritor único dá a exclusividade.
  - **Alternativa Rejeitada:** conexão paralela para o `VACUUM` (concorre com o escritor).

- **Decisão (R-11): `restore` com estágio, validação, fechamento explícito de handle no Windows e troca reversível.** Extrai no diretório `.restore-staging-<ts>-<pid>` dentro do diretório de dados, aceitando entradas tar regulares (`'0'`, `'\0'`), diretórios (`'5'`) e GNU Sparse (`'S'`), restritas à allowlist `config.toml`, `db`, `db/memory.sqlite`, `wiki` e `wiki/...` (recusa caminho absoluto, `..`, links e outros tipos); abre o banco extraído, roda as migrações e **fecha explicitamente a conexão SQLite antes de renomear qualquer diretório** (essencial no Windows para evitar `ERROR_ACCESS_DENIED`); move `wiki/` e `db/` existentes incondicionalmente para `.restore-previous-<ts>-<pid>`, promove o staging para `wiki/`, `db/` e `config.toml` por renomeação (desfazendo em falha) e por fim remove `.restore-previous-*` (tratando falha na limpeza final apenas como aviso em stderr). `reset` remove e recria `wiki/`, `db/` e `raw/`; `config.toml`, `logs/` e `models/` ficam. `uninstall` aceita `--purge-data` ou `--keep-data` (mutuamente exclusivos) e aplica `refuse_ambient_data_dir`. Os três comandos usam a guarda de processo vivo da 002 (`reset` também na simulação).
  - **Justificativa:** `docs/lifecycle-ops.md` § restore e § reset, `docs/cli.md` §9.1 e `docs/windows.md`.
  - **Alternativa Rejeitada:** extrair direto sobre os dados atuais ou manter a conexão de validação aberta durante o `rename`.

- **Decisão (R-12): contratos de teste.** Um teste por modo de falha listado no doc para cada operação; prévia e execução comparadas campo a campo; tombstone exercitado com hook atrasado, reindex e watcher (multi-sessão, `-race`); purga com mesmo caminho em dois projetos; fusão com as três políticas; `restore` com tarball malicioso (caminho absoluto, `..`, link).
  - **Justificativa:** constituição (teste multi-sessão nas mudanças de escopo e de escritor) e `docs/security-boundaries.md` §10.
  - **Alternativa Rejeitada:** só o caminho feliz de cada comando.

### O que não construir nesta feature
Webhooks de admissão (008), `reorg` e manutenção (004), vínculo de checkout local e sessões nativas no `rename-project` (010 e 013), `backup-agents` e `restore-agents` (010), atribuição de auditoria por usuário (007).

## 4. Auditoria de Constituição

**Portões de Qualidade**
- [ ] Build cruzado e `go vet` — só stdlib nova.
- [ ] `go test -race ./...` — R-03, R-04, R-05 e R-12.
- [ ] Teste multi-sessão em escopo, escritor e supersessão — R-04 (tombstone contra hook e watcher), R-07 e R-08.
- [ ] Invariantes com teste nomeado — INV-09 (recusa com processo vivo) e INV-16 (supersessão preservada no movimento real) ganham casos novos.
- [ ] Gherkin — os cenários da spec viram o `.feature` da 003.
- [ ] Dependência nova — nenhuma.

**Restrições Arquiteturais**
- [ ] Grafo de imports — sem mudança (R-04 usa `storage` a partir de `ingest` e `wiki`).
- [ ] INV-01 — sem leitura nova de ambiente.
- [ ] INV-02 e INV-03 — exclusões, tombstones, auditoria e re-stamp numa transação do escritor (R-03, R-07); compactação no escritor (R-10).
- [ ] INV-04 — re-stamp mantém `(workspace_id, project_id)` coerente; triggers de pareamento conferem.
- [ ] INV-06 — páginas copiadas na fusão passam pelo `WritePage` (R-08).
- [ ] INV-09 — `restore`, `reset` e `uninstall --purge-data` com a guarda de processo vivo (R-11).
- [ ] INV-10 — restauração de página e cópia na fusão usam a escrita atômica; troca do `restore` por renomeação.
- [ ] INV-16 — movimento real preserva a cadeia de supersessão; purga de sessão mantém handoffs só aceitos.
- [ ] Resolução de escopo — todas as rotas usam busca sem criação e falham fechado com escopo explícito inexistente; só `--create` do `move-session` e o destino do movimento criam escopo.
- [ ] Mutações da wiki só pela camada `wiki` — R-05 a R-09.

**Regras de Negócio Transversais**
- [ ] Rollback do banco quando o disco falha no `true-move` — R-07 (banco primeiro, revertido se a renomeação do diretório falhar).
- [ ] Tombamento desligado por padrão — inalterado; purga é outra coisa (tombstone de escopo e sessão).

**Anti-Padrões Proibidos**
- [ ] Sem criar escopo em caminho destrutivo — R-01.
- [ ] Sem stub — R-02.
- [ ] Sem `panic` — falhas viram os códigos do doc.
- [ ] Goroutine com `context` — nenhuma goroutine nova fora do escritor.
