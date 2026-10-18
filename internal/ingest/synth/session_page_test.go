package synth

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
	"github.com/phm-aguiar/ai-memory-go/internal/ingest"
)

// spNow é o horário base das observações de teste (UTC).
var spNow = time.Date(2026, 10, 13, 10, 0, 0, 0, time.UTC)

const spSessionUUID = "7f3a1c2e-9b4d-4e5f-8a6b-1c2d3e4f5a6b"

// spID devolve o identificador de sessão fixo dos testes, na forma canônica.
func spID(t *testing.T) domain.SessionID {
	t.Helper()
	sid, err := domain.SessionIDFromNative(spSessionUUID)
	if err != nil {
		t.Fatalf("SessionIDFromNative: %v", err)
	}
	return sid
}

// spPage sintetiza a página com a sessão e o agente de teste.
func spPage(t *testing.T, events ...ingest.Event) SessionPage {
	t.Helper()
	return SynthesizeSessionPage(SessionInput{Session: spID(t), Agent: "claude-code", Observations: events})
}

// spPrompt é um prompt do usuário no horário dado.
func spPrompt(text string, at time.Time) ingest.Event {
	return ingest.Event{Hook: domain.HookUserPrompt, Title: text, Body: text, OccurredAt: at}
}

// spPost é uma pós-ferramenta com o nome dado. Família ausente vira unknown.
func spPost(name string, at time.Time) ingest.Event {
	return ingest.Event{Hook: domain.HookPostToolUse, ToolName: name, ToolFamily: ingest.ToolFamilyNonFile, Title: "tool non-file", OccurredAt: at}
}

// spPre é uma pré-ferramenta com o nome dado. Não conta no resumo.
func spPre(name string, at time.Time) ingest.Event {
	return ingest.Event{Hook: domain.HookPreToolUse, ToolName: name, ToolFamily: ingest.ToolFamilyNonFile, Title: "tool non-file", OccurredAt: at}
}

// spEv é um evento genérico com título.
func spEv(hook domain.HookEvent, title string, at time.Time) ingest.Event {
	return ingest.Event{Hook: hook, Title: title, OccurredAt: at}
}

// spSummary devolve só o resumo das observações dadas.
func spSummary(t *testing.T, events ...ingest.Event) string {
	t.Helper()
	return spPage(t, events...).Summary
}

// --- Título (LEI §10.1) ----------------------------------------------------

func TestSessionPage_TituloPrimeiroPrompt(t *testing.T) {
	p := spPage(t,
		spEv(domain.HookSessionStart, "Claude Opus", spNow),
		spPrompt("Refactor the parser\nwith more detail", spNow.Add(time.Minute)),
		spPrompt("Second prompt", spNow.Add(2*time.Minute)),
	)
	if want := "Refactor the parser"; p.Title != want {
		t.Fatalf("Title = %q, want %q", p.Title, want)
	}
}

func TestSessionPage_TituloPulaPromptVazioEMarkup(t *testing.T) {
	p := spPage(t,
		spPrompt("   ", spNow),
		spPrompt("<ide_selection>file.go</ide_selection>", spNow.Add(time.Second)),
		spPrompt("Fix login", spNow.Add(2*time.Second)),
	)
	if want := "Fix login"; p.Title != want {
		t.Fatalf("Title = %q, want %q", p.Title, want)
	}
}

func TestSessionPage_TituloPrimeiroPromptCortadoEm80(t *testing.T) {
	long := strings.Repeat("a", 100)
	p := spPage(t, spPrompt(long, spNow))
	if n := utf8.RuneCountInString(p.Title); n != 80 {
		t.Fatalf("len(Title) = %d runes, want 80", n)
	}
	if !strings.HasSuffix(p.Title, "…") {
		t.Fatalf("Title = %q, want suffix …", p.Title)
	}
}

func TestSessionPage_TituloPreambuloSubagenteLinhaSeguinte(t *testing.T) {
	text := subagentPreamble + "\n\nSearch the repo for TODOs\nmore context"
	p := spPage(t, spPrompt(text, spNow))
	if want := "Search the repo for TODOs"; p.Title != want {
		t.Fatalf("Title = %q, want %q", p.Title, want)
	}
}

