# Spec: Validade temporal (as_of), arestas tipadas e sinais de ranking opcionais

## Metadados
- **ID:** 005
- **Status:** draft (revisada contra os docs em 2026-10-13; nova revisão prevista quando as features anteriores terminarem)
- **Autor:** phm-aguiar
- **Data:** 2026-10-13
- **Feature Anterior:** 002 (`memory_query`, `memory_write_page`, `memory_status`), 004 (aposentadoria por decaimento que fecha janelas)
- **Roadmap:** `specs/features/roadmap.md`
- **Aprovado:** false

## Propósito
> Para quem, o quê e por quê.

Responder "o que a memória considerava atual naquele instante?" para auditorias e análises pós-incidente, e deixar explícito no grafo de que tipo é uma relação entre páginas (`causes`, `fixes`, `contradicts`). A 001 já grava as janelas de ingestão de página e de vínculo; falta a consulta `as_of` com seus dois fluxos, e falta completar as arestas tipadas na escrita pública, na explicação da busca e no status.

## Escopo

### Dentro do escopo
- **`as_of` em `memory_query`** (`docs/temporal.md`, `docs/design-page-ingestion-windows.md` §6.2):
  - entrada ISO-8601 com data, hora e fuso; texto vazio equivale a ausência; valor inválido é erro; combinar com `global` ou `scopes` é erro;
  - fluxo de entidades pelas janelas de vínculo (`valid_from ≤ T` e `superseded_at` nulo ou `> T`), sem filtro de expiração, com as regras de termo e de casamento do `temporal.md` (3 a 64 caracteres, até 12 termos, partes de compostos, casamento por igualdade, prefixo do nome ou prefixo de palavra do nome, peso `1/N`);
  - fluxo de texto pelas janelas de página (`valid_from ≤ T` e `valid_to` nulo ou `> T`), com a expiração avaliada em T;
  - até `max(4 × limite, 20)` candidatos por fluxo, fusão RRF com k = 60, ajuste de autoridade em `[0,55; 1,50]` com teto 0,65 para `superseded` e 0,80 para `historical`, autoridade lida da própria versão para páginas que só vêm do fluxo de entidades;
  - sem vetor, grafo, observações brutas nem reforço de acesso; `answer` pedido traz aviso em `answer_unavailable`;
  - `explain` com posição e contribuição de cada fluxo, fator de autoridade e `streams_active = ["entity", "fts"]` só quando `explain` é verdadeiro.
- **Arestas tipadas** (`docs/typed-edges.md`):
  - campo `relations` nos metadados da escrita pública (`memory_write_page`), com os limites (até 32 alvos no total, 1024 caracteres por alvo, sem caractere de controle, qualificador com no máximo uma `/`, alvo portável) e rejeição da escrita inteira quando um alvo quebra uma regra;
  - no frontmatter de arquivo, chave de relação desconhecida e alvo inválido são **ignorados com aviso**, sem limite de quantidade;
  - em toda nova versão de um caminho (supersessão, ressurreição ou cadeia nova), todas as linhas de link cujo alvo casa com o caminho são reapontadas para ela, inclusive as já resolvidas (`docs/typed-edges.md` § Storage); a 001 só faz isso na supersessão;
  - campo `edge` no `explain` da busca para saltos de grafo cujo rótulo não é `references`;
  - contagem por relação dos links tipados vindos de versões atuais no status.

- **Sinais de ranking opcionais** (`docs/storage-and-retrieval.md` §11.5 e §13, `docs/architecture.md` § `[retrieval]`, `docs/mcp-server.md` §3.2, `docs/design-hindsight-borrowings.md` P2 e P4; nenhum depende de LLM e todos ficam desligados por padrão):
  - **confiança derivada na leitura**, nunca armazenada: amplitude por sessões distintas (`session` e `reconsolidation`) mais crédito de 0,2 por evidência residual com teto 1,0; suporte `1 − e^(−amplitude/3)`; recência `0,5 + 0,5·e^(−idade/30 dias)` (1,0 sem instante, idade negativa vira zero); penalidade `1/(1 + contradições vivas)`, contando só arestas `contradicts` para página atual; teto 0,95; sem evidência, 0;
  - **peso de crença** `[retrieval].belief_authority_weight` (padrão 0,0): soma `peso × confiança` ao fator de autoridade dentro de `[0,55; 1,50]`; com peso 0 e sem `explain`, nenhuma consulta de evidência no caminho de busca; versões superseded nunca recebem o bônus;
  - **roteamento de recall de sessão** `[retrieval].query_intent` (desligado) e `session_recall_bonus` (0,25): consulta com marcador léxico de recall de sessão devolve às páginas `session` a penalidade de tipo e de tier e soma o bônus; intenção `session_recall` no `explain`. Os marcadores são os exemplos do doc ("上次", "last time", "what did we", "sessão anterior", "decisão anterior", "onde paramos ontem"); a lista completa é **decisão nossa** a partir deles;
  - `explain` com `evidence_count`, `confidence`, `belief_factor` (quando o peso está ativo) e a intenção; a confiança é calculada no `explain` mesmo com o peso desligado;
  - **`settled_first`** no `memory_briefing` (padrão falso): lista `settled` com até 8 páginas atuais de regra ou decisão, não expiradas, por contagem de evidência e depois recência;
  - `evidence_rows` (total de registros de evidência do projeto) no status.

