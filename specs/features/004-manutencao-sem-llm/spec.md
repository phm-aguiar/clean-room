# Spec: Envelhecimento e manutenção sem LLM (retenção, varredura de esquecimento, compactação, lint, curador)

## Metadados
- **ID:** 004
- **Status:** spec auditada contra o original (ver "Correções da auditoria" e os relatórios em `reports/`); sem plano nem tasks; em toda dúvida, reproduzir o original
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
- **Reforço de acesso** conforme §4: incremento só em versões atuais, `last_accessed_at`, `page_access` por ator (só com identidade), throttle de 60 s por (página, operador) em memória; consultas `as_of` e buscas globais não reforçam. Estado de retenção (saliência, contador, último acesso) é **por versão**: nova versão recomeça do zero. Conferir e alinhar o que a 001 e a 002 já fazem.
- **Varredura de esquecimento** (§5; `docs/consolidation.md` §1): carga, classificação (expirada, fria), ordem de aplicação (dedup fora do escopo, TTL, compactação, despejo, remoção definitiva de tombstones, poda de observações, **poda de ledgers de eventos**), guardas de seleção, relatório JSON **com os seis contadores de ledger**, `dry_run`; entradas `memory_forget_sweep` (exige capacidade Admin quando o deploy distingue operadores), `POST /admin/forget-sweep`, CLI `forget-sweep` e o agendador.
- **Expiração por TTL** (§5.1): exclusão **física de todas as versões do caminho** (sem tombstone), mesmo fixada; o `as_of` deixa de achar a página em qualquer instante.
- **Despejo por decaimento** (§5.2): remove o arquivo e transforma a linha atual em tombstone de decaimento (`is_latest = 0`, `superseded_at` e `valid_to` com o mesmo instante, vínculos de entidade abertos fechados, auditoria `soft_delete_for_decay`).
- **Remoção definitiva** (§5.3) dos tombstones com `superseded_at <= agora − hard_delete_after_days` (relógio lido de novo após os despejos), com a ancestralidade do mesmo caminho, pela camada wiki; arquivo existente no disco é preservado (a página é reindexada em vez de apagada).
- **Poda de observações** opt-in (§5.4): `observation_retention_days`, lotes de `observation_prune_batch` (5000; zero falha na carga), só de observações mais antigas que o corte, de sessões com página-resumo viva, **sem job de consolidação pendente/em execução** e **sem observações novas desde o último fim de sessão**; ressincroniza `ended_observation_count` e grava auditoria.
- **Poda de ledgers de eventos** opt-in (§5.7): `[decay] ledger_retention_months`, arquivos `log-AAAA-MM.md`, nunca o mês atual nem o anterior.
- **Compactação extrativa A2** (§6; `docs/consolidation.md` §2): `compact_cold_episodic`, L1 (500 **bytes**; leitura do parágrafo por linhas), keep-tokens (7 classes com padrões exatos em `design-memory-aging.md` §6.1, até 48, 2 a 128 **bytes**), corpo e rodapé com formato exato (LF), `compacted: true` e `pages.compacted_at`.
- **Tombamento opcional de páginas sumidas do disco** (`docs/wiki-files-layer.md` §8.3): `reconcile_tombstones_deleted_pages`, ausência em duas passagens com a mesma versão, varredura parcial não conta, disjuntor (`max(3, ceil(total/2))` ou varredura vazia), candidatos sem pendências/manifestos/`bootstrap.md`/ledgers/sessões, intervalo de reconciliação de 30 s.
- **Lint por regra** (`docs/consolidation.md` §3.1, com mensagens exatas): `_lint/report.md` (página `semantic`, `kind: lint-report`, apagada quando não há achados; poda de `_lint/AAAA-MM-DD.md` legados), achados `stale`, `duplicate`, `rule_suggestion`, `pinned_expiring`, `broken_link`, `feedback_flagged`, `contradiction` (aresta declarada); páginas expiradas são ignoradas pelas regras por página; `memory_lint` (sem exigência de Admin), `POST /admin/lint` (com `no_llm`), CLI `lint`.
- **Curador** em modo relatório (`docs/consolidation.md` §3.2, `docs/api-reference.md` §3.4): `cold_episodic` (exclui compactadas), `stale_slot`, `duplicate_title`, `dangling_cross_project_link`, até 25 por tipo; `POST /admin/curator` (`dry_run` padrão; `dry_run`+`stage` ou `mode` desconhecido = 422), CLI `curator`.
- **Agendadores** (`docs/cli.md` §4, `docs/design-memory-aging.md` §5.6): varredura de esquecimento (padrão 86 400 s) e lint de regras (padrão 86 400 s), com último sucesso persistido em `maintenance_scheduler_state` (`forget_sweep`, `rule_lint`), atraso de início `min(intervalo, 60 s)` ou o restante do intervalo, nova tentativa em `min(intervalo, 60 s)` após falha; varredura de projetos vazios (primeira execução 60 s após o início, depois a cada 24 h, sem estado persistido, 7 dias, nunca `scratch`, `_global`, `_profile` nem `_profile.*`). Cada tick percorre todos os projetos de todos os workspaces.
- **`reorg`** (`POST /admin/reorg`, CLI `reorg`; `docs/consolidation.md` §3.3): só no workspace `default`; projeto = último componente do `cwd`; cria projetos mesmo em `dry_run`; reatribui sessões e observações; "cemitério" = aposenta (`is_latest = 0`, `valid_to`) **todas** as páginas atuais do workspace numa transação, sem `superseded_at` e **sem tocar nos arquivos** (não usa `_graveyard/`).
- **`reclaim-ledger-versions`** (`POST /admin/reclaim-ledger-versions`, CLI; `docs/consolidation.md` §3.4): global; só linhas substituídas sem `superseded_at` de ledgers aprovados pelo reconhecimento de conteúdo da 002; `confirm` obrigatório fora de `dry_run`; `drop_latest` e `compact` opcionais.

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
- **Poda de observações:** sessão cuja página foi despejada ou apagada nesta execução mantém a captura bruta, porque a poda roda depois das passadas de página. Sessão com job de consolidação `pending`/`running`, ou com observações novas desde o último fim de sessão, também é preservada. A poda é irreversível.
- **Ledgers:** o mês atual e o anterior nunca são apagados, qualquer que seja a configuração; recusa de webhook ou falha vira `ledgers_undeleted` e nova tentativa; linha de índice legada do ledger é só contada (`ledger_page_rows_left`).
- **Escopo do agendador:** falha em qualquer escopo faz o tick falhar e reexecutar todos os escopos em até 60 s; o sucesso só é gravado após o tick completo.
- **Compactação com corpo só de títulos ou vazio:** sem resumo; o corpo fica só com `## Retained facts` (se houver) e o rodapé.
- **Configuração inválida:** `breadth_weight` negativo/não finito, `half_life_days.<tier>` ≤ 0, `observation_retention_days` < 0, `observation_prune_batch` = 0, `ledger_retention_months` < 0 e faixa de contradição inválida **falham na carga**; `lambda`, `sigma`, `mu`, `cold_threshold`, `salience_default` e `hard_delete_after_days` não são validados (negativo em `hard_delete_after_days` vale 0).
- **Falha por página:** vira aviso, a página fica `applied = false` e é retentada na próxima varredura; nenhuma falha aborta a execução.
- **Página compactada com compactação desligada:** pode ser despejada se ficar fria.
- **Tombamento de página sumida:** ausência em duas passagens com a mesma versão; varredura parcial não conta; disjuntor quando os ausentes passam de `max(3, ceil(total/2))` no escopo; páginas de sessão nunca são candidatas.
- **TTL e fixação:** TTL vence a fixação; o lint avisa com `pinned_expiring`.

