package config

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/klawdyo/streamedia/internal/db"
)

func TestLoad_RequiredVarsMissing(t *testing.T) {
	t.Setenv("ROOT_TOKEN", "")

	_, err := Load()
	if err == nil {
		t.Fatal("esperava erro quando ROOT_TOKEN está ausente, mas Load() retornou nil")
	}
}

func TestLoad_RequiredVarsPresent(t *testing.T) {
	t.Setenv("ROOT_TOKEN", "secret-upload")
	t.Setenv("GOOGLE_CLIENT_ID", "test-client-id")
	t.Setenv("GOOGLE_CLIENT_SECRET", "test-client-secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() retornou erro inesperado: %v", err)
	}
	if cfg.RootToken != "secret-upload" {
		t.Errorf("RootToken: esperado %q, obtido %q", "secret-upload", cfg.RootToken)
	}
	if cfg.GoogleClientID != "test-client-id" {
		t.Errorf("GoogleClientID: esperado %q, obtido %q", "test-client-id", cfg.GoogleClientID)
	}
	if cfg.GoogleClientSecret != "test-client-secret" {
		t.Errorf("GoogleClientSecret: esperado %q, obtido %q", "test-client-secret", cfg.GoogleClientSecret)
	}
}

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("ROOT_TOKEN", "s")
	t.Setenv("GOOGLE_CLIENT_ID", "test-id")
	t.Setenv("GOOGLE_CLIENT_SECRET", "test-secret")
	t.Setenv("ENV", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() retornou erro inesperado: %v", err)
	}

	if cfg.Environment != "development" {
		t.Errorf("Environment: esperado 'development' (default), obtido %q", cfg.Environment)
	}
	if cfg.Port != 3000 {
		t.Errorf("Port: esperado 3000, obtido %d", cfg.Port)
	}
	if cfg.SQLitePath != "/data/media.db" {
		t.Errorf("SQLitePath: esperado /data/media.db, obtido %q", cfg.SQLitePath)
	}
	if cfg.MediaDir != "/media" {
		t.Errorf("MediaDir: esperado /media, obtido %q", cfg.MediaDir)
	}
	if cfg.UploadTmpDir != "/media/.uploads" {
		t.Errorf("UploadTmpDir: esperado /media/.uploads, obtido %q", cfg.UploadTmpDir)
	}
	if cfg.SPADir != "./web/dist" {
		t.Errorf("SPADir: esperado ./web/dist, obtido %q", cfg.SPADir)
	}
	if cfg.SessionCookieSecure != false {
		t.Errorf("SessionCookieSecure: esperado false (ENV=development), obtido %v", cfg.SessionCookieSecure)
	}
	// Defaults operacionais (serão sobrescritos pelo banco via ApplyFromDB)
	if cfg.MaxUploadSizeBytes != 10*1024*1024 {
		t.Errorf("MaxUploadSizeBytes: esperado %d (10MB), obtido %d", 10*1024*1024, cfg.MaxUploadSizeBytes)
	}
	if cfg.QueueMaxSize != 50 {
		t.Errorf("QueueMaxSize: esperado 50, obtido %d", cfg.QueueMaxSize)
	}
	if cfg.TranscodeWorkers != 1 {
		t.Errorf("TranscodeWorkers: esperado 1, obtido %d", cfg.TranscodeWorkers)
	}
	if cfg.KeepOriginal != false {
		t.Errorf("KeepOriginal: esperado false, obtido %v", cfg.KeepOriginal)
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	t.Setenv("ROOT_TOKEN", "s")
	t.Setenv("GOOGLE_CLIENT_ID", "test-id")
	t.Setenv("GOOGLE_CLIENT_SECRET", "test-secret")
	t.Setenv("PORT", "8080")
	t.Setenv("ENV", "production")
	t.Setenv("SQLITE_PATH", "/custom/path.db")
	t.Setenv("SESSION_COOKIE_SECURE", "true")
	t.Setenv("SPA_DIR", "/custom/spa")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() retornou erro inesperado: %v", err)
	}

	if cfg.Port != 8080 {
		t.Errorf("Port: esperado 8080, obtido %d", cfg.Port)
	}
	if cfg.Environment != "production" {
		t.Errorf("Environment: esperado 'production', obtido %q", cfg.Environment)
	}
	if cfg.SQLitePath != "/custom/path.db" {
		t.Errorf("SQLitePath: esperado /custom/path.db, obtido %q", cfg.SQLitePath)
	}
	if cfg.SessionCookieSecure != true {
		t.Errorf("SessionCookieSecure: esperado true, obtido %v", cfg.SessionCookieSecure)
	}
	if cfg.SPADir != "/custom/spa" {
		t.Errorf("SPADir: esperado /custom/spa, obtido %q", cfg.SPADir)
	}
}

