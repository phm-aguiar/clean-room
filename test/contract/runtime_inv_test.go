// Testes de contrato de runtime para as invariantes INV-01, INV-09, INV-11, INV-13 e
// INV-15 da constituição. Cada teste exercita o contrato pela superfície pública de
// internal/cli, internal/config, internal/wiki e internal/storage, sem duplicar a
// análise de grafo de internal/archtest. Todo arquivo temporário fica em t.TempDir().
package contract

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/cli"
	"github.com/phm-aguiar/ai-memory-go/internal/config"
	"github.com/phm-aguiar/ai-memory-go/internal/domain"
	"github.com/phm-aguiar/ai-memory-go/internal/storage"
	"github.com/phm-aguiar/ai-memory-go/internal/wiki"
)

// Chave e valor da marca interna do filtro INV-15 (espelham internal/cli/logging.go;
// o contrato é o atributo JSON, não o identificador não exportado).
const (
	originKey  = "ai_memory_origin"
	originSelf = "logging"
)

// envOrigemRe casa leituras de ambiente proibidas em código de produção.
var envOrigemRe = regexp.MustCompile(`os\.(Getenv|LookupEnv|Environ)\b`)

// configLoadRe casa chamadas a config.Load em código de produção.
var configLoadRe = regexp.MustCompile(`config\.Load\(`)

// isolateEnv remove do processo as variáveis AI_MEMORY_* e as de provedor LLM
// (LLM, OPENAI, ANTHROPIC, EMBED), para que o teste dependa só do que escreve.
// t.Setenv restaura os valores originais ao fim do teste.
func isolateEnv(t *testing.T) {
	t.Helper()
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if key == "" {
			continue
		}
		up := strings.ToUpper(key)
		if strings.HasPrefix(up, "AI_MEMORY_") || provedorLLM(up) {
			t.Setenv(key, "")
			_ = os.Unsetenv(key)
		}
	}
}

// provedorLLM diz se a chave da variável menciona um provedor de modelo.
func provedorLLM(up string) bool {
	for _, marca := range []string{"LLM", "OPENAI", "ANTHROPIC", "EMBED"} {
		if strings.Contains(up, marca) {
			return true
		}
	}
	return false
}

// fixture grava um config.toml com data_dir apontando para <tmp>/data e devolve as
// opções de config (TOML explícito, .env inexistente) e o data_dir.
func fixture(t *testing.T) (config.Options, string, string) {
	t.Helper()
	isolateEnv(t)
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "data")
	cfgPath := filepath.Join(tmp, "config.toml")
	// Literal TOML (aspas simples): barras invertidas do Windows não viram escapes.
	if err := os.WriteFile(cfgPath, []byte("data_dir = '"+dataDir+"'\n"), 0o600); err != nil {
		t.Fatalf("gravar config.toml: %v", err)
	}
	envPath := filepath.Join(tmp, "ausente.env")
	return config.Options{ConfigFile: cfgPath, EnvFile: envPath}, dataDir, tmp
}

// runRoot monta a raiz com os subcomandos reindex e backup e executa args com as
// flags globais de opts. Devolve a saída combinada e o erro do cobra.
func runRoot(t *testing.T, opts config.Options, args ...string) (string, error) {
	t.Helper()
	root := cli.NewRootCommand()
	root.AddCommand(cli.NewReindexCommand())
	root.AddCommand(cli.NewBackupCommand())
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	full := append(append([]string{}, args...),
		"--config", opts.ConfigFile, "--env-file", opts.EnvFile)
	root.SetArgs(full)
	err := root.Execute()
	return out.String(), err
}

// moduleRoot sobe a partir do diretório do teste até achar go.mod.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("go.mod não encontrado acima de %s", dir)
		}
		dir = parent
	}
}

// productionGoFiles devolve os caminhos (relativos à raiz) dos .go de produção sob
// internal/ e cmd/, sem _test.go e sem o diretório internal/config.
func productionGoFiles(t *testing.T, root string) []string {
	t.Helper()
	configDir := filepath.Join(root, "internal", "config")
	var files []string
	for _, sub := range []string{"internal", "cmd"} {
		base := filepath.Join(root, sub)
		if _, err := os.Stat(base); err != nil {
			continue
		}
		err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if path == configDir {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
				rel, rerr := filepath.Rel(root, path)
				if rerr != nil {
					return rerr
				}
				files = append(files, rel)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("percorrer %s: %v", sub, err)
		}
	}
	return files
}

