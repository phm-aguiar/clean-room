# Modelo de ameaças e tratamento de dados

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

> Descreve o modelo de ameaças e o tratamento de dados do ai-memory: o que o desenho defende, o que
> fica fora de escopo, os riscos que ainda não têm controle, e os caminhos que levam dados para fora
> do equipamento. Serve para (a) saber quais garantias uma implementação deve oferecer e (b) responder
> a revisões de segurança e de privacidade. O texto consolida a política de segurança e a nota de
> tratamento de dados do projeto de origem, conferidas contra o comportamento implementado.
> Complementa [security.md](security.md) (modelo operacional), [security-boundaries.md](security-boundaries.md)
> (fronteiras e testes adversariais), [https-via-proxy.md](https-via-proxy.md), [sso.md](sso.md) e
> [airgapped-install.md](airgapped-install.md).

## 1. Premissa de confiança

- O ai-memory é um serviço de **um único inquilino**, pensado para estação de trabalho ou servidor doméstico.
- Suporta vários usuários atribuídos, mas todo usuário autenticado pertence ao **mesmo domínio de
  confiança** e lê a mesma memória de projeto. Páginas são compartilhadas; handoffs têm dono.
- Um projeto pode ser `open` (padrão) ou `restricted`. Projeto restrito só é acessado por quem tem
  concessão de `read` ou `write` (ver [design-per-project-authz.md](design-per-project-authz.md)).
  A opção de criar projetos novos já restritos (`[auth].new_projects_restricted`) é **desligada por
  omissão** (confirmado no original): projetos novos são abertos. Ligada, um projeto novo admite só o
  criador (que recebe `write`) e o raiz; projetos existentes nunca mudam, e `scratch` e o escopo global
  de preferências ficam sempre abertos.
- Não há criptografia em repouso nem isolamento forte entre inquilinos. Quem precisa separar dados
  roda **servidores e diretórios de dados distintos**.

## 2. Controles implementados

### 2.1 Confidencialidade local

- Wiki e banco de dados ficam num único diretório de dados do usuário do sistema operacional.
  Não há criptografia em repouso: a proteção depende das permissões do sistema operacional.
- Em Unix, diretórios criados pelo programa recebem modo `0700`. Arquivos criados (configuração,
  banco de dados, spool de eventos de hook, backups baixados, arquivo de credenciais de provedores)
  recebem `0600`. O modo é definido na criação; o umask do processo só pode restringir mais.
- Instalações existentes não são alteradas automaticamente.
- Em Windows, o programa não define ACL explícita: vale a ACL herdada do diretório (confirmado no
  original: os ajustes de modo existem só em Unix; não há código de ACL para Windows).

### 2.2 Exposição de rede

- Endereço padrão: loopback, porta `49374`, **sem autenticação**.
- Bind em endereço não loopback sem autenticação é **recusado** (falha fechada). Para liberar, defina
  o token de autenticação (`AI_MEMORY_AUTH_TOKEN`, gerado pelo comando de geração de token) ou use a
  opção explícita `--allow-insecure-no-auth`, reservada a LAN deliberada em HTTP puro. Essa opção
  emite aviso de sistema em toda inicialização.
- Em contêiner, a verificação **apenas avisa**, porque quem decide o alcance é a publicação de porta
  no host. `-p 127.0.0.1:49374:49374` é seguro sem token; qualquer publicação mais ampla exige token.
  A detecção de contêiner usa marcadores de arquivo que os runtimes de contêiner criam no sistema de arquivos.
- Login humano (senha e sessão de navegador) em endereço não loopback só é aceito com cookie seguro
  (`[auth].secure_cookie=true`), atrás de proxy HTTPS confiável.
- Autenticação não cifra o token. Para tráfego além do loopback, use proxy reverso com TLS. O servidor
  não confia em cabeçalhos de protocolo encaminhado para decidir se o cookie é seguro.

### 2.3 Cabeçalho Host e tamanho de requisição

