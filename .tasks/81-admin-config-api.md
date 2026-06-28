# T81 — Config API: GET/PUT/DELETE /admin/config com validação e agrupamento

**Status:** done
**Depende de:** T77, T79
**Issue relacionada:** — (parte do admin unificado, spec/admin-unificado.md §5.5)

## Objetivo

Implementar a API de configurações dinâmicas: GET retorna agrupado por
`group_key` com título e descrição; PUT atualiza valor com validação de tipo e
regex; DELETE remove do banco (fallback para default). Configs `visible=0`
(secrets) nunca são retornadas no GET, só aceitam PUT.

## QA Instructions

- Testar GET /admin/config retorna groups[] com items (sem secrets)
- Testar PUT /admin/config/{key} atualiza valor e retorna config atualizada
- Testar validação de tipo: número inválido rejeitado, booleano inválido rejeitado
- Testar validação regex: valor que não casa com validation regex é rejeitado
- Testar DELETE /admin/config/{key} remove e fallback para default
- Testar que config visible=0 não aparece no GET
- Testar que config visible=0 aceita PUT (mas valor nunca é exposto)

## Dev Instructions

1. Criar `internal/admin/config_api.go` com handlers
2. GET /admin/config agrupa por group_key, ordenado alfabeticamente
3. PUT /admin/config/{key} valida type (int, bool, duration) e validation regex
4. DELETE /admin/config/{key} restrito a role `dev`
5. Integrar com dbconfig.Manager para Reload após PUT/DELETE

## Definition of Done

- [x] GET /admin/config funcional com agrupamento e sem secrets
- [x] PUT /admin/config/{key} com validação de tipo e regex
- [x] DELETE /admin/config/{key} restrito a dev
- [x] Manager.Reload() chamado após cada PUT/DELETE
- [x] `go build ./...` e `go test ./...` passam

## Resolução

**Data:** 2026-06-28
**Commit:** `fbee1b8` feat(T79-T81): RoleAuth middleware, users CRUD com regra de nível, config API e reprocess

Arquivo criado:
- `internal/admin/config_api.go`: Handlers `HandleListConfig` (GET, agrupado por group_key com título/descrição, secrets omitidos), `HandleUpdateConfig` (PUT, valida type: number→atoi, boolean→parseBool, duration→ParseDuration; valida validation regex se presente), `HandleDeleteConfig` (DELETE, restrito a role `dev`).

Agrupamento: cada grupo tem `key`, `title`, `description` e `items[]`. Títulos em português: paths→"Caminhos", upload→"Upload", transcode→"Transcodificação", token→"Tokens", rate_limit→"Rate Limiting", webhook→"Webhook", discord→"Discord", session→"Sessão".

Integração com dbconfig.Manager: após PUT/DELETE bem-sucedidos, `mgr.Reload()` é chamado para refletir mudanças imediatamente sem restart.
