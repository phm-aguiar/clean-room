package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// entScope converte o escopo de fixture (bytes) no domain.Scope usado pela API.
func entScope(t *testing.T, s schemaScope) domain.Scope {
	t.Helper()
	ws, err := domain.ParseIDBytes(s.ws1)
	if err != nil {
		t.Fatalf("ParseIDBytes ws: %v", err)
	}
	proj, err := domain.ParseIDBytes(s.proj1)
	if err != nil {
		t.Fatalf("ParseIDBytes proj: %v", err)
	}
	return domain.NewScope(domain.WorkspaceID(ws), domain.ProjectID(proj))
}

// entPageID converte bytes de fixture em domain.PageID.
func entPageID(t *testing.T, b []byte) domain.PageID {
	t.Helper()
	id, err := domain.ParseIDBytes(b)
	if err != nil {
		t.Fatalf("ParseIDBytes page: %v", err)
	}
	return domain.PageID(id)
}

func TestEntities_NormalizaBordasEspacosECaixa(t *testing.T) {
	casos := map[string]string{
		"  Postgres  Driver ": "postgres driver",
		"POSTGRES":            "postgres",
		"postgres":            "postgres",
		"\tGo\n\tmodule ":     "go module",
		"a   b \t c":          "a b c",
	}
	for raw, want := range casos {
		got, err := NormalizeEntity(raw)
		if err != nil {
			t.Errorf("NormalizeEntity(%q): erro inesperado %v", raw, err)
			continue
		}
		if got != want {
			t.Errorf("NormalizeEntity(%q) = %q, esperado %q", raw, got, want)
		}
	}
}

func TestEntities_RejeitaVaziaESoEspacos(t *testing.T) {
	for _, raw := range []string{"", "   ", "\t\n "} {
		if _, err := NormalizeEntity(raw); !errors.Is(err, ErrInvalidEntity) {
			t.Errorf("NormalizeEntity(%q): erro = %v, esperado ErrInvalidEntity", raw, err)
		}
	}
}

func TestEntities_LimiteDeSessentaEQuatroCaracteresBrutos(t *testing.T) {
	aceito := strings.Repeat("a", 64)
	if got, err := NormalizeEntity(aceito); err != nil || got != aceito {
		t.Errorf("64 caracteres: got %q, err %v; esperado aceito", got, err)
	}

	// 64 runas multibyte são 64 caracteres, mesmo com mais de 64 bytes.
	multi := strings.Repeat("é", 64)
	if got, err := NormalizeEntity(multi); err != nil || got != multi {
		t.Errorf("64 runas multibyte: got %q, err %v; esperado aceito", got, err)
	}

	rejeitado := strings.Repeat("a", 65)
	if _, err := NormalizeEntity(rejeitado); !errors.Is(err, ErrInvalidEntity) {
		t.Errorf("65 caracteres: erro = %v, esperado ErrInvalidEntity", err)
	}

	// O limite vale para o bruto: 65 caracteres brutos com espaço final são rejeitados,
	// mesmo que a normalização (que apara) chegasse a 64.
	brutoComEspaco := strings.Repeat("a", 64) + " "
	if _, err := NormalizeEntity(brutoComEspaco); !errors.Is(err, ErrInvalidEntity) {
		t.Errorf("65 brutos com espaço final: erro = %v, esperado ErrInvalidEntity (limite é do bruto)", err)
	}
}

func TestEntities_RecusaCaractereDeControle(t *testing.T) {
	for _, raw := range []string{"a\x01b", "a\x7fb", "\x00", "ab\x1b"} {
		if _, err := NormalizeEntity(raw); !errors.Is(err, ErrInvalidEntity) {
			t.Errorf("NormalizeEntity(%q): erro = %v, esperado ErrInvalidEntity", raw, err)
		}
	}
}

func TestEntities_RecusaUTF8Invalido(t *testing.T) {
	if _, err := NormalizeEntity("a\xffb"); !errors.Is(err, ErrInvalidEntity) {
		t.Errorf("UTF-8 inválido: erro = %v, esperado ErrInvalidEntity", err)
	}
}

