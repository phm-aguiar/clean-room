# Nota: detalhes de busca

Fontes abreviadas: SR = docs/storage-and-retrieval.md; DB = docs/database-schema.md; DV = docs/domain-vocabulary.md; SPEC = specs/features/001-nucleo/spec.md. A constituição (.github/memory/constitution.md) só contribui com INV-03 (índices FTS, entidades e links commitam na mesma transação dos dados) e INV-04 (identidade tipada workspace_id, project_id, path). Em toda dúvida vale o comportamento do original descrito nos docs.

## (a) Fluxos de busca e fusão RRF (SR §11.3; SPEC escopo "Busca")

- **Janela de candidatos:** `candidate_limit = clamp(limite × 4, mínimo 20, máximo limite + 300)`. Com limite 0, a janela é 0. A janela é larga porque a autoridade é aplicada depois da fusão (SR §11.3 passo 1).
- **Filtro comum:** todos os fluxos usam só versões correntes e não expiradas. A exceção é `include_superseded` verdadeiro. O instante de corte de expiração é entrada da busca (SR §11.3 passo 2 e §8 "Expiração").
- **Fluxo FTS5:** usa a consulta preparada (ver d), ordenada pelo rank BM25 padrão (sem pesos por coluna), limitada à janela; trecho = `snippet` da coluna `body` com `<mark>`/`</mark>`, reticências `…` e janela de **24** tokens. O índice é `pages_fts(title, body, path_search)`, com `content='pages'` e tokenizer `unicode61 remove_diacritics 2 tokenchars '/_-'`. São obrigatórios três triggers (ai, ad, au), e o de update só dispara em mudança de title, body ou path_search (DB §1.1).
- **Fluxo de entidades:** a consulta é cortada em todo caractere que não seja alfanumérico Unicode, `-` ou `_`; cada pedaço perde `-` e `_` das pontas e é descartado se vazio; tamanho de **3 a 64** caracteres Unicode medido antes das minúsculas (Unicode completas); token fora do tamanho é só pulado; o composto entra antes das partes (cortadas em `-` e `_`); para em **12** tokens distintos. Um token casa uma entidade do projeto se o nome é igual, começa com o token, ou contém o token logo após espaço, `-` ou `_`. Peso da página = soma de `1/freq` por entidade casada, onde `freq` é o número de páginas do projeto com essa entidade sob o mesmo filtro de versão da busca (correntes e não expiradas; todas as versões não expiradas com `include_superseded`). Ordem: peso desc., número de casamentos desc., caminho asc. Trecho = descritor da página (SR §11.3).
- **Fluxo de vetor:** só com vetor de consulta; vetores do projeto com o mesmo `(provedor, modelo, dimensão)`; similaridade por produto escalar; vetor de abstract é opt-in. Sem embedder (feature 017), o fluxo não participa.
- **Fluxo de grafo (vizinhos de link):** sementes = páginas do fluxo FTS, depois vetorial, vetor de abstract e entidades, nessa ordem, sem repetição. Expande links nas duas direções, só no mesmo workspace e projeto. Links pendentes não são percorridos. O vizinho não pode ser semente; página alcançada por várias sementes conta só na primeira posição; para ao chegar à janela. Ordem: posição da semente, direção (saída antes de entrada), `updated_at` desc., caminho asc. O tipo de aresta é informativo e não altera o ranking.
- **Fluxo de observações** (ligado por padrão, `observation_stream`): consulta preparada em `observations_fts` do projeto, rank BM25, até `min(janela × 4, 1000)` linhas; agrupa por sessão na ordem da primeira aparição, com o trecho da melhor observação (janela de **16** tokens) e até 3 ids; cada sessão vota na página `sessions/<id>.md` corrente e não expirada (sessão sem página é pulada); para na janela; sessão escondida pelo filtro de dono vota sem trecho nem ids. Entra junto das páginas de sessão (feature 002).
- **Fusão RRF:** `score(d) = Σ sobre os fluxos de 1 / (60 + posição_no_fluxo(d))`, com posição a partir de 1, ou seja **k = 60** (SR §11.3 passo 3; SPEC). Depois `rank = −score`, convenção FTS5 em que menor é melhor. Página com voto do fluxo de observações tem contribuição **zero** do fluxo FTS (a posição FTS continua ocupada); o trecho é o da FTS se ela entrou por lá, senão o do voto (e o título vem do voto se faltava).
- **Pós-fusão:** passo 4: páginas sem título (só vetor, ou voto sem trecho) recebem título e descritor (600 caracteres de entrada, linhas puladas, 240 de saída com `…`; SR §11.3), sem substituir trecho de observação já escolhido. Passo 5: autoridade (ver b); dados de autoridade de quem não veio da FTS são lidos num único acesso, e página sem esses dados fica sem fator. Aplicação do fator (passo 6): se `rank ≤ 0`, multiplica por `fator`; se `rank > 0`, divide por `fator`. Fator maior sempre significa melhor posição. Passo 7: ordena por rank e, em empate, por caminho (comparação de bytes); trunca em `limit`; se `include_superseded`, marca as versões superseded.
- **Vários escopos:** cada escopo buscado com o mesmo limite; união mantém o melhor rank da página; ordena por rank, caminho, id; corta em `limit`.
- **Explicação (variante explicada):** devolve rank e pontuação por fluxo, contribuição RRF, entidades casadas, origem do grafo (semente, direção, aresta), fator de autoridade, confiança, quantidade de evidências e intenção (SR §11.3). Faz parte do contrato do original; a superfície que a expõe vem com a 002.
- **Busca temporal `as_of`** (SR §11.4): só texto e entidades, com `valid_from ≤ T` e (`valid_to` nulo ou `> T`). Para vínculos de entidade, `valid_from ≤ T` e (`superseded_at` nulo ou `> T`). Não usa vetor, grafo nem busca crua e não incrementa contadores de acesso. A superfície `as_of` é da feature 005.

