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
	// RootToken é a ÚNICA credencial durável de gestão (env ROOT_TOKEN):
	// o backend principal a apresenta em Authorization: Bearer para criar
	// vídeos (upload-init), emitir URLs de play, consultar status, listar e
	// apagar. Sem vínculo com nenhum dado — pode ser trocada a qualquer
	// momento (basta mudar o env e reiniciar).
	RootToken            string
	WebhookURL           string
	WebhookSecret        string
	MaxUploadSizeBytes   int64         // convertido de MB para bytes
	MediaDir             string
	UploadTmpDir         string
	SQLitePath           string
	QueueMaxSize         int
	TranscodeWorkers     int
	UploadTokenTTL       time.Duration // env UPLOAD_TOKEN_TTL (segundos) — TTL do token de upload (vida curta, ~20min)
	PlayTokenTTL         time.Duration // env PLAY_TOKEN_TTL (segundos) — TTL do token de play emitido por /api/play/init
	UploadIdleTimeout    time.Duration // env UPLOAD_IDLE_TIMEOUT (segundos)
	TranscodeStuckTime   time.Duration // env TRANSCODE_STUCK (segundos)
	MaxTranscodeAttempts int
	KeepOriginal         bool
	Port                 int
	RateLimitPerMin      int
}

// Load lê a configuração das variáveis de ambiente, aplicando valores
// padrão para as opcionais e validando as obrigatórias.
func Load() (*Config, error) {
	// Variáveis obrigatórias.
	rootToken := os.Getenv("ROOT_TOKEN")
	if rootToken == "" {
		return nil, fmt.Errorf("variável de ambiente ROOT_TOKEN é obrigatória")
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
	// Variáveis de tempo, todas em SEGUNDOS (o nome não carrega o sufixo de
	// unidade; o significado está documentado aqui e no .env.example).
	// UPLOAD_TOKEN_TTL: TTL do token de upload (padrão 1200 = 20min).
	uploadTokenTTLSeconds, err := getEnvInt("UPLOAD_TOKEN_TTL", 1200)
	if err != nil {
		return nil, err
	}
	// PLAY_TOKEN_TTL: TTL do token de play emitido por /api/play/init (padrão 3600 = 1h).
	playTokenTTLSeconds, err := getEnvInt("PLAY_TOKEN_TTL", 3600)
	if err != nil {
		return nil, err
	}
	// UPLOAD_IDLE_TIMEOUT: tempo de inatividade até matar um upload (padrão 600 = 10min).
	uploadIdleTimeoutSeconds, err := getEnvInt("UPLOAD_IDLE_TIMEOUT", 600)
	if err != nil {
		return nil, err
	}
	// TRANSCODE_STUCK: tempo para considerar uma transcodificação travada (padrão 1800 = 30min).
	transcodeStuckSeconds, err := getEnvInt("TRANSCODE_STUCK", 1800)
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
		RootToken:            rootToken,
		WebhookURL:           webhookURL,
		WebhookSecret:        webhookSecret,
		MaxUploadSizeBytes:   int64(maxUploadSizeMB) * 1024 * 1024,
		MediaDir:             getEnvStr("MEDIA_DIR", "/media"),
		UploadTmpDir:         getEnvStr("UPLOAD_TMP_DIR", "/media/.uploads"),
		SQLitePath:           getEnvStr("SQLITE_PATH", "/data/media.db"),
		QueueMaxSize:         queueMaxSize,
		TranscodeWorkers:     transcodeWorkers,
		UploadTokenTTL:       time.Second * time.Duration(uploadTokenTTLSeconds),
		PlayTokenTTL:         time.Second * time.Duration(playTokenTTLSeconds),
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
