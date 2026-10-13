# Superfícies de extensão e plugins

> Pesquisa sobre o que cada produto de terceiro permite empacotar (skills,
> hooks, servidores MCP) e como o ai-memory se integra a essas superfícies hoje.
> Cada afirmação sobre um produto externo que não tem evidência no código do
> projeto está marcada como **a confirmar** e não deve servir de base para
> decisão até ser testada ao vivo.

## Convenções

- **Verificado**: o comportamento está implementado no código do ai-memory.
- **A confirmar**: afirmação sobre um produto de terceiro sem evidência no
  código do projeto.

## O que o ai-memory instala

### Skills

O comando de instalação de skills grava um conjunto fixo de seis skills
gerenciadas. Verificado.

- Padrão: escopo `project` e agente `claude-code`.
- Escopo `project` grava sob o diretório atual; escopo `global` grava sob o
  diretório pessoal do usuário.
- `--target-dir` substitui escopo e agente: as skills são gravadas sob a raiz
  informada.
- `--print` mostra destinos e conteúdo sem gravar nada.

| Agente | Raiz no escopo `project` | Raiz no escopo `global` |
| --- | --- | --- |
| `claude-code` | `.claude/skills` | `$CLAUDE_CONFIG_DIR/skills` se a variável estiver definida; senão `~/.claude/skills` |
| `agents` | `.agents/skills` | `~/.agents/skills` |
| `devin` | `.devin/skills` | No Windows, `%APPDATA%/devin/skills`; nos demais sistemas, `~/.devin/skills` |
| `grok` | `.grok/skills` | `$GROK_HOME/skills` se definida; senão `~/.grok/skills` |
| `hermes` | `.hermes/skills` | `~/.hermes/skills` |
| `both` | `.claude/skills` e `.agents/skills` | `claude-code` e `agents` no escopo global |

Skills gerenciadas (cada uma é um diretório `ai-memory-<nome>` com um arquivo
de instrução):

- `ai-memory-retrieval`
- `ai-memory-handoff`
- `ai-memory-messaging`
- `ai-memory-durable-pages`
- `ai-memory-learning-maintenance`
- `ai-memory-routing-install`

Regras de sobrescrita:

- Cada arquivo gerenciado contém um marcador fixo de gerenciamento.
- Se já existe um arquivo com o mesmo caminho e o marcador não está presente,
  a instalação é recusada com erro, sem alterar nada.
- Com `--force`, o arquivo não gerenciado é substituído. O código informa que
  um backup é gravado ao lado do arquivo substituído.
- Reexecutar a instalação sobre arquivos já atualizados não os altera.

### Hooks e extensões por agente

Verificado para cada linha: o instalador grava ou mescla o arquivo indicado.

| Agente | Arquivo ou local gravado | Notas |
| --- | --- | --- |
| Claude Code | `settings.json` do usuário (`$CLAUDE_CONFIG_DIR/settings.json` ou `~/.claude/settings.json`) ou `.claude/settings.local.json` do repositório | Escopo global dispara em todos os projetos; escopo de projeto só nas sessões iniciadas naquele checkout. O arquivo de projeto é ignorado pelo Git. |
| Codex | `hooks.json` em `$CODEX_HOME`, ou `~/.codex/hooks.json` | Ver "Confiança dos hooks do Codex" abaixo. |
| Cursor | `~/.cursor/hooks.json` | Entrada do ai-memory é mesclada ao arquivo. Ver "Dupla captura no Cursor". |
| Antigravity CLI | `~/.gemini/config/hooks.json` | Grupo de hooks com nome `ai-memory`. |
| Zero | `~/.config/zero/hooks.json` (padrão) | Caminhos de XDG personalizados exigem `--config-file`. |
| OpenCode | Arquivo de plugin `ai-memory` no diretório de plugins de `~/.config/opencode/plugins/` | Duas formas de plugin: a função (versão 1) e o objeto com `id` e `setup` (versão 2). Reiniciar o OpenCode para carregar. |
| Oh My Pi (`omp`) | Extensão `ai-memory-omp` em `<diretório do agente>/extensions/` | O diretório do agente segue a ordem: perfil nomeado (`--profile`, depois `OMP_PROFILE`, depois `PI_PROFILE`); senão `PI_CODING_AGENT_DIR`; senão `~/.omp/agent`. `PI_CONFIG_DIR` renomeia `~/.omp`. Reiniciar `omp`. |
| Pi | Extensão `ai-memory-pi` em `~/.pi/agent/extensions/` | `PI_CODING_AGENT_DIR` redireciona o diretório do agente; a extensão é carregada no próximo início. |
| OpenClaw | Pacote gerado em `ai-memory/openclaw-plugin` no diretório de dados locais do usuário (ou o caminho de `--config-file`) | Grava o manifesto do pacote, o manifesto do plugin e o ponto de entrada. Com o CLI do OpenClaw no PATH, executa `openclaw plugins install --link <pacote> --force` e depois `openclaw plugins enable ai-memory`. Falha no install é erro; falha no enable é aviso. Sem o CLI, grava apenas os arquivos e imprime os comandos para instalação manual. |
| Zed (cliente MCP) | `settings.json` do Zed em `Zed` (Windows) ou `zed` (demais sistemas) dentro do diretório de configuração | Entrada MCP sob a chave `context_servers` de topo. A mesclagem preserva comentários e as demais entradas. |

