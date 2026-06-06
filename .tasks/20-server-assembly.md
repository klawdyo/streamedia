# T20: Montagem do servidor — chi + todas as rotas

**Status:** pending
**Dependências:** T08, T09, T10, T11, T12, T13, T14, T15, T16, T17, T18, T19
**Estimativa:** média

## Contexto

Integra todos os handlers em um único servidor HTTP chi. Esta é a tarefa de
"montagem final" que liga todas as peças criadas anteriormente.

### Todas as rotas

```
POST   /upload/init                        InitHandler (T08) + backend auth middleware
POST   /files/{video_id}                   TUSHandler (T07) — TUS create
PATCH  /files/{video_id}                   TUSHandler (T07) — TUS patch
HEAD   /files/{video_id}                   TUSHandler (T07) — TUS head

GET    /videos/{videoID}/master.m3u8       MasterHandler (T12) — autenticado
GET    /videos/{videoID}/{res}/playlist.m3u8  StaticHandler (T12) — público
GET    /videos/{videoID}/{res}/{segment}   StaticHandler (T12) — público

GET    /api/status/{videoID}               StatusHandler (T13) + backend auth middleware

GET    /admin/videos                       AdminHandler (T18) + admin auth middleware
GET    /admin/queue                        AdminHandler (T18) + admin auth middleware

GET    /healthz                            health check inline
```

### Middlewares globais

- Rate limiting (T19) — para todas as rotas
- Logging de requests (duração, status, path)
- Recovery de panic (retorna 500 em vez de crashar)
- CORS se necessário (configurável)

### /healthz

Resposta simples: `{ "status": "ok" }` com 200.
Verificações adicionais opcionais: ping no banco SQLite.

### Inicialização do servidor

O `cmd/server/main.go` deve:
1. Carregar config (`config.Load()`)
2. Abrir banco (`db.Open(cfg.SQLitePath)`)
3. Criar fila de transcode (`transcode.NewQueue(...)`)
4. Criar worker FFmpeg (`transcode.NewWorker(...)`)
5. Criar cliente de webhook (`webhook.NewClient(...)`)
6. Criar todos os handlers
7. Criar e iniciar jobs de manutenção
8. Montar o router chi com todas as rotas
9. Iniciar o servidor HTTP
10. Graceful shutdown em SIGINT/SIGTERM

### Graceful shutdown

```go
ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
defer cancel()
// Inicia servidor
// Aguarda ctx.Done()
// Para os jobs
// Para a fila (drena)
// Fecha banco
// HTTP server.Shutdown(5s timeout)
```

## QA Instructions

Crie `internal/server_test.go` (ou `cmd/server/integration_test.go`):

```
TestHealthz
  - GET /healthz
  - Espera 200 com {"status":"ok"}

TestHealthz_DBCheck
  - Servidor com banco válido
  - /healthz deve retornar 200

TestRouteNotFound
  - GET /rota-que-nao-existe
  - Espera 404

TestRateLimitApplied
  - Sobe servidor completo
  - Faz muitas requests para /healthz do mesmo IP
  - Eventualmente recebe 429

TestAllRoutesRegistered
  - Verifica que as rotas principais existem (POST /upload/init retorna 401, não 404)
  - (404 significaria rota não registrada; 401 significa rota existe mas auth falhou)

TestUploadInitE2E
  - Sobe servidor completo com banco em memória
  - POST /upload/init com HMAC correto
  - Espera 200 com upload_url e token

TestGracefulShutdown
  - Inicia servidor em goroutine
  - Envia SIGTERM
  - Verifica que servidor para sem panic
```

## Dev Instructions

### Atualizar cmd/server/main.go

Substitua o stub da T01 pela implementação real:

```go
func main() {
    // Carrega configuração — falha se variáveis obrigatórias ausentes
    cfg, err := config.Load()
    if err != nil {
        log.Fatalf("Falha ao carregar configuração: %v", err)
    }

    // Abre banco SQLite com WAL e foreign keys
    database, err := db.Open(cfg.SQLitePath)
    if err != nil {
        log.Fatalf("Falha ao abrir banco de dados: %v", err)
    }
    defer database.Close()

    // ... inicializa componentes ...
    // ... monta router ...
    // ... inicia servidor ...
    // ... graceful shutdown ...
}
```

### Crie internal/server/server.go

Extraia a lógica de montagem do servidor para um pacote testável:

```go
func NewRouter(cfg *config.Config, db *sql.DB, queue *transcode.Queue) http.Handler
```

Isso permite testar o router sem iniciar um processo.

### Logging middleware

Use `chi/middleware.Logger` ou implemente um simples:
- Formato: `[2006-01-02 15:04:05] METHOD /path 200 123ms`

### Recovery middleware

Use `chi/middleware.Recoverer` para capturar panics e retornar 500.

## Arquivos a criar/modificar

- `cmd/server/main.go` (implementação real)
- `internal/server/server.go` (montagem do router)
- `internal/server/server_test.go`

## Definition of Done

- [ ] Todas as rotas registradas e funcionando
- [ ] Rate limiting global ativo
- [ ] /healthz retorna 200
- [ ] Graceful shutdown funciona (não trava)
- [ ] Logs de request em cada requisição
- [ ] Todos os testes passam
