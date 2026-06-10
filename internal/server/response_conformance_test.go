// Testes de conformidade do envelope de resposta — garantem que TODA rota
// JSON da API segue o formato {error, message, data, status_code} e que
// rotas não-API (HLS, docs, métricas) não são afetadas pela migração.
//
// Esta suíte é o "pente fino" permanente: qualquer rota nova que escreva JSON
// manualmente vai falhar aqui, forçando o desenvolvedor a usar o envelope.
package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/klawdyo/streamedia/internal/apiresponse"
	"github.com/klawdyo/streamedia/internal/middleware"
	"github.com/klawdyo/streamedia/internal/models"
)

// validUUID é um UUID usado nos testes que precisam de um video_id válido.
const validUUID = "550e8400-e29b-4100-8716-446655440000"

// uuidUploadInit é um UUID separado para o teste de upload/init, evitando
// conflito com outros testes que inserem vídeos no mesmo banco.
const uuidUploadInit = "550e8400-e29b-4100-8716-446655440001"

// uuidAdminVideo é o UUID usado pelo teste de admin/videos.
const uuidAdminVideo = "550e8400-e29b-4100-8716-446655440002"

// decodeEnvelope lê o corpo da resposta como apiresponse.Envelope.
func decodeEnvelope(t *testing.T, rec *httptest.ResponseRecorder) apiresponse.Envelope {
	t.Helper()
	var env apiresponse.Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("corpo não é um envelope JSON válido: %v (body: %s)", err, rec.Body.String())
	}
	return env
}

// TestAllJSONRoutes_ErrorResponses_FollowEnvelope verifica que toda rota JSON
// da API, quando recebe uma requisição sem autenticação ou com dados inválidos,
// responde no envelope padrão: error=true, data=nil, status_code==header.
func TestAllJSONRoutes_ErrorResponses_FollowEnvelope(t *testing.T) {
	router, _ := newTestRouter(t, newTestConfig(t))

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		header map[string]string
	}{
		{
			name:   "POST /api/upload/init sem auth",
			method: http.MethodPost,
			path:   "/api/upload/init",
			body:   `{"tag":"t","video_id":"` + validUUID + `","declared_size_bytes":1024}`,
		},
		{
			name:   "POST /api/play/init sem auth",
			method: http.MethodPost,
			path:   "/api/play/init",
			body:   `{"video_id":"` + validUUID + `"}`,
		},
		{
			name:   "GET /api/status sem auth",
			method: http.MethodGet,
			path:   "/api/status/" + validUUID,
		},
		{
			name:   "GET /admin/videos sem auth",
			method: http.MethodGet,
			path:   "/admin/videos",
		},
		{
			name:   "GET /admin/queue sem auth",
			method: http.MethodGet,
			path:   "/admin/queue",
		},
		{
			name:   "GET /admin/stats sem auth",
			method: http.MethodGet,
			path:   "/admin/stats",
		},
		{
			name:   "DELETE /admin/videos/{id} sem auth",
			method: http.MethodDelete,
			path:   "/admin/videos/" + validUUID,
		},
		{
			// Endurecimento de segurança: /docs deixou de ser público e agora
			// exige token de admin — sem auth deve responder 401 no envelope.
			name:   "GET /docs sem auth",
			method: http.MethodGet,
			path:   "/docs",
		},
		{
			name:   "GET /docs/openapi.json sem auth",
			method: http.MethodGet,
			path:   "/docs/openapi.json",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var req *http.Request
			if tc.body != "" {
				req = httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			} else {
				req = httptest.NewRequest(tc.method, tc.path, nil)
			}
			for k, v := range tc.header {
				req.Header.Set(k, v)
			}

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			env := decodeEnvelope(t, rec)

			// Em erro, o envelope deve ter error=true.
			if !env.Error {
				t.Errorf("esperado error=true, obtido %v", env.Error)
			}

			// message não pode ser vazia.
			if env.Message == "" {
				t.Error("message está vazia")
			}

			// data deve ser nil (null no JSON).
			if env.Data != nil {
				t.Errorf("esperado data=nil em erro, obtido %v", env.Data)
			}

			// status_code no corpo deve bater com o header HTTP.
			if env.StatusCode != rec.Code {
				t.Errorf("status_code no envelope (%d) != status do header (%d)",
					env.StatusCode, rec.Code)
			}

			// Content-Type deve incluir charset utf-8.
			ct := rec.Header().Get("Content-Type")
			if !strings.Contains(ct, "application/json") {
				t.Errorf("Content-Type deveria ser JSON, obtido %q", ct)
			}
		})
	}
}

