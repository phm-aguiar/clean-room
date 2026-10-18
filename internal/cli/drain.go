// Drenagem do spool local por POST /hook/batch (docs/cli.md §5.3; docs/lifecycle-event-ingestion.md
// §2.2; docs/complementary-contracts.md §2; plan da 002, Q-09).
//
// Cada passagem pega um lote do spool (Batch), envia ao servidor atual e aplica o
// reconhecimento: itens confirmados saem da fila (Ack); o item de failed_index ganha uma
// tentativa (Fail) e interrompe o lote; itens não reconhecidos sem failed_index ficam para a
// próxima passagem, sem tentativa. 401 repete uma vez com o token atual do config. 429 não
// gasta tentativa. Qualquer falha de transporte ou status fora de 2xx/401/429 gasta uma
// tentativa dos itens do lote e encerra a passagem.
//
// A trava da fila não é tomada aqui: quem chama (hook-drain, T047) toma TryLock antes.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// drainBatchTimeout é o tempo máximo de um POST /hook/batch. A confirmar, INV-05 da
// constituição: docs/cli.md:363 deixa o valor marcado como "a confirmar".
const drainBatchTimeout = 2 * time.Second

const (
	drainBatchPath     = "/hook/batch"
	drainAckMaxBytes   = 1 << 20
	drainStatusTooMany = http.StatusTooManyRequests
)

// drainTarget é o destino atual da drenagem. serverURL é o server_url do config vigente;
// token devolve o token atual do config (AuthToken()), usado no retry do 401. client nil
// usa http.DefaultClient.
type drainTarget struct {
	serverURL string
	token     func() string
	client    *http.Client
}

// drainResult resume uma passagem. Acked são itens confirmados e removidos. Failed são
// tentativas gastas. Descartados são itens com auth_mode fora de static e none (o modo OIDC
// fica fora da 002), apagados como ilegíveis. Parado diz por que a passagem terminou antes do
// fim do lote, vazio se terminou normalmente.
type drainResult struct {
	Acked       int
	Failed      int
	Descartados int
	Parado      string
}

// batchEntry é um elemento do array enviado a /hook/batch: {url, body}.
type batchEntry struct {
	URL  string          `json:"url"`
	Body json.RawMessage `json:"body"`
}

// batchAck é o reconhecimento. accepted_indices tem prioridade; accepted vale como prefixo
// quando o índice não vem (docs/lifecycle-event-ingestion.md §2.2).
type batchAck struct {
	Accepted        *int  `json:"accepted"`
	AcceptedIndices []int `json:"accepted_indices"`
	FailedIndex     *int  `json:"failed_index"`
}

// drenarSpool faz uma passagem sobre o lote atual do spool e devolve o resumo. Erro só para
// falha local no spool (ler, apagar ou gravar a fila); falhas do servidor viram Parado ou
// tentativas gastas.
func drenarSpool(ctx context.Context, sp *hookSpool, alvo drainTarget) (drainResult, error) {
	var res drainResult
	if _, err := url.Parse(alvo.serverURL); err != nil || alvo.serverURL == "" {
		return res, fmt.Errorf("drenagem: server_url inválido %q", alvo.serverURL)
	}
	lote, err := sp.Batch()
	if err != nil {
		return res, err
	}
	var validos []spoolItem
	var ilegiveis []string
	for _, it := range lote {
		if it.AuthMode != spoolAuthStatic && it.AuthMode != spoolAuthNone {
			ilegiveis = append(ilegiveis, it.ID)
			continue
		}
		validos = append(validos, it)
	}
	if len(ilegiveis) > 0 {
		if err := sp.Ack(ilegiveis); err != nil {
			return res, err
		}
		res.Descartados = len(ilegiveis)
	}
	for _, corrida := range corridasDeLote(validos) {
		if err := ctx.Err(); err != nil {
			res.Parado = "contexto encerrado"
			return res, nil
		}
		parado, err := enviarCorrida(ctx, sp, alvo, corrida, &res)
		if err != nil {
			return res, err
		}
		if parado != "" {
			res.Parado = parado
			return res, nil
		}
	}
	return res, nil
}

