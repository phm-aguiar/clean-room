// Package archtest aplica as restrições arquiteturais da constituição como
// testes: grafo de imports entre componentes internos (Restrições
// Arquiteturais) e guarda de leitura de ambiente fora de internal/config
// (INV-01).
package archtest

import (
	"fmt"
	"go/build"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

const (
	modulePath     = "github.com/phm-aguiar/ai-memory-go"
	internalPrefix = modulePath + "/internal/"
)

// allowedImports é o grafo permitido. Chave: componente; valor: componentes
// internos que ele pode importar em código de produção. Um componente que não
// está aqui é violação: novo pacote interno exige decisão explícita.
// Constituição (Restrições Arquiteturais): mcp → domain, storage, wiki; mcp NÃO
// importa ingest em produção (só em testes). ingest → domain, storage, wiki.
var allowedImports = map[string][]string{
	"domain":   {},
	"config":   {},
	"archtest": {},
	"storage":  {"domain"},
	"wiki":     {"domain", "storage"},
	"ingest":   {"domain", "storage", "wiki"},
	"mcp":      {"domain", "storage", "wiki"},
	"cli":      {"domain", "config", "storage", "wiki", "ingest", "mcp"},
}

// mcpForbidden é a regra explícita da constituição: o pacote mcp nunca importa
// ingest em código de produção. Testes (_test.go) não entram em go list .Imports,
// então exceções de teste ficam permitidas por construção.
const mcpForbidden = internalPrefix + "ingest"

// envCallRe casa leituras de ambiente proibidas fora de internal/config.
var envCallRe = regexp.MustCompile(`os\.(Getenv|LookupEnv|Environ)\(`)

// componentOf extrai o componente de um import path interno. Retorna false
// para imports fora de internal/ do módulo.
func componentOf(importPath string) (string, bool) {
	if !strings.HasPrefix(importPath, internalPrefix) {
		return "", false
	}
	comp, _, _ := strings.Cut(strings.TrimPrefix(importPath, internalPrefix), "/")
	return comp, true
}

// checkImportGraph recebe pacote -> imports diretos de produção e devolve as
// violações do grafo permitido. Função pura, testável com entradas sintéticas.
func checkImportGraph(pkgs map[string][]string) []string {
	var violations []string
	for pkg, imports := range pkgs {
		comp, ok := componentOf(pkg)
		if !ok {
			continue
		}
		allowed, known := allowedImports[comp]
		if !known {
			violations = append(violations, fmt.Sprintf("%s: componente %q sem regra no grafo", pkg, comp))
			continue
		}
		allowedSet := make(map[string]bool, len(allowed))
		for _, a := range allowed {
			allowedSet[a] = true
		}
		for _, imp := range imports {
			ic, internal := componentOf(imp)
			if !internal || ic == comp {
				continue
			}
			if !allowedSet[ic] {
				violations = append(violations, fmt.Sprintf("%s importa %s (%s não é permitido para %s)", pkg, imp, ic, comp))
			}
		}
	}
	sort.Strings(violations)
	return violations
}

// goCommand localiza o binário go pelo PATH; se ausente, usa GOROOT do próprio
// binário de teste.
func goCommand() (string, error) {
	if p, err := exec.LookPath("go"); err == nil {
		return p, nil
	}
	exe := "go"
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	p := filepath.Join(build.Default.GOROOT, "bin", exe)
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("go não encontrado no PATH nem em %s", p)
	}
	return p, nil
}

// moduleRoot sobe a partir do diretório do teste até achar go.mod.
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod não encontrado acima do diretório do teste")
		}
		dir = parent
	}
}

// listImports executa `go list` em ./internal/... e devolve pacote -> imports
// diretos de produção (sem TestImports/XTestImports).
func listImports(root string) (map[string][]string, error) {
	goBin, err := goCommand()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(goBin, "list", "-e", "-f", `{{.ImportPath}} {{join .Imports " "}}`, "./internal/...")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = string(ee.Stderr)
		}
		return nil, fmt.Errorf("go list falhou: %v\n%s", err, stderr)
	}
	pkgs := make(map[string][]string)
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		pkgs[fields[0]] = fields[1:]
	}
	return pkgs, nil
}

