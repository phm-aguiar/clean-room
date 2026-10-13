// Package contract reúne testes de contrato de invariantes da constituição (INV-06, INV-10 e
// INV-16) que atravessam pacotes. Cada teste usa só a API exportada de internal/*, banco e raiz
// da wiki em diretórios temporários, e não deixa goroutine viva ao terminar.
package contract

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
	"github.com/phm-aguiar/ai-memory-go/internal/domain/sanitize"
	"github.com/phm-aguiar/ai-memory-go/internal/storage"
	"github.com/phm-aguiar/ai-memory-go/internal/wiki"
)

// invTmpPrefix é o prefixo do temporário da escrita atômica (INV-10). Está repetido aqui porque a
// constante do pacote wiki não é exportada.
const invTmpPrefix = ".ai-memory-tmp."

// invEnv reúne o ambiente de um teste de invariante: raiz da wiki, banco temporário, escritor
// real, leitor real, repositório git e um Store. Tudo vive em t.TempDir.
type invEnv struct {
	ctx     context.Context
	root    string
	db      *sql.DB
	writer  storage.Writer
	reader  storage.Reader
	store   *wiki.Store
	scope   domain.Scope
	clock   *time.Time
	wsName  string
	prjName string
}

func invNewEnv(t *testing.T) *invEnv {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	repo, err := wiki.Open(root)
	if err != nil {
		t.Fatalf("wiki.Open: %v", err)
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
	env := &invEnv{
		ctx:     ctx,
		root:    root,
		db:      db,
		writer:  w,
		reader:  storage.NewReader(db),
		scope:   domain.NewScope(domain.WorkspaceID(ws), domain.ProjectID(pj)),
		clock:   &clock,
		wsName:  "ws-contrato",
		prjName: "proj-contrato",
	}
	env.store = &wiki.Store{
		Root:   root,
		Writer: w,
		Repo:   repo,
		Locks:  wiki.NewPageLocks(),
		Now:    func() time.Time { return *env.clock },
	}
	return env
}

// invInput monta um pedido de escrita de página com tier semantic e evidência de sessão. Se title
// não for vazio, vai para o frontmatter.
func invInput(t *testing.T, env *invEnv, path, body, title, session string) wiki.PageInput {
	t.Helper()
	p, err := domain.ParsePagePath(path)
	if err != nil {
		t.Fatalf("ParsePagePath(%q): %v", path, err)
	}
	var fm domain.Frontmatter
	if title != "" {
		if err := fm.Set("title", title); err != nil {
			t.Fatalf("Set(title): %v", err)
		}
	}
	return wiki.PageInput{
		Scope:         env.scope,
		WorkspaceName: env.wsName,
		ProjectName:   env.prjName,
		Path:          p,
		Tier:          domain.TierSemantic,
		Body:          body,
		Frontmatter:   fm,
		Evidence:      storage.EvidenceSource{Kind: storage.SourceSession, ID: session},
	}
}

// invRef é a identidade da página para leitura no banco.
func invRef(t *testing.T, env *invEnv, path string) domain.PageRef {
	t.Helper()
	p, err := domain.ParsePagePath(path)
	if err != nil {
		t.Fatalf("ParsePagePath(%q): %v", path, err)
	}
	return domain.NewPageRef(env.scope, p)
}

// invDir é o diretório da página no disco, montado só para o teste.
func invDir(env *invEnv, rel string) string {
	return filepath.Dir(filepath.Join(env.root, env.scope.WorkspaceID.String(), env.scope.ProjectID.String(), filepath.FromSlash(rel)))
}

// invAbs é o caminho absoluto da página no disco.
func invAbs(env *invEnv, rel string) string {
	return filepath.Join(env.root, env.scope.WorkspaceID.String(), env.scope.ProjectID.String(), filepath.FromSlash(rel))
}

// invFiles lista todos os arquivos da raiz da wiki, exceto o diretório .git, com o conteúdo.
func invFiles(t *testing.T, root string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" && p != root {
				return filepath.SkipDir
			}
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[p] = data
		return nil
	})
	if err != nil {
		t.Fatalf("percorrer a wiki: %v", err)
	}
	return out
}

