# Fase 0: achados consolidados

Feature 001-nucleo, task T007. Entradas: as 6 notas (T001 a T006), spec.md, plan.md, constitution.md, tech.md e tasks.md. Nenhum arquivo do projeto foi alterado por esta análise. Os IDs "ADR-nn" abaixo são locais à Fase 0. O discovery já usa ADR-01 a ADR-10, então, fora deste documento, cite como "F0-ADR-nn" para evitar colisão. Toda decisão marcada como "decisão nossa" vale pela constituição ("a confirmar" nunca é fato; decidir e registrar em ADR). Os valores sem origem nas notas dizem de onde vem a justificativa.

## 0. Revisão das ADRs contra a referência (auditoria de 2026-10-18, passada 2)

> As ADRs abaixo foram decididas quando os docs tinham lacunas. A auditoria comparou cada uma com o
> comportamento de referência, agora descrito nos docs. Onde a tabela da seção 2 diverge desta, **vale
> esta seção** (e o doc citado). As linhas originais ficam como histórico. Itens marcados "decisão do
> autor" estão em tasks bloqueadas da `tasks.md`.

| ADR | Veredito | O que a referência faz (fonte) |
|---|---|---|
| ADR-02 | **mantida** | `\` é recusado, não convertido. A "normalização POSIX" do plan §2.1 foi corrigida (DV §4.2). |
| ADR-03 | **corrigida** | `ensure_portable` não faz parte do construtor de caminho: só a escrita de página a aplica, a caminho novo. A leitura, os links, a reindexação e o watcher usam só a validação básica (DV §4.2; wiki, escrita de página passo 1). |
| ADR-05 | **revogada (decisão do autor, T108)** | Não há lista de nomes reservados na validação de caminho. `_meta.md`, `bootstrap.md`, ledgers e `_pending/` são excluídos pelo watcher e pela reindexação, não pela validação (DV §4.2; wiki §8). |
| ADR-08 | **corrigida** | Na autoridade, o `kind` é usado como está (sem aparar, sem caixa: `Decision` ≠ `decision`) e, sem `kind`, vem do prefixo do caminho, senão `fact` (SR §11.5). |
| ADR-10 | **revogada** | Título de página não tem teto. O teto de 80 vale só para título de observação (prompts §11.1; wiki §3, título). |
| ADR-13 | **revogada** | Allowlist e padrões extras existem e valem na escrita de página (prompts §11.3; T093). |
| ADR-17 | **corrigida** | Ordem de inserção preservada; chaves novas no fim. A conformidade OKF acrescenta, nesta ordem e só se faltarem: `type`, `description`, `stale_after`, `sources`, `generated` (com `by`, depois `at`). Antes dela a escrita grava `tier` e, se fixada, `pinned`. Estilo de emissão: lista compacta (`-` na coluna da chave), bloco literal `|` para texto com quebra, aspas duplas só quando necessário, datas sem aspas, `{}`/`[]` para vazio, sem dobra de linha (wiki §3, emissão). |
| ADR-18 | **corrigida** | O arquivo **é** regravado mesmo com conteúdo idêntico (mesmos bytes, nova data de modificação); a herança de `generated.at` garante os mesmos bytes, e o evento resultante do watcher é absorvido pelo caso A. Emissão do bloco em LF, sem BOM; corpo com os fins de linha originais (wiki §3; §5.2 propriedades). |
| ADR-19 | **mantida** | Link de corpo grava `references`. Relação desconhecida é ignorada com aviso (wiki §3, relações). |
| ADR-21, ADR-22 | **revogadas** (já na 002/T080 e T089) | A escrita de página não escreve ledger. `log.md`/`log-AAAA-MM.md` são o ledger bruto de eventos de hook (wiki §9). |
| ADR-23 | **corrigida** | Formato do `_meta.md` de workspace e de projeto em wiki §1.1. Escrito na primeira escrita do escopo no processo, melhor esforço (falha só gera aviso). |
| ADR-24 | **revogada** (002/T089) | `index.md` só é criado pela migração OKF. Na operação normal ele é uma página comum: **não** é ignorado pelo watcher nem pela reindexação. |
| ADR-25, ADR-26 | **revogadas** | Não há commit por escrita de página. Commits automáticos só nos pontos de wiki §7, com o adaptador de escritas rastreadas (varredura completa nos casos do §7); a ferramenta MCP de escrita faz o commit `memory_write_page: <caminho>`. Nenhum commit é criado sem mudança. |
| ADR-27 | **corrigida** | Ignorados: metadados Git em qualquer nível, links e pontos de reparse, temporários, `_meta.md` em qualquer nível, `bootstrap.md` e `_pending/` só na raiz do projeto, ledgers por nome **e** conteúdo, e arquivos cuja extensão não é exatamente `.md` (em minúsculas). `index.md` **não** é ignorado (wiki §8). |
| ADR-29 | **corrigida** | O throttle de reforço fica na camada que atende a leitura, não no escritor; a chave é (página, operador) e o anônimo é um operador próprio que incrementa o contador (SR §8; T099). |
| ADR-31 | **revogada** | A lista de 57 palavras é a do doc, configurável (SR §11.2; T094). |
| ADR-34 | **decisão do autor (T113)** | A referência usa `refinery_schema_history` (DB §0; nome de tabela é contrato). Com outro nome, bancos da referência e do Go não são intercambiáveis. |

## 1. Resumo executivo

- As 6 notas apontam 40 lacunas e conflitos. Este documento decide todos, em 36 ADRs e 4 itens de dependências. Nenhum depende de resposta externa.
- A única pendência de política da constituição é de dependência. `golang.org/x/text/unicode/norm` (NFC da `PortableKey`) não consta em `tech.md`, e a constituição manda registrar ADR antes (ADR-01). Os demais pacotes planejados (yaml.v3, fsnotify, koanf e seus providers, cobra, uuid) já constam em `tech.md`.
- Os conflitos principais ficam resolvidos assim:
  - `\` em `PagePath` é rejeitado (ADR-02).
  - A 001 usa `log.md` único (ADR-21).
  - Go é 1.26 (ADR-35).
  - O controle de migração é `schema_migrations` (ADR-34).
- As regexes do sanitizador são decisão nossa. A base são as 18 propostas da nota T003, com um ajuste: JWT estrito, exigindo `eyJ` (ADR-12).
- Os fixtures `01` a `05` não trazem ranking esperado. O golden de ranking e o de sanitização são criados por nós (ADR-13 e ADR-14).
- Nenhuma decisão exige alterar `constitution.md`. Duas são sugestões opcionais, sujeitas a confirmação do usuário (seção 6).
- Veredito: PRONTO PARA FASE 1, com duas ações mecânicas antes do `go get` (seção 7).

## 2. Tabela de decisões ADR (todas "decisão nossa")

| ID | Tema | Opções consideradas | Decisão | Justificativa | Tasks afetadas |
|---|---|---|---|---|---|
| ADR-01 | Dependência `golang.org/x/text/unicode/norm` | (a) usar x/text; (b) NFC próprio; (c) só `strings.ToLower` sem NFC | Adotar x/text, só o subpacote `norm`. Acrescentar em `tech.md` | A stdlib não tem NFC. DV §4.2 exige NFC depois de minúsculas. Constituição: dependência nova exige ADR ou `tech.md`, e "preferir stdlib" cede quando a stdlib não resolve | T008, T010, T070, T075 |
| ADR-02 | `\` em `PagePath` | Converter para `/` ou rejeitar | Rejeitar. A "normalização POSIX" do plan §2.1 vale só para quem lê do disco: converte com `filepath.ToSlash` antes de chamar | DV §4.2 é literal. Falha fechada evita dois caminhos para o mesmo arquivo. Combina com T004, onde links com `\` são ignorados | T010, T040, T042, T048 |
| ADR-03 | `ensure_portable` e leitura tolerante | Uma função só; ou duas | `ParsePagePath` (estrito, já com portabilidade, só para caminho novo) e `ParseStoredPagePath` (só DV §4.2: sem portabilidade, para linhas já persistidas) | DV §4.2: apertar a leitura tornaria ilegível uma listagem inteira por causa de uma página antiga. O plan diz que `ParsePagePath` já recusa nomes não portáveis, então ele fica estrito. O segundo construtor tem chamador real em `ReadPage`/`Recent`/`PageChain`, o que respeita "sem abstração preemptiva" | T010, T029, T031, T046, T048 |
| ADR-04 | Lista de nomes não portáveis | Só `CON`/`NUL`/`:`/`?`; ou a lista completa do Windows | Por segmento, sem distinguir caixa e com ou sem extensão (`CON.md` é recusado): `CON PRN AUX NUL COM1..COM9 LPT1..LPT9`. Caracteres inválidos: `< > : " \| ? *` e controles 0x00 a 0x1F. Também recusa ponto ou espaço no fim do segmento | O plan §2.1 dá só exemplos. Esta lista vem da convenção pública de nomes do Windows, não das notas. DV §4.2: a regra é igual em todas as plataformas | T010 |
| ADR-05 | Segmentos reservados em `ParsePagePath` | Sem lista; ou lista mínima | Recusar: segmento `.git`; segmento com prefixo `.ai-memory-tmp.`; primeiro segmento `_pending` ou `_graveyard`; nome base `_meta.md`, `index.md`, `bootstrap.md`, `log.md` ou `log-AAAA-MM.md`. Não são reservados `_rules`, `_lint`, `_slots` nem `_global`: são famílias de página válidas (DV §14) | O plan cita "segmentos reservados" sem lista. WFL §8.1/§9 e OKF tratam esses nomes como não-página. Sem a reserva, `WritePage` sobrescreveria o ledger ou o manifesto | T010, T042, T045, T048, T049 |
| ADR-06 | Armazenar e checar `PortableKey` | Coluna nova e índice; ou consulta | Sem coluna. No caso C do `UpsertPage` (só página nova), listar os caminhos `is_latest=1` do escopo, comparar `PortableKey` em Go e devolver `ErrPathCollision`. Registrar o custo O(n) como débito | O plan P-01/§2.3 fixa V01 igual ao doc, e nome de coluna é contrato. O escritor único torna a consulta segura contra corrida. 5 mil páginas não afetam a escrita | T010, T017 (sem coluna), T029 |
| ADR-07 | Tipos de ID | Um `ID` só; ou tipos distintos | `ID [16]byte` base, mais tipos definidos `WorkspaceID`, `ProjectID`, `PageID`, `SessionID`, `ObservationID`. `Scope{WorkspaceID, ProjectID}`. Leitura de bytes exige exatamente 16 | DV §4.1 exige tipos distintos. INV-04 é "identidade tipada". Todos têm chamador na 001. Isto altera a assinatura de `Scope` no plan §2.1 (anotar no plan) | T009, T012, T014, T022, T024, T025 |
| ADR-08 | `PageKind` | Enum fechado; ou texto livre | `PageKind` é string tipada, igual ao `kind` do frontmatter (até 64, aparado, sem controle; vazio é ausente). Constantes só para `fact`, `note`, `procedure` e `decision` | DV §6.6 define `kind` como texto livre. DV §14 só usa os 4 valores na derivação OKF | T011, T040 |
| ADR-09 | Aliases de `ObservationKind` | Só os 3 exemplos; ou regra | Aceitar o canônico kebab. Para o resto, normalizar removendo `-` e `_`, em minúsculas, e comparar com o canônico normalizado (cobre `PostToolUse`, `session_start`, `PreCompact`, `PostCompaction`). Alias explícito `UserPromptSubmit` para `user-prompt`. Desconhecido gera erro "registro malformado" | DV §5.1 dá a forma canônica de 10 valores e 3 exemplos. A regra mecânica evita inventar tabela. Estrito para desconhecido, como DV §5.1 | T011 |
| ADR-10 | Limites do tipo `Sanitized` | Truncar tudo; ou só observações | Título: 80 runes, com `…` (U+2026) dentro dos 80. `Limits{TitleMaxRunes, BodyMaxBytes}`, com 0 igual a sem teto. Observação: título 80 e corpo 16384 bytes. Página wiki: sanitiza o corpo sem truncar (arquivo é a fonte da verdade) e o título derivado entra no índice com 80 runes. O arquivo não é cortado. Grafemas ficam fora | O plan P-07 diz "fronteira de rune". INV-06 fala em "observação". Truncar corpo de página destruiria o markdown. Os 80 no título de página são coerência nossa, sem fonte. Recomenda-se o usuário revisar, mas não altera a constituição | T013, T014, T035, T040, T046 |
| ADR-11 | Teto de 16 KiB com o marcador | Garantir por teste | O corte de cabeça e cauda parte de teto menos 48. Cada ponta é reduzida até fronteira UTF-8. O marcador `\n...[truncated N bytes]...\n` ocupa 27 bytes mais os dígitos de N, no máximo 48. O total fica ≤ teto. Teto < 64: só cabeça. Teste de propriedade: `len(saída) ≤ 16384` e UTF-8 válido. Marcador `[REDACTED:x]` cortado na ponta é aceito, pois não revela segredo | DV §7.4 fixa os 48 bytes e as regras. A conta fecha com os números do doc | T013 |
| ADR-12 | Regexes do sanitizador | Reproduzir o original (ausente); ou nossas | Adotar as 18 propostas RE2 da nota T003 como base, com ajuste: `jwt` exige `eyJ` no primeiro segmento. A forma frouxa redigiria `internal.storage.writer` em páginas wiki. Rótulo em `[a-z0-9_]+`. Sanitização idempotente. A lista de variáveis conhecidas de `env_secret` é dispensada: os sufixos (`_KEY`, `_TOKEN`...) já cobrem `OPENAI_API_KEY`, `GITHUB_TOKEN`, `HF_TOKEN` e `AWS_SECRET_ACCESS_KEY` | `docs-adiados/` está ausente. A spec manda reescrever os padrões a partir dos docs. Tem de ser clean-room. Falso positivo é aceitável e vazamento não (DV §7.1) | T013, T061, T064 |
| ADR-13 | Allowlist, `extra_patterns` e `[sanitize]` | Implementar agora; ou adiar | Adiar. `Limits` não ganha esses campos e `config` não ganha a seção `[sanitize]`. Registrar como débito de fidelidade | Constituição: "sem superfície pública sem chamador". Nenhuma task da 001 os consome | T013, T015 |
| ADR-14 | Fixtures de sanitização | Segredos literais; ou montados | `testdata/secrets.txt` guarda templates com marcadores de prefixo. O teste monta o valor (prefixo mais preenchimento), como o harness já faz com `{{HOME}}`. Cada classe tem caso positivo, negativo (`ASIAPACIFICREGION`, `Idempotency-Key`) e caso cortado pelo limite, que prova a ordem sanitizar antes de truncar. Fuzz de idempotência e de não-pânico | Spec: "nenhum segredo cru nas fixtures". Evita disparar scanners de segredo no repo | T013, T061, T064 |
| ADR-15 | Fixtures `01` a `05` e golden de ranking | Usar os fixtures como estão; ou golden próprio | Os fixtures são observações e não têm ranking esperado. O teste de aceitação grava as observações (cobre `raw_hits`) e escreve páginas derivadas de `name` e `description` de cada sessão. O ranking esperado é golden nosso, em `test/acceptance/testdata`. Os 4 fixtures de captura, rota, remote e Kiro são da feature 002 e a 001 não os usa | A spec cita "fixtures de docs/assets/fixtures" sem esperado (AMBIGUOUS). T002 confirma que não há `expected` | T055, T057 |
| ADR-16 | Nomes dos testes `TestINV16_*` | Livre | `TestINV16_ConcurrentSupersede`, `TestINV16_LoserReachable`, `TestINV16_ReadNotFilteredByAuthor`. Duas sessões (`Writer` e `Reader` sobre o mesmo banco temporário), com `t.Parallel` e `-race`. O número de escritas é decidido na T060 | Plan P-10. Nenhuma fonte fixa nomes ou contagens | T060, T064 |
| ADR-17 | Ordem das chaves e estilo do YAML | Reordenar; ou preservar | Chaves existentes na ordem original. Derivadas acrescentadas ao fim, nesta ordem: `type`, `description`, `generated`, `sources`, `stale_after`. `yaml.v3` sobre `Node`, indentação 2, LF, sem BOM | O plan modela "mapa ordenado". Nenhuma fonte fixa ordem de emissão | T014, T039, T040 |
| ADR-18 | CRLF, BOM e bytes idênticos | Normalizar o arquivo; ou comparar conteúdo | A emissão do bloco é sempre LF, nunca BOM. O corpo mantém seus fins de linha, e arquivo com fins de linha mistos é aceito. Antes de gravar, compara conteúdo (corpo mais frontmatter sem `generated.at`, título, tier e fixação). Se igual, não toca no arquivo nem cria versão | OKF "Determinismo" e WFL §5.2 definem a regra de conteúdo idêntico. A lacuna era só a emissão | T039, T046, T048 |
| ADR-19 | `link_type` e relação desconhecida | Ignorar; ou virar referência | Link de corpo grava `references` (o DEFAULT do schema). Chave de `relations` fora de `causes\|fixes\|contradicts`: o alvo vira `references` e o log registra aviso (valor truncado a 200 bytes). Dedupe pela chave lógica antes de inserir | DV §6.8 ("não é erro, vira referência simples") e WFL §3.5 ("ignorada com aviso") são compatíveis nesta leitura | T026, T040 |
| ADR-20 | `generated_by_model` e importação OKF "a confirmar" | Implementar; ou adiar | Se presente, `generated.by = generated_by_model`. Senão, `process:ai-memory/<versão do build>`. Importação, exportação e reescrita de links OKF ficam fora da 001. Arquivo desempacotado é tratado como edição manual (watcher e reindex normais) | Constituição: decidir "a confirmar" em ADR. A exportação é de escopo posterior | T039, T040 |
| ADR-21 | `log.md` ou `log-AAAA-MM.md` | Um dos dois | A 001 escreve `log.md` único. Watcher e reindex ignoram `log.md` e `log-AAAA-MM.md`. Rotação mensal fica para a feature 002. Linha: `## [AAAA-MM-DDTHH:MM:SSZ] evento \| título`, com evento em `page-write` ou `page-delete` e título já sanitizado | A spec aprovada (cenário principal, passo 4) e o plan §2.4 dizem `log.md`. A frase do OKF ("não adota") é sobre o pacote de exportação | T045, T046, T047, T049 |
| ADR-22 | Ordem do `log.md` e do commit | Linha depois do commit; ou antes | Anexar ao `log.md` antes do commit e incluir `log.md` no mesmo commit (um commit por evento). Falha ao anexar vira aviso | A spec cita as duas ações sem ordem. Assim o checkpoint devolvido já contém a linha e a árvore não fica suja | T045, T046, T043 |
| ADR-23 | Chaves de `_meta.md` | Só projeto; ou projeto e workspace | Escrever na primeira escrita do escopo, projeto e workspace (`<ws>/_meta.md`). Frontmatter: `type: Scope Manifest`, `name`, `workspace_id` (mais `project_id` e `repo_path` quando for projeto), sem corpo. Se faltar no reindex, o escopo é reconstruído pelos UUIDs dos diretórios e o nome vira o próprio id. Aviso no log | WFL §1, §5.2 passo 7 e LOPS mostram os dois manifestos. As chaves são nossas | T045, T048 |
| ADR-24 | Criação de `index.md` | Criar; ou só na migração | Criar uma vez se ausente. Frontmatter `okf_version: "0.2"`, título `# Índice`, sem lista. A operação normal não o regrava. A lista de diretórios fica para exportação ou migração (fora da 001) | O plan §2.4 lista `index.md` no layout. OKF diz que a operação normal não o escreve | T045 |
| ADR-25 | Mensagens de commit e `git init` | Livre | `page-write: <ws>/<proj>/<path>`; `page-delete: <ws>/<proj>/<path>`; `checkpoint: <mensagem>` para o comando de commit. `git init --initial-branch=main` e `.gitignore` interno com `.ai-memory-tmp.*`. Sem commit inicial. Autor `ai-memory <ai-memory@local>` e `commit.gpgsign=false`, conforme P-09 | WFL/OKF só trazem as mensagens da migração e da compactação, que estão fora da 001 | T043, T046, T047 |
| ADR-26 | Varredura do commit por caminho | Adaptador de escritas rastreadas do WFL §7; ou add por evento | Só `git add -- <caminhos do evento>` mais `commit`, sob mutex. Sem `walk_needed`, sem varredura de 600 s e sem `unreported_writes`. Débito registrado | Plan P-09 manda `add` e `commit` por evento. O adaptador é do go-git, que a constituição veta | T043 |
| ADR-27 | Ignorados pelo watcher e pelo reindex | Lista do WFL §8.1/§8.2 | Ignorar: `.git/`, symlinks e pontos de reparse, prefixo `.ai-memory-tmp.`, `_meta.md`, `index.md`, `bootstrap.md`, `log*.md`, `_pending/`, `_graveyard/`, não-`.md`, e caminho fora de `<ws>/<proj>/...`. Demais diretórios ocultos não são pulados | WFL §8.1 e §8.2 (que diz explicitamente "ocultos não são pulados"). Combina com ADR-05 | T048, T049 |
| ADR-28 | Mapa tipo para diretório e `Tier` para OKF | Definir mapa; ou deixar com o chamador | A 001 não define mapa nem mapeamento de tier. O chamador escolhe o caminho. Os diretórios nascem sob demanda. `WritePage` exige `Tier` explícito. Na indexação sem `tier` no frontmatter, o padrão é `semantic`. A retenção por tier fica fora da 001 | DV §6.1 define semantic como "as páginas da wiki". `storage-and-retrieval.md` não foi lido para retenção, que não é da 001 | T011, T040, T046, T048 |
| ADR-29 | Throttle do `BumpAccess` | Coluna nova em `page_access`; ou memória | Estado em memória no `Writer`: mapa (PageID corrente, actor) para último bump. Máximo 1 por minuto. Relógio injetável nos testes. Entradas expiradas são limpas de forma preguiçosa para não crescer sem limite. O estado se perde no reinício | `page_access` não tem timestamp, e o schema é contrato (V01 igual ao doc). Constituição: sem fan-out ilimitado | T036 |
| ADR-30 | Itens opcionais da busca | Implementar; ou adiar | Fora da 001: `as_of`, `include_superseded`, variante explicada, roteamento de recall de sessão e peso de crença. A versão perdedora é alcançável por `PageChain` | SPEC e PLAN P-05 não os citam. Sem chamador implica não implementar | T033, T034, T037 |
| ADR-31 | Stop words (57) e sintaxe explícita | Buscar a lista; ou definir | Lista embutida fixa de 57 palavras comuns do inglês, escolhida por nós na T032, com teste que confere a contagem. A sintaxe explícita (aspas, parênteses, `OR\|AND\|NOT\|NEAR`) vai direto à sonda, sem stop words nem OR, e degrada se a sonda falhar. Não há stop words em português: débito | O doc diz "57" mas não lista. Plan P-05 mantém 57 | T032 |
| ADR-32 | Formato do `raw_hits` | Livre | `RawHit{ObservationID, SessionID, Kind, Title, Snippet, Rank}`, limitado pelo limite da busca, escopo workspace e projeto. A consulta usa `prepareFTS5Query` | A spec só diz "raw_hits por FTS". O doc não define o formato | T035, T037 |
| ADR-33 | Primitivos de SO sem dependência nova | `golang.org/x/sys`; ou stdlib | Erros 32/33 por `errors.As` com `syscall.Errno`, e checagem de processo vivo com `syscall` por build tag. Se isso se mostrar insuficiente no Windows, abrir ADR antes de importar `x/sys` | Constituição: preferir stdlib e exigir `tech.md` | T041, T052 |
| ADR-34 | Controle de migração | `refinery_schema_history` (DB §0); ou `schema_migrations` (plan P-01) | `schema_migrations(version,name,applied_on,checksum)`. A tabela do refinery não entra em V01 | O próprio DB marca o nome como "a validar". O plan P-01 define o runner próprio | T017, T018, T021 |
| ADR-35 | Versão do Go | 1.25 (contexto da task); ou 1.26 | `go 1.26` no `go.mod`. O toolchain local é 1.26.5, e o CI usa a versão do `go.mod` | O plan §1 e o `tech.md` (detectado) dizem 1.26. O 1.25 vem só do contexto de agente | T008, T067, T069, T075 |
| ADR-36 | Mensagens e `tech.md` pós-go.mod | Manter "planejado" | Atualizar `tech.md` com as versões reais após o `go.mod` (já previsto) | O `tech.md` pede reexecução após criar o `go.mod` | T070, T075 |

