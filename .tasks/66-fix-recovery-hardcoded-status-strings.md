# T66: Substituir strings literais de status em `recovery.go` por constantes

**Status:** done
**Dependências:** T21, T04
**Estimativa:** pequena
**Origem:** análise de código — fragilidade de manutencao
**Severidade:** media

## Contexto

Em `internal/transcode/recovery.go:59,66`, `RunStartupRecovery` usa strings
literais em vez das constantes definidas em `models/video.go`:

```go
if v.status == "upload_complete" {      // linha 59
} else if v.status == "transcoding" {   // linha 66
```

E na query SQL (linha 26):
```sql
WHERE status IN ('transcoding', 'upload_complete')
```

O pacote `models` define `StatusUploadComplete` e `StatusTranscoding` como
constantes tipadas (`VideoStatus`). Usar strings literais:
1. Nao gera erro de compilacao se o valor da constante mudar.
2. Nao aparece em buscas por referencia ("Find All References" na IDE).
3. E propenso a typo (ex.: `"upload_completed"` passaria despercebido).

## Impacto

- **Fragilidade**: se as constantes mudarem de valor, o recovery
  silenciosamente para de funcionar (nenhum video seria detectado).
- Risco baixo (constantes de status raramente mudam), mas o fix e trivial.

## Dev Instructions

### 1. Substituir strings literais por constantes

```go
// Na query SQL
WHERE status IN (?, ?)
// ... com args:
models.StatusTranscoding, models.StatusUploadComplete

// Nas comparacoes
if v.status == string(models.StatusUploadComplete) {
} else if v.status == string(models.StatusTranscoding) {
```

Nota: como `v.status` e `string` (struct local `videoRecord`), a comparacao
precisa de cast explicito ou mudar o tipo do campo para `models.VideoStatus`.

### 2. Verificacao

- `go test ./internal/transcode/...` — sem regressoes
- `go vet ./...` — sem warnings

## Arquivos a editar

- `internal/transcode/recovery.go` (substituir strings por constantes)

## Resolução

Arquivos alterados:
- `internal/transcode/recovery.go`: query SQL agora usa `?` com
  `models.StatusTranscoding` e `models.StatusUploadComplete`. Comparações
  usam `string(models.StatusUploadComplete)` e `string(models.StatusTranscoding)`.

## Definition of Done

- [x] Nenhuma string literal de status no arquivo
- [x] Query SQL usa parametros `?` com constantes
- [x] Comparacoes usam constantes tipadas
- [x] `go test ./...` passa
- [x] `go vet ./...` limpo
