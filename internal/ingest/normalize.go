// Package ingest contém a normalização pura dos eventos de hook (LEI §3 e §5).
//
// Função pura: não lê arquivo, não lê variável de ambiente e não faz I/O. O relógio
// entra por parâmetro (Input.Now). Fontes: docs/lifecycle-event-ingestion.md (LEI) §3 e §5;
// specs/features/002-captura-e-acesso/notes/hook-contract.md (T001) e phase0-findings.md §2.
package ingest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/phm-aguiar/ai-memory-go/internal/domain"
)

// Limites e constantes de formato (LEI §3.3; phase0-findings L8).
const (
	maxIngestKeyLen     = 64  // LEI §3.3: de 1 a 64 caracteres.
	maxToolCallIDBytes  = 512 // L8: limite de 512 bytes, herdado da allowlist de metadados da API.
	maxFutureSkew       = 5 * time.Minute
	noOutputPlaceholder = "(no output captured)" // LEI §3.5.
	outcomeUnknown      = "unknown"              // LEI §3.5: desfecho sempre unknown para Claude Code.
	toolOutputSeparator = "---"                  // LEI §3.5: separador entre desfecho e saída.
)

// ToolFamily é a família da ferramenta no esquema fechado (LEI §3.5).
type ToolFamily string

// Famílias de ferramenta (LEI §3.5).
const (
	ToolFamilyFile       ToolFamily = "file"
	ToolFamilySearchList ToolFamily = "search-list"
	ToolFamilyNonFile    ToolFamily = "non-file"
	ToolFamilyUnknown    ToolFamily = "unknown"
)

// toolFamilyByName mapeia o nome da ferramenta do Claude Code para a família (phase0-findings L6).
//
// HIPÓTESE DE IMPLEMENTAÇÃO, a revisar contra o Claude Code real: a LEI §3.5 não lista nomes.
// Nomes exatos (sensíveis a caixa). Nome ausente da tabela vira ToolFamilyUnknown.
var toolFamilyByName = map[string]ToolFamily{
	// Arquivo.
	"Read":         ToolFamilyFile,
	"Edit":         ToolFamilyFile,
	"Write":        ToolFamilyFile,
	"MultiEdit":    ToolFamilyFile,
	"NotebookEdit": ToolFamilyFile,
	// Busca e listagem.
	"Grep": ToolFamilySearchList,
	"Glob": ToolFamilySearchList,
	"LS":   ToolFamilySearchList,
	// Demais conhecidos (shell e outras ferramentas), hipótese a revisar.
	"Bash":         ToolFamilyNonFile,
	"BashOutput":   ToolFamilyNonFile,
	"KillBash":     ToolFamilyNonFile,
	"KillShell":    ToolFamilyNonFile,
	"Task":         ToolFamilyNonFile,
	"TodoWrite":    ToolFamilyNonFile,
	"WebFetch":     ToolFamilyNonFile,
	"WebSearch":    ToolFamilyNonFile,
	"ExitPlanMode": ToolFamilyNonFile,
}

// Input são os parâmetros de consulta do hook e o instante de referência. O corpo do hook
// chega à parte, em Normalize.
type Input struct {
	Agent     string    // parâmetro agent da consulta (LEI §2.1).
	Event     string    // parâmetro event da consulta; qualquer forma aceita por ParseHookEvent.
	Cwd       string    // parâmetro cwd da consulta; fallback quando o corpo não traz (LEI §3.3).
	SessionID string    // parâmetro session_id da consulta; fallback quando o corpo não traz.
	Now       time.Time // relógio do servidor, usado em occurred_at (LEI §3.3).
}

// Event é o evento normalizado. Não contém last_assistant_message em nenhum campo.
//
// Title e Body são o texto bruto do evento, antes da sanitização e dos limites de corpo
// (LEI §3.6), que são aplicados em etapas posteriores.
type Event struct {
	Agent      string
	Hook       domain.HookEvent
	Session    domain.SessionID
	HasSession bool
	Cwd        string
	Title      string
	Body       string
	OccurredAt time.Time
	IngestKey  string // vazio quando ausente ou inválida (LEI §3.3).
	IsSubagent bool   // subagentType ou agent_id não vazio no topo do corpo (LEI §3.3).

	// Campos de ferramenta (somente pré e pós-ferramenta).
	ToolName   string
	ToolFamily ToolFamily
	ToolCallID string // vazio quando ausente ou inválido (L8).

	// ToolInput é o valor bruto de tool_input, sem extração. A política de captura é outra etapa
	// e lê daqui. Nunca entra em Title nem em Body (LEI §3.3).
	ToolInput json.RawMessage
}

