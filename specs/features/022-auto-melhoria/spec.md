# Spec: Auto-melhoria (revisão, propostas, portão de avaliação, aprovação e experiência)

## Metadados
- **ID:** 022
- **Status:** draft (revisada contra os docs em 2026-10-13, incluindo `docs-adiados/`; nova revisão prevista quando a 021 terminar)
- **Autor:** phm-aguiar
- **Data:** 2026-10-13
- **Feature Anterior:** 020 (projeção de observações, sanitização de caminho, retentativa), 021 (filtro de entropia), 016 (provedor), 014 (navegador web), 008 (admissão), 007 (atores e slots por usuário), 004 (curador)
- **Roadmap:** `specs/features/roadmap.md`
- **Aprovado:** false

## Propósito
> Para quem, o quê e por quê.

Com um provedor, o servidor revisa sessões concluídas e propõe pequenas edições duráveis na wiki (uma armadilha nova, uma regra promovida, um procedimento repetido). Cada proposta é validada por código, pode passar por um portão de avaliação externo, fica registrada com trilha de auditoria e um arquivo de revisão em `_pending/auto-improve/`, e é aprovada automaticamente ou deixada para revisão humana (`require_approval`). É um ciclo limitado, observável e revisável, nunca escrita livre do agente.

## Escopo

### Dentro do escopo
- **Revisão de uma sessão** (`docs-adiados/auto-improvement-loop.md`, `consolidation-llm.md` §5.1, `prompts-schemas-and-sanitizer.md` §5): escopo e pré-filtros (`too_few_observations`, `session_too_short`, registro com provedor `none`), contexto limitado (até 100 páginas recentes sem `sessions/`, página de sessão até 1/3 do orçamento e 32 000, observações amostradas 48 ou 72 com `input_budget_sampled`, até 8 páginas `_rules/`/`procedures/` com corpo até 8000, buffer de rejeições), uma chamada estruturada (temperatura 0,1, 16 000 tokens, operação nova, sem retentativa), revisão somente leitura.
- **Formato e normalizações** (prompts §5 e §10): `operation`, `edit_mode`, `kind` com aliases, aliases de corpo, evidência como objeto ou texto, `expected_base_body_sha256` reservado ao servidor, H1 inserido com aviso.
- **Validação** (`consolidation-llm.md` §5.2, `auto-improvement-loop.md` § Validação): ordem e todos os motivos de rejeição listados; página inteira (tamanho, orçamento de regra e procedimento, H1, destinos aceitos, duplicata de caminho e título exceto `_slots/current-focus.md`, tipo compatível com caminho); patch (alvo no contexto, 1 a 5 edições, 4000 por edição, 12 000 por proposta, 8 por execução, âncoras exatas, `append`, `add_section`, `replace_section` com hash, sem `delete_section`, sem âncora H1, um único H1 no resultado, hash do corpo base); máximo de 5 propostas.
- **Portão de avaliação** (`docs-adiados/auto-improve-eval-gates.md`): `[auto_improve.eval]`, alvos por prefixo exato, execução sem shell, stdin JSON, stdout até 64 KiB, tempo único para escrita, leitura e espera, `passed` obrigatório, `min_delta`, resultados `success`/`rejected`/`failure`/`timeout`, evidência `auto_improve_eval`, registro em `config_json.eval_results` com digests, motivo sanitizado e limitado a 2048, os 7 testes de aceitação do doc; nunca no caminho de hooks.
- **Registro e aprovação** (`auto-improvement-loop.md` § Registro, `docs/storage-and-retrieval.md` §16): execução com provedor, modelo, resumo, avisos, rejeições, configuração, gatilho e resultados do portão; proposta `create`/`update` com snapshot do alvo; pulos (página existe, não existe, pendência no mesmo caminho por operador); arquivo de revisão sanitizado em `_pending/auto-improve/<id>.md` fora do índice; aprovação automática pelo ator próprio com admissão de escrita, ou pendente com `require_approval`; conflito para página fixada (exceto slots de estado) e alvo mudado; estados `pending`, `approved`, `rejected`, `conflict`, `failed`; eventos append-only; rejeição manual com motivo.
- **Buffer de rejeições** (§ Buffer): rejeições manuais, conflitos e falhas, candidatas com motivo; resumo de até 50 entradas em 180 dias, até 1/8 do orçamento e 12 000 caracteres; impressão digital insensível a espaço e caixa.
- **Agendador** (§ Gatilhos, `consolidation-llm.md` §5.4): só com `enabled`, `interval_secs > 0`, `max_sessions_per_tick > 0` e provedor; dorme depois de cada ciclo; marca d'água no primeiro contato; candidatas (fim registrado, depois da marca, idade mínima, sem execução, sem reivindicação viva); reivindicação antes do LLM; falha libera e conta; 3 falhas estacionam; erro só com classe e status; pré-filtro reprovado registra execução; sem gatilho no fim de sessão (`on_session_end` sem consumidor).
- **Passada de experiência** (`docs-adiados/experience.md`, prompts §6): `experience_every_sessions` (0 desliga) e `experience_sessions` (10); âncora `COALESCE(last_experience_run_at, initialized_at)`; janela de até o dobro de sessões, páginas de sessão até 6000 caracteres, filtro de entropia da 021 opcional com aviso; mesma validação, portão e registro; âncora só avança em execução concluída.
- **Superfícies** (§ Comandos, `docs/api-reference.md` §3.2 a §3.4, `docs/mcp-server.md` §3.3): `ai-memory auto-improve --session-id`, `auto-improve-report` (janela 30 dias, 10 linhas, `--stage`), `pending-writes list|show|diff|approve|reject`, `memory_auto_improve` (sem id, a mais recente sem execução; `dry_run`, `stage` e `mode` rejeitados), `/admin/auto-improve` (resposta do doc), `/admin/auto-improve/report`, `/admin/pending-writes` (GET e POST, 409 em conflito), página web `/pending` só para raiz (`docs/wiki-browser.md` §2), modo `stage` do curador da 004 (`notes/curator-<data>.md`).
- **Telemetria** (`consolidation-llm.md` §5.7): taxas terminais, top motivos, impressões digitais repetidas, saída em estrutura ou markdown.

