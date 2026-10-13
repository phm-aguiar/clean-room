# Camada de arquivos do wiki (markdown é a fonte da verdade)

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

> **Papel:** dono do wiki em disco. É o **único caminho correto de escrita** de
> páginas: higieniza o conteúdo, passa pelos webhooks de admissão, grava o arquivo
> de forma **atômica**, replica a alteração para o índice relacional (o banco nunca
> diverge do arquivo), gera embeddings, versiona em Git e reconcilia edições externas
> (Obsidian, editores de texto) por um observador de arquivos. Também contém as
> migrações de estrutura do wiki e o backup prévio à migração para o formato OKF.

Dependências conceituais: o domínio compartilhado (tipos de página, identificadores,
higienização de texto), a camada de armazenamento (escritor único e leitores), a
interface de embedding do cliente de modelo de linguagem, Git embutido, observação de
arquivos com *debounce*, serialização YAML e cliente HTTP para os webhooks.

Termos usados neste documento: **página** é um arquivo markdown do wiki com
frontmatter opcional; **projeto** e **workspace** são os escopos de agrupamento;
**OKF** é o formato de bundle descrito em [okf.md](okf.md).

---

## 1. Layout em disco

```text
<data_dir>/wiki/                       ← raiz (repositório Git)
  <workspace_id>/
    <project_id>/
      _meta.md                         ← manifesto do escopo (nomes humanos ↔ identificadores)
      index.md                         ← raiz do bundle OKF (declara okf_version)
      log.md / log-AAAA-MM.md          ← ledger bruto de eventos de hook (somente acréscimo)
      sessions/<session_id>.md
      concepts/ decisions/ gotchas/ procedures/ …   ← páginas de conteúdo, por tipo
      _pending/                        ← sidecars de propostas (não indexados)
```

Regras:

- **Todo** caminho é derivado pela raiz do wiki e pelos identificadores do escopo;
  nenhum componente monta caminhos por concatenação manual.
- O diretório do projeto usa **identificadores (UUID)**, não nomes: renomear projeto
  ou workspace não move arquivos. `_meta.md` guarda os nomes humanos e permite que a
  reindexação reconstrua os escopos a partir do disco (ver `ReindexSummary` na seção 5).
- Autor e e-mail dos commits automáticos são fixos: nome `ai-memory` e e-mail
  `ai-memory@local`. Não vazam a identidade Git do usuário.

---

## 2. Componentes

| Componente | Responsabilidade |
|---|---|
| Wiki | Escrita, leitura, remoção, purga, movimentação, reindexação e aprovação de propostas. Ponto único de escrita de páginas. |
| Markdown | Leitura e emissão de frontmatter YAML, cálculo de título e extração de links. |
| Escrita atômica | Gravação por arquivo temporário + sincronização + troca, com tolerância a bloqueios transitórios no Windows. |
| Adaptador Git | Inicialização, commits, checkpoints, leitura de arquivos em revisões antigas e escritas rastreadas. |
| Observador (watcher) | Eventos de arquivo com *debounce* e reconciliação periódica entre disco e índice. |
| Admissão | Cadeia de webhooks HTTP executada antes e depois de operações. |
| Confinamento | Recusa links simbólicos e pontos de reparse na árvore do wiki (seção 12). |
| Ledger | Reconhece o ledger bruto de eventos `log*.md`, por nome **e** conteúdo. |
| Backup | Arquivo verificado do diretório de dados antes de migrações. |
| Migrações | Framework de migrações de estrutura do wiki e a migração de conformidade OKF. |
| Erros | Tipo de erro único do wiki (seção 11). |

---

## 3. Markdown: leitura, emissão, títulos e links

### 3.1 Frontmatter

- Só é reconhecido o **bloco canônico** no início do documento: uma linha `---`, o
  conteúdo YAML, e uma linha `---` de fechamento seguida de quebra de linha. As
  quebras podem ser **LF ou CRLF** (um editor no Windows com conversão automática de
  fim de linha grava CRLF; ignorar isso faria o bloco virar corpo e duplicaria o
  frontmatter na migração OKF).
- Um **BOM UTF-8** no início é descartado antes de tudo. Se permanecesse no corpo, ocultaria
  o primeiro título e seria regravado depois do bloco de frontmatter.
- Um bloco sem fechamento, ou sem quebra de linha depois do fechamento, não é
  frontmatter: o documento inteiro é tratado como corpo. Um bloco vazio (duas linhas
  `---` consecutivas) também não é reconhecido.
- Frontmatter que existe mas não é YAML válido gera erro de YAML (seção 11).
- O corpo mantém suas quebras de linha originais em qualquer caso.
- A leitura usa o parser YAML diretamente, em vez de um extrator de frontmatter de
  terceiros, porque esses extratores perdem comentários e a ordem das chaves. Como
  consequência, a reemissão não preserva comentários do frontmatter original.

### 3.2 Emissão

- Se o frontmatter está ausente, nulo ou é um objeto vazio, o documento sai sem
  bloco. Caso contrário, sai o bloco YAML (com quebras LF) seguido do corpo.

### 3.3 Título

Ordem de prioridade:

