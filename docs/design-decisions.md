# Decisões de design do ai-memory

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

> Síntese das decisões de arquitetura e de produto do ai-memory. Cada decisão
> indica uma de três situações:
>
> - **Comportamento verificado**: o comportamento descrito é o que o sistema faz hoje.
> - **Decisão de processo ou de produto**: não tem implementação verificável; é
>   uma escolha do projeto, não uma afirmação sobre o código.
> - **A confirmar**: ainda não conferido contra a implementação.
>
> Para o mapa operacional atual, leia [architecture.md](architecture.md). Para o
> suporte por cliente, leia [support-matrix.md](support-matrix.md).

## 1. Forma do produto

O ai-memory é um binário único que:

1. **Atua como servidor MCP** para CLIs de agentes de código. Expõe transporte
   stdio e HTTP com streaming. *Comportamento verificado.*
2. **Captura observações de ciclo de vida automaticamente**, sem chamada manual
   de escrita. As observações chegam por scripts de hook ou extensões geradas
   que os clientes disparam. *Comportamento verificado.* Os limites de tamanho
   estão na seção 6.
3. **Mantém uma wiki em Markdown** com páginas, links entre páginas, supersessão
   de versões e um log de eventos (`log.md`). *Comportamento verificado* para o
   log e a supersessão. A existência de um índice (`index.md`) é **a confirmar**.
4. **Expõe uma superfície MCP estreita**: 23 ferramentas, não dezenas. A lista
   está na seção 10. *Comportamento verificado.* Essa escolha é uma decisão de
   produto: menos ferramentas reduzem a confusão do usuário e do agente.
5. **Distribui-se como imagem de contêiner e binário**, para trocar de máquina
   (desktop e servidor doméstico) sem reconfigurar o armazenamento. A porta
   padrão é 49374 e o endereço de escuta padrão é local (127.0.0.1).
   *Comportamento verificado.* O nome e o volume da imagem no exemplo de uso são
   **a confirmar**.
6. **Recupera-se sozinho em alguns pontos**: aplica migrações de esquema na
   inicialização, tira um snapshot dos dados antes de migrar, e ignora vetores
   de embedding incompatíveis em vez de bloquear o arranque.
   *Comportamento verificado.* A auditoria periódica de integridade é **a confirmar**.

## 2. Requisitos de engenharia

Estes são requisitos do projeto, sem evidência de código por si só:

- Arquitetura limpa e modular, com testes unitários.
- Formatação automatizada e padronizada.
- Implantável em contêiner, com backup simples e mudança fácil de máquina.
- Automatizar captura e recuperação de memória, com o mínimo de invocação manual.
- Separar memória de curto prazo de memória de longo prazo.
- Permitir passagem de contexto entre CLIs de agentes diferentes.
- Planejar por iterações: cada funcionalidade funciona antes da próxima começar,
  sem código morto.

*Decisões de processo.*

## 3. Armazenamento: Markdown é a fonte primária

**Decisão: as páginas da wiki em Markdown dentro de um diretório (de preferência
um repositório git) são a fonte primária. O banco SQLite é um índice derivado,
que pode ser reconstruído a partir dos arquivos.** *Decisão de produto, com
comportamento verificado nos pontos abaixo.*

Motivos:

- Backup e mudança de máquina são simples: basta copiar ou clonar o diretório.
- A wiki fica legível e editável por qualquer ferramenta que leia Markdown. Não
  depende de integração MCP.
- Corrupção do banco é recuperável, porque o banco é derivado.

Comportamento verificado:

- **Escritas passam pela camada de wiki.** A escrita de uma página e a escrita
  de um lote de páginas são as únicas entradas para gravar arquivos. Assim,
  sanitização, admissão, atribuição, reversão e atualização do banco ficam juntas.
  Handlers não gravam arquivos da wiki diretamente. *Decisão verificada na
  interface de escrita; a regra de que nenhum handler escreve diretamente é a confirmar.*
- **Não há transação atômica entre arquivo e banco.** Se uma escrita no banco
  falhar depois que o arquivo foi instalado, o sistema tenta desfazer o arquivo
  de forma best-effort. Falhas entre as duas etapas são resolvidas pelo caminho
  de reindexação a partir do Markdown. *A reversão é a confirmar.*
- **O observador de arquivos é rede de segurança**, não a entrada principal. Ele
  faz uma reconciliação completa a cada 30 segundos para capturar eventos
  perdidos de edição externa.
