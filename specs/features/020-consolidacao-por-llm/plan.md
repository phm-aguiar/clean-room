# plan.md: Plano de Implementação Técnica — Consolidação por LLM (020)

> Plano gerado em 2026-10-13, antes de a 002 ser executada. Revisar quando a 019 terminar.

## 1. Metadados do Plano
- **Stack Tecnológico:** a da 016, sem dependência nova (`golang.org/x/text/unicode/norm`, já no `go.mod`, para o slug com decomposição NFD).
- **Feature Fonte:** `specs/features/020-consolidacao-por-llm/spec.md`
- **Escopo:** tipos, projeção, orçamento, página única, lote, fila, trabalhador, checkpoints, `memory_consolidate`, bootstrap, lint por LLM, perfil por LLM.

## 2. Design de Contratos e Fronteiras
- **`internal/domain/consolidate`:** tipos de saída, `ProjectObservations`, `Budget`, `RuleSlug`, `SanitizeModelPath`, validação do §10.2.
- **`internal/wiki`:** `ConsolidateSession`, `ConsolidateBatch`, `Bootstrap`, extensão do lint e da passada de perfil, todos recebendo `llm.Provider` (ou nulo).
- **`internal/storage`:** fila `session_consolidation_jobs`; progresso de bootstrap.
- **Rotas:** `POST /admin/bootstrap`, `no_llm` em `/admin/lint`; ferramenta `memory_consolidate`; comando `bootstrap`.
- **Esquema SQL:** sem migração.

## 3. Decisões Arquiteturais e Justificativas

- **Decisão (K-01): preflight de admissão e `dry_run` antes de qualquer chamada.** Toda operação que gasta modelo pergunta à cadeia de admissão da 008 com a operação `consolidate` e devolve o plano em `dry_run` sem chamar o provedor.
  - **Justificativa:** `consolidation-llm.md` § Notas, item 3.
  - **Alternativa Rejeitada:** checar admissão só na escrita.

- **Decisão (K-02): retentativa no consumidor, não no cliente.** Até 3 tentativas com 2 s para transitórios, usando a classificação da 016; o id de operação é o mesmo nas tentativas e novo a cada invocação.
  - **Justificativa:** `llm-client.md` §1 princípio 3; `consolidation-llm.md` §3.4.
  - **Alternativa Rejeitada:** retentativa dentro do provedor.

- **Decisão (K-03): saída do modelo como entrada não confiável.** Caminhos sanitizados, enums normalizados, tamanhos limitados; um item ruim é pulado e o lote segue.
  - **Justificativa:** `consolidation-llm.md` § Notas, item 5.
  - **Alternativa Rejeitada:** abortar o lote no primeiro item inválido.

- **Decisão (K-04): fila durável com `claim_id` pelo escritor.** Reivindicação em transação `IMMEDIATE`; completar, falhar e liberar exigem o `claim_id`; esperas de 30, 60, 120 e 240 s calculadas pelo trabalhador.
  - **Justificativa:** `docs/storage-and-retrieval.md` §15.
  - **Alternativa Rejeitada:** fila em memória.

- **Decisão (K-05): bootstrap em duas metades.** O cliente coleta e poda as fontes (git, README, `docs/`, cabeçalhos, regras) e envia; o servidor divide em chunks, grava o progresso por fingerprint e retoma só o prefixo contíguo.
  - **Justificativa:** `docs/api-reference.md` §3.4 e `consolidation-llm.md` §11.
  - **Alternativa Rejeitada:** servidor lendo o repositório do cliente.

- **Decisão (K-06): contratos de teste.** Provedor falso programável; golden da projeção; teste multi-sessão da fila; bootstrap com falha no meio e retomada; checkpoint com recusa de admissão.

## 4. Auditoria de Constituição
- [ ] Build cruzado, `go vet`, `go test -race`.
- [ ] Teste multi-sessão na reivindicação da fila (K-04).
- [ ] INV-02 e INV-03 — fila e lotes pelo escritor, em transação; lote de páginas atômico.
- [ ] INV-06 — preferências e motivos sanitizados antes do prompt e do log.
- [ ] INV-07 e INV-13.
- [ ] Mutação da wiki só pelo caminho de escrita, com commit fora do `WritePage` (002/T080).
- [ ] Goroutines com `context`; trabalhador respeita a janela de graça.
