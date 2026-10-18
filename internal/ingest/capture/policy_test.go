package capture

import (
	"strconv"
	"strings"
	"testing"
)

const (
	testMarkerDir = "/repo"
	testHome      = "/home/dev"
)

// capMarker monta um marcador com [capture] e ignore_paths.
func capMarker(patterns ...string) []byte {
	quoted := make([]string, len(patterns))
	for i, p := range patterns {
		quoted[i] = strconv.Quote(p)
	}
	return []byte("[capture]\nignore_paths = [" + strings.Join(quoted, ", ") + "]\n")
}

func activePolicy(t *testing.T, patterns ...string) Policy {
	t.Helper()
	p := Compile(capMarker(patterns...), testMarkerDir, testHome)
	if p.State() != StateActive {
		t.Fatalf("Compile(%v) state = %v, want active", patterns, p.State())
	}
	return p
}

func fileEvent(cwd string, candidates ...string) Event {
	return Event{
		SessionID:  "sess-1",
		Cwd:        cwd,
		Family:     FamilyFile,
		ToolName:   "Edit",
		CallID:     "call-1",
		Candidates: candidates,
		Extracted:  true,
	}
}

func shellEvent(cwd string, words ...string) Event {
	return Event{
		SessionID:  "sess-1",
		Cwd:        cwd,
		Family:     FamilyShell,
		ToolName:   "Bash",
		CallID:     "call-2",
		Candidates: words,
		Extracted:  true,
	}
}

func wantDecision(t *testing.T, label string, got Result, want Decision) {
	t.Helper()
	if got.Decision != want {
		t.Fatalf("%s: decision = %v, want %v (body=%q)", label, got.Decision, want, got.Body)
	}
}

// --- Estados (MF "Inspection states", LEI §4.3) ---

func TestCompileStates(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		want    State
	}{
		{"sem marcador (nil)", nil, StateInactive},
		{"marcador vazio", []byte(""), StateInactive},
		{"marcador só com escopo", []byte("workspace = \"acme\"\n"), StateInactive},
		{"sem tabela capture", []byte("[recall]\ndefault_global = true\n"), StateInactive},
		{"capture vazio", []byte("[capture]\n"), StateInactive},
		{"ignore_paths vazio", []byte("[capture]\nignore_paths = []\n"), StateInactive},
		{"padrão válido", capMarker("secret/**"), StateActive},
		{"TOML quebrado", []byte("[capture\nignore_paths = []\n"), StateInvalid},
		{"chave desconhecida em capture", []byte("[capture]\nignore_paths = [\"a\"]\nother = 1\n"), StateInvalid},
		{"subtabela em capture", []byte("[capture]\nignore_paths = [\"a\"]\n[capture.sub]\nx = 1\n"), StateInvalid},
		{"capture não é tabela", []byte("capture = 1\n"), StateInvalid},
		{"ignore_paths não é array", []byte("[capture]\nignore_paths = \"a/**\"\n"), StateInvalid},
		{"entrada não é string", []byte("[capture]\nignore_paths = [1]\n"), StateInvalid},
		{"UTF-8 inválido", []byte{0xff, 0xfe, '\n'}, StateInvalid},
		{"acima de 64 KiB", []byte(strings.Repeat("#\n", MaxMarkerBytes)), StateInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Compile(tt.content, testMarkerDir, testHome)
			if p.State() != tt.want {
				t.Fatalf("state = %v, want %v", p.State(), tt.want)
			}
		})
	}
}

func TestCompileAtSizeLimitIsAccepted(t *testing.T) {
	base := capMarker("secret/**")
	pad := MaxMarkerBytes - len(base)
	content := append([]byte(strings.Repeat("#", pad-1)+"\n"), base...)
	if len(content) != MaxMarkerBytes {
		t.Fatalf("setup: len = %d, want %d", len(content), MaxMarkerBytes)
	}
	if p := Compile(content, testMarkerDir, testHome); p.State() != StateActive {
		t.Fatalf("state at 64 KiB = %v, want active", p.State())
	}
}

// --- Tabela de decisão (capture-contract.md §6) ---

