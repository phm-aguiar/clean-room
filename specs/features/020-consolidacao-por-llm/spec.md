# Spec: Consolidação por LLM, bootstrap, lint por LLM e perfil por LLM

## Metadados
- **ID:** 020
- **Status:** draft (revisada contra os docs em 2026-10-13; nova revisão prevista quando a 019 terminar)
- **Autor:** phm-aguiar
- **Data:** 2026-10-13
- **Feature Anterior:** 016 (provedor), 008 (admissão `consolidate`), 007 (slots por usuário), 004 (lint), 012 (perfil), 002 (fim de sessão, checkpoints, página de sessão por regras)
- **Roadmap:** `specs/features/roadmap.md`
- **Aprovado:** false

## Propósito
> Para quem, o quê e por quê.

Sem LLM, a página de sessão é sintetizada por regras. Com um provedor, a sessão vira uma página durável bem escrita (ou um lote de até 5 páginas de conceito, decisão, armadilha e regra), um projeto antigo pode ser semeado a partir do histórico (bootstrap), o lint ganha a verificação de contradições por modelo e o perfil entre projetos ganha classificação e mesclagem por modelo. Tudo continua opcional e com recaída para o caminho sem LLM.

## Escopo

### Dentro do escopo
- **Tipos de saída** (`docs/consolidation-llm.md` §2, `prompts-schemas-and-sanitizer.md` §10): `ConsolidatedPage`, `ConsolidatedPageUpdate`, `ConsolidatedBatch`, `BootstrapOutput`, `LintReport`; validações do §10.2 (resumo de uma linha, `kind = rule` para `_rules/<slug>.md` com o slug do doc, título vazio pelo H1 ou pelo caminho, entidades normalizadas até 10, relações só do vocabulário).
- **Sanitização de caminho do modelo** (`consolidation-llm.md` §7.1): caracteres ilegais viram `-`, `.md` acrescentado, idempotente; item não portável é pulado sem abortar o lote.
- **Projeção de observações** (§4): pontuação por tipo, recência, âncoras, termos de alto sinal, boilerplate; seleção (primeira, última, 16 amostras uniformes, pontuação); poda; marcadores; limites de renderização.
- **Orçamento e retentativa** (§3.3 e §3.4): `[consolidation] max_input_tokens` (100 000; mínimo efetivo de 20 040 caracteres = 8350 tokens com a margem 0,8), `max_output_tokens` (32 000, mínimo 1000), margem 0,8, reservas, contexto opcional limitado; até 3 tentativas com 2 s, só para transitórios (classificação da 016); id de operação novo por invocação.
- **Consolidação de página única** (§3.1, `prompts-schemas-and-sanitizer.md` §1): escopo onde as observações foram gravadas, preflight de admissão antes do LLM, `dry_run` sem LLM, contexto com corpo atual, preferências de `_prompts/consolidation.md` limitadas a 2000 caracteres, sanitizadas e enviadas como JSON não confiável, `instructions` por chamada substituindo a página, títulos existentes, desambiguação de título com `(session <id8>)`, frontmatter com `consolidated: true`, evidência de sessão, commit `consolidate(session <id8>): <título>`.
- **Consolidação em lote** (§3.2, prompts §2): 1 a 5 atualizações, guardas por atualização (slots por usuário, slot invariante, página fixada, caminho não portável), aplicação atômica, commit `consolidate-batch(...)`.
- **Prompts** (§3.5, prompts §0 a §4): fronteira de segurança, fidelidade, links, idioma, formato; temperaturas e limites da tabela do §0.
- **`memory_consolidate`** (`docs/mcp-server.md` §3.3): `multi_page`, `dry_run`, `instructions`; sem `session_id`, a última sessão concluída visível; erro `no completed session in <scope>`.
- **Fila do fim de sessão** (`docs/storage-and-retrieval.md` §15, `docs/llm-providers.md` §6.3): enfileirar (geração, `superseded`), reivindicar (`IMMEDIATE`, `claim_id`), completar, falhar com esperas de 30, 60, 120 e 240 s, liberar, reconciliação manual, 5 tentativas; trabalhador (15 s ou notificação, lease de 10 min, encerramento pela janela de graça); `consolidate_on_session_end` (`docs/lifecycle-event-ingestion.md` §8.3 passo 6).
- **Checkpoints com LLM** (`docs/lifecycle-event-ingestion.md` §8.2, corrigido): a página por regras é gravada **na hora** (com admissão `consolidate`; recusa de política falha fechado) e a reescrita por LLM vai para a fila durável; o provedor nunca é chamado dentro do hook.
- **Bootstrap** (`consolidation-llm.md` §11, prompts §3, `docs/api-reference.md` §3.4, `docs/install.md` § Bootstrap mid-project): `POST /admin/bootstrap` (fontes, `max_input_tokens` 50 000, `chunk_input_tokens` 24 000, `dry_run`, `force`, `resume`; 422, 503, 502), cliente `ai-memory bootstrap` coletando fontes (regras do projeto nunca descartadas, README, `docs/`, commits substantivos, cabeçalhos de módulo) com as prioridades de descarte, as flags e a saída JSON do `--dry-run`; chunks com progresso durável em `bootstrap_chunk_progress` e retomada só do prefixo contíguo; manifesto `bootstrap.md`; `bootstrapped_at`.
- **Lint por LLM** (prompts §4, `consolidation-llm.md` § Contradição por LLM): até 20 páginas semânticas e procedurais mais acessadas, prévias de 400 caracteres, achados `contradiction`, `stale`, `duplicate`; falha vira aviso; `no_llm` no `/admin/lint` (`docs/api-reference.md` §3.4).
- **Perfil por LLM** (`docs/cross-project-profile.md` § With an LLM, `docs/design-cross-project-profile.md` §6, prompts §9): `[profile] llm`, classificação em lotes de 20 (até 8 chamadas), mesclagem (até 16 chamadas), pós-processamento do §9, recaída para o caminho sem LLM por lote ou entrada, contagem de recaídas no relatório.

