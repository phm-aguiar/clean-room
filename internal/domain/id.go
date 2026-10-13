package domain

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// ErrInvalidID é a causa de todo erro de leitura de identificador (texto ou bytes).
var ErrInvalidID = errors.New("identificador inválido")

// ID é um UUIDv7 de 16 bytes. Tipo base dos identificadores do domínio (ADR-07).
// Tipos como WorkspaceID são tipos definidos distintos: não há conversão implícita entre eles,
// e a passagem de um no lugar de outro é erro de compilação. Converta explicitamente, por exemplo WorkspaceID(id).
type ID [16]byte

// WorkspaceID identifica um workspace.
type WorkspaceID ID

// ProjectID identifica um projeto dentro de um workspace.
type ProjectID ID

// PageID identifica uma versão de página (cada atualização gera um novo PageID).
type PageID ID

// SessionID identifica uma sessão de agente.
type SessionID ID

// ObservationID identifica uma observação.
type ObservationID ID

// NewID gera um UUIDv7 novo com google/uuid. Propaga o erro do gerador.
func NewID() (ID, error) {
	u, err := uuid.NewV7()
	if err != nil {
		return ID{}, fmt.Errorf("gerar UUIDv7: %w", err)
	}
	return ID(u), nil
}

// ParseID aceita somente a forma textual do UUID com hífens (8-4-4-4-12, hexadecimal, maiúsculas ou minúsculas).
// Outras formas (sem hífens, com chaves, prefixo urn:uuid:, espaços) são recusadas.
func ParseID(s string) (ID, error) {
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return ID{}, fmt.Errorf("%w: %q não está na forma canônica com hífens", ErrInvalidID, s)
	}
	u, err := uuid.Parse(s)
	if err != nil {
		return ID{}, fmt.Errorf("%w: %q", ErrInvalidID, s)
	}
	return ID(u), nil
}

// String devolve a forma canônica com hífens, em minúsculas. É o texto usado em JSON e em URL.
func (id ID) String() string {
	return uuid.UUID(id).String()
}

// Bytes devolve uma cópia nova de 16 bytes, para o driver de banco (BLOB).
// Alterar o slice devolvido não altera o ID.
func (id ID) Bytes() []byte {
	return append([]byte(nil), id[:]...)
}

// ParseIDBytes lê um ID de exatamente 16 bytes. Qualquer outro tamanho é erro.
func ParseIDBytes(b []byte) (ID, error) {
	if len(b) != 16 {
		return ID{}, fmt.Errorf("%w: esperados 16 bytes, recebidos %d", ErrInvalidID, len(b))
	}
	var id ID
	copy(id[:], b)
	return id, nil
}

// String devolve a forma canônica com hífens do identificador de workspace.
func (v WorkspaceID) String() string { return ID(v).String() }

// Bytes devolve uma cópia de 16 bytes do identificador de workspace.
func (v WorkspaceID) Bytes() []byte { return ID(v).Bytes() }

// String devolve a forma canônica com hífens do identificador de projeto.
func (v ProjectID) String() string { return ID(v).String() }

// Bytes devolve uma cópia de 16 bytes do identificador de projeto.
func (v ProjectID) Bytes() []byte { return ID(v).Bytes() }

// String devolve a forma canônica com hífens do identificador de página.
func (v PageID) String() string { return ID(v).String() }

// Bytes devolve uma cópia de 16 bytes do identificador de página.
func (v PageID) Bytes() []byte { return ID(v).Bytes() }

// String devolve a forma canônica com hífens do identificador de sessão.
func (v SessionID) String() string { return ID(v).String() }

// Bytes devolve uma cópia de 16 bytes do identificador de sessão.
func (v SessionID) Bytes() []byte { return ID(v).Bytes() }

// String devolve a forma canônica com hífens do identificador de observação.
func (v ObservationID) String() string { return ID(v).String() }

// Bytes devolve uma cópia de 16 bytes do identificador de observação.
func (v ObservationID) Bytes() []byte { return ID(v).Bytes() }
