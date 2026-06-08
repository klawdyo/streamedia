package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoad_RequiredVarsMissing(t *testing.T) {
	// Garante que Load() falha quando variáveis obrigatórias estão ausentes.
	t.Setenv("UPLOAD_TOKEN_SECRET", "")
	t.Setenv("WEBHOOK_URL", "")
	t.Setenv("WEBHOOK_SECRET", "")

	_, err := Load()
	if err == nil {
		t.Fatal("esperava erro quando variáveis obrigatórias estão ausentes, mas Load() retornou nil")
	}
}

func TestLoad_RequiredVarsPresent(t *testing.T) {
	// Verifica que Load() funciona com as variáveis obrigatórias definidas.
	t.Setenv("UPLOAD_TOKEN_SECRET", "secret-upload")
	t.Setenv("WEBHOOK_URL", "https://backend.exemplo.com/webhooks")
	t.Setenv("WEBHOOK_SECRET", "secret-webhook")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() retornou erro inesperado: %v", err)
	}
	if cfg.UploadTokenSecret != "secret-upload" {
		t.Errorf("UploadTokenSecret: esperado %q, obtido %q", "secret-upload", cfg.UploadTokenSecret)
	}
	if cfg.WebhookURL != "https://backend.exemplo.com/webhooks" {
		t.Errorf("WebhookURL: esperado %q, obtido %q", "https://backend.exemplo.com/webhooks", cfg.WebhookURL)
	}
}

func TestLoad_Defaults(t *testing.T) {
	// Verifica que os valores padrão são aplicados para variáveis opcionais.
	t.Setenv("UPLOAD_TOKEN_SECRET", "s")
	t.Setenv("WEBHOOK_URL", "https://x.com")
	t.Setenv("WEBHOOK_SECRET", "s2")
	// Garante que variáveis opcionais não estão setadas
	t.Setenv("MAX_UPLOAD_SIZE_MB", "")
	t.Setenv("QUEUE_MAX_SIZE", "")
	t.Setenv("TRANSCODE_WORKERS", "")
	t.Setenv("PORT", "")
	t.Setenv("KEEP_ORIGINAL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() retornou erro inesperado: %v", err)
	}

	// Verifica os padrões documentados na spec
	if cfg.MaxUploadSizeBytes != 10*1024*1024 {
		t.Errorf("MaxUploadSizeBytes: esperado %d (10MB), obtido %d", 10*1024*1024, cfg.MaxUploadSizeBytes)
	}
	if cfg.QueueMaxSize != 50 {
		t.Errorf("QueueMaxSize: esperado 50, obtido %d", cfg.QueueMaxSize)
	}
	if cfg.TranscodeWorkers != 1 {
		t.Errorf("TranscodeWorkers: esperado 1, obtido %d", cfg.TranscodeWorkers)
	}
	if cfg.Port != 3000 {
		t.Errorf("Port: esperado 3000, obtido %d", cfg.Port)
	}
	if cfg.KeepOriginal != false {
		t.Errorf("KeepOriginal: esperado false, obtido %v", cfg.KeepOriginal)
	}
}

func TestLoad_OverrideDefaults(t *testing.T) {
	// Verifica que valores das variáveis de ambiente sobrescrevem os padrões.
	t.Setenv("UPLOAD_TOKEN_SECRET", "s")
	t.Setenv("WEBHOOK_URL", "https://x.com")
	t.Setenv("WEBHOOK_SECRET", "s2")
	t.Setenv("MAX_UPLOAD_SIZE_MB", "500")
	t.Setenv("TRANSCODE_WORKERS", "4")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() retornou erro inesperado: %v", err)
	}

	if cfg.MaxUploadSizeBytes != 500*1024*1024 {
		t.Errorf("MaxUploadSizeBytes: esperado %d (500MB), obtido %d", 500*1024*1024, cfg.MaxUploadSizeBytes)
	}
	if cfg.TranscodeWorkers != 4 {
		t.Errorf("TranscodeWorkers: esperado 4, obtido %d", cfg.TranscodeWorkers)
	}
}

