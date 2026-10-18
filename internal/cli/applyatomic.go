package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"
)

// Escrita idempotente de arquivo de agente (apply_atomic, docs/cli.md §8 e
// docs/install.md "--apply").
//
// Regras, conforme o doc:
//   - Links simbólicos são seguidos até maxSymlinkHops níveis. Cadeia mais
//     profunda é recusada. A escrita vai para o alvo final, e o link não é
//     substituído.
//   - Se o alvo final não existir, ele é criado (o doc prevê alvo ainda
//     inexistente).
//   - Se o conteúdo resultante for igual ao atual, nada é gravado no disco
//     (NoOp): não há escrita nem mudança de mtime.
//   - Caso contrário, o conteúdo atual é copiado para
//     `<caminho visível>.bak-<unix em segundos>`. O backup fica ao lado do
//     caminho que o usuário enxerga (o link), não ao lado do alvo.
//   - A gravação é atômica: temporário no mesmo diretório do alvo, Sync,
//     Close, rename sobre o alvo e fsync do diretório (no Unix).
//
// Diretório pai inexistente: o doc não define o caso. Esta implementação não
// cria diretórios e devolve erro.

// ApplyStatus diz o que ApplyAtomic fez.
type ApplyStatus int

const (
	// ApplyNoOp: o conteúdo resultante é igual ao atual. O disco não foi tocado.
	ApplyNoOp ApplyStatus = iota + 1
	// ApplyWritten: o alvo foi gravado, com backup prévio quando já existia.
	ApplyWritten
)

// String devolve o nome do status, para logs e mensagens.
func (s ApplyStatus) String() string {
	switch s {
	case ApplyNoOp:
		return "NoOp"
	case ApplyWritten:
		return "Written"
	default:
		return "ApplyStatus(" + strconv.Itoa(int(s)) + ")"
	}
}

// maxSymlinkHops é o número máximo de links simbólicos seguidos (docs/install.md).
const maxSymlinkHops = 40

// defaultFileMode é a permissão de um arquivo novo no Unix. No Windows, só o
// bit de somente leitura tem efeito.
const defaultFileMode fs.FileMode = 0o644

// ErrSymlinkChainTooDeep indica cadeia de links acima de maxSymlinkHops.
var ErrSymlinkChainTooDeep = errors.New("apply_atomic: cadeia de links simbólicos acima de 40 níveis")

// ErrBackupExists indica que o `.bak-<unix>` do instante já existe. Um
// backup nunca é sobrescrito.
var ErrBackupExists = errors.New("apply_atomic: arquivo de backup já existe")

// nowUnix é o relógio usado para o sufixo do backup. Os testes o substituem
// quando precisam de um instante fixo.
var nowUnix = func() int64 { return time.Now().Unix() }

// ApplyAtomic lê o conteúdo atual de path (seguindo links simbólicos), chama
// transform com esse conteúdo e grava o resultado de forma idempotente.
//
// Se path não existe, transform recebe nil. Se transform devolve erro, nada
// é alterado. O retorno é ApplyNoOp quando o resultado é igual ao atual e
// ApplyWritten quando o alvo foi gravado.
func ApplyAtomic(path string, transform func(current []byte) ([]byte, error)) (ApplyStatus, error) {
	target, err := resolveSymlinkTarget(path)
	if err != nil {
		return 0, err
	}

	current, mode, exists, err := readCurrent(target)
	if err != nil {
		return 0, err
	}

	next, err := transform(current)
	if err != nil {
		return 0, fmt.Errorf("apply_atomic: transformação de %s: %w", path, err)
	}

	if exists && bytes.Equal(current, next) {
		return ApplyNoOp, nil
	}

	if exists {
		if err := writeBackup(path, current, mode); err != nil {
			return 0, err
		}
	}

	if err := writeFileAtomic(target, next, mode); err != nil {
		return 0, err
	}
	return ApplyWritten, nil
}

