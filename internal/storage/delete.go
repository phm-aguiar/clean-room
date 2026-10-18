package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// auditPageDelete é a op de audit_log da remoção física feita por DeletePath.
const auditPageDelete = "page-delete"

// DeletePath remove fisicamente todas as versões do caminho ref (corrente e superseded), em uma
// transação própria por Writer.Do (INV-03). É condicional e idempotente, e nunca devolve erro por
// versão divergente:
//
//   - Se não há versão corrente do caminho, devolve deleted = false e erro nil (nada é feito e
//     nada é auditado).
//   - Se a versão corrente tem id diferente de expectedVersion, devolve deleted = false e erro nil.
//     Nada é alterado: quem chama decide o que fazer com o arquivo (a wiki restaura o conteúdo).
//   - Caso contrário, apaga as linhas de pages do caminho no escopo (todas as versões), grava
//     auditoria page-delete com a versão corrente apagada e remove entidades órfãs do escopo, tudo
//     na mesma transação. Só grava auditoria se alguma linha foi removida.
//
// Efeitos nos vínculos das versões apagadas, pelas FKs do esquema: page_evidence, page_access,
// entity_page_links e links de origem (from_page_id) caem por ON DELETE CASCADE; links de destino
// (to_page_id) viram pendentes por ON DELETE SET NULL, com to_path preservado.
//
// Nenhuma versão é reativada. A remoção física é a exclusão por caminho de docs/storage-and-retrieval.md
// §5.2; os tombstones de decaimento (§5.3) são de outro caminho e não passam por aqui.
//
// Não apaga arquivo da wiki: isso é da wiki (wiki.DeletePage). Erros voltam com %w.
func DeletePath(ctx context.Context, w Writer, ref domain.PageRef, expectedVersion domain.PageID) (bool, error) {
	deleted := false
	err := w.Do(ctx, func(tx *sql.Tx) error {
		head, found, err := findLatestPage(tx, ref)
		if err != nil {
			return err
		}
		if !found || head.id != expectedVersion {
			return nil
		}

		res, err := tx.Exec(
			`DELETE FROM pages WHERE workspace_id = ? AND project_id = ? AND path = ?`,
			ref.Scope.WorkspaceID.Bytes(), ref.Scope.ProjectID.Bytes(), ref.Path.String(),
		)
		if err != nil {
			return fmt.Errorf("apagar versões do caminho: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("apagar versões do caminho: %w", err)
		}
		if n < 1 {
			return fmt.Errorf("apagar versões do caminho de %s: nenhuma linha afetada", head.id.String())
		}

		// Entidades do escopo sem nenhum vínculo em entity_page_links. Roda depois do DELETE das páginas,
		// porque os vínculos das versões apagadas já saíram pelo CASCADE.
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
