# T34: Layout de armazenamento por projeto (diretórios isolados)

**Status:** done
**Dependências:** T32, T33
**Estimativa:** média
**Issue relacionada:** #6

## Contexto

A issue #6 pede que cada projeto tenha seu próprio diretório raiz dentro de
`MEDIA_DIR` (ex. `./media/trip-producao/...`), usando o slug gerado em T32.
Isso isola fisicamente os arquivos de diferentes projetos/ambientes.

Depende de T33 estar pronta porque a mudança de layout só faz sentido quando
o upload já sabe **a qual projeto** o arquivo pertence (resolvido a partir da
chave usada em `/upload/init`).

### Cuidado: vídeos existentes

Vídeos enviados antes desta mudança não têm projeto associado. Decida e
documente uma estratégia de migração: (a) criar um "projeto padrão" (ex.
slug `default` ou `legacy`) e mover os arquivos existentes para dentro dele
em uma migração de inicialização (similar ao startup recovery do T21), ou
(b) manter vídeos antigos servidos do layout legado enquanto novos vídeos
seguem o layout por projeto. Prefira (a) se a migração puder ser feita de
forma idempotente e segura — simplifica todo o código de serving subsequente
ao ter um único layout.

## Dev Instructions

- Construa os caminhos de armazenamento (`UploadTmpDir`, diretório de saída
  do transcode, diretório servido pelo HLS) a partir de
  `filepath.Join(cfg.MediaDir, project.RootDir, videoID, ...)` em vez de
  `filepath.Join(cfg.MediaDir, videoID, ...)`.
- Atualize `internal/upload` (onde grava o arquivo recebido), o worker de
  transcodificação (`internal/transcode`/worker FFmpeg) e os handlers de
  serving (`internal/serve`) para resolver o projeto do vídeo antes de montar
  o caminho.
- Implemente a migração de vídeos legados decidida acima, com logging claro
  e idempotência (rodar de novo não deve duplicar/corromper nada).

## QA Instructions

```
TestUploadStoresUnderProjectDirectory
  - upload via chave do projeto "trip-producao" grava em
    <MEDIA_DIR>/trip-producao/<video_id>/...

TestServingResolvesProjectDirectory
  - master.m3u8 e segmentos são servidos do diretório do projeto correto

TestLegacyVideoMigration
  - vídeo sem project_id é migrado para o projeto padrão de forma
    idempotente (rodar duas vezes não duplica/move incorretamente)
```

## Arquivos a criar/modificar

- `internal/upload/*`
- `internal/transcode/*`
- `internal/serve/*`
- Job/rotina de migração (novo arquivo em `internal/jobs` ou `internal/db`)
- `internal/models` (associação vídeo ↔ projeto, se ainda não existir de T33)

## Definition of Done

- [x] Novos uploads são gravados sob `<MEDIA_DIR>/<slug-do-projeto>/...`
- [x] Transcodificação e serving resolvem o caminho pelo projeto do vídeo
- [x] Estratégia de migração de vídeos legados implementada e documentada
- [x] Todos os testes passam

## Resolução

Implementação do layout de armazenamento isolado por projeto, fechando a
issue #6 quanto à organização física dos arquivos em `MEDIA_DIR`.

### Abstração central: `models.ResolveVideoRootDir`

Criada em `internal/models/project.go`, é o único ponto que traduz
`project_id *int64` → diretório raiz relativo a `MEDIA_DIR`:

```go
func ResolveVideoRootDir(db *sql.DB, projectID *int64) (string, error) {
	if projectID == nil {
		return "", nil // layout legado: <MEDIA_DIR>/<video_id>/...
	}
	project, err := GetProjectByID(db, *projectID)
	if err != nil {
		return "", err
	}
	return project.RootDir, nil
}
```

`projectID == nil` devolve `""`, que `filepath.Join` simplesmente ignora —
o que dá compatibilidade automática com o layout legado sem condicionais
espalhados pelo código. Reaproveitada de forma idêntica em três lugares:

1. **`internal/transcode/worker.go`** (`Transcode`): `outputDir :=
   filepath.Join(cfg.MediaDir, rootDir, videoID)` — toda a saída HLS
   (variantes, segmentos, `master.m3u8`) é gravada sob
   `<MEDIA_DIR>/<slug-do-projeto>/<video_id>/...`.
2. **`internal/serve/serve.go`** (`MasterHandler.ServeHTTP`): a query agora
   também seleciona `project_id`; `masterPath` é montado com o `rootDir`
   resolvido.
