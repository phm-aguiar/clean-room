// Guarda de processo vivo (INV-09): reset, restore, reindex e uninstall --purge-data
// checam, antes de agir sobre o data_dir, se outro processo ai-memory está vivo.
//
// Mecanismo: o processo servidor grava o próprio PID em <data_dir>/ai-memory.pid.
// A guarda lê esse arquivo e verifica se o processo existe. A checagem por SO fica
// em liveguard_windows.go e liveguard_other.go (ADR-33: só stdlib, sem x/sys).
//
// Limitação conhecida: PIDs podem ser reutilizados pelo SO. Um PID antigo que
// foi reaproveitado por outro processo faz a guarda recusar a operação. Isso é
// conservador (falha fechada) e a mensagem de erro traz o PID para diagnóstico.
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

// PIDFileName é o marcador de PID do processo servidor dentro do data_dir.
const PIDFileName = "ai-memory.pid"

// ErrProcessLive indica que há um processo ai-memory vivo usando o data_dir.
var ErrProcessLive = errors.New("processo ai-memory vivo")

// ErrPIDFileFormat indica que o marcador de PID existe mas não contém um PID válido.
var ErrPIDFileFormat = errors.New("arquivo de PID inválido")

// CheckNoLiveProcess devolve nil quando é seguro agir sobre dataDir: o marcador
// não existe ou o PID nele não está vivo. Devolve erro com %w de ErrProcessLive
// quando o processo está vivo, e de ErrPIDFileFormat quando o marcador está
// corrompido. Outros erros (leitura ou consulta ao SO) são propagados com %w.
func CheckNoLiveProcess(dataDir string) error {
	path := filepath.Join(dataDir, PIDFileName)
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("ler marcador de PID %s: %w", path, err)
	}
	pid, err := parsePID(raw)
	if err != nil {
		return fmt.Errorf("marcador de PID %s: %w", path, err)
	}
	alive, err := processAlive(pid)
	if err != nil {
		return fmt.Errorf("verificar processo %d: %w", pid, err)
	}
	if alive {
		return fmt.Errorf("%w: pid %d usa o data_dir %s", ErrProcessLive, pid, dataDir)
	}
	return nil
}

// parsePID aceita apenas inteiros positivos de 32 bits. PID 0 e negativos são
// rejeitados: kill(0, 0) no Unix atinge o grupo do processo, não um PID.
func parsePID(raw []byte) (int, error) {
	s := strings.TrimSpace(string(raw))
	n, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%w: conteúdo %q não é um inteiro de 32 bits", ErrPIDFileFormat, s)
	}
	if n <= 0 {
		return 0, fmt.Errorf("%w: PID %d não é positivo", ErrPIDFileFormat, n)
	}
	return int(n), nil
}
