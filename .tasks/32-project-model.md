# T32: Model de Projeto (slug, diretório raiz, chave mestra)

**Status:** pending
**Dependências:** T03, T31 (evita retrabalho em config)
**Estimativa:** média
**Issue relacionada:** #6

## Contexto

A issue #6 pede suporte a múltiplos "projetos internos" — cada app/ambiente
(produção, staging, teste) que usa o Streamedia deve ter seu próprio
namespace: nome, slug (derivado do nome, ex. "Trip Produção" → `trip-producao`,
com colisão resolvida por sufixo `-2`, `-3`...), diretório raiz dentro de
`MEDIA_DIR` (ex. `./media/trip-producao/`) e uma chave mestra própria.

Esta é a tarefa **fundação** de toda a issue #6 — as demais (T33, T34, T35)
dependem do model e da tabela criados aqui.

### Escopo desta tarefa

Apenas o model e a persistência — **sem** rotas HTTP ainda (isso é T35) e
**sem** mudar o layout de armazenamento existente (isso é T34). O objetivo
aqui é ter `CreateProject`, `GetProject`, `ListProjects` funcionando sobre uma
tabela `projects`.

### Geração de slug

- Normalizar: minúsculas, remover acentos, espaços → `-`, remover caracteres
  fora de `[a-z0-9-]`.
- Em colisão, anexar `-2`, `-3`, ... (primeiro número livre).
- Persistir o slug junto com o projeto (não recalcular em runtime — o
  diretório no disco depende dele permanecer estável).

### Chave mestra

- Gerada na criação do projeto (ex. `crypto/rand`, formato similar aos
  tokens HMAC já usados em `internal/auth`), armazenada com hash (nunca em
  texto puro) — siga o padrão já usado para `UploadToken` em
  `internal/models` / `internal/auth`, se houver hashing lá. Devolva o valor
  em texto puro **apenas** na resposta de criação (não é recuperável depois).

## QA Instructions

Crie `internal/models/project_test.go`:

```
TestSlugify
  - "Trip Produção" → "trip-producao"
  - "  Multi   Espaços!! " → "multi-espacos"
  - colisão: criar "Trip" duas vezes → slugs "trip" e "trip-2"

TestCreateProject_PersistsAndGeneratesMasterKey
  - cria projeto, confere slug, diretório raiz e que a master key
    devolvida bate com o hash armazenado

TestGetProject_NotFound
  - busca por slug/id inexistente → erro sql.ErrNoRows (ou equivalente)

TestListProjects_ReturnsAll
```

## Dev Instructions

- Adicione a tabela `projects` em `internal/db/schema.go` (id, name, slug
  único, root_dir, master_key_hash, created_at).
- Crie `internal/models/project.go` com a struct `Project` e as funções
  `CreateProject`, `GetProjectBySlug`, `GetProjectByID`, `ListProjects`.
- Reaproveite utilidades de geração/hash de token já existentes em
  `internal/auth` em vez de duplicar lógica criptográfica.

## Arquivos a criar/modificar

- `internal/db/schema.go`
- `internal/models/project.go`
- `internal/models/project_test.go`

## Definition of Done

- [ ] Tabela `projects` criada via migração em `schema.go`
- [ ] Slugify determinístico e com resolução de colisão
- [ ] `CreateProject` gera e retorna a chave mestra em texto puro uma única vez
- [ ] CRUD básico de leitura (`GetProjectBySlug`, `GetProjectByID`, `ListProjects`)
- [ ] Todos os testes passam
</content>
