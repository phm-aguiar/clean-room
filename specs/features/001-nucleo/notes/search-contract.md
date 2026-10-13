# Nota: detalhes de busca

Fontes abreviadas: SR = docs/storage-and-retrieval.md; DB = docs/database-schema.md; DV = docs/domain-vocabulary.md; PLAN = specs/features/001-nucleo/plan.md; SPEC = specs/features/001-nucleo/spec.md; TASKS = specs/features/001-nucleo/tasks.md. A constituição (.github/memory/constitution.md) só contribui com INV-03 (índices FTS, entidades e links commitam na mesma transação dos dados) e INV-04 (identidade tipada workspace_id, project_id, path).

## (a) Fluxos de busca e fusão RRF (SR §11.3; PLAN P-05; SPEC escopo "Busca")

- **Janela de candidatos:** `candidate_limit = clamp(limite × 4, mínimo 20, máximo limite + 300)`. Com limite 0, a janela é 0. A janela é larga porque a autoridade é aplicada depois da fusão (SR §11.3 passo 1).
- **Filtro comum:** todos os fluxos usam só versões correntes e não expiradas. A exceção é `include_superseded` verdadeiro. O instante de corte de expiração é entrada da busca (SR §11.3 passo 2 e §8 "Expiração").
- **Fluxo FTS5:** usa a consulta preparada (ver d), devolve trecho e rank BM25. O índice é `pages_fts(title, body, path_search)`, com `content='pages'` e tokenizer `unicode61 remove_diacritics 2 tokenchars '/_-'`. São obrigatórios três triggers (ai, ad, au), e o de update só dispara em mudança de title, body ou path_search (DB §1.1).
- **Fluxo de entidades:** a consulta é tokenizada. Tokens têm de 3 a 64 caracteres, no máximo 12 tokens, sem repetição. Termos compostos com `-` ou `_` também geram suas partes. Um token casa uma entidade se o nome é igual, começa com o token, ou contém o token logo após espaço, `-` ou `_`. Peso da página = soma de `1/freq` por entidade casada, onde `freq` é o número de páginas correntes do projeto com essa entidade. Ordem: peso desc., número de casamentos desc., caminho asc. (SR §11.3).
- **Fluxo de grafo (vizinhos de link):** as sementes são a união dos fluxos anteriores, sem repetição. Expande links nas duas direções. Links pendentes (sem destino) não são percorridos. O vizinho não pode ser semente. Ordem: posição da semente, direção (saída antes de entrada), `updated_at` desc., caminho asc. O tipo de aresta (`causes`, `fixes`, `contradicts`) é informativo e não altera o ranking (SR §11.3).
- **Fluxo de vetor:** o doc SR prevê vetor (e vetor de abstract opt-in), mas o PLAN P-05 define "sem fluxo de vetor (campo ausente, não stub)". Fora do escopo da 001.
- **Fusão RRF:** `score(d) = Σ sobre os fluxos de 1 / (60 + posição_no_fluxo(d))`, com posição a partir de 1, ou seja **k = 60** (SR §11.3 passo 3; SPEC). Depois `rank = −score`, convenção FTS5 em que menor é melhor.
- **Pós-fusão:** o passo 4 (títulos de páginas vindas só do vetor) não se aplica sem vetor. A autoridade é aplicada no passo 5 (ver b). Aplicação do fator (passo 6): se `rank ≤ 0`, multiplica por `fator`; se `rank > 0`, divide por `fator`. Fator maior sempre significa melhor posição. Passo 7: ordena por rank e, em empate, por caminho; trunca em `limit`; se `include_superseded`, marca as versões superseded.
- **Explicação (variante explicada):** devolve rank e pontuação por fluxo, contribuição RRF, entidades casadas, origem do grafo (semente, direção, aresta), fator de autoridade, confiança, quantidade de evidências e intenção (SR §11.3). LACUNA: o PLAN/SPEC não dizem se a 001 implementa a variante explicada.
- **Busca temporal `as_of`** (SR §11.4): só texto e entidades, com `valid_from ≤ T` e (`valid_to` nulo ou `> T`). Para vínculos de entidade, `valid_from ≤ T` e (`superseded_at` nulo ou `> T`). Não usa vetor, grafo nem busca crua e não incrementa contadores de acesso. LACUNA: não está claro se `as_of` entra na 001 (a SPEC não o cita).

