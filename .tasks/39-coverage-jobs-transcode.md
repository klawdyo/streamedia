# T39: Cobertura de testes — jobs de manutenção e transcodificação

**Status:** pending
**Dependências:** nenhuma (revisão do código existente)
**Estimativa:** média
**Origem:** Issue #7 — "Revisão geral do código: procure pontos não cobertos por testes"

## Contexto

`internal/jobs` está em 56.3% de cobertura e `internal/transcode` em 72.5%.
Esses pacotes controlam processos de longa duração e recuperação de falhas
(killer de uploads inativos, reenfileirador de transcodes travados, limpeza
de tokens, fila de transcodificação, worker FFmpeg, recuperação na
inicialização) — código que só "aparece" em cenários de erro/timeout, e por
isso é fácil deixar caminhos sem teste.

## Arquivos sob revisão

- `internal/jobs/killer.go` (+ `killer_test.go`)
- `internal/jobs/requeue.go` (+ `requeue_test.go`)
- `internal/jobs/cleanup.go` (+ `cleanup_test.go`)
- `internal/transcode/queue.go` (+ `queue_test.go`)
- `internal/transcode/worker.go` (+ `worker_test.go`)
- `internal/transcode/recovery.go` (+ `recovery_test.go`)

## QA Instructions

1. Rode `go test ./internal/jobs/... ./internal/transcode/... -coverprofile=coverage.out`
   e `go tool cover -func=coverage.out` para localizar funções/branches
   descobertos.
2. Foque em cenários de borda típicos de jobs e filas, como:
   - Job rodando quando não há nada a processar (caminho "vazio")
   - Erros de banco de dados durante a varredura/atualização de status
   - Concorrência: job disparado enquanto outro já está em execução
   - Fila cheia (buffer do channel) e comportamento de backpressure
   - Worker recebendo contexto cancelado / timeout do FFmpeg
   - Recuperação de startup encontrando vídeos em estados inconsistentes
     (ex.: `transcoding` sem worker ativo, `uploading` travado)
3. Escreva testes table-driven cobrindo esses caminhos, usando fakes/mocks
   para banco e processo externo (FFmpeg) onde já existir suporte no
   código (verifique se há interfaces/abstrações de exec.Command).
4. Reporte qualquer caminho que pareça logicamente inalcançável (dead code)
   — pode ser um sinal de bug ou de simplificação possível.

## Dev Instructions

1. Implemente os testes que o QA não conseguiu escrever por falta de
   pontos de extensão (ex.: se o worker não permite injetar um executor de
   comando fake, adicione essa abstração mínima sem mudar o comportamento
   em produção).
2. Corrija bugs reais expostos pelos novos testes (ex.: condição de corrida
   não tratada, erro de SQL ignorado, falha em marcar vídeo como
   `failed_upload`/`failed_transcode`).
3. Rode `go test ./... -race -cover` — esses pacotes lidam com goroutines e
   concorrência, então a checagem de race é obrigatória aqui.

## Arquivos a revisar/editar

- `internal/jobs/killer_test.go`
- `internal/jobs/requeue_test.go`
- `internal/jobs/cleanup_test.go`
- `internal/transcode/queue_test.go`
- `internal/transcode/worker_test.go`
- `internal/transcode/recovery_test.go`

## Resolução

Cobertura "antes": `internal/jobs` 56.3%, `internal/transcode` 72.5%.
Cobertura "depois": `internal/jobs` **78.6%**, `internal/transcode` **82.8%**.

Testes novos (27, table-driven onde fazia sentido):

- `internal/jobs/jobs_integration_test.go` (novo, 12 testes): `Start`/`Stop`
  das três jobs, cenários de erro de DB (`QueryError`, `UpdateStatusError`),
  exclusão segura de arquivos inexistentes, uso de `last_chunk_at` vs
  `created_at` no killer.
- `internal/transcode/transcode_coverage_test.go` (novo, 15+ testes):
  `parseDurationSeconds` (decimais/negativos/vazio), `determineResolutions`
  (matriz landscape/portrait), `generateMasterM3U8`, `buildFFmpegArgs`,
  `scanRenditionDir`, `RunStartupRecovery` (DB vazio + estados múltiplos),
  `RealFFmpeg.Run` (contexto/timeout), e `probeVideo` via fake `FFprobeExecutor`
  (JSON válido, comando falha, JSON malformado).
- Ajustes pontuais nos testes existentes (`queue_test.go`: import faltante;
  `worker_test.go`: funções duplicadas removidas; `cleanup_test.go`:
  constraint violation corrigida no fixture).

**Bug real encontrado e corrigido** (`internal/jobs/requeue.go` ~linhas
126-147): o job de reenfileiramento alterava o status do vídeo para
`upload_complete` ANTES de `enqueue()`; se o enqueue falhasse, o vídeo
ficava preso num estado inconsistente (parecia estar na fila lógica mas
nunca entraria na fila real de processamento). Corrigido com **rollback
explícito**: se `enqueue()` falhar, o status volta para `StatusTranscoding`
(best-effort, com log se o próprio rollback falhar). Comentário em
português documenta a invariante. `TestRequeueJob_EnqueueErrorContinues`
passa.

**Ponto de extensão adicionado**: `probeVideo()` em
`internal/transcode/worker.go` chamava `exec.CommandContext("ffprobe", ...)`
diretamente (38.9% de cobertura, impossível mockar). Criada a interface
`FFprobeExecutor` (análoga à `FFmpegExecutor` já existente), com
implementação real `RealFFprobe` preservando o comportamento de produção
e injeção via `Worker.ffprobe` / `NewWorker`.

`go test ./internal/jobs/... ./internal/transcode/... -race -cover` passa
sem detectar races. `go test ./...` passa integralmente (sem regressões).

## Definition of Done

- [x] Relatório de cobertura "antes/depois" documentado
- [x] Cenários de erro e concorrência cobertos por testes novos
- [x] `go test ./internal/jobs/... ./internal/transcode/... -race -cover`
      passa sem detectar races e com cobertura maior que a inicial
- [x] Bugs reais encontrados corrigidos com mudança mínima — rollback de
      status em `requeue.go` quando `enqueue()` falha
- [x] `go test ./...` continua passando sem regressões
