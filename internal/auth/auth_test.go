package auth

import (
	"testing"
	"time"
)

func TestGenerateUploadToken_Deterministic(t *testing.T) {
	// HMAC é determinístico — mesmos inputs sempre geram o mesmo token.
	t1 := GenerateUploadToken("secret123", "video-abc")
	t2 := GenerateUploadToken("secret123", "video-abc")
	if t1 != t2 {
		t.Errorf("GenerateUploadToken deveria ser determinístico: obteve %q e %q", t1, t2)
	}
	if t1 == "" {
		t.Error("GenerateUploadToken não deve retornar string vazia")
	}
}

func TestGenerateUploadToken_DifferentInputs(t *testing.T) {
	// Inputs diferentes devem gerar tokens diferentes.
	tok1 := GenerateUploadToken("secret", "video-id-1")
	tok2 := GenerateUploadToken("secret", "video-id-2")
	if tok1 == tok2 {
		t.Error("tokens para video_ids diferentes devem ser diferentes")
	}
}

func TestValidatePlayToken_Valid(t *testing.T) {
	// Token gerado e validado com os mesmos parâmetros deve ser aceito.
	expires := time.Now().Add(time.Hour).Unix()
	token := GeneratePlayToken("secret-play", "vid-123", expires)

	if err := ValidatePlayToken("secret-play", "vid-123", expires, token, 6*time.Hour); err != nil {
		t.Errorf("token válido deveria ser aceito, mas retornou erro: %v", err)
	}
}

func TestValidatePlayToken_WrongSecret(t *testing.T) {
	// Token gerado com secret A não deve ser validado com secret B.
	expires := time.Now().Add(time.Hour).Unix()
	token := GeneratePlayToken("secret-A", "vid-123", expires)

	if err := ValidatePlayToken("secret-B", "vid-123", expires, token, 6*time.Hour); err == nil {
		t.Error("token gerado com secret errado deveria ser rejeitado")
	}
}

func TestValidatePlayToken_Expired(t *testing.T) {
	// Token com timestamp de expiração no passado deve ser rejeitado.
	expires := time.Now().Add(-time.Hour).Unix() // 1 hora no passado
	token := GeneratePlayToken("secret", "vid-exp", expires)

	if err := ValidatePlayToken("secret", "vid-exp", expires, token, 6*time.Hour); err == nil {
		t.Error("token expirado deveria retornar erro")
	}
}

func TestValidatePlayToken_ExceedsMaxTTL(t *testing.T) {
	// Token com expiração muito no futuro (além do TTL máximo) deve ser rejeitado.
	expires := time.Now().Add(100 * time.Hour).Unix() // 100 horas no futuro
	token := GeneratePlayToken("secret", "vid-ttl", expires)

	// TTL máximo é 6 horas
	if err := ValidatePlayToken("secret", "vid-ttl", expires, token, 6*time.Hour); err == nil {
		t.Error("token com TTL excedido deveria retornar erro")
	}
}

func TestValidatePlayToken_Tampered(t *testing.T) {
	// Token com qualquer byte modificado deve ser rejeitado.
	expires := time.Now().Add(time.Hour).Unix()
	token := GeneratePlayToken("secret", "vid-tamp", expires)

	// Modifica o último caractere do token
	if len(token) == 0 {
		t.Skip("token vazio — não é possível adulterar")
	}
	tampered := token[:len(token)-1] + "x"
	if tampered == token {
		tampered = token[:len(token)-1] + "y"
	}

	if err := ValidatePlayToken("secret", "vid-tamp", expires, tampered, 6*time.Hour); err == nil {
		t.Error("token adulterado deveria ser rejeitado")
	}
}

func TestValidateBackendAuth_Valid(t *testing.T) {
	// Assinatura gerada e validada com mesmo body e secret deve ser aceita.
	body := []byte(`{"video_id":"abc","declared_size_bytes":1024}`)
	sig := SignBackendRequest("backend-secret", body)

	if !ValidateBackendAuth("backend-secret", body, sig) {
		t.Error("assinatura backend válida deveria ser aceita")
	}
}

func TestValidateBackendAuth_Invalid(t *testing.T) {
	// Assinatura com body diferente do assinado deve ser rejeitada.
	body := []byte(`{"video_id":"abc","declared_size_bytes":1024}`)
	sig := SignBackendRequest("backend-secret", body)

	differentBody := []byte(`{"video_id":"xyz","declared_size_bytes":999}`)
	if ValidateBackendAuth("backend-secret", differentBody, sig) {
		t.Error("assinatura deveria ser rejeitada para body diferente")
	}
}

func TestTimingConstant_SignaturesNotEmpty(t *testing.T) {
	// Teste documental: verifica que as funções de assinatura retornam valores não-vazios.
	// A garantia de tempo constante está na implementação (uso de hmac.Equal).
	body := []byte("test-body")
	sig := SignBackendRequest("secret", body)
	if sig == "" {
		t.Error("SignBackendRequest não deve retornar string vazia")
	}

	token := GenerateUploadToken("secret", "vid")
	if token == "" {
		t.Error("GenerateUploadToken não deve retornar string vazia")
	}

	playToken := GeneratePlayToken("secret", "vid", time.Now().Add(time.Hour).Unix())
	if playToken == "" {
		t.Error("GeneratePlayToken não deve retornar string vazia")
	}
}
