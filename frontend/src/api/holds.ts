import { apiFetch } from './client'
import type { Hold, HoldDetailResponse } from './types'

export async function listHolds(): Promise<Hold[]> {
  const res = await apiFetch<{ holds: Hold[] }>('/api/v1/internal/holds/')
  return res.holds ?? []
}

export async function createHold(data: {
  name: string
  description: string
  template_id?: string
  expires_at?: string
}): Promise<Hold> {
  const res = await apiFetch<{ hold: Hold }>('/api/v1/internal/holds/', {
    method: 'POST',
    body: JSON.stringify(data),
  })
  return res.hold
}

export async function getHold(id: string): Promise<HoldDetailResponse> {
  return apiFetch<HoldDetailResponse>(`/api/v1/internal/holds/${id}`)
}

export async function addCustodian(holdId: string, email: string): Promise<void> {
  await apiFetch<void>(`/api/v1/internal/holds/${holdId}/custodians`, {
    method: 'POST',
    body: JSON.stringify({ email }),
  })
}

export async function removeCustodian(holdId: string, custodianId: string): Promise<void> {
  await apiFetch<void>(`/api/v1/internal/holds/${holdId}/custodians/${custodianId}`, {
    method: 'DELETE',
  })
}

export async function releaseHold(holdId: string, reason: string): Promise<void> {
  await apiFetch<void>(`/api/v1/internal/holds/${holdId}/release`, {
    method: 'POST',
    body: JSON.stringify({ reason }),
  })
}
