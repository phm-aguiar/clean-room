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
- **Operações de servidor** (rotas `/admin/*`; a CLI é cliente fino) (`docs/lifecycle-ops.md`):
  - `purge-project` (`--workspace`, `--project`, `--confirm`, `--force`, `--compact`);
  - `purge-session` (`--session-id` UUID completo, `--confirm`, `--compact`);
  - `handoffs --expire-all --confirm` (`POST /admin/handoffs/expire`);
  - `rename-project` (`--from`, `--to`) e `POST /admin/rename-workspace`;
  - `POST /admin/delete-workspace` (`force`, `compact`, `dry_run`; sem `confirm`);
  - `move-project` (`--from-workspace`, `--project`, `--to-workspace`, `--confirm`, `--force`, `--on-conflict` `block|overwrite|duplicate`), com movimento real e fusão por cópia e purga;
  - `POST /admin/merge-workspace` (`from`, `to`, `confirm` obrigatório, `force`, `on_conflict`): move cada projeto do workspace de origem e apaga o workspace vazio, respondendo `{from, to, projects_merged[], source_workspace_deleted}` (`docs/api-reference.md` §3.5);
  - `POST /admin/compact` (`confirm` obrigatório): compactação isolada do armazenamento, respondendo `{bytes_before, bytes_after, bytes_reclaimed}` (`docs/api-reference.md` §3.4);
  - `move-session` (id posicional ou `--from-project`, `--to`, `--to-workspace`, `--pages` `move|regenerate`, `--confirm`, `--force`, `--create`), simulação por padrão;
  - `checkpoints` (`GET /admin/checkpoints`, 1 a 100, padrão 20; `--json`), `restore-page` (`--path`, `--from`), `commit` (`POST /admin/commit`).
- **Operações diretas em disco** (sem servidor; recusam com outro processo `ai-memory` vivo): `restore --from <tarball> --force`, `reset --confirm` e `uninstall --purge-data` (`docs/lifecycle-ops.md`, `docs/cli.md` §8 e §9.1).
- **Compactação** (`--compact`, `compact: true`): depois do commit da exclusão, reconstrói os três índices FTS (observações, páginas, eventos de workstream) e reescreve o arquivo do banco.
- **Tombstones de purga** (`purged_sessions`, `purged_scopes`, já na V01), gravados na mesma transação da exclusão: o reindex pula escopo tombstoned e deixa os arquivos; o watcher não reindexa a página de sessão purgada; a ingestão de hook não recria sessão nem escopo purgado (`docs/storage-and-retrieval.md` §10, `docs/wiki-files-layer.md` §8.1).
- **Efeitos depois da operação:** limpar o ponteiro de projeto ativo que apontava para o escopo removido ou retargetá-lo no movimento, invalidar o cache de resolução de escopo do hook, reescrever os manifestos `_meta.md` em renomeação e movimento.
- **Checkpoint de linha de base** `upgrade baseline` no boot de um diretório de dados cuja wiki não tem commits (`docs/lifecycle-ops.md` § checkpoints).

### Fora do escopo (explicitamente)
- Webhooks de admissão das purgas e movimentos (`purge_project`, `purge_session`, `purge_workspace`, `move_project`, `move_session`): o passo fica no lugar certo da ordem, sem webhook, até a feature 008.
- `reorg` (rederivar o projeto de todas as sessões pelo diretório), varredura de esquecimento e compactação de tier (feature 004).
- Atualização do vínculo de checkout local em `rename-project` e o efeito em sessões nativas de workstream (features 010 e 013).
- `backup-agents` e `restore-agents` (feature 010).
- Atribuição da auditoria a usuário autenticado (feature 007); na 003 a auditoria registra o ator de operador único.

## Comportamento Esperado

