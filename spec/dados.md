# Modelo de dados e armazenamento

Banco: SQLite, schema versionado via goose (`internal/db/migrations/`).
Definição autoritativa: `0001_init.sql`.

## Tabelas

### `videos`
Registro central de cada vídeo.

| Coluna | Tipo | Notas |
|---|---|---|
| `video_id` | TEXT PK | UUID. |
| `status` | TEXT | máquina de estados (ver [pipeline.md](pipeline.md)). |
| `declared_size_bytes` / `actual_size_bytes` | INTEGER | declarado no init / real após upload. |
| `duration_s` | INTEGER | preenchido na transcodificação. |
| `resolutions` | TEXT | JSON com as resoluções geradas. |
| `transcode_attempts` | INTEGER | contador para o limite de tentativas. |
| `last_chunk_at` | DATETIME | último chunk recebido (usado pelo killer). |
| `error_message` | TEXT | motivo da última falha. |
| `tag` | TEXT NOT NULL DEFAULT `'default'` | namespace; define o diretório no disco. |
| `webhook_url` | TEXT NOT NULL DEFAULT `''` | destino de webhook customizado deste vídeo (issue #20); `''` = usa a `WEBHOOK_URL` global. |
| `created_at` / `updated_at` | DATETIME | `updated_at` mantido por trigger. |

Índices: `status`, `last_chunk_at`, `tag`.

### `access_tokens`
Tokens efêmeros de upload e play (ver [autenticacao.md](autenticacao.md)).

| Coluna | Tipo | Notas |
|---|---|---|
| `token` | TEXT PK | string aleatória opaca. |
| `video_id` | TEXT | FK → `videos`. |
| `purpose` | TEXT NOT NULL | `'upload'` ou `'play'`. |
| `expires_at` | DATETIME NOT NULL | limpeza diária remove os expirados. |

Constraint `UNIQUE(video_id, purpose)`; índice em `expires_at`.

### `video_renditions`
Tamanho e contagem de segmentos por variante (estatísticas de armazenamento).
PK `(video_id, resolution)`.

### `playback_events`
Eventos brutos de uso (`playback`, `download_segment`, `upload_complete`),
agregados por `/admin/stats`.

### `webhook_log`
Histórico de tentativas de webhook (evento, payload, sucesso).

## Armazenamento em disco

```
<MEDIA_DIR>/<tag>/<video_id>/
├── master.m3u8
├── 480/  playlist.m3u8  0.ts 1.ts ...
├── 720/  playlist.m3u8  ...
└── 1080/ ...
```

Uploads em andamento (TUS) ficam em `<UPLOAD_TMP_DIR>` até a validação final;
em sucesso o arquivo bruto é removido (a menos que `KEEP_ORIGINAL=true`).
