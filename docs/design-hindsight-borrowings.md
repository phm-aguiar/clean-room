# Empréstimos de design: estado implementado e propostas

*Última revisão: 2026-10-11, contra a base de código atual (versão 2.6.2 no registro de mudanças).*

Este documento descreve cinco ideias tomadas de um estudo externo sobre sistemas de memória para agentes. Para cada ideia, separa-se o que está **implementado** (comportamento atual, descrito com algoritmo, constantes, valores padrão e casos de borda) do que é **proposta** (sem código correspondente, descrita só como intenção). Nenhuma proposta deve ser lida como comportamento atual.

Os empréstimos seguem três restrições. Nenhum deles exige arquitetura nova. Nenhum deles pode exigir provedor de LLM para captura, busca ou contagem. Nenhum deles pode fazer uma correção ser bloqueada ou apagada por ter pouca evidência.

## 1. Resumo do estado

| Item | Ideia emprestada | Estado | O que falta |
|---|---|---|---|
| P1 | Rótulos tipados de redação | Implementado (versão 2.2.0) | Nada |
| P2 | Força de crença por evidência | Substrato, contagem e explicação implementados. Fator de autoridade implementado, mas desligado por padrão | Ativação por padrão depende de medição de recuperação |
| P3 | Recuperação por aresta causal tipada | Tipo da aresta aparece na explicação. Ranking não usa o tipo | Peso por tipo, limite por contradição e travessia de cadeia são propostas |
| P4 | Páginas de respostas assentadas | Opção `settled_first` implementada (desligada por padrão). Passe de experiência existe e é opt-in | Faixa de páginas mantidas pelo passe é proposta |
| P5 | Onboarding por assinatura | Implementado no guia de instalação | Nada |

Além disso, a recuperação temporal (janelas de validade e consulta `as_of`) já existia antes destes empréstimos e não é tratada aqui. Ver [temporal.md](temporal.md) e [design-page-ingestion-windows.md](design-page-ingestion-windows.md).

## 2. Invariantes que delimitam os empréstimos

- **Caminho padrão sem provedor de LLM.** Captura, busca e contagem de evidência funcionam sem provedor configurado. A contagem de evidência é baseada em regras. O passe de experiência é opt-in.
- **Escrita única.** Toda escrita passa por um único ator de escrita. A evidência de uma página é gravada na mesma transação da escrita da página.
- **Migrações com arquivo de segurança.** Antes de uma migração de esquema avançar, o sistema grava um arquivo de segurança pré-migração.
- **Superfície de ferramentas MCP fixa.** A lista tem 23 ferramentas e um teste confere o conjunto registrado. Nenhum empréstimo adiciona ferramenta. Os novos campos entram como parâmetros opcionais ou campos de explicação.
- **Correção vence contagem.** Uma nova versão que diverge substitui a versão atual, mas o histórico continua acessível. A quantidade de evidência nunca impede uma correção de ser gravada ou retornada.

## 3. P1: rótulos tipados de redação

### 3.1 Implementado

Quando um trecho casa com um padrão de segredo, o texto é substituído por `[REDACTED:<tipo>]`, em vez de uma máscara genérica. O tipo indica o que foi removido, sem revelar o valor.

Tipos embutidos e o que cobrem:

| Tipo | Cobre |
|---|---|
| `bearer_token` | Credencial precedida de "Bearer", com 16 ou mais caracteres válidos |
| `api_key` | Chaves com prefixo `sk-` de 16 ou mais caracteres |
| `stripe_key` | Chaves secretas e restritas de conta Stripe em modo real |
| `github_token` | Todos os prefixos de token GitHub (`ghp_`, `gho_`, `ghu_`, `ghs_`, `ghr_`) e tokens de granularidade fina |
| `aws_key` | IDs de chave AWS de longo prazo (`AKIA`) e temporários (`ASIA`), no formato exato de 20 caracteres, com fronteira de palavra |
| `google_api_key` | Chaves Google/Gemini no formato antigo (`AIza`) e no formato de autorização (`AQ.Ab`) |
| `google_oauth` | Refresh tokens OAuth do Google (prefixo `1//`) |
| `meta_token` | Tokens de acesso do Meta Graph (prefixo `EAA`) |
| `telegram_token` | Token de bot Telegram, no formato emitido e no formato dos exemplos publicados pelo fornecedor |
| `ghl_token` | Tokens `pit-` seguidos de UUID |
| `slack_token` | Tokens `xox?-` e `xapp-` |
| `jwt` | Três segmentos base64url separados por ponto |
| `base64_json_token` | Blob base64 de JSON, sem ponto, com 40 ou mais caracteres (inclui tokens de túnel) |
| `private_key` | Bloco PEM de chave privada, inclusive em várias linhas |
| `url_credentials` | Credenciais embutidas em URL (`esquema://usuário:senha@host`) |
| `auth_header` | Cabeçalhos de autenticação com valor opaco (por exemplo `X-Api-Key`, `Private-Token`, `Ocp-Apim-Subscription-Key`). Sufixos genéricos `key` ou `token` só contam quando qualificados por palavra de autenticação |
| `env_secret` | Atribuições de variáveis de ambiente conhecidas (por exemplo `ANTHROPIC_API_KEY`, `GITHUB_TOKEN`) e a família genérica `*_KEY`, `*_TOKEN`, `*_SECRET`, `*_PASSWORD`, `*_PASSWD`, `*_CREDENTIAL(S)`, `*_PRIVATE_KEY`, sem diferenciar maiúsculas. A família `*_KEY_ID`, `*_PASSPHRASE`, `*_SIGNING_KEY`, `*_PEPPER` e `*_SALT` só casa em maiúsculas |
| `credential_path` | Caminhos de diretórios de credenciais (`.ssh`, `.aws`, `.kube`, configuração do gcloud, `.gnupg`), com separadores Windows ou Unix e prefixo de unidade opcional |
| `custom` | Padrões definidos pelo operador em `[sanitize].extra_patterns` |

