# T04: Model Video + máquina de estados

**Status:** pending
**Dependências:** T03
**Estimativa:** média

## Contexto

O model `Video` representa um vídeo em qualquer etapa do ciclo de vida.
A máquina de estados controla quais transições são válidas e bloqueia as inválidas.

## Estados e transições válidas

```
pending_upload   → uploading | failed_upload
uploading        → uploading | upload_complete | failed_upload
upload_complete  → transcoding
transcoding      → transcoding | ready | failed_transcode
```

Estados terminais (nunca são alterados por jobs ou código após entrar):
- `failed_upload`
- `failed_transcode`

O estado `ready` não é terminal no sentido de que o registro permanece,
mas nenhuma transição sai dele.

## Campo resolutions

O campo `resolutions` no banco é uma string JSON, ex: `[480,720,1080]`.
O model deve serializar/deserializar automaticamente.

## QA Instructions

Crie `internal/models/video_test.go`:

```
TestVideoStatusTransitions_Valid
  - pending_upload → uploading: deve ser aceita
  - uploading → upload_complete: deve ser aceita
  - upload_complete → transcoding: deve ser aceita
  - transcoding → ready: deve ser aceita

TestVideoStatusTransitions_Invalid
  - ready → uploading: deve retornar erro
  - failed_upload → uploading: deve retornar erro (terminal)
  - failed_transcode → transcoding: deve retornar erro (terminal)
  - pending_upload → ready: deve retornar erro (salto inválido)

TestVideoCreate
  - Insere vídeo com InsertVideo(db, videoID, declaredSize)
  - Verifica que status inicial é "pending_upload"
  - Verifica que transcode_attempts é 0
  - Verifica que created_at não é zero

TestVideoGet_NotFound
  - Chama GetVideo(db, "uuid-inexistente")
  - Espera sql.ErrNoRows ou erro equivalente

TestVideoUpdateStatus
  - Insere vídeo
  - Atualiza status para "uploading"
  - Busca novamente e verifica que status mudou

TestVideoDuplicateID
  - Insere vídeo com UUID X
  - Tenta inserir outro vídeo com mesmo UUID X
  - Espera erro de constraint (UNIQUE PRIMARY KEY)

TestVideoResolutionsSerialization
  - Cria video com Resolutions: []int{480, 720}
  - Salva no banco
  - Lê de volta
  - Verifica que Resolutions == []int{480, 720}

TestVideoTransitionBlocksTerminal
  - Insere vídeo, vai para failed_upload via UpdateStatus
  - Chama UpdateStatus novamente tentando ir para "uploading"
  - Espera erro de transição inválida
```

Use banco SQLite em memória com `db.Open(":memory:")`.

## Dev Instructions

### Struct Video

```go
type VideoStatus string

const (
    StatusPendingUpload  VideoStatus = "pending_upload"
    StatusUploading      VideoStatus = "uploading"
    StatusUploadComplete VideoStatus = "upload_complete"
    StatusTranscoding    VideoStatus = "transcoding"
    StatusReady          VideoStatus = "ready"
    StatusFailedUpload   VideoStatus = "failed_upload"
    StatusFailedTranscode VideoStatus = "failed_transcode"
)

type Video struct {
    VideoID           string
    Status            VideoStatus
    DeclaredSizeBytes int64
    ActualSizeBytes   int64
    DurationS         int
    Resolutions       []int      // serializado como JSON no banco
    TranscodeAttempts int
    LastChunkAt       *time.Time
    ErrorMessage      string
    CreatedAt         time.Time
    UpdatedAt         time.Time
}
```

### Funções de acesso ao banco

```go
func InsertVideo(db *sql.DB, videoID string, declaredSize int64) error
func GetVideo(db *sql.DB, videoID string) (*Video, error)
func UpdateStatus(db *sql.DB, videoID string, newStatus VideoStatus) error
func UpdateStatusWithError(db *sql.DB, videoID string, newStatus VideoStatus, errMsg string) error
func UpdateLastChunk(db *sql.DB, videoID string) error
func SetUploadComplete(db *sql.DB, videoID string, actualSize int64) error
func SetReady(db *sql.DB, videoID string, durationS int, resolutions []int) error
func IncrementTranscodeAttempts(db *sql.DB, videoID string) error
func ListByStatus(db *sql.DB, status VideoStatus) ([]*Video, error)
```

### Máquina de estados

```go
// validTransitions define as transições permitidas.
// Mapa de: estado atual → estados destino permitidos.
var validTransitions = map[VideoStatus][]VideoStatus{
    StatusPendingUpload:  {StatusUploading, StatusFailedUpload},
    StatusUploading:      {StatusUploading, StatusUploadComplete, StatusFailedUpload},
    StatusUploadComplete: {StatusTranscoding},
    StatusTranscoding:    {StatusTranscoding, StatusReady, StatusFailedTranscode},
}

func isValidTransition(from, to VideoStatus) bool
```

`UpdateStatus` deve verificar a transição antes de executar o UPDATE.
Se inválida, retorna erro em português: "Transição de estado inválida: X → Y"

### Serialização de resolutions

Use `encoding/json` para marshal/unmarshal do campo `resolutions`.

## Arquivos a criar/modificar

- `internal/models/video.go`
- `internal/models/video_test.go`

## Definition of Done

- [ ] Struct `Video` com todos os campos
- [ ] Todas as funções de CRUD implementadas
- [ ] Máquina de estados valida todas as transições
- [ ] Estados terminais bloqueiam qualquer transição de saída
- [ ] Serialização de resolutions funciona
- [ ] Todos os testes passam
