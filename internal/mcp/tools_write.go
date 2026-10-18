package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
	"github.com/phm-aguiar/ai-memory-go/internal/storage"
	"github.com/phm-aguiar/ai-memory-go/internal/wiki"
)

// Ferramentas de escrita de página (docs/mcp-server.md §3.3; specs/features/002-captura-e-acesso/plan.md §2.3).
//
// Escopo: storage.ResolveWriteScope. Escrita sem escopo e sem ponteiro resolvido é recusada como
// escrita ambígua. scope "global" escreve no escopo reservado default/_global. A exclusão usa a mesma
// resolução, sem criar escopo: escopo explícito inexistente falha fechado.
//
// Checkpoint: depois da operação, commit dos caminhos registrados pelo adaptador git (Repo.CommitTracked).
// Falha no commit vira aviso, não erro (docs/mcp-server.md §3.3). O gancho de pré e pós-compactação
// (T030) não é usado aqui.
//
// Identidade: a chave do ator vem de ActorKeyFrom. Nenhum handler lê cabeçalho cru.

const (
	// writeScopeGlobal é o único valor de scope aceito nesta versão.
	writeScopeGlobal = "global"
	// writeGlobalWorkspace e writeGlobalProject são o escopo reservado de busca global (docs/wiki-files-layer.md).
	writeGlobalWorkspace = "default"
	writeGlobalProject   = "_global"
	// writeSessionsDir é o diretório da página de sessão, cujo tier padrão é episodic.
	writeSessionsDir = "sessions/"
	// writeEvidenceID identifica a escrita manual como fonte de evidência. Decisão nossa: o conjunto
	// fechado de SourceKind não tem valor para escrita manual; usa-se reconsolidation, como o reindex.
	writeEvidenceID = "memory_write_page"
	// Rótulos dos commits de checkpoint. Decisão nossa: o doc não fixa rótulo para estas tools.
	labelPageWrite     = "page-write: "
	labelPageDelete    = "page-delete: "
	labelPrePageDelete = "pre-page-delete: "
)

var (
	errWriteAmbiguous  = fmt.Errorf("%w: escrita ambígua, sem escopo resolvido: informe workspace e project, ou scope \"global\"", errToolInput)
	errWriteNeedsNames = fmt.Errorf("%w: informe workspace e project nomeados para gravar", errToolInput)
)

// WriteDeps são as dependências das ferramentas de escrita. Store e Reader são obrigatórios e o Store
// precisa de Repo. Pointer pode ser nil (sem ponteiro de projeto ativo). Now nil usa o relógio atual.
type WriteDeps struct {
	Store   *wiki.Store
	Reader  storage.Reader
	Pointer *storage.ActiveProjectPointer
	Now     func() time.Time
}

// RegisterWriteTools registra memory_write_page e memory_delete_page em srv.
func RegisterWriteTools(srv ToolServer, deps WriteDeps) error {
	if srv == nil {
		return errors.New("mcp: servidor nulo para ferramentas de escrita")
	}
	if deps.Store == nil || deps.Store.Repo == nil {
		return errors.New("mcp: store da wiki sem repositório para ferramentas de escrita")
	}
	if deps.Reader == nil {
		return errors.New("mcp: leitor nulo para ferramentas de escrita")
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if err := srv.AddTool(ToolDef{
		Name: "memory_write_page",
		Description: "Grava uma página da wiki pelo caminho. tier padrão semantic (episodic em sessions/<id>.md). " +
			"scope \"global\" grava no escopo reservado _global; sem scope, usa workspace e project ou o ponteiro " +
			"de projeto ativo. Frontmatter: title, tags, pinned (ausente mantém a fixação anterior; false desafixa), " +
			"expires_at (RFC 3339 ou AAAA-MM-DD). Faz checkpoint depois da escrita.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"path":{"type":"string"},` +
			`"body":{"type":"string"},` +
			`"title":{"type":"string"},` +
			`"tags":{"type":"array","items":{"type":"string"}},` +
			`"pinned":{"type":"boolean"},` +
			`"expires_at":{"type":"string"},` +
			`"tier":{"type":"string","enum":["working","episodic","semantic","procedural"]},` +
			`"scope":{"type":"string","enum":["global"]},` +
			`"workspace":{"type":"string"},` +
			`"project":{"type":"string"}},` +
			`"required":["path","body"]}`),
	}, deps.writePage); err != nil {
		return err
	}
	return srv.AddTool(ToolDef{
		Name: "memory_delete_page",
		Description: "Remove uma página pelo caminho exato. Escopo como em memory_write_page, sem criar escopo. " +
			"Idempotente: página ausente conta como removida. Responde deleted e os checkpoints anterior e posterior.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"path":{"type":"string"},` +
			`"scope":{"type":"string","enum":["global"]},` +
			`"workspace":{"type":"string"},` +
			`"project":{"type":"string"}},` +
			`"required":["path"]}`),
	}, deps.deletePage)
}