- **Allowlist de Host** (`AI_MEMORY_ALLOWED_HOSTS`). Padrão: `localhost`, `127.0.0.1` e `::1`.
  A comparação ignora maiúsculas e aceita o host com porta. Host fora da lista → **403**.
  Requisição sem cabeçalho Host → **400**. O filtro vale para todas as rotas.
- A allowlist defende contra DNS rebinding, em que o navegador define o Host. Não substitui
  autenticação: um cliente que alcança a porta define o Host livremente.
- **Corpo HTTP limitado a 10 MiB** nas rotas gerais. Exceção: a rota de bootstrap administrativo aceita
  até **32 MiB**.

### 2.4 Autenticação e autorização

Identidades aceitas, da mais forte para a mais fraca:

1. **Token raiz estático** (`[auth].bearer_token` ou `AI_MEMORY_AUTH_TOKEN`). Comparação em tempo
   constante. Resulta em papel raiz.
2. **Token de proxy confiável** (`[auth].actor_proxy_bearer_token`, diferente do token raiz). Com ele, o
   proxy afirma a identidade do usuário por cabeçalhos. Cabeçalho repetido ou com vírgula, ou identidade
   parcial (só emissor ou só sujeito), gera **400**. A identidade afirmada só vira raiz se casar exatamente
   com a identidade raiz configurada (par emissor e sujeito). Caso contrário, vira papel de usuário.
3. **Token de usuário do banco** (credencial com prefixo `aim_`). Papel de usuário. Serve para
   **atribuição, nunca administração**. O último acesso é atualizado em segundo plano.
4. **Sessão humana** (login por senha). Gera cookie de sessão `HttpOnly` com `SameSite=Strict` e token
   CSRF enviado em cabeçalho. Duração: 7 dias.
5. **Identidade OIDC**. O cliente obtém token por fluxo de autorização de dispositivo e o envia. No
   servidor, a identidade chega como afirmação de proxy confiável (item 2). Confirmado no original: o
   servidor **não** valida o token OIDC (nem assinatura, emissor, audiência, expiração ou claims); token
   OIDC cru recebe 401 em servidor protegido. A validação cabe ao gateway (ver [sso.md](sso.md)).

Regras de autorização:

- **Sem usuários no banco e sem identidade de proxy confiável**, as rotas `/admin/*` não distinguem
  operadores: qualquer chamador autorizado pelo nível de acesso atual as usa. Isso é aceitável só em
  instalação local.
- **Com o primeiro usuário no banco ou com identidade de proxy confiável**, todas as rotas `/admin/*`
  ficam **só para raiz**. Usuário de banco nunca tem papel administrativo. A checagem é feita a cada
  requisição, sem cache, então o primeiro usuário fecha o acesso de bootstrap sem reiniciar o servidor.
- Gestão de usuários é só para raiz. Ignorar a cadeia de admissão de webhooks também é só para raiz.

### 2.5 Credenciais armazenadas

- **Tokens de usuário do banco**: 32 bytes aleatórios (256 bits), codificados em base64 sem padding
  (43 caracteres), com prefixo `aim_`. Armazenamento: **SHA-256 de (token + ":" + pepper)**. A tabela
  guarda também uma prévia não autenticável (últimos 4 caracteres). Revogação marca data; rotação troca o hash.
- **Pepper**: 32 bytes aleatórios em hexadecimal, gerados no `init` e gravados no arquivo de configuração
  privado (`0600`). Ele é usado em todos os tokens de usuário do banco.
- **Token raiz, token de proxy, token de recuperação e senha inicial** ficam em texto no arquivo de
  configuração ou em variável de ambiente. Representações de depuração ocultam esses valores.
  Token de recuperação tem **mínimo de 32 caracteres**. Nenhum desses segredos pode colidir com uma
  credencial já existente (checagem pelo hash com o pepper).
