package sanitize

import (
	"bufio"
	"fmt"
	"math/rand/v2"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

// expectedLabels são os 18 rótulos de DV §7.2, na ordem dos padrões.
var expectedLabels = []string{
	"private_key", "url_credentials", "auth_header", "bearer_token", "api_key", "stripe_key",
	"github_token", "aws_key", "google_api_key", "google_oauth", "meta_token", "telegram_token",
	"ghl_token", "slack_token", "jwt", "base64_json_token", "env_secret", "credential_path",
}

// fixture é uma linha de testdata/secrets.txt com o template já expandido.
type fixture struct {
	label    string
	kind     string // pos, neg ou cut
	tmpl     string
	expanded string
}

var (
	placeholderRe = regexp.MustCompile(`<<(UP|HEX|MIX)#([0-9]+)>>|<<DOT#([a-z]+)>>|<<PEM#([A-Z ]+)>>`)
	alphabets     = map[string]string{
		"UP":  "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789",
		"HEX": "0123456789abcdef",
		"MIX": "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789",
	}
	fixedReplacer = strings.NewReplacer(
		"<<EQ>>", "=",
		"<<COLON>>", ":",
		"<<HOME>>", "/home/dev",
		"<<WIN_HOME>>", `C:\Users\dev`,
		`\n`, "\n",
	)
)

// fill monta n caracteres cíclicos do alfabeto. Os valores são sintéticos e determinísticos.
func fill(alpha string, n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteByte(alpha[i%len(alpha)])
	}
	return b.String()
}

func expandPlaceholder(m string) string {
	sub := placeholderRe.FindStringSubmatch(m)
	switch {
	case sub[1] != "":
		n, _ := strconv.Atoi(sub[2])
		return fill(alphabets[sub[1]], n)
	case sub[3] != "":
		return "." + sub[3]
	default:
		return "-----" + sub[4] + "-----"
	}
}

// expandTemplate monta o valor real em tempo de execução. O arquivo nunca contém o valor cru.
func expandTemplate(tmpl string) string {
	return placeholderRe.ReplaceAllStringFunc(fixedReplacer.Replace(tmpl), expandPlaceholder)
}

// loadFixtures lê testdata/secrets.txt e expande cada template.
func loadFixtures(tb testing.TB) []fixture {
	tb.Helper()
	f, err := os.Open("testdata/secrets.txt")
	if err != nil {
		tb.Fatalf("abrir fixtures: %v", err)
	}
	defer func() { _ = f.Close() }()

	var out []fixture
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		if len(parts) != 3 {
			tb.Fatalf("linha de fixture malformada: %q", line)
		}
		out = append(out, fixture{
			label:    parts[0],
			kind:     parts[1],
			tmpl:     parts[2],
			expanded: expandTemplate(parts[2]),
		})
	}
	if err := sc.Err(); err != nil {
		tb.Fatalf("ler fixtures: %v", err)
	}
	if len(out) == 0 {
		tb.Fatal("nenhuma fixture carregada")
	}
	return out
}

// --- Controle, título e corpo ---

func TestStripControl_RemoveEscapeControleEBidi(t *testing.T) {
	casos := []struct{ entrada, want string }{
		{"\x1b[31mred\x1b[0m\x00\u202efoo\tbar\n", "redfoo\tbar\n"},
		{"a\u009b31mb", "ab"},
		{"a\x1b]0;titulo\x07b", "ab"},
		{"a\u200bb\ufeffc\u2066d", "abcd"},
		{"a\x7fb", "ab"},
		{"a\xffb", "ab"},
		{"linha\r\nnova", "linha\r\nnova"},
	}
	for _, c := range casos {
		if got := stripControl(c.entrada); got != c.want {
			t.Errorf("stripControl(%q) = %q, quer %q", c.entrada, got, c.want)
		}
	}
}

