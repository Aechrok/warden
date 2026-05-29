<template>
  <div class="min-h-screen flex" style="background: var(--surface);">
    <!-- Desktop sidebar -->
    <aside
      class="hidden md:flex flex-col fixed left-0 top-0 bottom-0 z-30 transition-all duration-200"
      :style="{
        width: sidebarCollapsed ? '64px' : '240px',
        background: 'var(--sidebar-bg)',
        borderRight: '1px solid var(--border)',
      }"
    >
      <!-- Logo -->
      <div class="flex items-center gap-3 px-4 py-5" style="border-bottom: 1px solid var(--border);">
        <div class="w-8 h-8 rounded-lg flex items-center justify-center flex-shrink-0 bg-indigo-500 text-white font-bold text-sm">W</div>
        <span v-if="!sidebarCollapsed" class="font-semibold text-sm" style="color: var(--text-primary);">Warden</span>
        <button
          class="ml-auto opacity-50 hover:opacity-100 flex-shrink-0"
          style="color: var(--text-muted);"
          :title="sidebarCollapsed ? 'Expand' : 'Collapse'"
          @click="sidebarCollapsed = !sidebarCollapsed"
        >
          <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
            <path v-if="!sidebarCollapsed" d="M6 8l4-4v8z"/>
            <path v-else d="M10 8L6 4v8z"/>
          </svg>
        </button>
      </div>

      <!-- Nav items -->
      <nav class="flex-1 py-3 overflow-y-auto overflow-x-hidden">
        <NavItem
          v-for="item in navItems"
          :key="item.to"
          :item="item"
          :collapsed="sidebarCollapsed"
        />

        <!-- Divider + version below Settings (the last nav item) -->
        <div class="mx-3 mt-3 mb-1" style="height: 1px; background: var(--border);"></div>
        <div
          v-if="!sidebarCollapsed"
          class="px-5 py-1 text-xs select-none"
          style="color: var(--text-muted);"
        >v{{ appVersion }}</div>
      </nav>

      <!-- Footer: user info + sign out -->
      <div style="border-top: 1px solid var(--border);" class="p-3">
        <div v-if="authStore.user" class="flex items-center gap-3 px-3 py-2">
          <div
            class="w-7 h-7 rounded-full flex items-center justify-center text-xs font-bold text-white bg-indigo-500 flex-shrink-0"
          >
            {{ userInitial }}
          </div>
          <div v-if="!sidebarCollapsed" class="flex-1 min-w-0">
            <div class="text-xs font-medium truncate" style="color: var(--text-primary);">
              {{ authStore.user.name || authStore.user.email }}
            </div>
            <button
              class="text-xs hover:underline"
              style="color: var(--text-muted);"
              @click="handleSignOut"
            >
              Sign out
            </button>
          </div>
        </div>
      </div>
    </aside>

    <!-- Main content area -->
    <main
      class="flex-1 min-h-screen pb-16 md:pb-0"
      :style="{ marginLeft: isMd ? (sidebarCollapsed ? '64px' : '240px') : '0' }"
    >
      <!-- Mobile header -->
      <header
        class="md:hidden flex items-center justify-between px-4 py-3 sticky top-0 z-20"
        style="background: var(--card); border-bottom: 1px solid var(--border);"
      >
        <div class="flex items-center gap-2">
          <div class="w-7 h-7 rounded-lg bg-indigo-500 flex items-center justify-center text-white font-bold text-xs">W</div>
          <span class="font-semibold text-sm" style="color: var(--text-primary);">Warden</span>
        </div>
        <div class="flex items-center gap-1">
          <!-- DEV: debug icon -->
          <button
            v-if="isDev"
            class="flex items-center justify-center w-8 h-8 rounded-lg transition-colors"
            style="background: var(--border); color: var(--text-primary);"
            title="Debug panel"
            @click="debugOpen = true"
          >
            <svg width="15" height="15" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
              <rect x="2" y="3" width="12" height="10" rx="1.5"/>
              <path d="M5 7l2 2-2 2"/><line x1="8.5" y1="11" x2="11" y2="11"/>
            </svg>
          </button>
          <!-- Theme toggle -->
          <button
            class="flex items-center justify-center w-8 h-8 rounded-lg transition-colors"
            style="background: var(--border); color: var(--text-primary);"
            :title="themeStore.theme === 'dark' ? 'Switch to light' : 'Switch to dark'"
            @click="themeStore.toggle()"
          >
            <span class="text-base leading-none">{{ themeStore.theme === 'dark' ? '☀' : '☽' }}</span>
          </button>
        </div>
      </header>

      <!-- Desktop: top-right icon bar -->
      <div class="hidden md:flex items-center justify-end gap-1.5 px-6 py-3">
        <!-- DEV: debug icon -->
        <button
          v-if="isDev"
          class="flex items-center justify-center w-8 h-8 rounded-lg transition-colors"
          style="background: var(--border); color: var(--text-primary);"
          title="Debug panel"
          @click="debugOpen = true"
        >
          <svg width="15" height="15" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
            <rect x="2" y="3" width="12" height="10" rx="1.5"/>
            <path d="M5 7l2 2-2 2"/><line x1="8.5" y1="11" x2="11" y2="11"/>
          </svg>
        </button>
        <!-- Theme toggle (icon only) -->
        <button
          class="flex items-center justify-center w-8 h-8 rounded-lg transition-colors"
          style="background: var(--border); color: var(--text-primary);"
          :title="themeStore.theme === 'dark' ? 'Switch to light' : 'Switch to dark'"
          @click="themeStore.toggle()"
        >
          <span class="text-base leading-none">{{ themeStore.theme === 'dark' ? '☀' : '☽' }}</span>
        </button>
      </div>

      <div class="px-4 pb-4 md:px-6 md:pb-6 max-w-6xl mx-auto">
        <RouterView />
      </div>
    </main>

    <!-- Mobile bottom tab bar -->
    <nav
      class="md:hidden fixed bottom-0 left-0 right-0 z-30 flex items-stretch"
      style="background: var(--card); border-top: 1px solid var(--border); height: 56px;"
    >
      <RouterLink
        v-for="item in mobileNavItems"
        :key="item.to"
        :to="item.to"
        class="flex-1 flex flex-col items-center justify-center gap-0.5 text-xs transition-colors relative"
        :style="($route.path === item.to || $route.path.startsWith(item.to + '/') && item.to !== '/')
          ? 'color: var(--nav-active-text);'
          : 'color: var(--text-muted);'"
      >
        <span class="text-lg leading-none">{{ item.icon }}</span>
        <span class="text-[10px]">{{ item.label }}</span>
        <!-- Approval badge -->
        <span
          v-if="item.badge && approvalsStore.pendingCount > 0"
          class="absolute top-1 right-1/4 bg-red-500 text-white text-[9px] font-bold rounded-full min-w-[16px] h-4 flex items-center justify-center px-1"
        >
          {{ approvalsStore.pendingCount > 99 ? '99+' : approvalsStore.pendingCount }}
        </span>
      </RouterLink>
    </nav>

    <!-- Debug panel (DEV only — tree-shaken out of production builds) -->
    <Teleport to="body">
      <component
        :is="DebugPanel"
        v-if="isDev && debugOpen"
        @close="debugOpen = false"
      />
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, defineAsyncComponent, onMounted, onBeforeUnmount } from 'vue'
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import { useThemeStore } from '../stores/theme'
import { useApprovalsStore } from '../stores/approvals'
import NavItem from './NavItem.vue'

