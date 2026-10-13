# plan.md: Plano de Implementação Técnica — Manutenção sem LLM (004)

> Plano gerado em 2026-10-13, antes de a 002 e a 003 serem executadas. Nomes de arquivo e de função citados aqui vêm dos `tasks.md` delas; revisar quando a 003 terminar.

## 1. Metadados do Plano
- **Stack Tecnológico:** a da 002. Nada novo: `math` (exponencial e logaritmo), `regexp` (keep-tokens, RE2) e `encoding/json` (relatórios).
- **Feature Fonte:** `specs/features/004-manutencao-sem-llm/spec.md`
- **Escopo:** retenção, feedback, reforço de acesso, varredura de esquecimento (TTL, despejo, compactação A2, remoção definitiva, poda), tombamento opcional de páginas sumidas, lint por regra, curador em modo relatório, agendadores, `reorg` e `reclaim-ledger-versions`. Sem embeddings nem LLM.
- **Fontes de contrato:** `docs/design-memory-aging.md` (§1 a §6, §11, §12), `docs/consolidation.md`, `docs/storage-and-retrieval.md` §5.3, `docs/wiki-files-layer.md` §8.3, `docs/cli.md` §2.2 e §4, `docs/mcp-server.md` §3.3, `docs/api-reference.md` §3.4, `docs/temporal.md`, `docs/complementary-contracts.md` §4.

### Layout
```text
internal/domain/maintenance/   ← puro, sem I/O: retenção, classificação, keep-tokens, compactação A2, regras de lint e curador
internal/storage/              ← + tombstone de decaimento, remoção de cadeia decaída, poda de observações, feedback, estado de agendador
internal/wiki/                 ← + aplicação em lote da varredura, relatório de lint em _lint/, tombamento de página sumida
internal/cli/                  ← + rotas /admin/forget-sweep, lint, curator, reorg, reclaim-ledger-versions; agendadores no serve
internal/mcp/                  ← + memory_forget_sweep, memory_lint, memory_feedback
```
`internal/domain/maintenance` é subpacote de `domain`: sem I/O e sem importar outros componentes, como manda a constituição.

## 2. Design de Contratos e Fronteiras
- **Contrato:** relatório JSON da varredura (`docs/design-memory-aging.md` §5.5), achados de lint e curador (`docs/consolidation.md` §3), rotas e tools de `docs/api-reference.md` §3.4 e `docs/mcp-server.md` §3.3, chaves `[decay]` e `[maintenance]`.
- **Esquema SQL:** sem migração. Usa `pages.salience`, `pages.compacted_at`, `pages.superseded_at`, `pages.access_count`, `pages.last_accessed_at`, `page_access`, `page_feedback`, `maintenance_scheduler_state` e `observations`, todos já na V01.

### 2.1 Configuração
| Chave | Padrão | Validação na carga |
|---|---|---|
| `[decay] lambda`, `sigma`, `mu`, `salience_default`, `cold_threshold` | 0,02; 0,6; 0,04; 1,0; 0,20 | finitos |
| `[decay] hard_delete_after_days` | 180 | negativo vira 0 |
| `[decay] breadth_weight` | 0,0 | finito e ≥ 0 |
| `[decay] half_life_days.<tier>` | sem valor | > 0 e finito |
| `[decay] compact_cold_episodic` | false | — |
| `[decay] observation_retention_days`, `observation_prune_batch` | 0; 5000 | ≥ 0; > 0 |
| `[maintenance] enabled`, `forget_sweep_interval_secs`, `lint_interval_secs`, `reconcile_tombstones_deleted_pages` | conforme `docs/cli.md` §4 e `docs/wiki-files-layer.md` §8.3 | ≥ 0 |

## 3. Decisões Arquiteturais e Justificativas

