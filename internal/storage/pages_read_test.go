package storage

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// prEnv abre o banco real em um diretório temporário, com escritor e pool somente leitura.
// Devolve o pool de escrita (para contar linhas e preparar casos), o Writer e o Reader.
func prEnv(t *testing.T) (*sql.DB, Writer, Reader, domain.WorkspaceID) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	write, err := Open(ctx, dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = write.Close() })
	w, err := NewWriter(ctx, write)
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
	return write, w, NewReader(rdb), domain.WorkspaceID(wsID)
}

// prSnap é a contagem de linhas e somas que uma escrita alteraria.
type prSnap struct {
	pages, latest, accessCount, audit, evidence int
}

func prTake(t *testing.T, db *sql.DB) prSnap {
	t.Helper()
	var s prSnap
	err := db.QueryRow(
		`SELECT (SELECT COUNT(*) FROM pages),
		        (SELECT COALESCE(SUM(is_latest), 0) FROM pages),
		        (SELECT COALESCE(SUM(access_count), 0) FROM pages),
		        (SELECT COUNT(*) FROM audit_log),
		        (SELECT COUNT(*) FROM page_evidence)`,
	).Scan(&s.pages, &s.latest, &s.accessCount, &s.audit, &s.evidence)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	return s
}

// prEntities lê a lista entities do frontmatter devolvido.
func prEntities(t *testing.T, p domain.Page) []string {
	t.Helper()
	v, ok := p.Frontmatter.Get("entities")
	if !ok {
		t.Fatalf("frontmatter sem entities: %v", p.Frontmatter.Keys())
	}
	list, ok := v.([]string)
	if !ok {
		t.Fatalf("entities com tipo %T, esperado []string", v)
	}
	return list
}

// prMustRead chama ReadPage e falha o teste em caso de erro.
func prMustRead(t *testing.T, r Reader, ref domain.PageRef) domain.Page {
	t.Helper()
	p, err := ReadPage(context.Background(), r, ref)
	if err != nil {
		t.Fatalf("ReadPage(%s): %v", ref.String(), err)
	}
	return p
}

func TestPagesReadReadPageDevolveVersaoCorrenteAposSupersessao(t *testing.T) {
	_, w, r, ws := prEnv(t)
	scope := upScope(t, w, ws, "ws-r", "pj-r")
	ref := domain.NewPageRef(scope, upPath(t, "notas/rota.md"))

	r1 := upMustUpsert(t, w, upIn(t, ref, "corpo v1", "sess-1", upT0))
	r2 := upMustUpsert(t, w, upIn(t, ref, "corpo v2", "sess-2", upT0.Add(time.Minute)))
	if r2.Outcome != UpsertSuperseded {
		t.Fatalf("segunda escrita: outcome %q, esperado %q", r2.Outcome, UpsertSuperseded)
	}
	if r1.Version == r2.Version {
		t.Fatal("versões iguais após supersessão")
	}

	p := prMustRead(t, r, ref)
	if p.Version != r2.Version {
		t.Errorf("Version = %s, esperada a corrente %s", p.Version.String(), r2.Version.String())
	}
	if p.Body != "corpo v2" {
		t.Errorf("Body = %q, esperado %q", p.Body, "corpo v2")
	}
	if p.Title != "Rota" || p.Tier != domain.TierSemantic {
		t.Errorf("Title/Tier = %q/%q, esperados Rota/semantic", p.Title, p.Tier)
	}
	if p.Kind != domain.PageKindNote {
		t.Errorf("Kind = %q, esperado %q (chave kind do frontmatter)", p.Kind, domain.PageKindNote)
	}
	if _, ok := p.Frontmatter.Get(kindKey); ok {
		t.Error("Frontmatter repete a chave kind; devia ficar só em Kind")
	}
	if ents := prEntities(t, p); len(ents) != 1 || ents[0] != "Postgres" {
		t.Errorf("entities = %v, esperado [Postgres]", ents)
	}
	if p.Ref.String() != ref.String() {
		t.Errorf("Ref = %s, esperado %s", p.Ref.String(), ref.String())
	}
}

