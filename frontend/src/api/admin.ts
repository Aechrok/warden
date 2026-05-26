import { apiFetch } from './client'
import type { Instance, Role, PBACPolicy, HoldTemplate, VIPIdentity } from './types'

// --- Instances ---

export async function listInstances(): Promise<Instance[]> {
  const res = await apiFetch<{ instances: Instance[] }>('/api/v1/internal/admin/instances')
  return res.instances ?? []
}

export async function createInstance(data: Partial<Instance>): Promise<Instance> {
  const res = await apiFetch<{ instance: Instance }>('/api/v1/internal/admin/instances', {
    method: 'POST',
    body: JSON.stringify(data),
  })
  return res.instance
}

export async function updateInstance(id: string, data: Partial<Instance>): Promise<Instance> {
  const res = await apiFetch<{ instance: Instance }>(`/api/v1/internal/admin/instances/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  })
  return res.instance
}

export async function deleteInstance(id: string): Promise<void> {
  await apiFetch<void>(`/api/v1/internal/admin/instances/${id}`, { method: 'DELETE' })
}

// --- Roles ---

export async function listRoles(): Promise<Role[]> {
  const res = await apiFetch<{ roles: Role[] }>('/api/v1/internal/admin/roles')
  return res.roles ?? []
}

export async function assignRole(roleName: string, userId: string): Promise<void> {
  await apiFetch<void>(`/api/v1/internal/admin/roles/${roleName}/assign`, {
    method: 'POST',
    body: JSON.stringify({ user_id: userId }),
  })
}

export async function revokeRole(roleName: string, userId: string): Promise<void> {
  await apiFetch<void>(`/api/v1/internal/admin/roles/${roleName}/users/${userId}`, {
    method: 'DELETE',
  })
}

// --- PBAC Policies ---

export async function listPBACPolicies(): Promise<PBACPolicy[]> {
  const res = await apiFetch<{ policies: PBACPolicy[] }>('/api/v1/internal/admin/pbac')
  return res.policies ?? []
}

export async function updatePBACPolicy(name: string, data: Partial<PBACPolicy>): Promise<PBACPolicy> {
  const res = await apiFetch<{ policy: PBACPolicy }>(`/api/v1/internal/admin/pbac/${name}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  })
  return res.policy
}

// --- Hold Templates ---

export async function listHoldTemplates(): Promise<HoldTemplate[]> {
  const res = await apiFetch<{ templates: HoldTemplate[] }>('/api/v1/internal/admin/hold-templates')
  return res.templates ?? []
}

export async function createHoldTemplate(data: Partial<HoldTemplate>): Promise<HoldTemplate> {
  const res = await apiFetch<{ template: HoldTemplate }>('/api/v1/internal/admin/hold-templates', {
    method: 'POST',
    body: JSON.stringify(data),
  })
  return res.template
}

export async function updateHoldTemplate(id: string, data: Partial<HoldTemplate>): Promise<HoldTemplate> {
  const res = await apiFetch<{ template: HoldTemplate }>(`/api/v1/internal/admin/hold-templates/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  })
  return res.template
}

export async function deleteHoldTemplate(id: string): Promise<void> {
  await apiFetch<void>(`/api/v1/internal/admin/hold-templates/${id}`, { method: 'DELETE' })
}

// --- VIP Identities ---

export async function listVIPIdentities(): Promise<VIPIdentity[]> {
  const res = await apiFetch<{ identities: VIPIdentity[] }>('/api/v1/internal/admin/vip')
  return res.identities ?? []
}

export async function addVIPIdentity(email: string, reason: string): Promise<VIPIdentity> {
  const res = await apiFetch<{ identity: VIPIdentity }>('/api/v1/internal/admin/vip', {
    method: 'POST',
    body: JSON.stringify({ email, reason }),
  })
  return res.identity
}

export async function removeVIPIdentity(id: string): Promise<void> {
  await apiFetch<void>(`/api/v1/internal/admin/vip/${id}`, { method: 'DELETE' })
}
