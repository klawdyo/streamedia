// Package config carrega e valida a configuração da aplicação a partir
// de variáveis de ambiente.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config agrega todas as configurações da aplicação.
type Config struct {
	UploadTokenSecret    string
	WebhookURL           string
	WebhookSecret        string
	AdminToken           string
	MaxUploadSizeBytes   int64         // convertido de MB para bytes
	MediaDir             string
	UploadTmpDir         string
	SQLitePath           string
	QueueMaxSize         int
	TranscodeWorkers     int
	UploadTokenTTL       time.Duration // de segundos (UPLOAD_TOKEN_TTL_SECONDS)
	PlayTokenMaxTTL      time.Duration // de segundos (PLAY_TOKEN_MAX_TTL_SECONDS)
	UploadIdleTimeout    time.Duration // de segundos (UPLOAD_IDLE_TIMEOUT_SECONDS)
	TranscodeStuckTime   time.Duration // de segundos (TRANSCODE_STUCK_SECONDS)
	MaxTranscodeAttempts int
	KeepOriginal         bool
	Port                 int
	RateLimitPerMin      int
}

// Load lê a configuração das variáveis de ambiente, aplicando valores
// padrão para as opcionais e validando as obrigatórias.
func Load() (*Config, error) {
	// Variáveis obrigatórias.
	uploadTokenSecret := os.Getenv("UPLOAD_TOKEN_SECRET")
	if uploadTokenSecret == "" {
		return nil, fmt.Errorf("variável de ambiente UPLOAD_TOKEN_SECRET é obrigatória")
	}
	webhookURL := os.Getenv("WEBHOOK_URL")
	if webhookURL == "" {
		return nil, fmt.Errorf("variável de ambiente WEBHOOK_URL é obrigatória")
	}
	webhookSecret := os.Getenv("WEBHOOK_SECRET")
	if webhookSecret == "" {
		return nil, fmt.Errorf("variável de ambiente WEBHOOK_SECRET é obrigatória")
	}

	// Variáveis inteiras opcionais.
	maxUploadSizeMB, err := getEnvInt("MAX_UPLOAD_SIZE_MB", 10)
	if err != nil {
		return nil, err
	}
	queueMaxSize, err := getEnvInt("QUEUE_MAX_SIZE", 50)
	if err != nil {
		return nil, err
	}
	transcodeWorkers, err := getEnvInt("TRANSCODE_WORKERS", 1)
	if err != nil {
		return nil, err
	}
	// Padronização em segundos (issue #4): todas as variáveis de tempo usam
	// o sufixo _SECONDS, eliminando a mistura de unidades (horas e minutos)
	// que existia antes (UPLOAD_TOKEN_TTL_H, UPLOAD_IDLE_TIMEOUT_MIN, etc.).
	// Defaults equivalentes aos valores anteriores: 6h = 21600s, 10min = 600s,
	// 30min = 1800s.
	uploadTokenTTLSeconds, err := getEnvInt("UPLOAD_TOKEN_TTL_SECONDS", 21600)
	if err != nil {
		return nil, err
	}
	playTokenMaxTTLSeconds, err := getEnvInt("PLAY_TOKEN_MAX_TTL_SECONDS", 21600)
	if err != nil {
		return nil, err
	}
	uploadIdleTimeoutSeconds, err := getEnvInt("UPLOAD_IDLE_TIMEOUT_SECONDS", 600)
	if err != nil {
		return nil, err
	}
	transcodeStuckSeconds, err := getEnvInt("TRANSCODE_STUCK_SECONDS", 1800)
	if err != nil {
		return nil, err
	}
	maxTranscodeAttempts, err := getEnvInt("MAX_TRANSCODE_ATTEMPTS", 3)
	if err != nil {
		return nil, err
	}
	port, err := getEnvInt("PORT", 3000)
	if err != nil {
		return nil, err
	}
	rateLimitPerMin, err := getEnvInt("RATE_LIMIT_PER_MIN", 60)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		UploadTokenSecret:    uploadTokenSecret,
		WebhookURL:           webhookURL,
		WebhookSecret:        webhookSecret,
		AdminToken:           getEnvStr("ADMIN_TOKEN", ""),
		MaxUploadSizeBytes:   int64(maxUploadSizeMB) * 1024 * 1024,
		MediaDir:             getEnvStr("MEDIA_DIR", "/media"),
		UploadTmpDir:         getEnvStr("UPLOAD_TMP_DIR", "/media/.uploads"),
		SQLitePath:           getEnvStr("SQLITE_PATH", "/data/media.db"),
		QueueMaxSize:         queueMaxSize,
		TranscodeWorkers:     transcodeWorkers,
		UploadTokenTTL:       time.Second * time.Duration(uploadTokenTTLSeconds),
		PlayTokenMaxTTL:      time.Second * time.Duration(playTokenMaxTTLSeconds),
		UploadIdleTimeout:    time.Second * time.Duration(uploadIdleTimeoutSeconds),
		TranscodeStuckTime:   time.Second * time.Duration(transcodeStuckSeconds),
		MaxTranscodeAttempts: maxTranscodeAttempts,
		KeepOriginal:         getEnvBool("KEEP_ORIGINAL", false),
		Port:                 port,
		RateLimitPerMin:      rateLimitPerMin,
	}

	return cfg, nil
}

// getEnvStr retorna o valor da variável de ambiente se definido e não
// vazio; caso contrário retorna o valor padrão.
func getEnvStr(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// getEnvInt retorna o valor inteiro da variável de ambiente. Se a
// variável estiver vazia, usa o padrão; se estiver definida mas não for
// um inteiro válido, retorna um erro descritivo.
func getEnvInt(key string, defaultVal int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("variável de ambiente %s deve ser um número inteiro válido, obtido %q", key, v)
	}
	return n, nil
}

// getEnvBool interpreta a variável de ambiente como booleano. Aceita
// "true"/"1" como verdadeiro e "false"/"0" como falso; vazio ou valor
// desconhecido usa o padrão.
func getEnvBool(key string, defaultVal bool) bool {
	switch os.Getenv(key) {
	case "true", "1":
		return true
	case "false", "0":
		return false
	default:
		return defaultVal
	}
}
