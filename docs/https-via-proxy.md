# HTTPS via proxy reverso

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

> O ai-memory não encerra TLS. Este guia descreve como colocar um proxy
> reverso com TLS na frente do servidor, para que tokens e cookies do `/web`
> trafeguem cifrados entre clientes e servidor. A instalação padrão continua
> em HTTP simples, somente em loopback.

## Quando não é necessário

Não é preciso TLS nestes cenários:

- **Transporte stdio com usuário único.** O transporte stdio não abre porta de rede.
- **HTTP somente em loopback**, para um usuário, sem acesso ao `/web` a partir de outra máquina. O endereço padrão `127.0.0.1:49374` não é alcançável de fora do host.
- **Desenvolvimento local e experimentos pontuais.**

A instalação de usuário único cai nesse caso. A maioria das instalações não precisa de proxy.

## Quando é necessário

Adicione um proxy com TLS quando qualquer condição abaixo for verdadeira:

- **Autenticação de usuários ativa.** Há usuários cadastrados ou credenciais de API, ou login por senha configurado. Nesse caso `[auth].token_pepper` é obrigatório e não pode ser vazio. Chaves de API com prefixo `aim_` trafegam entre clientes e servidor e ficariam legíveis em HTTP na rede local. Ver [users.md](users.md).
- **Bind além do loopback.** `AI_MEMORY_BIND` com `0.0.0.0` ou com um IP alcançável pela rede local. Qualquer um no segmento de rede vê o tráfego de tokens e os cookies do `/web`.
- **Acesso ao `/web` a partir de outra máquina.** O cookie de sessão definido após o login por senha trafega em claro sem TLS.
- **Exposição além da rede local.**

## Regras de inicialização

O servidor recusa ou aceita HTTP sem autenticação conforme o endereço de escuta efetivo e o estado de autenticação. Termos usados abaixo:

- **Autenticação por token:** `AI_MEMORY_AUTH_TOKEN` (`[auth].bearer_token`) configurado.
- **Autenticação humana:** ativa quando o bootstrap do root já foi concluído, há senha cadastrada, ou há senha inicial ou token de recuperação configurados.
- **Autenticação ativa:** autenticação por token ou autenticação humana.
- **Contêiner:** a variável `AI_MEMORY_IN_CONTAINER` está definida com valor não vazio, ou existe o marcador de arquivo que os runtimes de contêiner criam. A imagem oficial define a variável.

Regras, na ordem em que são avaliadas:

1. Endereço de loopback: aceito, sem aviso.
2. Endereço que não é loopback com autenticação humana e `secure_cookie = false`: **recusa** a inicialização.
3. Endereço que não é loopback com autenticação ativa e (se houver autenticação humana) `secure_cookie = true`: aceito, sem aviso específico. O log informativo registra `auth = true`.
4. Endereço que não é loopback sem autenticação, com `--allow-insecure-no-auth`: aceito, com aviso impresso no stderr e alerta no log. Qualquer cliente que alcance a porta pode chamar as ferramentas destrutivas.
5. Endereço que não é loopback sem autenticação, dentro de contêiner: aceito, com aviso. O servidor não consegue saber se a porta publicada alcança a rede; isso depende de como o host publica a porta. Publicada como `127.0.0.1:49374:49374`, a configuração é segura. Publicada em `0.0.0.0` ou em IP de LAN, qualquer um na rede pode chamar as ferramentas destrutivas. O aviso recomenda gerar um token com o comando `ai-memory generate-auth-token` e definir `AI_MEMORY_AUTH_TOKEN`.
6. Endereço que não é loopback sem autenticação, fora de contêiner: **recusa** a inicialização. A mensagem orienta a configurar `AI_MEMORY_AUTH_TOKEN`, usar endereço de loopback, ou passar `--allow-insecure-no-auth` para uso intencional em rede local.

Consequências:

- Cookies de sessão sem o atributo `Secure` só são aceitos em endereço de loopback.
- Com proxy no mesmo host, o servidor pode continuar em loopback. Não é preciso expor `0.0.0.0`.
- Se o servidor estiver em outro contêiner e o proxy alcançá-lo pela rede do contêiner, o endereço de escuta não é loopback. As regras 2 a 6 passam a valer.

## Escolha do caminho

