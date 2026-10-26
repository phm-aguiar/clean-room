# Spec: Workstreams gerenciados (`ai-memory run`)

## Metadados
- **ID:** 013
- **Status:** spec auditada contra o original (ver "Correções da auditoria" e os relatórios em `reports/`); sem plano nem tasks; em toda dúvida, reproduzir o original
- **Autor:** phm-aguiar
- **Data:** 2026-10-13
- **Feature Anterior:** 010 (adaptadores de transcrição, instalação nos agentes), 002 (`/handoff` e reivindicação), 007 (dono e acesso por projeto)
- **Roadmap:** `specs/features/roadmap.md`
- **Aprovado:** false

## Propósito
> Para quem, o quê e por quê.

Quem troca de harness no meio de um trabalho quer que a retomada nativa do harness (`claude --resume`, o seletor de sessões) continue valendo, e que o próximo agente receba o histórico de ferramentas, não só um resumo. `ai-memory run` é um lançador opcional que seleciona o workstream do checkout, cria ou retoma a sessão nativa correta, entrega o trecho novo do ledger como contexto e, ao sair, importa o transcript para um ledger portátil. A continuidade básica entre agentes já funciona só com os hooks (002 e 010); esta feature é para quem quer a retomada nativa entre harnesses.

## Escopo

### Dentro do escopo
- **Conceitos e estados** (`docs/managed-workstreams.md` § Conceitos): workstream por `(workspace, projeto, fingerprint do repositório, fingerprint do worktree)`, execução com lease, sessão nativa vinculada, ledger append-only, cursores de origem e de entrega; estados `active`, `finished`, `expired`.
- **CLI:** `run` (sintaxe completa — `--workspace --project --workstream --new --executable --profile --env --env-file --require-server --yolo --true-yolo --fresh --force-unlock --no-autowire --jail --no-jail` —, flags consumidas depois do harness, aliases `claude*`, `copilot`, `--executable`, ambiente do filho), **menu pós-execução**, `workstreams`, `rename-workstream`, `workstream-search`, `show`, `continue`, `resume` (`docs/managed-workstreams.md`, `docs/cli.md` §7 e §9.1).
- **Seleção** de workstream e de harness (explícito ou automático), **ordem do lançamento** em 17 etapas, **sessão nativa** (estabelecido, adoção interativa até 8 sessões, seletores explícitos, `--fresh`, sessão vinculada que sumiu), **lançamento degradado** com servidor indisponível (`--require-server`).
- **Contexto entregue:** pacote com até 256 eventos, 6000 caracteres por evento, 30 000 no total, envolvido como histórico não confiável; entrega pelo hook de início de sessão com confirmação e avanço do cursor na mesma transação; **mesma transação da reivindicação do handoff**, que não é consumido se a confirmação do contexto falhar; Crush (arquivo de configuração temporário) e Grok (opção de regras) pelo lançador.
- **Importação** ao sair: lotes de até 400 eventos ou 1 MiB, identificadores determinísticos, cursor por finalização, segmento sanitizado imutável em `raw/`, limites (4096 por finalização, 64 KiB de entrada, 16 KiB no ledger, id até 512 bytes, papel até 32), 3 tentativas.
- **Lease e heartbeat:** 90 s, heartbeat a cada 30 s com tempo de 10 s, espera por lease alheio, `--force-unlock` só para o mesmo operador, cancelamento e códigos de saída.
- **Recuperação** (`docs/downtime-recovery.md`): diário local, `ai-memory recover`, rota `recover/finish`.
- **Registro local do cliente** (`client-projects.json`) e `continue`/`resume` (`--all`, `--search`, `--limit`).
- **Rotas `/workstream/*`** (12, incluindo `recover/finish`) e limites (`docs/lifecycle-event-ingestion.md` §12, `docs/api-reference.md` §6); acesso por projeto e regra de dono nas mutações de run (`docs/design-per-project-authz.md`).
- **Auto-wire** (`run_autowire`, uma vez por harness, versão e local; sem MCP no degradado).
- **Yolo e ai-jail** (`docs/design-yolo-safety-ai-jail.md`): aviso do `--yolo`, `--true-yolo` do Claude. **No MVP Windows o jail é inexistente**: `--jail` falha com a mensagem do doc, `--no-jail` é aceito, nunca há oferta nem reexecução; detecção, oferta, checklist e seleção do que o jail expõe ficam para as plataformas Unix.
- **Contribuições de harness gerenciado** (`docs/managed-harness-contributions.md`): contrato para registrar um harness novo, script de aceitação.
- Integração com a purga de projeto da 003 (lease vivo bloqueia sem `--force`) e com o movimento de projeto.

