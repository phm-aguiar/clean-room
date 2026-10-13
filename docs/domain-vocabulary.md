# Vocabulário do domínio

> **Papel:** fecha o vocabulário do sistema inteiro: identificadores tipados, tipos de
> domínio (página, observação, handoff, mensagem, usuário, fluxo de trabalho gerenciado),
> identidade e autorização do chamador, a sanitização de segredos e utilitários puros de
> conteúdo. Esta camada **não faz I/O**: todas as regras são funções puras sobre valores,
> testáveis isoladamente. As demais partes do sistema dependem dela; ela não depende de
> nenhuma outra.
>
> Este documento descreve o **comportamento** esperado. Quem reimplementar o sistema deve
> seguir os contratos aqui descritos, não a forma como o código original os expressa.

---

## 1. Áreas do domínio

| Área | Responsabilidade | Detalhes |
|---|---|---|
| Constantes globais e erros | Nomes reservados, categorias de erro | §2, §3 |
| Identificadores e caminho de página | IDs tipados (UUIDv7), caminho relativo da página, tipo de agente | §4 |
| Observações e sessões | Eventos capturados do ciclo de vida do agente | §5 |
| Páginas | Versões, evidência, entidades, TTL, links, relações tipadas, feedback | §6 |
| Sanitização | Remoção de segredos e limites de tamanho antes de gravar | §7 |
| Identidade e autorização | Ator do pedido, níveis de acesso, dono de registros | §8 |
| Ponteiro de projeto ativo | Qual projeto as ferramentas leem por padrão | §9 |
| Handoff | Continuidade entre sessões de agentes | §10 |
| Mensagens entre projetos | Caixa de entrada e saída entre projetos | §11 |
| Usuários | Credenciais, papéis, política de senha | §12 |
| Slots | Páginas pequenas injetadas no início de cada sessão | §13 |
| OKF | Conformidade com o formato de conceitos OKF v0.2 | §14 |
| Fluxo de trabalho gerenciado | Histórico portátil entre harnesses | §15 |
| Utilitários | Métricas de ingestão, detector de andaime, snippet de roteamento | §16 |

Detalhes de cada área estão também em [complementary-contracts.md](complementary-contracts.md)
(identidade de repositório, ledger de log, manifesto de backup de ativos de agentes,
perfil entre projetos), [agent-messaging.md](agent-messaging.md),
[okf.md](okf.md), [managed-workstreams.md](managed-workstreams.md),
[prompts-schemas-and-sanitizer.md](prompts-schemas-and-sanitizer.md) e
[marker-file.md](marker-file.md).

---

## 2. Constantes globais

| Constante | Valor | Uso |
|---|---|---|
| Workspace padrão | `default` | Único workspace do fluxo v1. |
| Projeto de fallback | `scratch` | Usado só quando não há diretório de trabalho nem projeto. |
| Projeto de escopo global | `_global` | Projeto reservado para preferências do usuário ou do time (contexto permanente que todo projeto herda). Escrito só por pedido explícito com escopo `global`. A captura automática de eventos nunca é atribuída a ele: quando um marcador ou diretório aponta para `_global`, a captura cai no projeto padrão do servidor. |

O prefixo `_` segue a convenção de nomes reservados da wiki (como `_pending/` e `_lint/`).

---

## 3. Erros

Categorias de erro do domínio:

- **Caminho fora da raiz de dados**: defesa em profundidade contra escape de caminho.
- **Caminho de página inválido**.
- **Registro malformado**: dado persistido que não pode ser lido.
- **Usuário inválido**, **e-mail inválido**, **senha inválida**.
- **Erro de I/O** (transparente) e **erro de serialização** (convertido em texto).

Regras:

- Todo conversor de texto para um conjunto fechado de valores do domínio devolve
  "registro malformado" para valor desconhecido. A exceção é o conversor de tipo de agente,
  que é tolerante (§4.3).
- A mensagem de senha inválida é sempre genérica e nunca revela qual regra falhou nem o
  valor informado.

---

## 4. Identificadores e caminho de página

### 4.1 Identificadores UUID

Todo identificador é um UUID do tipo **v7** (ordenado por tempo), gerado na criação. Cada
tipo de identificador é distinto: um identificador de projeto não pode ser passado no lugar
de um de workspace. Tipos:

`WorkspaceId`, `ProjectId`, `SessionId`, `ObservationId`, `PageId`, `EntityId`,
`HandoffId`, `MessageId`, `WorkstreamId`, `ManagedRunId`, `UserId`, `ApiCredentialId`,
`AutoImproveRunId`, `AutoImproveProposalId`, `PageFeedbackId`.

Comportamento comum:

- Representação textual padrão do UUID; leitura por texto falha com "registro malformado"
  se não for UUID válido.
- Leitura de bytes exige exatamente 16 bytes (senão, registro malformado).
- Serialização transparente: o identificador aparece na API como o próprio texto do UUID.
- Todo identificador é tratado como texto em JSON e em URL (`uuid` como string).

Casos especiais:

- **`PageId` identifica uma versão da página**, não o caminho. Cada atualização gera um novo
  `PageId` (§6.3).
- **`SessionId` vindo de um harness**: se o identificador nativo for um UUID, é usado como
  está (em forma canônica). Qualquer outra string (Codex, OpenCode e similares) é convertida
  de forma **determinística** em UUID v5 a partir de um namespace fixo. Assim, os POSTs dos
  hooks e as buscas posteriores por identificador nativo usam a mesma chave. Qualquer
  ferramenta offline que cruze transcrições por identificador nativo deve aplicar a mesma regra.

### 4.2 Caminho da página (`PagePath`)

Caminho relativo da página dentro da árvore da wiki. A validação ocorre na construção, então
o armazenamento pode tratar o caminho como chave plana.

Regras obrigatórias:

