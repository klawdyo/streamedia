# T47: Centralizar regex de segmento HLS e construção de URL pública (scheme/host)

**Status:** pending
**Dependências:** nenhuma (refatoração isolada do código existente)
**Estimativa:** pequena
**Origem:** solicitação direta do usuário — "pente fino" em busca de código
duplicado que deveria ser centralizado (mesmo princípio identificado e
corrigido na T44 para a validação de UUID)

## Contexto

Concluindo a varredura por duplicação que motivou a T44 (várias regex de
UUID fazendo a mesma validação em paralelo), encontrei mais **dois**
casos concretos do mesmo padrão — lógica idêntica reimplementada em
paralelo, em vez de centralizada em um único lugar:

### 1. Regex de nome de segmento HLS (`^[0-9]+\.ts$`)

Definida **duas vezes**, byte-a-byte idêntica:

- `internal/serve/serve.go:41` — `var segmentRe = regexp.MustCompile(`^[0-9]+\.ts$`)`
- `internal/transcode/worker.go:227` — `var renditionSegmentRe = regexp.MustCompile(`^[0-9]+\.ts$`)`

O comentário em `worker.go:224-227` já **reconhece** a duplicação
("mesmo padrão usado no serving, ver internal/serve.segmentRe") mas não a
elimina — sinal claro de que deveria virar uma única definição
compartilhada.

### 2. Resolução de scheme/host para montar URL pública a partir de headers de proxy

Bloco de 13 linhas **idêntico** (mesma lógica, mesmas variáveis, mesma
ordem de checagem de `X-Forwarded-Proto`/`X-Forwarded-Host`/`r.TLS`)
duplicado em:

- `internal/upload/init.go:144-156`
- `internal/admin/projects.go:261-273`

Ambos terminam montando a mesma forma de URL:
`fmt.Sprintf("%s://%s/files/%s", scheme, host, videoID)`. Se amanhã for
preciso ajustar essa lógica (ex.: suportar `Forwarded` no formato RFC 7239,
ou tratar `X-Forwarded-Port`), alguém vai lembrar de mudar só um dos dois
lugares — exatamente o tipo de inconsistência que o usuário quer evitar.

## QA Instructions

### 1. Regex de segmento

Crie/estenda testes que comprovem que o comportamento se mantém idêntico
após a centralização (não é uma mudança de comportamento, é remoção de
duplicação):

```
TestSegmentNameRegex_TableDriven (no pacote onde a regex centralizada
viver — provavelmente internal/models ou um novo internal/hls)
  - tabela: "0.ts" → match, "123.ts" → match, "01.ts" → match,
    "abc.ts" → não casa, "1.m4s" → não casa, "1.TS" → não casa,
    "../1.ts" → não casa, "" → não casa
```

Verifique que `internal/serve` e `internal/transcode` continuam usando a
MESMA instância/definição (não duas cópias) — pode ser por meio de um
teste que importa a variável/função de ambos os contextos, ou apenas por
inspeção de código no QA de verificação (Fase 2).

### 2. Construção de URL pública

```
TestBuildPublicUploadURL_TableDriven (no pacote/local escolhido para a
função centralizada)
  - sem headers de proxy, sem TLS → "http://<host>/files/<id>"
  - sem headers de proxy, com TLS → "https://<host>/files/<id>"
  - com X-Forwarded-Proto: "https" → usa "https" mesmo sem TLS
  - com X-Forwarded-Host: "cdn.example.com" → usa o host do header
  - com ambos os headers → usa os dois valores informados
  - confirme que tanto o teste de /upload/init quanto o de
    /admin/projects/{slug}/upload-tokens continuam passando após migrarem
    para a função centralizada (não pode haver regressão de comportamento)
```

Confirme que os testes passam contra o comportamento atual antes de migrar
o código (eles documentam o contrato existente — não é um ciclo
red→green de feature nova, é blindagem antes de centralizar).

## Dev Instructions

### 1. Centralize a regex de segmento HLS

Escolha **um** lugar para a definição única — sugestão: `internal/models`
(já é o "dono" do conceito de vídeo/rendition) ou, se fizer mais sentido
ao ler o código, um pacote pequeno e específico (ex. `internal/hls`).
Documente a escolha na seção "Resolução" desta tarefa.

```go
// SegmentNameRe casa nomes de segmento HLS gerados pelo FFmpeg e servidos
// estaticamente: um ou mais dígitos seguidos de ".ts". Definição única —
// reaproveitada tanto pela geração (worker) quanto pelo serving estático,
// evitando que as duas pontas do mesmo contrato divirjam com o tempo.
var SegmentNameRe = regexp.MustCompile(`^[0-9]+\.ts$`)
```

- Remova `segmentRe` de `internal/serve/serve.go:41` e `renditionSegmentRe`
  de `internal/transcode/worker.go:227`; aponte os dois usos para a
  definição única.

### 2. Centralize a construção de URL pública

Crie uma função única (sugestão: pacote `internal/httputil` — ainda não
existe; ou, se a T45 já tiver criado um pacote de infraestrutura HTTP
compartilhada como `internal/apiresponse`, avalie se cabe lá; documente a
decisão):

