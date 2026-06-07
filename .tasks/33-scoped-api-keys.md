# T33: Chaves de API escopadas por projeto (upload / listagem / admin)

**Status:** pending
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

- [ ] Token de upload curto (15-20min), escopado a projeto + vídeo único
- [ ] Token de leitura escopado a projeto + vídeo único
- [ ] Autenticação/autorização admin considera o projeto
- [ ] Decisão sobre o modelo de admin (global vs. por projeto) documentada
- [ ] Todos os testes passam
</content>
