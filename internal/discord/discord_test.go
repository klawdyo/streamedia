package discord

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// captureServer cria um servidor HTTP de teste que coleta os corpos JSON
// recebidos (thread-safe) e responde 204, como o Discord faz.
func captureServer(t *testing.T) (*httptest.Server, *[]webhookPayload, *sync.Mutex) {
	t.Helper()
	var mu sync.Mutex
	var got []webhookPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var p webhookPayload
		_ = json.Unmarshal(body, &p)
		mu.Lock()
		got = append(got, p)
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	return srv, &got, &mu
}

// TestNilAlerterIsNoOp garante que um *Alerter nil (DISCORD_WEBHOOK_URL ausente)
// é seguro: todos os métodos viram no-op sem panic.
func TestNilAlerterIsNoOp(t *testing.T) {
	a := NewAlerter("") // URL vazia → nil
	if a != nil {
		t.Fatalf("NewAlerter(\"\") deveria devolver nil, devolveu %v", a)
	}
	// Nenhuma destas chamadas deve causar panic.
	a.AlertTranscodeFailure("v1", "failed_transcode", "boom")
	a.AlertQueueFull("v1")
	a.AlertTranscodeStuck("v1", 2)
	a.RecordTranscodeSuccess()
}

// TestAlertTranscodeFailure_SendsEmbed verifica que a falha de transcodificação
// envia um embed com os campos esperados (video_id, status, error_message).
func TestAlertTranscodeFailure_SendsEmbed(t *testing.T) {
	srv, got, mu := captureServer(t)
	a := NewAlerter(srv.URL)

	a.AlertTranscodeFailure("vid-123", "failed_transcode", "ffmpeg quebrou")

	mu.Lock()
	defer mu.Unlock()
	if len(*got) != 1 {
		t.Fatalf("esperava 1 embed, obteve %d", len(*got))
	}
	embeds := (*got)[0].Embeds
	if len(embeds) != 1 {
		t.Fatalf("esperava 1 embed no payload, obteve %d", len(embeds))
	}
	fields := map[string]string{}
	for _, f := range embeds[0].Fields {
		fields[f.Name] = f.Value
	}
	if fields["video_id"] != "vid-123" {
		t.Errorf("video_id = %q", fields["video_id"])
	}
	if fields["status"] != "failed_transcode" {
		t.Errorf("status = %q", fields["status"])
	}
	if fields["error_message"] != "ffmpeg quebrou" {
		t.Errorf("error_message = %q", fields["error_message"])
	}
	if embeds[0].Timestamp == "" {
		t.Error("timestamp vazio")
	}
}

// TestConsecutiveFailuresThreshold garante que, ao atingir o limiar de falhas
// consecutivas, um alerta ADICIONAL de "aumento anormal" é disparado; e que um
// sucesso no meio zera o contador.
func TestConsecutiveFailuresThreshold(t *testing.T) {
	srv, got, mu := captureServer(t)
	a := NewAlerter(srv.URL)

	// (threshold-1) falhas: nenhum alerta extra ainda.
	for i := 0; i < consecutiveFailureThreshold-1; i++ {
		a.AlertTranscodeFailure("v", "failed_transcode", "x")
	}
	// Um sucesso zera o contador.
	a.RecordTranscodeSuccess()
	// Mais (threshold-1) falhas: ainda sem alerta extra (contador reiniciou).
	for i := 0; i < consecutiveFailureThreshold-1; i++ {
		a.AlertTranscodeFailure("v", "failed_transcode", "x")
	}

	countExtra := func() (failures, extras int) {
		mu.Lock()
		defer mu.Unlock()
		for _, p := range *got {
			for _, e := range p.Embeds {
				if e.Color == colorRed && len(e.Fields) > 0 && e.Fields[0].Name == "falhas_consecutivas" {
					extras++
				} else {
					failures++
				}
			}
		}
		return
	}

	_, extras := countExtra()
	if extras != 0 {
		t.Fatalf("não deveria haver alerta de falhas consecutivas ainda, obteve %d", extras)
	}

	// Mais uma falha cruza o limiar → dispara o alerta extra.
	a.AlertTranscodeFailure("v-final", "failed_transcode", "x")
	_, extras = countExtra()
	if extras != 1 {
		t.Fatalf("esperava 1 alerta de falhas consecutivas, obteve %d", extras)
	}
}

// TestAlertQueueFull e TestAlertTranscodeStuck cobrem os outros dois gatilhos.
func TestAlertQueueFull(t *testing.T) {
	srv, got, mu := captureServer(t)
	a := NewAlerter(srv.URL)
	a.AlertQueueFull("vid-q")

	mu.Lock()
	defer mu.Unlock()
	if len(*got) != 1 || len((*got)[0].Embeds) != 1 {
		t.Fatalf("esperava 1 embed, obteve %+v", *got)
	}
	if (*got)[0].Embeds[0].Color != colorOrange {
		t.Errorf("cor inesperada: %d", (*got)[0].Embeds[0].Color)
	}
}

func TestAlertTranscodeStuck(t *testing.T) {
	srv, got, mu := captureServer(t)
	a := NewAlerter(srv.URL)
	a.AlertTranscodeStuck("vid-s", 3)

	mu.Lock()
	defer mu.Unlock()
	if len(*got) != 1 || len((*got)[0].Embeds) != 1 {
		t.Fatalf("esperava 1 embed, obteve %+v", *got)
	}
	fields := map[string]string{}
	for _, f := range (*got)[0].Embeds[0].Fields {
		fields[f.Name] = f.Value
	}
	if fields["video_id"] != "vid-s" {
		t.Errorf("video_id = %q", fields["video_id"])
	}
	if fields["tentativas"] != "3" {
		t.Errorf("tentativas = %q", fields["tentativas"])
	}
}

// TestTruncate cobre o corte de campos longos (limite de 1024 do Discord).
func TestTruncate(t *testing.T) {
	if got := truncate("abc", 10); got != "abc" {
		t.Errorf("truncate sem corte: %q", got)
	}
	got := truncate("abcdef", 4)
	if []rune(got)[len([]rune(got))-1] != '…' {
		t.Errorf("esperava reticências no fim, obteve %q", got)
	}
	if len([]rune(got)) != 4 {
		t.Errorf("esperava 4 runes, obteve %d (%q)", len([]rune(got)), got)
	}
}
