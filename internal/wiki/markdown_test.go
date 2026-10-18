package wiki

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
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
		"sintaxe":                   "---\ntype: [Note\n---\ncorpo\n",
		"nulo":                      "---\ntitle:\n---\n",
		"nulo em outra chave":       "---\ngenerated:\n  by: x\n  at: null\n---\n",
		"nulo em entities aninhado": "---\ngenerated:\n  entities:\n---\n",
		"nulo em item de lista":     "---\nsources:\n  - resource: r\n  -\n---\n",
		"lista mista":               "---\nsources:\n  - resource: r\n  - texto\n---\n",
		"lista de texto com mapa":   "---\ntags:\n  - a\n  - b: 1\n---\n",
		"chave duplicada":           "---\na: x\na: y\n---\n",
		"chave duplicada aninhada":  "---\ngenerated:\n  by: x\n  by: y\n---\n",
		"chave nao texto aninhada":  "---\ngenerated:\n  1: x\n---\n",
		"raiz sequencia":            "---\n- a\n- b\n---\n",
		"alias":                     "---\na: &x v\nb: *x\n---\n",
		"alias aninhado":            "---\ngenerated:\n  by: &x v\n  at: *x\n---\n",
		"alias em lista de mapas":   "---\nsources:\n  - &m\n    resource: r\n  - *m\n---\n",
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

// mapaDe lê o valor de key como mapeamento aninhado e falha o teste se não for.
func mapaDe(t *testing.T, fm domain.Frontmatter, key string) domain.Frontmatter {
	t.Helper()
	v, ok := fm.Get(key)
	if !ok {
		t.Fatalf("chave %q ausente", key)
	}
	m, ok := v.(domain.Frontmatter)
	if !ok {
		t.Fatalf("chave %q = %T, quer domain.Frontmatter", key, v)
	}
	return m
}

// listaDeMapasDe lê o valor de key como lista de mapeamentos e falha o teste se não for.
func listaDeMapasDe(t *testing.T, fm domain.Frontmatter, key string) []domain.Frontmatter {
	t.Helper()
	v, ok := fm.Get(key)
	if !ok {
		t.Fatalf("chave %q ausente", key)
	}
	l, ok := v.([]domain.Frontmatter)
	if !ok {
		t.Fatalf("chave %q = %T, quer []domain.Frontmatter", key, v)
	}
	return l
}

// TestMarkdownLeMapeamentoEListaDeMapas: generated (mapeamento), sources (lista de mapeamentos) e
// relations (mapeamento com listas de texto) são lidos com a ordem das chaves preservada.
func TestMarkdownLeMapeamentoEListaDeMapas(t *testing.T) {
	doc := "---\ntype: Note\ngenerated:\n  by: process:ai-memory/x\n  at: 2026-03-01T10:00:00Z\n" +
		"sources:\n  - resource: ai-memory://session/s1\n    author: process:claude\n" +
		"relations:\n  fixes:\n    - gotchas/a.md\n  causes:\n    - decisions/b.md\n---\ncorpo\n"
	fm, body := mustRead(t, []byte(doc))
	if got, want := fm.Keys(), []string{"type", "generated", "sources", "relations"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Keys = %v, quer %v", got, want)
	}

	gen := mapaDe(t, fm, "generated")
	if got, want := gen.Keys(), []string{"by", "at"}; !reflect.DeepEqual(got, want) {
		t.Errorf("generated.Keys = %v, quer %v", got, want)
	}
	if v, _ := gen.Get("at"); v != "2026-03-01T10:00:00Z" {
		t.Errorf("generated.at = %#v, quer o texto literal", v)
	}

	src := listaDeMapasDe(t, fm, "sources")
	if len(src) != 1 {
		t.Fatalf("sources tem %d itens, quer 1", len(src))
	}
	if got, want := src[0].Keys(), []string{"resource", "author"}; !reflect.DeepEqual(got, want) {
		t.Errorf("sources[0].Keys = %v, quer %v", got, want)
	}
	if v, _ := src[0].Get("resource"); v != "ai-memory://session/s1" {
		t.Errorf("sources[0].resource = %#v", v)
	}

	rel := mapaDe(t, fm, "relations")
	if got, want := rel.Keys(), []string{"fixes", "causes"}; !reflect.DeepEqual(got, want) {
		t.Errorf("relations.Keys = %v, quer %v", got, want)
	}
	if v, _ := rel.Get("fixes"); !reflect.DeepEqual(v, []string{"gotchas/a.md"}) {
		t.Errorf("relations.fixes = %#v", v)
	}
	if body != "corpo\n" {
		t.Errorf("body = %q", body)
	}
}

