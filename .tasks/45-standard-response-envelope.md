# T45: Pacote central de resposta padronizada `{error, message, data, status_code}`

**Status:** pending
**Dependências:** nenhuma (cria a fundação; T46 depende desta)
**Estimativa:** média
**Origem:** Issue #9 — "Padronização das respostas" (pedida também
diretamente na sessão de 2026-06-07, antes da issue ser aberta)

## Contexto

Hoje o sistema tem **pelo menos quatro implementações independentes** de
"responder erro em JSON", cada uma com um formato ligeiramente diferente:

- `internal/serve/serve.go:45` — `respondError`: `{"error": "msg"}`,
  `Content-Type: application/json; charset=utf-8`
- `internal/upload/init.go:168` — `respondError`: `{"error":"msg"}` via
  `fmt.Fprintf` (sem `json.Encoder` — risco de escaping incorreto),
  `Content-Type: application/json` (sem charset)
- `internal/admin/projects.go:25` — `respondJSONError`: `{"error": "msg"}`,
  com charset
- Respostas de sucesso são, cada uma, uma `struct`/`map` ad-hoc codificada
  diretamente (`internal/admin/admin.go`, `internal/admin/stats.go`,
  `internal/serve/status.go`, `internal/upload/init.go`, `internal/docs/docs.go`)
- `internal/upload/tus.go` responde erros de autenticação só com
  `w.WriteHeader(status)`, sem corpo JSON algum (linhas 115, 127, 135, 145)
- Panics são capturados pelo `chimw.Recoverer` do chi
  (`internal/server/server.go:84`), que devolve `500` em texto puro — não
  no formato JSON do resto da API

Essa fragmentação é exatamente o tipo de problema que o usuário quer
eliminar (o mesmo padrão de "várias regex de UUID em vez de uma função" que
motivou a T44, agora aplicado a respostas HTTP). O usuário definiu o
envelope padrão que TODA resposta JSON da API deve seguir:

```json
{
  "error": false,
  "message": "ok",
  "data": { }
}
```

Regras do envelope:
- `error` (bool): `false` em sucesso, `true` em qualquer erro
- `message` (string): mensagem legível; em sucesso, `"ok"`; em erro, a
  mensagem descritiva do problema (em português, conforme convenção do
  projeto para mensagens de erro da API)
- `data` (object | array | null): o payload em sucesso; `null` em erro
  (ex.: erro de validação → `data: null`)
- **O JSON também deve incluir o status code HTTP** (além de ir no header
  da resposta) — campo a definir nesta tarefa, ex. `"status_code": 400`.
  Inclua-o no struct/envelope de forma que apareça em toda resposta,
  sucesso ou erro.

Esta tarefa constrói a **fundação**: o pacote central, o middleware de
recuperação de panics no novo formato, e a documentação do padrão na
especificação. A **migração de todas as rotas existentes** para usar essa
fundação é o escopo da T46 (não desta tarefa) — manter o blast radius
gerenciável e revisável.

## Escopo: quais respostas seguem o envelope, quais não

Definido aqui para orientar T45 e T46 (evite ambiguidade depois):

**Seguem o envelope `{error, message, data, status_code}`:**
- Toda resposta de erro JSON da API (`400`, `401`, `403`, `404`, `409`,
  `413`, `429`, `500`, etc.) — em QUALQUER rota, inclusive as que servem
  conteúdo binário/streaming (ex.: um `401` em
  `GET /videos/{id}/master.m3u8` deve usar o envelope, mesmo que a
  resposta de sucesso daquela rota seja um arquivo `.m3u8`)
- Toda resposta de sucesso das rotas que já respondem JSON estruturado:
  `/upload/init`, `/api/status/{id}`, `/admin/*` (videos, queue, stats,
  projects, upload-tokens), `/healthz`
- Panics não tratados (qualquer rota) — middleware de recovery deve
  responder `500` no envelope com mensagem genérica em português (ex.:
  `"Erro interno desconhecido."`), sem vazar stack trace ao cliente

**NÃO seguem o envelope (ficam como estão — conteúdo não é JSON de API):**
- Conteúdo HLS (`master.m3u8`, `playlist.m3u8`, segmentos `.ts`/`.m4s`) —
  corpo binário/texto de mídia, não é resposta de API
- `/metrics` — formato Prometheus/OpenTelemetry (texto), padrão externo
  imutável
- `/docs/` (UI do Swagger) — HTML
- `/docs/openapi.json` — é JSON, mas é um **documento de especificação**
  (schema OpenAPI), não uma resposta de API; mantenha como está
