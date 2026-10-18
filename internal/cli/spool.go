// Spool local do cliente de hook (docs/cli.md §5.3; docs/hook-scripts-and-container-deploy.md
// §2.5; docs/external-lifecycle.md §8; plan da 002, Q-09).
//
// Cada item é um arquivo JSON 0600 em <data_dir>/hook-spool/. A gravação é atômica
// (temporário + rename), então quem drena nunca lê um arquivo parcial. O nome do arquivo
// (13 dígitos de milissegundos, pid, 16 hex aleatórios, .json) põe a ordem de chegada na
// ordem alfabética. A chave de ingestão vai no item e na query da URL; reenvios reaproveitam
// a mesma chave, então o servidor descarta duplicatas.
//
// A trava exclusiva da fila é o arquivo .drain.lock, criado com O_EXCL e contendo o PID do
// detentor. Se o detentor estiver morto (ou o conteúdo não tiver um PID legível), a trava é
// retomada. A checagem de vida usa a mesma enumeração de processos da guarda INV-09
// (liveguard.go).
package cli

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	spoolDirName       = "hook-spool"
	spoolLockName      = ".drain.lock"
	spoolMaxAttempts   = 8
	spoolMaxAge        = 7 * 24 * time.Hour
	spoolMaxFiles      = 10000
	spoolBatchMaxItems = 256
	spoolBatchMaxBytes = 8 << 20
	spoolLockRetries   = 3
	spoolFileMode      = 0o600
	spoolAuthStatic    = "static"
	spoolAuthNone      = "none"
)

// ErrSpoolLocked indica que outro processo vivo segura a trava da fila.
var ErrSpoolLocked = errors.New("spool travado por outro processo vivo")

// errSpoolCorrupt marca um item cujo JSON não pode ser lido.
var errSpoolCorrupt = errors.New("item do spool ilegível")

// spoolItem é o conteúdo de um arquivo do spool. ID é o nome do arquivo e não vai ao disco.
// created_ms segue docs/hook-scripts-and-container-deploy.md §2.5 (o plan dizia created_at).
// auth_mode é "static" ou "none"; o token em texto só existe quando auth_mode é "static"
// (o doc diz que o token fica no arquivo, protegido só pelas permissões).
type spoolItem struct {
	ID        string          `json:"-"`
	URL       string          `json:"url"`
	Body      json.RawMessage `json:"body"`
	IngestKey string          `json:"ingest_key"`
	AuthMode  string          `json:"auth_mode"`
	Token     string          `json:"token,omitempty"`
	Attempts  int             `json:"attempts"`
	CreatedMS int64           `json:"created_ms"`
}

// hookSpool é a fila em disco de um data_dir. now é o relógio, trocável nos testes.
type hookSpool struct {
	dir string
	now func() time.Time
}

// openHookSpool cria <dataDir>/hook-spool (0700) se faltar e devolve a fila.
func openHookSpool(dataDir string) (*hookSpool, error) {
	dir := filepath.Join(dataDir, spoolDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("criar spool %s: %w", dir, err)
	}
	return &hookSpool{dir: dir, now: time.Now}, nil
}

// Enqueue grava um item novo com attempts 0 e devolve quantos itens mais antigos foram
// descartados para respeitar o teto de 10 000 arquivos. Body precisa ser JSON válido.
// authMode é "static" (grava o token em texto) ou "none" (o token é descartado). Quem chama
// emite o aviso de descarte.
func (s *hookSpool) Enqueue(url, ingestKey, authMode, token string, body []byte) (int, error) {
	if ingestKey == "" {
		return 0, errors.New("spool: chave de ingestão vazia")
	}
	if authMode != spoolAuthStatic && authMode != spoolAuthNone {
		return 0, fmt.Errorf("spool: auth_mode inválido %q", authMode)
	}
	if !json.Valid(body) {
		return 0, errors.New("spool: corpo não é JSON válido")
	}
	if authMode == spoolAuthNone {
		token = ""
	}
	now := s.now()
	data, err := json.Marshal(spoolItem{
		URL:       url,
		Body:      body,
		IngestKey: ingestKey,
		AuthMode:  authMode,
		Token:     token,
		CreatedMS: now.UnixMilli(),
	})
	if err != nil {
		return 0, fmt.Errorf("spool: serializar item: %w", err)
	}
	name, err := spoolFileName(now)
	if err != nil {
		return 0, err
	}
	if err := writeFileAtomic(filepath.Join(s.dir, name), data, spoolFileMode); err != nil {
		return 0, err
	}
	return s.enforceCap()
}