## Constraints
- **Performance:** candidatas lidas numa consulta; reescritas de um escopo num lote do escritor (sem N+1); poda em lotes de 5000.
- **Tecnologia:** zero-LLM; só stdlib e as dependências já em `tech.md`.
- **Segurança:** toda mutação de página pela camada wiki; reescrita é supersessão, e as únicas remoções são TTL (física, todas as versões), remoção definitiva após o prazo, a poda opt-in de observações, a poda opt-in de ledgers (arquivos) e as operações administrativas `reclaim-ledger-versions` e hollow-project (`docs/consolidation.md` §4). O `reorg` aposenta páginas sem apagá-las.

## Critérios de Sucesso
- [ ] Os 8 cenários de aceitação de `docs/design-memory-aging.md` §12, exceto o 5 (dedup) e o 6 (dream), passam como teste
- [ ] As propriedades da fórmula de retenção do §3 (página nova pontua `salience`; episódica de 365 dias sem acesso fica abaixo de 0,20) passam como teste
- [ ] Relatório JSON da varredura com os campos do §5.5, incluindo os seis contadores de ledger
- [ ] Poda de observações respeita as 4 condições do §5.4 (corte, página viva, sem job ativo, sem observações novas) e ressincroniza o marcador de fim de sessão
- [ ] Poda de ledgers nunca apaga o mês atual nem o anterior; `reorg` não altera arquivos
- [ ] Lint e curador produzem os achados por regra de `docs/consolidation.md` §3 sem editar páginas
- [ ] Agendadores persistem o último sucesso e não reexecutam após reinício
- [ ] `go vet ./...`, `go test -race ./...` e `golangci-lint run` passam sem apontamentos

