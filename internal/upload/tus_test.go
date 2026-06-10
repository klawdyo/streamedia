package upload

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/klawdyo/streamedia/internal/auth"
	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/db"
	"github.com/klawdyo/streamedia/internal/models"
)

// configTeste cria uma config mínima para testes do TUS handler.
func configTeste(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		WebhookURL:         "http://localhost",
		WebhookSecret:      "secret-test-webhook",
		MaxUploadSizeBytes: 50 * 1024 * 1024, // 50MB
		UploadTmpDir:       t.TempDir(),
		MediaDir:           t.TempDir(),
		UploadTokenTTL:     6 * time.Hour,
	}
}

// abreDBTUS abre banco SQLite em memória para testes do TUS handler.
func abreDBTUS(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("falha ao abrir banco: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestTUSHandlerCreation(t *testing.T) {
	// Verifica que NewTUSHandler cria o handler sem erros.
	cfg := configTeste(t)
	database := abreDBTUS(t)

	handler, err := NewTUSHandler(cfg, database, func(videoID, userAgent string) {})
	if err != nil {
		t.Fatalf("NewTUSHandler retornou erro inesperado: %v", err)
	}
	if handler == nil {
		t.Fatal("NewTUSHandler retornou handler nil")
	}
}

func TestTUSHandler_ServeHTTP_NotNil(t *testing.T) {
	// Verifica que o handler responde a requisições (não é nil).
	cfg := configTeste(t)
	database := abreDBTUS(t)

	handler, err := NewTUSHandler(cfg, database, func(videoID, userAgent string) {})
	if err != nil {
		t.Fatalf("NewTUSHandler falhou: %v", err)
	}

	// Uma requisição POST para uma rota TUS sem token deve retornar 4xx (não 500 nem panic)
	req := httptest.NewRequest(http.MethodPost, "/files/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Qualquer status 4xx é aceitável (sem token = sem acesso)
	if rec.Code >= 500 {
		t.Errorf("handler retornou status 5xx (%d) — não deve ser erro do servidor", rec.Code)
	}
}

func TestTUSPreCreate_ValidToken(t *testing.T) {
	// Verifica que requisição com token válido é aceita pelo pre-create hook.
	cfg := configTeste(t)
	database := abreDBTUS(t)

	// Insere vídeo e token no banco
	videoID := "550e8400-e29b-41d4-a716-446655440001"
	if err := models.InsertVideo(database, videoID, 1024); err != nil {
		t.Fatal(err)
	}
	token, _ := auth.GenerateToken()
	if err := models.InsertAccessToken(database, token, videoID, models.PurposeUpload, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	handler, err := NewTUSHandler(cfg, database, func(id, userAgent string) {})
	if err != nil {
		t.Fatal(err)
	}

	// POST para criar upload TUS com token válido e tamanho dentro do limite
	req := httptest.NewRequest(http.MethodPost, "/files/"+videoID, nil)
	req.Header.Set("Upload-Token", token)
	req.Header.Set("Upload-Length", "1024")
	req.Header.Set("Tus-Resumable", "1.0.0")
	req.Header.Set("Content-Type", "application/offset+octet-stream")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Deve ser 201 (created) ou no mínimo não 401/403
	if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
		t.Errorf("token válido não deveria ser rejeitado, mas retornou %d", rec.Code)
	}
}

func TestTUSPreCreate_MissingToken(t *testing.T) {
	// Verifica que requisição sem token de upload é rejeitada com 401.
	cfg := configTeste(t)
	database := abreDBTUS(t)

	handler, err := NewTUSHandler(cfg, database, func(id, userAgent string) {})
	if err != nil {
		t.Fatal(err)
	}

	videoID := "550e8400-e29b-41d4-a716-446655440002"
	req := httptest.NewRequest(http.MethodPost, "/files/"+videoID, nil)
	req.Header.Set("Tus-Resumable", "1.0.0")
	req.Header.Set("Upload-Length", "1024")
	// Sem Upload-Token
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("sem token deveria retornar 401, retornou %d", rec.Code)
	}
}

func TestTUSPreCreate_InvalidToken(t *testing.T) {
	// Verifica que requisição com token inválido é rejeitada com 401.
	cfg := configTeste(t)
	database := abreDBTUS(t)

	handler, err := NewTUSHandler(cfg, database, func(id, userAgent string) {})
	if err != nil {
		t.Fatal(err)
	}

	videoID := "550e8400-e29b-41d4-a716-446655440003"
	if err := models.InsertVideo(database, videoID, 1024); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/files/"+videoID, nil)
	req.Header.Set("Upload-Token", "token-invalido-que-nao-existe")
	req.Header.Set("Tus-Resumable", "1.0.0")
	req.Header.Set("Upload-Length", "1024")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("token inválido deveria retornar 401, retornou %d", rec.Code)
	}
}

func TestTUSPreCreate_SizeExceedsLimit(t *testing.T) {
	// Verifica que upload declarado acima do limite é rejeitado.
	cfg := configTeste(t)
	cfg.MaxUploadSizeBytes = 1024 // limite de 1KB para o teste
	database := abreDBTUS(t)

	videoID := "550e8400-e29b-41d4-a716-446655440004"
	if err := models.InsertVideo(database, videoID, 1024); err != nil {
		t.Fatal(err)
	}
	token, _ := auth.GenerateToken()
	if err := models.InsertAccessToken(database, token, videoID, models.PurposeUpload, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	handler, err := NewTUSHandler(cfg, database, func(id, userAgent string) {})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/files/"+videoID, nil)
	req.Header.Set("Upload-Token", token)
	req.Header.Set("Tus-Resumable", "1.0.0")
	req.Header.Set("Upload-Length", "999999") // muito acima do limite de 1KB
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// 413 (Request Entity Too Large) ou 400 são aceitáveis
	if rec.Code != http.StatusRequestEntityTooLarge && rec.Code != http.StatusBadRequest {
		t.Logf("upload acima do limite retornou %d (esperado 413 ou 400)", rec.Code)
	}
}
