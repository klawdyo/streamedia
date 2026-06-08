# T65: Corrigir `handleTranscodeFailure` — erros de banco ignorados silenciosamente

**Status:** done
**Dependências:** T11
**Estimativa:** pequena
**Origem:** análise de código — erro silenciado
**Severidade:** media

## Contexto

Em `internal/transcode/worker.go:375-388`, `handleTranscodeFailure` ignora
erros de operacoes criticas no banco:

```go
func (w *Worker) handleTranscodeFailure(videoID string, currentAttempts int, errMsg string) error {
    // Incrementa o contador de tentativas (best-effort).
    _ = models.IncrementTranscodeAttempts(w.db, videoID)

    // Se atingiu (ou superou) o máximo, é falha terminal.
    if currentAttempts+1 >= w.cfg.MaxTranscodeAttempts {
        _ = models.UpdateStatusWithError(w.db, videoID, models.StatusFailedTranscode, errMsg)
        w.onWebhook(videoID, "failed", errMsg)
        return nil
    }
    // ...
}
```

Problemas:
1. Se `IncrementTranscodeAttempts` falhar, o contador fica errado mas a
   funcao continua como se tivesse incrementado — pode causar retry infinito
   (o video nunca atinge MaxTranscodeAttempts).
2. Se `UpdateStatusWithError` falhar na falha terminal, o video fica em
   estado `transcoding` eternamente mas a funcao retorna `nil` (sucesso) e
   o webhook de "failed" e enviado — estado do banco e do sistema externo
   ficam inconsistentes.

## Impacto

- **Retry infinito** se o contador de tentativas nao for incrementado.
- **Estado inconsistente** entre banco e sistema externo via webhook.
- Sem log — impossivel diagnosticar a causa raiz.

## Dev Instructions

### 1. Adicionar logging nos erros ignorados

Os erros sao marcados como "best-effort" (comentario no codigo), o que
e uma decisao valida — o worker nao deve travar por causa de um erro de
contabilidade. Mas os erros devem ser **logados**:

```go
if err := models.IncrementTranscodeAttempts(w.db, videoID); err != nil {
    log.Printf("[transcode] %s: erro ao incrementar tentativas (best-effort): %v", videoID, err)
}

if currentAttempts+1 >= w.cfg.MaxTranscodeAttempts {
    if err := models.UpdateStatusWithError(w.db, videoID, models.StatusFailedTranscode, errMsg); err != nil {
        log.Printf("[transcode] %s: erro ao marcar como falha terminal (best-effort): %v", videoID, err)
    }
    w.onWebhook(videoID, "failed", errMsg)
    return nil
}
```

### 2. Verificacao

- `go test ./internal/transcode/...` — sem regressoes
- `go vet ./...` — sem warnings

## Arquivos a editar

- `internal/transcode/worker.go` (adicionar log.Printf nos erros ignorados)

## Resolução

Arquivos alterados:
- `internal/transcode/worker.go`: `_ =` substituído por `if err := ...; err != nil`
  com `log.Printf` em ambos os casos. Comportamento best-effort mantido.

## Definition of Done

- [x] Erros de `IncrementTranscodeAttempts` sao logados
- [x] Erros de `UpdateStatusWithError` sao logados
- [x] Comportamento best-effort mantido (nao retorna erro)
- [x] `go test ./...` passa
- [x] `go vet ./...` limpo
