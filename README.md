# Streamedia — Servidor de Mídia

Serviço Go de upload, transcodificação e entrega de vídeo em HLS. Substitui o Bunny Stream em uma rede social de vídeo estilo Instagram.

## Papel na Arquitetura

O Streamedia atua como um serviço independente especializado no pipeline de vídeo:

```
Backend Principal ──POST /api/upload/init──► Streamedia
                                              │
Flutter Client ──TUS Upload──────────────────┘
                                              │
                                        FFmpeg (HLS)
                                              │
                                    S3 / Volume Local
                                              │
Streamedia ──Webhook──────────────► Backend Principal
                                              │
Flutter Client ◄── GET /video/{tag}/{id}.m3u8?token=...
```

## Fluxo de Upload

```
1. Backend Principal chama POST /api/upload/init (Authorization: Bearer ROOT_TOKEN; body com tag)
2. Streamedia registra o vídeo no banco (status: pending_upload)
3. Streamedia devolve { video_id, tag, upload_url, token }
4. Flutter Client usa TUS para enviar o arquivo para upload_url
5. Streamedia enfileira o vídeo para transcodificação (status: upload_complete)
6. Worker de transcodificação chama FFmpeg → gera HLS (status: transcoding)
7. Transcodificação concluída (status: ready)
8. Streamedia dispara webhook ao Backend Principal em cada transição
```

## Pré-requisitos

- Docker >= 24
- docker compose >= 2.x
- **FFmpeg já está incluído na imagem Docker** — não é necessário instalar localmente

Para desenvolvimento local com `go run`, você precisa de:
- Go 1.25+
- FFmpeg instalado no PATH do sistema

## Desenvolvimento Local

### 1. Clonar e copiar o exemplo de variáveis

```bash
git clone https://github.com/klawdyo/streamedia
cd streamedia
cp .env.example .env
```

### 2. Gerar os segredos obrigatórios

```bash
# Gera ROOT_TOKEN (credencial única de gestão)
echo "ROOT_TOKEN=$(openssl rand -hex 32)" >> .env

# Gera WEBHOOK_SECRET (assina os webhooks ao backend principal)
echo "WEBHOOK_SECRET=$(openssl rand -hex 32)" >> .env
```

Edite `.env` e defina também `WEBHOOK_URL` com o endereço do seu backend principal.

### 3. Subir com Docker Compose

```bash
docker compose up --build
```

O servidor estará disponível em `http://localhost:3000`.

### 4. Verificar saúde

```bash
curl http://localhost:3000/healthz
# {"status":"ok"}
```

## Variáveis de Ambiente

> As variáveis de tempo são expressas em **segundos** (o nome não carrega o
> sufixo de unidade; a descrição abaixo indica o valor).

| Variável | Obrigatória | Padrão | Descrição |
|---|---|---|---|
| `ROOT_TOKEN` | Sim | — | Credencial única de gestão. O backend principal a apresenta em `Authorization: Bearer <ROOT_TOKEN>` para iniciar uploads, emitir URLs de play, consultar status, listar e apagar. Gere com `openssl rand -hex 32`. Pode ser trocada a qualquer momento. |
| `WEBHOOK_SECRET` | Condic. | — | Obrigatório quando há webhooks (`WEBHOOK_URL` global ou `webhook_url` por vídeo). Assina (HMAC) os webhooks; único segredo compartilhado entre os dois lados, independente do destino. |
| `WEBHOOK_URL` | Não | — | URL **global** do backend principal que recebe os webhooks de evento. Pode ser sobrescrita por vídeo via o campo `webhook_url` no `POST /api/upload/init`. |
| `DISCORD_WEBHOOK_URL` | Não | — | Webhook do Discord para **alertas operacionais** internos (falha de transcode, fila cheia, transcode travado, falhas consecutivas). Vazio desabilita o canal. |
| `MAX_UPLOAD_SIZE_MB` | Não | `10` | Tamanho máximo de upload em MB. |
| `MEDIA_DIR` | Não | `/media` | Diretório raiz onde os arquivos HLS são armazenados (`<MEDIA_DIR>/<tag>/<video_id>/...`). |
| `UPLOAD_TMP_DIR` | Não | `/media/.uploads` | Diretório temporário para receber os uploads TUS. |
| `SQLITE_PATH` | Não | `/data/media.db` | Caminho do arquivo SQLite. |
| `QUEUE_MAX_SIZE` | Não | `50` | Capacidade máxima da fila de transcodificação. |
| `TRANSCODE_WORKERS` | Não | `1` | Número de workers paralelos de transcodificação. |
| `UPLOAD_TOKEN_TTL` | Não | `1200` | TTL do token de upload, em segundos (1200 = 20min). |
| `PLAY_TOKEN_TTL` | Não | `3600` | TTL do token de play emitido por `/api/play/init`, em segundos (3600 = 1h). |
| `UPLOAD_IDLE_TIMEOUT` | Não | `600` | Timeout de inatividade de upload, em segundos (600 = 10min). |
| `TRANSCODE_STUCK` | Não | `1800` | Timeout de transcodificação travada, em segundos (1800 = 30min). |
| `MAX_TRANSCODE_ATTEMPTS` | Não | `3` | Número máximo de tentativas de transcodificação por vídeo. |
| `KEEP_ORIGINAL` | Não | `false` | Se `true`, mantém o arquivo original após transcodificação. |
| `PORT` | Não | `3000` | Porta HTTP do servidor. |
| `RATE_LIMIT_PER_MIN` | Não | `60` | Limite de requisições por minuto por IP. |