func TestSessionPage_TituloPreambuloSubagenteMesmaLinha(t *testing.T) {
	text := subagentPreamble + " Count the tests"
	p := spPage(t, spPrompt(text, spNow))
	if want := "Count the tests"; p.Title != want {
		t.Fatalf("Title = %q, want %q", p.Title, want)
	}
}

// Preâmbulo sem linha útil dá `Session <id>` e não cai para outras observações, mesmo que elas
// tenham título útil (LEI §10.1, item 2).
func TestSessionPage_PreambuloSemLinhaUtilNaoCaiParaOutraObservacao(t *testing.T) {
	p := spPage(t,
		spPrompt(subagentPreamble+"\n\n   ", spNow),
		spEv(domain.HookNotification, "Build finished", spNow.Add(time.Minute)),
		spPost("Bash", spNow.Add(2*time.Minute)),
	)
	if want := "Session " + spSessionUUID; p.Title != want {
		t.Fatalf("Title = %q, want %q", p.Title, want)
	}
}

// Cada evento que não pode dar título, sozinho, cai em `Session <id>` (LEI §10.1, item 3 e 4).
func TestSessionPage_TituloOutrosEventosRejeitados(t *testing.T) {
	cases := map[string]ingest.Event{
		"inicio de sessao com titulo": spEv(domain.HookSessionStart, "Claude Opus", spNow),
		"rotulo tool familia":         {Hook: domain.HookPreToolUse, Title: "tool file", OccurredAt: spNow},
		"rotulo tool unknown pos":     {Hook: domain.HookPostToolUse, Title: "tool unknown", OccurredAt: spNow},
		"titulo igual ao tipo":        spEv(domain.HookNotification, "Notification", spNow),
		"titulo igual ao tipo kebab":  spEv(domain.HookPreCompact, "pre-compact", spNow),
		"titulo vazio":                spEv(domain.HookNotification, "  ", spNow),
	}
	for name, ev := range cases {
		t.Run(name, func(t *testing.T) {
			p := spPage(t, ev)
			if want := "Session " + spSessionUUID; p.Title != want {
				t.Fatalf("Title = %q, want %q", p.Title, want)
			}
		})
	}
}

// Qualquer outro título útil de outro evento vence (LEI §10.1, item 3).
func TestSessionPage_TituloOutroEventoUtil(t *testing.T) {
	p := spPage(t,
		spEv(domain.HookSessionStart, "Claude Opus", spNow),
		spPre("Bash", spNow.Add(time.Second)),
		spEv(domain.HookNotification, "Notification", spNow.Add(2*time.Second)),
		spEv(domain.HookPostCompaction, "Compacted: auth refactor", spNow.Add(3*time.Second)),
	)
	if want := "Compacted: auth refactor"; p.Title != want {
		t.Fatalf("Title = %q, want %q", p.Title, want)
	}
}

// Observação com título útil, sem prompt, define o título mesmo que todos os demais sejam pulados.
func TestSessionPage_TituloNaoSessaoSemPrompt(t *testing.T) {
	p := spPage(t, spEv(domain.HookStop, "Waiting for input", spNow))
	if want := "Waiting for input"; p.Title != want {
		t.Fatalf("Title = %q, want %q", p.Title, want)
	}
}

// Observações sem título útil nenhum: `Session <id>` (LEI §10.1, item 4).
func TestSessionPage_TituloSessaoComIdSemTituloUtil(t *testing.T) {
	p := spPage(t, spEv(domain.HookStop, "", spNow), spPost("Bash", spNow))
	if want := "Session " + spSessionUUID; p.Title != want {
		t.Fatalf("Title = %q, want %q", p.Title, want)
	}
}

// Sem observação alguma: literal `session` (LEI §10.1, item 5).
func TestSessionPage_TituloSemObservacao(t *testing.T) {
	p := spPage(t)
	if p.Title != "session" {
		t.Fatalf("Title = %q, want session", p.Title)
	}
	if p.Summary != "" {
		t.Fatalf("Summary = %q, want vazio", p.Summary)
	}
	if strings.Contains(p.Body, "## Raw observations") {
		t.Fatalf("Body tem Raw observations sem observação:\n%s", p.Body)
	}
}

