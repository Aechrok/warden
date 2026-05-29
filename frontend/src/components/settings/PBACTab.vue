<template>
  <div>
    <div class="mb-6">
      <h2 class="font-semibold mb-1" style="color: var(--text-primary);">PBAC Policies</h2>
      <p class="text-sm" style="color: var(--text-muted);">
        Policy-based access control rules evaluated on every action. The engine applies
        the most restrictive result — Deny overrides RequireApproval, which overrides Allow.
        The only exception is Incident Window Expansion, which overrides all other policies.
      </p>
    </div>

    <ErrorBanner :message="error" class="mb-4" />

    <div v-if="loading" class="py-8 flex items-center justify-center">
      <div class="animate-spin w-5 h-5 rounded-full border-2 border-indigo-500 border-t-transparent"></div>
    </div>

    <!-- Category sections -->
    <div v-if="!loading" class="space-y-8">
      <div v-for="category in categories" :key="category.name">
        <div class="flex items-center gap-3 mb-3">
          <span class="text-xs font-semibold uppercase tracking-wider" style="color: var(--text-muted);">
            {{ category.label }}
          </span>
          <div class="flex-1 h-px" style="background: var(--border);"></div>
        </div>

        <div class="space-y-3">
          <div
            v-for="policyName in category.policyNames"
            :key="policyName"
            class="rounded-xl"
            style="background: var(--card); border: 1px solid var(--border);"
          >
            <!-- Policy header -->
            <div class="p-4">
              <div class="flex items-start justify-between gap-4">
                <div class="flex-1 min-w-0">
                  <div class="flex items-center gap-2 flex-wrap mb-1">
                    <span class="font-semibold text-sm" style="color: var(--text-primary);">
                      {{ meta[policyName]?.title ?? policyName }}
                    </span>
                    <!-- Effect badge -->
                    <span
                      class="px-2 py-0.5 rounded-full text-xs font-medium"
                      :style="effectStyle(meta[policyName]?.effect)"
                    >
                      {{ meta[policyName]?.effect ?? 'Unknown' }}
                    </span>
                    <!-- Enabled badge -->
                    <span
                      v-if="policyByName(policyName)?.is_enabled"
                      class="px-2 py-0.5 rounded-full text-xs font-medium"
                      style="background: #dcfce7; color: #15803d;"
                    >Active</span>
                    <span
                      v-else
                      class="px-2 py-0.5 rounded-full text-xs font-medium"
                      style="background: var(--surface); color: var(--text-muted); border: 1px solid var(--border);"
                    >Disabled</span>
                  </div>
                  <p class="text-xs leading-relaxed" style="color: var(--text-muted);">
                    {{ meta[policyName]?.description }}
                  </p>
                </div>

                <!-- Toggle button -->
                <button
                  class="flex-shrink-0 text-xs px-3 py-1.5 rounded-lg font-medium transition-colors"
                  :style="policyByName(policyName)?.is_enabled
                    ? 'background: var(--surface); color: var(--text-muted); border: 1px solid var(--border);'
                    : 'background: #dcfce7; color: #15803d; border: 1px solid #bbf7d0;'"
                  @click="togglePolicy(policyName)"
                >
                  {{ policyByName(policyName)?.is_enabled ? 'Disable' : 'Enable' }}
                </button>
              </div>
            </div>

            <!-- Config fields (only if policy has configurable fields) -->
            <div
              v-if="meta[policyName]?.fields?.length"
              class="px-4 pb-4 pt-0"
            >
              <div
                class="rounded-lg p-3"
                style="background: var(--surface); border: 1px solid var(--border);"
              >
                <div class="text-xs font-semibold mb-3" style="color: var(--text-muted);">Configuration</div>
                <div class="space-y-3">
                  <component
                    v-for="field in meta[policyName].fields"
                    :key="field.key"
                    :is="'div'"
                  >
                    <!-- Number input -->
                    <template v-if="field.type === 'number'">
                      <label class="block">
                        <span class="block text-xs font-medium mb-1" style="color: var(--text-primary);">{{ field.label }}</span>
                        <p v-if="field.help" class="text-xs mb-1.5" style="color: var(--text-muted);">{{ field.help }}</p>
                        <input
                          type="number"
                          :min="field.min"
                          :max="field.max"
                          :value="getDraft(policyName, field.key)"
                          @input="setDraft(policyName, field.key, Number(($event.target as HTMLInputElement).value))"
                          class="w-32 rounded-lg px-3 py-1.5 text-sm"
                          style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
                        />
                        <span v-if="field.unit" class="ml-2 text-xs" style="color: var(--text-muted);">{{ field.unit }}</span>
                      </label>
                    </template>

                    <!-- Boolean (checkbox) -->
                    <template v-else-if="field.type === 'boolean'">
                      <label class="flex items-start gap-2 cursor-pointer">
                        <input
                          type="checkbox"
                          :checked="!!getDraft(policyName, field.key)"
                          @change="setDraft(policyName, field.key, ($event.target as HTMLInputElement).checked)"
                          class="mt-0.5 rounded"
                          style="accent-color: var(--accent);"
                        />
                        <div>
                          <span class="block text-xs font-medium" style="color: var(--text-primary);">{{ field.label }}</span>
                          <p v-if="field.help" class="text-xs mt-0.5" style="color: var(--text-muted);">{{ field.help }}</p>
                        </div>
                      </label>
                    </template>

                    <!-- Text input (glob pattern, single string) -->
                    <template v-else-if="field.type === 'text'">
                      <label class="block">
                        <span class="block text-xs font-medium mb-1" style="color: var(--text-primary);">{{ field.label }}</span>
                        <p v-if="field.help" class="text-xs mb-1.5" style="color: var(--text-muted);">{{ field.help }}</p>
                        <input
                          type="text"
                          :placeholder="field.placeholder ?? ''"
                          :value="getDraft(policyName, field.key)"
                          @input="setDraft(policyName, field.key, ($event.target as HTMLInputElement).value)"
                          class="w-full rounded-lg px-3 py-1.5 text-sm font-mono"
                          style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
                        />
                      </label>
                    </template>

                    <!-- String list (CIDRs, regions, glob patterns) -->
                    <template v-else-if="field.type === 'stringlist'">
                      <div>
                        <span class="block text-xs font-medium mb-1" style="color: var(--text-primary);">{{ field.label }}</span>
                        <p v-if="field.help" class="text-xs mb-1.5" style="color: var(--text-muted);">{{ field.help }}</p>
                        <div class="space-y-1.5">
                          <div
                            v-for="(item, idx) in (getDraft(policyName, field.key) as string[] ?? [])"
                            :key="idx"
                            class="flex items-center gap-2"
                          >
                            <input
                              type="text"
                              :value="item"
                              :placeholder="field.placeholder ?? ''"
                              @input="updateListItem(policyName, field.key, idx, ($event.target as HTMLInputElement).value)"
                              class="flex-1 rounded-lg px-3 py-1.5 text-sm font-mono"
                              style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
                            />
                            <button
                              class="text-xs px-2 py-1.5 rounded-lg"
                              style="color: #ef4444; background: #fee2e2; border: 1px solid #fecaca;"
                              @click="removeListItem(policyName, field.key, idx)"
                            >Remove</button>
                          </div>
                          <button
                            class="text-xs px-3 py-1.5 rounded-lg"
                            style="background: var(--surface); border: 1px solid var(--border); color: var(--text-primary);"
                            @click="addListItem(policyName, field.key)"
                          >+ Add entry</button>
                        </div>
                      </div>
                    </template>

                    <!-- Day-of-week checkboxes -->
                    <template v-else-if="field.type === 'dayofweek'">
                      <div>
                        <span class="block text-xs font-medium mb-1.5" style="color: var(--text-primary);">{{ field.label }}</span>
                        <p v-if="field.help" class="text-xs mb-2" style="color: var(--text-muted);">{{ field.help }}</p>
                        <div class="flex gap-2 flex-wrap">
                          <label
                            v-for="(day, idx) in dayNames"
                            :key="idx"
                            class="flex items-center gap-1.5 text-xs cursor-pointer px-2 py-1 rounded-lg"
                            :style="isDaySelected(policyName, field.key, idx)
                              ? 'background: var(--nav-active-bg); color: var(--nav-active-text); border: 1px solid var(--accent);'
                              : 'background: var(--surface); color: var(--text-muted); border: 1px solid var(--border);'"
                          >
                            <input
                              type="checkbox"
                              :checked="isDaySelected(policyName, field.key, idx)"
                              @change="toggleDay(policyName, field.key, idx)"
                              class="sr-only"
                            />
                            {{ day }}
                          </label>
                        </div>
                      </div>
                    </template>

                    <!-- Hour range (time-of-day) -->
                    <template v-else-if="field.type === 'hourrange'">
                      <div>
                        <span class="block text-xs font-medium mb-1.5" style="color: var(--text-primary);">{{ field.label }}</span>
                        <p v-if="field.help" class="text-xs mb-2" style="color: var(--text-muted);">{{ field.help }}</p>
                        <div class="flex items-center gap-3 flex-wrap">
                          <div class="flex items-center gap-2">
                            <span class="text-xs" style="color: var(--text-muted);">From</span>
                            <select
                              :value="getDraft(policyName, 'start_hour')"
                              @change="setDraft(policyName, 'start_hour', Number(($event.target as HTMLSelectElement).value))"
                              class="rounded-lg px-3 py-1.5 text-sm"
                              style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
                            >
                              <option v-for="h in 24" :key="h-1" :value="h-1">{{ formatHour(h-1) }}</option>
                            </select>
                          </div>
                          <div class="flex items-center gap-2">
                            <span class="text-xs" style="color: var(--text-muted);">To</span>
                            <select
                              :value="getDraft(policyName, 'end_hour')"
                              @change="setDraft(policyName, 'end_hour', Number(($event.target as HTMLSelectElement).value))"
                              class="rounded-lg px-3 py-1.5 text-sm"
                              style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
                            >
                              <option v-for="h in 24" :key="h-1" :value="h-1">{{ formatHour(h-1) }}</option>
                            </select>
                          </div>
                          <span class="text-xs" style="color: var(--text-muted);">(24-hour, server local time)</span>
                        </div>
                      </div>
                    </template>
                  </component>
                </div>

                <!-- Save button — only visible when config is dirty -->
                <div class="mt-4 flex items-center gap-3">
                  <button
                    v-if="isDirty(policyName)"
                    class="text-xs px-4 py-1.5 rounded-lg font-medium text-white bg-indigo-500 hover:bg-indigo-600 transition-colors"
                    :disabled="saving === policyName"
                    @click="saveConfig(policyName)"
                  >
                    {{ saving === policyName ? 'Saving…' : 'Save changes' }}
                  </button>
                  <button
                    v-if="isDirty(policyName)"
                    class="text-xs px-3 py-1.5 rounded-lg transition-colors"
                    style="background: var(--surface); border: 1px solid var(--border); color: var(--text-muted);"
                    @click="discardDraft(policyName)"
                  >
                    Discard
                  </button>
                  <span v-if="!isDirty(policyName)" class="text-xs" style="color: var(--text-muted);">No unsaved changes</span>
                </div>
              </div>
            </div>
            <!-- No-config policies get a subtle note -->
            <div v-else class="px-4 pb-3 -mt-1">
              <p class="text-xs italic" style="color: var(--text-muted);">No configuration required — enable or disable to take effect.</p>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { listPBACPolicies, updatePBACPolicy } from '../../api/admin'
