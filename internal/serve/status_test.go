package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/klawdyo/streamedia/internal/apiresponse"
)

// Nota: a autenticação de /api/status passou a ser feita pelo middleware
// RootAuth no roteador (testado em internal/server e internal/admin). O
// StatusHandler em si não valida credencial — estes testes focam no conteúdo
// da resposta.

func decodeStatus(t *testing.T, rec *httptest.ResponseRecorder) StatusResponse {
	t.Helper()
	var env apiresponse.Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("erro ao desserializar resposta JSON: %v", err)
	}
	dataJSON, _ := json.Marshal(env.Data)
	var resp StatusResponse
	if err := json.Unmarshal(dataJSON, &resp); err != nil {
		t.Fatalf("erro ao desserializar data: %v", err)
	}
	return resp
}

func TestStatusRoute_ValidRequest(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)
	insertVideo(t, database, testVideoID, "ready", testTag)

	h := NewStatusHandler(cfg, database)
	req := httptest.NewRequest(http.MethodGet, "/api/status/"+testVideoID, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtido %d (body: %s)", rec.Code, rec.Body.String())
	}
	resp := decodeStatus(t, rec)
	if resp.VideoID != testVideoID {
		t.Fatalf("esperado video_id %q, obtido %q", testVideoID, resp.VideoID)
	}
	if resp.Status != "ready" {
		t.Fatalf("esperado status 'ready', obtido %q", resp.Status)
	}
}

func TestStatusRoute_NotFound(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t) // sem inserir o vídeo

	h := NewStatusHandler(cfg, database)
	req := httptest.NewRequest(http.MethodGet, "/api/status/"+testVideoID, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("esperado 404, obtido %d", rec.Code)
	}
}

func TestStatusRoute_InvalidVideoID(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)

	h := NewStatusHandler(cfg, database)
	req := httptest.NewRequest(http.MethodGet, "/api/status/not-a-uuid", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("esperado 400, obtido %d", rec.Code)
	}
}

func TestStatusRoute_ResponseFields(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)
	if _, err := database.Exec(
		"INSERT INTO videos (video_id, tag, status, error_message) VALUES (?, 'default', ?, ?)",
		testVideoID, "failed_transcode", "something went wrong",
	); err != nil {
		t.Fatalf("erro ao inserir vídeo: %v", err)
	}

	h := NewStatusHandler(cfg, database)
	req := httptest.NewRequest(http.MethodGet, "/api/status/"+testVideoID, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtido %d", rec.Code)
	}
	resp := decodeStatus(t, rec)
	if resp.Status != "failed_transcode" {
		t.Fatalf("esperado status 'failed_transcode', obtido %q", resp.Status)
	}
	if resp.ErrorMessage == nil || *resp.ErrorMessage != "something went wrong" {
		t.Fatalf("esperado error_message 'something went wrong', obtido %v", resp.ErrorMessage)
	}
}

func TestStatusRoute_ResolutionsDeserialized(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)
	if _, err := database.Exec(
		"INSERT INTO videos (video_id, tag, status, resolutions) VALUES (?, 'default', ?, ?)",
		testVideoID, "ready", "[480,720]",
	); err != nil {
		t.Fatalf("erro ao inserir vídeo: %v", err)
	}

	h := NewStatusHandler(cfg, database)
	req := httptest.NewRequest(http.MethodGet, "/api/status/"+testVideoID, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	resp := decodeStatus(t, rec)
	if len(resp.Resolutions) != 2 || resp.Resolutions[0] != 480 || resp.Resolutions[1] != 720 {
		t.Fatalf("esperado [480, 720], obtido %v", resp.Resolutions)
	}
}

func TestStatusRoute_DurationSNilWhenZero(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)
	insertVideo(t, database, testVideoID, "transcoding", testTag)

	h := NewStatusHandler(cfg, database)
	req := httptest.NewRequest(http.MethodGet, "/api/status/"+testVideoID, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	resp := decodeStatus(t, rec)
	if resp.DurationS != nil {
		t.Fatalf("esperado duration_s nil, obtido %v", resp.DurationS)
	}
}

