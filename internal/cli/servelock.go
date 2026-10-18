// Trava de instância única do `serve` (docs/windows.md § Single instance, docs/cli.md §3.1).
//
// Mecanismo: trava exclusiva do sistema operacional sobre <data_dir>/.serve.lock, mantida
// enquanto o processo vive. O SO a libera ao sair, inclusive após queda, então um servidor
// que caiu nunca trava ninguém. A trava em si fica em servelock_unix.go (syscall.Flock com
// LOCK_EX|LOCK_NB) e servelock_windows.go (LockFileEx com LOCKFILE_EXCLUSIVE_LOCK|
// LOCKFILE_FAIL_IMMEDIATELY). Só a stdlib (ADR-33).
//
// Detentor: o PID é gravado em <data_dir>/.serve.lock.holder logo após a trava. O arquivo
// é informativo e de melhor esforço; a trava é a guarda real. A recusa cita o PID lido dele.
// Na liberação, o arquivo só é removido se ainda contiver o PID deste processo.
//
// Classificação dos erros de abrir e travar:
//   - ocupada (EWOULDBLOCK; ERROR_LOCK_VIOLATION): nunca repetida. Recusa com o PID do
//     detentor. Só --force sobe, com aviso no log.
//   - transitória (EINTR, EMFILE, ENFILE; ERROR_SHARING_VIOLATION, ERROR_TOO_MANY_OPEN_FILES):
//     repetida até serveLockAttempts tentativas no total, com serveLockBackoff entre elas.
//     Esgotadas, a falha é erro (recusa).
//   - sem suporte do sistema de arquivos (ENOLCK, EOPNOTSUPP, ENOTSUP; ERROR_INVALID_FUNCTION,
//     ERROR_NOT_SUPPORTED): aviso no log e o servidor sobe sem trava. Vale só para a trava,
//     não para a abertura do arquivo.
//   - qualquer outro erro: recusa o boot (falha fechada). Decisão do projeto: --force não
//     contorna este caso, porque não há trava ocupada a ignorar.
//
// Decisão de implementação: a abertura usa MkdirAll do diretório de dados, porque a trava
// é o primeiro passo que toca o disco no boot e o diretório pode não existir em uma
// instalação nova.
package cli

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	// serveLockFile é o arquivo que recebe a trava exclusiva.
	serveLockFile = ".serve.lock"
	// serveHolderFile guarda o PID do detentor. É informativo.
	serveHolderFile = ".serve.lock.holder"
	// serveLockAttempts é o total de tentativas para erros transitórios, contando a primeira.
	serveLockAttempts = 5
	// serveLockBackoff é a espera entre tentativas transitórias.
	serveLockBackoff = 25 * time.Millisecond
)

// ErrInstanceLocked indica que outro processo mantém a trava do diretório de dados.
var ErrInstanceLocked = errors.New("outra instância do servidor usa este diretório de dados")

// sysOpen, sysLock e sysUnlock são as operações do SO. Os testes as trocam para simular
// erros sem depender do sistema operacional. A trava real fica nos arquivos por plataforma.
var (
	sysOpen = func(path string) (*os.File, error) {
		return os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	}
	sysLock   = lockExclusive
	sysUnlock = unlockFile
)

// ServeLock é a trava de instância única de um processo serve. Um ServeLock sem arquivo
// (campo f nil) não guarda nada: é o caso --force ou sistema de arquivos sem suporte.
type ServeLock struct {
	f          *os.File
	holderPath string
}

// Held informa se este processo mantém a trava exclusiva.
func (l *ServeLock) Held() bool {
	return l != nil && l.f != nil
}

