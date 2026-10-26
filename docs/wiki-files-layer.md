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
  reindexação reconstrua os escopos a partir do disco (ver o resumo da reconstrução na seção 5.4).
- Autor e e-mail dos commits automáticos são fixos: nome `ai-memory` e e-mail
  `ai-memory@local`. Não vazam a identidade Git do usuário.
- Os nomes de diretório são a forma canônica do UUID (minúsculas, com hífens). Na leitura da árvore
  (observador, reindexação), um diretório que não é UUID válido é ignorado.
- A raiz da wiki é criada na primeira abertura, se não existir, e o repositório Git é inicializado
  nela (seção 7).

### 1.1 Manifestos de escopo (`_meta.md`)

Cada diretório de workspace e cada diretório de projeto tem um `_meta.md` só com frontmatter e corpo
vazio. Formato exato (emitido pelas regras da seção 3.3):

```text
<wiki>/<workspace_id>/_meta.md
---
workspace: <nome do workspace>
type: Scope Manifest
---
```

```text
<wiki>/<workspace_id>/<project_id>/_meta.md
---
project: <nome do projeto>
repo_path: <caminho do checkout>          (só se existe)
identity: <identidade do repositório>     (só se existe; vem junto com identity_source)
identity_source: <origem da identidade>   (só se existe)
canonical_name: <chave canônica>          (só se derivável da identidade)
legacy_name: <nome legado>                (só se derivável da identidade)
type: Scope Manifest
---
```

- As chaves saem nessa ordem; `type` é sempre a última. Chaves sem valor são omitidas.
- A escrita do manifesto é idempotente: se o arquivo já tem exatamente esses bytes, não é tocado.
  Um manifesto que é link simbólico ou ponto de reparse é recusado.
- O manifesto é garantido junto com a primeira página do escopo (seção 5.2, passo 7) e por um
  preenchimento idempotente em toda inicialização do servidor, para árvores antigas ou manifestos
  apagados à mão.
- A leitura (reindexação) exige `workspace` (texto) no do workspace e `project` (texto) no do projeto.
  `identity` e `identity_source` vêm juntos ou não vêm. Se `canonical_name` ou `legacy_name` estão
  presentes, precisam ser iguais aos derivados da identidade. Qualquer violação é erro que cita o
  caminho do arquivo.
- O observador e a reindexação nunca tratam `_meta.md` como página, em qualquer profundidade.

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

### 3.1 Frontmatter: reconhecimento do bloco

Algoritmo, sobre o texto do arquivo:

1. Um **BOM UTF-8** (U+FEFF) no início é descartado antes de tudo, haja ou não frontmatter. Se
   permanecesse no corpo, ocultaria o primeiro título e seria regravado depois do bloco.
2. O texto precisa começar **exatamente** com `---\n` ou `---\r\n`. Qualquer outra coisa (espaço
   antes, `--- ` com espaço depois, `----`) significa sem frontmatter: o documento inteiro é corpo.
3. A quebra da linha de abertura define a quebra esperada no fechamento. O bloco termina na
   **primeira** ocorrência, depois da abertura, de uma quebra de linha seguida de `---` e da mesma
   quebra da abertura (`\n---\n` quando a abertura foi LF, `\n---\r\n` quando foi CRLF). O corpo
   começa logo depois dessa sequência.
4. Se essa sequência não existe, o bloco ainda é aceito quando o texto **termina** em `\n---`, sem
   quebra final (arquivo só de frontmatter salvo por um editor que não põe quebra no fim). O corpo é
   vazio.
5. Sem fechamento, o documento inteiro é corpo.
6. Do conteúdo do bloco são removidos os `\r` finais, e o resto vai para o leitor YAML (§3.2).
   Linhas internas com CRLF são aceitas pelo leitor.

Consequências que um reimplementador precisa reproduzir:

- Quebras **LF ou CRLF** são aceitas (um editor no Windows com conversão automática de fim de linha
  grava CRLF, e o Git com `core.autocrlf=true` faz o mesmo no checkout). Sem isso, o bloco viraria
  corpo e a migração OKF gravaria um segundo frontmatter acima dele.
- Abertura LF com fechamento CRLF (ou o contrário) **não** é reconhecida, exceto no caso do item 4.
- `---\n---\n` (duas linhas seguidas) **não** é bloco: o texto depois da abertura começa com `---`,
  sem a quebra antes. O documento inteiro é corpo.
- `---\n\n---\n` **é** bloco, com YAML vazio: o frontmatter lido é nulo e o corpo é o que vem depois.
- Uma linha `---` dentro de um valor de bloco do YAML encerra o frontmatter ali.
- Frontmatter que existe mas não é YAML válido gera erro de YAML (seção 11), com linha e coluna.
- O corpo mantém suas quebras de linha originais, byte a byte.
- Comentários do frontmatter não sobrevivem a uma reemissão.

### 3.2 Leitura do YAML

O frontmatter vira um valor JSON (objeto, lista, texto, número, booleano ou nulo). As regras abaixo
são as de uma leitura YAML "estilo 1.2 com compatibilidade". Outra biblioteca YAML lê vários valores
de outro jeito, e a regra de idempotência da escrita (§5.2) depende de o valor lido ser exatamente
este.

**Escalares sem aspas e sem tag:**

| Texto | Valor |
|---|---|
| vazio, `~`, `null`, `Null`, `NULL` | nulo |
| `true`, `True`, `TRUE` / `false`, `False`, `FALSE` | booleano |
| `yes`, `no`, `on`, `off`, `y`, `n` e outras grafias | **texto** |
| inteiro decimal com sinal opcional (`42`, `+42`, `-7`) | número inteiro |
| `0x…`, `0o…`, `0b…` (prefixo em minúsculas, sinal opcional) | inteiro em base 16, 8 ou 2 |
| `0X1F`, `0O17` (prefixo em maiúsculas) | **texto** |
| zero à esquerda seguido só de dígitos (`007`, `+08`, `-01`) | **texto** |
| separador de dígitos (`1_000`) | **texto** |
| inteiro que não cabe em 64 bits (com sinal ou sem sinal) | **erro** (página recusada) |
| número decimal ou com expoente (`0.92`, `1e3`) | número de ponto flutuante |
| decimal que estoura (`1e999`) | **texto** |
| `.inf`, `.Inf`, `.INF`, `-.inf` e variações, `.nan`, `.NaN`, `.NAN` | nulo (JSON não tem infinito nem NaN) |
| `+.nan` | texto |
| datas e instantes (`2026-10-12`, `2026-10-12T01:08:40Z`) | **texto**, sem conversão |
| `1:20` (sexagesimal) | texto |