import type { PBACPolicy } from '../../api/types'
import { useToastStore } from '../../stores/toast'
import ErrorBanner from '../ErrorBanner.vue'

// ── Types ──────────────────────────────────────────────────────────────────

type FieldType = 'number' | 'boolean' | 'text' | 'stringlist' | 'dayofweek' | 'hourrange'

interface FieldDef {
  key: string
  label: string
  type: FieldType
  help?: string
  placeholder?: string
  unit?: string
  min?: number
  max?: number
}

type EffectLabel =
  | 'Blocks action'
  | 'Requires approval'
  | 'Overrides all policies'
  | 'Blocks or requires approval'

interface PolicyMeta {
  title: string
  description: string
  effect: EffectLabel
  fields: FieldDef[]
}

// ── Policy metadata ────────────────────────────────────────────────────────

const meta: Record<string, PolicyMeta> = {
  time_of_day: {
    title: 'Time-of-Day Gate',
    description:
      'Restricts actions to a configured hour window (server local time). Requests outside the window are either blocked outright or sent to the approval queue.',
    effect: 'Blocks or requires approval',
    fields: [
      {
        key: 'hourrange',
        label: 'Allowed window',
        type: 'hourrange',
        help: 'Actions are permitted between the start hour (inclusive) and end hour (exclusive). Windows that wrap midnight are supported — e.g. 22:00 → 06:00.',
      },
      {
        key: 'apply_to_destructive_only',
        label: 'Apply to destructive actions only',
        type: 'boolean',
        help: 'When checked, read-only or non-destructive actions are always allowed regardless of the hour.',
      },
      {
        key: 'require_approval_outside',
        label: 'Require approval instead of blocking',
        type: 'boolean',
        help: 'When checked, out-of-window requests are queued for approval rather than denied outright.',
      },
    ],
  },
  day_of_week: {
    title: 'Day-of-Week Restriction',
    description:
      'Limits action execution to specific days of the week. Requests on unlisted days are denied.',
    effect: 'Blocks action',
    fields: [
      {
        key: 'allowed_days',
        label: 'Allowed days',
        type: 'dayofweek',
        help: 'Select the days on which actions are permitted. Typical business-hours config: Monday–Friday.',
      },
    ],
  },
  change_freeze_window: {
    title: 'Change Freeze Window',
    description:
      'Denies actions that fall inside any configured freeze interval. Useful for release freezes, holiday lock-outs, or compliance blackout periods.',
    effect: 'Blocks action',
    fields: [
      {
        key: 'applies_to_destructive_only',
        label: 'Apply to destructive actions only',
        type: 'boolean',
        help: 'When checked, read-only actions proceed even during a freeze. When unchecked, all actions are blocked.',
      },
    ],
  },
  source_ip: {
    title: 'Source IP Allowlist',
    description:
      'Restricts actions to requests originating from an allowlisted set of IP ranges (CIDR notation). Requests from unknown IPs are blocked or queued.',
    effect: 'Blocks or requires approval',
    fields: [
      {
        key: 'allowed_cidrs',
        label: 'Allowed IP ranges (CIDR)',
        type: 'stringlist',
        placeholder: '10.0.0.0/8',
        help: 'IPv4 or IPv6 CIDR blocks. Example: 10.0.0.0/8, 192.168.1.0/24, 2001:db8::/32.',
      },
      {
        key: 'require_approval_on_violation',
        label: 'Require approval instead of blocking',
        type: 'boolean',
        help: 'When checked, off-network requests are sent to the approval queue rather than denied outright.',
      },
    ],
  },
  geographic_anomaly: {
    title: 'Geographic Anomaly Block',
    description:
      'Denies actions from geographic regions not in the allowlist. Region codes are matched against the GeoIP result for the operator\'s IP (e.g. "US-CA", "DE").',
    effect: 'Blocks action',
    fields: [
      {
        key: 'allowed_regions',
        label: 'Allowed regions',
        type: 'stringlist',
        placeholder: 'US-CA',
        help: 'Country or region codes as returned by your GeoIP provider. Leave empty to disable this policy entirely.',
      },
    ],
  },
  step_up_mfa: {
    title: 'Step-Up MFA',
    description:
      'Requires re-authentication (via the approval queue) before a destructive action if the operator\'s session is older than the configured threshold.',
    effect: 'Requires approval',
    fields: [
      {
        key: 'max_session_age_minutes',
        label: 'Max session age',
        type: 'number',
        min: 1,
        max: 10080,
        unit: 'minutes',
        help: 'If the operator\'s session is older than this many minutes when a destructive action is attempted, they are prompted to re-authenticate. Set to 0 to disable.',
      },
    ],
  },
  concurrent_session_limit: {
    title: 'Concurrent Session Limit',
    description:
      'Denies actions when the operator has more active sessions than the configured maximum. A spike in session count is a common indicator of credential compromise.',
    effect: 'Blocks action',
    fields: [
      {
        key: 'max_sessions',
        label: 'Maximum concurrent sessions',
        type: 'number',
        min: 1,
        max: 100,
        unit: 'sessions',
        help: 'Actions are denied when the operator\'s active session count exceeds this value. Set to 0 to disable.',
      },
    ],
  },
  vip_protection: {
    title: 'VIP Identity Protection',
    description:
      'Requires two-operator approval (via the approval queue) for any action targeting a VIP identity — C-suite, board members, or legal counsel. No configuration needed.',
    effect: 'Requires approval',
    fields: [],
  },
  self_targeting_block: {
    title: 'Self-Targeting Block',
    description:
      'Prevents an operator from performing actions on their own account. Guards against accidental self-lockout and social-engineering escalation attempts.',
    effect: 'Blocks action',
    fields: [],
  },
  bulk_action_threshold: {
    title: 'Bulk Action Threshold',
    description:
      'Denies an action when the same operator has already executed it more than the configured number of times within a rolling window — a rate-limiter for high-volume repetitive operations.',
    effect: 'Blocks action',
    fields: [
      {
        key: 'max_count',
        label: 'Maximum executions',
        type: 'number',
        min: 1,
        unit: 'times',
        help: 'Actions are denied once the operator has executed the same action more than this many times within the window.',
      },
      {
        key: 'window_minutes',
        label: 'Rolling window',
        type: 'number',
        min: 1,
        unit: 'minutes',
        help: 'The lookback period used by the action dispatcher when counting recent executions.',
      },
    ],
  },
  legal_hold_conflict: {
    title: 'Legal Hold Conflict Block',
    description:
      'Prevents releasing a legal hold on a custodian who is still referenced by another active hold. Ensures custodians are not inadvertently freed from preservation obligations.',
    effect: 'Blocks action',
    fields: [],
  },
  production_instance_gate: {
    title: 'Production Instance Gate',
    description:
      'Requires approval before executing destructive actions against instances whose name matches the production pattern. Keeps staging frictionless while raising the bar for production.',
    effect: 'Requires approval',
    fields: [
      {
        key: 'prod_pattern',
        label: 'Production instance glob',
        type: 'text',
        placeholder: '*-prod',
        help: 'A glob pattern (filepath.Match syntax) matched against the instance name. Default: *-prod. Examples: *-prod, prod-*, *production*.',
      },
    ],
  },
  integration_health_check: {
    title: 'Integration Health Gate',
    description:
      'Blocks destructive actions against an integration instance whose last health probe failed. Prevents partial-apply failures when the upstream API is degraded.',
    effect: 'Blocks action',
    fields: [],
  },
  new_operator_probation: {
    title: 'New Operator Probation',
    description:
      'Blocks destructive actions for operators whose accounts were created within the probation window. Gives administrators time to verify training, hardware, and account hygiene.',
    effect: 'Blocks action',
    fields: [
      {
        key: 'probation_days',
        label: 'Probation period',
        type: 'number',
        min: 1,
        unit: 'days',
        help: 'New operators cannot perform destructive actions until their account is at least this many days old. Set to 0 to disable.',
      },
    ],
  },
  on_call_verification: {
    title: 'On-Call Verification',
    description:
      'Denies high-impact actions for operators who are not on the current on-call roster. On-call status is sourced from PagerDuty or OpsGenie.',
    effect: 'Blocks action',
    fields: [
      {
        key: 'required_for',
        label: 'Action key patterns',
        type: 'stringlist',
        placeholder: 'db:*',
        help: 'Glob patterns (filepath.Match) matched against the action key. On-call membership is only enforced for matching actions. Leave empty to disable.',
      },
    ],
  },
  breakglass_cooldown: {
    title: 'Break-Glass Cooldown',
    description:
      'Prevents repeated break-glass invocations within a cooldown window. Operators who repeatedly need emergency override must escalate to a peer review instead.',
    effect: 'Blocks action',
    fields: [
      {
        key: 'cooldown_hours',
        label: 'Cooldown period',
        type: 'number',
        min: 1,
        unit: 'hours',
        help: 'How long after a break-glass invocation before the same operator can invoke it again. Set to 0 to disable.',
      },
    ],
  },
  incident_window_expansion: {
    title: 'Incident Window Expansion',
    description:
      'During a declared incident, overrides all other policies and allows any action regardless of Deny or RequireApproval results. Every expanded action is recorded in the audit log.',
    effect: 'Overrides all policies',
    fields: [],
  },
}