func TestDecisionTable(t *testing.T) {
	active := activePolicy(t, "secret/**")
	inactive := Compile(nil, testMarkerDir, testHome)
	invalid := Compile([]byte("[capture\n"), testMarkerDir, testHome)

	tests := []struct {
		name string
		p    Policy
		ev   Event
		want Decision
	}{
		// Inativa: Keep em todas as famílias.
		{"inativa arquivo casa", inactive, fileEvent("/repo", "/repo/secret/a"), Keep},
		{"inativa shell casa", inactive, shellEvent("/repo", "cat", "secret/a"), Keep},
		{"inativa busca", inactive, Event{Family: FamilySearchList}, Keep},
		{"inativa demais", inactive, Event{Family: FamilyNonFile}, Keep},

		// Ativa: busca ou listagem é Drop.
		{"ativa busca", active, Event{Family: FamilySearchList, Extracted: false}, Drop},
		{"ativa busca sem candidatos", active, Event{Family: FamilySearchList, Extracted: true}, Drop},

		// Ativa: arquivo.
		{"ativa arquivo casa", active, fileEvent("/repo", "secret/a"), Drop},
		{"ativa arquivo não casa", active, fileEvent("/repo", "public/a"), Keep},
		{"ativa arquivo sem extração", active, Event{Family: FamilyFile, Candidates: []string{"public/a"}, Extracted: false}, MetadataOnly},
		{"ativa arquivo sem candidatos", active, fileEvent("/repo"), MetadataOnly},
		{"ativa arquivo candidato malformado", active, fileEvent("/repo", "   "), MetadataOnly},
		{"ativa arquivo candidato com controle", active, fileEvent("/repo", "public/\x00a"), MetadataOnly},
		{"ativa arquivo casa e malformado", active, fileEvent("/repo", "bad\x01", "secret/a"), Drop},
		{"ativa arquivo malformado e não casa", active, fileEvent("/repo", "bad\x01", "public/a"), MetadataOnly},
		{"ativa arquivo relativo sem cwd absoluto", active, fileEvent("sub", "public/a"), MetadataOnly},

		// Ativa: shell.
		{"ativa shell casa", active, shellEvent("/repo", "cat", "secret/a"), Drop},
		{"ativa shell não casa", active, shellEvent("/repo", "cat", "public/a"), Keep},
		{"ativa shell sem palavras", active, shellEvent("/repo"), Keep},
		{"ativa shell palavras vazias", active, shellEvent("/repo", "", "  "), Keep},
		{"ativa shell palavra com controle", active, shellEvent("/repo", "a\nb"), Drop},
		{"ativa shell relativa sem cwd", active, shellEvent("", "public/a"), Drop},

		// Ativa: demais famílias.
		{"ativa não-arquivo", active, Event{Family: FamilyNonFile, Candidates: []string{"secret/a"}, Extracted: true}, Keep},
		{"ativa desconhecida", active, Event{Family: FamilyUnknown, Candidates: []string{"secret/a"}, Extracted: true}, Keep},

		// Inválida: arquivo e shell viram MetadataOnly; demais, Keep.
		{"inválida arquivo", invalid, fileEvent("/repo", "public/a"), MetadataOnly},
		{"inválida shell", invalid, shellEvent("/repo", "cat", "public/a"), MetadataOnly},
		{"inválida busca", invalid, Event{Family: FamilySearchList}, Keep},
		{"inválida não-arquivo", invalid, Event{Family: FamilyNonFile}, Keep},
		{"inválida desconhecida", invalid, Event{Family: FamilyUnknown}, Keep},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantDecision(t, tt.name, tt.p.Decide(tt.ev), tt.want)
		})
	}
}

// --- Marcador inválido: cada motivo de MF "Parsing and failure" ---

func TestInvalidMarkersFailClosedForFilesAndShell(t *testing.T) {
	bad := map[string][]byte{
		"pattern com colchete": capMarker("a[b]/**"),
		"pattern com ..":       capMarker("../x"),
		"til sem barra":        capMarker("~x/y"),
		"drive relativo":       capMarker("C:foo"),
		"sequência ${":         capMarker("a${B}"),
		"três asteriscos":      capMarker("a***"),
		"caractere %":          capMarker("100%"),
		"pattern vazio":        capMarker(""),
	}
	for name, content := range bad {
		t.Run(name, func(t *testing.T) {
			p := Compile(content, testMarkerDir, testHome)
			if p.State() != StateInvalid {
				t.Fatalf("state = %v, want invalid", p.State())
			}
			wantDecision(t, "file", p.Decide(fileEvent("/repo", "public/a")), MetadataOnly)
			wantDecision(t, "shell", p.Decide(shellEvent("/repo", "cat", "public/a")), MetadataOnly)
		})
	}
}

func TestRelativePatternNeedsAbsoluteMarkerDir(t *testing.T) {
	p := Compile(capMarker("secret/**"), "", testHome)
	if p.State() != StateInvalid {
		t.Fatalf("relative pattern without marker dir: state = %v, want invalid", p.State())
	}
	p = Compile(capMarker("~/notes/**"), testMarkerDir, "")
	if p.State() != StateInvalid {
		t.Fatalf("~/ pattern without home: state = %v, want invalid", p.State())
	}
}

