# Spec: Integração com outros agentes (hooks, MCP, scripts, transcrição, backfill, doctor)

## Metadados
- **ID:** 010
- **Status:** draft (revisada contra os docs em 2026-10-13 no nível de escopo; os contratos por agente são extraídos na fase 0 do `tasks.md`; nova revisão prevista quando a 002 terminar)
- **Autor:** phm-aguiar
- **Data:** 2026-10-13
- **Feature Anterior:** 002 (ingestão, spool, cliente de hook, `install-*` do Claude Code); 007 e 009 para credenciais por pessoa e OIDC nos hooks
- **Roadmap:** `specs/features/roadmap.md`
- **Aprovado:** false

## Propósito
> Para quem, o quê e por quê.

A promessa do produto é trocar de agente no meio da tarefa sem reexplicar o contexto. A 002 cobre só o Claude Code. Esta feature leva a captura, a entrega de handoff e o MCP aos demais agentes da matriz de suporte, com os scripts de compatibilidade, a ponte MCP com sessão, a captura por orquestrador externo, os adaptadores de transcrição somente leitura, o backfill do histórico anterior ao hook, o diagnóstico `doctor` e o encerramento manual de sessão para agentes sem evento de fim.

## Escopo

### Dentro do escopo
- **Instalação por agente** (`docs/support-matrix.md`, `docs/mcp-install.md`): `install-hooks --agent` e `install-mcp --client` com os nomes e aliases da matriz, os arquivos e formatos de cada agente (JSON, TOML, YAML impresso, plugins e extensões gerados), backup ao lado e escrita idempotente; regras comuns (`--apply`, Kiro exige HTTPS fora de loopback, `schema_version` do Muse).
- **Eventos por agente** (`docs/support-matrix.md` § captura): eventos instalados, onde o handoff é entregue (início de sessão, `additionalContext`, primeiro prompt no Kimi, primeiro `PostToolUse` no Grok, só a invocação 0 no Antigravity) e o evento de fim.
- **Normalização por agente** (`docs/lifecycle-event-ingestion.md` §3.3 a §3.5): marca de versão do Cursor, agentes de esquema fechado (nomes e ids de chamada por agente, desfecho do pós-ferramenta), caminho genérico (Gemini, Kimi, Devin), extensões (`extension`, `source_event`).
- **Captura comum** (`docs/support-matrix.md`): `--capture-mode`, captura da resposta do assistente com opt-in duplo (Claude Code, Codex, OpenCode 2; `capture_assistant` no servidor), descarte de subagente com conjunto de sessões para agentes que não marcam a sessão pai (limite 4096), cópia redundante do Cursor descartada, checkpoint de turno do OpenCode (`docs/lifecycle-event-ingestion.md` §5, §4.5 e §8.4).
- **`finalize-session`** para agentes sem evento de fim (Command Code, Kiro, Antigravity, ZCode, Pool).
- **Scripts de hook** POSIX e PowerShell com o mesmo contrato do hook nativo, seleção por `AI_MEMORY_HOOK_PLATFORM`, aviso de que não aplicam a política de captura (`docs/hook-scripts-and-container-deploy.md` §1 a §3, `docs/cli.md` §5.4).
- **Ponte `mcp-bridge`** com `X-Memory-Actor-Session-Id` e `install-mcp --session-aware` (Claude Code), recusando subir sem id de sessão (`docs/mcp-install.md`, `docs/auto-scope.md` § "Session-aware clients").
- **Captura por orquestrador externo** (`docs/external-lifecycle.md`): dono de captura declarado antes do lançamento, contrato público de eventos, campos de proveniência, chave de ingestão estável, lotes e confirmações, limites do spool nativo.
- **Adaptadores de transcrição somente leitura** (`docs/harness-adapters.md` §3): JSONL em arquivo, SQLite nativo, Antigravity; normalização; descoberta e verificação de sessão.
- **Backfill** (`docs/design-boot-backfill.md`): gatilho destacado no início de sessão por sentinela, comando `backfill` com `--dry-run`, `--force`, `--session`, `--max-sessions`, `--json`, `--quiet`, seleção, mapeamento de eventos, horário, lotes e retentativas, relatório JSON, portão de vazio, limites, `backfill_on_start` e `AI_MEMORY_BACKFILL_ON_START`; `repair-backfill-timestamps` e `POST /admin/repair-session-times`.
- **`doctor`** (`docs/cli.md` §8): compara sessões locais e capturadas por agente e mostra o comando de correção.
- **Integração restante** (`docs/cli.md` §8): `install-instructions`, `install-skills`, `setup-agent`, `uninstall` simétrico para todos os agentes; `backup-agents` e `restore-agents` (`docs/design-backup-agent-assets.md`).

