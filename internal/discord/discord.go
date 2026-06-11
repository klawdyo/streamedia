// Pacote discord envia alertas operacionais para um webhook do Discord
// (issue #21). É um canal de ALERTA INTERNO para operadores, distinto do
// webhook de negócio (internal/webhook): aquele notifica o backend principal
// sobre transições de estado do vídeo (processing/ready/failed); este avisa a
// equipe sobre FALHAS operacionais que comprometem o funcionamento do serviço.
//
// É totalmente OPCIONAL: sem DISCORD_WEBHOOK_URL configurada, NewAlerter
// devolve nil e todos os métodos viram no-op (são seguros para receptor nil).
// O envio é best-effort — cada tentativa (sucesso/falha) é registrada no log
// da aplicação e nunca propaga erro nem bloqueia o pipeline.
package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// consecutiveFailureThreshold é o número de falhas terminais de
// transcodificação CONSECUTIVAS (sem nenhum sucesso entre elas) que dispara o
// alerta de "aumento anormal de falhas". Sinaliza um problema sistêmico
// (ex.: ffmpeg quebrado, disco cheio) em vez de uma falha isolada de um vídeo.
const consecutiveFailureThreshold = 5

// Cores (decimal) usadas nos embeds do Discord.
const (
	colorRed    = 0xE74C3C // falhas (failed_transcode, falhas consecutivas)
	colorOrange = 0xE67E22 // alertas de capacidade/saúde (fila cheia, transcode travado)
)

// sendTimeout limita cada POST ao webhook do Discord.
const sendTimeout = 10 * time.Second

// Alerter envia alertas para um webhook do Discord. Use NewAlerter para criar;
// um *Alerter nil é válido e torna todos os métodos no-op (canal desabilitado).
type Alerter struct {
	url  string
	http *http.Client

	// consecutiveFailures conta falhas terminais de transcodificação seguidas.
	// Protegido por mu porque o worker e o job de requeue podem reportar falhas
	// de goroutines diferentes.
	mu                  sync.Mutex
	consecutiveFailures int
}

// NewAlerter cria um Alerter para a URL informada. Se webhookURL for vazia,
// devolve nil — o canal fica desabilitado e todos os métodos viram no-op.
func NewAlerter(webhookURL string) *Alerter {
	if webhookURL == "" {
		return nil
	}
	return &Alerter{
		url:  webhookURL,
		http: &http.Client{Timeout: sendTimeout},
	}
}

// embedField é um campo "name/value" de um embed do Discord.
type embedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

// embed é um embed do Discord (cartão colorido com título e campos).
type embed struct {
	Title     string       `json:"title"`
	Color     int          `json:"color"`
	Fields    []embedField `json:"fields,omitempty"`
	Timestamp string       `json:"timestamp"`
}

// webhookPayload é o corpo JSON do webhook do Discord.
type webhookPayload struct {
	Embeds []embed `json:"embeds"`
}

// AlertTranscodeFailure alerta sobre uma falha TERMINAL de transcodificação
// (o vídeo esgotou as tentativas e foi marcado como failed_transcode). Também
// alimenta o contador de falhas consecutivas: ao atingir o limiar, dispara um
// alerta adicional de "aumento anormal de falhas".
func (a *Alerter) AlertTranscodeFailure(videoID, status, errMsg string) {
	if a == nil {
		return
	}
	if errMsg == "" {
		errMsg = "(sem mensagem de erro)"
	}
	a.send(embed{
		Title: "❌ Falha na transcodificação",
		Color: colorRed,
		Fields: []embedField{
			{Name: "video_id", Value: videoID, Inline: true},
			{Name: "status", Value: status, Inline: true},
			{Name: "error_message", Value: truncate(errMsg, 1024)},
		},
		Timestamp: now(),
	})

	// Contabiliza falhas consecutivas e, ao cruzar o limiar, alerta o problema
	// sistêmico e zera o contador (evita disparar a cada falha subsequente).
	a.mu.Lock()
	a.consecutiveFailures++
	n := a.consecutiveFailures
	cross := n >= consecutiveFailureThreshold
	if cross {
		a.consecutiveFailures = 0
	}
	a.mu.Unlock()

	if cross {
		a.send(embed{
			Title: "🚨 Aumento anormal de falhas de transcodificação",
			Color: colorRed,
			Fields: []embedField{
				{Name: "falhas_consecutivas", Value: fmt.Sprintf("%d", n), Inline: true},
				{Name: "última_falha", Value: videoID, Inline: true},
			},
			Timestamp: now(),
		})
	}
}