1. campo `title` do frontmatter, se for texto e não vazio após remover espaços das pontas;
2. primeira linha do corpo que começa com `# ` e tem texto depois (também após remover espaços);
3. nome do arquivo sem o sufixo `.md` (sem o caminho de diretórios).

### 3.4 Links

**Ignorado como link:**

- conteúdo de blocos cercados por três ou mais crases ou tis; o fechamento precisa usar
  o mesmo caractere e pelo menos o mesmo tamanho, e não pode ter texto depois;
- trechos de código inline (entre crases): uma sequência de crases só fecha com
  outra sequência de mesmo tamanho na mesma linha; sem fechamento, as crases são texto.
  Trechos que continuam na linha seguinte não são vistos;
- imagens (link precedido de `!`);
- URLs com esquema (contêm `://`) e esquemas `mailto`, `data`, `javascript`,
  `vbscript`, `tel` e `file`;
- âncora pura (`#...`), destinos com barra invertida, destinos vazios;
- destinos que apontam para diretório (terminam em barra);
- destinos cujo último segmento tem ponto mas não termina em `.md`.

**Reconhecido:**

- wikilinks `[[caminho]]` e `[[caminho|rótulo]]`;
- wikilinks com escopo: `[[projeto:caminho]]` (mesmo workspace) e
  `[[workspace/projeto:caminho]]` (outro workspace);
- `[[_global:caminho]]`: forma reservada do projeto de preferências, que fica no
  workspace padrão (e não no workspace da página de origem), a menos que o escopo
  explícito `workspace/_global` seja usado;
- links markdown `[rótulo](destino)`. O destino é lido até no máximo 512 bytes; um
  título opcional entre aspas é descartado.

**Normalização:**

- Antes de resolver, são descartados a consulta (`?...`) e a âncora (`#...`) do destino.
- Destino sem extensão recebe `.md` no final.
- Destino que começa com barra ou é wikilink é relativo à raiz do projeto; os demais são
  relativos ao diretório da página de origem (com resolução de `..`).
- Resultados são deduplicados e ordenados (saída estável).
- Destinos que não formam um caminho de página válido são descartados silenciosamente.

### 3.5 Relações tipadas

- Lidas da chave `relations` do frontmatter, com a mesma gramática de alvo dos wikilinks.
- O vocabulário é **fechado**: `causes` (causa), `fixes` (corrige), `contradicts` (contradiz).
  Chave fora desse vocabulário é ignorada com aviso, para que um erro de digitação não
  crie um tipo de aresta novo.
- Alvo que não nomeia uma página (diretório, `.md` sem nome, extensão diferente de `.md`)
  é ignorado com aviso.
- Valores de log vindos do frontmatter são truncados a 200 bytes, por serem entrada
  não confiável.

### 3.6 Conjunto de links da página

Todo caminho de escrita grava o **conjunto completo** de links de saída: os links do
corpo somados às relações do frontmatter. Há uma única função de extração para isso,
para que um novo ponto de escrita não esqueça a parte tipada.

---

## 4. Escrita atômica

Gravar um arquivo substituindo o anterior segue estas etapas:

1. cria um arquivo temporário **no mesmo diretório** do destino, com prefixo
   `.ai-memory-tmp.`;
2. escreve o conteúdo e sincroniza os dados do temporário com o disco;
3. troca o temporário pelo destino (renomeação sobre o arquivo existente);
4. sincroniza o diretório pai, em modo de melhor esforço.

Detalhes:

- O observador (seção 8) ignora arquivos com o prefixo do temporário.
- **No Windows**, a troca pode falhar com violação de compartilhamento ou de bloqueio
  (códigos de sistema 32 e 33), geralmente causada por antivírus ou indexador que
  está lendo o arquivo recém-criado. Nesses casos a troca é repetida até 5 vezes, com
  espera linear de 10, 20, 30, 40 e 50 ms (no máximo cerca de 150 ms). Se o problema
  persistir, o erro original é propagado.
- **Em outras plataformas** nenhum erro é considerado transitório.
- A gravação devolve um identificador do arquivo gravado (número de inode no Unix;
  índice de arquivo NTFS no Windows, com valor zero quando o volume não expõe esse
  índice). O observador usa esse identificador para reconhecer as próprias escritas.

---

## 5. Escrita de página: fluxo passo a passo

### 5.1 Estado do componente

O componente Wiki é compartilhável entre tarefas e mantém:

- raiz do wiki, escritor do banco, adaptador Git;
- embedder opcional (ausente = nenhum embedding é gerado);
- higienizador de texto, cadeia de admissão opcional e leitor do banco opcional;
- **guarda de mutação** do tipo leitor/escritor: escrita de página usa a guarda
  compartilhada; remoções, purgas, movimentações e manutenção de escopo usam a guarda
  exclusiva;
- **trava por página**: uma trava assíncrona por combinação (workspace, projeto,
  caminho), com coleta oportunista das travas sem uso;
- conjunto de escopos cujo manifesto já foi garantido nesta sessão.

Construção: a configuração é encadeada (cadeia de admissão, leitor do banco, higienizador,
embedder ou ausência de embedder). Sem leitor do banco, a resolução de nomes humanos
para webhooks fica vazia.

