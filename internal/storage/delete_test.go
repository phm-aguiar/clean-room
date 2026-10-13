package storage

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// delEnv abre um banco com escritor, cria um escopo e devolve a referência do caminho.
func delEnv(t *testing.T, path string) (*sql.DB, Writer, domain.Scope, domain.PageRef) {
	t.Helper()
	db, w, ws := upEnv(t)
	scope := upScope(t, w, ws, "ws-del", "pj-del")
	return db, w, scope, domain.NewPageRef(scope, upPath(t, path))
}

// delMustDelete chama DeletePath e falha o teste se houver erro.
func delMustDelete(t *testing.T, w Writer, ref domain.PageRef, expected domain.PageID) bool {
	t.Helper()
	deleted, err := DeletePath(context.Background(), w, ref, expected)
	if err != nil {
		t.Fatalf("DeletePath: %v", err)
	}
	return deleted
}

// delAuditCount conta as linhas de auditoria page-delete do escopo.
func delAuditCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	return upCount(t, db, `SELECT COUNT(*) FROM audit_log WHERE op = ?`, auditPageDelete)
}

func TestDeletePath_ApagaVersaoCorrenteQuandoIDBate(t *testing.T) {
	db, w, _, ref := delEnv(t, "notas/rota.md")
	r := upMustUpsert(t, w, upIn(t, ref, "corpo", "sess-1", upT0))

	if n := upCount(t, db, `SELECT COUNT(*) FROM page_evidence WHERE page_id = ?`, r.Version.Bytes()); n != 1 {
		t.Fatalf("pré-condição: evidência da página = %d, esperada 1", n)
	}
	if n := upCount(t, db, `SELECT COUNT(*) FROM entity_page_links WHERE page_id = ?`, r.Version.Bytes()); n != 1 {
		t.Fatalf("pré-condição: vínculos de entidade = %d, esperado 1", n)
	}

	if !delMustDelete(t, w, ref, r.Version) {
		t.Fatalf("deleted = false, esperado true")
	}

	if n := upCount(t, db, `SELECT COUNT(*) FROM pages WHERE id = ?`, r.Version.Bytes()); n != 0 {
		t.Errorf("versão apagada ainda existe em pages (%d linhas)", n)
	}
	if n := upCount(t, db, `SELECT COUNT(*) FROM pages WHERE path = ? AND is_latest = 1`, "notas/rota.md"); n != 0 {
		t.Errorf("caminho ainda tem versão corrente (%d linhas)", n)
	}
	if n := upCount(t, db, `SELECT COUNT(*) FROM page_evidence WHERE page_id = ?`, r.Version.Bytes()); n != 0 {
		t.Errorf("evidência da versão apagada sobreviveu (%d linhas)", n)
	}
	if n := upCount(t, db, `SELECT COUNT(*) FROM entity_page_links WHERE page_id = ?`, r.Version.Bytes()); n != 0 {
		t.Errorf("vínculos de entidade da versão apagada sobreviveram (%d linhas)", n)
	}
}

func TestDeletePath_IDEsperadoDiferenteDevolveMismatchESemMudanca(t *testing.T) {
	db, w, _, ref := delEnv(t, "notas/rota.md")
	r1 := upMustUpsert(t, w, upIn(t, ref, "corpo v1", "sess-1", upT0))
	r2 := upMustUpsert(t, w, upIn(t, ref, "corpo v2", "sess-2", upT0.Add(time.Minute)))
	antes := upTake(t, db)
	auditAntes := delAuditCount(t, db)

	deleted, err := DeletePath(context.Background(), w, ref, r1.Version)
	if !errors.Is(err, ErrVersionMismatch) {
		t.Fatalf("erro = %v, esperado ErrVersionMismatch", err)
	}
	if deleted {
		t.Fatalf("deleted = true, esperado false")
	}

	if depois := upTake(t, db); depois != antes {
		t.Errorf("mismatch alterou o banco: antes %+v, depois %+v", antes, depois)
	}
	if n := delAuditCount(t, db); n != auditAntes {
		t.Errorf("mismatch gravou auditoria page-delete: antes %d, depois %d", auditAntes, n)
	}
	if f := upReadPage(t, db, r2.Version); f.isLatest != 1 {
		t.Errorf("versão corrente r2 perdeu is_latest após mismatch: %d", f.isLatest)
	}
}

