# T15: Job 2 — Reenfileirador de transcodes travados

**Status:** pending
**Dependências:** T10
**Estimativa:** pequena

## Contexto

Um worker FFmpeg pode travar (processo zumbi, timeout não funcionou, OOM silencioso).
Se o status ficou em `transcoding` por mais de `TRANSCODE_STUCK_MIN` minutos, o
vídeo precisa ser reenfileirado ou marcado como falha definitiva.

### Regra de reenfileiramento

```
status = transcoding
AND updated_at < (agora - TRANSCODE_STUCK_MIN minutos)
AND transcode_attempts < MAX_TRANSCODE_ATTEMPTS
→ status = upload_complete (volta para fila)
→ transcode_attempts + 1
```

### Regra de falha definitiva

```
status = transcoding
AND updated_at < (agora - TRANSCODE_STUCK_MIN minutos)
AND transcode_attempts >= MAX_TRANSCODE_ATTEMPTS
→ status = failed_transcode
→ dispara webhook de falha
```

### Diferença em relação ao upload killer

O transcode PODE ser reenfileirado automaticamente porque o arquivo de input
ainda está em disco. O FFmpeg não tem checkpoint, mas a retentativa roda do
zero sem exigir novo upload.

### Execução

Roda como goroutine com `time.Ticker` a cada 5 minutos.

## QA Instructions

Crie `internal/jobs/requeue_test.go`:

```
TestRequeueJob_RequeuesStuckTranscode
  - Vídeo com status transcoding, updated_at = 31 minutos atrás, attempts = 0
  - Roda o job com stuckTimeout = 30 minutos, maxAttempts = 3
  - Verifica: status = upload_complete (volta para fila)
  - Verifica: transcode_attempts = 1 (incrementado)

TestRequeueJob_SkipsRecentTranscode
  - Vídeo com status transcoding, updated_at = 29 minutos atrás
  - Roda o job
  - Verifica: status ainda é transcoding (não foi tocado)

TestRequeueJob_FailsAfterMaxAttempts
  - Vídeo com status transcoding, updated_at = 31 minutos atrás, attempts = 3
  - Roda o job com maxAttempts = 3
  - Verifica: status = failed_transcode
  - Verifica: webhook de falha foi disparado

TestRequeueJob_SkipsNonTranscodingStatuses
  - Vídeo com status uploading e updated_at = 1 hora atrás
  - Roda o job
  - Verifica: status ainda é uploading (job só age em transcoding)

TestRequeueJob_ExactlyAtLimit
  - transcode_attempts = MAX_TRANSCODE_ATTEMPTS - 1 (exatamente na última chance)
  - Vídeo travado
  - Verifica: reenfileirado (não falha, ainda tem tentativa)

TestRequeueJob_CallsEnqueue
  - Vídeo reenfileirado deve chamar o callback de enqueue
  - Verifica que Enqueue foi chamado com o video_id correto
```

## Dev Instructions

Crie `internal/jobs/requeue.go`:

### Struct TranscodeRequeueJob

```go
type TranscodeRequeueJob struct {
    cfg       *config.Config
    db        *sql.DB
    enqueue   func(videoID string) error
    onWebhook func(videoID, event, errMsg string)
    ticker    *time.Ticker
    stopCh    chan struct{}
}

func NewTranscodeRequeueJob(
    cfg *config.Config,
    db *sql.DB,
    enqueue func(videoID string) error,
    onWebhook func(videoID, event, errMsg string),
) *TranscodeRequeueJob
```

### Método runOnce (testável isoladamente)

```go
func (j *TranscodeRequeueJob) runOnce() error
```

Query: busca vídeos com `status = transcoding` AND
`updated_at < datetime('now', '-N minutes')` onde N = TranscodeStuckMin.

Para cada vídeo:
- Se `transcode_attempts < cfg.MaxTranscodeAttempts`:
  1. Incrementa `transcode_attempts`
  2. Atualiza status para `upload_complete`
  3. Chama `j.enqueue(videoID)`
- Se `transcode_attempts >= cfg.MaxTranscodeAttempts`:
  1. Atualiza status para `failed_transcode` com mensagem
  2. Chama `j.onWebhook(videoID, "failed", msg)`

### Mensagem de erro

```go
"Transcodificação falhou após %d tentativas. O vídeo não pôde ser processado."
```

### Método Run e Stop

Mesma estrutura do UploadKillerJob, mas com ticker de 5 minutos.

## Arquivos a criar/modificar

- `internal/jobs/requeue.go`
- `internal/jobs/requeue_test.go`

## Definition of Done

- [ ] Reenfileira se `attempts < max` e estava travado
- [ ] Falha definitiva se `attempts >= max`
- [ ] Não toca outros status
- [ ] transcode_attempts incrementado antes de reenfileirar
- [ ] Todos os testes passam
