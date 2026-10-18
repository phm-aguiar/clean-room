package mcp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
	"github.com/phm-aguiar/ai-memory-go/internal/storage"
	"github.com/phm-aguiar/ai-memory-go/internal/wiki"
)

// writeFixture monta um banco real, uma wiki git em diretório temporário, os escopos default/_global e
// default/scratch e o servidor MCP com as ferramentas de escrita registradas. Sem ponteiro de projeto
// ativo: a escrita sem escopo deve ser recusada.
type writeFixture struct {
	t       *testing.T
	ctx     context.Context
	root    string
	reader  storage.Reader
	srv     ToolServer
	global  domain.Scope
	scratch domain.Scope
}

func newWriteFixture(t *testing.T) *writeFixture {
	t.Helper()
	ctx := context.Background()

	dataDir := t.TempDir()
	db, err := storage.Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	writer, err := storage.NewWriter(ctx, db)
	if err != nil {
		t.Fatalf("storage.NewWriter: %v", err)
	}
	t.Cleanup(func() { _ = writer.Close() })

	readDB, err := storage.OpenReader(ctx, dataDir)
	if err != nil {
		t.Fatalf("storage.OpenReader: %v", err)
	}
	t.Cleanup(func() { _ = readDB.Close() })
	reader := storage.NewReader(readDB)

	root := filepath.Join(dataDir, "wiki")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir wiki: %v", err)
	}
	repo, err := wiki.Open(root)
	if err != nil {
		t.Fatalf("wiki.Open: %v", err)
	}
	store := &wiki.Store{
		Root:   root,
		Writer: writer,
		Repo:   repo,
		Locks:  wiki.NewPageLocks(),
		Now:    func() time.Time { return baseTime },
	}

	ws := domain.WorkspaceID(newTestID(t))
	f := &writeFixture{
		t:       t,
		ctx:     ctx,
		root:    root,
		reader:  reader,
		global:  domain.NewScope(ws, domain.ProjectID(newTestID(t))),
		scratch: domain.NewScope(ws, domain.ProjectID(newTestID(t))),
	}
	if err := storage.EnsureScope(ctx, writer, f.global, "default", "_global", baseTime); err != nil {
		t.Fatalf("EnsureScope global: %v", err)
	}
	if err := storage.EnsureScope(ctx, writer, f.scratch, "default", "scratch", baseTime); err != nil {
		t.Fatalf("EnsureScope scratch: %v", err)
	}

	srv := NewToolServer()
	if err := RegisterWriteTools(srv, WriteDeps{
		Store:  store,
		Reader: reader,
		Now:    func() time.Time { return baseTime },
	}); err != nil {
		t.Fatalf("RegisterWriteTools: %v", err)
	}
	f.srv = srv
	return f
}

// invoke chama uma ferramenta pelo protocolo (tools/call), sem identidade de ator.
func (f *writeFixture) invoke(name string, args map[string]any) toolOutcome {
	f.t.Helper()
	resp := rpc(f.t, f.srv, f.ctx, 1, "tools/call", map[string]any{"name": name, "arguments": args})
	if resp == nil {
		f.t.Fatalf("%s: sem resposta", name)
	}
	if e, ok := resp["error"]; ok {
		f.t.Fatalf("%s: erro JSON-RPC inesperado: %v", name, e)
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		f.t.Fatalf("%s: resultado ausente: %v", name, resp)
	}
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		f.t.Fatalf("%s: conteúdo ausente: %v", name, result)
	}
	first, _ := content[0].(map[string]any)
	text, _ := first["text"].(string)
	isErr, _ := result["isError"].(bool)
	return toolOutcome{text: text, isError: isErr}
}

// readFrontmatter lê o frontmatter do arquivo da página gravada, pelo caminho relativo devolvido.
func (f *writeFixture) readFrontmatter(rel string) map[string]any {
	f.t.Helper()
	data, err := os.ReadFile(filepath.Join(f.root, filepath.FromSlash(rel)))
	if err != nil {
		f.t.Fatalf("ler página %s: %v", rel, err)
	}
	fm, _, err := wiki.ReadMarkdown(data)
	if err != nil {
		f.t.Fatalf("frontmatter de %s: %v", rel, err)
	}
	out := map[string]any{}
	for _, key := range []string{"title", "tier", "pinned", "expires_at"} {
		if v, ok := fm.Get(key); ok {
			out[key] = v
		}
	}
	return out
}

func mustWritePath(t *testing.T, s string) domain.PagePath {
	t.Helper()
	pp, err := domain.ParsePagePath(s)
	if err != nil {
		t.Fatalf("ParsePagePath(%q): %v", s, err)
	}
	return pp
}