- **Sessões web**: segredo de 32 bytes armazenado em **SHA-256 sem pepper**. O token CSRF também é guardado
  só em hash.
- **Senhas humanas**: Argon2id com sal aleatório de 16 bytes e parâmetros padrão da implementação
  (19 MiB de memória, 2 iterações, paralelismo 1). Tamanho entre **12 e 1024 bytes**. A senha não pode
  começar com prefixo de credencial nem ser igual ao nome de usuário.
- **Verificação de senha** roda fora da thread de escrita, com no máximo **2 operações simultâneas** e
  **32 na fila**. O excedente recebe **429**. Usuário inexistente ou desativado passa por uma verificação
  fictícia, para que o tempo de resposta não revele a existência da conta.
- **Limite de falhas de login**: 10 falhas por IP em janela de 60 s.
- **Senha temporária**: 24 bytes aleatórios (32 caracteres).
- **Credenciais de provedores de modelo** (chave ou OAuth) ficam num arquivo de credenciais com `0600`,
  **em texto**. Não há cifragem nem cofre de segredos.

### 2.6 Isolamento por projeto

- Linhas e arquivos são segmentados pela dupla **workspace + projeto**. Remover o projeto A não apaga
  arquivos nem linhas do projeto B.
- Consultas de entidade filtram pelo projeto. Gatilhos no banco rejeitam entidade ou vínculo entre
  workspaces ou projetos diferentes.
- O isolamento é de **escopo**, não de usuário. Usuários do banco não têm memória privada: não há ACL por
  usuário além da concessão por projeto.

### 2.7 Wiki como entrada não confiável

- O conteúdo da wiki é tratado como **dado importado e editável**. Links simbólicos e pontos de reparse
  do Windows na raiz da wiki, nos workspaces, nos projetos ou em qualquer descendente são **recusados**
  em leitura, escrita, indexação, recuperação e limpeza.
- A verificação é feita componente a componente, sem seguir o link. A escrita é atômica.
- Caminhos relativos precisam ser normalizados: sem componentes de subida, sem absolutos, sem vazios.
- Isso protege contra links **já existentes** quando a operação roda. Não protege contra um processo
  malicioso concorrente do mesmo usuário do sistema operacional.

### 2.8 Sanitização e redação de segredos

**Regra**: texto de usuário ou de agente é redigido **antes** de ser persistido, em todo ponto de entrada
que grava texto. Não há caminho de gravação que pule a redação.

**Padrões embutidos** (sempre ativos), por categoria:

- Tokens de autorização do tipo *bearer* com 16 ou mais caracteres.
- Chaves de API com prefixos conhecidos de provedores de modelo, de nuvem, de pagamentos, de
  repositórios de código e de mensageria. Inclui tokens de OAuth e tokens de bot.
- Blocos de chave privada em PEM.
- Credenciais embutidas em URL (`usuário:senha@host`).
- JWT (três segmentos) e blobs JSON em base64 com 40 ou mais caracteres, como tokens de túnel.
- Cabeçalhos HTTP de autenticação com valor opaco (chave, token, senha, autorização).
- Variáveis de ambiente de provedores conhecidos e atribuições genéricas
  `NOME_(KEY|TOKEN|SECRET|PASSWORD|CREDENTIAL|PRIVATE_KEY)=valor`. Também os sufixos em maiúsculas
  `_KEY_ID`, `_PASSPHRASE`, `_SIGNING_KEY`, `_PEPPER` e `_SALT`.
- Caminhos de credenciais: `.ssh`, `.aws`, `.kube`, `.config/gcloud` e `.gnupg`, com barra normal ou
  invertida e letra de unidade opcional.

**Mecânica**:

1. Antes da redação, removem-se sequências de escape de terminal, caracteres de controle (exceto tab,
   quebra de linha e retorno) e sobrescritas bidirecionais. Texto com NUL viraria arquivo binário.
2. Cada correspondência vira `[REDACTED:<tipo>]`. Padrões extras do operador (`[sanitize].extra_patterns`)
   usam o tipo `custom`.
