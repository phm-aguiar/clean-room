package acceptance

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
	"github.com/phm-aguiar/ai-memory-go/internal/domain/sanitize"
	"github.com/phm-aguiar/ai-memory-go/internal/storage"
	"github.com/phm-aguiar/ai-memory-go/internal/wiki"
)

// scAlfabeto é o alfabeto do preenchimento do segredo sintético (letras e dígitos).
const scAlfabeto = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

// scRotuloGitHub é o rótulo que o sanitizador usa para token do GitHub (DV §7.2).
const scRotuloGitHub = "[REDACTED:github_token]"

// scSegredo monta, em runtime, um token do GitHub sintético: prefixo `ghp_` mais 36 caracteres
// cíclicos do alfabeto. O prefixo é concatenado para que o fonte não contenha o valor cru.
func scSegredo() string {
	var b strings.Builder
	b.WriteString("gh" + "p_")
	for i := 0; i < 36; i++ {
		b.WriteByte(scAlfabeto[i%len(scAlfabeto)])
	}
	return b.String()
}

// TestSecretSanitizationSegredoRemovidoAntesDoLimite espelha o cenário 8 de
// specs/features/001-nucleo/acceptance/nucleo.feature: "Segredo no texto de entrada é removido
// antes de limitar o tamanho". O segredo vai no título e no corpo, pela página (WritePage) e pela
// observação (InsertObservation, com sanitize.Title e sanitize.Body). Depois se verifica o banco
// inteiro, o arquivo em disco e os limites de tamanho.
func TestSecretSanitizationSegredoRemovidoAntesDoLimite(t *testing.T) {
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

	segredo := scSegredo()
	const rel = "decisions/segredo.md"

	var (
		scope   domain.Scope
		absPath string
		sessao  domain.SessionID
	)

	if !t.Run("Dado que o texto de entrada contém um segredo no título e no corpo", func(t *testing.T) {
		wsID, err := domain.NewID()
		if err != nil {
			t.Fatalf("NewID do workspace: %v", err)
		}
		projID, err := domain.NewID()
		if err != nil {
			t.Fatalf("NewID do projeto: %v", err)
		}
		scope = domain.NewScope(domain.WorkspaceID(wsID), domain.ProjectID(projID))
		absPath = filepath.Join(wikiRoot, scope.WorkspaceID.String(), scope.ProjectID.String(),
			"decisions", "segredo.md")
		if !strings.HasPrefix(segredo, "ghp_") || len(segredo) != 40 {
			t.Fatalf("segredo sintético malformado: tamanho %d", len(segredo))
		}
	}) {
		return
	}

	if !t.Run("Quando o chamador pede a escrita da página com esse texto", func(t *testing.T) {
		// Título acima de 80 runes depois da redação: o índice precisa limitá-lo.
		titulo := "Token de acesso " + segredo + " " + strings.Repeat("ç", 90)
		var fm domain.Frontmatter
		if err := fm.Set("title", titulo); err != nil {
			t.Fatalf("frontmatter title: %v", err)
		}
		corpo := "A configuração de CI usa o token " + segredo + " para publicar a documentação.\n"
		if _, err := store.WritePage(ctx, wiki.PageInput{
			Scope:         scope,
			WorkspaceName: "workspace-teste",
			ProjectName:   "demo",
			RepoPath:      "repo-de-teste",
			Path:          mustPath(t, rel),
			Tier:          domain.TierSemantic,
			Body:          corpo,
			Frontmatter:   fm,
			Evidence:      storage.EvidenceSource{Kind: storage.SourceSession, ID: "sessao-segredo"},
		}); err != nil {
			t.Fatalf("WritePage: %v", err)
		}
	}) {
		return
	}

	if !t.Run("Quando o chamador grava uma observação com segredo e corpo acima de 16 KiB", func(t *testing.T) {
		sid, err := domain.NewID()
		if err != nil {
			t.Fatalf("NewID da sessão: %v", err)
		}
		sessao = domain.SessionID(sid)
		oid, err := domain.NewID()
		if err != nil {
			t.Fatalf("NewID da observação: %v", err)
		}

		tituloCru := "Observação " + segredo + " " + strings.Repeat("x", 120)
		corpoCru := "Comando executado com " + segredo + ".\n" +
			strings.Repeat("saída sem segredo\n", 1200) // acima de 16 KiB
		if len(corpoCru) <= sanitize.ObservationBodyMaxBytes {
			t.Fatalf("corpo de entrada de %d bytes não excede 16 KiB", len(corpoCru))
		}

		obs, err := domain.NewObservation(
			domain.ObservationID(oid),
			sessao,
			scope,
			domain.ObservationPostToolUse,
			sanitize.Title(tituloCru, sanitize.Limits{TitleMaxRunes: sanitize.ObservationTitleMaxRunes}),
			sanitize.Body(corpoCru, sanitize.Limits{BodyMaxBytes: sanitize.ObservationBodyMaxBytes}),
			5,
		)
		if err != nil {
			t.Fatalf("NewObservation: %v", err)
		}

		// A sessão precisa existir antes da observação (chave estrangeira). Sessão e observação
		// entram na mesma transação do escritor único.
		err = writer.Do(ctx, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO sessions (id, workspace_id, project_id, agent_kind, started_at)
				 VALUES (?, ?, ?, 'claude-code', ?)`,
				sessao.Bytes(), scope.WorkspaceID.Bytes(), scope.ProjectID.Bytes(), time.Now().UnixMicro(),
			); err != nil {
				return fmt.Errorf("inserir sessão: %w", err)
			}
			return storage.InsertObservation(tx, obs)
		})
		if err != nil {
			t.Fatalf("gravar observação: %v", err)
		}
	}) {
		return
	}

	// Então o segredo nunca é persistido em texto cru no armazenamento.
	t.Run("Então o segredo não aparece em nenhuma linha de nenhuma tabela do banco", func(t *testing.T) {
		conteudo, linhas := scDumpBanco(ctx, t, reader)
		for _, tab := range []string{"pages", "observations", "pages_fts", "observations_fts"} {
			if _, ok := conteudo[tab]; !ok {
				t.Fatalf("tabela %s não encontrada no esquema", tab)
			}
		}
		if linhas["pages"] == 0 || linhas["observations"] == 0 {
			t.Fatalf("varredura vazia: pages=%d observations=%d; a checagem não teria valor",
				linhas["pages"], linhas["observations"])
		}
		for tab, texto := range conteudo {
			if strings.Contains(texto, segredo) {
				t.Errorf("segredo cru encontrado na tabela %s", tab)
			}
		}
	})

	// Então o arquivo markdown em disco também não contém o segredo cru.
	t.Run("Então o arquivo markdown em disco não contém o segredo cru", func(t *testing.T) {
		data, err := os.ReadFile(absPath)
		if err != nil {
			t.Fatalf("ler %s: %v", absPath, err)
		}
		if strings.Contains(string(data), segredo) {
			t.Errorf("segredo cru encontrado no arquivo %s", absPath)
		}
	})

	// Então o texto redigido aparece como [REDACTED: no lugar do segredo.
	t.Run("Então o texto redigido aparece como marcador REDACTED no lugar do segredo", func(t *testing.T) {
		page, err := storage.ReadPage(ctx, reader, domain.NewPageRef(scope, mustPath(t, rel)))
		if err != nil {
			t.Fatalf("storage.ReadPage: %v", err)
		}
		if !strings.Contains(page.Body, scRotuloGitHub) {
			t.Errorf("corpo da página sem o marcador %s: %q", scRotuloGitHub, page.Body)
		}
		conteudo, _ := scDumpBanco(ctx, t, reader)
		if !strings.Contains(conteudo["observations"], scRotuloGitHub) {
			t.Errorf("observação sem o marcador %s", scRotuloGitHub)
		}
		data, err := os.ReadFile(absPath)
		if err != nil {
			t.Fatalf("ler %s: %v", absPath, err)
		}
		if !strings.Contains(string(data), scRotuloGitHub) {
			t.Errorf("arquivo em disco sem o marcador %s", scRotuloGitHub)
		}
	})

	// Então o título e o corpo são limitados aos tamanhos máximos após a remoção.
	t.Run("Então o título de página é limitado a 80 runes e a observação a 80 runes e 16 KiB", func(t *testing.T) {
		page, err := storage.ReadPage(ctx, reader, domain.NewPageRef(scope, mustPath(t, rel)))
		if err != nil {
			t.Fatalf("storage.ReadPage: %v", err)
		}
		if n := utf8.RuneCountInString(page.Title); n > sanitize.ObservationTitleMaxRunes {
			t.Errorf("título da página com %d runes, quer no máximo %d", n, sanitize.ObservationTitleMaxRunes)
		}
		if !strings.HasSuffix(page.Title, "…") {
			t.Errorf("título da página não foi cortado: %q", page.Title)
		}

		var obsTitulo, obsCorpo string
		err = reader.Read(ctx, func(tx *sql.Tx) error {
			return tx.QueryRowContext(ctx,
				`SELECT title, body FROM observations WHERE session_id = ?`, sessao.Bytes(),
			).Scan(&obsTitulo, &obsCorpo)
		})
		if err != nil {
			t.Fatalf("ler observação: %v", err)
		}
		if n := utf8.RuneCountInString(obsTitulo); n != sanitize.ObservationTitleMaxRunes {
			t.Errorf("título da observação com %d runes, quer %d", n, sanitize.ObservationTitleMaxRunes)
		}
		if !strings.HasSuffix(obsTitulo, "…") {
			t.Errorf("título da observação não foi cortado: %q", obsTitulo)
		}
		if n := len(obsCorpo); n > sanitize.ObservationBodyMaxBytes {
			t.Errorf("corpo da observação com %d bytes, quer no máximo %d", n, sanitize.ObservationBodyMaxBytes)
		}
		if !strings.Contains(obsCorpo, "[truncated ") {
			t.Errorf("corpo da observação sem marcador de truncamento")
		}
	})
}

// scDumpBanco lê todas as linhas de todas as tabelas do banco (inclusive as FTS e suas tabelas de
// apoio) e devolve, por tabela, o texto de todas as colunas e a contagem de linhas. Só lê.
func scDumpBanco(ctx context.Context, t *testing.T, r storage.Reader) (map[string]string, map[string]int) {
	t.Helper()
	conteudo := map[string]string{}
	linhas := map[string]int{}
	err := r.Read(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx,
			`SELECT name FROM sqlite_master WHERE type = 'table' AND substr(name, 1, 7) <> 'sqlite_'`)
		if err != nil {
			return fmt.Errorf("listar tabelas: %w", err)
		}
		var tabelas []string
		for rows.Next() {
			var nome string
			if err := rows.Scan(&nome); err != nil {
				_ = rows.Close()
				return fmt.Errorf("ler nome de tabela: %w", err)
			}
			tabelas = append(tabelas, nome)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return fmt.Errorf("listar tabelas: %w", err)
		}
		_ = rows.Close()

		for _, tab := range tabelas {
			if err := scLeTabela(ctx, tx, tab, conteudo, linhas); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("varrer banco: %v", err)
	}
	return conteudo, linhas
}

// scLeTabela acrescenta em conteudo o texto de todas as linhas de tab e conta as linhas.
func scLeTabela(ctx context.Context, tx *sql.Tx, tab string, conteudo map[string]string, linhas map[string]int) error {
	rows, err := tx.QueryContext(ctx, `SELECT * FROM "`+tab+`"`)
	if err != nil {
		return fmt.Errorf("ler tabela %s: %w", tab, err)
	}
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("colunas de %s: %w", tab, err)
	}
	var b strings.Builder
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return fmt.Errorf("varrer linha de %s: %w", tab, err)
		}
		for _, v := range vals {
			switch x := v.(type) {
			case nil:
			case []byte:
				b.Write(x)
			default:
				fmt.Fprint(&b, x)
			}
			b.WriteByte('\x1f')
		}
		b.WriteByte('\n')
		linhas[tab]++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterar %s: %w", tab, err)
	}
	conteudo[tab] = b.String()
	return nil
}
