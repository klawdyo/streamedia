# T33: Chaves de API escopadas por projeto (upload / listagem / admin)

**Status:** done
**Dependências:** T32
**Estimativa:** grande
**Issue relacionada:** #6

## Contexto

A issue #6 detalha três tipos de chave, todas vinculadas a um projeto:

- **Upload**: token de curtíssima duração (a issue pede **15-20 minutos**,
  bem menos que o `UPLOAD_TOKEN_TTL_SECONDS` global atual de 6h), válido
  para um único arquivo/vídeo — gerado a partir da chave mestra do projeto.
- **Leitura/listagem**: token escopado a um vídeo específico (já existe a
  base disso no fluxo de play token — `PlayTokenMaxTTL` — mas hoje não é
  vinculado a projeto).
- **Administração**: equivalente ao `ADMIN_TOKEN` atual, mas por projeto —
  permite operar `/admin/*` apenas sobre os vídeos daquele projeto.

### Por que isso é grande

Esta tarefa toca a cadeia de autenticação inteira (`internal/auth`,
`internal/upload`, `internal/serve`, `internal/admin`) para acrescentar
"escopo de projeto" a cada verificação — não é uma mudança isolada num
pacote. Considere dividir o trabalho de implementação em sub-PRs por tipo de
chave (upload → leitura → admin) mesmo mantendo esta como uma tarefa só no
manifesto, para reduzir o tamanho de cada revisão.

## Dev Instructions

- Estenda `internal/auth` para validar HMAC com a chave mestra do projeto
  (em vez do segredo global único), resolvendo o projeto a partir de um
  identificador na requisição (ex. header `X-Project-Key` ou prefixo no
  payload assinado — escolha o que exigir menos mudança de contrato).
- `POST /upload/init` passa a exigir a chave do projeto e gerar um token de
  upload com TTL próprio e curto (constante nova, ex.
  `UploadTokenScopedTTLSeconds = 1200`, configurável via env se fizer
  sentido) e vinculado a `project_id` + `video_id` (um único arquivo).
