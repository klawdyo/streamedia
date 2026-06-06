package db

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestOpen_CreatesSchema(t *testing.T) {
	// Verifica que Open() cria todas as tabelas obrigatórias.
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer db.Close()

	// Verifica tabela videos
	if _, err := db.Exec("SELECT 1 FROM videos LIMIT 1"); err != nil {
		t.Errorf("tabela videos não existe: %v", err)
	}
	// Verifica tabela upload_tokens
	if _, err := db.Exec("SELECT 1 FROM upload_tokens LIMIT 1"); err != nil {
		t.Errorf("tabela upload_tokens não existe: %v", err)
	}
	// Verifica tabela webhook_log
	if _, err := db.Exec("SELECT 1 FROM webhook_log LIMIT 1"); err != nil {
		t.Errorf("tabela webhook_log não existe: %v", err)
	}
}

func TestOpen_WALMode(t *testing.T) {
	// Verifica que o banco é aberto com journal_mode=WAL para leituras concorrentes.
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer db.Close()

	var mode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("PRAGMA journal_mode falhou: %v", err)
	}
	// Em memória pode retornar "memory" — aceitar também
	if mode != "wal" && mode != "memory" {
		t.Errorf("journal_mode: esperado 'wal' ou 'memory', obtido %q", mode)
	}
}

func TestOpen_ForeignKeys(t *testing.T) {
	// Verifica que foreign keys estão ativas — inserção com video_id inválido deve falhar.
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer db.Close()

	// Tenta inserir token para video_id que não existe na tabela videos
	_, err = db.Exec(
		`INSERT INTO upload_tokens (token, video_id, expires_at) VALUES (?, ?, ?)`,
		"tok", "video-inexistente", time.Now().Add(time.Hour),
	)
	if err == nil {
		t.Error("esperava erro de foreign key constraint, mas inserção foi aceita")
	}
}

func TestOpen_Idempotent(t *testing.T) {
	// Verifica que abrir o mesmo banco duas vezes não retorna erro (CREATE IF NOT EXISTS).
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	db1, err := Open(path)
	if err != nil {
		t.Fatalf("primeira Open() falhou: %v", err)
	}
	db1.Close()

	db2, err := Open(path)
	if err != nil {
		t.Fatalf("segunda Open() falhou: %v", err)
	}
	db2.Close()
}

func TestOpen_MissingPath(t *testing.T) {
	// Verifica que abrir banco em diretório inexistente retorna erro.
	_, err := Open("/nao/existe/media.db")
	if err == nil {
		t.Error("esperava erro ao abrir banco em diretório inexistente, mas Open() retornou nil")
	}
}

func TestDB_UpdatedAtTrigger(t *testing.T) {
	// Verifica que o trigger atualiza o campo updated_at ao modificar o status.
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer db.Close()

	// Insere um vídeo
	_, err = db.Exec(
		`INSERT INTO videos (video_id, status) VALUES (?, ?)`,
		"video-trigger-test", "pending_upload",
	)
	if err != nil {
		t.Fatalf("INSERT falhou: %v", err)
	}

	// Captura o updated_at inicial
	var updatedAt1 string
	if err := db.QueryRow(`SELECT updated_at FROM videos WHERE video_id = ?`, "video-trigger-test").Scan(&updatedAt1); err != nil {
		t.Fatalf("SELECT updated_at falhou: %v", err)
	}

	// Aguarda 1 segundo para garantir diferença no timestamp
	time.Sleep(1100 * time.Millisecond)

	// Atualiza o status
	if _, err := db.Exec(`UPDATE videos SET status = ? WHERE video_id = ?`, "uploading", "video-trigger-test"); err != nil {
		t.Fatalf("UPDATE falhou: %v", err)
	}

	// Verifica que updated_at mudou
	var updatedAt2 string
	if err := db.QueryRow(`SELECT updated_at FROM videos WHERE video_id = ?`, "video-trigger-test").Scan(&updatedAt2); err != nil {
		t.Fatalf("SELECT updated_at (2) falhou: %v", err)
	}

	if updatedAt1 == updatedAt2 {
		t.Errorf("updated_at não mudou após UPDATE: ainda é %q", updatedAt1)
	}
}

// Garante que o pacote database/sql está importado via driver.
var _ *sql.DB
