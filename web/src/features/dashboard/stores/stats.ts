// Store de estatísticas do dashboard



import { defineStore } from 'pinia'

import { ref } from 'vue'

import type { StatsResponse, QueueResponse } from '@/types'

import { api } from '@/api/client'



export const useStatsStore = defineStore('stats', () => {

  const stats = ref<StatsResponse | null>(null)

  const queue = ref<QueueResponse | null>(null)

  const loadingStats = ref(false)

  const loadingQueue = ref(false)

  const error = ref<string | null>(null)



  async function fetchStats() {

  loadingStats.value = true

  error.value = null

  const res = await api.get<StatsResponse>('/admin/stats')

  if (res.error) {

    error.value = res.error

  } else {

    stats.value = res.data

  }

  loadingStats.value = false

  }



  async function fetchQueue() {

  loadingQueue.value = true

  error.value = null

  const res = await api.get<QueueResponse>('/admin/queue')

  if (res.error) {

    error.value = res.error

  } else {

    queue.value = res.data

  }

  loadingQueue.value = false

  }



  async function fetchAll() {

  await Promise.all([fetchStats(), fetchQueue()])

  }



  function reset() {

  stats.value = null

  queue.value = null

  error.value = null

  loadingStats.value = false

  loadingQueue.value = false

  }



  return {

  stats,

  queue,

  loadingStats,

  loadingQueue,

  error,

  fetchStats,

  fetchQueue,

  fetchAll,

  reset,

  }

})