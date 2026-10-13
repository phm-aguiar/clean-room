# ai-memory cookbook

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

Folha de receitas orientada a tarefas: "quero fazer X" → como. Para a referência
completa, veja [`architecture.md`](architecture.md). Para instalação do Windows, veja
[`windows.md`](windows.md).
Para a tabela de roteamento de ferramentas, veja [`usage.md`](usage.md).

## O que é o ai-memory, em um parágrafo

O ai-memory dá aos seus agentes de código memória de longo prazo, entre sessões.
Ele roda como um único servidor. O agente conversa com ele por MCP (ferramentas como
`memory_query` e `memory_write_page`) e por hooks de ciclo de vida que capturam
automaticamente o que você faz. A memória é uma **wiki em markdown versionada em git**
(fonte de verdade, editável à mão) mais um **índice derivado** para busca. Tudo é
**separado por projeto** `(workspace, project)`, resolvido a partir do diretório de
trabalho. Ele funciona sem nenhum LLM (captura, busca em texto completo e checkpoints
por regras). Ao configurar um provedor de LLM, habilita a consolidação e a
auto-melhoria.

## Tarefas do dia a dia (pelo agente, via MCP)

Você normalmente só conversa com o agente, e ele chama a ferramenta certa. Casos comuns:

| Você quer | Diga ao agente | Ferramenta usada |
|---|---|---|
| Recuperar trabalho anterior | "Já discutimos X?" / "o que decidimos sobre Y?" | `memory_query` |
| Retomar depois de um tempo longe | "Onde paramos?" / "me atualize" | bloco de handoff / `memory_explore` |
| Lembrar um fato durável | "Lembre que sempre usamos tool-a aqui" | `memory_write_page` |
| Lembrar até uma data | "Lembre disto até a migração sair" | `memory_write_page` com `expires_at` |
| Uma regra permanente para *todos* os projetos | "Sempre: nunca faça force-push" | `memory_write_page` com `scope: "profile"` |
| Ler uma página salva por inteiro | "Leia a página `_rules/deploy`" | `memory_read_page` |
| Salvar contexto para a próxima sessão | "Encerrando, salve o contexto" | `memory_handoff_begin` |
| Ver estatísticas do projeto | "Quanta memória temos?" | `memory_status` / `memory_briefing` |

Você raramente escreve páginas à mão. Hooks de ciclo de vida capturam prompts e chamadas
de ferramenta automaticamente e, com um LLM, a consolidação os transforma em páginas.

## Receita: manter uma regra que o projeto deve seguir

> "Lembre: antes de mexer no código de pagamento, leia as notas de PCI."

O agente chama `memory_write_page` com um caminho estável. Regras usam o prefixo
`_rules/` (por exemplo `_rules/pci`). O caminho é escolhido pelo agente, não é
roteado automaticamente.

Na sessão seguinte, `memory_query` encontra a regra. Se o `.ai-memory.toml` do projeto
tiver `[briefing]` com `inject_on_session_start = true`, o brief de início de sessão
inclui as regras com o corpo, sem você pedir.

Para a regra valer em **todos** os seus projetos, use uma destas opções:

- `scope: "profile"`: preferências permanentes. A página entra no perfil entre projetos,
  vira padrão no início de cada sessão e aparece nas buscas padrão.
- `scope: "global"`: grava no escopo compartilhado `_global`. Ele aparece como resultado
  extra em toda consulta de projeto único.

## Receita: fazer um projeto ler um documento específico antes de implementar

Há duas formas, conforme onde o documento está:

1. **Já é uma página da wiki** (você salvou ou importou, veja abaixo): peça ao agente
   "leia a página `norms/gdpr-retention` antes de implementar". Ele chama
   `memory_read_page` com esse caminho e trabalha com o corpo completo.
2. **É um arquivo externo** (uma norma ou especificação no disco): salve-o como página
   primeiro ("lembre este spec como `norms/gdpr-retention`", colando o texto ou
   apontando o arquivo) e depois referencie como no item 1. Referências grandes
   funcionam melhor divididas em uma página por documento, para que a busca traga só a
   parte relevante.

