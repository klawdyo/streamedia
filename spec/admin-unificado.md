# Admin Unificado — Especificação Técnica

Substitui `/playground`, `/dashboard`, `/docs` e `POST /admin/session` legados
por uma SPA Vue 3 + Vite + shadcn-vue com autenticação Google OAuth, servida
em `/app` pelo mesmo binário Go que serve a API REST em `/api`.

---

## 1. Arquitetura

```
Navegador ──▶ streamedia (Go, porta $PORT)
              ├── /              → GET /api (versão, ambiente)
              ├── /healthz       → health check
              ├── /api/*         → API REST JSON
              ├── /api/auth/*    → Google OAuth flow
              ├── /app/*         → SPA Vue (web/dist/, servidor estático)
              └── /app/auth      → rota SPA: tela de login
```

Serviço único Go. Build do Vue em Dockerfile multi-stage. Em dev, Vite proxy → Go.

---

## 2. Database — Novas tabelas (Migration 0004)

### 2.1 `users`

| Coluna | Tipo | Descrição |
|--------|------|-----------|
| id | INTEGER PK | Autoincremento |
| email | TEXT UNIQUE NOT NULL | Email Google verificado |
| name | TEXT DEFAULT '' | Nome vindo do Google |
| picture | TEXT DEFAULT '' | URL do avatar Google |
| created_at | DATETIME DEFAULT CURRENT_TIMESTAMP | |

### 2.2 `user_roles`

| Coluna | Tipo | Descrição |
|--------|------|-----------|
| user_id | INTEGER FK users(id) | |
| role | TEXT NOT NULL | 'dev' \| 'admin' \| 'acl' \| 'manager' |
| level_num | INTEGER NOT NULL | 1..4 (menor = mais poder) |
| granted_by | INTEGER FK users(id) NULL | Quem concedeu |
| granted_at | DATETIME DEFAULT CURRENT_TIMESTAMP | |
| PK | (user_id, role) | |

### 2.3 `configurations`

| Coluna | Tipo | Descrição |
|--------|------|-----------|
| key | TEXT PK | Ex: 'transcode.workers' |
| value | TEXT NOT NULL | Sempre string; type define parse |
| type | TEXT DEFAULT 'string' | 'string' \| 'number' \| 'boolean' \| 'duration_seconds' \| 'url' \| 'secret' |
| description | TEXT DEFAULT '' | Exibido na UI |
| group_key | TEXT DEFAULT '' | Agrupamento: 'upload', 'transcode', 'token', 'rate_limit', 'webhook', 'discord', 'paths', 'session' |
| validation | TEXT DEFAULT '' | Regex de validação (vazio = sem validação) |
| visible | INTEGER DEFAULT 1 | 0 = secreto (nunca retornado no GET), só aceita PUT |
| updated_at | DATETIME DEFAULT CURRENT_TIMESTAMP | |

---

## 3. Sistema de roles

### 3.1 Níveis (quanto menor o number, maior o poder)

| Role | level_num | Permissões |
|------|-----------|------------|
| dev | 1 | Tudo, inclusive deletar configs do DB |
| admin | 2 | Tudo exceto deletar configs; gerencia usuários nível >= 2 |
| acl | 3 | CRUD de usuários (nível >= 3) + upload + dados + reprocess |
| manager | 4 | Upload, ver dados, reprocessar vídeos |

### 3.2 Nível efetivo

`effective_level = MIN(level_num)` entre todas as roles do usuário.

### 3.3 Regra de escalonamento (ACL)

Ao conceder/alterar roles de outro usuário:

```
effective_level(grantee) > target_role_level_num  →  403 Forbidden
```

### 3.4 Primeiro login (bootstrapping)

Se `users` está vazio, primeiro login Google OAuth é aceito automaticamente com role `dev`. Depois, só emails existentes em `users` podem logar.

---

## 4. ENV vs DB

### 4.1 Variáveis de ambiente (obrigatórias no boot)

