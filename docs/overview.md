# Visão geral, arquitetura e invariantes

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

> Ponto de entrada conceitual para quem vai **reimplementar** o ai-memory.
> Resume o que o sistema é, seus componentes, as invariantes que não podem ser violadas e as
> regras de fronteira. O detalhe de cada parte está nos documentos indicados.
> A numeração das invariantes é a do projeto de origem. Afirmações marcadas como **a confirmar**
> não foram verificadas no código de referência.

## 1. O que é

Um **servidor único** (binário) que dá memória de longo prazo e entre sessões a agentes de
código (Claude Code, Codex, Cursor, Gemini CLI, OpenCode, Devin, Kiro e outros), via **MCP** e
**hooks de ciclo de vida**. Permite sair de um agente no meio da tarefa, abrir outro no mesmo
diretório e continuar sem re-explicar o contexto.

Ideias de projeto:

- **Markdown em git é a fonte da verdade.** A wiki fica em `<data_dir>/wiki/`, é editável à mão
  e cada passe de consolidação gera um commit.
- **SQLite é o índice derivado** (`<data_dir>/db/memory.sqlite`, modo WAL): busca textual FTS5,
  sessões, observações, handoffs, usuários, auditoria, links/entidades, embeddings e o registro
  (ledger) de workstreams. Um **escritor único** (ator de escrita em thread dedicada) é dono da
  conexão de escrita; a leitura usa um pool somente leitura.
- **Captura automática** por hooks: o agente envia observações sanitizadas e limitadas para o
  endpoint `/hook`; o servidor transforma a sessão em páginas duráveis (*compilar, não recuperar*).
- **Recuperação** por FTS5 + casamento lexical de entidades + vizinhos de link, fundidos por
  RRF (Reciprocal Rank Fusion, constante k = 60, verificada na versão 2.6.2), com RRF vetorial
  opcional (se há embeddings) e *fallback* limitado para observações cruas quando nenhuma página
  compilada casa.
- **LLM é opcional.** O modo zero-LLM captura, busca (FTS5) e escreve resumos por regras.
  Provedores (Anthropic, OpenAI, Codex, GitHub Copilot, Gemini, OpenCode, endpoints compatíveis
  com OpenAI) habilitam consolidação, lint e o laço de auto-melhoria.
- **Isolamento por construção:** toda linha e página tem chave `(workspace_id, project_id, path)`,
  resolvida pelo diretório do chamador, por um marcador `.ai-memory.toml` ou por escopo explícito.

## 2. Componentes e onde estão documentados

| Componente | Responsabilidade | Documento |
|---|---|---|
| Domínio | tipos de domínio, identificadores, caminho de página, sanitização e identidade/autorização (sem I/O) | `domain-vocabulary.md` |
| Armazenamento | SQLite, escritor único, leitor, busca híbrida RRF, decaimento, crença, credenciais | `storage-and-retrieval.md`, `database-schema.md` |
| Wiki | escrita atômica de markdown, git, watcher, webhooks de admissão | `wiki-files-layer.md`, `admission-webhooks.md`, `okf.md` |
| Ingestão de hooks | endpoint `/hook`, política de captura, síntese de sessão, handoff | `lifecycle-event-ingestion.md`, `marker-file.md` |
| Cliente LLM | provedores, embeddings, reranker, autenticação, fallback | `llm-client.md`, `llm-providers.md`, `llm-provider-fallback.md`, `local-embeddings.md` |
| Consolidação | consolidação, auto-melhoria, decaimento, lint, curador, bootstrap | `consolidation.md`, `prompts-schemas-and-sanitizer.md`, `auto-improvement-loop.md` |
| Servidor MCP | ferramentas MCP (23), rotas `/admin/*`, autenticação | `mcp-server.md`, `api-reference.md` |
| Navegador web | navegador somente leitura da wiki e API `/api/v1` | `wiki-browser.md`, `frontend-api.md` |
| Continuidade de workstream | continuidade entre agentes (`ai-memory run`) | `harness-adapters.md`, `managed-workstreams.md` |
| CLI | binário: `serve`, clientes finos, hooks nativos com spool, instaladores | `cli.md`, `usage.md` |
| Hooks em script, Docker, deploy | | `hook-scripts-and-container-deploy.md`, `deploy.md` |
| Companions | relay, importer, wikisync, cliente, app macOS | `external-companions-and-tests.md` |
| Contratos transversais | captura externa, identidade de repositório, backups, perfil | `complementary-contracts.md` |

## 3. Fluxo de dados resumido