## Receita: importar uma base de conhecimento existente (por exemplo normas, leis ou especificações em OKF)

A wiki do ai-memory segue o formato OKF (markdown compatível com Obsidian mais
frontmatter). Para trazer um conjunto de documentos existente como memória do projeto:

- **Poucos documentos**: salve cada um como página durável pelo agente
  (`memory_write_page`) ou pela CLI (`ai-memory write-page --path <caminho> --body <texto>`),
  uma página por documento, sob um prefixo estável como `norms/…`. Depois, qualquer
  projeto pode ler um documento específico antes de implementar (receita acima).
- **Um vault OKF/Obsidian inteiro ou uma exportação de outra ferramenta**: use o
  importador de companions (veja [`companions.md`](companions.md)). Pelo que foi
  verificado, ele lê exportações do wiki OMC e conversas externas. Se ele cobre
  diretamente um vault OKF/Obsidian, a confirmar. O comando `ai-memory export-okf` faz o
  caminho inverso (exporta a wiki de um projeto como bundle OKF), útil para ver o formato
  exato que seus documentos devem ter em disco.
- A wiki fica em `<data_dir>/wiki/<workspace>/<project>/` e é markdown puro versionado
  em git. Você pode colocar arquivos ali. Um observador de arquivos reindexa os arquivos
  markdown criados ou alterados. Mantenha-os sob um prefixo claro e faça commit.

Depois que o material estiver salvo como páginas no escopo certo, qualquer projeto pode
buscá-lo (`memory_query`) e ler um documento específico por inteiro (`memory_read_page`)
antes de implementar.

## Receita: controlar o que é mantido, envelhecido ou consolidado

Por padrão você não precisa pensar nisso. Com o servidor ativo, uma **varredura de
retenção** roda sozinha uma vez por dia (intervalo padrão de 24 horas). Ela remove
páginas episódicas frias, ou seja, aquelas cujo score de retenção ficou abaixo do limiar.
Só o tier episódico decai: páginas dos tiers semântico, procedural e working ficam fora
da varredura. O tier padrão de `memory_write_page` é `semantic`, então páginas escritas
pelo agente não decaem por padrão.

Quando você quiser ajustar, tudo é opt-in e reversível. O original de qualquer coisa
compactada ou mesclada continua no histórico do git e na cadeia de supersessão. Use
`ai-memory restore-page --path <caminho> --from <checkpoint>` para restaurar uma página a
partir de um checkpoint git.

- **Manter algo para sempre**: fixe a página (`pinned=true` ou `pinned: true` no
  frontmatter). Uma página fixada fica fora da varredura, qualquer que seja o tier ou a
  idade.
- **Fazer uma nota expirar em uma data**: peça ao agente para lembrar "até <data>". Isso
  grava `expires_at`, que aceita RFC3339 ou `AAAA-MM-DD`. A página fica oculta das leituras
  após a data, e a varredura a remove por completo (arquivo e registros), mesmo que tenha
  sido lida muitas vezes. TTL tem prioridade sobre fixação. `memory_lint` avisa quando
  uma página é fixada e tem expiração ao mesmo tempo.
- **Ajustar quanto tempo cada tier dura**: no `config.toml` do servidor (em
  `<data_dir>/config.toml` ou como variável de ambiente `AI_MEMORY_DECAY__...`), use a
  seção `[decay.half_life_days]` com as chaves `working`, `episodic`, `semantic` e
  `procedural`, em dias. Cada chave é opcional. Sem chave, vale a curva única padrão. Na
  varredura, só a meia-vida do tier episódico tem efeito verificado.
- **Compactar em vez de remover**: `[decay] compact_cold_episodic = true` reescreve uma
  página episódica fria em vez de removê-la. A versão compacta mantém o resumo curto da
  página, um resumo do primeiro parágrafo e até 48 tokens-chave minerados por padrões
  (identificadores, caminhos, URLs, trechos de código e códigos de erro). O corpo em
  prosa é descartado. A versão anterior continua no histórico do git e na cadeia de
  supersessão. Não exige LLM. Desligado por padrão.
