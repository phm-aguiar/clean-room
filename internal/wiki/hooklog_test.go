package wiki

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

func TestHookLogFormataLinha(t *testing.T) {
	ts := time.Date(2026, 10, 13, 8, 5, 9, 0, time.UTC)
	got, err := FormatHookLogLine(ts, domain.HookUserPrompt, "Pergunta do usuário")
	if err != nil {
		t.Fatalf("FormatHookLogLine: erro inesperado: %v", err)
	}
	want := "## [2026-10-13T08:05:09Z] user-prompt | Pergunta do usuário\n"
	if got != want {
		t.Errorf("linha = %q, quer %q", got, want)
	}
}

func TestHookLogCarimboEhUTC(t *testing.T) {
	// 11:05:09 em UTC-3 é 14:05:09 em UTC.
	ts := time.Date(2026, 10, 13, 11, 5, 9, 0, time.FixedZone("-03", -3*3600))
	got, err := FormatHookLogLine(ts, domain.HookStop, "fim")
	if err != nil {
		t.Fatalf("FormatHookLogLine: erro inesperado: %v", err)
	}
	if want := "## [2026-10-13T14:05:09Z] stop | fim\n"; got != want {
		t.Errorf("linha = %q, quer %q", got, want)
	}
}

func TestHookLogEventoNaoCanonicoRecusado(t *testing.T) {
	ts := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	for _, ev := range []domain.HookEvent{"", "bogus", "userprompt", "UserPrompt"} {
		if _, err := FormatHookLogLine(ts, ev, "x"); err == nil {
			t.Errorf("FormatHookLogLine(%q) sem erro, quer erro", ev)
		}
	}
}

func TestHookLogEventosDeSubagenteUsamNomeCanonico(t *testing.T) {
	ts := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	got, err := FormatHookLogLine(ts, domain.HookSubagentStart, "agente")
	if err != nil {
		t.Fatalf("FormatHookLogLine: erro inesperado: %v", err)
	}
	if want := "## [2026-10-13T00:00:00Z] subagent-start | agente\n"; got != want {
		t.Errorf("linha = %q, quer %q", got, want)
	}
}

func TestHookLogTrocaQuebrasDeLinhaPorEspaco(t *testing.T) {
	ts := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	got, err := FormatHookLogLine(ts, domain.HookNotification, "uma\nduas\r\ntrês\tquatro")
	if err != nil {
		t.Fatalf("FormatHookLogLine: erro inesperado: %v", err)
	}
	if want := "## [2026-10-13T00:00:00Z] notification | uma duas  três quatro\n"; got != want {
		t.Errorf("linha = %q, quer %q", got, want)
	}
	if strings.Count(got, "\n") != 1 {
		t.Errorf("a linha tem quebra interna: %q", got)
	}
}

func TestHookLogTruncaTituloEmCaracteresNaoBytes(t *testing.T) {
	ts := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		nome   string
		titulo string
		want   string
	}{
		{"ascii exatamente 120", strings.Repeat("a", 120), strings.Repeat("a", 120)},
		{"ascii 119 fica", strings.Repeat("a", 119), strings.Repeat("a", 119)},
		{"ascii 121 corta", strings.Repeat("a", 121), strings.Repeat("a", 120)},
		{"multibyte 150 vira 120", strings.Repeat("é", 150), strings.Repeat("é", 120)},
		{"multibyte 120 inteiro", strings.Repeat("日", 120), strings.Repeat("日", 120)},
	} {
		t.Run(tc.nome, func(t *testing.T) {
			got, err := FormatHookLogLine(ts, domain.HookPreToolUse, tc.titulo)
			if err != nil {
				t.Fatalf("FormatHookLogLine: erro inesperado: %v", err)
			}
			prefixo := "## [2026-10-13T00:00:00Z] pre-tool-use | "
			if !strings.HasPrefix(got, prefixo) || !strings.HasSuffix(got, "\n") {
				t.Fatalf("forma da linha errada: %q", got)
			}
			titulo := strings.TrimSuffix(strings.TrimPrefix(got, prefixo), "\n")
			if titulo != tc.want {
				t.Errorf("título = %q (%d caracteres), quer %q (%d caracteres)",
					titulo, utf8.RuneCountInString(titulo), tc.want, utf8.RuneCountInString(tc.want))
			}
			if n := utf8.RuneCountInString(titulo); n > 120 {
				t.Errorf("título com %d caracteres, máximo 120", n)
			}
		})
	}
}

