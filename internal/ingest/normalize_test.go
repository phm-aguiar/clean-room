package ingest

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// testNow é o relógio fixo dos testes. Normalize não lê o relógio do sistema.
var testNow = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

const fixtureDir = "testdata/claude-code"

// fixture é um caso de teste em JSON: a consulta, o corpo exato que o hook envia e o resultado
// esperado. Campo ausente em expect não é conferido. occurred_at "now" significa o relógio de teste.
type fixture struct {
	Description string `json:"description"`
	Input       struct {
		Agent     string `json:"agent"`
		Event     string `json:"event"`
		Cwd       string `json:"cwd"`
		SessionID string `json:"session_id"`
	} `json:"input"`
	Body   json.RawMessage `json:"body"`
	Expect expectation     `json:"expect"`
}

type expectation struct {
	Hook          *string  `json:"hook"`
	Agent         *string  `json:"agent"`
	HasSession    *bool    `json:"has_session"`
	SessionID     *string  `json:"session_id"`
	Cwd           *string  `json:"cwd"`
	Title         *string  `json:"title"`
	Body          *string  `json:"body"`
	OccurredAt    *string  `json:"occurred_at"`
	IngestKey     *string  `json:"ingest_key"`
	IsSubagent    *bool    `json:"is_subagent"`
	ToolName      *string  `json:"tool_name"`
	ToolFamily    *string  `json:"tool_family"`
	ToolCallID    *string  `json:"tool_call_id"`
	AbsentInText  []string `json:"absent_in_text"`
	AbsentInEvent []string `json:"absent_in_event"`
}

// requiredFixtures são os casos mínimos da tabela de hook-contract.md §11 exigidos pela T023.
var requiredFixtures = []string{
	"session_start",
	"user_prompt_submit",
	"pre_tool_use_file",
	"pre_tool_use_shell",
	"post_tool_use_success",
	"stop_with_last_assistant_message",
	"subagent_start",
	"cwd_in_body_vs_query",
	"occurred_at_future",
	"ingest_key_invalid",
	"empty_object",
}

func loadFixture(t *testing.T, path string) fixture {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ler fixture: %v", err)
	}
	var f fixture
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("decodificar fixture %s: %v", path, err)
	}
	return f
}

