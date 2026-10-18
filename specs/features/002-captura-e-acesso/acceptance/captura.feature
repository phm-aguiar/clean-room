# language: pt
# Convenção de contagem deste arquivo: cada cenário é uma linha "Cenário:" precedida por uma tag de tipo.
#   @principal   = Cenário Principal (Happy Path) da spec.md (1 cenário)
#   @alternativo = itens de "Cenários Alternativos" da spec.md (5 cenários)
#   @borda       = itens de "Edge Cases" da spec.md (6 cenários)
# Conferência (linhas de cenário, sem os comentários): grep -c "^  Cenário:" -> 12; grep -c "^  @principal" -> 1; grep -c "^  @alternativo" -> 5; grep -c "^  @borda" -> 6.
# Fonte: specs/features/002-captura-e-acesso/spec.md. Escopo: sem cenário de admissão recusada (fora da 002, phase0-findings.md item 3.5).

Funcionalidade: Captura e acesso (hooks, sessões, handoffs, MCP e CLI)
  Como autor que alterna entre agentes no mesmo diretório
  Quero que o trabalho dos agentes seja capturado sem esforço manual e entregue ao próximo agente
  Para continuar uma tarefa em outro agente sem re-explicar o contexto

  @principal
  Cenário: Captura de uma sessão e entrega ao agente seguinte
    Dado que os hooks do Claude Code foram registrados e o servidor MCP está instalado
    E que o servidor ai-memory está no ar
    Quando o agente emite os eventos de início de sessão, envio de prompt, pós-ferramenta e fim de sessão no diretório do projeto
    Então cada evento passa pela política de captura, é gravado no spool e enviado ao endpoint de hook
    E o servidor responde 202 para cada evento aceito
    E a observação sanitizada e a chave de ingestão são gravadas na mesma transação
    Quando a sessão encerra
    Então o servidor escreve a página da sessão sem usar LLM
    E abre o handoff automático
    E faz commit git
    Quando outro agente inicia no mesmo diretório
    Então o hook de início de sessão ou o aceite de handoff entrega o resumo da sessão anterior
    E a consulta de memória encontra o conhecimento da sessão anterior

  @alternativo
  Cenário: Servidor saturado responde 429 e o agente não é bloqueado
    Dado que o servidor está saturado e responde 429 ao endpoint de hook
    Quando o hook recebe um evento do agente
    Então o evento permanece no spool
    E o hook devolve o controle ao agente sem bloqueá-lo
    Quando chega um lote com mais de 256 itens
    Então o lote é rejeitado por inteiro

  @alternativo
  Cenário: Leitura com escopo explícito inexistente falha fechada, e leitura sem escopo degrada
    Dado que o escopo informado explicitamente na requisição não existe
    Quando o agente pede a consulta ou a leitura de uma página nesse escopo
    Então a operação falha com erro de não encontrado
    E nenhum workspace é criado
    E nenhum projeto é criado
    Quando o agente pede a leitura de uma página sem informar escopo
    Então a operação não falha
    E a leitura degrada pelo ponteiro de projeto ativo, pela semente de startup e pelo padrão do servidor
    E a resposta informa de qual dessas origens veio o escopo

  @alternativo
  Cenário: Escrita sem escopo e sem ponteiro resolvido é recusada como ambígua
    Dado que a escrita não informa escopo
    E que nenhum ponteiro de projeto ativo foi resolvido
    Quando o agente pede a escrita de uma página
    Então a escrita é recusada como escrita ambígua
    E nada é gravado no escopo padrão

  @alternativo
  Cenário: Falha no banco após escrever o arquivo reverte o arquivo
    Dado que o arquivo da página foi escrito no disco
    E que a gravação no banco falha em seguida
    Quando o agente pede a escrita da página com memory_write_page
    Então o arquivo é revertido ao estado anterior
    E o erro de escrita é devolvido ao agente de forma não ambígua

  @alternativo
  Cenário: Servidor offline mantém o evento no spool até a drenagem
    Dado que o servidor está offline
    Quando o hook recebe um evento do agente
    Então o evento fica no spool local
    Quando o servidor volta a responder
    Então um helper com lock drena o spool e envia os eventos pendentes

  @borda
  Cenário: Replay do mesmo evento não duplica a observação
    Dado que um evento com uma chave de ingestão já foi processado
    Quando o mesmo evento chega de novo, inclusive após interrupção no meio do fim de sessão
    Então nenhuma segunda observação é criada
    E os efeitos posteriores convergem para o mesmo resultado

  @borda
  Cenário: Handoff disputado tem exatamente um vencedor
    Dado que existe um handoff aberto
    Quando dois agentes aceitam o mesmo handoff ao mesmo tempo
    Então exatamente um agente recebe o handoff
    E o outro agente recebe handoff nulo

  @borda
  Cenário: Escritas concorrentes na mesma página formam cadeia de supersessão
    Dado que duas sessões escrevem a mesma página ao mesmo tempo
    Quando as duas escritas são processadas
    Então uma versão se torna a corrente
    E a versão perdedora supersede a anterior numa cadeia de supersessão
    E nenhuma versão é perdida

  @borda
  Cenário: Segredo no payload do hook é removido antes de gravar e os campos são limitados
    Dado que o payload do hook contém um segredo
    Quando o hook processa o evento
    Então o segredo é removido antes de qualquer gravação
    E o título fica com no máximo 80 caracteres após a remoção
    E o corpo fica limitado ao tamanho do tipo de evento, com 16 KiB para prompt, início e fim de sessão, pré-compactação, pós-compactação e subagentes
    E o corpo fica limitado a 2 KB para notificação, pré-ferramenta, pós-ferramenta, parada e outros tipos

  @borda
  Cenário: Resposta bruta do assistente nunca é persistida nem transmitida
    Dado que o evento de hook traz o campo bruto da última mensagem do assistente
    Quando o hook processa o evento
    Então o campo bruto é removido antes de qualquer etapa
    E o campo bruto nunca chega a log, spool, rede ou armazenamento

  @borda
  Cenário: Dois agentes no mesmo projeto não sobrescrevem o ponteiro um do outro
    Dado que dois agentes diferentes trabalham no mesmo projeto
    Quando cada agente atualiza o seu ponteiro de projeto ativo
    Então cada ponteiro fica associado ao seu próprio ator
    E o ponteiro de um agente não é sobrescrito pelo outro
