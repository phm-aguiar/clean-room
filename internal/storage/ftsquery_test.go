package storage

import (
	"database/sql"
	"strings"
	"testing"
)

// ftsMatchDB é uma tabela FTS5 real, com o esquema de pages_fts, usada para provar
// que as saídas são aceitas pelo MATCH de verdade (não só pela sonda).
func ftsMatchDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(ftsProbeDDL); err != nil {
		t.Fatalf("create fts table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO probe(title, body, path_search) VALUES
		('cats and dogs', 'the current.md handoff pick wiki body', 'wiki/current.md')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	return db
}

// ftsMatchErr devolve o erro do MATCH real, se houver.
func ftsMatchErr(db *sql.DB, q string) error {
	var n int
	return db.QueryRow(`SELECT COUNT(*) FROM probe WHERE probe MATCH ?`, q).Scan(&n)
}

func TestFTSStopWordsCount(t *testing.T) {
	if got := len(ftsStopWords); got != 57 {
		t.Fatalf("stop words: got %d, want 57", got)
	}
}

func TestFTSStopWordsRemovedAndJoinedWithOR(t *testing.T) {
	if got := prepareFTS5Query("the cats and dogs"); got != "cats OR dogs" {
		t.Fatalf("got %q, want %q", got, "cats OR dogs")
	}
}

func TestFTSStopWordsIgnoreCase(t *testing.T) {
	if got := prepareFTS5Query("THE Cats"); got != "Cats" {
		t.Fatalf("got %q, want %q", got, "Cats")
	}
}

func TestFTSStopWordsDoNotIgnoreDiacritics(t *testing.T) {
	// "thé" não é a stop word "the": o acento não é removido na comparação.
	if got := prepareFTS5Query("thé cats"); got != "thé OR cats" {
		t.Fatalf("got %q, want %q", got, "thé OR cats")
	}
}

func TestFTSOnlyStopWordsKeepsAll(t *testing.T) {
	if got := prepareFTS5Query("the and of"); got != "the OR and OR of" {
		t.Fatalf("got %q, want %q", got, "the OR and OR of")
	}
}

func TestFTSExplicitSyntaxKeepsStopWords(t *testing.T) {
	cases := []string{
		"the OR cats",
		"the AND cats",
		"the NOT cats",
		"the NEAR(cats dogs, 3)",
		`"the cats" dogs`,
		"(the cats)",
	}
	for _, q := range cases {
		if got := prepareFTS5Query(q); got != q {
			t.Errorf("prepareFTS5Query(%q) = %q, want the query unchanged", q, got)
		}
	}
}

func TestFTSExplicitSyntaxLowercaseOperatorIsNaked(t *testing.T) {
	// FTS5 só reconhece operadores em maiúsculas: "or" é palavra comum (stop word).
	if got := prepareFTS5Query("cats or dogs"); got != "cats OR dogs" {
		t.Fatalf("got %q, want %q", got, "cats OR dogs")
	}
}

func TestFTSColonWithoutColumnBecomesTerm(t *testing.T) {
	if got := prepareFTS5Query("pick: handoff"); got != `"pick" OR handoff` {
		t.Fatalf("got %q, want %q", got, `"pick" OR handoff`)
	}
	if got := prepareFTS5Query("pick:handoff"); got != `"pick" OR "handoff"` {
		t.Fatalf("got %q, want %q", got, `"pick" OR "handoff"`)
	}
}

func TestFTSKnownColumnsPreserved(t *testing.T) {
	if got := prepareFTS5Query("title:handoff body:wiki"); got != "title:handoff OR body:wiki" {
		t.Fatalf("got %q, want %q", got, "title:handoff OR body:wiki")
	}
}

func TestFTSPunctuationIsQuoted(t *testing.T) {
	if got := prepareFTS5Query("current.md"); got != `"current.md"` {
		t.Fatalf("got %q, want %q", got, `"current.md"`)
	}
}

func TestFTSFinalStarPassesUnquoted(t *testing.T) {
	if got := prepareFTS5Query("hand*"); got != "hand*" {
		t.Fatalf("got %q, want %q", got, "hand*")
	}
	if got := prepareFTS5Query("title:hand*"); got != "title:hand*" {
		t.Fatalf("got %q, want %q", got, "title:hand*")
	}
}

func TestFTSEmptyQueryReturnsEmpty(t *testing.T) {
	for _, q := range []string{"", "   ", "\t \n"} {
		if got := prepareFTS5Query(q); got != "" {
			t.Errorf("prepareFTS5Query(%q) = %q, want empty", q, got)
		}
	}
}

func TestFTSPunctuationOnlyReturnsEmpty(t *testing.T) {
	for _, q := range []string{"...", "*", ":::"} {
		if got := prepareFTS5Query(q); got != "" {
			t.Errorf("prepareFTS5Query(%q) = %q, want empty", q, got)
		}
	}
}

func TestFTSDegradationWhenProbeRejects(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{`"cats dogs`, `"cats" OR "dogs"`},
		{`cats OR`, `"cats" OR "OR"`},
		{`NOT`, `"NOT"`},
		{`title: (handoff`, `"title" OR "handoff"`},
	}
	for _, c := range cases {
		if got := prepareFTS5Query(c.in); got != c.want {
			t.Errorf("prepareFTS5Query(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFTSOutputsAcceptedByRealMatch(t *testing.T) {
	db := ftsMatchDB(t)
	inputs := []string{
		"",
		"the cats and dogs",
		"the OR cats",
		"pick: handoff",
		"pick:handoff",
		"title:handoff body:wiki",
		"title: handoff",
		"title:current.md",
		"current.md",
		"hand*",
		"current.md*",
		"title:hand*",
		`"cats dogs`,
		`cats OR`,
		`NOT`,
		`"`,
		`""`,
		`"""`,
		`(`,
		`)(`,
		`NEAR(`,
		`a AND`,
		`OR AND NOT NEAR`,
		`title:`,
		`body:"x`,
		`:::`,
		`-`,
		`a-b_c/d`,
		`\ ' ! @ # $ % ^ & [ ] { } | ; < > ?`,
		`cats*dogs`,
		`* *`,
		"tab\tsep\nnl",
		"é ü 名前 cats",
		"the and of",
	}
	for _, in := range inputs {
		out := prepareFTS5Query(in)
		if out == "" {
			continue
		}
		if err := ftsMatchErr(db, out); err != nil {
			t.Errorf("prepareFTS5Query(%q) = %q, MATCH error: %v", in, out, err)
		}
	}
}

func TestFTSDegradedOutputHasNoOperators(t *testing.T) {
	// Degradação: palavras citadas, sem aspas internas nem parênteses.
	out := degradeFTS5(`"a (b) c:d* e`)
	if strings.ContainsAny(out, "():*") {
		t.Fatalf("degraded output has unexpected characters: %q", out)
	}
	if err := ftsMatchErr(ftsMatchDB(t), out); err != nil {
		t.Fatalf("degraded output %q: MATCH error: %v", out, err)
	}
}
