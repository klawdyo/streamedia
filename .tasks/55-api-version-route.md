# T55: Rota `GET /api` — nome, status e versão da API

**Status:** done
**Dependências:** nenhuma (cria o pacote `internal/version`; pode ser feita a qualquer momento)
**Estimativa:** pequena
**Origem:** solicitação direta do usuário — expor nome, status e versão da API
em uma rota pública com rate limiting baixo

## Contexto

O usuário quer uma rota que qualquer um possa bater para saber:
- **Nome** da API (`"Streamedia"`)
- **Versão** (ex.: `"0.1.0"`)
- **Status** (`"ok"` ou `"degraded"`)

Sem autenticação (rota pública de diagnóstico), mas com **rate limiting baixo**
(ex.: 10 req/min) para mitigar abuso. A rota não deve conflitar com
`/api/status/{video_id}` (T13).

A resposta segue o envelope padrão (`apiresponse.Success`, T45):

```json
{
  "error": false,
  "message": "ok",
  "data": {
    "name": "Streamedia",
    "version": "0.1.0",
    "status": "ok"
  },
  "status_code": 200
}
```

Esta task também cria a **infraestrutura de versionamento** que faltava no
projeto — o pacote `internal/version` com variáveis injetáveis via `-ldflags`
no `go build`, que é o padrão da comunidade Go (Kubernetes, Prometheus,
Terraform, etc.).

## Padrão de versionamento em Go: `-ldflags`

A comunidade Go adota universalmente **injeção de valores em tempo de link**
via `-ldflags`. Nenhuma dependência externa, nenhum runtime cost:

```go
// internal/version/version.go
package version

// Version é a versão semântica do binário, injetada via -ldflags no build.
// O valor padrão "0.0.0-dev" é usado em desenvolvimento (go run / go test);
// em builds oficiais (Docker, CI), o valor real é injetado.
var Version = "0.0.0-dev"

// Commit é o hash curto do commit Git no momento do build.
var Commit = "unknown"

// BuildTime é o timestamp UTC de quando o binário foi compilado.
var BuildTime = "unknown"
```

### Injeção no build local

```bash
go build -ldflags="
  -X github.com/klawdyo/streamedia/internal/version.Version=0.1.0
  -X github.com/klawdyo/streamedia/internal/version.Commit=$(git rev-parse --short HEAD)
  -X github.com/klawdyo/streamedia/internal/version.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)
" -o mediaserver ./cmd/server
```

### No Dockerfile (multi-stage)

```dockerfile
ARG VERSION=0.0.0-dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

RUN CGO_ENABLED=0 go build \
  -ldflags="-X github.com/klawdyo/streamedia/internal/version.Version=${VERSION} \
            -X github.com/klawdyo/streamedia/internal/version.Commit=${COMMIT} \
            -X github.com/klawdyo/streamedia/internal/version.BuildTime=${BUILD_TIME}" \
  -o /mediaserver ./cmd/server
```

### No GitHub Actions (release.yml)

```yaml
- run: |
    go build -ldflags="
      -X github.com/klawdyo/streamedia/internal/version.Version=${{ github.ref_name }}
      -X github.com/klawdyo/streamedia/internal/version.Commit=${{ github.sha }}
    " -o mediaserver ./cmd/server
```

### Origem da versão

A versão injetada vem do **agente Versioner** (`.agents/versioner.md`), que
calcula `MAJOR.MINOR.PATCH` a partir do histórico de commits semânticos
(`feat:` → MINOR++, `fix:` → PATCH++). O Versioner gera um commit
`release: vX.Y.Z` que serve de checkpoint, e o valor de `vX.Y.Z` é o que
vai para o `-ldflags`.

## Rate limiting dedicado

Esta rota usa um **rate limiter separado**, com threshold baixo (10 req/min),
diferente do rate limiter global (padrão 60 req/min). Isso evita que um
scraper agressivo em `/api` consuma o orçamento de outras rotas
públicas (healthz, upload, etc.).

O middleware de rate limiting por IP já existe (`internal/middleware/ratelimit.go`)
— a rota simplesmente instancia um segundo `RateLimiter` com limite menor.

