# Spec: Formato OKF e migrações de estrutura da wiki

## Metadados
- **ID:** 006
- **Status:** spec auditada contra o original (ver "Correções da auditoria" e os relatórios em `reports/`); sem plano nem tasks; em toda dúvida, reproduzir o original
- **Autor:** phm-aguiar
- **Data:** 2026-10-13
- **Feature Anterior:** 002 (conformidade OKF na escrita; servidor e rotas administrativas)
- **Roadmap:** `specs/features/roadmap.md`
- **Aprovado:** false

## Propósito
> Para quem, o quê e por quê.

A memória precisa ser portável para fora do ai-memory: um pacote de projeto em Open Knowledge Format (OKF) 0.2 pode ser entregue a outra pessoa e lido em qualquer editor ou ferramenta compatível. A wiki já é nativamente OKF na escrita (002); esta feature entrega a exportação do pacote, o executor de migrações de estrutura da wiki com a trava contra formato mais novo, a migração de conformidade OKF com cópia de segurança verificada e o reparo idempotente de `stale_after`.

## Escopo

### Dentro do escopo
- **Executor de migrações de estrutura da wiki** (`docs/wiki-migrations.md`): lista única e ordenada, nomes `AAAA_MM_DDTHH_MM_<descricao>`, registro em `wiki_migrations` (já na V01), contrato da migração (idempotente, sem exclusão sem cópia em `_graveyard/<carimbo>/`, sem LLM, sem SQL fora do escritor único), roda só na inicialização do servidor e na ordem do boot.
- **Recusa de formato mais novo:** nome registrado que o build não conhece impede a abertura da wiki, com mensagem que cita o nome e as duas saídas (atualizar ou restaurar).
- **Migração de conformidade OKF v0.2** (`2026_09_01T18_00_okf_v02_conformance`; `docs/okf.md` § "Migração das lojas existentes"): verificação de pendências (linha do banco: conformar muda qualquer chave **ou** falta `generated.at`; arquivo: falta `type` ou, fora `_meta.md`, `generated.at`), cópia verificada, checkpoint `pre-okf-migration checkpoint`, passo do banco (versões atuais, mesmo id e `updated_at`, `generated.at` do `updated_at`), passo dos arquivos (só se mudou; `generated.at` do banco, senão do mtime, senão 1970), `index.md` por projeto quando falta (texto exato do doc; sem pendências não cria nada), commit único `okf-migration: conform wiki to OKF v0.2`, registro.
- **Cópia de segurança prévia** (`docs/okf.md` § "Cópia de segurança"): tirada **antes** de abrir o banco (gatilho: arquivos não conformes e sem recibo com cópia presente); destino `AI_MEMORY_BACKUP_DIR`, `backups/` no diretório de dados em contêiner, senão a pasta pessoal (no Windows, o perfil do usuário); arquivo `ai-memory-backup-okf-v0.2-AAAAMMDD-HHMMSS.tar.gz` (UTC); exclusões (cópia, subárvore do destino se interna, `pre-migration-backup.json.tmp`, `.serve.lock*`), falha diante de link simbólico ou ponto de reparse em `wiki/` ou em qualquer parte do diretório de dados; verificação; recibo `pre-migration-backup.json` (campos exatos no doc) gravado por substituição atômica; reuso de cópia válida.
- **Reparo de `stale_after`** de data pura a cada início (depois da migração e do preenchimento dos manifestos), só em páginas atuais com `expires_at` de data pura, linha e arquivo, com commit `okf: repair date-only stale_after on existing pages` se algum arquivo mudou; falha vira aviso.
- **Exportação** `export-okf --workspace --project -o/--to` (`POST /admin/export-okf`): `index.md` gerado com texto e lista exatos (todo diretório de primeiro nível exceto `_pending`), exclusões (`index.md` e `log.md` por nome em qualquer nível, `log-AAAA-MM.md` com conteúdo de ledger, `_pending` em qualquer nível, não `.md`), `_meta.md` incluído, conformidade exigida (`type` não vazio) com falha 422 que identifica a página, título (fora de blocos de código) e descrição derivados na cópia, páginas reserializadas, reescrita de links internos para links markdown relativos (regras do doc). Diretórios pais do destino são criados; a saída imprime `exported OKF bundle to <caminho> (<bytes> bytes)`.
- **Status** mostra a cópia de segurança enquanto ela estiver em disco (`docs/okf.md` § "Aviso na página inicial").

