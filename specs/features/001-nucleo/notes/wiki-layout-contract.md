# Nota: layout em disco, _meta, index, log e histórico git

Fontes abreviadas: WFL = `docs/wiki-files-layer.md`; OKF = `docs/okf.md`; LOPS = `docs/lifecycle-ops.md`; DV = `docs/domain-vocabulary.md`; SPEC = `specs/features/001-nucleo/spec.md`; CONST = `.github/memory/constitution.md`; ARCH = `docs/architecture.md`; BKP = `docs/backup.md`; LEI = `docs/lifecycle-event-ingestion.md`.

Regra: `docs/wiki-files-layer.md` §1, §1.1, §4, §5, §7, §8 e §9 foram reescritos contra o original e
**vencem** esta nota. Em toda dúvida vale o comportamento do original.

## (a) Árvore de diretórios da wiki por escopo

Árvore canônica (WFL §1):

```text
<data_dir>/wiki/                       ← raiz (um único repositório Git)
  <workspace_id>/
    _meta.md          ← manifesto do workspace
    <project_id>/
      _meta.md        ← manifesto do projeto (nomes humanos ↔ identificadores)
      index.md        ← página comum; só a migração OKF o cria
      log-AAAA-MM.md  ← ledger bruto de eventos de hook (mês UTC; `log.md` só é reconhecido)
      sessions/<session_id>.md
      concepts/ decisions/ gotchas/ procedures/ …   ← páginas por tipo
      _pending/       ← sidecars de propostas (não indexados)
```

- `<data_dir>/wiki/` é a fonte da verdade em markdown. O índice derivado fica em `<data_dir>/db/memory.sqlite` (ARCH). `<data_dir>/raw/` guarda segmentos JSONL de workstreams (fora da 001).
- Os diretórios são por UUID, nunca por nome. Renomear projeto ou workspace não move arquivos, e o `_meta.md` permite reconstruir os escopos a partir do disco (WFL §1; LOPS). Dois projetos podem ter o mesmo caminho de página sem colidir.
- O histórico git tem uma raiz só, com cada projeto como subárvore (`git log -- <workspace_id>/<project_id>/`).
- Atribuição de evento ao escopo: os dois primeiros segmentos do caminho relativo à raiz são os identificadores (WFL §8.1).
- Duas páginas vivas do mesmo projeto cujo caminho coincide após minúsculas Unicode e NFC são recusadas na criação, em todas as plataformas (WFL §5.2; DV §4.2).

Convenções de nomes:
- O prefixo `_` é convenção da wiki (`_pending/`, `_lint/`, `_rules/`, `_slots/`). Não há lista de nomes reservados na validação de caminho de página (DV §4.2).
- `_global` é o projeto reservado de preferências, no workspace `default` (DV §2). Workspace padrão `default`; projeto de fallback `scratch`.
- `_graveyard/<timestamp>/` é a área de descarte das migrações de estrutura (WFL §10.1, feature 006). `pre-migration-backup.json` fica na raiz de `<data_dir>` (WFL §10.3).
- `sessions/<session_id>.md` fica fora dos candidatos a lápide da reconciliação (WFL §8.3).

## (b) `_meta.md`

- Manifesto de escopo de workspace e de projeto; formato exato e regras de leitura em WFL §1.1.
- Escrito na primeira escrita do escopo no processo, em melhor esforço (falha só gera aviso), ao renomear projeto, e no boot do servidor, que preenche os manifestos faltantes de forma idempotente (`docs/cli.md` §3.1; LOPS).
- Não é página de conceito. Na migração OKF recebe `type: Scope Manifest` quando não tem `type`.
- Manifesto ausente no `reindex` é erro com o caminho exato; a instrução é subir o servidor uma vez para o preenchimento escrevê-lo (LOPS § reindex).
- O watcher e a reconciliação o ignoram em qualquer nível. Escopos purgados com marcador (`purged_scopes`) não são ressuscitados pela reindexação.