// invDumpDB devolve o texto de todas as linhas de todas as tabelas, sqlite_master incluído.
// Valores binários entram como bytes crus, então um segredo gravado em BLOB também aparece.
func invDumpDB(t *testing.T, db *sql.DB) string {
	t.Helper()
	ctx := context.Background()
	names := []string{"sqlite_master"}
	rows, err := db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table'`)
	if err != nil {
		t.Fatalf("listar tabelas: %v", err)
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			t.Fatalf("ler nome de tabela: %v", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		t.Fatalf("iterar tabelas: %v", err)
	}
	_ = rows.Close()

	var b strings.Builder
	for _, name := range names {
		b.WriteString("\n#table " + name + "\n")
		r, err := db.QueryContext(ctx, fmt.Sprintf(`SELECT * FROM "%s"`, name))
		if err != nil {
			t.Fatalf("consultar %s: %v", name, err)
		}
		cols, err := r.Columns()
		if err != nil {
			_ = r.Close()
			t.Fatalf("colunas de %s: %v", name, err)
		}
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		for r.Next() {
			if err := r.Scan(ptrs...); err != nil {
				_ = r.Close()
				t.Fatalf("ler linha de %s: %v", name, err)
			}
			for _, v := range vals {
				switch x := v.(type) {
				case nil:
				case []byte:
					b.Write(x)
				case string:
					b.WriteString(x)
				default:
					fmt.Fprint(&b, x)
				}
				b.WriteByte('\n')
			}
		}
		if err := r.Err(); err != nil {
			_ = r.Close()
			t.Fatalf("iterar %s: %v", name, err)
		}
		_ = r.Close()
	}
	return b.String()
}

// invSecret monta um token GitHub sintético em runtime, no formato gh + p_ + 36 alfanuméricos.
// A montagem por partes evita que o literal apareça no código-fonte como segredo.
func invSecret() string {
	return "gh" + "p_" + strings.Repeat("Zx9Qw7", 6)
}

// invEnsureScope cria workspace e projeto, necessários às chaves estrangeiras de observações.
func invEnsureScope(t *testing.T, env *invEnv) {
	t.Helper()
	if err := storage.EnsureScope(env.ctx, env.writer, env.scope, env.wsName, env.prjName, *env.clock); err != nil {
		t.Fatalf("EnsureScope: %v", err)
	}
}

// invSession cria uma sessão de agente no escopo e devolve o identificador.
func invSession(t *testing.T, env *invEnv) domain.SessionID {
	t.Helper()
	id, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	err = env.writer.Do(env.ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(
			`INSERT INTO sessions (id, workspace_id, project_id, agent_kind, started_at) VALUES (?, ?, ?, ?, ?)`,
			id.Bytes(), env.scope.WorkspaceID.Bytes(), env.scope.ProjectID.Bytes(), "agente-contrato", env.clock.UnixMicro(),
		)
		return err
	})
	if err != nil {
		t.Fatalf("inserir sessão: %v", err)
	}
	return domain.SessionID(id)
}

// invInsertObs grava uma observação pelo caminho de produção: Writer.Do e InsertObservation.
func invInsertObs(t *testing.T, env *invEnv, sess domain.SessionID, title, body string) {
	t.Helper()
	id, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	obs := domain.Observation{
		ID:         domain.ObservationID(id),
		SessionID:  sess,
		Scope:      env.scope,
		Kind:       domain.ObservationPostToolUse,
		Title:      sanitize.Title(title, sanitize.Limits{TitleMaxRunes: sanitize.ObservationTitleMaxRunes}),
		Body:       sanitize.Body(body, sanitize.Limits{BodyMaxBytes: sanitize.ObservationBodyMaxBytes}),
		Importance: 5,
		OccurredAt: env.clock.UnixMicro(),
	}
	err = env.writer.Do(env.ctx, func(tx *sql.Tx) error {
		return storage.InsertObservation(tx, obs)
	})
	if err != nil {
		t.Fatalf("InsertObservation: %v", err)
	}
}

