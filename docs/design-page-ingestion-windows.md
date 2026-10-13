# Janelas de ingestão por página e `as_of` com busca por versão

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

*Status: a fase A está implementada na versão 2.2.0. Cada seção indica o que já está **implementado** e o que é **proposta** (sem comportamento correspondente ainda). A fase B, que trata do tempo do mundo, continua adiada.*

## 1. Problema

A consulta temporal `as_of` ([temporal.md](temporal.md)) responde perguntas como "o que sabíamos sobre X em junho" por meio de janelas de validade nos vínculos entre entidade e página. Essa abordagem deixa três lacunas no nível da página, não do vínculo:

1. **Cegueira a paráfrases no modo auditoria.** Antes desta fase, `as_of` consultava apenas entidades. A pergunta precisava conter a entidade aposentada exata, e a página precisava trazer a entidade ou a tag certa. Busca textual, vetorial e por grafo eram ignoradas. Por isso, "Qual banco usávamos quando o incidente X aconteceu?" podia não encontrar nada se a formulação fosse diferente.
2. **A linha do tempo da página é implícita.** As versões formam uma cadeia (sucessão e data de criação), mas não havia um predicado que dissesse "versões vigentes no instante T" sem recalcular a cadeia a cada consulta.
3. **Risco da classe V58 sem equivalente na página.** Caminhos de fusão deixaram janelas de vínculo abertas, o que permitia a `as_of` ressuscitar conhecimento aposentado. A solução no nível da página precisa fechar a janela na mesma transação de toda aposentadoria, não apenas da sucessão.

## 2. Vocabulário: tempo de ingestão, não tempo válido

As janelas da fase A são de **tempo de ingestão**: o instante em que o armazenamento passou a tratar uma versão como atual e o instante em que ela deixou de ser. A mesma dimensão já era usada nas janelas de vínculo; esta fase apenas a materializa no nível da página. O termo "tempo válido" fica reservado para o tempo do mundo (fase B, seção 7).

| Termo | Significado | Estado |
|---|---|---|
| janela de ingestão (grão de página) | `valid_from` é a data de criação da versão. `valid_to` é a data de criação da versão sucessora, ou o instante de aposentadoria (seção 3), ou nulo enquanto a versão for a última viva | Implementado |
| janela de ingestão (grão de vínculo) | `valid_from` e `superseded_at` nos vínculos entre entidade e página | Implementado (anterior a esta fase) |
| tempo válido / tempo do mundo | quando um fato era verdadeiro no mundo real | Proposta (fase B, adiada) |

### 2.1 Nomes das colunas

- **`valid_from`** espelha a coluna homônima dos vínculos, de propósito: mesma dimensão, mesmo formato de predicado, mesmos testes. Aqui "válido" significa "a versão que o armazenamento tratou como atual", e não verdade no mundo. Implementado.
- **`valid_to`**, e não `superseded_at`, pelo seguinte motivo: `superseded_at` já marca a remoção por decaimento (varredura de esquecimento), que ocorre quando a versão não tem sucessora. Reutilizar o nome confundiria "removida pela varredura" com "substituída por uma versão mais nova". `valid_to` também cobre o fim sem versão sucessora, caso em que `superseded_at` seria impreciso. Implementado.
- **`valid_from` duplica `created_at`** por construção. A duplicação é aceita (seção 4) porque mantém cada linha autoexplicativa e deixa o predicado de `as_of` simétrico ao do grão de vínculo. Implementado.

## 3. Fim da janela: sucessão, aposentadoria ou aberta

Uma versão de página fecha sua janela em exatamente um dos casos abaixo.

1. **Sucessão.** Uma nova versão do mesmo caminho é gravada. `valid_to` recebe a data de criação da nova versão. A janela fecha na mesma transação em que a versão anterior deixa de ser a última. Na mesma transação, os vínculos de entidade da versão anterior são fechados no mesmo instante. Implementado.
2. **Aposentadoria sem sucessora.** Ocorre em quatro caminhos: remoção por decaimento (varredura de esquecimento); reconciliação que marca como removida uma página cujo arquivo sumiu (opcional, desligada por padrão); reorganização de sessões, que aposenta as páginas atuais do workspace; e regeneração na movimentação de sessão, em que a página atual é aposentada para ser gerada de novo. Nesses casos, `valid_to` recebe o instante da aposentadoria, gravado explicitamente nos dois grãos na mesma transação. Todo novo caminho de aposentadoria deve fechar os dois grãos. Implementado.
3. **Aberta.** `valid_to` é nulo enquanto a versão for a última viva. Implementado.

Regras complementares, todas implementadas:

- **Nova escrita em caminho aposentado.** A nova versão é ligada à cadeia aposentada, e não inicia uma cadeia separada. O `valid_to` original da aposentadoria é preservado (não é movido para o instante da nova escrita). Assim o histórico continua íntegro e não há ressurreição de conhecimento aposentado.
- **Exclusão explícita.** Apagar a página remove suas versões. A linha do tempo não sobrevive, e `as_of` não encontra a página em nenhum instante, nem mesmo anterior à exclusão.
- **Expiração (TTL) é ortogonal.** A busca textual em `as_of` aplica a expiração avaliada no instante T. O ramo de entidades ignora a expiração, como já acontecia antes desta fase (ver [temporal.md](temporal.md)).