Área de staging dos hooks:

- Os scripts de hook são copiados para `<diretório de dados>/hooks/<agente>`
  antes de serem referenciados pela configuração do agente.
- Dentro de contêiner, a área de staging fica no diretório de dados locais do
  usuário, porque o agente executa os hooks no host.

### Transporte dos hooks

Verificado.

- Cada agente tem um conjunto de scripts em `hooks/<agente>/` para Unix e
  Windows. Os scripts leem o JSON da entrada padrão e o enviam por POST para
  `<URL>/hook?event=<evento>&agent=<agente>`.
- A URL vem de `AI_MEMORY_HOOK_URL`; o padrão é `http://127.0.0.1:49374`.
- Os scripts respondem `{}` ao agente e não bloqueiam a sessão.
- Parâmetros de projeto (workspace, projeto e estratégia de projeto) vêm do
  marcador `.ai-memory.toml` encontrado subindo a partir do diretório atual.
- O binário nativo também atende o comando `hook` com `--event` e `--agent`,
  com as regras de descarte descritas abaixo.

### Regras específicas de agente

**Antigravity CLI.** Um evento `SessionStart` só é processado quando
`invocationNum` é `0`; os demais são descartados. O `Stop` encerra um ciclo
de execução, não a conversa: após o último turno, `finalize-session` deve
ser executado para gerar o resumo final e o handoff (o instalador imprime essa
orientação).

**Dupla captura no Cursor.** O CLI do Cursor também executa os comandos de
hook das configurações do Claude Code. Quando os hooks nativos do Cursor estão
instalados (entrada `--agent cursor` em `~/.cursor/hooks.json`), a cópia que
chega como `claude-code` com o campo `cursor_version` é descartada, para
manter uma observação por evento. Arquivo ilegível ou malformado conta como
"não instalado". Verificado.

