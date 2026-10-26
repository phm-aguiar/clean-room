# Consolidação, manutenção e curadoria da memória

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

> **Papel:** manutenção do wiki sem modelo de linguagem. Mantém a memória saudável
> com varredura de esquecimento, compactação extrativa, lint por regra e curadoria.
> Todas as passadas deste documento funcionam sem provedor.

**Dependências lógicas:** camada de armazenamento (leitura e escrita), camada de wiki
e normalização Unicode.

> O *quando* (agendadores e ticks) pertence à camada de servidor e de linha de
> comando. Este documento define o *o quê* de cada passada, sem estado global.

## Áreas funcionais

| Área | Função |
|---|---|
| Varredura de esquecimento | TTL, decaimento, compactação, remoção de marcadores e poda de observações. |
| Compactação e palavras-chave | Rebaixamento extrativo de tier (A2). |
| Lint por regra | Achados determinísticos, sem modelo. |
| Curador | Relatório conservador de sinais de manutenção. |

---

## 1. Varredura de esquecimento

Entradas:

- parâmetros de decaimento, incluindo o limiar de frio e `hard_delete_after_days`
  (padrão 180);
- `breadth_weight`, finito e maior ou igual a zero;
- retenção de observações: `days` maior ou igual a zero, e `batch`;
- flag de compactação de episódicas frias;
- `dry_run`;
- wiki opcional.

**Classificação:**

- **Expirada:** `expires_at` menor ou igual ao momento atual (TTL).
- **Fria:** somente episódica, não fixada, com score de retenção abaixo do limiar frio.
  O score combina tier, idade, número de acessos, tempo desde o último acesso, relevância
  e amplitude de acesso. Páginas semânticas, procedurais e de trabalho não decaem nesta
  passada.

**Ordem de execução.** Em `dry_run`, só o plano é produzido.

a. **TTL:** remove as páginas expiradas que ainda são a última versão. O TTL remove mesmo
   páginas fixadas. A remoção é **física e apaga todas as versões do caminho** (arquivo, linhas, vínculos, embeddings), sem
   deixar marcador. O lint avisa sobre essa combinação (`pinned_expiring`), mas só antes de a página expirar.
a'. **Deduplicação A3 de frias por agrupamento** (opt-in, desligada por padrão; depende de
   embeddings, então fica **fora do escopo sem LLM**; sem embeddings é inerte): agrupa o mesmo
   conjunto limitado de frias que as passadas b/c usariam (densidade mínima padrão **2**, teto de
   distância de cosseno **0,15**; valores 0 caem nesses padrões), funde cada grupo num
   sobrevivente cujo corpo une as palavras-chave do grupo e cita os membros, e nada é apagado
   fisicamente (proveniência em evidências). As páginas reivindicadas pela deduplicação são
   puladas pelas passadas seguintes. Aparece no relatório como "mescladas". Detalhes em
   design-memory-aging.md (registrado pela auditoria 002).
b. **Compactação A2** em lote, para as frias, quando a flag está ligada. Páginas já
   compactadas são terminais: não são recompactadas nem removidas.
c. **Remoção por decaimento** (quando a compactação está desligada): remove a página e
   deixa um marcador de remoção, se ainda for a última versão. Se ela mudou desde a
   seleção, pula.
d. **Remoção definitiva de marcadores** mais antigos que `hard_delete_after_days`, junto
   com a cadeia de supersessão deles. Uma página recriada no mesmo caminho é religada à
   cadeia e não é apagada.
e. **Poda de observações** (opt-in, com `days` maior que zero): remove, em lotes (padrão
   de 5 000 linhas por transação), as observações de sessões cujo resumo ainda é uma página
   viva e que não têm trabalho de consolidação pendente nem novas observações desde o último fim de sessão (predicado completo
   em [envelhecimento de memória](design-memory-aging.md) §5.4). Roda após as demais passadas de página de propósito. As remoções desta mesma execução já tiram
   sessões da lista. **Irreversível:** uma sessão podada nunca mais pode ser reprocessada a partir das observações.
