package domain

import "testing"

// mustParseID é auxiliar de teste: IDs fixos e válidos.
func mustParseID(t *testing.T, s string) ID {
	t.Helper()
	id, err := ParseID(s)
	if err != nil {
		t.Fatalf("ParseID(%q): %v", s, err)
	}
	return id
}

// mustParsePath é auxiliar de teste: caminhos fixos e válidos.
func mustParsePath(t *testing.T, s string) PagePath {
	t.Helper()
	p, err := ParsePagePath(s)
	if err != nil {
		t.Fatalf("ParsePagePath(%q): %v", s, err)
	}
	return p
}

func TestScope_MesmoValorEhIgualPorIgualdade(t *testing.T) {
	ws := WorkspaceID(mustParseID(t, "018f3c2a-7b1d-7000-8000-000000000001"))
	pr := ProjectID(mustParseID(t, "018f3c2a-7b1d-7000-8000-000000000002"))

	a := NewScope(ws, pr)
	b := NewScope(ws, pr)
	if a != b {
		t.Fatal("Scope com os mesmos IDs deveria ser igual com ==")
	}
}

func TestPageRef_MesmoPathProjetosDiferentesSaoDistintos(t *testing.T) {
	ws := WorkspaceID(mustParseID(t, "018f3c2a-7b1d-7000-8000-000000000001"))
	projA := ProjectID(mustParseID(t, "018f3c2a-7b1d-7000-8000-00000000000a"))
	projB := ProjectID(mustParseID(t, "018f3c2a-7b1d-7000-8000-00000000000b"))
	path := mustParsePath(t, "notas/rota.md")

	a := NewPageRef(NewScope(ws, projA), path)
	b := NewPageRef(NewScope(ws, projB), path)

	if a == b {
		t.Fatal("mesmo path em projetos diferentes não pode ser o mesmo PageRef (INV-04)")
	}
}

func TestPageRef_MesmoPageRefEhIgualPorIgualdade(t *testing.T) {
	ws := WorkspaceID(mustParseID(t, "018f3c2a-7b1d-7000-8000-000000000001"))
	pr := ProjectID(mustParseID(t, "018f3c2a-7b1d-7000-8000-000000000002"))
	path := mustParsePath(t, "decisions/api.md")

	a := NewPageRef(NewScope(ws, pr), path)
	b := NewPageRef(NewScope(ws, pr), mustParsePath(t, "decisions/api.md"))

	if a != b {
		t.Fatal("PageRef com os mesmos três elementos deveria ser igual com ==")
	}
}

func TestPageRef_String(t *testing.T) {
	ws := WorkspaceID(mustParseID(t, "018F3C2A-7B1D-7000-8000-000000000001"))
	pr := ProjectID(mustParseID(t, "018f3c2a-7b1d-7000-8000-000000000002"))
	ref := NewPageRef(NewScope(ws, pr), mustParsePath(t, "sessions/2026-10-12.md"))

	want := "018f3c2a-7b1d-7000-8000-000000000001/018f3c2a-7b1d-7000-8000-000000000002:sessions/2026-10-12.md"
	if got := ref.String(); got != want {
		t.Fatalf("String() = %q, quer %q", got, want)
	}
}

func TestPageRef_CaixaDiferenteEhDistintoPorIgualdade(t *testing.T) {
	ws := WorkspaceID(mustParseID(t, "018f3c2a-7b1d-7000-8000-000000000001"))
	pr := ProjectID(mustParseID(t, "018f3c2a-7b1d-7000-8000-000000000002"))
	s := NewScope(ws, pr)

	a := NewPageRef(s, mustParsePath(t, "Notas/Rota.md"))
	b := NewPageRef(s, mustParsePath(t, "notas/rota.md"))

	// Por igualdade são distintos. A colisão por caixa é tratada depois, por PortableKey, em storage.
	if a == b {
		t.Fatal("PageRef com caixa diferente deveria ser distinto por == (colisão fica para PortableKey)")
	}
}
