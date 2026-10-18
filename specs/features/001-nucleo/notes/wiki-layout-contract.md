# Nota: layout em disco, _meta, index, log e histórico git

Fontes abreviadas: WFL = `docs/wiki-files-layer.md`; OKF = `docs/okf.md`; LOPS = `docs/lifecycle-ops.md`; DV = `docs/domain-vocabulary.md`; PLAN = `specs/features/001-nucleo/plan.md`; SPEC = `specs/features/001-nucleo/spec.md`; CONST = `.github/memory/constitution.md`; ARCH = `docs/architecture.md`; BKP = `docs/backup.md`; LEI = `docs/lifecycle-event-ingestion.md`.

> **Auditoria de 2026-10-18, passada 2.** `docs/wiki-files-layer.md` §1, §1.1, §7, §8 e §9 foram
> reescritos contra a referência e **vencem** esta nota. Correções que afetam o código:
> - a escrita de página **não** anexa linha a `log.md` e **não** faz commit. O "commit git por evento"
>   (seção (d)) e a linha de `log.md` (seção (c)) estão revogados (002/T080 e T089). Ledgers são só de
>   eventos de hook. O commit com `checkpoint` na resposta é da ferramenta MCP de escrita
>   (`memory_write_page: <caminho>`, `checkpoint` nulo quando nada mudou ou o commit falhou);
> - commits automáticos só nos pontos de wiki §7, com escritas rastreadas por caminho e varredura completa
>   nos casos do §7; nenhum commit sem mudança; autor `ai-memory <ai-memory@local>`. As mensagens de
>   commit inventadas (ADR-25) estão revogadas;
> - o identificador de arquivo devolvido pela escrita atômica **não** é usado pelo watcher (o item da
>   seção (e) que diz o contrário está errado); em falha, o temporário é apagado;
> - `index.md` não é criado pela escrita normal e não é ignorado pelo watcher: é página comum fora da
>   migração OKF;
> - diretório de projeto sem linha no banco é **órfão**: a reconciliação e o evento de diretório o pulam
>   inteiro, sem criar escopo;
> - formato do `_meta.md`: wiki §1.1.

## (a) Árvore de diretórios da wiki por escopo, com nomes reservados

Árvore canônica (WFL §1):

```text
<data_dir>/wiki/                       ← raiz (repositório Git)
  <workspace_id>/
    <project_id>/
      _meta.md        ← manifesto do escopo (nomes humanos ↔ identificadores)
      index.md        ← raiz do bundle OKF (declara okf_version)
      log.md / log-AAAA-MM.md   ← ledger bruto de eventos de hook (somente acréscimo)
      sessions/<session_id>.md
      concepts/ decisions/ gotchas/ procedures/ …   ← páginas por tipo
      _pending/       ← sidecars de propostas (não indexados)
```

Complementos:
- `<data_dir>/wiki/` é a fonte da verdade em markdown e um repositório git. O índice derivado fica em `<data_dir>/db/memory.sqlite` (ARCH "Storage architecture"). `<data_dir>/raw/` guarda segmentos JSONL de workstreams gerenciados (ARCH). Esse último está fora do escopo da 001.
- LOPS "disk layout" (linhas 138-150) mostra uma variante: o workspace também tem seu `_meta.md` (`<workspace_id>/_meta.md`, "workspace name, for rebuilds"). Mostra ainda `.git/` na raiz, `_rules/`, `bootstrap.md` e `log-YYYY-MM.md`. WFL §1 só desenha o `_meta.md` do projeto, mas WFL §5.2 passo 7 fala em "linhas do projeto e do workspace e o manifesto `_meta.md`", e OKF diz que `_meta.md` é manifesto de "workspace ou projeto".
- PLAN §2.4 (a decisão da 001) fixa: `<data_dir>/wiki/<workspace_id>/<project_id>/{_meta.md,index.md,log.md,sessions/,concepts/,decisions/,...}`. Os diretórios são por UUID, nunca por nome.

