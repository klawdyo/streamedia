# T23: GitHub Actions — ci.yml + release.yml

**Status:** pending
**Dependências:** T22
**Estimativa:** pequena

## Contexto

Dois workflows de CI/CD:

### ci.yml (em todo push e pull_request)

Executa em todo push para qualquer branch e em todo pull request.
Valida que o código compila, testa e passa no linter.

```
- checkout
- setup-go (versão 1.23)
- go mod download
- go vet ./...
- go test ./... com cobertura (-coverprofile=coverage.out)
- go build ./... (garante que compila)
- golangci-lint (usando action oficial)
```

### release.yml (em tag v*.*.*)

Cria e publica a imagem Docker no GitHub Container Registry.

```
- checkout
- setup Docker Buildx
- login no ghcr.io com GITHUB_TOKEN nativo
- docker build e push com tag da versão e "latest"
- Imagem: ghcr.io/{owner}/{repo}:{tag}
```

**Importante:** Nenhum secret de aplicação (UPLOAD_TOKEN_SECRET etc.) entra
nos workflows. Apenas o GITHUB_TOKEN nativo é usado no release.

## QA Instructions

Crie `.github/workflows/ci_test.go` — não, espere: testes de GitHub Actions
não são Go. Em vez disso:

Crie `internal/ci/ci_test.go`:

```
TestCIWorkflowExists
  - Verifica que .github/workflows/ci.yml existe

TestReleaseWorkflowExists
  - Verifica que .github/workflows/release.yml existe

TestCIWorkflowHasGoTest
  - Lê ci.yml
  - Verifica presença de "go test"

TestCIWorkflowHasGoVet
  - Verifica presença de "go vet"

TestCIWorkflowHasGolangciLint
  - Verifica presença de "golangci-lint"

TestReleaseWorkflowHasGHCR
  - Lê release.yml
  - Verifica presença de "ghcr.io"

TestReleaseWorkflowUsesGithubToken
  - Verifica presença de "GITHUB_TOKEN" no release.yml
  - Verifica que NÃO há "UPLOAD_TOKEN_SECRET" no release.yml (sem secrets de app)

TestReleaseWorkflowOnTag
  - Verifica que o trigger é em tags (on: push: tags: 'v*')
```

## Dev Instructions

### Criar .github/workflows/ci.yml

```yaml
name: CI

on:
  push:
  pull_request:

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
          cache: true

      - name: Download dependencies
        run: go mod download

      - name: Vet
        run: go vet ./...

      - name: Test
        run: go test ./... -coverprofile=coverage.out -covermode=atomic

      - name: Build
        run: CGO_ENABLED=0 go build ./cmd/server

      - name: Lint
        uses: golangci/golangci-lint-action@v6
        with:
          version: latest
```

### Criar .github/workflows/release.yml

```yaml
name: Release

on:
  push:
    tags:
      - 'v*.*.*'

jobs:
  docker:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write

    steps:
      - uses: actions/checkout@v4

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Log in to GitHub Container Registry
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Extract metadata
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: ghcr.io/${{ github.repository }}
          tags: |
            type=semver,pattern={{version}}
            type=raw,value=latest

      - name: Build and push
        uses: docker/build-push-action@v5
        with:
          context: .
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          cache-from: type=gha
          cache-to: type=gha,mode=max
```

## Arquivos a criar

- `.github/workflows/ci.yml`
- `.github/workflows/release.yml`
- `internal/ci/ci_test.go`

## Definition of Done

- [ ] ci.yml com go test, go vet, build e lint
- [ ] release.yml com push para ghcr.io em tags
- [ ] Sem secrets de aplicação nos workflows
- [ ] Todos os testes de verificação passam
