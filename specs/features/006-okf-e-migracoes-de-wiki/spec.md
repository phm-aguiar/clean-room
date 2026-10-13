# Spec: Formato OKF e migrações de estrutura da wiki

## Metadados
- **ID:** 006
- **Status:** draft (revisada contra os docs em 2026-10-13; nova revisão prevista quando as features anteriores terminarem)
- **Autor:** phm-aguiar
- **Data:** 2026-10-13
- **Feature Anterior:** 002 (conformidade OKF na escrita: 002/T090 e T091; servidor e rotas administrativas)
- **Roadmap:** `specs/features/roadmap.md`
- **Aprovado:** false

## Propósito
> Para quem, o quê e por quê.

A memória precisa ser portável para fora do ai-memory: um pacote de projeto em Open Knowledge Format (OKF) 0.2 pode ser entregue a outra pessoa e lido em qualquer editor ou ferramenta compatível. A wiki já é nativamente OKF na escrita (002); esta feature entrega a exportação do pacote, o executor de migrações de estrutura da wiki com a trava contra formato mais novo, a migração de conformidade OKF com cópia de segurança verificada e o reparo idempotente de `stale_after`.

## Escopo

### Dentro do escopo
- **Executor de migrações de estrutura da wiki** (`docs/wiki-migrations.md`): lista única e ordenada, nomes `AAAA_MM_DDTHH_MM_<descricao>`, registro em `wiki_migrations` (já na V01), contrato da migração (idempotente, sem exclusão sem cópia em `_graveyard/<carimbo>/`, sem LLM, sem SQL fora do escritor único), roda só na inicialização do servidor e na ordem do boot.
- **Recusa de formato mais novo:** nome registrado que o build não conhece impede a abertura da wiki, com mensagem que cita o nome e as duas saídas (atualizar ou restaurar).
- **Migração de conformidade OKF v0.2** (`2026_09_01T18_00_okf_v02_conformance`; `docs/okf.md` § "Migração das lojas existentes"): verificação de pendências, cópia verificada, checkpoint, passo do banco (versões atuais, mesmo id e `updated_at`), passo dos arquivos (só se mudou), `index.md` por projeto quando falta, commit único `okf-migration: conform wiki to OKF v0.2`, registro.
- **Cópia de segurança prévia** (`docs/okf.md` § "Cópia de segurança"): destino `AI_MEMORY_BACKUP_DIR`, `backups/` no diretório de dados em contêiner, senão a pasta pessoal; arquivo `ai-memory-backup-okf-v0.2-AAAAMMDD-HHMMSS.tar.gz`; conteúdo, verificação e recibo `pre-migration-backup.json` gravado por substituição atômica; reuso de cópia válida.
- **Reparo de `stale_after`** de data pura a cada início, com commit `okf: repair date-only stale_after on existing pages` se algo mudou; falha vira aviso.
- **Exportação** `export-okf --workspace --project -o/--to` (`POST /admin/export-okf`): `index.md` gerado, exclusões (`index.md`, `log.md`, ledgers pelo conteúdo, `_pending` em qualquer nível, não `.md`), conformidade exigida (`type` não vazio) com falha que identifica a página, título e descrição derivados na cópia, reescrita de links internos para links markdown relativos.
- **Status** mostra a cópia de segurança enquanto ela estiver em disco (`docs/okf.md` § "Aviso na página inicial").

### Fora do escopo (explicitamente)
- Diálogo de aviso na página inicial web (feature 014).
- Comando de importação: não existe no doc; desempacotar e reindexar.
- Conformidade OKF na escrita (feita na 002, T090 e T091).

## Comportamento Esperado

### Cenário Principal (Happy Path)
1. Numa instalação nova, o servidor sobe, o executor encontra a migração OKF pendente, ela não acha nada a conformar e termina sem cópia e sem commit, e o nome é registrado.
2. `ai-memory export-okf --project p -o p.tar.gz` pede o pacote ao servidor e grava o arquivo; o pacote tem `index.md` com `okf_version: "0.2"`, as páginas conformes e os links internos reescritos, e não tem ledgers.

### Cenários Alternativos
- **Wiki com arquivos editados à mão sem `type`:** a migração faz a cópia verificada, o checkpoint, conforma linhas e arquivos e faz um único commit.
- **Cópia de segurança não pode ser gravada ou verificada:** a migração aborta antes de tocar em qualquer dado, o servidor não sobe e tenta de novo no próximo início.
- **Nome desconhecido em `wiki_migrations`:** a wiki não abre e a mensagem cita o nome e as saídas.
- **Exportação com página não conforme:** falha inteira, identificando a página, sem entregar arquivo.
- **Exportação de projeto sem diretório de wiki:** falha.

## Edge Cases
- **Idempotência:** segunda execução não migra nada e não cria segunda cópia.
- **Sem churn:** depois da migração, ids e `updated_at` das versões atuais são idênticos aos de antes.
- **Falha ao registrar após sucesso:** a inicialização para; a migração roda de novo e não muda nada por ser idempotente.
- **Arquivo `log-AAAA-MM.md` com página comum:** continua no pacote e precisa ser conforme; ledger de verdade (pelo conteúdo) fica fora.
- **Links para outro projeto ou workspace:** ficam `[[...]]` literal no pacote; código cercado e inline não é alterado; destino inexprimível vai entre `<...>` ou fica literal.

## Constraints
- **Tecnologia:** `archive/tar` e `compress/gzip` da stdlib; `AI_MEMORY_BACKUP_DIR` lido por `internal/config` (INV-01).
- **Segurança:** cópia falha diante de link simbólico ou ponto de reparse; destino da cópia nunca é a raiz do diretório de dados.
- **Ordem de boot:** trava de instância única → cópia prévia → abertura do banco e backfills → saneamento → migrações da wiki → watcher (`docs/wiki-migrations.md` § "Quando roda").

## Critérios de Sucesso
- [ ] As propriedades de `docs/okf.md` § Verificação (ida e volta, conformidade, sem churn, idempotência, trava da cópia, exportação) passam como teste, exceto a do aviso web e a do benchmark
- [ ] Cada migração tem teste de pré-condição, pós-condição e reexecução sem efeito (`docs/wiki-migrations.md`)
- [ ] Recusa de formato mais novo testada
- [ ] `scripts/check.sh` e o lint passam sem apontamentos

## Abordagem Escolhida
Executor e migração no pacote `wiki`, chamados pelo `serve` na ordem do doc; a cópia prévia roda no `cli` antes de abrir o banco. A exportação é uma rota administrativa que lê a árvore do projeto, valida e monta o tarball em memória de fluxo, sem alterar a wiki.

## Dependências
- 002: conformidade OKF na escrita (T090, T091), escritas rastreadas e commit (T080), servidor e `/admin/*`, reconhecimento de ledger (T022), boot do `serve` (T042), `.serve.lock` (T040).
- `docs/okf.md`, `docs/wiki-migrations.md`, `docs/wiki-files-layer.md` §10, `docs/complementary-contracts.md` §4.

## Checklist de Prontidão
- [x] Propósito claro e sem ambiguidade
- [x] Escopo delimitado (dentro/fora)
- [x] Cenários cobrem happy path e alternativos
- [x] Edge cases identificados com comportamento esperado
- [x] Constraints explícitas
- [x] Critérios de sucesso mensuráveis
- [x] Abordagem escolhida justificada
