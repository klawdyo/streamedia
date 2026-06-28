// Package dbconfig gerencia configurações dinâmicas lidas do banco SQLite,
// com fallback para defaults definidos no código (models.DefaultValues).
// NUNCA crasha se o banco estiver indisponível ou a linha não existir.
package dbconfig

import (
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/klawdyo/streamedia/internal/models"
)

// DBConfig gerencia configurações dinâmicas lidas do banco SQLite,
// com fallback para defaults definidos no código (models.DefaultValues).
type DBConfig struct {
	db *sql.DB
}

// New cria um novo gerenciador de configurações dinâmicas
// vinculado ao banco de dados informado.
func New(db *sql.DB) *DBConfig {
	return &DBConfig{db: db}
}

// GetString busca o valor da configuração no banco.
// Se o banco falhar ou a linha não existir, retorna defaultVal.
func (c *DBConfig) GetString(key string, defaultVal string) string {
	cfg, err := models.GetConfiguration(c.db, key)
	if err != nil {
		return defaultVal
	}
	return cfg.Value
}

// GetNumber busca a configuração no banco e converte com strconv.Atoi.
// Se o banco falhar, a linha não existir ou a conversão falhar,
// retorna defaultVal.
func (c *DBConfig) GetNumber(key string, defaultVal int) int {
	cfg, err := models.GetConfiguration(c.db, key)
	if err != nil {
		return defaultVal
	}
	n, err := strconv.Atoi(cfg.Value)
	if err != nil {
		return defaultVal
	}
	return n
}

// GetBool busca a configuração no banco e converte "true"/"false".
// Se o banco falhar, a linha não existir ou o valor não for "true"/"false",
// retorna defaultVal.
func (c *DBConfig) GetBool(key string, defaultVal bool) bool {
	cfg, err := models.GetConfiguration(c.db, key)
	if err != nil {
		return defaultVal
	}
	switch cfg.Value {
	case "true":
		return true
	case "false":
		return false
	default:
		return defaultVal
	}
}

// GetDurationSeconds busca a configuração no banco, converte para inteiro
// (segundos) e retorna como time.Duration. Se o banco falhar, a linha não
// existir ou a conversão falhar, retorna defaultVal convertido para segundos.
func (c *DBConfig) GetDurationSeconds(key string, defaultVal int) time.Duration {
	n := c.GetNumber(key, defaultVal)
	return time.Duration(n) * time.Second
}

// GetAll retorna todos os grupos de configuração montados para exibição na UI.
// Delega para models.BuildConfigGroups que combina dados do banco com
// defaults do código.
func (c *DBConfig) GetAll() ([]models.ConfigGroup, error) {
	return models.BuildConfigGroups(c.db)
}

// Set persiste (insere ou atualiza) uma configuração no banco.
// Antes de persistir, busca a configuração existente para obter seu type
// e validação (regex), então valida o novo valor com ValidateConfigValue.
// Se a chave não existir no banco, rejeita com erro.
func (c *DBConfig) Set(key, value string) error {
	// Busca a configuração existente para obter type e validação
	cfg, err := models.GetConfiguration(c.db, key)
	if err != nil {
		return fmt.Errorf("configuração %q não encontrada no banco: %w", key, err)
	}

	// Valida o novo valor conforme o type e a regra de validação
	if err := models.ValidateConfigValue(cfg.Type, cfg.Validation, value); err != nil {
		return fmt.Errorf("valor inválido para %q: %w", key, err)
	}

	// Persiste
	return models.UpsertConfiguration(c.db, key, value)
}

// Delete remove uma configuração do banco.
// Após deletar, os métodos Get* passarão a usar os valores default informados.
func (c *DBConfig) Delete(key string) error {
	return models.DeleteConfiguration(c.db, key)
}
