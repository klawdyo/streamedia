# T53: Corrigir `ListByStatus` — omissão de `project_id` na query SELECT

**Status:** pending
**Dependências:** T04, T33
**Estimativa:** pequena
**Origem:** análise estática do código — bug funcional encontrado durante revisão geral

## Contexto

A função `ListByStatus` em `internal/models/video.go:263` faz uma query
`SELECT` que **omite a coluna `project_id`**, embora:

1. A struct `Video` (`video.go:39`) tenha o campo `ProjectID *int64`
2. A função `GetVideo` (`video.go:91-96`) — que faz a mesma varredura de
   linha — inclui `project_id` corretamente na query e no `Scan`
3. `ListByStatus` nem declara o `sql.NullInt64` para `project_id` nem o
   inclui na lista de destinos do `rows.Scan`

O resultado é que **todo vídeo obtido via `ListByStatus` sempre terá
`ProjectID = nil`**, mesmo que a linha no banco tenha `project_id`
preenchido (FK para `projects`, seja o projeto "default" ou outro).
Isso afeta qualquer código que chame `ListByStatus` e dependa do
`ProjectID` — ex.: filtro por escopo de projeto em rotas admin,
resolução de diretório raiz em `ResolveVideoRootDir` (que depende
de `project_id` para achar o slug e montar o path correto no disco).

**Não existe "path legado" neste projeto** — o serviço nunca foi
lançado e `project_id` deve sempre estar preenchido (todo vídeo
pertence a um projeto, seja o "default" ou outro criado pelo
usuário). Portanto `ProjectID = nil` é sempre um bug, nunca um
fallback válido.

Nenhum teste existente flagrou o bug porque os testes não populam
`project_id` nas fixtures de `ListByStatus`.

## QA Instructions

Estenda `internal/models/video_test.go`:

```
TestListByStatus_IncludesProjectID
  - Cria um projeto (ex.: "default", ou um projeto arbitrário)
  - Insere um vídeo com project_id = id do projeto
  - Chama models.ListByStatus(db, StatusPendingUpload)
  - Verifica que o vídeo retornado tem ProjectID != nil
  - Verifica que *ProjectID é igual ao id do projeto criado

TestGetVideo_IncludesProjectID (se ainda não existir)
  - Confirma que GetVideo JÁ retorna ProjectID corretamente
  - Serve como baseline: o teste deve passar ANTES da correção de
    ListByStatus, comprovando que só ListByStatus estava errada
```

## Dev Instructions

### 1. Corrigir a query e o Scan em `ListByStatus`

Em `internal/models/video.go`, função `ListByStatus` (linha ~263):

- Adicione `project_id` à lista de colunas do `SELECT`
- Declare `var projectID sql.NullInt64` no bloco de variáveis do loop
- Adicione `&projectID` à chamada `rows.Scan` (na mesma posição que
  aparece em `GetVideo`)
- Após o Scan, converta `projectID` para `*int64` (mesmo padrão de
  `GetVideo`, linhas ~133-135):
  ```go
  if projectID.Valid {
      v.ProjectID = &projectID.Int64
  }
  ```

Mudança mínima — NÃO altere a assinatura, o contrato de retorno, nem
qualquer outra query da função. Só adicione a coluna que faltava.

### 2. Verificação

- `go test ./internal/models/... -v` — os dois novos testes de QA + todos
  os existentes passam
- `go test ./...` — sem regressões em outros pacotes
- `go vet ./...` — sem novos warnings

## Arquivos a editar

- `internal/models/video.go` (corrigir `ListByStatus`)
- `internal/models/video_test.go` (adicionar os testes do QA)

## Resolução

<!-- Preencher ao concluir -->

## Definition of Done

- [ ] `ListByStatus` inclui `project_id` no SELECT, Scan e conversão
- [ ] Teste novo comprova que `ProjectID` é populado com o id correto do projeto
- [ ] `GetVideo` continua retornando `ProjectID` corretamente (baseline não regrediu)
- [ ] `go test ./...` passa sem regressões
- [ ] `go vet ./...` sem warnings