- **Tombamento opcional de páginas removidas do disco.** Por padrão, apagar um
  arquivo não apaga a página no banco; a remoção exige o comando de exclusão de
  página. Com a opção `reconcile_tombstones_deleted_pages` ligada, a página
  OKF importada que sumiu do disco em duas reconciliações seguidas recebe
  tombstone (não é apagada fisicamente). Um disjuntor recusa agir quando mais de
  metade das páginas candidatas do escopo, ou pelo menos três, parecem sumidas
  de uma vez, porque isso costuma indicar volume desmontado. A opção vem
  desligada.
- **Verificação de processos antes de operações destrutivas.** Reset,
  reindexação, restauração e desinstalação checam se há processos do sistema em
  execução antes de agir. A checagem para outras operações é **a confirmar**.

## 4. Banco: um arquivo SQLite

**Decisão: um único arquivo SQLite, com índice de texto completo, vetores em
formato compacto e grafo representado por tabelas de ligação.** *Decisão de
produto.*

Motivos:

- Não exige serviço externo. Um servidor de banco separado seria custo de
  operação para uma instalação pessoal ou doméstica.
- Um motor de busca separado, um banco vetorial dedicado ou um banco de grafos
  embutido somariam superfície de dependências sem ganho no volume esperado
  (centenas a poucos milhares de páginas por projeto). Esses caminhos foram
  rejeitados. Se a busca exaustiva de vetores deixar de bastar, a opção
  registrada é um índice vetorial dedicado dentro do mesmo banco.
- Grafos ficam em tabelas de ligação com consultas recursivas. Travessias em
  memória podem ser usadas em consolidação.

Comportamento verificado:

- O esquema é versionado por migrações. Há 76 arquivos de migração no módulo
  de armazenamento.
- A busca textual usa o índice de texto completo do próprio banco.
- Vetores são guardados com a chave `(provider, model, dim)` (ver seção 5).

## 5. Embeddings e LLM

### Embeddings

- **Desligados por padrão.** Sem provedor configurado, o sistema funciona só com
  busca textual e relações. *Comportamento verificado.*
- **Provedores opt-in:** OpenAI, Voyage, Google Gemini e um provedor
  compatível com a API da OpenAI, para servidores locais. *Comportamento
  verificado.* O provedor compatível exige modelo e dimensão configurados; não
  há valores padrão seguros para servidores próprios. A exigência de URL base e
  a identidade de provedor separada para evitar mistura de famílias de vetores
  são **a confirmar**.
- **Provedor local em processo.** Existe um provedor de embeddings local
  (modelo de 384 dimensões). O modelo não vem embutido no binário; é baixado
  uma vez. Detalhes em [local-embeddings.md](../docs-adiados/local-embeddings.md).
  *Comportamento verificado.* A localização do cache em `<data_dir>/models/`
  é **a confirmar**.
- **Chave de vetor.** Cada vetor guardado registra `{provider, model, dim}`.
  Quando o provedor configurado não bate com o registrado, o vetor é ignorado,
  sem bloquear o arranque. *Comportamento verificado.* A re-geração acontece pelo
  comando `embed --force` ou pelo backfill agendado. O backfill agendado vem
  desligado por padrão, porque pode chamar um provedor pago. *Comportamento verificado.*
- Os provedores implementam uma mesma interface de embedding e são escolhidos
  pela configuração tipada. *Comportamento verificado.*

### LLM para consolidação

- **Desligado por padrão.** Sem provedor, o sistema usa compressão por regras:
  resumos de sessão e página de handoff são gerados sem LLM. Não há reescrita
  de páginas por LLM. *Comportamento verificado* (ferramenta de consolidação
  exige provedor configurado).
- **Com provedor**, a consolidação por LLM roda sob demanda pela ferramenta
  `memory_consolidate`. Na chegada de uma compactação de contexto (PreCompact) o
  sistema faz um checkpoint de consolidação; se esse checkpoint usa LLM é **a
  confirmar**.
- **Fim de sessão.** A consolidação por LLM no fim de sessão só roda com
  `consolidate_on_session_end = true` (variável `AI_MEMORY_CONSOLIDATE_ON_SESSION_END`).
  O padrão é desligado. *Comportamento verificado.*
- Uma sessão substantiva que termina sempre gera uma página de resumo baseada em
  regras e um handoff automático. Uma sessão que tem só marcadores de início e
  fim fecha sem artefatos e libera qualquer handoff de início vinculado a ela.
  *Comportamento verificado.* A definição exata de "substantiva" é **a confirmar**.
- **Transação do fim de sessão.** O handoff automático e a marca de sessão
  encerrada são gravados na mesma transação do banco. *Comportamento verificado.*
  O restante do trabalho de fim de sessão (fila de provedor, reenvio e contagem
  de observações cobertas) é **a confirmar**.
