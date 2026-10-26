# 014 — Varredura "Chinese Wall" de vazamentos

Data: 2026-10-25. Escopo: `docs/` (com `assets/` e subpastas), `specs/features/*/spec.md` e `specs/features/*/notes/`. `plan.md` e `tasks.md` não foram tocados. O código original foi usado só para conferir se um nome é interno ou se tem forma serializada (contrato).

Método: grep de caminhos `crates/`, `.rs`, nomes de crates, sintaxe Rust (`Some(`, `Self::`, `impl`, `->`, blocos ```rust```), identificadores CamelCase e snake_case entre crases. Depois, cada identificador foi cruzado com o original: nome que só existe como `fn`/variante de enum foi trocado. Nome com forma em fio (serde/`as_str`, string literal, coluna, chave de config) foi mantido ou trocado pela forma em fio.

## Trocas (arquivo:linha, antes → depois)

| Arquivo:linha | Antes | Depois |
|---|---|---|
| docs/assets/diagrams/architecture-overview.svg:68 | `one Rust binary` | `one binary` |
| docs/assets/diagrams/architecture-overview.svg:92 | `single mpsc queue, one rusqlite writer` | `single queue, one SQLite writer` |
| docs/assets/examples/jev-reranker-adapter/jev_rerank_shim_choice.py:49 | `# crates/ai-memory-llm/src/reranker.rs in the ai-memory tree.` | `# docs/prompts-schemas-and-sanitizer.md (reranker prompt).` |
| docs/cli.md:339 | disposição interna é `MetadataOnly` | disposição é só metadados |
| docs/cli.md:486 | `RootsRequired` | `rejected-roots-required` (forma em fio) |
| docs/cli.md:491 | `OutsideRoots` | `rejected-outside-roots` |
| docs/cli.md:493 | `NoToken` | `rejected-no-token` |
| docs/cli.md:517 | `` `clear_tokens` (chamado pela desinstalação) `` | A limpeza de tokens (chamada pela desinstalação) |
| docs/cli.md:582 | Escrita idempotente (`apply_atomic`) | Escrita idempotente |
| docs/cli.md:584 | disco (`NoOp`) | disco (nada a fazer) |
| docs/cli.md:785 | `get_json`, `post_json`, `post_json_no_content` | funções auxiliares de requisição (GET e POST com JSON, POST sem conteúdo de resposta) |
| docs/consolidation-llm.md:308 | em página única (`consolidate_session`) | em página única |
| docs/consolidation-llm.md:883-884 | `` `classify(texto, configuração)` devolve `Keep` ou `Skip(motivo)` ... sempre devolve `Keep` `` | A classificação de um texto (dada a configuração) decide manter ou pular, com motivo ... sempre decide manter |
| docs/consolidation-llm.md:1042 | `Basename`: | Por basename: |
| docs/consolidation-llm.md:1044 | `MainRepoRoot`: | Pela raiz do repositório principal: |
| docs/consolidation-llm.md:1051 | `NoSources`/`AlreadyBootstrapped` → 422 | nenhuma fonte coletada ou bootstrap já existente → 422 |
| docs/llm-client.md:487, 504, 508 | `embed`, `embed_document`, `embed_query` (métodos internos) | embedding genérico, de documento e de consulta (mesma regra de delegação e do Google) |
| docs/storage-and-retrieval.md:329 | (`startup_handoff`) | removido; o texto já diz "entrega automática no início de sessão" |
| docs/wiki-browser.md:927 | `split_web_routers` produz | A divisão das rotas web produz |
| docs/design-memory-aging.md:104-105 | `Helpful`, `NotHelpful`, `Stale`, `Wrong` | `helpful`, `not_helpful`, `stale`, `wrong` (valores em fio do feedback) |
| docs/design-memory-aging.md:658 | run `merge_one_cluster` | run the per-cluster merge |
| docs/complementary-contracts.md:132 | Degrau (`IdentitySource`) | Degrau (origem da identidade) |
| docs/complementary-contracts.md:177 | (`MarkerAliases`) | removido |
| docs/complementary-contracts.md:181 | `TooMany`, `Invalid`, `InvalidWire` | mensagens literais `too many marker aliases`, `invalid marker alias`, `invalid marker aliases wire value` |
| docs/complementary-contracts.md:182 | (`IdentityStyle`) | removido (os valores `host_path`/`path` continuam) |
| docs/complementary-contracts.md:250-251 | `McpConfig`, `Skill`, `Plugin`, `Instruction`; `Global`, `Project` | `mcp-config`, `skill`, `plugin`, `instruction`; `global`, `project` (kebab-case serializado) |
| docs/mcp-server.md:76 | (`Matched`, `MatchedShared` ou `StartupSeed`) | (entrada própria, slot compartilhado ou semente de inicialização) |
| docs/mcp-server.md:77 | **`Unset`** | **vazio** |
| docs/mcp-server.md:78 | em **`Mismatch`** | **divergente** (mensagem `ambiguous unscoped write` mantida) |
| docs/session-brief.md:61, 87, 91 | `TitleOnly` | title-only / Title-only tier |
| docs/frontend-api.md:588-590, 597 | `HealthPage` | páginas de saúde / Cada página de saúde (os campos continuam) |
| docs/api-reference.md:169 | `OwnerFilter` do operador | filtro de dono do operador |
| docs/design-handoff-offer-not-claim.md:103, 165, 166, 198 | `HandoffAccept` | `handoff_accept` (operação de admissão em fio) |
| docs/harness-adapters.md:33 | `Export` ... (`SpoolOnly`: | exportação de transcript ... (só spool: |
| docs/harness-adapters.md:551, 677 | `SpoolOnly` | só spool |
| docs/lifecycle-event-ingestion.md:29 | `Keep / Drop / MetadataOnly -> Drop: 202` | `keep / drop / metadata-only -> drop: 202` |
| docs/lifecycle-event-ingestion.md:259, 347-428 (todas as ocorrências) | `` `MetadataOnly` ``, `` `Keep` ``, `` `Drop` `` | `metadata-only`, `keep`, `drop` (valores de `disposition` do protocolo `_ai_memory_capture`) |
| docs/lifecycle-event-ingestion.md:365 | `MissingOrMalformed` | `missing-or-malformed` |
| docs/lifecycle-event-ingestion.md:412 | (`SearchList`) | (`search-list`, valor de `tool_family`) |
| specs/features/001-nucleo/spec.md:122 | `Explicit`, `Session`, `SharedSlot`, `StartupSeed`, `Default`, `DefaultAfterMismatch` | `explicit`, `session`, `shared_slot`, `startup_seed`, `default`, `default_after_mismatch` |
| specs/features/017-embeddings-e-busca-vetorial/spec.md:20 | `embed`/`embed_document`/`embed_query` | embedding genérico, de documento e de consulta |

O CRLF/LF de cada arquivo foi preservado: as trocas foram feitas em modo binário, com conversão de `\n` quando o arquivo usa CRLF.

## Mantidos por serem contrato (ou exceção justificada)

- **Esquemas de saída estruturada nos prompts** (`ConsolidatedPage`, `ConsolidatedPageUpdate`, `ConsolidatedBatch`, `BootstrapOutput`, `LintReport`, `AutoImproveLlmResponse`, `ClassifyResponse`, `MergeResponse`) em `docs/assets/prompts/*`, `prompts-schemas-and-sanitizer.md` e na spec 020: são nomes enviados ao provedor no texto do prompt e no schema. Contam como texto de produto.
- **Rótulo de erro `serde`** (`llm-client.md`, `llm-provider-fallback.md`, `consolidation-llm.md`, `storage-and-retrieval.md`, spec 016): valor literal de `last_error_class` e prefixo de mensagem.
- **`PerActor`, `PerSession`** (`deploy.md`, `users.md`, `auto-scope.md`): aparecem na linha de log de inicialização (`mode=PerActor`) e como valor rejeitado na config.
- **`AssistantMessage`, `ToolResults`, `ContextualHookInvoked`** (`harness-adapters.md`): tipos de registro do formato externo do Kiro v3, comparados como strings.
- **Nomes de eventos e ferramentas de harness externos** (`PreToolUse`, `SessionStart`, `TodoWrite`, `WebFetch`, `AbsolutePath`, `TargetFile` etc.): contrato dos agentes de terceiros.
- **`ai_memory_mcp_tool_calls_total`** (`mcp-server.md`): nome de métrica exposta.
- **`--client-id "ai-memory-cli"`** (`install.md`, `sso.md`, spec 009): valor de cliente OIDC visível ao usuário.
- **`server_unreachable`, `expired_at`, `home_dir`, `release_base_url`, `reconcile_tombstones_deleted_pages`**: campos JSON de relatório (`doctor`, varredura) e chaves de configuração.
- **`RUST_LOG=ai_memory=info,...`** (`docs/assets/deploy-templates/docker-compose.prod.yml.example:41` e `hook-scripts-and-container-deploy.md:264`): é o conteúdo literal do compose distribuído, e o doc já o marca como específico da implementação de referência. Mantido como registro de artefato. Recomendação: o template do produto novo deve trocar a linha.
- **`crates/ai-memory-cli/templates/config.default.toml`** (`airgapped-install.md:229`): é uma entrada da lista fixa de arquivos validados no arquivo de release, ou seja, layout observável do pacote. Mantido. A reimplementação define a própria lista.
- **`ensure_portable`** (docs e spec 001): a spec 001 (linha 248) já registrou a decisão de mantê-lo como **nome da regra** de portabilidade, porque acceptance e notas o citam. Mantido por coerência. Fica a recomendação de renomear para "validação de portabilidade" se a regra for revista.
- **Fixtures de avaliação** (`docs/assets/fixtures/01-rust-bug-fix.json`, `02-architecture-decision.json` etc.): o conteúdo simula sessões reais, com `writer.rs`, `rusqlite` e `try_send().unwrap()` como texto de observação. São dados de entrada de teste, e mudá-los alteraria a linha de base da avaliação. Mantidos.
- **Tabelas de tags e extensões em `design-cross-project-profile.md`** (`rust`, `tokio`, `serde`, `.rs`, `.go`, `build.rs`, `crates/store/src/ops.rs:600-640`): vocabulário e casos de teste da regra de classificação. São contrato de comportamento, não código do original.
- **PowerShell em `windows.md`** (`[Environment]::...`): script para o usuário rodar, não código-fonte.
- **`x::y/n`** em `https-via-proxy.md`: notação de CIDR IPv6.
- **Menções a Go em `specs/features/*/spec.md` e `notes/`** (`go build`, `go test -race`, `internal/...`, `mcp-go`, `modernc.org/sqlite`, `golang:<go.mod>`): são artefatos da própria reimplementação, como portões, auditorias e ADRs, e não vazamento do original. A regra de agnosticismo vale para `docs/`, onde não foi achada nenhuma menção a Go como linguagem de reimplementação. `.go` só aparece na tabela de extensões, que é contrato.
- **Notas com chaves de fio** (`_ai_memory_capture`, `_ai_memory_capture_policy`, `_ai_memory_assistant`, cookies `ai_memory_session`/`ai_memory_csrf`): contrato.

## Não encontrado

Nenhum bloco de código Rust, assinatura, macro (`#[...]`) ou sintaxe `Some(`/`Self::`/`impl` em `docs/` nem nas specs e notas. Não sobrou nenhum caminho `crates/...` fora das duas exceções acima.
