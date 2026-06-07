package jobs

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/models"
)

// requeueTickInterval define a frequência com que o job varre o banco em
// busca de transcodificações travadas.
const requeueTickInterval = 2 * time.Minute

// TranscodeRequeueJob é o job periódico que reenfileira transcodificações
// que ficaram travadas (sem progredir) por mais tempo que o timeout configurado.
type TranscodeRequeueJob struct {
	cfg       *config.Config
	db        *sql.DB
	enqueue   func(videoID string) error
	onWebhook func(videoID, event, errMsg string)
	ticker    *time.Ticker
	stopCh    chan struct{}
}

// NewTranscodeRequeueJob cria uma nova instância do job de reenfileiramento
// de transcodificações. enqueue é chamado para reinserir o vídeo na fila,
// e onWebhook é chamado quando a transcodificação falha após atingir o
// número máximo de tentativas.
func NewTranscodeRequeueJob(
	cfg *config.Config,
	db *sql.DB,
	enqueue func(videoID string) error,
	onWebhook func(videoID, event, errMsg string),
) *TranscodeRequeueJob {
	return &TranscodeRequeueJob{
		cfg:       cfg,
		db:        db,
		enqueue:   enqueue,
		onWebhook: onWebhook,
		ticker:    time.NewTicker(requeueTickInterval),
		stopCh:    make(chan struct{}),
	}
}

// Start inicia a goroutine que executa o job a cada intervalo do ticker.
func (j *TranscodeRequeueJob) Start() {
	go func() {
		for {
			select {
			case <-j.ticker.C:
				// Ignora o erro: o próximo tick tentará novamente.
				_ = j.runOnce()
			case <-j.stopCh:
				j.ticker.Stop()
				return
			}
		}
	}()
}

// Stop encerra a goroutine do job.
func (j *TranscodeRequeueJob) Stop() {
	close(j.stopCh)
}

// runOnce executa uma única varredura: encontra transcodificações travadas,
// verifica o número de tentativas, reenfileira ou marca como falha.
// É a unidade testável da lógica do job.
func (j *TranscodeRequeueJob) runOnce() error {
	// Calcula o timeout em minutos a partir da duração configurada.
	timeoutMin := int(j.cfg.TranscodeStuckTime.Minutes())

	// Seleciona vídeos em transcodificação que ficaram travados.
	rows, err := j.db.Query(
		`SELECT video_id, transcode_attempts
		   FROM videos
		  WHERE status = ?
		    AND datetime(updated_at) < datetime('now', ?)`,
		models.StatusTranscoding,
		fmt.Sprintf("-%d minutes", timeoutMin),
	)
	if err != nil {
		return fmt.Errorf("erro ao consultar transcodificações travadas: %w", err)
	}

	// Coleta os registros antes de processar, para liberar o cursor de leitura
	// e evitar conflitos com as escritas (UPDATE) abaixo.
	type stuckTranscode struct {
		videoID string
		attempts int
	}
	var transcodes []stuckTranscode
	for rows.Next() {
		var videoID string
		var attempts int
		if err := rows.Scan(&videoID, &attempts); err != nil {
			_ = rows.Close()
			return fmt.Errorf("erro ao ler video_id e transcode_attempts: %w", err)
		}
		transcodes = append(transcodes, stuckTranscode{videoID, attempts})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("erro ao iterar transcodificações travadas: %w", err)
	}
	_ = rows.Close()

	// Processa cada transcodificação travada encontrada.
	for _, tc := range transcodes {
		if tc.attempts < j.cfg.MaxTranscodeAttempts {
			// Ainda há tentativas: reenfileira.

			// Incrementa o contador de tentativas.
			if err := models.IncrementTranscodeAttempts(j.db, tc.videoID); err != nil {
				// Não interrompe os demais; segue para o próximo.
				continue
			}

			// Volta para a fila de transcodificação (upload_complete).
			if err := models.UpdateStatus(j.db, tc.videoID, models.StatusUploadComplete); err != nil {
				// Não interrompe os demais; segue para o próximo.
				continue
			}

			// Chama a função de enfileiramento para reinseri-lo na fila de transcodificação.
			//
			// Invariante: o status só pode permanecer em upload_complete se o
			// vídeo realmente entrou na fila de processamento. Se o enqueue
			// falhar (ex.: fila cheia), precisamos desfazer a mudança de status
			// acima — caso contrário o vídeo ficaria "preso" em upload_complete
			// sem nunca ter sido enfileirado de fato, parecendo pendente na fila
			// lógica mas invisível para o worker. Por isso o UpdateStatus vem
			// antes do enqueue (algumas implementações de enqueue esperam o vídeo
			// já no estado correto), mas com rollback explícito em caso de falha.
			if err := j.enqueue(tc.videoID); err != nil {
				// Rollback: volta o vídeo ao estado anterior (transcoding) para
				// não deixá-lo em estado inconsistente. O rollback é best-effort;
				// se ele próprio falhar, apenas logamos e seguimos.
				if rbErr := models.UpdateStatus(j.db, tc.videoID, models.StatusTranscoding); rbErr != nil {
					log.Printf("[requeue] %s: falha ao reverter status após erro no enqueue: %v", tc.videoID, rbErr)
				}
				// Não interrompe os demais; segue para o próximo.
				continue
			}
		} else {
			// Atingiu o limite de tentativas: marca como falha.

			// Mensagem de erro gravada e enviada no webhook.
			errMsg := fmt.Sprintf(
				"Transcodificação falhou após %d tentativas. O vídeo não pôde ser processado.",
				tc.attempts,
			)

			// Marca como falha de transcodificação.
			if err := models.UpdateStatusWithError(j.db, tc.videoID, models.StatusFailedTranscode, errMsg); err != nil {
				// Não interrompe os demais; segue para o próximo.
				continue
			}

			// Notifica o sistema externo via webhook.
			if j.onWebhook != nil {
				j.onWebhook(tc.videoID, "failed", errMsg)
			}
		}
	}

	return nil
}