func TestLoad_InvalidInt(t *testing.T) {
	// Verifica que Load() retorna erro quando um inteiro inválido é fornecido.
	t.Setenv("UPLOAD_TOKEN_SECRET", "s")
	t.Setenv("WEBHOOK_URL", "https://x.com")
	t.Setenv("WEBHOOK_SECRET", "s2")
	t.Setenv("MAX_UPLOAD_SIZE_MB", "nao_e_numero")

	_, err := Load()
	if err == nil {
		t.Fatal("esperava erro para MAX_UPLOAD_SIZE_MB inválido, mas Load() retornou nil")
	}
}

func TestLoad_TimeVarsDefaultsAreInSeconds(t *testing.T) {
	// issue #4: as variáveis de tempo devem ser lidas em segundos, com
	// defaults equivalentes aos valores anteriores (6h, 10min, 30min).
	t.Setenv("UPLOAD_TOKEN_SECRET", "s")
	t.Setenv("WEBHOOK_URL", "https://x.com")
	t.Setenv("WEBHOOK_SECRET", "s2")
	t.Setenv("UPLOAD_TOKEN_TTL_SECONDS", "")
	t.Setenv("PLAY_TOKEN_MAX_TTL_SECONDS", "")
	t.Setenv("UPLOAD_IDLE_TIMEOUT_SECONDS", "")
	t.Setenv("TRANSCODE_STUCK_SECONDS", "")

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
		{"PlayTokenMaxTTL", cfg.PlayTokenMaxTTL, 6 * time.Hour},
		{"UploadIdleTimeout", cfg.UploadIdleTimeout, 10 * time.Minute},
		{"TranscodeStuckTime", cfg.TranscodeStuckTime, 30 * time.Minute},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: esperado %v, obtido %v", c.name, c.want, c.got)
		}
	}
}

func TestLoad_TimeVarsReadInSeconds(t *testing.T) {
	// issue #4: definir as novas variáveis _SECONDS deve refletir
	// diretamente em time.Duration via time.Second, sem conversões ocultas.
	t.Setenv("UPLOAD_TOKEN_SECRET", "s")
	t.Setenv("WEBHOOK_URL", "https://x.com")
	t.Setenv("WEBHOOK_SECRET", "s2")
	t.Setenv("UPLOAD_TOKEN_TTL_SECONDS", "900")
	t.Setenv("PLAY_TOKEN_MAX_TTL_SECONDS", "1200")
	t.Setenv("UPLOAD_IDLE_TIMEOUT_SECONDS", "120")
	t.Setenv("TRANSCODE_STUCK_SECONDS", "300")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() retornou erro inesperado: %v", err)
	}

	if cfg.UploadTokenTTL != 900*time.Second {
		t.Errorf("UploadTokenTTL: esperado %v, obtido %v", 900*time.Second, cfg.UploadTokenTTL)
	}
	if cfg.PlayTokenMaxTTL != 1200*time.Second {
		t.Errorf("PlayTokenMaxTTL: esperado %v, obtido %v", 1200*time.Second, cfg.PlayTokenMaxTTL)
	}
	if cfg.UploadIdleTimeout != 120*time.Second {
		t.Errorf("UploadIdleTimeout: esperado %v, obtido %v", 120*time.Second, cfg.UploadIdleTimeout)
	}
	if cfg.TranscodeStuckTime != 300*time.Second {
		t.Errorf("TranscodeStuckTime: esperado %v, obtido %v", 300*time.Second, cfg.TranscodeStuckTime)
	}
}