func TestEntities_ListaRemoveDuplicataMantendoPrimeiraOrdem(t *testing.T) {
	got := NormalizeEntities([]string{"Go", "b", "GO", "c", " b ", "go"})
	want := []string{"go", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("NormalizeEntities = %v, esperado %v", got, want)
	}
}

func TestEntities_ListaDescartaInvalidasAntesDeContar(t *testing.T) {
	entrada := []string{"", "\x01", strings.Repeat("x", 65)}
	for i := 0; i < 12; i++ {
		entrada = append(entrada, fmt.Sprintf("ent%02d", i))
	}
	got := NormalizeEntities(entrada)
	if len(got) != MaxEntitiesPerPage {
		t.Fatalf("len = %d, esperado %d", len(got), MaxEntitiesPerPage)
	}
	if got[0] != "ent00" || got[9] != "ent09" {
		t.Errorf("válidas não foram contadas a partir da primeira: %v", got)
	}
}

func TestEntities_ListaCortaEmDez(t *testing.T) {
	entrada := make([]string, 0, 15)
	for i := 0; i < 15; i++ {
		entrada = append(entrada, fmt.Sprintf("nome%02d", i))
	}
	got := NormalizeEntities(entrada)
	if len(got) != 10 {
		t.Fatalf("len = %d, esperado 10", len(got))
	}
	if got[9] != "nome09" {
		t.Errorf("último = %q, esperado nome09", got[9])
	}
}

func TestEntities_DerivacaoExplicitasAntesDeTagsNoLimite(t *testing.T) {
	var fm domain.Frontmatter
	explicitas := make([]string, 0, 10)
	for i := 0; i < 10; i++ {
		explicitas = append(explicitas, fmt.Sprintf("explicita%d", i))
	}
	if err := fm.Set("entities", explicitas); err != nil {
		t.Fatalf("Set entities: %v", err)
	}
	if err := fm.Set("tags", []string{"tag-a", "tag-b"}); err != nil {
		t.Fatalf("Set tags: %v", err)
	}
	got := DeriveEntities(fm)
	if !reflect.DeepEqual(got, explicitas) {
		t.Errorf("DeriveEntities = %v, esperado apenas as 10 explícitas %v", got, explicitas)
	}
}

func TestEntities_DerivacaoCompletaExplicitasDepoisTags(t *testing.T) {
	var fm domain.Frontmatter
	if err := fm.Set("entities", []string{"Alpha", "beta", "alpha"}); err != nil {
		t.Fatalf("Set entities: %v", err)
	}
	if err := fm.Set("tags", []string{"gamma", "beta", "delta", "a", "b", "c", "d", "e", "f"}); err != nil {
		t.Fatalf("Set tags: %v", err)
	}
	got := DeriveEntities(fm)
	want := []string{"alpha", "beta", "gamma", "delta", "a", "b", "c", "d", "e", "f"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("DeriveEntities = %v, esperado %v", got, want)
	}
}

func TestEntities_CampoNaoListaNaoQuebra(t *testing.T) {
	var fm domain.Frontmatter
	if err := fm.Set("entities", "postgres"); err != nil {
		t.Fatalf("Set entities string: %v", err)
	}
	if err := fm.Set("tags", []string{"x"}); err != nil {
		t.Fatalf("Set tags: %v", err)
	}
	got := DeriveEntities(fm)
	if !reflect.DeepEqual(got, []string{"x"}) {
		t.Errorf("DeriveEntities com entities não-lista = %v, esperado [x]", got)
	}

	var vazio domain.Frontmatter
	if got := DeriveEntities(vazio); got == nil || len(got) != 0 {
		t.Errorf("DeriveEntities sem campos = %v, esperado vazio não nulo", got)
	}
}

// entFixture abre o banco com o esquema real, cria escopo e duas versões de página
// (v1 antiga, v2 corrente) no mesmo path.
type entFixture struct {
	db     *sql.DB
	w      Writer
	scope  domain.Scope
	v1, v2 domain.PageID
}

