<template>
  <div class="flex items-start gap-3 py-2 text-xs" style="border-bottom: 1px solid var(--border);">
    <span class="flex-shrink-0 mt-0.5 font-mono" style="color: var(--text-muted);">
      {{ formatTime(event.occurred_at) }}
    </span>
    <div class="flex-1 min-w-0">
      <span
        class="inline-block px-1.5 py-0.5 rounded text-[10px] font-medium mr-1 mb-0.5"
        style="background: var(--nav-active-bg); color: var(--nav-active-text);"
      >
        {{ event.aggregate_type }}
      </span>
      <span style="color: var(--text-primary);">{{ event.event_type }}</span>
      <span v-if="event.actor_email" class="ml-1" style="color: var(--text-muted);">
        by {{ event.actor_email }}
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { AuditEvent } from '../api/types'

defineProps<{ event: AuditEvent }>()

function formatTime(ts: string): string {
  const d = new Date(ts)
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}
</script>
