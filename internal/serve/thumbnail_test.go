package serve

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestThumbnailHandler_ServesExistingFile(t *testing.T) {
	cfg := newTestConfig(t)
	h := NewThumbnailHandler(cfg)

	// Cria o thumbnail no disco: <MediaDir>/default/<id>/thumb_480.jpg.
	path := filepath.Join(cfg.MediaDir, testTag, testVideoID, "thumb_480.jpg")
	writeFile(t, path, "fake-jpeg-bytes")

	req := httptest.NewRequest(http.MethodGet, "/video/"+testTag+"/"+testVideoID+"/thumb_480.jpg", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtido %d (body: %s)", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "fake-jpeg-bytes" {
		t.Fatalf("conteúdo do thumbnail inesperado: %q", rec.Body.String())
	}
}

func TestThumbnailHandler_NotFound(t *testing.T) {
	cfg := newTestConfig(t)
	h := NewThumbnailHandler(cfg)

	// Não cria o arquivo no disco.
	req := httptest.NewRequest(http.MethodGet, "/video/"+testTag+"/"+testVideoID+"/thumb_720.jpg", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("esperado 404, obtido %d", rec.Code)
	}
}

func TestThumbnailHandler_InvalidVideoID(t *testing.T) {
	cfg := newTestConfig(t)
	h := NewThumbnailHandler(cfg)

	req := httptest.NewRequest(http.MethodGet, "/video/"+testTag+"/not-a-uuid/thumb_480.jpg", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("esperado 400, obtido %d", rec.Code)
	}
}

func TestThumbnailHandler_RejectsInvalidFilenames(t *testing.T) {
	cfg := newTestConfig(t)
	h := NewThumbnailHandler(cfg)

	cases := []struct {
		name string
		file string
	}{
		{"resolução não suportada", "thumb_360.jpg"},
		{"extensão errada", "thumb_480.png"},
		{"nome arbitrário", "evil.jpg"},
		{"path traversal", "..%2f..%2fmaster.m3u8"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/video/"+testTag+"/"+testVideoID+"/"+tc.file, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code == http.StatusOK {
				t.Fatalf("esperado erro para filename %q, obtido 200", tc.file)
			}
		})
	}
}
