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
	// Carrega a configuração a partir das variáveis de ambiente.
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// Garante que os diretórios persistidos existam ANTES de abrir o banco
	// e de aceitar uploads. Em Docker, o `mkdir` do Dockerfile roda em build
	// time e é sobrescrito quando um volume é montado em runtime — se o volume
	// estiver vazio (ou tiver sido apagado), os diretórios precisam ser
	// recriados aqui. db.Open já cuida do diretório do SQLite, mas o diretório
	// de mídia e o de uploads temporários (usado pelo tusd) também precisam
	// existir, senão o upload falha ao gravar em disco.
	if err := ensureRuntimeDirs(cfg); err != nil {
		log.Fatalf("diretórios: %v", err)
	}

	// Abre o banco SQLite (aplica migrations internamente).
	database, err := db.Open(cfg.SQLitePath)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer database.Close()

	// Camada de notificações: cada evento do pipeline é distribuído para os
	// destinos (sinks) registrados — o cliente de webhook (se houver URL) e o
	// hub de SSE (se houver ouvinte em /api/events). O notifier.Notify substitui
	// o antigo sendWebhook e tem a mesma assinatura (videoID, event, errMsg).
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
	worker := transcode.NewWorker(cfg, database, sendWebhook)
	worker.SetAlerter(alerter)
	queue := transcode.NewQueue(cfg, database, worker.Transcode)
	queue.SetAlerter(alerter)

	// Recupera vídeos em estado inconsistente após crash.
	if err := transcode.RunStartupRecovery(database, cfg, queue.Enqueue, sendWebhook); err != nil {
		log.Printf("recovery: %v", err)
	}

	queue.Start()
	defer queue.Stop()

	// Job que mata uploads ociosos (idle timeout).
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
	// O closer encerra recursos internos (goroutines do TUS handler, etc.)
	// e deve ser chamado no shutdown (T59).
	router, routerCloser, err := server.NewRouter(cfg, database, queue, notifier, sseHub)
	if err != nil {
		log.Fatalf("router: %v", err)
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
		log.Printf("diretório persistido garantido: %s", dir)
	}
	return nil
}
