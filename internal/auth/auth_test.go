package auth

import (
	"strings"
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

// TestValidateUploadToken_TableDriven testa table-driven casos (válido, inválido, malformado, etc.)
func TestValidateUploadToken_TableDriven(t *testing.T) {
	const secret = "secret-upload"
	const videoID = "video-123"
	validToken := GenerateUploadToken(secret, videoID)

	cases := []struct {
		name      string
		secret    string
		videoID   string
		token     string
		expected  bool
		desc      string
	}{
		{
			name:     "valid_token",
			secret:   secret,
			videoID:  videoID,
			token:    validToken,
			expected: true,
			desc:     "token válido com secret e video_id corretos",
		},
		{
			name:     "wrong_secret",
			secret:   "wrong-secret",
			videoID:  videoID,
			token:    validToken,
			expected: false,
			desc:     "token gerado com secret diferente deve ser rejeitado",
		},
		{
			name:     "wrong_videoid",
			secret:   secret,
			videoID:  "video-999",
			token:    validToken,
			expected: false,
			desc:     "token gerado com video_id diferente deve ser rejeitado",
		},
		{
			name:     "tampered_token_single_char",
			secret:   secret,
			videoID:  videoID,
			token:    tamperToken(validToken, 0, 'x'),
			expected: false,
			desc:     "token modificado em 1 byte deve ser rejeitado",
		},
		{
			name:     "tampered_token_middle",
			secret:   secret,
			videoID:  videoID,
			token:    tamperToken(validToken, len(validToken)/2, 'y'),
			expected: false,
			desc:     "token modificado no meio deve ser rejeitado",
		},
		{
			name:     "empty_token",
			secret:   secret,
			videoID:  videoID,
			token:    "",
			expected: false,
			desc:     "token vazio deve ser rejeitado",
		},
		{
			name:     "malformed_hex",
			secret:   secret,
			videoID:  videoID,
			token:    "not-valid-hex-zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz",
			expected: false,
			desc:     "token com hex inválido deve ser rejeitado",
		},
		{
			name:     "odd_length_hex",
			secret:   secret,
			videoID:  videoID,
			token:    validToken[:len(validToken)-1],
			expected: false,
			desc:     "token com comprimento hex ímpar deve ser rejeitado",
		},
		{
			name:     "case_insensitive_hex",
			secret:   secret,
			videoID:  videoID,
			token:    strings.ToUpper(validToken[:20]) + strings.ToLower(validToken[20:]),
			expected: true,
			desc:     "hex case-insensitive deve ser aceito",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := ValidateUploadToken(tc.secret, tc.videoID, tc.token)
			if result != tc.expected {
				t.Errorf("%s: esperado %v, obtido %v", tc.desc, tc.expected, result)
			}
		})
	}
}

// tamperToken modifica um token em uma posição específica (helper para testes)
func tamperToken(token string, pos int, newChar byte) string {
	if pos >= len(token) {
		return token
	}
	runes := []rune(token)
	if newChar > 0x9 && newChar < 0xa {
		newChar = byte('x')
	}
	runes[pos] = rune(newChar)
	return string(runes)
}

