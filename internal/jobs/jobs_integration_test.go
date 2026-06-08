package jobs

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/db"
	"github.com/klawdyo/streamedia/internal/models"
)

// TestTokenCleanupJob_StartAndStop verifica que Start() inicia uma goroutine
// e Stop() a encerra sem panic.
func TestTokenCleanupJob_StartAndStop(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir banco em memória: %v", err)
	}
	defer database.Close()

	job := NewTokenCleanupJob(database)
	job.Start()
	time.Sleep(10 * time.Millisecond) // Garante que goroutine foi iniciada
	job.Stop()
	// Se chegou aqui sem panic, passou
}

// TestUploadKillerJob_StartAndStop verifica que Start() inicia uma goroutine
// e Stop() a encerra sem panic.
func TestUploadKillerJob_StartAndStop(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir banco em memória: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		UploadTmpDir:      t.TempDir(),
		UploadIdleTimeout: 10 * time.Minute,
	}

	job := NewUploadKillerJob(cfg, database, func(string, string, string) {})
	job.Start()
	time.Sleep(10 * time.Millisecond) // Garante que goroutine foi iniciada
	job.Stop()
	// Se chegou aqui sem panic, passou
}

// TestTranscodeRequeueJob_StartAndStop verifica que Start() inicia uma goroutine
// e Stop() a encerra sem panic.
func TestTranscodeRequeueJob_StartAndStop(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir banco em memória: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		TranscodeStuckTime:   30 * time.Minute,
		MaxTranscodeAttempts: 3,
	}

	job := NewTranscodeRequeueJob(cfg, database, func(string) error { return nil }, func(string, string, string) {})
	job.Start()
	time.Sleep(10 * time.Millisecond) // Garante que goroutine foi iniciada
	job.Stop()
	// Se chegou aqui sem panic, passou
}

// TestUploadKillerJob_QueryError_DBFailure testa que se a query falhar,
// runOnce retorna um erro sem panic.
func TestUploadKillerJob_QueryError_DBFailure(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir banco em memória: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		UploadTmpDir:      t.TempDir(),
		UploadIdleTimeout: 10 * time.Minute,
	}

	job := NewUploadKillerJob(cfg, database, func(string, string, string) {})

	// Fecha o banco para simular falha de acesso
	_ = database.Close()

	err = job.runOnce()
	if err == nil {
		t.Error("esperado erro de query, mas runOnce retornou nil")
	}
}

// TestTokenCleanupJob_DBError_ModelsFails testa que se DeleteExpiredTokens
// falhar, runOnce retorna o erro.
func TestTokenCleanupJob_QueryError_Fails(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir banco em memória: %v", err)
	}
	defer database.Close()

	job := NewTokenCleanupJob(database)

	// Fecha o banco para simular falha de acesso
	_ = database.Close()

	_, err = job.runOnce()
	if err == nil {
		t.Error("esperado erro de query, mas runOnce retornou nil")
	}
}

// TestTranscodeRequeueJob_UpdateStatusError_Continues testa que se
// UpdateStatus falhar para um vídeo, o job continua processando os demais.
func TestTranscodeRequeueJob_UpdateStatusError_Continues(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir banco em memória: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		TranscodeStuckTime:   30 * time.Minute,
		MaxTranscodeAttempts: 3,
	}

	// Insere dois vídeos travados
	now := time.Now().UTC()
	_, _ = database.Exec(
		"INSERT INTO videos (video_id, status, transcode_attempts, updated_at) VALUES (?, ?, ?, ?)",
		"vid-1", models.StatusTranscoding, 1, now.Add(-31*time.Minute).Format(time.RFC3339),
	)
	_, _ = database.Exec(
		"INSERT INTO videos (video_id, status, transcode_attempts, updated_at) VALUES (?, ?, ?, ?)",
		"vid-2", models.StatusTranscoding, 1, now.Add(-31*time.Minute).Format(time.RFC3339),
	)

	var enqueueCalls []string
	job := NewTranscodeRequeueJob(cfg, database, func(vID string) error {
		enqueueCalls = append(enqueueCalls, vID)
		return nil
	}, func(string, string, string) {})

	// Primeiro runOnce funciona
	if err := job.runOnce(); err != nil {
		t.Fatalf("primeiro runOnce falhou: %v", err)
	}

	// Ambos devem ter sido processados
	if len(enqueueCalls) != 2 {
		t.Errorf("esperado 2 chamadas de enqueue, obtido %d", len(enqueueCalls))
	}
}

// TestUploadKillerJob_InfoFileDoesNotExist testa que killer continua mesmo
// se arquivo .info não existir (ignora erro de RemoveFile).
func TestUploadKillerJob_InfoFileDoesNotExist(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir banco em memória: %v", err)
	}
	defer database.Close()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		UploadTmpDir:      tmpDir,
		UploadIdleTimeout: 10 * time.Minute,
	}

	// Insere vídeo inativo
	now := time.Now().UTC()
	_, _ = database.Exec(
		"INSERT INTO videos (video_id, status, created_at) VALUES (?, ?, ?)",
		"vid-no-info", "uploading", now.Add(-11*time.Minute).Format(time.RFC3339),
	)

	// Cria arquivo de upload mas NÃO o arquivo .info
	uploadFile := filepath.Join(tmpDir, "vid-no-info")
	if err := os.WriteFile(uploadFile, []byte("partial upload"), 0600); err != nil {
		t.Fatalf("erro ao criar arquivo de upload: %v", err)
	}

	var webhookCalls []string
	job := NewUploadKillerJob(cfg, database, func(videoID, event, errMsg string) {
		if event == "failed" {
			webhookCalls = append(webhookCalls, videoID)
		}
	})

	if err := job.runOnce(); err != nil {
		t.Fatalf("runOnce falhou: %v", err)
	}

	// Deve ter matado o upload e chamado webhook
	if len(webhookCalls) != 1 {
		t.Errorf("esperado 1 webhook call, obtido %d", len(webhookCalls))
	}

	// Arquivo deve ter sido removido
	if _, err := os.Stat(uploadFile); !os.IsNotExist(err) {
		t.Error("arquivo de upload deveria ter sido removido")
	}
}

