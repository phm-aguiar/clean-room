# Estrutura Canônica dos Artefatos SDD

> Extraído de `specs/features/002-sdd-templates/spec.md`. Atualizado com regras de atomicidade.
> Este documento define o formato alvo para cada artefato.
> O refatorador usa estas estruturas como contrato.

## spec.md — Especificação Funcional

Foco no "O quê" e no "Por quê". Agnóstico de tecnologia. Contrato de comportamento.

```markdown
# spec.md: {{TÍTULO}}

## 1. Visão Geral e Intenção de Negócios
- **Funcionalidade:** {{descrição curta}}
- **Objetivo:** {{por que isso existe, valor de negócio}}

## 2. Histórias de Usuário e Comportamento (BDD)
- **Como** {{ator}}, **Quero** {{ação}}, **Para que** {{objetivo}}.
- **Cenário Principal:**
  - *Dado* que {{precondição}},
  - *Quando* {{evento}},
  - *Então* {{resultado}}.

## 3. Limites, Restrições e Não-Funcionais
- **Segurança:** {{regras de segurança}}
- **Performance:** {{SLAs, latência, throughput}}

## 4. Checklist de Ambiguidade para a IA
- [ ] {{pergunta para esclarecer — se vazio, feature está ambígua}}
```

## plan.md — Plano Arquitetural

Tradução técnica da spec. ADR vivo.

```markdown
# plan.md: Plano de Implementação Técnica

## 1. Metadados do Plano
- **Stack Tecnológico:** {{linguagens, frameworks, versões}}
- **Feature Fonte:** `specs/features/{{id}}-{{nome}}/spec.md`

## 2. Design de Contratos e Fronteiras (Schema-Driven)
- **Contrato:** {{modificações em contratos formais, ex: asyncapi.yaml, openapi.yaml}}
- **Geração de Código:** {{plugins/ferramentas de codegen, se aplicável}}

## 3. Decisões Arquiteturais e Justificativas
- **Decisão:** {{o que foi decidido}}
- **Alternativa Rejeitada:** {{o que foi descartado e por quê}}

## 4. Auditoria de Constituição
- [ ] {{checklist de conformidade com constitution.md}}
```

## tasks.md — Matriz de Execução

Passos atômicos, testáveis e ordenados. **Tarefas na mesma fase são paralelizáveis.**

### Regras de atomicidade

1. **Uma ação por tarefa**: "Criar X e testar Y" são duas tarefas separadas.
2. **Mesma fase = paralelizável**: se B depende de A, B vai na fase seguinte ou declara `(Depende de Tnnn)`.
3. **Formato fixo**: `- [ ] **Tnnn:** descrição (Depende de Tnnn)`.
4. **Numeração global**: T001, T002... não reinicia por fase.
5. **Fases lógicas**: Fundação → Implementação → Validação → Documentação.
6. **Tarefas concluídas**: marcar `[x]` se já implementado.

```markdown
# tasks.md: Lista de Execução

## Fase 1: {{nome da fase}} (Paralelizável)
- [ ] **T{{nnn}}:** {{descrição da tarefa}}
- [ ] **T{{nnn}}:** {{descrição da tarefa}}

## Fase 2: {{nome da fase}} (Paralelizável)
- [ ] **T{{nnn}}:** {{descrição da tarefa}} (Depende de T{{nnn}})
- [ ] **T{{nnn}}:** {{descrição da tarefa}} (Depende de T{{nnn}})

## Fase 3: {{nome da fase}} (Paralelizável)
- [ ] **T{{nnn}}:** {{descrição da tarefa}}
```

## CLAUDE.md — Diretrizes para Agentes IA

Sistema de regras primárias. Obediência cega.

A seção SDD Workflow deve conter:

```markdown
## SDD Workflow

Este repositório utiliza Spec-Driven Development (SDD).
O código atua como subproduto das definições em `/specs`.

### Comportamento Fundamental e Restrições
- NUNCA inicie implementação sem `spec.md`, `plan.md` e `tasks.md` consolidados.
- NUNCA preencha informações faltantes; pare e exija que o humano resolva marcadores `{{...}}`.
- Siga rigorosamente a ordem do `tasks.md`. Marque tarefas concluídas com `[x]`.

### Tecnologias e Padrões Obrigatórios
- Consulte `.github/memory/tech.md` para stack homologado.
- Consulte `.github/memory/constitution.md` para portões de qualidade.
```

## llms.txt — Navegação para LLMs

Especificação emergente. Guia de fora para dentro.

```markdown
# {{NOME_DO_PROJETO}}

> {{descrição de uma frase}}

## Navegação do Repositório e Especificações
- `/specs/features/`: Fonte da Verdade (SSOT). Features modeladas aqui antes do código.
- `/specs/domain-events/`: Contratos formais (AsyncAPI, OpenAPI).
- `/specs/infra/`: Requisitos de infraestrutura genéricos.
- `.github/memory/constitution.md`: Princípios invioláveis.
- `.github/memory/tech.md`: Stack homologado.

## Camadas de Código (Derivadas)
- {{estrutura de diretórios do projeto e propósito de cada camada}}

## Links Adicionais e Regras
- [Constituição do Código](.github/memory/constitution.md)
- [Stack Tecnológica](.github/memory/tech.md)
```
