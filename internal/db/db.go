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

	return db, nil
}
