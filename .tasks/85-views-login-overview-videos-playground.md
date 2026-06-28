# T85 — Views: Login, Overview, Videos, Video, Playground

**Status:** done
**Depende de:** T84
**Issue relacionada:** — (parte do admin unificado, spec/admin-unificado.md §6-7)

## Objetivo

Implementar as views principais do frontend: LoginView (Google OAuth),
OverviewView (dashboard com stats, gráficos e fila), VideosView (biblioteca
com paginação/filtros/ordenação), VideoView (player HLS + stats do vídeo), e
PlaygroundView (upload TUS + SSE + player HLS + documentação interativa da API).

## QA Instructions

- Testar LoginView: botão "Entrar com Google" redireciona para /api/auth/google
- Testar OverviewView: carrega stats, renderiza gráficos (Chart.js), mostra fila
- Testar VideosView: paginação, filtro por status/tag, ordenação, delete, reprocess
- Testar VideoView: player HLS funcional, stats do vídeo, SSE eventos ao vivo
- Testar PlaygroundView: upload TUS com progresso, SSE log, player após conclusão

## Dev Instructions

1. Criar `web/src/features/auth/views/LoginView.vue`
2. Criar `web/src/features/dashboard/views/OverviewView.vue` + components (StatsGrid, StatsCard, QueueWidget)
3. Criar `web/src/features/videos/views/VideosView.vue` + VideoTable + stores (videos, video)
4. Criar `web/src/features/videos/views/VideoView.vue`
5. Criar `web/src/features/playground/views/PlaygroundView.vue` + components (UploadForm, UploadProgress, SSELog, PlaybackPanel) + useApiDocs composable
6. Criar `web/src/components/player/VideoPlayer.vue` (compartilhado entre Video e Playground)
7. Criar `web/src/components/layout/` (AppLayout, AppSidebar, AppHeader, ThemeToggle)

## Definition of Done

- [x] LoginView funcional com botão Google OAuth
- [x] OverviewView com StatsGrid, gráficos Chart.js e QueueWidget
- [x] VideosView com paginação, filtros, ordenação, ações
- [x] VideoView com player HLS (hls.js) e painel de stats/SSE
- [x] PlaygroundView com upload TUS, progresso, SSE e player
- [x] Componentes de layout (sidebar, header, theme toggle)
- [x] `vue-tsc --noEmit` passa sem erros

## Resolução

**Data:** 2026-06-28
**Commit:** `35e3dc1` feat(T84-T86): frontend completo - router, stores, guards, menu, views e componentes

Views implementadas:
- `web/src/features/auth/views/LoginView.vue`: Tela centralizada com logo, botão "Entrar com Google" (redireciona para `/api/auth/google`), e mensagem de erro via query param.
- `web/src/features/dashboard/views/OverviewView.vue`: Cards de stats (total vídeos, prontos, processando, falha, espaço, duração), gráficos Chart.js (uploads/reproduções por data/dia/hora), fila atual, tabela de últimos vídeos.
- `web/src/features/dashboard/components/StatsGrid.vue`, `StatsCard.vue`: Grid responsivo de cards com ícone, valor formatado e label.
- `web/src/features/videos/views/VideosView.vue`: Tabela com paginação, filtros (status dropdown, tag input), ordenação (sort/order), ações (delete com confirm, reprocess). Store `videos.ts` com fetchVideos, deleteVideo, reprocessVideo.
- `web/src/features/videos/views/VideoView.vue`: Player HLS via `VideoPlayer.vue`, painel de metadados (status, resoluções, thumbnails, duração, tamanho), stats do vídeo (reproduções por data/resolução/SO). Store `video.ts` com fetchVideo, playInit, SSE.
- `web/src/features/playground/views/PlaygroundView.vue`: Upload TUS com `UploadForm.vue` (selecionar arquivo, configurar tag/tamanho/webhook), `UploadProgress.vue` (barra de progresso unificada), `SSELog.vue` (eventos SSE ao vivo), `PlaybackPanel.vue` (players HLS por resolução). Documentação interativa via `useApiDocs.ts`.
- `web/src/components/player/VideoPlayer.vue`: Componente compartilhado usando hls.js, com controles nativos e poster do thumbnail.
- `web/src/components/layout/AppLayout.vue`, `AppSidebar.vue`, `AppHeader.vue`, `ThemeToggle.vue`: Layout responsivo com sidebar colapsável, header com breadcrumb e toggle dark/light.
- `web/src/composables/useSSE.ts`: Composable para EventSource com reconexão automática.
- `web/src/composables/useTheme.ts`: Toggle dark/light com persistência em localStorage + classe no `<html>`.
- `web/src/features/playground/composables/useApiDocs.ts`: Documentação de todos os endpoints como array tipado.

Decisões: PlaygroundView reimplementa o playground legado como SPA Vue com os mesmos componentes do restante do app (tema consistente, mesma sidebar). SSE implementado com `EventSource` nativo e composable de reconexão.
