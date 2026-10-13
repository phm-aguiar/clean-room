package storage

import (
	"context"
	"database/sql"
)

// Writer é o ator de escrita (INV-02): uma goroutine dona da única conexão de
// escrita, consumindo uma fila de capacidade 1024. Cada chamada a Do é uma
// transação atômica (BEGIN, closure, COMMIT ou ROLLBACK).
type Writer interface {
	// Do enfileira fn e bloqueia até que ela seja executada na transação e a
	// transação termine. Se fn devolver erro, a transação sofre ROLLBACK e o
	// mesmo erro é devolvido ao chamador.
	//
	// Se a fila estiver cheia, Do bloqueia até ctx cancelar e devolve o erro de
	// ctx (backpressure). Depois de aceito na fila, Do não abandona a espera:
	// o resultado sempre volta, então ctx não interrompe uma transação em curso.
	// Depois de Close, devolve ErrWriterClosed.
	Do(ctx context.Context, fn func(*sql.Tx) error) error

	// QueueLen devolve quantos comandos estão aguardando na fila agora.
	QueueLen() int

	// Close para de aceitar comandos, executa os que já estavam na fila no
	// momento do fechamento e libera a conexão de volta ao pool. Não fecha o
	// *sql.DB. É idempotente: chamadas seguintes devolvem o mesmo resultado.
	Close() error
}

// Reader é a leitura em pool somente leitura (implementado pela T023). Cada
// chamada a Read roda fn dentro de uma transação de leitura, para que várias
// consultas de uma mesma operação vejam um snapshot consistente. Leituras não
// passam pelo escritor e não bloqueiam nele.
type Reader interface {
	// Read executa fn com uma transação de leitura e devolve o erro de fn.
	Read(ctx context.Context, fn func(*sql.Tx) error) error
}