f. **Poda de ledgers de eventos** (opt-in, `ledger_retention_months` maior que zero): apaga do disco os arquivos mensais
   `log-AAAA-MM.md` cujo mês terminou há pelo menos esse número de meses completos (UTC); nunca o mês atual nem o anterior.
   Roda por último, também lista em `dry_run`, e não mexe no índice (ver §5.7 do documento de envelhecimento).

Sem wiki, as mutações que dependem dela são recusadas, com aviso. A recusa é uma falha
segura, porque a reconciliação reindexaria a partir do armazenamento. A poda de
observações não depende de wiki, pois observações não têm arquivo.

Falhas por página viram aviso e são retentadas na próxima varredura. Nenhuma aborta a
execução.

Relatório: candidatas avaliadas, removidas por decaimento, compactadas, mescladas,
expiradas, removidas definitivamente, observações podáveis (contadas também em
`dry_run`), observações podadas, sessões tocadas, lotes de poda e os seis contadores de ledgers
(`ledgers_prunable`, `ledger_bytes_prunable`, `ledgers_pruned`, `ledger_bytes_pruned`, `ledgers_undeleted`, `ledger_page_rows_left`).

---


## 2. Palavras-chave e compactação

### 2.1 Palavras-chave (keep-tokens)

Extração sem LLM, por padrões, na ordem de maior sinal para menor:

1. URLs `http` e `https`;
2. caminhos com barras e extensão;
3. nomes de arquivo com extensão comum de código ou configuração;
4. códigos de erro (ex.: `E0433`) e códigos HTTP (ex.: `HTTP 500`);
5. trechos entre crases (conteúdo interno, até 120 caracteres);
6. constantes e variáveis de ambiente em `MAIÚSCULAS_COM_UNDERSCORE`;
7. identificadores em `snake_case` ou `camelCase`.

Regras: pontuação de borda (`. , ; : ( ) " ' < >` e crase) é removida. A ordem é por classe (a ordem acima) e, dentro da classe,
a da primeira ocorrência, sem repetição. Máximo de 48 tokens. Tamanho entre 2 e 128 **bytes** (UTF-8) depois de aparar.
Os padrões exatos (limites de palavra Unicode, classes de caracteres, extensões aceitas) estão em
[envelhecimento de memória](design-memory-aging.md) §6.1.

### 2.2 Compactação (A2, rebaixamento de tier)

Constrói a versão compactada a partir do frontmatter e do corpo:

- **Frontmatter:** preservado, com `compacted: true` acrescentado. O resumo de nível L0
  (`abstract`) é mantido.
- **L1:** o resumo explícito do frontmatter (`summary` ou `abstract`). Se não houver, o
  primeiro parágrafo não vazio que não seja título (lido linha a linha; uma linha de título encerra o parágrafo). É colapsado em uma linha e limitado a
  500 **bytes** (em fronteira de caractere), com reticências ao ser cortado. O resumo vindo do frontmatter só é aparado, não colapsado.
  Detalhes e formato exato do corpo em [envelhecimento de memória](design-memory-aging.md) §6.
- **L2:** seção "Retained facts", com as palavras-chave em crases, uma por item. As
  palavras-chave vêm do corpo completo, inclusive de parágrafos descartados.
- **Rodapé fixo:** informa que o corpo completo continua no histórico git e na cadeia de
  supersessão, e que pode ser restaurado.
- Totalmente sem LLM e reversível pelo histórico.


## 3. Lint e curador

### 3.1 Lint

**Saída:** relatório em `_lint/report.md`, uma página por projeto, regravada a cada
execução. Sem achados, a página é removida. Relatórios datados antigos (`_lint/AAAA-MM-DD.md`)
são podados a cada execução que não seja simulação.

Cada achado tem tipo, severidade (`info` ou `warning`), mensagem e lista de páginas (e um `detail` opcional, nunca preenchido pelas regras).