- não vazio;
- não começa com `/`;
- sem prefixo de unidade de Windows (`C:/…`);
- sem `\` como separador;
- nenhum segmento vazio, `.` ou `..`.

Validação **separada e mais estrita**, `ensure_portable`: garante que o arquivo possa ser
materializado em qualquer sistema operacional (nomes reservados do Windows, por exemplo).
Ela é aplicada **somente quando um caminho novo entra no sistema**, não na leitura de linhas
já persistidas. Motivo: apertar a validação de leitura tornaria ilegível uma listagem inteira
por causa de uma única página antiga. A regra é a mesma em todas as plataformas, de propósito:
uma wiki criada em Linux precisa ser utilizável no Windows com a mesma versão.

**Chave de colisão de caminhos** (`portable_page_key`): dois caminhos com a mesma chave são o
mesmo arquivo em sistemas que dobram caixa ou normalizam Unicode (macOS, Windows). A chave é
obtida em duas etapas, nesta ordem: 1) minúsculas; 2) normalização Unicode NFC. A ordem
importa: a letra `İ` minúscula vira `i` seguido de U+0307, que só se recompõe depois da
conversão para minúsculas.

### 4.3 Tipo de agente (`AgentKind`)

Discriminador do agente de código que capturou uma observação ou handoff. Valores (texto no
fio):

| Valor no fio | Valor no fio | Valor no fio |
|---|---|---|
| `claude-code` | `open-code` | `kimi-code` |
| `codex` | `cursor` | `kiro-cli` |
| `gemini-cli` | `claude-desktop` | `command-code` |
| `openclaw` | `antigravity-cli` | `hermes` |
| `omp` | `pi` | `pool` |
| `crush` | `grok` | `zcode` |
| `zero` | `devin` | `copilot-cli` |
| `grizzybot` | `other` | |

Total: **23 valores**, incluindo `other`.

- **Conversão tolerante** a partir de texto livre: aceita aliases (por exemplo `claude`,
  `claude_code`, `agy`, `kimi`, `opencode`, `poolside`, `cmd`, `hermes-agent`,
  `copilot_cli`). **Valor desconhecido vira `other`**, para que a ingestão nunca falhe por
  causa de um agente novo ou de um erro de digitação.
- **Predicados de injeção**: definem se um agente consome a saída dos hooks como contexto.
  Eles existem porque a busca de handoff é **destrutiva** (marca o handoff como aceito).
  Buscar o handoff quando a saída será descartada perderia o handoff silenciosamente.

| Predicado | Verdadeiro para | Consequência |
|---|---|---|
| Injeta handoff no início de sessão | Todos, **exceto** `crush`, `grok`, `zero`, `kimi-code`, `hermes`, `pool`, `other` | Os demais buscam o handoff no hook de início. |
| Injeta handoff no envio de prompt | Só `kimi-code` | Entrega no evento de envio do prompt. |
| Injeta handoff após uso de ferramenta | Só `grok` | Aceita o handoff no primeiro evento de ferramenta da sessão. |
| Reusa o identificador de sessão após o fim | Só `grok` | Um reinício da mesma conversa reabre a sessão. Para os demais, sessão encerrada é definitiva. |

Para agentes sem injeção, o handoff continua disponível sob demanda pela ferramenta MCP
de aceite de handoff.

---

## 5. Observações e sessões

### 5.1 Tipo de observação (`ObservationKind`)

Eventos do ciclo de vida, em forma canônica de texto (kebab-case):
`session-start`, `user-prompt`, `pre-tool-use`, `post-tool-use`, `pre-compact`,
`post-compaction`, `notification`, `stop`, `session-end`, `other`.

A conversão aceita também a forma snake_case e a forma PascalCase dos mesmos eventos
(`PostToolUse`, `session_start`, `UserPromptSubmit` para `user-prompt`). Valor desconhecido
gera registro malformado.

### 5.2 Entrada de observação

| Campo | Regra |
|---|---|
| `session_id`, `workspace_id`, `project_id` | Escopo do registro. |
| `kind` | Um dos valores de §5.1. |
| `extension`, `source_event` | Preservam vocabulário de terceiros **sem** expandir o conjunto de tipos. Ausentes quando o evento não declara namespace. |
| `title`, `body` | Já sanitizados (§7). |
| `importance` | Inteiro de 1 a 10. Padrão do contrato: **5**. A ingestão de hooks atribui valores por tipo de evento (sessão 7 e fim de sessão 7, prompt 8, uso de ferramenta 5, parada e compactação 6, notificação e demais 3). Ver [lifecycle-event-ingestion.md](lifecycle-event-ingestion.md). |
| `occurred_at` | Instante original em microssegundos, quando conhecido (por exemplo, em reprocessamento de transcrição). Ausente significa "agora": o armazenamento preenche no momento da gravação. |

A observação materializada acrescenta o identificador e o instante de criação.

### 5.3 Sessão

Registro de início de sessão, com identificador **fornecido pelo chamador e idempotente**
(repetir o mesmo identificador não cria outra sessão). Campos: identificadores de escopo,
tipo de agente, diretório de trabalho no início (opcional), **usuário dono** (opcional) e
instante original (opcional).

- Dono ausente (`None`) significa **sessão compartilhada**. Esse é o comportamento de uma
  instalação de um só operador.
- O dono é a chave de armazenamento da identidade do ator (§8.4), quando o ator está
  autenticado e a implantação distingue operadores.

---

## 6. Páginas

Uma **página** é a unidade de memória de longo prazo: um arquivo markdown da wiki do projeto.

### 6.1 Níveis (`Tier`)

Quatro níveis, em texto minúsculo: `working`, `episodic`, `semantic`, `procedural`.

- `working`: sessão atual (últimas observações, arquivos em uso).
- `episodic`: resumos por sessão, com etiquetas de conceito e arquivos tocados.
- `semantic`: fatos, preferências e notas de arquitetura destiladas. São as páginas da wiki propriamente ditas.
- `procedural`: padrões repetidos extraídos de agrupamentos episódicos.

O nível define a **política de retenção** (ver [storage-and-retrieval.md](storage-and-retrieval.md)).
Na saída estruturada do consolidador, o conjunto de valores é **restrito** ao esquema; sem
essa restrição, o modelo inventava valores numéricos.

### 6.2 Entrada de escrita de página

| Campo | Regra |
|---|---|
| `workspace_id`, `project_id`, `path` | Escopo e caminho (§4.2). |
| `title` | Título (do frontmatter ou do primeiro H1). |
| `body` | Markdown sem o frontmatter. |
| `tier` | Um dos níveis de §6.1. |
| `frontmatter` | Mantido como JSON para consulta barata. |
| `pinned` | Página fixada pelo usuário: nunca decai. |
| `links` | Links de saída encontrados no corpo (§6.8). |
| `author_id` | Atribuição (quem escreveu). **Nunca é filtro de leitura.** Ausente para acesso anônimo e para o token raiz. |
| `expires_at` | TTL vindo do frontmatter (§6.7). Ausente = nunca expira. |
| `entities` | Substantivos relevantes, normalizados (§6.5). Vazio para páginas escritas à mão. |
| `evidence` | Fontes que sustentam a escrita (§6.4). |

### 6.3 Página materializada e versionamento

Campos: identificador da **versão**, escopo, caminho, título, corpo, **SHA-256 do corpo**,
nível, frontmatter, `is_latest`, `supersedes`, `pinned`, instantes de criação e atualização
da versão, e TTL.

**Versionamento por substituição**: uma atualização cria uma nova linha com `is_latest = true`
e `supersedes` apontando para a versão anterior. A versão anterior fica com `is_latest = false`.
Leituras padrão veem só a versão mais recente.

### 6.4 Evidência

Fontes que produziram ou reafirmaram uma versão de página. Tipos fechados:

- `session`: sessão consolidada que produziu ou tocou a página;
- `observation`: observação citada diretamente;
- `feedback`: feedback de usuário ou agente que reafirmou a página;
- `reconsolidation`: passagem de reconsolidação que reescreveu ou reconfirmou a página.

Cada item é gravado **na mesma transação** da escrita da página. Citar a mesma fonte duas vezes
não gera duplicata. Cada item tem um identificador de fonte opaco, único dentro do tipo para
aquela página.

**Nesta versão, a evidência é apenas registrada e exposta** (contagem de evidências na
explicação de busca). Ela **não entra no ranqueamento**. (Ver §6.9 sobre o que de fato
afeta relevância.)

### 6.5 Entidades

- Tamanho máximo de uma entidade: **64 caracteres**. Valor maior é rejeitado, não truncado.
- Máximo de **10 entidades por página**.

Normalização de uma entidade:

1. remove espaços no início e no fim;
2. colapsa sequências de espaços internos em um só;
3. converte para minúsculas;
4. rejeita o resultado se for vazio, se contiver caractere de controle ou se passar de 64
   caracteres.

O processamento para assim que o limite é ultrapassado: uma entrada hostil enorme não causa
alocação proporcional. A conversão para minúsculas torna o casamento de entidades **lexical**,
sem nova chamada a modelo: `Postgres`, `postgres` e `POSTGRES` são a mesma entidade.

Normalização de uma lista: descarta entradas inválidas, remove duplicatas **preservando a
ordem** e corta em 10.

Entidades derivadas do frontmatter: une a lista explícita `entities` (vem **primeiro**, para
ganhar o limite) com a lista `tags`. É tolerante: campo ausente ou que não é lista não
contribui com nada e nunca falha. A validação estrita fica na camada de escrita da wiki.

### 6.6 Metadados públicos de escrita

Campos aceitos pelas superfícies públicas de escrita de página:

| Campo | Limite |
|---|---|
| `kind` | Até 64 caracteres, sem caractere de controle; aparado. Vazio = ausente. |
| `entities` | Até 10 nomes, cada um até 64 caracteres brutos. A validação ocorre **antes** da normalização. |
| `abstract` (resumo L0) | Até 1024 caracteres brutos; aparado. Vazio = ausente. |
| `relations` | Até **32** alvos no total, cada um até 1024 caracteres, sem caractere de controle. |

Regras adicionais:

- É **substituição completa**, não correção parcial: campos omitidos não herdam o frontmatter anterior.
- Escopo, atribuição e controles de admissão **não podem** ser informados por este caminho.
- Alvo de relação aceita as formas `caminho`, `projeto:caminho` e `workspace/projeto:caminho`.
  Componentes de escopo não podem ter espaço nas bordas nem estar vazios; `.` e `..` são proibidos.
- Se o último segmento do caminho não tem extensão, `.md` é acrescentado. Um caminho com outra
  extensão é rejeitado. O caminho final precisa passar em `ensure_portable`.

### 6.7 TTL (`expires_at`)

Aceita:

- instante RFC 3339 (com fuso), ou
- data `YYYY-MM-DD`, que significa **fim desse dia em UTC** (23:59:59,999999).

Valor que não é nenhum dos dois não é um TTL válido. A validação de TTL da wiki e a derivação
de `stale_after` do formato OKF usam **a mesma função**, então não podem divergir sobre quando
uma página expira.

Comportamento: páginas expiradas somem das leituras (busca, recentes e briefing). **A remoção
física pela varredura de retenção está a confirmar** (ver evidência).

### 6.8 Links e relações tipadas

`LinkTarget`: escopo opcional de workspace, escopo opcional de projeto, caminho e relação opcional.

| Forma no corpo | Significado |
|---|---|
| `[[caminho]]` | Local: resolve no mesmo projeto da página de origem. |
| `[[projeto:caminho]]` | Projeto irmão, no mesmo workspace. |
| `[[workspace/projeto:caminho]]` | Projeto de outro workspace. |

Links cruzados tornam explícitas as dependências entre projetos e fazem as wikis de cada
projeto formar **um grafo**. Links que ainda não têm destino são mantidos como **links
pendentes** (sem destino resolvido) e resolvidos quando a página alvo existir.

**Relações** (vocabulário **fechado**, declaradas na chave `relations:` do frontmatter):

- `causes`: a página de origem descreve uma causa do alvo;
- `fixes`: a página de origem corrige o problema descrito no alvo;
- `contradicts`: a página de origem contradiz o alvo (o relatório de lint mostra essas).

Vocabulário fechado de propósito: relação em texto livre vira um conjunto de etiquetas que
não pode ser consultado. Nome de relação desconhecido não é erro: o link fica como referência
simples, sem relação.

### 6.9 Feedback (`FeedbackKind`)

Quatro sinais de utilidade de uma página recuperada, via ferramenta de feedback:

| Sinal | Efeito na relevância da página | Vai ao relatório de lint |
|---|---|---|
| `helpful` | Sobe a relevância em passo limitado (até um teto). | Não |
| `not_helpful` | Desce a relevância em passo limitado (até um piso). | Não |
| `stale` | Leva a relevância direto ao **piso**. | **Sim** |
| `wrong` | Leva a relevância direto ao **piso**. | **Sim** |

Regra central: feedback **nunca apaga** nada. A decisão de remoção fica com a varredura de
retenção, não com o agente. Um piso alto o bastante mantém uma página usada fora da zona de
remoção.

---

## 7. Sanitização de segredos

Esta é a fronteira de privacidade do sistema: **nada chega ao armazenamento sem passar por
aqui**.

### 7.1 Pipeline

Sobre um texto de entrada, na ordem:

1. **Remoção de controle.** Remove sequências de escape de terminal (controle de cursor, cor,
   título de janela), caracteres de controle C0 e C1 **exceto** tabulação, quebra de linha e
   retorno de carro, DEL, e marcadores de direção bidirecional (que fazem o texto parecer outra
   coisa). Esta etapa roda **antes** das demais: um escape dentro de um segredo o tiraria do
   alcance dos padrões. Motivo adicional: um byte nulo tornaria o markdown binário.
2. **Padrões, em ordem** (mais específicos primeiro). Cada trecho casado vira
   `[REDACTED:<rótulo>]`, **exceto** quando o trecho contém uma entrada da lista de
   permissões (checada por trecho casado, não pelo texto inteiro). Um padrão continua rodando
   mesmo quando há exceção; só o trecho permitido sobrevive.

Filosofia: **falso positivo é aceitável; vazamento não.**

### 7.2 Padrões embutidos

| Rótulo | O que captura |
|---|---|
| `bearer_token` | Cabeçalho `Bearer` seguido de token de 16 ou mais caracteres. |
| `api_key` | Chaves com prefixo `sk-` (16 ou mais caracteres). |
| `stripe_key` | Chaves Stripe `sk_live_` e `rk_live_` (secreta e restrita). |
| `github_token` | `ghp_`, `gho_`, `ghu_`, `ghs_`, `ghr_` e `github_pat_`. |
| `aws_key` | Identificadores de chave AWS `AKIA` (longa) e `ASIA` (temporária), com exatamente 20 caracteres e borda de palavra. A âncora exata evita apagar palavras comuns como `ASIAPACIFICREGION`. |
| `google_api_key` | Chaves `AIza…` (30 ou mais caracteres) e chaves de autorização `AQ.Ab…` (30 ou mais caracteres após o prefixo). |
| `google_oauth` | Tokens de atualização OAuth `1//…` (20 ou mais caracteres). |
| `meta_token` | Tokens de acesso Graph `EAA…`. |
| `telegram_token` | `<id de 6 a 10 dígitos>:<segredo>`, em duas formas: a que começa com `AA` e a de 34 a 35 caracteres. |
| `ghl_token` | Tokens `pit-` seguidos de UUID. |
| `slack_token` | `xoxb-`, `xoxa-`, `xoxp-`, `xoxr-`, `xoxs-` e `xapp-`. |
| `jwt` | Três segmentos base64url separados por ponto. |
| `base64_json_token` | Blob base64 sem ponto começando em `eyJ`, com 40 ou mais caracteres (inclui tokens de túnel). |
| `private_key` | Bloco PEM de chave privada, multilinha. |
| `url_credentials` | `esquema://usuário:senha@host`. |
| `auth_header` | Cabeçalhos de autenticação com valor opaco (por exemplo `X-Api-Key`, `Authorization`, `Private-Token`, `Ocp-Apim-Subscription-Key`, `X-Amz-Security-Token`). Um sufixo `key` ou `token` sozinho **não** basta: precisa de palavra de autenticação antes (por exemplo `Idempotency-Key` não é redigido). |
| `env_secret` | Três formas: (a) atribuições de variáveis conhecidas (`OPENAI_API_KEY`, `GITHUB_TOKEN`, `HF_TOKEN`, `AWS_SECRET_ACCESS_KEY` e outras); (b) qualquer `NOME_EM_MAIUSCULAS` terminado em `_KEY`, `_TOKEN`, `_SECRET`, `_PASSWORD`, `_PASSWD`, `_CREDENTIAL(S)` ou `_PRIVATE_KEY`, com `=` ou `:`; (c) sufixos vizinhos `_KEY_ID`, `_PASSPHRASE`, `_SIGNING_KEY`, `_PEPPER` e `_SALT`, só em maiúsculas. |
| `credential_path` | Caminhos com `.ssh`, `.aws`, `.kube`, `.config/gcloud` e `.gnupg`, em separador `/` ou `\`, com ou sem letra de unidade e sem distinção de caixa. |
| `custom` | Padrões definidos pelo operador (§7.3). |

**Não capturado de propósito:** strings de alta entropia soltas (por exemplo, hexadecimal
aleatório de 32 caracteres). Sem saber a estrutura, não dá para redigir sem falsos positivos
demais. Quem precisa desse nível de cuidado deve adicionar um padrão próprio.

### 7.3 Configuração

Seção `[sanitize]` do arquivo de configuração:

- `extra_patterns`: expressões adicionais a redigir (rótulo `custom`).
- `allowlist`: trechos que **nunca** devem ser redigidos, mesmo que um padrão os case. Útil
  quando um nome de projeto colide com a regra genérica de variável de ambiente.

Expressão inválida **aborta a inicialização** com erro claro. O sanitizador é construído uma
vez e pode ser compartilhado livremente (copiar é barato).

### 7.4 Limites de tamanho e o tipo-fronteira

O único caminho para criar uma observação sanitizada é a função de construção, que:

1. sanitiza o **título**, depois aplica o teto de título;
2. sanitiza o **corpo**, depois aplica o teto de corpo.

**Sanitiza antes de truncar.** Truncar primeiro poderia cortar um segredo ao meio, deixando um
fragmento curto demais para casar com um padrão, que então iria para o armazenamento sem
redação. Como a redação ocorre antes do corte, um marcador `[REDACTED:…]` já está no lugar
quando o limite é aplicado.

**Teto de título**: **80 caracteres exibidos**, com reticências ao final quando cortado.

**Teto de corpo**: **16 KiB** (16 × 1024 bytes), com truncamento de **cabeça e cauda**:

- a cabeça recebe metade do orçamento útil e a cauda a outra metade;
- o orçamento útil é o teto menos **48 bytes** reservados para o marcador;
- no meio entra um marcador da forma `\n...[truncated N bytes]...\n`, onde N é o número de bytes omitidos;
- se o teto for menor que 64 bytes, o corte é só de cabeça;
- nunca corta no meio de um caractere multibyte.

Truncamento simples de texto (usado em outros pontos): corta em fronteira de caractere e
reserva o espaço das reticências **dentro** do teto em bytes.

O armazenamento só aceita observações sanitizadas por essa construção. Assim, **não existe
caminho do texto bruto até o disco**.

---

## 8. Identidade e autorização

### 8.1 Contexto do ator

Campos opcionais: agente, usuário, nome, e-mail, identificador de sessão, emissor (OIDC),
sujeito (OIDC), cliente. Ausência de todos (padrão) significa **anônimo**.

O contexto é injetado **uma vez por pedido**, pela camada de autenticação, e propagado até a
escrita. Assim a atribuição cai **na mesma transação** que o dado. Só identidade resolvida pela
autenticação é confiável: cabeçalhos brutos de ator não são lidos pelas ferramentas.

### 8.2 Níveis de autenticação

Resolvidos fora do domínio; listados aqui para referência, do mais fraco ao mais forte:

0. **Anônimo**: sem autenticação configurada.
1. **Raiz**: o token definido em `AI_MEMORY_AUTH_TOKEN` confere com o token raiz da configuração.
2. **Usuário de banco**: o bearer confere com o hash de uma credencial de API cadastrada.
3. **Proxy de autenticação externo**: um intermediário confiável, com bearer próprio, afirma a
   identidade pelos cabeçalhos `X-Memory-Actor-*`: usuário, ou o par completo emissor + sujeito.

O modelo é de **atribuição, não de controle de acesso**: registra *quem* escreveu. Não decide
*se podia* escrever, salvo pelas capacidades de §8.3.

### 8.3 Nível e capacidades

Níveis: `anonymous`, `root`, `user`.

Capacidades e regra de autorização:

| Capacidade | Regra |
|---|---|
| Leitura normal e escrita normal | Sempre permitidas. |
| Pular a cadeia de admissão | Proibida para `user`. Permitida para `anonymous` e `root`. |
| Administração | Permitida se a implantação **não** distingue operadores (modo de um usuário). Caso contrário, exige `root`. |
| Gestão de usuários | Sempre exige `root`. |

Erro de autorização tem duas formas: **"autenticação necessária"**, que vira HTTP **401**, e
**"proibido"**, que vira HTTP **403**. Anônimo sem permissão recebe 401; usuário autenticado sem
permissão recebe 403.

**Distinção de operadores**: a implantação distingue operadores quando existe **pelo menos um
usuário cadastrado** no banco **ou** há um proxy confiável configurado. Enquanto nenhum dos dois
existir, o servidor opera como de um só operador.

Cabeçalho de pular cadeia de admissão (`X-Memory-Skip-Admission-Chain`): lista separada por
vírgulas; espaços nas bordas de cada item são aparados e itens vazios são descartados. É honrado **só** para quem tem a capacidade. Motivo:
o cabeçalho é controlado pelo cliente, e um usuário comum não pode furar um webhook de admissão
com política de rejeição.

### 8.4 Chave de identidade (`IdentityKey`)

Identidade **qualificada** do operador. É o único valor pelo qual a propriedade de um registro
é decidida. Duas formas:

- **Sujeito**: par emissor + sujeito (OIDC). Vence quando **ambos** estão presentes e não vazios
  depois de aparar espaços.
- **Usuário**: nome de usuário.

Regra de escolha (aparar espaços antes): se houver par emissor + sujeito completo, usa-o. Caso
contrário, usa o nome de usuário. **Um par incompleto (só emissor, ou só sujeito) é ignorado**
e a escolha cai no nome de usuário; não gera erro. Sem nenhum dos dois, não há identidade.

**Chave de armazenamento**:

- sujeito: `oidc:<tamanho em bytes do emissor>:<emissor><sujeito>`
- usuário: `user:<nome>`

O prefixo de tamanho torna a separação entre emissor e sujeito **não ambígua**. A operação
inversa reconstrói a identidade e a revalida.

**Segmento de caminho seguro** (usado em nomes de diretório da wiki, por exemplo em slots):

- usuário com até 64 bytes, não vazio, só `[a-z0-9._-]` em minúsculas e sem ponto final: `u-<nome>`;
- qualquer outro usuário: `uh-<hash>`, onde o hash é um UUID v5 da chave de armazenamento;
- sujeito: `o-<hash>`, pelo mesmo critério.

Qualificar a identidade torna **impossível** que um nome de usuário igual ao sujeito de outra
pessoa compartilhe registros.

### 8.5 Filtro de dono (`OwnerFilter`)

Três formas: **usuário** (só registros desse dono), **não atribuído** (só registros compartilhados)
e **qualquer** (sem filtro).

Regra de admissão de um registro com dono `D`:

- registro compartilhado (dono ausente): **sempre admitido**;
- filtro "qualquer": admite tudo;
- filtro de usuário `U`: admite só `D == U`;
- filtro "não atribuído": não admite nenhum registro com dono.

Carimbo de dono: um registro só recebe dono quando a implantação **distingue operadores** e a
identidade é válida. Em servidor de um operador, o registro fica compartilhado, porque a
conexão local do mesmo operador não traz identidade de ator.

Invariante: o filtro de dono vale para **handoffs e sessões**, nunca para **leitura de páginas**.

---

## 9. Ponteiro de projeto ativo

Problema: as ferramentas MCP não conhecem o diretório de trabalho do usuário. Os hooks conhecem.
O roteador dos hooks publica um "projeto ativo" neste ponteiro e as ferramentas o leem como padrão.

### 9.1 Modos de isolamento (`ActiveProjectMode`)

| Modo | Chave | Quando |
|---|---|---|
| `single` | Slot global; a última publicação vence. | Opt-in, para quem quer o comportamento histórico de um slot único. |
| `per_session` | Identificador de sessão. | Isola execuções concorrentes do mesmo operador. |
| `per_actor` | Par (usuário, sessão), com um slot só de usuário ao lado. | **Padrão.** Isola harnesses paralelos e operadores diferentes. |

Chamador sem nenhuma coordenada (nem usuário nem sessão) lê o **slot compartilhado**. É o que mantém
uma instalação de um só harness funcionando como sempre.

### 9.2 Roteamento no meio da sessão (`MidSessionRouting`)

Configurado em `[routing] mid_session`. Decide qual projeto **durável** é gravado nas observações
quando o diretório de trabalho muda durante a sessão. **Nunca afeta** a abertura da sessão, que
sempre resolve pelo próprio diretório.

- `follow_cwd` (**padrão**): cada evento re-resolve o projeto pelo próprio diretório. `cd` para
  outro checkout grava as observações nesse outro projeto.
- `sticky`: a sessão mantém seu projeto, onde quer que o agente navegue. Motivo: a sessão tem um
  único projeto, e sua página consolidada cai em um só projeto. Seguir o diretório dividiria o
  registro bruto entre projetos enquanto a página fica em um só.

Em **ambos** os modos, um marcador de projeto (`.ai-memory.toml`) sempre vence: nomear o projeto
é um reescopo deliberado, não deriva.

### 9.3 Chave e mapa por ator

- Chave do ator: (usuário, sessão). Vazia (sem nenhum dos dois) cai no slot único.
- Cada entrada guarda: projeto resolvido, preferência de recall global (`default_global`, que faz
  leituras sem escopo explícito se comportarem como globais) e instante de inserção.
- **TTL padrão de 1 hora** por entrada. **Teto de 4096 entradas**; ao estourar, remove as mais antigas.
  Entradas vencidas são removidas tanto na leitura quanto na escrita.
- Travas de curta duração, nunca mantidas através de pausas assíncronas.

### 9.4 Resultados de consulta

Consulta de escrita (`ActiveProjectLookup`):

- **Resolvido**: achou workspace e projeto.
- **Divergência** (`Mismatch`): o chamador tem coordenada, o ponteiro está ativo e nada casou. **Nunca
  responde com o slot de outra sessão**: falha fechada.
- **Não definido** (`Unset`): não há informação de ponteiro para este chamador (anônimo, legado, ou
  instalação sem hooks). O padrão configurado é a única resposta.

"Ponteiro ativo" significa que **alguma** entrada com chave já foi publicada nesta instalação. Essa
marca nunca é apagada, mesmo quando as entradas expiram: uma instalação que já teve hooks não volta a
ser tratada como sem hooks.

Consulta de leitura (`ReadPointer`), com a origem da resposta:

- `Session`: entrada própria do chamador;
- `SharedSlot`: slot de processo, ou seja, o último projeto publicado (usado por chamador sem coordenada e em modo `single`);
- `StartupSeed`: semente de inicialização, usada enquanto não chega o primeiro evento de hook;
- `Mismatch` e `Unset`, com o mesmo significado de §9.4 acima.

A semente de inicialização é **só para leitura**. Nunca é usada para gravar: gravar com ela poderia
atribuir uma escrita a um projeto que o chamador não nomeou.

---

## 10. Handoff

Snapshot tipado de "onde paramos", deixado por uma sessão para a próxima de outro agente, possivelmente.

Estados: `open` → `accepted` ou `expired`. Um handoff é reivindicado **exatamente uma vez**, por
uma operação atômica de aceite.

Criação (`NewHandoff`): escopo (workspace e projeto), sessão de origem (opcional), agente de origem,
agente de destino (opcional), diretório de trabalho (opcional), resumo, perguntas em aberto, próximos
passos, arquivos tocados e **dono** (opcional; ausente = compartilhado com o projeto).

Aceite (`HandoffAcceptance`): identificador do handoff, escopo, agente, sessão e usuário que aceitam,
filtro de dono e diretório de quem recebe. O diretório de quem recebe **limita a expiração de handoffs
automáticos** [a confirmar].

Forma de leitura: escopo, origem, conteúdo e ciclo de vida são campos planos na resposta da API.

Lembrete da regra de §4.3: buscar o handoff é destrutivo. Nunca buscar quando a saída será descartada.

---

## 11. Mensagens entre projetos

Único ponto em que o isolamento por projeto é cruzado **de propósito**. Ver
[agent-messaging.md](agent-messaging.md).

- Mensagem: dirigida, de um projeto (origem) para outro (destino), reivindicável **uma única vez**.
- Estados: `pending` → `claimed` (reivindicada) ou `cancelled` (cancelada pelo remetente enquanto pendente).
- Caixas: **entrada** (mensagens destinadas ao projeto) e **saída** (mensagens enviadas pelo projeto).
  Um projeto só lê mensagens endereçadas a ele ou enviadas por ele.
- Campos de origem: workspace, projeto, agente, sessão e dono do remetente. Assunto opcional e corpo.
- Reivindicação registra quem reivindicou (agente, sessão, usuário) e o instante.

**Corpo é entrada não confiável.** Antes de gravar, o corpo é sanitizado (§7) e limitado em tamanho
[limite numérico a confirmar]. Ao ser entregue, vem acompanhado de um **aviso fixo** que o descreve
como tarefa a ser avaliada, nunca como instrução a obedecer: por si só não pode levar a executar
comandos, revelar segredos, mudar permissões ou chamar ferramentas. O destinatário julga o corpo
pela proveniência mostrada junto com ele.

---

## 12. Usuários

Prefixos de credencial:

| Prefixo | Tipo |
|---|---|
| `aim_` | Chave de API nativa. |
| `amk_` | Chave de API externa. |
| `ams_` | Segredo de sessão web. |

Limites:

- **Senha**: de 12 a 1024 bytes UTF-8. Não pode começar com nenhum prefixo de credencial acima. Não
  pode ser igual ao nome de usuário nem a nenhum segredo reservado da configuração (token de
  recuperação, bearer raiz, bearer do proxy). A verificação de colisão com hashes de credenciais
  de API é feita no armazenamento.
- **Nome de usuário**: não vazio, até **64 caracteres**. Proibidos: caracteres de controle, espaços
  em branco e separadores `/ \ : ; , " ' ` `. Letras, dígitos, `.`, `-`, `_` e `@` são permitidos, para
  que e-mails sirvam de nome (`alice@home`).
