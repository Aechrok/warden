// --- Auth / User ---

export interface User {
  id: string
  email: string
  name: string
  is_active: boolean
}

export interface MeResponse {
  user: User
  permissions: string[]
  roles: string[]
}

// --- Identity ---

export interface Identity {
  id: string
  email: string
  display_name: string
  instance_id: string
  provider: string
  status: string
  raw?: Record<string, unknown>
}

export interface IdentitySearchResponse {
  identities: Identity[]
}

// --- Actions ---

export interface ActionDef {
  key: string
  label: string
  description: string
  instance_id: string
  plugin: string
  destructive: boolean
  params?: ActionParam[]
}

export interface ActionParam {
  key: string
  label: string
  type: 'string' | 'boolean' | 'select'
  required: boolean
  options?: string[]
}

export interface ActionExecuteRequest {
  instance_id: string
  action_key: string
  target_email: string
  params?: Record<string, unknown>
}

export interface ActionExecuteResponse {
  result?: string
  status?: 'pending_approval'
  approval_id?: string
}

// --- Holds ---

export interface Hold {
  id: string
  name: string
  description: string
  status: 'active' | 'released' | 'expired'
  template_id?: string
  placed_by: string
  created_at: string
  released_at?: string
  expires_at?: string
}

export interface Custodian {
  id: string
  hold_id: string
  email: string
  added_at: string
  added_by: string
}

export interface CascadeState {
  id: string
  hold_id: string
  custodian_id: string
  provider: string
  instance_id: string
  status: 'pending' | 'placed' | 'failed' | 'released'
  last_updated: string
  error?: string
}

export interface HoldDetailResponse {
  hold: Hold
  custodians: Custodian[]
  cascade_states: CascadeState[]
}

export interface HoldTemplate {
  id: string
  name: string
  description: string
  provider_glob: string
  default_expiry_days?: number
  created_at: string
}

// --- Audit ---

export interface AuditEvent {
  id: string
  aggregate_type: string
  aggregate_id: string
  event_type: string
  actor_email: string
  actor_id: string
  occurred_at: string
  payload?: Record<string, unknown>
}

export interface AuditQueryParams {
  aggregate_type?: string
  aggregate_id?: string
  since?: string
  limit?: number
}

// --- Approvals ---

export interface Approval {
  id: string
  action_key: string
  instance_id: string
  target_email: string
  requested_by: string
  requested_at: string
  reason?: string
  status: 'pending' | 'approved' | 'rejected'
  reviewed_by?: string
  reviewed_at?: string
  note?: string
}

// --- Break-glass ---

export interface BreakGlassInvokeRequest {
  action_key: string
  instance_id: string
  target_email: string
  reason: string
}

export interface Incident {
  id: string
  action_key: string
  instance_id: string
  target_email: string
  operator_id: string
  operator_email: string
  reason: string
  invoked_at: string
  review_status: 'pending' | 'reviewed'
  review_note?: string
  reviewed_by?: string
  reviewed_at?: string
}

// --- Tokens ---

export interface Token {
  id: string
  name: string
  scopes: string[]
  created_at: string
  expires_at?: string
  last_used_at?: string
  token?: string // only present on creation
}

export interface CreateTokenRequest {
  name: string
  scopes: string[]
  expires_at?: string
}

// --- Admin ---

export interface Instance {
  id: string
  name: string
  plugin: string
  enabled: boolean
  created_at: string
}

export interface Role {
  name: string
  description: string
  permissions: string[]
  users?: RoleUser[]
}

export interface RoleUser {
  id: string
  email: string
  name: string
}

export interface PBACPolicy {
  name: string
  description: string
  enabled: boolean
  config: Record<string, unknown>
}

export interface VIPIdentity {
  id: string
  email: string
  reason: string
  created_at: string
}
