// Package synth sintetiza, sem LLM, o texto do handoff automático de fim de sessão e de
// checkpoint de turno (LEI §9). A função é pura: não lê arquivo, não faz I/O e não chama modelo.
//
// Fontes: docs/lifecycle-event-ingestion.md §9.1 a §9.3 (vence sobre a spec e o plan) e
// specs/features/002-captura-e-acesso/notes/handoff-contract.md §6.2 a §6.4.
//
// Decisões nossas, não fixadas pelo doc:
//   - Prompt de confirmação é uma lista fechada de frases curtas (ver confirmations). Sem lista
//     não há como distinguir "ok" de um pedido de até 20 caracteres.
//   - Markup do harness é texto que começa com uma tag `<nome ...>` e termina com `>`.
//   - Os textos de prompt passam por sanitize.Body antes do corte, como manda LEI §3.6
//     (sanitização antes dos limites). O corte conta 1500 runes de conteúdo e acrescenta `…`.
//   - Em §9.3, a regra 1 só casa se o último prompt não markup for pergunta e não for confirmação.
//     As regras 3 e 4 exigem um prompt substantivo para terem texto a mostrar.
package synth

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
	"github.com/phm-aguiar/ai-memory-go/internal/domain/sanitize"
	"github.com/phm-aguiar/ai-memory-go/internal/ingest"
)

// Mode separa o handoff de fim de sessão do checkpoint de turno (LEI §9.1 e §9.3).
type Mode int

const (
	// ModeSessionEnd é o handoff automático de fim de sessão.
	ModeSessionEnd Mode = iota
	// ModeTurnCheckpoint é o checkpoint de turno (só uma regra de perguntas em aberto).
	ModeTurnCheckpoint
)

// Input são as observações normalizadas da sessão, na ordem em que foram gravadas, e o modo.
type Input struct {
	Mode         Mode
	Observations []ingest.Event
}

// Handoff é o texto estruturado do handoff automático. Summary pode ter quebra de linha
// (formato "Started: ...", linha em branco, "Last: ..."). NextSteps é vazio quando nenhuma
// ferramenta foi usada. OpenQuestions é vazio quando nenhuma regra casa.
type Handoff struct {
	Summary       string
	NextSteps     string
	OpenQuestions []string
}

// promptMaxRunes é o limite de cada texto de prompt, em caracteres, antes do `…` (LEI §9.1).
const promptMaxRunes = 1500

// ellipsis é o marcador de corte de prompt longo (LEI §9.1).
const ellipsis = "…"

// confirmationMaxRunes é o teto de caracteres de um prompt de confirmação (LEI §9.3).
const confirmationMaxRunes = 20

// Textos fixos de §9.1 a §9.3. Os formatos com %d seguem o doc.
const (
	summaryFocusedPrefix = "Session focused on: "
	summaryStartedPrefix = "Started: "
	summaryLastPrefix    = "Last: "
	summaryEndedFmt      = "Session ended; %d observations recorded."
	summaryCheckpointFmt = "Turn checkpoint; %d observations recorded."

	nextStepsPrefix = "Tools used: "

	openUnresolvedPrefix = "Unresolved question: "
	openNoStopFiles      = "Session ended without a normal stop while working with files"
	openCheckTree        = "Check the working tree for uncommitted or unverified changes"
	openMidTaskPrefix    = "Continue from (mid-task exit): "
	openContinuePrefix   = "Continue from: "
	openCheckpointPrefix = "Continue from last request: "
)

// confirmations é a lista fechada de frases de confirmação (LEI §9.3). A comparação ignora caixa
// e pontuação final.
var confirmations = map[string]bool{
	"ok":          true,
	"okay":        true,
	"k":           true,
	"thanks":      true,
	"thank you":   true,
	"thx":         true,
	"got it":      true,
	"understood":  true,
	"sure":        true,
	"yes":         true,
	"yep":         true,
	"yeah":        true,
	"cool":        true,
	"great":       true,
	"perfect":     true,
	"nice":        true,
	"sounds good": true,
	"makes sense": true,
	"好的":          true,
	"谢谢":          true,
	"收到":          true,
	"明白":          true,
	"明白了":         true,
	"是的":          true,
	"嗯":           true,
}

// markupPattern casa o início de um bloco de markup do harness: `<nome` seguido de espaço, `/`
// ou `>`. Combinado com o `>` final, identifica o bloco entregue como turno do usuário.
var markupPattern = regexp.MustCompile(`^<[A-Za-z][A-Za-z0-9_:-]*[\s/>]`)

// Synthesize monta o texto do handoff a partir das observações normalizadas (LEI §9).
// Função pura e determinística: a mesma entrada produz a mesma saída.
func Synthesize(in Input) Handoff {
	prompts := userPrompts(in.Observations)
	h := Handoff{
		Summary: summary(in.Mode, prompts, len(in.Observations)),
	}
	h.NextSteps = nextSteps(in.Observations)
	h.OpenQuestions = openQuestions(in.Mode, in.Observations, prompts)
	return h
}

// userPrompts devolve os textos de prompt do usuário, na ordem, sem markup do harness e sem
// textos vazios. Cada texto está sanitizado e aparado, mas ainda não cortado.
func userPrompts(events []ingest.Event) []string {
	var out []string
	for _, ev := range events {
		if ev.Hook != domain.HookUserPrompt {
			continue
		}
		raw := ev.Body
		if raw == "" {
			raw = ev.Title
		}
		text := strings.TrimSpace(sanitize.Body(raw, sanitize.Limits{}).Text())
		if text == "" || isMarkup(text) {
			continue
		}
		out = append(out, text)
	}
	return out
}

