package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/klawdyo/streamedia/internal/apiresponse"
	"github.com/klawdyo/streamedia/internal/models"
)

// playInitData espelha o payload de dados de POST /api/play/init para os
// testes, incluindo o campo resolutions adicionado para montar players por
// resolução no cliente.
type playInitData struct {
	VideoID     string `json:"video_id"`
	Tag         string `json:"tag"`
	PlayURL     string `json:"play_url"`
	Token       string `json:"token"`
	ExpiresAt   string `json:"expires_at"`
	Resolutions []int  `json:"resolutions"`
}

func decodePlayInit(t *testing.T, rec *httptest.ResponseRecorder) playInitData {
	t.Helper()
	var env apiresponse.Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("erro ao desserializar envelope: %v", err)
	}
	dataJSON, _ := json.Marshal(env.Data)
	var data playInitData
	if err := json.Unmarshal(dataJSON, &data); err != nil {
		t.Fatalf("erro ao desserializar data: %v", err)
	}
	return data
}

func callPlayInit(t *testing.T, h *PlayInitHandler, videoID string) *httptest.ResponseRecorder {
	t.Helper()
	body := strings.NewReader(`{"video_id":"` + videoID + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/play/init", body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TestPlayInit_IncludesResolutions verifica que o play/init devolve as
// resoluções das variantes HLS geradas, ordenadas asc — mesmo que tenham
// sido gravadas fora de ordem.
func TestPlayInit_IncludesResolutions(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)
	insertVideo(t, database, testVideoID, string(models.StatusReady), testTag)

	// Grava as variantes fora de ordem para exercitar o ORDER BY resolution ASC.
	for _, res := range []int{720, 480, 1080} {
		if err := models.UpsertVideoRendition(database, testVideoID, res, 1024, 3); err != nil {
			t.Fatalf("UpsertVideoRendition(%d): %v", res, err)
		}
	}

	h := NewPlayInitHandler(cfg, database)
	rec := callPlayInit(t, h, testVideoID)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtido %d (body: %s)", rec.Code, rec.Body.String())
	}
	data := decodePlayInit(t, rec)
	want := []int{480, 720, 1080}
	if len(data.Resolutions) != len(want) {
		t.Fatalf("resolutions: esperado %v, obtido %v", want, data.Resolutions)
	}
	for i, v := range want {
		if data.Resolutions[i] != v {
			t.Fatalf("resolutions[%d]: esperado %d, obtido %d (lista %v)", i, v, data.Resolutions[i], data.Resolutions)
		}
	}
}

// TestPlayInit_EmptyResolutions garante que um vídeo ready sem variantes
// registradas devolve uma lista vazia (não null) — o cliente cai no fallback.
func TestPlayInit_EmptyResolutions(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)
	insertVideo(t, database, testVideoID, string(models.StatusReady), testTag)

	h := NewPlayInitHandler(cfg, database)
	rec := callPlayInit(t, h, testVideoID)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtido %d (body: %s)", rec.Code, rec.Body.String())
	}
	// O JSON deve conter "resolutions":[] (array vazio, nunca null).
	if !strings.Contains(rec.Body.String(), `"resolutions":[]`) {
		t.Fatalf("esperado resolutions:[] no corpo, obtido: %s", rec.Body.String())
	}
}
