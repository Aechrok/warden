<template>
  <!-- completed -->
  <svg v-if="status === 'completed'" class="w-4 h-4" viewBox="0 0 16 16" fill="none">
    <circle cx="8" cy="8" r="7" fill="#22c55e" fill-opacity="0.15" stroke="#22c55e" stroke-width="1.5"/>
    <path d="M5 8l2 2 4-4" stroke="#22c55e" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
  </svg>

  <!-- failed -->
  <svg v-else-if="status === 'failed'" class="w-4 h-4" viewBox="0 0 16 16" fill="none">
    <circle cx="8" cy="8" r="7" fill="#ef4444" fill-opacity="0.15" stroke="#ef4444" stroke-width="1.5"/>
    <path d="M5.5 5.5l5 5M10.5 5.5l-5 5" stroke="#ef4444" stroke-width="1.5" stroke-linecap="round"/>
  </svg>

  <!-- partial -->
  <svg v-else-if="status === 'partial'" class="w-4 h-4" viewBox="0 0 16 16" fill="none">
    <circle cx="8" cy="8" r="7" fill="#f97316" fill-opacity="0.15" stroke="#f97316" stroke-width="1.5"/>
    <path d="M8 5v3.5M8 10.5v.5" stroke="#f97316" stroke-width="1.5" stroke-linecap="round"/>
  </svg>

  <!-- in_progress — spinning -->
  <svg v-else-if="status === 'in_progress'" class="w-4 h-4 animate-spin" viewBox="0 0 16 16" fill="none">
    <circle cx="8" cy="8" r="6" stroke="#6366f1" stroke-width="1.5" stroke-opacity="0.25"/>
    <path d="M8 2a6 6 0 0 1 6 6" stroke="#6366f1" stroke-width="1.5" stroke-linecap="round"/>
  </svg>

  <!-- pending -->
  <svg v-else-if="status === 'pending'" class="w-4 h-4" viewBox="0 0 16 16" fill="none">
    <circle cx="8" cy="8" r="7" fill="#eab308" fill-opacity="0.15" stroke="#eab308" stroke-width="1.5"/>
    <path d="M8 5v3l1.5 1.5" stroke="#eab308" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
  </svg>

  <!-- none / unknown -->
  <svg v-else class="w-4 h-4" viewBox="0 0 16 16" fill="none">
    <circle cx="8" cy="8" r="7" stroke="currentColor" stroke-width="1.5" stroke-opacity="0.3"/>
    <circle cx="8" cy="8" r="1.5" fill="currentColor" fill-opacity="0.3"/>
  </svg>
</template>

<script setup lang="ts">
import type { CascadeStatus } from '../api/types'

defineProps<{
  status: CascadeStatus | 'none'
}>()
</script>
