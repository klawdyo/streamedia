package serve

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/klawdyo/streamedia/internal/models"
)

// awaitStats devolve um callback para passar como onStatsRecorded e uma
// função wait() que bloqueia até que exatamente n gravações tenham concluído.
func awaitStats(n int) (onDone func(error), wait func()) {
	var wg sync.WaitGroup
	wg.Add(n)
	onDone = func(error) { wg.Done() }
	wait = func() { wg.Wait() }
	return onDone, wait
}

// --- Testes de coleta de estatísticas (T27) ---

func TestMasterHandler_RecordsPlaybackEvent(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)

	insertVideo(t, database, testVideoID, "ready", testTag)
	writeFile(t, filepath.Join(cfg.MediaDir, testTag, testVideoID, "master.m3u8"), "#EXTM3U\n")
	token := insertPlayToken(t, database, testVideoID, time.Now().Add(time.Hour))

	onDone, wait := awaitStats(1)
	h := NewMasterHandler(cfg, database)
	h.onStatsRecorded = onDone

	req := httptest.NewRequest(http.MethodGet, masterURL(token), nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtido %d (body: %s)", rec.Code, rec.Body.String())
	}
	wait()

	count, err := models.CountEventsByType(database, "playback")
	if err != nil {
		t.Fatalf("CountEventsByType falhou: %v", err)
	}
	if count != 1 {
		t.Fatalf("esperava 1 evento \"playback\", obteve %d", count)
	}

	var resolution interface{}
	row := database.QueryRow(`SELECT resolution FROM playback_events WHERE video_id = ? AND event_type = 'playback'`, testVideoID)
	if err := row.Scan(&resolution); err != nil {
		t.Fatalf("erro ao buscar evento: %v", err)
	}
	if resolution != nil {
		t.Errorf("esperava resolution NULL para evento de master.m3u8, obteve %v", resolution)
	}
}

func TestStaticHandler_RecordsSegmentDownloadEvent(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)

	insertVideo(t, database, testVideoID, "ready", testTag)
	writeFile(t, filepath.Join(cfg.MediaDir, testTag, testVideoID, "720", "0.ts"), "TS_DATA")

	onDone, wait := awaitStats(1)
	h := NewStaticHandler(cfg, database)
	h.onStatsRecorded = onDone

	req := httptest.NewRequest(http.MethodGet, "/video/"+testTag+"/"+testVideoID+"/720/0.ts", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 14; Pixel 8)")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtido %d", rec.Code)
	}
	wait()

	result, err := models.AggregateByResolution(database, testVideoID)
	if err != nil {
		t.Fatalf("AggregateByResolution falhou: %v", err)
	}
	if result[720] != 1 {
		t.Fatalf("esperava 1 evento de download_segment com resolution=720, obteve %d", result[720])
	}

	count, err := models.CountEventsByType(database, "download_segment")
	if err != nil {
		t.Fatalf("CountEventsByType falhou: %v", err)
	}
	if count != 1 {
		t.Fatalf("esperava 1 evento \"download_segment\", obteve %d", count)
	}
}

func TestStaticHandler_DoesNotRecordOnAuthFailure(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)

	h := NewStaticHandler(cfg, database)
	req := httptest.NewRequest(http.MethodGet, "/video/"+testTag+"/"+testVideoID+"/9999/0.ts", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("esperado 400, obtido %d", rec.Code)
	}

	count, err := models.CountEventsByType(database, "download_segment")
	if err != nil {
		t.Fatalf("CountEventsByType falhou: %v", err)
	}
	if count != 0 {
		t.Fatalf("esperava 0 eventos após falha de validação, obteve %d", count)
	}
}

func TestUploadCompleteRecordsEvent(t *testing.T) {
	database := newTestDB(t)
	insertVideo(t, database, testVideoID, "ready", testTag)

	if err := models.RecordEvent(database, testVideoID, "upload_complete", nil, ""); err != nil {
		t.Fatalf("RecordEvent falhou: %v", err)
	}

	count, err := models.CountEventsByType(database, "upload_complete")
	if err != nil {
		t.Fatalf("CountEventsByType falhou: %v", err)
	}
	if count != 1 {
		t.Fatalf("esperava 1 evento \"upload_complete\", obteve %d", count)
	}
}
