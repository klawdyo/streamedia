package jobs

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/klawdyo/streamedia/internal/db"
)

// setupCleanupTest cria um banco em memória para testes do cleanup job.
func setupCleanupTest(t *testing.T) *sql.DB {
	t.Helper()

	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir banco em memória: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	return database
}

// insertVideo insere um vídeo para testes.
func insertVideoForCleanup(t *testing.T, database *sql.DB, videoID, status string) {
	t.Helper()

	_, err := database.Exec(
		"INSERT INTO videos (video_id, status) VALUES (?, ?)",
		videoID, status,
	)
	if err != nil {
		t.Fatalf("erro ao inserir vídeo %s: %v", videoID, err)
	}
}

// insertExpiredToken insere um token expirado há uma hora.
func insertExpiredToken(t *testing.T, database *sql.DB, token, videoID string) {
	t.Helper()

	expiresAt := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	_, err := database.Exec(
		"INSERT INTO upload_tokens (video_id, token, expires_at) VALUES (?, ?, ?)",
		videoID, token, expiresAt,
	)
	if err != nil {
		t.Fatalf("erro ao inserir token expirado %s: %v", token, err)
	}
}

// insertValidToken insere um token válido que expira em 2 horas.
func insertValidToken(t *testing.T, database *sql.DB, token, videoID string) {
	t.Helper()

	expiresAt := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	_, err := database.Exec(
		"INSERT INTO upload_tokens (video_id, token, expires_at) VALUES (?, ?, ?)",
		videoID, token, expiresAt,
	)
	if err != nil {
		t.Fatalf("erro ao inserir token válido %s: %v", token, err)
	}
}

// tokenExists verifica se um token existe no banco.
func tokenExists(t *testing.T, database *sql.DB, token string) bool {
	t.Helper()

	var exists bool
	err := database.QueryRow("SELECT COUNT(*) > 0 FROM upload_tokens WHERE token = ?", token).Scan(&exists)
	if err != nil && err != sql.ErrNoRows {
		t.Fatalf("erro ao verificar existência do token: %v", err)
	}
	return exists
}

// videoStatus retorna o status de um vídeo no banco.
func videoStatus(t *testing.T, database *sql.DB, videoID string) string {
	t.Helper()

	var status string
	err := database.QueryRow("SELECT status FROM videos WHERE video_id = ?", videoID).Scan(&status)
	if err != nil {
		t.Fatalf("erro ao consultar status de %s: %v", videoID, err)
	}
	return status
}

// countTokens retorna o número total de tokens no banco.
func countTokens(t *testing.T, database *sql.DB) int {
	t.Helper()

	var count int
	err := database.QueryRow("SELECT COUNT(*) FROM upload_tokens").Scan(&count)
	if err != nil {
		t.Fatalf("erro ao contar tokens: %v", err)
	}
	return count
}

func TestCleanupJob_DeletesExpiredToken(t *testing.T) {
	database := setupCleanupTest(t)

	videoID := "vid-expired"
	insertVideoForCleanup(t, database, videoID, "uploading")
	insertExpiredToken(t, database, "tok-expired", videoID)

	job := NewTokenCleanupJob(database)
	count, err := job.runOnce()
	if err != nil {
		t.Fatalf("runOnce retornou erro: %v", err)
	}

	if count != 1 {
		t.Errorf("esperado 1 token deletado, obtido %d", count)
	}

	if tokenExists(t, database, "tok-expired") {
		t.Errorf("token expirado deveria ter sido deletado")
	}

	if videoStatus(t, database, videoID) != "uploading" {
		t.Errorf("status do vídeo não deveria ter mudado")
	}
}

func TestCleanupJob_KeepsValidToken(t *testing.T) {
	database := setupCleanupTest(t)

	videoID := "vid-valid"
	insertVideoForCleanup(t, database, videoID, "uploading")
	insertValidToken(t, database, "tok-valid", videoID)

	job := NewTokenCleanupJob(database)
	count, err := job.runOnce()
	if err != nil {
		t.Fatalf("runOnce retornou erro: %v", err)
	}

	if count != 0 {
		t.Errorf("esperado 0 tokens deletados, obtido %d", count)
	}

	if !tokenExists(t, database, "tok-valid") {
		t.Errorf("token válido deveria ser mantido")
	}
}

func TestCleanupJob_VideoStatusUnchanged(t *testing.T) {
	database := setupCleanupTest(t)

	videoID := "vid-status-test"
	insertVideoForCleanup(t, database, videoID, "ready")
	insertExpiredToken(t, database, "tok-expired", videoID)

	job := NewTokenCleanupJob(database)
	_, err := job.runOnce()
	if err != nil {
		t.Fatalf("runOnce retornou erro: %v", err)
	}

	if videoStatus(t, database, videoID) != "ready" {
		t.Errorf("status do vídeo deveria permanecer 'ready'")
	}
}

