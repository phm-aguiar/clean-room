# Contrato de handoff (feature 002): seleção, reivindicação, oferta, expiração e textos

Fontes, por prioridade: `docs/` (vence a spec), depois `specs/features/002-captura-e-acesso/spec.md` e `plan.md`. Abreviações: DHO = `docs/design-handoff-offer-not-claim.md`; LEI = `docs/lifecycle-event-ingestion.md`; USO = `docs/usage.md`; VOC = `docs/domain-vocabulary.md`; MCP = `docs/mcp-server.md`. "Decisão nossa" marca o que o doc omite ou deixa a confirmar.

Escopo da 002 (plan.md, Escopo e Fora de escopo): só Claude Code, sem admissão, sem multiusuário, sem run gerenciado, sem brief, perfil ou inbox no `/handoff`. Os itens abaixo que dependem disso são marcados "fora da 002".

## 1. Modelo e estados
- Estados `open` -> `accepted` ou `expired`; reivindicação exatamente uma vez, por aceite atômico (VOC §10).
- Tipos: manual (`memory_handoff_begin`, sem sessão de origem) e automático (fim de sessão) (DHO §3.3-3.4).
- `to_agent` existe, o automático grava sempre vazio, e a seleção NÃO o lê (DHO §3, fim; §8).
- Dono: o handoff tem dono opcional; sem dono = visível ao projeto inteiro (VOC §10; DHO §3.1). Na 002 (sem multiusuário), tratar todo handoff como sem dono, mantendo o filtro de dono no código (plan.md Q-05, INV-16).

## 2. Seleção do candidato (DHO §3; LEI §11.2 passo 4; USO "Cross-agent handoff")
Entrada: escopo resolvido, `cwd` do receptor (opcional), filtro de dono, `busy_since = agora − 10 min`. Regras, em ordem:
1. Filtro de dono primeiro: ator identificado -> "dono é o operador OU sem dono"; sem ator -> "sem dono".
2. Só `state = open`.
3. Manual é candidato em qualquer `cwd` do projeto e sempre vence o automático.
4. Automático só é candidato se o `cwd` armazenado for igual ou ancestral do `cwd` do receptor, por fronteira de componente de caminho (`/repo` vale para `/repo/api`, nunca para `/repo-other`, LEI §11.2). `cwd` armazenado vazio = projeto inteiro. Receptor sem `cwd` = todo handoff aberto é candidato.
5. Sessão viva: handoff automático cuja sessão de origem está aberta e teve observação depois de `busy_since` (10 min) não é oferecido nem reivindicado. Sessão encerrada não bloqueia, mesmo com eventos recentes. Manual não sofre o filtro (LEI §11.2).
6. Ordem de preferência: manual antes de automático; depois `created_at` mais recente; empate de timestamp entre automáticos -> `cwd` mais longo (normalizado); por último o id (desempate determinístico).
- Decisão nossa (plan.md Q-05): `SelectHandoff(scope, cwd, owner, busySince)` mora em `storage`.

