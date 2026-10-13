package wiki

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
	"github.com/phm-aguiar/ai-memory-go/internal/storage"
)

// DeleteResult é o resultado de DeletePage. Deleted é false quando não havia versão corrente
// (remoção idempotente, sem commit). Path é relativo à raiz da wiki (`<ws>/<proj>/<path>`).
// Checkpoint é o hash do commit, ou vazio quando não houve commit. Avisos traz as falhas tratadas
// como aviso (log, índice de projeto, commit).
type DeleteResult struct {
	Deleted    bool
	Path       string
	Checkpoint string
	Avisos     []string
}

// writerReader adapta o Writer ao Reader que storage.ReadPage exige. A leitura roda dentro do ator
// de escrita (uma transação); fn só faz SELECT, então nada é alterado. O Store não tem um Reader
// próprio, e write.go não é alterado por esta task.
type writerReader struct {
	w storage.Writer
}

func (r writerReader) Read(ctx context.Context, fn func(*sql.Tx) error) error {
	return r.w.Do(ctx, fn)
}

// DeletePage remove a versão corrente de uma página, na ordem:
//
//  1. Confinamento do caminho com pathFor. Nada é tocado antes.
//  2. Trava por página, mantida até o fim.
//  3. Leitura da versão corrente. Sem versão corrente, devolve Deleted = false, sem erro e sem commit.
//  4. Remoção do arquivo, guardando o conteúdo anterior. Arquivo já ausente segue em frente.
//  5. storage.DeletePath com expectedVersion. Se falhar (inclusive ErrVersionMismatch), o arquivo
//     é recriado com o conteúdo guardado. Se essa restauração falhar, o erro carrega
//     ErrInconsistentState.
//  6. Linha `page-delete` no log.md, antes do commit (ADR-22). Falha vira aviso.
//  7. Índice de projeto (EnsureProjectIndex). Falha vira aviso.
//  8. Commit com a remoção e o log.md, mensagem `page-delete: <ws>/<proj>/<path>`. Falha vira aviso.
//
// Não remove diretórios vazios nem apaga versões anteriores: versões superseded continuam no banco
// (INV-16). Erros de entrada e de índice voltam com %w.
func (s *Store) DeletePage(ctx context.Context, scope domain.Scope, path domain.PagePath, expectedVersion domain.PageID) (DeleteResult, error) {
	if s == nil || s.Writer == nil || s.Repo == nil || s.Locks == nil {
		return DeleteResult{}, errors.New("wiki: remoção de página exige Writer, Repo e Locks")
	}
	now := s.now()

	// 1. Confinamento. Nada é tocado antes desta checagem.
	abs, err := pathFor(s.Root, scope, path)
	if err != nil {
		return DeleteResult{}, err
	}
	rel := scopeRelPath(scope, path.String())
	ref := domain.NewPageRef(scope, path)

	// 2. Trava por página.
	unlock, err := s.Locks.Lock(ctx, ref)
	if err != nil {
		return DeleteResult{}, fmt.Errorf("wiki: remoção de página: %w", err)
	}
	defer unlock()

	// 3. Versão corrente. Sem ela, a remoção é idempotente: nada no disco, nada no banco, sem commit.
	page, err := storage.ReadPage(ctx, writerReader{w: s.Writer}, ref)
	if errors.Is(err, storage.ErrPageNotFound) {
		return DeleteResult{Path: rel}, nil
	}
	if err != nil {
		return DeleteResult{}, fmt.Errorf("wiki: remoção de página: %w", err)
	}

	// 4. Remoção do arquivo, com o conteúdo anterior guardado para a restauração.
	prev, existed, err := readIfExists(abs)
	if err != nil {
		return DeleteResult{}, err
	}
	if existed {
		if err := os.Remove(abs); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return DeleteResult{}, fmt.Errorf("wiki: remover %s: %w", abs, err)
		}
	}

	// 5. Remoção no índice, condicionada à versão esperada. Falha reverte o arquivo.
	deleted, err := storage.DeletePath(ctx, s.Writer, ref, expectedVersion)
	if err != nil || !deleted {
		if existed {
			if rbErr := restoreFile(abs, prev, existed); rbErr != nil {
				if err == nil {
					err = errors.New("versão corrente desapareceu antes da remoção")
				}
				return DeleteResult{}, fmt.Errorf("%w: restaurar %s: %w; erro original: %w",
					ErrInconsistentState, abs, rbErr, err)
			}
		}
		if err != nil {
			return DeleteResult{}, fmt.Errorf("wiki: remoção de página: %w", err)
		}
		// Sem erro e sem linha removida: a versão sumiu entre a leitura e a remoção. Nada mudou no banco.
		return DeleteResult{Path: rel}, nil
	}

	// 6. Linha do log antes do commit (ADR-22). Falha vira aviso.
	var avisos []string
	paths := []string{rel}
	if err := AppendLogLine(s.Root, scope, eventPageDelete, page.Title, now); err != nil {
		avisos = append(avisos, fmt.Sprintf("log.md não atualizado: %v", err))
	} else {
		paths = append(paths, scopeRelPath(scope, logFileName))
	}

	// 7. Índice de projeto. Falha vira aviso.
	projIndex, err := EnsureProjectIndex(s.Root, scope)
	if err != nil {
		avisos = append(avisos, fmt.Sprintf("index.md do projeto não atualizado: %v", err))
	} else {
		paths = append(paths, projIndex...)
	}

	// 8. Commit. Falha vira aviso e checkpoint vazio.
	checkpoint, err := s.Repo.Commit(ctx, paths, "page-delete: "+rel)
	if err != nil {
		avisos = append(avisos, fmt.Sprintf("commit não realizado: %v", err))
		checkpoint = ""
	}

	return DeleteResult{
		Deleted:    true,
		Path:       rel,
		Checkpoint: checkpoint,
		Avisos:     avisos,
	}, nil
}