## 3. Lacunas: bloqueantes versus resolvíveis durante a implementação

**Bloqueantes para a Fase 1.** Todas foram decididas acima e precisam apenas ser persistidas, pois afetam o contrato de T008 a T017:

| Lacuna | Decisão | Por que antes da Fase 1 |
|---|---|---|
| `x/text` fora do `tech.md` | ADR-01 | `go get` da T008 não pode vir antes da listagem |
| `\` e `ensure_portable` | ADR-02, ADR-03 | Definem a API de `PagePath` (T010) |
| Lista de não portáveis e reservados | ADR-04, ADR-05 | Definem a T010 e a T042 |
| Tipos de ID | ADR-07 | Alteram `Scope` do plan §2.1 (T009, T012, T014, T017) |
| Armazenar `PortableKey` | ADR-06 | Decide que o schema V01 não ganha coluna (T017) |
| Nome da tabela de migração | ADR-34 | T017 e T018 |
| Limites do `Sanitized` | ADR-10, ADR-11 | Fixam a assinatura de `sanitize.Text`, usada pelas T014, T035 e T046 |
| Go 1.26 | ADR-35 | T008 |

**Podem ser decididas durante a implementação.** Já têm decisão provisória, ajustável no código e nos testes:
- Lista de 57 stop words (ADR-31).
- Contagens e nomes finais dos testes `TestINV16_*` (ADR-16).
- Ajuste fino das regexes de `telegram_token`, `credential_path` e `~/.ssh` (ADR-12).
- Estilo exato do YAML (ADR-17).
- Texto do vocabulário de evento do log (ADR-21).
- Formato do `raw_hits` (ADR-32).
- Critério "fixtures esperados" da spec (ADR-15).

## 4. Conflitos entre fontes

| Conflito | Fontes | Decisão |
|---|---|---|
| `\` em `PagePath`: rejeitar ou converter | DV §4.2 contra plan §2.1 | Rejeitar (ADR-02) |
| `log.md` contra `log-AAAA-MM.md` | spec e plan contra WFL, OKF, LOPS | `log.md` na 001 (ADR-21) |
| Go 1.25 contra 1.26 | contexto da task contra plan e `tech.md` | 1.26 (ADR-35) |
| `refinery_schema_history` contra `schema_migrations` | DB §0 contra plan P-01 | `schema_migrations` (ADR-34) |
| `ensure_portable` separada ou embutida | DV §4.2 contra plan §2.1 | Dois construtores (ADR-03) |
| "80 caracteres exibidos" contra "fronteira de rune" | DV §7.4 contra plan P-07 | Runes (ADR-10) |
| Um `ID` contra tipos distintos | plan §2.1 contra DV §4.1 | Tipos distintos (ADR-07) |
| Relação desconhecida: ignorada com aviso ou referência | WFL §3.5 contra DV §6.8 | Vira `references` com aviso (ADR-19) |
| `index.md`: escrito na 001 ou só na migração | plan §2.4 contra OKF | Criado uma vez se ausente (ADR-24) |
| Watcher e ocultos | WFL §8.1 contra §8.2 | Só os ignorados listados (ADR-27) |
| Teto de 16 KiB também no corpo de página | INV-06 ("observação") contra spec ("texto de entrada") | Só em observação; página é sanitizada sem truncar (ADR-10) |

## 5. Dependências detectadas contra `tech.md`

| Dependência | Origem | Consta no `tech.md` | Ação |
|---|---|---|---|
| `golang.org/x/text/unicode/norm` | T001 (e.1), `PortableKey` | Não | Pendência de ADR: ADR-01. Acrescentar linha em `tech.md` antes da T008 |
| `gopkg.in/yaml.v3` | plan §1 | Sim | Nenhuma |
| `github.com/fsnotify/fsnotify` | plan P-08 | Sim | Nenhuma |
| `github.com/knadh/koanf` (providers e parsers file, env, toml, dotenv) | plan P-11 | Sim | Nenhuma |
| `github.com/spf13/cobra` | plan §1 | Sim | Nenhuma |
| `github.com/google/uuid` | plan §1 | Sim | Nenhuma |
| `modernc.org/sqlite` | plan §1 | Sim | Nenhuma |
| `golang.org/x/sys` | possível na T041 e T052 | Não | Não adotar (ADR-33). Abrir ADR só se a stdlib falhar |
| Biblioteca de Gherkin (por exemplo `godog`) | nenhuma task a cita; T055 gera só o `.feature` | Não | Não adotar. Os cenários são implementados como testes Go (T056 a T061), sem dependência nova |

## 6. Decisões que tocam a constituição

Nenhuma decisão exige alterar `constitution.md`. Nada foi alterado. As decisões abaixo interagem com ela. As duas marcadas como "confirmar" só se aplicam se o usuário quiser alterar o texto da constituição:

- **Confirmar com o usuário (opcional):** acrescentar uma referência aos ADRs de Fase 0 ao lado de INV-06 ("a confirmar") e de INV-05. A constituição já diz que a decisão fica em ADR, então a alteração não é necessária. Recomendo não alterar.
- **Confirmar com o usuário (opcional):** ADR-10 estende o teto de 80 do título (INV-06 cita só observação) ao título de página no índice. Se o usuário não quiser, basta remover essa parte do ADR, e a constituição fica intacta.
- Sem alteração, mas com registro: ADR-01 (x/text) aplica a regra de dependência da constituição. ADR-03 e ADR-13 respeitam "sem abstração preemptiva". ADR-26 respeita "sem go-git". ADR-33 respeita "preferir stdlib".
- O `tech.md` não é a constituição, mas precisa de uma linha nova por ADR-01.
- O plan §2.1 muda em dois pontos (assinatura de `Scope` pelo ADR-07; `ParseStoredPagePath` pelo ADR-03). Anotar no plan.md.

## 7. Veredito

**PRONTO PARA FASE 1.**

O que falta (mínimo, mecânico, sem decisão pendente):
1. Persistir os ADR-01 a ADR-36 (este documento vira `specs/features/001-nucleo/notes/phase0-findings.md`) e anotar no plan.md os dois ajustes de ADR-03 e ADR-07.
2. Acrescentar `golang.org/x/text` (subpacote `unicode/norm`) em `.github/memory/tech.md` antes do `go get` da T008. Esta é a única pendência de ADR sobre dependências.

Nada aqui precisa de resposta do usuário. As duas confirmações da seção 6 são opcionais e não bloqueiam.
