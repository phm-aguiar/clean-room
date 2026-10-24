# Spec: Embeddings e busca vetorial

## Metadados
- **ID:** 017
- **Status:** draft (revisada contra os docs em 2026-10-13; nova revisão prevista quando a 016 terminar)
- **Autor:** phm-aguiar
- **Data:** 2026-10-13
- **Feature Anterior:** 016 (erros, HTTP limitado, saúde passiva), 004 (agendador de manutenção), 002 (busca híbrida, status)
- **Roadmap:** `specs/features/roadmap.md`
- **Aprovado:** false

## Propósito
> Para quem, o quê e por quê.

A busca híbrida da 001 tem FTS, entidades e grafo. Com vetores, ela encontra paráfrases ("como fazemos deploy" acha "procedimento de release"). O padrão do produto é gerar os vetores **dentro do processo**, sem chave nem servidor auxiliar, com um modelo fixado por checksum; provedores remotos são opcionais. Falha de embedding nunca falha a busca.

## Escopo

### Dentro do escopo
- **Interface do embedder** (`docs/llm-client.md` §8.1): rótulo, nome do modelo, identidade do modelo para documentos, dimensão, `embed`/`embed_document`/`embed_query`, vetor unitário. Prefixo de documento não vazio muda a identidade (`+dp1-` e 16 hex do SHA-256 do prefixo); prefixo de consulta não muda.
- **Embedders remotos** (§8.2 a §8.4, `docs/llm-providers.md` §7): `openai`, `voyage`, `google`/`gemini`, `openai-compat`, com modelos e dimensões padrão, credenciais (`EMBEDDING_API_KEY`, `OPENAI_API_KEY`, `LLM_API_KEY` nos casos do doc, `VOYAGE_API_KEY`, `GEMINI_API_KEY`/`GOOGLE_API_KEY`), timeout de 120 s, retentativa só para 429 (2, 4, 8, 16, 32 s), prefixos só em `openai` e `openai-compat`, truncamento (`min(max_tokens × 3, 8000)` bytes), parse do formato OpenAI, dimensão divergente como forma inesperada, normalização L2. `copilot` fica na 018.
- **Embedder local** (`docs/local-embeddings.md`, `llm-client.md` §8.6): `all-MiniLM-L6-v2`, 384 dimensões, três arquivos com SHA-256 fixado (`model.safetensors`, `tokenizer.json`, `config.json`), diretório `<data_dir>/models/all-MiniLM-L6-v2/`, download do hub público (30 s de conexão, 600 s por arquivo, temporário e renomeação, checksum antes de instalar), sondagem barata por existência no boot, checksum no carregamento, pipeline (corte a 6144 bytes, tokenização com tokens especiais e sem preenchimento, 512 tokens, máscara real, média ponderada, L2, erro em norma zero ou não finita, inferência fora da goroutine de requisição), teste de calibração opt-in (0,15 a 0,45).
- **Padrão e degradação** (`local-embeddings.md` § Comportamento padrão, `llm-client.md` §8.5): sem `embedding_provider`, local em melhor esforço (download em segundo plano no primeiro boot, busca sem vetores até o boot seguinte, aviso e continuação se falhar); `none`/`off`/`disabled` desligam; provedor explícito que falha interrompe a inicialização; `local` explícito baixa antes de iniciar e falha se não conseguir.
- **Embedder sintético** (§8.7) só para testes.
- **Armazenamento** (`docs/vector-backend-policy.md` § Armazenamento, `docs/database-schema.md` §4): `page_embeddings` e `page_abstract_embeddings` (float32 little-endian, `dim × 4` bytes), `page_embed_failures` (`failed`, `unreadable`, `skipped_empty`, só a última falha). Sem extensão vetorial (decisões D1 e D2).
- **Busca vetorial** (`docs/storage-and-retrieval.md` §11.3, `vector-backend-policy.md` § Busca vetorial): fluxos `vector` e `abstract` (opt-in `abstract_vectors`), varredura completa por projeto e tripla, produto escalar, top-k na janela de candidatos, vetor da consulta com degradação para FTS, entidade e grafo; linha corrompida devolve erro da consulta (comportamento do doc); `streams_active` com `vector` e `abstract`.
- **Backfill** (`docs/consolidation-llm.md` §12, `local-embeddings.md` § Convivência): no boot e periódico (`maintenance.embedding_backfill_interval_secs`, padrão 0), lotes de 100, resumo em tabela própria, `reembed`, `dry_run`, contadores (`embedded`, `skipped`, `failed`, `would_embed`, `abstracts_embedded`), aviso no boot quando há outras triplas; `ai-memory embed [--force] [--project] [--dry-run]` como cliente HTTP de `POST /admin/embed` (`docs/api-reference.md` §3.4, 503 sem embedder).
- **Status e saúde**: páginas sem embedding (com corpo e vazias), linhas por tripla, papel `embedding` na saúde passiva da 016.

