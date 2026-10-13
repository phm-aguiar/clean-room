# plan.md: Plano de Implementação Técnica — Embeddings e busca vetorial (017)

> Plano gerado em 2026-10-13, antes de a 002 ser executada. Revisar quando a 016 terminar.

## 1. Metadados do Plano
- **Stack Tecnológico:** a da 002, sem dependência nova (`encoding/binary`, `math`, `crypto/sha256`, `net/http`).
- **Feature Fonte:** `specs/features/017-embeddings-e-busca-vetorial/spec.md`
- **Escopo:** embedders remotos, local e sintético; armazenamento; fluxos vetoriais; backfill; `embed`; status.
- **Fontes de contrato:** as listadas na spec.

## 2. Design de Contratos e Fronteiras
- **Pacote `internal/embed`:** `Embedder` (`Label`, `Model`, `DocumentModelIdentity`, `Dim`, `Embed`, `EmbedDocument`, `EmbedQuery`); `Triple{Provider, Model, Dim}`.
- **Pacote `internal/embed/minilm`:** `Tokenizer` (WordPiece do `tokenizer.json`), `Weights` (safetensors), `Encoder` (BERT 6 camadas, 12 cabeças, oculto 384, intermediário 1536), `Fetch` (download verificado).
- **Storage:** `UpsertEmbeddingTx`, `UpsertAbstractEmbeddingTx`, `RecordEmbedFailureTx`, `PagesMissingEmbedding`, fluxos `vector` e `abstract` na busca, contagens por tripla.
- **Configuração:** `embedding_provider`, `embedding_model`, `embedding_dim`, `embedding_base_url`, prefixos de consulta e de documento, `abstract_vectors`, `maintenance.embedding_backfill_interval_secs`.
- **Esquema SQL:** sem migração (tabelas já na V01).

## 3. Decisões Arquiteturais e Justificativas

- **Decisão (E-01): MiniLM em Go puro, provado por fixture.** Tokenizador, leitura de safetensors e encoder escritos em Go; vetores de referência para um conjunto de frases ficam versionados como fixture (gerados uma vez fora do CI) e o teste exige cosseno ≥ 0,999.
  - **Justificativa:** `CGO_ENABLED=0`; o doc fixa modelo, arquivos e pipeline.
  - **Alternativa Rejeitada:** ONNX Runtime via cgo (quebra o binário estático).

- **Decisão (E-02): identidade do vetor no tipo.** O `storage` só aceita gravar vetor com `Triple`; a leitura filtra pela tripla configurada; o prefixo de documento entra na identidade.
  - **Justificativa:** INV-08; `llm-client.md` §8.1.
  - **Alternativa Rejeitada:** guardar só o nome do modelo.

- **Decisão (E-03): varredura completa e inspecionável.** Leitura dos vetores do projeto, produto escalar e top-k em memória; sem índice vetorial.
  - **Justificativa:** `vector-backend-policy.md` D1 e D2.
  - **Alternativa Rejeitada:** `sqlite-vec`.

- **Decisão (E-04): padrão em melhor esforço com download em segundo plano.** No boot, a sondagem barata decide: arquivos presentes carregam (com checksum); ausentes disparam download em goroutine com `context` e o servidor sobe sem vetores.
  - **Justificativa:** `local-embeddings.md` § Comportamento padrão.
  - **Alternativa Rejeitada:** bloquear o boot no download.

- **Decisão (E-05): backfill pelo agendador da 004.** Passada no boot e laço opcional, em lotes de 100 pelo escritor; falha por página registrada sem abortar.
  - **Justificativa:** `consolidation-llm.md` §12; `docs/cli.md` §4.
  - **Alternativa Rejeitada:** gerar vetor na escrita da página (põe latência de modelo no caminho de escrita).

- **Decisão (E-06): contratos de teste.** Servidor falso por provedor remoto; embedder sintético nos testes de busca; teste de tripla diferente, de vetor corrompido e de degradação; fixture de referência do MiniLM; calibração com o modelo real só com variável de ambiente explícita.
  - **Justificativa:** nada de rede no CI.
  - **Alternativa Rejeitada:** baixar o modelo no CI.

### O que não construir nesta feature
Copilot (018), índice vetorial, curadoria por vetores (021).

## 4. Auditoria de Constituição
- [ ] Build cruzado com `CGO_ENABLED=0`, `go vet`, `go test -race`.
- [ ] INV-08 — tripla em todo vetor (E-02).
- [ ] INV-13 — suíte com `none`.
- [ ] INV-02 e INV-03 — gravação de vetores pelo escritor, em lote transacional.
- [ ] INV-11 — `models/` dentro do diretório de dados e fora do backup.
- [ ] Goroutines com `context` — download e backfill cancelados no encerramento.
- [ ] Sem `panic` no encoder; entradas inválidas viram erro.
