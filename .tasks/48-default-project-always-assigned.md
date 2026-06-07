# T48: Todo upload sempre pertence a um projeto — projeto padrão automático

**Status:** pending
**Dependências:** nenhuma (trabalha sobre código já existente de T32-T35)
**Estimativa:** média
**Origem:** Issue #10 — "Pq UPLOAD_TOKEN_SCOPED_TTL_SECONDS e UPLOAD_TOKEN_TTL_SECONDS são diferentes?"

## Contexto

A issue #10 aponta que a coexistência de dois fluxos de upload — um
vinculado a projeto (`project_id` preenchido) e outro "legado"
(`project_id = NULL`, gravado na raiz de `MEDIA_DIR`) — não faz sentido:
"sempre deve existir um projeto, nem que seja um default... Pq se uns
uploads forem feitos em projeto e outros forem feitos na raiz, vai poluir
a raiz do diretório de mídia."

Esta é a primeira de uma cadeia de 3 tarefas que resolvem a issue #10
(T48 → T49 → T50): aqui garantimos que **todo** upload sempre tenha um
projeto associado — criando um projeto "padrão" automaticamente quando o
cliente não apresenta `X-Project-Key`. As tarefas seguintes removem o
fluxo de autenticação legado (T49) e unificam as variáveis de TTL (T50),
que só fazem sentido depois que existe um único caminho.

