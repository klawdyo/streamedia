// Pacote notify centraliza a emissão de eventos do pipeline (upload concluído,
// transcodificação pronta, falha, etc.) como "notificações" e as distribui
// para um ou mais destinos (sinks): o cliente de webhook e o hub de SSE.
//
// Antes, cada evento ia direto para o webhook. Generalizar para notificações
// permite que o mesmo evento alimente, em paralelo, o webhook (se houver URL
// cadastrada) e qualquer cliente ouvindo via SSE (GET /api/events) — sem o
// pipeline saber quem está escutando.
package notify

import (
	"database/sql"
	"log"
	"time"

	"github.com/klawdyo/streamedia/internal/models"
)

// Notification é o payload canônico de um evento do pipeline. É a mesma
// estrutura entregue ao webhook (com os mesmos campos/JSON) e empurrada pelo
// SSE — "os mesmos dados, os mesmos eventos".
type Notification struct {
	VideoID      string    `json:"video_id"`
	Event        string    `json:"event"`
	Status       string    `json:"status"`
	DurationS    *int      `json:"duration_s"`
	Resolutions  []int     `json:"resolutions"`
	ErrorMessage *string   `json:"error_message"`
	Timestamp    time.Time `json:"timestamp"`
}

// Build monta uma Notification a partir do estado atual do vídeo. Campos
// opcionais (DurationS, ErrorMessage) viram ponteiros nil quando ausentes;
// Resolutions nunca é nil (vira []int{}).
func Build(videoID, event string, video *models.Video) Notification {
	n := Notification{
		VideoID:     videoID,
		Event:       event,
		Status:      string(video.Status),
		Timestamp:   time.Now().UTC(),
		Resolutions: []int{},
	}
	if video.DurationS > 0 {
		d := video.DurationS
		n.DurationS = &d
	}
	if len(video.Resolutions) > 0 {
		n.Resolutions = video.Resolutions
	}
	if video.ErrorMessage != "" {
		msg := video.ErrorMessage
		n.ErrorMessage = &msg
	}
	return n
}

// Sink é um destino de notificações. Cada destino decide internamente se e
// para quem entrega (o webhook só envia se houver URL; o SSE só empurra se
// houver ouvinte daquele vídeo). Deliver não deve bloquear o chamador por
// muito tempo — o Notifier já invoca cada sink em sua própria goroutine.
type Sink interface {
	Deliver(Notification)
}

// Notifier busca o vídeo no banco, monta a Notification e faz o fan-out para
// todos os sinks registrados.
type Notifier struct {
	db    *sql.DB
	sinks []Sink
}

// New cria um Notifier com os sinks informados (ex.: webhook, hub de SSE).
func New(db *sql.DB, sinks ...Sink) *Notifier {
	return &Notifier{db: db, sinks: sinks}
}

// Notify mantém a assinatura dos callbacks já usados no pipeline
// (videoID, event, errMsg) para ser um substituto direto do antigo
// sendWebhook. O parâmetro errMsg é aceito por compatibilidade, mas a
// mensagem de erro efetiva vem do próprio vídeo (video.ErrorMessage, gravado
// no banco antes da notificação) — preservando o comportamento anterior.
//
// O fan-out é feito em goroutines: um sink lento (ex.: webhook com retry e
// backoff) não atrasa os demais nem o pipeline. Cada sink trata seus próprios
// erros internamente.
func (no *Notifier) Notify(videoID, event, _ string) {
	if no == nil {
		return
	}
	video, err := models.GetVideo(no.db, videoID)
	if err != nil {
		log.Printf("[notify] erro ao buscar vídeo %s para evento %s: %v", videoID, event, err)
		return
	}
	n := Build(videoID, event, video)
	for _, s := range no.sinks {
		s := s
		go s.Deliver(n)
	}
}
