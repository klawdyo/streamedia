# Estágio de build — compila o binário estático
FROM golang:1.26-alpine AS build
WORKDIR /src

# Commit do git, injetado via build-arg. É opcional: quando o contexto de
# build não traz informação de git (ex.: o Coolify importa o repositório sem
# o diretório .git), permanece "unknown". Localmente pode-se passar
# `--build-arg GIT_COMMIT=$(git rev-parse --short HEAD)`.
ARG GIT_COMMIT=unknown

COPY go.mod go.sum ./
RUN go mod download
COPY . .

# A versão é lida do arquivo VERSION na raiz do repositório — o agente
# Versioner o atualiza a cada release. Nenhum --build-arg manual é necessário.
# O Commit vem do build-arg GIT_COMMIT (fallback "unknown"); não dependemos
# mais de um bind mount de .git, que falha quando o contexto não tem git.
RUN CGO_ENABLED=0 go build \
  -ldflags="-X github.com/klawdyo/streamedia/internal/version.Version=$(cat VERSION) \
            -X github.com/klawdyo/streamedia/internal/version.Commit=${GIT_COMMIT}" \
  -o /mediaserver ./cmd/server

# Estágio de runtime — imagem mínima com FFmpeg
FROM alpine:3.20
RUN apk add --no-cache ffmpeg wget && \
    adduser -D -u 10001 appuser && \
    mkdir -p /data /media && \
    chown appuser:appuser /data /media
COPY --from=build /mediaserver /usr/local/bin/mediaserver
USER appuser
EXPOSE 3000
ENTRYPOINT ["mediaserver"]
