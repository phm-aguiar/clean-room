# Auditoria 008: feature 013, parte B (comandos e mecanismos de apoio)

Data: 2026-10-24. Escopo: fechar as lacunas declaradas na auditoria 002 para a 013 (`show`, `finalize-session`, `backfill`, spool/quarentena/drenagem, `recover`, jail, autorização por projeto, handoff, lease/purga e concorrência). Fonte da verdade: o ai-memory em Rust (somente leitura). Nenhum código Go foi tocado; `plan.md` e `tasks.md` da 013 não foram recriados; nada foi commitado. Nenhum trecho de código Rust foi copiado: tudo foi traduzido para regras e tabelas.

## Status por área

| Área | Status | Observação |
|---|---|---|
| `show` (menus e saídas) | Fiel (era Ainda incompleta) | Contrato completo acrescentado a `docs/managed-workstreams.md`. |
| `finalize-session` | Fiel | O doc já estava correto; só detalhes menores acrescentados. |
| `backfill` / importação de intervalo exato | Fiel | `docs/design-boot-backfill.md` conferido contra o código inteiro, sem divergências. A parte do intervalo exato está em `downtime-recovery.md` § 3.1. |
| Spool, quarentena e drenagem | Fiel (era Incompleta, com 3 erros) | Novo § 4 de `docs/downtime-recovery.md`; `docs/cli.md` § 5.3 corrigido. |
| `ai-memory recover` | Fiel (precisava de ajustes) | Revalidado linha a linha; § 3.1 novo; um defeito do original registrado. |
| Jail / yolo | Fiel após ajustes (tinha 2 omissões de segurança e 1 lista errada) | `docs/design-yolo-safety-ai-jail.md` e seção Jail de `managed-workstreams.md`. |
| `design-per-project-authz.md` | Fiel | Regras de dono/escrita, ordem e degradação confirmadas; lacunas 1 e 3 parcialmente resolvidas. |
| `design-handoff-offer-not-claim.md` | Precisa de ajustes (feitos) | 3 correções; transação de reivindicação confirmada. |
| Lease, heartbeat, expiração, purga (003) | Fiel após 1 correção | Campo `active_workstreams` não existe. |
| Concorrência | Ainda incompleta no doc, agora Fiel | Seção nova; achado importante sobre a rota `finish`. |
| Spec 013 | Precisa de ajustes (feitos) | Bloco "Segunda rodada" e tarefas A10 a A13 acrescentados; o DAG precisa ser regerado (T005). |

## Arquivos corrigidos

| Arquivo | Mudança |
|---|---|
| `docs/downtime-recovery.md` | § 1 (verificação de links, trava bloqueante, mensagens do diário), tabela de entrada, § 2 (mensagens literais), novo § 3.1 (ordem real, falhas por entrada, texto humano, quarentena ao fim da entrada), novo § 4 (spool: arquivos, travas, enfileirar, drenagem, desfechos, quarentena, Windows), novo § 5 (concorrência). |
| `docs/cli.md` (CRLF preservado) | § 5.3: `auth_mode`, classificação de 4xx, 408, espera da trava do drenador; § 9.1: efeito local de `finalize-session`. |
| `docs/managed-workstreams.md` | Contrato detalhado de `show`; textos do seletor de `resume`; seção Concorrência; jail: 14 variáveis, valores resolvidos, recusa de token da configuração, "a confirmar" final reclassificado. |
| `docs/design-yolo-safety-ai-jail.md` | Lista de variáveis, recusa de token, `BWRAP_BIN`, pré-voo, JSON exato do `--settings`, mensagens literais, tabela de rótulos/notas/versões (§ 5.2.1), resumo, checklist, § 9.1 (resolução dos "a confirmar"). |
| `docs/design-handoff-offer-not-claim.md` | `any_owner`, varredura pós-reivindicação, regras da sessão receptora. |
| `docs/design-per-project-authz.md` | Lacunas 1 e 3 e mensagem de usuário inexistente. |
| `docs/mcp-server.md` | `all_owners` para `any_owner` nas 3 ferramentas de handoff. |
| `docs/lifecycle-ops.md`, `specs/features/003-ciclo-de-vida/spec.md` | Corpo do 409 da purga. |
| `specs/features/013-workstreams-gerenciados/spec.md` | "Segunda rodada" em "Correções da auditoria" e A10 a A13. |

## Equívocos encontrados no que existia

