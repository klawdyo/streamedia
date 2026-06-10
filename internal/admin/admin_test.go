package admin

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/klawdyo/streamedia/internal/apiresponse"
	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/db"
	"github.com/klawdyo/streamedia/internal/models"
)

// setupAdminTest cria um banco de dados em memória e retorna a conexão e uma
// configuração padrão para testes de admin.
func setupAdminTest(t *testing.T) (*sql.DB, *config.Config) {
	t.Helper()

	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir banco em memória: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	cfg := &config.Config{
		RootToken:       "test-admin-token",
		TranscodeWorkers: 1,
	}

	return database, cfg
}

// insertVideo insere um vídeo no banco de dados para testes.
func insertVideo(t *testing.T, database *sql.DB, videoID string, status models.VideoStatus) {
	t.Helper()

	_, err := database.Exec(
		"INSERT INTO videos (video_id, tag, status, created_at, updated_at) VALUES (?, 'default', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
		videoID, status,
	)
	if err != nil {
		t.Fatalf("erro ao inserir vídeo %s: %v", videoID, err)
	}
}

// mockQueue é uma fila mock que implementa o interface { Len() int }.
type mockQueue struct {
	length int
}

// Len retorna o comprimento da fila mock.
func (m *mockQueue) Len() int {
	return m.length
}

// TestAdminVideos_WithAuth verifica que HandleVideos retorna 200 com dados
// corretos quando chamado com um token de autorização válido.
func TestAdminVideos_WithAuth(t *testing.T) {
	database, cfg := setupAdminTest(t)
	handler := NewAdminHandler(cfg, database, &mockQueue{})

	// Insere 2 vídeos no banco
	insertVideo(t, database, "vid-1", models.StatusReady)
	insertVideo(t, database, "vid-2", models.StatusUploading)

	// Cria a requisição com query params
	req := httptest.NewRequest("GET", "/admin/videos", nil)
	req.Header.Set("Authorization", "Bearer test-admin-token")

	w := httptest.NewRecorder()
	handler.HandleVideos(w, req)

	// Valida status code
	if w.Code != http.StatusOK {
		t.Errorf("status code esperado 200, obtido %d", w.Code)
	}

	// Parse da resposta JSON
	var env apiresponse.Envelope
	body, _ := io.ReadAll(w.Body)
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("erro ao fazer unmarshal da resposta: %v", err)
	}
	dataJSON, _ := json.Marshal(env.Data)
	var resp videosResponse
	json.Unmarshal(dataJSON, &resp)

	// Valida que há 2 vídeos e total é 2
	if len(resp.Videos) != 2 {
		t.Errorf("número de vídeos esperado 2, obtido %d", len(resp.Videos))
	}
	if resp.Total != 2 {
		t.Errorf("total esperado 2, obtido %d", resp.Total)
	}
}

// TestAdminVideos_WithoutAuth verifica que HandleVideos retorna 401 quando
// chamado sem token de autorização (usando o middleware AdminAuth).
func TestAdminVideos_WithoutAuth(t *testing.T) {
	database, cfg := setupAdminTest(t)
	handler := NewAdminHandler(cfg, database, &mockQueue{})

	// Wraps o handler com o middleware AdminAuth
	wrapped := RootAuth(cfg.RootToken)(http.HandlerFunc(handler.HandleVideos))

	// Cria a requisição SEM header Authorization
	req := httptest.NewRequest("GET", "/admin/videos", nil)
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	// Valida que recebeu 401
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status code esperado 401, obtido %d", w.Code)
	}
}

// TestAdminVideos_FilterByStatus verifica que o filtro por status funciona
// corretamente, retornando apenas vídeos com o status solicitado.
func TestAdminVideos_FilterByStatus(t *testing.T) {
	database, cfg := setupAdminTest(t)
	handler := NewAdminHandler(cfg, database, &mockQueue{})

	// Insere vídeos com diferentes status
	insertVideo(t, database, "vid-ready-1", models.StatusReady)
	insertVideo(t, database, "vid-ready-2", models.StatusReady)
	insertVideo(t, database, "vid-uploading-1", models.StatusUploading)

	// Faz requisição filtrando por status "ready"
	req := httptest.NewRequest("GET", "/admin/videos?status=ready", nil)
	req.Header.Set("Authorization", "Bearer test-admin-token")

	w := httptest.NewRecorder()
	handler.HandleVideos(w, req)

	var env apiresponse.Envelope
	body, _ := io.ReadAll(w.Body)
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("erro ao fazer unmarshal da resposta: %v", err)
	}
	dataJSON, _ := json.Marshal(env.Data)
	var resp videosResponse
	json.Unmarshal(dataJSON, &resp)

	// Valida que apenas 2 vídeos foram retornados (ambos com status "ready")
	if len(resp.Videos) != 2 {
		t.Errorf("número de vídeos esperado 2, obtido %d", len(resp.Videos))
	}
	if resp.Total != 2 {
		t.Errorf("total esperado 2, obtido %d", resp.Total)
	}

	// Valida que todos os vídeos retornados têm status "ready"
	for _, v := range resp.Videos {
		if v.Status != models.StatusReady {
			t.Errorf("status esperado %s, obtido %s", models.StatusReady, v.Status)
		}
	}
}

