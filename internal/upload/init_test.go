package upload

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/klawdyo/streamedia/internal/apiresponse"
	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/db"
	"github.com/klawdyo/streamedia/internal/models"
)

// configInit cria config mínima para testes da rota de init. A autenticação
// (ROOT_TOKEN) é feita pelo middleware no roteador, então o handler em si não
// exige credencial — os testes chamam ServeHTTP direto.
func configInit(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		WebhookURL:         "http://localhost",
		WebhookSecret:      "wh-secret",
		MaxUploadSizeBytes: 50 * 1024 * 1024,
		UploadTokenTTL:     15 * time.Minute,
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

// fazRequestInit constrói um POST /api/upload/init com o corpo informado.
func fazRequestInit(t *testing.T, body []byte) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/upload/init", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestUploadInit_Success(t *testing.T) {
	cfg := configInit(t)
	database := abreDBInit(t)
	handler := NewInitHandler(cfg, database)

	body := []byte(`{"tag":"minha-tag","video_id":"550e8400-e29b-41d4-a716-446655440010","declared_size_bytes":1024000}`)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, fazRequestInit(t, body))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200, obteve %d: %s", rec.Code, rec.Body.String())
	}

	var env apiresponse.Envelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("falha ao decodificar resposta: %v", err)
	}
	data, ok := env.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("data não é um objeto: %T", env.Data)
	}
	if uploadURL, _ := data["upload_url"].(string); uploadURL == "" {
		t.Error("upload_url ausente na resposta")
	}
	if token, _ := data["token"].(string); token == "" {
		t.Error("token ausente na resposta")
	}
	if tag, _ := data["tag"].(string); tag != "minha-tag" {
		t.Errorf("tag na resposta: esperado %q, obtido %q", "minha-tag", tag)
	}
}

// TestValidateWebhookURL cobre a validação da webhook_url customizada (issue
// #20): HTTPS obrigatório, formato válido e limite de 2048 caracteres.
func TestValidateWebhookURL(t *testing.T) {
	longHost := "https://e.com/" + strings.Repeat("a", maxWebhookURLLen) // > 2048

	casos := []struct {
		nome   string
		in     string
		wantOK bool
	}{
		{"https válido", "https://hooks.example.com/abc", true},
		{"https com espaços nas bordas", "  https://e.com/x  ", true},
		{"http rejeitado", "http://e.com/x", false},
		{"sem esquema rejeitado", "e.com/x", false},
		{"relativo rejeitado", "/somente/path", false},
		{"esquema esquisito rejeitado", "ftp://e.com", false},
		{"acima de 2048 rejeitado", longHost, false},
		{"lixo rejeitado", "ht!tp://%%%", false},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			got, ok := validateWebhookURL(c.in)
			if ok != c.wantOK {
				t.Fatalf("validateWebhookURL(%q): ok=%v, esperava %v", c.in, ok, c.wantOK)
			}
			// Quando válida, a URL deve voltar sem espaços nas bordas.
			if ok && got != strings.TrimSpace(c.in) {
				t.Errorf("URL normalizada = %q, esperava %q", got, strings.TrimSpace(c.in))
			}
		})
	}
}

// TestUploadInit_CustomWebhookURL_Persisted garante que uma webhook_url HTTPS
// válida é aceita e persistida em videos.webhook_url (issue #20).
func TestUploadInit_CustomWebhookURL_Persisted(t *testing.T) {
	cfg := configInit(t)
	database := abreDBInit(t)
	handler := NewInitHandler(cfg, database)

	videoID := "550e8400-e29b-41d4-a716-446655440099"
	body := []byte(`{"tag":"t","video_id":"` + videoID + `","declared_size_bytes":1024,"webhook_url":"https://hooks.example.com/v1"}`)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, fazRequestInit(t, body))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200, obteve %d: %s", rec.Code, rec.Body.String())
	}

	v, err := models.GetVideo(database, videoID)
	if err != nil {
		t.Fatalf("erro ao buscar vídeo: %v", err)
	}
	if v.WebhookURL != "https://hooks.example.com/v1" {
		t.Errorf("webhook_url persistido = %q, esperava %q", v.WebhookURL, "https://hooks.example.com/v1")
	}
}