func TestStatusRoute_DurationSSetWhenNonZero(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)
	if _, err := database.Exec(
		"INSERT INTO videos (video_id, tag, status, duration_s) VALUES (?, 'default', ?, ?)",
		testVideoID, "ready", 120,
	); err != nil {
		t.Fatalf("erro ao inserir vídeo: %v", err)
	}

	h := NewStatusHandler(cfg, database)
	req := httptest.NewRequest(http.MethodGet, "/api/status/"+testVideoID, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	resp := decodeStatus(t, rec)
	if resp.DurationS == nil || *resp.DurationS != 120 {
		t.Fatalf("esperado duration_s 120, obtido %v", resp.DurationS)
	}
}

func TestStatusRoute_ErrorMessageNilWhenEmpty(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)
	insertVideo(t, database, testVideoID, "ready", testTag)

	h := NewStatusHandler(cfg, database)
	req := httptest.NewRequest(http.MethodGet, "/api/status/"+testVideoID, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	resp := decodeStatus(t, rec)
	if resp.ErrorMessage != nil {
		t.Fatalf("esperado error_message nil, obtido %v", resp.ErrorMessage)
	}
}

func TestStatusRoute_HasThumbnailsFalseWhenNoneOnDisk(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)
	if _, err := database.Exec(
		"INSERT INTO videos (video_id, tag, status, resolutions) VALUES (?, 'default', ?, ?)",
		testVideoID, "ready", "[480,720]",
	); err != nil {
		t.Fatalf("erro ao inserir vídeo: %v", err)
	}

	h := NewStatusHandler(cfg, database)
	req := httptest.NewRequest(http.MethodGet, "/api/status/"+testVideoID, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	resp := decodeStatus(t, rec)
	if resp.HasThumbnails {
		t.Fatalf("esperado has_thumbnails false sem arquivos no disco")
	}
	if len(resp.Thumbnails) != 0 {
		t.Fatalf("esperado thumbnails vazio, obtido %v", resp.Thumbnails)
	}
}

func TestStatusRoute_ThumbnailsListedWhenOnDisk(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)
	if _, err := database.Exec(
		"INSERT INTO videos (video_id, tag, status, resolutions) VALUES (?, 'default', ?, ?)",
		testVideoID, "ready", "[480,720]",
	); err != nil {
		t.Fatalf("erro ao inserir vídeo: %v", err)
	}

	// Cria apenas o thumbnail de 480p no disco: has_thumbnails deve ficar true
	// e a lista deve conter só a resolução existente.
	writeFile(t, filepath.Join(cfg.MediaDir, testTag, testVideoID, "thumb_480.jpg"), "jpeg")

	h := NewStatusHandler(cfg, database)
	req := httptest.NewRequest(http.MethodGet, "/api/status/"+testVideoID, nil)
	req.Host = "example.com"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	resp := decodeStatus(t, rec)
	if !resp.HasThumbnails {
		t.Fatalf("esperado has_thumbnails true com thumbnail no disco")
	}
	url, ok := resp.Thumbnails["480"]
	if !ok {
		t.Fatalf("esperado thumbnail 480 na lista, obtido %v", resp.Thumbnails)
	}
	want := "http://example.com/video/default/" + testVideoID + "/thumb_480.jpg"
	if url != want {
		t.Fatalf("URL do thumbnail inesperada:\n  esperado %q\n  obtido   %q", want, url)
	}
	if _, ok := resp.Thumbnails["720"]; ok {
		t.Fatalf("não esperava thumbnail 720 (não existe no disco): %v", resp.Thumbnails)
	}
}

func TestStatusRoute_TranscodeAttemptsField(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)
	if _, err := database.Exec(
		"INSERT INTO videos (video_id, tag, status, transcode_attempts) VALUES (?, 'default', ?, ?)",
		testVideoID, "transcoding", 3,
	); err != nil {
		t.Fatalf("erro ao inserir vídeo: %v", err)
	}

	h := NewStatusHandler(cfg, database)
	req := httptest.NewRequest(http.MethodGet, "/api/status/"+testVideoID, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	resp := decodeStatus(t, rec)
	if resp.TranscodeAttempts != 3 {
		t.Fatalf("esperado transcode_attempts 3, obtido %d", resp.TranscodeAttempts)
	}
}
