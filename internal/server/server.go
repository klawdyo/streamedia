// Package server monta o roteador HTTP da aplicação, conectando todos os
// handlers, middlewares e rotas em um único http.Handler testável.
package server

import (
	"database/sql"
	"net/http"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/klawdyo/streamedia/internal/admin"
	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/middleware"
	"github.com/klawdyo/streamedia/internal/models"
	"github.com/klawdyo/streamedia/internal/serve"
	"github.com/klawdyo/streamedia/internal/transcode"
	"github.com/klawdyo/streamedia/internal/upload"
	"github.com/klawdyo/streamedia/internal/webhook"
)

// NewRouter monta e devolve o roteador chi completo da aplicação.
//
// Recebe a config, o banco, a fila de transcodificação (já criada e conectada
// ao worker pelo chamador) e o client de webhook. Todos os handlers são
// construídos internamente. A fila é passada pronta porque ela é criada fora
// (com a função do worker) e também precisa ser iniciada/parada pelo main.
func NewRouter(
	cfg *config.Config,
	database *sql.DB,
	queue *transcode.Queue,
	wc *webhook.Client,
) http.Handler {
	// sendWebhook adapta o client de webhook para a assinatura usada pelos
	// callbacks (videoID, event, errMsg). Busca o vídeo no banco para enviar
	// o payload completo; ignora erros de busca (envia o que tiver).
	sendWebhook := func(videoID, event, errMsg string) {
		video, _ := models.GetVideo(database, videoID)
		_ = wc.Send(videoID, event, video)
	}

	// onFinish é chamado pelo handler TUS quando o upload termina: valida o
	// arquivo, enfileira a transcodificação e dispara webhooks.
	onFinish := func(videoID, userAgent string) {
		filePath := filepath.Join(cfg.UploadTmpDir, videoID)
		upload.HandlePostFinish(database, cfg, queue.Enqueue, sendWebhook, videoID, filePath, userAgent)
	}

	// Constrói todos os handlers da aplicação.
	initHandler := upload.NewInitHandler(cfg, database)
	tusHandler, _ := upload.NewTUSHandler(cfg, database, onFinish)
	masterHandler := serve.NewMasterHandler(cfg, database)
	staticHandler := serve.NewStaticHandler(cfg, database)
	statusHandler := serve.NewStatusHandler(cfg, database)
	adminHandler := admin.NewAdminHandler(cfg, database, queue)
	rateLimiter := middleware.NewRateLimiter(cfg.RateLimitPerMin)

	r := chi.NewRouter()

	// Middlewares globais aplicados a todas as rotas.
	r.Use(chimw.Recoverer)        // recupera de panics, evitando derrubar o servidor
	r.Use(chimw.Logger)           // loga cada requisição
	r.Use(rateLimiter.Middleware) // limita a taxa de requisições por IP

	// --- Upload ---
	// A autenticação dessas rotas é feita dentro do próprio handler (HMAC),
	// por isso não há middleware de auth aqui.
	r.Post("/upload/init", initHandler.ServeHTTP)

	// O handler TUS implementa http.Handler e trata todos os métodos TUS
	// internamente. O chi exige registro explícito de método, então mapeamos
	// cada verbo para o mesmo ServeHTTP.
	r.Post("/files/", tusHandler.ServeHTTP) // criação TUS sem video_id
	r.Post("/files/{videoID}", tusHandler.ServeHTTP)
	r.Patch("/files/{videoID}", tusHandler.ServeHTTP)
	r.Head("/files/{videoID}", tusHandler.ServeHTTP)
	r.Delete("/files/{videoID}", tusHandler.ServeHTTP)

	// --- Serving HLS ---
	// Os handlers fazem o parsing do path internamente (prefixo /videos/).
	r.Get("/videos/{videoID}/master.m3u8", masterHandler.ServeHTTP)
	r.Get("/videos/{videoID}/{res}/playlist.m3u8", staticHandler.ServeHTTP)
	r.Get("/videos/{videoID}/{res}/{segment}", staticHandler.ServeHTTP)

	// --- Status (autenticação HMAC dentro do handler) ---
	r.Get("/api/status/{videoID}", statusHandler.ServeHTTP)

	// --- Admin (protegido por middleware de token) ---
	r.Group(func(r chi.Router) {
		r.Use(admin.AdminAuth(cfg.AdminToken))
		r.Get("/admin/videos", adminHandler.HandleVideos)
		r.Get("/admin/queue", adminHandler.HandleQueue)
		r.Get("/admin/stats", adminHandler.HandleStats)
	})

	// --- Health check ---
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	return r
}