func TestPagesReadReadPageSemVersaoCorrenteDevolveErrPageNotFound(t *testing.T) {
	db, w, r, ws := prEnv(t)
	scope := upScope(t, w, ws, "ws-nf", "pj-nf")
	ctx := context.Background()

	// Caminho que nunca existiu.
	ref := domain.NewPageRef(scope, upPath(t, "notas/inexistente.md"))
	if _, err := ReadPage(ctx, r, ref); !errors.Is(err, ErrPageNotFound) {
		t.Fatalf("caminho inexistente: erro %v, esperado ErrPageNotFound", err)
	}

	// Caminho com lápide de decaimento (is_latest = 0, superseded_at preenchido): sem corrente.
	morta := domain.NewPageRef(scope, upPath(t, "notas/morta.md"))
	upMustUpsert(t, w, upIn(t, morta, "vai morrer", "sess-m", upT0))
	if _, err := db.Exec(
		`UPDATE pages SET is_latest = 0, superseded_at = ? WHERE path = ?`,
		upT0.Add(time.Hour).UnixMicro(), "notas/morta.md",
	); err != nil {
		t.Fatalf("simular lápide: %v", err)
	}
	if _, err := ReadPage(ctx, r, morta); !errors.Is(err, ErrPageNotFound) {
		t.Fatalf("lápide: erro %v, esperado ErrPageNotFound", err)
	}

	// Mesmo caminho em outro projeto não é o caminho exato do escopo.
	outro := upScope(t, w, ws, "ws-nf", "pj-outro")
	upMustUpsert(t, w, upIn(t, domain.NewPageRef(outro, upPath(t, "notas/so-outro.md")), "x", "sess-o", upT0))
	if _, err := ReadPage(ctx, r, domain.NewPageRef(scope, upPath(t, "notas/so-outro.md"))); !errors.Is(err, ErrPageNotFound) {
		t.Fatalf("outro projeto: erro %v, esperado ErrPageNotFound", err)
	}
}

func TestPagesReadRecentRespeitaLimiteEOrdem(t *testing.T) {
	_, w, r, ws := prEnv(t)
	scope := upScope(t, w, ws, "ws-rec", "pj-rec")
	ctx := context.Background()

	pa := domain.NewPageRef(scope, upPath(t, "a.md"))
	pb := domain.NewPageRef(scope, upPath(t, "b.md"))
	pc := domain.NewPageRef(scope, upPath(t, "c.md"))
	pd := domain.NewPageRef(scope, upPath(t, "d.md"))
	pe := domain.NewPageRef(scope, upPath(t, "e.md"))

	upMustUpsert(t, w, upIn(t, pa, "a", "s-a", upT0))
	upMustUpsert(t, w, upIn(t, pb, "b", "s-b", upT0.Add(time.Hour)))
	upMustUpsert(t, w, upIn(t, pc, "c", "s-c", upT0.Add(2*time.Hour)))
	// b é reescrita em +3h: a versão corrente de b é a mais nova do escopo.
	upMustUpsert(t, w, upIn(t, pb, "b v2", "s-b2", upT0.Add(3*time.Hour)))
	// d e e no mesmo instante (+4h): desempata por rowid DESC, então e (inserida depois) vem antes.
	upMustUpsert(t, w, upIn(t, pd, "d", "s-d", upT0.Add(4*time.Hour)))
	upMustUpsert(t, w, upIn(t, pe, "e", "s-e", upT0.Add(4*time.Hour)))

	ordem := []string{"e.md", "d.md", "b.md", "c.md", "a.md"}

	todas, err := Recent(ctx, r, scope, 10)
	if err != nil {
		t.Fatalf("Recent(10): %v", err)
	}
	if len(todas) != len(ordem) {
		t.Fatalf("Recent(10) = %d páginas, esperadas %d (só correntes, uma por caminho)", len(todas), len(ordem))
	}
	for i, p := range todas {
		if got := p.Ref.Path.String(); got != ordem[i] {
			t.Errorf("Recent(10)[%d] = %s, esperado %s", i, got, ordem[i])
		}
	}
	if todas[2].Body != "b v2" {
		t.Errorf("b devolvida com corpo %q, esperado a corrente %q", todas[2].Body, "b v2")
	}

	dois, err := Recent(ctx, r, scope, 2)
	if err != nil {
		t.Fatalf("Recent(2): %v", err)
	}
	if len(dois) != 2 || dois[0].Ref.Path.String() != "e.md" || dois[1].Ref.Path.String() != "d.md" {
		t.Errorf("Recent(2) = %v, esperado [e.md d.md]", caminhos(dois))
	}

	for _, lim := range []int{0, -1} {
		vazio, err := Recent(ctx, r, scope, lim)
		if err != nil {
			t.Fatalf("Recent(%d): erro %v, esperado nil", lim, err)
		}
		if len(vazio) != 0 {
			t.Errorf("Recent(%d) = %d páginas, esperado vazio", lim, len(vazio))
		}
	}
}