| Variável | Default | Motivo |
|----------|---------|--------|
| `PORT` | 3000 | Bind do servidor |
| `SQLITE_PATH` | /data/media.db | Abrir banco |
| `ENV` | development | `development` / `production` |
| `ROOT_TOKEN` | *(obrigatório)* | Backend-to-backend; scraper Prometheus |
| `GOOGLE_CLIENT_ID` | — | OAuth |
| `GOOGLE_CLIENT_SECRET` | — | OAuth |
| `GOOGLE_REDIRECT_URL` | — | OAuth callback |
| `SPA_DIR` | ./web/dist | Path do build Vue |
| `SESSION_COOKIE_SECURE` | true (se ENV=production) | Flag Secure do cookie |

### 4.2 Configs no banco (`configurations`) — com default no código

| Key | Tipo | Grupo | Default | Visible |
|-----|------|-------|---------|---------|
| `paths.media_dir` | string | paths | /media | 1 |
| `paths.upload_tmp_dir` | string | paths | /media/.uploads | 1 |
| `session.ttl_seconds` | duration_seconds | session | 43200 | 1 |
| `upload.max_size_mb` | number | upload | 10 | 1 |
| `upload.idle_timeout` | duration_seconds | upload | 600 | 1 |
| `transcode.workers` | number | transcode | 1 | 1 |
| `transcode.queue_max` | number | transcode | 50 | 1 |
| `transcode.stuck_timeout` | duration_seconds | transcode | 1800 | 1 |
| `transcode.max_attempts` | number | transcode | 3 | 1 |
| `transcode.keep_original` | boolean | transcode | false | 1 |
| `token.upload_ttl` | duration_seconds | token | 1200 | 1 |
| `token.play_ttl` | duration_seconds | token | 3600 | 1 |
| `rate_limit.per_minute` | number | rate_limit | 60 | 1 |
| `webhook.url` | url | webhook | "" | 1 |
| `webhook.secret` | secret | webhook | "" | 0 |
| `discord.webhook_url` | url | discord | "" | 1 |

### 4.3 Regra de fallback

Toda config do DB tem default no código. Se ausente, usa default — sem crash.

---

## 5. Backend — Endpoints e autorização

### 5.1 Google OAuth (`internal/auth/google/`)

| Método | Rota | Descrição |
|--------|------|-----------|
| GET | `/api/auth/google` | Redireciona para Google OAuth |
| GET | `/api/auth/google/callback` | Troca code → token, valida email, emite cookie, redireciona `/app` |
| GET | `/api/auth/me` | `{ email, name, picture, roles[], effective_level }` |
| DELETE | `/api/auth/session` | Apaga cookie (público) |

### 5.2 Session cookie

```
Formato: <exp_unix>.<user_id>.<roles_csv>.<hmac_hex>
```

HMAC-SHA256 assinado com `ROOT_TOKEN`. Stateless — sem query no DB por request.

### 5.3 Middleware `RoleAuth`

```go
r.Group(func(r chi.Router) {
    r.Use(admin.RootAuth(cfg.RootToken))
    r.Use(admin.RoleAuth(database, "admin", "acl", "manager"))
    r.Get("/admin/videos", adminHandler.HandleVideos)
})
```

### 5.4 Permissões por endpoint

| Método | Rota | Roles |
|--------|------|-------|
| GET | `/api/auth/me` | dev, admin, acl, manager |
| DELETE | `/api/auth/session` | dev, admin, acl, manager |
| POST | `/api/upload/init` | dev, admin, acl, manager |
| GET | `/api/status/{id}` | dev, admin, acl, manager |
| POST | `/api/play/init` | dev, admin, acl, manager |
| POST | `/api/videos/{id}/reprocess` | dev, admin, acl, manager |
| GET | `/admin/videos` | dev, admin, acl, manager |
| GET | `/admin/queue` | dev, admin, acl, manager |
| GET | `/admin/stats` | dev, admin, acl, manager |
| DELETE | `/admin/videos/{id}` | dev, admin, acl, manager |
| GET | `/admin/users` | dev, admin, acl |
| POST | `/admin/users` | dev, admin, acl |
| PUT | `/admin/users/{id}/roles` | dev, admin, acl |
| DELETE | `/admin/users/{id}` | dev, admin |
| GET | `/admin/config` | dev, admin |
| PUT | `/admin/config/{key}` | dev, admin |
| DELETE | `/admin/config/{key}` | dev |
| GET | `/metrics` | ROOT_TOKEN apenas |

