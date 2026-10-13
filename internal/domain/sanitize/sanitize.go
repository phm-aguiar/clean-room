// Package sanitize é a fronteira de privacidade do texto que entra no armazenamento (DV §7,
// INV-06). Nada chega ao disco sem passar por aqui.
//
// Pipeline, nesta ordem (DV §7.1, ADR-10, ADR-11):
//  1. remoção de sequências de escape de terminal e de caracteres de controle (C0 e C1 exceto
//     \t \n \r), DEL, marcadores bidirecionais e bytes UTF-8 inválidos;
//  2. padrões de segredo (rules), cada trecho casado vira `[REDACTED:<rótulo>]`;
//  3. limite de tamanho (título em runes, corpo em bytes com cabeça e cauda).
//
// A redação roda ANTES do limite: um segredo cortado ao meio não casaria com padrão algum e
// iria para o armazenamento. O limite só corta depois que o segredo já virou marcador.
//
// Tipo-fronteira: `Sanitized` tem campo privado. Só as funções deste pacote (Title e Body)
// criam um valor com conteúdo. A escrita de observação (T035) só aceita `Sanitized`, então
// não existe caminho do texto bruto até o disco. Observação: o zero value `Sanitized{}` é
// texto vazio, que não contém segredo.
package sanitize

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Limits são os tetos de tamanho de uma observação (ADR-10). Zero significa sem teto.
// Não há campos de allowlist nem de extra_patterns (ADR-13, adiados).
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

// Title sanitiza um título e aplica o teto em runes. Título cortado termina em `…`, que conta
// dentro do teto.
func Title(raw string, lim Limits) Sanitized {
	return Sanitized{text: process(raw, func(s string) string {
		return truncateTitle(s, lim.TitleMaxRunes)
	})}
}

// Body sanitiza um corpo e aplica o teto em bytes com cabeça e cauda (ADR-11).
func Body(raw string, lim Limits) Sanitized {
	return Sanitized{text: process(raw, func(s string) string {
		return truncateBody(s, lim.BodyMaxBytes)
	})}
}

// maxPasses limita as passagens de convergência de process. A saída de uma passagem é a
// entrada da seguinte até estabilizar. Isso garante que Title(Title(x)) == Title(x) mesmo
// quando o corte cria uma nova fronteira de palavra para um padrão.
const maxPasses = 8

// process aplica remoção de controle uma vez e depois, até convergir, redação seguida de limite.
// Cada saída passa pelo limite, então o resultado sempre respeita o teto.
func process(raw string, limit func(string) string) string {
	s := stripControl(raw)
	for i := 0; i < maxPasses; i++ {
		next := limit(redact(s))
		if next == s {
			break
		}
		s = next
	}
	return s
}

// escapeSeq casa sequências de escape de terminal: CSI (ESC [ ou o C1 U+009B), OSC terminado
// em BEL ou ST, e ESC seguido de um caractere final. O resto de um escape que não casar cai
// na filtragem de runes.
var escapeSeq = regexp.MustCompile(
	`\x1b\[[0-?]*[ -/]*[@-~]|\x{9b}[0-?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[ -/]*[0-~]`,
)

// stripControl remove escapes de terminal, controles, marcadores bidirecionais e bytes UTF-8
// inválidos. Preserva \t \n \r e qualquer rune válido que não esteja na lista.
func stripControl(s string) string {
	s = escapeSeq.ReplaceAllString(s, "")
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
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

// isStrippedRune diz se uma rune válida deve sair do texto (DV §7.1, etapa 1).
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
	case r >= 0x200b && r <= 0x200f: // marcas de espaço e direção
		return true
	case r >= 0x202a && r <= 0x202e: // embeddings e overrides bidirecionais
		return true
	case r >= 0x2060 && r <= 0x2069: // isolates e joiners
		return true
	case r == 0xfeff: // BOM / ZWNBSP
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
