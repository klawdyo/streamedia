# T59: Adicionar graceful shutdown ao `consumeHooks` do TUS handler

**Status:** done
**Dependências:** T07
**Estimativa:** pequena
**Origem:** análise de código — goroutine leak
**Severidade:** critica

## Contexto

Em `internal/upload/tus.go:114-123`, a goroutine `consumeHooks` roda em
loop infinito sem mecanismo de parada:

```go
func (h *TUSHandler) consumeHooks() {
    for {
        select {
        case event := <-h.handler.UploadProgress:
            h.postReceive(event)
        case event := <-h.handler.CompleteUploads:
            h.postFinish(event)
        }
    }
}
```

Problemas:
1. **Sem canal de parada**: quando o servidor faz shutdown, a goroutine
   continua rodando indefinidamente.
2. **Canais fechados**: se o tusd handler for fechado e os canais forem
   closed, a goroutine vai receber zero-values em loop infinito sem parar
   (channels que retornam zero-value depois de fechados).
3. **Sem `sync.WaitGroup`**: não há como saber quando a goroutine de fato
   terminou durante o shutdown.

## Impacto

- **Goroutine leak** no shutdown do servidor.
- Processamento de zero-value hooks depois que o handler tusd é fechado
  (ex.: `postFinish` com `hook.Upload.ID == ""` → operações no banco com
  video_id vazio).
- Impossibilidade de shutdown gracioso limpo.

## QA Instructions

```
TestTUSHandler_GracefulShutdown
  - Cria TUSHandler
  - Chama Stop()
  - Verifica que a goroutine termina (ex.: via WaitGroup ou timeout)

TestTUSHandler_ClosedChannels
  - Fecha os canais do handler tusd
  - Verifica que consumeHooks não entra em loop infinito
```

## Dev Instructions

### 1. Adicionar canal de parada e método `Stop` ao TUSHandler

```go
type TUSHandler struct {
    handler  *tusd.Handler
    cfg      *config.Config
    db       *sql.DB
    onFinish func(videoID, userAgent string)
    stopCh   chan struct{}
    wg       sync.WaitGroup
}
```

### 2. Modificar `consumeHooks` para respeitar o stopCh

```go
func (h *TUSHandler) consumeHooks() {
    defer h.wg.Done()
    for {
        select {
        case event, ok := <-h.handler.UploadProgress:
            if !ok { return }
            h.postReceive(event)
        case event, ok := <-h.handler.CompleteUploads:
            if !ok { return }
            h.postFinish(event)
        case <-h.stopCh:
            return
        }
    }
}
```

### 3. Adicionar método `Stop`

```go
func (h *TUSHandler) Stop() {
    close(h.stopCh)
    h.wg.Wait()
}
```

### 4. Chamar `Stop` no shutdown do servidor (main.go)

Garantir que o `Stop` do TUSHandler é chamado no fluxo de shutdown
gracioso do `main.go`.

## Arquivos a editar

- `internal/upload/tus.go` (stopCh, WaitGroup, Stop, consumeHooks)
- `cmd/server/main.go` (chamar tusHandler.Stop no shutdown)
- `internal/server/server.go` (se necessário expor o tusHandler)

## Resolução

Arquivos alterados:
- `internal/upload/tus.go`: adicionados campos `stopCh chan struct{}` e
  `wg sync.WaitGroup` à struct `TUSHandler`. `consumeHooks` agora faz
  `defer h.wg.Done()`, verifica `ok` nos receives (detecta canal fechado),
  e respeita `stopCh`. Método `Stop()` adicionado. `NewTUSHandler` inicializa
  `stopCh` e registra a goroutine no `WaitGroup`.
- `internal/server/server.go`: `NewRouter` retorna `io.Closer` que chama
  `tusHandler.Stop()` no shutdown (compartilhado com T57).
- `cmd/server/main.go`: chama `routerCloser.Close()` no defer do main.

## Definition of Done

- [x] `consumeHooks` respeita canal de parada
- [x] `consumeHooks` verifica `ok` no receive de canais (detecta canal fechado)
- [x] Método `Stop` fecha `stopCh` e espera a goroutine via `WaitGroup`
- [x] Shutdown do servidor chama `TUSHandler.Stop`
- [x] `go test ./...` passa
- [x] `go vet ./...` limpo
