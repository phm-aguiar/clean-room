# Spec: Núcleo (domínio, armazenamento e wiki)

## Metadados
- **ID:** 001
- **Status:** concluída (2026-10-13), com as correções contra os docs abertas como tasks na 002 (ver "Revisão contra os docs"); **reaberta** pela revisão de clean room de 2026-10-18 e pelas passadas da auditoria contra o comportamento de referência do mesmo dia (ver "Revisão de clean room", "Auditoria contra a referência", "Revisão da documentação contra a referência 3.1.1" e "Tasks da passada 2"; tasks T091 a T119)
- **Autor:** phm-aguiar
- **Data:** 2026-10-12
- **Feature Anterior:** — (primeira feature)
- **Discovery Report:** `reports/001-ai-memory-go-clean-room-discovery.md` (F001)
- **Aprovado:** true

## Propósito
> Para quem, o quê e por quê.

O ai-memory-go é uma reimplementação clean-room, em Go, do ai-memory: um servidor único que dá memória de
longo prazo a agentes de código. Antes de capturar sessões ou expor MCP, é preciso o alicerce que todo o
resto consome: um domínio tipado sem I/O, um índice SQLite com escritor único e busca híbrida, e uma wiki
em markdown versionada em git, que é a fonte da verdade. O público é o autor (dev solo, Windows e Linux).

Importa agora porque as invariantes (sanitização como fronteira de tipo, índice na mesma transação dos
dados, escrita atômica, isolamento por `(workspace_id, project_id, path)`) são caras de retrofitar. Esta
feature é **zero-LLM** e entrega o núcleo testável por contratos, sem hooks nem MCP.

## Escopo

### Dentro do escopo
- **Domínio** (`internal/domain`, sem I/O): UUIDv7, identidade `(workspace_id, project_id, path)`, caminho de página portável, tiers, tipos de observação, tipo sanitizado e sanitizador.
- **Configuração** (carregada uma vez): TOML + `AI_MEMORY_*` com `__`, diretório de dados absoluto e canônico registrado no log, logger `slog` com filtro do próprio módulo.
- **Armazenamento** (`internal/storage`): SQLite pure-Go (WAL, FTS5), migrações `V<nn>__nome.sql` **sem** cópia do banco antes de migrar (T118), escritor único (goroutine + canal de 1024), pool de leitura, resolução central de escopo. Escopo explícito inexistente falha fechado; leitura sem escopo usa o ponteiro ou o padrão e informa a origem.
- **Busca**: FTS5 + casamento de entidades + vizinhos de link do mesmo projeto, fundidos por RRF (k = 60), multiplicador de autoridade `[0,55; 1,50]`, fallback para observações cruas e reforço de acesso throttled (1/min por página e operador, anônimo incluso). Contrato: `docs/storage-and-retrieval.md` §11. A referência 3.1.1 tem também o **fluxo de observações**, ligado por padrão; fica fora da 001 como desvio registrado e vai para a 002, junto das páginas de sessão (T117).
- **Wiki** (`internal/wiki`): leitura e emissão de frontmatter pelo contrato de YAML do doc (T111, T119), escrita atômica, upsert na mesma transação com rollback do arquivo, supersessão, watcher (debounce 300 ms) e reconciliação de 30 s. A escrita de página **não** faz commit git nem acrescenta linha a `log.md`: o caminho fica registrado para o próximo commit automático (002/T080).
- **Reindex** a partir do markdown (o SQLite é derivado), sem higienizar o conteúdo lido do disco na referência (decisão em T115), e o primitivo de cópia online do SQLite (o backup completo é tarball, 002/T081 e T082).

### Fora do escopo (explicitamente)
- Hooks, `/hook`, spool, sessões e handoffs (**feature 002**).
- Servidor MCP e CLI de usuário, exceto o mínimo para executar os testes (**feature 002**).
- LLM, embeddings, reranker, consolidação, lint por LLM (features 016 a 022, ver `roadmap.md`).
- Multiusuário, SSO, autorização por projeto, webhooks de admissão, perfil entre projetos (features 007, 008, 009 e 012, ver `roadmap.md`).
- Navegador web, workstreams, macOS e demais agentes.

## Comportamento Esperado

