package cli

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/phm-aguiar/ai-memory-go/internal/storage"
	"github.com/phm-aguiar/ai-memory-go/internal/wiki"
)

const (
	backupConfigName   = "config.toml"
	backupDBEntry      = "db/memory.sqlite"
	backupWikiEntry    = "wiki"
	backupArchiveName  = "backup.tar.gz"
	backupGitDirName   = ".git"
	backupTmpPrefix    = ".ai-memory-tmp."
	backupSnapshotName = "memory.sqlite"
)

// NewAdminBackupHandler devolve o handler de POST /admin/backup. O handler não faz
// autenticação: quem monta a rota (httpserver, T039) põe o middleware de admin na
// frente. configFile é o caminho do TOML; vazio usa <diretório de dados>/config.toml,
// o mesmo padrão de config.Load. O arquivo só entra no pacote se existir.
//
// Fluxo: valida a árvore da wiki; grava um snapshot online do banco pela API de
// backup do SQLite (storage.Snapshot, sem parar o escritor); empacota wiki/,
// db/memory.sqlite e config.toml em um tar.gz temporário; responde o arquivo com
// Content-Type application/gzip. Falha responde 500 com {"error": "..."}.
func NewAdminBackupHandler(app *App, configFile string) http.Handler {
	return &adminBackupHandler{app: app, configFile: configFile}
}

type adminBackupHandler struct {
	app        *App
	configFile string
}

// ServeHTTP executa o backup. Só POST é aceito.
func (h *adminBackupHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeAdminError(w, http.StatusMethodNotAllowed, "método não permitido")
		return
	}
	if h.app == nil || h.app.DB == nil {
		writeAdminError(w, http.StatusInternalServerError, "backup indisponível: banco não montado")
		return
	}

	tmpDir, err := os.MkdirTemp("", "ai-memory-backup-")
	if err != nil {
		h.fail(w, "criar diretório temporário", err)
		return
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	archive := filepath.Join(tmpDir, backupArchiveName)
	if err := h.buildArchive(r.Context(), tmpDir, archive); err != nil {
		h.fail(w, "gerar arquivo de backup", err)
		return
	}

	f, err := os.Open(archive)
	if err != nil {
		h.fail(w, "abrir arquivo de backup", err)
		return
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		h.fail(w, "ler arquivo de backup", err)
		return
	}

	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+backupArchiveName+`"`)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, f); err != nil {
		// Cabeçalhos já foram enviados; só registra.
		h.logger().Error("backup: enviar corpo", "err", err)
	}
}

// buildArchive valida a wiki, grava o snapshot do banco em tmpDir e monta o tar.gz em
// archive.
func (h *adminBackupHandler) buildArchive(ctx context.Context, tmpDir, archive string) error {
	dataDir := h.app.Cfg.DataDir()
	wikiDir := filepath.Join(dataDir, wikiDirName)

	if err := validateWikiTree(dataDir, wikiDir); err != nil {
		return err
	}

	snapPath := filepath.Join(tmpDir, backupSnapshotName)
	if err := storage.Snapshot(ctx, h.app.DB, snapPath); err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}

	cfgPath := h.configFile
	if cfgPath == "" {
		cfgPath = filepath.Join(dataDir, backupConfigName)
	}
	return writeTarGz(archive, dataDir, wikiDir, snapPath, cfgPath)
}

// logger devolve o logger do App, ou o padrão se ele não foi definido.
func (h *adminBackupHandler) logger() *slog.Logger {
	if h.app != nil && h.app.Logger != nil {
		return h.app.Logger
	}
	return slog.Default()
}

// fail registra o erro completo no log e responde 500 sem caminhos do sistema. A
// árvore inválida é a exceção: sua mensagem já traz só o caminho relativo.
func (h *adminBackupHandler) fail(w http.ResponseWriter, stage string, err error) {
	h.logger().Error("backup: "+stage, "err", err)
	var tree *treeError
	if errors.As(err, &tree) {
		writeAdminError(w, http.StatusInternalServerError, tree.Error())
		return
	}
	writeAdminError(w, http.StatusInternalServerError, "falha ao "+stage)
}

// treeError indica uma entrada inválida da wiki. rel é o caminho relativo ao diretório
// de dados, com barras normais.
type treeError struct {
	rel    string
	reason string
}

func (e *treeError) Error() string {
	return fmt.Sprintf("árvore da wiki inválida: %s: %s", e.rel, e.reason)
}

