# Adicionando um harness gerenciado

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

O suporte a workstreams gerenciados é mais restrito que o suporte a MCP ou a hooks de ciclo de vida. Esta versão gerencia Claude Code, Codex, OpenCode, OpenCode 2 (beta), Pi, Crush, OMP, Kimi Code, Command Code, Kiro CLI v2 e v3, Grok Build CLI, Antigravity CLI e GitHub Copilot CLI (14 adaptadores). Gemini CLI, Devin CLI, Cursor e as demais integrações da [matriz de suporte](support-matrix.md) não passam a ser gerenciadas apenas porque o ai-memory consegue capturar seus hooks.

Um adaptador gerenciado precisa:

- preservar a sessão nativa real do harness;
- entregar o delta portátil do workstream exatamente uma vez;
- importar apenas o histórico visível, sem escrever no armazenamento privado do harness.

Para o comportamento visto pelo usuário, veja [workstreams gerenciados](managed-workstreams.md).

## 1. Estabelecer o contrato nativo

Comece pela documentação atual do harness e por fixtures repetíveis. Pesquisas prévias de terceiros ajudam, mas não substituem a verificação da versão da CLI instalada.

Documente e teste:

- o executável padrão e as plataformas suportadas. O nome pode mudar por plataforma (Command Code usa outro nome no Windows);
- a sintaxe de nova sessão e de retomada, incluindo a posição do seletor;
- os seletores explícitos fornecidos pelo usuário;
- os modos não interativos e os subcomandos utilitários, que devem ser repassados sem alteração;
- as raízes do armazenamento de sessões, as variáveis de ambiente de override e as opções de caminho;
- o identificador de checkout gravado pelo harness e a unidade dos timestamps;
- registros completos versus em andamento, regras de pareamento de chamadas de ferramenta, registros de compactação e registros que contêm raciocínio oculto ou credenciais;
- uma forma suportada de injetar contexto inicial antes do primeiro turno do modelo;
- a opção nativa de aprovação automática ou bypass de permissões, se existir.

Se a identidade exata da sessão, a extração somente leitura ou a entrega de contexto antes do turno não puderem ser demonstradas, documente a limitação em vez de publicar um adaptador parcial.

## 2. Registrar o harness

### Tipo de agente

- Adicione um tipo de agente novo ou reutilize um existente. Duas variantes podem compartilhar o mesmo tipo: OpenCode 2 usa o tipo do OpenCode, e Kiro v2 e v3 usam o mesmo tipo. Nesses casos, a variante difere pelo executável e pelo motor/armazenamento, e sessões de motores diferentes não se retomam entre si.
- Um tipo novo deve entrar na lista completa de tipos de agente. Os testes do armazenamento percorrem essa lista para garantir que a restrição de tipos aceita todos.
- Um tipo novo exige uma migração nova, somente para frente, que reconstrói a restrição de tipos da tabela de sessões e os gatilhos de pareamento, preservando as linhas existentes. Nunca edite uma migração já lançada. Veja [esquema do banco](database-schema.md).

### Superfície explícita

