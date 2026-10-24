# Recuperação após queda do servidor: diário local e `ai-memory recover`

> **Escopo.** Quando o servidor fica inacessível, `ai-memory run` não perde o trabalho: os hooks gravam no spool local (ver [cli.md](cli.md) §5.3) e o lançador registra, num diário local, o histórico de transcript que o servidor não recebeu. `ai-memory recover`, executado quando o servidor volta, drena o spool e reenvia esse histórico. Este documento foi acrescentado na auditoria de 2026-10-19: a funcionalidade existe na implementação de referência e não estava documentada. O ciclo de `run` está em [managed-workstreams.md](managed-workstreams.md).

## 1. O diário (`recovery-journal.json`)

Arquivo `<diretório de dados>/recovery-journal.json`, com trava exclusiva `recovery-journal.lock` no mesmo diretório (bloqueio de arquivo do sistema; no Windows, `LockFileEx`). Toda modificação é leitura-modificação-escrita sob a trava e a escrita é atômica (arquivo temporário `.recovery-journal.*` no mesmo diretório, `fsync` dos dados, rename, `fsync` do diretório em melhor esforço). O arquivo e a trava são privados ao dono (Unix: modo 0600; Windows: ACL reduzida ao usuário atual por `icacls <arquivo> /inheritance:r /grant:r <whoami>:(F)`; falha em restringir é erro). Um vínculo simbólico no diretório de dados, na trava ou no arquivo é recusado.

```json
{ "version": 1, "entries": [ { ... } ] }
```

Limites: no máximo **256** entradas (anexar com o diário cheio é recusado com `recovery journal is full (256 entries); run `ai-memory recover` before another degraded launch`); arquivo de até **16 MiB**; versão diferente de 1, JSON inválido ou excesso de entradas são erros que **nunca** apagam o arquivo. O diário não guarda conteúdo de transcript nem segredos: só localizadores, resumos ordenados e evidências de entrega.

### Entrada

| Campo | Significado |
|---|---|
| `id` | UUID da entrada. |
| `recorded_at` | RFC 3339 UTC. |
| `interval_started_ms`, `interval_ended_ms` | Limites (inclusivos, ms desde a época Unix) dos eventos de hook que pertencem a este lançamento. |
| `kind` | `degraded-run` (lançamento sem servidor; nenhuma execução existiu no servidor) ou `finish-failed` (a execução existe no servidor, mas a finalização não o alcançou). |
| `harness` | Nome exato do harness (`claude`, `codex`, `opencode`, `opencode2`, `pi`, `crush`, `omp`, `kimi`, `command-code`, `kiro`, `kiro-v3`, `grok`, `antigravity`, `copilot`). |
| `native_session_id` | Localizador da sessão (omitido se não descoberta). |
| `session_dir` | Substituição do diretório de sessões usada no lançamento, absoluta (relativa → junta ao `cwd`). |
| `cwd`, `workspace`, `project` | Checkout canônico e escopo. |
| `server_url` | Identidade do servidor alvo (para recusar reenviar a outro servidor). |
| `run_id` | Só em `finish-failed`: alvo da reimportação. |
| `event_digests` | Resumos ordenados (SHA-256) dos eventos do intervalo exato; `[]` prova uma exportação vazia bem-sucedida; ausente = exportação indisponível. |
| `spool_only` | Verdadeiro para harness sem exportador de transcript (Antigravity): só a evidência do spool vale. |
| `source_cursor`, `final_cursor` | Cursor do adaptador antes (retomada) e logo depois da saída do processo. |
| `correlated_events_seen`, `correlated_delivery_confirmed`, `correlated_loss` | Evidência de hooks correlacionados ao lançamento (ver 3). |
| `exit_code` | Código de saída do filho, reaplicado na reimportação. |
| `failures` | Até 3 motivos de tentativas anteriores (cada um sanitizado, ≤ 2 048 bytes, sem duplicatas, os mais novos por último). |

## 2. Quando o lançador grava uma entrada

