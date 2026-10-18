package wiki

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
	"gopkg.in/yaml.v3"
)

// ErrInvalidFrontmatter é a causa de ReadMarkdown quando o bloco de frontmatter existe, mas
// não é YAML válido ou usa algo que o domínio não aceita (nulo fora de `entities`, alias, chave
// duplicada, chave que não é texto, lista mista de texto e mapeamento, raiz que não é mapeamento).
var ErrInvalidFrontmatter = errors.New("frontmatter inválido")

const (
	utf8BOM     = "\xEF\xBB\xBF"
	fenceLine   = "---"
	tagStr      = "!!str"
	tagMap      = "!!map"
	tagSeq      = "!!seq"
	tagNull     = "!!null"
	entitiesKey = "entities"
)

// ReadMarkdown separa o frontmatter do corpo de um documento markdown (markdown-contract (a)).
//
// Descarta um BOM UTF-8 inicial. Reconhece o bloco só no início, com linha `---`, YAML e linha
// `---` de fechamento seguida de quebra de linha; aceita LF e CRLF. Quando não há bloco
// reconhecível (ausente, sem fechamento, sem quebra após o fechamento, vazio ou só com espaços),
// o documento inteiro, sem BOM, vira o corpo. O corpo é devolvido byte a byte, com os fins de
// linha originais.
//
// Com bloco presente, o YAML é lido com yaml.v3 sobre yaml.Node para manter a ordem das chaves,
// em todos os níveis. Valores aceitos: texto (string), lista só de textos ([]string), mapeamento
// aninhado (domain.Frontmatter, como `generated: {by, at}`) e lista só de mapeamentos
// ([]domain.Frontmatter, como `sources`). Nulo é recusado, exceto em `entities` no topo do
// frontmatter, onde a especificação aceita nulo: ali vale como lista vazia. Alias, chave que não é
// texto não vazio, chave duplicada em qualquer nível, lista mista e item nulo geram erro com
// ErrInvalidFrontmatter. Não descartamos esses valores em silêncio, porque isso quebraria a ida e
// volta. Comentários do bloco não são preservados. Erro devolve frontmatter zero.
func ReadMarkdown(data []byte) (domain.Frontmatter, string, error) {
	data = bytes.TrimPrefix(data, []byte(utf8BOM))
	yamlPart, body, ok := splitBlock(data)
	if !ok {
		return domain.Frontmatter{}, string(data), nil
	}
	fm, err := parseFrontmatter(yamlPart)
	if err != nil {
		return domain.Frontmatter{}, "", err
	}
	return fm, string(body), nil
}

// EmitMarkdown monta o documento: bloco `---` com o YAML do frontmatter e depois o corpo.
//
// Se o frontmatter está vazio, o documento sai sem bloco e o corpo fica como está. Caso contrário
// a saída é `---\n` + YAML com indentação 2 em LF + `---\n` + corpo. O bloco nunca tem BOM, e o
// corpo entra sem alteração. As chaves saem na ordem do domain.Frontmatter em todos os níveis, que
// é a ordem de inserção. Chaves existentes mantêm a posição original, e chaves novas devem ser
// gravadas ao fim pelo chamador (ADR-17). Erro só vem de falha do encoder YAML.
func EmitMarkdown(fm domain.Frontmatter, body string) ([]byte, error) {
	if len(fm.Keys()) == 0 {
		return []byte(body), nil
	}
	root, err := mappingNode(fm)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	buf.WriteString(fenceLine + "\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return nil, fmt.Errorf("emitir frontmatter: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("emitir frontmatter: %w", err)
	}
	buf.WriteString(fenceLine + "\n")
	buf.WriteString(body)
	return buf.Bytes(), nil
}

