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
	RootToken     string
	WebhookURL    string
	WebhookSecret string
	// DiscordWebhookURL (env DISCORD_WEBHOOK_URL) é o webhook do Discord para
	// alertas operacionais internos (issue #21). Opcional — vazio desabilita o
	// canal (nenhum envio é tentado).
	DiscordWebhookURL    string
	MaxUploadSizeBytes   int64 // convertido de MB para bytes
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
	Environment          string // ambiente de execução (ENV): "production", "development", etc. Exposto em GET /api.
	// SessionTTL (env SESSION_TTL, segundos) é a validade do cookie de sessão
	// de navegador (streamedia_session), emitido por POST /admin/session.
	// Padrão 43200 (12h).
	SessionTTL time.Duration
	// SessionCookieSecure (env SESSION_COOKIE_SECURE) define o atributo
	// Secure do cookie de sessão. Padrão: true quando Environment !=
	// "development" (produção atrás de HTTPS); false em desenvolvimento
	// local, onde um cookie Secure nunca voltaria ao servidor por HTTP puro.
	SessionCookieSecure bool
}

// Load lê a configuração das variáveis de ambiente, aplicando valores
// padrão para as opcionais e validando as obrigatórias.
func Load() (*Config, error) {
	// Variáveis obrigatórias.
	rootToken := os.Getenv("ROOT_TOKEN")
	if rootToken == "" {
		return nil, fmt.Errorf("variável de ambiente ROOT_TOKEN é obrigatória")
	}
	// WEBHOOK_URL é opcional: sem ela, nenhum webhook é enviado (mas o stream
	// de eventos via SSE em /api/events continua funcionando normalmente).
	// Quando definida, o segredo de assinatura HMAC passa a ser obrigatório.
	webhookURL := os.Getenv("WEBHOOK_URL")
	webhookSecret := os.Getenv("WEBHOOK_SECRET")
	if webhookURL != "" && webhookSecret == "" {
		return nil, fmt.Errorf("WEBHOOK_SECRET é obrigatório quando WEBHOOK_URL está definida")
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
	// SESSION_TTL: validade do cookie de sessão de navegador (padrão 43200 = 12h).
	sessionTTLSeconds, err := getEnvInt("SESSION_TTL", 43200)
	if err != nil {
		return nil, err
	}

	environment := getEnvStr("ENV", "development")
	// SESSION_COOKIE_SECURE: por padrão, exige HTTPS (Secure) fora de
	// desenvolvimento. Pode ser sobrescrito explicitamente via env.
	sessionCookieSecure := getEnvBool("SESSION_COOKIE_SECURE", environment != "development")

	cfg := &Config{
		RootToken:            rootToken,
		WebhookURL:           webhookURL,
		WebhookSecret:        webhookSecret,
		DiscordWebhookURL:    os.Getenv("DISCORD_WEBHOOK_URL"),
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
		// ENV identifica o ambiente de execução, exposto em GET /api. Default
		// "development": se a variável não estiver setada, assumimos o ambiente
		// mais conservador (não declarar "production" por engano). Em produção
		// o operador deve definir ENV=production explicitamente.
		Environment:         environment,
		SessionTTL:          time.Second * time.Duration(sessionTTLSeconds),
		SessionCookieSecure: sessionCookieSecure,
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
