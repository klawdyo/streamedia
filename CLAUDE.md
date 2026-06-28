# Streamedia — Media Server

Serviço Go de upload, transcodificação e entrega de vídeo em HLS.
Substitui Bunny Stream para uma rede social de vídeo estilo Instagram.

## Sistema de Agentes

Este repositório usa um sistema multi-agente estruturado:

| Agente | Modelo | Papel |
|--------|--------|-------|
| CTO | claude-sonnet-4-6 | Lê spec, cria tarefas, orquestra Dev e QA |
| Dev | claude-opus-4-8 | Sênior Go/media/streaming, implementa |
| QA | claude-haiku-4-5 | Test-first, escreve testes antes e verifica |
| Versioner | claude-haiku-4-5 | Calcula a próxima versão semântica a partir dos commits |
| Security Specialist | claude-fable-5 | Pentest, auditoria de segurança ofensiva/defensiva (sob demanda) |

## Fluxo de branches (Git Flow simplificado)

- **`main`** — branch principal/estável. Reflete sempre o que está em produção.
  Só recebe merge **via Pull Request**, e somente quando o usuário autorizar
  explicitamente. O PR deve descrever em detalhes tudo o que foi alterado.
- **`dev`** — branch de integração. **Todo o desenvolvimento parte daqui.**
- **Worktrees de trabalho — não troque de branch no checkout principal.**
  Para cada nova onda de tarefas (ou agente trabalhando em paralelo), crie
  um **worktree dedicado** (`git worktree add`) sempre a partir do estado
  mais recente de `dev`:
  ```
  git fetch origin dev
  git worktree add .worktrees/<assunto> -b <assunto> origin/dev
  ```
  Isso isola cada onda em um diretório e branch próprios — várias ondas (e
  vários agentes) podem rodar ao mesmo tempo sem disputar o `HEAD` do
  checkout compartilhado nem pisar no trabalho umas das outras. Ao concluir,
  faça merge da branch do worktree de volta para `dev`
  (`git merge --no-ff <assunto>` a partir de um checkout de `dev`, depois
  `git push origin dev`) e remova o worktree (`git worktree remove .worktrees/<assunto>`).
- **Nome do worktree/branch deve descrever o conteúdo do trabalho, não o
  processo.** Use o assunto/escopo das tarefas que serão feitas (ex.:
  `cobertura-testes-camada-de-dados`, `auditoria-seguranca-auth-tokens`,
  `envelope-resposta-padronizada`). Evite nomes genéricos que não dizem
  nada sobre o trabalho em si — como `revisar-issues`, `gerar-tarefas`,
  `continue-review`, `resume-latest-branch` ou variações baseadas em
  "última branch"/"retomar trabalho". Pense: alguém lendo só o nome do
  worktree/branch (sem o histórico da sessão) deve conseguir adivinhar do
  que ela trata.

```
dev (estado atual) → worktree dedicado + branch <assunto>
                   → (trabalho) → merge --no-ff de volta em dev → remove worktree
dev → ... → (quando autorizado) → Pull Request dev → main
```

**Nunca** dê push direto em `main`. **Nunca** abra PR para `main` sem pedido
explícito do usuário.

## Arquivos do sistema

- `spec/README.md` — Índice da especificação técnica, dividida em arquivos
  temáticos (arquitetura, autenticação, api, dados, pipeline, webhooks, operação).
  Descreve o fluxo atual (tag + ROOT_TOKEN). Fonte de verdade última: o código.
- `.agents/cto.md` — Instruções e workflow do agente CTO
- `.agents/dev.md` — Instruções do agente Dev
- `.agents/qa.md` — Instruções do agente QA
- `.agents/versioner.md` — Instruções do agente Versioner (versionamento semântico)
- `.agents/security-specialist.md` — Instruções do agente Security Specialist (sob demanda)
- `.tasks/00-manifest.md` — Lista de tarefas e status de cada uma
- `.tasks/NN-*.md` — Arquivos de tarefa individuais (autocontidos)

## Versionamento semântico

A versão do projeto segue `MAJOR.MINOR.PATCH` derivada dos commits semânticos
(Conventional Commits). O algoritmo completo — regras de incremento, exemplos
e procedimento de release — está em `.agents/versioner.md`.

