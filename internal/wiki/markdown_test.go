package wiki

import (
	"bytes"
	"errors"
	"reflect"
	"testing"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// mustRead lê o documento e falha o teste em erro.
func mustRead(t *testing.T, data []byte) (domain.Frontmatter, string) {
	t.Helper()
	fm, body, err := ReadMarkdown(data)
	if err != nil {
		t.Fatalf("ReadMarkdown(%q): %v", data, err)
	}
	return fm, body
}

// mustEmit emite o documento e falha o teste em erro.
func mustEmit(t *testing.T, fm domain.Frontmatter, body string) []byte {
	t.Helper()
	out, err := EmitMarkdown(fm, body)
	if err != nil {
		t.Fatalf("EmitMarkdown: %v", err)
	}
	return out
}

// mustSet grava chaves no frontmatter, na ordem dada, e falha o teste em erro.
func mustSet(t *testing.T, fm *domain.Frontmatter, pairs ...any) {
	t.Helper()
	for i := 0; i < len(pairs); i += 2 {
		if err := fm.Set(pairs[i].(string), pairs[i+1]); err != nil {
			t.Fatalf("Set(%v): %v", pairs[i], err)
		}
	}
}

func TestMarkdownReadLFLeBlocoEBody(t *testing.T) {
	fm, body := mustRead(t, []byte("---\ntype: Note\ntitle: Oi\n---\n# Corpo\n"))
	if got, want := fm.Keys(), []string{"type", "title"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Keys = %v, quer %v", got, want)
	}
	if v, _ := fm.Get("type"); v != "Note" {
		t.Errorf("type = %v, quer Note", v)
	}
	if v, _ := fm.Get("title"); v != "Oi" {
		t.Errorf("title = %v, quer Oi", v)
	}
	if body != "# Corpo\n" {
		t.Errorf("body = %q, quer %q", body, "# Corpo\n")
	}
}

func TestMarkdownReadCRLFLeBlocoEBody(t *testing.T) {
	fm, body := mustRead(t, []byte("---\r\ntype: Note\r\ntags:\r\n  - a\r\n  - b\r\n---\r\n# C\r\nlinha\r\n"))
	if v, _ := fm.Get("type"); v != "Note" {
		t.Errorf("type = %v, quer Note", v)
	}
	if v, _ := fm.Get("tags"); !reflect.DeepEqual(v, []string{"a", "b"}) {
		t.Errorf("tags = %v, quer [a b]", v)
	}
	if body != "# C\r\nlinha\r\n" {
		t.Errorf("body = %q, quer %q", body, "# C\r\nlinha\r\n")
	}
}

func TestMarkdownReadDescartaBOM(t *testing.T) {
	fm, body := mustRead(t, []byte("\xEF\xBB\xBF---\ntype: Note\n---\n# T\n"))
	if v, _ := fm.Get("type"); v != "Note" {
		t.Errorf("type = %v, quer Note", v)
	}
	if body != "# T\n" {
		t.Errorf("body = %q, quer %q (BOM não pode ficar no corpo)", body, "# T\n")
	}
}

func TestMarkdownReadBOMSemFrontmatterVaiParaCorpo(t *testing.T) {
	fm, body := mustRead(t, []byte("\xEF\xBB\xBF# Titulo\n"))
	if len(fm.Keys()) != 0 {
		t.Errorf("Keys = %v, quer vazio", fm.Keys())
	}
	if body != "# Titulo\n" {
		t.Errorf("body = %q, quer %q", body, "# Titulo\n")
	}
}

func TestMarkdownReadBlocoSemFechamentoViraCorpo(t *testing.T) {
	doc := "---\ntype: Note\n# corpo sem fechamento\n"
	fm, body := mustRead(t, []byte(doc))
	if len(fm.Keys()) != 0 {
		t.Errorf("Keys = %v, quer vazio", fm.Keys())
	}
	if body != doc {
		t.Errorf("body = %q, quer o documento inteiro %q", body, doc)
	}
}

func TestMarkdownReadFechamentoSemQuebraViraCorpo(t *testing.T) {
	doc := "---\ntype: Note\n---"
	fm, body := mustRead(t, []byte(doc))
	if len(fm.Keys()) != 0 {
		t.Errorf("Keys = %v, quer vazio", fm.Keys())
	}
	if body != doc {
		t.Errorf("body = %q, quer o documento inteiro %q", body, doc)
	}
}

func TestMarkdownReadBlocoVazioViraCorpo(t *testing.T) {
	doc := "---\n---\ncorpo\n"
	fm, body := mustRead(t, []byte(doc))
	if len(fm.Keys()) != 0 {
		t.Errorf("Keys = %v, quer vazio", fm.Keys())
	}
	if body != doc {
		t.Errorf("body = %q, quer o documento inteiro %q", body, doc)
	}
}

func TestMarkdownReadFrontmatterSoNoInicio(t *testing.T) {
	doc := "# Titulo\n\n---\ntype: Note\n---\n"
	fm, body := mustRead(t, []byte(doc))
	if len(fm.Keys()) != 0 {
		t.Errorf("Keys = %v, quer vazio (bloco fora do início não conta)", fm.Keys())
	}
	if body != doc {
		t.Errorf("body = %q, quer %q", body, doc)
	}
}

func TestMarkdownReadYAMLInvalidoDaErroTipado(t *testing.T) {
	casos := map[string]string{
		"sintaxe":         "---\ntype: [Note\n---\ncorpo\n",
		"nulo":            "---\ntitle:\n---\n",
		"mapeamento":      "---\ngenerated:\n  by: x\n---\n",
		"lista de mapas":  "---\ntags:\n  - a: 1\n---\n",
		"chave duplicada": "---\na: x\na: y\n---\n",
		"raiz sequencia":  "---\n- a\n- b\n---\n",
		"alias":           "---\na: &x v\nb: *x\n---\n",
	}
	for nome, doc := range casos {
		t.Run(nome, func(t *testing.T) {
			_, _, err := ReadMarkdown([]byte(doc))
			if !errors.Is(err, ErrInvalidFrontmatter) {
				t.Fatalf("err = %v, quer ErrInvalidFrontmatter", err)
			}
		})
	}
}

// TestMarkdownEscalaresNaoTextoViramTexto: `pinned: true`, `okf_version: 0.2`, número e data sem
// aspas valem pelo texto literal, e a reescrita emite com aspas e relê igual (T084).
func TestMarkdownEscalaresNaoTextoViramTexto(t *testing.T) {
	doc := "---\npinned: True\nokf_version: 0.2\nn: 3\ndata: 2026-03-01\ntags:\n  - 1\n  - go\n---\ncorpo\n"
	fm, body := mustRead(t, []byte(doc))
	for chave, want := range map[string]string{
		"pinned":      "true",
		"okf_version": "0.2",
		"n":           "3",
		"data":        "2026-03-01",
	} {
		v, ok := fm.Get(chave)
		if !ok || v != want {
			t.Errorf("%s = %#v (presente=%v), quer %q", chave, v, ok, want)
		}
	}
	if v, _ := fm.Get("tags"); !reflect.DeepEqual(v, []string{"1", "go"}) {
		t.Errorf("tags = %#v, quer [1 go]", v)
	}

	out := mustEmit(t, fm, body)
	fm2, _ := mustRead(t, out)
	if !fm.Equal(fm2) {
		t.Errorf("frontmatter mudou na ida e volta")
	}
	if !bytes.Contains(out, []byte(`pinned: "true"`)) {
		t.Errorf("reescrita deveria emitir pinned com aspas:\n%s", out)
	}
}

func TestMarkdownOrdemPreservadaIdaEVolta(t *testing.T) {
	doc := "---\nzeta: um\nalpha: dois\ntype: Note\nbeta: tres\n---\nbody\n"
	fm, body := mustRead(t, []byte(doc))
	want := []string{"zeta", "alpha", "type", "beta"}
	if got := fm.Keys(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Keys após leitura = %v, quer %v", got, want)
	}
	out := mustEmit(t, fm, body)
	fm2, body2 := mustRead(t, out)
	if got := fm2.Keys(); !reflect.DeepEqual(got, want) {
		t.Errorf("Keys após ida e volta = %v, quer %v", got, want)
	}
	if !fm.Equal(fm2) {
		t.Errorf("frontmatter diferente após ida e volta")
	}
	if body2 != body {
		t.Errorf("corpo mudou: %q, quer %q", body2, body)
	}
}

func TestMarkdownOrdemDerivadaVaiAoFim(t *testing.T) {
	var fm domain.Frontmatter
	mustSet(t, &fm, "title", "Oi", "type", "Note", "description", "d")
	out := mustEmit(t, fm, "x\n")
	want := "---\ntitle: Oi\ntype: Note\ndescription: d\n---\nx\n"
	if string(out) != want {
		t.Errorf("saída = %q, quer %q", out, want)
	}
}

func TestMarkdownCorpoCRLFPreservado(t *testing.T) {
	body := "# A\r\n\r\ntexto\nmisto\r\n"
	var fm domain.Frontmatter
	mustSet(t, &fm, "type", "Note")
	out := mustEmit(t, fm, body)
	fm2, body2 := mustRead(t, out)
	if body2 != body {
		t.Errorf("corpo = %q, quer %q (fins de linha originais)", body2, body)
	}
	if !fm.Equal(fm2) {
		t.Errorf("frontmatter diferente")
	}
	if !bytes.HasSuffix(out, []byte(body)) {
		t.Errorf("saída não termina com o corpo byte a byte: %q", out)
	}
}

func TestMarkdownSemFrontmatterSaiSemBloco(t *testing.T) {
	body := "# So corpo\r\nlinha\n"
	out := mustEmit(t, domain.Frontmatter{}, body)
	if string(out) != body {
		t.Errorf("saída = %q, quer só o corpo %q", out, body)
	}
	fm, b := mustRead(t, out)
	if len(fm.Keys()) != 0 || b != body {
		t.Errorf("releitura: keys=%v body=%q", fm.Keys(), b)
	}
}

func TestMarkdownEmitSemBOMComLF(t *testing.T) {
	var fm domain.Frontmatter
	mustSet(t, &fm, "type", "Note", "title", "Oi", "tags", []string{"a", "b"})
	out := mustEmit(t, fm, "# C\r\n")
	if bytes.HasPrefix(out, []byte("\xEF\xBB\xBF")) {
		t.Fatalf("saída começa com BOM")
	}
	want := "---\ntype: Note\ntitle: Oi\ntags:\n  - a\n  - b\n---\n# C\r\n"
	if string(out) != want {
		t.Errorf("saída = %q\nquer    %q", out, want)
	}
	if bytes.Contains(out[:bytes.Index(out, []byte("# C"))], []byte("\r")) {
		t.Errorf("bloco emitido tem CR; deveria ser só LF")
	}
}

func TestMarkdownEmitValoresAmbiguosIdaEVolta(t *testing.T) {
	var fm domain.Frontmatter
	mustSet(t, &fm,
		"versao", "0.2",
		"flag", "true",
		"vazio", "",
		"data", "2026-10-12",
		"dois_pontos", "a: b",
		"multi", "linha um\nlinha dois",
		"espacos", "  aparado  ",
		"lista_vazia", []string{},
	)
	out := mustEmit(t, fm, "corpo\n")
	fm2, body2 := mustRead(t, out)
	if !fm.Equal(fm2) {
		t.Fatalf("frontmatter mudou na ida e volta; saída:\n%s", out)
	}
	if body2 != "corpo\n" {
		t.Errorf("corpo = %q", body2)
	}
}

func TestMarkdownIdaEVoltaIdempotente(t *testing.T) {
	entradas := []string{
		"---\r\ntype: Note\r\nzeta: x\r\n---\r\n# T\r\nc\r\n",
		"\xEF\xBB\xBF---\ntags: [a, b]\nversao: \"0.2\"\n---\n\n# corpo\n",
		"sem frontmatter\r\n",
		"---\n---\nvazio\n",
		"---\ntype: Note\n---",
	}
	for _, x := range entradas {
		fm1, b1 := mustRead(t, []byte(x))
		y1 := mustEmit(t, fm1, b1)
		fm2, b2 := mustRead(t, y1)
		y2 := mustEmit(t, fm2, b2)
		if !bytes.Equal(y1, y2) {
			t.Errorf("não idempotente para %q:\n y1=%q\n y2=%q", x, y1, y2)
		}
		if !fm1.Equal(fm2) || b1 != b2 {
			t.Errorf("ida e volta alterou o documento %q", x)
		}
	}
}

func TestMarkdownReescritaSemMudancaPreservaBytes(t *testing.T) {
	var fm domain.Frontmatter
	mustSet(t, &fm, "type", "Note", "title", "Oi")
	original := mustEmit(t, fm, "# C\r\n\r\ncorpo\n")
	fm2, body2 := mustRead(t, original)
	again := mustEmit(t, fm2, body2)
	if !bytes.Equal(original, again) {
		t.Errorf("reescrita sem mudança alterou bytes:\n antes=%q\n depois=%q", original, again)
	}
}