// TestValidatePlayToken_MalformedTimestamp testa tokens com timestamp inválido/edge cases
func TestValidatePlayToken_MalformedTimestamp(t *testing.T) {
	cases := []struct {
		name     string
		expiresUnix int64
		desc     string
		shouldPass bool
	}{
		{
			name:        "zero_timestamp",
			expiresUnix: 0,
			desc:        "timestamp zero (1970-01-01) — deve ser rejeitado como expirado",
			shouldPass:  false,
		},
		{
			name:        "negative_timestamp",
			expiresUnix: -1000,
			desc:        "timestamp negativo — deve ser rejeitado como expirado",
			shouldPass:  false,
		},
		{
			name:        "far_future",
			expiresUnix: time.Now().Add(100 * time.Hour).Unix(),
			desc:        "timestamp muito no futuro — deve ser rejeitado por exceder maxTTL",
			shouldPass:  false,
		},
		{
			name:        "just_expired",
			expiresUnix: time.Now().Add(-1 * time.Second).Unix(),
			desc:        "timestamp expirado por 1 segundo — deve ser rejeitado",
			shouldPass:  false,
		},
		{
			name:        "valid_soon",
			expiresUnix: time.Now().Add(1 * time.Hour).Unix(),
			desc:        "timestamp válido em 1 hora — deve ser aceito",
			shouldPass:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			token := GeneratePlayToken("secret", "vid-ts", tc.expiresUnix)
			err := ValidatePlayToken("secret", "vid-ts", tc.expiresUnix, token, 6*time.Hour)
			if tc.shouldPass && err != nil {
				t.Errorf("%s: esperava aceitar, mas retornou erro: %v", tc.desc, err)
			} else if !tc.shouldPass && err == nil {
				t.Errorf("%s: esperava rejeitar, mas foi aceito", tc.desc)
			}
		})
	}
}

// TestValidatePlayToken_InvalidHexInToken testa tokens com conteúdo hex malformado
func TestValidatePlayToken_InvalidHexInToken(t *testing.T) {
	expires := time.Now().Add(time.Hour).Unix()
	validToken := GeneratePlayToken("secret", "vid", expires)

	cases := []struct {
		name  string
		token string
		desc  string
	}{
		{
			name:  "empty_token",
			token: "",
			desc:  "token vazio deve ser rejeitado",
		},
		{
			name:  "invalid_hex_chars",
			token: "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz",
			desc:  "token com caracteres hex inválidos (z) deve ser rejeitado",
		},
		{
			name:  "truncated_token",
			token: validToken[:len(validToken)/2],
			desc:  "token truncado no meio deve ser rejeitado",
		},
		{
			name:  "odd_length_token",
			token: validToken[:len(validToken)-1],
			desc:  "token com número ímpar de caracteres hex deve ser rejeitado",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePlayToken("secret", "vid", expires, tc.token, 6*time.Hour)
			if err == nil {
				t.Errorf("%s: esperava erro, mas foi aceito", tc.desc)
			}
		})
	}
}

// TestValidateBackendAuth_Malformed testa assinatura com conteúdo malformado
func TestValidateBackendAuth_Malformed(t *testing.T) {
	cases := []struct {
		name      string
		secret    string
		body      []byte
		signature string
		expected  bool
		desc      string
	}{
		{
			name:      "empty_signature",
			secret:    "secret",
			body:      []byte("body"),
			signature: "",
			expected:  false,
			desc:      "assinatura vazia deve ser rejeitada",
		},
		{
			name:      "invalid_hex_signature",
			secret:    "secret",
			body:      []byte("body"),
			signature: "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz",
			expected:  false,
			desc:      "assinatura com hex inválido deve ser rejeitada",
		},
		{
			name:      "odd_length_signature",
			secret:    "secret",
			body:      []byte("body"),
			signature: "abc",
			expected:  false,
			desc:      "assinatura com número ímpar de caracteres deve ser rejeitada",
		},
		{
			name:      "short_signature",
			secret:    "secret",
			body:      []byte("body"),
			signature: "abcd",
			expected:  false,
			desc:      "assinatura muito curta deve ser rejeitada",
		},
		{
			name:      "empty_body",
			secret:    "secret",
			body:      []byte{},
			signature: SignBackendRequest("secret", []byte{}),
			expected:  true,
			desc:      "corpo vazio com assinatura válida deve ser aceito",
		},
		{
			name:      "empty_secret",
			secret:    "",
			body:      []byte("body"),
			signature: SignBackendRequest("", []byte("body")),
			expected:  true,
			desc:      "secret vazio com assinatura válida deve ser aceito",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := ValidateBackendAuth(tc.secret, tc.body, tc.signature)
			if result != tc.expected {
				t.Errorf("%s: esperado %v, obtido %v", tc.desc, tc.expected, result)
			}
		})
	}
}

