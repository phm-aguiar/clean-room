package storage

import (
	"math"
	"testing"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

const rankEps = 1e-9

func rankRef(t *testing.T, path string) domain.PageRef {
	t.Helper()
	p, err := domain.ParsePagePath(path)
	if err != nil {
		t.Fatalf("ParsePagePath(%q): %v", path, err)
	}
	return domain.NewPageRef(domain.NewScope(domain.WorkspaceID{1}, domain.ProjectID{2}), p)
}

func rankPath(t *testing.T, path string) domain.PagePath {
	t.Helper()
	p, err := domain.ParsePagePath(path)
	if err != nil {
		t.Fatalf("ParsePagePath(%q): %v", path, err)
	}
	return p
}

func approx(a, b float64) bool { return math.Abs(a-b) < rankEps }

func findRRF(t *testing.T, res []RRFResult, ref domain.PageRef) RRFResult {
	t.Helper()
	for _, r := range res {
		if r.Ref == ref {
			return r
		}
	}
	t.Fatalf("página %s ausente do resultado", ref)
	return RRFResult{}
}

func TestRRFConstantIs60(t *testing.T) {
	if RRFK != 60 {
		t.Fatalf("RRFK = %d, want 60", RRFK)
	}
}

func TestRRFSingleFlowPositionOne(t *testing.T) {
	a := rankRef(t, "concepts/a.md")
	res := FuseRRF([][]domain.PageRef{{a}})
	if len(res) != 1 {
		t.Fatalf("len = %d, want 1", len(res))
	}
	if !approx(res[0].Score, 1.0/61.0) {
		t.Errorf("score = %v, want 1/61", res[0].Score)
	}
	if !approx(res[0].Rank, -1.0/61.0) {
		t.Errorf("rank = %v, want -1/61", res[0].Rank)
	}
}

func TestRRFTwoFlowsSum(t *testing.T) {
	a := rankRef(t, "concepts/a.md")
	b := rankRef(t, "concepts/b.md")
	// a: pos 1 no fluxo 1, pos 2 no fluxo 2. b: pos 2 no fluxo 1, pos 1 no fluxo 2.
	res := FuseRRF([][]domain.PageRef{{a, b}, {b, a}})
	ra := findRRF(t, res, a)
	rb := findRRF(t, res, b)
	want := 1.0/61.0 + 1.0/62.0
	if !approx(ra.Score, want) || !approx(rb.Score, want) {
		t.Errorf("scores a=%v b=%v, want both %v", ra.Score, rb.Score, want)
	}
}

func TestRRFMissingFromFlowNoContribution(t *testing.T) {
	a := rankRef(t, "concepts/a.md")
	b := rankRef(t, "concepts/b.md")
	res := FuseRRF([][]domain.PageRef{{a}, {b}})
	ra := findRRF(t, res, a)
	if !approx(ra.Score, 1.0/61.0) {
		t.Errorf("a score = %v, want só 1/61 (sem contribuição do fluxo onde está ausente)", ra.Score)
	}
}

func TestRRFDuplicateInFlowCountsFirstPosition(t *testing.T) {
	a := rankRef(t, "concepts/a.md")
	b := rankRef(t, "concepts/b.md")
	res := FuseRRF([][]domain.PageRef{{a, b, a}})
	ra := findRRF(t, res, a)
	if !approx(ra.Score, 1.0/61.0) {
		t.Errorf("a score = %v, want 1/61 (primeira posição)", ra.Score)
	}
}

func TestRRFEmptyInput(t *testing.T) {
	if res := FuseRRF(nil); len(res) != 0 {
		t.Errorf("len = %d, want 0", len(res))
	}
}

func TestRRFTieBreakDeterministic(t *testing.T) {
	// Mesmo score para todas: a ordem deve ser por caminho asc, sempre igual.
	z := rankRef(t, "z/page.md")
	m := rankRef(t, "m/page.md")
	a := rankRef(t, "a/page.md")
	for i := 0; i < 20; i++ {
		res := FuseRRF([][]domain.PageRef{{z}, {m}, {a}})
		if len(res) != 3 {
			t.Fatalf("len = %d, want 3", len(res))
		}
		got := []domain.PageRef{res[0].Ref, res[1].Ref, res[2].Ref}
		want := []domain.PageRef{a, m, z}
		for j := range want {
			if got[j] != want[j] {
				t.Fatalf("iteração %d posição %d: got %s want %s", i, j, got[j], want[j])
			}
		}
	}
}

func TestRRFOrderByScoreDesc(t *testing.T) {
	a := rankRef(t, "concepts/a.md")
	b := rankRef(t, "concepts/b.md")
	res := FuseRRF([][]domain.PageRef{{b}, {b, a}})
	if res[0].Ref != b {
		t.Errorf("primeiro = %s, want b (maior score)", res[0].Ref)
	}
	if res[0].Rank >= res[1].Rank {
		t.Errorf("rank do primeiro (%v) deveria ser menor que o do segundo (%v)", res[0].Rank, res[1].Rank)
	}
}

func TestAuthorityBaseIsOne(t *testing.T) {
	in := AuthorityInput{Kind: "fact", Tier: "", Path: rankPath(t, "concepts/x.md")}
	if got := Authority(in); !approx(got, 1.0) {
		t.Errorf("base = %v, want 1.0", got)
	}
}

func TestAuthorityEachAdjustment(t *testing.T) {
	cases := []struct {
		name string
		in   AuthorityInput
		want float64
	}{
		{"rule", AuthorityInput{Kind: "rule", Path: rankPath(t, "x/a.md")}, 1.15},
		{"decision", AuthorityInput{Kind: domain.PageKindDecision, Path: rankPath(t, "x/a.md")}, 1.15},
		{"procedure", AuthorityInput{Kind: domain.PageKindProcedure, Path: rankPath(t, "x/a.md")}, 1.12},
		{"gotcha", AuthorityInput{Kind: "gotcha", Path: rankPath(t, "x/a.md")}, 1.12},
		{"preference", AuthorityInput{Kind: "preference", Path: rankPath(t, "x/a.md")}, 1.12},
		{"concept", AuthorityInput{Kind: "concept", Path: rankPath(t, "x/a.md")}, 1.07},
		{"slot", AuthorityInput{Kind: "slot", Path: rankPath(t, "x/a.md")}, 1.07},
		{"session", AuthorityInput{Kind: "session", Path: rankPath(t, "x/a.md")}, 0.85},
		{"tier semantic", AuthorityInput{Tier: domain.TierSemantic, Path: rankPath(t, "x/a.md")}, 1.10},
		{"tier procedural", AuthorityInput{Tier: domain.TierProcedural, Path: rankPath(t, "x/a.md")}, 1.10},
		{"tier episodic", AuthorityInput{Tier: domain.TierEpisodic, Path: rankPath(t, "x/a.md")}, 0.92},
		{"tier working", AuthorityInput{Tier: domain.TierWorking, Path: rankPath(t, "x/a.md")}, 0.97},
		{"pinned", AuthorityInput{Pinned: true, Path: rankPath(t, "x/a.md")}, 1.08},
		{"_lint", AuthorityInput{Path: rankPath(t, "_lint/a.md")}, 0.80},
		{"investigations", AuthorityInput{Path: rankPath(t, "investigations/a.md")}, 0.95},
		{"canonical", AuthorityInput{Path: rankPath(t, "x/a.md"), Tags: []string{"canonical"}}, 1.20},
		{"source-of-truth", AuthorityInput{Path: rankPath(t, "x/a.md"), Tags: []string{"source-of-truth"}}, 1.20},
		{"active", AuthorityInput{Path: rankPath(t, "x/a.md"), Tags: []string{"active"}}, 1.05},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Authority(c.in); !approx(got, c.want) {
				t.Errorf("Authority = %v, want %v", got, c.want)
			}
		})
	}
}