// seedPage grava uma página pela API de escrita da wiki, com o escritor do App ativo.
func seedPage(t *testing.T, app *cli.App, scope domain.Scope, rel, body string) domain.PagePath {
	t.Helper()
	path, err := domain.ParsePagePath(rel)
	if err != nil {
		t.Fatalf("ParsePagePath(%q): %v", rel, err)
	}
	_, err = app.Store.WritePage(context.Background(), wiki.PageInput{
		Scope:         scope,
		WorkspaceName: "ws-contrato",
		ProjectName:   "proj-contrato",
		Path:          path,
		Tier:          domain.TierSemantic,
		Body:          body,
		Evidence:      storage.EvidenceSource{Kind: storage.SourceSession, ID: "sessao-contrato"},
	})
	if err != nil {
		t.Fatalf("WritePage(%q): %v", rel, err)
	}
	return path
}

// newScope cria um escopo com IDs novos (UUIDv7 via domain.NewID).
func newScope(t *testing.T) domain.Scope {
	t.Helper()
	ws, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	pj, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	return domain.NewScope(domain.WorkspaceID(ws), domain.ProjectID(pj))
}

// writePIDFile grava o marcador de PID do servidor em dataDir com o pid dado.
func writePIDFile(t *testing.T, dataDir string, pid int) {
	t.Helper()
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("criar data_dir: %v", err)
	}
	path := filepath.Join(dataDir, cli.PIDFileName)
	if err := os.WriteFile(path, []byte(strconv.Itoa(pid)), 0o600); err != nil {
		t.Fatalf("gravar marcador de PID: %v", err)
	}
}

// TestINV01_ConfigCarregadaUmaVez cobre INV-01 em três partes: (a) config.Load é
// chamada apenas a partir de internal/cli (o único caminho de composição); (b) existe
// exatamente um sítio de config.Load em código de produção, em root.go, dentro de
// loadConfig; (c) nenhum arquivo de produção fora de internal/config lê o ambiente. Esta é
// uma checagem de contrato independente da guarda de internal/archtest.
func TestINV01_ConfigCarregadaUmaVez(t *testing.T) {
	root := moduleRoot(t)
	files := productionGoFiles(t, root)
	if len(files) == 0 {
		t.Fatal("nenhum arquivo de produção encontrado em internal/ e cmd/")
	}

	var chamadores []string
	var sitios []string
	var envHits []string
	for _, rel := range files {
		content, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("ler %s: %v", rel, err)
		}
		sc := bufio.NewScanner(bytes.NewReader(content))
		linha := 0
		hasLoad := false
		funcAtual := ""
		for sc.Scan() {
			linha++
			bruto := sc.Text()
			if strings.HasPrefix(bruto, "func ") {
				funcAtual = bruto
			}
			trimmed := strings.TrimSpace(bruto)
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			if envOrigemRe.MatchString(trimmed) {
				envHits = append(envHits, rel+":"+strconv.Itoa(linha)+": "+trimmed)
			}
			if configLoadRe.MatchString(trimmed) {
				hasLoad = true
				sitios = append(sitios, filepath.ToSlash(rel)+":"+strconv.Itoa(linha)+" em "+funcAtual)
			}
		}
		if err := sc.Err(); err != nil {
			t.Fatalf("varrer %s: %v", rel, err)
		}
		if hasLoad {
			chamadores = append(chamadores, filepath.ToSlash(rel))
		}
	}

	for _, h := range envHits {
		t.Errorf("leitura de ambiente fora de internal/config (INV-01): %s", h)
	}

	if len(chamadores) == 0 {
		t.Fatal("nenhum código de produção chama config.Load")
	}
	for _, c := range chamadores {
		if !strings.HasPrefix(c, "internal/cli/") {
			t.Errorf("config.Load chamada fora da cli (INV-01 exige caminho único): %s", c)
		}
	}

	// Exatamente um sítio de produção, em root.go, dentro de loadConfig (chamada por
	// Compose e por runReindex).
	if len(sitios) != 1 {
		t.Errorf("config.Load deve aparecer exatamente 1 vez em produção (INV-01); achados: %d: %s",
			len(sitios), strings.Join(sitios, "; "))
	} else if !strings.HasPrefix(sitios[0], "internal/cli/root.go:") || !strings.Contains(sitios[0], "func loadConfig(") {
		t.Errorf("único sítio de config.Load deve ser em internal/cli/root.go dentro de loadConfig: %s", sitios[0])
	}

	// Carga real: config.Load com TOML explícito resolve o data_dir uma vez.
	opts, dataDir, _ := fixture(t)
	cfg, err := config.Load(opts)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if !filepath.IsAbs(cfg.DataDir()) {
		t.Fatalf("DataDir não é absoluto: %q", cfg.DataDir())
	}
	if cfg.DataDir() == "" || filepath.Base(cfg.DataDir()) != filepath.Base(dataDir) {
		t.Fatalf("DataDir %q não corresponde ao TOML %q", cfg.DataDir(), dataDir)
	}
}