**Com aspas ou estilo de bloco:** sempre texto (`'123'` vira o texto `123`).

**Tags:**

- `!!str`, `!!timestamp`, `!!binary` e outras tags do núcleo que não são as quatro abaixo: o valor
  fica como texto, sem conversão;
- `!!bool`, `!!int`, `!!float`, `!!null`: o texto precisa ser válido para o tipo (pelas regras
  acima), senão é erro;
- tag local (`!foo valor`): vira um objeto de uma chave, `{"!foo": valor}`. O conteúdo é lido sem a
  tag (`!foo [1, 007]` vira `{"!foo": [1, "007"]}`).

**Estrutura:**

- Chave repetida num mapeamento é **erro**, com linha e coluna. A comparação é pelo valor YAML da
  chave (`1` e `"1"` são chaves diferentes para essa checagem, embora ambas virem o texto `1`).
- Chaves que são texto, booleano ou número viram texto (`true`, `1`). Chave de outro tipo é erro.
- Âncoras e aliases são expandidos. `<<` é uma chave comum (não há mescla de mapeamentos).
- Mais de um documento YAML no bloco é erro.
- Profundidade máxima de 128 contêineres aninhados, contando o mapeamento de topo. Um alias conta a
  profundidade em que ele é expandido.
- No máximo 250 000 nós no total, contando as expansões de alias (proteção contra "bomba" de aliases).
  Acima disso é erro, recusado rápido.
- Um escalar de bloco (`|` ou `>`) que é o último valor do frontmatter perde a quebra de linha final,
  porque o bloco termina sem quebra antes do `---`: `b: |\n  last` lido no fim vale `"last"`, enquanto
  o mesmo valor no meio do bloco vale `"last\n"`.
- A ordem das chaves é preservada como no arquivo.

Se o frontmatter lido não é um mapeamento (uma lista ou um texto solto, por exemplo), as camadas de
escrita o trocam por um mapeamento vazio antes da conformidade OKF.

### 3.3 Emissão

- Frontmatter ausente, nulo ou mapeamento vazio: o documento sai **sem** bloco, só com o corpo.
- Caso contrário: `---\n`, o YAML, `\n` se o YAML não terminou em quebra, `---\n`, e o corpo
  exatamente como está. O bloco usa sempre LF, mesmo que o corpo use CRLF.
- Regras do YAML emitido:
  - chaves na **ordem de inserção** (a ordem lida do arquivo, com chaves novas no fim);
  - escalares simples, sem aspas, sempre que a leitura de §3.2 devolveria o mesmo texto. Isso inclui
    datas, instantes, texto não ASCII e texto longo, que ficam numa linha só (nunca dobrados);
  - **aspas duplas** quando o texto, sem aspas, seria lido como outro tipo ou outra estrutura: `""`,
    `"true"`, `"null"`, `"123"`, texto com `: ` ou ` #`, texto que começa com `#` ou `@`. Isso vale para
    chaves e valores;
  - texto com quebra de linha sai como bloco literal `|`, com indentação de 2 espaços;
  - listas saem com `- ` na mesma coluna da chave dona; um item que é mapeamento continua nas linhas
    seguintes com 2 espaços (`- kind: observation\n  id: …`);
  - mapeamento vazio sai como `{}` e lista vazia como `[]`.

Casos dourados (texto do arquivo → leitura → emissão, igual byte a byte):

```text
---
kind: decision
confidence: 0.92
pinned: false
created_at: 2026-10-12T01:08:40Z
status: yes
tags:
- backend
- 日本語
summary: |
  First line.
  Second line.
evidence:
- kind: observation
  id: 019e5633-4140-7dd0-9e2f-118825ca6355
  at: 2026-07-18T09:35:52Z
- kind: page
  path: decisions/0001.md
relations: {}
aliases: []
---
# Body
```

```text
frontmatter {"key: colon": "a # b", "#hash": "@at", "123": "true", "": "null", "plain": "2026-10-12"}
emite:
---
"key: colon": "a # b"
"#hash": "@at"
"123": "true"
"": "null"
plain: 2026-10-12
---
```

```text
frontmatter {"lit": "one\ntwo\n", "fold": "folded text\n", "strip": "keep"}
emite:
---
lit: |
  one
  two
fold: |
  folded text
strip: keep
---
```

### 3.4 Título

Ordem de prioridade:

1. campo `title` do frontmatter, se for texto e não vazio após remover espaços das pontas
   (o valor usado é o aparado);
2. primeira linha do corpo que começa com `# ` e tem texto depois (também após remover espaços),
   **fora de blocos de código cercados** (regra de cerca em §3.5). Uma linha `# ` dentro de um bloco
   cercado (comentário de shell ou de Python) não é título. A linha de título precisa começar na
   coluna 1, sem espaço antes do `#`. As linhas são separadas por LF, e um CR final é descartado;
3. nome do arquivo sem o sufixo `.md` (o último segmento do caminho; `.md` é sensível à caixa).

Um título explícito passado por quem chama vence os três e passa pelo higienizador. O título de
página **não tem teto de tamanho**: o limite de 80 caracteres é só de observação. O título derivado
não passa pelo higienizador de novo, mas é tirado de corpo e frontmatter já higienizados.

### 3.5 Links do corpo

O corpo é lido linha a linha (separadas por LF; CR final descartado).

**Cerca de código** (CommonMark §4.5, simplificada):

- uma linha abre ou fecha cerca se tem no máximo 3 espaços no início e depois uma sequência de pelo
  menos três crases ou três tis;
