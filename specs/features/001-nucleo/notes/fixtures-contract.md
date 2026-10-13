# Nota: contrato de fixtures e testes

Fontes lidas: `docs/test-fixtures.md`, os 9 arquivos de `docs/assets/fixtures/`, `specs/features/001-nucleo/spec.md`, `plan.md` (P-02, P-04, P-05, P-07, P-10, §4), `tasks.md` (T002, T013, T057, T060, T067) e `.github/memory/constitution.md` (INV-06, INV-16). Todos os caminhos abaixo são relativos à raiz do repo.

## (a) Inventário de fixtures e golden files

Diretório: `docs/assets/fixtures/`. O doc `docs/test-fixtures.md` §2 diz que os fixtures são JSON neutros de linguagem. Eles ficam junto aos testes de cada componente e são lidos por mais de uma implementação. Os fixtures vêm do projeto de origem, que era em Rust, e o clean-room em Go os consome como golden files. Não há outros fixtures além destes 9. O doc não define nenhum "golden file" com esse nome.

| Caminho | Tamanho | Conteúdo |
|---|---|---|
| `docs/assets/fixtures/01-rust-bug-fix.json` | 1,8 KB | Sessão `rust-bug-fix`, 11 observações (`session-start`, `user-prompt`, `pre-tool-use`, `post-tool-use`, `session-end`). |
| `docs/assets/fixtures/02-architecture-decision.json` | 1,7 KB | Sessão `architecture-decision`, 9 observações. |
| `docs/assets/fixtures/03-gotcha-with-rule.json` | 1,7 KB | Sessão `gotcha-with-rule`, 8 observações. |
| `docs/assets/fixtures/04-low-signal-session.json` | 1,0 KB | Sessão `low-signal-session`, 7 observações. |
| `docs/assets/fixtures/05-multi-topic-session.json` | 1,8 KB | Sessão `multi-topic-session`, 10 observações. |
| `docs/assets/fixtures/capture-policy.json` | 11,9 KB | Chaves `note`, `normalization` (13 vetores), `decisions` (31 vetores), `shell` (3 chaves) e `protocol` (`accept` e `reject`). |
| `docs/assets/fixtures/home_route_cases.json` | 9,9 KB | `_comment` e `cases` (35 casos). |
| `docs/assets/fixtures/kiro-v3-hook-payloads.json` | 1,6 KB | 5 eventos de uma sessão Kiro CLI 2.16.2 sanitizada de 2026-08-07. |
| `docs/assets/fixtures/remote_identity_cases.json` | 6,6 KB | `normalize` (32), `split_name` (6), `styled_key` (6), `identity_style` (6) e `marker_aliases` (17). |

Os fixtures `01` a `05` têm a forma `{name, description, observations:[{kind,title,body}]}`.

O `plan.md` P-07 prevê também fixtures próprios do sanitizador em `internal/domain/sanitize/testdata`. A `tasks.md` T013 os nomeia como `internal/domain/sanitize/testdata/secrets.txt`. Esse arquivo ainda não existe e é criado na T013.

LACUNA: `docs/test-fixtures.md` §1 descreve um dublê de "servidor fora do ar" para o cliente de hook (aceita e fecha a conexão; fallback `http://127.0.0.1:1`). Ele pertence à feature 002 (hooks e spool) e não é usado pela 001.

LACUNA: o `docs/test-fixtures.md` não lista os fixtures `01` a `05`. A tabela de §2 cobre só 4 arquivos. O propósito deles vem apenas do campo `description` de cada JSON.

## (b) O que cada fixture valida

**Sessões `01` a `05`.** O `description` de cada uma fala em "model", ou seja, nasceram para testar a consolidação por LLM.
- `01-rust-bug-fix`: extrair a decisão e o gotcha como páginas separadas, não só um log de sessão. Conteúdo: o actor de escrita entra em pânico com canal limitado cheio e a correção é `send().await` com backpressure.
- `02-architecture-decision`: produzir uma página ADR em `decisions/`, distinta do log. Conteúdo: escritor único versus pool de escrita, com "database is locked" e SQLITE_BUSY.
- `03-gotcha-with-rule`: classificar uma regra durável como `kind=rule`, mais um gotcha. Conteúdo: "nunca mockar o banco" e diretório de dados por teste.
- `04-low-signal-session`: resistir a fabricar páginas quando não há lição durável. Conteúdo: correção de typo no README.
- `05-multi-topic-session`: gerar páginas separadas para dois tópicos sem relação. Conteúdo: imagem Docker de 380 para 67 MB e workaround do `cargo audit`/RUSTSEC.

**`capture-policy.json`** (vetores multi-linguagem de política de captura):
- `normalization`: casamento de `ignore_paths`. Cobre `..`, `//`, UNC, drive do Windows sem diferenciar maiúsculas, `?` unicode e `foo..bar`.
- `decisions`: disposição por agente (`drop`, `keep`, `metadata-only`). Cobre `tool_family`, `extraction_state` e `path_count`. Agentes citados: claude-code, codex, cursor, gemini-cli, devin, kiro-cli, grok, zero e outros.
- `shell`: vetores de comando de terminal contra `ignore_paths` (`docs/adr/**`, `~/notes/**`, `C:/Secret/**` etc.).
- `protocol`: envelope `_ai_memory_capture`. `accept` é válido. `reject` traz o campo proibido `paths: ["SENTINEL"]`.
- Descrito em `lifecycle-event-ingestion.md` §4 e `marker-file.md`. O próprio fixture afirma que "all sentinel strings are test-only".

