package domain

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/phm-aguiar/ai-memory-go/internal/domain/sanitize"
)

func TestFrontmatter_PreservaOrdemDeInsercao(t *testing.T) {
	var f Frontmatter
	for _, k := range []string{"type", "title", "tier", "kind"} {
		if err := f.Set(k, "v-"+k); err != nil {
			t.Fatalf("Set(%q): %v", k, err)
		}
	}
	want := []string{"type", "title", "tier", "kind"}
	if got := f.Keys(); !reflect.DeepEqual(got, want) {
		t.Errorf("Keys() = %v, esperado %v", got, want)
	}
}

func TestFrontmatter_SetSubstituiSemReordenar(t *testing.T) {
	var f Frontmatter
	_ = f.Set("type", "fact")
	_ = f.Set("title", "A")
	_ = f.Set("tier", "semantic")
	if err := f.Set("title", "B"); err != nil {
		t.Fatalf("Set de chave existente: %v", err)
	}
	if got := f.Keys(); !reflect.DeepEqual(got, []string{"type", "title", "tier"}) {
		t.Errorf("Keys() = %v, substituição não pode reordenar", got)
	}
	v, ok := f.Get("title")
	if !ok || v != "B" {
		t.Errorf("Get(title) = %v, %v; esperado \"B\", true", v, ok)
	}
}

func TestFrontmatter_GetAusente(t *testing.T) {
	var f Frontmatter
	if v, ok := f.Get("nada"); ok || v != nil {
		t.Errorf("Get em mapa vazio = %v, %v", v, ok)
	}
}

func TestFrontmatter_ListaEhCopiada(t *testing.T) {
	var f Frontmatter
	src := []string{"a", "b"}
	if err := f.Set("tags", src); err != nil {
		t.Fatalf("Set(tags): %v", err)
	}
	src[0] = "X"
	v, _ := f.Get("tags")
	got := v.([]string)
	if !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("valor alterado pela origem: %v", got)
	}
	got[1] = "Y"
	again, _ := f.Get("tags")
	if !reflect.DeepEqual(again, []string{"a", "b"}) {
		t.Errorf("Get devolveu slice compartilhado: %v", again)
	}
}

func TestFrontmatter_RecusaTipoEChaveVazia(t *testing.T) {
	var f Frontmatter
	if err := f.Set("n", 3); !errors.Is(err, ErrInvalidFrontmatterValue) {
		t.Errorf("Set com int: erro = %v, esperado ErrInvalidFrontmatterValue", err)
	}
	if err := f.Set("", "x"); !errors.Is(err, ErrInvalidFrontmatterKey) {
		t.Errorf("Set com chave vazia: erro = %v, esperado ErrInvalidFrontmatterKey", err)
	}
	if len(f.Keys()) != 0 {
		t.Errorf("Set recusado alterou o mapa: %v", f.Keys())
	}
}

func TestFrontmatter_EqualDeterministico(t *testing.T) {
	var a, b, c Frontmatter
	_ = a.Set("type", "fact")
	_ = a.Set("tags", []string{"x"})
	_ = b.Set("type", "fact")
	_ = b.Set("tags", []string{"x"})
	_ = c.Set("tags", []string{"x"})
	_ = c.Set("type", "fact")
	if !a.Equal(b) {
		t.Error("mesmas chaves, mesma ordem e mesmos valores deveriam ser iguais")
	}
	if a.Equal(c) {
		t.Error("ordem diferente deveria ser diferente")
	}
}

func TestFrontmatter_MapeamentoAninhadoPreservaOrdemECopia(t *testing.T) {
	var gen Frontmatter
	_ = gen.Set("at", "2026-03-01T10:00:00Z")
	_ = gen.Set("by", "process:x")
	var fm Frontmatter
	if err := fm.Set("generated", gen); err != nil {
		t.Fatalf("Set(generated, mapeamento): %v", err)
	}
	// Alterar o original depois de gravar não pode mudar o frontmatter.
	_ = gen.Set("by", "mudado")

	v, ok := fm.Get("generated")
	if !ok {
		t.Fatal("Get(generated) ausente")
	}
	got, ok := v.(Frontmatter)
	if !ok {
		t.Fatalf("Get(generated) = %T, quer Frontmatter", v)
	}
	if want := []string{"at", "by"}; !reflect.DeepEqual(got.Keys(), want) {
		t.Errorf("Keys aninhadas = %v, quer %v", got.Keys(), want)
	}
	if by, _ := got.Get("by"); by != "process:x" {
		t.Errorf("by = %v, quer process:x (cópia independente)", by)
	}

	// Alterar o valor devolvido por Get também não pode mudar o frontmatter.
	_ = got.Set("by", "outro")
	again, _ := fm.Get("generated")
	if by, _ := again.(Frontmatter).Get("by"); by != "process:x" {
		t.Errorf("Get devolveu mapeamento compartilhado: by = %v", by)
	}
}