- uma abertura com crases cujo resto da linha (o "info") contém outra crase **não** é cerca (é uma
  linha com código inline);
- dentro de uma cerca, só fecha uma linha com o mesmo caractere, sequência de tamanho maior ou igual
  à da abertura e nada além de espaço depois;
- linhas de cerca e linhas dentro da cerca são ignoradas inteiras. Uma cerca nunca fechada vai até o
  fim do corpo. Blocos de código indentados (4 espaços) **não** são reconhecidos.

**Código inline:** numa linha fora de cerca, cada trecho de código inline é trocado por espaços
(o mesmo número de caracteres) antes da extração. Uma sequência de N crases abre um trecho que só
fecha com outra sequência de exatamente N crases na mesma linha. Sem fechamento na linha, as crases
são texto comum. Trocar por espaços, em vez de cortar, mantém intacto um link cujo rótulo é código
(`` [`foo`](foo.md) ``).

**Wikilinks:** cada `[[…]]` da linha, da esquerda para a direita. Um `!` antes do `[[` não muda
nada: `![[x]]` também é extraído como link. O fechamento é o primeiro `]]`
depois do `[[`. Um `[[` sem `]]` encerra a busca na linha.

1. Se há `|`, o texto antes do primeiro `|` é o alvo (o resto é rótulo). O alvo é aparado.
2. **Escopo:** se o alvo não contém `://` e não começa (sem distinguir caixa) com `mailto:`, `data:`,
   `javascript:`, `vbscript:`, `tel:` ou `file:`, e tem `:`, o trecho antes do **primeiro** `:`
   (aparado) é o escopo quando não é vazio e só tem letras ou dígitos Unicode, `-`, `_`, `/` e `.`.
   Com `/`, o que vem antes do primeiro `/` é o workspace e o resto é o projeto (ambos aparados). Sem
   `/`, é só o projeto. Projeto vazio desfaz o escopo (o alvo inteiro vira caminho). Projeto `_global`
   sem workspace recebe o workspace `default`. O caminho é o resto depois do `:`, aparado.
3. O caminho passa pela normalização abaixo, como **relativo à raiz do projeto**.

**Links markdown** `[rótulo](destino)`: a linha é varrida uma vez, da esquerda para a direita,
mantendo uma pilha de `[` abertos.

- `\` escapa o caractere seguinte (um `\!` não marca imagem; um `\[` não abre rótulo).
- Um `[` precedido de `!` não escapado abre o rótulo de uma **imagem**.
- Um `]` fecha o último rótulo aberto. Se vem logo seguido de `(`, o destino é lido. Destino de link
  comum (não imagem) vai para a normalização como **relativo ao diretório da página**, ou à raiz do
  projeto se começa com `/`. Imagens são ignoradas. Depois de um link comum, os rótulos abertos são
  descartados (imagem pode estar dentro do rótulo de um link; link não pode estar dentro de outro).
- Leitura do destino, a partir do `(` e limitada aos primeiros **512 bytes** (cortando numa fronteira
  de caractere):
  - espaços no início são pulados;
  - `<…>`: o destino é o que está entre `<` e o primeiro `>`, e depois dele precisa vir o fechamento
    descrito abaixo;
  - senão, o destino vai até o `)` que fecha (parênteses internos balanceados são permitidos). Um
    destino sem `<>` **pode** conter espaços quando não há título depois (`[x](my page.md)`). Em cada
    início de sequência de espaço (no máximo 8 tentativas), testa-se se o resto é um título opcional
    seguido de `)`. Se for, o destino termina ali;
  - fechamento: espaços opcionais, então `)`; ou, depois de pelo menos um espaço, um título entre
    `"…"`, `'…'` ou `(…)` (com `\` escapando o delimitador; um `(` não escapado dentro de título em
    parênteses invalida), espaços opcionais e `)`. O título é descartado;
  - sem fechamento válido, não há link.
- O destino de link markdown passa por **decodificação percentual, uma vez**, depois de cortar
  consulta e âncora: cada `%XX` com dois dígitos hexadecimais vira o byte; `%` sem dois hexadecimais
  fica literal; um byte de controle ASCII decodificado, ou bytes que não formam UTF-8 válido, descartam
  o link. Wikilinks **não** são decodificados.

**Normalização do alvo** (comum aos dois tipos):

1. corta o que vem depois do primeiro `|`; apara; remove `<` e `>` das pontas;
2. vazio ou começando com `#` (âncora pura): descartado;
3. corta no primeiro `#` ou `?` e apara;
4. (link markdown) decodificação percentual, como acima;
5. descartado se vazio, se contém `://` ou `\`, ou se começa (sem distinguir caixa) com `mailto:`,
   `data:`, `javascript:`, `vbscript:`, `tel:` ou `file:`;
6. descartado se o último segmento é vazio (termina em `/`) ou é exatamente `.md`;
7. se o último segmento tem `.`, precisa terminar em `.md` exatamente (`.MD` e `.markdown` são
   descartados). Se não tem `.`, recebe `.md`;
8. resolução: parte de uma lista vazia (raiz do projeto) ou dos segmentos do diretório da página;
   segmentos vazios e `.` são ignorados, `..` remove o último segmento e, **se não houver segmento a
   remover, descarta o link**; os demais são acrescentados. Resultado vazio é descartado;
9. o resultado precisa passar na validação de caminho (§4.2 do vocabulário). `ensure_portable` não é
   aplicado aqui.

Os links do corpo são deduplicados pela tripla (workspace, projeto, caminho) e ordenados por ela,
com ausente antes de presente e texto em ordem de bytes.

### 3.6 Relações tipadas (frontmatter)

- Lidas da chave `relations`. Se não é mapeamento, nada é lido (sem aviso).
- Para cada chave, na ordem do arquivo:
  - nome fora do vocabulário fechado (`causes`, `fixes`, `contradicts`, exatos e em minúsculas): a chave
    é ignorada inteira, com aviso no log. Um erro de digitação não cria um tipo de aresta novo;
  - valor que não é lista: ignorado sem aviso. Itens da lista que não são texto: ignorados sem aviso.
- Cada alvo de texto usa a mesma regra de escopo dos wikilinks (§3.5, passo 2). O caminho é aparado e:
  - não nomeia página (termina em `/`, último segmento `.md`, ou último segmento com `.` que não
    termina em `.md`): ignorado com aviso;
  - último segmento sem `.`: recebe `.md`;
  - falha na validação de caminho: ignorado com aviso.
- Diferente dos links do corpo, aqui **não** há corte de `#` ou `?`, nem resolução de `.` e `..`, nem
  decodificação percentual: o caminho é usado como escrito.
