package storage

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

var ssT0 = time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)

// ssEnv abre o banco em diretório temporário, com escritor e leitor, e um workspace novo.
func ssEnv(t *testing.T) (Writer, Reader, domain.WorkspaceID) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	db, err := Open(ctx, dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	w, err := NewWriter(ctx, db)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	rdb, err := OpenReader(ctx, dir)
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	t.Cleanup(func() { _ = rdb.Close() })

	wsID, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID ws: %v", err)
	}
	return w, NewReader(rdb), domain.WorkspaceID(wsID)
}

// ssRef monta a referência de uma página do escopo.
func ssRef(t *testing.T, scope domain.Scope, path string) domain.PageRef {
	t.Helper()
	return domain.NewPageRef(scope, upPath(t, path))
}

// ssFM devolve o frontmatter com entities quando há nomes; zero caso contrário.
func ssFM(t *testing.T, names ...string) domain.Frontmatter {
	t.Helper()
	if len(names) == 0 {
		return domain.Frontmatter{}
	}
	return upFM(t, "entities", names)
}

// ssPut grava uma versão de página por UpsertPage, no instante at.
func ssPut(t *testing.T, w Writer, ref domain.PageRef, title, body string, fm domain.Frontmatter, links []Link, at time.Time) {
	t.Helper()
	upMustUpsert(t, w, UpsertInput{
		Ref:         ref,
		Tier:        domain.TierSemantic,
		Kind:        domain.PageKindNote,
		Title:       title,
		Body:        body,
		Frontmatter: fm,
		Links:       links,
		Evidence:    EvidenceSource{Kind: SourceSession, ID: "ss-" + ref.Path.String()},
		At:          at,
	})
}

// ssPaths devolve os caminhos de uma lista de referências, na mesma ordem.
func ssPaths(refs []domain.PageRef) []string {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.Path.String())
	}
	return out
}

// ssEqual compara a lista de caminhos com a esperada, na mesma ordem.
func ssEqual(t *testing.T, got []domain.PageRef, want []string) {
	t.Helper()
	gotPaths := ssPaths(got)
	if len(gotPaths) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(gotPaths, want) {
		t.Fatalf("caminhos = %v, esperados %v", gotPaths, want)
	}
}

func TestStreamCandidateLimit(t *testing.T) {
	cases := []struct{ limite, want int }{
		{-3, 0},
		{0, 0},
		{1, 20},
		{5, 20},
		{10, 40},
		{100, 400},
		{200, 500},
	}
	for _, c := range cases {
		if got := CandidateLimit(c.limite); got != c.want {
			t.Errorf("CandidateLimit(%d) = %d, esperado %d", c.limite, got, c.want)
		}
	}
}

func TestStreamEntityTokens(t *testing.T) {
	got := entityQueryTokens("ab abc abc post-gres Kube_DNS")
	want := []string{"abc", "post-gres", "post", "gres", "kube_dns", "kube", "dns"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("entityQueryTokens = %v, esperado %v", got, want)
	}

	var muitos []byte
	for i := 0; i < 15; i++ {
		muitos = append(muitos, []byte("tok"+string(rune('a'+i))+" ")...)
	}
	if n := len(entityQueryTokens(string(muitos))); n != entityMaxTokens {
		t.Fatalf("tokens = %d, esperado %d (limite)", n, entityMaxTokens)
	}
}

func TestStreamFTSEncontraPaginaEIgnoraSuperseded(t *testing.T) {
	w, r, ws := ssEnv(t)
	scope := upScope(t, w, ws, "ws-fts", "pj-fts")
	ctx := context.Background()

	rota := ssRef(t, scope, "notas/rota.md")
	ssPut(t, w, rota, "Rota", "corpo com postgres antigo", ssFM(t), nil, ssT0)
	ssPut(t, w, rota, "Rota", "corpo com redis novo", ssFM(t), nil, ssT0.Add(time.Minute))
	ssPut(t, w, ssRef(t, scope, "notas/outra.md"), "Outra", "postgres em produção", ssFM(t), nil, ssT0)
	cutoff := ssT0.Add(time.Hour)

	got, err := FTSFlow(ctx, r, scope, "postgres", cutoff, 10)
	if err != nil {
		t.Fatalf("FTSFlow: %v", err)
	}
	ssEqual(t, got, []string{"notas/outra.md"})

	got, err = FTSFlow(ctx, r, scope, "redis", cutoff, 10)
	if err != nil {
		t.Fatalf("FTSFlow redis: %v", err)
	}
	ssEqual(t, got, []string{"notas/rota.md"})
}