### 5.5 Config API — formato de resposta

`GET /admin/config` retorna:

```json
{
  "groups": [
    {
      "key": "transcode",
      "title": "Transcodificação",
      "description": "Configurações do pipeline de transcodificação",
      "items": [
        {
          "key": "transcode.workers",
          "value": "2",
          "type": "number",
          "description": "Número de workers paralelos de transcodificação. Cada worker consome uma goroutine e um processo FFmpeg. Aumentar melhora throughput mas consome mais CPU/memória.",
          "validation": "^[1-9]\\d*$",
          "visible": true,
          "default": "1"
        }
      ]
    }
  ]
}
```

Campos `visible: false` (tipo `secret`) nunca são retornados — só aceitam `PUT`.

---

## 6. Frontend — Estrutura (`web/`)

### 6.1 Stack

- Vue 3 (Composition API, `<script setup lang="ts">`)
- Vite
- TypeScript estrito
- Vue Router 4
- Pinia (setup syntax)
- shadcn-vue (instalado via CLI)
- Tailwind CSS
- phosphor-icons (via @phosphor-icons/vue)
- hls.js, chart.js + vue-chartjs

### 6.2 Feature-based layout

```
web/
├── src/
│   ├── main.ts
│   ├── App.vue
│   ├── styles/
│   │   └── global.css
│   ├── types/
│   │   └── index.ts              # User, Video, Role, ConfigGroup, etc.
│   ├── router/
│   │   └── index.ts              # Rotas + RouteMeta estendido
│   ├── api/
│   │   └── client.ts             # Fetch wrapper (CSRF, error handling)
│   ├── composables/
│   │   ├── useMenu.ts            # Gera menu a partir do router + permissões
│   │   ├── useNavigationGuard.ts # beforeEach hook
│   │   ├── useSSE.ts             # EventSource /api/events
│   │   └── useTheme.ts           # Dark/light
│   ├── stores/
│   │   └── auth.ts               # useAuthStore (login, logout, me, canAccess, resetAll)
│   ├── components/               # Componentes compartilhados entre features
│   │   ├── layout/
│   │   │   ├── AppLayout.vue
│   │   │   ├── AppSidebar.vue
│   │   │   ├── AppHeader.vue
│   │   │   └── ThemeToggle.vue
│   │   ├── player/
│   │   │   └── VideoPlayer.vue   # hls.js, comum a Video + Playground
│   │   └── ui/                   # shadcn-vue (instalado CLI)
│   └── features/                 # Uma pasta por domínio
│       ├── auth/
│       │   ├── views/
│       │   │   └── LoginView.vue
│       │   └── stores/
│       │       └── auth.ts       # (o useAuthStore é central)
│       ├── dashboard/
│       │   ├── views/
│       │   │   └── OverviewView.vue
│       │   ├── components/
│       │   │   ├── StatsGrid.vue
│       │   │   ├── StatsCard.vue
│       │   │   └── QueueWidget.vue
│       │   └── stores/
│       │       ├── stats.ts
│       │       └── queue.ts
│       ├── videos/
│       │   ├── views/
│       │   │   ├── VideosView.vue
│       │   │   └── VideoView.vue
│       │   ├── components/
│       │   │   └── VideoTable.vue
│       │   └── stores/
│       │       ├── videos.ts     # Lista, filtros, delete, reprocess
│       │       └── video.ts      # Detalhe, play init, SSE por vídeo
│       ├── playground/
│       │   ├── views/
│       │   │   └── PlaygroundView.vue
│       │   ├── components/
│       │   │   ├── UploadForm.vue
│       │   │   ├── UploadProgress.vue
│       │   │   ├── SSELog.vue
│       │   │   └── PlaybackPanel.vue
│       │   ├── composables/
│       │   │   └── useApiDocs.ts  # Documentação de todos os endpoints
│       │   └── stores/
│       │       └── upload.ts
│       ├── users/
│       │   ├── views/
│       │   │   └── UsersView.vue
│       │   ├── components/
│       │   │   ├── UsersTable.vue
│       │   │   └── RolesSelect.vue
│       │   └── stores/
│       │       └── users.ts
│       └── config/
│           ├── views/
│           │   └── ConfigView.vue
│           ├── components/
│           │   └── ConfigEditor.vue
│           └── stores/
│               └── config.ts
```

