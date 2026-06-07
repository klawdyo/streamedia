# T29: Rota de métricas no padrão OpenTelemetry/Prometheus

**Status:** pending
**Dependências:** T20, T26
**Estimativa:** média
**Issue relacionada:** #1

## Contexto

A issue #1 pede uma "rota de estatísticas no padrão opentelemetry" — ou
seja, um endpoint de observabilidade compatível com o ecossistema
OpenTelemetry/Prometheus, distinto da rota administrativa de estatísticas
de negócio criada em T28.

Diferença importante:
- **T28 (`/admin/stats`)**: estatísticas de negócio (views por vídeo,
  resolução, SO, dia da semana) — formato JSON, consumido por humanos/admin.
- **T29 (`/metrics`)**: métricas operacionais/observabilidade no formato
  que ferramentas de monitoramento (Prometheus, Grafana, etc.) sabem
  coletar (`scrape`) — formato texto padronizado, exposto via OpenTelemetry.

### Abordagem

Use o SDK oficial `go.opentelemetry.io/otel` com o exporter Prometheus
(`go.opentelemetry.io/otel/exporters/prometheus`), que expõe automaticamente
uma rota compatível com `text/plain; version=0.0.4` no formato esperado por
scrapers Prometheus — esse é o "padrão OpenTelemetry" mais amplamente
adotado para rotas de métricas em serviços HTTP Go.

### Métricas mínimas a expor

Instrumentar contadores/histogramas que reflitam a operação do serviço:

- `streamedia_http_requests_total{method, path, status}` (contador)
- `streamedia_http_request_duration_seconds{method, path}` (histograma)
- `streamedia_playback_events_total{event_type}` (contador, alimentado pelo
  mesmo ponto de instrumentação de T27 — ou derivado de uma leitura
  periódica de `playback_events`, à escolha do Dev; documente a decisão)
- `streamedia_transcode_queue_length` (gauge, reaproveitando `queue.Len()`
  já usado em `/admin/queue`, T18)
- `streamedia_uploads_in_progress` (gauge — derive de uma contagem de vídeos
  com status `uploading`/`processing`, reaproveitando models de T04)

Não é necessário esgotar todas as métricas possíveis nesta tarefa — o
objetivo é estabelecer a infraestrutura (provider, exporter, middleware de
instrumentação HTTP) e cobrir um conjunto mínimo representativo. Deixe um
comentário indicando onde adicionar novas métricas no futuro.

### Rota

```
GET /metrics
```

Sem autenticação (padrão Prometheus — a rota geralmente é protegida na
camada de infraestrutura/rede, não na aplicação). Caso julgue necessário
proteger, documente a decisão em comentário e considere usar o mesmo
mecanismo de rate limiting (T19) para evitar abuso.

## QA Instructions

Crie `internal/telemetry/metrics_test.go`:

```
TestMetricsRouteServesPrometheusFormat
  - GET /metrics → 200
  - Content-Type compatível com Prometheus (text/plain; version=0.0.4 ou
    application/openmetrics-text)
  - Corpo contém ao menos um nome de métrica esperado
    (ex. "streamedia_http_requests_total")

TestHTTPRequestsCounterIncrements
  - Faz N requisições a uma rota instrumentada
  - GET /metrics → corpo reflete contador incrementado em N (ou >= N,
    dependendo de concorrência com outros testes — seja tolerante)

TestQueueLengthGaugeReflectsQueueState
  - Configura uma fila com tamanho conhecido (mock de queue.Len())
  - GET /metrics → valor do gauge "streamedia_transcode_queue_length"
    corresponde ao tamanho configurado

TestMetricsRouteDoesNotRequireAuth
  - GET /metrics sem header Authorization → 200 (não 401)
```

## Dev Instructions

- Crie um pacote `internal/telemetry/` com:
  - `Provider` (ou função `NewMeterProvider`) que configura o
    `MeterProvider` do OpenTelemetry com o exporter Prometheus.
  - `Middleware(next http.Handler) http.Handler` que instrumenta toda
    requisição HTTP (contador de requisições + histograma de duração),
    usando `chi` middleware pattern (compatível com o roteador de T20).
  - Funções/observers para gauges que precisam ler estado dinâmico
    (`queue.Len()`, contagem de uploads em progresso) — use
    `metric.Int64ObservableGauge` com callback, para não exigir polling
    manual.
- Registre o middleware globalmente na montagem do servidor (T20) e a rota
  `/metrics` via `promhttp.Handler()` (do exporter) ou equivalente.
- Adicione as dependências (`go.opentelemetry.io/otel`,
  `go.opentelemetry.io/otel/exporters/prometheus`,
  `go.opentelemetry.io/otel/sdk/metric`) ao `go.mod`.
- Atualize o `README.md` (T24) com uma seção breve sobre observabilidade,
  incluindo a rota `/metrics` e como integrá-la a um Prometheus local.

## Arquivos a criar/modificar

- `internal/telemetry/provider.go`
- `internal/telemetry/middleware.go`
- `internal/telemetry/metrics_test.go`
- `go.mod` / `go.sum`
- Arquivo de montagem do servidor (registro do middleware e da rota `/metrics`)
- `README.md` (seção de observabilidade)

## Definition of Done

- [ ] Rota `/metrics` expõe métricas em formato compatível com Prometheus/OpenTelemetry
- [ ] Contador de requisições HTTP e histograma de duração funcionando
- [ ] Gauge de tamanho da fila de transcodificação reflete estado real
- [ ] Rota acessível sem autenticação (ou decisão documentada em contrário)
- [ ] README atualizado com instruções de observabilidade
- [ ] Todos os testes passam
</content>
