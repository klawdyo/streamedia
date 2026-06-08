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
	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/docs"
	"github.com/klawdyo/streamedia/internal/middleware"
	"github.com/klawdyo/streamedia/internal/models"
	"github.com/klawdyo/streamedia/internal/serve"
	"github.com/klawdyo/streamedia/internal/telemetry"
	"github.com/klawdyo/streamedia/internal/transcode"
	"github.com/klawdyo/streamedia/internal/upload"
	"github.com/klawdyo/streamedia/internal/version"
	"github.com/klawdyo/streamedia/internal/webhook"
)

// NewRouter monta e devolve o roteador chi completo da aplicação e um
// io.Closer que deve ser chamado no shutdown para liberar recursos internos
// (goroutines do TUS handler, etc.). O chamador é responsável por chamar
// Close() antes de encerrar o servidor (T59).
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
) (http.Handler, io.Closer, error) {
	// sendWebhook adapta o client de webhook para a assinatura usada pelos
	// callbacks (videoID, event, errMsg). Busca o vídeo no banco para enviar
	// o payload completo; se o vídeo não for encontrado, loga e aborta (T56:
	// evita nil pointer dereference em buildPayload).
	sendWebhook := func(videoID, event, errMsg string) {
		video, err := models.GetVideo(database, videoID)
		if err != nil {
			log.Printf("[webhook] erro ao buscar vídeo %s para webhook %s: %v", videoID, event, err)
			return
		}
		if err := wc.Send(videoID, event, video); err != nil {
			log.Printf("[webhook] erro ao enviar webhook %s para vídeo %s: %v", event, videoID, err)
		}
	}

	// onFinish é chamado pelo handler TUS quando o upload termina: valida o
	// arquivo, enfileira a transcodificação e dispara webhooks.
	onFinish := func(videoID, userAgent string) {
		filePath := filepath.Join(cfg.UploadTmpDir, videoID)
		upload.HandlePostFinish(database, cfg, queue.Enqueue, sendWebhook, videoID, filePath, userAgent)
	}

	// Constrói todos os handlers da aplicação.
	initHandler := upload.NewInitHandler(cfg, database)
	tusHandler, err := upload.NewTUSHandler(cfg, database, onFinish)
	if err != nil {
		return nil, nil, fmt.Errorf("falha ao criar handler TUS: %w", err)
	}
	masterHandler := serve.NewMasterHandler(cfg, database)
	staticHandler := serve.NewStaticHandler(cfg, database)
	statusHandler := serve.NewStatusHandler(cfg, database)
	adminHandler := admin.NewAdminHandler(cfg, database, queue)
	rateLimiter := middleware.NewRateLimiter(cfg.RateLimitPerMin)

	// Provider de telemetria OpenTelemetry/Prometheus (T29, issue #1).
	// Falha ao montar o provider não deve impedir o servidor de subir —
	// observabilidade é um recurso auxiliar, não um requisito de operação.
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
	// RecoveryMiddleware recupera de panics e responde no envelope padrão
	// da API ({error, message, data, status_code}), substituindo o
	// chimw.Recoverer que respondia com texto puro (quebrava o contrato JSON).
	r.Use(middleware.RecoveryMiddleware)
	// StripSlashMiddleware remove a barra final de todas as requisições
	// antes do roteamento — sem redirect, sem declarar rota 2x.
	r.Use(middleware.StripSlashMiddleware)
	r.Use(chimw.Logger) // loga cada requisição
	if telemetryProvider != nil {
		r.Use(telemetryProvider.Middleware) // instrumenta requisições HTTP (T29)
	}
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
		r.Use(admin.AdminAuth(cfg.AdminToken, database))
		r.Get("/admin/videos", adminHandler.HandleVideos)
		r.Get("/admin/queue", adminHandler.HandleQueue)
		r.Get("/admin/stats", adminHandler.HandleStats)

		// Gerenciamento de projetos (T35, issue #6) — operação de
		// super-admin (ver admin.requireSuperAdmin: rejeita autenticação
		// por chave mestra de projeto, exige o ADMIN_TOKEN global).
		r.Post("/admin/projects", adminHandler.HandleCreateProject)
		r.Get("/admin/projects", adminHandler.HandleListProjects)
		r.Get("/admin/projects/{slug}", adminHandler.HandleGetProject)
	})

	// Emissão de token de upload escopado a um projeto (T35, issue #6):
	// autenticada pela própria chave mestra do projeto via X-Project-Key
	// (mesmo princípio de POST /upload/init), e não pelo AdminAuth — por
	// isso fica fora do grupo acima. Ver admin.HandleIssueUploadToken para
	// a justificativa completa dessa decisão de modelo de autenticação.
	r.Post("/admin/projects/{slug}/upload-tokens", adminHandler.HandleIssueUploadToken)

	// --- Health check ---
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		apiresponse.Success(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// --- Versão da API (T55) ---
	// Rota pública sem autenticação, com rate limiting baixo (10 req/min)
	// para mitigar abuso. Expõe nome, versão semântica, commit e status.
	// A versão é injetada via -ldflags no build (internal/version).
	versionLimiter := middleware.NewRateLimiter(10)
	r.Group(func(r chi.Router) {
		r.Use(versionLimiter.Middleware)
		r.Get("/api", func(w http.ResponseWriter, _ *http.Request) {
			apiresponse.Success(w, http.StatusOK, version.Get())
		})
	})

	// --- Métricas (OpenTelemetry/Prometheus, T29, issue #1) ---
	// Sem autenticação: é o padrão do ecossistema Prometheus — a proteção,
	// quando necessária, é feita na camada de infraestrutura/rede (ex.
	// regra de firewall restringindo a origem do scraper), não na aplicação.
	// O rate limiter (T19), já aplicado globalmente acima, mitiga abuso.
	if telemetryProvider != nil {
		r.Get("/metrics", telemetryProvider.Handler.ServeHTTP)
	}

	// --- Documentação da API (Scalar UI, T51, issue #12) ---
	// Sem autenticação — ver decisão registrada em internal/docs/docs.go.
	// StripSlashMiddleware (global) normaliza /docs/ → /docs, sem redirect.
	docsHandler := docs.NewHandler()
	r.Get("/docs", docsHandler.ServeUI)
	r.Get("/docs/openapi.json", docsHandler.ServeOpenAPISpec)

	// Handler 404 customizado — responde no envelope padrão da API em vez
	// do texto puro "404 page not found" padrão do chi.
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		apiresponse.Error(w, http.StatusNotFound, "Rota não encontrada.")
	})

	// Closer que encerra recursos internos (goroutines do TUS handler e rate limiters).
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
