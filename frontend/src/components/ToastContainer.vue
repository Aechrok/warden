<template>
  <div class="fixed bottom-4 right-4 z-50 flex flex-col gap-2 pointer-events-none" style="max-width: 360px;">
    <TransitionGroup name="toast">
      <div
        v-for="toast in toastStore.toasts"
        :key="toast.id"
        class="pointer-events-auto flex items-start gap-3 px-4 py-3 rounded-lg shadow-lg text-sm font-medium"
        :class="toastClasses(toast.type)"
        @click="toastStore.remove(toast.id)"
      >
        <span class="flex-shrink-0 text-base">{{ toastIcon(toast.type) }}</span>
        <span>{{ toast.message }}</span>
      </div>
    </TransitionGroup>
  </div>
</template>

<script setup lang="ts">
import { useToastStore } from '../stores/toast'
import type { ToastType } from '../stores/toast'

const toastStore = useToastStore()

function toastClasses(type: ToastType): string {
  const base = 'cursor-pointer'
  switch (type) {
    case 'success': return `${base} bg-green-600 text-white`
    case 'error':   return `${base} bg-red-600 text-white`
    case 'warning': return `${base} bg-yellow-500 text-white`
    default:        return `${base} bg-slate-800 text-white`
  }
}

function toastIcon(type: ToastType): string {
  switch (type) {
    case 'success': return '✓'
    case 'error':   return '✕'
    case 'warning': return '!'
    default:        return 'i'
  }
}
</script>

<style scoped>
.toast-enter-active,
.toast-leave-active {
  transition: all 0.25s ease;
}
.toast-enter-from {
  opacity: 0;
  transform: translateX(40px);
}
.toast-leave-to {
  opacity: 0;
  transform: translateX(40px);
}
</style>