## QA Instructions

```
TestVersionRoute_ReturnsNameVersionAndStatus
  - GET /api/version → 200
  - corpo decodifica como apiresponse.Envelope
  - error == false, message == "ok"
  - data.name == "Streamedia"
  - data.version não é vazia (em testes, será "0.0.0-dev")
  - data.status == "ok"
  - Content-Type: application/json; charset=utf-8

TestVersionRoute_RateLimited
  - excede 10 requisições em menos de 1 minuto
  - a 11ª retorna 429 no envelope padrão
  - o rate limiter da rota é independente do global (bater /api/version não
    afeta /healthz)

TestVersionPackage_Defaults
  - importa internal/version e verifica:
  - Version == "0.0.0-dev" (padrão sem ldflags)
  - Commit == "unknown"
  - BuildTime == "unknown"
  - GetVersionInfo() retorna struct com os 3 campos preenchidos
```

## Dev Instructions

### 1. Crie o pacote `internal/version`

```go
// internal/version/version.go
package version

// VersionInfo agrupa as informações de build expostas pela rota /api/version.
type VersionInfo struct {
    Name      string `json:"name"`
    Version   string `json:"version"`
    Commit    string `json:"commit,omitempty"`
    BuildTime string `json:"build_time,omitempty"`
    Status    string `json:"status"`
}

// GetVersionInfo devolve as informações de build, usando os valores
// injetados via -ldflags (ou os defaults em desenvolvimento).
func GetVersionInfo() VersionInfo {
    return VersionInfo{
        Name:      "Streamedia",
        Version:   Version,
        Commit:    Commit,
        BuildTime: BuildTime,
        Status:    "ok",
    }
}
```

### 2. Registre a rota em `internal/server/server.go`

```go
// --- Versão da API (pública, rate limit baixo) ---
versionLimiter := middleware.NewRateLimiter(10) // 10 req/min
r.Group(func(r chi.Router) {
    r.Use(versionLimiter.Middleware)
    r.Get("/api/version", func(w http.ResponseWriter, _ *http.Request) {
        apiresponse.Success(w, http.StatusOK, version.GetVersionInfo())
    })
})
```

### 3. Atualize o Dockerfile

Adicione `ARG VERSION=0.0.0-dev` e use `-ldflags` no `go build` (ver seção
"Padrão de versionamento" acima).

### 4. Atualize `release.yml` (GitHub Actions)

Passe a tag/versão do release para o `-ldflags` no step de build.

### 5. Atualize o agente Versioner

Adicione em `.agents/versioner.md` uma nota sobre o pacote `internal/version`
e como o valor calculado por ele chega ao binário (via `-ldflags`).

## Arquivos a criar/editar

- `internal/version/version.go` (novo pacote)
- `internal/version/version_test.go`
- `internal/server/server.go` (nova rota + rate limiter dedicado)
- `internal/server/server_test.go` (teste da rota)
- `Dockerfile` (ARG VERSION + ldflags)
- `.github/workflows/release.yml` (ldflags no build)
- `.agents/versioner.md` (nota sobre o pacote)
- `spec/ESPECIFICACAOv4.md` (nova rota na seção 9)

## Resolução

T55 já estava implementada antes da análise de código (T56-T68). O pacote
`internal/version` existe com `Version`, `Commit`, `BuildTime` e `Get()`.
A rota `GET /api` está registrada em `server.go:168-174` com rate limiter
dedicado de 10 req/min. Dockerfile e server.go já usam o pacote.

## Definition of Done

- [x] Pacote `internal/version` criado com variáveis injetáveis via `-ldflags`
- [x] Rota `GET /api` registrada, respondendo no envelope padrão
- [x] Rate limiting dedicado de 10 req/min aplicado à rota
- [x] Dockerfile atualizado com `ARG VERSION` e `-ldflags`
- [x] `release.yml` atualizado para injetar versão no build
- [x] Agente Versioner atualizado com referência ao pacote
- [x] Spec atualizada com a nova rota
- [x] `go test ./...` e `go vet ./...` passam