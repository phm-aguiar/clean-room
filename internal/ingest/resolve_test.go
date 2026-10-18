package ingest

import (
	"errors"
	"fmt"
	"testing"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// sessionFor cria um SessionID determinístico para os testes.
func sessionFor(t *testing.T, s string) domain.SessionID {
	t.Helper()
	id, err := domain.SessionIDFromNative(s)
	if err != nil {
		t.Fatalf("SessionIDFromNative(%q): %v", s, err)
	}
	return id
}

func TestResolveMarkerReadsQuotedValuesByLine(t *testing.T) {
	content := []byte("# comentário\nworkspace = \"acme\"\nproject = \"api\"\nproject_strategy = \"repo_root\"\n")
	ms := ReadMarkerScope(content)
	if ms.Workspace != "acme" || ms.Project != "api" || ms.Strategy != StrategyRepoRoot {
		t.Fatalf("ReadMarkerScope = %+v", ms)
	}
}

func TestResolveMarkerIgnoresBareAndInvalidValues(t *testing.T) {
	// Valor nu não é lido; estratégia desconhecida vira vazia.
	ms := ReadMarkerScope([]byte("project = api\nproject_strategy = \"weird\"\n"))
	if ms.Project != "" || ms.Strategy != "" {
		t.Fatalf("ReadMarkerScope = %+v, quer vazio", ms)
	}
	// Chave `project_strategy` não casa com `project`.
	ms = ReadMarkerScope([]byte("project_strategy = \"repo-root\"\n"))
	if ms.Project != "" || ms.Strategy != StrategyRepoRoot {
		t.Fatalf("ReadMarkerScope = %+v", ms)
	}
}

func TestResolveMarkerFirstLineWins(t *testing.T) {
	ms := ReadMarkerScope([]byte("project = \"first\"\n[recall]\nproject = \"second\"\n"))
	if ms.Project != "first" {
		t.Fatalf("Project = %q, quer first", ms.Project)
	}
}

func TestResolveMarkerDeclaresScope(t *testing.T) {
	cases := map[string]struct {
		content string
		want    bool
	}{
		"só capture":       {"[capture]\nignore_paths = [\"private/**\"]\n", false},
		"project na raiz":  {"project = \"x\"\n", true},
		"chave em seção":   {"[briefing]\nmax_chars = 3000\n", true},
		"nada":             {"", false},
		"chave parecida":   {"projects = \"x\"\n", false},
		"project_strategy": {"project_strategy = \"basename\"\n", true},
	}
	for name, tc := range cases {
		if got := MarkerDeclaresScope([]byte(tc.content)); got != tc.want {
			t.Errorf("%s: MarkerDeclaresScope = %v, quer %v", name, got, tc.want)
		}
	}
}

func TestResolveOrderNoCwdUsesDefault(t *testing.T) {
	res, err := Resolve(ResolveInput{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Workspace != "default" || res.Project != "scratch" || res.Source != SourceDefault {
		t.Fatalf("Resolve = %+v, quer default/scratch", res)
	}
}

func TestResolveOrderMarkerOverlayWins(t *testing.T) {
	in := ResolveInput{
		Cwd:           "/work/acme/api/sub",
		RepoRoot:      "/work/acme/api",
		MarkerContent: []byte("workspace = \"acme\"\nproject = \"declared\"\n"),
		Known:         []KnownProject{{Workspace: "other", Project: "reg", Path: "/work/acme/api"}},
	}
	res, err := Resolve(in, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != SourceMarker || res.Project != "declared" || res.Workspace != "acme" {
		t.Fatalf("Resolve = %+v", res)
	}
	if res.RepoPath != "/work/acme/api" {
		t.Fatalf("RepoPath = %q, quer a raiz git", res.RepoPath)
	}
}

func TestResolveOrderPrefixBeforeDerived(t *testing.T) {
	in := ResolveInput{
		Cwd:   "/work/proj/src/pkg",
		Known: []KnownProject{{Workspace: "default", Project: "proj", Path: "/work/proj"}},
	}
	res, err := Resolve(in, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != SourcePrefix || res.Project != "proj" {
		t.Fatalf("Resolve = %+v: subdiretório deve cair no projeto pai", res)
	}
}

func TestResolveDerivedBasenameUsesCwdFolder(t *testing.T) {
	res, err := Resolve(ResolveInput{Cwd: `C:\Users\henri\Projetos\Meu-App`}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != SourceDerived || res.Workspace != "default" || res.Project != "Meu-App" {
		t.Fatalf("Resolve = %+v: nome deve manter a caixa crua", res)
	}
	if res.RepoPath != "" {
		t.Fatalf("RepoPath = %q: cwd sem git não grava caminho", res.RepoPath)
	}
}

func TestResolveDerivedRepoRootUsesMainRepoName(t *testing.T) {
	in := ResolveInput{
		Cwd:           "/work/checkout/app/internal",
		RepoRoot:      "/work/checkout/app",
		MarkerContent: []byte("project_strategy = \"repo-root\"\n"),
	}
	res, err := Resolve(in, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Project != "app" || res.Source != SourceDerived || res.RepoPath != "/work/checkout/app" {
		t.Fatalf("Resolve = %+v", res)
	}
}

func TestResolveDerivedRepoRootWithoutRepoUsesCwdAndNoPath(t *testing.T) {
	in := ResolveInput{
		Cwd:             "/data/scratch/run42",
		DefaultStrategy: StrategyRepoRoot,
	}
	res, err := Resolve(in, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Project != "run42" || res.RepoPath != "" {
		t.Fatalf("Resolve = %+v: sem raiz git, nome do cwd e sem caminho", res)
	}
}

func TestResolveDefaultStrategyIsBasenameWhenEmpty(t *testing.T) {
	in := ResolveInput{Cwd: "/work/repo/sub", RepoRoot: "/work/repo"}
	res, err := Resolve(in, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Project != "sub" {
		t.Fatalf("Project = %q, quer sub (basename do cwd)", res.Project)
	}
}

func TestResolveRefusesReservedGlobal(t *testing.T) {
	cases := map[string]ResolveInput{
		"nome derivado":      {Cwd: "/work/_global"},
		"sobreposição":       {Cwd: "/work/x", MarkerContent: []byte("project = \"_global\"\n")},
		"prefixo registrado": {Cwd: "/work/g", Known: []KnownProject{{Workspace: "default", Project: "_global", Path: "/work/g"}}},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			res, err := Resolve(in, nil)
			if err != nil {
				t.Fatal(err)
			}
			if res.Project == "_global" || res.Source != SourceDefault {
				t.Fatalf("Resolve = %+v: _global deve ser recusado e cair no padrão", res)
			}
			if res.RepoPath != "" {
				t.Fatalf("RepoPath = %q: recusa não grava caminho", res.RepoPath)
			}
		})
	}
}

func TestResolveLongestPrefixWins(t *testing.T) {
	known := []KnownProject{
		{Workspace: "default", Project: "mono", Path: "/work/mono"},
		{Workspace: "default", Project: "mono-web", Path: "/work/mono/web"},
		{Workspace: "default", Project: "sem-caminho", Path: ""},
	}
	res, err := Resolve(ResolveInput{Cwd: "/work/mono/web/src", Known: known}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Project != "mono-web" || res.Source != SourcePrefix {
		t.Fatalf("Resolve = %+v, quer mono-web (prefixo mais longo)", res)
	}
	res, err = Resolve(ResolveInput{Cwd: "/work/mono/api", Known: known}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Project != "mono" {
		t.Fatalf("Project = %q, quer mono", res.Project)
	}
}

func TestResolvePrefixMatchesWholeComponentsOnly(t *testing.T) {
	known := []KnownProject{{Workspace: "default", Project: "b", Path: "/a/b"}}
	res, err := Resolve(ResolveInput{Cwd: "/a/bc", Known: known}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Source == SourcePrefix {
		t.Fatalf("Resolve = %+v: /a/bc não é subdiretório de /a/b", res)
	}
	if res.Project != "bc" {
		t.Fatalf("Project = %q, quer bc", res.Project)
	}
}

func TestResolvePrefixRespectsMarkerWorkspace(t *testing.T) {
	known := []KnownProject{{Workspace: "other", Project: "proj", Path: "/work/proj"}}
	in := ResolveInput{
		Cwd:           "/work/proj/src",
		Known:         known,
		MarkerContent: []byte("workspace = \"acme\"\n"),
	}
	res, err := Resolve(in, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Source == SourcePrefix || res.Workspace != "acme" || res.Project != "src" {
		t.Fatalf("Resolve = %+v: workspace do marcador filtra o prefixo", res)
	}
}

func TestResolveCaseIsKeptForPrefixPaths(t *testing.T) {
	// Caminho registrado com caixa diferente não casa: nome e prefixo usam o caminho cru.
	known := []KnownProject{{Workspace: "default", Project: "proj", Path: `C:\Work\Proj`}}
	res, err := Resolve(ResolveInput{Cwd: `C:\Work\Proj\src`, Known: known}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != SourcePrefix || res.Project != "proj" {
		t.Fatalf("Resolve = %+v", res)
	}
}

func TestResolveRepoPathOnlyForRealRepoRoot(t *testing.T) {
	res, err := Resolve(ResolveInput{Cwd: "/home/u/pasta", RepoRoot: ""}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.RepoPath != "" {
		t.Fatalf("RepoPath = %q: diretório atual nu nunca é gravado", res.RepoPath)
	}
}

func TestResolveRejectsUnknownMidSession(t *testing.T) {
	_, err := Resolve(ResolveInput{Cwd: "/x", MidSession: "outro"}, nil)
	if !errors.Is(err, ErrUnknownMidSession) {
		t.Fatalf("erro = %v, quer ErrUnknownMidSession", err)
	}
}

func stickyBase(t *testing.T) ResolveInput {
	return ResolveInput{
		HasSession: true,
		Session:    sessionFor(t, "sess-1"),
		HomeDir:    "/home/u",
	}
}

var projectAnchor = &Anchor{Dir: "/work/proj", Workspace: "default", Project: "proj"}

func TestResolveStickyFollowCwdInsideSubtree(t *testing.T) {
	in := stickyBase(t)
	in.Cwd = "/work/proj/tmp"
	in.MidSession = MidSessionFollowCwd
	res, err := Resolve(in, projectAnchor)
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != SourceSticky || res.Project != "proj" {
		t.Fatalf("Resolve = %+v, quer sticky em proj", res)
	}
}

func TestResolveStickyFollowCwdOutsideSubtreeFollows(t *testing.T) {
	in := stickyBase(t)
	in.Cwd = "/work/outro"
	in.MidSession = MidSessionFollowCwd
	res, err := Resolve(in, projectAnchor)
	if err != nil {
		t.Fatal(err)
	}
	if res.Source == SourceSticky || res.Project != "outro" {
		t.Fatalf("Resolve = %+v, follow-cwd deve seguir o cwd", res)
	}
}

func TestResolveStickyFollowCwdRepoRootOutsideRepoInherits(t *testing.T) {
	in := stickyBase(t)
	in.Cwd = "/data/scratch"
	in.DefaultStrategy = StrategyRepoRoot
	res, err := Resolve(in, projectAnchor)
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != SourceSticky || res.Project != "proj" {
		t.Fatalf("Resolve = %+v: repo-root fora de repo herda o projeto da sessão", res)
	}
}

func TestResolveStickyFollowCwdRepoRootInsideRepoFollows(t *testing.T) {
	in := stickyBase(t)
	in.Cwd = "/data/outro-repo"
	in.RepoRoot = "/data/outro-repo"
	in.DefaultStrategy = StrategyRepoRoot
	res, err := Resolve(in, projectAnchor)
	if err != nil {
		t.Fatal(err)
	}
	if res.Source == SourceSticky {
		t.Fatalf("Resolve = %+v: dentro de outro repositório segue o cwd", res)
	}
}

func TestResolveStickyModeOverridesOutsideSubtree(t *testing.T) {
	in := stickyBase(t)
	in.Cwd = "/work/outro-repo"
	in.RepoRoot = "/work/outro-repo"
	in.MidSession = MidSessionSticky
	res, err := Resolve(in, projectAnchor)
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != SourceSticky || res.Project != "proj" {
		t.Fatalf("Resolve = %+v: sticky gruda fora da subárvore", res)
	}
}

func TestResolveStickyMarkerProjectAlwaysWins(t *testing.T) {
	for _, mode := range []string{MidSessionFollowCwd, MidSessionSticky} {
		in := stickyBase(t)
		in.Cwd = "/work/proj/tmp"
		in.MidSession = mode
		in.MarkerContent = []byte("project = \"declarado\"\n")
		res, err := Resolve(in, projectAnchor)
		if err != nil {
			t.Fatal(err)
		}
		if res.Source != SourceMarker || res.Project != "declarado" {
			t.Fatalf("modo %s: Resolve = %+v, marcador deve vencer", mode, res)
		}
	}
}

func TestResolveStickyMarkerWorkspaceRescopeDisables(t *testing.T) {
	in := stickyBase(t)
	in.Cwd = "/work/proj/tmp"
	in.MidSession = MidSessionSticky
	in.MarkerContent = []byte("workspace = \"outro\"\n")
	res, err := Resolve(in, projectAnchor)
	if err != nil {
		t.Fatal(err)
	}
	if res.Source == SourceSticky || res.Workspace != "outro" {
		t.Fatalf("Resolve = %+v: reescopo de workspace desliga a sessão pegajosa", res)
	}
}

func TestResolveStickyBroadAnchorNeverSticks(t *testing.T) {
	anchors := map[string]Anchor{
		"home":         {Dir: "/home/u", Workspace: "default", Project: "u"},
		"raiz":         {Dir: "/", Workspace: "default", Project: "root"},
		"raiz-unidade": {Dir: `C:\`, Workspace: "default", Project: "c"},
	}
	for name, a := range anchors {
		t.Run(name, func(t *testing.T) {
			in := stickyBase(t)
			in.Cwd = a.Dir + "/tmp"
			in.MidSession = MidSessionSticky
			res, err := Resolve(in, &a)
			if err != nil {
				t.Fatal(err)
			}
			if res.Source == SourceSticky {
				t.Fatalf("Resolve = %+v: âncora ampla não gruda", res)
			}
		})
	}
}

func TestResolveStickyOpeningEventNeverSticks(t *testing.T) {
	in := stickyBase(t)
	in.Cwd = "/work/proj/tmp"
	in.MidSession = MidSessionSticky
	in.Opening = true
	res, err := Resolve(in, projectAnchor)
	if err != nil {
		t.Fatal(err)
	}
	if res.Source == SourceSticky {
		t.Fatalf("Resolve = %+v: evento de abertura usa o próprio diretório", res)
	}
}

func TestResolveStickyNoSessionNoStick(t *testing.T) {
	in := stickyBase(t)
	in.HasSession = false
	in.Cwd = "/work/proj/tmp"
	in.MidSession = MidSessionSticky
	res, err := Resolve(in, projectAnchor)
	if err != nil {
		t.Fatal(err)
	}
	if res.Source == SourceSticky {
		t.Fatalf("Resolve = %+v: sem sessão não há âncora", res)
	}
}

func TestResolveStickyDefaultModeIsFollowCwd(t *testing.T) {
	in := stickyBase(t)
	in.Cwd = "/work/outro"
	in.MidSession = ""
	res, err := Resolve(in, projectAnchor)
	if err != nil {
		t.Fatal(err)
	}
	if res.Source == SourceSticky {
		t.Fatalf("Resolve = %+v: vazio equivale a follow-cwd", res)
	}
}

func TestResolveScopeRequestCreates(t *testing.T) {
	req := Resolution{Workspace: "acme", Project: "api"}.ScopeRequest()
	if !req.Create || req.WorkspaceName != "acme" || req.ProjectName != "api" {
		t.Fatalf("ScopeRequest = %+v", req)
	}
}

func TestResolverStickyCacheRecordsAndSticks(t *testing.T) {
	r := NewResolver()
	open := stickyBase(t)
	open.Cwd = "/work/proj"
	open.Opening = true
	res, err := r.Resolve(open)
	if err != nil {
		t.Fatal(err)
	}
	if res.Project != "proj" {
		t.Fatalf("abertura: Project = %q", res.Project)
	}

	mid := stickyBase(t)
	mid.Cwd = "/work/proj/tmp"
	mid.MidSession = MidSessionSticky
	res, err = r.Resolve(mid)
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != SourceSticky || res.Project != "proj" {
		t.Fatalf("mid-sessão: Resolve = %+v", res)
	}
}

func TestResolverStickyCacheUnknownSessionResolvesFully(t *testing.T) {
	r := NewResolver()
	in := stickyBase(t)
	in.Cwd = "/work/x"
	in.MidSession = MidSessionSticky
	res, err := r.Resolve(in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Source == SourceSticky || res.Project != "x" {
		t.Fatalf("Resolve = %+v: sem âncora, resolve pelo próprio cwd", res)
	}
	if r.cache.Len() != 1 {
		t.Fatalf("cache Len = %d, quer 1 (primeiro evento vira âncora)", r.cache.Len())
	}
}

func TestResolverStickyCacheReopenReplacesAnchor(t *testing.T) {
	r := NewResolver()
	first := stickyBase(t)
	first.Cwd = "/work/proj"
	first.Opening = true
	if _, err := r.Resolve(first); err != nil {
		t.Fatal(err)
	}
	again := stickyBase(t)
	again.Cwd = "/work/outro"
	again.Opening = true
	res, err := r.Resolve(again)
	if err != nil {
		t.Fatal(err)
	}
	if res.Project != "outro" {
		t.Fatalf("reabertura: Project = %q, quer outro", res.Project)
	}
	if a, ok := r.cache.Get(again.Session); !ok || a.Project != "outro" {
		t.Fatalf("âncora após reabertura = %+v, %v", a, ok)
	}
}

func TestResolveStickyCacheLimit(t *testing.T) {
	c := NewStickyCache()
	if c.max != 4096 || StickyCacheLimit != 4096 {
		t.Fatalf("limite = %d, quer 4096", c.max)
	}
	ids := make([]domain.SessionID, 0, StickyCacheLimit+1)
	for i := 0; i <= StickyCacheLimit; i++ {
		ids = append(ids, sessionFor(t, fmt.Sprintf("s-%d", i)))
	}
	for i := 0; i < StickyCacheLimit; i++ {
		c.Put(ids[i], Anchor{Dir: "/d", Project: fmt.Sprint(i)})
	}
	if c.Len() != StickyCacheLimit {
		t.Fatalf("Len = %d, quer %d", c.Len(), StickyCacheLimit)
	}
	// Usa a mais antiga: ela passa a ser a mais recente e a segunda sai.
	if _, ok := c.Get(ids[0]); !ok {
		t.Fatal("entrada 0 deveria existir")
	}
	c.Put(ids[StickyCacheLimit], Anchor{Dir: "/d", Project: "novo"})
	if c.Len() != StickyCacheLimit {
		t.Fatalf("após estouro Len = %d, quer %d", c.Len(), StickyCacheLimit)
	}
	if _, ok := c.Get(ids[1]); ok {
		t.Fatal("entrada 1 (menos usada) deveria ter saído")
	}
	if _, ok := c.Get(ids[0]); !ok {
		t.Fatal("entrada 0 (recém usada) não deveria ter saído")
	}
	if _, ok := c.Get(ids[StickyCacheLimit]); !ok {
		t.Fatal("entrada nova deveria existir")
	}
}

func TestResolveIsWithin(t *testing.T) {
	cases := []struct {
		dir, root string
		want      bool
	}{
		{"/a/b", "/a/b", true},
		{"/a/b/c", "/a/b", true},
		{"/a/bc", "/a/b", false},
		{"/a", "/a/b", false},
		{"/anything", "/", true},
		{`C:\Work\x`, `C:\Work`, true},
		{`C:\Workx`, `C:\Work`, false},
		{"/a/b", "", false},
	}
	for _, tc := range cases {
		if got := isWithin(tc.dir, tc.root); got != tc.want {
			t.Errorf("isWithin(%q, %q) = %v, quer %v", tc.dir, tc.root, got, tc.want)
		}
	}
}