// TestMarkdownOrdemDeChavesAninhadasNaEmissao: a ordem das chaves de mapeamentos e de itens de
// lista aninhados sai como foi gravada, e a ida e volta é idempotente.
func TestMarkdownOrdemDeChavesAninhadasNaEmissao(t *testing.T) {
	doc := "---\ngenerated:\n  at: 2026-03-01T10:00:00Z\n  by: process:x\n" +
		"sources:\n  - author: process:claude\n    resource: ai-memory://session/s1\n" +
		"relations:\n  contradicts:\n    - decisions/0007.md\n  fixes:\n    - gotchas/a.md\n---\ncorpo\n"
	fm, body := mustRead(t, []byte(doc))
	out := mustEmit(t, fm, body)

	fm2, body2 := mustRead(t, out)
	if !fm.Equal(fm2) {
		t.Fatalf("frontmatter mudou na ida e volta:\n%s", out)
	}
	if body2 != body {
		t.Errorf("corpo = %q, quer %q", body2, body)
	}
	if got, want := mapaDe(t, fm2, "generated").Keys(), []string{"at", "by"}; !reflect.DeepEqual(got, want) {
		t.Errorf("generated.Keys após ida e volta = %v, quer %v", got, want)
	}
	if got, want := listaDeMapasDe(t, fm2, "sources")[0].Keys(), []string{"author", "resource"}; !reflect.DeepEqual(got, want) {
		t.Errorf("sources[0].Keys após ida e volta = %v, quer %v", got, want)
	}
	if got, want := mapaDe(t, fm2, "relations").Keys(), []string{"contradicts", "fixes"}; !reflect.DeepEqual(got, want) {
		t.Errorf("relations.Keys após ida e volta = %v, quer %v", got, want)
	}

	s := string(out)
	if !(strings.Index(s, "at:") < strings.Index(s, "by:") &&
		strings.Index(s, "author:") < strings.Index(s, "resource:") &&
		strings.Index(s, "contradicts:") < strings.Index(s, "fixes:")) {
		t.Errorf("ordem das chaves aninhadas não preservada na saída:\n%s", s)
	}

	again := mustEmit(t, fm2, body2)
	if !bytes.Equal(out, again) {
		t.Errorf("emissão não idempotente:\n y1=%q\n y2=%q", out, again)
	}
}

// TestMarkdownEmitMapeamentoEListaDeMapasIdaEVolta: valores montados por Set (generated, sources
// e relations) saem no bloco e voltam iguais, com a mesma ordem.
func TestMarkdownEmitMapeamentoEListaDeMapasIdaEVolta(t *testing.T) {
	var gen domain.Frontmatter
	mustSet(t, &gen, "by", "process:ai-memory/0.1", "at", "2026-03-01T10:00:00Z")
	var src domain.Frontmatter
	mustSet(t, &src, "resource", "ai-memory://session/s1", "author", "process:claude")
	var rel domain.Frontmatter
	mustSet(t, &rel, "causes", []string{"decisions/b.md"}, "fixes", []string{"gotchas/a.md"})

	var fm domain.Frontmatter
	mustSet(t, &fm,
		"type", "Note",
		"generated", gen,
		"sources", []domain.Frontmatter{src},
		"relations", rel,
	)
	out := mustEmit(t, fm, "corpo\n")
	fm2, body2 := mustRead(t, out)
	if !fm.Equal(fm2) {
		t.Fatalf("frontmatter mudou na ida e volta; saída:\n%s", out)
	}
	if got, want := fm2.Keys(), []string{"type", "generated", "sources", "relations"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Keys = %v, quer %v", got, want)
	}
	if body2 != "corpo\n" {
		t.Errorf("corpo = %q", body2)
	}
}

// TestMarkdownEntitiesNuloEhListaVazia: entities nulo é aceito e lido como lista vazia, no topo.
func TestMarkdownEntitiesNuloEhListaVazia(t *testing.T) {
	for _, doc := range []string{
		"---\ntype: Note\nentities:\n---\ncorpo\n",
		"---\ntype: Note\nentities: null\n---\ncorpo\n",
		"---\ntype: Note\nentities: ~\n---\ncorpo\n",
	} {
		fm, body := mustRead(t, []byte(doc))
		v, ok := fm.Get("entities")
		if !ok || !reflect.DeepEqual(v, []string{}) {
			t.Errorf("entities = %#v (presente=%v) para %q, quer lista vazia", v, ok, doc)
		}
		if got, want := fm.Keys(), []string{"type", "entities"}; !reflect.DeepEqual(got, want) {
			t.Errorf("Keys = %v, quer %v", got, want)
		}
		if body != "corpo\n" {
			t.Errorf("body = %q", body)
		}
	}
}
