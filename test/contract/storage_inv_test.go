// Testes de contrato de storage para as invariantes INV-02, INV-03 e INV-04 da
// constituição. Exercitam o contrato pela superfície pública de internal/storage
// e internal/domain, sem helpers de outros pacotes de teste. Todo arquivo
// temporário fica em t.TempDir().
package contract

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
	"github.com/phm-aguiar/ai-memory-go/internal/storage"
)

// sinvT0 é o instante fixo das escritas do arquivo.
var sinvT0 = time.Date(2026, 10, 12, 15, 30, 0, 0, time.UTC)

// sinvTimeout limita qualquer espera de goroutine do arquivo.
const sinvTimeout = 10 * time.Second

// sinvSegura é quanto tempo a transação de teste segura o escritor.
const sinvSegura = 200 * time.Millisecond

// sinvLimiteLeitura é o teto de tempo de uma leitura enquanto há escrita aberta.
const sinvLimiteLeitura = 150 * time.Millisecond

// errSinvFalhaSimulada força o rollback de uma transação que já escreveu.
var errSinvFalhaSimulada = errors.New("falha simulada depois da inserção")

// sinvFailWriter é um Writer de teste: executa a closure e depois devolve erro,
// de modo que o escritor real faz ROLLBACK de tudo o que a closure gravou.
type sinvFailWriter struct {
	inner storage.Writer
}

var _ storage.Writer = sinvFailWriter{}

// Do executa fn dentro da transação real e devolve errSinvFalhaSimulada.
func (f sinvFailWriter) Do(ctx context.Context, fn func(*sql.Tx) error) error {
	return f.inner.Do(ctx, func(tx *sql.Tx) error {
		if err := fn(tx); err != nil {
			return err
		}
		return errSinvFalhaSimulada
	})
}

// QueueLen delega ao escritor real.
func (f sinvFailWriter) QueueLen() int { return f.inner.QueueLen() }

// Close delega ao escritor real.
func (f sinvFailWriter) Close() error { return f.inner.Close() }

// sinvQuerier é o subconjunto de *sql.DB usado para contar linhas.
type sinvQuerier interface {
	QueryRow(query string, args ...any) *sql.Row
}