3. A **allowlist** (`[sanitize].allowlist`) é checada **por correspondência**: um trecho que contém um item
   da lista fica intacto, mas os outros padrões continuam rodando.
4. A redação é **irreversível**. Não há como recuperar o texto original.
5. Títulos são redigidos **antes** do corte de 80 caracteres, para que um segredo não seja cortado pela
   metade e vazado.

**Limitação**: strings isoladas de alta entropia sem formato reconhecível (por exemplo, hexadecimal de 32
caracteres) **não** são redigidas. O operador que precisa disso deve adicionar padrão extra.

**Onde se aplica**:

- Ingestão de eventos de agente no servidor: redige corpo e título. Depois, limita o corpo durável a
  **16 KiB**, preservando início e fim, com marcador no meio.
- Escritas gerenciadas de páginas: corpo, título e textos do frontmatter.
- Texto que entra em prompts de consolidação, revisão, lint, perfil entre projetos e auto-melhoria.
- Pacotes de workstream: valores redigidos e truncados em **512 bytes**.
- Exportação de backup de agentes e mensagens de diagnóstico.
- Cliente de captura do assistente: padrões embutidos, antes do spool e do envio (ver 2.9).

**Onde não se aplica**:

- Texto enviado a embeddings: o envio é o conteúdo armazenado, sem nova redação (ver 2.12).
- Consulta do rerank por LLM: não é redigida.
- Arquivos da wiki editados à mão fora do programa: podem conter texto não redigido.
- Clientes auxiliares de captura fora do núcleo: usam conjunto **menor** de padrões e não redigem valores
  com menos de 6 caracteres.

### 2.9 Captura do assistente (Stop)

- **Desligada por padrão.** Exige **dupla adesão**: `capture_assistant` no servidor
  (`AI_MEMORY_CAPTURE_ASSISTANT=true`) e `install-hooks --capture-assistant` no cliente. O cliente só aceita
  essa opção para agentes nativos suportados (claude-code, codex e opencode2).
- Sem a adesão do servidor, o marcador é removido e o evento Stop fica **vazio**.
- Entrada máxima de **64 KiB**. Trecho final limitado a **2 000 bytes**. O servidor reaplica esse limite
  antes de gravar, sem confiar no cliente.
- **Redação em duas etapas**: o cliente redige com os padrões embutidos, antes do spool e do envio. O
  servidor redige de novo com `[sanitize]`, antes de gravar.
- Padrões extras do servidor **não rodam no cliente**. Um segredo que só eles capturam pode ficar no
  spool e no envio. A redação do cliente é irreversível, e a allowlist do servidor não a desfaz.
- O trecho entra no **handoff automático** da sessão, como "última resposta". A próxima sessão que assume
  o handoff recebe esse trecho como contexto de início. Ele **não** aparece na página de sessão versionada
  no git.
- O trecho entra em prompts de consolidação e revisão. Se houver provedor de chat em nuvem, ele é enviado
  a esse provedor.
- A opção é **global à instalação**: não há exclusão por projeto, porque o texto do assistente não tem
  atribuição de caminho. Para desligar em todo lugar, desligue a opção no servidor.
- O trecho pode citar código, segredos ou conteúdo de caminhos que o ai-memory não vê. O sanitizador é
  **melhor esforço**, não garantia.

### 2.10 Entidades e limites de texto

- Por página: no máximo **10 entidades**, cada uma com no máximo **64 caracteres**.
- Nomes são normalizados: minúsculas e espaços colapsados. Entradas com caractere de controle, vazias ou
  longas são **descartadas**, sem rejeitar a página. Duplicatas são removidas.
- Entidades do consolidador e do frontmatter escrito à mão passam pela mesma normalização.
- Consulta por entidade: até **12 termos**, com no mínimo **3 caracteres** cada. A correspondência é
  lexical e **não faz chamada externa**.
