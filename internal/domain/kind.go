package domain

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ErrInvalidPageKind é a causa de todo erro devolvido por ParsePageKind.
var ErrInvalidPageKind = errors.New("kind de página inválido")

// pageKindMaxRunes é o teto de caracteres de PageKind (DV §6.6).
const pageKindMaxRunes = 64

// PageKind é o `kind` do frontmatter de uma página (ADR-08, DV §6.6). É texto livre dentro das
// regras de ParsePageKind. Não é enum fechado: as constantes abaixo são só os valores que o
// derivador OKF reconhece (DV §14).
type PageKind string

// Valores de PageKind reconhecidos pela derivação OKF.
const (
	PageKindFact      PageKind = "fact"
	PageKindNote      PageKind = "note"
	PageKindProcedure PageKind = "procedure"
	PageKindDecision  PageKind = "decision"
)

// ParsePageKind aplica DV §6.6: remove espaços das pontas, depois recusa valor com mais de 64
// caracteres ou com caractere de controle. Qualquer outro texto é aceito.
// Entrada vazia (ou só espaços) devolve o zero value (PageKind("")) sem erro, e o zero value
// significa "kind ausente".
func ParsePageKind(s string) (PageKind, error) {
	t := strings.TrimSpace(s)
	if t == "" {
		return "", nil
	}
	if n := utf8.RuneCountInString(t); n > pageKindMaxRunes {
		return "", fmt.Errorf("%w: %d caracteres, máximo %d", ErrInvalidPageKind, n, pageKindMaxRunes)
	}
	for _, r := range t {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("%w: caractere de controle U+%04X", ErrInvalidPageKind, r)
		}
	}
	return PageKind(t), nil
}

// ObservationKind é o tipo de um evento de ciclo de vida (DV §5.1, ADR-09). A forma canônica
// é kebab-case e o conjunto é fechado.
type ObservationKind string

// Os dez tipos canônicos de observação.
const (
	ObservationSessionStart   ObservationKind = "session-start"
	ObservationUserPrompt     ObservationKind = "user-prompt"
	ObservationPreToolUse     ObservationKind = "pre-tool-use"
	ObservationPostToolUse    ObservationKind = "post-tool-use"
	ObservationPreCompact     ObservationKind = "pre-compact"
	ObservationPostCompaction ObservationKind = "post-compaction"
	ObservationNotification   ObservationKind = "notification"
	ObservationStop           ObservationKind = "stop"
	ObservationSessionEnd     ObservationKind = "session-end"
	ObservationOther          ObservationKind = "other"
)

// ParseObservationKind converte texto em ObservationKind (ADR-09):
//  1. Canônico exato é aceito.
//  2. Alias explícito: `UserPromptSubmit` vira `user-prompt`.
//  3. Senão, remove `-` e `_`, passa para minúsculas e compara com o canônico normalizado.
//
// Desconhecido devolve erro que encapsula ErrMalformedRecord.
func ParseObservationKind(s string) (ObservationKind, error) {
	if k, ok := observationKindExact(s); ok {
		return k, nil
	}
	if s == "UserPromptSubmit" {
		return ObservationUserPrompt, nil
	}
	norm := normalizeObservationKind(s)
	for _, k := range observationKinds {
		if normalizeObservationKind(string(k)) == norm {
			return k, nil
		}
	}
	return "", fmt.Errorf("observation kind %q: %w", s, ErrMalformedRecord)
}

// observationKinds lista os dez canônicos, na ordem de DV §5.1.
var observationKinds = []ObservationKind{
	ObservationSessionStart,
	ObservationUserPrompt,
	ObservationPreToolUse,
	ObservationPostToolUse,
	ObservationPreCompact,
	ObservationPostCompaction,
	ObservationNotification,
	ObservationStop,
	ObservationSessionEnd,
	ObservationOther,
}

func observationKindExact(s string) (ObservationKind, bool) {
	for _, k := range observationKinds {
		if string(k) == s {
			return k, true
		}
	}
	return "", false
}

// normalizeObservationKind remove `-` e `_` e passa para minúsculas (ADR-09).
func normalizeObservationKind(s string) string {
	return strings.ToLower(strings.NewReplacer("-", "", "_", "").Replace(s))
}
