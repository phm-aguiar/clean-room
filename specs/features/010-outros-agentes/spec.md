# Spec: Integração com outros agentes (hooks, MCP, scripts, transcrição, backfill, doctor)

## Metadados
- **ID:** 010
- **Status:** draft (revisada contra os docs em 2026-10-13 no nível de escopo; **auditada contra o comportamento do original em 2026-10-19**: backfill, ponte `mcp-bridge`, `finalize-session`, `doctor`, token em arquivo, matriz de suporte (agentes novos Droid, CodeBuddy, IBM Bob, Prime Agent, DSH, GrizzyBot; Muse com hooks e ponte), formatos de `install-mcp` e de `install-hooks` por agente, normalização por agente, lista de descarte do servidor, motor da política de captura, adaptadores de transcrição (14, com Copilot CLI) e spool corrigidos nos docs. **Verificação parcial, a revisar:** analisadores de transcrição de Claude Code, Codex, Pi/OMP, Kimi, Command Code, Kiro e Grok (só as regras já descritas no doc foram conferidas), scripts de hook POSIX/PowerShell e o diário de recuperação do spool, que pertence à 013)
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
- **Agentes adicionais ao original:** Factory Droid, Muse Code (hooks e MCP), CodeBuddy Code, IBM Bob, Prime Agent (só MCP), DeepSeek Harness (só impressão) e GrizzyBot (externo); regras de fusão que preservam hooks de terceiros e reconhecimento de entrada própria (`docs/install.md` § "How `install-hooks --apply` merges…").
- **Lista de descarte do servidor** (`[ingest] drop_tool_use_id_prefixes` / `drop_tool_names`, `docs/lifecycle-event-ingestion.md` §4.6) e **motor de decisão da política de captura** (§4.4a), inclusive reinspeção no servidor.
- **`finalize-session`** para agentes sem evento de fim (Command Code, Kiro, Antigravity, ZCode, Pool, IBM Bob).
- **Scripts de hook** POSIX e PowerShell com o mesmo contrato do hook nativo, seleção por `AI_MEMORY_HOOK_PLATFORM`, aviso de que não aplicam a política de captura (`docs/hook-scripts-and-container-deploy.md` §1 a §3, `docs/cli.md` §5.4).
- **Ponte `mcp-bridge`** em dois modos (`docs/mcp-install.md` § Claude Code): **com sessão** (`X-Memory-Actor-Session-Id`; `--agent claude-code|muse` escolhe a única variável lida; recusa subir sem id) e **sem sessão** (`--no-session-id`, para os demais clientes); conecta ao servidor antes de servir, responde "método não encontrado" antes do `initialize`; `install-mcp --session-aware` (só Claude Code), entrada fixa do Muse (`mcp-bridge --agent muse`, mapa `mcpServers` ou legado `mcp_servers`, recusa com os dois) e roteamento do token em arquivo (`docs/users.md` § "Local root bearer token file"): Claude Code → ponte com sessão; Codex, Prime Agent e Droid → referência por nome a `AI_MEMORY_AUTH_TOKEN`; ZCode → recusa; demais → ponte sem sessão no formato do cliente.
- **`auth enable`** e o token raiz gerado pelo `init` em instalação nova (`docs/users.md` § "Local root bearer token file"): o `install-*` reaproveita esse caminho; o restante pertence à 007.
- **Captura por orquestrador externo** (`docs/external-lifecycle.md`): dono de captura declarado antes do lançamento, contrato público de eventos, campos de proveniência, chave de ingestão estável, lotes e confirmações, limites do spool nativo.
- **Adaptadores de transcrição somente leitura** (`docs/harness-adapters.md` §3): JSONL em arquivo, SQLite nativo, Antigravity; normalização; descoberta e verificação de sessão.
- **Backfill** (`docs/design-boot-backfill.md`): gatilho destacado no início de sessão por sentinela **e marcador "em andamento" com trava de arquivo e token de dono (30 min para tomar um órfão)**, comando `backfill` com `--dry-run`, `--force`, `--session`, `--max-sessions`, `--json`, `--quiet` (mais as opções ocultas `--auto`, `--inflight-claim`, `--server-url`, `--server-profile`), **portão de admissão do modo de captura (`skipped_not_admitted`)**, seleção, mapeamento de eventos **com a política de captura/`ignore_paths` aplicada pelo cliente**, horário, lotes (256 itens e 8 MiB) com validação da confirmação item a item e retentativas, **chaves de ingestão `recovery_<55 hex>` que o servidor aceita e deduplica**, relatório JSON, portão de vazio (consulta de 30 s), limites, `backfill_on_start` e `AI_MEMORY_BACKFILL_ON_START`; `repair-backfill-timestamps` e `POST /admin/repair-session-times`.
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
- Chaves de ingestão do backfill (`recovery_` + 55 hexadecimais do SHA-256 de intervalo, sessão, tipo e id do evento) são aceitas pelo servidor: reenviar o mesmo evento (por exemplo `--force`) não duplica.
- Diretório não admitido pelo modo de captura (lista de permissão sem marcador): nada importado, servidor não contatado, sem sentinela.
- Rajada de inícios de sessão no mesmo diretório: um só backfill automático (marcador "em andamento").
- Ponte sem id de sessão disponível: não sobe (modo com sessão); `--no-session-id` ignora `CLAUDE_CODE_SESSION_ID` mesmo presente.

## Constraints
- **Segurança:** token nunca na linha de comando dos hooks nem em configuração de harness quando vem de arquivo; a sanitização do servidor vale também para o backfill; **`ignore_paths` e o modo de captura se aplicam ao backfill, pelo cliente** (tabela de decisão do hook ao vivo).
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