func TestHookLogNomeDoMesPeloUTC(t *testing.T) {
	cases := []struct {
		nome string
		ts   time.Time
		want string
	}{
		{"meio do mês", time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC), "log-2026-10.md"},
		{"virada para o próximo mês em UTC", time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC), "log-2026-01.md"},
		{"primeiro instante do mês seguinte", time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), "log-2026-02.md"},
		{"local ainda em março, UTC já em abril", time.Date(2026, 3, 31, 23, 30, 0, 0, time.FixedZone("-03", -3*3600)), "log-2026-04.md"},
		{"local já em abril, UTC ainda em março", time.Date(2026, 4, 1, 0, 30, 0, 0, time.FixedZone("+05", 5*3600)), "log-2026-03.md"},
	}
	for _, c := range cases {
		t.Run(c.nome, func(t *testing.T) {
			if got := HookLogFileName(c.ts); got != c.want {
				t.Errorf("HookLogFileName = %q, quer %q", got, c.want)
			}
			if !ledgerName(HookLogFileName(c.ts)) {
				t.Errorf("nome %q não tem a forma do ledger", HookLogFileName(c.ts))
			}
		})
	}
}

func TestHookLogViradaDeMesGravaNoArquivoCerto(t *testing.T) {
	dir := t.TempDir()
	antes, err := AppendHookLog(dir, time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC), domain.HookStop, "fim de janeiro")
	if err != nil {
		t.Fatalf("AppendHookLog: %v", err)
	}
	depois, err := AppendHookLog(dir, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), domain.HookStop, "início de fevereiro")
	if err != nil {
		t.Fatalf("AppendHookLog: %v", err)
	}
	if filepath.Base(antes) != "log-2026-01.md" || filepath.Base(depois) != "log-2026-02.md" {
		t.Errorf("arquivos = %q e %q, quer log-2026-01.md e log-2026-02.md", filepath.Base(antes), filepath.Base(depois))
	}
	if got := readHookLogFile(t, antes); got != "## [2026-01-31T23:59:59Z] stop | fim de janeiro\n" {
		t.Errorf("conteúdo de janeiro = %q", got)
	}
}

func TestHookLogAcrescimoSemReescrita(t *testing.T) {
	dir := t.TempDir()
	ts := time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)

	path, err := AppendHookLog(dir, ts, domain.HookSessionStart, "primeira")
	if err != nil {
		t.Fatalf("AppendHookLog 1: %v", err)
	}
	primeiro := readHookLogFile(t, path)

	if _, err := AppendHookLog(dir, ts.Add(time.Minute), domain.HookUserPrompt, "segunda"); err != nil {
		t.Fatalf("AppendHookLog 2: %v", err)
	}
	segundo := readHookLogFile(t, path)

	if !strings.HasPrefix(segundo, primeiro) {
		t.Errorf("o segundo acréscimo reescreveu o início do arquivo:\nantes:  %q\ndepois: %q", primeiro, segundo)
	}
	want := "## [2026-10-13T09:00:00Z] session-start | primeira\n" +
		"## [2026-10-13T09:01:00Z] user-prompt | segunda\n"
	if segundo != want {
		t.Errorf("conteúdo = %q, quer %q", segundo, want)
	}
}

func TestHookLogPreservaConteudoExistente(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log-2026-10.md")
	existente := "## [2026-10-01T00:00:00Z] stop | antigo\n"
	if err := os.WriteFile(path, []byte(existente), 0o644); err != nil {
		t.Fatalf("preparar arquivo: %v", err)
	}
	if _, err := AppendHookLog(dir, time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC), domain.HookStop, "novo"); err != nil {
		t.Fatalf("AppendHookLog: %v", err)
	}
	got := readHookLogFile(t, path)
	want := existente + "## [2026-10-13T00:00:00Z] stop | novo\n"
	if got != want {
		t.Errorf("conteúdo = %q, quer %q", got, want)
	}
}

func TestHookLogNaoCriaLogNemIndice(t *testing.T) {
	dir := t.TempDir()
	if _, err := AppendHookLog(dir, time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC), domain.HookStop, "x"); err != nil {
		t.Fatalf("AppendHookLog: %v", err)
	}
	entradas, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entradas) != 1 || entradas[0].Name() != "log-2026-10.md" {
		var nomes []string
		for _, e := range entradas {
			nomes = append(nomes, e.Name())
		}
		t.Errorf("arquivos no diretório = %v, quer só [log-2026-10.md]", nomes)
	}
}

func TestHookLogArquivoEhReconhecidoComoLedger(t *testing.T) {
	dir := t.TempDir()
	path, err := AppendHookLog(dir, time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC), domain.HookSessionStart, "início")
	if err != nil {
		t.Fatalf("AppendHookLog: %v", err)
	}
	if _, err := AppendHookLog(dir, time.Date(2026, 10, 13, 0, 0, 1, 0, time.UTC), domain.HookPreToolUse, "Read src/x.go"); err != nil {
		t.Fatalf("AppendHookLog 2: %v", err)
	}
	ok, err := isLedgerFile(path)
	if err != nil {
		t.Fatalf("isLedgerFile: %v", err)
	}
	if !ok {
		t.Errorf("isLedgerFile(%q) = false, quer true para arquivo criado por AppendHookLog", path)
	}
}

// readHookLogFile lê o arquivo inteiro e falha o teste se a leitura der erro.
func readHookLogFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ler %s: %v", path, err)
	}
	return string(b)
}