| Caminho | Indicado para | O que é preciso fora do servidor |
|---|---|---|
| Proxy com domínio público e certificado automático de autoridade pública | Operadores com domínio e portas 80 e 443 alcançáveis pela internet (caso comum em redes domésticas com encaminhamento de porta). | Registro DNS apontando para o IP público. |
| Proxy com autoridade interna (somente rede local) | Usuários múltiplos na rede local, sem exposição pública. Cada máquina cliente precisa confiar na raiz da autoridade uma vez. | Instalação da raiz em cada cliente. |
| Túnel de saída gerenciado (exemplo: Cloudflare Tunnel) | Quem não quer abrir portas no roteador. O túnel é de saída e o TLS termina na borda do provedor. | Conta no provedor e domínio gerenciado por ele. |
| Arquivos de certificado externos | Quem já tem uma autoridade corporativa ou doméstica emitindo certificados para os serviços. | Arquivos de certificado e chave, emitidos por qualquer meio. |
| Proxy reverso genérico (exemplo: nginx) | Quem já opera um proxy e quer uma única linguagem de configuração. | Igual ao caminho com domínio público ou ao caminho com arquivos externos. |

Os exemplos de configuração de proxy nas seções seguintes são ilustrativos. Qualquer proxy com terminação TLS, encaminhamento para o servidor e preservação dos cabeçalhos citados serve.

## Configuração do servidor

Variáveis de ambiente relevantes para o proxy:

| Variável | Padrão | Efeito |
|---|---|---|
| `AI_MEMORY_BIND` | `127.0.0.1:49374` | Endereço de escuta. A opção `--bind` da linha de comando tem precedência sobre a variável. A imagem de contêiner já inicia com `--bind 0.0.0.0:49374`, então a variável não tem efeito enquanto esse comando estiver em uso. |
| `AI_MEMORY_AUTH_TOKEN` | nenhum | Token bearer (`[auth].bearer_token`). Gerar com `ai-memory generate-auth-token`. |
| `AI_MEMORY_AUTH__SECURE_COOKIE` | `false` | Equivale a `[auth].secure_cookie`. Marca o cookie de sessão como `Secure`. É obrigatório com autenticação humana em endereço que não é loopback. |
| `AI_MEMORY_ALLOWED_HOSTS` | `localhost,127.0.0.1,::1` | Lista de valores de `Host` aceitos. Ver a seção de Host. |
| `AI_MEMORY_CORS_ALLOW_ORIGINS` | vazio | Origens permitidas para CORS. Ver a seção de CORS. |
| `AI_MEMORY_BASE_PATH` | vazio | Prefixo de URL para todo o servidor. Ver a seção de subcaminho. |
| `AI_MEMORY_WEB_SLUG` | `/web` | Caminho da interface web dentro do prefixo. |

A lista de hosts aceita valores separados por vírgula ou lista no arquivo de configuração. Na imagem de contêiner, a lista padrão é `localhost,127.0.0.1,::1,host.docker.internal`. Definir `AI_MEMORY_ALLOWED_HOSTS` no ambiente substitui essa lista inteira.

## Bind e rota do proxy até o servidor

- A ligação entre proxy e servidor é HTTP simples, na porta padrão `49374`. O TLS termina no proxy.
- **Proxy no mesmo host:** o proxy aponta para `127.0.0.1:49374`. O bind de loopback basta.
- **Proxy em contêiner separado:** o servidor escuta em `0.0.0.0:49374` dentro da rede do contêiner e **não publica** a porta no host. Somente o proxy publica portas. Publique 80 e 443 no proxy. A porta 80 serve ao desafio de certificado de autoridade pública e pode ser omitida na autoridade interna.
- Feche o acesso HTTP direto ao hostname público, ou redirecione para HTTPS no proxy.

## Host público e lista de hosts

- A lista `AI_MEMORY_ALLOWED_HOSTS` **deve conter o hostname público**, não apenas `localhost`. A verificação de `Host` roda antes da autenticação. Ela enxerga o `Host` que o proxy repassa, e recusaria a requisição caso o nome público não esteja na lista.
- O proxy deve repassar o cabeçalho `Host` original do cliente.
- A comparação ignora diferença de maiúsculas e minúsculas e ignora a porta. Não há curinga: cada hostname precisa estar listado literalmente.
- Requisição sem cabeçalho `Host` recebe 400. Requisição com `Host` fora da lista recebe 403.
- A lista é uma defesa contra DNS rebinding em navegadores. Não é controle de acesso. Um cliente que alcança a porta pode enviar qualquer `Host`.

