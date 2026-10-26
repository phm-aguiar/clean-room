# Spec: Ciclo de vida dos dados (purga, renomear, mover, restaurar, reset, checkpoints)

## Metadados
- **ID:** 003
- **Status:** spec auditada contra o original (ver "Correções da auditoria" e os relatórios em `reports/`); sem plano nem tasks; em toda dúvida, reproduzir o original
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
  - `purge-session` (`--session-id` UUID completo, `--workspace` e `--project` opcionais na CLI; na rota HTTP `POST /admin/purge-session` o corpo recebe `{workspace, project, session_id, confirm, compact, dry_run}` (nomes obrigatórios); a sessão precisa pertencer a esse escopo, senão 404. Correção da auditoria 011: a rota não resolve o escopo pela linha da sessão);
  - `handoffs --expire-all [--older-than-days <N>] --confirm` (`POST /admin/handoffs/expire` com `workspace`, `project`, `confirm`, `dry_run`, `older_than_days`);
  - `rename-project` (`--workspace`, `--from` ou `--project-id <uuid>`, `--to`) e `POST /admin/rename-workspace` (`from`, `to`);
  - `POST /admin/delete-workspace` (`workspace`, `force`, `compact`, `dry_run`; sem `confirm`);
  - `move-project` (`--from-workspace`, `--project` ou `--project-id <uuid>`, `--to-workspace`, `--confirm`, `--force`, `--on-conflict` `block|overwrite|duplicate`), com movimento real (`moved_via: "true-move"`) e fusão por cópia e purga (`moved_via: "copy-purge"`);
  - `POST /admin/merge-workspace` (`from`, `to`, `confirm` obrigatório, `force`, `on_conflict`): move cada projeto do workspace de origem (projetos `_profile.*` não são filtrados: o movimento de um deles falha com 422 e a fusão para; ver "Correções da auditoria") e apaga o workspace vazio, respondendo `{from, to, projects_merged[], source_workspace_deleted}` (`docs/api-reference.md` §3.5);
  - `POST /admin/compact` (corpo `{confirm: true}` obrigatório; sem ele 400; a CLI também exige `--confirm` e recusa localmente): compactação isolada do armazenamento, respondendo `{bytes_before, bytes_after, bytes_reclaimed}` (`docs/api-reference.md` §3.4);
  - `move-session` (id posicional ou `--from-project`, `--to`, `--to-workspace`, `--pages` `move|regenerate` (não existe `keep`), `--confirm`, `--force`, `--create`; na rota HTTP: `session_id` (único) **ou** `from_project` + `from_workspace` (lote, todas as sessões que tocam o escopo), destino `workspace`/`project`, `pages`, `confirm`, `force`, `create`), simulação por padrão (o movimento roda numa transação revertida);
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
- **Sem `--confirm` (nas rotas destrutivas `purge-project`, `purge-session`, `handoffs/expire`, `move-project`, `merge-workspace`, `move-session`):** 400 (servidor) quando `dry_run: false` e `confirm: false`, e nada muda; na CLI sem `--confirm` para purgas, mostra a prévia antes e sai com código != 0.
- **Conflito de identificadores (`project` e `project_id` passados juntos ou ambos ausentes em `purge-project`, `rename-project`, `move-project`):** 400 Bad Request (`project and project_id are mutually exclusive` / `either project or project_id is required`; UUID inválido: 400 `invalid project id: …`). Correção da auditoria 011: não é 422.
- **Escopo ou sessão inexistente:** 404, nada criado nem apagado.
- **Checkpoint prévio falhou:** 500, nada apagado.
- **Remoção de arquivo falhou depois do commit:** 200 com `files_failed` preenchido; o banco já foi apagado; uma segunda chamada devolve 404.
- **`purge-project` com execução gerenciada de lease vivo e sem `--force`:** 409 Conflict (corpo `{"error": "project has <n> active managed run(s) in workstream(s) …"}`, **sem** campo `active_workstreams`; correção da auditoria 008), **inclusive em `dry_run: true`**; lease vencido não bloqueia.
- **`delete-workspace` em workspace não vazio sem `force: true`:** 409 `workspace still has <n> project(s); pass force to delete anyway` (sem lista de projetos), **inclusive em `dry_run: true`**. A rota **não tem** campo `confirm`: sem `dry_run`, apaga (correção da auditoria 011).
- **`move-project` para o mesmo workspace:** 422; projeto ativo do hook sem `--force`: 409; execução gerenciada viva na origem em modo fusão: 409 mesmo com `--force`.
- **Fusão (`copy-purge`) com conflito e política `block`:** 409 com a lista `conflicts`, nada copiado; sob `duplicate`, sufixos colidentes iniciam em `<stem>-from-<slug>.<ext>` e depois `-1`, `-2`, ... sem limite superior (correção da auditoria 011); página ilegível: 200 com `source_purged: false`, `pages_skipped` e commit git `"move-project-partial <from_ws>/<proj> -> <to_ws>/<proj>"`.
- **`merge-workspace` com `from == to`:** 422; falha no meio da migração de um projeto aborta imediatamente retornando erro HTTP com `{error, failed_project, reason, projects_merged, source_workspace_deleted: false}`.
- **`move-session` com destino inexistente e `create: false`:** 404. Com `pages == "move"` e arquivo `sessions/<id>.md` já existente no destino, ou versão atual no destino: **409**, nada muda (correção da auditoria 011: não existe `page_conflict`). Sessão aberta (fora de re-home) ou job de consolidação pendente/em execução: 409, que `force` **ignora**.
- **`restore` com processo vivo, tarball ausente, diretório ocupado sem `--force`, arquivo inválido (entradas fora da allowlist `config.toml`, `db`, `db/memory.sqlite`, `wiki`, `wiki/...`) ou banco que não abre:** recusa e os dados atuais ficam intactos; no Windows, fecha explicitamente a conexão SQLite de validação do diretório de staging antes de renomear diretórios; falha na troca: desfaz os movimentos; falha ao apagar `.restore-previous-*` após a troca: apenas aviso em stderr (retorna 0).
- **`reset` sem `--confirm`:** lista o que removeria e sai com sucesso (0); com processo vivo recusa, inclusive na simulação.
- **`rename-project` / `rename-workspace`:** `from == to` não é caso especial (200; em projeto ainda grava auditoria e reescreve manifestos); nome vazio, com `/` ou reservado (`_profile` / `_profile.*` como destino, ou origem de perfil, em projeto): 422; nome de destino (`to`) já existente: **422** (não 409; correção da auditoria 011). Não há verificação de tamanho nem de caracteres de controle no caminho de renomeação.