// TestAllJSONRoutes_SuccessResponses_FollowEnvelope verifica que rotas de API,
// quando bem-sucedidas, respondem no envelope: error=false, message="ok",
// status_code==header, e data contém o payload esperado.
func TestAllJSONRoutes_SuccessResponses_FollowEnvelope(t *testing.T) {
	cfg := newTestConfig(t)
	router, db := newTestRouter(t, cfg)

	// --- Health check ---
	t.Run("GET /healthz", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("esperado 200, obtido %d", rec.Code)
		}

		env := decodeEnvelope(t, rec)

		if env.Error != false {
			t.Errorf("esperado error=false, obtido %v", env.Error)
		}
		if env.Message != "ok" {
			t.Errorf("esperado message='ok', obtido '%s'", env.Message)
		}
		if env.StatusCode != http.StatusOK {
			t.Errorf("esperado status_code=200, obtido %d", env.StatusCode)
		}
		if env.Data == nil {
			t.Error("esperado data não-nil (status='ok')")
		}
	})

	// --- Upload init com ROOT_TOKEN válido ---
	t.Run("POST /api/upload/init com auth", func(t *testing.T) {
		body := `{"tag":"conformance","video_id":"` + uuidUploadInit + `","declared_size_bytes":1024}`

		req := httptest.NewRequest(http.MethodPost, "/api/upload/init", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+cfg.RootToken)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("esperado 200, obtido %d (body: %s)", rec.Code, rec.Body.String())
		}

		env := decodeEnvelope(t, rec)

		if env.Error != false {
			t.Errorf("esperado error=false, obtido %v", env.Error)
		}
		if env.Message != "ok" {
			t.Errorf("esperado message='ok', obtido '%s'", env.Message)
		}
		if env.StatusCode != http.StatusOK {
			t.Errorf("esperado status_code=200, obtido %d", env.StatusCode)
		}

		// data deve conter upload_url, token e video_id.
		data, ok := env.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("data deveria ser um objeto, obtido %T", env.Data)
		}
		if uploadURL, _ := data["upload_url"].(string); uploadURL == "" {
			t.Error("data.upload_url está vazio")
		}
		if token, _ := data["token"].(string); token == "" {
			t.Error("data.token está vazio")
		}
	})

	// --- Admin com token Bearer válido ---
	t.Run("GET /admin/videos com auth", func(t *testing.T) {
		// Insere um vídeo para a listagem não ser vazia.
		insertTestVideo(t, db, uuidAdminVideo, models.StatusReady)

		req := httptest.NewRequest(http.MethodGet, "/admin/videos", nil)
		req.Header.Set("Authorization", "Bearer "+cfg.RootToken)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("esperado 200, obtido %d (body: %s)", rec.Code, rec.Body.String())
		}

		env := decodeEnvelope(t, rec)

		if env.Error != false {
			t.Errorf("esperado error=false, obtido %v", env.Error)
		}
		if env.Message != "ok" {
			t.Errorf("esperado message='ok', obtido '%s'", env.Message)
		}
		if env.StatusCode != http.StatusOK {
			t.Errorf("esperado status_code=200, obtido %d", env.StatusCode)
		}
		if env.Data == nil {
			t.Error("esperado data não-nil")
		}
	})

	t.Run("GET /admin/queue com auth", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/queue", nil)
		req.Header.Set("Authorization", "Bearer "+cfg.RootToken)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("esperado 200, obtido %d (body: %s)", rec.Code, rec.Body.String())
		}

		env := decodeEnvelope(t, rec)

		if env.Error != false {
			t.Errorf("esperado error=false, obtido %v", env.Error)
		}
		if env.Message != "ok" {
			t.Errorf("esperado message='ok', obtido '%s'", env.Message)
		}
		if env.StatusCode != http.StatusOK {
			t.Errorf("esperado status_code=200, obtido %d", env.StatusCode)
		}
	})

	t.Run("GET /admin/stats com auth", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/stats", nil)
		req.Header.Set("Authorization", "Bearer "+cfg.RootToken)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("esperado 200, obtido %d (body: %s)", rec.Code, rec.Body.String())
		}

		env := decodeEnvelope(t, rec)

		if env.Error != false {
			t.Errorf("esperado error=false, obtido %v", env.Error)
		}
		if env.Message != "ok" {
			t.Errorf("esperado message='ok', obtido '%s'", env.Message)
		}
		if env.StatusCode != http.StatusOK {
			t.Errorf("esperado status_code=200, obtido %d", env.StatusCode)
		}
	})
}