### Cenário Principal (Happy Path)
1. O chamador pede a escrita da página `decisions/db.md` no projeto `demo`. O escopo é resolvido pelo resolvedor central de escrita, o único que cria workspace e projeto.
2. O caminho passa pelo confinamento e pela portabilidade. O corpo e as strings do frontmatter passam pelo sanitizador configurado. O frontmatter recebe a forma canônica: `tier` do pedido, `pinned: true` se fixada, chaves OKF e `generated.at`.
3. Sob a guarda compartilhada e a trava da página, a escrita confere que o projeto existe e pertence ao workspace, sem criar nada (T109), e garante os `_meta.md`. O arquivo é instalado de forma atômica (temporário, `fsync`, `rename`, `fsync` do arquivo, `fsync` do diretório em melhor esforço), mesmo quando os bytes são iguais aos atuais (T109). A página e os índices FTS, entidades e links commitam em **uma** transação do escritor único.
4. A resposta traz o identificador da versão e o caminho. A escrita **não** faz commit e **não** escreve em `log.md`: os caminhos ficam registrados para o próximo commit automático (`docs/wiki-files-layer.md` §5.2 e §7).
5. Uma consulta posterior retorna a página, ranqueada por RRF entre FTS, entidades e vizinhos de link, com autoridade aplicada.

### Cenários Alternativos
- **Falha no banco após instalar o arquivo:** o arquivo é revertido, o erro é propagado de forma não ambígua e o índice permanece inalterado. Se a reversão também falhar, o erro é de estado inconsistente.
- **Escrita em projeto inexistente ou de outro workspace:** erro de não encontrado antes de tocar o disco; nada é criado.
- **Leitura com escopo explícito inexistente:** falha fechada com erro de não encontrado; nenhum workspace ou projeto é criado. Só workspace, sem projeto: erro de entrada. Só projeto: procurado no workspace ativo e depois no padrão.
- **Leitura sem escopo:** nunca falha por falta de escopo; usa o ponteiro de leitura ou o projeto padrão e informa a origem (`docs/storage-and-retrieval.md` §14.1).
- **Nenhuma página compilada casa:** a consulta devolve `raw_hits` por FTS nas observações do escopo.

