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
	"context"
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

// contextKey é um tipo privado para chaves de contexto — evita colisão com
// chaves de outros pacotes.
type contextKey string

// UserIDContextKey é a chave de contexto onde o middleware RootAuth armazena
// o user_id extraído do cookie de sessão (formato novo com user).
const UserIDContextKey contextKey = "user_id"

// GetUserIDFromContext extrai o userID do contexto da requisição.
// Retorna zero value e false se não estiver presente.
func GetUserIDFromContext(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(UserIDContextKey).(int64)
	return id, ok
}

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

// IssueSessionTokenWithUser gera um cookie de sessão de navegador com
// informações do usuário autenticado via Google OAuth.
//
// Formato: <exp_unix>.<user_id>.<roles_csv>.<hmac_hex>
//
// O HMAC-SHA256 cobre a string "<exp_unix>:<user_id>:<roles_csv>", usando
// o ROOT_TOKEN como chave — mesma estratégia stateless do IssueSessionToken
// original, agora incluindo a identidade do usuário no payload.
//
// Emitido por HandleCallback (internal/auth/google) após autenticação bem-
// sucedida via Google OAuth.
func IssueSessionTokenWithUser(rootToken string, userID int64, roles []string, ttl time.Duration) (value string, expiresAt time.Time) {
	expiresAt = time.Now().Add(ttl)
	exp := strconv.FormatInt(expiresAt.Unix(), 10)
	uid := strconv.FormatInt(userID, 10)
	rolesCSV := strings.Join(roles, ",")
	payload := exp + ":" + uid + ":" + rolesCSV
	mac := sessionTokenMAC(rootToken, payload)
	return exp + "." + uid + "." + rolesCSV + "." + mac, expiresAt
}

// ValidateSessionTokenWithUser valida um cookie de sessão no formato novo
// (com user_id e roles). Retorna os campos extraídos e ok=true se o token
// for íntegro e não expirado.
//
// Se o valor tiver o formato antigo (2 partes, sem user_id), retorna
// ok=false — o chamador deve tentar ValidateSessionToken como fallback.
func ValidateSessionTokenWithUser(rootToken, value string) (expUnix int64, userID int64, rolesCSV string, ok bool) {
	parts := strings.SplitN(value, ".", 4)
	if len(parts) != 4 {
		return 0, 0, "", false
	}
	exp, uid, csv, mac := parts[0], parts[1], parts[2], parts[3]
	if exp == "" || uid == "" || csv == "" || mac == "" {
		return 0, 0, "", false
	}
	payload := exp + ":" + uid + ":" + csv
	if !SecureCompare(mac, sessionTokenMAC(rootToken, payload)) {
		return 0, 0, "", false
	}
	expUnix, err := strconv.ParseInt(exp, 10, 64)
	if err != nil {
		return 0, 0, "", false
	}
	userID, err = strconv.ParseInt(uid, 10, 64)
	if err != nil {
		return 0, 0, "", false
	}
	if time.Now().Unix() > expUnix {
		return 0, 0, "", false
	}
	return expUnix, userID, csv, true
}