func TestLoad_InvalidInt(t *testing.T) {
	t.Setenv("ROOT_TOKEN", "s")
	t.Setenv("GOOGLE_CLIENT_ID", "test-id")
	t.Setenv("GOOGLE_CLIENT_SECRET", "test-secret")
	t.Setenv("PORT", "nao_e_numero")

	_, err := Load()
	if err == nil {
		t.Fatal("esperava erro para PORT inválido, mas Load() retornou nil")
	}
}

func TestLoad_MissingRootToken(t *testing.T) {
	t.Setenv("ROOT_TOKEN", "")

	_, err := Load()
	if err == nil {
		t.Fatal("esperava erro, mas Load() retornou nil")
	}
	if !strings.Contains(err.Error(), "ROOT_TOKEN") {
		t.Errorf("erro deve mencionar ROOT_TOKEN, mas foi: %v", err)
	}
}

func TestLoad_MissingGoogleOAuth(t *testing.T) {
	// GOOGLE_CLIENT_ID e GOOGLE_CLIENT_SECRET sao obrigatorios.
	t.Setenv("ROOT_TOKEN", "s")

	_, err := Load()
	if err == nil {
		t.Fatal("esperava erro quando Google OAuth esta ausente, mas Load() retornou nil")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "GOOGLE_CLIENT_ID") {
		t.Errorf("erro deve mencionar GOOGLE_CLIENT_ID, mas foi: %v", err)
	}
	if !strings.Contains(errStr, "GOOGLE_CLIENT_SECRET") {
		t.Errorf("erro deve mencionar GOOGLE_CLIENT_SECRET, mas foi: %v", err)
	}
}

func TestLoad_MissingGoogleClientSecretOnly(t *testing.T) {
	t.Setenv("ROOT_TOKEN", "s")
	t.Setenv("GOOGLE_CLIENT_ID", "test-id")

	_, err := Load()
	if err == nil {
		t.Fatal("esperava erro quando GOOGLE_CLIENT_SECRET esta ausente, mas Load() retornou nil")
	}
	if !strings.Contains(err.Error(), "GOOGLE_CLIENT_SECRET") {
		t.Errorf("erro deve mencionar GOOGLE_CLIENT_SECRET, mas foi: %v", err)
	}
}

func TestLoad_SessionCookieSecureEnv(t *testing.T) {
	t.Setenv("ROOT_TOKEN", "s")
	t.Setenv("GOOGLE_CLIENT_ID", "test-id")
	t.Setenv("GOOGLE_CLIENT_SECRET", "test-secret")

	// Default: false em development
	t.Setenv("ENV", "development")
	cfg, _ := Load()
	if cfg.SessionCookieSecure != false {
		t.Errorf("em development: esperado false, obtido %v", cfg.SessionCookieSecure)
	}

	// Default: true em production
	t.Setenv("ENV", "production")
	cfg, _ = Load()
	if cfg.SessionCookieSecure != true {
		t.Errorf("em production: esperado true, obtido %v", cfg.SessionCookieSecure)
	}

	// Sobrescrito explicitamente
	t.Setenv("ENV", "development")
	t.Setenv("SESSION_COOKIE_SECURE", "true")
	cfg, _ = Load()
	if cfg.SessionCookieSecure != true {
		t.Errorf("explícito true: esperado true, obtido %v", cfg.SessionCookieSecure)
	}
}

