package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/klawdyo/streamedia/internal/auth"
)

// TestStatusRoute_ValidRequest testa uma requisição válida ao endpoint /api/status/{video_id}.
func TestStatusRoute_ValidRequest(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)
	insertVideo(t, database, testVideoID, "ready")

	h := NewStatusHandler(cfg, database)

	// Gera o HMAC correto para o video_id
	hmac := auth.SignBackendRequest(testSecret, []byte(testVideoID))

	req := httptest.NewRequest(http.MethodGet, "/api/status/"+testVideoID, nil)
	req.Header.Set("X-Status-Auth", hmac)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtido %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp StatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("erro ao desserializar resposta JSON: %v", err)
	}

	if resp.VideoID != testVideoID {
		t.Fatalf("esperado video_id %q, obtido %q", testVideoID, resp.VideoID)
	}

	if resp.Status != "ready" {
		t.Fatalf("esperado status 'ready', obtido %q", resp.Status)
	}
}

// TestStatusRoute_InvalidAuth testa rejeição com header X-Status-Auth inválido.
func TestStatusRoute_InvalidAuth(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)
	insertVideo(t, database, testVideoID, "ready")

	h := NewStatusHandler(cfg, database)

	// HMAC inválido
	req := httptest.NewRequest(http.MethodGet, "/api/status/"+testVideoID, nil)
	req.Header.Set("X-Status-Auth", "deadbeef")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401, obtido %d", rec.Code)
	}
}

// TestStatusRoute_InvalidAuth_MissingHeader testa rejeição sem header X-Status-Auth.
func TestStatusRoute_InvalidAuth_MissingHeader(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)
	insertVideo(t, database, testVideoID, "ready")

	h := NewStatusHandler(cfg, database)

	// Sem header X-Status-Auth
	req := httptest.NewRequest(http.MethodGet, "/api/status/"+testVideoID, nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401, obtido %d", rec.Code)
	}
}

// TestStatusRoute_NotFound testa resposta 404 para video_id válido não encontrado.
func TestStatusRoute_NotFound(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t) // sem inserir o vídeo

	h := NewStatusHandler(cfg, database)

	// HMAC correto para um video_id válido (mas não no banco)
	hmac := auth.SignBackendRequest(testSecret, []byte(testVideoID))

	req := httptest.NewRequest(http.MethodGet, "/api/status/"+testVideoID, nil)
	req.Header.Set("X-Status-Auth", hmac)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("esperado 404, obtido %d", rec.Code)
	}
}

// TestStatusRoute_InvalidVideoID testa resposta 400 para video_id inválido (não é UUID).
func TestStatusRoute_InvalidVideoID(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)

	h := NewStatusHandler(cfg, database)

	invalidID := "not-a-uuid"
	hmac := auth.SignBackendRequest(testSecret, []byte(invalidID))

	req := httptest.NewRequest(http.MethodGet, "/api/status/"+invalidID, nil)
	req.Header.Set("X-Status-Auth", hmac)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("esperado 400, obtido %d", rec.Code)
	}
}

// TestStatusRoute_ResponseFields testa os campos da resposta para status="failed_transcode".
func TestStatusRoute_ResponseFields(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)

	// Insere vídeo com status "failed_transcode" e error_message.
	_, err := database.Exec(
		"INSERT INTO videos (video_id, status, error_message) VALUES (?, ?, ?)",
		testVideoID, "failed_transcode", "something went wrong",
	)
	if err != nil {
		t.Fatalf("erro ao inserir vídeo: %v", err)
	}

	h := NewStatusHandler(cfg, database)

	hmac := auth.SignBackendRequest(testSecret, []byte(testVideoID))

	req := httptest.NewRequest(http.MethodGet, "/api/status/"+testVideoID, nil)
	req.Header.Set("X-Status-Auth", hmac)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtido %d", rec.Code)
	}

	var resp StatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("erro ao desserializar resposta JSON: %v", err)
	}

	if resp.Status != "failed_transcode" {
		t.Fatalf("esperado status 'failed_transcode', obtido %q", resp.Status)
	}

	if resp.ErrorMessage == nil || *resp.ErrorMessage != "something went wrong" {
		t.Fatalf("esperado error_message 'something went wrong', obtido %v", resp.ErrorMessage)
	}
}

