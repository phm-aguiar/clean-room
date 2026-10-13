package acceptance

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
	"github.com/phm-aguiar/ai-memory-go/internal/storage"
	"github.com/phm-aguiar/ai-memory-go/internal/wiki"
)

// Parâmetros do cenário 7. O debounce é o de 300 ms da feature. A reconciliação é curta e
// injetada, para que o teste não espere o ciclo de 30 s. Quando o watcher não captura a edição,
// a reconciliação cobre, e o teste passa do mesmo jeito (o cenário diz "watcher OU reconciliação").
const (
	meDebounce    = 300 * time.Millisecond
	meReconcile   = 500 * time.Millisecond
	mePollTimeout = 5 * time.Second
	mePollStep    = 20 * time.Millisecond
	// meSettle é a janela para eventos atrasados do próprio servidor chegarem ao watcher antes da
	// contagem: cobre o debounce e pelo menos um ciclo de reconciliação.
	meSettle     = 2 * (meDebounce + meReconcile)
	meRunTimeout = 30 * time.Second

	meRel          = "decisions/db.md"
	meBodyServidor = "Escrito pelo servidor: usamos SQLite em modo WAL."
	meBodyEditado  = "Editado à mão no disco: passamos a usar PostgreSQL."
)

// TestEdicaoManualDaWikiEReindexada espelha o cenário 7 de
// specs/features/001-nucleo/acceptance/nucleo.feature: "Edição manual da wiki no disco é
// reindexada". Os passos Dado/Quando/Então viram subtestes em sequência.
//
// Sequência: a página é escrita pelo servidor (WritePage) com o watcher já rodando, o que gera
// eventos de disco do próprio servidor; depois de um intervalo de acomodação, a contagem de versões
// é a linha de base. Em seguida o arquivo é editado diretamente no disco, sem WritePage. A
// verificação espera (polling, sem sleep fixo) a versão corrente refletir a edição e confere que a
// edição gerou exatamente uma versão a mais, sem versão extra vinda do próprio servidor.
func TestEdicaoManualDaWikiEReindexada(t *testing.T) {
	ctx := context.Background()

	// O TempDir é registrado primeiro, então é removido por último, depois que os bancos fecharem.
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

	wikiRoot := filepath.Join(dataDir, "wiki")
	repo, err := wiki.Open(wikiRoot)
	if err != nil {
		t.Fatalf("wiki.Open: %v", err)
	}
	store := &wiki.Store{
		Root:   wikiRoot,
		Writer: writer,
		Repo:   repo,
		Locks:  wiki.NewPageLocks(),
	}

	var (
		scope domain.Scope
		ref   domain.PageRef
		abs   string
	)

	if !t.Run("Dado que a página decisions/db.md do projeto demo está indexada", func(t *testing.T) {
		wsID, err := domain.NewID()
		if err != nil {
			t.Fatalf("NewID do workspace: %v", err)
		}
		projID, err := domain.NewID()
		if err != nil {
			t.Fatalf("NewID do projeto: %v", err)
		}
		scope = domain.NewScope(domain.WorkspaceID(wsID), domain.ProjectID(projID))
		ref = domain.NewPageRef(scope, mustPath(t, meRel))
		abs = filepath.Join(wikiRoot, scope.WorkspaceID.String(), scope.ProjectID.String(),
			"decisions", "db.md")

		// Setup: o servidor escreve a página (WritePage) antes do watcher começar.
		meEscreve(ctx, t, store, scope, meBodyServidor)
		if n := meContarVersoes(ctx, t, reader, scope); n != 1 {
			t.Fatalf("versões após a escrita inicial = %d, quer 1", n)
		}
	}) {
		return
	}

	// O watcher roda com o mesmo Store do servidor, então compartilha a trava por página.
	stop := meIniciaWatcher(t, store)
	defer stop()

	if !t.Run("Quando o servidor escreve a página com o watcher ativo e a acomodação termina", func(t *testing.T) {
		// Escrita do próprio servidor enquanto o watcher observa a árvore: gera eventos de disco
		// (arquivo temporário, renomeação e a própria página). Nenhum deles deve virar versão.
		meEscreve(ctx, t, store, scope, meBodyServidor+" Segunda escrita do servidor.")
		time.Sleep(meSettle)
		if n := meContarVersoes(ctx, t, reader, scope); n != 2 {
			t.Fatalf("versões após escrita do servidor e acomodação = %d, quer 2 (sem versão extra do watcher)", n)
		}
	}) {
		return
	}

	var linhaDeBase int
	if !t.Run("Dado que a contagem de versões está estabilizada", func(t *testing.T) {
		linhaDeBase = meContarVersoes(ctx, t, reader, scope)
		if linhaDeBase != 2 {
			t.Fatalf("linha de base de versões = %d, quer 2", linhaDeBase)
		}
	}) {
		return
	}

	if !t.Run("Quando o usuário edita manualmente o arquivo decisions/db.md no disco", func(t *testing.T) {
		data, err := os.ReadFile(abs)
		if err != nil {
			t.Fatalf("ler %s: %v", abs, err)
		}
		// Edição direta no arquivo, sem WritePage: troca só o corpo e preserva o frontmatter.
		editado := strings.Replace(string(data), meBodyServidor+" Segunda escrita do servidor.", meBodyEditado, 1)
		if editado == string(data) {
			t.Fatalf("o corpo escrito pelo servidor não está no arquivo; conteúdo = %q", string(data))
		}
		if err := os.WriteFile(abs, []byte(editado), 0o644); err != nil {
			t.Fatalf("WriteFile manual: %v", err)
		}
	}) {
		return
	}

	t.Run("Então o watcher reindexa a página e a versão corrente traz o corpo editado", func(t *testing.T) {
		// Polling curto: o watcher reindexa após o debounce de 300 ms; a reconciliação é o fallback.
		deadline := time.Now().Add(mePollTimeout)
		var body string
		for {
			page, err := storage.ReadPage(ctx, reader, ref)
			if err == nil {
				body = page.Body
				if strings.Contains(body, meBodyEditado) {
					break
				}
			}
			if time.Now().After(deadline) {
				t.Fatalf("versão corrente não refletiu a edição em %s; corpo = %q", mePollTimeout, body)
			}
			time.Sleep(mePollStep)
		}
	})

	t.Run("Então a edição manual gera exatamente uma versão nova e a escrita do servidor não gera outra", func(t *testing.T) {
		// Espera a acomodação para que eventos atrasados (inclusive os do próprio servidor) sejam
		// processados antes da contagem final.
		time.Sleep(meSettle)
		n := meContarVersoes(ctx, t, reader, scope)
		if n != linhaDeBase+1 {
			t.Fatalf("versões após edição manual = %d, quer %d (linha de base %d + 1 da edição)",
				n, linhaDeBase+1, linhaDeBase)
		}
	})
}

