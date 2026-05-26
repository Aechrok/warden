<template>
  <div
    class="rounded-xl p-4"
    style="background: var(--card); border: 1px solid var(--border);"
  >
    <div class="flex items-center justify-between mb-2">
      <span class="text-lg">{{ icon }}</span>
      <span
        class="w-2 h-2 rounded-full"
        :class="`bg-${color}-500`"
      ></span>
    </div>
    <div class="text-2xl font-bold" style="color: var(--text-primary);">{{ value }}</div>
    <div class="text-xs mt-1" style="color: var(--text-muted);">{{ label }}</div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  label: string
  value: number
  icon: string
  color: string
}>()
</script>
