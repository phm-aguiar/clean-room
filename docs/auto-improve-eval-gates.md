# Gates de avaliação da auto-melhoria

O gate de avaliação é uma checagem opcional, executada por um programa definido pelo operador, que valida propostas de auto-melhoria de alto impacto. Ele roda depois da validação da resposta do modelo e antes do staging e da auto-aprovação. Hooks nunca executam o comando. Para o contexto geral, veja o [loop de auto-melhoria](auto-improvement-loop.md).

## Configuração

A seção é `[auto_improve.eval]`. Todas as chaves são opcionais.

```toml
[auto_improve.eval]
enabled = false          # padrão
command = ""             # padrão: vazio
timeout_secs = 120       # padrão
targets = ["_rules", "procedures"]   # padrão
min_delta = 0.0          # padrão
```

| Chave | Padrão | Semântica |
|---|---|---|
| `enabled` | `false` | Desligado: nenhuma proposta passa pelo gate. |
| `command` | `""` | Programa seguido de argumentos, separados por espaços em branco. É executado diretamente, sem shell: não há aspas, pipes, expansão de `~` nem de variáveis. Para casos mais complexos, use um script wrapper. Valor só com espaços equivale a vazio. |
| `timeout_secs` | `120` | Limite por proposta avaliada. Cobre a escrita da entrada, a leitura da saída e a espera pelo processo. Ao estourar, o processo é encerrado e a proposta é rejeitada. |
| `targets` | `["_rules", "procedures"]` | Prefixos de caminho. Uma proposta é alvo se o caminho for igual ao alvo ou começar com o alvo seguido de `/`. Espaços em volta e barra final do alvo são removidos; alvo vazio é ignorado. A comparação é exata e diferencia maiúsculas de minúsculas. Lista vazia: nenhuma proposta passa pelo gate. |
| `min_delta` | `0.0` | Mínimo de `score_after - score_before`. Só é aplicado quando os dois scores são retornados. |

Com o gate habilitado e `command` vazio, cada proposta-alvo é rejeitada com `eval_gate_error`, e o relatório recebe o aviso `auto-improve eval gate enabled without a command; targeted proposals rejected`. Propostas fora dos alvos seguem normalmente.

### Exemplo de configuração

Exemplo apontando para o anexo em [assets/examples/auto-improve-eval/](assets/examples/auto-improve-eval/):

```toml
[auto_improve.eval]
enabled = true
command = "python3 /caminho/absoluto/assets/examples/auto-improve-eval/score_proposal.py"
timeout_secs = 30
targets = ["_rules", "procedures"]
min_delta = 0.0
```

`command` roda sem shell: não há expansão de `~` nem de variáveis, e os argumentos são separados por espaço. Use caminho absoluto, sem espaços.

## Execução

1. Para cada proposta-alvo, o gate lê o corpo atual da página (`before_body`; vazio se a página não existe). Essa leitura é feita antes de qualquer avaliação.
2. Inicia o processo, escreve a entrada JSON no stdin e fecha o stdin. O stderr é descartado. O stdout é lido até 64 KiB; se passar desse limite, é erro.
3. Escrita, leitura e espera pelo processo estão sob o mesmo `timeout_secs`.
4. Status de saída diferente de zero é erro. O stdout deve conter um único objeto JSON válido; campos desconhecidos são ignorados.
5. As propostas são avaliadas uma a uma, em sequência. A falha de uma rejeita só a proposta correspondente.

## Contrato de entrada (stdin)

```json
{
  "path": "procedures/release.md",
  "kind": "procedure",
  "operation": "create_or_update",
  "edit_mode": "patch",
  "title": "Release Procedure",
  "confidence": 0.91,
  "rationale": "Capture the repeated release checklist.",
  "before_body": "# Release Procedure\n\n## Steps\n- Run tests\n",
  "after_body": "# Release Procedure\n\n## Steps\n- Run tests\n- Run deploy smoke checks\n",
  "expected_base_body_sha256": "..."
}
```

- `operation` é sempre `create_or_update`, o valor canônico da proposta. Grafias alternativas que o modelo pode usar (`create`, `update`, `upsert`, `write`) são normalizadas antes do gate. Outras operações são rejeitadas pela validação, antes do gate.
- `after_body` é o corpo final já materializado, inclusive para patches.
- `expected_base_body_sha256` é definido pelo servidor: é `null` exceto em patches, onde traz o hash do corpo base usado pelo patch. Um valor enviado pelo modelo é descartado.

## Contrato de resposta (stdout)

```json
{ "score_before": 0.72, "score_after": 0.76, "passed": true, "reason": "optional" }
```

- `passed` (booleano) é obrigatório. Se faltar ou tiver outro tipo, a saída é inválida e vira erro.
- `score_before` e `score_after` são opcionais. Se os dois vierem, `delta = after - before` precisa ser maior ou igual a `min_delta`. Se só um vier, o delta não é calculado e a decisão depende apenas de `passed`.
- `reason` é opcional. Antes de ser registrado, passa por sanitização (segredos são substituídos por marcador de redação) e é truncado em 2048 caracteres, seguido de marcador com a quantidade omitida.