- Valores de texto vindos do frontmatter e copiados para o log são cortados em 200 bytes (com
  reticências dentro do limite), por serem entrada não confiável.

### 3.7 Conjunto de links da página

Todo caminho de escrita grava o **conjunto completo** de links de saída: os links do corpo (§3.5)
seguidos das relações do frontmatter (§3.6), por uma única função de extração, para que um novo ponto
de escrita não esqueça a parte tipada. O armazenamento descarta repetições pela chave
(workspace, projeto, caminho, relação), mantendo a primeira. Um mesmo alvo pode aparecer uma vez como
`references` (do corpo) e uma vez com a relação tipada.

---

## 4. Escrita atômica

Gravar um arquivo substituindo o anterior segue estas etapas:

0. cria os diretórios que faltam até o destino;
1. cria um arquivo temporário **no mesmo diretório** do destino, com prefixo
   `.ai-memory-tmp.` seguido de um sufixo aleatório. No Unix, o temporário nasce com permissão
   **0600** (só o dono). Como a troca é por renomeação, o arquivo final fica com essa mesma
   permissão, inclusive quando substitui um arquivo que tinha outra;
   O sufixo aleatório tem 6 caracteres alfanuméricos. No Windows não há bits de permissão: o arquivo
   herda as permissões (ACL) do diretório. O temporário nasce com o atributo `TEMPORARY`, que é trocado
   para `NORMAL` antes da troca.
2. escreve o conteúdo e sincroniza os dados do temporário com o disco;
3. troca o temporário pelo destino (renomeação sobre o arquivo existente; no Windows, renomeação
   com substituição do destino existente) e sincroniza de novo os dados do arquivo já renomeado;
4. sincroniza o diretório pai, em modo de melhor esforço: se o diretório não abre ou a sincronização
   falha, o erro é ignorado em silêncio e a escrita conta como feita. **No Windows este passo não faz
   nada na prática:** abrir um diretório como arquivo comum falha, e a falha é ignorada.

Falha em qualquer etapa de 0 a 3 é erro da escrita. Se a falha ocorre antes da troca, o temporário é
apagado.

Detalhes:

- O observador (seção 8) ignora arquivos com o prefixo do temporário.
- **No Windows**, a troca pode falhar com violação de compartilhamento ou de bloqueio
  (códigos de sistema 32 e 33), geralmente causada por antivírus ou indexador que
  está lendo o arquivo recém-criado. Nesses casos a troca é repetida até 5 vezes, com
  espera linear de 10, 20, 30, 40 e 50 ms (no máximo cerca de 150 ms). Se o problema
  persistir, o erro original é propagado. **Acesso negado (código 5) não é repetido**: costuma ser
  problema real de permissão e propaga na primeira falha. A repetição reaproveita o mesmo temporário
  (os bytes já gravados e sincronizados continuam lá), sem reescrevê-lo. A espera bloqueia a thread que
  escreve. O mesmo primitivo (com a mesma repetição) é usado por toda gravação atômica da wiki, inclusive
  as do adaptador Git (conferido na referência 3.1.1).
- **Em outras plataformas** nenhum erro é considerado transitório (no Unix o código 32 é outro erro, de
  pipe quebrado, e não deve ser repetido).
- A sincronização depois da troca é só dos **dados** do arquivo (não dos metadados); a do diretório pai
  é uma sincronização completa, em melhor esforço.
- A gravação devolve um identificador do arquivo gravado (número de inode no Unix;
  índice de arquivo NTFS no Windows). Se o identificador não pode ser lido, ou o volume não o
  expõe (FAT), o valor é zero, não erro. **Nenhum componente usa esse identificador hoje:** o
  observador não distingue as próprias escritas. Ele ignora os temporários pelo prefixo e absorve a
  escrita já instalada pela regra de conteúdo idêntico (seção 13, item 3).

---

## 5. Escrita de página: fluxo passo a passo

### 5.1 Estado do componente

O componente Wiki é compartilhável entre tarefas e mantém:

- raiz do wiki, escritor do banco, adaptador Git;
- embedder opcional (ausente = nenhum embedding é gerado);
- higienizador de texto, cadeia de admissão opcional e leitor do banco opcional;
- **guarda de mutação** do tipo leitor/escritor: escrita de página usa a guarda
  compartilhada; remoções condicionais (por versão esperada, por decaimento, lápide da
  reconciliação e remoção definitiva de lápide), purgas, movimentações e manutenção de escopo
  usam a guarda exclusiva. A remoção simples de página usa a guarda **compartilhada** (seção 5.4);
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
3. **Fixação e autoria.** Um caminho que começa com `_slots/` é sempre fixado. Se o ator tem
   nome de usuário não vazio, o frontmatter recebe a chave `last_modified_by`, um mapeamento com
   `username` e, quando conhecidos, `name` e `email`, nessa ordem. A chave é substituída se já
   existia (mantendo a posição) ou acrescentada no fim. Frontmatter nulo vira mapeamento antes
   disso; frontmatter que é outro tipo (lista, texto) fica como está e não recebe a chave. Ator
   anônimo não gera bloco.
4. **Cadeia de admissão** (se configurada). Os nomes humanos de workspace e projeto
   são resolvidos para o contexto e os webhooks **bloqueantes** são executados em
   ordem. Eles podem alterar frontmatter e corpo, ou rejeitar a escrita.
