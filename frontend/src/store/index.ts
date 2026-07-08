import { defineStore } from 'pinia'
import { ref, computed, watch } from 'vue'

function decodeToken(token: string): { userId: number; role: string; permissions: string[] } {
  try {
    const payload = token.split('.')[1]
    const json = JSON.parse(atob(payload.replace(/-/g, '+').replace(/_/g, '/')))
    let perms: string[] = []
    if (Array.isArray(json.permissions)) {
      perms = json.permissions.map((x: any) => String(x))
    } else if (typeof json.permissions === 'string' && json.permissions) {
      try {
        const arr = JSON.parse(json.permissions)
        if (Array.isArray(arr)) perms = arr.map((x: any) => String(x))
      } catch { /* ignore */ }
    }
    return {
      userId: Number(json.user_id) || 0,
      role: String(json.role || ''),
      permissions: perms
    }
  } catch {
    return { userId: 0, role: '', permissions: [] }
  }
}

export const useAuthStore = defineStore('auth', () => {
  const DEFAULT_USER_PERMISSIONS = ['/dashboard', '/monitor', '/logs']

  const token = ref(localStorage.getItem('token') || '')
  const username = ref(localStorage.getItem('username') || '')
  const role = ref(localStorage.getItem('role') || '')
  const userId = ref(Number(localStorage.getItem('userId') || '0'))
  const rawPermissions = JSON.parse(localStorage.getItem('permissions') || '[]')
  const permissions = ref<string[]>(Array.isArray(rawPermissions) ? rawPermissions : [])

  const isLoggedIn = computed(() => !!token.value)
  const isAdmin = computed(() => role.value === 'admin')

  // 兼容老数据：普通用户 permissions 为空时返回默认查看权限
  const effectivePermissions = computed(() => {
    if (role.value === 'admin') return []
    if (!permissions.value || permissions.value.length === 0) return DEFAULT_USER_PERMISSIONS
    return permissions.value
  })

  function setAuth(t: string, u: string, r: string = '', perms: string[] = []) {
    token.value = t
    username.value = u
    role.value = r
    const decoded = decodeToken(t)
    if (decoded.userId) userId.value = decoded.userId
    if (perms && perms.length) {
      permissions.value = perms
    } else if (decoded.permissions.length) {
      permissions.value = decoded.permissions
    }
    localStorage.setItem('token', t)
    localStorage.setItem('username', u)
    localStorage.setItem('role', r)
    if (decoded.userId) localStorage.setItem('userId', String(decoded.userId))
    localStorage.setItem('permissions', JSON.stringify(permissions.value))
  }

  function clearAuth() {
    token.value = ''
    username.value = ''
    role.value = ''
    userId.value = 0
    permissions.value = []
    localStorage.removeItem('token')
    localStorage.removeItem('username')
    localStorage.removeItem('role')
    localStorage.removeItem('userId')
    localStorage.removeItem('permissions')
  }

  function hasFeature(key: string): boolean {
    if (role.value === 'admin') return true
    return effectivePermissions.value.includes(key)
  }

  return { token, username, role, userId, permissions, isLoggedIn, isAdmin, setAuth, clearAuth, hasFeature, effectivePermissions }
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

  return { mode, isDark, applyTheme, setTheme, toggleTheme }
})
