# T14: Job 1 — Killer de uploads inativos

**Status:** pending
**Dependências:** T04
**Estimativa:** pequena

## Contexto

Uploads que pararam de receber chunks (app fechado, conexão perdida) devem ser
eliminados após um período de inatividade. O critério é: `last_chunk_at` mais
antigo que `UPLOAD_IDLE_TIMEOUT_MIN` minutos atrás.

**Importante:** A regra é "sem chunk novo por N minutos", não "N minutos desde
o início do upload". Um vídeo grande em conexão lenta que continua enviando
chunks NÃO é morto. Apenas uploads que pararam de vez.

### Regra

```
status IN ('pending_upload', 'uploading')
AND last_chunk_at < (agora - UPLOAD_IDLE_TIMEOUT_MIN minutos)
```

### Ação ao matar

1. Deleta os arquivos do disco: `{UPLOAD_TMP_DIR}/{videoID}` e `{UPLOAD_TMP_DIR}/{videoID}.info`
2. Atualiza status para `failed_upload` com mensagem de erro
3. Dispara webhook de falha (event: "failed")

### Execução

Roda como goroutine com `time.Ticker` a cada 2 minutos.

## QA Instructions

Crie `internal/jobs/killer_test.go`:

```
TestKillerJob_KillsInactiveUpload
  - Insere vídeo com status uploading
  - Define last_chunk_at = 11 minutos atrás
  - Cria arquivo temporário no UPLOAD_TMP_DIR
  - Roda o job com idleTimeout = 10 minutos
  - Verifica: status = failed_upload
  - Verifica: arquivo deletado do disco

TestKillerJob_SkipsActiveUpload
  - Insere vídeo com status uploading
  - Define last_chunk_at = 9 minutos atrás (dentro do timeout)
  - Roda o job
  - Verifica: status ainda é uploading (não foi morto)

TestKillerJob_SkipsReadyVideo
  - Insere vídeo com status ready e last_chunk_at = 2 horas atrás
  - Roda o job
  - Verifica: status ainda é ready (job só age em pending_upload e uploading)

TestKillerJob_SkipsTerminalStates
  - Insere vídeo com status failed_upload e last_chunk_at = 1 hora atrás
  - Roda o job
  - Verifica: status permanece failed_upload (não dupla-morte)

TestKillerJob_DeletesInfoFile
  - Arquivo .info também deve ser deletado
  - Verifica que {videoID}.info foi deletado

TestKillerJob_NullLastChunkAt
  - Vídeo com status pending_upload e last_chunk_at NULL
  - E created_at = 11 minutos atrás
  - Roda o job
  - Verifica: status = failed_upload (usa created_at como fallback)

TestKillerJob_DispatachesWebhook
  - Verifica que o callback de webhook foi chamado com event="failed"
```

Use banco SQLite em memória e diretório temporário (`t.TempDir()`).

## Dev Instructions

Crie `internal/jobs/killer.go`:

### Struct UploadKillerJob

```go
type UploadKillerJob struct {
    cfg        *config.Config
    db         *sql.DB
    onWebhook  func(videoID, event, errMsg string)
    ticker     *time.Ticker
    stopCh     chan struct{}
}

func NewUploadKillerJob(cfg *config.Config, db *sql.DB, onWebhook func(videoID, event, errMsg string)) *UploadKillerJob
```

### Método Run

```go
func (j *UploadKillerJob) Run()
```

Roda a cada 2 minutos (ticker). Para quando `stopCh` é fechado.

### Método runOnce (testável isoladamente)

```go
func (j *UploadKillerJob) runOnce() error
```

- Query: busca vídeos com status `pending_upload` ou `uploading`
  onde `last_chunk_at < (agora - idleTimeout)` ou (last_chunk_at IS NULL
  AND created_at < (agora - idleTimeout))
- Para cada vídeo encontrado:
  1. Tenta deletar `{uploadTmpDir}/{videoID}` (ignora erro se não existir)
  2. Tenta deletar `{uploadTmpDir}/{videoID}.info` (ignora erro se não existir)
  3. Chama `models.UpdateStatusWithError(db, videoID, StatusFailedUpload, msg)`
  4. Chama `j.onWebhook(videoID, "failed", msg)`

### Mensagem de erro

```go
"Upload encerrado por inatividade: nenhum chunk recebido nos últimos N minutos."
```

### Método Stop

```go
func (j *UploadKillerJob) Stop()
```

Fecha `stopCh` para parar o ticker.

## Arquivos a criar/modificar

- `internal/jobs/killer.go`
- `internal/jobs/killer_test.go`

## Definition of Done

- [ ] Apenas `pending_upload` e `uploading` são afetados
- [ ] Limite de tempo usa `last_chunk_at` (não `created_at`)
- [ ] Arquivos .info deletados junto com o arquivo principal
- [ ] Estados terminais e `ready` não são tocados
- [ ] Todos os testes passam