5. **Nova higienização e normalização.** O corpo e **todo valor de texto** do frontmatter, em
   qualquer profundidade (mapeamentos e listas), são higienizados de novo, porque webhooks são
   entrada externa. Chaves não são higienizadas. Depois:
   - **fixação final** = pedido de fixação **ou** caminho de slot **ou** `pinned: true` no
     frontmatter (só o booleano conta: o texto `"true"` não fixa);
   - **forma canônica:** a chave `tier` recebe o nível passado por quem chama (que **vence** o valor
     do frontmatter; a chave mantém a posição se existia, senão vai para o fim). Se a página é fixada,
     `pinned: true` é gravado. Um `pinned: false` presente fica como está. Não há herança de fixação
     da versão anterior: o frontmatter enviado é a verdade;
   - **conformidade OKF** ([okf.md](okf.md)): preenche as chaves deterministas que faltam;
   - **`generated.at`:** se o arquivo atual no disco existe, é legível e tem o mesmo corpo e o mesmo
     frontmatter (comparados sem `generated.at`), o `generated.at` dele é herdado. Senão, recebe o
     instante atual em UTC, `AAAA-MM-DDTHH:MM:SSZ`. É isso que faz uma reescrita idêntica gerar os
     mesmos bytes.
6. **Derivações.** Título (explícito, ou pela regra da seção 3.4), links (seção 3.7),
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
   compartilhada de mutação e a trava da página, nessa ordem. Dentro dessas guardas o
   confinamento é verificado de novo, e confere-se que o projeto **existe** no índice e pertence ao
   workspace. A escrita **não cria** linhas de escopo: projeto inexistente é erro de não encontrado
   ("o projeto não existe"), e projeto de outro workspace também ("não pertence ao workspace", com o
   workspace dono). Quem cria escopo é a resolução de escrita
   ([storage-and-retrieval.md](storage-and-retrieval.md) §14.1). Depois são garantidos os
   manifestos `_meta.md` (seção 1.1). Os manifestos são garantidos uma vez por escopo durante a
   vida do processo; uma falha ao escrevê-los só gera aviso e não interrompe a escrita.
   Numa escrita condicional (só se a versão atual for a esperada, ou só se o caminho não existir),
   a condição é conferida aqui, antes de tocar no disco, e de novo dentro da transação do índice.
   Se ela não vale, nada é escrito e quem chama recebe a versão atual.
8. **Instalação do arquivo.** O arquivo é gravado guardando o conteúdo anterior, para
   rollback. Isso acontece **antes** da gravação no índice: o markdown nunca fica atrás
   do banco.
9. **Gravação no índice.** Cria a nova versão da página e marca a anterior como
   superada. Se a gravação falhar, o arquivo anterior é restaurado. Se a restauração
   também falhar, o sistema retorna um erro explícito de **estado inconsistente**
   (arquivos mudaram, o índice não, e a reversão falhou), em vez de mascarar o problema.
   Texto literal do erro (referência 3.1.1): `INCONSISTENT STATE: wiki files changed but store
   write failed (<causa>) and rollback failed (<erro da reversão>)`. Regras da reversão: o conteúdo
   anterior é lido **antes** de instalar (arquivo ausente = "não existia"; qualquer outro erro de
   leitura aborta a escrita antes de tocar o disco); a reversão percorre os arquivos instalados na
   **ordem inversa**; arquivo que existia é regravado com os bytes antigos pela escrita atômica;
   arquivo que não existia é apagado (já ausente não é erro), com sincronização do diretório em melhor
   esforço. Quando a reversão dá certo, quem chama recebe o erro original do banco.
10. **Embedding.** Se houver embedder, o corpo é embutido **na mesma execução** que a
    escrita, sem indexação em segundo plano. Se o resumo existir, ele recebe um embedding
    próprio. Falha de embedding gera aviso e registro da falha no índice, mas **não
    desfaz a escrita**. Se a página reescrita não tem mais resumo, o embedding de resumo
    anterior é removido.
11. **Webhooks não bloqueantes.** São disparados depois que a página foi persistida e
    indexada, e não podem alterar nem rejeitar a escrita.

Propriedades observáveis:

- Reescrever conteúdo idêntico não cria uma nova versão e não muda os bytes do arquivo. O arquivo
  ainda é regravado (mesmo conteúdo, nova data de modificação), o que gera um evento para o
  observador, absorvido pela mesma regra de conteúdo idêntico. A evidência da escrita é registrada
  contra a versão atual.
- A escrita **não faz commit** no Git. Os caminhos tocados ficam registrados para o próximo commit
  automático (seção 7).
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

- **Remover página:** remove a página. A cadeia de admissão é consultada **antes** de qualquer
  alteração, e um webhook bloqueante que rejeite interrompe a operação.
- **Remover página se a versão for a esperada:** só remove se a versão atual ainda for a esperada
  (verificação de versão, para evitar apagar algo que foi reescrito no meio do caminho).
- **Evicção por decaimento:** segue a mesma verificação de versão. Move o arquivo para uma área
  de quarentena, apaga a versão no índice e cria um marcador de decaimento (tombstone). Se a remoção
  no índice falhar, o arquivo é restaurado da quarentena. Em caso de sucesso, a cópia em quarentena
  é apagada.
  A quarentena é um arquivo **no mesmo diretório** da página, com prefixo `.ai-memory-delete.` e
  sufixo aleatório (sem extensão `.md`, por isso o observador o ignora): o nome é reservado criando
  um temporário, que é apagado e então recebe a página por renomeação. Se a renomeação falha, o nome
  reservado é apagado e o erro propaga, sem tocar no índice.

**Fluxo comum das três remoções acima** (conferido na referência 3.1.1; corrige a impressão de que só
a remoção por decaimento usa quarentena):

1. confinamento do caminho (sem criar diretórios); a remoção simples adquire a guarda de mutação
   **compartilhada**, e as duas condicionais (por versão esperada e por decaimento) adquirem a guarda
   **exclusiva**, que mantém escritas normais fora entre a comparação e a quarentena;
2. confere que o projeto existe e pertence ao workspace (nunca cria escopo);
3. nas condicionais, compara a versão corrente do índice com a esperada; divergente devolve "não
   removeu" sem tocar disco nem índice (a comparação é repetida dentro da transação do banco);
4. cadeia de admissão (bloqueante), operação `delete`: uma rejeição interrompe antes de qualquer
   alteração;
