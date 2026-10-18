package wiki

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// Ledger mensal de eventos de hook (docs/lifecycle-event-ingestion.md §13, docs/wiki-files-layer.md
// §9, docs/complementary-contracts.md §4). Cada evento processado acrescenta uma linha ao arquivo
// `log-AAAA-MM.md` do mês do evento, em UTC. A linha tem a forma `## [ts] evento | título`.

// hookLogTitleMaxRunes é o teto do título em caracteres Unicode (não em bytes), como manda LEI §13.
const hookLogTitleMaxRunes = 120

// hookLogTimeLayout é o carimbo da linha: AAAA-MM-DDTHH:MM:SSZ, sempre em UTC.
const hookLogTimeLayout = "2006-01-02T15:04:05Z"

// HookLogFileName devolve o nome do ledger do mês de ts, pelo horário em UTC: `log-AAAA-MM.md`.
// O nome casa com ledgerName (ledger_detect.go).
func HookLogFileName(ts time.Time) string {
	return "log-" + ts.UTC().Format("2006-01") + ".md"
}

// FormatHookLogLine monta a linha do ledger, já com o `\n` final: `## [ts] evento | título`.
// O evento precisa ser a forma canônica de domain.HookEvent (kebab-case). O título tem as quebras
// e os demais caracteres de controle trocados por espaço, e é cortado em 120 caracteres Unicode.
func FormatHookLogLine(ts time.Time, ev domain.HookEvent, titulo string) (string, error) {
	canonico, err := domain.ParseHookEvent(string(ev))
	if err != nil {
		return "", err
	}
	if canonico != ev {
		return "", fmt.Errorf("evento de hook não canônico: %q", ev)
	}
	return "## [" + ts.UTC().Format(hookLogTimeLayout) + "] " + string(ev) + " | " +
		hookLogTitle(titulo) + "\n", nil
}

// hookLogTitle prepara o título: caracteres de controle viram espaço e o resultado é cortado em
// hookLogTitleMaxRunes caracteres Unicode.
func hookLogTitle(s string) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n == hookLogTitleMaxRunes {
			break
		}
		if unicode.IsControl(r) {
			r = ' '
		}
		b.WriteRune(r)
		n++
	}
	return b.String()
}

// AppendHookLog acrescenta a linha do evento ao ledger do mês de ts, dentro de dir (a raiz do
// projeto na wiki). Devolve o caminho do arquivo. O arquivo só cresce: a linha é anexada e nada
// existente é reescrito. Não cria log.md nem index.md.
func AppendHookLog(dir string, ts time.Time, ev domain.HookEvent, titulo string) (string, error) {
	line, err := FormatHookLogLine(ts, ev, titulo)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, HookLogFileName(ts))
	if err := appendHookLogLine(path, line); err != nil {
		return "", err
	}
	return path, nil
}

// appendHookLogLine anexa line ao fim de path, criando o arquivo se faltar. A linha vai em uma
// única escrita, para que escritas em modo append não se intercalem (LEI §13).
func appendHookLogLine(path, line string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("abrir ledger de hook: %w", err)
	}
	if _, err := f.WriteString(line); err != nil {
		f.Close()
		return fmt.Errorf("anexar linha ao ledger de hook: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("fechar ledger de hook: %w", err)
	}
	return nil
}
