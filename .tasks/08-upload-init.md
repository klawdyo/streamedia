# T08: Rota POST /upload/init

**Status:** pending
**Dependências:** T07
**Estimativa:** pequena

## Contexto

Esta rota é chamada pelo backend principal para inicializar um upload. O backend
principal já validou o usuário e gerou o UUID do vídeo no Flutter.

### Fluxo

```
1. Backend principal → POST /upload/init
   Headers: X-Upload-Auth: {HMAC do body}
   Body: { "video_id": "uuid-v4", "declared_size_bytes": 12345678 }

2. Media server valida:
   a. HMAC do body com UPLOAD_TOKEN_SECRET (autorização backend-to-backend)
   b. video_id é UUID v4 estrito
   c. video_id não existe no banco (senão 409)
   d. declared_size_bytes > 0 e <= MaxUploadSizeBytes

3. Media server:
   a. Insere vídeo com status pending_upload
   b. Gera token de upload: HMAC-SHA256(UPLOAD_TOKEN_SECRET, video_id)
   c. Grava token em upload_tokens com expiração de UPLOAD_TOKEN_TTL_H horas
   d. Retorna { "upload_url": "http://host/files/{video_id}", "token": "{token}" }

4. Backend principal repassa upload_url e token ao Flutter
5. Flutter usa o token no header Upload-Token nas requisições TUS
```

### Respostas da rota

- `200 OK` + `{ "upload_url": "...", "token": "..." }` — sucesso
- `400 Bad Request` + `{ "error": "..." }` — body inválido, UUID inválido, tamanho inválido
- `401 Unauthorized` + `{ "error": "..." }` — HMAC inválido
- `409 Conflict` + `{ "error": "..." }` — video_id já existe

## QA Instructions

Crie `internal/upload/init_test.go`:

```
TestUploadInit_Success
  - POST /upload/init com body válido e HMAC correto
  - Espera 200 com upload_url e token no body

TestUploadInit_InvalidHMAC
  - POST com HMAC errado no header X-Upload-Auth
  - Espera 401

TestUploadInit_MissingAuthHeader
  - POST sem header X-Upload-Auth
  - Espera 401

TestUploadInit_InvalidVideoID_NotUUID
  - video_id = "nao-e-um-uuid"
  - Espera 400

TestUploadInit_InvalidVideoID_PathTraversal
  - video_id = "../etc/passwd"
  - Espera 400

TestUploadInit_DuplicateVideoID
  - Insere vídeo X no banco antes do teste
  - POST com video_id X
  - Espera 409

TestUploadInit_ZeroSize
  - declared_size_bytes = 0
  - Espera 400

TestUploadInit_SizeExceedsLimit
  - declared_size_bytes > MaxUploadSizeBytes
  - Espera 400 ou 413

TestUploadInit_TokenStoredInDB
  - POST bem-sucedido
  - Busca token no banco via GetUploadTokenByVideoID
  - Verifica que token existe e expira no futuro

TestUploadInit_VideoCreatedInDB
  - POST bem-sucedido
  - Busca vídeo no banco via GetVideo
  - Verifica status == pending_upload
  - Verifica declared_size_bytes correto
```

Use `httptest.NewRecorder` e `httptest.NewServer` do pacote padrão.

## Dev Instructions

Crie `internal/upload/init.go`:

### Handler InitUpload

```go
type InitHandler struct {
    cfg *config.Config
    db  *sql.DB
}

func NewInitHandler(cfg *config.Config, db *sql.DB) *InitHandler

func (h *InitHandler) ServeHTTP(w http.ResponseWriter, r *http.Request)
```

### Implementação

1. Leia o body completo (limite de 1MB para o JSON de init)
2. Valide HMAC: `auth.ValidateBackendAuth(cfg.UploadTokenSecret, body, r.Header.Get("X-Upload-Auth"))`
3. Parse o JSON do body
4. Valide video_id com regex UUID v4:
   `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`
5. Valide declared_size_bytes > 0 e <= cfg.MaxUploadSizeBytes
6. Insira o vídeo no banco (trate UNIQUE constraint como 409)
7. Gere token: `auth.GenerateUploadToken(cfg.UploadTokenSecret, videoID)`
8. Grave token no banco com expiração `time.Now().Add(cfg.UploadTokenTTL)`
9. Construa a upload URL: `fmt.Sprintf("%s/files/%s", baseURL, videoID)`
   - A baseURL deve ser construída a partir da request: `r.Host` ou variável de config
10. Responda com JSON

### Função helper para construir baseURL

Use `r.Header.Get("X-Forwarded-Proto")` + `r.Host` para construir a URL base.
Em desenvolvimento, use `http://` se não houver o header.

### Erros em português

```go
var (
    errInvalidAuth     = "Autorização inválida."
    errInvalidVideoID  = "O identificador do vídeo é inválido."
    errVideoExists     = "O vídeo já existe e não pode ser enviado novamente."
    errInvalidSize     = "O tamanho declarado do vídeo é inválido."
    errSizeExceedsMax  = "O vídeo excede o tamanho máximo permitido."
)
```

## Arquivos a criar/modificar

- `internal/upload/init.go`
- `internal/upload/init_test.go`

## Definition of Done

- [ ] Validação HMAC backend-to-backend
- [ ] Validação UUID v4 estrito
- [ ] 409 para video_id duplicado
- [ ] Token gerado e persistido com expiração correta
- [ ] Upload URL retornada no formato correto
- [ ] Todos os testes passam
