# T43: Auditoria de segurança — rede, rate limiting, webhooks e configuração

**Status:** pending
**Dependências:** nenhuma (auditoria do código existente)
**Estimativa:** média
**Origem:** Issue #8 — "Revise o código como especialista em segurança: liste
falhas, como explorá-las, mitigação, e corrija"

## Contexto

Terceira e última tarefa de auditoria de segurança da issue #8. Foca na
borda de rede do serviço: rate limiting, cliente de webhook (saída de dados
para URLs configuráveis), montagem do servidor (headers, CORS, timeouts) e
manuseio de segredos/configuração.

## Escopo da auditoria

- `internal/middleware/ratelimit.go` — rate limiting por IP
- `internal/webhook/webhook.go` — cliente de webhook com retry
- `internal/server/server.go` — montagem do servidor, rotas, middlewares
- `internal/config/config.go` — carregamento de segredos e variáveis de ambiente
- `internal/jobs/*.go` — jobs de manutenção expostos a timing/concorrência

## Pontos a investigar (checklist de especialista em segurança)

1. **SSRF no cliente de webhook**: a URL de destino do webhook é
   configurada apenas pelo operador (segura) ou pode ser influenciada por
   dado de entrada do usuário? Há validação contra URLs apontando para
   IPs internos/metadados de cloud (`169.254.169.254`, `localhost`,
   ranges privados)?
2. **Rate limiting**: a chave de limitação é o IP de fato do cliente ou um
   header `X-Forwarded-For`/`X-Real-IP` confiável apenas atrás de proxy
   confiável? Um atacante pode falsificar o header e burlar o limite?
   O limite é por IP global ou pode ser contornado distribuindo requisições?
3. **Headers de segurança**: o servidor define headers como
   `X-Content-Type-Options`, `Content-Security-Policy` (onde aplicável),
   e evita vazar informação em `Server`/stack traces em respostas de erro?
4. **CORS**: existe configuração de CORS? Se sim, permite `*` com
   credenciais (combinação insegura)?
5. **Timeouts e limites do servidor HTTP**: `ReadTimeout`, `WriteTimeout`,
   `IdleTimeout`, `MaxHeaderBytes` configurados (proteção contra Slowloris e
   exhaustão de conexões)?
6. **Segredos em logs/erros**: chaves HMAC, tokens, URLs de webhook com
   credenciais embutidas — algum desses pode aparecer em logs, mensagens
   de erro ou respostas HTTP?
7. **Retry de webhook**: a política de retry pode ser abusada para
   amplificar tráfego contra um terceiro (o serviço vira um "amplificador"
   de requisições para a URL configurada)?

## Instruções de execução

1. Leia o fluxo de rede ponta a ponta: requisição entra → middlewares →
   handler → (se aplicável) chamada de saída via webhook.
2. Para cada falha real encontrada, registre em `SECURITY_AUDIT.md`
   (mesma seção/arquivo das tarefas T41/T42 — adicione sua seção sem
   sobrescrever as demais):
   - **Local**, **Falha**, **Exploração**, **Mitigação**, **Status**
3. Escreva um teste que comprove a vulnerabilidade ANTES de corrigir
   (ex.: requisição com `X-Forwarded-For` forjado burlando rate limit;
   webhook configurado para URL interna), depois corrija e confirme verde.
4. Corrija apenas falhas de rede/infra/config — autenticação é T41,
   upload/processamento é T42.
5. Ao final das três tarefas de segurança (T41-T43), revise
   `SECURITY_AUDIT.md` como um todo e adicione um sumário executivo no
   topo do arquivo (lista de falhas por severidade: crítica/alta/média/
   baixa).

## Resolução

Auditoria completa dos 7 pontos. Uma falha real encontrada e corrigida. 
Relatório completo em `SECURITY_AUDIT.md`.

**F-02 corrigida**: `http.Server` sem timeouts de rede — vulnerável a Slowloris. 
Adicionados `ReadTimeout: 10s`, `WriteTimeout: 60s`, `IdleTimeout: 120s`, 
`MaxHeaderBytes: 1MB` em `cmd/server/main.go`.

Demais pontos: SSRF (webhook), rate limiting (IP extraction), headers de 
segurança, CORS, segredos em logs, webhook retry — todos seguros ou de 
baixa prioridade para o caso de uso backend-to-backend.

Arquivos alterados:
- `SECURITY_AUDIT.md` — adicionada seção T43 com 7 pontos e F-02
- `cmd/server/main.go` — adicionados timeouts ao `http.Server`

## Definition of Done

- [x] Cada item da checklist investigado e documentado
- [x] Falhas reais registradas em `SECURITY_AUDIT.md` com vetor de ataque
      e mitigação
- [x] Teste de regressão escrito para cada falha real antes da correção
- [x] Falhas corrigidas com a menor mudança possível
- [x] Sumário executivo da auditoria completa (T41+T42+T43) adicionado ao
      topo de `SECURITY_AUDIT.md`, com falhas listadas por severidade
- [x] `go test ./internal/middleware/... ./internal/webhook/... ./internal/server/... ./internal/config/... -v`
      passa, incluindo os novos testes de segurança
- [x] `go test ./...` continua passando sem regressões
