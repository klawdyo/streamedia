# Estágio de build — compila o binário estático
FROM golang:1.23-alpine AS build
WORKDIR /src

# Versão injetada no binário via -ldflags. O padrão "0.0.0-dev" aparece em
# builds locais; em CI/release, o valor real é passado via --build-arg.
ARG VERSION=0.0.0-dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

COPY go.mod go.sum ./
RUN go mod download
COPY . .

# CGO_ENABLED=0 gera binário estático (sem dependência de libc).
# -ldflags injeta as variáveis do pacote internal/version no binário.
RUN CGO_ENABLED=0 go build \
  -ldflags="-X github.com/klawdyo/streamedia/internal/version.Version=${VERSION} \
            -X github.com/klawdyo/streamedia/internal/version.Commit=${COMMIT} \
            -X github.com/klawdyo/streamedia/internal/version.BuildTime=${BUILD_TIME}" \
  -o /mediaserver ./cmd/server

# Estágio de runtime — imagem mínima com FFmpeg
FROM alpine:3.20
RUN apk add --no-cache ffmpeg wget && \
    adduser -D -u 10001 appuser
COPY --from=build /mediaserver /usr/local/bin/mediaserver
USER appuser
EXPOSE 3000
ENTRYPOINT ["mediaserver"]