func TestTitle_CortaDentroDos80Runes(t *testing.T) {
	lim := Limits{TitleMaxRunes: ObservationTitleMaxRunes}

	curto := Title("título curto", lim).Text()
	if curto != "título curto" {
		t.Errorf("título curto alterado: %q", curto)
	}

	longo := Title(strings.Repeat("á", 200), lim).Text()
	if n := utf8.RuneCountInString(longo); n != 80 {
		t.Errorf("runes = %d, quer 80", n)
	}
	if !strings.HasSuffix(longo, "…") {
		t.Errorf("título cortado não termina em …: %q", longo[len(longo)-6:])
	}
}

func TestTitle_SanitizaAntesDeCortar(t *testing.T) {
	// Segredo de 40 chars começa perto do teto de 80 runes. Redigido antes, nunca aparece.
	segredo := expandTemplate("sk-<<MIX#40>>")
	raw := strings.Repeat("a", 70) + " " + segredo
	out := Title(raw, Limits{TitleMaxRunes: 80}).Text()
	if strings.Contains(out, segredo[:12]) {
		t.Errorf("título vazou fragmento do segredo: %q", out)
	}
}

func TestBody_CabecaECaudaExatas(t *testing.T) {
	// 10000 'a' + 10000 'b' com teto 16384. Orçamento útil 16336, metade para cada ponta.
	raw := strings.Repeat("a", 10000) + strings.Repeat("b", 10000)
	out := Body(raw, Limits{BodyMaxBytes: 16384}).Text()
	want := strings.Repeat("a", 8168) + "\n...[truncated 3664 bytes]...\n" + strings.Repeat("b", 8168)
	if out != want {
		t.Errorf("corpo truncado diferente do esperado: len=%d quer len=%d", len(out), len(want))
	}
}

func TestBody_SemTetoQuandoZero(t *testing.T) {
	raw := strings.Repeat("x", 100000)
	if out := Body(raw, Limits{}).Text(); out != raw {
		t.Errorf("Limits{} zero truncou o corpo: len=%d", len(out))
	}
}

func TestBody_SemCortarMultibyte(t *testing.T) {
	// Deslocamento ímpar faz a metade cair em continuação de 'é' (2 bytes).
	raw := "a" + strings.Repeat("é", 20000)
	out := Body(raw, Limits{BodyMaxBytes: 16384}).Text()
	if !utf8.ValidString(out) {
		t.Fatal("saída com UTF-8 inválido")
	}
	if len(out) > 16384 {
		t.Fatalf("len = %d > 16384", len(out))
	}
}

func TestBody_TetoPequenoSoCabeca(t *testing.T) {
	raw := strings.Repeat("z", 500)
	out := Body(raw, Limits{BodyMaxBytes: 40}).Text()
	if len(out) > 40 {
		t.Fatalf("len = %d > 40", len(out))
	}
	if !strings.HasPrefix(out, "z") || !strings.HasSuffix(out, " bytes]...\n") {
		t.Errorf("corte de cabeça inesperado: %q", out)
	}
}

// --- Padrões e fixtures ---

func TestPadroes_SaoOsDezoitoRotulosEmOrdem(t *testing.T) {
	if len(rules) != len(expectedLabels) {
		t.Fatalf("%d padrões, quer %d", len(rules), len(expectedLabels))
	}
	for i, r := range rules {
		if r.label != expectedLabels[i] {
			t.Errorf("padrão %d = %q, quer %q", i, r.label, expectedLabels[i])
		}
	}
}

func TestFixtures_CadaClasseTemPosNegECortado(t *testing.T) {
	cont := map[string]map[string]int{}
	for _, fx := range loadFixtures(t) {
		if cont[fx.label] == nil {
			cont[fx.label] = map[string]int{}
		}
		cont[fx.label][fx.kind]++
	}
	for _, label := range expectedLabels {
		for _, kind := range []string{"pos", "neg", "cut"} {
			if cont[label][kind] == 0 {
				t.Errorf("classe %s sem caso %s", label, kind)
			}
		}
	}
}

