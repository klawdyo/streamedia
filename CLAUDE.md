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

## Arquivos do sistema

- `spec/ESPECIFICACAOv4.md` — Especificação técnica completa (fonte de verdade)
- `.agents/cto.md` — Instruções e workflow do agente CTO
- `.agents/dev.md` — Instruções do agente Dev
- `.agents/qa.md` — Instruções do agente QA
- `.tasks/00-manifest.md` — Lista de tarefas e status de cada uma
- `.tasks/NN-*.md` — Arquivos de tarefa individuais (autocontidos)

## Convenção de idioma

- Identificadores, nomes de arquivo, pacotes: **inglês**
- Comentários no código, mensagens de erro da API, README: **português**
- Comentários abundantes, mesmo em trechos óbvios

## Workflow rápido

```
CTO lê manifest → lê tarefa → QA escreve testes → Dev implementa → QA verifica → CTO atualiza manifest → próxima
```

Ver `.agents/cto.md` para o workflow detalhado.