// --- Regras dos padrões (MF "Pattern rules") ---

func TestPatternRules(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		candidate string
		cwd       string
		match     bool
	}{
		{"prefixo de caminho inteiro", "docs/adr/**", "docs/adr", "/repo", true},
		{"não é substring", "docs/adr/**", "docs/adrx/a", "/repo", false},
		{"** cruza diretórios", "docs/**/x.md", "docs/a/b/x.md", "/repo", true},
		{"** pode ser zero diretórios", "docs/**/x.md", "docs/x.md", "/repo", true},
		{"* fica num componente", "docs/*.md", "docs/x/a.md", "/repo", false},
		{"* casa no componente", "docs/*.md", "docs/a.md", "/repo", true},
		{"? casa um caractere", "docs/a?r/**", "docs/adr", "/repo", true},
		{"? não casa dois", "docs/a?r/**", "docs/adrx", "/repo", false},
		{"relativo ancorado no marcador", "secret/**", "/repo/secret/a", "/repo/sub", true},
		{"relativo casa candidato relativo ao cwd", "secret/**", "../secret/x", "/repo/sub", true},
		{"absoluto usado como está", "/etc/**", "/etc/passwd", "/repo", true},
		{"til expande para home", "~/notes/**", "~/notes/a.md", "/repo", true},
		{"til do candidato expande", "~/notes/**", "/home/dev/notes/a.md", "/repo", true},
		{"POSIX sensível à caixa", "Secret/**", "secret/a", "/repo", false},
		{"barra invertida no candidato", "secret/**", `secret\a.txt`, "/repo", true},
		{"candidato com .. é normalizado", "secret/**", "a/../secret/file", "/repo", true},
		{"candidato com barra dupla POSIX", "/repo/secret/**", "//repo/secret/token.txt", "/repo", true},
		{"** zero com barra dupla", "/repo/secret/**", "//repo/secret", "/repo", true},
		{"ponto ponto dentro de segmento", "foo..bar", "foo..bar", "/repo", true},
		{"? casa caractere não ASCII", "unicode/?.txt", "unicode/é.txt", "/repo", true},
		{"** sozinho casa tudo", "**", "/repo/a/b", "/repo", true},
		{"a**b é válido", "a**b", "axyzb", "/repo", true},
		// Vetores de docs/assets/fixtures/capture-policy.json, bloco normalization.
		{"fixture: /** casa raiz", "/**", "/", "/repo", true},
		{"fixture: C:/** casa C:/", "C:/**", "C:/", "C:/", true},
		{"fixture: UNC sem caixa", `\\server\share\**`, "//SERVER/SHARE", "C:/", true},
		{"fixture: unidade sem caixa", `C:\Secret\**`, "c:/SECRET", "C:/", true},
		{"fixture: UNC com arquivo", `\\server\share\x`, "//SERVER/SHARE/X", "C:/", true},
		{"unidade diferente não casa", "C:/Secret/**", "D:/Secret/a", "C:/", false},
		{"POSIX com candidato UNC compara sem caixa", "/repo/Secret/**", "//REPO/secret/a", "/repo", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := activePolicy(t, tt.pattern)
			got := p.Decide(fileEvent(tt.cwd, tt.candidate))
			want := Keep
			if tt.match {
				want = Drop
			}
			wantDecision(t, tt.name, got, want)
		})
	}
}

func TestPatternRejections(t *testing.T) {
	rejected := []string{
		"a(b)", "a{b,c}", "!x", "a|b", "a^b", "$HOME/x", "100%", "a[0]",
		"${X}/a", "a***b", "../x", "a/../b", "~x/y", "a~", "C:foo", "C:",
	}
	for _, pat := range rejected {
		t.Run(pat, func(t *testing.T) {
			if p := Compile(capMarker(pat), testMarkerDir, testHome); p.State() != StateInvalid {
				t.Fatalf("pattern %q: state = %v, want invalid", pat, p.State())
			}
		})
	}
	accepted := []string{"docs/adr/**", "foo..bar", "~/x", "**", "a**b", "C:/x/**", `\\server\share\**`}
	for _, pat := range accepted {
		t.Run("aceito "+pat, func(t *testing.T) {
			if p := Compile(capMarker(pat), testMarkerDir, testHome); p.State() != StateActive {
				t.Fatalf("pattern %q: state = %v, want active", pat, p.State())
			}
		})
	}
}

// --- Limites (MF "Bounds"; LEI §4.2) ---