func TestFixtures_Positivos(t *testing.T) {
	for _, fx := range loadFixtures(t) {
		if fx.kind != "pos" {
			continue
		}
		out := Body(fx.expanded, Limits{}).Text()
		if want := "[REDACTED:" + fx.label + "]"; !strings.Contains(out, want) {
			t.Errorf("%s: sem %s na saída %q", fx.label, want, out)
		}
		prefixo := fx.expanded
		if len(prefixo) > 12 {
			prefixo = prefixo[:12]
		}
		if strings.Contains(out, prefixo) {
			t.Errorf("%s: vazou prefixo do segredo %q na saída %q", fx.label, prefixo, out)
		}
	}
}

func TestFixtures_Negativos(t *testing.T) {
	for _, fx := range loadFixtures(t) {
		if fx.kind != "neg" {
			continue
		}
		out := Body(fx.expanded, Limits{}).Text()
		if out != fx.expanded {
			t.Errorf("%s: negativo alterado: entrada %q, saída %q", fx.label, fx.expanded, out)
		}
	}
}

// TestFixtures_CortadoPeloLimite prova a ordem sanitizar-antes-de-truncar (INV-06). O segredo
// começa dentro da cabeça de um corte de 100 bytes. Se o corte viesse antes, o fragmento
// da cabeça não casaria com padrão nenhum e vazaria. A saída real não pode conter o fragmento.
func TestFixtures_CortadoPeloLimite(t *testing.T) {
	const teto = 100
	headB := (teto - bodyHeadTailReserve) / 2 // 26
	for _, fx := range loadFixtures(t) {
		if fx.kind != "cut" {
			continue
		}
		seg := fx.expanded
		if len(seg) < 12 {
			t.Fatalf("%s: segredo curto demais para o teste de corte", fx.label)
		}
		fragmento := seg[:12]
		start := headB - 12
		raw := strings.Repeat("x ", start/2) + seg + strings.Repeat(" y", 30)

		// Controle: truncar antes deixaria o fragmento na cabeça sem casar com padrão.
		ingenuo := redact(raw[:headB])
		if !strings.Contains(ingenuo, fragmento) {
			t.Errorf("%s: controle inválido, o fragmento não sobreviveria ao corte ingênuo", fx.label)
		}

		out := Body(raw, Limits{BodyMaxBytes: teto}).Text()
		if len(out) > teto {
			t.Errorf("%s: len = %d > %d", fx.label, len(out), teto)
		}
		if strings.Contains(out, fragmento) {
			t.Errorf("%s: fragmento vazou no corte: %q", fx.label, out)
		}
	}
}

// TestFixtures_ArquivoCruSemSegredo confere que o próprio arquivo, lido cru, não casa com
// nenhum padrão e não tem controle. É a exigência de ADR-14.
func TestFixtures_ArquivoCruSemSegredo(t *testing.T) {
	raw, err := os.ReadFile("testdata/secrets.txt")
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, r := range rules {
		for _, m := range r.re.FindAllString(s, -1) {
			t.Errorf("testdata/secrets.txt: rótulo %s casa com %q", r.label, m)
		}
	}
	if stripControl(s) != s {
		t.Errorf("testdata/secrets.txt contém caractere de controle")
	}
}

func TestMarcadorNaoCasaComPadrao(t *testing.T) {
	for _, label := range expectedLabels {
		m := "[REDACTED:" + label + "]"
		if redact(m) != m {
			t.Errorf("marcador %s casou com padrão", m)
		}
	}
}

