//go:build !windows

package cli

import (
	"errors"
	"syscall"
)

// processAlive usa kill(pid, 0): não envia sinal, só testa existência e permissão.
// ESRCH significa que o processo não existe. EPERM significa que existe, mas
// pertence a outro usuário, e nesse caso é tratado como vivo (falha fechada).
func processAlive(pid int) (bool, error) {
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, syscall.ESRCH) {
		return false, nil
	}
	if errors.Is(err, syscall.EPERM) {
		return true, nil
	}
	return false, err
}
