package auth

import "testing"

func TestGenerateToken_NonEmptyAndRandom(t *testing.T) {
	a, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken falhou: %v", err)
	}
	if len(a) != 64 { // 32 bytes em hex
		t.Errorf("token deveria ter 64 chars hex, obteve %d", len(a))
	}
	b, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken falhou: %v", err)
	}
	if a == b {
		t.Error("dois tokens gerados não deveriam ser iguais (aleatoriedade)")
	}
}

func TestSecureCompare(t *testing.T) {
	if !SecureCompare("abc123", "abc123") {
		t.Error("SecureCompare deveria retornar true para strings iguais")
	}
	if SecureCompare("abc123", "abc124") {
		t.Error("SecureCompare deveria retornar false para strings diferentes")
	}
	if SecureCompare("abc", "abcd") {
		t.Error("SecureCompare deveria retornar false para tamanhos diferentes")
	}
	if SecureCompare("", "x") {
		t.Error("SecureCompare deveria retornar false quando uma é vazia")
	}
}

func TestSignWebhook_DeterministicAndKeyed(t *testing.T) {
	body := []byte(`{"event":"ready"}`)
	s1 := SignWebhook("secret", body)
	s2 := SignWebhook("secret", body)
	if s1 != s2 {
		t.Error("SignWebhook deve ser determinístico para mesmo secret+body")
	}
	if SignWebhook("outro-secret", body) == s1 {
		t.Error("SignWebhook com secret diferente deve produzir assinatura diferente")
	}
	if s1 == "" {
		t.Error("SignWebhook não deve retornar string vazia")
	}
}
