package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // driver SQLite em Go puro, sem CGo
)

// Open abre (ou cria) o banco SQLite no caminho informado.
// Cria o diretório pai se necessário, ativa WAL mode, foreign keys,
// busy timeout e aplica o schema completo.
func Open(path string) (*sql.DB, error) {
	// Para banco em memória, pula a verificação de diretório
	if path != ":memory:" {
		// Exige que o diretório pai exista. Não criamos diretórios
		// arbitrários: abrir um banco em um caminho cujo diretório
		// não existe é um erro de configuração e deve falhar.
		dir := filepath.Dir(path)
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			return nil, fmt.Errorf("diretório do banco não existe: %s", dir)
		}
	}

	// Abre o banco com o driver modernc/sqlite
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("erro ao abrir banco SQLite: %w", err)
	}

	// SQLite não suporta múltiplas conexões de escrita simultâneas.
	// Limita a 1 conexão para evitar "database is locked".
	db.SetMaxOpenConns(1)

	// Ativa WAL mode para permitir leituras concorrentes durante escritas
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("erro ao ativar WAL mode: %w", err)
	}

	// Ativa validação de foreign keys (desabilitado por padrão no SQLite)
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("erro ao ativar foreign keys: %w", err)
	}

	// Configura timeout de espera quando banco está bloqueado (em ms)
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("erro ao configurar busy_timeout: %w", err)
	}

	// Aplica o schema completo (tabelas, índices, trigger de updated_at)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("erro ao aplicar schema: %w", err)
	}

	// Migração leve (T33, issue #6): associa vídeos e tokens de upload a um
	// projeto. CREATE TABLE IF NOT EXISTS não altera tabelas já existentes
	// em instalações antigas — por isso a coluna é adicionada à parte, de
	// forma idempotente, em vez de fazer parte do DDL de criação acima.
	if err := ensureColumn(db, "videos", "project_id", "project_id INTEGER REFERENCES projects(id)"); err != nil {
		db.Close()
		return nil, err
	}
	if err := ensureColumn(db, "upload_tokens", "project_id", "project_id INTEGER REFERENCES projects(id)"); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_videos_project ON videos(project_id)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("erro ao criar índice idx_videos_project: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_upload_tokens_project ON upload_tokens(project_id)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("erro ao criar índice idx_upload_tokens_project: %w", err)
	}

	return db, nil
}

// ensureColumn adiciona a coluna informada à tabela caso ela ainda não
// exista. Necessário porque "CREATE TABLE IF NOT EXISTS" não modifica
// tabelas já criadas — sem isso, instalações existentes nunca ganhariam a
// coluna nova (não há sistema de migrações versionadas neste projeto).
func ensureColumn(db *sql.DB, table, column, columnDDL string) error {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return fmt.Errorf("erro ao inspecionar colunas de %s: %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid, notNull, pk int
		var name, ctype string
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dfltValue, &pk); err != nil {
			return fmt.Errorf("erro ao ler metadados de %s: %w", table, err)
		}
		if name == column {
			return nil // coluna já existe — nada a fazer
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("erro ao iterar colunas de %s: %w", table, err)
	}

	if _, err := db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", table, columnDDL)); err != nil {
		return fmt.Errorf("erro ao adicionar coluna %s em %s: %w", column, table, err)
	}
	return nil
}
