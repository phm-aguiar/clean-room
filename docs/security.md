# Segurança

> Visão geral do modelo de segurança do ai-memory: autenticação, autorização, armazenamento de
> credenciais, exposição de rede e recomendações de operação. Os limites de confiança e os testes
> adversariais estão em [security-boundaries.md](security-boundaries.md). As ameaças e o tratamento
> de dados estão em [threat-model-and-data-handling.md](threat-model-and-data-handling.md). O guia de
> HTTPS atrás de proxy reverso está em [https-via-proxy.md](https-via-proxy.md). A escada de
> usuários e a atribuição por operador estão em [users.md](users.md).

## Modelo em uma página

- Por padrão, o servidor escuta só em loopback (`127.0.0.1`, porta `49374`) e não exige autenticação.
  Isso é seguro em um equipamento de usuário único: nenhum processo fora da máquina alcança o servidor.
- Escutar em endereço não loopback sem autenticação é recusado na inicialização. Há duas exceções:
  dentro de contêiner, o servidor inicia com aviso; e, em qualquer ambiente, a opção
  `--allow-insecure-no-auth` libera HTTP puro em LAN de forma deliberada, com aviso permanente no
  terminal.
- Autenticação não cifra o token. Para acesso além do loopback, coloque um proxy reverso com TLS
  na frente (ver [https-via-proxy.md](https-via-proxy.md)). O servidor não termina TLS.
- Usuários humanos entram com senha e recebem sessão em cookie. Agentes e clientes de máquina usam
  `Authorization: Bearer`.
- Dados pertencem a um único locatário. Não há controle de acesso por página. A autorização é por
  projeto (ver [security-boundaries.md](security-boundaries.md)).

## Exposição de rede

### Regras de inicialização

As regras abaixo se aplicam ao endereço efetivamente escutado. Endereços de loopback sempre passam.
Para os demais, a primeira condição que bater decide:

| Situação | Resultado |
|---|---|
| Autenticação humana configurada e cookie de sessão não marcado como `Secure` | Recusado. |
| Autenticação por token raiz ou humana configurada | Inicia. |
| `--allow-insecure-no-auth` informado | Inicia, com aviso de que qualquer um na rede pode chamar ferramentas destrutivas. |
| Processo rodando em contêiner | Inicia, com aviso. A decisão de alcance fica com a publicação de porta no host. |
| Nenhuma das anteriores | Recusado. |

Detecção de contêiner: variável `AI_MEMORY_IN_CONTAINER` não vazia, ou marcadores de arquivo que os
runtimes de contêiner criam. Em contêiner, `-p 127.0.0.1:49374:49374` é seguro sem token. Qualquer
publicação em `0.0.0.0` ou em endereço da LAN exige token.

Os avisos de exposição são impressos no terminal (saída de erro) independentemente do nível de log,
para que um nível de log alto (por variável de ambiente ou pela configuração) não os silencie. Quando há autenticação configurada
mas o servidor escuta em endereço não loopback em HTTP puro, o servidor registra um aviso de que as
credenciais trafegam em texto claro.

### Allowlist de Host

- Variável `AI_MEMORY_ALLOWED_HOSTS` (lista separada por vírgulas). Padrão: `localhost`, `127.0.0.1`
  e `::1`. A imagem de contêiner oficial acrescenta `host.docker.internal`.
- A comparação ignora maiúsculas e aceita o host com ou sem porta.
- Requisição sem cabeçalho Host recebe **400**. Host fora da lista recebe **403**.
- O filtro vale para todo o servidor, inclusive para as rotas internas de introspecção.
- A allowlist defende contra DNS rebinding, em que o navegador define o Host. Não é controle de
  acesso: um cliente que alcança a porta define o Host livremente. Por isso não substitui o token.
- Em endereço não loopback, inclua no `AI_MEMORY_ALLOWED_HOSTS` o nome e o IP pelos quais o servidor
  será acessado.

### Limite de corpo

Corpos de requisição são limitados a 10 MiB nas rotas gerais. A rota de bootstrap administrativo
aceita até 32 MiB. A autenticação de login aceita no máximo 64 KiB de corpo.

## Autenticação de máquina (Bearer)

### Token raiz

