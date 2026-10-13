//go:build !windows

package wiki

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// applyMode define a permissão 0644 do temporário. Usa Chmod, que não sofre
// a máscara do umask (CreateTemp cria com 0600).
func applyMode(f *os.File) error {
	return f.Chmod(fileMode)
}

// renameFile troca tmpName por dst. Em outros SOs não há repetição.
func renameFile(tmpName, dst string) error {
	return os.Rename(tmpName, dst)
}

// syncDir faz fsync do diretório pai, para persistir o rename. Se o SO ou o
// sistema de arquivos não suporta fsync de diretório (EINVAL ou ENOTSUP), o
// erro é ignorado. Outros erros são devolvidos.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("abrir diretório: %w", err)
	}
	syncErr := d.Sync()
	closeErr := d.Close()
	if syncErr != nil {
		if errors.Is(syncErr, syscall.EINVAL) || errors.Is(syncErr, syscall.ENOTSUP) {
			return nil
		}
		return syncErr
	}
	return closeErr
}

// fileID devolve o inode do arquivo (syscall.Stat_t.Ino). Se o sistema não
// expõe Stat_t, o id é 0.
func fileID(f *os.File) (uint64, error) {
	fi, err := f.Stat()
	if err != nil {
		return 0, fmt.Errorf("stat: %w", err)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, nil
	}
	return uint64(st.Ino), nil
}