// --- Resumo (LEI §10.2) ----------------------------------------------------

func TestSessionPage_ResumoPluralizacaoPrompts(t *testing.T) {
	if got := spSummary(t, spPrompt("one", spNow)); got != "1 prompt." {
		t.Fatalf("Summary = %q, want %q", got, "1 prompt.")
	}
	if got := spSummary(t, spPrompt("one", spNow), spPrompt("two", spNow)); got != "2 prompts." {
		t.Fatalf("Summary = %q, want %q", got, "2 prompts.")
	}
}

func TestSessionPage_ResumoSoPosFerramentaConta(t *testing.T) {
	got := spSummary(t, spPre("Bash", spNow), spPre("Bash", spNow), spPost("Bash", spNow))
	if want := "1 completed tool call across Bash."; got != want {
		t.Fatalf("Summary = %q, want %q", got, want)
	}
}

func TestSessionPage_ResumoSingularEPluralChamadas(t *testing.T) {
	got := spSummary(t, spPost("Edit", spNow))
	if want := "1 completed tool call across Edit."; got != want {
		t.Fatalf("Summary = %q, want %q", got, want)
	}
}

func TestSessionPage_ResumoDoisNomesESessao(t *testing.T) {
	got := spSummary(t,
		spPrompt("one", spNow),
		spPrompt("two", spNow),
		spPost("Bash", spNow.Add(time.Second)),
		spPost("Edit", spNow.Add(18*time.Minute)),
	)
	if want := "2 prompts, 2 completed tool calls across Bash and Edit, over 18m."; got != want {
		t.Fatalf("Summary = %q, want %q", got, want)
	}
}

// Até três nomes, por volume, com desempate alfabético, e depois `and <K> more`.
func TestSessionPage_ResumoNomesOrdenadosEMore(t *testing.T) {
	var events []ingest.Event
	// Bash 3, Edit 2, Grep 1, Read 1: três nomeados e 1 more.
	for i := 0; i < 3; i++ {
		events = append(events, spPost("Bash", spNow))
	}
	events = append(events, spPost("Edit", spNow), spPost("Edit", spNow))
	events = append(events, spPost("Read", spNow), spPost("Grep", spNow))
	want := "7 completed tool calls across Bash, Edit, Grep and 1 more."
	if got := spSummary(t, events...); got != want {
		t.Fatalf("Summary = %q, want %q", got, want)
	}
}

// Empate de volume: ordem alfabética decide quem é nomeado.
func TestSessionPage_ResumoDesempateAlfabetico(t *testing.T) {
	got := spSummary(t, spPost("Zeta", spNow), spPost("Alpha", spNow), spPost("Mid", spNow), spPost("Beta", spNow))
	want := "4 completed tool calls across Alpha, Beta, Mid and 1 more."
	if got != want {
		t.Fatalf("Summary = %q, want %q", got, want)
	}
}

// Rótulos de família contam, mas não são nomeados. Chamada sem nome também só conta.
func TestSessionPage_ResumoRotulosContadosNaoNomeados(t *testing.T) {
	got := spSummary(t,
		spPost("file", spNow),
		spPost("tool search-list", spNow),
		spPost("", spNow),
		spPost("Bash", spNow),
	)
	if want := "4 completed tool calls across Bash."; got != want {
		t.Fatalf("Summary = %q, want %q", got, want)
	}
}

// Só rótulos: a cláusula de nomes é omitida.
func TestSessionPage_ResumoSoRotulosOmiteNomes(t *testing.T) {
	got := spSummary(t, spPost("file", spNow), spPost("unknown", spNow))
	if want := "2 completed tool calls."; got != want {
		t.Fatalf("Summary = %q, want %q", got, want)
	}
}

