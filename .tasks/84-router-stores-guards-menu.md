# T84 — Router + stores + guards + menu + api client

**Status:** done
**Depende de:** T83
**Issue relacionada:** — (parte do admin unificado, spec/admin-unificado.md §6.3-6.7)

## Objetivo

Implementar a infraestrutura de navegação e estado do frontend: router com
RouteMeta estendido (permissions, icon, showInMenu), Pinia stores (auth com
fetchMe/canAccess/logout), navigation guards, menu composable, e api client
com CSRF protection.

## QA Instructions

- Testar que router bloqueia rotas sem permissão (redireciona para login)
- Testar que usuário logado é redirecionado de /app/auth para /app/overview
- Testar que canAccess verifica interseção de roles
- Testar que logout reseta todas as stores (resetAllStores)
- Testar que api client adiciona header CSRF em chamadas não-GET
- Testar que menu é gerado dinamicamente a partir do router meta

## Dev Instructions

1. Criar `web/src/router/index.ts`: rotas com RouteMeta estendido (spec §6.4)
2. Criar `web/src/stores/auth.ts`: useAuthStore com fetchMe, canAccess, logout, resetAll
3. Criar `web/src/composables/useNavigationGuard.ts`: beforeEach hook (spec §6.6)
4. Criar `web/src/composables/useMenu.ts`: gera menu a partir do router + permissões (spec §6.5)
5. Criar `web/src/api/client.ts`: fetch wrapper com CSRF, error handling

## Definition of Done

- [x] Router com 7 rotas configuradas e RouteMeta estendido
- [x] useAuthStore funcional (login state, fetchMe, canAccess, logout)
- [x] Navigation guard bloqueia rotas não autorizadas
- [x] Menu gerado dinamicamente com nesting e ícones
- [x] Api client com CSRF e tratamento de erro
- [x] `vue-tsc --noEmit` passa sem erros

## Resolução

**Data:** 2026-06-28
**Commit:** `35e3dc1` feat(T84-T86): frontend completo - router, stores, guards, menu, views e componentes

Arquivos criados:
- `web/src/types/index.ts`: Tipos TypeScript — User, Video, Role, ConfigGroup, ConfigItem, Session, PlayInfo, etc.
- `web/src/router/index.ts`: 7 rotas com RouteMeta estendido (title, permissions, showInMenu, icon, parent, order). Rotas: /app/auth (Login), /app/overview (Overview), /app/videos (Videos), /app/videos/:id (Video), /app/playground (Playground), /app/users (Users), /app/config (Config).
- `web/src/stores/auth.ts`: useAuthStore com estado `user` (ref<User|null>), `checked` (ref<boolean>). `fetchMe()` chama GET /api/auth/me, `canAccess(roles)` verifica interseção, `logout()` chama DELETE /api/auth/session + resetAllStores.
- `web/src/composables/useNavigationGuard.ts`: beforeEach que verifica auth.checked, redireciona login→overview se já logado, overview→login se não logado, overview se sem permissão.
- `web/src/composables/useMenu.ts`: Gera menu a partir de router.getRoutes(), filtra por showInMenu e canAccess, agrupa por parent, ordena por order.
- `web/src/api/client.ts`: Função `api<T>(method, path, body?)` — adiciona `X-Requested-With: XMLHttpRequest` (CSRF), trata respostas de erro (401→logout, 403→overview), parseia envelope padrão `{error, message, data}`.

Decisão: stores Pinia usam setup syntax (`defineStore('x', () => { ... })`). Reset no logout limpa TODAS as stores (não só auth) para evitar vazamento de dados entre sessões.
