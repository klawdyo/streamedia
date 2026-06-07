package telemetry

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// httpMetrics agrupa os instrumentos usados para medir o tráfego HTTP.
type httpMetrics struct {
	requestsTotal   metric.Int64Counter
	requestDuration metric.Float64Histogram
}

// newHTTPMetrics cria os instrumentos de contagem e duração de requisições
// HTTP a partir do Meter do provider.
func newHTTPMetrics(meter metric.Meter) (*httpMetrics, error) {
	requestsTotal, err := meter.Int64Counter(
		"streamedia_http_requests_total",
		metric.WithDescription("Total de requisições HTTP recebidas, por método, rota e status"),
	)
	if err != nil {
		return nil, err
	}

	requestDuration, err := meter.Float64Histogram(
		"streamedia_http_request_duration_seconds",
		metric.WithDescription("Duração das requisições HTTP em segundos, por método e rota"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	return &httpMetrics{requestsTotal: requestsTotal, requestDuration: requestDuration}, nil
}

// statusRecorder envolve um http.ResponseWriter para capturar o status code
// final, já que http.ResponseWriter não expõe esse valor depois de escrito.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (rec *statusRecorder) WriteHeader(status int) {
	rec.status = status
	rec.ResponseWriter.WriteHeader(status)
}

// Middleware instrumenta toda requisição HTTP com um contador de requisições
// e um histograma de duração, ambos rotulados por método, rota (template do
// chi, ex. "/videos/{videoID}/master.m3u8" — evita explosão de séries
// temporais por valores de path) e status HTTP.
//
// Em caso de falha ao criar os instrumentos (extremamente improvável — só
// ocorreria com configuração inválida do Meter), a instrumentação é
// desativada e o handler segue sem registrar métricas: observabilidade
// nunca deve impedir o serviço de funcionar.
func (p *Provider) Middleware(next http.Handler) http.Handler {
	metrics, err := newHTTPMetrics(p.Meter())
	if err != nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		duration := time.Since(start).Seconds()
		route := routeTemplate(r)
		attrs := metric.WithAttributes(
			attribute.String("method", r.Method),
			attribute.String("route", route),
			attribute.String("status", strconv.Itoa(rec.status)),
		)

		metrics.requestsTotal.Add(r.Context(), 1, attrs)
		metrics.requestDuration.Record(r.Context(), duration, attrs)
	})
}

// routeTemplate devolve o template de rota registrado no chi (ex.
// "/videos/{videoID}/master.m3u8") para rotular as métricas sem explodir a
// cardinalidade com valores reais de path (UUIDs, nomes de arquivo, etc.).
// Se o contexto de roteamento não estiver disponível, recorre ao path bruto.
func routeTemplate(r *http.Request) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		if pattern := rctx.RoutePattern(); pattern != "" {
			return pattern
		}
	}
	return r.URL.Path
}