### Fora do escopo (explicitamente)
- Herdr (ferramenta externa, `docs/managed-workstreams.md` § Herdr).
- macOS.
- Consolidação por LLM das sessões (020).

## Comportamento Esperado

### Cenário Principal (Happy Path)
1. `ai-memory run claude` no checkout cria o workstream `default`, prepara a execução com lease e inicia o Claude Code; heartbeats a cada 30 s.
2. Ao sair, o lançador importa o transcript para o ledger e finaliza a execução (`finished`).
3. `ai-memory run codex` no mesmo checkout cria uma sessão nova no Codex e entrega o pacote de contexto com o histórico do Claude.
4. `ai-memory run claude` de novo retoma a sessão nativa vinculada do Claude com o trecho novo do ledger.

### Cenários Alternativos
- **Servidor indisponível:** lançamento degradado com aviso (sem lease, sem `AI_MEMORY_RUN_ID`), ou falha com `--require-server`.
- **Workstream ocupado por outro lançador:** espera até o lease vencer (interativo) ou recusa e sugere `--new`.
- **Preparação recusada (autenticação, erro de resposta):** o agente não inicia.
- **Sessão vinculada apagada:** sessão nova e reapontamento.

## Correções da auditoria (2026-10-19)
- **Rotas:** `POST …/context` exige escrita (não leitura) e só vale para Crush/Grok; `cancel` é idempotente (204 também em execução terminal; 404 se inexistente); `heartbeat` renova mesmo com o instante vencido enquanto `active`; `finish` em execução `expired` = 409 `managed run is not recoverable`; erro de ocupação tem formato fixo que o cliente interpreta (`workstream is already active: owned by <dono> until <RFC 3339>`). Detalhes em `docs/api-reference.md` §6.
- **Importação:** conteúdo cortado em 64 KiB com marcador na rota e em 16 KiB com `…` no ledger; eventos de fronteira `managed-run:<run>:checkpoint|losses`; validação de agente/sessão do evento (`unresolved:<run>` sem sessão); sequência por workstream começa em 1; cursor de entrega ao concluir = `max(atual, X)`; segmento em `raw/workstreams/<id>/segments/<run>-<16 hex>.jsonl`.
- **Pacote de contexto:** texto literal e regras de 6 000/30 000 **caracteres** em `docs/managed-workstreams.md`; troca de harness cria o dossiê `AI_MEMORY_PREVIOUS_SESSION_HISTORY`; reivindicação numa transação com o handoff, sem handoff no texto quando a reivindicação do contexto falha.
- **Ambiente do filho** (`--profile/--env/--env-file`, comparação de nomes sem caixa no Windows) e **resolução de executável** por `PATH` + `PATHEXT` (nunca o irmão sem extensão; `cmdc` no Windows).
- **Fingerprints** com fórmula exata (`docs/harness-adapters.md`).
- **Registro do cliente** e **diário de recuperação**: formatos, limites, travas (`LockFileEx`) e proteção de arquivo no Windows.
- **Purga de projeto (003):** só bloqueia execução `active` com lease **ainda válido** (`lease_expires_at > agora`); `--force` ignora.
- **Sem timeout** nas chamadas HTTP do lançador, exceto sondagem (2 s) e heartbeat (10 s).