### 5.2 Escrita de uma página

Ordem fixa:

1. **Confinamento e portabilidade.** Verifica se o caminho cabe na árvore do projeto sem
   atravessar links simbólicos (seção 12), e depois se o caminho é portável em todos os
   sistemas operacionais suportados (`ensure_portable`). Ambas as verificações acontecem
   **antes de qualquer escrita**. A portabilidade é checada aqui, e não na criação do
   caminho, para não quebrar a leitura de páginas antigas. Um caminho que não pode ser
   gravado e registrado em Git causaria uma página que não pode ser criada nem removida
   pela API normal.
2. **Higienização do corpo.** O corpo passa pelo higienizador, qualquer que seja o
   chamador (consolidação por LLM, CLI, MCP), como defesa em profundidade.
3. **Fixação e autoria.** Caminhos de slot são marcados como fixados. Se o ator não é
   anônimo, o frontmatter recebe o bloco `last_modified_by` (com identificador do autor,
   quando houver). Ator anônimo não gera bloco.
4. **Cadeia de admissão** (se configurada). Os nomes humanos de workspace e projeto
   são resolvidos para o contexto e os webhooks **bloqueantes** são executados em
   ordem. Eles podem alterar frontmatter e corpo, ou rejeitar a escrita.
5. **Nova higienização e normalização.** O corpo e as strings do frontmatter são
   higienizados de novo, porque webhooks são entrada externa. O campo `pinned` também
   é lido do frontmatter. Frontmatter de índice recebe a forma canônica (nível de
   retenção e fixação) e a conformidade OKF para o disco.
6. **Derivações.** Título (explícito, ou pela regra da seção 3.3), links (seção 3.6),
   validade, entidades e resumo. Esses valores são calculados depois da cadeia, para
   que alterações de webhooks entrem neles:
   - `expires_at`: ausente, nulo ou vazio significa sem validade. Aceita data e hora
     em RFC 3339, ou apenas a data (`AAAA-MM-DD`), que vale até o fim desse dia em UTC
     (23:59:59,999999). Qualquer outro valor, ou valor que não seja texto, é erro. O
     erro é de propósito: um valor inválido não pode significar "nunca expira" por
     engano;
   - `entities`: ausente, nulo ou lista são aceitos. Qualquer outro tipo é erro. As
     etiquetas (`tags`) também geram entidades, com normalização (minúsculas, espaços
     colapsados, sem duplicatas, valores que não são texto descartados);
   - `abstract`: texto do frontmatter, usado como resumo de uma linha. Vazio ou que não
     é texto é ignorado.
7. **Emissão e trava.** O markdown é emitido. Em seguida são adquiridas a guarda
   compartilhada de mutação e a trava da página. Dentro dessas guardas o confinamento é
   verificado de novo, e são garantidas as linhas do projeto e do workspace e o
   manifesto `_meta.md`.
8. **Instalação do arquivo.** O arquivo é gravado guardando o conteúdo anterior, para
   rollback. Isso acontece **antes** da gravação no índice: o markdown nunca fica atrás
   do banco.
9. **Gravação no índice.** Cria a nova versão da página e marca a anterior como
   superada. Se a gravação falhar, o arquivo anterior é restaurado. Se a restauração
   também falhar, o sistema retorna um erro explícito de **estado inconsistente**
   (arquivos mudaram, o índice não, e a reversão falhou), em vez de mascarar o problema.
10. **Embedding.** Se houver embedder, o corpo é embutido **na mesma execução** que a
    escrita, sem indexação em segundo plano. Se o resumo existir, ele recebe um embedding
    próprio. Falha de embedding gera aviso e registro da falha no índice, mas **não
    desfaz a escrita**. Se a página reescrita não tem mais resumo, o embedding de resumo
    anterior é removido.
11. **Webhooks não bloqueantes.** São disparados depois que a página foi persistida e
    indexada, e não podem alterar nem rejeitar a escrita.

Propriedades observáveis:

- Reescrever conteúdo idêntico não altera os bytes do arquivo nem cria uma nova versão.
- Criar uma página cujo caminho coincide com o de outra página viva do mesmo projeto,
  após converter para minúsculas e normalizar Unicode (forma composta), é recusado,
  mesmo que os dois caminhos sejam diferentes no sistema de arquivos. O erro cita os
  dois caminhos. A regra vale em todos os sistemas, para que o wiki sincronizado entre
  plataformas não dependa do sistema de arquivos local. A checagem só roda em criações;
  reescritas de uma versão existente não podem criar esse conflito.

### 5.3 Escrita em lote

Mesma lógica para N páginas, com **atomicidade**:

1. valida a portabilidade e o confinamento de todos os caminhos;
2. pré-calcula o markdown de cada página (com higienização e cadeia de admissão);
3. adquire as travas por página **em ordem determinística** (ordenadas e sem repetição),
   o que evita deadlock com outros escritores;