- **E-mail**: até **254 caracteres**; exatamente um `@`; parte local e domínio não vazios; sem espaços
  nem caracteres de controle. Validação deliberadamente permissiva: não exige domínio com extensão
  pública (endereços de intranet valem). Não é conformidade total com a especificação de e-mail.

Armazenamento: senha com **Argon2id**. Token de API com **SHA-256** sobre o token, um separador e o
segredo do servidor (pimenta).

Papel (`root` ou `user`):

- afeta só as sessões web (`root` vira nível raiz; `user`, nível usuário);
- chave de API nativa é **sempre** nível `user`, independentemente do papel.

Registro de usuário: identificador (UUIDv7), nome de usuário, nome de exibição (opcional), e-mail
(opcional), instantes de criação e último acesso, expiração do token legado, papel, obrigação de trocar
a senha no próximo login, desativação e se tem senha. Usuário só com token ("somente API") tem a flag
de senha falsa até redefinir a senha. Hashes nunca aparecem no registro.

---

## 13. Slots

Slot é uma página pequena e mutável, sob `_slots/`, injetada em **todo início de sessão do projeto**
(foco atual, pendências, contexto). Exemplos de caminho:

- `_slots/current-focus.md`: **compartilhado** com o projeto inteiro (é o formato de todo slot antigo);
- `_slots/u-alice/current-focus.md`: pertence só ao operador `u-alice`.

