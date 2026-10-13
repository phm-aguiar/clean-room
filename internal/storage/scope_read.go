package storage

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// Nomes do escopo padrão de leitura (docs/domain-vocabulary.md §2): workspace
// `default` e projeto de fallback `scratch`. Só são procurados, nunca criados.
const (
	scopeDefaultWorkspaceName = "default"
	scopeDefaultProjectName   = "scratch"
)

// ScopeSource diz por qual caminho ResolveScope encontrou o escopo.
type ScopeSource string

const (
	// ScopeSourceExplicit: workspace e projeto informados por nome.
	ScopeSourceExplicit ScopeSource = "explicit"
	// ScopeSourceDefault: nenhum nome informado; usado o escopo padrão por nome.
	ScopeSourceDefault ScopeSource = "default"
	// ScopeSourceRepoPath: projeto cujo repo_path casa com o informado.
	ScopeSourceRepoPath ScopeSource = "repo_path"
)

// ScopeRequest pede um escopo de leitura. Os caminhos são exclusivos:
//   - WorkspaceName e ProjectName juntos: nome explícito (um sem o outro falha fechado).
//   - RepoPath: projeto cujo repo_path é exatamente esse valor.
//   - tudo vazio: escopo padrão (workspace `default`, projeto `scratch`).
//
// Misturar RepoPath com nome também falha fechado.
type ScopeRequest struct {
	WorkspaceName string
	ProjectName   string
	RepoPath      string
}

// mode valida a requisição e devolve o caminho de resolução. Nunca escolhe um
// caminho quando a requisição é ambígua ou parcial.
func (req ScopeRequest) mode() (ScopeSource, error) {
	hasWS := req.WorkspaceName != ""
	hasPJ := req.ProjectName != ""
	hasRepo := req.RepoPath != ""

	switch {
	case hasRepo && (hasWS || hasPJ):
		return "", fmt.Errorf("%w: repo_path não combina com workspace/projeto por nome", ErrScopeNotFound)
	case hasRepo:
		return ScopeSourceRepoPath, nil
	case hasWS && hasPJ:
		return ScopeSourceExplicit, nil
	case hasWS || hasPJ:
		return "", fmt.Errorf("%w: escopo parcial: workspace e projeto devem ser informados juntos", ErrScopeNotFound)
	default:
		return ScopeSourceDefault, nil
	}
}

// ResolveScope devolve o escopo pedido em req, lendo por r. É somente leitura:
// roda dentro de Reader.Read (transação read-only, banco com query_only) e nunca
// insere em workspaces nem em projects. Quem cria escopo é EnsureScope.
//
// Falha fechada:
//   - nenhum casamento: erro envolvendo ErrScopeNotFound;
//   - mais de um casamento: erro envolvendo ErrScopeAmbiguous;
//   - nunca devolve um escopo aproximado ou inventado.
//
// Erros são envolvidos com %w. A função não entra em pânico; um pânico dentro
// de Reader.Read já é capturado lá.
//
// Esta função é livre (não é método de Reader) porque Reader é fixo na T023;
// o plano cita ResolveScope junto de Reader, mas a interface não muda aqui.
func ResolveScope(ctx context.Context, r Reader, req ScopeRequest) (domain.Scope, ScopeSource, error) {
	src, err := req.mode()
	if err != nil {
		return domain.Scope{}, "", err
	}

	var (
		query string
		args  []any
		label string
	)
	switch src {
	case ScopeSourceExplicit:
		query = `SELECT p.id, p.workspace_id FROM projects p
		         JOIN workspaces w ON w.id = p.workspace_id
		         WHERE w.name = ? AND p.name = ?`
		args = []any{req.WorkspaceName, req.ProjectName}
		label = fmt.Sprintf("workspace %q, projeto %q", req.WorkspaceName, req.ProjectName)
	case ScopeSourceDefault:
		query = `SELECT p.id, p.workspace_id FROM projects p
		         JOIN workspaces w ON w.id = p.workspace_id
		         WHERE w.name = ? AND p.name = ?`
		args = []any{scopeDefaultWorkspaceName, scopeDefaultProjectName}
		label = fmt.Sprintf("padrão (workspace %q, projeto %q)", scopeDefaultWorkspaceName, scopeDefaultProjectName)
	case ScopeSourceRepoPath:
		query = `SELECT id, workspace_id FROM projects WHERE repo_path = ?`
		args = []any{req.RepoPath}
		label = fmt.Sprintf("repo_path %q", req.RepoPath)
	}

	var found []domain.Scope
	err = r.Read(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("consulta de escopo: %w", err)
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			var pjB, wsB []byte
			if err := rows.Scan(&pjB, &wsB); err != nil {
				return fmt.Errorf("ler escopo: %w", err)
			}
			pjID, err := domain.ParseIDBytes(pjB)
			if err != nil {
				return fmt.Errorf("id do projeto: %w", err)
			}
			wsID, err := domain.ParseIDBytes(wsB)
			if err != nil {
				return fmt.Errorf("id do workspace: %w", err)
			}
			found = append(found, domain.NewScope(domain.WorkspaceID(wsID), domain.ProjectID(pjID)))
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterar escopos: %w", err)
		}
		return nil
	})
	if err != nil {
		return domain.Scope{}, "", fmt.Errorf("resolver escopo %s: %w", label, err)
	}

	switch len(found) {
	case 0:
		return domain.Scope{}, "", fmt.Errorf("resolver escopo %s: %w", label, ErrScopeNotFound)
	case 1:
		return found[0], src, nil
	default:
		return domain.Scope{}, "", fmt.Errorf("resolver escopo %s (%d casamentos): %w", label, len(found), ErrScopeAmbiguous)
	}
}
