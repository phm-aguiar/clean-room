package synth

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
	"github.com/phm-aguiar/ai-memory-go/internal/domain/sanitize"
	"github.com/phm-aguiar/ai-memory-go/internal/ingest"
)

// Página de sessão sintetizada, sem LLM (LEI §10). Função pura: não lê arquivo, não faz I/O.
//
// Fontes: docs/lifecycle-event-ingestion.md §10 (vence sobre a spec e o plan) e
// specs/features/002-captura-e-acesso/notes/handoff-contract.md §7.
//
// Decisões nossas, não fixadas pelo doc:
//   - O título de prompt usa a primeira linha útil do prompt, sem quebra de linha, com teto de
//     80 caracteres (o mesmo teto de título de observação).
//   - A lista de prompts usa os mesmos prompts válidos que a contagem do resumo (sem vazio e sem
//     markup do harness).
//   - Contagem de chamadas por nome no corpo: só pós-ferramenta, como no resumo. Rótulos de
//     família aparecem pelo nome; chamada sem nome aparece como `(unnamed)`.
//   - Duração com menos de 1 segundo (inteiro) é omitida.
//   - Resumo só com duração (sem prompt nem ferramenta) é `over <dur>.`, seguindo o formato do doc.
//   - A nota de corte das observações conta as omitidas no meio; as 250 primeiras e 250 últimas
//     ficam na ordem gravada.

// sessionPageDir é a pasta da página de sessão (LEI §10).
const sessionPageDir = "sessions/"

// sessionTitleFallback é o título quando não há observação alguma (LEI §10.1, item 5).
const sessionTitleFallback = "session"

// subagentPreamble é o preâmbulo de subagente (LEI §10.1, item 2).
const subagentPreamble = "You are a subagent spawned by another session."

// sessionTitleMaxRunes é o teto de caracteres de título e de linha de observação.
const sessionTitleMaxRunes = sanitize.ObservationTitleMaxRunes

// rawObsFullLimit é o número de observações até o qual todas aparecem (LEI §10.3, item 5).
const rawObsFullLimit = 500

// rawObsHalf é quantas observações ficam no começo e no fim quando há corte (LEI §10.3).
const rawObsHalf = 250

// summaryNamedTools é o número de ferramentas nomeadas no resumo (LEI §10.2).
const summaryNamedTools = 3

// rawObsTimeLayout formata horário em UTC (LEI §10.3). O literal Z é texto, não token.
const rawObsTimeLayout = "2006-01-02T15:04:05Z"

// SessionInput são os dados para a página de sessão: o identificador, a origem da sessão
// (agent, imutável) e as observações normalizadas, na ordem em que foram gravadas.
type SessionInput struct {
	Session      domain.SessionID
	Agent        string
	Observations []ingest.Event
}

// SessionPage é a página de sessão sintetizada (LEI §10).
//
// Path é o caminho relativo `sessions/<id>.md`. Summary é vazio quando não há o que dizer; nesse
// caso a chave summary não entra no frontmatter. Frontmatter traz title, session_id, agent,
// tier (episodic) e summary (opcional). A autoria não entra aqui: quem grava a página a atribui
// ao dono da sessão. Body é o markdown sem frontmatter.
type SessionPage struct {
	Path        string
	Title       string
	Summary     string
	Frontmatter domain.Frontmatter
	Body        string
}

// SynthesizeSessionPage monta a página de sessão a partir das observações normalizadas
// (LEI §10). Função pura e determinística: a mesma entrada produz a mesma saída.
func SynthesizeSessionPage(in SessionInput) SessionPage {
	id := in.Session.String()
	prompts := userPrompts(in.Observations)
	title := sessionTitle(in.Observations, prompts, id)
	summary := sessionSummary(in.Observations, prompts)

	return SessionPage{
		Path:        sessionPageDir + id + ".md",
		Title:       title,
		Summary:     summary,
		Frontmatter: sessionFrontmatter(in.Agent, id, title, summary),
		Body:        sessionBody(in.Observations, prompts, title, id),
	}
}

