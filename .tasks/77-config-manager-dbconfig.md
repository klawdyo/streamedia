# T77 — Pacote config/dbconfig: gerenciador de configs com fallback para defaults

**Status:** done
**Depende de:** T75
**Issue relacionada:** — (parte do admin unificado, spec/admin-unificado.md §4.2-4.3)

## Objetivo

Criar o pacote `internal/config/dbconfig` que gerencia configurações dinâmicas
do banco com fallback para defaults definidos em código. Substitui as env vars
de configuração operacional (MAX_UPLOAD_SIZE_MB, QUEUE_MAX_SIZE, etc.) por
valores lidos de `configurations` com default local.

## QA Instructions

- Testar que `Get` retorna o valor do banco quando existe
- Testar que `Get` retorna o default quando a config não está no banco
- Testar `GetInt`, `GetBool`, `GetDuration` com parsing correto
- Testar que config do tipo `secret` (visible=0) não é exposta em `List`
- Testar que `Reload` recarrega do banco

## Dev Instructions

1. Criar `internal/config/dbconfig/dbconfig.go` com struct Manager
2. Manager mantém cache em memória (map[string]string) com fallback para defaults
3. Métodos: `NewManager(db, defaults)`, `Get(key)`, `GetInt(key)`, `GetBool(key)`, `GetDuration(key)`, `Reload()`
4. Defaults definidos como mapa estático no pacote (conforme spec §4.2)
5. Integrar no `internal/config.Config` para que o servidor leia do Manager em vez de env vars

## Definition of Done

- [x] `internal/config/dbconfig/dbconfig.go` implementado com cache + fallback
- [x] `internal/config/dbconfig/dbconfig_test.go` com cobertura de todos os métodos
- [x] Manager integrado no config.Config
- [x] `go build ./...` e `go test ./...` passam

## Resolução

**Data:** 2026-06-28
**Commit:** `1561a36` feat(T75-T77): migration users/roles/configs, modelos Go e config manager com fallback

Arquivos criados:
- `internal/config/dbconfig/dbconfig.go`: Manager com cache em `map[string]string`. `NewManager` recebe `*sql.DB` + mapa de defaults. Métodos `Get(key) string`, `GetInt(key) int`, `GetBool(key) bool`, `GetDuration(key) time.Duration`. `Reload()` recarrega do banco. Configs `visible=0` nunca expostas em `List()`.
- `internal/config/dbconfig/dbconfig_test.go`: testes table-driven para Get/fallback, GetInt, GetBool, GetDuration, Reload, e filtro de secrets.

Defaults definidos conforme spec §4.2: 16 configurações cobrindo paths, upload, transcode, token, rate_limit, webhook, discord, session. Integração com config.Config feita em T82 (server wire).
