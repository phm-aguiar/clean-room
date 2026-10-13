# Nota: padrões de segredo e limites de sanitização

Task T003, feature 001-nucleo. Esta nota é somente leitura e não reproduz segredos reais. Os padrões RE2 da seção (c) são propostas nossas, escritas do zero a partir da descrição textual dos docs. Não foi visto nem copiado código original.

## 0. Fontes lidas e achado principal

- `docs/security.md` e `docs/security-boundaries.md` (§7, "Fronteira do sanitizador") dão a garantia estrutural (tipo-fronteira) e as classes de token do próprio servidor. O §7 remete a `docs/prompts-schemas-and-sanitizer.md`, que **não existe** no repo.
- A fonte detalhada dos padrões está em `docs/domain-vocabulary.md` §7 (pipeline, tabela de padrões, limites). Ela não estava na lista da tarefa, mas é a fonte principal.
- `docs/test-fixtures.md` não lista fixtures de segredo. Em `docs/assets/fixtures` não há fixture de sanitização. Os resultados de `grep secret` em `capture-policy.json` são só caminhos `secret/**` de política de captura.
- `docs/backup.md` L58-66, `docs/architecture.md` L220-223 e L546-548, `docs/admission-webhooks.md` §2.1, `docs/api-reference.md` L377 e `docs/design-backup-agent-assets.md` §5 complementam o quadro.
- Constituição INV-06 (`.github/memory/constitution.md` L34) e o spec (edge case "Segredo no texto de entrada") fixam a regra de ordem e os tetos.

## (a) Classes de segredo mencionadas nas fontes

Classes de `docs/domain-vocabulary.md` §7.2. O rótulo é o do doc e o marcador de saída é `[REDACTED:<rótulo>]`.