// splitBlock acha o bloco de frontmatter no início de data. yamlPart é o YAML entre as linhas
// de fechamento e abertura, e body é o que vem depois da linha de fechamento. ok false significa
// que o documento não tem frontmatter reconhecível.
func splitBlock(data []byte) (yamlPart, body []byte, ok bool) {
	var start int
	switch {
	case bytes.HasPrefix(data, []byte(fenceLine+"\n")):
		start = len(fenceLine) + 1
	case bytes.HasPrefix(data, []byte(fenceLine+"\r\n")):
		start = len(fenceLine) + 2
	default:
		return nil, nil, false
	}
	pos := start
	for pos < len(data) {
		nl := bytes.IndexByte(data[pos:], '\n')
		if nl < 0 {
			// Sem fechamento, ou fechamento sem quebra de linha depois: não é frontmatter.
			return nil, nil, false
		}
		line := bytes.TrimSuffix(data[pos:pos+nl], []byte("\r"))
		if string(line) == fenceLine {
			yamlPart = data[start:pos]
			if len(bytes.TrimSpace(yamlPart)) == 0 {
				// Bloco vazio (duas linhas --- seguidas, ou só espaços): vira corpo.
				return nil, nil, false
			}
			return yamlPart, data[pos+nl+1:], true
		}
		pos += nl + 1
	}
	return nil, nil, false
}

// parseFrontmatter lê o YAML do bloco para domain.Frontmatter, na ordem do documento.
// Um bloco só de comentários dá frontmatter vazio.
func parseFrontmatter(src []byte) (domain.Frontmatter, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return domain.Frontmatter{}, fmt.Errorf("%w: YAML: %w", ErrInvalidFrontmatter, err)
	}
	if doc.Kind == 0 || len(doc.Content) == 0 {
		return domain.Frontmatter{}, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return domain.Frontmatter{}, fmt.Errorf("%w: a raiz do YAML deve ser um mapeamento", ErrInvalidFrontmatter)
	}
	return mappingValue(root, true)
}

// mappingValue lê um mapeamento YAML para domain.Frontmatter, na ordem do documento, em qualquer
// nível. top diz se é a raiz do frontmatter: só ali `entities` aceita nulo.
func mappingValue(n *yaml.Node, top bool) (domain.Frontmatter, error) {
	var fm domain.Frontmatter
	for i := 0; i+1 < len(n.Content); i += 2 {
		kn, vn := n.Content[i], n.Content[i+1]
		if kn.Kind != yaml.ScalarNode || kn.ShortTag() != tagStr || kn.Value == "" {
			return domain.Frontmatter{}, fmt.Errorf("%w: chave deve ser texto não vazio (linha %d)", ErrInvalidFrontmatter, kn.Line)
		}
		if _, dup := fm.Get(kn.Value); dup {
			return domain.Frontmatter{}, fmt.Errorf("%w: chave duplicada %q", ErrInvalidFrontmatter, kn.Value)
		}
		var val any
		var err error
		if top && kn.Value == entitiesKey && isNull(vn) {
			// A especificação aceita entities nulo, com o mesmo sentido de ausente: lista vazia.
			val = []string{}
		} else {
			val, err = frontmatterValue(kn.Value, vn)
			if err != nil {
				return domain.Frontmatter{}, err
			}
		}
		if err := fm.Set(kn.Value, val); err != nil {
			return domain.Frontmatter{}, fmt.Errorf("%w: %w", ErrInvalidFrontmatter, err)
		}
	}
	return fm, nil
}

// frontmatterValue converte um nó YAML para string, []string, domain.Frontmatter ou
// []domain.Frontmatter. Escalares de texto, booleano, inteiro, decimal e data/hora viram texto
// (ver scalarText). Nulo, alias e lista mista são erro.
func frontmatterValue(key string, n *yaml.Node) (any, error) {
	switch n.Kind {
	case yaml.ScalarNode:
		if s, ok := scalarText(n); ok {
			return s, nil
		}
	case yaml.MappingNode:
		fm, err := mappingValue(n, false)
		if err != nil {
			return nil, err
		}
		return fm, nil
	case yaml.SequenceNode:
		return sequenceValue(key, n)
	}
	return nil, fmt.Errorf("%w: chave %q, tipo YAML %s não suportado", ErrInvalidFrontmatter, key, n.ShortTag())
}

