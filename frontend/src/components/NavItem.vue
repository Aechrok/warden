<template>
  <RouterLink
    :to="item.to"
    class="flex items-center gap-3 mx-2 px-3 py-2 rounded-lg text-sm font-medium transition-colors relative"
    :style="isActive
      ? 'background: var(--nav-active-bg); color: var(--nav-active-text);'
      : 'color: var(--text-muted);'"
    :title="collapsed ? item.label : undefined"
  >
    <span class="text-base flex-shrink-0">{{ item.icon }}</span>
    <span v-if="!collapsed" class="flex-1 truncate">{{ item.label }}</span>
    <!-- Approval badge -->
    <span
      v-if="item.badge && !collapsed && approvalsStore.pendingCount > 0"
      class="bg-red-500 text-white text-[10px] font-bold rounded-full min-w-[18px] h-[18px] flex items-center justify-center px-1"
    >
      {{ approvalsStore.pendingCount > 99 ? '99+' : approvalsStore.pendingCount }}
    </span>
    <span
      v-if="item.badge && collapsed && approvalsStore.pendingCount > 0"
      class="absolute top-1 right-1 bg-red-500 text-white text-[9px] font-bold rounded-full min-w-[14px] h-[14px] flex items-center justify-center px-0.5"
    >
      {{ approvalsStore.pendingCount }}
    </span>
  </RouterLink>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { useApprovalsStore } from '../stores/approvals'

const props = defineProps<{
  item: {
    to: string
    label: string
    icon: string
    badge?: boolean
  }
  collapsed: boolean
}>()

const route = useRoute()
const approvalsStore = useApprovalsStore()

const isActive = computed(() => {
  if (props.item.to === '/') return route.path === '/'
  return route.path === props.item.to || route.path.startsWith(props.item.to + '/')
})
</script>
