package synth

import (
	"reflect"
	"strings"
	"testing"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
	"github.com/phm-aguiar/ai-memory-go/internal/ingest"
)

func promptEv(text string) ingest.Event {
	return ingest.Event{Hook: domain.HookUserPrompt, Title: text, Body: text}
}

func toolEv(hook domain.HookEvent, name string, family ingest.ToolFamily) ingest.Event {
	return ingest.Event{Hook: hook, ToolName: name, ToolFamily: family}
}

func postTool(name string, family ingest.ToolFamily) ingest.Event {
	return toolEv(domain.HookPostToolUse, name, family)
}

func stopEv() ingest.Event { return ingest.Event{Hook: domain.HookStop} }

func endEv() ingest.Event { return ingest.Event{Hook: domain.HookSessionEnd} }

func repeat(r rune, n int) string { return strings.Repeat(string(r), n) }

// --- §9.1 Resumo -----------------------------------------------------------

func TestSummary_UmPrompt(t *testing.T) {
	h := Synthesize(Input{Mode: ModeSessionEnd, Observations: []ingest.Event{
		promptEv("Refactor the parser"),
	}})
	if want := "Session focused on: Refactor the parser"; h.Summary != want {
		t.Fatalf("Summary = %q, want %q", h.Summary, want)
	}
}

func TestSummary_VariosPrompts(t *testing.T) {
	h := Synthesize(Input{Mode: ModeSessionEnd, Observations: []ingest.Event{
		promptEv("first ask"),
		postTool("Read", ingest.ToolFamilyFile),
		promptEv("middle ask"),
		promptEv("last ask"),
	}})
	want := "Started: first ask\n\nLast: last ask"
	if h.Summary != want {
		t.Fatalf("Summary = %q, want %q", h.Summary, want)
	}
}

func TestSummary_NenhumPromptSessaoEncerrada(t *testing.T) {
	h := Synthesize(Input{Mode: ModeSessionEnd, Observations: []ingest.Event{
		ingest.Event{Hook: domain.HookSessionStart},
		postTool("Bash", ingest.ToolFamilyNonFile),
		stopEv(),
	}})
	if want := "Session ended; 3 observations recorded."; h.Summary != want {
		t.Fatalf("Summary = %q, want %q", h.Summary, want)
	}
}

func TestSummary_NenhumPromptCheckpointDeTurno(t *testing.T) {
	h := Synthesize(Input{Mode: ModeTurnCheckpoint, Observations: []ingest.Event{
		postTool("Bash", ingest.ToolFamilyNonFile),
	}})
	if want := "Turn checkpoint; 1 observations recorded."; h.Summary != want {
		t.Fatalf("Summary = %q, want %q", h.Summary, want)
	}
}

func TestSummary_ContagemZero(t *testing.T) {
	h := Synthesize(Input{Mode: ModeSessionEnd})
	if want := "Session ended; 0 observations recorded."; h.Summary != want {
		t.Fatalf("Summary = %q, want %q", h.Summary, want)
	}
}

func TestSummary_MarkupNaoContaComoPrompt(t *testing.T) {
	cases := []struct {
		name  string
		input ingest.Event
	}{
		{"system-reminder", promptEv("<system-reminder>context</system-reminder>")},
		{"task-notification", promptEv("<task-notification>\nfinished\n</task-notification>")},
		{"blocos encadeados", promptEv("<command-message>x</command-message>\n<command-name>/y</command-name>")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := Synthesize(Input{Mode: ModeSessionEnd, Observations: []ingest.Event{
				promptEv("real ask"), c.input,
			}})
			if want := "Session focused on: real ask"; h.Summary != want {
				t.Fatalf("Summary = %q, want %q", h.Summary, want)
			}
		})
	}
}

func TestSummary_TextoQueParecaTagMasNaoEMarkup(t *testing.T) {
	// Termina sem `>` ou começa com dígito: não é markup e conta como prompt.
	for _, text := range []string{"<div> is broken, why?", "<3 this tool", "<b>bold"} {
		h := Synthesize(Input{Mode: ModeSessionEnd, Observations: []ingest.Event{promptEv(text)}})
		if want := "Session focused on: " + text; h.Summary != want {
			t.Fatalf("texto %q: Summary = %q, want %q", text, h.Summary, want)
		}
	}
}

func TestSummary_PromptVazioNaoConta(t *testing.T) {
	h := Synthesize(Input{Mode: ModeSessionEnd, Observations: []ingest.Event{
		promptEv("   "), promptEv("real ask"),
	}})
	if want := "Session focused on: real ask"; h.Summary != want {
		t.Fatalf("Summary = %q, want %q", h.Summary, want)
	}
}

