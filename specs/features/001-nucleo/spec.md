# Spec: Núcleo (domínio, armazenamento e wiki)

## Metadados
- **ID:** 001
- **Status:** spec auditada contra o original (relatório `reports/009-auditoria-001.md`); sem plano nem tasks; não há código de reimplementação no workspace
- **Autor:** phm-aguiar
- **Data:** 2026-10-12 (revisada em 2026-10-25)
- **Feature Anterior:** — (primeira feature)
- **Discovery Report:** — (o relatório de discovery não está mais em `reports/`)
- **Aprovado:** true

## Propósito
> Para quem, o quê e por quê.

O ai-memory-go é uma reimplementação clean-room, em Go 1.26, do ai-memory: um servidor único que dá memória de
longo prazo a agentes de código. Antes de capturar sessões ou expor MCP, é preciso o alicerce que todo o
resto consome: um domínio tipado sem I/O, um índice SQLite com escritor único e busca híbrida, e uma wiki
em markdown versionada em git, que é a fonte da verdade. O público é o autor (dev solo; Windows é o primeiro MVP).

Regra geral: em toda dúvida, o comportamento é o mais parecido possível com o original, inclusive nos
defeitos conhecidos descritos em `docs/`.

Esta feature é **zero-LLM** e entrega o núcleo testável por contratos, sem hooks nem MCP.

## Escopo

### Dentro do escopo
- **Domínio** (pacote sem I/O): UUIDv7, identidade `(workspace_id, project_id, path)`, caminho de página portável, tiers, tipos de observação, tipo sanitizado (barreira de tipo só para observações) e sanitizador.
- **Configuração** (carregada uma vez): TOML + `AI_MEMORY_*` com `__`, diretório de dados canonicalizado como no original (canonicaliza o caminho, ou o pai e reanexa o nome; se nenhum existe, fica como foi dado) e registrado no log, logger `log/slog` com filtro do próprio módulo.
- **Armazenamento**: SQLite pure-Go (WAL, FTS5, `journal_size_limit` 64 MiB), migrações `V<nn>__nome.sql` controladas pela tabela `refinery_schema_history` (`docs/database-schema.md` §0), **sem** cópia do banco antes de migrar, escritor único (goroutine + canal de 1024), pool de leitura de 4, resolução central de escopo. Escopo explícito inexistente falha fechado; leitura sem escopo usa o ponteiro ou o padrão e informa a origem.
- **Busca**: FTS5 + casamento de entidades + vizinhos de link do mesmo projeto (+ fluxo de observações, ligado por padrão, entregue junto das páginas de sessão na 002), fundidos por RRF (k = 60), multiplicador de autoridade `[0,55; 1,50]`, fallback para observações cruas e reforço de acesso throttled (1/min por página e operador, anônimo incluso e incrementando o contador). Lista das 57 palavras de parada do doc, configurável. Contrato: `docs/storage-and-retrieval.md` §11.
- **Wiki**: leitura e emissão de frontmatter pelo contrato de YAML do doc (`docs/wiki-files-layer.md` §3), escrita atômica, upsert na mesma transação com rollback do arquivo, supersessão, remoção com quarentena `.ai-memory-delete.*` (§5.4), watcher (debounce 300 ms) e reconciliação de 30 s. A escrita de página **não** faz commit git nem acrescenta linha a `log.md`: o caminho fica registrado para o próximo commit automático (002).
- **Reindex** a partir do markdown (o SQLite é derivado), sem higienizar o conteúdo lido do disco, e o primitivo de cópia online do SQLite (o backup completo é tarball, 002).

### Fora do escopo (explicitamente)
- Hooks, `/hook`, spool, sessões e handoffs (**feature 002**).
- Servidor MCP e CLI de usuário, exceto o mínimo para executar os testes (**feature 002**).
- LLM, embeddings, reranker, consolidação, lint por LLM (features 016 a 022, ver `roadmap.md`).
- Multiusuário, SSO, autorização por projeto, webhooks de admissão, perfil entre projetos (features 007, 008, 009 e 012, ver `roadmap.md`).
- Navegador web, workstreams, macOS e demais agentes.
- Penalidade de sessão e peso de crença (opt-in no doc; feature 005).

## Comportamento Esperado

