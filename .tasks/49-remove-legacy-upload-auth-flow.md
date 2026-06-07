# T49: Remover fluxo de autenticação legado (HMAC global) de /upload/init

**Status:** pending
**Dependências:** T48 (precisa do projeto padrão automático já implementado)
**Estimativa:** pequena/média
**Origem:** Issue #10 — "Pq UPLOAD_TOKEN_SCOPED_TTL_SECONDS e UPLOAD_TOKEN_TTL_SECONDS são diferentes?"

## Contexto

Segunda tarefa da cadeia que resolve a issue #10 (T48 → T49 → T50).
Hoje `POST /upload/init` (`internal/upload/init.go:60-81`) suporta dois
fluxos de autenticação mutuamente exclusivos:

```go
if projectKey = r.Header.Get("X-Project-Key"); projectKey != "" {
    project, err = models.GetProjectByMasterKeyHash(...)
    // ...
} else {
    sig := r.Header.Get("X-Upload-Auth")
    if sig == "" || !auth.ValidateBackendAuth(h.cfg.UploadTokenSecret, bodyBytes, sig) {
        // 401
    }
}
```

Com a T48, todo upload já resolve um projeto (explícito ou o projeto
padrão). Isso torna o branch `else` — HMAC global assinado com
`UPLOAD_TOKEN_SECRET` via header `X-Upload-Auth` — **redundante**: não
existe mais cenário em que um upload precise (ou deva) acontecer fora do
contexto de um projeto. O usuário confirmou que não há instalações em
produção dependendo desse fluxo ("esse projeto não está sendo usado por
ninguém... eu quero ele limpo e sem vestígios de coisa velha").

## ⚠️ Atenção — `UPLOAD_TOKEN_SECRET` é compartilhado com outros usos

`h.cfg.UploadTokenSecret` (config var `UPLOAD_TOKEN_SECRET`) **não é
exclusivo do fluxo legado de upload**. É reaproveitado como segredo HMAC
geral do backend em pelo menos mais dois lugares, que **não devem ser
alterados** por esta tarefa:

- `internal/serve/serve.go:126-127` — `auth.ValidatePlayToken(h.cfg.UploadTokenSecret, ...)`,
  geração/validação de tokens de reprodução (play tokens)
- `internal/serve/status.go:70` — `auth.ValidateBackendAuth(h.cfg.UploadTokenSecret, []byte(videoID), signature)`,
  autenticação de requisições assinadas pelo backend à rota de status

Ou seja: **NÃO remova** a variável de config `UploadTokenSecret`/
`UPLOAD_TOKEN_SECRET` nem a função `auth.ValidateBackendAuth` — ambas
continuam em uso fora do contexto de upload. O que se remove aqui é
apenas **o branch que usa esse secret especificamente para autenticar
`POST /upload/init`** (header `X-Upload-Auth` + geração de token de
upload com `h.cfg.UploadTokenSecret` no fluxo sem projeto).

(Nota à parte, fora do escopo desta tarefa: o nome `UPLOAD_TOKEN_SECRET`
é enganoso — sugere que é exclusivo de upload quando na verdade é um
segredo HMAC de backend de uso geral. Pode valer a pena renomear no
futuro para algo como `BACKEND_AUTH_SECRET`, mas isso é uma mudança maior
de superfície de configuração e não está coberto pela issue #10 — não
faça isso aqui.)

## O que muda

1. **`POST /upload/init` passa a exigir `X-Project-Key` sempre** —
   remove o branch `else` (HMAC global / `X-Upload-Auth`). A ausência do
   header não é mais "use o fluxo legado": é simplesmente "use a chave
   do projeto que vai receber este upload" — que pode ser a chave do
   projeto padrão criado pela T48 (devolvida ao operador na primeira
   inicialização) ou de um projeto explícito.
   - Decida e documente: a chave do projeto padrão DEVE ser informada
     explicitamente via `X-Project-Key` (unificando 100% em um único
     mecanismo de auth — chave mestra de projeto), ou o servidor pode
     resolver para o projeto padrão mesmo sem header e sem validar
     nenhuma chave? A issue ("crie uma key default pra incluí-lo lá")
     sugere a primeira opção — manter um único mecanismo de
     autenticação (chave de projeto, sempre validada via
     `GetProjectByMasterKeyHash`) é mais simples, mais seguro e elimina
     de vez a ambiguidade "upload autenticado vs. não-autenticado".
     Recomendado: exigir `X-Project-Key` sempre — sem ela, 401.
2. **Remover `auth.GenerateUploadToken(h.cfg.UploadTokenSecret, ...)`**
   do branch sem projeto em `init.go:133-136` — a geração de token de
   upload passa a usar exclusivamente a chave mestra do projeto
   resolvido (`auth.GenerateUploadToken(projectKey, req.VideoID)`).
3. **Atualizar a documentação inline** do handler (`init.go:38-51`) —
   remover a descrição do "fluxo legado/global" e documentar o único
   fluxo restante (chave de projeto, explícita ou padrão).
4. **Varrer comentários/docs que descrevem o fluxo duplo** —
   `internal/models/token.go:16,45` e `internal/models/video.go:38`
   mencionam "a chave global UPLOAD_TOKEN_SECRET" como alternativa ao
   projeto; ajuste para refletir que o token de upload é **sempre**
   vinculado a um projeto.
5. **Verificar se `auth.ValidateBackendAuth` continua exportada e usada**
   — ela é usada por `status.go` (ver seção de atenção acima), então
   permanece. Apenas o *call site* em `init.go` é removido.

## QA Instructions

1. Atualize/escreva testes de `POST /upload/init`:
   - Requisição sem `X-Project-Key` (e sem `X-Upload-Auth`, que não
     existe mais) → 401, com mensagem clara
   - Requisição com `X-Upload-Auth` (header agora ignorado/obsoleto) →
     também 401 — não deve haver "fallback silencioso" para um fluxo
     que não existe mais
   - Requisição com `X-Project-Key` válida (projeto explícito ou
     padrão) → 200, fluxo completo gera token vinculado ao projeto
   - Requisição com `X-Project-Key` inválida → 401
2. Confirme, rodando a suíte completa (`go test ./internal/serve/...`),
   que `ValidatePlayToken`/`ValidateBackendAuth` em `serve.go`/
   `status.go` continuam funcionando exatamente como antes — esta
   tarefa NÃO deve alterar comportamento de play tokens nem da rota de
   status.
3. Rode `go vet ./...` / verifique se sobrou código morto (ex.:
   `auth.GenerateUploadToken` ainda usado? `ValidateBackendAuth` ainda
   referenciada em `init.go`?) — reporte qualquer função que ficou sem
   chamadores.

## Dev Instructions

1. Remova o branch HMAC legado de `init.go` conforme o item 1 (decisão:
   `X-Project-Key` sempre obrigatória).
2. Ajuste a geração do token de upload para usar exclusivamente a chave
   do projeto resolvido.
3. Atualize comentários/docs conforme itens 3 e 4.
4. **Não toque** em `UploadTokenSecret`/`UPLOAD_TOKEN_SECRET`,
   `ValidateBackendAuth`, `ValidatePlayToken` nem nos call sites de
   `serve.go`/`status.go` — são usos legítimos e não relacionados.
5. Rode `go test ./...` e confirme que tudo passa, sem regressões em
   `internal/serve`.

## Arquivos a revisar/editar

- `internal/upload/init.go` (remover branch HMAC, atualizar doc do handler)
- `internal/models/token.go` (comentários sobre fluxo legado)
- `internal/models/video.go` (comentário sobre fluxo legado)
- `internal/upload/init_test.go`

## Definition of Done

- [ ] `POST /upload/init` exige `X-Project-Key` em toda requisição —
      branch HMAC global removido
- [ ] Geração de token de upload usa exclusivamente a chave do projeto
- [ ] Comentários/docs do pacote `upload`/`models` não mencionam mais o
      fluxo legado de upload
- [ ] `UploadTokenSecret`/`ValidateBackendAuth`/`ValidatePlayToken`
      preservados e funcionando em `serve.go`/`status.go` (nenhuma
      regressão fora do escopo de upload)
- [ ] `go test ./...` passa sem regressões
