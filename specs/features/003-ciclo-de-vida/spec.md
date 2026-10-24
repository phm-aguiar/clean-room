# Spec: Ciclo de vida dos dados (purga, renomear, mover, restaurar, reset, checkpoints)

## Metadados
- **ID:** 003
- **Status:** draft (revisada contra os docs em 2026-10-13; nova revisão prevista ao fim da 002, com o que a 001 e a 002 entregarem)
- **Autor:** phm-aguiar
- **Data:** 2026-10-13
- **Feature Anterior:** 002 (servidor, rotas administrativas, `.serve.lock`, guarda de processo vivo, escritas rastreadas no git, ponteiro de projeto ativo)
- **Roadmap:** `specs/features/roadmap.md`
- **Aprovado:** false

## Propósito
> Para quem, o quê e por quê.

O operador precisa corrigir e limpar a memória sem editar o banco à mão: apagar um projeto ou uma sessão capturada no lugar errado, renomear ou mover projetos e sessões, restaurar uma página de um checkpoint git, restaurar um backup e recomeçar do zero. Cada operação tem regra de segurança própria: prévia (dry-run), confirmação explícita, escopo explícito que nunca é criado na leitura, checagem de processo vivo nas operações diretas em disco, checkpoint git antes e depois, e tombstone para que um evento atrasado não ressuscite o que foi apagado.

Importa agora porque, com a captura automática da 002 ligada, sessões caem em projetos errados e projetos de teste se acumulam. Sem estas operações, a única saída é `reset`.

## Escopo

### Dentro do escopo
- **Operações de servidor** (rotas `/admin/*`; a CLI é cliente fino) (`docs/lifecycle-ops.md`, `docs/api-reference.md` §3):
  - `purge-project` (`--workspace`, `--project` ou `--project-id <uuid>`, `--confirm`, `--force`, `--compact`; na rota HTTP: `workspace`, `project`, `project_id`, `confirm`, `dry_run`, `force`, `compact`);
  - `purge-session` (`--session-id` UUID completo, `--workspace` e `--project` opcionais na CLI; na rota HTTP `POST /admin/purge-session` o corpo recebe `{session_id, confirm, compact, dry_run}` e resolve o escopo direto pela linha da sessão em `sessions`);
  - `handoffs --expire-all [--older-than-days <N>] --confirm` (`POST /admin/handoffs/expire` com `workspace`, `project`, `confirm`, `dry_run`, `older_than_days`);
  - `rename-project` (`--workspace`, `--from` ou `--project-id <uuid>`, `--to`) e `POST /admin/rename-workspace` (`from`, `to`);
  - `POST /admin/delete-workspace` (`workspace`, `confirm` obrigatório quando `dry_run: false`, `force`, `compact`, `dry_run`);
  - `move-project` (`--from-workspace`, `--project` ou `--project-id <uuid>`, `--to-workspace`, `--confirm`, `--force`, `--on-conflict` `block|overwrite|duplicate`), com movimento real (`moved_via: "true-move"`) e fusão por cópia e purga (`moved_via: "copy-purge"`);
  - `POST /admin/merge-workspace` (`from`, `to`, `confirm` obrigatório, `force`, `on_conflict`): move cada projeto elegível (excluindo `_profile` e `_profile.*`) do workspace de origem e apaga o workspace vazio, respondendo `{from, to, projects_merged[], source_workspace_deleted}` (`docs/api-reference.md` §3.5);
  - `POST /admin/compact` (corpo JSON `{}` aceito na rota HTTP; na CLI exige `--confirm`): compactação isolada do armazenamento, respondendo `{bytes_before, bytes_after, bytes_reclaimed}` (`docs/api-reference.md` §3.4);
  - `move-session` (id posicional ou `--from-project`, `--to`, `--to-workspace`, `--pages` `move|keep|regenerate`, `--confirm`, `--create`; na rota HTTP aceita `session_id` único ou `session_ids[]` em lote após resolução prévia via `GET /admin/sessions`, `to_workspace`, `to_project`, `create`, `pages`, `confirm`), simulação por padrão;
  - `checkpoints` (`GET /admin/checkpoints`, `limit` de 1 a 100, padrão 20; `--json`), `restore-page` (`--path`, `--from <rev>` na CLI; campo JSON `rev` em `POST /admin/restore-page`), `commit` (`POST /admin/commit` com `message` opcional).
