# plan.md: Plano de Implementação Técnica — Cliente LLM e provedores por chave (016)

> Plano gerado em 2026-10-13, antes de a 002 ser executada. Revisar quando a 015 terminar.

## 1. Metadados do Plano
- **Stack Tecnológico:** a da 002, sem dependência nova (`net/http`, `encoding/json`, `reflect`; UUIDv7 de `github.com/google/uuid`).
- **Feature Fonte:** `specs/features/016-cliente-llm/spec.md`
- **Escopo:** interfaces, erros, cabeçalhos, esforço, provedores por chave, contingência, saúde, `llm-test`.
- **Fontes de contrato:** `docs/llm-client.md`, `llm-providers.md`, `llm-provider-fallback.md`.

## 2. Design de Contratos e Fronteiras
- **Pacote `internal/llm`:** `Provider` (nome, modelo, `Complete`, `CompleteStructured`, estado de candidatos), `Typed[T]` (deriva o esquema e desserializa), `OperationID`, `Error` (categoria, classe, status, transitório).
- **Camadas:** `llm` depende só de `config` e `domain`; `config` entrega `LLMSettings` com credenciais já resolvidas (INV-14); o `cli` monta o provedor no boot e o expõe aos consumidores das features 019 a 022.
- **Status:** campos `llm` e `llm_candidates` no `/admin/status` (rota da 002).

## 3. Decisões Arquiteturais e Justificativas

- **Decisão (C-01): esquema derivado de tipos Go por reflexão própria.** Um gerador pequeno cobre o que os esquemas do produto usam (objeto, lista, texto, número, inteiro, booleano, enum por tag, campo opcional como anulável); a normalização estrita do OpenAI e a preparação do Gemini são transformações sobre esse esquema.
  - **Justificativa:** `llm-client.md` §2.1 e §5; evita dependência.
  - **Alternativa Rejeitada:** biblioteca de JSON Schema.

- **Decisão (C-02): um arquivo por provedor, sem camada genérica.** Cada provedor lê a resposta em structs nomeadas; divergência de protocolo vira erro de forma inesperada.
  - **Justificativa:** `llm-client.md` §1, princípio 1.
  - **Alternativa Rejeitada:** cliente "OpenAI-like" único com adaptações.

- **Decisão (C-03): classificação de erro centralizada.** Uma função decide transitório ou determinístico a partir da categoria e do status; a cadeia e os consumidores (retentativas da 020) usam a mesma função.
  - **Justificativa:** `llm-client.md` §3.2; `llm-provider-fallback.md` § Classificação.
  - **Alternativa Rejeitada:** cada consumidor classificar por conta própria.

- **Decisão (C-04): cadeia como um `Provider`.** A cadeia implementa a mesma interface, expõe o nome e o modelo do primário, guarda circuitos por posição em memória e só é montada pelo `serve`.
  - **Justificativa:** `llm-provider-fallback.md` § Montagem.
  - **Alternativa Rejeitada:** contingência dentro de cada consumidor.

- **Decisão (C-05): saúde passiva como decorador.** Envolve o provedor em uso e registra cada chamada real, sem sondagem.
  - **Justificativa:** `llm-client.md` §7.
  - **Alternativa Rejeitada:** checagem periódica (gera tráfego e custo).

- **Decisão (C-06): contratos de teste.** Servidor HTTP falso por provedor (corpo enviado, cabeçalhos, esquema normalizado, resposta); testes de tabela da classificação, dos cabeçalhos e do esforço; teste da cadeia com relógio injetado para o circuito.
  - **Justificativa:** nenhum teste chama provedor real.
  - **Alternativa Rejeitada:** testes contra APIs reais no CI.

### O que não construir nesta feature
Provedores por login (018), embeddings (017), reranker e consumidores (019 a 022).

## 4. Auditoria de Constituição
- [ ] Build cruzado, `go vet`, `go test -race`.
- [ ] INV-01 — `llm_*`, `llm_fallbacks` e variáveis de credencial só em `internal/config`.
- [ ] INV-07 — toda saída estruturada por JSON Schema (C-01).
- [ ] INV-13 — sem provedor, nenhum caminho muda.
- [ ] INV-14 — credencial resolvida antes da construção; o cliente não lê o ambiente.
- [ ] Segurança — segredos redigidos em `String()`/`GoString()` e nunca em log.
- [ ] Sem `panic`; erros com `%w` e classe estável.
