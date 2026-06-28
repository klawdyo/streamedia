package models

import (
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// ConfigType representa o tipo de dado de uma configuração.
type ConfigType string

// Tipos suportados de configuração.
const (
	ConfigTypeString          ConfigType = "string"
	ConfigTypeNumber          ConfigType = "number"
	ConfigTypeBoolean         ConfigType = "boolean"
	ConfigTypeDurationSeconds ConfigType = "duration_seconds"
	ConfigTypeURL             ConfigType = "url"
	ConfigTypeSecret          ConfigType = "secret"
)

// Configuration representa uma entrada da tabela configurations.
type Configuration struct {
	Key         string
	Value       string
	Type        ConfigType
	Description string
	GroupKey    string
	Validation  string
	Visible     bool // true = visível no GET, false = write-only (secret)
	UpdatedAt   time.Time
}

// ConfigGroup agrupa configurações para exibição na UI.
type ConfigGroup struct {
	Key         string          `json:"key"`
	Title       string          `json:"title"`
	Description string          `json:"description,omitempty"`
	Items       []ConfigItem    `json:"items"`
}

// ConfigItem é uma configuração individual, pronta para a UI do frontend.
// Campos Visible=false nunca são serializados (omitidos do JSON).
type ConfigItem struct {
	Key         string `json:"key"`
	Value       string `json:"value,omitempty"`       // omitido se não visível
	Type        string `json:"type"`
	Description string `json:"description"`
	Validation  string `json:"validation,omitempty"`
	Default     string `json:"default,omitempty"`      // valor default do código Go
	Visible     bool   `json:"visible"`
}

// GroupTitles mapeia group_key → título legível e descrição para a UI.
var GroupTitles = map[string]struct{ Title, Description string }{
	"paths":       {"Caminhos", "Diretórios de armazenamento do sistema"},
	"session":     {"Sessão", "Configurações do cookie de sessão do navegador"},
	"upload":      {"Upload", "Limites e timeouts de upload de vídeo"},
	"transcode":   {"Transcodificação", "Workers, fila e políticas de retentativa"},
	"token":       {"Tokens", "TTL dos tokens efêmeros de upload e play"},
	"rate_limit":  {"Rate Limit", "Limite de requisições por minuto"},
	"webhook":     {"Webhook", "URL e segredo de assinatura de webhooks"},
	"discord":     {"Discord", "Alertas operacionais via webhook do Discord"},
}

// DefaultValues mapeia cada key de configuração para seu valor default no código.
// Usado como fallback quando a linha não existe no banco, e exibido na UI.
var DefaultValues = map[string]string{
	"paths.media_dir":          "/media",
	"paths.upload_tmp_dir":     "/media/.uploads",
	"session.ttl_seconds":      "43200",
	"upload.max_size_mb":       "10",
	"upload.idle_timeout":      "600",
	"transcode.workers":        "1",
	"transcode.queue_max":      "50",
	"transcode.stuck_timeout":  "1800",
	"transcode.max_attempts":   "3",
	"transcode.keep_original":  "false",
	"token.upload_ttl":         "1200",
	"token.play_ttl":           "3600",
	"rate_limit.per_minute":    "60",
	"webhook.url":              "",
	"webhook.secret":           "",
	"discord.webhook_url":      "",
}

// scanConfiguration lê uma linha de configurations para a struct.
func scanConfiguration(scan func(dest ...any) error) (*Configuration, error) {
	var c Configuration
	var updatedAt string
	var visible int
	if err := scan(&c.Key, &c.Value, &c.Type, &c.Description, &c.GroupKey, &c.Validation, &visible, &updatedAt); err != nil {
		return nil, err
	}
	c.Visible = visible == 1
	c.UpdatedAt = parseDateTime(updatedAt)
	return &c, nil
}

// GetConfiguration busca uma configuração pela key.
func GetConfiguration(db *sql.DB, key string) (*Configuration, error) {
	row := db.QueryRow(
		`SELECT key, value, type, description, group_key, validation, visible, updated_at FROM configurations WHERE key = ?`,
		key,
	)
	return scanConfiguration(row.Scan)
}

// GetAllConfigurations retorna todas as configurações do banco.
func GetAllConfigurations(db *sql.DB) ([]Configuration, error) {
	rows, err := db.Query(
		`SELECT key, value, type, description, group_key, validation, visible, updated_at FROM configurations ORDER BY group_key, key`,
	)
	if err != nil {
		return nil, fmt.Errorf("erro ao listar configurações: %w", err)
	}
	defer rows.Close()

	var configs []Configuration
	for rows.Next() {
		c, err := scanConfiguration(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("erro ao escanear configuração: %w", err)
		}
		configs = append(configs, *c)
	}
	return configs, rows.Err()
}

// UpsertConfiguration insere ou atualiza uma configuração.
func UpsertConfiguration(db *sql.DB, key, value string) error {
	_, err := db.Exec(
		`INSERT INTO configurations (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`,
		key, value,
	)
	return err
}

// DeleteConfiguration remove uma configuração do banco.
// Após deletar, o sistema usará o DefaultValues[key] como fallback.
func DeleteConfiguration(db *sql.DB, key string) error {
	_, err := db.Exec(`DELETE FROM configurations WHERE key = ?`, key)
	return err
}

// ValidateConfigValue valida o valor de uma configuração conforme seu tipo e
// regra de validação (regex). Retorna nil se válido, ou um erro descritivo.
func ValidateConfigValue(configType ConfigType, validation, value string) error {
	// Validação de regex (se houver)
	if validation != "" {
		matched, err := regexp.MatchString(validation, value)
		if err != nil {
			return fmt.Errorf("erro na regex de validação %q: %w", validation, err)
		}
		if !matched {
			return fmt.Errorf("valor %q não atende à validação %q", value, validation)
		}
	}

	// Validação por tipo
	switch configType {
	case ConfigTypeNumber:
		if _, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("valor %q não é um número inteiro válido", value)
		}
	case ConfigTypeBoolean:
		if value != "true" && value != "false" {
			return fmt.Errorf("valor %q não é um booleano válido (use true ou false)", value)
		}
	case ConfigTypeDurationSeconds:
		n, err := strconv.Atoi(value)
		if err != nil || n <= 0 {
			return fmt.Errorf("valor %q não é uma duração válida em segundos (inteiro positivo)", value)
		}
	case ConfigTypeURL:
		// Validação básica: se não vazio, deve conter "://"
		if value != "" {
			matched, _ := regexp.MatchString(`^https?://`, value)
			if !matched {
				return fmt.Errorf("valor %q não é uma URL HTTP(S) válida", value)
			}
		}
	case ConfigTypeSecret:
		// Secrets não têm validação específica além da regex (opcional)
		// São sempre write-only; o valor nunca é validado além do formato básico
	}

	return nil
}

