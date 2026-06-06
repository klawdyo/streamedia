# T10: Fila de transcodificação (channel + workers)

**Status:** pending
**Dependências:** T04
**Estimativa:** média

## Contexto

A fila existe para não derrubar a VPS ao aceitar múltiplos uploads simultâneos.
O risco real é contenção de CPU e disco pelo FFmpeg, não memória.

### Implementação

- Um `chan string` com buffer de `QUEUE_MAX_SIZE` (padrão 50)
- Um pool de `TRANSCODE_WORKERS` goroutines (padrão 1) consumindo do channel
- Quando um upload completa, `Enqueue(videoID)` adiciona à fila
- Se a fila estiver cheia, retorna erro (não bloqueia)
- Workers executam o transcode sequencialmente

### Interface pública

```go
type Queue struct { ... }

func NewQueue(cfg *config.Config, db *sql.DB, worker TranscodeFunc) *Queue
func (q *Queue) Start() // inicia os workers (goroutines)
func (q *Queue) Stop()  // drena e fecha (graceful shutdown)
func (q *Queue) Enqueue(videoID string) error
func (q *Queue) Len() int // itens atualmente na fila (para /admin/queue)
```

`TranscodeFunc` é o tipo do worker: `type TranscodeFunc func(videoID string) error`
A implementação real do FFmpeg vem na T11.

## QA Instructions

Crie `internal/transcode/queue_test.go`:

```
TestQueue_EnqueueAndProcess
  - Cria fila com 1 worker e TranscodeFunc que grava em slice
  - Enfileira 3 video_ids
  - Aguarda processamento (com timeout de 2s)
  - Verifica que todos foram processados

TestQueue_SequentialWithOneWorker
  - Worker dorme 50ms
  - Enfileira 2 itens
  - Mede tempo total: deve ser >= 100ms (sequencial)
  - Verifica que não foram paralelos

TestQueue_FullQueueReturnsError
  - Cria fila com buffer de 2
  - Enfileira 2 itens (1 worker ocupado, 2 no buffer)
  - Tenta enfileirar um 4o item
  - Espera erro (fila cheia)

TestQueue_LenReturnsCurrentSize
  - Fila vazia: Len() == 0
  - Enfileira 3 (worker parado): Len() == 3
  - Após processar: Len() == 0

TestQueue_StopDrainsGracefully
  - Enfileira 2 itens
  - Chama Stop() logo depois
  - Verifica que os workers terminam sem panic

TestQueue_WorkerErrorDoesNotCrash
  - TranscodeFunc retorna erro
  - Fila continua processando próximos itens
  - Não deve causar panic nem travar

TestQueue_UpdatesStatusOnStart
  - Simula vídeo com status upload_complete no banco
  - Worker verifica que status mudou para transcoding ao iniciar
```

Use `sync.WaitGroup` e canais para sincronização nos testes.

## Dev Instructions

Crie `internal/transcode/queue.go`:

### Struct Queue

```go
type TranscodeFunc func(videoID string) error

type Queue struct {
    ch      chan string
    cfg     *config.Config
    db      *sql.DB
    worker  TranscodeFunc
    wg      sync.WaitGroup
    once    sync.Once
    stopCh  chan struct{}
}
```

### NewQueue

- Cria o channel com buffer `cfg.QueueMaxSize`
- Não inicia os workers ainda (isso é feito em `Start`)

### Start

- Inicia `cfg.TranscodeWorkers` goroutines
- Cada goroutine lê do channel e chama `worker(videoID)`
- Erros do worker são logados, não propagados (o worker já atualiza o banco)
- Usa `wg.Add(1)` + `defer wg.Done()` para tracking

### Stop

- Fecha o channel de stop
- Aguarda todas as goroutines terminarem via `wg.Wait()`
- Não deve bloquear indefinidamente — use timeout de 30s

### Enqueue

```go
func (q *Queue) Enqueue(videoID string) error
```

- Tenta escrever no channel com `select { case q.ch <- videoID: ... default: ... }`
- Se o channel estiver cheio, retorna erro em português: "Fila de transcodificação está cheia."
- Atualiza status do vídeo para `transcoding` no banco ANTES de enfileirar
  (se o crash acontecer antes do worker pegar, a T21 de recovery detecta)

### Len

```go
func (q *Queue) Len() int {
    return len(q.ch)
}
```

## Arquivos a criar/modificar

- `internal/transcode/queue.go`
- `internal/transcode/queue_test.go`

## Definition of Done

- [ ] Channel com buffer configurável
- [ ] Pool de workers configável (padrão 1)
- [ ] Enqueue falha rápido quando fila cheia (não bloqueia)
- [ ] Worker errors não travam a fila
- [ ] Stop é graceful
- [ ] Todos os testes passam