Regras de caminho:
- Todo caminho é derivado da raiz do wiki e dos identificadores do escopo. Nenhum componente monta caminho por concatenação manual (WFL §1; CONST "Anti-Padrões": usar o tipo de caminho do domínio).
- O diretório usa UUID, não nome. Renomear projeto ou workspace não move arquivos, e o `_meta.md` permite reconstruir os escopos a partir do disco (WFL §1; LOPS 156-170, 361-364). Dois projetos podem ter o mesmo caminho de página sem colidir no disco (LOPS).
- O histórico git tem uma raiz só, com cada projeto como subárvore. `git log -- <workspace_id>/<project_id>/` mostra o histórico de um projeto (LOPS).
- Atribuição de evento ao escopo: os dois primeiros segmentos do caminho relativo à raiz são os identificadores (WFL §8.1).
- Duas páginas vivas do mesmo projeto cujo caminho coincide após minúsculas e Unicode composto (NFC) são recusadas na criação, mesmo que sejam diferentes no sistema de arquivos. A regra vale em todas as plataformas (WFL §5.2, propriedades observáveis; PLAN P-04 caso C `PortableKey`).

Nomes reservados e convenção `_`:
- O prefixo `_` segue a convenção de nomes reservados da wiki (como `_pending/` e `_lint/`) (DV §2, linha 51).
- `_global` é o projeto reservado de preferências, no workspace `default` (DV §2). O wikilink `[[_global:caminho]]` aponta para ele (WFL §3.4).
- Workspace padrão é `default`. O projeto de fallback é `scratch` (DV §2).
- `_meta.md`: manifesto de identidade (WFL §1; OKF "Arquivos no diretório").
- `_pending/`: sidecars de propostas (automelhoria), não indexados (WFL §1, §5.4). Na migração OKF só é ignorado na raiz do projeto. Na exportação é ignorado em qualquer nível (OKF 198).
- `_graveyard/<timestamp>/`: área de descarte das migrações de estrutura (WFL §10.1). Fora do escopo da 001.
- `_rules/` aparece em LOPS (linha 147). `_lint/` aparece só como exemplo em DV §2.
- `bootstrap.md` é ignorado pelo watcher (WFL §8.1, §8.2) e aparece em LOPS. Fora do escopo da 001.
- `index.md` e `log.md` são nomes reservados pelo formato OKF (OKF 24).
- `sessions/<session_id>.md`: página de sessão. Fica fora dos candidatos a tombstone (WFL §8.3).
- `.git/` na raiz da wiki.
- Fora da wiki, o backup pré-migração grava `pre-migration-backup.json` na raiz de `<data_dir>` (WFL §10.3). Fora do escopo da 001.

LACUNA: a lista completa de diretórios de tipo (`concepts/ decisions/ gotchas/ procedures/ …`) é aberta nas fontes lidas. O mapeamento tipo→diretório está em OKF "Mapeamento de páginas", que não foi lido (só consta nas seções `type` e `_pending` de OKF 86). `_rules/` e `bootstrap.md` não estão no layout da 001 (PLAN §2.4).

## (b) `_meta.md`: conteúdo e formato

- Papel: manifesto de escopo, "nomes humanos ↔ identificadores" (WFL §1). Guarda o nome humano do workspace ou do projeto, e para projetos também o caminho do repositório. É um pequeno manifesto em frontmatter, usado para reconstruir escopos no `reindex` sem o banco (LOPS 143-170, 961; WFL §1).
- Quando é escrito:
  - na escrita da primeira página do escopo, garantido sob a trava da página (WFL §5.2 passo 7);
  - ao renomear projeto, com checkpoint se a árvore mudou (LOPS 361-364);
  - no backfill de inicialização, `backfill_scope_manifests`, que cria o manifesto dos escopos que não o têm (WFL §5.4; LOPS 961-963; `docs/cli.md:172`).
  - Um conjunto em memória registra os escopos cujo manifesto já foi garantido na sessão (WFL §5.1).
- Não é página de conceito. Na migração OKF recebe apenas `type: Scope Manifest`, quando não tem `type` (OKF 38, 89; WFL §10.2 passo 4).
- Se o manifesto faltar, o `reindex` reporta o problema. A instrução é subir o servidor uma vez para o backfill escrevê-lo e só então rodar o `reindex` (LOPS 961-968).
- O watcher o ignora (ver (e)). O `reindex_all` não ressuscita escopos purgados que têm marcador (WFL §5.4). O motivo: o `_meta.md` recriaria o escopo (tabela `purged_scopes`, `docs/database-schema.md` §6).