// BuildConfigGroups monta a estrutura de grupos de configuração para a UI.
// Combina dados do banco com defaults do código. Configs com Visible=false
// têm Value omitido (string vazia) — o frontend as trata como write-only.
func BuildConfigGroups(db *sql.DB) ([]ConfigGroup, error) {
	dbConfigs, err := GetAllConfigurations(db)
	if err != nil {
		return nil, err
	}

	// Mapa key → config do banco
	dbMap := make(map[string]Configuration, len(dbConfigs))
	for _, c := range dbConfigs {
		dbMap[c.Key] = c
	}

	// Constrói uma entrada para cada default conhecido
	// (assim a UI sempre mostra todas as configs possíveis,
	// mesmo as que não têm linha no banco)
	groupsMap := make(map[string]*ConfigGroup)

	for key, defaultVal := range DefaultValues {
		// Determina group_key a partir da key (ex: "transcode.workers" → "transcode")
		groupKey := ""
		for i := len(key) - 1; i >= 0; i-- {
			if key[i] == '.' {
				groupKey = key[:i]
				break
			}
		}

		// Garante que o grupo existe
		if _, ok := groupsMap[groupKey]; !ok {
			title := groupKey
			desc := ""
			if info, ok := GroupTitles[groupKey]; ok {
				title = info.Title
				desc = info.Description
			}
			groupsMap[groupKey] = &ConfigGroup{
				Key:         groupKey,
				Title:       title,
				Description: desc,
			}
		}

		dbCfg, exists := dbMap[key]
		item := ConfigItem{
			Key:         key,
			Type:        "string",
			Description: "",
			Default:     defaultVal,
			Visible:     true,
		}

		if exists {
			item.Description = dbCfg.Description
			item.Validation = dbCfg.Validation
			item.Type = string(dbCfg.Type)
			item.Visible = dbCfg.Visible
			if dbCfg.Visible {
				item.Value = dbCfg.Value
			}
		}

		groupsMap[groupKey].Items = append(groupsMap[groupKey].Items, item)
	}

	// Converte mapa para slice ordenado
	groupOrder := []string{"paths", "session", "upload", "transcode", "token", "rate_limit", "webhook", "discord"}
	var groups []ConfigGroup
	for _, gk := range groupOrder {
		if g, ok := groupsMap[gk]; ok {
			groups = append(groups, *g)
		}
	}

	return groups, nil
}
