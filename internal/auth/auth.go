// Pacote auth reúne os utilitários de credenciais do sistema.
//
// No modelo de TAG + ROOT_TOKEN único, não há mais tokens HMAC determinísticos:
//   - O ROOT_TOKEN é comparado em tempo constante (SecureCompare).
//   - Os tokens efêmeros de upload/play são strings aleatórias opacas
//     (GenerateToken), persistidas em access_tokens e validadas por lookup
//     no banco (ver internal/models/token.go) — sem segredo de assinatura.
//
// REGRA DE SEGURANÇA: comparações de credencial usam subtle.ConstantTimeCompare
// (tempo constante) para prevenir timing attacks. Nunca use == para comparar.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
)

// GenerateToken gera um token opaco aleatório (32 bytes, hex = 64 chars).
// Usado para os tokens efêmeros de upload e de play: o valor não carrega
// significado — a autorização vem da linha correspondente em access_tokens.
func GenerateToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("erro ao gerar token aleatório: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// SecureCompare compara duas strings em tempo constante. Usada para validar
// o ROOT_TOKEN apresentado em Authorization: Bearer.
func SecureCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// SignWebhook assina o corpo de um webhook com HMAC-SHA256 (hex), usando o
// WEBHOOK_SECRET. É o único segredo compartilhado com o backend principal: o
// outro lado valida a assinatura recalculando o HMAC com o mesmo segredo.
func SignWebhook(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