- Tokens de leitura/listagem passam a carregar `project_id` e continuam
  escopados a um único vídeo (reforce: "um token de um vídeo não serve para
  outro", conforme a issue).
- Admin: ou (a) mantenha `ADMIN_TOKEN` global como "super admin" e
  acrescente uma chave admin por projeto que filtra `/admin/*` pelos vídeos
  daquele projeto, ou (b) substitua totalmente por chaves por projeto.
  Documente a decisão escolhida — a opção (a) é mais simples de migrar e
  preserva compatibilidade operacional.

## QA Instructions

Crie testes (local sugerido: `internal/auth/project_scope_test.go` +
extensões em `internal/upload`, `internal/serve`, `internal/admin`):

```
TestUploadToken_ExpiresInScopedTTL
  - gera token via chave de projeto, confere TTL curto (15-20min) e
    vinculação a project_id + video_id

TestUploadToken_RejectsForOtherProject
  - token gerado para o projeto A não autentica upload no projeto B

TestPlayToken_ScopedToSingleVideo
  - token de leitura do vídeo X não autentica acesso ao vídeo Y,
    mesmo no mesmo projeto

TestAdminAuth_ScopedToProject
  - chave admin do projeto A não enxerga/opera vídeos do projeto B
    (ajuste conforme a opção (a)/(b) escolhida)
```

## Arquivos a criar/modificar

- `internal/auth/*`
- `internal/upload/*`
- `internal/serve/*`
- `internal/admin/*`
- `internal/models/token.go` (ou equivalente — adicionar `project_id`)
- `internal/db/schema.go` (colunas/índices novos para vincular tokens a projeto)

## Definition of Done

- [x] Token de upload curto (15-20min), escopado a projeto + vídeo único
- [x] Token de leitura escopado a projeto + vídeo único
- [x] Autenticação/autorização admin considera o projeto
- [x] Decisão sobre o modelo de admin (global vs. por projeto) documentada
- [x] Todos os testes passam

## Resolução

Implementadas as três chaves escopadas a projeto da issue #6, sem quebrar
nenhum dos fluxos legados (mudança aditiva — instalações sem projetos
continuam funcionando exatamente como antes):

### Schema (`internal/db/schema.go` + `internal/db/db.go`)

- `videos` e `upload_tokens` ganham coluna `project_id INTEGER` (nullable,
  `REFERENCES projects(id)`) + índices `idx_videos_project` e
  `idx_upload_tokens_project`. Como este projeto não tem sistema de
  migrações versionadas e `CREATE TABLE IF NOT EXISTS` não altera tabelas
  já existentes, a coluna é adicionada de forma idempotente via
  `ensureColumn` (consulta `PRAGMA table_info`, executa `ALTER TABLE ...
  ADD COLUMN` só se a coluna ainda não existir) — funciona tanto para
  bancos novos quanto para instalações existentes.
- `project_id = NULL` identifica vídeos/tokens do fluxo legado (sem projeto).

### Upload — `X-Project-Key` (`internal/upload/init.go`)

`POST /upload/init` agora aceita dois fluxos de autenticação:

- **Escopado a projeto**: header `X-Project-Key: <chave mestra em texto
  puro>`. O servidor calcula `models.HashMasterKey` e resolve o projeto via
  `models.GetProjectByMasterKeyHash` — análogo ao `Authorization: Bearer`
  do `ADMIN_TOKEN`, e evita ter que reter/recuperar a chave em claro só
  para verificar HMAC (decisão deliberada: a chave mestra já existe como
  segredo do tipo "bearer", diferente do `UPLOAD_TOKEN_SECRET` que é
  compartilhado para assinar). Tem prioridade sobre `X-Upload-Auth`.
  - O token de upload gerado reaproveita `auth.GenerateUploadToken`, mas
    assinado com a **própria chave mestra do projeto** (presente na
    requisição) em vez do segredo global — "Estenda internal/auth para
    validar HMAC com a chave mestra do projeto" do spec, sem duplicar
    lógica criptográfica nem introduzir novo formato de token.
  - TTL curto e configurável: `UploadTokenScopedTTL`
    (`UPLOAD_TOKEN_SCOPED_TTL_SECONDS`, default `1200` = 20min — dentro da
    janela "15-20 minutos" pedida na issue, bem menor que o TTL global de 6h).
  - Vídeo e token persistidos com `project_id` preenchido
    (`InsertVideoForProject`/`InsertUploadTokenForProject`).
- **Legado/global**: header `X-Upload-Auth` com HMAC sobre o corpo,
  assinado com `UPLOAD_TOKEN_SECRET` — comportamento 100% preservado;
  `project_id` fica `NULL`.

### Leitura/listagem — play tokens (`internal/serve`)

Os tokens de reprodução (`auth.GeneratePlayToken`/`ValidatePlayToken`) já
eram escopados a um único `video_id` (payload assinado
`"{video_id}:{expires}"`) — "um token de um vídeo não serve para outro" já
valia antes desta tarefa (coberto agora explicitamente por
`TestPlayToken_ScopedToSingleVideo`, que prova que um token emitido para um
vídeo é rejeitado (401) ao tentar abrir outro, mesmo "ready" e válido).

O vínculo de **projeto** é transitivo, não redundante: como o vídeo agora
carrega `project_id`, e o token já é escopado a um único `video_id`, o
token fica automaticamente escopado ao projeto daquele vídeo — sem precisar
embutir `project_id` no payload assinado nem trocar o esquema HMAC
stateless dos play tokens (que exigiria segredos por projeto, uma mudança
de modelo de dados bem maior e fora do escopo desta tarefa).

### Admin — opção (a) do spec (`internal/admin/admin.go`)

Decisão registrada: **opção (a)** — `ADMIN_TOKEN` global continua
funcionando como super-admin (sem escopo, vê tudo — comportamento
preservado), e a chave mestra de um projeto também autentica em
`/admin/*`, mas restrita aos vídeos daquele projeto. Escolhida por ser a
migração mais simples e não quebrar instalações que dependem só do
`ADMIN_TOKEN` (conforme sugerido no próprio spec da tarefa).

- `AdminAuth(adminToken, db)`: agora recebe a conexão para resolver
  projetos. Compara o bearer token primeiro com `ADMIN_TOKEN` (tempo
  constante, super-admin sem escopo) e, se não bater, tenta resolver um
  projeto pelo hash da chave (`GetProjectByMasterKeyHash`) — se encontrar,
  propaga o `project_id` no contexto da requisição
  (`ProjectScopeFromContext`); se nenhum dos dois casar, 401.
- `HandleVideos` passa a filtrar por `project_id` quando o contexto traz um
  escopo — a cláusula `WHERE` é montada dinamicamente (status + escopo de
  projeto), preservando o filtro por status e a paginação existentes.
  `HandleQueue`/`HandleStats` continuam globais (fila de transcodificação e
  agregações de eventos não têm granularidade de projeto no modelo atual —
  fora do escopo desta tarefa; ambas seguem acessíveis tanto a super-admins
  quanto a admins de projeto).

### Testes (QA da T33, todos novos)

- `internal/upload/project_scope_test.go`:
  `TestUploadToken_ExpiresInScopedTTL` (TTL curto + vínculo a project_id e
  video_id) e `TestUploadToken_RejectsForOtherProject` (chave inválida
  rejeitada com 401; chave de outro projeto não "vaza" vínculo).
- `internal/serve/project_scope_test.go`: `TestPlayToken_ScopedToSingleVideo`
  (token de um vídeo não abre outro, mesmo "ready").
- `internal/admin/project_scope_test.go`: `TestAdminAuth_ScopedToProject`
  (chave do Projeto A só vê vídeos do Projeto A, chave do Projeto B só vê
  os de B, e o `ADMIN_TOKEN` global continua vendo todos).
- Ajustes de compatibilidade em `internal/admin/admin_test.go` e
  `internal/admin/stats_test.go`: chamadas a `AdminAuth` passam a receber
  `db` (nova assinatura).

Toda a suíte (`go test ./...`) passa, incluindo os fluxos legados — a
mudança é estritamente aditiva.
</content>
