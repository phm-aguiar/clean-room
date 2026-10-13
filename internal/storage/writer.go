package storage

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
)

// queueCap é a capacidade da fila de comandos do escritor (INV-02).
const queueCap = 1024

// command é uma transação pendente. reply tem capacidade 1 e é usado uma vez.
type command struct {
	fn    func(*sql.Tx) error
	reply chan error
}

// writer implementa Writer. Ele é dono da *sql.Conn de escrita, obtida de um
// *sql.DB por NewWriter: a conexão só é usada pela goroutine run. Close devolve
// a conexão ao pool; o *sql.DB continua a cargo de quem o abriu.
type writer struct {
	conn *sql.Conn
	cmds chan command

	// ctx é cancelado por Close e é o único sinal de encerramento da goroutine.
	ctx    context.Context
	cancel context.CancelFunc
	// done é fechado quando run termina.
	done chan struct{}

	closeOnce sync.Once
	closeErr  error
}

var _ Writer = (*writer)(nil)

// NewWriter obtém a conexão de escrita de db e inicia o ator. O chamador deve
// chamar Close para encerrar a goroutine e devolver a conexão.
func NewWriter(ctx context.Context, db *sql.DB) (Writer, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("storage writer: connection: %w", err)
	}

	wctx, cancel := context.WithCancel(context.Background())
	w := &writer{
		conn:   conn,
		cmds:   make(chan command, queueCap),
		ctx:    wctx,
		cancel: cancel,
		done:   make(chan struct{}),
	}
	go w.run()
	return w, nil
}

// run é a única goroutine que toca a conexão de escrita. Ao receber o
// cancelamento, executa só os comandos já presentes na fila (snapshot de len)
// e termina. Comandos enviados depois ficam sem resposta e quem espera em Do
// recebe ErrWriterClosed pelo canal done.
func (w *writer) run() {
	defer close(w.done)
	for {
		select {
		case cmd := <-w.cmds:
			w.exec(cmd)
		case <-w.ctx.Done():
			for n := len(w.cmds); n > 0; n-- {
				w.exec(<-w.cmds)
			}
			return
		}
	}
}

// exec roda o comando numa transação e responde ao chamador.
// Usa context.Background: a transação não deve ser abortada pelo cancelamento
// do ator, senão um comando já aceito perderia o resultado.
func (w *writer) exec(cmd command) {
	cmd.reply <- w.runTx(cmd.fn)
}

func (w *writer) runTx(fn func(*sql.Tx) error) error {
	tx, err := w.conn.BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("storage writer: begin: %w", err)
	}
	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("storage writer: rollback (after %v): %w", err, rbErr)
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("storage writer: commit: %w", err)
	}
	return nil
}

// Do implementa Writer.
func (w *writer) Do(ctx context.Context, fn func(*sql.Tx) error) error {
	if w.ctx.Err() != nil {
		return ErrWriterClosed
	}

	cmd := command{fn: fn, reply: make(chan error, 1)}
	select {
	case w.cmds <- cmd:
	case <-ctx.Done():
		return fmt.Errorf("storage writer: enqueue: %w", ctx.Err())
	case <-w.ctx.Done():
		return ErrWriterClosed
	}

	select {
	case err := <-cmd.reply:
		return err
	case <-w.done:
		// A goroutine terminou. Se o comando foi executado, a resposta já está
		// no canal (run responde antes de sair). Senão ele ficou sem executar.
		select {
		case err := <-cmd.reply:
			return err
		default:
			return ErrWriterClosed
		}
	}
}

// QueueLen implementa Writer.
func (w *writer) QueueLen() int {
	return len(w.cmds)
}

// Close implementa Writer.
func (w *writer) Close() error {
	w.closeOnce.Do(func() {
		w.cancel()
		<-w.done
		w.closeErr = w.conn.Close()
	})
	return w.closeErr
}
