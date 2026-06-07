package transcode

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/db"
	"github.com/klawdyo/streamedia/internal/models"
)

// TestRecovery_RequeuesTranscoding verifica que um vídeo em estado
// 'transcoding' com tentativas disponíveis é reenfileirado.
func TestRecovery_RequeuesTranscoding(t *testing.T) {
	database := openTestDB(t)
	defer database.Close()

	videoID := "video-1"
	insertTestVideo(t, database, videoID, "transcoding", 0)

	cfg := &config.Config{MaxTranscodeAttempts: 3}

	var enqueuedVideos []string
	enqueue := func(id string) error {
		enqueuedVideos = append(enqueuedVideos, id)
		return nil
	}

	onWebhook := func(videoID, event, errMsg string) {}

	err := RunStartupRecovery(database, cfg, enqueue, onWebhook)
	if err != nil {
		t.Fatalf("RunStartupRecovery: %v", err)
	}

	// Verifica que o vídeo foi reenfileirado.
	if len(enqueuedVideos) != 1 || enqueuedVideos[0] != videoID {
		t.Errorf("esperava enqueue para %s, obteve %v", videoID, enqueuedVideos)
	}

	// Verifica que o status foi atualizado para 'upload_complete'.
	video, err := models.GetVideo(database, videoID)
	if err != nil {
		t.Fatalf("GetVideo: %v", err)
	}
	if video.Status != models.StatusUploadComplete {
		t.Errorf("esperava status %s, obteve %s", models.StatusUploadComplete, video.Status)
	}

	// Verifica que o contador de tentativas foi incrementado.
	if video.TranscodeAttempts != 1 {
		t.Errorf("esperava transcode_attempts=1, obteve %d", video.TranscodeAttempts)
	}
}

// TestRecovery_RequeuesUploadComplete verifica que um vídeo em estado
// 'upload_complete' é reenfileirado.
func TestRecovery_RequeuesUploadComplete(t *testing.T) {
	database := openTestDB(t)
	defer database.Close()

	videoID := "video-2"
	insertTestVideo(t, database, videoID, "upload_complete", 0)

	cfg := &config.Config{MaxTranscodeAttempts: 3}

	var enqueuedVideos []string
	enqueue := func(id string) error {
		enqueuedVideos = append(enqueuedVideos, id)
		return nil
	}

	onWebhook := func(videoID, event, errMsg string) {}

	err := RunStartupRecovery(database, cfg, enqueue, onWebhook)
	if err != nil {
		t.Fatalf("RunStartupRecovery: %v", err)
	}

	// Verifica que o vídeo foi reenfileirado.
	if len(enqueuedVideos) != 1 || enqueuedVideos[0] != videoID {
		t.Errorf("esperava enqueue para %s, obteve %v", videoID, enqueuedVideos)
	}
}

// TestRecovery_FailsTranscodingAtMaxAttempts verifica que um vídeo em estado
// 'transcoding' com limite de tentativas atingido é marcado como falha.
func TestRecovery_FailsTranscodingAtMaxAttempts(t *testing.T) {
	database := openTestDB(t)
	defer database.Close()

	videoID := "video-3"
	insertTestVideo(t, database, videoID, "transcoding", 3)

	cfg := &config.Config{MaxTranscodeAttempts: 3}

	var enqueuedVideos []string
	enqueue := func(id string) error {
		enqueuedVideos = append(enqueuedVideos, id)
		return nil
	}

	var webhookCalls []struct {
		videoID string
		event   string
		errMsg  string
	}
	onWebhook := func(videoID, event, errMsg string) {
		webhookCalls = append(webhookCalls, struct {
			videoID string
			event   string
			errMsg  string
		}{videoID, event, errMsg})
	}

	err := RunStartupRecovery(database, cfg, enqueue, onWebhook)
	if err != nil {
		t.Fatalf("RunStartupRecovery: %v", err)
	}

	// Verifica que enqueue NÃO foi chamado.
	if len(enqueuedVideos) != 0 {
		t.Errorf("esperava nenhuma enqueue, obteve %v", enqueuedVideos)
	}

	// Verifica que o status foi alterado para 'failed_transcode'.
	video, err := models.GetVideo(database, videoID)
	if err != nil {
		t.Fatalf("GetVideo: %v", err)
	}
	if video.Status != models.StatusFailedTranscode {
		t.Errorf("esperava status %s, obteve %s", models.StatusFailedTranscode, video.Status)
	}

	// Verifica que o webhook foi chamado com o evento 'failed'.
	if len(webhookCalls) != 1 {
		t.Errorf("esperava 1 webhook call, obteve %d", len(webhookCalls))
	} else if webhookCalls[0].event != "failed" {
		t.Errorf("esperava event='failed', obteve '%s'", webhookCalls[0].event)
	}
}

