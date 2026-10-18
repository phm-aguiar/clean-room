package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// drainReq guarda o que o servidor fake recebeu em uma chamada de /hook/batch.
type drainReq struct {
	path  string
	auth  string
	items []batchEntry
}

// drainServidor é um servidor fake de /hook/batch. O handler decide a resposta de cada
// chamada, na ordem em que chegam.
type drainServidor struct {
	mu       sync.Mutex
	chamadas []drainReq
	srv      *httptest.Server
}

func novoDrainServidor(t *testing.T, responde func(n int, req drainReq) (int, string)) *drainServidor {
	t.Helper()
	ds := &drainServidor{}
	ds.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		corpo, _ := io.ReadAll(r.Body)
		var itens []batchEntry
		if err := json.Unmarshal(corpo, &itens); err != nil {
			t.Errorf("corpo do lote não é array JSON: %v", err)
		}
		req := drainReq{path: r.URL.Path, auth: r.Header.Get("Authorization"), items: itens}
		ds.mu.Lock()
		ds.chamadas = append(ds.chamadas, req)
		n := len(ds.chamadas)
		ds.mu.Unlock()
		status, resp := responde(n, req)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, resp)
	}))
	t.Cleanup(ds.srv.Close)
	return ds
}

func (ds *drainServidor) lista() []drainReq {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	return append([]drainReq(nil), ds.chamadas...)
}

// novoSpoolDrain cria um spool temporário.
func novoSpoolDrain(t *testing.T) *hookSpool {
	t.Helper()
	sp, err := openHookSpool(t.TempDir())
	if err != nil {
		t.Fatalf("abrir spool: %v", err)
	}
	return sp
}

// enfileirarDrain grava um item com endereço do servidor antigo (127.0.0.1:1) e a chave dada.
func enfileirarDrain(t *testing.T, sp *hookSpool, authMode, token, chave string) {
	t.Helper()
	url := "http://127.0.0.1:1/hook?event=PostToolUse&ingest_key=" + chave
	if _, err := sp.Enqueue(url, chave, authMode, token, []byte(`{"session_id":"s-`+chave+`"}`)); err != nil {
		t.Fatalf("enfileirar %s: %v", chave, err)
	}
}

// restantes devolve os itens que ainda estão no spool, com as tentativas.
func restantes(t *testing.T, sp *hookSpool) []spoolItem {
	t.Helper()
	lote, err := sp.Batch()
	if err != nil {
		t.Fatalf("ler spool: %v", err)
	}
	return lote
}

// ackJSON monta um corpo de reconhecimento.
func ackJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("serializar ack: %v", err)
	}
	return string(b)
}

func TestDrenarConfirmacaoPorIndice(t *testing.T) {
	ds := novoDrainServidor(t, func(int, drainReq) (int, string) {
		return http.StatusOK, `{"accepted":0,"accepted_indices":[0,2]}`
	})
	sp := novoSpoolDrain(t)
	enfileirarDrain(t, sp, spoolAuthNone, "", "k1")
	enfileirarDrain(t, sp, spoolAuthNone, "", "k2")
	enfileirarDrain(t, sp, spoolAuthNone, "", "k3")

	res, err := drenarSpool(context.Background(), sp, drainTarget{serverURL: ds.srv.URL})
	if err != nil {
		t.Fatalf("drenar: %v", err)
	}
	if res.Acked != 2 || res.Failed != 0 || res.Parado != "" {
		t.Fatalf("resultado = %+v; quer Acked=2 Failed=0 sem parada", res)
	}
	left := restantes(t, sp)
	if len(left) != 1 || left[0].IngestKey != "k2" {
		t.Fatalf("restantes = %+v; quer só k2 (o índice 1 não foi confirmado)", left)
	}
	if left[0].Attempts != 0 {
		t.Fatalf("item não reconhecido ganhou tentativa (%d); quer 0", left[0].Attempts)
	}
}