1. **`cli.md` § 5.3, formato em disco:** `auth_mode` grava `static`, `oidc`, `none` (o doc dizia `static_token`, `oidc_at_drain`, `anonymous`). Quem lê spool de outra versão quebraria.
2. **`cli.md` § 5.3, descarte:** só o `403` descarta o item de imediato; o doc dizia "`4xx` permanentes exceto 401, 408, 429". `400`, `408`, `413`, `422` cobram tentativa; `408` consome tentativa (o doc dizia que não).
3. **`design-yolo-safety-ai-jail.md` § 2.3:** a lista de variáveis repassadas tem 14 nomes; faltava `AI_MEMORY_BASE_PATH`. Faltavam também os valores resolvidos da configuração (URL do servidor e caminho base são definidos no processo do ai-jail).
4. **Omissão de segurança no jail:** `--jail` (e a oferta aceita) é recusado se o token do servidor veio do `config.toml` e `AI_MEMORY_AUTH_TOKEN` não está no ambiente. Não estava documentado.
5. **`--true-yolo`:** o JSON exato do `--settings` não estava no doc.
6. **`design-handoff...` § 7:** o argumento da ferramenta é `any_owner`, não `all_owners` (o nome errado também estava em `mcp-server.md`); a varredura pós-reivindicação da ferramenta poupa **toda** sessão de origem aberta (o doc dizia "últimos 5 minutos", janela que não existe); faltavam as regras da sessão receptora (existir, mesmo escopo e agente, não encerrada, exceção do Grok).
7. **Purga (003):** o 409 devolve só `{"error": "…"}`; o campo `active_workstreams` citado em `lifecycle-ops.md` e na spec 003 não existe.
8. **Rota `finish` não é atômica:** lê o estado, grava o segmento bruto e só depois chama o escritor. Um `cancel` nessa janela produz `400 invalid state: managed run <id> is expired` e deixa um segmento órfão; o doc só descrevia o 409 `managed run is not recoverable`.
9. **`recover`:** faltavam a ordem real, as mensagens de cada falha, o texto humano, a exceção "já entregue" (marcador `.completed` converte a falha em `recovered-via-spool`) e a diferença da chave de ingestão (o id da entrada do diário entra como primeiro campo).
10. **Defeito do original:** o leitor de nomes de harness do diário não conhece `copilot`; uma entrada do Copilot falharia com `journal names unsupported harness 'copilot'`, embora o lançador a grave.
11. **Nome da trava de capacidade:** o original monta `hook-spool..capacity.lock` (ponto duplicado); `cli.md` dizia `hook-spool.capacity.lock`. Interno, sem outro leitor; registrado.

## "A confirmar" do jail (`design-yolo-safety-ai-jail.md` § 9.1)

- **Resolvidos pelo Rust:** `--no-save-config` existe desde o ai-jail 2.4.1 (o teste com ai-jail real do original afirma isso); `--network` (padrão desligado), Docker, SSH, Tailscale e imagens desligados por padrão (texto de ajuda 2.4.1 usado como fixture); caminho fixo `~/.config/gh` (ajuda 2.5.0) e o ai-memory nunca consulta `GH_CONFIG_DIR`/`XDG_CONFIG_HOME`; tabela de versões por toggle.
- **Externos (continuam suposições documentadas):** precedência de `--settings` e regras `ask`/`deny` do Claude Code; hostname `ai-sandbox`; prefixo `(jail) ` no macOS; arquivo `.ai-jail` de projeto não habilita credenciais; rede no macOS; efeito real do `--dry-run`.

## Pendências (decisão ou verificação)

1. **Decisão do usuário:** o Go deve aceitar `copilot` no diário de recuperação (recomendado), em vez de reproduzir o defeito do original.
2. **Decisão do usuário (segurança, Windows):** o original não restringe ACL do spool nem de seus arquivos (herdam o diretório de dados); o diário recebe `icacls`. Itens do spool podem trazer o token estático embutido. Recomendação: aplicar a mesma restrição do diário.
3. **Requisito de implementação Go no Windows:** abrir arquivos do spool e do diário com `FILE_SHARE_DELETE` (o padrão da biblioteca-padrão do Go não inclui) ou repetir rename/remoção nos erros 32 e 5. O original não trata o 32 porque o ambiente de execução dele abre com compartilhamento total. Tratar como ocupada a violação de trava 33. Recusar também junções (o original trata como link). Comparar caminhos de checkout em forma normalizada (o canônico do original tem prefixo `\\?\`).
4. **Lacunas de autorização ainda abertas** (`design-per-project-authz.md`): "page evidence counts" e "aplicação estrutural" não foram verificadas; consolidação de sessão sem alvo e coexistência "usuários existem com o portão de bearer desligado" dependem das auditorias da 020 e da 007.
5. **Não verificado nesta rodada:** leitura das transcrições por harness (adaptadores da 010) e o conteúdo dos planos de lançamento por harness; `docs/design-boot-backfill.md` foi conferido, mas o exportador de cada harness não.
6. **Regerar o DAG da 013** (T005) incorporando A1 a A13 antes de implementar; prioridade Windows: A2, A3, A5, A10, A12.
