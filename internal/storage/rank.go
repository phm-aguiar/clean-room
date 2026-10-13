package storage

import (
	"sort"
	"strings"
	"unicode"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// RRFK é a constante k da fusão Reciprocal Rank Fusion (SR §11.3 passo 3). Fixa em 60.
const RRFK = 60

// AuthorityFloor e AuthorityCeiling delimitam o fator de autoridade (SR §11.5): [0.55, 1.50].
const (
	AuthorityFloor   = 0.55
	AuthorityCeiling = 1.50
)

// RRFResult é a pontuação RRF de uma página e o rank derivado (rank = -score, menor é melhor).
type RRFResult struct {
	Ref   domain.PageRef
	Score float64
	Rank  float64
}

// AuthorityInput são os sinais de uma página usados para calcular o fator de autoridade.
// Kind e Tier são comparados pelo texto exato. Path é comparado pelo prefixo `_lint/` e
// `investigations/`. Tags são normalizadas antes da comparação. Pinned vem da coluna ou do frontmatter.
type AuthorityInput struct {
	Kind   domain.PageKind
	Tier   domain.Tier
	Path   domain.PagePath
	Tags   []string
	Pinned bool
}

// FuseRRF calcula score(d) = Σ 1/(RRFK + posição(d)) sobre os fluxos, com posição a partir de 1.
// Um fluxo é uma sequência ordenada de refs. Uma página ausente de um fluxo não recebe contribuição
// desse fluxo. Se a mesma página aparece mais de uma vez no mesmo fluxo, conta só a primeira posição.
// A saída é ordenada por score desc, depois caminho asc, depois PageRef.String() asc, para ser
// determinística. Não faz I/O e não altera os fluxos de entrada.
func FuseRRF(flows [][]domain.PageRef) []RRFResult {
	scores := make(map[domain.PageRef]float64)
	for _, flow := range flows {
		seen := make(map[domain.PageRef]struct{}, len(flow))
		for i, ref := range flow {
			if _, dup := seen[ref]; dup {
				continue
			}
			seen[ref] = struct{}{}
			scores[ref] += 1.0 / float64(RRFK+i+1)
		}
	}
	out := make([]RRFResult, 0, len(scores))
	for ref, score := range scores {
		out = append(out, RRFResult{Ref: ref, Score: score, Rank: -score})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return refLess(out[i].Ref, out[j].Ref)
	})
	return out
}

// refLess ordena por caminho asc e, em empate, pela forma canônica completa do PageRef.
func refLess(a, b domain.PageRef) bool {
	pa, pb := a.Path.String(), b.Path.String()
	if pa != pb {
		return pa < pb
	}
	return a.String() < b.String()
}

// NormalizeTag remove espaços (Unicode), converte para minúsculas e troca `_` por `-`.
// Exemplo: "Do_Not_Answer_From" vira "do-not-answer-from".
func NormalizeTag(tag string) string {
	stripped := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, tag)
	return strings.ReplaceAll(strings.ToLower(stripped), "_", "-")
}

// Authority calcula o fator de autoridade (SR §11.5). Parte de 1.0, soma os ajustes da tabela,
// limita a [AuthorityFloor, AuthorityCeiling] e depois aplica o menor teto entre as tags presentes
// (do-not-answer-from e test-fixture: 0.55; superseded: 0.65; historical: 0.80). Nenhum sinal exclui a
// página. Não faz I/O e não entra em panic.
func Authority(in AuthorityInput) float64 {
	f := 1.0

	switch string(in.Kind) {
	case "rule", "decision":
		f += 0.15
	case "procedure", "gotcha", "preference":
		f += 0.12
	case "concept", "slot":
		f += 0.07
	case "session":
		f -= 0.15
	}

	switch in.Tier {
	case domain.TierSemantic, domain.TierProcedural:
		f += 0.10
	case domain.TierEpisodic:
		f -= 0.08
	case domain.TierWorking:
		f -= 0.03
	}

	if in.Pinned {
		f += 0.08
	}

	path := in.Path.String()
	if strings.HasPrefix(path, "_lint/") {
		f -= 0.20
	}
	if strings.HasPrefix(path, "investigations/") {
		f -= 0.05
	}

	ceiling := AuthorityCeiling
	for _, raw := range in.Tags {
		switch NormalizeTag(raw) {
		case "canonical", "source-of-truth":
			f += 0.20
		case "active":
			f += 0.05
		case "do-not-answer-from", "test-fixture":
			ceiling = minFloat(ceiling, 0.55)
		case "superseded":
			ceiling = minFloat(ceiling, 0.65)
		case "historical":
			ceiling = minFloat(ceiling, 0.80)
		}
	}

	f = clampFloat(f, AuthorityFloor, AuthorityCeiling)
	return minFloat(f, ceiling)
}

// ApplyAuthority aplica o fator ao rank (SR §11.3 passo 6). Se rank ≤ 0, multiplica pelo fator.
// Se rank > 0, divide pelo fator. Fator maior sempre significa melhor posição. O fator deve ser > 0,
// o que vale para qualquer retorno de Authority.
func ApplyAuthority(rank, factor float64) float64 {
	if rank <= 0 {
		return rank * factor
	}
	return rank / factor
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