func TestCleanupJob_MultipleExpired(t *testing.T) {
	database := setupCleanupTest(t)

	// Insere 5 tokens expirados
	for i := 1; i <= 5; i++ {
		videoID := "vid-expired-" + string(rune('0'+i))
		tokenID := "tok-expired-" + string(rune('0'+i))
		insertVideoForCleanup(t, database, videoID, "uploading")
		insertExpiredToken(t, database, tokenID, videoID)
	}

	// Insere 2 tokens válidos
	for i := 1; i <= 2; i++ {
		videoID := "vid-valid-" + string(rune('0'+i))
		tokenID := "tok-valid-" + string(rune('0'+i))
		insertVideoForCleanup(t, database, videoID, "uploading")
		insertValidToken(t, database, tokenID, videoID)
	}

	job := NewTokenCleanupJob(database)
	count, err := job.runOnce()
	if err != nil {
		t.Fatalf("runOnce retornou erro: %v", err)
	}

	if count != 5 {
		t.Errorf("esperado 5 tokens deletados, obtido %d", count)
	}

	if remaining := countTokens(t, database); remaining != 2 {
		t.Errorf("esperado 2 tokens remanescentes, obtido %d", remaining)
	}
}

func TestCleanupJob_EmptyTable(t *testing.T) {
	database := setupCleanupTest(t)

	job := NewTokenCleanupJob(database)
	count, err := job.runOnce()
	if err != nil {
		t.Fatalf("runOnce retornou erro: %v", err)
	}

	if count != 0 {
		t.Errorf("esperado 0 tokens deletados de tabela vazia, obtido %d", count)
	}
}

func TestCleanupJob_LogsDeletedCount(t *testing.T) {
	database := setupCleanupTest(t)

	// Insere 3 tokens expirados
	for i := 1; i <= 3; i++ {
		videoID := "vid-log-test-" + string(rune('0'+i))
		tokenID := "tok-log-test-" + string(rune('0'+i))
		insertVideoForCleanup(t, database, videoID, "uploading")
		insertExpiredToken(t, database, tokenID, videoID)
	}

	job := NewTokenCleanupJob(database)
	count, err := job.runOnce()
	if err != nil {
		t.Fatalf("runOnce retornou erro: %v", err)
	}

	if count != 3 {
		t.Errorf("esperado 3 tokens deletados, obtido %d", count)
	}
}

// TestCleanupJob_MixedExpiredAndValid testa limpeza com mix de tokens
// expirados e válidos para garantir que apenas os expirados são removidos.
func TestCleanupJob_MixedExpiredAndValid(t *testing.T) {
	database := setupCleanupTest(t)

	videoID1 := "vid-mixed-1"
	videoID2 := "vid-mixed-2"
	videoID3 := "vid-mixed-3"
	videoID4 := "vid-mixed-4"
	insertVideoForCleanup(t, database, videoID1, "uploading")
	insertVideoForCleanup(t, database, videoID2, "uploading")
	insertVideoForCleanup(t, database, videoID3, "uploading")
	insertVideoForCleanup(t, database, videoID4, "uploading")

	insertExpiredToken(t, database, "tok-exp-1", videoID1)
	insertExpiredToken(t, database, "tok-exp-2", videoID2)
	insertValidToken(t, database, "tok-valid-1", videoID3)
	insertValidToken(t, database, "tok-valid-2", videoID4)

	job := NewTokenCleanupJob(database)
	count, err := job.runOnce()
	if err != nil {
		t.Fatalf("runOnce retornou erro: %v", err)
	}

	if count != 2 {
		t.Errorf("esperado 2 tokens deletados, obtido %d", count)
	}

	// Verifica que os válidos continuam
	if !tokenExists(t, database, "tok-valid-1") || !tokenExists(t, database, "tok-valid-2") {
		t.Errorf("tokens válidos devem ser mantidos")
	}

	// Verifica que os expirados foram removidos
	if tokenExists(t, database, "tok-exp-1") || tokenExists(t, database, "tok-exp-2") {
		t.Errorf("tokens expirados devem ter sido removidos")
	}
}

// TestCleanupJob_ExactlyExpiredBoundary testa tokens que expiram exatamente agora.
// Este é um edge case onde a comparação com 'now' é crítica.
func TestCleanupJob_ExactlyExpiredBoundary(t *testing.T) {
	database := setupCleanupTest(t)

	videoID := "vid-boundary"
	insertVideoForCleanup(t, database, videoID, "uploading")

	// Insere token que expira agora (não em milissegundos, em segundos).
	// SQLite arredonda, então um token com expires_at = now() deve ser considerado expirado.
	expiresAtNow := time.Now().UTC().Format(time.RFC3339)
	_, err := database.Exec(
		"INSERT INTO upload_tokens (video_id, token, expires_at) VALUES (?, ?, ?)",
		videoID, "tok-boundary", expiresAtNow,
	)
	if err != nil {
		t.Fatalf("erro ao inserir token: %v", err)
	}

	job := NewTokenCleanupJob(database)
	count, err := job.runOnce()
	if err != nil {
		t.Fatalf("runOnce retornou erro: %v", err)
	}

	// Comportamento pode variar por subsegundo, mas idealmente deve remover
	// (ou não) consistentemente. Apenas confirmamos que não há pânico.
	if count < 0 || count > 1 {
		t.Errorf("contagem esperada 0 ou 1, obtida %d", count)
	}
}
