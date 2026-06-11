package dashboard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// serve executa o handler dado e devolve o ResponseRecorder.
func serve(t *testing.T, fn http.HandlerFunc, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	fn(rec, req)
	return rec
}

// TestServePages verifica que cada página do dashboard é servida como HTML e
// contém âncoras que garantem que a página certa foi entregue.
func TestServePages(t *testing.T) {
	h := NewHandler()

	cases := []struct {
		name    string
		fn      http.HandlerFunc
		target  string
		anchors []string
	}{
		{"overview", h.ServeOverview, "/dashboard", []string{"Streamedia", "/admin/stats", "Últimos vídeos", "chart.js"}},
		{"videos", h.ServeVideos, "/dashboard/videos", []string{"Streamedia", "/admin/videos", "Ordenar por"}},
		{"video", h.ServeVideo, "/dashboard/videos/abc", []string{"Streamedia", "/api/play/init", "hls.js", "/api/status/"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := serve(t, c.fn, c.target)
			if rec.Code != http.StatusOK {
				t.Fatalf("status: esperado 200, obtido %d", rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
				t.Errorf("Content-Type: esperado text/html, obtido %q", ct)
			}
			body := rec.Body.String()
			for _, a := range c.anchors {
				if !strings.Contains(body, a) {
					t.Errorf("página %s não contém %q", c.name, a)
				}
			}
		})
	}
}

// TestServeAsset verifica que os assets são servidos com o Content-Type certo
// e que a proteção contra path traversal mantém o acesso restrito a assets/.
func TestServeAsset(t *testing.T) {
	h := NewHandler()

	css := serve(t, h.ServeAsset, "/dashboard/assets/theme.css")
	if css.Code != http.StatusOK {
		t.Fatalf("theme.css: esperado 200, obtido %d", css.Code)
	}
	if ct := css.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
		t.Errorf("theme.css Content-Type = %q, esperava text/css", ct)
	}

	js := serve(t, h.ServeAsset, "/dashboard/assets/app.js")
	if js.Code != http.StatusOK {
		t.Fatalf("app.js: esperado 200, obtido %d", js.Code)
	}
	if ct := js.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/javascript") {
		t.Errorf("app.js Content-Type = %q, esperava application/javascript", ct)
	}

	// Asset inexistente → 404.
	missing := serve(t, h.ServeAsset, "/dashboard/assets/naoexiste.css")
	if missing.Code != http.StatusNotFound {
		t.Errorf("asset inexistente: esperado 404, obtido %d", missing.Code)
	}

	// Path traversal: path.Base reduz ao nome do arquivo; "../dashboard.go"
	// vira "dashboard.go", que não está em assets/ → 404 (nunca serve o fonte).
	traversal := serve(t, h.ServeAsset, "/dashboard/assets/../dashboard.go")
	if traversal.Code != http.StatusNotFound {
		t.Errorf("traversal: esperado 404, obtido %d", traversal.Code)
	}
}
