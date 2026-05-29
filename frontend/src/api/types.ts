// --- Auth / User ---

export interface User {
  id: string
  email: string
  name: string
  is_active: boolean
  origin: string
}

export interface MeResponse {
  user: User
  permissions: string[]
  roles: string[]
}

// --- Identity ---

export interface Identity {
  email: string
  display_name: string
  instance_id: string
  instance_name: string
  data?: Record<string, unknown>
  fetched_at?: string
}

export interface IdentitySearchResponse {
  identities: Identity[]
  on_hold: boolean
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
  applicable_states?: string[]
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
  placed_by?: string
  created_at: string
  released_at?: string
  expires_at?: string
}

export interface Custodian {
  id: string
  hold_id: string
  email: string
  added_by?: string
  created_at: string
}

export type CascadeStatus = 'pending' | 'in_progress' | 'completed' | 'partial' | 'failed'

export interface CascadeState {
  id: string
  hold_id: string
  custodian_email: string
  instance_id: string
  status: CascadeStatus
  last_error?: string
  attempts: number
  completed_at?: string
  created_at: string
  updated_at: string
}

export interface HoldEnriched extends Hold {
  placed_by_name?: string
  custodians: Custodian[]
  cascade_states: CascadeState[]
}

export interface HoldDetailResponse {
  hold: Hold
  placed_by_name?: string
  custodians: Custodian[]
  cascade_states: CascadeState[]
}

export interface HoldTemplate {
  id: string
  name: string
  description: string
  provider_glob: string
  expiration_days?: number
  is_default: boolean
  created_at: string
}

// --- Audit ---

export interface AuditEvent {
  id: string
  aggregate_type: string
  aggregate_id: string
  version: number
  type: string
  actor_id?: string
  actor_type?: string
  actor_display?: string
  created_at: string
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
  plugin_id: string
  is_active: boolean
  created_at: string
}

export interface CredentialField {
  key: string
  label: string
  type: 'string' | 'json' | 'bool'
  required: boolean
  secret: boolean
  description?: string
}

export interface PluginSchema {
  id: string
  name: string
  schema: CredentialField[]
}

export interface Role {
  id: string
  name: string
  description: string
  is_builtin: boolean
  permissions: string[]
}

export interface UserWithRoles {
  id: string
  email: string
  name: string
  is_active: boolean
  origin: 'oidc' | 'scim'
  roles: string[]
  created_at: string
}

export interface RoleUser {
  id: string
  email: string
  name: string
}

export interface PBACPolicy {
  id: string
  name: string
  policy_type: string
  is_enabled: boolean
  config: Record<string, unknown>
  created_at: string
  updated_at: string
}

export interface SCIMGroup {
  id: string
  external_id: string
  name: string
  role_id?: string
  role_name?: string
  created_at: string
  updated_at: string
}

export interface VIPIdentity {
  id: string
  email: string
  reason: string
  created_at: string
}

export interface SSOConfig {
  oidc_issuer: string
  oidc_internal_issuer: string
  oidc_client_id: string
  has_secret: boolean
  oidc_redirect_url: string
  sso_enabled: boolean
  enforce_sso: boolean
  updated_at: string
}

export interface AuthConfig {
  sso_enabled: boolean
  enforce_sso: boolean
}

export interface Invitation {
  id: string
  token: string
  email: string
  role_name?: string
  label: string
  used_at?: string
  expires_at: string
  created_at: string
  invited_by_email?: string
}
