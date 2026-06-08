package db

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestSchema_TablesExist(t *testing.T) {
	// Verifica que o schema cria todas as tabelas esperadas.
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer database.Close()

	tables := []string{
		"videos",
		"upload_tokens",
		"webhook_log",
		"playback_events",
		"projects",
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

func TestSchema_VideosTable_Columns(t *testing.T) {
	// Verifica que a tabela videos tem todas as colunas esperadas.
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer database.Close()

	expectedColumns := []string{
		"video_id",
		"status",
		"declared_size_bytes",
		"actual_size_bytes",
		"duration_s",
		"resolutions",
		"transcode_attempts",
		"last_chunk_at",
		"error_message",
		"created_at",
		"updated_at",
	}

	rows, err := database.Query("PRAGMA table_info(videos)")
	if err != nil {
		t.Fatalf("PRAGMA table_info(videos) falhou: %v", err)
	}
	defer rows.Close()

	foundColumns := make(map[string]bool)
	for rows.Next() {
		var cid, notNull, pk int
		var name, ctype string
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dfltValue, &pk); err != nil {
			t.Fatalf("erro ao ler coluna: %v", err)
		}
		foundColumns[name] = true
	}

	for _, col := range expectedColumns {
		if !foundColumns[col] {
			t.Errorf("coluna %q não encontrada em videos", col)
		}
	}
}

func TestSchema_UploadTokensTable_Columns(t *testing.T) {
	// Verifica que a tabela upload_tokens tem as colunas esperadas.
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer database.Close()

	expectedColumns := []string{
		"token",
		"video_id",
		"expires_at",
	}

	rows, err := database.Query("PRAGMA table_info(upload_tokens)")
	if err != nil {
		t.Fatalf("PRAGMA table_info(upload_tokens) falhou: %v", err)
	}
	defer rows.Close()

	foundColumns := make(map[string]bool)
	for rows.Next() {
		var cid, notNull, pk int
		var name, ctype string
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dfltValue, &pk); err != nil {
			t.Fatalf("erro ao ler coluna: %v", err)
		}
		foundColumns[name] = true
	}

	for _, col := range expectedColumns {
		if !foundColumns[col] {
			t.Errorf("coluna %q não encontrada em upload_tokens", col)
		}
	}
}

func TestSchema_ProjectsTable_Columns(t *testing.T) {
	// Verifica que a tabela projects (T33) tem as colunas esperadas.
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer database.Close()

	expectedColumns := []string{
		"id",
		"name",
		"slug",
		"root_dir",
		"master_key_hash",
		"created_at",
	}

	rows, err := database.Query("PRAGMA table_info(projects)")
	if err != nil {
		t.Fatalf("PRAGMA table_info(projects) falhou: %v", err)
	}
	defer rows.Close()

	foundColumns := make(map[string]bool)
	for rows.Next() {
		var cid, notNull, pk int
		var name, ctype string
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dfltValue, &pk); err != nil {
			t.Fatalf("erro ao ler coluna: %v", err)
		}
		foundColumns[name] = true
	}

	for _, col := range expectedColumns {
		if !foundColumns[col] {
			t.Errorf("coluna %q não encontrada em projects", col)
		}
	}
}

func TestSchema_VideoRenditionsTable(t *testing.T) {
	// Verifica que a tabela video_renditions (T36) foi criada.
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer database.Close()

	expectedColumns := []string{
		"video_id",
		"resolution",
		"size_bytes",
		"segment_count",
		"updated_at",
	}

	rows, err := database.Query("PRAGMA table_info(video_renditions)")
	if err != nil {
		t.Fatalf("PRAGMA table_info(video_renditions) falhou: %v", err)
	}
	defer rows.Close()

	foundColumns := make(map[string]bool)
	for rows.Next() {
		var cid, notNull, pk int
		var name, ctype string
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dfltValue, &pk); err != nil {
			t.Fatalf("erro ao ler coluna: %v", err)
		}
		foundColumns[name] = true
	}

	for _, col := range expectedColumns {
		if !foundColumns[col] {
			t.Errorf("coluna %q não encontrada em video_renditions", col)
		}
	}
}

func TestSchema_IndicesCreated(t *testing.T) {
	// Verifica que todos os índices foram criados.
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer database.Close()

	expectedIndices := []string{
		"idx_videos_status",
		"idx_videos_last_chunk",
		"idx_tokens_expires",
		"idx_playback_events_video",
		"idx_playback_events_occurred",
		"idx_projects_slug",
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
	// Verifica que o trigger updated_at foi criado.
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

func TestSchema_ProjectIDMigration_VideosTable(t *testing.T) {
	// Verifica que a coluna project_id foi adicionada à tabela videos (T33, issue #6).
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer database.Close()

	var count int
	if err := database.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?",
		"idx_videos_project",
	).Scan(&count); err != nil {
		t.Fatalf("erro ao verificar índice: %v", err)
	}
	if count != 1 {
		t.Error("índice idx_videos_project não existe (coluna project_id não foi adicionada)")
	}
}

func TestSchema_ProjectIDMigration_TokensTable(t *testing.T) {
	// Verifica que a coluna project_id foi adicionada à tabela upload_tokens (T33, issue #6).
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer database.Close()

	var count int
	if err := database.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?",
		"idx_upload_tokens_project",
	).Scan(&count); err != nil {
		t.Fatalf("erro ao verificar índice: %v", err)
	}
	if count != 1 {
		t.Error("índice idx_upload_tokens_project não existe (coluna project_id não foi adicionada)")
	}
}

func TestSchema_VideosSlugUnique(t *testing.T) {
	// Verifica que projects.slug tem constraint UNIQUE.
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer database.Close()

	// Tenta inserir dois projetos com o mesmo slug
	_, err = database.Exec(
		"INSERT INTO projects (name, slug, root_dir, master_key_hash) VALUES (?, ?, ?, ?)",
		"Projeto 1", "projeto1", "/root1", "hash1",
	)
	if err != nil {
		t.Fatalf("primeiro INSERT em projects falhou: %v", err)
	}

	_, err = database.Exec(
		"INSERT INTO projects (name, slug, root_dir, master_key_hash) VALUES (?, ?, ?, ?)",
		"Projeto 2", "projeto1", "/root2", "hash2", // mesmo slug
	)
	if err == nil {
		t.Error("esperava erro de UNIQUE constraint para slug duplicado em projects")
	}
}
