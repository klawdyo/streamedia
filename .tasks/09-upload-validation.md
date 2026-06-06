# T09: Hook post-finish: validação do arquivo

**Status:** pending
**Dependências:** T08
**Estimativa:** pequena

## Contexto

Quando o TUS completa o upload de um arquivo (todos os chunks recebidos), o hook
`post-finish` é disparado. Neste momento precisamos:

1. Verificar que o tamanho real do arquivo bate com o tamanho declarado
2. Verificar os magic bytes do arquivo (é realmente um vídeo?)
3. Usar FFprobe para extrair duração e dimensões (valida que o arquivo é legível)
4. Se tudo ok: marcar status `upload_complete`, enfileirar transcode
5. Se falhou: marcar `failed_upload`, deletar arquivo, disparar webhook de falha

### Magic bytes suportados

| Container | Magic bytes (hex) | Offset |
|-----------|------------------|--------|
| MP4/MOV (ftyp) | 66 74 79 70 | offset 4 |
| MP4 (fallback) | 00 00 00 xx 66 74 79 70 | offset 0-7 |
| QuickTime | 00 00 00 xx 6d 6f 6f 76 | offset 0-7 |
| MKV/WebM | 1a 45 df a3 | offset 0 |
| AVI | 52 49 46 46 (RIFF) | offset 0 |

### FFprobe

Chamar: `ffprobe -v quiet -print_format json -show_streams {filepath}`
Extrair: duração em segundos e dimensões (width × height).
Usar `context.WithTimeout(5 * time.Second)`.

## QA Instructions

Crie `internal/upload/validation_test.go`:

```
TestValidateMagicBytes_MP4
  - Cria arquivo temporário com header de MP4 (bytes: 00 00 00 18 66 74 79 70...)
  - Chama validateMagicBytes(path)
  - Espera true

TestValidateMagicBytes_TextFile
  - Cria arquivo com conteúdo texto "Hello World"
  - Chama validateMagicBytes(path)
  - Espera false

TestValidateMagicBytes_EmptyFile
  - Arquivo vazio
  - Espera false

TestValidateMagicBytes_MKV
  - Cria arquivo com magic bytes MKV: 1a 45 df a3
  - Espera true

TestValidateFileSize_Match
  - actual == declared
  - Espera nil

TestValidateFileSize_Mismatch
  - actual != declared
  - Espera erro descritivo

TestPostFinishValidation_InvalidMagicBytes
  - Arquivo com conteúdo inválido
  - Chama handlePostFinish com esse arquivo
  - Verifica que status mudou para failed_upload no banco
  - Verifica que arquivo foi deletado do disco

TestPostFinishValidation_SizeMismatch
  - Arquivo cujo tamanho não bate com declared_size_bytes
  - Verifica failed_upload e arquivo deletado
```

Nota: testes de FFprobe requerem FFprobe instalado. Marque-os com `t.Skip`
se o binário não existir no ambiente de teste:
```go
if _, err := exec.LookPath("ffprobe"); err != nil {
    t.Skip("ffprobe não disponível")
}
```

## Dev Instructions

Crie `internal/upload/validation.go`:

### Função validateMagicBytes

```go
func validateMagicBytes(path string) (bool, error)
```

- Abre o arquivo e lê os primeiros 12 bytes
- Verifica contra as assinaturas conhecidas (tabela acima)
- Retorna `true` se reconhecido como container de vídeo

### Função validateFileSize

```go
func validateFileSize(actualBytes, declaredBytes int64) error
```

- Retorna nil se os tamanhos batem (tolerância zero)
- Retorna erro em português se divergirem

### Função runFFprobe

```go
type FFprobeResult struct {
    DurationS int
    Width     int
    Height    int
}

func runFFprobe(ctx context.Context, path string) (*FFprobeResult, error)
```

- Executa `ffprobe -v quiet -print_format json -show_streams {path}`
- Usa context com timeout de 5 segundos
- Parse do JSON de saída
- Extrai duration do primeiro stream com `codec_type: "video"`
- Retorna erro se FFprobe retornar código não-zero

### Função HandlePostFinish

Esta é a implementação do callback `onFinish` da T07:

```go
func HandlePostFinish(
    db *sql.DB,
    cfg *config.Config,
    enqueue func(videoID string) error,
    sendWebhook func(videoID string, event string, errMsg string),
    videoID string,
    filePath string,
)
```

Fluxo:
1. Busca o vídeo no banco para obter `declared_size_bytes`
2. Obtém tamanho real do arquivo: `os.Stat(filePath).Size()`
3. Valida tamanho: chama `validateFileSize`
4. Valida magic bytes: chama `validateMagicBytes`
5. Roda FFprobe para extrair duração e dimensões
6. Se qualquer validação falhar:
   - Marca `failed_upload` no banco com mensagem de erro
   - Deleta o arquivo: `os.Remove(filePath)` + `os.Remove(filePath + ".info")`
   - Chama `sendWebhook(videoID, "failed", errMsg)`
   - Retorna
7. Se tudo ok:
   - Chama `models.SetUploadComplete(db, videoID, actualSize)`
   - Chama `enqueue(videoID)` para colocar na fila de transcode
   - Chama `sendWebhook(videoID, "processing", "")`

## Arquivos a criar/modificar

- `internal/upload/validation.go`
- `internal/upload/validation_test.go`

## Definition of Done

- [ ] Magic bytes de MP4, MKV, AVI, QuickTime reconhecidos
- [ ] Tamanho divergente rejeita o arquivo
- [ ] FFprobe extrai duração e dimensões
- [ ] HandlePostFinish orquestra todo o fluxo
- [ ] Arquivo deletado em caso de falha
- [ ] Todos os testes passam (com skip para FFprobe se não disponível)
