# T67: Adicionar `sync.WaitGroup` ao `Stop` dos 3 jobs periodicos

**Status:** done
**Dependências:** T14, T15, T16
**Estimativa:** pequena
**Origem:** análise de código — race condition no shutdown
**Severidade:** media

## Contexto

Os tres jobs periodicos (`cleanup.go`, `killer.go`, `requeue.go`) usam o
mesmo padrao de start/stop:

```go
func (j *Job) Start() {
    go func() {
        for {
            select {
            case <-j.ticker.C:
                j.runOnce()
            case <-j.stopCh:
                j.ticker.Stop()
                return
            }
        }
    }()
}

func (j *Job) Stop() {
    close(j.stopCh)
}
```

`Stop()` fecha o canal mas **nao espera** a goroutine terminar. Isso causa:

1. **Race condition**: se `Stop()` for seguido de nova operacao no mesmo
   recurso (ex.: fechar o banco), a goroutine pode ainda estar executando
   `runOnce()` quando o banco e fechado.
2. **Shutdown incompleto**: `main.go` chama `Stop()` e continua com o
   encerramento, mas a goroutine pode ainda estar rodando.

## Dev Instructions

### 1. Adicionar WaitGroup a cada job

```go
type TranscodeRequeueJob struct {
    // ... campos existentes ...
    wg sync.WaitGroup
}
```

### 2. Modificar Start/Stop

```go
func (j *Job) Start() {
    j.wg.Add(1)
    go func() {
        defer j.wg.Done()
        for {
            select {
            case <-j.ticker.C:
                j.runOnce()
            case <-j.stopCh:
                j.ticker.Stop()
                return
            }
        }
    }()
}

func (j *Job) Stop() {
    close(j.stopCh)
    j.wg.Wait()
}
```

### 3. Aplicar nos 3 jobs

- `internal/jobs/cleanup.go` (TokenCleanupJob)
- `internal/jobs/killer.go` (UploadKillerJob)
- `internal/jobs/requeue.go` (TranscodeRequeueJob)

### 4. Verificacao

- `go test ./internal/jobs/...` — sem regressoes
- `go vet ./...` — sem warnings
- Verificar que `main.go` chama `Stop()` de todos os jobs no shutdown

## Arquivos a editar

- `internal/jobs/cleanup.go`
- `internal/jobs/killer.go`
- `internal/jobs/requeue.go`

## Resolução

Arquivos alterados:
- `internal/jobs/cleanup.go`: campo `wg sync.WaitGroup` adicionado a
  `TokenCleanupJob`. `Start()` faz `wg.Add(1)`, goroutine faz `defer wg.Done()`,
  `Stop()` chama `wg.Wait()`.
- `internal/jobs/killer.go`: mesmo padrão para `UploadKillerJob`.
- `internal/jobs/requeue.go`: mesmo padrão para `TranscodeRequeueJob`.

## Definition of Done

- [x] Todos os 3 jobs tem `sync.WaitGroup`
- [x] `Stop()` bloqueia ate a goroutine terminar
- [x] `go test ./...` passa
- [x] `go vet ./...` limpo
- [x] `-race` nao detecta data races no shutdown
