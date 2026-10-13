package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// genAtKey é a chave de frontmatter fora da comparação de conteúdo (ADR-18).
const genAtKey = "generated.at"

// kindKey é a chave de frontmatter onde o kind é persistido. A tabela pages não tem
// coluna kind: o kind é uma chave do frontmatter (markdown-contract, tabela de chaves).
const kindKey = "kind"

// Operações de audit_log gravadas pelo UpsertPage. O catálogo de ops não está
// fixado no esquema; estes são os valores usados pela 001.
const (
	auditPageUnchanged = "page-unchanged"
	auditPageSupersede = "page-supersede"
	auditPageCreate    = "page-create"
	auditPageResurrect = "page-resurrect"
)

// UpsertOutcome diz qual caso do P-04 o UpsertPage executou.
type UpsertOutcome string

const (
	// UpsertUnchanged é o caso A: conteúdo idêntico, nenhuma versão nova.
	UpsertUnchanged UpsertOutcome = "unchanged"
	// UpsertSuperseded é o caso B: a versão corrente foi substituída por uma nova.
	UpsertSuperseded UpsertOutcome = "superseded"
	// UpsertCreated é o caso C: primeira versão do caminho no escopo.
	UpsertCreated UpsertOutcome = "created"
	// UpsertResurrected é o caso C com tombstone (lápide de decaimento) do mesmo caminho exato.
	UpsertResurrected UpsertOutcome = "resurrected"
)

// EvidenceSource identifica a fonte que motivou a escrita. Kind é um SourceKind fechado;
// ID é o identificador da fonte (sessão, observação, feedback ou reconsolidação).
type EvidenceSource struct {
	Kind SourceKind
	ID   string
}

// UpsertInput é a página a gravar. Ref traz escopo e caminho; Frontmatter é o frontmatter
// sem o kind (o kind entra como chave kind quando não vazio). Links são os links de saída
// já extraídos pelo chamador. At é o instante da escrita (versões, janelas e auditoria).
type UpsertInput struct {
	Ref         domain.PageRef
	Tier        domain.Tier
	Kind        domain.PageKind
	Title       string
	Body        string
	Frontmatter domain.Frontmatter
	Pinned      bool
	Links       []Link
	Evidence    EvidenceSource
	At          time.Time
}

// UpsertResult devolve a versão corrente do caminho depois da escrita, o caso executado
// e os avisos de ReplaceLinks (relações desconhecidas gravadas como references).
type UpsertResult struct {
	Version domain.PageID
	Outcome UpsertOutcome
	Avisos  []string
}

// pageRow é a versão corrente lida para comparação.
type pageRow struct {
	id          domain.PageID
	title       string
	tier        string
	body        string
	pinned      bool
	frontmatter string
}

// UpsertPage executa UpsertPageTx em uma transação própria, por Writer.Do. Erros voltam
// com %w: ErrPathCollision é preservado para errors.Is.
func UpsertPage(ctx context.Context, w Writer, in UpsertInput) (UpsertResult, error) {
	var res UpsertResult
	err := w.Do(ctx, func(tx *sql.Tx) error {
		r, err := UpsertPageTx(tx, in)
		if err != nil {
			return err
		}
		res = r
		return nil
	})
	if err != nil {
		return UpsertResult{}, fmt.Errorf("upsert page %s: %w", in.Ref.String(), err)
	}
	return res, nil
}

