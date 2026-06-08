# T62: Otimizar `decompose` — `strings.NewReplacer` alocado a cada chamada

**Status:** done
**Dependências:** T32
**Estimativa:** pequena
**Origem:** análise de código — ineficiencia
**Severidade:** alta

## Contexto

Em `internal/models/project.go:95-111`, a funcao `decompose` cria um novo
`strings.NewReplacer` com 44 pares a cada invocacao:

```go
func decompose(s string) string {
    replacer := strings.NewReplacer(
        "a", "a", "a", "a", ... // 44 pares
    )
    return replacer.Replace(s)
}
```

`strings.NewReplacer` constroi internamente uma tabela de substituicao
otimizada (trie ou tabela de lookup) — essa construcao tem custo
proporcional ao numero de pares. Recriar a cada chamada desperdica CPU e
memoria desnecessariamente.

A funcao e chamada via `Slugify` → `stripDiacritics` → `decompose`, que e
acionada em toda criacao de projeto e na resolucao de slug.

## Impacto

- **Alocacoes desnecessarias** a cada chamada de `Slugify`.
- Impacto real e baixo (criacao de projeto e operacao rara), mas o fix e
  trivial (uma linha) e elimina uma ineficiencia obvia.

## Dev Instructions

### 1. Mover o Replacer para variavel de pacote

```go
var accentReplacer = strings.NewReplacer(
    "a", "a", "a", "a", "a", "a", "a", "a", "a", "a",
    // ... demais pares ...
)

func decompose(s string) string {
    return accentReplacer.Replace(s)
}
```

`strings.NewReplacer` e thread-safe — pode ser chamado concorrentemente.

### 2. Verificacao

- `go test ./internal/models/...` — testes de Slugify existentes passam
- `go vet ./...` — sem warnings

## Arquivos a editar

- `internal/models/project.go` (mover NewReplacer para var de pacote)

## Resolução

Arquivos alterados:
- `internal/models/project.go`: `strings.NewReplacer` movido para var de pacote
  `accentReplacer`. `decompose()` reduzida a uma linha.

## Definition of Done

- [x] `strings.NewReplacer` e criado uma unica vez como variavel de pacote
- [x] `decompose` usa a variavel em vez de criar nova instancia
- [x] Testes de `Slugify` passam sem alteracao
- [x] `go test ./...` passa
- [x] `go vet ./...` limpo
