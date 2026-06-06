# T11: Worker FFmpeg — geração HLS

**Status:** pending
**Dependências:** T10
**Estimativa:** grande

## Contexto

O worker recebe um `videoID`, encontra o arquivo bruto em `UPLOAD_TMP_DIR`,
roda o FFmpeg para gerar HLS em múltiplas resoluções, e grava os arquivos
em `MEDIA_DIR/{videoID}/`.

### Regra de não-upscaling

Gerar apenas resoluções MENORES OU IGUAIS à resolução de origem.
- Origem 480p → gera só 480p
- Origem 720p → gera 480p e 720p
- Origem 1080p ou maior → gera 480p, 720p e 1080p

### Estrutura de arquivos gerada

```
MEDIA_DIR/{videoID}/
├── master.m3u8
├── 480/
│   ├── playlist.m3u8
│   ├── 0.ts
│   ├── 1.ts
│   └── ...
├── 720/
│   ├── playlist.m3u8
│   └── ...
└── 1080/
    ├── playlist.m3u8
    └── ...
```

### Bitrates alvo

| Resolução | Escala | Bitrate vídeo | Bitrate áudio |
|-----------|--------|---------------|---------------|
| 480p | 854:480 | 900k | 128k |
| 720p | 1280:720 | 2000k | 128k |
| 1080p | 1920:1080 | 3500k | 192k |

### Conteúdo do master.m3u8

```
#EXTM3U
#EXT-X-STREAM-INF:BANDWIDTH=900000,RESOLUTION=854x480
480/playlist.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=2000000,RESOLUTION=1280x720
720/playlist.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=3500000,RESOLUTION=1920x1080
1080/playlist.m3u8
```

Apenas as resoluções efetivamente geradas aparecem no master.

### Comando FFmpeg por resolução

```bash
ffmpeg -i {input} \
  -vf scale={width}:{height} \
  -c:v libx264 -b:v {video_bitrate} \
  -c:a aac -b:a {audio_bitrate} \
  -hls_time 6 \
  -hls_list_size 0 \
  -hls_segment_filename "{output_dir}/{res}/%d.ts" \
  -f hls \
  "{output_dir}/{res}/playlist.m3u8"
```

## QA Instructions

Crie `internal/transcode/worker_test.go`:

```
TestDetermineResolutions_480pOrigin
  - Origem 640x480
  - Espera resoluções: [480]

TestDetermineResolutions_720pOrigin
  - Origem 1280x720
  - Espera resoluções: [480, 720]

TestDetermineResolutions_1080pOrigin
  - Origem 1920x1080
  - Espera resoluções: [480, 720, 1080]

TestDetermineResolutions_4KOrigin
  - Origem 3840x2160 (4K)
  - Espera resoluções: [480, 720, 1080] (capped em 1080p)

TestDetermineResolutions_PortraitVideo
  - Origem 720x1280 (vertical/portrait)
  - Deve considerar a dimensão maior (1280) para determinar resoluções
  - Espera: [480, 720]

TestGenerateMasterM3U8_TwoResolutions
  - Chama generateMasterM3U8([480, 720])
  - Verifica que contém #EXTM3U
  - Verifica que contém 480/playlist.m3u8
  - Verifica que contém 720/playlist.m3u8
  - Verifica que NÃO contém 1080/playlist.m3u8
  - Verifica formato BANDWIDTH correto

TestGenerateMasterM3U8_ThreeResolutions
  - Chama generateMasterM3U8([480, 720, 1080])
  - Verifica as três resoluções no output

TestBuildFFmpegArgs_480p
  - Chama buildFFmpegArgs(input, outputDir, 480)
  - Verifica que contém scale=854:480
  - Verifica que contém -b:v 900k
  - Verifica que contém -b:a 128k

TestBuildFFmpegArgs_1080p
  - Verifica scale=1920:1080, -b:v 3500k, -b:a 192k

TestTranscodeWorker_FFmpegNotAvailable
  - Injeta um executor de FFmpeg que sempre falha
  - Verifica que o status do vídeo vai para failed_transcode após 3 tentativas

TestTranscodeWorker_UpdatesStatus
  - Simula transcode bem-sucedido (FFmpeg mockado)
  - Verifica que status do vídeo é "ready" após conclusão
  - Verifica que resolutions foi gravado no banco
```

Para testar sem FFmpeg real, use uma interface `FFmpegExecutor`:
```go
type FFmpegExecutor interface {
    Run(ctx context.Context, args []string) error
}
```

## Dev Instructions

Crie `internal/transcode/worker.go`:

### Interface FFmpegExecutor

```go
type FFmpegExecutor interface {
    Run(ctx context.Context, args []string) error
}

// RealFFmpeg implementa FFmpegExecutor usando os/exec
type RealFFmpeg struct{}
```

### Struct Worker

```go
type Worker struct {
    cfg      *config.Config
    db       *sql.DB
    ffmpeg   FFmpegExecutor
    onWebhook func(videoID, event, errMsg string)
}

func NewWorker(cfg *config.Config, db *sql.DB, onWebhook func(videoID, event, errMsg string)) *Worker
```

### Função principal

```go
func (w *Worker) Transcode(videoID string) error
```

Fluxo:
1. Busca o vídeo no banco
2. Atualiza status para `transcoding`
3. Localiza o arquivo de input: `cfg.UploadTmpDir + "/" + videoID`
4. Usa FFprobe para descobrir dimensões de origem
5. Calcula resoluções a gerar (sem upscaling)
6. Cria diretório de output: `cfg.MediaDir + "/" + videoID`
7. Para cada resolução: roda FFmpeg com timeout de 30min
8. Se qualquer FFmpeg falhar:
   - Incrementa `transcode_attempts` no banco
   - Se `attempts >= MAX_TRANSCODE_ATTEMPTS`: status `failed_transcode`, webhook "failed"
   - Caso contrário: retorna erro (a fila recolocará via job T15)
9. Gera `master.m3u8` manualmente (texto simples)
10. Chama `models.SetReady(db, videoID, durationS, resolutions)`
11. Se `KEEP_ORIGINAL=false`: deleta o arquivo de input
12. Dispara webhook "ready"

### Função generateMasterM3U8

```go
func generateMasterM3U8(resolutions []int) string
```

Gera o conteúdo do arquivo `master.m3u8` como string.

### Função determineResolutions

```go
func determineResolutions(originWidth, originHeight int) []int
```

- Considera a dimensão maior (suporte a vídeos portrait)
- Retorna apenas resoluções <= origem
- Sempre ordena: [480, 720, 1080] filtrado

### Timeout do FFmpeg

Use `context.WithTimeout(context.Background(), 30*time.Minute)` para cada
execução do FFmpeg. Se o timeout estourar, conta como tentativa falha.

## Arquivos a criar/modificar

- `internal/transcode/worker.go`
- `internal/transcode/worker_test.go`

## Definition of Done

- [ ] Sem upscaling (origem 720p não gera 1080p)
- [ ] master.m3u8 gerado corretamente para cada combinação de resoluções
- [ ] Timeout do FFmpeg via context
- [ ] KEEP_ORIGINAL=false deleta o arquivo de input
- [ ] transcode_attempts incrementado corretamente
- [ ] Todos os testes passam