// TestINV09_GuardaDeProcessoVivoAntesDeReindex cobre INV-09: com o marcador de PID
// apontando para o processo de teste (vivo), reindex é recusado com ErrProcessLive e
// nada é executado; backup online não é recusado pela guarda e roda com o escritor
// ativo, pela API de backup do SQLite.
func TestINV09_GuardaDeProcessoVivoAntesDeReindex(t *testing.T) {
	opts, dataDir, tmp := fixture(t)
	ctx := context.Background()

	app, err := cli.Compose(ctx, opts)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	defer func() {
		if cerr := app.Close(); cerr != nil {
			t.Errorf("fechar App: %v", cerr)
		}
	}()
	scope := newScope(t)
	seedPage(t, app, scope, "notas/guarda.md", "# Guarda\n\ncorpo da guarda")

	writePIDFile(t, dataDir, os.Getpid())

	out, err := runRoot(t, opts, "reindex")
	if err == nil {
		t.Fatalf("reindex executou com processo vivo; saída:\n%s", out)
	}
	if !errors.Is(err, cli.ErrProcessLive) {
		t.Fatalf("esperava erro encadeado com ErrProcessLive, veio %v", err)
	}
	if strings.Contains(out, "reindex concluído") {
		t.Fatalf("reindex imprimiu relatório com processo vivo:\n%s", out)
	}

	dest := filepath.Join(tmp, "snap.sqlite")
	if out, err := runRoot(t, opts, "backup", "--to", dest); err != nil {
		t.Fatalf("backup recusado ou falhou com processo vivo (deve ser online): %v\nsaída:\n%s", err, out)
	}
	raw, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("ler snapshot: %v", err)
	}
	if len(raw) < 16 || string(raw[:15]) != "SQLite format 3" {
		t.Fatalf("snapshot não tem cabeçalho SQLite: %d bytes", len(raw))
	}
}

// TestINV11_DataDirAbsolutoECanonicoNoLog cobre INV-11: o data_dir registrado por
// cli.LogStartup é absoluto, canônico e igual a config.Load(...).DataDir(). O TOML
// usa um caminho com "..", que a canonização deve resolver.
func TestINV11_DataDirAbsolutoECanonicoNoLog(t *testing.T) {
	isolateEnv(t)
	tmp := t.TempDir()
	dados := filepath.Join(tmp, "data")
	if err := os.MkdirAll(dados, 0o700); err != nil {
		t.Fatalf("criar data_dir: %v", err)
	}
	indireto := filepath.Join(tmp, "sub", "..", "data")
	cfgPath := filepath.Join(tmp, "config.toml")
	if err := os.WriteFile(cfgPath, []byte("data_dir = '"+indireto+"'\n"), 0o600); err != nil {
		t.Fatalf("gravar config.toml: %v", err)
	}
	opts := config.Options{ConfigFile: cfgPath, EnvFile: filepath.Join(tmp, "ausente.env")}

	cfg, err := config.Load(opts)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}

	var buf bytes.Buffer
	logger, err := cli.NewLogger(cfg, &buf)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	cli.LogStartup(logger, cfg)

	logged := dataDirDoRegistro(t, buf.Bytes())
	if logged == "" {
		t.Fatalf("registro de inicialização sem data_dir; saída: %s", buf.String())
	}
	if !filepath.IsAbs(logged) {
		t.Fatalf("data_dir no log não é absoluto: %q", logged)
	}
	if logged != cfg.DataDir() {
		t.Fatalf("data_dir no log %q difere de config.DataDir() %q", logged, cfg.DataDir())
	}
	if strings.Contains(logged, "..") {
		t.Fatalf("data_dir no log não está canonizado: %q", logged)
	}
	canon, err := filepath.EvalSymlinks(logged)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", logged, err)
	}
	if canon != logged {
		t.Fatalf("data_dir no log não é canônico: %q (canônico: %q)", logged, canon)
	}
	esperado, err := filepath.EvalSymlinks(dados)
	if err != nil {
		t.Fatalf("EvalSymlinks(real): %v", err)
	}
	if logged != esperado {
		t.Fatalf("data_dir no log %q, esperado %q", logged, esperado)
	}
}