func TestLoad_OldTimeVarNamesAreIgnored(t *testing.T) {
	// issue #4: os nomes antigos (sufixos _H e _MIN) não devem mais ser
	// lidos — é uma mudança incompatível intencional. Defini-los não deve
	// influenciar o resultado; os defaults em segundos devem prevalecer.
	t.Setenv("UPLOAD_TOKEN_SECRET", "s")
	t.Setenv("WEBHOOK_URL", "https://x.com")
	t.Setenv("WEBHOOK_SECRET", "s2")
	t.Setenv("UPLOAD_TOKEN_TTL_H", "1")
	t.Setenv("PLAY_TOKEN_MAX_TTL_H", "1")
	t.Setenv("UPLOAD_IDLE_TIMEOUT_MIN", "1")
	t.Setenv("TRANSCODE_STUCK_MIN", "1")
	t.Setenv("UPLOAD_TOKEN_TTL_SECONDS", "")
	t.Setenv("PLAY_TOKEN_MAX_TTL_SECONDS", "")
	t.Setenv("UPLOAD_IDLE_TIMEOUT_SECONDS", "")
	t.Setenv("TRANSCODE_STUCK_SECONDS", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() retornou erro inesperado: %v", err)
	}

	if cfg.UploadTokenTTL != 20*time.Minute {
		t.Errorf("UploadTokenTTL: variável antiga UPLOAD_TOKEN_TTL_H não deveria ser lida; esperado default %v, obtido %v", 20*time.Minute, cfg.UploadTokenTTL)
	}
	if cfg.PlayTokenMaxTTL != 6*time.Hour {
		t.Errorf("PlayTokenMaxTTL: variável antiga PLAY_TOKEN_MAX_TTL_H não deveria ser lida; esperado default %v, obtido %v", 6*time.Hour, cfg.PlayTokenMaxTTL)
	}
	if cfg.UploadIdleTimeout != 10*time.Minute {
		t.Errorf("UploadIdleTimeout: variável antiga UPLOAD_IDLE_TIMEOUT_MIN não deveria ser lida; esperado default %v, obtido %v", 10*time.Minute, cfg.UploadIdleTimeout)
	}
	if cfg.TranscodeStuckTime != 30*time.Minute {
		t.Errorf("TranscodeStuckTime: variável antiga TRANSCODE_STUCK_MIN não deveria ser lida; esperado default %v, obtido %v", 30*time.Minute, cfg.TranscodeStuckTime)
	}
}

func TestLoad_MissingUploadSecret(t *testing.T) {
	// Verifica que o erro menciona UPLOAD_TOKEN_SECRET quando ele está ausente.
	t.Setenv("UPLOAD_TOKEN_SECRET", "")
	t.Setenv("WEBHOOK_URL", "https://x.com")
	t.Setenv("WEBHOOK_SECRET", "s2")

	_, err := Load()
	if err == nil {
		t.Fatal("esperava erro, mas Load() retornou nil")
	}
	if !strings.Contains(err.Error(), "UPLOAD_TOKEN_SECRET") {
		t.Errorf("erro deve mencionar UPLOAD_TOKEN_SECRET, mas foi: %v", err)
	}
}

