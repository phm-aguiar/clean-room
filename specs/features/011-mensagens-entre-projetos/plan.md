# plan.md: Plano de Implementação Técnica — Mensagens entre projetos (011)

> Plano gerado em 2026-10-13, antes de a 002 ser executada. Revisar quando as features anteriores terminarem.

## 1. Metadados do Plano
- **Stack Tecnológico:** a da 002, sem dependência nova.
- **Feature Fonte:** `specs/features/011-mensagens-entre-projetos/spec.md`
- **Escopo:** fila de mensagens, quatro tools, CLI, rotas administrativas, admissão, aviso de caixa de entrada, contagem no briefing.
- **Fontes de contrato:** `docs/agent-messaging.md`, `docs/mcp-server.md` §3.3 e §6, `docs/admission-webhooks.md` §2.4, `docs/design-per-project-authz.md`.

## 2. Design de Contratos e Fronteiras
- **Tools e CLI:** conforme `docs/agent-messaging.md` (tabelas de tools e de opções).
- **Rotas:** `POST /admin/messages/send`, `/admin/messages/pop`, `/admin/messages/cancel`, `GET /admin/messages`.
- **Esquema SQL:** sem migração; `agent_messages` já na V01.

## 3. Decisões Arquiteturais e Justificativas

- **Decisão (M2-01): reivindicação atômica no `storage`.** `pop` é uma transação no escritor: escolhe a pendente mais antiga (ou a de id exato) da caixa do destinatário e faz `UPDATE ... SET state = 'claimed', claimed_at, claimed_by_* WHERE id = ? AND state = 'pending'`; zero linhas afetadas devolve `null`. Capacidade: contagem de pendentes na mesma transação do envio.
  - **Justificativa:** "exatamente um vence" (`docs/agent-messaging.md` § Pop), mesmo padrão das duas guardas do handoff (INV-16).
  - **Alternativa Rejeitada:** trava em memória.

- **Decisão (M2-02): cortes por caminho.** O MCP corta em caracteres com marcador de truncamento; o HTTP em bytes sem marcador; o armazenamento em 16 KiB. A sanitização vem antes dos cortes (INV-06).
  - **Justificativa:** tabela de limites do doc (a divergência entre caminhos é documentada).
  - **Alternativa Rejeitada:** unificar os cortes (mudaria o contrato observável).

- **Decisão (M2-03): admissão em todos os caminhos.** `message_send`, `message_pop` e `message_cancel` passam pelo "decidir, agir, notificar" da cadeia da 008, inclusive nas rotas administrativas (decisão nossa sobre o "a confirmar" do doc).
  - **Justificativa:** um caminho sem admissão permitiria contornar a política pela CLI.
  - **Alternativa Rejeitada:** só no MCP, como o doc descreve hoje.

- **Decisão (M2-04): proveniência com agente e sessão quando conhecidos.** O envio grava `from_agent` e `from_session_id` do contexto do ator; `other` e `NULL` quando desconhecidos (decisão nossa).
  - **Justificativa:** o doc registra o "sempre `other`" como "a confirmar se é intencional"; as colunas existem.
  - **Alternativa Rejeitada:** manter `other` sempre (perde a proveniência que o aviso de segurança pede para avaliar).

- **Decisão (M2-05): contratos de teste.** Cada regra do doc; corrida de `pop` com `-race`; aviso sem texto com corpo hostil; isolamento (nenhuma consulta junta terceiro projeto); recusa de corpo vazio.
  - **Justificativa:** constituição (multi-sessão em reivindicação).
  - **Alternativa Rejeitada:** só caminho feliz.

### O que não construir nesta feature
Mensagens entre servidores.

## 4. Auditoria de Constituição
- [ ] Build cruzado, `go vet`, `go test -race` — M2-05.
- [ ] Teste multi-sessão em reivindicação — M2-01.
- [ ] INV-06 — M2-02.
- [ ] INV-16 — reivindicação única (M2-01); atribuição nunca vira filtro.
- [ ] Resolução de escopo — remetente e destinatário nunca criados; falha fechada.
- [ ] Autorização por capacidade e projeto — leitura e escrita conforme a spec.
- [ ] Sem stub; sem dependência nova.