- Vem de `AI_MEMORY_AUTH_TOKEN` ou de `[auth].bearer_token` no arquivo de configuração.
- Gere com `ai-memory generate-auth-token`. O padrão são 32 bytes aleatórios, impressos como 64
  caracteres hexadecimais (minúsculos, CSPRNG do sistema). O parâmetro `--bytes` altera o tamanho; não
  há mínimo validado (`--bytes 0` imprime linha vazia). Falha do gerador: `generating random token: <erro>`.
- A comparação é em tempo constante, para que o tempo de resposta não revele o token byte a byte.
- Valor vazio ou só com espaços é tratado como ausente. Nunca autentica como raiz.
- Com token raiz configurado, credencial ausente ou inválida recebe **401** com
  `WWW-Authenticate: Bearer`. O servidor nunca anuncia Basic nessa resposta.
- Esquemas diferentes de Bearer são ignorados. Mais de um cabeçalho Bearer no mesmo pedido falha
  fechado, sem escolher um deles.
- Sem nenhum token ou usuário configurado, todas as chamadas passam como anônimas. Esse é o modo de
  loopback de usuário único. Um cabeçalho Bearer antigo enviado a um servidor sem autenticação é
  tratado como anônimo, não como erro.

### Token de proxy confiável

Para um SSO que termina no proxy e não repassa a credencial do usuário:

- Configure `[auth].actor_proxy_bearer_token`. O valor precisa ser diferente do token raiz. Sem token
  raiz configurado, o servidor não inicia com essa opção.
- O proxy se autentica com esse token e descreve o usuário em cabeçalhos `X-Memory-Actor-*`
  (usuário, ou par emissor e sujeito, mais agente, cliente e id de sessão).
- O proxy **precisa substituir** os cabeçalhos enviados pelo cliente, não acrescentar aos deles.
  Cabeçalho repetido ou com vírgula gera **400** (ambíguo). Emissor sem sujeito, ou o contrário, gera
  **400**. Identidade sem usuário nem par emissor e sujeito gera **400**.
- A identidade só vira papel raiz se casar exatamente com o par `[auth].root_issuer` e
  `[auth].root_subject`, que devem ser configurados juntos. Nome de usuário nunca concede raiz.
  Qualquer outra identidade vira papel de usuário.
- Cabeçalhos `X-Memory-Actor-*` enviados com o token raiz ou com token de usuário do banco são
  ignorados.
- Use esse token só quando o servidor for alcançável apenas pelo proxy.

### Tokens de usuário do banco (`aim_`)

- Criados com `ai-memory api-key add`. Cada token tem 32 bytes aleatórios, codificados em base64 sem
  preenchimento (43 caracteres), precedidos pelo prefixo `aim_`.
- O servidor guarda `SHA-256(token + ":" + pepper)`. Nunca guarda o token. A tabela mantém só uma
  prévia com os últimos 4 caracteres, que não autentica.
- Esses tokens dão papel de usuário: servem para atribuição, nunca para administração.
- O último uso é registrado em segundo plano e não atrasa a resposta.
- Revogação marca a data de revogação. Rotação troca o hash.
- Sem `[auth].token_pepper`, tokens de usuário do banco não autenticam. Se já existirem credenciais
  e o pepper estiver ausente ou vazio, o servidor não inicia.

### Pepper

- `ai-memory init` gera o pepper para instalações novas: 32 bytes aleatórios em hexadecimal (64
  caracteres). Ele é gravado no arquivo de configuração. Em sistemas Unix, o arquivo é criado com
  permissão `0600`.
- O pepper não é usado para senhas humanas nem para sessões.
- Não troque o pepper depois de criar a primeira credencial `aim_`. Trocá-lo invalida todas as
  credenciais existentes.
- Ter o pepper configurado não muda o comportamento de usuário único: sem usuários cadastrados, o
  servidor se comporta como antes.

## Autenticação humana (senha e sessão)

### Senhas

- Hash com Argon2id, sal aleatório de 16 bytes e parâmetros padrão da implementação (memória de
  19456 KiB, 2 iterações, paralelismo 1).
- Tamanho entre 12 e 1024 bytes (UTF-8). A senha não pode começar com `aim_`, `ams_` ou `amk_`, não pode
  ser igual ao nome de usuário, e não pode ser igual a um segredo reservado configurado.