- **Operações diretas em disco** (sem servidor; recusam com outro processo `ai-memory` vivo): `restore --from <tarball> [--force]`, `reset [--confirm]` e `uninstall [--agent claude-code] [--purge-data | --keep-data] [--yes]` (`docs/lifecycle-ops.md`, `docs/cli.md` §8 e §9.1).
- **Compactação** (`--compact`, `compact: true`): depois do commit da exclusão, reconstrói os três índices FTS (observações, páginas, eventos de workstream) e reescreve o arquivo do banco (`VACUUM`), medindo `bytes_before`, `bytes_after` e `bytes_reclaimed`.
- **Tombstones de purga** (`purged_sessions`, `purged_scopes`, já na V01), gravados na mesma transação da exclusão: o reindex pula escopo tombstoned e deixa os arquivos; o watcher não reindexa a página de sessão purgada; a ingestão de hook não recria sessão nem escopo purgado (`docs/storage-and-retrieval.md` §10, `docs/wiki-files-layer.md` §8.1).
- **Efeitos depois da operação:** limpar o ponteiro de projeto ativo que apontava para o escopo removido ou retargetá-lo no movimento/renomeação, invalidar o cache de resolução de escopo do hook, reescrever os manifestos `_meta.md` em renomeação e movimento.
- **Checkpoint de linha de base** `upgrade baseline` no boot de um diretório de dados cuja wiki já tem páginas rastreadas mas nenhum commit git (`docs/lifecycle-ops.md` § checkpoints).

### Fora do escopo (explicitamente)
- Webhooks de admissão das purgas e movimentos (`purge_project`, `purge_session`, `purge_workspace`, `move_project`, `move_session`): o passo fica no lugar certo da ordem, sem webhook, até a feature 008.
- `reorg` (rederivar o projeto de todas as sessões pelo diretório), varredura de esquecimento e compactação de tier (feature 004).
- Atualização do vínculo de checkout local em `rename-project` e o efeito em sessões nativas de workstream (features 010 e 013).
- `backup-agents` e `restore-agents` (feature 010).
- Atribuição da auditoria a usuário autenticado (feature 007); na 003 a auditoria registra o ator local (`local`).

## Comportamento Esperado

### Cenário Principal (Happy Path)
1. O operador roda `ai-memory purge-session --session-id <uuid>` sem `--confirm`: a CLI pede a prévia ao servidor (`dry_run: true`, timeout de 5 s), mostra as contagens e sai com erro pedindo `--confirm`. Nada é apagado.
2. Com `--confirm`, o servidor localiza a sessão por `session_id` (404 se não existir), faz checkpoint se a árvore estiver suja, adquire a trava exclusiva de mutação da wiki e, numa transação única do escritor, coleta e apaga a página-resumo (`summary_page_id`), todas as linhas de `pages` no escopo da sessão com `path = 'sessions/<session_id>.md'` e quaisquer outras páginas no escopo cujo frontmatter traga `session_id`, além de `observations`, `handoffs` originados na sessão, `profile_candidates` (`source_kind = 'prompt' AND source_ref = 'session:<id>'`), `ingest_keys` e a linha de `sessions`, grava o tombstone em `purged_sessions` e a entrada em `audit_log`, remove os arquivos em disco ainda sob a trava exclusiva e faz o checkpoint final.
3. Um evento atrasado do hook dessa sessão chega: não recria a sessão (descartado com 202).
4. `ai-memory checkpoints` lista os commits; `restore-page --path <caminho> --from <rev>` valida o caminho antes do escopo, volta a página ao estado daquele checkpoint (`rev`) e a reindexa.