## Cabeçalhos de proxy

### Cabeçalhos de esquema e host não são interpretados

O servidor não infere HTTPS a partir de `X-Forwarded-Proto` nem de qualquer outro cabeçalho de proxy. O atributo `Secure` do cookie depende somente de `AI_MEMORY_AUTH__SECURE_COOKIE`. O proxy pode enviar esses cabeçalhos, mas eles não têm efeito no servidor.

O proxy deve repassar o cabeçalho `Authorization` sem alterá-lo. O token bearer é lido diretamente dele.

A resposta do `/api/v1` tem ETag calculado sobre o corpo da resposta. Ele não depende do host nem do esquema recebidos. Confirmado no código de referência: nenhuma resposta do navegador embutido, da API ou da interface customizada monta URL absoluta com o esquema ou o host da requisição. Os redirecionamentos (login, troca de senha, barra final) devolvem `Location` só com o **caminho** (começando por `/`, com o prefixo de base já incluído), e os links do HTML são relativos à base injetada. Por isso um proxy que reescreve `Host` ou o esquema não quebra os links.

### IP do cliente (`X-Forwarded-For`)

O cabeçalho `X-Forwarded-For` é usado somente para limitar tentativas de login e de recuperação de senha. Ele é honrado apenas quando a conexão vem de um CIDR listado em `[auth].trusted_proxy_cidrs`:

- Os valores de todas as ocorrências do cabeçalho são lidos na ordem recebida e concatenados em uma só lista; cada valor é dividido em vírgulas e cada item é aparado.
- O servidor percorre os IPs da direita para a esquerda e usa o primeiro que não pertence aos CIDRs confiáveis. Se todos pertencerem, usa o primeiro IP da lista (o mais à esquerda da primeira ocorrência do cabeçalho).
- Se a conexão não vier de um CIDR confiável, se o cabeçalho estiver ausente, ou se algum item não for um IP válido (item vazio, como em `1.2.3.4,`, valor com porta, valor com bytes que não são ASCII), vale o IP da conexão. Um único item inválido invalida a lista inteira.
- Sem informação do par da conexão (não ocorre em um servidor TCP normal), o endereço usado é `0.0.0.0`.
- `[auth].trusted_proxy_cidrs` (variável `AI_MEMORY_AUTH__TRUSTED_PROXY_CIDRS`, lista separada por vírgula): cada entrada é `a.b.c.d/n` ou `x::y/n` (prefixo até 32 em IPv4 e até 128 em IPv6) ou um endereço puro (`/32` ou `/128`); na variável de ambiente as entradas são aparadas e as vazias são descartadas. Uma entrada vazia (só possível no arquivo de configuração), com endereço ou prefixo inválido impede a inicialização, com o texto `invalid [auth].trusted_proxy_cidrs entry "<entrada>": <motivo>` (motivos: `empty CIDR`, `invalid CIDR prefix in "<entrada>"`, `invalid CIDR address in "<entrada>"`, `CIDR prefix N exceeds 32|128`). IPv4 e IPv6 nunca se comparam entre si (um IPv4 mapeado em IPv6 não casa com um CIDR IPv4).
- O IP escolhido alimenta só o limitador de tentativas de login e de recuperação: no máximo **10 falhas por IP em 60 s** (janela deslizante, cada falha expira individualmente quando passa de 60 s), no máximo 4 096 IPs guardados (cheio: descarta os expirados e, se ainda cheio, o IP cuja falha mais antiga é a mais velha); acima do limite a resposta é `429` com `{"error":"too many login attempts"}`. Ver [users.md](users.md).

### Identidade asserida pelo proxy (multiusuário)

Quando um proxy identifica o usuário final, ele se autentica com um token bearer dedicado em `[auth].actor_proxy_bearer_token`:

- O token do proxy deve ser diferente de `[auth].bearer_token`, que é o root. Ambos devem ser configurados juntos. Configurar só o do proxy, ou usar o mesmo valor nos dois, impede a inicialização.
- A identidade é lida dos cabeçalhos `X-Memory-Actor-User`, `X-Memory-Actor-Issuer`, `X-Memory-Actor-Sub`, `X-Memory-Actor-Agent`, `X-Memory-Actor-Client` e `X-Memory-Actor-Session-Id`.
- O proxy deve **substituir** esses cabeçalhos, não anexar. Se o cliente enviou valores próprios, eles precisam ser removidos ou sobrescritos antes de chegar ao servidor. Com anexação, o valor do cliente chega primeiro e é o que o servidor lê.
- Um cabeçalho repetido, ou com valor separado por vírgula, faz a requisição falhar com 400, porque a identidade fica ambígua.
- Emissor (`Issuer`) e sujeito (`Sub`) são aceitos somente juntos. Um sem o outro resulta em 400.
- A asserção precisa de uma identidade: usuário, ou emissor com sujeito. Sem ela, a requisição recebe 400.
- As respostas de erro desses casos são texto simples.
- O token do proxy só vale com o esquema `Bearer`. Não vale com Basic nem com cookie.
- `[auth].root_issuer` e `[auth].root_subject` devem ser configurados juntos.

Ver [users.md](users.md) para o modelo de usuários e de atribuição.

## Cookies do `/web`

- Cookie de sessão `ai_memory_session`: sempre `HttpOnly`, `SameSite=Strict` e `Path=/`, com `Max-Age` igual ao tempo restante da sessão. Com `secure_cookie = true`, também `Secure`.
- Cookie CSRF `ai_memory_csrf`: não é `HttpOnly`. Os demais atributos seguem a mesma regra.
- A cada definição de cookie de sessão, o servidor também envia a expiração do cookie de autenticação legado.

Com HTTPS, defina `AI_MEMORY_AUTH__SECURE_COOKIE=true`, ou `[auth] secure_cookie = true` no arquivo de configuração. Sem essa opção, a autenticação humana não inicia em endereço que não é loopback. O valor padrão é `false`, para permitir smoke tests e desenvolvimento em loopback com HTTP.

## CORS

- Sem origens configuradas (padrão), nenhuma resposta traz cabeçalhos CORS.
- As origens vêm de `AI_MEMORY_CORS_ALLOW_ORIGINS` (lista separada por vírgula) ou da opção `--cors-allow-origin`, repetível. As duas fontes são combinadas, sem duplicatas, com as origens do arquivo de configuração primeiro.
- Cada origem precisa começar com `http://` ou `https://`, não pode terminar em `/`, e o valor `*` é rejeitado. Origem inválida impede a inicialização.
- O CORS vale somente para `/api/v1`. As rotas `/mcp`, `/hook`, `/admin` e `/web` não recebem cabeçalhos CORS.
- Para as origens configuradas, o preflight aceita os métodos `GET`, `POST` e `OPTIONS`, os cabeçalhos `Authorization`, `Content-Type` e `X-CSRF-Token`, e credenciais. A resposta de preflight tem `max-age` de 600 segundos.
- Interface servida pelo mesmo host e pelo mesmo proxy não precisa de CORS. Use CORS somente para origens diferentes.

## Subcaminho (`AI_MEMORY_BASE_PATH`)

Se o servidor compartilha o hostname com outros aplicativos, mantenha o prefixo ao repassar a requisição e informe o prefixo ao servidor:

```bash
AI_MEMORY_BASE_PATH=/wiki
```

```caddyfile
memory.example.com {
    handle /wiki/* {
        reverse_proxy ai-memory:49374
    }
}
```

Não use uma diretiva que remova o prefixo antes de repassar a requisição. O servidor serve todas as rotas sob o prefixo configurado. Com o prefixo removido, as rotas não casam.

Com o exemplo acima, os clientes usam:

```bash
ai-memory install-mcp   --client claude-code --apply \
    --server-url "https://memory.example.com/wiki/mcp" --auth-token "$AI_MEMORY_AUTH_TOKEN"
ai-memory install-hooks --agent  claude-code --apply \
    --server-url "https://memory.example.com/wiki" --auth-token "$AI_MEMORY_AUTH_TOKEN"
```

O caminho da URL configurada no cliente é usado como prefixo base pelo cliente. Regras exatas (comandos clientes como `status`, `backup`, `compact`, e o teste de saúde do contêiner):