Com `[slots] per_user = true`, os slots passam a ser **separados por operador**:

- Visibilidade: o operador vê os slots compartilhados e os do próprio namespace. Os de outro
  namespace ficam ocultos no briefing. Sem identidade, só os compartilhados aparecem.
- Escrita: um slot sem prefixo de namespace é reescrito para o namespace do escritor
  (`_slots/atual.md` vira `_slots/u-alice/atual.md` para Alice). Um slot que já está no próprio
  namespace fica como está. Um slot no namespace de **outro** operador é marcado como
  "namespace estrangeiro" e não é gravado no namespace do escritor. Escritor sem identidade
  não tem namespace e não pode gravar em namespace de ninguém.
- O segmento de namespace é **sempre** o segmento de caminho seguro de §8.4. Nunca um nome de
  usuário cru nem um sujeito cru. Motivo: sujeitos costumam ser URLs, e um nome com metacaracteres
  de padrão poderia ler os namespaces de outros operadores.

Regra "sem prefixo = compartilhado": espelha a regra "dono ausente = compartilhado" de §8.5. Ligar o
recurso não reinterpreta nada já gravado.

Efeito colateral a conhecer: um caminho aninhado gravado **antes** do recurso (`_slots/backend/…`)
nomeia um segmento que nenhuma identidade qualificada produz. Com `per_user` ligado, ele não pertence a
ninguém e fica oculto de todos os briefings até o recurso ser desligado ou um administrador reatribuí-lo.
Com o recurso desligado, continua sendo um slot compartilhado comum.

