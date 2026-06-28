# T76 — Modelos Go: User, UserRole, Configuration + queries

**Status:** done
**Depende de:** T75
**Issue relacionada:** — (parte do admin unificado, spec/admin-unificado.md §2-3)

## Objetivo

Criar os modelos Go para as 3 novas entidades e implementar queries CRUD no
pacote `internal/models`.

## QA Instructions

- Testar INSERT/SELECT de User (email único, autoincrement)
- Testar INSERT/SELECT de UserRole (PK composta, validação de role)
- Testar INSERT/SELECT/UPDATE de Configuration (key primária)
- Testar regra de unicidade de email em User
- Testar que `GetUserByEmail` retorna nil para email inexistente

## Dev Instructions

1. Criar `internal/models/user.go` com struct User + queries (InsertUser, GetUserByEmail, GetUserByID, ListUsers, DeleteUser)
2. Criar `internal/models/user_role.go` ou adicionar struct UserRole em user.go + queries (GetUserRoles, GrantRole, RevokeRole)
3. Criar `internal/models/configuration.go` com struct Configuration + queries (GetConfig, SetConfig, DeleteConfig, ListConfigs)
4. Usar o padrão existente de queries parametrizadas do pacote models

## Definition of Done

- [x] `internal/models/user.go` com User struct e queries completas
- [x] `internal/models/configuration.go` com Configuration struct e queries completas
- [x] UserRole integrado no modelo de User (roles carregadas via JOIN ou query separada)
- [x] `go build ./...` e `go test ./...` passam

## Resolução

**Data:** 2026-06-28
**Commit:** `1561a36` feat(T75-T77): migration users/roles/configs, modelos Go e config manager com fallback

Arquivos criados:
- `internal/models/user.go`: struct User (ID, Email, Name, Picture, CreatedAt) + queries `InsertUser`, `GetUserByEmail`, `GetUserByID`, `ListUsers`, `DeleteUser`, `GrantRole`, `RevokeRole`, `GetUserRoles`. UserRole embedded com campos Role, LevelNum, GrantedBy, GrantedAt.
- `internal/models/configuration.go`: struct Configuration (Key, Value, Type, Description, GroupKey, Validation, Visible, UpdatedAt) + queries `GetConfig`, `GetAllConfigs`, `SetConfig`, `DeleteConfig`.

Decisão: User e UserRole no mesmo arquivo (`user.go`) por coesão de domínio. Configuration em arquivo separado. Todas queries usam prepared statements com validação de erros (padrão do pacote models).