- Nome de entidade continua sendo dado não confiável quando aparece em explicações.
- Limite de tamanho de consulta livre e de parâmetros SQL: a confirmar.

### 2.11 Injeção de prompt por conteúdo armazenado

- Prompts de consolidação, lint e bootstrap marcam observações e páginas como **dado não confiável, não
  instrução**. Handoffs, pacotes de workstream e roteamento MCP seguem a mesma convenção. A cobertura
  completa de cada superfície é a confirmar.
- Sanitização e esquemas de saída estruturada são **defesa em profundidade, não prova**. Afirmações
  sensíveis devem ser verificadas contra as instruções atuais e contra o checkout.

### 2.12 Envio para fora: rerank e embeddings de provedor

- **Rerank por LLM** (opt-in, `AI_MEMORY_RERANKER=llm`): envia a consulta e títulos e trechos de páginas.
  Limites: consulta até **1 000 bytes** (não redigida), título até **200 bytes**, trecho até **600 bytes**,
  no máximo **30 candidatos**, no máximo **4 chamadas simultâneas** e timeout de **20 s**. A saída é JSON,
  e cada score precisa ser finito e estar entre 0 e 1. Falha, timeout ou saturação mantêm a ordem anterior.
- **Embeddings de provedor** (opt-in: `openai`, `openai-compat`, `voyage` ou `google`): o texto de cada página
  armazenada vai ao endpoint de embeddings. No caminho compatível com OpenAI, o texto é cortado a
  **8 000 bytes** antes do envio. Voyage e Google enviam sem esse corte local.
- O envio de embeddings **não passa por redação**. O conteúdo armazenado já foi redigido nas escritas
  gerenciadas, mas páginas editadas à mão não foram.
- Padrão: `local` (ver 5.1). `embedding_provider = none` desliga embeddings e mantém busca textual,
  entidades e grafo.

### 2.13 Integridade e varredura de segredos do projeto

- Wrapper de contêiner e instalador de hooks baixam artefatos de release. Ambos verificam o **SHA-256**
  publicado junto ao arquivo antes de usar.
- Ações de CI estão **fixadas em commit** revisado. Só o job de publicação de release tem permissão de escrita.
- Varredura de segredos com configuração versionada: roda em cada push e pull request (comparação
  incremental) e semanalmente no **histórico completo**, também sob demanda.
- O arquivo de exceções da varredura aceita **só impressões digitais exatas**, revisadas. Adicionar uma
  exceção **não substitui** remover o valor do código e rotacionar a credencial real.

## 3. Riscos em aberto

Ameaças listadas abaixo **não têm controle implementado**. Devem ser tratadas como pendências, não como
garantias.

- **Sem criptografia em repouso.** Banco, wiki e backups dependem das permissões do sistema operacional.
- **Pepper e token raiz no mesmo arquivo de configuração**, sem cofre. Quem lê esse arquivo obtém o token
  raiz e o pepper.
- **Token raiz configurado manualmente não tem tamanho mínimo validado.** Só o token de recuperação tem
  mínimo (32 bytes após aparar, sem prefixo reservado `ams_`/`aim_`/`amk_`). Confirmado no original.
- **Sem remoção de conta de usuário.** Foram encontradas as ações de desativar e de expirar, mas não uma
  rota de exclusão (confirmado no original: as rotas de usuário são listar, criar, alterar, redefinir
  senha, desativar, ativar e concessões). Isso enfraquece o atendimento a pedido de apagamento de dado pessoal.
- **Webhooks de admissão recebem a página inteira** (caminho, frontmatter e corpo). A URL não tem validação
  de esquema, então HTTP sem TLS é aceito. A falha é ignorada por padrão. Ver 5.2.
