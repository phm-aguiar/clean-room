# Matriz invariante para teste: F001 Núcleo (T077)

- **Data:** 2026-10-12
- **Fonte:** `.github/memory/constitution.md` (INV-01 a INV-16); `specs/features/001-nucleo/spec.md` (critério: "Invariantes INV-01..06, 09..11, 13, 15 e 16 têm teste de contrato nomeado ou débito registrado").
- **Testes lidos:** `test/contract/wiki_inv_test.go`, `test/contract/runtime_inv_test.go`, `test/contract/storage_inv_test.go` (localizados por `grep "func TestINV"`).
- **Regra:** cada INV tem teste nomeado ou débito registrado em ADR (constituição, Portões de Qualidade).

## Tabela

| Invariante | Descrição curta | Arquivo do teste | Nome(s) do teste | Status |
|---|---|---|---|---|
| INV-01 | Configuração carregada uma única vez, em `internal/config`, chamada pela `cli` | `test/contract/runtime_inv_test.go` | `TestINV01_ConfigCarregadaUmaVez` | coberto |
| INV-02 | Escritor SQLite único; leitura em pool somente leitura | `test/contract/storage_inv_test.go` | `TestINV02_EscritorSerializaConcorrentes`; `TestINV02_LeituraNaoEsperaTransacaoDeEscrita`; `TestINV02_PoolDeLeituraRecusaEscrita` | coberto |
| INV-03 | Índices commitam na mesma transação dos dados | `test/contract/storage_inv_test.go` | `TestINV03_IndicesMesmaTransacaoAposRetorno`; `TestINV03_FalhaDesfazIndicesEPagina` | coberto |
| INV-04 | Identidade tipada `(workspace_id, project_id, path)` em toda linha e página | `test/contract/storage_inv_test.go` | `TestINV04_MesmoPathProjetosDiferentesCoexistem`; `TestINV04_MesmoPathWorkspacesDiferentesCoexistem`; `TestINV04_MesmoEscopoSupersedeSemDuplicarCorrente`; `TestINV04_TodaLinhaTemIdentidadeTipada` | coberto |
| INV-05 | Hooks fire-and-forget; `/hook` 202/429 | n/a | n/a | fora da 001 (feature 002) |
| INV-06 | Sanitização antes do limite de tamanho; título ≤ 80, corpo ≤ 16 KiB; segredo nunca chega ao armazenamento | `test/contract/wiki_inv_test.go` | `TestINV06_SegredoNuncaChegaAoArmazenamento`; `TestINV06_RedacaoAntesDoLimiteDeCorpo`; `TestINV06_CorpoLimitadoA16KiB`; `TestINV06_TituloLimitadoA80Runas` | coberto |
| INV-07 | Saídas LLM estruturadas por JSON Schema | n/a | n/a | fora da 001 (F003) |
| INV-08 | `{provider, model, dim}` junto de todo embedding | n/a | n/a | fora da 001 (F003) |
| INV-09 | `reset`, `restore`, `reindex`, `uninstall --purge-data` checam processo vivo; backup online | `test/contract/runtime_inv_test.go` | `TestINV09_GuardaDeProcessoVivoAntesDeReindex` | coberto |
| INV-10 | Escrita de arquivo atômica (temp + fsync + rename + fsync do diretório) | `test/contract/wiki_inv_test.go` | `TestINV10_EscritaAtomicaSemTemporarioSobrando`; `TestINV10_WikiWriteUsaRenameSemTemporario` | coberto |
| INV-11 | Diretório de dados absoluto e canônico, registrado no log | `test/contract/runtime_inv_test.go` | `TestINV11_DataDirAbsolutoECanonicoNoLog` | coberto |
| INV-12 | Reservada (sem regra imposta; ADR-04) | n/a | n/a | reservada (sem teste) |
| INV-13 | Caminho padrão zero-LLM | `test/contract/runtime_inv_test.go` | `TestINV13_FuncionaSemProvedorLLM` | coberto |
| INV-14 | Autenticação do provedor resolvida antes do cliente | n/a | n/a | fora da 001 (F003) |
| INV-15 | Assinantes de log filtram o próprio módulo | `test/contract/runtime_inv_test.go` | `TestINV15_LogIgnoraProprioModulo` | coberto |
| INV-16 | Multi-sessão: páginas compartilhadas, supersede nunca destrói, leitura sem filtro de autoria | `test/contract/wiki_inv_test.go` | `TestINV16_SupersedeNuncaDestroi`; `TestINV16_ReadPageDevolveCorrenteSemFiltrarAutoria`; `TestINV16_EscritaConcorrenteDivergenteSupersede` | coberto (parcial, ver nota) |

**Nota INV-16:** os testes cobrem supersessão, leitura sem filtro de autoria e escrita concorrente divergente, que são escopo da 001 (`plan.md`). Handoff com reivindicação única (duas guardas) e ponteiro de projeto ativo por ator são escopo da feature 002 (`plan.md`, linha de INV-16) e não têm teste nesta matriz.

## Verificação por grep dos nomes

Comando: `grep -rn "func TestINV" test/contract/`. Todos os 23 nomes da tabela aparecem exatamente como escritos (arquivo:linha):

- `runtime_inv_test.go`: 211 (INV01), 280 (INV09), 325 (INV11), 404 (INV13), 463 (INV15)
- `storage_inv_test.go`: 179, 249, 317 (INV02); 351, 396 (INV03); 442, 474, 501, 547 (INV04)
- `wiki_inv_test.go`: 311, 350, 371, 399 (INV06); 420, 461 (INV10); 510, 568, 619 (INV16)

Não há `t.Skip` nos três arquivos (`grep "t.Skip" test/contract/*.go` sem resultado), portanto nenhum teste da tabela está desabilitado por skip.

## Débitos registrados

Nenhum. Todos os invariantes da 001 (INV-01 a 04, 06, 09, 10, 11, 13, 15, 16) têm teste nomeado. INV-12 é reservada, sem teste, por definição.

## Desvios e dúvidas

1. **INV-06, fronteira de tipo:** a constituição marca "(a confirmar) que não há outro caminho no original". Os testes cobrem segredo, ordem redação/limite e limites de tamanho, mas nenhum nome de teste cobre explicitamente a criação do tipo sanitizado só pela função de sanitização. Não é débito pelo critério atual (há teste nomeado), mas fica como lacuna de cobertura a confirmar.
2. **INV-16, parcialidade:** ver nota acima. Se o critério exigir a dupla guarda de handoff na 001, seria débito; `plan.md` diz que fica na 002, então não foi registrado como débito aqui.
3. **INV-01:** o teste nomeado existe, mas o critério "nenhum outro código lê `os.Getenv`" não foi verificado nesta task (não há execução de código, conforme escopo).
4. **Conteúdo dos testes não foi lido:** a verificação é de existência e nome (critério DONE da T077), não de que cada teste exercita integralmente o invariante.
5. **Referências a ADR:** a constituição cita ADR-04 para INV-12 e para o timeout de hook. Não foi conferido se o ADR-04 existe como arquivo em `specs/`; a matriz cita apenas a referência textual da constituição.