### Cenário Principal (Happy Path)
1. O chamador pede a escrita da página `decisions/db.md` no projeto `demo`. O escopo já precisa existir: a escrita de página só confere que o projeto existe e pertence ao workspace, sem criar nada (`docs/wiki-files-layer.md` §5.2 passo 7; `docs/storage-and-retrieval.md` §14.1). Quem cria escopo são as operações listadas em `docs/auto-scope.md`.
2. O caminho passa pelo confinamento e, por ser caminho novo, pela portabilidade (`ensure_portable`, `docs/domain-vocabulary.md` §4.2). Não há lista de nomes de página reservados na validação. O corpo e as strings do frontmatter passam pelo sanitizador configurado (padrões embutidos, extras e allowlist do operador). O frontmatter recebe a forma canônica: `tier` do pedido, `pinned: true` se fixada (sem herança da versão anterior), chaves OKF e `generated.at`.
3. Sob a guarda compartilhada e a trava da página, garante os `_meta.md`. O arquivo é instalado de forma atômica (temporário, `fsync`, `rename`, `fsync` do diretório em melhor esforço), mesmo quando os bytes são iguais aos atuais (mesmos bytes, nova data de modificação; o evento do watcher é absorvido pelo caso A). A página e os índices FTS, entidades e links commitam em **uma** transação do escritor único. Página nova grava `pages.expires_at` quando houver TTL.
4. A resposta traz o identificador da versão e o caminho. A escrita **não** faz commit e **não** escreve em `log.md`: os caminhos ficam registrados para o próximo commit automático (`docs/wiki-files-layer.md` §5.2 e §7).
5. Uma consulta posterior retorna a página, ranqueada por RRF entre FTS, entidades e vizinhos de link, com autoridade aplicada.

### Cenários Alternativos
- **Falha no banco após instalar o arquivo:** o arquivo é revertido (ordem inversa), o erro é propagado de forma não ambígua e o índice permanece inalterado. Se a reversão também falhar, o erro é de estado inconsistente, com o texto literal de `docs/wiki-files-layer.md` §5.2 passo 9.
- **Escrita em projeto inexistente ou de outro workspace:** erro de não encontrado antes de tocar o disco; nada é criado.
- **Leitura com escopo explícito inexistente:** falha fechada com erro de não encontrado; nenhum workspace ou projeto é criado. Só workspace, sem projeto: erro de entrada (`docs/auto-scope.md`). Só projeto é aceito (procurado no workspace ativo e depois no padrão).
- **Leitura sem escopo:** nunca falha por falta de escopo; usa o ponteiro de leitura ou o projeto padrão e informa a origem (`resolved_by`, `docs/storage-and-retrieval.md` §14.1).
- **Nenhuma página compilada casa:** a consulta devolve `raw_hits` por FTS nas observações do escopo.
- **Remoção por caminho:** remove fisicamente todas as versões do caminho, com quarentena do arquivo e guarda compartilhada; com versão esperada diferente, não faz nada e informa que não removeu (`docs/storage-and-retrieval.md` §5.2; `docs/wiki-files-layer.md` §5.4).