- **Decisão (M-01): passadas puras e aplicação em lote pela wiki.** Retenção, classificação, keep-tokens, compactação A2, regras de lint e sinais do curador são funções puras em `domain/maintenance`, testadas sem disco. A varredura carrega as candidatas numa consulta, calcula o plano e aplica por escopo num lote: a wiki reescreve ou remove os arquivos e o `storage` grava no escritor.
  - **Justificativa:** princípios do `design-memory-aging.md` §1 (só a wiki muta páginas, escritor único, sem N+1, tudo observável).
  - **Alternativa Rejeitada:** passadas com I/O misturado (impede testar a fórmula e a ordem sem disco).

- **Decisão (M-02): tombstone de decaimento e remoção definitiva no `storage`.** Despejo: a wiki remove o arquivo e o `storage` marca a linha atual com `is_latest = 0`, `superseded_at` e `valid_to` no mesmo instante, fechando os vínculos de entidade, **só se o id esperado ainda for o atual**. Remoção definitiva: apaga os tombstones com `superseded_at` mais antigo que o corte, junto com a ancestralidade, dentro do escopo, preservando página recriada no mesmo caminho.
  - **Justificativa:** `docs/storage-and-retrieval.md` §5.3 e `docs/temporal.md` (tabela de retiradas).
  - **Alternativa Rejeitada:** apagar a linha no despejo (perde a recuperação por `restore-page` durante a carência).

- **Decisão (M-03): `compacted_at` gravado pelo `UpsertPage`.** O `UpsertPage` da 001 passa a preencher `pages.compacted_at` com o instante da escrita quando o frontmatter traz `compacted: true`, e `NULL` caso contrário.
  - **Justificativa:** `docs/design-memory-aging.md` §2 e §6 ("on write, the store sets `pages.compacted_at` from the frontmatter flag").
  - **Alternativa Rejeitada:** marcar depois numa segunda escrita (fora da transação da versão).

- **Decisão (M-04): uma função de varredura para as quatro entradas.** `memory_forget_sweep`, `POST /admin/forget-sweep`, CLI `forget-sweep` e o agendador chamam a mesma função, com a mesma ordem de passes. A deduplicação, que só o agendador roda, entra com a 016.
  - **Justificativa:** `docs/design-memory-aging.md` §5.
  - **Alternativa Rejeitada:** caminhos separados por entrada.

- **Decisão (M-05): agendadores no `serve` com estado persistido.** Uma goroutine por job (varredura, projetos vazios, lint), com o `context` do servidor; último sucesso em `maintenance_scheduler_state`; atraso de início calculado pelo tempo restante, com teto de 60 s; nova tentativa com atraso limitado pelo intervalo e por 60 s. Um tick percorre todos os escopos; falha num escopo é contada e o tick é dado como falho depois dos demais.
  - **Justificativa:** `docs/cli.md` §4 e `docs/design-memory-aging.md` §5.6.
  - **Alternativa Rejeitada:** `time.Ticker` sem persistência (reinício reexecuta tudo).

- **Decisão (M-06): reforço de acesso como no doc.** Conferir o `BumpAccess` da 001 e os disparos das tools da 002 contra o §4: incremento, `last_accessed_at` e `page_access` numa transação, só em `is_latest = 1`, ids apagados pulados sem falhar o lote; throttle de 60 s por (página, operador) num mapa em memória, sem renovar o carimbo quando suprimido; anônimos compartilham a chave; tools disparam depois da resposta e só em sucesso.
  - **Justificativa:** a retenção depende desses dados; uma divergência aqui muda quem é despejado.
  - **Alternativa Rejeitada:** confiar no que a 001 fez sem conferir.

- **Decisão (M-07): lint e curador como relatório.** O lint grava `_lint/report.md` pelo `WritePage` (ou remove a página quando não há achados) e poda relatórios datados antigos fora da simulação; `no_llm` é aceito e não muda nada nesta feature (só há regras determinísticas). O curador devolve o relatório; o modo `stage` responde 422 até a 016.
  - **Justificativa:** `docs/consolidation.md` §3 e `docs/api-reference.md` §3.4.
  - **Alternativa Rejeitada:** criar já o modo `stage` sem a infraestrutura de propostas (stub).

