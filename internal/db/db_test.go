package db

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/klawdyo/streamedia/internal/db/migrations"
)

func TestOpen_RunsMigrations(t *testing.T) {
	// Verifica que Open() cria todas as tabelas e a tabela de controle do goose.
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer db.Close()

	// Verifica tabelas principais
	tables := []string{"videos", "upload_tokens", "webhook_log", "playback_events", "projects", "video_renditions"}
	for _, table := range tables {
		if _, err := db.Exec("SELECT 1 FROM " + table + " LIMIT 1"); err != nil {
			t.Errorf("tabela %q não existe: %v", table, err)
		}
	}

	// Verifica que a tabela de controle do goose existe e tem ao menos um registro
	var versionCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM goose_db_version").Scan(&versionCount); err != nil {
		t.Errorf("tabela goose_db_version não existe ou não é acessível: %v", err)
	}
	if versionCount < 1 {
		t.Errorf("esperava ao menos 1 migration aplicada, obtive %d", versionCount)
	}
}

func TestOpen_MigrationsAreIdempotent(t *testing.T) {
	// Verifica que reabrir o banco não reaplica migrations já executadas.
	dir := t.TempDir()
	path := filepath.Join(dir, "idempotent.db")

	db1, err := Open(path)
	if err != nil {
		t.Fatalf("primeira Open() falhou: %v", err)
	}

	// Conta quantas migrations foram aplicadas na primeira abertura
	var count1 int
	if err := db1.QueryRow("SELECT COUNT(*) FROM goose_db_version").Scan(&count1); err != nil {
		db1.Close()
		t.Fatalf("erro ao contar migrations: %v", err)
	}
	db1.Close()

	// Segunda abertura — não deve aplicar migrations novas
	db2, err := Open(path)
	if err != nil {
		t.Fatalf("segunda Open() falhou: %v", err)
	}

	var count2 int
	if err := db2.QueryRow("SELECT COUNT(*) FROM goose_db_version").Scan(&count2); err != nil {
		db2.Close()
		t.Fatalf("erro ao contar migrations (2): %v", err)
	}
	db2.Close()

	if count2 != count1 {
		t.Errorf("migrations duplicadas: primeira abertura %d, segunda %d", count1, count2)
	}
}

func TestOpen_WALMode(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer db.Close()

	var mode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("PRAGMA journal_mode falhou: %v", err)
	}
	if mode != "wal" && mode != "memory" {
		t.Errorf("journal_mode: esperado 'wal' ou 'memory', obtido %q", mode)
	}
}

func TestOpen_ForeignKeys(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer db.Close()

	// Tenta inserir token para video_id que não existe
	_, err = db.Exec(
		`INSERT INTO upload_tokens (token, video_id, expires_at) VALUES (?, ?, ?)`,
		"tok", "video-inexistente", time.Now().Add(time.Hour),
	)
	if err == nil {
		t.Error("esperava erro de foreign key constraint, mas inserção foi aceita")
	}
}

func TestOpen_Idempotent(t *testing.T) {
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

func TestDB_UpdatedAtTrigger(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(
		`INSERT INTO videos (video_id, status) VALUES (?, ?)`,
		"video-trigger-test", "pending_upload",
	)
	if err != nil {
		t.Fatalf("INSERT falhou: %v", err)
	}

	var updatedAt1 string
	if err := db.QueryRow(`SELECT updated_at FROM videos WHERE video_id = ?`, "video-trigger-test").Scan(&updatedAt1); err != nil {
		t.Fatalf("SELECT updated_at falhou: %v", err)
	}

	time.Sleep(1100 * time.Millisecond)

	if _, err := db.Exec(`UPDATE videos SET status = ? WHERE video_id = ?`, "uploading", "video-trigger-test"); err != nil {
		t.Fatalf("UPDATE falhou: %v", err)
	}

	var updatedAt2 string
	if err := db.QueryRow(`SELECT updated_at FROM videos WHERE video_id = ?`, "video-trigger-test").Scan(&updatedAt2); err != nil {
		t.Fatalf("SELECT updated_at (2) falhou: %v", err)
	}

	if updatedAt1 == updatedAt2 {
		t.Errorf("updated_at não mudou após UPDATE: ainda é %q", updatedAt1)
	}
}

func TestOpen_InMemoryDatabase(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:) falhou: %v", err)
	}
	defer db.Close()

	var result int
	if err := db.QueryRow("SELECT 1").Scan(&result); err != nil {
		t.Fatalf("erro ao executar query simples: %v", err)
	}
	if result != 1 {
		t.Errorf("resultado esperado 1, obtido %d", result)
	}
}

