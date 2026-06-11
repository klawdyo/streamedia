# T74 — Webhook do Discord para notificação de erros operacionais

- **Status:** done
- **Issue relacionada:** #21
- **Depende de:** T10 (fila), T11 (worker), T15 (job de requeue)

## Objetivo

Criar um canal de **alerta operacional** (Discord) para falhas internas que
comprometem o serviço — distinto dos webhooks de negócio (ciclo de vida do
vídeo), que já passam por `WEBHOOK_URL`.

## Escopo (da issue #21)

- Nova variável de ambiente opcional `DISCORD_WEBHOOK_URL`.
- Disparar notificação no Discord quando ocorrerem:
  - Falha na transcodificação (`failed_transcode`).
  - Fila de transcodificação cheia (bloqueio de novos jobs).
  - Transcode travado detectado pelo job de manutenção (`TRANSCODE_STUCK`).
  - Aumento anormal de falhas consecutivas.
- Formato: embed Discord com `video_id`, `status`, `error_message`, timestamp.
- Não obrigatório — sem a variável, nenhum envio é tentado.
- Registrar tentativas (sucesso/falha) nos logs da aplicação.

## Fora de escopo

- Webhooks do Discord para eventos de negócio (ex.: `video ready`).
- Canais além do Discord (Slack, Telegram, etc.).

## Decisões de implementação

1. **Pacote dedicado `internal/discord`.** `NewAlerter(url)` devolve `nil` se a
   URL é vazia; todos os métodos são **nil-safe** (no-op em receptor nil), então
   o canal desabilitado não exige nenhum `if` nos pontos de chamada.
2. **Injeção via setters (`SetAlerter`), não construtores.** Worker, Queue e o
   job de requeue ganham um campo `*discord.Alerter` e um setter. Evita alterar
   ~40 chamadas de teste de `NewWorker`/`NewQueue`/`NewTranscodeRequeueJob`.
   O `main` cria o alerter uma vez e o injeta nos três.
3. **Gatilhos:**
   - `failed_transcode`: `worker.handleTranscodeFailure` (falha terminal) e
     `requeue.runOnce` (terminal por esgotar tentativas).
   - fila cheia: `queue.Enqueue` no ramo `default` (buffer cheio).
   - transcode travado: `requeue.runOnce` para cada vídeo travado detectado.
   - falhas consecutivas: contador thread-safe **dentro do Alerter**; cada
     `AlertTranscodeFailure` incrementa, `RecordTranscodeSuccess` (chamado pelo
     worker no `ready`) zera. Ao atingir o limiar (5), dispara um alerta extra
     de "aumento anormal" e reinicia o contador.
4. **Best-effort.** `send` registra sucesso/falha em log (`[discord] ...`),
   timeout de 10s, e nunca propaga erro nem bloqueia o pipeline. Aceita 2xx
   (o Discord responde `204`).
5. **Recovery de boot não dispara alerta.** `RunStartupRecovery` ficou fora
   (assinatura intocada): os gatilhos de runtime já cobrem os quatro itens da
   issue; alertar falhas terminais no boot seria ruído de inicialização.

## Arquivos alterados

- `internal/discord/discord.go` (novo) — `Alerter`, embeds, contador de falhas.
- `internal/discord/discord_test.go` (novo) — no-op nil, embed, limiar, gatilhos.
- `internal/config/config.go` — `DiscordWebhookURL` (env `DISCORD_WEBHOOK_URL`).
- `internal/transcode/worker.go` — `SetAlerter`; alerta falha terminal + reset no sucesso.
- `internal/transcode/queue.go` — `SetAlerter`; alerta fila cheia.
- `internal/jobs/requeue.go` — `SetAlerter`; alerta travado + falha terminal.
- `cmd/server/main.go` — cria o alerter e injeta nos três.
- Testes: `internal/transcode/queue_test.go` (alerta de fila cheia ponta-a-ponta).
- Docs: `spec/operacao.md` (env + seção de alertas), `spec/webhooks.md`,
  `.env.example`, `README.md`.

## Definition of Done

- [x] `DISCORD_WEBHOOK_URL` opcional; vazio = canal desabilitado (no-op).
- [x] Alertas nos quatro gatilhos, com embed (`video_id`, `status`, `error_message`, timestamp).
- [x] Tentativas registradas no log (sucesso/falha).
- [x] Best-effort: nunca bloqueia nem propaga erro.
- [x] Testes do pacote `discord` e do gatilho de fila cheia.
- [x] Documentação atualizada.

## Resolução

Implementado conforme acima. O Alerter nil-safe permite injeção opcional sem
ramos condicionais nos pontos de chamada. Limiar de falhas consecutivas = 5
(constante `consecutiveFailureThreshold`). Cores: vermelho para falhas,
laranja para capacidade/saúde (fila cheia, travado). Suíte do pacote `discord`
e o teste de fila cheia (`TestQueue_FullQueueAlertsDiscord`) passam; build e
`go vet` limpos.