// sinvEnv abre um banco temporário com a migração real e um escritor. O
// escritor é fechado antes do *sql.DB (ordem LIFO dos Cleanup).
func sinvEnv(t *testing.T) (string, *sql.DB, storage.Writer) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	db, err := storage.Open(ctx, dir)
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	w, err := storage.NewWriter(ctx, db)
	if err != nil {
		t.Fatalf("storage.NewWriter: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return dir, db, w
}

// sinvReader abre o pool somente leitura do banco de dir.
func sinvReader(t *testing.T, dir string) (*sql.DB, storage.Reader) {
	t.Helper()
	rdb, err := storage.OpenReader(context.Background(), dir)
	if err != nil {
		t.Fatalf("storage.OpenReader: %v", err)
	}
	t.Cleanup(func() { _ = rdb.Close() })
	return rdb, storage.NewReader(rdb)
}

// sinvNewID gera um identificador novo ou falha o teste.
func sinvNewID(t *testing.T) domain.ID {
	t.Helper()
	id, err := domain.NewID()
	if err != nil {
		t.Fatalf("domain.NewID: %v", err)
	}
	return id
}

// sinvNewWS gera um workspace novo (ainda não criado no banco).
func sinvNewWS(t *testing.T) domain.WorkspaceID {
	t.Helper()
	return domain.WorkspaceID(sinvNewID(t))
}

// sinvScope cria um projeto novo no workspace ws, com EnsureScope.
func sinvScope(t *testing.T, w storage.Writer, ws domain.WorkspaceID, wsName, pjName string) domain.Scope {
	t.Helper()
	scope := domain.NewScope(ws, domain.ProjectID(sinvNewID(t)))
	if err := storage.EnsureScope(context.Background(), w, scope, wsName, pjName, sinvT0); err != nil {
		t.Fatalf("EnsureScope(%s/%s): %v", wsName, pjName, err)
	}
	return scope
}

// sinvPath valida um caminho de página para o teste.
func sinvPath(t *testing.T, s string) domain.PagePath {
	t.Helper()
	p, err := domain.ParsePagePath(s)
	if err != nil {
		t.Fatalf("domain.ParsePagePath(%q): %v", s, err)
	}
	return p
}

// sinvInput monta uma página com entidades (no frontmatter) e links de saída.
func sinvInput(t *testing.T, ref domain.PageRef, body, sourceID string, entities []string, links []storage.Link) storage.UpsertInput {
	t.Helper()
	var fm domain.Frontmatter
	if len(entities) > 0 {
		if err := fm.Set("entities", entities); err != nil {
			t.Fatalf("frontmatter entities: %v", err)
		}
	}
	return storage.UpsertInput{
		Ref:         ref,
		Tier:        domain.TierSemantic,
		Kind:        domain.PageKindNote,
		Title:       "Nota de contrato",
		Body:        body,
		Frontmatter: fm,
		Links:       links,
		Evidence:    storage.EvidenceSource{Kind: storage.SourceSession, ID: sourceID},
		At:          sinvT0,
	}
}

// sinvUpsert chama UpsertPage e falha o teste em erro.
func sinvUpsert(t *testing.T, w storage.Writer, in storage.UpsertInput) storage.UpsertResult {
	t.Helper()
	res, err := storage.UpsertPage(context.Background(), w, in)
	if err != nil {
		t.Fatalf("UpsertPage(%s): %v", in.Ref.String(), err)
	}
	return res
}

// sinvCount executa uma consulta COUNT(*) e falha o teste em erro.
func sinvCount(t *testing.T, q sinvQuerier, query string, args ...any) int {
	t.Helper()
	var n int
	if err := q.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("contar (%s): %v", query, err)
	}
	return n
}

// sinvEscopoBytes devolve os ids de workspace e projeto do escopo como bytes.
func sinvEscopoBytes(scope domain.Scope) (ws, pj []byte) {
	return scope.WorkspaceID.Bytes(), scope.ProjectID.Bytes()
}

// TestINV02_EscritorSerializaConcorrentes: duas escritas concorrentes passam pelo
// escritor único e ambas terminam sem erro (sem SQLITE_BUSY). Cobre caminhos
// diferentes e o mesmo caminho (criação e supersessão em sequência).
func TestINV02_EscritorSerializaConcorrentes(t *testing.T) {
	_, db, w := sinvEnv(t)
	ws := sinvNewWS(t)
	scope := sinvScope(t, w, ws, "ws-inv02a", "pj-inv02a")
	wsB, pjB := sinvEscopoBytes(scope)

	ctx, cancel := context.WithTimeout(context.Background(), sinvTimeout)
	defer cancel()

	// rodar dispara duas escritas a partir de um canal de largada comum.
	rodar := func(ins []storage.UpsertInput) []error {
		errs := make([]error, len(ins))
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := range ins {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, errs[i] = storage.UpsertPage(ctx, w, ins[i])
			}()
		}
		close(start)
		wg.Wait()
		return errs
	}

	// Caminhos diferentes: duas criações concorrentes.
	refA := domain.NewPageRef(scope, sinvPath(t, "notas/concorrente-a.md"))
	refB := domain.NewPageRef(scope, sinvPath(t, "notas/concorrente-b.md"))
	insA := []storage.UpsertInput{
		sinvInput(t, refA, "corpo a", "sess-a", nil, nil),
		sinvInput(t, refB, "corpo b", "sess-b", nil, nil),
	}
	for i, err := range rodar(insA) {
		if err != nil {
			t.Fatalf("escrita concorrente %d (caminhos distintos): %v", i, err)
		}
	}
	for _, ref := range []domain.PageRef{refA, refB} {
		if n := sinvCount(t, db, `SELECT COUNT(*) FROM pages WHERE workspace_id = ? AND project_id = ? AND path = ? AND is_latest = 1`,
			wsB, pjB, ref.Path.String()); n != 1 {
			t.Fatalf("caminho %s: %d versões correntes, esperado 1", ref.Path.String(), n)
		}
	}

	// Mesmo caminho e conteúdos diferentes: uma criação e uma supersessão, em série.
	refC := domain.NewPageRef(scope, sinvPath(t, "notas/concorrente-c.md"))
	insC := []storage.UpsertInput{
		sinvInput(t, refC, "versão 1", "sess-c1", nil, nil),
		sinvInput(t, refC, "versão 2", "sess-c2", nil, nil),
	}
	for i, err := range rodar(insC) {
		if err != nil {
			t.Fatalf("escrita concorrente %d (mesmo caminho): %v", i, err)
		}
	}
	if n := sinvCount(t, db, `SELECT COUNT(*) FROM pages WHERE workspace_id = ? AND project_id = ? AND path = ?`,
		wsB, pjB, refC.Path.String()); n != 2 {
		t.Fatalf("mesmo caminho: %d versões gravadas, esperado 2", n)
	}
	if n := sinvCount(t, db, `SELECT COUNT(*) FROM pages WHERE workspace_id = ? AND project_id = ? AND path = ? AND is_latest = 1`,
		wsB, pjB, refC.Path.String()); n != 1 {
		t.Fatalf("mesmo caminho: %d versões correntes, esperado 1", n)
	}
}

