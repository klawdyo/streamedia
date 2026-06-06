# Manifest de Tarefas — Streamedia

Atualizado pelo agente CTO a cada transição de estado.
Status possíveis: `pending` | `in-progress` | `done` | `blocked`

## Progresso geral

```
Total: 25 tarefas
Done:  7
```

## Lista de tarefas

| # | Arquivo | Título | Status | Notas |
|---|---------|--------|--------|-------|
| T01 | `.tasks/01-scaffold.md` | Scaffold do projeto Go | done | go 1.25 (tusd exige) |
| T02 | `.tasks/02-config.md` | Pacote de configuração | done | |
| T03 | `.tasks/03-database.md` | Camada SQLite | done | |
| T04 | `.tasks/04-video-model.md` | Model Video + máquina de estados | done | |
| T05 | `.tasks/05-token-model.md` | Model UploadToken | done | |
| T06 | `.tasks/06-hmac-auth.md` | Pacote de autenticação HMAC | done | |
| T07 | `.tasks/07-tus-handler.md` | Handler TUS (tusd como biblioteca) | done | auth no ServeHTTP (preCreate não cobre POST /files/{id}) |
| T08 | `.tasks/08-upload-init.md` | Rota POST /upload/init | pending | depende T07 |
| T09 | `.tasks/09-upload-validation.md` | Hook post-finish: validação do arquivo | pending | depende T08 |
| T10 | `.tasks/10-transcode-queue.md` | Fila de transcodificação (channel + workers) | pending | depende T04 |
| T11 | `.tasks/11-ffmpeg-worker.md` | Worker FFmpeg: geração HLS | pending | depende T10 |
| T12 | `.tasks/12-hls-serving.md` | Serving HLS estático + master.m3u8 autenticado | pending | depende T11, T06 |
| T13 | `.tasks/13-status-route.md` | Rota GET /api/status/{video_id} | pending | depende T04 |
| T14 | `.tasks/14-job-upload-killer.md` | Job 1: killer de uploads inativos | pending | depende T04 |
| T15 | `.tasks/15-job-transcode-requeue.md` | Job 2: reenfileirador de transcodes travados | pending | depende T10 |
| T16 | `.tasks/16-job-token-cleanup.md` | Job 3: limpeza de tokens expirados | pending | depende T05 |
| T17 | `.tasks/17-webhook-client.md` | Cliente de webhook com retry | pending | depende T04 |
| T18 | `.tasks/18-admin-routes.md` | Rotas admin (/admin/videos, /admin/queue) | pending | depende T04, T10 |
| T19 | `.tasks/19-rate-limit.md` | Middleware de rate limiting por IP | pending | depende T01 |
| T20 | `.tasks/20-server-assembly.md` | Montagem do servidor: chi + todas as rotas | pending | depende T08-T19 |
| T21 | `.tasks/21-startup-recovery.md` | Recuperação de crash na inicialização | pending | depende T10 |
| T22 | `.tasks/22-docker-config.md` | Dockerfile + docker-compose + .env.example | pending | depende T20 |
| T23 | `.tasks/23-github-actions.md` | GitHub Actions: ci.yml + release.yml | pending | depende T22 |
| T24 | `.tasks/24-readme.md` | README.md completo | pending | depende T22 |
| T25 | `.tasks/25-integration-tests.md` | Suite de testes de integração completa | pending | depende T20 |

## Log de mudanças de status

<!-- CTO registra aqui cada transição com data/hora -->
<!-- Formato: [YYYY-MM-DD HH:MM] TNN: pending → in-progress -->
[2026-06-06 20:30] T01: pending → in-progress
[2026-06-06 20:35] T01: in-progress → done
