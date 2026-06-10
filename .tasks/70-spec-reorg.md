# T70 — Reorganizar a especificação em arquivos menores + índice

**Status:** done
**Origem:** solicitação direta do usuário.
**Depende de:** T69 (a spec já deve descrever só o fluxo atual).

## Objetivo

O `spec/ESPECIFICACAOv4.md` (≈900 linhas) é grande demais para consulta
pontual. Dividir em arquivos temáticos menores, e transformar o arquivo
unificado num **índice** que relaciona cada arquivo e explica o que contém —
para consultar só o que interessa sem ler o documento inteiro.

## Escopo

- Criar `spec/` com arquivos por tema, por exemplo:
  - `00-indice.md` (ou manter `ESPECIFICACAOv4.md` como índice) — visão geral +
    links e resumo de uma linha de cada arquivo.
  - `arquitetura.md` — papel do serviço, componentes, fluxo de dados.
  - `autenticacao.md` — ROOT_TOKEN, tokens efêmeros (access_tokens), tags.
  - `api-upload.md` — `/api/upload/init` + TUS `/files`.
  - `api-play.md` — `/api/play/init` + serving `/video/<tag>/<id>.m3u8`.
  - `api-admin.md` — `/admin/*` e `/api/status`.
  - `transcodificacao.md` — pipeline FFmpeg/HLS, fila, recovery.
  - `webhooks.md` — eventos e assinatura.
  - `dados.md` — schema (videos, access_tokens, renditions, eventos).
  - `operacao.md` — variáveis de ambiente, deploy, observabilidade.
  (Os agrupamentos exatos ficam a critério de quem executa; o importante é que
  cada arquivo seja autocontido e o índice explique todos.)
- Remover do conteúdo qualquer resquício do fluxo antigo (alinhado à T69).
- Atualizar referências ao caminho da spec onde houver (ex. `CLAUDE.md`).

## Definition of Done

- [x] `spec/` dividida em arquivos temáticos pequenos.
- [x] Índice único relacionando e resumindo cada arquivo.
- [x] Conteúdo descreve apenas o fluxo atual (tag + ROOT_TOKEN).
- [x] `CLAUDE.md` aponta para o novo índice.

## Resolução

`spec/ESPECIFICACAOv4.md` (894 linhas, fluxo antigo) removido. Criados arquivos
temáticos enxutos baseados na implementação atual: `spec/README.md` (índice),
`arquitetura.md`, `autenticacao.md`, `api.md`, `dados.md`, `pipeline.md`,
`webhooks.md`, `operacao.md`. Referências atualizadas em `CLAUDE.md`,
`.agents/cto.md` e `.agents/README.md` para `spec/README.md`.
