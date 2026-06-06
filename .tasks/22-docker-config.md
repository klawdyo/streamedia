# T22: Dockerfile + docker-compose.yml + .env.example

**Status:** pending
**Dependências:** T20
**Estimativa:** pequena

## Contexto

O serviço roda em um único container Docker. O Dockerfile usa multi-stage build
para produzir um binário estático mínimo. O docker-compose.yml é fonte de
verdade das variáveis de ambiente para o Coolify.

## Dockerfile (multi-stage)

```dockerfile
# Estágio de build
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /mediaserver ./cmd/server

# Estágio de runtime
FROM alpine:3.20
RUN apk add --no-cache ffmpeg wget && \
    adduser -D -u 10001 appuser
COPY --from=build /mediaserver /usr/local/bin/mediaserver
USER appuser
EXPOSE 3000
ENTRYPOINT ["mediaserver"]
```

Notas:
- `CGO_ENABLED=0` é possível porque o driver SQLite é `modernc.org/sqlite` (Go puro)
- `ffmpeg` instalado via apk (inclui ffprobe)
- `wget` necessário para o healthcheck do Coolify
- `appuser` sem privilégios (UID 10001)

## docker-compose.yml

O bloco `environment` é a fonte de verdade para o Coolify.
Todas as variáveis declaradas com `${VAR}` ou `${VAR:-padrao}`.
Inclui variável `ADMIN_TOKEN` adicionada na T18.

```yaml
services:
  mediaserver:
    build: .
    restart: unless-stopped
    environment:
      - UPLOAD_TOKEN_SECRET=${UPLOAD_TOKEN_SECRET}
      - WEBHOOK_URL=${WEBHOOK_URL}
      - WEBHOOK_SECRET=${WEBHOOK_SECRET}
      - ADMIN_TOKEN=${ADMIN_TOKEN}
      - MAX_UPLOAD_SIZE_MB=${MAX_UPLOAD_SIZE_MB:-10}
      - MEDIA_DIR=${MEDIA_DIR:-/media}
      - UPLOAD_TMP_DIR=${UPLOAD_TMP_DIR:-/media/.uploads}
      - SQLITE_PATH=${SQLITE_PATH:-/data/media.db}
      - QUEUE_MAX_SIZE=${QUEUE_MAX_SIZE:-50}
      - TRANSCODE_WORKERS=${TRANSCODE_WORKERS:-1}
      - UPLOAD_TOKEN_TTL_H=${UPLOAD_TOKEN_TTL_H:-6}
      - PLAY_TOKEN_MAX_TTL_H=${PLAY_TOKEN_MAX_TTL_H:-6}
      - UPLOAD_IDLE_TIMEOUT_MIN=${UPLOAD_IDLE_TIMEOUT_MIN:-10}
      - TRANSCODE_STUCK_MIN=${TRANSCODE_STUCK_MIN:-30}
      - MAX_TRANSCODE_ATTEMPTS=${MAX_TRANSCODE_ATTEMPTS:-3}
      - KEEP_ORIGINAL=${KEEP_ORIGINAL:-false}
      - PORT=${PORT:-3000}
      - RATE_LIMIT_PER_MIN=${RATE_LIMIT_PER_MIN:-60}
    volumes:
      - media_files:/media
      - db_data:/data
    ports:
      - "${PORT:-3000}:3000"
    healthcheck:
      test: ["CMD", "wget", "--spider", "-q", "http://localhost:3000/healthz"]
      interval: 30s
      timeout: 5s
      retries: 3

volumes:
  media_files:
  db_data:
```

## .env.example

Ver conteúdo completo na spec `spec/ESPECIFICACAOv4.md` seção 11.
Inclui ADMIN_TOKEN com comentário explicativo.

## QA Instructions

Crie `docker_test.go` na raiz (ou use script de verificação):

```
TestDockerfileExists
  - Verifica que Dockerfile existe no repositório

TestDockerComposeExists
  - Verifica que docker-compose.yml existe

TestEnvExampleExists
  - Verifica que .env.example existe

TestEnvExampleHasAllVars
  - Lê .env.example
  - Verifica que todas as variáveis do docker-compose estão documentadas:
    UPLOAD_TOKEN_SECRET, WEBHOOK_URL, WEBHOOK_SECRET, ADMIN_TOKEN,
    MAX_UPLOAD_SIZE_MB, MEDIA_DIR, UPLOAD_TMP_DIR, SQLITE_PATH,
    QUEUE_MAX_SIZE, TRANSCODE_WORKERS, etc.

TestGitignoreHasEnv
  - Lê .gitignore
  - Verifica que ".env" está presente

TestDockerComposeSyntax
  - Parseia o docker-compose.yml como YAML
  - Verifica que o campo services.mediaserver existe
  - Verifica que volumes estão definidos

TestDockerfileMultiStage
  - Lê Dockerfile
  - Verifica presença de "FROM golang" e "FROM alpine"
  - Verifica presença de "CGO_ENABLED=0"
  - Verifica presença de "USER appuser"
```

Esses são testes de configuração — verificam a integridade dos arquivos de deploy.
Use `os.ReadFile` e comparações de string.

## Dev Instructions

1. Crie o `Dockerfile` conforme especificado acima
2. Crie o `docker-compose.yml` conforme especificado
3. Crie o `.env.example` com comentários em português (ver spec seção 11)
   - Adicione `ADMIN_TOKEN` com comentário explicativo
4. Verifique que o `.gitignore` já tem `.env` (criado na T01)

## Arquivos a criar/modificar

- `Dockerfile`
- `docker-compose.yml`
- `.env.example`
- `docker_test.go`

## Definition of Done

- [ ] `docker build .` funciona (se Docker disponível)
- [ ] docker-compose.yml sintaticamente válido
- [ ] .env.example documenta todas as variáveis
- [ ] ADMIN_TOKEN incluído em todos os arquivos
- [ ] Todos os testes passam