// ── Category groupings ─────────────────────────────────────────────────────

const categories = [
  {
    name: 'temporal',
    label: 'Temporal Controls',
    policyNames: ['time_of_day', 'day_of_week', 'change_freeze_window'],
  },
  {
    name: 'network',
    label: 'Network & Location',
    policyNames: ['source_ip', 'geographic_anomaly'],
  },
  {
    name: 'identity',
    label: 'Identity & Session',
    policyNames: ['step_up_mfa', 'concurrent_session_limit', 'vip_protection', 'self_targeting_block', 'new_operator_probation'],
  },
  {
    name: 'operational',
    label: 'Operational Limits',
    policyNames: ['bulk_action_threshold', 'production_instance_gate', 'integration_health_check', 'on_call_verification'],
  },
  {
    name: 'compliance',
    label: 'Compliance & Emergency',
    policyNames: ['legal_hold_conflict', 'breakglass_cooldown', 'incident_window_expansion'],
  },
]

// ── State ──────────────────────────────────────────────────────────────────

const toastStore = useToastStore()
const policies = ref<PBACPolicy[]>([])
const loading = ref(false)
const error = ref<string | null>(null)
const saving = ref<string | null>(null)

// drafts: policyName → { fieldKey → value }
const drafts = reactive<Record<string, Record<string, unknown>>>({})
// saved snapshots to detect dirty state
const snapshots = reactive<Record<string, Record<string, unknown>>>({})