func TestStreamFTSExpiradaNaoAparece(t *testing.T) {
	w, r, ws := ssEnv(t)
	scope := upScope(t, w, ws, "ws-exp", "pj-exp")
	ctx := context.Background()

	outra := ssRef(t, scope, "notas/outra.md")
	ssPut(t, w, outra, "Outra", "postgres em produção", ssFM(t), nil, ssT0)
	if err := w.Do(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE pages SET expires_at = ? WHERE path = ?`,
			ssT0.Add(30*time.Minute).UnixMicro(), "notas/outra.md")
		return err
	}); err != nil {
		t.Fatalf("expirar: %v", err)
	}

	got, err := FTSFlow(ctx, r, scope, "postgres", ssT0.Add(time.Hour), 10)
	if err != nil {
		t.Fatalf("FTSFlow: %v", err)
	}
	ssEqual(t, got, nil)

	got, err = FTSFlow(ctx, r, scope, "postgres", ssT0.Add(10*time.Minute), 10)
	if err != nil {
		t.Fatalf("FTSFlow antes da expiração: %v", err)
	}
	ssEqual(t, got, []string{"notas/outra.md"})
}

func TestStreamFTSSoStopWordsOuPontuacaoNaoQuebra(t *testing.T) {
	w, r, ws := ssEnv(t)
	scope := upScope(t, w, ws, "ws-sw", "pj-sw")
	ctx := context.Background()
	ssPut(t, w, ssRef(t, scope, "notas/nota.md"), "Nota", "corpo sem termos", ssFM(t), nil, ssT0)
	cutoff := ssT0.Add(time.Hour)

	for _, q := range []string{"the and of", "!!! ???", "   ", ""} {
		got, err := FTSFlow(ctx, r, scope, q, cutoff, 10)
		if err != nil {
			t.Fatalf("FTSFlow(%q): erro %v", q, err)
		}
		ssEqual(t, got, nil)
	}
}

func TestStreamEntidadesRegras(t *testing.T) {
	w, r, ws := ssEnv(t)
	scope := upScope(t, w, ws, "ws-ent", "pj-ent")
	ctx := context.Background()
	cutoff := ssT0.Add(time.Hour)

	// Common tem freq 2 (peso 1/2 cada); Rare tem freq 1 (peso 1).
	ssPut(t, w, ssRef(t, scope, "ent/a.md"), "A", "corpo a", ssFM(t, "Common"), nil, ssT0)
	ssPut(t, w, ssRef(t, scope, "ent/b.md"), "B", "corpo b", ssFM(t, "Common"), nil, ssT0)
	ssPut(t, w, ssRef(t, scope, "ent/c.md"), "C", "corpo c", ssFM(t, "Rare"), nil, ssT0)
	ssPut(t, w, ssRef(t, scope, "ent/d.md"), "D", "corpo d", ssFM(t, "Pgbouncer"), nil, ssT0)
	ssPut(t, w, ssRef(t, scope, "ent/e.md"), "E", "corpo e", ssFM(t, "Kube-DNS"), nil, ssT0)
	ssPut(t, w, ssRef(t, scope, "ent/old.md"), "Old", "corpo old", ssFM(t, "Legacy"), nil, ssT0)
	ssPut(t, w, ssRef(t, scope, "ent/old.md"), "Old", "corpo old v2", ssFM(t, "Modern"), nil, ssT0.Add(time.Minute))

	cases := []struct {
		nome, q string
		limit   int
		want    []string
	}{
		{"token de 2 letras é ignorado", "pg", 10, nil},
		{"prefixo casa entidade", "pgb", 10, []string{"ent/d.md"}},
		{"nome exato casa", "pgbouncer", 10, []string{"ent/d.md"}},
		{"prefixo com freq 2 empata e cai no caminho", "com", 10, []string{"ent/a.md", "ent/b.md"}},
		{"peso 1/freq: freq 1 vence freq 2", "common rare", 10, []string{"ent/c.md", "ent/a.md", "ent/b.md"}},
		{"limite corta a saída", "common rare", 2, []string{"ent/c.md", "ent/a.md"}},
		{"parte após hífen casa", "dns", 10, []string{"ent/e.md"}},
		{"parte de termo composto com hífen casa", "ops-pgbouncer", 10, []string{"ent/d.md"}},
		{"versão superseded não casa", "legacy", 10, nil},
		{"versão corrente casa", "modern", 10, []string{"ent/old.md"}},
	}
	for _, c := range cases {
		got, err := EntityFlow(ctx, r, scope, c.q, cutoff, c.limit)
		if err != nil {
			t.Fatalf("%s: EntityFlow(%q): %v", c.nome, c.q, err)
		}
		if (len(got) != 0 || len(c.want) != 0) && !reflect.DeepEqual(ssPaths(got), c.want) {
			t.Errorf("%s: EntityFlow(%q) = %v, esperado %v", c.nome, c.q, ssPaths(got), c.want)
		}
	}
}

func TestStreamGrafoBidirecionalRegras(t *testing.T) {
	w, r, ws := ssEnv(t)
	scope := upScope(t, w, ws, "ws-gr", "pj-gr")
	ctx := context.Background()
	cutoff := ssT0.Add(time.Hour)
	t1 := ssT0.Add(time.Minute)
	t2 := ssT0.Add(2 * time.Minute)
	t3 := ssT0.Add(3 * time.Minute)
	t4 := ssT0.Add(4 * time.Minute)

	// Alvos antes das origens, para que os links resolvam na escrita.
	ssPut(t, w, ssRef(t, scope, "g/late.md"), "Late", "corpo late", ssFM(t), nil, t1)
	ssPut(t, w, ssRef(t, scope, "g/a.md"), "A", "corpo a", ssFM(t), nil, ssT0)
	ssPut(t, w, ssRef(t, scope, "g/z.md"), "Z", "corpo z", ssFM(t), nil, ssT0)

	link := func(p string, rel string) Link {
		return Link{ToPath: upPath(t, p), Relation: rel}
	}
	// seed: saída para z, a e late, e um link pendente para g/missing.md.
	ssPut(t, w, ssRef(t, scope, "g/seed.md"), "Seed", "corpo seed", ssFM(t), []Link{
		link("g/z.md", ""),
		link("g/a.md", "causes"),
		link("g/late.md", ""),
		link("g/missing.md", ""),
	}, t2)
	// in: entrada para seed.
	ssPut(t, w, ssRef(t, scope, "g/in.md"), "In", "corpo in", ssFM(t), []Link{link("g/seed.md", "fixes")}, t3)
	// pend: só link pendente, para página que não existe.
	ssPut(t, w, ssRef(t, scope, "g/pend.md"), "Pend", "corpo pend", ssFM(t), []Link{link("g/ghost.md", "")}, t4)

	seed := ssRef(t, scope, "g/seed.md")
	a := ssRef(t, scope, "g/a.md")
	pend := ssRef(t, scope, "g/pend.md")
	in := ssRef(t, scope, "g/in.md")

	// Saída antes de entrada; saída por updated_at desc (late), depois caminho asc (a, z).
	got, err := GraphFlow(ctx, r, scope, []domain.PageRef{seed}, cutoff, 10)
	if err != nil {
		t.Fatalf("GraphFlow seed: %v", err)
	}
	ssEqual(t, got, []string{"g/late.md", "g/a.md", "g/z.md", "g/in.md"})

	// Semente não aparece como vizinho: a é semente e some da saída.
	got, err = GraphFlow(ctx, r, scope, []domain.PageRef{seed, a}, cutoff, 10)
	if err != nil {
		t.Fatalf("GraphFlow seed+a: %v", err)
	}
	ssEqual(t, got, []string{"g/late.md", "g/z.md", "g/in.md"})

	// Entrada: in aponta para seed, então seed é vizinho de saída de in.
	got, err = GraphFlow(ctx, r, scope, []domain.PageRef{in}, cutoff, 10)
	if err != nil {
		t.Fatalf("GraphFlow in: %v", err)
	}
	ssEqual(t, got, []string{"g/seed.md"})

	// Link pendente não é percorrido.
	got, err = GraphFlow(ctx, r, scope, []domain.PageRef{pend}, cutoff, 10)
	if err != nil {
		t.Fatalf("GraphFlow pend: %v", err)
	}
	ssEqual(t, got, nil)

	// Sementes repetidas e vazias: sem repetir a expansão e sem I/O.
	got, err = GraphFlow(ctx, r, scope, []domain.PageRef{seed, seed}, cutoff, 10)
	if err != nil {
		t.Fatalf("GraphFlow repetida: %v", err)
	}
	ssEqual(t, got, []string{"g/late.md", "g/a.md", "g/z.md", "g/in.md"})
	got, err = GraphFlow(ctx, r, scope, nil, cutoff, 10)
	if err != nil {
		t.Fatalf("GraphFlow vazia: %v", err)
	}
	ssEqual(t, got, nil)

	// Limite corta a saída e a ordem é mantida.
	got, err = GraphFlow(ctx, r, scope, []domain.PageRef{seed}, cutoff, 2)
	if err != nil {
		t.Fatalf("GraphFlow limite: %v", err)
	}
	ssEqual(t, got, []string{"g/late.md", "g/a.md"})

	// Repetição da mesma consulta devolve a mesma ordem (determinismo).
	again, err := GraphFlow(ctx, r, scope, []domain.PageRef{seed}, cutoff, 10)
	if err != nil {
		t.Fatalf("GraphFlow repetição: %v", err)
	}
	ssEqual(t, again, []string{"g/late.md", "g/a.md", "g/z.md", "g/in.md"})
}

func TestStreamSeedsFromFlowsEntrelacaPorPosicao(t *testing.T) {
	w, _, ws := ssEnv(t)
	scope := upScope(t, w, ws, "ws-sd", "pj-sd")
	a := ssRef(t, scope, "s/a.md")
	b := ssRef(t, scope, "s/b.md")
	c := ssRef(t, scope, "s/c.md")

	got := SeedsFromFlows([]domain.PageRef{a, b}, []domain.PageRef{c, a})
	want := []domain.PageRef{a, c, b}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SeedsFromFlows = %v, esperado %v", ssPaths(got), ssPaths(want))
	}
	if got := SeedsFromFlows(); len(got) != 0 {
		t.Fatalf("SeedsFromFlows() = %v, esperado vazio", got)
	}
}