## 4. Armazenamento: colunas materializadas (decisão tomada)

Havia duas formas de obter a janela:

- **Visão ou junção derivada.** `valid_from` seria `created_at`, e `valid_to` seria a data de criação da sucessora, com `updated_at` como fallback para aposentadorias sem sucessora. Não exige migração. A desvantagem é que cada consulta `as_of` pagaria uma autojunção sobre os candidatos de busca textual, e a regra de instante de aposentadoria precisaria viver dentro da definição da visão.
- **Colunas materializadas.** Duas colunas anuláveis, um índice e um preenchimento único. Esta é a forma adotada.

**Decisão: colunas materializadas.** Os fatores decisivos foram a simetria de predicado entre os dois grãos (uma única semântica de janela, duas tabelas) e a possibilidade de o predicado de `as_of` usar índice. O custo de migração é real, mas limitado e já coberto pelo padrão da migração de janelas de vínculo e pelo backup prévio (seção 5).

A busca textual não precisa de reconstrução de índice em nenhuma das formas, porque os gatilhos de indexação já cobrem todas as versões, inclusive as superadas. Filtrar por versão é mudança de predicado de consulta, não de índice. Implementado.

## 5. Migração e preenchimento

### 5.1 Duas etapas (implementado)

- **Migração de esquema (V62).** Adiciona as duas colunas, anuláveis, e um índice sobre (workspace, projeto, `valid_from`, `valid_to`). Não altera dados. É apenas DDL, para que o passo de esquema seja rápido.
- **Preenchimento na abertura do armazenamento.** Roda de forma sequencial antes de o servidor aceitar tráfego, em lotes de 5.000 páginas. Cada lote é uma transação seguida de um checkpoint do log de escrita antecipada (WAL) que trunca o log. Assim o log nunca cresce além de um lote. O cursor é "páginas com `valid_from` nulo", que diminui a cada lote confirmado. Se for interrompido, retoma de onde parou. Em um armazenamento já preenchido, não há escrita.

Regras de preenchimento, que produzem o mesmo estado final da migração original:

- `valid_from` = `created_at` da própria versão.
- `valid_to`, se a versão tem sucessora: o menor `created_at` entre as sucessoras.
- `valid_to`, se a versão não é a última e não tem sucessora: o primeiro valor não nulo, nesta ordem: instante de decaimento; menor instante de fechamento de vínculo; `updated_at`.
- `valid_to` nulo se a versão é a última e não tem sucessora.

O preenchimento fazia parte da migração original, em uma única transação. Em um armazenamento de 242 GB (caso relatado), essa transação levou horas e fez o log crescer até perto do tamanho do banco, sem progresso visível. Por isso foi separado. Implementado.

### 5.2 Tolerância a checksum divergente (implementado)

A migração V62 foi reescrita in place. Armazenamentos que já a haviam aplicado com o texto anterior têm checksum diferente. O executor de migrações tolera esse checksum divergente em migração já aplicada. Continua recusando quando o armazenamento está à frente do binário, com erro de esquema "à frente", que identifica a migração e orienta a correção.

### 5.3 Backup antes da migração (implementado)

Na inicialização do servidor, antes de abrir o armazenamento e aplicar a migração de esquema, o sistema cria um arquivo de backup pré-migração se houver arquivos de página que ainda não estão no formato de conformidade OKF ([okf.md](okf.md)). O arquivo é reaberto e verificado (contagem de entradas). Se o backup ou a verificação falhar, a inicialização aborta antes de qualquer alteração de esquema.

Instalação nova, ou armazenamento já migrado, não gera backup, porque não há estado anterior a preservar.

### 5.4 Reversão (implementado)

Para voltar a uma versão anterior, restaure o backup verificado antes de iniciar essa versão. A versão anterior recusa abrir um armazenamento com migrações que ela não conhece. Reexecutar as migrações na versão atual não altera nada.

## 6. `as_of` com busca textual filtrada por versão (implementado)

### 6.1 A mudança de regra

Antes desta fase, `as_of` era uma consulta de entidades que pulava busca textual, vetorial e por grafo, sob o argumento de que misturar relevância atual com validade passada não responde nenhuma das duas perguntas com honestidade.

Esta fase adiciona busca textual sobre as versões vigentes em T, e a justificativa é a seguinte. A objeção original era sobre misturar **relevância no presente** com **validade no passado**. Ao restringir o corpus às versões cuja janela contém T, o conteúdo recuperado passa a ser histórico.

Limite assumido: a ordenação não é um retrato do passado. A pontuação textual (BM25) usa as estatísticas do índice atual, que inclui versões posteriores e outros projetos. Escritas posteriores podem mudar a ordem e o conjunto limitado de resultados mesmo com o mesmo T. Esta fase recupera versões históricas com relevância do índice atual, e não reproduz o que a busca teria retornado naquele momento.

### 6.2 Comportamento

