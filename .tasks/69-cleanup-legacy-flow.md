# T69 — Limpeza total do fluxo legado (projects / HMAC / scoped)

**Status:** done
**Origem:** solicitação direta do usuário (pós-refactor tag + ROOT_TOKEN).
**Depende de:** o refactor do modelo de tag + ROOT_TOKEN (commit `feat!: modelo de tag + ROOT_TOKEN único`).

## Objetivo

Não deve sobrar **nada** que lembre o fluxo antigo (projetos, chaves mestras,
`X-Project-Key`, `X-Upload-Auth`, `X-Status-Auth`, `UPLOAD_TOKEN_SECRET`,
`ADMIN_TOKEN`, tokens de play HMAC, sufixo `_SECONDS`): código morto, variáveis
não usadas, comentários, exemplos e a especificação OpenAPI. Quem quiser o
fluxo antigo consulta o histórico do git.

## Escopo

- **OpenAPI (`internal/docs/spec.go`)**: reescrever para o fluxo atual —
  `POST /api/upload/init` (Bearer ROOT_TOKEN + `tag`), `POST /api/play/init`,
  `GET /video/{tag}/{video_id}.m3u8` + estáticos, `GET /api/status/{video_id}`
  (Bearer), `/admin/videos`, `/admin/queue`, `/admin/stats`,
  `DELETE /admin/videos/{video_id}`. Remover todas as rotas `/admin/projects*`,
  `X-Project-Key`, `master_key`, menções a HMAC de play e a `/videos/`.
- **Telemetria (`internal/telemetry/middleware.go`)**: atualizar o template de
  rota (`/videos/...` → `/video/{tag}/...`) usado nas labels de métrica.
- **Comentários de código**: remover referências a issue #6/#10, T32–T35,
  T48–T50, "projeto", "chave mestra", "legado/legacy", "scoped/escopado" em
  todos os pacotes de produção.
- **Testes**: remover nomes/paths legados (`X-Project-Key inválida`, `/videos/`)
  em `response_conformance_test.go`, `status_test.go`, etc.; manter cobertura
  equivalente no fluxo novo.
- **`internal/ci/ci_test.go`**: a asserção que verifica vazamento de segredo no
  workflow de release deve referir-se a `ROOT_TOKEN` (não mais `UPLOAD_TOKEN_SECRET`).
- **Código morto / vars não usadas**: `go vet ./...` + revisão manual.

## Definition of Done

- [x] Nenhuma ocorrência de `X-Project-Key`, `X-Upload-Auth`, `X-Status-Auth`,
      `UPLOAD_TOKEN_SECRET`, `ADMIN_TOKEN`, `master_key`, `project_id`,
      `GeneratePlayToken` em código/spec de produção.
- [x] `internal/docs/spec.go` descreve apenas o fluxo atual.
- [x] Telemetria normaliza as rotas `/video/...` corretamente.
- [x] `go build ./...` e `go vet ./...` limpos.
- [x] Suíte verde (exceto falhas pré-existentes de Windows não relacionadas).

## Resolução

- `internal/docs/spec.go` reescrito: OpenAPI descreve só o fluxo atual
  (`/api/upload/init` com Bearer + tag, `/api/play/init`, `/video/<tag>/<id>.m3u8`,
  `/api/status`, `/admin/*` incl. `DELETE /admin/videos/{id}`). Removidas todas
  as rotas `/admin/projects*`, `X-Project-Key`, `master_key`, `adminToken`/
  `projectKey` securitySchemes → `rootToken`. `docs_test.go` ajustado.
- `internal/telemetry/middleware.go`: comentários de exemplo de rota
  `/videos/...` → `/video/{tag}/...` (o código já era genérico via chi).
- Testes: `response_conformance_test.go` (casos de erro e paths HLS para o
  novo modelo), `config_test.go` (removido `TestLoad_OldTimeVarNamesAreIgnored`,
  limpos comentários "issue #4"), `ci_test.go` (asserção agora sobre `ROOT_TOKEN`).
- `go build ./...` e `go vet ./...` limpos; suíte verde exceto as 3 falhas
  pré-existentes de Windows (transcode/upload) — idênticas na `main`.
