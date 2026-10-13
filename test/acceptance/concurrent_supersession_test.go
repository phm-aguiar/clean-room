package acceptance

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
	"github.com/phm-aguiar/ai-memory-go/internal/storage"
	"github.com/phm-aguiar/ai-memory-go/internal/wiki"
)

// concPaths são as rodadas do cenário 6. A primeira é o caminho literal do cenário
// (decisions/db.md); as demais repetem a corrida em caminhos novos, para aumentar a chance de
// expor uma falha intermitente sem alongar o teste.
var concPaths = []string{
	"decisions/db.md",
	"decisions/db-1.md",
	"decisions/db-2.md",
	"decisions/db-3.md",
	"decisions/db-4.md",
}

// concRoundTimeout limita cada escrita de uma sessão. Uma escrita travada falha em vez de pendurar o teste.
const concRoundTimeout = 10 * time.Second

// concOutcome é o que uma sessão recebe de WritePage numa rodada.
type concOutcome struct {
	res  wiki.WriteResult
	err  error
	body string
}

// TestConcurrentSupersessionMesmoCaminho espelha o cenário 6 de
// specs/features/001-nucleo/acceptance/nucleo.feature: "Duas sessões escrevem o mesmo caminho
// ao mesmo tempo". INV-16 exige teste multi-sessão: as duas sessões são duas instâncias de
// wiki.Store sobre o mesmo Writer, o mesmo Repo e as mesmas Locks, que é o que serializa a
// escrita de uma página. A largada é simultânea (canal de início). Cada rodada verifica:
//
//	(a) as duas escritas terminam sem erro, uma como criação e outra como supersessão;
//	(b) a cadeia do caminho tem duas versões, a corrente e a perdedora, ambas alcançáveis;
//	(c) a versão perdedora continua no banco com o corpo original;
//	(d) ReadPage devolve a versão corrente só pelo caminho, sem parâmetro de autor.
func TestConcurrentSupersessionMesmoCaminho(t *testing.T) {
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

	// Duas sessões, uma trava compartilhada: a única coisa que as separa é o identificador
	// de evidência, que vai para a fonte de cada escrita.
	locks := wiki.NewPageLocks()
	sessions := [2]*wiki.Store{
		{Root: wikiRoot, Writer: writer, Repo: repo, Locks: locks},
		{Root: wikiRoot, Writer: writer, Repo: repo, Locks: locks},
	}

	// Dado que duas sessões distintas do projeto "demo" existem.
	wsID, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID do workspace: %v", err)
	}
	projID, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID do projeto: %v", err)
	}
	scope := domain.NewScope(domain.WorkspaceID(wsID), domain.ProjectID(projID))

	for _, rel := range concPaths {
		concRunRound(ctx, t, reader, scope, sessions, rel)
	}
}

