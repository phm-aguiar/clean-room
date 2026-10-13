# plan.md: Plano de Implementação Técnica — OKF e migrações de wiki (006)

> Plano gerado em 2026-10-13, antes de a 002 ser executada. Revisar quando as features anteriores terminarem.

## 1. Metadados do Plano
- **Stack Tecnológico:** a da 002, sem dependência nova. `archive/tar`, `compress/gzip`, `encoding/json` (recibo).
- **Feature Fonte:** `specs/features/006-okf-e-migracoes-de-wiki/spec.md`
- **Escopo:** executor de migrações da wiki, recusa de formato mais novo, migração de conformidade OKF com cópia prévia e recibo, `index.md` por projeto, reparo de `stale_after`, exportação OKF.
- **Fontes de contrato:** `docs/okf.md`, `docs/wiki-migrations.md`, `docs/wiki-files-layer.md` §10, `docs/cli.md` §3.1, `docs/complementary-contracts.md` §4.

## 2. Design de Contratos e Fronteiras
- **Contrato:** rota `POST /admin/export-okf?workspace=…&project=…` (resposta `application/gzip`); comando `export-okf`; recibo `pre-migration-backup.json` (`path`, `size_bytes`, `entries`, `created_at`, `label`, `free_bytes`); nome de migração `2026_09_01T18_00_okf_v02_conformance`; mensagem de commit `okf-migration: conform wiki to OKF v0.2`; mensagem de reparo `okf: repair date-only stale_after on existing pages`.
- **Esquema SQL:** sem migração. `wiki_migrations(name, applied_at)` já na V01.

### 2.1 Interface no `wiki`
```go
// WikiMigration é uma migração de estrutura da wiki (docs/wiki-migrations.md).
type WikiMigration struct {
    Name, Description string
    Apply func(ctx context.Context, w storage.Writer, root string) error
}
// RunWikiMigrations aplica as pendentes na ordem; recusa nome desconhecido já registrado.
func RunWikiMigrations(ctx context.Context, w storage.Writer, r storage.Reader, root string, list []WikiMigration) error
```
A lista tem um único item nesta feature (a migração OKF), então a interface tem chamador real.

## 3. Decisões Arquiteturais e Justificativas

- **Decisão (K-01): executor no `wiki`, registro pelo escritor.** O executor lê os nomes aplicados, recusa nome desconhecido antes de rodar qualquer pendência (só se a lista do build não for vazia), aplica na ordem e registra cada sucesso com `INSERT ... ON CONFLICT DO NOTHING` pelo escritor único. Erro interrompe, não registra e propaga; o servidor não sobe.
  - **Justificativa:** `docs/wiki-migrations.md` § "Algoritmo do executor".
  - **Alternativa Rejeitada:** registrar antes de aplicar (uma falha deixaria a migração marcada sem ter rodado).

- **Decisão (K-02): cópia prévia no `cli`, antes de abrir o banco.** O `serve` chama a verificação de pendência OKF sobre os arquivos (sem banco) e, se há páginas não conformes e não há recibo com cópia presente, gera o tarball do diretório de dados (sem a própria cópia, o temporário do recibo e as travas), relê para conferir entradas e tamanho, grava o recibo por substituição atômica. Falha impede a subida. O destino vem de `AI_MEMORY_BACKUP_DIR` (lido em `internal/config`), de `backups/` no diretório de dados quando em contêiner, senão da pasta pessoal; nunca a raiz do diretório de dados.
  - **Justificativa:** `docs/okf.md` § "Cópia de segurança" e passo 1 da migração.
  - **Alternativa Rejeitada:** fazer a cópia dentro da migração (o banco já teria avançado o esquema).

- **Decisão (K-03): migração OKF em passos com checkpoint e commit único.** Verifica pendências (linhas e arquivos); sem pendência, termina sem cópia e sem commit. Com pendência: reaproveita ou cria a cópia, checkpoint, passo do banco (versões atuais conformadas no lugar: mesmo id, mesma versão, `updated_at` e corpo intocados, `generated.at` do `updated_at`), passo dos arquivos (só reescreve se mudou; `_meta.md` recebe só `type`), `index.md` por projeto quando falta, um commit. Ignora `index.md`, ledgers pelo conteúdo e `_pending` só na raiz do projeto.
  - **Justificativa:** `docs/okf.md`, passos 3 a 10.
  - **Alternativa Rejeitada:** reescrever pelo `WritePage` (criaria versões novas e quebraria "sem churn").

