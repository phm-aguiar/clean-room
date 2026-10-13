# Validade temporal

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

A validade temporal responde à pergunta "o que o armazenamento considerava atual naquele instante?". Cada versão de página e cada vínculo de entidade carrega uma janela de **tempo de ingestão**: o intervalo em que o armazenamento tratou aquela versão como a vigente. A consulta com `as_of` usa essas janelas para reconstruir o conhecimento de um instante passado.

## Escopo

- **Só tempo de ingestão.** As janelas registram quando o ai-memory aprendeu e substituiu um fato, não quando o fato era verdadeiro no mundo. Uma dimensão de tempo do mundo, extraída por modelo de linguagem, fica fora de escopo: exigiria confiar num modelo para datar fatos, e o caminho padrão de escrita não depende de modelo. Essa dimensão fica reservada para uma evolução futura. O vocabulário está fixado em [janelas de ingestão de página](design-page-ingestion-windows.md).
- **Duas granularidades, uma semântica.** Vínculos de entidade (granularidade de vínculo) e versões de página (granularidade de página) usam a mesma forma de janela. Um vínculo herda a linha do tempo da versão à qual pertence. Uma versão de página carrega a própria janela.
- **Remoção é remoção.** Uma página removida fisicamente leva seus vínculos de entidade em cascata. A linha do tempo não sobrevive a uma purga explícita (ver [ciclo de vida e purga](lifecycle-ops.md)).
- **Arestas tipadas** (ver [arestas tipadas](typed-edges.md)) ficam presas à versão de origem: somem apenas quando a linha da versão é removida. Não têm colunas de vigência próprias e não participam da consulta `as_of`.

Armazenamento, escrita e leitura estão descritos em [armazenamento e recuperação](storage-and-retrieval.md). O esquema completo das colunas e das versões de migração está em [esquema do banco](database-schema.md). Este documento descreve o comportamento sobre essas colunas.

## Campos de vigência

### Granularidade de vínculo

| Campo | Significado |
|---|---|
| `valid_from` | Instante de criação da versão de página à qual o vínculo pertence. |
| `superseded_at` | Fim da janela do vínculo. `NULL` enquanto a versão é a atual. Quando preenchido, é o instante em que a versão deixou de valer: a criação da versão sucessora, no caso de supersessão, ou o instante da retirada, nos demais casos. |

### Granularidade de página

| Campo | Significado |
|---|---|
| `valid_from` | Instante de criação da própria versão. |
| `valid_to` | Fim da janela da versão: a criação da versão sucessora, ou o instante da retirada quando não há sucessora. `NULL` enquanto a versão é a atual. |

### Nomes e papéis

- O fim da janela de página se chama `valid_to` porque `superseded_at` na tabela de páginas já tem outro significado. Ele é a marca de **tombstone**: a marca de uma versão retirada sem sucessora, gravada por decaimento ou por reconciliação de arquivo ausente. Essa marca não indica "substituída por" e não serve como fim de janela.
- Na tabela de páginas, a supersessão grava `is_latest = 0` e `valid_to`. Nunca grava `superseded_at`.
- Na tabela de vínculos, a supersessão fecha `superseded_at`, que é a coluna de fim da janela do vínculo. O nome coincide com a marca de tombstone da tabela de páginas, mas são colunas de tabelas diferentes com papéis diferentes.
- "Atual" significa `is_latest = 1`, a versão corrente do caminho.

### Preenchimento em escrita nova

- Página nova ou versão nova: `valid_from`, `created_at` e `updated_at` recebem o mesmo instante da escrita.
- Vínculos criados junto com a versão recebem `valid_from` igual à criação dessa versão, e `superseded_at` fica `NULL`.

## Preenchimento de dados antigos

### Vínculos