**Confiança dos hooks do Codex.** O instalador avisa que o Codex só executa
hooks novos depois que o usuário os confirma na interface ("Trust all and
continue"), ou com a flag de bypass da confirmação. O aviso só é impresso
quando o arquivo foi alterado. Verificado no texto do instalador; o
comportamento do Codex em runtime (por exemplo, se hooks não confiáveis são
ignorados em silêncio) é **a confirmar**.

### Backup e restauração de agentes

Verificado.

- O comando `backup-agents` copia skills, plugins e instruções dos agentes
  para um arquivo portátil. As pastas de plugins coletadas são:
  - `~/.claude/plugins`;
  - `~/.codex/plugins`;
  - `~/.gemini/antigravity-cli/plugins`;
  - `~/.config/opencode/plugins`;
  - `~/.openclaw/extensions`.
- Escopo padrão `both`. Configurações MCP são redigidas por padrão;
  `--include-secrets` desativa a redação.
- Todo arquivo gerado é gravado com modo `0600` em Unix, porque instruções e
  plugins podem conter segredos.
- O comando `restore-agents` faz a restauração correspondente.

## Superfícies de terceiros (pesquisa)

### Claude

Todas as afirmações abaixo sobre o que cada superfície carrega são **a
confirmar**. O código do projeto não contém evidência delas.

- Um mesmo pacote de plugin é instalado em Chat, Cowork e Claude Code, mas cada
  superfície carrega um subconjunto diferente de componentes.
- Skills e comandos: carregam em Chat, Cowork e Claude Code.
- Hooks: ignorados no Chat; carregam no Cowork e no Claude Code.
- Servidor MCP remoto (URL fixa): listado nos conectores do plugin no Chat;
  carrega no Cowork e no Claude Code.
- Servidor MCP local (comando): ignorado no Chat; carrega no Cowork quando a
  sessão roda no computador do usuário; carrega no Claude Code.
- MCP com referência a `${user_config.*}`: ignorado no Chat quando a URL usa
  a referência; no Cowork, ignorado sem valor padrão; no Claude Code, carrega
  e pede os valores.
- Executáveis em `bin/` no topo do pacote: não instalam no Chat nem no Cowork;
  carregam no Claude Code.
- Chat e Cowork usam a interface de plugins da conta; o Claude Code instala por
  máquina.

Consequências para o ai-memory (hipóteses, todas **a confirmar**):

- O Cowork poderia ganhar um caminho de captura, se os hooks de plugin
  carregarem lá. Em Linux o Cowork roda em máquina virtual, e um comando ou
  URL de loopback que funciona no host pode não funcionar dentro dela.
- O Chat continuaria sem captura: hooks e servidores MCP locais seriam ignorados.
  Se o conector remoto alcança um servidor no computador do usuário depende de
  como o aplicativo faz a conexão, também **a confirmar**.
- Um plugin não instalaria o binário do servidor no Chat ou no Cowork. Verificado
  apenas que o instalador grava configuração e scripts de hook, e que os hooks
  enviam eventos para um servidor já em execução.
- O projeto não declara captura no Cowork. Verificado.

### Outros aplicativos

Coluna "Integração hoje" é verificada no código. Demais colunas são **a confirmar**.

| Aplicativo | Integração hoje (verificada) | Unidade de extensão e empacotamento de hooks e MCP | Caminho de instalação de terceiros |
| --- | --- | --- | --- |
| Codex (CLI e desktop) | Arquivo de hooks independente (ver confiança acima) | A confirmar | A confirmar |
| Cursor | Arquivo de hooks independente; dupla captura com Claude Code | A confirmar | A confirmar (inclui links de instalação de MCP, a confirmar) |
| Antigravity | Hooks globais da CLI; IDE a confirmar | A confirmar (empacotamento de hooks apenas na CLI, segundo a documentação do produto, a confirmar) | A confirmar |
| Zed | Entrada MCP em `context_servers` | A confirmar (extensões de MCP) | A confirmar (registro de extensões) |
| ChatGPT (chat hospedado) | Sem instalador no projeto | A confirmar (plugin apenas remoto) | A confirmar (diretório com revisão) |

Pontos a confirmar no caso do Codex: se plugins listados por um espaço de
trabalho do ChatGPT têm confiança automática para seus hooks, e se hooks de
plugins de outras origens passam pela confirmação normal por hook. Se isso for
verdade, um plugin não elimina a falha silenciosa de hook não confiável.

## Pendências (a confirmar)

1. Carregamento de hooks e MCP de plugin no Cowork, inclusive em Linux.
2. Limites do Chat para hooks e MCP local, e alcance de conectores remotos.
3. Comportamento do Codex em runtime para hooks não confiáveis.
4. Empacotamento de hooks e MCP para Codex, Cursor e Antigravity, e aprovação
   de hooks de plugin no Cursor.
5. Honra dos hooks globais pelo Antigravity IDE.
6. Extensões MCP do Zed e fluxo de instalação de MCP pelo Cursor.
7. Requisitos de publicação de plugin do ChatGPT.

Documentos relacionados:

- [Claude Desktop](claude-desktop.md)
- [Codex desktop](codex-desktop.md)
- [Cursor](cursor.md)
- [Antigravity](antigravity.md)
- [Zed](zed.md)
- [Plano de suporte desktop](plan.md)
- [Verificação de outubro de 2026](verification-2026-10.md)
