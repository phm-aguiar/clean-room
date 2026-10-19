# Armazenamento e recuperação

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

> **Papel:** a camada de persistência *derivada*. Um arquivo SQLite (`<data_dir>/db/memory.sqlite`,
> modo WAL) com **um único escritor** (ator dedicado) e um **pool de conexões somente-leitura**.
> Contém o esquema e as migrações, toda a lógica transacional de escrita (versionamento de páginas
> por supersessão, sessões e observações idempotentes, handoffs, mensagens, usuários e credenciais,
> propostas de auto-melhoria, workstreams), a busca híbrida (FTS5 + entidades + grafo + vetores,
> fundidos por RRF) e a matemática de decaimento e de crença.
>
> A **fonte da verdade são os arquivos markdown do wiki** (ver [camada de arquivos do wiki](wiki-files-layer.md)).
> O banco é o índice derivado e pode ser reconstruído a partir dos arquivos (comando `reindex`, ver [CLI](cli.md)).

---

## 1. Áreas funcionais

| Área | Conteúdo |
|---|---|
| Abertura | Criação do diretório de dados, pragmas, migrações, backfills e início do escritor e do pool de leitura. |
| Escritor | Fila de comandos e thread única que executa todas as escritas. |
| Esquema | Versões V01 a V83 (83 migrações na versão 3.1.1), descritas em [esquema do banco](database-schema.md). As migrações 77 a 83 não tocam o núcleo da feature 001: colunas de recuperação em `ingest_keys` (77), novos tipos de agente no `CHECK` de `sessions` (78, 80, 81, 83), remoção de colunas legadas de token em `users` (79) e sessão nativa anterior em `managed_runs` (82). |
| Escrita de páginas | Versionamento por supersessão, tombstones e exclusão. |
| Sessões e observações | Admissão idempotente de eventos de ciclo de vida, encerramento e poda. |
| Handoffs | Batões de continuidade entre sessões: publicação, seleção, aceite. |
| Mensagens | Caixa de correio entre projetos. |
| Leitura | Pool de conexões, consulta FTS5 segura, busca híbrida, busca temporal, grafo. |
| Decaimento e crença | Funções puras de retenção, saliência e confiança. |
| Escopo e autorização | Resolução de workspace e projeto, controle de acesso por projeto, coordenadas de projeto. |
| Fila de consolidação | Trabalho durável de consolidação ao fim de sessão. |
| Auto-melhoria | Persistência de propostas, execuções, rejeições e agendador. |
| Workstreams | Leases e ledger das execuções gerenciadas. |
| Credenciais | Chaves de API, senhas humanas e sessões web. |

---

## 2. Abertura do armazenamento

Sequência de abertura (importa para reimplementar):

1. Cria o diretório `db/` com permissão `0700` e o arquivo do banco com `0600`, **antes** de o
   SQLite escrever a primeira página (somente em sistemas Unix). Diretórios e arquivos já existentes
   mantêm as permissões atuais.
2. Abre a conexão com modo WAL, `synchronous = NORMAL` e *busy timeout* de **5000 ms**. Fixa também o
   limite de tamanho do arquivo `-wal` depois de um checkpoint em **64 MiB** (`journal_size_limit`), só
   na conexão de escrita.
   Sem esse limite, o `-wal` fica no maior tamanho já atingido (uma transação grande o deixa do tamanho
   do banco), e uma queda faz o próximo início reprocessar o arquivo inteiro. O limite não interrompe
   uma transação em curso.
3. Desliga a verificação de chaves estrangeiras **durante** as migrações (cada migração roda em
   transação, e migrações reconstroem tabelas), e religa ao final.
4. Aplica as migrações. Se o banco tem **qualquer** migração aplicada que o binário não conhece (não
   só uma versão maior que a maior embutida: um buraco no meio também conta), a abertura falha com
   **erro de schema à frente**, que nomeia a migração aplicada e a maior versão suportada, e o banco não
   é aberto. Um binário antigo nunca toca um esquema novo. Uma migração já aplicada cujo conteúdo
   mudou (soma de verificação divergente) é **tolerada**: o banco abre normalmente.
5. Executa backfills síncronos, antes de o escritor existir e antes de o servidor aceitar tráfego:
   - índice de entidades, populado a partir do frontmatter existente;
   - janelas de validade das páginas (`valid_from`/`valid_to`), em lotes de **5000** páginas, com
     checkpoint de WAL entre lotes e cursor retomável. Uma base já preenchida é um no-op rápido;
   - chaves de coordenadas de projeto, em lotes de **256** por transação.
6. Inicia o escritor e o pool de leitura com limite flexível (*soft cap*) de **4** conexões.

Os dois handles (escritor e leitor) são baratos de clonar e podem ser compartilhados entre tarefas.

**Sem cópia do banco na abertura.** A abertura do armazenamento **não** copia nem fotografa o banco antes
de aplicar migrações. A proteção contra uma migração ruim é a transação de cada migração (uma falha
reverte só aquela migração e a abertura falha) e, na inicialização do servidor, o backup verificado do
diretório de dados feito **antes** de abrir o banco. Esse backup só é criado quando a wiki ainda tem
arquivos fora da conformidade OKF (instalação antiga sendo atualizada) **e** não existe recibo de backup
válido; numa instalação nova ou já conforme, nada é copiado (ver
[camada de arquivos do wiki](wiki-files-layer.md), seção 10.2). Quem abre o armazenamento fora do
servidor (testes, comandos administrativos) não ganha cópia nenhuma.

**Fechamento ordenado** (na parada do processo):

1. Fecha o pool de leitura: as conexões ociosas são descartadas e toda leitura iniciada depois disso
   falha com erro de **leitor fechado**. Uma leitura já em curso não é interrompida nem aguardada; ela
   termina na própria conexão, e essa conexão é descartada (não volta ao pool).
2. Pede ao escritor um checkpoint do WAL no modo que trunca o arquivo `-wal`. É melhor esforço: se um
   leitor ainda segura um instantâneo e bloqueia o checkpoint, ou se o checkpoint falha, só há um aviso
   no log (o `-wal` será reprocessado no próximo início).
3. O escritor fecha a conexão **antes** de responder, de modo que o retorno do pedido significa conexão
   liberada, e para. Comandos enviados depois recebem **escritor fechado**.

O fechamento espera atrás do que o escritor estiver executando; quem chama deve limitá-lo com um tempo
máximo.

---

## 3. Escritor único (invariante #2)

Existe **uma única thread** dedicada que possui a conexão de escrita e processa comandos em ordem.
A entrada é uma fila limitada de **1024** comandos. Cada comando carrega um canal de resposta de uso único.

Padrão de cada método público:

1. Cria o canal de resposta.
2. Envia o comando para a fila. Se o escritor estiver fechado, retorna erro de **escritor fechado**.
3. Aguarda a resposta. Se o canal de resposta for descartado sem resposta, retorna o mesmo erro.

`Shutdown` encerra a thread e aguarda o término. A ocupação da fila é exposta (usada por
*backpressure* e pelo status) como o par (comandos na fila, capacidade).

O escritor processa **um comando por vez, na ordem de chegada**. Ele não agrupa comandos diferentes
numa transação. O que vai junto numa transação é o trabalho de **um** comando (por exemplo, a escrita em
lote de páginas ou um reforço de acesso com várias páginas). Quem precisa de atomicidade entre várias
mudanças manda um único comando que faz todas.

Detalhes que importam:

- **Fila cheia:** o envio **espera** até abrir espaço. O armazenamento não tem tempo máximo nem recusa
  por fila cheia; quem precisa responder rápido (por exemplo, a ingestão de hooks com 429) decide isso
  na própria camada, olhando a ocupação.
- **Encerramento descarta o que está na fila:** ao receber o pedido de parada (ou de fechamento, seção
  2), o escritor para de aceitar envios e **descarta sem executar** todos os comandos ainda na fila,
  inclusive os de quem já tinha garantido a vaga e estava enviando. Cada chamador desses recebe
  **escritor fechado**, nunca uma espera infinita.

Efeito: como há exatamente um escritor, escritas concorrentes não disputam o lock do banco entre si.
Leituras usam o *busy timeout* de 5000 ms.

