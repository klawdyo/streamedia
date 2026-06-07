# T36: Model de armazenamento por vídeo (bytes, duração, status)

**Status:** pending
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

- [ ] `videos` armazena tamanho original e duração
- [ ] `video_renditions` registra tamanho e nº de segmentos por variante gerada
- [ ] Funções de agregação cobrindo: total em bytes, total em minutos,
      contagem por status
- [ ] Todos os testes passam
</content>