func TestFixtures(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join(fixtureDir, "*.json"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(paths) == 0 {
		t.Fatalf("nenhuma fixture em %s", fixtureDir)
	}
	for _, path := range paths {
		name := strings.TrimSuffix(filepath.Base(path), ".json")
		t.Run(name, func(t *testing.T) {
			f := loadFixture(t, path)
			ev, err := Normalize(Input{
				Agent:     f.Input.Agent,
				Event:     f.Input.Event,
				Cwd:       f.Input.Cwd,
				SessionID: f.Input.SessionID,
				Now:       testNow,
			}, f.Body)
			if err != nil {
				t.Fatalf("Normalize: %v", err)
			}
			checkExpectation(t, ev, f.Expect)
		})
	}
}

func checkExpectation(t *testing.T, ev Event, e expectation) {
	t.Helper()
	if e.Hook != nil && string(ev.Hook) != *e.Hook {
		t.Errorf("Hook = %q, want %q", ev.Hook, *e.Hook)
	}
	if e.Agent != nil && ev.Agent != *e.Agent {
		t.Errorf("Agent = %q, want %q", ev.Agent, *e.Agent)
	}
	if e.HasSession != nil && ev.HasSession != *e.HasSession {
		t.Errorf("HasSession = %v, want %v", ev.HasSession, *e.HasSession)
	}
	if e.SessionID != nil && ev.Session.String() != *e.SessionID {
		t.Errorf("Session = %q, want %q", ev.Session.String(), *e.SessionID)
	}
	if e.Cwd != nil && ev.Cwd != *e.Cwd {
		t.Errorf("Cwd = %q, want %q", ev.Cwd, *e.Cwd)
	}
	if e.Title != nil && ev.Title != *e.Title {
		t.Errorf("Title = %q, want %q", ev.Title, *e.Title)
	}
	if e.Body != nil && ev.Body != *e.Body {
		t.Errorf("Body = %q, want %q", ev.Body, *e.Body)
	}
	if e.OccurredAt != nil {
		want := testNow
		if *e.OccurredAt != "now" {
			parsed, err := time.Parse(time.RFC3339, *e.OccurredAt)
			if err != nil {
				t.Fatalf("occurred_at esperado inválido na fixture: %v", err)
			}
			want = parsed
		}
		if !ev.OccurredAt.Equal(want) {
			t.Errorf("OccurredAt = %s, want %s", ev.OccurredAt.Format(time.RFC3339Nano), want.Format(time.RFC3339Nano))
		}
	}
	if e.IngestKey != nil && ev.IngestKey != *e.IngestKey {
		t.Errorf("IngestKey = %q, want %q", ev.IngestKey, *e.IngestKey)
	}
	if e.IsSubagent != nil && ev.IsSubagent != *e.IsSubagent {
		t.Errorf("IsSubagent = %v, want %v", ev.IsSubagent, *e.IsSubagent)
	}
	if e.ToolName != nil && ev.ToolName != *e.ToolName {
		t.Errorf("ToolName = %q, want %q", ev.ToolName, *e.ToolName)
	}
	if e.ToolFamily != nil && string(ev.ToolFamily) != *e.ToolFamily {
		t.Errorf("ToolFamily = %q, want %q", ev.ToolFamily, *e.ToolFamily)
	}
	if e.ToolCallID != nil && ev.ToolCallID != *e.ToolCallID {
		t.Errorf("ToolCallID = %q, want %q", ev.ToolCallID, *e.ToolCallID)
	}
	for _, s := range e.AbsentInText {
		if strings.Contains(ev.Title, s) || strings.Contains(ev.Body, s) {
			t.Errorf("texto de título ou corpo contém %q (argumento vazou)", s)
		}
	}
	if len(e.AbsentInEvent) > 0 {
		dump, err := json.Marshal(ev)
		if err != nil {
			t.Fatalf("marshal do evento: %v", err)
		}
		for _, s := range e.AbsentInEvent {
			if strings.Contains(string(dump), s) {
				t.Errorf("evento contém %q em algum campo", s)
			}
		}
	}
}

func TestFixtureCoverage(t *testing.T) {
	for _, name := range requiredFixtures {
		path := filepath.Join(fixtureDir, name+".json")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("fixture obrigatória ausente: %s (%v)", path, err)
		}
	}
}

func TestNormalizeRejectsOnlyMalformedInput(t *testing.T) {
	cases := []struct {
		name  string
		event string
		body  string
	}{
		{"evento vazio", "", `{}`},
		{"evento só com espaços", "   ", `{}`},
		{"corpo não é JSON", "stop", `não é json`},
		{"corpo é lista", "stop", `[]`},
		{"corpo é null", "stop", `null`},
		{"corpo vazio", "stop", ``},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Normalize(Input{Event: c.event, Now: testNow}, []byte(c.body))
			if err == nil {
				t.Fatal("Normalize aceitou entrada malformada")
			}
			if !errors.Is(err, domain.ErrMalformedRecord) {
				t.Errorf("erro sem ErrMalformedRecord: %v", err)
			}
		})
	}
}

func TestOccurredAtBoundaries(t *testing.T) {
	cases := []struct {
		name  string
		value string // JSON literal do campo occurred_at
		want  time.Time
	}{
		{"exatamente cinco minutos à frente vale", `"2026-10-13T12:05:00Z"`, time.Date(2026, 10, 13, 12, 5, 0, 0, time.UTC)},
		{"um nanossegundo além de cinco minutos é descartado", `"2026-10-13T12:05:00.000000001Z"`, testNow},
		{"epoca exata nao e positiva", `"1970-01-01T00:00:00Z"`, testNow},
		{"um nanossegundo após a época é positivo", `"1970-01-01T00:00:00.000000001Z"`, time.Unix(0, 1)},
		{"texto que não é RFC 3339 vira agora", `"ontem"`, testNow},
		{"número não é aceito como instante, vira agora", `1700000000`, testNow},
		{"ausente vira agora", "", testNow},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body := `{}`
			if c.value != "" {
				body = `{"occurred_at": ` + c.value + `}`
			}
			ev, err := Normalize(Input{Event: "user-prompt", Now: testNow}, []byte(body))
			if err != nil {
				t.Fatalf("Normalize: %v", err)
			}
			if !ev.OccurredAt.Equal(c.want) {
				t.Errorf("OccurredAt = %s, want %s", ev.OccurredAt.Format(time.RFC3339Nano), c.want.Format(time.RFC3339Nano))
			}
		})
	}
}

