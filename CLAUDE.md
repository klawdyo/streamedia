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

## Fluxo de branches (Git Flow simplificado)

- **`main`** — branch principal/estável. Reflete sempre o que está em produção.
  Só recebe merge **via Pull Request**, e somente quando o usuário autorizar
  explicitamente. O PR deve descrever em detalhes tudo o que foi alterado.
- **`dev`** — branch de integração. **Todo o desenvolvimento parte daqui.**
- **Branches de feature** — para cada nova funcionalidade/tarefa, crie uma
  branch a partir de `dev` (ex.: `feature/nome-da-feature`). Ao concluir,
  faça merge de volta para `dev`.

```
dev → feature/xyz → (trabalho) → merge de volta em dev
dev → ... → (quando autorizado) → Pull Request dev → main
```

**Nunca** dê push direto em `main`. **Nunca** abra PR para `main` sem pedido
explícito do usuário.

## Arquivos do sistema

- `spec/ESPECIFICACAOv4.md` — Especificação técnica completa (fonte de verdade)
- `.agents/cto.md` — Instruções e workflow do agente CTO
- `.agents/dev.md` — Instruções do agente Dev
- `.agents/qa.md` — Instruções do agente QA
- `.agents/versioner.md` — Instruções do agente Versioner (versionamento semântico)
- `.tasks/00-manifest.md` — Lista de tarefas e status de cada uma
- `.tasks/NN-*.md` — Arquivos de tarefa individuais (autocontidos)

## Versionamento semântico

A versão do projeto segue `MAJOR.MINOR.PATCH` derivada dos commits semânticos
(Conventional Commits). Ver `.agents/versioner.md` para o algoritmo completo.
Resumo das regras, aplicadas em ordem cronológica desde a última tag:

- `feat:` → incrementa `MINOR` e zera `PATCH` (cada feature inicia um novo ciclo)
- `fix:` → incrementa `PATCH`
- `BREAKING CHANGE:` / `feat!:` / `fix!:` → incrementa `MAJOR`, zera `MINOR` e `PATCH`
- `chore:`, `docs:`, `refactor:`, `test:`, etc. → não alteram a versão

Exemplo: `fix, fix, feat, fix` a partir de `0.0.0` → `0.0.1` → `0.0.2` → `0.1.0` → `0.1.1`.
Se vier outro `feat` em seguida, o ciclo reinicia: `feat, fix` → `0.2.0` → `0.2.1`.

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
