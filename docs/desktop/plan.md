# Suporte a aplicativos desktop: plano em fases

> Este documento é uma proposta de pesquisa (RFC), não um roadmap agendado. Ele
> reconcilia a RFC #878 com os achados de pesquisa de 2026-10-07. Cada fase
> lista o que precisaria mudar e onde está o risco.
>
> Os itens são marcados com um estado:
>
> - **Implementado**: o comportamento existe no software. O que foi conferido
>   vem descrito na linha.
> - **Pesquisa**: observado ao vivo ou em documentação de terceiros. Não é
>   comportamento do software.
> - **Proposta**: ainda não existe.
> - **A confirmar**: não foi possível confirmar nesta revisão.

## O que mudou em relação à RFC

A premissa de que "a maioria dos apps de chat desktop não expõe superfície de
hooks" deixou de valer para as superfícies de programação. **Pesquisa**:

- A aba de código do Claude Desktop roda o Claude Code com os hooks do usuário.
  A captura foi observada sem código novo. A aba Cowork não foi verificada.
- O Codex desktop compartilha a configuração de hooks de `~/.codex`. O motor
  foi verificado.
- O Antigravity, incluindo a IDE, tem hooks de ciclo de vida com caminho da
  transcrição no payload.

Os apps sem superfície de hooks são: a aba de chat do Claude Desktop, o
ChatGPT hospedado (chat no navegador) e o Grok Bot.

Além disso, o pacote `.mcpb` como caminho de instalação em um clique para a
Fase 1 foi revalidado em 2026-10-08. **Pesquisa**: a documentação de suporte da
Anthropic apresenta extensões `.mcpb` como a forma principal de instalar um
servidor MCP local no app desktop. Plugins de marketplace, que podem incluir
servidores MCP e hooks, são a superfície de extensão da aba de código.

## Estado atual relevante para desktop

Tudo abaixo foi conferido na implementação atual. Nenhum item é proposta.

