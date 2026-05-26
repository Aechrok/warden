<template>
  <RouterView />
  <ToastContainer />
</template>

<script setup lang="ts">
import { RouterView } from 'vue-router'
import ToastContainer from './components/ToastContainer.vue'
import { useThemeStore } from './stores/theme'

// Initialize theme store on app startup
useThemeStore()
</script>