**`home_route_cases.json`**: gramática e seleção de rotas do arquivo pessoal de marcação. Regras verificadas:
- Limites: 64 rotas, 512 bytes por seletor e por valor, 64 KiB no arquivo.
- Falha fechada: seletor repetido, campo desconhecido, arquivo malformado e `~/..` são inválidos.
- Seleção: identidade vence caminho, e entre caminhos vence o prefixo mais longo por componentes.
- Aceita caminhos POSIX, drive do Windows e UNC.
- Marcador local vence a rota da home.
- Os casos também checam flags de sub-agente, recall global, briefing e perfil.
- Tokens `{{HOME}}` e `{{LONG_513}}` são expandidos pelo harness.

**`remote_identity_cases.json`**: normalização de remote git. Exemplo: `git@github.com:Acme/API.git` vira `github.com/acme/api`. Também valida nome curto, nome em estilo caminho, estilo de identidade (`path` e `host_path`) e a gramática de aliases do marcador (máximo de 16 entradas e 128 bytes, sem `-` inicial nem separador de caminho, sem escapes). É um fixture compartilhado por todos os clientes.

**`kiro-v3-hook-payloads.json`**: cinco eventos (`session-start`, `user-prompt-submit`, `pre-tool-use`, `post-tool-use`, `stop`) com o mesmo `cwd` (`/workspace/project`) e o mesmo `session_id` (`kiro-v3-session`). O teste exigido pelo doc é que o contexto extraído seja igual em todos os eventos.

LACUNA: os fixtures de rota da home, identidade de remote, política de captura e Kiro pertencem à captura e ao marcador, que ficam na feature 002 (hooks e CLI). O spec 001 não declara quais deles a 001 usa, e só cita "fixtures de `docs/assets/fixtures`" de forma genérica.

## (c) Contrato dos testes multi-sessão (supersessão e escritor)

**Exigência da constituição** (`.github/memory/constitution.md`):
- `go test -race ./...` precisa passar.
- Toda mudança em resolução de escopo, supersessão de página, identidade de sessão, aceitação de handoff, escritor SQLite ou filtros de dono exige teste multi-sessão explícito. Teste de sessão única não basta.
- Cada invariante tem teste nomeado com seu `INV-nn`, ou um débito registrado em ADR.
- INV-16, o conteúdo testado:
  - Páginas são compartilhadas e a autoria nunca vira filtro de leitura.
  - Escrita concorrente divergente supersede, nunca destrói, e a versão perdedora continua alcançável.
  - Handoff é reivindicado exatamente uma vez, com duas guardas independentes de estado `aberto`.
  - O ponteiro de projeto ativo é chaveado por ator.
- INV-02: escritor único, goroutine dona da conexão de escrita, canal de 1024, leitura em pool somente leitura.

**Desenho do teste** (`plan.md` P-10): testes multi-sessão usam duas `Writer`/`Reader` sobre o mesmo banco temporário, com `t.Parallel` e `-race`. Nomeação: `TestINVnn_*`; os de multi-sessão são `TestINV16_*`. A alternativa "só testes unitários por função" foi rejeitada porque não exercita concorrência.

**Supersessão** (`plan.md` P-04), que o teste deve verificar em `UpsertPage`, em uma única transação:
- Caso A (conteúdo idêntico): não cria versão, só registra evidência.
- Caso B (mudou): a versão antiga recebe `is_latest=0` e `valid_to=agora`, as janelas de entidade são fechadas, e a nova versão traz `supersedes`.
- Caso C (nova): checa colisão de caminho por `PortableKey` e ressuscita tombstone, se houver.
- O índice único parcial `idx_pages_latest_path` é a rede de segurança que rejeita a corrida.
- Colunas `author_id` e `embedding_*` existem no esquema, mas a 001 não as lê nem as escreve.

**Task T060** (`tasks.md`): "Duas sessões escrevem o mesmo caminho ao mesmo tempo (multi-sessão, -race, supersessão, perdedora alcançável)". Resultado esperado do spec: uma versão é escrita e a outra supersede a anterior numa cadeia, a perdedora continua alcançável e a leitura não é filtrada por autoria.

**Escritor** (`plan.md` P-02): o teste exercita o ator de escrita genérico.
- Uma goroutine consome `chan command` de capacidade 1024. Cada comando é uma closure `func(*sql.Tx) error` com canal de resposta de uso único. Cada método público corresponde a uma transação.
- Fila cheia bloqueia o chamador até o `ctx` cancelar. O 429 não é desta feature, é do ingest na feature 002.
- O critério de sucesso do spec exige que "leitura nunca bloqueie no escritor".