## (b) Multiplicador de autoridade e tetos (SR §11.5)

- O fator parte de **1.0**, soma os ajustes abaixo e é limitado a **[0.55, 1.50]**. Nenhum sinal exclui a página.
- O "tipo" usado aqui é o `kind` do frontmatter convertido para texto quando presente e não nulo (sem aparar, comparação sensível à caixa; `kind: 5` vale `"5"`), senão o prefixo do caminho comparado sem distinguir maiúsculas ASCII (`_rules/`, `_slots/`, `sessions/`, `decisions/`, `gotchas/`, `concepts/`, `procedures/`, `notes/`, `profile/`), senão `fact` (SR §11.5). Nível da coluna `tier`, fixação da coluna `pinned`.

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
- **Tags:** da lista `tags` do frontmatter; itens não texto ignorados; só as pontas aparadas (espaços internos ficam), letras **ASCII** em minúsculas, `_` vira `-`. Cada ajuste e cada teto valem no máximo uma vez por página (`canonical` + `source-of-truth` somam +0.20 uma vez).
- **Ordem de cálculo:** soma a partir de 1.0, aplica o menor teto presente, limita a [0.55, 1.50]. `_lint/` e `investigations/` são exclusivos.
- **Opt-in desligados por padrão:** roteamento de recall de sessão (bônus padrão 0.25, intenção `session_recall`, lista de marcadores em SR §11.5) e peso de crença (padrão 0.0, nunca para superseded; confiança só calculada com `explain` ou peso positivo). Ficam na feature 005, com os mesmos padrões do original.
- Observação: a SPEC escreve o intervalo com vírgula decimal ([0,55; 1,50]); o doc fonte usa ponto.

## (c) Regras de entidades (DV §6.5, §6.6; DB §4; SR §11.3)

- **Tamanho:** máximo 64 caracteres. Valor maior é rejeitado, não truncado. O processamento para ao ultrapassar o limite (sem alocação proporcional a entrada hostil). A validação de 64 caracteres brutos ocorre antes da normalização (DV §6.6).
- **Quantidade:** máximo 10 por página.
- **Normalização de uma entidade:** (1) aparar bordas; (2) colapsar espaços internos; (3) minúsculas; (4) rejeitar se vazio, com caractere de controle ou acima de 64. Isso torna o casamento lexical (`Postgres` = `postgres` = `POSTGRES`).
- **Normalização de lista:** descarta inválidas, remove duplicatas preservando a ordem e corta em 10.
- **Derivação do frontmatter:** une `entities` explícitas (vêm primeiro, para ganhar o limite) com `tags`. É tolerante: campo ausente ou não lista não contribui e não falha. A validação estrita fica na camada de escrita da wiki.
- **Schema (DB §4):** `entities(id, workspace_id, project_id, name CHECK length 1..64, created_at, UNIQUE(workspace_id, project_id, name))` e índice `idx_entities_name`. `entity_page_links(entity_id, page_id, valid_from, superseded_at, PK(entity_id,page_id))`, com índices por page_id e por (entity_id, valid_from, superseded_at). Trigger BEFORE INSERT exige mesmo workspace e projeto entre entidade e página.
- **Janela de validade:** `valid_from` = instante de criação da versão da página. `superseded_at` = instante de criação da versão que a substituiu (NULL = janela aberta). Também é preenchido quando a página é aposentada sem sucessora (decaimento, purga com regeração ou fusão). Na supersessão (caso B do upsert, SR §5.1), as janelas da versão antiga são fechadas (`superseded_at = agora`) (SR §5 linha 162; DB §4).
- Entidades órfãs (sem nenhum vínculo) são removidas em limpeza (SR linha 185). Entidades de páginas escritas à mão ficam vazias (DV §2/§6 tabela de campos).

