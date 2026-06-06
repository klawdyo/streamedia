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
	MaxUploadSizeBytes   int64         // convertido de MB para bytes
	MediaDir             string
	UploadTmpDir         string
	SQLitePath           string
	QueueMaxSize         int
	TranscodeWorkers     int
	UploadTokenTTL       time.Duration // de horas
	PlayTokenMaxTTL      time.Duration // de horas
	UploadIdleTimeout    time.Duration // de minutos
	TranscodeStuckTime   time.Duration // de minutos
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
	uploadTokenTTLH, err := getEnvInt("UPLOAD_TOKEN_TTL_H", 6)
	if err != nil {
		return nil, err
	}
	playTokenMaxTTLH, err := getEnvInt("PLAY_TOKEN_MAX_TTL_H", 6)
	if err != nil {
		return nil, err
	}
	uploadIdleTimeoutMin, err := getEnvInt("UPLOAD_IDLE_TIMEOUT_MIN", 10)
	if err != nil {
		return nil, err
	}
	transcodeStuckMin, err := getEnvInt("TRANSCODE_STUCK_MIN", 30)
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
		MaxUploadSizeBytes:   int64(maxUploadSizeMB) * 1024 * 1024,
		MediaDir:             getEnvStr("MEDIA_DIR", "/media"),
		UploadTmpDir:         getEnvStr("UPLOAD_TMP_DIR", "/media/.uploads"),
		SQLitePath:           getEnvStr("SQLITE_PATH", "/data/media.db"),
		QueueMaxSize:         queueMaxSize,
		TranscodeWorkers:     transcodeWorkers,
		UploadTokenTTL:       time.Hour * time.Duration(uploadTokenTTLH),
		PlayTokenMaxTTL:      time.Hour * time.Duration(playTokenMaxTTLH),
		UploadIdleTimeout:    time.Minute * time.Duration(uploadIdleTimeoutMin),
		TranscodeStuckTime:   time.Minute * time.Duration(transcodeStuckMin),
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