// TestUploadInit_InvalidWebhookURL_Rejected garante que uma webhook_url
// informada mas inválida (não-HTTPS) é rejeitada com 400 (issue #20).
func TestUploadInit_InvalidWebhookURL_Rejected(t *testing.T) {
	cfg := configInit(t)
	database := abreDBInit(t)
	handler := NewInitHandler(cfg, database)

	body := []byte(`{"tag":"t","video_id":"550e8400-e29b-41d4-a716-446655440098","declared_size_bytes":1024,"webhook_url":"http://inseguro.example.com"}`)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, fazRequestInit(t, body))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("esperava 400 para webhook_url inválida, obteve %d: %s", rec.Code, rec.Body.String())
	}
}

// TestUploadInit_OmittedWebhookURL_Empty garante que omitir webhook_url deixa
// a coluna vazia (o vídeo usará a WEBHOOK_URL global) — sem erro (issue #20).
func TestUploadInit_OmittedWebhookURL_Empty(t *testing.T) {
	cfg := configInit(t)
	database := abreDBInit(t)
	handler := NewInitHandler(cfg, database)

	videoID := "550e8400-e29b-41d4-a716-446655440097"
	body := []byte(`{"tag":"t","video_id":"` + videoID + `","declared_size_bytes":1024}`)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, fazRequestInit(t, body))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200, obteve %d: %s", rec.Code, rec.Body.String())
	}
	v, err := models.GetVideo(database, videoID)
	if err != nil {
		t.Fatalf("erro ao buscar vídeo: %v", err)
	}
	if v.WebhookURL != "" {
		t.Errorf("webhook_url deveria ser vazio, obteve %q", v.WebhookURL)
	}
}

func TestUploadInit_MissingTag(t *testing.T) {
	// Sem tag (ou tag que normaliza para vazio) deve retornar 400.
	cfg := configInit(t)
	database := abreDBInit(t)
	handler := NewInitHandler(cfg, database)

	body := []byte(`{"video_id":"550e8400-e29b-41d4-a716-446655440011","declared_size_bytes":1024}`)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, fazRequestInit(t, body))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("tag ausente deveria retornar 400, obteve %d", rec.Code)
	}
}

func TestUploadInit_TagNormalized(t *testing.T) {
	// A tag é normalizada por Slugify (acentos/espaços/maiúsculas).
	cfg := configInit(t)
	database := abreDBInit(t)
	handler := NewInitHandler(cfg, database)

	body := []byte(`{"tag":"Minha Tag Ção","video_id":"550e8400-e29b-41d4-a716-446655440022","declared_size_bytes":1024}`)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, fazRequestInit(t, body))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200, obteve %d: %s", rec.Code, rec.Body.String())
	}
	video, err := models.GetVideo(database, "550e8400-e29b-41d4-a716-446655440022")
	if err != nil {
		t.Fatal(err)
	}
	if video.Tag != "minha-tag-cao" {
		t.Errorf("tag normalizada: esperado %q, obtido %q", "minha-tag-cao", video.Tag)
	}
}

func TestUploadInit_InvalidVideoID_NotUUID(t *testing.T) {
	cfg := configInit(t)
	database := abreDBInit(t)
	handler := NewInitHandler(cfg, database)

	body := []byte(`{"tag":"t","video_id":"nao-e-um-uuid-valido","declared_size_bytes":1024}`)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, fazRequestInit(t, body))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("video_id inválido deveria retornar 400, obteve %d", rec.Code)
	}
}

func TestUploadInit_InvalidVideoID_PathTraversal(t *testing.T) {
	cfg := configInit(t)
	database := abreDBInit(t)
	handler := NewInitHandler(cfg, database)

	body := []byte(`{"tag":"t","video_id":"../etc/passwd","declared_size_bytes":1024}`)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, fazRequestInit(t, body))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("path traversal deveria retornar 400, obteve %d", rec.Code)
	}
}

func TestUploadInit_DuplicateVideoID(t *testing.T) {
	cfg := configInit(t)
	database := abreDBInit(t)
	handler := NewInitHandler(cfg, database)

	videoID := "550e8400-e29b-41d4-a716-446655440013"
	if err := models.InsertVideo(database, videoID, 1024); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(map[string]interface{}{"tag": "t", "video_id": videoID, "declared_size_bytes": 1024})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, fazRequestInit(t, body))

	if rec.Code != http.StatusConflict {
		t.Errorf("video_id duplicado deveria retornar 409, obteve %d", rec.Code)
	}
}

func TestUploadInit_ZeroSize(t *testing.T) {
	cfg := configInit(t)
	database := abreDBInit(t)
	handler := NewInitHandler(cfg, database)

	body := []byte(`{"tag":"t","video_id":"550e8400-e29b-41d4-a716-446655440014","declared_size_bytes":0}`)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, fazRequestInit(t, body))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("tamanho zero deveria retornar 400, obteve %d", rec.Code)
	}
}