4. grava todos os arquivos temporários e os sincroniza;
5. instala os arquivos, guardando o conteúdo anterior de cada um;
6. grava todas as páginas no índice em **uma única transação**;
7. se a transação falhar, reverte todos os arquivos instalados;
8. só depois de tudo gravado, dispara os webhooks não bloqueantes de cada página.

Chamadores: consolidação, bootstrap, varredura de manutenção e o passe de consolidação
em lote (*dream*).

### 5.4 Outras operações

**Leitura:** leitura de página, caminho absoluto da página, raiz do projeto, lista dos
checkpoints recentes e checkpoint de linha de base para atualização (ver seção 7).

**Remoção:**

- `delete_page`: remove a página. A cadeia de admissão é consultada **antes** de qualquer
  alteração, e um webhook bloqueante que rejeite interrompe a operação.
- `delete_page_if_latest`: só remove se a versão atual ainda for a esperada (verificação
  de versão, para evitar apagar algo que foi reescrito no meio do caminho).
- `evict_page_if_latest` (remoção por decaimento): segue a mesma verificação de versão.
  Move o arquivo para uma área de quarentena, apaga a versão no índice e cria um
  marcador de decaimento (tombstone). Se a remoção no índice falhar, o arquivo é
  restaurado da quarentena. Em caso de sucesso, a cópia em quarentena é apagada.
- `hard_delete_decay_tombstone`: apaga definitivamente um marcador de decaimento antigo e
  sua cadeia de versões. Nunca apaga um arquivo que existe. Se um arquivo recriado no
  mesmo caminho ainda não foi indexado, ele é reindexado antes.

**Escopo:**

- `purge_project`, `purge_session`, `purge_workspace`: removem dados de um escopo. Antes de
  tocar em qualquer coisa, consultam a cadeia de admissão (`admit_purge_*`).
- `move_project_workspace`: move um projeto para outro workspace **sem mudar o identificador
  do projeto**. Usa a guarda exclusiva, renomeia o diretório e atualiza o índice.
- `move_session_page`: move a página de uma sessão para outro projeto.
- `remove_project_dir`, `remove_workspace_dir`: removem diretórios de escopo.
- Propostas de automelhoria: escrevem um sidecar de revisão que **não é indexado**
  (`write_auto_improve_sidecar`). A aprovação (`approve_auto_improve_proposal`) aplica a
  proposta pelo mesmo caminho de escrita da seção 5.2 e trata conflito revertendo o
  arquivo instalado.

**Restauração e reindexação:**

- `restore_page_from_checkpoint`: lê a página na revisão indicada pelo histórico Git e a
  regrava pelo caminho normal.
- `reindex_page`: reindexa uma página a partir do disco.
- `reindex_all`: reconstrói o índice inteiro a partir do disco. Pula escopos e sessões
  purgados que têm marcador (não os ressuscita) e pula pares de caminhos que só diferem
  por caixa ou Unicode (contados em `skipped_collisions`). O resumo (`ReindexSummary`)
  informa workspaces, projetos, páginas e os contadores de pulos.
- `backfill_scope_manifests`: cria o `_meta.md` de escopos que não o têm.
- `repair_date_only_stale_after`: converte validades só com data para o instante de fim
  do dia, no formato OKF.

**Ledger:** `append_under_project` acrescenta bytes a um arquivo `log*.md` do projeto e
reporta a alteração ao Git.

**Admissão:** `preflight_admission` consulta a cadeia **antes** de trabalho caro (por
exemplo, antes de chamar um modelo de linguagem), sem gravar nada. `authorize_operation`,
`notify_operation_observers` e `dispatch_purge` seguem a regra da seção 6.

---

## 6. Admissão por webhooks

Único ponto de extensão do motor (princípio aberto/fechado): novos comportamentos viram
**serviços HTTP externos**, e o motor não cresce para cada um deles. Ver também
[admission-webhooks.md](admission-webhooks.md).

### 6.1 Operações que disparam a cadeia

Valores enviados no cabeçalho `X-Memory-Op`:

`write_page`, `consolidate`, `delete`, `purge_project`, `purge_session`,
`purge_workspace`, `move_project`, `move_session`, `handoff_begin`, `handoff_accept`,
`handoff_cancel`, `message_send`, `message_pop`, `message_cancel`.

Escrita de página, escrita em lote e aprovação de propostas executam a cadeia com
alteração (`run`). Operações sem corpo (remoção, purga, movimentação, transferências e
mensagens) usam notificação e autorização (ver 6.4).

### 6.2 Configuração de cada webhook

Vem da seção de webhooks de admissão do arquivo de configuração:

| Campo | Padrão | Significado |
|---|---|---|
| nome | obrigatório | Identificador estável, usado em listas de exceção e nos logs. |
| URL | obrigatório | Endpoint que recebe o POST. |
| tempo limite (ms) | 2000 | Limitado ao intervalo de 1 a 30000 ms. Valores fora dele são ajustados, não recusados. |
| política de falha | ignorar | `ignorar` registra e segue; `rejeitar` aborta a operação. |
| eventos | vazio | Lista de operações. Lista vazia = webhook desabilitado. |
| bloqueante | verdadeiro | Se verdadeiro, roda de forma síncrona e pode alterar ou rejeitar. Se falso, roda depois da persistência e apenas observa. |