## Edge Cases
- **Escrita concorrente no mesmo caminho:** uma versão é escrita e a outra supersede a anterior numa cadeia; a perdedora continua alcançável; a leitura não é filtrada por autoria (INV-16).
- **Edição manual da wiki no disco:** o watcher (300 ms) ou a reconciliação (30 s) reindexa. Arquivos temporários são ignorados pelo prefixo `.ai-memory-tmp.`; a escrita já instalada pelo próprio servidor é absorvida porque reindexar conteúdo idêntico não cria versão (`docs/wiki-files-layer.md` §8.1). O identificador do arquivo (inode ou índice NTFS) não participa disso. A reindexação lê o arquivo como está, sem higienizar (decisão em T115). Diretório de projeto sem linha no banco é pulado inteiro.
- **Frontmatter escrito à mão:** é lido pelo contrato de YAML do doc (`docs/wiki-files-layer.md` §3.1 e §3.2): `pinned: true` é booleano, `yes` é texto, `007` é texto, inteiro fora de 64 bits é erro, fechamento `---` no fim do arquivo sem quebra é aceito e CRLF é aceito quando abertura e fechamento usam a mesma quebra. Frontmatter inválido impede a indexação daquela página e gera aviso, sem parar a reconciliação.
- **Windows:** extensão `.md` só em minúsculas, nomes de dispositivo e caracteres proibidos recusados na escrita (`ensure_portable`), repetição da troca atômica em violação de compartilhamento ou de bloqueio, diretório de dados padrão `%LOCALAPPDATA%\ai-memory` com forma canônica `\\?\` (`docs/cli.md` §2.5).
- **Segredo no texto de entrada:** o segredo é removido **antes** de limitar tamanho, numa passada única (título de observação ≤ 80 caracteres; corpo de observação ≤ 16 KiB). Página não tem teto de corpo nem de título. As classes `\s`, `\d` e `\b` dos padrões são Unicode (um `Bearer` com espaço não separável precisa ser redigido). O tipo sanitizado só nasce da função de sanitização, e a escrita de página usa o sanitizador configurado (extras e allowlist do operador).
- **Banco corrompido ou ausente:** `reindex` reconstrói o SQLite a partir do markdown, com guarda de processo vivo.

## Constraints
- **Performance:** consulta com 5 mil páginas < 100 ms (baseline registrado em benchmark); leitura nunca bloqueia no escritor.
- **Tecnologia:** Go, `modernc.org/sqlite`, `git` do sistema, `slog`, `regexp` RE2; `CGO_ENABLED=0`; módulo único; grafo de imports da constituição. **Atenção (auditoria 2026-10-18):** a referência usa biblioteca Git embutida, e só no Windows cai para o `git` do sistema. Com o executável, a implementação precisa reproduzir `docs/wiki-files-layer.md` §7 por conta própria: autor fixo, ledgers fora do índice, nenhum commit sem mudança, e não herdar hooks, assinatura nem identidade do usuário. O RE2 tem `\s`, `\d` e `\b` só ASCII, e os padrões de segredo exigem a semântica Unicode (`docs/prompts-schemas-and-sanitizer.md` §11.2).
- **Prazo:** não especificado.
- **Segurança/Compliance:** segredos nunca persistidos crus; comparação de token em tempo constante; escuta padrão `127.0.0.1`; diretório de dados absoluto e canônico.

## Critérios de Sucesso
- [x] `CGO_ENABLED=0 go build ./...` e `go vet ./...` passam para `windows/amd64` e `linux/amd64`
- [x] `go test -race ./...` passa, incluindo testes multi-sessão (supersessão e escritor)
- [ ] Escrever uma página produz arquivo e linhas no SQLite consistentes, sem commit git nem linha em `log.md`, e reverte o arquivo quando o banco falha. **Reaberto em 2026-10-18:** o rollback está coberto; a ausência de commit e de `log.md` na escrita vem do doc (o commit é do ponto de commit automático, 002/T080); a criação de escopo dentro da escrita precisa sair (T109)
- [ ] Um arquivo escrito à mão no formato dos casos dourados de `docs/wiki-files-layer.md` §3.2 e §3.3 é lido com os mesmos valores da referência e reemitido com os mesmos bytes (T111, T119)
- [ ] As tasks BLOQUEADAS T107, T108, T113 e T115 têm decisão do autor registrada no `plan.md`
- [x] Busca RRF retorna os resultados esperados nas fixtures de `docs/assets/fixtures`
- [x] Nenhum segredo cru persistido nas fixtures de sanitização
- [x] Invariantes INV-01..06, 09..11, 13, 15 e 16 têm teste de contrato nomeado ou débito registrado
- [x] Testes automatizados cobrem o happy path e os edge cases identificados

> **Evidência (2026-10-13):** `sh scripts/check.sh` completo e verde (build cruzado, `go vet` nos dois alvos e `go test -race ./...` em todos os pacotes, no container `golang:1.26` com gcc 14.2) e `golangci-lint` v2.14.0 com 0 apontamentos, na última versão do código Go da 001 (depois de 001/T083 a T085). Uma repetição posterior foi interrompida por queda do Docker Desktop, sem mudança de código entre as duas. Testes: `test/acceptance/write_page_test.go` e `rollback_test.go` (critério 3), `search_rrf_test.go` (4), `secret_sanitization_test.go` (5), `test/contract/*_inv_test.go` e `notes/inv-test-matrix.md` (6), `concurrent_supersession_test.go` e `manual_edit_test.go` (multi-sessão e edge cases). CI do GitHub fora do critério, por decisão do autor (001/T090).

## Abordagem Escolhida
Módulo Go único com camadas em `internal/`, sem CGO. O domínio não faz I/O. O armazenamento usa
`modernc.org/sqlite` e uma goroutine dona da conexão de escrita (canal de capacidade 1024, o que dá
backpressure natural). A wiki usa o `git` do sistema e a escrita atômica de arquivo. Decisões em ADR-01,
02, 03, 04, 07, 08 e 10 do discovery.

Débitos relevantes: `docs-adiados/` ausente na época (hoje em `docs/prompts-schemas-and-sanitizer.md` §11; alinhamento na 002/T093) (os padrões de segredo do sanitizador serão reescritos a
partir dos docs e de fixtures próprias); pontos "a confirmar" decididos por nós via ADR; desempenho do
FTS5 do modernc ainda não medido.

### Alternativas Consideradas
| Abordagem | Trade-off | Por que não |
|-----------|-----------|-------------|
| `mattn/go-sqlite3` (CGO) | Mais rápido e maduro | Exige toolchain C; complica build e cross-compile no Windows |
| `go-git` | Sem dependência externa de git | Operações lentas ou incompletas; ignora configuração do repositório do usuário |
| `go.work` com módulos por componente | Isolamento forte | Overhead alto neste estágio; o grafo é verificável por teste de imports |
| Mutex sobre uma conexão | Mais simples | Sem backpressure nem lote de comandos |

## Revisão contra os docs (2026-10-13)
> Feita antes de executar a 002. Neste projeto o doc vence a spec. A 001 continua implementada como foi aprovada; as divergências abaixo viram tasks de correção na 002 (`specs/features/002-captura-e-acesso/tasks.md`, fase 1b).

| Spec 001 dizia | Doc define | Fonte | Correção |
|---|---|---|---|
| Diretório de dados configurável (padrão `data`, relativo ao diretório atual); só `--config` | Padrão no diretório de dados local do usuário, subpasta `ai-memory`; `--data-dir` > `AI_MEMORY_DATA_DIR` > arquivo; `config.toml` no diretório de dados | `docs/design-decisions.md` (diretório de dados padrão), `docs/cli.md` §1, §2.1 e §2.5 | 002/T079 |
| "commit git por evento" (cada escrita de página faz commit `page-write`) | Escritas rastreadas por caminho; commits automáticos em pontos definidos (fim de sessão com `session <id8>: <título>`, checkpoints, comando de commit, `memory_write_page`) | `docs/wiki-files-layer.md` §7 | 002/T080 |
| "backup online do SQLite" (`backup --to` grava um arquivo SQLite) | `backup --to` (ou `-o`) pede ao servidor um tarball gzip com `wiki/`, `db/memory.sqlite` e `config.toml` | `docs/lifecycle-ops.md` § backup, `docs/cli.md` §9, `docs/mcp-server.md` §6 | 002/T081 e T082 |
| Reindex reconstrói o nome do escopo pelo id quando falta `_meta.md` | Manifesto ausente é erro com o caminho exato | `docs/lifecycle-ops.md` § reindex | 002/T083 |
| Reindex sem checagem do banco | Recusa se as tabelas reconstruídas já têm linhas e nomeia as contagens | `docs/lifecycle-ops.md` § reindex | 002/T084 |
| (ausente) | Boot do servidor preenche `_meta.md` faltante de forma idempotente | `docs/cli.md` §3.1 (passo 11), `docs/lifecycle-ops.md` § reindex | 002/T085 |
| Watcher só conta remoções | Remoção registra o caminho no Git para o próximo commit | `docs/wiki-files-layer.md` §8.1 | 002/T086 |
| Ledger reconhecido pelo nome (`log*.md`) | Nome exato (`log.md` ou `log-AAAA-MM.md`) e conteúdo que abre com entrada, até 8 KiB | `docs/wiki-files-layer.md` §9 | 002/T022 |
| Guarda de processo vivo por `ai-memory.pid` | `.serve.lock` e enumeração dos processos do sistema | `docs/cli.md` §3.1, `docs/overview.md` item 9 | 002/T040 e T041 |
| Escrita e exclusão de página acrescentam linha em `log.md`, garantem `index.md` e fazem commit | Escrita de página sem linha de log, sem `index.md` e sem commit; ledger é só de eventos de hook; `log.md` não é adotado; `index.md` só pela migração OKF; páginas de `_slots/` são fixadas | `docs/wiki-files-layer.md` §5.2, `docs/okf.md` (arquivos no diretório da wiki) | 002/T080 e T089 |
| Frontmatter só aceita texto e lista de texto; mapeamento é recusado (001/T084 manteve a recusa); `generated.by`/`generated.at` gravados como chaves planas; `relations` lido como lista de texto | `generated: {by, at}` é mapeamento; `sources` é lista de mapeamentos; `relations` é mapeamento por relação (`{fixes: [...]}`). Um arquivo escrito à mão no formato do doc era recusado inteiro | `docs/okf.md` § Proveniência, `docs/typed-edges.md` | 002/T090 e T091 |
| Relação desconhecida no frontmatter vira `references` | Chave desconhecida é ignorada com aviso (conflito com `docs/domain-vocabulary.md` §6.8, resolvido pelo doc específico) | `docs/typed-edges.md`, `docs/wiki-files-layer.md` §3.5 | 005/T009 |
| Links reapontados só na supersessão | Toda nova versão reaponta todas as linhas cujo alvo casa com o caminho, inclusive as já resolvidas | `docs/typed-edges.md` § Storage | 005/T022 |
| Exclusão por caminho remove só a versão corrente; versão diferente da esperada é erro | Remove fisicamente todas as versões do caminho; com versão diferente, não faz nada e informa que não removeu | `docs/storage-and-retrieval.md` §5.2 | 002/T087 |
| Ressurreição mantém o `superseded_at` da lápide e fecha o `valid_to` | Limpa o `superseded_at` da lápide e mantém o `valid_to` original | `docs/storage-and-retrieval.md` §5.1 (caso C) | 002/T088 |
| Origens de escopo `explicit`, `default`, `repo_path` | `Explicit`, `Session`, `SharedSlot`, `StartupSeed`, `Default`, `DefaultAfterMismatch` | `docs/storage-and-retrieval.md` (resolução de escopo) | 002/T020 |

## Revisão de clean room (2026-10-18)
> Revisão do código Go contra a documentação e contra o comportamento de referência, sem reproduzir código de referência. Os itens abaixo são desvios ou lacunas. As decisões foram tomadas pela auditoria em 2026-10-18 com base na documentação e marcadas como **decidido**; o autor pode revertê-las. O código Go ainda não segue as decisões marcadas: a correção é trabalho de implementação.

| Item | Situação | Ação |
|---|---|---|
| Stop words de busca: 57 palavras sem lista nos docs (LACUNA em `notes/search-contract.md`); a lista Go diverge do comportamento de referência | **revogado pela auditoria** | a lista que esta revisão pôs no doc era inventada (só a contagem 57 conferia). O doc §11.2 agora traz a lista real. A lista Go precisa trocar 21 palavras (T094) |
| Sintaxe explícita: a degradação troca a consulta inteira, perdendo frases e operadores | **revogado pela auditoria** | a referência degrada a consulta **inteira** para saco de palavras citadas (doc §11.2, item 6). O Go já faz isso e não deve mudar para "token a token"; falta a variante dividida (T095) |
| `UpsertInput` recebe texto sem o tipo `Sanitized` (P-07 exige o contrário em storage e wiki) | desvio do plano | corrigir a assinatura; `InsertObservation` já cumpre |
| Falha de fsync do diretório depois do rename reverte a escrita (P-06 diz "melhor esforço") | desvio do plano, **decidido** | melhor esforço: vira aviso e não reverte a escrita (`docs/wiki-files-layer.md`:163 já diz isso) |
| Corte de corpo abaixo de 64 bytes: marcador com contagem (Go) ou reticências (referência) | **revogado pela auditoria** | vale a referência: só cabeça, reticências `…` dentro do teto, vazio com teto < 3 (doc prompts §11.1, passo 5; T092) |
| Permissão do arquivo de página: 0644 fixo no Go; a spec não define | **confirmado pela auditoria** | a referência produz 0600 no Unix (o temporário nasce 0600 e a renomeação preserva). Agora em `docs/wiki-files-layer.md` §4. O código Go precisa mudar (T100) |
| Variante com forma dividida para tokens com pontuação (`"ai memory"` além de `"ai-memory"`) | **confirmado pela auditoria** | é comportamento de referência (doc §11.2, item 4), inclusive na degradação e com coluna conhecida. O Go ainda não implementa (T095) |
| Penalidade de sessão e peso de crença (opt-in no doc) | fora do escopo desta feature | **decidido:** ficam para outra feature |
| Sonda FTS abre banco em memória a cada consulta | desempenho | reaproveitar conexão de sonda |
| Comentário de `asciiPunct` contradiz a constante | cosmético | alinhar o comentário ao código |

## Auditoria contra a referência (2026-10-18)
> Comparação dos docs e do código Go com o comportamento do sistema original, feita fora da clean room por um auditor ("chinese wall"). Os docs foram corrigidos em regras de negócio, sem trechos do original. Aqui, o doc corrigido vence a spec e o código. As tasks estão na Fase 6 de `tasks.md`.

**Status da documentação: incompleta, com dois trechos incorretos** (lista de palavras de parada e degradação da consulta, ambos introduzidos pela revisão anterior).

| # | Tema | Doc corrigido | Go hoje | Severidade | Task |
|---|---|---|---|---|---|
| 1 | Classes `\s`/`\d`/`\b` Unicode nos padrões | prompts §11.2, notas de leitura | RE2 ASCII: `Bearer` + U+00A0 + token **não é redigido** | **alta (vazamento)** | T091 |
| 2 | Escrita de página usa o sanitizador configurado | prompts §11.1, "Páginas da wiki" | `WritePage` usa só padrões embutidos; extras e allowlist ignorados | alta | T093 |
| 3 | `pages.expires_at` gravado na escrita | SR §8 (expiração), wiki §5.2 passo 6 | coluna nunca preenchida: página vencida continua na busca | alta | T096 |
| 4 | Reforço de acesso anônimo incrementa o contador | SR §8 | ator vazio não grava nada | média | T099 |
| 5 | Lista de palavras de parada (57 reais), configurável | SR §11.2, item 2 | lista errada, fixa no código | média | T094 |
| 6 | Consulta FTS: sintaxe explícita processada por token, variante dividida, coluna sensível à caixa, degradação com variante | SR §11.2, itens 3 a 6 | sintaxe explícita vai crua para a sonda; sem variante; `TITLE:` aceito; degradação apaga `:` e `*` | média | T095 |
| 7 | `generated.at` aninhado ignorado na comparação do caso A | SR §5.1 | o armazenamento procura a chave plana `generated.at`, que não existe | média | T097 |
| 8 | Caso A sem auditoria; nomes das ações de auditoria | SR §5.1 | grava `page-unchanged`; nomes `page-*` divergentes | baixa | T098 |
| 9 | Título de página: sem teto e ignorando código cercado | wiki §3.3 | corta em 80 e lê `# ` dentro de bloco cercado | média | T101 |
| 10 | `ensure_portable` completo | DV §4.2 | faltam 255 bytes, `com¹`…, espaço antes da extensão (`CON .md`), `git~1`…`git~4` | média | T102 |
| 11 | Chave de colisão com minúsculas Unicode completas | DV §4.2 | conversão simples (`İ` e sigma final divergem) | baixa | T103 |
| 12 | Tags de autoridade: só pontas aparadas, bônus uma vez | SR §11.5 | remove espaços internos; soma bônus repetido | baixa | T104 |
| 13 | Sanitização: passada única, faixas de escape, cauda arredondada para fora, < 64 bytes com reticências | prompts §11.1 | laço de convergência; `ESC [` e CSI com `:` removidos inteiros; cauda arredondada para dentro; marcador com contagem | baixa | T092 |
| 14 | `journal_size_limit` 64 MiB | SR §2 | ausente | baixa | T105 |
| 15 | Schema à frente por qualquer migração desconhecida | SR §2 | só compara a maior versão | baixa | T106 |
| 16 | Arquivo de página 0600 | wiki §4 | 0644 | baixa | T100 |
| 17 | Allowlist vazia e padrão extra vazio | prompts §11.3 | recusados na construção (desvio mais seguro) | decisão do autor | T107 |
| 18 | Nomes reservados no construtor de caminho (ADR-05) | DV §4.2: não fazem parte da validação | `ParsePagePath` recusa `index.md`, `_pending/` etc. | decisão do autor | T108 |

Conferidos e corretos (sem mudança): os 27 padrões de segredo e sua ordem; RRF com k = 60 e posição a partir de 1; janela de candidatos; tabela de autoridade e tetos; aplicação do fator ao rank; casos A, B e C do upsert, incluindo colisão antes da ressurreição; escritor com fila de 1024; pragmas de abertura e pool de leitura de 4; escrita atômica com repetição no Windows; trava por página; rollback com erro de estado inconsistente; constantes do watcher (300 ms, 30 s, 5 falhas, disjuntor).

## Revisão da documentação contra a referência 3.1.1 (2026-10-18)
> Segunda passada da auditoria, focada nos docs, para que um implementador em qualquer linguagem replique o comportamento sem ler o código original. Os docs tinham sido escritos contra a versão 2.6.2; o código de referência é a **3.1.1** (83 migrações). Windows é o MVP, então o comportamento específico do Windows foi tornado explícito. Neste projeto, o doc corrigido vence a spec.

**Equívocos dos docs que levaram a specs erradas, e a correção:**

| Doc (seção) | O que o doc dizia, ou deixava em aberto | Comportamento de referência, agora no doc | O que isso causou na spec 001 |
|---|---|---|---|
| `domain-vocabulary.md` §4.1 | UUID v5 "a partir de um namespace fixo" | namespace OID da RFC 9562 e bytes UTF-8 do id nativo, sem aparar; formas de UUID aceitas na leitura | risco de chaves de sessão incompatíveis (002) |
| `domain-vocabulary.md` §4.2 | só "nomes reservados do Windows" | lista completa de `ensure_portable`; nenhuma lista de nomes de página reservados | ADR-05 pôs nomes reservados no construtor (T108); portabilidade incompleta (T102) |
| `domain-vocabulary.md` §4.3 | 23 tipos de agente | 27, com apelidos exatos e o predicado "injeta contexto no prompt" | 002 |
| `domain-vocabulary.md` §6.8 | relação desconhecida "fica como referência simples" | ignorada com aviso (contradizia `typed-edges.md`) | a 001 gravava `references` (já corrigido para a 005) |
| `domain-vocabulary.md` §6.6, §6.7 | limites sem detalhe | caracteres aceitos, `applies_to`/`enforced_by`, alvo gravado como enviado, TTL aparado e com deslocamento obrigatório | — |
| `wiki-files-layer.md` §3.1 | fechamento sem quebra final "não é frontmatter" | fechamento no fim do arquivo é aceito; regras de LF/CRLF casadas | — |
| `wiki-files-layer.md` §3.2, §3.3 | sem regra de leitura nem de emissão de YAML (LACUNA na `notes/markdown-contract.md`) | tabela de escalares, limites, ordem de chaves, aspas e casos dourados | a emissão Go foi decidida por ADR; precisa ser conferida contra os casos dourados |
| `wiki-files-layer.md` §3.4 | título sem teto definido e sem regra de código cercado (LACUNA) | sem teto para página; ignora `# ` em código cercado | ADR-10 cortou o título de página em 80 (T101) |
| `wiki-files-layer.md` §3.5, §3.6 | regras de link resumidas | algoritmo completo (decodificação `%XX`, `<…>`, imagens, `..` acima da raiz, `.MD`) | — |
| `wiki-files-layer.md` §5.2 | "garantidas as linhas do projeto"; sem regra de `tier`/`pinned` | a escrita só confere o escopo; o `tier` do pedido vence; não há herança de `pinned`; arquivo idêntico é regravado com os mesmos bytes | o plano cria escopo no WritePage; a decisão de herdar `pinned` (001/T080) e a ADR-18 (não regravar) divergem da referência |
| `wiki-files-layer.md` §5.5 (nova) | reindexação de página sem descrição | sem higienizador; `tier` padrão `semantic`; não reescreve o arquivo | — |
| `wiki-files-layer.md` §1.1 (nova) | formato do `_meta.md` ausente | formato exato e regras de leitura | 002/T083 e T085 dependem disso |
| `wiki-files-layer.md` §7 | fallback Git aparentava valer fora do Windows; sem regra de commit | fallback só no Windows; regras do commit; ledgers nunca no índice | ver Constraints |
| `wiki-files-layer.md` §8, §9 | alcance dos filtros e regra do ledger ambíguos | alcance de cada filtro; algoritmo do ledger em passos | 002/T022 |
| `wiki-files-layer.md` §4, §12 | o observador "usa o identificador do arquivo"; reparse sem consequência | o identificador não é usado; qualquer ponto de reparse (inclusive OneDrive sob demanda) é recusado | — |
| `storage-and-retrieval.md` §11.3 | sem fluxo de observações; grafo sem limite de projeto | fluxo de observações ligado por padrão; grafo só no mesmo projeto; ordem das sementes | a busca da 001 não tem o fluxo de observações |
| `storage-and-retrieval.md` §11.5 | origem do "tipo" não definida | `kind` do frontmatter, senão prefixo do caminho, senão `fact` | — |
| `storage-and-retrieval.md` §14.1 | "escrita sem escopo e sem ponteiro é recusada"; "informar só um é erro" | só ponteiro divergente recusa; projeto sozinho é aceito; rótulo `ws/proj` | 002/T020 |
| `cli.md` §2 | "diretório local conforme a plataforma" | `%LOCALAPPDATA%\ai-memory` no Windows; onde o `config.toml` é procurado; prefixo `\\?\`; variável de stop words | 002/T079 |
| `database-schema.md`, `overview.md` | estado das 76 migrações; invariante 12 "a confirmar" | 83 migrações; a numeração do original não tem a 12 | — |

Divergências da spec e do plano em relação à referência, ainda não decididas pelo autor (o código Go não foi alterado nesta revisão):

- **Criação de escopo na escrita de página:** o plano faz o `WritePage` criar o escopo; na referência a escrita só confere que ele existe.
- **Herança de `pinned`** da versão anterior quando a chave falta (001/T080): não existe na referência.
- **Não regravar arquivo idêntico** (ADR-18): a referência regrava os mesmos bytes, o que muda a data de modificação e gera evento no observador.
- **Fluxo de observações** na busca: ligado por padrão na referência e ausente da 001.

## Tasks da passada 2 (2026-10-18)
> Liga as correções da seção anterior ("Revisão da documentação contra a referência 3.1.1") e do `plan.md` (marcadas "passada 2") ao código Go atual. O Go não foi alterado nesta auditoria. As tasks estão nas Fases 6b e 6c de `tasks.md`. A coluna "Go hoje" foi conferida lendo o código, sem executá-lo.

| # | Tema | Doc corrigido | Go hoje | Severidade | Task |
|---|---|---|---|---|---|
| 1 | Leitura do frontmatter: tipos de valor (booleano, número, nulo), escalares do YAML, aliases, chaves, fechamento sem quebra final | wiki §3.1, §3.2 | todo escalar vira texto; nulo, alias e lista mista são recusados; bloco no fim do arquivo não é reconhecido | **alta** | T119, T111 |
| 2 | Emissão do frontmatter (listas na coluna da chave, aspas duplas, bloco literal) | wiki §3.3 | listas indentadas pelo encoder | média | T111 |
| 3 | Escrita de página não cria escopo; garante `_meta.md` | wiki §5.2 passo 7, SR §14.1 | `WritePage` chama `EnsureScope` | **alta** | T109 |
| 4 | Forma canônica na escrita: sem herança de fixação, `tier` do pedido, ordem das chaves OKF, validade estrita | wiki §5.2 passos 3, 5 e 6 | herda `pinned` da versão anterior | média | T109 |
| 5 | Sem cópia do banco antes de migrar | SR §2 | `snapshotIfPending` copia | média | T118 |
| 6 | Tabela de histórico de migrações `refinery_schema_history` | DB §0 | `schema_migrations` | decisão do autor | T113 |
| 7 | Reindexação a partir do disco não higieniza | wiki §5.5 | a T093 mandava higienizar (corrigida) | decisão do autor | T115 |
| 8 | Fluxo de observações na busca, ligado por padrão | SR §11.3 | ausente | desvio registrado (vai para a 002) | T117 |
| 9 | Tokens de entidade, `freq` sem expiradas, tipo da página, ordem das sementes, grafo só do projeto | SR §11.3, §11.5 | regras parciais | média | T110 |
| 10 | Watcher: órfãos, evento de diretório de qualquer tipo, `.md` exato, registro de todo evento; reindex com `tier` estrito | wiki §5.5, §8 | sem guarda de órfão | média | T116 |
| 11 | Links: decodificação percentual, destinos, relações sempre relativas à raiz | wiki §3.5 a §3.7 | sem decodificação | baixa | T111 |
| 12 | Tipos de agente (27), apelidos com comparação exata | DV §4.3 | fora da 001 (002) | — | — |
| 13 | Diretório de dados no Windows e palavras de parada configuráveis | cli §2 | parcial | baixa | T094 |
| 14 | Arquivo idêntico é regravado com os mesmos bytes (gera evento no observador, absorvido pelo caso A) | wiki §5.2 | ADR-18: não regrava | baixa | T109 |

Conferidos e corretos nesta passada, sem mudança no doc: `ensure_portable` e chave de colisão (DV §4.2); 27 padrões de segredo e pipeline (prompts §11); casos A, B e C do upsert (SR §5.1); escritor, pragmas e `journal_size_limit`; preparação segura da consulta FTS (SR §11.2); reforço de acesso (SR §8); constantes do watcher; escrita atômica no Windows (o identificador do arquivo existe, mas o observador não o usa); guarda de processo vivo e checagem de banco limpo do `reindex` (`docs/lifecycle-ops.md`).

## Dependências
- `docs/overview.md`, `domain-vocabulary.md`, `database-schema.md`, `storage-and-retrieval.md`, `wiki-files-layer.md` (fonte dos contratos).
- `docs/assets/fixtures` e `docs/test-fixtures.md` (golden files para os testes de contrato).
- `.github/memory/constitution.md` e `.github/memory/tech.md`.

## Checklist de Prontidão
- [x] Propósito claro e sem ambiguidade
- [x] Escopo delimitado (dentro/fora)
- [x] Cenários cobrem happy path e alternativos
- [x] Edge cases identificados com comportamento esperado
- [x] Constraints explícitas
- [x] Critérios de sucesso mensuráveis
- [x] Abordagem escolhida justificada
