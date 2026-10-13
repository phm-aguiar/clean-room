# Quality Gates — Discovery Report Scoring

Referência para `sdd-brainstorm`. Cada dimensão vale 0-5 pontos. **Mínimo para aprovação: 20/25.**

---

## Dimensão 1: Completeness (0-5)

Mede se todas as seções do discovery template estão preenchidas sem lacunas.

| Score | Critério |
|-------|---------|
| 5 | Todas as 9 seções preenchidas, zero `TODO`/`TBD`, zero placeholders `{{...}}` |
| 4 | ≤ 2 campos vazios ou vagos em seções não-críticas (ex: RF-04 opcional, risco de baixo impacto) |
| 3 | Algumas seções incompletas mas as críticas (Gherkin, Débitos, DoD) estão presentes |
| 2 | Seções críticas incompletas — falta Gherkin ou DoD ou Decisão Arquitetural |
| 1 | Apenas esboço — metade ou mais das seções com `{{placeholder}}` |
| 0 | Template quase vazio |

**Seções críticas** (nunca podem ser deixadas incompletas para score ≥ 3):
- Cenários Gherkin (seção 4)
- Débitos Técnicos (seção 6)
- Critérios de Aceitação (seção 9)
- Decisão Arquitetural (seção 5)

---

## Dimensão 2: Gherkin Coverage (0-5)

Mede a qualidade e completude dos cenários BDD.

| Score | Critério |
|-------|---------|
| 5 | ≥ 3 sucesso + ≥ 2 falha + ≥ 1 edge case; todos com Given/When/Then completos |
| 4 | ≥ 3 sucesso + ≥ 2 falha; edge case ausente mas outros detalhados |
| 3 | ≥ 2 sucesso + ≥ 2 falha; cobertura mínima presente |
| 2 | Cenários de sucesso presentes mas falha ausente ou vaga |
| 1 | Apenas 1 cenário de sucesso; sem falha |
| 0 | Nenhum cenário Gherkin |

**Falhas de qualidade que reduzem score:**
- `Então o sistema trata o erro` (sem especificar como) → -1
- Cenário sem `Dado` (Given) → -1
- Cenário de falha sem "E o estado permanece inalterado" (quando aplicável) → -1

---

## Dimensão 3: Ambiguity (0-5)

Mede se termos vagos foram identificados e definidos no discovery.

| Score | Critério |
|-------|---------|
| 5 | Todos os termos do domínio têm definição explícita; nenhum "notificar o usuário" sem especificar como |
| 4 | ≤ 1 termo vago sem definição explícita em seções secundárias |
| 3 | Termos vagos nas seções de requisito mas clarificados nos cenários Gherkin |
| 2 | Múltiplos termos ambíguos que podem gerar interpretações divergentes na implementação |
| 1 | Propósito central ainda vago após entrevista |
| 0 | Discovery ilegível sem contexto adicional |

**Termos que exigem definição:**
- "notificar" → via qual canal? websocket? email? badge?
- "administrador" → qual role em `board_staff`?
- "recentemente" → qual janela de tempo?
- "aprovar" → quem aprova? há workflow?

---

## Dimensão 4: Debt Surface (0-5)

Mede se os débitos técnicos antecipados foram surfaceados honestamente.

| Score | Critério |
|-------|---------|
| 5 | Todos os débitos identificados têm impacto classificado E mitigação concreta |
| 4 | Débitos identificados com impacto mas mitigação genérica ("monitorar", "refatorar depois") |
| 3 | Débitos listados mas sem impacto ou mitigação |
| 2 | Apenas 1 débito identificado — suspeito de sub-report |
| 1 | Seção "Nenhum" sem justificativa documentada |
| 0 | Seção ausente ou vazia |

**Débitos comuns que raramente estão ausentes:**
- Ausência de testes automatizados para algum cenário
- Acoplamento temporário para atingir prazo
- Migração de schema sem rollback garantido
- Dependência de serviço externo sem circuit breaker
- Performance não otimizada (N+1, índice ausente)

---

## Dimensão 5: Wiki Alignment (0-5)

Mede se o discovery aproveitou o conhecimento acumulado na wiki e no código.

| Score | Critério |
|-------|---------|
| 5 | ≥ 3 fontes wiki/código consultadas; implicações documentadas; decisão arquitetural cita padrões existentes |
| 4 | ≥ 2 fontes consultadas; pelo menos 1 implicação documentada |
| 3 | ≥ 1 fonte consultada; evidência de que o codebase foi escaneado |
| 2 | Cross-reference preenchido mas superficial ("não encontrado nada relevante" sem busca documentada) |
| 1 | Seção de cross-reference vazia ou com `{{placeholder}}` |
| 0 | Etapa de cross-reference saltada completamente |

**Sinais de boa wiki alignment:**
- "O padrão X já existe em `internal/forum/store/boards.go` — reutilizando"
- "Wiki chunk `hub-ws-pattern` indica que o hub atual não suporta Y — planejar extensão"
- "Encontrado código similar em `internal/auth/handler.go:45` — extrair helper"

---

## Como calcular o score final

```
Score Total = Completeness + Gherkin + Ambiguity + Debt + Wiki
```

| Faixa | Interpretação | Ação |
|-------|--------------|------|
| 23-25 | Excelente — discovery maduro | Aprovar |
| 20-22 | Bom — threshold mínimo atingido | Aprovar com ressalvas documentadas |
| 17-19 | Insuficiente — gaps específicos | Iterar nas dimensões < 3 |
| 14-16 | Fraco — discovery incompleto | Revisão substancial necessária |
| < 14 | Rejeitar — recomeçar o brainstorm | Abortar e reiniciar |

---

## Como apresentar o score ao usuário

```
Discovery Report: <id>-<slug>
Quality Score: XX/25

✓ Completeness:     X/5 — <nota breve>
✓ Gherkin coverage: X/5 — <nota breve>
! Ambiguity:        X/5 — <o que está vago>
! Debt surface:     X/5 — <o que falta>
✓ Wiki alignment:   X/5 — <nota breve>

[aprovado | não aprovado — iterar em: Dimensão A, Dimensão B]
```
