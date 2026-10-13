---
graph-operators: enabled
heartbeat-threshold: 4
max-rounds: 40
---

# tasks.md: Embeddings e busca vetorial

> Feature `017-embeddings-e-busca-vetorial` · Spec: `spec.md` (draft) · Plano: `plan.md` (E-01 a E-06)
>
> **Provisório:** gerado em 2026-10-13, antes da execução das features anteriores. A T001 reconfere tudo contra o código entregue antes de qualquer execução. Sem dependência nova no `go.mod`.


## Fase 0: Descoberta

- [ ] **T001:** Reconferir este DAG contra o código entregue (busca híbrida, agendador da 004, saúde da 016) e consolidar as notas da fase 0
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/017-embeddings-e-busca-vetorial/notes/phase0-findings.md`
  - **Wiki-Keywords:** busca híbrida entregue, agendador, 016

- [ ] **T002:** Extrair a arquitetura do MiniLM (config, nomes dos tensores, tokenizador), gerar os vetores de referência da fixture fora do CI e medir o custo por página em Go
  - **Papel:** researcher
  - **agent:** researcher1
  - **Dependências:** Nenhuma
  - **depends_on:** []
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/017-embeddings-e-busca-vetorial/notes/minilm-contract.md`
  - **Wiki-Keywords:** MiniLM, safetensors, WordPiece, BERT


## Fase 1: Fundação

- [ ] **T003:** Criar o arquivo .feature da 017 com os cenários da spec
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001, T002
  - **depends_on:** [T001, T002]
  - **Paralelizável:** true
  - **Arquivos:** `specs/features/017-embeddings-e-busca-vetorial/acceptance/embeddings.feature`
  - **Wiki-Keywords:** Gherkin, local, vetor, degradação

- [ ] **T004:** Acrescentar à configuração `embedding_*`, prefixos, `abstract_vectors` e `maintenance.embedding_backfill_interval_secs`, com validação e teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/config/config.go`, `internal/config/config_test.go`
  - **Wiki-Keywords:** embedding_*, prefixos, abstract_vectors, backfill

- [ ] **T005:** Versionar a fixture de vetores de referência do MiniLM
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T002
  - **depends_on:** [T002]
  - **Paralelizável:** true
  - **Arquivos:** `test/fixtures/minilm/reference_vectors.json`
  - **Wiki-Keywords:** fixture, vetores de referência


## Fase 2: Implementação

### 2A Embedders

- [ ] **T006:** Criar a interface, a tripla, a identidade com prefixo de documento e o embedder sintético, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T001
  - **depends_on:** [T001]
  - **Paralelizável:** true
  - **Arquivos:** `internal/embed/embedder.go`, `internal/embed/embedder_test.go`, `internal/embed/synthetic.go`
  - **Wiki-Keywords:** Embedder, tripla, identidade, sintético

- [ ] **T007:** Criar os embedders remotos com retentativa, truncamento, prefixos e parse, com teste contra servidor falso
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T006
  - **depends_on:** [T006]
  - **Paralelizável:** false
  - **Arquivos:** `internal/embed/remote.go`, `internal/embed/remote_test.go`
  - **Wiki-Keywords:** openai, voyage, google, openai-compat, 429, truncamento

- [ ] **T008:** Criar o tokenizador WordPiece a partir do `tokenizer.json`, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T002
  - **depends_on:** [T002]
  - **Paralelizável:** true
  - **Arquivos:** `internal/embed/minilm/tokenizer.go`, `internal/embed/minilm/tokenizer_test.go`
  - **Wiki-Keywords:** WordPiece, tokenizer.json, 512

- [ ] **T009:** Criar a leitura dos pesos em safetensors, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T002
  - **depends_on:** [T002]
  - **Paralelizável:** true
  - **Arquivos:** `internal/embed/minilm/weights.go`, `internal/embed/minilm/weights_test.go`
  - **Wiki-Keywords:** safetensors, float32

- [ ] **T010:** Criar o encoder com média ponderada e normalização, com teste contra a fixture
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T008, T009, T005
  - **depends_on:** [T008, T009, T005]
  - **Paralelizável:** false
  - **Arquivos:** `internal/embed/minilm/encoder.go`, `internal/embed/minilm/encoder_test.go`
  - **Wiki-Keywords:** BERT, média ponderada, L2, fixture

- [ ] **T011:** Criar o download verificado e a sondagem de presença, com teste contra servidor falso
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T006
  - **depends_on:** [T006]
  - **Paralelizável:** true
  - **Arquivos:** `internal/embed/minilm/fetch.go`, `internal/embed/minilm/fetch_test.go`
  - **Wiki-Keywords:** download, sha256 fixado, temporário

- [ ] **T012:** Criar a escolha do embedder (padrão em melhor esforço, `none`, explícito fatal), com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T010, T011, T007, T004
  - **depends_on:** [T010, T011, T007, T004]
  - **Paralelizável:** false
  - **Arquivos:** `internal/embed/build.go`, `internal/embed/build_test.go`
  - **Wiki-Keywords:** padrão local, none, explícito, degradação


### 2B Storage

- [ ] **T013:** Criar a gravação e a leitura de vetores por tripla e o registro de falhas, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T006
  - **depends_on:** [T006]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/embeddings.go`, `internal/storage/embeddings_test.go`
  - **Wiki-Keywords:** page_embeddings, abstract, falhas, float32 LE

