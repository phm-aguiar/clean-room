package wiki

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
	"github.com/phm-aguiar/ai-memory-go/internal/storage"
)

// errBoom é a falha injetada depois que o arquivo foi instalado.
var errBoom = errors.New("falha injetada no índice")

// wpEnv reúne o ambiente de um teste de escrita: raiz da wiki em diretório temporário, banco
// temporário com Writer e Reader reais, repositório git e o Store.
type wpEnv struct {
	ctx     context.Context
	root    string
	db      *sql.DB
	writer  storage.Writer
	reader  storage.Reader
	repo    *Repo
	store   *Store
	scope   domain.Scope
	clock   *time.Time
	wsName  string
	prjName string
}

func wpNewEnv(t *testing.T) *wpEnv {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	repo, err := Open(root)
	if err != nil {
		t.Fatalf("Open(repo): %v", err)
	}
	db, err := storage.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	w, err := storage.NewWriter(ctx, db)
	if err != nil {
		t.Fatalf("storage.NewWriter: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })

	ws, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	pj, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	clock := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	env := &wpEnv{
		ctx:     ctx,
		root:    root,
		db:      db,
		writer:  w,
		reader:  storage.NewReader(db),
		repo:    repo,
		scope:   domain.NewScope(domain.WorkspaceID(ws), domain.ProjectID(pj)),
		clock:   &clock,
		wsName:  "ws-teste",
		prjName: "proj-teste",
	}
	env.store = &Store{
		Root:   root,
		Writer: w,
		Repo:   repo,
		Locks:  NewPageLocks(),
		Now:    func() time.Time { return *env.clock },
	}
	return env
}

// wpInput monta um pedido de escrita de página com tier semantic e evidência de sessão.
func wpInput(t *testing.T, env *wpEnv, path, body string) PageInput {
	t.Helper()
	p, err := domain.ParsePagePath(path)
	if err != nil {
		t.Fatalf("ParsePagePath(%q): %v", path, err)
	}
	return PageInput{
		Scope:         env.scope,
		WorkspaceName: env.wsName,
		ProjectName:   env.prjName,
		Path:          p,
		Tier:          domain.TierSemantic,
		Body:          body,
		Evidence:      storage.EvidenceSource{Kind: storage.SourceSession, ID: "sessao-1"},
	}
}

// wpAbs é o caminho absoluto da página no disco, montado só para o teste.
func wpAbs(env *wpEnv, rel string) string {
	return filepath.Join(env.root, env.scope.WorkspaceID.String(), env.scope.ProjectID.String(), filepath.FromSlash(rel))
}

// wpRef é a identidade da página para leitura no banco.
func wpRef(t *testing.T, env *wpEnv, path string) domain.PageRef {
	t.Helper()
	p, err := domain.ParsePagePath(path)
	if err != nil {
		t.Fatalf("ParsePagePath(%q): %v", path, err)
	}
	return domain.NewPageRef(env.scope, p)
}

// wpCommits conta os commits do repositório da wiki. Sem commit ainda devolve 0.
func wpCommits(t *testing.T, env *wpEnv) int {
	t.Helper()
	cmd := gitInDir(t, env.root, "rev-list", "--count", "HEAD")
	n, err := strconv.Atoi(strings.TrimSpace(cmd))
	if err != nil {
		t.Fatalf("rev-list --count: %v (%q)", err, cmd)
	}
	return n
}

// wpMustWrite chama WritePage e falha o teste se der erro.
func wpMustWrite(t *testing.T, env *wpEnv, in PageInput) WriteResult {
	t.Helper()
	res, err := env.store.WritePage(env.ctx, in)
	if err != nil {
		t.Fatalf("WritePage: %v", err)
	}
	return res
}

func TestWritePage_CaminhoFeliz(t *testing.T) {
	env := wpNewEnv(t)
	in := wpInput(t, env, "notas/rota.md", "# Rota\n\nCorpo da rota.\n")
	in.Frontmatter = mustFM(t, "summary", "resumo curto")

	res := wpMustWrite(t, env, in)

	if res.Outcome != storage.UpsertCreated {
		t.Fatalf("Outcome = %q, quer created", res.Outcome)
	}
	if res.Version == (domain.PageID{}) {
		t.Fatal("Version vazia")
	}
	rel := env.scope.WorkspaceID.String() + "/" + env.scope.ProjectID.String() + "/notas/rota.md"
	if res.Path != rel {
		t.Fatalf("Path = %q, quer %q", res.Path, rel)
	}
	if res.Checkpoint == "" {
		t.Fatal("Checkpoint vazio: deveria ter commit")
	}
	if len(res.Avisos) != 0 {
		t.Fatalf("Avisos inesperados: %v", res.Avisos)
	}

	// Arquivo com frontmatter derivado e corpo sem alteração. O frontmatter é lido de volta, para
	// não depender do estilo de aspas do YAML.
	data := readFile(t, wpAbs(env, "notas/rota.md"))
	fmRead, bodyRead := mustRead(t, []byte(data))
	for k, want := range map[string]string{
		"tier": "semantic", "type": "Note", "description": "resumo curto",
		"generated.at": "2026-03-01T10:00:00Z", "generated.by": generatedBy,
	} {
		if got := wpGet(fmRead, k); got != want {
			t.Fatalf("frontmatter %s = %q, quer %q\n%s", k, got, want, data)
		}
	}
	if bodyRead != "# Rota\n\nCorpo da rota.\n" {
		t.Fatalf("corpo no arquivo: %q", bodyRead)
	}

	// Linha no banco.
	page, err := storage.ReadPage(env.ctx, env.reader, wpRef(t, env, "notas/rota.md"))
	if err != nil {
		t.Fatalf("ReadPage: %v", err)
	}
	if page.Title != "Rota" || page.Body != "# Rota\n\nCorpo da rota.\n" {
		t.Fatalf("página no banco: título %q, corpo %q", page.Title, page.Body)
	}
	if page.Version != res.Version {
		t.Fatalf("Version do banco %s, do resultado %s", page.Version.String(), res.Version.String())
	}

	// Commit com a mensagem exata e o arquivo da página.
	if subj := strings.TrimSpace(gitInDir(t, env.root, "log", "-1", "--format=%s")); subj != "page-write: "+rel {
		t.Fatalf("mensagem do commit = %q, quer %q", subj, "page-write: "+rel)
	}
	if hash := strings.TrimSpace(gitInDir(t, env.root, "rev-parse", "HEAD")); hash != res.Checkpoint {
		t.Fatalf("Checkpoint %q, HEAD %q", res.Checkpoint, hash)
	}
	files := gitInDir(t, env.root, "show", "--name-only", "--format=", "HEAD")
	for _, want := range []string{
		rel,
		env.scope.WorkspaceID.String() + "/_meta.md",
		env.scope.WorkspaceID.String() + "/" + env.scope.ProjectID.String() + "/_meta.md",
		env.scope.WorkspaceID.String() + "/" + env.scope.ProjectID.String() + "/index.md",
		env.scope.WorkspaceID.String() + "/" + env.scope.ProjectID.String() + "/log.md",
	} {
		if !strings.Contains(files, want) {
			t.Fatalf("commit sem %q; arquivos:\n%s", want, files)
		}
	}

	// Linha no log.md, antes do commit (ADR-22).
	logLine := "## [2026-03-01T10:00:00Z] page-write | Rota\n"
	logData := readFile(t, wpAbs(env, "log.md"))
	if logData != logLine {
		t.Fatalf("log.md = %q, quer %q", logData, logLine)
	}

	// Manifestos e index.md.
	manifest := readFile(t, filepath.Join(env.root, env.scope.WorkspaceID.String(), "_meta.md"))
	if !strings.Contains(manifest, "name: ws-teste") {
		t.Fatalf("manifesto do workspace sem nome:\n%s", manifest)
	}
	projManifest := readFile(t, wpAbs(env, "_meta.md"))
	if !strings.Contains(projManifest, "name: proj-teste") {
		t.Fatalf("manifesto do projeto sem nome:\n%s", projManifest)
	}
	if idx := readFile(t, wpAbs(env, "index.md")); !strings.Contains(idx, "# Índice") {
		t.Fatalf("index.md sem título:\n%s", idx)
	}
}

func TestWritePage_CorpoSemTeto(t *testing.T) {
	env := wpNewEnv(t)
	body := "# Grande\n\n" + strings.Repeat("linha de texto da página\n", 1000)
	wpMustWrite(t, env, wpInput(t, env, "notas/grande.md", body))

	page, err := storage.ReadPage(env.ctx, env.reader, wpRef(t, env, "notas/grande.md"))
	if err != nil {
		t.Fatalf("ReadPage: %v", err)
	}
	if page.Body != body {
		t.Fatalf("corpo cortado: %d bytes, quer %d", len(page.Body), len(body))
	}
	if !strings.Contains(readFile(t, wpAbs(env, "notas/grande.md")), body) {
		t.Fatal("arquivo não contém o corpo inteiro")
	}
}

func TestWritePage_IdenticaNaoReescreveNemVersionaNemCommita(t *testing.T) {
	env := wpNewEnv(t)
	in := wpInput(t, env, "notas/igual.md", "# Igual\n\ncorpo\n")
	first := wpMustWrite(t, env, in)
	commits := wpCommits(t, env)
	logBefore := readFile(t, wpAbs(env, "log.md"))

	// Data antiga no arquivo: se for reescrito, a data muda.
	old := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	path := wpAbs(env, "notas/igual.md")
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	contentBefore := readFile(t, path)

	*env.clock = env.clock.Add(time.Hour)
	second := wpMustWrite(t, env, in)

	if second.Outcome != storage.UpsertUnchanged {
		t.Fatalf("Outcome = %q, quer unchanged", second.Outcome)
	}
	if second.Version != first.Version {
		t.Fatal("escrita idêntica criou versão nova")
	}
	if second.Checkpoint != "" {
		t.Fatalf("escrita idêntica devolveu checkpoint %q", second.Checkpoint)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !info.ModTime().Equal(old) {
		t.Fatalf("arquivo reescrito: mtime %v", info.ModTime())
	}
	if readFile(t, path) != contentBefore {
		t.Fatal("conteúdo do arquivo mudou")
	}
	if n := wpCommits(t, env); n != commits {
		t.Fatalf("commits %d -> %d: escrita idêntica não deveria commitar", commits, n)
	}
	if readFile(t, wpAbs(env, "log.md")) != logBefore {
		t.Fatal("escrita idêntica acrescentou linha ao log")
	}
}

func TestWritePage_SupersedeTrocaArquivoECriaVersao(t *testing.T) {
	env := wpNewEnv(t)
	first := wpMustWrite(t, env, wpInput(t, env, "notas/mud.md", "# Mud\n\nversão um\n"))
	commits := wpCommits(t, env)

	*env.clock = env.clock.Add(time.Minute)
	second := wpMustWrite(t, env, wpInput(t, env, "notas/mud.md", "# Mud\n\nversão dois\n"))

	if second.Outcome != storage.UpsertSuperseded {
		t.Fatalf("Outcome = %q, quer superseded", second.Outcome)
	}
	if second.Version == first.Version {
		t.Fatal("supersessão não criou versão nova")
	}
	if !strings.Contains(readFile(t, wpAbs(env, "notas/mud.md")), "versão dois") {
		t.Fatal("arquivo não recebeu o conteúdo novo")
	}
	page, err := storage.ReadPage(env.ctx, env.reader, wpRef(t, env, "notas/mud.md"))
	if err != nil {
		t.Fatalf("ReadPage: %v", err)
	}
	if page.Body != "# Mud\n\nversão dois\n" {
		t.Fatalf("corpo corrente %q", page.Body)
	}
	chain, err := storage.PageChain(env.ctx, env.reader, wpRef(t, env, "notas/mud.md"))
	if err != nil {
		t.Fatalf("PageChain: %v", err)
	}
	if len(chain) != 2 {
		t.Fatalf("cadeia com %d versões, quer 2", len(chain))
	}
	if n := wpCommits(t, env); n != commits+1 {
		t.Fatalf("commits %d -> %d, quer +1", commits, n)
	}
	if logData := readFile(t, wpAbs(env, "log.md")); strings.Count(logData, "page-write") != 2 {
		t.Fatalf("log.md deveria ter duas linhas page-write:\n%s", logData)
	}
}

// TestWritePage_DuasSessoesSupersedeSemDestruir cobre a supersessão multi-sessão (INV-16): duas
// instâncias de Store, com o mesmo Writer, a mesma raiz e as mesmas travas, escrevem a mesma página.
// A versão perdedora continua no banco.
func TestWritePage_DuasSessoesSupersedeSemDestruir(t *testing.T) {
	env := wpNewEnv(t)
	sessaoDois := *env.store
	sessaoDois.Now = func() time.Time { return env.clock.Add(time.Minute) }

	first := wpMustWrite(t, env, wpInput(t, env, "notas/dupla.md", "# Dupla\n\nda sessão um\n"))
	res, err := sessaoDois.WritePage(env.ctx, wpInput(t, env, "notas/dupla.md", "# Dupla\n\nda sessão dois\n"))
	if err != nil {
		t.Fatalf("WritePage sessão dois: %v", err)
	}
	if res.Outcome != storage.UpsertSuperseded {
		t.Fatalf("Outcome = %q, quer superseded", res.Outcome)
	}
	chain, err := storage.PageChain(env.ctx, env.reader, wpRef(t, env, "notas/dupla.md"))
	if err != nil {
		t.Fatalf("PageChain: %v", err)
	}
	if len(chain) != 2 {
		t.Fatalf("cadeia com %d versões, quer 2", len(chain))
	}
	found := false
	for _, v := range chain {
		if v.ID == first.Version {
			found = true
		}
	}
	if !found {
		t.Fatal("versão da sessão um não é mais alcançável")
	}
}

// wpFailingWriter deixa passar as chamadas de Do até a de número failOn (1 = primeira). Nessa
// chamada executa a closure dentro de uma transação real e depois devolve errBoom, o que faz o
// storage desfazer a transação. Em WritePage, a chamada 1 é EnsureScope e a 2 é UpsertPage, então
// failOn = 2 simula falha do índice depois da instalação do arquivo. beforeTx, se não for nil, roda
// antes dessa chamada (usado para sabotar a restauração).
type wpFailingWriter struct {
	storage.Writer
	failOn   int
	calls    int
	beforeTx func()
}

func (f *wpFailingWriter) Do(ctx context.Context, fn func(*sql.Tx) error) error {
	f.calls++
	if f.calls != f.failOn {
		return f.Writer.Do(ctx, fn)
	}
	if f.beforeTx != nil {
		f.beforeTx()
	}
	return f.Writer.Do(ctx, func(tx *sql.Tx) error {
		if err := fn(tx); err != nil {
			return err
		}
		return errBoom
	})
}

func TestWritePage_FalhaDoBancoDepoisDeInstalarReverteArquivo(t *testing.T) {
	env := wpNewEnv(t)
	first := wpMustWrite(t, env, wpInput(t, env, "notas/falha.md", "# Falha\n\nconteúdo anterior\n"))
	prev := readFile(t, wpAbs(env, "notas/falha.md"))
	commits := wpCommits(t, env)
	logBefore := readFile(t, wpAbs(env, "log.md"))

	failing := *env.store
	failing.Writer = &wpFailingWriter{Writer: env.writer, failOn: 2}
	*env.clock = env.clock.Add(time.Minute)
	_, err := failing.WritePage(env.ctx, wpInput(t, env, "notas/falha.md", "# Falha\n\nconteúdo novo\n"))
	if !errors.Is(err, errBoom) {
		t.Fatalf("erro = %v, quer encadear errBoom", err)
	}
	if errors.Is(err, ErrInconsistentState) {
		t.Fatalf("restauração funcionou e não deveria devolver ErrInconsistentState: %v", err)
	}

	if got := readFile(t, wpAbs(env, "notas/falha.md")); got != prev {
		t.Fatalf("arquivo não foi revertido:\n%s", got)
	}
	if n := wpCommits(t, env); n != commits {
		t.Fatalf("commits %d -> %d: falha não deveria commitar", commits, n)
	}
	if readFile(t, wpAbs(env, "log.md")) != logBefore {
		t.Fatal("log.md alterado numa falha")
	}
	page, err := storage.ReadPage(env.ctx, env.reader, wpRef(t, env, "notas/falha.md"))
	if err != nil {
		t.Fatalf("ReadPage: %v", err)
	}
	if page.Version != first.Version || page.Body != "# Falha\n\nconteúdo anterior\n" {
		t.Fatalf("índice mudou na falha: versão %s, corpo %q", page.Version.String(), page.Body)
	}
	entries, err := os.ReadDir(filepath.Dir(wpAbs(env, "notas/falha.md")))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), tmpPrefix) {
			t.Fatalf("temporário deixado para trás: %s", e.Name())
		}
	}
}

