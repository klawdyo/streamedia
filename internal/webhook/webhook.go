// Pacote webhook implementa o cliente de notificação ao backend principal.
package webhook

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/klawdyo/streamedia/internal/auth"
	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/models"
	"github.com/klawdyo/streamedia/internal/notify"
)

// WebhookPayload é o payload enviado ao backend principal. É um alias para
// notify.Notification (a notificação canônica do pipeline), garantindo que
// webhook e SSE entreguem exatamente os mesmos dados.
type WebhookPayload = notify.Notification

// WebhookLogEntry representa um registro na tabela webhook_log.
type WebhookLogEntry struct {
	ID      int64
	VideoID string
	Event   string
	Payload string
	SentAt  time.Time
	Success bool
}

// Client é o cliente de webhook com suporte a retry. Implementa notify.Sink.
type Client struct {
	cfg  *config.Config
	db   *sql.DB
	http *http.Client
	// resolveURL devolve a URL de destino do webhook para um vídeo e se há
	// destino (ok). Quando ok=false, nenhum webhook é enviado. A URL por vídeo
	// (videos.webhook_url, issue #20) tem prioridade sobre a WEBHOOK_URL global;
	// se o vídeo não tiver URL própria, cai na global.
	resolveURL func(videoID string) (url string, ok bool)
}

// NewClient cria um novo cliente de webhook. A URL de destino é resolvida por
// vídeo: se o vídeo tiver uma webhook_url própria (issue #20), ela é usada; do
// contrário, usa a WEBHOOK_URL global. Sem nenhuma das duas, o resolvedor
// devolve ok=false e nenhum webhook é enviado.
func NewClient(cfg *config.Config, db *sql.DB) *Client {
	c := &Client{
		cfg: cfg,
		db:  db,
		// Sem Timeout no client: o timeout por tentativa é controlado pelo
		// context.WithTimeout de 10s em sendAttempt — evita redundância.
		http: &http.Client{},
	}
	c.resolveURL = func(videoID string) (string, bool) {
		// Override por vídeo: a webhook_url do próprio vídeo tem prioridade.
		// Falha de lookup (vídeo inexistente, erro de banco) não é fatal aqui —
		// apenas caímos no destino global, preservando o comportamento anterior.
		if v, err := models.GetVideo(db, videoID); err == nil && v.WebhookURL != "" {
			return v.WebhookURL, true
		}
		return cfg.WebhookURL, cfg.WebhookURL != ""
	}
	return c
}

// Deliver implementa notify.Sink: envia a notificação via webhook se houver
// URL cadastrada para o vídeo. Erros são apenas logados — o fan-out do
// Notifier não propaga erros de sink.
func (c *Client) Deliver(n notify.Notification) {
	url, ok := c.resolveURL(n.VideoID)
	if !ok {
		// Sem URL → nenhum webhook é enviado (o SSE em /api/events segue valendo).
		return
	}
	if err := c.send(n, url); err != nil {
		log.Printf("[webhook] erro ao enviar evento %s do vídeo %s: %v", n.Event, n.VideoID, err)
	}
}

// Send envia um webhook para o vídeo/evento informados, com retry. Mantido
// para chamadas diretas e testes; devolve o erro do envio (ou nil se não há
// URL configurada). O fluxo de produção usa Deliver, via Notifier.
func (c *Client) Send(videoID, event string, video *models.Video) error {
	url, ok := c.resolveURL(videoID)
	if !ok {
		return nil
	}
	return c.send(notify.Build(videoID, event, video), url)
}

// send serializa, assina e envia a notificação para a URL informada, com até
// 3 tentativas (backoff exponencial 1s/2s/4s), registrando cada uma na tabela
// webhook_log.
func (c *Client) send(n notify.Notification, url string) error {
	payloadBytes, err := json.Marshal(n)
	if err != nil {
		return fmt.Errorf("erro ao serializar payload: %w", err)
	}

	// Assina a requisição (apenas se WEBHOOK_SECRET estiver definido)
	var signature string
	if c.cfg.WebhookSecret != "" {
		signature = auth.SignWebhook(c.cfg.WebhookSecret, payloadBytes)
	}

	// Realiza até 3 tentativas com backoff exponencial
	backoffs := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}
	var lastErr error

	for attempt := 0; attempt < 3; attempt++ {
		// Tenta enviar o webhook
		success, err := c.sendAttempt(url, payloadBytes, signature)

		// Registra a tentativa no banco
		logErr := insertWebhookLog(c.db, n.VideoID, n.Event, string(payloadBytes), success)
		if logErr != nil {
			return fmt.Errorf("erro ao registrar tentativa de webhook: %w", logErr)
		}

		if success {
			return nil
		}

		lastErr = err
		if attempt < 2 {
			// Aguarda antes da próxima tentativa
			time.Sleep(backoffs[attempt])
		}
	}

	return fmt.Errorf("webhook falhou após 3 tentativas: %w", lastErr)
}

// sendAttempt realiza uma tentativa de envio do webhook para a URL informada.
// Retorna true se o envio foi bem-sucedido (status 2xx), false caso contrário.
func (c *Client) sendAttempt(url string, payloadBytes []byte, signature string) (bool, error) {
	// Cria um contexto com timeout de 10 segundos
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Cria a requisição
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payloadBytes))
	if err != nil {
		return false, fmt.Errorf("erro ao criar requisição: %w", err)
	}

	// Define headers
	req.Header.Set("Content-Type", "application/json")
	if signature != "" {
		req.Header.Set("X-Signature", fmt.Sprintf("sha256=%s", signature))
	}

	// Envia a requisição
	resp, err := c.http.Do(req)
	if err != nil {
		return false, fmt.Errorf("erro ao enviar requisição: %w", err)
	}
	defer resp.Body.Close()

	// Verifica se a resposta é 2xx
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true, nil
	}

	return false, fmt.Errorf("status HTTP %d recebido", resp.StatusCode)
}

// insertWebhookLog registra uma tentativa de envio de webhook no banco.
func insertWebhookLog(db *sql.DB, videoID, event, payload string, success bool) error {
	successInt := 0
	if success {
		successInt = 1
	}

	_, err := db.Exec(
		"INSERT INTO webhook_log (video_id, event, payload, success) VALUES (?, ?, ?, ?)",
		videoID, event, payload, successInt,
	)
	if err != nil {
		return fmt.Errorf("erro ao inserir webhook_log: %w", err)
	}
	return nil
}

// GetWebhookLog busca todos os registros de webhook para um videoID.
func GetWebhookLog(db *sql.DB, videoID string) ([]*WebhookLogEntry, error) {
	rows, err := db.Query(
		"SELECT id, video_id, event, payload, sent_at, success FROM webhook_log WHERE video_id = ? ORDER BY sent_at",
		videoID,
	)
	if err != nil {
		return nil, fmt.Errorf("erro ao consultar webhook_log: %w", err)
	}
	defer rows.Close()

	var entries []*WebhookLogEntry
	for rows.Next() {
		var entry WebhookLogEntry
		var successInt int

		err := rows.Scan(&entry.ID, &entry.VideoID, &entry.Event, &entry.Payload, &entry.SentAt, &successInt)
		if err != nil {
			return nil, fmt.Errorf("erro ao ler linha de webhook_log: %w", err)
		}

		entry.Success = successInt == 1
		entries = append(entries, &entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erro ao iterar webhook_log: %w", err)
	}

	return entries, nil
}