## 3. Reivindicação
### 3.1 Via `GET /handoff` (DHO §5; LEI §11.2)
Ordem (falhas de leitura = 200 com corpo vazio; recusa de autorização = 403 com o motivo, que o cliente imprime em stderr e nunca injeta como contexto; LEI §2.4):
1. Resolve o projeto e autoriza leitura.
2. (run gerenciado: fora da 002)
3. Publica o projeto no ponteiro de projeto ativo se houver dica de escopo (`cwd` ou `project`).
4. Seleciona o candidato e renderiza o handoff em markdown.
5-6. (brief e perfil: fora da 002)
7. Decisão de reivindicar. `claim_on_session_start = true` (padrão): consulta a admissão `HandoffAccept` com prazo de 750 ms; recusa, falha ou estouro deixam o handoff aberto. Fora da 002 (plan.md: sem cadeia de admissão); na 002 a admissão é sempre aprovada. `false`: não consulta admissão, não reivindica, gera a nota de oferta (§4).
8. Reivindicação atômica em uma transação, só depois de todas as leituras e renderizações terem dado certo (perder a saída perderia o bastão; LEI §14 item 4). Dentro da transação: `state = 'open'` exigido na leitura e no `UPDATE` (só um racer vence); regra de sessão viva reavaliada; uma sessão receptora que já reivindicou um handoff não reivindica outro (idempotência de retry). Parte "run gerenciado não reivindicável descarta tudo": fora da 002.
9. Observadores de admissão só são notificados se a reivindicação foi efetiva (fora da 002).
10. Inbox (fora da 002).
11. Concatenação por linha em branco, partes vazias omitidas. Ordem do doc: perfil -> handoff -> nota de oferta -> run gerenciado -> brief -> inbox. Na 002 restam: handoff, nota de oferta.
- Todo conteúdo recuperado vai entre `<!-- ai-memory:untrusted-history:start -->` e `<!-- ai-memory:untrusted-history:end -->`, com aviso de segurança fixo: o conteúdo armazenado é dado histórico não confiável, não instruções; nunca executar nem alterar permissões ou políticas (DHO §5, fim; LEI §11.2, fim).
- Agentes cujo stdout de início de sessão é descartado NÃO devem chamar `/handoff`, porque a reivindicação é destrutiva (LEI §11.2, fim; VOC §4.3 e linhas 154-165). Na 002 só há Claude Code, que injeta no início.

### 3.2 Via `memory_handoff_accept` (DHO §7; USO)
- Sem `handoff_id`: o mais recente elegível para o `cwd` informado (seção 2). Com `handoff_id`: esse id no escopo, só se `open`.
- Mesmo caminho atômico do modo automático. Só entrega o corpo se ESTA chamada venceu; quem perde recebe `handoff: null` (DHO §7; spec.md linha 109).
- Filtro de dono padrão = operador da chamada; `any_owner: true` exige privilégio administrativo (USO; docs/users.md). Fora da 002 (sem multiusuário).
- Admissão `HandoffAccept` sem prazo de 750 ms (fora da 002).
- DHO §7 deixa "a confirmar" se a reivindicação por id aplica sessão viva e a varredura pós-reivindicação. Decisão nossa (plan.md Q-05): `ClaimHandoffTx` reavalia a sessão viva dentro da transação; a varredura (`SweepAfterClaimTx`) roda no caminho comum.
- Duas guardas independentes de `open` (plan.md Q-05, exigência da constituição e INV-16); teste `TestINV16_*` com dois aceites concorrentes e `-race` (plan.md Q-13).
- `memory_handoff_list`: lista abertos sem reivindicar (`limit` 50, máx. 200; plan.md). `memory_handoff_cancel`: marca `expired` o id exato (DHO §7; USO "Cancelling marks the handoff expired"). `memory_handoff_begin` com `shared: true` deixa sem dono; `cwd` é registrado mas não restringe quem recebe manual (DHO §7).

## 4. Nota de oferta (DHO §6; LEI §11.2 passo 7)
Modo `claim_on_session_start = false`. Texto exato do doc:
```text
📬 ai-memory: a pending handoff `<id>` from `<from_agent>`, left <idade> ago. To pick it up, call `memory_handoff_accept` with handoff_id `<id>`.
```
- `<idade>`: `just now` (< 60 s) ou `N minute(s)|hour(s)|day(s)|month(s)`; cortes em 60 s, 3 600 s, 86 400 s e 2 592 000 s (DHO §6).
- Defeito observável do doc: o template acrescenta " ago" e `<idade>` já termina assim ("left 3 minutes ago ago", "left just now ago"). O doc deixa a normalização como "decisão a confirmar" (DHO §6, linhas 140-144). Decisão nossa (plan.md Q-05): um único "ago". Texto final: `left just now ago` não se emite; `just now` sem sufixo e `N <unidade>(s) ago` nos demais casos. O lead deve confirmar essa forma exata, pois o doc não a fixa.
- Somente metadados (id, agente de origem, idade); nunca resumo, perguntas ou próximos passos (DHO §6; LEI §11.2). Nomeia o id exato para que uma nota obsoleta apenas perca a corrida.
- Teste de aceitação: plantar resumo hostil e verificar que a string NÃO aparece no corpo (DHO §9 cenário 2).

