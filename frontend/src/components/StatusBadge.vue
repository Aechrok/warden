<template>
  <span
    class="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium"
    :class="badgeClass"
  >
    {{ label ?? status }}
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  status: string
  label?: string
}>()

const badgeClass = computed(() => {
  switch (props.status) {
    case 'active':
    case 'placed':
    case 'approved':
    case 'reviewed':
      return 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400'
    case 'pending':
      return 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400'
    case 'released':
    case 'inactive':
    case 'suspended':
      return 'bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-400'
    case 'expired':
    case 'failed':
    case 'rejected':
      return 'bg-orange-100 text-orange-800 dark:bg-orange-900/30 dark:text-orange-400'
    default:
      return 'bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-400'
  }
})
</script>
