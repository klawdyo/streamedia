// Store de listagem de vídeos (admin)

import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { Video } from '@/types'
import { api } from '@/api/client'

interface VideosListResponse {
  videos: Video[]
  total: number
}

export interface VideoFilters {
  status?: string
  tag?: string
  sort?: string
  page?: number
  limit?: number
}

export const useVideosStore = defineStore('videos', () => {
  const videos = ref<Video[]>([])
  const total = ref(0)
  const page = ref(1)
  const limit = ref(20)
  const totalPages = ref(0)
  const loading = ref(false)
  const error = ref<string | null>(null)
  const filters = ref<VideoFilters>({})

  async function fetchVideos() {
    loading.value = true
    error.value = null

    const params = new URLSearchParams()
    if (filters.value.status) params.set('status', filters.value.status)
    if (filters.value.tag) params.set('tag', filters.value.tag)
    if (filters.value.sort) params.set('sort', filters.value.sort)
    const l = filters.value.limit ?? limit.value
    params.set('limit', String(l))
    const p = filters.value.page ?? page.value
    params.set('offset', String((p - 1) * l))

    const res = await api.get<VideosListResponse>(`/admin/videos?${params.toString()}`)
    if (res.error) {
      error.value = res.error
      loading.value = false
      return
    }

    const list = res.data?.videos ?? []
    videos.value = list
    total.value = res.data?.total ?? list.length
    page.value = filters.value.page ?? page.value
    totalPages.value = Math.max(1, Math.ceil(total.value / l))
    loading.value = false
  }

  async function deleteVideo(videoId: string) {
    const res = await api.del<{ ok: boolean }>(`/admin/videos/${videoId}`)
    if (res.error) {
      error.value = res.error
      return false
    }
    videos.value = videos.value.filter((v) => v.video_id !== videoId)
    return true
  }

  async function reprocessVideo(videoId: string) {
    const res = await api.post<{ ok: boolean }>(`/api/videos/${videoId}/reprocess`)
    if (res.error) {
      error.value = res.error
      return false
    }
    return true
  }

  function setFilters(newFilters: Partial<VideoFilters>) {
    filters.value = { ...filters.value, ...newFilters }
  }

  function reset() {
    videos.value = []
    total.value = 0
    page.value = 1
    totalPages.value = 0
    loading.value = false
    error.value = null
    filters.value = {}
  }

  return {
    videos,
    total,
    page,
    limit,
    totalPages,
    loading,
    error,
    filters,
    fetchVideos,
    deleteVideo,
    reprocessVideo,
    setFilters,
    reset,
  }
})