// dataDirDoRegistro devolve o campo data_dir do registro de inicialização (msg
// "inicialização") de uma saída JSON de uma linha por registro.
func dataDirDoRegistro(t *testing.T, saida []byte) string {
	t.Helper()
	sc := bufio.NewScanner(bytes.NewReader(saida))
	for sc.Scan() {
		var reg map[string]any
		if err := json.Unmarshal(sc.Bytes(), &reg); err != nil {
			t.Fatalf("linha de log não é JSON: %v: %q", err, sc.Text())
		}
		if msg, _ := reg["msg"].(string); msg == "inicialização" {
			dd, _ := reg["data_dir"].(string)
			return dd
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("varrer log: %v", err)
	}
	return ""
}

// TestINV13_FuncionaSemProvedorLLM cobre INV-13: o fluxo de escrita, leitura e busca
// da wiki funciona sem nenhuma variável de provedor LLM. isolateEnv remove qualquer
// variável LLM/OPENAI/ANTHROPIC/EMBED e o teste confirma que nenhuma sobrou.
func TestINV13_FuncionaSemProvedorLLM(t *testing.T) {
	opts, _, _ := fixture(t)

	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if key != "" && provedorLLM(strings.ToUpper(key)) {
			t.Fatalf("variável de provedor LLM ainda presente no processo: %s", key)
		}
	}

	ctx := context.Background()
	app, err := cli.Compose(ctx, opts)
	if err != nil {
		t.Fatalf("Compose sem provedor: %v", err)
	}
	defer func() {
		if cerr := app.Close(); cerr != nil {
			t.Errorf("fechar App: %v", cerr)
		}
	}()

	scope := newScope(t)
	const termo = "termounicosemprovedor"
	path := seedPage(t, app, scope, "notas/sem-provedor.md", "# Sem provedor\n\n"+termo+" corpo da página")

	r, err := storage.OpenReader(ctx, app.Cfg.DataDir())
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	defer func() {
		if cerr := r.Close(); cerr != nil {
			t.Errorf("fechar leitor: %v", cerr)
		}
	}()
	reader := storage.NewReader(r)

	page, err := storage.ReadPage(ctx, reader, domain.NewPageRef(scope, path))
	if err != nil {
		t.Fatalf("ReadPage: %v", err)
	}
	if !strings.Contains(page.Body, termo) {
		t.Fatalf("ReadPage devolveu corpo sem o termo: %q", page.Body)
	}

	res, err := storage.Search(ctx, reader, scope, termo, time.Now().UTC(), 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Pages) == 0 {
		t.Fatalf("Search não encontrou a página escrita sem provedor")
	}
	if res.Pages[0].Ref.Path != path {
		t.Fatalf("Search primeiro resultado %q, esperado %q", res.Pages[0].Ref.Path, path)
	}
}

// TestINV15_LogIgnoraProprioModulo cobre INV-15: registros marcados com
// ai_memory_origin="logging" não chegam ao destino, nem na própria chamada nem em um
// logger derivado com With; registros sem a marca passam normalmente.
func TestINV15_LogIgnoraProprioModulo(t *testing.T) {
	isolateEnv(t)
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")
	if err := os.WriteFile(cfgPath, []byte("data_dir = '"+filepath.Join(tmp, "data")+"'\n"), 0o600); err != nil {
		t.Fatalf("gravar config.toml: %v", err)
	}
	cfg, err := config.Load(config.Options{ConfigFile: cfgPath, EnvFile: filepath.Join(tmp, "ausente.env")})
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}

	var buf bytes.Buffer
	logger, err := cli.NewLogger(cfg, &buf)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	logger.Info("sinal-visivel")
	logger.Info("sinal-marcado-direto", originKey, originSelf)
	logger.With(originKey, originSelf).Info("sinal-marcado-derivado")

	saida := buf.String()
	if !strings.Contains(saida, "sinal-visivel") {
		t.Fatalf("registro sem marca não chegou ao destino: %q", saida)
	}
	for _, oculto := range []string{"sinal-marcado-direto", "sinal-marcado-derivado"} {
		if strings.Contains(saida, oculto) {
			t.Errorf("registro marcado com origem própria foi reemitido (INV-15): %s\nsaída: %s", oculto, saida)
		}
	}
	if strings.Contains(saida, originSelf) && strings.Contains(saida, `"`+originKey+`":"`+originSelf+`"`) {
		t.Errorf("marca %s=%s vazou para o destino: %s", originKey, originSelf, saida)
	}
}
