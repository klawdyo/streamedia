# T19: Middleware de rate limiting por IP

**Status:** pending
**Dependências:** T01
**Estimativa:** pequena

## Contexto

Rate limiting por IP para proteger contra flood de requisições de upload.
O limite é `RATE_LIMIT_PER_MIN` requisições por minuto por IP.

### Implementação

Use `golang.org/x/time/rate` para o rate limiter.
Um `sync.Map` de limiters por IP (um limiter por IP).
Limpeza periódica dos limiters de IPs inativos.

### Extração do IP

- Prefira o header `X-Real-IP` (quando há proxy/nginx na frente)
- Fallback para `X-Forwarded-For` (primeiro IP)
- Fallback para `r.RemoteAddr`

### Resposta ao limite excedido

- `429 Too Many Requests`
- Header: `Retry-After: 60`
- Body: `{ "error": "Muitas requisições. Tente novamente em alguns instantes." }`

## QA Instructions

Crie `internal/middleware/ratelimit_test.go`:

```
TestRateLimit_AllowsUnderLimit
  - Configura limite de 5 req/min
  - Faz 5 requests do mesmo IP
  - Todas devem retornar o status do handler (200, não 429)

TestRateLimit_BlocksOverLimit
  - Configura limite de 3 req/min
  - Faz 4 requests do mesmo IP
  - A 4a deve retornar 429

TestRateLimit_DifferentIPsAreIndependent
  - Configura limite de 2 req/min
  - IP-A faz 2 requests (limite atingido)
  - IP-B faz 2 requests (não deve ser bloqueado por conta do IP-A)

TestRateLimit_HeaderRetryAfter
  - Request bloqueada por rate limit
  - Verifica header Retry-After: 60

TestRateLimit_ExtractsRealIP
  - Request com header X-Real-IP: 10.0.0.1
  - Verifica que o limiter usa 10.0.0.1 e não o RemoteAddr

TestRateLimit_ResponseJSON
  - Request bloqueada
  - Verifica que body é JSON com campo "error"
```

## Dev Instructions

Crie `internal/middleware/ratelimit.go`:

### Struct RateLimiter

```go
type RateLimiter struct {
    limiters sync.Map  // IP → *rate.Limiter
    rate     rate.Limit
    burst    int
}

func NewRateLimiter(perMin int) *RateLimiter
```

- `rate.Limit` é o número de tokens por segundo: `rate.Limit(perMin) / 60`
- `burst` = `perMin` (permite rajadas iguais ao limite por minuto)

### Middleware

```go
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler
```

Extrai IP, obtém ou cria limiter para o IP, verifica com `limiter.Allow()`.

### Limpeza periódica

Adicione goroutine que a cada 1 minuto limpa limiters de IPs que não fazem
requests há mais de 5 minutos. Use `sync.Map.Range` + `LoadOrDelete`.

### Função extractIP

```go
func extractIP(r *http.Request) string
```

Ordem de prioridade:
1. `X-Real-IP`
2. Primeiro valor de `X-Forwarded-For` (split por `,`)
3. `r.RemoteAddr` (removendo a porta via `net.SplitHostPort`)

## Arquivos a criar/modificar

- `internal/middleware/ratelimit.go`
- `internal/middleware/ratelimit_test.go`

## Definition of Done

- [ ] Rate limiting por IP funcional
- [ ] IPs diferentes são independentes
- [ ] 429 com Retry-After quando limite excedido
- [ ] X-Real-IP e X-Forwarded-For respeitados
- [ ] Todos os testes passam