// TestINV02_LeituraNaoEsperaTransacaoDeEscrita: com uma transação do escritor
// aberta (segura ~200 ms), uma leitura pelo pool somente leitura termina em menos
// de 150 ms e não enxerga a linha ainda não commitada.
func TestINV02_LeituraNaoEsperaTransacaoDeEscrita(t *testing.T) {
	dir, _, w := sinvEnv(t)
	ws := sinvNewWS(t)
	scope := sinvScope(t, w, ws, "ws-inv02b", "pj-inv02b")
	ref := domain.NewPageRef(scope, sinvPath(t, "notas/segurada.md"))
	in := sinvInput(t, ref, "corpo segurado", "sess-b", nil, nil)
	_, rd := sinvReader(t, dir)

	ctx, cancel := context.WithTimeout(context.Background(), sinvTimeout)
	defer cancel()

	inside := make(chan struct{})
	var wg sync.WaitGroup
	var writeErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		writeErr = w.Do(ctx, func(tx *sql.Tx) error {
			if _, err := storage.UpsertPageTx(tx, in); err != nil {
				return err
			}
			close(inside)
			time.Sleep(sinvSegura)
			return nil
		})
	}()

	select {
	case <-inside:
	case <-ctx.Done():
		wg.Wait()
		t.Fatalf("transação de escrita não entrou a tempo: %v", ctx.Err())
	}

	var visiveis int
	inicio := time.Now()
	errLeitura := rd.Read(ctx, func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM pages WHERE path = ?`, ref.Path.String()).Scan(&visiveis)
	})
	decorrido := time.Since(inicio)
	wg.Wait()

	if errLeitura != nil {
		t.Fatalf("leitura durante escrita aberta: %v", errLeitura)
	}
	if decorrido >= sinvLimiteLeitura {
		t.Fatalf("leitura levou %v com escrita aberta, esperado < %v", decorrido, sinvLimiteLeitura)
	}
	if visiveis != 0 {
		t.Fatalf("leitura viu %d linha(s) não commitada(s), esperado 0", visiveis)
	}
	if writeErr != nil {
		t.Fatalf("transação de escrita: %v", writeErr)
	}

	var depois int
	if err := rd.Read(ctx, func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM pages WHERE path = ?`, ref.Path.String()).Scan(&depois)
	}); err != nil {
		t.Fatalf("leitura após commit: %v", err)
	}
	if depois != 1 {
		t.Fatalf("após commit a leitura vê %d linha(s), esperado 1", depois)
	}
}