```go
// PublicUploadURL monta a URL pública de upload TUS para video_id,
// resolvendo scheme e host a partir dos headers de proxy padrão
// (X-Forwarded-Proto, X-Forwarded-Host) com fallback para r.TLS/r.Host.
// Centraliza a lógica antes duplicada entre /upload/init e
// /admin/projects/{slug}/upload-tokens — ambas constroem a mesma forma
// de URL (<scheme>://<host>/files/<video_id>) a partir da mesma
// requisição de entrada.
func PublicUploadURL(r *http.Request, videoID string) string
```

- Substitua os blocos duplicados em `internal/upload/init.go:144-156` e
  `internal/admin/projects.go:261-273` por uma chamada a essa função.
- Se notar que a lógica de resolver scheme/host sozinha (sem montar a URL
  final) é útil em outro lugar, separe em uma função menor
  (`ResolvePublicSchemeAndHost(r *http.Request) (scheme, host string)`) e
  componha — mas não generalize além do que os dois usos reais pedem.

### 3. Verificação

- `go test ./... -v` — tudo passa, nenhuma regressão
- `go vet ./...` sem warnings
- `grep -rn "X-Forwarded-Proto" internal/ --include="*.go" | grep -v _test`
  deve apontar para um único ponto de definição (mais os pontos de
  chamada da função centralizada, se você optar por manter o nome do
  header visível nos call sites — documente a escolha)

## Arquivos a criar/editar

- Novo arquivo (local a definir pelo Dev — ex. `internal/models/hls.go` ou
  `internal/hls/hls.go`): `SegmentNameRe`
- Novo arquivo (local a definir — ex. `internal/httputil/url.go`):
  `PublicUploadURL` (e, se aplicável, `ResolvePublicSchemeAndHost`)
- `internal/serve/serve.go`: remove `segmentRe`, usa a definição centralizada
- `internal/transcode/worker.go`: remove `renditionSegmentRe`, usa a
  definição centralizada
- `internal/upload/init.go`: remove o bloco de resolução de scheme/host,
  usa `PublicUploadURL`
- `internal/admin/projects.go`: remove o bloco duplicado, usa
  `PublicUploadURL`
- Testes correspondentes (tabela para a regex e para a função de URL)

## Resolução

<!-- Preencher ao concluir: onde cada definição centralizada acabou
morando, e por quê. -->

## Definition of Done

- [x] Regex de nome de segmento HLS definida em um único lugar e
      reaproveitada por `serve` e `transcode` — nenhuma cópia restante
- [x] Construção de URL pública (scheme/host a partir de headers de proxy)
      centralizada em uma função única, reaproveitada por `/upload/init`
      e `/admin/projects/{slug}/upload-tokens` — nenhum bloco duplicado restante
- [x] Testes de tabela cobrindo os contratos de ambas as centralizações
- [x] `go test ./... -v` passa sem regressões
- [x] `go vet ./...` sem warnings
- [x] Seção "Resolução" preenchida com as decisões de localização tomadas

## Resolução

### 1. Regex de segmento HLS — `models.SegmentNameRe`

**Localização**: `internal/models/hls.go` (pacote `models`)
**Justificativa**: O pacote `models` já é o "dono" dos conceitos de vídeo e formato — UUID validation, status, etc. Centralizar a regex de segmento aqui mantém a coesão do domínio (formato de mídia é conceito do modelo de dados, não de infraestrutura HTTP). Evita criar um pacote `internal/hls` com uma única variável.

**Alterações**:
- `internal/models/hls.go` — novo arquivo com `var SegmentNameRe = regexp.MustCompile(...)`
- `internal/models/hls_test.go` — 13 casos table-driven documentando o contrato
- `internal/serve/serve.go` — removido `segmentRe`, usos trocados para `models.SegmentNameRe`
- `internal/transcode/worker.go` — removido `renditionSegmentRe`, uso trocado para `models.SegmentNameRe`; import `regexp` removido

### 2. URL pública de upload — `httputil.PublicUploadURL`

**Localização**: `internal/httputil/url.go` (novo pacote `httputil`)
**Justificativa**: Diferente da regex (que é conceito de domínio), a construção de URL a partir de headers de proxy é uma utilidade de infraestrutura HTTP. Não pertence a `models` nem a `apiresponse` (que é só sobre o formato do envelope de resposta). Um pacote `httputil` é o lugar natural para funções auxiliares de HTTP compartilhadas entre handlers — e já fica disponível para futuras rotas que precisarem resolver scheme/host.

A função foi decomposta em `PublicUploadURL` (pública, composta) + `resolveScheme`/`resolveHost` (privadas, testáveis isoladamente via `TestResolveScheme`).

**Alterações**:
- `internal/httputil/url.go` — novo arquivo com `PublicUploadURL`, `resolveScheme`, `resolveHost`
- `internal/httputil/url_test.go` — 8 cenários de URL + 4 cenários de scheme
- `internal/upload/init.go` — bloco de 13 linhas substituído por `httputil.PublicUploadURL(r, videoID)`; imports `fmt` removido, `httputil` adicionado
- `internal/admin/projects.go` — bloco de 13 linhas substituído por `httputil.PublicUploadURL(r, videoID)`; imports `fmt` removido, `httputil` adicionado
