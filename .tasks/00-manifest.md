# Manifest de Tarefas — Streamedia

Atualizado pelo agente CTO a cada transição de estado.
Status possíveis: `pending` | `in-progress` | `done` | `blocked`

## Progresso geral

```
Total: 90 tarefas
Done:  90
Pending: 0
```

## ⚠️ ARQUITETURA ATUAL (fluxo vigente) — leia antes de mexer

O modelo de **"projetos" com chave mestra foi REMOVIDO** (T69). Várias tarefas
antigas (T06, T32–T35, T44, T48–T50) descrevem um fluxo que **não existe mais** —
elas ficam no histórico apenas como registro; **não** reflitam o código atual.

Fluxo atual, enxuto:

- **`ROOT_TOKEN`** (env) — credencial única de gestão, em
  `Authorization: Bearer`. Protege `/api/upload/init`, `/api/play/init`,
  `/api/status` e `/admin/*`. Substitui `ADMIN_TOKEN` e `UPLOAD_TOKEN_SECRET`
  (ambos eliminados).
- **`tag`** — namespace organizacional do vídeo (coluna `videos.tag`, indexada,
  default `default`). Sem credencial própria. Disco: `<MEDIA_DIR>/<tag>/<id>`.
  Substitui a tabela `projects` (removida).
- **`access_tokens`** — tokens efêmeros de upload/play (linha no banco, coluna
  `purpose`, validados por lookup, sem HMAC). Substitui `upload_tokens` e os
  play tokens HMAC.
- **Play** — `POST /api/play/init` emite `/video/<tag>/<id>.m3u8?token=...`
  (master dinâmico; segmentos estáticos). Não há mais `/videos/...`.
- **Webhook** — mantém HMAC (`WEBHOOK_SECRET`), único segredo compartilhado.
- Variáveis de tempo **sem** sufixo `_SECONDS` (`UPLOAD_TOKEN_TTL`,
  `PLAY_TOKEN_TTL`, `UPLOAD_IDLE_TIMEOUT`, `TRANSCODE_STUCK`).

