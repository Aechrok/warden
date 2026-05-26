import { defineStore } from 'pinia'
import { ref } from 'vue'

export type ToastType = 'success' | 'error' | 'info' | 'warning'

export interface Toast {
  id: string
  message: string
  type: ToastType
  duration?: number
}

export const useToastStore = defineStore('toast', () => {
  const toasts = ref<Toast[]>([])

  function add(message: string, type: ToastType = 'info', duration = 4000): string {
    const id = Math.random().toString(36).slice(2)
    toasts.value.push({ id, message, type, duration })
    if (duration > 0) {
      setTimeout(() => remove(id), duration)
    }
    return id
  }

  function remove(id: string) {
    toasts.value = toasts.value.filter((t) => t.id !== id)
  }

  function success(msg: string) { return add(msg, 'success') }
  function error(msg: string) { return add(msg, 'error') }
  function info(msg: string) { return add(msg, 'info') }
  function warning(msg: string) { return add(msg, 'warning') }

  return { toasts, add, remove, success, error, info, warning }
})
