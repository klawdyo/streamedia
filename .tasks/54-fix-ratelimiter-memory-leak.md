# T54: Corrigir memory leak no rate limiter — `sync.Map` nunca limpa IPs inativos

**Status:** pending
**Dependências:** T19
**Estimativa:** pequena
**Origem:** análise estática do código — memory leak identificado durante revisão geral

## Contexto

O middleware de rate limiting (`internal/middleware/ratelimit.go:15`)
usa um `sync.Map` (`limiters`) que mapeia `string` (IP) → `*rate.Limiter`:

```go
func (rl *RateLimiter) getLimiter(ip string) *rate.Limiter {
    limiter, _ := rl.limiters.LoadOrStore(ip, rate.NewLimiter(rl.rate, rl.burst))
    return limiter.(*rate.Limiter)
}
```

O `LoadOrStore` cria um limiter novo para cada IP que acessa o servidor,
mas **nunca remove entradas**. Com o tempo, IPs efêmeros (que acessaram
uma vez e nunca mais voltarão) acumulam no map indefinidamente, causando
crescimento linear de memória ao longo dos dias/semanas de operação.

Não há goroutine de limpeza periódica, nem uso de `Delete`, nem TTL por
entrada. Para um serviço público com milhões de IPs únicos ao longo de
meses, isso é um memory leak real.

Adicionalmente, a mensagem de erro do rate limiter está em **inglês**
(`"rate limit exceeded"`), violando a convenção do projeto ("mensagens
de erro da API em português" — `.agents/dev.md:59-67`).

## QA Instructions

Crie/estenda `internal/middleware/ratelimit_test.go`:

```
TestRateLimiter_ExpiredEntriesAreCleaned
  - Cria um RateLimiter com um TTL configurável (ex.: 1ms para teste)
  - Simula requisições de múltiplos IPs distintos
  - Aguarda o TTL expirar + o ciclo de limpeza rodar
  - Verifica que os limiters dos IPs expirados foram removidos
  - Verifica que o número de entradas no map não cresce indefinidamente
    com IPs únicos ao longo do tempo

TestRateLimiter_ActiveEntriesAreNotCleaned
  - Simula requisições do mesmo IP com intervalo menor que o TTL
  - Verifica que o limiter desse IP NÃO é removido entre as requisições

TestRateLimitErrorMessage_IsPortuguese
  - Dispara o rate limiter (envia requisições até estourar)
  - Verifica que o corpo da resposta contém mensagem em português
  - Verifica que a mensagem NÃO contém "rate limit exceeded" literal
```

## Dev Instructions

### 1. Adicionar mecanismo de expiração ao `RateLimiter`

Opções (escolha a mais simples que resolva, documente a decisão na
seção Resolução):

**Opção A (recomendada):** Adicionar um campo `lastAccess` ao valor
armazenado no map (em vez de `*rate.Limiter` puro, usar uma struct
`limiterEntry` com `limiter *rate.Limiter` + `lastAccess int64`
atualizado atomicamente a cada `Allow()`), e uma goroutine de
background (`Start()`/`Stop()`) que periodicamente varre o map e
remove entradas cujo `lastAccess` exceda um TTL configurável.

**Opção B:** Trocar `sync.Map` por um map comum protegido por
`sync.RWMutex`, com um `time.Ticker` de limpeza periódica.

**Opção C:** Usar um cache com TTL do ecossistema Go
(ex.: `github.com/patrickmn/go-cache`), mas isso adiciona dependência
externa — pese contra a filosofia de "biblioteca padrão para tudo
possível" (`.agents/dev.md:76`).

Independente da opção:
- O comportamento de rate limiting deve ser **idêntico** ao atual para
  IPs ativos — a limpeza não pode afetar a precisão do limite.
- A goroutine de limpeza deve ser iniciada/parada junto com o
  RateLimiter (adicionar métodos `Start()`/`Stop()`, chamados em
  `main.go` ou no `server.NewRouter` — avalie e documente).
- O TTL de inatividade deve ser configurável (sugestão: variável de
  ambiente `RATE_LIMITER_ENTRY_TTL_SECONDS`, default razoável como
  3600 = 1 hora).

### 2. Corrigir mensagem de erro para português

Em `internal/middleware/ratelimit.go:79`, trocar:
```go
"error": "rate limit exceeded"
```
por:
```go
"error": "Limite de requisições excedido. Tente novamente em 60 segundos."
```

### 3. Verificação

- `go test ./internal/middleware/... -v -count=1` — testes novos e
  existentes passam
- `go test ./... -race` — sem data races (o map agora tem concorrência
  entre o middleware e a goroutine de limpeza)
- `go vet ./...` — sem warnings

## Arquivos a editar

- `internal/middleware/ratelimit.go` (adicionar limpeza + corrigir mensagem)
- `internal/middleware/ratelimit_test.go` (adicionar testes do QA)
- Possivelmente `cmd/server/main.go` ou `internal/server/server.go`
  (se o RateLimiter passar a ter `Start()`/`Stop()`)

## Resolução

<!-- Preencher ao concluir: qual opção foi escolhida e por quê -->

## Definition of Done

- [ ] RateLimiter remove entradas expiradas periodicamente (memory leak
      corrigido)
- [ ] Comportamento de rate limiting para IPs ativos inalterado
- [ ] Mensagem de erro "rate limit exceeded" substituída por texto em
      português
- [ ] Testes novos comprovam remoção de entradas expiradas e preservação
      de entradas ativas
- [ ] `go test ./... -race` passa sem data races
- [ ] `go vet ./...` sem warnings
