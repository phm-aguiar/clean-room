package domain

import (
	"errors"
	"fmt"

	"github.com/phm-aguiar/ai-memory-go/internal/domain/sanitize"
)

// ErrInvalidFrontmatterKey é a causa de Set quando a chave é vazia.
var ErrInvalidFrontmatterKey = errors.New("chave de frontmatter inválida")

// ErrInvalidFrontmatterValue é a causa de Set quando o valor não é string nem []string.
var ErrInvalidFrontmatterValue = errors.New("valor de frontmatter inválido")

// ErrInvalidImportance é a causa de NewObservation quando a importância está fora de 1 a 10.
var ErrInvalidImportance = errors.New("importância fora de 1 a 10")

// ErrInvalidObservationKind é a causa de NewObservation quando o kind não é um dos dez canônicos.
var ErrInvalidObservationKind = errors.New("kind de observação fora do conjunto canônico")

// Limites e padrão de importância de observação (DV §5.2).
const (
	MinImportance     = 1
	MaxImportance     = 10
	DefaultImportance = 5
)

// Frontmatter é um mapa com chaves ordenadas, sem YAML (ADR-17). Preserva a ordem de inserção:
// Set em chave nova acrescenta ao fim, e Set em chave existente troca o valor sem mudar a posição.
// O zero value é um frontmatter vazio e pronto para uso. Não faz I/O e não entra em panic.
//
// Valores aceitos: string ou []string. Outros tipos são recusados por Set com
// ErrInvalidFrontmatterValue. A leitura e a emissão YAML ficam na T039.
type Frontmatter struct {
	entries []frontmatterEntry
}

type frontmatterEntry struct {
	key   string
	value any
}

// Set grava key com value. Chave nova vai ao fim; chave existente recebe o novo valor na mesma
// posição. Chave vazia ou valor de tipo não aceito devolve erro e não altera o mapa.
// []string é copiado, para que alterar o slice de origem não mude o frontmatter.
func (f *Frontmatter) Set(key string, value any) error {
	if key == "" {
		return ErrInvalidFrontmatterKey
	}
	var stored any
	switch v := value.(type) {
	case string:
		stored = v
	case []string:
		stored = append([]string(nil), v...)
	default:
		return fmt.Errorf("%w: chave %q, tipo %T", ErrInvalidFrontmatterValue, key, value)
	}
	for i := range f.entries {
		if f.entries[i].key == key {
			f.entries[i].value = stored
			return nil
		}
	}
	f.entries = append(f.entries, frontmatterEntry{key: key, value: stored})
	return nil
}

// Get devolve o valor de key e true, ou nil e false se a chave não existe.
// O valor devolvido é string ou []string. Para []string, a cópia devolvida é independente.
func (f Frontmatter) Get(key string) (any, bool) {
	for _, e := range f.entries {
		if e.key == key {
			if s, ok := e.value.([]string); ok {
				return append([]string(nil), s...), true
			}
			return e.value, true
		}
	}
	return nil, false
}

// Keys devolve as chaves na ordem de inserção. O slice é uma cópia nova.
func (f Frontmatter) Keys() []string {
	keys := make([]string, 0, len(f.entries))
	for _, e := range f.entries {
		keys = append(keys, e.key)
	}
	return keys
}

// Equal diz se os dois frontmatters têm as mesmas chaves, na mesma ordem, com os mesmos valores.
// A comparação é determinística: a ordem faz parte da igualdade.
func (f Frontmatter) Equal(other Frontmatter) bool {
	if len(f.entries) != len(other.entries) {
		return false
	}
	for i := range f.entries {
		a, b := f.entries[i], other.entries[i]
		if a.key != b.key {
			return false
		}
		switch av := a.value.(type) {
		case string:
			bv, ok := b.value.(string)
			if !ok || av != bv {
				return false
			}
		case []string:
			bv, ok := b.value.([]string)
			if !ok || len(av) != len(bv) {
				return false
			}
			for j := range av {
				if av[j] != bv[j] {
					return false
				}
			}
		default:
			return false
		}
	}
	return true
}

// Page é a página materializada da 001 (DV §6.3, PLAN §2.1). Só traz os campos com chamador
// na 001. O corpo e o título são o texto do arquivo, já sanitizados por quem chama.
//
// Title é string aparada e não sanitize.Sanitized: o título de página não tem teto fixo
// na 001 (ADR-10, lacuna em markdown-contract (b)). Aplicar Limits aqui forçaria uma decisão
// que ainda está em aberto. O corpo também é string: ADR-10 manda não truncar o corpo de página.
// Kind vazio significa "kind ausente" (ADR-08).
type Page struct {
	Ref         PageRef
	Tier        Tier
	Kind        PageKind
	Title       string
	Body        string
	Frontmatter Frontmatter
	// Version é o PageID da versão corrente (DV §4.1: o PageID identifica a versão, não o caminho).
	Version PageID
}

// PageVersion identifica uma versão de página e sua cadeia de supersessão (DV §6.3).
//
//   - ID é o PageID da versão. Cada atualização gera um novo.
//   - Supersedes aponta para a versão anterior. Nil significa primeira versão da cadeia.
//   - IsLatest vale true só na versão corrente.
//   - ValidFrom e ValidTo são instantes em microssegundos Unix UTC. ValidTo zero significa
//     versão ainda aberta (corrente).
type PageVersion struct {
	ID         PageID
	Supersedes *PageID
	IsLatest   bool
	ValidFrom  int64
	ValidTo    int64
}

// Observation é uma observação de ciclo de vida já pronta para o armazenamento (DV §5.2).
// Title e Body são do tipo sanitize.Sanitized: só a sanitização produz esse texto (INV-06).
//
// Importance vai de MinImportance a MaxImportance. OccurredAt é em microssegundos Unix UTC,
// e zero significa "agora", preenchido na gravação por quem chama. Extension e SourceEvent são
// strings vazias quando o evento não declara namespace ou origem.
type Observation struct {
	ID          ObservationID
	SessionID   SessionID
	Scope       Scope
	Kind        ObservationKind
	Title       sanitize.Sanitized
	Body        sanitize.Sanitized
	Importance  int
	OccurredAt  int64
	Extension   string
	SourceEvent string
}

// NewObservation monta uma Observation validada. Recusa importância fora de 1..10 com
// ErrInvalidImportance e kind fora dos dez canônicos com ErrInvalidObservationKind.
// Recebe Title e Body já como sanitize.Sanitized, então não há caminho de texto cru.
// OccurredAt e os campos opcionais ficam com o zero value, para o chamador preencher.
func NewObservation(
	id ObservationID,
	session SessionID,
	scope Scope,
	kind ObservationKind,
	title, body sanitize.Sanitized,
	importance int,
) (Observation, error) {
	if importance < MinImportance || importance > MaxImportance {
		return Observation{}, fmt.Errorf("%w: %d", ErrInvalidImportance, importance)
	}
	if _, ok := observationKindExact(string(kind)); !ok {
		return Observation{}, fmt.Errorf("%w: %q", ErrInvalidObservationKind, kind)
	}
	return Observation{
		ID:         id,
		SessionID:  session,
		Scope:      scope,
		Kind:       kind,
		Title:      title,
		Body:       body,
		Importance: importance,
	}, nil
}