- A atualização do esquema preenche `valid_from` com a criação da versão de cada vínculo. Para versões não atuais que têm sucessora, `superseded_at` recebe a criação da sucessora.
- Versões não atuais sem sucessora, retiradas antes da correção, ficam com a janela aberta até uma correção posterior. Essa correção fecha a janela em `updated_at` da página, que é uma aproximação do instante da retirada.
- Na abertura do armazenamento, uma passagem idempotente procura versões atuais que não têm nenhum vínculo. Para cada uma, deriva as entidades do frontmatter (ver [Derivação das entidades](#derivação-das-entidades)) e cria os vínculos com `valid_from` igual à criação da versão. Versões que não geram entidade são puladas sem escrita. Versões não atuais não são tocadas. Quando não há candidatas, a passagem não altera nada. Nenhum comando manual é necessário.

### Páginas

- A atualização do esquema adiciona as colunas e o índice de janela de página, sem preenchê-los.
- O preenchimento acontece na abertura do armazenamento, em lotes de **5000** versões, com ponto de controle após cada lote. Cada lote seleciona as versões que ainda não têm `valid_from`. Por isso a passagem é retomável após interrupção, e reexecutá-la converge para o mesmo resultado.
- Regra para cada versão:
  1. `valid_from` = criação da própria versão.
  2. Se a versão tem sucessora, `valid_to` = a menor criação entre as sucessoras.
  3. Se a versão não é atual e não tem sucessora, `valid_to` = o primeiro valor disponível entre: a marca de tombstone, o menor fim de janela de vínculo registrado, e `updated_at` como último recurso.
  4. Caso contrário, `valid_to` = `NULL`. Uma versão atual sem sucessora permanece aberta.
- Um armazenamento já preenchido não faz nenhuma escrita nessa passagem.

## Derivação das entidades

As entidades de uma página vêm do frontmatter, tanto na escrita quanto no preenchimento de dados antigos:

1. **Lista `entities`.** Lista explícita, que pode ser emitida pelo consolidador. Na escrita, um valor que não é lista é erro de escrita. No preenchimento de dados antigos, o mesmo valor é ignorado.
2. **Lista `tags`.** Quase toda página tem. Tags são o que a página é sobre, por isso semeiam o índice de entidades de forma determinística, sem passo de modelo. Uma lista malformada ou ausente não contribui e nunca falha a escrita.

A ordem é `entities` primeiro, depois `tags`. Em seguida cada nome passa por normalização:

- aparam-se as bordas, colapsam-se espaços internos e converte-se para minúsculas;
- nome vazio, com caractere de controle ou com mais de **64** caracteres é descartado, nunca truncado;
- duplicatas são removidas, mantendo a primeira ocorrência;
- no máximo **10** entidades por versão. O excedente é descartado. Como `entities` vêm primeiro, as entidades explícitas ganham as vagas.

Uma entidade é única dentro de cada par workspace e projeto. Cada versão tem no máximo um vínculo com a mesma entidade.

## Retirada de versões

Quando a versão atual de um caminho é retirada, o mesmo instante é gravado na janela de página e na janela de vínculo, na mesma transação:

| Caminho de retirada | Instante gravado | Efeito na página | Efeito nos vínculos | Marca de tombstone |
|---|---|---|---|---|
| Supersessão (escrita de conteúdo diferente) | Criação da nova versão | `is_latest = 0` e `valid_to` | `superseded_at` nos vínculos ainda abertos | Não |
| Decaimento (esquecimento) | Instante do decaimento | `is_latest = 0`, `superseded_at` e `valid_to` | `superseded_at` nos vínculos ainda abertos | Sim |
| Reconciliação de arquivo ausente (opt-in) | Instante da reconciliação | Igual ao decaimento | Igual ao decaimento | Sim |
| Reorganização de workspace (cemitério) | Instante único da operação, para todas as versões atuais do workspace | `is_latest = 0` e `valid_to` | `superseded_at` nos vínculos ainda abertos | Não |
| Movimento de sessão com regeneração | Instante da operação | `is_latest = 0` e `valid_to` | `superseded_at` nos vínculos ainda abertos | Não |

- A retirada por decaimento só se aplica se o identificador esperado ainda for a versão atual. Um candidato antigo não retira uma página que já foi reescrita.
- A remoção física (exclusão de página por caminho, purga de sessão, de projeto ou de workspace) apaga as linhas. Os vínculos saem em cascata. Não existe janela para uma linha removida.

## Escrita de uma nova versão

Cada escrita de página passa pela mesma regra, dentro de uma transação. Todos os instantes gravados nessa transação são o mesmo instante.

1. **Conteúdo idêntico à versão atual.** O hash do corpo, o frontmatter (ignorando o carimbo `generated.at`), o título, o tier e o `pinned` são iguais. Não cria versão e não altera nenhuma janela. Registra apenas a evidência contra o identificador atual e devolve esse identificador.
2. **Conteúdo diferente (supersessão).** A versão atual recebe `is_latest = 0` e `valid_to`. Os vínculos ainda abertos dela recebem `superseded_at`. A nova versão é inserida com `supersedes` apontando para a anterior, e com `valid_from`, `created_at` e `updated_at` iguais ao instante. Os vínculos da nova versão são criados com `valid_from` igual ao instante. A auditoria registra a supersessão.
3. **Caminho sem versão atual, mas com tombstone.** A nova versão é encadeada ao tombstone por `supersedes`, e a marca de tombstone dele é limpa. O `valid_to` do tombstone é mantido: ele registra o instante em que a página deixou de existir, e não é empurrado para o presente. A auditoria registra a ressurreição. Consequência: entre o `valid_to` do tombstone e a criação da nova versão, nenhuma versão daquele caminho vale.
4. **Caminho sem versão atual e sem tombstone.** Nova cadeia, com `valid_from` igual ao instante. A auditoria registra a criação.
5. **Colisão de caminho.** Se outra página atual do mesmo projeto ocupa um caminho equivalente (diferença apenas de caixa ou de normalização Unicode), a escrita é recusada com erro de colisão.

Pelos caminhos de escrita descritos, um caminho tem no máximo uma versão válida em cada instante.

## Consulta em um instante (`as_of`)

### Entrada

- O parâmetro `as_of` de `memory_query` aceita um instante ISO-8601 com data, hora e fuso, por exemplo `2026-06-01T00:00:00Z`. A aceitação de uma forma apenas com data é a confirmar.
- Texto vazio ou só com espaços equivale a ausência do parâmetro: a consulta é normal.
- Valor que não é um instante gera erro.
- Combinar `as_of` com `global` ou com `scopes` gera erro.
- `answer` não é sintetizado em consulta `as_of`. A resposta traz um aviso em `answer_unavailable`.
- Sem `as_of`, nada muda.

### Regra de vigência

- Uma versão de página vale no instante T quando `valid_from <= T` e (`valid_to` é nulo ou `valid_to > T`).
- Um vínculo vale em T quando `valid_from <= T` e (`superseded_at` é nulo ou `superseded_at > T`).
- O intervalo é meio-aberto: no instante exato de uma supersessão, vale a versão nova, e a antiga já não vale.

### Dois fluxos

**Fluxo de entidades.** Recupera as páginas que carregavam uma entidade citada na consulta, naquele instante.

- Termos da consulta: a consulta é separada em trechos formados por letras, números, hífen e sublinhado. Hífen e sublinhado das bordas são removidos e tudo vira minúsculas. Só entram termos com 3 a 64 caracteres, sem repetição. São usados no máximo **12** termos, na ordem em que aparecem. Um termo composto com hífen ou sublinhado também gera suas partes.
- Casamento com o nome de uma entidade: o termo é igual ao nome, ou o nome começa pelo termo, ou alguma palavra do nome começa pelo termo. As palavras do nome são separadas por espaço, hífen ou sublinhado. Por exemplo, `actor` casa com `writer actor`, `writer-actor` e `writer_actor`.
- Peso: cada entidade casada contribui com `1 / N`, onde N é o número de páginas que carregam essa entidade no instante T. Tags largas, presentes em muitas páginas, pesam pouco.
- Uma página recebe a soma dos pesos das entidades casadas. A ordenação é por peso decrescente, depois por número de termos casados decrescente, e por último pelo caminho em ordem alfabética.
- O vínculo precisa estar com a janela contendo T. A validade por tempo de expiração é ignorada neste fluxo.

**Fluxo de texto.** Recupera as versões cuja janela de página contém T, e que não estavam expiradas em T.

- A expiração é avaliada em T. Uma página já expirada em T já estava oculta da busca naquele instante.
- A ordenação usa a relevância da busca textual (BM25). Essa relevância usa as estatísticas do índice atual, que incluem versões posteriores e outros projetos. Por isso a ordem e o conjunto limitado podem mudar com escritas posteriores, mesmo para o mesmo T. Esse fluxo recupera conteúdo histórico, mas não reproduz o ranking que teria sido devolvido em T.

**Fusão.**

- Cada fluxo numera suas posições a partir de 1. Cada página recebe, de cada fluxo em que aparece, `1 / (60 + posição)`. A soma é sua pontuação combinada.
- O mesmo ajuste de autoridade da busca padrão é aplicado a todas as candidatas. O fator fica entre 0,55 e 1,50. Marcadores de baixa autoridade no frontmatter reduzem o teto desse fator.
- A ordenação final usa a pontuação ajustada, com a melhor primeiro. O empate é desfeito pelo caminho em ordem alfabética. Depois corta-se pelo limite pedido.
- Não participam da consulta `as_of`: busca vetorial, vizinhos de grafo e busca de observações brutas. Consultas de auditoria não contabilizam acesso e não passam por reordenação extra.

**Explicação (`explain`).** Com `explain` ativo, a resposta traz `streams_active` igual a `["entity", "fts"]`, mesmo quando um dos fluxos não produziu resultados. Cada página traz a posição em cada fluxo, o peso de entidade, os termos casados, a contribuição de cada fluxo e o fator de autoridade.

## Exemplo

Uma página registrava `postgres` como banco de dados, e depois foi migrada para outro banco:

```jsonc
// Junho: notes/db.md diz "usamos postgres"    (entidade: postgres)
// Agosto: a página é substituída: "migramos para sqlite" (entidade: sqlite)

memory_query { "query": "postgres" }
// → nenhum resultado: o conhecimento atual mudou

memory_query { "query": "postgres", "as_of": "2026-07-01T00:00:00Z" }
// → a versão de junho de notes/db.md: a página que carregava a entidade
//   `postgres` no início de julho, mesmo já tendo sido substituída depois
```

O fluxo de entidades só responde quando a consulta cita uma entidade acompanhada pela versão. O fluxo de texto cobre o resto: uma versão sem `entities` ou `tags` no frontmatter, ou uma pergunta feita sem o termo exato que foi retirado ("qual banco usávamos durante a queda?"), ainda é resolvida pelo texto que estava vigente em T.

## Quando usar

A consulta comum (`memory_query` sem `as_of`) responde "o que sabemos agora" e é a escolha certa na maioria dos casos. `as_of` serve para auditorias e análises pós-incidente: "o que a wiki dizia quando aquela decisão foi tomada", "já sabíamos desse problema antes do incidente".

## Limites

- Só tempo de ingestão. Não há resposta sobre o que era verdade no mundo em um instante.
- Consulta por entidade depende de a página ter `entities` ou `tags` no frontmatter. Páginas sem nenhum dos dois só aparecem pelo fluxo de texto.
- A ordem devolvida não reproduz o ranking que a busca teria dado em T. Ela recupera o conteúdo histórico, não a classificação histórica.
- Sem resposta sintetizada, sem busca vetorial e sem vizinhos de grafo em consulta `as_of`.