// sessionFrontmatter monta title, session_id, agent, tier e summary (opcional) (LEI §10).
func sessionFrontmatter(agent, id, title, summary string) domain.Frontmatter {
	var fm domain.Frontmatter
	setString(&fm, "title", title)
	setString(&fm, "session_id", id)
	setString(&fm, "agent", agent)
	setString(&fm, "tier", string(domain.TierEpisodic))
	if summary != "" {
		setString(&fm, "summary", summary)
	}
	return fm
}

// setString grava um texto no frontmatter. Set só recusa chave vazia e tipo não aceito, e aqui
// nenhum dos dois ocorre.
func setString(fm *domain.Frontmatter, key, value string) {
	_ = fm.Set(key, value)
}

// --- Título (LEI §10.1) ----------------------------------------------------

// sessionTitle aplica a ordem de preferência de §10.1.
func sessionTitle(events []ingest.Event, prompts []string, id string) string {
	// Itens 1 e 2: o primeiro prompt válido decide. Preâmbulo de subagente não cai para outras
	// observações.
	if len(prompts) > 0 {
		first := prompts[0]
		if strings.HasPrefix(strings.TrimSpace(first), subagentPreamble) {
			if t := subagentTitle(first); t != "" {
				return t
			}
			return "Session " + id
		}
		if t := cleanLine(first); t != "" {
			return t
		}
	}
	// Item 5: sem observação alguma.
	if len(events) == 0 {
		return sessionTitleFallback
	}
	// Item 3: outro título útil de outro evento.
	if t := otherTitle(events); t != "" {
		return t
	}
	// Item 4.
	return "Session " + id
}

// subagentTitle devolve a primeira linha útil do preâmbulo de subagente, sem o próprio preâmbulo
// (LEI §10.1, item 2). Vazio quando não há linha útil.
func subagentTitle(text string) string {
	for _, line := range strings.Split(text, "\n") {
		rest := strings.TrimPrefix(strings.TrimSpace(line), subagentPreamble)
		if t := cleanLine(rest); t != "" {
			return t
		}
	}
	return ""
}

// otherTitle devolve o primeiro título útil de evento que não é prompt, pulando início de sessão,
// títulos iguais ao nome do tipo de evento e rótulos `tool <família>` (LEI §10.1, item 3).
// Prompts ficam de fora: quando esta função roda, nenhum prompt válido existe.
func otherTitle(events []ingest.Event) string {
	for _, ev := range events {
		if ev.Hook == domain.HookSessionStart || ev.Hook == domain.HookUserPrompt {
			continue
		}
		t := cleanLine(ev.Title)
		if t == "" || isEventTypeName(t, ev.Hook) || isToolFamilyTitle(t) {
			continue
		}
		return t
	}
	return ""
}

// isEventTypeName informa se o título tem o mesmo nome do tipo do evento, na forma canônica ou
// na de observação. Caixa, `-`, `_` e espaços são ignorados.
func isEventTypeName(title string, hook domain.HookEvent) bool {
	n := normName(title)
	return n == normName(string(hook)) || n == normName(string(hook.ObservationKind()))
}

// normName normaliza um nome para comparação: minúsculas, sem `-`, `_` e espaços.
func normName(s string) string {
	return strings.ToLower(strings.NewReplacer("-", "", "_", "", " ", "").Replace(s))
}

// isToolFamilyTitle informa se o título é `tool <família>` (LEI §3.5 e §10.1).
func isToolFamilyTitle(title string) bool {
	rest, ok := strings.CutPrefix(strings.ToLower(title), "tool ")
	if !ok {
		return false
	}
	for _, f := range toolFamilyLabels {
		if rest == string(f) {
			return true
		}
	}
	return false
}

// --- Resumo (LEI §10.2) ----------------------------------------------------

