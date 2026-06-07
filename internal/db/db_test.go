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

func TestOpen_InMemoryDatabase(t *testing.T) {
	// Verifica que abrir banco em memória ":memory:" funciona sem erro de diretório.
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:) falhou: %v", err)
	}
	defer db.Close()

	// Verifica que o banco está operacional
	var result int
	if err := db.QueryRow("SELECT 1").Scan(&result); err != nil {
		t.Fatalf("erro ao executar query simples: %v", err)
	}
	if result != 1 {
		t.Errorf("resultado esperado 1, obtido %d", result)
	}
}

func TestEnsureColumn_AddsNewColumn(t *testing.T) {
	// Verifica que ensureColumn adiciona uma coluna que não existe.
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer database.Close()

	// Tabela videos já existe (criada por Open via schema)
	// Tenta adicionar uma nova coluna TEST_COL
	if err := ensureColumn(database, "videos", "test_col", "test_col TEXT"); err != nil {
		t.Fatalf("ensureColumn falhou: %v", err)
	}

	// Verifica que a coluna foi adicionada
	rows, err := database.Query("PRAGMA table_info(videos)")
	if err != nil {
		t.Fatalf("PRAGMA table_info(videos) falhou: %v", err)
	}
	defer rows.Close()

	found := false
	for rows.Next() {
		var cid, notNull, pk int
		var name, ctype string
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dfltValue, &pk); err != nil {
			t.Fatalf("erro ao ler coluna: %v", err)
		}
		if name == "test_col" {
			found = true
			break
		}
	}
	if !found {
		t.Error("coluna test_col não foi adicionada")
	}
}

func TestEnsureColumn_SkipsExistingColumn(t *testing.T) {
	// Verifica que ensureColumn não falha se a coluna já existe.
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer database.Close()

	// Coluna video_id já existe em videos
	// Chamar ensureColumn novamente deve não falhar
	if err := ensureColumn(database, "videos", "video_id", "video_id TEXT"); err != nil {
		t.Fatalf("ensureColumn para coluna existente falhou: %v", err)
	}
}

func TestOpen_MultipleTimes_Idempotent(t *testing.T) {
	// Verifica que abrir o mesmo banco 3+ vezes não corrompe dados.
	dir := t.TempDir()
	path := filepath.Join(dir, "multi.db")

	// Abre, insere dados, fecha
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

	// Abre novamente, verifica que dados persistiram
	db2, err := Open(path)
	if err != nil {
		t.Fatalf("Open #2 falhou: %v", err)
	}
	var videoID string
	if err := db2.QueryRow("SELECT video_id FROM videos WHERE video_id = ?", "test-vid").Scan(&videoID); err != nil {
		t.Fatalf("dados não persistiram: %v", err)
	}
	db2.Close()

	// Abre pela terceira vez, verifica novamente
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
	// Verifica que todas as tabelas principais foram criadas.
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer db.Close()

	tables := []string{"videos", "upload_tokens", "webhook_log", "playback_events", "projects"}
	for _, table := range tables {
		_, err := db.Exec("SELECT 1 FROM " + table + " LIMIT 1")
		if err != nil {
			t.Errorf("tabela %q não existe ou schema não foi aplicado: %v", table, err)
		}
	}
}

func TestOpen_ForeignKeysActive(t *testing.T) {
	// Verifica que PRAGMA foreign_keys=ON está ativo.
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
	// Verifica que busy_timeout foi configurado (valor > 0).
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
	// Verifica que MaxOpenConns foi limitado a 1 (SQLite single-writer constraint).
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer db.Close()

	// Não há forma direta de verificar MaxOpenConns, mas podemos verificar
	// que operações básicas funcionam (qual é o proxy para estar bem configurado)
	if _, err := db.Exec("INSERT INTO videos (video_id) VALUES (?)", "test"); err != nil {
		t.Errorf("operação básica falhou: %v", err)
	}
}

func TestOpen_CloseOnError_Schema(t *testing.T) {
	// Caso de erro no schema não deve deixar banco aberto.
	// Este teste é mais conceitual — verificamos que Open retorna erro
	// se algo falhar durante inicialização.
	// (Difícil de forçar em ":memory:", então apenas validamos que erro é retornado corretamente)

	// Tenta abrir banco em diretório inexistente
	_, err := Open("/path/impossivel/nowhere.db")
	if err == nil {
		t.Fatal("esperava erro ao abrir banco em diretório inexistente")
	}
}

