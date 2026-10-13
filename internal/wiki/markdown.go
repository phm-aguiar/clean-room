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
// não é YAML válido ou usa algo que o domínio não aceita (valores nulos ou mapeamentos, listas
// de algo que não é escalar, chave duplicada, raiz que não é mapeamento).
var ErrInvalidFrontmatter = errors.New("frontmatter inválido")

const (
	utf8BOM   = "\xEF\xBB\xBF"
	fenceLine = "---"
	tagStr    = "!!str"
)

// ReadMarkdown separa o frontmatter do corpo de um documento markdown (markdown-contract (a)).
//
// Descarta um BOM UTF-8 inicial. Reconhece o bloco só no início, com linha `---`, YAML e linha
// `---` de fechamento seguida de quebra de linha; aceita LF e CRLF. Quando não há bloco
// reconhecível (ausente, sem fechamento, sem quebra após o fechamento, vazio ou só com espaços),
// o documento inteiro, sem BOM, vira o corpo. O corpo é devolvido byte a byte, com os fins de
// linha originais.
//
// Com bloco presente, o YAML é lido com yaml.v3 sobre yaml.Node para manter a ordem das chaves.
// Valores aceitos: texto (string) e lista só de textos ([]string). Outros tipos YAML (inteiro,
// booleano, nulo, data, float, mapeamento, alias e lista com item que não é texto) geram erro
// com ErrInvalidFrontmatter. Não descartamos esses valores em silêncio, porque isso quebraria a
// ida e volta. Comentários do bloco não são preservados. Erro devolve frontmatter zero.
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
// corpo entra sem alteração. As chaves saem na ordem do domain.Frontmatter, que é a ordem de
// inserção. Chaves existentes mantêm a posição original, e chaves novas devem ser gravadas ao fim
// pelo chamador (ADR-17). Erro só vem de falha do encoder YAML.
func EmitMarkdown(fm domain.Frontmatter, body string) ([]byte, error) {
	keys := fm.Keys()
	if len(keys) == 0 {
		return []byte(body), nil
	}
	root := &yaml.Node{Kind: yaml.MappingNode}
	for _, k := range keys {
		v, _ := fm.Get(k)
		valNode, err := valueNode(k, v)
		if err != nil {
			return nil, err
		}
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: tagStr, Value: k}, valNode)
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
	var fm domain.Frontmatter
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return domain.Frontmatter{}, fmt.Errorf("%w: YAML: %w", ErrInvalidFrontmatter, err)
	}
	if doc.Kind == 0 || len(doc.Content) == 0 {
		return fm, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return domain.Frontmatter{}, fmt.Errorf("%w: a raiz do YAML deve ser um mapeamento", ErrInvalidFrontmatter)
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		kn, vn := root.Content[i], root.Content[i+1]
		if kn.Kind != yaml.ScalarNode || kn.ShortTag() != tagStr || kn.Value == "" {
			return domain.Frontmatter{}, fmt.Errorf("%w: chave deve ser texto não vazio (linha %d)", ErrInvalidFrontmatter, kn.Line)
		}
		if _, dup := fm.Get(kn.Value); dup {
			return domain.Frontmatter{}, fmt.Errorf("%w: chave duplicada %q", ErrInvalidFrontmatter, kn.Value)
		}
		val, err := frontmatterValue(kn.Value, vn)
		if err != nil {
			return domain.Frontmatter{}, err
		}
		if err := fm.Set(kn.Value, val); err != nil {
			return domain.Frontmatter{}, fmt.Errorf("%w: %w", ErrInvalidFrontmatter, err)
		}
	}
	return fm, nil
}

// frontmatterValue converte um nó YAML para string ou []string. Escalares de texto, booleano,
// inteiro, decimal e data/hora viram texto (ver scalarText). Nulo, mapeamento e alias são erro.
func frontmatterValue(key string, n *yaml.Node) (any, error) {
	switch n.Kind {
	case yaml.ScalarNode:
		if s, ok := scalarText(n); ok {
			return s, nil
		}
	case yaml.SequenceNode:
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
	return nil, fmt.Errorf("%w: chave %q, tipo YAML %s não suportado", ErrInvalidFrontmatter, key, n.ShortTag())
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

// valueNode converte string ou []string do domínio para nó YAML com tag de texto, para que
// valores como "true" ou "0.2" saiam entre aspas e não mudem de tipo na releitura.
func valueNode(key string, v any) (*yaml.Node, error) {
	switch val := v.(type) {
	case string:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: tagStr, Value: val}, nil
	case []string:
		seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, item := range val {
			seq.Content = append(seq.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: tagStr, Value: item})
		}
		return seq, nil
	}
	return nil, fmt.Errorf("%w: chave %q, tipo %T", ErrInvalidFrontmatter, key, v)
}
