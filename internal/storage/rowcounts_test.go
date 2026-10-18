package storage

import (
	"database/sql"
	"testing"
)

// rowCountsInTx roda CountRows numa transação do próprio db e faz rollback (só leitura).
func rowCountsInTx(t *testing.T, db *sql.DB) []RowCount {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	counts, err := CountRows(tx)
	if err != nil {
		t.Fatalf("CountRows: %v", err)
	}
	return counts
}

func TestRowCountBancoLimpo(t *testing.T) {
	db := openSchemaDB(t)

	counts := rowCountsInTx(t, db)
	if len(counts) != len(CleanTargetTables) {
		t.Fatalf("CountRows devolveu %d tabelas, esperado %d", len(counts), len(CleanTargetTables))
	}
	for i, c := range counts {
		if c.Table != CleanTargetTables[i] {
			t.Fatalf("ordem: posição %d é %q, esperado %q", i, c.Table, CleanTargetTables[i])
		}
		if c.Rows != 0 {
			t.Fatalf("banco limpo: %s = %d, esperado 0", c.Table, c.Rows)
		}
	}
	if nz := NonZeroRowCounts(counts); len(nz) != 0 {
		t.Fatalf("NonZeroRowCounts em banco limpo = %+v, esperado vazio", nz)
	}
}

func TestRowCountUmaTabela(t *testing.T) {
	db := openSchemaDB(t)
	mustSchemaExec(t, db, `INSERT INTO workspaces (id, name, created_at) VALUES (?, ?, ?)`, schemaID(0x01), "ws-solo", schemaTS)

	nz := NonZeroRowCounts(rowCountsInTx(t, db))
	if len(nz) != 1 {
		t.Fatalf("uma tabela com linhas: NonZeroRowCounts = %+v, esperado uma entrada", nz)
	}
	if nz[0].Table != "workspaces" || nz[0].Rows != 1 {
		t.Fatalf("NonZeroRowCounts = %+v, esperado workspaces com 1", nz[0])
	}
}

func TestRowCountVariasTabelas(t *testing.T) {
	db := openSchemaDB(t)
	seedSchemaScope(t, db)

	nz := NonZeroRowCounts(rowCountsInTx(t, db))
	if len(nz) < 2 {
		t.Fatalf("várias tabelas com linhas: NonZeroRowCounts = %+v, esperado pelo menos duas", nz)
	}
	// Cada contagem precisa bater com um SELECT COUNT(*) direto na tabela.
	for _, c := range nz {
		var direct int64
		if err := db.QueryRow("SELECT COUNT(*) FROM " + c.Table).Scan(&direct); err != nil {
			t.Fatalf("COUNT direto em %s: %v", c.Table, err)
		}
		if direct != c.Rows {
			t.Fatalf("%s: CountRows = %d, COUNT direto = %d", c.Table, c.Rows, direct)
		}
	}
	found := map[string]bool{}
	for _, c := range nz {
		found[c.Table] = true
	}
	if !found["workspaces"] || !found["projects"] {
		t.Fatalf("esperava workspaces e projects com linhas, veio %+v", nz)
	}
}