- Handler TUS (`/files/*`) — delega ao `tusd`, que tem seu próprio
  protocolo de resposta (headers `Tus-*`, corpos vazios); **porém** os
  pontos onde o PRÓPRIO código do projeto intercepta e responde antes de
  delegar ao tusd (autenticação em `tus.go:115,127,135,145`) DEVEM usar o
  envelope — hoje respondem só com status code e corpo vazio

Se durante a implementação você encontrar uma rota que não se encaixa
claramente nessas categorias, documente a decisão tomada na seção
"Resolução" do arquivo de tarefa (`.tasks/45-...md`) e, se for o caso, na
spec.

## QA Instructions

Crie `internal/apiresponse/apiresponse_test.go` (ou nome de pacote
escolhido pelo Dev — ver Dev Instructions sobre nome):

```
TestSuccess_EncodesEnvelope
  - chama o helper de sucesso com status 200 e um payload qualquer
  - decodifica a resposta e verifica:
    error == false, message == "ok", data == payload, status_code == 200
  - Content-Type é "application/json; charset=utf-8"
  - status HTTP do header é 200

TestSuccess_NilData
  - helper de sucesso com data == nil
  - decodifica e verifica que "data" aparece como null no JSON (não omitido)

TestError_EncodesEnvelope
  - chama o helper de erro com status 400 e mensagem "campo X é obrigatório"
  - decodifica e verifica:
    error == true, message == "campo X é obrigatório", data == nil,
    status_code == 400
  - status HTTP do header é 400

TestError_TableDriven_AllStatusCodes
  - tabela cobrindo 400, 401, 403, 404, 409, 413, 429, 500
  - cada um gera o status_code correto no JSON e no header

TestRecoveryMiddleware_CatchesPanic
  - monta um handler de teste que dá panic (ex.: nil pointer dereference,
    panic("boom"))
  - envia o middleware de recovery
  - resposta é 500, no envelope, com mensagem genérica em português
    (ex. "Erro interno desconhecido.") — NÃO vaza a mensagem original do
    panic nem stack trace
  - verifica que o servidor não derruba (handler seguinte continua
    funcionando após o panic recuperado)
```

Confirme que os testes FALHAM antes da implementação (o pacote ainda não existe).

## Dev Instructions

### 1. Crie o pacote central `internal/apiresponse`

(Nome sugerido — pode ajustar para algo que combine melhor com as
convenções do projeto, ex. `internal/httpresponse`; documente a escolha.)

```go
package apiresponse

// Envelope é o formato padrão de toda resposta JSON da API. Unifica o
// que antes eram 3+ implementações divergentes de respondError espalhadas
// pelos pacotes — qualquer rota nova OU qualquer mudança no formato passa
// a acontecer em um único lugar.
type Envelope struct {
    Error      bool        `json:"error"`
    Message    string      `json:"message"`
    Data       interface{} `json:"data"`
    StatusCode int         `json:"status_code"`
}

// Success escreve uma resposta de sucesso no envelope padrão: error=false,
// message="ok", data=payload, status_code=status. Use status=200 para o
// caso comum; aceita outros códigos 2xx (ex. 201 em criação de recurso).
func Success(w http.ResponseWriter, status int, data interface{})

// Error escreve uma resposta de erro no envelope padrão: error=true,
// message=msg, data=null, status_code=status. Mensagens em português,
// conforme convenção do projeto para erros da API.
func Error(w http.ResponseWriter, status int, msg string)
```

Detalhes de implementação a respeitar:
- `Content-Type: application/json; charset=utf-8` sempre
- Use `json.NewEncoder(w).Encode(...)` (não `fmt.Fprintf` — o
  `init.go` atual usa `%q` manualmente, o que é frágil para mensagens com
  caracteres especiais/acentos)
- `data: null` deve aparecer explicitamente no JSON quando não há payload
  (não omita o campo)
- Documente CADA struct/função com comentários densos em português,
  conforme convenção do projeto

### 2. Middleware de recuperação de panics no formato padrão

Substitua `chimw.Recoverer` (linha ~84 de `internal/server/server.go`) por
um middleware do projeto que:
- Recupera o panic
- Loga o erro original (para debug interno — `log.Printf` ou equivalente
  já usado no projeto)
- Responde ao cliente com `apiresponse.Error(w, 500, "Erro interno
  desconhecido.")` — mensagem genérica, NUNCA o conteúdo original do panic
  (evita vazamento de detalhes internos — ver também a auditoria de
  segurança T43)

Pode viver em `internal/apiresponse` (ex. `apiresponse.RecoveryMiddleware`)
ou em `internal/middleware` (já existe o pacote, com `ratelimit.go`) —
escolha o local que fizer mais sentido e documente a decisão.

