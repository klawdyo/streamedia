# T16: Job 3 — Limpeza de tokens expirados

**Status:** pending
**Dependências:** T05
**Estimativa:** pequena

## Contexto

Tokens de upload expiram após `UPLOAD_TOKEN_TTL_H` horas. Após a expiração, o
token não pode mais ser usado, mas o registro fica na tabela até este job de
limpeza rodá-lo.

### Regra

```
expires_at < agora
→ DELETE da tabela upload_tokens
→ O video_id permanece na tabela videos com seu status atual
```

A deleção do token NÃO afeta o vídeo. Se o upload já completou e o vídeo está
em `ready`, ele permanece assim. O token é apenas a credencial de upload.

### Execução

Roda uma vez por dia com `time.Ticker` de 24 horas.

## QA Instructions

Crie `internal/jobs/cleanup_test.go`:

```
TestCleanupJob_DeletesExpiredToken
  - Insere vídeo + token com expires_at = 1 hora atrás
  - Roda o job
  - Verifica: token foi deletado da tabela upload_tokens
  - Verifica: vídeo ainda existe na tabela videos

TestCleanupJob_KeepsValidToken
  - Insere token com expires_at = 2 horas no futuro
  - Roda o job
  - Verifica: token ainda existe

TestCleanupJob_VideoStatusUnchanged
  - Insere vídeo com status ready + token expirado
  - Roda o job
  - Verifica: status do vídeo ainda é ready

TestCleanupJob_MultipleExpired
  - Insere 5 tokens expirados e 2 válidos
  - Roda o job
  - Verifica: apenas os 5 expirados foram deletados

TestCleanupJob_EmptyTable
  - Tabela de tokens vazia
  - Roda o job
  - Não deve retornar erro

TestCleanupJob_LogsDeletedCount
  - Job retorna ou loga o número de tokens deletados
  - Verifica que a contagem está correta
```

## Dev Instructions

Crie `internal/jobs/cleanup.go`:

### Struct TokenCleanupJob

```go
type TokenCleanupJob struct {
    db     *sql.DB
    ticker *time.Ticker
    stopCh chan struct{}
}

func NewTokenCleanupJob(db *sql.DB) *TokenCleanupJob
```

### Método runOnce (testável isoladamente)

```go
func (j *TokenCleanupJob) runOnce() (int64, error)
```

Delega para `models.DeleteExpiredTokens(j.db)` (implementado na T05).
Retorna o número de tokens deletados.

Loga: "Limpeza de tokens: %d tokens expirados removidos."

### Método Run e Stop

Ticker de 24 horas. Mesma estrutura dos outros jobs.

## Arquivos a criar/modificar

- `internal/jobs/cleanup.go`
- `internal/jobs/cleanup_test.go`

## Definition of Done

- [ ] Deleta tokens com `expires_at < now()`
- [ ] Não deleta tokens válidos
- [ ] Não afeta tabela videos
- [ ] Todos os testes passam
