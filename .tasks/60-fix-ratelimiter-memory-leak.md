# T60: Corrigir memory leak no rate limiter — `sync.Map` sem eviction

**Status:** done
**Dependências:** T19
**Estimativa:** media
**Origem:** análise de código — degradação progressiva em produção
**Severidade:** alta

## Contexto

Em `internal/middleware/ratelimit.go:16`, o `RateLimiter` usa um `sync.Map`
para armazenar um `*rate.Limiter` por IP:

```go
type RateLimiter struct {
    limiters sync.Map  // string IP -> *rate.Limiter
    rate     rate.Limit
    burst    int
}
```

**Não há nenhum mecanismo de limpeza ou eviction.** Cada IP único que acessa
o servidor cria uma entrada permanente no mapa. Em produção, com tráfego
variado (crawlers, bots, IPs dinâmicos, ataques DDoS), o mapa cresce
indefinidamente.

Um `*rate.Limiter` ocupa ~80 bytes + overhead do mapa. Com 1M de IPs
distintos ao longo de semanas/meses, são ~80MB+ de memória desperdiçada
que nunca é liberada.

## Impacto

- **Crescimento de memória ilimitado** ao longo do tempo — não é crash
  imediato, mas degrada progressivamente até OOM.
- Em cenários de ataque (IP spoofing, botnets), pode ser explorado como
  vetor de DoS de memória.
- O problema é invisível em testes e deployments curtos — só se manifesta
  em produção com uptime longo.

## QA Instructions

```
TestRateLimiter_EvictsStaleEntries
  - Configura rate limiter com TTL curto para teste (ex.: 100ms)
  - Faz requisições de 100 IPs diferentes
  - Espera o dobro do TTL
  - Faz uma requisição de 1 IP novo (para acionar a limpeza, se lazy)
  - Verifica que as 100 entradas antigas foram removidas

TestRateLimiter_ActiveEntriesNotEvicted
  - Configura rate limiter com TTL
  - Faz requisições de um IP
  - Espera menos que o TTL
  - Verifica que o IP ainda está no mapa e mantém o rate state
```

## Dev Instructions

### Opção recomendada: cleanup periódico em background

Adicionar um campo `lastSeen` e um goroutine de limpeza periódica:

1. Criar wrapper que registra `lastSeen`:
```go
type limiterEntry struct {
    limiter  *rate.Limiter
    lastSeen atomic.Int64 // unix timestamp
}
```

2. No `getLimiter`, atualizar `lastSeen` a cada acesso.

3. Iniciar goroutine no `NewRateLimiter` que a cada N minutos faz
   `Range` no `sync.Map` e deleta entries com `lastSeen` mais antigo
   que o TTL (ex.: 10 minutos).

4. Adicionar canal de parada e método `Stop()` para o cleanup goroutine.

### Alternativa: usar lib pronta

`golang.org/x/sync/singleflight` + cache com TTL como
`github.com/hashicorp/golang-lru/v2` ou `github.com/dgraph-io/ristretto`.
Avaliar se vale adicionar dependência para isso.

## Arquivos a editar

- `internal/middleware/ratelimit.go` (eviction periódica)
- `internal/server/server.go` (se preciso chamar Stop no shutdown)

## Resolução

Arquivos alterados:
- `internal/middleware/ratelimit.go`: `sync.Map` agora armazena `*limiterEntry`
  (com `lastSeen atomic.Int64`). Goroutine `evictLoop` remove entries inativas
  há mais de 10 minutos a cada 1 minuto. Método `Stop()` com `sync.WaitGroup`.
- `internal/middleware/ratelimit_test.go`: helper `newTestRateLimiter` criado
  para registrar cleanup do goroutine de eviction em todos os testes.
- `internal/server/server.go`: closer agora chama `rateLimiter.Stop()` e
  `versionLimiter.Stop()` no shutdown.

## Definition of Done

- [x] Entries do rate limiter são removidas após TTL de inatividade
- [x] Goroutine de cleanup tem mecanismo de parada (Stop)
- [x] Entries ativas (com tráfego recente) não são removidas prematuramente
- [x] `go test ./internal/middleware/...` passa com testes de eviction
- [x] `go test ./...` sem regressões
- [x] `go vet ./...` limpo
