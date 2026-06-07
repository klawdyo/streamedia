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

---

## T42: Upload, validação e execução de processos (FFmpeg)

### 1. Path traversal

Todo `video_id` usado em paths de arquivo é validado como UUID estrito via regex 
(`uuidV4Re`) antes de tocar o filesystem — em `init.go:91`, `serve.go:111`, 
`status.go:57`. O caminho do arquivo de upload é montado via 
`filepath.Join(cfg.UploadTmpDir, videoID)`, onde `videoID` é UUID-validado.

**Conclusão:** ✅ Seguro. Nenhum ponto de concatenação direta de input não validado.

### 2. Command injection no FFmpeg

`buildFFmpegArgs` (`worker.go:144-167`) monta argumentos a partir de literais Go 
(birate strings, codec strings, paths construídos com `filepath.Join`). Nenhum 
dado do usuário (nome de arquivo, metadados) flui para os argumentos. 
`exec.Command` é usado (sem shell), prevenindo shell injection.

**Conclusão:** ✅ Seguro. Sem command injection possível.

### 3. Validação de tipo de arquivo

`validateMagicBytes` (`validation.go:30-74`) inspeciona os primeiros 12 bytes 
do arquivo para assinaturas de contêineres conhecidos (MP4, MKV, WebM, AVI, 
QuickTime). Não confia em extensão ou Content-Type informado pelo cliente.

**Conclusão:** ✅ Seguro. Validação por conteúdo real.

### 4. Limites de recursos

- Tamanho máximo de upload: `MaxUploadSizeBytes` (do config) aplicado em 
  `init.go:101` e `tus.go:71` (MaxSize do tusd)
- Timeout FFmpeg: 30 minutos por variante (`worker.go:325`)
- Timeout ffprobe: 5 segundos (`validation.go:222`, `worker.go:196`)
- Uploads simultâneos: limitados pelo número de workers do tusd (implícito)

**Conclusão:** ✅ Limites adequados para prevenção de DoS.

### 5. Simlinks e permissões

Arquivos são criados pelo tusd em `UploadTmpDir` e movidos pelo worker para 
`MediaDir`. O serving (`serve.go:263-275`) verifica `os.Stat` (segue symlinks) 
e confirma que o path resolvido está contido em `MediaDir`. Nenhum symlink é 
criado pelo próprio código.

**Conclusão:** ✅ Seguro contra symlink attacks.

### 6. Directory listing

`serve.go:263-275`: se `os.Stat` retorna que o alvo é diretório, retorna 404. 
A validação de filename (`segmentRe` + "playlist.m3u8") impede acesso a 
arquivos arbitrários. Path vazio (terminando em "/") retorna 404.

**Conclusão:** ✅ Directory listing desabilitado.

### Falhas encontradas

**Nenhuma.** O código de upload, validação e processamento FFmpeg está seguro 
contra as vulnerabilidades investigadas. Path traversal, command injection, 
file type spoofing e resource exhaustion são adequadamente mitigados.

---

## T43: Rede, rate limiting, webhooks e configuração

### 1. SSRF no cliente de webhook

`WebhookURL` vem de variável de ambiente obrigatória (`config.go:43-50`). 
Configurada apenas pelo operador — sem possibilidade de influência por dado 
de entrada do usuário.

**Conclusão:** ✅ Seguro. Sem vetor SSRF.

### 2. Rate limiting

O `extractIP` (`ratelimit.go:39-63`) prioriza `X-Real-IP` e `X-Forwarded-For` 
sobre `RemoteAddr`. Correto quando atrás de proxy confiável. Se exposto 
diretamente (sem proxy), um atacante pode falsificar esses headers e burlar 
o rate limit — mas isso é uma decisão de deployment, não um bug de código.

**Conclusão:** ✅ Comportamento correto. Responsabilidade do operador confiar nos headers.

### 3. Headers de segurança

Nenhum header de segurança (`X-Content-Type-Options`, etc.) é definido. O 
servidor é backend-to-backend (não browser-facing), então o impacto é mínimo. 
O Content-Type é explicitamente definido em todas as respostas.

**Conclusão:** ⚪ Baixa prioridade. API backend, não navegador.

### 4. CORS

Sem configuração de CORS. Para API backend-to-backend, CORS não é necessário. 
Se um frontend browser precisasse acessar, seria preciso configurar.

**Conclusão:** ⚪ Baixa prioridade. Não aplicável ao caso de uso atual.

### 5. Timeouts do servidor HTTP

**Falha (média):** `http.Server` em `main.go:84-87` não configurava 
`ReadTimeout`, `WriteTimeout`, `IdleTimeout` ou `MaxHeaderBytes`. Um atacante 
pode explorar Slowloris: abrir conexões e nunca enviar headers completos, 
esgotando o pool de goroutines do servidor.

**Mitigação:** Adicionados timeouts: `ReadTimeout: 10s`, `WriteTimeout: 60s` 
(generoso para servir HLS), `IdleTimeout: 120s`, `MaxHeaderBytes: 1MB`.

**Severidade:** Média

**Status:** Corrigida nesta tarefa

### 6. Segredos em logs

Nenhum segredo (chave HMAC, token, URL de webhook com credenciais) aparece 
em `log.Printf` ou mensagens de erro. O middleware Logger do chi loga path 
e método, não headers ou body.

**Conclusão:** ✅ Seguro. Segredos não vazam em logs.

### 7. Retry de webhook — amplificação

Política de retry: 3 tentativas com backoff de 1s, 2s, 4s (máximo 7s total). 
Volume insignificante para amplificação de tráfego.

**Conclusão:** ✅ Seguro.

### Falhas encontradas

#### F-02: http.Server sem timeouts de rede (Slowloris)

- **Local:** `cmd/server/main.go:84-87`
- **Falha:** `http.Server` criado sem `ReadTimeout`, `WriteTimeout`, 
  `IdleTimeout` ou `MaxHeaderBytes`
- **Exploração:** Atacante abre conexões TCP e nunca envia headers completos 
  (Slowloris). O servidor mantém goroutines abertas indefinidamente, esgotando 
  recursos e impedindo novas conexões legítimas.
- **Mitigação:** Adicionados `ReadTimeout: 10s`, `WriteTimeout: 60s`, 
  `IdleTimeout: 120s`, `MaxHeaderBytes: 1MB`
- **Severidade:** Média
- **Status:** Corrigida nesta tarefa
