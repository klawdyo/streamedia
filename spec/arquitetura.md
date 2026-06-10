# Arquitetura e componentes

O Streamedia é um serviço independente especializado no pipeline de vídeo.
Quem decide *quem pode o quê* é o **backend principal** (fora deste projeto);
o Streamedia executa upload, transcodificação e entrega.

## Fluxo de dados

```
Backend Principal ──POST /api/upload/init (Bearer ROOT_TOKEN)──► Streamedia
                                                                  │
App/Client ──TUS Upload (Upload-Token)────────────────────────────┘
                                                                  │
                                                            FFmpeg → HLS
                                                                  │
                                                        <MEDIA_DIR>/<tag>/<id>/
                                                                  │
Streamedia ──Webhook (assinado)─────────────────────► Backend Principal
                                                                  │
App/Client ◄── GET /video/<tag>/<id>.m3u8?token=...  (via /api/play/init)
```

## Componentes

- **HTTP (chi):** roteador, middlewares (recovery, rate limit, telemetria) e
  os handlers. Montado em `internal/server`.
- **Upload:** `internal/upload` — `/api/upload/init` (registra vídeo + emite
  token) e o handler TUS (`tusd`) para o envio resumível dos bytes.
- **Transcodificação:** `internal/transcode` — fila + workers FFmpeg que geram
  as variantes HLS e o `master.m3u8`.
- **Serving:** `internal/serve` — `/api/play/init` (emite URL assinada) e a
  entrega do master (dinâmico) e segmentos (estáticos).
- **Dados:** `internal/models` + `internal/db` (SQLite via migrations goose).
- **Jobs:** `internal/jobs` — manutenção periódica (ver [pipeline.md](pipeline.md)).
- **Webhook:** `internal/webhook` — notifica o backend principal a cada
  transição relevante (ver [webhooks.md](webhooks.md)).
- **Observabilidade:** `internal/telemetry` (`/metrics`) + `internal/version` (`/api`).

## Princípios

- **Único cliente privilegiado:** só o backend principal detém credencial
  durável (`ROOT_TOKEN`). O usuário final nunca toca a API de gestão.
- **Tag como namespace:** organização e isolamento de armazenamento, sem
  credencial própria (ver [autenticacao.md](autenticacao.md)).
- **Observabilidade nunca derruba o serviço:** falhas de métrica/estatística
  são logadas e ignoradas.
