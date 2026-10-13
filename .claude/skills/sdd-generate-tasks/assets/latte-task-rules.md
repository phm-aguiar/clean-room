---
graph-operators: enabled
heartbeat-threshold: 4
max-rounds: 40
---

# Extensão LATTE — Coordination Graph para tasks.md

Referência para `sdd-generate_tasks` v3.0.0 com suporte a LATTE (coordination graphs dinâmicos).
Estende o schema base definido em `task-rules.md` (v2.0.0) com operadores de grafo, heartbeat e
atribuição explícita de agentes.

## Visão geral

Quando `graph-operators: enabled`, o `tasks.md` ganha capacidades de coordination graph dinâmico
(G₀ → G₁ → ... → Gₙ). O LATTE gerencia múltiplos workers (Lead, executor1, executor2, researcher1, ...) que
executam tarefas concorrentemente, com heartbeats, round-robin e reatribuição de tarefas em caso
de falha.

### Campos YAML Frontmatter (novos)

| Campo | Obrigatório | Valores | Padrão | Descrição |
|---|---|---|---|---|
| `graph-operators` | Sim (quando LATTE) | `enabled` \| `disabled` | `disabled` | Habilita/desabilita os operadores de grafo LATTE |
| `heartbeat-threshold` | Sim (quando `enabled`) | inteiro positivo | `4` | Número de rounds sem heartbeat antes de marcar worker como `lost` |
| `max-rounds` | Sim (quando `enabled`) | inteiro positivo | `40` | Número máximo de rounds de coordenação antes de forçar timeout |

### Contrato

Quando `graph-operators: disabled` (ou ausente), o comportamento é idêntico ao `task-rules.md`
v2.0.0 — DAG estático, sem coordenação dinâmica. Quando `enabled`, as regras abaixo se aplicam
**em adição** às regras base.

---

## Sintaxe G₀ no corpo do tasks.md

Cada task ganha dois campos opcionais que alimentam o coordination graph inicial (G₀):

### Formato estendido da task

```markdown
- [ ] **Tnnn:** Descrição da tarefa
  - **Papel:** researcher | analyst | executor
  - **agent:** Lead | executor1 | executor2 | researcher1 | analyst1
  - **Dependências:** Txxx, Tyyy | Nenhuma
  - **depends_on:** [Txxx, Tyyy]
  - **Paralelizável:** true | false
  - **Arquivos:** `path/to/file.go`, `path/to/other.go`
  - **Wiki-Keywords:** keyword1, keyword2, keyword3
```

### Campos estendidos

| Campo | Obrigatoriedade | Valores | Descrição |
|---|---|---|---|
| `agent` | Opcional | `Lead`, `executor1`, `executor2`, ..., `executorN`, `researcher1`, `analyst1` | Worker designado para executar esta task no round inicial. Se omitido, o LATTE atribui dinamicamente no primeiro round. |
| `depends_on` | Opcional | Lista YAML de IDs: `[T001, T002]` | Dependências explícitas no formato de lista. Redundante com `Dependências` no modo LATTE — **prefira `depends_on`** quando `graph-operators: enabled`. O campo `Dependências` (string legada) é mantido para compatibilidade com v2.0.0, mas `depends_on` tem precedência na resolução do grafo. |

### Regras de transição Dependências ↔ depends_on

1. Se ambos `Dependências` e `depends_on` estão presentes, `depends_on` vence.
2. Se apenas `Dependências` (formato string legada), o parser converte: `"T001, T002"` → `[T001, T002]` e `"Nenhuma"` → `[]`.
3. Se apenas `depends_on`, `Dependências` é ignorado na geração do grafo.
4. Se nenhum dos dois, a task é tratada como sem dependências (`depends_on: []`).

### Nomenclatura de agentes

| Agente | Papel típico | Descrição |
|---|---|---|
| `Lead` | sessão principal | Coordenador. Gerencia o grafo, reatribui tasks, monitora heartbeats. Sempre presente. |
| `executor1`, `executor2`, ..., `executorN` | executor | Workers de implementação genérica (código Go, React, testes, docs). |
| `researcher1` | researcher | Worker de descoberta read-only. Usado em Fase 0 de features complexas. |
| `analyst1` | analyst | Worker de síntese. Audita constitution.md, produz plano atômico. |

