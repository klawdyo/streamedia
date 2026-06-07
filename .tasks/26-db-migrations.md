# T26: Sistema de migrations versionadas para o banco SQLite

**Status:** pending
**Dependências:** T03
**Estimativa:** média

## Contexto

A Issue #13 apontou um problema real na camada de persistência (T03): o schema
do banco é definido como uma **string DDL única** (`internal/db/schema.go`),
aplicada via `db.Exec(schema)` com `CREATE TABLE IF NOT EXISTS` toda vez que o
banco abre. Esse modelo só funciona enquanto as mudanças são puramente
aditivas e compatíveis com `IF NOT EXISTS` — ele quebra na primeira alteração
estrutural real (renomear coluna, alterar tipo, dropar índice antigo, etc.),
porque SQLite não suporta `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` nem
`DROP COLUMN IF EXISTS` de forma idempotente, e não há histórico de quais
alterações já foram aplicadas a um banco existente.

A solução escolhida — registrada na Issue #13 — é substituir a string única
por **arquivos de migration versionados**, gerenciados pela lib
[`pressly/goose`](https://github.com/pressly/goose), usada como **biblioteca
embutida** (não como CLI externo). Os arquivos `.sql` ficam no repositório,
embutidos no binário via `go:embed`, e o runner de migrations roda
**automaticamente toda vez que o servidor inicia** (dentro de `db.Open()`),
de forma idempotente: o goose mantém uma tabela de controle
(`goose_db_version`) com o histórico de versões já aplicadas, então cada
migration roda exatamente uma vez por banco, e bancos diferentes (dev,
produção, testes) convergem para o mesmo estado final aplicando apenas o que
ainda falta.

Isso substitui o conceito de "recriar tudo que não existe" por "aplicar, em
ordem, só os passos que esse banco específico ainda não viu" — o que permite
evoluir o schema com segurança (incluindo `down` migrations para rollback).

## O que muda em relação ao T03

- **Remove** `internal/db/schema.go` (a constante de schema deixa de existir).
- **Adiciona** `internal/db/migrations/`, com um arquivo `.sql` por alteração
  estrutural, numerado sequencialmente.
- A primeira migration (`0001_init.sql`) recria o schema atual (tabelas
  `videos`, `upload_tokens`, `webhook_log`, índices e o trigger
  `videos_updated_at`) — ou seja, é equivalente ao conteúdo atual de
  `schema.go`, só que expresso como um passo versionado e com bloco `Down`
  para rollback.
- `db.Open()` passa a, depois de configurar os `PRAGMA`s, **executar as
  migrations pendentes** (`goose.Up`) antes de retornar a conexão — isso
  acontece toda vez que o processo do servidor sobe, e é seguro porque o
  goose pula migrations já registradas como aplicadas.

## Estrutura de migrations

```
internal/db/migrations/
  0001_init.sql
```

Formato de cada arquivo (convenção do goose, com diretivas em comentário SQL):

```sql
-- +goose Up
CREATE TABLE IF NOT EXISTS videos (
  ...
);
-- (demais CREATE TABLE / CREATE INDEX / CREATE TRIGGER do schema atual)

-- +goose Down
DROP TRIGGER IF EXISTS videos_updated_at;
DROP TABLE IF EXISTS webhook_log;
DROP TABLE IF EXISTS upload_tokens;
DROP TABLE IF EXISTS videos;
```

Migrations futuras seguem a mesma convenção, numeradas sequencialmente
(`0002_xxx.sql`, `0003_xxx.sql`, ...), cada uma cobrindo uma única alteração
estrutural coesa.

## QA Instructions

Atualize/substitua `internal/db/db_test.go`:

```
TestOpen_RunsMigrations
  - Abre banco em memória (:memory:)
  - Verifica que a tabela de controle do goose (goose_db_version, ou nome
    equivalente) existe e contém ao menos um registro
  - Verifica que tabela videos existe (SELECT 1 FROM videos LIMIT 1 não falha)
  - Verifica que tabela upload_tokens existe
  - Verifica que tabela webhook_log existe

TestOpen_MigrationsAreIdempotent
  - Abre o mesmo arquivo de banco duas vezes em sequência (fechando entre
    as aberturas)
  - Verifica que a segunda abertura não falha e não duplica migrations
    (a contagem de linhas na tabela de controle não muda)

TestOpen_WALMode
  - (mantém do T03) Abre banco em memória, executa PRAGMA journal_mode,
    verifica que retorna "wal"

TestOpen_ForeignKeys
  - (mantém do T03) Tenta inserir upload_token com video_id inexistente,
    espera erro de foreign key constraint

TestOpen_MissingPath
  - (mantém do T03) Tenta abrir banco em diretório inexistente, espera erro

TestDB_UpdatedAtTrigger
  - (mantém do T03) Insere vídeo, atualiza status, verifica que updated_at
    mudou — confirma que o trigger criado pela migration 0001 está ativo

TestMigrations_EmbeddedFilesPresent
  - Verifica (via fs.Glob no embed.FS) que existe ao menos um arquivo
    "*.sql" embutido em internal/db/migrations
```

## Dev Instructions

### Dependência

Adicione `github.com/pressly/goose/v3` ao `go.mod` (biblioteca, não CLI —
não é necessário instalar nada no ambiente de execução).

### Embutindo as migrations no binário

Em `internal/db/migrations/migrations.go` (ou arquivo equivalente), use
`go:embed` para embutir os `.sql`:

```go
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
```

### Runner de migrations dentro de Open

Modifique `internal/db/db.go`:

```go
func Open(path string) (*sql.DB, error)
```

- Mantém todo o comportamento atual do T03: cria diretório pai, abre o banco,
  configura `PRAGMA journal_mode=WAL`, `PRAGMA foreign_keys=ON`,
  `PRAGMA busy_timeout=5000`, `SetMaxOpenConns(1)`.
- **Substitui** o trecho que executava a constante `schema` por uma chamada
  ao runner de migrations do goose, usando `migrations.FS` como fonte e o
  dialeto `sqlite3` (ou `sqlite`, conforme exigido pela versão do goose
  compatível com `modernc.org/sqlite`):
  - configura o provider/dialeto do goose;
  - executa `Up` (aplica todas as migrations pendentes, em ordem, até a
    versão mais recente).
- Se o runner retornar erro, `Open` retorna o erro (sem deixar o servidor
  subir com schema parcialmente aplicado).
- Comentário no código explicando que isso roda **a cada inicialização do
  servidor** e é seguro por ser idempotente (goose registra o que já foi
  aplicado).

### Migration inicial

Crie `internal/db/migrations/0001_init.sql` com `Up` equivalente ao conteúdo
atual de `schema.go` (as três tabelas, os três índices e o trigger
`videos_updated_at` documentados em T03) e `Down` que desfaz tudo na ordem
inversa (drops respeitando foreign keys).

### Remoção do schema antigo

Apague `internal/db/schema.go` — seu conteúdo agora vive, de forma
versionada, em `0001_init.sql`.

## Arquivos a criar/modificar

- `internal/db/db.go` (modificar — troca aplicação de schema por runner de migrations)
- `internal/db/migrations/migrations.go` (criar — embed.FS)
- `internal/db/migrations/0001_init.sql` (criar — schema inicial)
- `internal/db/schema.go` (remover)
- `internal/db/db_test.go` (atualizar conforme QA Instructions)
- `go.mod` / `go.sum` (adicionar `pressly/goose/v3`)

## Definition of Done

- [ ] `Open()` aplica automaticamente as migrations pendentes a cada
      inicialização, antes de o servidor aceitar conexões
- [ ] Tabela de controle de versão do goose existe e reflete o estado real
- [ ] Reabrir um banco já migrado não falha nem reaplica migrations
- [ ] `0001_init.sql` reproduz fielmente o schema documentado em T03
      (tabelas, índices, trigger `videos_updated_at`)
- [ ] `internal/db/schema.go` removido
- [ ] Todos os testes passam
- [ ] `go vet ./...` limpo