### Fora do escopo (explicitamente)
- Promoção de memória para regras governadas (`docs/design-rules-promotion.md`): proposta no próprio doc.
- Faixa de respostas assentadas mantida pela experiência (proposta em `design-hindsight-borrowings.md` §6.3).

## Comportamento Esperado

### Cenário Principal (Happy Path)
1. Uma sessão de 20 observações e 10 minutos termina; depois de `min_session_age_secs`, o agendador a reivindica, revisa e registra 2 propostas válidas com arquivos de revisão; com `require_approval = false`, ambas são aprovadas e as páginas escritas pelo caminho normal.
2. Com `require_approval = true`, as propostas ficam `pending`; `ai-memory pending-writes approve <id>` aplica; uma proposta cujo alvo mudou vira `conflict` (409).

### Cenários Alternativos
- **Sessão com 5 observações:** execução registrada com `too_few_observations`, sem LLM.
- **Provedor falha 3 vezes na mesma sessão:** sessão estacionada.
- **Portão com `passed=false`:** `eval_gate_failed`, a proposta entra no buffer de rejeições.
- **Proposta para página fixada:** `conflict` "target page is pinned".

## Edge Cases
- Patch com âncora duplicada no alvo: `duplicate_anchor`.
- Duas revisões concorrentes da mesma sessão: só uma reivindica (teste multi-sessão).
- Experiência com menos páginas utilizáveis que o mínimo: pulada sem mover a âncora.

## Constraints
- Invariantes 1 a 15 de `auto-improvement-loop.md` § Invariantes (nada na latência de hooks; escrita só pelo caminho da wiki; ator próprio; limites de custo e mutação; registro de auditoria).
- **INV-07, INV-13**; admissão em toda aprovação.

## Critérios de Sucesso
- [ ] Cada motivo de rejeição da tabela do doc com teste
- [ ] Os 7 testes de aceitação do portão de avaliação
- [ ] Teste multi-sessão da reivindicação do agendador
- [ ] `scripts/check.sh` e o lint passam sem apontamentos

## Abordagem Escolhida
Validação e materialização de patch como funções puras em `internal/domain/autoimprove`; revisão, experiência e aplicação na `wiki` (recebendo o `Provider`); registro, eventos, buffer e agendador no `storage`; portão no `cli` (processo externo); superfícies no `cli`, `mcp` e no navegador da 014.

## Dependências
- 020, 021, 016, 014, 008, 007, 004; `docs-adiados/auto-improvement-loop.md`, `auto-improve-eval-gates.md`, `experience.md`, `consolidation-llm.md` §5, `prompts-schemas-and-sanitizer.md` §5, §6 e §10; `docs/storage-and-retrieval.md` §16, `docs/api-reference.md` §3, `docs/wiki-browser.md` §2.

## Checklist de Prontidão
- [x] Propósito claro e sem ambiguidade
- [x] Escopo delimitado (dentro/fora)
- [x] Cenários cobrem happy path e alternativos
- [x] Edge cases identificados com comportamento esperado
- [x] Constraints explícitas
- [x] Critérios de sucesso mensuráveis
- [x] Abordagem escolhida justificada
