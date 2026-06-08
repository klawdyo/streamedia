package transcode

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/models"
)

// RunStartupRecovery verifica o banco na inicialização e reenfileira vídeos
// que estavam em processamento quando o servidor crashou.
// Vídeos com status 'upload_complete' são reenfileirados.
// Vídeos com status 'transcoding' são reenfileirados se ainda houver tentativas,
// caso contrário marcados como falha.
func RunStartupRecovery(
	db *sql.DB,
	cfg *config.Config,
	enqueue func(videoID string) error,
	onWebhook func(videoID, event, errMsg string),
) error {
	// Query: busca vídeos em estado 'transcoding' ou 'upload_complete'
	rows, err := db.Query(
		`SELECT video_id, status, transcode_attempts FROM videos
		 WHERE status IN (?, ?)`,
		models.StatusTranscoding, models.StatusUploadComplete,
	)
	if err != nil {
		return fmt.Errorf("erro ao consultar vídeos para recuperação: %w", err)
	}

	// Coleta todos os vídeos antes de fazer qualquer escrita.
	type videoRecord struct {
		videoID           string
		status            string
		transcodeAttempts int
	}

	var videos []videoRecord
	for rows.Next() {
		var v videoRecord
		if err := rows.Scan(&v.videoID, &v.status, &v.transcodeAttempts); err != nil {
			rows.Close()
			return fmt.Errorf("erro ao ler linha de vídeo: %w", err)
		}
		videos = append(videos, v)
	}

	// Fecha as linhas antes de fazer qualquer escrita no banco.
	if err := rows.Err(); err != nil {
		return fmt.Errorf("erro ao iterar vídeos: %w", err)
	}
	rows.Close()

	var requeuedCount, failedCount int

	// Processa cada vídeo.
	for _, v := range videos {
		if v.status == string(models.StatusUploadComplete) {
			// Reenfileira diretamente.
			if err := enqueue(v.videoID); err != nil {
				log.Printf("recuperação: erro ao reenfileirar %s: %v", v.videoID, err)
				continue
			}
			requeuedCount++
		} else if v.status == string(models.StatusTranscoding) {
			// Verifica se ainda há tentativas disponíveis.
			if v.transcodeAttempts < cfg.MaxTranscodeAttempts {
				// Incrementa o contador de tentativas.
				if err := models.IncrementTranscodeAttempts(db, v.videoID); err != nil {
					log.Printf("recuperação: erro ao incrementar tentativas de %s: %v", v.videoID, err)
					continue
				}

				// Volta para 'upload_complete' para ser reenfileirado.
				if err := models.UpdateStatus(db, v.videoID, models.StatusUploadComplete); err != nil {
					log.Printf("recuperação: erro ao atualizar status de %s: %v", v.videoID, err)
					continue
				}

				// Reenfileira.
				if err := enqueue(v.videoID); err != nil {
					log.Printf("recuperação: erro ao reenfileirar %s: %v", v.videoID, err)
					continue
				}
				requeuedCount++
			} else {
				// Limite de tentativas atingido: marca como falha.
				errMsg := "Vídeo marcado como falha na recuperação de inicialização: limite de tentativas atingido."
				if err := models.UpdateStatusWithError(db, v.videoID, models.StatusFailedTranscode, errMsg); err != nil {
					log.Printf("recuperação: erro ao marcar %s como falha: %v", v.videoID, err)
					continue
				}
				onWebhook(v.videoID, "failed", errMsg)
				failedCount++
			}
		}
	}

	log.Printf("Recuperação de inicialização: %d vídeos reenfileirados, %d marcados como falha.", requeuedCount, failedCount)
	return nil
}
