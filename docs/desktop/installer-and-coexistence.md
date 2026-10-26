# Instalação, atualização e convivência com instalações existentes

Este documento descreve como uma instalação nativa do ai-memory é organizada, como é atualizada e removida, como convive com outras instalações e com aplicativos de agentes de terceiros, e quais requisitos uma futura aplicação de bandeja (menu do sistema) deve atender.

Comportamentos de aplicativos de terceiros que não foram verificados no projeto aparecem como "a confirmar" ou não aparecem.

## Termos

- **Instalação nativa**: executável e diretório de hooks lado a lado, em um prefixo gravável pelo usuário.
- **Instalação por contêiner**: o wrapper de Docker do projeto. Não é atualizada pelo comando de atualização nativo.
- **Diretório de dados**: onde ficam a memória persistente, a configuração, os logs, os modelos, a fila de eventos e os artefatos de hooks.
- **Hooks**: integrações que os agentes executam em eventos (início de sessão, prompt, uso de ferramenta, fim de sessão) e que enviam eventos ao servidor.

## 1. Artefatos de release

Cada release publica um arquivo por sistema e arquitetura:

| Sistema e arquitetura | Arquivo |
|---|---|
| Linux x86_64 | `ai-memory-linux-x86_64.tar.gz` |
| Linux aarch64 | `ai-memory-linux-aarch64.tar.gz` |
| macOS aarch64 | `ai-memory-macos-aarch64.tar.gz` |
| macOS x86_64 | `ai-memory-macos-x86_64.tar.gz` |
| Windows x86_64 | `ai-memory-windows-x86_64.zip` |

Não há artefato para Windows aarch64.

Cada arquivo traz:

- o executável (`ai-memory`; no Windows, `ai-memory.exe`);
- um diretório `hooks/` com os scripts de hook por agente;
- arquivos de apoio: licença, README, modelo de configuração, documentação de instalação e modelos de serviço (variáveis de ambiente, LaunchAgent, unidades systemd, usuário de sistema e tmpfiles).

Os arquivos de apoio são validados e ignorados pela atualização. Qualquer entrada fora dessa lista faz a extração falhar.

## 2. Layout da instalação nativa

- O executável e o diretório `hooks/` ficam no mesmo prefixo. A atualização troca o executável e, se o `hooks/` irmão já existir, também o substitui.
- Os scripts de hook que os agentes executam não ficam no prefixo do binário. Ficam no diretório de dados (seção 3).
- Substituir o executável sozinho não reescreve as configurações dos agentes. A atualização, porém, reaplica os hooks dos agentes detectados (seção 5.2, passo 11), o que pode regravar essas configurações.

## 3. Diretório de dados

### 3.1 Resolução

O diretório de dados é resolvido nesta ordem:

1. opção `--data-dir` da CLI;
2. variável de ambiente `AI_MEMORY_DATA_DIR`;
3. padrão da plataforma:
   - **macOS**: `~/Library/Application Support/ai-memory`;
   - **Linux**: `$XDG_DATA_HOME/ai-memory`; se a variável estiver ausente ou vazia, `~/.local/share/ai-memory`;
   - **Windows**: `%LOCALAPPDATA%\ai-memory`; se a variável estiver ausente, `~/AppData/Local/ai-memory`.

Se a plataforma não informar diretório local de dados, o padrão de configuração cai no diretório atual. Um caminho com prefixo de caminho longo do Windows é normalizado antes de ser usado pelos hooks.

### 3.2 Conteúdo