LACUNA: as chaves exatas do frontmatter de `_meta.md` não estão nas fontes lidas. Sabe-se que há nome humano, `type` (OKF) e, para projeto, caminho do repositório. Nome de campo, formato e se o manifesto do workspace é obrigatório na 001 não estão especificados. O PLAN §2.4 só cita `_meta.md` no projeto. Decisão a registrar em ADR (CONST proíbe tratar "a confirmar" como fato).

## (c) `index.md` e `log.md`

### `index.md`
- Raiz do bundle OKF do projeto. Declara `okf_version` (WFL §1). A versão atual é `okf_version: "0.2"`, e o frontmatter permitido em `index.md` é só esse (OKF 19-23).
- Forma gerada (OKF 46): frontmatter `okf_version: "0.2"`, depois um corpo com título fixo seguido de uma lista de links, um por diretório de primeiro nível do projeto, em ordem alfabética. Não há prosa fora da lista.
- Quem escreve: a migração de conformidade escreve o `index.md` quando ele falta. "A operação normal não o escreve" (OKF 37; WFL §10.2 passo 4). A exportação regenera o da raiz. O `index.md` é ignorado como página em qualquer nível (OKF 51, 198).
- LACUNA: o texto do "título fixo", o formato exato dos links e o momento em que a 001 cria o `index.md` não estão nas fontes lidas. A spec e o PLAN §2.4 listam `index.md` no layout, mas não dizem se a escrita normal o cria na 001. O OKF diz que a operação normal não o escreve. É conflito ou decisão a registrar em ADR.

### `log.md` / `log-AAAA-MM.md`
- Papel: ledger bruto de eventos de hook, somente acréscimo (WFL §1, §9).
- Formato de cada entrada: `## [timestamp] evento | título` (WFL §9). Fica na raiz do projeto, sem frontmatter (OKF 39). `log-AAAA-MM.md` é um arquivo por mês em UTC (OKF 39; LOPS 148).
- Reconhecimento como ledger (WFL §9): o nome tem de ser exatamente `log.md` ou `log-AAAA-MM.md`, e o conteúdo tem de abrir com uma entrada de log. Se há frontmatter, o bloco precisa fechar dentro de um limite de linhas, e a primeira linha não vazia depois dele tem de ser a entrada. Linha vazia antes da primeira entrada significa que não é ledger. Só os primeiros 8 KiB são lidos para decidir.
- Escrita: `append_under_project` acrescenta bytes ao `log*.md` do projeto e reporta a alteração ao Git (WFL §5.4). Passa pelo adaptador de escritas rastreadas (WFL §7).
- Na 001: SPEC "Cenário principal" passo 4 diz que, após o commit, "uma linha é anexada ao `log.md`" e a resposta traz identificador, caminho e checkpoint. PLAN §2.4 também usa `log.md`.
- Conflito entre fontes: OKF 40 diz que `log.md` é nome reservado pelo formato e "o ai-memory não o adota", porque o histórico é o próprio histórico de commits. Esse mesmo OKF e WFL §1/§9 adotam `log-AAAA-MM.md` (mensal). WFL §1 e §9 citam ambos os nomes.
- LACUNA: o formato exato de `timestamp` (RFC 3339 UTC, presumivelmente) e o vocabulário de `evento` e `título` não estão nas fontes lidas. A decisão entre `log.md` único (SPEC/PLAN) e `log-AAAA-MM.md` (WFL/OKF/LOPS) precisa de ADR. Não há regra de rotação além de "um arquivo por mês UTC".

## (d) Regras de commit git por evento

