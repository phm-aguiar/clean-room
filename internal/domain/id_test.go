package domain

import (
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"
)

func TestIDRoundTripStringParse(t *testing.T) {
	id, err := NewID()
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	s := id.String()
	if len(s) != 36 {
		t.Fatalf("String() = %q, esperados 36 caracteres", s)
	}
	back, err := ParseID(s)
	if err != nil {
		t.Fatalf("ParseID(%q): %v", s, err)
	}
	if back != id {
		t.Fatalf("round-trip divergiu: %v != %v", back, id)
	}
	if back.String() != s {
		t.Fatalf("segundo String() = %q, esperado %q", back.String(), s)
	}
}

func TestIDGeneratedIsVersion7(t *testing.T) {
	id, err := NewID()
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	u := uuid.UUID(id)
	if v := u.Version(); v != 7 {
		t.Fatalf("versão = %d, esperada 7", v)
	}
	if u.Variant() != uuid.RFC4122 {
		t.Fatalf("variante = %v, esperada RFC4122", u.Variant())
	}
}

func TestIDParseAcceptsUpperCaseAndNormalizesToLower(t *testing.T) {
	const upper = "0190F3A2-1B2C-7D4E-8F50-A1B2C3D4E5F6"
	id, err := ParseID(upper)
	if err != nil {
		t.Fatalf("ParseID(%q): %v", upper, err)
	}
	if got, want := id.String(), "0190f3a2-1b2c-7d4e-8f50-a1b2c3d4e5f6"; got != want {
		t.Fatalf("String() = %q, esperado %q", got, want)
	}
}

func TestIDParseRejectsInvalidText(t *testing.T) {
	cases := map[string]string{
		"vazio":               "",
		"lixo":                "not-a-uuid",
		"sem hífens":          "0190f3a21b2c7d4e8f50a1b2c3d4e5f6",
		"com chaves":          "{0190f3a2-1b2c-7d4e-8f50-a1b2c3d4e5f6}",
		"prefixo urn":         "urn:uuid:0190f3a2-1b2c-7d4e-8f50-a1b2c3d4e5f6",
		"espaço nas bordas":   " 0190f3a2-1b2c-7d4e-8f50-a1b2c3d4e5f6",
		"hífen fora de lugar": "0190f3a21-b2c-7d4e-8f50-a1b2c3d4e5f6",
		"dois-pontos":         "0190f3a2:1b2c:7d4e:8f50:a1b2c3d4e5f6",
		"não hexadecimal":     "0190f3a2-1b2c-7d4e-8f50-a1b2c3d4e5fg",
		"longo demais":        "0190f3a2-1b2c-7d4e-8f50-a1b2c3d4e5f6a",
	}
	for name, s := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseID(s)
			if err == nil {
				t.Fatalf("ParseID(%q) aceitou entrada inválida", s)
			}
			if !errors.Is(err, ErrInvalidID) {
				t.Fatalf("erro = %v, esperado ErrInvalidID", err)
			}
		})
	}
}

func TestIDParseIDBytesRequiresExactly16(t *testing.T) {
	for _, n := range []int{0, 15, 17, 32} {
		_, err := ParseIDBytes(make([]byte, n))
		if err == nil {
			t.Fatalf("ParseIDBytes com %d bytes foi aceito", n)
		}
		if !errors.Is(err, ErrInvalidID) {
			t.Fatalf("%d bytes: erro = %v, esperado ErrInvalidID", n, err)
		}
	}

	raw := make([]byte, 16)
	for i := range raw {
		raw[i] = byte(i + 1)
	}
	id, err := ParseIDBytes(raw)
	if err != nil {
		t.Fatalf("ParseIDBytes com 16 bytes: %v", err)
	}
	if got := id.Bytes(); !reflect.DeepEqual(got, raw) {
		t.Fatalf("Bytes() = %v, esperado %v", got, raw)
	}
}

func TestIDBytesReturnsCopy(t *testing.T) {
	id, err := NewID()
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	before := id.String()
	b := id.Bytes()
	for i := range b {
		b[i] = 0
	}
	if id.String() != before {
		t.Fatalf("alterar Bytes() mudou o ID: %s -> %s", before, id.String())
	}
}

func TestIDTypedIDsAreDistinct(t *testing.T) {
	// Tipos definidos sobre ID não são alias: o compilador não converte um no outro.
	// A conversão precisa ser explícita, por exemplo ProjectID(id). Um teste de
	// compilação negativo não é necessário; aqui basta checar que os tipos são distintos.
	types := []reflect.Type{
		reflect.TypeOf(WorkspaceID{}),
		reflect.TypeOf(ProjectID{}),
		reflect.TypeOf(PageID{}),
		reflect.TypeOf(SessionID{}),
		reflect.TypeOf(ObservationID{}),
	}
	seen := make(map[reflect.Type]bool, len(types))
	for _, typ := range types {
		if seen[typ] {
			t.Fatalf("tipo %v repetido", typ)
		}
		seen[typ] = true
	}

	id, err := NewID()
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	w := WorkspaceID(id)
	p := ProjectID(id)
	if ID(w) != id || ID(p) != id {
		t.Fatalf("conversão explícita perdeu o valor")
	}
	if w.String() != id.String() || p.Bytes() == nil {
		t.Fatalf("métodos dos tipos derivados divergem do ID base")
	}
}