| Item | Papel | Preservado por purge (uninstall com `--purge-data`) |
|---|---|---|
| `config.toml` | Configuração. Sua ausência indica primeira execução, e o companion de macOS roda a inicialização nesse caso. | Sim |
| `wiki/`, `db/`, `raw/` | Memória persistente. | Não (são apagados e recriados vazios) |
| `logs/` | Logs. | Sim |
| `models/` | Modelos de embedding baixados. | Sim |
| Fila de eventos de hooks | Eventos que não puderam ser entregues ao servidor. São reenviados depois. A reentrega é idempotente pela chave de ingestão. | Sim |
| `auth-token` | Token de autenticação lido pelos hooks. | Removido ao desinstalar hooks |
| `servers.toml` e `auth-tokens/<perfil>` | Registro de perfis de servidor e um token por perfil (seção 7.3). | Registro mantido; tokens removidos ao desinstalar |
| `hooks/<agente>/` | Scripts de hook por agente, copiados do pacote. | Sim |
| `backups/claude-settings-local/` | Cópias de segurança de configurações de projeto antes de alterações. | Sim |

Se a configuração do companion de macOS tiver um diretório de dados sobrescrito, o caminho escolhido é gravado apenas na variável de ambiente do serviço e nas chamadas do próprio companion.

## 4. Endereços e padrões de rede

- Bind padrão do servidor: `127.0.0.1:49374`.
- URL padrão do servidor: `http://127.0.0.1:49374`.
- URL padrão do endpoint MCP: `http://127.0.0.1:49374/mcp`.
- Hosts aceitos por padrão no cabeçalho `Host`: `localhost`, `127.0.0.1` e `::1`. Essa lista é a defesa contra reassociação de DNS (DNS rebinding). Para acesso pela rede local, os hosts devem ser adicionados explicitamente.
- Autenticação: o token vem de `AI_MEMORY_AUTH_TOKEN` ou da seção de autenticação do `config.toml`. Quando há token, os hooks enviam `Authorization: Bearer`.
- A ausência de token no servidor local padrão é a confirmar.

Uma URL de servidor é considerada local quando contém `127.0.0.1`, `localhost`, `[::1]` ou `0.0.0.0`. Qualquer outra é tratada como remota.

## 5. Atualização (`upgrade`)

### 5.1 Escopo

A atualização troca somente a instalação nativa local. Nunca atualiza um servidor remoto. Instalações por contêiner e instalações gerenciadas por gerenciador de pacotes são recusadas.

### 5.2 Fluxo

1. **Localizar o executável.** O caminho é resolvido seguindo links simbólicos. Se não for possível resolver, usa-se o caminho original.
2. **Classificar a instalação.** As recusas são verificadas nesta ordem:
   - a) execução dentro de contêiner: a mensagem orienta a rodar a atualização no wrapper do host ou a trocar a imagem com um pull;
   - b) caminho que começa com um dos prefixos gerenciados: `/opt/homebrew/`, `/usr/local/Cellar/`, `/home/linuxbrew/.linuxbrew/`, `/usr/bin/`, `/bin/`, `/sbin/`, `/usr/sbin/` e `/nix/store/`. A mensagem orienta a usar o gerenciador de pacotes. A lista contém apenas caminhos Unix; equivalentes para Windows são a confirmar;
   - c) diretório pai do executável não gravável. A verificação cria e remove um arquivo de sondagem. A mensagem orienta a instalar em um prefixo do usuário, como `~/.local`. Só o diretório pai precisa ser gravável, porque a troca é feita por renomeação.
3. **Resolver a versão alvo.**
   - Com a opção de versão fixada, a tag é aceita com ou sem `v` inicial. Valor vazio é recusado. A tag recebe `v` quando não o tem.
   - Sem versão fixada, a tag é a da última release, lida do campo `tag_name` da API do GitHub.
   - A base de releases pode ser trocada pela configuração `release_base_url` (valor vazio é ignorado). Com uma base que não é do GitHub, a tag vem do endpoint `{base}/latest/tag`, que devolve texto puro.
4. **Comparar com a versão instalada.** Os prefixos `v` e espaços são ignorados. Se as versões forem iguais, a atualização informa que já está atualizada e termina com sucesso, a menos que a opção de forçar seja usada.
5. **Baixar** o arquivo da release em `{base}/download/{tag}/{arquivo}` e o checksum em `{arquivo}.sha256`.
6. **Verificar o checksum.**
   - A primeira linha não vazia e que não começa com `#` deve ter um hash SHA-256 de 64 caracteres hexadecimais.
   - Se a linha trouxer um nome de arquivo, ele deve ser igual ao nome do arquivo baixado. Um `*` inicial é ignorado.
   - A comparação ignora maiúsculas e minúsculas. Divergência aborta a atualização sem substituir nada.
