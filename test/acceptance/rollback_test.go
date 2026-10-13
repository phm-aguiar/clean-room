package acceptance

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
	"github.com/phm-aguiar/ai-memory-go/internal/storage"
	"github.com/phm-aguiar/ai-memory-go/internal/wiki"
)

// Textos e caminho do cenário 4 de specs/features/001-nucleo/acceptance/nucleo.feature. O corpo não
// tem segredo.
const (
	rbPagePath   = "decisions/db.md"
	rbFirstBody  = "Escolhemos SQLite em modo WAL para o armazenamento local, sem servidor externo."
	rbSecondBody = "Escolhemos PostgreSQL para o armazenamento compartilhado entre máquinas."
)

// errFalhaBanco é a falha injetada no banco, depois que a closure rodou e o arquivo já foi instalado.
var errFalhaBanco = errors.New("falha injetada no banco depois de instalar o arquivo")

// rbFalhaDepoisDaClosure espelha a ideia do wpFailingWriter de internal/wiki: na chamada número
// failOn, executa a closure dentro de uma transação real (efeitos no banco) e depois devolve erro.
// Assim o storage faz ROLLBACK e o erro volta ao chamador. Chamadas anteriores passam direto.
type rbFalhaDepoisDaClosure struct {
	storage.Writer
	failOn int
	calls  int
	// aposClosure roda dentro da transação, depois da closure e antes do erro. Usada para observar o
	// disco no instante em que a falha acontece.
	aposClosure func()
	disparou    bool
}

func (f *rbFalhaDepoisDaClosure) Do(ctx context.Context, fn func(*sql.Tx) error) error {
	f.calls++
	if f.calls != f.failOn {
		return f.Writer.Do(ctx, fn)
	}
	return f.Writer.Do(ctx, func(tx *sql.Tx) error {
		if err := fn(tx); err != nil {
			return err
		}
		f.disparou = true
		if f.aposClosure != nil {
			f.aposClosure()
		}
		return errFalhaBanco
	})
}

// TestRollbackFalhaDoBancoReverteArquivo espelha o cenário 4 da feature: "Falha no banco após instalar
// o arquivo reverte a página". Os passos Dado/Quando/Então viram subtestes em sequência. Se um passo
// falha, os seguintes não rodam. Não há goroutine nem panic no teste.
func TestRollbackFalhaDoBancoReverteArquivo(t *testing.T) {
	ctx := context.Background()

	// Setup comum. O TempDir é registrado primeiro, então é removido por último, depois que o escritor
	// e os bancos forem fechados.
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
		scope        domain.Scope
		abs          string // caminho absoluto do arquivo decisions/db.md
		prevBytes    []byte // conteúdo do arquivo antes da falha
		prevPage     domain.Page
		commitsAntes int
		headAntes    string
		failing      *wiki.Store
		injetor      *rbFalhaDepoisDaClosure
		noDisco      []byte // conteúdo do arquivo no instante da falha, dentro da transação
		writeErr     error
	)

	if !t.Run("Dado que o arquivo decisions/db.md foi instalado com sucesso no disco", func(t *testing.T) {
		wsID, err := domain.NewID()
		if err != nil {
			t.Fatalf("NewID do workspace: %v", err)
		}
		projID, err := domain.NewID()
		if err != nil {
			t.Fatalf("NewID do projeto: %v", err)
		}
		scope = domain.NewScope(domain.WorkspaceID(wsID), domain.ProjectID(projID))
		abs = filepath.Join(wikiRoot, scope.WorkspaceID.String(), scope.ProjectID.String(),
			"decisions", "db.md")

		// Passo 1: versão inicial com o Writer real, sem injeção.
		if _, err := store.WritePage(ctx, rbEntrada(t, scope, rbFirstBody)); err != nil {
			t.Fatalf("WritePage da versão inicial: %v", err)
		}
		prevBytes, err = os.ReadFile(abs)
		if err != nil {
			t.Fatalf("ler arquivo instalado %s: %v", abs, err)
		}
		prevPage = rbLerPagina(t, ctx, reader, scope)
		commitsAntes = rbContarCommits(t, wikiRoot)
		headAntes = rbHead(t, wikiRoot)
	}) {
		return
	}

	if !t.Run("E que a transação do banco falha em seguida", func(t *testing.T) {
		// Injeção: o Writer executa a closure do UpsertPage e depois falha. Chamada 1 é EnsureScope,
		// chamada 2 é UpsertPage, então failOn = 2 pega a escrita do índice, depois da instalação.
		injetor = &rbFalhaDepoisDaClosure{
			Writer: writer,
			failOn: 2,
			aposClosure: func() {
				conteudo, err := os.ReadFile(abs)
				if err != nil {
					// Não há como registrar o erro de dentro da transação com segurança; fica vazio e a
					// verificação de instalação na Quando falha.
					return
				}
				noDisco = conteudo
			},
		}
		copia := *store
		copia.Writer = injetor
		failing = &copia
	}) {
		return
	}

	if !t.Run("Quando o chamador pede a escrita da página decisions/db.md no projeto demo", func(t *testing.T) {
		_, writeErr = failing.WritePage(ctx, rbEntrada(t, scope, rbSecondBody))
		if writeErr == nil {
			t.Fatalf("WritePage não devolveu erro; a falha injetada não foi exercida")
		}
		if !injetor.disparou {
			t.Fatalf("a falha não foi injetada: a transação do índice não rodou")
		}
		// Pré-condição da cena: o arquivo novo foi instalado antes da falha. Sem isso o teste de
		// reversão seria vazio.
		if !strings.Contains(string(noDisco), rbSecondBody) {
			t.Fatalf("arquivo no instante da falha não tem o conteúdo novo; corpo = %q", string(noDisco))
		}
		if bytes.Equal(noDisco, prevBytes) {
			t.Fatalf("arquivo no instante da falha ainda é o anterior; a instalação não aconteceu")
		}
	}) {
		return
	}

	t.Run("Então o arquivo decisions/db.md é revertido ao estado anterior", func(t *testing.T) {
		atual, err := os.ReadFile(abs)
		if err != nil {
			t.Fatalf("ler arquivo após a falha %s: %v", abs, err)
		}
		if !bytes.Equal(atual, prevBytes) {
			t.Errorf("arquivo não voltou ao conteúdo anterior:\nquer %q\nobteve %q", prevBytes, atual)
		}
	})

	t.Run("E o erro é propagado de forma não ambígua ao chamador", func(t *testing.T) {
		if writeErr == nil {
			t.Fatalf("erro ausente")
		}
		if !errors.Is(writeErr, errFalhaBanco) {
			t.Errorf("erro não encadeia a falha do banco: %v", writeErr)
		}
		if errors.Is(writeErr, wiki.ErrInconsistentState) {
			t.Errorf("reversão funcionou e não deveria devolver ErrInconsistentState: %v", writeErr)
		}
		if errors.Is(writeErr, storage.ErrInconsistentState) {
			t.Errorf("reversão funcionou e não deveria devolver storage.ErrInconsistentState: %v", writeErr)
		}
		msg := writeErr.Error()
		if strings.TrimSpace(msg) == "" {
			t.Errorf("mensagem de erro vazia")
		}
		if strings.Contains(msg, "restaurar") {
			t.Errorf("mensagem sugere falha na restauração, o que seria ambíguo: %q", msg)
		}
	})

	t.Run("E nenhum commit git é criado", func(t *testing.T) {
		if n := rbContarCommits(t, wikiRoot); n != commitsAntes {
			t.Errorf("commits %d -> %d: a falha não deveria commitar", commitsAntes, n)
		}
		if h := rbHead(t, wikiRoot); h != headAntes {
			t.Errorf("HEAD mudou de %q para %q numa falha", headAntes, h)
		}
	})

	t.Run("E o índice permanece inalterado", func(t *testing.T) {
		depois := rbLerPagina(t, ctx, reader, scope)
		if depois.Version != prevPage.Version {
			t.Errorf("versão corrente mudou: antes %s, depois %s",
				prevPage.Version.String(), depois.Version.String())
		}
		if depois.Ref.Path.String() != prevPage.Ref.Path.String() {
			t.Errorf("caminho mudou: antes %q, depois %q",
				prevPage.Ref.Path.String(), depois.Ref.Path.String())
		}
		if depois.Body != prevPage.Body {
			t.Errorf("corpo no índice mudou:\nantes %q\ndepois %q", prevPage.Body, depois.Body)
		}
		if strings.Contains(depois.Body, rbSecondBody) {
			t.Errorf("índice contém o corpo que deveria ter sido revertido")
		}
	})
}

