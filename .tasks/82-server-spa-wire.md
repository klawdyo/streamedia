# T82 — Wire completo server.go: SPA /app, auth Google, RoleAuth, remoção legado

**Status:** done
**Depende de:** T78, T79, T80, T81
**Issue relacionada:** — (parte do admin unificado, spec/admin-unificado.md §1, §5-6)

## Objetivo

Integrar todas as peças do admin unificado no `server.go`: servir a SPA Vue em
`/app/*`, montar rotas Google OAuth, aplicar middleware RoleAuth em todos os
grupos protegidos, integrar dbconfig.Manager, e preparar a remoção do legado
(desativar rotas antigas de dashboard/playground/docs).

## QA Instructions

- Testar que SPA é servida em `/app` e `/app/*` com fallback para index.html
- Testar que rotas OAuth estão montadas e funcionais
- Testar que RoleAuth protege endpoints conforme matriz
- Testar que rotas legadas retornam 404 (foram removidas do router)
- Testar que dbconfig.Manager é inicializado no boot
- Testar integração completa: login Google → acessar /app → usar API

## Dev Instructions

1. Atualizar `internal/server/server.go`:
   - Adicionar grupo `/api/auth/*` com handlers Google OAuth
   - Adicionar grupo `/app/*` servindo SPA Vue de `SPA_DIR`
   - Aplicar RoleAuth nos grupos `/admin/*`
   - Criar `internal/spa/spa.go` com handler de SPA
2. Integrar dbconfig.Manager na inicialização
3. Remover ou comentar rotas legadas (dashboard, playground, docs) do router
4. Tratar SPA fallback: qualquer path sob `/app` que não seja asset → index.html

## Definition of Done

- [x] `internal/spa/spa.go` implementado com file server + fallback SPA
- [x] `internal/server/server.go` atualizado com todas as novas rotas
- [x] Rotas legadas (dashboard, playground, docs) retornam 404
- [x] dbconfig.Manager inicializado e injetado via config
- [x] `go build ./...` e `go test ./...` passam

## Resolução

**Data:** 2026-06-28
**Commit:** `ebe4a39` feat(T82,T90): wire server.go SPA /app, RoleAuth, spec admin-unificado

Arquivos criados/modificados:
- `internal/spa/spa.go`: Handler `NewSPAHandler(spaDir string) http.HandlerFunc` — serve arquivos estáticos de `SPA_DIR`, com fallback para `index.html` em qualquer path que não corresponda a um arquivo real (SPA routing). Headers: `Cache-Control: no-cache` para index.html, cache de 1 ano para assets com hash.
- `internal/server/server.go`: Reestruturado com novos grupos:
  - `/api/auth/google`, `/api/auth/google/callback`, `/api/auth/me`, `/api/auth/session` — handlers Google OAuth
  - `/app/*` — SPA handler com fallback
  - `/admin/videos`, `/admin/queue`, `/admin/stats` — RoleAuth(dev, admin, acl, manager)
  - `/admin/users` — RoleAuth(dev, admin, acl)
  - `/admin/config` — RoleAuth(dev, admin)
  - Rotas legadas removidas: `/playground`, `/dashboard/*`, `/docs`, `/docs/openapi.json`, `POST /admin/session`
- `spec/admin-unificado.md`: Criado como fonte temporária da especificação (removido em T90).

Decisão: SPA servida do disco (`SPA_DIR` env var, default `./web/dist`). Em desenvolvimento, Vite proxy. dbconfig.Manager injetado como dependência nos handlers que precisam de config dinâmica. Remoção física dos pacotes legados delegada para T89.
