// Store de listagem de vídeos (admin)

import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { Video } from '@/types'
import type { PaginatedResponse } from '@/types'
import { api } from '@/api/client'

export interface VideoFilters {
  status?: string
  tag?: string
  sort?: string
  page?: number
  per_page?: number
}

export const useVideosStore = defineStore('videos', () => {
  const videos = ref<Video[]>([])
  const total = ref(0)
  const page = ref(1)
  const perPage = ref(20)
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
    params.set('page', String(filters.value.page || page.value))
    params.set('per_page', String(filters.value.per_page || perPage.value))

    const res = await api.get<PaginatedResponse<Video>>(`/admin/videos?${params.toString()}`)
    if (res.error) {
      error.value = res.error
    } else {
      const pageData = res as unknown as PaginatedResponse<Video>
      videos.value = pageData.data || (res.data as unknown as Video[])
      if (res.meta) {
        total.value = res.meta.total
        page.value = res.meta.page
        totalPages.value = res.meta.total_pages
      }
    }
    loading.value = false
  }

  async function deleteVideo(videoId: string) {
    const res = await api.del<{ ok: boolean }>(`/admin/videos/${videoId}`)
    if (res.error) {
      error.value = res.error
      return false
    }
    // Remove da lista local
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
    perPage,
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