Spec dividida em `spec/` (índice + arquivos temáticos) — ver T70.

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
| T32 | `.tasks/32-project-model.md` | Model de Projeto (slug, diretório raiz, chave mestra) | done | depende T03, T31 — issue #6 |
| T33 | `.tasks/33-scoped-api-keys.md` | Chaves de API escopadas por projeto (upload/listagem/admin) | done | depende T32 — issue #6 |
| T34 | `.tasks/34-project-storage-layout.md` | Layout de armazenamento por projeto (diretórios isolados) | done | depende T32, T33 — issue #6 |
| T35 | `.tasks/35-project-management-routes.md` | Rotas de gerenciamento de projetos | done | depende T32, T33 — issue #6 — fecha a issue #6 |
| T36 | `.tasks/36-storage-stats-model.md` | Model de armazenamento por vídeo (bytes, duração, status) | done | depende T03, T04 (recomendado após T34) — issue #5 |
| T37 | `.tasks/37-storage-stats-route.md` | Expor estatísticas de armazenamento e fila em `/admin/stats` | done | depende T36, T28 — issue #5 — fecha a issue #5 |
| T38 | `.tasks/38-coverage-data-layer.md` | Cobertura de testes — camada de dados (models + db) | done | origem: issue #7 — cobertura models 56.6%→80.8%, db 57.1%→58.0%, 27 testes novos, nenhum bug real encontrado |
| T39 | `.tasks/39-coverage-jobs-transcode.md` | Cobertura de testes — jobs de manutenção e transcodificação | done | origem: issue #7 — cobertura jobs 56.3%→78.6%, transcode 72.5%→82.8%; corrigido bug de rollback em requeue.go e adicionada abstração FFprobeExecutor |
| T40 | `.tasks/40-coverage-upload-auth-config.md` | Cobertura de testes — upload, autenticação e configuração | done | origem: issue #7 — cobertura upload 69.0%→72.0%, auth 74.4%→93.0%, config 74.5%→82.8%; nenhum bug real confirmado — fecha a issue #7 (T38→T39→T40) |
| T41 | `.tasks/41-security-auth-tokens.md` | Auditoria de segurança — autenticação, autorização e tokens | done | origem: issue #8 |
| T42 | `.tasks/42-security-upload-processing.md` | Auditoria de segurança — upload, validação e execução de processos (FFmpeg) | done | origem: issue #8; depende logicamente de T41 (não bloqueante) |
| T43 | `.tasks/43-security-network-infra.md` | Auditoria de segurança — rede, rate limiting, webhooks e configuração | done | origem: issue #8; fecha o sumário executivo de T41+T42+T43 — fecha a issue #8 |
| T44 | `.tasks/44-optional-video-id-uuidv7.md` | video_id opcional em /upload/init — gera UUID v7 quando ausente, aceita qualquer versão quando informado | done | origem: solicitação direta (não vinculada a issue); depende T08, T35 |
| T45 | `.tasks/45-standard-response-envelope.md` | Pacote central de resposta padronizada `{error, message, data, status_code}` | done | issue #9 — fundação criada (pacote apiresponse, middleware recovery, spec atualizada) |
| T46 | `.tasks/46-migrate-routes-standard-response.md` | Migrar todas as rotas para o envelope padrão + testes de conformidade | done | issue #9 — todas as rotas migradas, respondError/respondJSONError/http.Error removidos, testes adaptados, conformance suite criada — fecha issue #9 |
| T47 | `.tasks/47-centralize-hls-regex-and-url-builder.md` | Centralizar regex de segmento HLS e construção de URL pública (scheme/host) | done | SegmentNameRe em models/hls.go; PublicUploadURL em httputil/url.go; duplicações removidas de serve.go, worker.go, init.go, projects.go |
| T48 | `.tasks/48-default-project-always-assigned.md` | Todo upload sempre pertence a um projeto — projeto padrão automático | done | issue #10 — EnsureDefaultProject em models/project.go, init.go sempre resolve projeto, ResolveVideoRootDir retorna erro p/ nil, project_migration.go removido |
| T49 | `.tasks/49-remove-legacy-upload-auth-flow.md` | Remover fluxo de autenticação legado (HMAC global) de /upload/init | done | issue #10 — X-Project-Key obrigatório, branch HMAC/X-Upload-Auth removido, token usa chave do projeto, UploadTokenSecret preservado p/ serve/status |
| T50 | `.tasks/50-unify-upload-token-ttl.md` | Unificar UPLOAD_TOKEN_TTL_SECONDS e UPLOAD_TOKEN_SCOPED_TTL_SECONDS em uma única variável | done | issue #10 — UploadTokenScopedTTL removido, UploadTokenTTL com default 1200s (20min), fecha issue #10 (cadeia T48→T49→T50) |
| T51 | `.tasks/51-docs-ui-scalar.md` | Trocar UI de documentação da API de Swagger para Scalar | done | origem: issue #12 (continuação da issue #3/T30); troca só a UI, spec OpenAPI inalterada |
| T52 | `.tasks/52-db-migrations.md` | Migrations versionadas (goose) substituindo schema.go monolítico | done | issue #13 — pressly/goose/v3 como bib embutida, 0001_init.sql com schema completo via go:embed, schema.go removido, ensureColumn removido, project_id nasce no CREATE TABLE — fecha issue #13 |
| T53 | `.tasks/53-fix-listbystatus-project-id.md` | Corrigir ListByStatus — omissão de project_id na query SELECT | done | depende T04, T33 — origem: análise de código — bug funcional |
| T54 | `.tasks/54-fix-queue-enqueue-silent-db-error.md` | Corrigir Queue.Enqueue — ignora erro de banco silenciosamente | done | depende T10 — origem: análise de código — bug de consistência |
| T55 | `.tasks/55-api-version-route.md` | Rota GET /api — nome, status e versão da API com rate limiting baixo | done | pacote internal/version + rota GET /api com rate limiter 10 req/min |
| T56 | `.tasks/56-fix-nil-deref-webhook-send.md` | Fix: nil pointer dereference em `sendWebhook` quando `GetVideo` falha | done | **critica** — crash em produção; server.go:43 |
| T57 | `.tasks/57-fix-tus-handler-error-ignored.md` | Fix: erro de `NewTUSHandler` ignorado — servidor sobe com handler nil | done | **critica** — crash em produção; server.go:57 |
| T58 | `.tasks/58-fix-enqueue-bypass-state-machine.md` | Fix: `Enqueue` bypassa máquina de estados com UPDATE direto | done | **critica** — corrupção de estado; queue.go:92 |
| T59 | `.tasks/59-fix-tus-consumehooks-no-shutdown.md` | Fix: goroutine `consumeHooks` sem graceful shutdown | done | **critica** — goroutine leak; tus.go:114 |
| T60 | `.tasks/60-fix-ratelimiter-memory-leak.md` | Fix: `sync.Map` do rate limiter sem eviction — memory leak | done | **alta** — eviction periódica com limiterEntry.lastSeen |
| T61 | `.tasks/61-fix-adminauth-silent-db-error.md` | Fix: `AdminAuth` silencia erro de banco como 401 | done | **alta** — distingue sql.ErrNoRows de erro real |
| T62 | `.tasks/62-fix-decompose-replacer-allocation.md` | Fix: `strings.NewReplacer` recriado a cada chamada de `decompose` | done | **alta** — movido para var de pacote |
| T63 | `.tasks/63-fix-timestamp-format-inconsistency.md` | Fix: formato de timestamp inconsistente entre `Enqueue` e demais escritas | done | **alta** — resolvida junto com T58 |
| T64 | `.tasks/64-refactor-video-scan-duplication.md` | Refactor: extrair `scanVideoRow` — código de scan duplicado 3x | done | **media** — ScanVideoRow exportada, ~80 linhas de duplicação removidas |
| T65 | `.tasks/65-fix-handle-transcode-failure-silent-errors.md` | Fix: `handleTranscodeFailure` ignora erros de banco silenciosamente | done | **media** — logging adicionado, best-effort mantido |
| T66 | `.tasks/66-fix-recovery-hardcoded-status-strings.md` | Fix: strings literais de status em `recovery.go` em vez de constantes | done | **media** — constantes models.Status* usadas |
| T67 | `.tasks/67-fix-jobs-stop-no-waitgroup.md` | Fix: jobs periódicos sem `WaitGroup` no `Stop` — race no shutdown | done | **media** — WaitGroup em todos os 3 jobs |
| T68 | `.tasks/68-fix-webhook-timeout-mismatch.md` | Fix: timeout incoerente no webhook client (30s client vs 10s context) | done | **media** — client timeout removido, só context 10s |
| T69 | `.tasks/69-cleanup-legacy-flow.md` | Limpeza total do fluxo legado (projects/HMAC/scoped) — código, vars, comentários, OpenAPI | done | substitui o fluxo de projetos pelo de tag + ROOT_TOKEN; **supersede T06, T32–T35, T44, T48–T50** |
| T70 | `.tasks/70-spec-reorg.md` | Reorganizar a especificação em arquivos menores + índice | done | `spec/` temática; depende T69 |
| T71 | `.tasks/71-test-console-pipeline.md` | Console de teste interativo do pipeline completo (`GET /playground`) | done | issue #18 — pacote internal/playground, receptor de webhooks local, players HLS por resolução — fecha issue #18 |
| T72 | `.tasks/72-thumbnails-por-resolucao.md` | Gerar thumbnails (poster) por resolução ao final da transcodificação | done | issue #19 — geração JPEG por resolução (frame a 1s, proporção preservada), rota pública `/video/<tag>/<id>/thumb_<res>.jpg`, `has_thumbnails`+`thumbnails` no status — fecha issue #19 |
| T73 | `.tasks/73-webhook-url-por-video.md` | URL de webhook customizada por vídeo no POST /api/upload/init | done | issue #20 — coluna `videos.webhook_url` (migration 0002), validação HTTPS/≤2048 no init, `webhook.Client.resolveURL` prioriza URL do vídeo sobre a global — fecha issue #20 |
| T74 | `.tasks/74-discord-alertas-operacionais.md` | Webhook do Discord para notificação de erros operacionais | done | issue #21 — pacote `internal/discord` (Alerter nil-safe), `DISCORD_WEBHOOK_URL`, alertas de failed_transcode/fila cheia/transcode travado/falhas consecutivas injetados via SetAlerter em worker/queue/requeue — fecha issue #21 |
| T75 | `.tasks/75-users-roles-config-migration.md` | Migration SQL: tabelas users, user_roles, configurations | done | — |
| T76 | `.tasks/76-user-role-config-models.md` | Modelos Go: User, UserRole, Configuration + queries | done | depende T75 |
| T77 | `.tasks/77-config-manager-dbconfig.md` | Pacote config/dbconfig — gerenciador de configs com fallback para defaults | done | depende T75 |
| T78 | `.tasks/78-google-oauth-session.md` | Google OAuth2 — fluxo login/callback + session cookie com user_id e roles | done | depende T76 |
| T79 | `.tasks/79-role-auth-middleware.md` | Middleware RoleAuth — autorização por roles explícitas em cada endpoint | done | depende T76 |
| T80 | `.tasks/80-admin-users-crud-reprocess.md` | CRUD /admin/users com regra de nível + POST /api/videos/{id}/reprocess | done | depende T76, T79 |
| T81 | `.tasks/81-admin-config-api.md` | Config API: GET/PUT/DELETE /admin/config com validação e agrupamento | done | depende T77, T79 |
| T82 | `.tasks/82-server-spa-wire.md` | Wire completo server.go: SPA /app, auth Google, RoleAuth, remoção legado | done | depende T78, T79, T80, T81 |
| T83 | `.tasks/83-vue-vite-scaffold.md` | Scaffold web/: Vite + Vue 3 + TS + Tailwind + shadcn-vue + phosphor-icons | done | — |
| T84 | `.tasks/84-router-stores-guards-menu.md` | Router meta, Pinia stores, navigation guards, menu composable, api client | done | depende T83 |
| T85 | `.tasks/85-views-login-overview-videos-playground.md` | Views: Login, Overview, Videos, Video, Playground | done | depende T84 |
| T86 | `.tasks/86-views-users-config-components.md` | Views: Users, Config + RolesSelect + ConfigEditor | done | depende T84 |
| T87 | `.tasks/87-docker-multistage-coolify.md` | Dockerfile multi-stage + docker-compose Coolify final | done | depende T82, T85 |
| T88 | `.tasks/88-testes-backend-frontend.md` | Testes Go (auth, roles, users, config) + Vitest (stores, guards, menu) | done | depende T82, T86 |
| T89 | `.tasks/89-remocao-legado.md` | Remoção de legacy: dashboard, playground, docs, POST /admin/session | done | depende T82 |
| T90 | `.tasks/90-atualizar-spec-versioner.md` | Atualizar spec/ + .agents/versioner.md (sync package.json) | done | depende T89 |

