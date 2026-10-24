# plan.md: Plano de Implementação Técnica — Workstreams gerenciados (013)

> Plano gerado em 2026-10-13, antes de a 002 ser executada. Os contratos por harness vêm da fase 0 do `tasks.md`; revisar quando a 010 terminar.

## 1. Metadados do Plano
- **Stack Tecnológico:** a da 002, sem dependência nova. `os/exec` (processo filho sem shell), `os/signal`.
- **Feature Fonte:** `specs/features/013-workstreams-gerenciados/spec.md`
- **Escopo:** lançador `run`, workstreams, lease, ledger, contexto, importação, rotas `/workstream/*`, auto-wire, yolo e jail, comandos auxiliares.
- **Fontes de contrato:** as listadas na spec.

## 2. Design de Contratos e Fronteiras
- **Rotas:** `/workstream/runs` (POST), `/workstream/runs/{id}` (GET), `/heartbeat`, `/cancel`, `/context`, `/context/accept`, `/link`, `/finish`, `/recover/finish`, `/workstream/recent`, `/workstream/rename`, `/workstream/{id}/events` (GET), com os limites de `docs/lifecycle-event-ingestion.md` §12.
- **Ambiente do filho:** `AI_MEMORY_RUN_ID`, `AI_MEMORY_WORKSTREAM_ID`, `AI_MEMORY_HOOK_URL` (lidos pelo hook através de `internal/config`).
- **Esquema SQL:** sem migração; `workstreams`, `workstream_native_sessions`, `managed_runs`, `workstream_events` e o FTS já na V01.

## 3. Decisões Arquiteturais e Justificativas

- **Decisão (W-01): preparação em uma transação no servidor.** Expirar leases vencidos, escolher ou criar o workstream, criar a execução com lease de 90 s e atualizar seleção e diretório numa transação do escritor; índice `idx_managed_runs_one_active` garante uma ativa por workstream.
  - **Justificativa:** `docs/managed-workstreams.md` etapa 5 e "Exclusão mútua".
  - **Alternativa Rejeitada:** trava no cliente (não protege dois lançadores em máquinas diferentes).

- **Decisão (W-02): contexto e handoff na mesma transação.** A rota `/handoff` da 002 aceita o parâmetro de execução gerenciada; a reivindicação atômica do handoff e a confirmação do contexto (avanço do cursor de entrega) vão juntas; se o contexto não puder ser confirmado (já entregue, lease inativo), a transação inteira é descartada e o handoff fica aberto.
  - **Justificativa:** `docs/managed-workstreams.md` § "Entrega e confirmação" e `docs/design-handoff-offer-not-claim.md` §5 passo 8.
  - **Alternativa Rejeitada:** duas transações (perder a saída perderia o handoff).

- **Decisão (W-03): importação idempotente por identificador determinístico.** Cada evento tem id derivado da fonte; o mesmo id com identidade diferente é recusado; o cursor de origem avança por finalização; segmento sanitizado imutável gravado em `raw/` (fora da wiki, então fora da regra de mutação da wiki).
  - **Justificativa:** `docs/managed-workstreams.md` § "Eventos e importação".
  - **Alternativa Rejeitada:** importar sem id estável (reenvio duplicaria).

- **Decisão (W-04): lançador no `cli`, harness como descritor.** O lançador reaproveita os descritores e os adaptadores de transcrição da 010; cada harness declara seletores nativos, flag de yolo, se tem hook de início e de fim, e o mecanismo de entrega de contexto (hook, arquivo temporário do Crush, opção de regras do Grok).
  - **Justificativa:** `docs/managed-harness-contributions.md` (contrato para registrar um harness).
  - **Alternativa Rejeitada:** um lançador por harness.

- **Decisão (W-05): jail por reexecução.** Detecção do `ai-jail`, oferta, reexecução com a seleção exata, sem oferecer os interruptores que o doc proíbe; o lease é cancelado antes da reexecução.
  - **Justificativa:** `docs/design-yolo-safety-ai-jail.md`.
  - **Alternativa Rejeitada:** sandbox próprio (não objetivo do doc).

- **Decisão (W-06): contratos de teste.** Harness falso e servidor de teste para a ordem de lançamento, lease, heartbeat, cancelamento, degradado, adoção e importação; teste multi-sessão de W-02; script de aceitação do `managed-harness-contributions.md` por harness.
  - **Justificativa:** o doc lista a cobertura esperada.
  - **Alternativa Rejeitada:** testar com harnesses reais no CI.

### O que não construir nesta feature
Herdr, macOS, consolidação por LLM.

## 4. Auditoria de Constituição
- [ ] Build cruzado, `go vet`, `go test -race` — W-06.
- [ ] Teste multi-sessão em reivindicação — W-02.
- [ ] INV-01 — variáveis do filho e `run.require_server`/`AI_MEMORY_RUN_REQUIRE_SERVER` só em `internal/config`.
- [ ] INV-02 e INV-03 — preparação e finalização numa transação (W-01, W-03).
- [ ] INV-06 — sanitização antes de gravar ledger e segmento (W-03).
- [ ] INV-16 — handoff exatamente uma vez com o contexto gerenciado (W-02).
- [ ] Resolução de escopo e acesso por projeto nas rotas — regra de dono e caminho estrito nas mutações de run.
- [ ] Goroutine com `context` — heartbeat cancelado no fim do processo.