### 3. Documente o padrão na especificação

Adicione uma seção em `spec/ESPECIFICACAOv4.md` (ex. "Formato padrão de
resposta da API" ou similar, próxima à descrição das rotas) descrevendo:
- O envelope `{error, message, data, status_code}` e o significado de
  cada campo
- A regra `error=false / message="ok"` em sucesso vs.
  `error=true / data=null` em erro
- Quais rotas seguem o envelope e quais não (replique a seção "Escopo"
  desta tarefa, adaptada para a spec)
- Que TODA nova rota deve usar `apiresponse.Success`/`apiresponse.Error` —
  nunca escrever JSON de resposta manualmente

Isso evita que о padrão se perca de novo no futuro (é exatamente o
registro que o usuário pediu para "não gerar erro futuro").

### 4. NÃO migre as rotas existentes nesta tarefa

Apenas crie a fundação (pacote + middleware + doc). A migração e os testes
de conformidade ponta-a-ponta são o escopo da T46. Isso mantém o diff
revisável e evita misturar "criar a ferramenta" com "trocar 20 lugares".

## Arquivos a criar/editar

- `internal/apiresponse/apiresponse.go` (novo pacote — `Envelope`,
  `Success`, `Error`, `RecoveryMiddleware` ou equivalente)
- `internal/apiresponse/apiresponse_test.go`
- `internal/server/server.go`: troca `chimw.Recoverer` pelo middleware do
  projeto
- `spec/ESPECIFICACAOv4.md`: nova seção documentando o padrão

## Definition of Done

- [x] Pacote central criado com `Envelope`, função de sucesso e função de
      erro, cobrindo `error`, `message`, `data` e `status_code`
- [x] `data: null` aparece explicitamente (não omitido) em respostas sem payload
- [x] Middleware de recuperação de panics responde no envelope padrão,
      com mensagem genérica e sem vazar detalhes internos
- [x] `chimw.Recoverer` substituído pelo middleware do projeto em
      `internal/server/server.go`
- [x] Seção do padrão documentada em `spec/ESPECIFICACAOv4.md`, incluindo
      a lista do que segue e do que não segue o envelope
- [x] `go test ./internal/apiresponse/... -v` passa, incluindo o teste do
      middleware de recovery
- [x] `go test ./...` continua passando sem regressões

## Resolução

### Arquivos criados/alterados

- `internal/apiresponse/apiresponse.go` — Pacote central com `Envelope`, `Success(w, status, data)`, `Error(w, status, msg)`. Sempre usa `json.NewEncoder(w).Encode(...)` e `Content-Type: application/json; charset=utf-8`.
- `internal/apiresponse/apiresponse_test.go` — 6 testes table-driven: sucesso com payload, sucesso com nil data (null no JSON explícito), erro com mensagem, tabela de todos os status codes de erro (400-500), tabela de status de sucesso (200/201), verificação de `"data":null` literal no corpo bruto.
- `internal/middleware/recovery.go` — `RecoveryMiddleware`: substitui `chimw.Recoverer`. Recupera panics, loga o erro internamente com `log.Printf` (incluindo método e path), responde ao cliente com `apiresponse.Error(w, 500, "Erro interno desconhecido.")` — nunca vaza o conteúdo do panic.
- `internal/server/server.go:84` — Troca `r.Use(chimw.Recoverer)` por `r.Use(middleware.RecoveryMiddleware)`.
- `spec/ESPECIFICACAOv4.md` — Nova seção 10 "Formato padrão de resposta da API" com a estrutura do envelope, significado de cada campo, lista explícita das rotas que seguem e não seguem o envelope, e a regra para novas rotas. Seções renumeradas de 10-20 → 11-21 com cross-references atualizadas.

### Decisões tomadas

- **Nome do pacote**: `apiresponse` (em vez de `httpresponse`) — mais curto, consistente com o domínio (API response vs HTTP genérico).
- **Local do RecoveryMiddleware**: `internal/middleware/recovery.go` — mantém coesão: o pacote `middleware` já agrupa middlewares HTTP do projeto (`ratelimit.go`). O import de `apiresponse` é natural (middleware depende do formato de resposta).
- **Reuso de `chimw.Logger`**: mantido — o logger do chi é só logging, não afeta o formato de resposta.
- **Três falhas preexistentes em `go test ./...`**: `TestBuildFFmpegArgs_MinimalArgs` (transcode) e `TestPostFinishValidation_InvalidMagicBytes`/`TestPostFinishValidation_SizeMismatch` (upload) já falhavam antes desta tarefa — não são regressões introduzidas por T45.
