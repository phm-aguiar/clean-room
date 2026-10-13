package storage

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
)

// readerMaxOpenConns é o soft cap do pool de leitura. É um limite de conexões
// abertas, não de leituras aceitas: leitura extra espera uma conexão livre até
// o ctx da chamada cancelar (backpressure), sem erro próprio.
const readerMaxOpenConns = 4

// readerDSNPragmas abrem o pool de leitura. query_only=1 faz o próprio SQLite
// recusar qualquer escrita em toda conexão do pool (somente leitura de fato,
// independente da transação). busy_timeout vale também para leitura. WAL é
// persistente no arquivo e é verificado por OpenReader.
//
// Escolha: query_only (e não mode=ro) porque mantém o mesmo formato de caminho
// simples de Open e aplica o pragma em toda conexão via _pragma, sem depender
// de URI file:. Com mode=ro seria preciso montar file: URI e escapar o caminho.
const readerDSNPragmas = "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=query_only(1)"

// reader implementa Reader sobre um *sql.DB somente leitura. Não tem estado
// próprio: o pool é dono do chamador, que o fecha.
type reader struct {
	db *sql.DB
}

var _ Reader = (*reader)(nil)

// OpenReader abre o pool somente leitura do banco <dataDir>/db/memory.sqlite,
// separado do pool de escrita. Não cria o banco: ele precisa existir (ver Open).
// Valida, lendo de volta em uma conexão, que o banco está em WAL e que
// busy_timeout e query_only valem. O chamador deve fechar o *sql.DB devolvido.
func OpenReader(ctx context.Context, dataDir string) (*sql.DB, error) {
	path := filepath.Join(dataDir, dbDirName, dbFileName)
	exists, err := fileExists(path)
	if err != nil {
		return nil, fmt.Errorf("storage reader: stat database: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("storage reader: banco inexistente em %s: abra com Open antes", path)
	}

	db, err := sql.Open("sqlite", path+readerDSNPragmas)
	if err != nil {
		return nil, fmt.Errorf("storage reader: open sqlite: %w", err)
	}
	db.SetMaxOpenConns(readerMaxOpenConns)

	if err := verifyReaderPragmas(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// verifyReaderPragmas lê de volta, em uma conexão, os pragmas do pool de leitura.
func verifyReaderPragmas(ctx context.Context, db *sql.DB) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("storage reader: connection: %w", err)
	}
	defer func() { _ = conn.Close() }()

	var journal string
	if err := conn.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journal); err != nil {
		return fmt.Errorf("storage reader: read journal_mode: %w", err)
	}
	if journal != "wal" {
		return fmt.Errorf("storage reader: journal_mode is %q, want \"wal\"", journal)
	}

	var busy int
	if err := conn.QueryRowContext(ctx, `PRAGMA busy_timeout`).Scan(&busy); err != nil {
		return fmt.Errorf("storage reader: read busy_timeout: %w", err)
	}
	if busy != 5000 {
		return fmt.Errorf("storage reader: busy_timeout is %d, want 5000", busy)
	}

	var qo int
	if err := conn.QueryRowContext(ctx, `PRAGMA query_only`).Scan(&qo); err != nil {
		return fmt.Errorf("storage reader: read query_only: %w", err)
	}
	if qo != 1 {
		return fmt.Errorf("storage reader: query_only is %d, want 1", qo)
	}
	return nil
}

// NewReader devolve o leitor sobre db, que deve ser o pool de OpenReader (não o
// de escrita). O chamador é dono de db e o fecha.
func NewReader(db *sql.DB) Reader {
	return &reader{db: db}
}

// Read implementa Reader. Roda fn numa transação somente leitura (BEGIN
// read-only) e faz COMMIT se fn der nil, ROLLBACK caso contrário. Um pânico em
// fn é capturado aqui, vira erro e a transação é desfeita; não se propaga.
func (r *reader) Read(ctx context.Context, fn func(*sql.Tx) error) (err error) {
	var tx *sql.Tx
	defer func() {
		if p := recover(); p != nil {
			if tx != nil {
				_ = tx.Rollback()
			}
			err = fmt.Errorf("leitura: pânico: %v", p)
		}
	}()

	tx, err = r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("leitura: begin: %w", err)
	}
	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("leitura: rollback (após erro: %w): %v", err, rbErr)
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("leitura: commit: %w", err)
	}
	return nil
}
