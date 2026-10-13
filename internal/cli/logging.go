// Package cli compõe os componentes do processo. Este arquivo cuida do logging:
// log/slog com handler JSON (uma linha por registro), nível vindo de config e
// filtro de realimentação (INV-15).
//
// Escolha do handler: slog.NewJSONHandler. Motivos: uma linha por registro,
// escape determinístico (caminhos Windows com "\" e valores com espaço ficam
// inequívocos) e leitura direta por ferramentas. Não há cor nem formatação
// humana; quem quiser ler no terminal usa um visualizador.
//
// Filtro INV-15: o handler de produção é um selfFilter que descarta registros
// marcados com o atributo interno originKey=originSelf. A marca pode vir no
// próprio registro (logger.Info("x", originKey, originSelf)) ou em um logger
// derivado com With (logger.With(originKey, originSelf)). Registros marcados
// nunca chegam ao destino, então um registro emitido pelo próprio módulo de log
// não volta a ser processado pelo mesmo handler. Registros sem a marca passam
// normalmente, inclusive os de LogStartup.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/phm-aguiar/ai-memory-go/internal/config"
)

// ErrLogLevel indica nível de log fora de debug|info|warn|error.
var ErrLogLevel = errors.New("nível de log inválido")

// originKey e originSelf formam a marca interna do filtro INV-15.
const (
	originKey  = "ai_memory_origin"
	originSelf = "logging"
)

// NewLogger cria um slog.Logger com handler JSON gravando em out, nível vindo de
// cfg.LogLevel(). Retorna erro (que envolve ErrLogLevel) se o nível não for
// debug, info, warn ou error. Não toca em os.Stderr: out é injetado pelo chamador.
func NewLogger(cfg config.Config, out io.Writer) (*slog.Logger, error) {
	if out == nil {
		return nil, errors.New("cli: log: destino nil")
	}
	level, err := parseLevel(cfg.LogLevel())
	if err != nil {
		return nil, err
	}
	inner := slog.NewJSONHandler(out, &slog.HandlerOptions{Level: level})
	return slog.New(&selfFilter{next: inner}), nil
}

// LogStartup registra na inicialização o diretório de dados canônico (INV-11) e
// o nível de log. Não registra ambiente nem segredos. Logger nil é ignorado.
func LogStartup(logger *slog.Logger, cfg config.Config) {
	if logger == nil {
		return
	}
	logger.Info("inicialização", "data_dir", cfg.DataDir(), "log_level", cfg.LogLevel())
}

func parseLevel(s string) (slog.Level, error) {
	switch s {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return 0, fmt.Errorf("cli: log: %w: %q (use debug|info|warn|error)", ErrLogLevel, s)
}

// selfFilter é o filtro INV-15. Envolve o handler real e descarta registros que
// carregam a marca interna. self fica true quando a marca entrou por WithAttrs,
// porque nesse caso os atributos não aparecem no slog.Record.
type selfFilter struct {
	next slog.Handler
	self bool
}

func (h *selfFilter) Enabled(ctx context.Context, l slog.Level) bool {
	return !h.self && h.next.Enabled(ctx, l)
}

func (h *selfFilter) Handle(ctx context.Context, r slog.Record) error {
	if h.self {
		return nil
	}
	internal := false
	r.Attrs(func(a slog.Attr) bool {
		if isOrigin(a) {
			internal = true
			return false
		}
		return true
	})
	if internal {
		return nil
	}
	return h.next.Handle(ctx, r)
}

func (h *selfFilter) WithAttrs(attrs []slog.Attr) slog.Handler {
	self := h.self
	for _, a := range attrs {
		if isOrigin(a) {
			self = true
		}
	}
	return &selfFilter{next: h.next.WithAttrs(attrs), self: self}
}

func (h *selfFilter) WithGroup(name string) slog.Handler {
	return &selfFilter{next: h.next.WithGroup(name), self: h.self}
}

func isOrigin(a slog.Attr) bool {
	if a.Key != originKey {
		return false
	}
	v := a.Value.Resolve()
	return v.Kind() == slog.KindString && v.String() == originSelf
}