| Item | Estado | O que foi conferido |
|---|---|---|
| Tipo de agente do Antigravity CLI | Implementado | O nome de tipo é `antigravity-cli`. `antigravity` e `agy` são aceitos só como alias de entrada. Não existe tipo separado para a IDE. |
| Instalação dos hooks do Antigravity | Implementado | A instalação grava um arquivo global em `~/.gemini/config/hooks.json`. Não existe opção de instalação por workspace. |
| Início de sessão do Antigravity | Implementado | O evento de pré-invocação só vira início de sessão quando o contador `invocationNum` é inteiro e igual a zero. Ausente, não inteiro (`0.5`, `"0"`) ou diferente de zero, o evento é descartado. Falha fechada. |
| Identidade de sessão e cwd do Antigravity | Implementado | A sessão usa `conversationId`. O cwd é `cwd` se presente; senão, o primeiro caminho não vazio de `workspacePaths` (ou `workspace_roots`). |
| Eventos de ferramenta do Antigravity | Implementado | Nome em `toolCall.name` e argumentos em `toolCall.args`. A saída é complementada a partir do arquivo de saída do passo, com teto de 2048 bytes. O complemento vale só para: `run_command`, `view_file`, `list_dir`, `find_by_name`, `grep_search`, `search_web`, `manage_task`, `manage_subagents`. Ferramentas de escrita não são complementadas, porque o conteúdo capturado vem dos argumentos. Ferramentas não comprovadas são tratadas como desconhecidas. |
| Fim de sessão do Antigravity | Implementado (parcial) | Há um hook de parada. Não há evento de fim de sessão. O evento `PostInvocation` não existe no software. |
| Cursor: rascunho vazio | Implementado | Início de sessão com id `empty-state-draft` é descartado. A checagem está em dois pontos do roteador. |
| Cursor: cópia redundante pelo Claude Code | Implementado | Um evento do Claude Code cujo payload identifica o Cursor é descartado quando os hooks nativos do Cursor estão registrados no arquivo de hooks do usuário. Sem hooks nativos, a cópia é mantida. Referência: issue #721. |
| Codex: fim de sessão | Implementado | O evento de fim de sessão é instalado para Codex CLI 0.145.0 ou superior. O arquivo de hooks é `$CODEX_HOME/hooks.json` ou, se a variável não existir, `~/.codex/hooks.json`. |
| Claude Desktop: configuração MCP | Implementado | Caminho por plataforma: Windows (dados locais e roaming, com detecção da versão empacotada MSIX); Linux (`Claude/claude_desktop_config.json` sob o diretório de configuração do usuário). A opção `--config-file` sobrescreve o caminho padrão. Existe o tipo de agente `claude-desktop`. |
| Sessão sem diretório de trabalho | Implementado | Cai no projeto padrão de rascunho: workspace `default`, projeto `scratch`. |
| Ferramentas MCP | Implementado | 23 ferramentas. Conferidas pela contagem de anotações reais e pela lista de nomes. São: `memory_query`, `memory_recent`, `memory_status`, `memory_briefing`, `memory_explore`, `memory_handoff_accept`, `memory_handoff_begin`, `memory_handoff_cancel`, `memory_handoff_list`, `memory_message_send`, `memory_message_list`, `memory_message_pop`, `memory_message_cancel`, `memory_consolidate`, `memory_auto_improve`, `memory_write_page`, `memory_read_page`, `memory_read_session_observations`, `memory_delete_page`, `memory_feedback`, `memory_lint`, `memory_forget_sweep`, `memory_install_self_routing`. |
| Captura explícita por ferramenta | Implementado (ausente) | Nenhuma das 23 ferramentas captura conversa de chat sem hooks. `memory_write_page` grava páginas da wiki a pedido do usuário. |
| Instruções e bloco de roteamento | Implementado | O servidor MCP envia instruções de uso na inicialização. O bloco de roteamento copiado para as regras do projeto é delimitado por marcadores de início e fim. Há uma versão compacta do bloco. |
| Recebimento de hooks | Implementado | Rota `POST /hook` no servidor. |
| Companion para macOS | Implementado | Componentes: runtime empacotado, controle de serviço via launchd, execução de processos, cliente de saúde, armazenamento de token, log do servidor. |
| Empacotamento de serviço | Implementado | Há modelos para systemd (Linux), launchd (macOS), rpm, aur, variáveis de ambiente, sysusers e tmpfiles. |
| Perfis de servidor | Implementado | Há busca, resolução e cadastro de perfis. Um teste garante que um host remoto indisponível não é redirecionado para o servidor local. |

Não encontrado no software (**Proposta**, não existe ainda):

- Evento `PostInvocation` do Antigravity.
- Opção de instalação de hooks por workspace (`.agents/hooks.json`).
- Tipo de agente `antigravity` cobrindo CLI, IDE e versão 2.0.
- Leitura de metadados de sessão do Claude Desktop.
- Mecanismo de marcação (tag) por projeto para sessões de desktop. **A confirmar**.
- Escopo fixo `personal/desktop-chat` para chat de desktop.
- Captura via protocolo ACP. **A confirmar**: o nome aparece só como palavra reservada de subcomando.

## Fase 0: documentação e verificações ao vivo

Ship a qualquer momento, sem código novo. O estado de cada verificação foi
registrado em [verification-2026-10.md](verification-2026-10.md). São
**Pesquisa**, não podem ser reconferidas no software.