func TestDeletePath_SemVersaoCorrenteEIdempotente(t *testing.T) {
	db, w, _, ref := delEnv(t, "notas/rota.md")
	r := upMustUpsert(t, w, upIn(t, ref, "corpo", "sess-1", upT0))

	if !delMustDelete(t, w, ref, r.Version) {
		t.Fatalf("primeira chamada: deleted = false, esperado true")
	}
	auditDepois1 := delAuditCount(t, db)
	antes := upTake(t, db)

	// Segunda chamada com o mesmo id: não há versão corrente, então não é erro e nada muda.
	if delMustDelete(t, w, ref, r.Version) {
		t.Errorf("segunda chamada: deleted = true, esperado false")
	}
	if depois := upTake(t, db); depois != antes {
		t.Errorf("segunda chamada alterou o banco: antes %+v, depois %+v", antes, depois)
	}
	if n := delAuditCount(t, db); n != auditDepois1 {
		t.Errorf("segunda chamada gravou auditoria: antes %d, depois %d", auditDepois1, n)
	}

	// Caminho que nunca existiu, com um id qualquer: também sem erro e sem efeito.
	outro := domain.NewPageRef(ref.Scope, upPath(t, "nunca/existiu.md"))
	qualquer, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	if delMustDelete(t, w, outro, domain.PageID(qualquer)) {
		t.Errorf("caminho inexistente: deleted = true, esperado false")
	}
}

func TestDeletePath_VersoesSupersededPermanecem(t *testing.T) {
	db, w, _, ref := delEnv(t, "notas/rota.md")
	r1 := upMustUpsert(t, w, upIn(t, ref, "corpo v1", "sess-1", upT0))
	r2 := upMustUpsert(t, w, upIn(t, ref, "corpo v2", "sess-2", upT0.Add(time.Minute)))

	if !delMustDelete(t, w, ref, r2.Version) {
		t.Fatalf("deleted = false, esperado true")
	}

	v1 := upReadPage(t, db, r1.Version)
	if v1.body != "corpo v1" {
		t.Errorf("corpo da versão superseded mudou: %q", v1.body)
	}
	if v1.isLatest != 0 {
		t.Errorf("versão superseded voltou a ser corrente: is_latest = %d", v1.isLatest)
	}
	if !v1.validTo.Valid {
		t.Errorf("janela da versão superseded foi alterada: valid_to NULL")
	}
	if n := upCount(t, db, `SELECT COUNT(*) FROM pages WHERE path = ? AND is_latest = 1`, "notas/rota.md"); n != 0 {
		t.Errorf("DeletePath reativou uma versão (%d correntes), esperado 0", n)
	}
}

func TestDeletePath_EntidadeOrfaRemovidaEEntidadeUsadaPermanece(t *testing.T) {
	db, w, scope, _ := delEnv(t, "notas/a.md")
	refA := domain.NewPageRef(scope, upPath(t, "notas/a.md"))
	refB := domain.NewPageRef(scope, upPath(t, "notas/b.md"))

	inA := upIn(t, refA, "corpo a", "sess-1", upT0)
	inA.Frontmatter = upFM(t, "entities", []string{"Postgres", "Redis"})
	rA := upMustUpsert(t, w, inA)
	upMustUpsert(t, w, upIn(t, refB, "corpo b", "sess-2", upT0)) // entities: Postgres

	if n := upCount(t, db, `SELECT COUNT(*) FROM entities WHERE name = 'redis'`); n != 1 {
		t.Fatalf("pré-condição: entidade redis = %d, esperada 1", n)
	}

	if !delMustDelete(t, w, refA, rA.Version) {
		t.Fatalf("deleted = false, esperado true")
	}

	if n := upCount(t, db, `SELECT COUNT(*) FROM entities WHERE name = 'redis'`); n != 0 {
		t.Errorf("entidade órfã redis sobreviveu (%d linhas)", n)
	}
	if n := upCount(t, db, `SELECT COUNT(*) FROM entities WHERE name = 'postgres'`); n != 1 {
		t.Errorf("entidade postgres, ainda usada por b.md, foi removida (%d linhas)", n)
	}
	if n := upCount(t, db, `SELECT COUNT(*) FROM entity_page_links l JOIN entities e ON e.id = l.entity_id
	                          WHERE e.name = 'postgres'`); n != 1 {
		t.Errorf("vínculo de postgres com b.md foi perdido (%d linhas)", n)
	}
}

func TestDeletePath_GravaAuditoriaPageDelete(t *testing.T) {
	db, w, scope, ref := delEnv(t, "notas/rota.md")
	r := upMustUpsert(t, w, upIn(t, ref, "corpo", "sess-1", upT0))

	if !delMustDelete(t, w, ref, r.Version) {
		t.Fatalf("deleted = false, esperado true")
	}

	if n := upCount(t, db,
		`SELECT COUNT(*) FROM audit_log
		  WHERE op = ? AND page_id = ? AND workspace_id = ? AND project_id = ?`,
		auditPageDelete, r.Version.Bytes(), scope.WorkspaceID.Bytes(), scope.ProjectID.Bytes(),
	); n != 1 {
		t.Errorf("auditoria page-delete com alvo e escopo: %d linhas, esperada 1", n)
	}
}

