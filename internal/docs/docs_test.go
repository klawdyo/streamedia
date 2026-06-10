package docs

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestScalarUIRouteServesHTML garante que a UI interativa do Scalar é
// servida como HTML, referencia "scalar" — sinal de que carrega o
// componente correto — e aponta para a spec em /docs/openapi.json.
func TestScalarUIRouteServesHTML(t *testing.T) {
	h := NewHandler()

	req := httptest.NewRequest(http.MethodGet, "/docs/", nil)
	rec := httptest.NewRecorder()
	h.ServeUI(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: esperado %d, obtido %d", http.StatusOK, rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "text/html") {
		t.Errorf("Content-Type: esperado prefixo %q, obtido %q", "text/html", contentType)
	}

	body := strings.ToLower(rec.Body.String())
	if !strings.Contains(body, "scalar") {
		t.Errorf("corpo da resposta não contém referência a 'scalar': %s", rec.Body.String())
	}
	if !strings.Contains(body, "/docs/openapi.json") {
		t.Errorf("corpo da resposta não referencia /docs/openapi.json: %s", rec.Body.String())
	}
}

// TestOpenAPISpecIsValidJSON garante que a especificação é JSON válido e
// contém os campos obrigatórios de um documento OpenAPI/Swagger.
func TestOpenAPISpecIsValidJSON(t *testing.T) {
	h := NewHandler()

	req := httptest.NewRequest(http.MethodGet, "/docs/openapi.json", nil)
	rec := httptest.NewRecorder()
	h.ServeOpenAPISpec(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: esperado %d, obtido %d", http.StatusOK, rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") {
		t.Errorf("Content-Type: esperado prefixo %q, obtido %q", "application/json", contentType)
	}

	var spec map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &spec); err != nil {
		t.Fatalf("corpo não é JSON válido: %v", err)
	}

	for _, field := range []string{"info", "paths"} {
		if _, ok := spec[field]; !ok {
			t.Errorf("spec não contém o campo obrigatório %q", field)
		}
	}

	if _, hasOpenAPI := spec["openapi"]; !hasOpenAPI {
		if _, hasSwagger := spec["swagger"]; !hasSwagger {
			t.Error(`spec não contém nem "openapi" nem "swagger" — versão da especificação ausente`)
		}
	}
}

// TestOpenAPISpecDocumentsKnownRoutes garante que as rotas centrais da API
// (upload, status, admin/stats) aparecem na especificação.
func TestOpenAPISpecDocumentsKnownRoutes(t *testing.T) {
	h := NewHandler()

	req := httptest.NewRequest(http.MethodGet, "/docs/openapi.json", nil)
	rec := httptest.NewRecorder()
	h.ServeOpenAPISpec(rec, req)

	var spec map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &spec); err != nil {
		t.Fatalf("corpo não é JSON válido: %v", err)
	}

	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		t.Fatal("spec não contém um objeto 'paths'")
	}

	for _, wantPath := range []string{"/api/upload/init", "/api/status/{video_id}", "/admin/stats"} {
		if _, ok := paths[wantPath]; !ok {
			t.Errorf("spec não documenta o path %q (paths presentes: %v)", wantPath, keysOf(paths))
		}
	}
}

func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