// UpsertPageTx grava a página dentro de tx, que deve ser a transação de Writer.Do (INV-03).
// Escolhe um dos três casos do P-04:
//
//   - A: já existe versão corrente com o mesmo conteúdo. Não cria versão; grava evidência e auditoria.
//   - B: já existe versão corrente com conteúdo diferente. A versão corrente vira is_latest = 0
//     com valid_to = At; as janelas de entidade dela são fechadas; a nova versão recebe
//     supersedes apontando para ela. Os links de entrada de outras páginas passam a apontar para
//     a versão nova. A versão antiga permanece no banco (INV-16).
//   - C: não há versão corrente do caminho exato. Antes de inserir, compara PortableKey com os
//     caminhos correntes do mesmo projeto (ADR-06) e devolve ErrPathCollision se houver colisão.
//     Se existe lápide de decaimento do caminho exato, a nova versão a sucede (ressurreição).
//
// Nenhuma escrita de arquivo nem commit: isso é da T046. A rede de segurança contra corrida é o
// índice único parcial idx_pages_latest_path, que não é contornado aqui.
func UpsertPageTx(tx *sql.Tx, in UpsertInput) (UpsertResult, error) {
	if tx == nil {
		return UpsertResult{}, errors.New("upsert page: transação nula")
	}
	if in.Ref.Path.String() == "" {
		return UpsertResult{}, errors.New("upsert page: caminho vazio")
	}
	if in.At.IsZero() {
		return UpsertResult{}, errors.New("upsert page: instante ausente")
	}
	if in.Evidence.Kind == "" || in.Evidence.ID == "" {
		return UpsertResult{}, errors.New("upsert page: fonte de evidência ausente")
	}

	fm, err := effectiveFrontmatter(in)
	if err != nil {
		return UpsertResult{}, fmt.Errorf("upsert page: frontmatter: %w", err)
	}

	head, found, err := findLatestPage(tx, in.Ref)
	if err != nil {
		return UpsertResult{}, err
	}
	if found {
		same, err := sameContent(head, in, fm)
		if err != nil {
			return UpsertResult{}, err
		}
		if same {
			return upsertUnchanged(tx, in, head.id)
		}
		return upsertSupersede(tx, in, fm, head)
	}
	return upsertCreate(tx, in, fm)
}

// upsertUnchanged é o caso A: só evidência e auditoria, sobre a versão corrente.
func upsertUnchanged(tx *sql.Tx, in UpsertInput, id domain.PageID) (UpsertResult, error) {
	if err := InsertEvidence(tx, id, in.Evidence.Kind, in.Evidence.ID, in.At); err != nil {
		return UpsertResult{}, err
	}
	if err := insertPageAudit(tx, in, auditPageUnchanged, id); err != nil {
		return UpsertResult{}, err
	}
	return UpsertResult{Version: id, Outcome: UpsertUnchanged}, nil
}