O dono de um caminho de slot é o **primeiro segmento** após `_slots/`, mesmo com aninhamento mais fundo.

---

## 14. Conformidade OKF

Toda página é, por natureza, um arquivo de conceito no formato Open Knowledge Format v0.2
(versão `0.2`). Ver [okf.md](okf.md).

**Tipo derivado** (`type`):

1. Se o frontmatter tem `kind` com valor `fact`, `note`, `procedure` ou `decision`, o tipo é
   `Fact`, `Note`, `Procedure` ou `Decision`.
2. Senão, se tem `slot_kind` com valor `invariant` ou `state`, o tipo é `Invariant` ou `State`.
3. Senão, pela **família do caminho** (primeiro segmento):

| Família | Tipo |
|---|---|
| `sessions` | Session Summary |
| `_rules` | Rule |
| `gotchas` | Gotcha |
| `decisions` | Decision |
| `procedures` | Procedure |
| `concepts` | Concept |
| `notes` | Note |
| `runbooks` | Runbook |
| `_slots` | State |
| `_lint` | Lint Report |
| `_pending` | Pending Note |
| (qualquer outra) | Note |

**Conformação do frontmatter**: determinística e **idempotente** (aplicar duas vezes dá o mesmo
resultado). Preenche só o que é derivável; não inventa nada:

