package wiki

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
	"github.com/phm-aguiar/ai-memory-go/internal/storage"
)

// ErrInvalidExpiresAt é a causa de ParseExpiresAt e DeriveStaleAfter quando expires_at não é
// RFC 3339 nem AAAA-MM-DD. Também encadeia ErrInvalidFrontmatter, porque é falha fechada na
// escrita (wiki-files-layer §5.2).
var ErrInvalidExpiresAt = fmt.Errorf("expires_at inválido: %w", ErrInvalidFrontmatter)

// errAlvoNaoPagina marca alvo de link ou relação que não forma um PagePath válido.
var errAlvoNaoPagina = errors.New("alvo não nomeia página")

const (
	// maxWarnBytes é o limite de bytes de um valor vindo do frontmatter que entra em aviso.
	maxWarnBytes = 200
	// maxMarkdownDestBytes é o limite de bytes lidos do destino de um link markdown.
	maxMarkdownDestBytes = 512
	dateLayout           = "2006-01-02"
	staleDateSuffix      = "T23:59:59.999999Z"
	relationsKey         = "relations"
	// defaultWorkspaceName e globalProjectName formam o escopo reservado `default/_global`.
	defaultWorkspaceName = "default"
	globalProjectName    = "_global"
)

// ignoredSchemes são os esquemas de destino markdown que não são página (contrato (c)).
var ignoredSchemes = []string{"mailto:", "data:", "javascript:", "vbscript:", "tel:", "file:"}

// DeriveTitle deriva o título de uma página (markdown-contract (b)). Ordem: campo `title` do
// frontmatter, se for texto e não vazio após aparar; senão a primeira linha do corpo que começa
// com `# ` e tem texto depois, aparado (a busca é por linha e não pula blocos de código, okf);
// senão o nome do arquivo sem `.md` e sem diretórios. Não aplica limite de tamanho.
func DeriveTitle(fm domain.Frontmatter, body string, filename string) string {
	if v, ok := fm.Get("title"); ok {
		if s, ok := v.(string); ok {
			if t := strings.TrimSpace(s); t != "" {
				return t
			}
		}
	}
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "# ") {
			continue
		}
		if t := strings.TrimSpace(line[2:]); t != "" {
			return t
		}
	}
	base := filename
	if i := strings.LastIndexAny(base, `/\`); i >= 0 {
		base = base[i+1:]
	}
	return strings.TrimSuffix(base, ".md")
}

// ExtractLinks extrai os links de saída de uma página (markdown-contract (c)) e devolve
// links deduplicados e ordenados, mais avisos.
//
// Corpo: wikilinks [[caminho]], [[projeto:caminho]], [[workspace/projeto:caminho]], com rótulo
// [[alvo|rótulo]] aceito, e links markdown [rótulo](destino). Wikilinks são relativos à raiz do
// projeto. Destinos markdown iniciados por `/` são relativos à raiz; os demais, ao diretório de
// fromPath. Ignorados: código cercado e inline, imagens, URLs com `://`, mailto e outros
// esquemas, âncora pura, destinos com barra invertida ou barra no fim, e destinos com ponto que
// não terminam em `.md`. Consulta (`?`) e âncora (`#`) saem antes de resolver. Destino sem
// extensão recebe `.md`. Destino inválido é descartado em silêncio.
//
// Relações: chave `relations` do frontmatter, aceita como string (uma relação) ou []string.
// Cada item tem o formato `relação:alvo`, em que o alvo usa a mesma gramática dos wikilinks
// (`caminho`, `projeto:caminho` ou `workspace/projeto:caminho`). A divisão é no primeiro `:`,
// então `causes:proj:b` vira relação `causes` com alvo `proj:b`. Relação fora de
// causes|fixes|contradicts vira link de corpo (Relation vazio, references, ADR-19) e gera
// aviso. Alvo que não nomeia página é descartado e gera aviso. Valor no aviso é truncado a 200
// bytes.
//
// Erro só vem de `relations` com tipo que o domínio não representa.
func ExtractLinks(body string, fm domain.Frontmatter, fromPath domain.PagePath) ([]storage.Link, []string, error) {
	found := make([]storage.Link, 0)
	for _, line := range maskCode(body) {
		found = scanLine(line, fromPath, found)
	}
	rels, warns, err := relationLinks(fm)
	if err != nil {
		return nil, nil, err
	}
	found = append(found, rels...)
	return dedupeSortLinks(found), warns, nil
}

// ParseExpiresAt lê expires_at e devolve o instante em microssegundos Unix UTC (wiki-files-layer
// §5.2; domain-vocabulary §6.7). Aceita RFC 3339 com fuso, em que o instante é o do texto, e
// AAAA-MM-DD, em que o instante é o fim do dia UTC (23:59:59,999999). Texto vazio ou só com
// espaços significa sem validade e devolve 0 sem erro. Outro valor devolve ErrInvalidExpiresAt.
func ParseExpiresAt(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	t, dateOnly, err := parseExpires(s)
	if err != nil {
		return 0, err
	}
	if dateOnly {
		t = endOfDay(t)
	}
	return t.UnixMicro(), nil
}

// DeriveStaleAfter devolve o valor de stale_after para um expires_at (okf "stale_after"). RFC 3339
// é copiado como está (aparado). Data pura vira AAAA-MM-DDT23:59:59.999999Z. Texto vazio não cria
// nada e devolve false sem erro. Outro valor não cria nada e devolve ErrInvalidExpiresAt.
func DeriveStaleAfter(expiresAt string) (string, bool, error) {
	s := strings.TrimSpace(expiresAt)
	if s == "" {
		return "", false, nil
	}
	t, dateOnly, err := parseExpires(s)
	if err != nil {
		return "", false, err
	}
	if !dateOnly {
		return s, true, nil
	}
	return t.Format(dateLayout) + staleDateSuffix, true, nil
}

// parseExpires aceita RFC 3339 (com fuso) ou AAAA-MM-DD. dateOnly indica o segundo caso.
func parseExpires(s string) (t time.Time, dateOnly bool, err error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, false, nil
	}
	if t, err := time.Parse(dateLayout, s); err == nil {
		return t, true, nil
	}
	return time.Time{}, false, fmt.Errorf("%w: %q", ErrInvalidExpiresAt, s)
}