| Item | Estado |
|---|---|
| Motor do Codex: `codex app-server` dispara hooks confiáveis com o payload da CLI; hooks não confiáveis ou modificados são ignorados sem aviso; `SessionEnd` ao arquivar ou encerrar | Verificado |
| App Codex: uma solicitação no app com os hooks do ai-memory instalados | Pendente |
| Antigravity CLI: hooks de workspace, reset de `invocationNum` por turno, layout da transcrição | Verificado |
| Antigravity IDE: uma solicitação em workspace com arquivo de hooks de workspace | Pendente |
| Claude Desktop, aba Código: hooks disparam (confirmado pelas transcrições) | Verificado |
| Claude Desktop, Cowork: uma tarefa, quando a VM estiver disponível | Pendente |
| Cursor desktop: executa os hooks do Claude Code em `~/.claude/settings.json` por padrão (sessão de início, disparada no lançamento, foi capturada) | Verificado |
| Cursor desktop: uma conversa no workspace de teste | Pendente |

Itens de documentação:

1. Atualizar a documentação de instalação de clientes MCP, cobrindo o Linux
   (versão beta, pela pesquisa), o caminho de configuração com a opção de
   caminho explícito, o estado da extensão `.mcpb` e a fronteira do Cowork.
   **Concluído** segundo o plano. **A confirmar** nesta revisão.
2. Documentar que as sessões da aba Código do Claude Desktop já chegam ao
   ai-memory, incluindo o comportamento de nomeação do projeto de rascunho.
   Documentado em `mcp-install.md` e `support-matrix.md`. **A confirmar** nesta
   revisão.

## Fase 1: paridade de captura do Antigravity IDE

**Proposta.** É o menor passo de código novo e o melhor alvo da pesquisa.

- Verificar se a instalação global do tipo `antigravity-cli` também atende a
  IDE. Hoje a instalação é global; a IDE ainda não foi verificada (Fase 0).
- Acrescentar a opção de instalar os hooks por workspace, em `.agents/hooks.json`.
- Decidir o nome do tipo de agente. A proposta usa `antigravity` para cobrir
  CLI, IDE e versão 2.0. Hoje o tipo é `antigravity-cli`, e `antigravity` é só
  alias de entrada. Trocar o nome do tipo exige avaliar o impacto nos registros
  já gravados. **A confirmar**.
- Mapeamento de payload para observações. Já implementado: sessão a partir de
  `conversationId`, cwd a partir de `workspacePaths`, ferramentas a partir de
  `toolCall`. **Proposta**: fechar a sessão em um evento de pós-invocação ou de
  parada. Hoje não existe evento de pós-invocação; o hook de parada existe.
- Testes de mapeamento de payload contra o esquema documentado (campos em
  camelCase, variantes do diretório de dados por superfície). Já existem testes
  para a regra do contador `invocationNum` e para `workspacePaths`. Os testes de
  mapeamento por superfície são **Proposta**.

## Fase 2: escopo e deduplicação para sessões vindas do desktop

- **Roteamento de rascunhos.** Hoje o rascunho cai em `default/scratch`
  (Implementado). Duas opções, decisão de produto em aberto:
  - um projeto único para o desktop (nome sugerido: `desktop/claude-scratch`);
  - um projeto por diretório temporário, com marcação por app (mecanismo não
    localizado; **a confirmar**).
  Ver [capture-and-dedup.md](capture-and-dedup.md), seção 2.
- **Deduplicação.** **Pesquisa**: as sessões que passam da CLI para o desktop
  mantêm um único id. Por isso não é preciso uma camada de deduplicação por id.
  A duplicação do Cursor já é tratada (ver tabela de estado atual).
- **Enriquecimento de metadados (opcional).** **Proposta**: ler títulos e
  conta/organização dos arquivos locais de sessão do app. O software não lê
  esses arquivos. São internals de app fechado, então é estritamente opcional.

## Fase 3: captura explícita para superfícies sem hooks

**Proposta.** Para o chat do Claude Desktop, o Grok Bot e o ChatGPT hospedado.

- Uma ferramenta nova que o modelo chama nos apps sem hooks. Exige:
  - mudar a contagem de ferramentas (hoje 23) e a lista de nomes;
  - atualizar as instruções do servidor e o bloco de roteamento;
  - testes de regressão da superfície de prompt;
  - um teste adversarial de fronteira.
