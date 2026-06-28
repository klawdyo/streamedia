# T78 — Google OAuth2: fluxo login/callback + session cookie com user_id e roles

**Status:** done
**Depende de:** T76
**Issue relacionada:** — (parte do admin unificado, spec/admin-unificado.md §5.1-5.2)

## Objetivo

Implementar o fluxo Google OAuth2: login via `/api/auth/google`, callback que
troca code por token e emite session cookie stateless (HMAC assinado com
ROOT_TOKEN), rota `/api/auth/me` que devolve dados do usuário logado, e
bootstrapping do primeiro login (se tabela users vazia, primeiro login recebe
role `dev` automaticamente).

## QA Instructions

- Testar que GET `/api/auth/google` redireciona para Google OAuth
- Testar que callback sem code retorna erro
- Testar que callback com code inválido retorna erro
- Testar que primeiro login (tabela vazia) cria usuário com role `dev`
- Testar que login subsequente de email não cadastrado é rejeitado
- Testar que `/api/auth/me` retorna email, name, picture, roles[], effective_level
- Testar que DELETE `/api/auth/session` limpa o cookie
- Testar que cookie é HMAC-válido e expira corretamente

## Dev Instructions

1. Criar `internal/auth/google/google.go` com handlers OAuth2
2. Configurar `golang.org/x/oauth2` com Google endpoints
3. Session cookie: formato `<exp_unix>.<user_id>.<roles_csv>.<hmac_hex>`, assinado com ROOT_TOKEN
4. Bootstrapping: se `CountUsers() == 0`, primeiro login aceito automaticamente com role `dev`
5. Após primeiro usuário, só emails em `users` podem logar

## Definition of Done

- [x] `internal/auth/google/google.go` com handlers Login, Callback, Me, Logout
- [x] `internal/auth/google/google_test.go` com cobertura do fluxo OAuth + session
- [x] Session cookie stateless com HMAC-SHA256 assinado por ROOT_TOKEN
- [x] Bootstrapping primeiro login funcional
- [x] Novas env vars: GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET, GOOGLE_REDIRECT_URL
- [x] `go build ./...` e `go test ./...` passam

## Resolução

**Data:** 2026-06-28
**Commit:** `b66179e` feat(T78): Google OAuth2 flow + session cookie com user_id e roles

Arquivos criados/modificados:
- `internal/auth/google/google.go`: Handlers `HandleLogin` (redirect Google), `HandleCallback` (troca code→token, valida email, cria usuário no bootstrapping, emite cookie), `HandleMe` (parse cookie, retorna user+roles), `HandleLogout` (clear cookie). Cookie formato `<exp>.<user_id>.<roles_csv>.<hmac>` stateless.
- `internal/auth/google/google_test.go`: Testes do fluxo OAuth com mock HTTP server.
- `internal/auth/auth.go`: Tipos compartilhados de sessão (Session, SessionClaims).
- `internal/config/config.go`: Adicionadas env vars `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `GOOGLE_REDIRECT_URL`, `SESSION_COOKIE_SECURE`.

Decisão: cookie stateless (sem query no DB por request) assinado com ROOT_TOKEN via HMAC-SHA256. Bootstrapping: `CountUsers() == 0` → primeiro login automático com role `dev`. Após isso, whitelist por email.