- **Decisão (M-08): tombamento opcional com estado em memória.** Com `reconcile_tombstones_deleted_pages`, a reconciliação guarda, por escopo, os candidatos ausentes e a versão vista; marca só na segunda ausência seguida com a mesma versão, sob a trava exclusiva da wiki e depois de reconferir; aplica o disjuntor `max(3, ceil(total/2))`. O estado não sobrevive a reinício (decisão nossa: o doc não diz onde guardá-lo; reiniciar só adia uma marcação).
  - **Justificativa:** `docs/wiki-files-layer.md` §8.3.
  - **Alternativa Rejeitada:** marcar na primeira ausência (o doc proíbe).

- **Decisão (M-09): `reorg` e `reclaim-ledger-versions` com o contrato do `api-reference.md`.** `reorg` rederiva o projeto de cada sessão pelo `cwd` com a mesma resolução do hook (002/T027), responde o plano e o resumo, e retira as páginas afetadas para o cemitério conforme a tabela do `temporal.md`; `dry_run` tem padrão `false`, como o doc define. `reclaim-ledger-versions` usa o reconhecimento de ledger da 002 pela entrada de conteúdo em memória, com `confirm`, `dry_run`, `drop_latest` e `compact`.
  - **Justificativa:** são as únicas fontes; pontos que o doc não fecha ficam para a revisão da feature.
  - **Alternativa Rejeitada:** adiar as duas (são rotas de manutenção listadas no núcleo).

- **Decisão (M-10): contratos de teste.** Cenários 1, 2, 3, 4, 7 e 8 do `design-memory-aging.md` §12; propriedades da fórmula do §3; keep-tokens por classe e corte de 48; guardas de seleção com página alterada no meio (multi-sessão); poda que poupa sessão despejada na mesma execução; lint com cada regra; agendador que não reexecuta após reinício.
  - **Justificativa:** constituição (invariante nomeada e teste multi-sessão no escritor).
  - **Alternativa Rejeitada:** só teste de integração da varredura inteira.

### O que não construir nesta feature
Deduplicação, banda de contradição, *dream pass*, contradição por LLM no lint, pré-filtro de entropia e `stage` do curador (021, 020 e 022); influência da retenção na busca (não implementada no doc).

## 4. Auditoria de Constituição

**Portões de Qualidade**
- [ ] Build cruzado, `go vet`, `go test -race` — M-02, M-05 e M-10.
- [ ] Teste multi-sessão no escritor e na supersessão — guardas de seleção (M-02) e tombamento (M-08).
- [ ] Gherkin — os cenários da spec viram o `.feature` da 004.
- [ ] Dependência nova — nenhuma.

**Restrições Arquiteturais**
- [ ] Grafo de imports — `domain/maintenance` sem I/O e sem importar componentes (M-01).
- [ ] INV-01 — chaves novas só em `internal/config`, com a validação da `cli.md` §2.2.
- [ ] INV-02 e INV-03 — aplicação em lote no escritor; tombstone e vínculos de entidade na mesma transação (M-02).
- [ ] INV-10 — reescritas pela escrita atômica da wiki.
- [ ] INV-13 — nenhum provedor; tudo zero-LLM.
- [ ] INV-16 — compactação e despejo supersedem; a versão anterior fica alcançável até a remoção definitiva.
- [ ] Resolução de escopo — rotas e tools exigem escopo existente e não criam.
- [ ] Mutações da wiki só pela camada `wiki` — M-01, M-07 e M-08.

**Regras de Negócio Transversais**
- [ ] Tombamento de páginas sumidas desligado por padrão — M-08.
- [ ] Zero-LLM — §1.

**Anti-Padrões Proibidos**
- [ ] Sem stub — `stage` do curador recusado com 422 em vez de vazio (M-07).
- [ ] Goroutine com `context` e sem fan-out ilimitado — M-05.
- [ ] "A confirmar" como decisão — M-08 (estado do tombamento) e M-09 (pontos do `reorg`).
