package domain

// Scope identifica o escopo de uma página: workspace e projeto (ADR-07).
// Só contém identificadores de tipo definido, então é comparável com == e não exige validação.
type Scope struct {
	WorkspaceID WorkspaceID
	ProjectID   ProjectID
}

// NewScope monta um Scope. Não há erro: os IDs já são válidos pelo tipo.
func NewScope(ws WorkspaceID, p ProjectID) Scope {
	return Scope{WorkspaceID: ws, ProjectID: p}
}

// PageRef é a identidade de uma página: (workspace_id, project_id, path), INV-04.
// Dois PageRef são iguais com == somente se os três elementos são iguais.
// A colisão por caixa ou Unicode não é tratada aqui: é função de PagePath.PortableKey, no storage.
type PageRef struct {
	Scope Scope
	Path  PagePath
}

// NewPageRef monta um PageRef a partir do escopo e do caminho já validado.
func NewPageRef(s Scope, p PagePath) PageRef {
	return PageRef{Scope: s, Path: p}
}

// String devolve "<workspace>/<projeto>:<caminho>", com os IDs na forma canônica com hífens.
// Serve a log e a mensagem de erro. Não faz I/O.
func (r PageRef) String() string {
	return r.Scope.WorkspaceID.String() + "/" + r.Scope.ProjectID.String() + ":" + r.Path.String()
}
