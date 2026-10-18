package domain

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// sessionNameSpace é o namespace OID usado para derivar o UUID v5 de um identificador de sessão
// que não é UUID (lifecycle-event-ingestion.md §3.3 e §8, passo 1; hook-contract.md §4).
var sessionNameSpace = uuid.NameSpaceOID

// SessionIDFromNative converte o identificador de sessão nativo de um evento em SessionID.
//
// Regras (lifecycle-event-ingestion.md §3.3 e §8; hook-contract.md §4):
//   - Entrada UUID válida (forma canônica com hífens, 8-4-4-4-12): é usada como está.
//     Decisão nossa: o resultado é gravado na forma canônica minúscula (SessionID.String()),
//     porque o doc diz só "usado como está" e não fixa caixa. Maiúsculas e minúsculas do mesmo
//     UUID caem na mesma chave, e a forma minúscula é a usada em JSON e URL no domínio.
//   - Entrada que não é UUID (qualquer string não vazia): vira UUID v5 no namespace OID
//     sobre os bytes exatos da string, sem trim. É determinística: POST e GET da mesma sessão
//     chegam à mesma chave.
//   - Entrada vazia ou só com espaços: erro (ErrInvalidID). Esta função não gera identificador.
//     Para SessionStart sem session_id, a geração de um identificador novo (LEI §8, passo 1)
//     é responsabilidade de quem chama, não desta função.
//
// Função pura, sem I/O.
func SessionIDFromNative(s string) (SessionID, error) {
	if strings.TrimSpace(s) == "" {
		return SessionID{}, fmt.Errorf("%w: session_id vazio", ErrInvalidID)
	}
	if id, err := ParseID(s); err == nil {
		return SessionID(id), nil
	}
	return SessionID(uuid.NewSHA1(sessionNameSpace, []byte(s))), nil
}
