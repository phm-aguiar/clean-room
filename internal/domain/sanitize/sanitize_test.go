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

// expectedLabels são os 18 rótulos distintos de prompts §11.2 (27 linhas, alguns rótulos se repetem).
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

// redact aplica os padrões embutidos, sem allowlist, como o pipeline faz na etapa 2.
func redact(s string) string {
	return Sanitizer{}.redact(s)
}

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

// TestStripControl_RemoveEscapeControleEBidi cobre o §11.1, passos 1 e 2. Controles fora da lista
// (U+200B, U+FEFF) ficam no texto; ESC seguido de caractere fora de `@` a `_` não é sequência.
func TestStripControl_RemoveEscapeControleEBidi(t *testing.T) {
	casos := []struct{ entrada, want string }{
		{"\x1b[31mred\x1b[0m\x00\u202efoo\tbar\n", "redfoo\tbar\n"},
		{"a\x1b]0;titulo\x07b", "ab"},
		{"a\x1b]0;titulo\x1b\\b", "ab"},
		{"a\u2066d\u2069e", "ade"},
		{"a\u200bb\ufeffcd\u2066e", "a\u200bb\ufeffcde"},
		{"a\x7fb", "ab"},
		{"a\xffb", "ab"},
		{"linha\r\nnova", "linha\r\nnova"},
		// C1 de 8 bits não é sequência no §11.1: sai como controle e o resto fica como texto.
		{"a\u009b31mb", "a31mb"},
		// ESC seguido de `(` não está em `@` a `_`: sai só o ESC, o resto fica.
		{"a\x1b(Bb", "a(Bb"},
	}
	for _, c := range casos {
		if got := stripControl(c.entrada); got != c.want {
			t.Errorf("stripControl(%q) = %q, quer %q", c.entrada, got, c.want)
		}
	}
}