```text
Agente → hook (entrada JSON) → comando `ai-memory hook` (política de captura, spool local)
       → POST /hook ou /hook/batch (chave de ingestão idempotente)
       → sanitização → transação única: sessão + observação + chave de ingestão
       → SessionEnd: página de sessão (zero-LLM) → gravação da página na wiki → handoff automático → git
                     → (opcional, desligado por padrão) fila durável → consolidação por LLM → 1-5 atualizações
Agente → MCP (memory_query…) → FTS5 + entidades + grafo [+ vetor] → RRF → autoridade → [rerank] → [resposta]
Em background: varredura/decaimento, lint, backfill de embeddings, auto-melhoria, experiência, dream
```

## 4. Invariantes (não violar)

Cada invariante nasceu de um defeito observado em outro sistema ou de um incidente interno.

1. **Um único caminho de leitura de configuração.** A configuração é carregada uma vez no início;
   o restante do código não lê variáveis de ambiente diretamente.
2. **Único escritor SQLite (ator).** Toda escrita passa por um canal para uma thread dedicada.
   Agrupe o trabalho quente num único comando/transação; evite N+1.
3. **Índices commitam na mesma transação dos dados.** Sem tarefa de "indexar depois do retorno".
4. **Identidade tipada de 3 elementos** `(workspace_id, project_id, path)` em toda linha de domínio.
5. **Hooks são fire-and-forget e limitados.** O endpoint de ingestão responde 202 quando enfileira
   e 429 quando satura (até 1024 requisições de ingestão em voo; até 256 itens por lote de hook).
   Hooks em script do bundle de shell têm timeout de POST de 0,2 s. Os bundles PowerShell usam
   3 s e os plugins gerados usam 2 s (**a confirmar** qual valor é o alvo). Sem fan-out ilimitado
   nem filas nos caminhos de hook.
6. **A remoção de privacidade é uma fronteira de tipo.** Uma observação só pode ser armazenada
   depois de passar pela sanitização, que remove segredos antes de limitar o tamanho: título em
   no máximo 80 caracteres e corpo em no máximo 16 KiB. O sanitizador é o caminho para o
   armazenamento de texto não confiável (**a confirmar** que não há outro caminho).
7. **Somente saídas estruturadas por JSON Schema** nas chamadas de LLM (sem XML nem bibliotecas-invólucro).
8. **`{provider, model, dim}` desnormalizado ao lado de todo embedding**; vetores velhos geram
   aviso e são ignorados quando a configuração diverge.
9. **Checagem de processo vivo antes de operações de ciclo de vida direto em disco.** `reset`,
   `restore`, `reindex` e `uninstall --purge-data` consultam os processos do sistema (a guarda do
   uninstall só vale com `--purge-data`). `backup` continua online: o cliente pede ao servidor um
   snapshot do SQLite pela API de backup online, com o escritor ativo.
10. **Escritas de arquivo atômicas** (arquivo temporário + rename + fsync); o watcher ignora as
    próprias escritas pelo prefixo de nome dos arquivos temporários.
11. **Diretório de dados absoluto e canônico**, registrado no log na inicialização.
12. *(Entrada ausente nesta lista; ver a nota abaixo.)*
13. **Caminho padrão zero-LLM**: o sistema funciona completo sem provedor.
14. **A autenticação do provedor resolve antes da construção do cliente**; clientes de provedor
    consomem material de autenticação tipado e nunca leem variáveis de ambiente.
15. **Assinantes de log filtram o próprio módulo** — sem laços de realimentação de log.
16. **Multi-sessão e multiusuário no mesmo projeto é capacidade central.** Um operador com vários
    agentes simultâneos e vários operadores num servidor compartilhado precisam funcionar, e o
    conhecimento escrito por qualquer um precisa ser legível pelo outro no mesmo projeto:
    - **Páginas são compartilhadas; handoffs têm dono.** A coluna de autoria das páginas serve
      para atribuição e **nunca** pode virar filtro de leitura; o filtro de dono vale para
      handoffs e fica lá. Escopar a leitura de páginas por operador faz a colaboração do time
      parar silenciosamente, enquanto todo teste de usuário único continua passando.
    - **Escrita concorrente divergente supersede, nunca destrói.** Dois agentes editando o mesmo
      caminho produzem uma cadeia de supersessão; a versão perdedora continua alcançável.
    - **Um handoff é reivindicado exatamente uma vez**, por duas guardas independentes de estado
      `aberto` (a consulta de metadados e a atualização de reivindicação). Mantenha as duas: é
      defesa em profundidade, não duplicação.
    - **O ponteiro de "projeto ativo" é chaveado pela coordenada do chamador** (modo por ator, por
      padrão), para que agentes paralelos e operadores distintos não sobrescrevam a noção de
      "projeto atual" um do outro. Escritas sem escopo também usam esse ponteiro.

    Testes unitários não cobrem isso, porque exercitam uma sessão por vez. Qualquer mudança em
    resolução de escopo, supersessão de página, identidade de sessão, aceitação de handoff, escritor
    ou filtros de dono precisa de teste multi-sessão explícito (ver `test-fixtures.md`).

