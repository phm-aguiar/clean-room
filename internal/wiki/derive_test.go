package wiki

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
	"github.com/phm-aguiar/ai-memory-go/internal/storage"
)

// fmDe monta um frontmatter com pares chave, valor, e falha o teste em erro.
func fmDe(t *testing.T, pairs ...any) domain.Frontmatter {
	t.Helper()
	var fm domain.Frontmatter
	for i := 0; i < len(pairs); i += 2 {
		if err := fm.Set(pairs[i].(string), pairs[i+1]); err != nil {
			t.Fatalf("Set(%v): %v", pairs[i], err)
		}
	}
	return fm
}

func pagePath(t *testing.T, s string) domain.PagePath {
	t.Helper()
	p, err := domain.ParsePagePath(s)
	if err != nil {
		t.Fatalf("ParsePagePath(%q): %v", s, err)
	}
	return p
}

// linkStr dá a forma canônica de um link: [ws/][pj:]caminho[ rel=relação].
func linkStr(l storage.Link) string {
	var b strings.Builder
	if l.ToWorkspace != nil {
		b.WriteString(*l.ToWorkspace + "/")
	}
	if l.ToProject != nil {
		b.WriteString(*l.ToProject + ":")
	}
	b.WriteString(l.ToPath.String())
	if l.Relation != "" {
		b.WriteString(" rel=" + l.Relation)
	}
	return b.String()
}

func linkStrs(ls []storage.Link) []string {
	out := make([]string, 0, len(ls))
	for _, l := range ls {
		out = append(out, linkStr(l))
	}
	return out
}

func TestDeriveTitle_FrontmatterVence(t *testing.T) {
	fm := fmDe(t, "title", "  Título do fm  ")
	got := DeriveTitle(fm, "# Outro\n", "x.md")
	if got != "Título do fm" {
		t.Fatalf("got %q, want %q", got, "Título do fm")
	}
}

func TestDeriveTitle_FrontmatterVazioCaiParaCorpo(t *testing.T) {
	fm := fmDe(t, "title", "   ")
	if got := DeriveTitle(fm, "# Corpo\n", "x.md"); got != "Corpo" {
		t.Fatalf("got %q, want %q", got, "Corpo")
	}
}

func TestDeriveTitle_HeadingComBlocoDeCodigoAntes(t *testing.T) {
	body := "```go\nx := 1\n```\n\n# Título real\n\ntexto\n"
	if got := DeriveTitle(domain.Frontmatter{}, body, "x.md"); got != "Título real" {
		t.Fatalf("got %q, want %q", got, "Título real")
	}
}

func TestDeriveTitle_BlocoDeCodigoNaoEhPulado(t *testing.T) {
	// okf: a busca é por linha e não pula blocos de código, então `# ` dentro da cerca conta.
	body := "```\n# dentro do bloco\n```\n# Depois\n"
	if got := DeriveTitle(domain.Frontmatter{}, body, "x.md"); got != "dentro do bloco" {
		t.Fatalf("got %q, want %q", got, "dentro do bloco")
	}
}

func TestDeriveTitle_HeadingVazioEhIgnorado(t *testing.T) {
	body := "#\n# \n## Sub\n#  Alvo  \r\n"
	if got := DeriveTitle(domain.Frontmatter{}, body, "x.md"); got != "Alvo" {
		t.Fatalf("got %q, want %q", got, "Alvo")
	}
}

func TestDeriveTitle_NomeDoArquivo(t *testing.T) {
	cases := map[string]string{
		"minha-pagina.md":           "minha-pagina",
		"notas/sub/minha-pagina.md": "minha-pagina",
		`notas\sub\outra.md`:        "outra",
	}
	for file, want := range cases {
		if got := DeriveTitle(domain.Frontmatter{}, "sem heading\n", file); got != want {
			t.Errorf("DeriveTitle(%q) = %q, want %q", file, got, want)
		}
	}
}

func TestDeriveTitle_SemLimiteDeTamanho(t *testing.T) {
	long := strings.Repeat("a", 300)
	if got := DeriveTitle(fmDe(t, "title", long), "", "x.md"); got != long {
		t.Fatalf("título truncado: len=%d", len(got))
	}
}

func TestDeriveExtractLinks_FormasDoCorpo(t *testing.T) {
	body := "Veja [[a]], [[proj:b]], [[ws/proj:c]], [[_global:d]] e [x](e.md#sec).\n"
	links, warns, err := ExtractLinks(body, domain.Frontmatter{}, pagePath(t, "notes/origem.md"))
	if err != nil {
		t.Fatalf("ExtractLinks: %v", err)
	}
	if len(warns) != 0 {
		t.Fatalf("avisos inesperados: %v", warns)
	}
	want := []string{
		"a.md",
		"notes/e.md",
		"proj:b.md",
		"default/_global:d.md",
		"ws/proj:c.md",
	}
	if got := linkStrs(links); !reflect.DeepEqual(got, want) {
		t.Fatalf("links = %v, want %v", got, want)
	}
}