// TestUploadKillerJob_UsesLastChunkAt_WhenAvailable testa que killer usa
// last_chunk_at quando disponível (não created_at).
func TestUploadKillerJob_UsesLastChunkAt_WhenAvailable(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir banco em memória: %v", err)
	}
	defer database.Close()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		UploadTmpDir:      tmpDir,
		UploadIdleTimeout: 10 * time.Minute,
	}

	now := time.Now().UTC()
	createdAt := now.Add(-60 * time.Minute) // Criado há 60 min
	lastChunkAt := now.Add(-5 * time.Minute) // Último chunk há 5 min (NOT inativo)

	// Insere vídeo com último chunk recente
	_, _ = database.Exec(
		"INSERT INTO videos (video_id, status, created_at, last_chunk_at) VALUES (?, ?, ?, ?)",
		"vid-recent-chunk", "uploading",
		createdAt.Format(time.RFC3339),
		lastChunkAt.Format(time.RFC3339),
	)

	var webhookCalls []string
	job := NewUploadKillerJob(cfg, database, func(videoID, event, errMsg string) {
		webhookCalls = append(webhookCalls, videoID)
	})

	if err := job.runOnce(); err != nil {
		t.Fatalf("runOnce falhou: %v", err)
	}

	// Não deve ter matado (chunk recente, mesmo com created_at antigo)
	if len(webhookCalls) != 0 {
		t.Errorf("esperado 0 webhooks (chunk recente), obtido %d", len(webhookCalls))
	}
}

// TestTokenCleanupJob_RunOnceReturnsCount testa que runOnce retorna o número
// correto de tokens deletados.
func TestTokenCleanupJob_RunOnceReturnsCount(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir banco em memória: %v", err)
	}
	defer database.Close()

	now := time.Now().UTC()
	// Insere vídeos
	_, _ = database.Exec(
		"INSERT INTO videos (video_id, status) VALUES (?, ?)",
		"vid-1", "uploading",
	)
	_, _ = database.Exec(
		"INSERT INTO videos (video_id, status) VALUES (?, ?)",
		"vid-2", "uploading",
	)

	// Insere tokens expirados
	_, _ = database.Exec(
		"INSERT INTO upload_tokens (video_id, token, expires_at) VALUES (?, ?, ?)",
		"vid-1", "tok-1", now.Add(-1*time.Hour).Format(time.RFC3339),
	)
	_, _ = database.Exec(
		"INSERT INTO upload_tokens (video_id, token, expires_at) VALUES (?, ?, ?)",
		"vid-2", "tok-2", now.Add(-2*time.Hour).Format(time.RFC3339),
	)

	job := NewTokenCleanupJob(database)
	count, err := job.runOnce()

	if err != nil {
		t.Fatalf("runOnce retornou erro: %v", err)
	}

	if count != 2 {
		t.Errorf("esperado 2 tokens deletados, obtido %d", count)
	}
}

// TestTranscodeRequeueJob_IncrementAttempts_ThenUpdate verifica que attempts
// é incrementado antes de UpdateStatus ser chamado.
func TestTranscodeRequeueJob_IncrementAttempts_ThenUpdate(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir banco em memória: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		TranscodeStuckTime:   30 * time.Minute,
		MaxTranscodeAttempts: 3,
	}

	now := time.Now().UTC()
	_, _ = database.Exec(
		"INSERT INTO videos (video_id, status, transcode_attempts, updated_at) VALUES (?, ?, ?, ?)",
		"vid-requeue", models.StatusTranscoding, 0, now.Add(-31*time.Minute).Format(time.RFC3339),
	)

	job := NewTranscodeRequeueJob(cfg, database, func(vID string) error {
		return nil
	}, func(string, string, string) {})

	if err := job.runOnce(); err != nil {
		t.Fatalf("runOnce falhou: %v", err)
	}

	// Verifica que attempts foi incrementado
	var attempts int
	if err := database.QueryRow("SELECT transcode_attempts FROM videos WHERE video_id = ?", "vid-requeue").Scan(&attempts); err != nil {
		t.Fatalf("erro ao consultar attempts: %v", err)
	}

	if attempts != 1 {
		t.Errorf("esperado attempts=1, obtido %d", attempts)
	}

	// Verifica que status foi atualizado para upload_complete
	var status string
	if err := database.QueryRow("SELECT status FROM videos WHERE video_id = ?", "vid-requeue").Scan(&status); err != nil {
		t.Fatalf("erro ao consultar status: %v", err)
	}

	if status != string(models.StatusUploadComplete) {
		t.Errorf("esperado status=%s, obtido %s", models.StatusUploadComplete, status)
	}
}