func TestLoad_WebhookSecretOptional(t *testing.T) {
	// WEBHOOK_URL agora vem do banco — não há mais validação no Load().
	// WEBHOOK_SECRET é sempre opcional.
	t.Setenv("ROOT_TOKEN", "s")
	t.Setenv("GOOGLE_CLIENT_ID", "test-id")
	t.Setenv("GOOGLE_CLIENT_SECRET", "test-secret")
	t.Setenv("WEBHOOK_SECRET", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() retornou erro inesperado: %v", err)
	}
	if cfg.WebhookSecret != "" {
		t.Errorf("WebhookSecret: esperado \"\", obtido %q", cfg.WebhookSecret)
	}
}

func TestLoad_TimeDefaults(t *testing.T) {
	// Valores de tempo agora são defaults hardcoded (vindos do código),
	// não do env. O banco sobrescreve via ApplyFromDB.
	t.Setenv("ROOT_TOKEN", "s")
	t.Setenv("GOOGLE_CLIENT_ID", "test-id")
	t.Setenv("GOOGLE_CLIENT_SECRET", "test-secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() retornou erro inesperado: %v", err)
	}

	cases := []struct {
		name string
		got  time.Duration
		want time.Duration
	}{
		{"UploadTokenTTL", cfg.UploadTokenTTL, 20 * time.Minute},
		{"PlayTokenTTL", cfg.PlayTokenTTL, 1 * time.Hour},
		{"UploadIdleTimeout", cfg.UploadIdleTimeout, 10 * time.Minute},
		{"TranscodeStuckTime", cfg.TranscodeStuckTime, 30 * time.Minute},
		{"SessionTTL", cfg.SessionTTL, 12 * time.Hour},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: esperado %v, obtido %v", c.name, c.want, c.got)
		}
	}
}

// --- Testes do ApplyFromDB ---

func TestApplyFromDB_DefaultsPreserved(t *testing.T) {
	// Sem a tabela configurations no banco, os defaults do código são mantidos.
	tdb := openTestDB(t)
	defer tdb.Close()

	cfg := testCfg()
	cfg.ApplyFromDB(tdb)

	if cfg.MaxUploadSizeBytes != 10*1024*1024 {
		t.Errorf("MaxUploadSizeBytes: esperado %d, obtido %d", 10*1024*1024, cfg.MaxUploadSizeBytes)
	}
	if cfg.TranscodeWorkers != 1 {
		t.Errorf("TranscodeWorkers: esperado 1, obtido %d", cfg.TranscodeWorkers)
	}
	if cfg.WebhookURL != "" {
		t.Errorf("WebhookURL: esperado vazio, obtido %q", cfg.WebhookURL)
	}
}

func TestApplyFromDB_OverridesFromDB(t *testing.T) {
	tdb := openTestDB(t)
	defer tdb.Close()

	// Insere valores customizados na tabela configurations
	tdb.Exec(`INSERT OR REPLACE INTO configurations (key, value, type, description, group_key) VALUES ('upload.max_size_mb', '50', 'number', 'test', 'upload')`)
	tdb.Exec(`INSERT OR REPLACE INTO configurations (key, value, type, description, group_key) VALUES ('transcode.workers', '4', 'number', 'test', 'transcode')`)
	tdb.Exec(`INSERT OR REPLACE INTO configurations (key, value, type, description, group_key) VALUES ('webhook.url', 'https://example.com/hook', 'url', 'test', 'webhook')`)
	tdb.Exec(`INSERT OR REPLACE INTO configurations (key, value, type, description, group_key) VALUES ('token.upload_ttl', '900', 'duration_seconds', 'test', 'token')`)
	tdb.Exec(`INSERT OR REPLACE INTO configurations (key, value, type, description, group_key) VALUES ('transcode.keep_original', 'true', 'boolean', 'test', 'transcode')`)

	cfg := testCfg()
	cfg.ApplyFromDB(tdb)

	if cfg.MaxUploadSizeBytes != 50*1024*1024 {
		t.Errorf("MaxUploadSizeBytes: esperado %d, obtido %d", 50*1024*1024, cfg.MaxUploadSizeBytes)
	}
	if cfg.TranscodeWorkers != 4 {
		t.Errorf("TranscodeWorkers: esperado 4, obtido %d", cfg.TranscodeWorkers)
	}
	if cfg.WebhookURL != "https://example.com/hook" {
		t.Errorf("WebhookURL: esperado https://example.com/hook, obtido %q", cfg.WebhookURL)
	}
	if cfg.UploadTokenTTL != 900*time.Second {
		t.Errorf("UploadTokenTTL: esperado %v, obtido %v", 900*time.Second, cfg.UploadTokenTTL)
	}
	if cfg.KeepOriginal != true {
		t.Errorf("KeepOriginal: esperado true, obtido %v", cfg.KeepOriginal)
	}
}

