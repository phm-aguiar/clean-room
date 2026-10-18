package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"

	"github.com/knadh/koanf/parsers/toml"
)

// Leitura do marcador `.ai-memory.toml` no cliente (docs/marker-file.md, "Discovery",
// "File format and reading" e "Capture exclusions"; specs/features/002-captura-e-acesso/notes/capture-contract.md §1 a §3).
//
// O diretório home entra por parâmetro: quem compõe o cliente (cli) o obtém; este arquivo não lê o ambiente (INV-01).
// Campos de escopo são lidos por linha. `[capture]` é lido por TOML estrito, com teto de 64 KiB.

const (
	markerFileName = ".ai-memory.toml"
	// markerCaptureMaxBytes é o teto de [capture] e da inspeção (MF "File format and reading": 64 KiB).
	markerCaptureMaxBytes = 64 << 10
)

// markerScopeKeys são as chaves que fazem um marcador "declarar algo" na caminhada de escopo
// (MF "Two walks"). Um marcador que só tem [capture] é pulado por essa caminhada.
var markerScopeKeys = []string{
	"workspace", "project", "project_strategy", "drop_subagent_captures", "identity",
	"identity_style", "aliases", "server", "default_global", "inject_on_session_start",
	"max_chars", "contribute", "consume",
}

// scopeMarker é o marcador de escopo mais próximo: o arquivo e seu conteúdo.
type scopeMarker struct {
	path string
	data []byte
}

// captureState é o estado da política de captura lida do marcador (contrato §3 e §10).
type captureState int

const (
	// captureInactive: sem marcador, sem [capture] ou com ignore_paths vazio.
	captureInactive captureState = iota
	// captureActive: [capture] válido com ao menos um caminho.
	captureActive
	// captureInvalid: qualquer falha de leitura ou validação do bloco. Falha fechado.
	captureInvalid
)

// capturePolicy é o bloco [capture] lido. ignorePaths só tem valor quando state é captureActive.
// A validação dos padrões (caracteres, limites) não é feita aqui; cabe à política de captura.
type capturePolicy struct {
	state       captureState
	ignorePaths []string
}

