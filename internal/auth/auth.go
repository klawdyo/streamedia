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
	"strconv"
	"strings"
	"time"
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

// IssueSessionToken gera o valor de um cookie de sessão de navegador
// (streamedia_session), no formato "<exp_unix>.<hmac_hex>". O HMAC é
// calculado com o próprio ROOT_TOKEN como chave — não exige nenhum segredo
// novo, e o token é stateless: qualquer instância do servidor consegue
// validá-lo apenas com o ROOT_TOKEN, sem consultar banco de dados. Emitido
// por POST /admin/session após validar Authorization: Bearer <ROOT_TOKEN>.
func IssueSessionToken(rootToken string, ttl time.Duration) (value string, expiresAt time.Time) {
	expiresAt = time.Now().Add(ttl)
	exp := strconv.FormatInt(expiresAt.Unix(), 10)
	return exp + "." + sessionTokenMAC(rootToken, exp), expiresAt
}

// ValidateSessionToken verifica um valor de cookie streamedia_session emitido
// por IssueSessionToken: confere o formato "<exp_unix>.<hmac_hex>", recalcula
// o HMAC com o ROOT_TOKEN atual (SecureCompare, tempo constante) e checa que
// o prazo de expiração ainda não passou.
func ValidateSessionToken(rootToken, value string) bool {
	exp, mac, ok := strings.Cut(value, ".")
	if !ok || exp == "" || mac == "" {
		return false
	}
	if !SecureCompare(mac, sessionTokenMAC(rootToken, exp)) {
		return false
	}
	expUnix, err := strconv.ParseInt(exp, 10, 64)
	if err != nil {
		return false
	}
	return time.Now().Unix() <= expUnix
}

// sessionTokenMAC calcula o HMAC-SHA256 (hex) de payload usando o ROOT_TOKEN
// como chave — função auxiliar compartilhada por IssueSessionToken e
// ValidateSessionToken.
func sessionTokenMAC(rootToken, payload string) string {
	mac := hmac.New(sha256.New, []byte(rootToken))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

// SignWebhook assina o corpo de um webhook com HMAC-SHA256 (hex), usando o
// WEBHOOK_SECRET. É o único segredo compartilhado com o backend principal: o
// outro lado valida a assinatura recalculando o HMAC com o mesmo segredo.
func SignWebhook(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