func TestIngestKeyBoundaries(t *testing.T) {
	cases := []struct {
		key  string
		want bool
	}{
		{"a", true},
		{strings.Repeat("a", 64), true},
		{strings.Repeat("a", 65), false},
		{"", false},
		{"A-z_09", true},
		{"a b", false},
		{"a.b", false},
		{"chave-é", false},
		{" a", false},
	}
	for _, c := range cases {
		if got := validIngestKey(c.key); got != c.want {
			t.Errorf("validIngestKey(%q) = %v, want %v", c.key, got, c.want)
		}
	}
}

func TestToolFamilyTable(t *testing.T) {
	cases := map[string]ToolFamily{
		"Read":         ToolFamilyFile,
		"Edit":         ToolFamilyFile,
		"Write":        ToolFamilyFile,
		"MultiEdit":    ToolFamilyFile,
		"NotebookEdit": ToolFamilyFile,
		"Grep":         ToolFamilySearchList,
		"Glob":         ToolFamilySearchList,
		"LS":           ToolFamilySearchList,
		"Bash":         ToolFamilyNonFile,
		"Task":         ToolFamilyNonFile,
		"mcp__x__y":    ToolFamilyUnknown,
		"read":         ToolFamilyUnknown, // nomes são sensíveis a caixa
		"":             ToolFamilyUnknown,
	}
	for name, want := range cases {
		if got := familyOf(name); got != want {
			t.Errorf("familyOf(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestSubagentFlagNeedsNonEmptyValue(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"agent_id vazio não marca", `{"agent_id": ""}`, false},
		{"subagentType só com espaços não marca", `{"subagentType": "   "}`, false},
		{"agent_id não string não marca", `{"agent_id": 7}`, false},
		{"agent_id não vazio marca", `{"agent_id": "a1"}`, true},
		{"subagentType não vazio marca", `{"subagentType": "Explore"}`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ev, err := Normalize(Input{Event: "pre-tool-use", Now: testNow}, []byte(c.body))
			if err != nil {
				t.Fatalf("Normalize: %v", err)
			}
			if ev.IsSubagent != c.want {
				t.Errorf("IsSubagent = %v, want %v", ev.IsSubagent, c.want)
			}
		})
	}
}

func TestSessionFallsBackToQueryOnlyWhenBodyHasNone(t *testing.T) {
	ev, err := Normalize(Input{Event: "stop", SessionID: "sess-query", Now: testNow},
		[]byte(`{"session_id": "   "}`))
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	want, err := domain.SessionIDFromNative("sess-query")
	if err != nil {
		t.Fatalf("SessionIDFromNative: %v", err)
	}
	if !ev.HasSession || ev.Session != want {
		t.Errorf("sessão = %v (has %v), want %v", ev.Session, ev.HasSession, want)
	}
}

func TestToolInputIsKeptRawAndNeverInText(t *testing.T) {
	body := `{"tool_name": "Write", "tool_use_id": "t1", "tool_input": {"file_path": "/x/y.go", "content": "segredo"}}`
	ev, err := Normalize(Input{Event: "pre-tool-use", Now: testNow}, []byte(body))
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if string(ev.ToolInput) != `{"file_path": "/x/y.go", "content": "segredo"}` {
		t.Errorf("ToolInput = %s, want o valor bruto de tool_input", ev.ToolInput)
	}
	if strings.Contains(ev.Title, "y.go") || strings.Contains(ev.Body, "segredo") || strings.Contains(ev.Body, "y.go") {
		t.Errorf("argumento vazou para título ou corpo: %q / %q", ev.Title, ev.Body)
	}
}

func TestToolCallIDRules(t *testing.T) {
	cases := []struct {
		name string
		id   string
		want string
	}{
		{"id comum", "toolu_01", "toolu_01"},
		{"vazio", "", ""},
		{"só espaços", "   ", ""},
		{"caractere de controle", "a\nb", ""},
		{"512 bytes vale", strings.Repeat("x", 512), strings.Repeat("x", 512)},
		{"513 bytes é inválido", strings.Repeat("x", 513), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := validToolCallID(c.id); got != c.want {
				t.Errorf("validToolCallID = %q, want %q", got, c.want)
			}
		})
	}
}
