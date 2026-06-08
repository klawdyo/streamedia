# T35: Rotas de gerenciamento de projetos

**Status:** done
**Dependências:** T32, T33
**Estimativa:** pequena/média
**Issue relacionada:** #6

## Contexto

Expõe via HTTP o CRUD básico de projetos criado em T32, e a emissão de novas
chaves de upload escopadas (T33), para que o backend principal consiga: (1)
criar projetos, (2) listar/consultar projetos existentes, (3) trocar a chave
mestra de um projeto por uma chave de upload de curta duração para um vídeo
específico.

Operação sensível — protegida por um nível de acesso acima do `ADMIN_TOKEN`
por projeto (estas rotas criam os próprios projetos e suas chaves mestras, e
portanto devem usar o equivalente a um "super admin" global).

## Dev Instructions

Rotas sugeridas (ajuste nomes/verbos ao padrão já usado em `internal/admin`):

- `POST /admin/projects` — cria projeto a partir de `{"name": "..."}`,
  devolve `{id, name, slug, root_dir, master_key}` (master key em texto puro,
  uma única vez).
- `GET /admin/projects` — lista projetos (sem expor `master_key_hash`).
- `GET /admin/projects/{slug}` — detalhe de um projeto.
- `POST /admin/projects/{slug}/upload-tokens` — troca a chave mestra (no
  corpo ou header de auth) por um token de upload escopado a um `video_id`
  recém-gerado, com TTL curto (T33).

Proteja com o mesmo middleware `admin.AdminAuth` usado nas demais rotas
`/admin/*` (super admin global), e documente por que essas rotas
especificamente não usam chave por projeto (elas gerenciam os próprios
projetos).

## QA Instructions

Crie `internal/admin/projects_test.go`:

```
TestCreateProject_ReturnsSlugAndMasterKey
TestCreateProject_RequiresAdminAuth
TestListProjects_OmitsMasterKeyHash
TestGetProject_NotFound
TestIssueUploadToken_RequiresProjectMasterKey
TestIssueUploadToken_ShortTTL
  - confere que o token emitido expira em 15-20 minutos
```

## Arquivos a criar/modificar

- `internal/admin/projects.go` (novo handler)
- `internal/admin/projects_test.go`
- `internal/server/server.go` (registro das rotas)
- `README.md` (documentar as novas rotas, seguindo o padrão das seções
  "Rotas da API")
- `internal/docs/spec.go` (acrescentar os novos paths à especificação
  OpenAPI criada em T30 — mantém a documentação completa)

## Definition of Done

- [x] CRUD de projetos exposto via `/admin/projects*`, protegido por admin auth
- [x] Emissão de token de upload escopado via chave mestra do projeto
- [x] `master_key`/hash nunca exposto fora da criação inicial
- [x] README e spec OpenAPI atualizados com as novas rotas
- [x] Todos os testes passam

## Resolução

Implementadas as quatro rotas sugeridas em `internal/admin/projects.go`
(novo arquivo), registradas em `internal/server/server.go` — fechando a
issue #6 (a cadeia T32→T33→T34→T35 está completa).

### Rotas e modelo de autenticação — duas categorias distintas

A tarefa descreve quatro operações, mas elas se dividem em dois grupos com
necessidades de autenticação genuinamente diferentes — em vez de forçar
todas atrás do mesmo middleware, documentei e implementei essa distinção
explicitamente:

**1. Gerenciamento de projetos — exclusivamente super-admin (`ADMIN_TOKEN`global):**
- `POST /admin/projects` — cria o projeto, devolve `{id, name, slug,
  root_dir, master_key}` (chave mestra em texto puro, única vez).
- `GET /admin/projects` — lista projetos via `models.ListProjects`,
  convertidos para `projectResponse` (sem `MasterKeyHash`).
- `GET /admin/projects/{slug}` — detalhe via `models.GetProjectBySlug`,
  mesma omissão de campos sensíveis; `404` se o slug não existir.

  Essas três ficam dentro do `r.Group` que já usa `admin.AdminAuth` (T33),
  mas adicionei o helper `requireSuperAdmin` que rejeita explicitamente
  qualquer requisição autenticada por chave mestra de projeto
  (`ProjectScopeFromContext != nil` → `403 Forbidden`). Justificativa
  (também documentada no código): estas rotas criam os próprios projetos e
  geram suas chaves mestras — autenticar com a chave mestra de um projeto
  já existente para criar/listar/inspecionar outros projetos seria uma
  escalada de privilégio. Segue à risca a recomendação do Contexto da
  tarefa ("nível de acesso acima do ADMIN_TOKEN por projeto").