func TestApplyFromDB_InvalidValuesFallback(t *testing.T) {
	tdb := openTestDB(t)
	defer tdb.Close()

	// Valor inválido: transcode.workers como string não-numérica
	tdb.Exec(`INSERT OR REPLACE INTO configurations (key, value, type, description, group_key) VALUES ('transcode.workers', 'abc', 'number', 'test', 'transcode')`)
	// Valor inválido: keep_original não é boolean
	tdb.Exec(`INSERT OR REPLACE INTO configurations (key, value, type, description, group_key) VALUES ('transcode.keep_original', 'xyz', 'boolean', 'test', 'transcode')`)
	// Valor inválido: upload.max_size_mb negativo
	tdb.Exec(`INSERT OR REPLACE INTO configurations (key, value, type, description, group_key) VALUES ('upload.max_size_mb', '-10', 'number', 'test', 'upload')`)

	cfg := testCfg()
	cfg.ApplyFromDB(tdb)

	// Deve manter os defaults (fallback)
	if cfg.TranscodeWorkers != 1 {
		t.Errorf("TranscodeWorkers: esperado fallback 1, obtido %d", cfg.TranscodeWorkers)
	}
	if cfg.KeepOriginal != false {
		t.Errorf("KeepOriginal: esperado fallback false, obtido %v", cfg.KeepOriginal)
	}
	if cfg.MaxUploadSizeBytes < 1*1024*1024 {
		t.Errorf("MaxUploadSizeBytes: valor negativo foi corrigido, obtido %d (mínimo 1MB)", cfg.MaxUploadSizeBytes)
	}
}

func TestApplyFromDB_DisabledFeatures(t *testing.T) {
	tdb := openTestDB(t)
	defer tdb.Close()

	cfg := testCfg()

	// Discord e webhook desabilitados por padrão
	cfg.ApplyFromDB(tdb)
	if cfg.DiscordWebhookURL != "" {
		t.Errorf("DiscordWebhookURL: esperado vazio (desabilitado), obtido %q", cfg.DiscordWebhookURL)
	}
	if cfg.WebhookURL != "" {
		t.Errorf("WebhookURL: esperado vazio (desabilitado), obtido %q", cfg.WebhookURL)
	}

	// Habilita Discord
	tdb.Exec(`INSERT OR REPLACE INTO configurations (key, value, type, description, group_key) VALUES ('discord.webhook_url', 'https://discord.com/api/webhooks/123/abc', 'url', 'test', 'discord')`)
	cfg.ApplyFromDB(tdb)
	if cfg.DiscordWebhookURL != "https://discord.com/api/webhooks/123/abc" {
		t.Errorf("DiscordWebhookURL: esperado URL do Discord, obtido %q", cfg.DiscordWebhookURL)
	}
}

// --- Helpers ---

func testCfg() *Config {
	return &Config{
		MaxUploadSizeBytes: 10 * 1024 * 1024,
		QueueMaxSize:       50,
		TranscodeWorkers:   1,
		UploadTokenTTL:     20 * time.Minute,
		PlayTokenTTL:       1 * time.Hour,
		UploadIdleTimeout:  10 * time.Minute,
		TranscodeStuckTime: 30 * time.Minute,
		MaxTranscodeAttempts: 3,
		KeepOriginal:      false,
		RateLimitPerMin:   60,
		SessionTTL:        12 * time.Hour,
		WebhookURL:        "",
		DiscordWebhookURL: "",
	}
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("falha ao abrir banco em memória: %v", err)
	}
	return database
}

