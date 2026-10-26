# Embeddings locais

O modo local gera embeddings dentro do próprio processo do servidor. Não exige chave de API, servidor auxiliar nem GPU. O modelo é `all-MiniLM-L6-v2`, com vetores de 384 dimensões, calculados em CPU.

## Comportamento padrão

Sem `embedding_provider` configurado, o servidor usa o provedor `local` em modo best-effort:

- Na primeira inicialização, o modelo é baixado em segundo plano (cerca de 87 MB, uma única vez). Essa inicialização roda sem vetores: a busca usa texto, entidades e grafo. A busca híbrida passa a funcionar na inicialização seguinte.
- Se os arquivos do modelo já estiverem presentes e carregarem, a busca híbrida fica ativa na mesma inicialização.
- Se o host não conseguir baixar o modelo, ou se o carregamento falhar, o servidor segue sem vetores e registra um aviso. A inicialização não é bloqueada.
- Páginas que já existiam recebem vetores automaticamente em uma passagem de backfill no início do servidor.

Para desativar os vetores, use `embedding_provider = "none"` (também aceita `off` e `disabled`). Um provedor configurado explicitamente nunca é substituído.

```toml
# config.toml, opcional
embedding_provider = "local"   # explícito: falha se o modelo estiver indisponível
# embedding_provider = "none"  # sem vetores: somente texto, entidades e grafo
```