func TestAuthorityFloorAndCeiling(t *testing.T) {
	// Ceiling: rule(+0.15) + procedural(+0.10) + pinned(+0.08) + canonical(+0.20) + active(+0.05) = 1.58 -> 1.50.
	top := AuthorityInput{
		Kind:   "rule",
		Tier:   domain.TierProcedural,
		Pinned: true,
		Path:   rankPath(t, "x/a.md"),
		Tags:   []string{"canonical", "active"},
	}
	if got := Authority(top); !approx(got, AuthorityCeiling) {
		t.Errorf("teto: got %v, want 1.50", got)
	}
	// Soma mínima alcançável pela tabela: session(-0.15) + episodic(-0.08) + _lint(-0.20) = 0.57 (acima do piso).
	bottom := AuthorityInput{
		Kind: "session",
		Tier: domain.TierEpisodic,
		Path: rankPath(t, "_lint/a.md"),
		Tags: nil,
	}
	if got := Authority(bottom); !approx(got, 0.57) {
		t.Errorf("mínimo da tabela: got %v, want 0.57", got)
	}
	// O piso em si é exercido diretamente pelo clamp, pois a tabela não o alcança.
	if got := clampFloat(0.54, AuthorityFloor, AuthorityCeiling); !approx(got, AuthorityFloor) {
		t.Errorf("clamp piso: got %v, want 0.55", got)
	}
	if got := clampFloat(1.60, AuthorityFloor, AuthorityCeiling); !approx(got, AuthorityCeiling) {
		t.Errorf("clamp teto: got %v, want 1.50", got)
	}
	// Tier working also negative, path investigations too: 1.0 - 0.15 - 0.03 - 0.05 = 0.77 (sem piso).
	mid := AuthorityInput{Kind: "session", Tier: domain.TierWorking, Path: rankPath(t, "investigations/a.md")}
	if got := Authority(mid); !approx(got, 0.77) {
		t.Errorf("mid = %v, want 0.77", got)
	}
}

