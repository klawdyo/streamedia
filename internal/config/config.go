// Package config carrega e valida a configuração da aplicação a partir
// de variáveis de ambiente (apenas as essenciais) e do banco de dados
// (configurações operacionais que podem ser alteradas via painel admin).
package config

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/klawdyo/streamedia/internal/config/dbconfig"
)

// Config agrega todas as configurações da aplicação. Os campos marcados
// como "env" são lidos de variáveis de ambiente no Load(); os marcados
// como "db" recebem defaults do código e são sobrescritos pelo banco
// via ApplyFromDB() após db.Open().
type Config struct {
	// --- Env: credenciais e segredos (8 variáveis no .env.example) ---

	// RootToken é a ÚNICA credencial durável de gestão (env ROOT_TOKEN):
	// o backend principal a apresenta em Authorization: Bearer para criar
	// vídeos (upload-init), emitir URLs de play, consultar status, listar e
	// apagar. Sem vínculo com nenhum dado — pode ser trocada a qualquer
	// momento (basta mudar o env e reiniciar).
	RootToken string // env ROOT_TOKEN — obrigatório

	// WebhookSecret (env WEBHOOK_SECRET) é o segredo compartilhado de
	// assinatura HMAC dos webhooks enviados ao backend principal.
	// Opcional — se vazio, webhooks são enviados sem assinatura.
	WebhookSecret string // env WEBHOOK_SECRET — opcional

	// GoogleClientID (env GOOGLE_CLIENT_ID) é o client_id da aplicação
	// registrada no Google Cloud Console para OAuth 2.0.
	GoogleClientID string // env GOOGLE_CLIENT_ID — opcional

	// GoogleClientSecret (env GOOGLE_CLIENT_SECRET) é o segredo do cliente
	// OAuth 2.0 — nunca exposto ao frontend.
	GoogleClientSecret string // env GOOGLE_CLIENT_SECRET — opcional

	// GoogleRedirectURL (env GOOGLE_REDIRECT_URL) é a URL de callback
	// registrada no Google Cloud Console.
	GoogleRedirectURL string // env GOOGLE_REDIRECT_URL — opcional

	// --- Env: infraestrutura ---

	// SQLitePath (env SQLITE_PATH) é o caminho do arquivo do banco.
	SQLitePath string // env SQLITE_PATH, padrão /data/media.db

	// Port (env PORT) é a porta HTTP do servidor.
	Port int // env PORT, padrão 3000

	// Environment (env ENV) é o ambiente de execução: "production",
	// "development", etc. Exposto em GET /api.
	Environment string // env ENV, padrão "development"

	// SessionCookieSecure (env SESSION_COOKIE_SECURE) define o atributo
	// Secure do cookie de sessão. Padrão: true quando ENV != "development".
	SessionCookieSecure bool // env SESSION_COOKIE_SECURE — opcional

	// SPADir (env SPA_DIR) é o caminho para o diretório de build da SPA
	// Vue.js. Em produção, o Dockerfile sobrescreve via ENV.
	// Default: ./web/dist (desenvolvimento local).
	SPADir string // env SPA_DIR, padrão ./web/dist

	// --- Caminhos hardcoded (não vêm de env nem de banco) ---

	// MediaDir é o diretório raiz dos arquivos HLS transcodificados.
	MediaDir string // hardcoded: /media

	// UploadTmpDir é o diretório temporário de uploads TUS.
	UploadTmpDir string // hardcoded: /media/.uploads

	// --- Configurações operacionais (defaults do código, sobrescritas pelo banco via ApplyFromDB) ---

	// WebhookURL é a URL global de webhook. Vazia = desabilitado.
	// Pode ser sobrescrita por vídeo via campo webhook_url no upload-init.
	// Banco: webhook.url (default "")
	WebhookURL string

	// DiscordWebhookURL é o webhook do Discord para alertas operacionais.
	// Vazia = canal desabilitado.
	// Banco: discord.webhook_url (default "")
	DiscordWebhookURL string

	// MaxUploadSizeBytes é o tamanho máximo de upload em bytes.
	// Banco: upload.max_size_mb (default 10)
	MaxUploadSizeBytes int64

	// QueueMaxSize é o tamanho máximo da fila de transcodificação.
	// Banco: transcode.queue_max (default 50)
	QueueMaxSize int

	// TranscodeWorkers é o número de workers paralelos de transcodificação.
	// Banco: transcode.workers (default 1)
	TranscodeWorkers int

	// UploadTokenTTL é o TTL do token de upload.
	// Banco: token.upload_ttl (default 1200s = 20min)
	UploadTokenTTL time.Duration

	// PlayTokenTTL é o TTL do token de play emitido por /api/play/init.
	// Banco: token.play_ttl (default 3600s = 1h)
	PlayTokenTTL time.Duration

	// UploadIdleTimeout é o tempo de inatividade até matar um upload.
	// Banco: upload.idle_timeout (default 600s = 10min)
	UploadIdleTimeout time.Duration

	// TranscodeStuckTime é o tempo para considerar uma transcode travada.
	// Banco: transcode.stuck_timeout (default 1800s = 30min)
	TranscodeStuckTime time.Duration

	// MaxTranscodeAttempts é o número máximo de tentativas de transcode.
	// Banco: transcode.max_attempts (default 3)
	MaxTranscodeAttempts int

	// KeepOriginal mantém o arquivo original após transcode se true.
	// Banco: transcode.keep_original (default false)
	KeepOriginal bool

	// RateLimitPerMin é o limite de requisições por minuto por IP.
	// Banco: rate_limit.per_minute (default 60)
	RateLimitPerMin int

	// SessionTTL é a validade do cookie de sessão de navegador.
	// Banco: session.ttl_seconds (default 43200 = 12h)
	SessionTTL time.Duration
}