func TestUploadInit_SizeExceedsLimit(t *testing.T) {
	cfg := configInit(t)
	cfg.MaxUploadSizeBytes = 1024
	database := abreDBInit(t)
	handler := NewInitHandler(cfg, database)

	body := []byte(`{"tag":"t","video_id":"550e8400-e29b-41d4-a716-446655440015","declared_size_bytes":9999999}`)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, fazRequestInit(t, body))

	if rec.Code == http.StatusOK {
		t.Error("tamanho acima do limite não deveria retornar 200")
	}
}

func TestUploadInit_TokenStoredInDB(t *testing.T) {
	cfg := configInit(t)
	database := abreDBInit(t)
	handler := NewInitHandler(cfg, database)

	videoID := "550e8400-e29b-41d4-a716-446655440016"
	body, _ := json.Marshal(map[string]interface{}{"tag": "t", "video_id": videoID, "declared_size_bytes": 2048})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, fazRequestInit(t, body))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200, obteve %d", rec.Code)
	}

	var env apiresponse.Envelope
	json.NewDecoder(rec.Body).Decode(&env)
	data, _ := env.Data.(map[string]interface{})
	tokenStr, _ := data["token"].(string)

	tok, err := models.GetAccessToken(database, tokenStr)
	if err != nil {
		t.Fatalf("token não encontrado no banco: %v", err)
	}
	if tok.Purpose != models.PurposeUpload {
		t.Errorf("purpose: esperado %q, obtido %q", models.PurposeUpload, tok.Purpose)
	}
	if tok.IsExpired() {
		t.Error("token recém-criado não deveria estar expirado")
	}
}

func TestUploadInit_VideoCreatedInDB(t *testing.T) {
	cfg := configInit(t)
	database := abreDBInit(t)
	handler := NewInitHandler(cfg, database)

	videoID := "550e8400-e29b-41d4-a716-446655440017"
	body, _ := json.Marshal(map[string]interface{}{"tag": "t", "video_id": videoID, "declared_size_bytes": 4096})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, fazRequestInit(t, body))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200, obteve %d", rec.Code)
	}

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

func TestUploadInit_MalformedJSON(t *testing.T) {
	cfg := configInit(t)
	database := abreDBInit(t)
	handler := NewInitHandler(cfg, database)

	cases := []struct {
		name     string
		jsonBody string
	}{
		{"incomplete_json", `{"tag":"t","video_id":"550e8400-e29b-41d4-a716-446655440018"`},
		{"single_quoted", `{'tag':'t'}`},
		{"unquoted_keys", `{tag:"t"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, fazRequestInit(t, []byte(tc.jsonBody)))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("esperava 400, obteve %d", rec.Code)
			}
		})
	}
}

func TestUploadInit_AnyUUIDVersionAccepted(t *testing.T) {
	cfg := configInit(t)
	database := abreDBInit(t)
	handler := NewInitHandler(cfg, database)

	cases := []struct {
		name    string
		videoID string
	}{
		{"v1", "550e8400-e29b-11d4-a716-446655440000"},
		{"v4", "550e8400-e29b-41d4-a716-446655440000"},
		{"v7", "550e8400-e29b-71d4-a716-446655440000"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]interface{}{"tag": "t", "video_id": tc.videoID, "declared_size_bytes": 1024})
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, fazRequestInit(t, body))
			if rec.Code != http.StatusOK {
				t.Errorf("UUID %s deveria ser aceito, mas retornou %d", tc.name, rec.Code)
			}
		})
	}
}

// TestUploadInit_GeneratesVideoIDWhenOmitted verifica que omitir video_id gera
// um UUID v7 automaticamente.
func TestUploadInit_GeneratesVideoIDWhenOmitted(t *testing.T) {
	cfg := configInit(t)
	database := abreDBInit(t)
	handler := NewInitHandler(cfg, database)

	body := []byte(`{"tag":"t","declared_size_bytes":1024}`)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, fazRequestInit(t, body))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200, obteve %d: %s", rec.Code, rec.Body.String())
	}
	var env apiresponse.Envelope
	json.NewDecoder(rec.Body).Decode(&env)
	data, _ := env.Data.(map[string]interface{})
	if videoID, _ := data["video_id"].(string); videoID == "" {
		t.Error("video_id deveria ter sido gerado automaticamente")
	}
}

var _ = fmt.Sprintf
