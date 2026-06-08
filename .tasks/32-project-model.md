# T32: Model de Projeto (slug, diretório raiz, chave mestra)

**Status:** done
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

- [x] Tabela `projects` criada via migração em `schema.go`
- [x] Slugify determinístico e com resolução de colisão
- [x] `CreateProject` gera e retorna a chave mestra em texto puro uma única vez
- [x] CRUD básico de leitura (`GetProjectBySlug`, `GetProjectByID`, `ListProjects`)
- [x] Todos os testes passam

## Resolução

Criada a fundação do sistema de projetos internos da issue #6:

- **`internal/db/schema.go`**: nova tabela `projects` (id, name, slug
  `UNIQUE`, root_dir, master_key_hash, created_at) + índice em `slug`.
  Segue o padrão `IF NOT EXISTS` das demais tabelas (migração idempotente).
- **`internal/models/project.go`**:
  - `Slugify(name)`: normaliza para `[a-z0-9-]`, removendo acentos via uma
    tabela de substituição própria (`decompose` + `stripDiacritics` —
    evita depender de `golang.org/x/text/unicode/norm` para um caso de uso
    pequeno e estável: nomes em português). Ex.: `"Trip Produção"` →
    `"trip-producao"`.
  - `uniqueSlug`: resolve colisões anexando `-2`, `-3`, ... consultando o
    banco — exatamente como descrito na issue ("se o projeto já existir
    [...] meta um -2 ou -3 etc ao final da Key").
  - `generateMasterKey`/`HashMasterKey`: a chave mestra é gerada com
    `crypto/rand` (32 bytes, hex — mesma entropia dos demais segredos do
    sistema) e **persistida apenas como hash SHA-256**; o valor em texto
    puro só existe no retorno de `CreateProject`, nunca é logado ou
    armazenado em claro.
  - `CreateProject`, `GetProjectByID`, `GetProjectBySlug`, `ListProjects`:
    CRUD básico de leitura/criação, devolvendo `sql.ErrNoRows` em buscas
    sem resultado (consistente com `models.GetVideo`/`GetUploadToken`).
- **`internal/models/project_test.go`**: cobre slugify (incluindo acentos,
  espaços múltiplos e caracteres especiais), geração+persistência da chave
  mestra (hash correto, texto puro nunca igual ao hash), resolução de
  colisão de slug (`trip` → `trip-2` → `trip-3`), buscas inexistentes
  (`sql.ErrNoRows`), listagem ordenada e determinismo do hash.

Esta tarefa **não** expõe rotas HTTP nem altera o layout de armazenamento —
isso fica para T33 (chaves escopadas) e T34 (diretórios por projeto), que
dependem deste model.
</content>
