package sse

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/klawdyo/streamedia/internal/notify"
)

// allowAuth é um Authorizer que aceita tudo (usado nos testes de stream).
func allowAuth(token, videoID string) bool { return true }

// TestServeHTTP_MissingParams devolve 400 sem video_id/token.
func TestServeHTTP_MissingParams(t *testing.T) {
	h := NewHandler(NewHub(), allowAuth)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/events", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("esperado 400, obtido %d", rec.Code)
	}
}

// TestServeHTTP_BadToken devolve 401 quando o autorizador rejeita.
func TestServeHTTP_BadToken(t *testing.T) {
	deny := func(token, videoID string) bool { return false }
	h := NewHandler(NewHub(), deny)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/events?video_id=v&token=bad", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401, obtido %d", rec.Code)
	}
}

// TestHubDeliver_RoutesByVideo confirma que a entrega só vai para os ouvintes
// do video_id correspondente.
func TestHubDeliver_RoutesByVideo(t *testing.T) {
	hub := NewHub()
	subA := hub.subscribe("video-A")
	subB := hub.subscribe("video-B")

	hub.Deliver(notify.Notification{VideoID: "video-A", Event: "ready"})

	select {
	case n := <-subA.ch:
		if n.Event != "ready" {
			t.Errorf("subA: evento inesperado %q", n.Event)
		}
	case <-time.After(time.Second):
		t.Fatal("subA: timeout — deveria ter recebido o evento")
	}

	select {
	case n := <-subB.ch:
		t.Fatalf("subB: não deveria receber evento de outro vídeo, recebeu %+v", n)
	case <-time.After(100 * time.Millisecond):
		// esperado: nada
	}
}

// TestHubUnsubscribe_StopsDelivery garante que após unsubscribe o ouvinte some
// do índice (sem pânico em Deliver concorrente).
func TestHubUnsubscribe_StopsDelivery(t *testing.T) {
	hub := NewHub()
	sub := hub.subscribe("v")
	if hub.SubscriberCount("v") != 1 {
		t.Fatalf("esperado 1 ouvinte, obtido %d", hub.SubscriberCount("v"))
	}
	hub.unsubscribe("v", sub)
	if hub.SubscriberCount("v") != 0 {
		t.Fatalf("esperado 0 ouvintes após unsubscribe, obtido %d", hub.SubscriberCount("v"))
	}
	// Deliver após unsubscribe não deve causar pânico.
	hub.Deliver(notify.Notification{VideoID: "v", Event: "ready"})
}

// TestStream_DeliversEvent sobe o handler num servidor real, conecta um cliente
// SSE, entrega uma notificação pelo hub e verifica que o evento chega no stream.
func TestStream_DeliversEvent(t *testing.T) {
	hub := NewHub()
	srv := httptest.NewServer(NewHandler(hub, allowAuth))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/events?video_id=vid&token=tok", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("conexão SSE: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("esperado 200, obtido %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type: esperado text/event-stream, obtido %q", ct)
	}

	// Aguarda o handler registrar a inscrição antes de entregar.
	deadline := time.Now().Add(2 * time.Second)
	for hub.SubscriberCount("vid") == 0 {
		if time.Now().After(deadline) {
			t.Fatal("timeout: inscrição não registrada")
		}
		time.Sleep(10 * time.Millisecond)
	}

	hub.Deliver(notify.Notification{VideoID: "vid", Event: "ready", Status: "ready"})

	// Lê o stream até achar a linha de evento.
	reader := bufio.NewReader(resp.Body)
	var sawEvent, sawData bool
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			line, err := reader.ReadString('\n')
			if strings.HasPrefix(line, "event: ready") {
				sawEvent = true
			}
			if strings.HasPrefix(line, "data:") && strings.Contains(line, `"event":"ready"`) {
				sawData = true
			}
			if sawEvent && sawData {
				return
			}
			if err != nil {
				return
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout esperando o evento no stream")
	}
	if !sawEvent || !sawData {
		t.Fatalf("evento não recebido corretamente (event=%v data=%v)", sawEvent, sawData)
	}
}