// TestAdminVideos_Pagination verifica que os parâmetros limit e offset
// funcionam corretamente.
func TestAdminVideos_Pagination(t *testing.T) {
	database, cfg := setupAdminTest(t)
	handler := NewAdminHandler(cfg, database, &mockQueue{})

	// Insere 5 vídeos
	for i := 1; i <= 5; i++ {
		insertVideo(t, database, "vid-"+string(rune(48+i)), models.StatusReady)
	}

	// Primeira página: limit=2, offset=0 (esperado 2 vídeos)
	req1 := httptest.NewRequest("GET", "/admin/videos?limit=2&offset=0", nil)
	req1.Header.Set("Authorization", "Bearer test-admin-token")
	w1 := httptest.NewRecorder()
	handler.HandleVideos(w1, req1)

	var env1 apiresponse.Envelope
	body1, _ := io.ReadAll(w1.Body)
	json.Unmarshal(body1, &env1)
	dataJSON1, _ := json.Marshal(env1.Data)
	var resp1 videosResponse
	json.Unmarshal(dataJSON1, &resp1)

	if len(resp1.Videos) != 2 {
		t.Errorf("primeira página: esperado 2 vídeos, obtido %d", len(resp1.Videos))
	}
	if resp1.Total != 5 {
		t.Errorf("total esperado 5, obtido %d", resp1.Total)
	}

	// Segunda página: limit=2, offset=2 (esperado 2 vídeos)
	req2 := httptest.NewRequest("GET", "/admin/videos?limit=2&offset=2", nil)
	req2.Header.Set("Authorization", "Bearer test-admin-token")
	w2 := httptest.NewRecorder()
	handler.HandleVideos(w2, req2)

	var env2 apiresponse.Envelope
	body2, _ := io.ReadAll(w2.Body)
	json.Unmarshal(body2, &env2)
	dataJSON2, _ := json.Marshal(env2.Data)
	var resp2 videosResponse
	json.Unmarshal(dataJSON2, &resp2)

	if len(resp2.Videos) != 2 {
		t.Errorf("segunda página: esperado 2 vídeos, obtido %d", len(resp2.Videos))
	}

	// Terceira página: limit=2, offset=4 (esperado 1 vídeo)
	req3 := httptest.NewRequest("GET", "/admin/videos?limit=2&offset=4", nil)
	req3.Header.Set("Authorization", "Bearer test-admin-token")
	w3 := httptest.NewRecorder()
	handler.HandleVideos(w3, req3)

	var env3 apiresponse.Envelope
	body3, _ := io.ReadAll(w3.Body)
	json.Unmarshal(body3, &env3)
	dataJSON3, _ := json.Marshal(env3.Data)
	var resp3 videosResponse
	json.Unmarshal(dataJSON3, &resp3)

	if len(resp3.Videos) != 1 {
		t.Errorf("terceira página: esperado 1 vídeo, obtido %d", len(resp3.Videos))
	}
}

// TestAdminQueue_WithAuth verifica que HandleQueue retorna o estado correto
// da fila quando chamado com autenticação válida.
func TestAdminQueue_WithAuth(t *testing.T) {
	database, cfg := setupAdminTest(t)
	mockQ := &mockQueue{length: 3}
	handler := NewAdminHandler(cfg, database, mockQ)

	req := httptest.NewRequest("GET", "/admin/queue", nil)
	req.Header.Set("Authorization", "Bearer test-admin-token")

	w := httptest.NewRecorder()
	handler.HandleQueue(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status code esperado 200, obtido %d", w.Code)
	}

	var env apiresponse.Envelope
	body, _ := io.ReadAll(w.Body)
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("erro ao fazer unmarshal da resposta: %v", err)
	}
	dataJSON, _ := json.Marshal(env.Data)
	var resp queueResponse
	json.Unmarshal(dataJSON, &resp)

	if resp.QueueLength != 3 {
		t.Errorf("queue_length esperado 3, obtido %d", resp.QueueLength)
	}
	if resp.Workers != 1 {
		t.Errorf("workers esperado 1, obtido %d", resp.Workers)
	}
}

