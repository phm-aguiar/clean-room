// Package capture implementa a política pura de captura por marcador
// (`.ai-memory.toml`, seção `[capture]`).
//
// A política recebe o conteúdo já lido do marcador e o evento já normalizado,
// e devolve Keep, Drop ou MetadataOnly. Não faz leitura de arquivo nem consulta
// variáveis de ambiente: quem faz a E/S passa o conteúdo e os diretórios.
//
// Fontes: docs/marker-file.md (MF) e docs/lifecycle-event-ingestion.md §4 (LEI).
// O contrato resumido está em specs/features/002-captura-e-acesso/notes/capture-contract.md.
package capture

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	ktoml "github.com/knadh/koanf/parsers/toml"
)

// Limites (MF "Bounds"; LEI §4.2).
const (
	MaxMarkerBytes     = 65536   // 64 KiB: tamanho máximo do marcador lido para [capture]
	MaxPatterns        = 128     // padrões por marcador
	MaxPatternChars    = 1024    // caracteres por padrão
	MaxCandidates      = 32      // caminhos candidatos diretos por evento de arquivo
	MaxCandidateChars  = 4096    // caracteres por candidato
	MaxComparisons     = 1000000 // comparações padrão x candidato por evento
	ProtocolVersion    = 1       // marca de protocolo do cliente e do servidor (LEI §4.4)
	maxSessionIDBytes  = 512
	maxCallIDBytes     = 512
	maxCwdBytes        = 16384
	maxToolNameLength  = 64
	captureSection     = "capture"
	captureIgnorePaths = "ignore_paths"
)

// State é o estado da política para o decisor (LEI §4.3).
type State int

const (
	// StateInactive: sem marcador em modo denylist, ou marcador sem ignore_paths.
	StateInactive State = iota
	// StateActive: marcador válido com pelo menos um padrão.
	StateActive
	// StateInvalid: marcador malformado. Falha fechado para arquivo e shell.
	StateInvalid
)

func (s State) String() string {
	switch s {
	case StateInactive:
		return "inactive"
	case StateActive:
		return "active"
	case StateInvalid:
		return "invalid"
	}
	return "unknown"
}

// Family é a família da ferramenta para fins de captura.
// A família de shell não aparece como rótulo no esquema de eventos: seu rótulo
// público é "non-file" (Label).
type Family int

const (
	FamilyUnknown Family = iota
	FamilyFile
	FamilySearchList
	FamilyShell
	FamilyNonFile
)

// Label devolve o rótulo de família usado nos títulos e corpos de evento (LEI §3.5).
func (f Family) Label() string {
	switch f {
	case FamilyFile:
		return "file"
	case FamilySearchList:
		return "search-list"
	case FamilyShell, FamilyNonFile:
		return "non-file"
	}
	return "unknown"
}

// Decision é o resultado da política.
type Decision int

const (
	Keep Decision = iota
	Drop
	MetadataOnly
)

func (d Decision) String() string {
	switch d {
	case Keep:
		return "keep"
	case Drop:
		return "drop"
	case MetadataOnly:
		return "metadata-only"
	}
	return "unknown"
}

// Policy é a política compilada a partir do conteúdo do marcador.
// O valor zero equivale a StateInactive.
type Policy struct {
	state    State
	home     string
	patterns []pattern
}

// State devolve o estado da política.
func (p Policy) State() State { return p.state }

// Pattern é um padrão já validado e resolvido para caminho absoluto.
type pattern struct {
	raw  string
	orig normPath
	low  normPath
}

// Event é o evento já normalizado que a política avalia.
//
// Candidates são os caminhos (arquivo) ou as palavras (shell) já extraídos pelo
// chamador, a partir apenas dos campos documentados. Para arquivo, Extracted
// deve ser false quando os campos de caminho estão ausentes, malformados ou não
// suportados. Essa checagem fica com quem extrai, não com a política.
type Event struct {
	SessionID  string
	Cwd        string
	Family     Family
	ToolName   string
	CallID     string
	Candidates []string
	Extracted  bool
}

// Result é a decisão e, para MetadataOnly, o corpo substituto.
type Result struct {
	Decision Decision
	Body     string
}

