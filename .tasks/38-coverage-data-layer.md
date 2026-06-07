# T38: Cobertura de testes — camada de dados (models + db)

**Status:** pending
**Dependências:** nenhuma (revisão do código existente)
**Estimativa:** média
**Origem:** Issue #7 — "Revisão geral do código: procure pontos não cobertos por testes"

## Contexto

A suíte atual reporta cobertura de `internal/models` em 56.6% e `internal/db`
em 57.1% — as camadas mais centrais do sistema (modelos de domínio e acesso
ao banco) estão entre as menos testadas. Esta tarefa é uma auditoria
focada: mapear exatamente quais funções/branches não estão cobertas e
escrever os testes que faltam, sem alterar o comportamento do código de
produção (a menos que a ausência de teste revele um bug real).

## Arquivos sob revisão

- `internal/models/video.go` (+ `video_test.go`)
- `internal/models/token.go` (+ `token_test.go`)
- `internal/db/db.go` (+ `db_test.go`)
- `internal/db/schema.go` (sem teste dedicado)

## QA Instructions

1. Rode `go test ./internal/models/... ./internal/db/... -coverprofile=coverage.out`
   e gere o relatório por linha com `go tool cover -func=coverage.out`.
2. Identifique funções e branches com 0% ou cobertura parcial — preste
   atenção especial a:
   - Transições de estado da máquina de estados de `Video` (caminhos de erro,
     transições inválidas)
   - Validações em `UploadToken` (expiração, formato, casos-limite)
   - Funções de `db.go` que lidam com erros de conexão, migrações e
     transações (rollback em caso de falha)
   - `schema.go`: criação de tabelas/índices, idempotência de migrações
3. Escreva testes (table-driven sempre que fizer sentido) que cubram os
   caminhos identificados, incluindo casos de erro e valores-limite.
4. Os novos testes devem FALHAR apenas se revelarem um bug real — caso
   contrário, devem passar contra o código atual (não é um ciclo red→green
   de feature nova, é fechamento de lacuna de cobertura).

## Dev Instructions

1. Receba a lista de gaps de cobertura e os testes novos do QA.
2. Se algum teste novo falhar revelando um bug real (ex.: transição de
   estado inválida permitida, erro de SQL não tratado, rollback ausente),
   corrija o código de produção com o mínimo de mudança necessária.
3. Não refatore código que já está coberto e funcionando — foco é
   exclusivamente fechar lacunas e corrigir bugs reais encontrados.
4. Rode `go test ./... -cover` e confirme que a cobertura de
   `internal/models` e `internal/db` subiu de forma mensurável.

## Arquivos a revisar/editar

- `internal/models/video_test.go`
- `internal/models/token_test.go`
- `internal/db/db_test.go`
- `internal/db/schema_test.go` (criar, se schema.go não tiver teste dedicado)

## Definition of Done

- [ ] Relatório de cobertura "antes" documentado no PR/commit
- [ ] Gaps de cobertura identificados e listados (função + motivo)
- [ ] Testes novos escritos para os gaps relevantes
- [ ] Bugs reais encontrados (se houver) corrigidos com mudança mínima
- [ ] `go test ./internal/models/... ./internal/db/... -cover` mostra
      aumento de cobertura
- [ ] `go test ./...` continua passando sem regressões
