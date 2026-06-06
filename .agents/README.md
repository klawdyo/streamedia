# Sistema Multi-Agente — Streamedia

## Visão geral

O desenvolvimento do media server é orquestrado por três agentes especializados
que trabalham em sequência para cada micro-tarefa.

```
┌─────────────────────────────────────────────────────────┐
│                    AGENTE CTO                           │
│  claude-sonnet-4-6                                      │
│  - Lê spec completa e cria micro-tarefas                │
│  - Mantém manifest de status em .tasks/00-manifest.md   │
│  - Orquestra QA e Dev para cada tarefa                  │
│  - Armazena todo contexto no repositório, não em memória│
└──────────────┬──────────────────┬───────────────────────┘
               │                  │
               ▼                  ▼
┌──────────────────────┐  ┌──────────────────────┐
│    AGENTE QA         │  │    AGENTE DEV        │
│  claude-haiku-4-5    │  │  claude-opus-4-8     │
│  - Recebe tarefa     │  │  - Recebe tarefa     │
│  - Escreve testes    │  │  - Recebe testes QA  │
│  - Verifica passing  │  │  - Implementa até    │
│  - Roda antes E      │  │    testes passarem   │
│    depois do Dev     │  │  - Go sênior media   │
└──────────────────────┘  └──────────────────────┘
```

## Workflow por tarefa

1. **CTO** lê `.tasks/00-manifest.md` → encontra próxima tarefa `pending`
2. **CTO** lê `.tasks/NN-nome.md` → extrai contexto da tarefa
3. **CTO** atualiza manifest: status `in-progress`
4. **CTO** spawna **QA** com contexto da tarefa → QA escreve testes (devem falhar)
5. **CTO** spawna **Dev** com contexto + arquivos de teste → Dev implementa
6. **CTO** spawna **QA** novamente → QA verifica que testes passam
7. **CTO** atualiza manifest: status `done`
8. **CTO** faz commit com mensagem descritiva
9. Repete para próxima tarefa

## Contratos entre agentes

### CTO → QA (escrita de testes)

```
Tarefa: [título]
Arquivo de tarefa: .tasks/NN-nome.md
Instrução: Leia o arquivo de tarefa e escreva os testes conforme
           a seção "QA Instructions". Testes devem FALHAR antes
           da implementação (red-green-refactor).
```

### CTO → Dev (implementação)

```
Tarefa: [título]
Arquivo de tarefa: .tasks/NN-nome.md
Testes escritos pelo QA em: [lista de arquivos]
Instrução: Implemente conforme "Dev Instructions" até todos
           os testes passarem. Siga convenção de idioma (CLAUDE.md).
```

### CTO → QA (verificação)

```
Tarefa: [título]
Instrução: Rode os testes e confirme que todos passam.
           Se algum falhar, reporte o erro exato.
```

## Filosofia de contexto mínimo

Cada agente recebe APENAS o que precisa para sua tarefa:
- O arquivo `.tasks/NN-nome.md` já contém todo o contexto necessário
- Dev e QA NÃO precisam ler a spec completa
- O CTO é o único que conhece a spec completa e o estado global

## Arquivos relevantes

- `spec/ESPECIFICACAOv4.md` — Spec completa (só o CTO precisa)
- `.tasks/00-manifest.md` — Estado atual de todas as tarefas
- `.tasks/NN-*.md` — Tarefa individual (autocontida)