// TestRegressao_TokenColadoEmLetrasERedigido cobre INV-06 e DV §7.1. Um segredo colado em letras
// não tem fronteira de palavra; sem `\b` no início, o padrão ainda casa e o token cru some. Os
// valores são expandidos em runtime e cercados por letras com strings.Join, então o fonte não
// contém segredo literal.
func TestRegressao_TokenColadoEmLetrasERedigido(t *testing.T) {
	casos := []struct{ label, tmpl string }{
		{"github_token", "ghp_<<MIX#24>>"},
		{"github_token", "github_pat_<<MIX#24>>"},
		{"api_key", "sk-<<MIX#24>>"},
		{"aws_key", "AKIA<<UP#16>>"},
		{"bearer_token", "Bearer <<MIX#24>>"},
		{"auth_header", "Authorization<<COLON>> <<MIX#24>>"},
		{"stripe_key", "sk_live_<<MIX#16>>"},
		{"google_api_key", "AIza<<MIX#30>>"},
		{"google_api_key", "AQ.Ab<<MIX#30>>"},
		{"google_oauth", "1//<<MIX#20>>"},
		{"meta_token", "EAA<<MIX#24>>"},
		{"telegram_token", "123456789<<COLON>><<MIX#35>>"},
		{"ghl_token", "pit-<<HEX#8>>-<<HEX#4>>-<<HEX#4>>-<<HEX#4>>-<<HEX#12>>"},
		{"slack_token", "xoxb-<<MIX#24>>"},
		{"slack_token", "xapp-<<MIX#24>>"},
		{"jwt", "eyJ<<MIX#20>>.<<MIX#20>>.<<MIX#20>>"},
		{"base64_json_token", "eyJ<<MIX#40>>"},
		{"env_secret", "GITHUB_TOKEN<<EQ>><<MIX#20>>"},
	}
	for _, c := range casos {
		segredo := expandTemplate(c.tmpl)
		raw := strings.Join([]string{"xxxx", segredo, "yyyy"}, "")
		out := Body(raw, Limits{}).Text()
		if want := "[REDACTED:" + c.label + "]"; !strings.Contains(out, want) {
			t.Errorf("%s colado em letras: sem %s na saída %q", c.label, want, out)
		}
		if strings.Contains(out, segredo) {
			t.Errorf("%s colado em letras: token cru sobreviveu na saída %q", c.label, out)
		}
	}
}

// TestRegressao_NegativosDaT013 fixa os negativos citados na correção de T013: nenhum deles pode
// ser alterado pela remoção de `\b`.
func TestRegressao_NegativosDaT013(t *testing.T) {
	for _, neg := range []string{
		"ASIAPACIFICREGION",
		"Idempotency-Key: 6f1c2e9a-retry-safe",
		"internal.storage.writer",
		"MAX_TOKENS=100",
		"sk-curto",
	} {
		if out := redact(neg); out != neg {
			t.Errorf("negativo alterado: entrada %q, saída %q", neg, out)
		}
	}
}

// --- Propriedades ---

// piecesRuido são fragmentos variados. Misturados com segredos, geram entradas difíceis.
var piecesRuido = []string{
	"a", " ", "\n", "\t", "é", "中", "😀", "\x1b[31m", "\x00", "\u202e", "sk-", "AKIA",
	"Bearer ", "eyJ", ":", "=", "/", ".ssh", "\xff", "x-api-key", "_TOKEN=",
}

func ruido(r *rand.Rand, n int, segredos []string) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		if len(segredos) > 0 && r.IntN(5) == 0 {
			b.WriteString(segredos[r.IntN(len(segredos))])
		} else {
			b.WriteString(piecesRuido[r.IntN(len(piecesRuido))])
		}
	}
	return b.String()
}

func segredosDasFixtures(tb testing.TB) []string {
	var out []string
	for _, fx := range loadFixtures(tb) {
		out = append(out, fx.expanded)
	}
	return out
}

// checaInvariantesCorpo verifica o contrato do corpo: teto, UTF-8 válido, sem segredo na
// saída, sem controle e idempotência.
func checaInvariantesCorpo(t testing.TB, raw string, teto int) {
	t.Helper()
	lim := Limits{BodyMaxBytes: teto}
	out := Body(raw, lim).Text()
	if teto > 0 && len(out) > teto {
		t.Fatalf("len(saída) = %d > teto %d", len(out), teto)
	}
	if !utf8.ValidString(out) {
		t.Fatalf("saída com UTF-8 inválido")
	}
	if redact(out) != out {
		t.Fatalf("segredo sobrevive na saída: %q", out)
	}
	if stripControl(out) != out {
		t.Fatalf("saída com controle")
	}
	if again := Body(out, lim).Text(); again != out {
		t.Fatalf("não idempotente: %q -> %q", out, again)
	}
}

