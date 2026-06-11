# T73 — URL de webhook customizada por vídeo no POST /api/upload/init

- **Status:** done
- **Issue relacionada:** #20
- **Depende de:** T08 (upload/init), T17 (cliente de webhook), T52 (migrations)

## Objetivo

Permitir que o backend principal especifique uma URL de webhook diferente por
vídeo no momento do `upload/init`. Quando informada (e válida), os webhooks
daquele vídeo são enviados para essa URL em vez da `WEBHOOK_URL` global; caso
contrário, usa a global (comportamento histórico).

## Escopo (da issue #20)

- Campo opcional `webhook_url` no corpo de `POST /api/upload/init`.
- URL customizada persistida no banco (`videos.webhook_url`) — sobrevive a
  crash/restart, pois o worker de webhook a relê do banco.
- A assinatura HMAC (`WEBHOOK_SECRET`) permanece a mesma, independente do destino.

## Validação

- HTTPS obrigatório.
- Formato de URL válido (absoluta, com host).
- Máximo de 2048 caracteres.

## Fora de escopo

- Múltiplos webhooks por vídeo.
- Webhook secret customizado por vídeo.

## Decisões de implementação

1. **`webhook_url` informado-mas-inválido → `400`** (não fallback silencioso).
   A issue diz "se omitido ou inválido, usa WEBHOOK_URL", mas a seção de
   Validação lista regras explícitas. Adotamos: **omitido/vazio** → fallback
   à global (sem erro); **informado mas malformado** → `400`. Em cenário
   multi-tenant, cair na URL global por engano vazaria eventos de um tenant
   para o destino global — rejeitar é mais seguro e mais depurável.
2. **Coluna nova via migration `0002_video_webhook_url.sql`** — `webhook_url
   TEXT NOT NULL DEFAULT ''`. O default `''` significa "usar a URL global" e
   dispensa migração de dados (vídeos antigos continuam válidos).
3. **Resolução do destino centralizada no `webhook.Client.resolveURL`** (ponto
   único já existente, extraído na T17 justamente para isto). Faz lookup do
   vídeo; se `v.WebhookURL != ""`, usa-a; senão cai na global. Falha de lookup
   não é fatal — cai na global.
4. **`SelectVideoColumns` + `ScanVideoRow` como fonte única.** A coluna foi
   adicionada nos dois lugares (mesma ordem), mantendo `GetVideo`,
   `ListByStatus` e `admin.HandleVideos` consistentes sem tocar cada query.
5. **`InsertVideoWithTagAndWebhook`** nova; `InsertVideoWithTag` delega a ela
   com `""` — zero churn nos chamadores existentes.

## Arquivos alterados

- `internal/db/migrations/0002_video_webhook_url.sql` (novo) — `ALTER TABLE`.
- `internal/models/video.go` — campo `WebhookURL`, `SelectVideoColumns`,
  `ScanVideoRow`, `InsertVideoWithTagAndWebhook`.
- `internal/upload/init.go` — campo `webhook_url` em `initRequest`,
  `validateWebhookURL`, persistência via novo insert.
- `internal/webhook/webhook.go` — `resolveURL` prioriza a URL por vídeo.
- Testes: `internal/upload/init_test.go`, `internal/webhook/webhook_test.go`.
- Docs: `spec/api.md`, `spec/webhooks.md`, `spec/dados.md`,
  `internal/docs/spec.go` (OpenAPI), `api.http`, `.env.example`, `README.md`.

## Definition of Done

- [x] `webhook_url` aceito e validado (HTTPS, formato, ≤ 2048).
- [x] Persistido em `videos.webhook_url` e relido pelo cliente de webhook.
- [x] Override por vídeo prevalece sobre a global; fallback à global quando vazio.
- [x] Assinatura HMAC inalterada.
- [x] Testes cobrindo validação, persistência, override e fallback.
- [x] Documentação atualizada (spec, OpenAPI, .env, README, api.http).

## Resolução

Implementado conforme as decisões acima. Migration `0002` aplica a coluna
idempotentemente no boot (goose). A validação rejeita não-HTTPS, URLs
relativas, esquemas estranhos e strings > 2048; espaços nas bordas são
removidos. `resolveURL` faz o override por vídeo. Suíte do pacote `webhook`
(incluindo os novos testes de override/fallback) e do `upload` (validação +
persistência) passam. Build e `go vet` limpos.