// validateWikiTree percorre wikiDir e recusa: raiz ausente ou que não é diretório;
// link simbólico, junção ou tipo especial de arquivo; arquivo .md que não é UTF-8 ou
// cujo frontmatter não lê. O diretório .git não tem o conteúdo validado. Arquivos
// temporários da escrita atômica são ignorados (não são páginas).
func validateWikiTree(dataDir, wikiDir string) error {
	fi, err := os.Lstat(wikiDir)
	if err != nil {
		return fmt.Errorf("raiz da wiki: %w", err)
	}
	if !fi.IsDir() {
		return &treeError{rel: relBackupPath(dataDir, wikiDir), reason: "raiz não é diretório"}
	}

	return filepath.WalkDir(wikiDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("caminhar wiki: %w", err)
		}
		if p == wikiDir {
			return nil
		}
		name := d.Name()
		if d.IsDir() && name == backupGitDirName {
			return fs.SkipDir
		}
		if strings.HasPrefix(name, backupTmpPrefix) {
			return nil
		}
		rel := relBackupPath(dataDir, p)
		if err := checkEntryType(d, rel); err != nil {
			return err
		}
		if d.IsDir() || !strings.EqualFold(filepath.Ext(name), ".md") {
			return nil
		}
		return validateMarkdown(p, rel)
	})
}

// checkEntryType recusa o que não é diretório nem arquivo regular.
func checkEntryType(d fs.DirEntry, rel string) error {
	t := d.Type()
	if t&(fs.ModeSymlink|fs.ModeIrregular|fs.ModeDevice|fs.ModeNamedPipe|fs.ModeSocket) != 0 {
		return &treeError{rel: rel, reason: "link simbólico, ponto de reparse ou tipo especial"}
	}
	return nil
}

// validateMarkdown exige UTF-8 válido e frontmatter legível (wiki.ReadMarkdown).
func validateMarkdown(path, rel string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("ler página: %w", err)
	}
	if !utf8.Valid(data) {
		return &treeError{rel: rel, reason: "não é UTF-8"}
	}
	if _, _, err := wiki.ReadMarkdown(data); err != nil {
		return &treeError{rel: rel, reason: "frontmatter inválido"}
	}
	return nil
}

// writeTarGz grava em archive o tar.gz com wiki/ (árvore completa, inclusive .git),
// db/memory.sqlite a partir de snapPath e config.toml se cfgPath existir como arquivo
// regular.
func writeTarGz(archive, dataDir, wikiDir, snapPath, cfgPath string) (err error) {
	out, err := os.OpenFile(archive, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("criar arquivo: %w", err)
	}
	defer func() {
		if cerr := out.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("fechar arquivo: %w", cerr)
		}
	}()

	gz := gzip.NewWriter(out)
	tw := tar.NewWriter(gz)
	if err := addWikiTree(tw, dataDir, wikiDir); err != nil {
		return err
	}
	if err := addRegularFile(tw, snapPath, backupDBEntry); err != nil {
		return err
	}
	if err := addOptionalConfig(tw, cfgPath); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("fechar tar: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("fechar gzip: %w", err)
	}
	return nil
}

// addWikiTree adiciona wiki/ e tudo abaixo dele. Os caminhos já foram validados.
func addWikiTree(tw *tar.Writer, dataDir, wikiDir string) error {
	return filepath.WalkDir(wikiDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("caminhar wiki: %w", err)
		}
		if strings.HasPrefix(d.Name(), backupTmpPrefix) && p != wikiDir {
			return nil
		}
		rel := relBackupPath(dataDir, p)
		if d.IsDir() {
			return addDirEntry(tw, p, rel+"/")
		}
		return addRegularFile(tw, p, rel)
	})
}

// addDirEntry grava o cabeçalho de um diretório com o nome informado.
func addDirEntry(tw *tar.Writer, path, name string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("ler diretório: %w", err)
	}
	hdr, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return fmt.Errorf("cabeçalho de diretório: %w", err)
	}
	hdr.Name = name
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("gravar cabeçalho: %w", err)
	}
	return nil
}

// addRegularFile grava um arquivo regular com o nome informado. Link ou tipo especial
// falha: o conteúdo é sempre lido de um arquivo comum.
func addRegularFile(tw *tar.Writer, path, name string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("ler arquivo: %w", err)
	}
	if !info.Mode().IsRegular() {
		return &treeError{rel: name, reason: "não é arquivo regular"}
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("abrir arquivo: %w", err)
	}
	defer func() { _ = f.Close() }()

	hdr := &tar.Header{
		Name:     name,
		Mode:     0o600,
		Size:     info.Size(),
		ModTime:  info.ModTime(),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("gravar cabeçalho: %w", err)
	}
	if _, err := io.Copy(tw, f); err != nil {
		return fmt.Errorf("gravar conteúdo: %w", err)
	}
	return nil
}

// addOptionalConfig grava config.toml só se o arquivo existir.
func addOptionalConfig(tw *tar.Writer, cfgPath string) error {
	if _, err := os.Lstat(cfgPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("ler configuração: %w", err)
	}
	return addRegularFile(tw, cfgPath, backupConfigName)
}

// relBackupPath devolve p relativo a dataDir com barras normais (formato do tar).
func relBackupPath(dataDir, p string) string {
	rel, err := filepath.Rel(dataDir, p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(rel)
}

// writeAdminError grava {"error": msg} com o status informado.
func writeAdminError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