func TestSessionPage_ResumoDuracoes(t *testing.T) {
	cases := []struct {
		name string
		span time.Duration
		want string
	}{
		{"segundos", 45 * time.Second, "over 45s."},
		{"minutos", 18 * time.Minute, "over 18m."},
		{"horas exatas", 2 * time.Hour, "over 2h."},
		{"horas e minutos", 90 * time.Minute, "over 1h 30m."},
		{"zero omitida", 0, ""},
		{"menos de um segundo omitida", 500 * time.Millisecond, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := spSummary(t, spEv(domain.HookSessionStart, "", spNow), spEv(domain.HookSessionEnd, "", spNow.Add(c.span)))
			if got != c.want {
				t.Fatalf("Summary = %q, want %q", got, c.want)
			}
		})
	}
}

func TestSessionPage_FormatDuration(t *testing.T) {
	cases := map[time.Duration]string{
		1 * time.Second:                 "1s",
		59 * time.Second:                "59s",
		60 * time.Second:                "1m",
		18*time.Minute + 59*time.Second: "18m",
		59*time.Minute + 59*time.Second: "59m",
		time.Hour:                       "1h",
		time.Hour + 5*time.Minute:       "1h 5m",
	}
	for d, want := range cases {
		if got := formatDuration(d); got != want {
			t.Errorf("formatDuration(%v) = %q, want %q", d, got, want)
		}
	}
}

// Só a duração: sem prompt nem ferramenta, o resumo ainda diz o que há.
func TestSessionPage_ResumoSoDuracao(t *testing.T) {
	got := spSummary(t, spEv(domain.HookNotification, "x", spNow), spEv(domain.HookStop, "", spNow.Add(5*time.Minute)))
	if want := "over 5m."; got != want {
		t.Fatalf("Summary = %q, want %q", got, want)
	}
}

// Nada couber: resumo omitido.
func TestSessionPage_ResumoOmitidoQuandoNadaCabe(t *testing.T) {
	if got := spSummary(t, spEv(domain.HookNotification, "x", spNow)); got != "" {
		t.Fatalf("Summary = %q, want vazio", got)
	}
}

// --- Corpo (LEI §10.3) -----------------------------------------------------

func TestSessionPage_CorpoSecoesNaOrdem(t *testing.T) {
	p := spPage(t,
		spPrompt("Fix login", spNow),
		spPost("Bash", spNow.Add(time.Minute)),
	)
	heads := []string{"# Fix login", "## Session metadata", "## Prompts", "## Tool calls", "## Raw observations"}
	last := -1
	for _, h := range heads {
		i := strings.Index(p.Body, h)
		if i < 0 {
			t.Fatalf("Body sem %q:\n%s", h, p.Body)
		}
		if i <= last {
			t.Fatalf("%q fora de ordem:\n%s", h, p.Body)
		}
		last = i
	}
}

func TestSessionPage_CorpoMetadados(t *testing.T) {
	p := spPage(t,
		spPrompt("Fix login", spNow),
		spPost("Bash", spNow.Add(time.Minute)),
		spEv(domain.HookSessionEnd, "", spNow.Add(2*time.Minute)),
	)
	for _, want := range []string{
		"- Session: " + spSessionUUID,
		"- Started: 2026-10-13T10:00:00Z",
		"- Ended: 2026-10-13T10:02:00Z",
		"- Observations: 3",
	} {
		if !strings.Contains(p.Body, want) {
			t.Errorf("Body sem %q:\n%s", want, p.Body)
		}
	}
}

func TestSessionPage_CorpoPromptsNumeradosESemSecaoVazia(t *testing.T) {
	p := spPage(t, spPrompt("First", spNow), spPrompt("Second", spNow.Add(time.Second)))
	if !strings.Contains(p.Body, "## Prompts\n\n1. First\n2. Second\n") {
		t.Fatalf("lista de prompts errada:\n%s", p.Body)
	}
	empty := spPage(t, spEv(domain.HookNotification, "note", spNow))
	if strings.Contains(empty.Body, "## Prompts") || strings.Contains(empty.Body, "## Tool calls") {
		t.Fatalf("seções vazias não deviam aparecer:\n%s", empty.Body)
	}
}

