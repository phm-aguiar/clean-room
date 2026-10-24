# Spec: Curadoria por embeddings (deduplicação, banda de contradição, filtro de entropia e *dream*)

## Metadados
- **ID:** 021
- **Status:** draft (revisada contra os docs em 2026-10-13; nova revisão prevista quando a 020 terminar)
- **Autor:** phm-aguiar
- **Data:** 2026-10-13
- **Feature Anterior:** 017 (vetores armazenados por tripla), 016 (provedor), 004 (varredura, conjunto frio, compactação, lint), 008 (admissão `consolidate`)
- **Roadmap:** `specs/features/roadmap.md`
- **Aprovado:** false

## Propósito
> Para quem, o quê e por quê.

Com vetores armazenados, a manutenção pode juntar páginas frias quase duplicadas sem LLM (deduplicação extrativa), apontar pares do mesmo assunto que provavelmente se contradizem (banda de contradição, só relatório) e, com um provedor, reescrever cada cluster frio numa página coerente (*dream*), sempre sem apagar fonte. Tudo desligado por padrão e sem efeito sem embedder.

## Escopo

### Dentro do escopo
- **Deduplicação de clusters frios (A3)** (`docs/design-memory-aging.md` §7, `docs/consolidation-llm.md` §7.4): `[decay] dedup_cold_clusters` (falso), `dedup_min_pts` (0 = 2), `dedup_max_eps` (0 ou inválido = 0,15); conjunto elegível (frio sem `compacted_at`); distância cosseno limitada a [0, 2] (1,0 para tamanho divergente ou vetor zero); eps adaptativo pelo cotovelo; DBSCAN determinístico; sobrevivente de maior retenção reescrito pelo construtor de compactação da 004 sobre a união dos corpos, com linha de fusão, `merged_from` e `compacted: true`; stubs com `merged_into`; evidência `a3_merge:<id>`; um lote; cluster com leitura falha pulado; só na varredura agendada; `eps` no relatório.
- **Filtro de entropia e boilerplate (A4)** (`design-memory-aging.md` §8, `consolidation-llm.md` §9): função pura `keep`/`skip(near_empty|low_entropy|repetitive)` com a configuração `[auto_improve.scheduler.experience_entropy_filter]` validada na carga mesmo desligada; consumida pela passada de experiência da 022.
- **Banda de contradição** (`design-memory-aging.md` §9): `contradiction_band_min` (0,4) e `contradiction_band_max` (0,75) com variáveis de ambiente; 60 páginas `semantic`/`procedural` mais frias; pares em ordem de caminho; achado `contradiction` `info` com a mensagem do doc; até 25; nada editado nem aresta persistida.
- **Dream pass (B2 a B4)** (`design-memory-aging.md` §10, `prompts-schemas-and-sanitizer.md` §7, `consolidation-llm.md` §8): configuração `[dream]` e a forma por ambiente; agendador só com `enabled`, provedor e embedder; relógio de atividade carimbado em cada chamada de ferramenta MCP (inicializado no boot); janela ociosa; observador de cerca de 2 s; cancelamento só entre clusters; erro de escopo registrado sem parar; algoritmo do §10.3 (conjunto frio, vetores frios e conhecimento existente, DBSCAN, surprisal, ordenação e corte, `dry_run` sem LLM, preflight de admissão `consolidate`, leitura, uma chamada estruturada com o prompt literal, corpo de cada membro até 6000 bytes com `[truncated]`, `max_tokens` 4000, temperatura 0,1, esquema `{title, body_markdown}` com extras rejeitados, lote do sobrevivente e dos stubs, evidência `b2_dream:`); sobrevivente não marcado `compacted`; relatório JSON do §10.4.

### Fora do escopo (explicitamente)
- Deduplicação nas entradas manuais da varredura (MCP, rota, CLI): o doc diz que só a varredura agendada faz.
- Aresta `contradicts` persistida pela banda.

## Comportamento Esperado