### Segunda rodada: comandos e mecanismos de apoio (auditoria 008, 2026-10-24)
- **`show`:** contrato completo (ordem, menus, teclas, textos, varredura de 12 marcadores, criação de projeto, `--json`) em `docs/managed-workstreams.md` § Contrato detalhado de `show`; `show` não aceita `--jail`/`--no-jail`.
- **`finalize-session`:** conferido sem divergências (`docs/cli.md` §9.1); acrescenta-se que o `session_id` é UUID, que `--reopen` exige `--session-id` e que o id guardado só é apagado quando alguma sessão foi finalizada.
- **`backfill` e importação de intervalo exato:** conferidos contra `docs/design-boot-backfill.md` (completo); a única diferença da recuperação é a chave de ingestão com o `id` da entrada do diário (`docs/downtime-recovery.md` § 3.1).
- **Spool, quarentena e drenagem:** `docs/downtime-recovery.md` § 4 (formato em disco, travas, orçamentos, tentativas, tabela de desfechos, Windows 32/33/`LockFileEx`). **Equívocos corrigidos em `docs/cli.md` § 5.3:** `auth_mode` grava `static`/`oidc`/`none` (não `static_token`/`oidc_at_drain`/`anonymous`); só o `403` descarta o item de imediato (o doc dizia "`4xx` exceto 401/408/429"); `408` consome tentativa.
- **`ai-memory recover`:** `docs/downtime-recovery.md` revalidado linha a linha; § 3.1 acrescenta ordem real, mensagens literais de cada falha, texto humano, fim de entrada com quarentena e a exceção "já entregue". **Defeito do original, reproduzir tal como está (resolvido: reproduzir o original):** o leitor do diário não conhece o harness `copilot`; a entrada falha com `journal names unsupported harness 'copilot'` e fica no diário (`docs/downtime-recovery.md`; `docs/harness-adapters.md` §6.11).
- **Jail:** `docs/design-yolo-safety-ai-jail.md` corrigido: a lista de variáveis repassadas tem **14** nomes (faltava `AI_MEMORY_BASE_PATH`), valores resolvidos da configuração, recusa de token vindo do `config.toml` sem `AI_MEMORY_AUTH_TOKEN`, JSON exato do `--settings` do `--true-yolo`, `BWRAP_BIN`, mensagens literais, tabela de rótulos/notas do checklist (§ 5.2.1), "a confirmar" resolvidos ou classificados como externos (§ 9.1). No Windows nada disso executa (nunca há oferta nem reexecução).
- **Autorização e handoff:** regras de dono/escrita e transação de reivindicação conferidas; correções: o argumento da ferramenta é `any_owner` (não `all_owners`; também em `docs/mcp-server.md`); a varredura pós-reivindicação da ferramenta poupa **toda** sessão de origem aberta (não "5 minutos"); regras da sessão receptora (existência, escopo, encerrada, exceção do Grok) documentadas.
- **Lease/purga (003):** o 409 da purga não tem campo `active_workstreams` (só `error` com a lista); critério de lease vivo confirmado (`active` e `lease_expires_at > agora`); `docs/lifecycle-ops.md` e a spec 003 corrigidos.
- **Concorrência:** `docs/managed-workstreams.md` § Concorrência: escritor serial; a rota `finish` **não** é atômica (lê estado, grava segmento, depois transação), então um `cancel` no meio dá `400 invalid state: managed run <id> is expired` e deixa segmento órfão; demais corridas tabeladas.

