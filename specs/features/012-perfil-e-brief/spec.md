# Spec: Perfil entre projetos e brief de início de sessão

## Metadados
- **ID:** 012
- **Status:** draft (revisada contra os docs em 2026-10-13; nova revisão prevista quando as features anteriores terminarem)
- **Autor:** phm-aguiar
- **Data:** 2026-10-13
- **Feature Anterior:** 002 (`/handoff`, MCP, marcador), 004 (manutenção e agendadores), 007 (topologia de operadores, projetos restritos)
- **Roadmap:** `specs/features/roadmap.md`
- **Aprovado:** false

## Propósito
> Para quem, o quê e por quê.

O usuário repete as mesmas preferências em todo projeto ("sempre rode os testes antes", "prefira X a Y"). O perfil entre projetos colhe essas preferências dos prompts e das páginas curadas, sem LLM, admite só o que aparece em mais de um projeto ou é dito como geral, e entrega um resumo estável no início de cada sessão como **padrão a usar quando as regras do projeto e o usuário nada dizem**. O brief de início de sessão entrega, por opt-in do projeto, as páginas centrais dele. No servidor de um operador só, o perfil vem ligado por padrão.

## Escopo

### Dentro do escopo
- **Onde o perfil vive** (`docs/design-cross-project-profile.md` §4): `share` `auto` (global em operador único; desligado em multiusuário até `enabled = true`, que dá `user`), `global`, `workspace` (`_profile`), `user` (`_profile.<id>` restrito), `off`; projetos de perfil são reservados e nunca recebem captura.
- **Colheita sem LLM** (§5.1): projetos com `contribute` ligado, exceto perfis e `scratch`; marcas de até 2000 prompts e 2000 páginas por passada; fontes (prompts do usuário, páginas curadas em `_rules/`, `decisions/`, `gotchas/`, `procedures/`, sinais de stack com 13 tags de linguagem); regras de detecção com os marcadores em inglês e português; chave de tópico; candidato idempotente.
- **Convergência sem LLM** (§5.2): até 20 000 candidatos por escopo; agrupamento por Jaccard ≥ 0,5; projetos independentes (mesma citação em uma hora conta uma vez); barra de `min_projects` (2) ou geral; barra de dois operadores no perfil de equipe; entradas de stack; decisão mais recente; caminho `profile/<categoria>/<slug>.md`; casamento com página existente; tópicos esquecidos; páginas editadas à mão nunca reescritas.
- **Página de entrada** (§5.3): frontmatter (`title`, `kind: preference`, `summary`, `category`, `topic`, `applies_to`, `projects`, `first_seen`, `last_seen`, `generality`, `confidence`, `evidence`, `generated_by: profile-harvest`, `generated_sha`, `enforced_by` manual), corpo determinístico, fórmula de confiança, página gerenciada pelo `generated_sha`, escrita pelo caminho de escrita da wiki, ledger em `profile_entry_ledger`.
- **Entrega** (§5.4): resumo cercado no início do payload de `/handoff`, antes de handoff, contexto gerenciado, brief e aviso de caixa de entrada; seleção por tags de linguagem; ordem; orçamento `digest_max_bytes` (3000) e `baseline_max_bytes` (6000); linhas neutralizadas; byte a byte estável entre sessões; variante de equipe; casos em que é pulado. União na consulta para perfis `workspace` e `user`. `consume` respeitado.
- **Quando roda** (§5.5): trabalhador no `serve`, primeira passada 120 s depois do boot, acordado pelo fim de sessão com espera de 30 s, senão a cada hora; topologia lida a cada passada.
- **Configuração** (§7): `[profile]` com validação; `contribute` e `consume` no marcador, persistidos no projeto (`projects.profile_contribute`, `profile_consume`).
- **Comandos e rotas** (§8, `docs/mcp-server.md` §6.1): `profile status|list|show|forget|review|rebuild|apply` e `/admin/profile/*`; `apply` com alvo (`AGENTS.md`, `CLAUDE.md`), bloco gerenciado, `--dry-run`, `--remove`, `--user`; `memory_write_page` e `memory_delete_page` com `scope: "profile"`.
- **Brief de início de sessão** (`docs/marker-file.md` § "Session brief"): `[briefing] inject_on_session_start` e `max_chars` (4000, 1500..20000); páginas fixadas, `_rules/` e `_slots/` com corpo, títulos recentes sem corpo (até 10 e 24 centrais); envolvido como histórico não confiável; escopo só do próprio projeto; entrega no primeiro prompt para hosts que descartam a saída do início de sessão.

