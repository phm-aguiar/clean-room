package domain

import (
	"fmt"
	"strings"
)

// HookEvent é o evento canônico de ciclo de vida recebido dos hooks dos agentes
// (LEI §3.1). Cada valor usa a forma canônica em kebab-case. O conjunto é fechado: só os
// valores abaixo são válidos. Nome desconhecido é normalizado para HookOther por ParseHookEvent.
type HookEvent string

// Eventos canônicos (LEI §3.1).
const (
	HookSessionStart   HookEvent = "session-start"
	HookUserPrompt     HookEvent = "user-prompt"
	HookPreToolUse     HookEvent = "pre-tool-use"
	HookPostToolUse    HookEvent = "post-tool-use"
	HookPreCompact     HookEvent = "pre-compact"
	HookPostCompaction HookEvent = "post-compaction"
	HookNotification   HookEvent = "notification"
	HookStop           HookEvent = "stop"
	HookSessionEnd     HookEvent = "session-end"
	HookSubagentStart  HookEvent = "subagent-start"
	HookSubagentStop   HookEvent = "subagent-stop"
	HookOther          HookEvent = "other"
)

// hookEventByName mapeia o nome normalizado (sem `-`, `_` e em minúsculas) para o evento
// canônico. Cobre as formas kebab-case, snake_case, PascalCase e camelCase de cada canônico,
// além dos aliases de harnesses listados em LEI §3.1.
var hookEventByName = map[string]HookEvent{
	// Início de sessão.
	"sessionstart": HookSessionStart,
	// Prompt do usuário.
	"userprompt":         HookUserPrompt,
	"userpromptsubmit":   HookUserPrompt,
	"beforesubmitprompt": HookUserPrompt,
	// Pré-ferramenta.
	"pretooluse": HookPreToolUse,
	"beforetool": HookPreToolUse,
	// Pós-ferramenta.
	"posttooluse":        HookPostToolUse,
	"posttoolusefailure": HookPostToolUse,
	"aftertool":          HookPostToolUse,
	// Pré-compactação.
	"precompact":  HookPreCompact,
	"precompress": HookPreCompact,
	// Pós-compactação.
	"postcompaction": HookPostCompaction,
	// Notificação.
	"notification": HookNotification,
	// Parada.
	"stop": HookStop,
	// Fim de sessão.
	"sessionend": HookSessionEnd,
	// Início de subagente.
	"subagentstart": HookSubagentStart,
	// Fim de subagente.
	"subagentstop": HookSubagentStop,
	"subagentend":  HookSubagentStop,
	// Outro.
	"other": HookOther,
}

// ParseHookEvent converte o nome recebido do hook em HookEvent. Aceita a forma canônica,
// kebab-case, snake_case, PascalCase, camelCase e os aliases de harnesses da LEI §3.1. A
// comparação ignora `-`, `_` e caixa. Nome desconhecido vira HookOther, como manda a LEI §3.1
// ("Outro: qualquer outro nome"). Nome vazio ou só com espaços é recusado com erro que
// encapsula ErrMalformedRecord, sem pânico.
func ParseHookEvent(s string) (HookEvent, error) {
	normalized := normalizeHookEventName(strings.TrimSpace(s))
	if normalized == "" {
		return "", fmt.Errorf("evento de hook vazio: %w", ErrMalformedRecord)
	}
	if e, ok := hookEventByName[normalized]; ok {
		return e, nil
	}
	return HookOther, nil
}

// normalizeHookEventName remove `-` e `_` e passa para minúsculas.
func normalizeHookEventName(s string) string {
	return strings.ToLower(strings.NewReplacer("-", "", "_", "").Replace(s))
}

// Valid informa se e é um dos doze eventos canônicos. Os métodos abaixo devolvem zero para
// valor inválido, então o chamador deve obter o HookEvent por ParseHookEvent.
func (e HookEvent) Valid() bool {
	_, ok := hookEventImportance[e]
	return ok
}

