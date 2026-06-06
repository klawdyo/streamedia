# T03: Camada SQLite

**Status:** pending
**Dependências:** T02
**Estimativa:** média

## Contexto

O banco SQLite guarda o estado de todos os vídeos, tokens de upload e log de
webhooks. É a fonte de verdade do sistema — a existência de pastas no disco NÃO
indica que um vídeo está pronto; apenas o status no banco indica.

O banco deve ser aberto com:
- `PRAGMA journal_mode=WAL` — permite leituras concorrentes durante escritas
- `PRAGMA foreign_keys=ON` — valida foreign keys
- `PRAGMA busy_timeout=5000` — aguarda até 5s se banco estiver bloqueado

## Schema completo

```sql
CREATE TABLE IF NOT EXISTS videos (
  video_id            TEXT PRIMARY KEY,
  status              TEXT NOT NULL DEFAULT 'pending_upload',
  declared_size_bytes INTEGER,
  actual_size_bytes   INTEGER,
  duration_s          INTEGER,
  resolutions         TEXT,
  transcode_attempts  INTEGER NOT NULL DEFAULT 0,
  last_chunk_at       DATETIME,
  error_message       TEXT,
  created_at          DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at          DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS upload_tokens (
  token      TEXT PRIMARY KEY,
  video_id   TEXT NOT NULL UNIQUE,
  expires_at DATETIME NOT NULL,
  FOREIGN KEY (video_id) REFERENCES videos(video_id)
);

CREATE TABLE IF NOT EXISTS webhook_log (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  video_id   TEXT NOT NULL,
  event      TEXT NOT NULL,
  payload    TEXT,
  sent_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
  success    INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_videos_status ON videos(status);
CREATE INDEX IF NOT EXISTS idx_videos_last_chunk ON videos(last_chunk_at);
CREATE INDEX IF NOT EXISTS idx_tokens_expires ON upload_tokens(expires_at);
```

## QA Instructions

Crie `internal/db/db_test.go`:

```
TestOpen_CreatesSchema
  - Abre banco em memória (:memory:)
  - Verifica que tabela videos existe (SELECT 1 FROM videos LIMIT 1 não falha)
  - Verifica que tabela upload_tokens existe
  - Verifica que tabela webhook_log existe

TestOpen_WALMode
  - Abre banco em memória
  - Executa PRAGMA journal_mode
  - Verifica que retorna "wal"

TestOpen_ForeignKeys
  - Abre banco em memória
  - Tenta inserir upload_token com video_id inexistente
  - Espera erro de foreign key constraint

TestOpen_Idempotent
  - Abre banco e fecha
  - Abre o mesmo arquivo novamente
  - Verifica que não falha (CREATE TABLE IF NOT EXISTS)

TestOpen_MissingPath
  - Tenta abrir banco em diretório inexistente (/nao/existe/media.db)
  - Espera erro não-nulo

TestDB_UpdatedAtTrigger
  - Insere um vídeo
  - Aguarda 1ms
  - Atualiza o status do vídeo
  - Verifica que updated_at mudou
```

## Dev Instructions

Crie `internal/db/db.go`:

### Função Open

```go
func Open(path string) (*sql.DB, error)
```

- Usa `database/sql` com driver `modernc.org/sqlite`
- Cria o diretório pai se não existir
- Abre o banco com o path fornecido
- Executa `PRAGMA journal_mode=WAL`
- Executa `PRAGMA foreign_keys=ON`
- Executa `PRAGMA busy_timeout=5000`
- Executa o schema (CREATE TABLE IF NOT EXISTS para cada tabela)
- Cria os índices
- Configura `SetMaxOpenConns(1)` — SQLite não suporta múltiplas escritas simultâneas
- Retorna o db ou erro

### Trigger updated_at

Adicione um trigger para manter `updated_at` atualizado na tabela videos:

```sql
CREATE TRIGGER IF NOT EXISTS videos_updated_at
AFTER UPDATE ON videos
FOR EACH ROW
BEGIN
  UPDATE videos SET updated_at = CURRENT_TIMESTAMP WHERE video_id = NEW.video_id;
END;
```

### Schema como constante

Defina o schema DDL como uma constante ou variável string em `schema.go`
separado, para facilitar leitura e manutenção.

## Arquivos a criar/modificar

- `internal/db/db.go`
- `internal/db/schema.go`
- `internal/db/db_test.go`

## Definition of Done

- [ ] `Open()` cria todas as tabelas e índices
- [ ] WAL mode ativado
- [ ] Foreign keys ativadas
- [ ] Todos os testes passam
- [ ] `go vet ./...` limpo
