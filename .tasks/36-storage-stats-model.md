# T36: Model de armazenamento por vídeo (bytes, duração, status)

**Status:** done
**Dependências:** T03, T04 — recomenda-se também depois de T34 (ver nota)
**Estimativa:** média
**Issue relacionada:** #5

## Contexto

A issue #5 pede estatísticas de **armazenamento e fila**, distintas das
estatísticas de **uso/reprodução** já cobertas por T26-T28 (issue #2,
fechada): quanto espaço cada vídeo ocupa (MB), quantos minutos de vídeo estão
armazenados ao todo, quantos arquivos estão pendentes/em processamento/prontos,
etc. A issue enfatiza que isso só é possível se "cada chunk ou arquivo salvo
tiver sua própria linha de dados no banco, com índices e valores corretos".

### Decisão de granularidade: por vídeo, não por chunk do TUS

Os chunks do protocolo TUS são temporários (descartados após a montagem do
arquivo final em `UploadTmpDir` — ver T07/T09) — não há valor em persistir uma
linha por chunk de upload. O que a issue realmente precisa (somar MB usados,
minutos armazenados, contagem por status) é responder a partir dos dados que
**já existem** por vídeo (`videos.size_bytes`? confirme o schema atual em
`internal/models/video.go` — se o tamanho do arquivo original e a duração não
estiverem armazenados, esta tarefa adiciona essas colunas) mais os artefatos
HLS gerados (soma dos tamanhos dos segmentos por resolução).

Portanto: granularidade **por vídeo + por variante de resolução gerada**, não
por chunk — atende ao pedido da issue (uma linha por arquivo salvo) com bem
menos overhead e sem reter dados efêmeros.

### Nota sobre T34

Se a T34 (layout de armazenamento por projeto) for feita antes, os caminhos
escaneados aqui já estarão sob o diretório do projeto — sem isso, o cálculo
ainda funciona (escaneia o layout atual), só precisará de um ajuste pequeno
depois. Não é um bloqueio rígido, mas evita recomputar a lógica de paths duas
vezes.

## Dev Instructions

- Adicione (se ainda não existirem) colunas em `videos`: `size_bytes`
  (tamanho do arquivo original) e `duration_seconds`. Preencha-as no momento
  da validação pós-upload (T09 — onde o FFprobe já roda) e/ou no fim da
  transcodificação.
- Crie uma tabela `video_renditions` (ou nome equivalente): `video_id`,
  `resolution`, `size_bytes` (soma dos segmentos da variante), `segment_count`
  — preenchida pelo worker FFmpeg (T11) ao concluir cada variante.
- Crie `internal/models/storage.go` com funções de leitura/agregação:
  `TotalStorageBytes`, `TotalDurationSeconds`, `CountVideosByStatus`,
  `StorageByVideo(videoID)`.

## QA Instructions

Crie `internal/models/storage_test.go`:

```
TestVideoRenditions_PersistsSizeAndSegmentCount

TestTotalStorageBytes_SumsOriginalsAndRenditions

TestTotalDurationSeconds_SumsAcrossVideos

TestCountVideosByStatus_GroupsCorrectly
  - cria vídeos em diferentes estados (pending_upload, transcoding, ready,
    failed) e confere as contagens por status
```

## Arquivos a criar/modificar

- `internal/db/schema.go` (colunas novas em `videos` + tabela `video_renditions`)
- `internal/models/video.go` (gravação de `size_bytes`/`duration_seconds`)
- `internal/models/storage.go`
- `internal/models/storage_test.go`
- `internal/upload/validation.go` (preenche `size_bytes`/`duration_seconds` via FFprobe)
- `internal/transcode/*` (preenche `video_renditions` ao concluir cada variante)

## Definition of Done

- [x] `videos` armazena tamanho original e duração
- [x] `video_renditions` registra tamanho e nº de segmentos por variante gerada
- [x] Funções de agregação cobrindo: total em bytes, total em minutos,
      contagem por status
- [x] Todos os testes passam

## Resolução

### Descoberta: `videos.actual_size_bytes`/`duration_s` já existiam