5. quarentena do arquivo (descrita acima). Arquivo já ausente não é erro: segue sem quarentena;
6. remoção no índice (física para remoção simples ou condicional, lápide para decaimento). Se falha,
   ou se a condição deixou de valer dentro da transação, o arquivo volta da quarentena para o caminho
   original e o resultado é o erro, ou "não removeu";
7. sucesso: a cópia em quarentena é apagada e os webhooks não bloqueantes são disparados.

A lápide criada pela reconciliação quando o arquivo sumiu (opção desligada por padrão, seção 8) não
usa esse fluxo: não roda a cadeia bloqueante nem mexe no disco; confere de novo, sob a guarda
exclusiva, a versão esperada **e** que o arquivo continua ausente (se voltou, nada é feito) e, no
sucesso, só dispara os webhooks não bloqueantes da operação `delete`.

- **Remoção definitiva de lápide de decaimento:** apaga definitivamente um marcador de decaimento
  antigo e sua cadeia de versões. Nunca apaga um arquivo que existe. Se um arquivo recriado no
  mesmo caminho ainda não foi indexado, ele é reindexado antes.

**Escopo:**

- **Purgar projeto, sessão ou workspace:** removem dados de um escopo. Antes de tocar em qualquer
  coisa, consultam a cadeia de admissão (operações `purge_project`, `purge_session`,
  `purge_workspace` da seção 6.1).
- **Mover projeto para outro workspace:** move **sem mudar o identificador do projeto**. Usa a
  guarda exclusiva, renomeia o diretório e atualiza o índice.
- **Mover página de sessão:** move a página de uma sessão para outro projeto.
- **Remover diretório de projeto ou de workspace:** removem os diretórios de escopo.
- Propostas de automelhoria: escrevem um arquivo lateral de revisão que **não é indexado**. A
  aprovação aplica a proposta pelo mesmo caminho de escrita da seção 5.2 e trata conflito
  revertendo o arquivo instalado.

**Restauração e reindexação:**

- **Restaurar página de um checkpoint:** lê a página na revisão indicada pelo histórico Git e a
  regrava pelo caminho normal.
- **Reindexar uma página:** reindexa uma página a partir do disco (seção 5.5).
- **Reconstruir o índice inteiro** a partir do disco:
  1. valida a árvore inteira (seção 12);
  2. percorre os diretórios `<workspace_id>/<project_id>/` com dois UUIDs válidos;
  3. pula escopos purgados (tombstone de escopo): não os ressuscita;
  4. na primeira vez que vê um workspace, lê o `_meta.md` dele e garante a linha do workspace
     **com o identificador do diretório**; depois lê o `_meta.md` do projeto e garante a linha do
     projeto com o identificador do diretório, o nome, o caminho do checkout e a identidade
     (seção 1.1). Manifesto ausente ou inválido é erro que interrompe a reconstrução;
  5. percorre as páginas do projeto (mesmas exclusões da varredura da seção 8.2), pula páginas de
     sessões purgadas (`sessions/<id>.md` com a sessão na lista de purgadas) e reindexa cada uma;
  6. uma página que colide com outra já indexada (só difere por caixa ou Unicode) é pulada e contada
     em `skipped_collisions`. **Qualquer outro erro de página interrompe a reconstrução inteira.**

  O resumo da reconstrução informa workspaces, projetos, páginas, `skipped_purged`,
  `skipped_purged_sessions` e `skipped_collisions`.
- **Preencher manifestos faltantes:** cria o `_meta.md` de escopos que não o têm.
- **Reparar validade só com data:** converte validades só com data para o instante de fim do dia,
  no formato OKF.

**Ledger:** o acréscimo ao ledger grava bytes no fim de um arquivo `log*.md` do projeto e
reporta a alteração ao Git (seção 9).

**Admissão:** a **consulta prévia** à cadeia roda **antes** de trabalho caro (por exemplo, antes de
chamar um modelo de linguagem), sem gravar nada. A autorização de operação, o aviso aos
observadores de operação e o disparo de purga seguem a regra da seção 6.

### 5.5 Reindexação de uma página a partir do disco

Usada pelo observador (seção 8) e pela reconstrução do índice inteiro. A restauração de checkpoint não
usa este fluxo: ela regrava a página pelo fluxo da seção 5.2. Difere da escrita da seção 5.2 em pontos
que mudam o resultado:

1. Recusa caminhos em `_pending/` (propostas não são indexadas).
2. Sob a guarda compartilhada e a trava da página, lê o arquivo do disco como texto UTF-8 e interpreta
   o frontmatter (seção 3). Arquivo que não é UTF-8 válido, ou frontmatter inválido, é erro.
3. **Não** passa pelo higienizador, **não** roda webhooks e **não** reescreve o arquivo: o markdown do
   disco é a fonte da verdade, e o observador nunca escreve no disco.
4. Derivações a partir do próprio arquivo:
   - **nível:** chave `tier` do frontmatter; ausente vale `semantic`. Um texto que não é nível válido,
     ou um valor que não é texto, é erro;
   - **fixação:** caminho de slot ou `pinned: true` (booleano);
   - título, links, validade e entidades pelas mesmas regras da seção 5.2 (validade e `entities`
     inválidos são erro);
   - sem autor e sem evidência.
5. Grava no índice pelo mesmo upsert ([storage-and-retrieval.md](storage-and-retrieval.md) §5.1). A
   conformidade OKF do upsert vale só para a linha do índice, não para o arquivo.
6. Com embedder configurado, só gera embedding se o upsert criou uma versão nova. Uma reindexação que
   não muda nada (a reconciliação passa por todas as páginas a cada 30 s) não gera embedding.

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
  - o último registro não é confiável (o adaptador marcou que precisa varrer);
  - nenhum caminho foi registrado desde o último commit;
  - passaram 600 s desde a última varredura.

  A varredura compara a árvore com os caminhos registrados e conta as escritas que não
  foram reportadas (contador interno, só usado em diagnóstico e não exposto em nenhuma API), guardando uma amostra de até 5 caminhos.
  Escrita não reportada significa que alguém contornou o wiki.
