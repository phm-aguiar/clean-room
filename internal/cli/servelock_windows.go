//go:build windows

package cli

import (
	"os"
	"syscall"
	"unsafe"
)

// Constantes da API do Windows que o pacote syscall não exporta.
const (
	errorSharingViolation   syscall.Errno = 32
	errorLockViolation      syscall.Errno = 33
	errorTooManyOpenFiles   syscall.Errno = 4
	errorInvalidFunction    syscall.Errno = 1
	errorNotSupported       syscall.Errno = 50
	lockfileFailImmediately               = 0x00000001
	lockfileExclusiveLock                 = 0x00000002
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = kernel32.NewProc("LockFileEx")
	procUnlockFileEx = kernel32.NewProc("UnlockFileEx")
)

// lockHeldErrnos são os erros de trava ocupada. LockFileEx com LOCKFILE_FAIL_IMMEDIATELY
// devolve ERROR_LOCK_VIOLATION quando outro handle mantém a trava. Nunca são repetidos.
var lockHeldErrnos = []syscall.Errno{errorLockViolation}

// lockTransientErrnos são as falhas transitórias de abrir ou travar, repetidas até cinco
// tentativas no total.
var lockTransientErrnos = []syscall.Errno{
	syscall.EINTR, syscall.EMFILE, syscall.ENFILE,
	errorSharingViolation, errorTooManyOpenFiles,
}

// lockUnsupportedErrnos indicam que o sistema de arquivos não suporta trava. O servidor
// sobe com aviso e sem a guarda.
var lockUnsupportedErrnos = []syscall.Errno{
	syscall.ENOLCK, syscall.EOPNOTSUPP, syscall.ENOTSUP,
	errorInvalidFunction, errorNotSupported,
}

// lockExclusive toma LockFileEx exclusivo sobre o primeiro byte do arquivo, sem esperar.
// Um byte basta, porque a trava é só um mutex entre processos.
func lockExclusive(f *os.File) error {
	var ov syscall.Overlapped
	r1, _, err := procLockFileEx.Call(
		f.Fd(),
		lockfileExclusiveLock|lockfileFailImmediately,
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&ov)),
	)
	if r1 == 0 {
		return callErr(err)
	}
	return nil
}

// unlockFile libera a trava de LockFileEx sobre o mesmo byte. Fechar o handle também a
// solta.
func unlockFile(f *os.File) error {
	var ov syscall.Overlapped
	r1, _, err := procUnlockFileEx.Call(
		f.Fd(),
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&ov)),
	)
	if r1 == 0 {
		return callErr(err)
	}
	return nil
}

// callErr devolve o erro de uma chamada que falhou. Quando o SO não informa código, usa
// EINVAL, para que a falha continue fechada.
func callErr(err error) error {
	if err != nil {
		return err
	}
	return syscall.EINVAL
}
