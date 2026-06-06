# T21: Recuperação de crash na inicialização

**Status:** pending
**Dependências:** T10
**Estimativa:** pequena

## Contexto

Se o servidor crashar (kill -9, OOM, reboot) enquanto há jobs em andamento, o
banco ficará com entradas em estados inconsistentes:

- Status `transcoding`: FFmpeg estava rodando mas foi morto → arquivo existe,
  precisa ser reenfileirado
- Status `upload_complete`: estava na fila mas não foi pego → precisa ser
  reenfileirado

Na inicialização, antes de aceitar qualquer request, o sistema deve varrer o
banco e reenfileirar esses vídeos.

### Ação por estado

| Status encontrado | Ação |
|-------------------|------|
| `transcoding` | Incrementa transcode_attempts. Se < max: volta para upload_complete e reenfileira. Se >= max: failed_transcode |
| `upload_complete` | Reenfileira diretamente (sem incrementar attempts) |
| `uploading` (muito antigo) | Não recupera aqui — o job killer (T14) cuida |
| `pending_upload` (muito antigo) | Não recupera aqui — o job killer cuida |

## QA Instructions

Crie `internal/transcode/recovery_test.go`:

```
TestRecovery_RequeuesTranscoding
  - Insere vídeo com status transcoding, attempts=0
  - Chama RunStartupRecovery(db, cfg, enqueue)
  - Verifica: vídeo está com status upload_complete
  - Verifica: enqueue foi chamado com o video_id

TestRecovery_RequeuesUploadComplete
  - Insere vídeo com status upload_complete
  - Chama RunStartupRecovery
  - Verifica: enqueue foi chamado

TestRecovery_FailsTranscodingAtMaxAttempts
  - Insere vídeo com status transcoding, attempts = MAX_TRANSCODE_ATTEMPTS
  - Chama RunStartupRecovery
  - Verifica: status = failed_transcode
  - Verifica: enqueue NÃO foi chamado

TestRecovery_MultipleVideos
  - Insere 3 vídeos: 2 transcoding, 1 upload_complete
  - Chama RunStartupRecovery
  - Verifica que todos os 3 foram reenfileirados (ou falharam se no limite)

TestRecovery_SkipsOtherStatuses
  - Insere vídeos com status: ready, failed_upload, uploading, pending_upload
  - Chama RunStartupRecovery
  - Verifica que nenhum deles foi alterado

TestRecovery_EmptyDB
  - Banco vazio
  - Chama RunStartupRecovery
  - Não deve retornar erro
```

## Dev Instructions

Crie `internal/transcode/recovery.go`:

### Função RunStartupRecovery

```go
func RunStartupRecovery(
    db *sql.DB,
    cfg *config.Config,
    enqueue func(videoID string) error,
    onWebhook func(videoID, event, errMsg string),
) error
```

Fluxo:
1. Busca todos os vídeos com status `transcoding` ou `upload_complete`
2. Para cada `upload_complete`: chama `enqueue(videoID)`
3. Para cada `transcoding`:
   - Se `attempts < cfg.MaxTranscodeAttempts`:
     - Incrementa attempts
     - Atualiza status para `upload_complete`
     - Chama `enqueue(videoID)`
   - Senão:
     - Atualiza status para `failed_transcode`
     - Chama `onWebhook(videoID, "failed", msg)`
4. Loga o número de vídeos recuperados

### Chamada em main.go

`RunStartupRecovery` deve ser chamada em `main()` ANTES de `queue.Start()`
e ANTES de aceitar requests, mas DEPOIS de criar a fila (para poder enfileirar).

## Arquivos a criar/modificar

- `internal/transcode/recovery.go`
- `internal/transcode/recovery_test.go`
- Atualizar `cmd/server/main.go` para chamar a recovery

## Definition of Done

- [ ] Vídeos em `transcoding` são reenfileirados ou marcados como falha
- [ ] Vídeos em `upload_complete` são reenfileirados
- [ ] Outros status não são tocados
- [ ] Chamada em main.go antes de aceitar requests
- [ ] Todos os testes passam
