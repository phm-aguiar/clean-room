package wiki

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// entradaLog é uma linha de log válida, com fim de linha LF.
const entradaLog = "## [2026-03-01T10:00:00Z] page-write | x\n"

func TestLedgerNameAceitaFormaExata(t *testing.T) {
	for _, name := range []string{"log.md", "log-2026-09.md", "log-1999-13.md"} {
		if !ledgerName(name) {
			t.Errorf("ledgerName(%q) = false, quer true", name)
		}
	}
}

func TestLedgerNameRecusaNomeFora(t *testing.T) {
	for _, name := range []string{
		"log-2026-9.md",      // mês com um dígito
		"log-2026-09.txt",    // extensão errada
		"log_2026-09.md",     // separador errado
		"log-2026-09.md.bak", // sufixo extra
		"xlog-2026-09.md",    // prefixo extra
		"log-abcd-ef.md",     // não são dígitos
		"log-2026-09x.md",    // sufixo colado
		"pagina-log.md",      // nome comum
		"log.md.tmp",         // temporário
		"LOG.md",             // caixa diferente: a forma é exata
		"log-20260-09.md",    // tamanho errado
		"log.md ",            // espaço no fim
	} {
		if ledgerName(name) {
			t.Errorf("ledgerName(%q) = true, quer false", name)
		}
	}
}

func TestLedgerContentAbreComEntrada(t *testing.T) {
	if !ledgerContent([]byte(entradaLog), true) {
		t.Fatal("ledger que abre com entrada recusado")
	}
	// Mais de uma entrada: decide a primeira.
	if !ledgerContent([]byte(entradaLog+entradaLog), true) {
		t.Fatal("ledger com duas entradas recusado")
	}
}

func TestLedgerContentAceitaFrontmatterFechado(t *testing.T) {
	// Migração OKF carimba `type: Note` nos ledgers.
	conteudo := "---\ntype: Note\n---\n" + entradaLog
	if !ledgerContent([]byte(conteudo), true) {
		t.Fatal("ledger com frontmatter fechado recusado")
	}
}

func TestLedgerContentAceitaCRLFeBOM(t *testing.T) {
	if !ledgerContent([]byte("## [2026-03-01T10:00:00Z] page-write | x\r\n"), true) {
		t.Fatal("entrada com CRLF recusada")
	}
	if !ledgerContent([]byte("\xEF\xBB\xBF"+entradaLog), true) {
		t.Fatal("entrada com BOM recusada")
	}
}

func TestLedgerContentRecusaSemEntradaNoInicio(t *testing.T) {
	for name, conteudo := range map[string]string{
		"texto comum":          "Uma página comum.\n" + entradaLog,
		"cabeçalho de página":  "# Título\n\n" + entradaLog,
		"entrada sem colchete": "## sem colchete\n",
		"vazio":                "",
		"só quebra de linha":   "\n",
	} {
		if ledgerContent([]byte(conteudo), true) {
			t.Errorf("%s: aceito como ledger, quer recusa", name)
		}
	}
}

func TestLedgerContentLinhaVaziaNoInicioDecideContra(t *testing.T) {
	if ledgerContent([]byte("\n"+entradaLog), true) {
		t.Fatal("linha vazia no início aceita como ledger")
	}
	// Linha vazia entre o frontmatter e a entrada também decide contra.
	if ledgerContent([]byte("---\ntype: Note\n---\n\n"+entradaLog), true) {
		t.Fatal("linha vazia depois do frontmatter aceita como ledger")
	}
}

func TestLedgerContentFrontmatterSemFechamentoEm64Linhas(t *testing.T) {
	// Linha 1 é a de abertura. Fechamento na linha 64 ainda vale.
	ate64 := "---\n" + strings.Repeat("k: v\n", 62) + "---\n" + entradaLog
	if !ledgerContent([]byte(ate64), true) {
		t.Fatal("frontmatter fechado na linha 64 recusado")
	}
	// Fechamento só na linha 65: sem fechamento dentro do limite, não é ledger.
	alem := "---\n" + strings.Repeat("k: v\n", 63) + "---\n" + entradaLog
	if ledgerContent([]byte(alem), true) {
		t.Fatal("frontmatter sem fechamento em 64 linhas aceito como ledger")
	}
	semFechar := "---\n" + strings.Repeat("k: v\n", 80) + entradaLog
	if ledgerContent([]byte(semFechar), true) {
		t.Fatal("frontmatter nunca fechado aceito como ledger")
	}
}

func TestLedgerContentLimiteDe8KiB(t *testing.T) {
	if ledgerPrefixBytes != 8192 {
		t.Fatalf("ledgerPrefixBytes = %d, quer 8192", ledgerPrefixBytes)
	}
	// Frontmatter que fecha antes do limite: aceito.
	dentro := "---\nk: " + strings.Repeat("a", 4000) + "\n---\n" + entradaLog
	if !ledgerContent([]byte(dentro), true) {
		t.Fatal("frontmatter que fecha dentro de 8 KiB recusado")
	}
	// Mesma forma, mas o fechamento cai depois de 8 KiB: o prefixo corta antes da decisão.
	alem := "---\nk: " + strings.Repeat("a", 9000) + "\n---\n" + entradaLog
	prefixo := []byte(alem)[:ledgerPrefixBytes]
	if ledgerContent(prefixo, false) {
		t.Fatal("decisão tomada depois do limite de 8 KiB")
	}
}

func TestLedgerContentEntradaCortadaPeloLimiteRecusa(t *testing.T) {
	// Entrada sem fim de linha dentro do prefixo, e o arquivo continua: não decide.
	if ledgerContent([]byte("## [2026-03-01T10:00:00Z] page-wr"), false) {
		t.Fatal("entrada cortada pelo limite aceita")
	}
	// Mesmo texto, arquivo inteiro e sem `\n` final: a entrada é completa.
	if !ledgerContent([]byte("## [2026-03-01T10:00:00Z] page-write | x"), true) {
		t.Fatal("entrada sem fim de linha no arquivo inteiro recusada")
	}
}

func TestLedgerFileCombinaNomeEConteudo(t *testing.T) {
	dir := t.TempDir()
	escreve := func(nome, conteudo string) string {
		t.Helper()
		p := filepath.Join(dir, nome)
		if err := os.WriteFile(p, []byte(conteudo), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", nome, err)
		}
		return p
	}

	// Nome exato e conteúdo de ledger: é ledger.
	ok, err := isLedgerFile(escreve("log-2026-09.md", entradaLog))
	if err != nil || !ok {
		t.Fatalf("log-2026-09.md com entrada: ok=%v err=%v, quer true", ok, err)
	}
	// Nome exato, mas conteúdo de página: não é ledger. O nome sozinho não basta.
	ok, err = isLedgerFile(escreve("log-2026-10.md", "Uma página com nome parecido.\n"))
	if err != nil || ok {
		t.Fatalf("log-2026-10.md com página: ok=%v err=%v, quer false", ok, err)
	}
	// Conteúdo de ledger, mas nome que não casa: não é ledger, e nem precisa ler.
	ok, err = isLedgerFile(escreve("notas.md", entradaLog))
	if err != nil || ok {
		t.Fatalf("notas.md com entrada: ok=%v err=%v, quer false", ok, err)
	}
	// Arquivo inexistente com nome válido: erro, não ledger.
	ok, err = isLedgerFile(filepath.Join(dir, "log.md"))
	if err == nil || ok {
		t.Fatalf("log.md inexistente: ok=%v err=%v, quer erro e false", ok, err)
	}
}