## (c) `index.md` e ledger

### `index.md`
- Só a migração de conformidade OKF o cria quando falta (OKF; WFL §10.2 passo 4), com `okf_version: "0.2"`. A exportação regenera o da raiz.
- Na operação normal, `index.md` é **página comum**: a escrita de página não o cria e o watcher/reindex não o ignoram.

### `log-AAAA-MM.md` (ledger)
- Ledger bruto de eventos de **hook**, somente acréscimo (WFL §9). A escrita de página não acrescenta linha.
- O servidor escreve só no arquivo mensal `log-AAAA-MM.md` (mês UTC do evento); `log.md` é só reconhecido como ledger.
- Formato da linha, os 12 eventos e o título (quebras trocadas por espaço, cortado em 120 caracteres): WFL §9. Acréscimo sem troca atômica.
- Reconhecimento como ledger: nome exatamente `log.md` ou `log-AAAA-MM.md` e conteúdo que abre com uma entrada, lendo até 8 KiB (WFL §9).

## (d) Histórico git

- Autor fixo `ai-memory <ai-memory@local>`; sem herdar hooks, assinatura nem identidade do usuário (WFL §1, §7).
- Caminho normal, igual ao original: biblioteca Git embutida (em Go, `go-git`). O programa `git` é fallback **só no Windows** (`docs/windows-native-extras.md` §6.2), conferido no original:
  - **Quando:** ao abrir o repositório, a biblioteca falha com erro que **não** é de dono do repositório e que é "não encontrado" ou erro do sistema operacional com o texto `failed to resolve path`. Erro de dono nunca cai no fallback. Fora do Windows, o mesmo erro é propagado.
  - **Onde:** validação do repositório na abertura, commit, contagem de commits de HEAD, lista de checkpoints recentes e leitura de arquivo numa revisão. A inicialização (`init`) é diferente: no Windows, **qualquer** falha da biblioteca ao inicializar cai para `git init -q`.
  - **Antes de usar o `git`:** valida o confinamento da raiz (diretório `.git`, árvore sem links) e confere `rev-parse --show-toplevel`, `--absolute-git-dir` e `--git-common-dir` contra a raiz. O `git` roda com `-C <raiz>` e sem as variáveis `GIT_DIR`, `GIT_WORK_TREE`, `GIT_COMMON_DIR`, `GIT_INDEX_FILE`, `GIT_OBJECT_DIRECTORY` e `GIT_ALTERNATE_OBJECT_DIRECTORIES`. Cada uso registra aviso (`libgit2 … failed; trying git CLI fallback`).
  - **Commit:** tira do índice os ledgers já rastreados (`rm --cached --force --ignore-unmatch`), adiciona todo o resto (`add -A`) excluindo os ledgers, com os caminhos pela entrada padrão separados por NUL (limite de linha de comando do Windows); `diff --cached --quiet` sem diferença = nenhum commit; senão `commit -q -m <mensagem>` com só `user.name=ai-memory` e `user.email=ai-memory@local` fixados (assinatura e hooks do repositório não são desligados nesse caminho; reproduzir) e devolve `rev-parse HEAD`.
  - **Leituras:** contagem por `rev-list --count HEAD`; checkpoints por `log -n <limite>` (resumo vazio vira `(no summary)`); arquivo numa revisão por `show <rev>:<caminho>`.
