# T28: Rota administrativa de estatísticas agregadas

**Status:** pending
**Dependências:** T26, T27, T18
**Estimativa:** média
**Issue relacionada:** #2

## Contexto

A issue #2 pede estatísticas agregadas de uso — totais e por vídeo, por
resolução, por sistema operacional, por data/hora/dia da semana — derivadas
da tabela bruta `playback_events` (T26) já populada pela instrumentação de
T27.

Esta tarefa expõe essas agregações através de uma rota administrativa
(seguindo o padrão de autenticação `AdminAuth` já usado em `/admin/videos`
e `/admin/queue`, T18), permitindo consulta sob demanda sem pré-cálculo.

### Rota

```
GET /admin/stats
GET /admin/stats?video_id={uuid}
```

- Sem `video_id`: estatísticas agregadas globais (todos os vídeos).
- Com `video_id`: estatísticas filtradas para aquele vídeo específico
  (retornar 404 se o vídeo não existir — reaproveite a checagem de
  existência já usada em outras rotas admin).

### Formato da resposta (sugestão — ajuste conforme idioma/convenção JSON do projeto)

```json
{
  "video_id": "uuid-ou-null-se-global",
  "totals": {
    "playback": 1234,
    "download_segment": 5678,
    "upload_complete": 12
  },
  "by_resolution": { "480": 100, "720": 500, "1080": 80 },
  "by_os": { "ios": 300, "android": 250, "windows": 400, "macos": 50, "linux": 30, "other": 10 },
  "by_day_of_week": { "0": 80, "1": 120, "2": 95, "3": 110, "4": 130, "5": 200, "6": 150 }
}
```

Reaproveite as funções de agregação criadas em T26
(`CountEventsByType`, `AggregateByResolution`, `AggregateByOS`,
`AggregateByDayOfWeek`). Para o filtro por `video_id`, pode ser necessário
adicionar variantes com filtro (ex. `AggregateByOSForVideo`,
`AggregateByDayOfWeekForVideo`) — avalie se vale a pena generalizar as
funções de T26 com um parâmetro `videoID string` opcional (string vazia =
todos os vídeos) em vez de duplicar; se optar por alterar as assinaturas de
T26, atualize os testes existentes de `stats_test.go`.

## QA Instructions

Crie `internal/admin/stats_test.go`:

```
TestStatsRoute_RequiresAdminAuth
  - GET /admin/stats sem header Authorization → 401

TestStatsRoute_GlobalAggregation
  - Insere eventos variados (tipos, resoluções, OS, datas diferentes)
  - GET /admin/stats com token válido → 200
  - Resposta contém totals, by_resolution, by_os, by_day_of_week corretos

TestStatsRoute_FilteredByVideoID
  - Insere eventos para dois vídeos distintos
  - GET /admin/stats?video_id={vid1} → retorna apenas agregações de vid1

TestStatsRoute_UnknownVideoID
  - GET /admin/stats?video_id={uuid-inexistente} → 404

TestStatsRoute_EmptyDataset
  - Sem eventos inseridos → 200 com mapas/contadores vazios (não erro)
```

## Dev Instructions

- Adicione o handler em `internal/admin/` (mesmo pacote de T18), seguindo o
  padrão de injeção de dependências (`AdminHandler` com `db *sql.DB`).
- Registre a rota `/admin/stats` no grupo já protegido por `AdminAuth` na
  montagem do servidor (T20).
- Reaproveite `respondError`/helpers JSON já existentes no pacote `admin`
  (ou no padrão usado em `serve`), mantendo consistência de formato de erro.
- Para checar existência de `video_id`, reaproveite `models.GetVideo` (ou
  equivalente de T04).
- Lembre-se: todas as queries de agregação devem normalizar datas via
  `datetime()`/`strftime()` (já garantido pelas funções de T26, desde que
  você não introduza novas comparações de data diretamente nesta rota).

## Arquivos a criar/modificar

- `internal/admin/stats.go` (novo handler)
- `internal/admin/stats_test.go`
- `internal/models/stats.go` (se optar por generalizar as agregações com filtro por vídeo)
- `internal/models/stats_test.go` (se as assinaturas de T26 mudarem)
- Arquivo de montagem do servidor (registro da rota `/admin/stats`)

## Definition of Done

- [ ] Rota `/admin/stats` protegida por `AdminAuth`
- [ ] Agregação global funciona corretamente (totals, resolução, OS, dia da semana)
- [ ] Filtro por `video_id` funciona e retorna 404 para vídeo inexistente
- [ ] Dataset vazio retorna 200 com estruturas vazias (não erro/500)
- [ ] Resposta em JSON consistente com o padrão das demais rotas admin
- [ ] Todos os testes passam
</content>