// endOfDay devolve o último microssegundo do dia UTC de t (t é meia-noite UTC).
func endOfDay(t time.Time) time.Time {
	return t.Add(24*time.Hour - time.Microsecond)
}

// fence é um bloco cercado aberto: caractere (` ou ~) e tamanho da cerca.
type fence struct {
	ch byte
	n  int
}

// maskCode devolve as linhas do corpo com código cercado e código inline apagados (linha
// vazia ou espaços no lugar), para que a varredura de links não os enxergue. Cerca sem
// fechamento vai até o fim do documento, como no CommonMark.
func maskCode(body string) []string {
	lines := strings.Split(body, "\n")
	var open fence
	for i, line := range lines {
		if open.n > 0 {
			if closesFence(line, open) {
				open = fence{}
			}
			lines[i] = ""
			continue
		}
		if f, ok := openFence(line); ok {
			open = f
			lines[i] = ""
			continue
		}
		lines[i] = maskInline(line)
	}
	return lines
}

// openFence reconhece a linha de abertura de um bloco cercado: três ou mais crases ou tis.
func openFence(line string) (fence, bool) {
	t := strings.TrimLeft(line, " \t")
	if len(t) == 0 || (t[0] != '`' && t[0] != '~') {
		return fence{}, false
	}
	n := runLen(t, 0)
	if n < 3 {
		return fence{}, false
	}
	return fence{ch: t[0], n: n}, true
}

// closesFence diz se line fecha o bloco: só o mesmo caractere, em tamanho igual ou maior.
func closesFence(line string, f fence) bool {
	t := strings.TrimRight(strings.TrimLeft(line, " \t"), " \t\r")
	if len(t) == 0 || t[0] != f.ch {
		return false
	}
	n := runLen(t, 0)
	return n >= f.n && n == len(t)
}

// runLen conta o tamanho da sequência de t[from] repetido a partir de from.
func runLen(t string, from int) int {
	n := 0
	for from+n < len(t) && t[from+n] == t[from] {
		n++
	}
	return n
}

// maskInline apaga o código inline da linha. Uma sequência de n crases só fecha com outra de
// exatamente n crases na mesma linha. Sem fechamento, as crases são texto.
func maskInline(line string) string {
	b := []byte(line)
	for i := 0; i < len(b); {
		if b[i] != '`' {
			i++
			continue
		}
		start := i
		n := runLen(line, i)
		i += n
		end := findBackticks(b, i, n)
		if end < 0 {
			continue
		}
		for k := start; k < end+n; k++ {
			b[k] = ' '
		}
		i = end + n
	}
	return string(b)
}

// findBackticks procura, a partir de from, uma sequência de exatamente n crases.
func findBackticks(b []byte, from, n int) int {
	for j := from; j < len(b); {
		if b[j] != '`' {
			j++
			continue
		}
		s := j
		for j < len(b) && b[j] == '`' {
			j++
		}
		if j-s == n {
			return s
		}
	}
	return -1
}