- **Entrada.** `memory_query` com `as_of` em ISO-8601 (ver [api-reference.md](api-reference.md)). `as_of` não combina com `global` nem com `scopes`; a combinação retorna erro. Um instante inválido também retorna erro.
- **Ramo de entidades.** Mantém a lógica existente de janelas de vínculo: `valid_from` ≤ T e (`superseded_at` nulo ou > T). Sem filtro de expiração.
- **Ramo de texto.** Considera versões com `valid_from` ≤ T e (`valid_to` nulo ou > T). Aplica a expiração avaliada em T: uma página já expirada em T também já estava oculta em T.
- **Conjunto candidato.** Cada ramo traz no máximo max(4 × limite, 20) candidatos.
- **Fusão.** Reciprocal Rank Fusion com k = 60. Cada ramo soma 1 / (60 + posição) para cada página encontrada.
- **Ajuste de autoridade.** Após a fusão, o mesmo ajuste limitado do caminho padrão é aplicado. O fator fica entre 0,55 e 1,50. A etiqueta `superseded` limita o fator a 0,65, e a etiqueta `historical` limita a 0,80. Para páginas que só aparecem pelo ramo de entidades, a autoridade é lida da própria versão, sem filtro de versão mais recente, porque a consulta em T também ranqueia versões superadas.
- **Fora do modo `as_of`:** vetorial (embeddings são artefatos do texto atual e não têm escopo de versão), vizinhos de grafo (escopados à versão mais recente por construção) e o fallback de observações brutas.
- **Síntese de resposta.** Não é suportada em `as_of`. Se `answer` for pedido, a resposta traz um aviso de indisponibilidade, e não uma síntese.
- **Efeitos colaterais.** A consulta em `as_of` não altera estatísticas de acesso e não reordena pelo histórico de acesso. É uma leitura de auditoria.
- **`explain`.** Com `explain` ativo, cada resultado traz a posição e a contribuição de cada ramo, além do fator de autoridade. A lista `streams_active` vem com `["entity", "fts"]` somente quando `explain` é verdadeiro.
- **Caminho padrão (sem `as_of`).** Não muda. Busca apenas as versões mais recentes, como antes.

## 7. Fora da fase A

- **Fase B: tempo do mundo (proposta, não implementada).** Campos de verdade no mundo (`true_from`, `true_until`) com um nível de confiança no frontmatter. Foi adiada porque datar fatos por modelo de linguagem, sem fallback de confiança, não é confiável, e o caminho sem modelo nunca conseguiria preencher esses campos. A regra `NULL = desconhecido, usar tempo de ingestão` é proposta para o futuro e não existe no código.
- **Mudança de ranking no caminho padrão (não implementada).** A proposta de limitar, no ranking padrão, páginas superadas ou com validade expirada não foi feita. Razão: o caminho padrão só indexa versões com `is_latest = 1`, então versões superadas nunca entram no ranking. O problema real é o de páginas "desatualizadas mas ainda últimas", que é outro problema. A autoridade por etiqueta (`superseded`, `historical`) já cobre o caso de etiquetas de obsolescência. Qualquer sinal de frescor no ranking é proposta separada, que deve vir com números de avaliação e linha de base antes da mudança.
- **Arestas tipadas `contradicts` no ranking (proposta).** Não entram no ranking. Ficam como informação para uma futura fase, depois que as janelas de página existirem.

## 8. Critérios de aceitação e cobertura atual

Cobertura já existente, descrita por cenário:

- Consulta `as_of` dentro da janela de uma versão retorna essa versão pelo ramo de entidades. Cenário: a versão 1 cita uma entidade e a versão 2, criada depois, cita outra. Em T, entre as duas criações, só a versão 1 aparece.
- Consulta `as_of` funde os ramos de entidade e de texto em T.
- A sucessão fecha a janela da versão anterior no instante de criação da nova, e a última versão fica aberta.
- O preenchimento reconstrói as janelas a partir de um estado sem preenchimento. Com lotes de duas linhas, o resultado coincide com o do caminho de escrita. Reexecutar não altera nada.
- A aposentadoria por decaimento fecha as duas janelas e impede que `as_of` ressuscite a página.
- A exclusão explícita remove a linha do tempo, e `as_of` não encontra a página em nenhum instante.

A confirmar: um teste de ida e volta entre dois motores de banco com a mesma consulta `as_of` nos dois sentidos, citado no plano original. Não encontrei esse teste; existe um teste de troca de entidades, mas sem a ida e volta entre motores. A confirmar também: cobertura específica de fechamento de janelas nos caminhos de reorganização de projeto e de regeneração na movimentação de sessão.

## 9. Decisões registradas

As perguntas abertas do plano original foram resolvidas da seguinte forma:

1. **Versão de entrega.** A fase A foi entregue na versão 2.2.0. Novas funcionalidades seguem a linha 2.2, e não a linha 2.1.
2. **Par de colunas.** Foi adotado o par materializado `valid_from` + `valid_to`, pela simetria com o grão de vínculo (seção 4).
3. **Modo de fusão.** A busca textual e a de entidades são fundidas por RRF em `as_of`. Não há fallback de busca textual só quando o ramo de entidades não encontra nada (seção 6.2).
