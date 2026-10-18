//go:build windows

package cli

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

// enumerateProcesses usa CreateToolhelp32Snapshot com TH32CS_SNAPPROCESS, pela stdlib
// syscall (sem x/sys, ADR-33). Percorre a lista com Process32First e Process32Next até
// ERROR_NO_MORE_FILES, que é o fim normal da enumeração.
func enumerateProcesses() ([]procInfo, error) {
	snap, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("CreateToolhelp32Snapshot: %w", err)
	}
	defer func() { _ = syscall.CloseHandle(snap) }()

	var entry syscall.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	var procs []procInfo
	err = syscall.Process32First(snap, &entry)
	for err == nil {
		procs = append(procs, procInfo{
			pid:  int(entry.ProcessID),
			name: syscall.UTF16ToString(entry.ExeFile[:]),
		})
		err = syscall.Process32Next(snap, &entry)
	}
	if !errors.Is(err, syscall.ERROR_NO_MORE_FILES) {
		return nil, fmt.Errorf("enumerar processos (Process32): %w", err)
	}
	return procs, nil
}
