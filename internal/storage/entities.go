package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// Limites de entidades (DV §6.5, §6.6).
const (
	// MaxEntityRunes é o teto de caracteres de uma entidade, medido no valor bruto.
	MaxEntityRunes = 64
	// MaxEntitiesPerPage é o teto de entidades por página.
	MaxEntitiesPerPage = 10
)

// ErrInvalidEntity é a causa de NormalizeEntity quando o nome é recusado.
var ErrInvalidEntity = errors.New("entidade inválida")

// NormalizeEntity aplica a normalização de DV §6.5 a um nome de entidade:
//
//  1. o limite de 64 caracteres é verificado sobre o valor bruto, antes de qualquer
//     transformação (DV §6.6); a contagem para ao ultrapassar o limite;
//  2. aparam-se as bordas e colapsam-se espaços internos em um só (espaço é qualquer
//     espaço Unicode, inclusive tab e quebra de linha);
//  3. converte-se para minúsculas;
//  4. recusa-se o resultado se for vazio ou se contiver caractere de controle.
//
// Rejeita, não trunca. Os erros têm ErrInvalidEntity na cadeia.
func NormalizeEntity(raw string) (string, error) {
	runes := 0
	for i := 0; i < len(raw); {
		r, size := utf8.DecodeRuneInString(raw[i:])
		if r == utf8.RuneError && size == 1 {
			return "", fmt.Errorf("%w: UTF-8 inválido", ErrInvalidEntity)
		}
		runes++
		if runes > MaxEntityRunes {
			return "", fmt.Errorf("%w: mais de %d caracteres brutos", ErrInvalidEntity, MaxEntityRunes)
		}
		i += size
	}

	var b strings.Builder
	pendingSpace := false
	for _, r := range raw {
		if unicode.IsSpace(r) {
			pendingSpace = b.Len() > 0
			continue
		}
		if unicode.IsControl(r) {
			return "", fmt.Errorf("%w: caractere de controle U+%04X", ErrInvalidEntity, r)
		}
		if pendingSpace {
			b.WriteByte(' ')
			pendingSpace = false
		}
		b.WriteRune(unicode.ToLower(r))
	}
	if b.Len() == 0 {
		return "", fmt.Errorf("%w: vazia", ErrInvalidEntity)
	}
	return b.String(), nil
}