## Ondas concluídas

### Admin Unificado Vue.js + Google OAuth (T75-T90)

Ordem de execução: T75 → T76 → T77 (paralelo) | T78 → T79 → T80+T81 (paralelo) → T82 → T83 → T84 → T85+T86 (paralelo) → T87 → T88 → T89 → T90.

> **Correções T56-T68 (onda anterior, já concluída)**

Ordem de prioridade por severidade e dependências:

### Críticas (crash em produção / corrupção de estado)
1. **T56** — nil deref no webhook (crash imediato, fix trivial)
2. **T57** — TUS handler nil (crash em qualquer upload, fix trivial)
3. **T58** — Enqueue bypassa state machine (corrupção de estado; T63
   pode ser resolvida junto se T58 usar `models.UpdateStatus`)
4. **T59** — consumeHooks sem shutdown (goroutine leak)

### Altas (degradação progressiva / erros invisíveis)
5. **T61** — AdminAuth silencia erro de banco (fix trivial, alto impacto diagnóstico)
6. **T62** — Replacer alocado a cada chamada (fix de uma linha)
7. **T63** — Timestamp inconsistente (pode ser resolvida junto com T58)
8. **T60** — Rate limiter memory leak (mais complexa, exige design de eviction)

### Médias (qualidade / manutenção / robustez)
9. **T65** — handleTranscodeFailure silent errors (adicionar logs)
10. **T66** — Recovery hardcoded strings (substituir por constantes)
11. **T67** — Jobs sem WaitGroup (adicionar sync no shutdown)
12. **T68** — Webhook timeout mismatch (alinhar timeouts)
13. **T64** — Scan duplicado 3x (refatoração maior, mas previne bugs futuros)