## Edge Cases
- **Escrita concorrente no mesmo caminho:** uma versão é escrita e a outra supersede a anterior numa cadeia; a perdedora continua alcançável; a leitura não é filtrada por autoria (INV-16).
- **Edição manual da wiki no disco:** o watcher (300 ms) ou a reconciliação (30 s) reindexa. Arquivos temporários são ignorados pelo prefixo `.ai-memory-tmp.`; a escrita já instalada pelo próprio servidor é absorvida porque reindexar conteúdo idêntico não cria versão (`docs/wiki-files-layer.md` §8.1). O identificador do arquivo (inode ou índice NTFS) não participa disso. A reindexação lê o arquivo como está, sem higienizar, com `tier` padrão `semantic`. Diretório de projeto sem linha no banco é pulado inteiro. `index.md` é página comum.
- **Frontmatter escrito à mão:** é lido pelo contrato de YAML do doc (`docs/wiki-files-layer.md` §3.1 e §3.2): `pinned: true` é booleano, `yes` é texto, `007` é texto, inteiro fora de 64 bits é erro, fechamento `---` no fim do arquivo sem quebra é aceito e CRLF é aceito quando abertura e fechamento usam a mesma quebra. Frontmatter inválido impede a indexação daquela página e gera aviso, sem parar a reconciliação. Relação desconhecida em `relations` é ignorada com aviso.
- **Windows:** extensão `.md` só em minúsculas, nomes de dispositivo e caracteres proibidos recusados na escrita (`ensure_portable`), repetição da troca atômica só nos códigos 32 e 33 (acesso negado, 5, propaga na hora), diretório de dados padrão `%LOCALAPPDATA%\ai-memory` com a forma canônica `\\?\` que o original produz (`docs/cli.md` §2.5).
- **Segredo no texto de entrada:** o segredo é removido **antes** de limitar tamanho, numa passada única (título de observação ≤ 80 caracteres; corpo de observação ≤ 16 KiB; abaixo de 64 bytes, só cabeça com reticências). Página não tem teto de corpo nem de título. As classes `\s`, `\d` e `\b` dos padrões são Unicode (um `Bearer` com espaço não separável precisa ser redigido). Allowlist com entrada vazia e padrão extra que casa vazio são aceitos como no original, com as consequências descritas em `docs/prompts-schemas-and-sanitizer.md` §11.3.
- **Banco corrompido ou ausente:** `reindex` reconstrói o SQLite a partir do markdown, com guarda de processo vivo; recusa se as tabelas reconstruídas já têm linhas; manifesto ausente é erro com o caminho exato.
- **Arquivo de página:** permissão 0600 no Unix (`docs/wiki-files-layer.md` §4).

## Constraints
- **Performance:** consulta com 5 mil páginas < 100 ms; leitura nunca bloqueia no escritor.
- **Tecnologia:** Go 1.26, `modernc.org/sqlite`, `log/slog`, `CGO_ENABLED=0`, módulo único; grafo de imports da constituição. Git, igual ao original: biblioteca Git embutida como caminho normal (em Go, `github.com/go-git/go-git/v5`, registrada em `tech.md`), seguindo `docs/wiki-files-layer.md` §7 (autor fixo, ledgers fora do índice, nenhum commit sem mudança, sem herdar hooks, assinatura nem identidade do usuário). O programa `git` é só fallback, **apenas no Windows** (`docs/windows-native-extras.md` §6.2), com as regras de `notes/wiki-layout-contract.md` (d). O `regexp` (RE2) tem `\s`, `\d` e `\b` só ASCII, e os padrões de segredo exigem a semântica Unicode do original (`docs/prompts-schemas-and-sanitizer.md` §11.2).
- **Prazo:** não especificado.
- **Segurança/Compliance:** segredos nunca persistidos crus em observações; comparação de token em tempo constante; escuta padrão `127.0.0.1`.

## Critérios de Sucesso
- [ ] `CGO_ENABLED=0 go build ./...` e `go vet ./...` passam para `windows/amd64` e `linux/amd64`
- [ ] `go test -race ./...` passa, incluindo testes multi-sessão (supersessão e escritor)
- [ ] Escrever uma página produz arquivo e linhas no SQLite consistentes, sem criar escopo, sem commit git nem linha em `log.md`, e reverte o arquivo quando o banco falha
- [ ] Um arquivo escrito à mão no formato dos casos dourados de `docs/wiki-files-layer.md` §3.2 e §3.3 é lido com os mesmos valores do original e reemitido com os mesmos bytes
- [ ] Busca RRF retorna os resultados esperados nas fixtures de `docs/assets/fixtures`
- [ ] Nenhum segredo cru persistido nas fixtures de sanitização
- [ ] Invariantes INV-01..06, 09..11, 13, 15 e 16 têm teste de contrato nomeado (`TestINVnn_...`)
- [ ] Testes automatizados cobrem o happy path e os edge cases identificados

## Abordagem Escolhida
Módulo Go único com pacotes em `internal/` (convenção futura), sem CGO. O domínio não faz I/O. O armazenamento usa
`modernc.org/sqlite` e uma goroutine dona da conexão de escrita (canal de capacidade 1024, o que dá
backpressure natural). A wiki usa uma biblioteca Git embutida (`go-git`), com fallback para o programa `git` só no Windows, como o original, e a escrita atômica de arquivo.

### Alternativas Consideradas
| Abordagem | Trade-off | Por que não |
|-----------|-----------|-------------|
| `mattn/go-sqlite3` (CGO) | Mais rápido e maduro | Exige toolchain C; complica build e cross-compile no Windows |
| Só o programa `git` (`os/exec`) | Menos código | O original usa biblioteca embutida e só recorre ao `git` como fallback no Windows; exigiria `git` no PATH em todos os casos |
| `go.work` com módulos por componente | Isolamento forte | Overhead alto neste estágio; o grafo é verificável por teste de imports |
| Mutex sobre uma conexão | Mais simples | Sem backpressure nem lote de comandos |

## Correções da auditoria (2026-10-25)
> Passada "chinese wall" contra a referência 3.1.1, focada nos docs da 001. Os docs foram corrigidos em regras e tabelas, sem código. Relatório: `reports/009-auditoria-001.md`. Neste projeto o doc vence a spec. Todas as decisões que antes estavam em aberto foram **resolvidas: reproduzir o original** (sem criação de escopo na escrita, sem herança de `pinned`, regravar arquivo idêntico, `refinery_schema_history`, reindex sem higienizar, sem nomes reservados na validação de caminho, allowlist/extras vazios aceitos, fluxo de observações ligado por padrão).

| Doc (seção) | Dizia, ou deixava em aberto | Referência 3.1.1, agora no doc | Efeito na 001 |
|---|---|---|---|
| `design-decisions.md` §1 item 6 | a abertura "tira um snapshot dos dados antes de migrar" | sem cópia do banco; só o backup do diretório de dados quando a wiki não é OKF e não há recibo | sem cópia antes de migrar |
| `design-decisions.md` §1 item 3, §3 | `index.md` e reversão do arquivo "a confirmar" | `index.md` só pela migração OKF; reversão conferida, com texto literal do estado inconsistente | — |
| `architecture.md` §2 (fluxo) | "cada escrita da wiki acrescenta linha ao log" | só eventos de hook vão ao ledger, sempre no arquivo mensal `log-AAAA-MM.md` (UTC); `log.md` só é reconhecido | sem log na escrita |
| `wiki-files-layer.md` §9 | sem regra de quem escreve o ledger | arquivo mensal, formato da linha, 12 eventos, título com quebras trocadas por espaço e cortado em 120 caracteres | 002 |
| `wiki-files-layer.md` §5.1, §5.4 | quarentena só na remoção por decaimento; "remoções usam a guarda exclusiva" | as três remoções usam quarentena `.ai-memory-delete.*`; remoção simples usa guarda **compartilhada**; ordem completa do fluxo | remoção com quarentena |
| `wiki-files-layer.md` §5.2 passo 9 | só "erro de estado inconsistente" | texto literal, ordem inversa da reversão, leitura prévia do conteúdo | mensagem de erro da 001 |
| `wiki-files-layer.md` §4 | só códigos 32 e 33 | acesso negado (5) **não** repete; mesmo temporário reaproveitado; código 32 no Unix não repete | escrita atômica |
| `storage-and-retrieval.md` §11.3 | descritor e tokenização de entidades resumidos | algoritmo do descritor (600 de entrada, 240 de saída, linhas puladas); tokenização exata das entidades | busca |
| `storage-and-retrieval.md` §12 | `_slots/` e `working` "a confirmar"; TTL sem detalhe | slots só isentos por fixação; `working` sem regra de fim de sessão; TTL remove de vez, sem lápide | — |
| `storage-and-retrieval.md` §19 | só categorias | textos literais dos erros do núcleo | mensagens da 001 |
| `database-schema.md` §0, §10 | colunas do histórico sem tipo | tipos de `refinery_schema_history`; reconstruções 78, 80, 81, 83 e `users` em 54 e 79; 77 e 82 | nome e tipos da tabela de migrações |
| `overview.md` invariantes 4, 6, 11 e sanitizador | "a confirmar" | cobertura do escopo por tabela; barreira de tipo só para observações (os outros textos são higienizados pelo chamador); caminho absoluto só se o caminho ou o pai existe; motor sem retrocesso com classes Unicode | INV-06 e INV-11 da 001 |
| `data-model.md` §6 | reindexação "reconstrói só" | também grava auditoria e, com embedder, embedding | — |
| `test-fixtures.md` | fixtures 01 a 05 sem descrição | 01 a 05 descritos (avaliação de LLM); 06 e 07 ausentes | 002 |

Conferidos sem mudança: esquema das tabelas, índices e gatilhos do núcleo (83 migrações), tokenizador e gatilhos FTS, `path_search`, pragmas e `journal_size_limit`, pool de leitura, casos A, B e C do upsert (incluindo carimbo de `generated.at` só com o mapeamento `generated` e escolha da lápide mais recente), janela de candidatos, constantes do fluxo de observações, caminhada relacionada (3 e 50), tipos de observação e grafias aceitas, derivação de `type` OKF, os 27 padrões de segredo e sua ordem, constantes do watcher (300 ms, 30 s, 5 falhas, disjuntor). O original não tem um tipo `PageKind`: o "tipo" da página é o `kind` livre do frontmatter, o tipo de ranking (`storage-and-retrieval.md` §11.5) ou o `type` OKF (`okf.md`), todos derivados (`notes/domain-contract.md`).

Regra "nenhum handler escreve direto na wiki": confirmada, com uma exceção (`design-decisions.md` §3): só `reset`, `uninstall --purge-data` e `restore` apagam ou trocam diretórios inteiros, com o servidor parado e a guarda de processo vivo. O rótulo `ensure_portable` é o **nome da regra** de portabilidade, não um identificador de código.

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