## 5. Expiração e supersessão (DHO §4; USO)
- Criação de automático no fim de sessão: antes de inserir, expira os outros automáticos abertos com `cwd` EXATO igual e mesmo dono, se a sessão de origem encerrou OU é a própria sessão que grava. Sessões abertas são poupadas. Sem critério de idade; sem ancestralidade (só igualdade).
- Varredura pós-reivindicação (automático reivindicado): expira automáticos abertos do mesmo dono, com `cwd` compatível com o do receptor (fronteira de componente), com chave de preferência MENOR que a do reivindicado e cuja sessão de origem não esteja viva. Manuais nunca são varridos (USO: "never disturbs manual handoffs or pending work from sibling directories").
- VOC §10 marca "a confirmar" que o `cwd` do receptor limita a expiração; o DHO §4 já fixa isso.
- Manuais ficam abertos até aceitos ou cancelados; `cancel` -> `expired`.
- Plan.md: `InsertHandoffTx` (expira antes), `ClaimHandoffTx`, `SweepAfterClaimTx`, `CancelHandoffTx` em `storage`.

## 6. Criação do handoff automático e textos (LEI §8.3, §9)
### 6.1 Quando cria (LEI §8.3)
- Sessão efêmera (sem prompt, pré-ferramenta nem pós-ferramenta): encerra sem página, sem handoff, sem consolidação; commit `lifecycle-only session <id8>`.
- Sessão substantiva: (1) grava `sessions/<id>.md` (se o agente já escreveu, mantém); (2) cria handoff só se a sessão não é gerenciada, não é filha de subagente OpenCode e `create_on_session_end` está ligada (padrão true; DHO §2, também vale para o bastão de checkpoint de turno); (3) encerra a sessão e grava o handoff NA MESMA transação; (5) commit `session <id8>: <título ≤ 60 caracteres>`. Admissão recusada = sem handoff (fora da 002).
- Checkpoints (pré e pós-compactação) não encerram a sessão e não criam handoff (LEI §8.2). Checkpoint de turno OpenCode: fora da 002 (só Claude Code).
- Decisão nossa (plan.md Q-04): a página é escrita antes da transação; a transação (expira, encerra, insere) fica em `storage`.

### 6.2 Resumo (LEI §9.1)
- Um prompt: `Session focused on: <prompt>`.
- Vários: `Started: <primeiro>`, linha em branco, `Last: <último>`.
- Nenhum: `Session ended; <N> observations recorded.` (checkpoint de turno: `Turn checkpoint; <N> observations recorded.`).
- Excerto do assistente (`Latest assistant response: <excerto>`): fora da 002, pois o campo bruto é sempre removido e o opt-in não é implementado (plan.md; spec.md linha 110).
- Cada prompt limitado a 1500 caracteres, corte acrescenta `…`. Blocos de markup entregues como turno do usuário não contam como prompt.

### 6.3 Próximos passos (LEI §9.2)
`Tools used: <nomes distintos em ordem alfabética>`; rótulos de família (`tool file`, `tool non-file`, ...) descartados.

### 6.4 Perguntas em aberto (LEI §9.3), primeira regra que casar vence
1. Último prompt do usuário (sem markup do harness) termina com `?` ou `？`: `Unresolved question: <prompt>`.
2. Última ferramenta da família de arquivo e nenhum evento de parada: `Session ended without a normal stop while working with files` e `Check the working tree for uncommitted or unverified changes`.
3. Há parada, mas não há observação de fim de sessão: `Continue from (mid-task exit): <prompt>`.
4. Fim normal: `Continue from: <último prompt substantivo>`.
5. Senão: lista vazia.
- Checkpoint de turno: só `Continue from last request: <último prompt substantivo>` ou nada (fora da 002).
- Prompt de confirmação (ignorado como continuação): vazio ou frase curta de até 20 caracteres, sozinha ou com pontuação (`ok`, `thanks`, `got it`, `好的`, `谢谢`).