// concRunRound executa uma rodada do cenário 6 no caminho rel como subteste e verifica (a) a (d).
func concRunRound(ctx context.Context, t *testing.T, reader storage.Reader, scope domain.Scope, sessions [2]*wiki.Store, rel string) {
	t.Run(rel, func(t *testing.T) {
		path := mustConcPath(t, rel)
		ref := domain.NewPageRef(scope, path)

		// Dado que as duas sessões escrevem a página simultaneamente: conteúdos diferentes.
		bodies := [2]string{
			"Sessão A decidiu usar SQLite em modo WAL para o armazenamento local, no caminho " + rel + ".",
			"Sessão B decidiu usar PostgreSQL para o armazenamento local, no caminho " + rel + ".",
		}

		// Quando as duas escritas são processadas pelo escritor único: largada simultânea.
		start := make(chan struct{})
		var (
			wg       sync.WaitGroup
			outcomes [2]concOutcome
		)
		for i := range sessions {
			wg.Add(1)
			go func() {
				defer wg.Done()
				cctx, cancel := context.WithTimeout(ctx, concRoundTimeout)
				defer cancel()

				var fm domain.Frontmatter
				if err := fm.Set("title", "Decisão de banco"); err != nil {
					outcomes[i] = concOutcome{err: fmt.Errorf("frontmatter: %w", err)}
					return
				}

				<-start
				res, err := sessions[i].WritePage(cctx, wiki.PageInput{
					Scope:         scope,
					WorkspaceName: "workspace-teste",
					ProjectName:   "demo",
					RepoPath:      "repo-de-teste",
					Path:          path,
					Tier:          domain.TierSemantic,
					Body:          bodies[i],
					Frontmatter:   fm,
					Evidence:      storage.EvidenceSource{Kind: storage.SourceSession, ID: fmt.Sprintf("sessao-%c", 'a'+i)},
				})
				outcomes[i] = concOutcome{res: res, err: err, body: bodies[i]}
			}()
		}
		close(start)
		wg.Wait()

		// (a) Ambas as escritas são aceitas. Colisão ou estado inconsistente são falhas nomeadas.
		for i, o := range outcomes {
			if o.err != nil {
				t.Fatalf("(a) sessão %c: WritePage falhou: %v%s", 'A'+i, o.err, concDescreveErro(o.err))
			}
		}
		var primeira, segunda *concOutcome
		for i := range outcomes {
			switch outcomes[i].res.Outcome {
			case storage.UpsertCreated:
				primeira = &outcomes[i]
			case storage.UpsertSuperseded:
				segunda = &outcomes[i]
			}
		}
		if primeira == nil || segunda == nil {
			t.Fatalf("(a) outcomes = [%q, %q]; quer uma criação e uma supersessão",
				outcomes[0].res.Outcome, outcomes[1].res.Outcome)
		}

		// (b) A cadeia do caminho tem duas versões: a corrente e a perdedora, ambas alcançáveis.
		chain, err := storage.PageChain(ctx, reader, ref)
		if err != nil {
			t.Fatalf("(b) storage.PageChain: %v", err)
		}
		if len(chain) != 2 {
			t.Fatalf("(b) tamanho da cadeia = %d, quer 2", len(chain))
		}
		if !chain[0].IsLatest || chain[0].ID != segunda.res.Version {
			t.Errorf("(b) cadeia[0] = {id %s, latest %v}, quer a corrente {id %s, latest true}",
				chain[0].ID.String(), chain[0].IsLatest, segunda.res.Version.String())
		}
		if chain[1].IsLatest || chain[1].ID != primeira.res.Version {
			t.Errorf("(b) cadeia[1] = {id %s, latest %v}, quer a perdedora {id %s, latest false}",
				chain[1].ID.String(), chain[1].IsLatest, primeira.res.Version.String())
		}
		if chain[0].Supersedes == nil || *chain[0].Supersedes != chain[1].ID {
			t.Errorf("(b) a corrente não supersede a perdedora: supersedes = %v", chain[0].Supersedes)
		}
		if chain[1].Supersedes != nil {
			t.Errorf("(b) a primeira versão não deveria ter supersedes, tem %s", chain[1].Supersedes.String())
		}

		// (c) A versão perdedora continua no banco, com o corpo original e fora da corrente.
		body, isLatest, err := concCorpoDaVersao(ctx, reader, chain[1].ID)
		if err != nil {
			t.Fatalf("(c) ler versão perdedora %s: %v", chain[1].ID.String(), err)
		}
		if isLatest {
			t.Errorf("(c) versão perdedora marcada como corrente")
		}
		if body != primeira.body {
			t.Errorf("(c) corpo da versão perdedora = %q, quer o original %q", body, primeira.body)
		}

		// (d) ReadPage devolve a corrente só pelo caminho. A chamada não recebe autor.
		page, err := storage.ReadPage(ctx, reader, ref)
		if err != nil {
			t.Fatalf("(d) storage.ReadPage: %v", err)
		}
		if page.Version != segunda.res.Version {
			t.Errorf("(d) ReadPage versão = %s, quer a corrente %s",
				page.Version.String(), segunda.res.Version.String())
		}
		if page.Body != segunda.body {
			t.Errorf("(d) ReadPage corpo = %q, quer o da corrente %q", page.Body, segunda.body)
		}
		if page.Ref.Path.String() != rel {
			t.Errorf("(d) ReadPage caminho = %q, quer %q", page.Ref.Path.String(), rel)
		}
	})
}

// concCorpoDaVersao lê o corpo e a marca de corrente de uma versão pelo id, em leitura somente leitura.
func concCorpoDaVersao(ctx context.Context, reader storage.Reader, id domain.PageID) (string, bool, error) {
	var (
		body     string
		isLatest int
	)
	err := reader.Read(ctx, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			`SELECT body, is_latest FROM pages WHERE id = ?`, id.Bytes(),
		).Scan(&body, &isLatest)
	})
	if err != nil {
		return "", false, err
	}
	return body, isLatest != 0, nil
}

// concDescreveErro nomeia a falha de colisão ou de estado inconsistente, se for esse o caso.
// O cenário espera que as duas escritas sejam aceitas, então qualquer uma dessas é falha.
func concDescreveErro(err error) string {
	switch {
	case errors.Is(err, storage.ErrPathCollision):
		return " [ErrPathCollision: o cenário exige que a segunda escrita supersede a primeira]"
	case errors.Is(err, storage.ErrInconsistentState):
		return " [ErrInconsistentState: arquivo não revertido após falha do índice]"
	default:
		return ""
	}
}

// mustConcPath faz o parse do caminho da rodada; falha encerra o subteste.
func mustConcPath(t *testing.T, s string) domain.PagePath {
	t.Helper()
	p, err := domain.ParsePagePath(s)
	if err != nil {
		t.Fatalf("ParsePagePath(%q): %v", s, err)
	}
	return p
}
