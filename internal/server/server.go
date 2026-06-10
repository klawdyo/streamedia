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
	})

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

	// --- Observabilidade e documentação (protegidas pelo ROOT_TOKEN) ---
	// /metrics (OpenTelemetry/Prometheus) e /docs (Scalar UI) exigem o mesmo
	// ROOT_TOKEN das rotas /admin/*: /metrics expõe detalhes operacionais
	// internos (tamanho da fila, uploads em andamento, contadores de eventos) e
	// /docs descreve toda a superfície da API — informação valiosa para
	// reconhecimento por scanners/bots. Protegê-las reduz a superfície exposta
	// sem remover as rotas; o scraper Prometheus deve enviar
	// `Authorization: Bearer <ROOT_TOKEN>`. StripSlashMiddleware (global)
	// normaliza /docs/ → /docs, sem redirect.
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
