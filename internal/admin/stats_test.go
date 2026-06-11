package admin

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/klawdyo/streamedia/internal/apiresponse"
	"github.com/klawdyo/streamedia/internal/models"
)

// statsTestResponse espelha a estrutura JSON retornada por HandleStats,
// usada para decodificar e inspecionar a resposta nos testes.
type statsTestResponse struct {
	VideoID      *string                  `json:"video_id"`
	Totals       map[string]int64         `json:"totals"`
	ByResolution map[int]int64            `json:"by_resolution"`
	ByOS         map[string]int64         `json:"by_os"`
	ByDayOfWeek  map[int]int64            `json:"by_day_of_week"`
	ByHour       map[int]int64            `json:"by_hour"`
	ByDate       map[string]int64         `json:"by_date"`
	Storage      *storageTestSection      `json:"storage"`
	Uploads      *uploadsTestSection      `json:"uploads"`
	VideoStorage *videoStorageTestSection `json:"video_storage"`
}

// storageTestSection espelha a seção "storage" da resposta (T36/T37,
// issue #5): armazenamento e fila, somadas às estatísticas de uso já
// existentes (T28). Workers foi acrescentado para o dashboard.
type storageTestSection struct {
	TotalBytes           int64                      `json:"total_bytes"`
	TotalDurationSeconds int64                      `json:"total_duration_seconds"`
	VideosByStatus       map[models.VideoStatus]int `json:"videos_by_status"`
	QueuePending         int                        `json:"queue_pending"`
	Workers              int                        `json:"workers"`
}

// uploadsTestSection espelha a seção "uploads" (movimentação de envios).
type uploadsTestSection struct {
	Total       int64            `json:"total"`
	ByDate      map[string]int64 `json:"by_date"`
	ByDayOfWeek map[int]int64    `json:"by_day_of_week"`
	ByHour      map[int]int64    `json:"by_hour"`
}

// videoStorageTestSection espelha a ficha de armazenamento por vídeo.
type videoStorageTestSection struct {
	Renditions []struct {
		Resolution   int   `json:"resolution"`
		SizeBytes    int64 `json:"size_bytes"`
		SegmentCount int   `json:"segment_count"`
	} `json:"renditions"`
	TotalBytes      int64 `json:"total_bytes"`
	DurationSeconds int   `json:"duration_seconds"`
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

	wrapped := RootAuth(cfg.RootToken)(http.HandlerFunc(handler.HandleStats))

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
	req.Header.Set("Authorization", "Bearer "+cfg.RootToken)
	rec := httptest.NewRecorder()
	handler.HandleStats(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtido %d (body: %s)", rec.Code, rec.Body.String())
	}

	var env apiresponse.Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("erro ao decodificar resposta: %v", err)
	}
	dataJSON, _ := json.Marshal(env.Data)
	var resp statsTestResponse
	json.Unmarshal(dataJSON, &resp)

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
	req.Header.Set("Authorization", "Bearer "+cfg.RootToken)
	rec := httptest.NewRecorder()
	handler.HandleStats(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtido %d (body: %s)", rec.Code, rec.Body.String())
	}

	var env apiresponse.Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("erro ao decodificar resposta: %v", err)
	}
	dataJSON, _ := json.Marshal(env.Data)
	var resp statsTestResponse
	json.Unmarshal(dataJSON, &resp)

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
	req.Header.Set("Authorization", "Bearer "+cfg.RootToken)
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
	req.Header.Set("Authorization", "Bearer "+cfg.RootToken)
	rec := httptest.NewRecorder()
	handler.HandleStats(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200 mesmo sem eventos, obtido %d (body: %s)", rec.Code, rec.Body.String())
	}

	var env apiresponse.Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("erro ao decodificar resposta: %v", err)
	}
	dataJSON, _ := json.Marshal(env.Data)
	var resp statsTestResponse
	json.Unmarshal(dataJSON, &resp)

	if resp.Totals["playback"] != 0 || resp.Totals["download_segment"] != 0 || resp.Totals["upload_complete"] != 0 {
		t.Errorf("esperava totals zerados, obteve %+v", resp.Totals)
	}
	if len(resp.ByResolution) != 0 || len(resp.ByOS) != 0 || len(resp.ByDayOfWeek) != 0 {
		t.Errorf("esperava mapas de agregação vazios, obteve resolution=%+v os=%+v dow=%+v",
			resp.ByResolution, resp.ByOS, resp.ByDayOfWeek)
	}
}

