package wiki

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// ErrConfinement é a causa de todo erro de confinamento da árvore da wiki (WFL §12): raiz
// relativa ou não canônica, caminho que sai da raiz, ou link simbólico / ponto de reparse
// em qualquer componente do caminho.
var ErrConfinement = errors.New("caminho fora do confinamento da wiki")

// fileAttrReparsePoint é FILE_ATTRIBUTE_REPARSE_POINT (winnt.h). Fica local porque
// syscall só exporta a constante no Windows.
const fileAttrReparsePoint = 0x400

// pathFor monta o caminho absoluto de uma página: <root>/<workspace_id>/<project_id>/<path>.
// É o único ponto que monta esse caminho. Os diretórios são por UUID (ADR-27 e
// wiki-layout-contract (a)). Não cria nada no disco: só valida.
func pathFor(root string, scope domain.Scope, page domain.PagePath) (string, error) {
	if err := checkRoot(root); err != nil {
		return "", err
	}
	if page.String() == "" {
		return "", domain.ErrInvalidPagePath
	}
	parts := []string{root, scope.WorkspaceID.String(), scope.ProjectID.String()}
	parts = append(parts, strings.Split(page.String(), "/")...)
	full := filepath.Join(parts...)
	if err := ensureWithin(root, full); err != nil {
		return "", err
	}
	if err := refuseLinks(root, full); err != nil {
		return "", err
	}
	return full, nil
}

// checkRoot exige raiz absoluta e canônica: já limpa e igual ao resultado de EvalSymlinks.
// Raiz que é link, ou que passa por link, falha aqui.
func checkRoot(root string) error {
	if !filepath.IsAbs(root) {
		return fmt.Errorf("%w: raiz %q não é absoluta", ErrConfinement, root)
	}
	clean := filepath.Clean(root)
	fi, err := os.Lstat(clean)
	if err != nil {
		return fmt.Errorf("raiz da wiki %q: %w", root, err)
	}
	// EvalSymlinks não resolve junções no Go 1.26 (devolve o próprio caminho), então a raiz
	// em si é checada também por Lstat: link simbólico ou ponto de reparse não é raiz válida.
	if fi.Mode()&os.ModeSymlink != 0 || hasReparseAttr(fi) {
		return fmt.Errorf("%w: raiz %q é link simbólico ou ponto de reparse", ErrConfinement, root)
	}
	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil {
		return fmt.Errorf("raiz da wiki %q: %w", root, err)
	}
	if !samePath(clean, resolved) {
		return fmt.Errorf("%w: raiz %q não é canônica (resolve para %q)", ErrConfinement, root, resolved)
	}
	return nil
}

// samePath compara caminhos canônicos. No Windows a comparação ignora caixa, porque
// EvalSymlinks devolve a caixa do sistema de arquivos.
func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// ensureWithin recusa p se, depois de limpo, ele não fica estritamente dentro de root.
// Caminho de página validado pelo domínio não produz isso; a checagem protege a montagem.
func ensureWithin(root, p string) error {
	rel, err := filepath.Rel(root, p)
	if err != nil || rel == "." || rel == ".." ||
		strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("%w: %q fora da raiz %q", ErrConfinement, p, root)
	}
	return nil
}

// refuseLinks faz Lstat em cada componente de full, a partir de root (sem incluir root, que
// checkRoot já valida). Um componente que é link simbólico ou ponto de reparse gera
// ErrConfinement. Componente inexistente encerra a verificação: o que vem depois também não existe.
func refuseLinks(root, full string) error {
	rel, err := filepath.Rel(root, full)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrConfinement, err)
	}
	cur := root
	for _, seg := range strings.Split(rel, string(filepath.Separator)) {
		cur = filepath.Join(cur, seg)
		fi, err := os.Lstat(cur)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("verificar componente %q: %w", cur, err)
		}
		if fi.Mode()&os.ModeSymlink != 0 || hasReparseAttr(fi) {
			return fmt.Errorf("%w: componente %q é link simbólico ou ponto de reparse", ErrConfinement, cur)
		}
	}
	return nil
}

// hasReparseAttr diz se o FileInfo traz FILE_ATTRIBUTE_REPARSE_POINT. Lstat não segue o link,
// então o atributo aparece no próprio item. No Windows Sys() é *syscall.Win32FileAttributeData,
// com o campo FileAttributes. No Unix é *syscall.Stat_t, sem esse campo, e o resultado é false
// (o link já cai em ModeSymlink). Usa reflexão porque o tipo do Windows não compila em outros SOs
// e esta task não pode criar arquivos por SO. Limitação: o atributo vale para qualquer reparse
// (inclusive placeholders do OneDrive), não só para links. Essa recusa é intencional.
func hasReparseAttr(fi os.FileInfo) bool {
	v := reflect.ValueOf(fi.Sys())
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return false
	}
	f := v.FieldByName("FileAttributes")
	return f.IsValid() && f.CanUint() && f.Uint()&fileAttrReparsePoint != 0
}