### Cenários Alternativos
- **Sem `--confirm` (nas rotas destrutivas, incluindo `delete-workspace`, `purge-project`, `purge-session`, `handoffs/expire`, `move-project`, `merge-workspace`, `move-session`):** 400 (servidor) quando `dry_run: false` e `confirm: false`, e nada muda; na CLI sem `--confirm` para purgas, mostra a prévia antes e sai com código != 0.
- **Conflito de identificadores (`project` e `project_id` passados juntos ou ambos ausentes em `purge-project`, `rename-project`, `move-project`):** 422 Unprocessable Entity.
- **Escopo ou sessão inexistente:** 404, nada criado nem apagado.
- **Checkpoint prévio falhou:** 500, nada apagado.
- **Remoção de arquivo falhou depois do commit:** 200 com `files_failed` preenchido; o banco já foi apagado; uma segunda chamada devolve 404.
- **`purge-project` com execução gerenciada de lease vivo e sem `--force`:** 409 Conflict (corpo `{"error": "project has <n> active managed run(s) in workstream(s) …"}`, **sem** campo `active_workstreams`; correção da auditoria 008), **inclusive em `dry_run: true`**; lease vencido não bloqueia.
- **`delete-workspace` em workspace não vazio sem `force: true`:** 409 Conflict com a lista `projects`, **inclusive em `dry_run: true`**; sem `confirm: true` quando `dry_run: false`: 400 Bad Request.
- **`move-project` para o mesmo workspace:** 422; projeto ativo do hook sem `--force`: 409; execução gerenciada viva na origem em modo fusão: 409 mesmo com `--force`.
- **Fusão (`copy-purge`) com conflito e política `block`:** 409 com a lista `conflicts`, nada copiado; sob `duplicate`, sufixos colidentes iniciam em `<stem>-from-<slug>.<ext>` e depois `-1`, `-2`, ... até `-9999`; página ilegível: 200 com `source_purged: false`, `pages_skipped` e commit git `"move-project-partial <from_ws>/<proj> -> <to_ws>/<proj>"`.
- **`merge-workspace` com `from == to`:** 422; falha no meio da migração de um projeto aborta imediatamente retornando erro HTTP com `{error, failed_project, reason, projects_merged, source_workspace_deleted: false}`.
- **`move-session` com destino inexistente e `create: false`:** 404; quando `pages == "move"` e `sessions/<id>.md` já existe no destino, **não** retorna 409 — marca `page_conflict = true` (`pages_moved = 0`), preserva o arquivo de origem e conclui o re-stamp no banco com 200 OK.
- **`restore` com processo vivo, tarball ausente, diretório ocupado sem `--force`, arquivo inválido (entradas fora da allowlist `config.toml`, `db`, `db/memory.sqlite`, `wiki`, `wiki/...`) ou banco que não abre:** recusa e os dados atuais ficam intactos; no Windows, fecha explicitamente a conexão SQLite de validação do diretório de staging antes de renomear diretórios; falha na troca: desfaz os movimentos; falha ao apagar `.restore-previous-*` após a troca: apenas aviso em stderr (retorna 0).
- **`reset` sem `--confirm`:** lista o que removeria e sai com sucesso (0); com processo vivo recusa, inclusive na simulação.
- **`rename-project` / `rename-workspace`:** quando `from == to`, retorna 200 OK como no-op (`checkpoint: null`); com nome vazio, com `/`, controles ou reservado (`_profile` / `_profile.*` em projeto): 422; quando o nome de destino (`to`) já existe: **409 Conflict**.

## Edge Cases
- **Mesmo UUID de sessão ou caminho manual:** a purga de sessão opera estritamente dentro do `(workspace_id, project_id)` da sessão, removendo `summary_page_id`, todas as linhas de `pages` daquele escopo com `path = 'sessions/<session_id>.md'` e páginas daquele escopo com frontmatter `session_id` igual, sem jamais tocar em páginas de outro projeto.
- **Dano colateral:** observações que a sessão gravou em outro projeto são apagadas junto (`collateral_observations_deleted`); handoffs de outros projetos que ela criou ou aceitou mantêm o texto e perdem o vínculo (`collateral_handoffs_denulled`). Em `purge-project` e `purge-session`, candidatos de perfil (`profile_candidates`) e chaves de ingestão (`ingest_keys`) associados também são removidos na mesma transação.
- **Handoffs que a sessão só aceitou:** mantidos (`accepted_by_session_id` vira `NULL`).
- **`move-session` espalhada:** dependentes (`observations`, `handoffs`, `profile_candidates`, `ingest_keys` com `UPDATE OR IGNORE` seguido de `DELETE` de sobras) são achados pelo id da sessão e vêm de todos os escopos; o relatório lista `source_scopes`. O movimento não é reversível de forma limpa.
- **Movimento real (`true-move`) transacional:** sob a trava exclusiva de mutação da wiki, atualiza o banco SQLite **primeiro** (trocando `workspace_id` em todas as tabelas de projeto e `audit_log`) e **depois** renomeia o diretório `wiki/<from_ws>/<proj_id>` para `wiki/<to_ws>/<proj_id>`. Se a renomeação em disco falhar, reverte o `workspace_id` no banco e apaga o registro de `audit_log`; se a reversão também falhar, o erro 500 detalha o reparo manual.
- **Reindex depois de purga de projeto:** o diretório do escopo tombstoned é pulado e os arquivos ficam como estão; reindex nunca apaga arquivo.
- **Compactação:** não é apagamento forense; o histórico git e os backups anteriores mantêm o conteúdo.
- **Handles abertos no Windows durante `restore`:** o SQLite mantém lock de sistema operacional no arquivo `db/memory.sqlite` e nos arquivos `-wal`/`-shm` enquanto a conexão estiver aberta; a conexão de validação no staging deve ser explicitamente fechada antes do `rename` do diretório de staging para `db/`, evitando `ERROR_ACCESS_DENIED`.

