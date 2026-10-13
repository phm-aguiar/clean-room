package domain

import (
	"errors"
	"fmt"
)

// ErrMalformedRecord é a causa dos conversores de texto para conjunto fechado quando o valor
// é desconhecido (DV §3, DV §5.1). Os erros devolvidos encapsulam este sentinela com %w.
var ErrMalformedRecord = errors.New("registro malformado")

// Tier é o nível de memória de uma página (DV §6.1, ADR-28). O conjunto é fechado.
type Tier string

// Os quatro níveis, em minúsculas.
const (
	TierWorking    Tier = "working"
	TierEpisodic   Tier = "episodic"
	TierSemantic   Tier = "semantic"
	TierProcedural Tier = "procedural"
)

// ParseTier aceita somente os quatro valores exatos de Tier. Não normaliza caixa nem aparagem:
// `Working` é recusado. Valor desconhecido devolve erro que encapsula ErrMalformedRecord.
func ParseTier(s string) (Tier, error) {
	switch Tier(s) {
	case TierWorking, TierEpisodic, TierSemantic, TierProcedural:
		return Tier(s), nil
	}
	return "", fmt.Errorf("tier %q: %w", s, ErrMalformedRecord)
}
