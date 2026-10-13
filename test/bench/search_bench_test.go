// Package bench mede a latência da busca (storage.Search) com um corpus sintético de 5 mil
// páginas. É a evidência do alvo de performance da SPEC 001 (consulta com 5 mil páginas
// abaixo de 100 ms) e do baseline registrado em specs/features/001-nucleo/notes/bench-baseline.md.
//
// O corpus é determinístico (math/rand/v2 com semente fixa), tem vocabulário pequeno e um
// conjunto fixo de entidades. Tudo vive em t.TempDir(). Não há goroutine no benchmark.
package bench

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand/v2"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
	"github.com/phm-aguiar/ai-memory-go/internal/storage"
)

const (
	// corpusTamanho é o número de páginas do cenário de referência (SPEC: 5 mil páginas).
	corpusTamanho = 5000
	// loteTamanho é quantas páginas entram em cada transação do escritor durante a carga.
	loteTamanho = 500
	// limiteResultados é o limite da consulta medida (limit=10).
	limiteResultados = 10
	// alvoLatencia é a meta da SPEC: consulta com 5 mil páginas < 100 ms.
	alvoLatencia = 100 * time.Millisecond
	// execucoesAlvo é quantas medições o teste de alvo faz depois do aquecimento. A média de
	// 30 execuções reduz o ruído de uma única medição sem deixar o teste lento.
	execucoesAlvo = 30
)

// corpusBase é o instante de escrita de todas as páginas do corpus. O corte da busca fica
// uma hora depois, para que nenhuma versão esteja expirada.
var corpusBase = time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)

// vocabulario é o conjunto pequeno de termos que compõe o corpo e os títulos.
var vocabulario = []string{
	"postgres", "replicação", "indexação", "busca", "sessão", "decisão", "gotcha", "timeout",
	"cache", "fila", "escritor", "leitor", "snapshot", "migração", "backup", "token", "rotas",
	"deploy", "sqlite", "wal", "latência", "memória", "entidade", "grafo", "vínculo", "regra",
	"procedimento", "preferência", "conceito", "erro", "log", "config", "sincronização", "pool",
	"conexão", "transação", "índice", "consulta", "ranking", "autoridade", "pinagem", "tombstone",
}

// entidadesCorpus são as entidades usadas no frontmatter. Cada página recebe duas delas.
var entidadesCorpus = []string{
	"postgres", "sqlite", "busca", "cache", "fila", "backup", "deploy", "wal",
	"escritor", "leitor", "token", "migracao",
}

// tiers rodam em rodízio para que o fator de autoridade e o tier variem entre as páginas.
var tiers = []domain.Tier{domain.TierSemantic, domain.TierEpisodic, domain.TierWorking, domain.TierProcedural}

// consultas são as consultas medidas em rodízio pelo benchmark. Todas têm termos do
// vocabulário, então casam páginas nos três fluxos (FTS, entidades e grafo).
var consultas = []string{
	"postgres replicação",
	"timeout cache",
	"sessão decisão",
	"gotcha fila escritor",
}

// ambiente abre o banco temporário, o escritor, o leitor e cria o escopo do benchmark.
func ambiente(t testing.TB) (storage.Writer, storage.Reader, domain.Scope) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()

	db, err := storage.Open(ctx, dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	w, err := storage.NewWriter(ctx, db)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })

	rdb, err := storage.OpenReader(ctx, dir)
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	t.Cleanup(func() { _ = rdb.Close() })

	wsID, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID ws: %v", err)
	}
	pjID, err := domain.NewID()
	if err != nil {
		t.Fatalf("NewID pj: %v", err)
	}
	scope := domain.NewScope(domain.WorkspaceID(wsID), domain.ProjectID(pjID))
	if err := storage.EnsureScope(ctx, w, scope, "bench-ws", "bench-pj", corpusBase); err != nil {
		t.Fatalf("EnsureScope: %v", err)
	}
	return w, storage.NewReader(rdb), scope
}

