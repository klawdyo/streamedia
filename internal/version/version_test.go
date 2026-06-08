package version

import "testing"

// TestGet_ReturnsDefaults verifica que em ambiente de teste (sem -ldflags),
// Get() retorna os valores padrão: "0.0.0-dev", "unknown", "ok".
func TestGet_ReturnsDefaults(t *testing.T) {
	info := Get()

	if info.Name != "Streamedia" {
		t.Errorf("Name: esperado 'Streamedia', obtido %q", info.Name)
	}
	if info.Version != "0.0.0-dev" {
		t.Errorf("Version: esperado '0.0.0-dev', obtido %q", info.Version)
	}
	if info.Commit != "unknown" {
		t.Errorf("Commit: esperado 'unknown', obtido %q", info.Commit)
	}
	if info.BuildTime != "unknown" {
		t.Errorf("BuildTime: esperado 'unknown', obtido %q", info.BuildTime)
	}
	if info.Status != "ok" {
		t.Errorf("Status: esperado 'ok', obtido %q", info.Status)
	}
}

// TestGet_FieldsAreNonEmpty verifica que nenhum campo essencial está vazio.
func TestGet_FieldsAreNonEmpty(t *testing.T) {
	info := Get()

	if info.Name == "" {
		t.Error("Name está vazio")
	}
	if info.Version == "" {
		t.Error("Version está vazio")
	}
	if info.Status == "" {
		t.Error("Status está vazio")
	}
}
