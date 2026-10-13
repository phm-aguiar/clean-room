# Design: promoção de memória para regras governadas no arquivo de instruções

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

*Status: **proposta, não implementada.** Esta página separa o que já existe no
produto (seção 2 e seção 7) do que ainda é proposta (seções 3 a 6 e seção 8).
Nenhuma versão lançada até a 2.6.2 inclui um mecanismo de promoção de memória
para o arquivo de instruções do agente. A proposta continua em aberto e precisa
de decisão do mantenedor antes de ser implementada.*

## 1. A ideia e a armadilha

O princípio que motiva a proposta é: **memória é contexto não revisado, não
política executável. Conhecimento aceito deve ser promovido para documentação de
projeto governada.** O produto já aponta nessa direção. O lint emite um
achado `rule_suggestion` ("esta página parece uma regra durável; considere
copiá-la para CLAUDE.md ou AGENTS.md"). A proposta fecha esse ciclo: transformar
uma memória durável em uma **regra governada, sempre carregada**, que o agente
obedece em todo turno.

A armadilha, e o motivo pelo qual a feature pode falhar se for mal feita: **o
arquivo de instruções é um orçamento escasso e compartilhado, não um arquivo
morto.** Se a promoção despejar toda regra aprendida nele, o arquivo incha, a
aderência cai e o agente piora sem aviso.

**Motivação externa (a confirmar).** Práticas de 2026 sobre arquivos de instruções
de agentes sugerem que o modelo segue um número limitado de instruções (faixa
citada: 150 a 200, parte dela já ocupada pelo prompt do próprio sistema); que
textos longos degradam a aderência, inclusive às regras mais importantes (queda
citada: acima de 30%); e que apontar para outro arquivo não economiza contexto,
porque o conteúdo é carregado na inicialização. Esses números não foram medidos
neste repositório: a confirmar. O teste útil para cada linha é "remover esta
linha faria o agente errar?". Se a resposta for não, a linha não pertence ao
arquivo. Por isso a promoção deve ser subtrativa por padrão: disputa um número
fixo de vagas e precisa remover para admitir.

## 2. Estado atual: o que já existe

Esta seção lista apenas comportamentos verificados na implementação. Itens marcados
como "a confirmar" ainda não foram conferidos até o fim.

| Capacidade | Situação | Comportamento verificado |
|---|---|---|
| Achado `rule_suggestion` no lint | Implementado | Gerado para página com metadado `kind: rule` **ou** caminho sob `_rules/`. Severidade `info`. A mensagem sugere copiar a página para CLAUDE.md ou AGENTS.md. |
| Opção de não gravar relatório no lint | Implementado | A opção de simulação (`--dry-run`) calcula os achados sem gravar o relatório na wiki. O relatório normal fica em `wiki/_lint/report.md`. |
| Bloco delimitado por marcadores de início e fim | Implementado | Marcadores `<!-- ai-memory:start -->` e `<!-- ai-memory:end -->`. Usados pelo comando de instalação de instruções, que é idempotente: re-executar substitui o bloco sem duplicá-lo. A desinstalação remove o bloco entre os marcadores. |
| Bloco de perfil em arquivo de regras | Implementado | O subcomando `profile apply` escreve as entradas de perfil que cabem no repositório dentro de um bloco próprio, delimitado por `<!-- ai-memory:profile:start -->` e `<!-- ai-memory:profile:end -->`. Nada fora do bloco é alterado. A opção de remoção retira o bloco. |
| Propostas de auto-melhoria sobre páginas de regra | Implementado | O revisor é somente-leitura. Propostas com tipo `rule` precisam apontar para caminho sob `_rules/`. Propostas abaixo do limiar de confiança são rejeitadas. As aprovadas vão para uma área de escrita pendente, `_pending/auto-improve`, e só viram páginas com aprovação humana. |
| Portão de avaliação opcional | Implementado | Comando externo opcional que avalia propostas para os alvos `_rules` e `procedures` (padrão). |
| Orçamento de tamanho de página de regra | Implementado | Teto aproximado de tokens por página final de `_rules/`. Padrão: 2.000. A estimativa usa 4 caracteres por token. |
| Páginas de regra no contexto do agente | Implementado | A busca retorna páginas `_rules/`. O resumo estruturado (memory_briefing) inclui uma lista `rules`. As instruções do servidor tratam essas páginas como evidência de alto valor, não como autoridade: o agente deve lê-las por completo e validá-las contra o pedido atual. |
| Relações tipadas `causes`, `fixes`, `contradicts` | Implementado | A consolidação emite essas relações somente quando há evidência. A leitura as expõe. Contradições vivas entram no cálculo de confiança de crença (ver abaixo). |
| Saliência e decaimento por página | Implementado | Saliência padrão 1,0. Feedback explícito move a saliência da página. Há um módulo de decaimento por nível de página. O efeito do decaimento no ranking: a confirmar. |
| Confiança de crença por página | Implementado, sem efeito por padrão | Calculada na leitura, não armazenada. Peso de autoridade padrão 0,0, portanto **não altera o ranking por padrão**. Exposição ao usuário: a confirmar. Detalhes na seção 7. |
| Comandos `rules recommend`, `approve`, `edit`, `remove`, `list` | **Não existem** | Nenhum subcomando, ferramenta MCP ou rota com esses nomes. |
| Bloco de promoção, chaves `[rules_promotion]` e subbloco "regras promovidas" | **Não existem** | Nenhuma chave de configuração, marcador ou escritor com esses nomes. |
| Escrita automática no arquivo de instruções | **Não existe** | Propostas de auto-melhoria alvejam páginas da wiki, não o arquivo de instruções. O prompt do revisor proíbe promover trechos de roteamento ou instruções do próprio arquivo, a menos que a sessão tenha mudado explicitamente a política do projeto. |

**Conclusão:** a ideia de promoção não tem módulo dedicado. Os blocos de base
(sinal de candidato, marcadores, revisão humana e confiança) existem. Falta a
camada que decide, pontua, limita e escreve as regras.

## 3. Duas camadas, uma fronteira clara

*(Proposta de modelo. Os dois níveis já existem na prática como páginas de
regra e páginas de conhecimento, mas a separação de custo e de carregamento
descrita abaixo ainda não foi implementada.)*

| | **Regra governada** (arquivo de instruções) | **Conhecimento de recuperação** (padrão) |
|---|---|---|
| Carregamento | Em todo turno, sempre | Sob demanda, durante o planejamento, pela busca de memória |
| Custo | Ocupa um espaço do orçamento escasso | Sem custo fixo |
| Para quê | Invariantes, guardrails e convenções que o agente nunca deve violar e que valem amplamente | Fatos específicos de contexto, decisões, armadilhas e histórico: "o que fizemos e por que X é assim" |
| Teste | "Esquecer isto causa erro em muitas tarefas" | "Só é relevante quando a tarefa toca nisso" |
| Exemplos | "Toda escrita no armazenamento passa por um único ponto de escrita"; "nunca escreva arquivos no lugar: use arquivo temporário e renomeie" | "Em julho escolhemos X em vez de Y porque..."; uma armadilha de depuração de um mês atrás |

**O padrão é recuperação.** Uma memória de tipo regra e durável continua
somente de recuperação, a menos que atenda a um critério mais alto (seção 4).
Essa é a postura anti-inchaço: promover é exceção, não recompensa por
durabilidade. Conhecimento antigo, armadilhas pontuais e decisões com contexto
devem aparecer no planejamento de tarefas, nunca permanentemente.

## 4. Critérios para promoção (proposta)

Um candidato (página `_rules/` que o lint já sinaliza) só é elegível se for
**amplo, imperativo, durável e barato de enunciar**:

1. **Aplicabilidade ampla:** restringe muitas tarefas, não um arquivo ou uma
   funcionalidade. Heurística: referenciada ou acessada em várias sessões e
   caminhos, não uma nota de escopo único.
2. **Imperativo e falsificável:** diz "faça X" ou "nunca Y", não "discutimos Z".
   Deve ser possível flagrar o agente violando a regra.
3. **Durável e de alta confiança:** sobreviveu a N sessões sem contradição e
   tem confiança acima de um limiar. Não pode haver relação `contradicts`
   aberta apontando para ela. *(Proposta. A confiança de crença já existe
   (seção 7), mas o limiar e a fonte da confiança para promoção ainda precisam
   de decisão.)*
4. **Compacta:** uma ou duas linhas. Se precisar de um parágrafo de contexto,
   é conhecimento de recuperação, não regra.

Tudo que falhar em algum critério permanece somente de recuperação. Assim o
conjunto promovido fica pequeno por construção, e não por poda posterior.

## 5. Anti-inchaço por mecanismo, não por disciplina (proposta)

- **Orçamento rígido.** Um teto configurável sobre o bloco gerenciado. Valor
  sugerido, pequeno: até 15 regras promovidas ou aproximadamente 40 linhas. O
  valor é pergunta aberta (seção 9). A promoção é controlada por admissão contra
  esse teto.
- **Substituir para admitir, mas o usuário escolhe.** No limite, o comando de
  aprovação não transborda em silêncio. Ele nomeia a regra atual de menor
  pontuação e pede ao usuário que a remova primeiro, ou que aumente o teto.
  Nada é rebaixado sem um comando explícito. A pontuação proposta é confiança ×
  amplitude × recência de uso, usada só para ordenar e para sugerir o que
  remover.
- **Decaimento reduz a posição, nunca remove.** Uma regra promovida que deixa de
  ser exercitada cai na ordenação da recomendação e pode ser marcada como
  possivelmente obsoleta, para o usuário remover. A remoção é sempre um comando
  humano, nunca automática.
- **Nunca escreve no caminho quente.** O classificador e o pontuador que geram
  candidatos podem reaproveitar a maquinaria de consolidação. O bloco do
  arquivo de instruções só é escrito por aprovação, edição ou remoção explícitas,
  nunca como efeito colateral de uma sessão.

## 6. Comandos e garantias (proposta)

Nenhum destes comandos existe hoje (ver seção 2).

**Superfície proposta** (CLI, espelhada como ferramentas MCP para que um agente
possa executá-la quando o usuário pedir):

- **`rules recommend`:** somente leitura. Lista os candidatos ordenados, cada
  um com a regra em uma linha, a página de origem, a confiança, o sinal de
  amplitude e o motivo da qualificação. Não altera nada. É a superfície de
  descoberta.
- **`rules approve <id>`:** promove um candidato para o bloco gerenciado do
  arquivo de instruções. Se o bloco estiver no teto, o comando informa o
  usuário (por exemplo, "no limite de 15/15; aprovar significa remover a regra
  de menor pontuação, ou aumentar o teto") e não faz nada até a decisão do
  usuário.
- **`rules edit <id>`:** ajusta a redação da regra antes ou depois de promover.
- **`rules remove <id>`:** rebaixa uma regra promovida para somente
  recuperação. A página `_rules/` não é alterada; só a linha do arquivo de
  instruções sai.
- **`rules list`:** mostra o que está promovido no bloco gerenciado, com
  origem.

**Garantias propostas:**

- **Um bloco gerenciado e delimitado.** Reaproveita o mecanismo de marcadores já
  existente, em um subbloco rotulado, por exemplo
  `<!-- ai-memory: promoted rules (managed) -->`. O conteúdo escrito à mão fora
  dos marcadores **nunca** é alterado.
- **Proveniência por regra.** Cada linha promovida traz um rastro curto com a
  página de origem e a confiança, para o leitor saber por que ela existe e onde
  abrir a memória.
- **Recomendações são puxadas, não empurradas.** Os candidatos se acumulam em
  silêncio e só aparecem quando o usuário executa `recommend`. A única
  sinalização opcional seria uma linha de status, desligada por padrão, quando o
  conjunto de candidatos cresce. Nada de mensagens a cada turno.
- **Totalmente reversível.** Sem a aprovação, ou removendo o bloco gerenciado, o
  arquivo de instruções fica exatamente como o humano escreveu. Remover ou
  rebaixar não perde nada, porque a página `_rules/` continua consultável.

**Fluxo de propostas automáticas que já existe.** A revisão de propostas de
auto-melhoria segue outro caminho e não escreve no arquivo de instruções. A
proposta fica em `_pending/auto-improve` e só vira página da wiki quando o
usuário aprova pelo comando de propostas pendentes (listar, ver, comparar,
aprovar, rejeitar). A promoção proposta aqui não usa esse caminho: é um
comando humano de escrita no arquivo de instruções. O classificador e o
pontuador podem reaproveitar a maquinaria de consolidação; só a escrita fica
condicionada ao comando.

## 7. Reaproveitamento do que já existe

Nada aqui exige um subsistema novo. A proposta compõe primitivas que já existem:

- **Sinal de candidato:** o achado `rule_suggestion` do lint (seção 2).
- **Durabilidade e contradição:** páginas `_rules/`, relações tipadas
  `contradicts` e saliência com decaimento.
- **Revisão:** o caminho de propostas de auto-melhoria em área pendente, com
  portão de avaliação opcional.
- **Escrita:** o mecanismo de bloco delimitado por marcadores, já usado pela
  instalação de instruções e pela aplicação de perfil.
- **Confiança, em duas formas que já existem:**
  - *Confiança de proposta* de auto-melhoria. O limiar padrão é 0,75 e está
    na configuração de exemplo.
  - *Confiança de crença por página.* Calculada na leitura a partir de evidência,
    sem valor armazenado. Três fatores multiplicados e limitados a 0,95:
    - **Sessões distintas:** curva de saturação `1 − e^(−amplitude/3)`. Evidência
      sem sessão independente conta 0,2 por registro, com teto de 1,0 sessão
      equivalente. Uma sessão citada cinquenta vezes vale pouco.
    - **Recência:** multiplicador entre 0,5 (piso) e 1,0, com constante de tempo
      de 30 dias. Evidência antiga perde posição, mas não some.
    - **Contradições vivas:** fator `1 / (1 + n)`.
    - O peso de autoridade padrão é 0,0, então esse valor **não altera o
      ranking por padrão**. Qualquer uso dele na promoção depende de decisão.

## 8. Escopo proposto e depois

**Proposta de escopo da primeira entrega (ainda não iniciada):**

- Configuração `[rules_promotion]` com `enabled` (padrão falso), `max_promoted`,
  `min_confidence` e `recommend_hint` (dica opcional de status, padrão desligada).
  **Não há `auto_apply`. Esse modo não existe e não deve ser criado.**
- O classificador (seção 4) e o pontuador (seção 5) sobre candidatos `_rules/`.
- A superfície de comandos da seção 6, com `recommend` e `list` somente leitura.
  `approve`, `edit` e `remove` alteram o bloco gerenciado e exigem comando
  explícito.
- O escritor do subbloco "regras promovidas", com rastros de proveniência,
  escrito somente por `approve`, `edit` e `remove`.
- Relato de status das regras promovidas e, se houver, a dica de novos candidatos.
- Rebaixamento por `remove`. Decaimento só reduz a posição em `recommend` e pode
  marcar uma regra para o usuário remover. Nunca remove automaticamente.

**Fora do escopo da primeira entrega:**

- Compartilhamento de regras entre projetos ou em equipe.
- Regras com escopo por padrão de arquivo, em vez de um único bloco global.
- Síntese automática de regras (fundir várias memórias em uma regra). Começaria
  apenas com revisão humana.

## 9. Perguntas abertas

1. O teto padrão de 15 regras ou 40 linhas é o certo, ou deve ser mais apertado?
2. O bloco gerenciado deve ficar no arquivo de instruções do agente, no arquivo
   de regras do Claude Code, ou em ambos? Como evitar carregamento duplicado
   quando uma ferramenta lê os dois?
3. A aprovação deve gerar opcionalmente um commit, para registrar a promoção no
   histórico do repositório, ou deixar isso para o fluxo normal do usuário?
4. A saída do `recommend` deve mostrar só os N primeiros (cinco, por exemplo),
   com uma opção para listar tudo?
5. Limiar de promoção: usar a confiança de proposta, a confiança de crença por
   página ou uma combinação? Em qual valor?

*Decisão registrada (2026-09-03): o produto não edita o arquivo de instruções
por conta própria. A feature só recomenda; os comandos `approve`, `edit` e
`remove` são os únicos que escrevem o bloco gerenciado. A seção 6 reflete essa
decisão.*

## Referências internas

- [Ciclo de melhoria automática](auto-improvement-loop.md): propostas,
  revisão e área pendente.
- [Portões de avaliação da melhoria automática](auto-improve-eval-gates.md).
- [Relações tipadas](typed-edges.md): `causes`, `fixes` e `contradicts`.
- [Envelhecimento de memória](design-memory-aging.md): decaimento e saliência.
- [Empréstimos de design para confiança de crença](design-hindsight-borrowings.md).
- [Perfil entre projetos](cross-project-profile.md): aplicação de perfil em
  bloco delimitado.
- [CLI](cli.md): instalação de instruções e comandos.
