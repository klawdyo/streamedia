// Comando server: ponto de entrada do servidor de mídia Streamedia.
// Inicializa config, banco, jobs de background, a fila de transcodificação e
// o servidor HTTP, com shutdown gracioso em SIGINT/SIGTERM.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/db"
	"github.com/klawdyo/streamedia/internal/discord"
	"github.com/klawdyo/streamedia/internal/jobs"
	"github.com/klawdyo/streamedia/internal/notify"
	"github.com/klawdyo/streamedia/internal/server"
	"github.com/klawdyo/streamedia/internal/sse"
	"github.com/klawdyo/streamedia/internal/transcode"
	"github.com/klawdyo/streamedia/internal/webhook"
)

func main() {
	// --- Fase 1: Carregar configuração das variáveis de ambiente ---
	log.Println("[init] Carregando configuração...")
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("[FATAL] config: %v", err)
	}
	log.Printf("[init] ROOT_TOKEN: configurado=%v, ENV=%s, PORT=%d, SQLITE_PATH=%s",
		cfg.RootToken != "", cfg.Environment, cfg.Port, cfg.SQLitePath)

	// --- Fase 2: Garantir diretórios de runtime ---
	log.Println("[init] Garantindo diretórios de runtime...")
	if err := ensureRuntimeDirs(cfg); err != nil {
		log.Fatalf("[FATAL] diretórios: %v", err)
	}

	// --- Fase 3: Abrir banco de dados ---
	log.Println("[init] Abrindo banco de dados...")
	database, err := db.Open(cfg.SQLitePath)
	if err != nil {
		log.Fatalf("[FATAL] db: %v", err)
	}
	defer database.Close()

	// --- Fase 4: Carregar configurações operacionais do banco ---
	log.Println("[init] Carregando configurações do banco...")
	cfg.ApplyFromDB(database)
	log.Printf("[init] %d valores carregados da tabela configurations", 13)

	// Camada de notificações: cada evento do pipeline é distribuído para os
	// destinos (sinks) registrados — o cliente de webhook (se houver URL) e o
	// hub de SSE (se houver ouvinte em /api/events). O notifier.Notify substitui
	// o antigo sendWebhook e tem a mesma assinatura (videoID, event, errMsg).
	log.Println("[init] Criando camada de notificações...")
	webhookClient := webhook.NewClient(cfg, database)
	sseHub := sse.NewHub()
	notifier := notify.New(database, webhookClient, sseHub)
	sendWebhook := notifier.Notify

	// Alerter operacional do Discord (issue #21): canal de alerta interno para
	// falhas que comprometem o serviço (transcode falho/travado, fila cheia,
	// falhas consecutivas). Opcional — NewAlerter devolve nil sem
	// DISCORD_WEBHOOK_URL, e os métodos são no-op em receptor nil.
	alerter := discord.NewAlerter(cfg.DiscordWebhookURL)

	// Worker e fila de transcodificação. A fila é criada com a função do
	// worker e iniciada aqui; o roteador a recebe pronta.
	log.Println("[init] Iniciando worker e fila de transcodificação...")
	worker := transcode.NewWorker(cfg, database, sendWebhook)
	worker.SetAlerter(alerter)
	queue := transcode.NewQueue(cfg, database, worker.Transcode)
	queue.SetAlerter(alerter)

	// Recupera vídeos em estado inconsistente após crash.
	log.Println("[init] Executando recuperação de inicialização...")
	if err := transcode.RunStartupRecovery(database, cfg, queue.Enqueue, sendWebhook); err != nil {
		log.Printf("[init] recovery: %v", err)
	}

	queue.Start()
	defer queue.Stop()

	// Job que mata uploads ociosos (idle timeout).
	log.Println("[init] Iniciando jobs de background...")
	killerJob := jobs.NewUploadKillerJob(cfg, database, sendWebhook)
	killerJob.Start()
	defer killerJob.Stop()

	// Job que reenfileira transcodificações travadas.
	requeueJob := jobs.NewTranscodeRequeueJob(cfg, database, queue.Enqueue, sendWebhook)
	requeueJob.SetAlerter(alerter)
	requeueJob.Start()
	defer requeueJob.Stop()

	// Job que limpa tokens expirados do banco.
	cleanupJob := jobs.NewTokenCleanupJob(database)
	cleanupJob.Start()
	defer cleanupJob.Stop()

	// Monta o roteador HTTP com todas as rotas e handlers.
	log.Println("[init] Montando roteador HTTP...")
	router, routerCloser, err := server.NewRouter(cfg, database, queue, notifier, sseHub)
	if err != nil {
		log.Fatalf("[FATAL] router: %v", err)
	}
	defer routerCloser.Close()

	srv := &http.Server{
		Addr:    ":" + strconv.Itoa(cfg.Port),
		Handler: router,
		// Timeouts de rede: protegem contra Slowloris (ReadTimeout), clientes
		// lentos (WriteTimeout) e conexões ociosas (IdleTimeout). Sem esses
		// timeouts, um atacante pode abrir conexões e nunca enviar headers,
		// esgotando o pool de goroutines do servidor.
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 60 * time.Second, // generoso para servir segmentos HLS longos
		IdleTimeout:  120 * time.Second,
		// Limita o tamanho dos headers para prevenir ataques de header grande.
		MaxHeaderBytes: 1 << 20, // 1MB
	}

	// Contexto cancelado em SIGINT/SIGTERM para shutdown gracioso.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Sobe o servidor em uma goroutine para não bloquear a espera do sinal.
	go func() {
		log.Printf("[init] Servidor iniciado na porta %d", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[FATAL] servidor: %v", err)
		}
	}()

	// Aguarda o sinal de término.
	<-ctx.Done()
	log.Println("[shutdown] Encerrando servidor...")

	// Concede até 5s para finalizar requisições em andamento.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)
}

// ensureRuntimeDirs cria, se necessário, todos os diretórios que a aplicação
// precisa para persistir dados em disco. É idempotente (MkdirAll não falha se
// o diretório já existir) e roda a cada inicialização — assim, se o volume
// Docker for recriado/apagado, os diretórios são restaurados automaticamente.
//
// Diretórios garantidos:
//   - diretório do banco SQLite (pai de SQLitePath)
//   - MediaDir: onde ficam os HLS transcodificados servidos ao público
//   - UploadTmpDir: onde o tusd grava os uploads em andamento (.uploads)
func ensureRuntimeDirs(cfg *config.Config) error {
	dirs := []string{
		filepath.Dir(cfg.SQLitePath),
		cfg.MediaDir,
		cfg.UploadTmpDir,
	}
	for _, dir := range dirs {
		// Caminhos especiais (ex.: ":memory:" para o banco) não são diretórios.
		if dir == "" || dir == "." || dir == ":memory:" {
			continue
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
		log.Printf("[init] diretório garantido: %s", dir)
	}
	return nil
}