## Abordagem Escolhida
Passadas como funções puras sobre dados carregados (classificação, retenção, keep-tokens, compactação, lint, curador), com a aplicação feita pela camada wiki em lote no escritor. As entradas (MCP, rota, CLI e agendador) chamam a mesma função de varredura. Os agendadores ficam no `serve`, com estado em `maintenance_scheduler_state`.

## Dependências
- 003: tombstones de purga, remoção de árvore, compactação do banco e trava de mutação da wiki.
- 002: servidor, MCP, `/admin/*`, escritas rastreadas no git, reconhecimento de ledger.
- 001: `UpsertPage`, `BumpAccess`, cadeia de supersessão e ressurreição de lápide que limpa o `superseded_at`.

## Checklist de Prontidão
- [x] Propósito claro e sem ambiguidade
- [x] Escopo delimitado (dentro/fora)
- [x] Cenários cobrem happy path e alternativos
- [x] Edge cases identificados com comportamento esperado
- [x] Constraints explícitas
- [x] Critérios de sucesso mensuráveis
- [x] Abordagem escolhida justificada

## Correções da auditoria

Auditoria 011 (2026-10-25), validação linha a linha contra o original. Relatório: `reports/011-auditoria-003-004-005.md`.

- Fórmula de retenção, feedback de saliência, limiar de `stale` (0,6/λ, com retorno a 30 dias), regras de lint (lista completa: `stale`, `rule_suggestion`, `pinned_expiring`, `duplicate`, `broken_link`, `contradiction` declarada, `feedback_flagged`, `contradiction` por faixa de similaridade, mais as do LLM, nessa ordem), curador (4 sinais, 25 por tipo, 7/30 dias) e compactação A2 (L1 de 500 bytes, 48 keep-tokens de 2 a 128 bytes, rodapé literal): **conferidos e fiéis**.
- **Resolvidos três "a confirmar"** em `docs/design-memory-aging.md`: (1) a pontuação de retenção só é calculada pela varredura (e pelas passadas que reaproveitam o conjunto frio) e pelo curador, e **não** entra no ranking da busca; (2) a lista de ferramentas que reforçam acesso está completa: `memory_query`, `memory_recent`, `memory_read_page` e `memory_explore`, só via MCP; (3) `memory_query` também reforça os acertos vindos do escopo global reservado e do escopo de perfil.
- **Lint `broken_link`:** "outro projeto" = qualquer alvo gravado com qualificador de projeto, inclusive o próprio; a comparação de nomes é exata.
- **Lint `contradiction` (aresta declarada):** a mensagem usa só o caminho do alvo, sem qualificador.
- **Lint `duplicate`:** o título vai entre aspas com escape de string.
### Correções da auditoria (rodada 2)

- Varredura de esquecimento revalidada linha a linha. Ordem das passadas, guardas, relatório, poda de observações (4 condições, laço de lotes, auditoria só com exclusão) e poda de ledgers: **fiéis**.
- Acréscimos:
  - `ledger_retention_months < 0` também falha antes de qualquer leitura;
  - `observations_prunable` é contado antes das mutações;
  - a contagem de observações da 4ª condição cobre todos os escopos;
  - caminho inválido é pulado em silêncio;
  - a rota administrativa responde 404 para escopo ausente e 500 para erro da varredura, e passa compactação e retenções da configuração, com dedup sempre desligado.