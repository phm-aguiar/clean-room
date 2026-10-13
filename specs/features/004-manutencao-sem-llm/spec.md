# Spec: Envelhecimento e manutenção sem LLM (retenção, varredura de esquecimento, compactação, lint, curador)

## Metadados
- **ID:** 004
- **Status:** draft (revisada contra os docs em 2026-10-13; nova revisão prevista ao fim da 003, com o que a 001 a 003 entregarem)
- **Autor:** phm-aguiar
- **Data:** 2026-10-13
- **Feature Anterior:** 003 (purga, tombstones e compactação do banco), 002 (servidor, MCP, `/admin/*`)
- **Roadmap:** `specs/features/roadmap.md`
- **Aprovado:** false

## Propósito
> Para quem, o quê e por quê.

Com a captura automática ligada, páginas de sessão (tier `episodic`) se acumulam. A memória precisa envelhecer sem LLM: uma pontuação de retenção que cai com o tempo e sobe com o uso, uma varredura agendada que expira por TTL, despeja ou compacta páginas episódicas frias, apaga de vez os tombstones antigos e, se o operador quiser, poda observações já consolidadas. Lint e curador apontam problemas sem editar nada. Tudo funciona sem provedor e tudo é observável por relatório, inclusive em simulação.

Esta feature também é a produtora da lápide de decaimento que a ressurreição da 001 consome (`docs/storage-and-retrieval.md` §5.3).

## Escopo

### Dentro do escopo
- **Pontuação de retenção** (`docs/design-memory-aging.md` §3): fórmula com termo de tempo, termo de acesso e amplitude; chaves `[decay]` (`lambda` 0,02, `sigma` 0,6, `mu` 0,04, `salience_default` 1,0, `cold_threshold` 0,20, `hard_delete_after_days` 180, `breadth_weight` 0,0, `half_life_days.<tier>`), com validação na carga (`docs/cli.md` §2.2).
- **Feedback de saliência** (§3.1): `memory_feedback` (`helpful`, `not_helpful`, `stale`, `wrong`; motivo higienizado e cortado em 500 caracteres; `docs/mcp-server.md` §3.3), passos de 0,25 em `[0,25; 2,0]`, tabela `page_feedback` (já na V01).
- **Reforço de acesso** conforme §4: incremento, `last_accessed_at`, `page_access` por ator, throttle de 60 s por (página, operador) em memória; conferir e alinhar o que a 001 e a 002 já fazem.
- **Varredura de esquecimento** (§5; `docs/consolidation.md` §1): carga, classificação (expirada, fria), ordem de aplicação (TTL, compactação, despejo, remoção definitiva de tombstones, poda de observações), guardas de seleção, relatório JSON, `dry_run`; entradas `memory_forget_sweep`, `POST /admin/forget-sweep`, CLI `forget-sweep` e o agendador.
- **Despejo por decaimento** (§5.2): remove o arquivo e transforma a linha atual em tombstone de decaimento (`is_latest = 0`, `superseded_at`, `valid_to`).
- **Remoção definitiva** (§5.3) dos tombstones mais antigos que `hard_delete_after_days`, com a ancestralidade, pela camada wiki.
- **Poda de observações** opt-in (§5.4): `observation_retention_days`, lotes de `observation_prune_batch` (5000), só de sessões cuja página-resumo está viva.
- **Compactação extrativa A2** (§6; `docs/consolidation.md` §2): `compact_cold_episodic`, L1 (500 bytes), keep-tokens (7 classes, até 48, 2 a 128 bytes), rodapé fixo, `compacted: true` e `pages.compacted_at`.
- **Tombamento opcional de páginas sumidas do disco** (`docs/wiki-files-layer.md` §8.3): `reconcile_tombstones_deleted_pages`, ausência em duas passagens, disjuntor.
- **Lint por regra** (`docs/consolidation.md` §3.1): `_lint/report.md`, achados `stale`, `duplicate`, `rule_suggestion`, `pinned_expiring`, `broken_link`, `feedback_flagged`, `contradiction` (aresta declarada); `memory_lint`, `POST /admin/lint` (com `no_llm`), CLI `lint`.
- **Curador** em modo relatório (`docs/consolidation.md` §3.2, `docs/api-reference.md` §3.4): `cold_episodic`, `stale_slot`, `duplicate_title`, `dangling_cross_project_link`, até 25 por tipo; `POST /admin/curator`, CLI `curator`.
- **Agendadores** (`docs/cli.md` §4, `docs/design-memory-aging.md` §5.6): varredura de esquecimento (padrão 86 400 s), varredura de projetos vazios (diária, 7 dias, nunca `scratch`, `_profile` nem o escopo global) e lint de regras, com último sucesso persistido em `maintenance_scheduler_state` e atraso de início limitado a 60 s.
- **`reorg`** (`POST /admin/reorg`, CLI `reorg`; `docs/api-reference.md` §3.4, `docs/temporal.md`): rederiva o projeto de cada sessão pelo diretório, com plano e cemitério (`_graveyard/`). Contrato curto no doc; detalhar na revisão.
- **`reclaim-ledger-versions`** (`POST /admin/reclaim-ledger-versions`, CLI; `docs/api-reference.md` §3.4, `docs/complementary-contracts.md` §4): remove versões substituídas de ledgers, usando a mesma regra de reconhecimento de ledger da 002.

