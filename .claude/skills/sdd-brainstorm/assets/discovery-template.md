# Discovery Report: {{FEATURE_NAME}}

## Metadados

| Campo | Valor |
|-------|-------|
| **ID** | {{FEATURE_ID}} |
| **Slug** | {{SLUG}} |
| **Status** | proposed |
| **Autor** | {{AUTHOR}} |
| **Data** | {{DATE}} |
| **Versão** | 1.0 |
| **Discovery Report** | `reports/{{FEATURE_ID}}-{{SLUG}}-discovery.md` |
| **Aprovado** | false |

---

## 1. Contexto e Problema

> Qual dor ou oportunidade motivou esta feature? Quem é impactado? O que muda se não fizermos?

{{CONTEXT_NARRATIVE}}

### Usuários Impactados

- **Primários:** {{PRIMARY_USERS}}
- **Secundários (indiretos):** {{SECONDARY_USERS}}

### Situação Atual (sem a feature)

{{AS_IS_STATE}}

---

## 2. Objetivos e Não-Objetivos

### Objetivos — o que esta feature FAZ

- {{GOAL_1}}
- {{GOAL_2}}
- {{GOAL_3}}

### Não-Objetivos — o que esta feature explicitamente NÃO FAZ

- {{NON_GOAL_1}}
- {{NON_GOAL_2}}

> Regra: se não está na lista de objetivos, está fora. Nenhuma suposição silenciosa.

---

## 3. Requisitos

### 3.1 Funcionais

| ID | Requisito | Prioridade |
|----|-----------|-----------|
| RF-01 | {{REQ_F_1}} | Must |
| RF-02 | {{REQ_F_2}} | Must |
| RF-03 | {{REQ_F_3}} | Should |
| RF-04 | {{REQ_F_4}} | Could |

> **Prioridades:** Must = obrigatório para o MVP; Should = importante mas não bloqueante; Could = nice-to-have.

### 3.2 Não-Funcionais

| Categoria | Requisito | Métrica de Aceite |
|-----------|-----------|------------------|
| Performance | {{PERF_REQ}} | {{PERF_METRIC}} |
| Segurança | {{SEC_REQ}} | {{SEC_METRIC}} |
| Observabilidade | {{OBS_REQ}} | {{OBS_METRIC}} |
| Compatibilidade | {{COMPAT_REQ}} | {{COMPAT_METRIC}} |

---

## 4. Cenários Gherkin (BDD)

> Estes cenários são o contrato de desenvolvimento. Nenhuma task de implementação deve começar
> sem que o cenário correspondente esteja aqui definido.

```gherkin
Feature: {{FEATURE_GHERKIN_TITLE}}
  Como {{ROLE}}
  Quero {{ACTION}}
  Para {{VALUE}}

  Background:
    Dado que {{BACKGROUND_PRECONDITION}}
    E {{BACKGROUND_PRECONDITION_2}}

  # ============================================================
  # CENÁRIOS DE SUCESSO (mínimo 3)
  # ============================================================

  Scenario: {{HAPPY_SCENARIO_1_TITLE}}
    Dado {{GIVEN_S1}}
    Quando {{WHEN_S1}}
    Então {{THEN_S1}}
    E {{AND_S1}}

  Scenario: {{HAPPY_SCENARIO_2_TITLE}}
    Dado {{GIVEN_S2}}
    Quando {{WHEN_S2}}
    Então {{THEN_S2}}

  Scenario: {{HAPPY_SCENARIO_3_TITLE}}
    Dado {{GIVEN_S3}}
    Quando {{WHEN_S3}}
    Então {{THEN_S3}}

  # ============================================================
  # CENÁRIOS DE FALHA (mínimo 2)
  # ============================================================

  Scenario: {{FAILURE_SCENARIO_1_TITLE}}
    Dado {{GIVEN_F1}}
    Quando {{WHEN_F1}}
    Então o sistema retorna erro "{{ERROR_MESSAGE_F1}}"
    E o estado do sistema permanece inalterado

  Scenario: {{FAILURE_SCENARIO_2_TITLE}}
    Dado {{GIVEN_F2}}
    Quando {{WHEN_F2}}
    Então {{THEN_F2}}
    E {{AND_F2}}

  # ============================================================
  # EDGE CASES (mínimo 1)
  # ============================================================

  Scenario: {{EDGE_CASE_TITLE}}
    Dado {{GIVEN_EDGE}}
    Quando {{WHEN_EDGE}}
    Então {{THEN_EDGE}}
```

### Cobertura

| Tipo | Quantidade | Status |
|------|-----------|--------|
| Cenários de sucesso | {{COUNT_SUCCESS}} | [ ] ≥ 3 |
| Cenários de falha | {{COUNT_FAILURE}} | [ ] ≥ 2 |
| Edge cases | {{COUNT_EDGE}} | [ ] ≥ 1 |