- **Comando `ai-memory run`:** inclua a variante na lista de harnesses escolhíveis, com nome canônico e aliases.
- **Mapeamento de harness gerenciado:** defina o nome canônico estável, o executável padrão, o tipo de agente correspondente e se o harness emite evento de fim de sessão. Harnesses sem esse evento (Command Code, Kiro v2 e v3, Antigravity) mantêm a sessão aberta até `ai-memory finalize-session`.
- **Lista de validação do servidor:** o servidor aceita execuções gerenciadas somente de tipos listados explicitamente (hoje, os 12 tipos de agente: `claude-code`, `codex`, `open-code`, `pi`, `crush`, `omp`, `kimi-code`, `command-code`, `kiro-cli`, `grok`, `antigravity-cli` e `copilot-cli`; qualquer outro é recusado com `400 managed run requires a supported command-line harness`). Inclua o tipo novo nessa lista.
- **Seleção automática:** `ai-memory run` sem harness explícito escolhe entre uma lista do cliente (Claude Code, Codex, OpenCode, Pi, Crush, Kimi Code, Command Code, Kiro v2 e Kiro v3) e uma lista do servidor (os tipos `claude-code`, `codex`, `open-code`, `pi`, `crush`, `kimi-code`, `command-code` e `kiro-cli`; um agente automático fora dela é recusado). Não inclua o harness em nenhuma delas antes de a descoberta de candidatos no checkout ser confiável. A data de modificação de um arquivo local é apenas uma dica de bootstrap. Em um workstream já estabelecido, a ligação atual do harness no servidor prevalece.
- **Documentação:** atualize a matriz de suporte, a [arquitetura](architecture.md) e as [decisões de design](design-decisions.md), e registre a mudança no changelog do projeto. O changelog é gate de merge para mudanças visíveis ao usuário. Esse item é convenção de revisão; o código não o verifica.

## 3. Preservar o argv nativo e a posse da sessão

Implemente os modos de nova sessão, retomada e seletor explícito no componente de adaptadores de harness (veja [adaptadores de harness](harness-adapters.md)).

### Argumentos

- Preserve todos os argumentos do usuário, na mesma ordem, exceto os tokens exatos `--yolo`, `--true-yolo`, `--fresh`, `--force-unlock`, `--no-autowire`, `--jail`, `--jail=LISTA` e `--no-jail`, que pertencem ao wrapper. Todas as ocorrências desses tokens são removidas (um argumento que não é UTF-8 nunca é tocado).
- Um seletor nativo explícito sempre vence o vínculo do ai-memory.
- Ajuda, versão, modo sem persistência (`--no-session` e equivalentes) e subcomandos utilitários (como login, doctor e export) rodam em modo passthrough: nenhuma flag de sessão é injetada.
- Modos não interativos (exec, run, print, prompt e equivalentes) continuam sendo sessões, mas não oferecem adoção de sessão existente.

### Seletores por harness

Ao retomar um vínculo, o wrapper injeta o seletor abaixo somente se o usuário não passou um seletor nativo.

| Harness | Nova sessão | Retomada do vínculo |
|---|---|---|
| Claude Code | `--session-id <uuid>` gerado | `--resume <id>` |
| Codex | nenhum (descoberta após a saída) | `resume <id>` no início; `exec resume <id>` em modo não interativo |
| OpenCode e OpenCode 2 | nenhum (descoberta após a saída) | `--session <id>` no início; após `run`, se for esse o subcomando |
| Pi | `--session-id <uuid>` gerado | `--session <id>` |
| Crush | nenhum | `--session <id>` no início |
| OMP | nenhum | `--resume=<id>` no fim |
| Kimi Code | nenhum | `--session <id>` no fim |
| Command Code | nenhum | `--session <id>` no fim |
| Kiro v2 e v3 | nenhum | `--resume-id <id>` no fim |
| Grok Build | `--session-id <uuid>` gerado | `--resume <id>` |
| Antigravity | nenhum | `--conversation <id>` no fim |
| Copilot CLI | `--session-id <uuid>` gerado, no fim | `--resume=<id>` (forma com `=`) no fim |

Regras complementares:

- Gere um id próprio só quando a CLI aceitar um id fornecido pelo chamador. Hoje, isso vale para Claude Code, Pi, Grok e Copilot CLI. Nos demais, deixe o harness criar a sessão e descubra-a após a saída, pelo checkout exato e pelo horário de início.
- Não infira a sessão a partir de "a mais recente globalmente".
- Em Kiro v3, se nenhum motor for escolhido, a opção de motor v3 é inserida no início do argv.

### `--yolo`

