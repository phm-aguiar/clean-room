// Package wiki contém a camada de arquivos markdown da wiki.
package wiki

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// tmpPrefix é o prefixo do temporário da escrita atômica. O watcher ignora
// os eventos cujo nome começa com ele (INV-10, wiki-layout-contract (e)).
const tmpPrefix = ".ai-memory-tmp."

// fileMode é a permissão do arquivo final no Unix. No Windows não se aplica.
const fileMode os.FileMode = 0o644

// retryDelays são as esperas entre as tentativas de rename no Windows quando
// o erro é de compartilhamento ou de bloqueio (32 ou 33): no máximo 5 novas
// tentativas, após a primeira.
var retryDelays = []time.Duration{
	10 * time.Millisecond,
	20 * time.Millisecond,
	30 * time.Millisecond,
	40 * time.Millisecond,
	50 * time.Millisecond,
}

// Write grava data em path de forma atômica e devolve o identificador do
// arquivo instalado, para o watcher reconhecer as próprias escritas.
//
// Sequência: temporário `.ai-memory-tmp.*` no mesmo diretório, permissão 0644
// (Unix), escrita, Sync, fechamento, rename sobre path e fsync do diretório
// pai. Em falha antes do rename, o temporário é removido. Se o fsync do
// diretório falha depois do rename, o arquivo já está instalado: o id é
// devolvido junto com o erro.
//
// O id é o inode no Unix e o índice de arquivo NTFS no Windows. Quando o
// volume não expõe o índice, o id é 0.
func Write(path string, data []byte) (uint64, error) {
	dir := filepath.Dir(path)

	tmp, err := createTemp(dir)
	if err != nil {
		return 0, fmt.Errorf("wiki: criar temporário em %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = tmp.Close() // pode já estar fechado; o erro é irrelevante aqui
			_ = os.Remove(tmpName)
		}
	}()

	if err := applyMode(tmp); err != nil {
		return 0, fmt.Errorf("wiki: definir permissão do temporário: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		return 0, fmt.Errorf("wiki: gravar temporário: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return 0, fmt.Errorf("wiki: sync do temporário: %w", err)
	}
	id, err := fileID(tmp)
	if err != nil {
		return 0, fmt.Errorf("wiki: identificador do temporário: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return 0, fmt.Errorf("wiki: fechar temporário: %w", err)
	}
	if err := renameFile(tmpName, path); err != nil {
		return 0, fmt.Errorf("wiki: instalar %s: %w", path, err)
	}
	renamed = true

	if err := syncDir(dir); err != nil {
		return id, fmt.Errorf("wiki: fsync do diretório %s: %w", dir, err)
	}
	return id, nil
}

// createTemp cria o temporário no diretório dado, com o prefixo de INV-10.
func createTemp(dir string) (*os.File, error) {
	return os.CreateTemp(dir, tmpPrefix+"*")
}

// isSharingViolation diz se err é um erro de compartilhamento (32) ou de
// bloqueio (33) do Windows, que justifica nova tentativa do rename.
func isSharingViolation(err error) bool {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return false
	}
	return errno == errSharingViolation || errno == errLockViolation
}

// Códigos ERROR_SHARING_VIOLATION (32) e ERROR_LOCK_VIOLATION (33) do Windows.
// Ficam aqui, sem build tag, para que o teste rode em qualquer SO.
const (
	errSharingViolation = syscall.Errno(32)
	errLockViolation    = syscall.Errno(33)
)