### Fora do escopo (explicitamente)
- Classificação e fusão por LLM (§6): feature 020; sem provedor, o caminho sem LLM é completo.
- Propostas não implementadas do doc (§12).

## Comportamento Esperado

### Cenário Principal (Happy Path)
1. Num servidor de operador único, o usuário diz "sempre rode os testes antes do commit" em prompts de dois projetos.
2. A passada colhe as duas frases, agrupa, admite pela barra de dois projetos e grava `profile/workflow/<slug>.md` em `default/_global`.
3. A próxima sessão em um terceiro projeto recebe o resumo no início do payload, com a linha `- statement (profile/workflow/<slug>.md)`.
4. `ai-memory profile apply` grava a entrada no bloco gerenciado do `AGENTS.md` do repositório.

### Cenários Alternativos
- **Projeto com `consume = false`:** sem resumo, sem `apply` (recusado), sem união na consulta.
- **Projeto com `contribute = false`:** nunca colhido.
- **Página de perfil editada à mão:** não é reescrita; aparece como editada à mão no `review`.
- **`forget` de uma entrada:** o tópico não volta até uma frase mais nova.
- **Servidor multiusuário com `share = auto`:** perfil desligado até `enabled = true`.

## Edge Cases
- Mesma mensagem enviada a vários projetos em uma hora conta como um projeto.
- Projeto sem stack ainda recebe todas as entradas (linha de base, com `baseline_max_bytes` e a dica de `profile apply`).
- Resumo maior que o orçamento: para na primeira entrada que não cabe e informa quantas faltaram.
- Texto armazenado tentando fechar a cerca do resumo ou o bloco gerenciado: neutralizado.

## Constraints
- **Determinismo:** a renderização não depende do relógio; o resumo é idêntico byte a byte até o perfil mudar.
- **Segurança:** candidatos só de prompts e páginas curadas, depois do sanitizador; perfil `user` privado; perfil compartilhado colhe só de projetos não restritos em multiusuário.
- **Tecnologia:** sem dependência nova; tabelas `profile_*` já na V01.

## Critérios de Sucesso
- [ ] Regras de detecção, agrupamento, admissão e confiança do §5 cobertas por teste com textos fixos
- [ ] Resumo estável byte a byte entre sessões com o mesmo perfil
- [ ] Fronteiras de segurança do §9 com teste adversarial cada
- [ ] `profile apply` com cada alvo e com `--dry-run` e `--remove`
- [ ] `scripts/check.sh` e o lint passam sem apontamentos

## Abordagem Escolhida
Colheita, detecção, agrupamento e renderização como funções puras em `domain`; o trabalhador no `cli` (boot do `serve`) lê pelo `storage` e grava pela `wiki`; o resumo e o brief entram na montagem do `/handoff` da 002 na ordem do doc.

## Dependências
- 002, 004, 007; `docs/cross-project-profile.md`, `docs/design-cross-project-profile.md`, `docs/marker-file.md`, `docs/mcp-server.md` §6.1, `docs/complementary-contracts.md` §7.

## Checklist de Prontidão
- [x] Propósito claro e sem ambiguidade
- [x] Escopo delimitado (dentro/fora)
- [x] Cenários cobrem happy path e alternativos
- [x] Edge cases identificados com comportamento esperado
- [x] Constraints explícitas
- [x] Critérios de sucesso mensuráveis
- [x] Abordagem escolhida justificada