func TestWritePage_EscopoGlobalGravaEmDefaultGlobalComFrontmatter(t *testing.T) {
	f := newWriteFixture(t)
	const path = "regras/nunca-force-push.md"

	out := f.invoke("memory_write_page", map[string]any{
		"scope":      "global",
		"path":       path,
		"body":       "Nunca faça force-push na branch principal.",
		"title":      "Nunca force-push",
		"tags":       []string{"git"},
		"pinned":     true,
		"expires_at": "2030-01-01",
	}).ok(t)

	if out["page_id"] == "" || out["page_id"] == nil {
		t.Fatalf("page_id ausente: %v", out)
	}
	rel, _ := out["path"].(string)
	if rel == "" {
		t.Fatalf("path ausente: %v", out)
	}
	if cp, _ := out["checkpoint"].(string); cp == "" {
		t.Fatalf("checkpoint ausente com escrita commitável: %v", out)
	}

	// A página existe no escopo global, não em outro.
	page, err := storage.ReadPage(f.ctx, f.reader, domain.NewPageRef(f.global, mustWritePath(t, path)))
	if err != nil {
		t.Fatalf("ReadPage no escopo global: %v", err)
	}
	if !strings.Contains(page.Body, "force-push") {
		t.Fatalf("corpo gravado não confere: %q", page.Body)
	}

	// Frontmatter aceito: title, tier padrão, pinned como texto e expires_at.
	fm := f.readFrontmatter(rel)
	if fm["title"] != "Nunca force-push" {
		t.Errorf("title = %v", fm["title"])
	}
	if fm["tier"] != "semantic" {
		t.Errorf("tier padrão = %v, quer semantic", fm["tier"])
	}
	if fm["pinned"] != "true" {
		t.Errorf("pinned = %v, quer texto \"true\"", fm["pinned"])
	}
	if fm["expires_at"] != "2030-01-01" {
		t.Errorf("expires_at = %v", fm["expires_at"])
	}
}

func TestWritePage_PinnedAusenteHerdaEFalseExplicitoDesafixa(t *testing.T) {
	f := newWriteFixture(t)
	const path = "regras/fixada.md"

	first := f.invoke("memory_write_page", map[string]any{
		"scope": "global", "path": path, "body": "Regra fixada.", "pinned": true,
	}).ok(t)
	rel := first["path"].(string)

	// Reescrita sem pinned mantém a fixação da versão anterior.
	f.invoke("memory_write_page", map[string]any{
		"scope": "global", "path": path, "body": "Regra reescrita sem o campo pinned.",
	}).ok(t)
	if fm := f.readFrontmatter(rel); fm["pinned"] != "true" {
		t.Fatalf("pinned ausente deveria herdar \"true\", veio %v", fm["pinned"])
	}

	// pinned false explícito desafixa.
	f.invoke("memory_write_page", map[string]any{
		"scope": "global", "path": path, "body": "Regra reescrita sem o campo pinned.", "pinned": false,
	}).ok(t)
	if fm := f.readFrontmatter(rel); fm["pinned"] != "false" {
		t.Fatalf("pinned false explícito deveria desafixar, veio %v", fm["pinned"])
	}
}

func TestWritePage_TierPadraoEpisodicoEmPaginaDeSessao(t *testing.T) {
	f := newWriteFixture(t)
	scratch := map[string]any{"workspace": "default", "project": "scratch"}

	sess := f.invoke("memory_write_page", mergeWriteArgs(scratch, map[string]any{
		"path": "sessions/abc123.md", "body": "Sessão.",
	})).ok(t)
	if fm := f.readFrontmatter(sess["path"].(string)); fm["tier"] != "episodic" {
		t.Errorf("tier da página de sessão = %v, quer episodic", fm["tier"])
	}

	note := f.invoke("memory_write_page", mergeWriteArgs(scratch, map[string]any{
		"path": "notas/a.md", "body": "Nota.",
	})).ok(t)
	if fm := f.readFrontmatter(note["path"].(string)); fm["tier"] != "semantic" {
		t.Errorf("tier padrão = %v, quer semantic", fm["tier"])
	}
}

func TestWritePage_EscritaAmbiguaSemEscopoRecusada(t *testing.T) {
	f := newWriteFixture(t)
	const path = "solto/sem-escopo.md"

	out := f.invoke("memory_write_page", map[string]any{"path": path, "body": "Sem escopo."})
	if !out.isError {
		t.Fatalf("escrita sem escopo e sem ponteiro deveria ser recusada, veio %q", out.text)
	}
	if !strings.Contains(out.text, "ambígua") {
		t.Fatalf("recusa deveria citar escrita ambígua, veio %q", out.text)
	}

	// Nada foi gravado no padrão em silêncio.
	if _, err := storage.ReadPage(f.ctx, f.reader, domain.NewPageRef(f.scratch, mustWritePath(t, path))); err == nil {
		t.Fatal("escrita ambígua não deveria gravar no escopo padrão")
	}
	if _, err := storage.ReadPage(f.ctx, f.reader, domain.NewPageRef(f.global, mustWritePath(t, path))); err == nil {
		t.Fatal("escrita ambígua não deveria gravar no escopo global")
	}
}

func TestWritePage_CriaEscopoComNomesNovos(t *testing.T) {
	f := newWriteFixture(t)
	out := f.invoke("memory_write_page", map[string]any{
		"workspace": "workspace-novo", "project": "projeto-novo", "path": "a.md", "body": "Primeira.",
	}).ok(t)
	if out["page_id"] == nil || out["path"] == nil {
		t.Fatalf("resposta incompleta: %v", out)
	}
}

