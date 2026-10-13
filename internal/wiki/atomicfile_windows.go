//go:build windows

package wiki

import (
	"os"
	"syscall"
	"time"
)

// applyMode não se aplica no Windows: a permissão 0644 é só do Unix.
func applyMode(*os.File) error {
	return nil
}

// renameFile troca tmpName por dst. Repete o rename nos erros 32 e 33 do
// Windows, com as esperas de retryDelays. Outros erros voltam na hora.
func renameFile(tmpName, dst string) error {
	err := os.Rename(tmpName, dst)
	for _, d := range retryDelays {
		if err == nil || !isSharingViolation(err) {
			return err
		}
		time.Sleep(d)
		err = os.Rename(tmpName, dst)
	}
	return err
}

// syncDir é no-op no Windows: não há fsync de diretório pela stdlib.
func syncDir(string) error {
	return nil
}

// fileID devolve o índice de arquivo NTFS (FileIndexHigh e FileIndexLow)
// via GetFileInformationByHandle da stdlib. Se o volume não expõe o índice,
// a chamada falha e o id é 0, sem erro.
func fileID(f *os.File) (uint64, error) {
	var info syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(syscall.Handle(f.Fd()), &info); err != nil {
		return 0, nil
	}
	return uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow), nil
}