- A URL do servidor é dividida em origem (esquema + autoridade) e caminho (tudo a partir da primeira `/` depois da autoridade); barras finais são aparadas. Uma URL sem `://` é tratada como origem opaca, sem caminho.
- O prefixo de base do cliente é, nesta ordem: o caminho da URL do servidor (`http://host:49374/wiki` → `/wiki`); se a URL não tem caminho, o valor de `AI_MEMORY_BASE_PATH` (ou `base_path` do arquivo de configuração). O caminho da URL **vence** a variável. O prefixo escolhido passa pelo mesmo normalizador de prefixo (abaixo): um caminho com caractere fora do conjunto não reservado ou com segmento `.`/`..` vira raiz (`""`), sem aviso.
- Toda rota chamada é `origem + prefixo + rota` (ex.: `http://host:49374/wiki/admin/status`).
- A URL padrão do servidor, quando nem o arquivo de configuração nem `AI_MEMORY_SERVER_URL` a definem, é derivada do endereço de escuta configurado (`AI_MEMORY_BIND`/`bind`): o `host:port` vira `http://host:port` (divide no último `:`; `0.0.0.0` vira `127.0.0.1` e `[::]` vira `[::1]`; nome de host e IPv6 entre colchetes são mantidos; host vazio, porta 0 ou inválida, ou IPv6 sem colchetes deixam a URL padrão). Sem nenhum dos dois: `http://127.0.0.1:49374`. A opção `--bind` da linha de comando do `serve` **não** entra nessa derivação (é de outro processo).

A interface web fica em `https://memory.example.com/wiki/web`. Para servir a interface diretamente em `https://memory.example.com/wiki`, defina `AI_MEMORY_WEB_SLUG=/`. Isso vale para a interface integrada e para uma interface customizada de página única (`--web-ui-dir`).

Login humano: a interface integrada tem página de login própria, sob o caminho da interface web (ex.: `/web/login`), e redireciona para ela as requisições HTML sem sessão. Com interface customizada (`--web-ui-dir`), as páginas são públicas e o login é feito pela própria interface.

### Normalização e segurança dos prefixos

`AI_MEMORY_BASE_PATH` e `AI_MEMORY_WEB_SLUG` passam pelo mesmo normalizador:

- Espaços nas extremidades são removidos. Barras repetidas e barras nas extremidades são descartadas: `//wiki//` vira `/wiki`.
- `/` ou valor vazio significam raiz.
- Cada segmento precisa conter apenas caracteres não reservados da RFC 3986 (`A-Z`, `a-z`, `0-9`, `-`, `.`, `_`, `~`).
- Os segmentos `.` e `..` são rejeitados. Em fronteira de segmento significam diretório atual e pai, e aceitá-los permitiria percurso de diretório no prefixo.
- Se qualquer segmento for inválido, o valor inteiro cai para montagem na raiz. O log de inicialização registra o motivo.
- O redirecionamento permanente de barra final, de `{base_path}{web_slug}/` para a forma canônica, usa status **308** e preserva a query string.
- O aviso de inicialização para prefixo inseguro é `AI_MEMORY_BASE_PATH is not a safe path prefix; serving at root instead` (e `AI_MEMORY_WEB_SLUG is not a safe path prefix; serving the web UI at the base-path root instead`), com o valor bruto no registro. Entrada que só tem espaços ou barras não gera aviso (é raiz legítima).
- O healthcheck do contêiner (`status`) usa o mesmo prefixo (variável `AI_MEMORY_BASE_PATH` do ambiente do contêiner) e o `Host` `127.0.0.1:49374`; a lista de hosts permitidos deve manter `127.0.0.1` ou `localhost`, senão a própria verificação de saúde recebe 403 e o contêiner fica `unhealthy`.

## Clientes

Exemplo com um cliente de agente de código, com `https://` e o token em `Authorization: Bearer`:

```bash
ai-memory install-mcp   --client claude-code --apply \
    --server-url "https://memory.example.com/mcp" --auth-token "$AI_MEMORY_AUTH_TOKEN"
ai-memory install-hooks --agent  claude-code --apply \
    --server-url "https://memory.example.com" --auth-token "$AI_MEMORY_AUTH_TOKEN"
```

Os demais clientes seguem o mesmo formato. Ver [mcp-install.md](mcp-install.md) para a lista de clientes suportados.

### Transporte com proxy: `mcp-bridge`

O `mcp-bridge` aceita URLs `https://`. Ele usa o verificador de certificados da plataforma, que confia nas mesmas raízes do sistema operacional. Um certificado que o sistema não confia (autoassinado, ou de autoridade privada não instalada no repositório de confiança do sistema) é rejeitado. Aponte o bridge diretamente para a URL HTTPS do proxy. Não é preciso um segundo proxy local em cada máquina cliente.

## Certificados e confiança

### Autoridade pública

Certificados de autoridade pública são aceitos pelos sistemas e clientes sem instalação manual.

