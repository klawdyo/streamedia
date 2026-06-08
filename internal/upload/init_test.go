package upload

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/klawdyo/streamedia/internal/apiresponse"
	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/db"
	"github.com/klawdyo/streamedia/internal/models"
)

// configInit cria config mínima para testes da rota de init.
func configInit(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		UploadTokenSecret:   "secret-init-test",
		WebhookURL:          "http://localhost",
		WebhookSecret:       "wh-secret",
		MaxUploadSizeBytes:  50 * 1024 * 1024,
		UploadTokenTTL:      15 * time.Minute,
		UploadTmpDir:        t.TempDir(),
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

// fazRequestInit constrói um POST /upload/init autenticado com X-Project-Key.
func fazRequestInit(t *testing.T, body []byte, projectKey string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/upload/init", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project-Key", projectKey)
	return req
}

// setupProject cria um projeto de teste e retorna sua chave mestra.
func setupProject(t *testing.T, db *sql.DB) string {
	t.Helper()
	_, key, err := models.CreateProject(db, "Test Project")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	return key
}

func TestUploadInit_Success(t *testing.T) {
	// Verifica que POST /upload/init com dados válidos retorna 200 com upload_url e token.
	cfg := configInit(t)
	database := abreDBInit(t)
	handler := NewInitHandler(cfg, database)
	projectKey := setupProject(t, database)

	body := []byte(`{"video_id":"550e8400-e29b-41d4-a716-446655440010","declared_size_bytes":1024000}`)
	req := fazRequestInit(t, body, projectKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200, obteve %d: %s", rec.Code, rec.Body.String())
	}

	var env apiresponse.Envelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("falha ao decodificar resposta: %v", err)
	}
	// Extrai o payload de data — o envelope agora envolve a resposta.
	data, ok := env.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("data não é um objeto: %T", env.Data)
	}
	uploadURL, _ := data["upload_url"].(string)
	token, _ := data["token"].(string)
	if uploadURL == "" {
		t.Error("upload_url ausente na resposta")
	}
	if token == "" {
		t.Error("token ausente na resposta")
	}
}

func TestUploadInit_InvalidProjectKey(t *testing.T) {
	// Verifica que requisição com X-Project-Key inválida é rejeitada com 401.
	cfg := configInit(t)
	database := abreDBInit(t)
	handler := NewInitHandler(cfg, database)

	body := []byte(`{"video_id":"550e8400-e29b-41d4-a716-446655440011","declared_size_bytes":1024}`)
	req := httptest.NewRequest(http.MethodPost, "/upload/init", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project-Key", "chave-invalida-zzz")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("chave de projeto inválida deveria retornar 401, obteve %d", rec.Code)
	}
}

func TestUploadInit_MissingAuthHeader(t *testing.T) {
	// Verifica que requisição sem X-Project-Key usa o projeto padrão (200, não 401).
	cfg := configInit(t)
	database := abreDBInit(t)
	handler := NewInitHandler(cfg, database)

	body := []byte(`{"video_id":"550e8400-e29b-41d4-a716-446655440012","declared_size_bytes":1024}`)
	req := httptest.NewRequest(http.MethodPost, "/upload/init", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// Sem X-Project-Key — deve usar o projeto padrão automaticamente.

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("sem X-Project-Key deveria usar projeto padrão e retornar 200, obteve %d (body: %s)",
			rec.Code, rec.Body.String())
	}
}