## (d) Preparação segura da consulta FTS (SR §11.2)

Objetivo: nunca produzir erro de consulta por causa do texto. Resumo do contrato (o doc tem o detalhe):

"Token" = trecho separado por espaço em branco Unicode.

1. **Sintaxe explícita:** aspas, `(`, `)`, ou algum token **exatamente** `OR`, `AND`, `NOT` ou `NEAR` em maiúsculas (`or`, `NOT,` não contam).
2. **Palavras de parada** (só sem sintaxe explícita): comparadas no token bruto, antes de pontuação e coluna (`the,`, `title:the`, `the*` nunca são removidos), em minúsculas Unicode completas e sem remover diacríticos. A lista padrão tem 57 palavras e está no doc. A chave de configuração ausente usa a padrão; lista configurada substitui; lista vazia explícita desliga o filtro. Se todos os tokens forem palavras de parada, ficam todos.
3. **Por token** (com e sem sintaxe explícita): coluna desconhecida (com `:`, sem aspas, sem começar exatamente por `title:`/`body:`, sensível à caixa) tem cada `:` trocado por espaço e cada pedaço vira termo citado; coluna conhecida com valor que precisa de citação mantém a coluna e cita só o valor; precisa de citação o token que não está entre aspas e tem pontuação ASCII diferente de `:` (ignorando **um** `*` final). Os demais passam como estão.
4. **Citação:** termo com aspas tem cada `"` dobrado e vai entre aspas, sem variante; `*` final (com pelo menos uma letra ou dígito antes) sai para depois de cada fechamento; variante dividida troca cada pontuação ASCII por espaço — vazia ou igual ao termo gera só `"termo"`, senão `("termo" OR "variante")`.
5. **Junção:** ` OR ` sem sintaxe explícita; um espaço, na ordem original, com sintaxe explícita.
6. **Degradação:** a candidata é testada numa tabela FTS5 em memória (`title`, `body`), com conexão de sonda aberta uma vez e reaproveitada (uma por thread de leitura). Se ela recusa, ou se a sonda não abre, a consulta **inteira** vira saco de palavras: em cada token bruto `"`, `(` e `)` viram espaço, cada pedaço passa pela regra 4 (com variante) e os pedaços são unidos por ` OR `, sem filtro de palavras de parada nem coluna. Frases e operadores não sobrevivem.
7. Consulta vazia, ou sem nenhum termo depois do tratamento, devolve vazio.

## (e) Reforço de acesso (SR §8; DB §4 e §1; SPEC)

- **Throttle:** no máximo **1 por 60 s por par (página, operador)**, com estado só em memória, na camada que atende a leitura (SR §8). O anônimo é um operador próprio: **leitura anônima reforça o contador compartilhado** e só não grava linha em `page_access`. Par ainda na janela é pulado **sem** renovar o instante; entradas vencidas são descartadas a cada chamada.
- **Execução:** em segundo plano, depois da resposta (falha só gera aviso); leitura que terminou em erro não reforça; várias páginas numa transação; id desconhecido é ignorado sem derrubar o lote.
- **Efeito:** incrementa `pages.access_count` (INTEGER NOT NULL DEFAULT 0) e `pages.last_accessed_at` **somente em versões correntes** (SR §8; DB §1 colunas de decaimento).
- **Tabela `page_access` (DB §4):** `page_id BLOB REFERENCES pages(id) ON DELETE CASCADE`, `actor TEXT NOT NULL`, `PRIMARY KEY (page_id, actor)`, `WITHOUT ROWID`. Uma linha por página e operador, sem duplicar. `actor` é a chave de identidade do operador em TEXT, não FK (DB §0 "Texto de dono"). Só é registrada se há ator identificado; se a página não existe mais como corrente, a linha por operador não é criada (SR §8).
- A busca temporal `as_of` não incrementa contadores (SR §11.4).

## (f) Fallback raw_hits (SPEC; SR §11.3.1)

- Feito pela camada que atende a consulta (a ferramenta MCP de busca), não pela busca híbrida. Quando a busca de páginas devolve **nenhum** resultado, a mesma consulta preparada (d) roda em `observations_fts` do escopo, em ordem de rank BM25, com o mesmo limite da consulta e trecho de 24 tokens. Se houver qualquer página, `raw_hits` vem vazio.
- Filtro de dono: só observações de sessões sem dono ou do próprio chamador (quando a implantação distingue operadores). Com vários escopos explícitos, busca em cada um, junta, ordena por rank e corta no limite.
- A tabela `observations` entra na 001 por causa desse fallback; a escrita real de observações vem com a feature 002.
- A "busca crua" não é usada em `as_of` (SR §11.4). Schema de `observations_fts`: DB §2.