// paginaSintetica monta a i-ésima página do corpus. O rng vem com semente fixa, então o corpus
// é o mesmo em todas as execuções.
func paginaSintetica(t testing.TB, rng *rand.Rand, scope domain.Scope, i int) storage.UpsertInput {
	t.Helper()
	palavras := make([]string, 24)
	for k := range palavras {
		palavras[k] = vocabulario[rng.IntN(len(vocabulario))]
	}
	e1 := entidadesCorpus[rng.IntN(len(entidadesCorpus))]
	e2 := entidadesCorpus[rng.IntN(len(entidadesCorpus))]

	var fm domain.Frontmatter
	if err := fm.Set("entities", []string{e1, e2}); err != nil {
		t.Fatalf("frontmatter entities: %v", err)
	}

	path, err := domain.ParsePagePath(fmt.Sprintf("notas/grupo-%02d/pagina-%05d.md", i%50, i))
	if err != nil {
		t.Fatalf("ParsePagePath(%d): %v", i, err)
	}
	return storage.UpsertInput{
		Ref:         domain.NewPageRef(scope, path),
		Tier:        tiers[i%len(tiers)],
		Kind:        domain.PageKindNote,
		Title:       fmt.Sprintf("Página %d sobre %s", i, palavras[0]),
		Body:        strings.Join(palavras, " "),
		Frontmatter: fm,
		Pinned:      i%97 == 0,
		Evidence:    storage.EvidenceSource{Kind: storage.SourceSession, ID: "bench-session"},
		At:          corpusBase,
	}
}

