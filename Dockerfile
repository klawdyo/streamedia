# Estágio de build — compila o binário estático
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /mediaserver ./cmd/server

# Estágio de runtime — imagem mínima com FFmpeg
FROM alpine:3.20
RUN apk add --no-cache ffmpeg wget && \
    adduser -D -u 10001 appuser
COPY --from=build /mediaserver /usr/local/bin/mediaserver
USER appuser
EXPOSE 3000
ENTRYPOINT ["mediaserver"]
