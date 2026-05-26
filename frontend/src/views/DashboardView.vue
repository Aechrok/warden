<template>
  <div>
    <h1 class="text-2xl font-bold mb-6" style="color: var(--text-primary);">Dashboard</h1>

    <!-- Stat cards -->
    <div class="grid grid-cols-2 md:grid-cols-4 gap-4 mb-8">
      <StatCard label="Active Holds" :value="stats.activeHolds" icon="⊞" color="indigo" />
      <StatCard label="Pending Approvals" :value="stats.pendingApprovals" icon="✓" color="yellow" />
      <StatCard label="Incidents Today" :value="stats.incidentsToday" icon="⚡" color="red" />
      <StatCard label="Active Integrations" :value="stats.activeIntegrations" icon="◈" color="green" />
    </div>

    <div class="grid md:grid-cols-2 gap-6">
      <!-- Live audit stream -->
      <div
        class="rounded-xl p-4"
        style="background: var(--card); border: 1px solid var(--border);"
      >
        <div class="flex items-center justify-between mb-3">
          <h2 class="font-semibold text-sm" style="color: var(--text-primary);">Live Audit Events</h2>
          <span class="flex items-center gap-1 text-xs" style="color: var(--text-muted);">
            <span
              class="w-2 h-2 rounded-full inline-block"
              :class="polling ? 'bg-green-500 animate-pulse' : 'bg-slate-400'"
            ></span>
            {{ polling ? 'Live' : 'Paused' }}
          </span>
        </div>

        <div class="space-y-1 overflow-y-auto" style="max-height: 320px;">
          <div v-if="auditEvents.length === 0 && !auditLoading" class="py-6 text-center text-sm" style="color: var(--text-muted);">
            No events yet
          </div>
          <div v-if="auditLoading && auditEvents.length === 0" class="py-6 text-center">
            <div class="animate-spin w-5 h-5 rounded-full border-2 border-indigo-500 border-t-transparent mx-auto"></div>
          </div>
          <AuditEventRow
            v-for="event in auditEvents"
            :key="event.id"
            :event="event"
          />
        </div>
      </div>

      <!-- Pending approvals widget -->
      <div
        class="rounded-xl p-4"
        style="background: var(--card); border: 1px solid var(--border);"
      >
        <div class="flex items-center justify-between mb-3">
          <h2 class="font-semibold text-sm" style="color: var(--text-primary);">Pending Approvals</h2>
          <RouterLink to="/approvals" class="text-xs text-indigo-500 hover:underline">View all</RouterLink>
        </div>

        <div v-if="approvalsStore.pendingApprovals.length === 0" class="py-6 text-center text-sm" style="color: var(--text-muted);">
          No pending approvals
        </div>

        <div class="space-y-2">
          <div
            v-for="approval in approvalsStore.pendingApprovals.slice(0, 3)"
            :key="approval.id"
            class="rounded-lg p-3"
            style="background: var(--surface); border: 1px solid var(--border);"
          >
            <div class="flex items-start justify-between gap-2">
              <div class="min-w-0">
                <div class="text-sm font-medium truncate" style="color: var(--text-primary);">
                  {{ approval.action_key }}
                </div>
                <div class="text-xs" style="color: var(--text-muted);">
                  {{ approval.target_email }} · by {{ approval.requested_by }}
                </div>
              </div>
              <RouterLink
                to="/approvals"
                class="text-xs px-2 py-1 rounded bg-indigo-500 text-white hover:bg-indigo-600 flex-shrink-0"
              >
                Review
              </RouterLink>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, onBeforeUnmount } from 'vue'
import { RouterLink } from 'vue-router'
import { queryAuditEvents } from '../api/audit'
import { listHolds } from '../api/holds'
import { listInstances } from '../api/admin'
import { listIncidents } from '../api/breakglass'
import { useApprovalsStore } from '../stores/approvals'
import type { AuditEvent } from '../api/types'
import StatCard from '../components/StatCard.vue'
import AuditEventRow from '../components/AuditEventRow.vue'

const approvalsStore = useApprovalsStore()

const auditEvents = ref<AuditEvent[]>([])
const auditLoading = ref(false)
const polling = ref(false)

const stats = reactive({
  activeHolds: 0,
  pendingApprovals: 0,
  incidentsToday: 0,
  activeIntegrations: 0,
})

let pollTimer: ReturnType<typeof setInterval> | undefined

async function loadAuditEvents() {
  try {
    auditEvents.value = await queryAuditEvents({ limit: 20 })
    polling.value = true
  } catch {
    polling.value = false
  }
}

async function loadStats() {
  try {
    const [holds, instances, incidents] = await Promise.allSettled([
      listHolds(),
      listInstances(),
      listIncidents(),
    ])

    if (holds.status === 'fulfilled') {
      stats.activeHolds = holds.value.filter((h) => h.status === 'active').length
    }
    if (instances.status === 'fulfilled') {
      stats.activeIntegrations = instances.value.filter((i) => i.enabled).length
    }
    if (incidents.status === 'fulfilled') {
      const today = new Date().toDateString()
      stats.incidentsToday = incidents.value.filter(
        (i) => new Date(i.invoked_at).toDateString() === today
      ).length
    }

    stats.pendingApprovals = approvalsStore.pendingCount
  } catch {
    // non-critical
  }
}

onMounted(async () => {
  auditLoading.value = true
  await Promise.all([loadAuditEvents(), loadStats(), approvalsStore.fetchApprovals()])
  auditLoading.value = false
  pollTimer = setInterval(loadAuditEvents, 10000)
})

onBeforeUnmount(() => {
  if (pollTimer) clearInterval(pollTimer)
})
</script>