// carregarCorpus grava corpusTamanho páginas no escopo. Cada lote roda em um único w.Do, com
// UpsertPageTx dentro da transação, para que a carga passe pelo escritor único (INV-02) sem
// uma transação por página. Devolve o corte da busca.
func carregarCorpus(t testing.TB, w storage.Writer, scope domain.Scope) time.Time {
	t.Helper()
	ctx := context.Background()
	rng := rand.New(rand.NewPCG(42, 7))

	for inicio := 0; inicio < corpusTamanho; inicio += loteTamanho {
		fim := inicio + loteTamanho
		if fim > corpusTamanho {
			fim = corpusTamanho
		}
		entradas := make([]storage.UpsertInput, 0, fim-inicio)
		for i := inicio; i < fim; i++ {
			entradas = append(entradas, paginaSintetica(t, rng, scope, i))
		}
		err := w.Do(ctx, func(tx *sql.Tx) error {
			for _, in := range entradas {
				res, err := storage.UpsertPageTx(tx, in)
				if err != nil {
					return err
				}
				if res.Outcome != storage.UpsertCreated {
					return fmt.Errorf("página %s: outcome %q, esperado created", in.Ref.Path, res.Outcome)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("carga do lote %d-%d: %v", inicio, fim, err)
		}
	}
	return corpusBase.Add(time.Hour)
}

// medirConsulta roda uma busca e devolve a duração medida em torno de storage.Search.
func medirConsulta(t testing.TB, r storage.Reader, scope domain.Scope, consulta string, corte time.Time) (time.Duration, storage.SearchResult) {
	t.Helper()
	inicio := time.Now()
	res, err := storage.Search(context.Background(), r, scope, consulta, corte, limiteResultados)
	dur := time.Since(inicio)
	if err != nil {
		t.Fatalf("Search(%q): %v", consulta, err)
	}
	return dur, res
}

// percentil devolve o percentil p (0..1) pelo método do posto mais próximo sobre durações já
// ordenadas. Lista vazia devolve zero.
func percentil(ordenadas []time.Duration, p float64) time.Duration {
	if len(ordenadas) == 0 {
		return 0
	}
	idx := int(float64(len(ordenadas))*p+0.999999) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(ordenadas) {
		idx = len(ordenadas) - 1
	}
	return ordenadas[idx]
}

// resumo ordena uma cópia das durações e devolve p50, p95 e média.
func resumo(durs []time.Duration) (p50, p95, media time.Duration) {
	if len(durs) == 0 {
		return 0, 0, 0
	}
	ordenadas := append([]time.Duration(nil), durs...)
	sort.Slice(ordenadas, func(i, j int) bool { return ordenadas[i] < ordenadas[j] })
	var soma time.Duration
	for _, d := range durs {
		soma += d
	}
	return percentil(ordenadas, 0.50), percentil(ordenadas, 0.95), soma / time.Duration(len(durs))
}

// BenchmarkSearch5k mede storage.Search com 5 mil páginas. A carga fica fora do timer. Cada
// iteração roda uma consulta do rodízio; as durações são guardadas por consulta, e o resumo
// (p50, p95) sai em log e em métricas do benchmark. Com -benchtime=Nx, o número de amostras
// é pequeno e os percentis são indicativos.
func BenchmarkSearch5k(b *testing.B) {
	b.StopTimer()
	w, r, scope := ambiente(b)
	corte := carregarCorpus(b, w, scope)
	// Aquecimento fora do timer: primeira leitura abre páginas e prepara o cache do SQLite.
	for _, q := range consultas {
		medirConsulta(b, r, scope, q, corte)
	}
	b.StartTimer()

	porConsulta := make([][]time.Duration, len(consultas))
	todas := make([]time.Duration, 0, b.N)
	for n := 0; n < b.N; n++ {
		qi := n % len(consultas)
		dur, res := medirConsulta(b, r, scope, consultas[qi], corte)
		if len(res.Pages) == 0 {
			b.Fatalf("consulta %q sem páginas: o corpus não está casando", consultas[qi])
		}
		porConsulta[qi] = append(porConsulta[qi], dur)
		todas = append(todas, dur)
	}
	b.StopTimer()

	for qi, q := range consultas {
		p50, p95, media := resumo(porConsulta[qi])
		b.Logf("consulta %q: amostras=%d p50=%v p95=%v média=%v", q, len(porConsulta[qi]), p50, p95, media)
	}
	p50, p95, media := resumo(todas)
	b.Logf("todas as consultas: amostras=%d p50=%v p95=%v média=%v (alvo < %v)", len(todas), p50, p95, media, alvoLatencia)
	b.ReportMetric(float64(p50)/float64(time.Millisecond), "p50-ms")
	b.ReportMetric(float64(p95)/float64(time.Millisecond), "p95-ms")
	b.ReportMetric(float64(media)/float64(time.Millisecond), "media-ms")
}

// TestSearch5kAbaixoDe100ms checa a meta da SPEC com uma consulta sobre 5 mil páginas. Faz
// um aquecimento e depois execucoesAlvo medições. Falha se a média passar de alvoLatencia.
// Pula em -short, porque a carga leva alguns segundos. Pula também sob -race: o detector
// torna a execução várias vezes mais lenta, e o alvo de tempo só vale sem ele.
//
// Decisão sobre ruído: o critério é a média de 30 execuções (e não uma medição isolada), e
// o p50/p95 vão para o log, para que uma máquina lenta apareça como número e não só como
// falha. A falha continua valendo para a média, porque é a meta da SPEC.
func TestSearch5kAbaixoDe100ms(t *testing.T) {
	if testing.Short() {
		t.Skip("pulado em -short: a carga de 5 mil páginas leva alguns segundos")
	}
	if detectorRaceAtivo {
		t.Skip("pulado sob -race: o detector invalida o alvo de tempo (ver comentário do teste)")
	}
	w, r, scope := ambiente(t)
	corte := carregarCorpus(t, w, scope)
	consulta := consultas[0]

	medirConsulta(t, r, scope, consulta, corte) // aquecimento
	durs := make([]time.Duration, 0, execucoesAlvo)
	for i := 0; i < execucoesAlvo; i++ {
		dur, res := medirConsulta(t, r, scope, consulta, corte)
		if len(res.Pages) == 0 {
			t.Fatalf("consulta %q sem páginas", consulta)
		}
		durs = append(durs, dur)
	}

	p50, p95, media := resumo(durs)
	t.Logf("máquina: GOOS=%s GOARCH=%s CPUs=%d go=%s", runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), runtime.Version())
	t.Logf("consulta %q, %d páginas, %d execuções: p50=%v p95=%v média=%v (alvo < %v)",
		consulta, corpusTamanho, execucoesAlvo, p50, p95, media, alvoLatencia)
	if media > alvoLatencia {
		t.Errorf("média %v acima do alvo %v (p50 %v, p95 %v)", media, alvoLatencia, p50, p95)
	}
}