// TestStripControl_ListaDoSecao111 confere a lista exata de controles do §11.1, passo 2.
func TestStripControl_ListaDoSecao111(t *testing.T) {
	for c := rune(0); c < 0x20; c++ {
		if c == '\t' || c == '\n' || c == '\r' {
			continue
		}
		if got := stripControl(string(c)); got != "" {
			t.Errorf("C0 U+%04X não removido: %q", c, got)
		}
	}
	for c := rune(0x7f); c <= 0x9f; c++ {
		if got := stripControl(string(c)); got != "" {
			t.Errorf("U+%04X (DEL/C1) não removido: %q", c, got)
		}
	}
	for c := rune(0x202a); c <= 0x202e; c++ {
		if got := stripControl(string(c)); got != "" {
			t.Errorf("U+%04X (bidi) não removido: %q", c, got)
		}
	}
	for c := rune(0x2066); c <= 0x2069; c++ {
		if got := stripControl(string(c)); got != "" {
			t.Errorf("U+%04X (isolate) não removido: %q", c, got)
		}
	}
	for _, c := range []rune{'\t', '\n', '\r', 0xa0, 0x200b, 0xfeff} {
		if got := stripControl(string(c)); got != string(c) {
			t.Errorf("U+%04X removido indevidamente: %q", c, got)
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

// --- Padrões: uma linha da tabela §11.2 por teste ---

// linhaPadrao é uma linha da tabela §11.2: número (1 a 27), rótulo, exemplos que devem redigir
// (pos) e exemplos que não podem mudar (neg, inclui os falsos positivos do §11.4).
type linhaPadrao struct {
	linha int
	label string
	pos   []string
	neg   []string
}

var tabela112 = []linhaPadrao{
	{1, "bearer_token", []string{"Bearer <<MIX#24>>"}, []string{"bearer curto"}},
	{2, "api_key", []string{"sk-<<MIX#24>>"}, []string{"sk-curto"}},
	{3, "stripe_key", []string{"sk_live_<<MIX#16>>", "rk_live_<<MIX#16>>"}, []string{"sk_test_<<MIX#16>>"}},
	{4, "github_token", []string{"ghp_<<MIX#24>>", "gho_<<MIX#24>>", "ghu_<<MIX#24>>", "ghs_<<MIX#24>>", "ghr_<<MIX#24>>"}, []string{"ghp_curto"}},
	{5, "github_token", []string{"github_pat_<<MIX#24>>"}, []string{"github_pat_curto"}},
	{6, "aws_key", []string{"chave AKIA<<UP#16>> fim", "ASIA<<UP#16>>"}, []string{"ASIAPACIFICREGION"}},
	{7, "google_api_key", []string{"AIza<<MIX#30>>"}, []string{"AIzaCurto"}},
	{8, "google_api_key", []string{"AQ.Ab<<MIX#30>>"}, []string{"AQ.Abcurto"}},
	{9, "google_oauth", []string{"1//<<MIX#20>>"}, []string{"1//curto"}},
	{10, "meta_token", []string{"EAA<<MIX#24>>"}, []string{"EAAcurto"}},
	{11, "telegram_token", []string{"123456789<<COLON>><<MIX#35>>", "123456789<<COLON>>AA<<MIX#30>>"}, []string{"12345<<COLON>>curto-token"}},
	{12, "ghl_token", []string{"pit-<<HEX#8>>-<<HEX#4>>-<<HEX#4>>-<<HEX#4>>-<<HEX#12>>"}, []string{"pit-sem-uuid", "pit-stop-strategy-analysis"}},
	{13, "slack_token", []string{"xoxb-<<MIX#24>>", "xoxp-<<MIX#24>>"}, []string{"xoxb-curto"}},
	{14, "slack_token", []string{"xapp-<<MIX#24>>"}, []string{"xapp-curto"}},
	{15, "jwt", []string{"eyJ<<MIX#20>>.<<MIX#20>>.<<MIX#20>>"}, []string{"internal.storage.writer", "eyJ<<MIX#15>>.<<MIX#20>>.<<MIX#20>>", "eyJ<<MIX#20>>.<<MIX#15>>.<<MIX#20>>"}},
	{16, "base64_json_token", []string{"eyJ<<MIX#40>>"}, []string{"eyJcurto", "eyJ<<MIX#39>>"}},
	{17, "private_key", []string{"<<PEM#BEGIN RSA PRIVATE KEY>>\n<<MIX#64>>\n<<PEM#END RSA PRIVATE KEY>>"}, []string{"<<PEM#BEGIN PUBLIC KEY>>\nMIIBIjANBgkqhkiG9w0BAQEF\n<<PEM#END PUBLIC KEY>>"}},
	{18, "url_credentials", []string{"https://deploy<<COLON>><<MIX#16>>@git.example.com/repo.git"}, []string{"https://git.example.com/repo.git", "https://exemplo.org:8080/caminho"}},
	{19, "auth_header", []string{"Authorization<<COLON>> <<MIX#24>>", "X-Api-Key<<COLON>> <<MIX#24>>", "Private-Token<<COLON>> <<MIX#24>>", "Ocp-Apim-Subscription-Key<<COLON>> <<MIX#24>>", "X-Amz-Security-Token<<COLON>> <<MIX#24>>"}, []string{"Idempotency-Key<<COLON>> 6f1c2e9a-retry-safe", "Continuation-Token<<COLON>> <<MIX#24>>"}},
	{20, "env_secret", []string{"GITHUB_TOKEN<<EQ>><<MIX#20>>", "openai_api_key<<EQ>><<MIX#20>>", "AWS_SECRET_ACCESS_KEY<<COLON>> <<MIX#20>>"}, []string{"GITHUB_TOKEN is set"}},
	{21, "env_secret", []string{"MY_APP_TOKEN<<EQ>><<MIX#20>>", "my_app_token<<EQ>><<MIX#20>>", "db_secret<<EQ>><<MIX#20>>"}, []string{"MAX_TOKENS<<EQ>>100", "api_key_id<<EQ>><<MIX#20>>"}},
	{22, "env_secret", []string{"APP_PEPPER<<EQ>><<MIX#20>>", "MY_KEY_ID<<EQ>><<MIX#20>>", "APP_SALT<<COLON>> <<MIX#20>>"}, []string{"app_pepper<<EQ>><<MIX#20>>", "password_salt<<EQ>><<MIX#20>>"}},
	{23, "credential_path", []string{"<<HOME>>/<<DOT#ssh>>/id_ed25519"}, []string{"/home/dev/docs/readme.md", "~/projects/sshd-notes"}},
	{24, "credential_path", []string{`<<WIN_HOME>>\<<DOT#aws>>\credentials`}, []string{`C:\Users\dev\awesome\notas.txt`}},
	{25, "credential_path", []string{"<<HOME>>/<<DOT#kube>>/config"}, []string{"/home/dev/kubernetes/readme.md"}},
	{26, "credential_path", []string{"<<HOME>>/<<DOT#config>>/gcloud/credentials.db"}, []string{"/home/dev/.config/other/x.toml"}},
	{27, "credential_path", []string{"<<HOME>>/<<DOT#gnupg>>/secring"}, []string{"/home/dev/gnupg-notes.md"}},
}

// TestPadroes_SaoOs27NaOrdemDoDoc confere a ordem e os rótulos da tabela §11.2.
func TestPadroes_SaoOs27NaOrdemDoDoc(t *testing.T) {
	quer := []string{
		"bearer_token", "api_key", "stripe_key", "github_token", "github_token", "aws_key",
		"google_api_key", "google_api_key", "google_oauth", "meta_token", "telegram_token",
		"ghl_token", "slack_token", "slack_token", "jwt", "base64_json_token", "private_key",
		"url_credentials", "auth_header", "env_secret", "env_secret", "env_secret",
		"credential_path", "credential_path", "credential_path", "credential_path", "credential_path",
	}
	if len(builtinRules) != len(quer) {
		t.Fatalf("%d padrões, quer %d", len(builtinRules), len(quer))
	}
	for i, r := range builtinRules {
		if r.label != quer[i] {
			t.Errorf("padrão %d = %q, quer %q", i+1, r.label, quer[i])
		}
	}
	if len(tabela112) != len(quer) {
		t.Fatalf("tabela de teste com %d linhas, quer %d", len(tabela112), len(quer))
	}
}

// TestPadroes_CadaLinhaTemPositivoENegativo cobre cada uma das 27 linhas: o positivo casa com o
// padrão da própria linha e sai como `[REDACTED:<rótulo>]`; o negativo não muda o texto.
func TestPadroes_CadaLinhaTemPositivoENegativo(t *testing.T) {
	for _, l := range tabela112 {
		r := builtinRules[l.linha-1]
		if r.label != l.label {
			t.Errorf("linha %d: rótulo %q, quer %q", l.linha, r.label, l.label)
		}
		if len(l.pos) == 0 || len(l.neg) == 0 {
			t.Fatalf("linha %d sem positivo ou sem negativo", l.linha)
		}
		for _, tmpl := range l.pos {
			segredo := expandTemplate(tmpl)
			if !r.re.MatchString(segredo) {
				t.Errorf("linha %d (%s): padrão da linha não casa o positivo %q", l.linha, l.label, tmpl)
				continue
			}
			out := Body(segredo, Limits{}).Text()
			if want := "[REDACTED:" + l.label + "]"; !strings.Contains(out, want) {
				t.Errorf("linha %d: sem %s na saída %q", l.linha, want, out)
			}
			if strings.Contains(out, segredo) {
				t.Errorf("linha %d: segredo cru sobreviveu: %q", l.linha, out)
			}
		}
		for _, tmpl := range l.neg {
			neg := expandTemplate(tmpl)
			if out := Body(neg, Limits{}).Text(); out != neg {
				t.Errorf("linha %d (%s): negativo alterado: entrada %q, saída %q", l.linha, l.label, neg, out)
			}
		}
	}
}

// TestFixtures_CadaClasseTemPosNegECortado confere que o arquivo de fixtures cobre cada
// rótulo com positivo, negativo e caso de corte.
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
	for _, r := range builtinRules {
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

// TestFalsosPositivos_Sec114 fixa os falsos positivos citados em prompts §11.4 e nas correções
// anteriores: nenhum deles pode ser alterado.
func TestFalsosPositivos_Sec114(t *testing.T) {
	for _, neg := range []string{
		"ASIAPACIFICREGION",
		"pit-stop-strategy-analysis",
		"Idempotency-Key: 6f1c2e9a-retry-safe",
		"Continuation-Token: 6f1c2e9a-retry-safe",
		"api_key_id=valor-de-teste-123",
		"password_salt=valor-de-teste-123",
		"MAX_TOKENS=100",
		"internal.storage.writer",
		"sk-curto",
	} {
		if out := redact(neg); out != neg {
			t.Errorf("falso positivo: entrada %q, saída %q", neg, out)
		}
	}
}

// TestBordaDePalavra_AwsETelegramExigemFronteira fixa a decisão do doc (§11.2, linhas 6 e 11):
// `aws_key` e `telegram_token` exigem fronteira de palavra. Um token colado em letras não casa.
// É uma decisão de doc, não uma lacuna do código; mudar exige mudar o doc antes.
func TestBordaDePalavra_AwsETelegramExigemFronteira(t *testing.T) {
	for _, colado := range []string{
		"xxxx" + expandTemplate("AKIA<<UP#16>>") + "yyyy",
		"xxxx" + expandTemplate("123456789<<COLON>><<MIX#35>>") + "yyyy",
	} {
		if out := redact(colado); out != colado {
			t.Errorf("token colado em letras foi alterado (o doc exige borda): saída %q", out)
		}
	}
}

// TestRegressao_TokenColadoEmLetrasERedigido cobre os padrões sem borda de palavra: um segredo
// colado em letras ainda é redigido e o token cru some (INV-06, DV §7.1). Os valores são expandidos
// em runtime e cercados por letras com strings.Join, então o fonte não contém segredo literal.
func TestRegressao_TokenColadoEmLetrasERedigido(t *testing.T) {
	casos := []struct{ label, tmpl string }{
		{"github_token", "ghp_<<MIX#24>>"},
		{"github_token", "github_pat_<<MIX#24>>"},
		{"api_key", "sk-<<MIX#24>>"},
		{"bearer_token", "Bearer <<MIX#24>>"},
		{"auth_header", "Authorization<<COLON>> <<MIX#24>>"},
		{"stripe_key", "sk_live_<<MIX#16>>"},
		{"google_api_key", "AIza<<MIX#30>>"},
		{"google_api_key", "AQ.Ab<<MIX#30>>"},
		{"google_oauth", "1//<<MIX#20>>"},
		{"meta_token", "EAA<<MIX#24>>"},
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

// TestRelabelagem_VariavelDeProvedorComValorRedigido fixa o comportamento observado do §11.1,
// passo 3 ("a confirmar por teste"): o valor já redigido por `github_token` é casado de novo pela
// atribuição de variável, e o marcador final é `env_secret`. O segredo cru não sobrevive.
func TestRelabelagem_VariavelDeProvedorComValorRedigido(t *testing.T) {
	segredo := expandTemplate("ghp_<<MIX#24>>")
	raw := "GITHUB_TOKEN=" + segredo
	out := Body(raw, Limits{}).Text()
	if strings.Contains(out, segredo) {
		t.Fatalf("token cru sobreviveu: %q", out)
	}
	if !strings.Contains(out, "[REDACTED:") {
		t.Errorf("sem marcador na saída %q", out)
	}
	if redact(out) != out {
		t.Errorf("a saída não é estável na segunda redação: %q", out)
	}
}

// TestRegressao_NegativosDaT013 fixa os negativos citados na correção de T013.
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

// --- Allowlist e padrões extras (prompts §11.3, DV §7.3) ---

// TestAllowlist_ChecadaPorOcorrencia: a entrada da allowlist protege só a ocorrência que a contém;
// a mesma regra continua redigindo as outras ocorrências do texto.
func TestAllowlist_ChecadaPorOcorrencia(t *testing.T) {
	s, err := New(nil, []string{"ALLOWED"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tok1 := expandTemplate("sk-ALLOWED<<MIX#24>>")
	tok2 := expandTemplate("sk-<<MIX#24>>")
	out := s.Body(tok1+" e "+tok2, Limits{}).Text()
	if !strings.Contains(out, tok1) {
		t.Errorf("ocorrência da allowlist foi alterada: %q", out)
	}
	if strings.Contains(out, tok2) {
		t.Errorf("ocorrência fora da allowlist vazou: %q", out)
	}
	if !strings.Contains(out, "[REDACTED:api_key]") {
		t.Errorf("sem marcador para a ocorrência fora da allowlist: %q", out)
	}
}

// TestExtraPatterns_RotuloCustomNoFim cobre o padrão extra: rótulo custom, no fim da ordem, e a
// allowlist também vale para ele.
func TestExtraPatterns_RotuloCustomNoFim(t *testing.T) {
	s, err := New([]string{`corp-[0-9]{4}`}, []string{"corp-1234"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	out := s.Body("id corp-1234 e corp-9999 fim", Limits{}).Text()
	if want := "id corp-1234 e [REDACTED:custom] fim"; out != want {
		t.Errorf("saída = %q, quer %q", out, want)
	}
	rules := s.patterns()
	if last := rules[len(rules)-1]; last.label != "custom" || len(rules) != len(builtinRules)+1 {
		t.Errorf("extra fora do fim da ordem: último rótulo %q, %d padrões", last.label, len(rules))
	}
}

// TestNew_EntradaInvalidaImpedeConstrucao: expressão inválida, extra vazio e entrada vazia na
// allowlist impedem a construção (o serviço não inicia).
func TestNew_EntradaInvalidaImpedeConstrucao(t *testing.T) {
	casos := []struct {
		nome  string
		extra []string
		allow []string
	}{
		{"expressão inválida", []string{"[aberto"}, nil},
		{"extra vazio", []string{""}, nil},
		{"allowlist vazia", nil, []string{""}},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if _, err := New(c.extra, c.allow); err == nil {
				t.Fatal("esperava erro")
			}
		})
	}
}

// TestSanitizer_ZeroValueUsaPadroesEmbutidos garante que o zero value não deixa passar segredo.
func TestSanitizer_ZeroValueUsaPadroesEmbutidos(t *testing.T) {
	segredo := expandTemplate("sk-<<MIX#24>>")
	if out := (Sanitizer{}).Body(segredo, Limits{}).Text(); !strings.Contains(out, "[REDACTED:api_key]") {
		t.Errorf("zero value não redigiu: %q", out)
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
