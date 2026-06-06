# T13: Rota GET /api/status/{video_id}

**Status:** pending
**Dependências:** T04
**Estimativa:** pequena

## Contexto

Rota de consulta do status atual de um vídeo. Usada para:
- Debug e monitoramento
- Reconciliação pelo backend principal quando o webhook falhou

### Autenticação

Protegida por HMAC backend-to-backend (mesmo mecanismo do `/upload/init`).
O backend principal assina a requisição com `UPLOAD_TOKEN_SECRET`.

Header: `X-Status-Auth: {HMAC-SHA256(UPLOAD_TOKEN_SECRET, video_id)}`

### Resposta

```json
{
  "video_id": "uuid-v4",
  "status": "ready",
  "duration_s": 47,
  "resolutions": [480, 720, 1080],
  "transcode_attempts": 1,
  "error_message": null,
  "created_at": "2026-06-05T10:00:00Z",
  "updated_at": "2026-06-05T10:05:00Z"
}
```

### Respostas HTTP

- `200 OK` + JSON acima
- `400 Bad Request` — video_id não é UUID v4
- `401 Unauthorized` — HMAC inválido
- `404 Not Found` — vídeo não existe

## QA Instructions

Crie `internal/serve/status_test.go` (mesmo pacote do T12):

```
TestStatusRoute_ValidRequest
  - Insere vídeo com status ready no banco
  - GET /api/status/{video_id} com HMAC correto
  - Espera 200 com JSON completo
  - Verifica campos: video_id, status, created_at

TestStatusRoute_InvalidAuth
  - HMAC errado ou ausente
  - Espera 401

TestStatusRoute_NotFound
  - video_id válido mas não existe no banco
  - Espera 404

TestStatusRoute_InvalidVideoID
  - video_id = "nao-e-uuid"
  - Espera 400

TestStatusRoute_ResponseFields
  - Vídeo com status failed_transcode e error_message
  - Verifica que error_message aparece no JSON
  - Verifica que resolutions é null para vídeo que não chegou em ready

TestStatusRoute_ResolutionsDeserialized
  - Vídeo com status ready e resolutions [480, 720]
  - Verifica que JSON retorna array numérico [480, 720]
```

## Dev Instructions

Crie `internal/serve/status.go`:

### Handler StatusHandler

```go
type StatusHandler struct {
    cfg *config.Config
    db  *sql.DB
}

func NewStatusHandler(cfg *config.Config, db *sql.DB) *StatusHandler

func (h *StatusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request)
```

Fluxo:
1. Extrai `video_id` do path via chi
2. Valida UUID v4
3. Valida HMAC: `auth.ValidateBackendAuth(cfg.UploadTokenSecret, []byte(videoID), r.Header.Get("X-Status-Auth"))`
4. Busca vídeo no banco
5. Responde com JSON

### Struct de resposta

```go
type StatusResponse struct {
    VideoID           string    `json:"video_id"`
    Status            string    `json:"status"`
    DurationS         *int      `json:"duration_s"`
    Resolutions       []int     `json:"resolutions"`
    TranscodeAttempts int       `json:"transcode_attempts"`
    ErrorMessage      *string   `json:"error_message"`
    CreatedAt         time.Time `json:"created_at"`
    UpdatedAt         time.Time `json:"updated_at"`
}
```

Use ponteiros para campos que podem ser nulos (`DurationS`, `ErrorMessage`).

## Arquivos a criar/modificar

- `internal/serve/status.go`
- `internal/serve/status_test.go`

## Definition of Done

- [ ] Autenticação HMAC funciona
- [ ] 404 para vídeo inexistente
- [ ] JSON com todos os campos, nulos corretos
- [ ] Todos os testes passam