| Configuração | Comportamento |
|---|---|
| Não definida | Modo local em best-effort, como descrito acima. |
| `none`, `off` ou `disabled` | Sem vetores. A busca usa texto, entidades e grafo. |
| `local` | Modo local explícito. Se o modelo estiver ausente, o servidor o baixa antes de iniciar. Falha de download ou de carregamento interrompe a inicialização. |
| Outro provedor | Ver [Provedores remotos](#provedores-remotos). |

## Por que embeddings locais

A busca é híbrida: combina busca textual, entidades, grafo e, quando há um provedor de embeddings, vetores. O canal vetorial encontra paráfrases. Uma consulta como "como fazemos deploy" encontra a página "procedimento de release" mesmo sem palavras em comum.

Antes deste modo, ativar vetores exigia uma destas opções:

- uma chave de API de um provedor remoto. Há custo por chamada, e cada página e cada consulta de busca saem da máquina;
- um servidor auto-hospedado compatível com a API de embeddings da OpenAI. Não exige chave, mas é mais um serviço para executar e manter na mesma rede.

O modo local dispensa as duas opções:

- a memória não sai da máquina;
- o servidor continua sendo um único binário com um diretório de dados;
- o modelo é fixado por checksum, então os vetores são reproduzíveis e não mudam por decisão do provedor.

Um provedor remoto ou auto-hospedado faz mais sentido quando já existe um em uso, quando se quer um modelo de maior capacidade ou com outro idioma de treino, ou quando o cálculo de embeddings deve ficar fora do servidor de memória.

## Modelo e arquivos

- Modelo: `all-MiniLM-L6-v2`, 384 dimensões. Os vetores são gravados com o provedor `local` e o nome do modelo acima.
- Tamanho: cerca de 87 MB para o arquivo de pesos; o tokenizador e a configuração somam alguns MB (tamanho exato não é verificado pelo ai-memory, só o SHA-256).
- Licença do modelo: o ai-memory não a registra nem a verifica (o modelo público é distribuído pelo repositório de origem sob a licença dele); fora do escopo do comportamento.
- Diretório de cache: `<data_dir>/models/all-MiniLM-L6-v2/`.

Arquivos do modelo e checksums SHA-256 fixados em 2026-09-01:

| Arquivo | SHA-256 |
|---|---|
| `model.safetensors` | `53aa51172d142c89d9012cce15ae4d6cc0ca6895895114379cacb4fab128d9db` |
| `tokenizer.json` | `be50c3628f2bf5bb5e3a7f17b1f74611b2561a3a27eeab05e5aa30f411572037` |
| `config.json` | `953f9c0d463486b10a6871cc2fd59f223b2c70184f49815e7efbcab5d8908b41` |

### Download

- A origem é o repositório público `sentence-transformers/all-MiniLM-L6-v2` no hub Hugging Face: `GET https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/main/<arquivo>` (segue redirecionamentos para outro host).
- Os três arquivos são baixados em ordem. Um arquivo que já existe com o checksum correto é pulado. Um arquivo que existe com checksum errado é baixado de novo e substituído.
- Cada arquivo é verificado antes de ser gravado. A gravação passa por um arquivo temporário e termina com renomeação. Se o checksum não bater, o download é recusado e nada é instalado.
- Timeouts do download: 30 s para estabelecer a conexão e 600 s por arquivo. O corpo é lido inteiro em memória antes da verificação; o temporário tem o mesmo nome com a extensão trocada por `part`.
- Mensagens: `downloaded <arquivo> sha256 <hash> does not match pin <fixado>; refusing to install an unverified model`; no carregamento, `local embedding model file missing: <caminho>. Start the server once with network access to fetch it, or download <URL> manually into <diretório>` e `local embedding model file <caminho> sha256 <hash> does not match the pinned <fixado>; delete it and re-fetch`.
- A checagem de presença, usada no início do servidor, só confirma que os três arquivos existem. O checksum é verificado ao carregar o modelo.
- Consequência: um arquivo presente, mas corrompido, não é baixado de novo. No modo padrão, o servidor registra um aviso e segue sem vetores. No modo explícito, a inicialização falha. Nos dois casos, apague o arquivo para forçar um novo download.
- Enquanto o modelo estiver ausente, cada inicialização tenta baixá-lo de novo.

### Instalação offline

Baixe os três arquivos em qualquer máquina com acesso à origem e copie-os para o diretório de cache. O carregamento verifica os mesmos checksums e não acessa a rede. Ver também [Instalação em ambiente isolado](airgapped-install.md).

## Pipeline de vetorização

Para cada texto:

1. Se o texto passar de cerca de 6 KB (6144 bytes), ele é cortado antes da tokenização. O corte respeita limites de caractere e termina com reticências.
2. O texto é tokenizado com os tokens especiais de início e fim. Não há preenchimento até um comprimento fixo. O limite é de 512 tokens. O excedente é descartado, então só a cabeça do texto é vetorizada.
3. A inferência roda em CPU com a máscara de atenção real. Preenchimento fixo fica desligado: com preenchimento e máscara incorreta, as similaridades ficam infladas, perto de 0,85.
4. Os tokens são agregados por média ponderada pela máscara. Tokens de preenchimento não entram na média.
5. O vetor é normalizado pela norma L2, resultando em um vetor unitário de 384 valores. Se a norma for zero ou não finita, a operação retorna erro.
6. A inferência roda no pool de threads de bloqueio. O runtime assíncrono do servidor não é bloqueado.

Referência de calibração: para as frases "The cat sits outside" e "The dog plays in the garden", o produto escalar entre os vetores deve ficar entre 0,15 e 0,45. Esse teste exige o modelo baixado e não roda por padrão.

Ao iniciar, o modelo é lido inteiro para a memória, uma vez por inicialização do servidor. Textos são vetorizados um a um, sem lotes.

## Convivência e migração

- Cada vetor guarda a tripla (provedor, modelo, dimensão). A busca usa apenas vetores cuja tripla bate com a configuração atual. Vetores de outras triplas permanecem guardados e são ignorados.
- Trocar de provedor é uma mudança de configuração. Voltar ao provedor anterior reativa os vetores antigos.
- Backfill de inicialização: quando há vetorização ativa (qualquer provedor), o servidor calcula, em segundo plano, os vetores que faltam para a tripla atual. Páginas que possuem resumo (campo `abstract`) recebem um vetor próprio para ele. Páginas já vetorizadas são puladas. Toda escrita de página com embedder ativo também embute o corpo e o resumo na hora (ver [wiki-files-layer](wiki-files-layer.md)).
- Backfill periódico: opcional, via configuração de manutenção (`maintenance.embedding_backfill_interval_secs`). O padrão é 0, desligado.
- Para recalcular tudo com o provedor atual, use `ai-memory embed --force`. Sem `--project`, o comando vale para todos os projetos. A CLI é cliente HTTP, então o servidor precisa estar em execução. `--dry-run` apenas conta o que seria feito.
- Se a inicialização encontrar triplas diferentes da configuração atual, o servidor registra um aviso. Isso não bloqueia a inicialização.

## Provedores remotos

Para comparação com o modo local. Cada provedor usa a própria chave ou autenticação.

| Provedor (`embedding_provider`) | Modelo padrão | Dimensão padrão | Requisitos |
|---|---|---|---|
| `openai` | `text-embedding-3-small` | 1536 | Chave da OpenAI |
| `voyage` | `voyage-3` | 1024 | Chave da Voyage |
| `google` ou `gemini` | `gemini-embedding-001` | 768 | Chave do Gemini ou do Google |
| `openai-compat` | Obrigatório em `embedding_model` | Obrigatória em `embedding_dim` | URL base obrigatória; chave opcional |
| `copilot` | `text-embedding-3-small` | 1536 | Autenticação OAuth do Copilot |

Clientes de OpenAI, Voyage, Google e openai-compat usam timeout de 120 s por requisição; o do Copilot é de 300 s. Retentativa: ver [cliente de LLM](llm-client.md) §8.3 (OpenAI e compatível repetem 429 e 503 com `Retry-After`; Google e Copilot, só 429; Voyage, nenhuma).

Os provedores remotos enviam o texto das páginas e das consultas ao serviço externo. Ver [Provedores de LLM](llm-providers.md) para a configuração completa de provedores.

## Limites

- Só a cabeça de cada texto é vetorizada: até cerca de 6 KB e 512 tokens.
- O nome do modelo e a dimensão do modo local são fixos (`all-MiniLM-L6-v2`, 384). Os valores de `embedding_model` e `embedding_dim` não alteram o modelo local.
- A inferência roda só em CPU, sem lotes. Para volumes grandes, o custo de CPU pode ser relevante.
- Sem o modelo presente, a busca usa apenas texto, entidades e grafo, e o aviso fica registrado no log.