**Nota sobre a entrada 12.** A lista acima não tem a entrada 12. Ela não foi renumerada nem
reconstruída aqui. A tabela de imposição abaixo marca o caso como **a confirmar**.

Regras de fronteira adicionais:

- **Resolução de escopo:** toda rota nova (MCP, administração, web) usa o resolvedor central de
  escopo ou seus auxiliares explícitos (busca de escopo existente, criação explícita de escopo,
  busca em lote de escopos existentes). Nunca monte cadeias de busca de workspace/projeto à mão.
  Caminhos de leitura, busca, embedding, retenção e destrutivos usam busca **sem criar** e
  **falham fechado** em escopo parcial ou ausente. Só caminhos explícitos de escrita/criação
  podem criar workspaces ou projetos.
- **Autorização:** preserve as fronteiras pela checagem de capacidade do nível de autenticação
  (ver `security-boundaries.md`); sem comparação de nome de usuário nem checagem de root ad hoc.
  Em modo multiusuário, toda rota `/admin/*` é só para root; tokens de usuário do banco nunca
  contornam os gates de administração nem os webhooks de admissão.
- **Mutações da wiki** passam pela gravação de página, pela aplicação de lote ou pelos helpers
  destrutivos existentes, para que sanitização, admissão, atribuição, rollback e atualização de
  índices andem juntos. Nunca escreva arquivos da wiki diretamente de um handler.

### Onde cada invariante é imposta

A numeração é a da lista acima. "A confirmar" indica que o local de imposição ou a cobertura
completa não foi verificada no código de referência.

| # | Onde é imposta |
|---|---|
| 1 | Carregamento de configuração na CLI, feito uma vez no início. **A confirmar:** o caminho de hook faz carregamento próprio e tolerante; instaladores de agentes, o componente de workstream e o de backup/migração leem variáveis de ambiente diretamente. |
| 2 | Camada de armazenamento: escritor único em thread dedicada, com fila de capacidade 1024. |
| 3 | Upsert de página no armazenamento, dentro da transação. A gravação da wiki desfaz o arquivo se o upsert falhar. A ingestão de hook grava sessão, observação e chave de ingestão na mesma transação. |
| 4 | Tipos de identidade do domínio e colunas de workspace e projeto no esquema inicial. **A confirmar:** cobertura por tabela. |
| 5 | Roteador de ingestão de hooks (202, 429 por saturação, limite de lote e limite de taxa por origem) e comando de hook da CLI com spool local. **A confirmar:** valor-alvo do timeout de hooks em script (ver texto da invariante). |
| 6 | Tipo de observação sanitizada no domínio, criável apenas pela função de sanitização. **A confirmar:** se todo texto não confiável que chega ao armazenamento passa por ela (há outros pontos de chamada, no servidor MCP, na administração e no armazenamento). |
| 7 | Adaptador de provedor LLM (saída com JSON Schema) e tipos de saída da consolidação. |
| 8 | Armazenamento de embeddings (colunas `provider`, `model`, `dim`; remoção de vetores obsoletos) e interface de embedder do cliente LLM. **A confirmar:** aviso ao detectar divergência. |
| 9 | Guarda de processos vivos na CLI (`reset`, `restore`, `reindex`, `uninstall` com purge). O backup usa snapshot online do armazenamento. |
| 10 | Helper de escrita atômica da wiki (arquivo temporário, sincronização de dados, rename, sincronização do diretório) e aplicação de lote da wiki. O watcher ignora arquivos temporários pelo prefixo. |
| 11 | Configuração e inicialização da CLI. O caminho é canonicalizado e registrado no log de nível informação na inicialização. **A confirmar:** caminho absoluto garantido só quando o caminho ou seu diretório pai existe. |
| 12 | **A confirmar.** A entrada não aparece nesta lista. O projeto de origem a registra, mas o local de imposição no código não foi conferido. |
| 13 | Consolidação (varredura, lint, curador) no componente de consolidação; síntese de sessão no componente de ingestão de hooks. |
| 14 | Autenticação de provedores: material tipado resolvido na CLI antes de construir o cliente. Clientes não leem variáveis de ambiente (as leituras que existem no componente LLM estão em testes). |
| 15 | Filtro de log padrão da CLI: o módulo que escreve o log é limitado ao nível de aviso, o que impede realimentação. |
| 16 | Upsert de página no armazenamento (autoria não filtra leitura) e filtro de dono no domínio (handoffs). |

