package domain

import (
	"errors"
	"testing"
)

// hookEventCase liga cada evento canônico às formas de nome aceitas (LEI §3.1), à importância
// (LEI §3.2), ao limite de corpo (LEI §3.6) e ao tipo de observação (LEI §3.1).
type hookEventCase struct {
	canonical   HookEvent
	names       []string // formas aceitas: canônica, kebab, snake, Pascal, camel e aliases
	importance  int
	bodyLimit   int
	observation ObservationKind
}

var hookEventCases = []hookEventCase{
	{
		// LEI §3.1 (início de sessão); §3.2 importância 7; §3.6 demais, 16 KiB.
		canonical:   HookSessionStart,
		names:       []string{"session-start", "session_start", "SessionStart", "sessionStart"},
		importance:  7,
		bodyLimit:   16 * KiB,
		observation: ObservationSessionStart,
	},
	{
		// LEI §3.1 (prompt do usuário); §3.2 importância 8; §3.6 prompt, 16 KiB.
		canonical:   HookUserPrompt,
		names:       []string{"user-prompt", "user_prompt", "UserPrompt", "userPrompt", "UserPromptSubmit", "user-prompt-submit", "beforeSubmitPrompt"},
		importance:  8,
		bodyLimit:   16 * KiB,
		observation: ObservationUserPrompt,
	},
	{
		// LEI §3.1 (pré-ferramenta); §3.2 importância 5; §3.6 pré e pós-ferramenta, 2 KB.
		canonical:   HookPreToolUse,
		names:       []string{"pre-tool-use", "pre_tool_use", "PreToolUse", "preToolUse", "BeforeTool"},
		importance:  5,
		bodyLimit:   hookEventShortBodyLimit,
		observation: ObservationPreToolUse,
	},
	{
		// LEI §3.1 (pós-ferramenta); §3.2 importância 5; §3.6 pré e pós-ferramenta, 2 KB.
		canonical:   HookPostToolUse,
		names:       []string{"post-tool-use", "post_tool_use", "PostToolUse", "postToolUse", "postToolUseFailure", "AfterTool"},
		importance:  5,
		bodyLimit:   hookEventShortBodyLimit,
		observation: ObservationPostToolUse,
	},
	{
		// LEI §3.1 (pré-compactação); §3.2 importância 6; §3.6 demais, 16 KiB.
		canonical:   HookPreCompact,
		names:       []string{"pre-compact", "pre_compact", "PreCompact", "preCompact", "PreCompress"},
		importance:  6,
		bodyLimit:   16 * KiB,
		observation: ObservationPreCompact,
	},
	{
		// LEI §3.1 (pós-compactação); §3.2 importância 6; §3.6 prompt e pós-compactação, 16 KiB.
		canonical:   HookPostCompaction,
		names:       []string{"post-compaction", "post_compaction", "PostCompaction", "postCompaction"},
		importance:  6,
		bodyLimit:   16 * KiB,
		observation: ObservationPostCompaction,
	},
	{
		// LEI §3.1 (notificação); §3.2 importância 3; §3.6 notificação, 2 KB.
		canonical:   HookNotification,
		names:       []string{"notification", "Notification"},
		importance:  3,
		bodyLimit:   hookEventShortBodyLimit,
		observation: ObservationNotification,
	},
	{
		// LEI §3.1 (parada, Stop); §3.2 importância 6; §3.6 parada, 2 KB.
		canonical:   HookStop,
		names:       []string{"stop", "Stop"},
		importance:  6,
		bodyLimit:   hookEventShortBodyLimit,
		observation: ObservationStop,
	},
	{
		// LEI §3.1 (fim de sessão); §3.2 importância 7; §3.6 demais, 16 KiB.
		canonical:   HookSessionEnd,
		names:       []string{"session-end", "session_end", "SessionEnd", "sessionEnd"},
		importance:  7,
		bodyLimit:   16 * KiB,
		observation: ObservationSessionEnd,
	},
	{
		// LEI §3.1 (início de subagente, tipo "outro"); §3.2 importância 3; §3.6 demais, 16 KiB.
		canonical:   HookSubagentStart,
		names:       []string{"subagent-start", "subagent_start", "SubagentStart", "subagentStart"},
		importance:  3,
		bodyLimit:   16 * KiB,
		observation: ObservationOther,
	},
	{
		// LEI §3.1 (fim de subagente, tipo "outro"); §3.2 importância 3; §3.6 demais, 16 KiB.
		canonical:   HookSubagentStop,
		names:       []string{"subagent-stop", "subagent_stop", "SubagentStop", "subagentStop", "subagent-end", "SubagentEnd"},
		importance:  3,
		bodyLimit:   16 * KiB,
		observation: ObservationOther,
	},
	{
		// LEI §3.1 (outro); §3.2 importância 3; §3.6 outro, 2 KB.
		canonical:   HookOther,
		names:       []string{"other", "Other"},
		importance:  3,
		bodyLimit:   hookEventShortBodyLimit,
		observation: ObservationOther,
	},
}