// caminhos devolve os caminhos das páginas, para mensagens de erro.
func caminhos(ps []domain.Page) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.Ref.Path.String())
	}
	return out
}

func TestPagesReadPageChainIncluiVersaoPerdedoraNaOrdemNovaParaAntiga(t *testing.T) {
	_, w, r, ws := prEnv(t)
	scope := upScope(t, w, ws, "ws-ch", "pj-ch")
	ctx := context.Background()
	ref := domain.NewPageRef(scope, upPath(t, "notas/cadeia.md"))

	r1 := upMustUpsert(t, w, upIn(t, ref, "v1", "sess-1", upT0))
	r2 := upMustUpsert(t, w, upIn(t, ref, "v2", "sess-2", upT0.Add(time.Minute)))
	r3 := upMustUpsert(t, w, upIn(t, ref, "v3", "sess-3", upT0.Add(2*time.Minute)))

	chain, err := PageChain(ctx, r, ref)
	if err != nil {
		t.Fatalf("PageChain: %v", err)
	}
	if len(chain) != 3 {
		t.Fatalf("PageChain = %d versões, esperadas 3 (inclui as perdedoras)", len(chain))
	}

	ids := []domain.PageID{r3.Version, r2.Version, r1.Version}
	for i, v := range chain {
		if v.ID != ids[i] {
			t.Errorf("chain[%d].ID = %s, esperado %s", i, v.ID.String(), ids[i].String())
		}
	}

	if !chain[0].IsLatest || chain[1].IsLatest || chain[2].IsLatest {
		t.Errorf("IsLatest = [%v %v %v], esperado [true false false]", chain[0].IsLatest, chain[1].IsLatest, chain[2].IsLatest)
	}
	if chain[0].Supersedes == nil || *chain[0].Supersedes != r2.Version {
		t.Errorf("chain[0].Supersedes errado, esperado %s", r2.Version.String())
	}
	if chain[1].Supersedes == nil || *chain[1].Supersedes != r1.Version {
		t.Errorf("chain[1].Supersedes errado, esperado %s", r1.Version.String())
	}
	if chain[2].Supersedes != nil {
		t.Errorf("primeira versão com Supersedes = %s, esperado nil", chain[2].Supersedes.String())
	}

	// Janelas: a perdedora fecha em valid_to = instante da supersessão; a corrente fica aberta.
	if got, want := chain[2].ValidFrom, upT0.UnixMicro(); got != want {
		t.Errorf("v1 ValidFrom = %d, esperado %d", got, want)
	}
	if got, want := chain[2].ValidTo, upT0.Add(time.Minute).UnixMicro(); got != want {
		t.Errorf("v1 ValidTo = %d, esperado %d", got, want)
	}
	if got, want := chain[1].ValidTo, upT0.Add(2*time.Minute).UnixMicro(); got != want {
		t.Errorf("v2 ValidTo = %d, esperado %d", got, want)
	}
	if chain[0].ValidTo != 0 {
		t.Errorf("corrente ValidTo = %d, esperado 0 (aberta)", chain[0].ValidTo)
	}
}