// writeScopeArgs são os argumentos de escopo comuns às duas tools. Scope nil significa sem escopo global.
type writeScopeArgs struct {
	Scope     *string `json:"scope"`
	Workspace string  `json:"workspace"`
	Project   string  `json:"project"`
}

type writePageArgs struct {
	writeScopeArgs
	Path      string    `json:"path"`
	Body      *string   `json:"body"`
	Title     *string   `json:"title"`
	Tags      *[]string `json:"tags"`
	Pinned    *bool     `json:"pinned"`
	ExpiresAt *string   `json:"expires_at"`
	Tier      *string   `json:"tier"`
}

type deletePageArgs struct {
	writeScopeArgs
	Path string `json:"path"`
}

type writePageOut struct {
	PageID     string   `json:"page_id"`
	Path       string   `json:"path"`
	Checkpoint string   `json:"checkpoint,omitempty"`
	Avisos     []string `json:"avisos,omitempty"`
}

type deleteOut struct {
	Path          string   `json:"path"`
	Deleted       bool     `json:"deleted"`
	PreCheckpoint string   `json:"pre_checkpoint,omitempty"`
	Checkpoint    string   `json:"checkpoint,omitempty"`
	Avisos        []string `json:"avisos,omitempty"`
}

// writeTarget é o escopo de uma escrita ou exclusão. Workspace e Project são os nomes que o
// WritePage exige; ficam vazios quando o escopo vem do ponteiro, porque o resolvedor não devolve nomes.
type writeTarget struct {
	scope     domain.Scope
	workspace string
	project   string
}

// writePage grava uma página. Ordem: entrada, escopo, tier, frontmatter, escrita, checkpoint.
func (d WriteDeps) writePage(ctx context.Context, args json.RawMessage) (ToolResult, error) {
	var in writePageArgs
	if err := decodeToolArgs(args, &in); err != nil {
		return writeFailure("gravar", err)
	}
	pp, err := domain.ParsePagePath(in.Path)
	if err != nil {
		return writeFailure("gravar", fmt.Errorf("%w: caminho de página inválido", errToolInput))
	}
	if in.Body == nil || *in.Body == "" {
		return writeFailure("gravar", fmt.Errorf("%w: body é obrigatório", errToolInput))
	}
	tier, err := writeTierFor(in.Tier, pp)
	if err != nil {
		return writeFailure("gravar", err)
	}
	fm, err := writeFrontmatter(in)
	if err != nil {
		return writeFailure("gravar", err)
	}
	target, err := d.resolveTarget(ctx, in.writeScopeArgs, true)
	if err != nil {
		return writeFailure("gravar", err)
	}
	if target.workspace == "" || target.project == "" {
		return writeFailure("gravar", errWriteNeedsNames)
	}

	res, err := d.Store.WritePage(ctx, wiki.PageInput{
		Scope:         target.scope,
		WorkspaceName: target.workspace,
		ProjectName:   target.project,
		Path:          pp,
		Tier:          tier,
		Body:          *in.Body,
		Frontmatter:   fm,
		Evidence:      storage.EvidenceSource{Kind: storage.SourceReconsolidation, ID: writeEvidenceID},
	})
	if err != nil {
		return writeFailure("gravar", err)
	}
	avisos := res.Avisos
	cp, avisos := d.checkpoint(ctx, labelPageWrite+res.Path, avisos)
	return jsonResult(writePageOut{
		PageID:     res.Version.String(),
		Path:       res.Path,
		Checkpoint: cp,
		Avisos:     avisos,
	})
}