## Edge Cases
- **Mesmo UUID de sessão ou caminho manual:** a purga de sessão opera estritamente dentro do `(workspace_id, project_id)` da sessão, removendo `summary_page_id`, todas as linhas de `pages` daquele escopo com `path = 'sessions/<session_id>.md'` e páginas daquele escopo com frontmatter `session_id` igual, sem jamais tocar em páginas de outro projeto.
- **Dano colateral:** observações que a sessão gravou em outro projeto são apagadas junto (`collateral_observations_deleted`); handoffs de outros projetos que ela criou ou aceitou mantêm o texto e perdem o vínculo (`collateral_handoffs_denulled`). Em `purge-session`, os candidatos de perfil com origem `prompt` e referência `session:<id>` são apagados explicitamente (em qualquer projeto). Em `purge-project`, tudo sai pela cascata do projeto (correção da auditoria 011).
- **Handoffs que a sessão só aceitou:** mantidos (`accepted_by_session_id` vira `NULL`).
- **`move-session` espalhada:** dependentes (`observations`, `handoffs`, `profile_candidates`, `ingest_keys` com `UPDATE OR IGNORE` seguido de `DELETE` de sobras) são achados pelo id da sessão e vêm de todos os escopos; o relatório lista `source_scopes`. O movimento não é reversível de forma limpa.
- **Movimento real (`true-move`) transacional:** sob a trava exclusiva de mutação da wiki, roda a admissão, renomeia **primeiro** o diretório `wiki/<from_ws>/<proj_id>` para `wiki/<to_ws>/<proj_id>` e **depois** troca o `workspace_id` numa transação (as tabelas do projeto, `audit_log` e outras; não mexe nos links). Se a transação falhar, renomeia o diretório de volta; se isso também falhar, o erro 500 (`INCONSISTENT STATE: …`) detalha o reparo manual (correção da auditoria 011: a ordem é disco primeiro).
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
- [ ] `go vet ./...`, `go test -race ./...` e `golangci-lint run` passam sem apontamentos

