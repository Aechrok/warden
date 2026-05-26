<template>
  <div
    v-if="message"
    class="flex items-center gap-3 px-4 py-3 rounded-lg text-sm"
    :class="bannerClass"
  >
    <span class="font-bold">{{ icon }}</span>
    <span>{{ message }}</span>
    <button
      v-if="dismissible"
      class="ml-auto text-current opacity-60 hover:opacity-100"
      @click="$emit('dismiss')"
    >
      ✕
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  message?: string | null
  type?: 'error' | 'warning' | 'info'
  dismissible?: boolean
}>()

defineEmits<{ (e: 'dismiss'): void }>()

const bannerClass = computed(() => {
  switch (props.type) {
    case 'warning': return 'bg-yellow-50 text-yellow-800 border border-yellow-200'
    case 'info':    return 'bg-blue-50 text-blue-800 border border-blue-200'
    default:        return 'bg-red-50 text-red-800 border border-red-200'
  }
})

const icon = computed(() => {
  switch (props.type) {
    case 'warning': return '!'
    case 'info':    return 'i'
    default:        return '✕'
  }
})
</script>