### Regra obrigatória de prefixo nos commits

**Todo commit que implementa uma tarefa DEVE usar um prefixo Conventional Commits.**
O Versioner classifica commits exclusivamente pelo prefixo — commits sem ele
(como `"T33: chaves de API..."`) **não são reconhecidos** e a versão não
avança, mesmo que o conteúdo seja uma feature completa.

**Sem exceção**: o prefixo é obrigatório em todo commit. A numeração da task
(`T33`) pode vir em seguida, entre parênteses ou após o escopo — o que importa
é que o primeiro token do título seja um prefixo reconhecido pelo Versioner.

O agente CTO é responsável por revisar as mensagens de commit antes do merge
e rejeitar qualquer uma que não siga esta regra.

### Commit-checkpoint `release:`

Para não precisar reler todo o histórico a cada cálculo, o agente Versioner usa
o commit `release: vX.Y.Z - resumo das mudanças` mais recente como ponto de
partida e analisa apenas os commits posteriores a ele. Esse commit é criado
SOMENTE mediante confirmação explícita do usuário — ver `.agents/versioner.md`.


## Convenção de idioma

- Identificadores, nomes de arquivo, pacotes: **inglês**
- Comentários no código, mensagens de erro da API, README: **português**
- Comentários abundantes, mesmo em trechos óbvios

## Workflow rápido

```
CTO lê manifest → lê tarefa → QA escreve testes → Dev implementa → QA verifica → CTO atualiza manifest → próxima
```

Ver `.agents/cto.md` para o workflow detalhado.

## Issues do GitHub referenciadas em tarefas

Toda tarefa (`.tasks/NN-*.md`) que tem uma "Issue relacionada" tem uma
contrapartida obrigatória no GitHub — não basta documentar a solução só no
arquivo de tarefa local:

- **SEMPRE comente na issue** ao concluir a tarefa que a referencia,
  descrevendo a solução executada (o que foi feito, arquivos/rotas/commits
  envolvidos, como verificar) — mesmo quando a tarefa apenas avança a issue
  sem fechá-la (ex. uma de várias micro-tarefas em cadeia). Quando a tarefa
  **fecha** a issue, o comentário deve resumir a cadeia inteira (todas as
  micro-tarefas que contribuíram), não só a última.
- **SEMPRE referencie a issue no(s) commit(s)** da tarefa (ex. `Refs #2`
  para tarefas que avançam a issue sem fechá-la, `Closes #2`/`Fixes #2`
  para a que fecha), para que o vínculo fique rastreável no histórico do
  GitHub.
- **Feche a issue explicitamente** quando a tarefa que a conclui for
  mergeada — não dependa apenas da palavra-chave `Closes #N` no commit: ela
  só fecha automaticamente quando o merge acontece no branch padrão do
  repositório (geralmente `main`) via PR. Como o fluxo deste projeto faz
  merge em `dev` (não no padrão), feche a issue manualmente
  (`issue_write` com `state: closed`) depois de comentar a solução.

## Manter o contexto local nos arquivos (não na memória)

O objetivo é nunca precisar reler todas as tarefas para saber o que falta.
Para isso:

- **Sempre** atualize `.tasks/00-manifest.md` a cada transição de status
  (pending → in-progress → done/blocked), incluindo a entrada no "Log de
  mudanças de status" com data/hora e um resumo de uma linha do que foi
  feito (e a referência à issue, quando houver: `Refs #N` / `fecha issue #N`).
  O manifest deve, sozinho, responder "o que já foi feito e o que falta".
- **Sempre** escreva a seção `## Resolução` no arquivo da própria tarefa
  (`.tasks/NN-*.md`) ao concluí-la — documentando decisões tomadas, arquivos
  alterados, e qualquer descoberta relevante (ex. "coluna X já existia") —
  e marque os itens da "Definition of Done" (`[x]`). Essa seção é a fonte
  de verdade sobre COMO algo foi resolvido; evita ter que recuperar esse
  raciocínio relendo commits ou pedindo ao usuário.
- Ao retomar trabalho após interrupção, **leia primeiro o manifest** (e,
  se necessário, a seção Resolução das tarefas mais recentes) — evite reler
  o conjunto inteiro de arquivos de tarefa do zero.
