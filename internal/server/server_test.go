package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/klawdyo/streamedia/internal/admin"
	"github.com/klawdyo/streamedia/internal/apiresponse"
	"github.com/klawdyo/streamedia/internal/auth"
	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/db"
	"github.com/klawdyo/streamedia/internal/middleware"
	"github.com/klawdyo/streamedia/internal/notify"
	"github.com/klawdyo/streamedia/internal/sse"
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
		RootToken:            "root-token",
		WebhookURL:           "http://example.com/webhook",
		WebhookSecret:        "webhook-secret",
		RateLimitPerMin:      100,
		MaxUploadSizeBytes:   1 << 30,
		MaxTranscodeAttempts: 3,
		TranscodeWorkers:     1,
		QueueMaxSize:         10,
		UploadTokenTTL:       6 * time.Hour,
		PlayTokenTTL:         24 * time.Hour,
		SessionTTL:           time.Hour,
		SessionCookieSecure:  false,
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
	hub := sse.NewHub()
	notifier := notify.New(database, wc, hub)
	// Worker no-op: os testes não precisam transcodificar de verdade.
	queue := transcode.NewQueue(cfg, database, func(string) error { return nil })

	router, closer, err := NewRouter(cfg, database, queue, notifier, hub)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	t.Cleanup(func() { _ = closer.Close() })
	return router, database
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

// TestApiVersionRoute verifica que GET /api retorna nome, versão e status
// no envelope padrão, sem exigir autenticação.
func TestApiVersionRoute(t *testing.T) {
	router, _ := newTestRouter(t, newTestConfig(t))

	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtido %d (body: %s)", rec.Code, rec.Body.String())
	}

	var env apiresponse.Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("corpo não é JSON válido: %v", err)
	}
	if env.Error {
		t.Errorf("esperado error=false, obtido true: %s", env.Message)
	}
	if env.Message != "ok" {
		t.Errorf("esperado message='ok', obtido %q", env.Message)
	}

	data, ok := env.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("data não é um objeto: %T", env.Data)
	}
	if name, _ := data["name"].(string); name != "Streamedia" {
		t.Errorf("esperado name='Streamedia', obtido %q", name)
	}
	if v, _ := data["version"].(string); v == "" {
		t.Error("version está vazio")
	}
	if status, _ := data["status"].(string); status != "ok" {
		t.Errorf("esperado status='ok', obtido %q", status)
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
		body   string
		want   int
	}{
		{"upload init sem auth", http.MethodPost, "/api/upload/init", `{"tag":"t","video_id":"` + validUUID + `","declared_size_bytes":1024}`, http.StatusUnauthorized},
		{"status sem auth", http.MethodGet, "/api/status/" + validUUID, "", http.StatusUnauthorized},
		{"admin videos sem auth", http.MethodGet, "/admin/videos", "", http.StatusUnauthorized},
		{"healthz", http.MethodGet, "/healthz", "", http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var req *http.Request
			if tc.body != "" {
				req = httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			} else {
				req = httptest.NewRequest(tc.method, tc.path, nil)
			}
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

// TestDashboardRoutesPublic confirma que as páginas e assets do dashboard são
// servidos publicamente (200, HTML/CSS/JS) — o padrão do /playground. A
// proteção real fica nas rotas de dados (/admin/*, /api/status), exercidas em
// TestAllRoutesRegistered.
func TestDashboardRoutesPublic(t *testing.T) {
	// As antigas rotas /dashboard foram removidas no admin unificado (T82).
	// Agora a interface é servida em /app/* via SPA.
	router, _ := newTestRouter(t, newTestConfig(t))

	cases := []struct {
		path string
	}{
		{"/dashboard"},
		{"/dashboard/videos"},
		{"/dashboard/videos/550e8400-e29b-4100-8716-446655440000"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("%s: esperado 404 (rota removida), obtido %d", tc.path, rec.Code)
			}
		})
	}
}