func TestFrontmatter_ListaDeMapasEhCopiadaEOrdenada(t *testing.T) {
	var item Frontmatter
	_ = item.Set("resource", "ai-memory://session/s1")
	_ = item.Set("author", "process:claude")
	var fm Frontmatter
	if err := fm.Set("sources", []Frontmatter{item}); err != nil {
		t.Fatalf("Set(sources, lista de mapas): %v", err)
	}
	_ = item.Set("resource", "mudado")

	v, _ := fm.Get("sources")
	lista, ok := v.([]Frontmatter)
	if !ok || len(lista) != 1 {
		t.Fatalf("Get(sources) = %#v, quer lista de 1 mapeamento", v)
	}
	if want := []string{"resource", "author"}; !reflect.DeepEqual(lista[0].Keys(), want) {
		t.Errorf("Keys do item = %v, quer %v", lista[0].Keys(), want)
	}
	if res, _ := lista[0].Get("resource"); res != "ai-memory://session/s1" {
		t.Errorf("resource = %v, quer o valor gravado (cópia)", res)
	}
	_ = lista[0].Set("resource", "outro")
	again, _ := fm.Get("sources")
	if res, _ := again.([]Frontmatter)[0].Get("resource"); res != "ai-memory://session/s1" {
		t.Errorf("Get devolveu lista compartilhada: resource = %v", res)
	}
}

func TestFrontmatter_ListaVaziaDeMapasViraListaDeTexto(t *testing.T) {
	var fm Frontmatter
	if err := fm.Set("sources", []Frontmatter{}); err != nil {
		t.Fatalf("Set(sources, vazia): %v", err)
	}
	v, _ := fm.Get("sources")
	if !reflect.DeepEqual(v, []string{}) {
		t.Errorf("Get(sources) = %#v, quer []string vazia", v)
	}
}

func TestFrontmatter_EqualRecursivoSensivelAOrdem(t *testing.T) {
	var g1, g2, g3 Frontmatter
	_ = g1.Set("by", "p")
	_ = g1.Set("at", "t")
	_ = g2.Set("by", "p")
	_ = g2.Set("at", "t")
	_ = g3.Set("at", "t")
	_ = g3.Set("by", "p")

	var a, b, c Frontmatter
	_ = a.Set("generated", g1)
	_ = b.Set("generated", g2)
	_ = c.Set("generated", g3)
	if !a.Equal(b) {
		t.Error("mapeamentos aninhados iguais deveriam ser iguais")
	}
	if a.Equal(c) {
		t.Error("ordem das chaves aninhadas faz parte da igualdade")
	}

	var d, e Frontmatter
	_ = d.Set("sources", []Frontmatter{g1})
	_ = e.Set("sources", []Frontmatter{g3})
	if d.Equal(e) {
		t.Error("lista de mapas com ordem interna diferente deveria ser diferente")
	}
}

func TestFrontmatter_RecusaTipoNaoAceitoEmAninhado(t *testing.T) {
	var f Frontmatter
	if err := f.Set("x", map[string]string{"a": "b"}); !errors.Is(err, ErrInvalidFrontmatterValue) {
		t.Errorf("Set com map[string]string: erro = %v, esperado ErrInvalidFrontmatterValue", err)
	}
	if err := f.Set("x", []any{"a"}); !errors.Is(err, ErrInvalidFrontmatterValue) {
		t.Errorf("Set com []any: erro = %v, esperado ErrInvalidFrontmatterValue", err)
	}
	if len(f.Keys()) != 0 {
		t.Errorf("Set recusado alterou o mapa: %v", f.Keys())
	}
}

func TestObservation_ImportanciaLimites(t *testing.T) {
	ref := testObservationFixture(t)

	for _, n := range []int{0, 11, -1} {
		_, err := NewObservation(ref.id, ref.session, ref.scope, ObservationStop, ref.title, ref.body, n)
		if !errors.Is(err, ErrInvalidImportance) {
			t.Errorf("importância %d: erro = %v, esperado ErrInvalidImportance", n, err)
		}
	}
	for _, n := range []int{1, DefaultImportance, 10} {
		obs, err := NewObservation(ref.id, ref.session, ref.scope, ObservationStop, ref.title, ref.body, n)
		if err != nil {
			t.Errorf("importância %d deveria ser aceita: %v", n, err)
			continue
		}
		if obs.Importance != n {
			t.Errorf("Importance = %d, esperado %d", obs.Importance, n)
		}
	}
	if DefaultImportance != 5 {
		t.Errorf("DefaultImportance = %d, esperado 5", DefaultImportance)
	}
}

