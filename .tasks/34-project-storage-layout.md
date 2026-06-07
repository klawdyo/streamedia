# T34: Layout de armazenamento por projeto (diretórios isolados)

**Status:** pending
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

- [ ] Novos uploads são gravados sob `<MEDIA_DIR>/<slug-do-projeto>/...`
- [ ] Transcodificação e serving resolvem o caminho pelo projeto do vídeo
- [ ] Estratégia de migração de vídeos legados implementada e documentada
- [ ] Todos os testes passam
</content>
