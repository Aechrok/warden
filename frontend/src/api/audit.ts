import { apiFetch, apiDownload } from './client'
import type { AuditEvent, AuditQueryParams } from './types'

export async function queryAuditEvents(params: AuditQueryParams = {}): Promise<AuditEvent[]> {
  const q = new URLSearchParams()
  if (params.aggregate_type) q.set('aggregate_type', params.aggregate_type)
  if (params.aggregate_id) q.set('aggregate_id', params.aggregate_id)
  if (params.since) q.set('since', params.since)
  if (params.limit !== undefined) q.set('limit', String(params.limit))
  const res = await apiFetch<{ events: AuditEvent[] }>(`/api/v1/internal/audit/events?${q}`)
  return res.events ?? []
}

export function exportAudit(format: 'json' | 'csv'): void {
  apiDownload(`/api/v1/internal/audit/export?format=${format}`)
}
