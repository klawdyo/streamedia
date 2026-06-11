# Rotas da API

Detalhe completo (schemas, parâmetros) em OpenAPI: `GET /docs/openapi.json`
(UI em `GET /docs`). Abaixo, o resumo por rota.

## Upload

### `POST /api/upload/init` — Bearer ROOT_TOKEN
Registra o vídeo no namespace (`tag`) e emite um token de upload efêmero.
Corpo: `{ "tag": "...", "video_id"?: "<uuid>", "declared_size_bytes": N, "webhook_url"?: "https://..." }`.
`video_id` é opcional (UUID; gerado como v7 se omitido). `webhook_url` é
opcional: quando informado (URL **HTTPS** válida, ≤ 2048 caracteres), os
webhooks **deste vídeo** vão para essa URL em vez da `WEBHOOK_URL` global;
omitido usa a global. A assinatura HMAC (`WEBHOOK_SECRET`) é a mesma em ambos.
Resposta `200`: `{ video_id, tag, upload_url, token }`.
Erros: `400` (tag ausente / video_id inválido / `webhook_url` inválida), `401`,
`409` (já existe), `413` (acima do limite).

### TUS `/files` e `/files/{video_id}`
Protocolo TUS resumível (tusd). Autenticado pelo `Upload-Token` (o `token` do
init). `POST` cria, `PATCH` envia chunk, `HEAD` consulta offset, `DELETE`
cancela. Ao concluir, o arquivo é validado (tamanho + magic bytes + ffprobe) e
enfileirado para transcodificação.

## Play e serving

### `POST /api/play/init` — Bearer ROOT_TOKEN
O backend (já tendo autorizado o usuário) troca o ROOT_TOKEN por uma URL
assinada. Corpo: `{ "video_id": "<uuid>" }`. Exige vídeo `ready`.
Resposta `200`: `{ video_id, tag, play_url, token, expires_at, resolutions }`.
`resolutions` é a lista de alturas das variantes HLS disponíveis, ordenada
asc (ex.: `[480, 720, 1080]`) — útil para montar players por resolução; as
playlists públicas ficam em `/video/{tag}/{video_id}/{resolution}/playlist.m3u8`.

### `GET /video/{tag}/{video_id}.m3u8?token=...`
Master playlist **dinâmico**: valida o token de play (lookup), exige status
`ready`, e reescreve as referências de variante para incluir o `video_id`. O
caminho real no disco fica escondido.

### `GET /video/{tag}/{video_id}/{resolution}/playlist.m3u8` e `.../{segment}`
Playlists de resolução e segmentos `.ts` — **estáticos e públicos** (os nomes
opacos no master funcionam como a "chave"). Validação rígida de path
(resolução permitida, sem traversal).

### `GET /video/{tag}/{video_id}/thumb_{resolution}.jpg`
Thumbnail (poster) **JPEG** da resolução, **público e sem autenticação** (poster
é público por natureza). Gerado ao final da transcodificação, um por resolução,
a partir de um frame a 1s do vídeo (fallback para o primeiro frame em vídeos
curtos), escalado preservando a proporção original (16:9, 9:16, 4:3, …) com a
menor dimensão igual à resolução. Disco:
`<MEDIA_DIR>/<tag>/<video_id>/thumb_<resolution>.jpg`. Só aceita as resoluções
suportadas (480/720/1080); demais nomes retornam `400`.

> Padrão recomendado de entrega: o backend devolve ao app uma URL própria
> estável (`backend/api/video/{id}/play`); no play, autoriza e responde **302**
> (com `Cache-Control: no-store`) para a `play_url`. Assim só há contato com o
> Streamedia na reprodução, nunca ao listar a timeline.

## Eventos do pipeline (notificações)

Cada evento do pipeline (`processing`, `ready`, `failed`) vira uma
**notificação** com o mesmo payload, distribuída em paralelo para os destinos
ativos: o **webhook** (se houver URL) e o **SSE** (se houver ouvinte). Payload:
`{ video_id, event, status, duration_s, resolutions, error_message, timestamp }`.

### `GET /api/events?video_id=<uuid>&token=<upload-token>` — SSE
Stream **Server-Sent Events** ao vivo dos eventos de um vídeo. Escopado por
`video_id` e autenticado pelo **token de upload** do vídeo (o `token` do
`upload/init`), passado na query porque o `EventSource` do navegador não envia
cabeçalhos. Permite a um app de usuário acompanhar o próprio upload/transcode
sem rotear pelo backend nem expor o `ROOT_TOKEN`. Cada evento chega como
`event: <nome>\ndata: <json>`. Sem buffer/replay: entrega apenas o que ocorre
enquanto o cliente está conectado. Erros: `400` (faltam `video_id`/`token`),
`401` (token inválido/expirado ou de outro vídeo).

