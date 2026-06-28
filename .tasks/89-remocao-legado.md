# T89 — Remoção de legado: dashboard, playground, docs, POST /admin/session

**Status:** done
**Depende de:** T82
**Issue relacionada:** — (parte do admin unificado, spec/admin-unificado.md §9)

## Objetivo

Remover fisicamente os pacotes e rotas legados substituídos pelo admin
unificado: `internal/dashboard/`, `internal/playground/`, `internal/docs/`,
e a rota `POST /admin/session` (substituída por Google OAuth).

## QA Instructions

- Verificar que `internal/dashboard/` foi removido do disco
- Verificar que `internal/playground/` foi removido do disco
- Verificar que `internal/docs/` foi removido do disco
- Verificar que `POST /admin/session` retorna 404
- Verificar que GET /playground, /dashboard/*, /docs, /docs/openapi.json retornam 404
- Verificar que nenhum import quebrado em outros pacotes
- Verificar que `go build ./...` e `go test ./...` passam

## Dev Instructions

1. Remover `internal/dashboard/` (dashboard.go, HTMLs, assets/)
2. Remover `internal/playground/` (playground.go, index.html)
3. Remover `internal/docs/` (docs.go, spec.go)
4. Remover handler `POST /admin/session` de server.go
5. Confirmar que router não referencia nenhum dos pacotes removidos
6. Rodar `go vet ./...` para detectar imports órfãos

## Definition of Done

- [x] `internal/dashboard/` removido completamente
- [x] `internal/playground/` removido completamente
- [x] `internal/docs/` removido completamente
- [x] `POST /admin/session` removido do router
- [x] Rotas legadas retornam 404 (não 500 por handler nil)
- [x] `go build ./...`, `go vet ./...`, `go test ./...` passam

## Resolução

**Data:** 2026-06-28
**Commit:** `21500f8` chore(T89,T90): remoção de dashboard, playground, docs legados; versioner sync package.json

Pacotes removidos:
- `internal/dashboard/`: dashboard.go, overview.html, videos.html, video.html, dashboard_test.go, assets/app.js, assets/theme.css — dashboard HTML legado substituído pelo SPA Vue em /app
- `internal/playground/`: playground.go, index.html, playground_test.go — playground legado substituído pelo PlaygroundView Vue
- `internal/docs/`: docs.go, docs_test.go, spec.go (OpenAPI) — documentação movida para o composable `useApiDocs.ts` no frontend Vue

Handler removido:
- `POST /admin/session`: removido de server.go — sessão agora é gerenciada pelo cookie Google OAuth (stateless HMAC)

Rotas removidas do router:
- `GET /playground`
- `GET /dashboard`, `GET /dashboard/videos`, `GET /dashboard/videos/{id}`, `GET /dashboard/assets/*`
- `GET /docs`, `GET /docs/openapi.json`
- `POST /admin/session`

Verificação pós-remoção:
- `go build ./...` limpo (sem imports órfãos)
- `go vet ./...` limpo
- `go test ./...` todos passam (testes que referenciavam os pacotes removidos foram removidos junto)
- Nenhuma referência residual nos arquivos de spec (api.md, operacao.md atualizados)

Decisão: remoção hard-delete (não comentar/desabilitar) — o Git preserva o histórico se necessário. Código morto não tem lugar no branch principal.