func TestSummary_PromptLongoCortaEmMilEQuinhentos(t *testing.T) {
	long := repeat('a', 1600)
	h := Synthesize(Input{Mode: ModeSessionEnd, Observations: []ingest.Event{promptEv(long)}})
	want := "Session focused on: " + repeat('a', 1500) + "…"
	if h.Summary != want {
		t.Fatalf("Summary tem %d runes, want %d", len([]rune(h.Summary)), len([]rune(want)))
	}
}

func TestCutPrompt(t *testing.T) {
	exact := repeat('é', 1500)
	if got := cutPrompt(exact); got != exact {
		t.Fatalf("1500 runes foi cortado")
	}
	over := repeat('é', 1501)
	want := repeat('é', 1500) + "…"
	if got := cutPrompt(over); got != want {
		t.Fatalf("1501 runes: got %d runes, want 1500 + …", len([]rune(got)))
	}
	cjk := repeat('好', 1600)
	if got := cutPrompt(cjk); !strings.HasSuffix(got, "…") || len([]rune(got)) != 1501 {
		t.Fatalf("CJK: corte inválido, %d runes", len([]rune(got)))
	}
}

func TestSummary_PromptsComSanitizacao(t *testing.T) {
	// Sanitização roda antes do corte: segredo não vai para o handoff.
	secret := "use key sk-ant-abcdefghijklmnopqrstuvwxyz123456 please"
	h := Synthesize(Input{Mode: ModeSessionEnd, Observations: []ingest.Event{promptEv(secret)}})
	if strings.Contains(h.Summary, "sk-ant-abcdefghijklmnopqrstuvwxyz123456") {
		t.Fatalf("segredo vazou no resumo: %q", h.Summary)
	}
}

// --- §9.2 Próximos passos --------------------------------------------------

func TestNextSteps_ToolsUsedOrdenadoEDistinto(t *testing.T) {
	h := Synthesize(Input{Mode: ModeSessionEnd, Observations: []ingest.Event{
		promptEv("go"),
		toolEv(domain.HookPreToolUse, "Read", ingest.ToolFamilyFile),
		postTool("Read", ingest.ToolFamilyFile),
		postTool("Bash", ingest.ToolFamilyNonFile),
		postTool("Grep", ingest.ToolFamilySearchList),
		postTool("Bash", ingest.ToolFamilyNonFile),
	}})
	if want := "Tools used: Bash, Grep, Read"; h.NextSteps != want {
		t.Fatalf("NextSteps = %q, want %q", h.NextSteps, want)
	}
}

func TestNextSteps_RotulosDeFamiliaDescartados(t *testing.T) {
	h := Synthesize(Input{Mode: ModeSessionEnd, Observations: []ingest.Event{
		postTool("tool file", ingest.ToolFamilyFile),
		postTool("tool non-file", ingest.ToolFamilyNonFile),
		postTool("file", ingest.ToolFamilyFile),
		postTool("unknown", ingest.ToolFamilyUnknown),
		postTool("tool search-list", ingest.ToolFamilySearchList),
		postTool("Edit", ingest.ToolFamilyFile),
	}})
	if want := "Tools used: Edit"; h.NextSteps != want {
		t.Fatalf("NextSteps = %q, want %q", h.NextSteps, want)
	}
}

func TestNextSteps_SemFerramentaVazio(t *testing.T) {
	h := Synthesize(Input{Mode: ModeSessionEnd, Observations: []ingest.Event{
		promptEv("hi"), postTool("tool file", ingest.ToolFamilyFile),
	}})
	if h.NextSteps != "" {
		t.Fatalf("NextSteps = %q, want vazio", h.NextSteps)
	}
}

func TestNextSteps_EventoNaoFerramentaIgnorado(t *testing.T) {
	h := Synthesize(Input{Mode: ModeSessionEnd, Observations: []ingest.Event{
		{Hook: domain.HookStop, ToolName: "Bash"},
	}})
	if h.NextSteps != "" {
		t.Fatalf("NextSteps = %q, want vazio", h.NextSteps)
	}
}

// --- §9.3 Perguntas em aberto ----------------------------------------------

func TestOpenQuestions_Regra1PerguntaNaoResolvida(t *testing.T) {
	h := Synthesize(Input{Mode: ModeSessionEnd, Observations: []ingest.Event{
		promptEv("Why does it fail?"), postTool("Bash", ingest.ToolFamilyNonFile), stopEv(), endEv(),
	}})
	want := []string{"Unresolved question: Why does it fail?"}
	assertLines(t, h.OpenQuestions, want)
}

