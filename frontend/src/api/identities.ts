import { apiFetch } from './client'
import type { IdentitySearchResponse } from './types'

export async function searchIdentities(email: string, instanceId?: string): Promise<IdentitySearchResponse> {
  const params = new URLSearchParams({ email })
  if (instanceId) params.set('instance_id', instanceId)
  const res = await apiFetch<IdentitySearchResponse>(`/api/v1/internal/identities/search?${params}`)
  return { identities: res.identities ?? [], on_hold: res.on_hold ?? false }
}

export async function refreshIdentityCache(email: string, instanceId: string): Promise<void> {
  await apiFetch<void>('/api/v1/internal/identities/cache/refresh', {
    method: 'POST',
    body: JSON.stringify({ email, instance_id: instanceId }),
  })
}
