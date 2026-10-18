package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

var spoolTestBase = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

// spoolTestNew abre um spool num diretório temporário com relógio que avança 1 ms por chamada,
// para que a ordem de chegada seja determinística.
func spoolTestNew(t *testing.T) *hookSpool {
	t.Helper()
	s, err := openHookSpool(t.TempDir())
	if err != nil {
		t.Fatalf("openHookSpool: %v", err)
	}
	cur := spoolTestBase
	s.now = func() time.Time {
		cur = cur.Add(time.Millisecond)
		return cur
	}
	return s
}

// spoolTestProcs troca a enumeração de processos (T041) por uma lista fixa.
func spoolTestProcs(t *testing.T, fn func() ([]procInfo, error)) {
	t.Helper()
	prev := listProcesses
	listProcesses = fn
	t.Cleanup(func() { listProcesses = prev })
}

func spoolTestProcList(procs ...procInfo) func() ([]procInfo, error) {
	return func() ([]procInfo, error) { return procs, nil }
}

func spoolTestEnqueue(t *testing.T, s *hookSpool, key string, body string) {
	t.Helper()
	if _, err := s.Enqueue("/hook?ingest_key="+key, key, spoolAuthNone, "", []byte(body)); err != nil {
		t.Fatalf("Enqueue(%s): %v", key, err)
	}
}

func spoolTestLockPath(s *hookSpool) string {
	return filepath.Join(s.dir, spoolLockName)
}

func TestSpoolItemGravadoComPermissao0600(t *testing.T) {
	s := spoolTestNew(t)
	spoolTestEnqueue(t, s, "key-perm", `{"event":"stop"}`)

	names, err := s.names()
	if err != nil || len(names) != 1 {
		t.Fatalf("names = %v, %v; quer 1 arquivo", names, err)
	}
	path := filepath.Join(s.dir, names[0])
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if runtime.GOOS == "windows" {
		// No Windows o modo POSIX não se aplica; garante ao menos que o item não é somente leitura.
		if info.Mode().Perm()&0o200 == 0 {
			t.Fatalf("item somente leitura no Windows: %v", info.Mode())
		}
		t.Log("permissão 0600 não verificável por modo POSIX no Windows; ACL herdada do perfil do usuário")
	} else {
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("permissão do item = %o; quer 0600", got)
		}
		dirInfo, err := os.Stat(s.dir)
		if err != nil {
			t.Fatalf("Stat dir: %v", err)
		}
		if got := dirInfo.Mode().Perm(); got != 0o700 {
			t.Fatalf("permissão do diretório = %o; quer 0700", got)
		}
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(raw) >= 3 && raw[0] == 0xEF && raw[1] == 0xBB && raw[2] == 0xBF {
		t.Fatal("item com BOM")
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("item não é JSON: %v", err)
	}
	if got["ingest_key"] != "key-perm" {
		t.Errorf("ingest_key = %v; quer key-perm", got["ingest_key"])
	}
	if got["attempts"] != float64(0) {
		t.Errorf("attempts = %v; quer 0", got["attempts"])
	}
	if _, ok := got["created_ms"]; !ok {
		t.Error("created_ms ausente")
	}
	if !strings.Contains(got["url"].(string), "ingest_key=key-perm") {
		t.Errorf("url sem a chave de ingestão: %v", got["url"])
	}
}

// spoolTestAssertPerm confere a permissão do item: 0600 em POSIX; no Windows o modo não se
// aplica, então só se confere que o item não é somente leitura.
func spoolTestAssertPerm(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if runtime.GOOS == "windows" {
		if info.Mode().Perm()&0o200 == 0 {
			t.Fatalf("item somente leitura no Windows: %v", info.Mode())
		}
		return
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("permissão do item = %o; quer 0600", got)
	}
}

func TestSpoolGravaTokenNoModoStatic(t *testing.T) {
	s := spoolTestNew(t)
	if _, err := s.Enqueue("/hook?ingest_key=st", "st", spoolAuthStatic, "tok-estatico", []byte(`{}`)); err != nil {
		t.Fatalf("Enqueue static: %v", err)
	}
	names, err := s.names()
	if err != nil || len(names) != 1 {
		t.Fatalf("names = %v, %v; quer 1 arquivo", names, err)
	}
	path := filepath.Join(s.dir, names[0])
	spoolTestAssertPerm(t, path)

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("item não é JSON: %v", err)
	}
	if got["auth_mode"] != "static" {
		t.Errorf("auth_mode = %v; quer static", got["auth_mode"])
	}
	if got["token"] != "tok-estatico" {
		t.Errorf("token = %v; quer o token em texto", got["token"])
	}

	batch, err := s.Batch()
	if err != nil || len(batch) != 1 {
		t.Fatalf("Batch = %d, %v; quer 1", len(batch), err)
	}
	if batch[0].AuthMode != spoolAuthStatic || batch[0].Token != "tok-estatico" {
		t.Errorf("leitura static = (%q, %q); quer (static, tok-estatico)", batch[0].AuthMode, batch[0].Token)
	}
}