func TestOpenQuestions_Regra1PerguntaFullwidth(t *testing.T) {
	h := Synthesize(Input{Mode: ModeSessionEnd, Observations: []ingest.Event{
		promptEv("Por que falha？"), stopEv(), endEv(),
	}})
	assertLines(t, h.OpenQuestions, []string{"Unresolved question: Por que falha？"})
}

func TestOpenQuestions_Regra1VenceRegra2(t *testing.T) {
	// Última ferramenta é de arquivo e não há parada, mas a pergunta vem primeiro.
	h := Synthesize(Input{Mode: ModeSessionEnd, Observations: []ingest.Event{
		promptEv("Can you fix it?"), postTool("Edit", ingest.ToolFamilyFile),
	}})
	assertLines(t, h.OpenQuestions, []string{"Unresolved question: Can you fix it?"})
}

func TestOpenQuestions_Regra1UsaUltimoPromptNaoMarkup(t *testing.T) {
	// Markup após a pergunta não conta: a pergunta continua sendo o último prompt.
	h := Synthesize(Input{Mode: ModeSessionEnd, Observations: []ingest.Event{
		promptEv("Why?"), promptEv("<system-reminder>x</system-reminder>"), stopEv(), endEv(),
	}})
	assertLines(t, h.OpenQuestions, []string{"Unresolved question: Why?"})
}

func TestOpenQuestions_Regra2ArquivoSemParada(t *testing.T) {
	h := Synthesize(Input{Mode: ModeSessionEnd, Observations: []ingest.Event{
		promptEv("Refactor X"), postTool("Edit", ingest.ToolFamilyFile),
	}})
	want := []string{
		"Session ended without a normal stop while working with files",
		"Check the working tree for uncommitted or unverified changes",
	}
	assertLines(t, h.OpenQuestions, want)
}

func TestOpenQuestions_Regra2NaoCasaComParada(t *testing.T) {
	// Com parada, a regra 2 não casa; a regra 3 (sem fim de sessão) vale.
	h := Synthesize(Input{Mode: ModeSessionEnd, Observations: []ingest.Event{
		promptEv("Refactor X"), postTool("Edit", ingest.ToolFamilyFile), stopEv(),
	}})
	assertLines(t, h.OpenQuestions, []string{"Continue from (mid-task exit): Refactor X"})
}

func TestOpenQuestions_Regra2NaoCasaSeUltimaFerramentaNaoEArquivo(t *testing.T) {
	h := Synthesize(Input{Mode: ModeSessionEnd, Observations: []ingest.Event{
		promptEv("Refactor X"),
		postTool("Edit", ingest.ToolFamilyFile),
		postTool("Bash", ingest.ToolFamilyNonFile),
	}})
	assertLines(t, h.OpenQuestions, []string{"Continue from: Refactor X"})
}

func TestOpenQuestions_Regra3ParadaSemFimDeSessao(t *testing.T) {
	h := Synthesize(Input{Mode: ModeSessionEnd, Observations: []ingest.Event{
		promptEv("first"), promptEv("Refactor X"), stopEv(),
	}})
	assertLines(t, h.OpenQuestions, []string{"Continue from (mid-task exit): Refactor X"})
}

func TestOpenQuestions_Regra4FimNormal(t *testing.T) {
	h := Synthesize(Input{Mode: ModeSessionEnd, Observations: []ingest.Event{
		promptEv("Refactor X"), stopEv(), endEv(),
	}})
	assertLines(t, h.OpenQuestions, []string{"Continue from: Refactor X"})
}

func TestOpenQuestions_Regra4UsaUltimoSubstantivo(t *testing.T) {
	h := Synthesize(Input{Mode: ModeSessionEnd, Observations: []ingest.Event{
		promptEv("Refactor X"), promptEv("ok"), stopEv(), endEv(),
	}})
	assertLines(t, h.OpenQuestions, []string{"Continue from: Refactor X"})
}

func TestOpenQuestions_Regra5Vazia(t *testing.T) {
	h := Synthesize(Input{Mode: ModeSessionEnd, Observations: []ingest.Event{
		ingest.Event{Hook: domain.HookSessionStart},
	}})
	if h.OpenQuestions != nil {
		t.Fatalf("OpenQuestions = %#v, want nil", h.OpenQuestions)
	}
}