7. **Extrair em diretório temporário**, com estas regras:
   - entradas aceitas: o executável e a árvore `hooks/`;
   - entradas de apoio listadas são validadas e ignoradas;
   - qualquer outra entrada, links simbólicos ou físicos, tipos especiais, caminhos absolutos e caminhos com `..` fazem a extração falhar;
   - a entrada de raiz `./` é tolerada;
   - limite de 4096 entradas e de 512 MiB de bytes descompactados. Ao ultrapassar, a extração falha;
   - o pacote precisa conter o executável, ou a atualização falha.
8. **Limites de download.** Cada requisição tem timeout de 120 segundos. O corpo é limitado a 128 MiB. O limite é checado primeiro pelo tamanho declarado e depois durante a leitura, porque o tamanho declarado pode ser falso ou ausente. Resposta fora da faixa 2xx falha.
9. **Substituir o executável.**
   - Em Unix: o novo binário é copiado para `<nome>.new` ao lado do atual, recebe permissão de execução `755` e é renomeado sobre o original.
   - No Windows: o executável em uso é movido para `<nome>.old`, o `.new` é promovido, e o `.old` é apagado por melhor esforço. Se a promoção falhar, o `.old` é restaurado. A remoção do `.old` pode falhar enquanto o processo estiver ativo (a confirmar o comportamento).
   - Se a troca falhar no meio, a mensagem de erro aponta os arquivos `.new` e `.old` deixados para trás.
10. **Substituir o `hooks/` irmão.** Só acontece quando o pacote traz `hooks/` e já existe um `hooks/` ao lado do executável. Não cria diretório que não existia. A troca copia para um diretório `.hooks.new`, move o atual para `.hooks.old` e renomeia. Em falha, restaura o anterior.
11. **Reaplicar hooks dos agentes.**
    - A raiz de staging é `<diretório de dados>/hooks`. Dentro de contêiner, a raiz é o diretório local padrão do usuário, porque o diretório de dados do servidor é um volume que o host não consegue executar.
    - Se a raiz não existir, a atualização informa que não há nada a atualizar e termina com sucesso.
    - Os diretórios `lib` e os que começam com `_` são ignorados, pois contêm auxiliares e não agentes. Nomes desconhecidos são ignorados com aviso.
    - Para cada agente reconhecido, reaplica-se a instalação de hooks com o escopo global e sem servidor ou token explícitos. Uma falha de agente é reportada e não interrompe os demais.
    - Caso especial do Claude Code: se o arquivo de configuração de usuário não existir ou não tiver hooks do ai-memory, só os scripts são restagiados. A atualização não cria uma instalação de usuário que o operador não pediu. Se o arquivo não puder ser lido ou interpretado, a reaplicação segue e reporta o erro.
    - Se a reaplicação falhar, a mensagem orienta a rodar de novo com o mesmo servidor e token usados originalmente. Se a reaplicação sem servidor explícito preserva a configuração anterior, a confirmar.
12. **Aviso de servidor remoto.** Se a URL do servidor configurada for remota (seção 4), a atualização imprime uma nota: só o binário e os hooks locais foram atualizados, e o servidor remoto deve ser reimplantado separadamente.
13. **Concluir** com a mensagem da versão aplicada.

Opções: `--version` fixa a tag. `--force` baixa e substitui mesmo com a versão já instalada.

### 5.3 Wrapper de contêiner

O wrapper de contêiner mantém um cliente nativo no diretório de dados do usuário, em Linux e macOS: `$XDG_DATA_HOME/ai-memory/native-runner`, com `~/.local/share/ai-memory/native-runner` como padrão. O diretório `hooks/` do pacote fica ao lado do cliente. Alguns agentes têm seus comandos de hook apontando para esse cliente. Esse cliente não fica no diretório de cache.