- [ ] **T014:** Acrescentar os fluxos `vector` e `abstract` à busca híbrida (tripla, corrompido como erro), com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T013
  - **depends_on:** [T013]
  - **Paralelizável:** false
  - **Arquivos:** `internal/storage/search.go`, `internal/storage/search_test.go`
  - **Wiki-Keywords:** fluxo vector, abstract, corrompido, streams_active

- [ ] **T015:** Criar as contagens de páginas sem embedding e de linhas por tripla, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T013
  - **depends_on:** [T013]
  - **Paralelizável:** true
  - **Arquivos:** `internal/storage/embed_status.go`, `internal/storage/embed_status_test.go`
  - **Wiki-Keywords:** sem embedding, triplas


### 2C Servidor e CLI

- [ ] **T016:** Criar o backfill no boot e periódico pelo agendador, com teste
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T012, T015
  - **depends_on:** [T012, T015]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/embed_backfill.go`, `internal/cli/embed_backfill_test.go`
  - **Wiki-Keywords:** backfill, lotes de 100, boot, periódico

- [ ] **T017:** Criar `POST /admin/embed` e o comando `embed`, com teste
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T016
  - **depends_on:** [T016]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/admin_embed.go`, `internal/cli/admin_embed_test.go`, `internal/cli/embed_cmd.go`, `internal/cli/embed_cmd_test.go`
  - **Wiki-Keywords:** /admin/embed, embed --force, 503

- [ ] **T018:** Gerar o vetor da consulta no `memory_query` com degradação para FTS, entidade e grafo, com teste
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T014, T012
  - **depends_on:** [T014, T012]
  - **Paralelizável:** false
  - **Arquivos:** `internal/mcp/tools_read.go`, `internal/mcp/tools_read_test.go`
  - **Wiki-Keywords:** vetor da consulta, degradação, memory_query

- [ ] **T019:** Montar o embedder no boot (download em segundo plano, aviso de triplas), o papel `embedding` na saúde, as contagens no status, a rota e o comando
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T017, T018
  - **depends_on:** [T017, T018]
  - **Paralelizável:** false
  - **Arquivos:** `internal/cli/serve_cmd.go`, `internal/cli/admin_status.go`, `internal/cli/root.go`, `internal/cli/httpserver.go`
  - **Wiki-Keywords:** boot, saúde embedding, status, rotas


### Cenários Gherkin

- [ ] **T020:** Implementar Scenario: primeiro boot sem vetores, segundo boot com backfill e fluxo vetorial (embedder sintético no lugar do download)
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T019, T003
  - **depends_on:** [T019, T003]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/embeddings_flow_test.go`
  - **Wiki-Keywords:** primeiro boot, backfill, vector

- [ ] **T021:** Implementar tratamento de erro: checksum errado, 429 esgotado, `none` e 503 no `/admin/embed`
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T019, T003
  - **depends_on:** [T019, T003]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/embeddings_errors_test.go`
  - **Wiki-Keywords:** checksum, 429, none, 503

