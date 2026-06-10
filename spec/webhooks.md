# Webhooks

O Streamedia notifica o backend principal (`WEBHOOK_URL`) a cada transição
relevante. É a única comunicação de saída e usa o único segredo compartilhado.

## Eventos

| Evento | Quando |
|---|---|
| `processing` | upload validado e enfileirado para transcodificação. |
| `ready` | transcodificação concluída com sucesso. |
| `failed` | falha de upload (validação) ou de transcodificação. |

(Há também o evento interno de estatística `upload_complete`, registrado em
`playback_events`; não é necessariamente um webhook de negócio.)

## Payload

JSON com:

```json
{
  "video_id": "<uuid>",
  "event": "ready",
  "status": "ready",
  "timestamp": "2026-01-01T12:00:00Z",
  "duration_s": 120,
  "resolutions": [480, 720],
  "error_message": null
}
```

`duration_s` e `error_message` são `null` quando não se aplicam;
`resolutions` é `[]` quando ainda não há variantes.

## Assinatura

Cada requisição inclui o header `X-Signature` com o **HMAC-SHA256** do corpo,
assinado com `WEBHOOK_SECRET` (`auth.SignWebhook`). O backend principal deve
recalcular o HMAC com o mesmo segredo e comparar em tempo constante.

## Entrega

Até 3 tentativas com backoff exponencial (1s, 2s, 4s). Cada tentativa é
registrada em `webhook_log`. Timeout por requisição: 10s (context).