## 5. Convenções de código do projeto original (úteis como convenção)

- Seguir o estilo do arquivo; comentários explicam o *porquê*.
- Mudanças pequenas e com escopo; sem abstrações especulativas nem superfície pública sem chamador.
- Sem código morto; trabalho futuro vai para notas de design, não para stubs.
- Preferir fallbacks explícitos a falhas por valor ausente ou marcadores de "inalcançável" em caminhos de execução.
- **Fronteiras tipadas são estruturais:** identificadores, caminho de página, tipo de agente,
  sanitização, resolução de workspace/projeto, capacidade de autorização e dialetos de provedor
  são interpretados/normalizados uma vez e reutilizados.
- Comandos de CLI finos: interpretar argumentos, resolver configuração uma vez, chamar funções
  tipadas do componente, renderizar. Comportamento específico de provedor fica no cliente LLM,
  nunca em handlers de CLI ou administração.

## 6. Mapa de tecnologias (sugestão de implementação, não é contrato)

> Esta tabela é **uma sugestão de partida**. Não faz parte do comportamento do produto nem é
> contrato. Escolha bibliotecas e versões no seu ambiente, validando licenças e manutenção.
> O que é comportamento do produto está nas invariantes e nos documentos de cada componente.

| Papel | Sugestão de implementação |
|---|---|
| Concorrência assíncrona | tarefas leves com cancelamento por contexto |
| Servidor HTTP e middlewares | roteador HTTP padrão com middlewares próprios; limite de corpo de 10 MiB nas entradas |
| Servidor MCP (HTTP) | SDK MCP oficial da linguagem escolhida, se existir; senão, implementar o protocolo |
| Banco SQLite | driver que inclua FTS5 e a API de backup online |
| Migrações | ferramenta de migração versionada com arquivos `V<nn>__nome.sql` (esquema inicial: `V01__init.sql`) |
| Commits da wiki | biblioteca git ou invocação do executável git |
| Watcher de arquivos | notificação do sistema de arquivos com debounce próprio (referência: 300 ms, versão 2.6.2) e reconciliação periódica (referência: 30 s) |
| Cliente HTTP de provedores | cliente HTTP da linguagem com TLS |
| Configuração | leitura de TOML com variáveis `AI_MEMORY_*`, usando `__` como separador de níveis (comportamento do produto) |
| CLI | framework de subcomandos |
| JSON Schema de saída estruturada | geração a partir dos tipos de saída |
| Sanitizador | motor de expressões regulares sem retrocesso; os padrões de `prompts-schemas-and-sanitizer.md` devem rodar nele (**a confirmar**) |
| Hash de senha | algoritmo memory-hard Argon2id, em formato PHC |
| Hashes, comparação em tempo constante, aleatoriedade | SHA-256; comparação de tokens em tempo constante; gerador criptográfico do sistema |
| Identificadores | UUIDv7 (é comportamento do produto) |
| Tempo | precisão de microssegundos |
| Checagem de processo vivo | enumeração de processos do sistema operacional |
| Logs estruturados | logger estruturado com níveis e filtros por módulo |
| Lock do spool de hooks | lock de arquivo com liberação no encerramento |
| Embeddings locais | modelo local opcional; ou cliente de um serviço de embeddings compatível com OpenAI |

## 7. Ordem sugerida de implementação (sugestão, não é contrato)

Para reimplementar do zero, nesta ordem (cada passo depende dos anteriores):

1. **Domínio:** identificadores UUIDv7, caminho de página, níveis de página, tipos de observação, sanitização e o tipo que marca texto sanitizado.
2. **Armazenamento mínimo:** esquema `V01`, escritor único, gravação de página na mesma transação, FTS e pool de leitura.
3. **Wiki:** escrita atômica com frontmatter e git (sem webhooks nem watcher no início).
4. **Hooks:** endpoint `/hook` com os eventos de início de sessão, prompt do usuário, uso de ferramenta e fim de sessão (o produto aceita mais eventos; ver `lifecycle-event-ingestion.md`), síntese de sessão e handoff automático.
5. **Servidor MCP:** ferramentas `memory_query`, `memory_write_page`, `memory_recent` e `memory_read_page`, com autenticação Bearer.
6. **CLI:** `serve`, `hook` com spool e instalação de hooks para **um** agente.
7. **Cliente LLM e consolidação:** página única.
8. **Incremental:** embeddings e RRF vetorial, varredura/decaimento, lint, auto-melhoria com aprovação obrigatória, handoffs e mensagens, usuários, web e workstreams.