- **Embeddings de provedor não passam por redação** no envio (ver 2.12).
- **Consulta do rerank por LLM não é redigida.**
- **Páginas editadas à mão** não são redigidas ao serem lidas, indexadas ou enviadas.
- **Alta entropia sem formato** não é redigida (ver 2.8).
- **Padrões extras do servidor não protegem o cliente de captura** (ver 2.9).
- **Validação do JWT OIDC no servidor** não existe (confirmado no original); depende de gateway.
- **Ferramentas auxiliares fora do núcleo** (relay, importador, sincronização da wiki, cliente de captura
  externo) não foram revisadas nesta versão. A confirmar.
- **Permissões no Windows** dependem da ACL herdada. Não há ajuste explícito (confirmado no original).
- **Injeção de prompt** não tem prova. A convenção de dado não confiável é mitigação, não garantia.
- **Contêiner com publicação ampla** depende do operador. A verificação só avisa.

## 4. Fora de escopo (v1)

- Isolamento de inquilinos e ACL por usuário além das concessões de projeto. Usuários do banco são
  atribuição, não memória privada.
- Criptografia em repouso.
- Segurança do canal de sincronização remota da wiki. Se você envia o repositório git da wiki para um
  remoto, proteger esse canal é responsabilidade do operador (ver [complementary-contracts.md](complementary-contracts.md)).
- Prevenção perfeita de injeção de prompt semântica.
- Exposição direta à Internet hostil. Filas de hook, corpos, limites de taxa e concorrência são limitados,
  mas o serviço não foi projetado para isso. Mantenha-o atrás de controles normais de rede.

## 5. Tratamento de dados (revisão de privacidade)

### 5.1 Padrão local e sem telemetria

- Não há análises de uso, relatório de falhas nem telemetria. O programa não tem dependência de telemetria.
- Páginas da wiki e banco de dados ficam no diretório de dados do operador.
- O padrão de embeddings é **local em processo**: modelo all-MiniLM-L6-v2, 384 dimensões, sem chave de API e
  sem envio de conteúdo ([local-embeddings.md](local-embeddings.md)).
- **Ressalva**: na primeira inicialização, o modelo (cerca de 87 MB) é baixado **uma vez** de um repositório
  público de modelos. Os arquivos têm SHA-256 fixado, verificado antes do uso. Em ambiente sem internet,
  instale o modelo antes ([airgapped-install.md](airgapped-install.md)).
- Consolidação, lint, auto-melhoria e perfil entre projetos **exigem provedor de chat configurado**. Sem
  provedor (padrão), nada disso roda e não há envio.

### 5.2 Caminhos que enviam dados para fora

Todos são **desligados por padrão** e exigem configuração explícita.

| Caminho | O que sai do equipamento | Como ativar |
|---|---|---|
| Embeddings de provedor (`openai`, `openai-compat`, `voyage`, `google`) | texto de cada página armazenada | `embedding_provider` |
| Captura do assistente | trecho final do assistente, redigido duas vezes; com provedor de chat em nuvem, também vai a ele | dupla adesão |
| Rerank por LLM | consulta integral (não redigida), títulos e trechos limitados | `AI_MEMORY_RERANKER=llm` |
| Provedor de chat | observações e páginas para consolidação, lint, auto-melhoria e perfil | `llm_provider` |
| Webhooks de admissão | caminho, frontmatter e corpo da página; em operações de remoção, o caminho | `[[admission_webhooks]]` ou variável JSON |
| Servidor de destino dos hooks | eventos de agente (prompts, saídas de ferramenta) | só sai se o servidor configurado for remoto |

Notas:

1. **Embeddings de provedor** são a via mais ampla. Trocar de `local` para a nuvem **reenvia o corpus
   existente**: há a rota de backfill manual (`POST /admin/embed`, ou o comando `embed --force`) e o backfill
   na inicialização descrito no modelo de configuração. A confirmar o gatilho exato após a troca. Para
   conteúdo que não pode sair, mantenha `local` ou use um endpoint interno compatível com OpenAI.
