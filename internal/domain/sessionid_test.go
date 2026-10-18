package domain

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

// Valor de referência calculado uma vez com a biblioteca Python `uuid.uuid5(uuid.NAMESPACE_OID, "session-alpha-001")`,
// fora do pacote Go, para não depender só da própria implementação.
const sessionAlphaV5 = "9f907ad5-58b7-54a6-beee-a55f0fe0cb7b"

func TestSessionIDFromNativeKeepsV4UUID(t *testing.T) {
	const in = "6f1c2e0a-3b4d-4e5f-8a9b-0c1d2e3f4a5b"
	got, err := SessionIDFromNative(in)
	if err != nil {
		t.Fatalf("SessionIDFromNative(%q): %v", in, err)
	}
	if got.String() != in {
		t.Fatalf("UUID v4 alterado: got %q, esperado %q", got.String(), in)
	}
	if v := uuid.UUID(got).Version(); v != 4 {
		t.Fatalf("versão = %d, esperada 4 (UUID nativo mantido)", v)
	}
}

func TestSessionIDFromNativeNormalizesUppercaseUUID(t *testing.T) {
	// Decisão documentada no comentário da função: forma canônica minúscula.
	const in = "6F1C2E0A-3B4D-4E5F-8A9B-0C1D2E3F4A5B"
	const want = "6f1c2e0a-3b4d-4e5f-8a9b-0c1d2e3f4a5b"
	got, err := SessionIDFromNative(in)
	if err != nil {
		t.Fatalf("SessionIDFromNative(%q): %v", in, err)
	}
	if got.String() != want {
		t.Fatalf("got %q, esperado %q", got.String(), want)
	}
}

func TestSessionIDFromNativeNonUUIDIsV5InOIDNamespace(t *testing.T) {
	got, err := SessionIDFromNative("session-alpha-001")
	if err != nil {
		t.Fatalf("SessionIDFromNative: %v", err)
	}
	if got.String() != sessionAlphaV5 {
		t.Fatalf("got %q, esperado (referência Python) %q", got.String(), sessionAlphaV5)
	}
	u := uuid.UUID(got)
	if v := u.Version(); v != 5 {
		t.Fatalf("versão = %d, esperada 5", v)
	}
	if u.Variant() != uuid.RFC4122 {
		t.Fatalf("variante = %v, esperada RFC 4122", u.Variant())
	}
}

func TestSessionIDFromNativeIsDeterministic(t *testing.T) {
	const in = "claude-code-session-xyz"
	a, err := SessionIDFromNative(in)
	if err != nil {
		t.Fatalf("primeira chamada: %v", err)
	}
	b, err := SessionIDFromNative(in)
	if err != nil {
		t.Fatalf("segunda chamada: %v", err)
	}
	if a != b {
		t.Fatalf("resultados divergiram: %s != %s", a.String(), b.String())
	}
	u := uuid.UUID(a)
	if u.Version() != 5 || u.Variant() != uuid.RFC4122 {
		t.Fatalf("versão/variante = %d/%v, esperadas 5/RFC4122", u.Version(), u.Variant())
	}
}

func TestSessionIDFromNativeDifferentStringsDiffer(t *testing.T) {
	a, _ := SessionIDFromNative("session-a")
	b, _ := SessionIDFromNative("session-b")
	if a == b {
		t.Fatalf("strings diferentes geraram o mesmo identificador: %s", a.String())
	}
}

func TestSessionIDFromNativeEmptyIsError(t *testing.T) {
	for _, in := range []string{"", "   "} {
		got, err := SessionIDFromNative(in)
		if err == nil {
			t.Fatalf("SessionIDFromNative(%q) sem erro, esperado erro", in)
		}
		if !errors.Is(err, ErrInvalidID) {
			t.Fatalf("erro = %v, esperado ErrInvalidID", err)
		}
		if got != (SessionID{}) {
			t.Fatalf("entrada vazia devolveu identificador %s junto com erro", got.String())
		}
	}
}
