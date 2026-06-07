// Pacote webhook implementa o cliente de notificação ao backend principal.
package webhook

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/klawdyo/streamedia/internal/auth"
	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/models"
)

// WebhookPayload representa os dados enviados ao webhook do backend principal.
type WebhookPayload struct {
	VideoID      string `json:"video_id"`
	Event        string `json:"event"`
	Status       string `json:"status"`
	DurationS    *int   `json:"duration_s"`
	Resolutions  []int  `json:"resolutions"`
	ErrorMessage *string `json:"error_message"`
	Timestamp    time.Time `json:"timestamp"`
}

// WebhookLogEntry representa um registro na tabela webhook_log.
type WebhookLogEntry struct {
	ID      int64
	VideoID string
	Event   string
	Payload string
	SentAt  time.Time
	Success bool
}

// Client é o cliente de webhook com suporte a retry.
type Client struct {
	cfg  *config.Config
	db   *sql.DB
	http *http.Client
}

// NewClient cria um novo cliente de webhook.
func NewClient(cfg *config.Config, db *sql.DB) *Client {
	return &Client{
		cfg: cfg,
		db:  db,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Send envia um webhook ao backend principal com retry automático.
// Realiza até 3 tentativas com backoff exponencial (1s, 2s, 4s).
// Registra cada tentativa na tabela webhook_log.
func (c *Client) Send(videoID, event string, video *models.Video) error {
	// Constrói o payload a partir do vídeo
	payload := buildPayload(videoID, event, video)

	// Marshala para JSON
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("erro ao serializar payload: %w", err)
	}

	// Assina a requisição
	signature := auth.SignBackendRequest(c.cfg.WebhookSecret, payloadBytes)

	// Realiza até 3 tentativas com backoff exponencial
	backoffs := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}
	var lastErr error

	for attempt := 0; attempt < 3; attempt++ {
		// Tenta enviar o webhook
		success, err := c.sendAttempt(payloadBytes, signature)

		// Registra a tentativa no banco
		logErr := insertWebhookLog(c.db, videoID, event, string(payloadBytes), success)
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

// sendAttempt realiza uma tentativa de envio do webhook.
// Retorna true se o envio foi bem-sucedido (status 2xx), false caso contrário.
func (c *Client) sendAttempt(payloadBytes []byte, signature string) (bool, error) {
	// Cria um contexto com timeout de 10 segundos
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Cria a requisição
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.WebhookURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return false, fmt.Errorf("erro ao criar requisição: %w", err)
	}

	// Define headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Signature", fmt.Sprintf("sha256=%s", signature))

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

// buildPayload constrói o payload a partir dos dados do vídeo.
// Trata corretamente os campos opcionais (DurationS e ErrorMessage como ponteiros).
func buildPayload(videoID, event string, video *models.Video) *WebhookPayload {
	payload := &WebhookPayload{
		VideoID:   videoID,
		Event:     event,
		Status:    string(video.Status),
		Timestamp: time.Now().UTC(),
	}

	// Define DurationS como ponteiro (nil se 0, ou *int se > 0)
	if video.DurationS > 0 {
		duration := video.DurationS
		payload.DurationS = &duration
	}

	// Define Resolutions com o slice do vídeo (vazio se nil)
	if video.Resolutions != nil && len(video.Resolutions) > 0 {
		payload.Resolutions = video.Resolutions
	} else {
		payload.Resolutions = []int{}
	}

	// Define ErrorMessage como ponteiro (nil se vazio, ou *string se preenchido)
	if video.ErrorMessage != "" {
		payload.ErrorMessage = &video.ErrorMessage
	}

	return payload
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