A cadeia aceita no máximo **16 webhooks**. Uma configuração com mais que isso é recusada
na construção.

### 6.3 Requisição e resposta

Requisição: POST com JSON contendo:

- `page`: caminho, frontmatter e corpo da página (vazios nas operações sem corpo);
- `ctx`: workspace e projeto (nomes humanos quando resolvidos), destino em movimentações,
  ator, operação e indicador de falha parcial. O campo de destino aparece só quando
  existe, e o indicador de falha parcial só aparece quando é verdadeiro.

Resposta de um webhook bloqueante:

- **200** com envelope `page`: substitui os campos presentes (frontmatter e/ou corpo).
  Campos ausentes continuam como estavam;
- **204**: nenhuma alteração; a cadeia segue;
- **3xx, 4xx, 5xx ou erro de rede**: se a política for `ignorar`, registra aviso e segue
  com a página sem alteração. Se for `rejeitar`, a operação falha com erro de I/O;
- **corpo acima de 1 MiB** ou **JSON inválido**: tratado como sem alteração, com aviso.

Execução sequencial, na ordem da configuração. Cada webhook vê a saída do anterior.

### 6.4 Webhooks não bloqueantes e notificações

- Webhook não bloqueante é disparado **sem esperar** a resposta, depois que a operação é
  durável. Não altera nem rejeita nada.
- Um webhook é **decisivo** quando é bloqueante **e** tem política `rejeitar`.
- Em escrita de página e em lote, todo webhook bloqueante roda de forma síncrona, na
  ordem da configuração, e pode alterar ou rejeitar.
- Em operações sem corpo, só webhooks decisivos são aguardados antes da operação, e podem
  recusá-la. Os demais webhooks dessas operações são observadores, notificados depois que
  a operação é concluída.
- Limite de **256 requisições em voo** no processo, compartilhado por todos os disparos.
  Quando o limite é atingido, a requisição é descartada e registrada:
  - se o webhook está configurado como bloqueante, o registro é de **erro**, porque a
    configuração diz que ele deve rodar;
  - se é observador, o registro é de **aviso**.
  A operação durável já aconteceu, então o descarte nunca bloqueia quem chamou.

### 6.5 Anti-loop

- Cabeçalho `X-Memory-Skip-Admission-Chain` com nomes de webhooks separados por vírgula.
  Espaços em volta e entradas vazias são ignorados.
- O cabeçalho só é honrado para acessos com a capacidade de pular a cadeia. Um usuário
  comum não tem essa capacidade, e a lista enviada por ele é descartada. Isso impede que
  um cliente comum contorne um webhook de rejeição.
- Webhooks listados são pulados tanto nas chamadas síncronas quanto nas assíncronas.

---

## 7. Histórico Git

- Um adaptador abre o repositório do wiki e **mantém-no aberto entre commits**. Reabrir
  a cada commit descartaria o cache de árvores do índice e obrigaria a reconstruir todas
  as árvores em cada commit.
- Um commit por vez por repositório. Concorrência é serializada por trava: commits
  concorrentes esperam, em vez de falhar. Outro adaptador aberto sobre a mesma raiz não
  compartilha essa trava.
- **Escritas rastreadas.** Gravar, acrescentar, renomear, remover arquivo e remover
  diretório passam pelo adaptador, que registra os caminhos afetados. Um lint do
  componente proíbe chamadas diretas de gravação de arquivo fora desse caminho.
- **Commit por caminho.** Só os caminhos registrados são incluídos. Uma **varredura
  completa** da árvore é feita quando:
  - o último registro não é confiável (`walk_needed`);
  - nenhum caminho foi registrado desde o último commit;
  - passaram 600 s desde a última varredura.

  A varredura compara a árvore com os caminhos registrados e conta as escritas que não
  foram reportadas (`unreported_writes`), guardando uma amostra de até 5 caminhos.
  Escrita não reportada significa que alguém contornou o wiki.
- Se o HEAD mudou por fora do adaptador, o índice é relido e a próxima passagem varre
  a árvore.
- **Leitura em corrida.** Se um arquivo muda enquanto é lido para o commit (um escritor
  fora da trava), o commit tenta de novo: até 4 tentativas, com 25 ms de espera entre
  elas. Se ainda falhar, os caminhos continuam registrados para o próximo commit.
- `commit_all` devolve nenhum identificador de commit quando não há mudança.
- **Fallback.** Se o acesso pela biblioteca Git embutida falha porque o repositório não
  foi encontrado, ou (só no Windows) porque um caminho não pôde ser resolvido, o
  adaptador tenta o comando `git` do sistema, depois das mesmas validações de
  confinamento. Erro de dono do repositório (*owner*) **nunca** aciona o fallback.
- Leituras de histórico: lista dos checkpoints recentes (mais novos primeiro), contagem
  de commits e leitura do conteúdo de um arquivo em uma revisão. Esta última é a base da
  restauração de página.
- **Commits automáticos** acontecem: no fim de sessão (roteador de hooks), na
  consolidação, no bootstrap, no comando de commit (CLI e administração do MCP), ao
  renomear projeto, ao reparar datas de validade, na linha de base de atualização
  (checkpoint dos arquivos já existentes antes das primeiras mudanças) e nas migrações.
