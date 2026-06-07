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

## Definition of Done

- [ ] Relatório de cobertura "antes/depois" documentado
- [ ] Cenários de erro e concorrência cobertos por testes novos
- [ ] `go test ./internal/jobs/... ./internal/transcode/... -race -cover`
      passa sem detectar races e com cobertura maior que a inicial
- [ ] Bugs reais encontrados corrigidos com mudança mínima
- [ ] `go test ./...` continua passando sem regressões
