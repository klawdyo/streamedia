// Store de detalhe de um vídeo único

import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { VideoStatusResponse, PlayInitResponse } from '@/types'
import { api } from '@/api/client'

export const useVideoStore = defineStore('video', () => {
  const video = ref<VideoStatusResponse | null>(null)
  const playData = ref<PlayInitResponse | null>(null)
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function fetchStatus(videoId: string) {
    loading.value = true
    error.value = null
    const res = await api.get<VideoStatusResponse>(`/api/status/${videoId}`)
    if (res.error) {
      error.value = res.error
    } else {
      video.value = res.data
    }
    loading.value = false
  }

  async function initPlay(videoId: string, token: string) {
    const res = await api.post<PlayInitResponse>('/api/play/init', { video_id: videoId, token })
    if (res.error) {
      error.value = res.error
      return null
    }
    playData.value = res.data
    return res.data
  }

  function reset() {
    video.value = null
    playData.value = null
    loading.value = false
    error.value = null
  }

  return {
    video,
    playData,
    loading,
    error,
    fetchStatus,
    initPlay,
    reset,
  }
})
