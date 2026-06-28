// Store de upload (playground) — TUS upload com progresso



import { defineStore } from 'pinia'

import { ref } from 'vue'

import type { UploadInitResponse } from '@/types'

import { api } from '@/api/client'



export interface UploadProgress {

  upload_id: string

  tag: string

  progress: number // 0 a 100

  status: 'init' | 'uploading' | 'done' | 'error'

  error?: string

  video_id?: string

}



export const useUploadStore = defineStore('upload', () => {

  const uploads = ref<UploadProgress[]>([])

  const currentUpload = ref<UploadProgress | null>(null)



  async function initUpload(tag: string, _filename: string, fileSize: number): Promise<UploadInitResponse | null> {

  const res = await api.post<UploadInitResponse>('/api/upload/init', {

    tag,

    declared_size_bytes: fileSize,

  })

  if (res.error) {

    return null

  }

  currentUpload.value = {

    upload_id: res.data.upload_id,

    tag,

    progress: 0,

    status: 'init',

    video_id: res.data.video_id,

  }

  uploads.value.push({ ...currentUpload.value })

  return res.data

  }



  /**

   * Upload via TUS protocol (resumable upload).

   * Recebe o arquivo (File), a location do init, e reporta progresso.

   */

  async function tusUpload(file: File, location: string, uploadId: string): Promise<boolean> {

  if (!currentUpload.value) return false



  currentUpload.value.status = 'uploading'



  return new Promise((resolve) => {

    const xhr = new XMLHttpRequest()



    xhr.upload.addEventListener('progress', (e) => {

    if (e.lengthComputable && currentUpload.value) {

      currentUpload.value.progress = Math.round((e.loaded / e.total) * 100)

      // Atualiza também na lista

      const idx = uploads.value.findIndex((u) => u.upload_id === uploadId)

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

      const idx = uploads.value.findIndex((u) => u.upload_id === uploadId)

      if (idx >= 0) {

        uploads.value[idx].status = 'done'

      }

      resolve(true)

      } else {

      currentUpload.value.status = 'error'

      currentUpload.value.error = `HTTP ${xhr.status}`

      const idx = uploads.value.findIndex((u) => u.upload_id === uploadId)

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

      const idx = uploads.value.findIndex((u) => u.upload_id === uploadId)

      if (idx >= 0) {

      uploads.value[idx].status = 'error'

      uploads.value[idx].error = 'Erro de rede'

      }

    }

    resolve(false)

    })



    xhr.open('PATCH', location)

    xhr.setRequestHeader('Content-Type', 'application/offset+octet-stream')

    xhr.setRequestHeader('Upload-Offset', '0')

    xhr.setRequestHeader('Tus-Resumable', '1.0.0')

    xhr.send(file)

  })

  }



  function reset() {

  uploads.value = []

  currentUpload.value = null

  }



  return {

  uploads,

  currentUpload,

  initUpload,

  tusUpload,

  reset,

  }

})