// invTmpLeftovers devolve os caminhos de arquivos temporários da escrita atômica na árvore.
func invTmpLeftovers(t *testing.T, root string) []string {
	t.Helper()
	var left []string
	for p := range invFiles(t, root) {
		if strings.HasPrefix(filepath.Base(p), invTmpPrefix) {
			left = append(left, p)
		}
	}
	return left
}

// --- INV-06 -----------------------------------------------------------------------------------

// TestINV06_SegredoNuncaChegaAoArmazenamento grava um token sintético pelos dois caminhos de
// escrita (WritePage em título e corpo, InsertObservation em título e corpo) e confere que o valor
// cru não aparece em nenhuma tabela do banco nem em nenhum arquivo da wiki. Também confere que a
// redação aconteceu (marcador presente), para o teste não passar por ausência de dados.
func TestINV06_SegredoNuncaChegaAoArmazenamento(t *testing.T) {
	env := invNewEnv(t)
	secret := invSecret()

	if _, err := env.store.WritePage(env.ctx, invInput(t, env, "notas/segredo.md",
		"# Config\n\nuso: "+secret+"\n", "Config "+secret, "sessao-seg")); err != nil {
		t.Fatalf("WritePage com segredo: %v", err)
	}

	invEnsureScope(t, env)
	sess := invSession(t, env)
	invInsertObs(t, env, sess, "token em uso "+secret, "linha de log: "+secret+"\n")

	dump := invDumpDB(t, env.db)
	if strings.Contains(dump, secret) {
		t.Fatalf("segredo cru apareceu em alguma tabela do banco")
	}
	if !strings.Contains(dump, "[REDACTED:github_token]") {
		t.Fatalf("marcador de redação ausente no banco: a sanitização não rodou")
	}

	for p, data := range invFiles(t, env.root) {
		if strings.Contains(string(data), secret) {
			t.Fatalf("segredo cru apareceu no arquivo %s", p)
		}
	}
	page, err := os.ReadFile(invAbs(env, "notas/segredo.md"))
	if err != nil {
		t.Fatalf("ler página: %v", err)
	}
	if !strings.Contains(string(page), "[REDACTED:github_token]") {
		t.Fatalf("página em disco sem marcador de redação")
	}
}

// TestINV06_RedacaoAntesDoLimiteDeCorpo cobre a ordem da constituição: o segredo é removido antes
// do corte de tamanho. O token começa dentro dos últimos 10 bytes antes do teto de 16 KiB, e há um
// espaço antes dele para que o padrão `\b` case (o padrão exige fronteira de palavra). Se o corte
// viesse antes da redação, o token ficaria partido e escaparia.
func TestINV06_RedacaoAntesDoLimiteDeCorpo(t *testing.T) {
	env := invNewEnv(t)
	invEnsureScope(t, env)
	sess := invSession(t, env)
	secret := invSecret()
	body := strings.Repeat("a", sanitize.ObservationBodyMaxBytes-11) + " " + secret

	invInsertObs(t, env, sess, "limite", body)

	dump := invDumpDB(t, env.db)
	if strings.Contains(dump, secret) {
		t.Fatalf("segredo cru sobreviveu ao corte de tamanho")
	}
	if strings.Contains(dump, "gh"+"p_") {
		t.Fatalf("fragmento do token sobreviveu ao corte de tamanho")
	}
}

// TestINV06_CorpoLimitadoA16KiB grava uma observação com corpo de 20 KiB e confere que o valor
// armazenado tem no máximo 16384 bytes. Também confere que os tetos da sanitização são os da
// constituição (80 runas no título, 16 KiB no corpo).
func TestINV06_CorpoLimitadoA16KiB(t *testing.T) {
	if sanitize.ObservationTitleMaxRunes != 80 {
		t.Fatalf("ObservationTitleMaxRunes = %d, quer 80", sanitize.ObservationTitleMaxRunes)
	}
	if sanitize.ObservationBodyMaxBytes != 16*1024 {
		t.Fatalf("ObservationBodyMaxBytes = %d, quer 16384", sanitize.ObservationBodyMaxBytes)
	}

	env := invNewEnv(t)
	invEnsureScope(t, env)
	sess := invSession(t, env)
	invInsertObs(t, env, sess, "corpo grande", strings.Repeat("corpo ", 20*1024/6+1))

	var stored string
	err := env.db.QueryRowContext(env.ctx, `SELECT body FROM observations`).Scan(&stored)
	if err != nil {
		t.Fatalf("ler corpo armazenado: %v", err)
	}
	if len(stored) > 16384 {
		t.Fatalf("corpo armazenado com %d bytes, quer no máximo 16384", len(stored))
	}
	if len(stored) == 0 {
		t.Fatalf("corpo armazenado vazio")
	}
}

