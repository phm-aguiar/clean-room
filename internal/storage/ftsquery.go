package storage

import (
	"database/sql"
	"strings"
	"unicode"
)

// ftsProbeDDL é a tabela usada pela sonda. Espelha as colunas e o tokenizer de
// pages_fts (V01__init.sql), para que a sonda aceite a mesma sintaxe que a tabela real.
const ftsProbeDDL = `CREATE VIRTUAL TABLE probe USING fts5(
  title, body, path_search,
  tokenize="unicode61 remove_diacritics 2 tokenchars '/_-'"
)`

// ftsStopWords é a lista embutida de 57 palavras de parada em inglês (ADR-31).
// Comparação ignora caixa e não ignora diacríticos. O literal é um mapa, então
// uma palavra repetida é erro de compilação; a contagem exata é checada em teste.
var ftsStopWords = map[string]struct{}{
	"a": {}, "about": {}, "after": {}, "again": {}, "all": {}, "am": {}, "an": {},
	"and": {}, "any": {}, "are": {}, "as": {}, "at": {}, "be": {}, "because": {},
	"been": {}, "before": {}, "being": {}, "but": {}, "by": {}, "can": {}, "could": {},
	"did": {}, "do": {}, "does": {}, "down": {}, "for": {}, "from": {}, "had": {},
	"has": {}, "have": {}, "he": {}, "her": {}, "him": {}, "his": {}, "how": {},
	"if": {}, "in": {}, "into": {}, "is": {}, "it": {}, "its": {}, "me": {},
	"more": {}, "most": {}, "no": {}, "nor": {}, "not": {}, "of": {}, "off": {},
	"on": {}, "or": {}, "the": {}, "this": {}, "that": {}, "to": {}, "was": {},
	"with": {},
}

// asciiPunct são os caracteres de pontuação ASCII que o tokenizer não trata como
// parte de termo (os tokenchars '/_-' ficam de fora de propósito: também são citados).
const asciiPunct = "!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~"

// ftsColumns são as únicas colunas preservadas quando aparecem como `coluna:`.
var ftsColumns = []string{"title", "body"}

// prepareFTS5Query transforma texto livre numa consulta MATCH do FTS5 que não
// produz erro de sintaxe (SR §11.2, ADR-31).
//
// Regras:
//   - Com sintaxe explícita (aspas, parênteses, ou OR/AND/NOT/NEAR em maiúsculas,
//     que são os únicos operadores que o FTS5 reconhece) a consulta vai direto à
//     sonda, sem remover stop words nem trocar por OR.
//   - Sem sintaxe explícita: remove stop words (se só há stop words, mantém
//     todas) e une os termos com OR. Dois-pontos sem coluna conhecida viram
//     espaço e o termo é citado; `title:` e `body:` são preservados; tokens com
//     pontuação ASCII são citados; um `*` final passa sem citação.
//   - A candidata é validada pela sonda. Se a sonda recusa, degrada para palavras
//     citadas unidas por OR.
//
// Devolve "" para consulta vazia ou sem nenhum termo (só pontuação). O chamador
// deve pular a consulta nesse caso. Não há panic nem I/O em disco: a sonda usa um
// banco em memória.
//
// A sonda é feita por chamada (um banco em memória por chamada): é a forma mais
// simples, não há estado global nem cache a invalidar, e o custo é pequeno frente
// à busca.
func prepareFTS5Query(q string) string {
	trimmed := strings.TrimSpace(q)
	if trimmed == "" {
		return ""
	}

	if hasExplicitFTSSyntax(trimmed) {
		if probeFTS5(trimmed) {
			return trimmed
		}
		return degradeFTS5(trimmed)
	}

	candidate := buildNakedFTSQuery(trimmed)
	if candidate == "" {
		return ""
	}
	if probeFTS5(candidate) {
		return candidate
	}
	return degradeFTS5(trimmed)
}

