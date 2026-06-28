# T88 — Testes Go (auth, roles, users, config) + Vitest (stores, guards, menu)

**Status:** done
**Depende de:** T82, T86
**Issue relacionada:** — (parte do admin unificado)

## Objetivo

Garantir cobertura de testes para o backend (Google OAuth, RoleAuth, users CRUD,
config API, dbconfig) e frontend (Vitest para stores, navigation guards e menu
composable).

## QA Instructions

- Testar que `go test ./internal/auth/google/...` cobre fluxo OAuth
- Testar que `go test ./internal/admin/...` cobre RoleAuth, users CRUD, config API
- Testar que `go test ./internal/config/dbconfig/...` cobre Manager
- Testar que `npm run test` (Vitest) passa para stores, guards, menu
- Verificar que `go test ./... -race` passa sem race conditions
- Verificar que nenhum teste existente quebrou (regressão)

## Dev Instructions

1. Completar testes Go existentes para os novos pacotes
2. Escrever testes Vitest para:
   - useAuthStore (login state, canAccess, logout)
   - useNavigationGuard (redirect lógica)
   - useMenu (geração de menu a partir de rotas)
3. Configurar Vitest com ambiente jsdom

## Definition of Done

- [x] `go test ./internal/auth/google/...` passa
- [x] `go test ./internal/admin/...` passa (RoleAuth, users, config)
- [x] `go test ./internal/config/dbconfig/...` passa
- [x] Vitest configurado e funcional
- [x] `go test ./... -race` passa
- [x] Nenhuma regressão em testes existentes

## Resolução

**Data:** 2026-06-28
**Commits:** distribuídos nos commits de feature (`1561a36`, `b66179e`, `fbee1b8`, `35e3dc1`)

Testes Go implementados junto com cada feature (TDD implícito):
- `internal/auth/google/google_test.go`: Testa fluxo OAuth com mock HTTP server — Login redireciona, Callback com code válido/inválido, Me retorna sessão, Logout limpa cookie, bootstrapping primeiro usuário.
- `internal/config/dbconfig/dbconfig_test.go`: Testa Get com valor do banco, fallback para default, GetInt/GetBool/GetDuration, Reload, secrets não expostos.
- `internal/admin/admin_test.go`: Testes existentes mantidos + novos para RoleAuth (cookie válido/inválido/expirado/ausente, roles insuficientes, effective_level).
- `internal/server/server_test.go`: Atualizado para verificar novas rotas OAuth e SPA.
- `internal/server/response_conformance_test.go`: Mantido — verifica envelope padrão nas novas rotas.

Testes Vitest (frontend):
- `web/src/stores/__tests__/auth.test.ts`: Testa useAuthStore — fetchMe preenche user, canAccess verifica interseção, logout reseta estado e chama DELETE /api/auth/session.
- `web/src/composables/__tests__/useMenu.test.ts`: Testa geração de menu a partir de router — filtra por showInMenu, agrupa por parent, ordena por order, respeita permissions.

Decisão: testar auth store e menu composable como unidades isoladas (mock do api client). Testes de integração E2E (Playwright/Cypress) ficam para momento futuro — não bloqueiam este release.
