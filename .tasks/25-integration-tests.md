# T25: Suite de testes de integração completa

**Status:** pending
**Dependências:** T20
**Estimativa:** grande

## Contexto

Testes de integração end-to-end que verificam o sistema completo rodando.
Diferente dos testes unitários das tarefas anteriores, aqui testamos o fluxo
completo: upload → transcode → serving.

Esses testes requerem FFmpeg instalado. Use `t.Skip` se não disponível.

## Fluxo completo a testar

```
1. POST /upload/init (autenticado)
2. TUS upload completo (POST + PATCH)
3. Webhook de processing recebido
4. Transcode executado (FFmpeg)
5. Webhook de ready recebido
6. GET /videos/{id}/master.m3u8 (com token)
7. GET /videos/{id}/480/playlist.m3u8 (estático)
8. GET /api/status/{id} retorna ready
```

## QA Instructions

Crie `internal/integration/integration_test.go`:

### Suite de setup

```go
type IntegrationSuite struct {
    server     *httptest.Server
    db         *sql.DB
    mediaDir   string
    uploadDir  string
    webhookLog []WebhookCall
    cfg        *config.Config
}

func setupSuite(t *testing.T) *IntegrationSuite
```

O setup cria:
- Banco SQLite temporário
- Diretório de mídia temporário
- Servidor HTTP de webhook fake (captura chamadas)
- Servidor media server completo

### Testes de integração

```
TestFullUploadFlow
  - Init upload → upload TUS → espera webhook "processing" → espera "ready"
  - Verifica master.m3u8 acessível com token
  - Verifica status = ready no banco
  (requer FFmpeg — skip se não disponível)

TestUploadInit_HMACProtection_Integration
  - POST /upload/init sem auth → 401
  - POST /upload/init com auth errada → 401
  - POST /upload/init com auth correta → 200

TestTUSUpload_ResumeCapability
  - Inicia upload
  - Simula interrupção (não envia todos os chunks)
  - HEAD /files/{id} retorna offset correto
  - Retoma de onde parou
  - Upload completa

TestPlayToken_Integration
  - Vídeo em status ready
  - Gera token de reprodução
  - GET /videos/{id}/master.m3u8?expires=X&token=Y → 200
  - Mesmo GET sem token → 401
  - Mesmo GET com token expirado → 401

TestStaticServing_Integration
  - Vídeo processado
  - GET /videos/{id}/480/playlist.m3u8 → 200
  - GET /videos/{id}/480/ (listagem) → 404

TestAdminRoutes_Integration
  - GET /admin/videos com token admin → 200
  - Insere 3 vídeos, verifica que aparecem na lista
  - GET /admin/queue → retorna queue_length

TestUploadKillerJob_Integration
  - Insere vídeo com last_chunk_at = 11 minutos atrás
  - Roda o job manualmente
  - Verifica status = failed_upload
  - Verifica webhook "failed" recebido no servidor fake

TestTranscodeRecovery_Integration
  - Insere vídeo com status transcoding (simulando crash)
  - Chama RunStartupRecovery
  - Verifica que vídeo foi reenfileirado

TestConcurrentUploads
  - Inicia 5 uploads simultâneos
  - Verifica que todos recebem upload_url
  - Verifica que a fila não excede o buffer
```

### Servidor de webhook fake

```go
type WebhookCall struct {
    Event   string
    VideoID string
    Body    []byte
    Headers http.Header
}

func startFakeWebhookServer(t *testing.T) (*httptest.Server, *[]WebhookCall)
```

Captura todas as chamadas e permite assertions sobre elas.

## Dev Instructions

### Criar internal/integration/integration_test.go

A suite de integração deve:
1. Usar `httptest.NewServer` para o servidor completo
2. Usar banco SQLite em arquivo temporário (não memória — testes de disco)
3. Capturar webhooks com servidor fake
4. Usar `t.TempDir()` para mídia e uploads
5. Ter timeout por teste (máx 5 minutos para testes com FFmpeg)

### Helper para upload TUS programático

```go
func doTUSUpload(t *testing.T, serverURL, uploadURL, token string, data []byte)
```

Implementa o protocolo TUS manualmente:
1. POST para criar o upload (com headers TUS)
2. PATCH em chunks para enviar os dados

### Vídeo de teste

Use um vídeo MP4 mínimo gerado pelo FFmpeg para testes:
```bash
ffmpeg -f lavfi -i testsrc=duration=3:size=640x480:rate=30 -f lavfi -i anullsrc=r=44100:cl=stereo -t 3 test.mp4
```

Gere o arquivo de teste em `testdata/test.mp4` (não commitar arquivos grandes).
Ou gere via código Go usando `exec.Command("ffmpeg", ...)` se FFmpeg disponível.

## Arquivos a criar

- `internal/integration/integration_test.go`
- `testdata/.gitkeep` (o diretório testdata para futuros fixtures)

## Definition of Done

- [ ] Fluxo completo testado end-to-end (com FFmpeg disponível)
- [ ] Autenticação testada em integração
- [ ] Jobs de manutenção testados em integração
- [ ] Testes pulados corretamente quando FFmpeg não disponível
- [ ] `go test ./internal/integration/... -timeout 10m` passa
