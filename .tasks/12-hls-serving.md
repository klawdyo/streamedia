# T12: Serving HLS estático + master.m3u8 autenticado

**Status:** pending
**Dependências:** T11, T06
**Estimativa:** média

## Contexto

Existem dois tipos de serving diferentes:

### 1. master.m3u8 — autenticado por HMAC

```
GET /videos/{video_id}/master.m3u8?expires={unix}&token={hex}
```

- Valida o token HMAC de reprodução (seção 5.2 da spec)
- Valida que `video_id` é UUID v4 estrito
- Valida que o vídeo está em status `ready` no banco
- Se válido: serve o arquivo `{MEDIA_DIR}/{video_id}/master.m3u8`
- Se inválido: 401 ou 403

### 2. Segmentos e playlists de resolução — estáticos públicos

```
GET /videos/{video_id}/{resolution}/playlist.m3u8
GET /videos/{video_id}/{resolution}/{segment}.ts
```

- Sem autenticação — são arquivos estáticos com nomes opacos
- O master.m3u8 é a "chave" que revela os caminhos
- `{resolution}` deve ser um dos valores permitidos: 480, 720, 1080
- Validação de path traversal: video_id deve ser UUID v4, resolution deve ser número
- **Directory listing DESABILITADO**

### Segurança

- Path traversal: rejeitar qualquer `video_id` ou `resolution` que não sigam o formato esperado
- Directory listing: o handler de arquivos estáticos deve ter listing desabilitado
- O status do vídeo é verificado APENAS para o master.m3u8 (não para segmentos individuais)

## QA Instructions

Crie `internal/serve/serve_test.go`:

```
TestMasterM3U8_ValidToken
  - Insere vídeo com status ready e cria master.m3u8 em temp dir
  - GET /videos/{id}/master.m3u8?expires={futuro}&token={válido}
  - Espera 200 e conteúdo do m3u8

TestMasterM3U8_InvalidToken
  - Token HMAC adulterado
  - Espera 401

TestMasterM3U8_ExpiredToken
  - expires no passado
  - Espera 401

TestMasterM3U8_VideoNotReady
  - Vídeo existe mas status = transcoding
  - Espera 404 ou 403

TestMasterM3U8_VideoIDPathTraversal
  - video_id = "../etc/passwd"
  - Espera 400

TestMasterM3U8_VideoNotFound
  - video_id válido mas vídeo não existe no banco
  - Espera 404

TestStaticSegment_ValidPath
  - Cria arquivo temporário simulando 0.ts
  - GET /videos/{id}/480/0.ts
  - Espera 200

TestStaticSegment_InvalidResolution
  - GET /videos/{id}/9999/0.ts (resolução não existe)
  - Espera 400 ou 404

TestStaticSegment_PathTraversal
  - GET /videos/{id}/480/../../../etc/passwd
  - Espera 400

TestStaticServing_NoDirectoryListing
  - GET /videos/{id}/480/ (listagem de diretório)
  - Espera 404 ou 403 (nunca 200 com listagem)

TestStaticSegment_SegmentNotFound
  - Arquivo .ts inexistente
  - Espera 404
```

## Dev Instructions

Crie `internal/serve/serve.go`:

### Handler para master.m3u8

```go
type MasterHandler struct {
    cfg *config.Config
    db  *sql.DB
}

func NewMasterHandler(cfg *config.Config, db *sql.DB) *MasterHandler

func (h *MasterHandler) ServeHTTP(w http.ResponseWriter, r *http.Request)
```

Fluxo:
1. Extrai `video_id` do path (via chi: `chi.URLParam(r, "videoID")`)
2. Valida UUID v4
3. Extrai `expires` e `token` dos query params
4. Valida token: `auth.ValidatePlayToken(secret, videoID, expires, token, maxTTL)`
5. Busca vídeo no banco, verifica `status == ready`
6. Serve o arquivo: `http.ServeFile(w, r, filepath)`

### Handler para arquivos estáticos (sem listing)

```go
type StaticHandler struct {
    cfg *config.Config
}

func NewStaticHandler(cfg *config.Config) *StaticHandler

func (h *StaticHandler) ServeHTTP(w http.ResponseWriter, r *http.Request)
```

Fluxo:
1. Extrai `video_id` e `resolution` do path
2. Valida UUID v4 para video_id
3. Valida resolution: deve ser "480", "720" ou "1080"
4. Valida o filename do segmento: só dígitos + extensão .ts, ou "playlist.m3u8"
5. Constrói o path: `cfg.MediaDir + "/" + videoID + "/" + resolution + "/" + filename`
6. Verifica que o path resultante começa com `cfg.MediaDir` (proteção extra contra traversal)
7. Serve com `http.ServeFile`

### Desabilitar directory listing

Use um wrapper do `http.FileSystem` que retorna 404 para diretórios:

```go
type noListFS struct {
    base http.FileSystem
}

func (f noListFS) Open(name string) (http.File, error) {
    file, err := f.base.Open(name)
    if err != nil {
        return nil, err
    }
    stat, err := file.Stat()
    if err != nil || stat.IsDir() {
        file.Close()
        return nil, os.ErrNotExist
    }
    return file, nil
}
```

## Arquivos a criar/modificar

- `internal/serve/serve.go`
- `internal/serve/serve_test.go`

## Definition of Done

- [ ] master.m3u8 validado por HMAC e status do vídeo
- [ ] Segmentos estáticos servidos sem autenticação
- [ ] Directory listing desabilitado
- [ ] Path traversal bloqueado em todas as rotas
- [ ] Erros de auth retornam 401 (não 500)
- [ ] Todos os testes passam