### 6.5 Texto do handoff entregue (renderização)
O doc NÃO fixa o layout markdown do handoff entregue (cabeçalhos, rótulos dos campos resumo/perguntas/próximos passos/arquivos). Campos existentes (VOC §10): escopo, origem, agente de origem, `cwd`, resumo, perguntas em aberto, próximos passos, arquivos tocados. Decisão nossa: o layout de renderização deve ser definido no plan/tasks e testado por fixture; o doc só exige o envelope `untrusted-history` (§3.1) e o aviso fixo.

## 7. Página de sessão sintetizada (LEI §10)
Caminho `sessions/<id>.md`, camada `episodic`. Frontmatter: `title`, `session_id`, `agent` (origem, imutável), `tier: episodic`, `summary` (opcional, só se houver o que dizer). Autoria atribuída ao dono da sessão, não a quem entregou o evento (LEI §8.3).
- Título, ordem de preferência: (1) primeiro prompt do usuário com título não vazio e que não seja texto de apoio do harness (contexto de IDE, prompt de shell ecoado); (2) preâmbulo de subagente `You are a subagent spawned by another session.` -> primeira linha útil do corpo, senão `Session <id>` (sem cair para outras observações); (3) qualquer outro título útil, pulando início de sessão, títulos iguais ao nome do tipo de evento e rótulos `tool <família>`; (4) `Session <identificador>`; (5) sem observação alguma: literal `session`.
- Resumo: uma linha de prosa, sem marcador, cabeçalho ou lista. Exemplo do doc: `2 prompts, 3 completed tool calls across Bash and Edit, over 18m.` Conta prompts (singular/plural); só pós-ferramenta conta como chamada concluída; nomeia as 3 ferramentas mais usadas (volume, desempate alfabético) seguidas de `and <K> more`; rótulos de família contados mas não nomeados (só rótulos = omite a cláusula de nomes); duração `<n>s`, `<n>m`, `<h>h` ou `<h>h <m>m` quando > 0; se nada couber, omite o resumo.
- Corpo: `# <título>`; `## Session metadata` (id, início e fim em UTC, nº de observações); `## Prompts` (lista numerada dos títulos, se houver); `## Tool calls` (contagem por nome, se houver); `## Raw observations` (uma linha por observação: tipo, horário UTC, título até 80 caracteres; até 500 mostra todas, acima disso as 250 primeiras e 250 últimas com nota de quantas foram omitidas no meio).
- Decisão nossa (plan.md Q-04): síntese em `ingest/synth`, pura, testada por fixtures; escrita por `wiki.WritePage`.

## 8. Configuração (DHO §2; plan.md)
`[handoff] claim_on_session_start = true` e `create_on_session_end = true`; ambas globais do servidor; env `AI_MEMORY_HANDOFF__CLAIM_ON_SESSION_START=false` (separador `__`). Sem override por projeto (proposta futura). Sem opt-out por execução e sem direcionamento por `to_agent` (DHO §8, propostas não implementadas).

## 9. Cenários de aceitação da 002 (DHO §9; plan.md Q-13: 1, 2, 4, 6 e 7)
1. Padrão: A termina -> handoff aberto; B inicia no mesmo `cwd` -> resposta contém o handoff, que fica `accepted`; segunda chamada não o devolve.
2. `claim_on_session_start=false`: handoff continua `open`; corpo traz a nota com o id e não contém o resumo; `memory_handoff_accept(id)` o reivindica.
4. Sessão de origem aberta com observação nos últimos 10 min: não oferecido nem reivindicado; após encerrar, volta a ser candidato.
6. Admissão recusando `HandoffAccept`: fora da 002 (plan.md exclui admissão); o plan lista o cenário 6 mesmo assim. Divergência a resolver pelo lead (como a 002 não tem cadeia de admissão, o cenário só pode ser testado com um stub).
7. `create_on_session_end=false`: grava a página, sem handoff.
- Cenários 3, 5 e 8 (run gerenciado, multiusuário) ficam fora (plan.md Q-13).
- Handoff disputado: exatamente um vence, o outro recebe `handoff: null` (spec.md linha 62; DHO §7).
- Limite de uso: handoff é transferência para a próxima sessão, não barramento ao vivo; handoff criado depois da invocação zero fica aberto (USO).