- **Rollback de arquivo.** Toda instalação guarda o conteúdo anterior (ou a ausência
  dele). A reversão restaura o conteúdo anterior ou remove o arquivo novo, e reporta se
  a reversão em si falhou (seção 5.2, passo 9).

---

## 8. Observador (watcher)

Dois mecanismos complementares.

### 8.1 Eventos com *debounce*

- Janela de *debounce* de **300 ms**.
- Criação, modificação ou evento genérico em arquivo `.md`: lê o disco, interpreta a
  página e reindexa. As próprias escritas do wiki são absorvidas por uma checagem de hash
  de conteúdo no índice: reindexar um conteúdo idêntico não faz nada, e o ciclo termina.
- Remoção: só registra o caminho no Git, para que o próximo commit o inclua. Não mexe
  no índice (ver 8.3).
- Criação de diretório dentro de um projeto: reindexa o projeto inteiro.
- **Ignorados:** caminhos internos do Git (inclusive metadados aninhados), links simbólicos
  e pontos de reparse, arquivos temporários do wiki, `_meta.md`, `bootstrap.md`, ledgers
  `log*.md` (por nome **e** conteúdo, seção 9), caminhos em `_pending/` e arquivos que não
  são markdown.
- A página de uma sessão **purgada** não volta por evento: se o caminho é `sessions/<id>.md`
  e a sessão está na lista de purgadas, o evento é ignorado. Se a consulta falhar, o evento
  é processado mesmo assim, porque um erro transitório não deve parar a indexação.
- Atribuição do evento ao escopo: os dois primeiros segmentos do caminho relativo à raiz
  (identificadores). Eventos fora desse formato são ignorados.
- O observador **nunca escreve no disco**. Edições externas alimentam o índice; escritas
  internas passam pelo fluxo da seção 5.

### 8.2 Reconciliação periódica

- A cada **30 s** a árvore inteira é percorrida e as páginas são reindexadas. Isso cobre
  eventos que o sistema operacional perdeu (buffers de notificação, montagens de rede).
- O primeiro disparo ocorre no início e é descartado, para que a reconciliação não rode
  logo no boot.
- Diretórios ocultos (nome começando com ponto) **não** são pulados.
- Ignora links simbólicos, arquivos temporários, `_pending/` no nível da raiz, `_meta.md`,
  `bootstrap.md` e ledgers.
- Contador de falhas consecutivas: ao chegar a **5**, registra um erro de nível alto com
  o marcador `watcher_degraded` (o índice pode estar fora de sincronia com o disco).
  A reconciliação continua tentando a cada 30 s. Ao voltar a funcionar, registra a
  recuperação.

### 8.3 Remoção de páginas que sumiram do disco (opcional)

Desligada por padrão. Ativada pela chave `reconcile_tombstones_deleted_pages` na seção de
manutenção do arquivo de configuração.

- **Sem a opção**, uma página cujo arquivo foi removido por fora do sistema **continua no
  índice**. A reconciliação só reindexa o que existe no disco.
- **Com a opção**, o observador compara os caminhos de versões atuais do índice com os
  caminhos percorridos. Só entram como candidatos as páginas que poderiam ter sido
  percorridas. Ficam de fora: pendências, metadados, `bootstrap.md`, ledgers e páginas de
  sessão (`sessions/<id>.md`).
- Um candidato só é marcado após estar **ausente em duas passagens consecutivas**, com a
  mesma versão atual. Se a versão mudou entre as passagens, a contagem recomeça.
- **Varredura parcial.** Se um diretório sumiu no meio da varredura (checkout, montagem
  ou mudança concorrente de projeto), a ausência não conta como evidência de remoção.
- **Disjuntor.** Se, no escopo, o número de candidatos ausentes ultrapassa o maior valor
  entre **3** e a **metade do total de candidatos** (arredondada para cima), o escopo
  inteiro é ignorado naquela passagem. Isso evita marcar tudo como removido por causa de
  um problema de montagem ou de permissão.
- Antes de marcar, sob a guarda exclusiva de mutação, a marcação confere de novo que a
  versão atual continua a mesma e que o arquivo continua ausente. Se qualquer coisa mudou,
  nada é feito.
- A marcação é uma **remoção lógica** (soft delete): a página fica invisível para a
  recuperação normal, mas não é apagada do banco. Não toca o disco. Webhooks não
  bloqueantes de observação são disparados. Webhooks bloqueantes **não** são consultados,
  porque não há ação do usuário a autorizar.

---

## 9. Ledger de eventos

`log.md` e `log-AAAA-MM.md` são o ledger bruto dos hooks: entradas no formato
`## [timestamp] evento | título`.

Um arquivo só é tratado como ledger se o **nome casa e o conteúdo também**:

- o nome deve ser `log.md` ou `log-AAAA-MM.md` exatamente;
- o conteúdo deve **abrir** com uma entrada de log. Se o arquivo começa com um bloco de
  frontmatter, o bloco precisa ser fechado (dentro de um limite de linhas) e a primeira
  linha não vazia depois dele deve ser a entrada. Linha vazia antes da primeira entrada
  já decide que não é ledger.
