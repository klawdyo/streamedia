# T79 — Middleware RoleAuth: autorização por roles em cada endpoint

**Status:** done
**Depende de:** T76
**Issue relacionada:** — (parte do admin unificado, spec/admin-unificado.md §5.3-5.4)

## Objetivo

Criar middleware `RoleAuth` que extrai o session cookie da requisição, valida o
HMAC e verifica se o usuário possui pelo menos uma das roles exigidas para a
rota. Implementar a regra de nível efetivo (`effective_level = MIN(level_num)`)
e proteger todos os endpoints conforme a matriz de permissões da spec §5.4.

## QA Instructions

- Testar que rota sem cookie retorna 401
- Testar que rota com cookie inválido/adulterado retorna 401
- Testar que rota com cookie expirado retorna 401
- Testar que usuário sem a role exigida retorna 403
- Testar que usuário com role suficiente acessa normalmente
- Testar `effective_level` = MIN(level_num) entre múltiplas roles
- Testar que `ROOT_TOKEN` bypassa RoleAuth (rotas de backend)

## Dev Instructions

1. Criar `internal/admin/roleauth.go` com middleware `RoleAuth(db, roles ...string) func(http.Handler) http.Handler`
2. Middleware parseia cookie, valida HMAC com ROOT_TOKEN, verifica roles
3. Se já autenticado por `RootAuth` (ROOT_TOKEN), bypassa RoleAuth
4. Aplicar em todos os grupos de rota conforme spec §5.4

## Definition of Done

- [x] Middleware `RoleAuth` funcional em `internal/admin/roleauth.go`
- [x] Todas as rotas protegidas conforme matriz de permissões da spec
- [x] Regra de `effective_level` = MIN(level_num) implementada
- [x] `go build ./...` e `go test ./...` passam

## Resolução

**Data:** 2026-06-28
**Commit:** `fbee1b8` feat(T79-T81): RoleAuth middleware, users CRUD com regra de nível, config API e reprocess

Arquivos criados/modificados:
- `internal/admin/admin.go`: Middleware `RoleAuth(database *sql.DB, requiredRoles ...string)` que extrai cookie, valida HMAC com ROOT_TOKEN, verifica interseção entre roles do usuário e requiredRoles. Se requisição já passou por `RootAuth` (ROOT_TOKEN), bypassa. Injeta `SessionClaims` no contexto.
- `internal/auth/auth.go`: Função `ParseSessionCookie` extraída do google.go para reuso. `EffectiveLevel(roles []UserRole) int` = MIN(level_num).

Matriz de permissões implementada conforme spec §5.4: /admin/users requer dev/admin/acl, /admin/config requer dev/admin (DELETE só dev), /admin/videos requer todos autenticados, /metrics mantido só ROOT_TOKEN.
