# Migrações de estrutura da wiki

Migrações de estrutura da wiki são transformações únicas nos **arquivos** da wiki, no diretório `<dados>/wiki/`, quando uma versão nova do ai-memory exige um layout diferente do que uma versão anterior criou. Elas não tratam do esquema do banco de dados.

## Escopo e fronteiras

Há três mecanismos de mudança de dados. Eles não se confundem:

| Mecanismo | O que altera | Quando roda | Onde está descrito |
|---|---|---|---|
| Migrações de esquema do banco | Tabelas e colunas do SQLite | Na abertura do banco, antes da wiki, automaticamente | [database-schema.md](database-schema.md) |
| Backfills do banco | Dados derivados no banco, como as janelas de ingestão das páginas | Na abertura do banco, de forma idempotente e em lotes | [database-schema.md](database-schema.md) |
| Migrações de estrutura da wiki (este documento) | Arquivos e diretórios da wiki, e o registro de aplicação em `wiki_migrations` | Na inicialização do servidor, depois do banco | Este documento |

O preenchimento das janelas de ingestão das páginas é um backfill do banco. Não é uma migração da wiki.

Hoje existe uma única migração de estrutura da wiki registrada: a de conformidade com o formato OKF v0.2, descrita em [okf.md](okf.md). O layout atual da wiki, com diretórios por espaço de trabalho e projeto, é o formato nativo desde a primeira versão e não exige migração.

## Quando escrever uma migração

Escreva uma migração quando uma versão nova exigir que um diretório da wiki criado por uma versão anterior seja reestruturado. Exemplos:

- o esquema de caminhos muda (por exemplo, páginas na raiz da wiki passam a ficar dentro de diretórios por espaço de trabalho e projeto);
- um diretório é renomeado, dividido ou fundido;
- toda página de um tipo passa a exigir um campo novo no frontmatter;
- muda o padrão de nome dos arquivos de backup do log.

Não escreva migração para mudanças puramente aditivas e compatíveis com versões anteriores, como um campo novo e opcional no frontmatter que tenha valor padrão nulo.

## Registro e ordem

- Todas as migrações ficam em uma lista única e são executadas na ordem em que aparecem.
- Uma migração nova entra sempre no fim da lista. Nunca reordene nem remova entradas: a ordem, junto com a tabela de registro, determina quais migrações estão pendentes.
- Cada migração tem um nome único e ordenável, no formato `AAAA_MM_DDTHH_MM_<descricao_em_snake_case>`, em UTC. O nome é escolhido na autoria, não na aplicação. Depois de publicado, nunca muda, pois é a chave de controle na tabela de registro.
- Cada migração tem uma descrição de uma linha, usada nos logs.

## Contrato de uma migração

Uma migração expõe um nome, uma descrição e uma operação de aplicação. A operação recebe o escritor único do banco e a raiz da wiki (`<dados>/wiki/`), e retorna sucesso ou erro.

Regras:

- **Idempotente.** Se o trabalho já estiver feito (arquivos já no layout de destino, diretório de origem já ausente), a operação retorna sucesso sem alterar nada. O executor registra a migração no primeiro sucesso e não a executa de novo, mas a implementação deve ser idempotente mesmo assim, porque uma falha ao registrar faz a migração rodar outra vez (ver a seção Falhas).
- **Sem exclusão sem cópia prévia.** Dados que forem removidos devem ser movidos antes para `<raiz da wiki>/_graveyard/<carimbo de tempo>/<caminho original>`, para continuarem recuperáveis por pelo menos um ciclo de versões.
- **Sem chamadas a modelos de linguagem.** Migrações rodam em toda inicialização, então devem ser rápidas e sem custo. Transformações que dependem de modelo pertencem a um comando manual único ou a um job de consolidação.
- **Sem SQL fora do escritor único.** Ajustes no índice do banco feitos junto com movimentos de arquivo passam pelos métodos do escritor único. Não abra uma segunda conexão com o banco nem chame diretamente as operações internas do banco. Assim se preserva a regra de escrita única.
- **Testes.** Cada migração deve ter um teste em diretório temporário que verifique: a pré-condição (layout antigo presente), a pós-condição (layout novo presente e antigo ausente) e a reexecução, que deve retornar sucesso sem efeito.

