# T05: Model UploadToken

**Status:** pending
**Dependências:** T03
**Estimativa:** pequena

## Contexto

O `UploadToken` vincula um token HMAC a um único `video_id`. A constraint `UNIQUE`
no campo `video_id` da tabela `upload_tokens` garante que cada vídeo tem no máximo
um token ativo. Um UUID → um token → uma chance de upload.

O token é gerado pelo media server em `/upload/init` e armazenado no banco.
Durante o upload TUS, cada requisição valida que o token presente é válido e não
expirou.

## QA Instructions

Crie `internal/models/token_test.go`:

```
TestInsertToken_Success
  - Insere vídeo X no banco
  - Chama InsertUploadToken(db, token, videoID, expiresAt)
  - Verifica que não retorna erro

TestInsertToken_DuplicateVideoID
  - Insere vídeo X
  - Insere token T1 para vídeo X
  - Tenta inserir token T2 para vídeo X
  - Espera erro de UNIQUE constraint (um vídeo, um token)

TestInsertToken_VideoNotFound
  - Tenta inserir token para video_id inexistente
  - Espera erro de foreign key

TestGetToken_Found
  - Insere token
  - Busca por GetUploadToken(db, token)
  - Verifica VideoID e ExpiresAt corretos

TestGetToken_NotFound
  - Busca token inexistente
  - Espera sql.ErrNoRows ou equivalente

TestGetTokenByVideoID_Found
  - Insere token para vídeo X
  - Busca por GetUploadTokenByVideoID(db, videoID)
  - Verifica token correto retornado

TestDeleteToken
  - Insere e depois deleta token
  - Verifica que GetUploadToken retorna not found

TestDeleteExpiredTokens
  - Insere token expirado (expires_at no passado)
  - Insere token válido (expires_at no futuro)
  - Chama DeleteExpiredTokens(db)
  - Verifica que token expirado foi deletado
  - Verifica que token válido permanece

TestTokenExpired
  - Cria UploadToken com ExpiresAt no passado
  - Chama token.IsExpired()
  - Espera true

TestTokenValid
  - Cria UploadToken com ExpiresAt no futuro
  - Chama token.IsExpired()
  - Espera false
```

## Dev Instructions

### Struct UploadToken

```go
type UploadToken struct {
    Token     string
    VideoID   string
    ExpiresAt time.Time
}

func (t *UploadToken) IsExpired() bool {
    return time.Now().After(t.ExpiresAt)
}
```

### Funções de acesso ao banco

```go
func InsertUploadToken(db *sql.DB, token, videoID string, expiresAt time.Time) error
func GetUploadToken(db *sql.DB, token string) (*UploadToken, error)
func GetUploadTokenByVideoID(db *sql.DB, videoID string) (*UploadToken, error)
func DeleteUploadToken(db *sql.DB, token string) error
func DeleteExpiredTokens(db *sql.DB) (int64, error)  // retorna qtd deletada
```

`DeleteExpiredTokens` deleta onde `expires_at < CURRENT_TIMESTAMP`.
Retorna o número de linhas deletadas (para log).

## Arquivos a criar/modificar

- `internal/models/token.go`
- `internal/models/token_test.go`

## Definition of Done

- [ ] Struct `UploadToken` com método `IsExpired()`
- [ ] CRUD completo implementado
- [ ] UNIQUE constraint em video_id é tratada
- [ ] `DeleteExpiredTokens` funciona corretamente
- [ ] Todos os testes passam
