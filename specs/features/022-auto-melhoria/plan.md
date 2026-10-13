# plan.md: Plano de Implementação Técnica — Auto-melhoria (022)

> Plano gerado em 2026-10-13, antes de a 002 ser executada. Revisar quando a 021 terminar.

## 1. Metadados do Plano
- **Stack Tecnológico:** a da 016, sem dependência nova (`os/exec` para o portão, `crypto/sha256`).
- **Feature Fonte:** `specs/features/022-auto-melhoria/spec.md`
- **Escopo:** revisão, validação, patch, portão, registro, aprovação, buffer, agendador, experiência, superfícies, telemetria.

## 2. Design de Contratos e Fronteiras
- **`internal/domain/autoimprove`:** normalização, validação com motivos, materialização de patch, hash de seção, montagem do contexto (orçamentos).
- **`internal/wiki`:** `ReviewSession`, `ExperiencePass`, `ApplyProposal` (aprovação pelo caminho de escrita, conflitos).
- **`internal/storage`:** execuções, propostas, eventos, rejeições, estado do agendador e reivindicações (tabelas da V01).
- **`internal/cli`:** portão de avaliação, agendador, comandos, rotas; **`internal/mcp`:** `memory_auto_improve`; **navegador da 014:** `/pending`.
- **Configuração:** `[auto_improve]`, `[auto_improve.scheduler]`, `[auto_improve.eval]`.

## 3. Decisões Arquiteturais e Justificativas

- **Decisão (A-01): validação por código antes de qualquer escrita.** Toda proposta passa pela validação pura e, se alvo, pelo portão; só então é registrada; aprovação revalida o snapshot.
  - **Justificativa:** `consolidation-llm.md` § Notas, item 2; `auto-improvement-loop.md` § Origem do desenho.
  - **Alternativa Rejeitada:** confiar no prompt.

- **Decisão (A-02): registro antes de aplicar, aplicação pelo caminho da wiki.** O registro da execução e das propostas acontece sempre; a aprovação automática chama a mesma aplicação da aprovação manual, com admissão de escrita e ator próprio.
  - **Justificativa:** invariantes 1, 8 e 12.
  - **Alternativa Rejeitada:** escrever a página direto no agendador.

- **Decisão (A-03): portão como processo sem shell com limites rígidos.** `exec.CommandContext` com argumentos separados por espaço, stdin JSON, stdout limitado a 64 KiB, stderr descartado, tempo único.
  - **Justificativa:** `auto-improve-eval-gates.md` § Execução.
  - **Alternativa Rejeitada:** `sh -c`.

- **Decisão (A-04): reivindicação transacional e estacionamento.** Reivindicar antes do LLM numa transação do escritor; falha libera e conta; 3 falhas estacionam.
  - **Justificativa:** `auto-improvement-loop.md` § Agendador; `docs/storage-and-retrieval.md` §16.
  - **Alternativa Rejeitada:** trava em memória.

- **Decisão (A-05): contratos de teste.** Provedor falso com respostas programadas para cada motivo de rejeição; scorers de teste (Go compilado no teste) para os 7 casos do portão; teste multi-sessão da reivindicação; relógio injetado para marca d'água, idade mínima e âncora da experiência.

## 4. Auditoria de Constituição
- [ ] Build cruzado, `go vet`, `go test -race`.
- [ ] Teste multi-sessão na reivindicação (A-04).
- [ ] INV-02 e INV-03 — registro, eventos e aprovação pelo escritor.
- [ ] INV-06 — arquivo de revisão e motivos sanitizados.
- [ ] INV-07 e INV-13 — sem provedor, nada roda e a revisão manual explica.
- [ ] Nada no caminho de hooks; escrita só pela wiki; páginas fixadas e slots invariantes protegidos.
- [ ] Autorização por capacidade nas rotas e na página `/pending` (só raiz).
