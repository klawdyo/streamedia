# T06: Pacote de autenticação HMAC

**Status:** pending
**Dependências:** T02
**Estimativa:** pequena

## Contexto

Há três tipos de operação HMAC no sistema, todas usando HMAC-SHA256:

1. **Token de upload:** gerado pelo media server em `/upload/init`, vincula um
   token a um `video_id`. Formato: `HMAC-SHA256(UPLOAD_TOKEN_SECRET, video_id)`

2. **Token de reprodução:** gerado pelo BACKEND PRINCIPAL (não por este serviço),
   validado aqui. Formato: `HMAC-SHA256(secret, video_id + ":" + expires_unix_str)`
   URL: `/videos/{video_id}/master.m3u8?expires={unix}&token={hex}`

3. **Autorização backend-to-backend:** o backend principal assina a requisição
   para `/upload/init`. O header `X-Upload-Auth` contém o HMAC do body JSON.
   Formato: `HMAC-SHA256(UPLOAD_TOKEN_SECRET, body_bytes)`

**REGRA DE SEGURANÇA CRÍTICA:** Toda comparação de HMAC deve usar `hmac.Equal`
(tempo constante), NUNCA comparação com `==` de strings. Isso previne timing attacks.

## QA Instructions

Crie `internal/auth/auth_test.go`:

```
TestGenerateUploadToken_Deterministic
  - Chama GenerateUploadToken("secret", "video-id") duas vezes
  - Verifica que os resultados são iguais (determinístico)

TestGenerateUploadToken_DifferentInputs
  - GenerateUploadToken("secret", "video-id-1") !=
    GenerateUploadToken("secret", "video-id-2")
  - Verifica que inputs diferentes geram tokens diferentes

TestValidatePlayToken_Valid
  - Gera token com GeneratePlayToken("secret", "video-id", expiresUnix)
  - Valida com ValidatePlayToken("secret", "video-id", expiresUnix, token)
  - Espera true, nil

TestValidatePlayToken_WrongSecret
  - Gera token com secret "A"
  - Valida com secret "B"
  - Espera false (HMAC inválido)

TestValidatePlayToken_Expired
  - Gera token com expires = time.Now().Add(-1*time.Hour).Unix()
  - Valida
  - Espera erro de token expirado

TestValidatePlayToken_ExceedsMaxTTL
  - Gera token com expires = time.Now().Add(100*time.Hour).Unix()
  - Valida com maxTTL = 6 * time.Hour
  - Espera erro de TTL excedido

TestValidatePlayToken_Tampered
  - Gera token válido
  - Modifica 1 byte do token
  - Valida
  - Espera false

TestValidateBackendAuth_Valid
  - Gera assinatura com SignBackendRequest("secret", bodyBytes)
  - Valida com ValidateBackendAuth("secret", bodyBytes, signature)
  - Espera true

TestValidateBackendAuth_Invalid
  - Body diferente do assinado
  - Espera false

TestTimingConstant_UsesHMACEqual
  - Teste documental: verifica que o código usa hmac.Equal
  - Pode ser feito com grep ou inspeção de código
  - (basta compilar — é verificação de code review)
```

## Dev Instructions

Crie `internal/auth/auth.go`:

### Funções de token de upload

```go
// GenerateUploadToken gera um token HMAC-SHA256 para autorizar o upload de um vídeo.
// O token é vinculado ao video_id: só autoriza o upload daquele vídeo específico.
func GenerateUploadToken(secret, videoID string) string

// ValidateUploadToken verifica se o token é válido para o video_id informado.
// Usa comparação em tempo constante para prevenir timing attacks.
func ValidateUploadToken(secret, videoID, token string) bool
```

### Funções de token de reprodução

```go
// GeneratePlayToken gera o token HMAC que o backend principal usa para criar URLs assinadas.
// O token codifica o video_id e o timestamp de expiração.
// Formato do payload assinado: "{video_id}:{expires_unix}"
func GeneratePlayToken(secret, videoID string, expiresUnix int64) string

// ValidatePlayToken valida um token de reprodução recebido na URL do master.m3u8.
// Verifica: assinatura HMAC, expiração no futuro, dentro do TTL máximo permitido.
func ValidatePlayToken(secret, videoID string, expiresUnix int64, token string, maxTTL time.Duration) error
```

### Funções de autorização backend-to-backend

```go
// SignBackendRequest assina o body de uma requisição backend-to-backend.
// O backend principal usa isso para autorizar chamadas ao /upload/init.
func SignBackendRequest(secret string, body []byte) string

// ValidateBackendAuth valida a assinatura de uma requisição do backend principal.
// Compara em tempo constante.
func ValidateBackendAuth(secret string, body []byte, signature string) bool
```

### Implementação

- Use `crypto/hmac` + `crypto/sha256`
- Use `encoding/hex` para codificar os bytes do HMAC em string hexadecimal
- Use `hmac.Equal` em TODAS as comparações — nunca `==`
- Para `ValidatePlayToken`, retorne erros descritivos em português:
  - "Token de reprodução expirado."
  - "Token de reprodução excede o tempo máximo permitido."
  - "Assinatura do token de reprodução inválida."

## Arquivos a criar/modificar

- `internal/auth/auth.go`
- `internal/auth/auth_test.go`

## Definition of Done

- [ ] Três pares de funções sign/validate implementados
- [ ] Todas as comparações usam `hmac.Equal`
- [ ] Erros em português
- [ ] Todos os testes passam incluindo expiração e TTL máximo
- [ ] `go vet ./...` limpo
