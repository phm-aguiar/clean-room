## SDD Workflow

Este projeto segue **Spec-Driven Development (SDD)**. Toda feature segue o fluxo:

1. `specs/features/<id>-<nome>/spec.md` — Especificação funcional (o QUE, não o COMO)
2. `specs/features/<id>-<nome>/plan.md` — Plano arquitetural (decisões técnicas, ADR)
3. `specs/features/<id>-<nome>/tasks.md` — Tarefas atômicas ordenadas
4. Implementação — Código derivado dos artefatos acima

### Regras
- **Nunca implemente sem spec.md e plan.md aprovados** pelo usuário.
- Leia `constitution.md` antes de qualquer alteração de código.
- Consulte `tech.md` antes de adicionar dependências.
- Valide a estrutura com `sdd-validate` periodicamente.
- Pergunte ao usuário ANTES de modificar `constitution.md`.

### Skills SDD disponíveis
- Inicializar estrutura: `sdd-init_repo`
- Mapear stack: `sdd-explore_tech`
- Validar conformidade: `sdd-validate`
- Refatorar artefatos: `sdd-refactor_artifact`
- Gerar plano (plan.md): `sdd-generate_plan`
- Gerar tarefas (tasks.md): `sdd-generate_tasks`
