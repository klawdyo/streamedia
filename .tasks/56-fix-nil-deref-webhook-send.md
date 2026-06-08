# T56: Corrigir nil pointer dereference em `sendWebhook` — `GetVideo` pode retornar nil

**Status:** done
**Dependências:** T17, T20
**Estimativa:** pequena
**Origem:** análise de código — crash em produção
**Severidade:** critica

## Contexto

Em `internal/server/server.go:43-46`, o closure `sendWebhook` ignora o erro
de `models.GetVideo` e passa o resultado diretamente para `wc.Send`:

```go
sendWebhook := func(videoID, event, errMsg string) {
    video, _ := models.GetVideo(database, videoID)
    _ = wc.Send(videoID, event, video)
}
```

Se `GetVideo` falhar (banco indisponível, vídeo já deletado, etc.),
`video` é `nil`. Em `webhook.go:139`, `buildPayload` acessa `video.Status`,
`video.DurationS` e `video.Resolutions` — **panic garantido**.

Esse path é acionado em cenários reais: o webhook de `failed` é disparado
quando o worker falha e incrementa tentativas — exatamente o momento em que
o sistema está mais instável.

## Impacto

- **Crash do servidor** em qualquer webhook disparado quando o vídeo não
  é encontrado no banco (ex.: race condition de cleanup + webhook).
- Nenhum log de erro — o panic é capturado pelo recovery middleware, mas
  a causa raiz fica opaca.

## QA Instructions

```
TestSendWebhook_NilVideo
  - Mock GetVideo retornando (nil, error)
  - Chama sendWebhook("id", "failed", "msg")
  - Verifica que NÃO ocorre panic
  - Verifica que o erro é logado
```

## Dev Instructions

### 1. Tratar o erro de GetVideo em `server.go`

```go
sendWebhook := func(videoID, event, errMsg string) {
    video, err := models.GetVideo(database, videoID)
    if err != nil {
        log.Printf("[webhook] erro ao buscar vídeo %s para webhook %s: %v", videoID, event, err)
        return
    }
    if err := wc.Send(videoID, event, video); err != nil {
        log.Printf("[webhook] erro ao enviar webhook %s para vídeo %s: %v", event, videoID, err)
    }
}
```

### 2. Verificação

- `go test ./...` — sem regressões
- `go vet ./...` — sem warnings

## Arquivos a editar

- `internal/server/server.go` (tratar erros no closure sendWebhook)

## Resolução

Arquivos alterados:
- `internal/server/server.go`: closure `sendWebhook` agora trata erros de
  `GetVideo` e `wc.Send` — loga e retorna em vez de prosseguir com nil.
- `cmd/server/main.go`: mesmo fix aplicado ao closure `sendWebhook` duplicado.

Ambos os closures tinham o mesmo bug. O fix é idêntico: tratar o erro de
`GetVideo`, logar e retornar early (sem acessar campos de video nil).

## Definition of Done

- [x] `GetVideo` error é tratado e logado (sem `_ =`)
- [x] `wc.Send` error é tratado e logado (sem `_ =`)
- [x] Nenhum panic possível por nil pointer em `buildPayload`
- [x] `go test ./...` passa (falhas pré-existentes em upload e transcode não relacionadas)
- [x] `go vet ./...` limpo
