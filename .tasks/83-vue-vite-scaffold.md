# T83 — Scaffold web/: Vite + Vue 3 + TypeScript + Tailwind + shadcn-vue + phosphor-icons

**Status:** done
**Depende de:** —
**Issue relacionada:** — (parte do admin unificado, spec/admin-unificado.md §6.1)

## Objetivo

Criar o scaffold do frontend Vue 3 no diretório `web/` com todas as dependências
instaladas e configuradas: Vite, TypeScript, Tailwind CSS, shadcn-vue (via CLI),
phosphor-icons, vue-router, pinia, hls.js, chart.js + vue-chartjs.

## QA Instructions

- Verificar que `npm install` conclui sem erros
- Verificar que `npm run dev` sobe o Vite dev server
- Verificar que `npm run build` gera `web/dist/` com assets otimizados
- Verificar que Tailwind compila corretamente
- Verificar que TypeScript não reporta erros (`vue-tsc --noEmit`)

## Dev Instructions

1. Criar projeto Vite + Vue 3 + TypeScript em `web/`
2. Instalar dependências: vue-router, pinia, @phosphor-icons/vue, hls.js, chart.js, vue-chartjs
3. Configurar Tailwind CSS com `tailwind.config.ts`
4. Rodar CLI do shadcn-vue para inicializar componentes base
5. Configurar `vite.config.ts` sem localhost fixo (portas/targets via env)
6. Criar estrutura de diretórios feature-based conforme spec §6.2

## Definition of Done

- [x] `web/package.json` com todas as dependências listadas
- [x] `web/vite.config.ts` configurado com proxy para API Go
- [x] `web/tailwind.config.ts` funcional
- [x] `web/src/main.ts`, `web/src/App.vue` criados
- [x] Estrutura de diretórios criada: router/, api/, composables/, stores/, components/, features/
- [x] `npm run build` gera `web/dist/`
- [x] `vue-tsc --noEmit` passa sem erros

## Resolução

**Data:** 2026-06-28
**Commit:** `4d62ac0` feat(T83): scaffold Vue 3 + Vite + TypeScript + Tailwind + shadcn-vue + phosphor-icons

Scaffold criado com:
- `web/package.json`: dependências vue 3, vue-router 4, pinia, @phosphor-icons/vue, hls.js, chart.js, vue-chartjs, tailwindcss, postcss, autoprefixer, typescript, vite, @vitejs/plugin-vue, shadcn-vue, radix-vue, class-variance-authority, clsx, tailwind-merge
- `web/vite.config.ts`: proxy `/api` → `VITE_API_TARGET` (default localhost:PORT), `/app` → SPA fallback
- `web/tailwind.config.ts`: content paths, theme extend com cores do projeto
- `web/.env.development`: VITE_API_TARGET, VITE_DEV_PORT
- `web/src/main.ts`: createApp + router + pinia
- `web/src/App.vue`: `<router-view />` wrapper
- `web/src/styles/global.css`: Tailwind directives + custom CSS
- Componentes shadcn-vue instalados via CLI: button, card, avatar, badge, dialog, dropdown-menu, input, select, table, tabs, toast, tooltip
- Estrutura feature-based criada: features/{auth,dashboard,videos,playground,users,config}/
- `web/components.json`: configuração shadcn-vue