// deletePage remove a versão corrente de uma página pelo caminho exato. Sem versão corrente, responde
// deleted true sem commit (remoção idempotente, docs/architecture.md: "Idempotent").
func (d WriteDeps) deletePage(ctx context.Context, args json.RawMessage) (ToolResult, error) {
	var in deletePageArgs
	if err := decodeToolArgs(args, &in); err != nil {
		return writeFailure("remover", err)
	}
	pp, err := domain.ParsePagePath(in.Path)
	if err != nil {
		return writeFailure("remover", fmt.Errorf("%w: caminho de página inválido", errToolInput))
	}
	target, err := d.resolveTarget(ctx, in.writeScopeArgs, false)
	if err != nil {
		return writeFailure("remover", err)
	}
	ref := domain.NewPageRef(target.scope, pp)
	current, err := storage.ReadPage(ctx, d.Reader, ref)
	if errors.Is(err, storage.ErrPageNotFound) {
		return jsonResult(deleteOut{Path: pp.String(), Deleted: true})
	}
	if err != nil {
		return writeFailure("remover", err)
	}

	pre, avisos := d.checkpoint(ctx, labelPrePageDelete+ref.String(), nil)
	res, err := d.Store.DeletePage(ctx, target.scope, pp, current.Version)
	if err != nil {
		return writeFailure("remover", err)
	}
	avisos = append(avisos, res.Avisos...)
	out := deleteOut{Path: pp.String(), Deleted: res.Deleted, PreCheckpoint: pre}
	if !res.Deleted {
		// Versão corrente mudou entre a leitura e a remoção: nada foi removido.
		avisos = append(avisos, "versão corrente mudou antes da remoção; nada removido")
	} else {
		out.Checkpoint, avisos = d.checkpoint(ctx, labelPageDelete+ref.String(), avisos)
	}
	out.Avisos = avisos
	return jsonResult(out)
}

// resolveTarget resolve o escopo de escrita (create verdadeiro) ou de exclusão (create falso).
// Só a escrita pode criar escopo; a exclusão falha fechado quando o escopo nomeado não existe.
func (d WriteDeps) resolveTarget(ctx context.Context, sa writeScopeArgs, create bool) (writeTarget, error) {
	if sa.Scope != nil {
		return d.resolveGlobal(ctx, sa, create)
	}
	key, _ := ActorKeyFrom(ctx)
	res, err := storage.ResolveWriteScope(ctx, d.Reader, storage.ScopeRequest{
		WorkspaceName: sa.Workspace,
		ProjectName:   sa.Project,
		Pointer:       d.Pointer,
		Actor:         storage.ActorKey{Identity: key.User, SessionID: key.SessionID},
		Now:           d.Now(),
		Create:        create,
	})
	if err != nil {
		return writeTarget{}, mapWriteScopeErr(err)
	}
	if res.Exists {
		return writeTarget{scope: res.Scope, workspace: sa.Workspace, project: sa.Project}, nil
	}
	// Alvo novo: só a escrita chega aqui (create verdadeiro). Só o par nomeado cria; projeto sem
	// workspace nomeado não tem como criar sem o ID do workspace existente.
	if sa.Workspace == "" || sa.Project == "" {
		return writeTarget{}, errWriteNeedsNames
	}
	return newWriteTarget(sa.Workspace, sa.Project)
}

// resolveGlobal resolve scope "global" para default/_global. Criação só na escrita.
func (d WriteDeps) resolveGlobal(ctx context.Context, sa writeScopeArgs, create bool) (writeTarget, error) {
	if *sa.Scope != writeScopeGlobal {
		return writeTarget{}, fmt.Errorf("%w: scope aceita apenas \"global\" nesta versão", errToolInput)
	}
	if sa.Workspace != "" || sa.Project != "" {
		return writeTarget{}, fmt.Errorf("%w: scope \"global\" não combina com workspace nem project", errToolInput)
	}
	s, err := storage.ResolveGlobalScope(ctx, d.Reader)
	if err == nil {
		return writeTarget{scope: s, workspace: writeGlobalWorkspace, project: writeGlobalProject}, nil
	}
	if !create || !errors.Is(err, storage.ErrScopeNotFound) {
		return writeTarget{}, mapWriteScopeErr(err)
	}
	return newWriteTarget(writeGlobalWorkspace, writeGlobalProject)
}

// newWriteTarget monta um escopo novo com IDs UUIDv7. O WritePage cria o escopo com EnsureScope.
func newWriteTarget(workspace, project string) (writeTarget, error) {
	ws, err := domain.NewID()
	if err != nil {
		return writeTarget{}, fmt.Errorf("gerar id de workspace: %w", err)
	}
	pj, err := domain.NewID()
	if err != nil {
		return writeTarget{}, fmt.Errorf("gerar id de projeto: %w", err)
	}
	return writeTarget{
		scope:     domain.NewScope(domain.WorkspaceID(ws), domain.ProjectID(pj)),
		workspace: workspace,
		project:   project,
	}, nil
}