// TestHandleStats_IncludesStorageSection verifica que /admin/stats (sem
// video_id) inclui a nova seção "storage" (T36/T37, issue #5) com os
// totais de armazenamento, duração, contagem por status e fila pendente —
// reaproveitando as agregações de internal/models/storage.go (T36).
func TestHandleStats_IncludesStorageSection(t *testing.T) {
	database, cfg := setupAdminTest(t)
	mockQ := &mockQueue{length: 2}
	handler := NewAdminHandler(cfg, database, mockQ)

	insertVideo(t, database, "vid-storage-1", models.StatusReady)
	insertVideo(t, database, "vid-storage-2", models.StatusTranscoding)

	if _, err := database.Exec(`UPDATE videos SET actual_size_bytes = 1000, duration_s = 60 WHERE video_id = ?`, "vid-storage-1"); err != nil {
		t.Fatalf("erro ao popular actual_size_bytes/duration_s: %v", err)
	}
	if _, err := database.Exec(`UPDATE videos SET actual_size_bytes = 2000, duration_s = 120 WHERE video_id = ?`, "vid-storage-2"); err != nil {
		t.Fatalf("erro ao popular actual_size_bytes/duration_s: %v", err)
	}
	if err := models.UpsertVideoRendition(database, "vid-storage-1", 480, 500, 5); err != nil {
		t.Fatalf("UpsertVideoRendition: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/stats", nil)
	req.Header.Set("Authorization", "Bearer "+cfg.RootToken)
	rec := httptest.NewRecorder()
	handler.HandleStats(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtido %d (body: %s)", rec.Code, rec.Body.String())
	}

	var env apiresponse.Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("erro ao decodificar resposta: %v", err)
	}
	dataJSON, _ := json.Marshal(env.Data)
	var resp statsTestResponse
	json.Unmarshal(dataJSON, &resp)

	if resp.Storage == nil {
		t.Fatalf("esperava seção 'storage' presente na agregação global, obteve nil")
	}
	if resp.Storage.TotalBytes != 1000+2000+500 {
		t.Errorf("total_bytes inesperado: esperava %d, obteve %d", 1000+2000+500, resp.Storage.TotalBytes)
	}
	if resp.Storage.TotalDurationSeconds != 60+120 {
		t.Errorf("total_duration_seconds inesperado: esperava %d, obteve %d", 60+120, resp.Storage.TotalDurationSeconds)
	}
	if resp.Storage.VideosByStatus[models.StatusReady] != 1 || resp.Storage.VideosByStatus[models.StatusTranscoding] != 1 {
		t.Errorf("videos_by_status inesperado: %+v", resp.Storage.VideosByStatus)
	}
	if resp.Storage.QueuePending != 2 {
		t.Errorf("queue_pending inesperado: esperava 2, obteve %d", resp.Storage.QueuePending)
	}
}

// TestHandleStats_StorageSectionConsistentWithQueueRoute verifica que o
// queue_pending devolvido em /admin/stats bate com o queue_length devolvido
// por /admin/queue — ambos devem refletir a mesma fonte (queue.Len()), sem
// recomputar a fila por caminhos diferentes (conforme orientado na T37).
func TestHandleStats_StorageSectionConsistentWithQueueRoute(t *testing.T) {
	database, cfg := setupAdminTest(t)
	mockQ := &mockQueue{length: 7}
	handler := NewAdminHandler(cfg, database, mockQ)

	statsReq := httptest.NewRequest(http.MethodGet, "/admin/stats", nil)
	statsReq.Header.Set("Authorization", "Bearer "+cfg.RootToken)
	statsRec := httptest.NewRecorder()
	handler.HandleStats(statsRec, statsReq)

	var statsEnv apiresponse.Envelope
	if err := json.Unmarshal(statsRec.Body.Bytes(), &statsEnv); err != nil {
		t.Fatalf("erro ao decodificar resposta de /admin/stats: %v", err)
	}
	statsDataJSON, _ := json.Marshal(statsEnv.Data)
	var statsResp statsTestResponse
	json.Unmarshal(statsDataJSON, &statsResp)
	if statsResp.Storage == nil {
		t.Fatalf("esperava seção 'storage' presente, obteve nil")
	}

	queueReq := httptest.NewRequest(http.MethodGet, "/admin/queue", nil)
	queueReq.Header.Set("Authorization", "Bearer "+cfg.RootToken)
	queueRec := httptest.NewRecorder()
	handler.HandleQueue(queueRec, queueReq)

	var queueEnv apiresponse.Envelope
	if err := json.Unmarshal(queueRec.Body.Bytes(), &queueEnv); err != nil {
		t.Fatalf("erro ao decodificar resposta de /admin/queue: %v", err)
	}
	queueDataJSON, _ := json.Marshal(queueEnv.Data)
	var queueResp queueResponse
	json.Unmarshal(queueDataJSON, &queueResp)

	if statsResp.Storage.QueuePending != queueResp.QueueLength {
		t.Errorf("queue_pending (%d) divergente de /admin/queue queue_length (%d)",
			statsResp.Storage.QueuePending, queueResp.QueueLength)
	}
	if statsResp.Storage.QueuePending != 7 {
		t.Errorf("esperava queue_pending = 7, obteve %d", statsResp.Storage.QueuePending)
	}
}

