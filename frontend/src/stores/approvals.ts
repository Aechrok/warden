import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { listApprovals } from '../api/approvals'
import type { Approval } from '../api/types'

export const useApprovalsStore = defineStore('approvals', () => {
  const approvals = ref<Approval[]>([])
  const loading = ref(false)

  const pendingCount = computed(
    () => approvals.value.filter((a) => a.status === 'pending').length
  )

  const pendingApprovals = computed(
    () => approvals.value.filter((a) => a.status === 'pending')
  )

  async function fetchApprovals() {
    loading.value = true
    try {
      approvals.value = await listApprovals()
    } catch {
      // silently fail for badge polling
    } finally {
      loading.value = false
    }
  }

  return { approvals, loading, pendingCount, pendingApprovals, fetchApprovals }
})