### Fora do escopo (explicitamente)
- Lançamento gerenciado `ai-memory run` e workstreams (feature 013); esta feature entrega os adaptadores de transcrição que a 013 reaproveita.
- macOS e o app de barra de menus (fora do escopo inicial em `tech.md`).
- Imagem de contêiner, AUR e serviço de sistema (feature 015).

## Comportamento Esperado

### Cenário Principal (Happy Path)
1. O operador roda `install-hooks --agent codex --apply` e `install-mcp --client codex --apply`.
2. Uma sessão no Codex é capturada como no Claude Code da 002; no fim, a página de sessão e o handoff são gravados.
3. Em seguida, uma sessão no Claude Code no mesmo diretório recebe o handoff do Codex pelo `SessionStart`.
4. Num projeto novo com histórico local anterior, o primeiro início de sessão dispara o backfill destacado, que importa as sessões locais uma única vez.

### Cenários Alternativos
- **Agente sem evento de fim:** `finalize-session --agent <agente>` encerra a sessão.
- **Agente que descarta a saída do início de sessão** (Zero, Pool, Hermes): o handoff é recuperado pelo MCP.
- **Ponte com sessão sem id disponível:** a ponte não sobe, sem cair para o slot compartilhado.
- **Backfill com loja não vazia:** nada é importado, `skipped_non_empty`, sentinela gravada.
- **Script de compatibilidade selecionado:** o instalador avisa que a política de captura não se aplica.

## Edge Cases
- Antigravity: só a invocação 0 vira início de sessão; chamadas seguintes não consomem handoff.
- Kimi Code: handoff entregue no primeiro prompt, uma vez por sessão.
- Grok: handoff na saída do primeiro `PostToolUse`; sessão sem tool recupera pelo MCP.
- Backfill interrompido: a primeira sessão gravada torna a loja não vazia; retomada só com `--force`.
- Chaves de ingestão do backfill (`<sessão>:<evento>`) não passam na validação do servidor (o doc registra isso; a deduplicação vem do portão de vazio e da sentinela).

## Constraints
- **Segurança:** token nunca na linha de comando dos hooks; a sanitização do servidor vale também para o backfill; `ignore_paths` não se aplica ao backfill (o doc registra como requisito não implementado).
- **Plataformas:** Windows e Linux; a matriz marca o Windows nativo como experimental.
- **Tecnologia:** leitura de SQLite nativo de harness pelo `modernc.org/sqlite` em modo somente leitura.

## Critérios de Sucesso
- [ ] Cada agente com status "Suportado" na matriz instala hooks e MCP e passa por um teste de captura com fixtures de payload versionadas
- [ ] Os testes de paridade entre hook nativo e scripts (`docs/hook-scripts-and-container-deploy.md` §6) passam
- [ ] Os casos de teste de `docs/design-boot-backfill.md` § Testes passam, incluindo o canário de segredo
- [ ] `scripts/check.sh` e o lint passam sem apontamentos

## Abordagem Escolhida
Instalador orientado por tabela (um descritor por agente: arquivos, formato, eventos, aliases), sobre a escrita idempotente da 002. Normalização por agente como funções puras no `ingest`, com fixtures. Adaptadores de transcrição no `cli` (leem arquivos do usuário, fora da wiki). Backfill reaproveita o lote do spool da 002.

## Dependências
- 002: ingestão, spool, `install-*`, escrita idempotente; 007 e 009: credencial por pessoa e OIDC na ordem de bearer.
- `docs/support-matrix.md`, `docs/mcp-install.md`, `docs/hook-scripts-and-container-deploy.md`, `docs/external-lifecycle.md`, `docs/harness-adapters.md` §3, `docs/design-boot-backfill.md`, `docs/auto-scope.md`, `docs/design-backup-agent-assets.md`, `docs/lifecycle-event-ingestion.md`, `docs/cli.md`.

## Checklist de Prontidão
- [x] Propósito claro e sem ambiguidade
- [x] Escopo delimitado (dentro/fora)
- [x] Cenários cobrem happy path e alternativos
- [x] Edge cases identificados com comportamento esperado
- [ ] Constraints explícitas (contratos por agente vêm da fase 0)
- [x] Critérios de sucesso mensuráveis
- [x] Abordagem escolhida justificada
