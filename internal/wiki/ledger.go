package wiki

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
	"unicode"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// logFileName é o ledger único do projeto (ADR-21).
const logFileName = "log.md"

// Eventos aceitos na linha do log (ADR-21).
const (
	eventPageWrite  = "page-write"
	eventPageDelete = "page-delete"
)

// ErrInvalidLogLine é a causa de AppendLogLine quando o evento não é um dos aceitos ou o título
// está vazio, tem quebra de linha ou tem caractere de controle.
var ErrInvalidLogLine = errors.New("linha de log inválida")

// AppendLogLine acrescenta uma linha ao `log.md` do projeto (ADR-21), no formato
//
//	## [AAAA-MM-DDTHH:MM:SSZ] evento | título
//
// com a data em UTC, RFC 3339 e sem fração de segundo. Evento é `page-write` ou `page-delete`.
// Título é texto já sanitizado pelo chamador: vazio, quebra de linha (LF, CR, U+2028, U+2029) ou
// caractere de controle são recusados com ErrInvalidLogLine.
//
// Os argumentos são validados antes de qualquer acesso ao disco. O arquivo é aberto com O_APPEND
// e criado se faltar. Não usa Write (atômico): Write reescreveria o arquivo inteiro, e o log é
// somente acréscimo (wiki-layout-contract (c)). Não lê o arquivo. O fsync ao fim garante que a
// linha esteja no disco antes do commit feito pelo chamador (ADR-22).
//
// O caminho sai de pathFor, com confinamento. Não cria commit. Quem chama devolve `log.md` para o
// commit, no mesmo commit do evento (ADR-22). Falha ao anexar é responsabilidade de quem chama
// tratar como aviso.
func AppendLogLine(root string, scope domain.Scope, event, title string, at time.Time) error {
	if event != eventPageWrite && event != eventPageDelete {
		return fmt.Errorf("%w: evento %q", ErrInvalidLogLine, event)
	}
	if err := checkLogTitle(title); err != nil {
		return err
	}
	path, err := pathFor(root, scope, logPage())
	if err != nil {
		return err
	}
	if err := ensureDir(filepath.Dir(path)); err != nil {
		return err
	}
	line := "## [" + at.UTC().Format(time.RFC3339) + "] " + event + " | " + title + "\n"

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, fileMode)
	if err != nil {
		return fmt.Errorf("wiki: abrir %s: %w", path, err)
	}
	if _, err := f.WriteString(line); err != nil {
		_ = f.Close()
		return fmt.Errorf("wiki: anexar a %s: %w", path, err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("wiki: sync de %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("wiki: fechar %s: %w", path, err)
	}
	return nil
}

// checkLogTitle recusa título vazio e qualquer texto que quebre a linha do ledger: caractere de
// controle (inclui LF, CR e TAB) e os separadores de linha Unicode U+2028 e U+2029.
func checkLogTitle(title string) error {
	if title == "" {
		return fmt.Errorf("%w: título vazio", ErrInvalidLogLine)
	}
	for _, r := range title {
		if unicode.IsControl(r) || r == ' ' || r == ' ' {
			return fmt.Errorf("%w: título com caractere de controle U+%04X", ErrInvalidLogLine, r)
		}
	}
	return nil
}
