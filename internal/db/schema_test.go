package db

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestSchema_TablesExist(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer database.Close()

	tables := []string{
		"videos",
		"access_tokens",
		"webhook_log",
		"playback_events",
		"video_renditions",
	}

	for _, tableName := range tables {
		var count int
		if err := database.QueryRow(
			"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?",
			tableName,
		).Scan(&count); err != nil {
			t.Fatalf("erro ao verificar tabela %s: %v", tableName, err)
		}
		if count != 1 {
			t.Errorf("tabela %q não existe no schema", tableName)
		}
	}
}

func TestSchema_ProjectsTableRemoved(t *testing.T) {
	// A tabela projects foi removida no modelo de tags.
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer database.Close()

	var count int
	if err := database.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='projects'",
	).Scan(&count); err != nil {
		t.Fatalf("erro ao consultar sqlite_master: %v", err)
	}
	if count != 0 {
		t.Error("tabela projects deveria ter sido removida do schema")
	}
}

func columnsOf(t *testing.T, database *sql.DB, table string) map[string]bool {
	t.Helper()
	rows, err := database.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		t.Fatalf("PRAGMA table_info(%s) falhou: %v", table, err)
	}
	defer rows.Close()

	found := make(map[string]bool)
	for rows.Next() {
		var cid, notNull, pk int
		var name, ctype string
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dfltValue, &pk); err != nil {
			t.Fatalf("erro ao ler coluna: %v", err)
		}
		found[name] = true
	}
	return found
}

func TestSchema_VideosTable_Columns(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer database.Close()

	expectedColumns := []string{
		"video_id", "status", "declared_size_bytes", "actual_size_bytes",
		"duration_s", "resolutions", "transcode_attempts", "last_chunk_at",
		"error_message", "tag", "created_at", "updated_at",
	}
	found := columnsOf(t, database, "videos")
	for _, col := range expectedColumns {
		if !found[col] {
			t.Errorf("coluna %q não encontrada em videos", col)
		}
	}
}

func TestSchema_AccessTokensTable_Columns(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer database.Close()

	expectedColumns := []string{"token", "video_id", "purpose", "expires_at"}
	found := columnsOf(t, database, "access_tokens")
	for _, col := range expectedColumns {
		if !found[col] {
			t.Errorf("coluna %q não encontrada em access_tokens", col)
		}
	}
}

func TestSchema_VideoRenditionsTable(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer database.Close()

	expectedColumns := []string{"video_id", "resolution", "size_bytes", "segment_count", "updated_at"}
	found := columnsOf(t, database, "video_renditions")
	for _, col := range expectedColumns {
		if !found[col] {
			t.Errorf("coluna %q não encontrada em video_renditions", col)
		}
	}
}

func TestSchema_IndicesCreated(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer database.Close()

	expectedIndices := []string{
		"idx_videos_status",
		"idx_videos_last_chunk",
		"idx_videos_tag",
		"idx_access_tokens_expires",
		"idx_playback_events_video",
		"idx_playback_events_occurred",
		"idx_video_renditions_video",
	}

	for _, indexName := range expectedIndices {
		var count int
		if err := database.QueryRow(
			"SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?",
			indexName,
		).Scan(&count); err != nil {
			t.Fatalf("erro ao verificar índice %s: %v", indexName, err)
		}
		if count != 1 {
			t.Errorf("índice %q não existe", indexName)
		}
	}
}

func TestSchema_TriggersCreated(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer database.Close()

	var count int
	if err := database.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='trigger' AND name=?",
		"videos_updated_at",
	).Scan(&count); err != nil {
		t.Fatalf("erro ao verificar trigger: %v", err)
	}
	if count != 1 {
		t.Error("trigger videos_updated_at não existe")
	}
}

func TestSchema_AccessTokensUniqueVideoPurpose(t *testing.T) {
	// UNIQUE(video_id, purpose): dois tokens de mesmo propósito para o mesmo
	// vídeo conflitam (INSERT puro falha).
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer database.Close()

	if _, err := database.Exec("INSERT INTO videos (video_id, tag) VALUES ('v1', 'default')"); err != nil {
		t.Fatalf("INSERT video falhou: %v", err)
	}
	if _, err := database.Exec("INSERT INTO access_tokens (token, video_id, purpose, expires_at) VALUES ('t1', 'v1', 'upload', '2099-01-01 00:00:00')"); err != nil {
		t.Fatalf("primeiro INSERT token falhou: %v", err)
	}
	_, err = database.Exec("INSERT INTO access_tokens (token, video_id, purpose, expires_at) VALUES ('t2', 'v1', 'upload', '2099-01-01 00:00:00')")
	if err == nil {
		t.Error("esperava erro de UNIQUE(video_id, purpose) para mesmo propósito duplicado")
	}
}