func TestOpenQuestions_Regra5SemPromptSubstantivo(t *testing.T) {
	// Só confirmação e parada: nenhuma regra de texto tem prompt para mostrar.
	h := Synthesize(Input{Mode: ModeSessionEnd, Observations: []ingest.Event{
		promptEv("ok"), stopEv(), endEv(),
	}})
	if h.OpenQuestions != nil {
		t.Fatalf("OpenQuestions = %#v, want nil", h.OpenQuestions)
	}
}

func TestOpenQuestions_PromptLongoCortadoNaRegra(t *testing.T) {
	long := repeat('b', 1600)
	h := Synthesize(Input{Mode: ModeSessionEnd, Observations: []ingest.Event{
		promptEv(long), stopEv(), endEv(),
	}})
	assertLines(t, h.OpenQuestions, []string{"Continue from: " + repeat('b', 1500) + "…"})
}

func TestOpenQuestions_CheckpointSoUmaRegra(t *testing.T) {
	// No checkpoint, a pergunta não resolvida não vale: só a última solicitação.
	h := Synthesize(Input{Mode: ModeTurnCheckpoint, Observations: []ingest.Event{
		promptEv("Why does it fail?"), postTool("Edit", ingest.ToolFamilyFile),
	}})
	assertLines(t, h.OpenQuestions, []string{"Continue from last request: Why does it fail?"})
}

func TestOpenQuestions_CheckpointSemSubstantivoVazio(t *testing.T) {
	h := Synthesize(Input{Mode: ModeTurnCheckpoint, Observations: []ingest.Event{
		promptEv("ok"), postTool("Edit", ingest.ToolFamilyFile),
	}})
	if h.OpenQuestions != nil {
		t.Fatalf("OpenQuestions = %#v, want nil", h.OpenQuestions)
	}
}

// --- Prompt de confirmação --------------------------------------------------

func TestIsConfirmation(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"", true},
		{"   ", true},
		{"...", true},
		{"ok", true},
		{"OK!", true},
		{"thanks.", true},
		{"thank you", true},
		{"got it", true},
		{"sounds good!!", true},
		{"好的", true},
		{"好的。", true},
		{"谢谢", true},
		{"谢谢!", true},
		{"Refactor the parser", false},
		{"fix it", false},
		{"ok, can you refactor the parser module?", false}, // mais de 20 caracteres
		{"好的，请帮我修复这个问题吧", false},                           // frase longa, não confirmação
		{"okay then, deploy", false},
	}
	for _, c := range cases {
		if got := isConfirmation(c.text); got != c.want {
			t.Errorf("isConfirmation(%q) = %v, want %v", c.text, got, c.want)
		}
	}
}

func TestConfirmation_NaoCasaComoPerguntaNaRegra1(t *testing.T) {
	// "ok?" termina com `?`, mas é confirmação: não vira pergunta não resolvida.
	h := Synthesize(Input{Mode: ModeSessionEnd, Observations: []ingest.Event{
		promptEv("Could you check?"), promptEv("ok?"), stopEv(), endEv(),
	}})
	assertLines(t, h.OpenQuestions, []string{"Continue from: Could you check?"})
}

func TestConfirmation_IgnoradaComoContinuacaoNaRegra2(t *testing.T) {
	h := Synthesize(Input{Mode: ModeSessionEnd, Observations: []ingest.Event{
		promptEv("Refactor X"), promptEv("thanks!"), postTool("Edit", ingest.ToolFamilyFile),
	}})
	want := []string{
		"Session ended without a normal stop while working with files",
		"Check the working tree for uncommitted or unverified changes",
	}
	assertLines(t, h.OpenQuestions, want)
}

func TestConfirmation_SummaryMantemConfirmacao(t *testing.T) {
	// Confirmação só é ignorada em perguntas em aberto; o resumo usa todos os prompts.
	h := Synthesize(Input{Mode: ModeSessionEnd, Observations: []ingest.Event{
		promptEv("Refactor X"), promptEv("ok"),
	}})
	if want := "Started: Refactor X\n\nLast: ok"; h.Summary != want {
		t.Fatalf("Summary = %q, want %q", h.Summary, want)
	}
}

// --- Ordem e pureza ---------------------------------------------------------

func TestSynthesize_Determinista(t *testing.T) {
	in := Input{Mode: ModeSessionEnd, Observations: []ingest.Event{
		promptEv("Refactor X"), postTool("Read", ingest.ToolFamilyFile), stopEv(), endEv(),
	}}
	a := Synthesize(in)
	b := Synthesize(in)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("saídas diferentes para a mesma entrada: %#v vs %#v", a, b)
	}
}

func assertLines(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("OpenQuestions = %#v, want %#v", got, want)
	}
}
