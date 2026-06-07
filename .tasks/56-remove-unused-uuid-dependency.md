# T56: Remover dependência órfã `google/uuid`

**Status:** pending
**Dependências:** nenhuma
**Estimativa:** pequena
**Origem:** análise estática do código — dependência não utilizada identificada durante revisão geral

## Contexto

O `go.mod` declara a dependência `github.com/google/uuid v1.6.0` (linha 7),
mas **nenhum arquivo `.go` do projeto importa esse pacote**. A validação
de UUID no código é feita exclusivamente via regex (`uuidV4Re`), sem
usar `uuid.Parse()` ou qualquer outra função do pacote `google/uuid`.

A dependência está presente no `go.mod` e em `go.sum`, o que significa que
ela é baixada em todo `go mod download`/build, polui o `go.sum` com hashes
de módulos transitivos que não são necessários, e aparece em relatórios de
SBOM/dependências como se fosse usada em produção — quando na verdade é
código morto.

O pacote provavelmente foi adicionado durante o scaffold inicial (T01) com
a intenção de ser usado para validação/geração de UUID, mas a implementação
acabou optando por regex pura (mais simples, sem dependência externa para
uma validação trivial) e o import nunca foi removido.

## QA Instructions

Crie/estenda `readme_test.go` (na raiz do projeto) ou um teste dedicado
em `internal/ci/ci_test.go`:

```
TestNoUnusedDependencies
  - Executa `go mod tidy` em um ambiente controlado
  - Verifica que `go.mod` NÃO contém `github.com/google/uuid`
  - Verifica que `go.sum` NÃO contém entradas para `github.com/google/uuid`
  - (Alternativa mais simples: faz grep no código-fonte e confirma que
     nenhum arquivo .go importa "github.com/google/uuid")

TestAllImportsAreUsed
  - Lista todos os imports `require` diretos do `go.mod`
  - Para cada um, verifica (via grep) que existe pelo menos um arquivo
    .go que o referencia
  - Falha se algum require direto não tem uso no código
```

Nota: o segundo teste (`TestAllImportsAreUsed`) pode ser deixado como
`t.Skip()` comentado se for complexo demais para automatizar — mas deve
ser documentado que a verificação manual foi feita.

## Dev Instructions

### 1. Remover a dependência

```bash
# Remove a dependência do go.mod
go mod edit -droprequire github.com/google/uuid

# Limpa o go.sum de entradas órfãs
go mod tidy
```

### 2. Verificar que nada quebrou

- `go build ./...` — compila sem erros (confirma que nada importava
  `google/uuid` indiretamente)
- `go test ./...` — todos os testes passam
- `go vet ./...` — sem warnings
- `grep -r "google/uuid" --include="*.go" .` — nenhum resultado

### 3. (Opcional) Se o projeto quiser usar UUID de verdade no futuro

Se houver planos de usar `google/uuid` para geração ou parsing de UUID
(em vez de regex), este NÃO é o momento de adicionar esse uso — a T44
(`44-optional-video-id-uuidv7.md`) já trata da geração de UUID v7 e deve
decidir se usa `google/uuid` ou outra abordagem. Esta tarefa (T56) apenas
remove o import morto; a T44 decidirá se reintroduz a dependência com uso
real.

## Arquivos a editar

- `go.mod` (remover `github.com/google/uuid`)
- `go.sum` (atualizado automaticamente por `go mod tidy`)

## Resolução

<!-- Preencher ao concluir -->

## Definition of Done

- [ ] `go.mod` não contém `github.com/google/uuid`
- [ ] `go.sum` não contém hashes de `github.com/google/uuid` ou seus
      transitivos órfãos
- [ ] Nenhum arquivo `.go` importa `google/uuid`
- [ ] `go build ./...` compila sem erros
- [ ] `go test ./...` passa sem regressões
- [ ] `go vet ./...` sem warnings