Notas:
- T58 + T63 podem ser resolvidas numa única task (mesmo arquivo, mesmo SQL)
- T56, T57, T61, T62 são fixes triviais (1-5 linhas cada) e podem ser
  feitas todas em sequência rápida
- T60 é a mais complexa — exige design de eviction e goroutine de cleanup

## Onda anterior — ordem de prioridade (T31-T37)

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

[2026-06-07] T56: pending → done (sendWebhook trata erros de GetVideo e wc.Send — evita nil deref)
[2026-06-07] T57: pending → done (NewRouter retorna (Handler, io.Closer, error) — TUS handler nil impossível)
[2026-06-07] T58: pending → done (Enqueue usa models.UpdateStatus — valida transições via state machine)
[2026-06-07] T59: pending → done (consumeHooks com stopCh + WaitGroup + ok check — graceful shutdown)
[2026-06-07] T63: pending → done (resolvida junto com T58 — strftime manual eliminado)
[2026-06-08] T62: pending → done (accentReplacer movido para var de pacote)
[2026-06-08] T65: pending → done (logging nos erros best-effort de handleTranscodeFailure)
[2026-06-08] T66: pending → done (strings hardcoded substituídas por models.Status* em recovery.go)
[2026-06-08] T67: pending → done (WaitGroup adicionado a TokenCleanupJob, UploadKillerJob, TranscodeRequeueJob)
[2026-06-08] T68: pending → done (Timeout: 30s removido do http.Client — só context 10s)
[2026-06-08] T61+T64: pending → done (AdminAuth distingue sql.ErrNoRows; ScanVideoRow+SelectVideoColumns extraídas)
[2026-06-08] T60: pending → done (limiterEntry com lastSeen, evictLoop periódico, Stop() no shutdown)
[2026-06-10] REFACTOR ARQUITETURAL: modelo de projetos/chave-mestra substituído por tag + ROOT_TOKEN único (commit feat!). access_tokens com purpose; /api/play/init; serving /video/<tag>/<id>.m3u8; status via Bearer; vars de tempo sem _SECONDS. Supersede T06/T32–T35/T44/T48–T50.
[2026-06-10] T69: in-progress → done (remoção total do fluxo legado: OpenAPI reescrito, telemetria /video/, comentários e testes limpos, ci_test usa ROOT_TOKEN; go vet limpo)
[2026-06-10] T70: in-progress → done (spec dividida em spec/ temática + índice; CLAUDE.md aponta para o novo índice)
[2026-06-10] T71: pending → done (console de teste interativo em GET /playground — pacote internal/playground: página autocontida via go:embed, cliente TUS em JS com progresso por chunk + unificado, polling de status, play/init, players HLS por resolução com medição de tempo até iniciar, e receptor de webhooks local /playground/webhook + /playground/webhook/events; rotas públicas; testes do pacote; spec/api.md e api.http atualizados — fecha issue #18)
[2026-06-08] T55: pending → done (já estava implementada — pacote internal/version e rota GET /api existentes)
[2026-06-10] T72: pending → done (thumbnails por resolução, issue #19: geração JPEG best-effort no worker (internal/transcode/thumbnail.go) a partir do frame a 1s do original com fallback ao primeiro frame, proporção preservada (menor dimensão = "p"), ~80% via mjpeg -q:v 5; rota pública GET /video/<tag>/<id>/thumb_<res>.jpg (serve/thumbnail.go) com blindagem de traversal; has_thumbnails+thumbnails derivados do disco no status; httputil.PublicThumbnailURL; ThumbnailNameRe/ThumbnailFileName em models/hls.go; testes em transcode/serve; spec/api.md e api.http atualizados — fecha issue #19. Worktree baseado em origin/main por dev estar 24 commits atrás (faltava a feature de SSE/notify/playground já lançada em main))
[2026-06-10] T72 (follow-up): playground usa o thumbnail como `poster` dos <video> por resolução (internal/playground/index.html) — antes, com preload="none", o player ficava um quadrado preto até o play; agora mostra a capa daquela resolução. Adicionado object-fit:contain p/ não distorcer portrait. Refs #19.

[2026-06-11] T73: pending → done (issue #20 — webhook_url por vídeo: migration 0002_video_webhook_url.sql adiciona coluna `videos.webhook_url TEXT NOT NULL DEFAULT ''`; initRequest aceita `webhook_url` com validação HTTPS/formato/≤2048 (informado-mas-inválido → 400, omitido → global); models.InsertVideoWithTagAndWebhook + WebhookURL em SelectVideoColumns/ScanVideoRow; webhook.Client.resolveURL prioriza a URL do vídeo sobre a WEBHOOK_URL global, com fallback; HMAC inalterado; testes de validação/persistência/override/fallback; docs spec/api.md, spec/webhooks.md, spec/dados.md, OpenAPI, api.http, .env.example, README — fecha issue #20)
[2026-06-11] T74: pending → done (issue #21 — alertas operacionais no Discord: pacote internal/discord com Alerter nil-safe (NewAlerter("")→nil, métodos no-op); DISCORD_WEBHOOK_URL opcional na config; gatilhos failed_transcode (worker+requeue), fila cheia (queue.Enqueue), transcode travado (requeue.runOnce) e falhas consecutivas (contador no Alerter, limiar 5, reset no sucesso); embed com video_id/status/error_message/timestamp; best-effort com log de sucesso/falha; injeção via SetAlerter em worker/queue/requeue + main; testes do pacote e gatilho de fila cheia ponta-a-ponta; docs spec/operacao.md, spec/webhooks.md, .env.example, README — fecha issue #21)

[2026-06-07] Análise completa de código: geradas T56-T68 (13 tasks) a partir de
  revisão profunda de todos os arquivos .go de produção (~20k linhas).
  Classificação: 4 críticas (T56-T59: crashes/corrupção), 4 altas
  (T60-T63: degradação/erros invisíveis), 5 médias (T64-T68: qualidade/robustez).
  Principais achados: nil deref no webhook (T56), TUS handler nil (T57),
  bypass da máquina de estados no Enqueue (T58), goroutine leak no TUS (T59),
  memory leak no rate limiter (T60), scan de Video duplicado 3x (T64).
  Status inicial de todas: pending.

[2026-06-07] CTO: geradas T53-T54 a partir de análise estática do código existente
  — bugs encontrados durante revisão geral:
  T53 (ListByStatus omite project_id na query — bug funcional),
  T54 (Queue.Enqueue ignora erro de banco — risco de estado inconsistente).
  Duas tasks adicionais (rate limiter memory leak e remoção de google/uuid)
  foram descartadas após revisão cruzada com as pendentes: conflitavam com
  T43 (auditoria de segurança do rate limiter) e T44 (que depende de
  google/uuid para geração de UUID v7). Estimativa pequena, sem issue
  vinculada. Status inicial: pending.

[2026-06-07 19:36] T53: pending → in-progress
[2026-06-07 19:36] T53: in-progress → done (adiciona project_id na query SELECT e Scan de ListByStatus — coluna e conversão estavam ausentes, vídeos sempre voltavam com ProjectID=nil)
[2026-06-07 19:37] T54: pending → in-progress
[2026-06-07 19:38] T54: in-progress → done (Enqueue captura erro do db.Exec em vez de ignorar com _, _ — evita estado inconsistente de vídeo na fila sem status transcoding no banco)
[2026-06-07 19:42] T41: pending → done (auditoria auth/tokens: F-01 corrigida — mensagens de erro de play token unificadas)
[2026-06-07 19:42] T42: pending → done (auditoria upload/FFmpeg: nenhuma falha encontrada)
[2026-06-07 19:42] T43: pending → done (auditoria rede/infra: F-02 corrigida — timeouts HTTP adicionados contra Slowloris; fecha issue #8)
[2026-06-07 19:59] T44: pending → done (video_id opcional em /upload/init, gera UUID v7, aceita qualquer versão, centralizado em models)

[2026-06-07 20:15] T45: pending → in-progress → done (pacote apiresponse criado: Envelope, Success, Error; RecoveryMiddleware em internal/middleware substituindo chimw.Recoverer no server.go; spec atualizada com nova seção 10 "Formato padrão de resposta da API" — Refs #9)
[2026-06-07 20:35] T46: pending → in-progress → done (migração completa: respondError/respondJSONError/http.Error removidos de serve, upload, admin; tus.go raw writes e tusd.HTTPResponse bodies migrados para envelope; healthz migrado; rate limiter migrado; todos os testes adaptados; response_conformance_test.go criado com 4 suítes — fecha issue #9)
[2026-06-07 20:45] T47: pending → in-progress → done (SegmentNameRe centralizada em models/hls.go, removida duplicação serve/worker; PublicUploadURL centralizada em httputil/url.go, removida duplicação init.go/projects.go; testes table-driven para ambos os contratos)
[2026-06-07 21:11] T48: pending → in-progress → done (EnsureDefaultProject em models/project.go com race handling; init.go resolve projeto padrão no fluxo HMAC legado; project_migration.go + teste + chamada main.go removidos; ResolveVideoRootDir rejeita nil; EnsureDefaultProject chamado no startup main.go — Refs #10)
[2026-06-07 21:18] T49: pending → in-progress → done (branch HMAC/X-Upload-Auth removido de init.go; X-Project-Key obrigatório; token usa chave do projeto; UploadTokenSecret/ValidateBackendAuth preservados em serve/status — Refs #10)
[2026-06-07 21:26] T50: pending → in-progress → done (UploadTokenScopedTTL removido da config; UploadTokenTTL default 1200s = 20min; referências atualizadas em init.go, projects.go, config_test.go, init_test.go, project_scope_test.go, projects_test.go — fecha issue #10)
[2026-06-07 21:44] T52: pending → in-progress → done (pressly/goose/v3 adicionado; migrations/migrations.go com go:embed; 0001_init.sql com schema completo; db.go troca schema.go+ensureColumn por goose.Up; schema.go e ensureColumn removidos; 26 testes passam — fecha issue #13)

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
[2026-06-07 08:00] T32: pending → in-progress
[2026-06-07 08:20] T32: in-progress → done (model de Projeto: slug, chave mestra com hash, CRUD básico — fundação da issue #6)
[2026-06-07 08:35] T33: pending → in-progress
[2026-06-07 09:10] T33: in-progress → done (chaves escopadas por projeto: X-Project-Key em /upload/init com TTL curto, leitura já escopada por video_id, admin com escopo por projeto via opção (a) — Refs #6)
[2026-06-07 09:30] T34: pending → in-progress
[2026-06-07 09:55] T34: in-progress → done (layout de armazenamento isolado por projeto: ResolveVideoRootDir unifica worker/serving, migração idempotente de vídeos legados para o projeto "Legacy" no startup — Refs #6)
[2026-06-07 10:10] T35: pending → in-progress
[2026-06-07 10:45] T35: in-progress → done (rotas de gerenciamento de projetos: CRUD via /admin/projects* protegido por super-admin, emissão de token de upload via X-Project-Key — fecha issue #6, encerrando a cadeia T32→T33→T34→T35)
[2026-06-07 11:00] T36: pending → in-progress
[2026-06-07 11:30] T36: in-progress → done (model de armazenamento: tabela video_renditions com UPSERT por (video_id, resolution), scanRenditionDir no worker FFmpeg, e funções de agregação TotalStorageBytes/TotalDurationSeconds/CountVideosByStatus/StorageByVideo em internal/models/storage.go — descoberta: actual_size_bytes/duration_s já existiam em videos — Refs #5)
[2026-06-07 11:40] T37: pending → in-progress
[2026-06-07 12:00] T37: in-progress → done (estende /admin/stats com a seção "storage" — total_bytes, total_duration_seconds, videos_by_status, queue_pending — reaproveitando as agregações de T36 e queue.Len(); seção omitida quando ?video_id= é informado, decisão documentada — fecha issue #5, encerrando a cadeia T36→T37)
[2026-06-07] CTO: geradas T38-T43 a partir das issues abertas #7 (cobertura
  de testes — divididas por área: T38 camada de dados, T39 jobs/transcode,
  T40 upload/auth/config) e #8 (auditoria de segurança — divididas por
  superfície: T41 autenticação/tokens, T42 upload/processamento/FFmpeg,
  T43 rede/infra, que também fecha o sumário executivo da auditoria).
  Numeração inicia em T38 (não T26) porque T26-T37 já existem nesta
  branch dev, concluídas a partir das issues #1-#6. Status inicial:
  pending. Aguardando início do workflow QA → Dev para cada uma.
[2026-06-07] CTO: gerada T44 a partir de solicitação direta do usuário
  (não vinculada a issue do GitHub): tornar video_id opcional em
  /upload/init (gera UUID v7 quando ausente), aceitar qualquer versão de
  UUID quando informado pelo cliente, e padronizar para que TODA geração
  de id pelo próprio sistema (incluindo /admin/projects/*/upload-token,
  T35) sempre privilegie UUID v7. Status inicial: pending.
[2026-06-07] CTO: geradas T45 e T46 a partir da issue #9 ("Padronização
  das respostas" — pedido feito também diretamente na sessão antes da
  issue ser aberta): padronizar TODAS as respostas JSON da
  API no envelope {error, message, data, status_code}, centralizado em um
  único pacote (mesmo princípio da T44 — eliminar reinvenções paralelas;
  hoje há 4+ implementações divergentes de respondError). T45 cria a
  fundação (pacote apiresponse, middleware de recovery de panics no
  formato padrão, documentação na spec). T46 migra todas as rotas
  existentes e cria a suíte de testes de conformidade que garante que
  nenhuma rota — nem exceções não tratadas — escapa do padrão. Status
  inicial: pending.
[2026-06-07] CTO: gerada T47 a partir de "pente fino" de duplicação
  solicitado pelo usuário (mesmo princípio da T44 — eliminar regex/lógica
  reimplementada em paralelo). Encontrados 2 casos concretos: (1) regex
  de nome de segmento HLS `^[0-9]+\.ts$` duplicada byte-a-byte em
  internal/serve/serve.go (segmentRe) e internal/transcode/worker.go
  (renditionSegmentRe — cujo comentário já reconhecia a duplicação sem
  eliminá-la); (2) bloco de 13 linhas idêntico de resolução de
  scheme/host via X-Forwarded-* para montar a URL pública de upload,
  duplicado em internal/upload/init.go e internal/admin/projects.go.
  Tarefa propõe centralizar ambos com testes de tabela documentando o
  contrato antes da migração. Status inicial: pending.
[2026-06-07 13:10] T38: pending → in-progress
[2026-06-07 13:18] T38: in-progress → done (cobertura de internal/models 56.6%→80.8% e internal/db 57.1%→58.0%; 27 testes novos table-driven, incluindo schema_test.go novo; nenhum bug real encontrado — Refs #7)
[2026-06-07 13:25] T39: pending → in-progress
[2026-06-07 13:55] T39: in-progress → done (cobertura jobs 56.3%→78.6%, transcode 72.5%→82.8%; corrige bug de estado inconsistente em requeue.go (rollback de status quando enqueue falha) e adiciona abstração FFprobeExecutor para testabilidade — Refs #7)
[2026-06-07 14:05] T40: pending → in-progress
[2026-06-07 14:35] T40: in-progress → done (cobertura upload 69.0%→72.0%, auth 74.4%→93.0%, config 74.5%→82.8%; suspeita de bug "UUID all-zeros" investigada e descartada — formato é RFC4122-compliant; superfícies de segurança HMAC/validação confirmadas seguras; fecha issue #7 — cadeia T38→T39→T40)
[2026-06-07] CTO: geradas T48, T49 e T50 a partir da issue #10 ("Pq
  UPLOAD_TOKEN_SCOPED_TTL_SECONDS e UPLOAD_TOKEN_TTL_SECONDS são
  diferentes?"). Cadeia em 3 micro-tarefas dependentes: T48 garante que
  todo upload sempre tenha um projeto associado (cria projeto "default"
  automático, elimina project_id=NULL, remove o job MigrateLegacyVideos
  que vira código morto); T49 remove o branch de autenticação HMAC
  legado (X-Upload-Auth/UPLOAD_TOKEN_SECRET) de /upload/init — com nota
  explícita de que UploadTokenSecret/ValidateBackendAuth/ValidatePlayToken
  permanecem intocados por serem usados também em serve.go/status.go,
  fora do escopo de upload; T50 unifica as duas variáveis de TTL em uma
  só (UPLOAD_TOKEN_TTL_SECONDS, valor padrão de vida curta ~20min) e
  fecha a issue #10. Por pedido explícito do usuário: nome final da
  variável não deve conter "scoped"; sem necessidade de retrocompatibilidade
  (projeto ainda não está em uso — "quero ele limpo e sem vestígios de
  coisa velha antes de lançar"). Status inicial de todas: pending.

[2026-06-07 14:00] T51: criada e concluída — troca da UI de documentação de
  Swagger para Scalar (issue #12, continuação da issue #3/T30 — o autor
  achou o Swagger feio e pediu alternativas). pending → in-progress → done.
  Spec OpenAPI inalterada; só `internal/docs/docs.go` (página HTML) e
  `docs_test.go` foram ajustados. Refs #12.
[2026-06-07] CTO: corrigida colisão de numeração — uma onda paralela
  também registrou uma tarefa como "T51" (a de cima, troca de UI para
  Scalar/issue #12, mesclada via PR #14). A tarefa de migrations
  (issue #13) foi renomeada de `51-db-migrations.md` para
  `52-db-migrations.md` e renumerada para T52 — restaurando aqui o
  registro original que se perdeu na resolução do merge:
  T52 — gerada a partir da issue #13 ("Como a lib trata as migrações
  de banco de dados?"). O usuário apontou que o schema hoje é uma
  string DDL única (internal/db/schema.go) reaplicada via
  CREATE TABLE IF NOT EXISTS a cada boot — modelo que não suporta
  alterações estruturais reais (rename/drop/alter de coluna) e não
  versiona o histórico de mudanças, e citou o PocketBase como
  inspiração (gera migrations comparando structs com o schema).
  Estudo de alternativas no ecossistema Go (golang-migrate, goose,
  atlas, GORM AutoMigrate, ent, sqlc) concluiu que o caminho de
  struct→diff→migration automática (PocketBase/Ent/Atlas) é
  desproporcional ao tamanho do projeto (3 tabelas, SQLite, filosofia
  de SQL puro já documentada na spec). Recomendação registrada na
  issue: adotar pressly/goose como biblioteca embutida — migrations
  SQL versionadas em internal/db/migrations/, embutidas via go:embed,
  executadas automaticamente em db.Open() a cada inicialização do
  servidor (idempotente via tabela goose_db_version), substituindo
  schema.go. T52 fecha a issue #13. Status inicial: pending.