func TestDeletePage_RemoveNoEscopoExplicitoEEhIdempotente(t *testing.T) {
	f := newWriteFixture(t)
	scratch := map[string]any{"workspace": "default", "project": "scratch"}
	const path = "notas/descartavel.md"

	written := f.invoke("memory_write_page", mergeWriteArgs(scratch, map[string]any{
		"path": path, "body": "Vai sumir.",
	})).ok(t)
	abs := filepath.Join(f.root, filepath.FromSlash(written["path"].(string)))
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("arquivo antes da remoção: %v", err)
	}

	del := f.invoke("memory_delete_page", mergeWriteArgs(scratch, map[string]any{"path": path})).ok(t)
	if del["deleted"] != true {
		t.Fatalf("deleted = %v, quer true", del["deleted"])
	}
	if cp, _ := del["checkpoint"].(string); cp == "" {
		t.Fatalf("checkpoint posterior ausente: %v", del)
	}
	if _, err := os.Stat(abs); !os.IsNotExist(err) {
		t.Fatalf("arquivo deveria ter sido removido, stat: %v", err)
	}
	if _, err := storage.ReadPage(f.ctx, f.reader, domain.NewPageRef(f.scratch, mustWritePath(t, path))); err == nil {
		t.Fatal("página removida ainda aparece no banco")
	}

	// Segunda remoção: página ausente conta como removida, sem erro.
	again := f.invoke("memory_delete_page", mergeWriteArgs(scratch, map[string]any{"path": path}))
	if again.isError {
		t.Fatalf("remoção repetida deveria ser idempotente, veio %q", again.text)
	}
	if again.ok(t)["deleted"] != true {
		t.Fatalf("remoção repetida deveria responder deleted true")
	}
}

func TestDeletePage_EscopoGlobal(t *testing.T) {
	f := newWriteFixture(t)
	const path = "regras/temporaria.md"

	f.invoke("memory_write_page", map[string]any{"scope": "global", "path": path, "body": "Temporária."}).ok(t)
	del := f.invoke("memory_delete_page", map[string]any{"scope": "global", "path": path}).ok(t)
	if del["deleted"] != true {
		t.Fatalf("deleted = %v, quer true", del["deleted"])
	}
	if _, err := storage.ReadPage(f.ctx, f.reader, domain.NewPageRef(f.global, mustWritePath(t, path))); err == nil {
		t.Fatal("página global removida ainda aparece no banco")
	}
}

func TestDeletePage_EscopoExplicitoInexistenteFalhaFechado(t *testing.T) {
	f := newWriteFixture(t)
	out := f.invoke("memory_delete_page", map[string]any{
		"workspace": "default", "project": "nao-existe", "path": "a.md",
	})
	if !out.isError {
		t.Fatalf("exclusão em escopo inexistente deveria falhar fechado, veio %q", out.text)
	}
	if !strings.Contains(out.text, "não") {
		t.Fatalf("mensagem de não encontrado inesperada: %q", out.text)
	}
}

func TestWritePage_EntradasInvalidasSaoRecusadas(t *testing.T) {
	f := newWriteFixture(t)
	scratch := map[string]any{"workspace": "default", "project": "scratch"}
	cases := []struct {
		name string
		args map[string]any
	}{
		{"global com workspace", map[string]any{"scope": "global", "workspace": "default", "path": "a.md", "body": "x"}},
		{"scope profile fora desta versão", map[string]any{"scope": "profile", "path": "a.md", "body": "x"}},
		{"expires_at inválido", mergeWriteArgs(scratch, map[string]any{"path": "a.md", "body": "x", "expires_at": "amanha"})},
		{"tier inválido", mergeWriteArgs(scratch, map[string]any{"path": "a.md", "body": "x", "tier": "eterno"})},
		{"caminho com travessia", mergeWriteArgs(scratch, map[string]any{"path": "../fora.md", "body": "x"})},
		{"sem body", mergeWriteArgs(scratch, map[string]any{"path": "a.md"})},
		{"só workspace", map[string]any{"workspace": "default", "path": "a.md", "body": "x"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if out := f.invoke("memory_write_page", tc.args); !out.isError {
				t.Fatalf("deveria ser recusada, veio %q", out.text)
			}
		})
	}
}

func TestRegisterWriteTools_ExigeDependencias(t *testing.T) {
	if err := RegisterWriteTools(nil, WriteDeps{}); err == nil {
		t.Fatal("servidor nulo deveria falhar")
	}
	if err := RegisterWriteTools(NewToolServer(), WriteDeps{}); err == nil {
		t.Fatal("store nulo deveria falhar")
	}
}

// mergeWriteArgs junta os argumentos de escopo aos argumentos da chamada, sem alterar o mapa de escopo.
func mergeWriteArgs(scope, extra map[string]any) map[string]any {
	out := make(map[string]any, len(scope)+len(extra))
	for k, v := range scope {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}
