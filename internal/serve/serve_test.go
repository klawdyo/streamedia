package serve

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/klawdyo/streamedia/internal/auth"
	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/db"
	"github.com/klawdyo/streamedia/internal/models"
)

// UUID v4 válido fixo usado nos testes (version=4, variant=b).
const testVideoID = "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"

// testTag é o namespace usado nos testes.
const testTag = "default"

// newTestConfig cria um Config apontando para um MediaDir temporário.
func newTestConfig(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		MediaDir:     t.TempDir(),
		PlayTokenTTL: time.Hour,
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

// insertVideo insere um vídeo com o status e a tag informados.
func insertVideo(t *testing.T, database *sql.DB, id, status, tag string) {
	t.Helper()
	if _, err := database.Exec("INSERT INTO videos (video_id, status, tag) VALUES (?, ?, ?)", id, status, tag); err != nil {
		t.Fatalf("erro ao inserir vídeo: %v", err)
	}
}

// insertPlayToken cria um token de play para o vídeo e devolve seu valor.
func insertPlayToken(t *testing.T, database *sql.DB, videoID string, expiresAt time.Time) string {
	t.Helper()
	token, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if err := models.InsertAccessToken(database, token, videoID, models.PurposePlay, expiresAt); err != nil {
		t.Fatalf("InsertAccessToken: %v", err)
	}
	return token
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

func masterURL(token string) string {
	return "/video/" + testTag + "/" + testVideoID + ".m3u8?token=" + token
}

// --- Testes do MasterHandler ---

func TestMasterM3U8_ValidToken(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)
	insertVideo(t, database, testVideoID, "ready", testTag)

	const m3u8Content = "#EXTM3U\n#EXT-X-VERSION:3\n"
	writeFile(t, filepath.Join(cfg.MediaDir, testTag, testVideoID, "master.m3u8"), m3u8Content)

	token := insertPlayToken(t, database, testVideoID, time.Now().Add(time.Hour))

	h := NewMasterHandler(cfg, database)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, masterURL(token), nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtido %d (body: %s)", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != m3u8Content {
		t.Fatalf("conteúdo do m3u8 inesperado: %q", rec.Body.String())
	}
}

func TestMasterM3U8_RewritesVariantPaths(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)
	insertVideo(t, database, testVideoID, "ready", testTag)

	const m3u8Content = "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=800000\n480/playlist.m3u8\n"
	writeFile(t, filepath.Join(cfg.MediaDir, testTag, testVideoID, "master.m3u8"), m3u8Content)

	token := insertPlayToken(t, database, testVideoID, time.Now().Add(time.Hour))

	h := NewMasterHandler(cfg, database)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, masterURL(token), nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtido %d", rec.Code)
	}
	want := testVideoID + "/480/playlist.m3u8"
	if !contains(rec.Body.String(), want) {
		t.Fatalf("master reescrito deveria conter %q, obtido: %q", want, rec.Body.String())
	}
}

func TestMasterM3U8_InvalidToken(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)
	insertVideo(t, database, testVideoID, "ready", testTag)
	writeFile(t, filepath.Join(cfg.MediaDir, testTag, testVideoID, "master.m3u8"), "#EXTM3U\n")

	h := NewMasterHandler(cfg, database)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, masterURL("token-inexistente"), nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401, obtido %d", rec.Code)
	}
}

func TestMasterM3U8_ExpiredToken(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)
	insertVideo(t, database, testVideoID, "ready", testTag)
	writeFile(t, filepath.Join(cfg.MediaDir, testTag, testVideoID, "master.m3u8"), "#EXTM3U\n")

	token := insertPlayToken(t, database, testVideoID, time.Now().Add(-time.Hour))

	h := NewMasterHandler(cfg, database)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, masterURL(token), nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401, obtido %d", rec.Code)
	}
}

func TestMasterM3U8_UploadTokenRejected(t *testing.T) {
	// Um token de upload não autoriza play (carimbo de propósito).
	cfg := newTestConfig(t)
	database := newTestDB(t)
	insertVideo(t, database, testVideoID, "ready", testTag)
	writeFile(t, filepath.Join(cfg.MediaDir, testTag, testVideoID, "master.m3u8"), "#EXTM3U\n")

	token, _ := auth.GenerateToken()
	if err := models.InsertAccessToken(database, token, testVideoID, models.PurposeUpload, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	h := NewMasterHandler(cfg, database)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, masterURL(token), nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("token de upload não deveria autorizar play; esperado 401, obtido %d", rec.Code)
	}
}

func TestMasterM3U8_VideoNotReady(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)
	insertVideo(t, database, testVideoID, "transcoding", testTag)
	writeFile(t, filepath.Join(cfg.MediaDir, testTag, testVideoID, "master.m3u8"), "#EXTM3U\n")

	token := insertPlayToken(t, database, testVideoID, time.Now().Add(time.Hour))

	h := NewMasterHandler(cfg, database)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, masterURL(token), nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("esperado 404, obtido %d", rec.Code)
	}
}

func TestMasterM3U8_InvalidVideoID(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)

	h := NewMasterHandler(cfg, database)
	req := httptest.NewRequest(http.MethodGet, "/video/default/nao-uuid.m3u8?token=x", nil)
	req.URL.Path = "/video/default/nao-uuid.m3u8"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("esperado 400, obtido %d", rec.Code)
	}
}

// --- Testes do StaticHandler ---

func TestStaticSegment_ValidPath(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)

	const segContent = "TS_SEGMENT_DATA"
	writeFile(t, filepath.Join(cfg.MediaDir, testTag, testVideoID, "480", "0.ts"), segContent)

	h := NewStaticHandler(cfg, database)
	req := httptest.NewRequest(http.MethodGet, "/video/"+testTag+"/"+testVideoID+"/480/0.ts", nil)
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
	database := newTestDB(t)

	h := NewStaticHandler(cfg, database)
	req := httptest.NewRequest(http.MethodGet, "/video/"+testTag+"/"+testVideoID+"/9999/0.ts", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("esperado 400, obtido %d", rec.Code)
	}
}

func TestStaticSegment_PathTraversal(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)

	h := NewStaticHandler(cfg, database)
	req := httptest.NewRequest(http.MethodGet, "/video/"+testTag+"/"+testVideoID+"/480/x.ts", nil)
	req.URL.Path = "/video/" + testTag + "/" + testVideoID + "/480/../../../etc/passwd"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("esperado 400, obtido %d", rec.Code)
	}
}

func TestStaticServing_NoDirectoryListing(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)

	writeFile(t, filepath.Join(cfg.MediaDir, testTag, testVideoID, "480", "0.ts"), "DATA")

	h := NewStaticHandler(cfg, database)
	req := httptest.NewRequest(http.MethodGet, "/video/"+testTag+"/"+testVideoID+"/480/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("esperado 404 (sem listagem de diretório), obtido %d", rec.Code)
	}
}

func TestStaticSegment_SegmentNotFound(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)

	h := NewStaticHandler(cfg, database)
	req := httptest.NewRequest(http.MethodGet, "/video/"+testTag+"/"+testVideoID+"/480/999.ts", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("esperado 404, obtido %d", rec.Code)
	}
}

// contains é um helper simples (evita import de strings só para isso).
func contains(s, sub string) bool {
	return len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
