# plan.md: Plano de Implementação Técnica — Curadoria por embeddings (021)

> Plano gerado em 2026-10-13, antes de a 002 ser executada. Revisar quando a 020 terminar.

## 1. Metadados do Plano
- **Stack Tecnológico:** a da 016, sem dependência nova (`math`).
- **Feature Fonte:** `specs/features/021-curadoria-por-embeddings/spec.md`
- **Escopo:** deduplicação A3, filtro de entropia A4, banda de contradição, dream.

## 2. Design de Contratos e Fronteiras
- **`internal/domain/maintenance`:** `CosineDistance`, `AdaptiveEps`, `DBSCAN`, `Surprisal`, `ClassifyEntropy`.
- **`internal/wiki`:** `dedup.go` (chamado pela varredura agendada da 004), `contradiction_band.go` (chamado pelo lint), `dream.go`.
- **`internal/mcp`:** relógio de atividade monotônico carimbado em cada chamada de ferramenta.
- **`internal/cli`:** agendador do dream.
- **Configuração:** `[decay] dedup_*`, `contradiction_band_*`, `[dream]`, `[auto_improve.scheduler.experience_entropy_filter]`.

## 3. Decisões Arquiteturais e Justificativas

- **Decisão (D-01): agrupamento puro e compartilhado.** Dedup e dream usam as mesmas funções; testes de tabela fixam eps e clusters.
  - **Justificativa:** `design-memory-aging.md` §7.2 e §10.3 passo 4.
  - **Alternativa Rejeitada:** biblioteca de clustering.

- **Decisão (D-02): cancelamento cooperativo entre clusters.** Observador em goroutine compara o relógio de atividade a cada ~2 s e sinaliza um `atomic.Bool`; o laço checa antes de cada cluster; lote em andamento termina.
  - **Justificativa:** §10.2.
  - **Alternativa Rejeitada:** cancelar o `context` da chamada em voo (deixaria lote pela metade).

- **Decisão (D-03): sobrevivente e stubs num lote do escritor.** Um lote por cluster na A3 (todos os clusters num único lote, como o doc) e um lote por cluster no dream; evidência gravada na mesma transação.
  - **Justificativa:** §7.3 e §10.3 passo 7.
  - **Alternativa Rejeitada:** escritas separadas.

- **Decisão (D-04): contratos de teste.** Embedder sintético para vetores controlados; provedor falso para o dream; relógio injetado para a janela ociosa e o observador.

## 4. Auditoria de Constituição
- [ ] Build cruzado, `go vet`, `go test -race` (observador e cancelamento).
- [ ] INV-02 e INV-03 — lotes pelo escritor.
- [ ] INV-07, INV-08, INV-13.
- [ ] Nunca apagar fonte; supersessão pelo caminho de escrita.
- [ ] Goroutines com `context`, encerradas no fim do servidor.