### Fora do escopo (explicitamente)
- Auto-melhoria e passada de experiência (022), dream (021).

## Comportamento Esperado

### Cenário Principal (Happy Path)
1. Com provedor e `consolidate_on_session_end = true`, o fim de uma sessão substantiva grava a página por regras, faz o commit e enfileira o job; o trabalhador reivindica, consolida e grava a página com `consolidated: true`.
2. `ai-memory bootstrap --dry-run` mostra as fontes enviadas e os tokens; sem `--dry-run`, grava as páginas e o manifesto.

### Cenários Alternativos
- **Provedor 503 três vezes no checkpoint:** página por regras gravada.
- **Admissão recusa `consolidate`:** falha fechado, sem página de fallback.
- **Bootstrap repetido sem `--force`:** 422.
- **Caminho do modelo com `:`:** vira `-`; caminho ainda não portável é pulado no lote.

## Edge Cases
- Reivindicante antigo de job com lease vencido não completa o job do substituto (teste multi-sessão).
- Título devolvido igual a título existente: sufixo `(session <id8>)`.
- Retomada de bootstrap com lacuna no meio: recomeça da lacuna.

## Constraints
- **INV-07, INV-13**; admissão antes de gastar LLM; toda escrita pelo caminho de escrita da wiki.
- **Segurança:** observações, páginas, preferências e fontes são dados não confiáveis no prompt.

## Critérios de Sucesso
- [ ] Projeção de observações determinística, com golden de seleção
- [ ] Fila com teste multi-sessão de `claim_id`
- [ ] Bootstrap com retomada por prefixo contíguo coberta por teste
- [ ] `scripts/check.sh` e o lint passam sem apontamentos

## Abordagem Escolhida
Partes puras (tipos, projeção, orçamento, slug e sanitização de caminho) em `internal/domain/consolidate`; orquestração que chama o modelo e escreve páginas na `wiki` (onde a 004 e a 012 já puseram lint e passada de perfil), recebendo o `Provider` da 016 por interface; fila no `storage`; trabalhador, rotas e cliente de bootstrap no `cli`; ferramenta no `mcp`.

