# Conformidade com o formato OKF (2.0)

> **Papel:** descreve como as páginas da wiki e o pacote exportado seguem o formato Open Knowledge Format (OKF) na versão 0.2: versão do formato, estrutura do pacote, mapeamento de tipo e metadados, exportação, validação e migração das lojas existentes.

## Por que isso importa

A memória fica portável para fora do ai-memory. Um pacote de projeto pode ser entregue a outra pessoa que use uma ferramenta compatível com OKF, ou nenhuma ferramenta: ela lê decisões, armadilhas e procedimentos como arquivos markdown comuns, com metadados padronizados. Os arquivos podem ser pesquisados com ferramentas de texto, abertos em editores de notas e importados por qualquer ferramenta compatível com OKF.

```bash
ai-memory export-okf --project myproject -o myproject-bundle.tar.gz
```

Nada fica preso ao ai-memory: a exportação é uma cópia validada dos próprios arquivos em que a wiki já vive.

A partir da geração 2.0, a wiki é nativamente um pacote OKF. Cada página lida do disco já é um arquivo de conceito conforme, e o pacote de um projeto é o diretório da wiki desse projeto. "Nativo" significa que os arquivos da wiki *são* os arquivos OKF: nenhuma etapa de exportação cria uma segunda versão da verdade. O comando de exportação existe para levar pacotes a outras ferramentas; não há comando de importação dedicado (ver [Importação](#importação)).

## Versão do formato

- O build escreve e entende a versão **0.2**. Ela aparece como `okf_version: "0.2"` no frontmatter do `index.md` da raiz do pacote.
- A 0.2 substitui a 0.1 com duas mudanças incompatíveis: a chave `timestamp` virou `generated: {by, at}`, e a seção de citações no corpo virou a chave `sources` no frontmatter. Ela também acrescenta famílias de confiança, ciclo de vida e proveniência.
- Requisitos resumidos da especificação pública do formato, na versão 0.2:
  - todo arquivo `.md` que não seja nome reservado tem frontmatter YAML analisável com `type` não vazio;
  - o `index.md` da raiz declara `okf_version: "0.2"` e lista o diretório; o frontmatter permitido em `index.md` é só esse;
  - os nomes reservados `index.md` e `log.md`, quando presentes, seguem a estrutura da especificação;
  - consumidores devem tolerar chaves desconhecidas, e as chaves de extensão do ai-memory são compatíveis com isso.

## Estrutura do pacote

### Unidade: o projeto

Um diretório de escopo de projeto é um pacote. O diretório da wiki é separado por workspace e por projeto (por identificadores, não por nomes). Dentro de cada projeto ficam os diretórios de famílias de páginas descritos no [mapeamento de tipo](#chave-type-obrigatória).

### Arquivos no diretório da wiki

| Arquivo | Papel |
|---|---|
| `index.md` na raiz do projeto | Escrito pela migração quando ausente: frontmatter `okf_version` e listagem dos diretórios. A operação normal não o escreve. |
| `_meta.md` | Manifesto de identidade do escopo (nome do workspace ou do projeto). Não é página de conceito: recebe apenas `type`. |
| `log-AAAA-MM.md` na raiz do projeto | Registro bruto de eventos dos hooks, um arquivo por mês UTC. Cada linha tem o formato `## [instante] evento | título`, sem frontmatter. Não é conceito. |
| `log.md` | Nome reservado pelo formato. O ai-memory não o adota: o histórico do wiki é o próprio histórico de commits. |

### Pacote exportado

O pacote é um tarball gzip cuja raiz contém:

- **`index.md` gerado**, byte a byte: `---\nokf_version: "0.2"\n---\n\n# Bundle index — concept files by directory\n\n` seguido de uma linha ``- [<dir>/](<dir>/)`` + `\n` por **diretório de primeiro nível** do projeto, em ordem alfabética (ordem de bytes do nome), e nada mais. A lista inclui todo diretório de primeiro nível existente, tenha ou não arquivos `.md` (inclusive vazio ou só com arquivos que o pacote exclui), **exceto `_pending`**. O corpo não tem prosa fora da lista. (O `index.md` escrito pela migração é diferente; ver a Migração.)
- **Demais arquivos `.md`** do projeto, com caminho relativo à raiz do projeto, separador `/` em qualquer sistema, com a reescrita descrita em [Reescrita de links](#reescrita-de-links-no-pacote). Isso inclui `_meta.md` (manifesto), que precisa ter `type` e vai no pacote. O `index.md` é a primeira entrada; a ordem das demais no original segue a ordem de leitura do diretório (não definida); uma reimplementação deve ordenar por caminho. Cada entrada é um arquivo comum, modo `0644`, data de modificação zero, só entradas de arquivo (sem entradas de diretório), tarball gzip com cabeçalhos GNU.
- Entrega pela rota: resposta 200 com `Content-Type: application/gzip`, `Content-Disposition: attachment; filename="okf-bundle.tar.gz"`, corpo em fluxo a partir de um arquivo temporário. Falha de validação: HTTP **422** com `{"error": "<mensagem>"}`; workspace ou projeto inexistente: erro de requisição (a rota não cria escopo).

Excluídos do pacote:

- `index.md` em qualquer nível (o da raiz é regenerado; os de outros níveis são ignorados);
- `log.md` em qualquer nível, pelo nome, sem checar o conteúdo;
- `log-AAAA-MM.md` cujo conteúdo abre com entrada de registro. Um arquivo com esse nome que contenha uma página comum continua no pacote e precisa ser conforme;
- diretórios chamados `_pending` em qualquer nível (área de propostas, não conceitos);
- arquivos que não sejam `.md`.

## Mapeamento de páginas

### Leitura de frontmatter

- Um BOM no início do arquivo é descartado.
- O bloco de frontmatter é reconhecido com fins de linha LF ou CRLF. Um bloco com CRLF não vira corpo.
- O corpo é preservado byte a byte, inclusive os fins de linha.

### Chave `type` (obrigatória)

Regras aplicadas nesta ordem; vale a primeira que se aplicar:

1. `type` presente e não vazio: mantido. Nunca é sobrescrito. Um valor só com espaços conta como ausente.
2. `kind` com valor `fact`, `note`, `procedure` ou `decision`: vira `Fact`, `Note`, `Procedure` ou `Decision`. Outros valores de `kind` são ignorados.
3. `slot_kind` com valor `invariant` ou `state`: vira `Invariant` ou `State`.
4. Família pelo primeiro segmento do caminho relativo ao projeto:

| Família | `type` |
|---|---|
| `sessions/` | Session Summary |
| `_rules/` | Rule |
| `gotchas/` | Gotcha |
| `decisions/` | Decision |
| `procedures/` | Procedure |
| `concepts/` | Concept |
| `notes/` | Note |
| `runbooks/` | Runbook |
| `_slots/` | State |
| `_lint/` | Lint Report |
| `_pending/` | Pending Note |
| qualquer outra | Note |

O manifesto `_meta.md` recebe `type: Scope Manifest` quando não tem `type`, independentemente das regras acima.

### Demais chaves

| Chave | Origem | Regra |
|---|---|---|
| `title` | frontmatter existente | Mantido. Na exportação, se ausente ou vazio, é derivado nesta ordem: `title` do frontmatter (aparado, não vazio); primeira linha do corpo que começa com `# ` (com espaço), aparada e não vazia, **fora de blocos de código cercados** (uma linha `# ` dentro de cerca é comentário de código, não título; a regra de cerca é a de CommonMark usada na extração de links); nome do arquivo sem `.md`. |
| `description` | `summary` | Preenchido só se ausente e se `summary` não estiver vazio. Na exportação, se ausente, usa `summary` e, na falta dele, `abstract`. |
| `tags` | frontmatter existente | Mantido. A conformidade não o altera. |
| `generated` | ver [Proveniência](#proveniência-e-validade) | `by` e `at`. |
| `sources` | ver [Proveniência](#proveniência-e-validade) | Derivado de provenância de sessão. |
| `stale_after` | `expires_at` | Ver [Validade](#stale_after). |

Nenhum `status` é gravado pelo ai-memory; a ausência de `status` equivale ao padrão do formato.

### Chaves de extensão preservadas

`tier`, `kind`, `slot_kind`, `entities`, `pinned`, `consolidated`, `session_id`, `agent`, `summary` e `expires_at` são mantidas como estão. A conformidade não remove nem altera chaves de extensão.

## Proveniência e validade

### `generated.by`

- Se a página trouxer `generated_by_model`, esse valor vira `generated.by`.
- Caso contrário, vale `process:ai-memory/<versão do build>`.
- Um `generated.by` já presente não é sobrescrito. Se `generated` existir mas não for um mapeamento, é substituído por um mapeamento com `by`.
- Confirmado: nenhum produtor do original grava `generated_by_model`; só o leitor existe. Na prática `generated.by` é sempre `process:ai-memory/<versão>`, salvo valor já presente.

### `generated.at`

- Formato: UTC com segundos, `AAAA-MM-DDTHH:MM:SSZ`.
- A conformidade de frontmatter não preenche `generated.at`, porque ele muda por versão.
- Ao escrever uma página: se o conteúdo mudou, `generated.at` recebe o instante da escrita. Se o conteúdo não mudou, mantém o valor anterior. "Conteúdo" é o corpo e o frontmatter sem `generated.at`.
  - No arquivo: depois de conformar, o valor de `generated.at` é **sempre** substituído: pelo do arquivo atual quando corpo e frontmatter (sem `generated.at`) são iguais aos do arquivo em disco, senão pelo instante atual. Um `generated.at` trazido pelo chamador, portanto, não sobrevive à escrita pela camada wiki.
  - Na linha do banco: o `generated.at` já presente no frontmatter conformado é mantido; só se ausente recebe o instante da escrita (a camada wiki o define antes, para arquivo e linha ficarem iguais).
- Na migração, a linha do banco recebe o `updated_at` da própria linha, em segundos UTC. Um arquivo cuja linha não foi reescrita recebe a data de modificação do arquivo, em segundos UTC.

### `sources`

- Criado só quando a página não tem `sources` e tem `session_id`.
- Contém um item com `resource: ai-memory://session/<session_id>`.
- Quando a página tem `agent`, o item também recebe `author: process:<agent>`. O prefixo `process:` é usado porque o nome do agente sozinho não atende à gramática de atores do formato, e o build não rastreia versão por agente.

### `stale_after`

Criado só quando a página não tem `stale_after` e tem `expires_at` analisável:

- data e hora com deslocamento de fuso (qualquer instante que o leitor de instantes RFC 3339 aceite): copiada **literalmente**, sem aparar espaços;
- data pura `AAAA-MM-DD`: vira o fim desse dia em UTC, `AAAA-MM-DDT23:59:59.999999Z`;
- qualquer outro valor: nenhum `stale_after` é criado.

`expires_at` nunca é alterado.

**Reparo:** um `stale_after` exatamente igual a um `expires_at` de data pura (e que não seja data e hora com fuso) é reescrito para o instante acima. Um `stale_after` diferente de `expires_at` não é tocado, porque não foi derivado pelo ai-memory.

## Determinismo e idempotência

- Conformar o mesmo frontmatter duas vezes produz exatamente o mesmo resultado; a segunda passada não altera nada.
- Na escrita, uma página não gera nova versão quando corpo, frontmatter (sem `generated.at`), título, tier e fixação são iguais aos da versão atual. Nesse caso, a versão atual é mantida e o `generated.at` original preservado.

## Aplicação na escrita

Toda criação de versão de página passa pela normalização no ponto único de escrita da camada de armazenamento. A camada de wiki aplica a mesma regra ao arquivo em disco, e herda o `generated.at` quando o conteúdo não mudou. Assim, o arquivo e a linha do banco ficam alinhados.

## Exportação

### Comando

```bash
ai-memory export-okf --project <nome> -o <arquivo.tar.gz>
```

| Opção | Tipo | Obrigatória | Padrão |
|---|---|---|---|
| `--workspace` | texto | não | `default` |
| `--project` | texto | sim | — |
| `-o`, `--to` | caminho | sim | — |

- O comando pede o pacote ao servidor configurado, por `POST /admin/export-okf?workspace=…&project=…`, e grava o arquivo recebido. Por isso, exige servidor em execução.
- Diretórios pais do destino são criados se faltarem.
- Ao final, imprime o caminho e o tamanho do arquivo em bytes.

### Regras no servidor

- Falha se o projeto ainda não tem diretório de wiki.
- Toda página `.md` que não esteja nas exclusões precisa ser conforme; a conformidade aqui exige apenas `type` não vazio (ao contrário da migração, que também exige `generated.at`). Se alguma não for, a exportação inteira falha com a mensagem `page <caminho relativo> is not OKF-conformant; run the server once to migrate before exporting` (o caminho vem na grafia do sistema operacional, com `\` no Windows). Frontmatter ilegível conta como não conforme; arquivo que não é UTF-8 válido falha a exportação com erro de leitura. Nenhum arquivo é entregue.
- A cópia de cada página no pacote é **reserializada** (frontmatter emitido de novo, corpo com os links reescritos); a ordem das chaves e a formatação YAML podem diferir do arquivo original, e só o significado do frontmatter é garantido.
- Preenchimento na cópia: `title` quando ausente ou vazio; `description` quando ausente ou vazia (de `summary` não vazio, senão `abstract` não vazio).
- A exportação não altera nada no diretório da wiki. A cópia do pacote recebe apenas os ajustes de título, descrição e links descritos aqui.

### Reescrita de links no pacote

- Links internos `[[alvo]]` e `[[alvo|rótulo]]` que apontam para o próprio projeto (sem qualificador, ou com `projeto:` igual ao nome do projeto exportado e sem workspace) viram links markdown `[texto](destino)` com destino **relativo ao diretório da página de origem** (`..` para subir, `/` como separador). O alvo é relativo à raiz do projeto, ganha `.md` quando necessário e tem `..`/`.` colapsados; âncora (`#`) e query (`?`) são separadas antes e coladas de volta ao destino. O texto do link é o rótulo aparado (se não vazio); sem rótulo, é o alvo exatamente como escrito (aparado, com qualificador se houver). No texto, `\`, `[`, `]`, `(` e `)` recebem `\` antes. Quando o projeto exportado é `_global`, o qualificador `_global:` também é local. O varrimento é por linha, preserva o fim de linha, e `[[` sem `]]` na mesma linha fica intacto.
- Links para outro projeto ou outro workspace ficam como `[[...]]` literal, porque não há equivalente relativo entre pacotes.
- Conteúdo dentro de blocos de código cercados ou de código inline não é alterado.
- Alvos vazios ou malformados ficam intactos.
- Destinos que não podem ser escritos como link markdown (espaços, caracteres de controle e parênteses não balanceados, entre outros) vão entre `<…>`. Se nem assim forem expressáveis, o link fica como `[[...]]` literal.

## Migração das lojas existentes

É uma migração de estrutura do wiki (ver [Migrações de estrutura do wiki](wiki-migrations.md)). Ela é registrada uma única vez e roda no início do servidor, nesta ordem fixa. Cada etapa só roda se a anterior tiver terminado.

1. **Cópia de segurança antes do schema do banco.** Antes de o banco de dados ser aberto (o que avança o schema), o servidor toma a cópia se o wiki tiver páginas não conformes e não houver um recibo com cópia ainda presente. Se a cópia falhar, o servidor não sobe.
2. **Abertura do banco e migrações de schema.** Só depois disso roda a migração de conformidade OKF.
3. **Verificação de pendências.** Se nenhuma linha do banco nem nenhum arquivo precisa de conformidade, a migração termina sem efeito, sem cópia, sem commit e **sem criar `index.md`**. É o caso de instalação nova ou de loja já conforme. Duas definições distintas de "precisa":
   - **linha do banco** (só versões atuais): precisa se conformar o frontmatter muda qualquer chave (`type`, `description`, `stale_after`, `sources`, `generated.by`) **ou** não há `generated.at`. Frontmatter ilegível no banco conta como vazio;
   - **arquivo**: precisa se não tem `type` não vazio ou, exceto `_meta.md`, não tem `generated.at`. A varredura é a da etapa 7.
   Por isso o gatilho da cópia da etapa 1 (só arquivos) pode ser mais estreito que o da migração (arquivos ou linhas).
4. **Cópia verificada.** Reaproveita a cópia da etapa 1 se ela existir e estiver presente. Caso contrário, cria uma. Se a cópia não puder ser criada ou verificada, a migração aborta antes de tocar em qualquer dado e é tentada de novo no próximo início.
5. **Checkpoint.** Abre (ou inicializa) o repositório git do wiki, confere a árvore do git e faz commit com a mensagem `pre-okf-migration checkpoint` do estado atual, antes de qualquer alteração.
6. **Passo do banco.** Para cada versão atual de página não conforme, o frontmatter é conformado no lugar: mesmo id, mesma versão, `updated_at` e corpo intocados, e `generated.at` derivado do `updated_at`. Versões históricas não são tocadas.
7. **Passo dos arquivos.** Para cada arquivo `.md` não conforme, o frontmatter é conformado e o corpo fica intacto. Um arquivo é não conforme quando não tem `type` não vazio ou, exceto para `_meta.md`, quando não tem `generated.at`. `_meta.md` recebe apenas `type`. A reescrita é feita só se o conteúdo mudou.
8. **`index.md` por projeto.** Cada diretório `<workspace>/<projeto>` da wiki (qualquer um, inclusive os que a migração não tocou) sem `index.md` na raiz recebe um, escrito por substituição atômica, com exatamente: `---\nokf_version: "0.2"\n---\n\n# Bundle index\n\nConcept files live in these directories:\n\n` e uma linha ``- [<dir>/](<dir>/)`` + `\n` por **subdiretório direto** do projeto (todos, inclusive `_pending`), em ordem alfabética. Um `index.md` existente nunca é alterado.
9. **Um único commit**, com a mensagem `okf-migration: conform wiki to OKF v0.2`.
10. **Registro** da migração como aplicada.

Ao varrer os arquivos, a migração ignora `index.md` em qualquer nível, ignora registros de ledger pelo critério de conteúdo (nome `log.md` ou `log-AAAA-MM.md` em qualquer nível **e** conteúdo de ledger), ignora o diretório `.git`, e ignora `_pending` apenas na raiz do projeto, isto é, no caminho `<workspace>/<projeto>/_pending` (em níveis mais fundos, `_pending` é varrido). Considera só arquivos com extensão exatamente `md`. Arquivo que não é UTF-8 válido é ignorado em silêncio. Arquivo cujo frontmatter não é YAML válido conta como sem frontmatter (não conforme) e, ao conformar, todo o conteúdo bruto vira corpo. A reescrita preserva o corpo byte a byte (um BOM inicial é descartado, não movido para o corpo; fins de linha CRLF do bloco de frontmatter são aceitos) e usa escrita atômica. O `generated.at` de um arquivo vem do `updated_at` da linha reescrita no passo do banco (segundos UTC), senão da data de modificação do arquivo (segundos UTC), senão de 1970-01-01T00:00:00Z. Os arquivos `_meta.md` recebem apenas `type: Scope Manifest` (se faltar). Já a exportação ignora `_pending` em qualquer nível.

**Ordem do boot do servidor (completa):** trava de instância única; cópia prévia (só arquivos não conformes e sem recibo válido; lê `AI_MEMORY_BACKUP_DIR` sem tratar valor vazio como ausente neste ponto, ao contrário da migração, que ignora valor vazio ou só com espaços); abertura do banco e migrações de esquema; correção de caminhos de repositório; validações de credenciais; migrações da wiki; preenchimento dos manifestos `_meta.md` ausentes (falha vira aviso); reparo de `stale_after` (falha vira aviso); checkpoint de linha de base da atualização; início do observador. O `okf_migrated` do status é verdadeiro quando algum nome em `wiki_migrations` contém `okf`.

### Cópia de segurança

- **Destino:** o valor de `AI_MEMORY_BACKUP_DIR`, quando definido. Sem ele, dentro de contêineres, a pasta `backups/` dentro do diretório de dados. Fora de contêineres, a pasta pessoal do usuário. O destino não pode ser a raiz do diretório de dados.
- **Arquivo:** `ai-memory-backup-okf-v0.2-AAAAMMDD-HHMMSS.tar.gz`.
- **Conteúdo:** todo o diretório de dados, exceto a própria cópia, **a subárvore do diretório de destino quando ele fica dentro do diretório de dados**, o arquivo temporário do recibo (`pre-migration-backup.json.tmp`) e os arquivos de trava do servidor (qualquer arquivo cujo nome começa com `.serve.lock`, inclusive `.serve.lock.holder`; no Windows a trava exclusiva obrigatória impediria a leitura do próprio arquivo, então ele é pulado por nome). O recibo `pre-migration-backup.json`, se existir, entra na cópia. Só entradas de arquivo (sem entradas de diretório), com caminhos relativos ao diretório de dados. Antes de criar o arquivo, a árvore `wiki/` e depois todo o diretório de dados são inspecionados: qualquer link simbólico ou ponto de reparse (inclusive junção do Windows) faz a cópia falhar sem criar arquivo.
- **Nome e destino:** o instante do nome é UTC (`%Y%m%d-%H%M%S`); o rótulo é `okf-v0.2`. O diretório de destino é criado se faltar (falha se o caminho existe como arquivo ou não pode ser criado, sem recuo para outro destino) e comparado em forma canônica com o diretório de dados (igual = erro).
- **Verificação:** o arquivo é relido. O número de entradas tem de bater com o número de arquivos gravados, e o tamanho tem de ser maior que zero. Se não bater, o arquivo é apagado e a migração aborta.
- **Recibo:** `pre-migration-backup.json` na raiz do diretório de dados (JSON indentado) com `archive_path` (absoluto), `size_bytes`, `entries`, `created_at` (`AAAA-MM-DDTHH:MM:SSZ`, UTC), `label` e `dest_free_bytes` (espaço livre no destino em bytes, nulo se ilegível, nunca falha a cópia). É gravado em arquivo temporário e renomeado sobre o definitivo.
- **Reuso:** um recibo existente, com arquivo ainda presente, é reaproveitado. Uma cópia válida nunca é sobrescrita.

### Trava contra formato mais novo

Se a tabela de migrações registrar um nome que este build não conhece, o wiki não é aberto. O erro indica que o formato é mais novo que o build, e lista as opções de recuperação. Isso impede que um build antigo misture formatos.

### Falhas

Qualquer erro em uma etapa faz o servidor não subir. A migração não é registrada, e será tentada de novo no próximo início.

### Idempotência

Uma segunda execução não migra nenhuma página e não cria segunda cópia. Depois de registrada, a migração não roda de novo.

### Aviso na página inicial

Enquanto existir um recibo, a página inicial mostra um diálogo único explicando a atualização para o formato 2.0:

- Se a cópia ainda existe: mostra o caminho do arquivo, o tamanho, a data de criação, os passos para reverter e a orientação de apagar a cópia depois de conferir.
- Se a cópia já foi apagada: informa que ela não existe mais e aponta o checkpoint de commit anterior à migração como referência.

O diálogo é exibido em toda carga da página enquanto o recibo existir, inclusive depois de apagar a cópia. Pode ser dispensado por navegador, e a dispensa fica associada à data de criação do recibo. O comando de status mostra a cópia enquanto ela estiver em disco.

### Reversão

- **Restauração completa:** parar o servidor, mover o diretório de dados atual para lado, desempacotar a cópia como diretório de dados e iniciar a versão anterior.
- **Reversão cirúrgica:** voltar o wiki ao checkpoint anterior à migração e rodar a reindexação (comando `reindex`, ver [CLI](cli.md)).

## Reparos de lojas já migradas

A cada início do servidor, depois das migrações, roda um reparo idempotente do `stale_after` copiado de um `expires_at` de data pura (regra descrita em [Validade](#stale_after)):

- Reescreve a linha do banco e o arquivo, preservando id, versão, `updated_at` e `generated.at`.
- Se algum arquivo mudou, cria um commit com a mensagem `okf: repair date-only stale_after on existing pages`.
- Falha no reparo não impede o servidor de subir; é registrada como aviso.
- Uma execução seguinte, sem nada a reparar, não altera nada.

O reparo não é uma nova migração registrada. Registrar um nome novo faria builds anteriores recusarem o wiki (ver [Trava contra formato mais novo](#trava-contra-formato-mais-novo)), e o reparo não precisa disso. Qualquer reescrita posterior de uma página (restauração, edição manual ou reindexação) também aplica a mesma correção.

## Importação

Não há comando de importação. Para trazer um pacote de outra ferramenta, desempacote os arquivos de conceito no diretório do projeto na wiki e deixe a reindexação ingerir (automática ou com o comando `reindex`). Confirmado: o observador de arquivos (eventos com atraso de 300 ms) e a reconciliação a cada 30 s indexam arquivos `.md` novos de um projeto que já tem linha no índice; um diretório de projeto sem linha é ignorado.

## Verificação

As propriedades abaixo têm verificação automatizada:

- **Ida e volta:** emitir um documento e lê-lo de volta produz o mesmo documento.
- **Conformidade da loja migrada:** toda página migrada tem frontmatter analisável, `type` não vazio e `generated.at`; o `index.md` da raiz declara `okf_version`.
- **Sem churn:** após a migração, os ids e os `updated_at` das versões atuais são idênticos aos de antes.
- **Idempotência:** uma segunda execução não migra nada nem cria segunda cópia.
- **Trava da cópia:** se a cópia não puder ser gravada, a migração recusa rodar e não altera linhas nem arquivos.
- **Aviso na página inicial:** mostra o caminho gravado no recibo e se adapta quando a cópia já não existe.
- **Exportação:** o pacote gerado passa na verificação de conformidade; uma página não conforme faz a exportação falhar; os registros de ledger não aparecem no pacote; os links internos são reescritos.
- **Regressão de recuperação:** a linha de base do benchmark LongMemEval deve ser reexecutada sem queda material. a confirmar: que isso é um gate de release automatizado.
