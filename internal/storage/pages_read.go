package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// ErrPageNotFound é a causa de ReadPage quando o caminho exato não tem versão corrente no escopo.
var ErrPageNotFound = errors.New("page not found")

// pageReadCols são as colunas que scanPageRow lê, nesta ordem.
const pageReadCols = `id, path, tier, title, body, frontmatter_json`

// rowScanner é o que scanPageRow precisa. *sql.Row e *sql.Rows o satisfazem.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanPageRow reconstrói a página de uma linha de pages lida com pageReadCols. O caminho é
// lido com ParseStoredPagePath (linha já persistida, ADR-03). O kind sai da chave kind do
// frontmatter, como grava o UpsertPage; o Frontmatter devolvido não repete essa chave, para
// espelhar UpsertInput. Erro de Scan volta com %w, então sql.ErrNoRows continua visível.
func scanPageRow(scope domain.Scope, sc rowScanner) (domain.Page, error) {
	var (
		idBytes                        []byte
		path, tier, title, body, fmRaw string
	)
	if err := sc.Scan(&idBytes, &path, &tier, &title, &body, &fmRaw); err != nil {
		return domain.Page{}, fmt.Errorf("ler linha de página: %w", err)
	}
	id, err := domain.ParseIDBytes(idBytes)
	if err != nil {
		return domain.Page{}, fmt.Errorf("ler id da página: %w", err)
	}
	pp, err := domain.ParseStoredPagePath(path)
	if err != nil {
		return domain.Page{}, fmt.Errorf("caminho armazenado inválido %q: %w", path, err)
	}
	fm, err := decodeFrontmatter(fmRaw)
	if err != nil {
		return domain.Page{}, fmt.Errorf("ler frontmatter da página %s: %w", id.String(), err)
	}

	kind := domain.PageKind("")
	rest := fm
	if v, ok := fm.Get(kindKey); ok {
		if s, isStr := v.(string); isStr {
			kind = domain.PageKind(s)
			rest, err = copyFrontmatter(fm, kindKey)
			if err != nil {
				return domain.Page{}, fmt.Errorf("copiar frontmatter da página %s: %w", id.String(), err)
			}
		}
	}

	return domain.Page{
		Ref:         domain.NewPageRef(scope, pp),
		Tier:        domain.Tier(tier),
		Kind:        kind,
		Title:       title,
		Body:        body,
		Frontmatter: rest,
		Version:     domain.PageID(id),
	}, nil
}

// ReadPage devolve a versão corrente (is_latest = 1) do caminho exato de ref, dentro de r.
// Se o caminho não tem versão corrente no escopo, devolve erro envolvendo ErrPageNotFound.
// Não filtra por autoria (INV-16). Só lê: roda numa transação somente leitura.
func ReadPage(ctx context.Context, r Reader, ref domain.PageRef) (domain.Page, error) {
	var page domain.Page
	err := r.Read(ctx, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx,
			`SELECT `+pageReadCols+`
			   FROM pages
			  WHERE workspace_id = ? AND project_id = ? AND path = ? AND is_latest = 1`,
			ref.Scope.WorkspaceID.Bytes(), ref.Scope.ProjectID.Bytes(), ref.Path.String(),
		)
		p, err := scanPageRow(ref.Scope, row)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%s: %w", ref.String(), ErrPageNotFound)
		}
		if err != nil {
			return err
		}
		page = p
		return nil
	})
	if err != nil {
		return domain.Page{}, fmt.Errorf("read page %s: %w", ref.String(), err)
	}
	return page, nil
}

// Recent lista as versões correntes do escopo, da mais nova para a mais antiga
// (created_at DESC, rowid DESC), com no máximo limit itens. limit <= 0 devolve lista vazia
// sem erro e sem abrir leitura. Não filtra por autoria (INV-16). Só lê.
func Recent(ctx context.Context, r Reader, scope domain.Scope, limit int) ([]domain.Page, error) {
	if limit <= 0 {
		return []domain.Page{}, nil
	}
	out := []domain.Page{}
	err := r.Read(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx,
			`SELECT `+pageReadCols+`
			   FROM pages
			  WHERE workspace_id = ? AND project_id = ? AND is_latest = 1
			  ORDER BY created_at DESC, rowid DESC
			  LIMIT ?`,
			scope.WorkspaceID.Bytes(), scope.ProjectID.Bytes(), limit,
		)
		if err != nil {
			return fmt.Errorf("listar páginas recentes: %w", err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			p, err := scanPageRow(scope, rows)
			if err != nil {
				return err
			}
			out = append(out, p)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterar páginas recentes: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("recent: %w", err)
	}
	return out, nil
}

// PageChain devolve a cadeia de versões do caminho exato de ref, da mais nova para a mais
// antiga, seguindo supersedes. A versão perdedora (superseded) aparece (INV-16). Não filtra
// por is_latest. A versão inicial é a de is_latest = 1 quando existe; senão, a mais nova do
// caminho (caso de lápide, em que nenhuma versão é corrente). Caminho sem nenhuma versão
// devolve lista vazia sem erro. Uma cadeia com ciclo ou com ponteiro para versão ausente
// devolve erro, sem panic. Só lê.
func PageChain(ctx context.Context, r Reader, ref domain.PageRef) ([]domain.PageVersion, error) {
	out := []domain.PageVersion{}
	err := r.Read(ctx, func(tx *sql.Tx) error {
		var start []byte
		err := tx.QueryRowContext(ctx,
			`SELECT id
			   FROM pages
			  WHERE workspace_id = ? AND project_id = ? AND path = ?
			  ORDER BY is_latest DESC, created_at DESC, rowid DESC
			  LIMIT 1`,
			ref.Scope.WorkspaceID.Bytes(), ref.Scope.ProjectID.Bytes(), ref.Path.String(),
		).Scan(&start)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("ler versão inicial da cadeia: %w", err)
		}

		visited := map[string]bool{}
		cur := start
		for cur != nil {
			if visited[string(cur)] {
				return fmt.Errorf("cadeia de supersessão com ciclo na versão %x", cur)
			}
			visited[string(cur)] = true

			var (
				idBytes, sup []byte
				isLatest     int
				validFrom    sql.NullInt64
				validTo      sql.NullInt64
			)
			err := tx.QueryRowContext(ctx,
				`SELECT id, supersedes, is_latest, valid_from, valid_to FROM pages WHERE id = ?`,
				cur,
			).Scan(&idBytes, &sup, &isLatest, &validFrom, &validTo)
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("cadeia de supersessão quebrada: versão %x ausente", cur)
			}
			if err != nil {
				return fmt.Errorf("ler versão da cadeia: %w", err)
			}
			id, err := domain.ParseIDBytes(idBytes)
			if err != nil {
				return fmt.Errorf("ler id da versão da cadeia: %w", err)
			}

			pv := domain.PageVersion{
				ID:        domain.PageID(id),
				IsLatest:  isLatest != 0,
				ValidFrom: validFrom.Int64,
				ValidTo:   validTo.Int64,
			}
			if sup != nil {
				prevID, err := domain.ParseIDBytes(sup)
				if err != nil {
					return fmt.Errorf("ler supersedes da versão: %w", err)
				}
				prev := domain.PageID(prevID)
				pv.Supersedes = &prev
			}
			out = append(out, pv)
			cur = sup
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("page chain %s: %w", ref.String(), err)
	}
	return out, nil
}
