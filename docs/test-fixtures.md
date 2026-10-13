# Apoio a testes: servidor fora do ar e fixtures de contrato

Este documento descreve dois apoios de teste: um endpoint HTTP que falha na hora e os
fixtures de contrato que todas as implementações devem satisfazer. Os utilitários
são apenas de teste e não fazem parte do produto. Os fixtures são dados neutros de
linguagem.

## 1. Dublê de "servidor fora do ar"

Testes do cliente de hook precisam de um endpoint HTTP que **falhe imediatamente**.

| Requisito | Detalhe |
|---|---|
| Comportamento | Escuta em `127.0.0.1` numa porta efêmera e **aceita e fecha** cada conexão sem responder. A escuta dura até o fim do processo de teste. |
| Por quê | Apontar para uma porta fechada falha em microssegundos em Linux e macOS, mas no Windows o SYN é retransmitido (cerca de 2 s por teste). Aceitar e fechar falha na hora em todas as plataformas. |
| Fallback | Se o bind falhar ou o endereço local não puder ser lido, usar `http://127.0.0.1:1`. Assim os testes mantêm o comportamento antigo em ambientes sem rede. |
| Classificação no cliente | Qualquer erro de transporte (conexão recusada, fechamento sem resposta HTTP ou timeout) é **inalcançável**. Esse erro não consome tentativas da fila local (spool). Um endereço morto é limitado pelo TTL e pelo limite de arquivos da fila. Ver [cli.md](cli.md) §5.3. |

## 2. Fixtures de contrato

Os fixtures são arquivos de dados (JSON). Ficam junto aos testes de cada componente e
são lidos por testes de mais de uma implementação. A tabela lista o que cada um cobre e
onde o comportamento está descrito.

| Fixture | Cobertura | Descrito em |
|---|---|---|
| `capture-policy.json` | Normalização e casamento de `ignore_paths`; decisões por agente (`drop`, `keep`, `metadata-only`); vetores de comandos de terminal; envelope de protocolo `_ai_memory_capture`. | [lifecycle-event-ingestion.md](lifecycle-event-ingestion.md) §4; [marker-file.md](marker-file.md), seção "Capture exclusions" |
| `home_route_cases.json` | Gramática e seleção das rotas do arquivo pessoal de marcação. Ver 2.1. | Esta página (2.1) |
| `remote_identity_cases.json` | Normalização de remote git, derivação de nomes, estilo de identidade e aliases do marcador. Ver 2.2. | Esta página (2.2); [complementary-contracts.md](complementary-contracts.md), seção de identidade de repositório |
| `kiro-v3-hook-payloads.json` | Cinco eventos de uma sessão real sanitizada do Kiro CLI v3 (2026-08-07): início de sessão, envio de prompt, antes e depois de ferramenta, e parada. | [lifecycle-event-ingestion.md](lifecycle-event-ingestion.md) §3 |

Os cinco eventos do fixture de Kiro compartilham o mesmo diretório de trabalho e o mesmo
identificador de sessão. Um teste deve confirmar que o contexto extraído é igual para
todos os eventos.

### 2.1 Rotas da home (`home_route_cases.json`)

O arquivo pessoal de marcação fica no diretório pessoal do usuário e declara rotas em
tabelas de dois tipos: por **identidade** de repositório e por **caminho**.

**Limites**

- No máximo 64 rotas, somando identidade e caminho.
- Cada seletor (a chave da tabela) tem até 512 bytes UTF-8. Cada valor também tem até 512 bytes.
- O arquivo inteiro tem até 64 KiB.

**Validade (falha fechada)**

- Seletor repetido é inválido, inclusive entre tipos (identidade e caminho).
- Dois caminhos que normalizam para o mesmo valor são inválidos (por exemplo, `~/work/api` e `~/work/./api`).
- Campo repetido ou desconhecido invalida a rota.
- Arquivo malformado é inválido por inteiro. Um erro nunca cai para as configurações da raiz da home.
- Host de identidade não pode começar nem terminar com ponto.

**Seleção**