// Normalize converte o corpo JSON do hook em Event (LEI §3.3 e §3.5).
//
// O parsing é tolerante: nenhum campo é obrigatório. A obrigatoriedade do identificador de
// sessão é aplicada depois, por quem chama (Event.HasSession). Erro só quando o evento de
// hook é vazio ou o corpo não é um objeto JSON. Ambos encapsulam domain.ErrMalformedRecord.
//
// A remoção de last_assistant_message (LEI §5) é o primeiro passo, incondicional.
func Normalize(in Input, body []byte) (Event, error) {
	hook, err := domain.ParseHookEvent(in.Event)
	if err != nil {
		return Event{}, err
	}
	fields, err := decodeObject(body)
	if err != nil {
		return Event{}, err
	}
	// LEI §5: remoção incondicional do campo bruto, antes de qualquer outra etapa.
	delete(fields, "last_assistant_message")

	ev := Event{
		Agent:      strings.TrimSpace(in.Agent),
		Hook:       hook,
		OccurredAt: in.Now,
	}

	// Identificador de sessão: corpo primeiro, depois a consulta (LEI §3.3).
	sessionSrc, ok := firstString(fields, "session_id", "sessionId", "sessionID", "session", "conversationId")
	if !ok {
		sessionSrc = in.SessionID
	}
	if strings.TrimSpace(sessionSrc) != "" {
		sid, err := domain.SessionIDFromNative(sessionSrc)
		if err != nil {
			return Event{}, err
		}
		ev.Session = sid
		ev.HasSession = true
	}

	// Diretório: o valor do corpo prevalece sobre o da consulta (LEI §3.3).
	cwd, ok := firstString(fields, "cwd", "current_dir", "working_dir", "directory")
	if !ok {
		cwd, ok = firstListHead(fields, "workspacePaths", "workspace_roots")
	}
	if !ok {
		cwd = in.Cwd
	}
	ev.Cwd = cwd

	// occurred_at: só do topo do corpo, positivo e no máximo 5 minutos à frente; senão agora.
	// Formato RFC 3339 (docs/architecture.md, "Client-supplied event time"; docs/harness-adapters.md).
	if s, ok := fieldString(fields, "occurred_at"); ok {
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil &&
			t.After(time.Unix(0, 0)) && !t.After(in.Now.Add(maxFutureSkew)) {
			ev.OccurredAt = t
		}
	}

	// ingest_key: inválida é ignorada, nunca gera erro (LEI §3.3).
	if s, ok := fieldString(fields, "ingest_key"); ok && validIngestKey(s) {
		ev.IngestKey = s
	}

	// Subagente: subagentType ou agent_id como string não vazia (LEI §3.3).
	_, ev.IsSubagent = firstString(fields, "subagentType", "agent_id")

	// Título e corpo por evento (LEI §3.4 e §3.5).
	switch hook {
	case domain.HookSessionStart:
		ev.Title = textOf(fields, "model", "title")
	case domain.HookUserPrompt:
		text := textOf(fields, "prompt", "message", "text")
		ev.Title, ev.Body = text, text
	case domain.HookPreToolUse:
		closedSchemaToolEvent(&ev, fields, false)
	case domain.HookPostToolUse:
		closedSchemaToolEvent(&ev, fields, true)
	}
	// Demais eventos (parada, pré-compactação, fim de sessão, subagentes): sem título nem corpo.

	return ev, nil
}

// closedSchemaToolEvent preenche título, corpo e campos de ferramenta de pré e pós-ferramenta
// no esquema fechado (LEI §3.5). Argumentos (tool_input) nunca entram em título nem corpo.
func closedSchemaToolEvent(ev *Event, fields map[string]json.RawMessage, post bool) {
	ev.ToolName = strings.TrimSpace(stringOrEmpty(fields, "tool_name"))
	family := familyOf(ev.ToolName)
	ev.ToolFamily = family
	ev.ToolCallID = validToolCallID(stringOrEmpty(fields, "tool_use_id"))
	if raw, ok := fields["tool_input"]; ok {
		ev.ToolInput = append(json.RawMessage(nil), raw...)
	}

	ev.Title = "tool " + string(family)
	lines := []string{"tool_family: " + string(family)}
	if ev.ToolCallID != "" {
		lines = append(lines, "tool_call_id: "+ev.ToolCallID)
	}
	if post {
		lines = append(lines, "outcome: "+outcomeUnknown)
		if family != ToolFamilyUnknown {
			lines = append(lines, toolOutputSeparator, toolOutput(fields))
		}
	}
	ev.Body = strings.Join(lines, "\n")
}

