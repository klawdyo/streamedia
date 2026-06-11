package auth

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

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

// TestIssueAndValidateSessionToken verifica o ciclo completo: um token
// emitido com IssueSessionToken deve ser aceito por ValidateSessionToken
// com o mesmo ROOT_TOKEN.
func TestIssueAndValidateSessionToken(t *testing.T) {
	value, expiresAt := IssueSessionToken("root-token-123", time.Hour)
	if value == "" {
		t.Fatal("IssueSessionToken não deveria retornar valor vazio")
	}
	if !strings.Contains(value, ".") {
		t.Errorf("valor do token deveria conter '.': %q", value)
	}
	if expiresAt.Before(time.Now()) {
		t.Errorf("expiresAt deveria estar no futuro, obteve %v", expiresAt)
	}
	if !ValidateSessionToken("root-token-123", value) {
		t.Error("ValidateSessionToken deveria aceitar um token recém-emitido com o mesmo ROOT_TOKEN")
	}
}

// TestValidateSessionToken_Expired verifica que um token com prazo de
// expiração no passado é rejeitado, mesmo com o HMAC correto.
func TestValidateSessionToken_Expired(t *testing.T) {
	value, _ := IssueSessionToken("root-token-123", -time.Hour)
	if ValidateSessionToken("root-token-123", value) {
		t.Error("ValidateSessionToken deveria rejeitar um token expirado")
	}
}

// TestValidateSessionToken_WrongSecret verifica que um token emitido com um
// ROOT_TOKEN não é aceito ao validar com outro ROOT_TOKEN (ex.: a credencial
// foi trocada).
func TestValidateSessionToken_WrongSecret(t *testing.T) {
	value, _ := IssueSessionToken("root-token-123", time.Hour)
	if ValidateSessionToken("outro-root-token", value) {
		t.Error("ValidateSessionToken deveria rejeitar um token assinado com outro ROOT_TOKEN")
	}
}

// TestValidateSessionToken_TamperedExpiry verifica que adulterar o prazo de
// expiração (sem recalcular o HMAC) invalida o token.
func TestValidateSessionToken_TamperedExpiry(t *testing.T) {
	value, expiresAt := IssueSessionToken("root-token-123", time.Hour)
	_, mac, _ := strings.Cut(value, ".")

	futureExp := strconv.FormatInt(expiresAt.Add(24*time.Hour).Unix(), 10)
	tamperedValue := futureExp + "." + mac
	if ValidateSessionToken("root-token-123", tamperedValue) {
		t.Error("ValidateSessionToken deveria rejeitar um token com prazo adulterado (HMAC não confere)")
	}
}

// TestValidateSessionToken_MalformedValue verifica que valores em formato
// inválido (sem ponto, vazio, etc.) são rejeitados sem panic.
func TestValidateSessionToken_MalformedValue(t *testing.T) {
	cases := []string{"", "semponto", ".", "123.", ".abc", "abc.def.ghi"}
	for _, c := range cases {
		if ValidateSessionToken("root-token-123", c) {
			t.Errorf("ValidateSessionToken(%q) deveria retornar false", c)
		}
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