- A verificação roda fora da thread de escrita, com no máximo 2 verificações simultâneas e 32 na
  fila. Acima disso a resposta é **429** (`kdf saturated`).
- Conta inexistente, desativada ou sem senha passa por uma verificação fictícia de custo equivalente.
  Assim, o tempo de resposta não revela se a conta existe.
- Senhas temporárias geradas pelo sistema têm 32 caracteres aleatórios (24 bytes).
- Conta marcada para troca obrigatória de senha recebe **403** (`password change required`) nas rotas
  de sessão até trocar a senha.

### Limite de tentativas de login

- Cada falha de login (usuário inexistente, senha errada, conta desativada ou conta sem senha) conta
  contra o IP de origem. Dez falhas dentro de uma janela deslizante de 60 segundos fazem as novas
  tentativas do IP receberem **429** (`too many login attempts`) até as falhas saírem da janela.
- O IP considerado é o endereço TCP de origem. Só quando esse endereço pertence a um CIDR listado em
  `AI_MEMORY_AUTH__TRUSTED_PROXY_CIDRS` o servidor lê `X-Forwarded-For`, usando o último salto não
  confiável. Cabeçalho malformado faz o servidor usar o endereço TCP.
- O limite é por IP, não por conta. Não há bloqueio de conta. Até 4096 IPs ficam rastreados ao mesmo tempo.
- Falhas no fluxo de recuperação contam no mesmo limite.

### Sessão de navegador

- Ao entrar com `POST /auth/login`, o servidor emite um segredo de sessão de 32 bytes com prefixo
  `ams_` e um token CSRF de 24 bytes. O servidor guarda só o SHA-256 de cada um, sem pepper.
- Nomes (confirmados no original): sessão `ai_memory_session`, CSRF `ai_memory_csrf`, legado
  `ai_memory_auth`. O CSRF exige cabeçalho e cookie presentes, iguais (tamanho e conteúdo, tempo
  constante) e o SHA-256 do cabeçalho igual ao hash guardado.
- Cookie de sessão: `HttpOnly`, `SameSite=Strict`, `Path=/`, com `Max-Age` igual ao tempo restante
  da sessão. Recebe o atributo `Secure` quando `[auth].secure_cookie` ou
  `AI_MEMORY_AUTH__SECURE_COOKIE=true` estiver ativo.
- Cookie CSRF: legível pela página, sem `HttpOnly`. Em toda requisição que não seja GET, HEAD ou
  OPTIONS, o cabeçalho `x-csrf-token` deve bater com o cookie e com o hash guardado. Falha gera
  **403** (`csrf required`). A comparação é em tempo constante.
- Validade de 7 dias contados da criação. Não há renovação por inatividade. Logout revoga a sessão.
- Contas desativadas perdem as sessões imediatamente: a sessão deixa de ser aceita.
- Sessões humanas nunca autenticam as rotas de máquina (`/mcp`, `/hook`, `/handoff`, `/workstream/*`).
- A interface do console não guarda nenhum Bearer no `localStorage`. Só guarda um sinal de "não mostrar
  de novo" para avisos da interface.
- `Secure` não depende de cabeçalhos encaminhados. O servidor não usa `X-Forwarded-Proto` para decidir
  se o cookie é seguro. Essa decisão é só da configuração.

### Bootstrap e recuperação

- `AI_MEMORY_AUTH__INITIAL_ROOT_PASSWORD` define a senha inicial do usuário raiz. Ela é consumida uma
  única vez, na primeira inicialização sem bootstrap concluído, e ignorada depois.
- `AI_MEMORY_AUTH__RECOVERY_TOKEN` é o segredo de recuperação de emergência de `POST /auth/recovery`.
  A comparação é em tempo constante. O servidor guarda só o SHA-256 dele, em memória, e nunca em banco.
- Se a autenticação humana estiver ativa e não existir nenhum usuário raiz recuperável, o servidor não
  inicia sem um token de recuperação configurado.

### Transporte legado

