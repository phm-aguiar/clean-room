package storage

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// EnsureScope garante que o workspace e o projeto de scope existem no banco.
//
// Esta é a ÚNICA função do pacote que insere em workspaces e projects. Ela
// só deve ser chamada por caminhos de escrita explícita: leitura, busca e
// operações destrutivas nunca criam workspace ou projeto (constituição, Resolução
// de escopo). A criação de workspace/projeto padrão (`default`, `scratch`,
// `_global`) também é decisão do chamador: esta função cria só o escopo pedido.
//
// A criação roda dentro de w.Do, em uma única transação. A função é idempotente:
// se o workspace ou o projeto já existe com o mesmo ID, nada é recriado e não há
// erro. Um conflito de nome com outro ID (workspaces.name é UNIQUE; projects tem
// UNIQUE (workspace_id, name)) continua sendo erro, para não mascarar escopos
// divergentes. Se o ID do projeto já existir em outro workspace, também é erro.
//
// nameWS e nameProj são os nomes humanos. createdAt é gravado como microssegundos
// Unix UTC e só vale na primeira criação. repo_path fica NULL: o esquema o
// permite e esta função não tem fonte para ele.
//
// ctx controla a espera na fila de w.Do. Depois de aceito, a transação não é
// interrompida pelo ctx. Erros são envolvidos com %w.
func EnsureScope(ctx context.Context, w Writer, scope domain.Scope, nameWS, nameProj string, createdAt time.Time) error {
	ws := scope.WorkspaceID.Bytes()
	pj := scope.ProjectID.Bytes()
	micros := createdAt.UnixMicro()

	err := w.Do(ctx, func(tx *sql.Tx) error {
		// Sem ctx nos comandos: a transação aceita não pode ser abortada no meio.
		if _, err := tx.Exec(
			`INSERT INTO workspaces (id, name, created_at) VALUES (?, ?, ?)
			 ON CONFLICT(id) DO NOTHING`,
			ws, nameWS, micros,
		); err != nil {
			return fmt.Errorf("insert workspace: %w", err)
		}

		if _, err := tx.Exec(
			`INSERT INTO projects (id, workspace_id, name, created_at) VALUES (?, ?, ?, ?)
			 ON CONFLICT(id) DO NOTHING`,
			pj, ws, nameProj, micros,
		); err != nil {
			return fmt.Errorf("insert project: %w", err)
		}

		var owner []byte
		if err := tx.QueryRow(`SELECT workspace_id FROM projects WHERE id = ?`, pj).Scan(&owner); err != nil {
			return fmt.Errorf("check project owner: %w", err)
		}
		if !bytes.Equal(owner, ws) {
			return fmt.Errorf("project %s already belongs to another workspace", scope.ProjectID.String())
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("ensure scope %s/%s: %w", scope.WorkspaceID.String(), scope.ProjectID.String(), err)
	}
	return nil
}