## Quando roda

- Somente na inicialização do servidor. Não há comando manual para aplicar migrações da wiki.
- A sequência de inicialização, nesta ordem, é:
  1. trava de instância única sobre o diretório de dados;
  2. cópia de segurança prévia à migração de esquema (ver a seção Backup prévio);
  3. abertura do banco, com as migrações de esquema e os backfills;
  4. saneamento de dados e verificações de credenciais e de configuração;
  5. migrações de estrutura da wiki;
  6. início do observador de arquivos da wiki e dos demais componentes.
- Restaurar uma cópia de segurança não aplica migrações da wiki. A restauração apenas abre o banco restaurado para validá-lo. As migrações da wiki rodam na próxima inicialização do servidor.

## Algoritmo do executor

1. Se a lista de migrações estiver vazia, retorne sucesso sem consultar nada.
2. Leia da tabela `wiki_migrations` os nomes já aplicados.
3. Se algum nome aplicado não existir na lista deste build, recuse a inicialização (ver a seção Recusa de formato mais novo). Nenhuma pendência é executada nesse caso.
4. Para cada migração, na ordem da lista:
   1. se o nome já estiver aplicado, pule;
   2. registre no log o início da execução, com nome e descrição, e execute a operação de aplicação;
   3. se a operação retornar erro, registre o erro no log, interrompa o executor e propague o erro. O nome não é registrado;
   4. se a operação tiver sucesso, registre o nome na tabela com o instante de aplicação, e registre no log que a migração foi aplicada.
5. Retorne sucesso.

## Registro de aplicação

A tabela `wiki_migrations` tem duas colunas:

- `name`: texto, chave primária;
- `applied_at`: inteiro, em microssegundos desde a época Unix, em UTC.

A tabela é criada por uma migração de esquema do banco (a de número 6), que roda antes de qualquer migração da wiki. A inserção ignora nomes já existentes.

Para listar as migrações aplicadas:

```sql
SELECT name, datetime(applied_at / 1000000, 'unixepoch') AS applied
FROM wiki_migrations
ORDER BY name;
```

## Backup prévio

- O backup prévio não faz parte do executor. Ele acontece na inicialização, antes de o banco ser aberto, independentemente de haver migrações pendentes.
- A decisão de criar a cópia é da migração de conformidade OKF. Ela só cria cópia quando há páginas não conformes e não existe cópia anterior ainda presente. Destino, conteúdo, verificação e recibo estão descritos em [okf.md](okf.md) e [backup.md](backup.md).
- Se a cópia falhar, a inicialização para antes de abrir o banco. Nada é migrado.
- Uma cópia válida nunca é sobrescrita por uma nova.

## Falhas

- Um erro em qualquer migração interrompe o executor. As migrações seguintes não rodam nesta inicialização, a que falhou não é registrada, e o servidor não sobe. O erro indica que a aplicação das migrações da wiki falhou.
- Na inicialização seguinte, as pendências são tentadas de novo, a partir da primeira que não estiver registrada.
- O executor não desfaz alterações parciais feitas antes do erro. A proteção contra isso é a idempotência de cada migração e a cópia prévia.
- Se a operação tiver sucesso mas o registro na tabela falhar, a inicialização também para. Na próxima, a migração roda de novo. Por isso a idempotência é obrigatória.

## Recusa de formato mais novo

- **Condição:** a tabela `wiki_migrations` contém um nome que o build atual não conhece. Isso acontece quando uma versão mais nova do ai-memory já migrou a wiki.
- **Efeito:** o servidor não sobe, e a verificação acontece antes de qualquer migração pendente rodar. A mensagem informa que a wiki foi migrada por uma versão mais nova, cita o nome desconhecido e indica duas saídas: atualizar o binário, ou restaurar a cópia de segurança anterior à migração.
- **Objetivo:** impedir que um build antigo abra a wiki e misture formatos.
- **Limite:** a verificação só ocorre quando a lista de migrações do build não está vazia. Um build sem migrações registradas não recusa uma wiki de formato mais novo.
- O procedimento de recuperação completo está em [okf.md](okf.md). A recusa do esquema do banco é um mecanismo separado, descrito em [database-schema.md](database-schema.md).