const dayNames = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']

// ── Helpers ────────────────────────────────────────────────────────────────

function policyByName(name: string): PBACPolicy | undefined {
  return policies.value.find((p) => p.name === name)
}

function effectStyle(effect?: EffectLabel): string {
  switch (effect) {
    case 'Blocks action':
      return 'background: #fee2e2; color: #b91c1c;'
    case 'Requires approval':
      return 'background: #fef9c3; color: #854d0e;'
    case 'Overrides all policies':
      return 'background: #dbeafe; color: #1e40af;'
    case 'Blocks or requires approval':
      return 'background: #ffedd5; color: #c2410c;'
    default:
      return 'background: var(--surface); color: var(--text-muted);'
  }
}

function formatHour(h: number): string {
  const ampm = h < 12 ? 'AM' : 'PM'
  const display = h % 12 === 0 ? 12 : h % 12
  return `${String(display).padStart(2, '0')}:00 ${ampm} (${String(h).padStart(2, '0')}:00)`
}

// ── Draft accessors ────────────────────────────────────────────────────────

function getDraft(policyName: string, key: string): unknown {
  return drafts[policyName]?.[key]
}

function setDraft(policyName: string, key: string, value: unknown) {
  if (!drafts[policyName]) drafts[policyName] = {}
  drafts[policyName][key] = value
}