- Se o HEAD mudou por fora do adaptador, o índice é relido e a próxima passagem varre
  a árvore.
- **Leitura em corrida.** Se um arquivo muda enquanto é lido para o commit (um escritor
  fora da trava), o commit tenta de novo: até 4 tentativas, com 25 ms de espera entre
  elas. Se ainda falhar, os caminhos continuam registrados para o próximo commit.
- **O que um commit faz** (caminho normal):
  1. inicializa o repositório na raiz da wiki na primeira abertura, se `.git` não existe;
  2. prepara o índice: só os caminhos registrados, ou a árvore inteira na varredura completa. Na
     varredura completa e num diretório registrado, arquivos ignorados pelo `.gitignore` do repositório
     ficam de fora, como no `git add` padrão. Um **arquivo** registrado individualmente entra mesmo
     que o `.gitignore` o ignore. Um caminho registrado que não existe mais sai do índice. Caminhos com
     um componente reservado do Git (`.git` em qualquer caixa, `git~1` a `git~4`) nunca entram;
  3. **ledgers nunca entram no índice** (seção 9, por nome **e** conteúdo, em qualquer profundidade).
     Um ledger que uma versão antiga commitou sai do índice; o arquivo fica no disco e as versões antigas
     continuam no histórico. Não se usa `.gitignore` para isso, porque o nome sozinho também esconderia
     uma página comum com nome de ledger;
  4. grava o arquivo de índice do Git em toda tentativa, mesmo sem commit, para que outras ferramentas
     não vejam páginas como "preparadas e não preparadas" ao mesmo tempo;
  5. se a árvore resultante é igual à do `HEAD`, **não há commit**. Num repositório sem `HEAD`, um
     índice vazio também não gera commit (nada de commit inicial vazio);
  6. o commit tem autor e committer `ai-memory <ai-memory@local>`, data atual, um único pai (o `HEAD`,
     quando existe) e atualiza o `HEAD`. No caminho normal (biblioteca embutida), o commit não é
     assinado e não executa hooks do Git. No fallback do Windows, por usar o executável, hooks e
     assinatura que o usuário configurou no repositório podem rodar;
  7. se a gravação da árvore falha porque o índice aponta para um objeto que não existe mais, o índice é
     esvaziado, a árvore inteira é preparada de novo e a gravação é repetida uma vez.
- O commit de tudo devolve nenhum identificador de commit quando não há mudança.
- Mensagens fixas usadas pelo próprio wiki: `okf-migration: conform wiki to OKF v0.2` (migração) e
  `okf: repair date-only stale_after on existing pages` (reparo). As demais mensagens vêm de quem chama
  o commit.
- **Fallback (só no Windows).** No Windows, se a biblioteca Git embutida falha ao abrir o
  repositório para commit porque ele não foi encontrado, ou porque um caminho não pôde ser
  resolvido, o adaptador tenta o executável `git` do sistema, depois das mesmas validações de
  confinamento. A inicialização do repositório tem o mesmo fallback (`git init -q`). Erro de dono do
  repositório (*owner*) **nunca** aciona o fallback. **Fora do Windows não há fallback:** o erro
  original é devolvido.
- **Como o executável é chamado no fallback:** com `-C <raiz da wiki>`, sem as variáveis de ambiente
  `GIT_DIR`, `GIT_WORK_TREE`, `GIT_COMMON_DIR`, `GIT_INDEX_FILE`, `GIT_OBJECT_DIRECTORY` e
  `GIT_ALTERNATE_OBJECT_DIRECTORIES`. O commit usa `-c user.name=ai-memory -c user.email=ai-memory@local
  commit -q -m <mensagem>`. Antes, os ledgers rastreados saem do índice (`rm --cached --force`) e o
  resto entra com `add -A` usando os ledgers como exclusões. As listas de caminhos vão pela entrada
  padrão, porque uma lista longa não cabe na linha de comando do Windows. Se `diff --cached --quiet`
  diz que não há mudança, nenhum commit é feito.
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
  no índice (ver 8.3). Temporários e caminhos internos do Git não são registrados.
- Evento de criação, modificação ou genérico **sobre um diretório** dentro de um projeto
  (qualquer um desses tipos, não só criação): reindexa o projeto inteiro, mas só se o projeto já
  tem linha no índice (um diretório sem escopo conhecido é ignorado, com mensagem de depuração).
  No Windows, editar um arquivo numa subpasta costuma gerar também um evento de modificação da
  pasta pai, e isso dispara a reindexação do projeto; ela é barata porque conteúdo idêntico não cria
  versão.
- Todo arquivo comum que gera evento (exceto temporários e internos do Git) é registrado no Git
  **antes** dos filtros abaixo. Por isso um ledger ou um arquivo que não é markdown também chega ao
  próximo commit (onde o ledger é então excluído pela regra da seção 7).
- **Ignorados para indexação**, com o alcance de cada regra:
  - caminhos com componente reservado do Git (`.git`, `git~1` a `git~4`), em qualquer nível;
  - links simbólicos e pontos de reparse (seção 12);
  - arquivos cujo nome começa com `.ai-memory-tmp.`, em qualquer nível;
  - arquivos cuja extensão não é exatamente `md` (sensível à caixa: `.MD` não é markdown);
  - eventos fora de `<workspace_id>/<project_id>/…`, ou com caminho de página inválido;
  - `_pending` e tudo sob `_pending/`, **só na raiz do projeto**;
  - `_meta.md` **em qualquer nível**;
  - `bootstrap.md` **só na raiz do projeto**;
  - `log.md` e `log-AAAA-MM.md` **só na raiz do projeto**, e só quando o conteúdo é de ledger
    (seção 9). Com esse nome mas conteúdo de página comum, o arquivo é indexado. Em subdiretórios,
    esses nomes são sempre páginas comuns.
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
- Ignora links simbólicos e pontos de reparse (sem entrar neles), arquivos temporários,
  `_pending/` na raiz do projeto, `_meta.md`, `bootstrap.md` e ledgers, com o mesmo alcance da
  seção 8.1. Um diretório que some durante a varredura é pulado (e marca a varredura como parcial
  para a seção 8.3).
