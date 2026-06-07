package docs

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDocsPage_ReturnsHTML(t *testing.T) {
	h := NewHandler()
	req := httptest.NewRequest(http.MethodGet, "/docs", nil)
	rec := httptest.NewRecorder()

	h.ServePage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("Content-Type = %q, esperado conter text/html", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "scalar") {
		t.Errorf("corpo da página não menciona o componente Scalar: %q", body)
	}
	if !strings.Contains(body, "/docs/openapi.json") {
		t.Errorf("corpo da página não referencia /docs/openapi.json: %q", body)
	}
}

func TestOpenAPISpec_ReturnsValidJSON(t *testing.T) {
	h := NewHandler()
	req := httptest.NewRequest(http.MethodGet, "/docs/openapi.json", nil)
	rec := httptest.NewRecorder()

	h.ServeSpec(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("Content-Type = %q, esperado conter application/json", ct)
	}

	var spec map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &spec); err != nil {
		t.Fatalf("corpo não é JSON válido: %v", err)
	}

	openapi, ok := spec["openapi"].(string)
	if !ok || !strings.HasPrefix(openapi, "3.") {
		t.Errorf("campo \"openapi\" ausente ou não é versão 3.x: %v", spec["openapi"])
	}

	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		t.Fatalf("campo \"paths\" ausente ou não é objeto")
	}

	for _, want := range []string{"/upload/init", "/api/status/{video_id}", "/healthz"} {
		if _, found := paths[want]; !found {
			t.Errorf("rota %q ausente em \"paths\"", want)
		}
	}
}

func TestDocsRoutes_NoAuthRequired(t *testing.T) {
	h := NewHandler()

	pageReq := httptest.NewRequest(http.MethodGet, "/docs", nil)
	pageRec := httptest.NewRecorder()
	h.ServePage(pageRec, pageReq)
	if pageRec.Code != http.StatusOK {
		t.Errorf("/docs sem autenticação: status = %d, esperado %d", pageRec.Code, http.StatusOK)
	}

	specReq := httptest.NewRequest(http.MethodGet, "/docs/openapi.json", nil)
	specRec := httptest.NewRecorder()
	h.ServeSpec(specRec, specReq)
	if specRec.Code != http.StatusOK {
		t.Errorf("/docs/openapi.json sem autenticação: status = %d, esperado %d", specRec.Code, http.StatusOK)
	}
}