### Cenário Principal (Happy Path)
1. O operador roda `ai-memory purge-session --workspace w --project p --session-id <uuid>` sem `--confirm`: a CLI pede a prévia ao servidor (até 5 s), mostra as contagens e sai com erro pedindo `--confirm`. Nada é apagado.
2. Com `--confirm`, o servidor confere o escopo (404 se não existir, sem criar), faz checkpoint se a árvore estiver suja, coleta a página de sessão e as versões de `sessions/<id>.md` marcadas com a sessão, apaga numa transação a sessão, as observações dela, os handoffs que ela criou e essas versões, grava o tombstone e a auditoria, remove os arquivos e faz o checkpoint final.
3. Um evento atrasado do hook dessa sessão chega: não recria a sessão.
4. `ai-memory checkpoints` lista os commits; `restore-page --from <id>` volta uma página ao estado daquele checkpoint e a reindexa.

### Cenários Alternativos
- **Sem `--confirm`:** 400 (servidor) e nada muda; a CLI mostra a prévia antes.
- **Escopo inexistente:** 404, nada criado nem apagado. Sessão fora do workspace e projeto nomeados também é 404, mesmo que o id exista em outro lugar.
- **Checkpoint prévio falhou:** 500, nada apagado.
- **Remoção de arquivo falhou depois do commit:** 200 com `files_failed`; o banco já foi apagado; a segunda chamada devolve 404.
- **`purge-project` com execução gerenciada de lease vivo e sem `--force`:** 409 nomeando os workstreams; lease vencido não bloqueia.
- **`move-project` para o mesmo workspace:** 422; projeto ativo do hook sem `--force`: 409; diretório de destino já existe no movimento real: 409.
- **Fusão com conflito e política `block`:** 409 com a lista `conflicts`, nada copiado; página ilegível: 200 com `source_purged: false` e `pages_skipped`.
- **`move-session` com sessão aberta, job de consolidação pendente ou (lote) projeto de origem ativo:** 409 sem `--force`; destino já tem página `sessions/<id>.md` em modo `move`: 409.
- **`restore` com processo vivo, tarball ausente, diretório ocupado sem `--force`, arquivo inválido ou banco que não abre:** recusa e os dados atuais ficam intactos; falha na troca: desfaz os movimentos.
- **`reset` sem `--confirm`:** lista o que removeria e sai com sucesso; com processo vivo recusa, inclusive na simulação.
- **`rename-project` com nome vazio, com `/`, reservado ou já usado:** 422.

## Edge Cases
- **Mesmo caminho em dois projetos:** a purga de sessão apaga versões de página **por id**, nunca por caminho, para não levar a página `sessions/<uuid>.md` de outro projeto.
- **Página escrita à mão no caminho da sessão:** versões sem marca da sessão são mantidas; se a versão viva é escrita à mão, o arquivo fica.
- **Dano colateral:** observações que a sessão gravou em outro projeto são apagadas junto (`collateral_observations_deleted`); handoffs de outros projetos que ela criou ou aceitou mantêm o texto e perdem o vínculo (`collateral_handoffs_denulled`).
- **Handoffs que a sessão só aceitou:** mantidos.
- **`move-session` espalhada:** dependentes são achados só pelo id da sessão e vêm de todos os escopos; a prévia lista `source_scopes`. O movimento não é reversível de forma limpa.
- **Movimento real com falha no banco:** o diretório volta para o lugar; se o rollback falhar, o erro nomeia o reparo manual.
- **Reindex depois de purga de projeto:** o diretório do escopo tombstoned é pulado e os arquivos ficam como estão; reindex nunca apaga arquivo.
- **Compactação:** não é apagamento forense; o histórico git e os backups anteriores mantêm o conteúdo.

## Constraints
- **Performance:** prévia da CLI com tempo máximo de 5 s; compactação segura a trava exclusiva do banco durante toda a reescrita e precisa de espaço livre perto do tamanho do banco.
- **Tecnologia:** só stdlib e as dependências já em `tech.md`; tarball com `archive/tar` e `compress/gzip`.
- **Segurança/Compliance:** operações destrutivas exigem escopo explícito e confirmação explícita; leitura de escopo nunca cria; extração do tarball aceita só `config.toml`, `db/memory.sqlite` e entradas sob `wiki/`, e recusa caminho absoluto, travessia e links (`docs/security-boundaries.md` §10, `docs/lifecycle-ops.md`).
- **Concorrência:** a purga de sessão segura a trava exclusiva de mutação da wiki do início da exclusão no banco até o fim da limpeza de arquivos; a exclusão no banco é uma transação no escritor único.

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