- **Deduplicar páginas quase iguais**: `[decay] dedup_cold_clusters = true` agrupa páginas
  episódicas frias quase duplicadas usando os embeddings armazenados. A similaridade de
  cosseno tem limiar adaptativo, com distância máxima padrão de 0,15 (similaridade de
  pelo menos 0,85) e densidade mínima de 2 páginas. Cada grupo vira uma página
  sobrevivente, e as demais são supersedidas com uma nota de mesclagem apontando para ela.
  Nada é apagado. Exige um embedder configurado. Sem ele, não faz nada. Desligado por
  padrão.
- **Manter a memória usada por mais tempo**: nada a configurar. Uma página aberta, que
  apareceu em uma busca ou em `memory_recent`/`memory_explore`, ou que foi alcançada por
  um percurso de páginas relacionadas, recebe reforço automático e resiste ao decaimento.
  O reforço tem limite de frequência por página e por operador.
- **Encontrar possíveis contradições**: execute `memory_lint` pelo agente, ou
  `ai-memory lint` pela CLI (`--no-llm` evita a parte com LLM). Com embeddings
  configurados, ele aponta pares de páginas do mesmo assunto cuja similaridade cai numa
  faixa de cosseno (padrão de 0,4 a 0,75, configurável). Esses achados são só consultivos,
  não persistem nada no grafo e o relatório é gravado na wiki (exceto em simulação).
  Similaridade mede vocabulário compartilhado tanto quanto desacordo. Em um repositório
  de um único domínio ou de um único idioma, a maioria dos achados será de pares a
  conferir, não defeitos.
- **Deixar um LLM consolidar em ociosidade ("dream")**: exige um provedor de LLM **e** um
  embedder configurados. Ative com `[dream] enabled = true` no `config.toml` do servidor.
  Quando ligado, o agendador verifica a cada hora (padrão de 3600 s) se você está ocioso
  por 5 minutos (padrão de 300 s). Se estiver, ele reescreve grupos de páginas frias em
  uma página coerente por grupo, com limite padrão de 8 grupos por execução e de 2 páginas
  frias mínimas. Qualquer atividade sua cancela a execução em andamento. Ele nunca apaga
  uma fonte: as páginas originais são supersedidas com uma nota de mesclagem. Existe modo
  de simulação. Não é o padrão. A ativação por padrão depende de uma avaliação de recall
  interna.
- **Atualizar sem mudar nada**: as opções novas começam desligadas (compactação,
  deduplicação e dream), e os valores que governam os scores não mudam. A varredura
  diária continua a remover páginas frias abaixo do limiar, como antes.

## Receita: dois agentes ou dois repositórios trabalhando juntos

- **Continuidade entre agentes no mesmo projeto** (fechar o Claude Code e abrir o Codex
  no mesmo repositório): automática. Uma handoff é criada ao fim da sessão, e o hook de
  início da sessão seguinte a entrega e a combina com o brief. Pergunte "onde paramos?".
- **Pedir a um agente de outro projeto que faça algo** sem carregar o contexto desse
  projeto aqui: mensageria entre projetos. Diga "envie ao projeto-b um pedido para
  adicionar o endpoint de exportação" (`memory_message_send`) e, lá, "verifique minha
  caixa de entrada" (`memory_message_pop`). Veja [`agent-messaging.md`](agent-messaging.md).

## Receita: rodar o servidor em um Mac

Use o app de barra de menu quando quiser um único bundle `AI Memory.app` que sobe o
servidor e abre as ferramentas que já existem: interface web, `ai-memory status`,
configuração e logs. Ele não substitui essas ferramentas por um segundo painel.

1. Gere o `AI Memory.app` com o script de build do pacote macOS do repositório e abra o
   bundle gerado.
2. Arraste **AI Memory.app** para `/Applications`.
3. No menu, escolha **Install & Start Server**.