Regras de aplicação:

- Os padrões são aplicados em ordem. Um trecho já substituído por um rótulo não é casado de novo.
- `[sanitize].allowlist` é checada por casamento. Se o trecho casado contém uma entrada da lista, ele fica intacto. O padrão continua valendo para o restante do texto.
- Um padrão de operador inválido impede a inicialização com erro claro.
- Segredos de alta entropia sem formato reconhecível não são detectados. O operador pode cobrir esse caso com `extra_patterns`.

### 3.2 Compatibilidade

- Texto já gravado com a marca antiga `[REDACTED]` não é reescrito. Escritas novas usam o rótulo tipado. Isso é uma mudança visível no conteúdo durável, por isso foi anunciada no registro de mudanças da versão 2.2.0.
- Redação continua completa e idempotente: aplicar o filtro duas vezes não altera o resultado.

### 3.3 Casos de borda conhecidos

- O limite de 80 caracteres de título é aplicado depois da redação. Se o corte cair dentro de um marcador, o título pode mostrar um fragmento como `[REDACTE…`. Nenhum segredo vaza, porque a redação já ocorreu. Esse defeito estético é aceito.

## 4. P2: força de crença por evidência

### 4.1 Implementado: registro de evidência

Cada versão de página pode ter zero ou mais registros de evidência. Cada registro tem tipo de fonte, identificador da fonte e instante de criação. A combinação de página, tipo e identificador é única. Repetir a mesma citação não aumenta a contagem.

Tipos de fonte definidos: `session`, `observation`, `feedback` e `reconsolidation`.

Produtores atuais:

- `session`: consolidação de sessão, no caminho individual e no de lote. Também é gravado quando uma escrita de página informa o identificador da sessão de origem.
- `reconsolidation`: passe de sonho e fusão de agrupamentos antigos. A fusão registra a proveniência da página resultante como evidência.
- `observation` e `feedback`: existem no vocabulário e são aceitos pelo armazenamento, mas nenhum produtor atual os grava.

Comportamento de escrita:

- A evidência é gravada na mesma transação da página. Se o conteúdo não mudou, a reafirmação ainda grava evidência contra a versão atual, sem criar nova versão.
- Apagar uma página remove a evidência dela em cascata.
- Página sem evidência tem contagem zero. Isso significa "desconhecido", não "sem suporte". Nenhum valor negativo é usado.

### 4.2 Implementado: confiança derivada na leitura

A confiança não é armazenada. É recalculada a cada consulta a partir da evidência e dos contraditórios vivos. Entradas:

- `contagem`: total de registros de evidência da versão.
- `sessões distintas`: número de identificadores distintos entre os tipos `session` e `reconsolidation`.
- `idade`: tempo desde o registro mais recente, em dias.
- `contradições vivas`: número de arestas `contradicts` declaradas pela página apontando para uma página atual. Arestas pendentes (alvo inexistente ou não atual) não contam.

Cálculo, com constantes:

1. Resíduo = max(0, contagem − sessões distintas).
2. Amplitude = sessões distintas + o menor valor entre (0,2 × resíduo) e 1,0. Registros que não são de sessão contribuem pouco e têm teto.
3. Suporte = 1 − e^(−amplitude / 3). A constante 3 é o ponto de saturação.
4. Recência = 0,5 + 0,5 × e^(−idade / 30 dias). Piso de 0,5. Se o instante do registro mais recente for desconhecido, a recência vale 1,0.
5. Penalidade de contradição = 1 / (1 + contradições vivas).
6. Confiança = suporte × recência × penalidade, limitada ao intervalo de 0 a 0,95.