## Abordagem Escolhida
Rotas administrativas no servidor da 002, uma por operação, cada uma seguindo a ordem do doc passo a passo, com a CLI como cliente fino. As operações diretas em disco ficam na CLI e usam a guarda de processo vivo da 002. As exclusões são uma transação no escritor único, com tombstone e auditoria juntos. O passo de admissão fica marcado na ordem, sem efeito, até a 008.

## Dependências
- 002: servidor HTTP e `/admin/*`, `.serve.lock` e guarda de processo vivo, escritas rastreadas e commit no git, ponteiro de projeto ativo e cache de resolução do hook, backup em tarball.
- `docs/lifecycle-ops.md`, `docs/storage-and-retrieval.md` §10, `docs/wiki-files-layer.md` §8, `docs/security-boundaries.md` §10, `docs/cli.md` §8 e §9.

## Checklist de Prontidão
- [x] Propósito claro e sem ambiguidade
- [x] Escopo delimitado (dentro/fora)
- [x] Cenários cobrem happy path e alternativos
- [x] Edge cases identificados com comportamento esperado
- [x] Constraints explícitas
- [x] Critérios de sucesso mensuráveis
- [x] Abordagem escolhida justificada

## Correções da auditoria

Auditoria 011 (2026-10-25), validação linha a linha contra o original. Relatório: `reports/011-auditoria-003-004-005.md`.

- **`POST /admin/compact` exige `confirm: true` no corpo** (antes: "corpo `{}` aceito"). Sem ele: 400 `compact rewrites the whole database under an exclusive lock; pass confirm: true to proceed`. A operação passa pela fila do escritor único do banco, **não** pela trava de mutação da wiki; depois tenta truncar o WAL (falha só gera aviso). Mensagens literais da CLI (`Compacted: …`, `Nothing to reclaim — …`) e formatação de bytes em `docs/lifecycle-ops.md` ("What compaction does").
- **Conflito `project`/`project_id`** (ambos ou nenhum; UUID inválido) em purga, renomeação e movimento de projeto: **400**, não 422.
- **Renomeação de projeto/workspace com nome já usado: 422**, não 409. `from == to` não é caso especial (200; grava auditoria). A validação de nome é só: vazio, `/`, perfil (destino ou origem, só projeto). Não existe limite de 256 bytes nem checagem de controle nesse caminho. Mensagens literais e saída da CLI (`Renamed …`, aviso de manifesto, falha do vínculo local de checkout) em `docs/lifecycle-ops.md`.
- **`reset`:** ordem fixa `wiki/`, `db/`, `raw/`; mensagens `would remove <path>`, `(dry-run; pass --confirm to wipe)`, `removing <path>` / `recreating <path>`; mesma rotina de `uninstall --purge-data`; mensagem literal da guarda de processo.
- **Risco no Windows:** a guarda de processo do original procura o nome exato `ai-memory`; no Windows o processo se chama `ai-memory.exe`. A reimplementação deve comparar com e sem `.exe`, sem diferenciar maiúsculas.
- Resolvido o "a confirmar" sobre a regra de nome de projeto após renomear (estratégias `basename`/`repo-root` e o marcador).
- Removidos de `docs/lifecycle-ops.md` nomes internos de funções que tinham vazado do original.
### Correções da auditoria (rodada 2, pendências fechadas)