### 6.3 Router — RouteMeta estendido

```ts
declare module 'vue-router' {
  interface RouteMeta {
    title: string
    permissions: string[]            // roles que podem acessar
    showInMenu: boolean
    icon: string                     // nome do ícone phosphor (ex: 'ph-film')
    iconUnselected?: string
    parent?: string                  // nome da rota pai p/ agrupamento
    order?: number
  }
}
```

### 6.4 Rotas

| Path | View | permissions | showInMenu | parent | icon |
|------|------|-------------|------------|--------|------|
| `/app/auth` | LoginView | [] | false | — | — |
| `/app/overview` | OverviewView | dev,admin,acl,manager | true | — | ph-gauge |
| `/app/videos` | VideosView | dev,admin,acl,manager | true | videos-group | ph-film-reel |
| `/app/videos/:id` | VideoView | dev,admin,acl,manager | false | — | — |
| `/app/playground` | PlaygroundView | dev,admin,acl,manager | true | videos-group | ph-flask |
| `/app/users` | UsersView | dev,admin,acl | true | — | ph-users |
| `/app/config` | ConfigView | dev,admin | true | — | ph-gear |

### 6.5 Menu gerado

```
📊 Dashboard
📹 Vídeos
   📋 Biblioteca
   🧪 Playground
👥 Usuários       (acl+)
⚙️ Configurações  (admin+)
```

### 6.6 Navigation guard

```ts
router.beforeEach(async (to, from, next) => {
  const auth = useAuthStore()
  if (!auth.checked) await auth.fetchMe()
  
  // Rota pública
  if (!to.meta.permissions?.length || to.name === 'login') {
    if (auth.isLoggedIn && to.name === 'login') return next({ name: 'overview' })
    return next()
  }
  
  // Não logado
  if (!auth.isLoggedIn) return next({ name: 'login', query: { redirect: to.fullPath } })
  
  // Sem permissão
  if (!auth.canAccess(to.meta.permissions)) return next({ name: 'overview' })
  
  next()
})
```

### 6.7 Limpeza no logout

No `useAuthStore.logout()`, todas as stores são resetadas automaticamente:

```ts
function logout() {
  user.value = null
  checked.value = false
  // Dispara reset em todas as stores via evento ou chamada direta
  resetAllStores()
}
```

---

## 7. Playground documentado

Substitui `/docs` (Scalar). Renderiza documentação de cada endpoint com:

- Método + Path
- Descrição
- Headers requeridos
- Request body (schema)
- Todas as responses (status + exemplo)
- Botão "Try it" (executa requisição real)

Definido no composable `useApiDocs.ts` como array tipado — não hardcoded no template.

---

## 8. Docker / Coolify

### 8.1 Dockerfile multi-stage

```
Stage 1 (ui-build): node:22-alpine → npm ci → npm run build → web/dist/
Stage 2 (go-build): golang:1.26-alpine → go build (SPA_DIR embedded ou em disco)
Stage 3 (runtime): alpine:3.20 + ffmpeg → copia binário + web/dist/
```

### 8.2 docker-compose.yml