function isDirty(policyName: string): boolean {
  const d = drafts[policyName]
  const s = snapshots[policyName]
  if (!d || !s) return false
  return JSON.stringify(d) !== JSON.stringify(s)
}

function discardDraft(policyName: string) {
  if (snapshots[policyName]) {
    drafts[policyName] = JSON.parse(JSON.stringify(snapshots[policyName]))
  }
}

// ── Stringlist helpers ─────────────────────────────────────────────────────

function updateListItem(policyName: string, key: string, idx: number, val: string) {
  const arr = [...((getDraft(policyName, key) as string[]) ?? [])]
  arr[idx] = val
  setDraft(policyName, key, arr)
}

function removeListItem(policyName: string, key: string, idx: number) {
  const arr = [...((getDraft(policyName, key) as string[]) ?? [])]
  arr.splice(idx, 1)
  setDraft(policyName, key, arr)
}

function addListItem(policyName: string, key: string) {
  const arr = [...((getDraft(policyName, key) as string[]) ?? [])]
  arr.push('')
  setDraft(policyName, key, arr)
}

// ── Day-of-week helpers ────────────────────────────────────────────────────

function isDaySelected(policyName: string, key: string, dayIndex: number): boolean {
  const arr = (getDraft(policyName, key) as number[]) ?? []
  return arr.includes(dayIndex)
}

