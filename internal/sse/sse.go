// Pacote sse entrega notificações do pipeline ao vivo via Server-Sent Events
// (SSE), na rota GET /api/events. É um destino (notify.Sink) par a par com o
// webhook: o mesmo evento que vira webhook é empurrado para quem estiver
// ouvindo aquele vídeo.
//
// O stream é escopado por video_id e autenticado pelo token de upload do
// vídeo (na query string, pois EventSource não permite enviar cabeçalhos).
// Assim um app de usuário pode acompanhar o próprio upload/transcodificação
// direto, sem rotear pelo backend principal e sem expor o ROOT_TOKEN.
//
// Não há buffer/replay: o SSE entrega apenas o que ocorre enquanto o cliente
// está conectado (eventos passados não são re-emitidos).
package sse

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/klawdyo/streamedia/internal/apiresponse"
	"github.com/klawdyo/streamedia/internal/notify"
)

// subscriber é um ouvinte conectado: um canal por onde recebe as notificações
// do seu vídeo. O canal é bufferizado para absorver picos; se encher (cliente
// lento), os eventos excedentes são descartados em vez de bloquear o emissor.
type subscriber struct {
	ch chan notify.Notification
}

// Hub mantém os ouvintes ativos indexados por video_id e distribui as
// notificações. Implementa notify.Sink.
type Hub struct {
	mu   sync.RWMutex
	subs map[string]map[*subscriber]struct{}
}

// NewHub cria um Hub vazio.
func NewHub() *Hub {
	return &Hub{subs: make(map[string]map[*subscriber]struct{})}
}

// subscribe registra um novo ouvinte para o vídeo e devolve seu subscriber.
func (h *Hub) subscribe(videoID string) *subscriber {
	s := &subscriber{ch: make(chan notify.Notification, 16)}
	h.mu.Lock()
	if h.subs[videoID] == nil {
		h.subs[videoID] = make(map[*subscriber]struct{})
	}
	h.subs[videoID][s] = struct{}{}
	h.mu.Unlock()
	return s
}

// unsubscribe remove o ouvinte do índice. NÃO fecha o canal de propósito: um
// Deliver concorrente pode já ter copiado este subscriber como alvo; deixar o
// canal aberto (bufferizado, com envio não-bloqueante) evita pânico de "send
// on closed channel". Sem referências, o canal é coletado pelo GC.
func (h *Hub) unsubscribe(videoID string, s *subscriber) {
	h.mu.Lock()
	if set := h.subs[videoID]; set != nil {
		delete(set, s)
		if len(set) == 0 {
			delete(h.subs, videoID)
		}
	}
	h.mu.Unlock()
}

// Deliver implementa notify.Sink: empurra a notificação para todos os ouvintes
// do vídeo. Envio não-bloqueante — se o canal de um ouvinte estiver cheio, o
// evento é descartado para aquele ouvinte (não trava os demais nem o pipeline).
func (h *Hub) Deliver(n notify.Notification) {
	h.mu.RLock()
	set := h.subs[n.VideoID]
	targets := make([]*subscriber, 0, len(set))
	for s := range set {
		targets = append(targets, s)
	}
	h.mu.RUnlock()

	for _, s := range targets {
		select {
		case s.ch <- n:
		default:
			log.Printf("[sse] canal cheio para vídeo %s — evento %s descartado", n.VideoID, n.Event)
		}
	}
}

// SubscriberCount devolve quantos ouvintes ativos há para um vídeo (usado em
// testes e diagnóstico).
func (h *Hub) SubscriberCount(videoID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subs[videoID])
}

// Authorizer valida se um token autoriza ouvir os eventos de um vídeo. A
// implementação de produção checa o token de upload (purpose=upload, não
// expirado, pertencente ao video_id) — os mesmos critérios do handler TUS.
type Authorizer func(token, videoID string) bool

// Handler serve o stream SSE em GET /api/events.
type Handler struct {
	hub  *Hub
	auth Authorizer
}

// NewHandler cria o handler de SSE com o hub e o autorizador informados.
func NewHandler(hub *Hub, auth Authorizer) *Handler {
	return &Handler{hub: hub, auth: auth}
}

// heartbeatInterval é o intervalo entre comentários de keepalive, que mantêm
// a conexão viva através de proxies e ajudam a detectar desconexão.
const heartbeatInterval = 25 * time.Second

// ServeHTTP valida o token + video_id e mantém o stream SSE aberto, escrevendo
// cada notificação como um evento até o cliente desconectar.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	videoID := r.URL.Query().Get("video_id")
	token := r.URL.Query().Get("token")
	if videoID == "" || token == "" {
		apiresponse.Error(w, http.StatusBadRequest, "Parâmetros 'video_id' e 'token' são obrigatórios.")
		return
	}
	if h.auth == nil || !h.auth(token, videoID) {
		apiresponse.Error(w, http.StatusUnauthorized, "Token inválido ou não corresponde ao vídeo.")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		apiresponse.Error(w, http.StatusInternalServerError, "Streaming não suportado pelo servidor.")
		return
	}

	// Cabeçalhos do protocolo SSE.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // desativa buffering em proxies (nginx)
	w.WriteHeader(http.StatusOK)

	sub := h.hub.subscribe(videoID)
	defer h.hub.unsubscribe(videoID, sub)

	// Comentário inicial — abre o stream e confirma a conexão.
	fmt.Fprintf(w, ": conectado ao stream do vídeo %s\n\n", videoID)
	flusher.Flush()

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			// Cliente desconectou.
			return
		case n := <-sub.ch:
			data, err := json.Marshal(n)
			if err != nil {
				log.Printf("[sse] erro ao serializar notificação do vídeo %s: %v", videoID, err)
				continue
			}
			// Formato SSE: linha 'event:' opcional + 'data:' + linha em branco.
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", n.Event, data)
			flusher.Flush()
		case <-ticker.C:
			// Keepalive (comentário SSE, ignorado pelo cliente).
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}