**A página de relatório** é uma página comum, gravada pela camada wiki como ator anônimo e operação de admissão `consolidate`,
com tier `semantic`, não fixada, título `Lint report AAAA-MM-DD` (data UTC do dia) e frontmatter `{title, tier: semantic, kind: lint-report}`.
Corpo: `# Lint findings`, linha em branco, `N finding(s).`, linha em branco e, para cada achado `i` (a partir de 1),
`## i — <tipo> (<severidade>)`, a mensagem, o `detail` se houver, e (se houver páginas) `Pages:` seguido de uma linha ``- `caminho` `` por página, cada bloco terminado por linha em branco.
Como a gravação é supersessão, repetir o lint no mesmo dia sem mudança de achados não cria versão nova. Sem achados: a página `_lint/report.md` é apagada (exclusão física, se existir).
Também a cada execução que não seja simulação são apagadas as páginas legadas `_lint/AAAA-MM-DD.md` (nome exatamente nesse formato, dígitos e hífens).
`dry_run` não grava, não apaga e não poda nada. O lint só roda com wiki.

**Ordem dos achados no resultado:** regras por página (na ordem das páginas atuais: `stale`, `rule_suggestion`, `pinned_expiring`), depois `duplicate` (ordem de grupos não especificada no original; reproduzir sem ordenação adicional), `broken_link`, `contradiction` (arestas declaradas), `feedback_flagged`, `contradiction` (faixa de similaridade, só com vetores) e, por último, os do LLM.
**Páginas expiradas (`expires_at` no passado) são ignoradas por todas as regras por página.**

**Achados por regra (sem LLM):**

- `stale` (info): episódica com `access_count = 0` e idade (desde `updated_at`) **maior que** o limiar. O limiar é 0,6 dividido
  pelo λ escalar de decaimento, se λ for finito e positivo; caso contrário, 30 dias. Mensagem:
  `Episodic page <caminho> is <N> days old with zero accesses` (N = idade arredondada para inteiro, `{:.0}`).
- `duplicate` (warning): mesmo `title` **do frontmatter** (minúsculas Unicode, sem aparar nem colapsar espaços; título vazio ou só com espaços não conta)
  em dois ou mais caminhos distintos. Mensagem: `Multiple pages share title "<título em minúsculas>"` (entre aspas duplas). Páginas: os caminhos do grupo.
- `rule_suggestion` (info): página com frontmatter `kind` igual a `rule` ou caminho começando por `_rules/`. Mensagem:
  `Page <caminho> looks like a durable project rule. Consider copying it into your project's CLAUDE.md / AGENTS.md so the agent sees it on every turn, not just when it remembers to call memory_query.`
- `pinned_expiring` (warning): página com `expires_at` e fixada (coluna ou `pinned: true` booleano no frontmatter), ainda não expirada.
  O TTL prevalece na varredura. Mensagem: `Page <caminho> is pinned but carries an expires_at TTL; the forget sweep will hard-delete it when the TTL passes despite the pin. Remove one of the two.`
- `broken_link` (warning): link (de qualquer rótulo) de página atual não expirada para outro projeto (alvo com qualificador de projeto) cujo alvo não
  resolve para página atual não expirada. Se o projeto existe: `Page <origem> links to <alvo> but that page does not exist in project `<projeto>` (missing, renamed, or deleted) — a broken cross-project dependency`;
  se não existe: `Page <origem> links to <alvo> but project `<projeto>` does not exist (typo or wrong name)`. `<alvo>` = `[workspace/]projeto:caminho`. Ordem: origem, projeto, caminho. "Outro projeto" = qualquer alvo gravado **com** qualificador de projeto, inclusive um que nomeia o próprio projeto; alvos sem qualificador (mesmo projeto) nunca entram nesta regra. A existência do projeto compara nome de projeto e de workspace (workspace ausente = o da página de origem) de forma exata (sensível a maiúsculas).
- `feedback_flagged` (warning): feedback `stale` ou `wrong` ainda anexado à versão atual não expirada; um achado por (página, sinal).
  Repete até a página ser reescrita. Mensagem: ``Page <caminho> was flagged `<sinal>` (<n> signal|signals, latest <instante ISO-8601 UTC>)`` seguida de `: <motivo mais recente>` quando houver motivo. Ordem: sinal mais recente primeiro, depois caminho, depois sinal.