### Fora do escopo (explicitamente)
- Fase B, tempo do mundo (`true_from`, `true_until`): adiada pelo próprio doc.
- Mudança de ranking no caminho padrão por frescor ou por `contradicts`, peso por tipo de aresta e travessia de cadeias `fixes`/`causes` (propostas nos docs, não implementadas).
- Preenchimento de janelas e de vínculos de dados antigos na abertura do armazenamento: o esquema V01 já nasce com as colunas e toda escrita as preenche; não há loja antiga a migrar (decisão nossa).
- Lint de `contradicts` e de `broken_link` (feature 004).

## Comportamento Esperado

### Cenário Principal (Happy Path)
1. Em junho, `notes/db.md` diz "usamos postgres" (entidade `postgres`). Em agosto, a página é substituída por "migramos para sqlite" (entidade `sqlite`).
2. `memory_query {"query": "postgres"}` não traz a página, porque o conhecimento atual mudou.
3. `memory_query {"query": "postgres", "as_of": "2026-07-01T00:00:00Z"}` traz a versão de junho, pelo fluxo de entidades.
4. Uma pergunta sem o termo exato ("qual banco usávamos?") com o mesmo `as_of` encontra a versão de junho pelo fluxo de texto.

### Cenários Alternativos
- **`as_of` inválido:** erro.
- **`as_of` com `global` ou `scopes`:** erro.
- **`as_of` vazio ou só espaços:** consulta normal.
- **`relations` na escrita pública com chave desconhecida, alvo inválido, mais de 32 alvos ou alvo acima de 1024 caracteres:** a escrita inteira é rejeitada com erro de validação e nada é salvo.
- **Chave de relação desconhecida no frontmatter do arquivo:** a chave é ignorada com aviso e a página é indexada.

## Edge Cases
- **Instante exato de supersessão:** vale a versão nova (intervalo meio-aberto).
- **Aposentadoria por decaimento:** fecha as duas janelas, e `as_of` não ressuscita a página depois do instante da retirada.
- **Exclusão explícita:** remove a linha do tempo; `as_of` não encontra a página em nenhum instante.
- **Ressurreição:** entre o `valid_to` do tombstone e a criação da nova versão, nenhuma versão daquele caminho vale.
- **Ranking em `as_of`:** usa as estatísticas do índice atual; a ordem pode mudar com escritas posteriores para o mesmo T. Os testes conferem o conjunto, não a ordem exata entre fluxos.
- **Mesmo alvo com rótulos diferentes:** são linhas separadas; o mesmo alvo com o mesmo rótulo numa página é gravado uma vez.

## Constraints
- **Performance:** cada fluxo limitado a `max(4 × limite, 20)` candidatos; consulta `as_of` sem efeito colateral de escrita.
- **Tecnologia:** sem dependência nova; SQL sobre as colunas de janela da V01.

## Conflito entre docs (registrado)
`docs/domain-vocabulary.md` §6.8 diz que nome de relação desconhecido "fica como referência simples"; `docs/wiki-files-layer.md` §3.5 e `docs/typed-edges.md` dizem que a chave desconhecida é ignorada com aviso, "para que um erro de digitação não crie um tipo de aresta novo". A 001 seguiu o primeiro. Esta feature segue o doc específico de arestas tipadas, que tem a concordância do doc da camada de arquivos.

## Critérios de Sucesso
- [ ] Os cenários de aceitação de `docs/design-page-ingestion-windows.md` §8 que se aplicam a uma loja nova passam como teste
- [ ] O exemplo de `docs/temporal.md` (postgres e sqlite) passa pelos dois fluxos
- [ ] `relations` na escrita pública respeita todos os limites e rejeita a escrita inteira em violação
- [ ] `explain` traz `edge` e `streams_active` como no doc; status traz a contagem por relação
- [ ] `scripts/check.sh` e o lint passam sem apontamentos

## Abordagem Escolhida
Um caminho de consulta `as_of` separado dentro da busca do `storage`, com os dois fluxos filtrados pelas janelas e a mesma fusão RRF e o mesmo ajuste de autoridade do caminho padrão. Validação de `relations` na fronteira da escrita pública, reaproveitando a gramática de alvo da 001. A correção da relação desconhecida fica na derivação de links da wiki.

## Dependências
- 001: janelas de página e de vínculo, derivação de links e relações, autoridade.
- 002: `memory_query`, `memory_write_page`, `memory_status`.
- 004: aposentadoria por decaimento (para o caso de borda de `as_of` depois da retirada).

## Checklist de Prontidão
- [x] Propósito claro e sem ambiguidade
- [x] Escopo delimitado (dentro/fora)
- [x] Cenários cobrem happy path e alternativos
- [x] Edge cases identificados com comportamento esperado
- [x] Constraints explícitas
- [x] Critérios de sucesso mensuráveis
- [x] Abordagem escolhida justificada