// findEnvAccess devolve "arquivo:linha: trecho" para cada leitura de ambiente
// em arquivo de produção (não _test.go) sob internal/ e cmd/, fora de
// internal/config. Linhas de comentário (// ...) são ignoradas.
func findEnvAccess(root string) ([]string, error) {
	root = filepath.Clean(root)
	configDir := filepath.Join(root, "internal", "config")
	var hits []string
	for _, sub := range []string{"internal", "cmd"} {
		base := filepath.Join(root, sub)
		if _, err := os.Stat(base); os.IsNotExist(err) {
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
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			for i, line := range strings.Split(string(content), "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "//") {
					continue
				}
				if envCallRe.MatchString(trimmed) {
					hits = append(hits, fmt.Sprintf("%s:%d: %s", rel, i+1, trimmed))
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return hits, nil
}

func TestImportGraph(t *testing.T) {
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	pkgs, err := listImports(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) == 0 {
		t.Fatal("go list não encontrou pacotes em ./internal/...")
	}
	for _, v := range checkImportGraph(pkgs) {
		t.Error(v)
	}
}

// TestMCPNaoImportaIngest verifica, no grafo real de produção, que nenhum
// pacote sob internal/mcp importa internal/ingest (ou subpacotes). Regra
// explícita da constituição; testes de mcp podem importar ingest.
func TestMCPNaoImportaIngest(t *testing.T) {
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	pkgs, err := listImports(root)
	if err != nil {
		t.Fatal(err)
	}
	for pkg, imports := range pkgs {
		if comp, ok := componentOf(pkg); !ok || comp != "mcp" {
			continue
		}
		for _, imp := range imports {
			if imp == mcpForbidden || strings.HasPrefix(imp, mcpForbidden+"/") {
				t.Errorf("%s importa %s em produção: mcp não importa ingest (constituição)", pkg, imp)
			}
		}
	}
}

func TestImportGraphDetectsViolation(t *testing.T) {
	const (
		domain  = internalPrefix + "domain"
		storage = internalPrefix + "storage"
		mcp     = internalPrefix + "mcp"
		ingest  = internalPrefix + "ingest"
	)
	pkgs := map[string][]string{
		domain:                        {storage}, // violação: domain não importa ninguém
		storage:                       {domain},  // permitido
		mcp:                           {ingest},  // violação: mcp não importa ingest em produção
		"github.com/outro/modulo/pkg": {domain},  // externo: ignorado
	}
	got := checkImportGraph(pkgs)
	if len(got) != 2 {
		t.Fatalf("esperava 2 violações, veio %d: %v", len(got), got)
	}
	if !strings.Contains(got[0], "domain") || !strings.Contains(got[1], "mcp") {
		t.Fatalf("violações inesperadas: %v", got)
	}
}

func TestEnvGuard(t *testing.T) {
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	hits, err := findEnvAccess(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hits {
		t.Errorf("leitura de ambiente fora de internal/config (INV-01): %s", h)
	}
}

func TestEnvGuardDetectsViolation(t *testing.T) {
	tmp := t.TempDir() // removido automaticamente ao fim do teste
	files := map[string]string{
		// deve ser detectado
		filepath.Join("internal", "foo", "bad.go"): "package foo\n\nimport \"os\"\n\nfunc f() string { return os.Getenv(\"X\") }\n",
		// deve ser detectado (cmd/ também é guardado)
		filepath.Join("cmd", "app", "main.go"): "package main\n\nimport \"os\"\n\nfunc main() { _, _ = os.LookupEnv(\"Y\") }\n",
		// não deve ser detectado: comentário
		filepath.Join("internal", "foo", "comment.go"): "package foo\n\n// os.Getenv(\"Z\") só em comentário\n",
		// não deve ser detectado: dentro de internal/config
		filepath.Join("internal", "config", "ok.go"): "package config\n\nimport \"os\"\n\nfunc g() []string { return os.Environ() }\n",
		// não deve ser detectado: arquivo de teste
		filepath.Join("internal", "foo", "bad_test.go"): "package foo\n\nimport \"os\"\n\nfunc h() string { return os.Getenv(\"T\") }\n",
	}
	for rel, content := range files {
		full := filepath.Join(tmp, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	hits, err := findEnvAccess(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Fatalf("esperava 2 detecções, veio %d: %v", len(hits), hits)
	}
	wantBad := filepath.Join("internal", "foo", "bad.go")
	wantMain := filepath.Join("cmd", "app", "main.go")
	if !strings.Contains(hits[0], wantBad) && !strings.Contains(hits[1], wantBad) {
		t.Errorf("não detectou %s: %v", wantBad, hits)
	}
	if !strings.Contains(hits[0], wantMain) && !strings.Contains(hits[1], wantMain) {
		t.Errorf("não detectou %s: %v", wantMain, hits)
	}
}
