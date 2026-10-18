package cli

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
)

// fakeProcesses troca a enumeração por uma lista fixa até o fim do teste.
func fakeProcesses(t *testing.T, procs ...procInfo) {
	t.Helper()
	prev := listProcesses
	listProcesses = func() ([]procInfo, error) { return procs, nil }
	t.Cleanup(func() { listProcesses = prev })
}

// fakeEnumError troca a enumeração por uma falha até o fim do teste.
func fakeEnumError(t *testing.T, err error) {
	t.Helper()
	prev := listProcesses
	listProcesses = func() ([]procInfo, error) { return nil, err }
	t.Cleanup(func() { listProcesses = prev })
}

func TestLiveGuard_SemOutroAIMemory(t *testing.T) {
	fakeProcesses(t, procInfo{pid: 100, name: "bash"}, procInfo{pid: 101, name: "ai-memory-hook"})
	if err := CheckNoLiveProcess(t.TempDir()); err != nil {
		t.Fatalf("sem outro ai-memory esperava nil, veio %v", err)
	}
}

func TestLiveGuard_OutroAIMemoryVivo(t *testing.T) {
	const pid = 4242
	fakeProcesses(t, procInfo{pid: pid, name: "ai-memory"})
	err := CheckNoLiveProcess(t.TempDir())
	if !errors.Is(err, ErrProcessLive) {
		t.Fatalf("outro ai-memory esperava ErrProcessLive, veio %v", err)
	}
	if !strings.Contains(err.Error(), strconv.Itoa(pid)) {
		t.Fatalf("mensagem deve trazer o pid %d: %v", pid, err)
	}
}

func TestLiveGuard_NomeComExtensaoEMaiusculas(t *testing.T) {
	fakeProcesses(t, procInfo{pid: 4243, name: "AI-MEMORY.EXE"})
	if err := CheckNoLiveProcess(t.TempDir()); !errors.Is(err, ErrProcessLive) {
		t.Fatalf("AI-MEMORY.EXE esperava ErrProcessLive, veio %v", err)
	}
}

func TestLiveGuard_PropriaPIDIgnorada(t *testing.T) {
	fakeProcesses(t, procInfo{pid: os.Getpid(), name: "ai-memory"})
	if err := CheckNoLiveProcess(t.TempDir()); err != nil {
		t.Fatalf("o próprio processo não conta como outro, esperava nil, veio %v", err)
	}
}

func TestLiveGuard_FalhaDeEnumeracaoFechaAPorta(t *testing.T) {
	boom := errors.New("enumeração indisponível")
	fakeEnumError(t, boom)
	err := CheckNoLiveProcess(t.TempDir())
	if !errors.Is(err, boom) {
		t.Fatalf("falha de enumeração esperava erro encadeado, veio %v", err)
	}
	if errors.Is(err, ErrProcessLive) {
		t.Fatalf("falha de enumeração não é ErrProcessLive: %v", err)
	}
}

func TestIsAIMemoryName(t *testing.T) {
	casos := map[string]bool{
		"ai-memory":      true,
		"ai-memory.exe":  true,
		"AI-Memory.exe":  true,
		"ai-memory-hook": false,
		"ai-memory2":     false,
		"ai":             false,
		"":               false,
		"bash":           false,
	}
	for nome, esperado := range casos {
		if got := isAIMemoryName(nome); got != esperado {
			t.Errorf("isAIMemoryName(%q) = %v, esperava %v", nome, got, esperado)
		}
	}
}

// TestLiveGuard_EnumeraProcessoAtual exercita a enumeração real: o processo de teste
// precisa aparecer na lista, com nome não vazio.
func TestLiveGuard_EnumeraProcessoAtual(t *testing.T) {
	procs, err := enumerateProcesses()
	if err != nil {
		t.Fatalf("enumerateProcesses: %v", err)
	}
	for _, p := range procs {
		if p.pid == os.Getpid() {
			if p.name == "" {
				t.Fatalf("nome vazio para o próprio processo %d", p.pid)
			}
			return
		}
	}
	t.Fatalf("o próprio processo %d não apareceu na enumeração (%d processos)", os.Getpid(), len(procs))
}
