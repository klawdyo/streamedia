# Manifest de Tarefas — Streamedia

Atualizado pelo agente CTO a cada transição de estado.
Status possíveis: `pending` | `in-progress` | `done` | `blocked`

## Progresso geral

```
Total: 37 tarefas
Done:  31
Pending: 6
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
| T27 | `.tasks/27-playback-stats-collection.md` | Coleta de eventos de estatísticas nos handlers de serving/upload | done | depende T26, T07, T09, T12 — issue #2 |
| T28 | `.tasks/28-stats-aggregation-route.md` | Rota administrativa de estatísticas agregadas (`/admin/stats`) | done | depende T26, T27, T18 — issue #2 — fecha a issue #2 |
| T29 | `.tasks/29-opentelemetry-metrics-route.md` | Rota de métricas no padrão OpenTelemetry/Prometheus (`/metrics`) | done | depende T20, T26 — fecha issue #1 |
| T30 | `.tasks/30-swagger-docs.md` | Documentação da API via Swagger/OpenAPI | done | depende T20, T13, T18, T28, T29 — issue #3 — fecha a issue #3 |
| T31 | `.tasks/31-env-vars-seconds.md` | Padronizar variáveis de tempo das envs em segundos | done | sem dependências — issue #4 — fecha a issue #4 |
| T32 | `.tasks/32-project-model.md` | Model de Projeto (slug, diretório raiz, chave mestra) | pending | depende T03, T31 — issue #6 |
| T33 | `.tasks/33-scoped-api-keys.md` | Chaves de API escopadas por projeto (upload/listagem/admin) | pending | depende T32 — issue #6 |
| T34 | `.tasks/34-project-storage-layout.md` | Layout de armazenamento por projeto (diretórios isolados) | pending | depende T32, T33 — issue #6 |
| T35 | `.tasks/35-project-management-routes.md` | Rotas de gerenciamento de projetos | pending | depende T32, T33 — issue #6 — fecha a issue #6 |
| T36 | `.tasks/36-storage-stats-model.md` | Model de armazenamento por vídeo (bytes, duração, status) | pending | depende T03, T04 (recomendado após T34) — issue #5 |
| T37 | `.tasks/37-storage-stats-route.md` | Expor estatísticas de armazenamento e fila em `/admin/stats` | pending | depende T36, T28 — issue #5 — fecha a issue #5 |

## Próxima onda — ordem de prioridade sugerida (T31-T37)

A ordem abaixo respeita as dependências técnicas reais entre as tarefas
(uma micro-tarefa só aparece depois de tudo que ela precisa já estar pronto).
Onde não há dependência direta, a ordem reflete risco/esforço — tarefas
pequenas e independentes vêm primeiro para não bloquear o restante:

1. **T31** (issue #4) — pequena, mecânica, sem dependências. Resolve antes
   de mexer de novo em `config.go` nas tarefas maiores (T32+).
2. **T32** (issue #6, fundação) — model de Projeto; tudo do "projetos" parte
   daqui.
3. **T33** (issue #6) — chaves escopadas por projeto; depende do model T32.
4. **T34** (issue #6) — layout de armazenamento por projeto; só faz sentido
   com chaves escopadas (T33) já resolvendo a qual projeto um upload pertence.
5. **T35** (issue #6, fecha a issue) — rotas HTTP de gerenciamento de
   projetos; expõe o que foi construído em T32/T33.
6. **T36** (issue #5) — model de estatísticas de armazenamento; tecnicamente
   só depende de T03/T04, mas fazer **depois de T34** evita recalcular paths
   de armazenamento duas vezes (uma vez no layout antigo, outra no novo).
7. **T37** (issue #5, fecha a issue) — expõe as agregações de T36 em
   `/admin/stats`, reaproveitando a rota do T28.

Resumo por issue:
- **#4** → T31 (pequena, isolada)
- **#6** → T32 → T33 → T34 → T35 (cadeia longa; é a maior mudança arquitetural)
- **#5** → T36 → T37 (independente de #6, mas posicionada após T34 por
  conveniência técnica — pode ser adiantada se a equipe preferir não esperar
  a cadeia de projetos)

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
[2026-06-07 04:15] T27: pending → in-progress
[2026-06-07 04:35] T27: in-progress → done
[2026-06-07 04:45] T28: pending → in-progress
[2026-06-07 05:10] T28: in-progress → done (fecha issue #2: T26+T27+T28 completam o ciclo de estatísticas de uso)
[2026-06-07 05:20] T29: pending → in-progress
[2026-06-07 05:50] T29: in-progress → done (fecha issue #1: rota /metrics no padrão OpenTelemetry/Prometheus)
[2026-06-07 06:50] T30: pending → in-progress
[2026-06-07 07:05] T30: in-progress → done (fecha issue #3: documentação interativa Swagger/OpenAPI em /docs/)
[2026-06-07 07:20] T31-T37 criadas a partir das issues #4, #5 e #6 (próxima onda: padronização de envs em segundos, sistema de projetos internos com chaves escopadas, e estatísticas de armazenamento) — ordem de prioridade documentada na seção "Próxima onda"
[2026-06-07 07:35] T31: pending → in-progress
[2026-06-07 07:50] T31: in-progress → done (fecha issue #4: variáveis de tempo padronizadas em segundos com sufixo _SECONDS)
