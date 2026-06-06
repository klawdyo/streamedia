# T07: Handler TUS (tusd como biblioteca)

**Status:** pending
**Dependências:** T05, T06
**Estimativa:** média

## Contexto

O protocolo TUS (tus.io) permite uploads resumíveis. O cliente Flutter envia os
chunks do vídeo diretamente ao media server via TUS.

A biblioteca `github.com/tus/tusd/v2` é usada COMO BIBLIOTECA, não como serviço
separado. Isso significa que o handler TUS é embutido no mesmo processo Go que
o resto do servidor.

### Rotas TUS

```
POST   /files/{video_id}    criação do upload
PATCH  /files/{video_id}    envio de chunk
HEAD   /files/{video_id}    consulta de offset (retomada)
```

### Armazenamento

Os arquivos brutos ficam em `UPLOAD_TMP_DIR` (padrão: `/media/.uploads`).
O tusd usa dois arquivos por upload: `{id}` (dado) e `{id}.info` (metadata TUS).

### Autenticação TUS

Cada requisição TUS deve apresentar o token de upload no header:
`Upload-Token: {token}`

O handler valida que:
1. O token existe na tabela `upload_tokens`
2. O token não está expirado
3. O token pertence ao `video_id` da URL

### Hooks do tusd

O tusd oferece hooks (callbacks) para eventos do ciclo de upload:
- `pre-create`: antes de criar o upload — validar token e tamanho
- `post-receive`: após cada chunk — atualizar `last_chunk_at` e status
- `post-finish`: quando upload completa — validar arquivo e enfileirar transcode

A implementação real do `post-finish` (validação de magic bytes, etc.) é feita
na T09. Nesta tarefa, o hook `post-finish` pode ser um stub que apenas loga.

## QA Instructions

Crie `internal/upload/tus_test.go`:

```
TestTUSHandlerCreation
  - Cria um TUSHandler com config e db válidos
  - Verifica que não retorna erro
  - Verifica que o handler http.Handler resultante não é nil

TestTUSPreCreateHook_ValidToken
  - Cria upload com token válido no header Upload-Token
  - Espera que o hook pré-create aceite (não retorne erro)

TestTUSPreCreateHook_InvalidToken
  - Requisição sem token ou com token inválido
  - Espera rejeição com status 401

TestTUSPreCreateHook_ExpiredToken
  - Token presente mas expirado
  - Espera rejeição com status 401

TestTUSPreCreateHook_SizeExceedsLimit
  - Header Upload-Length > MaxUploadSizeBytes
  - Espera rejeição com status 413

TestTUSPreCreateHook_VideoIDMismatch
  - Token válido mas para video_id diferente do da URL
  - Espera rejeição com status 403

TestTUSPostReceiveHook_UpdatesLastChunkAt
  - Simula recebimento de chunk
  - Verifica que last_chunk_at foi atualizado no banco
  - Verifica que status mudou para "uploading"
```

Use um banco SQLite em memória e um diretório temporário para armazenamento TUS.

## Dev Instructions

Crie `internal/upload/tus.go`:

### Struct TUSHandler

```go
type TUSHandler struct {
    handler http.Handler
    store   filestore.FileStore
    config  *config.Config
    db      *sql.DB
}

func NewTUSHandler(cfg *config.Config, db *sql.DB, onFinish func(videoID string)) (*TUSHandler, error)
```

O parâmetro `onFinish` é um callback que será chamado quando o upload completar.
Na T09, esse callback fará a validação do arquivo. Por agora, use um stub.

### Configuração do tusd

```go
// O tusd usa um FileStore para persistir os chunks em disco.
// O diretório de armazenamento é cfg.UploadTmpDir.
store := filestore.FileStore{Path: cfg.UploadTmpDir}

// O Composer registra os DataStores suportados.
composer := tusd.NewStoreComposer()
store.UseIn(composer)

// Config do tusd
tusConfig := tusd.Config{
    BasePath:              "/files/",
    StoreComposer:         composer,
    MaxSize:               cfg.MaxUploadSizeBytes,
    PreUploadCreateCallback: preCreateHook,
    PostReceiveCallback:     postReceiveHook,
    PostFinishCallback:      postFinishHook,
}
```

### Hook pre-create

- Extrai o video_id do path `/files/{video_id}`
- Valida UUID v4 do video_id
- Extrai token do header `Upload-Token`
- Busca token no banco, valida que não expirou e pertence ao video_id
- Rejeita se Upload-Length > MaxUploadSizeBytes

### Hook post-receive

- Busca video_id a partir do upload ID
- Atualiza `last_chunk_at` no banco
- Atualiza status para `uploading` se ainda estiver `pending_upload`

### Hook post-finish

Por agora, apenas chama o callback `onFinish(videoID)`.
A validação real é implementada na T09.

### Método ServeHTTP

```go
func (h *TUSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request)
```

Delega para o handler interno do tusd.

## Arquivos a criar/modificar

- `internal/upload/tus.go`
- `internal/upload/tus_test.go`

## Definition of Done

- [ ] TUSHandler criado com configuração correta
- [ ] Hook pre-create valida token, tamanho, UUID
- [ ] Hook post-receive atualiza last_chunk_at e status
- [ ] Hook post-finish chama callback onFinish
- [ ] Todos os testes passam
