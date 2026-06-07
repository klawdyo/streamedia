# T37: Expor estatísticas de armazenamento e fila (`/admin/stats`)

**Status:** done
**Dependências:** T36, T28
**Estimativa:** pequena
**Issue relacionada:** #5 — fecha a issue #5

## Contexto

Conclui a issue #5 somando, à rota `/admin/stats` já existente (T28 — hoje
focada em estatísticas de **uso**: totais por tipo de evento, por resolução,
por SO, por dia da semana), as agregações de **armazenamento e fila**
calculadas em T36: espaço total usado, minutos de vídeo armazenados, fila de
processamento pendente, contagem de vídeos por status.

Reaproveita a rota existente em vez de criar uma nova — evita fragmentar a
visão administrativa de estatísticas em múltiplos endpoints.

## Dev Instructions

- Estenda `statsResponse` (`internal/admin/stats.go`) com uma seção nova,
  por exemplo:

```json
{
  "storage": {
    "total_bytes": 123456789,
    "total_duration_seconds": 7384,
    "videos_by_status": {"pending_upload": 2, "transcoding": 1, "ready": 40, "failed": 1},
    "queue_pending": 1
  }
}
```

- `queue_pending` reaproveita `queue.Len()` (mesma fonte do `/admin/queue` e
  do gauge `streamedia_transcode_queue_length` do T29 — não recompute).
- Aceite o mesmo parâmetro `?video_id=` já suportado: quando presente, talvez
  faça sentido omitir a seção `storage` (que é uma visão agregada global) ou
  devolver apenas os dados daquele vídeo — escolha uma e documente com um
  comentário curto explicando o porquê.

## QA Instructions

Estenda `internal/admin/stats_test.go`:

```
TestHandleStats_IncludesStorageSection
  - confere presença e valores de total_bytes, total_duration_seconds,
    videos_by_status e queue_pending na resposta global

TestHandleStats_StorageSectionConsistentWithQueueRoute
  - queue_pending bate com o valor devolvido por /admin/queue
```

## Arquivos a criar/modificar

- `internal/admin/stats.go`
- `internal/admin/stats_test.go`
- `README.md` (seção "GET /admin/stats" — documentar o novo bloco `storage`)
- `internal/docs/spec.go` (atualizar o schema de resposta de `/admin/stats`
  na especificação OpenAPI do T30)

## Definition of Done

- [x] `/admin/stats` devolve totais de armazenamento (bytes e minutos)
- [x] `/admin/stats` devolve contagem de vídeos por status e fila pendente
- [x] README e spec OpenAPI atualizados
- [x] Issue #5 fechada
- [x] Todos os testes passam

## Resolução

Estendida a rota `/admin/stats` (já existente, T28) com uma nova seção
`storage`, reaproveitando integralmente as agregações criadas em T36 — sem
fragmentar a visão administrativa em endpoints separados, conforme sugerido
no Contexto.

### `internal/admin/stats.go`

- `statsResponse` ganhou o campo `Storage *storageStats` com
  `json:"storage,omitempty"`.
- Novo tipo `storageStats`: `total_bytes`, `total_duration_seconds`,
  `videos_by_status` (mapa `models.VideoStatus → int`) e `queue_pending`.
- Nova função `buildStorageStats(db, queue)` monta a seção chamando
  diretamente `models.TotalStorageBytes`, `models.TotalDurationSeconds` e
  `models.CountVideosByStatus` (T36), e `queue.Len()` para `queue_pending` —
  a MESMA fonte usada por `HandleQueue`/`/admin/queue` e pelo gauge
  `streamedia_transcode_queue_length` (T29). Não recomputa a fila por
  nenhum caminho alternativo, garantindo que as três rotas sempre reportem
  o mesmo valor.

### Decisão sobre `?video_id=`: omitir a seção `storage` no filtro por vídeo

A tarefa pedia para escolher entre "omitir a seção" ou "devolver apenas os
dados daquele vídeo" e documentar o porquê. Optei por **omitir**
(`omitempty`, `Storage` permanece `nil`): a seção `storage` é, por
natureza, uma visão **agregada GLOBAL** — `total_bytes`/`total_duration_seconds`
somam todos os vídeos, `videos_by_status` agrupa todo o catálogo, e
`queue_pending` é o tamanho de UMA fila compartilhada (não haveria como
"filtrar a fila por vídeo" de forma coerente). Devolver os mesmos totais
globais "disfarçados" de resposta filtrada criaria ambiguidade real: o
cliente poderia razoavelmente supor que `queue_pending` ou `total_bytes`
refletem apenas aquele vídeo, quando na verdade são números do sistema
inteiro. Omitir é a opção que preserva o contrato claro: a seção `storage`
SEMPRE é uma foto do todo, presente apenas na visão sem filtro. Esta decisão
está documentada com um comentário no código, no `README.md` e na descrição
do schema na spec OpenAPI.

(Quem precisar de estatísticas de armazenamento de UM vídeo específico já
tem `GET /admin/stats?video_id=X` → ainda retorna `totals`/`by_resolution`/
etc. filtrados, e `StorageByVideo` de T36 está disponível como função de
modelo para uma eventual rota dedicada — não fazia parte do escopo desta
tarefa expor uma "ficha de armazenamento por vídeo" via HTTP.)

### Testes — `internal/admin/stats_test.go`

- `TestHandleStats_IncludesStorageSection` — popula `actual_size_bytes`/
  `duration_s`/`video_renditions`, vídeos em status diferentes e uma fila
  mock com tamanho 2; confere que a seção `storage` aparece na resposta
  global com `total_bytes`, `total_duration_seconds`, `videos_by_status` e
  `queue_pending` corretos.
- `TestHandleStats_StorageSectionConsistentWithQueueRoute` — chama
  `HandleStats` e `HandleQueue` com a mesma fila mock e confere que
  `storage.queue_pending` (de `/admin/stats`) e `queue_length` (de
  `/admin/queue`) são idênticos — prova de que ambos vêm de `queue.Len()`,
  sem recomputo paralelo.

### Documentação

- `README.md`: nova subseção em "GET /admin/stats" documentando o bloco
  `storage` (exemplo JSON completo, explicação de cada campo e a decisão de
  omiti-lo no filtro por `video_id`).
- `internal/docs/spec.go`: schema de resposta de `/admin/stats` ganhou a
  propriedade `storage` (objeto `nullable`, com suas quatro subpropriedades
  documentadas) e a `description` da rota foi atualizada — testado por
  `TestOpenAPISpecIsValidJSON` e `TestOpenAPISpecDocumentsKnownRoutes`.

### Issue #5 — fechamento

Com T36 (model de agregação) e T37 (exposição via `/admin/stats`)
completos, a issue #5 ("estatísticas de armazenamento e fila") está
totalmente atendida: espaço ocupado por vídeo e no total, minutos
armazenados, contagem de vídeos por status e tamanho da fila — tudo
acessível por uma única chamada administrativa autenticada.

`go build ./...`, `go vet ./...` e `go test ./...` (suíte completa) passam
sem falhas.
</content>