// sessionSummary monta a linha de resumo de §10.2. Vazio quando nada cabe.
func sessionSummary(events []ingest.Event, prompts []string) string {
	var parts []string
	if n := len(prompts); n > 0 {
		parts = append(parts, plural(n, "prompt", "prompts"))
	}

	calls, counts := toolUsage(events)
	if calls > 0 {
		clause := plural(calls, "completed tool call", "completed tool calls")
		if names := toolNameClause(counts); names != "" {
			clause += " across " + names
		}
		parts = append(parts, clause)
	}

	if d, ok := sessionSpan(events); ok {
		parts = append(parts, "over "+formatDuration(d))
	}

	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ", ") + "."
}

// toolUsage conta as chamadas concluídas (só pós-ferramenta, LEI §10.2) e as separa por nome.
// O total inclui rótulos de família e chamadas sem nome. A chave "" é chamada sem nome.
func toolUsage(events []ingest.Event) (int, map[string]int) {
	total := 0
	counts := make(map[string]int)
	for _, ev := range events {
		if ev.Hook != domain.HookPostToolUse {
			continue
		}
		total++
		counts[cleanLine(ev.ToolName)]++
	}
	return total, counts
}

// toolNameClause nomeia as três ferramentas mais usadas, por volume e com desempate alfabético,
// seguidas de `and <K> more` quando houver mais. Rótulos de família e chamadas sem nome não são
// nomeados. Vazio quando não sobra nome.
func toolNameClause(counts map[string]int) string {
	type named struct {
		name  string
		count int
	}
	var all []named
	for name, count := range counts {
		if name == "" || isFamilyLabel(name) {
			continue
		}
		all = append(all, named{name, count})
	}
	if len(all) == 0 {
		return ""
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].count != all[j].count {
			return all[i].count > all[j].count
		}
		return all[i].name < all[j].name
	})

	var items []string
	for i := 0; i < len(all) && i < summaryNamedTools; i++ {
		items = append(items, all[i].name)
	}
	if extra := len(all) - len(items); extra > 0 {
		items = append(items, fmt.Sprintf("%d more", extra))
	}
	return joinList(items)
}