// Compile interpreta o conteúdo já lido do marcador mais próximo da caminhada
// de captura. markerContent nil ou vazio significa "sem marcador" e dá
// StateInactive. markerDir é o diretório do marcador (absoluto, usado pelos
// padrões relativos). homeDir é o diretório pessoal (usado por "~/").
func Compile(markerContent []byte, markerDir, homeDir string) Policy {
	p := Policy{state: StateInactive, home: slash(homeDir)}
	if len(markerContent) > MaxMarkerBytes || !utf8.Valid(markerContent) {
		p.state = StateInvalid
		return p
	}
	doc, err := ktoml.Parser().Unmarshal(markerContent)
	if err != nil {
		p.state = StateInvalid
		return p
	}
	raw, ok := doc[captureSection]
	if !ok {
		return p
	}
	table, ok := raw.(map[string]interface{})
	if !ok {
		p.state = StateInvalid
		return p
	}
	for key := range table {
		if key != captureIgnorePaths {
			p.state = StateInvalid
			return p
		}
	}
	rawList, ok := table[captureIgnorePaths]
	if !ok {
		return p
	}
	items, err := stringList(rawList)
	if err != nil {
		p.state = StateInvalid
		return p
	}
	if len(items) == 0 {
		return p
	}
	if len(items) > MaxPatterns {
		p.state = StateInvalid
		return p
	}
	compiled := make([]pattern, 0, len(items))
	for _, s := range items {
		pt, err := compilePattern(s, markerDir, p.home)
		if err != nil {
			p.state = StateInvalid
			return p
		}
		compiled = append(compiled, pt)
	}
	p.patterns = compiled
	p.state = StateActive
	return p
}

// Decide avalia um evento com o orçamento de comparações da LEI §4.2.
func (p Policy) Decide(ev Event) Result {
	return decide(p, ev, MaxComparisons)
}

func decide(p Policy, ev Event, budget int) Result {
	switch p.state {
	case StateInactive:
		return Result{Decision: Keep}
	case StateInvalid:
		if ev.Family == FamilyFile || ev.Family == FamilyShell {
			return metadataOnly(ev)
		}
		return Result{Decision: Keep}
	}

	switch ev.Family {
	case FamilySearchList:
		return Result{Decision: Drop}
	case FamilyFile:
		return decideFile(p, ev, budget)
	case FamilyShell:
		return decideShell(p, ev, budget)
	}
	return Result{Decision: Keep}
}

// decideFile: qualquer candidato que casa gera Drop. Candidato malformado ou
// excesso de candidatos gera MetadataOnly. Orçamento esgotado gera MetadataOnly.
func decideFile(p Policy, ev Event, budget int) Result {
	if !ev.Extracted || len(ev.Candidates) == 0 {
		return metadataOnly(ev)
	}
	malformed := false
	comparisons := 0
	for i, raw := range ev.Candidates {
		if i >= MaxCandidates {
			malformed = true
			break
		}
		c, ok := candidate(raw, ev.Cwd, p.home)
		if !ok {
			malformed = true
			continue
		}
		for _, pt := range p.patterns {
			comparisons++
			if comparisons > budget {
				return metadataOnly(ev)
			}
			if pt.match(c) {
				return Result{Decision: Drop}
			}
		}
	}
	if malformed {
		return metadataOnly(ev)
	}
	return Result{Decision: Keep}
}

// decideShell: palavras vazias são ignoradas. Palavra malformada, sem resolução
// ou acima do limite de caracteres gera Drop (falha fechado). Orçamento
// esgotado gera Drop. Casamento gera Drop.
func decideShell(p Policy, ev Event, budget int) Result {
	comparisons := 0
	for _, raw := range ev.Candidates {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		c, ok := candidate(raw, ev.Cwd, p.home)
		if !ok {
			return Result{Decision: Drop}
		}
		for _, pt := range p.patterns {
			comparisons++
			if comparisons > budget {
				return Result{Decision: Drop}
			}
			if pt.match(c) {
				return Result{Decision: Drop}
			}
		}
	}
	return Result{Decision: Keep}
}

