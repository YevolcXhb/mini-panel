<template>
  <router-view />
</template>

<script setup lang="ts">
import { useThemeStore } from './store'
const themeStore = useThemeStore()
themeStore.applyTheme()
</script>

<style>
html, body, #app {
  height: 100%;
  width: 100%;
}
</style>
