# T26: Model + armazenamento de eventos de reprodução/upload (estatísticas)

**Status:** pending
**Dependências:** T03, T04
**Estimativa:** média
**Issue relacionada:** #2

## Contexto

A spec atual (`spec/ESPECIFICACAOv4.md`) **não** prevê armazenamento de
estatísticas de uso (confirmado por busca no documento — issue #2 pedia essa
verificação). Esta tarefa cria a fundação: uma tabela de eventos que registra,
de forma granular, cada solicitação relevante de upload e playback, para que
agregações futuras (T27, T28) possam ser calculadas sob demanda.

### O que registrar

Cada evento relevante (download de segmento HLS, acesso ao master playlist,
upload concluído) deve gerar um registro com:

- `video_id`
- `event_type` (`playback`, `download_segment`, `upload_complete`)
- `resolution` (480/720/1080, ou NULL para eventos sem resolução, ex. master.m3u8)
- `user_agent` (string crua do header `User-Agent`)
- `os_family` (derivado do user-agent: `ios`, `android`, `windows`, `macos`,
  `linux`, `other`) — parsing simples por substring, sem dependência externa
- `occurred_at` (timestamp do evento)

A partir de `occurred_at` é possível derivar data, hora e dia da semana via
`strftime()` do SQLite — não é necessário armazenar essas colunas
redundantemente.

### Por que uma tabela de eventos brutos (em vez de contadores agregados)

Contadores agregados (ex. `views_total`, `views_by_resolution`) não permitem
responder "quantos acessos por dia da semana" ou "qual SO predomina às
sextas-feiras à noite" sem granularidade temporal. A tabela de eventos brutos
permite qualquer agregação futura via `GROUP BY strftime(...)`.

### Volume e retenção

Não é objetivo desta tarefa implementar limpeza/retenção — isso pode ser uma
tarefa futura (job de limpeza similar ao T16). Apenas deixe um comentário no
código apontando essa necessidade futura.

## QA Instructions

Crie `internal/models/stats_test.go`:

```
TestRecordPlaybackEvent_Inserts
  - Chama RecordEvent(db, "vid1", "playback", &res480, "Mozilla/5.0 (iPhone...)")
  - Verifica que um registro foi inserido com os campos corretos
  - Verifica que os_family = "ios"

TestRecordEvent_NilResolution
  - Chama RecordEvent(db, "vid1", "playback", nil, "Mozilla/5.0 (Windows NT...)")
  - Verifica que resolution é NULL no banco
  - Verifica que os_family = "windows"

TestDetectOSFamily_VariousUserAgents
  - Testa strings de UA representativas de iOS, Android, Windows, macOS, Linux
  - Testa string vazia/desconhecida → "other"

TestCountEventsByType
  - Insere eventos de tipos variados
  - CountEventsByType(db, "playback") retorna contagem correta

TestAggregateByResolution
  - Insere eventos com resoluções variadas (incluindo NULL)
  - AggregateByResolution(db, videoID) retorna mapa resolução→contagem

TestAggregateByOS
  - Insere eventos com os_family variados
  - AggregateByOS(db) retorna mapa os→contagem

TestAggregateByDayOfWeek
  - Insere eventos com occurred_at em dias da semana conhecidos
  - AggregateByDayOfWeek(db) retorna mapa dia→contagem
  - Use strftime('%w', occurred_at) — 0=domingo .. 6=sábado
```

## Dev Instructions

### Migration

Adicione ao `internal/db/schema.go`:

```sql
CREATE TABLE IF NOT EXISTS playback_events (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  video_id     TEXT NOT NULL,
  event_type   TEXT NOT NULL,
  resolution   INTEGER,
  user_agent   TEXT,
  os_family    TEXT,
  occurred_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_playback_events_video ON playback_events(video_id);
CREATE INDEX IF NOT EXISTS idx_playback_events_occurred ON playback_events(occurred_at);
```

### Crie `internal/models/stats.go`

```go
// RecordEvent insere um evento de uso. resolution pode ser nil.
func RecordEvent(db *sql.DB, videoID, eventType string, resolution *int, userAgent string) error

// detectOSFamily classifica o user-agent em uma família de SO conhecida.
// Reconhece (nesta ordem de prioridade): iOS (iPhone/iPad/iPod), Android,
// Windows, macOS (Macintosh, sem "iPhone"), Linux, "other" como fallback.
func detectOSFamily(userAgent string) string

// CountEventsByType retorna o total de eventos de um tipo.
func CountEventsByType(db *sql.DB, eventType string) (int64, error)

// AggregateByResolution retorna contagem de eventos por resolução para um vídeo.
func AggregateByResolution(db *sql.DB, videoID string) (map[int]int64, error)

// AggregateByOS retorna contagem de eventos por família de SO (todos os vídeos).
func AggregateByOS(db *sql.DB) (map[string]int64, error)

// AggregateByDayOfWeek retorna contagem de eventos por dia da semana (0=domingo).
func AggregateByDayOfWeek(db *sql.DB) (map[int]int64, error)
```

Use `datetime(occurred_at)` / `strftime()` nas queries de agregação para evitar
o mesmo bug de comparação de formato de data já corrigido em T14/T16
(RFC3339 com `T` vs formato SQLite com espaço) — sempre normalize com
`datetime()`/`strftime()` em ambos os lados.

## Arquivos a criar/modificar

- `internal/db/schema.go` (nova tabela `playback_events`)
- `internal/models/stats.go`
- `internal/models/stats_test.go`

## Definition of Done

- [ ] Tabela `playback_events` criada via migration idempotente
- [ ] `RecordEvent` insere corretamente, incluindo resolução nula
- [ ] `detectOSFamily` classifica corretamente os principais SOs
- [ ] Funções de agregação retornam dados corretos por resolução, SO e dia da semana
- [ ] Nenhuma comparação de data sem `datetime()`/`strftime()`
- [ ] Todos os testes passam
