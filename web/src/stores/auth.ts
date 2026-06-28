// Auth store (Pinia setup syntax) — Google OAuth via /api/auth/me.

// Gerencia permissões por roles (roles ∩ route.permissions).



import { defineStore } from 'pinia'

import { ref, computed } from 'vue'

import type { UserRole, AuthResponse } from '@/types'

import { api } from '@/api/client'

import type { Pinia } from 'pinia'



export const useAuthStore = defineStore('auth', () => {

  const user = ref<AuthResponse | null>(null)

  const checked = ref(false)



  const isLoggedIn = computed(() => user.value !== null)

  const roles = computed(() => user.value?.roles ?? [])

  const effectiveLevel = computed(() => user.value?.effective_level ?? 0)



  // Guarda referência externa das stores que precisam de reset

  const resetFns: (() => void)[] = []



  function canAccess(permissions: string[]): boolean {

  if (permissions.length === 0) return true

  const userRoles = roles.value.map((r: UserRole) => r.role)

  return permissions.some((p) => userRoles.includes(p))

  }



  async function fetchMe(): Promise<boolean> {

  const res = await api.get<AuthResponse>('/api/auth/me')

  if (res.error) {

    user.value = null

    checked.value = true

    return false

  }

  user.value = res.data

  checked.value = true

  return true

  }



  function logout() {

  user.value = null

  checked.value = false

  resetAll()

  }



  /**

   * Registra uma store para ser resetada no logout.

   * Chamado por cada store no seu setup.

   */

  function registerReset(fn: () => void) {

  resetFns.push(fn)

  }



  function resetAll() {

  for (const fn of resetFns) {

    fn()

  }

  }



  return {

  user,

  checked,

  isLoggedIn,

  roles,

  effectiveLevel,

  canAccess,

  fetchMe,

  logout,

  registerReset,

  resetAll,

  }

})



/**

 * Helper: acessa a store sem passar o Pinia (após instalado).

 * Para uso em guards e composables fora de setup.

 */

export function getAuthStore(pinia?: Pinia) {

  return useAuthStore(pinia)

}