## Deploy no Coolify

O `docker-compose.yml` usa a sintaxe `${VAR:-default}`, que o Coolify lê como variáveis de ambiente a serem configuradas no painel.

### Passo a passo

1. **Criar novo serviço** no Coolify → "Docker Compose" → apontar para este repositório.

2. **Configurar variáveis de ambiente** no painel do Coolify (aba "Environment Variables"). Para cada variável obrigatória, marque a opção **"Is Literal"** para que o Coolify não interprete o valor como referência:

   | Variável | Valor | Is Literal |
   |---|---|---|
   | `ROOT_TOKEN` | `<saída de openssl rand -hex 32>` | Sim |
   | `WEBHOOK_SECRET` | `<saída de openssl rand -hex 32>` | Sim |
   | `WEBHOOK_URL` | `https://seu-backend.com/webhooks/media` | Sim |
   | `DISCORD_WEBHOOK_URL` (opcional) | `https://discord.com/api/webhooks/...` | Sim |

3. **Adicionar volumes persistentes** no Coolify para que os dados sobrevivam a redeploys:
   - `media_files` → montado em `/media`
   - `db_data` → montado em `/data`

4. **Configurar domínio** e habilitar HTTPS no Coolify (necessário para o Flutter fazer uploads TUS por HTTPS).

5. **Fazer o deploy** e verificar o healthcheck: `GET /healthz` → `{"status":"ok"}`.

### Observação sobre variáveis com padrão

Variáveis como `MAX_UPLOAD_SIZE_MB`, `PORT`, etc. têm padrão definido no `docker-compose.yml` (`${PORT:-3000}`). Você só precisa defini-las no Coolify se quiser sobrescrever o padrão.

## Rotas da API

### POST /api/upload/init

Inicializa um upload. Chamada exclusivamente pelo **backend principal**
(server-to-server), autenticada com o `ROOT_TOKEN`.

**Autenticação:** header `Authorization: Bearer <ROOT_TOKEN>`.

O corpo informa a **tag** (namespace organizacional, normalizada para slug) e,
opcionalmente, o `video_id` (UUID; se omitido, o servidor gera um UUID v7). O
vídeo é gravado em `<MEDIA_DIR>/<tag>/<video_id>/...`.

**Requisição:**
```http
POST /api/upload/init
Content-Type: application/json
Authorization: Bearer <ROOT_TOKEN>

{
  "tag": "nome-da-tag",
  "video_id": "550e8400-e29b-41d4-a716-446655440000",
  "declared_size_bytes": 52428800
}
```

**Resposta 200 OK:**
```json
{
  "video_id": "550e8400-e29b-41d4-a716-446655440000",
  "tag": "nome-da-tag",
  "upload_url": "https://media.exemplo.com/files/550e8400-e29b-41d4-a716-446655440000",
  "token": "a3f1c9..."
}
```