// candidate valida o limite de tamanho e resolve um candidato para caminho absoluto.
func candidate(raw, cwd, home string) (candPath, bool) {
	if strings.TrimSpace(raw) == "" || !printable(raw) {
		return candPath{}, false
	}
	if utf8.RuneCountInString(raw) > MaxCandidateChars {
		return candPath{}, false
	}
	n, ok := resolve(raw, cwd, home)
	if !ok {
		return candPath{}, false
	}
	return candPath{orig: n, low: n.fold()}, true
}

// candPath é um candidato resolvido, com a forma original e a forma em caixa baixa.
type candPath struct {
	orig normPath
	low  normPath
}

func (p pattern) match(c candPath) bool {
	if p.orig.windows || c.orig.windows {
		return p.low.prefix == c.low.prefix && globSegments(p.low.segs, c.low.segs)
	}
	return p.orig.prefix == c.orig.prefix && globSegments(p.orig.segs, c.orig.segs)
}

// normPath é um caminho absoluto normalizado léxicamente.
// prefix é "/" para caminhos POSIX e UNC, ou a letra de unidade ("C:").
// windows marca caminhos de unidade e UNC, que casam sem distinguir caixa ASCII.
type normPath struct {
	prefix  string
	segs    []string
	windows bool
}

// fold devolve a forma em caixa ASCII baixa. Só é usada quando a comparação
// envolve caminho de unidade ou UNC (MF "Pattern rules", caixa).
func (n normPath) fold() normPath {
	out := normPath{prefix: asciiLower(n.prefix), windows: n.windows, segs: make([]string, len(n.segs))}
	for i, s := range n.segs {
		out.segs[i] = asciiLower(s)
	}
	return out
}

// resolve converte um caminho bruto em absoluto. Caminho relativo é ancorado em
// base. "~/" expande para home. Caminho relativo a unidade (C:foo) é recusado.
func resolve(raw, base, home string) (normPath, bool) {
	s := slash(raw)
	if strings.HasPrefix(s, "~/") {
		h := slash(home)
		if !isAbs(h) {
			return normPath{}, false
		}
		s = h + s[1:]
	} else if !isAbs(s) {
		b := slash(base)
		if isDriveRel(s) || !isAbs(b) {
			return normPath{}, false
		}
		s = b + "/" + s
	}
	return parseAbs(s), true
}

// parseAbs assume isAbs(s) e s já com barras normalizadas.
func parseAbs(s string) normPath {
	var n normPath
	rest := s
	if isDriveAbs(s) {
		n.prefix = s[:2]
		n.windows = true
		rest = s[2:]
	} else {
		n.prefix = "/"
		if strings.HasPrefix(s, "//") {
			n.windows = true
		}
	}
	for _, seg := range strings.Split(rest, "/") {
		switch seg {
		case "", ".":
		case "..":
			if len(n.segs) > 0 {
				n.segs = n.segs[:len(n.segs)-1]
			}
		default:
			n.segs = append(n.segs, seg)
		}
	}
	return n
}

// compilePattern valida um padrão de MF "Pattern rules" e o resolve.
func compilePattern(raw, markerDir, home string) (pattern, error) {
	if raw == "" {
		return pattern{}, errors.New("padrão vazio")
	}
	if utf8.RuneCountInString(raw) > MaxPatternChars {
		return pattern{}, errors.New("padrão longo demais")
	}
	if strings.ContainsAny(raw, "!{}[]()|^$%") {
		return pattern{}, errors.New("caractere não permitido no padrão")
	}
	if strings.Contains(raw, "${") || strings.Contains(raw, "***") {
		return pattern{}, errors.New("sequência não permitida no padrão")
	}
	s := slash(raw)
	for i := 0; i < len(s); i++ {
		if s[i] == '~' && (i+1 >= len(s) || s[i+1] != '/') {
			return pattern{}, errors.New("til sem barra no padrão")
		}
	}
	for _, seg := range strings.Split(s, "/") {
		if seg == ".." {
			return pattern{}, errors.New("segmento .. no padrão")
		}
	}
	if isDriveRel(s) {
		return pattern{}, errors.New("caminho relativo a unidade no padrão")
	}
	n, ok := resolve(s, markerDir, home)
	if !ok {
		return pattern{}, errors.New("padrão não resolvível (diretório do marcador ou home ausente)")
	}
	return pattern{raw: raw, orig: n, low: n.fold()}, nil
}

