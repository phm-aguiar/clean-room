package domain

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// ErrInvalidPagePath é a causa de todo erro devolvido pelos construtores de PagePath.
var ErrInvalidPagePath = errors.New("caminho de página inválido")

// PagePathError descreve qual regra o caminho violou.
type PagePathError struct {
	Input string
	Rule  string
}

func (e *PagePathError) Error() string {
	return fmt.Sprintf("%s (%s): %q", ErrInvalidPagePath.Error(), e.Rule, e.Input)
}

// Unwrap permite errors.Is(err, ErrInvalidPagePath).
func (e *PagePathError) Unwrap() error {
	return ErrInvalidPagePath
}

// PagePath é o caminho relativo de uma página dentro da árvore da wiki, com separador POSIX.
// Só é criado pelos construtores ParsePagePath e ParseStoredPagePath.
type PagePath struct {
	value string
}

// String devolve o caminho como texto (separador POSIX).
func (p PagePath) String() string {
	return p.value
}

// ParsePagePath é o construtor estrito, para caminho novo. Aplica as regras de DV §4.2 e,
// além delas, a portabilidade (ADR-04) e a lista de segmentos reservados (ADR-05).
// Não aceita `\`: quem lê do disco converte com filepath.ToSlash antes (ADR-02).
func ParsePagePath(s string) (PagePath, error) {
	if err := checkStructure(s); err != nil {
		return PagePath{}, err
	}
	segs := strings.Split(s, "/")
	for _, seg := range segs {
		if rule := portableViolation(seg); rule != "" {
			return PagePath{}, &PagePathError{Input: s, Rule: rule}
		}
	}
	if rule := reservedViolation(segs); rule != "" {
		return PagePath{}, &PagePathError{Input: s, Rule: rule}
	}
	return PagePath{value: s}, nil
}

// ParseStoredPagePath é o construtor de leitura, para linhas já persistidas (ADR-03).
// Aplica só as regras de DV §4.2: sem portabilidade e sem lista de reservados.
func ParseStoredPagePath(s string) (PagePath, error) {
	if err := checkStructure(s); err != nil {
		return PagePath{}, err
	}
	return PagePath{value: s}, nil
}

// PortableKey devolve a chave de colisão de caminhos (DV §4.2, ADR-06): minúsculas com
// strings.ToLower e, depois, normalização Unicode NFC. A ordem importa. Não faz I/O.
func (p PagePath) PortableKey() string {
	return norm.NFC.String(strings.ToLower(p.value))
}

// SearchText devolve o texto de busca do caminho, o valor de `path_search` (DB §1):
// troca `/` e `.` por espaço (segmentos); depois troca também `-` e `_` por espaço
// nos segmentos (palavras); devolve segmentos + " " + palavras. Sem trim e sem colapso de espaços.
func (p PagePath) SearchText() string {
	segmentos := strings.NewReplacer("/", " ", ".", " ").Replace(p.value)
	palavras := strings.NewReplacer("-", " ", "_", " ").Replace(segmentos)
	return segmentos + " " + palavras
}

// checkStructure aplica as regras de DV §4.2, comuns aos dois construtores.
// Devolve nil ou *PagePathError.
func checkStructure(s string) error {
	fail := func(rule string) error {
		return &PagePathError{Input: s, Rule: rule}
	}
	if s == "" {
		return fail("vazio")
	}
	if s[0] == '/' {
		return fail("caminho absoluto")
	}
	if len(s) >= 2 && isASCIILetter(s[0]) && s[1] == ':' {
		return fail("prefixo de unidade Windows")
	}
	if strings.IndexByte(s, '\\') >= 0 {
		return fail("barra invertida como separador")
	}
	for _, seg := range strings.Split(s, "/") {
		switch seg {
		case "":
			return fail("segmento vazio")
		case ".":
			return fail("segmento .")
		case "..":
			return fail("segmento ..")
		}
	}
	return nil
}

// portableViolation devolve a regra violada por um segmento (ADR-04), ou "" se é portável.
// O segmento já passou por checkStructure, então não é vazio.
func portableViolation(seg string) string {
	for i := 0; i < len(seg); i++ {
		c := seg[i]
		if c < 0x20 || strings.IndexByte(`<>:"|?*`, c) >= 0 {
			return "caractere inválido no nome portável"
		}
	}
	if last := seg[len(seg)-1]; last == '.' || last == ' ' {
		return "ponto ou espaço no fim do nome"
	}
	stem := seg
	if i := strings.IndexByte(seg, '.'); i >= 0 {
		stem = seg[:i]
	}
	if isWindowsDeviceName(strings.ToUpper(stem)) {
		return "nome reservado do Windows"
	}
	return ""
}

// reservedViolation devolve a regra violada pela lista de segmentos reservados (ADR-05),
// ou "" se nenhum segmento é reservado. Comparação sem distinguir caixa, porque
// Windows e macOS também não distinguem.
func reservedViolation(segs []string) string {
	for _, seg := range segs {
		lower := strings.ToLower(seg)
		if lower == ".git" {
			return "segmento .git reservado"
		}
		if strings.HasPrefix(lower, ".ai-memory-tmp.") {
			return "segmento com prefixo .ai-memory-tmp. reservado"
		}
	}
	first := strings.ToLower(segs[0])
	if first == "_pending" || first == "_graveyard" {
		return "diretório reservado"
	}
	base := strings.ToLower(segs[len(segs)-1])
	switch base {
	case "_meta.md", "index.md", "bootstrap.md", "log.md":
		return "nome base reservado"
	}
	if isLogMonthly(base) {
		return "nome base reservado"
	}
	return ""
}

// isWindowsDeviceName recebe o radical já em maiúsculas.
func isWindowsDeviceName(upper string) bool {
	switch upper {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	if len(upper) == 4 && (strings.HasPrefix(upper, "COM") || strings.HasPrefix(upper, "LPT")) {
		return upper[3] >= '1' && upper[3] <= '9'
	}
	return false
}

// isLogMonthly reconhece log-AAAA-MM.md (base já em minúsculas).
func isLogMonthly(base string) bool {
	if len(base) != len("log-AAAA-MM.md") ||
		!strings.HasPrefix(base, "log-") ||
		!strings.HasSuffix(base, ".md") {
		return false
	}
	if base[8] != '-' {
		return false
	}
	for _, i := range []int{4, 5, 6, 7, 9, 10} {
		if base[i] < '0' || base[i] > '9' {
			return false
		}
	}
	return true
}

func isASCIILetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