2. **Captura e rerank** passam pelo sanitizador, que é melhor esforço.
3. **Webhooks de admissão** são serviços externos ao ai-memory. Quem opera o webhook é responsável pelo
   destino e pelo que ele faz com a página. Limites: no máximo **16 webhooks** por cadeia; timeout padrão de
   **2 000 ms**, máximo de **30 000 ms**; resposta lida até **1 MiB**; no máximo **256** disparos assíncronos
   simultâneos, e o excedente é descartado com log de erro. Política de falha padrão: **ignorar** (a escrita
   segue). A política "rejeitar" aborta a escrita e fica a cargo do operador.
4. Sem provedor de chat configurado, as funções de consolidação não chamam LLM.

### 5.3 Dado pessoal (LGPD/GDPR)

- O sistema armazena artefatos de desenvolvimento derivados de sessões de agentes de código. Não foi
  desenhado para tratar dado pessoal como finalidade.
- **Mas a tabela de contas guarda dados de identificação**: nome de usuário, nome de exibição (opcional),
  e-mail (opcional), data de criação e data do último acesso. Quando a conta é de uma pessoa, esses dados são
  pessoais.
- Há um **perfil entre projetos** com preferências e declarações do desenvolvedor, derivadas das sessões
  ([cross-project-profile.md](cross-project-profile.md)).
- Dado pessoal incidental em sessões (um nome numa mensagem de commit, texto digitado por um desenvolvedor)
  é armazenado e retido como qualquer outro conteúdo.
- Não há campos de consentimento. Não há ferramenta de requisição de titular para exclusão de conta (ver
  riscos, seção 3).
- A organização é a controladora. Ela decide retenção, acesso e divulgação, na infraestrutura que opera.
  *Isto não é aconselhamento jurídico; confirme com a sua área de proteção de dados.*

### 5.4 Operador e processador

- Auto-hospedado e sem vias de saída ativas: não há terceiro no caminho dos dados, e o ai-memory não cria
  relação de operador.
- Com provedor de embeddings, de chat, de rerank ou webhook externo, **esse provedor ou serviço** passa a ser
  seu processador, sob o contrato que você tem com ele. O ai-memory é apenas um conduíte.

### 5.5 Retenção e exclusão

Os dados persistem até serem removidos, exceto por dois mecanismos **ligados por padrão**:

- **Decaimento de páginas episódicas frias.** A varredura diária (intervalo padrão de 86 400 s) considera
  só páginas **episódicas** e **não fixadas**. Semânticas, procedurais e de trabalho ficam isentas, e as
  observações cruas também (retenção padrão 0, que significa nunca podar). Uma página com pontuação abaixo de
  **0,20** tem o arquivo Markdown removido e vira marca de remoção. Marcas com mais de **180 dias** são apagadas
  definitivamente.
  - A pontuação é `saliência × e^(−0,02 × dias desde a última atualização)` mais um termo de acesso. O termo
    de acesso cresce com o número de acessos e decai com o tempo desde o último acesso.
  - **Piso de sobrevivência**: uma página com saliência padrão (1,0) e sem acessos fica acima do limiar por
    cerca de **80 dias**. Com saliência mínima (0,25, após feedback negativo), cai abaixo do limiar em cerca de
    **11 dias**. Reescrever a página reinicia a contagem.
- **Páginas com TTL.** Uma página com `expires_at` fica **oculta na leitura** ao expirar e é apagada (arquivo e
  linhas) na varredura seguinte. **TTL vence `pinned`**.
- **Exclusão** é em nível de sistema de arquivos: remover o diretório de dados remove tudo.
- **Exclusão com escopo**: purga por projeto e por sessão, além da remoção de workspace (com confirmação e
  força). A confirmar a rota exata do workspace. Essas operações são isoladas: não tocam outros projetos.
  Ver [database-schema.md](database-schema.md) e [api-reference.md](api-reference.md).

### 5.6 Exposição de rede