## 8. Considerações de ambiente corporativo

| Tema | Onde ver |
|---|---|
| Autenticação de máquina e humana, SSO/OIDC | `sso.md`, `users.md`, `api-reference.md` (login humano: seção 4), `mcp-server.md` |
| TLS e proxy reverso | `https-via-proxy.md`, `deploy.md` |
| Instalação sem internet | `airgapped-install.md` |
| LLM interno / endpoints compatíveis com OpenAI, fallback | `llm-providers.md`, `llm-provider-fallback.md` |
| O que sai da máquina e retenção | `threat-model-and-data-handling.md` |
| Autorização por projeto (grants) | `design-per-project-authz.md`, `database-schema.md` (seção 5) |
| Webhooks de admissão (política de escrita) | `admission-webhooks.md` |
| Backups | `complementary-contracts.md` (seção 5), `backup.md` |

## 9. Grafo de dependências entre componentes

Cada linha lista do que o componente depende (dependências de compilação entre componentes).

| Componente | Depende de |
|---|---|
| Domínio | nenhum outro componente; sem I/O |
| Armazenamento | Domínio |
| Cliente LLM | Domínio |
| Wiki | Domínio, Armazenamento, Cliente LLM (somente a interface de embedder) |
| Consolidação | Domínio, Armazenamento, Wiki, Cliente LLM |
| Ingestão de hooks | Domínio, Armazenamento, Wiki, Consolidação |
| Servidor MCP | Domínio, Armazenamento, Wiki, Cliente LLM, Consolidação (a ingestão de hooks só entra em testes) |
| Navegador web | Domínio, Armazenamento, Wiki (somente leitura) |
| Continuidade de workstream | Domínio e acesso local ao banco |
| CLI | todos os componentes (única composição) |

Regras estruturais:

- O domínio não depende de nenhum outro componente e **não faz I/O**.
- O servidor MCP **não** depende da ingestão de hooks em produção; por isso as métricas de ingestão ficam no domínio. A ingestão entra no servidor MCP só como dependência de teste.
- Só a CLI compõe os objetos. **A confirmar:** a regra "só a CLI lê o ambiente", porque o componente de workstream e o de backup/migração também leem variáveis de ambiente fora de testes.
- O cliente LLM é referenciado pela wiki apenas pela interface de embedder.

## 10. Escopo e exclusões

Removidos de propósito por não ajudarem na implementação: relatórios de benchmark, harness de
avaliação, estudos de concorrentes, listas de problemas de outros projetos, comparativos,
roadmap 2.0, guia de migração 2.0, design de CI do Windows, casos de uso de marketing, guia de
macOS e completions de shell. Referências a eles dentro dos documentos de design são **históricas**:
o conteúdo não está mais neste conjunto.

Constantes numéricas (limites, timeouts, padrões) foram conferidas na versão 2.6.2 do código de
referência e podem mudar entre versões. Confirme os valores que forem críticos.

## 11. Limites desta documentação

- Foi produzida lendo a implementação (módulos, tipos, funções e trechos centrais de fluxo), **não**
  executando a suíte nem o servidor. As afirmações foram conferidas uma a uma no código da versão 2.6.2.
- Partes grandes e repetitivas estão descritas por **contrato e estrutura**, não linha a linha: o
  módulo de instalação de hooks (cerca de 14 mil linhas no total, incluindo testes e a renderização
  por agente), handlers administrativos individuais, parsers de transcrição por agente e a
  desinstalação. Para um detalhe fino, consulte a implementação de referência.
- Constantes numéricas foram conferidas na versão 2.6.2; o changelog do projeto registra as alterações.
- O esquema SQL está consolidado (estado final das 76 migrações) em [database-schema.md](database-schema.md); os arquivos de migração individuais não estão transcritos.
- Os argumentos de cada ferramenta MCP e as rotas `/admin/*` estão em [api-reference.md](api-reference.md). Os prompts de LLM, os schemas de saída e as regexes do sanitizador estão literalmente em [prompts-schemas-and-sanitizer.md](../docs-adiados/prompts-schemas-and-sanitizer.md).