### Fora do escopo (explicitamente)
- Deduplicação de clusters frios e banda de contradição (precisam de embeddings armazenados; feature 021).
- *Dream pass*, passada de contradição por LLM no lint e pré-filtro de entropia (usado pela passada de experiência por LLM) (dream e entropia na 021; contradição por LLM no lint na 020).
- Modo `stage` do curador, que cria propostas de auto-melhoria (feature 022).
- Influência da retenção no ranking da busca (o doc lista como não implementado).

## Comportamento Esperado

### Cenário Principal (Happy Path)
1. Com a configuração padrão, uma página episódica de 365 dias nunca lida é despejada pela varredura: o arquivo sai, a linha vira tombstone e o relatório traz `evicted[...].deleted = true`.
2. Uma página semântica e uma fixada com a mesma idade ficam intactas; uma página com `expires_at` no passado é apagada mesmo fixada e semântica.
3. A mesma página lida ontem por `memory_read_page` tem retenção maior em `0,6 × ln 2 × e^(−0,04)` (com amplitude desligada) e sobrevive com leituras suficientes.
4. Um tombstone com mais de 180 dias é apagado de vez; os mais novos ficam. Reescrever um caminho tombstoned antes do prazo mantém a versão antiga na cadeia.

### Cenários Alternativos
- **`dry_run = true`:** o relatório lista os mesmos candidatos, todos com `deleted = false`; nenhum arquivo muda; `observations_prunable` é contado.
- **`compact_cold_episodic = true`:** a página fria vira resumo, fatos retidos e rodapé, com `pages.compacted_at`; o corpo anterior é recuperável por `restore-page`; a próxima varredura não toca nela.
- **Página mudou entre seleção e aplicação:** TTL e despejo recusam ou pulam essa página, e o relatório mostra `deleted = false`.
- **Sem wiki:** a varredura relata os candidatos e recusa as mutações de página; a poda de observações ainda roda.
- **`breadth_weight` negativo ou não finito, `observation_retention_days` negativo:** falha antes de mutar.
- **Escopo inexistente na rota administrativa:** erro de requisição; a varredura não cria escopo.

## Edge Cases
- **Poda de observações:** sessão cuja página foi despejada ou apagada nesta execução mantém a captura bruta, porque a poda roda por último. A poda é irreversível.
- **Falha por página:** vira aviso, a página fica `applied = false` e é retentada na próxima varredura; nenhuma falha aborta a execução.
- **Página compactada com compactação desligada:** pode ser despejada se ficar fria.
- **Tombamento de página sumida:** ausência em duas passagens com a mesma versão; varredura parcial não conta; disjuntor quando os ausentes passam de `max(3, ceil(total/2))` no escopo; páginas de sessão nunca são candidatas.
- **TTL e fixação:** TTL vence a fixação; o lint avisa com `pinned_expiring`.

## Constraints
- **Performance:** candidatas lidas numa consulta; reescritas de um escopo num lote do escritor (sem N+1); poda em lotes de 5000.
- **Tecnologia:** zero-LLM; só stdlib e as dependências já em `tech.md`.
- **Segurança:** toda mutação de página pela camada wiki; reescrita é supersessão, e as únicas remoções são TTL, remoção definitiva após o prazo e a poda opt-in (`docs/consolidation.md` §4).

## Critérios de Sucesso
- [ ] Os 8 cenários de aceitação de `docs/design-memory-aging.md` §12, exceto o 5 (dedup) e o 6 (dream), passam como teste
- [ ] As propriedades da fórmula de retenção do §3 (página nova pontua `salience`; episódica de 365 dias sem acesso fica abaixo de 0,20) passam como teste
- [ ] Relatório JSON da varredura com os campos do §5.5
- [ ] Lint e curador produzem os achados por regra de `docs/consolidation.md` §3 sem editar páginas
- [ ] Agendadores persistem o último sucesso e não reexecutam após reinício
- [ ] `scripts/check.sh` e o lint passam sem apontamentos

## Abordagem Escolhida
Passadas como funções puras sobre dados carregados (classificação, retenção, keep-tokens, compactação, lint, curador), com a aplicação feita pela camada wiki em lote no escritor. As entradas (MCP, rota, CLI e agendador) chamam a mesma função de varredura. Os agendadores ficam no `serve`, com estado em `maintenance_scheduler_state`.

## Dependências
- 003: tombstones de purga, remoção de árvore, compactação do banco e trava de mutação da wiki.
- 002: servidor, MCP, `/admin/*`, escritas rastreadas no git, reconhecimento de ledger (T022).
- 001: `UpsertPage`, `BumpAccess`, cadeia de supersessão e ressurreição corrigida (002/T088).

## Checklist de Prontidão
- [x] Propósito claro e sem ambiguidade
- [x] Escopo delimitado (dentro/fora)
- [x] Cenários cobrem happy path e alternativos
- [x] Edge cases identificados com comportamento esperado
- [x] Constraints explícitas
- [x] Critérios de sucesso mensuráveis
- [x] Abordagem escolhida justificada