**Importante — sem dados reais a preservar:** o usuário confirmou que o
projeto ainda não está em uso por ninguém ("esse projeto não está sendo
usado por ninguém... eu quero ele limpo e sem vestígios de coisa velha
antes de lançar"). Não é necessário projetar migração de dados de
produção — pode-se simplificar o esquema e remover mecanismos de
compatibilidade com confiança.

## O que muda

1. **Projeto padrão automático** — em vez do mecanismo atual de
   `MigrateLegacyVideos` (que roda na inicialização e *migra* vídeos
   órfãos para um projeto "Legacy" criado sob demanda), criar um
   mecanismo mais simples de "garantir que o projeto padrão existe"
   (idempotente, análogo a `getOrCreateLegacyProject` em
   `internal/jobs/project_migration.go:76-90`, mas sem a lógica de
   migração de diretórios — não há nada para migrar).
   - Nome sugerido para o projeto/slug: **"default"** (evitar "Legacy",
     que carrega a semântica errada — não é sobre vídeos antigos, é o
     projeto que recebe uploads quando o cliente não especifica outro).
     Avalie no código se esse nome colide com alguma convenção existente
     antes de fixá-lo.
   - A chave mestra do projeto padrão é gerada normalmente
     (`models.CreateProject`) e deve ser exibida/logada na primeira
     criação, do mesmo jeito que qualquer outro projeto novo — é o
     "default key" que a issue menciona ("crie uma key default pra
     incluí-lo lá").

2. **`POST /upload/init` sempre resolve um projeto** — atualmente
   (`internal/upload/init.go:60-81`), quando o cliente não envia
   `X-Project-Key`, o handler cai no fluxo HMAC legado e `project`
   permanece `nil`. Depois desta tarefa, a ausência de `X-Project-Key`
   deve resolver para o projeto padrão (criado/garantido no passo 1) —
   eliminando definitivamente o caminho que produz `project_id = NULL`.
   (A remoção do *outro lado* do `if`/`else` — o branch HMAC global em
   si — é objeto da T49; aqui o foco é garantir que, com ou sem chave de
   projeto explícita, sempre exista um projeto resolvido.)

3. **Remover `internal/jobs/project_migration.go`** (e seu teste) — uma
   vez que todo upload novo sempre associa um projeto, a rotina de
   *migração* de vídeos órfãos vira código morto (não há vídeos órfãos
   para migrar em uma instalação nova, e o usuário confirmou que não há
   dados de produção a preservar). Remover também a chamada em
   `cmd/server/main.go:55-60`.

4. **Simplificar `ResolveVideoRootDir`** (`internal/models/project.go:218-235`)
   — o branch `projectID == nil → ""` (layout legado, raiz de
   `MEDIA_DIR`) deixa de ser alcançável; avalie se deve virar um erro
   (`projectID nil é estado inválido — todo vídeo deve ter projeto`) ou
   se a assinatura da função pode ser simplificada para receber
   `int64` em vez de `*int64`.

5. **Coluna `project_id` — nullable vs. `NOT NULL`**: hoje
   `videos.project_id` e `upload_tokens.project_id` são `INTEGER
   REFERENCES projects(id)` (nullable, adicionadas via `ensureColumn` em
   `internal/db/db.go:65-70` porque foram introduzidas depois do
   `CREATE TABLE` original). SQLite não suporta `ALTER TABLE ... ALTER
   COLUMN ... SET NOT NULL` diretamente — exigiria reconstrução de
   tabela (`CREATE new → INSERT SELECT → DROP → RENAME`). Avalie e
   documente a decisão:
   - Opção A (mais segura/simples): manter a coluna nullable no schema,
     mas garantir `NOT NULL` *na prática* via código (toda inserção
     sempre passa um `project_id` não-nulo) — "limpo o suficiente" sem
     o custo/risco de reconstruir tabela.
   - Opção B (mais rigorosa): reescrever `schema.go` para já criar a
     coluna como `NOT NULL` em instalações novas, e migrar instalações
     existentes via reconstrução de tabela em `db.go`.
   Dado que não há instalações em produção, a Opção B é viável — mas a
   decisão final (e a documentação do porquê) cabe ao Dev, registrada na
   Resolução.

## QA Instructions

1. Escreva testes table-driven para o novo mecanismo de "garantir
   projeto padrão": idempotência (rodar duas vezes não duplica), criação
   na ausência, reuso quando já existe.
2. Escreva/adapte testes de `POST /upload/init` cobrindo:
   - Requisição sem `X-Project-Key` → vídeo é associado ao projeto
     padrão (não mais a `project_id = NULL`)
   - Requisição com `X-Project-Key` de projeto explícito → continua
     funcionando como hoje
   - Verificar que `ResolveVideoRootDir`/diretório de armazenamento
     resultante está correto em ambos os casos
3. Se o Dev optar pela Opção B (NOT NULL), escreva também testes de
   schema/migração garantindo que a reconstrução de tabela preserva
   dados e índices corretamente.
4. Confirme que `internal/jobs/project_migration_test.go` foi removido
   junto com o código de produção (não deixar teste órfão apontando para
   função inexistente).

## Dev Instructions

1. Implemente o mecanismo de "garantir projeto padrão" (criação
   idempotente + exibição da chave mestra na primeira criação).
2. Atualize `POST /upload/init` para resolver sempre um projeto (padrão
   ou explícito via `X-Project-Key`) — sem alterar ainda o branch de
   autenticação HMAC legado em si (isso é T49; aqui o objetivo é só
   garantir que `project` nunca fique `nil`).
3. Remova `internal/jobs/project_migration.go` + teste + a chamada em
   `cmd/server/main.go`.
4. Simplifique `ResolveVideoRootDir` conforme o item 4 acima.
5. Decida e documente (na Resolução) a estratégia para `project_id`
   (Opção A ou B do item 5), implementando a escolhida.
6. Rode `go test ./...` e confirme que tudo passa, incluindo os novos
   testes do QA.

## Arquivos a revisar/editar

- `internal/models/project.go` (`ResolveVideoRootDir`, possível adição
  de helper "garantir projeto padrão")
- `internal/upload/init.go` (resolução de projeto no `ServeHTTP`)
- `internal/jobs/project_migration.go` (remover)
- `internal/jobs/project_migration_test.go` (remover)
- `cmd/server/main.go` (remover chamada a `MigrateLegacyVideos`)
- `internal/db/schema.go` / `internal/db/db.go` (se Opção B for escolhida)

## Definition of Done

- [ ] Mecanismo de projeto padrão implementado, idempotente e testado
- [ ] `POST /upload/init` sempre resolve um projeto (nunca mais
      `project_id = NULL` em uploads novos)
- [ ] `internal/jobs/project_migration.go` removido (código morto
      eliminado, sem testes órfãos)
- [ ] `ResolveVideoRootDir` simplificado/documentado
- [ ] Decisão sobre `project_id NOT NULL` tomada, documentada e
      implementada
- [ ] `go test ./...` passa sem regressões
