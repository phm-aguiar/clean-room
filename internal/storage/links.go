package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// linkTypeReferences é o link_type padrão: link de corpo e relação desconhecida (ADR-19).
const linkTypeReferences = "references"

// maxRelationLogBytes é o limite de bytes de um valor de relação vindo do frontmatter (contrato, seção (c)).
const maxRelationLogBytes = 200

// Link é um link de saída já extraído de uma página. A extração (corpo e chave
// relations do frontmatter) é da T029; aqui só se grava o resultado.
type Link struct {
	// ToWorkspace é o nome do workspace de destino. nil = workspace da página de origem.
	ToWorkspace *string
	// ToProject é o nome do projeto de destino. nil = projeto da página de origem.
	ToProject *string
	// ToPath é o caminho do destino dentro do projeto.
	ToPath domain.PagePath
	// Relation é o nome da relação tipada. Vazio = link de corpo (references).
	Relation string
}

// linkKey é a chave lógica de dedupe (to_workspace, to_project, to_path, link_type).
// sql.NullString separa NULL de string vazia e é comparável com ==.
type linkKey struct {
	ws, pj sql.NullString
	path   string
	lt     string
}

// ReplaceLinks regrava o conjunto completo de links de from: apaga os links atuais
// da página e insere links. Deve rodar dentro da mesma transação tx da escrita da
// página (INV-03). A página from precisa existir em tx.
//
// Antes de inserir, deduplica pela chave lógica e preserva a primeira ocorrência.
// Relação fora de causes|fixes|contradicts vira references (ADR-19); o nome
// (truncado a 200 bytes) volta em avisos, uma entrada por link gravado com essa
// condição, para que o chamador registre o aviso. Não é erro.
//
// Se o alvo já existe (versão mais recente, is_latest = 1), to_page_id é preenchido
// já na inserção; senão o link fica pendente até RepairPendingLinks.
func ReplaceLinks(tx *sql.Tx, from domain.PageID, links []Link) (avisos []string, err error) {
	originWS, originPJ, _, err := pageIdentity(tx, from)
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(`DELETE FROM links WHERE from_page_id = ?`, from.Bytes()); err != nil {
		return nil, fmt.Errorf("delete links: %w", err)
	}

	seen := make(map[linkKey]struct{}, len(links))
	for _, l := range links {
		path := l.ToPath.String()
		if path == "" {
			return nil, errors.New("link target path is empty")
		}
		if l.ToWorkspace != nil && *l.ToWorkspace == "" {
			return nil, errors.New("link target workspace name is empty")
		}
		if l.ToProject != nil && *l.ToProject == "" {
			return nil, errors.New("link target project name is empty")
		}

		lt, known := linkTypeOf(l.Relation)
		key := linkKey{ws: nullString(l.ToWorkspace), pj: nullString(l.ToProject), path: path, lt: lt}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}

		ws, pj := originWS, originPJ
		if l.ToWorkspace != nil {
			ws = *l.ToWorkspace
		}
		if l.ToProject != nil {
			pj = *l.ToProject
		}
		target, err := latestPageID(tx, ws, pj, path)
		if err != nil {
			return nil, err
		}

		if _, err := tx.Exec(
			`INSERT INTO links (from_page_id, to_page_id, to_workspace, to_project, to_path, link_type)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			from.Bytes(), target, key.ws, key.pj, path, lt,
		); err != nil {
			return nil, fmt.Errorf("insert link: %w", err)
		}

		if !known {
			avisos = append(avisos, truncateUTF8(l.Relation, maxRelationLogBytes))
		}
	}
	return avisos, nil
}

// RepairPendingLinks resolve os links pendentes (to_page_id IS NULL) que apontam
// para a página page recém-criada: mesmo caminho e mesmo escopo de destino (nome de
// workspace e de projeto, com NULL valendo o escopo da origem). Cobre também links
// entre projetos do mesmo workspace. Deve rodar em tx, depois do INSERT da página.
func RepairPendingLinks(tx *sql.Tx, page domain.PageID) error {
	ws, pj, path, err := pageIdentity(tx, page)
	if err != nil {
		return err
	}

	if _, err := tx.Exec(
		`UPDATE links SET to_page_id = ?
		 WHERE to_page_id IS NULL
		   AND to_path = ?
		   AND COALESCE(to_workspace,
		         (SELECT w.name FROM pages f JOIN workspaces w ON w.id = f.workspace_id
		          WHERE f.id = links.from_page_id)) = ?
		   AND COALESCE(to_project,
		         (SELECT j.name FROM pages f JOIN projects j ON j.id = f.project_id
		          WHERE f.id = links.from_page_id)) = ?`,
		page.Bytes(), path, ws, pj,
	); err != nil {
		return fmt.Errorf("repair pending links: %w", err)
	}
	return nil
}

// pageIdentity devolve os nomes de workspace e de projeto e o caminho da página id.
func pageIdentity(tx *sql.Tx, id domain.PageID) (ws, pj, path string, err error) {
	err = tx.QueryRow(
		`SELECT w.name, j.name, p.path
		 FROM pages p
		 JOIN workspaces w ON w.id = p.workspace_id
		 JOIN projects j ON j.id = p.project_id
		 WHERE p.id = ?`,
		id.Bytes(),
	).Scan(&ws, &pj, &path)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", "", fmt.Errorf("page %s not found: %w", id.String(), err)
	}
	if err != nil {
		return "", "", "", fmt.Errorf("read page identity: %w", err)
	}
	return ws, pj, path, nil
}

// latestPageID devolve o id da versão mais recente de (ws, pj, path), ou nil se não existe.
// O retorno é any para que nil vire NULL na inserção.
func latestPageID(tx *sql.Tx, ws, pj, path string) (any, error) {
	var id []byte
	err := tx.QueryRow(
		`SELECT p.id
		 FROM pages p
		 JOIN workspaces w ON w.id = p.workspace_id
		 JOIN projects j ON j.id = p.project_id
		 WHERE w.name = ? AND j.name = ? AND p.path = ? AND p.is_latest = 1`,
		ws, pj, path,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lookup link target: %w", err)
	}
	return id, nil
}

// linkTypeOf mapeia o nome da relação para link_type. Vazio é link de corpo.
// O bool é false quando a relação é desconhecida e cai em references (ADR-19).
func linkTypeOf(relation string) (string, bool) {
	switch relation {
	case "":
		return linkTypeReferences, true
	case "causes", "fixes", "contradicts":
		return relation, true
	default:
		return linkTypeReferences, false
	}
}

func nullString(p *string) sql.NullString {
	if p == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *p, Valid: true}
}

// truncateUTF8 corta s em no máximo n bytes sem quebrar um caractere UTF-8.
func truncateUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

// RepointIncomingLinksTx reaponta os links que apontam para a versão antiga (from) para a versão
// nova (to) do mesmo caminho. Dentro da transação da supersessão (T081). Sem isso, os vizinhos
// do grafo, que são consultados por to_page_id, somem depois de qualquer nova versão.
func RepointIncomingLinksTx(tx *sql.Tx, from, to domain.PageID) error {
	if _, err := tx.Exec(
		`UPDATE links SET to_page_id = ? WHERE to_page_id = ?`,
		to.Bytes(), from.Bytes(),
	); err != nil {
		return fmt.Errorf("reapontar links de entrada: %w", err)
	}
	return nil
}