// globSegments casa segmentos de padrão com segmentos de caminho.
// "**" casa zero ou mais segmentos. Os demais casam um segmento com "*" e "?".
// Usa programação dinâmica sobre o sufixo: O(|pat| x |segs|) por comparação.
func globSegments(pat, segs []string) bool {
	n := len(segs)
	next := make([]bool, n+1)
	cur := make([]bool, n+1)
	next[n] = true
	for i := len(pat) - 1; i >= 0; i-- {
		if pat[i] == "**" {
			cur[n] = next[n]
			for j := n - 1; j >= 0; j-- {
				cur[j] = next[j] || cur[j+1]
			}
		} else {
			cur[n] = false
			for j := 0; j < n; j++ {
				cur[j] = next[j+1] && matchComponent(pat[i], segs[j])
			}
		}
		next, cur = cur, next
	}
	return next[0]
}

// matchComponent casa um componente com "*" (qualquer sequência) e "?" (um caractere).
func matchComponent(pat, s string) bool {
	p := []rune(pat)
	t := []rune(s)
	pi, ti := 0, 0
	star, mark := -1, 0
	for ti < len(t) {
		switch {
		case pi < len(p) && p[pi] == '*':
			star = pi
			mark = ti
			pi++
		case pi < len(p) && (p[pi] == '?' || p[pi] == t[ti]):
			pi++
			ti++
		case star >= 0:
			pi = star + 1
			mark++
			ti = mark
		default:
			return false
		}
	}
	for pi < len(p) && p[pi] == '*' {
		pi++
	}
	return pi == len(p)
}

// stringList aceita somente uma lista de strings.
func stringList(v interface{}) ([]string, error) {
	switch list := v.(type) {
	case []string:
		return list, nil
	case []interface{}:
		out := make([]string, 0, len(list))
		for _, item := range list {
			s, ok := item.(string)
			if !ok {
				return nil, errors.New("entrada não é string")
			}
			out = append(out, s)
		}
		return out, nil
	}
	return nil, errors.New("valor não é array")
}

// metadataOnly monta o corpo substituto (LEI §4.3): sessão, diretório, família,
// nome canônico, identificador da chamada e marca de protocolo. Não contém
// caminhos, padrões, argumentos, saída nem erro.
func metadataOnly(ev Event) Result {
	var b strings.Builder
	line := func(key, value string) {
		if value != "" {
			b.WriteString(key)
			b.WriteString(": ")
			b.WriteString(value)
			b.WriteString("\n")
		}
	}
	line("session_id", safeText(ev.SessionID, maxSessionIDBytes))
	line("cwd", safeText(ev.Cwd, maxCwdBytes))
	line("tool_family", ev.Family.Label())
	line("tool_name", safeToolName(ev.ToolName))
	line("tool_call_id", safeText(ev.CallID, maxCallIDBytes))
	line("capture_protocol", strconv.Itoa(ProtocolVersion))
	return Result{Decision: MetadataOnly, Body: b.String()}
}

var toolNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,63}$`)

func safeToolName(name string) string {
	if len(name) > maxToolNameLength || !toolNamePattern.MatchString(name) {
		return ""
	}
	return name
}

// safeText devolve o valor se ele couber no limite e não tiver caracteres de controle.
func safeText(v string, maxBytes int) string {
	if v == "" || len(v) > maxBytes || !printable(v) {
		return ""
	}
	return v
}

// printable recusa caracteres de controle e UTF-8 inválido.
func printable(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func slash(s string) string {
	return strings.ReplaceAll(s, `\`, "/")
}

func isAbs(s string) bool {
	return strings.HasPrefix(s, "/") || isDriveAbs(s)
}

func isDriveAbs(s string) bool {
	return len(s) >= 3 && isASCIILetter(s[0]) && s[1] == ':' && s[2] == '/'
}

func isDriveRel(s string) bool {
	return len(s) >= 2 && isASCIILetter(s[0]) && s[1] == ':' && !isDriveAbs(s)
}

func isASCIILetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}