- Mapeie `--yolo` apenas para uma opção nativa verificada, sem duplicá-la: se já houver uma flag equivalente (inclusive sinônimos documentados pelo harness), não adicione outra.
- Se o harness não tiver equivalente, não adicione flag nenhuma e documente isso. OMP e Kiro v3 não têm equivalente.
- Não amplie uma lista de confiança restrita escolhida pelo usuário.
- `--true-yolo` vale apenas para Claude Code: acrescenta uma configuração que força o modo de bypass de permissões. Regras explícitas de pergunta e de negação continuam valendo.

### `--fresh`

- Antes de injetar um seletor de retomada, verifique se o id vinculado existe no armazenamento nativo.
- Ausência confirmada: a execução começa uma sessão nova, e o vínculo é trocado depois que a nova sessão estiver estabelecida.
- Armazenamento ilegível ou malformado: o vínculo é mantido, um aviso é impresso e a execução segue com a retomada nativa. Isso não é tratado como ausência.
- `--fresh` combinado com um seletor nativo explícito (retomada, sessão, continuação ou fork) é erro.

### Permissões e sandbox

- Quando `--yolo` é pedido, o wrapper avisa e oferece executar dentro de um sandbox externo opcional, antes de qualquer trabalho de sessão. O adaptador não implementa o sandbox. Veja [o design de yolo com sandbox](design-yolo-safety-ai-jail.md).
- No re-exec sob sandbox, o argv original é repassado. Apenas uma lista fixa de variáveis de ambiente é repassada (URL do servidor, token, diretório de dados e chaves de provedores). Se o harness lê credenciais de variáveis de ambiente, adicione-as a essa lista; caso contrário, elas ficam invisíveis dentro do sandbox (a confirmar se há outras exigências de sandbox por harness).

## 4. Descobrir e exportar somente leitura

Implemente a descoberta de candidatos no componente de transcrições (veja [adaptadores de harness](harness-adapters.md)). Implemente a exportação incremental somente quando houver um formato documentado, ou observado de forma repetível, que exponha registros visíveis sem estado privado.

### Descoberta

- Considere apenas sessões cujo checkout registrado é exatamente o checkout atual. A comparação de caminhos usa caminho canônico; se a canonicalização falhar, a comparação falha fechada (não casa).
- Ordene por data de modificação, do mais recente para o mais antigo; em caso de empate, ordene pelo caminho.
- Considere no máximo 512 candidatos. Pare no primeiro arquivo modificado mais de 2 segundos antes do início do lançamento.
- A varredura de diretórios é limitada a 50 000 arquivos.
- Respeite as raízes de armazenamento definidas pelas variáveis de ambiente e opções de linha de comando documentadas pelo harness.
- Na verificação de existência para retomada, registro ausente significa "não existe". Falha de acesso ou de formato é erro. Um armazenamento ilegível não pode ser confundido com uma sessão apagada.

### Acesso ao armazenamento

- Abra os bancos locais do harness somente em leitura. Nunca crie, migre, compacte ou repare esses arquivos.

### Exportação incremental (formato de uma linha JSON por registro)

- Um registro final sem quebra de linha (incompleto ou ainda sendo escrito) não é consumido, e o cursor não avança além dele.
- Uma linha completa malformada registra uma perda com o deslocamento em bytes e é pulada; o cursor avança.
- Id do registro: o id de origem, quando existe; senão, o deslocamento em bytes. Em diários que o harness reescreve in place (fork, compactação, retomada ou rewind), use o hash SHA-256 da linha. Antes de confiar no cursor, valide o prefixo já consumido pelo hash de todos os bytes até o deslocamento. Se o hash não bater, recomece do zero.
- Id do evento: hash determinístico do tipo de agente, da sessão, do id do registro, do índice do bloco, do tipo do evento e do conteúdo, com prefixo `native:`. A mesma entrada gera o mesmo id, e a reimportação é idempotente.
- O conteúdo de cada evento é truncado em 128 KiB, respeitando os limites de caractere UTF-8. O servidor limita o conteúdo de cada evento a 64 KiB e aceita no máximo 4 096 eventos por finalização.
- O cursor de origem é um texto opaco, persistido apenas para a próxima leitura local.