**Erros possíveis:**
- `401 Unauthorized` — `ROOT_TOKEN` ausente ou inválido
- `400 Bad Request` — JSON inválido, `tag` ausente, ou `video_id` não é UUID
- `409 Conflict` — `video_id` já existe
- `413 Request Entity Too Large` — `declared_size_bytes` acima do limite

---

### TUS Upload — /files/{video_id}

Protocolo TUS v1 para upload resumível. O Flutter Client envia o arquivo diretamente para esta rota usando a biblioteca TUS.

**Autenticação:** header `Authorization: Bearer <token>` onde `<token>` é retornado pelo `/api/upload/init`.

| Método | Rota | Descrição |
|---|---|---|
| `POST` | `/files/` | Cria upload TUS (sem video_id, raramente usado) |
| `POST` | `/files/{video_id}` | Cria upload TUS para o video_id |
| `PATCH` | `/files/{video_id}` | Envia chunk de dados |
| `HEAD` | `/files/{video_id}` | Consulta offset atual (para retomada) |
| `DELETE` | `/files/{video_id}` | Cancela e remove o upload |

**Headers TUS obrigatórios:**
```http
Tus-Resumable: 1.0.0
Content-Type: application/offset+octet-stream
Upload-Offset: <offset-atual>
```

---

### POST /api/play/init

Emite uma URL de reprodução assinada. O backend principal (que já autorizou o
usuário) chama esta rota; o Streamedia gera um token de play efêmero (linha em
`access_tokens`, `purpose='play'`, TTL `PLAY_TOKEN_TTL`) e devolve a URL do
master playlist. Padrão típico: o backend devolve ao app uma URL própria
estável (ex. `backend/api/video/{id}/play`) e, no play, autoriza e responde
**302** (com `Cache-Control: no-store`) para a `play_url` retornada aqui.

**Autenticação:** header `Authorization: Bearer <ROOT_TOKEN>`.

**Requisição:**
```http
POST /api/play/init
Authorization: Bearer <ROOT_TOKEN>

{ "video_id": "550e8400-e29b-41d4-a716-446655440000" }
```

**Resposta 200 OK:**
```json
{
  "video_id": "550e8400-e29b-41d4-a716-446655440000",
  "tag": "nome-da-tag",
  "play_url": "https://media.exemplo.com/video/nome-da-tag/550e8400-e29b-41d4-a716-446655440000.m3u8?token=abc123",
  "token": "abc123",
  "expires_at": "2026-01-01T12:00:00Z"
}
```

---

### GET /video/{tag}/{video_id}.m3u8

Retorna o playlist HLS master (dinâmico). Requer o token de play na query
(validado por lookup no banco). O caminho real no disco
(`<MEDIA_DIR>/<tag>/<video_id>/master.m3u8`) fica escondido; o handler reescreve
as referências de variante para incluir o `video_id`. As playlists de resolução
e os segmentos `.ts` são servidos estaticamente em
`/video/{tag}/{video_id}/{resolução}/...` (públicos — os nomes opacos no master
funcionam como a "chave").

**Exemplo:**
```
GET /video/nome-da-tag/550e8400-e29b-41d4-a716-446655440000.m3u8?token=abc123
```

**Resposta 200 OK:** playlist HLS (Content-Type: `application/vnd.apple.mpegurl`)
```
#EXTM3U
#EXT-X-VERSION:3
#EXT-X-STREAM-INF:BANDWIDTH=800000,RESOLUTION=640x360
550e8400-e29b-41d4-a716-446655440000/360/playlist.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=1400000,RESOLUTION=1280x720
550e8400-e29b-41d4-a716-446655440000/720/playlist.m3u8
```

---

### GET /api/status/{video_id}

Consulta o status de um vídeo. Chamada pelo backend principal (server-to-server).

**Autenticação:** header `Authorization: Bearer <ROOT_TOKEN>`.

**Resposta 200 OK:**
```json
{
  "video_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "ready",
  "duration_s": 120,
  "resolutions": [360, 720],
  "declared_size_bytes": 52428800,
  "actual_size_bytes": 51200000
}
```

---

### GET /admin/videos

Lista os vídeos no banco.

**Autenticação:** header `Authorization: Bearer <ROOT_TOKEN>`. Aceita os query
params opcionais `status` e `tag` (filtra a listagem por namespace), além de
`limit`/`offset` para paginação.

