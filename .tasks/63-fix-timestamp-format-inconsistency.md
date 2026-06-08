# T63: Corrigir inconsistencia de formato de timestamp no SQLite

**Status:** done
**Dependências:** T10, T15
**Estimativa:** pequena
**Origem:** análise de código — risco de bug silencioso
**Severidade:** alta

## Contexto

O projeto usa formatos de timestamp inconsistentes nas escritas SQLite:

1. **`queue.go:92`** — `Enqueue` usa RFC3339 com microsegundos:
   ```sql
   strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
   -- Resultado: 2026-06-07T12:34:56.789Z
   ```

2. **`video.go:200`** — `UpdateLastChunk` usa `CURRENT_TIMESTAMP`:
   ```sql
   UPDATE videos SET last_chunk_at = CURRENT_TIMESTAMP
   -- Resultado: 2026-06-07 12:34:56
   ```

3. **`requeue.go:81`** — Job de requeue compara com `datetime()`:
   ```sql
   datetime(updated_at) < datetime('now', '-N minutes')
   ```

A funcao `datetime()` do SQLite normaliza para o formato
`YYYY-MM-DD HH:MM:SS`. Se `updated_at` estiver em formato ISO8601
(`...T...Z`), a conversao via `datetime()` pode funcionar, mas e uma
dependencia implicita do parser do SQLite que nao esta documentada e
pode causar comportamento inesperado em versoes futuras ou com valores
edge-case.

## Impacto

- **Bug silencioso**: se `datetime()` falhar em parsear um formato
  inesperado, retorna NULL, e a comparacao `NULL < datetime(...)` e
  sempre falsa — vídeos travados nunca seriam detectados pelo requeue job.
- Dificuldade de debug: o problema so se manifesta quando um video fica
  travado E o formato de timestamp nao e parseado corretamente.

## Dev Instructions

### 1. Solucao (sera resolvida junto com T58)

Se T58 substituir o SQL direto em `Enqueue` por `models.UpdateStatus`,
o problema de formato desaparece automaticamente — `UpdateStatus` nao
seta `updated_at` explicitamente (delega ao DEFAULT do banco).

Se T58 for resolvida de outra forma, garantir que o formato de timestamp
usado em `updated_at` e consistente em TODAS as escritas:
- Ou sempre `CURRENT_TIMESTAMP`
- Ou sempre `strftime('%Y-%m-%dT%H:%M:%fZ', 'now')`
- E que o job de requeue use o parser correto para o formato escolhido

### 2. Verificar que a migration define DEFAULT consistente

Em `0001_init.sql`, verificar que `updated_at` tem:
```sql
updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
```

E que ha um trigger de UPDATE que atualiza automaticamente:
```sql
CREATE TRIGGER IF NOT EXISTS update_videos_updated_at
  AFTER UPDATE ON videos
  FOR EACH ROW
  BEGIN UPDATE videos SET updated_at = CURRENT_TIMESTAMP WHERE video_id = OLD.video_id; END;
```

Se o trigger existe, NENHUMA query deveria setar `updated_at` manualmente.

### 3. Verificacao

- `go test ./internal/transcode/...` — sem regressoes
- `go test ./internal/jobs/...` — sem regressoes

## Arquivos a editar

- `internal/transcode/queue.go` (remover `updated_at` do SQL se T58 nao resolver)
- Verificar `internal/db/migrations/` (trigger de updated_at)

## Resolução

Resolvida automaticamente pela T58: ao substituir o `db.Exec("UPDATE ... strftime(...)")`
por `models.UpdateStatus()`, a escrita manual de `updated_at` com formato RFC3339 foi
eliminada. Agora todas as escritas de `updated_at` delegam ao DEFAULT/trigger do banco.

## Definition of Done

- [x] Formato de timestamp e consistente em todas as escritas de `updated_at`
- [x] Job de requeue funciona corretamente com o formato usado
- [x] Nenhuma query seta `updated_at` manualmente se trigger existe
- [x] `go test ./...` passa
- [x] `go vet ./...` limpo
