package wiki

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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
	probe := exec.Command("git", "rev-parse", "--verify", "-q", "HEAD")
	probe.Dir = env.root
	if err := probe.Run(); err != nil {
		return 0 // sem HEAD: nenhum commit ainda
	}
	cmd := gitInDir(t, env.root, "rev-list", "--count", "HEAD")
	n, err := strconv.Atoi(strings.TrimSpace(cmd))
	if err != nil {
		t.Fatalf("rev-list --count: %v (%q)", err, cmd)
	}
	return n
}

// wpCommitTracked faz o commit dos caminhos registrados, como quem chama WritePage deve fazer, e
// devolve o hash. Falha de commit encerra o teste.
func wpCommitTracked(t *testing.T, env *wpEnv, msg string) string {
	t.Helper()
	cp, err := env.repo.CommitTracked(env.ctx, msg)
	if err != nil {
		t.Fatalf("CommitTracked: %v", err)
	}
	return cp
}

// wpSemLedgerNemIndice confere o que a escrita de página não toca: log.md e index.md do projeto
// (docs/wiki-files-layer.md §5.2). O ledger é só de eventos de hook, e o index.md da raiz é criado
// só pela migração OKF.
func wpSemLedgerNemIndice(t *testing.T, env *wpEnv) {
	t.Helper()
	for _, nome := range []string{"log.md", "index.md"} {
		if _, err := os.Stat(wpAbs(env, nome)); !os.IsNotExist(err) {
			t.Fatalf("%s existe após a escrita de página (err=%v); a operação normal não o escreve", nome, err)
		}
	}
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
	if res.Checkpoint != "" {
		t.Fatalf("Checkpoint = %q: WritePage não deveria commitar", res.Checkpoint)
	}
	if n := wpCommits(t, env); n != 0 {
		t.Fatalf("WritePage criou %d commit(s), quer 0", n)
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
	} {
		if got := wpGet(fmRead, k); got != want {
			t.Fatalf("frontmatter %s = %q, quer %q\n%s", k, got, want, data)
		}
	}
	gen := wpGenerated(t, fmRead)
	if got := wpGet(gen, "at"); got != "2026-03-01T10:00:00Z" {
		t.Fatalf("generated.at = %q, quer 2026-03-01T10:00:00Z\n%s", got, data)
	}
	if got := wpGet(gen, "by"); got != generatedByPrefix+buildVersion() {
		t.Fatalf("generated.by = %q, quer %q", got, generatedByPrefix+buildVersion())
	}
	if !reflect.DeepEqual(gen.Keys(), []string{"by", "at"}) {
		t.Fatalf("chaves de generated = %v, quer [by at]", gen.Keys())
	}
	if _, ok := fmRead.Get("generated.at"); ok {
		t.Fatal("chave plana generated.at no frontmatter; quer mapeamento generated")
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

	// O chamador commita os caminhos registrados pela escrita: mensagem exata e arquivos da página.
	cp := wpCommitTracked(t, env, "page-write: "+rel)
	if subj := strings.TrimSpace(gitInDir(t, env.root, "log", "-1", "--format=%s")); subj != "page-write: "+rel {
		t.Fatalf("mensagem do commit = %q, quer %q", subj, "page-write: "+rel)
	}
	if hash := strings.TrimSpace(gitInDir(t, env.root, "rev-parse", "HEAD")); hash != cp {
		t.Fatalf("checkpoint do commit %q, HEAD %q", cp, hash)
	}
	if again, err := env.repo.CommitTracked(env.ctx, "page-write: "+rel); err != nil || again != "" {
		t.Fatalf("segundo CommitTracked = %q, %v; quer vazio e nil (registro esvaziado)", again, err)
	}
	files := gitInDir(t, env.root, "show", "--name-only", "--format=", "HEAD")
	for _, want := range []string{
		rel,
		env.scope.WorkspaceID.String() + "/_meta.md",
		env.scope.WorkspaceID.String() + "/" + env.scope.ProjectID.String() + "/_meta.md",
	} {
		if !strings.Contains(files, want) {
			t.Fatalf("commit sem %q; arquivos:\n%s", want, files)
		}
	}
	for _, nao := range []string{"/log.md", "/index.md"} {
		if strings.Contains(files, nao) {
			t.Fatalf("commit com %q: a escrita de página não deveria tocar esse arquivo:\n%s", nao, files)
		}
	}
	wpSemLedgerNemIndice(t, env)

	// Manifestos.
	manifest := readFile(t, filepath.Join(env.root, env.scope.WorkspaceID.String(), "_meta.md"))
	if !strings.Contains(manifest, "name: ws-teste") {
		t.Fatalf("manifesto do workspace sem nome:\n%s", manifest)
	}
	projManifest := readFile(t, wpAbs(env, "_meta.md"))
	if !strings.Contains(projManifest, "name: proj-teste") {
		t.Fatalf("manifesto do projeto sem nome:\n%s", projManifest)
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
	wpCommitTracked(t, env, "page-write: notas/igual.md")
	commits := wpCommits(t, env)

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
	if cp := wpCommitTracked(t, env, "page-write: notas/igual.md"); cp != "" {
		t.Fatalf("escrita idêntica registrou caminho para commit: checkpoint %q", cp)
	}
	wpSemLedgerNemIndice(t, env)
}

func TestWritePage_SupersedeTrocaArquivoECriaVersao(t *testing.T) {
	env := wpNewEnv(t)
	first := wpMustWrite(t, env, wpInput(t, env, "notas/mud.md", "# Mud\n\nversão um\n"))
	wpCommitTracked(t, env, "page-write: notas/mud.md")
	commits := wpCommits(t, env)

	*env.clock = env.clock.Add(time.Minute)
	second := wpMustWrite(t, env, wpInput(t, env, "notas/mud.md", "# Mud\n\nversão dois\n"))
	if second.Checkpoint != "" {
		t.Fatalf("supersessão devolveu checkpoint %q", second.Checkpoint)
	}
	wpCommitTracked(t, env, "page-write: notas/mud.md")

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
	wpSemLedgerNemIndice(t, env)
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
	wpSemLedgerNemIndice(t, env)
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

// TestWritePage_FalhaDeCommitNaoAfetaEscrita: WritePage não commita, então uma trava do git não
// impede a escrita. Quem commita recebe o erro (o chamador o trata como aviso, plan P-03) e o
// registro fica pendente: o commit seguinte, com a trava liberada, inclui a página.
func TestWritePage_FalhaDeCommitNaoAfetaEscrita(t *testing.T) {
	env := wpNewEnv(t)
	// Trava do índice do git: `git add` falha quando o commit é tentado.
	lock := filepath.Join(env.root, ".git", "index.lock")
	if err := os.WriteFile(lock, nil, 0o644); err != nil {
		t.Fatalf("WriteFile(index.lock): %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(lock) })

	res, err := env.store.WritePage(env.ctx, wpInput(t, env, "notas/commit.md", "# Commit\n"))
	if err != nil {
		t.Fatalf("WritePage com trava do git: %v", err)
	}
	if res.Checkpoint != "" {
		t.Fatalf("Checkpoint = %q, quer vazio", res.Checkpoint)
	}
	if len(res.Avisos) != 0 {
		t.Fatalf("WritePage sem commit não deveria ter aviso de commit: %v", res.Avisos)
	}
	if _, err := storage.ReadPage(env.ctx, env.reader, wpRef(t, env, "notas/commit.md")); err != nil {
		t.Fatalf("página não foi gravada no índice: %v", err)
	}

	if cp, err := env.repo.CommitTracked(env.ctx, "page-write: notas/commit.md"); err == nil {
		t.Fatalf("CommitTracked com trava do git deveria falhar, deu checkpoint %q", cp)
	}

	if err := os.Remove(lock); err != nil {
		t.Fatalf("Remove(index.lock): %v", err)
	}
	cp := wpCommitTracked(t, env, "page-write: notas/commit.md")
	if cp == "" {
		t.Fatal("registro perdido: o commit seguinte não criou commit")
	}
	files := gitInDir(t, env.root, "show", "--name-only", "--format=", "HEAD")
	if !strings.Contains(files, "notas/commit.md") {
		t.Fatalf("commit seguinte sem a página registrada:\n%s", files)
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

// TestWritePage_CaminhoDeSlotEFixado: página sob `_slots/` é fixada pelo caminho, sem a chave
// `pinned` (docs/wiki-files-layer.md §5.2, passo 3). Página comum fora de `_slots/` não é fixada.
func TestWritePage_CaminhoDeSlotEFixado(t *testing.T) {
	env := wpNewEnv(t)
	pinned := func(path string) int {
		return wcCount(t, env.db, `SELECT pinned FROM pages WHERE path = ? AND is_latest = 1`, path)
	}

	res := wpMustWrite(t, env, wpInput(t, env, "_slots/current-focus.md", "Foco atual.\n"))
	if pinned("_slots/current-focus.md") != 1 {
		t.Fatalf("slot sem chave pinned: pinned = %d, quer 1", pinned("_slots/current-focus.md"))
	}
	fmRead, _ := mustRead(t, []byte(readFile(t, wpAbs(env, "_slots/current-focus.md"))))
	if got := wpGet(fmRead, "pinned"); got != "true" {
		t.Fatalf("frontmatter pinned do slot = %q, quer true", got)
	}
	if res.Path == "" {
		t.Fatal("Path vazio")
	}

	wpMustWrite(t, env, wpInput(t, env, "notas/comum.md", "Corpo comum.\n"))
	if pinned("notas/comum.md") != 0 {
		t.Fatalf("página fora de _slots/ fixada sem pedido: pinned = %d", pinned("notas/comum.md"))
	}
}

// wpGenerated lê o mapeamento generated de um frontmatter lido do arquivo.
func wpGenerated(t *testing.T, fm domain.Frontmatter) domain.Frontmatter {
	t.Helper()
	v, ok := fm.Get("generated")
	if !ok {
		t.Fatal("frontmatter sem generated")
	}
	g, ok := v.(domain.Frontmatter)
	if !ok {
		t.Fatalf("generated = %T, quer mapeamento", v)
	}
	return g
}

// wpSources lê a lista de mapeamentos sources de um frontmatter lido do arquivo.
func wpSources(t *testing.T, fm domain.Frontmatter) []domain.Frontmatter {
	t.Helper()
	v, ok := fm.Get("sources")
	if !ok {
		return nil
	}
	items, ok := v.([]domain.Frontmatter)
	if !ok {
		t.Fatalf("sources = %T, quer lista de mapeamentos", v)
	}
	return items
}

// wpFileFM lê de volta o frontmatter do arquivo da página, sem depender do banco.
func wpFileFM(t *testing.T, env *wpEnv, rel string) domain.Frontmatter {
	t.Helper()
	fm, _ := mustRead(t, []byte(readFile(t, wpAbs(env, rel))))
	return fm
}

// TestBuildVersion_NuncaVaziaNemDevel: sem versão de módulo (build de desenvolvimento), a versão
// do generated.by é "dev", nunca vazia nem "(devel)".
func TestBuildVersion_NuncaVaziaNemDevel(t *testing.T) {
	if v := buildVersion(); v == "" || v == "(devel)" {
		t.Fatalf("buildVersion() = %q, quer versão não vazia e diferente de (devel)", v)
	}
}

// TestWritePage_GeneratedByDeModeloEDaEntrada: generated.by vale generated_by_model quando a página
// o traz; um generated.by já presente na entrada não é sobrescrito (docs/okf.md).
func TestWritePage_GeneratedByDeModeloEDaEntrada(t *testing.T) {
	env := wpNewEnv(t)

	modelo := wpInput(t, env, "notas/modelo.md", "Corpo.\n")
	modelo.Frontmatter = mustFM(t, "generated_by_model", "modelo-x")
	wpMustWrite(t, env, modelo)
	fmModelo := wpFileFM(t, env, "notas/modelo.md")
	if got := wpGet(wpGenerated(t, fmModelo), "by"); got != "modelo-x" {
		t.Fatalf("generated.by com generated_by_model = %q, quer modelo-x", got)
	}
	if got := wpGet(fmModelo, "generated_by_model"); got != "modelo-x" {
		t.Fatalf("generated_by_model (chave de extensão) = %q, quer preservada", got)
	}

	entrada := wpInput(t, env, "notas/entrada.md", "Corpo.\n")
	gen := domain.Frontmatter{}
	if err := gen.Set("by", "humano:revisor"); err != nil {
		t.Fatal(err)
	}
	entrada.Frontmatter = mustFM(t)
	if err := entrada.Frontmatter.Set("generated", gen); err != nil {
		t.Fatal(err)
	}
	wpMustWrite(t, env, entrada)
	if got := wpGet(wpGenerated(t, wpFileFM(t, env, "notas/entrada.md")), "by"); got != "humano:revisor" {
		t.Fatalf("generated.by da entrada = %q, quer humano:revisor (não sobrescrito)", got)
	}
}

// TestWritePage_GeneratedAtPreservadoQuandoConteudoIgual: reescrever com o mesmo conteúdo mantém
// generated.at; mudar o conteúdo carimba o instante da escrita (docs/okf.md, Determinismo).
func TestWritePage_GeneratedAtPreservadoQuandoConteudoIgual(t *testing.T) {
	env := wpNewEnv(t)
	wpMustWrite(t, env, wpInput(t, env, "notas/det.md", "Corpo.\n"))

	*env.clock = env.clock.Add(time.Hour)
	wpMustWrite(t, env, wpInput(t, env, "notas/det.md", "Corpo.\n"))
	if got := wpGet(wpGenerated(t, wpFileFM(t, env, "notas/det.md")), "at"); got != "2026-03-01T10:00:00Z" {
		t.Fatalf("generated.at após reescrita igual = %q, quer o original 2026-03-01T10:00:00Z", got)
	}

	*env.clock = env.clock.Add(time.Hour)
	wpMustWrite(t, env, wpInput(t, env, "notas/det.md", "Corpo v2.\n"))
	if got := wpGet(wpGenerated(t, wpFileFM(t, env, "notas/det.md")), "at"); got != "2026-03-01T12:00:00Z" {
		t.Fatalf("generated.at após mudança de conteúdo = %q, quer 2026-03-01T12:00:00Z", got)
	}
}

// TestWritePage_SourcesDeSessaoEAgente: sources nasce de session_id (resource ai-memory://session/<id>)
// e, com agent, ganha author process:<agent>. Sem session_id não nasce; sources da entrada não muda.
func TestWritePage_SourcesDeSessaoEAgente(t *testing.T) {
	env := wpNewEnv(t)

	com := wpInput(t, env, "notas/com-agente.md", "Corpo.\n")
	com.Frontmatter = mustFM(t, "session_id", "sess-9", "agent", "reviewer")
	wpMustWrite(t, env, com)
	itens := wpSources(t, wpFileFM(t, env, "notas/com-agente.md"))
	if len(itens) != 1 {
		t.Fatalf("sources com agente: %d itens, quer 1", len(itens))
	}
	if got := wpGet(itens[0], "resource"); got != "ai-memory://session/sess-9" {
		t.Fatalf("sources.resource = %q, quer ai-memory://session/sess-9", got)
	}
	if got := wpGet(itens[0], "author"); got != "process:reviewer" {
		t.Fatalf("sources.author = %q, quer process:reviewer", got)
	}

	sem := wpInput(t, env, "notas/sem-agente.md", "Corpo.\n")
	sem.Frontmatter = mustFM(t, "session_id", "sess-9")
	wpMustWrite(t, env, sem)
	itens = wpSources(t, wpFileFM(t, env, "notas/sem-agente.md"))
	if len(itens) != 1 || wpGet(itens[0], "resource") != "ai-memory://session/sess-9" {
		t.Fatalf("sources sem agente = %v, quer um item com resource da sessão", itens)
	}
	if _, ok := itens[0].Get("author"); ok {
		t.Fatal("sources sem agent não devia ter author")
	}

	nada := wpInput(t, env, "notas/sem-sessao.md", "Corpo.\n")
	wpMustWrite(t, env, nada)
	if itens := wpSources(t, wpFileFM(t, env, "notas/sem-sessao.md")); len(itens) != 0 {
		t.Fatalf("sources sem session_id = %v, quer nenhum", itens)
	}

	explicito := wpInput(t, env, "notas/explicito.md", "Corpo.\n")
	var fonte domain.Frontmatter
	if err := fonte.Set("resource", "https://exemplo.test/fonte"); err != nil {
		t.Fatal(err)
	}
	explicito.Frontmatter = mustFM(t, "session_id", "sess-9")
	if err := explicito.Frontmatter.Set("sources", []domain.Frontmatter{fonte}); err != nil {
		t.Fatal(err)
	}
	wpMustWrite(t, env, explicito)
	itens = wpSources(t, wpFileFM(t, env, "notas/explicito.md"))
	if len(itens) != 1 || wpGet(itens[0], "resource") != "https://exemplo.test/fonte" {
		t.Fatalf("sources da entrada foi alterado: %v", itens)
	}
}

// TestWritePage_RelationsComoMapeamentoPreservado: relations chega como mapeamento por relação e
// sobrevive à escrita (cleanCopy não o achata nem o perde).
func TestWritePage_RelationsComoMapeamentoPreservado(t *testing.T) {
	env := wpNewEnv(t)
	var rels domain.Frontmatter
	if err := rels.Set("fixes", []string{"gotchas/linker-oom.md"}); err != nil {
		t.Fatal(err)
	}
	in := wpInput(t, env, "notas/correcao.md", "# Correção\n\nCorpo.\n")
	in.Frontmatter = mustFM(t)
	if err := in.Frontmatter.Set("relations", rels); err != nil {
		t.Fatal(err)
	}
	wpMustWrite(t, env, in)

	fm := wpFileFM(t, env, "notas/correcao.md")
	v, ok := fm.Get("relations")
	if !ok {
		t.Fatal("relations sumiu do frontmatter")
	}
	lido, ok := v.(domain.Frontmatter)
	if !ok {
		t.Fatalf("relations = %T, quer mapeamento", v)
	}
	if !reflect.DeepEqual(lido.Keys(), []string{"fixes"}) {
		t.Fatalf("chaves de relations = %v, quer [fixes]", lido.Keys())
	}
	alvos, _ := lido.Get("fixes")
	if !reflect.DeepEqual(alvos, []string{"gotchas/linker-oom.md"}) {
		t.Fatalf("alvos de fixes = %v", alvos)
	}
}
