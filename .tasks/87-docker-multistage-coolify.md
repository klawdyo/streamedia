# T87 — Dockerfile multi-stage + docker-compose Coolify final

**Status:** done
**Depende de:** T82, T85
**Issue relacionada:** — (parte do admin unificado, spec/admin-unificado.md §8)

## Objetivo

Atualizar o Dockerfile para build multi-stage (Node+Vue → Go → Alpine+FFmpeg)
e o docker-compose.yml para produção com as novas env vars do admin unificado
(GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET, GOOGLE_REDIRECT_URL, SPA_DIR) e
remoção das env vars migradas para o banco.

## QA Instructions

- Verificar que Dockerfile multi-stage compila sem erros
- Verificar que imagem final contém binário Go + web/dist/
- Verificar que docker-compose.yml expõe apenas PORT (sem ports: fixos)
- Verificar que env vars obsoletas foram removidas do compose
- Verificar que novas env vars (GOOGLE_*) estão documentadas

## Dev Instructions

1. Atualizar `Dockerfile`: stage 1 node:22-alpine (npm ci + npm run build), stage 2 golang:1.26-alpine (go build com SPA_DIR), stage 3 alpine:3.20 + ffmpeg (copia binário + web/dist)
2. Atualizar `docker-compose.yml`: adicionar GOOGLE_* env vars, remover vars migradas ao banco, usar expose em vez de ports
3. Atualizar `.env.example` com as novas variáveis

## Definition of Done

- [x] Dockerfile multi-stage funcional (3 stages)
- [x] docker-compose.yml atualizado com env vars corretas
- [x] `.env.example` inclui GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET, GOOGLE_REDIRECT_URL
- [x] `docker build` conclui sem erros
- [x] `go build ./...` e `go test ./...` passam

## Resolução

**Data:** 2026-06-28
**Commit:** `dd7ca5d` feat(T87): Dockerfile multi-stage (Node+Vue → Go → Alpine) e docker-compose atualizado

Arquivos modificados:
- `Dockerfile`: 3 stages:
  1. `ui-build`: node:22-alpine, copia web/, `npm ci` (omitindo devDependencies), `npm run build` → web/dist/
  2. `go-build`: golang:1.26-alpine, copia fontes Go + web/dist/ do stage 1, `CGO_ENABLED=0 go build -ldflags="-X ...Version=$(cat VERSION)" -o /mediaserver ./cmd/server`
  3. `runtime`: alpine:3.20, `RUN apk add --no-cache ffmpeg`, copia binário do stage 2 + web/dist/, `USER nobody`, `EXPOSE ${PORT:-3000}`, `CMD ["./mediaserver"]`
- `docker-compose.yml`: Serviço com `expose: ["${PORT:-3000}"]` (Coolify), env vars atualizadas:
  - Mantidas: ROOT_TOKEN, WEBHOOK_SECRET, PORT, SQLITE_PATH, MEDIA_DIR, UPLOAD_TMP_DIR, ENV
  - Novas: GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET, GOOGLE_REDIRECT_URL, SPA_DIR=/app/web/dist, SESSION_COOKIE_SECURE
  - Removidas (agora no DB): MAX_UPLOAD_SIZE_MB, QUEUE_MAX_SIZE, TRANSCODE_WORKERS, UPLOAD_TOKEN_TTL, PLAY_TOKEN_TTL, UPLOAD_IDLE_TIMEOUT, TRANSCODE_STUCK, MAX_TRANSCODE_ATTEMPTS, KEEP_ORIGINAL, RATE_LIMIT_PER_MIN, WEBHOOK_URL, DISCORD_WEBHOOK_URL
- `.env.example`: Atualizado com as 4 novas env vars Google OAuth + SPA_DIR.

Decisão: ffmpeg instalado via apk no stage runtime (não no go-build). Binário compilado com CGO_ENABLED=0. SPA_DIR aponta para /app/web/dist dentro do container.
