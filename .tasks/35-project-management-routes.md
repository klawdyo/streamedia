# T35: Rotas de gerenciamento de projetos

**Status:** pending
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

- [ ] CRUD de projetos exposto via `/admin/projects*`, protegido por admin auth
- [ ] Emissão de token de upload escopado via chave mestra do projeto
- [ ] `master_key`/hash nunca exposto fora da criação inicial
- [ ] README e spec OpenAPI atualizados com as novas rotas
- [ ] Todos os testes passam
</content>
