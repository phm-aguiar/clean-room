package storage

import (
	"bytes"
	"context"
	"database/sql"
	"slices"
	"strings"
	"testing"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// lkMustPath cria um PagePath válido para o teste.
func lkMustPath(t *testing.T, s string) domain.PagePath {
	t.Helper()
	p, err := domain.ParsePagePath(s)
	if err != nil {
		t.Fatalf("ParsePagePath(%q): %v", s, err)
	}
	return p
}

// lkNewPageID gera um PageID novo.
func lkNewPageID(t *testing.T) domain.PageID {
	t.Helper()
	id, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	return domain.PageID(id)
}

// lkInsertPage insere uma página (versão mais recente) dentro de tx.
func lkInsertPage(tx *sql.Tx, id domain.PageID, scope domain.Scope, path string) error {
	micros := esTestT0.UnixMicro()
	_, err := tx.Exec(
		`INSERT INTO pages (id, workspace_id, project_id, path, title, tier, body, body_sha256, is_latest, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 'semantic', '', ?, 1, ?, ?)`,
		id.Bytes(), scope.WorkspaceID.Bytes(), scope.ProjectID.Bytes(), path, path, make([]byte, 32), micros, micros,
	)
	return err
}

// lkDo roda fn dentro de w.Do e falha o teste em caso de erro.
func lkDo(t *testing.T, w Writer, fn func(*sql.Tx) error) {
	t.Helper()
	if err := w.Do(context.Background(), fn); err != nil {
		t.Fatalf("Do: %v", err)
	}
}

// lkRows devolve "to_path|link_type|resolvido" dos links de from, ordenados.
func lkRows(t *testing.T, db *sql.DB, from domain.PageID) []string {
	t.Helper()
	rows, err := db.Query(
		`SELECT to_path, link_type, to_page_id IS NOT NULL FROM links WHERE from_page_id = ? ORDER BY to_path, link_type`,
		from.Bytes(),
	)
	if err != nil {
		t.Fatalf("query links: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var path, lt string
		var resolved bool
		if err := rows.Scan(&path, &lt, &resolved); err != nil {
			t.Fatalf("scan link: %v", err)
		}
		res := "false"
		if resolved {
			res = "true"
		}
		out = append(out, path+"|"+lt+"|"+res)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate links: %v", err)
	}
	return out
}

// lkTargetID devolve o to_page_id do link de from para toPath.
func lkTargetID(t *testing.T, db *sql.DB, from domain.PageID, toPath string) []byte {
	t.Helper()
	var id []byte
	if err := db.QueryRow(
		`SELECT to_page_id FROM links WHERE from_page_id = ? AND to_path = ?`,
		from.Bytes(), toPath,
	).Scan(&id); err != nil {
		t.Fatalf("target of %s: %v", toPath, err)
	}
	return id
}

func lkStr(s string) *string { return &s }

func TestLinkRegravaSubstituiConjuntoAntigo(t *testing.T) {
	db, w := esTestEnv(t)
	scope := esTestScope(t)
	if err := EnsureScope(context.Background(), w, scope, "ws-links", "p1", esTestT0); err != nil {
		t.Fatalf("EnsureScope: %v", err)
	}
	a := lkNewPageID(t)
	lkDo(t, w, func(tx *sql.Tx) error { return lkInsertPage(tx, a, scope, "a.md") })

	lkDo(t, w, func(tx *sql.Tx) error {
		_, err := ReplaceLinks(tx, a, []Link{
			{ToPath: lkMustPath(t, "b.md")},
			{ToPath: lkMustPath(t, "c.md"), Relation: "causes"},
		})
		return err
	})
	if got := lkRows(t, db, a); !slices.Equal(got, []string{"b.md|references|false", "c.md|causes|false"}) {
		t.Fatalf("primeira regravação: %v", got)
	}

	lkDo(t, w, func(tx *sql.Tx) error {
		_, err := ReplaceLinks(tx, a, []Link{{ToPath: lkMustPath(t, "d.md"), Relation: "fixes"}})
		return err
	})
	if got := lkRows(t, db, a); !slices.Equal(got, []string{"d.md|fixes|false"}) {
		t.Fatalf("segunda regravação deve substituir o conjunto: %v", got)
	}
}

func TestLinkDedupeNaMesmaChamada(t *testing.T) {
	db, w := esTestEnv(t)
	scope := esTestScope(t)
	if err := EnsureScope(context.Background(), w, scope, "ws-dedupe", "p1", esTestT0); err != nil {
		t.Fatalf("EnsureScope: %v", err)
	}
	a := lkNewPageID(t)
	lkDo(t, w, func(tx *sql.Tx) error { return lkInsertPage(tx, a, scope, "a.md") })

	var avisos []string
	lkDo(t, w, func(tx *sql.Tx) error {
		var err error
		avisos, err = ReplaceLinks(tx, a, []Link{
			{ToPath: lkMustPath(t, "b.md")},
			{ToPath: lkMustPath(t, "b.md")},
			{ToPath: lkMustPath(t, "b.md"), Relation: "causes"},
			// Mesma chave lógica que o primeiro (references, b.md) depois do fallback: descartado.
			{ToPath: lkMustPath(t, "b.md"), Relation: "supports"},
		})
		return err
	})
	if len(avisos) != 0 {
		t.Fatalf("duplicata descartada não deve gerar aviso: %v", avisos)
	}
	if got := lkRows(t, db, a); !slices.Equal(got, []string{"b.md|causes|false", "b.md|references|false"}) {
		t.Fatalf("dedupe: %v", got)
	}
}

func TestLinkRelacaoDesconhecidaViraReferences(t *testing.T) {
	db, w := esTestEnv(t)
	scope := esTestScope(t)
	if err := EnsureScope(context.Background(), w, scope, "ws-unk", "p1", esTestT0); err != nil {
		t.Fatalf("EnsureScope: %v", err)
	}
	a := lkNewPageID(t)
	lkDo(t, w, func(tx *sql.Tx) error { return lkInsertPage(tx, a, scope, "a.md") })

	var avisos []string
	lkDo(t, w, func(tx *sql.Tx) error {
		var err error
		avisos, err = ReplaceLinks(tx, a, []Link{{ToPath: lkMustPath(t, "c.md"), Relation: "supports"}})
		return err
	})
	if !slices.Equal(avisos, []string{"supports"}) {
		t.Fatalf("avisos: %v", avisos)
	}
	if got := lkRows(t, db, a); !slices.Equal(got, []string{"c.md|references|false"}) {
		t.Fatalf("relação desconhecida deve virar references: %v", got)
	}
}

func TestLinkRelacaoLongaTruncadaNoAviso(t *testing.T) {
	_, w := esTestEnv(t)
	scope := esTestScope(t)
	if err := EnsureScope(context.Background(), w, scope, "ws-trunc", "p1", esTestT0); err != nil {
		t.Fatalf("EnsureScope: %v", err)
	}
	a := lkNewPageID(t)
	lkDo(t, w, func(tx *sql.Tx) error { return lkInsertPage(tx, a, scope, "a.md") })

	// "é" ocupa 2 bytes: 150 vezes = 300 bytes; o corte em 200 cai no meio de um caractere e deve recuar.
	long := strings.Repeat("é", 150)
	var avisos []string
	lkDo(t, w, func(tx *sql.Tx) error {
		var err error
		avisos, err = ReplaceLinks(tx, a, []Link{{ToPath: lkMustPath(t, "c.md"), Relation: long}})
		return err
	})
	if len(avisos) != 1 || len(avisos[0]) > 200 || !strings.HasPrefix(long, avisos[0]) {
		t.Fatalf("aviso truncado inválido: %q", avisos)
	}
}

func TestLinkPendenteReparadoQuandoAlvoAparece(t *testing.T) {
	db, w := esTestEnv(t)
	scope := esTestScope(t)
	if err := EnsureScope(context.Background(), w, scope, "ws-rep", "p1", esTestT0); err != nil {
		t.Fatalf("EnsureScope: %v", err)
	}
	a := lkNewPageID(t)
	b := lkNewPageID(t)
	lkDo(t, w, func(tx *sql.Tx) error { return lkInsertPage(tx, a, scope, "a.md") })
	lkDo(t, w, func(tx *sql.Tx) error {
		_, err := ReplaceLinks(tx, a, []Link{{ToPath: lkMustPath(t, "b.md")}})
		return err
	})
	if got := lkRows(t, db, a); !slices.Equal(got, []string{"b.md|references|false"}) {
		t.Fatalf("link deve ficar pendente antes do alvo: %v", got)
	}

	lkDo(t, w, func(tx *sql.Tx) error {
		if err := lkInsertPage(tx, b, scope, "b.md"); err != nil {
			return err
		}
		return RepairPendingLinks(tx, b)
	})
	if got := lkRows(t, db, a); !slices.Equal(got, []string{"b.md|references|true"}) {
		t.Fatalf("link deve ser reparado: %v", got)
	}
	if got := lkTargetID(t, db, a, "b.md"); !bytes.Equal(got, b.Bytes()) {
		t.Fatalf("to_page_id = %x, quer %x", got, b.Bytes())
	}
}

func TestLinkAlvoExistenteResolvidoNaInsercao(t *testing.T) {
	db, w := esTestEnv(t)
	scope := esTestScope(t)
	if err := EnsureScope(context.Background(), w, scope, "ws-ins", "p1", esTestT0); err != nil {
		t.Fatalf("EnsureScope: %v", err)
	}
	a := lkNewPageID(t)
	b := lkNewPageID(t)
	lkDo(t, w, func(tx *sql.Tx) error {
		if err := lkInsertPage(tx, b, scope, "b.md"); err != nil {
			return err
		}
		return lkInsertPage(tx, a, scope, "a.md")
	})
	lkDo(t, w, func(tx *sql.Tx) error {
		_, err := ReplaceLinks(tx, a, []Link{{ToPath: lkMustPath(t, "b.md")}})
		return err
	})
	if got := lkRows(t, db, a); !slices.Equal(got, []string{"b.md|references|true"}) {
		t.Fatalf("alvo existente deve ser resolvido na inserção: %v", got)
	}
}

func TestLinkReparoEntreProjetosDoMesmoWorkspace(t *testing.T) {
	db, w := esTestEnv(t)
	ctx := context.Background()
	s1 := esTestScope(t)
	s2 := domain.NewScope(s1.WorkspaceID, esTestScope(t).ProjectID)
	if err := EnsureScope(ctx, w, s1, "ws-cross", "p1", esTestT0); err != nil {
		t.Fatalf("EnsureScope s1: %v", err)
	}
	if err := EnsureScope(ctx, w, s2, "ws-cross", "p2", esTestT0); err != nil {
		t.Fatalf("EnsureScope s2: %v", err)
	}
	// Outro workspace com um projeto de mesmo nome (p2): não deve casar com links do ws-cross.
	s3 := esTestScope(t)
	if err := EnsureScope(ctx, w, s3, "ws-outro", "p2", esTestT0); err != nil {
		t.Fatalf("EnsureScope s3: %v", err)
	}

	a := lkNewPageID(t)
	lkDo(t, w, func(tx *sql.Tx) error { return lkInsertPage(tx, a, s1, "a.md") })
	lkDo(t, w, func(tx *sql.Tx) error {
		_, err := ReplaceLinks(tx, a, []Link{
			{ToProject: lkStr("p2"), ToPath: lkMustPath(t, "c.md")},
			{ToWorkspace: lkStr("ws-cross"), ToProject: lkStr("p2"), ToPath: lkMustPath(t, "d.md")},
			{ToPath: lkMustPath(t, "e.md")}, // local: deve casar só com página de p1
		})
		return err
	})

	// Página de outro workspace com o caminho d.md: não repara o link para ws-cross/p2.
	z := lkNewPageID(t)
	lkDo(t, w, func(tx *sql.Tx) error {
		if err := lkInsertPage(tx, z, s3, "d.md"); err != nil {
			return err
		}
		return RepairPendingLinks(tx, z)
	})
	if got := lkRows(t, db, a); !slices.Equal(got, []string{"c.md|references|false", "d.md|references|false", "e.md|references|false"}) {
		t.Fatalf("página de outro workspace não deve reparar: %v", got)
	}

	// Página em p2 (mesmo workspace): repara c.md (projeto explícito) e d.md; e.md é local e não casa.
	c := lkNewPageID(t)
	d := lkNewPageID(t)
	e := lkNewPageID(t)
	lkDo(t, w, func(tx *sql.Tx) error {
		if err := lkInsertPage(tx, c, s2, "c.md"); err != nil {
			return err
		}
		if err := lkInsertPage(tx, d, s2, "d.md"); err != nil {
			return err
		}
		if err := RepairPendingLinks(tx, c); err != nil {
			return err
		}
		return RepairPendingLinks(tx, d)
	})
	if got := lkRows(t, db, a); !slices.Equal(got, []string{"c.md|references|true", "d.md|references|true", "e.md|references|false"}) {
		t.Fatalf("reparo entre projetos do mesmo workspace: %v", got)
	}
	if got := lkTargetID(t, db, a, "d.md"); !bytes.Equal(got, d.Bytes()) {
		t.Fatalf("d.md deve apontar para a página de ws-cross/p2, não para a de ws-outro: %x", got)
	}

	// Página local em p2 com o caminho e.md não casa com link local de p1.
	lkDo(t, w, func(tx *sql.Tx) error {
		if err := lkInsertPage(tx, e, s2, "e.md"); err != nil {
			return err
		}
		return RepairPendingLinks(tx, e)
	})
	if got := lkRows(t, db, a); !slices.Equal(got, []string{"c.md|references|true", "d.md|references|true", "e.md|references|false"}) {
		t.Fatalf("link local de p1 não deve ser reparado por página de p2: %v", got)
	}
}

func TestLinkRepairPageInexistenteErra(t *testing.T) {
	_, w := esTestEnv(t)
	err := w.Do(context.Background(), func(tx *sql.Tx) error {
		return RepairPendingLinks(tx, lkNewPageID(t))
	})
	if err == nil {
		t.Fatal("página inexistente deve dar erro")
	}
}