### Autoridade interna (somente rede local)

A autoridade interna gera uma raiz por servidor de proxy. Instale essa raiz **uma vez** em cada máquina cliente. Para extrair a raiz, copie o arquivo `root.crt` do volume de dados do proxy, no caminho `caddy/pki/authorities/local/root.crt` (exemplo com Caddy).

Instalação por sistema operacional:

| SO | Comando |
|---|---|
| macOS | `sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain raiz-do-proxy.crt` |
| Linux (Debian/Ubuntu) | `sudo cp raiz-do-proxy.crt /usr/local/share/ca-certificates/ && sudo update-ca-certificates` |
| Linux (Arch/openSUSE) | `sudo trust anchor --store raiz-do-proxy.crt` |
| Windows | `certutil -addstore -f "Root" raiz-do-proxy.crt` (PowerShell como administrador) |
| iOS / Android | Enviar o arquivo ao dispositivo, abrir e instalar como perfil. Em iOS, também ativar a confiança explícita em Ajustes → Geral → Sobre → Configurações de confiança de certificado. |

Cliente sem a raiz instalada fica em uma de duas situações. Ou recusa a conexão TLS (clientes MCP, curl), ou o usuário aprende a clicar em aviso (navegadores). Nos dois casos não há a proteção do TLS. Um certificado que o usuário aprende a ignorar é pior que HTTP com token, porque parece seguro e não é. Instale a raiz em toda máquina que se conecta, ou use o caminho com autoridade pública ou túnel.

Os mesmos passos valem para o caminho com arquivos externos, porque o proxy não se importa de onde vem o certificado.

## Timeouts para `bootstrap`

`ai-memory bootstrap` envia um **único POST** para `/admin/bootstrap` com o pacote de fontes do projeto. A requisição fica aberta durante todo o processamento em blocos, que inclui chamadas ao provedor de LLM. Em projetos grandes, isso pode levar 20 minutos ou mais, sem bytes trafegando na conexão. Um proxy com timeout de leitura ou de inatividade padrão corta a conexão (`connection reset by peer`) e a execução se perde.

Se o `bootstrap` passar pelo proxy, aumente ou desative o timeout de leitura e de escrita da rota do ai-memory no proxy:

- Em um proxy com diretiva de transporte para o backend, zere os timeouts de leitura e de escrita.
- Em nginx, aumente `proxy_read_timeout` e `proxy_send_timeout` acima da maior execução esperada. O padrão de nginx costuma ser 60 segundos (a confirmar: valor do produto, fora do repositório).

Esse ajuste só importa para o POST longo do `bootstrap`. Requisições comuns de MCP e de `/api/v1` são curtas. Uma falha em um bloco ainda pode perder a execução. A confirmar: comportamento da opção `--resume` nesse caso.

Configuração de nginx para o caso acima:

```nginx
location / {
    proxy_pass http://127.0.0.1:49374;
    proxy_read_timeout 3600s;
    proxy_send_timeout 3600s;
    # mais as linhas de http/1.1 e Connection "" do modelo de nginx da seção de caminhos
}
```

Para o transporte MCP Streamable HTTP, o proxy precisa usar HTTP/1.1 e limpar o cabeçalho `Connection`. No modo padrão, as respostas são JSON simples, sem streaming. No modo stateful (`--http-stateful`), as respostas usam SSE, e a configuração de HTTP/1.1 é obrigatória para o streaming funcionar.

## Não enfraqueça as proteções

1. **Não desative a verificação de Host.** Ela é a defesa contra DNS rebinding. Quando o proxy "deveria" filtrar, o resultado costuma ser um bug por suposição de camada. Inclua o hostname público na lista. Não existe curinga.
2. **Não use opções de confiança insegura no cliente** (como `-k`, `--insecure` ou equivalentes) para contornar a instalação da raiz. Isso deixa TLS sem autenticação do servidor, o que é pior que HTTP com token, porque parece seguro e não é.
3. **Não desative a verificação de certificado do túnel.** Em um túnel de saída, o trecho entre o túnel e o servidor é HTTP simples dentro da rede do contêiner. Se a verificação de certificado estiver falhando, algo mais está mal configurado.

Se não for possível seguir um desses caminhos com segurança, a resposta honesta é manter o servidor somente em loopback, ou expô-lo por um proxy já confiável para a equipe. Uma configuração que dá a impressão de segurança sem oferecê-la é pior que as duas opções.