// mapWriteScopeErr traduz erros do resolvedor de escrita para classes da ferramenta, sem eco de nomes.
func mapWriteScopeErr(err error) error {
	switch {
	case errors.Is(err, storage.ErrScopeRequired):
		return errWriteAmbiguous
	case errors.Is(err, storage.ErrScopeMismatch):
		return fmt.Errorf("%w: projeto ativo diverge do chamador; informe workspace e project", errToolInput)
	case errors.Is(err, storage.ErrScopeInvalid), errors.Is(err, storage.ErrScopeAmbiguous):
		return fmt.Errorf("%w: escopo inválido", errToolInput)
	case errors.Is(err, storage.ErrScopeNotFound):
		return fmt.Errorf("%w: escopo não existe", errToolNotFound)
	}
	return err
}

// writeTierFor devolve o tier pedido ou o padrão: episodic na página de sessão, semantic nas demais.
func writeTierFor(raw *string, pp domain.PagePath) (domain.Tier, error) {
	if raw != nil {
		t, err := domain.ParseTier(*raw)
		if err != nil {
			return "", fmt.Errorf("%w: tier inválido", errToolInput)
		}
		return t, nil
	}
	if isSessionPagePath(pp.String()) {
		return domain.TierEpisodic, nil
	}
	return domain.TierSemantic, nil
}

// isSessionPagePath diz se o caminho é sessions/<id>.md (um nível, sem subdiretório).
func isSessionPagePath(p string) bool {
	rest, ok := strings.CutPrefix(p, writeSessionsDir)
	return ok && rest != "" && !strings.Contains(rest, "/")
}

// writeFrontmatter monta o frontmatter aceito: title, tags, pinned e expires_at. Campo ausente não é
// gravado; a fixação ausente é herdada pela wiki (inheritPinned). pinned vai como texto "true"/"false",
// porque a wiki lê a fixação como texto (decisão 001/T084).
func writeFrontmatter(in writePageArgs) (domain.Frontmatter, error) {
	var fm domain.Frontmatter
	set := func(key string, value any) error {
		if err := fm.Set(key, value); err != nil {
			return fmt.Errorf("frontmatter %s: %w", key, err)
		}
		return nil
	}
	if in.Title != nil {
		if err := set("title", *in.Title); err != nil {
			return domain.Frontmatter{}, err
		}
	}
	if in.Tags != nil {
		if err := set("tags", *in.Tags); err != nil {
			return domain.Frontmatter{}, err
		}
	}
	if in.Pinned != nil {
		if err := set("pinned", strconv.FormatBool(*in.Pinned)); err != nil {
			return domain.Frontmatter{}, err
		}
	}
	if in.ExpiresAt != nil {
		if _, err := wiki.ParseExpiresAt(*in.ExpiresAt); err != nil {
			return domain.Frontmatter{}, fmt.Errorf("%w: expires_at deve ser RFC 3339 ou AAAA-MM-DD", errToolInput)
		}
		if err := set("expires_at", *in.ExpiresAt); err != nil {
			return domain.Frontmatter{}, err
		}
	}
	return fm, nil
}

// checkpoint faz o commit dos caminhos registrados depois de uma operação. Falha vira aviso. Sem
// mudança a commitar, devolve "" sem aviso.
func (d WriteDeps) checkpoint(ctx context.Context, label string, avisos []string) (string, []string) {
	sha, err := d.Store.Repo.CommitTracked(ctx, label)
	if err != nil {
		slog.Warn("checkpoint de escrita MCP não feito", "label", label, "err", err)
		return "", append(avisos, "checkpoint não feito: "+label)
	}
	return sha, avisos
}

// writeFailure traduz erros de escrita em resultado de ferramenta. Não repete nomes, caminhos nem SQL:
// a causa detalhada só vai para o log do servidor.
func writeFailure(op string, err error) (ToolResult, error) {
	switch {
	case errors.Is(err, errToolInput), errors.Is(err, errToolNotFound):
		return failure(err.Error())
	case errors.Is(err, wiki.ErrInvalidFrontmatter):
		return failure(err.Error())
	case errors.Is(err, wiki.ErrInconsistentState):
		slog.Error("estado inconsistente em ferramenta de escrita MCP", "op", op, "err", err)
		return failure("falha de consistência: arquivo e índice podem divergir; rode reindex")
	}
	slog.Error("falha em ferramenta de escrita MCP", "op", op, "err", err)
	return failure("falha ao " + op + " a página")
}