// --- Testes de getEnv* (mantidos da versão anterior) ---

func TestGetEnvStr_TableDriven(t *testing.T) {
	cases := []struct {
		name       string
		envValue   string
		defaultVal string
		expected   string
		desc       string
	}{
		{
			name: "env_set_non_empty", envValue: "http://example.com",
			defaultVal: "http://default.com", expected: "http://example.com",
			desc: "variável definida não-vazia deve retornar seu valor",
		},
		{
			name: "env_empty_uses_default", envValue: "",
			defaultVal: "http://default.com", expected: "http://default.com",
			desc: "variável vazia deve retornar default",
		},
		{
			name: "both_empty", envValue: "", defaultVal: "", expected: "",
			desc: "ambos vazios devem retornar string vazia",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TEST_ENV_STR", tc.envValue)
			result := getEnvStr("TEST_ENV_STR", tc.defaultVal)
			if result != tc.expected {
				t.Errorf("%s: esperado %q, obtido %q", tc.desc, tc.expected, result)
			}
		})
	}
}

func TestGetEnvBool_TableDriven(t *testing.T) {
	cases := []struct {
		name       string
		envValue   string
		defaultVal bool
		expected   bool
		desc       string
	}{
		{name: "true_lowercase", envValue: "true", defaultVal: false, expected: true, desc: "'true' → true"},
		{name: "false_lowercase", envValue: "false", defaultVal: true, expected: false, desc: "'false' → false"},
		{name: "one_string", envValue: "1", defaultVal: false, expected: true, desc: "'1' → true"},
		{name: "zero_string", envValue: "0", defaultVal: true, expected: false, desc: "'0' → false"},
		{name: "empty_uses_default", envValue: "", defaultVal: true, expected: true, desc: "vazio → default"},
		{name: "uppercase_true", envValue: "TRUE", defaultVal: false, expected: false, desc: "'TRUE' não é 'true' → default"},
		{name: "whitespace_around", envValue: " true ", defaultVal: false, expected: false, desc: "' true ' não é 'true' puro → default"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TEST_ENV_BOOL", tc.envValue)
			result := getEnvBool("TEST_ENV_BOOL", tc.defaultVal)
			if result != tc.expected {
				t.Errorf("%s: esperado %v, obtido %v", tc.desc, tc.expected, result)
			}
		})
	}
}

func TestGetEnvInt_NegativeAndZero(t *testing.T) {
	cases := []struct {
		name       string
		envValue   string
		defaultVal int
		expected   int
		shouldErr  bool
		desc       string
	}{
		{name: "zero_value", envValue: "0", defaultVal: 999, expected: 0, shouldErr: false, desc: "0 é válido"},
		{name: "negative_value", envValue: "-100", defaultVal: 999, expected: -100, shouldErr: false, desc: "negativo aceito"},
		{name: "invalid_non_numeric", envValue: "not-a-number", defaultVal: 999, expected: 0, shouldErr: true, desc: "string não-numérica → erro"},
		{name: "float_string", envValue: "123.45", defaultVal: 999, expected: 0, shouldErr: true, desc: "float → erro"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TEST_ENV_INT", tc.envValue)
			result, err := getEnvInt("TEST_ENV_INT", tc.defaultVal)
			if tc.shouldErr && err == nil {
				t.Errorf("%s: esperava erro, mas retornou nil", tc.desc)
			} else if !tc.shouldErr && err != nil {
				t.Errorf("%s: esperava sucesso, mas retornou erro: %v", tc.desc, err)
			} else if !tc.shouldErr && result != tc.expected {
				t.Errorf("%s: esperado %d, obtido %d", tc.desc, tc.expected, result)
			}
		})
	}
}
