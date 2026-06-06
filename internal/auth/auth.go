// Pacote auth implementa autenticação HMAC-SHA256 para os três tipos de token do sistema.
// REGRA CRÍTICA DE SEGURANÇA: todas as comparações de HMAC usam hmac.Equal (tempo constante)
// para prevenir timing attacks. Nunca use == para comparar tokens.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// GenerateUploadToken gera um token HMAC-SHA256 para autorizar o upload de um vídeo.
// O token é vinculado ao video_id: só autoriza o upload daquele vídeo específico.
// Mesmo secret + mesmo video_id sempre geram o mesmo token (determinístico).
func GenerateUploadToken(secret, videoID string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(videoID))
	return hex.EncodeToString(mac.Sum(nil))
}

// ValidateUploadToken verifica se o token é válido para o video_id informado.
// Usa comparação em tempo constante para prevenir timing attacks.
func ValidateUploadToken(secret, videoID, token string) bool {
	expected := GenerateUploadToken(secret, videoID)
	expectedBytes, err := hex.DecodeString(expected)
	if err != nil {
		return false
	}
	tokenBytes, err := hex.DecodeString(token)
	if err != nil {
		return false
	}
	return hmac.Equal(expectedBytes, tokenBytes)
}

// GeneratePlayToken gera o token HMAC que o backend principal usa para criar URLs assinadas.
// O payload assinado é "{video_id}:{expires_unix}" — vincula o token ao vídeo e à expiração.
func GeneratePlayToken(secret, videoID string, expiresUnix int64) string {
	payload := fmt.Sprintf("%s:%d", videoID, expiresUnix)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

// ValidatePlayToken valida um token de reprodução recebido na URL do master.m3u8.
// Verifica em ordem: expiração, TTL máximo, assinatura HMAC.
// Retorna erro descritivo em português para cada tipo de falha.
func ValidatePlayToken(secret, videoID string, expiresUnix int64, token string, maxTTL time.Duration) error {
	now := time.Now()
	expiresAt := time.Unix(expiresUnix, 0)

	// Verifica se o token já expirou
	if now.After(expiresAt) {
		return errors.New("Token de reprodução expirado.")
	}

	// Verifica se a expiração está dentro do TTL máximo permitido
	// (protege contra tokens com expiração absurdamente longa)
	maxExpires := now.Add(maxTTL)
	if expiresAt.After(maxExpires) {
		return errors.New("Token de reprodução excede o tempo máximo permitido.")
	}

	// Recalcula o HMAC esperado e compara em tempo constante
	expected := GeneratePlayToken(secret, videoID, expiresUnix)
	expectedBytes, err := hex.DecodeString(expected)
	if err != nil {
		return errors.New("Assinatura do token de reprodução inválida.")
	}
	tokenBytes, err := hex.DecodeString(token)
	if err != nil {
		return errors.New("Assinatura do token de reprodução inválida.")
	}
	if !hmac.Equal(expectedBytes, tokenBytes) {
		return errors.New("Assinatura do token de reprodução inválida.")
	}

	return nil
}

// SignBackendRequest assina o body de uma requisição backend-to-backend.
// O backend principal usa isso para autorizar chamadas ao /upload/init e /api/status.
func SignBackendRequest(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// ValidateBackendAuth valida a assinatura de uma requisição do backend principal.
// Compara em tempo constante para prevenir timing attacks.
func ValidateBackendAuth(secret string, body []byte, signature string) bool {
	expected := SignBackendRequest(secret, body)
	expectedBytes, err := hex.DecodeString(expected)
	if err != nil {
		return false
	}
	sigBytes, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	return hmac.Equal(expectedBytes, sigBytes)
}