func TestDrenarFailedIndexInterrompeLote(t *testing.T) {
	ds := novoDrainServidor(t, func(int, drainReq) (int, string) {
		return http.StatusOK, `{"accepted":1,"failed_index":1}`
	})
	sp := novoSpoolDrain(t)
	enfileirarDrain(t, sp, spoolAuthNone, "", "k1")
	enfileirarDrain(t, sp, spoolAuthNone, "", "k2")
	enfileirarDrain(t, sp, spoolAuthNone, "", "k3")

	res, err := drenarSpool(context.Background(), sp, drainTarget{serverURL: ds.srv.URL})
	if err != nil {
		t.Fatalf("drenar: %v", err)
	}
	if res.Acked != 1 || res.Failed != 1 || res.Parado == "" {
		t.Fatalf("resultado = %+v; quer Acked=1 Failed=1 e parada", res)
	}
	if n := len(ds.lista()); n != 1 {
		t.Fatalf("chamadas = %d; quer 1 (failed_index interrompe o lote)", n)
	}
	left := restantes(t, sp)
	if len(left) != 2 {
		t.Fatalf("restantes = %d; quer 2", len(left))
	}
	if left[0].IngestKey != "k2" || left[0].Attempts != 1 {
		t.Fatalf("item de failed_index = %s com %d tentativas; quer k2 com 1", left[0].IngestKey, left[0].Attempts)
	}
	if left[1].IngestKey != "k3" || left[1].Attempts != 0 {
		t.Fatalf("item depois do failed_index = %s com %d tentativas; quer k3 com 0", left[1].IngestKey, left[1].Attempts)
	}
}

func TestDrenar401RetryUmaVezComTokenAtual(t *testing.T) {
	t.Run("retry com token atual confirma", func(t *testing.T) {
		ds := novoDrainServidor(t, func(n int, req drainReq) (int, string) {
			if req.auth == "Bearer tok-velho" {
				return http.StatusUnauthorized, `{"error":"unauthorized"}`
			}
			return http.StatusOK, ackJSON(t, map[string]any{"accepted": 1})
		})
		sp := novoSpoolDrain(t)
		enfileirarDrain(t, sp, spoolAuthStatic, "tok-velho", "k1")

		res, err := drenarSpool(context.Background(), sp, drainTarget{
			serverURL: ds.srv.URL,
			token:     func() string { return "tok-novo" },
		})
		if err != nil {
			t.Fatalf("drenar: %v", err)
		}
		chamadas := ds.lista()
		if len(chamadas) != 2 {
			t.Fatalf("chamadas = %d; quer 2 (envio + um retry)", len(chamadas))
		}
		if chamadas[0].auth != "Bearer tok-velho" {
			t.Fatalf("primeiro envio com %q; quer o token guardado no item", chamadas[0].auth)
		}
		if chamadas[1].auth != "Bearer tok-novo" {
			t.Fatalf("retry com %q; quer o token atual do config", chamadas[1].auth)
		}
		if res.Acked != 1 || res.Failed != 0 {
			t.Fatalf("resultado = %+v; quer Acked=1 Failed=0", res)
		}
		if left := restantes(t, sp); len(left) != 0 {
			t.Fatalf("spool ainda tem %d itens após confirmação", len(left))
		}
	})

	t.Run("401 persistente repete uma vez e gasta tentativa", func(t *testing.T) {
		ds := novoDrainServidor(t, func(int, drainReq) (int, string) {
			return http.StatusUnauthorized, `{"error":"unauthorized"}`
		})
		sp := novoSpoolDrain(t)
		enfileirarDrain(t, sp, spoolAuthStatic, "tok-velho", "k1")

		res, err := drenarSpool(context.Background(), sp, drainTarget{
			serverURL: ds.srv.URL,
			token:     func() string { return "tok-novo" },
		})
		if err != nil {
			t.Fatalf("drenar: %v", err)
		}
		if n := len(ds.lista()); n != 2 {
			t.Fatalf("chamadas = %d; quer exatamente 2 (um retry só)", n)
		}
		if res.Parado == "" || res.Failed != 1 {
			t.Fatalf("resultado = %+v; quer parada e Failed=1", res)
		}
		left := restantes(t, sp)
		if len(left) != 1 || left[0].Attempts != 1 {
			t.Fatalf("restantes = %+v; quer 1 item com 1 tentativa", left)
		}
	})
}

func TestDrenar429NaoGastaTentativa(t *testing.T) {
	ds := novoDrainServidor(t, func(int, drainReq) (int, string) {
		return http.StatusTooManyRequests, `{"error":"hook queue full"}`
	})
	sp := novoSpoolDrain(t)
	enfileirarDrain(t, sp, spoolAuthNone, "", "k1")
	enfileirarDrain(t, sp, spoolAuthNone, "", "k2")

	res, err := drenarSpool(context.Background(), sp, drainTarget{serverURL: ds.srv.URL})
	if err != nil {
		t.Fatalf("drenar: %v", err)
	}
	if res.Failed != 0 || res.Acked != 0 || res.Parado == "" {
		t.Fatalf("resultado = %+v; quer Failed=0 Acked=0 e parada", res)
	}
	if n := len(ds.lista()); n != 1 {
		t.Fatalf("chamadas = %d; quer 1 (a passagem para no 429)", n)
	}
	left := restantes(t, sp)
	if len(left) != 2 {
		t.Fatalf("restantes = %d; quer 2 sem perda", len(left))
	}
	for _, it := range left {
		if it.Attempts != 0 {
			t.Fatalf("item %s com %d tentativas após 429; quer 0", it.IngestKey, it.Attempts)
		}
	}
}