func TestDeletePath_FalhaNoMeioFazRollbackDeTudo(t *testing.T) {
	db, w, scope, _ := delEnv(t, "notas/a.md")
	refA := domain.NewPageRef(scope, upPath(t, "notas/a.md"))
	inA := upIn(t, refA, "corpo a", "sess-1", upT0)
	inA.Frontmatter = upFM(t, "entities", []string{"Redis"})
	rA := upMustUpsert(t, w, inA)

	// Força falha depois do DELETE da página: a remoção de entidades órfãs passa por este gatilho.
	mustSchemaExec(t, db, `CREATE TRIGGER del_falha_simulada BEFORE DELETE ON entities
	                       BEGIN SELECT RAISE(ABORT, 'falha simulada'); END;`)

	antes := upTake(t, db)
	auditAntes := delAuditCount(t, db)

	deleted, err := DeletePath(context.Background(), w, refA, rA.Version)
	if err == nil {
		t.Fatalf("DeletePath: erro nil, esperado a falha simulada")
	}
	if deleted {
		t.Fatalf("deleted = true após falha")
	}
	if depois := upTake(t, db); depois != antes {
		t.Errorf("rollback incompleto: antes %+v, depois %+v", antes, depois)
	}
	if n := delAuditCount(t, db); n != auditAntes {
		t.Errorf("auditoria page-delete sobreviveu ao rollback: %d", n)
	}
	if n := upCount(t, db, `SELECT COUNT(*) FROM pages WHERE id = ?`, rA.Version.Bytes()); n != 1 {
		t.Errorf("página sumiu apesar do rollback (%d linhas)", n)
	}
}

func TestDeletePath_PassaPeloWriterDoEDevolveErrWriterClosed(t *testing.T) {
	db, w, _, ref := delEnv(t, "notas/rota.md")
	r := upMustUpsert(t, w, upIn(t, ref, "corpo", "sess-1", upT0))

	fechado, err := NewWriter(context.Background(), db)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	if err := fechado.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	deleted, err := DeletePath(context.Background(), fechado, ref, r.Version)
	if !errors.Is(err, ErrWriterClosed) {
		t.Fatalf("erro = %v, esperado ErrWriterClosed (a remoção deve passar por Writer.Do)", err)
	}
	if deleted {
		t.Fatalf("deleted = true com escritor fechado")
	}
	if n := upCount(t, db, `SELECT COUNT(*) FROM pages WHERE id = ?`, r.Version.Bytes()); n != 1 {
		t.Errorf("página apagada mesmo com escritor fechado (%d linhas)", n)
	}
}

func TestDeletePath_VinculosDeLinksPelasFKs(t *testing.T) {
	db, w, scope, _ := delEnv(t, "notas/a.md")
	refA := domain.NewPageRef(scope, upPath(t, "notas/a.md"))
	refB := domain.NewPageRef(scope, upPath(t, "notas/b.md"))

	inA := upIn(t, refA, "corpo a", "sess-1", upT0)
	inA.Links = []Link{{ToPath: upPath(t, "notas/b.md")}}
	rA := upMustUpsert(t, w, inA)
	rB := upMustUpsert(t, w, upIn(t, refB, "corpo b", "sess-2", upT0))

	if n := upCount(t, db, `SELECT COUNT(*) FROM links WHERE from_page_id = ? AND to_page_id = ?`,
		rA.Version.Bytes(), rB.Version.Bytes()); n != 1 {
		t.Fatalf("pré-condição: link a->b resolvido = %d, esperado 1", n)
	}

	// Apagar o destino: o link de origem continua, agora pendente (to_page_id NULL, to_path preservado).
	if !delMustDelete(t, w, refB, rB.Version) {
		t.Fatalf("deleted = false ao apagar b")
	}
	if n := upCount(t, db, `SELECT COUNT(*) FROM links WHERE from_page_id = ? AND to_page_id IS NULL AND to_path = ?`,
		rA.Version.Bytes(), "notas/b.md"); n != 1 {
		t.Errorf("link de a para b deveria ficar pendente, achado %d", n)
	}

	// Apagar a origem: os links de saída saem por CASCADE.
	if !delMustDelete(t, w, refA, rA.Version) {
		t.Fatalf("deleted = false ao apagar a")
	}
	if n := upCount(t, db, `SELECT COUNT(*) FROM links WHERE from_page_id = ?`, rA.Version.Bytes()); n != 0 {
		t.Errorf("links de origem da página apagada sobreviveram (%d)", n)
	}
}
