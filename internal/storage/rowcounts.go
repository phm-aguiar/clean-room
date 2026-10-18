package storage

import (
	"database/sql"
	"fmt"
)

// CleanTargetTables são as tabelas que o reindex exige vazias antes de reconstruir o banco
// a partir do wiki (docs/lifecycle-ops.md, "Clean-target check"), na ordem do doc. Os nomes
// são constantes do esquema V01; nunca vêm de entrada externa.
var CleanTargetTables = []string{
	"workspaces",
	"projects",
	"pages",
	"links",
	"page_embeddings",
	"sessions",
	"observations",
	"handoffs",
	"users",
	"audit_log",
}

// RowCount é o número de linhas de uma tabela.
type RowCount struct {
	Table string
	Rows  int64
}

// CountRows conta as linhas de cada tabela de CleanTargetTables dentro de tx, na mesma
// ordem da lista (zeros incluídos). Quem chama escolhe a transação: pelo Writer (Do) ou
// por um banco aberto (Read), para não ler concorrendo com uma escrita em curso.
func CountRows(tx *sql.Tx) ([]RowCount, error) {
	out := make([]RowCount, 0, len(CleanTargetTables))
	for _, table := range CleanTargetTables {
		var n int64
		if err := tx.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
			return nil, fmt.Errorf("storage: contar linhas de %s: %w", table, err)
		}
		out = append(out, RowCount{Table: table, Rows: n})
	}
	return out, nil
}

// NonZeroRowCounts devolve só as contagens com linhas, na ordem recebida.
func NonZeroRowCounts(counts []RowCount) []RowCount {
	var out []RowCount
	for _, c := range counts {
		if c.Rows != 0 {
			out = append(out, c)
		}
	}
	return out
}