// Batch devolve até spoolBatchMaxItems itens, do mais antigo para o mais novo, com
// tamanho somado do arquivo de até spoolBatchMaxBytes. Sempre há pelo menos um item se a
// fila não estiver vazia, mesmo que ele sozinho passe do limite. Na passagem, remove os
// itens vencidos (mais de spoolMaxAge) e os ilegíveis.
func (s *hookSpool) Batch() ([]spoolItem, error) {
	names, err := s.names()
	if err != nil {
		return nil, err
	}
	now := s.now()
	var batch []spoolItem
	var total int64
	for _, name := range names {
		if len(batch) == spoolBatchMaxItems {
			break
		}
		path := filepath.Join(s.dir, name)
		item, size, err := loadSpoolItem(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if errors.Is(err, errSpoolCorrupt) {
			if err := removeIfExists(path); err != nil {
				return nil, err
			}
			continue
		}
		if err != nil {
			return nil, err
		}
		if now.Sub(time.UnixMilli(item.CreatedMS)) > spoolMaxAge {
			if err := removeIfExists(path); err != nil {
				return nil, err
			}
			continue
		}
		if len(batch) > 0 && total+size > spoolBatchMaxBytes {
			break
		}
		item.ID = name
		batch = append(batch, item)
		total += size
	}
	return batch, nil
}

// Ack apaga os itens confirmados pelo servidor. Item que já sumiu não é erro.
func (s *hookSpool) Ack(ids []string) error {
	for _, id := range ids {
		if err := removeIfExists(filepath.Join(s.dir, id)); err != nil {
			return err
		}
	}
	return nil
}

// Fail soma uma tentativa a cada item falho. Ao chegar a spoolMaxAttempts, o item sai da
// fila. Item que já sumiu é ignorado.
func (s *hookSpool) Fail(ids []string) error {
	for _, id := range ids {
		path := filepath.Join(s.dir, id)
		item, _, err := loadSpoolItem(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if errors.Is(err, errSpoolCorrupt) {
			if err := removeIfExists(path); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		item.Attempts++
		if item.Attempts >= spoolMaxAttempts {
			if err := removeIfExists(path); err != nil {
				return err
			}
			continue
		}
		data, err := json.Marshal(item)
		if err != nil {
			return fmt.Errorf("spool: serializar item: %w", err)
		}
		if err := writeFileAtomic(path, data, spoolFileMode); err != nil {
			return err
		}
	}
	return nil
}

// TryLock toma a trava exclusiva da fila e devolve a função que a libera. Se outro processo
// vivo a segura, devolve ErrSpoolLocked. Se o detentor morreu, ou o conteúdo não tem um PID
// legível, a trava é retomada. Se a enumeração de processos falha, devolve o erro e não
// retoma (falha fechada).
func (s *hookSpool) TryLock() (func() error, error) {
	path := filepath.Join(s.dir, spoolLockName)
	for range spoolLockRetries {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			return holdSpoolLock(f, path)
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, fmt.Errorf("criar trava do spool: %w", err)
		}
		pid, stale, err := spoolLockStale(path)
		if err != nil {
			return nil, err
		}
		if !stale {
			return nil, fmt.Errorf("%w: pid %d (%s)", ErrSpoolLocked, pid, path)
		}
		if err := removeIfExists(path); err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("trava do spool instável após %d tentativas: %w", spoolLockRetries, ErrSpoolLocked)
}

// names lista os arquivos .json do spool em ordem alfabética, que é a ordem de chegada.
func (s *hookSpool) names() ([]string, error) {
	ents, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("listar spool: %w", err)
	}
	var names []string
	for _, e := range ents {
		if e.Type().IsRegular() && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// enforceCap descarta os itens mais antigos até restar spoolMaxFiles arquivos.
func (s *hookSpool) enforceCap() (int, error) {
	names, err := s.names()
	if err != nil {
		return 0, err
	}
	evicted := 0
	for i := 0; i < len(names)-spoolMaxFiles; i++ {
		if err := removeIfExists(filepath.Join(s.dir, names[i])); err != nil {
			return evicted, err
		}
		evicted++
	}
	return evicted, nil
}

// loadSpoolItem lê um item. Devolve o tamanho do arquivo, que é o que conta para o limite
// de lote. JSON inválido vira errSpoolCorrupt.
func loadSpoolItem(path string) (spoolItem, int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return spoolItem{}, 0, err
		}
		return spoolItem{}, 0, fmt.Errorf("ler item do spool: %w", err)
	}
	var item spoolItem
	if err := json.Unmarshal(data, &item); err != nil {
		return spoolItem{}, 0, fmt.Errorf("%w: %s: %v", errSpoolCorrupt, filepath.Base(path), err)
	}
	return item, int64(len(data)), nil
}

// spoolFileName monta o nome: milissegundos com 13 dígitos, pid, 16 hex aleatórios e .json.
func spoolFileName(now time.Time) (string, error) {
	var seq [8]byte
	if _, err := rand.Read(seq[:]); err != nil {
		return "", fmt.Errorf("sequência do spool: %w", err)
	}
	return fmt.Sprintf("%013d-%d-%s.json", now.UnixMilli(), os.Getpid(), hex.EncodeToString(seq[:])), nil
}

// removeIfExists remove o arquivo. Ausente não é erro.
func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remover %s: %w", filepath.Base(path), err)
	}
	return nil
}