---

## 5. Decisão Arquitetural (ADR)

### Status

`proposed` — aguardando aprovação

### Drivers de Decisão

- {{DECISION_DRIVER_1}}
- {{DECISION_DRIVER_2}}
- {{DECISION_DRIVER_3}}

### Opções Consideradas

| Opção | Descrição Resumida | Prós | Contras |
|-------|--------------------|------|---------|
| A: {{OPTION_A_NAME}} | {{OPTION_A_DESC}} | {{OPTION_A_PROS}} | {{OPTION_A_CONS}} |
| B: {{OPTION_B_NAME}} | {{OPTION_B_DESC}} | {{OPTION_B_PROS}} | {{OPTION_B_CONS}} |

### Decisão

**Opção escolhida:** {{CHOSEN_OPTION}}

**Justificativa:** {{JUSTIFICATION}}

### Consequências

**Positivas:**
- {{POSITIVE_CONSEQUENCE_1}}
- {{POSITIVE_CONSEQUENCE_2}}

**Negativas / Riscos:**
- {{NEGATIVE_CONSEQUENCE_1}}
- {{NEGATIVE_CONSEQUENCE_2}}

---

## 6. Débitos Técnicos Antecipados

> Liste explicitamente o que sabemos que será imperfeito neste ciclo. Débito não documentado
> é débito invisível — o pior tipo.

| Débito | Impacto | Plano de Mitigação |
|--------|---------|-------------------|
| {{DEBT_1}} | Alto / Médio / Baixo | {{DEBT_1_MITIGATION}} |
| {{DEBT_2}} | Alto / Médio / Baixo | {{DEBT_2_MITIGATION}} |

> Se genuinamente não houver débitos antecipados, escreva: "Nenhum identificado —
> justificativa: {{JUSTIFICATION}}". Nunca deixe esta seção vazia.

---

## 7. Cross-Reference (Wiki + Codebase)

### Padrões Encontrados na Wiki

| Fonte | Relevância | Implicação para esta feature |
|-------|-----------|------------------------------|
| {{WIKI_SOURCE_1}} | Alta / Média / Baixa | {{WIKI_IMPLICATION_1}} |
| {{WIKI_SOURCE_2}} | Alta / Média / Baixa | {{WIKI_IMPLICATION_2}} |

### Código Existente Relacionado

| Arquivo | Padrão Identificado | Reutilizável? | Ação |
|---------|-------------------|--------------|------|
| {{FILE_1}} | {{PATTERN_1}} | Sim / Não / Parcial | {{ACTION_1}} |
| {{FILE_2}} | {{PATTERN_2}} | Sim / Não / Parcial | {{ACTION_2}} |

### Ambiguidades Resolvidas Durante o Discovery

| Termo/Conceito Ambíguo | Definição Adotada |
|------------------------|------------------|
| {{AMBIGUITY_1}} | {{RESOLUTION_1}} |
| {{AMBIGUITY_2}} | {{RESOLUTION_2}} |

---

## 8. Riscos

| Risco | Probabilidade | Impacto | Mitigação |
|-------|--------------|---------|-----------|
| {{RISK_1}} | Alta / Média / Baixa | Alto / Médio / Baixo | {{RISK_1_MITIGATION}} |
| {{RISK_2}} | Alta / Média / Baixa | Alto / Médio / Baixo | {{RISK_2_MITIGATION}} |

---

## 9. Critérios de Aceitação (Definition of Done)

- [ ] {{ACCEPTANCE_1}}
- [ ] {{ACCEPTANCE_2}}
- [ ] {{ACCEPTANCE_3}}
- [ ] Todos os cenários Gherkin passam (automatizados ou verificados manualmente)
- [ ] `go build ./...` e `go vet ./...` passam (para mudanças em Go)
- [ ] `cd frontend && npm run build` passa (para mudanças no frontend)
- [ ] Nenhum débito técnico não documentado introduzido

---

## Quality Score

| Dimensão | Score | Observações |
|----------|-------|-------------|
| Completeness (0-5) | {{SCORE_COMPLETENESS}} | {{NOTES_COMPLETENESS}} |
| Gherkin coverage (0-5) | {{SCORE_GHERKIN}} | {{NOTES_GHERKIN}} |
| Ambiguity (0-5) | {{SCORE_AMBIGUITY}} | {{NOTES_AMBIGUITY}} |
| Debt surface (0-5) | {{SCORE_DEBT}} | {{NOTES_DEBT}} |
| Wiki alignment (0-5) | {{SCORE_WIKI}} | {{NOTES_WIKI}} |
| **Total** | **{{SCORE_TOTAL}}/25** | Mínimo: 20/25 |