Antes de haver senha humana ou marcador de bootstrap, as rotas de navegador (`/admin`, `/api/v1` e o
console) ainda aceitam HTTP Basic e o cookie legado `ai_memory_auth`, mas só em GET. Assim que a
autenticação humana for configurada, esses dois transportes deixam de valer, sem reinício. Enquanto a
autenticação humana estiver ativa, as respostas das rotas públicas trazem a expiração do cookie legado.
Um Basic bem-sucedido emite o cookie legado.

## Rotas e credenciais aceitas

| Superfície | Credenciais aceitas |
|---|---|
| `/mcp`, `/hook`, `/hook/batch`, `/handoff`, `/workstream/*` | Somente Bearer. |
| `/admin/*` | Bearer ou sessão humana. Com usuários cadastrados ou identidade de proxy configurada, só o papel raiz. |
| `/api/v1/*` | Bearer ou sessão humana. Mutações por sessão exigem CSRF. |
| Console integrado | Sessão humana. |
| `/auth/login`, `/auth/recovery` | Públicas (são as portas de entrada). |
| Interface personalizada do operador (`--web-ui-dir`) | Pública e estática. O login fica na própria interface. |

Sem nenhuma autenticação configurada, todas essas rotas são anônimas.

## Autorização

### Papéis

- **Anônimo**: nenhuma autenticação configurada.
- **Raiz**: token raiz válido, ou identidade de proxy que casa com o par raiz.
- **Usuário**: token de usuário do banco, ou identidade de proxy que não casa com o par raiz.

### Administração

- Com o primeiro usuário cadastrado, ou com identidade de proxy configurada, todas as rotas `/admin/*`
  ficam restritas ao papel raiz. Usuário recebe **403**. A checagem é feita a cada requisição, sem
  cache: o primeiro usuário fecha o acesso de bootstrap sem reiniciar o servidor.
- Sem usuários e sem identidade de proxy, `/admin/*` não distingue operadores. Isso é aceitável só em
  instalação local de usuário único.
- Quando operadores são distinguíveis, a rota inteira `/admin/*` é só do papel raiz, inclusive as
  rotas de status e busca.

### Projetos

- Projetos são abertos por padrão: qualquer usuário autenticado acessa. Com
  `[auth].new_projects_restricted`, projetos novos admitem só o criador (com permissão de escrita) e a
  raiz, até que outros recebam acesso. Projetos existentes não mudam com essa opção. Projetos
  reservados de uso geral ficam sempre abertos.
- Restrições e concessões por projeto estão descritas em [security-boundaries.md](security-boundaries.md).

## Limites de requisição e de tentativas

### Ingestão de hooks

- `AI_MEMORY_HOOK_RATE_PER_SEC` define quantos eventos por segundo cada fonte pode enviar. Desligado
  quando o valor é 0 ou não definido.
- `AI_MEMORY_HOOK_RATE_BURST` define a rajada permitida. Quando não definido ou 0, a rajada é
  `max(taxa, 1)`.
- A fonte é a combinação de usuário e id de sessão do hook. Sem id de sessão, é uma impressão digital
  dos campos do evento. Uma sessão descontrolada não bloqueia as demais.
- Evento acima da rajada recebe **429** (`hook source rate limited`) e é descartado. Até 4096 fontes
  ficam rastreadas. Ao exceder, a fonte inserida há mais tempo é descartada.

### CORS

- Só o `/api/v1` tem CORS. `/mcp`, `/hook`, `/admin` e `/web` não respondem a requisições de origem
  cruzada.
- Origens são configuradas com `--cors-allow-origin` (repetível) ou `AI_MEMORY_CORS_ALLOW_ORIGINS`.
  Sem origens, o CORS fica desligado.
- Validação na inicialização: `*` é recusado (a especificação não permite credenciais com curinga), toda
  origem precisa de esquema (`https://...`) e não pode ter barra final.
- Métodos permitidos: GET, POST e OPTIONS. Cabeçalhos permitidos: `Authorization`, `Content-Type` e
  `x-csrf-token`. Credenciais são permitidas. Pré-voo em cache por 600 segundos.

## Recomendações de operação

- Exija token em qualquer endereço não loopback. Gere-o com `ai-memory generate-auth-token` e
  informe-o ao servidor por `AI_MEMORY_AUTH_TOKEN` ou pela configuração.