func TestObservation_RecusaKindForaDoConjunto(t *testing.T) {
	ref := testObservationFixture(t)
	_, err := NewObservation(ref.id, ref.session, ref.scope, ObservationKind("inventado"), ref.title, ref.body, 5)
	if !errors.Is(err, ErrInvalidObservationKind) {
		t.Errorf("erro = %v, esperado ErrInvalidObservationKind", err)
	}
}

func TestObservation_SoAceitaSanitized(t *testing.T) {
	ref := testObservationFixture(t)
	obs, err := NewObservation(ref.id, ref.session, ref.scope, ObservationUserPrompt, ref.title, ref.body, DefaultImportance)
	if err != nil {
		t.Fatalf("NewObservation: %v", err)
	}
	if obs.Title.Text() != "Usar token" {
		t.Errorf("Title.Text() = %q", obs.Title.Text())
	}
	if strings.Contains(obs.Body.Text(), "sk-ABCDEFGHIJKLMNOP1234") {
		t.Errorf("segredo chegou ao corpo sem redação: %q", obs.Body.Text())
	}
	if !strings.Contains(obs.Body.Text(), "[REDACTED:api_key]") {
		t.Errorf("o corpo sanitizado deveria conter o marcador de redação: %q", obs.Body.Text())
	}
	if obs.OccurredAt != 0 || obs.Extension != "" || obs.SourceEvent != "" {
		t.Errorf("campos opcionais deveriam ser zero: %+v", obs)
	}
}

func TestObservation_TituloRespeitaTetoDeObservacao(t *testing.T) {
	lim := sanitize.Limits{TitleMaxRunes: sanitize.ObservationTitleMaxRunes, BodyMaxBytes: sanitize.ObservationBodyMaxBytes}
	longo := strings.Repeat("a", 200)
	title := sanitize.Title(longo, lim)
	if n := len([]rune(title.Text())); n != sanitize.ObservationTitleMaxRunes {
		t.Errorf("título com %d runes, esperado %d", n, sanitize.ObservationTitleMaxRunes)
	}
}

func TestPageVersion_SupersedesEValidToZero(t *testing.T) {
	anterior := mustParseID(t, "018f3c2a-7b1d-7000-8000-0000000000a1")
	atual := mustParseID(t, "018f3c2a-7b1d-7000-8000-0000000000a2")
	prev := PageID(anterior)

	v := PageVersion{
		ID:         PageID(atual),
		Supersedes: &prev,
		IsLatest:   true,
		ValidFrom:  1_700_000_000_000_000,
		ValidTo:    0,
	}
	if v.Supersedes == nil || *v.Supersedes != PageID(anterior) {
		t.Fatalf("Supersedes não aponta para a versão anterior: %v", v.Supersedes)
	}
	if v.ValidTo != 0 {
		t.Errorf("ValidTo = %d, esperado 0 (aberto)", v.ValidTo)
	}
	if !v.IsLatest {
		t.Error("versão corrente deveria ter IsLatest true")
	}

	primeira := PageVersion{ID: PageID(anterior)}
	if primeira.Supersedes != nil {
		t.Error("primeira versão deveria ter Supersedes nil")
	}
}

// observationFixture junta os insumos comuns dos testes de observação.
type observationFixture struct {
	id      ObservationID
	session SessionID
	scope   Scope
	title   sanitize.Sanitized
	body    sanitize.Sanitized
}

func testObservationFixture(t *testing.T) observationFixture {
	t.Helper()
	lim := sanitize.Limits{TitleMaxRunes: sanitize.ObservationTitleMaxRunes, BodyMaxBytes: sanitize.ObservationBodyMaxBytes}
	ws := WorkspaceID(mustParseID(t, "018f3c2a-7b1d-7000-8000-000000000001"))
	pr := ProjectID(mustParseID(t, "018f3c2a-7b1d-7000-8000-000000000002"))
	return observationFixture{
		id:      ObservationID(mustParseID(t, "018f3c2a-7b1d-7000-8000-0000000000b1")),
		session: SessionID(mustParseID(t, "018f3c2a-7b1d-7000-8000-0000000000c1")),
		scope:   NewScope(ws, pr),
		title:   sanitize.Title("Usar token", lim),
		body:    sanitize.Body("chave sk-ABCDEFGHIJKLMNOP1234 no código", lim),
	}
}