- Rota por identidade vence rota por caminho, mesmo que um caminho também case.
- Entre caminhos, vence o prefixo mais longo, comparado por componentes. `~/work/api` não casa com `~/work/api-sibling`.
- `~` expande para a home e não pode sair dela. `~/..` e `~/a/../..` são inválidos. `~/a/../b` normaliza para dentro da home.
- São aceitos caminhos absolutos POSIX, caminhos com letra de drive do Windows (o casamento ignora maiúsculas e minúsculas) e caminhos UNC.
- Uma rota casada substitui apenas os campos de roteamento. Aliases só são encaminhados quando há evidência de remote.
- Se nenhuma rota casa, valem as configurações da raiz da home.
- A seleção funciona para diretórios fora da home. Um marcador local (o `.ai-memory.toml` mais próximo) vence a rota da home.

O caso de identidade também verifica as bandeiras de sub-agente, recall global, briefing (ativação e orçamento de caracteres) e perfil (contribuir e consumir).

### 2.2 Identidade de remote e aliases (`remote_identity_cases.json`)

Este fixture é compartilhado por todo cliente que normaliza remote git ou lê aliases
locais. A gramática de aliases é deliberadamente limitada e portátil. Um caso novo
é verificado em todos os clientes, então eles não se separam.

**Normalização do remote**

- Espaços nas bordas são removidos. Entrada vazia ou só com espaços não tem identidade.
- Formas aceitas: scp (`usuario@host:caminho`) e URL (`esquema://...`). Um esquema vazio também é aceito.
- O esquema `file` não tem identidade. Caminhos locais (absolutos, relativos, com `~`, com barra invertida ou com letra de drive) também não têm identidade.
- Credenciais saem: tudo até o último `@` antes do caminho é removido, então uma senha com `@` não vaza.
- Porta numérica é removida na forma de URL.
- Host sem caminho não tem identidade. O resultado precisa ter ao menos uma barra.
- Resultado final: minúsculas, barras finais removidas, um único sufixo `.git` final removido e barras repetidas colapsadas.

Exemplos do fixture:

- `git@github.com:Acme/API.git` vira `github.com/acme/api`.
- `https://github.com/acme/api.git.git` vira `github.com/acme/api.git`.
- `https://github.com//acme//api` vira `github.com/acme/api`.
- `ssh://git@ssh.dev.azure.com:22/v3/acme/proj/api` vira `ssh.dev.azure.com/v3/acme/proj/api`.
- `https://github.com` e `/srv/git/api.git` não têm identidade.

**Derivação de nomes**

- *Nome curto:* para identidade com caminho, são os dois últimos segmentos unidos por hífen. Exemplo: `github.com/orgb/api` vira `orgb-api`. Caracteres que um nome de projeto não aceita viram hífen, sequências de hífen colapsam e as bordas são aparadas. Resultado vazio vira `repository`.
- *Nome em estilo caminho:* só para remote git. É o caminho inteiro, sem o host, unido por hífen. Exemplo: `github.com/acme/group/api` vira `acme-group-api`.
- *Chave com estilo:* a origem `git_remote` tem caminho sem host. As origens `explicit` e `folder_name` não têm nome de caminho.
- *Estilo de identidade:* os valores `path` e `host_path` são aceitos depois de aparar espaços. Diferença de maiúsculas (`Path`), `host-path` e vazio resultam em nenhum estilo.

**Aliases do marcador**

- A chave fica na raiz do arquivo, em uma linha, como lista de strings. Não há escapes, declaração repetida, tabela (dentro de `[capture]` é inválido), lista multilinha, vírgula final, comentário ou texto depois da lista.
- Chave ausente tem status "absent". Lista vazia, mesmo com espaços dentro dos colchetes, é válida e vazia.
- Cada alias é aparado. Duplicatas saem e a primeira ocorrência mantém a posição.
- No máximo 16 entradas declaradas. Acima disso é inválido, mesmo que as entradas se repitam.
- Cada alias tem até 128 bytes UTF-8. Com 128 bytes é válido; com 129, inválido.
- Alias não pode começar com `-` nem conter separador de caminho (por exemplo, `../foreign` e `nested/project` são inválidos).
- Sequência de escape como `\n` é inválida.

## 3. Convenções de teste recomendadas

- **Um binário de integração por componente**, com a suíte em um diretório próprio. Cada binário extra custa link e, em macOS e Windows, uma varredura na primeira execução.
- **Testes adversariais de fronteira de segurança:** tentar a violação, afirmar a recusa, incluir um controle legítimo e comprovar que o teste falha sem a guarda.
- **Repositórios git de fixture não herdam configuração da máquina.** A assinatura de commits fica desligada explicitamente e a identidade de commit (nome e e-mail) é fixa. Uma assinatura global obrigatória quebraria os commits de fixture.
