# plan.md: Plano de Implementação Técnica — Perfil entre projetos e brief (012)

> Plano gerado em 2026-10-13, antes de a 002 ser executada. Revisar quando as features anteriores terminarem.

## 1. Metadados do Plano
- **Stack Tecnológico:** a da 002, sem dependência nova. `crypto/sha256` para `generated_sha`.
- **Feature Fonte:** `specs/features/012-perfil-e-brief/spec.md`
- **Escopo:** perfil (colheita, convergência, página, entrega, trabalhador, comandos, rotas, `apply`) e brief de início de sessão. Sem LLM.
- **Fontes de contrato:** `docs/design-cross-project-profile.md` (fonte principal), `docs/cross-project-profile.md`, `docs/marker-file.md`, `docs/mcp-server.md` §6.1.

## 2. Design de Contratos e Fronteiras
- **Página de entrada:** `docs/design-cross-project-profile.md` §5.3.
- **Configuração:** `[profile]` (§7) e `AI_MEMORY_PROFILE__*`; marcador `[profile] contribute/consume` e `[briefing]`.
- **Rotas:** `GET /admin/profile/status|list|review|apply`, `POST /admin/profile/rebuild`.
- **Esquema SQL:** sem migração; `profile_candidates`, `profile_entry_ledger`, `profile_harvest_marks`, `projects.profile_contribute/consume` já na V01.

## 3. Decisões Arquiteturais e Justificativas

- **Decisão (P2-01): núcleo puro em `domain/profile`.** Detecção de frases, chave de tópico, agrupamento por Jaccard, admissão, confiança, caminho e renderização da página e do resumo são funções puras, testadas com textos fixos.
  - **Justificativa:** o doc é algorítmico e determinístico; precisa de teste sem banco.
  - **Alternativa Rejeitada:** lógica dentro do trabalhador.

- **Decisão (P2-02): trabalhador no `serve` com wake-up.** Uma goroutine com `context`: primeira passada em 120 s, wake-up no fim de sessão com espera de 30 s (coalescendo), senão a cada hora; lê a topologia a cada passada; falha num projeto ou escopo vai para o relatório e a passada continua.
  - **Justificativa:** §5.5.
  - **Alternativa Rejeitada:** passada síncrona no fim de sessão (atrasaria o hook).

- **Decisão (P2-03): escrita pelo caminho da wiki.** Entradas gravadas pelo `WritePage`, com sanitização e admissão; reescrita só de páginas gerenciadas (`generated_sha` igual ao corpo atual).
  - **Justificativa:** §5.3 e constituição (mutações só pela `wiki`).
  - **Alternativa Rejeitada:** escrita direta no `storage`.

- **Decisão (P2-04): ordem do payload do `/handoff`.** A montagem da 002 passa a concatenar, nesta ordem: resumo do perfil, handoff, nota de oferta, contexto gerenciado (013), brief, aviso de caixa de entrada (011); partes vazias omitidas.
  - **Justificativa:** `docs/design-handoff-offer-not-claim.md` §5 passo 11 e §5.4 do design do perfil (resumo primeiro).
  - **Alternativa Rejeitada:** resumo depois do handoff (o doc corrigiu isso na revisão 2.6.1).

- **Decisão (P2-05): `apply` com bloco gerenciado e backup.** Alvo pela ordem do doc; bloco entre linhas de comentário isoladas; escrita atômica; backup do arquivo anterior no diretório de dados; recusa com `consume = false`.
  - **Justificativa:** §8.
  - **Alternativa Rejeitada:** reescrever o arquivo inteiro.

- **Decisão (P2-06): contratos de teste.** Regras de detecção com frases em inglês e português; agrupamento e admissão; tópico esquecido; página editada à mão; estabilidade byte a byte do resumo; fronteiras do §9 com teste adversarial; brief com orçamento e neutralização.
  - **Justificativa:** §9 pede teste por fronteira.
  - **Alternativa Rejeitada:** só integração.

### O que não construir nesta feature
Classificação e fusão por LLM (020), propostas do §12.

## 4. Auditoria de Constituição
- [ ] Build cruzado, `go vet`, `go test -race` — P2-06.
- [ ] INV-01 — `[profile]` e `AI_MEMORY_PROFILE__*` só em `internal/config`.
- [ ] INV-06 — candidatos depois do sanitizador; escrita pelo `WritePage`.
- [ ] INV-13 — caminho completo sem LLM.
- [ ] INV-16 — perfil compartilhado legível por todos; perfil `user` restrito (acesso por projeto da 007).
- [ ] Mutações da wiki só pela camada `wiki` — P2-03.
- [ ] Goroutine com `context` — P2-02.
- [ ] Sem dependência nova.
