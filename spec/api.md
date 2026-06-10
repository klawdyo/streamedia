# Rotas da API

Detalhe completo (schemas, parâmetros) em OpenAPI: `GET /docs/openapi.json`
(UI em `GET /docs`). Abaixo, o resumo por rota.

## Upload

### `POST /api/upload/init` — Bearer ROOT_TOKEN
Registra o vídeo no namespace (`tag`) e emite um token de upload efêmero.
Corpo: `{ "tag": "...", "video_id"?: "<uuid>", "declared_size_bytes": N }`.
`video_id` é opcional (UUID; gerado como v7 se omitido).
Resposta `200`: `{ video_id, tag, upload_url, token }`.
Erros: `400` (tag ausente / video_id inválido), `401`, `409` (já existe),
`413` (acima do limite).

### TUS `/files` e `/files/{video_id}`
Protocolo TUS resumível (tusd). Autenticado pelo `Upload-Token` (o `token` do
init). `POST` cria, `PATCH` envia chunk, `HEAD` consulta offset, `DELETE`
cancela. Ao concluir, o arquivo é validado (tamanho + magic bytes + ffprobe) e
enfileirado para transcodificação.

## Play e serving

### `POST /api/play/init` — Bearer ROOT_TOKEN
O backend (já tendo autorizado o usuário) troca o ROOT_TOKEN por uma URL
assinada. Corpo: `{ "video_id": "<uuid>" }`. Exige vídeo `ready`.
Resposta `200`: `{ video_id, tag, play_url, token, expires_at }`.

### `GET /video/{tag}/{video_id}.m3u8?token=...`
Master playlist **dinâmico**: valida o token de play (lookup), exige status
`ready`, e reescreve as referências de variante para incluir o `video_id`. O
caminho real no disco fica escondido.

### `GET /video/{tag}/{video_id}/{resolution}/playlist.m3u8` e `.../{segment}`
Playlists de resolução e segmentos `.ts` — **estáticos e públicos** (os nomes
opacos no master funcionam como a "chave"). Validação rígida de path
(resolução permitida, sem traversal).

> Padrão recomendado de entrega: o backend devolve ao app uma URL própria
> estável (`backend/api/video/{id}/play`); no play, autoriza e responde **302**
> (com `Cache-Control: no-store`) para a `play_url`. Assim só há contato com o
> Streamedia na reprodução, nunca ao listar a timeline.

## Status e administração — Bearer ROOT_TOKEN

| Rota | Descrição |
|---|---|
| `GET /api/status/{video_id}` | Estado do vídeo + metadados. |
| `GET /admin/videos` | Lista paginada; filtros `status`, `tag`, `limit`, `offset`. |
| `GET /admin/queue` | Tamanho da fila + nº de workers. |
| `GET /admin/stats` | Estatísticas agregadas (e armazenamento/fila na visão global). |
| `DELETE /admin/videos/{video_id}` | Apaga linhas do banco + arquivos no disco. |

## Rotas públicas

| Rota | Descrição |
|---|---|
| `GET /healthz` | Healthcheck. |
| `GET /api` | Nome, versão e status (rate limit 10/min). |
| `GET /metrics` | Métricas Prometheus. |
| `GET /docs`, `GET /docs/openapi.json` | Documentação (Scalar UI + OpenAPI). |

## Envelope de resposta

Toda rota **JSON** responde no envelope padrão:

```json
{ "error": false, "message": "ok", "data": { ... }, "status_code": 200 }
```

- `error`: `true` em falha, `false` em sucesso.
- `message`: `"ok"` em sucesso; mensagem descritiva (em português) em erro.
- `data`: payload em sucesso; `null` em erro.
- `status_code`: espelha o status HTTP.

Exceções (servem conteúdo binário/texto, não JSON): os arquivos HLS
(`.m3u8`/`.ts`) em caso de sucesso, e `/metrics`. Erros dessas rotas (ex. token
inválido no master) ainda seguem o envelope.
