// Store de usuários (admin) — CRUD + gerenciamento de roles



import { defineStore } from 'pinia'

import { ref } from 'vue'

import type { UserWithRoles, UserRole } from '@/types'

import { api } from '@/api/client'



interface UsersListResponse {

  users: UserWithRoles[]

  total: number

}



function computeEffectiveLevel(roles: UserRole[]): number {

  if (!roles.length) return 999

  return Math.min(...roles.map((r) => r.level_num))

}



export const useUsersStore = defineStore('users', () => {

  const users = ref<UserWithRoles[]>([])

  const total = ref(0)

  const page = ref(1)

  const loading = ref(false)

  const error = ref<string | null>(null)



  async function fetchUsers() {

  loading.value = true

  error.value = null



  const res = await api.get<UsersListResponse>('/admin/users')

  if (res.error) {

    error.value = res.error

    loading.value = false

    return

  }



  const list = res.data?.users ?? []

  users.value = list.map((u) => ({

    ...u,

    effective_level: computeEffectiveLevel(u.roles),

  }))



  total.value = res.data?.total ?? list.length

  loading.value = false

  }



  async function createUser(email: string): Promise<UserWithRoles | null> {

  const res = await api.post<UserWithRoles>('/admin/users', { email })

  if (res.error) {

    error.value = res.error

    return null

  }

  const user = res.data

  user.effective_level = computeEffectiveLevel(user.roles)

  users.value.unshift(user)

  return user

  }



  async function deleteUser(userId: number) {

  const res = await api.del<{ ok: boolean }>(`/admin/users/${userId}`)

  if (res.error) {

    error.value = res.error

    return false

  }

  users.value = users.value.filter((u) => u.id !== userId)

  return true

  }



  async function updateRoles(userId: number, roles: string[]) {

  const res = await api.put<UserWithRoles>(`/admin/users/${userId}/roles`, { roles })

  if (res.error) {

    error.value = res.error

    return null

  }

  const user = res.data

  user.effective_level = computeEffectiveLevel(user.roles)

  const idx = users.value.findIndex((u) => u.id === userId)

  if (idx >= 0) {

    users.value[idx] = user

  }

  return user

  }



  function reset() {

  users.value = []

  total.value = 0

  page.value = 1

  loading.value = false

  error.value = null

  }



  return {

  users,

  total,

  page,

  loading,

  error,

  fetchUsers,

  createUser,

  deleteUser,

  updateRoles,

  reset,

  }

})