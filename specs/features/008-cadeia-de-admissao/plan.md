# plan.md: Plano de Implementação Técnica — Cadeia de admissão (008)

> Plano gerado em 2026-10-13, antes de a 002 ser executada. Revisar quando as features anteriores terminarem.

## 1. Metadados do Plano
- **Stack Tecnológico:** a da 002, sem dependência nova. `net/http`, `encoding/json`, `io.LimitReader` (1 MiB).
- **Feature Fonte:** `specs/features/008-cadeia-de-admissao/spec.md`
- **Escopo:** configuração dos ganchos, cadeia na escrita de página, notificação em exclusão, purga e movimento, decidir-agir-notificar em handoffs, pré-voo de `consolidate` do relatório de lint, prevenção de laço, limites. Operações de mensagem ficam para a 011.
- **Fontes de contrato:** `docs/admission-webhooks.md` (fonte principal), `docs/wiki-files-layer.md` §5.2, `docs/design-handoff-offer-not-claim.md` §5 e §7, `docs/mcp-server.md` §4.4.

## 2. Design de Contratos e Fronteiras
- **Contrato de fio:** `docs/admission-webhooks.md` §3 (requisição com `X-Memory-Op`, corpo `page` e `ctx`; resposta 200, 204, 2xx ilegível, 4xx/5xx).
- **Configuração:** `[[admission_webhooks]]` e `AI_MEMORY_ADMISSION_WEBHOOKS_JSON` (sobreposição explícita, `docs/cli.md` §2.1), lidos por `internal/config`.
- **Esquema SQL:** sem migração.

### 2.1 Interface no `wiki`
```go
type Op string // os 14 valores do doc
type Chain struct{ /* ganchos, cliente HTTP, semáforo de 256 */ }
// Mutate roda os bloqueantes de uma escrita de página, em ordem; devolve a página final ou recusa.
func (c *Chain) Mutate(ctx context.Context, op Op, p Page, ac ActorCtx, skip []string) (Page, error)
// Gate roda os bloqueantes de uma operação de notificação (sem mutação).
func (c *Chain) Gate(ctx context.Context, op Op, p Page, ac ActorCtx, skip []string) error
// Decide roda só os decisores (bloqueante + reject) de handoff e mensagem.
func (c *Chain) Decide(ctx context.Context, op Op, ac ActorCtx, skip []string) error
// Notify dispara os observadores depois da operação, sem esperar.
func (c *Chain) Notify(op Op, p Page, ac ActorCtx, skip []string, partialFailure bool)
```
A cadeia é opcional: `nil` quando a lista é vazia (o doc diz que o servidor não monta a cadeia), e cada ponto de operação testa `nil` uma vez.

## 3. Decisões Arquiteturais e Justificativas

- **Decisão (A-01): cadeia no `wiki`, chamada por todos os pontos de operação.** A escrita de página é da wiki (constituição) e a cadeia faz parte dela (§2.1). Purgas, movimentos, despejos e handoffs chamam `Gate`, `Decide` e `Notify` nos pontos que o doc define. `mcp` e `ingest` chegam à cadeia pela `wiki`, nunca um pelo outro.
  - **Justificativa:** `docs/wiki-files-layer.md` §5.2 põe a cadeia dentro do fluxo de escrita; o grafo de imports proíbe `mcp` → `ingest`.
  - **Alternativa Rejeitada:** pacote `admission` novo (exigiria mudar o grafo da constituição) e cadeia no `cli` (a escrita de página não passa por lá).

- **Decisão (A-02): sequencial, um cliente, limites do doc.** Bloqueantes em ordem, cada um com o próprio `timeout_ms` (cortado em 1..30000); resposta lida até 1 MiB; não bloqueantes e observadores num semáforo não bloqueante de 256 (cheio: descarta e registra ERROR ou WARN conforme `blocking`). Configuração com mais de 16 ganchos falha a carga.
  - **Justificativa:** §7 e §11 (fan-out paralelo é não objetivo).
  - **Alternativa Rejeitada:** fila para os observadores (o doc manda descartar, não enfileirar).

- **Decisão (A-03): segunda higienização e redrivação depois da cadeia.** A página devolvida passa de novo pelo sanitizador (corpo e todo texto do frontmatter), é normalizada para o disco e tem título e links derivados de novo; um título explícito de quem chamou continua vencendo.
  - **Justificativa:** §2.1 passos 5 e 6; INV-06.
  - **Alternativa Rejeitada:** confiar na resposta do gancho.

- **Decisão (A-04): decidir, agir, notificar nos handoffs.** `handoff_begin` no fim de sessão pede a decisão antes do commit do fim de sessão e, em recusa, termina sem handoff; `handoff_accept` no início de sessão roda os decisores com prazo total de 750 ms (recusa, estouro ou host inacessível deixam o handoff aberto); as tools devolvem erro em recusa. Aceitação sem handoff pendente e cancelamento recusado por dono não são anunciados.
  - **Justificativa:** §2.4 e `docs/design-handoff-offer-not-claim.md` §5.
  - **Alternativa Rejeitada:** notificar antes de agir.

- **Decisão (A-05): pulo da cadeia só para root e anônimo.** O cabeçalho é lido pelo middleware e passa no contexto; usuário comum tem o cabeçalho ignorado (sem erro). A lista nunca vai no fio.
  - **Justificativa:** §6 e `docs/mcp-server.md` §4.4.
  - **Alternativa Rejeitada:** recusar a requisição de usuário com o cabeçalho (o doc diz para ignorar).

- **Decisão (A-06): contratos de teste.** A lista de cobertura do §10 inteira, mais a tabela de combinações do §8.1 e o prazo de 750 ms, com servidores HTTP de teste.
  - **Justificativa:** o doc lista a suíte esperada.
  - **Alternativa Rejeitada:** testar só a escrita de página.

### O que não construir nesta feature
Operações de mensagem (011), consolidação por LLM e auto-melhoria (020 e 022), registro dinâmico e métricas por gancho (não objetivos do doc).

## 4. Auditoria de Constituição

**Portões de Qualidade**
- [ ] Build cruzado, `go vet`, `go test -race` — A-06.
- [ ] Teste multi-sessão em aceitação de handoff — A-04 com dois aceites e um decisor.
- [ ] Gherkin — os cenários da spec viram o `.feature` da 008.
- [ ] Dependência nova — nenhuma.

**Restrições Arquiteturais**
- [ ] Grafo de imports — A-01 (cadeia no `wiki`).
- [ ] INV-01 — configuração e variável JSON só em `internal/config`.
- [ ] INV-05 — os caminhos de hook (`/hook`) não passam pela cadeia; anexos de ledger também não (§2.6).
- [ ] INV-06 — A-03.
- [ ] INV-16 — duas guardas de `open` continuam na reivindicação; a decisão vem antes.
- [ ] Mutações da wiki só pela camada `wiki` — A-01.

**Anti-Padrões Proibidos**
- [ ] Goroutine com `context` e fan-out limitado — semáforo de 256 (A-02).
- [ ] Sem stub — cadeia `nil` quando não há ganchos, como o doc define.