## Caminho 1: proxy com domínio público e certificado automático

Melhor opção quando há domínio e portas 80 e 443 alcançáveis. O proxy obtém e renova o certificado de autoridade pública sem intervenção após a primeira inicialização.

### Serviços

- **Servidor:** porta 49374 exposta somente na rede interna do contêiner, sem publicação no host. Usa o arquivo de variáveis de produção, fora do controle de versão, com `AI_MEMORY_AUTH_TOKEN`, `AI_MEMORY_ALLOWED_HOSTS` e as credenciais do provedor de LLM. A imagem define verificação de saúde pelo comando `status`.
- **Proxy:** publica 80 e 443. Monta o arquivo de configuração do proxy e os volumes de dados (certificado e chave da conta ACME, que devem ser copiados em backup) e de configuração. Depende do servidor na ordem de inicialização.
- **Volume de dados do servidor:** persistente.

Imagem: `<imagem-ai-memory>:latest`. Substitua pelo nome da imagem publicada pelo seu canal de distribuição.

### Configuração do proxy

```caddyfile
memory.example.com {
    reverse_proxy ai-memory:49374
}
```

Com essa configuração, o proxy:

1. Resolve o desafio HTTP-01 no primeiro acesso ao hostname.
2. Emite o certificado.
3. Renova automaticamente antes do vencimento. A confirmar: antecedência de renovação, que é comportamento do proxy e não está no repositório.

### Arquivo de variáveis do servidor

```bash
AI_MEMORY_AUTH_TOKEN=...token-longo-gerado-com-generate-auth-token...
AI_MEMORY_AUTH__SECURE_COOKIE=true
AI_MEMORY_ALLOWED_HOSTS=memory.example.com,localhost,127.0.0.1
AI_MEMORY_BIND=0.0.0.0:49374
```

Na imagem de contêiner, o comando padrão já passa `--bind 0.0.0.0:49374`. A variável `AI_MEMORY_BIND` é redundante e não tem efeito enquanto esse comando estiver em uso. Mantê-la é inofensivo, mas não substitui o comando.

`AI_MEMORY_ALLOWED_HOSTS` deve incluir o hostname público, ou a verificação de Host recusa as requisições encaminhadas pelo proxy.

### Riscos

- **Porta 80 inacessível da internet:** o desafio falha e o proxy registra erro de emissão. Solução: encaminhar 80 e 443 do roteador para o host do proxy, ou usar o caminho com túnel de saída.
- **DNS sem propagação:** a primeira emissão falha com erro de nome sem endereço. Solução: aguardar a propagação ou conferir se o registro A aponta para o IP público.
- **Renovação falha silenciosamente meses depois:** o proxy registra o erro, mas ninguém lê o log. Solução: monitorar os logs do proxy ou usar verificação de saúde externa.

## Caminho 2: proxy com autoridade interna (somente rede local)

Para uso somente na rede local, sem domínio público e sem exposição à internet. A autoridade interna gera a raiz, que é instalada **uma vez** em cada cliente. Não há dependência da internet nem encaminhamento de porta.

### Configuração do proxy

```caddyfile
{
    local_certs
}

homelab.local, 192.168.1.50 {
    reverse_proxy ai-memory:49374
}
```

No endereço do site, liste todos os nomes e IPs que os clientes usarão (navegador, cliente MCP, curl). O proxy os coloca no certificado como nomes alternativos.

Na autoridade interna, a porta 80 não é necessária, porque não há desafio ACME. Não a publique.

### Instalação da raiz

