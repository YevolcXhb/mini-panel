import { defineStore } from 'pinia'
import { ref, computed, watch } from 'vue'

export const useAuthStore = defineStore('auth', () => {
  const token = ref(localStorage.getItem('token') || '')
  const username = ref(localStorage.getItem('username') || '')

  const isLoggedIn = computed(() => !!token.value)

  function setAuth(t: string, u: string) {
    token.value = t
    username.value = u
    localStorage.setItem('token', t)
    localStorage.setItem('username', u)
  }

  function clearAuth() {
    token.value = ''
    username.value = ''
    localStorage.removeItem('token')
    localStorage.removeItem('username')
  }

  return { token, username, isLoggedIn, setAuth, clearAuth }
})

type ThemeMode = 'light' | 'dark' | 'auto'

export const useThemeStore = defineStore('theme', () => {
  const mode = ref<ThemeMode>((localStorage.getItem('theme') as ThemeMode) || 'dark')

  const isDark = computed(() => {
    if (mode.value === 'auto') {
      return window.matchMedia('(prefers-color-scheme: dark)').matches
    }
    return mode.value === 'dark'
  })

  function applyTheme() {
    const root = document.documentElement
    root.classList.remove('light', 'dark')
    root.classList.add(isDark.value ? 'dark' : 'light')
  }

  function setTheme(m: ThemeMode) {
    mode.value = m
    localStorage.setItem('theme', m)
    applyTheme()
  }

  function toggleTheme() {
    const next: ThemeMode = isDark.value ? 'light' : 'dark'
    setTheme(next)
  }

  watch(mode, applyTheme)
  applyTheme()

  return { mode, isDark, setTheme, toggleTheme }
})