// decodeStats decodifica a resposta de HandleStats no envelope padrão.
func decodeStats(t *testing.T, rec *httptest.ResponseRecorder) statsTestResponse {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtido %d (body: %s)", rec.Code, rec.Body.String())
	}
	var env apiresponse.Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("erro ao decodificar resposta: %v", err)
	}
	dataJSON, _ := json.Marshal(env.Data)
	var resp statsTestResponse
	if err := json.Unmarshal(dataJSON, &resp); err != nil {
		t.Fatalf("erro ao decodificar data: %v", err)
	}
	return resp
}

// TestHandleStats_GlobalIncludesUploadsAndWorkers verifica as seções novas do
// dashboard na visão global: "uploads" (movimentação de envios por
// data/dia/hora) e storage.workers.
func TestHandleStats_GlobalIncludesUploadsAndWorkers(t *testing.T) {
	database, cfg := setupAdminTest(t)
	cfg.TranscodeWorkers = 3
	handler := NewAdminHandler(cfg, database, &mockQueue{length: 1})

	insertVideo(t, database, "vid-1", models.StatusReady)
	insertVideo(t, database, "vid-2", models.StatusReady)
	// created_at controlado: 2026-06-07 (domingo) 10h e 2026-06-08 (segunda) 22h.
	if _, err := database.Exec(`UPDATE videos SET created_at = datetime('2026-06-07 10:00:00') WHERE video_id = 'vid-1'`); err != nil {
		t.Fatalf("erro ao ajustar created_at: %v", err)
	}
	if _, err := database.Exec(`UPDATE videos SET created_at = datetime('2026-06-08 22:00:00') WHERE video_id = 'vid-2'`); err != nil {
		t.Fatalf("erro ao ajustar created_at: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/stats", nil)
	req.Header.Set("Authorization", "Bearer "+cfg.RootToken)
	rec := httptest.NewRecorder()
	handler.HandleStats(rec, req)

	resp := decodeStats(t, rec)

	if resp.Storage == nil || resp.Storage.Workers != 3 {
		t.Errorf("esperava storage.workers = 3, obteve %+v", resp.Storage)
	}
	if resp.Uploads == nil {
		t.Fatalf("esperava seção 'uploads' presente na visão global")
	}
	if resp.Uploads.Total != 2 {
		t.Errorf("uploads.total = %d, esperava 2", resp.Uploads.Total)
	}
	if resp.Uploads.ByDate["2026-06-07"] != 1 || resp.Uploads.ByDate["2026-06-08"] != 1 {
		t.Errorf("uploads.by_date inesperado: %+v", resp.Uploads.ByDate)
	}
	if resp.Uploads.ByDayOfWeek[0] != 1 || resp.Uploads.ByDayOfWeek[1] != 1 {
		t.Errorf("uploads.by_day_of_week inesperado: %+v", resp.Uploads.ByDayOfWeek)
	}
	if resp.Uploads.ByHour[10] != 1 || resp.Uploads.ByHour[22] != 1 {
		t.Errorf("uploads.by_hour inesperado: %+v", resp.Uploads.ByHour)
	}
	// VideoStorage não deve aparecer na visão global.
	if resp.VideoStorage != nil {
		t.Errorf("não esperava video_storage na visão global")
	}
}

// TestHandleStats_PlaybackByHourAndDate verifica as agregações de playback por
// hora e por data (alimentam os gráficos de horários/dias mais movimentados).
func TestHandleStats_PlaybackByHourAndDate(t *testing.T) {
	database, cfg := setupAdminTest(t)
	handler := NewAdminHandler(cfg, database, &mockQueue{})

	insertVideo(t, database, "vid-1", models.StatusReady)
	recordEvent(t, database, "vid-1", "playback", nil, "Mozilla/5.0 (Windows NT 10.0)")
	recordEvent(t, database, "vid-1", "playback", nil, "Mozilla/5.0 (Windows NT 10.0)")
	if _, err := database.Exec(`UPDATE playback_events SET occurred_at = datetime('2026-06-07 09:00:00') WHERE id = 1`); err != nil {
		t.Fatalf("erro ao ajustar occurred_at: %v", err)
	}
	if _, err := database.Exec(`UPDATE playback_events SET occurred_at = datetime('2026-06-07 09:30:00') WHERE id = 2`); err != nil {
		t.Fatalf("erro ao ajustar occurred_at: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/stats", nil)
	req.Header.Set("Authorization", "Bearer "+cfg.RootToken)
	rec := httptest.NewRecorder()
	handler.HandleStats(rec, req)

	resp := decodeStats(t, rec)
	if resp.ByHour[9] != 2 {
		t.Errorf("by_hour[9] = %d, esperava 2", resp.ByHour[9])
	}
	if resp.ByDate["2026-06-07"] != 2 {
		t.Errorf("by_date[2026-06-07] = %d, esperava 2", resp.ByDate["2026-06-07"])
	}
}

// TestHandleStats_PerVideoStorage verifica que ?video_id= inclui a ficha de
// armazenamento do vídeo (renditions + total_bytes + duração) e NÃO inclui as
// seções globais storage/uploads.
func TestHandleStats_PerVideoStorage(t *testing.T) {
	database, cfg := setupAdminTest(t)
	handler := NewAdminHandler(cfg, database, &mockQueue{})

	insertVideo(t, database, "vid-1", models.StatusReady)
	if _, err := database.Exec(`UPDATE videos SET duration_s = 90 WHERE video_id = 'vid-1'`); err != nil {
		t.Fatalf("erro ao ajustar duration_s: %v", err)
	}
	if err := models.UpsertVideoRendition(database, "vid-1", 480, 500, 5); err != nil {
		t.Fatalf("UpsertVideoRendition: %v", err)
	}
	if err := models.UpsertVideoRendition(database, "vid-1", 720, 1500, 7); err != nil {
		t.Fatalf("UpsertVideoRendition: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/stats?video_id=vid-1", nil)
	req.Header.Set("Authorization", "Bearer "+cfg.RootToken)
	rec := httptest.NewRecorder()
	handler.HandleStats(rec, req)

	resp := decodeStats(t, rec)
	if resp.Storage != nil || resp.Uploads != nil {
		t.Errorf("não esperava seções globais storage/uploads na visão por-vídeo")
	}
	if resp.VideoStorage == nil {
		t.Fatalf("esperava video_storage presente na visão por-vídeo")
	}
	if resp.VideoStorage.TotalBytes != 2000 {
		t.Errorf("video_storage.total_bytes = %d, esperava 2000", resp.VideoStorage.TotalBytes)
	}
	if resp.VideoStorage.DurationSeconds != 90 {
		t.Errorf("video_storage.duration_seconds = %d, esperava 90", resp.VideoStorage.DurationSeconds)
	}
	if len(resp.VideoStorage.Renditions) != 2 {
		t.Fatalf("esperava 2 renditions, obteve %d", len(resp.VideoStorage.Renditions))
	}
	// StorageByVideo ordena por resolução ASC.
	if resp.VideoStorage.Renditions[0].Resolution != 480 || resp.VideoStorage.Renditions[0].SizeBytes != 500 {
		t.Errorf("rendition[0] inesperado: %+v", resp.VideoStorage.Renditions[0])
	}
}
