# Auditoria 009: feature 001 (núcleo) contra a referência 3.1.1

- **Data:** 2026-10-25
- **Papel:** auditor "chinese wall" (leitura do código original autorizada; nenhum trecho de código copiado)
- **Escopo:** `docs/architecture.md`, `overview.md`, `data-model.md`, `database-schema.md`,
  `storage-and-retrieval.md` (exceto §15–16, já auditadas), `wiki-files-layer.md`, `domain-vocabulary.md`,
  `design-decisions.md`, `test-fixtures.md` e `docs/assets/fixtures`
- **Base:** código de referência 3.1.1 (83 migrações). Os docs já tinham passado por duas auditorias
  (2026-10-18); esta passada procurou o que ainda faltava ou estava errado.

## Status

**Precisa de ajustes → Fiel nos pontos do núcleo, ainda incompleta fora dele.**

O esquema, o FTS, o upsert, a busca híbrida e a escrita atômica estavam corretos. Os erros
encontrados estavam nas bordas: remoção de página, ledger, mensagens literais, e afirmações antigas
sobre snapshot e log em `design-decisions.md` e `architecture.md`, que contradiziam os docs já
corrigidos.

## Arquivos corrigidos

| Arquivo | Seções |
|---|---|
| `docs/storage-and-retrieval.md` | §11.3 (descritor, tokenização de entidades), §12 (slots, working, TTL), §19 (textos literais de erro) |
| `docs/wiki-files-layer.md` | §4 (Windows: código 5, reaproveitamento do temporário), §5.1 (guardas), §5.2 passo 9 (reversão e texto literal), §5.4 (quarentena e fluxo comum das remoções, lápide da reconciliação), §9 (quem escreve o ledger) |
| `docs/overview.md` | invariante 6 (texto), tabela de imposição (linhas 4, 6, 11), sanitizador na tabela de tecnologias |
| `docs/design-decisions.md` | §1 itens 3 e 6, §3 (reversão) |
| `docs/architecture.md` | fluxo de captura (ledger) |
| `docs/database-schema.md` | §0 (colunas do histórico de migrações), §10 (reconstruções e migrações 77–83) |
| `docs/data-model.md` | §6 (efeitos colaterais da reindexação) |
| `docs/test-fixtures.md` | tabela de fixtures (01–05) e estado das cópias |
| `specs/features/001-nucleo/spec.md` | nova seção "Correções da auditoria (2026-10-25)" |

## Conceito errado → correção

| Conceito nos docs | Correção (referência 3.1.1) |
|---|---|
| A abertura do banco tira snapshot antes de migrar (`design-decisions.md`) | Não há cópia do banco. Cada migração tem a própria transação; o único backup automático é o do diretório de dados, só com wiki fora da OKF e sem recibo. |
| Cada escrita da wiki acrescenta uma linha ao log (`architecture.md`) | Só eventos de hook vão ao ledger. O servidor escreve só em `log-AAAA-MM.md` (mês UTC do evento); `log.md` é só reconhecido. |
| Só a remoção por decaimento usa quarentena | As três remoções (simples, condicional por versão e por decaimento) movem o arquivo para `.ai-memory-delete.<aleatório>` no mesmo diretório e o restauram se o banco falhar ou a condição deixar de valer. |
| "Remoções usam a guarda exclusiva" | A remoção simples usa a guarda **compartilhada**; as condicionais, a lápide da reconciliação e a remoção definitiva usam a exclusiva. |
| Erro de estado inconsistente sem texto | Texto literal registrado; reversão em ordem inversa; conteúdo anterior lido antes de instalar (erro de leitura que não seja "ausente" aborta antes de tocar o disco). |
| Troca atômica no Windows repete "em violação de compartilhamento ou bloqueio" (sem dizer o resto) | Só 32 e 33 repetem; acesso negado (5) propaga na hora; o mesmo temporário é reaproveitado; o código 32 no Unix não é repetido. |
| Barreira de tipo de sanitização "a confirmar" para todo texto | A barreira de tipo só existe para observações. Páginas, handoff manual, mensagens, motivo de feedback e eventos de workstream são higienizados pelo chamador, sem tipo; a reindexação não higieniza. |
| Diretório de dados sempre absoluto | Canonicaliza o caminho, ou o pai e reanexa o nome; se nenhum existe, fica como foi dado (pode ser relativo). Sem diretório local do usuário, o padrão é `ai-memory` relativo. |
| `_slots/` isento da evicção, `working` com regra de fim de sessão ("a confirmar") | A varredura não olha o caminho (slots só são isentos por fixação). Não há regra de fim de sessão para `working`. O TTL remove de vez, sem lápide, ignorando fixação. |
| Reconstruções de `sessions` até a 73; `users` só na 54 | Também 78, 80, 81 e 83; `users` em 54 e 79; 77 e 82 alteram `ingest_keys` e `managed_runs`. |
| Reindexação "reconstrói só" as tabelas derivadas | Também grava `audit_log` e, com embedder, embedding de versão nova. |
| Fixtures em `docs/assets/fixtures` iguais à referência | `capture-policy.json` está defasado (faltam dezenas de casos); 01–05 são iguais; 06 e 07 não foram trazidos. |

