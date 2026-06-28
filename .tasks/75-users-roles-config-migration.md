# T75 — Migration SQL: tabelas users, user_roles, configurations

**Status:** done
**Depende de:** —
**Issue relacionada:** — (parte do admin unificado, spec/admin-unificado.md §2)

## Objetivo

Criar a migration `0004_users_config.sql` com as três novas tabelas do admin
unificado e aplicá-la via goose na inicialização do servidor.

## QA Instructions

- Verificar que a migration cria as 3 tabelas (`users`, `user_roles`, `configurations`)
- Verificar constraints: UNIQUE(email) em users, PK composta em user_roles, TEXT PK em configurations
- Verificar que goose aplica a migration idempotentemente
- Verificar que o servidor sobe sem erros com o banco limpo (migration roda)

## Dev Instructions

1. Criar arquivo `internal/db/migrations/0004_users_config.sql` com DDL das 3 tabelas
2. Confirmar que a migration é aplicada automaticamente em `db.Open()`

## Definition of Done

- [x] `internal/db/migrations/0004_users_config.sql` existe com DDL completo
- [x] Tabelas têm schema conforme spec: users (id, email UNIQUE, name, picture, created_at), user_roles (user_id FK, role, level_num, granted_by FK, granted_at, PK composta), configurations (key PK, value, type, description, group_key, validation, visible, updated_at)
- [x] Servidor aplica a migration automaticamente no boot
- [x] `go build ./...` e `go test ./...` passam

## Resolução

**Data:** 2026-06-28
**Commit:** `1561a36` feat(T75-T77): migration users/roles/configs, modelos Go e config manager com fallback

Arquivo de migration `internal/db/migrations/0004_users_config.sql` criado com:
- `users`: id INTEGER PK autoincrement, email TEXT UNIQUE NOT NULL, name TEXT DEFAULT '', picture TEXT DEFAULT '', created_at DATETIME DEFAULT CURRENT_TIMESTAMP
- `user_roles`: user_id INTEGER FK→users(id), role TEXT NOT NULL, level_num INTEGER NOT NULL, granted_by INTEGER FK→users(id) NULL, granted_at DATETIME DEFAULT CURRENT_TIMESTAMP, PK(user_id, role)
- `configurations`: key TEXT PK, value TEXT NOT NULL, type TEXT DEFAULT 'string', description TEXT DEFAULT '', group_key TEXT DEFAULT '', validation TEXT DEFAULT '', visible INTEGER DEFAULT 1, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP

Migration aplicada via goose automaticamente em `db.Open()`. Sem conflitos com migrations existentes (0001_init, 0002_video_webhook_url).