### Fora do escopo (explicitamente)
- `copilot` como embedder (018).
- `sqlite-vec` ou qualquer índice vetorial (adiado pelo próprio doc até os critérios de adoção).
- Consumidores que usam vetores em curadoria (dedup, banda, dream: 021).

## Comportamento Esperado

### Cenário Principal (Happy Path)
1. Sem configuração de embedding, o primeiro boot baixa o modelo local em segundo plano e segue só com texto, entidade e grafo.
2. No boot seguinte, o modelo carrega, o backfill gera os vetores que faltam e `memory_query` com `explain` mostra `vector` em `streams_active`.

### Cenários Alternativos
- **Checksum errado no download:** recusado, nada instalado; aviso no modo padrão; falha no modo `local` explícito.
- **Provedor remoto devolvendo 429 seis vezes:** erro de provedor depois de 62 s de espera; a página entra em `page_embed_failures`.
- **Falha ao gerar o vetor da consulta:** aviso no log e busca sem o fluxo vetorial.
- **`embedding_provider = "none"`:** sem vetores; `POST /admin/embed` responde 503.

## Edge Cases
- Troca de provedor: vetores da tripla antiga ficam guardados e ignorados; voltar reativa-os.
- Mudança de prefixo de documento: nova identidade de modelo, vetores antigos obsoletos.
- Texto que tokeniza para lista vazia no modelo local: erro daquela página, não do backfill.
- Arquivo do modelo presente mas corrompido: não é baixado de novo; aviso ou falha conforme o modo.

## Constraints
- **INV-08** (tripla junto de todo vetor), **INV-13**, binário `CGO_ENABLED=0`.
- **Tecnologia:** inferência do MiniLM em Go puro (leitura de safetensors, tokenizador WordPiece a partir do `tokenizer.json`, encoder BERT de 6 camadas). Sem dependência nova, a menos que a T002 encontre biblioteca pure Go madura (decisão registrada em `tech.md`).
- **Desempenho:** inferência em CPU, uma página por vez; a T002 mede o custo por página e a busca registra o tempo do vetor da consulta.

## Critérios de Sucesso
- [ ] Embedder local reproduz, para frases de referência, vetores com cosseno ≥ 0,999 contra vetores de referência gerados fora do CI (fixture versionada) e passa a calibração opt-in
- [ ] Cada embedder remoto testado contra servidor falso (retentativa, truncamento, prefixos, dimensão)
- [ ] Busca com tripla diferente ignorada e com vetor corrompido devolvendo erro, por teste
- [ ] Suíte inteira passa com `embedding_provider = "none"` e sem rede
- [ ] `scripts/check.sh` e o lint passam sem apontamentos

## Abordagem Escolhida
Pacote `internal/embed` (interface, remotos, sintético) e `internal/embed/minilm` (tokenizador, pesos, encoder, download). O `storage` grava e lê vetores por tripla e ganha os fluxos vetoriais; o `cli` cuida do backfill, do `embed` e de `/admin/embed`.