- **Decisão (K-04): detecção de contêiner por decisão nossa.** O doc diz "dentro de contêineres" sem dizer como detectar. Decisão nossa: presença de `/.dockerenv` ou de `container` em `/proc/1/cgroup` no Linux; no Windows, nunca contêiner.
  - **Justificativa:** o destino padrão depende disso; o doc é omisso sobre o método.
  - **Alternativa Rejeitada:** variável de ambiente própria (o doc não define uma).

- **Decisão (K-05): exportação somente leitura.** A rota lê a árvore do projeto, aplica as exclusões, exige `type` não vazio em cada página (falha inteira identificando a página), gera o `index.md` (frontmatter `okf_version: "0.2"`, título fixo e um link por diretório de primeiro nível em ordem alfabética), deriva `title` e `description` na cópia, reescreve os links internos e grava o tarball direto na resposta. Nada muda na wiki.
  - **Justificativa:** `docs/okf.md` § Exportação.
  - **Alternativa Rejeitada:** exportar pelo banco (o pacote é a cópia dos arquivos, não do índice).

- **Decisão (K-06): reescrita de links com o mesmo extrator da 001.** Os wikilinks do próprio projeto viram links markdown relativos ao arquivo de destino (rótulo como texto; sem rótulo, o alvo; `.md` acrescentado; âncora e query preservadas); outros projetos ficam `[[...]]`; blocos de código cercados e código inline são pulados; destinos inexprimíveis vão entre `<...>` ou ficam literais.
  - **Justificativa:** `docs/okf.md` § "Reescrita de links no pacote"; reaproveita a gramática de alvo única da 001.
  - **Alternativa Rejeitada:** regex sobre o texto inteiro (reescreveria código).

- **Decisão (K-07): reparo de `stale_after` fora do executor.** Roda a cada início, depois das migrações; reescreve linha e arquivo preservando id, versão, `updated_at` e `generated.at`; commit com a mensagem do doc se algo mudou; falha vira aviso.
  - **Justificativa:** `docs/okf.md` § "Reparos de lojas já migradas" (não é migração registrada).
  - **Alternativa Rejeitada:** registrar como migração (builds anteriores recusariam a wiki).

- **Decisão (K-08): contratos de teste.** Propriedades de `docs/okf.md` § Verificação; pré, pós e reexecução da migração em diretório temporário; recusa de formato mais novo; trava da cópia (falha de gravação não altera nada); exportação com ledger, página não conforme e links de cada tipo.
  - **Justificativa:** os dois docs listam esses testes.
  - **Alternativa Rejeitada:** testar só a exportação.

### O que não construir nesta feature
Diálogo web de aviso (014), importação (não existe no doc), conformidade na escrita (002).

## 4. Auditoria de Constituição

**Portões de Qualidade**
- [ ] Build cruzado, `go vet`, `go test -race` — K-08.
- [ ] Gherkin — os cenários da spec viram o `.feature` da 006.
- [ ] Dependência nova — nenhuma.

**Restrições Arquiteturais**
- [ ] Grafo de imports — executor e migração no `wiki`; cópia prévia e rota no `cli`.
- [ ] INV-01 — `AI_MEMORY_BACKUP_DIR` só em `internal/config` (K-02).
- [ ] INV-02 — registro e passo do banco pelo escritor único (K-01, K-03).
- [ ] INV-09 — a cópia prévia roda depois da trava de instância única.
- [ ] INV-10 — arquivos reescritos pela escrita atômica; recibo por substituição atômica.
- [ ] INV-13 — migrações sem LLM.
- [ ] Mutações da wiki só pela camada `wiki` — K-03 e K-07.
- [ ] Markdown é a fonte da verdade — exportação lê arquivos (K-05).

**Anti-Padrões Proibidos**
- [ ] Sem abstração preemptiva — `WikiMigration` tem um item real.
- [ ] "A confirmar" como decisão — K-04 (detecção de contêiner).