func newEntFixture(t *testing.T) entFixture {
	t.Helper()
	db := openSchemaDB(t)
	s := seedSchemaScope(t, db)

	v1Bytes := schemaID(0x31)
	v2Bytes := schemaID(0x32)
	if err := insertSchemaPage(db, v1Bytes, s.ws1, s.proj1, "notas.md", "Notas", "corpo v1", 0); err != nil {
		t.Fatalf("insert v1: %v", err)
	}
	if err := insertSchemaPage(db, v2Bytes, s.ws1, s.proj1, "notas.md", "Notas", "corpo v2", 1); err != nil {
		t.Fatalf("insert v2: %v", err)
	}

	w, err := NewWriter(context.Background(), db)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })

	return entFixture{
		db:    db,
		w:     w,
		scope: entScope(t, s),
		v1:    entPageID(t, v1Bytes),
		v2:    entPageID(t, v2Bytes),
	}
}

// linkWindow lê valid_from e superseded_at do vínculo de uma página com uma entidade.
func linkWindow(t *testing.T, db *sql.DB, name string, page domain.PageID) (validFrom int64, supersededAt sql.NullInt64, found bool) {
	t.Helper()
	err := db.QueryRow(
		`SELECT l.valid_from, l.superseded_at
		   FROM entity_page_links l JOIN entities e ON e.id = l.entity_id
		  WHERE e.name = ? AND l.page_id = ?`,
		name, page.Bytes(),
	).Scan(&validFrom, &supersededAt)
	if err == sql.ErrNoRows {
		return 0, sql.NullInt64{}, false
	}
	if err != nil {
		t.Fatalf("ler janela de %q: %v", name, err)
	}
	return validFrom, supersededAt, true
}

func TestEntities_JanelaVinculoNovoAbertoEFechamentoGravaInstante(t *testing.T) {
	f := newEntFixture(t)
	ctx := context.Background()
	const v1From int64 = 1000
	const v2From int64 = 2000

	if err := LinkEntities(ctx, f.w, f.scope, f.v1, v1From, []string{"Postgres"}); err != nil {
		t.Fatalf("LinkEntities v1: %v", err)
	}
	from, sup, ok := linkWindow(t, f.db, "postgres", f.v1)
	if !ok {
		t.Fatal("vínculo v1 não foi criado")
	}
	if from != v1From {
		t.Errorf("valid_from = %d, esperado %d", from, v1From)
	}
	if sup.Valid {
		t.Errorf("superseded_at = %d, esperado NULL (janela aberta)", sup.Int64)
	}

	if err := CloseEntityLinks(ctx, f.w, f.v1, v2From); err != nil {
		t.Fatalf("CloseEntityLinks: %v", err)
	}
	_, sup, _ = linkWindow(t, f.db, "postgres", f.v1)
	if !sup.Valid || sup.Int64 != v2From {
		t.Errorf("superseded_at após fechar = %+v, esperado %d", sup, v2From)
	}

	if err := LinkEntities(ctx, f.w, f.scope, f.v2, v2From, []string{"postgres"}); err != nil {
		t.Fatalf("LinkEntities v2: %v", err)
	}
	_, sup2, ok := linkWindow(t, f.db, "postgres", f.v2)
	if !ok || sup2.Valid {
		t.Errorf("vínculo v2: ok=%v superseded=%+v, esperado aberto", ok, sup2)
	}

	abertos := countSchemaRows(t, f.db,
		`SELECT COUNT(*) FROM entity_page_links l JOIN entities e ON e.id = l.entity_id
		  WHERE e.name = ? AND l.superseded_at IS NULL`, "postgres")
	if abertos != 1 {
		t.Errorf("janelas abertas para postgres = %d, esperado 1 (só a versão corrente)", abertos)
	}
}