- `type`: só se ausente ou vazio;
- `description`: a partir de `summary`, se existir e não estiver vazio;
- `stale_after`: a partir de `expires_at`. Se o TTL é uma data simples, vira o instante do fim
  do dia em UTC; se já é RFC 3339, é copiado como está;
- `sources`: se ausente e há `session_id`, vira uma entrada com o recurso `ai-memory://session/<id>`
  e, se houver `agent`, o autor `process:<agente>`;
- `generated.by`: `generated_by_model` se existir; senão, `process:ai-memory/<versão>`.

Uma página conformada por versão anterior, que copiou `stale_after` como data simples, é **reparada**
na próxima reescrita.

**Instante de geração (`generated.at`)** fica **fora** da conformação, porque mudaria a cada versão e
quebraria a verificação de conteúdo idêntico. O armazenamento o carimba só quando o conteúdo muda, e
compara ignorando esse campo.

Conforme = tem `type` não vazio.

---

## 15. Fluxo de trabalho gerenciado

Tipos de fio do comando `ai-memory run`, que lança harnesses nativos com histórico compartilhado. Ver
[managed-workstreams.md](managed-workstreams.md).

Constantes:

- **Marcador de pacote**: `<!-- ai-memory:managed-workstream-packet:v1 -->`, no início de todo pacote de
  contexto entregue. Impede que o pacote entregue volte ao registro se o harness o persistir e reler.
