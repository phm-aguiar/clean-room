package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// Semente de startup do ponteiro de projeto ativo (docs/auto-scope.md, "Surviving a restart"; T092).
//
// O ponteiro vive em memória: reiniciar o servidor o esvazia, inclusive para sessões ainda abertas.
// No boot do transporte HTTP, esta semente reconstrói o projeto da observação mais recente dentro de
// session_ttl_secs e a guarda no slot de startup do ponteiro. Limites do doc:
//   - só leitura: a semente nunca é gravada no slot compartilhado nem nas entradas chaveadas, e
//     Lookup (escrita) nunca a usa (ver ActiveProjectPointer.Read);
//   - aplicada só com o slot compartilhado vazio: se um evento de primeiro plano já publicou, não entra;
//   - descartada pelo primeiro evento de primeiro plano, porque Read passa a responder pelo slot
//     compartilhado antes de chegar à semente;
//   - invalidações administrativas chamam ClearStartupSeed;
//   - falha ou ausência de observação só é registrada; a subida do servidor não para.
//
// Janela: a observação vale se created_at > agora - TTL, o mesmo critério de expiração das entradas
// chaveadas (activeproject.go, sweepLocked). O TTL é o do próprio ponteiro, já normalizado.

// startupSeedSQL pega a observação mais recente com created_at dentro da janela. created_at é
// microssegundos Unix (observations.go). workspace_id e project_id vêm da própria linha, que o
// trigger de pareamento garante consistentes.
const startupSeedSQL = `SELECT workspace_id, project_id FROM observations
 WHERE created_at > ? ORDER BY created_at DESC LIMIT 1`

// SeedStartupFromRecent aplica a semente de startup e devolve true quando a semente foi gravada no
// ponteiro. now zero usa o relógio atual. Nenhuma falha é devolvida: ausência de observação, slot
// compartilhado já preenchido e erro de leitura são registrados em logger (nil usa slog.Default) e
// o resultado é false.
func SeedStartupFromRecent(ctx context.Context, r Reader, p *ActiveProjectPointer, now time.Time, logger *slog.Logger) bool {
	if logger == nil {
		logger = slog.Default()
	}
	if r == nil || p == nil {
		logger.Warn("startup seed: leitor ou ponteiro ausente; sem semente")
		return false
	}
	if now.IsZero() {
		now = time.Now()
	}
	ttlSecs := int64(p.ttl / time.Second)

	scope, found, err := readStartupScope(ctx, r, now.Add(-p.ttl).UnixMicro())
	if err != nil {
		logger.Warn("startup seed: falha ao ler a observação mais recente; sem semente", "error", err)
		return false
	}
	if !found {
		logger.Info("startup seed: nenhuma observação dentro do TTL; sem semente", "session_ttl_secs", ttlSecs)
		return false
	}

	// Verificação e gravação sob o mesmo mutex: um evento de primeiro plano que chegue entre a
	// leitura e a gravação não pode ser sobrescrito pela semente.
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.shared != nil {
		logger.Info("startup seed: slot compartilhado já preenchido; semente não aplicada")
		return false
	}
	p.seed = &pointerValue{scope: scope}
	logger.Info("startup seed: projeto da observação mais recente aplicado só para leitura",
		"workspace_id", scope.WorkspaceID.String(),
		"project_id", scope.ProjectID.String(),
		"session_ttl_secs", ttlSecs,
	)
	return true
}

// readStartupScope lê o escopo da observação mais recente com created_at > cutoff. found é falso
// quando não há nenhuma. Só leitura, por Reader.
func readStartupScope(ctx context.Context, r Reader, cutoff int64) (domain.Scope, bool, error) {
	var scope domain.Scope
	found := false
	err := r.Read(ctx, func(tx *sql.Tx) error {
		var wsB, pjB []byte
		err := tx.QueryRowContext(ctx, startupSeedSQL, cutoff).Scan(&wsB, &pjB)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("consultar observação mais recente: %w", err)
		}
		wsID, err := domain.ParseIDBytes(wsB)
		if err != nil {
			return fmt.Errorf("id do workspace: %w", err)
		}
		pjID, err := domain.ParseIDBytes(pjB)
		if err != nil {
			return fmt.Errorf("id do projeto: %w", err)
		}
		scope = domain.NewScope(domain.WorkspaceID(wsID), domain.ProjectID(pjID))
		found = true
		return nil
	})
	if err != nil {
		return domain.Scope{}, false, fmt.Errorf("semente de startup: %w", err)
	}
	return scope, found, nil
}
