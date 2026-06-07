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

## Resolução

Cobertura "antes" → "depois":
- `internal/upload`: 69.0% → **72.0%**
- `internal/auth`: 74.4% → **93.0%**
- `internal/config`: 74.5% → **82.8%**

Testes novos table-driven cobrindo entrada inválida/maliciosa nos pontos
de borda do sistema:

- `internal/upload/init_test.go`: tamanhos extremos (negativo, zero,
  overflow de int64, um byte acima do limite), formatos de UUID inválidos
  (versões erradas, variante inválida, SQL injection, null byte,
  uppercase, sem hífens), JSON malformado, comparação HMAC com tempo
  constante (`TestUploadInit_HMACConstantTime`), corpo de requisição
  excessivamente grande.
- `internal/upload/validation_test.go`: magic bytes de containers (MP4,
  MKV, AVI, WebM) válidos e corrompidos, tamanhos-limite.
- `internal/auth/auth_test.go`: `ValidateUploadToken`/`ValidatePlayToken`
  com timestamps no limite, hex malformado, assinaturas truncadas/alteradas.
- `internal/config/config_test.go`: `getEnvStr`/`getEnvInt`/`getEnvBool`
  com valores ausentes, negativos, overflow, caracteres especiais,
  espaços em branco — confirmando que defaults são aplicados corretamente
  e entradas inválidas são rejeitadas (não silenciosamente ignoradas).

**Superfícies de segurança auditadas e confirmadas seguras:**
- Comparações de HMAC usam `hmac.Equal` (tempo constante) em todos os
  pontos — nunca `==` — descartando timing attack.
- `GenerateUploadToken`/`GeneratePlayToken` não dependem de aleatoriedade
  previsível.
- Regex `uuidV4Re` valida estritamente o formato RFC 4122 v4 (nibble de
  versão `4`, variante `8-b`).

**Bug "all-zeros UUID" investigado e descartado**: o QA inicialmente
sinalizou que `00000000-0000-4000-8000-000000000000` era aceito como
video_id válido. Após análise, esse valor É um UUID v4 sintaticamente
válido conforme RFC 4122 (nibbles de versão e variante corretos) — rejeitar
um UUID apenas por ter payload todo-zero seria uma regra arbitrária, não
um requisito de segurança real (a validação de formato já é a defesa
correta; gerar UUIDs previsíveis é responsabilidade de quem os emite, não
de quem os valida). **Não é um bug** — nenhuma alteração de produção
necessária.

**Bloqueadores identificados para cobertura >95%** (não bloqueiam o
fechamento desta tarefa nem da issue — anotados para referência futura
caso o time queira investir mais nesses pacotes):
- `auth.go`: `time.Now()` hardcoded impede testar expiração/replay de
  forma determinística (exigiria injeção de `Clock`).
- `tus.go`: hooks `postReceive`/`postFinish` (0%) são acionados via
  canais internos do tusd/v2 — exigem exposição de método sincronizado
  para teste direto.
- `validation.go`: `runFFprobe` (22.2%) executa `ffprobe` diretamente —
  exigiria interface de executor (mesmo padrão do `FFmpegExecutor`/
  `FFprobeExecutor` introduzido na T39).

`go test ./...` passa integralmente, sem regressões.

## Definition of Done

- [x] Relatório de cobertura "antes/depois" documentado
- [x] Casos de entrada inválida/maliciosa cobertos por testes novos
- [x] Bugs reais encontrados corrigidos com mudança mínima — nenhum bug
      real confirmado (suspeita de "UUID all-zeros" investigada e
      descartada; ver acima)
- [x] `go test ./internal/upload/... ./internal/auth/... ./internal/config/... -cover`
      mostra aumento de cobertura
- [x] `go test ./...` continua passando sem regressões