// TestDeriveExtractLinks_GlobalFicaNoWorkspacePadrao: `[[_global:x]]` resolve para o workspace
// padrão e não para o de origem, a menos que se escreva `workspace/_global` (wiki-files-layer, T083).
func TestDeriveExtractLinks_GlobalFicaNoWorkspacePadrao(t *testing.T) {
	links, _, err := ExtractLinks("[[_global:d]] e [[outro/_global:e]]", domain.Frontmatter{}, pagePath(t, "a.md"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"default/_global:d.md", "outro/_global:e.md"}
	if got := linkStrs(links); !reflect.DeepEqual(got, want) {
		t.Fatalf("links = %v, want %v", got, want)
	}
}

func TestDeriveExtractLinks_EscopoDoWikilink(t *testing.T) {
	links, _, err := ExtractLinks("[[ws/proj:c]]", domain.Frontmatter{}, pagePath(t, "a.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 {
		t.Fatalf("len = %d, want 1", len(links))
	}
	l := links[0]
	if l.ToWorkspace == nil || *l.ToWorkspace != "ws" || l.ToProject == nil || *l.ToProject != "proj" {
		t.Fatalf("escopo errado: ws=%v pj=%v", l.ToWorkspace, l.ToProject)
	}
	if l.ToPath.String() != "c.md" || l.Relation != "" {
		t.Fatalf("caminho/relação errados: %q %q", l.ToPath.String(), l.Relation)
	}
}

func TestDeriveExtractLinks_RotuloDoWikilinkEDescartado(t *testing.T) {
	links, _, _ := ExtractLinks("[[alvo|texto visível]]", domain.Frontmatter{}, pagePath(t, "a.md"))
	if got := linkStrs(links); !reflect.DeepEqual(got, []string{"alvo.md"}) {
		t.Fatalf("links = %v", got)
	}
}

func TestDeriveExtractLinks_IgnoradosNoCorpo(t *testing.T) {
	body := strings.Join([]string{
		"[s](https://exemplo.com/a.md)",
		"![x](y.png)",
		"![[embutido]]",
		"[m](mailto:alguem@exemplo.com)",
		"[ancora](#so-ancora)",
		"[pasta](pasta/)",
		"[txt](arquivo.txt)",
		"[barra](pasta\\arq.md)",
		"[v](versao-1.2)",
		"[ok](ok.md)",
		"```",
		"[[no]]",
		"```",
		"Inline `[[no]]` aqui e [[sim]].",
	}, "\n")
	links, _, err := ExtractLinks(body, domain.Frontmatter{}, pagePath(t, "origem.md"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"ok.md", "sim.md"}
	if got := linkStrs(links); !reflect.DeepEqual(got, want) {
		t.Fatalf("links = %v, want %v", got, want)
	}
}

func TestDeriveExtractLinks_CercaSemFechamentoIgnoraAteOFim(t *testing.T) {
	links, _, _ := ExtractLinks("```\n[[no]]\n[[tambem]]", domain.Frontmatter{}, pagePath(t, "a.md"))
	if len(links) != 0 {
		t.Fatalf("links = %v, want none", linkStrs(links))
	}
}

func TestDeriveExtractLinks_CrasSemFechamentoSaoTexto(t *testing.T) {
	links, _, _ := ExtractLinks("abre ` aqui [[sim]]", domain.Frontmatter{}, pagePath(t, "a.md"))
	if got := linkStrs(links); !reflect.DeepEqual(got, []string{"sim.md"}) {
		t.Fatalf("links = %v", got)
	}
}

func TestDeriveExtractLinks_QueryEAncoraSaemAntesDeResolver(t *testing.T) {
	links, _, _ := ExtractLinks("[q](a.md?x=1#s) [r](/raiz.md) [u](../up.md)", domain.Frontmatter{}, pagePath(t, "notes/x/origem.md"))
	want := []string{"notes/up.md", "notes/x/a.md", "raiz.md"}
	if got := linkStrs(links); !reflect.DeepEqual(got, want) {
		t.Fatalf("links = %v, want %v", got, want)
	}
}

func TestDeriveExtractLinks_DedupeEOrdenacao(t *testing.T) {
	links, _, _ := ExtractLinks("[[b]] [[a]] [[a]] [a](a.md)", domain.Frontmatter{}, pagePath(t, "origem.md"))
	if got := linkStrs(links); !reflect.DeepEqual(got, []string{"a.md", "b.md"}) {
		t.Fatalf("links = %v", got)
	}
}

func TestDeriveExtractLinks_DestinoLongoNaoEhLink(t *testing.T) {
	long := strings.Repeat("a", 600) + ".md"
	links, _, _ := ExtractLinks("[l]("+long+")", domain.Frontmatter{}, pagePath(t, "origem.md"))
	if len(links) != 0 {
		t.Fatalf("destino acima de 512 bytes não devia ser link: %v", linkStrs(links))
	}
}

func TestDeriveExtractLinks_RelacoesComoLista(t *testing.T) {
	fm := fmDe(t, relationsKey, []string{"causes:proj:b", "fixes:c", "xpto:d"})
	links, warns, err := ExtractLinks("", fm, pagePath(t, "origem.md"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"c.md rel=fixes", "d.md", "proj:b.md rel=causes"}
	if got := linkStrs(links); !reflect.DeepEqual(got, want) {
		t.Fatalf("links = %v, want %v", got, want)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "xpto") {
		t.Fatalf("avisos = %v, want 1 com relação desconhecida", warns)
	}
}

func TestDeriveExtractLinks_RelacaoComoString(t *testing.T) {
	fm := fmDe(t, relationsKey, "contradicts:ws/proj:e")
	links, warns, err := ExtractLinks("", fm, pagePath(t, "origem.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 0 {
		t.Fatalf("avisos inesperados: %v", warns)
	}
	if got := linkStrs(links); !reflect.DeepEqual(got, []string{"ws/proj:e.md rel=contradicts"}) {
		t.Fatalf("links = %v", got)
	}
}

func TestDeriveExtractLinks_AlvoDeRelacaoInvalidoAvisa(t *testing.T) {
	fm := fmDe(t, relationsKey, []string{"fixes:pasta/", "causes:arq.txt", "fixes", "fixes:../fora"})
	links, warns, err := ExtractLinks("", fm, pagePath(t, "origem.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 0 {
		t.Fatalf("links = %v, want none", linkStrs(links))
	}
	if len(warns) != 4 {
		t.Fatalf("avisos = %v, want 4", warns)
	}
}

func TestDeriveExtractLinks_AvisoTruncadoEm200Bytes(t *testing.T) {
	rel := strings.Repeat("a", 300)
	fm := fmDe(t, relationsKey, []string{rel + ":alvo"})
	_, warns, err := ExtractLinks("", fm, pagePath(t, "origem.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 1 {
		t.Fatalf("avisos = %v", warns)
	}
	if !strings.Contains(warns[0], strings.Repeat("a", 200)) || strings.Contains(warns[0], strings.Repeat("a", 201)) {
		t.Fatalf("aviso não truncado em 200 bytes: %q", warns[0])
	}
}

func TestDeriveParseExpiresAt(t *testing.T) {
	fim := time.Date(2026, 10, 12, 23, 59, 59, 999999000, time.UTC).UnixMicro()
	cases := []struct {
		name string
		in   string
		want int64
	}{
		{"vazio", "", 0},
		{"só espaços", "   ", 0},
		{"rfc3339 utc", "2026-10-12T10:00:00Z", time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC).UnixMicro()},
		{"rfc3339 com fuso", "2026-10-12T07:00:00-03:00", time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC).UnixMicro()},
		{"data pura fim do dia", "2026-10-12", fim},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseExpiresAt(c.in)
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if got != c.want {
				t.Fatalf("got %d, want %d", got, c.want)
			}
		})
	}
}

func TestDeriveParseExpiresAt_Invalido(t *testing.T) {
	for _, in := range []string{"2026-02-30", "2026-10-12T10:00:00", "amanhã", "12/10/2026"} {
		_, err := ParseExpiresAt(in)
		if err == nil {
			t.Errorf("ParseExpiresAt(%q): esperava erro", in)
			continue
		}
		if !errors.Is(err, ErrInvalidExpiresAt) || !errors.Is(err, ErrInvalidFrontmatter) {
			t.Errorf("ParseExpiresAt(%q): erro sem sentinela: %v", in, err)
		}
	}
}

func TestDeriveStaleAfter(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		want   string
		wantOK bool
	}{
		{"rfc3339 copiado", "2026-10-12T10:00:00+02:00", "2026-10-12T10:00:00+02:00", true},
		{"data pura vira fim do dia", "2026-10-12", "2026-10-12T23:59:59.999999Z", true},
		{"vazio nao cria", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok, err := DeriveStaleAfter(c.in)
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if got != c.want || ok != c.wantOK {
				t.Fatalf("got (%q, %v), want (%q, %v)", got, ok, c.want, c.wantOK)
			}
		})
	}
}

func TestDeriveStaleAfter_Invalido(t *testing.T) {
	got, ok, err := DeriveStaleAfter("lixo")
	if !errors.Is(err, ErrInvalidExpiresAt) {
		t.Fatalf("esperava ErrInvalidExpiresAt, veio %v", err)
	}
	if got != "" || ok {
		t.Fatalf("não devia criar nada: (%q, %v)", got, ok)
	}
}
