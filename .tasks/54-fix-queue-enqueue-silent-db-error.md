# T54: Corrigir `Queue.Enqueue` — ignora erro de banco silenciosamente

**Status:** pending
**Dependências:** T10
**Estimativa:** pequena
**Origem:** análise estática do código — bug de consistência de estado identificado durante revisão geral

## Contexto

A função `Queue.Enqueue` em `internal/transcode/queue.go:86-99` faz duas
operações em sequência:

1. **Atualiza o status do vídeo para `'transcoding'` no banco** (linha 90)
2. **Enfileira o videoID no channel** (linhas 93-98)

O problema está na linha 90:

```go
_, _ = q.db.Exec("UPDATE videos SET status = 'transcoding', ...", videoID)
```

O erro do banco é **completamente ignorado** (`_, _`). Se o `UPDATE`
falhar (ex.: banco travado, conexão perdida, linha não encontrada), o
vídeo é enfileirado no channel **com o status antigo** — e o worker vai
processá-lo sem nunca ter a transição `upload_complete → transcoding`
registrada.

Isso causa um estado inconsistente: o vídeo está "em processamento" na
fila mas o banco diz que ele ainda está em `upload_complete`. Se o
servidor cair nesse momento, o recovery de startup (T21) não encontrará
o vídeo como `transcoding` para reenfileirar — ele será perdido.

**Comparação com T39:** O bug corrigido em T39 (`internal/jobs/requeue.go`)
era exatamente da mesma natureza — alteração de status ANTES de enfileirar,
com possibilidade de o enfileiramento falhar e deixar estado sujo. Aqui o
problema é inverso: o enfileiramento **sucede** mas o status **falhou**.

## QA Instructions

Estenda `internal/transcode/queue_test.go`:

```
TestEnqueue_DBErrorReturnsError
  - Cria uma queue com um db mockado/fake que retorna erro no Exec
  - Chama Enqueue(videoID)
  - Verifica que Enqueue retorna erro (não nil)
  - Verifica que o videoID NÃO foi enfileirado (queue.Len() == 0)

TestEnqueue_DBErrorDoesNotChangeQueueLength
  - Similar ao acima, mas usa um contador de chamadas
  - Confirma que, quando o DB falha, o channel permanece inalterado
```

Se o código atual não permitir mockar `*sql.DB` diretamente (não há
interface `DB` injetável), o QA deve documentar essa limitação e sugerir
a abstração mínima necessária (ex.: uma interface `StatusUpdater` com
`SetTranscoding(videoID string) error`).

## Dev Instructions

### 1. Corrigir `Enqueue` para não ignorar o erro do banco

Em `internal/transcode/queue.go:86-99`, modificar a função `Enqueue`:

- Extrair a lógica de atualização de status para um método ou função
  separada que **retorna o erro** em vez de ignorá-lo
- Se o `UPDATE` falhar, **retornar o erro imediatamente** — sem
  enfileirar o vídeo no channel
- Se o `UPDATE` tiver sucesso, enfileirar normalmente

Mudança mínima:
```go
func (q *Queue) Enqueue(videoID string) error {
    _, err := q.db.Exec(
        "UPDATE videos SET status = 'transcoding', updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE video_id = ?",
        videoID,
    )
    if err != nil {
        return fmt.Errorf("erro ao atualizar status para transcoding: %w", err)
    }

    select {
    case q.ch <- videoID:
        return nil
    default:
        return fmt.Errorf("Fila de transcodificação está cheia.")
    }
}
```

### 2. Se necessário, adicionar ponto de extensão para teste

Se o QA reportar que não consegue mockar `*sql.DB` para testar o cenário
de erro, adicione uma interface mínima — mas só se for estritamente
necessário para o teste. Documente na seção Resolução.

### 3. Verificação

- `go test ./internal/transcode/... -v` — testes novos e existentes passam
- `go test ./...` — sem regressões
- `go vet ./...` — sem warnings

## Arquivos a editar

- `internal/transcode/queue.go` (corrigir `Enqueue`)
- `internal/transcode/queue_test.go` (adicionar/estender testes)

## Resolução

<!-- Preencher ao concluir -->

## Definition of Done

- [ ] `Enqueue` retorna erro quando o `UPDATE` de status falha
- [ ] Quando o DB falha, o vídeo NÃO é enfileirado (queue.Len() inalterado)
- [ ] Comportamento de sucesso inalterado (UPDATE ok → enfileira normal)
- [ ] Testes novos cobrem o caminho de erro do banco
- [ ] `go test ./...` passa sem regressões
- [ ] `go vet ./...` sem warnings
