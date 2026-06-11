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
	"github.com/klawdyo/streamedia/internal/dashboard"
	"github.com/klawdyo/streamedia/internal/docs"
	"github.com/klawdyo/streamedia/internal/middleware"
	"github.com/klawdyo/streamedia/internal/models"
	"github.com/klawdyo/streamedia/internal/notify"
	"github.com/klawdyo/streamedia/internal/playground"
	"github.com/klawdyo/streamedia/internal/serve"
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

	// --- Gestão (protegida pelo ROOT_TOKEN via middleware RootAuth) ---
	// O backend principal usa o ROOT_TOKEN para iniciar uploads, emitir URLs
	// de play, consultar status, listar e apagar.
	r.Group(func(r chi.Router) {
		r.Use(admin.RootAuth(cfg.RootToken))

		// Inicialização de upload e emissão de URL de play.
		r.Post("/api/upload/init", initHandler.ServeHTTP)
		r.Post("/api/play/init", playInitHandler.ServeHTTP)

		// Status do vídeo (backend-to-backend).
		r.Get("/api/status/{videoID}", statusHandler.ServeHTTP)

		// Administração.
		r.Get("/admin/videos", adminHandler.HandleVideos)
		r.Get("/admin/queue", adminHandler.HandleQueue)
		r.Get("/admin/stats", adminHandler.HandleStats)
		r.Delete("/admin/videos/{videoID}", adminHandler.HandleDeleteVideo)

		// Login de sessão de navegador: troca o Bearer ROOT_TOKEN por um
		// cookie streamedia_session (ver internal/admin/session.go), que
		// passa a valer como autenticação alternativa em RootAuth — permite
		// navegar para /docs, /metrics e /dashboard/* sem reenviar o header
		// Authorization.
		r.Post("/admin/session", admin.HandleSessionLogin(cfg))
	})

	// Logout de sessão de navegador: apaga o cookie streamedia_session.
	// Pública e idempotente (não exige RootAuth) — encerrar uma sessão
	// inexistente ou já expirada não é um erro.
	r.Delete("/admin/session", admin.HandleSessionLogout(cfg))

	// --- Stream de eventos (SSE) em /api/events ---
	// Entrega ao vivo as notificações do pipeline (as mesmas do webhook),
	// escopadas por video_id e autenticadas pelo token de upload do vídeo na
	// query (EventSource não envia cabeçalhos). Fora do RootAuth: é uma
	// credencial de cliente, não o ROOT_TOKEN. Mesmos critérios do TUS: token
	// existente, purpose=upload, não expirado e pertencente ao vídeo.
	sseAuth := func(token, videoID string) bool {
		t, err := models.GetAccessToken(database, token)
		return err == nil && t.Purpose == models.PurposeUpload && !t.IsExpired() && t.VideoID == videoID
	}
	sseHandler := sse.NewHandler(hub, sseAuth)
	r.Get("/api/events", sseHandler.ServeHTTP)

	// --- Upload TUS ---
	// O handler TUS valida o token de upload efêmero (Upload-Token) por conta
	// própria; por isso não fica sob o RootAuth. O chi exige registro explícito
	// de método, então mapeamos cada verbo para o mesmo ServeHTTP.
	r.Post("/files/", tusHandler.ServeHTTP) // criação TUS sem video_id
	r.Post("/files/{videoID}", tusHandler.ServeHTTP)
	r.Patch("/files/{videoID}", tusHandler.ServeHTTP)
	r.Head("/files/{videoID}", tusHandler.ServeHTTP)
	r.Delete("/files/{videoID}", tusHandler.ServeHTTP)

	// --- Serving HLS ---
	// /video/<tag>/<id>.m3u8 é o master dinâmico (autenticado por token de play
	// na query). As playlists de resolução e segmentos são estáticos/públicos.
	// Os handlers fazem o parsing do path internamente (prefixo /video/).
	r.Get("/video/{tag}/{file}", masterHandler.ServeHTTP)
	// Thumbnail (poster) público por resolução: 3 segmentos após /video/,
	// distinto do master (2) e do segmento (4). Sem autenticação (issue #19).
	r.Get("/video/{tag}/{videoID}/{thumb}", thumbnailHandler.ServeHTTP)
	r.Get("/video/{tag}/{videoID}/{res}/{segment}", staticHandler.ServeHTTP)

	// --- Health check ---
	// Aceita GET e HEAD: o healthcheck do Docker e proxies/monitores podem
	// sondar com HEAD. Sem o HEAD registrado, o chi responderia 405 e o
	// healthcheck falharia, marcando o container como unhealthy (e fazendo
	// o proxy do Coolify deixar de rotear o tráfego para ele).
	healthz := func(w http.ResponseWriter, _ *http.Request) {
		apiresponse.Success(w, http.StatusOK, map[string]string{"status": "ok"})
	}
	r.Get("/healthz", healthz)
	r.Head("/healthz", healthz)

	// --- Versão da API (T55) ---
	// Rota pública sem autenticação, com rate limiting baixo (10 req/min)
	// para mitigar abuso. Expõe nome, versão semântica, ambiente e status.
	// A versão é injetada via -ldflags no build (internal/version); o ambiente
	// vem da config (variável ENV). O commit deixou de ser exposto aqui.
	versionLimiter := middleware.NewRateLimiter(10)
	r.Group(func(r chi.Router) {
		r.Use(versionLimiter.Middleware)
		r.Get("/api", func(w http.ResponseWriter, _ *http.Request) {
			apiresponse.Success(w, http.StatusOK, version.Get(cfg.Environment))
		})
	})

	// --- Playground da API em /playground (issue #18) ---
	// Página interativa que exercita o fluxo completo (auth → upload → play) e
	// acompanha os eventos ao vivo via SSE (/api/events). "playground" é o termo
	// usual para um testador interativo de API. Rota PÚBLICA (sem RootAuth): a
	// página só faz algo de útil com o ROOT_TOKEN colado manualmente pelo
	// usuário. O rate limiter global continua valendo.
	playgroundHandler := playground.NewHandler()
	r.Get("/playground", playgroundHandler.ServeUI)

	// --- Dashboard administrativo em /dashboard ---
	// Visão geral (estatísticas + gráficos), biblioteca de vídeos (lista com
	// paginação/filtros/ordenação) e página por vídeo (player + estatísticas).
	// Rotas PÚBLICAS (sem RootAuth), no MESMO padrão do /playground: as páginas
	// HTML não fazem nada de útil sem o ROOT_TOKEN — todo dado vem das rotas
	// protegidas (/admin/*, /api/status, /api/play/init), que continuam exigindo
	// o token. O usuário cola o ROOT_TOKEN uma vez (guardado no sessionStorage)
	// e o JS o envia em Authorization: Bearer; o mesmo JS também chama
	// POST /admin/session para estabelecer o cookie streamedia_session, que
	// libera a navegação normal (sem JS) para /docs e /metrics. O rate
	// limiter global continua valendo. StripSlashMiddleware (global)
	// normaliza /dashboard/ → /dashboard.
	dashboardHandler := dashboard.NewHandler()
	r.Get("/dashboard", dashboardHandler.ServeOverview)
	r.Get("/dashboard/videos", dashboardHandler.ServeVideos)
	r.Get("/dashboard/videos/{videoID}", dashboardHandler.ServeVideo)
	r.Get("/dashboard/assets/{file}", dashboardHandler.ServeAsset)

	// --- Observabilidade e documentação (protegidas pelo ROOT_TOKEN) ---
	// /metrics (OpenTelemetry/Prometheus) e /docs (Scalar UI) exigem a mesma
	// autenticação das rotas /admin/* (RootAuth: Bearer ROOT_TOKEN OU cookie
	// streamedia_session — ver internal/admin/admin.go e session.go):
	// /metrics expõe detalhes operacionais internos (tamanho da fila, uploads
	// em andamento, contadores de eventos) e /docs descreve toda a superfície
	// da API — informação valiosa para reconhecimento por scanners/bots.
	// Protegê-las reduz a superfície exposta sem remover as rotas; o scraper
	// Prometheus deve enviar `Authorization: Bearer <ROOT_TOKEN>`, enquanto a
	// navegação normal a partir do /dashboard usa o cookie de sessão.
	// StripSlashMiddleware (global) normaliza /docs/ → /docs, sem redirect.
	r.Group(func(r chi.Router) {
		r.Use(admin.RootAuth(cfg.RootToken))

		if telemetryProvider != nil {
			r.Get("/metrics", telemetryProvider.Handler.ServeHTTP)
		}

		docsHandler := docs.NewHandler()
		r.Get("/docs", docsHandler.ServeUI)
		r.Get("/docs/openapi.json", docsHandler.ServeOpenAPISpec)
	})

	// Handler 404 customizado — responde no envelope padrão da API em vez
	// do texto puro "404 page not found" padrão do chi. Inclui o método e o
	// caminho para tornar o erro mais explícito ao depurar (ex. acesso à raiz
	// "/" ou a uma rota inexistente).
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		apiresponse.Error(w, http.StatusNotFound,
			fmt.Sprintf("Rota não encontrada: %s %s", r.Method, r.URL.Path))
	})

	// Handler 405 customizado — quando o caminho existe mas o método não é
	// permitido (ex. HEAD em /healthz, que só aceita GET). Sem isto o chi
	// responde texto puro "405 method not allowed", quebrando o contrato JSON.
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		apiresponse.Error(w, http.StatusMethodNotAllowed,
			fmt.Sprintf("Método não permitido: %s %s", r.Method, r.URL.Path))
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