Esse caminho não é o destino dos hooks de uma instalação nativa, que ficam em `<diretório de dados>/hooks/<agente>` (seção 3.2).

## 6. Desinstalação (`uninstall`)

### 6.1 Comportamento geral

- Sem `--apply`, o comando só imprime o plano de remoção (dry-run). Com `--apply`, aplica.
- `--only` aceita `hooks`, `mcp`, `instructions` ou `skills`. Sem a opção, os quatro escopos são processados.
- Quando há terminal e `--yes` não foi usado, o comando pede confirmação. Só `y` ou `yes`, sem diferenciar maiúsculas, continua. Qualquer outra resposta aborta.
- Remove somente o que pertence ao ai-memory. Nunca remove entradas de terceiros.
- Não remove o executável nem o diretório de dados, a menos que `--purge-data` seja usado.
- Não executa comandos de Docker. Apenas imprime os passos manuais: derrubar a composição com volumes, remover o volume padrão e remover o wrapper, se instalado.
- Quando os dados não são purgados, imprime como apagá-los com o comando de reset.
- Ao remover hooks, imprime uma nota: cada outro checkout que usou hooks de escopo de projeto precisa ser desinstalado separadamente com `--only hooks`, porque o arquivo local desse checkout continua chamando o binário e o diretório de dados.

### 6.2 Critérios de propriedade

- **Hooks**: um comando é do ai-memory quando tem os tokens `hook`, `--event`, `--agent` e `--server-url` e referencia o ai-memory. Em entradas aninhadas, só os comandos próprios são removidos. A entrada sobrevive se restarem comandos de terceiros. Formatos antigos de cada agente também são reconhecidos.
- **Servidor MCP**: a entrada só é removida quando a URL corresponde ao endpoint informado, em campos `url`, `httpUrl` ou `serverUrl`, ou quando é uma ponte de stdio que contém essa URL. Um nome igual não basta. Se a opção de nome for usada, nome e URL precisam coincidir. O endpoint padrão é `http://127.0.0.1:49374/mcp`. A opção `--mcp-url` tem o alias `--server-url`. Alguns clientes gravam variantes de URL com sufixo de esquema, e essas variantes também são reconhecidas.
- **Instruções em arquivos de agente** (como `CLAUDE.md` e `AGENTS.md`): remove o bloco entre os marcadores `<!-- ai-memory:start -->` e `<!-- ai-memory:end -->`, inclusive a quebra de linha seguinte. Marcadores são reconhecidos só quando ocupam a própria linha. Sobras de versões antigas também são removidas. Instalar e depois desinstalar devolve o arquivo ao estado original.
- **Arquivos gerados** (integrações de agentes): só são removidos quando contêm o cabeçalho de geração do ai-memory e a constante do agente correspondente. Arquivos de auto-ligação vazios são identificados por sentinela.
- **Skills**: diretórios de skill que ficam vazios após a remoção do arquivo são apagados.

### 6.3 Credenciais

- Ao remover hooks, o token armazenado no diretório de dados é removido por melhor esforço. Uma falha só gera aviso. O motivo é que sem hooks o token ficaria órfão e vivo.
- Os tokens de perfis de servidor são sempre removidos. O registro de perfis (URLs) permanece.
- Com `--only mcp`, `instructions` ou `skills`, o token de hooks é mantido, porque os hooks continuam lendo-o.

### 6.4 Purge

- Quando `--purge-data` é pedido, o comando verifica processos ai-memory ativos antes de qualquer alteração. Se houver algum, recusa e não altera nada. A remoção só de fiação (sem purge) não faz essa verificação.
- Apaga `wiki/`, `db/` e `raw/` e os recria vazios. Subdiretórios ausentes são ignorados. `logs/` e `models/` são preservados.
- A remoção não é atômica: entre apagar e recriar, uma falha pode deixar um subdiretório faltando. Isso é uma limitação conhecida, igual à do reset.

