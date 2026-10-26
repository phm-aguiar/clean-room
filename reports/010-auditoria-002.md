# Relatório 010: auditoria da feature 002 (captura e acesso)

Data: 2026-10-25. Método: conferência pontual dos docs contra o código original (somente
leitura). Nenhum código-fonte foi transcrito; as regras foram traduzidas para tabelas e prosa.

## Arquivos corrigidos

| Arquivo | Alteração |
|---|---|
| docs/lifecycle-event-ingestion.md | §1 ordem de admissão + lista `[ingest]`; exceção de processamento do lote dentro da requisição; §2 rota `GET /recap`; §2.2 valores de `outcome` e regras de omissão; §2.3 motivo `ingest drop list`; nova §2.5 `GET /recap` |
| docs/mcp-server.md | 23→24 ferramentas; `memory_restore_page`; atomicidade de `memory_message_pop`; `memory_message_cancel` sem id; recusa de escopo inferido |
| docs/api-reference.md | 24 ferramentas; `GET /recap` no mapa de rotas |
| docs/mcp-install.md | 24 ferramentas |
| docs/cli.md | redação de segredos; encerramento (5 s por espera, WAL); pasta `hooks`; renovação OIDC; passo 9a "reload context"; drenagem no processo para o Zero; trava de capacidade da fila |
| docs/design-boot-backfill.md | limite de corpo do lote (10 MiB); `backfill_on_start` ausente do modelo padrão |
| docs/external-lifecycle.md | término de sessões filhas não é inferido (confirmado) |
| specs/features/002-captura-e-acesso/spec.md | seção "Correções da auditoria" |

## Conceito errado e correção

| Antes | Correção |
|---|---|
| A rota `/hook` só tinha `/hook`, `/hook/batch` e `/handoff` | Há também `GET /recap`, somente leitura, que nunca reivindica handoff |
| Aliases checados antes do excerto do assistente | O excerto (e o consumo da marca) vem antes; a lista `[ingest]` fica entre rascunho do Cursor e subagente |
| "O processamento pesado ocorre depois da resposta" (geral) | Vale só para `/hook`; o lote processa cada item dentro da requisição |
| 23 ferramentas MCP | 24 (`memory_restore_page`, somente administrador) |
| `cancel` de mensagem retira uma mensagem pelo identificador | Sem `message_id`, cancela todas as pendentes enviadas pelo projeto |
| Atomicidade do `pop` "a confirmar" | Troca condicional numa transação; quem perde recebe `{message: null}` |
| Fila: `<data_dir>/hook-spool.capacity.lock` | No original o nome sai com dois pontos (`hook-spool..capacity.lock`); a espera de 50 ms nunca descarta o evento |

## Principais equívocos

1. Falta de uma rota inteira (`/recap`) e do fluxo do cliente "reload context" (Claude Code, Codex,
   Grok, OpenCode).
2. Uma ferramenta MCP inteira não estava documentada (`memory_restore_page`).
3. Semântica de `memory_message_cancel` sem argumento (cancelamento em massa) não estava
   documentada, e há um risco real de reimplementar errado.

## Segunda rodada (progresso por doc)