- Só os primeiros 8 KiB são lidos para essa decisão. Um arquivo com entrada de log
  depois desse ponto não é reconhecido como ledger.

Sem essa checagem, uma página comum com nome parecido seria tratada como ledger, ou cada
acréscimo ao ledger superaria uma "página" de vários megabytes. A mesma regra é usada
pelo observador, pela migração de conformidade OKF e pela exportação de bundle OKF.

---

## 10. Migrações de estrutura do wiki

### 10.1 Framework

- Cada migração tem um **nome** estável e uma descrição curta para os logs, e uma
  operação `up` que recebe o escritor do banco e a raiz do wiki.
- Nome no formato `AAAA_MM_DDTHH_MM_descricao`, com horário UTC escolhido na criação. Usar
  horário em vez de número sequencial evita conflitos de merge quando duas pessoas criam
  migrações em paralelo.
- O registro de migrações é uma lista **somente com acréscimo**: nunca se reordena nem se
  remove uma entrada.
- A execução consulta a tabela de migrações aplicadas e roda as pendentes em ordem.
  Uma falha aborta a inicialização **sem** registrar a migração, então a próxima
  inicialização tenta de novo.
- **Guarda de geração.** Se o banco registra uma migração que este binário não conhece,
  significa que o wiki foi migrado por uma versão **mais nova**. A execução falha com erro
  de formato mais novo (seção 11). O erro orienta a atualizar o programa ou a restaurar o
  backup.
- Regras para migrações novas:
  - devem ser **idempotentes** (se o trabalho já foi feito, retornam sem tocar em nada);
  - devem ser rápidas e **não chamar modelo de linguagem**;
  - não executam SQL fora do escritor do banco;
  - não apagam nada sem antes mover para uma área de descarte com carimbo de tempo
    (`_graveyard/<timestamp>/`), para que os dados sejam recuperáveis por pelo menos um
    ciclo de release.

### 10.2 Migração de conformidade OKF

Nome registrado: `2026_09_01T18_00_okf_v02_conformance`. Descrição: conforma páginas e
linhas do índice ao OKF v0.2, com backup obrigatório, feito in loco.

Ordem fixa, em que cada passo condiciona o próximo:

1. **Backup verificado do diretório de dados inteiro.** Sem backup verificado, a migração
   não roda. Se já existe um recibo de backup válido (criado antes da abertura do banco,
   na inicialização), ele é reaproveitado. Só cria um backup novo quando não existe um.
2. **Checkpoint Git** do estado atual da árvore do wiki.
3. **Passe no banco:** conforma cada versão atual de página **in loco**: mesmo
   identificador, mesma linha de versão, sem alterar a data de atualização.
4. **Passe nos arquivos:** conforma cada `.md` **in loco**, sem alterar o corpo.
   - A data de geração (`generated.at`) vem da data de atualização da linha, quando o
     banco foi reescrito nesta migração. Caso contrário, vem da data de modificação do
     arquivo. Páginas já conformes não mudam.
   - `_meta.md` de escopo ganha apenas o campo `type`.
   - Cada diretório de projeto ganha um `index.md` declarando `okf_version: "0.2"`, quando
     ainda não tem um.
5. **Um único commit** `okf-migration: conform wiki to OKF v0.2` com toda a reescrita.

Idempotente: em um wiki já conforme, os dois passes não encontram nada e o backup não é
criado. Instalação nova também não cria backup, porque não há nada a proteger.

Um detalhe de ordem: a abertura do banco pode ter atualizado o esquema antes da migração
de wiki. Por isso, na inicialização, o backup é criado **antes** da abertura do banco
(snapshot anterior à migração de esquema). Sem esse snapshot, o rollback para a versão
anterior não seria possível.

### 10.3 Backup verificado

- Arquivo comprimido do diretório de dados, **fora** dele. O destino padrão é o diretório
  pessoal do usuário. Em contêiner, o diretório pessoal é efêmero, então o destino padrão
  passa a ser uma subpasta do diretório de dados. A variável `AI_MEMORY_BACKUP_DIR`
  substitui o destino em qualquer caso. Um destino igual à raiz do diretório de dados é
  recusado. O próprio arquivo de backup nunca entra no arquivo de backup.
- A detecção de contêiner usa a variável `AI_MEMORY_IN_CONTAINER` (qualquer valor não
  vazio) ou os marcadores de contêiner do sistema.
- **Verificação:** após gravar, o arquivo é lido de volta e o número de entradas é
  comparado com o número de arquivos percorridos. Divergência ou arquivo vazio: o arquivo
  é apagado e a migração é recusada.
- Recibo `pre-migration-backup.json` gravado na raiz do diretório de dados, com caminho do
  arquivo, tamanho, número de entradas e espaço livre no destino (apenas informativo; se a
  leitura do espaço falhar, o backup continua válido). A página inicial da interface web
  mostra um aviso com o caminho do backup até que o arquivo seja apagado.
- Se o recibo aponta para um arquivo que não existe mais, o recibo é tratado como ausente.