// TestGetEnvStr_TableDriven testa a função getEnvStr com múltiplos casos
func TestGetEnvStr_TableDriven(t *testing.T) {
	cases := []struct {
		name       string
		envValue   string
		defaultVal string
		expected   string
		desc       string
	}{
		{
			name:       "env_set_non_empty",
			envValue:   "http://example.com",
			defaultVal: "http://default.com",
			expected:   "http://example.com",
			desc:       "variável definida não-vazia deve retornar seu valor",
		},
		{
			name:       "env_empty_uses_default",
			envValue:   "",
			defaultVal: "http://default.com",
			expected:   "http://default.com",
			desc:       "variável vazia deve retornar default",
		},
		{
			name:       "env_unset_uses_default",
			envValue:   "",
			defaultVal: "http://default.com",
			expected:   "http://default.com",
			desc:       "variável não-setada deve retornar default",
		},
		{
			name:       "env_default_empty",
			envValue:   "some-value",
			defaultVal: "",
			expected:   "some-value",
			desc:       "default vazio é válido; env não-vazia prevalece",
		},
		{
			name:       "both_empty",
			envValue:   "",
			defaultVal: "",
			expected:   "",
			desc:       "ambos vazios devem retornar string vazia",
		},
		{
			name:       "special_chars",
			envValue:   "http://host.com:8080/path?query=value&other=123",
			defaultVal: "default",
			expected:   "http://host.com:8080/path?query=value&other=123",
			desc:       "caracteres especiais em URL devem ser preservados",
		},
		{
			name:       "whitespace_preserved",
			envValue:   "  spaces  ",
			defaultVal: "default",
			expected:   "  spaces  ",
			desc:       "espaços em branco devem ser preservados (não trimmed)",
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

// TestGetEnvBool_TableDriven testa a função getEnvBool com todos os casos edge
func TestGetEnvBool_TableDriven(t *testing.T) {
	cases := []struct {
		name       string
		envValue   string
		defaultVal bool
		expected   bool
		desc       string
	}{
		{
			name:       "true_lowercase",
			envValue:   "true",
			defaultVal: false,
			expected:   true,
			desc:       "valor 'true' (lowercase) deve retornar true",
		},
		{
			name:       "false_lowercase",
			envValue:   "false",
			defaultVal: true,
			expected:   false,
			desc:       "valor 'false' (lowercase) deve retornar false",
		},
		{
			name:       "one_string",
			envValue:   "1",
			defaultVal: false,
			expected:   true,
			desc:       "valor '1' deve retornar true",
		},
		{
			name:       "zero_string",
			envValue:   "0",
			defaultVal: true,
			expected:   false,
			desc:       "valor '0' deve retornar false",
		},
		{
			name:       "empty_uses_default",
			envValue:   "",
			defaultVal: true,
			expected:   true,
			desc:       "valor vazio deve usar default (true)",
		},
		{
			name:       "empty_uses_default_false",
			envValue:   "",
			defaultVal: false,
			expected:   false,
			desc:       "valor vazio deve usar default (false)",
		},
		{
			name:       "uppercase_true",
			envValue:   "TRUE",
			defaultVal: false,
			expected:   false,
			desc:       "valor 'TRUE' (uppercase) não é 'true' lowercase — usa default",
		},
		{
			name:       "uppercase_false",
			envValue:   "FALSE",
			defaultVal: true,
			expected:   true,
			desc:       "valor 'FALSE' (uppercase) não é 'false' lowercase — usa default",
		},
		{
			name:       "yes_is_not_true",
			envValue:   "yes",
			defaultVal: false,
			expected:   false,
			desc:       "valor 'yes' não é reconhecido — usa default",
		},
		{
			name:       "no_is_not_false",
			envValue:   "no",
			defaultVal: true,
			expected:   true,
			desc:       "valor 'no' não é reconhecido — usa default",
		},
		{
			name:       "two_is_unknown",
			envValue:   "2",
			defaultVal: false,
			expected:   false,
			desc:       "valor '2' não é '0' ou '1' — usa default",
		},
		{
			name:       "whitespace_around",
			envValue:   " true ",
			defaultVal: false,
			expected:   false,
			desc:       "valor ' true ' com whitespace não é 'true' puro — usa default",
		},
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

// TestGetEnvInt_NegativeAndZero testa getEnvInt com valores negativos e zero
func TestGetEnvInt_NegativeAndZero(t *testing.T) {
	cases := []struct {
		name       string
		envValue   string
		defaultVal int
		expected   int
		shouldErr  bool
		desc       string
	}{
		{
			name:       "zero_value",
			envValue:   "0",
			defaultVal: 999,
			expected:   0,
			shouldErr:  false,
			desc:       "valor 0 é válido",
		},
		{
			name:       "negative_value",
			envValue:   "-100",
			defaultVal: 999,
			expected:   -100,
			shouldErr:  false,
			desc:       "valor negativo é aceito (sem validação de range em getEnvInt)",
		},
		{
			name:       "large_positive",
			envValue:   "2147483647",
			defaultVal: 999,
			expected:   2147483647,
			shouldErr:  false,
			desc:       "valor máximo int32 é aceito",
		},
		{
			name:       "invalid_non_numeric",
			envValue:   "not-a-number",
			defaultVal: 999,
			expected:   0,
			shouldErr:  true,
			desc:       "string não-numérica deve retornar erro",
		},
		{
			name:       "float_string",
			envValue:   "123.45",
			defaultVal: 999,
			expected:   0,
			shouldErr:  true,
			desc:       "string float não é inteiro válido para strconv.Atoi",
		},
		{
			name:       "leading_whitespace",
			envValue:   " 123",
			defaultVal: 999,
			expected:   0,
			shouldErr:  true,
			desc:       "strconv.Atoi NÃO aceita whitespace leading — erro esperado",
		},
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

// TestLoad_AllVarsDefinedInvalidCombos testa combinações inválidas de variáveis
func TestLoad_AllVarsDefinedInvalidCombos(t *testing.T) {
	cases := []struct {
		name         string
		setupEnv     map[string]string
		shouldErr    bool
		errorPattern string
		desc         string
	}{
		{
			name: "all_required_present",
			setupEnv: map[string]string{
				"UPLOAD_TOKEN_SECRET": "secret",
				"WEBHOOK_URL":         "https://example.com",
				"WEBHOOK_SECRET":      "whsecret",
			},
			shouldErr: false,
			desc:      "com todas as obrigatórias, Load deve suceder",
		},
		{
			name: "missing_webhook_url",
			setupEnv: map[string]string{
				"UPLOAD_TOKEN_SECRET": "secret",
				"WEBHOOK_URL":         "",
				"WEBHOOK_SECRET":      "whsecret",
			},
			shouldErr:    true,
			errorPattern: "WEBHOOK_URL",
			desc:         "sem WEBHOOK_URL, deve retornar erro mencionando a variável",
		},
		{
			name: "missing_webhook_secret",
			setupEnv: map[string]string{
				"UPLOAD_TOKEN_SECRET": "secret",
				"WEBHOOK_URL":         "https://example.com",
				"WEBHOOK_SECRET":      "",
			},
			shouldErr:    true,
			errorPattern: "WEBHOOK_SECRET",
			desc:         "sem WEBHOOK_SECRET, deve retornar erro mencionando a variável",
		},
		{
			name: "zero_port",
			setupEnv: map[string]string{
				"UPLOAD_TOKEN_SECRET": "secret",
				"WEBHOOK_URL":         "https://example.com",
				"WEBHOOK_SECRET":      "whsecret",
				"PORT":                "0",
			},
			shouldErr: false,
			desc:      "PORT=0 é aceitável (bind em porta aleatória ou default)",
		},
		{
			name: "negative_max_size",
			setupEnv: map[string]string{
				"UPLOAD_TOKEN_SECRET": "secret",
				"WEBHOOK_URL":         "https://example.com",
				"WEBHOOK_SECRET":      "whsecret",
				"MAX_UPLOAD_SIZE_MB":  "-10",
			},
			shouldErr: false,
			desc:      "MAX_UPLOAD_SIZE_MB negativo é aceitável (sem validação adicional em Load)",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Limpa e define variáveis
			for k := range tc.setupEnv {
				t.Setenv(k, tc.setupEnv[k])
			}
			cfg, err := Load()
			if tc.shouldErr && err == nil {
				t.Errorf("%s: esperava erro, mas Load() retornou nil", tc.desc)
			} else if !tc.shouldErr && err != nil {
				t.Errorf("%s: esperava sucesso, mas retornou erro: %v", tc.desc, err)
			} else if tc.shouldErr && err != nil && !strings.Contains(err.Error(), tc.errorPattern) {
				t.Errorf("%s: erro não menciona '%s': %v", tc.desc, tc.errorPattern, err)
			} else if !tc.shouldErr && cfg == nil {
				t.Errorf("%s: cfg é nil mesmo sem erro", tc.desc)
			}
		})
	}
}
