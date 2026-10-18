// Package sanitize é a fronteira de privacidade do texto que entra no armazenamento (DV §7,
// prompts §11, INV-06). Nada chega ao disco sem passar por aqui.
//
// Pipeline, nesta ordem (prompts §11.1, DV §7.1, ADR-10, ADR-11):
//  1. remoção de sequências de escape de terminal e de caracteres de controle da lista do §11.1
//     (C0 exceto \t \n \r, DEL, C1 e sobrescritas bidirecionais), além de bytes UTF-8 inválidos;
//  2. padrões de segredo (§11.2, na ordem da tabela) seguidos dos extras do operador (rótulo
//     `custom`); cada trecho casado vira `[REDACTED:<rótulo>]`, exceto se contiver uma entrada da
//     allowlist (§11.3);
//  3. limite de tamanho (título em runes, corpo em bytes com cabeça e cauda).
//
// A redação roda ANTES do limite: um segredo cortado ao meio não casaria com padrão algum e
// iria para o armazenamento. O limite só corta depois que o segredo já virou marcador.
//
// Tipo-fronteira: `Sanitized` tem campo privado. Só as funções deste pacote (Title e Body, de
// Sanitizer ou pacote) criam um valor com conteúdo. A escrita de observação só aceita `Sanitized`,
// então não existe caminho do texto bruto até o disco. Observação: o zero value `Sanitized{}` é
// texto vazio, que não contém segredo.
package sanitize

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Limits são os tetos de tamanho de uma observação (ADR-10). Zero significa sem teto.
type Limits struct {
	// TitleMaxRunes é o teto do título em runes, reticências `…` (U+2026) incluídas.
	TitleMaxRunes int
	// BodyMaxBytes é o teto do corpo em bytes, marcador de truncamento incluído (ADR-11).
	BodyMaxBytes int
}

// ObservationTitleMaxRunes e ObservationBodyMaxBytes são os tetos de observação (INV-06).
const (
	ObservationTitleMaxRunes = 80
	ObservationBodyMaxBytes  = 16 * 1024
)

// Sanitized é um texto já sanitizado e limitado. O campo é privado: só Title e Body criam
// um valor com conteúdo.
type Sanitized struct {
	text string
}

// Text devolve o texto sanitizado.
func (s Sanitized) Text() string {
	return s.text
}

// Sanitizer aplica os padrões de segredo e a allowlist do operador (prompts §11.3, DV §7.3).
// É construído uma vez por New e pode ser copiado livremente. O zero value usa só os padrões
// embutidos, sem allowlist (decisão nossa: o doc não define o zero value).
type Sanitizer struct {
	rules []rule
	allow []string
}

// New monta o sanitizador com os padrões embutidos, seguidos dos extras do operador (rótulo
// `custom`, no fim da ordem), e com a allowlist. Expressão extra inválida, extra vazia ou entrada
// vazia na allowlist devolvem erro: quem chama deve impedir o início do serviço (DV §7.3).
func New(extra, allow []string) (Sanitizer, error) {
	rules := make([]rule, len(builtinRules), len(builtinRules)+len(extra))
	copy(rules, builtinRules)
	for i, p := range extra {
		if p == "" {
			return Sanitizer{}, fmt.Errorf("sanitize: extra_patterns[%d] vazio", i)
		}
		re, err := regexp.Compile(p)
		if err != nil {
			return Sanitizer{}, fmt.Errorf("sanitize: extra_patterns[%d]: %w", i, err)
		}
		rules = append(rules, rule{label: "custom", re: re})
	}
	for i, a := range allow {
		if a == "" {
			return Sanitizer{}, fmt.Errorf("sanitize: allowlist[%d] vazia liberaria todo o texto", i)
		}
	}
	return Sanitizer{rules: rules, allow: append([]string(nil), allow...)}, nil
}

// patterns devolve os padrões em uso. O zero value usa os embutidos.
func (s Sanitizer) patterns() []rule {
	if s.rules == nil {
		return builtinRules
	}
	return s.rules
}

// Title sanitiza um título e aplica o teto em runes. Título cortado termina em `…`, que conta
// dentro do teto.
func (s Sanitizer) Title(raw string, lim Limits) Sanitized {
	return Sanitized{text: s.process(raw, func(t string) string {
		return truncateTitle(t, lim.TitleMaxRunes)
	})}
}

// Body sanitiza um corpo e aplica o teto em bytes com cabeça e cauda (ADR-11).
func (s Sanitizer) Body(raw string, lim Limits) Sanitized {
	return Sanitized{text: s.process(raw, func(t string) string {
		return truncateBody(t, lim.BodyMaxBytes)
	})}
}

// Title sanitiza um título com os padrões embutidos, sem allowlist. Ver Sanitizer.Title.
func Title(raw string, lim Limits) Sanitized {
	return Sanitizer{}.Title(raw, lim)
}

// Body sanitiza um corpo com os padrões embutidos, sem allowlist. Ver Sanitizer.Body.
func Body(raw string, lim Limits) Sanitized {
	return Sanitizer{}.Body(raw, lim)
}

// maxPasses limita as passagens de convergência de process. A saída de uma passagem é a
// entrada da seguinte até estabilizar. Isso garante que Title(Title(x)) == Title(x) mesmo
// quando o corte cria uma nova fronteira de palavra para um padrão.
const maxPasses = 8