**Regra de implementação:** uma operação = uma transação. Índices (FTS, links, entidades, evidências,
auditoria) são atualizados **na mesma transação** que o dado (invariante #3).

### 3.1 Famílias de operações

| Família | Operações (resumo) |
|---|---|
| Escopo | Criar ou obter workspace e projeto; garantir escopo com id dado; verificar escopo purgado; varrer projetos vazios; curar caminhos do catch-all. |
| Páginas | Upsert unitário e em lote; excluir caminho; excluir só se a versão atual for a esperada; marcar tombstone de decaimento; apagar cadeia decaída; migração OKF; reparar datas. |
| Sessões e observações | Iniciar, encerrar (com ou sem handoff), admitir evento de hook, inserir observação, completar ingestão, podar observações consolidadas, reorganizar, mover e purgar sessão. |
| Consolidação | Enfileirar, reivindicar, completar, falhar, liberar e reconciliar jobs de consolidação. |
| Handoffs | Inserir, aceitar, aceitar contexto de startup, expirar abertos, cancelar, publicar checkpoint de turno. |
| Mensagens | Inserir, retirar (uma vez), cancelar. |
| Embeddings | Gravar vetores (corpo e abstract), apagar vetores obsoletos, registrar falha de embedding. |
| Reforço e feedback | Incrementar acessos, registrar atividade de cliente, registrar feedback de página. |
| Ciclo de vida | Purgar projeto, apagar workspace, renomear workspace e projeto, mover projeto entre workspaces, compactar. |
| Usuários e auth | Bootstrap e recuperação de root, criar usuários, rotacionar e expirar tokens, senha, desativar, sessões web, credenciais de API. |
| Auto-melhoria | Stagear execução, aprovar, rejeitar, falhar proposta; agendador; marca d'água de experiência. |
| Workstream | Preparar execução, heartbeat, cancelar, vincular sessão nativa, aceitar contexto, finalizar, renomear. |
| Manutenção | Registrar sucesso de job, bootstrap. |

---

## 4. Esquema (resumo por domínio)

Toda linha de domínio carrega a identidade `(workspace_id, project_id, …)`. Ids são blobs de **16 bytes**
(UUIDv7). Timestamps são inteiros em **microssegundos**. O detalhe completo de colunas está em
[esquema do banco](database-schema.md).

- **Topo:** workspaces (nome único) e projetos (único por workspace e nome).
- **Páginas (`pages`):** cada linha é uma **versão**. Campos principais: `is_latest`, `supersedes`,
  `pinned`, `body_sha256`, frontmatter em JSON, `tier` (restrito a working, episodic, semantic,
  procedural), autor, `expires_at`, `salience`, `compacted_at`, contadores de acesso, `superseded_at`
  (marca de tombstone), `valid_from`/`valid_to` e caminho normalizado para busca.
  Um **índice único parcial** em `(workspace_id, project_id, path)` com `is_latest = 1` garante uma
  única versão corrente por caminho.
- **Busca textual:** índice FTS5 sobre título, corpo e `path_search` (o caminho expandido em palavras), com tokenizador `unicode61` que trata `/`, `_`
  e `-` como parte do token e remove diacríticos. Mantido por gatilhos de inserção, remoção e
  atualização. Há índice análogo para observações e para eventos de workstream.
- **Sessões (`sessions`):** tipo de agente restrito por *CHECK*. Por isso, cada novo tipo de agente exige
  migração. Também guarda `ended_at`, a página de resumo, o dono e `ended_observation_count` (marca
  d'água de geração para reencerramento).
- **Observações:** tipo, extensão, evento de origem, título, corpo e `importance` entre 1 e 10 (padrão 5).
- **Chaves de ingestão (`ingest_keys`):** idempotência por evento, com `seen_at` e `completed_at`.
- **Fila de consolidação:** tabela própria (ver seção 15).
- **Links (`links`):** `to_page_id` nulo significa **link pendente** (referência para frente). Tipos:
  `references`, `causes`, `fixes`, `contradicts`. Links carregam workspace e projeto de destino.
- **Entidades:** substantivo normalizado por projeto. Vínculos entre entidade e página apontam para a
  versão imutável e carregam janela `valid_from`/`superseded_at`.
- **Embeddings:** tabela de vetores do corpo e tabela de vetores do abstract. Ambas guardam
  `(provider, model, dim)` ao lado do vetor. Há tabela de falhas de embedding.
- **Feedback, acesso e evidências:** feedback de página; acesso por página e operador; evidências
  que sustentam cada versão.
- **Mensagens e handoffs:** caixa de correio entre projetos; handoffs com dono opcional e estado.
- **Auditoria e atividade:** log de auditoria e atividade de clientes por dia.
- **Autenticação:** usuários (hash de token, hash de senha, papel, desativação), credenciais de API e
  sessões web.
- **Auto-melhoria e workstreams:** runs, propostas, eventos de proposta, rejeições, estado e claims do
  agendador; workstreams, execuções gerenciadas, sessões nativas e eventos.
- **Tombstones de purga:** registros de sessões purgadas e de escopos purgados. Impedem que um hook
  atrasado "ressuscite" algo que foi purgado.
- **Manutenção:** estado de agendamento de jobs globais, progresso de backfill e estado de experiência.

---

## 5. Escrita de páginas

### 5.1 Upsert: o ponto único de escrita

Toda escrita de página passa por uma única função, dentro de **uma** transação:

1. Conforma o frontmatter ao padrão OKF de forma determinística (ver [OKF](okf.md)).
2. Calcula o hash SHA-256 do corpo. Deriva `compacted_at` do marcador `compacted: true` no frontmatter,
   para que o marcador e o corpo compactado entrem juntos na mesma transação.
3. Lê a versão corrente (`is_latest = 1`) para o par `(workspace, projeto, caminho)`.
4. Decide o caso:

**Caso A: versão corrente idêntica.** Mesmo hash de corpo, mesmo frontmatter **ignorando** `generated.at`,
mesmo título, mesmo tier e mesmo `pinned`. Não cria versão. Registra apenas a evidência contra o id
vigente (reconsolidação reafirma a página) e devolve esse id. **Não grava auditoria.**

`generated.at` é o campo `at` **dentro** do mapeamento `generated` (`generated: {by, at}`). Ignorá-lo
significa comparar os dois frontmatters depois de remover só esse campo interno; `generated.by` continua
na comparação. A comparação é entre valores estruturados (a ordem das chaves não conta). Se o frontmatter
guardado da versão corrente não puder ser lido como JSON, ele vale como vazio, então a comparação dá
diferente e a escrita segue para o caso B.

Se a nova versão não traz `generated.at`, o armazenamento o carimba com o instante da escrita, em UTC com
precisão de segundos (`AAAA-MM-DDTHH:MM:SSZ`), mas só quando o mapeamento `generated` existe. Um valor já
presente (carimbado ou herdado pela camada da wiki) é mantido, para que arquivo e linha fiquem iguais.

**Caso B: versão corrente mudou (supersessão).**
- A versão antiga recebe `is_latest = 0` e `valid_to = agora`.
- As janelas de vínculo de entidade da versão antiga são fechadas (`superseded_at = agora`).
- Uma nova linha é inserida com `supersedes` apontando para a versão antiga, `valid_from = agora`
  e `generated.at` carimbado (se ausente).
- Em seguida: substitui os links, reatribui entidades, repara links de entrada pendentes (inclusive
  os de outros projetos), grava as evidências e registra auditoria da ação de supersessão.

Ações de auditoria gravadas pelo upsert (valor da coluna de ação, observável no log de auditoria):
`supersede_page` (caso B), `create_page` (caso C sem tombstone) e `resurrect_tombstoned_page` (caso C com
tombstone). A linha leva workspace, projeto, o id da **nova** versão e o autor, quando há.

**Caso C: página nova.**
- Antes de inserir, verifica colisão de caminho: se outra página **viva** do mesmo projeto tem a mesma
  chave portável (diferença apenas de caixa ou de normalização Unicode), recusa com erro de **colisão de
  caminho**.
- **Ressurreição de tombstone:** se existe uma linha de tombstone no mesmo caminho (`is_latest = 0`
  com `superseded_at` preenchido), a nova versão é encadeada a ela por `supersedes`, o `superseded_at` do
  tombstone é limpo e o `valid_to` original é mantido. A auditoria registra a ressurreição. Assim, uma
  escrita nova não deixa a cadeia antiga órfã e não expõe a página a uma exclusão definitiva posterior.
- Sem tombstone: cria uma cadeia nova e registra a criação na auditoria.

**Propriedades garantidas:** a versão antiga **permanece** consultável (invariante #16: a versão
perdedora continua alcançável). O identificador identifica a **versão**. O índice de busca é atualizado
pelos gatilhos.

### 5.2 Exclusão de página

- Exclusão por caminho remove **fisicamente** as linhas de todas as versões desse caminho. Depois remove
  entidades que ficaram sem nenhum vínculo. Só registra auditoria se alguma linha foi removida, o que a
  torna idempotente.
- A variante condicional só executa se a versão corrente for exatamente a esperada. Caso contrário,
  não faz nada e informa que não removeu.

### 5.3 Tombstones de decaimento

- Tombstone de decaimento: `is_latest = 0`, `superseded_at = agora` e `valid_to = agora`.
- Só é aplicado se o id esperado ainda for o corrente. Um candidato antigo não remove uma página que
  já foi reescrita.
- A remoção definitiva de cadeias decaídas só ocorre dentro do escopo resolvido da varredura e preserva
  página nova recriada no mesmo caminho.

---

## 6. Sessões e observações: ingestão idempotente

### 6.1 Admissão de evento de hook

Admissão de um evento de sessão é **uma transação**:

1. **Localiza ou cria a sessão.** Se existe, valida dono e tipo de agente. Divergência gera **colisão de
   sessão** (sem detalhes de linha), que é terminal. Se não existe e o evento é de fim, o evento é
   descartado como fim sem sessão.
2. **Escopo não é identidade.** Com roteamento que segue o diretório atual, um evento comum pode cair em
   outro projeto. Ele é gravado no projeto que nomeia. Um **fim de sessão** que aponta outro escopo só é
   aceito se o escopo for o da sessão **ou** se vier do mesmo diretório de trabalho armazenado da sessão.
   Caso contrário, é descartado como fim inválido de escopo.
3. **Reencerramento.** Um novo fim de sessão já encerrada só vale se o número de observações cresceu além
   de `ended_observation_count`. Caso contrário, o resultado é "já encerrada".
4. **Chave de ingestão (opcional).** Antes da consulta, remove chaves com mais de **30 dias** (a limpeza
   é global, não por projeto). Depois consulta `(projeto, chave)`:
   - chave já completa: `AlreadyComplete` (não insere nada);
   - chave existente sem conclusão: `ResumePending` (retoma os efeitos sem nova observação);
   - chave nova: grava a chave **e** a observação juntas (`Inserted`).
5. **Movimento de diretório.** Se o evento traz um diretório de origem diferente do armazenado, a sessão
   só é realocada na primeira admissão de um evento com chave, com comparação e troca condicionais
   (compare-and-set). Redelivery não arrasta a sessão de volta. Eventos sem chave não podem provar que não
   são repetição, então não realocam.

Resultados possíveis, em termos de conceito: observação admitida; fim com sessão aberta; reencerramento;
já encerrada; fim inválido de escopo; fim sem sessão.

### 6.2 Semântica de ingestão

- `complete` marca a chave como concluída **depois** dos efeitos a jusante. A semântica é **pelo menos
  uma vez** sob queda: se houver falha no meio, o evento pode ser retomado (`ResumePending`).
- Encerramento com handoff: numa transação, insere o handoff automático, carimba `ended_at` e grava
  `ended_observation_count`.
- **Poda de observações:** apaga, em lotes, observações anteriores a um instante de corte **somente** de
  sessões cuja página de resumo ainda está viva (não superseded). Ajusta `ended_observation_count` para
  baixo, para que o reencerramento continue correto.
- **Fila de consolidação:** ver seção 15.

---

## 7. Handoffs (batões de continuidade)

### 7.1 Publicação

- Uma **sessão aberta** publica um checkpoint por turno. Ele mantém **uma** linha aberta por sessão: o
  próprio batão não reclamado é atualizado no lugar (mesmo id, novo conteúdo e novo instante). Um
  checkpoint que chega depois do fim da sessão ou de uma purga não faz nada e não ressuscita batão
  reclamado.
- Inserção de handoff **automático** (com sessão de origem) expira os handoffs automáticos abertos
  anteriores do **mesmo diretório exato**, do **mesmo dono**, exceto os de sessões ainda abertas (que
  cuidam do próprio batão) e exceto a própria sessão.
- Handoff **manual** (sem sessão de origem) é projeto-wide e não expira outros.
- Campos de texto são truncados em **16 KiB** (por bytes UTF-8, sem quebrar caractere). Listas são
  limitadas a **256** itens.

### 7.2 Seleção do candidato

Função de seleção, em ordem:

1. **Dono primeiro.** O filtro de dono é checado antes de tudo, inclusive antes da regra de manual.
   Handoffs sem dono (anteriores ao controle de dono, ou escritos sem ator) são visíveis a todos.
2. **Manual** é sempre candidato, qualquer que seja o diretório.
3. **Automático** é candidato só se o diretório de origem for **ancestral ou igual** ao diretório da
   sessão receptora. A comparação respeita fronteira de componente de caminho (nunca prefixo solto).
   `/repo` casa com `/repo/api`, mas não com `/repository`. A raiz `/` é ancestral de todo caminho
   absoluto. Sem diretório armazenado, o handoff vale para o projeto inteiro.
4. **Desempate:** manual vence automático; depois o mais recente; em empate de instante, entre automáticos,
   o diretório mais específico (mais longo, após normalização) vence.

**Startup:** a entrega automática no início de sessão (`startup_handoff`) não oferece o batão de uma sessão
que ainda está aberta e que registrou observação depois de `busy_since`. Sessões encerradas e handoffs
manuais não são afetados. A leitura manual não tem essa restrição.

### 7.3 Aceite (compare-and-set)

- Aceite é **compare-and-set**. Duas guardas independentes: leitura de metadados e `UPDATE ... WHERE
  state = 'open'` (com o filtro de dono dentro do `WHERE`). Só um chamador transita `open → accepted`.
  Quem perde recebe `false` e **não** deve entregar o corpo ao agente.
- A sessão receptora deve existir, ter o mesmo escopo e o mesmo tipo de agente. Uma sessão **nunca consome
  dois batões**. Sessão encerrada só pode aceitar se o tipo de agente reutiliza o id da sessão após o fim;
  caso contrário, é erro de estado inválido.
- Usuário de aceite incompatível com o filtro de dono é erro de estado inválido.
- Se o aceito era **automático**, expira os automáticos abertos que são **do mesmo dono**, elegíveis para o
  diretório de quem recebe e anteriores ao aceito na ordem de seleção. Nunca cruza operadores. Batões de
  sessões ainda abertas e ocupadas são poupados.
- Só uma transição real (`open → accepted`) gera auditoria.

---

## 8. Reforço, feedback e decaimento

- **Reforço de acesso:** incrementa `access_count` e `last_accessed_at` **somente** em versões correntes.
  Se há ator identificado, registra também o acesso por operador (uma linha por página e operador, sem
  duplicar). Quando a página não existe mais como corrente, o registro por operador não é criado.
  - **Sem ator identificado (anônimo, o caso padrão de usuário único), o contador compartilhado é
    incrementado mesmo assim.** Só a linha por operador é omitida.
  - Várias páginas de uma leitura são reforçadas numa única transação. Id desconhecido é ignorado, sem
    erro, e não derruba o lote.
  - **Throttle** (fora do armazenamento, na camada que atende a leitura): no máximo um reforço por
    **60 s** para cada par (página, operador). O anônimo é um operador próprio para esse fim. O estado é
    só em memória e se perde no reinício. Um par que ainda está na janela é pulado **sem** renovar o
    instante, para que uma página consultada sem parar ainda receba um reforço por janela. Entradas
    vencidas são descartadas a cada chamada.
  - O reforço é disparado em segundo plano, depois da resposta: falha só gera aviso no log e nunca chega
    a quem leu. Leitura que terminou em erro não reforça nada.
- **Feedback:** insere o registro de feedback, calcula a nova saliência e atualiza a página, tudo numa
  transação, com auditoria. A saliência pertence à **versão**: uma supersessão recomeça do valor padrão.
- Valores de feedback (ver seção 12): `helpful` soma um passo; `not_helpful` subtrai um passo; `stale` e
  `wrong` fixam o mínimo.
- **Expiração:** páginas com `expires_at` vencido não aparecem em buscas (filtro de expiração em todas as
  consultas de candidato).

---

## 9. Mensagens entre projetos

- **Inserção:** recusada com erro de caixa cheia se o destinatário já tem **256** mensagens pendentes.
  Campos de texto são truncados em **16 KiB**.
- **Retirada:** transição `pending → claimed` por compare-and-set. Exatamente um retirante ganha a
  mensagem. Quem perde não recebe nada.
- **Cancelamento:** o remetente pode cancelar as próprias mensagens enquanto estão pendentes.
- **Remoção em cascata:** as duas pontas (remetente e destinatário) apontam para workspace e projeto com
  remoção em cascata. Purgar qualquer um dos lados remove as mensagens correspondentes.
- Estados possíveis: `pending`, `claimed`, `cancelled`.

---

## 10. Ciclo de vida

Purga de projeto e de sessão, exclusão de workspace, renomeações, movimento de projeto entre workspaces,
movimento e reorganização de sessões e compactação (recuperação de páginas livres).

- Todas gravam **tombstones** (sessões purgadas e escopos purgados) para impedir que um evento atrasado
  recrie o que foi removido.
- Para o procedimento operacional de cada uma, ver [operações de ciclo de vida](lifecycle-ops.md).

---

## 11. Leitura

### 11.1 Pool de conexões

- Pool de conexões somente-leitura com limite flexível de **4**. Conexões são abertas **sob demanda**. Ao
  devolver uma conexão, ela só volta ao pool se o pool estiver abaixo do limite. Caso contrário, é fechada.
- O limite de 4 vale só para conexões **ociosas** guardadas. **Não** há limite de leituras simultâneas:
  com o pool vazio, cada leitura abre uma conexão nova. Nenhuma leitura espera por conexão.
- A conexão é aberta em modo somente leitura (o próprio SQLite recusa escrita nela). Nenhuma outra
  configuração é aplicada além do *busy timeout*.
- Cada leitura roda em pool de bloqueio separado, para não prender o runtime assíncrono. Pânico dentro da
  leitura vira **erro de pânico no pool**, e a conexão usada nessa leitura é descartada (não volta ao
  pool). Um erro comum devolvido pela leitura **não** descarta a conexão.
- Conexões de leitura usam *busy timeout* de 5000 ms.
- A configuração de ranking (seção 11.4) e a lista de palavras de parada são parte do handle. Clones feitos
  depois de uma alteração herdam a alteração.

### 11.2 Consulta FTS segura (`prepare_fts5_query`)

Entrada: texto livre de usuário ou agente, mais a lista de palavras de parada ativa. Objetivo: **nunca**
produzir erro de consulta por causa do texto.

"Token" aqui é um trecho separado por espaço em branco Unicode. Todas as regras abaixo trabalham sobre
esses tokens brutos.

1. **Sintaxe explícita** existe se o texto contém aspas (`"`), `(` ou `)`, ou se algum token é
   **exatamente** `OR`, `AND`, `NOT` ou `NEAR`, em maiúsculas. `or`, `Or` ou `NOT,` não contam.
2. **Palavras de parada** (só sem sintaxe explícita):
   - a comparação é feita sobre o token bruto, **antes** de qualquer tratamento de pontuação ou de
     coluna. Por isso `the,`, `que?`, `title:the` e `the*` nunca são removidos;
   - token e lista são comparados em minúsculas com a conversão Unicode completa. Diacríticos não são
     removidos: `à` e `a` são palavras diferentes;
   - lista padrão, usada quando a configuração não define a chave: **57 palavras** em inglês:
     `a, an, and, are, as, at, be, been, but, by, can, could, did, do, does, for, from, had, has, have,
     how, i, if, in, is, it, its, me, my, of, on, or, our, she, should, so, that, the, their, them, they,
     this, to, was, we, were, what, when, where, which, who, why, will, with, would, you, your`;
   - lista configurada pelo operador substitui a padrão. Lista vazia explícita desativa o filtro;
   - se todos os tokens forem palavras de parada, nenhum é removido.
3. **Tratamento de cada token** (vale com e sem sintaxe explícita). Um token gera um ou mais termos:
   - **Coluna desconhecida:** token com `:`, sem aspas, que não começa exatamente com `title:` ou
     `body:` (comparação sensível à caixa: `Title:x` é coluna desconhecida). Cada `:` vira espaço e cada
     pedaço resultante vira um termo **citado** (regra de citação abaixo). Ex.: `pick:handoff` gera
     `"pick"` e `"handoff"`;
   - **Coluna conhecida** (`title:` ou `body:`) com valor não vazio que precisa de citação: a coluna é
     mantida e só o valor é citado. Ex.: `title:ai-memory` vira `title:("ai-memory" OR "ai memory")`;
   - **Precisa de citação** um token que não começa e termina com aspas e que tem algum caractere de
     pontuação ASCII diferente de `:`, ignorando **um** `*` final. Letras acentuadas e dígitos não são
     pontuação. Token que não precisa de citação passa como está.
4. **Regra de citação** de um termo:
   - se o termo contém aspas, cada `"` é dobrado e o termo inteiro vai entre aspas, sem variante;
   - se termina em `*` e o resto tem pelo menos uma letra ou dígito, o `*` sai de dentro das aspas e vai
     para depois de cada fechamento (`ai-mem*` vira `("ai-mem"* OR "ai mem"*)`);
   - a **variante dividida** troca cada pontuação ASCII por espaço e junta os pedaços com um espaço. Se
     ela é vazia ou igual ao termo, sai só `"termo"`; senão sai `("termo" OR "variante")`. A variante
     existe porque o índice de conteúdo mantém `/`, `_` e `-` dentro do token, enquanto o índice de
     caminho os troca por espaço: só as duas formas juntas casam com as duas superfícies.
5. **Junção:** sem sintaxe explícita, os termos são unidos com ` OR ` (o BM25 ordena, o que evita recall
   próximo de zero em linguagem natural). Com sintaxe explícita, os termos são unidos por um espaço, na
   ordem original, e as palavras de parada não são removidas.
6. **Validação e degradação:** a candidata é testada por um analisador FTS5 real, numa tabela em
   memória com as colunas `title` e `body`. A conexão de sonda é aberta uma vez e reaproveitada (uma por
   thread de leitura). Se a candidata passa, é usada. Se não passa (ou se a sonda não pôde ser aberta),
   a consulta **inteira** é refeita como saco de palavras: em cada token bruto, `"`, `(` e `)` viram
   espaço; cada pedaço resultante passa pela regra de citação do item 4 (com a variante dividida); os
   pedaços são unidos com ` OR `. Nessa degradação não há remoção de palavras de parada nem tratamento
   de coluna, e frases e operadores da consulta original **não** são preservados.
7. Consulta vazia (só espaços), ou sem nenhum termo depois do tratamento, devolve texto vazio. O
   chamador deve pular a consulta nesse caso.

### 11.3 Busca híbrida (`hybrid_search`)

**Entradas:** workspace, projeto, texto da consulta, vetor da consulta (opcional), provedor, modelo,
dimensão, limite, instante de corte de expiração e flag de incluir versões superseded.

**Passo 1: janela de candidatos.** `candidate_limit = clamp(limite × 4, mínimo 20, máximo limite + 300)`.
Com limite 0, a janela é 0. A janela é larga porque a autoridade é aplicada **depois** da fusão.

**Passo 2: fluxos de candidatos.** Todos restritos a versões correntes e não expiradas, exceto quando
`include_superseded` é verdadeiro:

- **Texto (FTS):** consulta da seção 11.2 sobre `pages_fts`, ordenada pelo rank BM25 padrão do FTS5
  (sem pesos por coluna), limitada à janela de candidatos. O trecho é o `snippet` da coluna `body`, com
  marcas `<mark>` e `</mark>`, reticências `…` e janela de **24** tokens. Cada candidato traz também o
  que a autoridade precisa (seção 11.5).
- **Entidades:** a consulta é tokenizada. Tokens com **3 a 64** caracteres, no máximo **12** tokens, sem
  repetição. Termos compostos com `-` ou `_` também geram suas partes. Um token casa uma entidade se o nome
  é igual, começa com o token, ou contém o token logo após espaço, `-` ou `_`. O peso de cada página é a
  soma de `1 / freq` por entidade casada, onde `freq` é o número de páginas correntes do projeto que têm
  essa entidade (frequência inversa). Ordem: peso desc., número de casamentos desc., caminho asc.
- **Vetor:** só se há vetor de consulta. Carrega os vetores do projeto com **o mesmo** `(provedor, modelo,
  dimensão)`. Vetores de outra configuração são ignorados. A similaridade é o **produto escalar**. Os
  top-k são selecionados e depois ordenados.
- **Vetor de abstract** (opt-in, desligado por padrão): mesma lógica sobre o vetor do abstract.
- **Grafo:** sementes = páginas do fluxo FTS, depois do fluxo vetorial, do vetor de abstract e do
  fluxo de entidades, nessa ordem, sem repetição (a posição de cada semente é a ordem dessa lista). Expande
  vizinhos por links **nas duas direções**. Links pendentes (sem destino) não são percorridos. Só entram
  vizinhos do **mesmo workspace e projeto** da busca: links entre projetos não são seguidos aqui. O vizinho
  não pode ser semente. Ordem: posição da semente, direção (saída antes de entrada), instante de
  atualização desc., caminho asc. Uma página que aparece por mais de uma semente conta só na primeira
  posição. O fluxo para ao chegar à janela de candidatos. O tipo de aresta (`causes`, `fixes`,
  `contradicts`) é informativo e não altera o ranking.
- **Observações** (ligado por padrão, chave de ajuste `observation_stream`): procura a consulta da seção
  11.2 em `observations_fts`, só nas observações do projeto, em ordem de rank BM25, lendo até
  `min(janela × 4, 1000)` linhas. As linhas são agrupadas por sessão, na ordem em que cada sessão aparece
  pela primeira vez; cada sessão guarda o trecho da sua melhor observação (`snippet` do corpo, janela de
  **16** tokens) e até 3 identificadores de observação. Cada sessão vira um **voto** para a página
  `sessions/<id da sessão>.md` do projeto, se ela existe como versão corrente e não expirada. Sessões
  sem página são puladas. O fluxo para ao chegar à janela de candidatos. Uma sessão que o filtro de dono
  esconde do chamador ainda vota, mas sem trecho e sem identificadores.

**Passo 3: fusão RRF.** Para cada página `d`:

```text
score(d) = Σ sobre os fluxos de 1 / (60 + posição_no_fluxo(d))
```

A posição é contada a partir de 1. Depois, `rank = −score`, na convenção do FTS5: **menor é melhor**.

Regra do fluxo de observações: uma página que recebeu voto do fluxo de observações tem contribuição
**zero** do fluxo FTS (a posição FTS dela continua ocupada, mas não soma). Assim a mesma sessão não é
contada duas vezes. Se a página entrou pela FTS, o trecho mostrado é o da FTS; se não, e o voto trouxe
trecho, o trecho mostrado é o da observação e, quando a página não tinha título, o título vem do voto.

**Passo 4: títulos.** Páginas que chegaram sem título (somente pelo vetor, ou por voto sem trecho)
recebem título e descritor. O descritor não substitui um trecho de observação já escolhido.

**Passo 5: autoridade.** Cada página recebe um fator de autoridade, limitado a **[0.55, 1.50]**.
Os ajustes são somados a partir de 1.0 (ver tabela na seção 11.5). Os dados de autoridade das páginas
que não vieram da FTS são lidos depois, num único acesso. Uma página cujos dados de autoridade não são
encontrados (removida entre os passos, por exemplo) fica sem fator: o rank dela não é ajustado.

**Passo 6: aplicação do fator.** Se o rank é `≤ 0` (vindo da fusão), multiplica por `fator`. Se é `> 0`,
divide por `fator`. Em ambos os casos, **fator maior sempre significa melhor posição**.

**Passo 7: ordenação e corte.** Ordena por rank e, em empate, por caminho (comparação de bytes). Trunca
em `limit`. Se `include_superseded` é verdadeiro, marca quais resultados são versões superseded.

**Vários escopos numa consulta:** cada escopo é buscado separadamente com o mesmo limite. Com um escopo
só, o resultado é o dele, sem mudança. Com vários, os resultados são unidos; uma página que aparece em
mais de um mantém o melhor rank; a união é ordenada por rank, depois caminho, depois id, e cortada em
`limit`.

**Explicação:** a variante explicada devolve também rank e pontuação por fluxo, contribuição RRF, entidades
casadas, origem do grafo (semente, direção e aresta), fator de autoridade, confiança, quantidade de
evidências e intenção (ver seção 11.5).

### 11.3.1 Recurso a observações cruas (`raw_hits`)

Feito pela camada que atende a consulta (a ferramenta MCP de busca), não pela busca híbrida. Quando a
busca de páginas devolve **nenhum** resultado, procura-se a mesma consulta (preparada pela seção 11.2) nas
observações do escopo, por FTS em `observations_fts`, em ordem de rank BM25, com o mesmo limite da
consulta e trecho de 24 tokens. O filtro de dono vale: entram só observações de sessões sem dono ou do
próprio chamador (quando a implantação distingue operadores). Com vários escopos explícitos, a busca é
feita em cada um, os resultados são juntados, ordenados por rank e cortados no limite. Se houver
qualquer página, `raw_hits` vem vazio.

### 11.4 Busca temporal (`as_of`)

Busca com instante `T`:

- Só considera páginas com `valid_from ≤ T` e (`valid_to` nulo ou `valid_to > T`).
- Usa texto e entidades, com janela de vínculo `valid_from ≤ T` e (`superseded_at` nulo ou `> T`).
- Não usa vetor, grafo nem busca crua, e **não** incrementa contadores de acesso.

Ver [validade temporal](temporal.md) e [janelas de ingestão de página](design-page-ingestion-windows.md).

### 11.5 Autoridade e sinais de ranking

O **tipo** da página, usado nas três primeiras linhas da tabela, é:

1. o valor de `kind` no frontmatter, convertido para texto (um `kind: 5` vale `"5"`), quando presente e
   não nulo;
2. senão, pelo prefixo do caminho, comparado **sem distinguir maiúsculas** (letras ASCII):
   `_rules/` → `rule`, `_slots/` → `slot`, `sessions/` → `session`, `decisions/` → `decision`,
   `gotchas/` → `gotcha`, `concepts/` → `concept`, `procedures/` → `procedure`, `notes/` → `note`,
   `profile/` → `preference`;
3. senão, `fact`.

O nível vem da coluna `tier` e a fixação, da coluna `pinned`.

| Sinal | Ajuste |
|---|---|
| Tipo `rule`, `decision` | +0.15 |
| Tipo `procedure`, `gotcha`, `preference` | +0.12 |
| Tipo `concept`, `slot` | +0.07 |
| Tipo `session` | −0.15 |
| Tier `semantic`, `procedural` | +0.10 |
| Tier `episodic` | −0.08 |
| Tier `working` | −0.03 |
| Página fixada (`pinned`) | +0.08 |
| Caminho `_lint/` | −0.20 |
| Caminho `investigations/` | −0.05 |
| Tag `canonical` ou `source-of-truth` | +0.20 |
| Tag `active` | +0.05 |

Tetos (aplicados depois dos ajustes, sem excluir a página):

- tags `do-not-answer-from` ou `test-fixture`: fator no máximo **0.55**;
- tag `superseded`: no máximo **0.65**;
- tag `historical`: no máximo **0.80**.

Tags vêm da lista `tags` do frontmatter. Itens que não são texto são ignorados. Cada tag é normalizada
assim: espaços removidos **só das pontas** (espaços internos ficam), letras ASCII em minúsculas e `_`
trocado por `-`. Cada ajuste e cada teto valem **no máximo uma vez** por página, por presença: `canonical`
e `source-of-truth` juntos somam +0.20 uma vez só, e `active` repetido soma +0.05 uma vez só.

Ordem de cálculo: soma os ajustes a partir de 1.0, aplica o menor teto presente e depois limita o resultado
a [0.55, 1.50]. Os dois caminhos de prefixo (`_lint/` e `investigations/`) são exclusivos.

**Roteamento de recall de sessão** (opt-in, desligado por padrão). Se a consulta tem marcador léxico de
recall de sessão em chinês, inglês ou português (ex.: "上次", "last time", "what did we", "sessão anterior",
"decisão anterior", "onde paramos ontem"), páginas do tipo `session` recebem de volta a penalidade de tipo e tier, mais um bônus
configurável (padrão **0.25**). O resultado continua dentro de [0.55, 1.50]. Esse roteamento é marcado na
explicação como intenção `session_recall`.

*Penalidade devolvida:* só páginas de tipo `session` a têm. Valor = 0,15 (tipo) + a penalidade de tier, quando negativa
(0,08 para `episodic`, 0,03 para `working`, 0 nos demais). Fator roteado = `limitar([0,55; 1,50], fator + penalidade devolvida + bônus)`.
O `explain` traz `intent = "session_recall"` e `intent_boost` = fator roteado ÷ fator não roteado.

*Lista completa de marcadores* (comparação sobre a consulta em minúsculas Unicode):

- **Chinês, por substring simples:** 之前, 上次, 上回, 上一次, 那次, 那回, 前几天, 前天, 昨天, 那天, 上周, 上星期, 上个月,
  的会话, 次会话, 个会话, 会话里, 会话中, 历史会话, 当时我们, 我们当时. ("会话" sozinho não conta: só nesses quadros de recall.)
- **Inglês, por palavra inteira:** `last time`, `last session`, `previous session`, `earlier session`, `that session`,
  `the session where`, `yesterday`, `the other day`, `last week`, `when we`, `we did`, `did we`, `what did we`,
  `how did we`, `back then`, `earlier we`, `previously`.
- **Português, por palavra inteira:** `última sessão`, `ultima sessao`, `última sessao`, `ultima sessão`, `sessão anterior`,
  `sessao anterior`, `onde paramos ontem`, `decisão anterior`, `decisao anterior`. ("sessão", "antes", "ontem" ou "anterior"
  isolados não contam; "penúltima sessão" e "decisão anteriormente" também não.)
- **Casamento por palavra inteira:** a consulta em minúsculas é transformada trocando por espaço todo caractere que não seja
  alfanumérico ASCII nem letra não ASCII, e recebe um espaço em cada ponta; o marcador casa se ` marcador ` (com espaços) aparece nesse texto.
  Assim "know" não dispara "now" e "(DECISÃO ANTERIOR)," dispara.

**Peso de crença** (opt-in, padrão **0.0**, ou seja, desligado): soma `peso × confiança` ao fator, ainda
dentro do limite. Versões superseded nunca recebem esse bônus. A confiança só é calculada quando há `explain` ou peso positivo;
com os dois desligados nenhuma consulta de evidência é feita. A contagem de contradições vivas considera apenas arestas
`contradicts` cujo alvo resolve para uma página **atual e não expirada**.

**Limites documentados:** o ranking é projetado para que a relevância continue primária. Até o melhor sinal
de autoridade fica em no máximo 1.5× e o pior em 0.55×. Nenhum sinal exclui a página.

### 11.6 Outras leituras

- Páginas recentes, páginas fixadas, páginas de sessão e de briefing (contagens, atividade, regras, slots
  visíveis, recentes e fixadas).
- Caminhamento relacionado (BFS limitado): profundidade máxima **3** e no máximo **50** nós.
- Handoffs abertos por projeto, mensagens, saúde (páginas obsoletas, duplicadas e órfãs), candidatos de
  decaimento, tombstones de decaimento, arestas de contradição, arestas entre projetos, auditoria,
  contadores de status, status de armazenamento e de índice derivado, cópia de backup online do banco.
- Busca de usuários, credenciais e sessões web, e telemetria de auto-melhoria.

Candidatos de decaimento são todas as versões correntes do projeto. A decisão de remover fica com a
varredura de retenção (ver [envelhecimento de memória](design-memory-aging.md)).

---

## 12. Decaimento e retenção

Fórmula (funções puras, sem banco):

```text
retention = salience · exp(−λ_tier · age_days)
          + σ · ln(1 + access_count) · exp(−μ · days_since_access) · breadth
breadth   = 1 + breadth_weight · ln_1p(max(distinct_actors, 1) − 1)
```

- O termo de acesso só existe se a página já foi acessada (`days_since_access` presente).
- `salience` ausente usa o valor padrão.
- `λ_tier` é a taxa do tier, se configurada; senão, a taxa escalar `λ`.
- `breadth_weight` inválido (negativo, infinito ou NaN) vira **0**: a falha é fechada e o bônus é desligado.
- Com `distinct_actors` 0 ou 1, `breadth` é 1. Com peso 0, a fórmula é a histórica.

**Parâmetros padrão:**

| Parâmetro | Valor | Significado |
|---|---|---|
| `λ` | 0.02 | Meia-vida de cerca de 35 dias |
| `σ` | 0.6 | Peso do reforço por acesso |
| `μ` | 0.04 | Decaimento do reforço desde o último acesso |
| `salience_default` | 1.0 | Saliência de página sem feedback |
| `cold_threshold` | 0.20 | Abaixo disso, a página é candidata a remoção |
| `hard_delete_after_days` | 180 | Dias que um tombstone e sua cadeia sobrevivem |

**Meia-vida por tier:** `λ = ln 2 / dias`. Um conjunto vazio de overrides por tier equivale ao escalar
(resultado idêntico ao de antes da existência do override).

**Saliência por feedback:**

- faixa válida: mínimo **0.25**, máximo **2.0**, passo **0.25**;
- `helpful` soma um passo; `not_helpful` subtrai um passo (sempre dentro da faixa);
- `stale` e `wrong` fixam o mínimo (0.25).

**Política de varredura (o que a varredura de retenção considera):**

- Só páginas do tier **episodic** são candidatas a evicção por retenção. Semantic, procedural e working
  ficam fora dessa varredura.
- Páginas fixadas (coluna `pinned` ou `pinned: true` no frontmatter) são isentas.
- A evicção cria tombstone. Tombstones mais antigos que `hard_delete_after_days` são removidos definitivamente.
- Páginas com `expires_at` vencido passam por uma varredura de TTL, independente do tier e da fixação.
- Isenção de páginas em `_slots/` da evicção por retenção: **a confirmar** (no código verificado, a isenção
  é por fixação).
- Comportamento de páginas *working* ao fim da sessão: **a confirmar** (não há regra de fim de sessão para
  esse tier no código de armazenamento verificado).

Ver também [envelhecimento de memória](design-memory-aging.md).

---

## 13. Crença (confiança derivada)

Confiança é **calculada na leitura**, não armazenada. Entradas: quantidade de evidências da versão,
quantidade de sessões distintas que a sustentam, instante da evidência mais recente e quantidade de
contradições não resolvidas.

```text
residual  = max(evidence_count − distinct_sessions, 0)
breadth   = distinct_sessions + min(0.2 · residual, 1.0)
support   = 1 − exp(−breadth / 3)
recency   = 0.5 + 0.5 · exp(−idade_em_segundos / (30 dias))
contrad   = 1 / (1 + contradições)
conf      = clamp(support · recency · contrad, 0, 0.95)
```

Propriedades:

- **Anti-entrenchment:** conta **sessões distintas**, não repetições. Uma sessão citando 50 vezes vale pouco.
  Evidência sem sessão tem crédito de 0.2 e o total desse crédito é limitado a 1.0.
- **Recência:** o piso é 0.5. Evidência antiga perde até metade do peso, mas nunca é apagada por idade.
- **Teto:** 0.95. Evidência sozinha nunca chega a confiança máxima.
- **Monotonia:** cresce com sessões e evidências; cai com a idade da evidência mais recente e com contradições.
- **Sem evidência** (ou breadth zero): confiança 0, neutra, sem efeito no ranking. Instante mais recente ausente
  conta como recência 1.0. Idade negativa é tratada como zero.
- **Supersessão sempre vence:** a confiança nunca impede a gravação de uma correção nem a devolução dela.

---

## 14. Escopo e autorização por projeto

### 14.1 Resolução de escopo

`ScopeResolver`, por requisição, só empresta handles. Funções auxiliares: buscar escopo existente sem criar,
criar escopo explícito e criar escopo global.

Nomes de workspace e de projeto recebidos são **aparados**; texto vazio depois de aparado conta como
ausente. Um nome de projeto com exatamente uma `/` e texto dos dois lados (`acme/api`) pode ser um rótulo
`workspace/projeto` passado no campo de projeto: veja abaixo.

**Leitura e operações sobre escopo existente** (busca, leitura, embedding, retenção, remoção, feedback,
lint, aceite de handoff): **nunca criam** workspace nem projeto. O nível de acesso exigido (leitura ou
escrita) é o da operação, não o formato dos argumentos.

| Argumentos | Resultado |
|---|---|
| workspace e projeto | Procura o workspace pelo nome e o projeto pelo nome dentro dele. Se não acha e o projeto é um rótulo `ws/proj`, tenta esse par; se o par também falha, devolve o erro da primeira tentativa. Inexistente: erro de **não encontrado**. Origem: `Explicit`. |
| só workspace | Erro de entrada: workspace e projeto precisam vir juntos. |
| só projeto | Procura o projeto no workspace do ponteiro de projeto ativo do chamador; se não acha (ou não há ponteiro), no workspace padrão do servidor. Nenhum dos dois: erro de projeto não encontrado no workspace ativo nem no padrão. Origem: `Explicit`. |
| nenhum | Usa o projeto do ponteiro de leitura do chamador. Sem ponteiro, usa o projeto padrão do servidor. **Nunca falha** por falta de escopo. A origem diz de onde veio (tabela de origens abaixo). |

O ponteiro de leitura inclui, além do ponteiro do chamador, a **semente de startup** (o projeto
reconstruído do histórico até chegar o primeiro evento de hook depois de um reinício). O ponteiro de
escrita não inclui a semente.

**Escrita**: única que pode criar workspace e projeto.

| Argumentos | Resultado |
|---|---|
| só workspace | Erro de entrada. |
| nenhum | Ponteiro de projeto ativo do chamador resolvido: usa esse projeto. Ponteiro que **diverge** do chamador: **recusa** com erro de escrita ambígua, cuja mensagem pede escopo explícito ou a instalação dos hooks. Sem ponteiro algum (chamador anônimo ou legado): usa o projeto padrão do servidor. |
| projeto (com ou sem workspace), rótulo `ws/proj` | Procura um projeto com esse nome exato (no workspace dado, ou no ativo, ou no padrão). Se não existe, usa o par do rótulo, **sem criar** nenhum dos dois. |
| projeto (com ou sem workspace), nome comum | Workspace: o dado (criado se não existe), ou o do ponteiro ativo, ou o padrão. Projeto: resolvido pelo nome no workspace (com as chaves de compatibilidade da seção 14.3, inclusive promoção de nome) e **criado** se não existe. Nomes reservados de perfil de outro usuário são recusados. |

Toda resolução passa pela autorização por projeto (seção 14.2). Uma recusa vira erro de **proibido**.

- **Origem da resolução** (`ScopeSource`), com o texto usado nos avisos: `Explicit` (`explicit`),
  `Session` (`session`), `SharedSlot` (`shared_slot`), `StartupSeed` (`startup_seed`), `Default`
  (`default`) e `DefaultAfterMismatch` (`default_after_mismatch`). São "fallback" a semente de startup e o
  padrão após divergência. São "inferidas" todas, exceto explícita e sessão. Na leitura sem argumentos, a
  origem vem do ponteiro: ponteiro de sessão → `Session`, slot compartilhado → `SharedSlot`, semente →
  `StartupSeed`, ponteiro divergente → `DefaultAfterMismatch` (e o escopo usado é o padrão), sem ponteiro
  → `Default`.
- Erros de escopo se dividem em **entrada inválida** (equivalente a 400: par incompleto, nome ambíguo,
  escrita ambígua, entradas vazias numa lista de escopos, lista acima do máximo), **não encontrado**
  (equivalente a 404) e **proibido** (equivalente a 403).

### 14.2 Autorização por projeto

Atribuição (quem escreveu) é diferente de autorização (quem pode entrar). Esta seção decide **entrar** num
projeto. É consultada pela resolução de escopo e, como defesa em profundidade, pela escrita.

Modos de acesso do projeto: **aberto** (padrão) e **restrito**. Níveis de concessão: **leitura** e **escrita**.

**Decisão, em ordem:**

1. Implantação que não distingue operadores (uso local, sem usuários no banco): **permite**.
2. Projeto reservado compartilhado (escopo global e perfil do workspace), para **escrita**: só root ou
   concessão de escrita. Ser criador não basta. Leitura continua aberta.
3. Projeto **aberto**: permite.
4. Projeto **restrito**: root e criador sempre; leitura exige **qualquer** concessão; escrita exige concessão
   de escrita. Os demais, inclusive anônimo, recebem negação com mensagem de projeto restrito.

**Resolução:** lê modo de acesso, criador e se o projeto é reservado. Criador é o próprio principal criador
ou quem coincide com o dono registrado do projeto. A concessão só é consultada se o projeto é restrito ou
reservado.

**Falha na resolução é aberta, não fechada**, para não tirar o acesso do operador aos próprios dados:
coluna ausente, valor desconhecido, projeto inexistente ou erro de leitura resultam em modo aberto com
aviso. Restrito com zero concessões ainda admite root e criador. Se a tabela de concessões está ilegível
(migração em andamento), degrada para aberto, **exceto** no escopo global, onde escrita é recusada.

**Variante estrita** (usada em mutações de execução gerenciada que liberam ou substituem estado de outro
operador): nessas mutações, falhas de resolução viram **erro** em vez de aberto. Usuário apagado também é
negado. Se o servidor tem usuários, conta como distinguindo operadores mesmo que o nível de autenticação do
chamador não diga o contrário.

**Invariante #16:** é checagem de autorização de **entrada**. **Nunca** vira filtro de linhas de página
nem escopo de leitura por usuário. Um time com concessões vê as mesmas páginas compartilhadas.
`author_id` jamais é filtro de leitura.

Ver [autorização por projeto](design-per-project-authz.md) e [modelo de segurança](security-boundaries.md).

### 14.3 Coordenadas de projeto

Localização indexada e privada de projeto por coordenadas estáticas de workspace e projeto, sem varredura.
Dado o nome pedido e, opcionalmente, a identidade do checkout (remoto git), encontra o projeto.

- **Campos usados:** nome, nome canônico (estilo caminho, derivado do remoto git), nome legado (só o último
  componente), identidade e origem da identidade, modo de acesso.
- **Busca:** cada tipo de casamento (nome exato, canônico, legado, identidade, nome-alvo do checkout) é
  limitado a **4** linhas. Resultados são unidos e deduplicados **antes** do limite. Ordem de preferência:
  identidade, depois nome exato, depois casamento por nome canônico ou legado, depois id.
- **Resolução por nome:** 0 candidatos = nenhum; 1 = esse projeto; mais de 1 = **nome ambíguo**. O nome de um
  projeto restrito é omitido da lista de empatados, para não revelar existência.
- **Aliases:** migração de nome via aliases do marcador de projeto. Só vale com identidade de origem remoto
  git e aliases não vazios. Se já existe projeto com o nome canônico, só é aceito se a identidade remota for
  a do repositório. Senão, é ambíguo, para evitar sequestro de nome.
- **Promoção:** se o pedido é a chave canônica do candidato e o nome atual é diferente, o projeto legado é
  renomeado para o nome canônico.
- **Backfill:** preenche nome canônico e nome legado de linhas com identidade remota e coordenadas vazias,
  em lotes de 256 por transação, até esgotar.

Ver [arquivo de marcador de projeto](marker-file.md).

---

## 15. Fila de consolidação de fim de sessão

Tabela durável por `(sessão, geração)`, onde geração é o número de observações no momento do enfileiramento.

Estados: `pending → running → completed | failed | superseded`.

- **Enfileirar:** só se a sessão já terminou e tem pelo menos uma observação. Se já existe job para a mesma
  sessão, o novo é inserido só se for de outra geração. Jobs `pending` de gerações anteriores viram
  `superseded`. Um job `running` mantém seu lease.
- **Reivindicar:** transação `IMMEDIATE`. Pega o job devido mais antigo: `pending` com menos de
  **5 tentativas** e `next_attempt_at ≤ agora`, ou `running` cujo lease venceu. Gera um `claim_id` que
  identifica a posse.
- **Completar, falhar, liberar:** exigem o `claim_id` atual. Um reivindicante antigo, com lease vencido,
  **não** consegue completar o trabalho de quem o substituiu.
- **Falhar** registra o erro e agenda `next_attempt_at` conforme o instante de nova tentativa informado pelo
  chamador. A política de *backoff* fica com o chamador.
- **Liberar** devolve o job a `pending` sem gastar tentativa.
- **Reconciliação manual:** marca como concluídos os jobs de uma sessão que estão `failed`, `pending` ou
  `superseded`, sem tocar em job `running` (lease vivo).

Limite de tentativas: **5** (`SESSION_CONSOLIDATION_MAX_ATTEMPTS`).

---

## 16. Auto-melhoria (persistência)

- **Propostas:** estados `pending`, `approved`, `rejected`, `failed`. Operações: `create` e `update`. Atualização
  guarda o modo de edição, o JSON de patch e o hash esperado do corpo base.
- **Snapshot no momento do stage:** cada proposta guarda o estado do alvo quando foi colocada em fila
  (hash do corpo, instante de atualização, fixação e frontmatter).
- **Aprovação:** revalida o snapshot. Se o alvo mudou, retorna **conflito** em vez de sobrescrever.
- **Páginas fixadas não são reescritas** pela auto-melhoria, com uma exceção: páginas de `_slots/` sem
  `slot_kind: invariant` (slots de estado) podem ser atualizadas. Slots com `slot_kind: invariant` continuam
  protegidos. Essa exceção é por caminho, não por fixação.
- **Caminho de artefato:** a proposta tem artefato em `_pending/auto-improve/<id>.md`.
- **Eventos:** histórico de status append-only (`staged`, `approved`, `rejected`, `failed`, `conflict`).
- **Rejeições:** normalizadas com impressão digital insensível a espaço e caixa, para telemetria detectar a
  mesma rejeição repetida.
- **Agendador:** marca d'água por projeto e claims por sessão. Cada sessão tem no máximo **3** tentativas
  (`AUTO_IMPROVE_CLAIM_MAX_ATTEMPTS`). Isso evita reprocessamento eterno. As claims tornam o revisor
  idempotente entre reinicializações.

Ver [loop de auto-melhoria](auto-improvement-loop.md) e [gates de avaliação](auto-improve-eval-gates.md).

---

## 17. Workstreams (armazenamento)

- **Lease:** **90 segundos**, renovado por heartbeat. Nome de workstream: até **128** caracteres.
  Conteúdo de evento: até **16 KiB**.
- **Seleção:** `Current` (continua a workstream mais recente, criando `default` se não existe), `Named`
  (seleciona uma existente) ou `New` (cria e seleciona uma nova).
- **Preparar execução:** abre o lease e devolve os cursores (de origem e de entrega).
- **Finalizar:** importa eventos com sequência monotônica por workstream e idempotentes, indexando-os para
  busca. Avança o cursor de origem **só** se a execução está marcada como completa.
- Execução que já foi finalizada: finalizar de novo é um no-op idempotente. Execução cancelada ou
  desconhecida é recusada.
- Eventos de um lote são gravados em transação única. Se um evento é estrangeiro, o lote inteiro é revertido.

Ver [workstreams gerenciados](managed-workstreams.md).

---

## 18. Credenciais

- **Chave de API nativa:** prefixo `aim_` + **43** caracteres URL-safe (**256 bits** de entropia, gerados por
  CSPRNG). Armazena `SHA-256(token ":" pepper)`, com pepper do servidor. A comparação é em **tempo
  constante**. Só a prévia (últimos 4 caracteres) é exibível. A autenticação por chave **não** consulta o
  hash de token legado do usuário.
- **Senha humana:** Argon2id (formato PHC). A computação **nunca** roda na thread escritora. Roda em
  pool de bloqueio atrás de semáforo de **2** permissões. Até **32** trabalhos ativos ou na fila. Acima disso,
  a resposta é **HTTP 429**.
- **Login inválido:** usuário inexistente ou desativado também executa uma verificação *dummy* com o mesmo
  custo de Argon2id, para o tempo de resposta não revelar a existência do usuário.
- **Sessão web:** prefixo `ams_` + **43** caracteres. Guarda SHA-256 (sem pepper) do segredo e do valor CSRF.
  Validade absoluta de **7 dias**. O campo de último uso é atualizado no máximo **uma vez por minuto**.

Ver [usuários](users.md) e [modelo de segurança](security-boundaries.md).

---

## 19. Erros

Categorias de erro de armazenamento. O conjunto é **aberto**: clientes devem tratar categorias desconhecidas.

- erro de banco (SQL), de migração, de I/O, de serialização e de sistema operacional;
- **erro de schema à frente**: o banco tem migração que o binário não conhece; o banco não é aberto;
- **escritor fechado**: a fila de comandos foi encerrada;
- **colisão de sessão**: mesmo id de sessão com dono ou tipo de agente diferente, sem detalhes de linha;
- **colisão de caminho de página**: chave portável já ocupada por outra página viva;
- **erro de pânico no pool**: leitura que entrou em pânico;
- **estado inválido**: por exemplo, sessão receptora inexistente, batão já reclamado por outra sessão,
  operador incompatível com o filtro de dono, ou KDF saturado (HTTP 429);
- **registro malformado**: dado persistido não reconhecido;
- **acesso proibido**: com motivo, por exemplo projeto restrito ou usuário apagado;
- **não encontrado**, **entrada inválida** (nome de workspace, projeto ou workstream), **nome já existente**
  (workspace, projeto ou workstream), **nome de projeto ambíguo**, **workspace não vazio**, **sessão purgada**,
  **execução gerenciada ativa**, **workstream ocupado** e **duplicado**;
- **erro do domínio de memória**, encapsulado.

---

## 20. Notas para reimplementação

1. Comece pelo mínimo: workspaces, projetos, páginas (com índice de texto), sessões, observações, links e
   trilha de auditoria. Com o escritor único e o upsert de página, já é possível gravar página e buscar por
   texto.
2. Implemente o escritor como uma thread dedicada com fila limitada e canal de resposta por comando. **Não**
   use conexão compartilhada protegida por mutex.
3. Use `is_latest` e o índice único parcial desde o início. A supersessão é o que viabiliza histórico,
   concorrência segura e consulta temporal.
4. Idempotência de ingestão = `(projeto, chave)` + `completed_at`. Separe "reivindicar" de "completar".
5. Mantenha ranking e decaimento como funções puras (decaimento, crença e autoridade), testáveis sem banco.
   Deixe os sinais experimentais **desligados por padrão**.
6. Teste explicitamente com várias sessões e vários usuários (invariante #16). Testes de uma sessão só não
   revelam defeitos de colaboração.
7. Trate o tombstone como estado de primeira classe. Uma escrita nova em caminho com tombstone deve ressuscitar
   a cadeia, não criar uma cadeia órfã.