- Escopo, conforme a RFC: fixo em `personal/desktop-chat` (opcionalmente por
  app), porque não existe diretório de trabalho nesses apps.
- Formato da entrada MCP do Grok Bot: só vale a pena investigar sob demanda.
  **Pesquisa**.

## Fase 4: instalador de bandeja para usuários não técnicos

**Proposta.**

- Escolha da pilha de interface nativa entre as duas alternativas pesquisadas:
  uma baseada em web view e outra de interface nativa imediata. Matriz de
  empacotamento. **A confirmar**: viabilidade do formato Flatpak.
- Regras de coexistência, detalhadas em
  [installer-and-coexistence.md](installer-and-coexistence.md):
  - detectar um servidor já em execução antes de iniciar outro;
  - herança dos perfis de servidor;
  - caminho do runner nativo estável entre atualizações;
  - não alterar o PATH do usuário.
  O companion para macOS já tem controle de serviço e de processos, e é o
  contato mais próximo dessas regras (**Implementado**).
- Custos de assinatura e notarização, como na RFC #878. Sem mudança.

## Explicitamente adiado ou rejeitado

- **Plugin para o ChatGPT hospedado** (servidor MCP remoto apontando para um
  servidor que o usuário roda): adiado. É uma nova superfície de distribuição e
  exige cumprir os requisitos de revisão de publicação do fornecedor (**a
  confirmar**). Uma RFC separada, se houver demanda.
- **Ler o banco de dados do agente nativo do editor Zed**: rejeitado. É frágil e
  preso à versão. Agentes que falam o protocolo ACP já dão captura por outro
  caminho (**a confirmar**).
- **Usar o socket interno de sessões do Claude Code** no Claude Desktop, ou o
  daemon de execução local do Grok Bot: rejeitado. São internals de app fechado
  e instáveis por definição.
- **Tabela de alias de ids de sessão** para ids locais do desktop: não é
  necessária, porque cada sessão tem um id só. Revisitar se o fornecedor separar
  os ids.

## Questões em aberto

Nenhuma delas bloqueia nada.

1. **Codex no app**: o motor está verificado. Falta uma solicitação no app
   (Fase 0).
2. **Antigravity IDE**: confirmar se a IDE respeita os hooks globais (Fase 0).
3. **`.mcpb` ou plugins para o Claude Desktop**: resolvido. Pela pesquisa de
   2026-10-08, `.mcpb` é a rota atual do fornecedor para servidores MCP locais
   no app desktop.
4. **Escopo do rascunho** (Fase 2): decisão de produto.
5. **Pilha do instalador e viabilidade do Flatpak** (Fase 4).
6. **Local store ou exportação para a aba de chat do Claude Desktop**: não há
   caminho hoje. Reverificar a cada trimestre.
7. **Início de sessão do Antigravity a cada turno**: a pesquisa indica que o
   contador `invocationNum` reinicia a cada turno, e o software trata zero como
   único marco de início. Manter assim, ou limitar por conversa? **A confirmar**.
8. **Cowork no Linux**, onde as tarefas rodam em VM.
9. **Cursor, sessões duplicadas e rascunho de início**: a sessão chega por dois
   caminhos (hooks nativos e cópia pelo Claude Code). Com os hooks nativos
   instalados, a cópia é descartada e fica uma observação por evento. Sem eles,
   fica a cópia, também uma observação. O rascunho de início (id
   `empty-state-draft`) já é descartado. **A confirmar** no app, com uma conversa
   real.
10. **Extensões** ([extensions.md](extensions.md)): um plugin do Claude carrega
    hooks no Cowork, mas não no Chat. Servidores MCP locais só funcionam no
    Cowork no computador e no Claude Code. Codex, Cursor e Antigravity podem
    empacotar hooks e MCP em plugins. A distribuição poderia migrar de escrever
    arquivos de configuração para plugins de marketplace; o servidor ainda
    precisa de instalação separada. Proposta de design na RFC #1166.