// scanLine acrescenta a out os links de uma linha já sem código.
func scanLine(line string, from domain.PagePath, out []storage.Link) []storage.Link {
	for i := 0; i < len(line); {
		switch {
		case strings.HasPrefix(line[i:], "[["):
			end := strings.Index(line[i+2:], "]]")
			if end < 0 {
				i++
				continue
			}
			if l, ok := wikiLink(line[i+2 : i+2+end]); ok && !isImage(line, i) {
				out = append(out, l)
			}
			i += 2 + end + 2
		case strings.HasPrefix(line[i:], "]("):
			var next int
			out, next = markdownLink(line, i, from, out)
			i = next
		default:
			i++
		}
	}
	return out
}

// isImage diz se o colchete em idx é precedido de `!`.
func isImage(line string, idx int) bool {
	return idx > 0 && line[idx-1] == '!'
}

// wikiLink interpreta o conteúdo de um wikilink, sem [[ e ]]. Rótulo após `|` é descartado.
func wikiLink(inner string) (storage.Link, bool) {
	if strings.ContainsAny(inner, "[]") {
		return storage.Link{}, false
	}
	if i := strings.IndexByte(inner, '|'); i >= 0 {
		inner = inner[:i]
	}
	t, err := parseTarget(strings.TrimSpace(inner))
	if err != nil {
		return storage.Link{}, false
	}
	return storage.Link{ToWorkspace: t.ws, ToProject: t.pj, ToPath: t.path}, true
}

// markdownLink trata o `](` que começa em i. Devolve a lista com o link acrescentado, se houver,
// e a posição de onde a varredura continua.
func markdownLink(line string, i int, from domain.PagePath, out []storage.Link) ([]storage.Link, int) {
	start := i + 2
	rest := line[start:]
	limit := min(len(rest), maxMarkdownDestBytes)
	closeIdx := strings.IndexByte(rest[:limit], ')')
	if closeIdx < 0 {
		return out, start
	}
	next := start + closeIdx + 1
	open := strings.LastIndexByte(line[:i], '[')
	if open < 0 || isImage(line, open) {
		return out, next
	}
	if l, ok := markdownDest(rest[:closeIdx], from); ok {
		out = append(out, l)
	}
	return out, next
}

// markdownDest interpreta o destino de um link markdown (texto entre `(` e `)`). Título entre
// aspas é descartado, pois só vale o primeiro campo.
func markdownDest(raw string, from domain.PagePath) (storage.Link, bool) {
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return storage.Link{}, false
	}
	dest := fields[0]
	if strings.Contains(dest, "://") || strings.ContainsRune(dest, '\\') {
		return storage.Link{}, false
	}
	lower := strings.ToLower(dest)
	for _, s := range ignoredSchemes {
		if strings.HasPrefix(lower, s) {
			return storage.Link{}, false
		}
	}
	if i := strings.IndexAny(dest, "?#"); i >= 0 {
		dest = dest[:i]
	}
	p, err := withMarkdownExt(dest)
	if err != nil {
		return storage.Link{}, false
	}
	if strings.HasPrefix(p, "/") {
		p = p[1:]
	} else {
		p = path.Join(path.Dir(from.String()), p)
	}
	pp, err := domain.ParsePagePath(p)
	if err != nil {
		return storage.Link{}, false
	}
	return storage.Link{ToPath: pp}, true
}

// scopedTarget é um alvo já separado: escopo opcional e caminho de página.
type scopedTarget struct {
	ws, pj *string
	path   domain.PagePath
}

// parseTarget interpreta `[workspace/][projeto:]caminho`, a gramática de wikilink e de relação.
// Consulta e âncora saem antes. Os componentes de escopo não podem ser vazios nem ter espaço nas
// bordas. Caminho sem extensão recebe `.md`. O resultado passa por ParsePagePath, que é estrito.
func parseTarget(raw string) (scopedTarget, error) {
	if strings.Contains(raw, "://") || strings.ContainsRune(raw, '\\') {
		return scopedTarget{}, errAlvoNaoPagina
	}
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		raw = raw[:i]
	}
	var t scopedTarget
	if idx := strings.IndexByte(raw, ':'); idx >= 0 {
		prefix := raw[:idx]
		pjName := prefix
		if w, p, found := strings.Cut(prefix, "/"); found {
			if !validScopeName(w) {
				return scopedTarget{}, errAlvoNaoPagina
			}
			t.ws = &w
			pjName = p
		}
		if !validScopeName(pjName) {
			return scopedTarget{}, errAlvoNaoPagina
		}
		// `[[_global:x]]` é a forma reservada do projeto de preferências, que fica no workspace
		// padrão e não no de origem, a menos que se escreva `workspace/_global` (wiki-files-layer).
		if t.ws == nil && pjName == globalProjectName {
			ws := defaultWorkspaceName
			t.ws = &ws
		}
		t.pj = &pjName
		raw = raw[idx+1:]
	}
	p, err := withMarkdownExt(raw)
	if err != nil {
		return scopedTarget{}, err
	}
	pp, err := domain.ParsePagePath(p)
	if err != nil {
		return scopedTarget{}, fmt.Errorf("%w: %w", errAlvoNaoPagina, err)
	}
	t.path = pp
	return t, nil
}