**Falha no banco** (task T058): injeção de falha no Writer deve reverter o arquivo, propagar erro não ambíguo, não criar commit git e deixar o índice inalterado. Se a restauração do arquivo falhar, o erro é `ErrInconsistentState`.

**Gate** (task T067): `CGO_ENABLED=0 go build`, `go vet` e `go test -race` em `windows/amd64` e `linux/amd64`. O README (T073) menciona que `-race` precisa de gcc.

LACUNA: handoffs e ponteiro de projeto ativo por ator ficam na feature 002. A 001 testa apenas páginas compartilhadas, supersessão, escritor e escopo. O `plan.md` linha 145 diz isso explicitamente.

LACUNA: nenhuma fonte define os nomes exatos dos testes `TestINV16_*` além do prefixo. Também não definem o número de goroutines ou escritas concorrentes, nem o formato de comparação (golden) dos resultados.

LACUNA: o `docs/test-fixtures.md` §3 só recomenda um binário de integração por componente, testes adversariais de fronteira de segurança (tentar a violação, afirmar a recusa, incluir um controle legítimo e provar que falha sem a guarda) e repositórios git de fixture sem herdar a configuração da máquina (assinatura desligada, nome e e-mail fixos). Esse último ponto vale para os testes de commit git da wiki. Não há uma convenção própria para golden files.

## (d) Fixtures da busca RRF e da sanitização de segredos

**Busca RRF** (spec: critério "Busca RRF retorna os resultados esperados nas fixtures de `docs/assets/fixtures`"; task T057 em `test/acceptance/search_rrf_test.go`, depende de T037, T046, T002 e T055):
- Candidatas naturais: as sessões `01` a `05`. São o único conjunto com conteúdo textual de domínio (writer, SQLite, Docker, audit) que serve de corpus para FTS5, entidades e vizinhos de link. O `04` serve de controle de pouco sinal e o `05` de separação de tópicos.
- Contrato do algoritmo (P-05): `score = Σ 1/(60+pos)` sobre FTS5, entidades e grafo, com fator de autoridade em `[0,55; 1,50]`. Se nenhuma página compilada casar, devolve `raw_hits` pelo `observations_fts`. Benchmark `BenchmarkSearch5k` com limite de 100 ms para 5 mil páginas.
- LACUNA: os fixtures não trazem resultados esperados de ranking (sem campo `expected`). Eles são entradas de observações; o ranking esperado precisa ser definido pelos testes da 001. As fixtures `01` a `05` não têm campos de página compilada, entidades nem links.
- Os fixtures `capture-policy`, `home_route_cases`, `remote_identity_cases` e `kiro-v3` não servem à busca RRF.

**Sanitização de segredos** (spec: "Nenhum segredo cru persistido nas fixtures de sanitização"; INV-06; T013):
- Fixtures próprios, ainda a criar: `internal/domain/sanitize/testdata/secrets.txt`. O spec, em "Débitos relevantes", registra que `docs-adiados/` está ausente e que os padrões serão reescritos a partir dos docs e de fixtures próprias.
- Padrões previstos em P-07: chaves de API de provedores comuns, tokens Bearer, JWT, chaves privadas PEM, `password=`/`secret=`/`token=`, URLs com credenciais. Em RE2. O segredo é removido antes de limitar (título 80, corpo 16 KiB) e o corte é em fronteira de rune.
- Dos fixtures existentes, nenhum contém segredo real; os `grep` por `sk-`, `Bearer`, `BEGIN`, `password`, `ghp_`, `AKIA` e `eyJ` só acharam ocorrências irrelevantes no texto (por exemplo "BEGIN IMMEDIATE" no `02`). O que mais se aproxima do tema:
  - `capture-policy.json` usa sentinelas de teste (`SENTINEL`, `secret/a`). `secret/**` são nomes de diretório, não credenciais, e servem à política de captura (`ignore_paths`), não ao sanitizador.
  - `remote_identity_cases.json` inclui casos com credenciais em URL (`https://user:secret@github.com/acme/api` sai como `github.com/acme/api`). Isso testa remoção de credenciais na normalização de remote, e pode servir de caso auxiliar para o padrão de "URLs com credenciais", mas a normalização de remote em si pertence à feature 002.
- Nenhum fixture existente é dedicado à sanitização de segredos. LACUNA: os casos positivos e negativos do sanitizador precisam ser escritos na T013, incluindo um caso que prove a remoção antes do truncamento.

## Resumo para o Analyst
- Existem 5 fixtures de sessão (`01` a `05`), usados para ingestão e busca RRF, e 4 fixtures de contrato do cliente (política de captura, rotas da home, identidade de remote, Kiro). Os 4 últimos são da feature 002.
- Não há golden files de saída esperada de ranking nem de sanitização. Ambos precisam ser criados pela 001.
- O contrato multi-sessão é: duas `Writer`/`Reader` sobre o mesmo SQLite temporário, `t.Parallel`, `-race`, testes `TestINV16_*`. Verifica-se que a versão perdedora fica alcançável, que `is_latest` e `valid_to` ficam coerentes e que a leitura não filtra por autor.
