# language: pt
# Revisado na auditoria contra a referência de 2026-10-18 (passada 2). Fontes: docs/wiki-files-layer.md,
# docs/storage-and-retrieval.md, docs/prompts-schemas-and-sanitizer.md §11. O doc vence este arquivo.
Funcionalidade: Núcleo do ai-memory-go (domínio, armazenamento e wiki)
  Como agente de código
  Quero escrever, buscar e ler páginas de memória de longo prazo
  Para que o conhecimento do projeto persista entre sessões, com arquivo e índice consistentes

  Cenário: Escrever uma página atualiza arquivo e índice juntos, sem commit nem ledger
    Dado que o projeto "demo" já existe no workspace "default"
    Quando o chamador pede a escrita da página "decisions/db.md" no projeto "demo" com tier "semantic"
    Então o caminho passa pelo confinamento e por "ensure_portable" antes de qualquer escrita
    E o corpo e todo valor de texto do frontmatter passam pelo sanitizador configurado
    E o arquivo "decisions/db.md" é instalado de forma atômica antes da gravação no índice
    E a página, os índices FTS, de entidades e de links são commitados em uma única transação do escritor único
    E o frontmatter gravado tem "tier: semantic" e o mapeamento "generated" com "by" e "at"
    E nenhum commit git é criado pela escrita de página
    E nenhuma linha é anexada a "log.md"
    E o caminho fica registrado para o próximo commit automático
    E a resposta traz o identificador da nova versão

  Cenário: Escrita de página não cria escopo
    Dado que o projeto com o identificador informado não existe no banco
    Quando o chamador pede a escrita de uma página nesse projeto pela camada da wiki
    Então a escrita falha com erro de não encontrado
    E nenhum workspace nem projeto é criado
    E nenhum arquivo é instalado

  Cenário: Reescrever conteúdo idêntico não cria versão e mantém os bytes
    Dado que a página "decisions/db.md" foi escrita com corpo e frontmatter X
    Quando o chamador escreve a mesma página com o mesmo corpo, frontmatter, título, tier e fixação
    Então nenhuma nova versão é criada e o identificador devolvido é o da versão atual
    E o "generated.at" do arquivo atual é herdado, de modo que os bytes do arquivo são os mesmos
    E nenhuma linha de auditoria é gravada
    E a evidência da escrita é registrada contra a versão atual

  Cenário: Fixação não é herdada da versão anterior
    Dado que a página "decisions/db.md" está fixada com "pinned: true"
    Quando o chamador reescreve a página sem pedir fixação e sem a chave "pinned"
    Então a nova versão não está fixada
    Mas uma página sob "_slots/" é sempre fixada

  Cenário: Busca híbrida retorna páginas ranqueadas por RRF com autoridade aplicada
    Dado que o escopo contém páginas que casam por FTS, por entidade e por vizinhança de link
    E uma página casa tanto por FTS quanto por entidade
    E outra página casa apenas por um dos fluxos
    Quando o chamador faz uma consulta que casa as duas páginas
    Então a página que casa FTS e entidade aparece acima da que casa só um fluxo
    E o ranqueamento usa RRF com k igual a 60 e posição a partir de 1
    E as sementes do grafo são as páginas do FTS seguidas das de entidades, sem intercalar
    E o fator de autoridade fica no intervalo [0,55; 1,50]

  Cenário: Tipo de página na autoridade vem do kind ou do prefixo do caminho
    Dado a página "decisions/x.md" sem chave "kind" e a página "notes/y.md" com "kind: decision"
    Quando a autoridade das duas é calculada
    Então as duas recebem o ajuste de tipo "decision" (+0,15)
    E a página "Decisions/z.md" sem "kind" também recebe esse ajuste

  Cenário: Leitura devolve a versão corrente após supersessão
    Dado que a página "decisions/db.md" do projeto "demo" foi escrita em uma versão anterior
    E que uma nova versão da mesma página supersede a anterior
    Quando o chamador lê a página "decisions/db.md" do projeto "demo"
    Então a resposta devolve a versão corrente da página
    E a versão anterior continua alcançável pela cadeia, mas não como corrente

  Cenário: Falha no banco após instalar o arquivo reverte a página
    Dado que o arquivo "decisions/db.md" foi instalado com sucesso no disco
    E que a transação do banco falha em seguida
    Quando o chamador pede a escrita da página "decisions/db.md" no projeto "demo"
    Então o arquivo "decisions/db.md" volta ao conteúdo anterior, ou é removido se não existia
    E o erro do banco é propagado ao chamador
    E o índice permanece inalterado
    Mas se a reversão do arquivo também falhar, o erro é de estado inconsistente

  Cenário: Leitura com escopo explícito inexistente falha fechada
    Dado que não existe projeto "fantasma" no workspace "default"
    Quando o chamador pede a leitura de uma página informando workspace "default" e projeto "fantasma"
    Então a operação falha com erro de não encontrado
    E nenhum workspace é criado
    E nenhum projeto é criado

  Cenário: Leitura sem escopo usa o padrão e informa a origem
    Dado que não há ponteiro de projeto ativo para o chamador
    Quando o chamador pede uma leitura sem informar workspace nem projeto
    Então a leitura usa o projeto padrão do servidor
    E a origem da resolução é "default"

  Cenário: Workspace sem projeto é erro de entrada
    Quando o chamador informa só o workspace numa leitura
    Então a operação falha com erro de entrada inválida

  Cenário: Duas sessões escrevem o mesmo caminho ao mesmo tempo
    Dado que duas sessões distintas do projeto "demo" existem
    E que as duas sessões escrevem a página "decisions/db.md" simultaneamente com conteúdos diferentes
    Quando as duas escritas são processadas pelo escritor único
    Então uma versão é escrita e a outra a supersede numa cadeia de supersessão
    E a versão perdedora continua alcançável pela cadeia
    E o arquivo no disco corresponde à versão corrente do índice
    E a leitura da página não é filtrada por autoria

  Cenário: Edição manual da wiki no disco é reindexada
    Dado que a página "decisions/db.md" do projeto "demo" está indexada
    Quando o usuário edita manualmente o arquivo "decisions/db.md" no disco
    Então o watcher reindexa a página após o debounce de 300 ms
    Ou a reconciliação reindexa a página no ciclo de 30 s caso o watcher não tenha capturado a alteração
    E arquivos com o prefixo ".ai-memory-tmp." são ignorados
    E a reindexação de conteúdo idêntico não cria versão

  Cenário: Arquivo com extensão em maiúsculas não é página
    Quando o usuário cria o arquivo "decisions/NOTA.MD" no diretório do projeto
    Então o arquivo não é indexado

  Cenário: Diretório de projeto sem linha no banco é ignorado
    Dado que existe na wiki um diretório "<ws>/<proj>" cujos identificadores não estão no banco
    Quando a reconciliação percorre a wiki
    Então o diretório inteiro é pulado e contado como órfão
    E nenhum escopo é criado

  Cenário: Segredo no corpo de uma página é removido sem teto de tamanho
    Dado que o corpo e o título explícito de uma página contêm um segredo
    E que o corpo tem mais de 16 KiB e o título mais de 80 caracteres
    Quando o chamador pede a escrita da página
    Então o segredo é redigido no corpo, no título e nos valores de texto do frontmatter
    E nem o corpo nem o título são cortados
    E o segredo nunca é persistido em texto cru no arquivo nem no índice

  Cenário: Segredo numa observação é removido antes de limitar o tamanho
    Dado que o texto de uma observação contém um segredo no título e no corpo
    Quando a observação é sanitizada
    Então o segredo é removido antes de qualquer limite de tamanho
    E o título fica com até 80 caracteres e o corpo com até 16 KiB
    E o tipo sanitizado só é produzido pela função de sanitização
