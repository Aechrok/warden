import { apiFetch } from './client'
import type { Approval } from './types'

export async function listApprovals(): Promise<Approval[]> {
  const res = await apiFetch<{ approvals: Approval[] }>('/api/v1/internal/approvals/')
  return res.approvals ?? []
}

export async function approveRequest(id: string, note: string): Promise<void> {
  await apiFetch<void>(`/api/v1/internal/approvals/${id}/approve`, {
    method: 'POST',
    body: JSON.stringify({ note }),
  })
}

export async function rejectRequest(id: string, note: string): Promise<void> {
  await apiFetch<void>(`/api/v1/internal/approvals/${id}/reject`, {
    method: 'POST',
    body: JSON.stringify({ note }),
  })
}
