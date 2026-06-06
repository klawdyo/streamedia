# T18: Rotas admin (/admin/videos, /admin/queue)

**Status:** pending
**Dependências:** T04, T10
**Estimativa:** pequena

## Contexto

Dashboard mínimo para operação e debug. As rotas admin são protegidas por um
token com role `admin`.

### Autenticação admin

O token admin é um token de reprodução (mesmo formato HMAC) com campo adicional.
Para simplificar: qualquer token HMAC válido gerado com `UPLOAD_TOKEN_SECRET`
e query param `role=admin` é aceito.

Header: `Authorization: Bearer {token}`
Onde token = `HMAC-SHA256(UPLOAD_TOKEN_SECRET, "admin:" + timestamp_unix)`

Alternativamente (mais simples): use um `ADMIN_TOKEN` fixo na config, comparado
em tempo constante. O Dev deve escolher a abordagem mais simples e segura.

### Rotas

```
GET /admin/videos
  Retorna lista de vídeos paginada.
  Query params: ?status={status}&limit={n}&offset={n}
  Default: limit=50, offset=0, todos os status

GET /admin/queue
  Retorna estado atual da fila de transcode.
  { "queue_length": 3, "workers": 1, "videos_in_queue": ["id1", "id2", "id3"] }
```

## QA Instructions

Crie `internal/admin/admin_test.go`:

```
TestAdminVideos_WithAuth
  - GET /admin/videos com token admin válido
  - Espera 200 com JSON de lista

TestAdminVideos_WithoutAuth
  - GET /admin/videos sem token
  - Espera 401

TestAdminVideos_FilterByStatus
  - Insere vídeos com status variados
  - GET /admin/videos?status=ready
  - Verifica que apenas vídeos ready aparecem

TestAdminVideos_Pagination
  - Insere 10 vídeos
  - GET /admin/videos?limit=3&offset=0 → 3 vídeos
  - GET /admin/videos?limit=3&offset=3 → próximos 3

TestAdminQueue_WithAuth
  - GET /admin/queue com auth válida
  - Espera 200 com queue_length e workers

TestAdminQueue_ShowsCurrentLength
  - Enfileira 2 itens antes da request
  - Verifica que queue_length = 2

TestAdminVideos_InvalidStatus
  - status=nao_existe não deve causar 500
  - Deve retornar lista vazia ou 400 com mensagem
```

## Dev Instructions

Crie `internal/admin/admin.go`:

### Autenticação admin

Adicione `ADMIN_TOKEN` às variáveis de configuração (T02 pode precisar de update).
Token fixo comparado com `subtle.ConstantTimeCompare`.

Middleware de admin:

```go
func AdminAuth(adminToken string) func(http.Handler) http.Handler
```

Verifica: `Authorization: Bearer {adminToken}` em tempo constante.

### Handler AdminVideos

```go
func (h *AdminHandler) HandleVideos(w http.ResponseWriter, r *http.Request)
```

- Lê params: `status`, `limit` (padrão 50, máx 200), `offset` (padrão 0)
- Consulta `models.ListByStatus` ou query customizada com paginação
- Retorna JSON com array de vídeos + total count

### Handler AdminQueue

```go
func (h *AdminHandler) HandleQueue(w http.ResponseWriter, r *http.Request)
```

- Retorna `queue.Len()` e número de workers configurados

### Struct AdminHandler

```go
type AdminHandler struct {
    cfg   *config.Config
    db    *sql.DB
    queue interface{ Len() int }
}
```

## Arquivos a criar/modificar

- `internal/admin/admin.go`
- `internal/admin/admin_test.go`
- Atualizar `internal/config/config.go` para adicionar `AdminToken`
- Atualizar `.env.example` com `ADMIN_TOKEN`

## Definition of Done

- [ ] Autenticação admin em tempo constante
- [ ] Listagem de vídeos com filtro e paginação
- [ ] Status da fila retornado corretamente
- [ ] Todos os testes passam