- `contradiction` (warning): aresta declarada `contradicts` de página atual não expirada. Resolvida (alvo atual e não expirado):
  `Page <origem> declares it contradicts <alvo> — reconcile them or supersede the outdated one`; não resolvida:
  `Page <origem> declares it contradicts <alvo>, which does not resolve to a page (deleted or renamed) — the declaration is stale`. Páginas: `[origem, alvo]`. Ordem: origem, alvo. `<alvo>` é só o caminho gravado do alvo, **sem** o qualificador de projeto/workspace, mesmo quando a aresta é entre projetos. O achado é de toda aresta `contradicts` de página atual não expirada do projeto, com alvo em qualquer projeto.
- Mensagem de `duplicate`: o título entre aspas segue a convenção de escape de string (aspas e barras invertidas internas escapadas com `\`, caracteres de controle como `\n`/`\t`/`\u{..}`).
- `contradiction` (info, faixa de similaridade): ver o documento de envelhecimento §9; só no lint pedido pelo usuário e com vetores.

### 3.2 Curador

Parâmetros: máximo de 25 achados por tipo; 7 dias para o foco atual
(`_slots/current-focus.md`); 30 dias para os demais slots; parâmetros de decaimento.

Sinais (somente relatório; o limite de 25 vale por tipo; a ordem dos grupos é: `cold_episodic`, `stale_slot`, `duplicate_title`, `dangling_cross_project_link`):

- `cold_episodic` (info): episódica, não fixada (coluna ou frontmatter), não expirada, **sem marca de compactação**, com retenção abaixo do limiar frio
  (mesma pontuação da varredura, inclusive amplitude). Mensagem: `Episodic page <caminho> is cold (score <3 casas>, age <N> days)`, `detail` `{score, threshold, age_days, access_count}`. Ordenados pela mensagem (isto é, pelo caminho).
- `stale_slot` (warning): página sob `_slots/` (não expirada) cuja última alteração é mais antiga que o limiar: 7 dias se o nome do arquivo do slot é `current-focus.md`
  (direto em `_slots/` ou em subpasta de operador `_slots/<operador>/current-focus.md`), 30 dias nos demais. Páginas de `_pending/` são
  ignoradas. Mensagem: `Slot <caminho> has not changed for <N> days (threshold <T>)` (números `{:.0}`), `detail` `{age_days, threshold_days}`. Ordenados por caminho.
- `duplicate_title` (info): duas ou mais páginas atuais não expiradas (fora de `_pending/`) com a mesma coluna de título após colapsar espaços e passar para minúsculas
  (título vazio ignorado). Mensagem: `<n> pages share the normalized title '<título>'`; páginas ordenadas; achados ordenados pela mensagem. `detail` `{normalized_title}`.
- `dangling_cross_project_link` (warning): mesma consulta do `broken_link`, na ordem origem/projeto/caminho. Mensagem: `<origem> links to missing cross-project target <workspace>/<projeto>/<caminho>`
  (workspace omitido = o do projeto revisado), `detail` `{target, project_exists}`.

Resposta do relatório: `{workspace, project, generated_at (ISO-8601 UTC), dry_run: true, summary, params, findings}`; `summary` é `No conservative curator findings.` ou `<n> conservative curator finding(s).`.
Os parâmetros padrão são fixos (25, 7, 30 e os `[decay]` do servidor). Falha se `breadth_weight` for negativo ou não finito.

Por padrão, só devolve o relatório, em modo simulação. Com o modo de staging, grava uma
página com o relatório.

---

---

## 3.3 Reorganização (`reorg`)

Operação administrativa (`POST /admin/reorg`, corpo `{dry_run}` com padrão **false**: grava; CLI `reorg`) que reatribui sessões antigas ao projeto do seu diretório.
Não tem parâmetro de workspace: age **sempre no workspace `default`** (que é criado se não existir).

1. Lê as sessões do workspace com `cwd` não vazio, por `started_at`.
2. Para cada `cwd` distinto, o projeto de destino é o **último componente do caminho** (`unknown` se não houver nome de arquivo, por exemplo raiz). Se esse nome for de escopo reservado
   (`scratch`, `_global`, `_profile`, `_profile.*`), o `cwd` é ignorado. Senão o projeto é obtido ou **criado** (com o caminho de repositório quando o `cwd` é a raiz de um repositório git).
   **A criação ocorre também em `dry_run`** (o plano já resolve os projetos).
3. O plano lista só as sessões cujo projeto atual é diferente do destino: `{session_id, cwd, new_project}`. `distinct_new_projects` = projetos de destino distintos do plano.
4. Se `dry_run` ou o plano está vazio: responde `{dry_run, plan, summary}` com `sessions_moved`, `observations_updated`, `pages_graveyarded` iguais a 0.
5. Senão, **numa transação**: (a) atualiza `project_id` de cada sessão do plano (e conta `sessions_moved`); (b) atualiza `project_id` de **todas** as observações dessas sessões (`observations_updated`);
   (c) "cemitério": fecha os vínculos de entidade abertos de todas as páginas atuais do workspace e marca **todas as páginas atuais do workspace inteiro** (todos os projetos) com `is_latest = 0` e `valid_to` = um único instante
   (`pages_graveyarded`), **sem** `superseded_at` e **sem tocar nos arquivos**. O objetivo é que a próxima consolidação regere páginas limpas por projeto. Não é o diretório `_graveyard/` das migrações de wiki.
6. Resposta: `{dry_run: false, plan, summary{sessions_moved, observations_updated, pages_graveyarded, distinct_new_projects}}`.

## 3.4 Limpeza de versões de ledger (`reclaim-ledger-versions`)

`POST /admin/reclaim-ledger-versions` `{confirm, dry_run, drop_latest (false), compact (false)}` — **global** (todos os workspaces e projetos, sem escopo).
Sem `dry_run` e sem `confirm`: HTTP 400. Remove as linhas de índice de versões **substituídas** de ledgers; nunca arquivos.

- Candidatas: linhas com `is_latest = 0`, `superseded_at` NULO (as que a varredura não alcança) e caminho exatamente `log.md` ou `log-DDDD-DD.md` (dígitos), cujo corpo, nos primeiros 8 KiB,
  passa no reconhecimento de ledger ([complementary-contracts.md](complementary-contracts.md) §4). Uma página comum com nome parecido nunca é candidata.
- Com `drop_latest`: apaga também a linha atual de cada ledger aprovado pelo mesmo critério (padrão: mantém).
- Contadores: `ledger_paths` (trincas workspace/projeto/caminho distintas com resíduo, sem contar as linhas atuais), `pages_deleted` (candidatas + atuais se `drop_latest`), `bytes_deleted` (soma dos tamanhos de corpo, em bytes), `dropped_latest` (houve linha atual removida).
  Os mesmos números saem em `dry_run`.
- Execução: uma transação apaga as linhas (vínculos, embeddings, vínculos de entidade e feedback caem em cascata); o índice de texto é reconstruído por inteiro ao final.
  Com `compact`, e se algo foi apagado, executa `VACUUM` (exige espaço livre de aproximadamente o tamanho do banco) e `compacted = true`.
- Resposta: `{ledger_paths, pages_deleted, bytes_deleted, bytes_reclaimed (antes − depois, só com compactação), dropped_latest, bytes_before, bytes_after, compacted}`.

## 4. Notas para reimplementação

1. **Ordem sugerida:** (a) varredura sem compactação; (b) lint por regra; (c) compactação extrativa; (d) curador.
2. Toda reescrita é supersessão. As remoções possíveis são: TTL explícito; remoção definitiva de marcadores após o prazo de `hard_delete_after_days`; e poda de observações (opt-in, irreversível). Documente isso para o operador.
3. Mantenha desligados por padrão os experimentos de compactação até medir ganho de recall.
