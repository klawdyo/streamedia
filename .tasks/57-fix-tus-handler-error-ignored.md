# T57: Corrigir erro de `NewTUSHandler` ignorado em `server.go`

**Status:** done
**Dependências:** T07, T20
**Estimativa:** pequena
**Origem:** análise de código — crash em produção
**Severidade:** critica

## Contexto

Em `internal/server/server.go:57`:

```go
tusHandler, _ := upload.NewTUSHandler(cfg, database, onFinish)
```

O erro retornado por `NewTUSHandler` é descartado com `_ =`. Se a criação
falhar (ex.: diretório de upload inexistente, erro do tusd), `tusHandler`
é `nil` e todas as rotas `/files/*` (linhas 107-111) vão causar panic ao
chamar `tusHandler.ServeHTTP` — o servidor sobe mas qualquer upload crasha.

## Impacto

- **Servidor sobe com handler nil** — todas as operações TUS causam panic.
- A falha é silenciosa: nenhum log indica que o handler TUS não foi criado.
- O problema se manifesta apenas quando um upload é tentado, não na inicialização.

## QA Instructions

```
TestNewRouter_TUSHandlerError
  - Configura cfg com UploadTmpDir apontando para diretório inexistente
  - Verifica que NewRouter retorna erro (ou que o servidor não sobe)
```

## Dev Instructions

### 1. Propagar o erro em `server.go`

`NewRouter` atualmente retorna `http.Handler`. Há duas opções:

**(a) Mudar assinatura para retornar erro** (preferível):
```go
func NewRouter(...) (http.Handler, error) {
```
E tratar o erro no `main.go` com `log.Fatal`.

**(b) Fail-fast com log.Fatal dentro de NewRouter** (mais simples se mudar
a assinatura for invasivo):
```go
tusHandler, err := upload.NewTUSHandler(cfg, database, onFinish)
if err != nil {
    log.Fatalf("[server] falha ao criar handler TUS: %v", err)
}
```

Avaliar qual opção é mais consistente com o padrão do projeto.

### 2. Verificação

- `go test ./...` — sem regressões
- `go vet ./...` — sem warnings
- Verificar que `cmd/server/main.go` trata o novo erro se opção (a)

## Arquivos a editar

- `internal/server/server.go` (tratar erro de NewTUSHandler)
- `cmd/server/main.go` (se opção (a), tratar novo retorno de erro)

## Resolução

Adotada opção (a): mudança de assinatura de `NewRouter` para retornar
`(http.Handler, io.Closer, error)`. O `io.Closer` encapsula o `tusHandler.Stop()`
(T59). O `main.go` trata o erro com `log.Fatalf`.

Arquivos alterados:
- `internal/server/server.go`: `NewRouter` retorna `(http.Handler, io.Closer, error)`;
  erro de `NewTUSHandler` propagado; `closerFunc` helper criada para o io.Closer.
- `cmd/server/main.go`: trata erro de `NewRouter` com `log.Fatalf`;
  chama `routerCloser.Close()` no defer.
- `internal/server/server_test.go`: adaptado para nova assinatura.
- `internal/integration/integration_test.go`: adaptado para nova assinatura.

## Definition of Done

- [x] Erro de `NewTUSHandler` é tratado — servidor não sobe com handler nil
- [x] Log claro indica a causa da falha
- [x] `go test ./...` passa
- [x] `go vet ./...` limpo
