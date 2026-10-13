# Bloco SDD Workflow para CLAUDE.md

Conteúdo a ser inserido ou merged no `CLAUDE.md` do repositório durante `sdd-init-repo`.

## Template

```markdown
## Fluxo SDD

Este projeto segue **Spec-Driven Development (SDD)**. Toda feature segue o fluxo:

1. `specs/features/<NNN>-<slug>/spec.md` — Especificação funcional (o QUE, não o COMO)
2. `specs/features/<NNN>-<slug>/plan.md` — Plano arquitetural (ADRs, contratos, auditoria)
3. `specs/features/<NNN>-<slug>/tasks.md` — DAG atômico de tarefas
4. Implementação — código derivado dos artefatos acima

### Regras

- **Nunca implemente sem `Aprovado: true` no spec.md**
- Leia `.github/memory/constitution.md` antes de qualquer alteração de código
- Consulte `.github/memory/tech.md` antes de adicionar dependências
- Valide a estrutura com `/sdd-validate` periodicamente

### Onboarding (primeira ação ao entrar no repo)

1. Leia `.github/memory/constitution.md`
2. Leia `llms.txt`
3. Leia `wiki-claude/index.md` (ou o vault configurado)

### Skills SDD

| Slash command | Função |
|---|---|
| `/sdd` | Pipeline completo — dispatcher 8 modos |
| `/sdd-brainstorm` | Entrevista interativa → spec.md |
| `/sdd-generate-plan` | spec.md → plan.md |
| `/sdd-generate-tasks` | plan.md → tasks.md (DAG) |
| `/sdd-validate` | Auditoria PASS/FAIL/WARN |
| `/sdd-explore-tech` | Detecta stack → tech.md |
| `/sdd-refactor-artifact` | Normaliza artefato SDD |
```

## Instruções de merge

1. Leia o `CLAUDE.md` atual.
2. Se já contém seção `## Fluxo SDD`, pule (idempotente).
3. Se não contém, adicione o bloco ao final do arquivo.
4. Se `CLAUDE.md` não existir, crie-o com este conteúdo como ponto de partida.