// TestStatusRoute_ResolutionsDeserialized testa que as resolutions são retornadas como array JSON.
func TestStatusRoute_ResolutionsDeserialized(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)

	// Insere vídeo com status "ready" e resolutions=[480, 720]
	_, err := database.Exec(
		"INSERT INTO videos (video_id, status, resolutions) VALUES (?, ?, ?)",
		testVideoID, "ready", "[480,720]",
	)
	if err != nil {
		t.Fatalf("erro ao inserir vídeo: %v", err)
	}

	h := NewStatusHandler(cfg, database)

	hmac := auth.SignBackendRequest(testSecret, []byte(testVideoID))

	req := httptest.NewRequest(http.MethodGet, "/api/status/"+testVideoID, nil)
	req.Header.Set("X-Status-Auth", hmac)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtido %d", rec.Code)
	}

	var resp StatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("erro ao desserializar resposta JSON: %v", err)
	}

	if len(resp.Resolutions) != 2 {
		t.Fatalf("esperado 2 resolutions, obtido %d", len(resp.Resolutions))
	}

	if resp.Resolutions[0] != 480 || resp.Resolutions[1] != 720 {
		t.Fatalf("esperado [480, 720], obtido %v", resp.Resolutions)
	}
}

// TestStatusRoute_DurationSNilWhenZero testa que duration_s é nil quando duration_s=0.
func TestStatusRoute_DurationSNilWhenZero(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)

	// Insere vídeo com status "transcoding" (sem duration_s)
	_, err := database.Exec(
		"INSERT INTO videos (video_id, status) VALUES (?, ?)",
		testVideoID, "transcoding",
	)
	if err != nil {
		t.Fatalf("erro ao inserir vídeo: %v", err)
	}

	h := NewStatusHandler(cfg, database)

	hmac := auth.SignBackendRequest(testSecret, []byte(testVideoID))

	req := httptest.NewRequest(http.MethodGet, "/api/status/"+testVideoID, nil)
	req.Header.Set("X-Status-Auth", hmac)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtido %d", rec.Code)
	}

	var resp StatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("erro ao desserializar resposta JSON: %v", err)
	}

	if resp.DurationS != nil {
		t.Fatalf("esperado duration_s nil, obtido %v", resp.DurationS)
	}
}

// TestStatusRoute_DurationSSetWhenNonZero testa que duration_s é preenchido quando > 0.
func TestStatusRoute_DurationSSetWhenNonZero(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)

	// Insere vídeo com status "ready" e duration_s=120
	_, err := database.Exec(
		"INSERT INTO videos (video_id, status, duration_s) VALUES (?, ?, ?)",
		testVideoID, "ready", 120,
	)
	if err != nil {
		t.Fatalf("erro ao inserir vídeo: %v", err)
	}

	h := NewStatusHandler(cfg, database)

	hmac := auth.SignBackendRequest(testSecret, []byte(testVideoID))

	req := httptest.NewRequest(http.MethodGet, "/api/status/"+testVideoID, nil)
	req.Header.Set("X-Status-Auth", hmac)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtido %d", rec.Code)
	}

	var resp StatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("erro ao desserializar resposta JSON: %v", err)
	}

	if resp.DurationS == nil || *resp.DurationS != 120 {
		t.Fatalf("esperado duration_s 120, obtido %v", resp.DurationS)
	}
}

// TestStatusRoute_ErrorMessageNilWhenEmpty testa que error_message é nil quando vazio.
func TestStatusRoute_ErrorMessageNilWhenEmpty(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)

	// Insere vídeo com status "ready" (sem error_message)
	_, err := database.Exec(
		"INSERT INTO videos (video_id, status) VALUES (?, ?)",
		testVideoID, "ready",
	)
	if err != nil {
		t.Fatalf("erro ao inserir vídeo: %v", err)
	}

	h := NewStatusHandler(cfg, database)

	hmac := auth.SignBackendRequest(testSecret, []byte(testVideoID))

	req := httptest.NewRequest(http.MethodGet, "/api/status/"+testVideoID, nil)
	req.Header.Set("X-Status-Auth", hmac)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtido %d", rec.Code)
	}

	var resp StatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("erro ao desserializar resposta JSON: %v", err)
	}

	if resp.ErrorMessage != nil {
		t.Fatalf("esperado error_message nil, obtido %v", resp.ErrorMessage)
	}
}

// TestStatusRoute_TranscodeAttemptsField testa que transcode_attempts é preenchido corretamente.
func TestStatusRoute_TranscodeAttemptsField(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)

	// Insere vídeo com transcode_attempts=3
	_, err := database.Exec(
		"INSERT INTO videos (video_id, status, transcode_attempts) VALUES (?, ?, ?)",
		testVideoID, "transcoding", 3,
	)
	if err != nil {
		t.Fatalf("erro ao inserir vídeo: %v", err)
	}

	h := NewStatusHandler(cfg, database)

	hmac := auth.SignBackendRequest(testSecret, []byte(testVideoID))

	req := httptest.NewRequest(http.MethodGet, "/api/status/"+testVideoID, nil)
	req.Header.Set("X-Status-Auth", hmac)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtido %d", rec.Code)
	}

	var resp StatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("erro ao desserializar resposta JSON: %v", err)
	}

	if resp.TranscodeAttempts != 3 {
		t.Fatalf("esperado transcode_attempts 3, obtido %d", resp.TranscodeAttempts)
	}
}
