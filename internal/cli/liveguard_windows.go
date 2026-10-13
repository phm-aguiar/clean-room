//go:build windows

package cli

import (
	"errors"
	"syscall"
)

const (
	// PROCESS_QUERY_LIMITED_INFORMATION: basta para GetExitCodeProcess.
	processQueryLimitedInformation = 0x1000
	// STILL_ACTIVE: código de saída de processo que ainda não terminou.
	stillActive = 259
	// ERROR_INVALID_PARAMETER: OpenProcess com PID inexistente.
	errorInvalidParameter = syscall.Errno(87)
	// ERROR_ACCESS_DENIED: o processo existe, mas sem permissão de consulta.
	errorAccessDenied = syscall.Errno(5)
)

// processAlive usa OpenProcess e GetExitCodeProcess (stdlib syscall). Um processo
// existe com código STILL_ACTIVE. ERROR_ACCESS_DENIED também conta como vivo
// (falha fechada). ERROR_INVALID_PARAMETER indica PID inexistente.
func processAlive(pid int) (bool, error) {
	h, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		if errors.Is(err, errorInvalidParameter) {
			return false, nil
		}
		if errors.Is(err, errorAccessDenied) {
			return true, nil
		}
		return false, err
	}
	defer syscall.CloseHandle(h)
	var code uint32
	if err := syscall.GetExitCodeProcess(h, &code); err != nil {
		return false, err
	}
	return code == stillActive, nil
}