**Resposta 200 OK:**
```json
[
  {
    "video_id": "550e8400-e29b-41d4-a716-446655440000",
    "status": "ready",
    "declared_size_bytes": 52428800,
    "actual_size_bytes": 51200000,
    "duration_s": 120,
    "resolutions": [360, 720],
    "transcode_attempts": 1,
    "created_at": "2024-01-01T10:00:00Z",
    "updated_at": "2024-01-01T10:05:00Z"
  }
]
```

---

### GET /admin/queue

Consulta o estado atual da fila de transcodificação. Requer `Authorization: Bearer <ROOT_TOKEN>`.

**Resposta 200 OK:**
```json
{
  "pending": 3,
  "capacity": 50
}
```

---

### DELETE /admin/videos/{video_id}

Apaga um vídeo: remove suas linhas no banco (tokens de acesso, variantes,
eventos e o próprio vídeo) e o diretório de arquivos no disco
(`<MEDIA_DIR>/<tag>/<video_id>`). Requer `Authorization: Bearer <ROOT_TOKEN>`.
`404` se o vídeo não existir.

**Resposta 200 OK:**
```json
{ "video_id": "550e8400-e29b-41d4-a716-446655440000", "deleted": "true" }
```

---

### GET /healthz

Healthcheck para Docker e load balancers. Não requer autenticação.

**Resposta 200 OK:**
```json
{"status":"ok"}
```

## Observabilidade

### GET /metrics

Expõe métricas operacionais no formato OpenTelemetry/Prometheus
(`text/plain; version=0.0.4`), prontas para `scrape` por um Prometheus
local ou qualquer ferramenta compatível. Não requer autenticação — é o
padrão do ecossistema Prometheus, que normalmente protege essa rota na
camada de infraestrutura/rede (ex. regra de firewall restringindo a origem
do scraper). O rate limiter da aplicação (ver seção de Rotas) também se
aplica a ela.

Métricas expostas:

- `streamedia_http_requests_total{method, route, status}` — contador de
  requisições HTTP, rotulado pelo template de rota do `chi` (evita
  explosão de séries por valores de path como UUIDs)
- `streamedia_http_request_duration_seconds{method, route, status}` —
  histograma de duração das requisições, em segundos
- `streamedia_transcode_queue_length` — gauge com o tamanho atual da fila
  de transcodificação (mesma fonte do `/admin/queue`)
- `streamedia_uploads_in_progress` — gauge com a quantidade de vídeos com
  upload em andamento (`pending_upload`/`uploading`)
- `streamedia_playback_events_total{event_type}` — gauge com o total
  acumulado de eventos de uso (`playback`, `download_segment`,
  `upload_complete`), lido sob demanda da tabela `playback_events` —
  mesma fonte de verdade usada pela rota `/admin/stats`

**Integrando com um Prometheus local:**

```yaml
# prometheus.yml
scrape_configs:
  - job_name: streamedia
    metrics_path: /metrics
    static_configs:
      - targets: ["localhost:8080"]
```

```bash
curl http://localhost:8080/metrics
```

### GET /admin/stats

Estatísticas de negócio (totais por tipo de evento, agregações por
resolução, sistema operacional e dia da semana), derivadas da tabela
`playback_events`. Formato JSON, voltado para consumo administrativo —
diferente de `/metrics` (formato Prometheus, consumo por ferramentas de
monitoramento). Requer `Authorization: Bearer <ROOT_TOKEN>`. Aceita o
parâmetro opcional `?video_id=` para filtrar as estatísticas de um vídeo
específico.

