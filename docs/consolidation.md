# Consolidação, manutenção e curadoria da memória

> **Escopo:** este documento contém partes que dependem de LLM ou de embeddings. Essas partes são opcionais e não fazem parte do escopo inicial, que funciona sem LLM. Podem ser removidas ou adiadas junto com as funções correspondentes.

> **Papel:** manutenção do wiki sem modelo de linguagem. Mantém a memória saudável
> com varredura de esquecimento, compactação extrativa, lint por regra e curadoria.
> Todas as passadas deste documento funcionam sem provedor.

**Dependências lógicas:** camada de armazenamento (leitura e escrita), camada de wiki
e normalização Unicode.

> O *quando* (agendadores e ticks) pertence à camada de servidor e de linha de
> comando. Este documento define o *o quê* de cada passada, sem estado global.

## Áreas funcionais

| Área | Função |
|---|---|
| Varredura de esquecimento | TTL, decaimento, compactação, remoção de marcadores e poda de observações. |
| Compactação e palavras-chave | Rebaixamento extrativo de tier (A2). |
| Lint por regra | Achados determinísticos, sem modelo. |
| Curador | Relatório conservador de sinais de manutenção. |

---

## 1. Varredura de esquecimento

Entradas:

- parâmetros de decaimento, incluindo o limiar de frio e `hard_delete_after_days`
  (padrão 180);
- `breadth_weight`, finito e maior ou igual a zero;
- retenção de observações: `days` maior ou igual a zero, e `batch`;
- flag de compactação de episódicas frias;
- `dry_run`;
- wiki opcional.

**Classificação:**

- **Expirada:** `expires_at` menor ou igual ao momento atual (TTL).
- **Fria:** somente episódica, não fixada, com score de retenção abaixo do limiar frio.
  O score combina tier, idade, número de acessos, tempo desde o último acesso, relevância
  e amplitude de acesso. Páginas semânticas, procedurais e de trabalho não decaem nesta
  passada.

**Ordem de execução.** Em `dry_run`, só o plano é produzido.

a. **TTL:** remove as páginas expiradas que ainda são a última versão. O TTL remove mesmo
   páginas fixadas. O lint avisa sobre essa combinação (`pinned_expiring`).
b. **Compactação A2** em lote, para as frias, quando a flag está ligada. Páginas já
   compactadas são terminais: não são recompactadas nem removidas.
c. **Remoção por decaimento** (quando a compactação está desligada): remove a página e
   deixa um marcador de remoção, se ainda for a última versão. Se ela mudou desde a
   seleção, pula.
d. **Remoção definitiva de marcadores** mais antigos que `hard_delete_after_days`, junto
   com a cadeia de supersessão deles. Uma página recriada no mesmo caminho é religada à
   cadeia e não é apagada.
e. **Poda de observações** (opt-in, com `days` maior que zero): remove, em lotes (padrão
   de 5 000 linhas por transação), as observações de sessões cujo resumo ainda é uma página
   viva. Roda por último de propósito. As remoções desta mesma execução já tiram
   sessões da lista. **Irreversível:** uma sessão podada nunca mais pode ser reprocessada a partir das observações.

Sem wiki, as mutações que dependem dela são recusadas, com aviso. A recusa é uma falha
segura, porque a reconciliação reindexaria a partir do armazenamento. A poda de
observações não depende de wiki, pois observações não têm arquivo.

Falhas por página viram aviso e são retentadas na próxima varredura. Nenhuma aborta a
execução.

Relatório: candidatas avaliadas, removidas por decaimento, compactadas, mescladas,
expiradas, removidas definitivamente, observações podáveis (contadas também em
`dry_run`), observações podadas, sessões tocadas e lotes de poda.

---


## 2. Palavras-chave e compactação

### 2.1 Palavras-chave (keep-tokens)

Extração sem LLM, por padrões, na ordem de maior sinal para menor:

1. URLs `http` e `https`;
2. caminhos com barras e extensão;
3. nomes de arquivo com extensão comum de código ou configuração;
4. códigos de erro (ex.: `E0433`) e códigos HTTP (ex.: `HTTP 500`);
5. trechos entre crases (conteúdo interno, até 120 caracteres);
6. constantes e variáveis de ambiente em `MAIÚSCULAS_COM_UNDERSCORE`;
7. identificadores em `snake_case` ou `camelCase`.

Regras: pontuação de borda é removida. A ordem é a da primeira ocorrência, sem repetição.
Máximo de 48 tokens. Tamanho entre 2 e 128 caracteres.

### 2.2 Compactação (A2, rebaixamento de tier)

Constrói a versão compactada a partir do frontmatter e do corpo:

- **Frontmatter:** preservado, com `compacted: true` acrescentado. O resumo de nível L0
  (`abstract`) é mantido.
- **L1:** o resumo explícito do frontmatter (`summary` ou `abstract`). Se não houver, o
  primeiro parágrafo não vazio que não seja título. É colapsado em uma linha e limitado a
  500 caracteres, com reticências ao ser cortado.
- **L2:** seção "Retained facts", com as palavras-chave em crases, uma por item. As
  palavras-chave vêm do corpo completo, inclusive de parágrafos descartados.
- **Rodapé fixo:** informa que o corpo completo continua no histórico git e na cadeia de
  supersessão, e que pode ser restaurado.
- Totalmente sem LLM e reversível pelo histórico.


## 3. Lint e curador

### 3.1 Lint

**Saída:** relatório em `_lint/report.md`, uma página por projeto, regravada a cada
execução. Sem achados, a página é removida. Relatórios datados antigos (`_lint/AAAA-MM-DD.md`)
são podados a cada execução que não seja simulação.

Cada achado tem tipo, severidade, mensagem e lista de páginas.

**Achados por regra (sem LLM):**

- `stale` (info): episódica sem acessos e com idade acima do limiar. O limiar é 0,6 dividido
  pelo λ de decaimento (em ln), se λ for válido e positivo. Caso contrário, 30 dias.
- `duplicate` (warning): mesmo título (sem diferença de caixa; títulos vazios não contam)
  em caminhos distintos.
- `rule_suggestion` (info): página de `kind: rule` ou em `_rules/`. Sugere migrá-la para o
  arquivo de instruções do projeto.
- `pinned_expiring` (warning): página fixada com TTL. O TTL prevalece na varredura.
- `broken_link` (warning): link para outro projeto que não existe, ou para página
  inexistente nesse projeto.
- `feedback_flagged` (warning): feedback aberto `stale` ou `wrong` na versão atual.
  Repete até a página ser reescrita.
- `contradiction` (warning): aresta declarada `contradicts`, resolvida ou não.

### 3.2 Curador

Parâmetros: máximo de 25 achados por tipo; 7 dias para o foco atual
(`_slots/current-focus.md`); 30 dias para os demais slots; parâmetros de decaimento.

Sinais (somente relatório):

- `cold_episodic` (info): episódica fria.
- `stale_slot` (warning): slot sem alteração além do limiar. Páginas de `_pending/` são
  ignoradas.
- `duplicate_title` (info): títulos normalizados iguais.
- `dangling_cross_project_link` (warning): link entre projetos sem destino.

Por padrão, só devolve o relatório, em modo simulação. Com o modo de staging, grava uma
página com o relatório.

---

---

## 4. Notas para reimplementação

1. **Ordem sugerida:** (a) varredura sem compactação; (b) lint por regra; (c) compactação extrativa; (d) curador.
2. Toda reescrita é supersessão. As remoções possíveis são: TTL explícito; remoção definitiva de marcadores após o prazo de `hard_delete_after_days`; e poda de observações (opt-in, irreversível). Documente isso para o operador.
3. Mantenha desligados por padrão os experimentos de compactação até medir ganho de recall.