**2. Emissão de token de upload — autenticação pela própria chave mestra do projeto:**
- `POST /admin/projects/{slug}/upload-tokens` — **decisão deliberada**: esta
  rota NÃO usa `admin.AdminAuth`/`Authorization: Bearer`. Ela é, na
  prática, a operação inversa de "gerenciar projetos": é o cliente
  apresentando a chave mestra de UM projeto para obter credenciais de
  upload PARA ESSE PROJETO — exatamente o mesmo fluxo que `POST
  /upload/init` já implementa no caminho escopado a projeto (T33), só que
  aqui o `video_id` é gerado pelo servidor (`uuid.NewString()`, lib
  `google/uuid` — já presente como dependência indireta via tusd, promovida
  a direta) em vez de informado pelo cliente. Por isso a autenticação
  natural é o header `X-Project-Key` (mesmo princípio: o servidor calcula
  o hash e resolve o projeto, nunca retém a chave em claro). Validação
  extra: o `{slug}` do path precisa corresponder ao slug do projeto
  resolvido pela chave — `403` caso contrário, defesa em profundidade
  contra o uso da chave de um projeto para "rotular" um token como de outro.

  Reaproveita integralmente a infraestrutura de T33: `auth.GenerateUploadToken`
  assinado com a chave mestra como segredo, `cfg.UploadTokenScopedTTL`
  (~15-20min) como TTL, e `models.InsertVideoForProject`/
  `InsertUploadTokenForProject` para persistir o vínculo ao projeto. A
  resposta espelha a de `/upload/init`: `{video_id, upload_url, token,
  expires_at}`.

  Esta escolha resolve, de forma consistente, uma aparente tensão entre o
  "Contexto" da tarefa (que enquadra todas as quatro rotas como exigindo
  nível de super-admin) e as "QA Instructions" (que nomeiam o teste
  `TestIssueUploadToken_RequiresProjectMasterKey` — exigindo a chave
  mestra do PROJETO, não o token global): a operação de "emitir um token
  de upload para MEU projeto" não é uma operação administrativa sobre
  projetos — é o mesmo tipo de operação que `/upload/init` já expõe.

### Respostas nunca expõem segredos

`projectResponse` (usado em listagem e detalhe) e `createProjectResponse`
(usado na criação) são tipos dedicados que nunca incluem `MasterKeyHash` —
a única exceção é `master_key` em texto puro, presente exclusivamente em
`createProjectResponse` (resposta de `POST /admin/projects`), o único
momento em que ela existe fora do hash persistido (mesmo princípio de
`models.CreateProject`, T32).

### Testes — `internal/admin/projects_test.go`

Criado `newProjectsTestRouter` (roteador chi mínimo reproduzindo o
agrupamento real do `server.go`) porque os handlers usam `chi.URLParam`
para `{slug}`, que exige um `RouteContext` real:

- `TestCreateProject_ReturnsSlugAndMasterKey` — cria, valida slug/root_dir/
  master_key na resposta e confirma que a chave devolvida resolve, via
  `GetProjectByMasterKeyHash`, para o mesmo projeto.
- `TestCreateProject_RequiresAdminAuth` — sem auth → 401; com chave mestra
  de outro projeto → 403 (`requireSuperAdmin`).
- `TestListProjects_OmitsMasterKeyHash` — confirma ausência de qualquer
  campo "master_key" no corpo da resposta.
- `TestGetProject_NotFound` — slug inexistente → 404.
- `TestIssueUploadToken_RequiresProjectMasterKey` — cobre quatro cenários:
  sem `X-Project-Key` (401), com a chave de OUTRO projeto (403), com o
  `ADMIN_TOKEN` global no header `X-Project-Key` (401 — não é uma chave de
  projeto válida), e com a própria chave (201 — valida vídeo registrado e
  associado ao projeto correto).
- `TestIssueUploadToken_ShortTTL` — confirma TTL entre 15-20min tanto na
  resposta (`expires_at`) quanto no registro persistido
  (`GetUploadTokenByVideoID`).

### Documentação

- `README.md`: quatro novas seções em "Rotas da API", entre `/admin/queue`
  e `/healthz`, documentando corpo/resposta/autenticação de cada rota —
  com destaque para o aviso de que `master_key` só aparece na resposta de
  criação.
- `internal/docs/spec.go`: adicionados os paths `/admin/projects`,
  `/admin/projects/{slug}` e `/admin/projects/{slug}/upload-tokens` à spec
  OpenAPI (T30), incluindo o novo security scheme `projectKey` (apiKey via
  header `X-Project-Key`) — testado por `TestOpenAPISpecIsValidJSON` e
  `TestOpenAPISpecDocumentsKnownRoutes`.

`go build ./...`, `go vet ./...` e `go test ./...` (suíte completa) passam
sem falhas.
</content>