// rbEntrada monta o pedido de escrita de decisions/db.md com o corpo dado, no projeto de teste.
func rbEntrada(t *testing.T, scope domain.Scope, body string) wiki.PageInput {
	t.Helper()
	var fm domain.Frontmatter
	if err := fm.Set("title", "Banco de dados"); err != nil {
		t.Fatalf("frontmatter title: %v", err)
	}
	path, err := domain.ParsePagePath(rbPagePath)
	if err != nil {
		t.Fatalf("ParsePagePath(%q): %v", rbPagePath, err)
	}
	return wiki.PageInput{
		Scope:         scope,
		WorkspaceName: "workspace-teste",
		ProjectName:   "demo",
		RepoPath:      "repo-de-teste",
		Path:          path,
		Tier:          domain.TierSemantic,
		Body:          body,
		Frontmatter:   fm,
		Evidence:      storage.EvidenceSource{Kind: storage.SourceSession, ID: "sessao-teste-rollback"},
	}
}

// rbLerPagina lê a versão corrente de decisions/db.md pelo Reader. Falha encerra o subteste.
func rbLerPagina(t *testing.T, ctx context.Context, r storage.Reader, scope domain.Scope) domain.Page {
	t.Helper()
	path, err := domain.ParsePagePath(rbPagePath)
	if err != nil {
		t.Fatalf("ParsePagePath(%q): %v", rbPagePath, err)
	}
	page, err := storage.ReadPage(ctx, r, domain.NewPageRef(scope, path))
	if err != nil {
		t.Fatalf("storage.ReadPage: %v", err)
	}
	return page
}

// rbContarCommits devolve o número de commits alcançáveis a partir de HEAD no repositório da wiki.
func rbContarCommits(t *testing.T, root string) int {
	t.Helper()
	out, err := exec.Command("git", "-C", root, "rev-list", "--count", "HEAD").Output()
	if err != nil {
		t.Fatalf("git rev-list --count HEAD: %v", err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatalf("rev-list --count HEAD: saída inesperada %q: %v", string(out), err)
	}
	return n
}

// rbHead devolve o hash de HEAD do repositório da wiki.
func rbHead(t *testing.T, root string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD: %v", err)
	}
	return strings.TrimSpace(string(out))
}