// validScopeName diz se um componente de escopo é utilizável: não vazio, sem espaço nas bordas,
// sem barra.
func validScopeName(s string) bool {
	return s != "" && strings.TrimSpace(s) == s && !strings.Contains(s, "/")
}

// withMarkdownExt aplica a regra de extensão de página: destino sem extensão recebe `.md`.
// Destino vazio, terminado em barra, com ponto que não termina em `.md` ou só `.md` é recusado.
func withMarkdownExt(p string) (string, error) {
	if p == "" || strings.HasSuffix(p, "/") {
		return "", errAlvoNaoPagina
	}
	base := p[strings.LastIndexByte(p, '/')+1:]
	if !strings.Contains(base, ".") {
		return p + ".md", nil
	}
	if !strings.HasSuffix(base, ".md") || base == ".md" {
		return "", errAlvoNaoPagina
	}
	return p, nil
}

// relationLinks lê a chave relations do frontmatter. Só string ou []string são representáveis
// pelo domínio; outro tipo é erro.
func relationLinks(fm domain.Frontmatter) ([]storage.Link, []string, error) {
	v, ok := fm.Get(relationsKey)
	if !ok {
		return nil, nil, nil
	}
	var items []string
	switch r := v.(type) {
	case string:
		items = []string{r}
	case []string:
		items = r
	default:
		return nil, nil, fmt.Errorf("%w: chave %s, tipo %T", ErrInvalidFrontmatter, relationsKey, v)
	}
	out := make([]storage.Link, 0, len(items))
	var warns []string
	for _, item := range items {
		rel, rawTarget, found := strings.Cut(item, ":")
		if !found {
			warns = append(warns, fmt.Sprintf("alvo de relação não nomeia página: %q", truncateBytes(item, maxWarnBytes)))
			continue
		}
		t, err := parseTarget(strings.TrimSpace(rawTarget))
		if err != nil {
			warns = append(warns, fmt.Sprintf("alvo de relação não nomeia página: %q", truncateBytes(item, maxWarnBytes)))
			continue
		}
		rel = strings.TrimSpace(rel)
		relation := rel
		if !isKnownRelation(rel) {
			warns = append(warns, fmt.Sprintf("relação desconhecida %q: gravada como references", truncateBytes(rel, maxWarnBytes)))
			relation = ""
		}
		out = append(out, storage.Link{ToWorkspace: t.ws, ToProject: t.pj, ToPath: t.path, Relation: relation})
	}
	return out, warns, nil
}

// isKnownRelation diz se a relação está no vocabulário fechado (ADR-19).
func isKnownRelation(rel string) bool {
	switch rel {
	case "causes", "fixes", "contradicts":
		return true
	}
	return false
}

// truncateBytes corta s em no máximo n bytes, sem separar um caractere UTF-8.
func truncateBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

// linkKey é a chave lógica de dedupe: (workspace, projeto, caminho, relação). Distingue nil de vazio.
type linkKey struct {
	ws, pj       string
	hasWS, hasPJ bool
	path, rel    string
}

func keyOf(l storage.Link) linkKey {
	k := linkKey{path: l.ToPath.String(), rel: l.Relation}
	if l.ToWorkspace != nil {
		k.ws, k.hasWS = *l.ToWorkspace, true
	}
	if l.ToProject != nil {
		k.pj, k.hasPJ = *l.ToProject, true
	}
	return k
}

// dedupeSortLinks remove duplicatas (a primeira ocorrência vence) e ordena: workspace (nil
// primeiro), projeto (nil primeiro), caminho e relação.
func dedupeSortLinks(links []storage.Link) []storage.Link {
	seen := make(map[linkKey]struct{}, len(links))
	out := make([]storage.Link, 0, len(links))
	for _, l := range links {
		k := keyOf(l)
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, l)
	}
	slices.SortFunc(out, compareLinks)
	return out
}

func compareLinks(a, b storage.Link) int {
	if c := compareOptional(a.ToWorkspace, b.ToWorkspace); c != 0 {
		return c
	}
	if c := compareOptional(a.ToProject, b.ToProject); c != 0 {
		return c
	}
	if c := strings.Compare(a.ToPath.String(), b.ToPath.String()); c != 0 {
		return c
	}
	return strings.Compare(a.Relation, b.Relation)
}

// compareOptional ordena ponteiro de texto com nil antes de qualquer valor.
func compareOptional(a, b *string) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return -1
	case b == nil:
		return 1
	}
	return strings.Compare(*a, *b)
}
