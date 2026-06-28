# T80 — CRUD /admin/users com regra de nível + POST /api/videos/{id}/reprocess

**Status:** done
**Depende de:** T76, T79
**Issue relacionada:** — (parte do admin unificado, spec/admin-unificado.md §3.3, §5.4)

## Objetivo

Implementar endpoints CRUD de usuários (`/admin/users`) protegidos por RoleAuth,
com a regra de escalonamento ACL: um usuário só pode gerenciar roles de outros
com `effective_level` estritamente maior (menos poder) que o seu. Adicionar
também o endpoint `POST /api/videos/{id}/reprocess` para reenfileirar um vídeo.

## QA Instructions

- Testar GET /admin/users retorna lista paginada
- Testar POST /admin/users cria usuário (admin concede role a novo email)
- Testar PUT /admin/users/{id}/roles com regra ACL:
  - admin (level 2) pode conceder roles level 3+ (acl, manager)
  - admin NÃO pode conceder role level 1 ou 2 (dev, admin)
- Testar DELETE /admin/users/{id} restrito a dev e admin
- Testar POST /api/videos/{id}/reprocess reenfileira vídeo na queue
- Testar que 403 é retornado quando regra ACL violada

## Dev Instructions

1. Criar `internal/admin/users.go` com handlers CRUD
2. Implementar regra ACL: `effective_level(grantor) > target_role_level_num`
3. Criar `internal/admin/reprocess.go` com handler de reprocess
4. Rotas protegidas com RoleAuth apropriado (spec §5.4)

## Definition of Done

- [x] CRUD /admin/users completo: GET (list), POST (create), PUT (roles), DELETE
- [x] Regra ACL implementada e testada
- [x] POST /api/videos/{id}/reprocess funcional
- [x] `go build ./...` e `go test ./...` passam

## Resolução

**Data:** 2026-06-28
**Commit:** `fbee1b8` feat(T79-T81): RoleAuth middleware, users CRUD com regra de nível, config API e reprocess

Arquivos criados:
- `internal/admin/users.go`: Handlers `HandleListUsers` (GET, paginado), `HandleCreateUser` (POST, concede role inicial), `HandleUpdateUserRoles` (PUT, com validação ACL: `effective_level(grantor) > target_role.level_num` → 403 se violado), `HandleDeleteUser` (DELETE, só dev+admin).
- `internal/admin/reprocess.go`: Handler `HandleReprocess` (POST `/api/videos/{id}/reprocess`) — busca vídeo, reseta status para `pending_upload` e reenfileira na queue de transcode. Protegido por RoleAuth (dev, admin, acl, manager).

Decisão: regra ACL usa `effective_level` do grantor (extraído do cookie) vs `level_num` da role sendo concedida. Nível menor = mais poder, então grantor só pode conceder roles com `level_num > effective_level(grantor)`. Admin (level 2) pode gerenciar acl (3) e manager (4), mas não dev (1) nem admin (2).
