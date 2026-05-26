<template>
  <Teleport to="body">
    <div
      v-if="modelValue"
      class="fixed inset-0 z-40 flex items-center justify-center p-4"
      style="background: rgba(0,0,0,0.6);"
      @click.self="$emit('update:modelValue', false)"
    >
      <div
        class="w-full max-w-md rounded-xl p-6 shadow-2xl"
        style="background: var(--card); border: 1px solid var(--border);"
      >
        <h3 class="text-lg font-semibold mb-2" style="color: var(--text-primary);">{{ title }}</h3>
        <p class="text-sm mb-4" style="color: var(--text-muted);">{{ message }}</p>

        <div v-if="showNote" class="mb-4">
          <label class="block text-sm font-medium mb-1" style="color: var(--text-primary);">
            {{ noteLabel ?? 'Note' }}
          </label>
          <textarea
            v-model="noteValue"
            rows="3"
            class="w-full rounded-lg px-3 py-2 text-sm resize-none"
            style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
            :placeholder="notePlaceholder ?? 'Add a note...'"
          />
        </div>

        <div class="flex gap-3 justify-end">
          <button
            class="px-4 py-2 rounded-lg text-sm font-medium"
            style="background: var(--border); color: var(--text-primary);"
            @click="$emit('update:modelValue', false)"
          >
            Cancel
          </button>
          <button
            class="px-4 py-2 rounded-lg text-sm font-medium text-white"
            :class="danger ? 'bg-red-500 hover:bg-red-600' : 'bg-indigo-500 hover:bg-indigo-600'"
            :disabled="loading"
            @click="confirm"
          >
            <span v-if="loading">Working...</span>
            <span v-else>{{ confirmLabel ?? 'Confirm' }}</span>
          </button>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { ref } from 'vue'

const props = defineProps<{
  modelValue: boolean
  title: string
  message: string
  danger?: boolean
  confirmLabel?: string
  showNote?: boolean
  noteLabel?: string
  notePlaceholder?: string
  loading?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', val: boolean): void
  (e: 'confirm', note?: string): void
}>()

const noteValue = ref('')

function confirm() {
  emit('confirm', props.showNote ? noteValue.value : undefined)
}
</script>
