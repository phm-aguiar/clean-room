# plan.md: Plano de Implementação Técnica — Provedores por assinatura (018)

> Plano gerado em 2026-10-13, antes de a 002 ser executada. Revisar quando a 017 terminar.

## 1. Metadados do Plano
- **Stack Tecnológico:** a da 016, sem dependência nova (`bufio` para SSE, `os/exec` para a recuperação do Codex).
- **Feature Fonte:** `specs/features/018-provedores-por-assinatura/spec.md`
- **Escopo:** quatro provedores de chat, embedder `copilot`, logins `auth`, repasse nos wrappers.

## 2. Design de Contratos e Fronteiras
- **`internal/llm`:** `anthropic_oauth.go` (reaproveita o protocolo do `anthropic`), `responses_stream.go` (SSE compartilhado), `openai_oauth.go`, `codex.go`, `copilot.go`, `oauth_refresh.go`.
- **`internal/embed`:** `copilot.go`.
- **`internal/cli`:** subcomandos `auth login openai-oauth|copilot`, `auth logout`, linhas no `auth status`.

## 3. Decisões Arquiteturais e Justificativas

- **Decisão (O-01): stream de Responses compartilhado.** Um leitor SSE reconstrói a resposta e aplica as regras (conclusão obrigatória, erro como 502 sintético com corpo até 1 KiB, truncamento).
  - **Justificativa:** `llm-client.md` §5.3 e §5.4.
  - **Alternativa Rejeitada:** um leitor por provedor.

- **Decisão (O-02): renovação dentro do provedor, persistência no `auth.json` da 009.** O provedor recebe o caminho do arquivo e a entrada; renova com 60 s de margem; grava atômico preservando as outras chaves.
  - **Justificativa:** INV-14 e `llm-client.md` §4.3 (sem trava entre processos, como o doc).
  - **Alternativa Rejeitada:** renovação num serviço à parte.

- **Decisão (O-03): recuperação do Codex serializada e limitada.** `sync.Mutex` no processo; `exec.CommandContext` com os limites de linha, total, stderr e tempo; processo encerrado em qualquer saída.
  - **Justificativa:** `llm-client.md` §5.4.
  - **Alternativa Rejeitada:** recuperação concorrente.

- **Decisão (O-04): contratos de teste.** Servidores falsos para Anthropic, Responses (SSE), troca de token do Copilot e catálogo; executável falso do Codex; relógio injetado na renovação.
  - **Justificativa:** nenhum teste toca serviço real.
  - **Alternativa Rejeitada:** testes manuais apenas.

## 4. Auditoria de Constituição
- [ ] Build cruzado, `go vet`, `go test -race`.
- [ ] INV-01 e INV-14 — variáveis e arquivos de credencial só pela configuração.
- [ ] Segurança — token GitHub cru nunca ao host do Copilot; nada de segredo em log ou status.
- [ ] Goroutines e processos com `context` e encerramento garantido.
