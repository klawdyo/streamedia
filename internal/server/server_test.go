package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/klawdyo/streamedia/internal/apiresponse"
	"github.com/klawdyo/streamedia/internal/auth"
	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/db"
	"github.com/klawdyo/streamedia/internal/middleware"
	"github.com/klawdyo/streamedia/internal/transcode"
	"github.com/klawdyo/streamedia/internal/webhook"
)

// newTestConfig devolve uma config mínima e válida para os testes, usando
// diretórios temporários para evitar tocar no sistema de arquivos real.
func newTestConfig(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		UploadTmpDir:         t.TempDir(),
		MediaDir:             t.TempDir(),
		SQLitePath:           ":memory:",
		UploadTokenSecret:    "test-secret",
		WebhookURL:           "http://example.com/webhook",
		WebhookSecret:        "webhook-secret",
		AdminToken:           "admin-token",
		RateLimitPerMin:      100,
		MaxUploadSizeBytes:   1 << 30,
		MaxTranscodeAttempts: 3,
		TranscodeWorkers:     1,
		QueueMaxSize:         10,
		UploadTokenTTL:       6 * time.Hour,
		PlayTokenMaxTTL:      24 * time.Hour,
	}
}

// newTestRouter monta um roteador com um banco em memória e uma fila com worker
// no-op, devolvendo também o banco e a config para os testes que precisarem.
func newTestRouter(t *testing.T, cfg *config.Config) (http.Handler, *sql.DB) {
	t.Helper()
	database, err := db.Open(cfg.SQLitePath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	wc := webhook.NewClient(cfg, database)
	// Worker no-op: os testes não precisam transcodificar de verdade.
	queue := transcode.NewQueue(cfg, database, func(string) error { return nil })

	return NewRouter(cfg, database, queue, wc), database
}

// TestHealthz verifica que /healthz responde 200 com "ok".
func TestHealthz(t *testing.T) {
	router, _ := newTestRouter(t, newTestConfig(t))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtido %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "ok") {
		t.Fatalf("corpo deveria conter \"ok\", obtido %q", rec.Body.String())
	}
}

// TestRouteNotFound verifica que uma rota não registrada devolve 404.
func TestRouteNotFound(t *testing.T) {
	router, _ := newTestRouter(t, newTestConfig(t))

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("esperado 404, obtido %d", rec.Code)
	}
}

// TestAllRoutesRegistered confirma que as rotas existem (não retornam 404),
// mesmo quando a autenticação falha (401).
func TestAllRoutesRegistered(t *testing.T) {
	router, _ := newTestRouter(t, newTestConfig(t))

	const validUUID = "550e8400-e29b-4100-8716-446655440000"

	cases := []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{"upload init sem auth", http.MethodPost, "/upload/init", http.StatusUnauthorized},
		{"status sem auth", http.MethodGet, "/api/status/" + validUUID, http.StatusUnauthorized},
		{"admin videos sem auth", http.MethodGet, "/admin/videos", http.StatusUnauthorized},
		{"healthz", http.MethodGet, "/healthz", http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code == http.StatusNotFound {
				t.Fatalf("rota %s %s não registrada (404)", tc.method, tc.path)
			}
			if rec.Code != tc.want {
				t.Fatalf("%s %s: esperado %d, obtido %d", tc.method, tc.path, tc.want, rec.Code)
			}
		})
	}
}

// TestUploadInitE2E faz um POST /upload/init completo com HMAC válido e
// verifica que a resposta 200 traz upload_url e token.
func TestUploadInitE2E(t *testing.T) {
	cfg := newTestConfig(t)
	router, _ := newTestRouter(t, cfg)

	const validUUID = "550e8400-e29b-4100-8716-446655440000"
	body := []byte(`{"video_id":"` + validUUID + `","declared_size_bytes":1024}`)
	sig := auth.SignBackendRequest(cfg.UploadTokenSecret, body)

	req := httptest.NewRequest(http.MethodPost, "/upload/init", strings.NewReader(string(body)))
	req.Header.Set("X-Upload-Auth", sig)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtido %d (corpo: %s)", rec.Code, rec.Body.String())
	}

	var env apiresponse.Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("resposta não é JSON válido: %v", err)
	}
	data, ok := env.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("data deveria ser um objeto, obtido %T", env.Data)
	}
	uploadURL, _ := data["upload_url"].(string)
	token, _ := data["token"].(string)
	if uploadURL == "" {
		t.Errorf("esperado upload_url não vazio, obtido %q", uploadURL)
	}
	if token == "" {
		t.Errorf("esperado token não vazio, obtido %q", token)
	}
	if !strings.Contains(uploadURL, validUUID) {
		t.Errorf("upload_url deveria conter o video_id, obtido %q", uploadURL)
	}
}

// TestRateLimitApplied confirma que o rate limiter está conectado ao roteador:
// com limite baixo, muitas requisições eventualmente recebem 429.
func TestRateLimitApplied(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.RateLimitPerMin = 2
	router, _ := newTestRouter(t, cfg)

	var got429 bool
	for i := 0; i < 50; i++ {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		// IP fixo para cair sempre no mesmo bucket do limitador.
		req.RemoteAddr = "10.0.0.1:1234"
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code == http.StatusTooManyRequests {
			got429 = true
			break
		}
	}

	if !got429 {
		t.Fatalf("esperado 429 após exceder o limite de %d/min", cfg.RateLimitPerMin)
	}
}

// Garante que o pacote middleware é importado mesmo se não usado diretamente
// em asserts (o limiter é exercido via roteador). Mantém o import explícito.
var _ = middleware.NewRateLimiter
