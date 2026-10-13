# Spec: Mensagens entre projetos (caixa de entrada e fila)

## Metadados
- **ID:** 011
- **Status:** draft (esqueleto gerado a partir dos docs em 2026-10-13; revisar contra os docs antes de aprovar)
- **Autor:** phm-aguiar
- **Data:** 2026-10-13
- **Feature Anterior:** 002
- **Roadmap:** `specs/features/roadmap.md`
- **Aprovado:** false

## Propósito
Um agente num projeto pede a um agente de outro projeto que faça algo, sem carregar o contexto do outro projeto: mensagem direta, reivindicada uma vez.

## Escopo

### Dentro do escopo
- Tools de mensagem `send`, `list`, `pop` e `cancel`: destinatário precisa existir (nunca cria), corpo sanitizado até 8000 caracteres e assunto até 200, corpo entregue como entrada não confiável (`docs/agent-messaging.md`, `docs/mcp-server.md` §3.3). Os nomes exatos das tools saem do doc na revisão.
- Comando `message` (`send`, `list`, `pop`, `cancel`) e rotas `/admin/messages*` (`docs/cli.md` §9, `docs/mcp-server.md` §6).
- Aviso de caixa de entrada no `GET /handoff`, só com a contagem (`docs/lifecycle-event-ingestion.md` §11.2).
- Tabela `agent_messages` (já na V01).

### Fora do escopo (explicitamente)
- Mensagens entre servidores diferentes.

## Fontes (docs)
- `docs/agent-messaging.md`, `docs/mcp-server.md` §3.3, `docs/lifecycle-event-ingestion.md` §11, `docs/cli.md`.

## Dependências
- 002 (MCP, `/handoff` e resolução de escopo).

## Comportamento Esperado
> A preencher na revisão contra os docs listados em "Fontes". Cenários Gherkin (sucesso, falha e borda) saem dos cenários de aceitação dos próprios docs, quando existirem.

## Critérios de Sucesso
- [ ] A preencher na revisão: cada regra numérica e cada cenário de aceitação dos docs com teste correspondente
- [ ] Testes de contrato das invariantes tocadas, com teste multi-sessão quando a constituição exigir
- [ ] `scripts/check.sh` e o lint passam sem apontamentos

## Checklist de Prontidão
- [ ] Propósito claro e sem ambiguidade
- [ ] Escopo delimitado (dentro/fora)
- [ ] Cenários cobrem happy path e alternativos
- [ ] Edge cases identificados com comportamento esperado
- [ ] Constraints explícitas
- [ ] Critérios de sucesso mensuráveis
- [ ] Abordagem escolhida justificada
