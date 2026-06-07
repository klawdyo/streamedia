# T41: Auditoria de segurança — autenticação, autorização e tokens

**Status:** pending
**Dependências:** nenhuma (auditoria do código existente)
**Estimativa:** média
**Origem:** Issue #8 — "Revise o código como especialista em segurança: liste
falhas, como explorá-las, mitigação, e corrija"

## Contexto

Esta é a primeira de três tarefas de auditoria de segurança que, juntas,
cobrem o pedido da issue #8: revisar o código inteiro em busca de falhas
exploráveis, documentar cada uma (vetor de ataque + mitigação) e corrigi-las.
Esta tarefa foca no perímetro de autenticação e autorização: HMAC de upload,
tokens de reprodução (play tokens), tokens de upload e rotas administrativas.

## Escopo da auditoria

- `internal/auth/auth.go` — geração e verificação de assinaturas HMAC
- `internal/models/token.go` — modelo de `UploadToken` e regras de expiração
- `internal/serve/serve.go` e `internal/serve/status.go` — verificação de
  play token para servir HLS e status
- `internal/admin/admin.go` — autenticação das rotas `/admin/*`
- `internal/jobs/cleanup.go` — limpeza de tokens expirados

## Pontos a investigar (checklist de especialista em segurança)

1. **Comparação de assinaturas/tokens**: usa `subtle.ConstantTimeCompare`
   ou equivalente (proteção contra timing attack), ou `==`/`bytes.Equal`
   (vulnerável)?
2. **Replay attacks**: tokens/assinaturas incluem timestamp + janela de
   expiração validada no servidor? É possível reutilizar uma URL assinada
   indefinidamente?
3. **Geração de segredo/chave**: a chave HMAC vem de configuração segura
   (env var) e tem entropia suficiente? Há fallback inseguro (chave vazia,
   chave hardcoded, chave previsível)?
4. **Escopo do token**: um play token gerado para o vídeo A pode ser usado
   para acessar o vídeo B (confusão de escopo / IDOR)?
5. **Rotas admin**: existe verificação de autenticação em TODAS as rotas
   `/admin/*`, inclusive as adicionadas por último? É possível bypassar via
   método HTTP alternativo, trailing slash, case-sensitivity de path?
6. **Mensagens de erro**: respostas 401/403 vazam informação que ajuda um
   atacante (ex.: "token expirado" vs "token inválido" revela validade do
   formato)?
7. **Expiração e limpeza**: tokens expirados continuam válidos por causa de
   comparação de data incorreta (lembrar do bug já corrigido em
   `DeleteExpiredTokens` — verificar se padrões similares existem em outros
   lugares que comparam datetime do SQLite)?

## Instruções de execução

1. Para cada ponto da checklist, leia o código relevante e determine se a
   vulnerabilidade existe. Não assuma — comprove lendo o fluxo completo
   (de onde o dado vem, como é validado, onde é usado).
2. Para cada falha real encontrada, documente em um arquivo
   `SECURITY_AUDIT.md` (criar na raiz, ou anexar a uma seção se já existir
   de uma tarefa irmã — combine com T42/T43 para não sobrescrever):
   - **Local**: arquivo e função
   - **Falha**: descrição técnica
   - **Exploração**: passo a passo de como um atacante abusaria
   - **Mitigação**: a correção recomendada
   - **Status**: corrigida nesta tarefa / requer decisão de produto / etc.
3. Escreva um teste que comprove a vulnerabilidade ANTES de corrigir
   (teste deve falhar/expor o problema), depois corrija o código e
   confirme que o teste passa (red → green, igual ao fluxo normal QA→Dev).
4. Corrija apenas o que está dentro do escopo desta tarefa (autenticação/
   autorização/tokens) — não toque em validação de upload ou rede, que são
   escopo de T42 e T43.

## Definition of Done

- [ ] Cada item da checklist investigado e documentado (vulnerável ou não)
- [ ] Falhas reais registradas em `SECURITY_AUDIT.md` com vetor de ataque
      e mitigação
- [ ] Teste de regressão escrito para cada falha real antes da correção
- [ ] Falhas corrigidas com a menor mudança possível, sem quebrar
      comportamento legítimo
- [ ] `go test ./internal/auth/... ./internal/models/... ./internal/serve/... ./internal/admin/... -v`
      passa, incluindo os novos testes de segurança
- [ ] `go test ./...` continua passando sem regressões
