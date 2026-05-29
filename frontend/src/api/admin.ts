import { apiFetch } from './client'
import type { Instance, Role, UserWithRoles, PBACPolicy, HoldTemplate, VIPIdentity, PluginSchema, SCIMGroup, SSOConfig, Invitation } from './types'

// --- Plugins ---

export async function listPlugins(): Promise<PluginSchema[]> {
  const res = await apiFetch<{ plugins: PluginSchema[] }>('/api/v1/internal/admin/plugins')
  return res.plugins ?? []
}

// --- Instances ---

export async function listInstances(): Promise<Instance[]> {
  const res = await apiFetch<{ instances: Instance[] }>('/api/v1/internal/admin/instances')
  return res.instances ?? []
}

export async function createInstance(data: {
  name: string
  plugin_id: string
  credentials?: Record<string, string>
}): Promise<{ id: string }> {
  return apiFetch<{ id: string }>('/api/v1/internal/admin/instances', {
    method: 'POST',
    body: JSON.stringify(data),
  })
}

export async function updateInstance(id: string, data: {
  name?: string
  is_active?: boolean
  credentials?: Record<string, string>
}): Promise<void> {
  await apiFetch<void>(`/api/v1/internal/admin/instances/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  })
}

export async function deleteInstance(id: string): Promise<void> {
  await apiFetch<void>(`/api/v1/internal/admin/instances/${id}`, { method: 'DELETE' })
}

// --- Users ---

export async function listUsers(): Promise<UserWithRoles[]> {
  const res = await apiFetch<{ users: UserWithRoles[] }>('/api/v1/internal/admin/users')
  return res.users ?? []
}

// --- Roles ---

export async function listRoles(): Promise<Role[]> {
  const res = await apiFetch<{ roles: Role[] }>('/api/v1/internal/admin/roles')
  return res.roles ?? []
}

export async function listPermissions(): Promise<string[]> {
  const res = await apiFetch<{ permissions: string[] }>('/api/v1/internal/admin/permissions')
  return res.permissions ?? []
}

export async function createRole(data: { name: string; description: string; permissions: string[] }): Promise<{ id: string }> {
  return apiFetch<{ id: string }>('/api/v1/internal/admin/roles', {
    method: 'POST',
    body: JSON.stringify(data),
  })
}

export async function updateRolePermissions(roleName: string, permissions: string[]): Promise<void> {
  await apiFetch<void>(`/api/v1/internal/admin/roles/${roleName}/permissions`, {
    method: 'PUT',
    body: JSON.stringify({ permissions }),
  })
}

export async function deleteRole(roleName: string): Promise<void> {
  await apiFetch<void>(`/api/v1/internal/admin/roles/${roleName}`, { method: 'DELETE' })
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

export async function updatePBACPolicy(name: string, data: { is_enabled?: boolean; config?: Record<string, unknown> }): Promise<void> {
  await apiFetch<void>(`/api/v1/internal/admin/pbac/${name}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  })
}

// --- Hold Templates ---

export async function listHoldTemplates(): Promise<HoldTemplate[]> {
  const res = await apiFetch<{ templates: HoldTemplate[] }>('/api/v1/internal/admin/hold-templates')
  return res.templates ?? []
}

export interface HoldTemplateInput {
  name: string
  description?: string
  provider_glob?: string
  expiration_days?: number
  is_default?: boolean
}

export async function createHoldTemplate(data: HoldTemplateInput): Promise<HoldTemplate> {
  const res = await apiFetch<{ template: HoldTemplate }>('/api/v1/internal/admin/hold-templates', {
    method: 'POST',
    body: JSON.stringify(data),
  })
  return res.template
}

export async function updateHoldTemplate(id: string, data: HoldTemplateInput): Promise<HoldTemplate> {
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
  const res = await apiFetch<{ vip_identities: VIPIdentity[] }>('/api/v1/internal/admin/vip')
  return res.vip_identities ?? []
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

// --- SCIM Groups ---

export async function listSCIMGroups(): Promise<SCIMGroup[]> {
  const res = await apiFetch<{ groups: SCIMGroup[] }>('/api/v1/internal/admin/scim-groups')
  return res.groups ?? []
}

export async function updateSCIMGroupRole(groupId: string, roleId: string | null): Promise<void> {
  await apiFetch<void>(`/api/v1/internal/admin/scim-groups/${groupId}/role`, {
    method: 'PUT',
    body: JSON.stringify({ role_id: roleId }),
  })
}

// --- SSO Config ---

export async function getSSOConfig(): Promise<SSOConfig> {
  return apiFetch<SSOConfig>('/api/v1/internal/admin/sso-config')
}

export async function updateSSOConfig(data: {
  oidc_issuer: string
  oidc_internal_issuer: string
  oidc_client_id: string
  oidc_client_secret?: string
  oidc_redirect_url: string
  sso_enabled: boolean
  enforce_sso: boolean
}): Promise<void> {
  await apiFetch<void>('/api/v1/internal/admin/sso-config', {
    method: 'PUT',
    body: JSON.stringify(data),
  })
}

// --- Users — password ---

export async function setUserPassword(userId: string, password: string): Promise<void> {
  await apiFetch<void>(`/api/v1/internal/admin/users/${userId}/password`, {
    method: 'PUT',
    body: JSON.stringify({ password }),
  })
}

// --- Invitations ---

export async function listInvitations(): Promise<Invitation[]> {
  const res = await apiFetch<{ invitations: Invitation[] }>('/api/v1/internal/admin/invitations')
  return res.invitations ?? []
}

export async function createInvitation(data: {
  email: string
  role_name?: string
  label?: string
  expiry_hours?: number
}): Promise<{ id: string; token: string }> {
  return apiFetch<{ id: string; token: string }>('/api/v1/internal/admin/invitations', {
    method: 'POST',
    body: JSON.stringify(data),
  })
}

export async function deleteInvitation(id: string): Promise<void> {
  await apiFetch<void>(`/api/v1/internal/admin/invitations/${id}`, { method: 'DELETE' })
}