// markerDirs devolve os diretórios percorridos a partir do cwd até a fronteira, inclusive,
// com o mais próximo primeiro (MF "Walk boundaries").
func markerDirs(cwd, home string) ([]string, error) {
	if !filepath.IsAbs(cwd) {
		return nil, fmt.Errorf("marker: cwd %q não é absoluto", cwd)
	}
	cwd = filepath.Clean(cwd)
	boundary, err := markerBoundary(cwd, home)
	if err != nil {
		return nil, err
	}
	var dirs []string
	dir := cwd
	for {
		dirs = append(dirs, dir)
		if markerSamePath(dir, boundary) {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return dirs, nil
}

// markerBoundary aplica a tabela de fronteiras de MF "Walk boundaries".
func markerBoundary(cwd, home string) (string, error) {
	if home == "" {
		return filepath.VolumeName(cwd) + string(filepath.Separator), nil
	}
	home = filepath.Clean(home)
	if markerWithin(cwd, home) {
		return home, nil
	}
	for dir := cwd; ; {
		ok, err := markerGitEntry(dir)
		if err != nil {
			return "", err
		}
		if ok {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return cwd, nil
}

// markerGitEntry diz se dir contém uma entrada `.git` (arquivo ou diretório).
func markerGitEntry(dir string) (bool, error) {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// markerFileIn devolve o caminho do marcador em dir, se existir como arquivo regular.
// Um diretório com esse nome é ignorado (MF "File name and location").
func markerFileIn(dir string) (string, bool, error) {
	p := filepath.Join(dir, markerFileName)
	fi, err := os.Stat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if !fi.Mode().IsRegular() {
		return "", false, nil
	}
	return p, true, nil
}

// readScopeMarker é a caminhada de escopo: o primeiro marcador, do cwd para cima, que declara
// alguma chave de escopo. Os campos nunca são mesclados entre arquivos (MF "File name and location").
func readScopeMarker(cwd, home string) (scopeMarker, bool, error) {
	dirs, err := markerDirs(cwd, home)
	if err != nil {
		return scopeMarker{}, false, err
	}
	for _, dir := range dirs {
		p, ok, err := markerFileIn(dir)
		if err != nil {
			return scopeMarker{}, false, err
		}
		if !ok {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return scopeMarker{}, false, fmt.Errorf("marker: lendo %s: %w", p, err)
		}
		if markerDeclaresScope(data) {
			return scopeMarker{path: p, data: data}, true, nil
		}
	}
	return scopeMarker{}, false, nil
}

// findCaptureMarker é a caminhada de captura: o marcador mais próximo de qualquer tipo, mesmo
// que só tenha [capture]. Esse marcador substitui por completo as regras dos externos.
func findCaptureMarker(cwd, home string) (string, bool, error) {
	dirs, err := markerDirs(cwd, home)
	if err != nil {
		return "", false, err
	}
	for _, dir := range dirs {
		p, ok, err := markerFileIn(dir)
		if err != nil {
			return "", false, err
		}
		if ok {
			return p, true, nil
		}
	}
	return "", false, nil
}

// loadCapturePolicy lê a política de captura do marcador mais próximo. Qualquer erro de
// leitura do arquivo selecionado torna a política inválida (falha fechado).
func loadCapturePolicy(cwd, home string) capturePolicy {
	p, ok, err := findCaptureMarker(cwd, home)
	if err != nil {
		return capturePolicy{state: captureInvalid}
	}
	if !ok {
		return capturePolicy{state: captureInactive}
	}
	return readCaptureBlock(p)
}

// readCaptureBlock lê o [capture] de um arquivo. Arquivo acima de 64 KiB é inválido.
func readCaptureBlock(path string) capturePolicy {
	fi, err := os.Stat(path)
	if err != nil || fi.Size() > markerCaptureMaxBytes {
		return capturePolicy{state: captureInvalid}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return capturePolicy{state: captureInvalid}
	}
	return parseCaptureBlock(data)
}

// parseCaptureBlock aplica a leitura estrita de [capture] (MF "Parsing and failure"). A política
// inteira é inválida ao primeiro problema: chave desconhecida, valor que não é array, entrada que
// não é string, TOML inválido, texto que não é UTF-8 ou conteúdo acima de 64 KiB.
func parseCaptureBlock(data []byte) capturePolicy {
	invalid := capturePolicy{state: captureInvalid}
	if len(data) > markerCaptureMaxBytes || !utf8.Valid(data) {
		return invalid
	}
	tree, err := toml.Parser().Unmarshal(data)
	if err != nil {
		return invalid
	}
	raw, ok := tree["capture"]
	if !ok {
		return capturePolicy{state: captureInactive}
	}
	table, ok := raw.(map[string]interface{})
	if !ok {
		return invalid
	}
	for key := range table {
		if key != "ignore_paths" {
			return invalid
		}
	}
	rawPaths, ok := table["ignore_paths"]
	if !ok {
		return capturePolicy{state: captureInactive}
	}
	list, ok := rawPaths.([]interface{})
	if !ok {
		return invalid
	}
	paths := make([]string, 0, len(list))
	for _, v := range list {
		s, ok := v.(string)
		if !ok {
			return invalid
		}
		paths = append(paths, s)
	}
	if len(paths) == 0 {
		return capturePolicy{state: captureInactive}
	}
	return capturePolicy{state: captureActive, ignorePaths: paths}
}

// markerDeclaresScope diz se o conteúdo contém alguma chave de markerScopeKeys, por linha.
func markerDeclaresScope(data []byte) bool {
	for _, key := range markerScopeKeys {
		if _, ok := markerRawValue(data, key); ok {
			return true
		}
	}
	return false
}

// markerRawValue devolve o texto depois do `=` da primeira linha que começa com key seguido de `=`.
// Cabeçalhos de seção não são rastreados, e a primeira linha que casa vence (MF "File format and reading").
func markerRawValue(data []byte, key string) (string, bool) {
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if !strings.HasPrefix(line, key) {
			continue
		}
		rest := strings.TrimLeft(line[len(key):], " \t")
		if !strings.HasPrefix(rest, "=") {
			continue
		}
		return rest[1:], true
	}
	return "", false
}

// markerString lê um valor de string com aspas duplas. O valor termina na próxima aspa dupla.
// Valor nu ou sem aspa de fechamento não é lido.
func markerString(data []byte, key string) (string, bool) {
	rest, ok := markerRawValue(data, key)
	if !ok {
		return "", false
	}
	rest = strings.TrimLeft(rest, " \t")
	if !strings.HasPrefix(rest, `"`) {
		return "", false
	}
	rest = rest[1:]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		return "", false
	}
	return rest[:end], true
}

// markerFlag lê uma flag verdadeira ou falsa, com ou sem aspas, sem diferenciar caixa.
// Comentário `#` inline é removido. Valor fora dos conjuntos devolve ok false.
func markerFlag(data []byte, key string) (value bool, ok bool) {
	rest, found := markerRawValue(data, key)
	if !found {
		return false, false
	}
	if i := strings.IndexByte(rest, '#'); i >= 0 {
		rest = rest[:i]
	}
	tok := strings.TrimSpace(rest)
	if len(tok) >= 2 && tok[0] == '"' && tok[len(tok)-1] == '"' {
		tok = tok[1 : len(tok)-1]
	}
	switch strings.ToLower(tok) {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off":
		return false, true
	}
	return false, false
}

// markerSamePath compara caminhos. No Windows a comparação ignora caixa.
func markerSamePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// markerWithin diz se p está em base ou abaixo dele, por componentes.
func markerWithin(p, base string) bool {
	if markerSamePath(p, base) {
		return true
	}
	prefix := base
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	if len(p) < len(prefix) {
		return false
	}
	head := p[:len(prefix)]
	if runtime.GOOS == "windows" {
		return strings.EqualFold(head, prefix)
	}
	return head == prefix
}
