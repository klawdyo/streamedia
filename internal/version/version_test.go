package version

import "testing"

// TestGet_ReturnsDefaults verifica que em ambiente de teste (sem -ldflags),
// Get() retorna os valores padrão de build e o ambiente informado.
func TestGet_ReturnsDefaults(t *testing.T) {
	info := Get("development")

	if info.Name != "Streamedia" {
		t.Errorf("Name: esperado 'Streamedia', obtido %q", info.Name)
	}
	if info.Version != "0.0.0-dev" {
		t.Errorf("Version: esperado '0.0.0-dev', obtido %q", info.Version)
	}
	if info.Environment != "development" {
		t.Errorf("Environment: esperado 'development', obtido %q", info.Environment)
	}
	if info.Status != "ok" {
		t.Errorf("Status: esperado 'ok', obtido %q", info.Status)
	}
}

// TestGet_PropagatesEnvironment verifica que o ambiente passado pelo chamador
// (config/ENV) é refletido na resposta — inclusive "production".
func TestGet_PropagatesEnvironment(t *testing.T) {
	if info := Get("production"); info.Environment != "production" {
		t.Errorf("Environment: esperado 'production', obtido %q", info.Environment)
	}
}

// TestGet_FieldsAreNonEmpty verifica que nenhum campo essencial está vazio.
func TestGet_FieldsAreNonEmpty(t *testing.T) {
	info := Get("development")

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
