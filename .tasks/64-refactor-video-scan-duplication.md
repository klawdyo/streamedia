# T64: Extrair `scanVideoRow` — codigo de scan duplicado 3 vezes

**Status:** done
**Dependências:** T04, T18
**Estimativa:** media
**Origem:** análise de código — codigo repetido
**Severidade:** media

## Contexto

A logica de scan de um `Video` a partir de uma row do banco — declarar
`sql.NullInt64/NullString/NullTime`, chamar `Scan`, converter para os
tipos da struct — aparece identica em 3 lugares:

1. **`models/video.go:98-150`** — `GetVideo` (query single row)
2. **`models/video.go:277-326`** — `ListByStatus` (query multi row)
3. **`admin/admin.go:184-236`** — `HandleVideos` (query multi row)

Sao ~40 linhas de codigo identico repetidas 3x. Qualquer mudanca na
struct `Video` (ex.: novo campo) precisa ser replicada em 3 lugares — e
a historia do projeto mostra que isso ja causou bug real (T53: `ListByStatus`
omitia `project_id` porque esqueceram de replicar a mudanca de `GetVideo`).

## Impacto

- **Risco de divergencia**: adicionar campo em um lugar e esquecer nos
  outros ja aconteceu (T53).
- **Manutencao**: qualquer alteracao na struct Video exige editar 3 funcoes.
- **Volume de codigo**: ~120 linhas que poderiam ser ~40.

## Dev Instructions

### 1. Criar funcao helper `scanVideoRow`

Em `internal/models/video.go`:

```go
// scanVideoRow le uma linha de Video do banco, tratando campos nullable.
// Aceita qualquer funcao que implemente a interface de Scan (sql.Row e
// sql.Rows compartilham a mesma assinatura).
func scanVideoRow(scan func(dest ...any) error) (*Video, error) {
    var (
        v            Video
        declaredSize sql.NullInt64
        actualSize   sql.NullInt64
        durationS    sql.NullInt64
        resolutions  sql.NullString
        lastChunkAt  sql.NullTime
        errorMessage sql.NullString
        projectID    sql.NullInt64
    )

    err := scan(
        &v.VideoID, &v.Status,
        &declaredSize, &actualSize, &durationS, &resolutions,
        &v.TranscodeAttempts, &lastChunkAt, &errorMessage,
        &projectID, &v.CreatedAt, &v.UpdatedAt,
    )
    if err != nil {
        return nil, err
    }

    v.DeclaredSizeBytes = declaredSize.Int64
    v.ActualSizeBytes = actualSize.Int64
    v.DurationS = int(durationS.Int64)
    v.ErrorMessage = errorMessage.String
    if projectID.Valid {
        v.ProjectID = &projectID.Int64
    }
    if lastChunkAt.Valid {
        t := lastChunkAt.Time
        v.LastChunkAt = &t
    }
    if resolutions.Valid && resolutions.String != "" {
        if err := json.Unmarshal([]byte(resolutions.String), &v.Resolutions); err != nil {
            return nil, fmt.Errorf("erro ao deserializar resolutions: %w", err)
        }
    } else {
        v.Resolutions = []int{}
    }

    return &v, nil
}
```

### 2. Refatorar os 3 call sites

- `GetVideo`: `return scanVideoRow(row.Scan)`
- `ListByStatus`: `v, err := scanVideoRow(rows.Scan)`
- `HandleVideos` em `admin.go`: `v, err := models.ScanVideoRow(rows.Scan)`
  (exportar como `ScanVideoRow` se admin precisa acessar)

### 3. Definir constante para colunas SELECT

```go
const selectVideoColumns = `video_id, status, declared_size_bytes, actual_size_bytes,
    duration_s, resolutions, transcode_attempts, last_chunk_at,
    error_message, project_id, created_at, updated_at`
```

Reusar em `GetVideo`, `ListByStatus` e `HandleVideos`.

### 4. Verificacao

- `go test ./internal/models/...` — todos os testes existentes passam
- `go test ./internal/admin/...` — testes de HandleVideos passam
- `go test ./...` — sem regressoes

## Arquivos a editar

- `internal/models/video.go` (criar scanVideoRow, refatorar GetVideo e ListByStatus)
- `internal/admin/admin.go` (usar scanVideoRow em HandleVideos)

## Resolução

Arquivos alterados:
- `internal/models/video.go`: criada `ScanVideoRow` (exportada) e constante
  `SelectVideoColumns`. `GetVideo` e `ListByStatus` refatorados para usá-las.
  ~80 linhas de scan duplicado removidas.
- `internal/admin/admin.go`: `HandleVideos` usa `models.ScanVideoRow(rows.Scan)`
  e `models.SelectVideoColumns`. Import de `encoding/json` removido (não mais necessário).

## Definition of Done

- [x] Funcao `scanVideoRow` (ou `ScanVideoRow` exportada) criada
- [x] `GetVideo` usa `scanVideoRow`
- [x] `ListByStatus` usa `scanVideoRow`
- [x] `HandleVideos` usa `scanVideoRow` (via export se necessario)
- [x] Constante de colunas SELECT centralizada e reusada
- [x] Nenhuma duplicacao de logica de scan restante
- [x] `go test ./...` passa
- [x] `go vet ./...` limpo
