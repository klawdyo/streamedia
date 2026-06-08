# T61: Corrigir `AdminAuth` — erro de banco silenciado como 401

**Status:** done
**Dependências:** T18, T33
**Estimativa:** pequena
**Origem:** análise de código — erro silenciado
**Severidade:** alta

## Contexto

Em `internal/admin/admin.go:94-99`, o middleware `AdminAuth` tenta resolver
o projeto pela chave mestra:

```go
if project, err := models.GetProjectByMasterKeyHash(db, models.HashMasterKey(token)); err == nil {
    projectID := project.ID
    ctx := context.WithValue(r.Context(), adminScopeContextKey{}, &projectID)
    next.ServeHTTP(w, r.WithContext(ctx))
    return
}
```

Se `GetProjectByMasterKeyHash` retornar um erro que **não** seja
`sql.ErrNoRows` (ex.: banco indisponível, timeout, conexão perdida, disco
cheio), o erro cai silenciosamente no `else` e responde **401 Unauthorized**.

O operador vê "Não autorizado" e acha que o token está errado, quando na
verdade o banco está com problema. O erro real fica invisível — sem log,
sem status 500.

## Impacto

- **Erros de infraestrutura disfarçados como erros de autenticação** —
  dificulta diagnóstico e triagem.
- Operador pode perder tempo trocando tokens quando o problema é no banco.
- Em cenário de degradação do banco, TODAS as requisições admin recebem
  401 em vez de 500.

## QA Instructions

```
TestAdminAuth_DBError_Returns500
  - Configura token que NÃO é o ADMIN_TOKEN global
  - Mock do banco retornando erro genérico (não sql.ErrNoRows)
  - Verifica que a resposta é 500 (Internal Server Error)
  - Verifica que a mensagem NÃO é "Não autorizado"

TestAdminAuth_NotFound_Returns401
  - Configura token que NÃO é o ADMIN_TOKEN global
  - Mock do banco retornando sql.ErrNoRows
  - Verifica que a resposta é 401
```

## Dev Instructions

### 1. Distinguir "não encontrado" de "erro de banco"

```go
project, err := models.GetProjectByMasterKeyHash(db, models.HashMasterKey(token))
if err == nil {
    // Sucesso: chave mestra válida, propaga escopo do projeto
    projectID := project.ID
    ctx := context.WithValue(r.Context(), adminScopeContextKey{}, &projectID)
    next.ServeHTTP(w, r.WithContext(ctx))
    return
}
if err != sql.ErrNoRows {
    // Erro de infraestrutura — não é "não encontrado"
    log.Printf("[admin] erro ao buscar projeto por chave mestra: %v", err)
    apiresponse.Error(w, http.StatusInternalServerError, "Erro interno ao validar credenciais.")
    return
}

// sql.ErrNoRows: token não corresponde a nenhum projeto → 401
apiresponse.Error(w, http.StatusUnauthorized, "Não autorizado.")
```

### 2. Adicionar import de `database/sql` se necessário

O pacote `sql` já está importado (vide `sql.NullInt64` em HandleVideos).

## Arquivos a editar

- `internal/admin/admin.go` (distinguir erros no AdminAuth)

## Resolução

Arquivos alterados:
- `internal/admin/admin.go`: AdminAuth agora separa o fluxo em 3 caminhos:
  err == nil (sucesso), err != sql.ErrNoRows (500 + log), sql.ErrNoRows (401).
  Adicionado import de `log`.

## Definition of Done

- [x] Erro de banco (não sql.ErrNoRows) retorna 500 com log
- [x] Token não encontrado (sql.ErrNoRows) retorna 401
- [x] Token válido continua funcionando normalmente
- [x] `go test ./internal/admin/...` passa
- [x] `go test ./...` sem regressões
- [x] `go vet ./...` limpo