func TestSessionPage_CorpoContagemPorNome(t *testing.T) {
	p := spPage(t,
		spPost("Edit", spNow),
		spPost("Bash", spNow),
		spPost("Bash", spNow),
		spPost("", spNow),
	)
	if !strings.Contains(p.Body, "## Tool calls\n\n- Bash: 2\n- (unnamed): 1\n- Edit: 1\n") {
		t.Fatalf("contagem por nome errada:\n%s", p.Body)
	}
}

func TestSessionPage_CorpoLinhaDeObservacao(t *testing.T) {
	p := spPage(t, spPrompt("Fix login", spNow.Add(90*time.Second)))
	want := "- user-prompt 2026-10-13T10:01:30Z Fix login"
	if !strings.Contains(p.Body, want) {
		t.Fatalf("Body sem %q:\n%s", want, p.Body)
	}
}

func TestSessionPage_CorpoTituloDaObservacaoAte80(t *testing.T) {
	long := strings.Repeat("b", 120)
	p := spPage(t, spEv(domain.HookNotification, long, spNow))
	var line string
	for _, l := range strings.Split(p.Body, "\n") {
		if strings.HasPrefix(l, "- notification ") {
			line = l
		}
	}
	title := strings.TrimPrefix(line, "- notification 2026-10-13T10:00:00Z ")
	if n := utf8.RuneCountInString(title); n != 80 {
		t.Fatalf("título da linha = %d runes, want 80: %q", n, title)
	}
	if !strings.HasSuffix(title, "…") {
		t.Fatalf("título da linha = %q, want sufixo …", title)
	}
}

// rawLines devolve as linhas da seção Raw observations.
func rawLines(body string) []string {
	_, after, ok := strings.Cut(body, "## Raw observations\n\n")
	if !ok {
		return nil
	}
	var out []string
	for _, l := range strings.Split(strings.TrimRight(after, "\n"), "\n") {
		out = append(out, l)
	}
	return out
}

// manyEvents gera n observações notification com título `obs-<i>`, a um segundo de distância.
func manyEvents(n int) []ingest.Event {
	out := make([]ingest.Event, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, spEv(domain.HookNotification, fmt.Sprintf("obs-%04d", i), spNow.Add(time.Duration(i)*time.Second)))
	}
	return out
}

func TestSessionPage_RawAte500MostraTodas(t *testing.T) {
	p := spPage(t, manyEvents(500)...)
	lines := rawLines(p.Body)
	if len(lines) != 500 {
		t.Fatalf("linhas = %d, want 500", len(lines))
	}
	if strings.Contains(p.Body, "omitted") {
		t.Fatalf("não devia ter nota de corte com 500 observações")
	}
}

func TestSessionPage_RawAcima500CortaCom250e250ENota(t *testing.T) {
	p := spPage(t, manyEvents(501)...)
	lines := rawLines(p.Body)
	if len(lines) != 250+1+250 {
		t.Fatalf("linhas = %d, want 501 (250 + nota + 250)", len(lines))
	}
	// Com 501 observações: obs-0000..obs-0249, a nota no índice 250 e obs-0251..obs-0500.
	assertRawWindow(t, lines)
	note := lines[250]
	if !strings.Contains(note, "1 observation omitted") || !strings.Contains(note, "501 recorded") {
		t.Fatalf("nota = %q, want 1 observação omitida de 501", note)
	}
}

func TestSessionPage_RawMilCortaQuinhentasComNota(t *testing.T) {
	p := spPage(t, manyEvents(1000)...)
	lines := rawLines(p.Body)
	if len(lines) != 250+1+250 {
		t.Fatalf("linhas = %d, want 501", len(lines))
	}
	if !strings.Contains(lines[250], "500 observations omitted") || !strings.Contains(lines[250], "1000 recorded") {
		t.Fatalf("nota = %q", lines[250])
	}
	// Com 1000 observações: primeiras obs-0000..obs-0249, últimas obs-0750..obs-0999.
	if lines[0] != "- notification 2026-10-13T10:00:00Z obs-0000" ||
		lines[249] != "- notification 2026-10-13T10:04:09Z obs-0249" ||
		lines[251] != "- notification 2026-10-13T10:12:30Z obs-0750" ||
		lines[500] != "- notification 2026-10-13T10:16:39Z obs-0999" {
		t.Fatalf("janela de corte errada: %q / %q / %q / %q", lines[0], lines[249], lines[251], lines[500])
	}
}

