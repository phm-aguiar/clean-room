# plan.md: Plano de Implementação Técnica — Navegador web e API (014)

> Plano gerado em 2026-10-13, antes de a 002 ser executada. O contrato campo a campo da API vem da fase 0 do `tasks.md`; revisar quando a 007 terminar.

## 1. Metadados do Plano
- **Stack Tecnológico:** a da 002 mais um **renderizador de markdown** (candidato: `github.com/yuin/goldmark`, pure Go, com HTML bruto desligado por padrão; a escolha final é da task que o importa, registrada em `tech.md`). `html/template`, `embed`, `crypto/sha256` (ETag) da stdlib.
- **Feature Fonte:** `specs/features/014-navegador-web-e-api/spec.md`
- **Escopo:** navegador HTML, API `/api/v1`, cache e ETag, CORS, markdown seguro, montagem e prefixo, interface customizada.
- **Fontes de contrato:** `docs/wiki-browser.md`, `docs/frontend-api.md`.

## 2. Design de Contratos e Fronteiras
- **Rotas HTML e da API:** `docs/wiki-browser.md` §2 e §3.2; campos por endpoint em `docs/frontend-api.md` §4 (extraídos na fase 0).
- **Configuração:** ligar a interface web, slug do prefixo, origens CORS, diretório da interface customizada.

## 3. Decisões Arquiteturais e Justificativas

- **Decisão (B-01): dependência de markdown com HTML bruto desligado.** O renderizador roda sem HTML bruto; uma etapa nossa sobre a árvore resultante neutraliza esquemas perigosos, aceita imagens só relativas e reescreve wikilinks com a gramática de alvo da 001.
  - **Justificativa:** `docs/wiki-browser.md` §4; a stdlib não tem renderizador.
  - **Alternativa Rejeitada:** renderizador próprio (superfície de XSS grande demais para escrever do zero).

- **Decisão (B-02): componente só de leitura no `cli`.** Handlers usam só o `Reader` e a leitura da `wiki`; a única ação de escrita (aprovar ou rejeitar propostas) fica fora desta feature.
  - **Justificativa:** `docs/wiki-browser.md` § Papel e §7.1.
  - **Alternativa Rejeitada:** reaproveitar handlers de escrita do MCP.

- **Decisão (B-03): cache por tabela e ETag da resposta inteira.** Um middleware por rota aplica a política do §3.4; a página completa calcula o SHA-256 do JSON serializado e responde 304 quando `If-None-Match` é idêntico.
  - **Justificativa:** §3.4.
  - **Alternativa Rejeitada:** ETag só do corpo (o doc inclui links e backlinks).

- **Decisão (B-04): cursor opaco versionado.** `recent` incremental usa um cursor base64 URL-safe sem preenchimento com JSON versão 1, validado inteiro (8192 caracteres, 6144 bytes, 2048 no caminho, mesmo escopo) antes de usar.
  - **Justificativa:** §3.3.
  - **Alternativa Rejeitada:** cursor com o id cru da página.

- **Decisão (B-05): contratos de teste.** Tabelas de parâmetros e de cache; suíte de XSS; escopo 403 e 404; base injetada atrás de subcaminho; CORS com e sem origem.
  - **Justificativa:** §7 do doc.
  - **Alternativa Rejeitada:** só testes de rota feliz.

### O que não construir nesta feature
`/pending` e aprovação de propostas (022), editor web (companion), diálogo de migração (pendente no doc).

## 4. Auditoria de Constituição
- [ ] Build cruzado, `go vet`, `go test -race`.
- [ ] Dependência nova só via `tech.md` — B-01.
- [ ] INV-16 — leitura sem filtro de autoria; handoffs com `redacted` conforme dono.
- [ ] Resolução de escopo — leitura nunca cria; 404 e 403 conforme o doc.
- [ ] Autorização por capacidade (007) — sessão web e bearer pelo ponto único.
- [ ] Sem `panic`; 500 genérico com detalhe só no log.
