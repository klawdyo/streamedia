# Estágio de build — compila o binário estático
FROM golang:1.26-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download
COPY . .

# A versão é lida do arquivo VERSION na raiz do repositório — o agente
# Versioner o atualiza a cada release. Nenhum --build-arg manual é necessário.
# O Commit é extraído do git (se disponível no contexto de build).
RUN --mount=type=bind,source=.git,target=.git \
  CGO_ENABLED=0 go build \
  -ldflags="-X github.com/klawdyo/streamedia/internal/version.Version=$(cat VERSION) \
            -X github.com/klawdyo/streamedia/internal/version.Commit=$(git rev-parse --short HEAD 2>/dev/null || echo unknown)" \
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
