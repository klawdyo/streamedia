package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pressly/goose/v3"

	"github.com/klawdyo/streamedia/internal/db/migrations"

	_ "modernc.org/sqlite" // driver SQLite em Go puro, sem CGo
)

// Open abre (ou cria) o banco SQLite no caminho informado.
// Cria o diretório pai se necessário, ativa WAL mode, foreign keys,
// busy timeout e aplica as migrations pendentes via goose.
//
// As migrations rodam automaticamente a cada inicialização do servidor
// e são idempotentes: o goose mantém a tabela goose_db_version com o
// histórico do que já foi aplicado, então cada migration roda exatamente
// uma vez por banco.
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

	// Configura o provedor goose para SQLite3 (dialeto compatível com
	// modernc.org/sqlite) e aplica todas as migrations pendentes, em
	// ordem, usando os arquivos .sql embutidos no binário.
	//
	// Isso roda a cada inicialização do servidor e é seguro porque o
	// goose registra o que já foi aplicado (tabela goose_db_version):
	// migrations já executadas são puladas automaticamente.
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		db.Close()
		return nil, fmt.Errorf("erro ao configurar dialeto goose: %w", err)
	}
	if err := goose.Up(db, "."); err != nil {
		db.Close()
		return nil, fmt.Errorf("erro ao aplicar migrations: %w", err)
	}

	return db, nil
}