func TestUploadInit_InvalidVideoID_NotUUID(t *testing.T) {
	// Verifica que video_id que não é UUID v4 é rejeitado com 400.
	cfg := configInit(t)
	database := abreDBInit(t)
	handler := NewInitHandler(cfg, database)
	projectKey := setupProject(t, database)

	body := []byte(`{"video_id":"nao-e-um-uuid-valido","declared_size_bytes":1024}`)
	req := fazRequestInit(t, body, projectKey)
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
	projectKey := setupProject(t, database)

	body := []byte(`{"video_id":"../etc/passwd","declared_size_bytes":1024}`)
	req := fazRequestInit(t, body, projectKey)
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

	projectKey := setupProject(t, database)
	body, _ := json.Marshal(map[string]interface{}{
		"video_id":            videoID,
		"declared_size_bytes": 1024,
	})
	req := fazRequestInit(t, body, projectKey)
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
	projectKey := setupProject(t, database)

	body := []byte(`{"video_id":"550e8400-e29b-41d4-a716-446655440014","declared_size_bytes":0}`)
	req := fazRequestInit(t, body, projectKey)
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
	projectKey := setupProject(t, database)

	body := []byte(`{"video_id":"550e8400-e29b-41d4-a716-446655440015","declared_size_bytes":9999999}`)
	req := fazRequestInit(t, body, projectKey)
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
	projectKey := setupProject(t, database)

	videoID := "550e8400-e29b-41d4-a716-446655440016"
	body, _ := json.Marshal(map[string]interface{}{
		"video_id":            videoID,
		"declared_size_bytes": 2048,
	})
	req := fazRequestInit(t, body, projectKey)
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
	projectKey := setupProject(t, database)

	videoID := "550e8400-e29b-41d4-a716-446655440017"
	body, _ := json.Marshal(map[string]interface{}{
		"video_id":            videoID,
		"declared_size_bytes": 4096,
	})
	req := fazRequestInit(t, body, projectKey)
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

// TestUploadInit_MalformedJSON testa requisições com JSON inválido
func TestUploadInit_MalformedJSON(t *testing.T) {
	cfg := configInit(t)
	database := abreDBInit(t)
	handler := NewInitHandler(cfg, database)
	projectKey := setupProject(t, database)

	cases := []struct {
		name     string
		jsonBody string
		desc     string
	}{
		{
			name:     "incomplete_json",
			jsonBody: `{"video_id":"550e8400-e29b-41d4-a716-446655440018"`,
			desc:     "JSON incompleto (falta fechar objeto)",
		},
		{
			name:     "trailing_comma",
			jsonBody: `{"video_id":"550e8400-e29b-41d4-a716-446655440019","declared_size_bytes":1024,}`,
			desc:     "JSON com vírgula no final antes do fechamento",
		},
		{
			name:     "single_quoted",
			jsonBody: `{'video_id':'550e8400-e29b-41d4-a716-446655440020','declared_size_bytes':1024}`,
			desc:     "JSON com single quotes em vez de double quotes",
		},
		{
			name:     "unquoted_keys",
			jsonBody: `{video_id:"550e8400-e29b-41d4-a716-446655440021",declared_size_bytes:1024}`,
			desc:     "JSON com chaves sem aspas",
		},
		{
			name:     "empty_object",
			jsonBody: `{}`,
			desc:     "JSON com objeto vazio",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/upload/init", bytes.NewReader([]byte(tc.jsonBody)))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Project-Key", projectKey)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			// JSON malformado deve retornar 400
			if rec.Code != http.StatusBadRequest {
				t.Errorf("%s: esperava 400, obteve %d", tc.desc, rec.Code)
			}
		})
	}
}

// TestUploadInit_NegativeSizeAndOverflow testa tamanhos inválidos
func TestUploadInit_NegativeSizeAndOverflow(t *testing.T) {
	cfg := configInit(t)
	database := abreDBInit(t)
	handler := NewInitHandler(cfg, database)
	projectKey := setupProject(t, database)

	cases := []struct {
		name    string
		size    int64
		desc    string
		expectError bool
	}{
		{
			name:    "negative_size",
			size:    -1024,
			desc:    "tamanho negativo deve ser rejeitado",
			expectError: true,
		},
		{
			name:    "zero_size",
			size:    0,
			desc:    "tamanho zero deve ser rejeitado",
			expectError: true,
		},
		{
			name:    "max_int64_overflow",
			size:    9223372036854775807,
			desc:    "máximo int64 acima do limite configurado é rejeitado",
			expectError: true,
		},
		{
			name:    "one_byte_over_limit",
			size:    cfg.MaxUploadSizeBytes + 1,
			desc:    "1 byte acima do limite deve ser rejeitado",
			expectError: true,
		},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Cria UUID válido determinístico a partir do índice
			uuid := fmt.Sprintf("550e8400-e29b-41d4-a716-4466554400%02d", i)
			body, _ := json.Marshal(map[string]interface{}{
				"video_id":            uuid,
				"declared_size_bytes": tc.size,
			})
			req := fazRequestInit(t, body, projectKey)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if tc.expectError && rec.Code < 400 {
				t.Errorf("%s: esperava erro (4xx/5xx), obteve %d", tc.desc, rec.Code)
			} else if !tc.expectError && rec.Code != http.StatusOK {
				t.Errorf("%s: esperava 200, obteve %d", tc.desc, rec.Code)
			}
		})
	}
}