func TestOpen_MultipleTimes_Idempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "multi.db")

	db1, err := Open(path)
	if err != nil {
		t.Fatalf("Open #1 falhou: %v", err)
	}
	if _, err := db1.Exec(
		"INSERT INTO videos (video_id, status) VALUES (?, ?)",
		"test-vid", "pending_upload",
	); err != nil {
		t.Fatalf("INSERT em db1 falhou: %v", err)
	}
	db1.Close()

	db2, err := Open(path)
	if err != nil {
		t.Fatalf("Open #2 falhou: %v", err)
	}
	var videoID string
	if err := db2.QueryRow("SELECT video_id FROM videos WHERE video_id = ?", "test-vid").Scan(&videoID); err != nil {
		t.Fatalf("dados não persistiram: %v", err)
	}
	db2.Close()

	db3, err := Open(path)
	if err != nil {
		t.Fatalf("Open #3 falhou: %v", err)
	}
	if err := db3.QueryRow("SELECT video_id FROM videos WHERE video_id = ?", "test-vid").Scan(&videoID); err != nil {
		t.Fatalf("dados não persistiram na terceira abertura: %v", err)
	}
	db3.Close()
}

func TestOpen_SchemaApplied(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer db.Close()

	tables := []string{"videos", "upload_tokens", "webhook_log", "playback_events", "projects", "video_renditions"}
	for _, table := range tables {
		_, err := db.Exec("SELECT 1 FROM " + table + " LIMIT 1")
		if err != nil {
			t.Errorf("tabela %q não existe ou schema não foi aplicado: %v", table, err)
		}
	}
}

func TestOpen_ForeignKeysActive(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer db.Close()

	var enabled int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&enabled); err != nil {
		t.Fatalf("PRAGMA foreign_keys falhou: %v", err)
	}
	if enabled != 1 {
		t.Error("foreign_keys não está ativo (esperado 1)")
	}
}

func TestOpen_BusyTimeout(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer db.Close()

	var timeout int
	if err := db.QueryRow("PRAGMA busy_timeout").Scan(&timeout); err != nil {
		t.Fatalf("PRAGMA busy_timeout falhou: %v", err)
	}
	if timeout <= 0 {
		t.Errorf("busy_timeout não foi configurado (esperado > 0, obtido %d)", timeout)
	}
}

func TestOpen_MaxOpenConns(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec("INSERT INTO videos (video_id) VALUES (?)", "test"); err != nil {
		t.Errorf("operação básica falhou: %v", err)
	}
}

func TestOpen_ActivatesForeignKeys_RejectsInvalidFK(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(
		"INSERT INTO videos (video_id) VALUES (?)",
		"vid-valid",
	)
	if err != nil {
		t.Fatalf("INSERT de vídeo válido falhou: %v", err)
	}

	_, err = db.Exec(
		"INSERT INTO upload_tokens (token, video_id, expires_at) VALUES (?, ?, ?)",
		"tok-ok", "vid-valid", "2025-12-31 23:59:59",
	)
	if err != nil {
		t.Fatalf("INSERT de token com FK válida falhou: %v", err)
	}

	_, err = db.Exec(
		"INSERT INTO upload_tokens (token, video_id, expires_at) VALUES (?, ?, ?)",
		"tok-bad", "vid-inexistente", "2025-12-31 23:59:59",
	)
	if err == nil {
		t.Error("esperava erro de FOREIGN KEY constraint ao inserir token com video_id inválido")
	}
}

func TestOpen_SetMaxOpenConns_SequentialWritesWork(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer db.Close()

	for i := 0; i < 5; i++ {
		_, err := db.Exec("INSERT INTO videos (video_id, status) VALUES (?, ?)", "v"+string(rune(48+i)), "pending_upload")
		if err != nil {
			t.Errorf("escrita sequencial %d falhou: %v", i, err)
		}
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM videos").Scan(&count); err != nil {
		t.Fatalf("COUNT falhou: %v", err)
	}
	if count != 5 {
		t.Errorf("COUNT: esperado 5, obtido %d", count)
	}
}

func TestMigrations_EmbeddedFilesPresent(t *testing.T) {
	// Verifica que ao menos um arquivo .sql está embutido via go:embed.
	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		t.Fatalf("erro ao ler migrations embutidas: %v", err)
	}
	if len(entries) == 0 {
		t.Error("nenhum arquivo .sql embutido em migrations.FS")
	}

	hasSQL := false
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".sql" {
			hasSQL = true
			break
		}
	}
	if !hasSQL {
		t.Error("nenhum arquivo .sql encontrado nas migrations embutidas")
	}
}

// Garante que o pacote database/sql está importado via driver.
var _ *sql.DB
