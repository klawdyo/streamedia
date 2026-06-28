package main

import (
	"os"
	"strings"
	"testing"
)

// TestDockerfileExists verifica se o Dockerfile existe
func TestDockerfileExists(t *testing.T) {
	_, err := os.Stat("Dockerfile")
	if err != nil {
		t.Fatalf("Dockerfile não encontrado: %v", err)
	}
}

// TestDockerComposeExists verifica se docker-compose.yml existe
func TestDockerComposeExists(t *testing.T) {
	_, err := os.Stat("docker-compose.yml")
	if err != nil {
		t.Fatalf("docker-compose.yml não encontrado: %v", err)
	}
}

// TestEnvExampleExists verifica se .env.example existe
func TestEnvExampleExists(t *testing.T) {
	_, err := os.Stat(".env.example")
	if err != nil {
		t.Fatalf(".env.example não encontrado: %v", err)
	}
}

// TestEnvExampleHasAllVars verifica se .env.example contém todas as variáveis esperadas
func TestEnvExampleHasAllVars(t *testing.T) {
	content, err := os.ReadFile(".env.example")
	if err != nil {
		t.Fatalf("Erro ao ler .env.example: %v", err)
	}

	text := string(content)
	requiredVars := []string{
		"ROOT_TOKEN",
		"WEBHOOK_SECRET",
		"GOOGLE_CLIENT_ID",
		"GOOGLE_CLIENT_SECRET",
		"GOOGLE_REDIRECT_URL",
		"SQLITE_PATH",
		"PORT",
		"ENV",
	}

	for _, varName := range requiredVars {
		if !strings.Contains(text, varName) {
			t.Errorf("Variável %s não encontrada em .env.example", varName)
		}
	}
}

// TestGitignoreHasEnv verifica se .gitignore contém .env
func TestGitignoreHasEnv(t *testing.T) {
	content, err := os.ReadFile(".gitignore")
	if err != nil {
		t.Fatalf("Erro ao ler .gitignore: %v", err)
	}

	text := string(content)
	if !strings.Contains(text, ".env") {
		t.Error(".env não encontrado em .gitignore")
	}
}

// TestDockerComposeSyntax verifica a sintaxe básica do docker-compose.yml
func TestDockerComposeSyntax(t *testing.T) {
	content, err := os.ReadFile("docker-compose.yml")
	if err != nil {
		t.Fatalf("Erro ao ler docker-compose.yml: %v", err)
	}

	text := string(content)
	requiredStrings := []string{
		"services:",
		"mediaserver:",
	}

	for _, str := range requiredStrings {
		if !strings.Contains(text, str) {
			t.Errorf("String '%s' não encontrada em docker-compose.yml", str)
		}
	}
}

// TestDockerfileMultiStage verifica se o Dockerfile tem os estágios de build e runtime
func TestDockerfileMultiStage(t *testing.T) {
	content, err := os.ReadFile("Dockerfile")
	if err != nil {
		t.Fatalf("Erro ao ler Dockerfile: %v", err)
	}

	text := string(content)
	requiredStrings := []string{
		"FROM golang",
		"FROM alpine",
		"CGO_ENABLED=0",
		// O servidor roda como não-root: o container sobe como root só para o
		// chown dos bind mounts no entrypoint e baixa o privilégio com su-exec.
		"su-exec",
	}

	for _, str := range requiredStrings {
		if !strings.Contains(text, str) {
			t.Errorf("String '%s' não encontrada em Dockerfile", str)
		}
	}
}
