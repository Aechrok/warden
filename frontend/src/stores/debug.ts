import { ref, computed } from 'vue'
import { defineStore } from 'pinia'

export interface ApiCall {
  method: string
  path: string
  status: number
  durationMs: number
  at: string
}

export const useDebugStore = defineStore('debug', () => {
  const calls = ref<ApiCall[]>([])
  const totalCalls = ref(0)
  const totalMs = ref(0)

  const avgMs = computed(() =>
    totalCalls.value > 0 ? Math.round((totalMs.value / totalCalls.value) * 10) / 10 : 0
  )

  function record(call: ApiCall) {
    calls.value.unshift(call)
    if (calls.value.length > 200) calls.value.length = 200
    totalCalls.value++
    totalMs.value += call.durationMs
  }

  function clear() {
    calls.value = []
    totalCalls.value = 0
    totalMs.value = 0
  }

  return { calls, totalCalls, avgMs, record, clear }
})