function toggleDay(policyName: string, key: string, dayIndex: number) {
  const arr = [...((getDraft(policyName, key) as number[]) ?? [])]
  const pos = arr.indexOf(dayIndex)
  if (pos === -1) arr.push(dayIndex)
  else arr.splice(pos, 1)
  arr.sort((a, b) => a - b)
  setDraft(policyName, key, arr)
}

// ── Load & sync drafts ─────────────────────────────────────────────────────

function initDraft(policy: PBACPolicy) {
  const m = meta[policy.policy_type]
  if (!m || !m.fields.length) return

  const config = policy.config as Record<string, unknown>
  const d: Record<string, unknown> = {}

  for (const field of m.fields) {
    // hourrange is a virtual field — it maps to two keys
    if (field.type === 'hourrange') {
      d['start_hour'] = config['start_hour'] ?? 9
      d['end_hour'] = config['end_hour'] ?? 18
    } else {
      d[field.key] = config[field.key] ?? defaultForField(field)
    }
  }
  drafts[policy.name] = d
  snapshots[policy.name] = JSON.parse(JSON.stringify(d))
}

function defaultForField(field: FieldDef): unknown {
  switch (field.type) {
    case 'number': return 0
    case 'boolean': return false
    case 'text': return ''
    case 'stringlist': return []
    case 'dayofweek': return []
    default: return null
  }
}