func TestLimitPatternsPerMarker(t *testing.T) {
	ok := make([]string, MaxPatterns)
	for i := range ok {
		ok[i] = "p" + strconv.Itoa(i) + "/**"
	}
	if p := Compile(capMarker(ok...), testMarkerDir, testHome); p.State() != StateActive {
		t.Fatalf("%d patterns: state = %v, want active", MaxPatterns, p.State())
	}
	over := append(ok, "extra/**")
	if p := Compile(capMarker(over...), testMarkerDir, testHome); p.State() != StateInvalid {
		t.Fatalf("%d patterns: state = %v, want invalid", len(over), p.State())
	}
}

func TestLimitPatternChars(t *testing.T) {
	atLimit := "a/" + strings.Repeat("b", MaxPatternChars-2)
	if p := Compile(capMarker(atLimit), testMarkerDir, testHome); p.State() != StateActive {
		t.Fatalf("pattern of %d chars: state = %v, want active", MaxPatternChars, p.State())
	}
	over := atLimit + "b"
	if p := Compile(capMarker(over), testMarkerDir, testHome); p.State() != StateInvalid {
		t.Fatalf("pattern of %d chars: state = %v, want invalid", MaxPatternChars+1, p.State())
	}
}

func TestLimitCandidatesPerEvent(t *testing.T) {
	p := activePolicy(t, "secret/**")
	nonMatching := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = "public/" + strconv.Itoa(i)
		}
		return out
	}

	wantDecision(t, "32 candidatos sem casamento", p.Decide(fileEvent("/repo", nonMatching(MaxCandidates)...)), Keep)
	wantDecision(t, "33 candidatos", p.Decide(fileEvent("/repo", nonMatching(MaxCandidates+1)...)), MetadataOnly)

	first := append([]string{"secret/a"}, nonMatching(MaxCandidates)...)
	wantDecision(t, "casamento antes do 33.º", p.Decide(fileEvent("/repo", first...)), Drop)

	// Casamento só entre os 32 primeiros, com o 33.º candidato malformado: Drop vence.
	pre := append(nonMatching(MaxCandidates-1), "secret/z")
	pre = append(pre, "secret/extra")
	wantDecision(t, "casamento dentro do limite com excesso depois", p.Decide(fileEvent("/repo", pre...)), Drop)
}

func TestLimitCandidateChars(t *testing.T) {
	p := activePolicy(t, "secret/**")
	atLimit := "public/" + strings.Repeat("x", MaxCandidateChars-len("public/"))
	wantDecision(t, "candidato de 4096 caracteres", p.Decide(fileEvent("/repo", atLimit)), Keep)
	over := atLimit + "x"
	wantDecision(t, "candidato de 4097 caracteres (arquivo)", p.Decide(fileEvent("/repo", over)), MetadataOnly)
	wantDecision(t, "candidato de 4097 caracteres (shell)", p.Decide(shellEvent("/repo", "cat", over)), Drop)
	wantDecision(t, "palavra de 4096 caracteres (shell)", p.Decide(shellEvent("/repo", atLimit)), Keep)
}

func TestMaxComparisonsConstant(t *testing.T) {
	if MaxComparisons != 1000000 {
		t.Fatalf("MaxComparisons = %d, want 1000000", MaxComparisons)
	}
}

func TestComparisonBudgetFileAndShell(t *testing.T) {
	p := activePolicy(t, "public/a", "secret/**")

	// Um candidato, duas comparações.
	wantDecision(t, "orçamento exato", decide(p, fileEvent("/repo", "keep/x"), 2), Keep)
	wantDecision(t, "orçamento esgotado (arquivo)", decide(p, fileEvent("/repo", "keep/x"), 1), MetadataOnly)
	wantDecision(t, "orçamento esgotado (shell)", decide(p, shellEvent("/repo", "keep/x"), 1), Drop)

	// Casamento na segunda comparação com orçamento de uma: esgota antes de casar.
	wantDecision(t, "casamento além do orçamento (arquivo)", decide(p, fileEvent("/repo", "secret/a"), 1), MetadataOnly)
	wantDecision(t, "casamento dentro do orçamento (arquivo)", decide(p, fileEvent("/repo", "secret/a"), 2), Drop)
}

func TestComparisonBudgetDefaultIsNotExhaustedAtMaxBounds(t *testing.T) {
	patterns := make([]string, MaxPatterns)
	for i := range patterns {
		patterns[i] = "p" + strconv.Itoa(i) + "/**"
	}
	p := activePolicy(t, patterns...)
	cands := make([]string, MaxCandidates)
	for i := range cands {
		cands[i] = "public/" + strconv.Itoa(i)
	}
	wantDecision(t, "limite máximo de padrões e candidatos", p.Decide(fileEvent("/repo", cands...)), Keep)
}

