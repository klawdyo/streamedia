package telemetry

import (
	"context"
	"database/sql"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// RegisterQueueGauge registra um gauge observável que reflete, a cada
// coleta (scrape), o tamanho atual da fila de transcodificação. Usa um
// callback (em vez de polling manual) — o SDK do OpenTelemetry invoca a
// função de observação somente quando alguém lê /metrics.
func (p *Provider) RegisterQueueGauge(queueLen func() int) error {
	gauge, err := p.Meter().Int64ObservableGauge(
		"streamedia_transcode_queue_length",
		metric.WithDescription("Número de vídeos atualmente na fila de transcodificação"),
	)
	if err != nil {
		return err
	}

	_, err = p.Meter().RegisterCallback(
		func(_ context.Context, o metric.Observer) error {
			o.ObserveInt64(gauge, int64(queueLen()))
			return nil
		},
		gauge,
	)
	return err
}

// RegisterPlaybackEventsGauge registra um gauge observável que reflete, a
// cada coleta, a contagem total de eventos de uso (T26/T27) acumulados na
// tabela playback_events, rotulado por event_type.
//
// Optamos por derivar este valor de uma leitura sob demanda da tabela bruta
// (em vez de incrementar um contador em paralelo a cada RecordEvent) para
// manter playback_events como única fonte de verdade — evita divergência
// entre o contador exposto aqui e o que /admin/stats calcula a partir do
// banco (T28).
func (p *Provider) RegisterPlaybackEventsGauge(db *sql.DB) error {
	gauge, err := p.Meter().Int64ObservableGauge(
		"streamedia_playback_events_total",
		metric.WithDescription("Total acumulado de eventos de uso (playback, download_segment, upload_complete) registrados"),
	)
	if err != nil {
		return err
	}

	_, err = p.Meter().RegisterCallback(
		func(ctx context.Context, o metric.Observer) error {
			rows, err := db.QueryContext(ctx,
				`SELECT event_type, COUNT(*) FROM playback_events GROUP BY event_type`,
			)
			if err != nil {
				return err
			}
			defer rows.Close()

			for rows.Next() {
				var eventType string
				var count int64
				if err := rows.Scan(&eventType, &count); err != nil {
					return err
				}
				o.ObserveInt64(gauge, count, metric.WithAttributes(attribute.String("event_type", eventType)))
			}
			return rows.Err()
		},
		gauge,
	)
	return err
}

// RegisterUploadsInProgressGauge registra um gauge observável que reflete,
// a cada coleta, a quantidade de vídeos cujo status indica um upload em
// andamento (pending_upload ou uploading).
//
// Para adicionar novas métricas de domínio no futuro (ex. vídeos em
// transcodificação, vídeos com falha), siga o mesmo padrão: crie um
// ObservableGauge e registre um callback que consulta o banco sob demanda.
func (p *Provider) RegisterUploadsInProgressGauge(db *sql.DB) error {
	gauge, err := p.Meter().Int64ObservableGauge(
		"streamedia_uploads_in_progress",
		metric.WithDescription("Número de vídeos com upload em andamento (pending_upload ou uploading)"),
	)
	if err != nil {
		return err
	}

	_, err = p.Meter().RegisterCallback(
		func(ctx context.Context, o metric.Observer) error {
			var count int64
			err := db.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM videos WHERE status IN ('pending_upload', 'uploading')`,
			).Scan(&count)
			if err != nil {
				return err
			}
			o.ObserveInt64(gauge, count)
			return nil
		},
		gauge,
	)
	return err
}
