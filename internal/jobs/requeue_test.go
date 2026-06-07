package jobs

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/db"
	"github.com/klawdyo/streamedia/internal/models"
)

// setupRequeueTest cria um banco em memória e uma config apontando para um
// timeout de transcodificação travada, com o número máximo de tentativas.
func setupRequeueTest(t *testing.T, transcodeStuckTime time.Duration, maxAttempts int) (*sql.DB, *config.Config) {
	t.Helper()

	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir banco em memória: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	cfg := &config.Config{
		TranscodeStuckTime:   transcodeStuckTime,
		MaxTranscodeAttempts: maxAttempts,
	}

	return database, cfg
}

// insertTranscodeVideo insere um vídeo com status de transcodificação e
// timestamps customizados.
func insertTranscodeVideo(
	t *testing.T,
	database *sql.DB,
	videoID string,
	transcodeAttempts int,
	updatedAt time.Time,
) {
	t.Helper()

	_, err := database.Exec(
		"INSERT INTO videos (video_id, status, transcode_attempts, updated_at) VALUES (?, ?, ?, ?)",
		videoID, models.StatusTranscoding, transcodeAttempts, updatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		t.Fatalf("erro ao inserir vídeo %s: %v", videoID, err)
	}
}

// statusOf retorna o status atual de um vídeo no banco.
func statusOfRequeue(t *testing.T, database *sql.DB, videoID string) string {
	t.Helper()
	var status string
	if err := database.QueryRow("SELECT status FROM videos WHERE video_id = ?", videoID).Scan(&status); err != nil {
		t.Fatalf("erro ao consultar status de %s: %v", videoID, err)
	}
	return status
}

// transcodeAttemptsOf retorna o número de tentativas de transcodificação de um vídeo.
func transcodeAttemptsOf(t *testing.T, database *sql.DB, videoID string) int {
	t.Helper()
	var attempts int
	if err := database.QueryRow("SELECT transcode_attempts FROM videos WHERE video_id = ?", videoID).Scan(&attempts); err != nil {
		t.Fatalf("erro ao consultar transcode_attempts de %s: %v", videoID, err)
	}
	return attempts
}

// errorMessageOf retorna a mensagem de erro armazenada para um vídeo.
func errorMessageOf(t *testing.T, database *sql.DB, videoID string) string {
	t.Helper()
	var errMsg sql.NullString
	if err := database.QueryRow("SELECT error_message FROM videos WHERE video_id = ?", videoID).Scan(&errMsg); err != nil {
		t.Fatalf("erro ao consultar error_message de %s: %v", videoID, err)
	}
	return errMsg.String
}

func TestRequeueJob_RequeuesStuckTranscode(t *testing.T) {
	database, cfg := setupRequeueTest(t, 30*time.Minute, 3)
	videoID := "vid-stuck"

	// Vídeo em transcodificação há 31 minutos (travado), com 0 tentativas.
	updatedAt := time.Now().Add(-31 * time.Minute)
	insertTranscodeVideo(t, database, videoID, 0, updatedAt)

	// Registra chamadas de enqueue.
	var enqueueCalls []string
	job := NewTranscodeRequeueJob(cfg, database, func(vID string) error {
		enqueueCalls = append(enqueueCalls, vID)
		return nil
	}, func(string, string, string) {})

	if err := job.runOnce(); err != nil {
		t.Fatalf("runOnce retornou erro: %v", err)
	}

	// Verifica se o status mudou para upload_complete.
	if got := statusOfRequeue(t, database, videoID); got != string(models.StatusUploadComplete) {
		t.Errorf("status esperado %s, obtido %q", models.StatusUploadComplete, got)
	}

	// Verifica se o contador de tentativas foi incrementado.
	if got := transcodeAttemptsOf(t, database, videoID); got != 1 {
		t.Errorf("transcode_attempts esperado 1, obtido %d", got)
	}

	// Verifica se enqueue foi chamado.
	if len(enqueueCalls) != 1 || enqueueCalls[0] != videoID {
		t.Errorf("enqueue esperado ser chamado com %q, foi chamado com %v", videoID, enqueueCalls)
	}
}

func TestRequeueJob_SkipsRecentTranscode(t *testing.T) {
	database, cfg := setupRequeueTest(t, 30*time.Minute, 3)
	videoID := "vid-recent"

	// Vídeo em transcodificação há 29 minutos (dentro do timeout).
	updatedAt := time.Now().Add(-29 * time.Minute)
	insertTranscodeVideo(t, database, videoID, 0, updatedAt)

	job := NewTranscodeRequeueJob(cfg, database, func(vID string) error {
		t.Errorf("enqueue não deveria ter sido chamado para vídeo recente")
		return nil
	}, func(string, string, string) {})

	if err := job.runOnce(); err != nil {
		t.Fatalf("runOnce retornou erro: %v", err)
	}

	// Status deve permanecer em transcodificação.
	if got := statusOfRequeue(t, database, videoID); got != string(models.StatusTranscoding) {
		t.Errorf("status esperado %s (inalterado), obtido %q", models.StatusTranscoding, got)
	}
}