// ── API actions ────────────────────────────────────────────────────────────

async function loadPolicies() {
  loading.value = true
  error.value = null
  try {
    policies.value = await listPBACPolicies()
    for (const p of policies.value) {
      initDraft(p)
    }
  } catch {
    error.value = 'Failed to load PBAC policies'
  } finally {
    loading.value = false
  }
}

async function togglePolicy(policyName: string) {
  const p = policyByName(policyName)
  if (!p) return
  try {
    await updatePBACPolicy(p.name, { is_enabled: !p.is_enabled })
    toastStore.success(p.is_enabled ? 'Policy disabled' : 'Policy enabled')
    await loadPolicies()
  } catch {
    toastStore.error('Failed to update policy')
  }
}

async function saveConfig(policyName: string) {
  const d = drafts[policyName]
  if (!d) return
  saving.value = policyName
  try {
    // Build the config object from the draft, merging hourrange fields
    const config: Record<string, unknown> = { ...d }
    await updatePBACPolicy(policyName, { config })
    toastStore.success('Configuration saved')
    snapshots[policyName] = JSON.parse(JSON.stringify(d))
    await loadPolicies()
  } catch {
    toastStore.error('Failed to save configuration')
  } finally {
    saving.value = null
  }
}

onMounted(loadPolicies)
</script>