### 6.5 Cópias de segurança

- Alterações em configurações de projeto do Claude Code guardam a cópia anterior no diretório de dados, em `backups/claude-settings-local`. Não ficam no repositório, para não aparecerem como arquivos não rastreados.
- Alterações em configurações globais guardam uma cópia ao lado do original, antes de cada escrita que muda o conteúdo (confirmado na auditoria de 2026-10-25): o nome é `<caminho>.bak-<segundos Unix>`, formado a partir do caminho pedido (se ele for um link simbólico, a escrita segue o link, mas o nome da cópia usa o caminho do link). Uma reexecução que não muda nada não grava nem copia. Duas escritas no mesmo segundo reusam o mesmo nome, e a segunda cópia substitui a primeira. Arquivo inexistente é criado sem cópia. Cadeias de links com mais de 40 saltos são recusadas.

## 7. Convivência com instalações existentes

### 7.1 Um único escritor por diretório de dados

- O banco de dados tem escritor único. Todas as escritas passam por um canal para uma única thread dedicada.
- Dois servidores sobre o mesmo diretório de dados não devem coexistir. **Há bloqueio explícito** (confirmado na auditoria de 2026-10-25): o servidor toma uma trava exclusiva em `.serve.lock` no diretório de dados e grava o detentor (`pid=<n>`) em `.serve.lock.holder`. Um segundo servidor recusa a subida citando o detentor, salvo com `--force`; um sistema de arquivos sem suporte a trava só gera aviso. Detalhes e textos em [cli.md](../cli.md) e [windows.md](../windows.md).
- Operações que mexem em disco diretamente (reset, restore, reindex e uninstall com purge) verificam processos ativos antes de agir.

### 7.2 Descoberta de servidor (requisito para a bandeja)

Uma aplicação de bandeja deve detectar um servidor existente antes de criar outro. A ordem proposta é:

1. consultar o estado do servidor configurado ou padrão com `GET /admin/status`. O companion de macOS usa timeout de 3 segundos;
2. consultar o registro de perfis (seção 7.3). Hooks de repositórios podem apontar para um servidor da rede local, e não para `localhost`;
3. só se nenhum servidor responder, oferecer (a) um serviço local gerenciado ou (b) conexão a um servidor remoto, informando URL e token.

### 7.3 Perfis de servidor

Perfis permitem que cada repositório envie eventos de hook a um servidor registrado localmente.

- **Registro**: arquivo `servers.toml` no diretório de dados, com uma tabela `servers`. Cada perfil tem:
  - `url`: `http://` ou `https://` com host. A barra final é removida;
  - `roots` (opcional): lista de diretórios absolutos ou começando com `~/`.
- **Seleção**: o repositório escolhe o perfil com `server = "<nome>"` no marcador `.ai-memory.toml`.
- **Nome**: de 1 a 64 caracteres, letras minúsculas, dígitos, `-` ou `_`. Deve começar com letra minúscula ou dígito. Nomes reservados de dispositivo do Windows são recusados.
- **Limites do registro**: no máximo 64 KiB. Chaves desconhecidas são erro. Um registro inválido faz o roteamento recusar, nunca adivinhar.
- **Tokens**: um arquivo por perfil em `auth-tokens/<nome>`, com permissão restrita ao dono no Unix. Os tokens não ficam no registro, na configuração do hook nem em argumentos de processo.
- **Resolução**: qualquer falha descarta o evento. Não há fallback para o servidor da instalação, porque isso entregaria a captura de uma equipe ao servidor de outra.

| Motivo de recusa | Quando ocorre |
|---|---|
| nome de perfil inválido | o nome do marcador não obedece às regras |
| registro inválido | arquivo ilegível, maior que o limite ou fora do formato |
| perfil desconhecido | o nome não está registrado |
| sem token | o perfil não tem token armazenado |
| `roots` obrigatório | há mais de um perfil e este não declara `roots` |
| fora dos `roots` | o marcador está fora de todos os `roots` do perfil |
| marcador ilegível | um marcador no caminho existe mas não pode ser lido |