## Resultado

| Situação | Efeito | Status registrado |
|---|---|---|
| `passed=true` e delta ok (ou sem os dois scores) | Proposta aceita. Recebe uma entrada de evidência com página `auto_improve_eval` e citação `eval passed; score_before=0.7200, score_after=0.7600, delta=0.0400` (quatro casas decimais). Sem os dois scores, a citação é só `eval passed`. O `reason` de sucesso aparece apenas no registro da avaliação, não na citação. | `success` |
| `passed=false`, ou delta menor que `min_delta` | Rejeitada com motivo `eval_gate_failed`. A evidência é `passed=...; score_before=...; score_after=...; delta=...; min_delta=...; <reason>` quando os dois scores existem, e `passed=...; <reason>` caso contrário. Sem `reason`, usa `eval did not pass`. | `rejected` |
| Timeout | Rejeitada com `eval_gate_timeout`, motivo `eval command timed out after Ns`. | `timeout` |
| Falha ao iniciar, saída não-zero, JSON inválido, stdout acima de 64 KiB, comando vazio | Rejeitada com `eval_gate_error`. O motivo descreve a falha, por exemplo `eval command is empty`, `eval command exited with status ...` ou `invalid eval JSON: ...`. | `failure` |

A evidência de rejeição do gate é limitada a 1024 caracteres, com marcador de truncamento.

Toda rejeição carrega o caminho-alvo, o tipo, a operação e o modo de edição da proposta, e entra no histórico de rejeições da revisão. Esse histórico alimenta o contexto das revisões seguintes, o que reduz a chance de a mesma proposta voltar. Outras propostas da mesma revisão seguem normalmente. Se todas falharem, a revisão ainda é registrada, com zero propostas.

### Registro das avaliações

Cada proposta-alvo avaliada gera um registro, inclusive quando é rejeitada ou falha. Os registros ficam em `config_json.eval_results` da execução e contêm:

- id da avaliação;
- nome do executável (último componente do caminho do comando);
- digest SHA-256 do comando completo;
- hash SHA-256 da entrada enviada;
- hashes SHA-256 do corpo antes e depois da proposta;
- hash do corpo base, quando a proposta é um patch;
- status: `success`, `rejected`, `failure` ou `timeout`;
- `passed`, `score_before` e `score_after`, nulos quando não houve resposta válida;
- `reason`, sanitizado.

Quando a proposta é aceita e o corpo avaliado é o mesmo que será staged, o registro é anexado à entrada `auto_improve_eval` da evidência, no campo `eval_result`.

## Regras para o scorer

- Deve ser determinístico, rápido e sem efeitos colaterais. Lê apenas a entrada e arquivos locais seguros.
- Não chama LLM, rede nem deploy.
- Falha fechada: qualquer erro bloqueia apenas a proposta-alvo.
- Retorne razões curtas.

## Exemplos e smoke test

Os exemplos em [assets/examples/auto-improve-eval/](assets/examples/auto-improve-eval/) são scorers pequenos, determinísticos e sem dependências. Cada um lê um objeto JSON de proposta no stdin e imprime um objeto de resposta no stdout. Use como modelo, não como gate universal: projetos reais devem trocar as checagens por invariantes próprias, seguras de rodar antes do staging da proposta.

O primeiro exemplo chama `python3` e retorna `score_before` e `score_after`. O segundo usa `sh` e retorna só `score_after`; sem os dois scores, nenhum delta é calculado.

Smoke test (a partir de `docs/`):

```bash
python3 assets/examples/auto-improve-eval/score_proposal.py < \
  assets/examples/auto-improve-eval/sample-proposal.json
sh assets/examples/auto-improve-eval/score_proposal.sh < \
  assets/examples/auto-improve-eval/sample-proposal.json
```

Saída esperada, uma linha por comando, com a proposta de exemplo do anexo:

```json
{"passed":true,"score_before":0.7,"score_after":0.9}
{"passed":true,"score_after":1.0}
```

## Testes de aceitação

1. Gate desligado: nenhuma proposta é avaliada e nenhum comando é iniciado.
2. Scorer que devolve `{"passed":true}`: proposta aceita, com evidência começando por `eval passed`.
3. `passed=true` com `score_after - score_before` menor que `min_delta`: rejeitada com `eval_gate_failed`.
4. Scorer que dorme além do timeout: rejeitada com `eval_gate_timeout`, e o processo é encerrado.
5. Proposta em `notes/` com `targets` padrão: não é avaliada.
6. Scorer que imprime texto não JSON: rejeitada com `eval_gate_error`.
7. Gate habilitado com `command` vazio: propostas-alvo rejeitadas com `eval_gate_error` e o aviso de comando ausente.
