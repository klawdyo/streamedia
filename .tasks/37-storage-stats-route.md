# T37: Expor estatísticas de armazenamento e fila (`/admin/stats`)

**Status:** pending
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

- [ ] `/admin/stats` devolve totais de armazenamento (bytes e minutos)
- [ ] `/admin/stats` devolve contagem de vídeos por status e fila pendente
- [ ] README e spec OpenAPI atualizados
- [ ] Issue #5 fechada
- [ ] Todos os testes passam
</content>
