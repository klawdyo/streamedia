package playground

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestServeUI verifica que a página do playground é servida como HTML e contém
// os marcadores das etapas esperadas do fluxo, incluindo o consumo do stream
// de eventos via /api/events.
func TestServeUI(t *testing.T) {
	h := NewHandler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/playground", nil)

	h.ServeUI(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: esperado 200, obtido %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type: esperado text/html, obtido %q", ct)
	}
	body := rec.Body.String()
	// Conteúdos-âncora que garantem que a página certa foi servida.
	for _, want := range []string{"Streamedia", "/api/upload/init", "/api/events", "Players por resolução"} {
		if !strings.Contains(body, want) {
			t.Errorf("página não contém %q", want)
		}
	}
}
