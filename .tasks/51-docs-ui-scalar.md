# T51: Trocar UI de documentação da API de Swagger para Scalar

**Status:** done
**Dependências:** T30
**Estimativa:** pequena
**Issue relacionada:** #12

## Contexto

A issue #3 ("Disponibilizar o swagger") foi fechada pela T30, que criou o
pacote `internal/docs` servindo `GET /docs/` (UI) e `GET /docs/openapi.json`
(spec OpenAPI 3.0), usando **Swagger UI via CDN**.

A issue #12 é uma continuação direta: o autor achou a UI padrão do Swagger
pouco agradável visualmente e pediu opções alternativas antes de qualquer
nova tarefa ser criada. As opções levantadas e discutidas com o autor:

- **Redoc** — leitura em três colunas, visual "documento".
- **Scalar** — UI moderna, tema escuro, mais interativa.
- Em ambos os casos, carregar via **CDN** (jsDelivr) em vez de embutir o
  bundle JS no binário — mantém o impacto no tamanho do executável
  desprezível (a página é só HTML estático) e não adiciona dependências Go.

Decisão: **Scalar via CDN**. A spec OpenAPI já existente (`internal/docs`,
escrita na T30) **não muda** — só a página HTML servida em `/docs/` troca
de Swagger UI para Scalar.

## Resolução

- `internal/docs/docs.go`: renomeada a constante `swaggerUIPage` para
  `scalarUIPage`, com o HTML mínimo que carrega `@scalar/api-reference`
  via `https://cdn.jsdelivr.net/npm/@scalar/api-reference` apontando
  `data-url="/docs/openapi.json"`. Comentários do pacote atualizados para
  registrar a decisão e o histórico (T30 → T51).
- `internal/docs/docs_test.go`: `TestSwaggerUIRouteServesHTML` renomeado
  para `TestScalarUIRouteServesHTML`, agora verificando referência a
  `"scalar"` e a `"/docs/openapi.json"` no corpo da página (em vez de
  `"swagger"`).
- `internal/docs/spec.go`: **sem alterações** — a especificação OpenAPI
  continua a mesma; só o consumidor (UI) mudou.
- `internal/server/server.go`: **sem alterações** — as rotas `/docs/` e
  `/docs/openapi.json` continuam apontando para os mesmos handlers
  (`ServeUI`, `ServeOpenAPISpec`); o conteúdo de `ServeUI` é que mudou.
- `README.md`: seção "Documentação interativa da API" atualizada de
  "(Swagger)" para "(Scalar)", trocando a referência ao Swagger UI pelo
  Scalar e registrando uma nota sobre a troca (para quem ler o histórico
  e estranhar a menção a Swagger em commits anteriores).

## Definition of Done

- [x] `GET /docs/` serve uma página HTML que carrega o Scalar via CDN
- [x] A spec OpenAPI em `/docs/openapi.json` permanece inalterada
- [x] Testes atualizados e passando (`TestScalarUIRouteServesHTML`,
      `TestOpenAPISpecIsValidJSON`, `TestOpenAPISpecDocumentsKnownRoutes`)
- [x] README atualizado