### Correções da auditoria dos adaptadores e planos de lançamento (2026-10-24; o relatório dessa rodada não está mais em `reports/`)
- **OpenCode:** `run opencode` sonda `--version` (2 s, major 1 ou 2; `opencode2` força V2); documentado em `docs/harness-adapters.md` § "Dialeto do OpenCode".
- **Eco do pacote no Claude Code** só é filtrado em resultados de ferramenta (não em mensagens); datas dos adaptadores SQLite ausentes; Kimi/Kiro v2/Antigravity convertem ms; demais repassam o texto de `timestamp` sem validar.
- **Antigravity** não exporta transcript no `run` (só backfill/listagem); janela de abertura é de 5 s para mais ou menos.
- **Kiro:** flags `--no-interactive`, `--list-sessions` etc. valem para v2 e v3; só `--agent-engine v3` seleciona v3; resolução de motor por existência do id nas duas lojas.
- **Cursores, limites e mensagens literais** de exportação, delta e intervalo transcritos (`harness-adapters.md` §3.4, §6.6, §6.7); exportação que estoura limite não avança o cursor e **não há paginação** (resolvido: reproduzir o original, §6.11).
- **Windows:** prefixo `\\?\` da canonicalização quebra a comparação textual de `directory` (OpenCode) e do nome de pasta do Claude Code; reproduzir o original tal como está, **sem normalizar** (§6.10).
- **Copilot** ausente na tabela do diário de recuperação do original: reproduzir o defeito (§6.11).
- **Erro de descoberta após a saída** cancela a execução e o comando falha com esse erro, perdendo o código do filho, como no original (§6.8, §6.11).
- Criada a referência por harness (`harness-adapters.md` §6.1 a §6.11): layout de armazenamento, cabeçalhos, mapeamento registro a registro, SQL, composição de argv/ambiente (Crush `CRUSH_GLOBAL_CONFIG`, Grok `--rules`, `claude attach`, `KIRO_HOME`).

## Edge Cases
- `kill -9` do lançador: lease vence em até 90 s; a próxima execução começa do último cursor comprometido, sem duplicar.
- Finalização repetida responde com zero eventos; finalizar execução `expired` é recusado.
- OpenCode 2 beta: a confirmação de contexto não é disparada (o doc registra; a consequência é reentrega, nunca perda).

## Constraints
- **Segurança:** conteúdo sanitizado antes de gravar no ledger e no segmento (remover segredo antes de cortar); pacote de contexto como dado não confiável.
- **Tecnologia:** `os/exec` para o processo filho, sem shell; no Windows, resolver o arquivo por `PATH`/`PATHEXT` e executar o arquivo resolvido (não o nome simples). Ctrl+C: tratar `CTRL_C_EVENT`; o filho recebe o mesmo evento do console.

## Critérios de Sucesso
- [ ] Ordem do lançamento e regras de lease testadas com um servidor e um harness falsos
- [ ] Confirmação de contexto e reivindicação de handoff na mesma transação (multi-sessão, `-race`)
- [ ] Testes esperados de `docs/managed-harness-contributions.md` § 7 para cada harness suportado
- [ ] `go vet ./...`, `go test -race ./...` e `golangci-lint run` passam sem apontamentos

## Abordagem Escolhida
Lançador no `cli` sobre os adaptadores de transcrição da 010; workstreams, runs, ledger e cursores no `storage` (tabelas da V01); rotas `/workstream/*` no servidor; entrega de contexto pela rota `/handoff` da 002 estendida com o parâmetro de execução gerenciada.

## Dependências
- 010, 002, 003, 007; `docs/managed-workstreams.md`, `docs/harness-adapters.md`, `docs/managed-harness-contributions.md`, `docs/design-yolo-safety-ai-jail.md`, `docs/lifecycle-event-ingestion.md` §12, `docs/api-reference.md` §6, `docs/design-handoff-offer-not-claim.md` §5.

## Checklist de Prontidão
- [x] Propósito claro e sem ambiguidade
- [x] Escopo delimitado (dentro/fora)
- [x] Cenários cobrem happy path e alternativos
- [x] Edge cases identificados com comportamento esperado
- [x] Constraints explícitas (contratos por harness em `docs/harness-adapters.md` §6)
- [x] Critérios de sucesso mensuráveis
- [x] Abordagem escolhida justificada

## Escopo exigido pela auditoria (2026-10-19)

Itens que a implementação deve cobrir; cada um aponta para o doc de origem.

- **A1:** ambiente do filho (`--profile`, `--env`, `--env-file`; nomes sem caixa no Windows; variáveis reservadas) — `docs/managed-workstreams.md`.
- **A2:** resolução de executável no Windows (`PATH` + `PATHEXT`, sem irmão sem extensão) e lançamento sem shell.
- **A3:** menu pós-execução (3 TTYs, código 0, não degradado, sem `--executable`).
- **A4:** registro local do cliente `client-projects.json` (trava exclusiva, limites, versão) e `continue`/`resume`/`show`.
- **A5:** diário de recuperação, rota `recover/finish` e `ai-memory recover` (`docs/downtime-recovery.md`).
- **A6:** dossiê da sessão anterior (`AI_MEMORY_PREVIOUS_SESSION_HISTORY`) e `claude attach` de sessão em segundo plano.
- **A7:** fingerprints (fórmula exata), checkpoint (`git status --porcelain=v1 -z`, 256 caminhos).
- **A8:** eventos de fronteira, segmento bruto e regras de cursor na finalização.
- **A9:** mensagens literais de stderr (espera por lease, heartbeat, degradado) — critérios de aceite por texto.
- **A10:** spool local, quarentena e drenagem (`docs/downtime-recovery.md` § 4): nomes, travas (`LockFileEx`, erros 32/33), orçamentos, tabela de desfechos e testes de corrida enfileirar × quarentena × drenar.
- **A11:** contrato do `show` (`docs/managed-workstreams.md`), com teste de menu por terminal falso e de `--json`.
- **A12:** concorrência de execuções (`docs/managed-workstreams.md` § Concorrência), incluindo a janela `cancel` entre a leitura de estado e a transação do `finish`.
- **A13:** jail e yolo (`docs/design-yolo-safety-ai-jail.md`): lista de 14 variáveis, recusa de token vindo da configuração, rótulos do checklist; no MVP Windows só o aviso de `--yolo` e a recusa de `--jail`.
