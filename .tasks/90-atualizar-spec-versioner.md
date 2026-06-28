# T90 — Atualizar spec/ + .agents/versioner.md (package.json sync)

**Status:** done
**Depende de:** T89
**Issue relacionada:** — (parte do admin unificado, spec/admin-unificado.md §10)

## Objetivo

Atualizar a documentação da spec para refletir o estado pós-admin-unificado
e configurar o versioner para sincronizar `web/package.json` com o `VERSION`
a cada release.

## QA Instructions

- Verificar que spec/api.md reflete as rotas atuais (sem legado)
- Verificar que spec/operacao.md documenta a migração de env vars para o banco
- Verificar que spec/README.md lista admin-unificado.md
- Verificar que .agents/versioner.md inclui passo de sync package.json
- Verificar que nenhuma referência a rotas legadas nos arquivos de spec

## Dev Instructions

1. Atualizar `spec/api.md`: remover referências a /docs, /playground, /dashboard
2. Atualizar `spec/operacao.md`: dividir env vars em "ambiente" vs "banco"
3. Atualizar `spec/README.md`: adicionar admin-unificado.md ao índice
4. Atualizar `.agents/versioner.md`: adicionar passo de sync web/package.json
5. Criar `spec/admin-unificado.md` com a especificação completa

## Definition of Done

- [x] spec/api.md atualizado sem rotas legadas
- [x] spec/operacao.md atualizado com env vars corretas
- [x] spec/README.md inclui admin-unificado.md
- [x] spec/admin-unificado.md criado com especificação completa
- [x] .agents/versioner.md atualizado com sync package.json

## Resolução

**Data:** 2026-06-28
**Commits:** `ebe4a39` (spec/admin-unificado.md criado) + `21500f8` (atualizações spec + versioner)

Arquivos criados/modificados:
- `spec/admin-unificado.md` (497 linhas): Especificação completa do admin unificado — arquitetura, database (3 novas tabelas), sistema de roles (4 níveis com ACL), ENV vs DB (16 configs migradas), endpoints e autorização (Google OAuth, RoleAuth, Config API), frontend (Vue 3 + Vite + shadcn-vue + Tailwind), Docker/Coolify, remoção de legado.
- `spec/api.md`: Rotas legadas removidas da tabela de públicas; documentação aponta para admin unificado em `/app`.
- `spec/operacao.md`: Variáveis de ambiente divididas em "obrigatórias no boot" (8) vs "migradas para o banco" (12). Referência a /dashboard substituída por /app.
- `spec/README.md`: admin-unificado.md adicionado ao índice (8º arquivo).
- `.agents/versioner.md`: Passo 4 reorganizado — atualizar VERSION, depois `web/package.json` campo "version", depois commit release incluindo ambos. Definition of Done inclui item de sync package.json.
- `web/package.json`: Campo "version" inicializado com `1.11.1` (mesmo valor do VERSION).

Decisão: `spec/admin-unificado.md` serve como registro completo da especificação que guiou T75-T90, mas a fonte de verdade última permanece o código. O arquivo será mantido como documentação de design (não removido).
