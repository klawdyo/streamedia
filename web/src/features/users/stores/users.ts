// Store de usuários (admin) — CRUD + gerenciamento de roles

import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { UserWithRoles } from '@/types'
import type { PaginatedResponse } from '@/types'
import { api } from '@/api/client'

export const useUsersStore = defineStore('users', () => {
  const users = ref<UserWithRoles[]>([])
  const total = ref(0)
  const page = ref(1)
  const totalPages = ref(0)
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function fetchUsers(params?: { page?: number; per_page?: number; search?: string }) {
    loading.value = true
    error.value = null

    const qs = new URLSearchParams()
    if (params?.page) qs.set('page', String(params.page))
    if (params?.per_page) qs.set('per_page', String(params.per_page))
    if (params?.search) qs.set('search', params.search)

    const res = await api.get<PaginatedResponse<UserWithRoles>>(`/admin/users?${qs.toString()}`)
    if (res.error) {
      error.value = res.error
    } else {
      users.value = res.data as unknown as UserWithRoles[]
      if (res.meta) {
        total.value = res.meta.total
        page.value = res.meta.page
        totalPages.value = res.meta.total_pages
      }
    }
    loading.value = false
  }

  async function createUser(email: string, name: string): Promise<UserWithRoles | null> {
    const res = await api.post<UserWithRoles>('/admin/users', { email, name })
    if (res.error) {
      error.value = res.error
      return null
    }
    users.value.unshift(res.data)
    return res.data
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
    // Atualiza local
    const idx = users.value.findIndex((u) => u.id === userId)
    if (idx >= 0) {
      users.value[idx] = res.data
    }
    return res.data
  }

  function reset() {
    users.value = []
    total.value = 0
    page.value = 1
    totalPages.value = 0
    loading.value = false
    error.value = null
  }

  return {
    users,
    total,
    page,
    totalPages,
    loading,
    error,
    fetchUsers,
    createUser,
    deleteUser,
    updateRoles,
    reset,
  }
})
