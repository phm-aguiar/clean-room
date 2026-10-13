package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestParseTier_Aceita(t *testing.T) {
	validos := []string{"working", "episodic", "semantic", "procedural"}
	for _, s := range validos {
		got, err := ParseTier(s)
		if err != nil {
			t.Errorf("ParseTier(%q) erro inesperado: %v", s, err)
			continue
		}
		if string(got) != s {
			t.Errorf("ParseTier(%q) = %q", s, got)
		}
	}
}

func TestParseTier_Recusa(t *testing.T) {
	invalidos := []string{"", "Working", "SEMANTIC", " semantic", "semantic ", "fact", "1", "sessions"}
	for _, s := range invalidos {
		_, err := ParseTier(s)
		if err == nil {
			t.Errorf("ParseTier(%q) deveria recusar", s)
			continue
		}
		if !errors.Is(err, ErrMalformedRecord) {
			t.Errorf("ParseTier(%q) erro sem ErrMalformedRecord: %v", s, err)
		}
	}
}

func TestParsePageKind_Vazio(t *testing.T) {
	for _, s := range []string{"", "   ", "\t \n"} {
		got, err := ParsePageKind(s)
		if err != nil {
			t.Errorf("ParsePageKind(%q) erro inesperado: %v", s, err)
		}
		if got != "" {
			t.Errorf("ParsePageKind(%q) = %q, esperado zero value (ausente)", s, got)
		}
	}
}

func TestParsePageKind_Constantes(t *testing.T) {
	for _, k := range []PageKind{PageKindFact, PageKindNote, PageKindProcedure, PageKindDecision} {
		got, err := ParsePageKind(string(k))
		if err != nil || got != k {
			t.Errorf("ParsePageKind(%q) = %q, %v", k, got, err)
		}
	}
}

func TestParsePageKind_TamanhoLimite(t *testing.T) {
	if _, err := ParsePageKind(strings.Repeat("a", 64)); err != nil {
		t.Errorf("64 caracteres deveria ser aceito: %v", err)
	}
	if _, err := ParsePageKind(strings.Repeat("a", 65)); err == nil {
		t.Error("65 caracteres deveria ser recusado")
	} else if !errors.Is(err, ErrInvalidPageKind) {
		t.Errorf("65 caracteres: erro sem ErrInvalidPageKind: %v", err)
	}
	// O limite conta caracteres, não bytes: 64 letras acentuadas (2 bytes cada) são aceitas.
	if _, err := ParsePageKind(strings.Repeat("é", 64)); err != nil {
		t.Errorf("64 caracteres multibyte deveria ser aceito: %v", err)
	}
	if _, err := ParsePageKind(strings.Repeat("é", 65)); err == nil {
		t.Error("65 caracteres multibyte deveria ser recusado")
	}
}

func TestParsePageKind_TamanhoAposAparagem(t *testing.T) {
	got, err := ParsePageKind("  " + strings.Repeat("a", 64) + "  ")
	if err != nil {
		t.Errorf("64 caracteres com espaços nas pontas deveria ser aceito: %v", err)
	}
	if string(got) != strings.Repeat("a", 64) {
		t.Errorf("aparagem incorreta: %q", got)
	}
}

func TestParsePageKind_RecusaControle(t *testing.T) {
	invalidos := []string{
		"fa\x00ct",   // NUL
		"fa\x1bct",   // ESC
		"fa\x7fct",   // DEL
		"fa\nct",     // LF no meio
		"fa\tct",     // TAB no meio
		"fa\u0085ct", // C1 NEL
	}
	for _, s := range invalidos {
		_, err := ParsePageKind(s)
		if err == nil {
			t.Errorf("ParsePageKind(%q) deveria recusar controle", s)
			continue
		}
		if !errors.Is(err, ErrInvalidPageKind) {
			t.Errorf("ParsePageKind(%q) erro sem ErrInvalidPageKind: %v", s, err)
		}
	}
}

func TestParsePageKind_Aparagem(t *testing.T) {
	cases := map[string]PageKind{
		"  fact  ":     PageKindFact,
		"\tnote\n":     PageKindNote,
		"  procedure":  PageKindProcedure,
		"decision   ":  PageKindDecision,
		"  uso livre ": "uso livre",
	}
	for in, want := range cases {
		got, err := ParsePageKind(in)
		if err != nil {
			t.Errorf("ParsePageKind(%q) erro inesperado: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParsePageKind(%q) = %q, esperado %q", in, got, want)
		}
	}
}

func TestParsePageKind_TextoLivreAceito(t *testing.T) {
	got, err := ParsePageKind("guia-de-deploy")
	if err != nil || got != "guia-de-deploy" {
		t.Errorf("texto livre: %q, %v", got, err)
	}
}

func TestParseObservationKind_Canonicos(t *testing.T) {
	canonicos := []string{
		"session-start", "user-prompt", "pre-tool-use", "post-tool-use", "pre-compact",
		"post-compaction", "notification", "stop", "session-end", "other",
	}
	if len(observationKinds) != len(canonicos) {
		t.Fatalf("esperados %d canônicos, há %d", len(canonicos), len(observationKinds))
	}
	for _, s := range canonicos {
		got, err := ParseObservationKind(s)
		if err != nil {
			t.Errorf("ParseObservationKind(%q) erro inesperado: %v", s, err)
			continue
		}
		if string(got) != s {
			t.Errorf("ParseObservationKind(%q) = %q", s, got)
		}
	}
}

func TestParseObservationKind_Variantes(t *testing.T) {
	cases := map[string]ObservationKind{
		"UserPromptSubmit": ObservationUserPrompt, // alias explícito
		"PostToolUse":      ObservationPostToolUse,
		"session_start":    ObservationSessionStart,
		"PreCompact":       ObservationPreCompact,
		"PostCompaction":   ObservationPostCompaction,
		"Stop":             ObservationStop,
		"SESSION-END":      ObservationSessionEnd,
	}
	for in, want := range cases {
		got, err := ParseObservationKind(in)
		if err != nil {
			t.Errorf("ParseObservationKind(%q) erro inesperado: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseObservationKind(%q) = %q, esperado %q", in, got, want)
		}
	}
}

func TestParseObservationKind_Desconhecido(t *testing.T) {
	invalidos := []string{"", "bogus", "session", "user-prompt-submit", "pre-tool", "unknown-event"}
	for _, s := range invalidos {
		_, err := ParseObservationKind(s)
		if err == nil {
			t.Errorf("ParseObservationKind(%q) deveria recusar", s)
			continue
		}
		if !errors.Is(err, ErrMalformedRecord) {
			t.Errorf("ParseObservationKind(%q) erro sem ErrMalformedRecord: %v", s, err)
		}
	}
}