## (b) Multiplicador de autoridade e tetos (SR §11.5; PLAN P-05)

- O fator parte de **1.0**, soma os ajustes abaixo e é limitado a **[0.55, 1.50]**. Nenhum sinal exclui a página.

| Sinal | Ajuste |
|---|---|
| Tipo `rule`, `decision` | +0.15 |
| Tipo `procedure`, `gotcha`, `preference` | +0.12 |
| Tipo `concept`, `slot` | +0.07 |
| Tipo `session` | −0.15 |
| Tier `semantic`, `procedural` | +0.10 |
| Tier `episodic` | −0.08 |
| Tier `working` | −0.03 |
| Página fixada (`pinned`) | +0.08 |
| Caminho `_lint/` | −0.20 |
| Caminho `investigations/` | −0.05 |
| Tag `canonical` ou `source-of-truth` | +0.20 |
| Tag `active` | +0.05 |

- **Tetos por tag** (aplicados depois dos ajustes): `do-not-answer-from` ou `test-fixture` dão fator máximo 0.55; `superseded` dá máximo 0.65; `historical` dá máximo 0.80.
- **Normalização de tags:** espaços removidos, minúsculas, `_` vira `-`.
- **Opt-in desligados por padrão** (provavelmente fora da 001): roteamento de recall de sessão (bônus padrão 0.25, intenção `session_recall`) e peso de crença (padrão 0.0, nunca para superseded). LACUNA: o PLAN não os menciona; a decisão de incluir ou não é do Analyst.
- Observação: a SPEC e o PLAN escrevem o intervalo com vírgula decimal ([0,55; 1,50]); o doc fonte usa ponto.

## (c) Regras de entidades (DV §6.5, §6.6; DB §4; SR §11.3)

- **Tamanho:** máximo 64 caracteres. Valor maior é rejeitado, não truncado. O processamento para ao ultrapassar o limite (sem alocação proporcional a entrada hostil). A validação de 64 caracteres brutos ocorre antes da normalização (DV §6.6).
- **Quantidade:** máximo 10 por página.
- **Normalização de uma entidade:** (1) aparar bordas; (2) colapsar espaços internos; (3) minúsculas; (4) rejeitar se vazio, com caractere de controle ou acima de 64. Isso torna o casamento lexical (`Postgres` = `postgres` = `POSTGRES`).
- **Normalização de lista:** descarta inválidas, remove duplicatas preservando a ordem e corta em 10.
- **Derivação do frontmatter:** une `entities` explícitas (vêm primeiro, para ganhar o limite) com `tags`. É tolerante: campo ausente ou não lista não contribui e não falha. A validação estrita fica na camada de escrita da wiki.
- **Schema (DB §4):** `entities(id, workspace_id, project_id, name CHECK length 1..64, created_at, UNIQUE(workspace_id, project_id, name))` e índice `idx_entities_name`. `entity_page_links(entity_id, page_id, valid_from, superseded_at, PK(entity_id,page_id))`, com índices por page_id e por (entity_id, valid_from, superseded_at). Trigger BEFORE INSERT exige mesmo workspace e projeto entre entidade e página.
- **Janela de validade:** `valid_from` = instante de criação da versão da página. `superseded_at` = instante de criação da versão que a substituiu (NULL = janela aberta). Também é preenchido quando a página é aposentada sem sucessora (decaimento, purga com regeração ou fusão). Na supersessão (caso B do P-04), as janelas da versão antiga são fechadas (`superseded_at = agora`) (SR §5 linha 162; PLAN P-04; DB §4).
- Entidades órfãs (sem nenhum vínculo) são removidas em limpeza (SR linha 185). Entidades de páginas escritas à mão ficam vazias (DV §2/§6 tabela de campos).

