package storage

import "errors"

var (
	// ErrWriterClosed is returned when a command is sent to a closed writer actor.
	ErrWriterClosed = errors.New("writer closed")

	// ErrScopeNotFound is returned when no scope matches the requested reference.
	ErrScopeNotFound = errors.New("scope not found")

	// ErrScopeAmbiguous is returned when more than one scope matches the requested reference.
	ErrScopeAmbiguous = errors.New("scope ambiguous")

	// ErrPathCollision is returned when a page path collides with an existing one under PortableKey.
	ErrPathCollision = errors.New("page path collision")

	// ErrSchemaAhead is returned when the database schema is newer than the running binary.
	ErrSchemaAhead = errors.New("schema ahead of binary")

	// ErrInconsistentState is returned when a file write cannot be rolled back after an index failure.
	ErrInconsistentState = errors.New("inconsistent state")
)