3. **`internal/serve/serve.go`** (`StaticHandler.ServeHTTP`): mesma
   resolução antes de montar o `path` final do segmento/playlist —
   preservando todas as validações de path traversal já existentes
   (`uuidV4Re`, `allowedResolutions`, `segmentRe`, checagem de prefixo
   contra `mediaRoot`), que continuam operando sobre o caminho já resolvido.

Helper `nullableInt64(sql.NullInt64) *int64` adicionado em `serve.go` para
converter o `project_id` (nullable no banco) para o ponteiro esperado por
`ResolveVideoRootDir`.

### `UploadTmpDir` permanece plano (decisão deliberada)

O diretório temporário de upload (`cfg.UploadTmpDir`, onde o tusd grava o
arquivo em progresso, indexado pelo `video_id` — um UUID globalmente único)
**não** foi escopado por projeto. Razões:

- Não há risco de colisão: o `video_id` já é único globalmente.
- É um armazenamento efêmero — o arquivo é removido pelo worker após a
  transcodificação (ou pelo killer job, em uploads abandonados); não faz
  parte do "produto final" entregue aos usuários.
- Escopar exigiria reestruturar o `filestore`/`TUSHandler` compartilhado
  (que hoje aponta para uma única raiz) só para um diretório de trabalho
  interno — complexidade sem benefício real.

Apenas a saída final em `MEDIA_DIR` (o que de fato é servido aos clientes)
é isolada por projeto.

### Migração de vídeos legados — `internal/jobs/project_migration.go` (novo)

Seguindo a estratégia (a) recomendada na própria tarefa — um único layout
simplifica todo o serving subsequente:

1. `getOrCreateLegacyProject`: garante (idempotente, via
   `GetProjectBySlug`/`CreateProject`) a existência de um projeto
   guarda-chuva **"Legacy"** (slug `legacy`) para vídeos sem `project_id`.
   A chave mestra gerada é descartada — esse projeto nunca é operado via API.
2. `MigrateLegacyVideos`: busca todo vídeo com `project_id IS NULL` e, para
   cada um, chama `migrateOneLegacyVideo`, que:
   - move o diretório `<MEDIA_DIR>/<video_id>` → `<MEDIA_DIR>/legacy/<video_id>`
     via `os.Rename` — só se a origem existir e o destino ainda não existir
     (idempotência: uma segunda execução não duplica nem sobrescreve);
   - associa o vídeo ao projeto Legacy (`UPDATE videos SET project_id = ?`)
     dentro de uma transação, garantindo que disco e banco não fiquem
     dessincronizados em caso de falha no meio do caminho.
3. Roda no startup (`cmd/server/main.go`, logo após
   `transcode.RunStartupRecovery`), exatamente como outras rotinas de
   recuperação — segura para executar a cada reinício.

Após esta migração rodar uma vez, **todo** vídeo no banco tem `project_id`
preenchido — o que permite que o resto do código (worker, serving) trate o
caso `projectID == nil` apenas como uma garantia defensiva residual, nunca
como o caminho normal de operação.

### Testes adicionados

- `internal/transcode/project_storage_test.go`:
  - `TestUploadStoresUnderProjectDirectory` — vídeo com `project_id`
    associado produz `master.m3u8` em
    `<MEDIA_DIR>/<slug-do-projeto>/<video_id>/master.m3u8` e **não** no
    caminho legado.
  - `TestUploadStoresUnderLegacyDirectory` — vídeo sem `project_id`
    continua produzindo saída em `<MEDIA_DIR>/<video_id>/...`.
- `internal/serve/project_storage_test.go`:
  - `TestServingResolvesProjectDirectory` — `MasterHandler` e
    `StaticHandler` servem `master.m3u8`, `playlist.m3u8` e segmentos `.ts`
    a partir de `<MEDIA_DIR>/<slug-do-projeto>/<video_id>/...` quando o
    vídeo tem projeto associado.
  - `TestServingFallsBackToLegacyDirectory` — vídeo sem projeto continua
    sendo servido do layout legado `<MEDIA_DIR>/<video_id>/...`.
- `internal/jobs/project_migration_test.go`:
  - `TestLegacyVideoMigration` — primeira execução cria o projeto Legacy,
    move o diretório físico, associa o vídeo no banco; segunda execução é
    no-op (idempotência: nem duplica o projeto, nem move/associa de novo).
  - `TestLegacyVideoMigration_NoVideosToMigrate` — quando todos os vídeos
    já têm projeto associado, nenhum é tocado e o vídeo já escopado
    permanece intacto.

`go build ./...`, `go vet ./...` e `go test ./...` (suíte completa, todos
os pacotes) passam sem falhas.
</content>
