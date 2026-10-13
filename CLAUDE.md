## SDD Workflow

Este projeto segue **Spec-Driven Development (SDD)**. Toda feature segue o fluxo:

1. `specs/features/<NNN>-<slug>/spec.md` — Especificação funcional (o QUE, não o COMO)
2. `specs/features/<NNN>-<slug>/plan.md` — Plano arquitetural (ADRs, contratos, auditoria)
3. `specs/features/<NNN>-<slug>/tasks.md` — DAG atômico de tarefas
4. Implementação — código derivado dos artefatos acima

### Regras

- Leia `.github/memory/constitution.md` antes de qualquer alteração de código
- Consulte `.github/memory/tech.md` antes de adicionar dependências
- Valide a estrutura com `/sdd-validate` periodicamente
- `docs/` é a referência do produto e tem prioridade sobre os artefatos SDD e o código; nunca modifique um doc para se ajustar a uma task (ajustes nos docs só a pedido do usuário)