func TestEntities_EntidadeRepetidaNaoDuplica(t *testing.T) {
	f := newEntFixture(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if err := LinkEntities(ctx, f.w, f.scope, f.v1, 1000, []string{"Go", "GO", " go "}); err != nil {
			t.Fatalf("LinkEntities repetição %d: %v", i, err)
		}
	}
	if n := countSchemaRows(t, f.db, `SELECT COUNT(*) FROM entities WHERE name = ?`, "go"); n != 1 {
		t.Errorf("entidades 'go' = %d, esperado 1", n)
	}
	if n := countSchemaRows(t, f.db, `SELECT COUNT(*) FROM entity_page_links WHERE page_id = ?`, f.v1.Bytes()); n != 1 {
		t.Errorf("vínculos da página = %d, esperado 1", n)
	}

	// Mesma entidade em outra página reaproveita a entidade e cria só o vínculo novo.
	if err := LinkEntities(ctx, f.w, f.scope, f.v2, 2000, []string{"go"}); err != nil {
		t.Fatalf("LinkEntities v2: %v", err)
	}
	if n := countSchemaRows(t, f.db, `SELECT COUNT(*) FROM entities WHERE name = ?`, "go"); n != 1 {
		t.Errorf("após segunda página, entidades 'go' = %d, esperado 1", n)
	}
	if n := countSchemaRows(t, f.db,
		`SELECT COUNT(*) FROM entity_page_links l JOIN entities e ON e.id = l.entity_id WHERE e.name = ?`, "go"); n != 2 {
		t.Errorf("vínculos de 'go' = %d, esperado 2", n)
	}
}

func TestEntities_RelinkNaoReabreJanelaFechada(t *testing.T) {
	f := newEntFixture(t)
	ctx := context.Background()

	if err := LinkEntities(ctx, f.w, f.scope, f.v1, 1000, []string{"go"}); err != nil {
		t.Fatalf("LinkEntities: %v", err)
	}
	if err := CloseEntityLinks(ctx, f.w, f.v1, 2000); err != nil {
		t.Fatalf("CloseEntityLinks: %v", err)
	}
	if err := LinkEntities(ctx, f.w, f.scope, f.v1, 1000, []string{"go"}); err != nil {
		t.Fatalf("LinkEntities repetido: %v", err)
	}
	_, sup, _ := linkWindow(t, f.db, "go", f.v1)
	if !sup.Valid || sup.Int64 != 2000 {
		t.Errorf("superseded_at após relink = %+v, esperado 2000 mantido", sup)
	}
}

func TestEntities_LinkDerivadoDoFrontmatterRespeitaLimite(t *testing.T) {
	f := newEntFixture(t)
	ctx := context.Background()

	var fm domain.Frontmatter
	nomes := make([]string, 0, 12)
	for i := 0; i < 12; i++ {
		nomes = append(nomes, fmt.Sprintf("ent%02d", i))
	}
	if err := fm.Set("entities", nomes); err != nil {
		t.Fatalf("Set entities: %v", err)
	}
	if err := LinkEntities(ctx, f.w, f.scope, f.v1, 1000, DeriveEntities(fm)); err != nil {
		t.Fatalf("LinkEntities: %v", err)
	}
	if n := countSchemaRows(t, f.db, `SELECT COUNT(*) FROM entity_page_links WHERE page_id = ?`, f.v1.Bytes()); n != 10 {
		t.Errorf("vínculos = %d, esperado 10 (teto por página)", n)
	}
	if n := countSchemaRows(t, f.db, `SELECT COUNT(*) FROM entities WHERE name = ?`, "ent10"); n != 0 {
		t.Errorf("entidade além do limite foi criada")
	}
}

func TestEntities_LinkDentroDeTransacaoRespeitaRollback(t *testing.T) {
	f := newEntFixture(t)
	ctx := context.Background()

	err := f.w.Do(ctx, func(tx *sql.Tx) error {
		if err := LinkEntitiesTx(tx, f.scope, f.v1, 1000, []string{"rollback"}); err != nil {
			return err
		}
		return errors.New("forçar rollback")
	})
	if err == nil {
		t.Fatal("Do devolveu nil, esperado erro da closure")
	}
	if n := countSchemaRows(t, f.db, `SELECT COUNT(*) FROM entity_page_links WHERE page_id = ?`, f.v1.Bytes()); n != 0 {
		t.Errorf("vínculos após rollback = %d, esperado 0", n)
	}
}
