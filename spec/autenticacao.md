# Autenticação, tags e segredos

Há **duas camadas** de credencial, que nunca se misturam.

## Camada de gestão — `ROOT_TOKEN`

Credencial única, em `Authorization: Bearer <ROOT_TOKEN>`. Só o backend
principal a detém. Protege:

- `POST /api/upload/init`
- `POST /api/play/init`
- `GET /api/status/{video_id}`
- todas as rotas `/admin/*`

Comparada em tempo constante. Não tem vínculo com nenhum dado — **pode ser
trocada a qualquer momento** (mudar o env e reiniciar).

## Camada de capacidade — tokens efêmeros

Emitidos pela camada de gestão e entregues ao cliente final. São strings
aleatórias opacas, persistidas na tabela `access_tokens` com uma coluna
`purpose`, e **validadas por lookup no banco** (sem HMAC):

- **`upload`** — autoriza o envio dos bytes de **um** vídeo via TUS
  (header `Upload-Token`). TTL `UPLOAD_TOKEN_TTL`.
- **`play`** — autoriza a leitura do `master.m3u8` de **um** vídeo
  (query `?token=`). TTL `PLAY_TOKEN_TTL`.

O `purpose` é checado no uso: um token de play **nunca** autoriza upload e
vice-versa. `UNIQUE(video_id, purpose)` garante no máximo um token ativo de
cada propósito por vídeo (reemitir substitui o anterior — rotação natural).

## Tag

`tag` é o **namespace organizacional** de um vídeo (coluna `videos.tag`,
indexada). Define o diretório de armazenamento (`<MEDIA_DIR>/<tag>/<video_id>/`)
e agrupa vídeos para consultas (`/admin/videos?tag=...`, estatísticas). É
normalizada para slug (`models.Slugify`), o que também neutraliza path
traversal. **Não é credencial.**

## Onde cada segredo mora

| Segredo | Papel | Casa | Compartilhado? |
|---|---|---|---|
| `ROOT_TOKEN` | gestão (Bearer) | env | não |
| `WEBHOOK_SECRET` | assina webhooks (HMAC) | env | **sim** (backend valida) |
| tokens de upload/play | capacidades efêmeras | linha em `access_tokens` | — |

> Não há mais segredo de assinatura de play (era HMAC): o token de play é uma
> linha no banco. O `WEBHOOK_SECRET` é o único segredo HMAC, porque o outro
> lado precisa validar a assinatura.