### Normalização

Importe apenas:

- mensagens visíveis de usuário e de assistente;
- chamadas e resultados de ferramenta concluídos;
- resumos de compactação.

Exclua, anotando a perda quando houver:

- mensagens de sistema ou de desenvolvedor, inclusive instruções privadas;
- raciocínio oculto, anotado com o nome do harness;
- chamadas de ferramenta não concluídas, nos adaptadores que expõem o status (Crush e OpenCode 2);
- mídia e conteúdo binário (por exemplo, resultados de imagem), anotados;
- metadados de provedor e credenciais, que não são copiados. O servidor também higieniza o conteúdo antes de gravar o segmento bruto; a cobertura é total e independente do formato do harness (a rota remove segredos do `content` de **todo** evento e só depois corta em 64 KiB; `source_record_id` e `metadata` passam pela lista de campos permitidos; o armazenamento do ledger higieniza de novo e limita a 16 KiB — ver [managed-workstreams.md](managed-workstreams.md#eventos-e-importação)). Os adaptadores, porém, **não** devem contar com isso para campos de identidade: o identificador nativo é validado pelo filtro de privacidade antes de qualquer uso;
- conteúdo sintético injetado pelo próprio harness e o pacote de contexto gerenciado já entregue, para não reimportar o próprio contexto.

As anotações de perda são deduplicadas. Se o conteúdo for opaco ou não documentado, devolva uma anotação de perda e conte com a captura de ciclo de vida sanitizada, sem adivinhar. Nunca copie um registro privado "por precaução".

## 5. Entregar o contexto antes de confirmá-lo

### Caminho preferido: hook de início de sessão

- O processo filho herda a variável `AI_MEMORY_RUN_ID`. O hook vincula a sessão nativa real, renderiza o trecho do workstream ainda não visto e o torna visível ao modelo.
- O pacote tem no máximo 30 000 caracteres, com até 6 000 caracteres por evento. Os eventos mais recentes entram primeiro. Se eventos mais antigos não couberem, o pacote avisa que houve omissão e indica a busca no histórico visível.
- Chamadas históricas de ferramenta são rotuladas como evidência concluída, para que outro harness não as trate como trabalho pendente. O histórico não confiável é delimitado por um aviso de segurança.
- Limitação atual: o servidor registra a entrega (marca o contexto como entregue e avança o cursor de entrega) na mesma requisição em que devolve o contexto ao hook, antes de o hook imprimi-lo. Portanto, a confirmação precede a visibilidade ao modelo (a confirmar: o que acontece se a resposta se perder depois dessa confirmação).

### Caminho do lançador: Crush e Grok

Use este caminho quando o harness não tem hook de início adequado, mas oferece um mecanismo nativo de contexto.

- **Crush (referência):** o lançador busca o pacote sem confirmá-lo; escreve uma cópia privada temporária da configuração global suportada, junto com um arquivo de contexto efêmero; inicia o processo; e confirma o pacote somente depois que o processo for criado com sucesso. A configuração original e o armazenamento de sessões nunca são escritos pelo ai-memory. O harness mantém suas escritas nativas normais, e o diretório temporário é removido ao sair.
- **Grok:** o pacote é passado por uma flag nativa de regras, que aceita um único uso. Se o usuário já tiver passado essa flag, o pacote não é entregue nessa execução e fica para a próxima execução que puder confirmá-lo.
- A busca do pacote é repetível até a confirmação.
- Falha na criação do processo: a execução é finalizada com a perda registrada, e o pacote não é confirmado, portanto será reenviado depois.
- Falha na confirmação: um aviso é impresso, e o pacote é reenviado na próxima execução.
- Uma falha nunca deve perder o contexto silenciosamente.

## 6. Manter os invariantes do workstream

- Um workstream lógico tem no máximo uma sessão nativa atual por harness.
- Um harness que entra em um workstream já estabelecido inicia uma sessão nativa limpa e recebe o histórico portátil. Ele não adota uma sessão local antiga e não relacionada.
- A adoção no primeiro uso só é permitida enquanto o estado autoritativo do servidor não tiver sessão nativa vinculada nem evento portátil substantivo (mensagem, chamada ou resultado de ferramenta, ou compactação). Checkpoints e anotações não contam.
- Se o id vinculado sumiu do armazenamento nativo, a execução começa uma sessão nova e o vínculo é trocado depois que ela for estabelecida. O workstream não fica preso a um id inexistente.
- Reutilize o caminho compartilhado: leases, cursores de entrega e de origem, segmentos sanitizados imutáveis, limites de lote e importação idempotente. Não crie um banco de sincronização específico do harness, nem altere o armazenamento privado para resolver precedência, renomeação de diretório ou conflito.

## 7. Testes e script de aceitação

### Cobertura esperada

A cobertura abaixo é convenção de revisão. O repositório não verifica a cobertura automaticamente, mas o script de aceitação cobre parte dela.

- Lançamento novo, retomada vinculada, recuperação de id ausente, precedência do seletor explícito, ordem dos argumentos, passthrough de utilitários, overrides de caminho, mapeamento de `--yolo` e de `--fresh`.
- Ordenação de candidatos, isolamento por checkout, tratamento de timestamps, acesso somente leitura, cursores incrementais, ids estáveis, registros incompletos, inclusão de registros visíveis e exclusão de registros privados.
- Adoção no primeiro uso e guarda contra sessão obsoleta em workstream estabelecido.
- Ordem de busca, injeção e confirmação do contexto, e reentrega após falha de criação do processo.
- Invariante do tipo de agente quando um tipo novo é adicionado (lista completa e migração).
- Teste manual com o harness real.

### Script de aceitação

O script de aceitação de workstreams gerenciados do repositório não é chamado pela integração contínua. Ele tem duas fases:

- **Fase determinística:** usa harnesses falsos e não precisa de credenciais. Pode rodar localmente com frequência. Com `AI_MEMORY_ACCEPTANCE_DETERMINISTIC_ONLY=1`, o script para ao fim dessa fase.
- **Fase real (opt-in):** usa as CLIs instaladas e as credenciais do operador, e isola cada etapa em um ambiente de teste. Falha se a configuração de agentes do operador mudar durante a execução. Os harnesses são escolhidos com `AI_MEMORY_ACCEPTANCE_HARNESSES`. Kiro é pulado nessa fase, porque a modalidade não interativa usa outro armazenamento; a aceitação interativa do Kiro deve ser rodada separadamente.

Critérios de cada etapa real:

- Se o adaptador tem transcrição legível, deve persistir um novo evento de assistente depois da sequência anterior do ledger.
- Se a etapa deve receber contexto, a execução gerenciada deve confirmar o cursor de entrega até a sequência anterior e marcar o contexto como entregue.
- Adaptador somente por hook (hoje, Antigravity): deve persistir uma observação do hook de início e vincular a conversa nativa exata.
- Ao voltar ao primeiro harness, a sessão nativa original deve ser retomada.

Não faça o resultado depender de o modelo citar o texto do pacote. Pacotes grandes podem ser externalizados para um arquivo, e a leitura depende de uma escolha do modelo.

### Gate antes de pedir revisão

O projeto exige os seguintes gates, e a integração contínua os verifica:

```text
1. formatação: verificar sem alterar arquivos
2. diff: nenhum erro de espaço em branco ou fim de linha nas alterações
3. testes: executar todos os testes do workspace, incluindo alvos de exemplo e benchmark
4. lint: análise estática com avisos tratados como erro
5. dependências: auditar licenças e vulnerabilidades
```

Registre na descrição do PR a versão da CLI testada e qualquer limitação de plataforma.