Para conectar um agente, use o binário que vem dentro do bundle:

```bash
BIN="/Applications/AI Memory.app/Contents/Resources/runtime/ai-memory"
"$BIN" install-mcp --client claude-code --apply
"$BIN" install-hooks --agent claude-code --apply
```

A memória fica em `~/Library/Application Support/ai-memory`. Substituir o `.app` não
reescreve a wiki, o índice, a configuração, os modelos nem os logs. Se o caminho do
binário dentro do bundle mudar após uma atualização, use **Restart Server** para
regenerar o agendamento de inicialização. Para os demais caminhos de instalação (pacote compactado,
instalação a partir do repositório, Docker e agendamento de inicialização), veja
[`external-companions-and-tests.md`](external-companions-and-tests.md), seção do app de
barra de menu.

## Pela linha de comando (CLI)

A maioria das pessoas nunca precisa disso, porque o agente faz tudo. Mas os comandos existem:

```bash
ai-memory status                     # contagens e caminhos
ai-memory doctor                     # todo harness que rodou aqui foi capturado?
ai-memory backfill                   # importa o histórico local anterior para uma store vazia
ai-memory backfill --dry-run         # mostra o que seria importado, sem importar
ai-memory write-page --path <caminho> --body <texto>   # salva uma página durável
ai-memory handoffs                   # lista as handoffs abertas do projeto
ai-memory message list               # caixa de entrada entre projetos (--outbox mostra as enviadas)
ai-memory export-okf --project <p> --to <pasta>        # exporta a wiki como bundle OKF
ai-memory serve                      # sobe o servidor
```

## Quando não está fazendo o que você espera

- **Nada está sendo lembrado**: os hooks podem não estar instalados. A forma mais fácil
  é iniciar o harness com `ai-memory run <harness>`: na primeira execução daquele harness,
  ele instala os hooks e o MCP automaticamente (uma vez por versão e por local de
  instalação; desative com `--no-autowire` ou `AI_MEMORY_RUN_AUTOWIRE=false`). Ou instale
  à mão com `ai-memory install-hooks --agent <seu-agente> --apply` (sem `--apply`, o
  comando só imprime a configuração). Depois confira com `ai-memory status` e
  `ai-memory doctor`.
- **Só *alguns* agentes estão sendo lembrados**: rode `ai-memory doctor`. Ele considera
  os harnesses com sessões locais recentes no projeto (últimos 30 dias por padrão,
  ajustável com `--since-days`). Se um deles não tiver nenhuma sessão capturada, aparece
  um aviso com o comando `install-hooks` exato para corrigir. Esse é o caso silencioso de
  um harness usado sem hook: ele mantém o próprio histórico local, mas não captura nada.
- **Instalei os hooks em um projeto em que já trabalho há um tempo**: na primeira vez
  que a sessão começa nesse projeto depois da instalação, se a store do projeto estiver
  vazia, o ai-memory importa o histórico local de sessões uma única vez, em segundo plano.
  Ele importa até 25 sessões, as mais recentes primeiro. O servidor sanitiza e limita
  cada evento. Se o projeto já tiver dados, nada é importado. Você pode disparar ou
  visualizar a importação com `ai-memory backfill` (`--dry-run` mostra o que seria
  importado). Para desligar o início automático, defina `AI_MEMORY_BACKFILL_ON_START=false`
  ou `backfill_on_start = false` no `config.toml`.
- **Uma busca não encontra algo que você salvou**: confira o escopo. A memória é separada
  por projeto. Uma página salva no projeto A não aparece no projeto B, a menos que tenha
  sido gravada no escopo global ou que a consulta use um escopo explícito.
- **Você quer um recurso com LLM (consolidação, resumos)**: defina um provedor com
  `AI_MEMORY_LLM_PROVIDER=…`. Sem ele, a captura e a busca continuam funcionando, a
  consolidação (`memory_consolidate`) fica desativada, o checkpoint de pré-compactação
  cai para regras e o lint roda só por regras.
