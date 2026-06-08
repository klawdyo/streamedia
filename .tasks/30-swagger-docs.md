# T30: Documentação da API via Swagger/OpenAPI

**Status:** pending
**Dependências:** T20, T13, T18, T28, T29
**Estimativa:** média
**Issue relacionada:** #3

## Contexto

A issue #3 pede para "disponibilizar o swagger" — ou seja, documentação
interativa da API HTTP do Streamedia no padrão OpenAPI (Swagger), acessível
via navegador.

Esta tarefa deve ser feita **por último** dentro desta onda (depende das
rotas de estatísticas/admin criadas em T28 e T29, além das rotas já
existentes de T13/T18/T20), para que a especificação documente a API
completa e atualizada.

### Abordagem

Usar anotações `swaggo/swag` (gera `docs/` a partir de comentários nos
handlers, padrão idiomático em projetos Go com `net/http`/`chi`) e servir a
UI via `swaggo/http-swagger`. Alternativa aceitável: escrever o arquivo
`openapi.yaml` manualmente e servir com `swagger-ui` estático — escolha a
abordagem que exigir menos manutenção contínua (favoreça geração automática
via anotações se a equipe achar razoável manter comentários nos handlers).

### Rotas a documentar (mínimo)

- `POST /upload/init` (T08)
- TUS endpoints (T07) — descrição de alto nível (protocolo TUS, não cada
  detalhe interno)
- `GET /api/status/{video_id}` (T13)
- `GET /videos/{video_id}/master.m3u8` e serving estático (T12)
- `GET /admin/videos`, `GET /admin/queue` (T18)
- `GET /admin/stats` (T28)
- `GET /metrics` (T29) — referenciar como "formato Prometheus", sem detalhar
  no schema OpenAPI (não é uma rota JSON)

### Endpoint de documentação

```
GET /docs/*  (ou /swagger/*)  → UI interativa do Swagger
GET /docs/openapi.json (ou .yaml) → spec bruta
```

Sem autenticação para a UI (mas considere se a exposição pública da
documentação de rotas administrativas é aceitável — se não for, registre
a rota de docs atrás de autenticação básica ou do mesmo `AdminAuth`,
documentando a decisão no código).

## QA Instructions

Crie `internal/docs/docs_test.go` (ou local equivalente ao pacote escolhido):

```
TestSwaggerUIRouteServesHTML
  - GET /docs/ (ou /swagger/) → 200, Content-Type text/html
  - Corpo contém referência a "swagger" (case-insensitive)

TestOpenAPISpecIsValidJSON
  - GET /docs/openapi.json → 200
  - Corpo é JSON válido e contém campos obrigatórios da spec OpenAPI
    ("openapi" ou "swagger", "info", "paths")

TestOpenAPISpecDocumentsKnownRoutes
  - Verifica que os paths da spec incluem pelo menos:
    "/upload/init", "/api/status/{video_id}", "/admin/stats"
    (ajuste os nomes de path conforme a convenção real de roteamento)
```

## Dev Instructions

- Se optar por `swaggo/swag`: adicione anotações `// @Summary`, `// @Tags`,
  `// @Param`, `// @Success`, `// @Router` etc. nos handlers já existentes
  (não precisa reescrever lógica — apenas comentários de documentação) e
  rode `swag init` para gerar `docs/docs.go`, `docs/swagger.json`,
  `docs/swagger.yaml`. Sirva a UI com
  `httpSwagger.WrapHandler` (`swaggo/http-swagger`).
- Se optar por spec manual: escreva `docs/openapi.yaml` cobrindo as rotas
  listadas acima, e sirva tanto o arquivo quanto uma UI estática (ex.
  `swagger-ui-dist` embutido via `embed.FS`).
- Registre a(s) rota(s) de documentação na montagem do servidor (T20).
- Adicione as dependências escolhidas ao `go.mod`.
- Documente no `README.md` (T24) como acessar a documentação interativa
  (ex. `http://localhost:PORT/docs/`).
- Comente (uma linha) a decisão de manter ou não a documentação atrás de
  autenticação, e por quê.

## Arquivos a criar/modificar

- Handlers existentes (anotações, se optar por `swaggo/swag`)
- `docs/` (gerado por `swag init`, ou `openapi.yaml` manual)
- `internal/docs/` ou pacote equivalente para servir a UI
- `internal/docs/docs_test.go`
- `go.mod` / `go.sum`
- `README.md` (instruções de acesso)
- Arquivo de montagem do servidor (registro das rotas de docs)

## Definition of Done

- [ ] UI interativa do Swagger acessível via rota dedicada
- [ ] Spec OpenAPI válida, servida em formato JSON (e/ou YAML)
- [ ] Rotas principais da API documentadas (upload, status, serving, admin, stats)
- [ ] README atualizado com instruções de acesso à documentação
- [ ] Decisão sobre autenticação da documentação registrada em comentário
- [ ] Todos os testes passam
</content>
