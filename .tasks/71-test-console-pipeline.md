# T71 — Console de teste interativo do pipeline completo

**Status:** done
**Origem:** issue #18.
**Depende de:** fluxo atual (T69) — upload/init, TUS, play/init, serving HLS.

## Objetivo

Uma página web autocontida (single-file, sem build step) que exercita todo o
ciclo de vida de um vídeo no Streamedia na mesma tela — autenticação → upload →
processamento → reprodução — com feedback visual de cada etapa, para facilitar
o desenvolvimento e a demonstração.

## Escopo

- Rota `GET /playground` — página HTML autocontida.
- Fluxo sequencial (cada etapa habilita após a anterior):
  1. Autenticação: campo para colar o `ROOT_TOKEN`.
  2. `POST /api/upload/init`: campo `tag` + botão; exibe resposta/headers/timing.
  3. Upload chunked via TUS: divide o arquivo em chunks (default 5 MB), barra de
     progresso por chunk + barra unificada; usa `Upload-Token`.
  4. Status (polling de `GET /api/status`) + `POST /api/play/init`.
  5. Players HLS por resolução (480/720/1080), com ▶ Play individual (mede o
     tempo até começar a rodar; não autoplay).
  6. Receptor de webhooks de teste (`POST /playground/webhook`) + exibição via polling.
- UI inspirada no Scalar (tema escuro, mono, cards), syntax highlighting de
  JSON, headers HTTP visíveis, estado de loading + tempo de carregamento.

## Fora de escopo

- Autenticação além do `ROOT_TOKEN` colado manualmente.
- Persistência de estado entre recarregamentos.
- Uploads simultâneos na mesma página.

## Definition of Done

- [x] `GET /playground` serve a página autocontida (HTML embutido via `go:embed`).
- [x] Fluxo sequencial com habilitação progressiva das etapas.
- [x] Upload TUS em chunks com progresso por chunk + unificado.
- [x] Players por resolução com ▶ Play individual e medição de tempo.
- [x] Receptor de webhooks local (`/playground/webhook` + `/playground/webhook/events`).
- [x] Visual Scalar-like, JSON destacado, headers e timing visíveis.
- [x] Testes do pacote `playground` (página, receptor, polling, eviction).

## Resolução

Criado o pacote `internal/playground`:

- `playground.go` — `Handler` com `ServeUI` (`GET /playground`, HTML embutido via
  `go:embed index.html`), `ReceiveWebhook` (`POST /playground/webhook`, buffer em
  memória protegido por mutex, ring buffer de 50 entradas) e `ListEvents`
  (`GET /playground/webhook/events?since=N`, polling incremental por número de
  sequência).
- `index.html` — página única com CSS+JS inline. Cliente TUS implementado em
  JS puro: `POST /files/{id}` para criar (`Upload-Length` = tamanho real) e
  `PATCH` por chunk via `XMLHttpRequest` (para expor `upload.onprogress` por
  chunk). Players usam `hls.js` via CDN (HLS nativo no Safari como fallback),
  carregando a playlist pública de cada resolução. Cada requisição é
  instrumentada (`performance.now()`) para exibir status/timing.
- `playground_test.go` — cobre a página servida, o ciclo receber→listar do webhook,
  o polling incremental `?since=` e a eviction do buffer.

Rotas registradas em `internal/server/server.go` **fora** do `RootAuth`: a
página só age com o token colado pelo usuário, e o receptor precisa aceitar
POSTs do próprio Streamedia (assinados por HMAC, não por Bearer).

### Endurecimento do upload (timeout / cancelar / retry-resume)

Refinamento posterior do cliente TUS no `index.html` (reportado em uso real:
upload travado por minutos sem como recuperar):

- **Timeout por chunk** — o `XMLHttpRequest` do `PATCH` agora define
  `xhr.timeout` (campo "timeout/chunk (s)", default 30s). Antes o default era
  `0` = infinito, então uma conexão travada pendurava para sempre. O `POST` de
  criação e o `HEAD` de offset usam `AbortController` com o mesmo timeout.
- **Cancelar** — botão que aborta o chunk em voo (`xhr.abort()`) e marca
  cancelamento, sem congelar o fluxo.
- **Retry / Continuar** — o upload virou uma máquina resumível: ao falhar
  (timeout/erro/cancelamento), o botão consulta o offset REAL no servidor via
  `HEAD` (TUS) e retoma o laço a partir dele, reconstruindo as barras de
  progresso. Erros são tipados (`timeout`/`abort`/`network`/`http`) para
  decidir a mensagem e o próximo estado.

### Correção do `declared_size_bytes` (etapa 2 antes da 3)

A versão inicial enviava `declared_size_bytes` de um campo manual (default
10MB) na etapa 2, **antes** de o arquivo ser escolhido na etapa 3. Como o
servidor valida no post-finish que `actual == declared` com igualdade exata
(`internal/upload/validation.go:validateFileSize`), qualquer upload real
terminava em `failed_upload` (bytes recebidos ≠ 10MB declarados), com o
arquivo apagado. Correção: o seletor de arquivo passou para a etapa 2; o
`Solicitar` só habilita após escolher o arquivo e envia
`declared_size_bytes = file.size` (o campo virou read-only, preenchido
automaticamente). A etapa 3 ficou só com o envio em chunks.

### Migração para SSE + camada de notificações

A etapa 6 (que antes era um receptor de webhooks em memória com polling) virou
**eventos ao vivo via SSE**, junto com uma generalização da estratégia de
webhooks para "notificações":

- **`internal/notify`** — `Notification` (payload canônico), `Sink` (destino) e
  `Notifier` que busca o vídeo e faz fan-out (goroutine por sink) dos eventos.
  `Notify(videoID, event, errMsg)` é substituto direto do antigo `sendWebhook`.
- **`internal/webhook`** — `Client` virou um `Sink`; a URL é resolvida por
  notificação (`resolveURL`, hoje a global; pronta para URL por vídeo). Sem
  URL, não envia. `WebhookPayload` é alias de `notify.Notification`.
- **`internal/sse`** — `Hub` (indexado por video_id, também um `Sink`) +
  handler de `GET /api/events?video_id=&token=`, autenticado pelo **token de
  upload** (EventSource não envia header → token na query). Sem buffer/replay.
- **config** — `WEBHOOK_URL` virou opcional; `WEBHOOK_SECRET` obrigatório só
  quando a URL está definida.
- **playground** — `index.html` troca o polling por um `EventSource` aberto
  após a etapa 2; `playground.go` perde o receptor (só `ServeUI`); rotas
  `/playground/webhook*` removidas.
- Forward-compat: `resolveURL` deixa a issue de URL-por-vídeo a um passo; o
  `Notification` é extensível para ganhar `thumbnails` depois.

### Decisão de escopo — webhook não é "injetado" em `/upload/init`

A issue menciona "A URL é automaticamente injetada no upload/init". O fluxo
atual não tem override de webhook por requisição (o `WEBHOOK_URL` é global, na
config). Adicionar override por upload exigiria coluna no banco + threading no
cliente de webhook — invasivo para uma ferramenta de teste e em tensão com o
"sem persistência" da própria issue. Optou-se pelo receptor local em memória;
a página exibe a URL `<origin>/playground/webhook` pronta para copiar e instrui a
apontar `WEBHOOK_URL` para ela. Self-contained e sem tocar no fluxo de produção.

Arquivos: `internal/playground/{playground.go,index.html,playground_test.go}` (novos),
`internal/server/server.go` (import + 3 rotas), `spec/api.md`, `api.http`.