// RecordTranscodeSuccess zera o contador de falhas consecutivas — chamado pelo
// worker quando um vídeo conclui a transcodificação com sucesso (ready).
func (a *Alerter) RecordTranscodeSuccess() {
	if a == nil {
		return
	}
	a.mu.Lock()
	a.consecutiveFailures = 0
	a.mu.Unlock()
}

// AlertQueueFull alerta que a fila de transcodificação está cheia e novos jobs
// estão sendo bloqueados (rejeitados). videoID é o vídeo que não pôde entrar.
func (a *Alerter) AlertQueueFull(videoID string) {
	if a == nil {
		return
	}
	a.send(embed{
		Title: "⚠️ Fila de transcodificação cheia",
		Color: colorOrange,
		Fields: []embedField{
			{Name: "video_id", Value: videoID, Inline: true},
			{Name: "status", Value: "queue_full", Inline: true},
			{Name: "error_message", Value: "A fila de transcodificação está cheia; novos jobs estão sendo bloqueados."},
		},
		Timestamp: now(),
	})
}

// AlertTranscodeStuck alerta que uma transcodificação travada foi detectada
// pelo job de manutenção (passou do TRANSCODE_STUCK sem progredir). attempts é
// o número de tentativas já acumuladas pelo vídeo.
func (a *Alerter) AlertTranscodeStuck(videoID string, attempts int) {
	if a == nil {
		return
	}
	a.send(embed{
		Title: "⏱️ Transcodificação travada detectada",
		Color: colorOrange,
		Fields: []embedField{
			{Name: "video_id", Value: videoID, Inline: true},
			{Name: "status", Value: "transcoding", Inline: true},
			{Name: "tentativas", Value: fmt.Sprintf("%d", attempts), Inline: true},
			{Name: "error_message", Value: "Transcode travado (TRANSCODE_STUCK) detectado pelo job de manutenção."},
		},
		Timestamp: now(),
	})
}

// send serializa e envia o embed ao webhook do Discord. Best-effort: registra
// o resultado no log e nunca propaga erro.
func (a *Alerter) send(e embed) {
	body, err := json.Marshal(webhookPayload{Embeds: []embed{e}})
	if err != nil {
		log.Printf("[discord] erro ao serializar alerta %q: %v", e.Title, err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.url, bytes.NewReader(body))
	if err != nil {
		log.Printf("[discord] erro ao criar requisição do alerta %q: %v", e.Title, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.http.Do(req)
	if err != nil {
		log.Printf("[discord] falha ao enviar alerta %q: %v", e.Title, err)
		return
	}
	defer resp.Body.Close()

	// O Discord responde 204 (No Content) em sucesso; aceitamos qualquer 2xx.
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Printf("[discord] alerta %q enviado com sucesso (status %d)", e.Title, resp.StatusCode)
		return
	}
	log.Printf("[discord] alerta %q rejeitado pelo Discord: status HTTP %d", e.Title, resp.StatusCode)
}

// now devolve o timestamp atual em RFC3339 (formato esperado pelo Discord).
func now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// truncate corta a string em no máximo max runes, anexando "…" quando cortada.
// O Discord limita cada campo de embed a 1024 caracteres.
func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return string(r[:max])
	}
	return string(r[:max-1]) + "…"
}