// holdSpoolLock escreve o PID no arquivo já criado com O_EXCL e devolve a função de liberação.
// Liberar só remove a trava se ela ainda tiver o PID deste processo (se outro processo a
// retomou, não mexe).
func holdSpoolLock(f *os.File, path string) (func() error, error) {
	pid := strconv.Itoa(os.Getpid())
	_, werr := f.WriteString(pid)
	serr := f.Sync()
	cerr := f.Close()
	if err := errors.Join(werr, serr, cerr); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("gravar PID na trava do spool: %w", err)
	}
	return func() error {
		raw, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("ler trava do spool: %w", err)
		}
		if strings.TrimSpace(string(raw)) != pid {
			return nil
		}
		return removeIfExists(path)
	}, nil
}

// spoolLockStale lê a trava existente. Devolve o PID do detentor e se a trava pode ser
// retomada. Trava que sumiu, sem PID legível ou de processo morto pode ser retomada.
// Processo ativo não. Erro quando não é possível decidir.
//
// Decisão nossa: conteúdo sem PID legível é tratado como abandonado. O drenador é
// idempotente pela chave de ingestão, então uma retomada indevida só causa reenvio.
func spoolLockStale(path string) (int, bool, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, true, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("ler trava do spool: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || pid <= 0 {
		return 0, true, nil
	}
	alive, err := spoolHolderAlive(pid)
	if err != nil {
		return pid, false, err
	}
	return pid, !alive, nil
}

// spoolHolderAlive diz se o PID é deste processo ou de um processo ai-memory vivo. Um PID
// vivo com outro nome (reutilizado pelo sistema) conta como morto. A enumeração vem de
// liveguard.go (T041).
func spoolHolderAlive(pid int) (bool, error) {
	if pid == os.Getpid() {
		return true, nil
	}
	procs, err := listProcesses()
	if err != nil {
		return false, fmt.Errorf("enumerar processos para a trava do spool: %w", err)
	}
	for _, p := range procs {
		if p.pid == pid && isAIMemoryName(p.name) {
			return true, nil
		}
	}
	return false, nil
}
