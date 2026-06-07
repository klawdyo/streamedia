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

Quando uma tarefa (`.tasks/NN-*.md`) tem uma "Issue relacionada" e seu
trabalho conclui ou resolve essa issue:

- **Comente na issue** descrevendo a solução executada (o que foi feito,
  arquivos/rotas envolvidos, como verificar).
- **Referencie a issue no(s) commit(s)** que a fecham (ex. `Closes #2` ou
  `Refs #2` na mensagem do commit), para que o vínculo fique rastreável no
  histórico do GitHub.