---

## 11. Erros

Tipo de erro único do wiki, **não exaustivo** (novas variantes podem ser adicionadas):

| Variante | Quando |
|---|---|
| Entrada/saída | Falhas de sistema de arquivos, inclusive a rejeição de um webhook bloqueante. |
| Confinamento | Caminho ou metadado recusado por links simbólicos, pontos de reparse ou metadados Git redirecionados. Traz o caminho e o motivo. |
| Persistência de temporário | Falha ao trocar o arquivo temporário pelo destino. |
| YAML | Frontmatter malformado. |
| JSON | Falha de serialização ou leitura de JSON. |
| Memória | Erro do domínio compartilhado (por exemplo, caminho de página inválido). |
| Armazenamento | Erro do banco. |
| Destino já existe | Diretório de destino de uma movimentação já existe. |
| Página de destino já existe | Arquivo de destino de uma movimentação já existe. |
| Formato mais novo | O wiki foi migrado por uma versão mais nova (seção 10.1). |
| Dono do repositório | Falha na checagem de dono do repositório Git. |

Um webhook rejeitado aparece como erro de entrada/saída.

---

## 12. Confinamento do armazenamento em disco

Toda E/S da árvore `<wiki>/<workspace_id>/<project_id>/…` passa pelo confinamento. Assim,
um link simbólico (ou ponto de reparse no Windows) colocado dentro do wiki nunca desvia
uma leitura ou escrita para fora da raiz. O erro é sempre o de confinamento, com caminho e
motivo.

- **Detecção de link.** Um item é "tipo link" se for um link simbólico ou, no Windows, se
  tiver o atributo de ponto de reparse. A verificação usa os metadados do próprio item,
  sem seguir o link.
- **Caminho relativo à árvore.** O caminho precisa ser normalizado: não vazio, sem
  componentes `..`, `.`, raiz ou prefixo de unidade. A raiz é verificada, e então cada
  ancestral existente é percorrido: cada um deve ser diretório e não pode ser link.
  Ancestral ausente é criado ou não, conforme o modo:
  - **inspeção:** não cria nada e para na primeira ausência;
  - **pais:** cria os diretórios pais do alvo;
  - **diretório:** cria também o alvo.
  O alvo final, se existir, não pode ser link.
- **Validação da árvore inteira** (`validate_wiki_tree`), usada por rotinas administrativas
  de verificação:
  - o diretório de metadados Git precisa existir **dentro da raiz**, ser diretório comum e
    não ser link;
  - são proibidos os redirecionamentos de objetos Git: diretório comum compartilhado,
    alternativas de objetos e alternativas HTTP de objetos (eles levariam metadados para
    fora da raiz);
  - a configuração de diretório de trabalho do Git (`core.worktree`), no arquivo de
    configuração principal e no específico de worktree, precisa resolver **exatamente** para
    a raiz do wiki (com caminhos canonicalizados). Qualquer outro valor é recusado;
  - a própria pasta de metadados Git é percorrida à parte, e o restante da árvore é
    percorrido excluindo essa pasta.
- **Caminhada.** Percurso iterativo (sem recursão). Qualquer item tipo link gera erro.
  Tolerância apenas a "sumiu": um item que desaparece entre a listagem e a leitura de
  metadados (commit concorrente mexendo em arquivos de bloqueio ou temporários do Git,
  ou troca atômica do escritor) é pulado. Consideram-se transitórios:
  - arquivo não encontrado;
  - permissão negada (no Windows, geralmente violação de compartilhamento);
  - item marcado para exclusão pendente no Windows.

  Mesmo assim, todo item que **consegue** ser examinado passa pela recusa a links.
- Raiz ausente: a caminhada da árvore sozinha passa, porque não há nada a confinar. A
  validação completa da árvore, por sua vez, exige a raiz e seus metadados Git, e falha
  com erro de sistema de arquivos quando a raiz não existe.

---

## 13. Notas para reimplementação

1. **Ordem sagrada na escrita:** arquivo → índice → (embedding) → webhooks assíncronos,
   com reversão do arquivo se a gravação no índice falhar. Inverter a ordem perde a
   garantia de que o markdown é a verdade.
2. **Dois níveis de trava:** uma trava global de leitor/escritor (operações estruturais) e
   uma trava por caminho (escritas concorrentes na mesma página). Ao adquirir travas em
   lote, use uma ordem total para evitar deadlock.
3. **Observador idempotente:** a checagem de hash de conteúdo torna as próprias escritas
   inofensivas. Não tente distinguir "minhas escritas" de "escritas externas".
4. **Sempre ofereça reindexação a partir do disco.** É isso que torna o índice descartável.
5. **Webhooks:** comece só com política `ignorar` e não bloqueantes (observadores). Use
   `rejeitar` apenas para validadores de segurança.
6. **Não confie em correspondência de caminhos entre sistemas de arquivos.** Use a chave
   normalizada (minúsculas e Unicode composto) para detectar colisões, em todas as
   plataformas.
7. **Falha fechada em validade.** Um valor de validade que não entende deve gerar erro, nunca
   ser interpretado como "sem validade".
