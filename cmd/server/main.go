// Comando server: ponto de entrada do servidor de mídia Streamedia.
// Inicializa config, banco, jobs de background, a fila de transcodificação e
// o servidor HTTP, com shutdown gracioso em SIGINT/SIGTERM.
package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/db"
	"github.com/klawdyo/streamedia/internal/jobs"
	"github.com/klawdyo/streamedia/internal/models"
	"github.com/klawdyo/streamedia/internal/server"
	"github.com/klawdyo/streamedia/internal/transcode"
	"github.com/klawdyo/streamedia/internal/webhook"
)

func main() {
	// Carrega a configuração a partir das variáveis de ambiente.
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// Abre o banco SQLite (aplica migrations internamente).
	database, err := db.Open(cfg.SQLitePath)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer database.Close()

	// Client de webhook e adaptador para os callbacks (videoID, event, errMsg).
	webhookClient := webhook.NewClient(cfg, database)
	sendWebhook := func(videoID, event, errMsg string) {
		video, _ := models.GetVideo(database, videoID)
		_ = webhookClient.Send(videoID, event, video)
	}

	// Worker e fila de transcodificação. A fila é criada com a função do
	// worker e iniciada aqui; o roteador a recebe pronta.
	worker := transcode.NewWorker(cfg, database, sendWebhook)
	queue := transcode.NewQueue(cfg, database, worker.Transcode)
	queue.Start()
	defer queue.Stop()

	// Job que mata uploads ociosos (idle timeout).
	killerJob := jobs.NewUploadKillerJob(cfg, database, sendWebhook)
	killerJob.Start()
	defer killerJob.Stop()

	// Job que reenfileira transcodificações travadas.
	requeueJob := jobs.NewTranscodeRequeueJob(cfg, database, queue.Enqueue, sendWebhook)
	requeueJob.Start()
	defer requeueJob.Stop()

	// Job que limpa tokens expirados do banco.
	cleanupJob := jobs.NewTokenCleanupJob(database)
	cleanupJob.Start()
	defer cleanupJob.Stop()

	// Monta o roteador HTTP com todas as rotas e handlers.
	router := server.NewRouter(cfg, database, queue, webhookClient)

	srv := &http.Server{
		Addr:    ":" + strconv.Itoa(cfg.Port),
		Handler: router,
	}

	// Contexto cancelado em SIGINT/SIGTERM para shutdown gracioso.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Sobe o servidor em uma goroutine para não bloquear a espera do sinal.
	go func() {
		log.Printf("Servidor iniciado na porta %d", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("servidor: %v", err)
		}
	}()

	// Aguarda o sinal de término.
	<-ctx.Done()
	log.Println("Encerrando servidor...")

	// Concede até 5s para finalizar requisições em andamento.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)
}