- **Agendamento de manutenção.** As tarefas de manutenção são agendadas pelo
  servidor. Varredura de retenção e lint rodam a cada 86400 segundos por padrão.
  Cada tarefa tem intervalo configurável, e zero desliga a tarefa. O primeiro
  disparo, quando a tarefa está atrasada, ocorre depois de um atraso limitado de
  até 60 segundos após o arranque. *Comportamento verificado.* Um timer de 6 horas
  não foi encontrado no código.

### Provedores e chamadas

- **Cliente HTTP nativo por provedor**, sem camada genérica de gateway. A
  motivação é evitar que um parâmetro seja descartado em silêncio por um
  intermediário. *Decisão de produto, com implementação verificada por provedor.*
- Provedores implementam uma interface com nome, modelo, completar texto e
  completar com saída estruturada. *Comportamento verificado.*
- **Saída estruturada por esquema JSON**, não XML nem envelopes de instrutor.
  Usa o modo nativo de cada provedor quando existe. *Decisão verificada; a
  validação estrita do esquema e a rejeição de campos desconhecidos são **a confirmar**.*
- Timeout padrão de requisição ao LLM: 300 segundos. *Comportamento verificado.*
- Provedores de reserva (`llm_fallbacks`) podem ser configurados. Cada perfil
  exige provedor e modelo. *Comportamento verificado.*
- Erros de provedor são redigidos antes de chegar ao cliente: a resposta traz a
  classe do erro, sem o corpo devolvido pelo provedor. *Comportamento verificado.*

## 6. Captura

Há três superfícies, em ordem de prioridade:

1. **Hooks de ciclo de vida.** Scripts ou extensões instalados uma vez pelo
   usuário. São a fonte principal. *Comportamento verificado.*
2. **Importação de transcrição em fluxo gerenciado** (opt-in, pelo comando
   `run`). Cada adaptador lê a sessão nativa vinculada depois de um lançamento
   gerenciado e acrescenta eventos visíveis ao registro portátil. Não existe
   observador universal sobre os armazenamentos privados das ferramentas.
   *Comportamento verificado.*
3. **Ferramenta MCP de escrita manual** (`memory_write_page`). Só para
   conhecimento durável pedido explicitamente pelo usuário. A captura de rotina
   continua automática. *Comportamento verificado* na descrição da ferramenta.

### Hooks

- **Disparo sem bloqueio.** O script lê o evento na entrada padrão, envia um POST
  e responde ao agente sem esperar o servidor. *Comportamento verificado.*
- **Respostas do servidor.** O servidor enfileira o evento e responde 202. Quando
  o evento é descartado por política (marcador, política de captura, subagente,
  rascunho do Cursor), também responde 202, com a razão no corpo. Quando a fila
  está cheia ou a origem excede a taxa, responde 429. *Comportamento verificado.*
- **Remoção de dados privados na borda do hook**, antes da fila e do disco.
  *Comportamento verificado.* Os tempos limite de escrita em menos de um segundo
  são **a confirmar**.
- **Eventos canônicos (12):** início de sessão, prompt do usuário, antes e depois
  de ferramenta, antes e depois de compactação, notificação, parada, fim de
  sessão, início e fim de subagente, e outro. Cada cliente mapeia seus nomes
  nativos para esses eventos. *Comportamento verificado.* A tabela por cliente
  está em [lifecycle-event-ingestion.md](lifecycle-event-ingestion.md).
- **Importância por evento** (escala usada na consolidação): início e fim de
  sessão = 7; prompt do usuário = 8; antes e depois de ferramenta = 5; parada,
  antes e depois de compactação = 6; notificação, outro e subagentes = 3.
  *Comportamento verificado.*

### Limites de tamanho do corpo

| Tipo de corpo | Limite |
|---|---|
| Prompt do usuário e resumo pós-compactação | 16 KiB |
| Notificação, antes e depois de ferramenta, parada, outro | 2000 bytes |
| Demais observações (teto de segurança) | 16 KiB |

Os limites são aplicados depois da sanitização. *Comportamento verificado.*

### Ferramentas de agentes

- **Ferramentas "fechadas"** (lista de agentes com metadados estruturados de
  ferramenta): título com a família da ferramenta (arquivo, busca e lista, não
  arquivo ou desconhecida), identificador da chamada quando existe e desfecho
  (sucesso, erro ou desconhecido). *Comportamento verificado.* O desfecho do
  Codex é sempre desconhecido, de propósito, porque o evento de falha também
  dispara quando um comando falha.
- **Subagentes.** Um evento marcado como subagente é descartado quando o projeto
  ativa a opção de descarte. *Comportamento verificado.*
- **Início de sessão do Antigravity.** O evento de pré-invocação só vira início
  de sessão quando o número da invocação é zero. Assim, um handoff manual criado
  no fim da sessão continua aberto para a próxima. *Comportamento verificado.*
