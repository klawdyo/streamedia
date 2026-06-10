# Pipeline: estados, fila, transcodificação e jobs

## Máquina de estados do vídeo

```
pending_upload ──► uploading ──► upload_complete ──► transcoding ──► ready
       │               │                                  │
       └──► failed_upload ◄──┘                             └──► failed_transcode
```

Transições válidas (impostas em `models.UpdateStatus`):

- `pending_upload` → `uploading`, `failed_upload`
- `uploading` → `uploading`, `upload_complete`, `failed_upload`
- `upload_complete` → `transcoding`
- `transcoding` → `transcoding`, `ready`, `failed_transcode`, `upload_complete`

`ready`, `failed_upload` e `failed_transcode` são terminais. Toda escrita de
status passa pela validação — não há `UPDATE` direto de status fora dela.

## Fila de transcodificação

Channel com workers (`TRANSCODE_WORKERS`, capacidade `QUEUE_MAX_SIZE`). Ao
concluir o upload e validar o arquivo, o vídeo é enfileirado; o worker executa
FFmpeg gerando as variantes HLS e o `master.m3u8`, registra `video_renditions`
e transiciona para `ready` (ou `failed_transcode` ao esgotar
`MAX_TRANSCODE_ATTEMPTS`). Não-upscaling: só gera resoluções ≤ à original.

## Jobs de manutenção (`internal/jobs`)

- **Killer de uploads inativos:** marca `failed_upload` os vídeos em
  `pending_upload`/`uploading` sem chunk há mais de `UPLOAD_IDLE_TIMEOUT`.
- **Reenfileirador de transcodes travados:** reenfileira vídeos presos em
  `transcoding` há mais de `TRANSCODE_STUCK` (respeitando o limite de
  tentativas; senão, `failed_transcode`).
- **Limpeza de tokens:** remove de `access_tokens` (qualquer `purpose`) os
  registros com `expires_at` no passado (varredura diária).

Todos os jobs têm shutdown gracioso (WaitGroup no `Stop`).

## Recuperação de crash

No boot, `transcode.RunStartupRecovery` varre vídeos deixados em `transcoding`:
reenfileira os que ainda têm tentativas, marca `failed_transcode` os que
atingiram o limite. Evita vídeos presos após uma queda do processo.