- Um diretório de projeto sem linha de projeto no índice (órfão) é pulado inteiro, sem reindexar
  nenhuma página e sem escrever `_meta.md`, e contado em `skipped_orphans`. A reconciliação, como a
  escrita, não cria escopos.
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
- **Detalhes das regras de contagem.** (1) Candidatos = versões atuais do índice tiradas **antes** da varredura da árvore
  (uma página gravada durante a varredura nunca parece removida), sem pendências, `_meta.md`, `bootstrap.md`, nomes no formato de ledger
  (`log.md`, `log-AAAA-MM.md`, por nome, sem ler conteúdo) e `sessions/<id>.md`. (2) Uma página encontrada no disco apaga a sequência
  dela, mesmo numa varredura parcial. (3) Varredura parcial: nada novo conta e as sequências existentes ficam como estão. (4) Disjuntor:
  se ausentes `>` `max(3, ceil(total/2))`, **ou** se a varredura não achou nenhuma página (mesmo num escopo pequeno), o escopo é ignorado e a passagem
  **não conta** (nem incrementa sequências). (5) A sequência guarda o id da versão ausente; se o id mudou, recomeça em 0. (6) Ao chegar a 2, a sequência
  é descartada e a marcação é tentada; se ainda ausente depois, começa nova sequência. (7) A marcação grava `is_latest = 0`, `superseded_at` e `valid_to` no
  mesmo instante, fecha os vínculos de entidade abertos e registra auditoria `soft_delete_for_reconcile`; se o arquivo reapareceu ou a versão mudou na conferência final, nada é feito.
  (8) Uma marcação tem o mesmo prazo de remoção definitiva que o decaimento (`hard_delete_after_days`); se o arquivo voltar, a nova versão religa a cadeia e limpa `superseded_at`.
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

**Quem escreve e como** (conferido na referência 3.1.1):

- O servidor só **escreve** no arquivo mensal `log-AAAA-MM.md`, na raiz do projeto, com ano e mês do
  instante do evento em UTC (um evento perto da virada do mês cai no arquivo do mês dele). `log.md`
  só é **reconhecido** (instalações antigas, migração OKF); nada novo é acrescentado a ele.
- Uma linha por evento, terminada em LF: `## [AAAA-MM-DDTHH:MM:SSZ] <evento> | <título>`, com o
  instante em UTC e precisão de segundos. `<evento>` é um de `session-start`, `user-prompt`,
  `pre-tool-use`, `post-tool-use`, `pre-compact`, `post-compaction`, `notification`, `stop`,
  `session-end`, `subagent-start`, `subagent-stop`, `other`. No título, cada `\n` e cada `\r` vira
  espaço e só os primeiros **120** caracteres Unicode são mantidos (sem reticências).
- O acréscimo é por abertura em modo de acréscimo (sem escrita atômica por troca); uma linha curta
  não se intercala com a de outro escritor. O caminho é informado ao Git como alterado, mas o ledger
  **nunca** entra no índice do Git (seção 7). Não há varredura de retenção dos arquivos mensais além
  da regra de meses da varredura de manutenção.

Um arquivo só é tratado como ledger se o **nome casa e o conteúdo também**:

- **nome:** exatamente `log.md`, ou `log-` seguido de 4 dígitos, `-`, 2 dígitos e `.md`. Comparação
  sensível à caixa (`LOG-2026-09.md` não casa). Mês e ano não são validados, só os dígitos.
  Não casam: `log.md.bak`, `log-2026-9.md`, `log-2026-09-01.md`, `log-2026.md`.
- **conteúdo**, decidido sobre os primeiros **8 KiB** do arquivo:
  1. se o arquivo encheu os 8 KiB, o trecho é cortado na última quebra de linha (uma linha lida pela
     metade não decide nada). Se não há quebra nesse trecho, o resultado é "não é ledger";
  2. trecho que não é UTF-8 válido: não é ledger. Arquivo ausente ou ilegível: não é ledger;
  3. se a primeira linha (sem espaços à direita) não é `---`, o arquivo é ledger se essa linha começa
     com `## [`. Uma primeira linha vazia já significa "não é ledger";
  4. se a primeira linha é `---`, procura-se a linha `---` de fechamento (comparada sem espaços à
     direita) nas **64** linhas seguintes. Sem fechamento nesse limite: não é ledger;
  5. depois do fechamento, a primeira linha que não é só espaço decide: é ledger se começa com `## [`.
     Linhas vazias entre o frontmatter e a entrada são permitidas. Sem essa linha: não é ledger.
- As linhas são separadas por LF, com CR final descartado. Um BOM no início faz a primeira linha não
  ser nem `---` nem `## [`.

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

- **Detecção de link.** Um item é "tipo link" se for um link simbólico (fora do Windows) ou, no
  Windows, se tiver o atributo de ponto de reparse (`FILE_ATTRIBUTE_REPARSE_POINT`, valor `0x400`).
  A verificação usa os metadados do próprio item, sem seguir o link. No Windows, o teste é só pelo
  atributo: links simbólicos, junções e **qualquer outro ponto de reparse** contam. Isso inclui os
  arquivos e pastas de provedores de nuvem que usam pontos de reparse (por exemplo, o OneDrive com
  arquivos sob demanda). Um diretório de dados dentro de uma pasta sincronizada assim tem as páginas
  recusadas pelo confinamento. O padrão do diretório de dados no Windows fica fora dessas pastas
  ([cli.md](cli.md) §2.5).
- **Caminho relativo à árvore.** O caminho precisa ser normalizado: não vazio, sem
  componentes `..`, `.`, raiz ou prefixo de unidade. A raiz é verificada, e então cada
  ancestral existente é percorrido: cada um deve ser diretório e não pode ser link.
  Ancestral ausente é criado ou não, conforme o modo:
  - **inspeção:** não cria nada e para na primeira ausência;
  - **pais:** cria os diretórios pais do alvo;
  - **diretório:** cria também o alvo.
  O alvo final, se existir, não pode ser link.
- **Validação da árvore inteira**, usada por rotinas administrativas
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