func TestAuthorityDoNotAnswerCapAppliedAfterPositives(t *testing.T) {
	in := AuthorityInput{
		Kind:   "rule",
		Tier:   domain.TierSemantic,
		Pinned: true,
		Path:   rankPath(t, "x/a.md"),
		Tags:   []string{"do-not-answer-from", "canonical"},
	}
	if got := Authority(in); !approx(got, 0.55) {
		t.Errorf("do-not-answer-from: got %v, want 0.55", got)
	}
	in.Tags = []string{"test-fixture"}
	if got := Authority(in); !approx(got, 0.55) {
		t.Errorf("test-fixture: got %v, want 0.55", got)
	}
}

func TestAuthoritySupersededCap(t *testing.T) {
	in := AuthorityInput{Kind: "rule", Tier: domain.TierSemantic, Pinned: true, Path: rankPath(t, "x/a.md"), Tags: []string{"superseded", "active"}}
	if got := Authority(in); !approx(got, 0.65) {
		t.Errorf("superseded: got %v, want 0.65", got)
	}
}

func TestAuthorityHistoricalCap(t *testing.T) {
	in := AuthorityInput{Kind: "rule", Tier: domain.TierSemantic, Pinned: true, Path: rankPath(t, "x/a.md"), Tags: []string{"historical"}}
	if got := Authority(in); !approx(got, 0.80) {
		t.Errorf("historical: got %v, want 0.80", got)
	}
}

func TestAuthorityCapBelowValueDoesNotRaise(t *testing.T) {
	// Fator já abaixo do teto de historical (sessão episódica = 0.77): o teto não o aumenta.
	in := AuthorityInput{Kind: "session", Tier: domain.TierWorking, Path: rankPath(t, "investigations/a.md"), Tags: []string{"historical"}}
	if got := Authority(in); !approx(got, 0.77) {
		t.Errorf("got %v, want 0.77", got)
	}
}

func TestNormalizeTag(t *testing.T) {
	cases := map[string]string{
		"Do_Not_Answer_From": "do-not-answer-from",
		"  Source of Truth ": "sourceoftruth",
		"SOURCE_OF_TRUTH":    "source-of-truth",
		"Active":             "active",
		"":                   "",
	}
	for in, want := range cases {
		if got := NormalizeTag(in); got != want {
			t.Errorf("NormalizeTag(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAuthorityTagNormalizationApplied(t *testing.T) {
	in := AuthorityInput{Path: rankPath(t, "x/a.md"), Tags: []string{"Do_Not_Answer_From"}, Kind: "rule", Tier: domain.TierSemantic, Pinned: true}
	if got := Authority(in); !approx(got, 0.55) {
		t.Errorf("tag normalizada não aplicou teto: got %v, want 0.55", got)
	}
}

func TestApplyAuthorityNegativeRankMultiplies(t *testing.T) {
	// rank -0.5, fator 1.2: multiplica -> -0.6 (melhor, mais negativo).
	if got := ApplyAuthority(-0.5, 1.2); !approx(got, -0.6) {
		t.Errorf("got %v, want -0.6", got)
	}
	// rank -0.5, fator 0.55: multiplica -> -0.275 (pior, menos negativo).
	if got := ApplyAuthority(-0.5, 0.55); !approx(got, -0.275) {
		t.Errorf("got %v, want -0.275", got)
	}
}

func TestApplyAuthorityPositiveRankDivides(t *testing.T) {
	// rank 0.5, fator 1.25: divide -> 0.4 (melhor, menor).
	if got := ApplyAuthority(0.5, 1.25); !approx(got, 0.4) {
		t.Errorf("got %v, want 0.4", got)
	}
	// rank 0.5, fator 0.5: divide -> 1.0 (pior, maior).
	if got := ApplyAuthority(0.5, 0.5); !approx(got, 1.0) {
		t.Errorf("got %v, want 1.0", got)
	}
}

func TestApplyAuthorityZeroRankStaysZero(t *testing.T) {
	if got := ApplyAuthority(0, 1.5); got != 0 {
		t.Errorf("got %v, want 0", got)
	}
}