// isMarkup informa se o texto aparado é um bloco de markup do harness (LEI §9.1).
func isMarkup(text string) bool {
	return markupPattern.MatchString(text) && strings.HasSuffix(text, ">")
}

// cutPrompt aplica o limite de 1500 caracteres. Texto mais longo fica com 1500 runes e `…`.
func cutPrompt(text string) string {
	if utf8.RuneCountInString(text) <= promptMaxRunes {
		return text
	}
	return string([]rune(text)[:promptMaxRunes]) + ellipsis
}

// summary monta o resumo de §9.1.
func summary(mode Mode, prompts []string, recorded int) string {
	switch len(prompts) {
	case 0:
		if mode == ModeTurnCheckpoint {
			return fmt.Sprintf(summaryCheckpointFmt, recorded)
		}
		return fmt.Sprintf(summaryEndedFmt, recorded)
	case 1:
		return summaryFocusedPrefix + cutPrompt(prompts[0])
	default:
		first := prompts[0]
		last := prompts[len(prompts)-1]
		return summaryStartedPrefix + cutPrompt(first) + "\n\n" + summaryLastPrefix + cutPrompt(last)
	}
}

// toolFamilyLabels são os rótulos de família que não dizem qual ferramenta foi usada (LEI §9.2).
var toolFamilyLabels = []ingest.ToolFamily{
	ingest.ToolFamilyFile,
	ingest.ToolFamilySearchList,
	ingest.ToolFamilyNonFile,
	ingest.ToolFamilyUnknown,
}

// nextSteps monta a linha `Tools used: <nomes>` de §9.2: nomes distintos em ordem alfabética,
// sem rótulos de família. Devolve vazio quando nenhum nome sobra.
func nextSteps(events []ingest.Event) string {
	seen := make(map[string]bool)
	for _, ev := range events {
		if ev.Hook != domain.HookPreToolUse && ev.Hook != domain.HookPostToolUse {
			continue
		}
		name := strings.TrimSpace(sanitize.Body(ev.ToolName, sanitize.Limits{}).Text())
		if name == "" || isFamilyLabel(name) {
			continue
		}
		seen[name] = true
	}
	if len(seen) == 0 {
		return ""
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	return nextStepsPrefix + strings.Join(names, ", ")
}

// isFamilyLabel informa se o nome é um rótulo de família, com ou sem o prefixo `tool `.
func isFamilyLabel(name string) bool {
	lower := strings.ToLower(name)
	for _, f := range toolFamilyLabels {
		label := string(f)
		if lower == label || lower == "tool "+label {
			return true
		}
	}
	return false
}

// openQuestions aplica §9.3. No checkpoint de turno só vale a regra de última solicitação.
// Em fim de sessão, a primeira regra que casar vence e as demais são ignoradas.
func openQuestions(mode Mode, events []ingest.Event, prompts []string) []string {
	substantive, hasSubstantive := lastSubstantive(prompts)

	if mode == ModeTurnCheckpoint {
		if !hasSubstantive {
			return nil
		}
		return []string{openCheckpointPrefix + cutPrompt(substantive)}
	}

	hasStop, hasEnd := false, false
	var lastToolFamily ingest.ToolFamily
	hasTool := false
	for _, ev := range events {
		switch ev.Hook.ObservationKind() {
		case domain.ObservationStop:
			hasStop = true
		case domain.ObservationSessionEnd:
			hasEnd = true
		}
		if ev.Hook == domain.HookPreToolUse || ev.Hook == domain.HookPostToolUse {
			lastToolFamily = ev.ToolFamily
			hasTool = true
		}
	}

	// Regra 1: último prompt do usuário (sem markup) termina com `?` ou `？`, e não é confirmação.
	if len(prompts) > 0 {
		last := prompts[len(prompts)-1]
		if endsWithQuestion(last) && !isConfirmation(last) {
			return []string{openUnresolvedPrefix + cutPrompt(last)}
		}
	}

	// Regra 2: última ferramenta da família de arquivo e nenhum evento de parada.
	if hasTool && lastToolFamily == ingest.ToolFamilyFile && !hasStop {
		return []string{openNoStopFiles, openCheckTree}
	}

	// Regra 3: há parada, mas não há fim de sessão.
	if hasSubstantive && hasStop && !hasEnd {
		return []string{openMidTaskPrefix + cutPrompt(substantive)}
	}

	// Regra 4: fim normal.
	if hasSubstantive {
		return []string{openContinuePrefix + cutPrompt(substantive)}
	}

	// Regra 5: nenhuma das anteriores.
	return nil
}

// lastSubstantive devolve o último prompt que não é confirmação (LEI §9.3).
func lastSubstantive(prompts []string) (string, bool) {
	for i := len(prompts) - 1; i >= 0; i-- {
		if !isConfirmation(prompts[i]) {
			return prompts[i], true
		}
	}
	return "", false
}

// endsWithQuestion informa se o último caractere do texto aparado é `?` ou `？`.
func endsWithQuestion(text string) bool {
	return strings.HasSuffix(text, "?") || strings.HasSuffix(text, "？")
}

// isConfirmation informa se o prompt é de confirmação: vazio ou só pontuação, ou uma frase da
// lista fechada com até 20 caracteres, sozinha ou seguida só de pontuação (LEI §9.3).
func isConfirmation(text string) bool {
	t := strings.TrimSpace(text)
	if utf8.RuneCountInString(t) > confirmationMaxRunes {
		return false
	}
	core := strings.TrimRightFunc(t, func(r rune) bool {
		return unicode.IsPunct(r) || unicode.IsSpace(r)
	})
	core = strings.ToLower(strings.TrimSpace(core))
	if core == "" {
		return true
	}
	return confirmations[core]
}
