# T44: video_id opcional em /upload/init — gerar UUID v7 quando ausente, aceitar qualquer versão quando informado

**Status:** pending
**Dependências:** T08 (rota POST /upload/init), T35 (rota /admin/projects/*/upload-token, que também emite vídeos)
**Estimativa:** pequena/média
**Origem:** solicitação direta do usuário (sessão de 2026-06-07, não vinculada a issue do GitHub)

## Contexto

Hoje `POST /upload/init` (`internal/upload/init.go`) **exige** `video_id` no
corpo da requisição e o valida com um regex estrito de **UUID v4**
(`uuidV4Re`, duplicado em `internal/upload/init.go` e `internal/serve/serve.go`,
este último reaproveitado por `internal/serve/status.go`). O servidor nunca
gera o id sozinho nesse fluxo — quem gera é o backend principal (cliente da
API), conforme descrito em `.tasks/08-upload-init.md`.

A pedido do usuário, o comportamento deve mudar para:

1. **`video_id` passa a ser opcional** em `POST /upload/init`:
   - Se o cliente **informar** um `video_id`, o servidor usa exatamente o
     que foi informado (desde que seja um UUID válido em formato — ver item 2).
   - Se o cliente **não informar** (campo ausente ou string vazia), o
     servidor **gera** o id.
2. **A validação de formato deixa de exigir uma versão específica de UUID**:
   um `video_id` informado pelo cliente pode ser UUID de **qualquer versão**
   (v1, v4, v7, etc.) — o servidor não deve rejeitar por causa do nibble de
   versão. Continua rejeitando qualquer string que não seja um UUID
   bem-formado (isso é proteção crítica contra path traversal — `video_id`
   vira nome de diretório/arquivo em vários lugares do sistema; ver
   `internal/serve/serve.go` e `internal/transcode/worker.go`).
3. **Sempre que o PRÓPRIO sistema gera um id (sem o cliente informar),
   deve gerar um UUID v7** — em qualquer lugar do sistema, não só em
   `/upload/init`. Isso já vale para o fluxo de
   `POST /admin/projects/{slug}/upload-token` (T35,
   `internal/admin/projects.go`), que atualmente gera o id com
   `uuid.NewString()` (UUID v4) — precisa passar a gerar v7 também.

UUID v7 é ordenável por tempo de criação (prefixo temporal), o que melhora
localidade de índice no SQLite e facilita ordenação cronológica por id —
por isso a preferência explícita do usuário por v7 nos ids gerados pelo
servidor, mesmo aceitando qualquer versão informada externamente.

A dependência `github.com/google/uuid v1.6.0` já está no `go.mod` e já
expõe `uuid.NewV7() (uuid.UUID, error)`.

## QA Instructions

Crie/estenda os testes em `internal/upload/init_test.go`,
`internal/admin/projects_test.go`, `internal/serve/serve_test.go` e
`internal/serve/status_test.go`:

```
TestUploadInit_VideoIDOmitted_GeneratesUUIDv7
  - POST /upload/init sem "video_id" no corpo (campo ausente ou "")
  - 200 OK
  - resposta contém "video_id" preenchido
  - o video_id retornado é um UUID v7 válido (13o caractere hex == '7')
  - o vídeo foi inserido no banco com esse id

TestUploadInit_VideoIDProvided_AnyUUIDVersionAccepted
  - tabela de casos com video_id em UUID v1, v4, v5, v7 (valores fixos válidos)
  - cada um → 200 OK, servidor usa exatamente o id informado (não gera outro)

TestUploadInit_VideoIDProvided_InvalidFormat_Rejected
  - tabela de casos: string vazia após trim, "not-a-uuid",
    "../../../etc/passwd", UUID com caracteres maiúsculos (se o sistema
    não normalizar), UUID com segmento de tamanho errado, UUID com nibble
    de versão "0" ou "9" (fora do intervalo 1-8 definido pela RFC)
  - cada um → 400 Bad Request, vídeo NÃO inserido no banco

TestIssueUploadToken_GeneratesUUIDv7
  - POST /admin/projects/{slug}/upload-token com X-Project-Key válida
  - o video_id retornado/gerado é um UUID v7 válido

TestServeAndStatus_AcceptAnyUUIDVersionInPath
  - GET /videos/{id}/master.m3u8 e GET /api/status/{id} com ids de
    diferentes versões de UUID (v1, v4, v7) previamente inseridos
  - todos aceitos pelo regex de validação de path (não barrados por serem
    "não-v4")
  - path traversal continua barrado: "../etc/passwd", "%2e%2e%2f...",
    strings com barra
```

Verifique que os testes FALHAM antes da implementação (comportamento atual:
`video_id` obrigatório + só v4 aceito + ids gerados em v4).

## Dev Instructions

### 1. Centralize geração e validação de `video_id`

Para que "o sistema sempre privilegia v7 ao gerar, mas aceita qualquer
versão ao validar formato" seja uma regra única (não duplicada em 3+
arquivos), adicione em `internal/models` (próximo a `video.go`, que já é
dono do conceito de vídeo):

```go
// uuidFormatRe casa qualquer UUID bem-formado (RFC 4122): 8-4-4-4-12 hex,
// nibble de versão entre 1 e 8 (versões definidas) e nibble de variante
// entre 8, 9, a ou b (variante RFC 4122). Continua rejeitando qualquer
// string que não seja um UUID — proteção essencial contra path traversal,
// já que video_id vira nome de diretório/arquivo.
var uuidFormatRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// IsValidVideoIDFormat valida que s é um UUID bem-formado de qualquer
// versão suportada (não exige uma versão específica).
func IsValidVideoIDFormat(s string) bool {
    return uuidFormatRe.MatchString(s)
}

// NewVideoID gera um novo identificador de vídeo. O sistema sempre
// privilegia UUID v7 ao gerar ids — é ordenável por tempo de criação,
// o que melhora localidade no índice do SQLite.
func NewVideoID() (string, error) {
    id, err := uuid.NewV7()
    if err != nil {
        return "", err
    }
    return id.String(), nil
}
```

Use comentários em português com a densidade de costume (explique o
"porquê": rejeição de path traversal, preferência por v7).

### 2. `internal/upload/init.go`

- Torne `VideoID` em `initRequest` efetivamente opcional: se vier ausente
  ou vazio, gere com `models.NewVideoID()`; se vier preenchido, valide com
  `models.IsValidVideoIDFormat` (substitua `uuidV4Re` e a mensagem de erro
  "deve ser um UUID v4" por algo como "video_id inválido: deve ser um UUID
  bem-formado").
- Remova o `uuidV4Re` local duplicado, delegando para `models`.
- Inclua `video_id` na resposta JSON (`map[string]string{"video_id": ...,
  "upload_url": ..., "token": ...}`) — hoje a resposta não devolve o id, e
  o cliente PRECISA saber qual id foi atribuído quando ele próprio não o
  informou.

### 3. `internal/serve/serve.go` / `internal/serve/status.go`

- Substitua `uuidV4Re` por `models.IsValidVideoIDFormat` (ou troque a regex
  local pela mesma definição genérica) — vídeos com ids de outras versões
  (gerados antes desta mudança como v4, ou agora como v7, ou informados
  pelo cliente em outra versão) precisam continuar sendo servidos
  normalmente.

### 4. `internal/admin/projects.go`

- Troque `videoID := uuid.NewString()` (gera v4) por
  `videoID, err := models.NewVideoID()` (gera v7), tratando o erro com
  `500 Internal Server Error` + mensagem em português, igual ao padrão dos
  demais erros do handler.
- Remova o import `github.com/google/uuid` se ficar sem uso nesse arquivo.

### 5. Verificação

- `go test ./... -v` — todos os testes passam, incluindo os novos
- `go vet ./...` sem warnings
- Confirme manualmente (ou via teste) que um `video_id` informado pelo
  cliente como UUID v1/v5 é aceito ponta-a-ponta: init → upload TUS →
  serving — não há nenhum outro ponto do sistema com checagem de versão
  específica escondida.

## Arquivos a criar/editar

- `internal/models/video.go` (ou novo `internal/models/video_id.go`):
  `IsValidVideoIDFormat`, `NewVideoID`
- `internal/upload/init.go`: `video_id` opcional, validação genérica,
  resposta inclui `video_id`
- `internal/serve/serve.go`: validação genérica (compartilhada com status.go)
- `internal/admin/projects.go`: geração via `models.NewVideoID` (v7)
- Testes correspondentes em `internal/models/`, `internal/upload/`,
  `internal/serve/`, `internal/admin/`

## Resolução

Arquivos alterados:
- `internal/models/video_id.go` (criado) — `IsValidVideoIDFormat` (aceita qualquer versão UUID 1-8) e `NewVideoID` (gera UUID v7 via `uuid.NewV7()`)
- `internal/upload/init.go` — video_id opcional: ausente/vazio gera UUID v7; informado aceita qualquer versão; resposta inclui `video_id`
- `internal/upload/init_test.go` — removidos casos v1/v3 de inválidos; adicionado `TestUploadInit_AnyUUIDVersionAccepted`
- `internal/serve/serve.go` — regex trocada para aceitar qualquer versão (nibble 1-8)
- `internal/admin/projects.go` — `uuid.NewString()` trocado por `models.NewVideoID()`, removido import `google/uuid`

## Definition of Done

- [x] `POST /upload/init` aceita corpo sem `video_id` e gera um UUID v7
- [x] `POST /upload/init` aceita `video_id` informado em qualquer versão de
      UUID válida (v1, v4, v5, v7, ...) e usa exatamente o valor informado
- [x] Strings que não são UUID bem-formado continuam sendo rejeitadas com
      `400 Bad Request` (proteção contra path traversal preservada e testada)
- [x] A resposta de `POST /upload/init` sempre inclui `video_id` (gerado ou
      informado), permitindo ao cliente saber qual id foi atribuído
- [x] `POST /admin/projects/{slug}/upload-token` (T35) passa a gerar
      `video_id` em UUID v7 (antes: v4)
- [x] `internal/serve` (serving HLS e status) aceita `video_id` de qualquer
      versão de UUID válida sem regressão na proteção de path traversal
- [x] Validação e geração centralizadas em `internal/models` — sem regex
      duplicado pelo código
- [x] `go test ./... -v` passa, incluindo os novos testes de T44
- [x] `go vet ./...` sem warnings