// sequenceValue lê uma lista YAML. Lista vazia é []string. Lista só de escalares de texto é
// []string. Lista só de mapeamentos é []domain.Frontmatter. Qualquer outra mistura é erro.
func sequenceValue(key string, n *yaml.Node) (any, error) {
	if len(n.Content) > 0 && n.Content[0].Kind == yaml.MappingNode {
		maps := make([]domain.Frontmatter, 0, len(n.Content))
		for _, it := range n.Content {
			if it.Kind != yaml.MappingNode {
				return nil, fmt.Errorf("%w: chave %q, item de lista não é mapeamento (tipo %s)", ErrInvalidFrontmatter, key, it.ShortTag())
			}
			fm, err := mappingValue(it, false)
			if err != nil {
				return nil, err
			}
			maps = append(maps, fm)
		}
		return maps, nil
	}
	items := make([]string, 0, len(n.Content))
	for _, it := range n.Content {
		s, ok := scalarText(it)
		if !ok {
			return nil, fmt.Errorf("%w: chave %q, item de lista não é escalar (tipo %s)", ErrInvalidFrontmatter, key, it.ShortTag())
		}
		items = append(items, s)
	}
	return items, nil
}

// isNull diz se o nó é um escalar nulo do YAML (`x:`, `x: null`, `x: ~`).
func isNull(n *yaml.Node) bool {
	return n.Kind == yaml.ScalarNode && n.ShortTag() == tagNull
}

// scalarText devolve o texto de um escalar YAML. Texto vale como está. Booleano, inteiro, decimal
// e data/hora sem aspas (`pinned: true`, `okf_version: 0.2`, `data: 2026-03-01`) valem pelo texto
// literal, para que um arquivo editado à mão não seja recusado por um detalhe de sintaxe; o
// booleano é normalizado para minúsculas. Na reescrita o sistema emite com aspas (valueNode), o
// que preserva o significado, porque o resto do código compara como texto (T084).
func scalarText(n *yaml.Node) (string, bool) {
	if n.Kind != yaml.ScalarNode {
		return "", false
	}
	switch n.ShortTag() {
	case tagStr, "!!int", "!!float", "!!timestamp":
		return n.Value, true
	case "!!bool":
		return strings.ToLower(n.Value), true
	}
	return "", false
}

// mappingNode monta o nó YAML de um mapeamento do domínio, com as chaves na ordem do domínio e
// os valores convertidos recursivamente por valueNode.
func mappingNode(fm domain.Frontmatter) (*yaml.Node, error) {
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: tagMap}
	for _, k := range fm.Keys() {
		v, _ := fm.Get(k)
		valNode, err := valueNode(k, v)
		if err != nil {
			return nil, err
		}
		node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: tagStr, Value: k}, valNode)
	}
	return node, nil
}

// valueNode converte um valor do domínio (string, []string, Frontmatter ou []Frontmatter) para nó
// YAML com tag de texto, para que valores como "true" ou "0.2" saiam entre aspas e não mudem de
// tipo na releitura.
func valueNode(key string, v any) (*yaml.Node, error) {
	switch val := v.(type) {
	case string:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: tagStr, Value: val}, nil
	case []string:
		seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: tagSeq}
		for _, item := range val {
			seq.Content = append(seq.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: tagStr, Value: item})
		}
		return seq, nil
	case domain.Frontmatter:
		return mappingNode(val)
	case []domain.Frontmatter:
		seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: tagSeq}
		for _, item := range val {
			m, err := mappingNode(item)
			if err != nil {
				return nil, err
			}
			seq.Content = append(seq.Content, m)
		}
		return seq, nil
	}
	return nil, fmt.Errorf("%w: chave %q, tipo %T", ErrInvalidFrontmatter, key, v)
}