func TestDrenarReapontaParaServerURLAtual(t *testing.T) {
	ds := novoDrainServidor(t, func(int, drainReq) (int, string) {
		return http.StatusOK, ackJSON(t, map[string]any{"accepted": 1})
	})
	sp := novoSpoolDrain(t)
	enfileirarDrain(t, sp, spoolAuthStatic, "tok", "k1")

	if _, err := drenarSpool(context.Background(), sp, drainTarget{
		serverURL: ds.srv.URL,
		token:     func() string { return "tok" },
	}); err != nil {
		t.Fatalf("drenar: %v", err)
	}
	chamadas := ds.lista()
	if len(chamadas) != 1 {
		t.Fatalf("chamadas = %d; quer 1", len(chamadas))
	}
	if chamadas[0].path != drainBatchPath {
		t.Fatalf("caminho = %q; quer %q", chamadas[0].path, drainBatchPath)
	}
	quer := strings.TrimSuffix(ds.srv.URL, "/") + "/hook?event=PostToolUse&ingest_key=k1"
	if len(chamadas[0].items) != 1 || chamadas[0].items[0].URL != quer {
		t.Fatalf("url enviada = %+v; quer %q (host do server_url atual, query preservada)", chamadas[0].items, quer)
	}
}

func TestDrenarAuthNoneSemCabecalho(t *testing.T) {
	ds := novoDrainServidor(t, func(int, drainReq) (int, string) {
		return http.StatusOK, ackJSON(t, map[string]any{"accepted": 1})
	})
	sp := novoSpoolDrain(t)
	enfileirarDrain(t, sp, spoolAuthNone, "", "k1")

	if _, err := drenarSpool(context.Background(), sp, drainTarget{serverURL: ds.srv.URL}); err != nil {
		t.Fatalf("drenar: %v", err)
	}
	chamadas := ds.lista()
	if len(chamadas) != 1 || chamadas[0].auth != "" {
		t.Fatalf("chamadas = %+v; quer 1 sem Authorization no modo none", chamadas)
	}
}

func TestDrenarDescartaAuthModeDesconhecido(t *testing.T) {
	ds := novoDrainServidor(t, func(int, drainReq) (int, string) {
		return http.StatusOK, ackJSON(t, map[string]any{"accepted": 1})
	})
	sp := novoSpoolDrain(t)
	// Escrita direta: Enqueue recusa auth_mode fora de static e none.
	ilegivel, err := json.Marshal(spoolItem{
		URL:       "http://127.0.0.1:1/hook?ingest_key=oidc",
		Body:      json.RawMessage(`{}`),
		IngestKey: "oidc",
		AuthMode:  "oidc",
		CreatedMS: sp.now().UnixMilli(),
	})
	if err != nil {
		t.Fatalf("serializar: %v", err)
	}
	nome, err := spoolFileName(sp.now())
	if err != nil {
		t.Fatalf("nome: %v", err)
	}
	if err := writeFileAtomic(filepath.Join(sp.dir, nome), ilegivel, spoolFileMode); err != nil {
		t.Fatalf("gravar item oidc: %v", err)
	}
	enfileirarDrain(t, sp, spoolAuthNone, "", "k1")

	res, err := drenarSpool(context.Background(), sp, drainTarget{serverURL: ds.srv.URL})
	if err != nil {
		t.Fatalf("drenar: %v", err)
	}
	if res.Descartados != 1 || res.Acked != 1 {
		t.Fatalf("resultado = %+v; quer Descartados=1 Acked=1", res)
	}
	for _, c := range ds.lista() {
		for _, it := range c.items {
			if strings.Contains(it.URL, "ingest_key=oidc") {
				t.Fatalf("item oidc foi enviado: %s", it.URL)
			}
		}
	}
	if left := restantes(t, sp); len(left) != 0 {
		t.Fatalf("spool ainda tem %d itens", len(left))
	}
}
