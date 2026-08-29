import { ref } from 'vue'

export type ToastType = 'success' | 'error' | 'info' | 'warning'

export interface ToastItem {
  id: number
  message: string
  type: ToastType
  duration: number
}

export const toasts = ref<ToastItem[]>([])

let toastId = 0

export const showToast = (message: string, type: ToastType = 'success', duration = 3200) => {
  const id = ++toastId
  toasts.value.push({ id, message, type, duration })

  setTimeout(() => {
    removeToast(id)
  }, duration)
}

export const removeToast = (id: number) => {
  toasts.value = toasts.value.filter((t) => t.id !== id)
}

export const toast = {
  success: (msg: string, duration?: number) => showToast(msg, 'success', duration),
  error: (msg: string, duration?: number) => showToast(msg, 'error', duration || 4500),
  info: (msg: string, duration?: number) => showToast(msg, 'info', duration),
  warning: (msg: string, duration?: number) => showToast(msg, 'warning', duration || 4000),
}