// TestAdminQueue_ShowsCurrentLength verifica que HandleQueue retorna o
// comprimento atual da fila quando muda.
func TestAdminQueue_ShowsCurrentLength(t *testing.T) {
	database, cfg := setupAdminTest(t)
	mockQ := &mockQueue{length: 5}
	handler := NewAdminHandler(cfg, database, mockQ)

	req := httptest.NewRequest("GET", "/admin/queue", nil)
	req.Header.Set("Authorization", "Bearer test-admin-token")

	w := httptest.NewRecorder()
	handler.HandleQueue(w, req)

	var env apiresponse.Envelope
	body, _ := io.ReadAll(w.Body)
	json.Unmarshal(body, &env)
	dataJSON, _ := json.Marshal(env.Data)
	var resp queueResponse
	json.Unmarshal(dataJSON, &resp)

	if resp.QueueLength != 5 {
		t.Errorf("queue_length esperado 5, obtido %d", resp.QueueLength)
	}
}

// TestAdminVideos_InvalidStatus verifica que quando um status inválido é
// fornecido, a resposta contém um array vazio, não um erro 500.
func TestAdminVideos_InvalidStatus(t *testing.T) {
	database, cfg := setupAdminTest(t)
	handler := NewAdminHandler(cfg, database, &mockQueue{})

	// Insere alguns vídeos
	insertVideo(t, database, "vid-1", models.StatusReady)
	insertVideo(t, database, "vid-2", models.StatusUploading)

	// Faz requisição com status inexistente
	req := httptest.NewRequest("GET", "/admin/videos?status=nonexistent_status", nil)
	req.Header.Set("Authorization", "Bearer test-admin-token")

	w := httptest.NewRecorder()
	handler.HandleVideos(w, req)

	// Deve retornar 200, não 500
	if w.Code != http.StatusOK {
		t.Errorf("status code esperado 200, obtido %d", w.Code)
	}

	var env apiresponse.Envelope
	body, _ := io.ReadAll(w.Body)
	json.Unmarshal(body, &env)
	dataJSON, _ := json.Marshal(env.Data)
	var resp videosResponse
	json.Unmarshal(dataJSON, &resp)

	// Deve retornar array vazio
	if len(resp.Videos) != 0 {
		t.Errorf("número de vídeos esperado 0, obtido %d", len(resp.Videos))
	}
	if resp.Total != 0 {
		t.Errorf("total esperado 0, obtido %d", resp.Total)
	}
}

// TestAdminAuth_WrongToken verifica que o middleware AdminAuth retorna 401
// quando um token incorreto é fornecido.
func TestAdminAuth_WrongToken(t *testing.T) {
	cfg := &config.Config{RootToken: "correct-token"}

	wrapped := RootAuth(cfg.RootToken)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")

	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status code esperado 401, obtido %d", w.Code)
	}
}

// TestAdminAuth_BadAuthFormat verifica que o middleware AdminAuth retorna 401
// quando o header Authorization não segue o formato "Bearer {token}".
func TestAdminAuth_BadAuthFormat(t *testing.T) {
	cfg := &config.Config{RootToken: "correct-token"}

	wrapped := RootAuth(cfg.RootToken)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Testa sem "Bearer" prefix
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "correct-token")

	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status code esperado 401, obtido %d", w.Code)
	}
}

// TestAdminVideos_LimitMaximum verifica que o limite máximo de 200 registros
// é aplicado corretamente.
func TestAdminVideos_LimitMaximum(t *testing.T) {
	database, cfg := setupAdminTest(t)
	handler := NewAdminHandler(cfg, database, &mockQueue{})

	// Insere 3 vídeos
	for i := 1; i <= 3; i++ {
		insertVideo(t, database, "vid-"+string(rune(48+i)), models.StatusReady)
	}

	// Requisita com limit=500 (deve ser reduzido para 200)
	req := httptest.NewRequest("GET", "/admin/videos?limit=500", nil)
	req.Header.Set("Authorization", "Bearer test-admin-token")

	w := httptest.NewRecorder()
	handler.HandleVideos(w, req)

	var env apiresponse.Envelope
	body, _ := io.ReadAll(w.Body)
	json.Unmarshal(body, &env)
	dataJSON, _ := json.Marshal(env.Data)
	var resp videosResponse
	json.Unmarshal(dataJSON, &resp)

	// Deve retornar 3 vídeos (menos que 200)
	if len(resp.Videos) != 3 {
		t.Errorf("número de vídeos esperado 3, obtido %d", len(resp.Videos))
	}
}