- Um perfil sem `roots` é aceito quando é o único registrado.
- A comparação de `roots` é por componente de caminho. `~/work/team-b` aceita `~/work/team-b/repo`, mas não `~/work/team-bb`.
- A busca de URL e token para o trabalho de backfill não repete a verificação de `roots`, porque o hook já validou o marcador que originou o trabalho.
- O comando de registro preserva comentários e os outros perfis. Valida o arquivo inteiro antes de gravar.

### 7.4 Hooks e aplicativos

- Os scripts de hook ficam no diretório de dados, não no pacote do aplicativo. Substituir o aplicativo não reescreve nem quebra a captura.
- Uma aplicação de bandeja não deve mover nem duplicar esse caminho. Atualizar os scripts só com consentimento do usuário, por meio da atualização (seção 5) ou da instalação de hooks.
- Agentes sem scripts usam integrações geradas, que são gravadas no lugar do agente e removidas pela desinstalação segundo a seção 6.2.

### 7.5 Versões diferentes

- O servidor define o esquema. Migrações rodam no escritor único do servidor.
- Um cliente de bandeja fala apenas HTTP (estado, interface web e API), para tolerar servidores de outras versões.
- Um cliente nunca sobrescreve o executável que está no `PATH` nem cria links simbólicos sobre ele. O companion de macOS segue essa regra: o serviço aponta para o binário dentro do próprio aplicativo.
- A atualização de um cliente nunca atualiza um servidor remoto (seção 5.1).

### 7.6 Configuração de clientes MCP

- A instalação de MCP é feita por cliente. A escrita é idempotente (repetir a instalação não duplica entradas) e preserva entradas não relacionadas.
- Para o Claude Desktop no Windows, há uma variante empacotada (MSIX) em `%LOCALAPPDATA%\Packages`. Um pacote que já tenha arquivo de configuração é preferido. Sem arquivo, um único pacote basta. Com vários pacotes e nenhum configurado (ou mais de um configurado), a instalação falha com um erro que lista os pacotes e pede o caminho explícito do arquivo `LocalCache\Roaming\Claude\claude_desktop_config.json` do pacote ativo (confirmado na auditoria de 2026-10-25; regra completa em [design-backup-agent-assets.md](../design-backup-agent-assets.md) §2). Veja também [Claude Desktop](claude-desktop.md).
- Uma aplicação de bandeja deve reutilizar essa lógica e mostrar uma lista de aplicativos detectados (requisito proposto).
- Clientes aceitos pela instalação de MCP (confirmado na auditoria de 2026-10-25; nomes de opção e apelidos): `claude-code`, `codex`, `open-code` (`opencode`), `opencode2` (`opencode-v2`, `open-code2`), `cursor`, `claude-desktop`, `gemini-cli`, `openclaw`, `pi`, `omp` (`oh-my-pi`), `antigravity-cli` (`antigravity`, `agy`), `zero`, `zcode`, `devin`, `grok`, `kimi-code` (`kimi`), `kiro-cli` (`kiro`), `command-code` (`commandcode`, `cmdc`, `cmd`), `ibm-bob` (`bob`), `copilot-cli`, `droid` (`factory-droid`), `swival`, `vscode-copilot` (`copilot`, `github-copilot`), `zed`, `muse` (`muse-code`, `musecode`), `dsh` (`deepseek-harness`; só imprime, recusa aplicar) e `prime-agent`. **O Zed está incluído**, como cliente só MCP (`context_servers` no `settings.json` do usuário).

## 8. Pacotes e serviços por sistema

### 8.1 macOS

