package wiki

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
	"github.com/phm-aguiar/ai-memory-go/internal/storage"
)

// rxFreshStore abre um banco novo (vazio) sobre a mesma raiz da wiki, com Writer real. É o cenário do
// reindex depois de o banco ser perdido.
func rxFreshStore(t *testing.T, root string, clock *time.Time) (*Store, *sql.DB) {
	t.Helper()
	ctx := context.Background()
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
	return &Store{
		Root:   root,
		Writer: w,
		Locks:  NewPageLocks(),
		Now:    func() time.Time { return *clock },
	}, db
}

// rxWriteRaw grava um arquivo cru dentro do projeto do env, sem passar por WritePage.
func rxWriteRaw(t *testing.T, env *wpEnv, rel, content string) string {
	t.Helper()
	abs := wpAbs(env, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", rel, err)
	}
	return abs
}

// rxSnapshot resume o conteúdo do banco em texto comparável: cada versão (com a contagem de versões do
// caminho) e cada vínculo de entidade da versão corrente, com as janelas de validade.
func rxSnapshot(t *testing.T, db *sql.DB) string {
	t.Helper()
	var b strings.Builder
	rows, err := db.Query(`SELECT p.path, p.is_latest, p.title, p.tier, p.body, p.frontmatter_json, p.pinned,
		(SELECT COUNT(*) FROM pages q WHERE q.workspace_id = p.workspace_id AND q.project_id = p.project_id AND q.path = p.path)
		FROM pages p ORDER BY p.path, p.is_latest`)
	if err != nil {
		t.Fatalf("query pages: %v", err)
	}
	for rows.Next() {
		var path, title, tier, body, fmJSON string
		var latest, pinned, versions int
		if err := rows.Scan(&path, &latest, &title, &tier, &body, &fmJSON, &pinned, &versions); err != nil {
			t.Fatalf("scan page: %v", err)
		}
		fmt.Fprintf(&b, "page|%s|%d|%s|%s|%q|%s|%d|%d\n", path, latest, title, tier, body, fmJSON, pinned, versions)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows pages: %v", err)
	}
	_ = rows.Close()

	ent, err := db.Query(`SELECT p.path, e.name, l.valid_from, COALESCE(l.superseded_at, -1)
		FROM entity_page_links l JOIN entities e ON e.id = l.entity_id JOIN pages p ON p.id = l.page_id
		ORDER BY p.path, e.name`)
	if err != nil {
		t.Fatalf("query entities: %v", err)
	}
	for ent.Next() {
		var path, name string
		var validFrom, supersededAt int64
		if err := ent.Scan(&path, &name, &validFrom, &supersededAt); err != nil {
			t.Fatalf("scan entity: %v", err)
		}
		fmt.Fprintf(&b, "entity|%s|%s|%d|%d\n", path, name, validFrom, supersededAt)
	}
	if err := ent.Err(); err != nil {
		t.Fatalf("rows entities: %v", err)
	}
	_ = ent.Close()
	return b.String()
}

