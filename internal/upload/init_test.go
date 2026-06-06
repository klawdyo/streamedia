package upload

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/klawdyo/streamedia/internal/auth"
	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/db"
	"github.com/klawdyo/streamedia/internal/models"
)

// configInit cria config mínima para testes da rota de init.
func configInit(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		UploadTokenSecret:  "secret-init-test",
		WebhookURL:         "http://localhost",
		WebhookSecret:      "wh-secret",
		MaxUploadSizeBytes: 50 * 1024 * 1024,
		UploadTokenTTL:     6 * time.Hour,
		UploadTmpDir:       t.TempDir(),
	}
}

// abreDBInit abre banco SQLite em memória para testes de init.
func abreDBInit(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("falha ao abrir banco: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

// fazRequestInit constrói um POST /upload/init com HMAC correto.
func fazRequestInit(t *testing.T, cfg *config.Config, body []byte) *http.Request {
	t.Helper()
	sig := auth.SignBackendRequest(cfg.UploadTokenSecret, body)
	req := httptest.NewRequest(http.MethodPost, "/upload/init", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Upload-Auth", sig)
	return req
}

func TestUploadInit_Success(t *testing.T) {
	// Verifica que POST /upload/init com dados válidos retorna 200 com upload_url e token.
	cfg := configInit(t)
	database := abreDBInit(t)
	handler := NewInitHandler(cfg, database)

	body := []byte(`{"video_id":"550e8400-e29b-41d4-a716-446655440010","declared_size_bytes":1024000}`)
	req := fazRequestInit(t, cfg, body)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200, obteve %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("falha ao decodificar resposta: %v", err)
	}
	if resp["upload_url"] == "" {
		t.Error("upload_url ausente na resposta")
	}
	if resp["token"] == "" {
		t.Error("token ausente na resposta")
	}
}

func TestUploadInit_InvalidHMAC(t *testing.T) {
	// Verifica que requisição com HMAC errado é rejeitada com 401.
	cfg := configInit(t)
	database := abreDBInit(t)
	handler := NewInitHandler(cfg, database)

	body := []byte(`{"video_id":"550e8400-e29b-41d4-a716-446655440011","declared_size_bytes":1024}`)
	req := httptest.NewRequest(http.MethodPost, "/upload/init", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Upload-Auth", "assinatura-errada-123")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("HMAC inválido deveria retornar 401, obteve %d", rec.Code)
	}
}

func TestUploadInit_MissingAuthHeader(t *testing.T) {
	// Verifica que requisição sem header X-Upload-Auth retorna 401.
	cfg := configInit(t)
	database := abreDBInit(t)
	handler := NewInitHandler(cfg, database)

	body := []byte(`{"video_id":"550e8400-e29b-41d4-a716-446655440012","declared_size_bytes":1024}`)
	req := httptest.NewRequest(http.MethodPost, "/upload/init", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// Sem X-Upload-Auth

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("ausência de header de auth deveria retornar 401, obteve %d", rec.Code)
	}
}

func TestUploadInit_InvalidVideoID_NotUUID(t *testing.T) {
	// Verifica que video_id que não é UUID v4 é rejeitado com 400.
	cfg := configInit(t)
	database := abreDBInit(t)
	handler := NewInitHandler(cfg, database)

	body := []byte(`{"video_id":"nao-e-um-uuid-valido","declared_size_bytes":1024}`)
	req := fazRequestInit(t, cfg, body)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("video_id inválido deveria retornar 400, obteve %d", rec.Code)
	}
}

func TestUploadInit_InvalidVideoID_PathTraversal(t *testing.T) {
	// Verifica que video_id com path traversal é rejeitado com 400.
	cfg := configInit(t)
	database := abreDBInit(t)
	handler := NewInitHandler(cfg, database)

	body := []byte(`{"video_id":"../etc/passwd","declared_size_bytes":1024}`)
	req := fazRequestInit(t, cfg, body)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("path traversal deveria retornar 400, obteve %d", rec.Code)
	}
}

func TestUploadInit_DuplicateVideoID(t *testing.T) {
	// Verifica que video_id já existente retorna 409 Conflict.
	cfg := configInit(t)
	database := abreDBInit(t)
	handler := NewInitHandler(cfg, database)

	// Insere o vídeo previamente no banco
	videoID := "550e8400-e29b-41d4-a716-446655440013"
	if err := models.InsertVideo(database, videoID, 1024); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(map[string]interface{}{
		"video_id":            videoID,
		"declared_size_bytes": 1024,
	})
	req := fazRequestInit(t, cfg, body)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("video_id duplicado deveria retornar 409, obteve %d", rec.Code)
	}
}

func TestUploadInit_ZeroSize(t *testing.T) {
	// Verifica que declared_size_bytes = 0 é rejeitado com 400.
	cfg := configInit(t)
	database := abreDBInit(t)
	handler := NewInitHandler(cfg, database)

	body := []byte(`{"video_id":"550e8400-e29b-41d4-a716-446655440014","declared_size_bytes":0}`)
	req := fazRequestInit(t, cfg, body)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("tamanho zero deveria retornar 400, obteve %d", rec.Code)
	}
}

func TestUploadInit_SizeExceedsLimit(t *testing.T) {
	// Verifica que declared_size_bytes acima do limite é rejeitado.
	cfg := configInit(t)
	cfg.MaxUploadSizeBytes = 1024 // limite de 1KB para o teste
	database := abreDBInit(t)
	handler := NewInitHandler(cfg, database)

	body := []byte(`{"video_id":"550e8400-e29b-41d4-a716-446655440015","declared_size_bytes":9999999}`)
	req := fazRequestInit(t, cfg, body)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Error("tamanho acima do limite não deveria retornar 200")
	}
}

func TestUploadInit_TokenStoredInDB(t *testing.T) {
	// Verifica que o token é gravado no banco com expiração no futuro.
	cfg := configInit(t)
	database := abreDBInit(t)
	handler := NewInitHandler(cfg, database)

	videoID := "550e8400-e29b-41d4-a716-446655440016"
	body, _ := json.Marshal(map[string]interface{}{
		"video_id":            videoID,
		"declared_size_bytes": 2048,
	})
	req := fazRequestInit(t, cfg, body)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200, obteve %d", rec.Code)
	}

	// Verifica que o token foi gravado no banco
	tok, err := models.GetUploadTokenByVideoID(database, videoID)
	if err != nil {
		t.Fatalf("token não encontrado no banco: %v", err)
	}
	if tok.IsExpired() {
		t.Error("token recém-criado não deveria estar expirado")
	}
}

func TestUploadInit_VideoCreatedInDB(t *testing.T) {
	// Verifica que o vídeo é criado no banco com status pending_upload.
	cfg := configInit(t)
	database := abreDBInit(t)
	handler := NewInitHandler(cfg, database)

	videoID := "550e8400-e29b-41d4-a716-446655440017"
	body, _ := json.Marshal(map[string]interface{}{
		"video_id":            videoID,
		"declared_size_bytes": 4096,
	})
	req := fazRequestInit(t, cfg, body)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200, obteve %d", rec.Code)
	}

	// Verifica que o vídeo foi criado com status correto
	video, err := models.GetVideo(database, videoID)
	if err != nil {
		t.Fatalf("vídeo não encontrado no banco: %v", err)
	}
	if video.Status != models.StatusPendingUpload {
		t.Errorf("status inicial: esperado %q, obtido %q", models.StatusPendingUpload, video.Status)
	}
	if video.DeclaredSizeBytes != 4096 {
		t.Errorf("declared_size_bytes: esperado 4096, obtido %d", video.DeclaredSizeBytes)
	}
}