- Use HTTPS na frente de qualquer acesso que saia do equipamento. Os modelos de proxy com TLS (Caddy
  com Let's Encrypt ou CA interna, e túnel da Cloudflare, sem portas abertas) estão em
  [https-via-proxy.md](https-via-proxy.md). Feche ou redirecione o HTTP direto para o mesmo nome.
- Com autenticação humana fora do loopback, defina `AI_MEMORY_AUTH__SECURE_COOKIE=true`.
- Inclua o nome público em `AI_MEMORY_ALLOWED_HOSTS` antes de expor o servidor.
- Dê a cada pessoa a própria conta. Não compartilhe o token raiz com pessoas nem com scripts de uso
  geral: use tokens `aim_` para automação.
- Se usar SSO com proxy, faça o proxy substituir os cabeçalhos `X-Memory-Actor-*`, e mantenha o
  servidor acessível apenas por esse proxy.
- Não troque o pepper depois da primeira credencial `aim_`.
- Proteja o arquivo de configuração. Ele contém o token raiz, o pepper e outros segredos em texto
  claro (ver [threat-model-and-data-handling.md](threat-model-and-data-handling.md)).
- Para a instalação de hooks em servidores compartilhados, veja também [deploy.md](deploy.md).

### Exemplo de implantação com token

Substitua `<servidor>` pelo endereço do servidor e `<imagem>` pela imagem de contêiner publicada pelo
projeto:

```bash
TOKEN=$(ai-memory generate-auth-token)

docker run -d --name ai-memory \
    --restart unless-stopped \
    -p 0.0.0.0:49374:49374 \
    -v ai-memory-data:/data \
    -e AI_MEMORY_AUTH_TOKEN="$TOKEN" \
    -e AI_MEMORY_ALLOWED_HOSTS="<servidor>,localhost,127.0.0.1" \
    <imagem>:latest

ai-memory install-mcp   --client claude-code --apply \
    --server-url "http://<servidor>:49374/mcp" --auth-token "$TOKEN"
ai-memory install-hooks --agent  claude-code --apply \
    --server-url "http://<servidor>:49374" --auth-token "$TOKEN"
```

Esse exemplo usa HTTP puro. Antes de usá-lo em rede, ponha um proxy com TLS na frente.

## Identidade OIDC

- O cliente pode obter um token OIDC por fluxo de autorização de dispositivo com
  `ai-memory auth login oidc-device`, informando emissor e id de cliente. O token fica armazenado no
  arquivo de autenticação da instalação, é renovado quando vence e é usado pelos hooks nativos e pelos
  comandos HTTP leves (como status e busca) quando não há token estático.
- Token estático (`--auth-token`, `AI_MEMORY_AUTH_TOKEN` ou `[auth].bearer_token`) tem precedência sobre
  o token OIDC.
- Um perfil de servidor usa só o próprio token. Nunca recebe o token OIDC da instalação, para não vazá-lo
  a outro servidor.
- O servidor não aceita token OIDC diretamente. Ele aceita token raiz, token de usuário do banco e
  identidade afirmada por proxy confiável. Com autenticação configurada, um token OIDC enviado
  diretamente ao servidor recebe **401**.
- Para ter papel administrativo com identidade OIDC, o proxy precisa traduzir a identidade para o par
  raiz configurado. Sem isso, `/admin/*` continua restrito à raiz.
- Id de sessão do provedor de login não é id de sessão de agente. Para isolamento por sessão em servidor
  compartilhado, use escopo explícito (workspace e projeto) ou um intermediário que repasse o id real da
  sessão de hook (ver [auto-scope.md](auto-scope.md)); a regra exata de isolamento de `[auto_scope]`
  está a confirmar.
- Hooks em script, que são o padrão em contêiner, não usam o token OIDC armazenado (confirmado no
  original: o script envia o evento direto por HTTP, sem spool local nem token OIDC; só o comando de
  hook nativo do binário, padrão no Windows e em Linux/macOS nativos, tem spool e o token OIDC como
  último recurso).
- Autenticação OIDC de gateways que traduzem a identidade para o servidor segue o mesmo caminho de
  proxy confiável descrito acima. Ver [sso.md](sso.md).