func TestSpoolModoNoneNaoGravaToken(t *testing.T) {
	s := spoolTestNew(t)
	// Mesmo com um token passado, o modo none não o grava.
	if _, err := s.Enqueue("/hook?ingest_key=nn", "nn", spoolAuthNone, "nao-deve-sair", []byte(`{}`)); err != nil {
		t.Fatalf("Enqueue none: %v", err)
	}
	names, err := s.names()
	if err != nil || len(names) != 1 {
		t.Fatalf("names = %v, %v; quer 1 arquivo", names, err)
	}
	path := filepath.Join(s.dir, names[0])
	spoolTestAssertPerm(t, path)

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(raw), "nao-deve-sair") {
		t.Fatal("token gravado no modo none")
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("item não é JSON: %v", err)
	}
	if got["auth_mode"] != "none" {
		t.Errorf("auth_mode = %v; quer none", got["auth_mode"])
	}
	if _, ok := got["token"]; ok {
		t.Errorf("campo token presente no modo none: %v", got["token"])
	}

	batch, err := s.Batch()
	if err != nil || len(batch) != 1 {
		t.Fatalf("Batch = %d, %v; quer 1", len(batch), err)
	}
	if batch[0].AuthMode != spoolAuthNone || batch[0].Token != "" {
		t.Errorf("leitura none = (%q, %q); quer (none, vazio)", batch[0].AuthMode, batch[0].Token)
	}
}

func TestSpoolRecusaAuthModeInvalido(t *testing.T) {
	s := spoolTestNew(t)
	if _, err := s.Enqueue("/hook?ingest_key=x", "x", "oidc", "t", []byte(`{}`)); err == nil {
		t.Fatal("auth_mode oidc aceito; modo OIDC fica fora desta task")
	}
	names, _ := s.names()
	if len(names) != 0 {
		t.Fatalf("item gravado com auth_mode inválido: %v", names)
	}
}

func TestSpoolLimiteDeDezMilArquivos(t *testing.T) {
	s := spoolTestNew(t)
	// Pré-cria o teto com nomes em ordem crescente (o mais antigo tem ms = 1).
	content := []byte(`{"url":"/hook","body":{},"ingest_key":"k","attempts":0,"created_ms":1}`)
	for i := 1; i <= spoolMaxFiles; i++ {
		name := fmt.Sprintf("%013d-1-%016x.json", i, i)
		if err := os.WriteFile(filepath.Join(s.dir, name), content, 0o600); err != nil {
			t.Fatalf("pré-criar %d: %v", i, err)
		}
	}
	evicted, err := s.Enqueue("/hook?ingest_key=novo", "novo", spoolAuthNone, "", []byte(`{}`))
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if evicted != 1 {
		t.Fatalf("evicted = %d; quer 1", evicted)
	}
	names, err := s.names()
	if err != nil {
		t.Fatalf("names: %v", err)
	}
	if len(names) != spoolMaxFiles {
		t.Fatalf("arquivos = %d; quer %d", len(names), spoolMaxFiles)
	}
	if !strings.HasPrefix(names[0], "0000000000002-") {
		t.Fatalf("o mais antigo restante = %s; o de ms=1 devia ter saído", names[0])
	}
	if spoolMaxFiles != 10000 {
		t.Fatalf("spoolMaxFiles = %d; o doc fixa 10000", spoolMaxFiles)
	}
}

func TestSpoolLoteLimitaEm256Itens(t *testing.T) {
	s := spoolTestNew(t)
	for i := 0; i < 300; i++ {
		spoolTestEnqueue(t, s, fmt.Sprintf("k-%03d", i), `{}`)
	}
	batch, err := s.Batch()
	if err != nil {
		t.Fatalf("Batch: %v", err)
	}
	if len(batch) != 256 {
		t.Fatalf("lote = %d itens; quer 256", len(batch))
	}
	if spoolBatchMaxItems != 256 {
		t.Fatalf("spoolBatchMaxItems = %d; o doc fixa 256", spoolBatchMaxItems)
	}
	if batch[0].IngestKey != "k-000" {
		t.Errorf("primeiro do lote = %s; quer k-000 (ordem de chegada)", batch[0].IngestKey)
	}
	if batch[255].IngestKey != "k-255" {
		t.Errorf("último do lote = %s; quer k-255", batch[255].IngestKey)
	}
}

