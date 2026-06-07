# T50: Unificar as duas variáveis de TTL de token de upload em uma só

**Status:** pending
**Dependências:** T49 (só faz sentido unificar depois que existe um único fluxo de auth de upload)
**Estimativa:** pequena
**Origem:** Issue #10 — "Pq UPLOAD_TOKEN_SCOPED_TTL_SECONDS e UPLOAD_TOKEN_TTL_SECONDS são diferentes?"

## Contexto

Última tarefa da cadeia que resolve a issue #10 (T48 → T49 → T50). A
issue questiona diretamente por que existem duas variáveis de TTL para
"a mesma funcionalidade" (upload):

> "Não tem justificativa um upload scoped e outro não scoped terem TTL
> diferentes."

Hoje (`internal/config/config.go`):

- `UploadTokenTTL` ← `UPLOAD_TOKEN_TTL_SECONDS` (padrão `21600` = 6h) —
  TTL do fluxo legado/global (token de vida longa, pensado para um
  backend confiável que poderia reutilizar o token por mais tempo)
- `UploadTokenScopedTTL` ← `UPLOAD_TOKEN_SCOPED_TTL_SECONDS` (padrão
  `1200` = 20min) — TTL do fluxo escopado a projeto (token de vida
  curta, pensado para "um único arquivo")

Com a T49, só resta o fluxo escopado a projeto — então só faz sentido
existir **um** TTL, e ele deve refletir a semântica que sobreviveu: vida
curta, pensada para autorizar o upload de um único arquivo
(~15-20 minutos).

**Sobre o nome da variável:** o usuário pediu explicitamente que o nome
final não contenha o termo "scoped" — faz sentido, já que depois desta
cadeia não existe mais distinção "scoped vs. não-scoped": é só "o TTL do
token de upload". Reaproveitar o nome já existente
`UPLOAD_TOKEN_TTL_SECONDS` / `UploadTokenTTL` é a opção mais limpa (não
introduz um terceiro nome, e o nome já é semanticamente correto agora
que só há um fluxo).

## O que muda

1. **Remover `UploadTokenScopedTTL`** (struct field em
   `internal/config/config.go:25`) **e** `UPLOAD_TOKEN_SCOPED_TTL_SECONDS`
   (env var, leitura em `config.go:77` e linha de atribuição ~118).
2. **Reaproveitar `UploadTokenTTL` / `UPLOAD_TOKEN_TTL_SECONDS`** como o
   único TTL de token de upload — mas **trocar o valor padrão** de
   `21600` (6h, semântica do fluxo legado que deixou de existir) para
   `1200` (20min, semântica que sobreviveu — "um único arquivo").
   Avalie se faz sentido manter o nome da constante/variável de ambiente
   ou se outro nome comunica melhor a vida curta (ex.:
   `UPLOAD_TOKEN_TTL_SECONDS` continua sendo o nome mais natural — "TTL
   do token de upload", sem qualificador).
3. **Atualizar `internal/upload/init.go`** — a lógica
   `if project != nil { ttl = h.cfg.UploadTokenScopedTTL } else { ttl =
   h.cfg.UploadTokenTTL }` (linhas ~129-136) colapsa para uma única
   atribuição `ttl = h.cfg.UploadTokenTTL`.
4. **Atualizar `.env.example`** — remover a linha
   `UPLOAD_TOKEN_SCOPED_TTL_SECONDS=1200` e ajustar
   `UPLOAD_TOKEN_TTL_SECONDS=21600` → `UPLOAD_TOKEN_TTL_SECONDS=1200`
   (com comentário explicando a semântica de "vida curta, um único
   upload").
5. **Atualizar `spec/ESPECIFICACAOv4.md`** — varrer referências a
   `UPLOAD_TOKEN_TTL_H` (formato antigo em horas, pré-T31),
   `UPLOAD_TOKEN_TTL_SECONDS` e `UPLOAD_TOKEN_SCOPED_TTL_SECONDS`
   (linhas ~404, 423, 482, 637, 647, 700) e atualizar para refletir a
   variável única, removendo qualquer menção ao fluxo dual.
6. **Varrer comentários no código** que documentam a distinção entre os
   dois TTLs (ex.: `internal/config/config.go:24-25`,
   `internal/upload/init.go` doc do handler — já deve ter sido ajustada
   na T49, revisar se sobrou algo) e atualizar para descrever um único
   TTL com semântica de vida curta.

## QA Instructions

1. Atualize `internal/config/config_test.go`: remova testes específicos
   de `UploadTokenScopedTTL`/`UPLOAD_TOKEN_SCOPED_TTL_SECONDS`; ajuste
   os de `UploadTokenTTL`/`UPLOAD_TOKEN_TTL_SECONDS` para refletir o
   novo valor padrão (`1200`) e a semântica única.
2. Atualize/escreva teste de integração de `POST /upload/init`
   confirmando que o `expires_at` do token persistido reflete
   `UploadTokenTTL` (não mais dois valores possíveis dependendo do
   fluxo).
3. Confirme que não sobrou nenhuma referência (código, comentário, env
   de exemplo, spec) a `UPLOAD_TOKEN_SCOPED_TTL_SECONDS` ou
   `UploadTokenScopedTTL` — `grep -rn "Scoped" internal/ spec/
   .env.example` deve voltar vazio (ou só com ocorrências legítimas não
   relacionadas a TTL, se houver).

## Dev Instructions

1. Implemente a unificação conforme os itens 1-3 acima.
2. Atualize `.env.example` e `spec/ESPECIFICACAOv4.md` (itens 4-5).
3. Revise comentários residuais (item 6).
4. Rode `go test ./...` e confirme que tudo passa, sem regressões.
5. Ao concluir esta tarefa, **a cadeia T48→T49→T50 fecha a issue #10**
   — a Resolução desta tarefa deve resumir as três micro-tarefas (o que
   cada uma fez), pois é ela quem referencia o fechamento da issue no
   manifest e no comentário do GitHub.

## Arquivos a revisar/editar

- `internal/config/config.go`
- `internal/config/config_test.go`
- `internal/upload/init.go`
- `.env.example`
- `spec/ESPECIFICACAOv4.md`

## Definition of Done

- [ ] `UploadTokenScopedTTL`/`UPLOAD_TOKEN_SCOPED_TTL_SECONDS` removidos
      por completo (struct, leitura de env, testes, `.env.example`, spec)
- [ ] `UploadTokenTTL`/`UPLOAD_TOKEN_TTL_SECONDS` é o único TTL de token
      de upload, com novo valor padrão de vida curta (`1200`)
- [ ] `internal/upload/init.go` usa um único caminho de atribuição de TTL
- [ ] `.env.example` e `spec/ESPECIFICACAOv4.md` atualizados, sem
      vestígios do esquema dual
- [ ] `go test ./...` passa sem regressões
- [ ] Issue #10 fechada (comentário resumindo T48+T49+T50 + `state: closed`)