// hasExplicitFTSSyntax detecta aspas, parênteses ou operadores FTS5 em maiúsculas.
func hasExplicitFTSSyntax(q string) bool {
	if strings.ContainsAny(q, `"()`) {
		return true
	}
	for _, w := range strings.Fields(q) {
		switch w {
		case "OR", "AND", "NOT", "NEAR":
			return true
		}
	}
	return false
}

// ftsItem é um termo da consulta nua antes da renderização.
type ftsItem struct {
	stem   string // texto do termo, sem coluna, sem '*' final
	column string // "title", "body" ou "" (sem coluna)
	quote  bool   // cita o termo como frase literal
	prefix bool   // havia '*' final
}

func (it ftsItem) render() string {
	s := it.stem
	if it.quote {
		s = `"` + s + `"`
	}
	if it.prefix {
		s += "*"
	}
	if it.column != "" {
		s = it.column + ":" + s
	}
	return s
}

// buildNakedFTSQuery monta a consulta de texto sem sintaxe explícita. Devolve ""
// se não sobra nenhum termo com letra ou dígito.
func buildNakedFTSQuery(q string) string {
	items := parseNakedFTS(q)
	if len(items) == 0 {
		return ""
	}

	kept := make([]ftsItem, 0, len(items))
	for _, it := range items {
		if !it.prefix {
			if _, stop := ftsStopWords[strings.ToLower(it.stem)]; stop {
				continue
			}
		}
		kept = append(kept, it)
	}
	if len(kept) == 0 {
		kept = items
	}

	parts := make([]string, len(kept))
	for i, it := range kept {
		parts[i] = it.render()
	}
	return strings.Join(parts, " OR ")
}

// parseNakedFTS separa a consulta em termos. Cada token de espaço vira zero ou mais
// termos: `title:`/`body:` com valor preservam a coluna; dois-pontos restantes
// viram espaço e os termos resultantes são citados.
func parseNakedFTS(q string) []ftsItem {
	var items []ftsItem
	for _, raw := range strings.Fields(q) {
		column := ""
		body := raw
		for _, col := range ftsColumns {
			n := len(col) + 1
			if len(raw) > n && strings.EqualFold(raw[:n], col+":") {
				column = col
				body = raw[n:]
				break
			}
		}

		core := strings.TrimRight(body, "*")
		prefix := len(core) < len(body)
		// Com coluna conhecida, a pontuação do valor decide a citação. Sem coluna,
		// qualquer dois-pontos cita o termo (o token original tinha pontuação).
		quote := hasASCIIPunct(core)
		if column == "" && strings.Contains(raw, ":") {
			quote = true
		}

		for _, sub := range strings.Fields(strings.ReplaceAll(core, ":", " ")) {
			if !hasLetterOrDigit(sub) {
				continue
			}
			items = append(items, ftsItem{
				stem:   sub,
				column: column,
				quote:  quote,
				prefix: prefix,
			})
		}
	}
	return items
}

// degradeFTS5 devolve as palavras (sem operadores) citadas e unidas por OR. Não
// remove stop words. Como nenhuma palavra contém aspas, a saída é sempre válida.
func degradeFTS5(q string) string {
	r := strings.NewReplacer(`"`, " ", "(", " ", ")", " ", ":", " ")
	var parts []string
	for _, w := range strings.Fields(r.Replace(q)) {
		w = strings.TrimRight(w, "*")
		if !hasLetterOrDigit(w) {
			continue
		}
		parts = append(parts, `"`+w+`"`)
	}
	return strings.Join(parts, " OR ")
}

// probeFTS5 indica se o analisador FTS5 aceita a consulta, usando uma tabela
// FTS5 em memória com o mesmo esquema de pages_fts.
func probeFTS5(candidate string) bool {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return false
	}
	defer func() { _ = db.Close() }()
	// Cada conexão em :memory: é um banco distinto; uma só conexão mantém a tabela.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(ftsProbeDDL); err != nil {
		return false
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM probe WHERE probe MATCH ?`, candidate).Scan(&n); err != nil {
		return false
	}
	return true
}

func hasASCIIPunct(s string) bool {
	return strings.ContainsAny(s, asciiPunct)
}

func hasLetterOrDigit(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}