- [ ] **T022:** Implementar edge case: troca de provedor e de prefixo com vetores antigos ignorados e reativados
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T019, T003
  - **depends_on:** [T019, T003]
  - **Paralelizável:** true
  - **Arquivos:** `test/acceptance/embeddings_triples_test.go`
  - **Wiki-Keywords:** troca de provedor, prefixo, triplas


## Fase 3: Validação

- [ ] **T023:** Rodar os portões e registrar o resultado com a saída
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T020, T021, T022
  - **depends_on:** [T020, T021, T022]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/017-embeddings-e-busca-vetorial/notes/gates-run.md`
  - **Wiki-Keywords:** check.sh, race, golangci-lint

- [ ] **T024:** Auditar a feature contra a constituição e contra os docs de embeddings
  - **Papel:** analyst
  - **agent:** analyst1
  - **Dependências:** T023
  - **depends_on:** [T023]
  - **Paralelizável:** false
  - **Arquivos:** `specs/features/017-embeddings-e-busca-vetorial/notes/constitution-audit.md`
  - **Wiki-Keywords:** constitution, auditoria, INV-08


## Fase 4: Documentação

- [ ] **T025:** Atualizar o README com embeddings, o modo local e a instalação offline do modelo
  - **Papel:** executor
  - **agent:** executor1
  - **Dependências:** T024
  - **depends_on:** [T024]
  - **Paralelizável:** true
  - **Arquivos:** `README.md`
  - **Wiki-Keywords:** README, embeddings, offline

- [ ] **T026:** Atualizar o llms.txt com a 017
  - **Papel:** executor
  - **agent:** executor2
  - **Dependências:** T025
  - **depends_on:** [T025]
  - **Paralelizável:** true
  - **Arquivos:** `llms.txt`
  - **Wiki-Keywords:** llms.txt, onboarding

- [ ] **T027:** Atualizar o .env.example com as chaves da 017
  - **Papel:** executor
  - **agent:** executor3
  - **Dependências:** T004
  - **depends_on:** [T004]
  - **Paralelizável:** true
  - **Arquivos:** `.env.example`
  - **Wiki-Keywords:** .env.example, embeddings



## Coordination Graph

G₀ (round 0):

- **nodes:** T001, T002, T003, T004, T005, T006, T007, T008, T009, T010, T011, T012, T013, T014, T015, T016, T017, T018, T019, T020, T021, T022, T023, T024, T025, T026, T027
- **edges:**
  - T001 → T003
  - T002 → T003
  - T001 → T004
  - T002 → T005
  - T001 → T006
  - T006 → T007
  - T002 → T008
  - T002 → T009
  - T008 → T010
  - T009 → T010
  - T005 → T010
  - T006 → T011
  - T010 → T012
  - T011 → T012
  - T007 → T012
  - T004 → T012
  - T006 → T013
  - T013 → T014
  - T013 → T015
  - T012 → T016
  - T015 → T016
  - T016 → T017
  - T014 → T018
  - T012 → T018
  - T017 → T019
  - T018 → T019
  - T019 → T020
  - T003 → T020
  - T019 → T021
  - T003 → T021
  - T019 → T022
  - T003 → T022
  - T020 → T023
  - T021 → T023
  - T022 → T023
  - T023 → T024
  - T024 → T025
  - T025 → T026
  - T004 → T027
- **assignments:**
  - T001: analyst1
  - T002: researcher1
  - T003: executor1
  - T004: executor2
  - T005: executor3
  - T006: executor1
  - T007: executor2
  - T008: executor3
  - T009: executor1
  - T010: executor2
  - T011: executor3
  - T012: executor1
  - T013: executor2
  - T014: executor3
  - T015: executor1
  - T016: executor2
  - T017: executor3
  - T018: executor1
  - T019: executor2
  - T020: executor3
  - T021: executor1
  - T022: executor2
  - T023: executor3
  - T024: analyst1
  - T025: executor1
  - T026: executor2
  - T027: executor3
- **ready:** [T001, T002]

> Workers: Lead, researcher1, analyst1, executor1..3. Validado por script: 27 tasks, sem ciclo, sem dependência quebrada, sem arquivo compartilhado entre tasks sem ordem. Mínimo de 12 rounds. Caminho crítico (calculado): T002 → T008 → T010 → T012 → T016 → T017 → T019 → T020 → T023 → T024 → T025 → T026.
