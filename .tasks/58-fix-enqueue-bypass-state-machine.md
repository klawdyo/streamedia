# T58: Corrigir `Enqueue` — bypass da máquina de estados com UPDATE direto

**Status:** done
**Dependências:** T10, T04
**Estimativa:** pequena
**Origem:** análise de código — corrupção de estado
**Severidade:** critica

## Contexto

Em `internal/transcode/queue.go:92`, `Enqueue` faz um UPDATE direto no banco
para mudar o status para `'transcoding'`:

```go
_, err := q.db.Exec("UPDATE videos SET status = 'transcoding', updated_at = strftime(...) WHERE video_id = ?", videoID)
```

Isso **bypassa completamente a máquina de estados** definida em
`models.UpdateStatus` / `validTransitions` (`video.go:47-52`). Qualquer
vídeo — independente do estado atual — pode ter seu status forçado para
`transcoding`, incluindo transições inválidas como:
- `ready` -> `transcoding` (vídeo já pronto, re-transcodificado por engano)
- `failed_upload` -> `transcoding` (upload falhou, nunca deveria transcodificar)
- `failed_transcode` -> `transcoding` (falha terminal, não deveria ser reprocessado sem reset explícito)

### Problema secundário: formato de timestamp

O UPDATE usa `strftime('%Y-%m-%dT%H:%M:%fZ', 'now')` (formato RFC3339 com
microsegundos), enquanto `models.UpdateStatus` e outros métodos de escrita
não setam `updated_at` explicitamente (delegam ao DEFAULT/trigger do banco).
Isso cria inconsistência de formato que pode afetar comparações `datetime()`
no SQLite (ex.: o job de requeue em `requeue.go:81` compara `datetime(updated_at)`).

## Impacto

- **Transições de estado inválidas** que corrompem a máquina de estados.
- Vídeos em estados terminais podem ser re-transcodificados.
- Timestamps inconsistentes podem fazer o requeue job ignorar vídeos
  travados ou detectar falsos positivos.

## QA Instructions

```
TestEnqueue_RejectsInvalidTransition
  - Insere vídeo com status 'ready'
  - Chama Enqueue(videoID)
  - Verifica que retorna erro de transição inválida
  - Verifica que o status no banco continua 'ready'

TestEnqueue_AcceptsValidTransition
  - Insere vídeo com status 'upload_complete'
  - Chama Enqueue(videoID)
  - Verifica sucesso
  - Verifica que o status no banco é 'transcoding'
```

## Dev Instructions

### 1. Usar `models.UpdateStatus` em vez de SQL direto

Substituir o `db.Exec` por:
```go
if err := models.UpdateStatus(q.db, videoID, models.StatusTranscoding); err != nil {
    return fmt.Errorf("erro ao atualizar status para transcoding: %w", err)
}
```

Isso garante que a transição é validada pela máquina de estados e que o
formato de timestamp é consistente com o resto do código.

### 2. Verificação

- `go test ./internal/transcode/...` — incluindo os novos testes de QA
- `go test ./...` — sem regressões
- `go vet ./...` — sem warnings

## Arquivos a editar

- `internal/transcode/queue.go` (substituir SQL direto por models.UpdateStatus)

## Resolução

Arquivos alterados:
- `internal/transcode/queue.go`: substituído `db.Exec("UPDATE videos SET status = 'transcoding'...")`
  por `models.UpdateStatus(q.db, videoID, models.StatusTranscoding)`. Adicionado import de
  `models`. A transição agora é validada pela state machine e o formato de timestamp é
  consistente (resolve também T63).
- `internal/transcode/queue_test.go`: todos os testes que chamavam `Enqueue` sem vídeo no
  banco foram atualizados para inserir o vídeo com status `upload_complete` antes. Helper
  `insertQueueTestVideo` criado (nome diferente de `insertTestVideo` em recovery_test.go).

## Definition of Done

- [x] `Enqueue` usa `models.UpdateStatus` para validar transições
- [x] Transições inválidas retornam erro
- [x] Formato de timestamp é consistente com o resto do sistema
- [x] `go test ./...` passa
- [x] `go vet ./...` limpo
