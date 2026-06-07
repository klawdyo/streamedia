# T40: Cobertura de testes — upload, autenticação e configuração

**Status:** pending
**Dependências:** nenhuma (revisão do código existente)
**Estimativa:** média
**Origem:** Issue #7 — "Revisão geral do código: procure pontos não cobertos por testes"

## Contexto

`internal/upload` está em 69.0%, `internal/auth` em 74.4% e `internal/config`
em 74.5% de cobertura. São pacotes de borda do sistema — recebem entrada
externa (uploads TUS, headers HMAC, variáveis de ambiente) — onde lacunas de
teste são particularmente arriscadas, pois é justamente onde dados não
confiáveis entram no sistema.

## Arquivos sob revisão

- `internal/upload/init.go` (+ `init_test.go`)
- `internal/upload/tus.go` (+ `tus_test.go`)
- `internal/upload/validation.go` (+ `validation_test.go`)
- `internal/auth/auth.go` (+ `auth_test.go`)
- `internal/config/config.go` (+ `config_test.go`)

## QA Instructions

1. Rode `go test ./internal/upload/... ./internal/auth/... ./internal/config/... -coverprofile=coverage.out`
   e `go tool cover -func=coverage.out` para localizar gaps.
2. Priorize os seguintes cenários, comuns em pontos de entrada não
   confiáveis:
   - `validation.go`: arquivos corrompidos, com extensão errada, tamanho
     zero, MIME type inconsistente com o conteúdo real
   - `tus.go`: headers TUS malformados, upload-id inexistente, offset
     inconsistente, hooks (`pre-create`, `post-finish`) com erros internos
   - `init.go`: payload inválido, limites de tamanho/quantidade excedidos
   - `auth.go`: assinatura HMAC malformada, timestamp expirado, replay,
     comparação não constante de tempo (timing attack)
   - `config.go`: variáveis de ambiente ausentes, valores fora de faixa,
     valores com tipos incorretos, defaults aplicados corretamente
3. Escreva testes table-driven cobrindo entradas válidas, inválidas e
   maliciosas (strings malformadas, valores extremos, encoding inesperado).

## Dev Instructions

1. Implemente os testes que exigirem pequenas adaptações no código para
   serem testáveis (ex.: extrair função pura de validação, permitir
   injeção de relógio/clock para testar expiração de forma determinística).
2. Corrija bugs reais expostos (ex.: validação que aceita entrada que
   deveria rejeitar, erro silenciosamente ignorado, mensagem de erro que
   vaza detalhes internos).
3. Rode `go test ./... -cover` e confirme aumento de cobertura nos três
   pacotes.

## Arquivos a revisar/editar

- `internal/upload/init_test.go`
- `internal/upload/tus_test.go`
- `internal/upload/validation_test.go`
- `internal/auth/auth_test.go`
- `internal/config/config_test.go`

## Definition of Done

- [ ] Relatório de cobertura "antes/depois" documentado
- [ ] Casos de entrada inválida/maliciosa cobertos por testes novos
- [ ] Bugs reais encontrados corrigidos com mudança mínima
- [ ] `go test ./internal/upload/... ./internal/auth/... ./internal/config/... -cover`
      mostra aumento de cobertura
- [ ] `go test ./...` continua passando sem regressões