Além das estatísticas de **uso** acima, a resposta global (sem `?video_id=`)
inclui uma seção `storage` com estatísticas de **armazenamento e fila**
(issue #5):

```json
{
  "video_id": null,
  "totals": { "playback": 120, "download_segment": 980, "upload_complete": 12 },
  "by_resolution": { "480": 600, "720": 380 },
  "by_os": { "android": 700, "ios": 280 },
  "by_day_of_week": { "0": 50, "1": 70 },
  "storage": {
    "total_bytes": 123456789,
    "total_duration_seconds": 7384,
    "videos_by_status": { "pending_upload": 2, "transcoding": 1, "ready": 40, "failed_transcode": 1 },
    "queue_pending": 1
  }
}
```

- `total_bytes` — soma do tamanho dos arquivos originais
  (`videos.actual_size_bytes`) com o tamanho de todas as variantes HLS
  geradas (`video_renditions.size_bytes`).
- `total_duration_seconds` — soma da duração de todos os vídeos cadastrados
  (`videos.duration_s`).
- `videos_by_status` — contagem de vídeos agrupados por status
  (`pending_upload`, `uploading`, `transcoding`, `ready`,
  `failed_transcode`, ...).
- `queue_pending` — tamanho atual da fila de transcodificação; mesma fonte
  (`queue.Len()`) usada por `GET /admin/queue` e pelo gauge
  `streamedia_transcode_queue_length` (`/metrics`) — não é recomputado por
  outro caminho, garantindo consistência entre as três rotas.

A seção `storage` é uma visão **agregada global** — por isso é omitida
quando `?video_id=` é informado (não faria sentido, por exemplo, devolver
`queue_pending` "filtrado por vídeo"; isso poderia ser mal interpretado como
o tamanho da fila relativo àquele vídeo específico).

## Documentação interativa da API (Scalar)

A documentação interativa da API (OpenAPI + Scalar UI) faz parte do Admin
Unificado — uma SPA Vue 3 com Google OAuth, acessível em `GET /app/*`.
A especificação OpenAPI continua disponível para consumo programático.
Ver `.tasks/` (T75-T90) para o registro completo da implementação.

## Token de Reprodução

O Streamedia **emite** o token de reprodução (o backend não o assina mais). O
fluxo: o backend principal, após autorizar o usuário, chama
`POST /api/play/init` com o `ROOT_TOKEN`; o Streamedia gera um token aleatório,
persiste em `access_tokens` (`purpose='play'`, TTL `PLAY_TOKEN_TTL`) e devolve a
`play_url` assinada do master playlist. A validação na entrega do
`master.m3u8` é feita por **lookup no banco** (não há HMAC).

**Exemplo (backend principal, server-to-server):**
```go
// POST /api/play/init com Authorization: Bearer <ROOT_TOKEN>
//   body: {"video_id": "<id>"}
// resposta: {"play_url": "https://media.exemplo.com/video/<tag>/<id>.m3u8?token=...", ...}
```

Padrão recomendado: o backend devolve ao app uma URL própria estável
(`backend/api/video/{id}/play`) e, no momento do play, autoriza o usuário e
responde **302** (com `Cache-Control: no-store`) para a `play_url`. Assim o
contato com o Streamedia só acontece na reprodução, nunca ao listar a timeline.

## Webhook

O Streamedia envia webhooks ao `WEBHOOK_URL` a cada transição de estado significativa. Cada chamada inclui o header `X-Signature` com o HMAC-SHA256 do payload.

> **Destino por vídeo:** o `WEBHOOK_URL` é o destino *global*. Cada vídeo pode
> sobrescrevê-lo informando `webhook_url` (URL HTTPS válida, ≤ 2048 caracteres)
> no `POST /api/upload/init` — útil em cenários multi-tenant. A assinatura HMAC
> (`WEBHOOK_SECRET`) é a mesma, qualquer que seja o destino.
>
> **Alertas operacionais (Discord):** separadamente, defina `DISCORD_WEBHOOK_URL`
> para receber alertas internos (falha de transcode, fila cheia, transcode
> travado, falhas consecutivas). É opcional e independente dos webhooks de negócio.

### Verificação da Assinatura

```go
import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "strings"
)

// ValidateWebhookSignature valida a assinatura do webhook recebido.
func ValidateWebhookSignature(secret string, body []byte, signatureHeader string) bool {
    // O header tem o formato "sha256=<hex>"
    sig := strings.TrimPrefix(signatureHeader, "sha256=")

    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write(body)
    expected := hex.EncodeToString(mac.Sum(nil))

    // Comparação em tempo constante para evitar timing attacks
    expectedBytes, _ := hex.DecodeString(expected)
    sigBytes, _ := hex.DecodeString(sig)
    return hmac.Equal(expectedBytes, sigBytes)
}
```

### Evento: processing (transcodificação iniciada)

```json
{
  "video_id": "550e8400-e29b-41d4-a716-446655440000",
  "event": "processing",
  "status": "transcoding",
  "duration_s": null,
  "resolutions": [],
  "error_message": null,
  "timestamp": "2024-01-01T10:01:00Z"
}
```

### Evento: ready (transcodificação concluída)

```json
{
  "video_id": "550e8400-e29b-41d4-a716-446655440000",
  "event": "ready",
  "status": "ready",
  "duration_s": 120,
  "resolutions": [360, 720],
  "error_message": null,
  "timestamp": "2024-01-01T10:05:00Z"
}
```

### Evento: failed (falha na transcodificação)

```json
{
  "video_id": "550e8400-e29b-41d4-a716-446655440000",
  "event": "failed",
  "status": "failed_transcode",
  "duration_s": null,
  "resolutions": [],
  "error_message": "ffmpeg exit code 1: codec not found",
  "timestamp": "2024-01-01T10:05:30Z"
}
```

O Streamedia realiza até **3 tentativas** com backoff exponencial (1s, 2s, 4s) em caso de falha de entrega do webhook. Todas as tentativas são registradas na tabela `webhook_log`.

## Como Rodar os Testes

```bash
# Rodar todos os testes
go test ./...

# Com verbose
go test -v ./...

# Apenas um pacote
go test ./internal/upload/...

# Com cobertura
go test -cover ./...
```

Os testes não dependem de Docker ou FFmpeg — usam SQLite in-memory e mocks.

## Tabela de Status dos Vídeos

| Status | Descrição |
|---|---|
| `pending_upload` | Vídeo registrado, aguardando início do upload TUS |
| `uploading` | Upload TUS em andamento (chunks sendo recebidos) |
| `upload_complete` | Upload finalizado, aguardando entrada na fila de transcodificação |
| `transcoding` | FFmpeg em execução, gerando os arquivos HLS |
| `ready` | Transcodificação concluída, vídeo disponível para reprodução |
| `failed_upload` | Falha durante o upload (timeout de inatividade ou erro de validação) |
| `failed_transcode` | FFmpeg falhou após todas as tentativas (`MAX_TRANSCODE_ATTEMPTS`) |

### Máquina de Estados

```
pending_upload → uploading → upload_complete → transcoding → ready
     └──────────────────────────► failed_upload
                                  transcoding ──────────────► failed_transcode
```

Estados terminais: `ready`, `failed_upload`, `failed_transcode`.

## Troubleshooting

### "variável de ambiente ROOT_TOKEN é obrigatória"

O servidor não iniciou porque as variáveis obrigatórias não estão definidas. Verifique se o arquivo `.env` existe e foi carregado (`docker compose --env-file .env up`).

### Upload trava e não progride

- Verifique `UPLOAD_IDLE_TIMEOUT_SECONDS` — o job `killer` cancela uploads sem atividade.
- Verifique se o Flutter Client está enviando `Tus-Resumable: 1.0.0` e `Content-Length`.

### Vídeo fica em "transcoding" para sempre

- Verifique os logs do worker: `docker compose logs -f mediaserver`
- O job `recovery` recoloca na fila uploads travados por mais de `TRANSCODE_STUCK_SECONDS` segundos.
- Verifique se o FFmpeg está instalado na imagem: `docker compose exec mediaserver ffmpeg -version`

### Webhook não está sendo recebido

- Confirme que `WEBHOOK_URL` está correto e acessível pelo container.
- Consulte o painel admin: `GET /admin/videos` para ver o status do vídeo.
- Verifique o `webhook_log` no SQLite para histórico de tentativas.

### Rate limit está bloqueando uploads legítimos

Aumente `RATE_LIMIT_PER_MIN` no `.env`. O limite é por IP — uploads TUS de um mesmo cliente fazem muitas requisições PATCH.

### 409 Conflict no /api/upload/init

O `video_id` já existe no banco. O backend principal deve gerar um novo UUID v4 para cada upload.

## Convenção de Idioma

| Contexto | Idioma |
|---|---|
| Identificadores (variáveis, funções, structs, pacotes, nomes de arquivo) | Inglês |
| Comentários no código | Português |
| Mensagens de erro retornadas pela API | Português |
| Documentação (este README) | Português |

Esta convenção garante compatibilidade com bibliotecas e ferramentas Go (inglês) enquanto mantém o contexto do projeto acessível para a equipe (português).
