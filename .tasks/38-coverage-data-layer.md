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

## Resolução

Cobertura "antes": `internal/models` 56.6%, `internal/db` 57.1%.
Cobertura "depois": `internal/models` 80.8%, `internal/db` 58.0%.

Gaps identificados e fechados com 27 testes novos (table-driven onde fazia
sentido), distribuídos em:

- `internal/models/token_test.go` (+3): `scanUploadToken` com `project_id`
  NULL, `DeleteExpiredTokens` no limite exato de expiração, inserção de
  token com `project_id` nil.
- `internal/models/video_test.go` (+8): branches de `UpdateStatus` e
  `UpdateStatusWithError` (persistência de `error_message`),
  `SetUploadComplete`/`SetReady` (serialização de `resolutions` e
  `actual_size_bytes`), `IncrementTranscodeAttempts` com múltiplos
  incrementos, `ListByStatus` com várias linhas, `GetVideo` com todos os
  campos anuláveis preenchidos/NULL.
- `internal/db/db_test.go` (+4): `Open` ativando `PRAGMA foreign_keys` e
  rejeitando FK inválida, `ensureColumn` criando coluna ausente e
  preservando coluna existente, escritas sequenciais com `MaxOpenConns=1`.
- `internal/db/schema_test.go` (novo, 14 testes): estrutura de cada
  tabela (`videos`, `upload_tokens`, `projects`, `video_renditions`,
  `playback_events`), índices, trigger `videos_updated_at`, constraints
  `UNIQUE` (`projects.slug`, `upload_tokens.video_id`) e `PRIMARY KEY`
  composta (`video_renditions`), e idempotência de `CREATE ... IF NOT
  EXISTS` ao reabrir o mesmo banco.

**Bugs reais encontrados:** nenhum. Os 3 testes que falharam durante a
escrita tinham problemas no próprio teste (FK ausente no fixture de setup,
race condition de timing no boundary de expiração) — corrigidos no teste,
sem tocar em código de produção.

**Fora de escopo:** `GetProjectByMasterKeyHash` e `ResolveVideoRootDir`
(em `internal/models/project.go`) seguem com baixa cobertura — pertencem
à cadeia de projetos (T32-T35), não à camada de dados core revisada aqui.

`go test ./...` passa integralmente (sem regressões).

## Definition of Done

- [x] Relatório de cobertura "antes" documentado no PR/commit
- [x] Gaps de cobertura identificados e listados (função + motivo)
- [x] Testes novos escritos para os gaps relevantes
- [x] Bugs reais encontrados (se houver) corrigidos com mudança mínima — nenhum encontrado
- [x] `go test ./internal/models/... ./internal/db/... -cover` mostra
      aumento de cobertura
- [x] `go test ./...` continua passando sem regressões
