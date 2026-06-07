# T27: Coleta de eventos de estatísticas nos handlers de serving/upload

**Status:** pending
**Dependências:** T26, T07, T09, T12
**Estimativa:** média
**Issue relacionada:** #2

## Contexto

T26 criou a fundação (tabela `playback_events` e `RecordEvent`/agregações),
mas nenhum evento é gravado ainda — falta conectar `RecordEvent` aos pontos
reais de acesso: download de segmentos HLS, acesso ao master playlist e
conclusão de upload.

Esta tarefa "liga os fios": cada requisição relevante deve gerar exatamente
um registro em `playback_events`, com o `resolution`, `event_type`,
`user_agent` e `os_family` corretos — sem bloquear ou atrasar a resposta ao
cliente.

### Pontos de instrumentação

1. **`internal/serve.MasterHandler.ServeHTTP`** (`internal/serve/serve.go`)
   — ao servir `master.m3u8` com sucesso (200), registrar evento
   `event_type = "playback"`, `resolution = NULL` (o master não tem
   resolução própria).

2. **`internal/serve.StaticHandler.ServeHTTP`** (`internal/serve/serve.go`)
   — ao servir um segmento `.ts` com sucesso, registrar
   `event_type = "download_segment"` com a `resolution` extraída do path
   (480/720/1080). Ao servir uma playlist de resolução (`{res}/playlist.m3u8`),
   pode-se opcionalmente registrar como `playback` com a resolução — decida
   e documente com um comentário (sugestão: registrar apenas o download de
   segmentos `.ts`, pois é o evento mais representativo de consumo real,
   evitando duplicidade com a entrega do master).

3. **Hook de upload concluído** — localize onde o upload é marcado como
   `validated`/concluído (T09, hook pós-`finish` do tusd) e registre
   `event_type = "upload_complete"`, `resolution = NULL`, com o
   `user_agent` da requisição de finalização (se disponível no contexto do
   hook; caso não esteja disponível, registre com string vazia e comente o
   porquê).

### Requisitos não-funcionais

- **Não bloquear a resposta**: grave o evento de forma assíncrona (ex.
  `go func() { ... }()` com tratamento de erro via log) ou, no mínimo,
  *após* `w.Write`/envio do conteúdo — a gravação no banco não deve atrasar
  a entrega do vídeo.
- **Não falhar a requisição por erro de gravação de estatística**: erros em
  `RecordEvent` devem ser logados (`log.Printf` ou logger do projeto) e
  silenciosamente ignorados do ponto de vista do cliente.
- Extraia o `User-Agent` via `r.Header.Get("User-Agent")`.

## QA Instructions

Crie/estenda testes em `internal/serve/serve_test.go` (ou novo arquivo
`internal/serve/stats_integration_test.go`):

```
TestMasterHandler_RecordsPlaybackEvent
  - Faz uma requisição autenticada e bem-sucedida ao master.m3u8
  - Verifica (após sincronizar/aguardar) que um registro "playback" com
    resolution NULL foi inserido para o video_id correto

TestStaticHandler_RecordsSegmentDownloadEvent
  - Requisita um segmento .ts de uma resolução válida (ex. 720)
  - Verifica que um registro "download_segment" com resolution=720 foi
    inserido

TestStaticHandler_DoesNotRecordOnAuthFailure
  - Requisição com token/autenticação inválida (ou vídeo inexistente)
  - Verifica que NENHUM evento foi inserido (apenas sucessos contam)

TestUploadCompleteRecordsEvent
  - Simula/aciona o hook de finalização de upload
  - Verifica que um registro "upload_complete" foi inserido para o video_id
```

Para lidar com a gravação assíncrona nos testes, use um pequeno
`time.Sleep` controlado ou — preferencialmente — torne o ponto de gravação
testável via uma função/canal injetável (ex. callback opcional chamado após
`RecordEvent` retornar), evitando flakiness.

## Dev Instructions

- Adicione um campo `db *sql.DB` (se ainda não existir) aos handlers que
  precisam registrar eventos — `MasterHandler` e `StaticHandler` já recebem
  `db`; reaproveite.
- Centralize a lógica de "extrair resolução do path + disparar RecordEvent"
  em uma função auxiliar não-exportada (ex. `recordPlaybackAsync`) para
  evitar duplicação entre os handlers.
- No hook de upload (T09), verifique se o handler já tem acesso a `*sql.DB`
  e ao `*http.Request` no momento da validação pós-finish; se não tiver
  acesso direto ao request, registre com `user_agent = ""` e comente.
- Use `log.Printf("[stats] erro ao registrar evento: %v", err)` (ou padrão
  de log já usado no projeto) em caso de falha — nunca `panic` ou retornar
  erro ao cliente por causa de estatísticas.

## Arquivos a criar/modificar

- `internal/serve/serve.go` (instrumentação de MasterHandler e StaticHandler)
- `internal/serve/serve_test.go` ou novo arquivo de teste de integração
- Handler/hook de upload (T09) — local exato a identificar durante a implementação

## Definition of Done

- [ ] Acesso ao master.m3u8 gera evento `playback` (resolution NULL)
- [ ] Download de segmento `.ts` gera evento `download_segment` com resolução correta
- [ ] Conclusão de upload gera evento `upload_complete`
- [ ] Falhas de autenticação/autorização NÃO geram eventos
- [ ] Gravação de evento não bloqueia nem atrasa a resposta ao cliente
- [ ] Erros de gravação são logados e não propagam ao cliente
- [ ] Todos os testes passam
</content>
