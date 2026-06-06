# T02: Pacote de configuração

**Status:** pending
**Dependências:** T01
**Estimativa:** pequena

## Contexto

O serviço carrega toda sua configuração de variáveis de ambiente. Três variáveis
são obrigatórias e causam falha na inicialização se ausentes:
`UPLOAD_TOKEN_SECRET`, `WEBHOOK_URL`, `WEBHOOK_SECRET`.

As demais têm valores padrão sensatos. O pacote `config` lê as variáveis na
inicialização e expõe uma struct `Config` usada por todos os outros pacotes.

## Variáveis de ambiente completas

| Variável | Tipo | Padrão | Obrigatória |
|----------|------|---------|-------------|
| `UPLOAD_TOKEN_SECRET` | string | — | sim |
| `WEBHOOK_URL` | string | — | sim |
| `WEBHOOK_SECRET` | string | — | sim |
| `MAX_UPLOAD_SIZE_MB` | int | 10 | não |
| `MEDIA_DIR` | string | /media | não |
| `UPLOAD_TMP_DIR` | string | /media/.uploads | não |
| `SQLITE_PATH` | string | /data/media.db | não |
| `QUEUE_MAX_SIZE` | int | 50 | não |
| `TRANSCODE_WORKERS` | int | 1 | não |
| `UPLOAD_TOKEN_TTL_H` | int | 6 | não |
| `PLAY_TOKEN_MAX_TTL_H` | int | 6 | não |
| `UPLOAD_IDLE_TIMEOUT_MIN` | int | 10 | não |
| `TRANSCODE_STUCK_MIN` | int | 30 | não |
| `MAX_TRANSCODE_ATTEMPTS` | int | 3 | não |
| `KEEP_ORIGINAL` | bool | false | não |
| `PORT` | int | 3000 | não |
| `RATE_LIMIT_PER_MIN` | int | 60 | não |

## QA Instructions

Crie `internal/config/config_test.go` com os seguintes casos:

```
TestLoad_RequiredVarsMissing
  - Chama Load() sem nenhuma variável de ambiente setada
  - Espera error não-nulo (falha ao iniciar sem secrets)

TestLoad_RequiredVarsPresent
  - Seta UPLOAD_TOKEN_SECRET, WEBHOOK_URL, WEBHOOK_SECRET
  - Chama Load()
  - Espera error nil
  - Espera que os valores foram carregados corretamente

TestLoad_Defaults
  - Seta apenas as vars obrigatórias
  - Chama Load()
  - Verifica: MaxUploadSizeMB == 10
  - Verifica: QueueMaxSize == 50
  - Verifica: TranscodeWorkers == 1
  - Verifica: Port == 3000
  - Verifica: KeepOriginal == false

TestLoad_OverrideDefaults
  - Seta as vars obrigatórias + MAX_UPLOAD_SIZE_MB=500 + TRANSCODE_WORKERS=4
  - Chama Load()
  - Verifica que os valores foram sobrescritos corretamente

TestLoad_InvalidInt
  - Seta MAX_UPLOAD_SIZE_MB="nao_e_numero"
  - Espera error informativo

TestLoad_MissingUploadSecret
  - Seta WEBHOOK_URL e WEBHOOK_SECRET mas não UPLOAD_TOKEN_SECRET
  - Espera error mencionando UPLOAD_TOKEN_SECRET
```

Use `t.Setenv` para setar variáveis nos testes (restaura automaticamente).

## Dev Instructions

Crie `internal/config/config.go`:

### Struct Config

```go
type Config struct {
    UploadTokenSecret    string
    WebhookURL           string
    WebhookSecret        string
    MaxUploadSizeBytes   int64  // convertido de MB
    MediaDir             string
    UploadTmpDir         string
    SQLitePath           string
    QueueMaxSize         int
    TranscodeWorkers     int
    UploadTokenTTL       time.Duration
    PlayTokenMaxTTL      time.Duration
    UploadIdleTimeout    time.Duration
    TranscodeStuckTime   time.Duration
    MaxTranscodeAttempts int
    KeepOriginal         bool
    Port                 int
    RateLimitPerMin      int
}
```

### Função Load

```go
func Load() (*Config, error)
```

- Lê cada variável com `os.Getenv`
- Para vars com padrão: usa o padrão se variável vazia
- Para vars obrigatórias: retorna erro descritivo em português se ausentes
- Converte MB → bytes: `MaxUploadSizeBytes = int64(mb) * 1024 * 1024`
- Converte horas/minutos para `time.Duration`
- Valida que inteiros são positivos

### Função helper privada

```go
func getEnvInt(key string, defaultVal int) (int, error)
func getEnvBool(key string, defaultVal bool) bool
func getEnvStr(key, defaultVal string) string
```

## Arquivos a criar/modificar

- `internal/config/config.go` (substituir doc.go)
- `internal/config/config_test.go`

## Definition of Done

- [ ] Struct `Config` com todos os campos
- [ ] `Load()` falha se obrigatórias ausentes (com mensagem em português)
- [ ] `Load()` usa defaults corretos para opcionais
- [ ] Todos os testes passam
- [ ] `go vet ./...` limpo