// familyOf devolve a família do nome de ferramenta pela tabela toolFamilyByName.
func familyOf(name string) ToolFamily {
	if f, ok := toolFamilyByName[name]; ok {
		return f
	}
	return ToolFamilyUnknown
}

// toolOutput escolhe a saída do pós-ferramenta: tool_response (string, objeto ou lista, como
// JSON compacto), depois tool_output, output, result e error. Sem saída, o placeholder (LEI §3.5).
// Decisão L9 da fase 0: tool_response vem primeiro, para casar com o caminho genérico.
func toolOutput(fields map[string]json.RawMessage) string {
	for _, key := range []string{"tool_response", "tool_output", "output", "result", "error"} {
		if s, ok := renderOutput(fields[key]); ok {
			return s
		}
	}
	return noOutputPlaceholder
}

// renderOutput converte um valor de saída em texto. Ausente, null, string vazia e string só com
// espaços não valem. Objeto e lista viram JSON compacto.
func renderOutput(raw json.RawMessage) (string, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return "", false
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil || strings.TrimSpace(s) == "" {
			return "", false
		}
		return strings.TrimSpace(s), true
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, trimmed); err != nil {
		return "", false
	}
	return buf.String(), true
}

// validToolCallID aceita o identificador da chamada se não for vazio, tiver até 512 bytes e
// não tiver caracteres de controle. Caso contrário devolve vazio e a chamada segue sem ele (L8).
func validToolCallID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > maxToolCallIDBytes {
		return ""
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return ""
		}
	}
	return s
}

// validIngestKey aceita de 1 a 64 caracteres entre letras ASCII, dígitos, '-' e '_' (LEI §3.3).
func validIngestKey(s string) bool {
	if len(s) < 1 || len(s) > maxIngestKeyLen {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}

// decodeObject lê o corpo como objeto JSON. Corpo que não é objeto é erro de registro malformado.
func decodeObject(body []byte) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, fmt.Errorf("%w: corpo não é objeto JSON: %v", domain.ErrMalformedRecord, err)
	}
	if fields == nil {
		return nil, fmt.Errorf("%w: corpo é null, não objeto", domain.ErrMalformedRecord)
	}
	return fields, nil
}

// fieldString devolve o valor da chave quando é string JSON. O valor é devolvido como está.
func fieldString(fields map[string]json.RawMessage, key string) (string, bool) {
	raw, ok := fields[key]
	if !ok {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}

// stringOrEmpty devolve o valor da chave como string, ou vazio quando ausente ou não string.
func stringOrEmpty(fields map[string]json.RawMessage, key string) string {
	s, _ := fieldString(fields, key)
	return s
}

// firstString devolve a primeira chave cujo valor é string não vazia, sem trim, na ordem dada.
func firstString(fields map[string]json.RawMessage, keys ...string) (string, bool) {
	for _, key := range keys {
		if s, ok := fieldString(fields, key); ok && strings.TrimSpace(s) != "" {
			return s, true
		}
	}
	return "", false
}

// firstListHead devolve o primeiro item não vazio de uma lista de strings (workspacePaths e
// workspace_roots, LEI §3.3), na ordem das chaves.
func firstListHead(fields map[string]json.RawMessage, keys ...string) (string, bool) {
	for _, key := range keys {
		raw, ok := fields[key]
		if !ok {
			continue
		}
		var items []string
		if err := json.Unmarshal(raw, &items); err != nil {
			continue
		}
		for _, item := range items {
			if strings.TrimSpace(item) != "" {
				return item, true
			}
		}
	}
	return "", false
}

// textOf devolve o texto da primeira chave com conteúdo não vazio (LEI §3.4). Aceita string e
// lista de blocos [{type:"text", text:"..."}]; os blocos de texto são unidos por quebra de linha.
func textOf(fields map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		raw, ok := fields[key]
		if !ok {
			continue
		}
		if s, ok := fieldString(fields, key); ok {
			if t := strings.TrimSpace(s); t != "" {
				return t
			}
			continue
		}
		var blocks []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(raw, &blocks); err != nil {
			continue
		}
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
				parts = append(parts, b.Text)
			}
		}
		if t := strings.TrimSpace(strings.Join(parts, "\n")); t != "" {
			return t
		}
	}
	return ""
}
