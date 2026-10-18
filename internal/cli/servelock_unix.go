//go:build !windows

package cli

import (
	"os"
	"syscall"
)

// lockHeldErrnos são os erros de trava ocupada. flock com LOCK_NB devolve EWOULDBLOCK
// (igual a EAGAIN) quando outro descritor mantém a trava. Nunca são repetidos.
var lockHeldErrnos = []syscall.Errno{syscall.EWOULDBLOCK}

// lockTransientErrnos são as falhas transitórias de abrir ou travar, repetidas até cinco
// tentativas no total.
var lockTransientErrnos = []syscall.Errno{syscall.EINTR, syscall.EMFILE, syscall.ENFILE}

// lockUnsupportedErrnos indicam que o sistema de arquivos não suporta trava. O servidor
// sobe com aviso e sem a guarda.
var lockUnsupportedErrnos = []syscall.Errno{syscall.ENOLCK, syscall.EOPNOTSUPP, syscall.ENOTSUP}

// lockExclusive toma a trava exclusiva sem esperar (LOCK_EX|LOCK_NB). A trava é do
// descritor e o SO a libera quando o processo sai.
func lockExclusive(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

// unlockFile solta a trava do descritor. Fechar o arquivo também a solta.
func unlockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
