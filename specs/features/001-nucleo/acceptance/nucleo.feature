# language: pt
Funcionalidade: Núcleo do ai-memory-go (domínio, armazenamento e wiki)
  Como agente de código
  Quero escrever, buscar e ler páginas de memória de longo prazo
  Para que o conhecimento do projeto persista entre sessões, com arquivo, índice e git consistentes

  Cenário: Escrever uma página atualiza arquivo, índice e git juntos
    Dado que o projeto "demo" existe no workspace resolvido pelo resolvedor central de escopo
    Quando o chamador pede a escrita da página "decisions/db.md" no projeto "demo"
    Então o corpo e o frontmatter passam pela validação do caminho e pela sanitização
    E o arquivo "decisions/db.md" é instalado de forma atômica
    E a página, os índices FTS, de entidades e de links são commitados em uma única transação do escritor único
    E um commit git é criado para o evento
    E uma linha é anexada ao "log.md" do projeto "demo"
    E a resposta traz o identificador, o caminho "decisions/db.md" e o checkpoint

  Cenário: Busca híbrida retorna páginas ranqueadas por RRF com autoridade aplicada
    Dado que o escopo "ai-memory" contém as páginas das fixtures de "docs/assets/fixtures"
    E uma página casa tanto por FTS quanto por entidade
    E outra página casa apenas por um dos fluxos
    Quando o chamador faz uma consulta que casa as duas páginas
    Então a página que casa FTS e entidade aparece acima da que casa só um fluxo
    E o ranqueamento usa RRF com k igual a 60
    E o multiplicador de autoridade ajusta a ordem final dentro do intervalo [0,55; 1,50]

  Cenário: Leitura devolve a versão corrente após supersessão
    Dado que a página "decisions/db.md" do projeto "demo" foi escrita em uma versão anterior
    E que uma nova versão da mesma página supersede a anterior
    Quando o chamador lê a página "decisions/db.md" do projeto "demo"
    Então a resposta devolve a versão corrente da página
    E a versão anterior não é retornada como corrente

  Cenário: Falha no banco após instalar o arquivo reverte a página
    Dado que o arquivo "decisions/db.md" foi instalado com sucesso no disco
    E que a transação do banco falha em seguida
    Quando o chamador pede a escrita da página "decisions/db.md" no projeto "demo"
    Então o arquivo "decisions/db.md" é revertido ao estado anterior
    E o erro é propagado de forma não ambígua ao chamador
    E nenhum commit git é criado
    E o índice permanece inalterado

  Cenário: Leitura sem escopo resolvível falha fechada
    Dado que nenhum workspace ou projeto pode ser resolvido para a requisição
    Quando o chamador pede a leitura de uma página
    Então a operação falha com erro de escopo
    E nenhum workspace é criado
    E nenhum projeto é criado

  Cenário: Duas sessões escrevem o mesmo caminho ao mesmo tempo
    Dado que duas sessões distintas do projeto "demo" existem
    E que as duas sessões escrevem a página "decisions/db.md" simultaneamente
    Quando as duas escritas são processadas pelo escritor único
    Então uma versão da página é escrita
    E a outra versão supersede a anterior numa cadeia de supersessão
    E a versão perdedora continua alcançável pela cadeia
    E a leitura da página não é filtrada por autoria

  Cenário: Edição manual da wiki no disco é reindexada
    Dado que a página "decisions/db.md" do projeto "demo" está indexada
    Quando o usuário edita manualmente o arquivo "decisions/db.md" no disco
    Então o watcher reindexa a página após o debounce de 300 ms
    Ou a reconciliação reindexa a página no ciclo de 30 s caso o watcher não tenha capturado a alteração
    E as escritas feitas pelo próprio servidor são ignoradas pelo prefixo do arquivo temporário

  Cenário: Segredo no texto de entrada é removido antes de limitar o tamanho
    Dado que o texto de entrada contém um segredo no título e no corpo
    E que o título deve ter até 80 caracteres e o corpo até 16 KiB
    Quando o chamador pede a escrita da página com esse texto
    Então o segredo é removido antes de qualquer limite de tamanho ser aplicado
    E o título e o corpo são limitados aos tamanhos máximos após a remoção
    E o segredo nunca é persistido em texto cru no armazenamento
    E o tipo sanitizado só é produzido pela função de sanitização