Propriedades garantidas por testes:

- Mais sessões distintas aumenta a confiança.
- Mais tempo desde a evidência mais recente diminui a confiança, mas nunca abaixo de metade da parte de suporte.
- Mais contradições vivas diminui a confiança.
- Muitas citações de uma única sessão valem muito menos que citações de muitas sessões.
- A confiança nunca chega a 1,0. O teto é 0,95.
- Sem evidência, a confiança é 0,0. Não há efeito no ranking.

### 4.3 Implementado: exposição

- A explicação da busca (quando solicitada) mostra por resultado: `evidence_count`, `confidence` e o fator de autoridade usado. Quando o fator de crença está ativo, também mostra `belief_factor`.
- A explicação calcula a confiança mesmo com o fator desligado. Ela é apenas leitura.
- O status do projeto mostra `evidence_rows`: total de registros de evidência do projeto. Não é contagem por página.

### 4.4 Implementado, desligado por padrão: fator de autoridade

- A configuração `belief_authority_weight`, na seção de recuperação, tem valor padrão `0.0`.
- Com peso zero, nenhuma consulta de evidência roda no caminho de busca sem explicação, e o ranking é idêntico ao de uma base sem o recurso.
- Com peso positivo, o fator de autoridade da página vira `base + peso × confiança`, limitado ao intervalo [0,55; 1,50]. Esse é o mesmo intervalo do fator de autoridade existente. Não há multiplicador novo.
- Quando a busca inclui versões antigas, versões marcadas como substituídas não recebem o acréscimo. Isso preserva a regra de que uma correção vence contagem.
- O comentário no código diz que ativar o recurso por padrão exige um delta positivo em avaliação de recuperação, e que essa medição ainda não foi feita. Por isso o padrão continua zero.

### 4.5 Semântica de reforço, enfraquecimento e extensão

- **Reforço:** uma nova sessão que cita a mesma página acrescenta um registro de evidência. A contagem sobe.
- **Enfraquecimento:** um contraditório vivo reduz a confiança na leitura. Nenhuma escrita acontece.
- **Extensão:** uma página que cresce ou muda gera uma nova versão e substitui a anterior. O histórico é mantido.

Três guardas contra "crença popular e errada":

- A amplitude vem de sessões distintas, não de contagem bruta.
- A recência reduz o peso de evidência antiga.
- O teto de 0,95 impede que só evidência leve a página ao máximo.

Uma quarta guarda está fora do cálculo. Uma correção sempre substitui a versão anterior, independentemente da contagem.

### 4.6 Proposta: não implementada

- Ativar o fator de autoridade por padrão. Depende de medição positiva em avaliação de recuperação.
- Produtores para os tipos `observation` e `feedback`. Hoje não há quem os grave.
- Evidência em arestas ou entidades, em vez de páginas. Mantida como questão em aberto.

## 5. P3: recuperação por aresta causal tipada

### 5.1 Implementado

- Arestas entre páginas têm um de quatro tipos: `references` (padrão, link simples), `causes`, `fixes` e `contradicts`. São persistidas com o tipo.
- O fluxo de vizinhança por grafo parte das páginas de semente que já foram encontradas por outros fluxos. Ele pega vizinhos de **um salto**, em ambas as direções (saída e entrada), **sem filtrar por tipo**. Vizinhos com arestas tipadas e simples entram com a mesma prioridade.
- A ordem do fluxo é: posição da semente, depois direção (saída antes de entrada), depois data de atualização mais recente, depois caminho em ordem alfabética. Uma página aparece uma vez só, e as sementes não aparecem como vizinhas. O resultado é cortado no limite do fluxo.
- Na explicação, cada vizinho traz `graph_via` com a semente de origem, a direção e, quando a aresta não é `references`, o tipo da aresta em `edge`. A explicação mostra o motivo, mas o tipo **não altera** a posição no ranking.
- A verificação de contradições do lint lê as arestas `contradicts` declaradas por páginas atuais e indica se o alvo existe (resolvida ou não).

### 5.2 Proposta: não implementada

- Peso por tipo de aresta no ranking. Uma aresta `fixes` ou `causes` pesaria mais que uma referência simples para consultas sobre bugs e decisões.
- Limite de autoridade para a página do lado não resolvido de uma aresta `contradicts`. Hoje só há efeito indireto: a contradição reduz a confiança, e a confiança só afeta o ranking quando o fator de crença está ativo.
- Travessia opcional de cadeias `fixes` e `causes`, com profundidade máxima de 2 e atrás de uma opção de configuração. Hoje a vizinhança é sempre de um salto.