### Webhook — opcional (`WEBHOOK_URL`)
Se `WEBHOOK_URL` estiver definida, cada notificação é enviada via `POST`
assinado (HMAC `X-Signature: sha256=...`, segredo `WEBHOOK_SECRET`), com até 3
tentativas (backoff 1s/2s/4s) e registro em `webhook_log`. **Sem `WEBHOOK_URL`,
nenhum webhook é enviado** (o SSE continua funcionando). Quando `WEBHOOK_URL`
está definida, `WEBHOOK_SECRET` passa a ser obrigatório.

## Status e administração — Bearer ROOT_TOKEN

| Rota | Descrição |
|---|---|
| `GET /api/status/{video_id}` | Estado do vídeo + metadados (inclui `has_thumbnails` e `thumbnails`, mapa resolução→URL pública do poster). |
| `GET /admin/videos` | Lista paginada; filtros `status`, `tag`, `limit`, `offset`; ordenação `sort` (`created_at`/`updated_at`/`status`/`actual_size_bytes`/`duration_s`, whitelist) + `order` (`asc`/`desc`). |
| `GET /admin/queue` | Tamanho da fila + nº de workers. |
| `GET /admin/stats` | Estatísticas agregadas de reprodução (totais, `by_resolution`, `by_os`, `by_day_of_week`, `by_hour`, `by_date`). Visão global (sem `?video_id=`) inclui `storage` (bytes, duração, vídeos por status, fila, `workers`) e `uploads` (envios por data/dia/hora). Com `?video_id=`, inclui `video_storage` (variantes HLS + peso do vídeo). |
| `DELETE /admin/videos/{video_id}` | Apaga linhas do banco + arquivos no disco. |

## Rotas públicas

| Rota | Descrição |
|---|---|
| `GET /healthz` | Healthcheck. |
| `GET /api` | Nome, versão e status (rate limit 10/min). |
| `GET /metrics` | Métricas Prometheus. |
| `GET /docs`, `GET /docs/openapi.json` | Documentação (Scalar UI + OpenAPI). |
| `GET /playground` | Playground interativo do pipeline (auth → upload → play). |
| `GET /dashboard`, `GET /dashboard/videos`, `GET /dashboard/videos/{id}`, `GET /dashboard/assets/{file}` | Dashboard administrativo (páginas HTML). |

### Playground da API (`GET /playground`) — issue #18

Página HTML autocontida (sem build step) que exercita o ciclo de vida completo
de um vídeo na mesma página: cola-se o `ROOT_TOKEN`, escolhe-se o arquivo e
solicita-se o link de upload, envia-se em chunks via TUS (com barra de progresso
por chunk e unificada, timeout/cancelar/retry), acompanha-se o status e os
**eventos ao vivo via SSE** (`/api/events`), emite-se o link de play e geram-se
players HLS para as resoluções disponíveis, cada um com ▶ Play individual.
Rota pública (a página só age com o `ROOT_TOKEN` colado pelo usuário).

### Dashboard administrativo (`GET /dashboard`)

Área visual de administração, no mesmo tema escuro "inspirado no Scalar" do
playground, em três páginas HTML autocontidas (Chart.js e hls.js via CDN):

- **`/dashboard`** — visão geral: cartões (total de vídeos, prontos, em
  processamento, com falha, espaço usado, duração total, fila + workers,
  reproduções) + gráficos de **uploads** e **reproduções** por data, dia da
  semana e hora + tabela dos últimos vídeos enviados, com link para "ver todos",
  `/playground` e `/docs`.
- **`/dashboard/videos`** — biblioteca completa com paginação, filtros
  (`status`, `tag`) e ordenação (`sort`/`order`), sobre `GET /admin/videos`.
- **`/dashboard/videos/{id}`** — player HLS (estilo YouTube) e as estatísticas
  do vídeo na mesma página (reproduções por data/dia/hora/resolução/SO e ficha
  de armazenamento), sobre `GET /api/status/{id}`, `GET /admin/stats?video_id=`
  e `POST /api/play/init`.

**Autenticação (mesmo padrão do playground):** as páginas são **públicas** — não
fazem nada de útil sem o `ROOT_TOKEN`. O token é colado uma vez, guardado no
`sessionStorage` do navegador e enviado em `Authorization: Bearer` a cada
chamada das rotas de dados (`/admin/*`, `/api/status`, `/api/play/init`), que
continuam exigindo o `ROOT_TOKEN` no servidor. Nenhum dado é acessível sem ele.

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