- **`degraded-run`:** ao fim de um lançamento degradado em modo de sessão (não passthrough). O lançador tira, antes do início, uma fotografia das sessões nativas do checkout (até 2 000; mais que isso invalida a fotografia). Depois da saída, a sessão usada é a esperada (id conhecido antes) ou a **única** sessão nova (`atualizada ≥ início − 2 s` e ausente da fotografia). Se não for única/identificável, nada é gravado e o aviso diz para preservar o transcript e rodar `ai-memory backfill --force --session <id>`. Para harness com exportador, é exigida uma exportação exata pós-execução (com espera de descarga do arquivo) que produza cursor final; sem ela, nada é gravado e o aviso explica. A resposta de sucesso: `ai-memory: recorded for recovery — run `ai-memory recover` once the server is back to replay this session's exact native transcript`.
- **`finish-failed`:** quando o processo já saiu, o transcript foi exportado de forma exata, e a importação falhou por erro de **transporte** (servidor inacessível/timeout; um erro HTTP respondido pelo servidor é falha dura, sem diário). O comando devolve o código do filho e imprime `ai-memory: warning: <erro>; the transcript for <harness> session <id> in <ws>/<proj> was not imported and the orphaned lease will expire automatically within 90 seconds. The harness exit code is preserved. Run `ai-memory recover` once the server is back.`, seguido de `ai-memory: recorded for recovery — … re-import this transcript`.
- Falha ao gravar o diário é aviso (o código de saída do filho prevalece) com instrução de reparo manual por `backfill`.

## 3. `ai-memory recover`

Opção `--json`. Fluxo:

1. Lê o diário (erro de leitura vira `journal_error`, mas a drenagem do spool continua) e sonda o servidor (2 s). Inacessível → relatório `reachable: false`, nada é feito, texto orientando iniciar o servidor ou corrigir `AI_MEMORY_SERVER_URL`; código de saída 0.
2. Obtém a trava de drenagem do spool com espera limitada (30 s). Ocupada → relatório `spool_lock_busy`, recuperação de transcript adiada (`spool_incomplete`).
3. Para cada entrada `degraded-run` (ou `spool_only`) com sessão conhecida, **põe em quarentena** os eventos de hook do spool correlacionados (mesmo servidor, agente, escopo, cwd, sessão e dentro de `[interval_started_ms, interval_ended_ms]`); marca `correlated_events_seen` e `correlated_loss` provisoriamente. Para harness sem exportador, drena a quarentena de forma estrita; só quando tudo foi entregue de modo durável e o spool dessa sessão está completo, marca `correlated_delivery_confirmed` e limpa `correlated_loss`.
4. Drena o restante do spool até a quiescência (orçamento total de 5 min; 30 s por evento; respeitando o máximo de tentativas configurado). Se sobrar ou for descartado algo, `spool_incomplete`: as entradas do diário **não** são processadas (falham com o motivo `hook spool recovery was incomplete (<n> queued, <m> dropped); transcript recovery was deferred`).
5. Processa cada entrada com orçamento de 60 s por entrada e 10 min no total do diário (esgotado: `journal entry recovery timed out; rerun to continue` / `total recovery budget exhausted; rerun to continue`):
   - Recusa reenviar a um servidor diferente do gravado (`refusing to replay an entry recorded for "<a>" against <b>`).
   - `spool_only`: sucesso (`recovered-via-spool`) só com entrega correlacionada confirmada e sem perda; senão permanece com o motivo `no durably delivered correlated hook evidence exists; retain this entry and repair the native session manually`.
   - `finish-failed`: exige `run_id` válido, `event_digests`, `final_cursor` e localizador; reexporta o **intervalo exato** `(source_cursor, final_cursor]` do armazenamento nativo; os resumos precisam ser idênticos aos gravados (senão `native transcript changed since the failed finish; refusing non-identical replay`); então reenvia por `POST /workstream/runs/{run_id}/recover/finish` em lotes de 400 eventos/1 MiB como em `run` (checkpoint vazio, `exit_code` gravado, cursor final).
   - `degraded-run` (com exportador): reproduz o intervalo exato pelo caminho de backfill sanitizado (`/hook/batch`, itens de início de sessão, conteúdo e fim com chaves de ingestão estáveis, idempotentes), com a política de captura/`ignore_paths` do checkout aplicada no cliente.
   - O servidor, em `recover/finish`, aceita uma execução `expired` (reativa-a) **se nenhuma execução mais nova existe no mesmo workstream**, e uma `finished` só como repetição exata (mesma sessão, mesmo código de saída, mesmos eventos e cursor); caso contrário `400/409`.
6. Entradas bem-sucedidas (`imported` ou `recovered-via-spool`) são removidas do diário; as que falharam recebem o motivo em `failures` (máx. 3). A quarentena é concluída (sucesso) ou restaurada ao spool (falha).

**Relatório** (`--json`): `server`, `reachable`, `spool {sent, remaining, dropped}`, `spool_lock_busy`, `spool_incomplete`, `journal_pending`, `journal_degraded`, `journal_finish_failed`, `sessions[] {kind, harness, native_session_id, workspace, project, recorded_at, outcome ∈ {imported, recovered-via-spool, failed}, imported_events, detail?}`, `journal_remaining`, `journal_error?`.

**Código de saída:** 1 se o servidor estava acessível e (`spool_incomplete` ou alguma entrada falhou ou há `journal_error`); 0 nos demais casos, inclusive servidor inacessível.