Essas propostas exigem medição antes de virar padrão. A regra é a mesma usada para o fator de crença.

## 6. P4: páginas de respostas assentadas

### 6.1 Implementado: opção `settled_first`

- A ferramenta de briefing aceita `settled_first` (booleano, padrão `false`).
- Com `false`, o briefing não muda.
- Com `true`, o briefing ganha uma lista `settled` com até 8 páginas atuais, do tipo regra ou decisão, que não estejam expiradas. A ordem é: contagem de evidência decrescente, depois data de atualização decrescente.
- A contagem usada é a de registros de evidência da página, incluindo tipos que não são sessão.

### 6.2 Implementado, opt-in: passe de experiência entre sessões

- Desligado por padrão. A configuração `experience_every_sessions` tem valor `0`. Com valor positivo, o passe roda por projeto depois que esse número de sessões novas, concluídas desde o último passe, for atingido.
- Lê até `experience_sessions` páginas-resumo de sessão mais recentes (padrão 10). Sessões sem página-resumo são ignoradas.
- Se houver menos páginas que o mínimo, o passe é pulado. Isso não é erro.
- Exige provedor de LLM. Sem provedor, o passe não roda.
- As propostas geradas passam pelo mesmo caminho de staging e pelo mesmo gate de avaliação da revisão por sessão. Os tipos cobertos são procedimentos, regras e preferências (com citação de sessão), e decisões quando várias sessões mostram uma decisão de fato não registrada.
- Um filtro de entropia opcional (desligado por padrão) remove páginas de baixa informação do prompt. A página é pulada, nunca apagada.

### 6.3 Proposta: não implementada

- Uma faixa explícita de "respostas assentadas" mantidas pelo passe de experiência. Hoje o passe gera propostas, não mantém páginas de resposta.
- Uma guarda contra reescrita em todo ciclo: reescrever só quando a evidência mudar.
- Ordenar o briefing por confiança, e não pela contagem bruta. Hoje a ordem é só contagem.
- Decidir se a lista `settled` deve aparecer como seção visível própria ou se deve apenas enviesar a ordem do briefing. Mantida como questão em aberto.

## 7. P5: onboarding por assinatura

### 7.1 Implementado

O guia de instalação diz que não é preciso chave de API paga para as funções de LLM. Ele lista as opções por assinatura já existentes:

- Plano Claude Pro ou Max, via provedor de token OAuth da Anthropic.
- Login ChatGPT Plus ou Pro, via provedor OAuth da OpenAI.
- Plano GitHub Copilot, via provedor Copilot.

A tabela de provedores do guia também traz essas opções com o nome da variável de ambiente correspondente. Ver [install.md](install.md).

### 7.2 Proposta

Nenhuma.

## 8. Ideias descartadas

Estas ideias do estudo não são planejadas:

- **Captura que exige provedor de LLM.** Contradiz a regra de caminho padrão sem provedor.
- **Substrato de banco de servidor.** O produto continua sendo um binário único e autocontido.
- **Isolamento estrito por "banco" de memória.** A autorização por projeto é tratada em [design-per-project-authz.md](design-per-project-authz.md). Ela acrescenta uma fronteira acima do compartilhamento dentro do projeto, sem substituí-lo.
- **Traços de disposição (ceticismo, literalidade, empatia).** Metáfora sem prova. Não há plano, até que um experimento de ablação mostre algum efeito.

## 9. Questões em aberto

1. **Ativação do fator de crença (P2).** Qual delta de recuperação justifica ligar `belief_authority_weight` por padrão, e com qual valor?
2. **Peso por tipo de aresta (P3).** Deve ser padrão depois de medido, ou opção explícita de configuração?
3. **Travessia de cadeias (P3).** Vale a complexidade de uma travessia de dois saltos, ou a vizinhança de um salto basta?
4. **Faixa de respostas assentadas (P4).** A lista `settled` deve ser uma seção própria no briefing, ou basta enviesar a ordem?
5. **Produtores de `observation` e `feedback` (P2).** Quais caminhos de escrita devem gravar esses tipos? Hoje nenhum grava.

Decisões já tomadas e implementadas:

- Rótulo tipado fica no corpo do texto gravado, e não só em metadado lateral (P1).
- Confiança é derivada na leitura, e não armazenada em coluna mutável (P2).
- A evidência fica no nível da página, que é a unidade de autoridade do ranking (P2).
