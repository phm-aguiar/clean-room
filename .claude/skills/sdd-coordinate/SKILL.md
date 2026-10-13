---
name: sdd-coordinate
description: Executa o DAG de um tasks.md aprovado com protocolo LATTE (Lead coordena workers researcher, analyst e executor por rounds, com frontier, DONE verificado, heartbeat e retry)
---

# Coordenação de DAG com LATTE

## Entrada

Argumento: caminho da pasta da feature, absoluto ou relativo ao cwd (ex.: `specs/features/001-doc-refactor-plan`). Se o caminho não existir, pare e liste as pastas em `specs/features/` para o usuário escolher. Não invente um caminho.

## Approval gate

Leia `spec.md`. Procure a linha `Approved: true` (com ou sem negrito). Se não achar `true`, aborte e informe. Não execute nada.

## Parse do tasks.md

Reconheça dois formatos de task. Use o primeiro que aparecer.

Formato A (usado nas features do projeto):

```
- [ ] **T001:** título
  - **Papel:** researcher
  - **Dependências:** Nenhuma | T001, T003
  - **Paralelizável:** true | false
  - **Arquivos:** `caminho/a.md`, `caminho/b/**`
```

Formato B (legado):

```
- **T001** | researcher | título
  - Deps: — | T001, T003
  - Paralelizável: true
  - Arquivos: caminho/**
```

Para cada task extraia: ID, Papel, Dependências (`Nenhuma`, `—` ou vazio = sem deps), Paralelizável, Arquivos (lista de caminhos, sem crases), estado (`[ ]` ou `[x]`) e a fase (último cabeçalho `## Fase N` antes da task).

Valide antes de executar: IDs únicos; toda dependência existe; não há ciclo (DFS); tasks paralelas do mesmo round não compartilham Arquivos. Se falhar, pare e liste o problema.

## Critério DONE

O tasks.md pode declarar um critério em bloco de citação com o rótulo `Critério DONE`. Se existir, ele vale. Se não existir, o padrão é:
- Todo caminho em **Arquivos** existe e foi alterado no sentido da task (confira com leitura ou grep, não só pela resposta do worker).
- Build: só roda se o frontmatter do tasks.md tiver `build-cmd`. Sem essa chave, não há gate de build.

Nunca marque `[x]` com base só no texto do worker. Confira os arquivos.

## Papéis e limites

- `researcher` é somente leitura. Se a task grava arquivos e o papel é `researcher`, peça ao worker que devolva o conteúdo na resposta e salve você mesmo, no caminho de **Arquivos**. Registre isso no relatório.
- `analyst` produz decisões e planos, não implementa.
- `executor` edita arquivos. Ele não faz commit, a não ser que a task diga.

## Rounds

1. **Frontier**: tasks pendentes cujas dependências estão todas `[x]`, na fase atual ou em fase anterior.
2. **Dispatch**: até 3 workers por round, via `Agent(subagent_type: "<papel>")`. Tasks com `Paralelizável: false` rodam sozinhas. Tasks paralelas com Arquivos que se sobrepõem não rodam no mesmo round.
3. **Prompt do worker**: autocontido. Inclua ID, título, Arquivos, Critério DONE, o que já foi concluído que a task usa, e o formato de resposta esperado. Não delegue entendimento: diga o que fazer.
4. **Verificação**: ao receber o resultado, confira os Arquivos e o Critério DONE.
   - Passou: aplique Close, trocando `[ ]` por `[x]` na linha da task (Edit com a linha exata). Atualize a frontier.
   - Falhou: reenvie com o motivo, até 3 tentativas. Depois disso marque a sub-árvore como bloqueada e escale ao usuário.
5. **Relatório curto** por round: tasks fechadas, em andamento, bloqueadas.

## Fases

Execute fase por fase, na ordem do arquivo. Ao terminar a última task de uma fase, confirme que ela está `[x]` e informe o resumo da fase antes de iniciar a próxima. Continue para a próxima fase sem nova confirmação, a menos que:
- uma task tenha falhado em 3 tentativas;
- a próxima fase contenha task marcada como destrutiva ou que apague arquivos (ex.: remoção de READMEs), caso em que pare e peça confirmação.

## Heartbeat e contexto

- Worker sem resposta após 4 rounds: reenvie a task com o contexto atualizado (tentativa N/3).
- A cada 4 rounds, resuma em 3 linhas o que foi concluído antes de continuar, para não carregar saídas longas.

## Guardrails

- Nunca agrupe duas tasks num mesmo worker.
- Nunca rode Fase N+1 com task de Fase N pendente que ela dependa.
- Não faça commit, push nem reset. Nada de git destrutivo.
- Não edite o critério DONE nem os Arquivos de uma task para fazê-la passar.
- Não altere o spec para contornar um bloqueio. Se o spec estiver errado, escale.

## Estados

| Estado | Quando | Ação do Lead |
|---|---|---|
| Pendente | Dependências não fechadas | Espera |
| Pronta | Dependências `[x]` | Entra na frontier |
| Atribuída | Dispatch feito | Aguarda resultado |
| Verificada | Arquivos e critério conferidos | Marca `[x]` |
| Bloqueada | 3 falhas | Bloqueia sub-árvore e escala |

## Próximos passos após o fim

Quando todas as fases estiverem `[x]`, rode a verificação final que o tasks.md descrever (ex.: validador) e informe o resultado.