// TestINV06_TituloLimitadoA80Runas grava um título de 200 runas e confere que o armazenado tem no
// máximo 80 runas.
func TestINV06_TituloLimitadoA80Runas(t *testing.T) {
	env := invNewEnv(t)
	invEnsureScope(t, env)
	sess := invSession(t, env)
	invInsertObs(t, env, sess, strings.Repeat("á", 200), "corpo curto")

	var stored string
	err := env.db.QueryRowContext(env.ctx, `SELECT title FROM observations`).Scan(&stored)
	if err != nil {
		t.Fatalf("ler título armazenado: %v", err)
	}
	if n := utf8.RuneCountInString(stored); n > 80 {
		t.Fatalf("título armazenado com %d runas, quer no máximo 80", n)
	}
}

// --- INV-10 -----------------------------------------------------------------------------------

// TestINV10_EscritaAtomicaSemTemporarioSobrando grava uma página duas vezes no mesmo caminho e
// confere, após cada escrita, que nenhum arquivo `.ai-memory-tmp.*` sobrou na árvore nem no
// diretório da página, e que o arquivo final tem o corpo exato escrito.
func TestINV10_EscritaAtomicaSemTemporarioSobrando(t *testing.T) {
	env := invNewEnv(t)

	bodies := []string{"# Atomica\n\nprimeira versão\n", "# Atomica\n\nsegunda versão\n"}
	for i, body := range bodies {
		if i > 0 {
			*env.clock = env.clock.Add(time.Minute)
		}
		if _, err := env.store.WritePage(env.ctx, invInput(t, env, "notas/atomica.md", body, "", "sessao-at")); err != nil {
			t.Fatalf("WritePage %d: %v", i+1, err)
		}
		if left := invTmpLeftovers(t, env.root); len(left) != 0 {
			t.Fatalf("temporários sobrando após escrita %d: %v", i+1, left)
		}
		entries, err := os.ReadDir(invDir(env, "notas/atomica.md"))
		if err != nil {
			t.Fatalf("ler diretório da página: %v", err)
		}
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), invTmpPrefix) {
				t.Fatalf("temporário %s sobrou no diretório da página", e.Name())
			}
		}

		data, err := os.ReadFile(invAbs(env, "notas/atomica.md"))
		if err != nil {
			t.Fatalf("ler página: %v", err)
		}
		_, got, err := wiki.ReadMarkdown(data)
		if err != nil {
			t.Fatalf("ReadMarkdown: %v", err)
		}
		if got != body {
			t.Fatalf("corpo em disco %q, quer %q", got, body)
		}
	}
}

// TestINV10_WikiWriteUsaRenameSemTemporario exercita wiki.Write diretamente. Confere que o arquivo
// final tem o conteúdo exato após uma substituição, que o diretório fica só com o alvo, e que uma
// falha antes de criar o temporário devolve erro sem deixar nada para trás.
func TestINV10_WikiWriteUsaRenameSemTemporario(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "alvo.md")

	if _, err := wiki.Write(target, []byte("conteúdo antigo\n")); err != nil {
		t.Fatalf("Write (primeira): %v", err)
	}
	want := []byte("conteúdo novo, exato\n\x00fim")
	if _, err := wiki.Write(target, want); err != nil {
		t.Fatalf("Write (substituição): %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ler alvo: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("conteúdo final %q, quer %q", got, want)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ler diretório: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "alvo.md" {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("diretório com %v, quer só alvo.md", names)
	}

	missing := filepath.Join(dir, "nao-existe", "x.md")
	if _, err := wiki.Write(missing, []byte("x")); err == nil {
		t.Fatalf("Write em diretório inexistente deveria falhar")
	}
	entries, err = os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ler diretório após falha: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("falha deixou %d entradas no diretório, quer 1", len(entries))
	}
}