func TestSpoolLoteLimitaEm8MiB(t *testing.T) {
	bigBody := func(mib int) string {
		return `"` + strings.Repeat("a", mib<<20) + `"`
	}

	t.Run("tres de 3 MiB cabem dois", func(t *testing.T) {
		s := spoolTestNew(t)
		for i := 0; i < 3; i++ {
			spoolTestEnqueue(t, s, fmt.Sprintf("big-%d", i), bigBody(3))
		}
		batch, err := s.Batch()
		if err != nil {
			t.Fatalf("Batch: %v", err)
		}
		if len(batch) != 2 {
			t.Fatalf("lote = %d itens; quer 2 (6 MiB, o terceiro passaria de 8 MiB)", len(batch))
		}
	})

	t.Run("item sozinho acima do limite sai sozinho", func(t *testing.T) {
		s := spoolTestNew(t)
		spoolTestEnqueue(t, s, "huge", bigBody(9))
		batch, err := s.Batch()
		if err != nil {
			t.Fatalf("Batch: %v", err)
		}
		if len(batch) != 1 {
			t.Fatalf("lote = %d itens; quer 1 (pelo menos um evento por lote)", len(batch))
		}
	})
}

func TestSpoolExpiraAposSeteDias(t *testing.T) {
	s := spoolTestNew(t)
	s.now = func() time.Time { return spoolTestBase.Add(-spoolMaxAge - time.Minute) }
	spoolTestEnqueue(t, s, "velho", `{}`)
	s.now = func() time.Time { return spoolTestBase.Add(-6 * 24 * time.Hour) }
	spoolTestEnqueue(t, s, "recente", `{}`)

	s.now = func() time.Time { return spoolTestBase }
	batch, err := s.Batch()
	if err != nil {
		t.Fatalf("Batch: %v", err)
	}
	if len(batch) != 1 || batch[0].IngestKey != "recente" {
		t.Fatalf("lote = %+v; quer só o item de 6 dias", batch)
	}
	names, _ := s.names()
	if len(names) != 1 {
		t.Fatalf("arquivos após a poda = %d; quer 1 (o vencido sai)", len(names))
	}
}

func TestSpoolOitoTentativasTiramItem(t *testing.T) {
	s := spoolTestNew(t)
	spoolTestEnqueue(t, s, "tentativas", `{}`)
	batch, err := s.Batch()
	if err != nil || len(batch) != 1 {
		t.Fatalf("Batch = %d, %v; quer 1", len(batch), err)
	}
	id := batch[0].ID

	for n := 1; n < spoolMaxAttempts; n++ {
		if err := s.Fail([]string{id}); err != nil {
			t.Fatalf("Fail #%d: %v", n, err)
		}
		item, _, err := loadSpoolItem(filepath.Join(s.dir, id))
		if err != nil {
			t.Fatalf("item sumiu após %d falhas (limite é %d): %v", n, spoolMaxAttempts, err)
		}
		if item.Attempts != n {
			t.Fatalf("attempts = %d após %d falhas", item.Attempts, n)
		}
	}
	if err := s.Fail([]string{id}); err != nil {
		t.Fatalf("Fail #%d: %v", spoolMaxAttempts, err)
	}
	if _, _, err := loadSpoolItem(filepath.Join(s.dir, id)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("após %d tentativas o item devia sair; err = %v", spoolMaxAttempts, err)
	}
	if spoolMaxAttempts != 8 {
		t.Fatalf("spoolMaxAttempts = %d; o doc fixa 8", spoolMaxAttempts)
	}
}

func TestSpoolAckRemoveItensConfirmados(t *testing.T) {
	s := spoolTestNew(t)
	spoolTestEnqueue(t, s, "a", `{}`)
	spoolTestEnqueue(t, s, "b", `{}`)
	batch, _ := s.Batch()
	if err := s.Ack([]string{batch[0].ID}); err != nil {
		t.Fatalf("Ack: %v", err)
	}
	if err := s.Ack([]string{batch[0].ID}); err != nil {
		t.Fatalf("Ack de item já ausente não é erro: %v", err)
	}
	left, _ := s.Batch()
	if len(left) != 1 || left[0].IngestKey != "b" {
		t.Fatalf("restante = %+v; quer só b", left)
	}
}