## Constraints
- **Performance:** prévia da CLI com tempo máximo de 5 s; compactação segura a trava exclusiva do banco durante toda a reescrita e precisa de espaço livre perto do tamanho do banco.
- **Tecnologia:** só stdlib e as dependências já em `tech.md`; tarball com `archive/tar` e `compress/gzip` (aceitando entradas regulares `'0'`/`'\0'`, diretórios `'5'` e GNU Sparse `'S'`).
- **Segurança/Compliance:** operações destrutivas exigem escopo explícito e confirmação explícita; leitura de escopo nunca cria; extração do tarball aceita estritamente `config.toml`, `db`, `db/memory.sqlite`, `wiki` e `wiki/...`, e recusa caminho absoluto, travessia (`..`) e links (`docs/security-boundaries.md` §10, `docs/lifecycle-ops.md`); `uninstall --purge-data` recusa diretórios de sistema, `$HOME` ou caminhos com menos de 2 componentes normais.
- **Concorrência:** a purga de sessão segura a trava exclusiva de mutação da wiki do início da exclusão no banco até o fim da limpeza de arquivos; a exclusão no banco é uma transação no escritor único; em `purge-Session` com `dry_run: true`, apenas consultas `COUNT(*)` somente-leitura são executadas sem abrir transação de escrita.

## Critérios de Sucesso
- [ ] Cada comando e rota da matriz de segurança de `docs/lifecycle-ops.md` (menos os de outras features) responde com os códigos de status e os campos de relatório do doc
- [ ] Prévia e simulação devolvem as mesmas contagens que a execução confirmada e não deixam linha de auditoria nem arquivo alterado
- [ ] Tombstones: depois de uma purga, um evento atrasado do hook não recria sessão nem escopo, o reindex pula o escopo e o watcher não traz a página de volta (teste multi-sessão)
- [ ] `move-project` real preserva o id do projeto e toda a cadeia de supersessão; fusão respeita `block`, `overwrite` e `duplicate`
- [ ] `restore` e `reset` recusam com outro processo `ai-memory` vivo e deixam os dados atuais intactos em qualquer falha antes da troca
- [ ] `scripts/check.sh` e o lint passam sem apontamentos

## Abordagem Escolhida
Rotas administrativas no servidor da 002, uma por operação, cada uma seguindo a ordem do doc passo a passo, com a CLI como cliente fino. As operações diretas em disco ficam na CLI e usam a guarda de processo vivo da 002. As exclusões são uma transação no escritor único, com tombstone e auditoria juntos. O passo de admissão fica marcado na ordem, sem efeito, até a 008.

## Dependências
- 002: servidor HTTP e `/admin/*` (T039), `.serve.lock` e guarda de processo vivo (T040, T041), escritas rastreadas e commit no git (T080), ponteiro de projeto ativo (T019) e cache de resolução do hook (T027), backup em tarball (T081).
- `docs/lifecycle-ops.md`, `docs/storage-and-retrieval.md` §10, `docs/wiki-files-layer.md` §8, `docs/security-boundaries.md` §10, `docs/cli.md` §8 e §9.

## Checklist de Prontidão
- [x] Propósito claro e sem ambiguidade
- [x] Escopo delimitado (dentro/fora)
- [x] Cenários cobrem happy path e alternativos
- [x] Edge cases identificados com comportamento esperado
- [x] Constraints explícitas
- [x] Critérios de sucesso mensuráveis
- [x] Abordagem escolhida justificada
