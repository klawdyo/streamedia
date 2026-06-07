package jobs

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/models"
)

// killerTickInterval define a frequência com que o job varre o banco em
// busca de uploads inativos.
const killerTickInterval = 2 * time.Minute

// UploadKillerJob é o job periódico que encerra uploads que ficaram
// inativos (sem receber chunks) por mais tempo que o timeout configurado.
type UploadKillerJob struct {
	cfg       *config.Config
	db        *sql.DB
	onWebhook func(videoID, event, errMsg string)
	ticker    *time.Ticker
	stopCh    chan struct{}
}

// NewUploadKillerJob cria uma nova instância do job de killer de uploads.
// onWebhook é chamado para cada upload encerrado, permitindo notificar
// o sistema externo via webhook.
func NewUploadKillerJob(cfg *config.Config, db *sql.DB, onWebhook func(videoID, event, errMsg string)) *UploadKillerJob {
	return &UploadKillerJob{
		cfg:       cfg,
		db:        db,
		onWebhook: onWebhook,
		ticker:    time.NewTicker(killerTickInterval),
		stopCh:    make(chan struct{}),
	}
}

// Start inicia a goroutine que executa o job a cada intervalo do ticker.
func (j *UploadKillerJob) Start() {
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
func (j *UploadKillerJob) Stop() {
	close(j.stopCh)
}

// runOnce executa uma única varredura: encontra uploads inativos, remove
// seus arquivos temporários, marca como falha e dispara o webhook.
// É a unidade testável da lógica do job.
func (j *UploadKillerJob) runOnce() error {
	// Calcula o timeout em minutos a partir da duração configurada.
	timeoutMin := int(j.cfg.UploadIdleTimeout.Minutes())

	// Mensagem de erro gravada e enviada no webhook.
	errMsg := fmt.Sprintf(
		"Upload encerrado por inatividade: nenhum chunk recebido nos últimos %d minutos.",
		timeoutMin,
	)

	// Seleciona vídeos em estados de upload que ficaram inativos.
	// Usa last_chunk_at quando disponível; caso contrário, cai para created_at.
	cutoff := fmt.Sprintf("-%d minutes", timeoutMin)
	rows, err := j.db.Query(
		`SELECT video_id
		   FROM videos
		  WHERE status IN ('pending_upload', 'uploading')
		    AND (
		          (last_chunk_at IS NOT NULL AND datetime(last_chunk_at) < datetime('now', ?))
		          OR
		          (last_chunk_at IS NULL AND datetime(created_at) < datetime('now', ?))
		        )`,
		cutoff, cutoff,
	)
	if err != nil {
		return fmt.Errorf("erro ao consultar uploads inativos: %w", err)
	}

	// Coleta os IDs antes de processar, para liberar o cursor de leitura
	// e evitar conflitos com as escritas (UPDATE) abaixo.
	var videoIDs []string
	for rows.Next() {
		var videoID string
		if err := rows.Scan(&videoID); err != nil {
			_ = rows.Close()
			return fmt.Errorf("erro ao ler video_id: %w", err)
		}
		videoIDs = append(videoIDs, videoID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("erro ao iterar uploads inativos: %w", err)
	}
	_ = rows.Close()

	// Processa cada upload inativo encontrado.
	for _, videoID := range videoIDs {
		// Remove o arquivo temporário do upload parcial (ignora erro:
		// pode não existir se nenhum chunk chegou a ser gravado).
		_ = os.Remove(filepath.Join(j.cfg.UploadTmpDir, videoID))
		// Remove o arquivo de metadados do upload (ignora erro).
		_ = os.Remove(filepath.Join(j.cfg.UploadTmpDir, videoID+".info"))

		// Marca o vídeo como falha de upload, gravando a mensagem de erro.
		if err := models.UpdateStatusWithError(j.db, videoID, models.StatusFailedUpload, errMsg); err != nil {
			// Não interrompe os demais; segue para o próximo.
			continue
		}

		// Notifica o sistema externo via webhook.
		if j.onWebhook != nil {
			j.onWebhook(videoID, "failed", errMsg)
		}
	}

	return nil
}