| Rótulo | Classe (descrição, sem valores) | Fonte |
|---|---|---|
| `bearer_token` | Cabeçalho `Bearer` + token com 16 ou mais caracteres | domain-vocabulary §7.2; backup.md L61-62 |
| `api_key` | Chave com prefixo `sk-`, 16 ou mais caracteres | domain-vocabulary §7.2 |
| `stripe_key` | Prefixos `sk_live_` e `rk_live_` | domain-vocabulary §7.2 |
| `github_token` | Prefixos `ghp_`, `gho_`, `ghu_`, `ghs_`, `ghr_`, `github_pat_` | domain-vocabulary §7.2 |
| `aws_key` | `AKIA` e `ASIA` com exatamente 20 caracteres e borda de palavra (evita falso positivo em palavras como `ASIAPACIFICREGION`) | domain-vocabulary §7.2 |
| `google_api_key` | Prefixo `AIza` (30 ou mais) e prefixo `AQ.Ab` (30 ou mais após o prefixo) | domain-vocabulary §7.2 |
| `google_oauth` | Refresh token começando com `1//`, 20 ou mais caracteres | domain-vocabulary §7.2 |
| `meta_token` | Token Graph com prefixo `EAA` | domain-vocabulary §7.2 |
| `telegram_token` | 6 a 10 dígitos, `:` e segredo (formas `AA…` e 34-35 caracteres) | domain-vocabulary §7.2 |
| `ghl_token` | Prefixo `pit-` + UUID | domain-vocabulary §7.2 |
| `slack_token` | Prefixos `xoxb-`, `xoxa-`, `xoxp-`, `xoxr-`, `xoxs-`, `xapp-` | domain-vocabulary §7.2 |
| `jwt` | Três segmentos base64url separados por ponto | domain-vocabulary §7.2 |
| `base64_json_token` | Blob base64 sem ponto, começando em `eyJ`, 40 ou mais caracteres | domain-vocabulary §7.2 |
| `private_key` | Bloco PEM de chave privada, multilinha | domain-vocabulary §7.2; backup.md L62-63 |
| `url_credentials` | `esquema://usuário:senha@host` | domain-vocabulary §7.2; test-fixtures §2.2 |
| `auth_header` | Cabeçalhos de autenticação com valor opaco (X-Api-Key, Authorization, Private-Token, Ocp-Apim-Subscription-Key, X-Amz-Security-Token). Sufixo `key`/`token` sozinho não basta: `Idempotency-Key` não é redigido | domain-vocabulary §7.2 |
| `env_secret` | (a) variáveis conhecidas; (b) `NOME_MAIUSCULO` terminado em `_KEY`, `_TOKEN`, `_SECRET`, `_PASSWORD`, `_PASSWD`, `_CREDENTIAL(S)`, `_PRIVATE_KEY`, com `=` ou `:`; (c) `_KEY_ID`, `_PASSPHRASE`, `_SIGNING_KEY`, `_PEPPER`, `_SALT`, só em maiúsculas | domain-vocabulary §7.2; backup.md L63-64; design-backup-agent-assets §5 |
| `credential_path` | Caminhos com `.ssh`, `.aws`, `.kube`, `.config/gcloud`, `.gnupg`, separador `/` ou `\`, com ou sem drive, sem distinção de caixa | domain-vocabulary §7.2 |
| `custom` | Padrões do operador (`[sanitize].extra_patterns`) | domain-vocabulary §7.3 |

Classes de credencial do próprio servidor, citadas em `docs/security.md`. São contexto de auth, não necessariamente padrões do sanitizador:
- Token de usuário do banco com prefixo `aim_` mais 43 caracteres base64 sem preenchimento (L101-105).
- Segredo de sessão com prefixo `ams_` (L153).
- Token raiz, que é hex de 64 caracteres (L71) e tem forma estruturalmente indistinguível de hex aleatório.
- Token do proxy, token de recuperação, senha inicial e pepper (L87, L131, L171-173, L263).

Não capturado de propósito: hex ou strings de alta entropia soltas (domain-vocabulary §7.2, parágrafo final). Falso positivo é aceitável; vazamento não (§7.1).

Pipeline (domain-vocabulary §7.1, ordem):
1. Remoção de controle. Isso inclui escapes de terminal, C0/C1 exceto `\t \n \r`, DEL e marcadores bidi (as faixas U+200B-200F, U+202A-202E, U+2060-2069 e U+FEFF aparecem em api-reference L372-376). Roda antes dos padrões, porque um escape dentro de um segredo o tiraria do alcance deles.
2. Padrões, dos mais específicos para os genéricos. Cada trecho casado vira `[REDACTED:<rótulo>]`, exceto se o trecho contém uma entrada da allowlist. A checagem é por trecho casado, não pelo texto inteiro.

## (b) Limites

- **Ordem (INV-06, spec edge case, domain-vocabulary §7.4):** remover segredo ANTES de limitar tamanho. Motivo: truncar primeiro pode cortar o segredo ao meio e deixar um fragmento curto demais para casar com o padrão.
- **Título:** sanitiza, depois aplica o teto de **80 caracteres exibidos**, com reticências ao final se cortado. As reticências ficam dentro do teto.
- **Corpo:** sanitiza, depois aplica o teto de **16 KiB (16384 bytes)**.
  - O truncamento é de cabeça e cauda. O orçamento útil é o teto menos 48 bytes de marcador, dividido metade para a cabeça e metade para a cauda.
  - O marcador no meio tem a forma `\n...[truncated N bytes]...\n`, com N igual aos bytes omitidos.
  - Se o teto for menor que 64 bytes, o corte é só de cabeça.
  - Nunca corta no meio de um caractere multibyte.
- **Tipo-fronteira:** o tipo sanitizado tem valor interno privado e só a função sanitizadora o constrói. A escrita aceita só esse tipo e as operações internas de escrita não são acessíveis fora de `storage` (security-boundaries §7). Cobertura STRONG (estrutural), mais um cenário ponta a ponta e testes unitários da remoção.
- **Reaplicação:** o servidor re-sanitiza o corpo, e o frontmatter de saída de hooks é re-sanitizado (admission-webhooks §2.1 passo 5). Em backup de assets MCP, arquivos não UTF-8 são copiados sem sanitizar (design-backup-agent-assets §5).
- **Outros limites vistos, fora do escopo de T003:** excerto do assistente 2 KB (architecture L222); mensagens entre agentes 8.000 caracteres ou bytes e assunto 200 (agent-messaging L122-123); identidade nativa de sessão rejeita a sequência `[REDACTED:` (api-reference L377).
- **Config:** `[sanitize].extra_patterns` e `allowlist`. Expressão inválida aborta a inicialização com erro claro. O sanitizador é construído uma vez e é compartilhável.

## (c) Padrões próprios propostos em RE2 (decisão nossa)

Todos os itens abaixo são **decisão nossa**: escritos de forma independente, a partir da descrição textual dos docs. Validar cada um com fixtures sintéticas próprias, nunca com segredos reais. Ordem sugerida: `private_key`, `url_credentials`, `auth_header`, `bearer_token` e prefixos específicos, `jwt`, `base64_json_token`, `env_secret`, `credential_path`. Sintaxe Go `regexp`, sem lookaround nem backreference.

| # | Rótulo | Proposta RE2 (decisão nossa) | Observação |
|---|---|---|---|
| 1 | `private_key` | `(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?(?:-----END [A-Z ]*PRIVATE KEY-----\|\z)` | O `\z` redige até o fim se o bloco vier truncado. Decisão nossa. |
| 2 | `url_credentials` | `[A-Za-z][A-Za-z0-9+.\-]*://[^\s/:@]+:[^\s/@]+@` | Redige só o trecho `esquema://u:s@`, preservando o host. |
| 3 | `auth_header` | `(?i)\b(?:authorization\|x-api-key\|private-token\|ocp-apim-subscription-key\|x-amz-security-token\|x-auth-token\|api-key)\s*:\s*[^\r\n]+` | Exige palavra de autenticação. `Idempotency-Key` não casa. |
| 4 | `bearer_token` | `(?i)\bbearer\s+[A-Za-z0-9._~+/=\-]{16,}` | Mínimo de 16 caracteres, como no doc. |
| 5 | `api_key` | `\bsk-[A-Za-z0-9_\-]{16,}` | |
| 6 | `stripe_key` | `\b[sr]k_live_[A-Za-z0-9]{8,}` | |
| 7 | `github_token` | `\b(?:gh[pousr]_[A-Za-z0-9]{20,}\|github_pat_[A-Za-z0-9_]{20,})` | |
| 8 | `aws_key` | `\b(?:AKIA\|ASIA)[A-Z0-9]{16}\b` | Âncora exata de 20 caracteres. |
| 9 | `google_api_key` | `\bAIza[A-Za-z0-9_\-]{26,}` e `\bAQ\.Ab[A-Za-z0-9_\-]{28,}` | Duas alternativas. |
| 10 | `google_oauth` | `\b1//[A-Za-z0-9_\-]{17,}` | |
| 11 | `meta_token` | `\bEAA[A-Za-z0-9]{20,}` | |
| 12 | `telegram_token` | `\b[0-9]{6,10}:[A-Za-z0-9_\-]{30,}` | O doc distingue duas formas (`AA…` e 34-35 caracteres). Aqui unifica, aceitando mais falsos positivos. |
| 13 | `ghl_token` | `\bpit-[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b` | |
| 14 | `slack_token` | `\bxox[baprs]-[A-Za-z0-9\-]{10,}\|\bxapp-[A-Za-z0-9\-]{10,}` | |
| 15 | `jwt` | `\b[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]{8,}\b` | Pode pegar identificadores com pontos, o que é aceitável. Alternativa mais estrita: exigir `eyJ` no primeiro segmento. |
| 16 | `base64_json_token` | `\beyJ[A-Za-z0-9_\-+/=]{37,}` | 40 ou mais no total. |
| 17 | `env_secret` | `\b[A-Z][A-Z0-9_]*(?:_KEY\|_TOKEN\|_SECRET\|_PASSWORD\|_PASSWD\|_CREDENTIALS?\|_PRIVATE_KEY\|_KEY_ID\|_PASSPHRASE\|_SIGNING_KEY\|_PEPPER\|_SALT)\s*[=:]\s*\S+` | Cobre as formas (b) e (c). Quanto à forma (a) (lista de variáveis conhecidas), a lista é LACUNA (ver seção d). |
| 18 | `credential_path` | `(?i)(?:[a-z]:)?[\\/][^\s"']*?(?:\.ssh\|\.aws\|\.kube\|\.config[\\/]gcloud\|\.gnupg)(?:[\\/][^\s"']*)?` | Também tratar `~/.ssh` (sem barra inicial). Ajustar nos testes. |

Decisões nossas adicionais:
- **D1.** Remoção de controle como etapa 0 no domínio, com faixas explícitas (C0/C1 exceto `\t \n \r`, DEL, bidi U+200B-200F, U+202A-202E, U+2060-2069, U+FEFF).
- **D2.** Marcador `[REDACTED:<rótulo>]` com rótulo em `[a-z0-9_]+`.
- **D3.** Allowlist por trecho casado, via substring ou igualdade, configurável em `[sanitize].allowlist`.
- **D4.** O corpo pode ser medido em bytes e cortado em fronteira UTF-8 (`utf8.RuneStart`). O título é contado em runes (hipótese para "caracteres exibidos"; ver LACUNA 4).
- **D5.** A etapa de sanitização deve ser idempotente: sanitizar duas vezes dá o mesmo resultado, e `[REDACTED:x]` não casa com nenhum padrão.
- **D6.** Testes: fixtures sintéticas próprias (valores obviamente falsos montados em tempo de teste a partir de prefixo + preenchimento, para não disparar scanners de segredo no repo). Cada classe precisa de caso positivo, caso negativo (`ASIAPACIFICREGION`, `Idempotency-Key`) e caso de segredo cortado pelo limite (prova da ordem sanitizar-antes-de-truncar). Fuzz de idempotência e de "nunca panica".
- **D7.** O critério do spec ("Nenhum segredo cru persistido nas fixtures de sanitização") exige criar essas fixtures próprias.

## (d) Lacunas

- **LACUNA 1:** `docs/prompts-schemas-and-sanitizer.md` (referenciado em security-boundaries L208) não existe. Os padrões exatos do original são desconhecidos. Só há descrições textuais em domain-vocabulary §7.2.
- **LACUNA 2:** não há fixtures de sanitização em `docs/assets/fixtures` nem em `docs/test-fixtures.md`. Os golden files de sanitização precisam ser criados por nós.
- **LACUNA 3:** a lista de "variáveis conhecidas" da forma (a) de `env_secret` é citada só por exemplos (`OPENAI_API_KEY`, `GITHUB_TOKEN`, `HF_TOKEN`, `AWS_SECRET_ACCESS_KEY` e "outras"). A lista completa é desconhecida. A forma (b), por sufixo, cobre a maioria.
- **LACUNA 4:** "80 caracteres exibidos" é ambíguo (runes ou grafemas). O doc não diz se as reticências ficam dentro dos 80. Há dois pontos abertos: o texto de reticências ("…" ou "...") e a regra de contagem. Precisa de ADR.
- **LACUNA 5:** não está especificado se o teto de 16 KiB conta o marcador `[REDACTED:…]` (conta, pois o corte vem depois da redação) nem o que acontece se o marcador for cortado na cabeça ou na cauda. Também não está dito se o teto de 16 KiB se aplica ao corpo de páginas wiki. O doc só fala de observações; o spec diz "texto de entrada" na escrita de página. Verificar com spec/plan.
- **LACUNA 6:** o tamanho exato do marcador de truncamento: o orçamento reserva 48 bytes, mas o marcador `\n...[truncated N bytes]...\n` depende dos dígitos de N. Definir como garantir que o total final fique ≤ 16384.
- **LACUNA 7:** a constituição marca INV-06 como "a confirmar" quanto à inexistência de outro caminho no original. Não verificável sem o código original, e não deve ser consultado em clean-room.
- **LACUNA 8:** o comportamento de `url_credentials` e `credential_path` em frontmatter (campos de chave/valor) e em nomes de arquivo não está especificado. admission-webhooks diz que todo valor string do frontmatter é re-sanitizado, mas não detalha o título derivado.
- **LACUNA 9:** nenhum doc especifica política para segredos em múltiplas linhas fora de PEM, nem para codificações ofuscadas (base64 de um segredo, URL-encoding). Pelo doc, ficam fora do alcance.
- **LACUNA 10:** a política de `[sanitize].allowlist` sobre trecho que contém e é maior que o casado ("checada por trecho casado") não tem exemplo. A semântica exata fica como decisão nossa D3.