// rxLatestPaths lista os caminhos da versão corrente de cada página, em ordem.
func rxLatestPaths(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT path FROM pages WHERE is_latest = 1 ORDER BY path`)
	if err != nil {
		t.Fatalf("query paths: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			t.Fatalf("scan path: %v", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows paths: %v", err)
	}
	return out
}

// rxFrontmatter monta um frontmatter com pares chave/valor de texto, na ordem recebida.
func rxFrontmatter(t *testing.T, pairs ...any) domain.Frontmatter {
	t.Helper()
	var fm domain.Frontmatter
	for i := 0; i < len(pairs); i += 2 {
		if err := fm.Set(pairs[i].(string), pairs[i+1]); err != nil {
			t.Fatalf("Set(%v): %v", pairs[i], err)
		}
	}
	return fm
}

func TestReindexReconstroiBancoComoAWikiEscrita(t *testing.T) {
	env := wpNewEnv(t)
	ctx := env.ctx

	inA := wpInput(t, env, "notas/a.md", "Corpo da nota A.\n")
	inA.Frontmatter = rxFrontmatter(t, "entities", []string{"Go", "SQLite"}, "summary", "Resumo da A")
	if _, err := env.store.WritePage(ctx, inA); err != nil {
		t.Fatalf("WritePage a: %v", err)
	}
	inC := wpInput(t, env, "conceitos/c.md", "Corpo do conceito.\n")
	inC.Kind = domain.PageKindFact
	inC.Frontmatter = rxFrontmatter(t, "pinned", "true", "tags", []string{"Go"})
	if _, err := env.store.WritePage(ctx, inC); err != nil {
		t.Fatalf("WritePage c: %v", err)
	}

	// Mesmo banco, arquivos iguais: nenhuma página nova, nenhuma versão nova.
	rep, err := Reindex(ctx, env.store, env.root)
	if err != nil {
		t.Fatalf("Reindex no próprio banco: %v", err)
	}
	if rep.Scopes != 1 || rep.Pages != 2 || rep.Unchanged != 2 || len(rep.Errors) != 0 {
		t.Fatalf("reindex sobre o banco atual: %+v", rep)
	}
	before := rxSnapshot(t, env.db)

	// Banco novo: a reconstrução tem de produzir o mesmo conteúdo, versões e entidades.
	store2, db2 := rxFreshStore(t, env.root, env.clock)
	rep2, err := Reindex(ctx, store2, env.root)
	if err != nil {
		t.Fatalf("Reindex em banco novo: %v", err)
	}
	if rep2.Scopes != 1 || rep2.Pages != 2 || rep2.Unchanged != 0 || len(rep2.Errors) != 0 || len(rep2.Avisos) != 0 {
		t.Fatalf("reindex em banco novo: %+v", rep2)
	}
	if after := rxSnapshot(t, db2); after != before {
		t.Fatalf("banco reconstruído difere do original:\n--- original\n%s\n--- reconstruído\n%s", before, after)
	}
}

// rxMetaPaths devolve os caminhos do _meta.md do workspace e do projeto do env.
func rxMetaPaths(env *wpEnv) (string, string) {
	wsDir := filepath.Join(env.root, env.scope.WorkspaceID.String())
	projDir := filepath.Join(wsDir, env.scope.ProjectID.String())
	return filepath.Join(wsDir, "_meta.md"), filepath.Join(projDir, "_meta.md")
}

// rxHasErrorWithPath diz se alguma mensagem de Errors traz o caminho exato do arquivo.
func rxHasErrorWithPath(errs []string, path string) bool {
	for _, e := range errs {
		if strings.Contains(e, path) {
			return true
		}
	}
	return false
}

// rxAssertNoScopeIndexed confere que o banco não ganhou workspace, projeto nem página: o nome não foi
// reconstruído pelo id e o escopo não foi criado.
func rxAssertNoScopeIndexed(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, q := range []string{
		`SELECT COUNT(*) FROM workspaces`,
		`SELECT COUNT(*) FROM projects`,
		`SELECT COUNT(*) FROM pages WHERE is_latest = 1`,
	} {
		var n int
		if err := db.QueryRow(q).Scan(&n); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		if n != 0 {
			t.Fatalf("%s = %d, esperado 0 (escopo sem _meta.md não é reconstruído)", q, n)
		}
	}
}

func TestReindexMetaAusenteFalhaComCaminhoExato(t *testing.T) {
	env := wpNewEnv(t)
	ctx := env.ctx
	if _, err := env.store.WritePage(ctx, wpInput(t, env, "notas/a.md", "Corpo.\n")); err != nil {
		t.Fatalf("WritePage: %v", err)
	}
	wsMeta, projMeta := rxMetaPaths(env)
	for _, p := range []string{wsMeta, projMeta} {
		if err := os.Remove(p); err != nil {
			t.Fatalf("remover %s: %v", p, err)
		}
	}

	store2, db2 := rxFreshStore(t, env.root, env.clock)
	rep, err := Reindex(ctx, store2, env.root)
	if err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	if rep.Scopes != 0 || rep.Pages != 0 {
		t.Fatalf("escopo sem _meta.md não devia ser indexado: %+v", rep)
	}
	if len(rep.Errors) != 2 {
		t.Fatalf("Errors = %v, esperado um erro por _meta.md ausente", rep.Errors)
	}
	for _, p := range []string{wsMeta, projMeta} {
		if !rxHasErrorWithPath(rep.Errors, p) {
			t.Errorf("Errors = %v, esperado o caminho exato %s", rep.Errors, p)
		}
	}
	if len(rep.Avisos) != 0 {
		t.Fatalf("Avisos = %v, esperado nenhum: não há mais reconstrução pelo id", rep.Avisos)
	}
	rxAssertNoScopeIndexed(t, db2)
	if _, err := os.Stat(projMeta); !os.IsNotExist(err) {
		t.Fatalf("reindex não deve gravar _meta.md; stat err = %v", err)
	}
}

func TestReindexMetaInvalidoFalhaComCaminhoExato(t *testing.T) {
	env := wpNewEnv(t)
	ctx := env.ctx
	if _, err := env.store.WritePage(ctx, wpInput(t, env, "notas/a.md", "Corpo.\n")); err != nil {
		t.Fatalf("WritePage: %v", err)
	}
	_, projMeta := rxMetaPaths(env)
	wsID := env.scope.WorkspaceID.String()
	projID := env.scope.ProjectID.String()
	cases := map[string]string{
		"yaml quebrado":      "---\nname: [\n---\n",
		"name ausente":       "---\nworkspace_id: " + wsID + "\nproject_id: " + projID + "\n---\n",
		"sem frontmatter":    "Sem frontmatter.\n",
		"identidade trocada": "---\nname: Outro\nworkspace_id: " + wsID + "\nproject_id: 00000000-0000-7000-8000-000000000000\n---\n",
	}
	for nome, conteudo := range cases {
		t.Run(nome, func(t *testing.T) {
			if err := os.WriteFile(projMeta, []byte(conteudo), 0o644); err != nil {
				t.Fatalf("WriteFile %s: %v", projMeta, err)
			}
			store2, db2 := rxFreshStore(t, env.root, env.clock)
			rep, err := Reindex(ctx, store2, env.root)
			if err != nil {
				t.Fatalf("Reindex: %v", err)
			}
			if len(rep.Errors) != 1 || !rxHasErrorWithPath(rep.Errors, projMeta) {
				t.Fatalf("Errors = %v, esperado um erro com o caminho exato %s", rep.Errors, projMeta)
			}
			if rep.Scopes != 0 || rep.Pages != 0 {
				t.Fatalf("escopo com _meta.md inválido não devia ser indexado: %+v", rep)
			}
			rxAssertNoScopeIndexed(t, db2)
		})
	}
}

func TestReindexIgnoraControlesEArquivosNaoMarkdown(t *testing.T) {
	env := wpNewEnv(t)
	ctx := env.ctx
	if _, err := env.store.WritePage(ctx, wpInput(t, env, "notas/a.md", "Corpo.\n")); err != nil {
		t.Fatalf("WritePage: %v", err)
	}
	rxWriteRaw(t, env, "log.md", "## [2026-03-01T10:00:00Z] page-write | x\n")
	rxWriteRaw(t, env, "log-2026-03.md", "## [2026-03-01T10:00:00Z] page-write | x\n")
	rxWriteRaw(t, env, "bootstrap.md", "Boot.\n")
	rxWriteRaw(t, env, "index.md", "# Índice\n")
	rxWriteRaw(t, env, "_pending/x.md", "Proposta.\n")
	rxWriteRaw(t, env, "_graveyard/y.md", "Descarte.\n")
	rxWriteRaw(t, env, "nota.txt", "texto\n")
	rxWriteRaw(t, env, ".ai-memory-tmp.z.md", "temporário\n")
	rxWriteRaw(t, env, "notas/.ai-memory-tmp.w.md", "temporário\n")
	rxWriteRaw(t, env, ".git/g.md", "git\n")
	// Diretório oculto que não é controle não é pulado (WFL §8.2).
	rxWriteRaw(t, env, ".oculto/z.md", "Visível.\n")
	// Link simbólico não é seguido. Se o sistema não permite criá-lo, o caso fica coberto pelos demais.
	if err := os.Symlink(wpAbs(env, "notas/a.md"), wpAbs(env, "notas/link.md")); err != nil {
		t.Logf("symlink indisponível neste SO: %v", err)
	}

	store2, db2 := rxFreshStore(t, env.root, env.clock)
	rep, err := Reindex(ctx, store2, env.root)
	if err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	if rep.Pages != 2 || len(rep.Errors) != 0 {
		t.Fatalf("reindex com ignorados: %+v", rep)
	}
	got := strings.Join(rxLatestPaths(t, db2), ",")
	if got != ".oculto/z.md,notas/a.md" {
		t.Fatalf("páginas indexadas = %q, esperado só .oculto/z.md e notas/a.md", got)
	}
}

func TestReindexErroDeYAMLVaiParaErrorsEReindexSegue(t *testing.T) {
	env := wpNewEnv(t)
	ctx := env.ctx
	if _, err := env.store.WritePage(ctx, wpInput(t, env, "notas/ok.md", "Bom.\n")); err != nil {
		t.Fatalf("WritePage: %v", err)
	}
	rxWriteRaw(t, env, "notas/ruim.md", "---\nkey: [\n---\ncorpo\n")

	store2, db2 := rxFreshStore(t, env.root, env.clock)
	rep, err := Reindex(ctx, store2, env.root)
	if err != nil {
		t.Fatalf("Reindex não deveria falhar por arquivo ruim: %v", err)
	}
	if rep.Pages != 1 {
		t.Fatalf("Pages = %d, esperado 1", rep.Pages)
	}
	if len(rep.Errors) != 1 || !strings.Contains(rep.Errors[0], "notas/ruim.md") {
		t.Fatalf("Errors = %v, esperado um erro citando notas/ruim.md", rep.Errors)
	}
	if got := strings.Join(rxLatestPaths(t, db2), ","); got != "notas/ok.md" {
		t.Fatalf("páginas indexadas = %q, esperado notas/ok.md", got)
	}
}

func TestReindexTierEKindDoFrontmatter(t *testing.T) {
	env := wpNewEnv(t)
	ctx := env.ctx
	rxWriteRaw(t, env, "notas/sem-tier.md", "Sem frontmatter.\n")
	rxWriteRaw(t, env, "notas/com-kind.md", "---\nkind: fact\ntier: procedural\n---\nCorpo.\n")
	rxWriteRaw(t, env, "notas/kind-invalido.md", "---\nkind: \"x\\ny\"\n---\nCorpo.\n")
	rxWriteRaw(t, env, "notas/tier-ruim.md", "---\ntier: bogus\n---\nCorpo.\n")

	store, db := rxFreshStore(t, env.root, env.clock)
	// O primeiro reindex precisa do escopo existente: o WritePage cria o _meta.md e o escopo.
	if _, err := env.store.WritePage(ctx, wpInput(t, env, "notas/semente.md", "Semente.\n")); err != nil {
		t.Fatalf("WritePage semente: %v", err)
	}
	rep, err := Reindex(ctx, store, env.root)
	if err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	if len(rep.Errors) != 1 || !strings.Contains(rep.Errors[0], "notas/tier-ruim.md") {
		t.Fatalf("Errors = %v, esperado só o tier inválido", rep.Errors)
	}
	tiers := map[string]string{}
	rows, err := db.Query(`SELECT path, tier FROM pages WHERE is_latest = 1`)
	if err != nil {
		t.Fatalf("query tiers: %v", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var p, tier string
		if err := rows.Scan(&p, &tier); err != nil {
			t.Fatalf("scan tier: %v", err)
		}
		tiers[p] = tier
	}
	if tiers["notas/sem-tier.md"] != string(domain.TierSemantic) {
		t.Fatalf("tier sem frontmatter = %q, esperado semantic", tiers["notas/sem-tier.md"])
	}
	if tiers["notas/com-kind.md"] != string(domain.TierProcedural) {
		t.Fatalf("tier do frontmatter = %q, esperado procedural", tiers["notas/com-kind.md"])
	}
	if _, ok := tiers["notas/tier-ruim.md"]; ok {
		t.Fatalf("página com tier inválido não devia ser indexada")
	}
	kindRef := wpRef(t, env, "notas/com-kind.md")
	page, err := storage.ReadPage(ctx, storage.NewReader(db), kindRef)
	if err != nil {
		t.Fatalf("ReadPage com-kind: %v", err)
	}
	if page.Kind != domain.PageKindFact {
		t.Fatalf("kind = %q, esperado fact (vindo da chave kind do frontmatter)", page.Kind)
	}
}

// TestReindexIndexaFrontmatterAninhado cobre a reindexação de página com generated (mapeamento),
// sources (lista de mapeamentos) e relations (mapeamento de listas): o índice guarda os três em
// todos os níveis, sem erro de tipo.
func TestReindexIndexaFrontmatterAninhado(t *testing.T) {
	env := wpNewEnv(t)
	ctx := env.ctx
	if _, err := env.store.WritePage(ctx, wpInput(t, env, "notas/semente.md", "Semente.\n")); err != nil {
		t.Fatalf("WritePage semente: %v", err)
	}
	rxWriteRaw(t, env, "notas/aninhado.md", "---\n"+
		"generated:\n  by: process:ai-memory/test\n  at: \"2026-03-01T10:00:00Z\"\n"+
		"sources:\n  - kind: session\n    id: sess-1\n  - kind: session\n    id: sess-2\n    note: segunda\n"+
		"relations:\n  causes:\n    - notas/semente.md\n"+
		"---\nCorpo.\n")

	store, db := rxFreshStore(t, env.root, env.clock)
	rep, err := Reindex(ctx, store, env.root)
	if err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	if len(rep.Errors) != 0 {
		t.Fatalf("Errors = %v, esperado nenhum", rep.Errors)
	}

	page, err := storage.ReadPage(ctx, storage.NewReader(db), wpRef(t, env, "notas/aninhado.md"))
	if err != nil {
		t.Fatalf("ReadPage aninhado: %v", err)
	}
	gv, ok := page.Frontmatter.Get("generated")
	gen, isMap := gv.(domain.Frontmatter)
	if !ok || !isMap {
		t.Fatalf("generated = %T, esperado mapeamento", gv)
	}
	if by, _ := gen.Get("by"); by != "process:ai-memory/test" {
		t.Fatalf("generated.by = %v", by)
	}
	sv, ok := page.Frontmatter.Get("sources")
	list, isList := sv.([]domain.Frontmatter)
	if !ok || !isList || len(list) != 2 {
		t.Fatalf("sources = %T %v, esperada lista de 2 mapeamentos", sv, sv)
	}
	if note, _ := list[1].Get("note"); note != "segunda" {
		t.Fatalf("sources[1].note = %v, esperado segunda", note)
	}
	rv, ok := page.Frontmatter.Get("relations")
	if _, isMap := rv.(domain.Frontmatter); !ok || !isMap {
		t.Fatalf("relations = %T, esperado mapeamento", rv)
	}
}

func TestReindexIndexFileNaoVersionaConteudoIdentico(t *testing.T) {
	env := wpNewEnv(t)
	ctx := env.ctx
	if _, err := env.store.WritePage(ctx, wpInput(t, env, "notas/a.md", "v1\n")); err != nil {
		t.Fatalf("WritePage v1: %v", err)
	}
	if _, err := env.store.WritePage(ctx, wpInput(t, env, "notas/a.md", "v2\n")); err != nil {
		t.Fatalf("WritePage v2: %v", err)
	}
	ref := wpRef(t, env, "notas/a.md")
	abs := wpAbs(env, "notas/a.md")

	chain, err := storage.PageChain(ctx, env.reader, ref)
	if err != nil || len(chain) != 2 {
		t.Fatalf("cadeia antes = %d (err %v), esperado 2", len(chain), err)
	}
	fileBefore, err := os.ReadFile(abs)
	if err != nil {
		t.Fatalf("ler arquivo: %v", err)
	}

	if err := IndexFile(ctx, env.store, ref, abs); err != nil {
		t.Fatalf("IndexFile conteúdo igual: %v", err)
	}
	chain, err = storage.PageChain(ctx, env.reader, ref)
	if err != nil || len(chain) != 2 {
		t.Fatalf("cadeia depois de conteúdo igual = %d (err %v), esperado 2", len(chain), err)
	}
	outcome, _, err := indexFile(ctx, env.store, ref, abs)
	if err != nil || outcome != storage.UpsertUnchanged {
		t.Fatalf("outcome = %q (err %v), esperado unchanged", outcome, err)
	}
	fileAfter, err := os.ReadFile(abs)
	if err != nil {
		t.Fatalf("ler arquivo depois: %v", err)
	}
	if string(fileAfter) != string(fileBefore) {
		t.Fatalf("IndexFile não deve escrever no arquivo")
	}

	// Edição manual do arquivo: o mesmo IndexFile cria a versão nova (supersessão).
	rxWriteRaw(t, env, "notas/a.md", "---\ntier: semantic\n---\nv3 editada\n")
	outcome, _, err = indexFile(ctx, env.store, ref, abs)
	if err != nil || outcome != storage.UpsertSuperseded {
		t.Fatalf("outcome após edição = %q (err %v), esperado superseded", outcome, err)
	}
	chain, err = storage.PageChain(ctx, env.reader, ref)
	if err != nil || len(chain) != 3 {
		t.Fatalf("cadeia após edição = %d (err %v), esperado 3", len(chain), err)
	}
}

func TestReindexRejeitaCaminhoForaDaPagina(t *testing.T) {
	env := wpNewEnv(t)
	ctx := env.ctx
	if _, err := env.store.WritePage(ctx, wpInput(t, env, "notas/a.md", "Corpo.\n")); err != nil {
		t.Fatalf("WritePage: %v", err)
	}
	ref := wpRef(t, env, "notas/a.md")
	if err := IndexFile(ctx, env.store, ref, filepath.Join(env.root, "fora.md")); err == nil {
		t.Fatalf("IndexFile com caminho de outra página devia falhar")
	}
	other := t.TempDir()
	if _, err := Reindex(ctx, env.store, other); err == nil {
		t.Fatalf("Reindex com raiz diferente da do Store devia falhar")
	}
}