- **Captura da mensagem do assistente.** Desligada por padrão. Precisa de
  opt-in duplo: a configuração do servidor (`capture_assistant`) e a instalação
  com a opção correspondente. O texto bruto é sempre removido do disco, da rede e
  do armazenamento. O texto aceito tem até 2000 bytes; entradas acima de 64 KiB
  são tratadas como ausentes. *Comportamento verificado.* O nome exato da opção
  de instalação é **a confirmar**.

### Política de captura por marcador de projeto

O marcador `.ai-memory.toml`, o mais próximo do diretório de trabalho, pode usar
`[capture] ignore_paths` para excluir eventos de ferramentas de arquivo antes de
saírem do cliente. É uma fronteira léxica estrita, não um filtro de conteúdo.
Limites: 128 padrões, 1024 caracteres por padrão, e 64 KiB por arquivo de marcador.
O modo padrão é lista de negação; o modo de lista de permissão não captura nada
em repositório sem marcador. A gramática completa está em
[marker-file.md](marker-file.md#capture-exclusions). *Comportamento verificado.*
A ausência de migração de banco para este recurso é **a confirmar**.

## 7. Modelo de memória temporal

Há quatro camadas, com tratamentos diferentes:

| Camada | O que guarda | Vida útil | Decaimento |
|---|---|---|---|
| **Trabalho** | Observações da sessão atual | Até o fim da sessão | A confirmar |
| **Episódica** | Resumos de sessão com etiquetas, arquivos tocados e decisões | Sujeita à retenção | Fórmula de retenção (abaixo) |
| **Semântica** | Fatos, preferências e notas de arquitetura: as próprias páginas da wiki | Indefinida, substituível | Versionada no lugar: a versão antiga fica com `is_latest` falso e a nova aponta para ela |
| **Procedural** | Padrões repetidos extraídos de grupos episódicos | Indefinida | A confirmar |

A camada de cada página é um campo da própria página. Observações brutas ficam
numa tabela separada. *Comportamento verificado* para os valores de camada
(trabalho, episódica, semântica e procedural; semântica é o padrão) e para a
tabela de observações.

### Fórmula de retenção

Pontuação de retenção (maior = manter):

```text
salience × e^(−λ · idade_em_dias)
  + σ × ln(1 + acessos) × e^(−μ · dias_desde_o_último_acesso) × amplificação_por_operadores
```

- Coeficientes padrão: λ = 0,02 (meia-vida de cerca de 35 dias), σ = 0,6,
  μ = 0,04, salience padrão = 1,0.
- A amplificação por operadores distintos é `1 + peso × ln(1 + (operadores − 1))`,
  com peso padrão zero. Com peso zero, a fórmula é a mesma de antes do recurso.
- O λ pode ser sobrescrito por camada, a partir de meia-vida em dias configurada.
  Sem sobrescrita, todas as camadas usam o λ global.
- Limiar frio: 0,20. Páginas episódicas abaixo dele são candidatas a remoção.
- Tombstones e ancestrais de versão são apagados definitivamente depois de
  `hard_delete_after_days`, padrão 180 dias. *Comportamento verificado* para
  todos os valores acima.
- Os limites "30 dias quentes e 180 dias frios" de uma versão anterior deste
  documento não foram encontrados no código. **A confirmar.**
- O limiar de frequência ≥ 2 para padrões procedurais não foi encontrado no
  código. **A confirmar.**

### Autoridade na recuperação

Depois da fusão de resultados, um multiplicador pequeno ajusta a ordem por tipo
de página, por `pinned` e por etiquetas do vocabulário embutido: `canonical`,
`source-of-truth`, `superseded`, `historical`, `test-fixture`,
`do-not-answer-from`. *Comportamento verificado.* A etiqueta `active` é **a
confirmar**.

O multiplicador não é um recuperador independente e não sobrepõe a relevância.
Serve para resolver empates entre conhecimento durável e evidência episódica sem
esconder resultados de sessões específicas. `pinned` protege contra a varredura
de retenção (não contra expiração por TTL). *Comportamento verificado.*

### Feedback de página

- `helpful` e `not_helpful` ajustam a saliência, que entra no termo de tempo da
  fórmula.
- `stale` e `wrong` levam a saliência ao piso e geram um achado no próximo lint.
  Nada é apagado. O sinal fica preso à versão atual; uma reescrita o limpa.
  *Comportamento verificado.*

## 8. Consolidação

### Operações

- **Ingestão de observações** (sem ferramenta MCP dedicada): cada observação
  escrita pelos hooks alimenta as páginas relacionadas; se não há correspondência,
  cria página nova; se a página existe, cria nova versão. Sem LLM, a observação
  vai para uma página de registro diário. O número de páginas por observação e
  o registro diário são **a confirmar**.
- **`memory_query`** (busca sob demanda): funde por RRF (*reciprocal rank
  fusion*, com constante 60) os resultados de texto completo, entidades, grafo
  e, quando configurado, vetores. Depois aplica a autoridade da seção 7 e, se
  pedido com `answer=true` e com provedor, uma resposta citada. Por padrão só
  considera a última versão de cada página; versões antigas entram com
  `include_superseded`. *Comportamento verificado.* Reordenação por LLM depende
  de configuração de reranker: **a confirmar**.
- **`memory_lint`** (agendado e sob demanda): audita páginas episódicas
  obsoletas, títulos duplicados e links quebrados. Com provedor, também procura
  contradições entre páginas semânticas. O relatório vai para `wiki/_lint/report.md`,
  exceto com `dry_run`. O agendamento padrão é diário, não por hora. *Comportamento
  verificado.*
- **Varredura de retenção** (`memory_forget_sweep`), em quatro passos:
  1. páginas com TTL vencido são apagadas de fato, mesmo fixadas ou de qualquer camada;
  2. páginas episódicas atuais abaixo do limiar frio viram tombstone; só este
     passo poupa páginas semânticas, procedurais e fixadas;
  3. tombstones mais antigos que o prazo de exclusão definitiva, com a cadeia de
     versões, são apagados;
  4. observações brutas mais antigas que `observation_retention_days` são podadas,
     só de sessões já consolidadas em página. O padrão é zero, ou seja, desligado.

  A varredura exige capacidade de administrador. Aceita `dry_run`. *Comportamento
  verificado.* A remoção do arquivo Markdown junto com o tombstone e a garantia
  de nunca apagar uma página recriada depois são **a confirmar**.

### Aprendizado automático

O loop de aprendizado (propostas, patches, rejeições, agendador) é separado da
consolidação normal de sessão. Detalhes em
[auto-improvement-loop.md](../docs-adiados/auto-improvement-loop.md). Padrões verificados:

- O agendador revisa sessões recém-concluídas quando há provedor de LLM. Ele vem
  ligado por padrão, a cada 3600 segundos, com no máximo uma sessão por projeto
  por ciclo e idade mínima de 600 segundos após o fim da sessão.
- O passo de experiência entre sessões vem desligado por padrão.
- `require_approval` vem desligado por padrão. Quando ligado, propostas ficam
  pendentes até aprovação humana, tanto as agendadas quanto as manuais.
- Toda escrita passa pelas mesmas regras de escopo e autorização. Não altera o
  contexto ativo do agente no meio do turno.

*Comportamento verificado* para os padrões e a ligação com o provedor. A
garantia de não alterar o contexto no meio do turno é **a confirmar**.

## 9. Passagem de contexto entre agentes (handoff)

**Decisão: o handoff é uma transferência para a próxima sessão, não uma
mensagem ao vivo entre agentes.** O usuário para uma ferramenta e inicia outra;
o início de sessão da segunda busca o handoff aberto do diretório. *Decisão de
produto, com comportamento verificado nas regras abaixo.*

### Campos do handoff

- agente de origem e agente de destino (opcional), sessão de origem, espaço de
  trabalho e projeto, diretório de trabalho, dono (operador), resumo, perguntas
  abertas, próximos passos e arquivos tocados (dica, não lista exaustiva).
  *Comportamento verificado.*
- Limites no armazenamento: 16 KiB por campo de texto e 256 itens por lista.
  *Comportamento verificado.* O limite do resumo lido pelo cliente é **a confirmar**.

### Ferramentas

- `memory_handoff_begin`: cria o handoff. Por padrão pertence ao operador que o
  criou. Com `shared=true`, fica visível para todo o projeto.
- `memory_handoff_list`: lista handoffs abertos sem reivindicá-los.
- `memory_handoff_accept`: reivindica um handoff. Estados de retorno: `claimed`,
  `consumed_by_hook` e `none_pending`. É de uso único.
- `memory_handoff_cancel`: marca um handoff aberto como expirado, para que o
  próximo início de sessão não o consuma.

*Comportamento verificado.* A ferramenta de listagem não aparecia na versão
anterior deste documento.

### Escopo e prioridade

- **Diretório por limite de caminho**, não por igualdade. Um handoff criado em
  `/repo` é entregue a uma sessão em `/repo/api`, mas não a `/repo-other`. A raiz
  `/` cobre todos os caminhos absolutos. Barras finais são removidas ao gravar.
  *Comportamento verificado.*
- **Dono.** A entrega é primeiro restrita ao dono e aos handoffs compartilhados.
  A opção `any_owner` só é aceita para a conta raiz, em recuperação com pedido
  explícito do usuário. *Comportamento verificado.*
- **Criar um handoff automático** expira os handoffs automáticos abertos do
  mesmo diretório exato e do mesmo dono. *Comportamento verificado.*
- **Aceitar um handoff automático** expira os candidatos automáticos mais antigos
  do mesmo dono elegíveis para o diretório receptor. Handoffs manuais, de
  diretórios irmãos e de outros donos continuam abertos. *Comportamento verificado.*
- **Prioridade entre candidatos.** Um handoff manual tem prioridade sobre o
  automático de fim de sessão. Entre automáticos elegíveis, o mais recente vence,
  e a especificidade do diretório só desempata timestamps iguais. **A confirmar**
  para ambas as regras.

### Handoffs de checkpoint

Um agente com turnos (por exemplo, o OpenCode, com parada que indica checkpoint
de turno) pode publicar um checkpoint de continuação sem encerrar a sessão.
*Comportamento verificado.*

## 10. Superfície MCP

Há 23 ferramentas. A coluna "Anotação" traduz as dicas de tipo de operação
declaradas pelo servidor: *leitura* (somente leitura), *escrita* (não
destrutiva) e *escrita destrutiva*.

| Ferramenta | Finalidade | Anotação |
|---|---|---|
| `memory_query` | Busca em FTS, entidades, grafo e vetores, com fusão RRF | leitura |
| `memory_recent` | Páginas atualizadas mais recentemente no projeto | leitura |
| `memory_status` | Contagens e status de execução | leitura |
| `memory_briefing` | Retrato estruturado sem LLM: janelas de 7 e 30 dias, pendências, regras | leitura |
| `memory_explore` | Resumo em prosa sobre o briefing; sem LLM, devolve só o briefing | leitura |
| `memory_read_page` | Corpo completo de uma página, por caminho ou pela melhor busca | leitura |
| `memory_read_session_observations` | Observações brutas de uma sessão, paginadas e limitadas | leitura |
| `memory_handoff_list` | Lista handoffs abertos sem reivindicar | leitura |
| `memory_message_list` | Lista mensagens pendentes sem consumir | leitura |
| `memory_install_self_routing` | Bloco canônico de roteamento e habilidades gerenciadas | leitura |
| `memory_feedback` | Registra utilidade de uma página | escrita destrutiva |
| `memory_forget_sweep` | Varredura de retenção; `dry_run` mostra prévia | escrita destrutiva |
| `memory_delete_page` | Apaga uma página pelo caminho exato | escrita destrutiva |
| `memory_handoff_accept` | Reivindica um handoff | escrita destrutiva |
| `memory_handoff_cancel` | Expira um handoff aberto por engano | escrita destrutiva |
| `memory_message_pop` | Consome uma mensagem da caixa de entrada | escrita destrutiva |
| `memory_message_cancel` | Cancela mensagens pendentes enviadas pelo projeto | escrita destrutiva |
| `memory_lint` | Auditoria da wiki; grava relatório em `_lint` | escrita |
| `memory_consolidate` | Consolidação por LLM da sessão; `multi_page` gera lote atômico | escrita |
| `memory_auto_improve` | Revisão manual de aprendizado de sessão concluída | escrita |
| `memory_write_page` | Grava página durável pedida pelo usuário | escrita |
| `memory_handoff_begin` | Cria handoff para a próxima sessão | escrita |
| `memory_message_send` | Envia mensagem à caixa de entrada de outro projeto | escrita |

*Comportamento verificado.* O servidor declara essas anotações por ferramenta.

### Convenções comuns

- **Escopo.** A maioria das ferramentas aceita `project` e `workspace` como texto
  opcional. Um escopo explícito incompleto falha de forma fechada em operações
  destrutivas. Criar handoff ou página pode criar o projeto se ele não existir.
  *Comportamento verificado.*
- **Aliases.** `query` aceita `q` e `search`. `limit` aceita `n` e `top_k`.
  *Comportamento verificado.*
- **Escopo com sessão e sem sessão.** Com sessão ativa, os parâmetros de escopo podem ser
  omitidos. Clientes sem sessão devem passar os dois juntos. *Comportamento
  verificado* na descrição das ferramentas.
- **Mensagens de outros projetos** são entrada não confiável. O conteúdo é tratado
  como pedido a avaliar, nunca como instrução. *Comportamento verificado* na
  descrição das ferramentas.

## 11. Identidade e escopo

**Decisão: três partes de identidade desde o início: espaço de trabalho,
projeto e caminho da página.** Todo parâmetro de API e de ferramenta carrega as
três, mesmo com um único espaço de trabalho. *Decisão de produto, com
comportamento verificado nos parâmetros das ferramentas.*

### Resolução do projeto

A resolução registra a origem do escopo usado. As origens registradas são:
`explicit`, `session`, `shared_slot`, `startup_seed`, `default` e
`default_after_mismatch`. As duas últimas indicam que a chamada não bateu com a
sessão. *Comportamento verificado.* A ordem exata de precedência entre essas
origens é **a confirmar**; a cadeia "parâmetro explícito, padrão do servidor,
heurística pelo diretório, erro" de uma versão anterior não foi verificada como
descrita.

### Estratégia do nome do projeto

- O padrão é o nome do diretório de trabalho (`basename`). *Comportamento verificado.*
- O marcador `.ai-memory.toml` é procurado subindo a partir do diretório de
  trabalho até o mais próximo. Pode fixar `project_strategy = "repo-root"`, que
  deriva o nome da raiz do repositório. *Comportamento verificado.*
- O comando de instalação de hooks aceita `--project-strategy repo-root`, que
  grava a estratégia no comando gerado. Um marcador que fixa a própria estratégia
  ou o próprio projeto tem prioridade sobre o valor gravado. O padrão não grava
  nada, então instalações existentes continuam iguais. *Comportamento verificado.*
- A variável `AI_MEMORY_PROJECT_STRATEGY` define o padrão global e só vale quando
  o marcador não fixa uma estratégia. *Comportamento verificado.*
- A variável `AI_MEMORY_IGNORE_MARKER` (valores verdadeiros: `1`, `true`, `yes`,
  `on`) desliga a leitura do marcador. *Comportamento verificado.* A estratégia
  do nome do projeto não é trocada por variável em tempo de execução; ela é
  gravada na instalação. *Decisão de produto.*

## 12. Operabilidade

- **Uma leitura de configuração.** O processo captura o ambiente uma vez e a
  configuração é montada na ordem: padrões, arquivo `config.toml`, variáveis de
  ambiente com prefixo `AI_MEMORY_` (com `__` separando seções) e, por último,
  sobreposições especiais. Chaves desconhecidas são ignoradas de forma explícita.
  *Comportamento verificado.*
- **Diretório de dados padrão.** É o diretório de dados local do usuário, na
  subpasta `ai-memory`, canonicalizado. A flag de linha de comando `--data-dir`
  tem precedência sobre a variável `AI_MEMORY_DATA_DIR`, que tem precedência
  sobre o arquivo. *Comportamento verificado.* O registro desse caminho no log de
  arranque é **a confirmar**.
- **Escritas e durabilidade.** A ordem exata entre a confirmação da escrita no
  banco e a linha do `log.md` é **a confirmar**. O índice é gravado na mesma
  transação dos dados, no caminho de fim de sessão. *Comportamento verificado*
  nesse caminho.
- **Migrações.** Aplicadas automaticamente na inicialização, depois de um snapshot
  dos dados. Não há DDL inline fora das migrações. *Comportamento verificado.*
- **Backup e restauração.** Os comandos de backup e restauração geram e consomem
  um arquivo compactado. Há exportação de wiki de um projeto no formato OKF.
  *Comportamento verificado.* Os nomes `export` e `import` por diretório e a
  opção de commit automático do diretório da wiki não foram encontrados no código.
  Ver [backup.md](backup.md) e [okf.md](okf.md).
- **Diagnóstico.** Existe um comando de saúde da instalação. Ferramentas MCP
  separadas para diagnóstico e reparo automático não foram encontradas.
  *Comportamento verificado* para o comando; a detecção automática de drift de
  vetor e de índice de texto é **a confirmar**.
- **Logs.** Rotação diária de arquivos. A variável `RUST_LOG` tem precedência
  sobre o nível de log da configuração. *Comportamento verificado.* Limite de
  tamanho em MB para os arquivos de log não foi encontrado; **a confirmar**.

## 13. Exclusões da versão inicial (histórico)

As exclusões abaixo são a fronteira de decisão da versão inicial. Algumas foram
superadas depois; a lista é mantida como registro, não como suporte atual.

- **Autenticação multiusuário e controle de acesso:** superadas. Há usuários e
  autorização por projeto. Ver [users.md](users.md) e
  [design-per-project-authz.md](design-per-project-authz.md).
- **Interface web:** superada. Há um módulo de navegação web. Ver
  [wiki-browser.md](wiki-browser.md).
- **Sincronização remota na nuvem:** superada parcialmente. O diretório da wiki
  é um repositório git; o espelho para remoto por backup é **a confirmar**. Ver
  [backup.md](backup.md).
- **Banco de dados em servidor (sem SQLite):** mantida. Não foi encontrada
  implementação de outro banco; **a confirmar**.
- **Bancos vetoriais alternativos:** mantida, com a ressalva de que a configuração
  de armazenamento menciona índices vetoriais do mesmo banco. **A confirmar.**
- **Banco de grafos separado:** mantida. Grafos ficam em consultas SQL.
- **Mídia que não é texto:** mantida. **A confirmar.**
- **Pacote de habilidades e comandos de barra gerais:** superada parcialmente.
  Há habilidades gerenciadas só para roteamento de ferramentas; o produto
  principal continua sendo hooks e MCP.
- **Benchmark de memória de longo prazo:** adiada. **A confirmar** se existe.

## 14. Regras de engenharia

Regras que o projeto aplica. Quando há implementação conferida, está indicada;
as demais são decisões de processo.

1. **Uma só leitura de configuração.** Verificado (seção 12).
2. **Índices na mesma transação da linha de origem.** Verificado no fim de sessão;
   o restante é **a confirmar**.
3. **Saída estruturada por esquema JSON, sem XML.** Decisão; ver seção 5.
4. **Hooks sem espera.** Verificado (seção 6).
5. **Nenhum trabalho de indexação depois do retorno.** Decisão; **a confirmar**.
6. **Identidade tripla desde o início.** Verificado (seção 11).
7. **Vetores com `{provider, model, dim}`; vetores incompatíveis são ignorados.**
   Verificado (seção 5).
8. **Cache de modelos em caminho absoluto, nunca em diretório temporário.**
   Decisão; **a confirmar**.
9. **Observador de arquivos com reconciliação periódica.** Verificado (seção 3).
10. **Checagem de processos antes de operações destrutivas.** Verificado para
    reset, reindexação, restauração e desinstalação (seção 3).
11. **Cliente HTTP tipado por provedor.** Verificado (seção 5).
12. **Ingestão idempotente com identificador determinístico.** **A confirmar.**
13. **Fronteira transacional única.** Verificado no fim de sessão (seção 5).
14. **Testes de propagação de filtros.** Decisão de processo.
15. **Diretório de dados padrão absoluto e canonicalizado.** Verificado (seção 12).
16. **Sem cache de configuração.** Decisão de processo; **a confirmar**.
17. **Filtros de projeto e conjunto de dados como parâmetros de consulta.**
    Decisão de produto; **a confirmar**.
18. **LLM desligado por padrão.** Verificado (seção 5).
19. **Auditoria de licenças das dependências transitivas.** Existe a política de
    auditoria no repositório. Decisão de processo.
20. **Versões travadas.** Existe arquivo de versões travadas no repositório.
    Decisão de processo.

## 15. Fluxos gerenciados e continuidade

Continuidade entre ferramentas é opt-in pelo comando `run`. Lançamentos diretos
das ferramentas mantêm o comportamento de hook e handoff de uso único. *Comportamento
verificado.* A lista de ferramentas gerenciadas está em
[harness-adapters.md](harness-adapters.md).

**Decisão: um registro portátil de eventos, não conversão de formato nativo.**
Um fluxo de trabalho lógico tem uma sessão nativa por ferramenta e um registro
de eventos portátil, só de acréscimo. Converter uma transcrição de uma
ferramenta em outra foi rejeitado: os armazenamentos nativos têm estado privado,
versionado e premissas de integridade que o ai-memory não controla.

Comportamento verificado:

- Adaptadores leem o armazenamento nativo sem modificá-lo e normalizam só mensagens
  visíveis, ferramentas concluídas e marcos de compactação. Raciocínio oculto e
  registros privados do provedor ficam de fora. *Verificado em parte; detalhes
  do adaptador são a confirmar.*
- Um arrendamento de escritor único, renovável, resolve precedência e concorrência,
  em vez de sincronização bidirecional de arquivos. *Existência verificada;
  regras de renovação são a confirmar.*
- Adoção de sessão nativa é restrita a iniciar um fluxo vazio. Lançamentos não
  interativos e fluxos novos começam do zero. *Verificado em parte; a desativação
  global da adoção após o primeiro vínculo é a confirmar.*
- Checkpoints do repositório são evidência nas fronteiras de execução e nunca
  fazem commit, stash ou reset. *A confirmar.*

Decisões de produto:

- A wiki em Markdown continua sendo a superfície de conhecimento durável. O
  registro gerenciado é um substrato de continuidade operacional.
- Mudança de nome do diretório do repositório não reescreve armazenamentos
  privados das ferramentas. Não há API compartilhada para isso, e o sistema não
  consegue distinguir um diretório movido de outro clone do mesmo remoto. A
  exceção é a ferramenta cujo banco local acompanha a árvore do projeto.