// resolveSymlinkTarget segue links simbólicos até o alvo final. O alvo final
// pode não existir: nesse caso o próprio caminho é devolvido. Links
// relativos são resolvidos a partir do diretório do link.
func resolveSymlinkTarget(path string) (string, error) {
	cur := path
	for hops := 0; ; hops++ {
		fi, err := os.Lstat(cur)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return cur, nil
			}
			return "", fmt.Errorf("apply_atomic: lstat %s: %w", cur, err)
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			return cur, nil
		}
		if hops == maxSymlinkHops {
			return "", fmt.Errorf("%w: %s", ErrSymlinkChainTooDeep, path)
		}
		link, err := os.Readlink(cur)
		if err != nil {
			return "", fmt.Errorf("apply_atomic: readlink %s: %w", cur, err)
		}
		if !filepath.IsAbs(link) {
			link = filepath.Join(filepath.Dir(cur), link)
		}
		cur = link
	}
}

// readCurrent lê o alvo. Se ele não existe, devolve exists=false e conteúdo
// nil. Um alvo que é diretório gera erro.
func readCurrent(target string) (data []byte, mode fs.FileMode, exists bool, err error) {
	fi, err := os.Stat(target)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, defaultFileMode, false, nil
		}
		return nil, 0, false, fmt.Errorf("apply_atomic: stat %s: %w", target, err)
	}
	if fi.IsDir() {
		return nil, 0, false, fmt.Errorf("apply_atomic: %s é um diretório", target)
	}
	data, err = os.ReadFile(target)
	if err != nil {
		return nil, 0, false, fmt.Errorf("apply_atomic: ler %s: %w", target, err)
	}
	return data, fi.Mode().Perm(), true, nil
}

// writeBackup copia data para `<path>.bak-<unix>`. path é o caminho visível,
// que pode ser um link. Usa O_EXCL: um backup existente não é sobrescrito.
func writeBackup(path string, data []byte, mode fs.FileMode) error {
	bak := path + ".bak-" + strconv.FormatInt(nowUnix(), 10)
	f, err := os.OpenFile(bak, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("%w: %s", ErrBackupExists, bak)
		}
		return fmt.Errorf("apply_atomic: criar backup %s: %w", bak, err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(bak)
		return fmt.Errorf("apply_atomic: gravar backup %s: %w", bak, err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(bak)
		return fmt.Errorf("apply_atomic: sync do backup %s: %w", bak, err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(bak)
		return fmt.Errorf("apply_atomic: fechar backup %s: %w", bak, err)
	}
	return nil
}

// writeFileAtomic grava data em target: temporário no mesmo diretório, Sync,
// Close, permissão, rename sobre target e fsync do diretório (Unix). Em falha
// antes do rename, o temporário é removido. target é sempre um caminho
// resolvido, então o rename troca o arquivo, e não o link.
//
// No Windows, os.Rename usa MoveFileEx com MOVEFILE_REPLACE_EXISTING, o que
// substitui um arquivo existente. Não há fsync de diretório no Windows, pois
// FlushFileBuffers não funciona em handle de diretório.
func writeFileAtomic(target string, data []byte, mode fs.FileMode) error {
	dir := filepath.Dir(target)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(target)+".tmp-*")
	if err != nil {
		return fmt.Errorf("apply_atomic: criar temporário em %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = tmp.Close() // pode já estar fechado; o erro não importa aqui
			_ = os.Remove(tmpName)
		}
	}()

	if err := tmp.Chmod(mode); err != nil {
		return fmt.Errorf("apply_atomic: permissão do temporário: %w", err)
	}
	if _, err := io.Copy(tmp, bytes.NewReader(data)); err != nil {
		return fmt.Errorf("apply_atomic: gravar temporário: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("apply_atomic: sync do temporário: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("apply_atomic: fechar temporário: %w", err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		return fmt.Errorf("apply_atomic: instalar %s: %w", target, err)
	}
	renamed = true

	if runtime.GOOS == "windows" {
		return nil
	}
	return syncDirApply(dir)
}

// syncDirApply faz fsync do diretório para persistir o rename. EINVAL e
// ENOTSUP, que indicam sistema de arquivos sem suporte, são ignorados.
func syncDirApply(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("apply_atomic: abrir diretório %s: %w", dir, err)
	}
	syncErr := d.Sync()
	closeErr := d.Close()
	if syncErr != nil {
		if errors.Is(syncErr, syscall.EINVAL) || errors.Is(syncErr, syscall.ENOTSUP) {
			return nil
		}
		return fmt.Errorf("apply_atomic: fsync do diretório %s: %w", dir, syncErr)
	}
	return closeErr
}
