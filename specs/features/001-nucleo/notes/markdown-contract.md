# Nota: contrato de frontmatter, OKF, título e links

Tarefa T004, feature 001-nucleo. Somente leitura. Fontes: `docs/okf.md`, `docs/wiki-files-layer.md` (§3, §5.2), `docs/domain-vocabulary.md` (§4.2, §6.6, §6.7, §6.8, §14), `docs/database-schema.md` (tabela `links`, linhas 188-210), `specs/features/001-nucleo/plan.md`, `.github/memory/constitution.md`.

> **Auditoria de 2026-10-18, passada 2.** Esta nota foi escrita com os docs antigos. Os docs
> `wiki-files-layer.md` §3 e §5 foram reescritos contra o comportamento de referência e **vencem** esta
> nota. Correções que afetam o código:
> - o fechamento `---` como **última linha do arquivo, sem quebra** é aceito (corpo vazio); o tipo de
>   quebra (LF ou CRLF) é decidido pela cerca de abertura;
> - o frontmatter guarda **tipos** (booleano, número, nulo, texto, lista, mapeamento), com a resolução
>   de escalares do núcleo YAML 1.2 descrita no doc: só `true/True/TRUE` e `false/False/FALSE` são
>   booleanos (`yes`, `on` são texto), datas são texto, `007` é texto, inteiro acima de 64 bits é erro,
>   chave duplicada é erro. Um frontmatter cuja raiz não é mapeamento é aceito na leitura; na escrita, a
>   conformidade OKF o troca por mapeamento vazio;
> - `pinned` só fixa quando é o **booleano** `true`; não há herança da fixação da versão anterior
>   (correção da decisão "herdar `pinned`" da 001/T080);
> - a chave `tier` é sempre gravada com o tier do pedido de escrita (vence o frontmatter); na
>   reindexação, o tier sai da chave `tier` (ausente → `semantic`; nulo, não texto ou desconhecido → erro);
> - título: **pula** linhas dentro de bloco cercado (a linha da tabela (b) que diz o contrário está
>   errada) e não tem teto;
> - ordem de emissão e estilo YAML: ver `phase0-findings.md` §0, ADR-17;
> - links: algoritmo completo no doc (links markdown com `<destino>`, destino com espaços, parênteses
>   aninhados, título opcional, escape com `\`, **decodificação `%XX` uma vez**; wikilink precedido de
>   `!` **é** link; relações só como lista de textos, relativas à raiz do projeto, sem decodificação);
> - as lacunas 1 a 5 do fim desta nota estão **fechadas** pelo doc.

## (a) Estrutura do frontmatter YAML

### Reconhecimento do bloco (wiki-files-layer §3.1; okf "Leitura de frontmatter")
- Só vale o bloco canônico no início do documento: linha `---`, conteúdo YAML, linha `---` de fechamento seguida de quebra de linha.
- As quebras podem ser LF ou CRLF. Um bloco CRLF não pode virar corpo, porque duplicaria o frontmatter na migração OKF.
- Um BOM UTF-8 inicial é descartado antes de tudo. Se ficasse no corpo, ocultaria o primeiro título e seria regravado depois do bloco.
- Não é frontmatter (o documento inteiro vira corpo) nestes casos:
  - bloco sem fechamento;
  - bloco sem quebra de linha após o fechamento;
  - bloco vazio, ou seja, duas linhas `---` consecutivas.
- Frontmatter presente mas com YAML inválido gera o erro tipado "YAML" (wiki-files-layer §11).
- O parser YAML é usado diretamente, e não um extrator de frontmatter de terceiros. Esses extratores perdem comentários e a ordem das chaves.
- Consequência: a reemissão não preserva comentários do frontmatter original.
- O corpo mantém suas quebras de linha originais em qualquer caso.
- O plano (plan §2.1) modela `Frontmatter` como "mapa ordenado", com `gopkg.in/yaml.v3` (plan §1).

### Chaves conhecidas e tipos

| Chave | Tipo / regra | Fonte |
|---|---|---|
| `type` | texto, obrigatório e não vazio para conformidade OKF. Valor só com espaços conta como ausente. | okf "Chave type"; domain-vocabulary §14 |
| `title` | texto, opcional. Entra na derivação de título (ver (b)). | wiki-files-layer §3.3 |
| `description` | derivada de `summary` se ausente e `summary` não vazio. | okf "Demais chaves" |
| `tags` | lista. Mantida sem alteração pela conformidade. Também gera entidades (minúsculas, espaços colapsados, sem duplicatas, não-texto descartado). | okf; wiki-files-layer §5.2 passo 6 |
| `generated` | mapeamento `{by, at}`. `at` é UTC com segundos, `AAAA-MM-DDTHH:MM:SSZ`. Se existir e não for mapeamento, é substituído por um mapeamento com `by`. | okf "Proveniência" |
| `sources` | lista de itens `{resource: ai-memory://session/<session_id>, author?: process:<agent>}`. Criada só se ausente e houver `session_id`. | okf "sources" |
| `stale_after` | derivada de `expires_at` (ver abaixo). | okf "stale_after" |
| `expires_at` | texto, RFC 3339 ou `AAAA-MM-DD` (fim do dia UTC, 23:59:59,999999). Ausente, nulo ou vazio significa sem validade. Qualquer outro valor, ou não-texto, é erro (falha fechada). | wiki-files-layer §5.2 passo 6; domain-vocabulary §6.7 |
| `entities` | ausente, nulo ou lista são aceitos. Outro tipo é erro. Limite público: até 10 nomes, 64 caracteres brutos cada, validados antes da normalização. | wiki-files-layer §5.2; domain-vocabulary §6.6 |
| `abstract` | texto. Resumo de uma linha. Vazio ou não-texto é ignorado. Limite público: 1024 caracteres brutos, aparado. | wiki-files-layer §5.2; domain-vocabulary §6.6 |
| `kind` | texto, até 64 caracteres, sem caractere de controle, aparado. Vazio significa ausente. | domain-vocabulary §6.6 |
| `relations` | mapa de relação para alvos (ver (c)). Até 32 alvos no total, cada um com até 1024 caracteres e sem caractere de controle. | domain-vocabulary §6.6; wiki-files-layer §3.5 |
| `pinned` | só o booleano `true` fixa (o texto `"true"` não). Fixação final = pedido **ou** `_slots/` **ou** `pinned: true`. Gravado como `pinned: true` quando fixada; `pinned: false` presente fica como está. Sem herança da versão anterior. | wiki-files-layer §5.2 passo 5 |
| `tier` | na escrita, sempre sobrescrito pelo tier do pedido (posição mantida se existia, senão no fim). Na reindexação, lido daqui (ausente → `semantic`; nulo, não texto ou desconhecido → erro). | wiki-files-layer §5.2 passo 5 e reindex de página |
| `last_modified_by` | bloco com identificador do autor, quando houver. Ator anônimo não gera bloco. | wiki-files-layer §5.2 passo 3 |
| `okf_version` | `"0.2"`, só no `index.md` da raiz. O frontmatter permitido em `index.md` é só esse. | okf "Versão do formato" |

- Chaves de extensão preservadas sem alteração: `tier`, `kind`, `slot_kind`, `entities`, `pinned`, `consolidated`, `session_id`, `agent`, `summary`, `expires_at` (okf "Chaves de extensão preservadas").
- Consumidores devem tolerar chaves desconhecidas (okf "Versão do formato").
- `generated_by_model`: se presente, vira `generated.by`. O doc marca "a confirmar" se algum produtor a grava. Pela constituição, isso deve ser tratado como decisão nossa em ADR.
- Nenhum `status` é gravado (okf).
- `_meta.md` é manifesto, não conceito. Recebe apenas `type: Scope Manifest` (okf).

### Emissão (wiki-files-layer §3.2)
- Frontmatter ausente, nulo ou objeto vazio: o documento sai sem bloco.
- Caso contrário: bloco YAML com quebras LF, seguido do corpo.

### Ordem canônica das chaves
LACUNA: nenhuma fonte define ordem canônica de emissão das chaves. Os docs só dizem que a ordem original é preservada na leitura (wiki-files-layer §3.1) e que o plano usa "mapa ordenado" (plan §2.1). A ordem de chaves novas, como `type`, `description`, `generated`, `sources` e `stale_after`, e o estilo de emissão do YAML (indentação, aspas) precisam de decisão do Analyst, registrada em ADR. Nada impede que seja: chaves existentes na ordem original, chaves derivadas acrescentadas ao final.

### Conformação (okf "Determinismo"; domain-vocabulary §14)
- É determinística e idempotente. Preenche só o que é derivável e não inventa nada.
- Conforme significa ter `type` não vazio. A exportação exige só isso. A migração também exige `generated.at`, exceto em `_meta.md`.
- `generated.at` fica fora da conformação de frontmatter. É carimbado só quando o conteúdo muda. "Conteúdo" é o corpo mais o frontmatter sem `generated.at`.
- Na reescrita sem mudança de conteúdo, o `generated.at` anterior é herdado.
- Regras de `stale_after` (okf "stale_after"):
  - criado só se ausente e `expires_at` for analisável;
  - RFC 3339 é copiado literalmente;
  - data pura vira `AAAA-MM-DDT23:59:59.999999Z`;
  - outro valor não cria nada.
- Reparo: um `stale_after` idêntico a um `expires_at` de data pura é reescrito para o instante de fim de dia.
- `generated.by`: `generated_by_model`, senão `process:ai-memory/<versão do build>`. Um valor já presente não é sobrescrito.
- A validação de `expires_at` e a derivação de `stale_after` usam a mesma função (domain-vocabulary §6.7).

## (b) Regras de título

Fonte: wiki-files-layer §3.3, okf "Demais chaves" (linha `title`).

Ordem de prioridade:
1. Campo `title` do frontmatter, se for texto e não vazio após remover espaços das pontas.
2. Primeira linha do corpo que começa com `# ` e tem texto depois (também após aparar). A busca é por linha e **pula** linhas dentro de blocos cercados (corrigido: a referência ignora `# ` em código cercado; wiki §3, título).
3. Nome do arquivo sem o sufixo `.md`, sem o caminho de diretórios.

Limites:
- Na escrita, o título é derivado depois da cadeia de admissão (wiki-files-layer §5.2 passo 6).
- O plano (plan §2.1 P-07) fixa título de observação ≤ 80 caracteres, na sanitização (constituição INV-06).
- LACUNA: nenhuma fonte fixa limite de tamanho para o título de página derivado do frontmatter ou do `# `. Os 80 caracteres valem para título de observação sanitizada (INV-06). Aplicar a páginas é decisão a registrar.
- A regra de derivação é idêntica à da exportação. Lá, `title` ausente ou vazio na cópia do pacote é derivado pela mesma ordem (okf).

## (c) Links e relações tipadas

### Formas reconhecidas (wiki-files-layer §3.4; domain-vocabulary §6.8)
- `[[caminho]]` e `[[caminho|rótulo]]`: locais, resolvem no mesmo projeto da página de origem.
- `[[projeto:caminho]]`: projeto irmão, no mesmo workspace.
- `[[workspace/projeto:caminho]]`: outro workspace.
- `[[_global:caminho]]`: projeto de preferências reservado. Fica no workspace padrão, e não no da página de origem, a menos que se use `workspace/_global`.
- Link markdown `[rótulo](destino)`. O destino é lido até 512 bytes. Um título opcional entre aspas é descartado.

### Ignorado como link
- Conteúdo de blocos cercados por três ou mais crases ou tis. O fechamento usa o mesmo caractere, com tamanho igual ou maior, e sem texto depois.
- Código inline: uma sequência de crases só fecha com sequência de mesmo tamanho na mesma linha. Sem fechamento, as crases são texto. Trechos que continuam na linha seguinte não são vistos.
- Imagens (precedidas de `!`).
- URLs com `://` e esquemas `mailto`, `data`, `javascript`, `vbscript`, `tel` e `file`.
- Âncora pura `#...`, destinos com barra invertida, destinos vazios.
- Destinos terminados em barra (diretório).
- Destinos cujo último segmento tem ponto mas não termina em `.md`.

### Normalização e alvo
- Descartar consulta (`?...`) e âncora (`#...`) antes de resolver.
- Destino sem extensão recebe `.md`.
- Destino iniciado por `/`, ou wikilink, é relativo à raiz do projeto. Os demais são relativos ao diretório da página de origem, com resolução de `..`.
- Resultados deduplicados e ordenados (saída estável).
- Destinos que não formam `PagePath` válido são descartados silenciosamente.
- `PagePath` (domain-vocabulary §4.2):
  - não vazio;
  - não começa com `/`;
  - sem prefixo de unidade Windows;
  - sem `\`;
  - sem segmento vazio, `.` ou `..`.
  - Existe validação separada e mais estrita, `ensure_portable`, aplicada só a caminhos novos.
- Alvo de relação (domain-vocabulary §6.6):
  - aceita as formas `caminho`, `projeto:caminho` e `workspace/projeto:caminho`;
  - componentes de escopo não podem ter espaço nas bordas nem ser vazios;
  - `.` e `..` são proibidos;
  - se o último segmento não tem extensão, acrescenta `.md`, e outra extensão é rejeitada;
  - o caminho final precisa passar em `ensure_portable`.

### Relações tipadas
- Vêm da chave `relations` do frontmatter, com a mesma gramática de alvo dos wikilinks (wiki-files-layer §3.5).
- Vocabulário fechado: `causes`, `fixes`, `contradicts`.
- Chave fora do vocabulário é ignorada com aviso (wiki-files-layer §3.5). O domain-vocabulary §6.8 diz que o nome desconhecido "não é erro" e o link fica como referência simples. As duas fontes são compatíveis. Resolver a leitura exata fica para o Analyst.
- Alvo que não nomeia uma página (diretório, `.md` sem nome, extensão diferente de `.md`) é ignorado com aviso.
- Valores de log vindos do frontmatter são truncados a 200 bytes, por serem entrada não confiável.
- Conjunto de links da página: toda escrita grava o conjunto completo, ou seja, links do corpo mais relações do frontmatter, por uma única função de extração (wiki-files-layer §3.6).

### Tabela `links` (database-schema.md:189-210)

| Coluna | Regra |
|---|---|
| `from_page_id` | `BLOB NOT NULL`, FK para `pages(id)` com `ON DELETE CASCADE`. |
| `to_page_id` | `BLOB`, FK com `ON DELETE SET NULL`. NULL significa link pendente, resolvido quando a página alvo existir. |
| `to_workspace` | NULL significa o workspace da origem. |
| `to_project` | NULL significa o projeto da origem. Ou o nome do projeto de destino, por exemplo `_global`. |
| `to_path` | `TEXT NOT NULL`. |
| `link_type` | `TEXT NOT NULL DEFAULT 'references'`. Valores: `references`, `causes`, `fixes`, `contradicts`. |

- Chave primária: `(from_page_id, to_workspace, to_project, to_path, link_type)`.
- NULL em coluna de PK não deduplica no SQLite. A aplicação deve deduplicar antes de inserir pela chave lógica `(to_workspace, to_project, to_path, link_type)`.
- Os links da página são regravados (delete e insert) a cada nova versão, na mesma transação da página (INV-03, constituição).
- Índices: `idx_links_to` (parcial, `to_page_id IS NOT NULL`), `idx_links_unresolved_path` (parcial, `to_page_id IS NULL`), `idx_links_to_path`, `idx_links_scope(to_project, to_path)`.
- Link de corpo (wikilink ou markdown) usa `link_type = 'references'` (padrão). Relação tipada usa o nome da relação. Isso é inferido do DEFAULT e do vocabulário. LACUNA: nenhuma fonte diz literalmente "link de corpo = references".

### Reescrita de links na exportação (okf "Reescrita de links no pacote")
- `[[alvo]]` e `[[alvo|rótulo]]` do próprio projeto viram link markdown relativo ao arquivo de destino. O texto é o rótulo, ou o alvo se não houver rótulo. `.md` é acrescentado quando necessário, e `#` e `?` são preservados.
- Links para outro projeto ou workspace ficam como `[[...]]` literal.
- Código cercado e código inline não são alterados.
- Alvos vazios ou malformados ficam intactos.
- Destinos com espaços, controles ou parênteses desbalanceados vão entre `<...>`. Se nem assim, ficam como `[[...]]`.
- A exportação é de escopo posterior. Provavelmente fora da 001, mas a regra existe no doc.

## (d) Derivação do tipo OKF

Fonte: okf "Chave type"; domain-vocabulary §14. As regras valem em ordem, e a primeira que se aplicar vence.

1. `type` presente e não vazio: mantido, nunca sobrescrito.
2. `kind` ∈ {`fact`, `note`, `procedure`, `decision`}: vira `Fact`, `Note`, `Procedure`, `Decision`. Outros valores de `kind` são ignorados.
3. `slot_kind` ∈ {`invariant`, `state`}: vira `Invariant` ou `State`.
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

- `_meta.md` recebe `type: Scope Manifest` quando não tem `type`, independentemente das regras acima.
- O okf.md compara por prefixo de diretório. O domain-vocabulary §14 lista sem barra. São equivalentes.
- Arquivos sem tratamento como conceito: `index.md`, `log.md`, `log-AAAA-MM.md` (ledger por nome e conteúdo, wiki-files-layer §9) e `_pending/` (okf; wiki-files-layer §8).
- Detalhe de escopo: o doc cita `index.md` na raiz do projeto com `okf_version: "0.2"` e listagem dos diretórios. A 001 tem `index.md` no layout (plan §2.4).

## (e) Bytes idênticos na reescrita, BOM e CRLF

- Reescrever conteúdo idêntico não altera os bytes do arquivo nem cria nova versão (wiki-files-layer §5.2, "Propriedades observáveis").
- No `UpsertPage`, o caso A (idêntico) não cria versão e só registra evidência (plan P-04). A comparação de conteúdo na escrita considera corpo, frontmatter sem `generated.at`, título, tier e fixação (okf "Determinismo e idempotência").
- Quando o conteúdo é igual, a versão atual é mantida e o `generated.at` original é preservado (okf).
- Idempotência da conformação: aplicar duas vezes dá o mesmo resultado, e a segunda passada não altera nada (okf; domain-vocabulary §14).
- Na migração e no reparo, a reescrita do arquivo só ocorre se o conteúdo mudou (okf, passo 7 e "Reparos").
- Verificação prevista: ida e volta. Emitir um documento e lê-lo de volta produz o mesmo documento (okf "Verificação").
- Corpo preservado byte a byte, inclusive fins de linha (okf "Leitura de frontmatter"; wiki-files-layer §3.1).
- BOM UTF-8: descartado na leitura (okf; wiki-files-layer §3.1). A emissão não grava BOM, pois o bloco emitido começa com `---`. LACUNA: nenhuma fonte diz explicitamente que a emissão nunca escreve BOM. O texto de wiki-files-layer §3.1 diz que, se o BOM permanecesse, seria "regravado depois do bloco".
- CRLF: o frontmatter é reconhecido com LF ou CRLF. A emissão do bloco YAML usa LF (§3.2).
- LACUNA: não está especificado como fica a quebra de linha do bloco emitido quando o arquivo original usava CRLF, nem se o corpo CRLF mais bloco LF gera arquivo com fins de linha mistos. O corpo mantém seus fins de linha originais. O Analyst deve decidir se isso afeta a regra de "bytes idênticos". Hipótese segura: reescrita sem mudança de conteúdo não toca o arquivo, então não há reescrita do bloco.
- Escrita atômica (wiki-files-layer §4; plan P-06; INV-10): temporário `.ai-memory-tmp.*` no mesmo diretório, `Sync`, rename e fsync do diretório. No Windows, repete até 5 vezes para os códigos 32 e 33, com espera de 10 a 50 ms.
- O watcher ignora o temporário pelo prefixo. O identificador do arquivo **não** é usado (corrigido). As próprias escritas são absorvidas pela checagem de conteúdo idêntico no índice (wiki-files-layer §4, §8.1 e §13.3).

## Gaps e itens "a confirmar" para o Analyst
1. LACUNA: ordem canônica das chaves e estilo de emissão do YAML (aspas, indentação) não especificados.
2. LACUNA: limite de tamanho do título de página derivado.
3. LACUNA: tratamento de CRLF na emissão do bloco e de fins de linha mistos.
4. LACUNA: que a emissão nunca escreve BOM.
5. LACUNA: confirmação literal de `link_type = 'references'` para links de corpo.
6. "A confirmar" nos docs: produtor de `generated_by_model` (okf), e se a ingestão automática pega arquivos desempacotados sem reindexação (okf "Importação"). Pela constituição, decidir em ADR.
7. Plano: `plan.md` §1 e §2.1 citam Go 1.26 e `yaml.v3`, enquanto o contexto do projeto menciona Go 1.25. Fora do escopo desta nota, mas divergente.
8. Não houve execução de `go build`. A tarefa era só de leitura de docs.