## Decisões tomadas
- **Inferência local em Go puro (decisão nossa de implementação):** o doc define o modelo, os arquivos e o pipeline, não a tecnologia. Como o binário é `CGO_ENABLED=0`, o encoder é escrito em Go; a fidelidade é provada contra vetores de referência.

## Dependências
- 016, 004, 002; `docs/llm-client.md` §8, `local-embeddings.md`, `vector-backend-policy.md`, `consolidation-llm.md` §12, `llm-providers.md` §7; `docs/storage-and-retrieval.md` §11.3, `docs/api-reference.md` §3.4, `docs/backup.md` (`models/` fora do backup).

## Checklist de Prontidão
- [x] Propósito claro e sem ambiguidade
- [x] Escopo delimitado (dentro/fora)
- [x] Cenários cobrem happy path e alternativos
- [x] Edge cases identificados com comportamento esperado
- [x] Constraints explícitas
- [x] Critérios de sucesso mensuráveis
- [x] Abordagem escolhida justificada

## Correções da auditoria

Auditoria de 2026-10-20/22 contra o comportamento original (relatório `reports/004-auditoria-016-017-018.md`).

- **Retentativa dos embeddings remotos:** `openai` e `openai-compat` repetem **429 e 503**, respeitam `Retry-After` inteiro (máx. 32 s) e têm orçamento de 62 s; Google e Copilot repetem só 429 (2, 4, 8, 16, 32 s); Voyage nunca. O cenário "seis 429 = 62 s" vale sem `Retry-After`.
- **Identidade Google:** o nome gravado inclui `models/`. Modelos `embedding-2` não enviam `taskType` e reformatam o texto (`title: none | text: ...` / `task: search result | query: ...`).
- **Prefixos:** `embed` simples não aplica prefixo; exemplos exatos (E5, Nemotron, e5-mistral, Qwen3 sem espaço final); variável de ambiente vazia apaga o prefixo do arquivo.
- **Parse de resposta OpenAI:** ordem e textos de erro literais (`llm-client.md` §8.4). Voyage: corpo `input` é lista de um item.
- **Embedder sintético:** FNV-1a 64 bits, minúsculas só ASCII, `hash mod dim`.
- **Local:** URLs/arquivos, nome do temporário, mensagens, download segue redirecionamento entre hosts, inferência detalhada, `degenerate embedding norm`, `empty tokenization`.
- **Escrita:** com embedder ativo, **toda escrita de página** (inclusive reindexação por arquivo editado fora e restauração) embute corpo e resumo na hora; falha só registra (`page_embed_failures`). Isso faltava no escopo.
- **Inicialização:** local padrão sem arquivos baixa em segundo plano e segue sem embedder; local explícito baixa antes e falha se não conseguir; divergência de tripla só avisa.
- **Backfill:** cada página só é lida se faltar vetor; resumo `abstract` sem registro de falha própria; `POST /admin/embed` **não** devolve `abstracts_embedded` (campos: `embedded`, `skipped`, `failed`, `would_embed`, `provider`, `model`, `dim`); `--force` sem `--project` varre todos os projetos do workspace; saída com `failed > 0` gera exit != 0 (`N page(s) failed to embed (E embedded, S skipped); run `ai-memory embed` again to retry them`); `--dry-run` imprime `dry-run: would embed N page(s), S already up-to-date`; o purge do `reembed` só apaga `page_embeddings`.
- **Status:** `latest_pages_missing_embeddings`, `latest_pages_unembeddable`, `embed_failures_unresolved/recovered`, tripla por contagem; detalhe truncado em 500 caracteres.
- Valores de configuração: provedores comparados exatamente; mensagens de erro literais em `llm-client.md` §8.2.

### Status
**Precisa de ajustes**: incorporar os itens acima (principalmente retentativa 503/Retry-After, embedding na escrita e o relatório sem `abstracts_embedded`). Pendências: valores de `config.json` do MiniLM (conferir contra o arquivo fixado) e a fidelidade numérica do encoder em Go (fixture de referência).