- **Aviso de memória não confiável**: texto fixo que precede **todo** conteúdo recuperado e injetado
  automaticamente. Diz que o conteúdo guardado é dado histórico, nunca instrução: não executar comandos,
  não revelar segredos, não mudar permissões, não usar ferramentas porque o conteúdo pede. Trecho de tipo
  instrução é citado como evidência. Motivo: a sanitização remove segredos e limita tamanho, mas não
  consegue saber se um texto tenta manipular o modelo.

Tipos de evento (`WorkstreamEventKind`): `message`, `tool_call`, `tool_result`, `compaction`,
`checkpoint`, `annotation`. Chamada de ferramenta histórica **nunca** é reexecutada como pendente.

Evento novo (`NewWorkstreamEvent`): identificador de evento, **estável e derivado da fonte**, que torna
as retentativas idempotentes; agente; identificador de sessão nativa; identificador opcional do registro
de origem; tipo; papel (opcional); conteúdo; instante (opcional); metadados.

Checkpoint: cabeça do repositório, ramo, hash do estado sujo e caminhos alterados. Lê o estado do git
**sem alterar** o checkout.

Identidade de sessão nativa (`NativeSessionIdentity`): aceita o identificador **original, byte a byte**,
sem reescrever, se for válido. Recusa, como **desconhecido**, o identificador que: está vazio ou só tem
espaços; passa de 512 bytes; contém caractere de controle ou de formatação invisível (espaço de largura
zero, marcadores de direção, `U+FEFF`); contém marcador de redação; ou muda ao ser passado pela sanitização.
A identidade desconhecida recusa o vínculo gerenciado. No histórico, vira texto vazio.