// TestUnhandledPanic_ReturnsStandardErrorEnvelope verifica que panics não
// tratados são capturados pelo RecoveryMiddleware e respondem com 500 no
// envelope padrão, sem vazar detalhes do panic ao cliente.
func TestUnhandledPanic_ReturnsStandardErrorEnvelope(t *testing.T) {
	// Cria um roteador chi com o RecoveryMiddleware do projeto para testar
	// a recuperação de panics isoladamente.
	r := chi.NewRouter()
	r.Use(middleware.RecoveryMiddleware)

	// Rota que causa panic deliberadamente — simula um nil pointer dereference
	// ou qualquer outro panic que escape de um handler real.
	r.Get("/panic-test", func(w http.ResponseWriter, _ *http.Request) {
		panic("boom — falha simulada em handler")
	})

	// Rota de health check — usada para confirmar que o servidor continua
	// funcionando após o panic (não derrubou o processo).
	r.Get("/healthz-after-panic", func(w http.ResponseWriter, _ *http.Request) {
		apiresponse.Success(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// 1. Dispara a rota que dá panic.
	req := httptest.NewRequest(http.MethodGet, "/panic-test", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	// Deve retornar 500 (Internal Server Error).
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("esperado 500 após panic, obtido %d (body: %s)", rec.Code, rec.Body.String())
	}

	env := decodeEnvelope(t, rec)

	// Deve ser um envelope de erro.
	if !env.Error {
		t.Errorf("esperado error=true, obtido %v", env.Error)
	}

	// A mensagem deve ser genérica — NUNCA o texto original do panic.
	if strings.Contains(env.Message, "boom") {
		t.Errorf("mensagem de erro não deveria conter o texto do panic, obtido '%s'", env.Message)
	}
	if strings.Contains(env.Message, "panic") {
		t.Errorf("mensagem de erro não deveria conter 'panic', obtido '%s'", env.Message)
	}

	// mensagem não pode ser vazia.
	if env.Message == "" {
		t.Error("message está vazia")
	}

	// data deve ser nil.
	if env.Data != nil {
		t.Errorf("esperado data=nil, obtido %v", env.Data)
	}

	// status_code no envelope deve ser 500.
	if env.StatusCode != http.StatusInternalServerError {
		t.Errorf("esperado status_code=500, obtido %d", env.StatusCode)
	}

	// 2. Confirma que o servidor continua funcionando após o panic.
	req2 := httptest.NewRequest(http.MethodGet, "/healthz-after-panic", nil)
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("servidor não respondeu após panic: esperado 200, obtido %d", rec2.Code)
	}
}

// TestNonAPIRoutes_NotForcedIntoEnvelope confirma que rotas não-API (docs,
// conteúdo HLS) continuam respondendo no formato original — o envelope
// não foi imposto indevidamente a conteúdo que não é JSON de API.
func TestNonAPIRoutes_NotForcedIntoEnvelope(t *testing.T) {
	cfg := newTestConfig(t)
	router, _ := newTestRouter(t, cfg)

	// --- /docs/ (Scalar UI — HTML) ---
	// /docs agora exige token de admin (endurecimento de segurança): manda o
	// Authorization Bearer para validar que, autenticado, o conteúdo é HTML
	// cru (não forçado ao envelope JSON da API).
	t.Run("GET /docs/ retorna HTML", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/docs/", nil)
		req.Header.Set("Authorization", "Bearer "+cfg.RootToken)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("esperado 200, obtido %d", rec.Code)
		}

		ct := rec.Header().Get("Content-Type")
		if !strings.Contains(ct, "text/html") {
			t.Errorf("esperado Content-Type text/html, obtido %q", ct)
		}
	})

	// --- /docs/openapi.json (spec OpenAPI) ---
	t.Run("GET /docs/openapi.json retorna spec", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/docs/openapi.json", nil)
		req.Header.Set("Authorization", "Bearer "+cfg.RootToken)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("esperado 200, obtido %d", rec.Code)
		}

		ct := rec.Header().Get("Content-Type")
		if !strings.Contains(ct, "application/json") {
			t.Errorf("esperado Content-Type application/json, obtido %q", ct)
		}
	})

	// --- Master HLS — sem token deve retornar erro NO ENVELOPE ---
	// (porque é um erro da API JSON, mesmo que a rota sirva conteúdo binário
	// em caso de sucesso — ver seção "Escopo" da T45)
	t.Run("GET master.m3u8 sem token retorna erro no envelope", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/video/default/"+validUUID+".m3u8", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		// Erro de auth em rota HLS ainda deve seguir o envelope.
		env := decodeEnvelope(t, rec)

		if !env.Error {
			t.Errorf("esperado error=true, obtido %v", env.Error)
		}
		if env.Message == "" {
			t.Error("message está vazia")
		}
		if env.Data != nil {
			t.Errorf("esperado data=nil, obtido %v", env.Data)
		}
	})

	// --- Static HLS — segmento inexistente retorna erro no envelope ---
	t.Run("GET segmento .ts inexistente retorna erro no envelope", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/video/default/"+validUUID+"/480/0.ts", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		// O erro (404 ou 400) deve vir no envelope.
		env := decodeEnvelope(t, rec)

		if !env.Error {
			t.Errorf("esperado error=true, obtido %v", env.Error)
		}
		if env.Message == "" {
			t.Error("message está vazia")
		}
	})
}

// insertTestVideo insere um vídeo de teste no banco para os testes de
// conformidade que precisam de dados pré-existentes (listagem, status).
func insertTestVideo(t *testing.T, db *sql.DB, videoID string, status models.VideoStatus) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO videos (video_id, status, created_at, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		videoID, string(status),
	)
	if err != nil {
		t.Fatalf("erro ao inserir vídeo de teste %s: %v", videoID, err)
	}
}
