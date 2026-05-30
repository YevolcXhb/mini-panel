import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useThemeStore } from '../store'

describe('useThemeStore outside component', () => {
  beforeEach(() => {
    const pinia = createPinia()
    setActivePinia(pinia)
    localStorage.clear()
    document.documentElement.className = ''
  })

  it('should call useThemeStore() without error', () => {
    expect(() => {
      const store = useThemeStore()
      expect(store).toBeDefined()
      expect(store.mode).toBe('dark')
    }).not.toThrow()
  })

  it('should have applyTheme method', () => {
    const store = useThemeStore()
    expect(typeof store.applyTheme).toBe('function')
  })

  it('applyTheme should add dark class by default', () => {
    const store = useThemeStore()
    store.applyTheme()
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(document.documentElement.classList.contains('light')).toBe(false)
  })

  it('setTheme should switch to light', () => {
    const store = useThemeStore()
    store.setTheme('light')
    expect(store.mode).toBe('light')
    expect(document.documentElement.classList.contains('light')).toBe(true)
  })

  it('toggleTheme should toggle between light and dark', () => {
    const store = useThemeStore()
    store.setTheme('dark')
    store.toggleTheme()
    expect(store.mode).toBe('light')
    store.toggleTheme()
    expect(store.mode).toBe('dark')
  })

  it('should persist theme to localStorage', () => {
    const store = useThemeStore()
    store.setTheme('light')
    expect(localStorage.getItem('theme')).toBe('light')
  })
})