// --- INV-16 -----------------------------------------------------------------------------------

// TestINV16_SupersedeNuncaDestroi escreve duas versões sequenciais no mesmo caminho e confere que
// as duas linhas continuam no banco, que a versão perdedora mantém seu corpo, e que PageChain a
// devolve como alcançável.
func TestINV16_SupersedeNuncaDestroi(t *testing.T) {
	env := invNewEnv(t)
	const path = "notas/mud.md"

	first, err := env.store.WritePage(env.ctx, invInput(t, env, path, "# Mud\n\nversão um\n", "", "sessao-1"))
	if err != nil {
		t.Fatalf("WritePage v1: %v", err)
	}
	*env.clock = env.clock.Add(time.Minute)
	second, err := env.store.WritePage(env.ctx, invInput(t, env, path, "# Mud\n\nversão dois\n", "", "sessao-1"))
	if err != nil {
		t.Fatalf("WritePage v2: %v", err)
	}
	if second.Outcome != storage.UpsertSuperseded {
		t.Fatalf("Outcome = %q, quer superseded", second.Outcome)
	}

	ref := invRef(t, env, path)
	var rows int
	err = env.db.QueryRowContext(env.ctx,
		`SELECT COUNT(*) FROM pages WHERE workspace_id = ? AND project_id = ? AND path = ?`,
		ref.Scope.WorkspaceID.Bytes(), ref.Scope.ProjectID.Bytes(), ref.Path.String(),
	).Scan(&rows)
	if err != nil {
		t.Fatalf("contar versões: %v", err)
	}
	if rows != 2 {
		t.Fatalf("linhas do caminho = %d, quer 2 (nenhuma versão pode ser destruída)", rows)
	}

	var loserBody string
	err = env.db.QueryRowContext(env.ctx, `SELECT body FROM pages WHERE id = ?`, first.Version.Bytes()).Scan(&loserBody)
	if err != nil {
		t.Fatalf("ler versão perdedora: %v", err)
	}
	if !strings.Contains(loserBody, "versão um") {
		t.Fatalf("corpo da versão perdedora = %q, quer o da versão um", loserBody)
	}

	chain, err := storage.PageChain(env.ctx, env.reader, ref)
	if err != nil {
		t.Fatalf("PageChain: %v", err)
	}
	if len(chain) != 2 {
		t.Fatalf("cadeia com %d versões, quer 2", len(chain))
	}
	if chain[0].ID != second.Version || !chain[0].IsLatest {
		t.Fatalf("primeira da cadeia não é a corrente v2")
	}
	if chain[1].ID != first.Version || chain[1].IsLatest {
		t.Fatalf("versão perdedora não aparece como superseded na cadeia")
	}
}