// AcquireServeLock toma a trava de instância única em dataDir.
//
// Devolve erro com %w de ErrInstanceLocked quando outro processo a mantém e force é false.
// A mensagem traz o PID do detentor. Com force true, sobe com aviso no log e devolve um
// ServeLock sem trava. Sistema de arquivos sem suporte devolve um ServeLock sem trava com
// aviso. Qualquer outro erro é devolvido sem bypass por force.
//
// log pode ser nil (usa slog.Default).
func AcquireServeLock(dataDir string, force bool, log *slog.Logger) (*ServeLock, error) {
	if log == nil {
		log = slog.Default()
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("criar diretório de dados: %w", err)
	}
	lockPath := filepath.Join(dataDir, serveLockFile)
	holderPath := filepath.Join(dataDir, serveHolderFile)

	var f *os.File
	err := retryTransient(func() error {
		var openErr error
		f, openErr = sysOpen(lockPath)
		return openErr
	})
	if err != nil {
		return nil, fmt.Errorf("abrir %s: %w", lockPath, err)
	}

	err = retryTransient(func() error { return sysLock(f) })
	switch {
	case err == nil:
		writeHolder(holderPath, log)
		return &ServeLock{f: f, holderPath: holderPath}, nil

	case isLockErrno(err, lockUnsupportedErrnos):
		_ = f.Close()
		log.Warn("sistema de arquivos sem suporte a trava; o servidor sobe sem a guarda de instância única",
			"path", lockPath, "err", err)
		return &ServeLock{}, nil

	case isLockErrno(err, lockHeldErrnos):
		_ = f.Close()
		holder := readHolder(holderPath)
		if !force {
			return nil, fmt.Errorf("%w: detentor pid %s; confirme que o processo não existe mais antes de usar --force (%s)",
				ErrInstanceLocked, holder, lockPath)
		}
		log.Warn("--force: subindo com a trava ocupada por outro processo; dados podem ser corrompidos se ele ainda estiver ativo",
			"path", lockPath, "holder_pid", holder)
		return &ServeLock{}, nil

	default:
		_ = f.Close()
		return nil, fmt.Errorf("travar %s: %w", lockPath, err)
	}
}

// Release libera a trava. Remove o holder só se ainda contiver o PID deste processo, antes
// de soltar a trava, para não apagar o registro de outro detentor. Chamar em ServeLock sem
// trava não faz nada.
func (l *ServeLock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	var errs []error
	if err := removeOwnHolder(l.holderPath); err != nil {
		errs = append(errs, err)
	}
	if err := sysUnlock(l.f); err != nil {
		errs = append(errs, fmt.Errorf("destravar: %w", err))
	}
	if err := l.f.Close(); err != nil {
		errs = append(errs, fmt.Errorf("fechar trava: %w", err))
	}
	l.f = nil
	return errors.Join(errs...)
}

// retryTransient executa op. Repete só quando o erro é transitório, até serveLockAttempts
// tentativas no total, com serveLockBackoff entre elas. Erro ocupado ou qualquer outro é
// devolvido na hora. Esgotadas as tentativas, devolve o último erro envolvido.
func retryTransient(op func() error) error {
	var err error
	for attempt := 1; attempt <= serveLockAttempts; attempt++ {
		err = op()
		if err == nil || !isLockErrno(err, lockTransientErrnos) {
			return err
		}
		if attempt < serveLockAttempts {
			time.Sleep(serveLockBackoff)
		}
	}
	return fmt.Errorf("%d tentativas transitórias esgotadas: %w", serveLockAttempts, err)
}

// isLockErrno informa se err contém algum dos errnos de set.
func isLockErrno(err error, set []syscall.Errno) bool {
	for _, e := range set {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}

// writeHolder grava o PID deste processo. Falha só vira aviso: o holder é informativo.
func writeHolder(path string, log *slog.Logger) {
	content := strconv.Itoa(os.Getpid()) + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		log.Warn("não gravou o arquivo de detentor da trava", "path", path, "err", err)
	}
}

// readHolder devolve o PID gravado no holder, ou "desconhecido" quando não é legível.
func readHolder(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "desconhecido"
	}
	pid := strings.TrimSpace(string(raw))
	if pid == "" {
		return "desconhecido"
	}
	return pid
}

// removeOwnHolder remove o holder só se ele contiver o PID deste processo. Holder ausente
// ou de outro PID não é erro.
func removeOwnHolder(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("ler detentor: %w", err)
	}
	if strings.TrimSpace(string(raw)) != strconv.Itoa(os.Getpid()) {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remover detentor: %w", err)
	}
	return nil
}