```yaml
expose:
  - "${PORT:-3000}"    # Coolify lê e roteia; nunca ports:
environment:
  GOOGLE_CLIENT_ID: ${GOOGLE_CLIENT_ID}
  GOOGLE_CLIENT_SECRET: ${GOOGLE_CLIENT_SECRET}
  GOOGLE_REDIRECT_URL: ${GOOGLE_REDIRECT_URL}
  SPA_DIR: /app/web/dist
  # env vars removidas (agora no DB): MAX_UPLOAD_SIZE_MB, QUEUE_MAX_SIZE,
  # TRANSCODE_WORKERS, *_TOKEN_TTL, *_IDLE_TIMEOUT, TRANSCODE_STUCK,
  # MAX_TRANSCODE_ATTEMPTS, KEEP_ORIGINAL, RATE_LIMIT_PER_MIN,
  # WEBHOOK_URL, DISCORD_WEBHOOK_URL
```

### 8.3 Desenvolvimento local

```bash
# Terminal 1: Go API
go run ./cmd/server

# Terminal 2: Vite dev
cd web && npm run dev   # VITE_DEV_PORT=5173, proxy /api → localhost:3000
```

`vite.config.ts` sem localhost fixo — portas e targets via env vars.

---

## 9. Remoção de legado

Ao final, remover completamente:
- `internal/dashboard/` (dashboard.go, HTMLs, assets/*)
- `internal/playground/` (playground.go, index.html)
- `internal/docs/` (docs.go, Scalar UI)
- `POST /admin/session` (substituído por Google OAuth)
- Rotas `/playground`, `/dashboard/*`, `/docs`, `/docs/openapi.json` em server.go

---

## 10. Versioner — sync package.json

No `.agents/versioner.md`, Passo 4a: ao criar release, atualizar também
`web/package.json` campo `"version"` com a mesma versão do `VERSION`.

---

## 11. Regras mandatórias de código

- **Go**: identificadores em inglês, comentários em português, erros de API em português
- **Vue**: `<script setup lang="ts">` em **todos** os componentes (nunca Options API)
- **Stores Pinia**: setup syntax (`defineStore('x', () => { ... })`)
- **Composables**: regras de negócio em arquivos separados, testáveis isoladamente
- **Features**: um diretório por domínio; componentes compartilhados em `src/components/`
- **Segurança**: `beforeEach` bloqueia antes de carregar; stores resetam no logout; CSRF header em toda chamada não-GET

---

## 12. Task list (T75–T90)

| # | Tarefa | Depende |
|---|--------|---------|
| T75 | Migration SQL + aplicar goose | — |
| T76 | Modelos Go: User, UserRole, Configuration + queries | T75 |
| T77 | Pacote config/dbconfig — config manager com fallback | T75 |
| T78 | Google OAuth2 flow + session com user_id + roles | T76 |
| T79 | Middleware RoleAuth + proteção de rotas no server.go | T76 |
| T80 | CRUD admin/users + regra de nível + reprocess endpoint | T76, T79 |
| T81 | Config API: GET/PUT/DELETE /admin/config | T77, T79 |
| T82 | Wire completo server.go (SPA + auth + roles + remoção legado) | T78, T79, T80, T81 |
| T83 | Scaffold web/ (Vite + Vue 3 + TS + shadcn-vue + Tailwind + phosphor) | — |
| T84 | Router + stores + guards + menu + api client | T83 |
| T85 | Views: Login, Overview, Videos, Video, Playground | T84 |
| T86 | Views: Users, Config + RolesSelect + ConfigEditor | T84 |
| T87 | Docker multi-stage + docker-compose final | T82, T85 |
| T88 | Testes Go (auth, roles, users, config, dbconfig) + Vitest (stores, guards, menu) | T82, T86 |
| T89 | Remoção de legado (dashboard, playground, docs, POST /admin/session) | T82 |
| T90 | Atualizar spec/ + .agents/versioner.md (package.json) | T89 |
