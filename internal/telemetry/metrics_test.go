package telemetry

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/klawdyo/streamedia/internal/db"
)

// newTestProvider cria um Provider isolado (registry próprio) para cada
// teste, evitando colisão de coletores entre execuções.
func newTestProvider(t *testing.T) *Provider {
	t.Helper()
	p, err := NewProvider()
	if err != nil {
		t.Fatalf("NewProvider falhou: %v", err)
	}
	return p
}

// scrapeMetrics monta um router mínimo com a rota /metrics e devolve o
// corpo da resposta de uma raspagem.
func scrapeMetrics(t *testing.T, p *Provider) (int, string, http.Header) {
	t.Helper()
	r := chi.NewRouter()
	r.Get("/metrics", p.Handler.ServeHTTP)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	return rec.Code, rec.Body.String(), rec.Header()
}

func TestMetricsRouteServesPrometheusFormat(t *testing.T) {
	p := newTestProvider(t)

	status, body, header := scrapeMetrics(t, p)

	if status != http.StatusOK {
		t.Fatalf("esperado 200, obtido %d", status)
	}

	contentType := header.Get("Content-Type")
	if !strings.Contains(contentType, "text/plain") && !strings.Contains(contentType, "openmetrics-text") {
		t.Errorf("Content-Type inesperado: %q", contentType)
	}

	// Aciona uma requisição instrumentada para garantir que a métrica HTTP
	// apareça na exposição.
	r := chi.NewRouter()
	r.Use(p.Middleware)
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)

	_, body, _ = scrapeMetrics(t, p)
	if !strings.Contains(body, "streamedia_http_requests_total") {
		t.Errorf("esperava \"streamedia_http_requests_total\" na exposição, corpo:\n%s", body)
	}
}

func TestHTTPRequestsCounterIncrements(t *testing.T) {
	p := newTestProvider(t)

	r := chi.NewRouter()
	r.Use(p.Middleware)
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	const n = 3
	for i := 0; i < n; i++ {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		r.ServeHTTP(httptest.NewRecorder(), req)
	}

	_, body, _ := scrapeMetrics(t, p)

	// Verifica que a série referente à rota /healthz com status 200 reflete
	// ao menos as N requisições disparadas (>= para tolerar concorrência
	// com outras possíveis requisições no mesmo provider).
	if !strings.Contains(body, `route="/healthz"`) {
		t.Fatalf("esperava série com route=\"/healthz\" na exposição, corpo:\n%s", body)
	}

	count := extractCounterValue(t, body, "streamedia_http_requests_total", `route="/healthz"`, `status="200"`)
	if count < n {
		t.Errorf("contador de requisições = %v, esperava >= %d", count, n)
	}
}

func TestQueueLengthGaugeReflectsQueueState(t *testing.T) {
	p := newTestProvider(t)

	const expectedLen = 7
	if err := p.RegisterQueueGauge(func() int { return expectedLen }); err != nil {
		t.Fatalf("RegisterQueueGauge falhou: %v", err)
	}

	_, body, _ := scrapeMetrics(t, p)

	if !strings.Contains(body, "streamedia_transcode_queue_length") {
		t.Fatalf("esperava \"streamedia_transcode_queue_length\" na exposição, corpo:\n%s", body)
	}

	value := extractGaugeValue(t, body, "streamedia_transcode_queue_length")
	if value != float64(expectedLen) {
		t.Errorf("streamedia_transcode_queue_length = %v, esperava %d", value, expectedLen)
	}
}

func TestMetricsRouteDoesNotRequireAuth(t *testing.T) {
	p := newTestProvider(t)

	status, _, _ := scrapeMetrics(t, p)
	if status == http.StatusUnauthorized {
		t.Errorf("esperava acesso sem autenticação, obteve 401")
	}
	if status != http.StatusOK {
		t.Errorf("esperado 200, obtido %d", status)
	}
}

func TestPlaybackEventsGaugeReflectsEventCounts(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir banco em memória: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if _, err := database.Exec(`INSERT INTO videos (video_id, status) VALUES ('vid-1', 'ready')`); err != nil {
		t.Fatalf("erro ao inserir vídeo: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO playback_events (video_id, event_type, user_agent, os_family) VALUES
		 ('vid-1', 'playback', 'ua', 'other'),
		 ('vid-1', 'playback', 'ua', 'other'),
		 ('vid-1', 'download_segment', 'ua', 'other')`,
	); err != nil {
		t.Fatalf("erro ao inserir eventos: %v", err)
	}

	p := newTestProvider(t)
	if err := p.RegisterPlaybackEventsGauge(database); err != nil {
		t.Fatalf("RegisterPlaybackEventsGauge falhou: %v", err)
	}

	_, body, _ := scrapeMetrics(t, p)

	playback := extractGaugeValueWithLabel(t, body, "streamedia_playback_events_total", `event_type="playback"`)
	if playback != 2 {
		t.Errorf("streamedia_playback_events_total{event_type=\"playback\"} = %v, esperava 2", playback)
	}

	download := extractGaugeValueWithLabel(t, body, "streamedia_playback_events_total", `event_type="download_segment"`)
	if download != 1 {
		t.Errorf("streamedia_playback_events_total{event_type=\"download_segment\"} = %v, esperava 1", download)
	}
}

// --- Helpers de parsing do formato de exposição Prometheus ---
//
// O formato é texto simples: cada linha de amostra tem o formato
// `nome_da_metrica{label="valor",...} valor`. Para os testes, basta
// localizar a linha que contém o nome da métrica (e, opcionalmente, os
// labels esperados) e extrair o último campo numérico.

func extractGaugeValue(t *testing.T, body, metricName string) float64 {
	t.Helper()
	return extractGaugeValueWithLabel(t, body, metricName, "")
}

func extractGaugeValueWithLabel(t *testing.T, body, metricName, label string) float64 {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, metricName) {
			continue
		}
		if label != "" && !strings.Contains(line, label) {
			continue
		}
		return parseLastField(t, line)
	}
	t.Fatalf("nenhuma linha encontrada para métrica %q (label %q) no corpo:\n%s", metricName, label, body)
	return 0
}

func extractCounterValue(t *testing.T, body, metricName string, labels ...string) float64 {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, metricName) {
			continue
		}
		matchesAll := true
		for _, label := range labels {
			if !strings.Contains(line, label) {
				matchesAll = false
				break
			}
		}
		if !matchesAll {
			continue
		}
		return parseLastField(t, line)
	}
	t.Fatalf("nenhuma linha encontrada para métrica %q (labels %v) no corpo:\n%s", metricName, labels, body)
	return 0
}

func parseLastField(t *testing.T, line string) float64 {
	t.Helper()
	fields := strings.Fields(line)
	if len(fields) == 0 {
		t.Fatalf("linha de métrica vazia: %q", line)
	}
	var value float64
	if _, err := fmt.Sscanf(fields[len(fields)-1], "%g", &value); err != nil {
		t.Fatalf("erro ao converter valor da métrica %q: %v", line, err)
	}
	return value
}
