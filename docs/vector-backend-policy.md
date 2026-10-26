# Política de backend vetorial

Este documento define qual armazenamento de vetores o ai-memory usa hoje, quando
ele deve mudar e por quê. Ele separa três coisas: o comportamento atual, as
decisões de projeto e as propostas ou alternativas descartadas. A geração dos
embeddings (modelo, provedores, instalação offline) está em
[local-embeddings.md](local-embeddings.md).

## Resumo

- Hoje os vetores ficam em tabelas comuns do SQLite do próprio sistema. A busca
  vetorial é uma varredura completa sobre as linhas do projeto, sem índice
  vetorial e sem extensão de banco.
- Decisão: a busca vetorial é um sinal entre vários, combinado por fusão RRF.
  Não é o único critério de ranking.
- Decisão: a extensão de índice vetorial `sqlite-vec` está adiada, não
  rejeitada. Só entra quando medições mostrarem gargalo e ganho de qualidade
  (ver [Critérios para adotar](#critérios-para-adotar)).

## Comportamento atual

### Armazenamento

- **Tabela de embeddings do corpo** (`page_embeddings`): uma linha por página,
  na versão mais recente. Campos: identificador da página (chave primária, com
  remoção em cascata quando a página é removida), vetor, provedor, modelo,
  dimensão (maior que zero) e instante de criação. Há índice por
  provedor, modelo e dimensão.
- **Tabela de embeddings de resumo** (`page_abstract_embeddings`): mesmo
  esquema, aplicado à linha `abstract:` do frontmatter da página. Só é usada
  quando o stream opt-in de resumo está ligado (ver
  [Fusão RRF](#fusão-rrf)).
- **Formato do vetor**: sequência de números de ponto flutuante de 32 bits em
  ordem little-endian, com exatamente dimensão × 4 bytes.
- **Normalização**: o contrato do embedder devolve vetores de comprimento
  unitário. Por isso o produto interno entre dois vetores equivale à
  similaridade de cosseno.
- **Identidade do embedding**: a tripla (provedor, modelo, dimensão) identifica
  as linhas. Um prefixo de documento configurado altera a identidade do modelo
  armazenado, com sufixo derivado de hash. Um prefixo de consulta não altera a
  identidade e não exige reembedding.
- **Falhas de embedding**: registradas em tabela própria
  (`page_embed_failures`), com resultado `failed`, `unreadable` ou
  `skipped_empty`. Só a última tentativa que falhou fica registrada. Um
  sucesso não grava nada nessa tabela.
- **Fonte da verdade**: o markdown do wiki. As tabelas de embedding são dados
  derivados e podem ser reconstruídos a partir do markdown (ver
  [architecture.md](architecture.md)).

### Busca vetorial

- **Escopo**: a busca vetorial recebe sempre um projeto. Não há varredura
  vetorial entre projetos. A busca cross-project usa o caminho somente FTS,
  conforme o texto de roteamento exposto ao cliente.
- **Filtros da varredura**: mesmo workspace e projeto; somente a versão mais
  recente de cada página, salvo opt-in para incluir versões superadas; somente
  páginas não expiradas, salvo opt-in para incluir expiradas; e linhas cujo
  provedor, modelo e dimensão são iguais aos da configuração em execução.
  Linhas de outra tripla são ignoradas em silêncio.
- **Algoritmo**: pontua todas as linhas filtradas pelo produto interno com o
  vetor da consulta, seleciona as top-k e ordena por pontuação decrescente.
  Não há limite na consulta SQL. O custo cresce com o número de páginas
  embutidas do projeto a cada busca.
- **Limite zero**: se o limite pedido for zero, o stream devolve vazio.
- **Janela de candidatos**: todos os streams usam a mesma janela. Com limite
  `k` maior que zero, o tamanho é `min(max(4·k, 20), k + 300)`. Exemplos:
  `k = 1` gera 20 candidatos; `k = 10` gera 40; `k = 1000` gera 1300.
  Com `k = 0`, a janela é zero.

### Fusão RRF

- Streams de candidatos: FTS, correspondência lexical de entidades, vetor do
  corpo, vetor de resumo (opt-in) e vizinhança de grafo das sementes.
- Cada stream contribui com `1 / (60 + posição)`, com posição a partir de 1.
  A pontuação final é a soma das contribuições. Uma página ausente de um stream
  não recebe contribuição desse stream.
- O rank devolvido é `−pontuação`, então valores menores são melhores. Essa é a
  mesma convenção do FTS.
- As sementes do grafo são as páginas encontradas pelos streams, sem repetição,
  na ordem FTS, vetor, resumo e entidade.
- Páginas encontradas só pelo vetor não trazem título nem descrição do stream.
  Essas duas informações são buscadas depois da fusão, só para as páginas que
  ainda não as têm.

### Ajuste de autoridade (após a fusão)

Depois do RRF, cada página recebe um fator de autoridade. O fator começa em
1,0 e recebe os ajustes abaixo.

| Critério | Ajuste |
|---|---|
| Tipo `rule` ou `decision` | +0,15 |
| Tipo `procedure`, `gotcha` ou `preference` | +0,12 |
| Tipo `concept` ou `slot` | +0,07 |
| Tipo `session` | −0,15 |
| Tier `semantic` ou `procedural` | +0,10 |
| Tier `episodic` | −0,08 |
| Tier `working` | −0,03 |
| Página fixada | +0,08 |
| Caminho começa com `_lint/` | −0,20 |
| Caminho começa com `investigations/` | −0,05 |
| Tag `canonical` ou `source-of-truth` | +0,20 |
| Tag `active` | +0,05 |

Depois dos ajustes, valem os tetos por tag, aplicados com o menor valor:

- `do-not-answer-from` ou `test-fixture`: teto de 0,55.
- `superseded`: teto de 0,65.
- `historical`: teto de 0,80.

O fator final fica sempre entre 0,55 e 1,50. Uma página marcada nunca sai da
busca; ela só perde posição.

Aplicação ao rank: se o rank é menor ou igual a zero (caso da fusão), ele é
multiplicado pelo fator. Se é positivo (caso FTS puro), ele é dividido pelo
fator. Nos dois casos, fator maior significa posição melhor.

**Roteamento de recuperação de sessão** (opt-in, desligado por padrão): em
consultas que pedem uma sessão passada, as páginas de sessão recebem de volta
a penalidade de tipo e tier e ganham um bônus de 0,25. O fator continua dentro
dos limites acima.

### Sem vetores

- **Sem embedder configurado**: a consulta roda sem o stream vetorial e sem o
  stream de resumo. Os streams ativos são FTS, entidade e grafo. Se as páginas
  compiladas não trazem resultado, a consulta usa o fallback de observações
  brutas.
- **Falha ao embutir a consulta**: o erro vai para o log como aviso e a
  consulta segue sem o stream vetorial. A consulta não falha.
- **Página sem linha de embedding**: a página não aparece no stream vetorial,
  mas continua disponível pelo FTS, pela entidade e pelo grafo.
- **Linhas de outra tripla**: são ignoradas pela busca. Na inicialização do
  servidor, a divergência gera só um aviso; o processo continua. A correção é
  o re-embed forçado, que apaga antes as linhas obsoletas da tripla e grava de
  novo.
- **Linha corrompida**: se o tamanho dos bytes de um vetor não é igual a
  dimensão × 4, a consulta inteira retorna erro. Ela não cai para o modo sem
  vetor. Este é o comportamento atual; se a política deve mudar, é decisão a
  validar.

### Status

O status do sistema informa o número de páginas sem embedding, separado entre
páginas com corpo (`latest_pages_missing_embeddings`) e páginas de corpo vazio
(`latest_pages_unembeddable`, que nenhum embedder cobre), o total de linhas de
embedding, a contagem de linhas por tripla (provedor, modelo, dimensão; ordenada por
contagem decrescente e depois por provedor, modelo, dimensão) e as falhas de
embedding: `embed_failures_unresolved` (páginas mais recentes cuja última tentativa
falhou ou foi pulada e que ainda não têm embedding) e `embed_failures_recovered`
(têm falha registrada mas já têm embedding). A CLI mostra `embeddings: <N> rows; <M>
latest pages missing`, a linha `unembeddable: <K> (empty body; no embedder can cover
these)` quando K>0, uma linha `<provedor>/<modelo> dim <D>: <N> rows` por tripla e,
só se houver pendência, `embed failures: <U> unresolved (<R> recovered since)`. O
registro de falha guarda no máximo 500 caracteres do erro (mais `…` quando cortado). Mais de uma tripla no status indica
heterogeneidade: o usuário trocou de provedor sem reembedar.

## Decisões de projeto

### D1. Vetores em tabelas comuns, sem extensão

Decisão. A busca vetorial é uma varredura simples e inspecionável sobre as
tabelas de embedding. Os motivos são:

- o corpo esperado é de centenas a poucos milhares de páginas mais recentes
  por projeto (estimativa de projeto, não medida);
- a varredura é rápida o suficiente enquanto o FTS, a entidade e o grafo
  respondem a parte importante da consulta;
- o vetor é só um dos sinais da fusão.

### D2. Sem extensão vetorial por convenção

Decisão. Não adicionar `sqlite-vec` só porque bancos vetoriais são
convencionais, nem porque a arquitetura fica mais limpa. Sem medição de
latência ou de recall, a dependência extra e a superfície de reparo não
compensam.

Os custos operacionais que pesam contra a extensão agora são:

- o carregamento precisa funcionar de forma idêntica em toda conexão;
- o empacotamento (container e binário estático) precisa continuar confiável
  nos hosts-alvo;
- o diagnóstico precisa separar falha de carregamento da extensão de drift de
  provedor, modelo ou dimensão;
- a tabela derivada precisa de caminhos seguros de reconstrução e
  preenchimento.

### D3. Vetor como sinal, e autoridade depois da fusão

Decisão. A similaridade semântica não decide sozinha o ranking. A autoridade
é aplicada depois da fusão. Assim, uma página de sessão com alta similaridade
não supera uma decisão mantida apenas por ser semanticamente próxima.

### D4. Markdown é a fonte da verdade

Decisão. O markdown do wiki é a fonte da verdade. As tabelas de embedding e
qualquer índice vetorial futuro são derivados. O reparo nunca apaga arquivos
do wiki.

## Critérios para adotar

Decisão. Os números abaixo são limiares de projeto. O código não mede p95 de
produção nem ganho de recall em tempo de execução. Existe um harness de
avaliação de recall (teste de recall@5, FTS contra híbrido), que serve de base
para medir o último critério.

Adotar `sqlite-vec` só quando todos os critérios forem verdadeiros:

- o número de páginas embutidas mais recentes por projeto passa de cerca de
  5 mil a 10 mil, de forma recorrente;
- o p95 de `memory_query` passa de cerca de 150 a 250 ms localmente, por causa
  da pontuação vetorial, depois que FTS e grafo já foram otimizados;
- o perfil de execução mostra que o produto interno consome CPU relevante,
  sobretudo sob chamadas MCP concorrentes;
- a avaliação de recall mostra que o vetor melhora o recall em relação a FTS,
  entidade e grafo somados, por margem relevante (por exemplo, 5 a 10% de
  recall@5);
- uma migração cria e preenche a tabela vetorial a partir de
  `page_embeddings` sem apagar dados do usuário;
- o índice derivado pode ser descartado e reconstruído a partir do markdown e
  de `page_embeddings`;
- a inicialização falha com mensagem clara quando a extensão não está
  disponível no empacotamento.

## Propostas (não implementadas)

Forma prevista quando o índice vetorial for adotado. Nada disso existe hoje.

1. Manter `page_embeddings` como metadado durável e armazenamento dos vetores.
2. Adicionar uma tabela vetorial virtual, preenchida a partir de
   `page_embeddings`.
3. Estender os diagnósticos com a contagem de linhas da tabela vetorial e de
   linhas obsoletas.
4. Criar um comando ou caminho seguro para reconstruir a tabela derivada.
5. Trocar a geração de candidatos da varredura para o índice, mantendo a
   fusão RRF e o ajuste de autoridade inalterados. A superfície MCP não muda.

A migração deve ser aditiva: nenhuma tabela existente é recriada.

## Alternativas descartadas

- **Adotar `sqlite-vec` já**: adiado (D2) até os critérios serem atendidos.
- **Adotar por convenção ou por preferência arquitetural**: rejeitado (D2).

## Referências internas

- [local-embeddings.md](local-embeddings.md): geração dos embeddings e modelo
  local.
- [storage-and-retrieval.md](storage-and-retrieval.md): armazenamento e
  recuperação no geral.
- [database-schema.md](database-schema.md): esquema completo do banco.
- [architecture.md](architecture.md): fonte da verdade e camadas de dados.
- [mcp-server.md](mcp-server.md): superfície de `memory_query`.