// TestINV02_PoolDeLeituraRecusaEscrita: o pool somente leitura recusa INSERT,
// tanto dentro de Reader.Read quanto direto no *sql.DB, e nada é gravado.
func TestINV02_PoolDeLeituraRecusaEscrita(t *testing.T) {
	dir, _, w := sinvEnv(t)
	_ = sinvScope(t, w, sinvNewWS(t), "ws-inv02c", "pj-inv02c")
	rdb, rd := sinvReader(t, dir)

	ctx, cancel := context.WithTimeout(context.Background(), sinvTimeout)
	defer cancel()

	const nome = "ws-escrita-proibida"
	id := sinvNewID(t)

	errRead := rd.Read(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO workspaces (id, name, created_at) VALUES (?, ?, ?)`,
			id.Bytes(), nome, sinvT0.UnixMicro())
		return err
	})
	if errRead == nil {
		t.Fatal("INSERT dentro de Reader.Read foi aceito, esperado recusa (somente leitura)")
	}

	_, errDB := rdb.Exec(`INSERT INTO workspaces (id, name, created_at) VALUES (?, ?, ?)`,
		sinvNewID(t).Bytes(), nome+"-direto", sinvT0.UnixMicro())
	if errDB == nil {
		t.Fatal("INSERT direto no pool de leitura foi aceito, esperado recusa (query_only)")
	}

	if n := sinvCount(t, rdb, `SELECT COUNT(*) FROM workspaces WHERE name IN (?, ?)`, nome, nome+"-direto"); n != 0 {
		t.Fatalf("pool de leitura gravou %d linha(s), esperado 0", n)
	}
}

// TestINV03_IndicesMesmaTransacaoAposRetorno: ao retornar sucesso, pages_fts,
// entities, entity_page_links e links já estão visíveis, tanto pelo escritor
// quanto por um pool somente leitura aberto em outra conexão.
func TestINV03_IndicesMesmaTransacaoAposRetorno(t *testing.T) {
	dir, db, w := sinvEnv(t)
	ws := sinvNewWS(t)
	scope := sinvScope(t, w, ws, "ws-inv03a", "pj-inv03a")
	wsB, pjB := sinvEscopoBytes(scope)
	ref := domain.NewPageRef(scope, sinvPath(t, "notas/indice.md"))
	const termo = "xylofonemagico"

	in := sinvInput(t, ref, "corpo com termo "+termo, "sess-3a",
		[]string{"Postgres", "Go"},
		[]storage.Link{{ToPath: sinvPath(t, "notas/alvo.md")}})
	res := sinvUpsert(t, w, in)
	pageID := res.Version.Bytes()

	rdb, _ := sinvReader(t, dir)

	for _, q := range []struct {
		nome string
		db   sinvQuerier
	}{{"escritor", db}, {"leitor", rdb}} {
		if n := sinvCount(t, q.db, `SELECT COUNT(*) FROM pages WHERE id = ?`, pageID); n != 1 {
			t.Fatalf("%s: pages = %d, esperado 1", q.nome, n)
		}
		if n := sinvCount(t, q.db,
			`SELECT COUNT(*) FROM pages_fts JOIN pages ON pages.rowid = pages_fts.rowid
			 WHERE pages_fts MATCH ? AND pages.id = ?`, termo, pageID); n != 1 {
			t.Fatalf("%s: pages_fts da versão = %d, esperado 1", q.nome, n)
		}
		if n := sinvCount(t, q.db,
			`SELECT COUNT(*) FROM entities WHERE workspace_id = ? AND project_id = ?`, wsB, pjB); n != 2 {
			t.Fatalf("%s: entities = %d, esperado 2", q.nome, n)
		}
		if n := sinvCount(t, q.db, `SELECT COUNT(*) FROM entity_page_links WHERE page_id = ?`, pageID); n != 2 {
			t.Fatalf("%s: entity_page_links = %d, esperado 2", q.nome, n)
		}
		if n := sinvCount(t, q.db, `SELECT COUNT(*) FROM links WHERE from_page_id = ?`, pageID); n != 1 {
			t.Fatalf("%s: links = %d, esperado 1", q.nome, n)
		}
	}
}

// TestINV03_FalhaDesfazIndicesEPagina: quando a transação falha depois de
// inserir a página, nenhuma linha de pages, pages_fts, entity_page_links ou links
// (nem entities) permanece no banco, e uma escrita posterior do mesmo caminho
// cria a página normalmente.
func TestINV03_FalhaDesfazIndicesEPagina(t *testing.T) {
	_, db, w := sinvEnv(t)
	ws := sinvNewWS(t)
	scope := sinvScope(t, w, ws, "ws-inv03b", "pj-inv03b")
	wsB, pjB := sinvEscopoBytes(scope)
	ref := domain.NewPageRef(scope, sinvPath(t, "notas/falha.md"))
	const termo = "zeppelinfalho"

	in := sinvInput(t, ref, "corpo com termo "+termo, "sess-3b",
		[]string{"Postgres", "Go"},
		[]storage.Link{{ToPath: sinvPath(t, "notas/alvo-falha.md")}})

	_, err := storage.UpsertPage(context.Background(), sinvFailWriter{inner: w}, in)
	if !errors.Is(err, errSinvFalhaSimulada) {
		t.Fatalf("UpsertPage com falha simulada: erro = %v, esperado errSinvFalhaSimulada", err)
	}

	zeros := []struct {
		nome  string
		query string
		args  []any
	}{
		{"pages", `SELECT COUNT(*) FROM pages WHERE workspace_id = ? AND project_id = ? AND path = ?`,
			[]any{wsB, pjB, ref.Path.String()}},
		{"pages_fts", `SELECT COUNT(*) FROM pages_fts WHERE pages_fts MATCH ?`, []any{termo}},
		{"entity_page_links", `SELECT COUNT(*) FROM entity_page_links`, nil},
		{"links", `SELECT COUNT(*) FROM links`, nil},
		{"entities", `SELECT COUNT(*) FROM entities`, nil},
	}
	for _, z := range zeros {
		if n := sinvCount(t, db, z.query, z.args...); n != 0 {
			t.Fatalf("após falha, %s tem %d linha(s), esperado 0", z.nome, n)
		}
	}

	res := sinvUpsert(t, w, in)
	if res.Outcome != storage.UpsertCreated {
		t.Fatalf("escrita posterior: outcome = %q, esperado %q", res.Outcome, storage.UpsertCreated)
	}
	if n := sinvCount(t, db, `SELECT COUNT(*) FROM pages_fts WHERE pages_fts MATCH ?`, termo); n != 1 {
		t.Fatalf("escrita posterior: pages_fts = %d, esperado 1", n)
	}
}

// TestINV04_MesmoPathProjetosDiferentesCoexistem: o mesmo path em dois projetos do
// mesmo workspace gera duas versões correntes, uma por projeto.
func TestINV04_MesmoPathProjetosDiferentesCoexistem(t *testing.T) {
	_, db, w := sinvEnv(t)
	ws := sinvNewWS(t)
	sA := sinvScope(t, w, ws, "ws-inv04a", "pj-a")
	sB := sinvScope(t, w, ws, "ws-inv04a", "pj-b")
	p := sinvPath(t, "notas/comum.md")

	rA := sinvUpsert(t, w, sinvInput(t, domain.NewPageRef(sA, p), "corpo do projeto A", "sess-a", nil, nil))
	rB := sinvUpsert(t, w, sinvInput(t, domain.NewPageRef(sB, p), "corpo do projeto B", "sess-b", nil, nil))

	if rA.Outcome != storage.UpsertCreated || rB.Outcome != storage.UpsertCreated {
		t.Fatalf("outcomes = %q/%q, esperado created/created", rA.Outcome, rB.Outcome)
	}
	if rA.Version == rB.Version {
		t.Fatal("os dois projetos devolveram o mesmo id de versão")
	}
	wsB := sA.WorkspaceID.Bytes()
	if n := sinvCount(t, db, `SELECT COUNT(*) FROM pages WHERE workspace_id = ? AND path = ? AND is_latest = 1`,
		wsB, p.String()); n != 2 {
		t.Fatalf("versões correntes do path no workspace = %d, esperado 2", n)
	}
	for _, s := range []domain.Scope{sA, sB} {
		_, pjB := sinvEscopoBytes(s)
		if n := sinvCount(t, db, `SELECT COUNT(*) FROM pages WHERE project_id = ? AND path = ? AND is_latest = 1`,
			pjB, p.String()); n != 1 {
			t.Fatalf("projeto %s: versões correntes = %d, esperado 1", s.ProjectID.String(), n)
		}
	}
}

// TestINV04_MesmoPathWorkspacesDiferentesCoexistem: o mesmo path em workspaces
// diferentes gera duas versões correntes, uma por workspace.
func TestINV04_MesmoPathWorkspacesDiferentesCoexistem(t *testing.T) {
	_, db, w := sinvEnv(t)
	sX := sinvScope(t, w, sinvNewWS(t), "ws-inv04b-x", "pj-x")
	sY := sinvScope(t, w, sinvNewWS(t), "ws-inv04b-y", "pj-y")
	p := sinvPath(t, "notas/comum.md")

	rX := sinvUpsert(t, w, sinvInput(t, domain.NewPageRef(sX, p), "corpo do workspace X", "sess-x", nil, nil))
	rY := sinvUpsert(t, w, sinvInput(t, domain.NewPageRef(sY, p), "corpo do workspace Y", "sess-y", nil, nil))

	if rX.Outcome != storage.UpsertCreated || rY.Outcome != storage.UpsertCreated {
		t.Fatalf("outcomes = %q/%q, esperado created/created", rX.Outcome, rY.Outcome)
	}
	if n := sinvCount(t, db, `SELECT COUNT(*) FROM pages WHERE path = ? AND is_latest = 1`, p.String()); n != 2 {
		t.Fatalf("versões correntes do path entre workspaces = %d, esperado 2", n)
	}
	for _, s := range []domain.Scope{sX, sY} {
		wsB, pjB := sinvEscopoBytes(s)
		if n := sinvCount(t, db, `SELECT COUNT(*) FROM pages WHERE workspace_id = ? AND project_id = ? AND path = ? AND is_latest = 1`,
			wsB, pjB, p.String()); n != 1 {
			t.Fatalf("workspace %s: versões correntes = %d, esperado 1", s.WorkspaceID.String(), n)
		}
	}
}

// TestINV04_MesmoEscopoSupersedeSemDuplicarCorrente: no mesmo escopo e com o
// mesmo path, uma nova versão supersede a corrente (is_latest = 0 na antiga) e
// o mesmo conteúdo não cria versão. Nunca há duas correntes.
func TestINV04_MesmoEscopoSupersedeSemDuplicarCorrente(t *testing.T) {
	_, db, w := sinvEnv(t)
	scope := sinvScope(t, w, sinvNewWS(t), "ws-inv04c", "pj-inv04c")
	wsB, pjB := sinvEscopoBytes(scope)
	ref := domain.NewPageRef(scope, sinvPath(t, "notas/versoes.md"))
	p := ref.Path.String()

	v1 := sinvUpsert(t, w, sinvInput(t, ref, "versão 1", "sess-1", nil, nil))
	if v1.Outcome != storage.UpsertCreated {
		t.Fatalf("v1: outcome = %q, esperado created", v1.Outcome)
	}

	v2 := sinvUpsert(t, w, sinvInput(t, ref, "versão 2", "sess-2", nil, nil))
	if v2.Outcome != storage.UpsertSuperseded {
		t.Fatalf("v2: outcome = %q, esperado superseded", v2.Outcome)
	}
	if n := sinvCount(t, db, `SELECT COUNT(*) FROM pages WHERE workspace_id = ? AND project_id = ? AND path = ?`,
		wsB, pjB, p); n != 2 {
		t.Fatalf("após supersessão: %d versões gravadas, esperado 2", n)
	}
	if n := sinvCount(t, db, `SELECT COUNT(*) FROM pages WHERE workspace_id = ? AND project_id = ? AND path = ? AND is_latest = 1`,
		wsB, pjB, p); n != 1 {
		t.Fatalf("após supersessão: %d correntes, esperado 1", n)
	}
	if n := sinvCount(t, db, `SELECT is_latest FROM pages WHERE id = ?`, v1.Version.Bytes()); n != 0 {
		t.Fatalf("versão antiga: is_latest = %d, esperado 0", n)
	}
	if n := sinvCount(t, db, `SELECT COUNT(*) FROM pages WHERE id = ? AND supersedes = ?`,
		v2.Version.Bytes(), v1.Version.Bytes()); n != 1 {
		t.Fatal("versão nova não aponta supersedes para a versão antiga")
	}

	// Mesmo conteúdo: caso A do P-04, sem versão nova.
	v3 := sinvUpsert(t, w, sinvInput(t, ref, "versão 2", "sess-3", nil, nil))
	if v3.Outcome != storage.UpsertUnchanged {
		t.Fatalf("conteúdo repetido: outcome = %q, esperado unchanged", v3.Outcome)
	}
	if n := sinvCount(t, db, `SELECT COUNT(*) FROM pages WHERE workspace_id = ? AND project_id = ? AND path = ?`,
		wsB, pjB, p); n != 2 {
		t.Fatalf("conteúdo repetido criou versão: %d gravadas, esperado 2", n)
	}
}

// TestINV04_TodaLinhaTemIdentidadeTipada: toda linha de pages tem workspace_id e
// project_id não nulos, e cada versão gravada carrega exatamente o escopo e o
// path pedidos.
func TestINV04_TodaLinhaTemIdentidadeTipada(t *testing.T) {
	_, db, w := sinvEnv(t)
	sA := sinvScope(t, w, sinvNewWS(t), "ws-inv04d-a", "pj-a")
	sB := sinvScope(t, w, sinvNewWS(t), "ws-inv04d-b", "pj-b")

	type gravado struct {
		ref     domain.PageRef
		version domain.PageID
	}
	var feitos []gravado
	for _, s := range []domain.Scope{sA, sB} {
		for _, nome := range []string{"notas/um.md", "notas/dois.md"} {
			ref := domain.NewPageRef(s, sinvPath(t, nome))
			res := sinvUpsert(t, w, sinvInput(t, ref, "corpo de "+nome, "sess-d", nil, nil))
			feitos = append(feitos, gravado{ref: ref, version: res.Version})
		}
	}

	if n := sinvCount(t, db, `SELECT COUNT(*) FROM pages WHERE workspace_id IS NULL OR project_id IS NULL`); n != 0 {
		t.Fatalf("linhas de pages com workspace_id ou project_id nulo = %d, esperado 0", n)
	}
	if n := sinvCount(t, db, `SELECT COUNT(*) FROM pages`); n != len(feitos) {
		t.Fatalf("pages = %d, esperado %d", n, len(feitos))
	}
	for _, g := range feitos {
		wsB, pjB := sinvEscopoBytes(g.ref.Scope)
		if n := sinvCount(t, db, `SELECT COUNT(*) FROM pages WHERE id = ? AND workspace_id = ? AND project_id = ? AND path = ?`,
			g.version.Bytes(), wsB, pjB, g.ref.Path.String()); n != 1 {
			t.Fatalf("versão %s não carrega a identidade (%s): contagem %d, esperado 1",
				g.version.String(), g.ref.String(), n)
		}
	}
}
