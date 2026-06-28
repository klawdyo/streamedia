// Package server monta o roteador HTTP da aplicação, conectando todos os
// handlers, middlewares e rotas em um único http.Handler testável.
package server

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/klawdyo/streamedia/internal/admin"
	"github.com/klawdyo/streamedia/internal/apiresponse"
	"github.com/klawdyo/streamedia/internal/auth/google"
	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/middleware"
	"github.com/klawdyo/streamedia/internal/models"
	"github.com/klawdyo/streamedia/internal/notify"
	"github.com/klawdyo/streamedia/internal/serve"
	"github.com/klawdyo/streamedia/internal/spa"
	"github.com/klawdyo/streamedia/internal/sse"
	"github.com/klawdyo/streamedia/internal/telemetry"
	"github.com/klawdyo/streamedia/internal/transcode"
	"github.com/klawdyo/streamedia/internal/upload"
	"github.com/klawdyo/streamedia/internal/version"
)

// NewRouter monta e devolve o roteador chi completo da aplicação e um
// io.Closer que deve ser chamado no shutdown para liberar recursos internos
// (goroutines do TUS handler, etc.). O chamador é responsável por chamar
// Close() antes de encerrar o servidor (T59).
//
// Recebe a config, o banco, a fila de transcodificação (já criada e conectada
// ao worker pelo chamador), o notifier (que faz fan-out dos eventos para
// webhook + SSE) e o hub de SSE (para a rota /api/events). Todos os handlers
// são construídos internamente. A fila e o notifier são passados prontos
// porque são compartilhados com o worker/jobs, criados pelo main.
func NewRouter(
	cfg *config.Config,
	database *sql.DB,
	queue *transcode.Queue,
	notifier *notify.Notifier,
	hub *sse.Hub,
) (http.Handler, io.Closer, error) {
	// onFinish é chamado pelo handler TUS quando o upload termina: valida o
	// arquivo, enfileira a transcodificação e emite a notificação (webhook + SSE).
	onFinish := func(videoID, userAgent string) {
		filePath := filepath.Join(cfg.UploadTmpDir, videoID)
		upload.HandlePostFinish(database, cfg, queue.Enqueue, notifier.Notify, videoID, filePath, userAgent)
	}

	// Constrói todos os handlers da aplicação.
	initHandler := upload.NewInitHandler(cfg, database)
	tusHandler, err := upload.NewTUSHandler(cfg, database, onFinish)
	if err != nil {
		return nil, nil, fmt.Errorf("falha ao criar handler TUS: %w", err)
	}
	masterHandler := serve.NewMasterHandler(cfg, database)
	staticHandler := serve.NewStaticHandler(cfg, database)
	thumbnailHandler := serve.NewThumbnailHandler(cfg)
	statusHandler := serve.NewStatusHandler(cfg, database)
	playInitHandler := serve.NewPlayInitHandler(cfg, database)
	adminHandler := admin.NewAdminHandler(cfg, database, queue)
	googleAuthHandler := google.NewGoogleHandler(cfg, database)
	spaHandler := spa.NewHandler(cfg.SPADir)
	rateLimiter := middleware.NewRateLimiter(cfg.RateLimitPerMin)

	// Provider de telemetria OpenTelemetry/Prometheus (T29, issue #1).
	telemetryProvider, err := telemetry.NewProvider()
	if err != nil {
		log.Printf("[telemetry] erro ao criar provider de métricas: %v — rota /metrics ficará indisponível", err)
	}
	if telemetryProvider != nil {
		if err := telemetryProvider.RegisterQueueGauge(queue.Len); err != nil {
			log.Printf("[telemetry] erro ao registrar gauge de fila: %v", err)
		}
		if err := telemetryProvider.RegisterUploadsInProgressGauge(database); err != nil {
			log.Printf("[telemetry] erro ao registrar gauge de uploads em andamento: %v", err)
		}
		if err := telemetryProvider.RegisterPlaybackEventsGauge(database); err != nil {
			log.Printf("[telemetry] erro ao registrar gauge de eventos de playback: %v", err)
		}
	}

	r := chi.NewRouter()

	// Middlewares globais aplicados a todas as rotas.
	r.Use(middleware.RecoveryMiddleware)
	r.Use(middleware.StripSlashMiddleware)
	r.Use(chimw.Logger)
	if telemetryProvider != nil {
		r.Use(telemetryProvider.Middleware)
	}
	r.Use(rateLimiter.Middleware)

	// --- Health check (público, sem rate limit forte) ---
	healthz := func(w http.ResponseWriter, _ *http.Request) {
		apiresponse.Success(w, http.StatusOK, map[string]string{"status": "ok"})
	}
	r.Get("/healthz", healthz)
	r.Head("/healthz", healthz)

	// --- Versão da API (pública, rate limit 10/min) ---
	versionLimiter := middleware.NewRateLimiter(10)
	r.Group(func(r chi.Router) {
		r.Use(versionLimiter.Middleware)
		r.Get("/api", func(w http.ResponseWriter, _ *http.Request) {
			apiresponse.Success(w, http.StatusOK, version.Get(cfg.Environment))
		})
	})

	// --- Autenticação Google OAuth (pública) ---
	r.Get("/api/auth/google", googleAuthHandler.HandleLogin)
	r.Get("/api/auth/google/callback", googleAuthHandler.HandleCallback)

	// --- Logout de sessão (pública e idempotente) ---
	r.Delete("/api/auth/session", admin.HandleSessionLogout(cfg))

	// --- Rotas protegidas: autenticadas (RootAuth) + qualquer role ---
	// Estas rotas aceitam Bearer ROOT_TOKEN OU cookie de sessão Google.
	// Qualquer usuário autenticado (manager+) pode acessar.
	r.Group(func(r chi.Router) {
		r.Use(admin.RootAuth(cfg.RootToken))

		// Dados do usuário logado
		r.Get("/api/auth/me", googleAuthHandler.HandleMe)

		// Inicialização de upload e emissão de URL de play
		r.Post("/api/upload/init", initHandler.ServeHTTP)
		r.Post("/api/play/init", playInitHandler.ServeHTTP)

		// Status do vídeo
		r.Get("/api/status/{videoID}", statusHandler.ServeHTTP)

		// Reprocessamento de vídeo
		r.Post("/api/videos/{videoID}/reprocess", adminHandler.HandleReprocessVideo)

		// Admin — leitura e operações básicas (qualquer role)
		r.Get("/admin/videos", adminHandler.HandleVideos)
		r.Get("/admin/queue", adminHandler.HandleQueue)
		r.Get("/admin/stats", adminHandler.HandleStats)
		r.Delete("/admin/videos/{videoID}", adminHandler.HandleDeleteVideo)
	})

	// --- Rotas protegidas: ACL+ (gerenciamento de usuários) ---
	r.Group(func(r chi.Router) {
		r.Use(admin.RootAuth(cfg.RootToken))
		r.Use(admin.RoleAuth(database, models.RoleDev, models.RoleAdmin, models.RoleACL))

		r.Get("/admin/users", adminHandler.HandleListUsers)
		r.Post("/admin/users", adminHandler.HandleCreateUser)
		r.Put("/admin/users/{userID}/roles", adminHandler.HandleUpdateUserRoles)
	})

	// --- Rotas protegidas: Admin+ (leitura de config) ---
	r.Group(func(r chi.Router) {
		r.Use(admin.RootAuth(cfg.RootToken))
		r.Use(admin.RoleAuth(database, models.RoleDev, models.RoleAdmin))

		r.Get("/admin/config", adminHandler.HandleGetConfig)
		r.Put("/admin/config/{key}", adminHandler.HandleUpdateConfig)
	})

	// --- Rotas protegidas: Dev only (delete de config) ---
	r.Group(func(r chi.Router) {
		r.Use(admin.RootAuth(cfg.RootToken))
		r.Use(admin.RoleAuth(database, models.RoleDev))

		r.Delete("/admin/config/{key}", adminHandler.HandleDeleteConfig)
	})

	// --- Rotas protegidas: Admin+ (delete de usuário) ---
	r.Group(func(r chi.Router) {
		r.Use(admin.RootAuth(cfg.RootToken))
		r.Use(admin.RoleAuth(database, models.RoleDev, models.RoleAdmin))

		r.Delete("/admin/users/{userID}", adminHandler.HandleDeleteUser)
	})

	// --- Stream de eventos (SSE) em /api/events ---
	sseAuth := func(token, videoID string) bool {
		t, err := models.GetAccessToken(database, token)
		return err == nil && t.Purpose == models.PurposeUpload && !t.IsExpired() && t.VideoID == videoID
	}
	sseHandler := sse.NewHandler(hub, sseAuth)
	r.Get("/api/events", sseHandler.ServeHTTP)

	// --- Upload TUS ---
	r.Post("/files/", tusHandler.ServeHTTP)
	r.Post("/files/{videoID}", tusHandler.ServeHTTP)
	r.Patch("/files/{videoID}", tusHandler.ServeHTTP)
	r.Head("/files/{videoID}", tusHandler.ServeHTTP)
	r.Delete("/files/{videoID}", tusHandler.ServeHTTP)

	// --- Serving HLS ---
	r.Get("/video/{tag}/{file}", masterHandler.ServeHTTP)
	r.Get("/video/{tag}/{videoID}/{thumb}", thumbnailHandler.ServeHTTP)
	r.Get("/video/{tag}/{videoID}/{res}/{segment}", staticHandler.ServeHTTP)

	// --- Métricas (ROOT_TOKEN apenas, sem cookie de sessão) ---
	// /metrics expõe dados operacionais internos — acesso somente via
	// Authorization: Bearer <ROOT_TOKEN> (sem suporte a cookie).
	if telemetryProvider != nil {
		r.Group(func(r chi.Router) {
			r.Use(admin.RootAuth(cfg.RootToken))
			r.Get("/metrics", telemetryProvider.Handler.ServeHTTP)
		})
	}

	// --- SPA (Single Page Application) em /app ---
	// Serve a interface Vue.js do admin unificado.
	// /app/assets/* → arquivos estáticos (JS, CSS, imagens)
	// /app → index.html (fallback SPA)
	// /app/* → fallback para index.html (SPA client-side routing)
	r.Get("/app/assets/*", spaHandler.ServeAssets)
	r.Get("/app", spaHandler.ServeIndex)
	r.Get("/app/*", spaHandler.ServeIndex)

	// --- Handler 404 customizado ---
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		apiresponse.Error(w, http.StatusNotFound,
			fmt.Sprintf("Rota não encontrada: %s %s", r.Method, r.URL.Path))
	})

	// --- Handler 405 customizado ---
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		apiresponse.Error(w, http.StatusMethodNotAllowed,
			fmt.Sprintf("Método não permitido: %s %s", r.Method, r.URL.Path))
	})

	// Closer que encerra recursos internos.
	closer := closerFunc(func() error {
		tusHandler.Stop()
		rateLimiter.Stop()
		versionLimiter.Stop()
		return nil
	})

	return r, closer, nil
}

// closerFunc adapta uma função para a interface io.Closer.
type closerFunc func() error

func (f closerFunc) Close() error { return f() }