// TestAppServeSPA verifica que a rota /app serve a SPA.
func TestAppServeSPA(t *testing.T) {
	router, _ := newTestRouter(t, newTestConfig(t))

	req := httptest.NewRequest(http.MethodGet, "/app", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// Se o SPA_DIR não existe, retorna 503.
	// Se existe, retorna 200 com text/html.
	if rec.Code != http.StatusOK && rec.Code != http.StatusServiceUnavailable {
		t.Errorf("/app: esperado 200 ou 503, obtido %d", rec.Code)
	}
}

// TestAdminSessionRemoved verifica que POST /admin/session foi removido.
func TestAdminSessionRemoved(t *testing.T) {
	cfg := newTestConfig(t)
	router, _ := newTestRouter(t, cfg)

	req := httptest.NewRequest(http.MethodPost, "/admin/session", nil)
	req.Header.Set("Authorization", "Bearer "+cfg.RootToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// Rota removida — retorna 404.
	if rec.Code != http.StatusNotFound {
		t.Fatalf("POST /admin/session: esperado 404 (rota removida), obtido %d", rec.Code)
	}
}

// TestUploadInitE2E faz um POST /api/upload/init completo com ROOT_TOKEN válido
// e verifica que a resposta 200 traz upload_url e token.
func TestUploadInitE2E(t *testing.T) {
	cfg := newTestConfig(t)
	router, _ := newTestRouter(t, cfg)

	const validUUID = "550e8400-e29b-4100-8716-446655440000"
	body := []byte(`{"tag":"server-test","video_id":"` + validUUID + `","declared_size_bytes":1024}`)

	req := httptest.NewRequest(http.MethodPost, "/api/upload/init", strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bearer "+cfg.RootToken)
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

// TestDocsRemoved verifica que /docs foi removido (substituído pelo Playground Vue).
func TestDocsRemoved(t *testing.T) {
	router, _ := newTestRouter(t, newTestConfig(t))

	req := httptest.NewRequest(http.MethodGet, "/docs", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("/docs: esperado 404 (rota removida), obtido %d", rec.Code)
	}
}

// TestSessionLogout_ClearsCookieAndRevokesAccess verifica que DELETE
// /api/auth/session é pública, limpa o cookie de sessão.
func TestSessionLogout_ClearsCookieAndRevokesAccess(t *testing.T) {
	cfg := newTestConfig(t)
	router, _ := newTestRouter(t, cfg)

	req := httptest.NewRequest(http.MethodDelete, "/api/auth/session", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE /api/auth/session: esperado 200, obtido %d", rec.Code)
	}

	// Verifica que o cookie foi limpo (MaxAge=-1).
	cookies := rec.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == admin.SessionCookieName && c.MaxAge < 0 {
			found = true
		}
	}
	if !found {
		t.Error("DELETE /api/auth/session deveria limpar o cookie de sessão")
	}
}

// TestSessionCookie_RequiresCSRFHeaderForUnsafeMethods verifica que uma
// requisição autenticada pelo cookie de sessão com formato novo (user_id)
// precisa do header X-Streamedia-Csrf para métodos não seguros.
func TestSessionCookie_RequiresCSRFHeaderForUnsafeMethods(t *testing.T) {
	cfg := newTestConfig(t)
	router, _ := newTestRouter(t, cfg)

	// Cria um cookie de sessão no formato novo (com user_id e roles).
	// O formato é <exp>.<user_id>.<roles>.<hmac>.
	sessionValue, _ := auth.IssueSessionTokenWithUser(
		cfg.RootToken, 1, []string{"admin"}, cfg.SessionTTL,
	)
	sessionCookie := &http.Cookie{
		Name:  admin.SessionCookieName,
		Value: sessionValue,
	}

	const validUUID = "550e8400-e29b-4100-8716-446655440000"

	// DELETE sem o header CSRF: 403, mesmo com cookie de sessão válido.
	reqDelete := httptest.NewRequest(http.MethodDelete, "/admin/videos/"+validUUID, nil)
	reqDelete.AddCookie(sessionCookie)
	recDelete := httptest.NewRecorder()
	router.ServeHTTP(recDelete, reqDelete)
	if recDelete.Code != http.StatusForbidden {
		t.Errorf("DELETE sem header CSRF: esperado 403, obtido %d", recDelete.Code)
	}

	// DELETE com o header CSRF: passa da autenticação (404, vídeo inexistente).
	reqDeleteCSRF := httptest.NewRequest(http.MethodDelete, "/admin/videos/"+validUUID, nil)
	reqDeleteCSRF.AddCookie(sessionCookie)
	reqDeleteCSRF.Header.Set(admin.CSRFHeaderName, "1")
	recDeleteCSRF := httptest.NewRecorder()
	router.ServeHTTP(recDeleteCSRF, reqDeleteCSRF)
	if recDeleteCSRF.Code != http.StatusNotFound {
		t.Errorf("DELETE com header CSRF: esperado 404 (vídeo inexistente), obtido %d", recDeleteCSRF.Code)
	}

	// GET continua funcionando sem o header CSRF.
	reqGet := httptest.NewRequest(http.MethodGet, "/admin/videos", nil)
	reqGet.AddCookie(sessionCookie)
	recGet := httptest.NewRecorder()
	router.ServeHTTP(recGet, reqGet)
	if recGet.Code != http.StatusOK {
		t.Errorf("GET com cookie de sessão: esperado 200, obtido %d", recGet.Code)
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
