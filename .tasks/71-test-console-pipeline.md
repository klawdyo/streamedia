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

- Rota `GET /ui` — página HTML autocontida.
- Fluxo sequencial (cada etapa habilita após a anterior):
  1. Autenticação: campo para colar o `ROOT_TOKEN`.
  2. `POST /api/upload/init`: campo `tag` + botão; exibe resposta/headers/timing.
  3. Upload chunked via TUS: divide o arquivo em chunks (default 5 MB), barra de
     progresso por chunk + barra unificada; usa `Upload-Token`.
  4. Status (polling de `GET /api/status`) + `POST /api/play/init`.
  5. Players HLS por resolução (480/720/1080), com ▶ Play individual (mede o
     tempo até começar a rodar; não autoplay).
  6. Receptor de webhooks de teste (`POST /ui/webhook`) + exibição via polling.
- UI inspirada no Scalar (tema escuro, mono, cards), syntax highlighting de
  JSON, headers HTTP visíveis, estado de loading + tempo de carregamento.

## Fora de escopo

- Autenticação além do `ROOT_TOKEN` colado manualmente.
- Persistência de estado entre recarregamentos.
- Uploads simultâneos na mesma página.

## Definition of Done

- [x] `GET /ui` serve a página autocontida (HTML embutido via `go:embed`).
- [x] Fluxo sequencial com habilitação progressiva das etapas.
- [x] Upload TUS em chunks com progresso por chunk + unificado.
- [x] Players por resolução com ▶ Play individual e medição de tempo.
- [x] Receptor de webhooks local (`/ui/webhook` + `/ui/webhook/events`).
- [x] Visual Scalar-like, JSON destacado, headers e timing visíveis.
- [x] Testes do pacote `ui` (página, receptor, polling, eviction).

## Resolução

Criado o pacote `internal/ui`:

- `ui.go` — `Handler` com `ServeUI` (`GET /ui`, HTML embutido via
  `go:embed index.html`), `ReceiveWebhook` (`POST /ui/webhook`, buffer em
  memória protegido por mutex, ring buffer de 50 entradas) e `ListEvents`
  (`GET /ui/webhook/events?since=N`, polling incremental por número de
  sequência).
- `index.html` — página única com CSS+JS inline. Cliente TUS implementado em
  JS puro: `POST /files/{id}` para criar (`Upload-Length` = tamanho real) e
  `PATCH` por chunk via `XMLHttpRequest` (para expor `upload.onprogress` por
  chunk). Players usam `hls.js` via CDN (HLS nativo no Safari como fallback),
  carregando a playlist pública de cada resolução. Cada requisição é
  instrumentada (`performance.now()`) para exibir status/timing.
- `ui_test.go` — cobre a página servida, o ciclo receber→listar do webhook,
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

### Decisão de escopo — webhook não é "injetado" em `/upload/init`

A issue menciona "A URL é automaticamente injetada no upload/init". O fluxo
atual não tem override de webhook por requisição (o `WEBHOOK_URL` é global, na
config). Adicionar override por upload exigiria coluna no banco + threading no
cliente de webhook — invasivo para uma ferramenta de teste e em tensão com o
"sem persistência" da própria issue. Optou-se pelo receptor local em memória;
a página exibe a URL `<origin>/ui/webhook` pronta para copiar e instrui a
apontar `WEBHOOK_URL` para ela. Self-contained e sem tocar no fluxo de produção.

Arquivos: `internal/ui/{ui.go,index.html,ui_test.go}` (novos),
`internal/server/server.go` (import + 3 rotas), `spec/api.md`, `api.http`.
