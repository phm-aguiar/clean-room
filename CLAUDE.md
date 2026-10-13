## Fluxo SDD

Este projeto segue **Spec-Driven Development (SDD)**. Toda feature segue o fluxo:

1. `specs/features/<NNN>-<slug>/spec.md` — Especificação funcional (o QUE, não o COMO)
2. `specs/features/<NNN>-<slug>/plan.md` — Plano arquitetural (ADRs, contratos, auditoria)
3. `specs/features/<NNN>-<slug>/tasks.md` — DAG atômico de tarefas
4. Implementação — código derivado dos artefatos acima

### Regras

- Leia `.github/memory/constitution.md` antes de qualquer alteração de código
- Consulte `.github/memory/tech.md` antes de adicionar dependências
- Valide a estrutura com `/sdd-validate` periodicamente
- Nunca edite as documentacoes referencia em C:\Users\henri\OneDrive\Documentos\projetos\clean-room\docs