// TestGeneratePlayToken_EdgeCaseExpirations testa geradores de token com timestamps extremos
func TestGeneratePlayToken_EdgeCaseExpirations(t *testing.T) {
	cases := []struct {
		name        string
		expiresUnix int64
		desc        string
	}{
		{
			name:        "unix_epoch",
			expiresUnix: 0,
			desc:        "timestamp no epoch (1970-01-01)",
		},
		{
			name:        "negative",
			expiresUnix: -86400,
			desc:        "timestamp negativo (antes de 1970)",
		},
		{
			name:        "far_future",
			expiresUnix: 9999999999,
			desc:        "timestamp distante no futuro",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			token := GeneratePlayToken("secret", "vid", tc.expiresUnix)
			// Token deve ser gerado sem panic, mesmo com timestamps extremos
			if token == "" {
				t.Errorf("%s: gerador retornou token vazio", tc.desc)
			}
			// Deve ser determinístico: mesmo timestamp e entrada geram mesmo token
			token2 := GeneratePlayToken("secret", "vid", tc.expiresUnix)
			if token != token2 {
				t.Errorf("%s: token não é determinístico", tc.desc)
			}
		})
	}
}

// TestValidatePlayToken_ErrorMessagesAreGeneric verifica que mensagens de erro
// de play token não distinguem entre token expirado, TTL excessivo e assinatura
// inválida — prevenindo enumeração de tokens (T41, F-01).
func TestValidatePlayToken_ErrorMessagesAreGeneric(t *testing.T) {
	secret := "secret-test"
	videoID := "vid-abc"

	// Caso 1: token expirado (expires no passado)
	pastExpires := time.Now().Add(-1 * time.Hour).Unix()
	pastToken := GeneratePlayToken(secret, videoID, pastExpires)
	errExpired := ValidatePlayToken(secret, videoID, pastExpires, pastToken, 6*time.Hour)
	if errExpired == nil {
		t.Fatal("token expirado deveria retornar erro")
	}
	if !strings.Contains(errExpired.Error(), "inválido") {
		t.Errorf("mensagem de token expirado deve ser genérica (conter 'inválido'), obtido: %q", errExpired.Error())
	}

	// Caso 2: TTL excessivo (expires muito no futuro)
	farFuture := time.Now().Add(24 * time.Hour).Unix()
	farToken := GeneratePlayToken(secret, videoID, farFuture)
	errTTL := ValidatePlayToken(secret, videoID, farFuture, farToken, 1*time.Hour)
	if errTTL == nil {
		t.Fatal("token com TTL excessivo deveria retornar erro")
	}
	if !strings.Contains(errTTL.Error(), "inválido") {
		t.Errorf("mensagem de TTL excessivo deve ser genérica (conter 'inválido'), obtido: %q", errTTL.Error())
	}

	// Caso 3: assinatura HMAC inválida (secret ou video_id diferente)
	errInvalid := ValidatePlayToken(secret, "video-diferente", time.Now().Add(1*time.Hour).Unix(), farToken, 6*time.Hour)
	if errInvalid == nil {
		t.Fatal("token com video_id diferente deveria retornar erro")
	}
	if !strings.Contains(errInvalid.Error(), "inválido") {
		t.Errorf("mensagem de assinatura inválida deve ser genérica (conter 'inválido'), obtido: %q", errInvalid.Error())
	}

	// Caso 4: token vazio ou malformado
	errEmpty := ValidatePlayToken(secret, videoID, time.Now().Add(1*time.Hour).Unix(), "", 6*time.Hour)
	if errEmpty == nil {
		t.Fatal("token vazio deveria retornar erro")
	}
	if !strings.Contains(errEmpty.Error(), "inválido") {
		t.Errorf("mensagem de token vazio deve ser genérica (conter 'inválido'), obtido: %q", errEmpty.Error())
	}
}
