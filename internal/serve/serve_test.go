package serve

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/klawdyo/streamedia/internal/auth"
	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/db"
)

// UUID v4 válido fixo usado nos testes (version=4, variant=b).
const testVideoID = "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"

// testSecret é o secret HMAC compartilhado usado para assinar tokens nos testes.
const testSecret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// newTestConfig cria um Config apontando para um MediaDir temporário.
func newTestConfig(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		MediaDir:        t.TempDir(),
		UploadTokenSecret: testSecret,
		PlayTokenMaxTTL: 6 * time.Hour,
	}
}

// newTestDB abre um banco SQLite em memória com o schema aplicado.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir banco em memória: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

// insertVideo insere um vídeo com o status informado.
func insertVideo(t *testing.T, database *sql.DB, id, status string) {
	t.Helper()
	if _, err := database.Exec("INSERT INTO videos (video_id, status) VALUES (?, ?)", id, status); err != nil {
		t.Fatalf("erro ao inserir vídeo: %v", err)
	}
}

// writeFile cria um arquivo (e seus diretórios) com o conteúdo informado.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("erro ao criar diretório: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("erro ao escrever arquivo: %v", err)
	}
}

// --- Testes do MasterHandler ---

func TestMasterM3U8_ValidToken(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)
	insertVideo(t, database, testVideoID, "ready")

	const m3u8Content = "#EXTM3U\n#EXT-X-VERSION:3\n"
	writeFile(t, filepath.Join(cfg.MediaDir, testVideoID, "master.m3u8"), m3u8Content)

	expires := time.Now().Add(time.Hour).Unix()
	token := auth.GeneratePlayToken(testSecret, testVideoID, expires)

	h := NewMasterHandler(cfg, database)
	req := httptest.NewRequest(http.MethodGet,
		"/videos/"+testVideoID+"/master.m3u8?expires="+itoa(expires)+"&token="+token, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtido %d (body: %s)", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != m3u8Content {
		t.Fatalf("conteúdo do m3u8 inesperado: %q", rec.Body.String())
	}
}

func TestMasterM3U8_InvalidToken(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)
	insertVideo(t, database, testVideoID, "ready")
	writeFile(t, filepath.Join(cfg.MediaDir, testVideoID, "master.m3u8"), "#EXTM3U\n")

	expires := time.Now().Add(time.Hour).Unix()
	// Token adulterado.
	token := "deadbeef" + auth.GeneratePlayToken(testSecret, testVideoID, expires)[8:]

	h := NewMasterHandler(cfg, database)
	req := httptest.NewRequest(http.MethodGet,
		"/videos/"+testVideoID+"/master.m3u8?expires="+itoa(expires)+"&token="+token, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401, obtido %d", rec.Code)
	}
}

func TestMasterM3U8_ExpiredToken(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)
	insertVideo(t, database, testVideoID, "ready")
	writeFile(t, filepath.Join(cfg.MediaDir, testVideoID, "master.m3u8"), "#EXTM3U\n")

	expires := time.Now().Add(-time.Hour).Unix() // já expirou
	token := auth.GeneratePlayToken(testSecret, testVideoID, expires)

	h := NewMasterHandler(cfg, database)
	req := httptest.NewRequest(http.MethodGet,
		"/videos/"+testVideoID+"/master.m3u8?expires="+itoa(expires)+"&token="+token, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401, obtido %d", rec.Code)
	}
}

func TestMasterM3U8_VideoNotReady(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)
	insertVideo(t, database, testVideoID, "transcoding")
	writeFile(t, filepath.Join(cfg.MediaDir, testVideoID, "master.m3u8"), "#EXTM3U\n")

	expires := time.Now().Add(time.Hour).Unix()
	token := auth.GeneratePlayToken(testSecret, testVideoID, expires)

	h := NewMasterHandler(cfg, database)
	req := httptest.NewRequest(http.MethodGet,
		"/videos/"+testVideoID+"/master.m3u8?expires="+itoa(expires)+"&token="+token, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("esperado 404, obtido %d", rec.Code)
	}
}

func TestMasterM3U8_VideoIDPathTraversal(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)

	expires := time.Now().Add(time.Hour).Unix()
	token := auth.GeneratePlayToken(testSecret, "../etc/passwd", expires)

	h := NewMasterHandler(cfg, database)
	req := httptest.NewRequest(http.MethodGet,
		"/videos/../etc/passwd/master.m3u8?expires="+itoa(expires)+"&token="+token, nil)
	// Sobrescreve o path cru para impedir normalização do http.NewRequest.
	req.URL.Path = "/videos/../etc/passwd/master.m3u8"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("esperado 400, obtido %d", rec.Code)
	}
}

func TestMasterM3U8_VideoNotFound(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t) // sem inserir o vídeo

	expires := time.Now().Add(time.Hour).Unix()
	token := auth.GeneratePlayToken(testSecret, testVideoID, expires)

	h := NewMasterHandler(cfg, database)
	req := httptest.NewRequest(http.MethodGet,
		"/videos/"+testVideoID+"/master.m3u8?expires="+itoa(expires)+"&token="+token, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("esperado 404, obtido %d", rec.Code)
	}
}

// --- Testes do StaticHandler ---

func TestStaticSegment_ValidPath(t *testing.T) {
	cfg := newTestConfig(t)

	const segContent = "TS_SEGMENT_DATA"
	writeFile(t, filepath.Join(cfg.MediaDir, testVideoID, "480", "0.ts"), segContent)

	h := NewStaticHandler(cfg)
	req := httptest.NewRequest(http.MethodGet, "/videos/"+testVideoID+"/480/0.ts", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtido %d (body: %s)", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != segContent {
		t.Fatalf("conteúdo do segmento inesperado: %q", rec.Body.String())
	}
}

func TestStaticSegment_InvalidResolution(t *testing.T) {
	cfg := newTestConfig(t)

	h := NewStaticHandler(cfg)
	req := httptest.NewRequest(http.MethodGet, "/videos/"+testVideoID+"/9999/0.ts", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("esperado 400, obtido %d", rec.Code)
	}
}

func TestStaticSegment_PathTraversal(t *testing.T) {
	cfg := newTestConfig(t)

	h := NewStaticHandler(cfg)
	req := httptest.NewRequest(http.MethodGet, "/videos/"+testVideoID+"/480/x.ts", nil)
	// Define o path cru com traversal, sem deixar o http.NewRequest normalizar.
	req.URL.Path = "/videos/" + testVideoID + "/480/../../../etc/passwd"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("esperado 400, obtido %d", rec.Code)
	}
}

func TestStaticServing_NoDirectoryListing(t *testing.T) {
	cfg := newTestConfig(t)

	// Cria o diretório de resolução com um arquivo dentro.
	writeFile(t, filepath.Join(cfg.MediaDir, testVideoID, "480", "0.ts"), "DATA")

	h := NewStaticHandler(cfg)
	req := httptest.NewRequest(http.MethodGet, "/videos/"+testVideoID+"/480/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("esperado 404 (sem listagem de diretório), obtido %d", rec.Code)
	}
}

func TestStaticSegment_SegmentNotFound(t *testing.T) {
	cfg := newTestConfig(t)

	h := NewStaticHandler(cfg)
	req := httptest.NewRequest(http.MethodGet, "/videos/"+testVideoID+"/480/999.ts", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("esperado 404, obtido %d", rec.Code)
	}
}

// itoa converte int64 para string base 10 (helper local para montar URLs).
func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}
