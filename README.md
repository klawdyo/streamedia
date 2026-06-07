# Streamedia — Servidor de Mídia

Serviço Go de upload, transcodificação e entrega de vídeo em HLS. Substitui o Bunny Stream em uma rede social de vídeo estilo Instagram.

## Papel na Arquitetura

O Streamedia atua como um serviço independente especializado no pipeline de vídeo:

```
Backend Principal ──POST /upload/init──► Streamedia
                                              │
Flutter Client ──TUS Upload──────────────────┘
                                              │
                                        FFmpeg (HLS)
                                              │
                                    S3 / Volume Local
                                              │
Streamedia ──Webhook──────────────► Backend Principal
                                              │
Flutter Client ◄── GET /videos/{id}/master.m3u8
```

## Fluxo de Upload

```
1. Backend Principal chama POST /upload/init (autenticado com HMAC-SHA256)
2. Streamedia registra o vídeo no banco (status: pending_upload)
3. Streamedia devolve { upload_url, token }
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
# Gera UPLOAD_TOKEN_SECRET
echo "UPLOAD_TOKEN_SECRET=$(openssl rand -hex 32)" >> .env

# Gera WEBHOOK_SECRET
echo "WEBHOOK_SECRET=$(openssl rand -hex 32)" >> .env

# Gera ADMIN_TOKEN
echo "ADMIN_TOKEN=$(openssl rand -hex 32)" >> .env
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

> **Mudança incompatível (issue #4):** todas as variáveis de tempo agora usam
> o sufixo `_SECONDS` e valores em segundos — antes misturavam horas
> (`UPLOAD_TOKEN_TTL_H`, `PLAY_TOKEN_MAX_TTL_H`) e minutos
> (`UPLOAD_IDLE_TIMEOUT_MIN`, `TRANSCODE_STUCK_MIN`). Quem atualiza uma
> instalação existente precisa renomear essas variáveis e converter os
> valores para segundos (ex. `UPLOAD_TOKEN_TTL_H=6` →
> `UPLOAD_TOKEN_TTL_SECONDS=21600`); os nomes antigos não são mais lidos.

| Variável | Obrigatória | Padrão | Descrição |
|---|---|---|---|
| `UPLOAD_TOKEN_SECRET` | Sim | — | Segredo HMAC-SHA256 para tokens de upload. Gere com `openssl rand -hex 32`. |
| `WEBHOOK_SECRET` | Sim | — | Segredo para assinar os webhooks enviados ao backend principal. |
| `WEBHOOK_URL` | Sim | — | URL do backend principal que recebe os webhooks de evento. |
| `ADMIN_TOKEN` | Não | `""` | Token estático para autenticar as rotas `/admin/*`. Se vazio, admin é desabilitado. |
| `MAX_UPLOAD_SIZE_MB` | Não | `10` | Tamanho máximo de upload em MB. |
| `MEDIA_DIR` | Não | `/media` | Diretório raiz onde os arquivos HLS são armazenados. |
| `UPLOAD_TMP_DIR` | Não | `/media/.uploads` | Diretório temporário para receber os uploads TUS. |
| `SQLITE_PATH` | Não | `/data/media.db` | Caminho do arquivo SQLite. |
| `QUEUE_MAX_SIZE` | Não | `50` | Capacidade máxima da fila de transcodificação. |
| `TRANSCODE_WORKERS` | Não | `1` | Número de workers paralelos de transcodificação. |
| `UPLOAD_TOKEN_TTL_SECONDS` | Não | `21600` | TTL do token de upload em segundos (21600 = 6h). |
| `UPLOAD_TOKEN_SCOPED_TTL_SECONDS` | Não | `1200` | TTL do token de upload escopado a projeto (`X-Project-Key`, issue #6/T33), em segundos — vida curta (1200 = 20min). |
| `PLAY_TOKEN_MAX_TTL_SECONDS` | Não | `21600` | TTL máximo do token de reprodução em segundos (21600 = 6h). |
| `UPLOAD_IDLE_TIMEOUT_SECONDS` | Não | `600` | Timeout de inatividade de upload em segundos (600 = 10min). |
| `TRANSCODE_STUCK_SECONDS` | Não | `1800` | Timeout de transcodificação travada em segundos (1800 = 30min). |
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
   | `UPLOAD_TOKEN_SECRET` | `<saída de openssl rand -hex 32>` | Sim |
   | `WEBHOOK_SECRET` | `<saída de openssl rand -hex 32>` | Sim |
   | `WEBHOOK_URL` | `https://seu-backend.com/webhooks/media` | Sim |
   | `ADMIN_TOKEN` | `<saída de openssl rand -hex 32>` | Sim |

3. **Adicionar volumes persistentes** no Coolify para que os dados sobrevivam a redeploys:
   - `media_files` → montado em `/media`
   - `db_data` → montado em `/data`

4. **Configurar domínio** e habilitar HTTPS no Coolify (necessário para o Flutter fazer uploads TUS por HTTPS).

5. **Fazer o deploy** e verificar o healthcheck: `GET /healthz` → `{"status":"ok"}`.

### Observação sobre variáveis com padrão

Variáveis como `MAX_UPLOAD_SIZE_MB`, `PORT`, etc. têm padrão definido no `docker-compose.yml` (`${PORT:-3000}`). Você só precisa defini-las no Coolify se quiser sobrescrever o padrão.

## Rotas da API

### POST /upload/init

Inicializa um upload. Chamada exclusivamente pelo **backend principal** (server-to-server)
ou por um **projeto interno** (issue #6/T33).

**Autenticação — dois fluxos coexistem:**

- **Escopado a projeto** (issue #6/T33): header `X-Project-Key: <chave mestra do projeto>`,
  obtida na criação do projeto. O servidor resolve o projeto pelo hash da chave —
  o vídeo e o token de upload gerados ficam vinculados àquele projeto, e o
  token tem TTL curto (`UPLOAD_TOKEN_SCOPED_TTL_SECONDS`, padrão 20min — "um
  único arquivo"). Tem prioridade sobre `X-Upload-Auth` se ambos vierem.
- **Legado/global**: header `X-Upload-Auth` com HMAC-SHA256 do corpo em hex,
  assinado com `UPLOAD_TOKEN_SECRET`. Continua funcionando sem mudanças —
  vídeo e token ficam sem projeto associado.

**Requisição (fluxo legado):**
```http
POST /upload/init
Content-Type: application/json
X-Upload-Auth: <hmac-sha256-hex>

{
  "video_id": "550e8400-e29b-41d4-a716-446655440000",
  "declared_size_bytes": 52428800
}
```

**Requisição (escopada a projeto):**
```http
POST /upload/init
Content-Type: application/json
X-Project-Key: <chave mestra do projeto>

{
  "video_id": "550e8400-e29b-41d4-a716-446655440000",
  "declared_size_bytes": 52428800
}
```

**Resposta 200 OK:**
```json
{
  "upload_url": "https://media.exemplo.com/files/550e8400-e29b-41d4-a716-446655440000",
  "token": "a3f1c9..."
}
```

**Erros possíveis:**
- `401 Unauthorized` — assinatura HMAC inválida
- `400 Bad Request` — JSON inválido ou `video_id` não é UUID v4
- `409 Conflict` — `video_id` já existe
- `413 Request Entity Too Large` — `declared_size_bytes` acima do limite

---

### TUS Upload — /files/{video_id}

Protocolo TUS v1 para upload resumível. O Flutter Client envia o arquivo diretamente para esta rota usando a biblioteca TUS.

**Autenticação:** header `Authorization: Bearer <token>` onde `<token>` é retornado pelo `/upload/init`.

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

### GET /videos/{video_id}/master.m3u8

Retorna o playlist HLS master. Requer token de reprodução assinado.

**Query params:**
- `token` — token HMAC-SHA256 gerado pelo backend principal
- `expires` — timestamp Unix de expiração

**Exemplo:**
```
GET /videos/550e8400-e29b-41d4-a716-446655440000/master.m3u8?token=abc123&expires=1735689600
```

**Resposta 200 OK:** playlist HLS (Content-Type: `application/vnd.apple.mpegurl`)
```
#EXTM3U
#EXT-X-VERSION:3
#EXT-X-STREAM-INF:BANDWIDTH=800000,RESOLUTION=640x360
360/playlist.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=1400000,RESOLUTION=1280x720
720/playlist.m3u8
```

---

### GET /api/status/{video_id}

Consulta o status de um vídeo. Chamada pelo backend principal (server-to-server).

**Autenticação:** header `X-Upload-Auth` com HMAC-SHA256 do body vazio.

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

**Autenticação — `Authorization: Bearer <token>`, dois níveis (issue #6/T33):**

- `<token>` = `ADMIN_TOKEN` global → **super-admin**, sem restrição: enxerga e
  opera sobre os vídeos de todos os projetos (comportamento original, preservado).
- `<token>` = chave mestra de um projeto → **admin de projeto**: as rotas
  `/admin/*` ficam restritas aos vídeos daquele projeto (`project_id`); nunca
  enxerga ou opera sobre vídeos de outro projeto.

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

Consulta o estado atual da fila de transcodificação. Requer `Authorization: Bearer <ADMIN_TOKEN>`.

**Resposta 200 OK:**
```json
{
  "pending": 3,
  "capacity": 50
}
```

---

### POST /admin/projects

Cria um novo "projeto interno" — um namespace com diretório de armazenamento e
chave mestra próprios (issue #6, T32/T35). Operação sensível: cria os próprios
projetos e suas chaves mestras, então **exige `Authorization: Bearer <ADMIN_TOKEN>`
global** — uma chave mestra de projeto não autentica aqui (403 Forbidden).

**Corpo:**
```json
{ "name": "Trip Produção" }
```

**Resposta 201 Created:**
```json
{
  "id": 3,
  "name": "Trip Produção",
  "slug": "trip-producao",
  "root_dir": "trip-producao",
  "master_key": "f3a1...64 chars hex..."
}
```

> ⚠️ `master_key` é devolvida **em texto puro apenas nesta resposta** —
> o servidor persiste somente seu hash (SHA-256) e nunca a recupera depois.
> Guarde-a com segurança: ela autentica `/upload/init`, as rotas `/admin/*`
> escopadas a este projeto, e a emissão de tokens abaixo.

---

### GET /admin/projects

Lista todos os projetos cadastrados (sem expor `master_key`/hash). Requer
`Authorization: Bearer <ADMIN_TOKEN>` global — mesma restrição de super-admin
do endpoint de criação.

**Resposta 200 OK:**
```json
{
  "projects": [
    { "id": 1, "name": "Legacy", "slug": "legacy", "root_dir": "legacy", "created_at": "2024-01-01T00:00:00Z" },
    { "id": 3, "name": "Trip Produção", "slug": "trip-producao", "root_dir": "trip-producao", "created_at": "2024-02-01T12:00:00Z" }
  ],
  "total": 2
}
```

---

### GET /admin/projects/{slug}

Detalhe de um projeto pelo slug (sem expor `master_key`/hash). Requer
`Authorization: Bearer <ADMIN_TOKEN>` global. `404` se o slug não existir.

**Resposta 200 OK:**
```json
{ "id": 3, "name": "Trip Produção", "slug": "trip-producao", "root_dir": "trip-producao", "created_at": "2024-02-01T12:00:00Z" }
```

---

### POST /admin/projects/{slug}/upload-tokens

Troca a chave mestra de um projeto por um token de upload de curta duração
para um `video_id` **gerado pelo servidor** (issue #6, T35) — o equivalente a
`POST /upload/init` no fluxo escopado a projeto (T33), só que sem o cliente
precisar gerar o UUID do vídeo previamente.

**Autenticação — diferente das demais rotas `/admin/projects*`:** não usa
`Authorization: Bearer`/`ADMIN_TOKEN`. Em vez disso, exige a **própria chave
mestra do projeto** no header `X-Project-Key` (mesmo princípio de
`/upload/init`: o servidor calcula o hash e resolve o projeto, nunca retém a
chave em texto puro). O `{slug}` no path precisa corresponder ao projeto
resolvido pela chave — caso contrário, `403 Forbidden`.

**Corpo (opcional):**
```json
{ "declared_size_bytes": 52428800 }
```

**Resposta 201 Created:**
```json
{
  "video_id": "550e8400-e29b-41d4-a716-446655440000",
  "upload_url": "https://media.example.com/files/550e8400-e29b-41d4-a716-446655440000",
  "token": "5e8f...",
  "expires_at": "2024-01-01T10:20:00Z"
}
```

O `token` expira em `UPLOAD_TOKEN_SCOPED_TTL_SECONDS` (padrão 1200s = 20min,
T33) e é assinado com a própria chave mestra do projeto — validado da mesma
forma que tokens emitidos por `/upload/init`.

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
monitoramento). Requer `Authorization: Bearer <ADMIN_TOKEN>`. Aceita o
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

## Documentação interativa da API (Swagger)

A API tem documentação interativa no padrão OpenAPI/Swagger, acessível pelo
navegador:

- `GET /docs/` — UI interativa do Swagger (carrega os assets do
  [Swagger UI](https://github.com/swagger-api/swagger-ui) via CDN)
- `GET /docs/openapi.json` — especificação OpenAPI 3.0 em JSON, consumida
  pela UI acima e por outras ferramentas (geração de clients, importação no
  Postman/Insomnia, etc.)

```bash
# abrir no navegador
xdg-open http://localhost:8080/docs/

# ou consultar a spec bruta
curl http://localhost:8080/docs/openapi.json | jq .
```

Não requer autenticação — mesma decisão tomada para `/metrics`: é material
de referência sobre os contratos da API (inclusive das rotas
administrativas, que continuam protegidas por `ADMIN_TOKEN`/HMAC nas rotas
reais; a spec apenas as descreve). O rate limiter da aplicação também se
aplica a essas rotas.

## Token de Reprodução

O backend principal deve gerar o token de reprodução antes de passar a URL do vídeo ao Flutter Client. O token é um HMAC-SHA256 sobre `"{video_id}:{expires_unix}"`.

**Exemplo em Go:**
```go
import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "time"
)

// GeneratePlayToken gera o token assinado para reprodução.
// Deve ser chamado pelo backend principal — nunca pelo cliente Flutter.
func GeneratePlayToken(secret, videoID string, expiresUnix int64) string {
    payload := fmt.Sprintf("%s:%d", videoID, expiresUnix)
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write([]byte(payload))
    return hex.EncodeToString(mac.Sum(nil))
}

// Exemplo de uso:
expires := time.Now().Add(2 * time.Hour).Unix()
token := GeneratePlayToken(os.Getenv("UPLOAD_TOKEN_SECRET"), videoID, expires)

playURL := fmt.Sprintf(
    "https://media.exemplo.com/videos/%s/master.m3u8?token=%s&expires=%d",
    videoID, token, expires,
)
```

O TTL máximo do token é controlado por `PLAY_TOKEN_MAX_TTL_SECONDS` (padrão: 21600 segundos = 6 horas). Tokens com expiração além desse limite são rejeitados.

## Webhook

O Streamedia envia webhooks ao `WEBHOOK_URL` a cada transição de estado significativa. Cada chamada inclui o header `X-Signature` com o HMAC-SHA256 do payload.

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

### "variável de ambiente UPLOAD_TOKEN_SECRET é obrigatória"

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

### 409 Conflict no /upload/init

O `video_id` já existe no banco. O backend principal deve gerar um novo UUID v4 para cada upload.

## Convenção de Idioma

| Contexto | Idioma |
|---|---|
| Identificadores (variáveis, funções, structs, pacotes, nomes de arquivo) | Inglês |
| Comentários no código | Português |
| Mensagens de erro retornadas pela API | Português |
| Documentação (este README) | Português |

Esta convenção garante compatibilidade com bibliotecas e ferramentas Go (inglês) enquanto mantém o contexto do projeto acessível para a equipe (português).