## Funcionalidade existente no código e ausente dos docs (agora registrada)

- Algoritmo do **descritor** de resultados que não vieram da FTS (fonte, linhas puladas, orçamento de 240 caracteres, reticências).
- **Tokenização exata** do fluxo de entidades (corte em não alfanumérico exceto `-`/`_`, tamanho medido antes das minúsculas, composto antes das partes).
- **Fluxo comum das remoções** com quarentena, e a lápide da reconciliação sem cadeia bloqueante.
- **Formato e regras do escritor do ledger** (12 eventos, título com quebras trocadas por espaço e cortado em 120 caracteres, acréscimo sem troca atômica).
- **Textos literais** dos erros do armazenamento (schema à frente, colisão de caminho, escritor e leitor fechados, sessão purgada, nomes ocupados e demais).
- Tipos das colunas de `refinery_schema_history` e leitura estrita delas.

## Principais equívocos

1. Docs genéricos (`design-decisions.md`, `architecture.md`) mantinham afirmações da versão 2.6.2
   (snapshot antes de migrar, linha de log por escrita) que contradiziam os docs específicos já
   corrigidos. Quem lesse só o doc geral implementaria errado.
2. A quarentena era descrita como exclusiva do decaimento. Uma remoção simples que apaga o arquivo
   antes do banco perde o arquivo se o banco falhar.
3. A garantia da invariante 6 parecia valer para todo texto. Na referência, ela só é imposta pelo
   tipo nas observações.

## Pendências

- `capture-policy.json` em `docs/assets/fixtures` precisa ser atualizado para a versão da referência
  (feature 002). Não foi copiado nesta auditoria.
- Fixtures de avaliação 06 e 07 ausentes (escopo LLM).
- Continuam "a confirmar" em `design-decisions.md` e `architecture.md` itens fora do núcleo (hooks,
  consolidação, workstreams, contêiner, anotações de ferramentas MCP).
- `overview.md` linhas 1, 5 e 8 da tabela de imposição (configuração no caminho de hook, timeout dos
  hooks em script, aviso de divergência de embedding) ficaram como estavam: fora da 001.
- O rótulo `ensure_portable` foi mantido como nome da regra de portabilidade. Ele é citado em specs,
  notas e no acceptance, e renomeá-lo quebraria a rastreabilidade. Se o autor quiser, troque-o por
  "verificação de portabilidade" em todos os artefatos de uma vez.
- Nos docs fora do escopo (`consolidation-llm.md`, `api-reference.md` e outros) ainda aparecem nomes
  internos. Não foram tocados.

## Complemento (pedido do coordenador)

1. **Nomes internos reescritos como operações de negócio.**
   - `wiki-files-layer.md`: §5.4 inteira (remover página, remover se a versão for a esperada, evicção
     por decaimento, remoção definitiva de lápide, purgas, mover projeto e página de sessão, restaurar
     de checkpoint, reindexar página, reconstruir índice, preencher manifestos, reparar validade, ledger,
     admissão), além de §1, §5.5, §7 (commit sem mudança, marcas internas do adaptador) e §12.
   - `storage-and-retrieval.md`: resultados da ingestão (já completa, retomar pendente, inserida), títulos
     de §11.2 e §11.3, resolvedor e origens de escopo (agora pelos valores de `resolved_by`) e constantes
     de §15 e §16.
   - `domain-vocabulary.md`: nomes dos tipos de identificador e dos tipos de domínio.
   - Ficaram só nomes que são contrato observável: tabelas, colunas, chaves, valores de operação e de
     auditoria e campos de resposta.
2. **"Nenhum handler escreve direto na wiki": confirmada, com uma exceção.**
   - Varri todos os componentes fora da camada de wiki (MCP, administração, hooks, web, consolidação,
     workstream, CLI), sem os módulos de teste. Nenhum handler grava, renomeia ou apaga arquivo na árvore
     da wiki.
   - As escritas diretas encontradas estão em `raw/`: segmentos de workstream e sua remoção na purga.
     Na CLI, os casos são arquivos de bloqueio, marcadores e o diretório do projeto do usuário.
   - Exceção: `reset`, `uninstall --purge-data` e `restore` apagam ou trocam `wiki/`, `db/` e `raw/`
     inteiros, offline e com a guarda de processo vivo.
   - Dentro da camada de wiki, um lint proíbe operações de arquivo sem o adaptador Git.
   - Registrado em `design-decisions.md` §3.
3. **Lacuna `PageKind`** de `notes/domain-contract.md` fechada. Esse tipo não existe na referência; a
   nota agora descreve as três formas derivadas do "tipo" (`kind` livre, tipo de ranking, `type` OKF),
   com as regras exatas.
- Reimplementação (não há código no workspace hoje): a remoção de página precisa da quarentena e da guarda descritas em §5.4; mensagens de erro
  devem seguir §19. Trabalho de implementação, não feito aqui.
