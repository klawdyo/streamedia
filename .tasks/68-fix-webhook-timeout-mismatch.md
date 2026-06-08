# T68: Corrigir timeout incoerente no webhook client

**Status:** done
**Dependências:** T17
**Estimativa:** pequena
**Origem:** análise de código — configuracao inefetiva
**Severidade:** media

## Contexto

Em `internal/webhook/webhook.go`, o cliente HTTP tem dois timeouts
conflitantes:

1. **Client timeout** (linha 52): `Timeout: 30 * time.Second`
2. **Context timeout** (linha 105): `context.WithTimeout(..., 10*time.Second)`

O context timeout (10s) e sempre mais restritivo que o client timeout (30s).
O `http.Client.Timeout` nunca sera atingido porque o context cancela antes.
Isso nao e um bug funcional (a requisicao vai falhar em 10s, que e o
comportamento desejado), mas e codigo enganoso — sugere que existe um
timeout de 30s quando na pratica nunca e usado.

## Impacto

- **Codigo enganoso**: quem le o `Timeout: 30s` acha que requisicoes podem
  durar ate 30s, mas na pratica sao canceladas em 10s.
- **Manutencao**: se alguem quiser aumentar o timeout para 15s, vai mudar
  o context e nao perceber que o client timeout ja cobre.

## Dev Instructions

### 1. Alinhar os timeouts

Escolher UMA estrategia:

**(a) Usar apenas o context timeout (recomendado)**:
- Remover `Timeout` do `http.Client` (ou setar para 0 = sem timeout).
- O context timeout de 10s ja controla o tempo maximo por tentativa.
- Vantagem: timeout explicito por request, visivel no `sendAttempt`.

**(b) Usar apenas o client timeout**:
- Remover o `context.WithTimeout` de `sendAttempt`.
- Setar `Timeout: 10 * time.Second` no client.
- Vantagem: timeout centralizado em um lugar.

### 2. Considerar extrair para config

Ambos os timeouts sao hardcoded. Se o projeto tiver necessidade futura de
configurar o timeout de webhook, considerar mover para `config.Config`.
Mas NAO adicionar isso agora se nao for pedido — YAGNI.

### 3. Verificacao

- `go test ./internal/webhook/...` — sem regressoes
- `go vet ./...` — sem warnings

## Arquivos a editar

- `internal/webhook/webhook.go`

## Resolução

Arquivos alterados:
- `internal/webhook/webhook.go`: removido `Timeout: 30 * time.Second` do
  `http.Client`. Agora só o `context.WithTimeout` de 10s em `sendAttempt`
  controla o timeout por tentativa. Comentário explica a decisão.

## Definition of Done

- [x] Um unico timeout efetivo (sem redundancia)
- [x] Timeout documentado em comentario se nao for obvio
- [x] `go test ./...` passa
- [x] `go vet ./...` limpo
