package jobs

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/db"
)

// setupTest cria um banco em memória e uma config apontando para um
// diretório temporário de uploads, com timeout de inatividade de 10 minutos.
func setupTest(t *testing.T) (*sql.DB, *config.Config) {
	t.Helper()

	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir banco em memória: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	cfg := &config.Config{
		UploadTmpDir:      t.TempDir(),
		UploadIdleTimeout: 10 * time.Minute,
	}

	return database, cfg
}

// insertVideo insere um vídeo com timestamps customizados. lastChunkAt e
// createdAt são strings RFC3339 ou vazias (para NULL).
func insertVideo(t *testing.T, database *sql.DB, videoID, status, lastChunkAt, createdAt string) {
	t.Helper()

	var lc interface{}
	if lastChunkAt == "" {
		lc = nil
	} else {
		lc = lastChunkAt
	}

	_, err := database.Exec(
		"INSERT INTO videos (video_id, status, last_chunk_at, created_at) VALUES (?, ?, ?, ?)",
		videoID, status, lc, createdAt,
	)
	if err != nil {
		t.Fatalf("erro ao inserir vídeo %s: %v", videoID, err)
	}
}

// statusOf retorna o status atual de um vídeo no banco.
func statusOf(t *testing.T, database *sql.DB, videoID string) string {
	t.Helper()
	var status string
	if err := database.QueryRow("SELECT status FROM videos WHERE video_id = ?", videoID).Scan(&status); err != nil {
		t.Fatalf("erro ao consultar status de %s: %v", videoID, err)
	}
	return status
}

// rfc formata um instante relativo ao agora, em UTC RFC3339.
func rfc(d time.Duration) string {
	return time.Now().Add(d).UTC().Format(time.RFC3339)
}

func TestKillerJob_KillsInactiveUpload(t *testing.T) {
	database, cfg := setupTest(t)
	videoID := "vid-inactive"

	insertVideo(t, database, videoID, "uploading", rfc(-11*time.Minute), rfc(-20*time.Minute))

	// Cria o arquivo temporário do upload em disco.
	tmpPath := filepath.Join(cfg.UploadTmpDir, videoID)
	if err := os.WriteFile(tmpPath, []byte("dados parciais"), 0o644); err != nil {
		t.Fatalf("erro ao criar arquivo temporário: %v", err)
	}

	job := NewUploadKillerJob(cfg, database, func(string, string, string) {})
	if err := job.runOnce(); err != nil {
		t.Fatalf("runOnce retornou erro: %v", err)
	}

	if got := statusOf(t, database, videoID); got != "failed_upload" {
		t.Errorf("status esperado failed_upload, obtido %q", got)
	}

	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Errorf("arquivo temporário deveria ter sido removido, mas ainda existe (err=%v)", err)
	}
}

func TestKillerJob_SkipsActiveUpload(t *testing.T) {
	database, cfg := setupTest(t)
	videoID := "vid-active"

	// last_chunk_at há 9 minutos: dentro do timeout de 10 minutos.
	insertVideo(t, database, videoID, "uploading", rfc(-9*time.Minute), rfc(-20*time.Minute))

	job := NewUploadKillerJob(cfg, database, func(string, string, string) {})
	if err := job.runOnce(); err != nil {
		t.Fatalf("runOnce retornou erro: %v", err)
	}

	if got := statusOf(t, database, videoID); got != "uploading" {
		t.Errorf("status esperado uploading (não morto), obtido %q", got)
	}
}

func TestKillerJob_SkipsReadyVideo(t *testing.T) {
	database, cfg := setupTest(t)
	videoID := "vid-ready"

	insertVideo(t, database, videoID, "ready", rfc(-2*time.Hour), rfc(-3*time.Hour))

	job := NewUploadKillerJob(cfg, database, func(string, string, string) {})
	if err := job.runOnce(); err != nil {
		t.Fatalf("runOnce retornou erro: %v", err)
	}

	if got := statusOf(t, database, videoID); got != "ready" {
		t.Errorf("status esperado ready, obtido %q", got)
	}
}

func TestKillerJob_SkipsTerminalStates(t *testing.T) {
	database, cfg := setupTest(t)
	videoID := "vid-terminal"

	insertVideo(t, database, videoID, "failed_upload", rfc(-1*time.Hour), rfc(-2*time.Hour))

	job := NewUploadKillerJob(cfg, database, func(string, string, string) {})
	if err := job.runOnce(); err != nil {
		t.Fatalf("runOnce retornou erro: %v", err)
	}

	if got := statusOf(t, database, videoID); got != "failed_upload" {
		t.Errorf("status esperado failed_upload (inalterado), obtido %q", got)
	}
}

func TestKillerJob_DeletesInfoFile(t *testing.T) {
	database, cfg := setupTest(t)
	videoID := "vid-info"

	insertVideo(t, database, videoID, "uploading", rfc(-11*time.Minute), rfc(-20*time.Minute))

	// Cria o arquivo {videoID}.info em disco.
	infoPath := filepath.Join(cfg.UploadTmpDir, videoID+".info")
	if err := os.WriteFile(infoPath, []byte("metadados"), 0o644); err != nil {
		t.Fatalf("erro ao criar arquivo .info: %v", err)
	}

	job := NewUploadKillerJob(cfg, database, func(string, string, string) {})
	if err := job.runOnce(); err != nil {
		t.Fatalf("runOnce retornou erro: %v", err)
	}

	if _, err := os.Stat(infoPath); !os.IsNotExist(err) {
		t.Errorf("arquivo .info deveria ter sido removido, mas ainda existe (err=%v)", err)
	}
}

func TestKillerJob_NullLastChunkAt(t *testing.T) {
	database, cfg := setupTest(t)
	videoID := "vid-null-chunk"

	// last_chunk_at NULL, created_at há 11 minutos → usa created_at como fallback.
	insertVideo(t, database, videoID, "pending_upload", "", rfc(-11*time.Minute))

	job := NewUploadKillerJob(cfg, database, func(string, string, string) {})
	if err := job.runOnce(); err != nil {
		t.Fatalf("runOnce retornou erro: %v", err)
	}

	if got := statusOf(t, database, videoID); got != "failed_upload" {
		t.Errorf("status esperado failed_upload (fallback created_at), obtido %q", got)
	}
}

func TestKillerJob_DispatchesWebhook(t *testing.T) {
	database, cfg := setupTest(t)
	videoID := "vid-webhook"

	insertVideo(t, database, videoID, "uploading", rfc(-11*time.Minute), rfc(-20*time.Minute))

	var (
		gotVideoID string
		gotEvent   string
		gotErrMsg  string
		called     int
	)
	job := NewUploadKillerJob(cfg, database, func(vID, event, errMsg string) {
		called++
		gotVideoID = vID
		gotEvent = event
		gotErrMsg = errMsg
	})
	if err := job.runOnce(); err != nil {
		t.Fatalf("runOnce retornou erro: %v", err)
	}

	if called != 1 {
		t.Fatalf("webhook esperado ser chamado 1 vez, foi chamado %d vezes", called)
	}
	if gotVideoID != videoID {
		t.Errorf("videoID do webhook esperado %q, obtido %q", videoID, gotVideoID)
	}
	if gotEvent != "failed" {
		t.Errorf("evento do webhook esperado \"failed\", obtido %q", gotEvent)
	}
	if gotErrMsg == "" {
		t.Errorf("mensagem de erro do webhook não deveria ser vazia")
	}
}