// upsertSupersede é o caso B.
func upsertSupersede(tx *sql.Tx, in UpsertInput, fm domain.Frontmatter, head pageRow) (UpsertResult, error) {
	at := in.At.UnixMicro()

	res, err := tx.Exec(
		`UPDATE pages SET is_latest = 0, valid_to = ? WHERE id = ? AND is_latest = 1`,
		at, head.id.Bytes(),
	)
	if err != nil {
		return UpsertResult{}, fmt.Errorf("retirar versão corrente: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return UpsertResult{}, fmt.Errorf("retirar versão corrente: %w", err)
	} else if n != 1 {
		return UpsertResult{}, fmt.Errorf("retirar versão corrente de %s: esperada 1 linha, afetadas %d", head.id.String(), n)
	}

	if err := CloseEntityLinksTx(tx, head.id, at); err != nil {
		return UpsertResult{}, err
	}

	prev := head.id
	newID, err := insertPageVersion(tx, in, fm, &prev, at)
	if err != nil {
		return UpsertResult{}, err
	}
	if err := RepointIncomingLinksTx(tx, prev, newID); err != nil {
		return UpsertResult{}, err
	}
	avisos, err := finishPageVersion(tx, in, fm, newID, at)
	if err != nil {
		return UpsertResult{}, err
	}
	if err := insertPageAudit(tx, in, auditPageSupersede, newID); err != nil {
		return UpsertResult{}, err
	}
	return UpsertResult{Version: newID, Outcome: UpsertSuperseded, Avisos: avisos}, nil
}

// upsertCreate é o caso C: colisão de caminho, ressurreição ou primeira versão.
func upsertCreate(tx *sql.Tx, in UpsertInput, fm domain.Frontmatter) (UpsertResult, error) {
	if err := checkPathCollision(tx, in.Ref); err != nil {
		return UpsertResult{}, err
	}

	at := in.At.UnixMicro()
	tomb, hasTomb, err := findTombstone(tx, in.Ref)
	if err != nil {
		return UpsertResult{}, err
	}

	var prev *domain.PageID
	outcome := UpsertCreated
	op := auditPageCreate
	if hasTomb {
		// A lápide não é apagada nem reativada: a nova versão a sucede. A janela de validade
		// dela é fechada se estiver aberta, para não sobrepor a nova versão no as_of.
		if _, err := tx.Exec(
			`UPDATE pages SET valid_to = ? WHERE id = ? AND valid_to IS NULL`,
			at, tomb.Bytes(),
		); err != nil {
			return UpsertResult{}, fmt.Errorf("fechar janela da lápide: %w", err)
		}
		if err := CloseEntityLinksTx(tx, tomb, at); err != nil {
			return UpsertResult{}, err
		}
		prev = &tomb
		outcome = UpsertResurrected
		op = auditPageResurrect
	}

	newID, err := insertPageVersion(tx, in, fm, prev, at)
	if err != nil {
		return UpsertResult{}, err
	}
	avisos, err := finishPageVersion(tx, in, fm, newID, at)
	if err != nil {
		return UpsertResult{}, err
	}
	if err := insertPageAudit(tx, in, op, newID); err != nil {
		return UpsertResult{}, err
	}
	return UpsertResult{Version: newID, Outcome: outcome, Avisos: avisos}, nil
}

// insertPageVersion insere a versão corrente (is_latest = 1). A unicidade do caminho é
// imposta por idx_pages_latest_path: uma corrida que chegasse aqui falha com erro do banco.
func insertPageVersion(tx *sql.Tx, in UpsertInput, fm domain.Frontmatter, supersedes *domain.PageID, at int64) (domain.PageID, error) {
	id, err := domain.NewID()
	if err != nil {
		return domain.PageID{}, fmt.Errorf("gerar id de página: %w", err)
	}
	pageID := domain.PageID(id)

	fmJSON, err := encodeFrontmatter(fm)
	if err != nil {
		return domain.PageID{}, fmt.Errorf("codificar frontmatter: %w", err)
	}
	sum := sha256.Sum256([]byte(in.Body))

	var sup any
	if supersedes != nil {
		sup = supersedes.Bytes()
	}
	pinned := 0
	if in.Pinned {
		pinned = 1
	}

	if _, err := tx.Exec(
		`INSERT INTO pages (id, workspace_id, project_id, path, title, tier, body, body_sha256,
		                    frontmatter_json, is_latest, supersedes, pinned, created_at, updated_at,
		                    path_search, valid_from)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?)`,
		pageID.Bytes(), in.Ref.Scope.WorkspaceID.Bytes(), in.Ref.Scope.ProjectID.Bytes(),
		in.Ref.Path.String(), in.Title, string(in.Tier), in.Body, sum[:],
		string(fmJSON), sup, pinned, at, at,
		in.Ref.Path.SearchText(), at,
	); err != nil {
		return domain.PageID{}, fmt.Errorf("inserir versão de página: %w", err)
	}
	return pageID, nil
}

// finishPageVersion grava links, repara pendentes, entidades e evidência da versão nova.
// ReplaceLinks e RepairPendingLinks precisam que a versão já exista na transação.
func finishPageVersion(tx *sql.Tx, in UpsertInput, fm domain.Frontmatter, id domain.PageID, at int64) ([]string, error) {
	avisos, err := ReplaceLinks(tx, id, in.Links)
	if err != nil {
		return nil, err
	}
	if err := RepairPendingLinks(tx, id); err != nil {
		return nil, err
	}
	if err := LinkEntitiesTx(tx, in.Ref.Scope, id, at, DeriveEntities(fm)); err != nil {
		return nil, err
	}
	if err := InsertEvidence(tx, id, in.Evidence.Kind, in.Evidence.ID, in.At); err != nil {
		return nil, err
	}
	return avisos, nil
}

// insertPageAudit grava a linha de auditoria da escrita, com a página alvo.
func insertPageAudit(tx *sql.Tx, in UpsertInput, op string, id domain.PageID) error {
	target := id
	return InsertAudit(tx, in.At, AuditEvent{Op: op, Scope: in.Ref.Scope, Page: &target})
}

// findLatestPage lê a versão corrente do caminho exato, se existir.
func findLatestPage(tx *sql.Tx, ref domain.PageRef) (pageRow, bool, error) {
	var (
		idBytes []byte
		row     pageRow
		pinned  int
	)
	err := tx.QueryRow(
		`SELECT id, title, tier, body, pinned, frontmatter_json
		   FROM pages
		  WHERE workspace_id = ? AND project_id = ? AND path = ? AND is_latest = 1`,
		ref.Scope.WorkspaceID.Bytes(), ref.Scope.ProjectID.Bytes(), ref.Path.String(),
	).Scan(&idBytes, &row.title, &row.tier, &row.body, &pinned, &row.frontmatter)
	if errors.Is(err, sql.ErrNoRows) {
		return pageRow{}, false, nil
	}
	if err != nil {
		return pageRow{}, false, fmt.Errorf("ler versão corrente: %w", err)
	}
	id, err := domain.ParseIDBytes(idBytes)
	if err != nil {
		return pageRow{}, false, fmt.Errorf("ler id da versão corrente: %w", err)
	}
	row.id = domain.PageID(id)
	row.pinned = pinned != 0
	return row, true, nil
}

// findTombstone devolve a cabeça da cadeia do caminho exato quando ela é uma lápide de
// decaimento: is_latest = 0 e superseded_at preenchido (database-schema, §1). Não há
// coluna de deleção lógica; a lápide é esse estado.
func findTombstone(tx *sql.Tx, ref domain.PageRef) (domain.PageID, bool, error) {
	var idBytes []byte
	err := tx.QueryRow(
		`SELECT id FROM pages
		  WHERE workspace_id = ? AND project_id = ? AND path = ?
		    AND is_latest = 0 AND superseded_at IS NOT NULL
		  ORDER BY created_at DESC, rowid DESC
		  LIMIT 1`,
		ref.Scope.WorkspaceID.Bytes(), ref.Scope.ProjectID.Bytes(), ref.Path.String(),
	).Scan(&idBytes)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PageID{}, false, nil
	}
	if err != nil {
		return domain.PageID{}, false, fmt.Errorf("ler lápide do caminho: %w", err)
	}
	id, err := domain.ParseIDBytes(idBytes)
	if err != nil {
		return domain.PageID{}, false, fmt.Errorf("ler id da lápide: %w", err)
	}
	return domain.PageID(id), true, nil
}

