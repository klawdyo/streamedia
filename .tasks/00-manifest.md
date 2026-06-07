# Manifest de Tarefas — Streamedia

Atualizado pelo agente CTO a cada transição de estado.
Status possíveis: `pending` | `in-progress` | `done` | `blocked`

## Progresso geral

```
Total: 30 tarefas
Done:  26
Pending: 4
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
| T08 | `.tasks/08-upload-init.md` | Rota POST /upload/init | done | |
| T09 | `.tasks/09-upload-validation.md` | Hook post-finish: validação do arquivo | done | |
| T10 | `.tasks/10-transcode-queue.md` | Fila de transcodificação (channel + workers) | done | |
| T11 | `.tasks/11-ffmpeg-worker.md` | Worker FFmpeg: geração HLS | done | |
| T12 | `.tasks/12-hls-serving.md` | Serving HLS estático + master.m3u8 autenticado | done | |
| T13 | `.tasks/13-status-route.md` | Rota GET /api/status/{video_id} | done | |
| T14 | `.tasks/14-job-upload-killer.md` | Job 1: killer de uploads inativos | done | |
| T15 | `.tasks/15-job-transcode-requeue.md` | Job 2: reenfileirador de transcodes travados | done | |
| T16 | `.tasks/16-job-token-cleanup.md` | Job 3: limpeza de tokens expirados | done | |
| T17 | `.tasks/17-webhook-client.md` | Cliente de webhook com retry | done | |
| T18 | `.tasks/18-admin-routes.md` | Rotas admin (/admin/videos, /admin/queue) | done | |
| T19 | `.tasks/19-rate-limit.md` | Middleware de rate limiting por IP | done | |
| T20 | `.tasks/20-server-assembly.md` | Montagem do servidor: chi + todas as rotas | done | |
| T21 | `.tasks/21-startup-recovery.md` | Recuperação de crash na inicialização | done | depende T10 |
| T22 | `.tasks/22-docker-config.md` | Dockerfile + docker-compose + .env.example | done | depende T20 |
| T23 | `.tasks/23-github-actions.md` | GitHub Actions: ci.yml + release.yml | done | depende T22 |
| T24 | `.tasks/24-readme.md` | README.md completo | done | depende T22 |
| T25 | `.tasks/25-integration-tests.md` | Suite de testes de integração completa | done | depende T20 |
| T26 | `.tasks/26-playback-stats-model.md` | Model + armazenamento de eventos de reprodução/upload (estatísticas) | done | depende T03, T04 — issue #2 |
| T27 | `.tasks/27-playback-stats-collection.md` | Coleta de eventos de estatísticas nos handlers de serving/upload | pending | depende T26, T07, T09, T12 — issue #2 |
| T28 | `.tasks/28-stats-aggregation-route.md` | Rota administrativa de estatísticas agregadas (`/admin/stats`) | pending | depende T26, T27, T18 — issue #2 |
| T29 | `.tasks/29-opentelemetry-metrics-route.md` | Rota de métricas no padrão OpenTelemetry/Prometheus (`/metrics`) | pending | depende T20, T26 — issue #1 |
| T30 | `.tasks/30-swagger-docs.md` | Documentação da API via Swagger/OpenAPI | pending | depende T20, T13, T18, T28, T29 — issue #3 |

## Log de mudanças de status

<!-- CTO registra aqui cada transição com data/hora -->
<!-- Formato: [YYYY-MM-DD HH:MM] TNN: pending → in-progress -->
[2026-06-06 20:30] T01: pending → in-progress
[2026-06-06 20:35] T01: in-progress → done
[2026-06-06 20:50] T08: pending → in-progress
[2026-06-06 20:55] T08: in-progress → done
[2026-06-06 20:55] T09: pending → in-progress
[2026-06-06 21:05] T09: in-progress → done
[2026-06-06 21:05] T10: pending → in-progress
[2026-06-06 21:15] T10: in-progress → done
[2026-06-06 21:15] T11: pending → in-progress
[2026-06-06 21:30] T11: in-progress → done
[2026-06-06 21:30] T12: pending → in-progress
[2026-06-07 00:30] T12: in-progress → done
[2026-06-07 00:30] T13: pending → in-progress
[2026-06-07 00:30] T14: pending → in-progress
[2026-06-07 00:35] T14: in-progress → done
[2026-06-07 00:35] T15: pending → in-progress
[2026-06-07 00:50] T13: in-progress → done
[2026-06-07 00:50] T15: in-progress → done
[2026-06-07 00:50] T16: pending → in-progress
[2026-06-07 00:50] T17: pending → in-progress
[2026-06-07 00:50] T18: pending → in-progress
[2026-06-07 00:50] T19: pending → in-progress
[2026-06-07 01:10] T16: in-progress → done
[2026-06-07 01:10] T17: in-progress → done
[2026-06-07 01:10] T18: in-progress → done
[2026-06-07 01:10] T19: in-progress → done
[2026-06-07 01:10] T20: pending → in-progress
[2026-06-07 01:25] T20: in-progress → done
[2026-06-07 01:25] T21: pending → in-progress
[2026-06-07 02:00] T21: in-progress → done
[2026-06-07 02:00] T22: pending → in-progress
[2026-06-07 02:00] T23: pending → in-progress
[2026-06-07 02:15] T22: in-progress → done
[2026-06-07 02:15] T23: in-progress → done
[2026-06-07 02:30] T24: pending → in-progress
[2026-06-07 02:45] T24: in-progress → done
[2026-06-07 02:45] T25: pending → in-progress
[2026-06-07 03:00] T25: in-progress → done
[2026-06-07 03:30] T26-T30 criadas a partir das issues #1, #2 e #3 (próxima onda de funcionalidades: estatísticas de uso, métricas OpenTelemetry e documentação Swagger)
[2026-06-07 03:45] T26: pending → in-progress
[2026-06-07 04:00] T26: in-progress → done
