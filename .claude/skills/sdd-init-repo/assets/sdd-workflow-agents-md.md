# Seção SDD Workflow para AGENTS.md

Conteúdo a ser inserido ou merged no `AGENTS.md` do repositório durante `sdd-init_repo`.

## Template

```markdown
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
```

## Instruções de merge

1. Leia o `AGENTS.md` atual.
2. Se já existir uma seção `## SDD Workflow`, pule (idempotente).
3. Se não existir, insira este bloco após a seção de linguagem/toolchain ou no final do arquivo.
4. Se o `AGENTS.md` não existir, crie-o com este conteúdo como primeira seção.

## Skills SDD (versão para AGENTS.md)

A lista de skills deve refletir as skills atualmente disponíveis. No momento da escrita:

```markdown
### Skills SDD (claude Agent — `.claude/skills/` no repo)
- Inicializar estrutura: `sdd-init_repo`
- Mapear stack: `sdd-explore_tech`
- Brainstorm → spec: `sdd-brainstorm`
- Validar conformidade: `sdd-validate`
- Refatorar artefatos: `sdd-refactor_artifact`
- Gerar plano (plan.md): `sdd-generate_plan`
- Gerar tarefas (tasks.md): `sdd-generate_tasks`
```