// assertRawWindow confere a janela de 501 observações: as 250 primeiras, a nota e as 250 últimas
// (obs-0251..obs-0500). obs-0250 é a omitida.
func assertRawWindow(t *testing.T, lines []string) {
	t.Helper()
	if lines[0] != "- notification 2026-10-13T10:00:00Z obs-0000" {
		t.Fatalf("primeira linha = %q", lines[0])
	}
	if lines[249] != "- notification 2026-10-13T10:04:09Z obs-0249" {
		t.Fatalf("249 = %q", lines[249])
	}
	if lines[251] != "- notification 2026-10-13T10:04:11Z obs-0251" {
		t.Fatalf("251 = %q", lines[251])
	}
	if lines[500] != "- notification 2026-10-13T10:08:20Z obs-0500" {
		t.Fatalf("última = %q", lines[500])
	}
	for _, l := range lines {
		if strings.HasSuffix(l, " obs-0250") {
			t.Fatalf("obs-0250 devia ter sido omitida")
		}
	}
}

// --- Frontmatter (LEI §10) -------------------------------------------------

func TestSessionPage_FrontmatterCompleto(t *testing.T) {
	p := spPage(t,
		spPrompt("Fix login", spNow),
		spPost("Bash", spNow.Add(time.Minute)),
	)
	if want := []string{"title", "session_id", "agent", "tier", "summary"}; !equalKeys(p.Frontmatter.Keys(), want) {
		t.Fatalf("keys = %v, want %v", p.Frontmatter.Keys(), want)
	}
	checkFM(t, p, "title", "Fix login")
	checkFM(t, p, "session_id", spSessionUUID)
	checkFM(t, p, "agent", "claude-code")
	checkFM(t, p, "tier", "episodic")
	checkFM(t, p, "summary", "1 prompt, 1 completed tool call across Bash, over 1m.")
	if want := "sessions/" + spSessionUUID + ".md"; p.Path != want {
		t.Fatalf("Path = %q, want %q", p.Path, want)
	}
}

func TestSessionPage_FrontmatterSemSummaryQuandoVazio(t *testing.T) {
	p := spPage(t, spEv(domain.HookNotification, "note", spNow))
	if want := []string{"title", "session_id", "agent", "tier"}; !equalKeys(p.Frontmatter.Keys(), want) {
		t.Fatalf("keys = %v, want %v", p.Frontmatter.Keys(), want)
	}
	if _, ok := p.Frontmatter.Get("summary"); ok {
		t.Fatalf("summary presente sem resumo")
	}
	if p.Summary != "" {
		t.Fatalf("Summary = %q, want vazio", p.Summary)
	}
}

func TestSessionPage_FrontmatterAgentEOrigemPreservada(t *testing.T) {
	in := SessionInput{Session: spID(t), Agent: "cursor", Observations: []ingest.Event{spPrompt("hi", spNow)}}
	p := SynthesizeSessionPage(in)
	checkFM(t, p, "agent", "cursor")
}

func TestSessionPage_Determinista(t *testing.T) {
	events := []ingest.Event{
		spPrompt("Fix login", spNow),
		spPost("Edit", spNow.Add(time.Second)),
		spPost("Bash", spNow.Add(2*time.Second)),
	}
	a := spPage(t, events...)
	b := spPage(t, events...)
	if a.Body != b.Body || a.Summary != b.Summary || a.Title != b.Title {
		t.Fatalf("saídas diferentes para a mesma entrada")
	}
}

// checkFM confere um valor de texto do frontmatter.
func checkFM(t *testing.T, p SessionPage, key, want string) {
	t.Helper()
	v, ok := p.Frontmatter.Get(key)
	if !ok {
		t.Fatalf("frontmatter sem %q", key)
	}
	if got, _ := v.(string); got != want {
		t.Fatalf("frontmatter[%q] = %q, want %q", key, got, want)
	}
}

// equalKeys compara duas listas de chaves na mesma ordem.
func equalKeys(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
