# plan.md: Plano de Implementação Técnica — Outros agentes (010)

> Plano gerado em 2026-10-13, antes de a 002 ser executada. Os contratos por agente são extraídos pela fase 0 do `tasks.md`; revisar o plano com as notas da fase 0 e com o código da 002.

## 1. Metadados do Plano
- **Stack Tecnológico:** a da 002. `encoding/json`, TOML pelo parser do `koanf` já em uso (ou o mesmo pacote que a 002 usar para `config.toml`), `text/template` para plugins e extensões gerados, `modernc.org/sqlite` em somente leitura para lojas nativas. Nenhuma dependência nova prevista; se um formato exigir uma (por exemplo YAML já existe via `yaml.v3`), a task correspondente registra em `tech.md`.
- **Feature Fonte:** `specs/features/010-outros-agentes/spec.md`
- **Escopo:** instalação e captura dos demais agentes, scripts, ponte MCP com sessão, captura externa, adaptadores de transcrição, backfill, `doctor`, `finalize-session`, integração restante e ativos de agente.
- **Fontes de contrato:** as listadas na spec.

## 2. Design de Contratos e Fronteiras
- **Descritor de agente** (dado, não código por agente): nome e aliases, arquivo de hooks e de MCP por plataforma (com variáveis de ambiente como `CODEX_HOME`, `KIMI_CODE_HOME`, `KIRO_HOME`, `GROK_HOME`, `COPILOT_HOME`, `XDG_CONFIG_HOME`, lidas por `internal/config`), formato, eventos nativos e mapeamento para o vocabulário canônico, onde entregar o handoff, se tem fim de sessão.
- **Contrato público de eventos externos** e confirmações de lote: `docs/external-lifecycle.md` §2 a §6.
- **Relatório do backfill:** `docs/design-boot-backfill.md` §8.

## 3. Decisões Arquiteturais e Justificativas

- **Decisão (G-01): instalador orientado por descritor.** Uma tabela de descritores alimenta `install-hooks`, `install-mcp`, `install-skills`, `uninstall` e `doctor`; o código por formato (JSON, TOML, YAML impresso, plugin gerado) é compartilhado.
  - **Justificativa:** a matriz tem mais de 25 agentes com o mesmo ciclo (imprimir, aplicar, backup, desinstalar).
  - **Alternativa Rejeitada:** um instalador por agente (duplicação e divergência entre `install` e `uninstall`).

- **Decisão (G-02): normalização por agente pura e testada por fixtures.** As regras de §3.3 a §3.5 do `lifecycle-event-ingestion.md` ficam no `ingest`, com um diretório de fixtures por agente extraído na fase 0.
  - **Justificativa:** os formatos dos agentes mudam; fixtures versionadas mostram a regressão.
  - **Alternativa Rejeitada:** normalizar no cliente (o servidor reinspeciona de qualquer forma).

- **Decisão (G-03): adaptadores de transcrição no `cli`, somente leitura.** Leem as lojas nativas (JSONL, SQLite, Antigravity) filtradas pelo diretório de trabalho, devolvem eventos visíveis e um registro de perdas; nunca escrevem nessas lojas. A 013 reaproveita os mesmos adaptadores.
  - **Justificativa:** `docs/harness-adapters.md` §3; o backfill e o `doctor` precisam deles antes da 013.
  - **Alternativa Rejeitada:** deixar os adaptadores na 013 (o backfill ficaria sem fonte).

- **Decisão (G-04): backfill como processo destacado sobre o lote do spool.** O gatilho no início de sessão olha a sentinela local e dispara o processo sem esperar; o comando reaproveita o envio em lote de 256 itens da 002, com as regras de seleção, horário, retentativa e relatório do doc.
  - **Justificativa:** `docs/design-boot-backfill.md` § "Base reutilizada" (replay pelo ingresso de captura ao vivo).
  - **Alternativa Rejeitada:** inserir direto no banco (pularia a sanitização).

- **Decisão (G-05): scripts de hook gerados como recurso embutido.** Os scripts POSIX e PowerShell são arquivos embutidos no binário e extraídos por `install-hooks` e `setup-agent`; os testes de paridade comparam a saída do script com a do hook nativo para os mesmos payloads.
  - **Justificativa:** `docs/hook-scripts-and-container-deploy.md` §6 exige paridade.
  - **Alternativa Rejeitada:** scripts mantidos fora do binário (versões divergentes).

### O que não construir nesta feature
`ai-memory run` e workstreams (013), macOS, contêiner, AUR e serviço de sistema (015).

## 4. Auditoria de Constituição

**Portões de Qualidade**
- [ ] Build cruzado, `go vet`, `go test -race`.
- [ ] Gherkin — os cenários da spec viram o `.feature` da 010.
- [ ] Dependência nova — nenhuma prevista; qualquer exceção registrada em `tech.md` pela task.

**Restrições Arquiteturais**
- [ ] INV-01 — variáveis de ambiente dos agentes (`CODEX_HOME` e demais) só em `internal/config`.
- [ ] INV-05 — hooks dos novos agentes seguem 202/429 e o spool da 002.
- [ ] INV-06 — backfill passa pela sanitização do ingresso (G-04).
- [ ] INV-16 — handoff exatamente uma vez também nos caminhos de entrega de cada agente.
- [ ] Grafo de imports — normalização no `ingest`; instalador e adaptadores no `cli`.

**Anti-Padrões Proibidos**
- [ ] Sem `os.Getenv` fora de `config` — inclusive nos adaptadores e no instalador.
- [ ] Goroutine com `context` — o backfill é processo separado; dentro dele, envio sequencial.
