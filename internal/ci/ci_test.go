package ci

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCIWorkflowExists verifica se o workflow de CI existe
func TestCIWorkflowExists(t *testing.T) {
	path := filepath.Join("../../.github/workflows/ci.yml")
	_, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Workflow CI não encontrado em %s: %v", path, err)
	}
}

// TestReleaseWorkflowExists verifica se o workflow de Release existe
func TestReleaseWorkflowExists(t *testing.T) {
	path := filepath.Join("../../.github/workflows/release.yml")
	_, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Workflow Release não encontrado em %s: %v", path, err)
	}
}

// TestCIWorkflowHasGoTest verifica se o workflow de CI contém "go test"
func TestCIWorkflowHasGoTest(t *testing.T) {
	path := filepath.Join("../../.github/workflows/ci.yml")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Erro ao ler workflow CI: %v", err)
	}

	text := string(content)
	if !strings.Contains(text, "go test") {
		t.Error("'go test' não encontrado no workflow CI")
	}
}

// TestCIWorkflowHasGoVet verifica se o workflow de CI contém "go vet"
func TestCIWorkflowHasGoVet(t *testing.T) {
	path := filepath.Join("../../.github/workflows/ci.yml")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Erro ao ler workflow CI: %v", err)
	}

	text := string(content)
	if !strings.Contains(text, "go vet") {
		t.Error("'go vet' não encontrado no workflow CI")
	}
}

// TestCIWorkflowHasBuild verifica se o workflow de CI contém step de build
func TestCIWorkflowHasBuild(t *testing.T) {
	path := filepath.Join("../../.github/workflows/ci.yml")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Erro ao ler workflow CI: %v", err)
	}

	text := string(content)
	if !strings.Contains(text, "go build") {
		t.Error("'go build' não encontrado no workflow CI")
	}
}

// TestReleaseWorkflowHasGHCR verifica se o workflow de Release contém referência ao GHCR
func TestReleaseWorkflowHasGHCR(t *testing.T) {
	path := filepath.Join("../../.github/workflows/release.yml")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Erro ao ler workflow Release: %v", err)
	}

	text := string(content)
	if !strings.Contains(text, "ghcr.io") {
		t.Error("'ghcr.io' não encontrado no workflow Release")
	}
}

// TestReleaseWorkflowUsesGithubToken verifica se o workflow de Release usa GITHUB_TOKEN
// e não contém UPLOAD_TOKEN_SECRET
func TestReleaseWorkflowUsesGithubToken(t *testing.T) {
	path := filepath.Join("../../.github/workflows/release.yml")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Erro ao ler workflow Release: %v", err)
	}

	text := string(content)
	if !strings.Contains(text, "GITHUB_TOKEN") {
		t.Error("'GITHUB_TOKEN' não encontrado no workflow Release")
	}

	if strings.Contains(text, "UPLOAD_TOKEN_SECRET") {
		t.Error("'UPLOAD_TOKEN_SECRET' não deveria estar no workflow Release")
	}
}

// TestReleaseWorkflowOnTag verifica se o workflow de Release está configurado para tags
func TestReleaseWorkflowOnTag(t *testing.T) {
	path := filepath.Join("../../.github/workflows/release.yml")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Erro ao ler workflow Release: %v", err)
	}

	text := string(content)
	if !strings.Contains(text, "v*") {
		t.Error("Trigger de tag 'v*' não encontrado no workflow Release")
	}
}
