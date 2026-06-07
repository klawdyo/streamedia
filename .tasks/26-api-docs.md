# T26: Documentação interativa da API (/docs com Scalar via CDN)

**Status:** done
**Dependências:** T20
**Estimativa:** pequena

## Contexto

A issue #12 pede uma forma de servir a documentação da API de forma mais
agradável que o Swagger UI clássico — ou ao menos disponibilizar o JSON da
especificação para que o cliente escolha como renderizar.

Decisão registrada na issue: usar **Scalar** (`@scalar/api-reference`) como UI,
carregado via **CDN** (jsDelivr), em vez de empacotar/embutir o bundle JS no
binário. Essa abordagem:

- Mantém o impacto no binário desprezível (apenas uma página HTML estática
  com poucas centenas de bytes).
- Evita adicionar qualquer dependência Go nova (sem swaggo, sem geração de
  código a partir de comentários).
- Usa um arquivo `openapi.json` escrito manualmente (fonte da verdade única),
  já que o projeto não usa nenhuma biblioteca de geração automática de spec.

Trade-off aceito: a página `/docs` depende de acesso ao CDN do jsDelivr para
carregar o componente do Scalar (não funciona 100% offline).

### Rotas a criar

```
GET /docs
  Serve uma página HTML estática que carrega o componente Scalar via CDN
  e aponta para /docs/openapi.json. Pública, sem autenticação.

GET /docs/openapi.json
  Serve o arquivo de especificação OpenAPI 3.x (JSON) com a descrição de
  todas as rotas públicas/documentáveis da API. Pública, sem autenticação.
```

## QA Instructions

Crie `internal/docs/docs_test.go`:

```
TestDocsPage_ReturnsHTML
  - GET /docs
  - Espera 200, Content-Type text/html, corpo contém "scalar" e
    referência a "/docs/openapi.json"

TestOpenAPISpec_ReturnsValidJSON
  - GET /docs/openapi.json
  - Espera 200, Content-Type application/json
  - Corpo deve ser JSON válido e conter campo "openapi" (versão 3.x) e
    "paths" com pelo menos as rotas principais (ex: /upload/init,
    /api/status/{video_id}, /healthz)

TestDocsRoutes_NoAuthRequired
  - GET /docs e GET /docs/openapi.json sem nenhum header de autenticação
  - Espera 200 em ambos (rotas públicas)
```

## Dev Instructions

Crie o pacote `internal/docs`:

### Arquivo de especificação

`internal/docs/openapi.json` — especificação OpenAPI 3.0/3.1 escrita à mão,
cobrindo pelo menos:

- `POST /upload/init`
- `POST /files/`, `POST|PATCH|HEAD|DELETE /files/{video_id}` (TUS)
- `GET /videos/{video_id}/master.m3u8`
- `GET /videos/{video_id}/{res}/playlist.m3u8`
- `GET /videos/{video_id}/{res}/{segment}`
- `GET /api/status/{video_id}`
- `GET /admin/videos`, `GET /admin/queue`
- `GET /healthz`

Use como referência a documentação manual já existente no `README.md`
(exemplos de request/response, esquemas de autenticação HMAC e Bearer).
Modele os esquemas de segurança (`securitySchemes`) para HMAC e Bearer
mesmo que de forma simplificada — o objetivo é que a spec sirva como
referência de consulta, não como contrato gerado automaticamente.

Embuta o arquivo com `go:embed`:

```go
//go:embed openapi.json
var openAPISpec []byte
```

### Handler da página /docs

Crie um handler simples que retorna um HTML estático carregando o Scalar
via CDN (jsDelivr) e apontando `data-url` para `/docs/openapi.json`:

```go
//go:embed index.html
var docsPage []byte

func NewHandler() *Handler { ... }

func (h *Handler) ServePage(w http.ResponseWriter, r *http.Request)
func (h *Handler) ServeSpec(w http.ResponseWriter, r *http.Request)
```

`index.html` deve conter algo equivalente a:

```html
<!doctype html>
<html>
  <head><title>Streamedia API Docs</title></head>
  <body>
    <script id="api-reference" data-url="/docs/openapi.json"></script>
    <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
  </body>
</html>
```

### Registro das rotas

Em `internal/server/server.go`, adicione (fora do grupo autenticado, junto
de `/healthz`):

```go
docsHandler := docs.NewHandler()
r.Get("/docs", docsHandler.ServePage)
r.Get("/docs/openapi.json", docsHandler.ServeSpec)
```

## Arquivos a criar/modificar

- `internal/docs/docs.go`
- `internal/docs/docs_test.go`
- `internal/docs/openapi.json`
- `internal/docs/index.html`
- Atualizar `internal/server/server.go` (registro das novas rotas)
- Atualizar `README.md` (mencionar a rota `/docs` como forma de consulta
  interativa da API, complementando a documentação manual existente)

## Definition of Done

- [ ] `GET /docs` serve página HTML que carrega o Scalar via CDN
- [ ] `GET /docs/openapi.json` serve spec OpenAPI 3.x válida cobrindo as
      rotas principais da API
- [ ] Ambas as rotas são públicas (sem autenticação)
- [ ] Todos os testes passam