Regras gerais (WFL §1 e §7; PLAN P-09; CONST):
- Autor e e-mail fixos: nome `ai-memory`, e-mail `ai-memory@local`, para não vazar a identidade git do usuário (WFL §1; PLAN §2.4 "ai-memory <ai-memory@local>").
- Implementação da 001 (PLAN P-09): git do sistema via `os/exec`, sem `go-git` (CONST "Preferências"). `gitrepo` inicializa o repositório e valida `git` no PATH na abertura, com erro claro. Faz `add` + `commit` por evento sob mutex próprio, com `-c user.name=ai-memory -c user.email=ai-memory@local -c commit.gpgsign=false`. Os paths vão sempre depois de `--` e relativos à raiz.
- `gpgsign`: desligado por flag, `commit.gpgsign=false`, em todo commit (PLAN P-09).
- Um commit por vez por repositório, serializado por trava. Commits concorrentes esperam, em vez de falhar (WFL §7).
- O commit só ocorre depois do sucesso do índice. Falha de commit vira aviso, não erro (PLAN P-03). Se o banco falhar após instalar o arquivo, o arquivo é revertido e nenhum commit é criado (SPEC "Cenários alternativos").
- Commit por caminho (WFL §7): só os caminhos registrados pelas escritas rastreadas entram. Gravar, acrescentar, renomear, remover arquivo e remover diretório passam pelo adaptador, e um lint proíbe gravação direta fora dele. Uma varredura completa acontece quando:
  - o último registro não é confiável (`walk_needed`);
  - nenhum caminho foi registrado desde o último commit;
  - passaram 600 s desde a última varredura.
  A varredura conta `unreported_writes`, com amostra de até 5 caminhos.
- Leitura em corrida: o commit tenta de novo até 4 vezes, com 25 ms entre elas. Se ainda falhar, os caminhos continuam registrados para o próximo commit (WFL §7).
- `commit_all` não devolve identificador quando não há mudança (WFL §7).
- Se o HEAD mudou por fora, o índice é relido e a próxima passagem varre a árvore (WFL §7).
- Fallback para o `git` do sistema só vale para o go-git de WFL. Na 001 o git do sistema já é o caminho principal, então esse fallback não se aplica (PLAN P-09).
- Checkpoint: a escrita de página devolve identificador, caminho e checkpoint (SPEC passo 4). Há também listagem dos checkpoints recentes (mais novos primeiro), contagem de commits e leitura de arquivo em uma revisão, que é a base de `restore_page_from_checkpoint` (WFL §5.4, §7).
- Rollback de arquivo: toda instalação guarda o conteúdo anterior ou a ausência dele. Falha na restauração gera `ErrInconsistentState` (WFL §5.2 passo 9; PLAN P-03).
- O repositório mantém o git aberto entre commits, na versão go-git de WFL (WFL §7). Não se aplica ao `os/exec` da 001.
- Validação de confinamento da árvore (WFL §12): `.git` dentro da raiz, sem links, sem alternates de objetos e sem `commondir`. `core.worktree` deve resolver exatamente para a raiz do wiki.

Eventos que geram commit automático (WFL §7):
- fim de sessão (roteador de hooks);
- consolidação;
- bootstrap;
- comando de commit (CLI e administração MCP);
- renomear projeto (LOPS 361-364);
- reparo de datas de validade;
- linha de base de atualização, checkpoint dos arquivos já existentes antes das primeiras mudanças;
- migrações.

Escopo da 001: escrita de página, comando de commit/checkpoint e reindex. Hooks, sessão e consolidação são da 002/F003 (SPEC "Fora do escopo").

Mensagens conhecidas:
- Migração OKF: `okf-migration: conform wiki to OKF v0.2`, um único commit (WFL §10.2 passo 5; OKF 195). Antes dela há um checkpoint Git do estado da árvore (WFL §10.2 passo 2). Fora do escopo da 001.
- Compactação: `pre-compact(session <id8>): checkpoint` ou `post-compaction(session <id8>): checkpoint`, com `<id8>` igual aos oito primeiros caracteres do id (LEI 445). Fora do escopo da 001.
- Backup espelho externo (não é o commit da wiki): `auto: snapshot <date>` (BKP 195, `docs/assets/examples/backup/ai-memory-snapshot.sh:85`).

LACUNA: não há mensagem canônica especificada para commit de escrita de página, consolidação, fim de sessão, bootstrap, rename e reparo de validade. Também não há a mensagem do checkpoint/linha de base de atualização. A 001 precisa definir isso em ADR. Também não há política de `git init` inicial (branch, `.gitignore` interno da wiki) nas fontes lidas. O BKP trata `.gitignore` do espelho externo, o que é outro assunto.

## (e) O que o watcher deve ignorar

Parâmetros (WFL §8; PLAN P-08; SPEC edge case "Edição manual"): debounce de 300 ms e reconciliação de 30 s. O primeiro disparo da reconciliação acontece no início e é descartado. A primeira reconciliação não roda logo no boot.