O número de workers (`N`) é definido pelo `plan.md` ou pelo operador humano. O `Lead` é fixo;
workers podem ser adicionados/removidos dinamicamente via operadores de grafo.

---

## Seção `## Coordination Graph` (opcional)

Quando presente, esta seção define o G₀ inicial do coordination graph. Se omitida, o G₀ é
derivado automaticamente do DAG de tasks (cada task vira um nó, `depends_on` define as arestas).

### Formato

```markdown
## Coordination Graph

G₀ (round 0):

- **nodes:** T001, T002, T003, T004, T005, T006
- **edges:**
  - T001 → T002
  - T001 → T003
  - T002 → T006
  - T003 → T006
  - T004 → T006
  - T005 → T006
- **assignments:**
  - T001: Lead
  - T002: Dev1
  - T003: Dev1
  - T004: QA1
  - T005: QA1
  - T006: Lead
- **ready:** [T001, T004]  # tasks sem dependências pendentes
```

### Campos do Coordination Graph

| Campo | Descrição |
|---|---|
| `nodes` | Lista de todos os IDs de task no grafo |
| `edges` | Arestas direcionadas: `origem → destino` (destino depende de origem) |
| `assignments` | Mapeamento inicial `task: agent`. Pode ser parcial — tasks não atribuídas recebem worker no primeiro round. |
| `ready` | Lista de tasks prontas para execução imediata (sem dependências não satisfeitas) |

### Regras

1. `nodes` deve conter exatamente os IDs de todas as tasks declaradas no arquivo.
2. `edges` deve ser consistente com `depends_on` de cada task.
3. `assignments` pode ser subconjunto de `nodes`; o LATTE completa no round 0.
4. Se a seção `## Coordination Graph` estiver ausente, o G₀ é inferido (ver seção abaixo).

### Inferência automática de G₀

Quando a seção é omitida, o parser gera G₀ assim:

```
nodes: todos os Tnnn do arquivo
edges: T_a → T_b para cada b em depends_on(T_a)  (aresta invertida: origem depende de destino concluído)
assignments: tasks com `agent` explícito são atribuídas; as demais recebem `unassigned`
ready: tasks com depends_on vazio (ou "Nenhuma")
```

---

## Operadores de Grafo (visão geral)

Quando `graph-operators: enabled`, o LATTE aplica estes operadores a cada round:

| Operador | Gatilho | Efeito |
|---|---|---|
| `assign` | Round 0 ou task `unassigned` | Atribui worker livre à task pronta |
| `reassign` | Worker marcado `lost` (H heartbeats sem resposta) | Redistribui tasks do worker perdido para workers vivos |
| `promote` | Task concluída com sucesso | Marca `[x]`, desbloqueia dependentes |
| `block` | Task falhou (erro não recuperável) | Bloqueia task e todas as suas dependentes transitivas |
| `retry` | Task falhou (erro recuperável) | Reenfileira para próximo round (máx. 3 retries) |
| `timeout` | `max-rounds` atingido sem conclusão | Força `[!]` nas tasks pendentes, encerra grafo |

---

## Exemplo completo: tasks.md com LATTE enabled

```markdown
---
graph-operators: enabled
heartbeat-threshold: 4
max-rounds: 40
---

# tasks.md: Feature Chat Messages (LATTE)

## Fase 1: Fundação

- [ ] **T001:** Criar modelo Message
  - **Papel:** executor
  - **agent:** executor1
  - **depends_on:** []
  - **Paralelizável:** true
  - **Arquivos:** `internal/model/message.go`

- [ ] **T002:** Criar schema do banco (migration)
  - **Papel:** executor
  - **agent:** executor1
  - **depends_on:** [T001]
  - **Paralelizável:** false
  - **Arquivos:** `internal/db/migrations/001_create_messages.sql`

## Fase 2: Implementação

- [ ] **T003:** Criar handler HTTP POST /messages
  - **Papel:** executor
  - **agent:** executor1
  - **depends_on:** [T001, T002]
  - **Paralelizável:** true
  - **Arquivos:** `internal/handler/message_handler.go`

- [ ] **T004:** Criar cenários de aceitação (Gherkin)
  - **Papel:** executor
  - **agent:** executor2
  - **depends_on:** []
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/004-chat/acceptance/chat.feature`

