// Store de upload (playground) — TUS upload com progresso

import { defineStore } from 'pinia'

import { ref } from 'vue'

import type { UploadInitResponse } from '@/types'

import { api } from '@/api/client'



export interface UploadProgress {

  video_id: string

  tag: string

  upload_url: string

  token: string

  progress: number // 0 a 100

  status: 'init' | 'uploading' | 'done' | 'error'

  error?: string

}



export const useUploadStore = defineStore('upload', () => {

  const uploads = ref<UploadProgress[]>([])

  const currentUpload = ref<UploadProgress | null>(null)

  const lastError = ref<string | null>(null)



  async function initUpload(tag: string, _filename: string, fileSize: number): Promise<UploadInitResponse | null> {

  const res = await api.post<UploadInitResponse>('/api/upload/init', {

    tag,

    declared_size_bytes: fileSize,

  })

  if (res.error) {

    lastError.value = res.error

    return null

  }

  currentUpload.value = {

    video_id: res.data.video_id,

    tag: tag,

    upload_url: res.data.upload_url,

    token: res.data.token,

    progress: 0,

    status: 'init',

  }

  uploads.value.push({ ...currentUpload.value })

  return res.data

  }



  /**

   * Upload via TUS protocol (resumable upload).

   * Recebe o arquivo (File), a URL absoluta de upload e o video_id.

   */

  async function tusUpload(file: File, uploadUrl: string, videoId: string): Promise<boolean> {

  if (!currentUpload.value) return false



  currentUpload.value.status = 'uploading'



  return new Promise((resolve) => {

    const xhr = new XMLHttpRequest()



    xhr.upload.addEventListener('progress', (e) => {

    if (e.lengthComputable && currentUpload.value) {

      currentUpload.value.progress = Math.round((e.loaded / e.total) * 100)

      const idx = uploads.value.findIndex((u) => u.video_id === videoId)

      if (idx >= 0) {

      uploads.value[idx].progress = currentUpload.value.progress

      uploads.value[idx].status = 'uploading'

      }

    }

    })



    xhr.addEventListener('load', () => {

    if (currentUpload.value) {

      if (xhr.status >= 200 && xhr.status < 300) {

      currentUpload.value.status = 'done'

      const idx = uploads.value.findIndex((u) => u.video_id === videoId)

      if (idx >= 0) {

        uploads.value[idx].status = 'done'

      }

      resolve(true)

      } else {

      currentUpload.value.status = 'error'

      currentUpload.value.error = `HTTP ${xhr.status}`

      const idx = uploads.value.findIndex((u) => u.video_id === videoId)

      if (idx >= 0) {

        uploads.value[idx].status = 'error'

        uploads.value[idx].error = `HTTP ${xhr.status}`

      }

      resolve(false)

      }

    }

    })



    xhr.addEventListener('error', () => {

    if (currentUpload.value) {

      currentUpload.value.status = 'error'

      currentUpload.value.error = 'Erro de rede'

      const idx = uploads.value.findIndex((u) => u.video_id === videoId)

      if (idx >= 0) {

      uploads.value[idx].status = 'error'

      uploads.value[idx].error = 'Erro de rede'

      }

    }

    resolve(false)

    })



    xhr.open('PATCH', uploadUrl)

    xhr.setRequestHeader('Content-Type', 'application/offset+octet-stream')

    xhr.setRequestHeader('Upload-Offset', '0')

    xhr.setRequestHeader('Tus-Resumable', '1.0.0')

    // Token de upload como Authorization para o TUS handler

    if (currentUpload.value.token) {

      xhr.setRequestHeader('Authorization', `Bearer ${currentUpload.value.token}`)

    }

    xhr.send(file)

  })

  }



  function reset() {

  uploads.value = []

  currentUpload.value = null

  lastError.value = null

  }



  return {

  uploads,

  currentUpload,

  lastError,

  initUpload,

  tusUpload,

  reset,

  }

})