Prefixo temporário:
- `.ai-memory-tmp.` é o prefixo do arquivo temporário. Ele fica no mesmo diretório do destino (WFL §4; PLAN §2.4 e P-06: `.ai-memory-tmp.*`; CONST INV-10).
- Escrita atômica: temp → sync dos dados → rename sobre o destino → fsync do diretório pai (melhor esforço; no-op no Windows). No Windows, os erros 32 e 33 repetem a troca até 5 vezes, com 10, 20, 30, 40 e 50 ms (WFL §4; PLAN P-06).
- A escrita devolve o identificador do arquivo (inode no Unix; índice NTFS no Windows, ou 0 se o volume não expõe) (**corrigido:** o watcher não usa esse identificador; só o prefixo do temporário e a regra de conteúdo idêntico. WFL §4 revisado). A checagem de hash de conteúdo no índice também torna a reindexação idêntica um no-op (WFL §8.1 e §13 item 3).

Eventos ignorados (WFL §8.1):
- caminhos internos do Git (inclusive metadados aninhados);
- links simbólicos e pontos de reparse;
- arquivos temporários do wiki (prefixo `.ai-memory-tmp.`);
- `_meta.md`;
- `bootstrap.md`;
- ledgers `log*.md`, por nome e por conteúdo (WFL §9);
- caminhos em `_pending/`;
- arquivos que não são `.md`;
- eventos fora do formato `<workspace_id>/<project_id>/…` (escopo não atribuível);
- `sessions/<id>.md` de sessão purgada, cujo evento é ignorado (se a consulta falhar, processa mesmo assim).

Reconciliação (WFL §8.2): ignora links simbólicos, temporários, `_pending/` no nível da raiz, `_meta.md`, `bootstrap.md` e ledgers. Diretórios ocultos (nome com ponto) NÃO são pulados. Após 5 falhas consecutivas registra erro `watcher_degraded`, continua tentando a cada 30 s e registra a recuperação.

Comportamentos do watcher que afetam o layout:
- Criação, modificação ou evento genérico em `.md` reindexa a página. Remoção só registra o caminho no Git, não mexe no índice. Criação de diretório no projeto reindexa o projeto inteiro. O watcher nunca escreve no disco (WFL §8.1).
- Remoção de arquivo não apaga a página por padrão. O tombstone (`reconcile_tombstones_deleted_pages`) é opcional e desligado (WFL §8.3; PLAN P-08; CONST). Exige ausência em duas passagens consecutivas e um disjuntor (> max(3, metade) candidatos ausentes no escopo ignora o escopo). É soft delete. Os candidatos excluem pendências, metadados, `bootstrap.md`, ledgers e `sessions/<id>.md`.
- Pelo CONST, a 001 deve usar o `fsnotify` (PLAN P-08). Watcher e reconciliação convergem para `wiki.IndexFile(ref)`, o mesmo caminho do `reindex`.

Confinamento (WFL §12), relevante ao watcher e ao reindex:
- A caminhada é iterativa. Qualquer item tipo link gera erro de confinamento.
- Itens que somem entre a listagem e a leitura de metadados são pulados (não encontrado, permissão negada/violação de compartilhamento, exclusão pendente no Windows).
- A leitura da página é por caminho do confinamento (sem links simbólicos nem pontos de reparse).

LACUNA: a lista de diretórios ocultos que o watcher da 001 deve pular não está definida além de `.git`. WFL §8.2 diz explicitamente que ocultos não são pulados na reconciliação. Os arquivos de lock `.git/index.lock` já são cobertos pela regra de caminhos internos do Git.

## Resumo de lacunas e conflitos para o ADR

1. Chaves exatas de `_meta.md` (workspace e projeto) e se o manifesto do workspace existe na 001.
2. `log.md` (SPEC/PLAN) versus `log-AAAA-MM.md` (WFL, OKF, LOPS). O OKF diz que `log.md` não é adotado. Formato do timestamp e vocabulário de `evento`.
3. Quando o `index.md` nasce na 001. O OKF diz que a operação normal não o escreve. O título fixo e o formato dos links não estão definidos.
4. Mensagens de commit por evento, fora `okf-migration: …` e `pre-compact/post-compaction(session <id8>): checkpoint`. Política de `git init`.
5. Tabela tipo→diretório de página (OKF "Mapeamento de páginas", não lido).