// TestPropriedade_TetoDe16KiB é o teste de propriedade do teto de corpo (ADR-11).
func TestPropriedade_TetoDe16KiB(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	segs := segredosDasFixtures(t)
	for i := 0; i < 150; i++ {
		raw := ruido(r, r.IntN(4000), segs)
		checaInvariantesCorpo(t, raw, ObservationBodyMaxBytes)
	}
}

// TestPropriedade_TetosPequenos cobre tetos abaixo de 64 bytes e a fronteira UTF-8.
func TestPropriedade_TetosPequenos(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	segs := segredosDasFixtures(t)
	for i := 0; i < 300; i++ {
		raw := ruido(r, r.IntN(300), segs)
		checaInvariantesCorpo(t, raw, 1+r.IntN(300))
	}
}

func TestTitulo_Idempotente(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 6))
	segs := segredosDasFixtures(t)
	lim := Limits{TitleMaxRunes: ObservationTitleMaxRunes}
	for i := 0; i < 200; i++ {
		raw := ruido(r, r.IntN(200), segs)
		out := Title(raw, lim).Text()
		if n := utf8.RuneCountInString(out); n > 80 {
			t.Fatalf("título com %d runes", n)
		}
		if again := Title(out, lim).Text(); again != out {
			t.Fatalf("título não idempotente: %q -> %q", out, again)
		}
		if redact(out) != out {
			t.Fatalf("segredo no título: %q", out)
		}
	}
}

// --- Fuzz ---

func FuzzSanitizeIdempotente(f *testing.F) {
	for _, fx := range loadFixtures(f) {
		f.Add(fx.expanded, uint16(0))
		f.Add(fx.expanded, uint16(100))
	}
	f.Add("lorem ipsum "+strings.Repeat("é", 5000), uint16(16384))
	f.Fuzz(func(t *testing.T, raw string, teto uint16) {
		checaInvariantesCorpo(t, raw, int(teto))
		lim := Limits{TitleMaxRunes: int(teto%120) + 1}
		out := Title(raw, lim).Text()
		if again := Title(out, lim).Text(); again != out {
			t.Fatalf("título não idempotente: %q -> %q", out, again)
		}
	})
}

func FuzzSanitizeNaoPanica(f *testing.F) {
	for _, fx := range loadFixtures(f) {
		f.Add(fx.expanded)
	}
	f.Add("\x1b[2J\x00\xff\u202e")
	f.Add("")
	f.Fuzz(func(t *testing.T, raw string) {
		for _, teto := range []int{0, 1, 27, 63, 64, 200, ObservationBodyMaxBytes} {
			_ = Body(raw, Limits{BodyMaxBytes: teto})
		}
		for _, n := range []int{0, 1, 2, 80} {
			out := Title(raw, Limits{TitleMaxRunes: n}).Text()
			if n > 0 && utf8.RuneCountInString(out) > n {
				t.Fatalf("título com %d runes > %d", utf8.RuneCountInString(out), n)
			}
		}
	})
}

// fmtSanity garante que o formato do marcador segue o contrato de ADR-11.
func TestMarcador_FormatoDeAdr11(t *testing.T) {
	for _, n := range []int{0, 7, 1234, 1 << 40} {
		m := truncationMarker(n)
		if want := fmt.Sprintf("\n...[truncated %d bytes]...\n", n); m != want {
			t.Errorf("marcador %q, quer %q", m, want)
		}
		if len(m) > bodyHeadTailReserve {
			t.Errorf("marcador com %d bytes excede a reserva de %d", len(m), bodyHeadTailReserve)
		}
	}
}