- [ ] **T005:** Criar testes unitários do modelo Message
  - **Papel:** executor
  - **agent:** executor2
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/model/message_test.go`

## Fase 3: Finalização

- [ ] **T006:** Smoke test fim a fim
  - **Papel:** executor
  - **agent:** Lead
  - **depends_on:** [T003, T004, T005]
  - **Paralelizável:** false
  - **Arquivos:** `test/smoke_test.go`

## Coordination Graph

G₀ (round 0):

- **nodes:** T001, T002, T003, T004, T005, T006
- **edges:**
  - T001 → T002
  - T001 → T003
  - T002 → T003
  - T001 → T005
  - T003 → T006
  - T004 → T006
  - T005 → T006
- **assignments:**
  - T001: Lead
  - T002: Dev1
  - T003: Dev1
  - T004: QA1
  - T005: QA1
  - T006: Lead
- **ready:** [T001, T004]
```

### Comportamento esperado (round a round)

```
Round 0: G₀ carregado. ready = [T001, T004].
  → Lead executa T001. QA1 executa T004.
  → Heartbeats recebidos: Lead=1, QA1=1, Dev1=0 (idle).

Round 1: T001 concluído [x]. T004 concluído [x].
  → ready agora = [T002, T005]  (T003 ainda depende de T002).
  → Dev1 executa T002. QA1 executa T005.
  → Heartbeats: Lead=2, Dev1=1, QA1=2.

Round 2: T002 concluído [x]. T005 concluído [x].
  → ready agora = [T003]  (T006 ainda depende de T003, T004, T005).
  → Dev1 executa T003.
  → Heartbeats: Lead=3, Dev1=2, QA1=3.

Round 3: T003 concluído [x].
  → ready agora = [T006].
  → Lead executa T006.
  → Heartbeats: Lead=4, Dev1=3, QA1=4.

Round 4: T006 concluído [x].
  → Grafo esgotado. Todas as tasks [x].
  → LATTE encerra com sucesso.
  → Total: 5 rounds (0–4).
```

---

## Exemplo de sumário para AskUserQuestion com LATTE

```
tasks.md: 6 tarefas em 3 fases (LATTE enabled, H=4, max-rounds=40)

G₀: 6 nodes, 7 edges, 3 workers (Lead, executor1, executor2)
  ready: T001 (executor1), T004 (executor2)
  esperando: T002, T003, T005, T006

Fase 1: Fundação (2 tasks)
  T001 Criar modelo Message (executor1, paralelo)
  T002 Criar migration (executor1, depende T001)

Fase 2: Implementação (3 tasks)
  T003 Criar handler POST /messages (executor1, depende T001,T002)
  T004 Criar cenários Gherkin (executor2, paralelo)
  T005 Criar testes unitários Message (executor2, depende T001)

Fase 3: Finalização (1 task)
  T006 Smoke test (Lead, depende T003,T004,T005)

Caminho crítico: T001 → T002 → T003 → T006 (4 rounds)
```

---

## Compatibilidade com task-rules.md v2.0.0

| Aspecto | v2.0.0 (DAG estático) | v3.0.0 (LATTE) |
|---|---|---|
| `graph-operators` | Ausente (implícito `disabled`) | `enabled` ou `disabled` |
| `heartbeat-threshold` | Ausente | Presente quando `enabled` |
| `max-rounds` | Ausente | Presente quando `enabled` |
| Campo `agent` | Ausente | Opcional por task |
| Campo `depends_on` | Ausente | Opcional por task (formato lista) |
| Campo `Dependências` | String: `"T001, T002"` ou `"Nenhuma"` | Mantido para compatibilidade; `depends_on` tem precedência |
| Seção `## Coordination Graph` | Ausente | Opcional |
| Execução | Sequencial por fase, paralelo intra-fase | Round-robin dinâmico com heartbeats |
| Tolerância a falhas | Nenhuma (DAG quebra) | Reassign, retry, timeout |
