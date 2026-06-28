// Composable para consumir eventos SSE (Server-Sent Events) de um vídeo específico.

// Conecta em /api/events?video_id=X&token=Y.

// O token pode ser uma string fixa ou uma função getter (útil para ler do localStorage).



import { ref, onUnmounted } from 'vue'

import type { SSEEvent } from '@/types'



export function useSSE(videoId: string, tokenOrGetter: string | (() => string)) {

  const events = ref<SSEEvent[]>([])

  const connected = ref(false)

  const error = ref<string | null>(null)

  let eventSource: EventSource | null = null



  function getToken(): string {

  return typeof tokenOrGetter === 'function' ? tokenOrGetter() : tokenOrGetter

  }



  function connect() {

  if (eventSource) return



  const token = getToken()

  const url = `/api/events?video_id=${encodeURIComponent(videoId)}&token=${encodeURIComponent(token)}`

  eventSource = new EventSource(url)



  eventSource.onopen = () => {

    connected.value = true

    error.value = null

  }



  eventSource.onmessage = (msg) => {

    try {

    const data = JSON.parse(msg.data) as SSEEvent

    events.value.push(data)

    } catch {

    // ignora eventos não-JSON

    }

  }



  eventSource.addEventListener('status', (msg) => {

    try {

    const data = JSON.parse(msg.data) as SSEEvent

    events.value.push({ ...data, event: 'status' })

    } catch {

    // ignora

    }

  })



  eventSource.addEventListener('progress', (msg) => {

    try {

    const data = JSON.parse(msg.data) as SSEEvent

    events.value.push({ ...data, event: 'progress' })

    } catch {

    // ignora

    }

  })



  eventSource.onerror = () => {

    connected.value = false

    error.value = 'Conexão SSE perdida'

    close()

  }

  }



  function close() {

  if (eventSource) {

    eventSource.close()

    eventSource = null

  }

  connected.value = false

  }



  function clearEvents() {

  events.value = []

  }



  // Limpeza automática ao desmontar o componente

  onUnmounted(() => {

  close()

  })



  return {

  events,

  connected,

  error,

  connect,

  close,

  clearEvents,

  }

}