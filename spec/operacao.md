# Operação: variáveis de ambiente, deploy e observabilidade

## Variáveis de ambiente

Lidas em `internal/config`. Exemplo em `.env.example`; em produção (Coolify),
configuradas no painel (o `docker-compose.yml` usa `${VAR:-default}`).

| Variável | Obrigatória | Padrão | Descrição |
|---|---|---|---|
| `ROOT_TOKEN` | Sim | — | Credencial única de gestão (Bearer). `openssl rand -hex 32`. |
| `WEBHOOK_SECRET` | Sim¹ | — | Assina os webhooks (HMAC). Compartilhado com o backend. |
| `WEBHOOK_URL` | Não | — | Destino global dos webhooks. Pode ser sobrescrito por vídeo (campo `webhook_url` no upload/init). |
| `DISCORD_WEBHOOK_URL` | Não | — | Webhook do Discord para **alertas operacionais** internos (falhas de transcode, fila cheia, transcode travado, falhas consecutivas). Vazio = canal desabilitado. |
| `MAX_UPLOAD_SIZE_MB` | Não | `10` | Tamanho máximo de upload (MB). |
| `MEDIA_DIR` | Não | `/media` | Raiz dos arquivos HLS (`<MEDIA_DIR>/<tag>/<id>/`). |
| `UPLOAD_TMP_DIR` | Não | `/media/.uploads` | Uploads TUS em andamento. |
| `SQLITE_PATH` | Não | `/data/media.db` | Arquivo SQLite. |
| `QUEUE_MAX_SIZE` | Não | `50` | Capacidade da fila de transcodificação. |
| `TRANSCODE_WORKERS` | Não | `1` | Workers FFmpeg simultâneos. |
| `UPLOAD_TOKEN_TTL` | Não | `1200` | TTL do token de upload (segundos). |
| `PLAY_TOKEN_TTL` | Não | `3600` | TTL do token de play (segundos). |
| `UPLOAD_IDLE_TIMEOUT` | Não | `600` | Inatividade até matar upload (segundos). |
| `TRANSCODE_STUCK` | Não | `1800` | Tempo até considerar transcode travado (segundos). |
| `MAX_TRANSCODE_ATTEMPTS` | Não | `3` | Tentativas antes de `failed_transcode`. |
| `KEEP_ORIGINAL` | Não | `false` | Manter o arquivo bruto após transcodificar. |
| `PORT` | Não | `3000` | Porta HTTP. |
| `RATE_LIMIT_PER_MIN` | Não | `60` | Limite de requisições por IP/min. |

> As variáveis de tempo são em **segundos** (sem sufixo no nome).
>
> ¹ `WEBHOOK_SECRET` é obrigatório **quando** `WEBHOOK_URL` está definida (ou
> quando se usa `webhook_url` por vídeo): é o segredo que assina todos os
> webhooks de negócio, independentemente do destino.

## Deploy

- **Docker:** imagem multi-stage; processo roda como usuário não-root.
  `docker-compose.yml` define o serviço, volumes nomeados (mídia + banco) e o
  healthcheck (`GET /healthz`).
- **Coolify:** apontar para o repositório (Docker Compose), configurar as
  variáveis no painel (marcar "Is Literal" nos segredos) e volumes persistentes.

## Observabilidade

- `GET /metrics` — métricas no formato Prometheus (contador/histograma de
  requisições rotulados por método/rota/status, gauges de fila, uploads em
  andamento e eventos de playback). Sem auth — proteja na camada de rede.
- `GET /api` — nome, versão (injetada via `-ldflags`) e status; rate limit baixo.
- `GET /admin/stats` — estatísticas agregadas de uso e armazenamento
  (Bearer ROOT_TOKEN).

## Alertas operacionais no Discord (opcional)

Independente dos webhooks de negócio (que notificam o backend principal sobre o
ciclo de vida do vídeo), o serviço pode alertar a **equipe de operação** sobre
falhas internas via um webhook do Discord. Configurado por `DISCORD_WEBHOOK_URL`
(`internal/discord`); sem ela, nenhum envio é tentado.

Gatilhos (cada um vira um *embed* com `video_id`, `status`, `error_message` e
timestamp):

| Gatilho | Quando |
|---|---|
| Falha na transcodificação (`failed_transcode`) | vídeo esgotou as tentativas e foi marcado como falha (no worker ou no job de requeue). |
| Fila de transcodificação cheia | um job não pôde entrar na fila (`QUEUE_MAX_SIZE` atingido). |
| Transcode travado (`TRANSCODE_STUCK`) | o job de manutenção detectou um vídeo parado em `transcoding` além do timeout. |
| Aumento anormal de falhas consecutivas | ≥ 5 falhas terminais seguidas, sem nenhum sucesso entre elas (indica problema sistêmico). |

O envio é *best-effort*: cada tentativa (sucesso/falha) é registrada no log da
aplicação (`[discord] ...`) e nunca bloqueia o pipeline nem propaga erro.