func TestWritePage_FalhaDeRestauracaoDevolveInconsistentState(t *testing.T) {
	env := wpNewEnv(t)
	wpMustWrite(t, env, wpInput(t, env, "notas/preso.md", "# Preso\n\nanterior\n"))
	path := wpAbs(env, "notas/preso.md")

	// Antes da transação, troca o arquivo por um diretório não vazio. A restauração não consegue
	// instalar o conteúdo anterior sobre ele, e o storage desfaz a transação.
	sabotar := func() {
		if err := os.Remove(path); err != nil {
			t.Fatalf("Remove: %v", err)
		}
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatalf("Mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(path, "preso.txt"), []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	failing := *env.store
	failing.Writer = &wpFailingWriter{Writer: env.writer, failOn: 2, beforeTx: sabotar}
	*env.clock = env.clock.Add(time.Minute)
	_, err := failing.WritePage(env.ctx, wpInput(t, env, "notas/preso.md", "# Preso\n\nnovo\n"))
	if !errors.Is(err, ErrInconsistentState) {
		t.Fatalf("erro = %v, quer ErrInconsistentState", err)
	}
	if !errors.Is(err, errBoom) {
		t.Fatalf("ErrInconsistentState não encadeia o erro original: %v", err)
	}
	if !errors.Is(err, storage.ErrInconsistentState) {
		t.Fatalf("ErrInconsistentState de wiki não é o de storage: %v", err)
	}
}

func TestWritePage_CaminhoForaDaRaizRecusado(t *testing.T) {
	env := wpNewEnv(t)
	in := wpInput(t, env, "notas/fora.md", "# Fora\n")

	relativa := *env.store
	relativa.Root = "relativa/raiz"
	if _, err := relativa.WritePage(env.ctx, in); !errors.Is(err, ErrConfinement) {
		t.Fatalf("raiz relativa: erro = %v, quer ErrConfinement", err)
	}

	// Componente do caminho que é link simbólico apontando para fora da raiz.
	fora := t.TempDir()
	link := filepath.Join(env.root, env.scope.WorkspaceID.String())
	if err := os.Symlink(fora, link); err != nil {
		// No Windows, o link simbólico exige privilégio. A junção (mklink /J) não exige e também é
		// ponto de reparse, que confine.go recusa do mesmo jeito.
		if out, jerr := exec.Command("cmd", "/c", "mklink", "/J", link, fora).CombinedOutput(); jerr != nil {
			t.Skipf("link simbólico e junção indisponíveis: %v; %v: %s", err, jerr, out)
		}
	}
	if _, err := env.store.WritePage(env.ctx, in); !errors.Is(err, ErrConfinement) {
		t.Fatalf("link no caminho: erro = %v, quer ErrConfinement", err)
	}
	if entries, _ := os.ReadDir(fora); len(entries) != 0 {
		t.Fatalf("escrita escapou para fora da raiz: %d entradas", len(entries))
	}
}

func TestWritePage_ExpiresAtInvalidoNaoTocaDisco(t *testing.T) {
	env := wpNewEnv(t)
	in := wpInput(t, env, "notas/validade.md", "# Validade\n")
	in.Frontmatter = mustFM(t, "expires_at", "amanha")

	_, err := env.store.WritePage(env.ctx, in)
	if !errors.Is(err, ErrInvalidExpiresAt) {
		t.Fatalf("erro = %v, quer ErrInvalidExpiresAt", err)
	}
	if _, err := os.Stat(wpAbs(env, "notas/validade.md")); !os.IsNotExist(err) {
		t.Fatalf("arquivo criado com expires_at inválido: %v", err)
	}
	if _, err := os.Stat(filepath.Join(env.root, env.scope.WorkspaceID.String(), "_meta.md")); !os.IsNotExist(err) {
		t.Fatalf("manifesto criado com expires_at inválido: %v", err)
	}
	if _, err := storage.ReadPage(env.ctx, env.reader, wpRef(t, env, "notas/validade.md")); !errors.Is(err, storage.ErrPageNotFound) {
		t.Fatalf("linha criada com expires_at inválido: %v", err)
	}
}

func TestWritePage_ExpiresAtDerivaStaleAfter(t *testing.T) {
	env := wpNewEnv(t)
	in := wpInput(t, env, "notas/prazo.md", "# Prazo\n")
	in.Frontmatter = mustFM(t, "expires_at", "2026-12-31")
	wpMustWrite(t, env, in)

	fm, _ := mustRead(t, []byte(readFile(t, wpAbs(env, "notas/prazo.md"))))
	if got := wpGet(fm, "stale_after"); got != "2026-12-31T23:59:59.999999Z" {
		t.Fatalf("stale_after = %q", got)
	}
}

// wpGet devolve o texto de key no frontmatter, ou vazio se a chave não existe ou não é texto.
func wpGet(fm domain.Frontmatter, key string) string {
	v, ok := fm.Get(key)
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

func TestWritePage_FalhaDeCommitViraAviso(t *testing.T) {
	env := wpNewEnv(t)
	// Trava do índice do git: `git add` falha e o commit não acontece.
	lock := filepath.Join(env.root, ".git", "index.lock")
	if err := os.WriteFile(lock, nil, 0o644); err != nil {
		t.Fatalf("WriteFile(index.lock): %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(lock) })

	res, err := env.store.WritePage(env.ctx, wpInput(t, env, "notas/commit.md", "# Commit\n"))
	if err != nil {
		t.Fatalf("falha de commit não deveria ser erro: %v", err)
	}
	if res.Checkpoint != "" {
		t.Fatalf("Checkpoint = %q, quer vazio", res.Checkpoint)
	}
	found := false
	for _, a := range res.Avisos {
		if strings.Contains(a, "commit não realizado") {
			found = true
		}
	}
	if !found {
		t.Fatalf("aviso de commit ausente: %v", res.Avisos)
	}
	if _, err := storage.ReadPage(env.ctx, env.reader, wpRef(t, env, "notas/commit.md")); err != nil {
		t.Fatalf("página não foi gravada no índice: %v", err)
	}
}

func TestWritePage_ValidacaoDeEntradaAntesDeTocarDisco(t *testing.T) {
	env := wpNewEnv(t)

	semTier := wpInput(t, env, "notas/tier.md", "# Tier\n")
	semTier.Tier = domain.Tier("")
	if _, err := env.store.WritePage(env.ctx, semTier); !errors.Is(err, domain.ErrMalformedRecord) {
		t.Fatalf("tier vazio: erro = %v, quer ErrMalformedRecord", err)
	}

	semEvidencia := wpInput(t, env, "notas/evid.md", "# Evid\n")
	semEvidencia.Evidence = storage.EvidenceSource{}
	if _, err := env.store.WritePage(env.ctx, semEvidencia); err == nil {
		t.Fatal("escrita sem evidência foi aceita")
	}

	if _, err := os.Stat(wpAbs(env, "notas/tier.md")); !os.IsNotExist(err) {
		t.Fatalf("arquivo criado com entrada inválida: %v", err)
	}
}

// mustFM monta um frontmatter com os pares chave/valor recebidos, na ordem da chamada.
func mustFM(t *testing.T, pairs ...string) domain.Frontmatter {
	t.Helper()
	var fm domain.Frontmatter
	for i := 0; i+1 < len(pairs); i += 2 {
		if err := fm.Set(pairs[i], pairs[i+1]); err != nil {
			t.Fatalf("Set(%q): %v", pairs[i], err)
		}
	}
	return fm
}

// TestWritePage_PinnedPersisteQuandoAusenteEFalseDesafixa: reescrever sem a chave `pinned` mantém a
// fixação; só `pinned: false` explícito desafixa (T085).
func TestWritePage_PinnedPersisteQuandoAusenteEFalseDesafixa(t *testing.T) {
	env := wpNewEnv(t)
	pinned := func() int {
		return wcCount(t, env.db, `SELECT pinned FROM pages WHERE path = ? AND is_latest = 1`, "notas/fixa.md")
	}

	v1 := wpInput(t, env, "notas/fixa.md", "Corpo v1.\n")
	v1.Frontmatter = mustFM(t, "pinned", "true")
	wpMustWrite(t, env, v1)
	if pinned() != 1 {
		t.Fatalf("pinned após v1 = %d, quer 1", pinned())
	}

	wpMustWrite(t, env, wpInput(t, env, "notas/fixa.md", "Corpo v2.\n"))
	if pinned() != 1 {
		t.Errorf("pinned após reescrita sem a chave = %d, quer 1 (a fixação deve persistir)", pinned())
	}

	v3 := wpInput(t, env, "notas/fixa.md", "Corpo v3.\n")
	v3.Frontmatter = mustFM(t, "pinned", "false")
	wpMustWrite(t, env, v3)
	if pinned() != 0 {
		t.Errorf("pinned após pinned: false = %d, quer 0", pinned())
	}
}
