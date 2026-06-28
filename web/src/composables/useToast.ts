// Sistema de snackbar/toast global — feedback visual de sucesso e erro
import { ref } from 'vue'

export interface Toast {
  id: number
  message: string
  type: 'success' | 'error' | 'info'
}

let nextId = 0
const toasts = ref<Toast[]>([])

export function useToast() {
  function show(message: string, type: Toast['type'] = 'info', durationMs = 4000) {
    const id = nextId++
    toasts.value.push({ id, message, type })
    if (durationMs > 0) {
      setTimeout(() => dismiss(id), durationMs)
    }
  }

  function dismiss(id: number) {
    const idx = toasts.value.findIndex((t) => t.id === id)
    if (idx >= 0) {
      toasts.value.splice(idx, 1)
    }
  }

  function success(message: string) {
    show(message, 'success')
  }

  function error(message: string) {
    show(message, 'error', 6000)
  }

  function info(message: string) {
    show(message, 'info')
  }

  return { toasts, show, dismiss, success, error, info }
}

// Singleton — mesmo estado em qualquer lugar
export const toast = useToast()
