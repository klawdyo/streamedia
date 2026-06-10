package main

import (
	"os"
	"strings"
	"testing"
)

// TestReadmeExists verifica se o arquivo README.md existe na raiz do projeto.
func TestReadmeExists(t *testing.T) {
	_, err := os.Stat("README.md")
	if err != nil {
		t.Fatalf("README.md não encontrado: %v", err)
	}
}

// TestReadmeSections verifica se o README.md contém as seções e termos obrigatórios.
func TestReadmeSections(t *testing.T) {
	content, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("Erro ao ler README.md: %v", err)
	}

	text := string(content)

	// Termos obrigatórios que devem constar no README
	requiredTerms := []string{
		"Coolify",               // seção de deploy
		"POST /api/upload/init", // documentação da rota
		"master.m3u8",           // rota de HLS
		"webhook",               // formato de webhook
		"go test",               // como rodar testes
		"ROOT_TOKEN",            // variável de ambiente
		"pending_upload",        // tabela de status dos vídeos
	}

	for _, term := range requiredTerms {
		if !strings.Contains(text, term) {
			t.Errorf("Termo obrigatório %q não encontrado em README.md", term)
		}
	}
}

// TestReadmeNotEmpty verifica se o README.md tem conteúdo substancial (mais de 1000 bytes).
func TestReadmeNotEmpty(t *testing.T) {
	info, err := os.Stat("README.md")
	if err != nil {
		t.Fatalf("Erro ao obter informações do README.md: %v", err)
	}

	const minSize = 1000
	if info.Size() < minSize {
		t.Errorf("README.md tem %d bytes, esperado pelo menos %d bytes", info.Size(), minSize)
	}
}
