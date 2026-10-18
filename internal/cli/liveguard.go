// Guarda de processo vivo (INV-09): reset, restore, reindex e uninstall --purge-data
// checam, antes de agir sobre o data_dir, se há outro processo ai-memory em execução.
//
// Mecanismo: enumeração dos processos do sistema operacional. Linux e demais sistemas
// POSIX leem /proc (liveguard_other.go); Windows usa CreateToolhelp32Snapshot
// (liveguard_windows.go). Ambos usam só a stdlib (ADR-33). Um processo é ai-memory
// quando o nome do executável é "ai-memory" (com ou sem ".exe"). O próprio processo
// é descartado, porque o comando que chama a guarda também é ai-memory.
//
// A enumeração não sabe qual data_dir cada processo usa. Por isso a guarda vale para
// o sistema inteiro: qualquer outro ai-memory em execução recusa a operação.
//
// Falha fechada: se a enumeração falhar (por exemplo, /proc ausente), a operação é
// recusada com o erro encadeado.
package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// aiMemoryProcessName é o nome do executável do ai-memory, sem extensão.
const aiMemoryProcessName = "ai-memory"

// ErrProcessLive indica que há outro processo ai-memory em execução.
var ErrProcessLive = errors.New("processo ai-memory vivo")

// procInfo é um processo do sistema: PID e nome do executável, sem caminho.
type procInfo struct {
	pid  int
	name string
}

// listProcesses enumera os processos do sistema. Os testes a trocam por uma lista
// fixa. A implementação real fica em liveguard_other.go e liveguard_windows.go.
var listProcesses = enumerateProcesses

// CheckNoLiveProcess devolve nil quando nenhum outro processo ai-memory está em
// execução. Devolve erro com %w de ErrProcessLive quando há algum, e erro encadeado
// quando a enumeração falha. dataDir só aparece na mensagem de diagnóstico.
func CheckNoLiveProcess(dataDir string) error {
	procs, err := listProcesses()
	if err != nil {
		return fmt.Errorf("enumerar processos do sistema: %w", err)
	}
	if pid, ok := findOtherAIMemory(procs, os.Getpid()); ok {
		return fmt.Errorf("%w: pid %d é outro processo ai-memory; data_dir %s", ErrProcessLive, pid, dataDir)
	}
	return nil
}

// findOtherAIMemory devolve o PID do primeiro processo ai-memory diferente de self.
func findOtherAIMemory(procs []procInfo, self int) (int, bool) {
	for _, p := range procs {
		if p.pid == self {
			continue
		}
		if isAIMemoryName(p.name) {
			return p.pid, true
		}
	}
	return 0, false
}

// isAIMemoryName aceita o nome do executável com ou sem ".exe", sem diferenciar
// maiúsculas.
func isAIMemoryName(name string) bool {
	return strings.EqualFold(name, aiMemoryProcessName) ||
		strings.EqualFold(name, aiMemoryProcessName+".exe")
}