// --- Arquivo sem família de caminho e shell sem casamento ---

func TestSearchListDropsRegardlessOfCandidates(t *testing.T) {
	p := activePolicy(t, "secret/**")
	wantDecision(t, "busca sem casamento", p.Decide(Event{Family: FamilySearchList, Candidates: []string{"public"}, Extracted: true}), Drop)
}

// --- Corpo substituto de MetadataOnly (LEI §4.3) ---

func parseBodyKeys(t *testing.T, body string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, line := range strings.Split(strings.TrimSuffix(body, "\n"), "\n") {
		k, v, ok := strings.Cut(line, ": ")
		if !ok {
			t.Fatalf("linha de corpo sem chave: %q", line)
		}
		out[k] = v
	}
	return out
}

func TestMetadataBodyCarriesOnlyAllowedFields(t *testing.T) {
	p := activePolicy(t, "secret/**")
	ev := fileEvent("/repo", "public/x")
	ev.Extracted = false // força MetadataOnly

	res := p.Decide(ev)
	wantDecision(t, "metadados", res, MetadataOnly)
	got := parseBodyKeys(t, res.Body)
	want := map[string]string{
		"session_id":       "sess-1",
		"cwd":              "/repo",
		"tool_family":      "file",
		"tool_name":        "Edit",
		"tool_call_id":     "call-1",
		"capture_protocol": "1",
	}
	if len(got) != len(want) {
		t.Fatalf("chaves do corpo = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("corpo[%s] = %q, want %q", k, got[k], v)
		}
	}
}

func TestMetadataBodyNeverLeaksPathsPatternsOrArguments(t *testing.T) {
	p := activePolicy(t, "marker-pattern/**")
	ev := Event{
		SessionID:  "sess-1",
		Cwd:        "/repo",
		Family:     FamilyFile,
		ToolName:   "Edit",
		CallID:     "call-1",
		Candidates: []string{"secret-leak/a\x00tail", "marker-pattern/b"},
		Extracted:  true,
	}
	res := p.Decide(ev)
	// O segundo candidato casa, então é Drop e não há corpo. Use um caso só malformado.
	wantDecision(t, "casa tem precedência", res, Drop)

	ev.Candidates = []string{"secret-leak/a\x00tail"}
	res = p.Decide(ev)
	wantDecision(t, "malformado", res, MetadataOnly)
	for _, leak := range []string{"secret-leak", "tail", "marker-pattern", "a/b", "public", "cat "} {
		if strings.Contains(res.Body, leak) {
			t.Fatalf("corpo vaza %q: %q", leak, res.Body)
		}
	}
}

func TestMetadataBodyInvalidStateShellAndSanitizedFields(t *testing.T) {
	invalid := Compile([]byte("[capture\n"), testMarkerDir, testHome)
	ev := Event{
		SessionID:  "sess-2",
		Cwd:        "/repo",
		Family:     FamilyShell,
		ToolName:   "Bash -c evil",       // não é nome canônico: omitido
		CallID:     "call-9\ninjected",   // controle: omitido
		Candidates: []string{"secret/a"}, // nunca aparece no corpo
		Extracted:  true,
	}
	res := invalid.Decide(ev)
	wantDecision(t, "shell inválido", res, MetadataOnly)
	got := parseBodyKeys(t, res.Body)
	if got["tool_family"] != "non-file" {
		t.Fatalf("tool_family = %q, want non-file", got["tool_family"])
	}
	if _, ok := got["tool_name"]; ok {
		t.Fatalf("tool_name com espaço deveria ser omitido: %q", res.Body)
	}
	if _, ok := got["tool_call_id"]; ok {
		t.Fatalf("tool_call_id com controle deveria ser omitido: %q", res.Body)
	}
	if strings.Contains(res.Body, "secret") || strings.Contains(res.Body, "evil") || strings.Contains(res.Body, "injected") {
		t.Fatalf("corpo vaza dado bruto: %q", res.Body)
	}
}

func TestMetadataBodyOnlyForMetadataDecision(t *testing.T) {
	p := activePolicy(t, "secret/**")
	if res := p.Decide(fileEvent("/repo", "secret/a")); res.Body != "" {
		t.Fatalf("Drop com corpo: %q", res.Body)
	}
	if res := p.Decide(fileEvent("/repo", "public/a")); res.Body != "" {
		t.Fatalf("Keep com corpo: %q", res.Body)
	}
}