// process aplica remoção de controle uma vez e depois, até convergir, redação seguida de limite.
// Cada saída passa pelo limite, então o resultado sempre respeita o teto.
func (s Sanitizer) process(raw string, limit func(string) string) string {
	t := stripControl(raw)
	for i := 0; i < maxPasses; i++ {
		next := limit(s.redact(t))
		if next == t {
			break
		}
		t = next
	}
	return t
}

// redact aplica os padrões na ordem. Cada trecho casado vira `[REDACTED:<rótulo>]`, salvo se
// contiver uma entrada da allowlist. Casamento de tamanho zero é ignorado, para que um padrão
// vazio não insira marcadores entre os caracteres.
func (s Sanitizer) redact(text string) string {
	for _, r := range s.patterns() {
		text = r.re.ReplaceAllStringFunc(text, func(m string) string {
			if m == "" || s.allowed(m) {
				return m
			}
			return "[REDACTED:" + r.label + "]"
		})
	}
	return text
}

// allowed diz se o trecho casado contém alguma entrada da allowlist (prompts §11.3: checagem
// por ocorrência, não pelo texto inteiro).
func (s Sanitizer) allowed(m string) bool {
	for _, a := range s.allow {
		if strings.Contains(m, a) {
			return true
		}
	}
	return false
}

// escapeSeq casa as sequências de escape do §11.1, passo 1: CSI (ESC `[` com parâmetros e
// caractere final), OSC (ESC `]` até BEL ou ESC `\`) e ESC seguido de um caractere de `@` a `_`.
// São removidas inteiras.
var escapeSeq = regexp.MustCompile(
	`\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[@-_]`,
)

// stripControl remove as sequências de escape, os controles da lista do §11.1 e os bytes UTF-8
// inválidos. Preserva \t \n \r e qualquer outro rune válido.
func stripControl(s string) string {
	s = escapeSeq.ReplaceAllString(s, "")
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		// Bytes inválidos não são controle (§11.1), mas o corte da etapa 5 precisa de UTF-8
		// válido. Decisão nossa, registrada: o doc não define o tratamento deles.
		if r == utf8.RuneError && size == 1 {
			continue
		}
		if isStrippedRune(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// isStrippedRune diz se uma rune válida sai do texto (prompts §11.1, passo 2).
func isStrippedRune(r rune) bool {
	switch {
	case r == '\t' || r == '\n' || r == '\r':
		return false
	case r < 0x20: // C0
		return true
	case r == 0x7f: // DEL
		return true
	case r >= 0x80 && r <= 0x9f: // C1
		return true
	case r >= 0x202a && r <= 0x202e: // sobrescritas bidirecionais
		return true
	case r >= 0x2066 && r <= 0x2069: // isolates bidirecionais
		return true
	}
	return false
}

// truncateTitle mantém até maxRunes runes. Quando corta, a última posição vira `…`.
// maxRunes <= 0 significa sem teto.
func truncateTitle(s string, maxRunes int) string {
	if maxRunes <= 0 || utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes-1]) + "…"
}

// bodyHeadTailReserve é o orçamento reservado ao marcador do corte de cabeça e cauda (ADR-11).
const bodyHeadTailReserve = 48

// truncateBody limita s a maxBytes bytes. Se cortar, mantém metade do orçamento útil no início
// e metade no fim, com marcador `\n...[truncated N bytes]...\n` no meio. Cada ponta é reduzida
// até a fronteira UTF-8. Com teto menor que 64 bytes, corta só a cabeça. maxBytes <= 0 é sem teto.
//
// Garantias: len(saída) <= maxBytes e a saída é UTF-8 válido (a entrada já passou por stripControl).
func truncateBody(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}
	if maxBytes < 64 {
		return truncateHeadOnly(s, maxBytes)
	}
	budget := maxBytes - bodyHeadTailReserve
	headB := budget / 2
	tailB := budget - headB

	h := runeStartAtOrBefore(s, headB)
	t := runeStartAtOrAfter(s, len(s)-tailB)
	marker := truncationMarker(t - h)
	return s[:h] + marker + s[t:]
}

// truncateHeadOnly corta só a cabeça e acrescenta o marcador, sem exceder maxBytes.
func truncateHeadOnly(s string, maxBytes int) string {
	reserve := len(truncationMarker(len(s)))
	if maxBytes <= reserve {
		return s[:runeStartAtOrBefore(s, maxBytes)]
	}
	h := runeStartAtOrBefore(s, maxBytes-reserve)
	return s[:h] + truncationMarker(len(s)-h)
}

// truncationMarker monta `\n...[truncated N bytes]...\n`, onde N é o número de bytes omitidos.
func truncationMarker(omitted int) string {
	return fmt.Sprintf("\n...[truncated %d bytes]...\n", omitted)
}

// runeStartAtOrBefore devolve o maior índice <= i que começa uma sequência UTF-8 (ou len(s)).
func runeStartAtOrBefore(s string, i int) int {
	if i >= len(s) {
		return len(s)
	}
	for i > 0 && !utf8.RuneStart(s[i]) {
		i--
	}
	return i
}

// runeStartAtOrAfter devolve o menor índice >= i que começa uma sequência UTF-8 (ou len(s)).
func runeStartAtOrAfter(s string, i int) int {
	for i < len(s) && !utf8.RuneStart(s[i]) {
		i++
	}
	return i
}