## Dependências
- 016, 008, 007, 004, 012, 002; `docs/consolidation-llm.md`, `prompts-schemas-and-sanitizer.md`, `llm-providers.md` §5.3 e §6, `install.md` § Bootstrap; `docs/lifecycle-event-ingestion.md` §8, `docs/storage-and-retrieval.md` §15, `docs/api-reference.md` §3.4, `docs/cross-project-profile.md`.

## Checklist de Prontidão
- [x] Propósito claro e sem ambiguidade
- [x] Escopo delimitado (dentro/fora)
- [x] Cenários cobrem happy path e alternativos
- [x] Edge cases identificados com comportamento esperado
- [x] Constraints explícitas
- [x] Critérios de sucesso mensuráveis
- [x] Abordagem escolhida justificada

## Correções da auditoria (2026-10-20, contra o original)

Docs reescritos/corrigidos: `consolidation-llm.md` (reescrito), `prompts-schemas-and-sanitizer.md` (§12 novo: todos os prompts e mensagens literais), `llm-providers.md` §5.3/§6, `lifecycle-event-ingestion.md` §8.2, `storage-and-retrieval.md` §15, `cli.md` (piso de entrada), `install.md` (bootstrap), `architecture-llm.md`.

Equívocos corrigidos que mudam o código a escrever:

1. **Piso de entrada = 8350 tokens (20 040 caracteres efetivos), não 6000.** O exemplo de `llm-providers.md` (6500) seria recusado pelo original.
2. **Checkpoints:** o doc dizia "LLM primeiro, cai para regras"; o original grava a página por regras primeiro e enfileira a reescrita (o provedor não roda no hook).
3. **Slug de regra:** título sem letras ASCII vira `rule-<8 hex do SHA-256>` (e só vira `rule` sem nenhuma letra/dígito Unicode).
4. **Projeção:** a lista de termos de alto sinal (22), as frases de boilerplate (3), a fórmula da amostra uniforme, a recência `floor(i×80/(N−1))`, os marcadores e o formato do bloco de cada observação estão agora no doc. O **resíduo literal** (`verbatim_residue`) não existia nos docs e foi documentado (§4.3).
5. **Prompts:** a seção "Terminal state and carried detail" e o texto completo do lote/bootstrap/lint/perfil não estavam nos docs; a mensagem de usuário do lote (campos, `tier`/`kind`/`slot_kind`/`entities`) agora é literal.
6. **Fila:** esperas 30/60/120/240 s, erros de forma terminais imediatos, lease 10 min, devolução de tentativa no `release`, checkpoints enfileiram independente de `consolidate_on_session_end`, trabalhador pula página escrita pelo agente.
7. **Bootstrap:** impressão digital (SHA-256 com bytes exatos), prefixo contíguo, primeiro-caminho-vence, `since` inválido ignorado em silêncio, nomes de arquivos de regras (`AGENT.md`, `claude.md`), manifesto literal, páginas sem admissão, serialização das execuções, dry-run sem verificar manifesto. Cabeçalhos de módulo são específicos da linguagem original — **decisão a tomar no Go** (equivalente em `doc.go`).
8. **Guarda de links gerados** (cross-escopo e relações) e sanitização de caminho/portabilidade por item.
9. **Lint por LLM** só no lint pedido (nunca no agendado); filtro `_lint/` não se aplica a ele.
10. **Perfil por LLM:** prompts literais e consistência com `design-cross-project-profile.md` §6 conferida (confiança ausente → 0,6).

**Status: Fiel** após as correções, **desde que** a implementação siga o `consolidation-llm.md` novo (não os trechos antigos). Pendências: tamanho exato do schema serializado (depende do gerador de schema; usar o tamanho do schema que a implementação envia); equivalente de "cabeçalho de módulo" em Go.