// TestUploadInit_InvalidUUIDFormats testa vários formatos UUID inválidos
func TestUploadInit_InvalidUUIDFormats(t *testing.T) {
	cfg := configInit(t)
	database := abreDBInit(t)
	handler := NewInitHandler(cfg, database)
	projectKey := setupProject(t, database)

	cases := []struct {
		name   string
		videoID string
		desc   string
	}{
		// v1 e v3 foram removidos da lista de inválidos — o sistema agora
		// aceita qualquer versão de UUID (T44). O teste de aceitação está
		// em TestUploadInit_AnyUUIDVersionAccepted.
		{
			name:    "invalid_variant",
			videoID: "550e8400-e29b-41d4-c716-446655440000",
			desc:    "UUID com variante inválida (deve ser 8-b)",
		},
		{
			name:    "sql_injection",
			videoID: "550e8400-e29b-41d4-a716-446655440000'; DROP TABLE videos; --",
			desc:    "tentativa de SQL injection no video_id",
		},
		{
			name:    "null_byte",
			videoID: "550e8400-e29b-41d4-a716-44665544\x0000",
			desc:    "UUID com null byte",
		},
		{
			name:    "uppercase_uuid",
			videoID: "550E8400-E29B-41D4-A716-446655440000",
			desc:    "UUID v4 válido mas em uppercase (deve ser lowercase)",
		},
		{
			name:    "no_hyphens",
			videoID: "550e8400e29b41d4a716446655440000",
			desc:    "UUID sem hífens",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]interface{}{
				"video_id":            tc.videoID,
				"declared_size_bytes": 1024,
			})
			req := fazRequestInit(t, body, projectKey)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			// UUID inválido deve retornar 400
			if rec.Code != http.StatusBadRequest {
				t.Errorf("%s: esperava 400, obteve %d", tc.desc, rec.Code)
			}
		})
	}
}

// TestUploadInit_BodyTooLarge testa requisição com corpo acima do limite
func TestUploadInit_BodyTooLarge(t *testing.T) {
	cfg := configInit(t)
	database := abreDBInit(t)
	handler := NewInitHandler(cfg, database)
	projectKey := setupProject(t, database)

	// Cria um body com 2MB (limite é 1MB)
	largeBody := make([]byte, 2*1024*1024)
	for i := range largeBody {
		largeBody[i] = 'x'
	}

	req := httptest.NewRequest(http.MethodPost, "/upload/init", bytes.NewReader(largeBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project-Key", projectKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Deve retornar 400 (corpo inválido ou truncado)
	if rec.Code < 400 {
		t.Errorf("body muito grande deveria retornar erro, obteve %d", rec.Code)
	}
}

// TestUploadInit_AnyUUIDVersionAccepted verifica que qualquer versão de UUID
// (v1, v3, v4, v5, v7) informada pelo cliente é aceita (T44).
func TestUploadInit_AnyUUIDVersionAccepted(t *testing.T) {
	cfg := configInit(t)
	database := abreDBInit(t)
	handler := NewInitHandler(cfg, database)
	projectKey := setupProject(t, database)

	cases := []struct {
		name    string
		videoID string
	}{
		{"v1", "550e8400-e29b-11d4-a716-446655440000"},
		{"v3", "550e8400-e29b-31d4-a716-446655440000"},
		{"v4", "550e8400-e29b-41d4-a716-446655440000"},
		{"v5", "550e8400-e29b-51d4-a716-446655440000"},
		{"v7", "550e8400-e29b-71d4-a716-446655440000"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]interface{}{
				"video_id":            tc.videoID,
				"declared_size_bytes": 1024,
			})
			req := fazRequestInit(t, body, projectKey)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("UUID %s deveria ser aceito, mas retornou %d", tc.name, rec.Code)
			}

			// Verifica que o video_id na resposta é o mesmo informado.
			// O payload agora está dentro do envelope {error, message, data, status_code}.
			var env apiresponse.Envelope
			json.NewDecoder(rec.Body).Decode(&env)
			data, _ := env.Data.(map[string]interface{})
			videoID, _ := data["video_id"].(string)
			if videoID != tc.videoID {
				t.Errorf("video_id na resposta: esperado %s, obtido %s", tc.videoID, videoID)
			}
		})
	}
}

// Helper para cortar string de forma segura
var _ = func() {}