// meEscreve grava a página pelo caminho do servidor (WritePage) e falha o subteste em erro.
func meEscreve(ctx context.Context, t *testing.T, store *wiki.Store, scope domain.Scope, body string) {
	t.Helper()
	var fm domain.Frontmatter
	if err := fm.Set("title", "Decisão de banco"); err != nil {
		t.Fatalf("frontmatter title: %v", err)
	}
	res, err := store.WritePage(ctx, wiki.PageInput{
		Scope:         scope,
		WorkspaceName: "workspace-teste",
		ProjectName:   "demo",
		RepoPath:      "repo-de-teste",
		Path:          mustPath(t, meRel),
		Tier:          domain.TierSemantic,
		Body:          body,
		Frontmatter:   fm,
		Evidence:      storage.EvidenceSource{Kind: storage.SourceSession, ID: "sessao-manual-edit"},
	})
	if err != nil {
		t.Fatalf("WritePage: %v", err)
	}
	if res.Version == (domain.PageID{}) {
		t.Fatalf("WritePage devolveu versão zero")
	}
}

// meIniciaWatcher roda o watcher em goroutine com ctx de timeout e devolve stop, que cancela,
// espera a goroutine terminar e confere o retorno de Run. Não há goroutine sem saída: o WaitGroup
// garante que stop só volta depois que Run sair.
func meIniciaWatcher(t *testing.T, store *wiki.Store) func() {
	t.Helper()
	w, err := wiki.NewWatcher(store, meDebounce, meReconcile)
	if err != nil {
		t.Fatalf("wiki.NewWatcher: %v", err)
	}
	runCtx, cancel := context.WithTimeout(context.Background(), meRunTimeout)
	var (
		wg     sync.WaitGroup
		runErr = make(chan error, 1)
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		runErr <- w.Run(runCtx)
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			cancel()
			wg.Wait()
			if err := <-runErr; err != nil {
				t.Errorf("watcher.Run: %v", err)
			}
		})
	}
}

// meContarVersoes conta todas as versões do caminho no escopo (corrente e supersedidas). Só lê.
func meContarVersoes(ctx context.Context, t *testing.T, r storage.Reader, scope domain.Scope) int {
	t.Helper()
	var n int
	err := r.Read(ctx, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM pages WHERE workspace_id = ? AND project_id = ? AND path = ?`,
			scope.WorkspaceID.Bytes(), scope.ProjectID.Bytes(), meRel,
		).Scan(&n)
	})
	if err != nil {
		t.Fatalf("contar versões: %v", err)
	}
	return n
}