A "Decisão de granularidade" deste arquivo já desconfiava disso ("confirme o
schema atual em `internal/models/video.go`") — e de fato, `videos` já possui
`actual_size_bytes` (preenchido por `SetUploadComplete`, no fluxo de validação
pós-upload da T09) e `duration_s` (preenchido por `SetReady`, ao final da
transcodificação, com a duração extraída via `probeVideo`/ffprobe). Portanto
**nenhuma alteração foi necessária em `internal/models/video.go` ou
`internal/upload/validation.go`** — as colunas pedidas pela tarefa já existem
sob nomes equivalentes e já são preenchidas nos pontos certos do pipeline. O
trabalho genuinamente novo desta tarefa é a tabela `video_renditions` e as
funções de agregação.

### Nova tabela `video_renditions` (`internal/db/schema.go`)

Uma linha por combinação `(video_id, resolution)` — chave primária composta —
com `size_bytes` (soma dos segmentos `.ts` da variante) e `segment_count`.
A PK composta permite usar `INSERT ... ON CONFLICT ... DO UPDATE` (UPSERT):
reprocessar um vídeo (re-transcodificação) substitui a linha existente em vez
de duplicá-la — a granularidade "por vídeo + por variante de resolução
gerada" descrita no Contexto, e não por chunk do TUS (efêmeros, sem valor
analítico duradouro, conforme já documentado na tarefa).

### `internal/models/storage.go` (novo arquivo)

- `VideoRendition` — struct espelhando uma linha de `video_renditions`.
- `UpsertVideoRendition(db, videoID, resolution, sizeBytes, segmentCount)` —
  grava/substitui via `INSERT ... ON CONFLICT (video_id, resolution) DO
  UPDATE`.
- `StorageByVideo(db, videoID)` — lista as variantes de um vídeo ordenadas
  por resolução ("ficha de armazenamento" de um vídeo específico).
- `TotalStorageBytes(db)` — soma `videos.actual_size_bytes` (originais) +
  `video_renditions.size_bytes` (variantes geradas) — responde "quantos MB
  estão armazenados ao todo".
- `TotalDurationSeconds(db)` — soma `videos.duration_s` — responde "quantos
  minutos de vídeo estão armazenados ao todo" (cada vídeo conta uma única
  vez; variantes compartilham a duração do original).
- `CountVideosByStatus(db)` — agrupa contagem de vídeos por `status` —
  responde "quantos arquivos estão pendentes/em processamento/prontos/com
  falha".

Todas usam `COALESCE`/inicialização segura para não quebrar quando não há
dados (ex.: `duration_s` NULL em vídeos ainda não transcodificados).

### Worker FFmpeg — preenchimento de `video_renditions` (`internal/transcode/worker.go`)

Adicionada `scanRenditionDir(resDir)`: varre o diretório de saída de uma
variante recém-gerada, soma o tamanho dos segmentos `.ts` (via
`renditionSegmentRe`, o mesmo padrão `^[0-9]+\.ts$` usado no serving) e conta
quantos existem — ignorando deliberadamente o `playlist.m3u8` (fração
desprezível e variável, que não representa o "peso" real do vídeo). Chamada
logo após cada `ffmpeg.Run` bem-sucedido no laço de resoluções de
`Transcode`, persistindo o resultado via `models.UpsertVideoRendition`.
Falhas aqui (erro ao ler diretório, erro ao gravar no banco) são logadas mas
NÃO interrompem a transcodificação — são estatísticas auxiliares, e o vídeo
não deve falhar por causa delas.

### Testes — `internal/models/storage_test.go`

- `TestVideoRenditions_PersistsSizeAndSegmentCount` — grava duas variantes,
  confere os valores, e confirma que re-transcodificar a mesma resolução
  SUBSTITUI a linha (UPSERT) em vez de duplicar.
- `TestTotalStorageBytes_SumsOriginalsAndRenditions` — confere que o total
  soma corretamente originais (`actual_size_bytes`) + variantes
  (`size_bytes`).
- `TestTotalDurationSeconds_SumsAcrossVideos` — soma duração de vários
  vídeos, incluindo um sem duração (NULL), confirmando o `COALESCE`.
- `TestCountVideosByStatus_GroupsCorrectly` — cria vídeos em
  `pending_upload`, `transcoding`, `ready` e `failed_transcode` (dois deste
  último, via `UpdateStatusWithError` como o worker realmente faz) e confere
  as contagens por status e o total.

`go build ./...`, `go vet ./...` e `go test ./...` (suíte completa) passam
sem falhas.
</content>
