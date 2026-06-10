# T72 — Gerar thumbnails (poster) por resolução ao final da transcodificação

- **Status:** done
- **Issue relacionada:** #19
- **Depende de:** T11 (worker FFmpeg), T12/T13 (serving + status)

## Objetivo

Após gerar as variantes HLS, extrair um frame representativo do vídeo e gravar
um thumbnail JPEG **por resolução** gerada, preservando a proporção original.
Expor `has_thumbnails` e a lista de URLs no `GET /api/status/{video_id}` e
servir os thumbnails por uma rota pública (sem autenticação).

## Escopo (da issue #19)

- Extrair frame a **1 segundo** do vídeo; fallback para o primeiro keyframe
  (frame em 0s) quando não houver frame em 1s (vídeo curto).
- Um thumbnail por resolução gerada (480/720/1080), respeitando:
  - a resolução alvo (480p → menor dimensão = 480);
  - a proporção original (16:9, 9:16, 4:3, …).
- Formato JPEG, qualidade ~80%.
- Disco: `<MEDIA_DIR>/<tag>/<video_id>/thumb_<resolucao>.jpg`.
- `has_thumbnails` (bool) + lista de URLs no payload de status.
- Servir via rota pública (poster é público por natureza).

## Fora de escopo

- Sprite sheet (thumbnail strip) para preview no hover.
- Thumbnails animados (GIF/WebP).
- Thumbnail para uploads que falharam na transcodificação.

## Decisões de implementação

1. **Geração best-effort.** Thumbnails são auxiliares (poster). Uma falha na
   geração é apenas logada e **nunca** reverte uma transcodificação já
   concluída — mesmo princípio do `scanRenditionDir` (estatísticas, T36).
2. **Frame por resolução a partir do ORIGINAL.** O frame é extraído sempre do
   arquivo de entrada original (melhor qualidade), reescalado por resolução.
   Por isso a geração ocorre **antes** da remoção do original (passo 10).
3. **Proporção preservada via a menor dimensão = "p".** Espelha
   `determineResolutions`: landscape 16:9 em 480p → 854×480; portrait 9:16 em
   480p → 480×854. Dimensões arredondadas para par.
4. **`-ss` antes do `-i` (input seeking).** Rápido e cai no keyframe mais
   próximo de 1s — que é exatamente o "fallback para o primeiro keyframe"
   pedido. Se falhar (vídeo < 1s), refaz com `-ss 0`.
5. **`has_thumbnails` derivado do disco, sem coluna nova.** O status verifica a
   existência de `thumb_<res>.jpg` para cada resolução do vídeo. Evita
   migração e o risco histórico de coluna esquecida em algum SELECT (ver T53);
   a fonte de verdade é o disco — sempre coerente com o que a rota pública
   serve.
6. **Rota pública** `GET /video/<tag>/<id>/thumb_<res>.jpg` (3 segmentos),
   distinta das rotas de master (2 segmentos) e segmento (4 segmentos). Mesma
   blindagem de path traversal do `StaticHandler`.

## Definition of Done

- [x] `models.ThumbnailNameRe` + `models.ThumbnailFileName` centralizados em
      `internal/models/hls.go`.
- [x] Geração de thumbnails no worker (`internal/transcode/thumbnail.go`),
      chamada no `Transcode` antes de remover o original.
- [x] `httputil.PublicThumbnailURL`.
- [x] `GET /api/status/{id}` retorna `has_thumbnails` + `thumbnails`
      (resolução→URL).
- [x] `ThumbnailHandler` público + rota registrada no `server.go`.
- [x] Testes: cálculo de escala, args do FFmpeg, geração com mock, handler de
      serving, e status com/sem thumbnails.
- [x] Spec (`spec/api.md`) e `api.http` atualizados.

## Resolução

Implementação concluída. Arquivos:

- **`internal/models/hls.go`** — `ThumbnailNameRe`
  (`^thumb_(480|720|1080)\.jpg$`) e `ThumbnailFileName(res)`, centralizados
  junto de `SegmentNameRe` (mesmas duas pontas: geração e serving).
- **`internal/transcode/thumbnail.go`** (novo) — `thumbnailScale` (proporção
  preservada, menor dimensão = "p", dimensões pares), `buildThumbnailArgs`
  (`-ss <s>` antes do `-i` para input seeking + keyframe-fallback, `-frames:v 1`,
  `scale`, `-q:v 5` ≈ JPEG 80%, `-f image2`), `generateThumbnails`
  (best-effort, um por resolução, com fallback `-ss 1` → `-ss 0`).
- **`internal/transcode/worker.go`** — passo 7.5 chama `generateThumbnails`
  a partir do ORIGINAL, antes da remoção do input (passo 10).
- **`internal/serve/thumbnail.go`** (novo) — `ThumbnailHandler` público
  (sem auth), mesma blindagem de path traversal do `StaticHandler`.
- **`internal/serve/status.go`** — `has_thumbnails` + `thumbnails`
  (resolução→URL), derivados do disco via `collectThumbnails`.
- **`internal/httputil/url.go`** — `PublicThumbnailURL`.
- **`internal/server/server.go`** — rota `GET /video/{tag}/{videoID}/{thumb}`
  (3 segmentos, distinta do master de 2 e do segmento de 4).

### Decisões e descobertas

- **Qualidade JPEG via mjpeg `-q:v`.** O FFmpeg não expõe a escala 0–100 do
  libjpeg para o mjpeg; usa 1 (melhor)–31 (pior). `q=5` ≈ 80% — aproximação
  documentada em `jpegQScale`.
- **`has_thumbnails` derivado do disco, sem coluna nova** (evita migração e o
  risco de SELECT esquecido, ver T53). A rota pública e o status leem a mesma
  fonte de verdade.
- **Frame extraído do original**, reescalado por resolução — melhor qualidade
  que partir de cada variante já comprimida.

### Testes

- `internal/transcode/thumbnail_test.go` — `thumbnailScale` (16:9, 9:16, 1:1,
  4:3, sem dimensões), `buildThumbnailArgs` (input seeking), `generateThumbnails`
  (um por resolução; fallback `-ss 1`→`-ss 0`). Mock `mockFFmpeg` ganhou
  `runFunc` para comportamento por-chamada.
- `internal/serve/thumbnail_test.go` — serve existente, 404, video_id inválido,
  filenames rejeitados (resolução não suportada, extensão, traversal).
- `internal/serve/status_test.go` — `has_thumbnails` false sem arquivos; lista
  só as resoluções presentes no disco com URL pública correta.

### Pré-existentes (NÃO causados por esta task)

Três testes já falhavam em `origin/main` por dependerem de comportamento
POSIX em Windows (separador de path / remoção de arquivo):
`TestBuildFFmpegArgs_MinimalArgs` (transcode),
`TestPostFinishValidation_InvalidMagicBytes` e `_SizeMismatch` (upload).
Confirmado rodando-os no checkout limpo de `main`. Fora do escopo da issue #19.