// corridasDeLote separa o lote em trechos contíguos com a mesma credencial (auth_mode e token),
// porque um lote é um único POST com um único cabeçalho de autorização. A ordem é mantida.
func corridasDeLote(lote []spoolItem) [][]spoolItem {
	var corridas [][]spoolItem
	for _, it := range lote {
		n := len(corridas)
		if n > 0 && corridas[n-1][0].AuthMode == it.AuthMode && corridas[n-1][0].Token == it.Token {
			corridas[n-1] = append(corridas[n-1], it)
			continue
		}
		corridas = append(corridas, []spoolItem{it})
	}
	return corridas
}

// enviarCorrida envia um trecho e aplica o reconhecimento. Devolve o motivo de parada, vazio
// se a passagem pode continuar.
func enviarCorrida(ctx context.Context, sp *hookSpool, alvo drainTarget, corrida []spoolItem, res *drainResult) (string, error) {
	var mapa []spoolItem
	var entradas []batchEntry
	var ruins []string
	for _, it := range corrida {
		u, err := retargetURL(it.URL, alvo.serverURL)
		if err != nil {
			ruins = append(ruins, it.ID)
			continue
		}
		mapa = append(mapa, it)
		entradas = append(entradas, batchEntry{URL: u, Body: it.Body})
	}
	if len(ruins) > 0 {
		if err := sp.Fail(ruins); err != nil {
			return "", err
		}
		res.Failed += len(ruins)
	}
	if len(mapa) == 0 {
		return "", nil
	}
	corpo, err := json.Marshal(entradas)
	if err != nil {
		return "", fmt.Errorf("drenagem: serializar lote: %w", err)
	}

	credencial := credencialDoItem(mapa[0])
	status, resp, err := postLote(ctx, alvo, credencial, corpo)
	if err != nil {
		if ctx.Err() != nil {
			return "contexto encerrado", nil
		}
		return falharLote(sp, mapa, res, "transporte: "+err.Error())
	}
	if status == http.StatusUnauthorized {
		if atual := tokenAtual(alvo); atual != "" {
			status, resp, err = postLote(ctx, alvo, atual, corpo)
			if err != nil {
				if ctx.Err() != nil {
					return "contexto encerrado", nil
				}
				return falharLote(sp, mapa, res, "transporte no retry: "+err.Error())
			}
		}
	}

	switch {
	case status == drainStatusTooMany:
		// Saturação global: não gasta tentativa. Aceita o que vier reconhecido e para.
		if ack, err := lerAck(resp); err == nil {
			if aceito, err := aceitosDoAck(ack, len(mapa)); err == nil {
				if err := confirmar(sp, mapa, aceito, res); err != nil {
					return "", err
				}
			}
		}
		return "limitado pelo servidor (429)", nil
	case status >= 200 && status < 300:
		return tratarAck(sp, mapa, resp, res)
	case status == http.StatusUnauthorized:
		return falharLote(sp, mapa, res, "401 mesmo após o retry com o token atual")
	default:
		return falharLote(sp, mapa, res, fmt.Sprintf("status %d", status))
	}
}

// tratarAck aplica um reconhecimento 2xx. Ack ilegível ou inconsistente gasta tentativa de
// todo o trecho.
func tratarAck(sp *hookSpool, mapa []spoolItem, corpo []byte, res *drainResult) (string, error) {
	ack, err := lerAck(corpo)
	if err != nil {
		return falharLote(sp, mapa, res, "reconhecimento ilegível: "+err.Error())
	}
	aceito, err := aceitosDoAck(ack, len(mapa))
	if err != nil {
		return falharLote(sp, mapa, res, "reconhecimento inconsistente: "+err.Error())
	}
	falha := -1
	if ack.FailedIndex != nil {
		falha = *ack.FailedIndex
		if falha < 0 || falha >= len(mapa) || aceito[falha] {
			return falharLote(sp, mapa, res, fmt.Sprintf("failed_index %d inválido", falha))
		}
		for j := falha + 1; j < len(mapa); j++ {
			if aceito[j] {
				return falharLote(sp, mapa, res, fmt.Sprintf("aceito depois de failed_index %d", falha))
			}
		}
	}
	if err := confirmar(sp, mapa, aceito, res); err != nil {
		return "", err
	}
	if falha >= 0 {
		if err := sp.Fail([]string{mapa[falha].ID}); err != nil {
			return "", err
		}
		res.Failed++
		return fmt.Sprintf("falha de processamento no índice %d", falha), nil
	}
	return "", nil
}

