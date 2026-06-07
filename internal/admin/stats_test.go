package admin

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/klawdyo/streamedia/internal/models"
)

// statsTestResponse espelha a estrutura JSON retornada por HandleStats,
// usada para decodificar e inspecionar a resposta nos testes.
type statsTestResponse struct {
	VideoID      *string          `json:"video_id"`
	Totals       map[string]int64 `json:"totals"`
	ByResolution map[int]int64    `json:"by_resolution"`
	ByOS         map[string]int64 `json:"by_os"`
	ByDayOfWeek  map[int]int64    `json:"by_day_of_week"`
}

// recordEvent é um atalho para inserir eventos de teste, abortando o teste
// em caso de erro.
func recordEvent(t *testing.T, database *sql.DB, videoID, eventType string, resolution *int, userAgent string) {
	t.Helper()
	if err := models.RecordEvent(database, videoID, eventType, resolution, userAgent); err != nil {
		t.Fatalf("RecordEvent falhou: %v", err)
	}
}

func intPtr(v int) *int { return &v }

func TestStatsRoute_RequiresAdminAuth(t *testing.T) {
	database, cfg := setupAdminTest(t)
	handler := NewAdminHandler(cfg, database, &mockQueue{})

	wrapped := AdminAuth(cfg.AdminToken)(http.HandlerFunc(handler.HandleStats))

	req := httptest.NewRequest(http.MethodGet, "/admin/stats", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401 sem Authorization, obtido %d", rec.Code)
	}
}

func TestStatsRoute_GlobalAggregation(t *testing.T) {
	database, cfg := setupAdminTest(t)
	handler := NewAdminHandler(cfg, database, &mockQueue{})

	insertVideo(t, database, "vid-1", models.StatusReady)
	insertVideo(t, database, "vid-2", models.StatusReady)

	recordEvent(t, database, "vid-1", "playback", nil, "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)")
	recordEvent(t, database, "vid-1", "download_segment", intPtr(720), "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)")
	recordEvent(t, database, "vid-2", "download_segment", intPtr(480), "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")
	recordEvent(t, database, "vid-2", "upload_complete", nil, "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

	req := httptest.NewRequest(http.MethodGet, "/admin/stats", nil)
	req.Header.Set("Authorization", "Bearer "+cfg.AdminToken)
	rec := httptest.NewRecorder()
	handler.HandleStats(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtido %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp statsTestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("erro ao decodificar resposta: %v", err)
	}

	if resp.VideoID != nil {
		t.Errorf("esperava video_id nulo na agregação global, obteve %v", *resp.VideoID)
	}
	if resp.Totals["playback"] != 1 || resp.Totals["download_segment"] != 2 || resp.Totals["upload_complete"] != 1 {
		t.Errorf("totals inesperados: %+v", resp.Totals)
	}
	if resp.ByResolution[720] != 1 || resp.ByResolution[480] != 1 {
		t.Errorf("by_resolution inesperado: %+v", resp.ByResolution)
	}
	if resp.ByOS["ios"] != 2 || resp.ByOS["windows"] != 2 {
		t.Errorf("by_os inesperado: %+v", resp.ByOS)
	}
	if len(resp.ByDayOfWeek) == 0 {
		t.Errorf("esperava agregação por dia da semana não-vazia")
	}
}

func TestStatsRoute_FilteredByVideoID(t *testing.T) {
	database, cfg := setupAdminTest(t)
	handler := NewAdminHandler(cfg, database, &mockQueue{})

	insertVideo(t, database, "vid-1", models.StatusReady)
	insertVideo(t, database, "vid-2", models.StatusReady)

	recordEvent(t, database, "vid-1", "download_segment", intPtr(720), "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)")
	recordEvent(t, database, "vid-1", "download_segment", intPtr(720), "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)")
	recordEvent(t, database, "vid-2", "download_segment", intPtr(480), "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

	req := httptest.NewRequest(http.MethodGet, "/admin/stats?video_id=vid-1", nil)
	req.Header.Set("Authorization", "Bearer "+cfg.AdminToken)
	rec := httptest.NewRecorder()
	handler.HandleStats(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtido %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp statsTestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("erro ao decodificar resposta: %v", err)
	}

	if resp.VideoID == nil || *resp.VideoID != "vid-1" {
		t.Fatalf("esperava video_id = \"vid-1\", obteve %v", resp.VideoID)
	}
	if resp.Totals["download_segment"] != 2 {
		t.Errorf("esperava 2 eventos download_segment para vid-1, obteve %d", resp.Totals["download_segment"])
	}
	if resp.ByResolution[480] != 0 {
		t.Errorf("não esperava eventos de resolução 480 (pertencem a vid-2), obteve %d", resp.ByResolution[480])
	}
	if resp.ByResolution[720] != 2 {
		t.Errorf("esperava 2 eventos de resolução 720, obteve %d", resp.ByResolution[720])
	}
}

func TestStatsRoute_UnknownVideoID(t *testing.T) {
	database, cfg := setupAdminTest(t)
	handler := NewAdminHandler(cfg, database, &mockQueue{})

	req := httptest.NewRequest(http.MethodGet, "/admin/stats?video_id=video-inexistente", nil)
	req.Header.Set("Authorization", "Bearer "+cfg.AdminToken)
	rec := httptest.NewRecorder()
	handler.HandleStats(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("esperado 404 para video_id inexistente, obtido %d", rec.Code)
	}
}

func TestStatsRoute_EmptyDataset(t *testing.T) {
	database, cfg := setupAdminTest(t)
	handler := NewAdminHandler(cfg, database, &mockQueue{})

	req := httptest.NewRequest(http.MethodGet, "/admin/stats", nil)
	req.Header.Set("Authorization", "Bearer "+cfg.AdminToken)
	rec := httptest.NewRecorder()
	handler.HandleStats(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200 mesmo sem eventos, obtido %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp statsTestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("erro ao decodificar resposta: %v", err)
	}

	if resp.Totals["playback"] != 0 || resp.Totals["download_segment"] != 0 || resp.Totals["upload_complete"] != 0 {
		t.Errorf("esperava totals zerados, obteve %+v", resp.Totals)
	}
	if len(resp.ByResolution) != 0 || len(resp.ByOS) != 0 || len(resp.ByDayOfWeek) != 0 {
		t.Errorf("esperava mapas de agregação vazios, obteve resolution=%+v os=%+v dow=%+v",
			resp.ByResolution, resp.ByOS, resp.ByDayOfWeek)
	}
}