func TestRequeueJob_FailsAfterMaxAttempts(t *testing.T) {
	database, cfg := setupRequeueTest(t, 30*time.Minute, 3)
	videoID := "vid-maxed"

	// Vídeo em transcodificação há 31 minutos, com 3 tentativas (máximo atingido).
	updatedAt := time.Now().Add(-31 * time.Minute)
	insertTranscodeVideo(t, database, videoID, 3, updatedAt)

	var webhookCalls []string
	var webhookEvents []string
	job := NewTranscodeRequeueJob(cfg, database, func(vID string) error {
		t.Errorf("enqueue não deveria ter sido chamado para vídeo maxed out")
		return nil
	}, func(vID, event, errMsg string) {
		webhookCalls = append(webhookCalls, vID)
		webhookEvents = append(webhookEvents, event)
	})

	if err := job.runOnce(); err != nil {
		t.Fatalf("runOnce retornou erro: %v", err)
	}

	// Status deve mudar para failed_transcode.
	if got := statusOfRequeue(t, database, videoID); got != string(models.StatusFailedTranscode) {
		t.Errorf("status esperado %s, obtido %q", models.StatusFailedTranscode, got)
	}

	// Webhook deve ter sido chamado com evento "failed".
	if len(webhookCalls) != 1 || webhookCalls[0] != videoID {
		t.Errorf("webhook esperado ser chamado com %q, foi chamado com %v", videoID, webhookCalls)
	}
	if len(webhookEvents) != 1 || webhookEvents[0] != "failed" {
		t.Errorf("evento do webhook esperado \"failed\", obtido %q", webhookEvents[0])
	}

	// Verifica se a mensagem de erro foi gravada.
	if got := errorMessageOf(t, database, videoID); got == "" {
		t.Errorf("mensagem de erro deveria ter sido gravada, mas está vazia")
	}
}

func TestRequeueJob_SkipsNonTranscodingStatuses(t *testing.T) {
	database, cfg := setupRequeueTest(t, 30*time.Minute, 3)
	videoID := "vid-uploading"

	// Vídeo em upload há 1 hora (mesmo sendo antigo, não é transcodificação).
	_, err := database.Exec(
		"INSERT INTO videos (video_id, status, transcode_attempts, updated_at) VALUES (?, ?, ?, ?)",
		videoID, models.StatusUploading, 0, time.Now().Add(-1*time.Hour).UTC().Format(time.RFC3339),
	)
	if err != nil {
		t.Fatalf("erro ao inserir vídeo: %v", err)
	}

	job := NewTranscodeRequeueJob(cfg, database, func(vID string) error {
		t.Errorf("enqueue não deveria ter sido chamado para status não-transcodificação")
		return nil
	}, func(string, string, string) {})

	if err := job.runOnce(); err != nil {
		t.Fatalf("runOnce retornou erro: %v", err)
	}

	// Status deve permanecer em uploading.
	if got := statusOfRequeue(t, database, videoID); got != string(models.StatusUploading) {
		t.Errorf("status esperado %s (inalterado), obtido %q", models.StatusUploading, got)
	}
}

func TestRequeueJob_ExactlyAtLimit(t *testing.T) {
	database, cfg := setupRequeueTest(t, 30*time.Minute, 3)
	videoID := "vid-last-chance"

	// Vídeo em transcodificação há 31 minutos, com 2 tentativas
	// (MaxTranscodeAttempts=3, então esta é a última chance).
	updatedAt := time.Now().Add(-31 * time.Minute)
	insertTranscodeVideo(t, database, videoID, 2, updatedAt)

	var enqueueCalls []string
	job := NewTranscodeRequeueJob(cfg, database, func(vID string) error {
		enqueueCalls = append(enqueueCalls, vID)
		return nil
	}, func(string, string, string) {})

	if err := job.runOnce(); err != nil {
		t.Fatalf("runOnce retornou erro: %v", err)
	}

	// Status deve ser upload_complete (reenfileirado, não falhado).
	if got := statusOfRequeue(t, database, videoID); got != string(models.StatusUploadComplete) {
		t.Errorf("status esperado %s (última chance, não falha), obtido %q", models.StatusUploadComplete, got)
	}

	// transcode_attempts deve ser 3.
	if got := transcodeAttemptsOf(t, database, videoID); got != 3 {
		t.Errorf("transcode_attempts esperado 3, obtido %d", got)
	}

	// Enqueue deve ter sido chamado (ainda há tentativa).
	if len(enqueueCalls) != 1 {
		t.Errorf("enqueue esperado ser chamado 1 vez, foi chamado %d vezes", len(enqueueCalls))
	}
}

func TestRequeueJob_CallsEnqueue(t *testing.T) {
	database, cfg := setupRequeueTest(t, 30*time.Minute, 3)
	videoID := "vid-enqueue"

	updatedAt := time.Now().Add(-31 * time.Minute)
	insertTranscodeVideo(t, database, videoID, 1, updatedAt)

	var enqueueCalls []string
	job := NewTranscodeRequeueJob(cfg, database, func(vID string) error {
		enqueueCalls = append(enqueueCalls, vID)
		return nil
	}, func(string, string, string) {})

	if err := job.runOnce(); err != nil {
		t.Fatalf("runOnce retornou erro: %v", err)
	}

	// Enqueue deve ter sido chamado com o videoID correto.
	if len(enqueueCalls) != 1 || enqueueCalls[0] != videoID {
		t.Errorf("enqueue esperado ser chamado com %q, foi chamado com %v", videoID, enqueueCalls)
	}
}