- **Companion**: aplicativo de barra de menu, sem ícone no Dock. Empacota o executável, a pasta `hooks/` e o modelo de LaunchAgent dentro de `Contents/Resources/runtime/`. Na primeira versão não é notarizado, não usa certificado Developer ID e não tem cask do Homebrew.
- **Validade do runtime**: o pacote é válido quando o executável é executável, a pasta `hooks/` existe e o modelo de LaunchAgent existe. A variável `AI_MEMORY_MENU_RUNTIME` tem prioridade sobre o pacote e serve para desenvolvimento.
- **LaunchAgent**:
  - instalado em `~/Library/LaunchAgents/` com o rótulo reverso do projeto; o nome do arquivo segue o rótulo;
  - o modelo tem dois marcadores, para o caminho do executável e para o diretório pessoal;
  - a execução é iniciada no login (`RunAtLoad`) e reiniciada se cair (`KeepAlive`);
  - `ProcessType` é `Interactive`. Sem isso, o launchd limita CPU e E/S, o que o orçamento de ingestão de hooks não suporta;
  - argumentos: `serve`, `--transport http`, `--enable-web`;
  - logs em `~/Library/Logs/ai-memory/stdout.log` e `stderr.log`. O launchd cria os arquivos, mas não o diretório, e o companion cria o diretório antes.
- **Diretório de dados sobrescrito**: se houver um, o companion inclui `AI_MEMORY_DATA_DIR` nas variáveis de ambiente do serviço, com escape de XML. Sem sobrescrita, o serviço usa o padrão da seção 3.1.
- **Ações**:
  - *Instalar e iniciar*: inicializa o diretório de dados se `config.toml` não existir; se o serviço estiver rodando, para-o; grava o modelo renderizado; registra e inicia o serviço no domínio do usuário.
  - *Iniciar*: grava e registra o serviço.
  - *Parar*: remove o serviço do domínio.
  - *Reiniciar*: se o caminho do executável no plist instalado for diferente do executável atual do pacote, para, regrava e registra. Caso contrário, reinicia o serviço já registrado.
- **Estados do serviço**: *não instalado* (sem plist e sem registro), *parado* e *executando* (a saída do status contém `state = running`).
- **Estado "iniciando"**: dura até 45 segundos após uma ação. A consulta de estado roda a cada 1 segundo nesse período e a cada 15 segundos fora dele. Uma ação com erro encerra o estado "iniciando".
- **Saúde**: o ícone fica verde quando `GET /admin/status` responde com sucesso, com timeout de 3 segundos.
- **Ações de conveniência**: abrir a interface web (`/web`), mostrar o status (executa o executável empacotado), abrir o `config.toml`, abrir o diretório de dados e abrir os logs.
- **Atualização**: substituir o aplicativo não toca no diretório de dados. Se o caminho do executável dentro do pacote mudou, reiniciar regrava o plist. Recomenda-se instalar em `/Applications` para um caminho estável.
- O companion não substitui o comando de status, a interface web nem a edição manual do `config.toml`.

### 8.2 Linux

- **Artefato**: arquivo `tar.gz` (seção 1).
- **Serviço de usuário**: executa o servidor com `--data-dir` em `~/.local/share/ai-memory` e `--config` em `~/.config/ai-memory/config.toml`. Carrega variáveis de `~/.config/ai-memory/env`, mas não falha se o arquivo não existir. Reinicia em falha, com espera de 5 segundos. Sem privilégios novos e com diretório temporário privado. Instalado para o alvo padrão do usuário.
- **Serviço de sistema**: roda com um usuário dedicado `ai-memory`, dados em `/var/lib/ai-memory` e configuração em `/etc/ai-memory`.
- **Pacotes de distribuição**: receitas para AUR (versão de build e versão de binário), especificação de RPM, modelo de variáveis de ambiente, usuário de sistema e tmpfiles. Não há pacote `.deb` no projeto (a confirmar).
- **Flatpak**: não há artefato no projeto. A viabilidade não foi avaliada; a questão de acesso a um servidor local e às configurações dos agentes segue em aberto (seção 9).

### 8.3 Windows

