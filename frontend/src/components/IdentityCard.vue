<template>
  <div
    class="rounded-xl p-4"
    style="background: var(--card); border: 1px solid var(--border);"
  >
    <div class="flex items-start justify-between gap-3">
      <div class="flex-1 min-w-0">
        <div class="flex items-center gap-2 flex-wrap">
          <span class="font-semibold text-sm" style="color: var(--text-primary);">
            {{ identity.display_name || identity.email }}
          </span>
          <StatusBadge :status="identity.status" />
          <span
            class="text-xs px-2 py-0.5 rounded-full"
            style="background: var(--nav-active-bg); color: var(--nav-active-text);"
          >
            {{ identity.provider }}
          </span>
        </div>
        <div class="text-xs mt-1" style="color: var(--text-muted);">{{ identity.email }}</div>
        <div class="text-xs" style="color: var(--text-muted);">Instance: {{ identity.instance_id }}</div>
      </div>

      <button
        class="flex-shrink-0 px-3 py-1.5 text-xs font-medium rounded-lg text-white bg-indigo-500 hover:bg-indigo-600"
        @click="showPanel = true"
      >
        Actions
      </button>
    </div>

    <!-- Desktop action panel -->
    <div
      v-if="showPanel && !isMobile"
      class="mt-4 pt-4"
      style="border-top: 1px solid var(--border);"
    >
      <ActionPanel
        :identity="identity"
        :actions="actions"
        @close="showPanel = false"
        @done="$emit('refresh')"
      />
    </div>

    <!-- Mobile action sheet -->
    <Teleport to="body">
      <div
        v-if="showPanel && isMobile"
        class="fixed inset-0 z-50 flex flex-col justify-end"
        style="background: rgba(0,0,0,0.5);"
        @click.self="showPanel = false"
      >
        <div
          class="rounded-t-2xl p-4 pb-8"
          style="background: var(--card);"
        >
          <div class="flex items-center justify-between mb-4">
            <h3 class="font-semibold" style="color: var(--text-primary);">
              Actions for {{ identity.email }}
            </h3>
            <button
              class="text-sm"
              style="color: var(--text-muted);"
              @click="showPanel = false"
            >
              ✕
            </button>
          </div>
          <ActionPanel
            :identity="identity"
            :actions="actions"
            @close="showPanel = false"
            @done="$emit('refresh'); showPanel = false"
          />
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount } from 'vue'
import type { Identity, ActionDef } from '../api/types'
import StatusBadge from './StatusBadge.vue'
import ActionPanel from './ActionPanel.vue'

const props = defineProps<{
  identity: Identity
  actions: ActionDef[]
}>()

defineEmits<{ (e: 'refresh'): void }>()

const showPanel = ref(false)
const isMobile = ref(window.innerWidth < 768)

function handleResize() {
  isMobile.value = window.innerWidth < 768
}

onMounted(() => window.addEventListener('resize', handleResize))
onBeforeUnmount(() => window.removeEventListener('resize', handleResize))
</script>