const authStore = useAuthStore()
const themeStore = useThemeStore()
const approvalsStore = useApprovalsStore()
const router = useRouter()

const sidebarCollapsed = ref(false)
const isMd = ref(window.innerWidth >= 768)
const debugOpen = ref(false)

// Compile-time constant — false in production, eliminating the async import.
const isDev = import.meta.env.DEV
const DebugPanel = isDev
  ? defineAsyncComponent(() => import('./DebugPanel.vue'))
  : null

const appVersion = __APP_VERSION__

const userInitial = computed(() => {
  const u = authStore.user
  if (!u) return '?'
  return (u.name || u.email)[0].toUpperCase()
})

interface NavItemDef {
  to: string
  label: string
  icon: string
  badge?: boolean
  permission?: string
}

const navItems = computed<NavItemDef[]>(() => [
  { to: '/', label: 'Dashboard', icon: '◈' },
  { to: '/identities', label: 'Identities', icon: '◉' },
  { to: '/devices', label: 'Devices', icon: '▣' },
  { to: '/holds', label: 'Legal Holds', icon: '⊞' },
  { to: '/approvals', label: 'Approvals', icon: '✓', badge: true },
  { to: '/audit', label: 'Audit Log', icon: '≡' },
  { to: '/breakglass', label: 'Break Glass', icon: '⚡' },
  { to: '/settings', label: 'Settings', icon: '⚙' },
])

const mobileNavItems = computed<NavItemDef[]>(() => [
  { to: '/', label: 'Home', icon: '◈' },
  { to: '/identities', label: 'Identities', icon: '◉' },
  { to: '/holds', label: 'Holds', icon: '⊞' },
  { to: '/approvals', label: 'Approvals', icon: '✓', badge: true },
  { to: '/settings', label: 'More', icon: '⋯' },
])

async function handleSignOut() {
  await authStore.logout()
  router.push({ name: 'login' })
}

function handleResize() {
  isMd.value = window.innerWidth >= 768
}

let pollInterval: ReturnType<typeof setInterval> | undefined

onMounted(() => {
  approvalsStore.fetchApprovals()
  pollInterval = setInterval(() => approvalsStore.fetchApprovals(), 30000)
  window.addEventListener('resize', handleResize)
})

onBeforeUnmount(() => {
  if (pollInterval) clearInterval(pollInterval)
  window.removeEventListener('resize', handleResize)
})
</script>
