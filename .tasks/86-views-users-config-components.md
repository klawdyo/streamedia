# T86 — Views: Users, Config + RolesSelect + ConfigEditor

**Status:** done
**Depende de:** T84
**Issue relacionada:** — (parte do admin unificado, spec/admin-unificado.md §3, §5.5)

## Objetivo

Implementar as views de administração de usuários e configurações: UsersView
(tabela de usuários com CRUD), ConfigView (editor de configurações agrupadas),
RolesSelect (componente de seleção de roles com validação de nível), e
ConfigEditor (input dinâmico por tipo de config).

## QA Instructions

- Testar UsersView: lista usuários, cria novo (email + role), edita roles, deleta
- Testar RolesSelect: mostra roles disponíveis, valida regra ACL (nível)
- Testar ConfigView: agrupamento correto, edição por tipo (number/string/boolean/duration)
- Testar ConfigEditor: input number, checkbox boolean, select duration, text string
- Testar validação regex no ConfigEditor
- Testar que secrets (visible=false) não aparecem na lista de configs

## Dev Instructions

1. Criar `web/src/features/users/views/UsersView.vue` + UsersTable + stores/users
2. Criar `web/src/features/users/components/RolesSelect.vue` com lógica ACL
3. Criar `web/src/features/config/views/ConfigView.vue` + ConfigEditor + stores/config
4. Criar `web/src/features/config/components/ConfigEditor.vue` com input dinâmico

## Definition of Done

- [x] UsersView com CRUD completo e RolesSelect integrado
- [x] RolesSelect com validação de nível (ACL)
- [x] ConfigView com agrupamento e editores por tipo
- [x] ConfigEditor com validação regex e type-specific inputs
- [x] `vue-tsc --noEmit` passa sem erros

## Resolução

**Data:** 2026-06-28
**Commit:** `35e3dc1` feat(T84-T86): frontend completo - router, stores, guards, menu, views e componentes

Views implementadas:
- `web/src/features/users/views/UsersView.vue`: Tabela de usuários (email, nome, avatar, roles, ações). Botão "Adicionar usuário" abre dialog com input de email + RolesSelect. Ações: editar roles (dialog com RolesSelect), remover (confirm dialog). Store `users.ts` com fetchUsers, createUser, updateUserRoles, deleteUser.
- `web/src/features/users/components/UsersTable.vue`: Tabela com colunas: avatar, email, nome, roles (Badge por role), actions dropdown. Paginação integrada.
- `web/src/features/users/components/RolesSelect.vue`: Multi-select de roles disponíveis (dev, admin, acl, manager) com badge de nível. Validação ACL: roles que o usuário atual não pode conceder aparecem desabilitadas com tooltip explicativo.
- `web/src/features/config/views/ConfigView.vue`: Accordion por grupo de configuração (Paths, Upload, Transcodificação, Tokens, Rate Limiting, Webhook, Discord, Sessão). Cada grupo expande mostrando ConfigEditor para cada item.
- `web/src/features/config/components/ConfigEditor.vue`: Input dinâmico baseado no `type` da config:
  - `string` → text input
  - `number` → number input com min/max
  - `boolean` → toggle switch
  - `duration_seconds` → input number com label "segundos"
  - `url` → text input com validação de URL
  - `secret` → password input (só em PUT, nunca visível)
  Validação regex aplicada no blur, com mensagem de erro inline. Botão "Restaurar default" se valor ≠ default. Store `config.ts` com fetchConfigs, updateConfig, deleteConfig.
- `web/src/features/dashboard/stores/stats.ts`: Store de stats com fetchStats, dados para gráficos.
- `web/src/features/dashboard/stores/queue.ts`: Store da fila com fetchQueue (polling opcional).

Decisão: ConfigEditor usa componente único com switch por tipo — evita duplicação de lógica de validação/save. RolesSelect valida regra ACL no frontend (feedback imediato) E backend (segurança real).