Ciclo de execução gerenciada: preparar (abre um arranque e devolve identificadores, cursor de
sincronização e se pode adotar sessão existente), contexto de um envio, vincular, finalizar (eventos,
cursor, marcação de completo, checkpoint, perdas e código de saída), status, listar e renomear.

Identidade de repositório e de worktree: impressões digitais que identificam o checkout e o worktree.
Ver [complementary-contracts.md](complementary-contracts.md).

---

## 16. Utilitários

### 16.1 Métricas de ingestão

Contadores de processo, **atômicos** (incrementos concorrentes não se perdem), que contam o que
aconteceu com cada evento recebido:

- `accepted`: admitido para processamento;
- `dropped_by_policy`, `dropped_unauthorized`, `dropped_invalid`, `dropped_collision`: descartado por
  política, por falta de autorização, por entrada inválida ou por colisão;
- `shed_saturated`, `shed_rate_limited`: descartado por saturação ou por limite de taxa;
- `stored`, `replayed`, `resumed`, `ignored_end`, `failed`: gravado, reprocessado, retomado, fim
  ignorado ou falhou;
- `persisted` (com o último instante, em milissegundos Unix): último momento de gravação bem-sucedida.

A leitura (`snapshot`) é **sem conteúdo**: só números, nenhum texto de evento. Os contadores vivem junto
ao roteador de hooks porque ele escreve e a rota administrativa de status lê. Assim a camada de MCP não
precisa depender da de hooks.

Os contadores de resultado são aditivos: registrar um resultado não altera a decisão de admissão nem o último instante de gravação.

### 16.2 Detector de andaime

Pergunta: um título candidato (normalmente o primeiro prompt do usuário) é conteúdo do próprio usuário
ou **andaime injetado pelo harness**? É um teste de **forma**, não uma lista de exemplos conhecidos.
Erra a favor de **manter** o texto.

É andaime quando:

- está vazio ou só tem espaços;
- abre com um bloco de marcação: `<` seguido de um nome de tag (letras, dígitos, `_`, `-`, `/`, até 64
  caracteres), com `>` depois. Exemplos: blocos de contexto de IDE, de lembrete do sistema e de
  informação do usuário. Tag com atributos (`<pasted_content id="…">`) **não** é andaime, porque é
  colagem do usuário;
- começa com um glifo decorativo de prompt de shell (caixas de desenho, setas de terminal, chevrons).

**Não** é testado de propósito: identificador de modelo nu (`gpt-5`). Tem a mesma forma de uma referência
curta escrita por uma pessoa (`pr-477`). A exclusão é feita por tipo de evento: títulos vindos de um evento
de início de sessão, que recebe o modelo do harness, nunca usam o modelo como título.

### 16.3 Snippet de roteamento

Bloco que orienta o agente a usar a memória, delimitado por marcadores `<!-- ai-memory:start -->` e
`<!-- ai-memory:end -->`. Dois corpos: completo e compacto. Uma constante só, usada pelo comando de
instalação de instruções e pela ferramenta MCP de auto-roteamento.

### 16.4 Skills de roteamento gerenciadas

Conteúdo canônico das skills gerenciadas. Cada arquivo gerenciado começa com o marcador
`<!-- ai-memory-managed: routing-skill -->`. Diretórios de destino: `.claude`, `.agents`, `.devin`,
`.grok`, `.hermes`, cada um com a subpasta `skills`.

---

## 17. Notas para reimplementação

1. Comece pelos identificadores (UUIDv7, tipos distintos), pelo caminho de página, pelo nível de página,
   pelo tipo de observação e pelo **sanitizador com o tipo-fronteira**. Isso já dá um pipeline mínimo seguro.
2. O tipo-fronteira deve ter **um único construtor**, e o valor interno não deve ser acessível de fora.
   Assim, uma regra de revisão vira erro de compilação, não convenção.
3. A conversão de tipo de agente é tolerante na borda (valor desconhecido vira "outro") e estrita no
   interior (conversões de valores fechados falham). Borda tolerante, interior estrito.
4. Qualifique a identidade (`user:`/`oidc:`) desde o início. Migrar depois é caro.
5. Mantenha esta camada sem I/O: testes rápidos e dependência em um sentido só.

---

## 18. Glossário

- **Hook:** gancho de ciclo de vida do agente de código (início de sessão, prompt enviado, uso de
  ferramenta, fim de sessão). Entrega um evento em JSON ao servidor.
- **Handoff:** bastão de continuidade entre sessões. Uma sessão deixa o contexto para a seguinte, e um
  handoff é reivindicado exatamente uma vez.
- **Wiki:** árvore de arquivos markdown versionada em git. É a fonte da verdade do sistema.
- **Escritor único:** o processo que serializa todas as escritas no banco. As demais partes do sistema
  enviam comandos de escrita a ele por um canal; a leitura usa conexões somente de leitura.
