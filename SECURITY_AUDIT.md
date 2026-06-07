# Relatório de Auditoria de Segurança — Streamedia

**Origem:** Issue #8 — T41 (autenticação, autorização e tokens)
**Data:** 2026-06-07

## Sumário executivo

Auditoria do perímetro de autenticação e autorização. Nenhuma falha crítica encontrada. 
O código segue boas práticas de segurança (HMAC em tempo constante, escopo de token 
vinculado ao recurso, sem fallback inseguro). Duas observações de baixa severidade.

## Checklist de investigação

### 1. Comparação de assinaturas/tokens

| Local | Método | Status |
|-------|--------|--------|
| `internal/auth/auth.go:36` | `hmac.Equal` | ✅ Seguro |
| `internal/auth/auth.go:77` | `hmac.Equal` | ✅ Seguro |
| `internal/auth/auth.go:104` | `hmac.Equal` | ✅ Seguro |
| `internal/admin/admin.go:83` | `subtle.ConstantTimeCompare` | ✅ Seguro |

**Conclusão:** Todas as comparações usam algoritmos de tempo constante. Nenhuma 
comparação com `==` ou `bytes.Equal`. Sem vulnerabilidade a timing attacks.

---

### 2. Replay attacks

O `ValidatePlayToken` (`auth.go:51-82`) implementa três verificações em ordem:
1. `now.After(expiresAt)` — expiração
2. `expiresAt.After(now.Add(maxTTL))` — TTL máximo
3. HMAC do payload `"{video_id}:{expires_unix}"`

O token é vinculado ao video_id e ao timestamp de expiração no payload assinado. 
Uma URL capturada não pode ser reutilizada após expirar, nem para um vídeo diferente.

**Observação (baixa):** `time.Now()` usa o relógio local do servidor. Clock skew entre 
o backend principal (que gera o token) e o Streamedia pode causar janela de replay 
curta (se o Streamedia estiver atrasado) ou rejeição prematura (se estiver adiantado). 
Mitigação natural: o TTL padrão (6h) é muito maior que o skew típico (<1s).

**Conclusão:** ✅ Protegido contra replay. Clock skew é risco aceitável.

---

### 3. Geração de segredo/chave

| Segredo | Origem | Obrigatório? | Fallback inseguro? |
|---------|--------|-------------|-------------------|
| `UPLOAD_TOKEN_SECRET` | env var | Sim | Não |
| `WEBHOOK_SECRET` | env var | Sim | Não |
| `ADMIN_TOKEN` | env var | Não | Não (vazio = admin desabilitado) |

Se `ADMIN_TOKEN` estiver vazio, o branch super-admin em `AdminAuth` é pulado — 
ninguém autentica como super-admin. Isso é seguro (admin desabilitado por 
configuração, não por fallback hardcoded).

**Conclusão:** ✅ Segredos bem gerenciados. Sem fallback inseguro.

---

### 4. Escopo do token / IDOR

| Token | Payload assinado | Vinculado a |
|-------|-----------------|-------------|
| Upload | `videoID` | vídeo específico |
| Play | `"{video_id}:{expires_unix}"` | vídeo específico + expiração |
| Backend | body da requisição | requisição específica |

O `ValidatePlayToken` recalcula o HMAC com o `videoID` do path da URL. Se um 
atacante tentar usar um play token do vídeo A na URL do vídeo B, o HMAC não 
baterá (porque o videoID no payload assinado é diferente).

**Conclusão:** ✅ Escopo correto. IDOR não é possível via token.

---

### 5. Rotas admin — bypass

**Rotas protegidas por `AdminAuth`** (grupo chi em `server.go:115-127`):
- `GET /admin/videos`
- `GET /admin/queue`
- `GET /admin/stats`
- `POST /admin/projects`
- `GET /admin/projects`
- `GET /admin/projects/{slug}`

**Rota fora do grupo** (`server.go:134`):
- `POST /admin/projects/{slug}/upload-tokens` — autenticada por `X-Project-Key` 
  (chave mestra do projeto), NÃO por `AdminAuth`. Documentado como decisão 
  intencional de design (T33/T35).

Não foram encontradas rotas admin desprotegidas. O middleware `AdminAuth` cobre 
todos os métodos (não apenas GET). O chi aplica o middleware a qualquer método 
registrado no grupo.

**Conclusão:** ✅ Rotas protegidas. A exceção é documentada e intencional.

---

### 6. Mensagens de erro — vazamento de informação

| Função | Mensagem | Revela? |
|--------|----------|---------|
| `ValidatePlayToken` (auth.go:57) | "Token de reprodução expirado." | Sim — token era válido |
| `ValidatePlayToken` (auth.go:64) | "Token de reprodução excede o tempo máximo permitido." | Sim — TTL excessivo |
| `ValidatePlayToken` (auth.go:71,75,78) | "Assinatura do token de reprodução inválida." | Não — genérico |
| `AdminAuth` (admin.go:68,75,100) | "Unauthorized" | Não — genérico |

**Falha (baixa):** Um atacante pode distinguir "token expirado" de "token 
inválido/forjado". Isso permite enumeração de tokens válidos no passado — 
um atacante sabe que aquele token já foi válido, o que pode ajudar em 
ataques de brute-force ou engenharia social.

**Mitigação:** Unificar as três mensagens de erro em `ValidatePlayToken` 
para uma única mensagem genérica: "Token de reprodução inválido."

**Severidade:** Baixa — o token já expirou e não pode ser reutilizado.

---

### 7. Expiração e limpeza

| Local | Comparação | Status |
|-------|-----------|--------|
| `token.go:22` (`IsExpired`) | `time.Now().After(t.ExpiresAt)` | ✅ |
| `token.go:101` (`DeleteExpiredTokens`) | `datetime(expires_at) < datetime('now')` | ✅ |
| `token.go:57` (armazenamento) | `.UTC().Format("2006-01-02 15:04:05")` | ✅ |
| `cleanup.go:38` (job) | Ticker 24h, erro logado | ✅ |

UTC usado consistentemente. `datetime()` do SQLite é usado na query de deleção 
de forma consistente com o formato de armazenamento. O job de limpeza loga erros 
e tenta novamente no próximo tick.

**Conclusão:** ✅ Sem bugs de comparação de data. Sem timezone issues.

---

## Falhas encontradas

### F-01: Mensagens de erro distinguem token expirado de token inválido

- **Local:** `internal/auth/auth.go:56-65` (`ValidatePlayToken`)
- **Falha:** Três mensagens de erro diferentes para três condições de falha 
  (expirado, TTL excessivo, HMAC inválido)
- **Exploração:** Atacante captura uma URL de master.m3u8, modifica o parâmetro 
  `expires` para o passado, e observa se recebe "Token de reprodução expirado" 
  (confirma que o token era válido) ou "Assinatura inválida" (token nunca foi 
  válido). Permite enumerar tokens válidos.
- **Mitigação:** Responder com uma única mensagem genérica para qualquer falha 
  de validação de play token.
- **Severidade:** Baixa
- **Status:** Corrigida nesta tarefa
