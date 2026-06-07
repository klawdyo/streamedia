// Pacote telemetry expõe métricas operacionais do Streamedia no padrão
// OpenTelemetry, coletáveis via scraping no formato Prometheus (issue #1).
//
// Diferença em relação ao pacote internal/admin (rota /admin/stats, T28):
// aquela rota expõe estatísticas de NEGÓCIO (views por vídeo, resolução,
// SO, dia da semana) em JSON, consumidas por administradores humanos. Este
// pacote expõe métricas OPERACIONAIS/observabilidade (taxa de requisições,
// latência, tamanho de filas) no formato que ferramentas como Prometheus e
// Grafana sabem coletar (scrape) — texto padronizado, não JSON.
package telemetry

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	otelprometheus "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// meterName identifica o instrumentation scope das métricas do Streamedia.
const meterName = "github.com/klawdyo/streamedia"

// Provider agrega tudo que é necessário para expor métricas OpenTelemetry
// no formato Prometheus: o MeterProvider (usado para criar instrumentos —
// contadores, histogramas, gauges) e o http.Handler que serve a rota de
// scraping (/metrics).
//
// Usamos um *prometheus.Registry próprio (em vez do registry global do
// cliente Prometheus) para que múltiplas instâncias — por exemplo em
// testes — não colidam ao registrar coletores com o mesmo nome.
type Provider struct {
	MeterProvider *sdkmetric.MeterProvider
	Handler       http.Handler
}

// NewProvider cria um Provider com um MeterProvider OpenTelemetry
// configurado com o exporter Prometheus (registrado em um registry próprio,
// não no global) e o http.Handler pronto para ser registrado na rota
// /metrics pelo chamador.
func NewProvider() (*Provider, error) {
	registry := prometheus.NewRegistry()

	exporter, err := otelprometheus.New(otelprometheus.WithRegisterer(registry))
	if err != nil {
		return nil, err
	}

	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))

	return &Provider{
		MeterProvider: mp,
		Handler:       promhttp.HandlerFor(registry, promhttp.HandlerOpts{}),
	}, nil
}

// Meter devolve o Meter padrão do Streamedia, usado para criar os
// instrumentos (contadores, histogramas, gauges observáveis).
//
// Para adicionar novas métricas no futuro, crie o instrumento aqui (ou em
// um novo arquivo do pacote) a partir deste Meter e instrumente o ponto de
// interesse — seja via Middleware (para rotas HTTP) ou diretamente no
// código relevante (para métricas de domínio, como filas e contadores).
func (p *Provider) Meter() metric.Meter {
	return p.MeterProvider.Meter(meterName)
}