// Load lê apenas as variáveis de ambiente essenciais (credenciais,
// segredos e infraestrutura). As configurações operacionais recebem
// defaults do código e serão sobrescritas pelo banco via ApplyFromDB().
func Load() (*Config, error) {
	// --- Env obrigatória ---
	rootToken := os.Getenv("ROOT_TOKEN")
	if rootToken == "" {
		return nil, fmt.Errorf("variável de ambiente ROOT_TOKEN é obrigatória")
	}

	// --- Env opcionais: segredos e credenciais ---
	webhookSecret := os.Getenv("WEBHOOK_SECRET")
	googleClientID := os.Getenv("GOOGLE_CLIENT_ID")
	googleClientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	googleRedirectURL := os.Getenv("GOOGLE_REDIRECT_URL")

	// --- Env opcionais: infraestrutura ---
	sqlitePath := getEnvStr("SQLITE_PATH", "/data/media.db")
	environment := getEnvStr("ENV", "development")
	sessionCookieSecure := getEnvBool("SESSION_COOKIE_SECURE", environment != "development")
	spaDir := getEnvStr("SPA_DIR", "./web/dist")

	port, err := getEnvInt("PORT", 3000)
	if err != nil {
		return nil, err
	}

	// --- Configurações operacionais: defaults do código ---
	// Serão sobrescritas pelo banco via ApplyFromDB() após db.Open().
	cfg := &Config{
		// Env
		RootToken:           rootToken,
		WebhookSecret:       webhookSecret,
		GoogleClientID:      googleClientID,
		GoogleClientSecret:  googleClientSecret,
		GoogleRedirectURL:   googleRedirectURL,
		SQLitePath:          sqlitePath,
		Port:                port,
		Environment:         environment,
		SessionCookieSecure: sessionCookieSecure,
		SPADir:              spaDir,

		// Hardcoded
		MediaDir:     "/media",
		UploadTmpDir: "/media/.uploads",

		// DB-bound (defaults do código)
		WebhookURL:          "",   // desabilitado por padrão
		DiscordWebhookURL:   "",   // desabilitado por padrão
		MaxUploadSizeBytes:  10 * 1024 * 1024, // 10 MB
		QueueMaxSize:        50,
		TranscodeWorkers:    1,
		UploadTokenTTL:      20 * time.Minute,
		PlayTokenTTL:        1 * time.Hour,
		UploadIdleTimeout:   10 * time.Minute,
		TranscodeStuckTime:  30 * time.Minute,
		MaxTranscodeAttempts: 3,
		KeepOriginal:        false,
		RateLimitPerMin:     60,
		SessionTTL:          12 * time.Hour,
	}

	return cfg, nil
}

// ApplyFromDB sobrescreve as configurações operacionais com os valores
// lidos da tabela configurations do banco. As chaves que não existirem
// no banco mantêm os defaults definidos em Load().
// Deve ser chamada após db.Open() e antes da criação da fila, workers,
// router etc.
func (c *Config) ApplyFromDB(db *sql.DB) {
	dbc := dbconfig.New(db)

	c.WebhookURL = dbc.GetString("webhook.url", c.WebhookURL)
	c.DiscordWebhookURL = dbc.GetString("discord.webhook_url", c.DiscordWebhookURL)
	c.MaxUploadSizeBytes = int64(max(dbc.GetNumber("upload.max_size_mb", int(c.MaxUploadSizeBytes/1024/1024)), 1)) * 1024 * 1024
	c.QueueMaxSize = max(dbc.GetNumber("transcode.queue_max", c.QueueMaxSize), 1)
	c.TranscodeWorkers = max(dbc.GetNumber("transcode.workers", c.TranscodeWorkers), 1)
	c.UploadTokenTTL = dbc.GetDurationSeconds("token.upload_ttl", int(c.UploadTokenTTL.Seconds()))
	c.PlayTokenTTL = dbc.GetDurationSeconds("token.play_ttl", int(c.PlayTokenTTL.Seconds()))
	c.UploadIdleTimeout = dbc.GetDurationSeconds("upload.idle_timeout", int(c.UploadIdleTimeout.Seconds()))
	c.TranscodeStuckTime = dbc.GetDurationSeconds("transcode.stuck_timeout", int(c.TranscodeStuckTime.Seconds()))
	c.MaxTranscodeAttempts = max(dbc.GetNumber("transcode.max_attempts", c.MaxTranscodeAttempts), 1)
	c.KeepOriginal = dbc.GetBool("transcode.keep_original", c.KeepOriginal)
	c.RateLimitPerMin = max(dbc.GetNumber("rate_limit.per_minute", c.RateLimitPerMin), 1)
	c.SessionTTL = dbc.GetDurationSeconds("session.ttl_seconds", int(c.SessionTTL.Seconds()))

	log.Printf("config: %d valores carregados do banco", 13)
}

// IsGoogleOAuthConfigured retorna true quando as três variáveis do Google
// OAuth estão definidas. Usado pelos handlers para decidir se o login via
// Google está disponível.
func (c *Config) IsGoogleOAuthConfigured() bool {
	return c.GoogleClientID != "" && c.GoogleClientSecret != "" && c.GoogleRedirectURL != ""
}

// --- helpers ---

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
