// Guard de navegação — redireciona não logados para /auth
// e usuários sem permissão para /overview.
// Os paths são relativos à base /app/ do Vue Router.

import type { Router } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

export function useNavigationGuard(router: Router) {
  router.beforeEach(async (to) => {
    // Rota pública — permite acesso direto
    if (to.path === '/auth') return true

    const auth = useAuthStore()

    // Aguarda a checagem de autenticação (fetchMe) se ainda não foi feita
    if (!auth.checked) {
      await auth.fetchMe()
    }

    // Não logado → login
    if (!auth.isLoggedIn) {
      return { path: '/auth', query: { redirect: to.fullPath } }
    }

    // Verifica permissão da rota
    const permissions = to.meta.permissions as string[] | undefined
    if (permissions && permissions.length > 0 && !auth.canAccess(permissions)) {
      // Usuário autenticado mas sem permissão → overview
      return { path: '/overview' }
    }

    return true
  })
}