// NormalizeEntities aplica DV §6.5 a uma lista: descarta entradas inválidas, remove
// duplicatas preservando a ordem da primeira ocorrência e corta em MaxEntitiesPerPage.
// A coleta para ao atingir o teto, então uma lista enorme não é processada até o fim.
// Devolve sempre um slice não nulo.
func NormalizeEntities(raw []string) []string {
	out := make([]string, 0, MaxEntitiesPerPage)
	seen := make(map[string]struct{}, MaxEntitiesPerPage)
	for _, r := range raw {
		if len(out) == MaxEntitiesPerPage {
			break
		}
		name, err := NormalizeEntity(r)
		if err != nil {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

// DeriveEntities deriva as entidades de uma página a partir do frontmatter (DV §6.5).
// Une a lista explícita `entities`, que vem primeiro para ganhar o limite, com `tags`,
// e normaliza o resultado com NormalizeEntities. É tolerante: campo ausente ou que não é
// lista não contribui e não gera erro. A validação estrita fica na escrita da wiki (T046).
func DeriveEntities(fm domain.Frontmatter) []string {
	all := append(listField(fm, "entities"), listField(fm, "tags")...)
	return NormalizeEntities(all)
}

// listField devolve o valor de key quando é []string; em qualquer outro caso, nil.
func listField(fm domain.Frontmatter, key string) []string {
	v, ok := fm.Get(key)
	if !ok {
		return nil
	}
	list, ok := v.([]string)
	if !ok {
		return nil
	}
	return list
}

// LinkEntitiesTx grava, dentro de uma transação já aberta por Writer.Do, os vínculos de
// pageID com as entidades de names. Names passa por NormalizeEntities. Cada entidade é
// criada se não existir (única por workspace, projeto e nome), e o vínculo é criado com
// valid_from = validFrom e superseded_at NULL (janela aberta). Vínculo que já existe é
// mantido como está, então a repetição não duplica nem reabre uma janela fechada.
//
// created_at da entidade recebe validFrom, porque é o instante de criação da versão que a
// introduziu. Quem chama deve estar dentro de Writer.Do.
func LinkEntitiesTx(tx *sql.Tx, scope domain.Scope, pageID domain.PageID, validFrom int64, names []string) error {
	for _, name := range NormalizeEntities(names) {
		entityID, err := upsertEntityTx(tx, scope, name, validFrom)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(
			`INSERT INTO entity_page_links (entity_id, page_id, valid_from, superseded_at)
			 VALUES (?, ?, ?, NULL)
			 ON CONFLICT (entity_id, page_id) DO NOTHING`,
			entityID, pageID.Bytes(), validFrom,
		); err != nil {
			return fmt.Errorf("gravar vínculo de entidade %q: %w", name, err)
		}
	}
	return nil
}

// upsertEntityTx cria a entidade name no escopo, se ainda não existe, e devolve o id dela.
// A criação é idempotente pela restrição UNIQUE (workspace, projeto, nome).
func upsertEntityTx(tx *sql.Tx, scope domain.Scope, name string, createdAt int64) ([]byte, error) {
	id, err := domain.NewID()
	if err != nil {
		return nil, fmt.Errorf("gerar id de entidade: %w", err)
	}
	ws := scope.WorkspaceID.Bytes()
	proj := scope.ProjectID.Bytes()
	if _, err := tx.Exec(
		`INSERT INTO entities (id, workspace_id, project_id, name, created_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (workspace_id, project_id, name) DO NOTHING`,
		id.Bytes(), ws, proj, name, createdAt,
	); err != nil {
		return nil, fmt.Errorf("gravar entidade %q: %w", name, err)
	}
	var entityID []byte
	if err := tx.QueryRow(
		`SELECT id FROM entities WHERE workspace_id = ? AND project_id = ? AND name = ?`,
		ws, proj, name,
	).Scan(&entityID); err != nil {
		return nil, fmt.Errorf("ler entidade %q: %w", name, err)
	}
	return entityID, nil
}

// CloseEntityLinksTx fecha as janelas abertas de pageID: superseded_at recebe supersededAt
// nos vínculos com superseded_at NULL. Dentro de uma transação de Writer.Do. Na supersessão,
// supersededAt é o instante de criação da versão nova (DB §4). Vínculos já fechados não mudam.
func CloseEntityLinksTx(tx *sql.Tx, pageID domain.PageID, supersededAt int64) error {
	if _, err := tx.Exec(
		`UPDATE entity_page_links SET superseded_at = ?
		 WHERE page_id = ? AND superseded_at IS NULL`,
		supersededAt, pageID.Bytes(),
	); err != nil {
		return fmt.Errorf("fechar janelas de entidade: %w", err)
	}
	return nil
}

// LinkEntities é a forma pública de LinkEntitiesTx: executa a gravação por Writer.Do, em
// uma transação própria.
func LinkEntities(ctx context.Context, w Writer, scope domain.Scope, pageID domain.PageID, validFrom int64, names []string) error {
	return w.Do(ctx, func(tx *sql.Tx) error {
		return LinkEntitiesTx(tx, scope, pageID, validFrom, names)
	})
}

// CloseEntityLinks é a forma pública de CloseEntityLinksTx: executa o fechamento por
// Writer.Do, em uma transação própria.
func CloseEntityLinks(ctx context.Context, w Writer, pageID domain.PageID, supersededAt int64) error {
	return w.Do(ctx, func(tx *sql.Tx) error {
		return CloseEntityLinksTx(tx, pageID, supersededAt)
	})
}