// confirmar apaga do spool os itens marcados em aceito e conta.
func confirmar(sp *hookSpool, mapa []spoolItem, aceito []bool, res *drainResult) error {
	var ids []string
	for i, ok := range aceito {
		if ok {
			ids = append(ids, mapa[i].ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	if err := sp.Ack(ids); err != nil {
		return err
	}
	res.Acked += len(ids)
	return nil
}

// falharLote gasta uma tentativa de todos os itens do trecho e encerra a passagem.
func falharLote(sp *hookSpool, mapa []spoolItem, res *drainResult, motivo string) (string, error) {
	ids := make([]string, len(mapa))
	for i, it := range mapa {
		ids[i] = it.ID
	}
	if err := sp.Fail(ids); err != nil {
		return "", err
	}
	res.Failed += len(ids)
	return motivo, nil
}

// aceitosDoAck devolve, por índice do trecho, se o servidor confirmou o item. Índice fora da
// faixa ou accepted maior que o trecho é reconhecimento inconsistente.
func aceitosDoAck(ack batchAck, n int) ([]bool, error) {
	aceito := make([]bool, n)
	switch {
	case ack.AcceptedIndices != nil:
		for _, i := range ack.AcceptedIndices {
			if i < 0 || i >= n {
				return nil, fmt.Errorf("accepted_indices com índice %d fora de 0..%d", i, n-1)
			}
			aceito[i] = true
		}
	case ack.Accepted != nil:
		if *ack.Accepted < 0 || *ack.Accepted > n {
			return nil, fmt.Errorf("accepted %d fora de 0..%d", *ack.Accepted, n)
		}
		for i := 0; i < *ack.Accepted; i++ {
			aceito[i] = true
		}
	}
	return aceito, nil
}

// lerAck decodifica o reconhecimento. Corpo vazio ou sem JSON é erro.
func lerAck(corpo []byte) (batchAck, error) {
	var ack batchAck
	if len(bytes.TrimSpace(corpo)) == 0 {
		return ack, errors.New("corpo vazio")
	}
	if err := json.Unmarshal(corpo, &ack); err != nil {
		return ack, err
	}
	return ack, nil
}

// postLote envia o array ao /hook/batch do server_url atual. credencial vazia não envia o
// cabeçalho Authorization (modo none). O timeout é drainBatchTimeout.
func postLote(ctx context.Context, alvo drainTarget, credencial string, corpo []byte) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, drainBatchTimeout)
	defer cancel()
	destino := strings.TrimSuffix(alvo.serverURL, "/") + drainBatchPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, destino, bytes.NewReader(corpo))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if credencial != "" {
		req.Header.Set("Authorization", "Bearer "+credencial)
	}
	cliente := alvo.client
	if cliente == nil {
		cliente = http.DefaultClient
	}
	resp, err := cliente.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	lido, err := io.ReadAll(io.LimitReader(resp.Body, drainAckMaxBytes))
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, lido, nil
}

// credencialDoItem é o token que o item carrega: o token guardado só vale para auth_mode static.
func credencialDoItem(it spoolItem) string {
	if it.AuthMode == spoolAuthStatic {
		return it.Token
	}
	return ""
}

// tokenAtual lê o token vigente do config. Sem função, não há token atual.
func tokenAtual(alvo drainTarget) string {
	if alvo.token == nil {
		return ""
	}
	return alvo.token()
}

// retargetURL troca esquema e host do endereço gravado pelo server_url atual e mantém caminho
// e query (que trazem a ingest_key). Endereço que não interpreta é erro.
func retargetURL(gravada, serverURL string) (string, error) {
	item, err := url.Parse(gravada)
	if err != nil {
		return "", fmt.Errorf("endereço do item: %w", err)
	}
	base, err := url.Parse(serverURL)
	if err != nil {
		return "", fmt.Errorf("server_url: %w", err)
	}
	item.Scheme = base.Scheme
	item.Host = base.Host
	item.User = base.User
	return item.String(), nil
}