func TestParseHookEvent_FormasAceitas(t *testing.T) {
	for _, c := range hookEventCases {
		for _, name := range c.names {
			got, err := ParseHookEvent(name)
			if err != nil {
				t.Errorf("ParseHookEvent(%q) erro inesperado: %v", name, err)
				continue
			}
			if got != c.canonical {
				t.Errorf("ParseHookEvent(%q) = %q, esperado %q", name, got, c.canonical)
			}
		}
	}
}

func TestHookEvent_ImportanciaELimite(t *testing.T) {
	for _, c := range hookEventCases {
		if !c.canonical.Valid() {
			t.Errorf("%q deveria ser válido", c.canonical)
		}
		if got := c.canonical.Importance(); got != c.importance {
			t.Errorf("%q.Importance() = %d, esperado %d", c.canonical, got, c.importance)
		}
		if got := c.canonical.Importance(); got < MinImportance || got > MaxImportance {
			t.Errorf("%q.Importance() = %d fora de %d a %d", c.canonical, got, MinImportance, MaxImportance)
		}
		if got := c.canonical.BodyLimit(); got != c.bodyLimit {
			t.Errorf("%q.BodyLimit() = %d, esperado %d", c.canonical, got, c.bodyLimit)
		}
		if got := c.canonical.ObservationKind(); got != c.observation {
			t.Errorf("%q.ObservationKind() = %q, esperado %q", c.canonical, got, c.observation)
		}
	}
}

func TestHookEvent_TipoDeObservacaoCanonico(t *testing.T) {
	// Todo tipo devolvido precisa existir no conjunto canônico de DV §5.1, senão NewObservation
	// recusa o evento.
	for _, c := range hookEventCases {
		if _, err := ParseObservationKind(string(c.observation)); err != nil {
			t.Errorf("%q.ObservationKind() = %q não é canônico: %v", c.canonical, c.observation, err)
		}
	}
}

func TestParseHookEvent_VazioDevolveErro(t *testing.T) {
	for _, name := range []string{"", "   "} {
		got, err := ParseHookEvent(name)
		if err == nil {
			t.Errorf("ParseHookEvent(%q) deveria recusar, devolveu %q", name, got)
			continue
		}
		if !errors.Is(err, ErrMalformedRecord) {
			t.Errorf("ParseHookEvent(%q) erro sem ErrMalformedRecord: %v", name, err)
		}
		if got != "" {
			t.Errorf("ParseHookEvent(%q) = %q, esperado zero value", name, got)
		}
	}
}

// LEI §3.1: "Outro | qualquer outro nome". Nome desconhecido não é erro; vira HookOther.
func TestParseHookEvent_DesconhecidoViraOutro(t *testing.T) {
	for _, name := range []string{"Unknown", "pre-tool", "session", "sessionStarted", "xyz"} {
		got, err := ParseHookEvent(name)
		if err != nil {
			t.Errorf("ParseHookEvent(%q) erro inesperado: %v", name, err)
			continue
		}
		if got != HookOther {
			t.Errorf("ParseHookEvent(%q) = %q, esperado %q", name, got, HookOther)
		}
	}
}

func TestHookEvent_InvalidoDevolveZero(t *testing.T) {
	var zero HookEvent
	if zero.Valid() {
		t.Error("zero value não deveria ser válido")
	}
	if zero.Importance() != 0 || zero.BodyLimit() != 0 || zero.ObservationKind() != "" {
		t.Error("zero value deveria devolver 0 e observação vazia")
	}
}
