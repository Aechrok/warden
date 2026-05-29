<template>
  <div class="rounded-xl flex flex-col overflow-hidden" style="background: var(--card); border: 1px solid var(--border);">
    <!-- Header -->
    <div
      class="px-4 py-3 text-xs font-semibold uppercase tracking-wider flex-shrink-0"
      style="border-bottom: 1px solid var(--border); color: var(--text-muted); letter-spacing: 0.07em;"
    >
      {{ singleInstance ? `Actions — ${groups[0]?.identity.instance_name}` : 'Actions' }}
    </div>

    <!-- Legal hold banner -->
    <div
      v-if="onHold"
      class="mx-3 mt-3 px-3 py-2 rounded-lg text-xs flex items-center gap-2 flex-shrink-0"
      style="background: rgba(168,85,247,.12); border: 1px solid rgba(168,85,247,.3); color: #a855f7;"
    >
      <span>⚠</span>
      <span>Identity is on legal hold. Destructive actions are restricted.</span>
    </div>

    <!-- Execution form -->
    <div v-if="executingAction && executingIdentity" class="p-4 flex flex-col gap-3">
      <div class="flex items-center justify-between">
        <span class="text-sm font-medium" style="color: var(--text-primary);">{{ executingAction.label }}</span>
        <button
          class="text-xs px-2 py-1 rounded"
          style="color: var(--text-muted); background: var(--surface);"
          @click="$emit('clear')"
        >
          ✕ Cancel
        </button>
      </div>

      <p v-if="executingAction.description" class="text-xs" style="color: var(--text-muted);">
        {{ executingAction.description }}
      </p>

      <!-- Params -->
      <div v-for="param in executingAction.params ?? []" :key="param.key" class="flex flex-col gap-1">
        <label class="text-xs font-medium" style="color: var(--text-primary);">
          {{ param.label }}{{ param.required ? ' *' : '' }}
        </label>
        <select
          v-if="param.type === 'select'"
          :value="paramValues[param.key] as string"
          class="rounded px-2 py-1.5 text-xs"
          style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
          @change="$emit('param-change', param.key, ($event.target as HTMLSelectElement).value)"
        >
          <option v-for="opt in param.options" :key="opt" :value="opt">{{ opt }}</option>
        </select>
        <input
          v-else
          :value="paramValues[param.key] as string"
          :type="param.type === 'boolean' ? 'checkbox' : 'text'"
          class="rounded px-2 py-1.5 text-xs"
          style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
          @input="$emit('param-change', param.key, ($event.target as HTMLInputElement).value)"
        />
      </div>

      <ErrorBanner :message="execError" />

      <button
        class="w-full py-2 rounded-lg text-xs font-medium text-white transition-colors"
        :class="executingAction.destructive ? 'bg-red-500 hover:bg-red-600' : ''"
        :style="!executingAction.destructive ? 'background: var(--accent);' : ''"
        :disabled="executing"
        @click="$emit('submit')"
      >
        {{ executing ? 'Executing…' : executingAction.destructive ? 'Execute (Destructive)' : 'Execute' }}
      </button>
    </div>

    <!-- Action groups -->
    <template v-else>
      <div
        v-for="group in groups"
        :key="group.identity.instance_id"
        class="px-3 py-3"
        :style="singleInstance ? '' : 'border-bottom: 1px solid var(--border);'"
      >
        <!-- Group label (only in grouped/overview mode) -->
        <div
          v-if="!singleInstance"
          class="text-xs font-semibold mb-2 flex items-center gap-2"
          style="color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.06em;"
        >
          <span class="w-1.5 h-1.5 rounded-full flex-shrink-0" style="background: #22c55e;"></span>
          {{ group.identity.instance_name }}
        </div>

        <!-- Buttons -->
        <div class="flex flex-col gap-1">
          <template v-for="action in group.actions" :key="action.key">
            <!-- Legal-hold-blocked (destructive + on hold) -->
            <button
              v-if="onHold && action.destructive"
              class="w-full text-left px-3 py-1.5 rounded-lg text-xs font-medium cursor-not-allowed"
              style="background: rgba(168,85,247,.12); color: #a855f7; border: 1px solid rgba(168,85,247,.25); opacity: 0.7;"
              :title="`Legal hold active — ${action.key} is blocked`"
              disabled
            >
              {{ action.label }}
            </button>

            <!-- Destructive (but not hold-blocked) -->
            <button
              v-else-if="action.destructive"
              class="w-full text-left px-3 py-1.5 rounded-lg text-xs font-medium transition-colors"
              style="background: rgba(239,68,68,.12); color: #ef4444; border: 1px solid rgba(239,68,68,.3);"
              @click="$emit('select', action, group.identity)"
            >
              {{ action.label }}
            </button>

            <!-- Normal -->
            <button
              v-else
              class="w-full text-left px-3 py-1.5 rounded-lg text-xs font-medium transition-colors"
              style="background: var(--surface); color: var(--text-primary); border: 1px solid var(--border);"
              @click="$emit('select', action, group.identity)"
            >
              {{ action.label }}
            </button>
          </template>

          <p v-if="group.actions.length === 0" class="text-xs py-1" style="color: var(--text-muted);">
            No actions available
          </p>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import type { ActionDef, Identity } from '../api/types'
import ErrorBanner from './ErrorBanner.vue'

export interface ActionGroup {
  identity: Identity
  actions: ActionDef[]
}

defineProps<{
  groups: ActionGroup[]
  onHold: boolean
  singleInstance?: boolean
  executingAction: ActionDef | null
  executingIdentity: Identity | null
  paramValues: Record<string, unknown>
  execError: string | null
  executing: boolean
}>()

defineEmits<{
  (e: 'select', action: ActionDef, identity: Identity): void
  (e: 'clear'): void
  (e: 'submit'): void
  (e: 'param-change', key: string, value: unknown): void
}>()
</script>