- **`purge-project`:** o `confirm` é checado antes do escopo. Na execução real, a checagem de lease vivo roda **dentro** da transação de exclusão, depois da admissão e do checkpoint prévio; um 409 pode vir depois de os webhooks terem sido chamados. A exclusão é uma única deleção da linha do projeto, com cascata. Com `compact`, uma falha na compactação devolve 500 com a exclusão já gravada e sem limpeza de arquivos. O diretório da wiki ausente conta como removido; diretórios brutos ausentes não são listados; os diretórios brutos são removidos fora da trava da wiki.
- **`purge-session`:** o corpo exige `workspace` e `project`. A admissão roda antes da checagem de pertencimento. A seleção de páginas é: o resumo registrado mais as versões de `sessions/<id>.md` cujo frontmatter traz esse `session_id` (edição manual sem a chave sobrevive). O arquivo só é removido se nenhuma versão atual fora das apagadas ocupa o caminho. Ordem de exclusão, lápide com atualização em conflito, mensagens literais, e: não mexe no ponteiro de projeto ativo nem no cache.
- **`move-session`:** formato real da requisição (`from_project` em lote, sem `session_ids`/`to_project`, sem modo `keep`). A simulação é a transação inteira revertida. `force` **ignora** as guardas de sessão aberta e de job. Arquivo ou versão no destino → 409. Lote: 422 para origem igual ao destino e 409 para projeto ativo. O arquivo vai primeiro e o banco depois, com rollback do arquivo. A lápide de purga não é tocada. Formato do relatório e mensagens da CLI documentados.
- **`move-project`:** no movimento real, o disco vai primeiro e o banco depois (o oposto do que estava escrito); os links não são re-carimbados; `pages_copied` conta todas as versões. A checagem de perfil privado roda pelo nome pedido e pelo nome resolvido. A sequência do sufixo de `duplicate` não tem limite. Na fusão, uma falha de cópia deixa no destino as páginas já copiadas.
- **`restore`:** o carimbo dos diretórios temporários é `AAAAMMDD-HHMMSS-<pid>` (UTC). `./` inicial é **rejeitado**. O arquivo **não** precisa conter `db/memory.sqlite`: sem ele, a restauração cria um banco vazio. Lista exata de permissões por tipo de entrada, mensagens literais, limpeza do staging em todos os casos e notas de Windows.
- **`restore-page`:** não passa pelo pipeline normal de escrita (sem admissão, sem sanitizador). Grava os bytes históricos literalmente sob a trava **compartilhada**, indexa e gera o embedding. Os nomes dos checkpoints têm `: ` entre o projeto e o caminho. Saída da CLI documentada.
- **`rename-workspace`:** confirmado no código que a reescrita de manifestos percorre todos os workspaces e projetos, sob a trava exclusiva; `manifests_refreshed` conta só os manifestos alterados.
### Correções da auditoria (rodada 3)

- **`delete-workspace`:** a rota **não tem** `confirm`. Um 409 de workspace não vazio não traz lista de projetos. A lápide é **uma** linha com id de projeto todo zero, sem linhas por projeto. Não há checagem de lease gerenciado nem auditoria. Uma falha de compactação devolve 500 depois do commit.
- **`merge-workspace`:** a ordem é `confirm` (400), depois `from == to` (422), depois origem ausente (404). Os projetos vão em ordem de atualização mais recente. Projetos `_profile.*` **não** são filtrados: o movimento de um deles falha com 422 e a fusão para. Se a exclusão final do workspace vazio falhar, a resposta continua 200, com `source_workspace_deleted: false`.
- **`backup`:** a cópia do banco usa a facilidade de backup online do motor de banco, não `VACUUM INTO`. A árvore da wiki é validada duas vezes. A resposta vai em streaming a partir de um arquivo temporário. O arquivo inclui o repositório git da wiki. Mensagens da CLI documentadas.
- **`ingest_keys` em `purge-session`:** pelo esquema, a coluna de sessão (migração V77) tem cascata na exclusão, então as chaves marcadas com a sessão somem junto com a linha da sessão. Chaves sem marca de sessão ficam.