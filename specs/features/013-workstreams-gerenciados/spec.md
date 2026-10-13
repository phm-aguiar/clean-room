# Spec: Workstreams gerenciados (`ai-memory run`)

## Metadados
- **ID:** 013
- **Status:** draft (revisada contra os docs em 2026-10-13 no nível de escopo; contratos por harness vêm da fase 0 do `tasks.md`; nova revisão prevista quando a 010 terminar)
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
- **CLI:** `run` (sintaxe, opções do wrapper antes do harness, flags consumidas, aliases `claude*`, `--executable`), `workstreams`, `rename-workstream`, `workstream-search`, `show`, `continue`, `resume` (`docs/managed-workstreams.md`, `docs/cli.md` §7 e §9.1).
- **Seleção** de workstream e de harness (explícito ou automático), **ordem do lançamento** em 17 etapas, **sessão nativa** (estabelecido, adoção interativa até 8 sessões, seletores explícitos, `--fresh`, sessão vinculada que sumiu), **lançamento degradado** com servidor indisponível (`--require-server`).
- **Contexto entregue:** pacote com até 256 eventos, 6000 caracteres por evento, 30 000 no total, envolvido como histórico não confiável; entrega pelo hook de início de sessão com confirmação e avanço do cursor na mesma transação; **mesma transação da reivindicação do handoff**, que não é consumido se a confirmação do contexto falhar; Crush (arquivo de configuração temporário) e Grok (opção de regras) pelo lançador.
- **Importação** ao sair: lotes de até 400 eventos ou 1 MiB, identificadores determinísticos, cursor por finalização, segmento sanitizado imutável em `raw/`, limites (4096 por finalização, 64 KiB de entrada, 16 KiB no ledger, id até 512 bytes, papel até 32), 3 tentativas.
- **Lease e heartbeat:** 90 s, heartbeat a cada 30 s com tempo de 10 s, espera por lease alheio, `--force-unlock` só para o mesmo operador, cancelamento e códigos de saída.
- **Rotas `/workstream/*`** e limites (`docs/lifecycle-event-ingestion.md` §12, `docs/api-reference.md` §6); acesso por projeto e regra de dono nas mutações de run (`docs/design-per-project-authz.md`).
- **Auto-wire** (`run_autowire`, uma vez por harness, versão e local; sem MCP no degradado).
- **Yolo e ai-jail** (`docs/design-yolo-safety-ai-jail.md`): aviso do `--yolo`, detecção e oferta do jail, reexecução, já dentro do jail, `--true-yolo` do Claude, seleção do que o jail expõe.
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

## Edge Cases
- `kill -9` do lançador: lease vence em até 90 s; a próxima execução começa do último cursor comprometido, sem duplicar.
- Finalização repetida responde com zero eventos; finalizar execução `expired` é recusado.
- OpenCode 2 beta: a confirmação de contexto não é disparada (o doc registra; a consequência é reentrega, nunca perda).

## Constraints
- **Segurança:** conteúdo sanitizado antes de gravar no ledger e no segmento (remover segredo antes de cortar); pacote de contexto como dado não confiável.
- **Tecnologia:** `os/exec` para o processo filho, sem shell.

## Critérios de Sucesso
- [ ] Ordem do lançamento e regras de lease testadas com um servidor e um harness falsos
- [ ] Confirmação de contexto e reivindicação de handoff na mesma transação (multi-sessão, `-race`)
- [ ] Testes esperados de `docs/managed-harness-contributions.md` § 7 para cada harness suportado
- [ ] `scripts/check.sh` e o lint passam sem apontamentos

## Abordagem Escolhida
Lançador no `cli` sobre os adaptadores de transcrição da 010; workstreams, runs, ledger e cursores no `storage` (tabelas da V01); rotas `/workstream/*` no servidor; entrega de contexto pela rota `/handoff` da 002 estendida com o parâmetro de execução gerenciada.

## Dependências
- 010, 002, 003, 007; `docs/managed-workstreams.md`, `docs/harness-adapters.md`, `docs/managed-harness-contributions.md`, `docs/design-yolo-safety-ai-jail.md`, `docs/lifecycle-event-ingestion.md` §12, `docs/api-reference.md` §6, `docs/design-handoff-offer-not-claim.md` §5.

## Checklist de Prontidão
- [x] Propósito claro e sem ambiguidade
- [x] Escopo delimitado (dentro/fora)
- [x] Cenários cobrem happy path e alternativos
- [x] Edge cases identificados com comportamento esperado
- [ ] Constraints explícitas (contratos por harness vêm da fase 0)
- [x] Critérios de sucesso mensuráveis
- [x] Abordagem escolhida justificada
