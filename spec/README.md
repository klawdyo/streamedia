# Especificação do Streamedia — Índice

Serviço Go de upload, transcodificação e entrega de vídeo em HLS.

A especificação foi dividida em arquivos temáticos pequenos para consulta
pontual — leia só o que interessa. Este índice relaciona e resume cada um.

> **Fonte de verdade:** quando a spec divergir do código, **o código vence**.
> Os arquivos abaixo descrevem o fluxo **atual** (tag + ROOT_TOKEN). O modelo
> antigo de "projetos/chave mestra" foi removido — para conhecê-lo, consulte o
> histórico do git.

| Arquivo | O que contém |
|---|---|
| [arquitetura.md](arquitetura.md) | Papel do serviço, componentes e o fluxo de dados ponta a ponta (upload → transcode → entrega). |
| [autenticacao.md](autenticacao.md) | `ROOT_TOKEN` (gestão), tokens efêmeros de upload/play (`access_tokens`), o conceito de `tag` e onde cada segredo mora. |
| [api.md](api.md) | Rotas HTTP: `/api/upload/init`, TUS `/files`, `/api/play/init`, serving `/video/<tag>/<id>.m3u8`, `/api/status`, `/admin/*`, envelope de resposta. |
| [dados.md](dados.md) | Schema SQLite (`videos`, `access_tokens`, `video_renditions`, `playback_events`, `webhook_log`) e layout de arquivos no disco. |
| [pipeline.md](pipeline.md) | Máquina de estados do vídeo, fila de transcodificação, worker FFmpeg/HLS, jobs de manutenção e recuperação de crash. |
| [webhooks.md](webhooks.md) | Eventos enviados ao backend principal e assinatura (`WEBHOOK_SECRET`). |
| [operacao.md](operacao.md) | Variáveis de ambiente, deploy (Docker/Coolify) e observabilidade (`/metrics`, `/api`). |

## Resumo de uma linha

O backend principal autentica com `ROOT_TOKEN` para iniciar uploads e emitir
URLs de play; o usuário final recebe apenas URLs efêmeras e assinadas. Cada
vídeo vive sob uma `tag` (namespace) em `<MEDIA_DIR>/<tag>/<video_id>/`.