- **Não há commit por escrita de página.** Commits automáticos só nos pontos de WFL §7 (fim de sessão com `session <id8>: <título>`, consolidação, bootstrap, comando de commit da CLI e da administração MCP, renomear projeto, reparo de validade, linha de base de atualização, migrações). A ferramenta MCP de escrita tenta `pre-checkpoint before mcp write-page <caminho>` antes e `mcp write-page <caminho>` depois; falha de checkpoint vira aviso (`docs/mcp-server.md` §3.3).
- Escritas rastreadas por caminho: gravar, acrescentar, renomear, remover arquivo e remover diretório passam pelo adaptador; só os caminhos registrados entram, com varredura completa nos casos do §7. Nenhum commit é criado sem mudança. Ledgers nunca entram no índice.
- Um commit por vez por repositório, serializado por trava; leitura em corrida repete conforme o §7.
- Se o HEAD mudou por fora, o índice é relido e a próxima passagem varre a árvore (WFL §7).
- Checkpoints: listagem dos recentes, contagem de commits e leitura de arquivo numa revisão (base da restauração de página, WFL §5.4, §7).
- Rollback de arquivo: toda instalação lê antes o conteúdo anterior ou a ausência dele; reversão em ordem inversa; falha na restauração é o erro de estado inconsistente com o texto literal de WFL §5.2 passo 9.
- Confinamento da árvore (WFL §12): `.git` dentro da raiz, sem links, sem alternates e sem `commondir`; `core.worktree` resolve exatamente para a raiz.
- Mensagens conhecidas de outras features: migração OKF `okf-migration: conform wiki to OKF v0.2`; compactação `pre-compact(session <id8>): checkpoint` / `post-compaction(session <id8>): checkpoint` (LEI).

## (e) Watcher e reconciliação

Parâmetros (WFL §8): debounce de 300 ms e reconciliação de 30 s. O primeiro disparo da reconciliação acontece no início e é descartado.

Escrita atômica (WFL §4; CONST INV-10):
- Temporário com prefixo `.ai-memory-tmp.` no mesmo diretório → sync dos dados → rename → fsync do diretório pai em melhor esforço. Em falha, o temporário é apagado.
- No Windows, só os códigos 32 e 33 repetem a troca, até 5 vezes, com 10, 20, 30, 40 e 50 ms, reaproveitando o mesmo temporário; acesso negado (5) propaga na hora.
- O identificador do arquivo (inode ou índice NTFS) **não** é usado pelo watcher; as próprias escritas são absorvidas pelo prefixo do temporário e pela regra de conteúdo idêntico (WFL §8.1 e §13).

Eventos ignorados (WFL §8.1):
- metadados do Git em qualquer nível;
- links simbólicos e pontos de reparse (inclusive OneDrive sob demanda);
- temporários (`.ai-memory-tmp.`);
- `_meta.md` em qualquer nível;
- `bootstrap.md` e `_pending/` só na raiz do projeto;
- ledgers por nome **e** conteúdo (WFL §9);
- arquivos cuja extensão não é exatamente `.md` (minúsculas);
- eventos fora de `<workspace_id>/<project_id>/…`;
- `sessions/<id>.md` de sessão purgada (se a consulta falhar, processa mesmo assim).

Reconciliação (WFL §8.2): mesmos filtros; diretórios ocultos (nome com ponto) **não** são pulados; diretório de projeto sem linha no banco é órfão e é pulado inteiro, sem criar escopo. Após 5 falhas consecutivas registra `watcher_degraded`, continua a cada 30 s e registra a recuperação.

Comportamentos:
- Criação, modificação ou evento genérico em `.md` reindexa a página. Remoção só registra o caminho no Git para o próximo commit, sem mexer no índice. Evento de diretório de qualquer tipo no projeto reindexa o projeto. O watcher nunca escreve no disco.
- A lápide por arquivo removido é opcional e desligada (WFL §8.3): ausência em duas passagens consecutivas, disjuntor (> max(3, metade) candidatos ausentes no escopo ignora o escopo), sem cadeia bloqueante.
- Watcher, reconciliação e `reindex` usam o mesmo caminho de indexação de página, sem higienizar o conteúdo e com `tier` estrito (WFL §5.5).

Confinamento (WFL §12): caminhada iterativa; item tipo link gera erro de confinamento; itens que somem entre a listagem e a leitura são pulados (não encontrado, permissão negada/violação de compartilhamento, exclusão pendente no Windows).