### Cenário Principal (Happy Path)
1. Cenário 5 do `design-memory-aging.md` §12: duas páginas frias quase idênticas com vetores e deduplicação ligada → um sobrevivente com `merged_from`, um stub com `merged_into` e uma evidência `a3_merge:...`.
2. Cenário 6: dream ligado com provedor e embedder e operador ocioso por 300 s → a execução começa; uma chamada de ferramenta durante a execução cancela depois do cluster atual e o relatório mostra `cancelled = true`.

### Cenários Alternativos
- **Sem embedder:** deduplicação, banda e dream não fazem nada e não dão erro.
- **Dream sem provedor:** o trabalho nunca começa; log explica.
- **Admissão recusa o cluster:** cluster pulado, tentado na próxima execução.
- **Falha do LLM num cluster:** pulado; os outros seguem; contado em `skipped`.

## Edge Cases
- Menos de `min_pts` páginas elegíveis ou com vetor: nada acontece.
- Páginas recém-reescritas não ficam frias na varredura seguinte; stubs e sobrevivente da A3 ficam terminais pelo `compacted_at`.
- Banda com empate de `updated_at`: a primeira em ordem de caminho é tratada como a mais nova.

## Constraints
- **INV-13**, **INV-07** (dream estruturado), INV-08 (vetores só da tripla configurada).
- Nunca apaga fonte; toda reescrita é supersessão pelo caminho de escrita da wiki.

## Critérios de Sucesso
- [ ] Cenários 5 e 6 do `design-memory-aging.md` §12 como teste
- [ ] Eps adaptativo e DBSCAN com testes de tabela determinísticos
- [ ] `scripts/check.sh` e o lint passam sem apontamentos

## Abordagem Escolhida
Funções puras (cosseno, eps, DBSCAN, surprisal, entropia) em `internal/domain/maintenance`, ao lado da retenção da 004; deduplicação, banda e dream na `wiki`, junto da varredura e do lint da 004; agendador do dream no `cli`; relógio de atividade no `mcp`.

## Dependências
- 017, 016, 004, 008; `docs/design-memory-aging.md` §7 a §12; `docs/consolidation-llm.md` §7.4, §8 e §9; `prompts-schemas-and-sanitizer.md` §7.

## Checklist de Prontidão
- [x] Propósito claro e sem ambiguidade
- [x] Escopo delimitado (dentro/fora)
- [x] Cenários cobrem happy path e alternativos
- [x] Edge cases identificados com comportamento esperado
- [x] Constraints explícitas
- [x] Critérios de sucesso mensuráveis
- [x] Abordagem escolhida justificada

## Correções da auditoria (2026-10-20, contra o original)

Docs corrigidos: `design-memory-aging.md` (§7.2, §7.3, §8, §9, §10, §12), `consolidation-llm.md` §7.4, §8, §9, §10, `architecture-llm.md`, `experience.md`.

- **Sobrevivente em empate exato de retenção** é o **último** membro do cluster (ordem em que o DBSCAN alcançou), não o primeiro. Vale na A3 e no dream.
- **Corpo unido da A3** = corpo do sobrevivente + corpos dos *outros* membros; a união de palavras-chave continua com teto de **48 tokens** no total (o "a confirmar" de `architecture-llm.md` foi resolvido).
- **Cotovelo:** máxima distância perpendicular (empate: primeiro índice); só com 2 pontos ou menos usa o último valor (não há "corda plana" especial).
- **Textos dos stubs** (A3 e dream) agora literais.
- **Banda de contradição:** validação de `contradiction_band_*` confirmada (finitos, `0 ≤ min < max ≤ 1`, falha na carga); roda **apenas no lint pedido pelo usuário** (`memory_lint`, `/admin/lint`, CLI), nunca no lint agendado; exclui `_lint/` e páginas expiradas; ordenação (acessos, `updated_at`, caminho) para as 60 mais frias.
- **Dream:** o laço dorme `interval_secs` (3600) **antes** de checar a ociosidade; observador de 2 s; cancelamento checado antes de cada escopo e de cada cluster; a atividade vem só das chamadas de ferramenta MCP (hooks e rotas admin não contam); relatório de no-op traz provedor `none`.
- **Filtro de entropia:** padrão desligado confirmado; entropia calculada sobre todos os caracteres, inclusive espaços.

**Status: Fiel** (docs completos e coerentes com o original após as correções).