| Doc | Resultado |
|---|---|
| mcp-server.md §4–§6 | **Precisava de ajustes, corrigido.** O 401 é texto `auth required` + `WWW-Authenticate: Bearer realm="ai-memory", error="invalid_token"`; o doc dizia JSON. Mensagens 400 do proxy registradas literalmente. KDF: 2 simultâneos, 32 pendentes, parâmetros Argon2id. `GET /admin/metrics` não estava documentado (agora tem tabela de famílias e histogramas). Limites de login (10/60 s, 4096 IPs, 64 KiB, sessão de 7 dias) conferidos. |
| consolidation.md | Constantes conferidas (48 tokens, 2–128 bytes, L1 500, lote de poda 5000, limiar de 30 dias e 0,6/λ, 25/7/30 do curador). Faltava a deduplicação A3 (opt-in, embeddings), agora registrada. |
| design-page-ingestion-windows.md | **Fiel.** Conferidos: candidatos max(4×limite, 20) (limite 0 → 0), RRF k=60, autoridade 0,55–1,50 com tetos 0,65 e 0,80, lote de preenchimento 5000. O "a confirmar" restante é sobre um teste entre motores, não sobre comportamento. |
| api-reference.md (restante) | Limites de corpo e de login conferidos. Corrigido o padrão do bootstrap (servidor 50 000 / CLI 150 000). |
| programmatic-memory.md | Limites conferidos (25 escopos, 10 fixadas, handoff e mensagem). Código `-32600` explicitado. |
| usage.md | "Reload context" acrescentado na recuperação de compactação. |
| cookbook.md | "a confirmar" resolvido: não há importador de vault OKF/Obsidian. |
| mcp-install.md | Resolvidos os itens que o código responde: merge do Devin sob `hooks` e workstreams do Command Code via `run`. Os demais "a confirmar" são comportamento de produtos de terceiros. |
| auto-scope.md | Pegada de memória: sem limite de tamanho de chave no código (recomendação registrada); TTL de 1 h. |
| external-lifecycle.md | Os dois "a confirmar" resolvidos. |
| design-decisions.md, design-hindsight-borrowings.md, desktop/plan.md | Contagem 23 → 24 (e `memory_restore_page` na tabela e na lista). |

## Terceira rodada

| Item | Resultado |
|---|---|
| auto-scope.md:115 | Lista completa no doc. Criam escopo: `memory_write_page` (e a escrita de `memory_restore_page` no escopo já resolvido), `memory_handoff_begin`, o ingresso de hooks e `POST /workstream/runs`. As demais ferramentas só buscam, com acesso de leitura ou de escrita conforme a mutação. |
| Prioridade de handoff (design-decisions.md) | Verificada. Ordem da chave: manual > mais recente > especificidade (só entre automáticos com o mesmo instante) > identificador. O filtro de dono vem antes da escolha. |
| mcp-install.md | Caminhos padrão de todos os clientes conferidos com o instalador. Corrigida a regra MSIX do Claude Desktop: precisa haver exatamente um pacote com configuração, senão o comando para com erro; a mensagem está registrada, e as variáveis de ambiente têm precedência. O que restava como "a confirmar" virou "depende do produto externo"; para o Kimi, o ai-memory não grava `timeout`. |
| usage.md / cookbook.md | Flags e subcomandos conferidos com a CLI (todos existem). Corrigido o caminho da wiki: as pastas usam os UUIDs do workspace e do projeto, e há um único repositório git na raiz. |
| capture-policy.json | Sincronizado com os vetores de teste originais (os dados são idênticos; só as notas foram reescritas). Seções e formatos novos descritos em test-fixtures.md. |

## Status

**Fiel** nos pontos conferidos. Não há mais "a confirmar" respondível pelo código nos docs da
feature 002. O que resta depende de produtos externos e está marcado como tal. Os docs conferidos estão fiéis nos pontos
verificados. Ainda não foram conferidos linha a linha: as seções por cliente de mcp-install.md,
que dependem sobretudo de produtos de terceiros, e a tabela de exemplos de usage.md e de
cookbook.md além dos pontos citados.

## Pendências

- cli.md §5.4: timeouts de rede e códigos de saída dos scripts `.sh`/`.ps1` do pacote de hooks. O
  pacote não está na cópia do código original (só o lançador do plugin, que não faz rede: sem
  binário, consome a entrada, responde `{}` ou uma `systemMessage` no `session-start` e sai com 0;
  com binário antigo que sai com 2, tenta de novo com `AI_MEMORY_SERVER_URL` ou
  `http://127.0.0.1:49374`).
- Itens marcados "depende do produto externo" (mcp-install.md, auto-scope.md, usage.md): só se
  resolvem com a documentação de cada cliente.
- Os docs auto-scope.md, external-lifecycle.md e usage.md estão em inglês; o idioma não foi alterado.