// hookEventImportance é a importância de cada evento canônico (LEI §3.2).
var hookEventImportance = map[HookEvent]int{
	HookSessionStart:   7, // LEI §3.2: início e fim de sessão.
	HookSessionEnd:     7, // LEI §3.2: início e fim de sessão.
	HookUserPrompt:     8, // LEI §3.2: prompt do usuário.
	HookPreToolUse:     5, // LEI §3.2: pré e pós-ferramenta.
	HookPostToolUse:    5, // LEI §3.2: pré e pós-ferramenta.
	HookStop:           6, // LEI §3.2: parada, pré-compactação, pós-compactação.
	HookPreCompact:     6, // LEI §3.2: parada, pré-compactação, pós-compactação.
	HookPostCompaction: 6, // LEI §3.2: parada, pré-compactação, pós-compactação.
	HookNotification:   3, // LEI §3.2: notificação, outro, início e fim de subagente.
	HookOther:          3, // LEI §3.2: notificação, outro, início e fim de subagente.
	HookSubagentStart:  3, // LEI §3.2: notificação, outro, início e fim de subagente.
	HookSubagentStop:   3, // LEI §3.2: notificação, outro, início e fim de subagente.
}

// Importance devolve a importância da observação do evento, entre MinImportance e
// MaxImportance (LEI §3.2). Valor inválido devolve 0.
func (e HookEvent) Importance() int {
	return hookEventImportance[e]
}

// KiB é um kibibyte, usado nos limites de corpo (LEI §3.6).
const KiB = 1024

// hookEventShortBodyLimit é o teto de "2 KB" de LEI §3.6, em bytes UTF-8 seguros. Vale 2000 e
// não 2048: LEI §5 e MF "What is kept" fixam 2000 bytes para o excerto, e a decisão L13 da
// consolidação da fase 0 (notes/phase0-findings.md) adota 2000 para todos os "2 KB".
const hookEventShortBodyLimit = 2000

// hookEventBodyLimit é o teto de corpo durável de cada evento, em bytes, aplicado depois da
// sanitização (LEI §3.6).
var hookEventBodyLimit = map[HookEvent]int{
	HookUserPrompt:     16 * KiB,                // LEI §3.6: prompt do usuário e pós-compactação.
	HookPostCompaction: 16 * KiB,                // LEI §3.6: prompt do usuário e pós-compactação.
	HookNotification:   hookEventShortBodyLimit, // LEI §3.6: notificação, pré e pós-ferramenta, parada, outro.
	HookPreToolUse:     hookEventShortBodyLimit, // LEI §3.6: notificação, pré e pós-ferramenta, parada, outro.
	HookPostToolUse:    hookEventShortBodyLimit, // LEI §3.6: notificação, pré e pós-ferramenta, parada, outro.
	HookStop:           hookEventShortBodyLimit, // LEI §3.6: notificação, pré e pós-ferramenta, parada, outro.
	HookOther:          hookEventShortBodyLimit, // LEI §3.6: notificação, pré e pós-ferramenta, parada, outro.
	HookSessionStart:   16 * KiB,                // LEI §3.6: demais (início e fim de sessão, pré-compactação, subagentes).
	HookSessionEnd:     16 * KiB,                // LEI §3.6: demais (início e fim de sessão, pré-compactação, subagentes).
	HookPreCompact:     16 * KiB,                // LEI §3.6: demais (início e fim de sessão, pré-compactação, subagentes).
	HookSubagentStart:  16 * KiB,                // LEI §3.6: demais (início e fim de sessão, pré-compactação, subagentes).
	HookSubagentStop:   16 * KiB,                // LEI §3.6: demais (início e fim de sessão, pré-compactação, subagentes).
}

// BodyLimit devolve o teto do corpo durável do evento, em bytes (LEI §3.6). Valor inválido
// devolve 0.
func (e HookEvent) BodyLimit() int {
	return hookEventBodyLimit[e]
}

// ObservationKind devolve o tipo de observação em que o evento é armazenado (LEI §3.1).
// Eventos de subagente e o evento "outro" usam ObservationOther. Os demais usam o tipo de
// mesmo nome. Valor inválido devolve zero value.
func (e HookEvent) ObservationKind() ObservationKind {
	switch e {
	case HookSessionStart:
		return ObservationSessionStart
	case HookUserPrompt:
		return ObservationUserPrompt
	case HookPreToolUse:
		return ObservationPreToolUse
	case HookPostToolUse:
		return ObservationPostToolUse
	case HookPreCompact:
		return ObservationPreCompact
	case HookPostCompaction:
		return ObservationPostCompaction
	case HookNotification:
		return ObservationNotification
	case HookStop:
		return ObservationStop
	case HookSessionEnd:
		return ObservationSessionEnd
	case HookSubagentStart, HookSubagentStop, HookOther:
		return ObservationOther
	}
	return ""
}