// checkPathCollision devolve ErrPathCollision se algum caminho corrente do mesmo projeto
// tem a mesma PortableKey e outra grafia (ADR-06). O custo é O(n) nas páginas correntes
// do projeto; é o débito registrado no ADR-06.
func checkPathCollision(tx *sql.Tx, ref domain.PageRef) error {
	paths, err := currentPaths(tx, ref)
	if err != nil {
		return err
	}
	key := ref.Path.PortableKey()
	for _, stored := range paths {
		other, err := domain.ParseStoredPagePath(stored)
		if err != nil {
			return fmt.Errorf("caminho armazenado inválido %q: %w", stored, err)
		}
		if other.String() == ref.Path.String() {
			continue
		}
		if other.PortableKey() == key {
			return fmt.Errorf("%w: %q colide com %q no mesmo projeto", ErrPathCollision, ref.Path.String(), stored)
		}
	}
	return nil
}

// currentPaths lista os caminhos correntes do projeto. Lê tudo antes de devolver, para
// que o cursor esteja fechado quando a transação executar a próxima instrução.
func currentPaths(tx *sql.Tx, ref domain.PageRef) ([]string, error) {
	rows, err := tx.Query(
		`SELECT path FROM pages WHERE workspace_id = ? AND project_id = ? AND is_latest = 1`,
		ref.Scope.WorkspaceID.Bytes(), ref.Scope.ProjectID.Bytes(),
	)
	if err != nil {
		return nil, fmt.Errorf("listar caminhos correntes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, fmt.Errorf("ler caminho corrente: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterar caminhos correntes: %w", err)
	}
	return out, nil
}

// sameContent diz se a versão corrente e a entrada têm o mesmo conteúdo (ADR-18): corpo,
// título, tier, fixação e frontmatter sem generated.at.
func sameContent(head pageRow, in UpsertInput, fm domain.Frontmatter) (bool, error) {
	if head.title != in.Title || head.tier != string(in.Tier) || head.body != in.Body || head.pinned != in.Pinned {
		return false, nil
	}
	stored, err := decodeFrontmatter(head.frontmatter)
	if err != nil {
		return false, fmt.Errorf("ler frontmatter da versão corrente: %w", err)
	}
	storedContent, err := copyFrontmatter(stored, genAtKey)
	if err != nil {
		return false, err
	}
	newContent, err := copyFrontmatter(fm, genAtKey)
	if err != nil {
		return false, err
	}
	return storedContent.Equal(newContent), nil
}

// effectiveFrontmatter devolve uma cópia do frontmatter da entrada com kind acrescentado
// quando não vazio. Não altera a entrada.
func effectiveFrontmatter(in UpsertInput) (domain.Frontmatter, error) {
	fm, err := copyFrontmatter(in.Frontmatter, "")
	if err != nil {
		return domain.Frontmatter{}, err
	}
	if in.Kind != "" {
		if err := fm.Set(kindKey, string(in.Kind)); err != nil {
			return domain.Frontmatter{}, err
		}
	}
	return fm, nil
}

// copyFrontmatter copia src na ordem de inserção, omitindo skip (vazio = nada omitido).
// Copia por Set, então o resultado não compartilha o slice de origem.
func copyFrontmatter(src domain.Frontmatter, skip string) (domain.Frontmatter, error) {
	var out domain.Frontmatter
	for _, k := range src.Keys() {
		if k == skip {
			continue
		}
		v, _ := src.Get(k)
		if err := out.Set(k, v); err != nil {
			return domain.Frontmatter{}, err
		}
	}
	return out, nil
}

// encodeFrontmatter serializa o frontmatter como objeto JSON na ordem de inserção. Valores são
// string ou []string, então json.Marshal não falha; o erro é devolvido por cautela.
func encodeFrontmatter(fm domain.Frontmatter) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range fm.Keys() {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		v, _ := fm.Get(k)
		vb, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		buf.Write(kb)
		buf.WriteByte(':')
		buf.Write(vb)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// decodeFrontmatter lê o frontmatter_json gravado por encodeFrontmatter, preservando a ordem.
func decodeFrontmatter(raw string) (domain.Frontmatter, error) {
	dec := json.NewDecoder(strings.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return domain.Frontmatter{}, fmt.Errorf("frontmatter_json: %w", err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return domain.Frontmatter{}, errors.New("frontmatter_json não é objeto")
	}
	var fm domain.Frontmatter
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return domain.Frontmatter{}, fmt.Errorf("frontmatter_json: %w", err)
		}
		key, ok := tok.(string)
		if !ok {
			return domain.Frontmatter{}, errors.New("frontmatter_json: chave não é texto")
		}
		var v any
		if err := dec.Decode(&v); err != nil {
			return domain.Frontmatter{}, fmt.Errorf("frontmatter_json: valor de %q: %w", key, err)
		}
		switch x := v.(type) {
		case string:
			err = fm.Set(key, x)
		case []any:
			list := make([]string, 0, len(x))
			for _, e := range x {
				s, ok := e.(string)
				if !ok {
					return domain.Frontmatter{}, fmt.Errorf("frontmatter_json: lista de %q não é só texto", key)
				}
				list = append(list, s)
			}
			err = fm.Set(key, list)
		default:
			return domain.Frontmatter{}, fmt.Errorf("frontmatter_json: tipo não suportado em %q", key)
		}
		if err != nil {
			return domain.Frontmatter{}, fmt.Errorf("frontmatter_json: %q: %w", key, err)
		}
	}
	if _, err := dec.Token(); err != nil {
		return domain.Frontmatter{}, fmt.Errorf("frontmatter_json: %w", err)
	}
	return fm, nil
}
