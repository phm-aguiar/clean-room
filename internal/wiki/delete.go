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
// (remoção idempotente, sem registro para commit). Path é relativo à raiz da wiki (`<ws>/<proj>/<path>`).
// Checkpoint é sempre vazio: DeletePage não faz commit (T080). Os caminhos afetados ficam registrados
// no Repo para o commit feito por quem chama (CommitTracked). Avisos traz as falhas tratadas como
// aviso (registro para commit).
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
//  5. storage.DeletePath com expectedVersion. Se falhar, ou se não removeu nada (versão corrente
//     diferente da esperada, sem erro), o arquivo é recriado com o conteúdo guardado e não há
//     remoção (Deleted = false). Se essa restauração falhar, o erro carrega ErrInconsistentState.
//  6. Registro com Repo.Track da remoção. Falha vira aviso. Não há log.md nem index.md: a remoção
//     não escreve nesses arquivos (docs/wiki-files-layer.md §5.2).
//     Não há commit aqui: quem chama faz CommitTracked com a mensagem `page-delete: <ws>/<proj>/<path>`
//     no ponto definido pelo doc (§7). Checkpoint fica vazio.
//
// Não remove diretórios vazios. A remoção do banco apaga todas as versões do caminho (docs/
// storage-and-retrieval.md §5.2); o arquivo é removido só se o banco confirmar. Erros de entrada e de
// índice voltam com %w.
func (s *Store) DeletePage(ctx context.Context, scope domain.Scope, path domain.PagePath, expectedVersion domain.PageID) (DeleteResult, error) {
	if s == nil || s.Writer == nil || s.Repo == nil || s.Locks == nil {
		return DeleteResult{}, errors.New("wiki: remoção de página exige Writer, Repo e Locks")
	}

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
	_, err = storage.ReadPage(ctx, writerReader{w: s.Writer}, ref)
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
					err = errors.New("versão corrente diferente da esperada ou ausente; nada removido")
				}
				return DeleteResult{}, fmt.Errorf("%w: restaurar %s: %w; erro original: %w",
					ErrInconsistentState, abs, rbErr, err)
			}
		}
		if err != nil {
			return DeleteResult{}, fmt.Errorf("wiki: remoção de página: %w", err)
		}
		// Sem erro e sem linha removida: versão diferente da esperada ou ausente. Nada mudou no banco.
		return DeleteResult{Path: rel}, nil
	}

	// 6. Registro para commit. DeletePage não faz commit: quem chama commita com CommitTracked.
	// Falha de registro vira aviso. Checkpoint fica vazio.
	var avisos []string
	if err := s.Repo.Track(rel); err != nil {
		avisos = append(avisos, fmt.Sprintf("registro para commit não feito: %v", err))
	}

	return DeleteResult{
		Deleted:    true,
		Path:       rel,
		Checkpoint: "",
		Avisos:     avisos,
	}, nil
}
