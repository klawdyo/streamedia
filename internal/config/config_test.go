package config

import (
	"strings"
	"testing"
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
