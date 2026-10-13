# Métricas de Qualidade de Conteúdo

Referência para o `sdd-validate`. Absorvido de `documentation-metrics.md` + `qa-docs-coverage` (AI-Agents-public).

## Freshness Scoring

Combine time-based e behaviour-based signals.

### Time-Based Tiers

| Tier | Idade | Ação |
|---|---|---|
| Fresh | < 90 dias | Nenhuma ação |
| Aging | 90-180 dias | Revisar no próximo ciclo |
| Stale | 180-365 dias | Flag para rewrite ou archive |
| Dead | > 365 dias | Arquivar ou deletar |

**Script de verificação:**
```bash
for f in specs/features/*/spec.md specs/features/*/plan.md specs/features/*/tasks.md; do
  [ -f "$f" ] || continue
  days=$(( ($(date +%s) - $(git log -1 --format=%ct -- "$f" 2>/dev/null || date +%s)) / 86400 ))
  echo "$days dias: $f"
done | sort -rn
```

### Behaviour-Based Signals (mais fortes que timestamps)

- Doc referencia versão de dependência 2+ majors atrás
- Caminho de código documentado foi deletado ou renomeado
- Arquivo fonte relacionado mudou mas doc não
- Spec menciona stack que não está mais em `tech.md`

## Métricas de Cobertura

### Thresholds Green/Yellow/Red

| Métrica | Green | Yellow | Red |
|---|---|---|---|
| Features com spec.md | 100% | 80-99% | < 80% |
| Specs com plan.md | 100% | 70-99% | < 70% |
| Plans com tasks.md | 100% | 70-99% | < 70% |
| Itens de constitution.md auditados | 100% | — | < 100% |
| Páginas stale (>180d) | < 10% | 10-25% | > 25% |

**Quando agir:**
- Yellow → colocar no próximo sprint
- Red → ação imediata (bloquear merge de features novas até resolver)

## Anti-padrões de Métricas

- **Métricas de vaidade:** Contar total de specs em vez de cobertura. Mais specs não é melhor
- **Auditoria só manual:** Se métrica precisa de humano pra computar, não vai ser computada. Automatize ou pule
- **Medir sem agir:** Dashboard que ninguém olha é desperdício. Atribua donos a cada threshold
- **Precisão teatral:** Reportar cobertura com 2 casas decimais quando o denominador é estimativa
- **Ignorar behavior signals:** Um spec editado ontem pode estar errado. Combine timestamp com code-change correlation

## Check de Qualidade por Artefato SDD

### spec.md
- [ ] Propósito claro (o QUE, não o COMO)
- [ ] Cenários BDD com Given/When/Then
- [ ] Restrições documentadas
- [ ] `updated` nos últimos 90 dias
- [ ] Consistente com `tech.md` (stack mencionada existe?)

### plan.md
- [ ] 4 seções canônicas presentes
- [ ] ADR com justificativa + alternativa rejeitada
- [ ] Auditoria de constituição completa
- [ ] Stack usada coincide com `tech.md`

### tasks.md
- [ ] Tarefas atômicas e ordenadas
- [ ] Cada task verificável (tem critério de done)
- [ ] Consistente com `plan.md` (tasks refletem ADRs?)

## Script de Automação

O script `scripts/check-sdd.sh` cobre validação estrutural. Para qualidade de conteúdo, usar:

```bash
# Freshness de specs
find specs/features -name "spec.md" -mtime +90 -exec echo "STALE: {}" \;

# Planos sem ADR
grep -L "Decisão:" specs/features/*/plan.md

# Tasks sem verificação de done
grep -L "\[x\]" specs/features/*/tasks.md
```