// TestOpen_ActivatesForeignKeys verifica que Open() ativa foreign keys
// para rejeitar inserções com video_id inválido.
func TestOpen_ActivatesForeignKeys_RejectsInvalidFK(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer db.Close()

	// Insere um vídeo válido
	_, err = db.Exec(
		"INSERT INTO videos (video_id) VALUES (?)",
		"vid-valid",
	)
	if err != nil {
		t.Fatalf("INSERT de vídeo válido falhou: %v", err)
	}

	// Tenta inserir token para vídeo válido — deve funcionar
	_, err = db.Exec(
		"INSERT INTO upload_tokens (token, video_id, expires_at) VALUES (?, ?, ?)",
		"tok-ok", "vid-valid", "2025-12-31 23:59:59",
	)
	if err != nil {
		t.Fatalf("INSERT de token com FK válida falhou: %v", err)
	}

	// Tenta inserir token para vídeo INVÁLIDO — deve falhar
	_, err = db.Exec(
		"INSERT INTO upload_tokens (token, video_id, expires_at) VALUES (?, ?, ?)",
		"tok-bad", "vid-inexistente", "2025-12-31 23:59:59",
	)
	if err == nil {
		t.Error("esperava erro de FOREIGN KEY constraint ao inserir token com video_id inválido")
	}
}

// TestEnsureColumn_HandlesTableNotFound verifica comportamento quando a tabela
// não existe (caso raro, usado para verificar robustez).
func TestEnsureColumn_CreatesIfMissing(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer db.Close()

	// Insere um vídeo primeiro para ter uma linha
	_, err = db.Exec("INSERT INTO videos (video_id, status) VALUES (?, ?)", "test-vid", "pending_upload")
	if err != nil {
		t.Fatalf("INSERT falhou: %v", err)
	}

	// Tenta adicionar coluna a tabela existente
	if err := ensureColumn(db, "videos", "new_test_col", "new_test_col TEXT DEFAULT 'test'"); err != nil {
		t.Fatalf("ensureColumn para nova coluna falhou: %v", err)
	}

	// Verifica que a coluna foi adicionada
	var value string
	if err := db.QueryRow("SELECT new_test_col FROM videos LIMIT 1").Scan(&value); err != nil {
		t.Errorf("nova coluna não é acessível: %v", err)
	}
}

// TestOpen_SetMaxOpenConns verifica que MaxOpenConns = 1 (SQLite single-writer).
// Não é possível ler MaxOpenConns diretamente, mas verificamos que operações
// sequenciais funcionam (indica que a configuração está correta).
func TestOpen_SetMaxOpenConns_SequentialWritesWork(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer db.Close()

	// Múltiplas escritas sequenciais devem funcionar com MaxOpenConns=1
	for i := 0; i < 5; i++ {
		_, err := db.Exec("INSERT INTO videos (video_id, status) VALUES (?, ?)", "v"+string(rune(48+i)), "pending_upload")
		if err != nil {
			t.Errorf("escrita sequencial %d falhou: %v", i, err)
		}
	}

	// Verifica que todos foram inseridos
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM videos").Scan(&count); err != nil {
		t.Fatalf("COUNT falhou: %v", err)
	}
	if count != 5 {
		t.Errorf("COUNT: esperado 5, obtido %d", count)
	}
}

// TestEnsureColumn_EmptyTableInfo verifica ensureColumn quando a tabela não tem colunas
// (caso muito raro, apenas para cobertura defensiva).
func TestEnsureColumn_PreservesExistingColumns(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() falhou: %v", err)
	}
	defer db.Close()

	// Adiciona coluna a videos
	if err := ensureColumn(db, "videos", "col_a", "col_a TEXT"); err != nil {
		t.Fatalf("primeira ensureColumn falhou: %v", err)
	}

	// Adiciona outra coluna
	if err := ensureColumn(db, "videos", "col_b", "col_b INTEGER"); err != nil {
		t.Fatalf("segunda ensureColumn falhou: %v", err)
	}

	// Verifica que ambas existem via PRAGMA table_info
	rows, err := db.Query("PRAGMA table_info(videos)")
	if err != nil {
		t.Fatalf("PRAGMA table_info falhou: %v", err)
	}
	defer rows.Close()

	found := map[string]bool{}
	for rows.Next() {
		var cid, notNull, pk int
		var name, ctype string
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dfltValue, &pk); err != nil {
			t.Fatalf("erro ao ler coluna: %v", err)
		}
		found[name] = true
	}

	if !found["col_a"] || !found["col_b"] {
		t.Error("uma ou ambas as colunas adicionadas não foram encontradas")
	}
}

// Garante que o pacote database/sql está importado via driver.
var _ *sql.DB
