# Auditoria de docs e specs 012 e 013 contra o ai-memory (Rust)

Data: 2026-10-19/20. Escopo: documentação e specs; **nenhum código Go foi tocado**. Plataforma-alvo: Windows (MVP).

**Status da Documentação:** Incompleta (e com trechos incorretos). Não havia alucinação de comportamento, mas faltavam dados essenciais e havia contradições com o código.

## Arquivos revisados / corrigidos

| Arquivo | O que estava errado/omitido | Correção |
|---|---|---|
| `docs/design-cross-project-profile.md` | Listas fixas só "por exemplo" (13 tags com nomes fictícios `lang-a`…); sem texto literal do resumo/bloco; página de entrada sem layout byte a byte; regras de colheita, marcas, fan-out de uma hora, pareamento de página, esquecidos, caminho/slug, `enforced_by`, flags de projeto, `apply`, trabalhador e MCP incompletas; um "to confirm" (quem envia `profile_digest`). | Reescritas 5.1–5.5, 7, 8; Apêndices A (listas), B (textos do resumo), C (bloco do `apply`). "To confirm" resolvido: só o hook do Kimi Code envia `profile_digest=0`, após o primeiro prompt. |
| `docs/cross-project-profile.md` | Nomes fictícios (`lang-*`, `tool-*`), "e outros". | Nomes reais e remissão aos apêndices. |
| `docs/session-brief.md` (novo) | O brief tinha 4 itens "a confirmar" e nenhum algoritmo. | Especificação completa: seleção, excertos L0/L1, camadas, orçamento em **bytes** (o doc dizia "caracteres"), omitidos, escape, entrega por host. |
| `docs/marker-file.md` | "a confirmar" no brief; orçamento em "caracteres"; ordem de corte desconhecida. | Resolvido e remetido a `session-brief.md`. |
| `docs/managed-workstreams.md` | Faltavam `--profile/--env/--env-file/--require-server/--true-yolo…`, harness Copilot, **menu pós-execução**, texto do pacote, segmentos, eventos de fronteira, regras de cursor, ambiente do filho, `resume` errado (padrão é o checkout atual; há `--all/--search/--limit`), registro `client-projects.json`, dossiê da sessão anterior, `claude attach`, mensagens, Windows. | Seções reescritas/acrescentadas; "a confirmar" do cancelamento resolvido (sem timeout). |
| `docs/downtime-recovery.md` (novo) | `ai-memory recover` e o diário de recuperação não existiam em nenhum doc. | Especificado. |
| `docs/api-reference.md` §6, `docs/lifecycle-event-ingestion.md` §11–12 | `context` exigia escrita (doc dizia leitura) e só vale para Crush/Grok; `cancel` idempotente; rota `recover/finish` ausente; erros e mensagens; limites (64 KiB na rota × 16 KiB no ledger). | Corrigidos. |
| `docs/harness-adapters.md`, `docs/cli.md` | Fórmula exata dos fingerprints; "a confirmar" da preparação. | Acrescentados/resolvidos. |
| `specs/012…`, `specs/013…` (spec/plan/tasks) | Ver abaixo. | Seção "Correções da auditoria" nas duas specs; T002 da 012 reescrita; tarefas A1–A9 da 013. |

## Principais equívocos encontrados
1. T002 da 012 mandava "registrar como decisão nossa" o que o doc não listava — levaria a vocabulários divergentes. Agora os docs trazem as listas.
2. Orçamento do brief em caracteres; é em bytes.
3. `enforced_by`: qualquer valor "não vazio" (inclusive `false`) aplica; o doc falava só em "não vazio".
4. `projects` do frontmatter ≠ projetos independentes da confiança.
5. Pareamento de página exclui só `profile/stack/<linguagem>.md`, não "toda página stack".
6. `profile apply --remove` independe de servidor e de `consume`.
7. 013: `POST …/context` é de escrita; só Crush/Grok; `cancel` idempotente; `resume` lista o checkout atual por padrão; existe menu pós-execução e `recover`.
8. 013/Windows: jail inexistente; executável por `PATHEXT`; `cmdc`; trava de arquivo; ACL por `icacls`.

**Status da Spec 012 (após correção):** Fiel (dependente de implementar com os apêndices; a parte LLM segue na 020).

**Status da Spec 013 (após correção):** Precisa de ajustes adicionais — as tasks A1–A9 devem ser integradas ao DAG pela T005; a leitura de transcrições por harness (adaptadores, `transcript.rs`) é da 010 e **não foi auditada linha a linha** aqui.

## Pontos que ainda precisam de atenção
- Adaptadores de transcrição por harness (`harness-adapters.md` §3 em diante) e planos de lançamento por harness: só amostrados (yolo, seletores) — coincidem; o restante é da 010.
- `finalize-session`, `show` (menus), `backfill`/`import_exact_session_range` e a quarentena do spool (usados pelo `recover`): descritos no nível do contrato, não do algoritmo interno.
- Checklist/toggles do jail: irrelevantes no Windows, não reauditados.
- `design-per-project-authz.md`, `design-handoff-offer-not-claim.md`: conferido apenas o comportamento da transação de reivindicação e a regra de dono/escrita das mutações.
- Cobertura de testes adversariais por fronteira do perfil (§9): continua "to confirm".
- O texto de `design-yolo-safety-ai-jail.md` tem vários "a confirmar" sobre o próprio ai-jail (ferramenta externa), fora do MVP Windows.

## Plano de ação
1. Revisar os diffs dos docs (principalmente os Apêndices A–C e `session-brief.md`/`downtime-recovery.md`).
2. Re-rodar a pipeline da **012** (T002 passa a ser transcrição; implementar brief conforme `session-brief.md`).
3. Rodar a fase 0 da **013** e integrar A1–A9 ao DAG (T005) antes de implementar; priorizar Windows.
4. Auditar em seguida a 010 (adaptadores) e a 003 (purga: critério de lease vivo já registrado na spec 013).
