# plan.md: Plano de Implementação Técnica — LLM na leitura (019)

> Plano gerado em 2026-10-13, antes de a 002 ser executada. Revisar quando a 018 terminar.

## 1. Metadados do Plano
- **Stack Tecnológico:** a da 016, sem dependência nova.
- **Feature Fonte:** `specs/features/019-llm-na-leitura/spec.md`
- **Escopo:** reranqueador, `answer`/`reasoning`, `memory_explore`.

## 2. Design de Contratos e Fronteiras
- **`internal/llm/rerank.go`:** `Reranker` (`Rerank(ctx, query, []Candidate) ([]Score, error)`), prompt e esquema do §8.1.
- **`internal/llm/prompts/`:** textos de prompt versionados no código (reranqueador com a primeira frase fixa; digest; `answer` como decisão nossa).
- **`internal/mcp`:** orquestração no `memory_query` e no `memory_explore`; configuração `reranker`.

## 3. Decisões Arquiteturais e Justificativas

- **Decisão (L-01): reranqueamento com semáforo não bloqueante.** Canal com 4 vagas; `select` com `default` mantém a ordem local quando cheio; `context.WithTimeout` de 20 s.
  - **Justificativa:** `llm-client.md` §9.3.
  - **Alternativa Rejeitada:** fila de espera.

- **Decisão (L-02): completude conferida no chamador.** O reranqueador devolve o que sobrou do mapeamento; o `mcp` exige exatamente uma pontuação por candidato e só então reordena (empate pela posição original, excedentes de 30 anexados ao fim).
  - **Justificativa:** `llm-client.md` §9.1 e §9.2; `mcp-server.md` §3.1.
  - **Alternativa Rejeitada:** aceitar resposta parcial.

- **Decisão (L-03): prompts como constantes testadas.** A primeira frase do prompt do reranqueador tem teste de igualdade (adaptadores externos dependem dela).
  - **Justificativa:** `prompts-schemas-and-sanitizer.md` §8.1.
  - **Alternativa Rejeitada:** prompts em arquivo editável pelo operador.

- **Decisão (L-04): contratos de teste.** Provedor falso programável (atraso, resposta parcial, erro); tabela de baldes com relógio injetado; teste de concorrência do semáforo.

## 4. Auditoria de Constituição
- [ ] Build cruzado, `go vet`, `go test -race` (semáforo e timeout).
- [ ] INV-13 — sem provedor, respostas idênticas às da busca sem LLM, mais as notas do doc.
- [ ] INV-07 — rerank por esquema.
- [ ] Goroutines com `context`; nada de vazamento quando o rerank estoura.
