//go:build !windows

package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// enumerateProcesses lê /proc: cada diretório com nome numérico é um processo, e
// /proc/<pid>/comm traz o nome do executável. Um processo que termina entre a
// listagem e a leitura (ENOENT) é ignorado. Qualquer outra falha de leitura é
// propagada, para a guarda recusar a operação. Sistemas sem /proc (fora do Linux)
// falham aqui.
func enumerateProcesses() ([]procInfo, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("ler /proc: %w", err)
	}
	procs := make([]procInfo, 0, len(entries))
	for _, ent := range entries {
		pid, err := strconv.Atoi(ent.Name())
		if err != nil || pid <= 0 {
			continue
		}
		raw, err := os.ReadFile(filepath.Join("/proc", ent.Name(), "comm"))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("ler nome do processo %d: %w", pid, err)
		}
		procs = append(procs, procInfo{pid: pid, name: strings.TrimSpace(string(raw))})
	}
	return procs, nil
}
