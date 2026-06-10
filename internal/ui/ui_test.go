package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestServeUI verifica que a página de teste é servida como HTML e contém os
// marcadores das etapas esperadas do fluxo.
func TestServeUI(t *testing.T) {
	h := NewHandler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui", nil)

	h.ServeUI(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: esperado 200, obtido %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type: esperado text/html, obtido %q", ct)
	}
	body := rec.Body.String()
	// Conteúdos-âncora que garantem que a página certa foi servida.
	for _, want := range []string{"Streamedia", "/api/upload/init", "/ui/webhook", "Players por resolução"} {
		if !strings.Contains(body, want) {
			t.Errorf("página não contém %q", want)
		}
	}
}

// TestReceiveAndListWebhooks cobre o ciclo completo do receptor: receber um
// webhook via POST e recuperá-lo via GET /ui/webhook/events.
func TestReceiveAndListWebhooks(t *testing.T) {
	h := NewHandler()

	// Recebe um webhook.
	payload := `{"video_id":"abc","event":"transcode_complete","status":"ready"}`
	postReq := httptest.NewRequest(http.MethodPost, "/ui/webhook", strings.NewReader(payload))
	postReq.Header.Set("Content-Type", "application/json")
	postReq.Header.Set("X-Signature", "sha256=deadbeef")
	postRec := httptest.NewRecorder()

	h.ReceiveWebhook(postRec, postReq)
	if postRec.Code != http.StatusOK {
		t.Fatalf("ReceiveWebhook status: esperado 200, obtido %d", postRec.Code)
	}

	// Lista os eventos.
	listRec := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/ui/webhook/events", nil)
	h.ListEvents(listRec, listReq)

	var events []webhookEvent
	if err := json.Unmarshal(listRec.Body.Bytes(), &events); err != nil {
		t.Fatalf("erro ao decodificar eventos: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("esperado 1 evento, obtido %d", len(events))
	}
	ev := events[0]
	if ev.Seq != 1 {
		t.Errorf("Seq: esperado 1, obtido %d", ev.Seq)
	}
	if ev.RawBody != payload {
		t.Errorf("RawBody divergente: %q", ev.RawBody)
	}
	if ev.Headers["X-Signature"] != "sha256=deadbeef" {
		t.Errorf("X-Signature não capturada: %v", ev.Headers)
	}
}

// TestListEventsSince garante o polling incremental: ?since=N só devolve
// eventos com Seq maior que N.
func TestListEventsSince(t *testing.T) {
	h := NewHandler()
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/ui/webhook", strings.NewReader(`{}`))
		h.ReceiveWebhook(rec, req)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/webhook/events?since=2", nil)
	h.ListEvents(rec, req)

	var events []webhookEvent
	if err := json.Unmarshal(rec.Body.Bytes(), &events); err != nil {
		t.Fatalf("erro ao decodificar: %v", err)
	}
	if len(events) != 1 || events[0].Seq != 3 {
		t.Fatalf("since=2 deveria devolver só o evento #3, obtido %+v", events)
	}
}

// TestWebhookBufferEviction confirma que o buffer descarta os webhooks mais
// antigos ao exceder maxWebhookEvents, mantendo apenas os mais recentes.
func TestWebhookBufferEviction(t *testing.T) {
	h := NewHandler()
	const extra = 5
	for i := 0; i < maxWebhookEvents+extra; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/ui/webhook", strings.NewReader(`{}`))
		h.ReceiveWebhook(rec, req)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/webhook/events", nil)
	h.ListEvents(rec, req)

	var events []webhookEvent
	if err := json.Unmarshal(rec.Body.Bytes(), &events); err != nil {
		t.Fatalf("erro ao decodificar: %v", err)
	}
	if len(events) != maxWebhookEvents {
		t.Fatalf("buffer deveria limitar a %d eventos, obtido %d", maxWebhookEvents, len(events))
	}
	// O evento mais antigo retido deve ser o de número (extra+1).
	if events[0].Seq != extra+1 {
		t.Errorf("evento mais antigo: esperado Seq=%d, obtido %d", extra+1, events[0].Seq)
	}
}