Ver a seção [Certificados e confiança](#certificados-e-confiança).

### Variáveis e clientes

Igual ao caminho 1, trocando o domínio pelo nome usado no certificado (ex.: `https://homelab.local`).

## Caminho 3: túnel de saída

O túnel é iniciado pelo lado do servidor e conecta de saída à borda do provedor. Não é preciso abrir portas no roteador nem ter IP público. O TLS termina na borda do provedor, com certificado de autoridade pública. Não há instalação de raiz nos clientes.

### Configuração única no provedor (exemplo: Cloudflare Tunnel)

1. Ter um domínio gerenciado pelo provedor (o registrador pode ser outro, mas o DNS precisa estar no provedor).
2. No painel do provedor, criar um túnel com nome descritivo e salvar.
3. Copiar o token do túnel.
4. Adicionar um hostname público ao túnel, com destino `http://ai-memory:49374`.
5. Opcional, mas recomendado: colocar o hostname atrás de uma aplicação de acesso zero trust (SSO do provedor). Isso adiciona autenticação humana **antes** do token bearer do ai-memory. As duas camadas precisam passar. Se a camada do provedor bloquear uma requisição, o ai-memory nunca a recebe.

### Serviços

- **Servidor:** igual ao caminho 1, sem publicação de porta no host. O túnel alcança o servidor pela rede interna.
- **Cliente do túnel:** imagem oficial do túnel, com o comando `tunnel --no-autoupdate run` e o token em `TUNNEL_TOKEN`, lido de `CLOUDFLARE_TUNNEL_TOKEN` no ambiente. Sem portas publicadas no host.

### Arquivo de variáveis do servidor

```bash
AI_MEMORY_AUTH_TOKEN=...token-longo...
AI_MEMORY_AUTH__SECURE_COOKIE=true
AI_MEMORY_ALLOWED_HOSTS=memory.example.com,localhost,127.0.0.1
AI_MEMORY_BIND=0.0.0.0:49374
CLOUDFLARE_TUNNEL_TOKEN=...token-do-painel...
```

`AI_MEMORY_ALLOWED_HOSTS` deve incluir o hostname público que o provedor encaminha, ou a verificação de Host recusa as requisições do túnel.

### Clientes

Igual ao caminho 1:

```bash
ai-memory install-mcp   --client claude-code --apply \
    --server-url "https://memory.example.com/mcp" --auth-token "$AI_MEMORY_AUTH_TOKEN"
```

### Riscos

- **Vazamento do token do túnel.** Quem tem o token pode executar um túnel para o hostname. Mantenha o arquivo de variáveis com permissão restrita (ex.: `0600`) e fora do controle de versão.
- **Túnel indisponível ou DNS em cache antigo.** O provedor pode responder com erro de gateway por alguns minutos após reinício. Normalmente se recupera sozinho. A confirmar: duração típica, que é comportamento do provedor.
- **Política de acesso confundida com token.** A camada de acesso do provedor é separada do token bearer. As duas precisam passar.
- **Token recusado pelo provedor.** O túnel encerra com erro e o gerenciador de processos reinicia em loop. Confira os logs do túnel na primeira inicialização para confirmar o registro.

Instalação nativa do túnel como serviço do sistema operacional: a confirmar. Não há comando de instalação nativo no repositório.

## Caminho 4: arquivos de certificado externos

Para quem já tem uma autoridade corporativa ou doméstica emitindo certificados para os serviços, e não quer que o proxy emita o seu próprio.

Configuração do proxy:

```caddyfile
memory.example.com {
    tls /etc/caddy/certs/memory.crt /etc/caddy/certs/memory.key
    reverse_proxy ai-memory:49374
}
```

Monte o diretório de certificados como somente leitura no serviço do proxy. O proxy recarrega o certificado quando os arquivos mudam, sem reinício. A confirmar: comportamento de recarga, que é do proxy.

Equivalente em nginx:

```nginx
server {
    listen 443 ssl http2;
    server_name memory.example.com;

    ssl_certificate     /etc/nginx/certs/memory.crt;
    ssl_certificate_key /etc/nginx/certs/memory.key;

    location / {
        proxy_pass http://ai-memory:49374;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        # Transporte MCP Streamable HTTP: HTTP/1.1 e Connection vazio.
        proxy_http_version 1.1;
        proxy_set_header Connection "";
    }
}
```

Os cabeçalhos `X-Forwarded-Proto` e `X-Forwarded-For` são opcionais para o servidor. O servidor não usa `X-Forwarded-Proto`. `X-Forwarded-For` só tem efeito com `trusted_proxy_cidrs` configurado.

## Proxy nativo (sem contêiner)

Para quem roda o ai-memory a partir da fonte ou de um gerenciador de pacotes, sem contêiner. O servidor permanece em `127.0.0.1:49374`, e o proxy é instalado pelo gerenciador de pacotes do sistema operacional:

```caddyfile
memory.example.com {
    reverse_proxy 127.0.0.1:49374
}
```

Como proxy e servidor estão no mesmo host, o bind de loopback basta. As regras de inicialização não pedem `0.0.0.0`. Os caminhos de certificado valem da mesma forma, porque o proxy não depende de onde o servidor roda.

Não é preciso usar `AI_MEMORY_BIND=0.0.0.0` neste caso.