## (d) prepare_fts5_query (SR §11.2; PLAN P-05)

Objetivo: nunca produzir erro de consulta por causa do texto.

1. **Sintaxe explícita** é detectada se há aspas, parênteses, ou os operadores `OR`, `AND`, `NOT`, `NEAR` como palavras.
2. **Consulta nua** (sem sintaxe explícita): remove stop words e une os termos com **`OR`** (não AND), deixando o BM25 ordenar, para evitar recall próximo de zero em linguagem natural.
   - A lista é configurável; o padrão é uma lista embutida de **57 palavras em inglês**. A comparação ignora caixa, mas não diacríticos. Se a consulta só tem stop words, mantém todas.
   - LACUNA: as 57 palavras não estão listadas nos docs lidos; precisam ser definidas ou buscadas em outro doc.
3. **Dois-pontos sem coluna conhecida** (ex.: `pick: handoff`): o `:` vira espaço e o termo é citado. Só `title:` e `body:` são preservados como coluna.
4. **Tokens com pontuação ASCII** são citados como frase literal (ex.: `current.md`). Um `*` final (prefixo) passa sem citação.
5. **Validação e degradação:** a consulta candidata é testada por um analisador FTS5 real, em banco em memória com sonda por thread. Se passa, é usada. Se não, degrada para palavras citadas unidas por `OR`, sempre válido.
6. Consulta vazia (só espaços) retorna vazio; o chamador pula a consulta.
- LACUNA: o comportamento da sintaxe explícita (se passa direto à validação sem remover stop words e sem OR) é inferido do texto, não afirmado.

## (e) Reforço de acesso (SR §8; DB §4 e §1; SPEC; TASKS T036)

- **Throttle:** no máximo **1 por minuto por página e operador** (SPEC escopo "Busca"; TASKS T036; PLAN `Writer.BumpAccess`, via escritor único). LACUNA: SR/DB não detalham o mecanismo do throttle (onde fica o instante do último bump, se é por `page_access` ou memória). O schema de `page_access` não tem coluna de timestamp, então o throttle exigiria coluna nova ou estado em memória; é decisão de design a registrar. A única menção a "uma vez por minuto" em SR (linha 710) é sobre o campo de último uso de um token, não de página.
- **Efeito:** incrementa `pages.access_count` (INTEGER NOT NULL DEFAULT 0) e `pages.last_accessed_at` **somente em versões correntes** (SR §8; DB §1 colunas de decaimento).
- **Tabela `page_access` (DB §4):** `page_id BLOB REFERENCES pages(id) ON DELETE CASCADE`, `actor TEXT NOT NULL`, `PRIMARY KEY (page_id, actor)`, `WITHOUT ROWID`. Uma linha por página e operador, sem duplicar. `actor` é a chave de identidade do operador em TEXT, não FK (DB §0 "Texto de dono"). Só é registrada se há ator identificado; se a página não existe mais como corrente, a linha por operador não é criada (SR §8).
- A busca temporal `as_of` não incrementa contadores (SR §11.4).

## (f) Fallback raw_hits (SPEC; PLAN P-05 e §2.3; TASKS T037)

- Se nenhuma página compilada casar, a consulta devolve `raw_hits` por FTS nas observações do escopo, usando `observations_fts` (SPEC "Nenhuma página compilada casa"; PLAN P-05; TASKS T037).
- A tabela `observations` e `InsertObservation` entram na 001 só por causa desse fallback; a escrita real de observações é exercida apenas em testes até a feature 002 (PLAN nota 4 e checklist de abstração).
- SR lista a "busca crua" entre os fluxos não usados em `as_of` (§11.4).
- LACUNA: o schema de `observations_fts` está em DB §2 (linha ~271), que não foi lido em detalhe; não há spec do formato de `raw_hits`, nem do limite, nem se a consulta usa `prepare_fts5_query`.

## Estado das tasks relacionadas

T032 prepareFTS5Query, T036 BumpAccess, T037 Search (depende de T033, T034, T035) estão em specs/features/001-nucleo/tasks.md (linhas ~304, 340, 349).