// joinList junta itens como `A`, `A and B` e `A, B and C`.
func joinList(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

// sessionSpan devolve a duração entre a primeira e a última observação com horário (LEI §10.2).
// Só vale quando tem pelo menos 1 segundo inteiro.
func sessionSpan(events []ingest.Event) (time.Duration, bool) {
	first, last, ok := bounds(events)
	if !ok {
		return 0, false
	}
	d := last.Sub(first)
	if int64(d/time.Second) < 1 {
		return 0, false
	}
	return d, true
}

// bounds devolve o menor e o maior horário das observações, ignorando horário zero.
func bounds(events []ingest.Event) (time.Time, time.Time, bool) {
	var first, last time.Time
	found := false
	for _, ev := range events {
		if ev.OccurredAt.IsZero() {
			continue
		}
		if !found || ev.OccurredAt.Before(first) {
			first = ev.OccurredAt
		}
		if !found || ev.OccurredAt.After(last) {
			last = ev.OccurredAt
		}
		found = true
	}
	return first.UTC(), last.UTC(), found
}

// formatDuration formata em `<n>s`, `<n>m`, `<h>h` ou `<h>h <m>m` (LEI §10.2). Trunca os segundos.
func formatDuration(d time.Duration) string {
	secs := int64(d / time.Second)
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	mins := secs / 60
	if mins < 60 {
		return fmt.Sprintf("%dm", mins)
	}
	hours, rem := mins/60, mins%60
	if rem == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh %dm", hours, rem)
}

// plural devolve `<n> <palavra>` no singular quando n é 1 e no plural nos demais casos.
func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// --- Corpo (LEI §10.3) -----------------------------------------------------

// sessionBody monta o markdown da página, na ordem de §10.3.
func sessionBody(events []ingest.Event, prompts []string, title, id string) string {
	var b strings.Builder
	b.WriteString("# " + title + "\n\n")

	b.WriteString("## Session metadata\n\n")
	b.WriteString("- Session: " + id + "\n")
	if first, last, ok := bounds(events); ok {
		b.WriteString("- Started: " + first.Format(rawObsTimeLayout) + "\n")
		b.WriteString("- Ended: " + last.Format(rawObsTimeLayout) + "\n")
	}
	b.WriteString(fmt.Sprintf("- Observations: %d\n", len(events)))

	if len(prompts) > 0 {
		b.WriteString("\n## Prompts\n\n")
		for i, p := range prompts {
			b.WriteString(fmt.Sprintf("%d. %s\n", i+1, cleanLine(p)))
		}
	}

	if calls, counts := toolUsage(events); calls > 0 {
		b.WriteString("\n## Tool calls\n\n")
		for _, c := range sortedCounts(counts) {
			name := c.name
			if name == "" {
				name = "(unnamed)"
			}
			b.WriteString(fmt.Sprintf("- %s: %d\n", name, c.count))
		}
	}

	if len(events) > 0 {
		b.WriteString("\n## Raw observations\n\n")
		for _, line := range rawObservationLines(events) {
			b.WriteString(line + "\n")
		}
	}
	return b.String()
}

// nameCount é uma contagem por nome de ferramenta.
type nameCount struct {
	name  string
	count int
}

// sortedCounts ordena por contagem decrescente e, no empate, por nome.
func sortedCounts(counts map[string]int) []nameCount {
	out := make([]nameCount, 0, len(counts))
	for name, count := range counts {
		out = append(out, nameCount{name, count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].count != out[j].count {
			return out[i].count > out[j].count
		}
		return out[i].name < out[j].name
	})
	return out
}

// rawObservationLines monta uma linha por observação: tipo, horário UTC e título até 80
// caracteres. Acima de 500 observações, mantém as 250 primeiras e as 250 últimas, com nota da
// quantidade omitida no meio (LEI §10.3, item 5).
func rawObservationLines(events []ingest.Event) []string {
	n := len(events)
	if n <= rawObsFullLimit {
		out := make([]string, 0, n)
		for _, ev := range events {
			out = append(out, rawObservationLine(ev))
		}
		return out
	}

	out := make([]string, 0, 2*rawObsHalf+1)
	for _, ev := range events[:rawObsHalf] {
		out = append(out, rawObservationLine(ev))
	}
	omitted := n - 2*rawObsHalf
	out = append(out, fmt.Sprintf("- _%s omitted from the middle: %d recorded, showing the first %d and the last %d._",
		plural(omitted, "observation", "observations"), n, rawObsHalf, rawObsHalf))
	for _, ev := range events[n-rawObsHalf:] {
		out = append(out, rawObservationLine(ev))
	}
	return out
}

// rawObservationLine formata `- <tipo> <horário UTC> <título>`.
func rawObservationLine(ev ingest.Event) string {
	line := "- " + string(ev.Hook.ObservationKind()) + " " + ev.OccurredAt.UTC().Format(rawObsTimeLayout)
	if t := cleanLine(ev.Title); t != "" {
		line += " " + t
	}
	return line
}

// --- Texto de linha ----------------------------------------------------------

// cleanLine sanitiza o texto, usa a primeira linha útil e limita a 80 caracteres numa só linha.
// Sanitiza antes de cortar (INV-06, sanitize). Vazio quando não sobra texto.
func cleanLine(raw string) string {
	clean := sanitize.Body(raw, sanitize.Limits{}).Text()
	line := firstUsefulLine(clean)
	capped := sanitize.Title(line, sanitize.Limits{TitleMaxRunes: sessionTitleMaxRunes}).Text()
	return strings.Join(strings.Fields(capped), " ")
}

// firstUsefulLine devolve a primeira linha com conteúdo, aparada.
func firstUsefulLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return ""
}