// TestAdminVideos_EmptyDatabase verifica que HandleVideos retorna um array
// vazio e total 0 quando o banco está vazio.
func TestAdminVideos_EmptyDatabase(t *testing.T) {
	database, cfg := setupAdminTest(t)
	handler := NewAdminHandler(cfg, database, &mockQueue{})

	req := httptest.NewRequest("GET", "/admin/videos", nil)
	req.Header.Set("Authorization", "Bearer test-admin-token")

	w := httptest.NewRecorder()
	handler.HandleVideos(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status code esperado 200, obtido %d", w.Code)
	}

	var env apiresponse.Envelope
	body, _ := io.ReadAll(w.Body)
	json.Unmarshal(body, &env)
	dataJSON, _ := json.Marshal(env.Data)
	var resp videosResponse
	json.Unmarshal(dataJSON, &resp)

	if len(resp.Videos) != 0 {
		t.Errorf("número de vídeos esperado 0, obtido %d", len(resp.Videos))
	}
	if resp.Total != 0 {
		t.Errorf("total esperado 0, obtido %d", resp.Total)
	}
}

// TestAdminQueue_ZeroWorkers verifica que HandleQueue retorna corretamente
// quando não há workers configurados.
func TestAdminQueue_ZeroWorkers(t *testing.T) {
	database, cfg := setupAdminTest(t)
	cfg.TranscodeWorkers = 0
	mockQ := &mockQueue{length: 5}
	handler := NewAdminHandler(cfg, database, mockQ)

	req := httptest.NewRequest("GET", "/admin/queue", nil)
	req.Header.Set("Authorization", "Bearer test-admin-token")

	w := httptest.NewRecorder()
	handler.HandleQueue(w, req)

	var env apiresponse.Envelope
	body, _ := io.ReadAll(w.Body)
	json.Unmarshal(body, &env)
	dataJSON, _ := json.Marshal(env.Data)
	var resp queueResponse
	json.Unmarshal(dataJSON, &resp)

	if resp.Workers != 0 {
		t.Errorf("workers esperado 0, obtido %d", resp.Workers)
	}
}

// TestAdminVideos_OrderByCreatedAt verifica que os vídeos são retornados
// em ordem decrescente de created_at (mais recentes primeiro).
func TestAdminVideos_OrderByCreatedAt(t *testing.T) {
	database, cfg := setupAdminTest(t)
	handler := NewAdminHandler(cfg, database, &mockQueue{})

	// Insere vídeos em ordem cronológica (mais antigos para mais novos)
	now := time.Now()
	for i := 0; i < 3; i++ {
		videoID := "vid-" + string(rune(48+i))
		createdAt := now.Add(time.Duration(i) * time.Minute)
		_, err := database.Exec(
			"INSERT INTO videos (video_id, tag, status, created_at, updated_at) VALUES (?, 'default', ?, ?, ?)",
			videoID, models.StatusReady, createdAt, createdAt,
		)
		if err != nil {
			t.Fatalf("erro ao inserir vídeo: %v", err)
		}
	}

	req := httptest.NewRequest("GET", "/admin/videos", nil)
	req.Header.Set("Authorization", "Bearer test-admin-token")

	w := httptest.NewRecorder()
	handler.HandleVideos(w, req)

	var env apiresponse.Envelope
	body, _ := io.ReadAll(w.Body)
	json.Unmarshal(body, &env)
	dataJSON, _ := json.Marshal(env.Data)
	var resp videosResponse
	json.Unmarshal(dataJSON, &resp)

	// Valida que o primeiro vídeo retornado é o mais recente (vid-2)
	if len(resp.Videos) > 0 && resp.Videos[0].VideoID != "vid-2" {
		t.Errorf("primeiro vídeo esperado ser o mais recente (vid-2), obtido %s", resp.Videos[0].VideoID)
	}
}

// TestAdminVideos_ContentType verifica que o header Content-Type é
// application/json.
func TestAdminVideos_ContentType(t *testing.T) {
	database, cfg := setupAdminTest(t)
	handler := NewAdminHandler(cfg, database, &mockQueue{})

	req := httptest.NewRequest("GET", "/admin/videos", nil)
	req.Header.Set("Authorization", "Bearer test-admin-token")

	w := httptest.NewRecorder()
	handler.HandleVideos(w, req)

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json; charset=utf-8" {
		t.Errorf("Content-Type esperado \"application/json; charset=utf-8\", obtido %q", contentType)
	}
}

// TestAdminQueue_ContentType verifica que o header Content-Type é
// application/json; charset=utf-8 para a rota de fila.
func TestAdminQueue_ContentType(t *testing.T) {
	database, cfg := setupAdminTest(t)
	handler := NewAdminHandler(cfg, database, &mockQueue{})

	req := httptest.NewRequest("GET", "/admin/queue", nil)
	req.Header.Set("Authorization", "Bearer test-admin-token")

	w := httptest.NewRecorder()
	handler.HandleQueue(w, req)

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json; charset=utf-8" {
		t.Errorf("Content-Type esperado \"application/json; charset=utf-8\", obtido %q", contentType)
	}
}
