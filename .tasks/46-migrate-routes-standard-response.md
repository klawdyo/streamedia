# T46: Migrar todas as rotas para o envelope padrão + testes de conformidade

**Status:** pending
**Dependências:** T45 (cria o pacote `apiresponse` e o middleware de recovery)
**Estimativa:** grande
**Origem:** Issue #9 — "Padronização das respostas" — segunda metade da
implementação (T45 cria a fundação, esta tarefa migra e testa)

## Contexto

A T45 cria a fundação (`apiresponse.Success`/`apiresponse.Error`/middleware
de recovery) e documenta o padrão na spec. Esta tarefa faz o trabalho de
**migrar TODAS as rotas existentes** para usar essa fundação — eliminando
de vez as implementações duplicadas — e escreve a suíte de testes que
**garante que nenhuma rota escapa do padrão**, hoje e no futuro (é a parte
do pedido do usuário: "criar testes pra garantir que nenhuma rota tá com
erro, mesmo as exceções não tratadas devem mostrar o código e a mensagem").

Releia a seção "Escopo: quais respostas seguem o envelope, quais não" em
`.tasks/45-standard-response-envelope.md` — ela é normativa para esta
tarefa também. Resumo: **todo erro JSON de qualquer rota** + **todo
sucesso das rotas de API estruturada** seguem o envelope; conteúdo
binário/streaming (HLS, métricas, docs UI/spec) não.

## Inventário de pontos a migrar (mapeados nesta auditoria — confirme e
## complete durante a implementação, código pode ter mudado)

Implementações de erro a remover, substituindo por `apiresponse.Error`:
- `internal/serve/serve.go:45` — `respondError` (usado também por
  `internal/serve/status.go`)
- `internal/upload/init.go:168` — `respondError`
- `internal/admin/projects.go:25` — `respondJSONError`
- `internal/admin/stats.go:151` — `respondIfVideoMissing` (avalie: pode
  encapsular `apiresponse.Error` por dentro, ou ser substituída
  diretamente — decida e documente)

Respostas de sucesso a migrar para `apiresponse.Success`:
- `internal/upload/init.go` — resposta de `POST /upload/init`
  (`upload_url`, `token`, e `video_id` se a T44 já tiver sido aplicada)
- `internal/serve/status.go` — resposta de `GET /api/status/{id}`
- `internal/admin/admin.go` — `HandleVideos`, `HandleQueue`, `HandleStats`
  (e o que mais existir)
- `internal/admin/stats.go` — agregações de `/admin/stats`
- `internal/admin/projects.go` — `HandleCreateProject`,
  `HandleListProjects`, `HandleGetProject`, `HandleIssueUploadToken`
- `internal/server/server.go` — handler de `/healthz` (hoje escreve
  `{"status":"ok"}` cru)

Pontos onde o código do projeto responde diretamente sem corpo JSON
(devem passar a usar `apiresponse.Error` quando é o PRÓPRIO projeto
respondendo, antes de delegar ao tusd):
- `internal/upload/tus.go:115,127,135,145` — respostas de autenticação/
  limite (`401`, `403`, `413`) que hoje são só `w.WriteHeader(status)`
  sem corpo. Cuidado: não altere os pontos em que quem responde é o
  `tusd` internamente (esses seguem o protocolo TUS, não a API JSON do
  projeto) — só os pontos onde o HANDLER DO PROJETO intercepta antes.

Conteúdo que **NÃO** muda (replicar aqui a lista da T45 para referência
rápida — não migrar):
- `master.m3u8` / `playlist.m3u8` / segmentos HLS (`internal/serve`)
- `/metrics` (OpenTelemetry/Prometheus)
- `/docs/` (HTML) e `/docs/openapi.json` (documento de spec)
- Protocolo TUS em si (delegado ao `tusd`)

## QA Instructions

### 1. Testes de conformidade — a parte mais importante do pedido

Crie `internal/server/response_conformance_test.go` (suíte de integração
leve, no padrão dos testes de `internal/server/server_test.go` — usa
`httptest.NewServer` com o servidor montado):

```
TestAllJSONRoutes_ErrorResponses_FollowEnvelope
  - tabela com TODAS as rotas JSON da API e um cenário de erro garantido
    para cada uma (ex.: requisição sem auth, video_id inválido, payload
    malformado, recurso inexistente — uma por rota)
  - para cada cenário, decodifica o corpo como apiresponse.Envelope e
    verifica:
      error == true
      message != "" (não vazia)
      data == nil
      status_code == status HTTP da resposta (igual ao header)
  - cobre pelo menos: /upload/init, /api/status/{id}, /admin/videos,
    /admin/queue, /admin/stats, /admin/projects (e subrotas),
    /admin/projects/{slug}/upload-tokens, e os pontos do handler TUS que
    respondem antes de delegar ao tusd

TestAllJSONRoutes_SuccessResponses_FollowEnvelope
  - tabela com cenários de sucesso para as mesmas rotas (setup mínimo:
    inserir vídeo/projeto/token conforme necessário)
  - decodifica como apiresponse.Envelope e verifica:
      error == false
      message == "ok"
      data != nil (quando a rota retorna payload) ou == nil (quando não)
      status_code == status HTTP da resposta

TestUnhandledPanic_ReturnsStandardErrorEnvelope
  - registra uma rota de teste que dá panic deliberadamente (ou usa um
    ponto do sistema real que possa ser forçado a panicar de forma
    controlada em teste)
  - faz a requisição through o servidor montado (com o middleware de
    recovery aplicado)
  - resposta é 500, no envelope padrão, com mensagem genérica em
    português (NÃO o texto original do panic)
  - confirma que o servidor continua respondendo normalmente depois
    (panic não derrubou o processo nem deixou estado inconsistente)

TestNonAPIRoutes_NotForcedIntoEnvelope
  - confirma que master.m3u8/segmentos, /metrics, /docs/* continuam
    respondendo no formato original (não quebrou nada migrando rotas
    vizinhas) — teste de regressão, não de conformidade
```

Esta suíte é o "pente fino" que garante, de forma automatizada e
permanente, que nenhuma rota — atual ou futura — escapa do padrão. Ela
deve rodar no CI normal (`go test ./...`).

### 2. Atualize os testes existentes que verificam o formato antigo

Arquivos como `internal/upload/init_test.go`, `internal/serve/serve_test.go`,
`internal/serve/status_test.go`, `internal/admin/*_test.go` provavelmente
fazem asserções sobre o corpo `{"error": "..."}` no formato antigo — ajuste
para o novo envelope. Não delete cobertura, só adapte ao novo formato.

## Dev Instructions

1. Para cada arquivo do inventário acima, substitua a chamada/implementação
   local por `apiresponse.Success(...)` / `apiresponse.Error(...)`.
2. Remova as funções `respondError`/`respondJSONError`/equivalentes que
   ficarem sem uso após a migração — não deixe código morto.
3. Garanta que `data` em sucesso contenha exatamente o payload que a rota
   já retornava (não mude o conteúdo de negócio, só o envelope em volta).
4. Para `tus.go`: adicione corpo JSON no envelope nos pontos onde o
   handler do projeto intercepta e responde (linhas mapeadas acima),
   preservando os headers TUS já presentes.
5. Rode `go test ./... -v` e confirme que TUDO passa, incluindo os testes
   de conformidade novos e os testes antigos adaptados.
6. Rode `go vet ./...` sem warnings.
7. Faça uma checagem manual final: `grep -rn "json.NewEncoder(w).Encode\|fmt.Fprintf(w, \`{" internal/ --include="*.go" | grep -v _test` —
   qualquer resultado fora de `internal/apiresponse` ou das rotas
   explicitamente fora do escopo (HLS, métricas, docs) é uma migração
   esquecida.

## Arquivos a editar

- `internal/serve/serve.go`, `internal/serve/status.go`
- `internal/upload/init.go`, `internal/upload/tus.go`
- `internal/admin/admin.go`, `internal/admin/stats.go`,
  `internal/admin/projects.go`
- `internal/server/server.go` (rota `/healthz`)
- Testes correspondentes a cada um dos acima
- Novo: `internal/server/response_conformance_test.go`

## Definition of Done

- [x] Nenhuma implementação local de resposta de erro/sucesso JSON
      sobrevive fora de `internal/apiresponse` (verificado via grep, ver
      passo 7 das Dev Instructions)
- [x] Toda rota de API JSON (sucesso e erro) responde no envelope
      `{error, message, data, status_code}`
- [x] `data` é `null` explícito em erros e em sucessos sem payload
- [x] Panics não tratados retornam `500` no envelope, com mensagem
      genérica, sem vazar detalhes internos
- [x] Rotas fora do escopo (HLS, `/metrics`, `/docs/*`, protocolo TUS)
      continuam funcionando sem alteração de formato
- [x] Suíte `response_conformance_test.go` cobre todas as rotas JSON da
      API (sucesso, erro e panic) e roda no `go test ./...`
- [x] Testes antigos que verificavam o formato anterior foram adaptados
      (não removidos sem necessidade)
- [x] `go test ./...` e `go vet ./...` passam sem erros/regressões

## Resolução

### Arquivos alterados (código de produção)

- `internal/serve/serve.go` — Removida `respondError`, substituída por `apiresponse.Error` em ~25 call sites. Import `encoding/json` removido.
- `internal/serve/status.go` — Removida `respondError` (herdada do pacote serve), substituída por `apiresponse.Error` em ~6 call sites. Sucesso migrado para `apiresponse.Success`. Import `encoding/json` removido.
- `internal/upload/init.go` — Removida `respondError` (que usava `fmt.Fprintf` frágil), substituída por `apiresponse.Error` em ~13 call sites. Sucesso migrado para `apiresponse.Success`. Import `fmt` mantido (ainda usado em `fmt.Sprintf`).
- `internal/upload/tus.go` — ServeHTTP: 4 raw `w.Write([]byte(...))` trocados por `apiresponse.Error`. preCreate: 5 `tusd.HTTPResponse{Body: "..."}` raw strings trocadas por `tusErrorBody()` (helper que serializa `apiresponse.Envelope` para string). `jsonContentType` atualizado para incluir `charset=utf-8`. Import `encoding/json` e `apiresponse` adicionados.
- `internal/admin/admin.go` — `http.Error` (7 call sites) trocado por `apiresponse.Error`. `HandleVideos`/`HandleQueue` sucesso migrado para `apiresponse.Success`. Import `apiresponse` adicionado.
- `internal/admin/stats.go` — `http.Error` (5 call sites) trocado por `apiresponse.Error`. `respondIfVideoMissing` migrado. Sucesso migrado para `apiresponse.Success`. Import `encoding/json` removido.
- `internal/admin/projects.go` — Removida `respondJSONError`, substituída por `apiresponse.Error` em ~16 call sites. Todos os 4 handlers de sucesso migrados para `apiresponse.Success`.
- `internal/server/server.go` — `/healthz` trocado de raw `w.Write` para `apiresponse.Success`. Import `apiresponse` adicionado.
- `internal/middleware/ratelimit.go` — Resposta de rate limit excedido migrada para `apiresponse.Error` (mensagem em português conforme convenção). `Retry-After` setado antes de `apiresponse.Error` (que chama `WriteHeader`).

### Funções removidas (código morto)

- `internal/upload/init.go:respondError` — usava `fmt.Fprintf` frágil
- `internal/serve/serve.go:respondError` — duplicata byte-a-byte com `admin.respondJSONError`
- `internal/admin/projects.go:respondJSONError` — idêntica a `serve.respondError` mas com nome diferente

### Arquivos de teste atualizados

- `internal/admin/admin_test.go` — 11 decode sites adaptados para envelope; Content-Type assertions atualizadas
- `internal/admin/stats_test.go` — 5 decode sites adaptados
- `internal/admin/projects_test.go` — 4 decode sites adaptados
- `internal/admin/project_scope_test.go` — 1 decode site adaptado
- `internal/upload/init_test.go` — 2 decode sites adaptados
- `internal/upload/project_scope_test.go` — 1 decode site adaptado
- `internal/serve/status_test.go` — 7 decode sites adaptados
- `internal/integration/integration_test.go` — 4 decode sites adaptados
- `internal/server/server_test.go` — 1 decode site adaptado
- `internal/middleware/ratelimit_test.go` — `TestRateLimit_ResponseJSON` adaptada para envelope

### Novo arquivo

- `internal/server/response_conformance_test.go` — 4 suítes de conformidade:
  1. `TestAllJSONRoutes_ErrorResponses_FollowEnvelope` — 9 cenários de erro table-driven (todas as rotas JSON)
  2. `TestAllJSONRoutes_SuccessResponses_FollowEnvelope` — 5 cenários de sucesso (healthz, upload/init, admin/*)
  3. `TestUnhandledPanic_ReturnsStandardErrorEnvelope` — panic recovery no envelope, servidor continua funcionando
  4. `TestNonAPIRoutes_NotForcedIntoEnvelope` — docs HTML/OpenAPI spec e erros em rotas HLS no envelope

### Verificações finais

- `go vet ./...` — limpo
- `grep` por `json.NewEncoder(w).Encode` e `fmt.Fprintf(w, `{` — apenas `internal/apiresponse` (o pacote central)
- `grep` por `w.Write([]byte(`{` — zero ocorrências
- `go test ./...` — 15/17 pacotes passam; 3 failures são pré-existentes (T39 transcode, T09 upload validation)