func TestSpoolDescartaItemIlegivel(t *testing.T) {
	s := spoolTestNew(t)
	if err := os.WriteFile(filepath.Join(s.dir, "0000000000001-1-0000000000000001.json"), []byte("{não json"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spoolTestEnqueue(t, s, "ok", `{}`)
	batch, err := s.Batch()
	if err != nil {
		t.Fatalf("Batch: %v", err)
	}
	if len(batch) != 1 || batch[0].IngestKey != "ok" {
		t.Fatalf("lote = %+v; quer só o item válido", batch)
	}
}

func TestSpoolTravaRetomaPIDMorto(t *testing.T) {
	s := spoolTestNew(t)
	spoolTestProcs(t, spoolTestProcList()) // nenhum processo vivo: o PID 999999 está morto
	if err := os.WriteFile(spoolTestLockPath(s), []byte("999999"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	unlock, err := s.TryLock()
	if err != nil {
		t.Fatalf("TryLock com trava de PID morto: %v", err)
	}
	raw, _ := os.ReadFile(spoolTestLockPath(s))
	if string(raw) != fmt.Sprint(os.Getpid()) {
		t.Fatalf("PID na trava = %q; quer o do processo atual", raw)
	}
	if err := unlock(); err != nil {
		t.Fatalf("unlock: %v", err)
	}
	if _, err := os.Stat(spoolTestLockPath(s)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("trava não foi removida: %v", err)
	}
}

func TestSpoolTravaDetentorVivoRecusa(t *testing.T) {
	s := spoolTestNew(t)
	spoolTestProcs(t, spoolTestProcList(procInfo{pid: 4242, name: "ai-memory.exe"}))
	if err := os.WriteFile(spoolTestLockPath(s), []byte("4242"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := s.TryLock()
	if !errors.Is(err, ErrSpoolLocked) {
		t.Fatalf("err = %v; quer ErrSpoolLocked", err)
	}
	if _, statErr := os.Stat(spoolTestLockPath(s)); statErr != nil {
		t.Fatalf("trava de detentor vivo foi removida: %v", statErr)
	}
}

func TestSpoolTravaPIDReutilizadoPorOutroProcessoEhMorto(t *testing.T) {
	s := spoolTestNew(t)
	spoolTestProcs(t, spoolTestProcList(procInfo{pid: 4242, name: "explorer.exe"}))
	if err := os.WriteFile(spoolTestLockPath(s), []byte("4242"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	unlock, err := s.TryLock()
	if err != nil {
		t.Fatalf("TryLock com PID reutilizado: %v", err)
	}
	_ = unlock()
}

func TestSpoolTravaFalhaDeEnumeracaoFechaAPorta(t *testing.T) {
	s := spoolTestNew(t)
	spoolTestProcs(t, func() ([]procInfo, error) { return nil, errors.New("sem /proc") })
	if err := os.WriteFile(spoolTestLockPath(s), []byte("4242"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := s.TryLock()
	if err == nil || errors.Is(err, ErrSpoolLocked) {
		t.Fatalf("err = %v; quer erro de enumeração (sem retomar a trava)", err)
	}
	if _, statErr := os.Stat(spoolTestLockPath(s)); statErr != nil {
		t.Fatalf("trava foi retomada com a enumeração falhando: %v", statErr)
	}
}

func TestSpoolTravaConteudoIlegivelEhRetomada(t *testing.T) {
	s := spoolTestNew(t)
	spoolTestProcs(t, spoolTestProcList())
	if err := os.WriteFile(spoolTestLockPath(s), []byte("lixo"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	unlock, err := s.TryLock()
	if err != nil {
		t.Fatalf("TryLock com conteúdo ilegível: %v", err)
	}
	_ = unlock()
}

func TestSpoolTravaSegundaTomadaNoMesmoProcessoRecusa(t *testing.T) {
	s := spoolTestNew(t)
	unlock, err := s.TryLock()
	if err != nil {
		t.Fatalf("primeiro TryLock: %v", err)
	}
	if _, err := s.TryLock(); !errors.Is(err, ErrSpoolLocked) {
		t.Fatalf("segundo TryLock err = %v; quer ErrSpoolLocked", err)
	}
	if err := unlock(); err != nil {
		t.Fatalf("unlock: %v", err)
	}
	unlock2, err := s.TryLock()
	if err != nil {
		t.Fatalf("TryLock após liberar: %v", err)
	}
	_ = unlock2()
}
