# T17: Cliente de webhook com retry

**Status:** pending
**Dependências:** T04
**Estimativa:** média

## Contexto

O media server notifica o backend principal em três eventos de estado:
- `processing`: upload completo e validado, transcode enfileirado
- `ready`: HLS pronto, vídeo pode ser servido
- `failed`: falha terminal (upload ou transcode)

### Payload JSON

```json
{
  "video_id": "uuid-v4-do-video",
  "event": "ready",
  "status": "ready",
  "duration_s": 47,
  "resolutions": [480, 720, 1080],
  "error_message": null,
  "timestamp": "2026-06-05T10:00:00Z"
}
```

Para `failed`: `status` é `failed_upload` ou `failed_transcode`, `error_message` tem o motivo.
Para `processing`: `duration_s` e `resolutions` são null (arquivo ainda não foi transcoded).

### Assinatura

```
Header: X-Signature: sha256={hex}
Valor:  HMAC-SHA256(WEBHOOK_SECRET, corpo_json_bruto)
```

### Retry

- 3 tentativas com backoff exponencial: 1s, 2s, 4s
- Timeout por tentativa: 10 segundos
- Qualquer status HTTP não-2xx é considerado falha
- Todas as tentativas são registradas em `webhook_log` com `success=0`
- Tentativa bem-sucedida registrada com `success=1`

## QA Instructions

Crie `internal/webhook/webhook_test.go`:

```
TestSend_SuccessOnFirstAttempt
  - Sobe servidor HTTP de teste que responde 200
  - Chama Send(videoID, "ready", video)
  - Verifica que exatamente 1 request foi feito
  - Verifica header X-Signature presente
  - Verifica que registro em webhook_log tem success=1

TestSend_RetryOnFailure
  - Servidor responde 500 nas 2 primeiras tentativas, 200 na 3a
  - Verifica que 3 requests foram feitos
  - Verifica success=1 no banco

TestSend_AllAttemptsFailure
  - Servidor sempre responde 500 (ou recusa conexão)
  - Verifica 3 tentativas
  - Verifica success=0 no banco
  - Retorna erro

TestSend_SignatureVerification
  - Captura o body e header X-Signature do request recebido
  - Recalcula HMAC-SHA256(WEBHOOK_SECRET, body)
  - Verifica que a assinatura bate

TestPayload_Processing
  - Chama buildPayload(videoID, "processing", nil) (sem duração/resoluções)
  - Verifica: event = "processing", duration_s = null, resolutions = null

TestPayload_Ready
  - Chama buildPayload com duração e resoluções
  - Verifica campos corretos no JSON

TestPayload_Failed
  - Chama buildPayload com error_message
  - Verifica error_message presente no JSON

TestWebhookLog_RecordsAttempts
  - Após Send com 2 falhas e 1 sucesso
  - Consulta webhook_log
  - Verifica 3 registros para o videoID
  - Verifica que o último tem success=1

TestSend_Timeout
  - Servidor demora mais de 10s para responder
  - Verifica que a tentativa é cancelada pelo timeout
```

Use `httptest.NewServer` para o servidor de teste.

## Dev Instructions

Crie `internal/webhook/webhook.go`:

### Struct WebhookPayload

```go
type WebhookPayload struct {
    VideoID      string    `json:"video_id"`
    Event        string    `json:"event"`
    Status       string    `json:"status"`
    DurationS    *int      `json:"duration_s"`
    Resolutions  []int     `json:"resolutions"`
    ErrorMessage *string   `json:"error_message"`
    Timestamp    time.Time `json:"timestamp"`
}
```

### Struct Client

```go
type Client struct {
    cfg    *config.Config
    db     *sql.DB
    http   *http.Client
}

func NewClient(cfg *config.Config, db *sql.DB) *Client
```

### Função Send

```go
func (c *Client) Send(videoID, event string, video *models.Video) error
```

Fluxo:
1. Constrói o payload JSON
2. Assina: `HMAC-SHA256(cfg.WebhookSecret, payloadBytes)` → header `X-Signature`
3. Tenta enviar até 3 vezes com backoff 1s, 2s, 4s
4. Cada tentativa: POST para `cfg.WebhookURL` com timeout de 10s
5. Registra cada tentativa em `webhook_log`
6. Retorna nil se alguma tentativa teve 2xx

### Struct WebhookLog + funções

```go
type WebhookLogEntry struct {
    ID      int64
    VideoID string
    Event   string
    Payload string
    SentAt  time.Time
    Success bool
}

func insertWebhookLog(db *sql.DB, videoID, event, payload string, success bool) error
func GetWebhookLog(db *sql.DB, videoID string) ([]*WebhookLogEntry, error)
```

### Assinatura do webhook

Use `auth.SignBackendRequest(cfg.WebhookSecret, payloadBytes)` (implementado na T06).
O formato do header é: `sha256={hex}`

## Arquivos a criar/modificar

- `internal/webhook/webhook.go`
- `internal/webhook/webhook_test.go`

## Definition of Done

- [ ] Payload JSON correto com campos nulos quando não aplicável
- [ ] Assinatura HMAC em X-Signature
- [ ] 3 tentativas com backoff exponencial
- [ ] Cada tentativa registrada no webhook_log
- [ ] Todos os testes passam
