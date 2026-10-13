package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// auditPageDelete é a op de audit_log da remoção física feita por DeletePath.
const auditPageDelete = "page-delete"

// ErrVersionMismatch é devolvido por DeletePath quando a versão corrente do caminho não é a
// versão esperada. Nesse caso nada é alterado.
var ErrVersionMismatch = errors.New("page version mismatch")

// DeletePath remove fisicamente a versão corrente do caminho ref, em uma transação própria por
// Writer.Do (INV-03). É condicional e idempotente:
//
//   - Se não há versão corrente do caminho, devolve deleted = false e erro nil (nada é feito e
//     nada é auditado).
//   - Se a versão corrente tem id diferente de expectedVersion, devolve deleted = false e um erro
//     com ErrVersionMismatch na cadeia (errors.Is). Nada é alterado.
//   - Caso contrário, apaga a linha da versão corrente de pages, grava auditoria page-delete com
//     a página apagada e remove entidades órfãs do escopo, tudo na mesma transação.
//
// Efeitos nos vínculos da versão apagada, pelas FKs do esquema: page_evidence, page_access,
// entity_page_links e links de origem (from_page_id) caem por ON DELETE CASCADE; links de destino
// (to_page_id) viram pendentes por ON DELETE SET NULL, com to_path preservado.
//
// As versões superseded do caminho não são tocadas (INV-16): nenhuma é reativada. Quem chama
// decide o que fazer com o caminho depois da remoção.
//
// Não apaga arquivo da wiki: isso é da T047. Erros voltam com %w.
func DeletePath(ctx context.Context, w Writer, ref domain.PageRef, expectedVersion domain.PageID) (bool, error) {
	deleted := false
	err := w.Do(ctx, func(tx *sql.Tx) error {
		head, found, err := findLatestPage(tx, ref)
		if err != nil {
			return err
		}
		if !found {
			return nil
		}
		if head.id != expectedVersion {
			return fmt.Errorf("%w: versão corrente %s, esperada %s",
				ErrVersionMismatch, head.id.String(), expectedVersion.String())
		}

		res, err := tx.Exec(
			`DELETE FROM pages WHERE id = ? AND is_latest = 1`,
			head.id.Bytes(),
		)
		if err != nil {
			return fmt.Errorf("apagar versão corrente: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("apagar versão corrente: %w", err)
		}
		if n != 1 {
			return fmt.Errorf("apagar versão corrente de %s: esperada 1 linha, afetadas %d", head.id.String(), n)
		}

		// Entidades do escopo sem nenhum vínculo em entity_page_links. Roda depois do DELETE da página,
		// porque os vínculos da versão apagada já saíram pelo CASCADE.
		if _, err := tx.Exec(
			`DELETE FROM entities
			  WHERE workspace_id = ? AND project_id = ?
			    AND NOT EXISTS (SELECT 1 FROM entity_page_links l WHERE l.entity_id = entities.id)`,
			ref.Scope.WorkspaceID.Bytes(), ref.Scope.ProjectID.Bytes(),
		); err != nil {
			return fmt.Errorf("remover entidades órfãs: %w", err)
		}

		target := head.id
		if err := InsertAudit(tx, time.Now(), AuditEvent{Op: auditPageDelete, Scope: ref.Scope, Page: &target}); err != nil {
			return err
		}
		deleted = true
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("delete path %s: %w", ref.String(), err)
	}
	return deleted, nil
}
