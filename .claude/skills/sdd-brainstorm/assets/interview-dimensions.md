# Dimensões da Entrevista Interativa

Este documento define as dimensões que a skill `sdd-brainstorm` deve cobrir durante
a entrevista com `AskUserQuestion`. Cada dimensão tem perguntas-base, dicas de follow-up
e critérios de "pronto" para saber quando avançar.

## Regras gerais

- **Ordem:** siga a ordem abaixo, mas seja flexível — se o usuário já respondeu
  uma dimensão antes de você perguntar, marque como coberta e pule.
- **Uma pergunta por `AskUserQuestion`:** nunca empilhe dimensões na mesma chamada.
- **Múltipla escolha sempre que possível:** transforme perguntas abertas em
  opções quando fizer sentido.
- **Critério de pronto:** cada dimensão lista o que constitui "insumo suficiente".

---

## 1. Propósito (Purpose)

**O que descobrir:** Qual problema resolve? Pra quem? Por que importa agora?

**Perguntas-base:**
- "Qual problema essa feature resolve?"
- "Quem é o usuário/público-alvo?"
- "Por que isso importa agora? O que muda se não fizermos?"

**Follow-ups se vago:**
- "Me dá um exemplo concreto de uso?"
- "Sem essa feature, como o usuário resolve hoje?"

**Pronto quando:** Dá pra responder "Essa feature resolve [problema X] para [público Y]
porque [motivo Z]" em 2 frases.

---

## 2. Escopo (Scope)

**O que descobrir:** O que entra e o que explicitamente NÃO entra nessa feature.

**Perguntas-base:**
- "O que essa feature DEVE fazer? Liste os 2-3 comportamentos principais."
- "O que definitivamente NÃO faz parte dessa feature? (vamos deixar explícito)"

**Follow-ups se vago:**
- "Isso que você mencionou — é core ou nice-to-have?"
- "Se tivesse que cortar algo por prazo, o que sairia primeiro?"

**Pronto quando:** Tem lista de "dentro" com 2-5 itens e lista de "fora" com pelo
menos 1 item explícito.

---

## 3. Comportamento Esperado (Happy Path)

**O que descobrir:** O fluxo principal, passo a passo, do início ao fim.

**Perguntas-base:**
- "Me conta o passo a passo do caso principal: o usuário abre o app/sistema e..."
- "O que o usuário vê/ouve/recebe em cada passo?"

**Follow-ups se vago:**
- "E depois desse passo, o que acontece?"
- "Tem alguma tela, mensagem, ou resposta específica nesse ponto?"

**Pronto quando:** Fluxo passo a passo cobre do início ao fim sem saltos lógicos.
Mínimo 3 passos para features não triviais.

---

## 4. Cenários Alternativos e Edge Cases

**O que descobrir:** O que pode dar errado? Como o sistema reage?

**Perguntas-base:**
- "O que acontece se [input inválido / falha de rede / dado faltando]?"
- "Tem cenário de concorrência? (dois usuários ao mesmo tempo)"
- "O que acontece se o serviço externo X estiver fora do ar?"

**Follow-ups se vago:**
- "Já viu esse problema acontecer em feature parecida?"
- "Qual o pior caso possível aqui?"

**Pronto quando:** Pelo menos 2 edge cases identificados com comportamento esperado
definido para cada um.

---

## 5. Constraints (Restrições)

**O que descobrir:** Limites técnicos, de tempo, budget, compliance.

**Perguntas-base:**
- "Tem limite de performance? (ex: 'tem que responder em menos de 200ms')"
- "Tem restrição de tecnologia? (ex: 'tem que ser em Go', 'não pode usar banco externo')"
- "Tem prazo? Budget?"

**Follow-ups se vago:**
- "Tem alguma regra de compliance ou segurança que se aplica?"
- "Roda em algum ambiente específico? (browser, mobile, embedded)"

**Pronto quando:** Constraints explícitas documentadas. Se não houver nenhuma,
registre "Nenhuma constraint além das definidas em constitution.md".

---

## 6. Critérios de Sucesso (Success Criteria)

**O que descobrir:** Como saber que a feature está pronta e funcionando?

**Perguntas-base:**
- "Como a gente sabe que essa feature ficou pronta? O que define 'done'?"
- "Tem métrica de sucesso? (ex: 'reduzir tempo de X em 50%', '0 erros no log')"

**Follow-ups se vago:**
- "Se fosse testar manualmente, o que verificaria?"
- "Tem algum benchmark ou baseline atual pra comparar?"

**Pronto quando:** Pelo menos 2 critérios mensuráveis (checklist style).
Se o usuário não souber, sugira critérios padrão: "testes passam",
"cenário principal funciona fim a fim".

---

## 7. Trade-offs e Abordagens (opcional, se houver escolha técnica)

**O que descobrir:** O usuário já tem preferência técnica ou quer sugestões?

**Perguntas-base:**
- "Tem preferência de arquitetura ou padrão? (ex: REST vs GraphQL, SQL vs NoSQL)"
- "Quer que eu sugira abordagens ou já tem uma em mente?"

**Pronto quando:** Abordagem técnica está clara OU o agente vai propor 2-3 opções
no Passo 4.

---

## Atalhos e Flexibilidade

- **"Não sei"**: o usuário não sabe responder. Ofereça sugestões baseadas em
  práticas comuns e peça para ele escolher ou refinar.
- **"Confio em você"**: preencha a dimensão com defaults razoáveis e apresente
  para validação no spec draft.
- **"Pula essa"**: pule a dimensão e registre "Não especificado" no spec.
  Só pule se o usuário insistir.
- **Usuário já cobriu a dimensão antes de você perguntar**: marque como coberta
  e avance. Não repita.