func TestPagesReadPageChainCaminhoInexistenteDevolveVazio(t *testing.T) {
	_, w, r, ws := prEnv(t)
	scope := upScope(t, w, ws, "ws-vz", "pj-vz")

	chain, err := PageChain(context.Background(), r, domain.NewPageRef(scope, upPath(t, "nada/aqui.md")))
	if err != nil {
		t.Fatalf("PageChain caminho inexistente: erro %v, esperado nil", err)
	}
	if len(chain) != 0 {
		t.Errorf("PageChain caminho inexistente = %d versões, esperado vazio", len(chain))
	}
}

func TestPagesReadNaoFiltraPorAutoria(t *testing.T) {
	_, w, r, ws := prEnv(t)
	scope := upScope(t, w, ws, "ws-au", "pj-au")
	ctx := context.Background()
	ref := domain.NewPageRef(scope, upPath(t, "notas/compartilhada.md"))

	// Duas sessões (autores de evidência) escrevem conteúdo divergente na mesma página.
	rA := upMustUpsert(t, w, upIn(t, ref, "versao de A", "sess-A", upT0))
	rB := upMustUpsert(t, w, upIn(t, ref, "versao de B", "sess-B", upT0.Add(time.Minute)))
	if rB.Outcome != UpsertSuperseded {
		t.Fatalf("escrita de B: outcome %q, esperado %q", rB.Outcome, UpsertSuperseded)
	}

	// A leitura da corrente não depende de quem escreveu.
	p := prMustRead(t, r, ref)
	if p.Body != "versao de B" || p.Version != rB.Version {
		t.Errorf("ReadPage = %q (%s), esperado a corrente de B", p.Body, p.Version.String())
	}

	// A versão de A (perdedora) continua alcançável pela cadeia.
	chain, err := PageChain(ctx, r, ref)
	if err != nil {
		t.Fatalf("PageChain: %v", err)
	}
	if len(chain) != 2 || chain[1].ID != rA.Version {
		t.Fatalf("PageChain = %d versões, esperadas 2 com a de A por último", len(chain))
	}

	// Recent também vê a corrente sem filtro de autor.
	rec, err := Recent(ctx, r, scope, 5)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(rec) != 1 || rec[0].Version != rB.Version {
		t.Errorf("Recent = %v, esperada só a corrente de B", caminhos(rec))
	}
}

func TestPagesReadNaoEscreve(t *testing.T) {
	db, w, r, ws := prEnv(t)
	scope := upScope(t, w, ws, "ws-ro", "pj-ro")
	ctx := context.Background()
	ref := domain.NewPageRef(scope, upPath(t, "notas/somente-leitura.md"))
	upMustUpsert(t, w, upIn(t, ref, "v1", "sess-1", upT0))
	upMustUpsert(t, w, upIn(t, ref, "v2", "sess-2", upT0.Add(time.Minute)))
	ausente := domain.NewPageRef(scope, upPath(t, "nada.md"))

	antes := prTake(t, db)

	if _, err := ReadPage(ctx, r, ref); err != nil {
		t.Fatalf("ReadPage: %v", err)
	}
	if _, err := ReadPage(ctx, r, ausente); !errors.Is(err, ErrPageNotFound) {
		t.Fatalf("ReadPage ausente: erro %v", err)
	}
	if _, err := Recent(ctx, r, scope, 10); err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if _, err := PageChain(ctx, r, ref); err != nil {
		t.Fatalf("PageChain: %v", err)
	}
	if _, err := PageChain(ctx, r, ausente); err != nil {
		t.Fatalf("PageChain ausente: %v", err)
	}

	depois := prTake(t, db)
	if antes != depois {
		t.Errorf("leitura alterou o banco: antes %+v, depois %+v", antes, depois)
	}
}
