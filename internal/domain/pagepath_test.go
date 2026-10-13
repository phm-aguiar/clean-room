package domain

import (
	"errors"
	"testing"
)

func TestParsePagePath_Aceita(t *testing.T) {
	validos := []string{
		"notes/foo.md",
		"a/b/c.md",
		"_rules/regra.md",   // ADR-05: não reservado
		"_lint/x.md",        // ADR-05: não reservado
		"_slots/s.md",       // ADR-05: não reservado
		"_global/g.md",      // ADR-05: não reservado
		"notes/log-x.md",    // não é log-AAAA-MM
		"notes/CONTRATO.md", // nome que só começa como CON, mas não é reservado
	}
	for _, s := range validos {
		p, err := ParsePagePath(s)
		if err != nil {
			t.Errorf("ParsePagePath(%q) erro inesperado: %v", s, err)
			continue
		}
		if p.String() != s {
			t.Errorf("ParsePagePath(%q).String() = %q", s, p.String())
		}
	}
}

func TestParsePagePath_Rejeita(t *testing.T) {
	invalidos := []string{
		// DV §4.2
		"",
		"../x.md",
		"a/../b.md",
		"a/./b.md",
		"/abs.md",
		"C:/x.md",
		"a\\b.md",
		"a//b.md",
		"a/",
		// ADR-04: nomes portáveis
		"CON.md",
		"con",
		"nul.txt",
		"PRN",
		"aux.md",
		"COM1.md",
		"lpt9",
		"a<b.md",
		"a>b.md",
		"a:b.md",
		"a\"b.md",
		"a|b.md",
		"q?.md",
		"st*r.md",
		"tab\x01.md",
		"trailing./x.md",
		"trailing.",
		"trailing /x.md",
		"dir/name ",
		// ADR-05: reservados
		".git/config",
		"x/.git",
		".ai-memory-tmp.abc/x.md",
		"_pending/x.md",
		"_graveyard/x.md",
		"p/_meta.md",
		"p/index.md",
		"p/bootstrap.md",
		"p/log.md",
		"p/log-2026-10.md",
	}
	for _, s := range invalidos {
		_, err := ParsePagePath(s)
		if err == nil {
			t.Errorf("ParsePagePath(%q) deveria recusar", s)
			continue
		}
		if !errors.Is(err, ErrInvalidPagePath) {
			t.Errorf("ParsePagePath(%q) erro não encadeia ErrInvalidPagePath: %v", s, err)
		}
		var pe *PagePathError
		if !errors.As(err, &pe) {
			t.Errorf("ParsePagePath(%q) erro não é *PagePathError: %T", s, err)
		}
	}
}

func TestParsePagePath_RejeitaCasos(t *testing.T) {
	// Caixa não importa para nomes reservados.
	for _, s := range []string{"con", "Con.MD", "NUL", "cOm1.md", ".GIT/x", "P/_META.md", "_PENDING/x.md"} {
		if _, err := ParsePagePath(s); err == nil {
			t.Errorf("ParsePagePath(%q) deveria recusar (caixa)", s)
		}
	}
}

func TestParseStoredPagePath_AceitaLegado(t *testing.T) {
	// Linhas já persistidas: só as regras de DV §4.2.
	legados := []string{"CON.md", "_meta.md", ".git/x.md", "a<b.md", "trailing./x.md"}
	for _, s := range legados {
		if _, err := ParseStoredPagePath(s); err != nil {
			t.Errorf("ParseStoredPagePath(%q) deveria aceitar (legado): %v", s, err)
		}
	}
}

func TestParseStoredPagePath_RejeitaDV(t *testing.T) {
	invalidos := []string{
		"",
		"..",
		"../x.md",
		"a/../b.md",
		"a/./b.md",
		"/x.md",
		"C:/x.md",
		"c:x.md",
		"a\\b.md",
		"a//b.md",
		"a/",
	}
	for _, s := range invalidos {
		_, err := ParseStoredPagePath(s)
		if err == nil {
			t.Errorf("ParseStoredPagePath(%q) deveria recusar", s)
			continue
		}
		if !errors.Is(err, ErrInvalidPagePath) {
			t.Errorf("ParseStoredPagePath(%q) erro não encadeia ErrInvalidPagePath: %v", s, err)
		}
	}
}

func TestPortableKey_Minusculas(t *testing.T) {
	a := mustParse(t, "Notes/Foo.md")
	b := mustParse(t, "notes/foo.md")
	if a.PortableKey() != b.PortableKey() {
		t.Errorf("chaves deveriam colidir: %q vs %q", a.PortableKey(), b.PortableKey())
	}
	if got := a.PortableKey(); got != "notes/foo.md" {
		t.Errorf("PortableKey = %q, quer %q", got, "notes/foo.md")
	}
}

func TestPortableKey_NFC(t *testing.T) {
	// "Café" decomposto (e + U+0301) e composto (U+00E9) devem colidir.
	decomposto := mustParse(t, "cafe\u0301.md")
	composto := mustParse(t, "caf\u00e9.md")
	if decomposto.PortableKey() != composto.PortableKey() {
		t.Errorf("NFC: %q vs %q", decomposto.PortableKey(), composto.PortableKey())
	}
	if got := decomposto.PortableKey(); got != "caf\u00e9.md" {
		t.Errorf("PortableKey decomposto = %q, quer forma NFC", got)
	}
}

func TestPortableKey_DottedCapitalI(t *testing.T) {
	// Ordem: minúsculas (strings.ToLower) antes de NFC (ADR-06).
	// Nota: strings.ToLower usa mapeamento simples, então "İ" vira "i" (não "i"+U+0307
	// como descreve DV §4.2). Por isso "İ.md" colide com "i.md". Ver relatório.
	p := mustParse(t, "\u0130.md")
	if got := p.PortableKey(); got != "i.md" {
		t.Errorf("PortableKey(İ.md) = %q, quer %q", got, "i.md")
	}
}

func TestSearchText_ExemploOficial(t *testing.T) {
	p := mustParse(t, "notes/foo-bar.md")
	want := "notes foo-bar md notes foo bar md"
	if got := p.SearchText(); got != want {
		t.Errorf("SearchText = %q, quer %q", got, want)
	}
}

func TestSearchText_Underscore(t *testing.T) {
	p := mustParse(t, "a_b/c.md")
	want := "a_b c md a b c md"
	if got := p.SearchText(); got != want {
		t.Errorf("SearchText = %q, quer %q", got, want)
	}
}

func mustParse(t *testing.T, s string) PagePath {
	t.Helper()
	p, err := ParsePagePath(s)
	if err != nil {
		t.Fatalf("ParsePagePath(%q): %v", s, err)
	}
	return p
}