// TestINV16_ReadPageDevolveCorrenteSemFiltrarAutoria faz duas sessões (duas instâncias de Store
// com o mesmo Writer, a mesma raiz e as mesmas travas) escreverem o mesmo caminho. ReadPage recebe
// só a referência do caminho, sem sessão nem autor, e devolve a versão corrente da segunda sessão.
// A versão da primeira continua na cadeia, alcançável.
func TestINV16_ReadPageDevolveCorrenteSemFiltrarAutoria(t *testing.T) {
	env := invNewEnv(t)
	const path = "notas/dupla.md"

	sessaoA := *env.store
	sessaoB := *env.store
	sessaoB.Now = func() time.Time { return env.clock.Add(time.Minute) }

	first, err := sessaoA.WritePage(env.ctx, invInput(t, env, path, "# Dupla\n\nda sessão A\n", "", "sessao-A"))
	if err != nil {
		t.Fatalf("WritePage sessão A: %v", err)
	}
	second, err := sessaoB.WritePage(env.ctx, invInput(t, env, path, "# Dupla\n\nda sessão B\n", "", "sessao-B"))
	if err != nil {
		t.Fatalf("WritePage sessão B: %v", err)
	}
	if second.Outcome != storage.UpsertSuperseded {
		t.Fatalf("Outcome = %q, quer superseded", second.Outcome)
	}

	ref := invRef(t, env, path)
	page, err := storage.ReadPage(env.ctx, env.reader, ref)
	if err != nil {
		t.Fatalf("ReadPage: %v", err)
	}
	if page.Version != second.Version {
		t.Fatalf("ReadPage devolveu versão %s, quer a corrente %s", page.Version, second.Version)
	}
	if page.Body != "# Dupla\n\nda sessão B\n" {
		t.Fatalf("corpo corrente %q", page.Body)
	}

	chain, err := storage.PageChain(env.ctx, env.reader, ref)
	if err != nil {
		t.Fatalf("PageChain: %v", err)
	}
	found := false
	for _, v := range chain {
		if v.ID == first.Version {
			found = true
		}
	}
	if !found {
		t.Fatalf("versão da sessão A não é alcançável pela cadeia")
	}
}

// TestINV16_EscritaConcorrenteDivergenteSupersede dispara duas escritas divergentes no mesmo caminho
// ao mesmo tempo, cada uma por uma sessão. O resultado tem de ser uma criação e uma supersessão,
// com duas linhas no banco e os dois corpos preservados. Nenhuma escrita pode destruir a outra.
// As goroutines terminam antes do retorno do teste (WaitGroup).
func TestINV16_EscritaConcorrenteDivergenteSupersede(t *testing.T) {
	env := invNewEnv(t)
	const path = "notas/corrida.md"

	sessaoA := *env.store
	sessaoA.Now = func() time.Time { return env.clock.Add(0) }
	sessaoB := *env.store
	sessaoB.Now = func() time.Time { return env.clock.Add(time.Minute) }

	inA := invInput(t, env, path, "# Corrida\n\nlado A\n", "", "sessao-A")
	inB := invInput(t, env, path, "# Corrida\n\nlado B\n", "", "sessao-B")

	type result struct {
		res wiki.WriteResult
		err error
	}
	stores := []*wiki.Store{&sessaoA, &sessaoB}
	inputs := []wiki.PageInput{inA, inB}
	out := make([]result, len(stores))

	var wg sync.WaitGroup
	for i := range stores {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			res, err := stores[i].WritePage(env.ctx, inputs[i])
			out[i] = result{res: res, err: err}
		}(i)
	}
	wg.Wait()

	created, superseded := 0, 0
	for i, r := range out {
		if r.err != nil {
			t.Fatalf("WritePage concorrente %d: %v", i, r.err)
		}
		switch r.res.Outcome {
		case storage.UpsertCreated:
			created++
		case storage.UpsertSuperseded:
			superseded++
		default:
			t.Fatalf("Outcome concorrente %d = %q, quer created ou superseded", i, r.res.Outcome)
		}
	}
	if created != 1 || superseded != 1 {
		t.Fatalf("resultados: %d created e %d superseded, quer 1 e 1", created, superseded)
	}

	ref := invRef(t, env, path)
	var rows int
	err := env.db.QueryRowContext(env.ctx,
		`SELECT COUNT(*) FROM pages WHERE workspace_id = ? AND project_id = ? AND path = ?`,
		ref.Scope.WorkspaceID.Bytes(), ref.Scope.ProjectID.Bytes(), ref.Path.String(),
	).Scan(&rows)
	if err != nil {
		t.Fatalf("contar versões: %v", err)
	}
	if rows != 2 {
		t.Fatalf("linhas do caminho = %d, quer 2", rows)
	}
	dump := invDumpDB(t, env.db)
	if !strings.Contains(dump, "lado A") || !strings.Contains(dump, "lado B") {
		t.Fatalf("um dos corpos divergentes foi perdido no banco")
	}
	if left := invTmpLeftovers(t, env.root); len(left) != 0 {
		t.Fatalf("temporários sobrando após escrita concorrente: %v", left)
	}
}