- **Artefato**: arquivo zip com o executável e os hooks (seção 1). Também há wrappers de linha de comando para Windows.
- **Hooks**: quando o caminho do executável vem entre aspas, o comando gravado para Windows usa o operador de chamada do shell do sistema. Sem ele, o hook falha e nenhum evento é capturado. O histórico de mudanças registra essa correção para pelo menos um agente.
- **Não há no projeto**: instalador NSIS ou MSI, pacote MSIX próprio, publicação no winget ou na Microsoft Store, nem assinatura de código. Tudo isso é a confirmar e está fora do escopo atual.
- **Atualização**: usa a estratégia de renomeação descrita na seção 5.2, passo 9.

### 8.4 Contêiner

- O wrapper de Docker é uma alternativa à instalação nativa. Não é atualizado pela atualização nativa (seção 5.1).
- O guia de implantação do contêiner e os passos de desinstalação manual estão descritos na seção 6.1.

## 9. Pesquisa: aplicativo de bandeja (proposta, não implementado)

Um aplicativo de bandeja permitiria que um usuário não técnico instale e controle o ai-memory. Hoje só existe o companion de macOS (seção 8.1).

### 9.1 Opções de implementação

- **Opção A: runtime nativo com interface em webview.** Inclui o executável do ai-memory como processo auxiliar e usa recursos de aplicativo de desktop: instância única, início automático, atualização e links profundos. Custo: uma webview e dependências adicionais. As capacidades das ferramentas de desktop escolhidas são a confirmar.
- **Opção B: interface nativa leve.** Um só binário, sem webview, com menu e uma tela de configuração. Custo: o projeto teria de implementar por conta própria instância única, atualização, assinatura, empacotamento e links profundos.
- **Opção C: manter o companion de macOS e criar uma contraparte para Linux.** Duas bases de código para um produto. O companion de macOS ainda não atende ao critério de distribuição para usuários não técnicos, porque a notarização e o cask estão pendentes.

**Recomendação de pesquisa**: a opção A se o objetivo for um aplicativo multiplataforma que absorva o papel do companion de macOS; a opção B se o escopo for só menu e uma tela. A decisão fica em aberto.

### 9.2 Requisitos já definidos

- Detectar e reutilizar o diretório de dados existente. Nunca criar um segundo em silêncio.
- Seguir a ordem de descoberta de servidor (seção 7.2).
- Não mover nem duplicar o caminho dos scripts de hook (seção 7.4).
- Não sobrescrever o executável do `PATH` (seção 7.5).
- Gravar configurações de forma idempotente e preservar entradas não relacionadas (seção 7.6).
- Não ser um console de administração. O aplicativo abre a interface web e o arquivo de configuração, sem duplicá-los.

### 9.3 Questões em aberto

1. Escopo da interface: só menu, ou também telas de configuração e visualizador de conversas.
2. Viabilidade de Flatpak: exige portais para o serviço em segundo plano e para o acesso às configurações dos agentes. A alternativa é publicar apenas pacotes diretos (AppImage, pacote de distribuição e AUR).
3. Política de atualização do binário embutido, separada da atualização do aplicativo e da instalação gerenciada pela CLI.
4. Se o aplicativo deve também gerenciar a instalação de hooks para os agentes detectados, como o ponto natural para ligar e desligar a captura.

## 10. Itens a confirmar

- Forma exata do comando de hook de cada agente com scripts: caminho do script ou chamada do binário.
- Se a reaplicação de hooks feita pela atualização, sem servidor e token explícitos, preserva a configuração anterior.
- Local exato das cópias de segurança de configurações globais.
- Comportamento da remoção do `.old` no Windows enquanto o processo está ativo.
- Ausência ou presença de token no servidor local padrão.
- Conteúdo de `auth.json` e de `capture-mode` no diretório de dados, citados na documentação do companion de macOS.
- Lista de clientes do MCP, incluindo a inclusão do Zed.
- Comportamento com vários pacotes MSIX do Claude Desktop e nenhum com configuração.
- Equivalentes Windows da lista de prefixos gerenciados.
- Existência de bloqueio explícito para dois servidores sobre o mesmo diretório de dados.
- Presença de pacote `.deb`, fórmula do Homebrew, instalador Windows, publicação em lojas e assinatura de código.