// TestRecovery_MultipleVideos verifica a recuperação com múltiplos vídeos.
func TestRecovery_MultipleVideos(t *testing.T) {
	database := openTestDB(t)
	defer database.Close()

	// Insere 2 vídeos em estado 'transcoding' com 0 tentativas
	insertTestVideo(t, database, "video-4a", "transcoding", 0)
	insertTestVideo(t, database, "video-4b", "transcoding", 0)
	// Insere 1 vídeo em estado 'upload_complete'
	insertTestVideo(t, database, "video-4c", "upload_complete", 0)

	cfg := &config.Config{MaxTranscodeAttempts: 3}

	var enqueuedVideos []string
	enqueue := func(id string) error {
		enqueuedVideos = append(enqueuedVideos, id)
		return nil
	}

	onWebhook := func(videoID, event, errMsg string) {}

	err := RunStartupRecovery(database, cfg, enqueue, onWebhook)
	if err != nil {
		t.Fatalf("RunStartupRecovery: %v", err)
	}

	// Verifica que todos os 3 vídeos foram reenfileirados.
	if len(enqueuedVideos) != 3 {
		t.Errorf("esperava 3 enqueues, obteve %d", len(enqueuedVideos))
	}
}

// TestRecovery_SkipsOtherStatuses verifica que vídeos com outros status
// não são modificados.
func TestRecovery_SkipsOtherStatuses(t *testing.T) {
	database := openTestDB(t)
	defer database.Close()

	// Insere vídeos com status que não devem ser afetados.
	insertTestVideo(t, database, "video-5a", "ready", 0)
	insertTestVideo(t, database, "video-5b", "failed_upload", 0)
	insertTestVideo(t, database, "video-5c", "uploading", 0)
	insertTestVideo(t, database, "video-5d", "pending_upload", 0)

	cfg := &config.Config{MaxTranscodeAttempts: 3}

	var enqueuedVideos []string
	enqueue := func(id string) error {
		enqueuedVideos = append(enqueuedVideos, id)
		return nil
	}

	onWebhook := func(videoID, event, errMsg string) {}

	err := RunStartupRecovery(database, cfg, enqueue, onWebhook)
	if err != nil {
		t.Fatalf("RunStartupRecovery: %v", err)
	}

	// Verifica que nenhum vídeo foi reenfileirado.
	if len(enqueuedVideos) != 0 {
		t.Errorf("esperava nenhuma enqueue, obteve %v", enqueuedVideos)
	}

	// Verifica que os status permaneceram inalterados.
	for _, videoID := range []string{"video-5a", "video-5b", "video-5c", "video-5d"} {
		video, err := models.GetVideo(database, videoID)
		if err != nil {
			t.Fatalf("GetVideo(%s): %v", videoID, err)
		}

		expectedStatuses := map[string]models.VideoStatus{
			"video-5a": models.StatusReady,
			"video-5b": models.StatusFailedUpload,
			"video-5c": models.StatusUploading,
			"video-5d": models.StatusPendingUpload,
		}

		if video.Status != expectedStatuses[videoID] {
			t.Errorf("%s: esperava status %s, obteve %s",
				videoID, expectedStatuses[videoID], video.Status)
		}
	}
}

// TestRecovery_EmptyDB verifica que a recuperação funciona com banco vazio.
func TestRecovery_EmptyDB(t *testing.T) {
	database := openTestDB(t)
	defer database.Close()

	cfg := &config.Config{MaxTranscodeAttempts: 3}

	var enqueuedVideos []string
	enqueue := func(id string) error {
		enqueuedVideos = append(enqueuedVideos, id)
		return nil
	}

	onWebhook := func(videoID, event, errMsg string) {}

	err := RunStartupRecovery(database, cfg, enqueue, onWebhook)
	if err != nil {
		t.Fatalf("RunStartupRecovery: %v", err)
	}

	// Verifica que nenhum vídeo foi reenfileirado.
	if len(enqueuedVideos) != 0 {
		t.Errorf("esperava nenhuma enqueue, obteve %v", enqueuedVideos)
	}
}

// openTestDB abre um banco SQLite em memória com o schema aplicado.
func openTestDB(t *testing.T) *sql.DB {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	return database
}

// insertTestVideo insere um vídeo de teste com status e tentativas especificados.
func insertTestVideo(t *testing.T, database *sql.DB, videoID, status string, attempts int) {
	_, err := database.Exec(
		"INSERT INTO videos (video_id, status, transcode_attempts) VALUES (?, ?, ?)",
		videoID, status, attempts,
	)
	if err != nil {
		t.Fatalf("inserir vídeo de teste %s: %v", videoID, err)
	}
}
