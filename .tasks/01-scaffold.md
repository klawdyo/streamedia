# T01: Scaffold do projeto Go

**Status:** pending
**Dependências:** nenhuma
**Estimativa:** pequena

## Contexto

O projeto é um media server Go chamado `streamedia`. É um único binário, um único
container. A estrutura de pacotes deve ser criada agora para que as tarefas
seguintes possam adicionar código nos lugares certos.

O binário é compilado com `CGO_ENABLED=0` (driver SQLite em Go puro).
O entrypoint é `cmd/server/main.go`.

## Estrutura alvo de pacotes

```
streamedia/
├── cmd/
│   └── server/
│       └── main.go          ← entrypoint (só chama internal/server.Run)
├── internal/
│   ├── config/              ← variáveis de ambiente (T02)
│   ├── db/                  ← SQLite, schema, queries (T03)
│   ├── models/              ← Video, UploadToken, WebhookLog (T04, T05)
│   ├── auth/                ← HMAC utilities (T06)
│   ├── upload/              ← TUS handler, /upload/init (T07, T08, T09)
│   ├── transcode/           ← fila, workers, FFmpeg (T10, T11)
│   ├── serve/               ← HLS serving (T12)
│   ├── jobs/                ← killer, requeue, cleanup (T14, T15, T16)
│   ├── webhook/             ← client, retry, log (T17)
│   ├── admin/               ← rotas admin (T18)
│   └── middleware/          ← rate limit, auth (T19)
├── .tasks/                  ← não é código Go, ignorar no build
├── .agents/                 ← não é código Go, ignorar no build
├── spec/                    ← não é código Go, ignorar no build
├── go.mod
├── go.sum
├── .gitignore
└── CLAUDE.md
```

## QA Instructions

Escreva `cmd/server/main_test.go` com:

```
TestMainPackageExists
  - Verifica que o pacote main existe e compila
  - (teste trivial, só garante que o scaffold compila)
```

Escreva `internal/config/config_test.go` com um placeholder:

```
TestConfigPlaceholder
  - Apenas verifica que o pacote config existe
  - t.Log("pacote config existe") — sempre passa
```

O objetivo dos testes nesta tarefa é apenas confirmar que o scaffold compila.
`go build ./...` e `go test ./...` devem passar.

## Dev Instructions

### 1. Inicializar go.mod

```
module github.com/klawdyo/streamedia

go 1.23
```

### 2. Adicionar dependências

```bash
go get github.com/go-chi/chi/v5
go get github.com/tus/tusd/v2
go get modernc.org/sqlite
go get golang.org/x/time
```

### 3. Criar cmd/server/main.go

```go
package main

func main() {
    // Ponto de entrada: inicializa config, banco, servidor
    // Implementação real virá em T20 (server assembly)
    // Por enquanto, apenas garante que o binário compila
}
```

### 4. Criar internal/config/config.go com stub vazio

```go
package config
// Implementação real na T02
```

### 5. Criar stub vazio para cada pacote interno

Cada pacote em `internal/` precisa de pelo menos um arquivo `.go` com
`package nomepacote` para que `go build ./...` não falhe por pacotes ausentes
quando outros pacotes os importarem nas tarefas seguintes.

Crie `doc.go` em cada subpacote:
```go
// Package config gerencia as variáveis de ambiente do serviço.
package config
```

### 6. Criar .gitignore

Conteúdo exato:
```
# Ambiente
.env

# Binários
/mediaserver
*.exe

# Banco e dados locais de dev
*.db
*.db-wal
*.db-shm
/data/
/media/

# Go
/vendor/

# Editor
.idea/
.vscode/
.DS_Store
```

## Arquivos a criar

- `go.mod`
- `go.sum` (gerado automaticamente)
- `cmd/server/main.go`
- `internal/config/doc.go`
- `internal/db/doc.go`
- `internal/models/doc.go`
- `internal/auth/doc.go`
- `internal/upload/doc.go`
- `internal/transcode/doc.go`
- `internal/serve/doc.go`
- `internal/jobs/doc.go`
- `internal/webhook/doc.go`
- `internal/admin/doc.go`
- `internal/middleware/doc.go`
- `.gitignore`
- `cmd/server/main_test.go`
- `internal/config/config_test.go`

## Definition of Done

- [ ] `go build ./...` — sem erros
- [ ] `go test ./...` — passa
- [ ] `go vet ./...` — sem warnings
- [ ] Estrutura de diretórios criada conforme especificado
- [ ] .gitignore presente
