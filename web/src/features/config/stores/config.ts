// Store de configurações dinâmicas (admin)



import { defineStore } from 'pinia'

import { ref } from 'vue'

import type { ConfigGroup, ConfigItem } from '@/types'

import { api } from '@/api/client'



export const useConfigStore = defineStore('config', () => {

  const groups = ref<ConfigGroup[]>([])

  const loading = ref(false)

  const error = ref<string | null>(null)



  async function fetchConfig() {

  loading.value = true

  error.value = null

  const res = await api.get<ConfigGroup[]>('/admin/config')

  if (res.error) {

    error.value = res.error

  } else {

    groups.value = res.data

  }

  loading.value = false

  }



  async function updateConfig(key: string, value: string) {

  const res = await api.put<ConfigItem>(`/admin/config/${key}`, { value })

  if (res.error) {

    error.value = res.error

    return false

  }

  // Atualiza localmente

  for (const group of groups.value) {

    const item = group.items.find((i) => i.key === key)

    if (item) {

    item.value = value

    break

    }

  }

  return true

  }



  async function deleteConfig(key: string) {

  const res = await api.del<{ ok: boolean }>(`/admin/config/${key}`)

  if (res.error) {

    error.value = res.error

    return false

  }

  // Remove localmente

  for (const group of groups.value) {

    const idx = group.items.findIndex((i) => i.key === key)

    if (idx >= 0) {

    group.items.splice(idx, 1)

    break

    }

  }

  return true

  }



  function reset() {

  groups.value = []

  loading.value = false

  error.value = null

  }



  return {

  groups,

  loading,

  error,

  fetchConfig,

  updateConfig,

  deleteConfig,

  reset,

  }

})