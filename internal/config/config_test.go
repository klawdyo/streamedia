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
		{"UploadTokenTTL", cfg.UploadTokenTTL, 6 * time.Hour},
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

	if cfg.UploadTokenTTL != 6*time.Hour {
		t.Errorf("UploadTokenTTL: variável antiga UPLOAD_TOKEN_TTL_H não deveria ser lida; esperado default %v, obtido %v", 6*time.Hour, cfg.UploadTokenTTL)
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
