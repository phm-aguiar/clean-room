# plan.md: Plano de Implementação Técnica — Validade temporal e arestas tipadas (005)

> Plano gerado em 2026-10-13, antes de a 002 ser executada. Revisar quando as features anteriores terminarem.

## 1. Metadados do Plano
- **Stack Tecnológico:** a da 002, sem dependência nova. `time.Parse` com `time.RFC3339Nano` para `as_of`.
- **Feature Fonte:** `specs/features/005-validade-temporal/spec.md`
- **Escopo:** `as_of` em `memory_query` com os fluxos de entidade e de texto; `relations` na escrita pública com limites; relação desconhecida no arquivo ignorada com aviso; `edge` no `explain`; contagem por relação no status.
- **Fontes de contrato:** `docs/temporal.md`, `docs/design-page-ingestion-windows.md` §6 e §8, `docs/typed-edges.md`, `docs/wiki-files-layer.md` §3.5, `docs/mcp-server.md` §3.1.

## 2. Design de Contratos e Fronteiras
- **Contrato:** parâmetro `as_of` de `memory_query`; campo `relations` nos metadados de `memory_write_page` (`{causes|fixes|contradicts: [alvo, ...]}`); campos `edge` no `explain` e contagem por relação no `memory_status`.
- **Esquema SQL:** sem migração. Índices de janela da V01 (`pages.valid_from`/`valid_to`, `entity_page_links.valid_from`/`superseded_at`).

### 2.1 Interface no `storage`
```go
// SearchAsOf busca o conhecimento vigente em t. Sem vetor, grafo, observações brutas
// nem reforço de acesso. Cada fluxo traz até max(4*limit, 20) candidatos.
func SearchAsOf(ctx context.Context, r Reader, q SearchQuery, t time.Time) (SearchResult, error)
```

## 3. Decisões Arquiteturais e Justificativas

- **Decisão (V-01): caminho de consulta `as_of` separado do caminho padrão.** `memory_query` com `as_of` válido chama `SearchAsOf`; sem `as_of`, nada muda. Os dois caminhos compartilham a fusão RRF (k = 60) e o ajuste de autoridade da 001.
  - **Justificativa:** o doc proíbe vetor, grafo, observações brutas e reforço de acesso em `as_of`; separar evita condicionais espalhadas no caminho padrão.
  - **Alternativa Rejeitada:** parâmetro `asOf` opcional no `Search` padrão (mistura regras e arrisca mudar o caminho padrão).

- **Decisão (V-02): fluxo de entidades com as regras de termo do `temporal.md`.** Separação em trechos de letras, números, hífen e sublinhado; bordas aparadas; minúsculas; 3 a 64 caracteres; até 12 termos; partes de compostos; casamento por igualdade, prefixo do nome ou prefixo de palavra do nome (separadores espaço, hífen e sublinhado); peso `1/N` com N contado em T; ordenação por peso, termos casados e caminho. Conferir se o fluxo de entidades do caminho padrão da 001 usa a mesma regra; se divergir, alinhar ao doc.
  - **Justificativa:** `docs/temporal.md` § "Dois fluxos".
  - **Alternativa Rejeitada:** reaproveitar o fluxo padrão sem conferir (o casamento por prefixo de palavra pode não existir lá).

- **Decisão (V-03): autoridade da própria versão.** Para candidatas que só vêm do fluxo de entidades, o ajuste de autoridade lê frontmatter, tier e `pinned` da própria versão vigente em T, sem filtro de versão mais recente, com os tetos de `superseded` (0,65) e `historical` (0,80).
  - **Justificativa:** `docs/design-page-ingestion-windows.md` §6.2.
  - **Alternativa Rejeitada:** usar a autoridade da versão atual (ranquearia o passado com dados do presente).

- **Decisão (V-04): `relations` validado na fronteira da escrita pública.** `memory_write_page` aceita `relations` nos metadados, valida chaves (vocabulário fechado, sensível a maiúsculas), tipo lista, até 32 alvos, 1024 caracteres por alvo, sem controle, qualificador com até uma `/` e sem componente vazio, `.` ou `..`, alvo portável; qualquer violação rejeita a escrita inteira. O valor validado entra no frontmatter da página, e a derivação de links da 001 cria as arestas.
  - **Justificativa:** `docs/typed-edges.md` § Limits e § "Where invalid input goes".
  - **Alternativa Rejeitada:** aplicar os limites também ao arquivo (o doc diz que arquivo e consolidador não têm limite).

- **Decisão (V-05): relação desconhecida no arquivo é ignorada com aviso.** A derivação de links da wiki deixa de gravar a relação desconhecida como `references`: a chave é pulada com aviso (valor truncado a 200 bytes). Alvo que não nomeia página também é pulado com aviso.
  - **Justificativa:** `docs/typed-edges.md` e `docs/wiki-files-layer.md` §3.5; conflito com `docs/domain-vocabulary.md` §6.8 registrado na spec.
  - **Alternativa Rejeitada:** manter o comportamento da 001 (deixa um erro de digitação virar link sem que o lint perceba).

- **Decisão (V-06): `edge` e contagem por relação.** O grafo da busca da 001 passa a carregar o rótulo do link do salto; o `explain` mostra `edge` quando o rótulo não é `references`. O status conta, por relação, os links tipados de versões atuais.
  - **Justificativa:** `docs/typed-edges.md` § "How retrieval uses them".
  - **Alternativa Rejeitada:** novo fluxo por relação (o doc diz que o rótulo não muda o ranking).

- **Decisão (V-07): contratos de teste.** Cenários do `design-page-ingestion-windows.md` §8 aplicáveis a loja nova; exemplo postgres/sqlite; intervalo meio-aberto no instante da supersessão; ressurreição sem versão válida no intervalo; decaimento e exclusão; limites de `relations` um a um; aviso de relação desconhecida no arquivo.
  - **Justificativa:** o doc lista essas coberturas como critério de aceitação.
  - **Alternativa Rejeitada:** comparar a ordem exata dos resultados (o doc diz que a ordem não reproduz o passado).

### O que não construir nesta feature
Fase B (tempo do mundo), ranking por frescor ou por `contradicts`, preenchimento de dados antigos.

## 4. Auditoria de Constituição

**Portões de Qualidade**
- [ ] Build cruzado, `go vet`, `go test -race` — V-07.
- [ ] Teste multi-sessão — supersessão concorrente com consulta `as_of` no instante da troca (V-07).
- [ ] Gherkin — os cenários da spec viram o `.feature` da 005.
- [ ] Dependência nova — nenhuma.

**Restrições Arquiteturais**
- [ ] Grafo de imports — `storage` continua só com `domain`; `mcp` chama `storage`.
- [ ] INV-03 — `as_of` é só leitura; sem escrita de acesso.
- [ ] INV-04 — as consultas filtram por `(workspace_id, project_id)`.
- [ ] INV-13 — sem vetor e sem síntese.
- [ ] INV-16 — leitura nunca filtra por autoria.
- [ ] Resolução de escopo — `as_of` usa a resolução de leitura da 002 e recusa `global` e `scopes`.

**Anti-Padrões Proibidos**
- [ ] Sem abstração preemptiva — V-01 é um caminho concreto, sem interface nova.
- [ ] "A confirmar" como decisão — aceitação de `as_of` só com data: recusada (exige data, hora e fuso, como diz o doc; a forma só data fica "a confirmar" no doc e é decisão nossa recusar).