### Fora do escopo (explicitamente)
- Diálogo de aviso na página inicial web (feature 014).
- Comando de importação: não existe no doc; desempacotar e reindexar.
- Conformidade OKF na escrita (feature 002).

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
- **Tecnologia:** `archive/tar` e `compress/gzip` da stdlib; `AI_MEMORY_BACKUP_DIR` lido pelo pacote de configuração (INV-01).
- **Segurança:** cópia falha diante de link simbólico ou ponto de reparse; destino da cópia nunca é a raiz do diretório de dados.
- **Ordem de boot:** trava de instância única → cópia prévia → abertura do banco e backfills → saneamento/credenciais → migrações da wiki → manifestos `_meta.md` → reparo de `stale_after` → checkpoint de linha de base → watcher (`docs/okf.md`, `docs/wiki-migrations.md` § "Quando roda").
- **Windows:** a trava exclusiva do servidor impede ler `.serve.lock*`; a cópia os pula por nome. Caminhos de erro de exportação saem com `\`. `AI_MEMORY_BACKUP_DIR` vazio vale como ausente na migração, mas não na cópia prévia (replicar a diferença, como no original).

## Critérios de Sucesso
- [ ] As propriedades de `docs/okf.md` § Verificação (ida e volta, conformidade, sem churn, idempotência, trava da cópia, exportação) passam como teste, exceto a do aviso web e a do benchmark
- [ ] Cada migração tem teste de pré-condição, pós-condição e reexecução sem efeito (`docs/wiki-migrations.md`)
- [ ] Recusa de formato mais novo testada
- [ ] `go vet ./...`, `go test -race ./...` e `golangci-lint run` passam sem apontamentos

## Abordagem Escolhida
Executor e migração no pacote `wiki`, chamados pelo `serve` na ordem do doc; a cópia prévia roda no `cli` antes de abrir o banco. A exportação é uma rota administrativa que lê a árvore do projeto, valida e monta o tarball em memória de fluxo, sem alterar a wiki.

## Dependências
- 002: conformidade OKF na escrita, escritas rastreadas e commit, servidor e `/admin/*`, reconhecimento de ledger, boot do `serve`, `.serve.lock`.
- `docs/okf.md`, `docs/wiki-migrations.md`, `docs/wiki-files-layer.md` §10, `docs/complementary-contracts.md` §4.

## Checklist de Prontidão
- [x] Propósito claro e sem ambiguidade
- [x] Escopo delimitado (dentro/fora)
- [x] Cenários cobrem happy path e alternativos
- [x] Edge cases identificados com comportamento esperado
- [x] Constraints explícitas
- [x] Critérios de sucesso mensuráveis
- [x] Abordagem escolhida justificada

## Correções da auditoria

Auditoria 012 (Chinese Wall, contra o código original). Relatório: `reports/012-auditoria-006-007-009.md`.

- `docs/wiki-migrations.md`: acrescentados o nome registrado e a descrição literal da migração OKF, a inspeção de confinamento prévia (link simbólico ou reparse point, inclusive junção no Windows, é erro), o texto literal da recusa de formato mais novo, as mensagens de log do executor e o `applied_at = 0` com relógio antes da época.
- `docs/okf.md`: frontmatter que não é mapeamento vira mapeamento vazio antes da conformidade; chaves de origem só contam quando são texto; `generated` mapeamento sem `by` recebe `by` preservando as demais chaves.
- Verificados sem divergência: versão `0.2`, tabela de famílias→`type`, precedência `kind`/`slot_kind`, `sources`/`author: process:<agent>`, `stale_after` e reparo, `generated.by`/`generated.at`, algoritmo do executor e registro `wiki_migrations`.
