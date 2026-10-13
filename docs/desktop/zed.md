# Zed

Pesquisa sobre o editor Zed (app de terceiros). O foco é como o projeto se integra a ele. Cada afirmação sobre o produto foi conferida com o comportamento do projeto. Quando não há evidência, o texto diz "a confirmar".

## Resumo

- A integração com o Zed é **somente MCP**. O projeto grava a entrada de servidor MCP na configuração de usuário do editor.
- Não há hooks de ciclo de vida para o Zed. Por isso, captura automática, injeção automática de handoff e continuidade do `ai-memory run` não ficam ativas por meio do editor.
- O comando `install-mcp --client zed` imprime a configuração ou aplica a alteração com `--apply`.

## Superfície de integração

| Item | Comportamento no projeto |
|---|---|
| Cliente de instalação | `install-mcp --client zed` |
| Hooks de ciclo de vida | Nenhum. `install-hooks` não oferece Zed como agente. |
| Chave de configuração | `context_servers`, no nível raiz da configuração do Zed |
| Nome padrão da entrada | `ai-memory` (alterável com `--name`) |
| Transporte gravado | Remoto, por URL (HTTP) |
| Autenticação gravada | Cabeçalho `Authorization` com token bearer estático, quando há token |

## Arquivo de configuração

| Sistema | Caminho usado pelo projeto |
|---|---|
| macOS | `~/.config/zed/settings.json` |
| Linux | `$XDG_CONFIG_HOME/zed/settings.json`; sem a variável, `~/.config/zed/settings.json` |
| Windows | `%APPDATA%\Zed\settings.json` |

- `--config-file` substitui o caminho detectado.
- Se o editor lê esse arquivo exatamente nesses caminhos em cada sistema: a confirmar com a documentação do produto.

## Entrada gravada

O conteúdo gravado tem este formato:

```json
{
  "context_servers": {
    "ai-memory": {
      "url": "http://127.0.0.1:49374/mcp",
      "headers": {
        "Authorization": "Bearer <token>"
      }
    }
  }
}
```

- O campo `headers` só aparece quando há token.
- Para o Zed, o projeto não grava campos de execução local (comando, argumentos ou variáveis de ambiente).
- O projeto não grava campo de tipo de transporte. Como o editor interpreta a ausência desse campo: a confirmar.
- O projeto não gera configuração de OAuth. Se o editor inicia um fluxo OAuth quando não há cabeçalho `Authorization`: a confirmar.

## URL e token

- **URL padrão:** `http://127.0.0.1:49374/mcp`.
- **Precedência:** `--server-url` vence a URL configurada (configuração ou variável de ambiente do servidor), que vence o padrão.
- **Normalização:** espaços e barras finais são removidos. Se a URL não termina em `/mcp`, o sufixo é acrescentado. Uma URL que já termina em `/mcp` não muda.
- **Token:** `--auth-token`. Se ausente, usa o token de autenticação da configuração do projeto. Sem token, não há cabeçalho.
- **Marcador de dialeto de schema:** `--flavor` (`moonshot`, `bedrock` ou `gemini`) acrescenta `?flavor=<valor>` à URL. O Zed não tem marcador padrão.

## Comandos

- **Imprimir:** `ai-memory install-mcp --client zed`. A saída começa com linhas de comentário iniciadas por `#`, que não são JSON. Copie apenas o bloco JSON.
- **Aplicar:** `ai-memory install-mcp --client zed --apply`, com `--server-url` e `--auth-token` quando necessário.
- **Remover:** o comando de desinstalação remove a entrada (ver abaixo).

## Comportamento de `--apply`

1. Lê o arquivo. Se não existe, começa com conteúdo vazio. Se o caminho é um link simbólico, grava no destino do link.
2. Faz o parse como JSONC. Comentários, vírgulas finais e formatação não relacionados são preservados.
3. Cria o mapa `context_servers` se ele não existir. Se a raiz ou o mapa existirem com outro tipo, a operação falha sem gravar.
4. Se já existe uma entrada com o mesmo nome, substitui o valor dela. Se não existe, acrescenta a entrada. Outras entradas não mudam.
5. Se o conteúdo resultante é igual ao atual, não grava e informa que a configuração já está atualizada.
6. Se o arquivo já existia e mudou, grava antes uma cópia `<arquivo>.bak-<segundos Unix>` no mesmo diretório.
7. Grava o arquivo de forma atômica. Se não existia, cria o arquivo e os diretórios pais.
8. `--session-aware` é recusado para o Zed. Essa opção só existe para o agente Claude Code.

## Remoção

- O comando de desinstalação remove, do mapa `context_servers`, as entradas que combinam com o nome da instalação e com a URL do projeto. Também considera as variantes da URL com marcador de dialeto.
- Comentários e demais entradas são preservados.
- Se o mapa ficar vazio após a remoção, ele também é removido.

## Captura e sessões

- **Hooks:** o Zed não tem superfície de hooks compatível com o projeto. Por isso, a captura automática não é ativada por meio do editor.
- **Agente nativo do Zed:** não há integração do projeto com as sessões desse agente. Onde ficam os dados internos dele: a confirmar.
- **Agentes externos e terminal dentro do Zed (ACP ou CLI):** a captura depende de o próprio agente ter integração de hooks com o projeto e de essa integração ser carregada quando o agente roda dentro do Zed. Isso é a confirmar para o Zed. O mecanismo geral está em [Captura e deduplicação no desktop](capture-and-dedup.md).
- **Deduplicação:** não há tratamento de deduplicação específico para o Zed no projeto. A identidade da sessão é a que o agente informa. Mapeamento para o Zed: a confirmar.

## Recursos do produto (sem evidência no projeto)

Os itens abaixo vêm da documentação do produto e não foram verificados neste projeto:

- Tipos de agente do painel do Zed (agente nativo, agentes externos via ACP e threads de terminal): a confirmar.
- Suporte MCP do Zed a ferramentas e prompts, sem amostragem nem elicitação, e tratamento de notificação de mudança de lista de ferramentas: a confirmar.
- Repasse de servidores MCP configurados no Zed a agentes externos via ACP: a confirmar.
- Permissões de ferramenta (padrão e por ferramenta, com chave `mcp:<servidor>:<ferramenta>`): a confirmar. O projeto não grava essas permissões.
- Servidores MCP distribuídos como extensões do Zed: a confirmar. O projeto não gera extensão para o Zed.

## Limites

- Sem hooks: sem captura automática, sem handoff automático e sem continuidade do `ai-memory run` pelo editor.
- Somente remoto por URL: o projeto não grava servidor local por comando.
- A autenticação é um token bearer estático. O projeto não gera OAuth.
- `--apply` altera apenas a entrada com o mesmo nome. Outros servidores MCP do usuário permanecem.

## Leitura relacionada

- [Instalação de MCP](../mcp-install.md)
- [Panorama de apps desktop](desktop-landscape.md)
- [Captura e deduplicação no desktop](capture-and-dedup.md)