Escolha operacional separada dos caminhos de saída: controla **quem alcança a API**, não o que o ai-memory
envia. Antes de expor, leia [https-via-proxy.md](https-via-proxy.md). Para integração de identidade, veja
[sso.md](sso.md). Para ambientes sem internet, veja [airgapped-install.md](airgapped-install.md).

## 6. Limites de referência

| Limite | Valor | Onde se aplica |
|---|---|---|
| Corpo HTTP geral | 10 MiB | todas as rotas, exceto bootstrap |
| Corpo da rota de bootstrap | 32 MiB | `/admin/bootstrap` |
| Corpo durável de observação | 16 KiB (depois da redação) | ingestão de eventos |
| Título exibido | 80 caracteres (depois da redação) | observações |
| Entrada de captura do assistente | 64 KiB | cliente |
| Trecho de captura do assistente | 2 000 bytes | cliente e servidor |
| Valores de pacote de workstream | 512 bytes | pacotes |
| Entidades por página | 10 | páginas |
| Tamanho de nome de entidade | 64 caracteres | páginas e consultas |
| Termos de consulta por entidade | 12 (mínimo 3 caracteres cada) | consultas |
| Escopos por consulta | 25 | consultas multiescopo |
| Rerank: candidatos | 30 | consulta |
| Rerank: chamadas simultâneas | 4 | consulta |
| Rerank: timeout | 20 s | consulta |
| Rerank: consulta, título e trecho | 1 000, 200 e 600 bytes | prompt de rerank |
| Embedding compatível com OpenAI | 8 000 bytes de texto | envio ao provedor |
| Dimensões do modelo local | 384 | embeddings locais |
| Timeout de conexão no download do modelo | 30 s | primeira inicialização |
| Timeout de requisição a provedor de LLM | 300 s | consolidação e chamadas |
| Webhooks de admissão por cadeia | 16 | cadeia de admissão |
| Timeout de webhook | 2 000 ms padrão, 30 000 ms máximo | cadeia de admissão |
| Resposta de webhook | 1 MiB | cadeia de admissão |
| Disparos assíncronos de webhook simultâneos | 256 | cadeia de admissão |
| Operações de senha simultâneas / na fila | 2 / 32 (excedente: 429) | login e troca de senha |
| Falhas de login por IP | 10 em 60 s | login humano |
| Duração da sessão humana | 7 dias | sessões web |
| Tamanho de senha humana | 12 a 1 024 bytes | contas humanas |
| Token de recuperação | mínimo 32 caracteres | configuração |
| Token de usuário do banco | 256 bits, prefixo `aim_` | credenciais |
| Varredura de decaimento | 86 400 s (padrão) | manutenção |
| Limiar de decaimento | 0,20 | páginas episódicas |
| Retenção de marcas de remoção | 180 dias | páginas episódicas |

## 7. Checklist para a sua implementação

1. Padrão **loopback com falha fechada** para bind externo sem token.
2. **Allowlist de Host** com loopback IPv4, IPv6 e o nome local, e **limite de corpo** (10 MiB, com exceção
   explícita para bootstrap).
3. Permissões `0700` e `0600` na criação, sem alterar instalações existentes. Equivalente em ACL no Windows.
4. Recusar links simbólicos e pontos de reparse ao tocar a wiki. Escrita atômica.
5. **Redigir antes de persistir**, com construção que impede gravar texto bruto, no cliente e no servidor.
6. Tratar todo texto recuperado como **dado não confiável**. Cercar e rotular nos prompts e nos contextos injetados.
7. **Opt-in explícito e documentado** para toda saída de dados. Dupla adesão para captura do assistente.
8. Armazenar **só o hash** de tokens e sessões, com comparação em tempo constante. Senhas com KDF de memória alta.
9. Rotas `/admin/*` só para raiz quando houver usuários. Token de usuário nunca contorna a admissão.
10. Teste adversarial para cada fronteira (ver [security-boundaries.md](security-boundaries.md)).
11. Registrar os riscos da seção 3 como **pendências**, não como controles implementados.
